//go:build js && wasm

// Package main compiles to WebAssembly and exposes RFC 10008 QUERY method
// demonstration functions to the browser demo.
package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"syscall/js"
	"time"
)

func main() {
	js.Global().Set("queryInit", js.FuncOf(queryInit))
	js.Global().Set("queryDemo", js.FuncOf(queryDemo))
	js.Global().Set("queryCacheDemo", js.FuncOf(queryCacheDemo))
	js.Global().Set("queryVsGet", js.FuncOf(queryVsGet))
	js.Global().Set("queryVsPost", js.FuncOf(queryVsPost))
	js.Global().Set("queryErrorDemo", js.FuncOf(queryErrorDemo))
	js.Global().Set("queryDBDemo", js.FuncOf(queryDBDemo))

	// Signal to the page that WASM is loaded.
	js.Global().Get("document").Call("dispatchEvent",
		js.Global().Get("CustomEvent").New("wasm-ready", map[string]interface{}{"detail": "http-query-method"}))

	// Keep the goroutine alive.
	select {}
}

// queryInit returns version info about the WASM module.
func queryInit(_ js.Value, _ []js.Value) interface{} {
	return map[string]interface{}{
		"ok":      true,
		"version": "RFC 10008 (published 2024-10)",
		"method":  "QUERY",
	}
}

// queryDemo simulates a QUERY request to a search endpoint.
// args[0]: JSON string with query parameters
// Returns a simulated server response object.
func queryDemo(_ js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return errorResult("missing query argument")
	}
	queryBody := args[0].String()

	var q map[string]interface{}
	if err := json.Unmarshal([]byte(queryBody), &q); err != nil {
		return errorResult("invalid JSON: " + err.Error())
	}

	// Simulate server-side query processing.
	filter, _ := q["filter"].(string)
	limit := 10
	if l, ok := q["limit"].(float64); ok {
		limit = int(l)
	}

	// Fake dataset.
	all := []map[string]interface{}{
		{"id": 1, "name": "Alice", "status": "active", "dept": "Engineering"},
		{"id": 2, "name": "Bob", "status": "inactive", "dept": "Sales"},
		{"id": 3, "name": "Carol", "status": "active", "dept": "Engineering"},
		{"id": 4, "name": "Dave", "status": "active", "dept": "HR"},
		{"id": 5, "name": "Eve", "status": "pending", "dept": "Sales"},
		{"id": 6, "name": "Frank", "status": "active", "dept": "Engineering"},
	}

	var results []interface{}
	for _, r := range all {
		if filter == "" || r["status"] == filter {
			results = append(results, r)
		}
		if len(results) >= limit {
			break
		}
	}
	if results == nil {
		results = []interface{}{}
	}

	bodyHash := simHash(queryBody)

	return map[string]interface{}{
		"ok": true,
		"request": map[string]interface{}{
			"method":       "QUERY",
			"url":          "https://api.example.com/users",
			"content_type": "application/json",
			"body":         queryBody,
			"body_hash":    bodyHash,
		},
		"response": map[string]interface{}{
			"status":       200,
			"content_type": "application/json",
			"body": map[string]interface{}{
				"total":   len(results),
				"results": results,
			},
			"headers": map[string]interface{}{
				"Content-Type":    "application/json",
				"Content-Length":  estimateJSONLen(results),
				"X-Cache":         "MISS",
				"Cache-Control":   "max-age=300",
				"Content-Location": "/users/queries/q-" + bodyHash[:8],
			},
		},
		"cache_key": bodyHash,
		"latency_ms": 12,
	}
}

