// Package fanwebcaddy implements a Caddy event handler that reports
// certificate lifecycle events (cert_obtained / cert_failed) back to the
// fanwebbaidu service.
package fanwebcaddy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"

	// Import caddyevents only for the Handler interface assertion.
	caddyevents "github.com/caddyserver/caddy/v2/modules/caddyevents"
)

func init() {
	caddy.RegisterModule(CertEventHandler{})
}

// CertEventHandler handles Caddy certificate lifecycle events and reports
// them to the fanwebbaidu service.
//
// Caddyfile usage:
//
//	events {
//	    on cert_obtained fanweb https://fanweb.example.com/api/v1/fan/fanCaddyOutside/certEvent
//	    on cert_failed fanweb https://fanweb.example.com/api/v1/fan/fanCaddyOutside/certEvent
//	}
//
// Or, to subscribe to all events and filter internally:
//
//	events {
//	    on * fanweb https://fanweb.example.com/api/v1/fan/fanCaddyOutside/certEvent
//	}
//
// JSON usage:
//
//	{
//	    "handler": "fanweb",
//	    "endpoint": "https://fanweb.example.com/api/v1/fan/fanCaddyOutside/certEvent"
//	}
type CertEventHandler struct {
	// Endpoint is the full URL of the fanwebbaidu certEvent callback.
	Endpoint string `json:"endpoint,omitempty"`
}

// CaddyModule returns the Caddy module information.
func (CertEventHandler) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "events.handlers.fanweb",
		New: func() caddy.Module { return new(CertEventHandler) },
	}
}

// Provision validates the handler configuration.
func (h *CertEventHandler) Provision(_ caddy.Context) error {
	h.Endpoint = strings.TrimSpace(h.Endpoint)
	if h.Endpoint == "" {
		return fmt.Errorf("fanweb cert event handler: endpoint is required")
	}
	return nil
}

// UnmarshalCaddyfile parses the Caddyfile block:
//
//	fanweb <endpoint>
func (h *CertEventHandler) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		if d.NextArg() {
			h.Endpoint = d.Val()
		}
		if d.NextArg() {
			return d.ArgErr()
		}
	}
	return nil
}

// Handle processes Caddy events and forwards certificate events to fanwebbaidu.
func (h *CertEventHandler) Handle(ctx context.Context, e caddy.Event) error {
	name := e.Name
	if name != "cert_obtained" && name != "cert_failed" {
		// Silently ignore events we are not interested in.
		// This allows the handler to be safely subscribed via "on *".
		return nil
	}

	identifier, _ := e.Data["identifier"].(string)
	if identifier == "" {
		return nil
	}

	payload := certEventReq{
		EventType:  name,
		Domain:     identifier,
		Issuer:     stringValue(e.Data["issuer"]),
		Renewal:    boolValue(e.Data["renewal"]),
		Error:      stringValue(e.Data["error"]),
		StorageKey: stringValue(e.Data["storage_key"]),
	}

	return h.report(ctx, payload)
}

func (h *CertEventHandler) report(ctx context.Context, payload certEventReq) error {
	buf, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal cert event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.Endpoint, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("build cert event request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "fanwebcaddy-events/1.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("report cert event: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cert event HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var r apiResp
	if err = json.Unmarshal(data, &r); err != nil {
		return nil
	}
	if r.Code != 0 {
		return fmt.Errorf("cert event api error %d: %s", r.Code, r.Message)
	}
	return nil
}

type certEventReq struct {
	EventType  string `json:"eventType"`  // cert_obtained / cert_failed
	Domain     string `json:"domain"`     // certificate identifier
	Issuer     string `json:"issuer"`     // issuer name
	Renewal    bool   `json:"renewal"`    // true if renewal
	Error      string `json:"error"`      // error message for cert_failed
	StorageKey string `json:"storageKey"` // certmagic storage key
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

func boolValue(v any) bool {
	b, _ := v.(bool)
	return b
}

// ---------- compile-time interface assertions ----------

var (
	_ caddy.Module          = (*CertEventHandler)(nil)
	_ caddy.Provisioner     = (*CertEventHandler)(nil)
	_ caddyfile.Unmarshaler = (*CertEventHandler)(nil)
	_ caddyevents.Handler   = (*CertEventHandler)(nil)
)
