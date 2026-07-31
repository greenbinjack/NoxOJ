package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRootRoute(t *testing.T) {
	r := newRouter()

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
