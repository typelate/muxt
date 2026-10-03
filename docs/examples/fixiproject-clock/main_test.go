package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestInZone(t *testing.T) {
	mux := http.NewServeMux()
	paths := TemplateRoutes(mux, clock{location: time.UTC})

	t.Run("zone names keep their slashes", func(t *testing.T) {
		if got, want := paths.InZone("America/New_York"), "/zone/America/New_York"; got != want {
			t.Errorf("InZone(%q) = %q, want %q", "America/New_York", got, want)
		}
	})

	t.Run("url metacharacters are escaped per segment", func(t *testing.T) {
		if got, want := paths.InZone("50%/#1"), "/zone/50%25/%231"; got != want {
			t.Errorf("InZone(%q) = %q, want %q", "50%/#1", got, want)
		}
	})

	t.Run("a known zone renders a timestamp", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, paths.InZone("America/New_York"), nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want %d", paths.InZone("America/New_York"), rec.Code, http.StatusOK)
		}
		if body := rec.Body.String(); !strings.Contains(body, "<time>") {
			t.Errorf("body %q does not contain a time element", body)
		}
	})

	t.Run("an unknown zone renders the error branch", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, paths.InZone("Nowhere/Special"), nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if body := rec.Body.String(); !strings.Contains(body, "unknown time zone") {
			t.Errorf("body %q does not contain the error message", body)
		}
	})
}
