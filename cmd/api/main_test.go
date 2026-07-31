package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
)

func TestRootRoute(t *testing.T) {
	r := newRouter(zerolog.Nop())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	want := "NoxOJ API — Sprint 1 skeleton is alive"
	if got := rec.Body.String(); got != want {
		t.Fatalf("expected body %q, got %q", want, got)
	}
}

func TestHealthzRoute(t *testing.T) {
	r := newRouter(zerolog.Nop())

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestReadyzRoute(t *testing.T) {
	r := newRouter(zerolog.Nop())

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}
