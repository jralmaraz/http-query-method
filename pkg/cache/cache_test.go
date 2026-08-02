package cache_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jralmaraz/http-query-method/pkg/cache"
)

func TestKey_DifferentBodies_DifferentKeys(t *testing.T) {
	k1 := cache.Key("/q", "application/json", []byte(`{"a":1}`))
	k2 := cache.Key("/q", "application/json", []byte(`{"a":2}`))
	if k1 == k2 {
		t.Error("different bodies must produce different keys")
	}
}

func TestKey_DifferentURIs_DifferentKeys(t *testing.T) {
	k1 := cache.Key("/q", "application/json", []byte(`{}`))
	k2 := cache.Key("/other", "application/json", []byte(`{}`))
	if k1 == k2 {
		t.Error("different URIs must produce different keys")
	}
}

func TestKey_DifferentContentTypes_DifferentKeys(t *testing.T) {
	k1 := cache.Key("/q", "application/json", []byte(`{}`))
	k2 := cache.Key("/q", "application/xml", []byte(`{}`))
	if k1 == k2 {
		t.Error("different content types must produce different keys")
	}
}

func TestKey_Deterministic(t *testing.T) {
	k1 := cache.Key("/q", "application/json", []byte(`{"filter":"active"}`))
	k2 := cache.Key("/q", "application/json", []byte(`{"filter":"active"}`))
	if k1 != k2 {
		t.Error("same input must always produce the same key")
	}
}

func TestStore_SetAndGet(t *testing.T) {
	s := cache.NewStore(time.Minute)
	key := cache.Key("/q", "application/json", []byte(`{}`))
	s.Set(key, []byte(`{"results":[]}`), "application/json")

	entry := s.Get(key)
	if entry == nil {
		t.Fatal("expected cache hit, got nil")
	}
	if string(entry.Body) != `{"results":[]}` {
		t.Errorf("unexpected body: %s", entry.Body)
	}
}

func TestStore_Miss_ReturnsNil(t *testing.T) {
	s := cache.NewStore(time.Minute)
	entry := s.Get("nonexistent-key")
	if entry != nil {
		t.Error("expected nil on miss, got entry")
	}
}

func TestStore_Expiry(t *testing.T) {
	s := cache.NewStore(time.Millisecond)
	key := cache.Key("/q", "application/json", []byte(`{}`))
	s.Set(key, []byte(`{}`), "application/json")

	time.Sleep(10 * time.Millisecond)
	if entry := s.Get(key); entry != nil {
		t.Error("expected expired entry to return nil")
	}
}

func TestStore_Invalidate(t *testing.T) {
	s := cache.NewStore(time.Minute)
	key := cache.Key("/q", "application/json", []byte(`{}`))
	s.Set(key, []byte(`{}`), "application/json")
	s.Invalidate(key)
	if entry := s.Get(key); entry != nil {
		t.Error("expected nil after invalidation")
	}
}

func TestCachingHandler_MissThenHit(t *testing.T) {
	callCount := 0
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"results":["a","b"]}`)) //nolint:errcheck
	})

	store := cache.NewStore(time.Minute)
	h := cache.NewCachingHandler(inner, store)

	// First request — MISS
	r1 := httptest.NewRequest("QUERY", "/q", strings.NewReader(`{"filter":"x"}`))
	r1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	h.ServeHTTP(w1, r1)

	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w1.Code)
	}
	if w1.Header().Get("X-Cache") != "MISS" {
		t.Errorf("expected X-Cache: MISS on first request, got %q", w1.Header().Get("X-Cache"))
	}
	if callCount != 1 {
		t.Errorf("expected inner called once, got %d", callCount)
	}

	// Second request with same body — HIT
	r2 := httptest.NewRequest("QUERY", "/q", strings.NewReader(`{"filter":"x"}`))
	r2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, r2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}
	if w2.Header().Get("X-Cache") != "HIT" {
		t.Errorf("expected X-Cache: HIT on second request, got %q", w2.Header().Get("X-Cache"))
	}
	if callCount != 1 {
		t.Errorf("inner should not have been called again; call count = %d", callCount)
	}
}

func TestCachingHandler_DifferentBodies_BothMiss(t *testing.T) {
	callCount := 0
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`)) //nolint:errcheck
	})

	store := cache.NewStore(time.Minute)
	h := cache.NewCachingHandler(inner, store)

	r1 := httptest.NewRequest("QUERY", "/q", strings.NewReader(`{"a":1}`))
	r1.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(httptest.NewRecorder(), r1)

	r2 := httptest.NewRequest("QUERY", "/q", strings.NewReader(`{"a":2}`))
	r2.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(httptest.NewRecorder(), r2)

	if callCount != 2 {
		t.Errorf("different bodies must be separate cache entries; got %d calls", callCount)
	}
}

func TestCachingHandler_NonQUERY_PassThrough(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	store := cache.NewStore(time.Minute)
	h := cache.NewCachingHandler(inner, store)

	r := httptest.NewRequest(http.MethodGet, "/q", nil)
	h.ServeHTTP(httptest.NewRecorder(), r)

	if !called {
		t.Error("non-QUERY requests must be passed through without caching")
	}
}
