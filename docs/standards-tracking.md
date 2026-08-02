# Standards Tracking Policy

This document explains how the HTTP QUERY Method PoC tracks the IETF standards it implements.

## Tracked standards

| Standard | Lifecycle Status | PoC Status | Notes |
|---|---|---|---|
| RFC 10008 — HTTP QUERY Method | Published RFC | Implemented | Core of this project |
| RFC 9110 — HTTP Semantics | Published RFC | Implemented | Method semantics, status codes |
| RFC 7234 — HTTP/1.1 Caching | Published RFC | Implemented | Cache body-keyed lookup |
| RFC 8288 — Web Linking | Published RFC | Monitoring | Content-Location semantics |
| RFC 7807 — Problem Details | Published RFC | Monitoring | Error body format candidate |

## Standard Lifecycle (IETF)

```
Individual Draft → WG Draft → Last Call → IESG Review → Published RFC
```

- **Individual Draft**: submitted by individuals, not yet adopted by any WG; highly unstable.
- **WG Draft (`draft-ietf-*`)**: adopted by a Working Group; stable enough to prototype against with caution.
- **Last Call**: community-wide review; breaking changes unlikely but possible.
- **IESG Review**: final technical review before publication.
- **Published RFC**: stable; production-ready. Errata may be filed; RFC can be obsoleted by a successor.

## PoC Status definitions

| Status | Meaning |
|---|---|
| **Implemented** | Has Go implementation + animated demo tab |
| **Monitoring** | Tracked in `standards-baseline.json`; no implementation yet |
| **Excluded** | Explicitly out of scope; decision documented here |

## Tracking Mechanisms

### 1. IETF Datatracker API (revision detection for WG drafts)

`scripts/check_standards.py` polls `https://datatracker.ietf.org/api/v1/doc/document/<id>/` daily. For WG drafts, a new `rev` opens a GitHub issue.

### 2. RFC Errata API (for published RFCs)

For published RFCs, the checker queries `https://www.rfc-editor.org/errata_search.php?rfc=<num>&output=json` and reports any new **verified** errata.

### 3. Obsoleted-by check

The Datatracker API `obsoleted_by` field is checked for each RFC. If an RFC is superseded, a HIGH-priority issue is opened immediately.

### 4. HTTPbis WG feed (new draft discovery)

The checker also polls `https://datatracker.ietf.org/group/httpbis/documents/feed/`. Any new draft ID not in `standards-baseline.json` is reported for triage.

---

## Due-Diligence Checklist for Every Standards Tracker Finding

### 1. Triage

- [ ] **Classify priority**: High (breaking / obsoleted) / Medium (errata / new draft) / Low (not applicable)
- [ ] **Identify affected packages**: `pkg/query`, `pkg/cache`, `cmd/demo-wasm`, `demo/index.html`

### 2. Diff review

- [ ] **Method semantics**: Has the QUERY method definition changed? Update `pkg/query/handler.go`
- [ ] **Status codes**: Any new or changed status codes? Update handler + tests
- [ ] **Header names**: Any new required headers (Accept-Query, Content-Location)? Update implementation
- [ ] **Caching key semantics**: Any change to how cache keys are computed? Update `pkg/cache`

### 3. Threat model

- [ ] **Cache poisoning**: Can the changed spec allow a crafted request to poison a shared cache?
- [ ] **Request body injection**: Can a malformed body bypass Content-Type validation?
- [ ] **Body size DoS**: Is there a new max body size guidance? Update the 10MB limit in `ParseRequest`

### 4. Animated demo — required for every implemented standard

Every standard that is **implemented** (not just monitored) **must** have an animated demo in `demo/index.html`.

- [ ] **Animated flow exists?** Check whether the relevant demo tab has an SVG animation or step-by-step flow.
  - If **yes**: verify the animation still accurately reflects the updated semantics after the change.
  - If **no**: create one using the pattern from the Protocol Flow tab (SVG `<circle>` packet animation + `.vstep` highlights).
- [ ] Update the **Standards Tracker table** in `demo/index.html`: add/update the row, set `Impl. commit`
- [ ] Update the **spec-ref** in affected demo tabs
- [ ] Update `docs/standards-tracking.md` Tracked standards table if the status changes
- [ ] Update the **README** if a new standard is added
- [ ] Update **`standards-baseline.json`**: set `implemented_rev` and `last_known_rev`

### 5. Test coverage

- [ ] Add or update a test in `pkg/query/handler_test.go` or `pkg/cache/cache_test.go`
- [ ] Run `go test ./... -race` — all tests pass

### 6. Commit and close

- [ ] Commit: `fix/feat(standard): align pkg with <rfc-id>`
- [ ] Update `Impl. commit` SHA in the Standards Tracker demo table
- [ ] Push — verify CI passes
- [ ] Post summary comment and close the issue
