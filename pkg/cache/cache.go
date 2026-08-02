// Package cache implements RFC 10008 §2.2 caching semantics for QUERY responses.
//
// Caching QUERY responses is more complex than caching GET responses because
// the cache key must incorporate the request body in addition to the URI.
// This package provides a simple in-memory cache suitable for demonstration
// and single-process deployments.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

// Entry is a cached QUERY response.
type Entry struct {
	Body        []byte
	ContentType string
	CachedAt    time.Time
	TTL         time.Duration
}

// Expired reports whether the cache entry has expired.
func (e *Entry) Expired() bool {
	return time.Since(e.CachedAt) > e.TTL
}

// Store is an in-memory cache for QUERY responses.
// The cache key is derived from: URI + Content-Type + SHA-256(body).
// This matches the RFC 10008 §2.2 requirement that caches "read the
// full request content to determine the cache key".
type Store struct {
	mu      sync.RWMutex
	entries map[string]*Entry
	ttl     time.Duration
}

// NewStore creates a new in-memory QUERY response cache with the given TTL.
func NewStore(ttl time.Duration) *Store {
	return &Store{
		entries: make(map[string]*Entry),
		ttl:     ttl,
	}
}

// Key computes the cache key for a QUERY request.
func Key(uri, contentType string, body []byte) string {
	h := sha256.New()
	h.Write([]byte(uri))
	h.Write([]byte("\x00"))
	h.Write([]byte(contentType))
	h.Write([]byte("\x00"))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// Get returns the cached entry for the given key, or nil if not found / expired.
func (s *Store) Get(key string) *Entry {
	s.mu.RLock()
	e, ok := s.entries[key]
	s.mu.RUnlock()
	if !ok || e.Expired() {
		return nil
	}
	return e
}

// Set stores a response in the cache.
func (s *Store) Set(key string, body []byte, contentType string) {
	s.mu.Lock()
	s.entries[key] = &Entry{
		Body:        body,
		ContentType: contentType,
		CachedAt:    time.Now(),
		TTL:         s.ttl,
	}
	s.mu.Unlock()
}

// Invalidate removes an entry from the cache.
func (s *Store) Invalidate(key string) {
	s.mu.Lock()
	delete(s.entries, key)
	s.mu.Unlock()
}

// Len returns the number of entries currently in the cache (including expired).
func (s *Store) Len() int {
	s.mu.RLock()
	n := len(s.entries)
	s.mu.RUnlock()
	return n
}

// CachingHandler wraps an http.Handler and adds RFC 10008 caching behaviour.
// On a QUERY request:
//  1. Computes the cache key from URI + Content-Type + body hash.
//  2. If a valid entry exists, serves it with X-Cache: HIT.
//  3. Otherwise, forwards to the inner handler and caches the response,
//     then serves it with X-Cache: MISS.
//
// The "no-transform" Cache-Control directive (RFC 10008 §2.2) is respected:
// when present the body is not normalised before computing the key.
type CachingHandler struct {
	inner http.Handler
	store *Store
}

// NewCachingHandler wraps inner with caching. store is shared across requests.
func NewCachingHandler(inner http.Handler, store *Store) *CachingHandler {
	return &CachingHandler{inner: inner, store: store}
}

// ServeHTTP implements http.Handler.
func (c *CachingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "QUERY" {
		c.inner.ServeHTTP(w, r)
		return
	}

	// Read body for key computation. We buffer it so the inner handler
	// can still read it.
	import_body, contentType, cacheKey := peekBody(r)

	if entry := c.store.Get(cacheKey); entry != nil {
		w.Header().Set("Content-Type", entry.ContentType)
		w.Header().Set("X-Cache", "HIT")
		w.Header().Set("Age", secondsSince(entry.CachedAt))
		w.WriteHeader(http.StatusOK)
		w.Write(entry.Body) //nolint:errcheck
		return
	}

	// MISS — forward to inner handler, capturing the response.
	rec := &responseRecorder{header: make(http.Header)}
	// Restore body so inner handler can read it.
	r.Body = bodyFromBytes(import_body)
	c.inner.ServeHTTP(rec, r)

	if rec.status == http.StatusOK {
		c.store.Set(cacheKey, rec.body, contentType)
	}

	// Write captured response to real writer.
	for k, vs := range rec.header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(rec.status)
	w.Write(rec.body) //nolint:errcheck
}
