package cache

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"
)

// peekBody reads the request body and returns the bytes,
// the Content-Type header, and the computed cache key.
func peekBody(r *http.Request) (body []byte, contentType, cacheKey string) {
	contentType = r.Header.Get("Content-Type")
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
		r.Body.Close()
	}
	cacheKey = Key(r.URL.RequestURI(), contentType, body)
	return
}

// bodyFromBytes wraps a byte slice as an io.ReadCloser.
func bodyFromBytes(b []byte) io.ReadCloser {
	return io.NopCloser(bytes.NewReader(b))
}

// secondsSince returns the number of elapsed seconds since t as a string.
func secondsSince(t time.Time) string {
	secs := int(time.Since(t).Seconds())
	if secs < 0 {
		secs = 0
	}
	return fmt.Sprintf("%d", secs)
}

// responseRecorder buffers an http.ResponseWriter's output.
type responseRecorder struct {
	header http.Header
	body   []byte
	status int
}

func (r *responseRecorder) Header() http.Header { return r.header }

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body = append(r.body, b...)
	return len(b), nil
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
}
