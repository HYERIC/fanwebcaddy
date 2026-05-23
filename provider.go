// Package fanwebcaddy implements a Caddy DNS provider module that manages
// TXT records via the fanwebbaidu service, enabling ACME DNS-01 challenge
// for wildcard SSL certificates.
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
	"github.com/libdns/libdns"
)

func init() {
	caddy.RegisterModule(Provider{})
}

// Provider implements a Caddy DNS provider that delegates TXT record
// operations to the fanwebbaidu service, which in turn calls zxdns.
//
// Caddyfile usage:
//
//	tls {
//	    dns fanweb <endpoint>
//	}
//
// JSON usage:
//
//	"challenges": {
//	    "dns": {
//	        "provider": {
//	            "name": "fanweb",
//	            "endpoint": "https://fanweb.example.com"
//	        }
//	    }
//	}
type Provider struct {
	// Endpoint is the base URL of the fanwebbaidu service.
	// Example: https://fanweb.example.com
	Endpoint string `json:"endpoint,omitempty"`
}

// CaddyModule returns the Caddy module information.
func (Provider) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "dns.providers.fanweb",
		New: func() caddy.Module { return new(Provider) },
	}
}

// Provision validates configuration and prepares the provider.
func (p *Provider) Provision(_ caddy.Context) error {
	p.Endpoint = strings.TrimRight(p.Endpoint, "/")
	if p.Endpoint == "" {
		return fmt.Errorf("fanweb dns: endpoint is required")
	}
	return nil
}

// UnmarshalCaddyfile parses the Caddyfile block:
//
//	dns fanweb <endpoint>
func (p *Provider) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		if d.NextArg() {
			p.Endpoint = d.Val()
		}
		if d.NextArg() {
			return d.ArgErr()
		}
	}
	return nil
}

// ---------- libdns interface ----------

// GetRecords returns records in the zone. Not required for ACME DNS-01;
// returns empty list.
func (p *Provider) GetRecords(_ context.Context, _ string) ([]libdns.Record, error) {
	return nil, nil
}

// AppendRecords writes TXT records to the zone via fanwebbaidu/txtSet.
// Only TXT records are processed; others are ignored.
func (p *Provider) AppendRecords(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error) {
	for i, rec := range recs {
		if rec.Type != "TXT" {
			continue
		}
		fqdn := absoluteName(rec.Name, zone)
		ttl := int(rec.TTL.Seconds())
		if ttl <= 0 {
			ttl = 120
		}
		if err := p.apiTxtSet(ctx, fqdn, rec.Value, ttl); err != nil {
			return recs[:i], fmt.Errorf("fanweb dns: set TXT %s: %w", fqdn, err)
		}
	}
	return recs, nil
}

// SetRecords sets TXT records in the zone (delegates to AppendRecords).
func (p *Provider) SetRecords(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error) {
	return p.AppendRecords(ctx, zone, recs)
}

// DeleteRecords removes TXT records from the zone via fanwebbaidu/txtDel.
func (p *Provider) DeleteRecords(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error) {
	for i, rec := range recs {
		if rec.Type != "TXT" {
			continue
		}
		fqdn := absoluteName(rec.Name, zone)
		if err := p.apiTxtDel(ctx, fqdn); err != nil {
			return recs[:i], fmt.Errorf("fanweb dns: delete TXT %s: %w", fqdn, err)
		}
	}
	return recs, nil
}

// ---------- fanwebbaidu API ----------

type txtSetReq struct {
	Domain string `json:"domain"` // FQDN, e.g. _acme-challenge.example.com.
	Value  string `json:"value"`  // TXT record value (ACME token)
	TTL    int    `json:"ttl"`    // TTL in seconds
}

type txtDelReq struct {
	Domain string `json:"domain"` // FQDN to delete
}

type apiResp struct {
	Code    int    `json:"code"` // 0 = success
	Message string `json:"message"`
}

func (p *Provider) apiTxtSet(ctx context.Context, domain, value string, ttl int) error {
	return p.doRequest(ctx, http.MethodPost, "/api/v1/fan/fanCaddyOutside/txtSet",
		txtSetReq{Domain: domain, Value: value, TTL: ttl})
}

func (p *Provider) apiTxtDel(ctx context.Context, domain string) error {
	return p.doRequest(ctx, http.MethodDelete, "/api/v1/fan/fanCaddyOutside/txtDel",
		txtDelReq{Domain: domain})
}

func (p *Provider) doRequest(ctx context.Context, method, path string, body any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.Endpoint+path, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var r apiResp
	if err = json.Unmarshal(data, &r); err != nil {
		return nil // HTTP 200 but non-standard body; treat as success
	}
	if r.Code != 0 {
		return fmt.Errorf("api error %d: %s", r.Code, r.Message)
	}
	return nil
}

// ---------- helper ----------

// absoluteName converts a relative record name to a fully-qualified domain name.
// zone should end with a dot (e.g. "example.com.").
func absoluteName(name, zone string) string {
	zone = strings.TrimSuffix(zone, ".")
	name = strings.TrimSuffix(name, ".")
	if name == "" || name == "@" {
		return zone + "."
	}
	// Already absolute (contains the zone)
	if strings.HasSuffix(name, "."+zone) || name == zone {
		return name + "."
	}
	return name + "." + zone + "."
}

// ---------- compile-time interface assertions ----------

var (
	_ caddy.Module          = (*Provider)(nil)
	_ caddy.Provisioner     = (*Provider)(nil)
	_ caddyfile.Unmarshaler = (*Provider)(nil)
	_ libdns.RecordGetter   = (*Provider)(nil)
	_ libdns.RecordAppender = (*Provider)(nil)
	_ libdns.RecordSetter   = (*Provider)(nil)
	_ libdns.RecordDeleter  = (*Provider)(nil)
)
