// Package query implements RFC 10008 — The HTTP QUERY Method.
//
// QUERY is a safe, idempotent HTTP method that accepts a request body
// (unlike GET) and whose response is cacheable (unlike POST). It is
// designed for server-side query processing where the query expression
// is too large or complex to encode safely in a URI.
//
// Reference: https://www.rfc-editor.org/rfc/rfc10008
package query

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

const (
	// Method is the HTTP method name defined by RFC 10008.
	Method = "QUERY"

	// HeaderAcceptQuery is the response header that advertises
	// which query media types a resource accepts (RFC 10008 §3).
	HeaderAcceptQuery = "Accept-Query"
)

// ErrMissingContentType is returned when a QUERY request arrives
// without a Content-Type header. RFC 10008 §2 requires servers to
// reject such requests with 400 Bad Request.
var ErrMissingContentType = &QueryError{
	Status:  http.StatusBadRequest,
	Message: "QUERY requests MUST include a Content-Type header",
}

// QueryError represents a protocol-level error with an HTTP status code.
type QueryError struct {
	Status  int
	Message string
}

func (e *QueryError) Error() string { return e.Message }

// Request is a parsed QUERY request.
type Request struct {
	// ContentType is the media type of the query body (e.g. "application/json").
	ContentType string

	// Body is the raw query expression bytes.
	Body []byte

	// Accept is the preferred response media type from the Accept header.
	Accept string

	// Raw is the original http.Request.
	Raw *http.Request
}

// ParseRequest reads and validates a QUERY request from r.
// Returns ErrMissingContentType (→ 400) if Content-Type is absent.
func ParseRequest(r *http.Request) (*Request, error) {
	if r.Method != Method {
		return nil, &QueryError{
			Status:  http.StatusMethodNotAllowed,
			Message: "method must be QUERY",
		}
	}

	ct := r.Header.Get("Content-Type")
	if ct == "" {
		return nil, ErrMissingContentType
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10 MB limit
	if err != nil {
		return nil, &QueryError{Status: http.StatusBadRequest, Message: "failed to read request body: " + err.Error()}
	}

	return &Request{
		ContentType: ct,
		Body:        body,
		Accept:      r.Header.Get("Accept"),
		Raw:         r,
	}, nil
}

// HandlerFunc is the signature for RFC 10008 query handlers.
// It receives the parsed QUERY request and returns the result bytes,
// a response Content-Type, and any error.
type HandlerFunc func(req *Request) (result []byte, contentType string, err error)

// Handler wraps a HandlerFunc and implements http.Handler.
// It enforces the RFC 10008 protocol requirements:
//   - Validates Content-Type presence
//   - Returns appropriate 4xx errors per the spec
//   - Advertises supported types via Accept-Query on 415 responses
type Handler struct {
	fn             HandlerFunc
	supportedTypes []string // media types accepted by this handler
}

// NewHandler creates a new RFC 10008 QUERY handler.
// supportedTypes lists the media types this handler accepts
// (used to populate Accept-Query on 415 responses).
func NewHandler(fn HandlerFunc, supportedTypes ...string) *Handler {
	return &Handler{fn: fn, supportedTypes: supportedTypes}
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != Method {
		w.Header().Set("Allow", Method+", GET, HEAD")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	req, err := ParseRequest(r)
	if err != nil {
		qe, ok := err.(*QueryError)
		if !ok {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if qe.Status == http.StatusUnsupportedMediaType && len(h.supportedTypes) > 0 {
			w.Header().Set(HeaderAcceptQuery, strings.Join(h.supportedTypes, ", "))
		}
		http.Error(w, qe.Message, qe.Status)
		return
	}

	// Validate that the Content-Type is one the handler accepts.
	if len(h.supportedTypes) > 0 && !h.accepts(req.ContentType) {
		w.Header().Set(HeaderAcceptQuery, strings.Join(h.supportedTypes, ", "))
		http.Error(w, "Unsupported Media Type: "+req.ContentType, http.StatusUnsupportedMediaType)
		return
	}

	result, ct, err := h.fn(req)
	if err != nil {
		if qe, ok := err.(*QueryError); ok {
			http.Error(w, qe.Message, qe.Status)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", ct)
	w.WriteHeader(http.StatusOK)
	w.Write(result) //nolint:errcheck
}

func (h *Handler) accepts(contentType string) bool {
	// Strip parameters (e.g. "application/json; charset=utf-8" → "application/json")
	base := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	for _, t := range h.supportedTypes {
		if strings.ToLower(strings.TrimSpace(t)) == base {
			return true
		}
	}
	return false
}

// JSONQueryHandler is a convenience handler for JSON query bodies.
// It decodes the request body as JSON, calls fn, and encodes the result.
func JSONQueryHandler(fn func(query map[string]interface{}) (interface{}, error)) *Handler {
	return NewHandler(func(req *Request) ([]byte, string, error) {
		var q map[string]interface{}
		if err := json.Unmarshal(req.Body, &q); err != nil {
			return nil, "", &QueryError{
				Status:  http.StatusUnprocessableEntity,
				Message: "query body is not valid JSON: " + err.Error(),
			}
		}
		result, err := fn(q)
		if err != nil {
			return nil, "", err
		}
		out, err := json.Marshal(result)
		if err != nil {
			return nil, "", &QueryError{Status: http.StatusInternalServerError, Message: err.Error()}
		}
		return out, "application/json", nil
	}, "application/json")
}
