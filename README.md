# HTTP QUERY Method — RFC 10008 Implementation

A Go implementation and interactive browser demo of [RFC 10008](https://www.rfc-editor.org/rfc/rfc10008) — the HTTP QUERY method. QUERY is a safe, idempotent, cacheable HTTP method that accepts a request body, filling the gap between GET and POST for complex search APIs.

**Live demo:** https://http-query-method.pages.dev/

## What is QUERY?

| Property | GET | **QUERY** | POST |
|---|---|---|---|
| Safe (no state change) | Yes | **Yes** | No |
| Idempotent | Yes | **Yes** | No |
| Cacheable by default | Yes | **Yes** | No |
| Carries a request body | Undefined | **Yes** | Yes |
| Query in URI | Required | Optional | No |
| Accept-Query negotiation | No | **Yes** | No |

## Project structure

```
http-query-method/
├── pkg/query/          RFC 10008 QUERY handler (Content-Type validation, 415 + Accept-Query)
├── pkg/cache/          RFC 7234 + 10008 body-keyed response cache (SHA-256 key)
├── cmd/demo-wasm/      WebAssembly demo binary (7 exported functions)
├── cmd/server/         Standalone HTTP server with QUERY + caching middleware
├── demo/               Browser demo (index.html + query.wasm + wasm_exec.js)
├── scripts/            check_standards.py — daily RFC tracker
├── .github/workflows/  standards-tracker.yml — daily cron job
├── docs/               standards-tracking.md — due-diligence checklist
└── standards-baseline.json
```

## Standards

| Standard | Lifecycle | PoC Status | Notes |
|---|---|---|---|
| [RFC 10008](https://www.rfc-editor.org/rfc/rfc10008) — HTTP QUERY Method | Published RFC | Implemented | Core: `pkg/query` |
| [RFC 9110](https://www.rfc-editor.org/rfc/rfc9110) — HTTP Semantics | Published RFC | Implemented | Method semantics, status codes |
| [RFC 7234](https://www.rfc-editor.org/rfc/rfc7234) — HTTP Caching | Published RFC | Implemented | Body-keyed cache: `pkg/cache` |
| [RFC 8288](https://www.rfc-editor.org/rfc/rfc8288) — Web Linking | Published RFC | Monitoring | Content-Location semantics |
| [RFC 7807](https://www.rfc-editor.org/rfc/rfc7807) — Problem Details | Published RFC | Monitoring | Error body format candidate |

**Standards update process**: The GitHub Actions workflow in `.github/workflows/standards-tracker.yml` checks for RFC errata and obsoleted-by relationships daily. See [`docs/standards-tracking.md`](docs/standards-tracking.md) for the due-diligence checklist that must be completed for every finding.

## Quick start

```bash
# Run tests
go test ./...

# Build WASM demo
GOOS=js GOARCH=wasm go build -o demo/query.wasm ./cmd/demo-wasm/

# Serve the demo locally
cd demo && python3 -m http.server 8080
# Open http://localhost:8080
```

## Usage

### Server

```go
import "github.com/jralmaraz/http-query-method/pkg/query"

// JSON query handler
h := query.JSONQueryHandler(func(q map[string]interface{}) (interface{}, error) {
    results := db.Search(q["filter"], q["limit"])
    return map[string]interface{}{"results": results}, nil
})

// Automatically enforces:
//   - 400 on missing Content-Type
//   - 415 + Accept-Query header on unsupported format
//   - 422 on invalid JSON body
//   - 405 + Allow header on non-QUERY requests
http.Handle("/users", h)
```

### Client (Go)

```go
req, _ := http.NewRequest("QUERY", "https://api.example.com/users", body)
req.Header.Set("Content-Type", "application/json")
resp, _ := client.Do(req)
// QUERY is safe + idempotent — retry on timeout is safe
```

### Client (curl)

```bash
curl -X QUERY https://api.example.com/users \
  -H "Content-Type: application/json" \
  -d '{"filter":{"status":"active"},"limit":50}'
```

### Caching middleware

```go
import "github.com/jralmaraz/http-query-method/pkg/cache"

store := cache.NewStore(5 * time.Minute)
cachedHandler := cache.NewCachingHandler(queryHandler, store)
// X-Cache: MISS on first request
// X-Cache: HIT on repeat with identical body
```

## Demo

The browser demo (`demo/index.html`) runs entirely in-browser via WebAssembly:

- **Overview** — method properties and quick comparison table
- **Protocol Flow** — animated SVG showing cache MISS, HIT, and 415 negotiation
- **vs GET & POST** — side-by-side comparison with language switcher (curl, Go, JS, Python, Rust)
- **Caching** — live WASM cache demo; implementation examples (Go, nginx, Varnish VCL)
- **Error Matrix** — RFC 10008 error scenarios with WASM simulator
- **DB Integration** — backend integration patterns (PostgreSQL, GraphQL, MongoDB)
- **Playground** — live QUERY request builder
- **Standards** — tracker table with lifecycle legend

## Goals of this project

This project explores RFC 10008 from a **backend and integration** perspective:

- What does QUERY enable that GET and POST cannot do cleanly?
- How do web servers (nginx, Varnish, Caddy) implement caching for body-keyed requests?
- How do database drivers expose query interfaces over HTTP QUERY?
- What does client library support look like across languages?

This is separate from the [WIMSE Identity Fabric](https://github.com/jralmaraz/wimse-identity-fabric) (workload identity) and [WIMSE Agent Fabric](https://github.com/jralmaraz/wimse-agent-fabric) (AI agent identity) projects.