// queryCacheDemo demonstrates the caching behaviour of QUERY responses.
// args[0]: JSON query body (same body repeated twice shows HIT).
func queryCacheDemo(_ js.Value, args []js.Value) interface{} {
	queryBody := `{"filter":"active","limit":5}`
	if len(args) > 0 {
		queryBody = args[0].String()
	}

	cacheKey := simHash(queryBody)

	return map[string]interface{}{
		"ok": true,
		"explanation": "RFC 10008 §2.2: the cache key includes the request URI + Content-Type + SHA-256(body). " +
			"Two QUERY requests to the same URI with the same body MUST be served from cache.",
		"first_request": map[string]interface{}{
			"method":    "QUERY",
			"body_hash": cacheKey,
			"x_cache":   "MISS",
			"latency":   "18ms",
			"stored":    true,
		},
		"second_request": map[string]interface{}{
			"method":    "QUERY",
			"body_hash": cacheKey,
			"x_cache":   "HIT",
			"latency":   "1ms",
			"stored":    false,
		},
		"cache_key":    cacheKey,
		"cache_max_age": 300,
	}
}

// queryVsGet compares QUERY semantics to GET for complex queries.
func queryVsGet(_ js.Value, _ []js.Value) interface{} {
	complexQuery := `{"filter":{"status":"active","dept":["Engineering","Sales"],"hired_after":"2023-01-01"},"sort":[{"field":"name","dir":"asc"}],"page":{"limit":50,"cursor":"eyJpZCI6MTAwfQ=="}}`
	encoded := uriEncode(complexQuery)

	return map[string]interface{}{
		"ok": true,
		"get": map[string]interface{}{
			"method":  "GET",
			"url":     "/users?q=" + encoded,
			"url_len": len("/users?q=") + len(encoded),
			"problem": "URI length exceeds 2000 chars on many proxies/servers; query is logged in access logs; encoding overhead +~33%",
			"safe":    true,
			"cacheable": true,
		},
		"query": map[string]interface{}{
			"method":       "QUERY",
			"url":          "/users",
			"url_len":      7,
			"body":         complexQuery,
			"body_len":     len(complexQuery),
			"content_type": "application/json",
			"problem":      "none — body carries the query, URI stays clean",
			"safe":         true,
			"cacheable":    true,
		},
		"post": map[string]interface{}{
			"method":      "POST",
			"url":         "/users/search",
			"body":        complexQuery,
			"content_type": "application/json",
			"problem":     "not safe (cache treats as state-mutating), not reliably cacheable, requires separate endpoint",
			"safe":        false,
			"cacheable":   false,
		},
	}
}

// queryVsPost explains the semantic difference between QUERY and POST.
func queryVsPost(_ js.Value, _ []js.Value) interface{} {
	return map[string]interface{}{
		"ok": true,
		"comparison": []map[string]interface{}{
			{
				"property":    "Safe (no server state change)",
				"query":       true,
				"post_search": false,
				"note":        "RFC 10008: QUERY is explicitly safe — caches, proxies, and retry logic treat it like GET",
			},
			{
				"property":    "Idempotent",
				"query":       true,
				"post_search": false,
				"note":        "Identical QUERY requests MUST produce identical results (barring external state changes)",
			},
			{
				"property":    "Cacheable by default",
				"query":       true,
				"post_search": false,
				"note":        "RFC 10008 §2.2: QUERY responses are cacheable; POST responses are not by default",
			},
			{
				"property":    "Body carries query",
				"query":       true,
				"post_search": true,
				"note":        "Both support a body — QUERY does it with explicit safe-method semantics",
			},
			{
				"property":    "Dedicated search URL",
				"query":       false,
				"post_search": true,
				"note":        "POST search typically needs /users/search; QUERY uses /users directly",
			},
			{
				"property":    "CORS preflight required",
				"query":       true,
				"post_search": true,
				"note":        "QUERY is not a CORS-safelisted method — preflight required from browsers",
			},
		},
	}
}

