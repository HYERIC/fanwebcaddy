# fanwebcaddy — Caddy DNS Provider Plugin

Caddy DNS-01 provider plugin. Delegates TXT record operations to **fanwebbaidu**,
which internally calls **zxdns** to complete the actual DNS record write/delete.

## Architecture

```
Caddy (DNS-01 challenge)
    ↓  AppendRecords / DeleteRecords
fanwebcaddy plugin
    ↓  POST /fan/fanCaddyOutside/txtSet
    ↓  DELETE /fan/fanCaddyOutside/txtDel
fanwebbaidu service
    ↓  internal call
zxdns (TXT record management)
    ↓
Let's Encrypt verifies _acme-challenge TXT
    ↓
Wildcard cert issued
```

## Build

```bash
xcaddy build \
  --with fanweb/caddy-dns-fanweb=../fanwebcaddy
```

Or after publishing to GitHub:

```bash
xcaddy build \
  --with github.com/yourorg/caddy-dns-fanweb
```

## Configuration

### Caddyfile

```caddyfile
*.example.com example.com {
    tls {
        dns fanweb https://fanweb.example.com
    }
}
```

### JSON (Admin API — used by fanwebbaidu Push())

```json
{
  "@id": "fansitetls_example_com",
  "subjects": ["*.example.com", "example.com"],
  "issuers": [{
    "module": "acme",
    "email": "your@email.com",
    "challenges": {
      "dns": {
        "provider": {
          "name": "fanweb",
          "endpoint": "https://fanweb.example.com"
        }
      }
    }
  }]
}
```

## fanwebbaidu API Contract

The plugin calls two endpoints on the fanwebbaidu service:

### POST /fan/fanCaddyOutside/txtSet

Write a TXT record (ACME DNS-01 challenge).

**Request:**
```json
{
  "domain": "_acme-challenge.example.com.",
  "value":  "LE_token_xxxxx",
  "ttl":    120
}
```

**Response:**
```json
{ "code": 0, "message": "success" }
```

Error codes:
- `403` / `code != 0` — domain not registered or rate limit exceeded

### DELETE /fan/fanCaddyOutside/txtDel

Delete the TXT record after ACME verification completes.

**Request:**
```json
{
  "domain": "_acme-challenge.example.com."
}
```

**Response:**
```json
{ "code": 0, "message": "success" }
```

## zxdns API Contract

See below for the interface that zxdns must implement.
fanwebbaidu calls these internally.

### POST /api/v1/dns/txt

```json
{
  "domain": "_acme-challenge.example.com",
  "value":  "LE_token_xxxxx",
  "ttl":    120
}
```

### DELETE /api/v1/dns/txt

```json
{
  "domain": "_acme-challenge.example.com"
}
```
