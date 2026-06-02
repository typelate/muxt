package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	TemplateRoutes(mux, new(Server))
	return mux
}

func TestIndexRendersActions(t *testing.T) {
	rec := httptest.NewRecorder()
	newMux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	// &#39; is the html-escaped single quote the browser decodes back to '.
	for _, want := range []string{
		`data-on:click="@post(&#39;/increment&#39;)"`,
		`data-init="@get(&#39;/clock&#39;)"`,
		`data-on:click="@get(&#39;/hello.js&#39;)"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("index missing %q\nbody: %s", want, body)
		}
	}
}

func TestIncrementEmitsSignals(t *testing.T) {
	mux := newMux()
	for i, want := range []string{`{"count":1}`, `{"count":2}`} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/increment", nil))
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Fatalf("call %d content-type = %q, want application/json", i, ct)
		}
		if got := strings.TrimSpace(rec.Body.String()); got != want {
			t.Errorf("call %d body = %q, want %q", i, got, want)
		}
	}
}

func TestHelloRespondsJavaScript(t *testing.T) {
	rec := httptest.NewRecorder()
	newMux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/hello.js", nil))
	if ct := rec.Header().Get("Content-Type"); ct != "text/javascript" {
		t.Fatalf("content-type = %q, want text/javascript", ct)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `console.log("datastar script executed");` {
		t.Errorf("body = %q", got)
	}
}

func TestClockStreamsPatchElements(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/clock", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { newMux().ServeHTTP(rec, req); close(done) }()

	// The first frame is written synchronously before the one-second tick;
	// cancel once it has been flushed.
	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"event: datastar-patch-elements\n",
		"data: selector #clock\n",
		"data: mode inner\n",
		"data: elements ",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("clock stream missing %q\nbody: %s", want, body)
		}
	}
}