// queryErrorDemo shows the RFC 10008 error response matrix.
func queryErrorDemo(_ js.Value, args []js.Value) interface{} {
	scenario := "missing-content-type"
	if len(args) > 0 {
		scenario = args[0].String()
	}

	scenarios := map[string]map[string]interface{}{
		"missing-content-type": {
			"status":     400,
			"status_text": "Bad Request",
			"error":      "Content-Type header is required for QUERY requests",
			"rfc_ref":    "RFC 10008 §2: servers MUST fail if Content-Type is absent",
		},
		"unsupported-media-type": {
			"status":     415,
			"status_text": "Unsupported Media Type",
			"error":      "The query format 'application/xml' is not supported",
			"headers":    map[string]interface{}{"Accept-Query": "application/json, application/x-www-form-urlencoded"},
			"rfc_ref":    "RFC 10008 §3: server MUST send Accept-Query header on 415",
		},
		"invalid-query": {
			"status":     422,
			"status_text": "Unprocessable Content",
			"error":      "Query body is syntactically valid JSON but semantically invalid: unknown field 'unknownFilter'",
			"rfc_ref":    "RFC 10008 §2: syntactically correct but semantically invalid → 422",
		},
		"not-acceptable": {
			"status":     406,
			"status_text": "Not Acceptable",
			"error":      "The server cannot produce 'application/xml' — supported: application/json",
			"rfc_ref":    "RFC 10008 §2: unsupported Accept format → 406",
		},
	}

	result, ok := scenarios[scenario]
	if !ok {
		result = scenarios["missing-content-type"]
	}
	result["ok"] = true
	result["scenario"] = scenario
	return result
}

// queryDBDemo shows how a database driver would handle a QUERY method request.
func queryDBDemo(_ js.Value, args []js.Value) interface{} {
	queryType := "sql"
	if len(args) > 0 {
		queryType = args[0].String()
	}

	now := time.Now().Format(time.RFC3339)
	examples := map[string]map[string]interface{}{
		"sql": {
			"content_type": "application/sql",
			"query_body":   "SELECT id, name, dept FROM users WHERE status = 'active' ORDER BY name LIMIT 50",
			"driver_note":  "PostgreSQL/MySQL QUERY handler: parse SQL, validate against schema, execute read-only transaction, return JSON rows",
			"response_ct":  "application/json",
			"execution_plan": map[string]interface{}{
				"type":       "Index Scan",
				"index":      "idx_users_status",
				"rows":       50,
				"cost":       0.28,
				"read_only":  true,
			},
		},
		"graphql": {
			"content_type": "application/graphql",
			"query_body":   "{ users(filter: {status: \"active\"}, first: 50) { id name dept } }",
			"driver_note":  "GraphQL QUERY handler: parse query document, validate schema, execute resolver, return typed JSON",
			"response_ct":  "application/json",
			"execution_plan": map[string]interface{}{
				"resolver":   "Query.users",
				"fields":     []string{"id", "name", "dept"},
				"complexity": 15,
			},
		},
		"jsonpath": {
			"content_type": "application/json",
			"query_body":   `{"path":"$.users[?(@.status=='active')]","projection":["id","name","dept"],"limit":50}`,
			"driver_note":  "Document store QUERY handler: evaluate JSONPath expression against collection, project fields, return matches",
			"response_ct":  "application/json",
			"execution_plan": map[string]interface{}{
				"collection":  "users",
				"index_used":  "status_idx",
				"scanned":     6,
				"returned":    4,
			},
		},
	}

	ex, ok := examples[queryType]
	if !ok {
		ex = examples["sql"]
	}
	ex["ok"] = true
	ex["query_type"] = queryType
	ex["timestamp"] = now
	ex["safe"] = true
	ex["cacheable"] = true
	return ex
}

// --- helpers ---

func simHash(s string) string {
	h := 0
	for _, c := range s {
		h = h*31 + int(c)
		if h > 1<<28 {
			h ^= h >> 14
		}
	}
	if h < 0 {
		h = -h
	}
	return fmt.Sprintf("%08x%08x%08x%08x", h, h^0xdeadbeef, h^0xcafebabe, h^0x12345678)
}

func uriEncode(s string) string {
	var buf strings.Builder
	for _, c := range s {
		if c == '{' || c == '}' || c == '"' || c == ':' || c == ',' || c == '[' || c == ']' || c == ' ' {
			fmt.Fprintf(&buf, "%%%02X", c)
		} else {
			buf.WriteRune(c)
		}
	}
	return buf.String()
}

func estimateJSONLen(v interface{}) int {
	b, _ := json.Marshal(v)
	return len(b)
}

func errorResult(msg string) map[string]interface{} {
	return map[string]interface{}{"ok": false, "error": msg}
}
