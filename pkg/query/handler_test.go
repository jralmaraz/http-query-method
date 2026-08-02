package query_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jralmaraz/http-query-method/pkg/query"
)

func echoHandler(req *query.Request) ([]byte, string, error) {
	return req.Body, req.ContentType, nil
}

func TestMethod_Constant(t *testing.T) {
	if query.Method != "QUERY" {
		t.Fatalf("expected QUERY, got %s", query.Method)
	}
}

func TestParseRequest_HappyPath(t *testing.T) {
	body := `{"filter":{"status":"active"}}`
	r := httptest.NewRequest(query.Method, "/resources", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	req, err := query.ParseRequest(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.ContentType != "application/json" {
		t.Errorf("expected application/json, got %q", req.ContentType)
	}
	if string(req.Body) != body {
		t.Errorf("body mismatch: %q", req.Body)
	}
}

func TestParseRequest_MissingContentType(t *testing.T) {
	r := httptest.NewRequest(query.Method, "/resources", strings.NewReader("{}"))
	// No Content-Type set

	_, err := query.ParseRequest(r)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	qe, ok := err.(*query.QueryError)
	if !ok {
		t.Fatalf("expected *QueryError, got %T", err)
	}
	if qe.Status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", qe.Status)
	}
}

func TestParseRequest_WrongMethod(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/resources", strings.NewReader("{}"))
	r.Header.Set("Content-Type", "application/json")

	_, err := query.ParseRequest(r)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	qe, ok := err.(*query.QueryError)
	if !ok {
		t.Fatalf("expected *QueryError, got %T", err)
	}
	if qe.Status != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", qe.Status)
	}
}

func TestHandler_HappyPath(t *testing.T) {
	h := query.NewHandler(echoHandler, "application/json")
	r := httptest.NewRequest(query.Method, "/q", strings.NewReader(`{"q":"test"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
}

func TestHandler_MissingContentType_Returns400(t *testing.T) {
	h := query.NewHandler(echoHandler, "application/json")
	r := httptest.NewRequest(query.Method, "/q", strings.NewReader("{}"))
	// No Content-Type
	w := httptest.NewRecorder()

	h.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandler_UnsupportedMediaType_Returns415_WithAcceptQuery(t *testing.T) {
	h := query.NewHandler(echoHandler, "application/json")
	r := httptest.NewRequest(query.Method, "/q", strings.NewReader("<query/>"))
	r.Header.Set("Content-Type", "application/xml")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, r)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415, got %d", w.Code)
	}
	// RFC 10008 §3: server MUST send Accept-Query on 415
	if aq := w.Header().Get(query.HeaderAcceptQuery); aq == "" {
		t.Error("expected Accept-Query header on 415 response")
	}
}

func TestHandler_NonQUERYMethod_Returns405(t *testing.T) {
	h := query.NewHandler(echoHandler, "application/json")
	r := httptest.NewRequest(http.MethodPost, "/q", strings.NewReader("{}"))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
	if allow := w.Header().Get("Allow"); !strings.Contains(allow, query.Method) {
		t.Errorf("Allow header should include QUERY, got %q", allow)
	}
}

func TestJSONQueryHandler_HappyPath(t *testing.T) {
	h := query.JSONQueryHandler(func(q map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{"matched": 3, "filter": q}, nil
	})

	r := httptest.NewRequest(query.Method, "/search", strings.NewReader(`{"status":"active"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body, _ := io.ReadAll(w.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if result["matched"] != float64(3) {
		t.Errorf("unexpected matched count: %v", result["matched"])
	}
}

func TestJSONQueryHandler_InvalidBody_Returns422(t *testing.T) {
	h := query.JSONQueryHandler(func(q map[string]interface{}) (interface{}, error) {
		return q, nil
	})

	r := httptest.NewRequest(query.Method, "/search", strings.NewReader(`not json`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, r)

	// RFC 10008: syntactically invalid query body → 422 Unprocessable Content
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", w.Code)
	}
}

// TestEndToEnd wires up an httptest.Server and uses a real http.Client
// with a custom transport that adds the QUERY method.
func TestEndToEnd(t *testing.T) {
	h := query.JSONQueryHandler(func(q map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{"query_received": q, "results": []string{"a", "b"}}, nil
	})

	srv := httptest.NewServer(h)
	defer srv.Close()

	body := strings.NewReader(`{"filter":"active","limit":10}`)
	req, _ := http.NewRequest(query.Method, srv.URL+"/search", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck
	if _, ok := result["results"]; !ok {
		t.Error("expected 'results' key in response")
	}
}
