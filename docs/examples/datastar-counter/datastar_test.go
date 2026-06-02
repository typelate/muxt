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

func do(t *testing.T, mux *http.ServeMux, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestIndexRendersActions(t *testing.T) {
	rec := do(t, newMux(), httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	// &#39; is the html-escaped single quote the browser decodes back to '.
	for _, want := range []string{
		`data-on:click="@post(&#39;/count/increment&#39;)"`,      // @post
		`data-on:click="@patch(&#39;/count/5&#39;)"`,             // @patch + path param
		`data-on:click="@put(&#39;/count&#39;)"`,                 // @put
		`data-on:click="@delete(&#39;/count&#39;)"`,              // @delete
		`data-init="@get(&#39;/clock&#39;)"`,                     // String() in non-event ctx
		`@post(&#39;/greet&#39;, {contentType: &#39;form&#39;})`, // action option
		`href="/"`, // .Path helper
	} {
		if !strings.Contains(body, want) {
			t.Errorf("index missing %q", want)
		}
	}
}

func TestSignalVerbs(t *testing.T) {
	mux := newMux()
	cases := []struct {
		method, path, want string
	}{
		{http.MethodPost, "/count/increment", `{"count":1}`},
		{http.MethodPost, "/count/increment", `{"count":2}`},
		{http.MethodPatch, "/count/5", `{"count":7}`},
		{http.MethodPost, "/count/decrement", `{"count":6}`},
		{http.MethodPut, "/count", `{"count":0}`},
		{http.MethodPost, "/count/increment", `{"count":1}`},
		{http.MethodDelete, "/count", `{"count":0}`},
	}
	for _, tc := range cases {
		rec := do(t, mux, httptest.NewRequest(tc.method, tc.path, nil))
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s %s content-type = %q, want application/json", tc.method, tc.path, ct)
		}
		if got := strings.TrimSpace(rec.Body.String()); got != tc.want {
			t.Errorf("%s %s body = %q, want %q", tc.method, tc.path, got, tc.want)
		}
	}
}

func TestClockStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/clock", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { newMux().ServeHTTP(rec, req); close(done) }()
	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q", ct)
	}
	for _, want := range []string{
		"event: datastar-patch-elements\n",
		"data: selector #clock\n",
		"data: mode inner\n",
		"data: useViewTransition true\n",
		"data: elements ",
	} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("clock stream missing %q\n%s", want, rec.Body.String())
		}
	}
}

func TestFeedAppendAndInlineSignals(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/feed", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { newMux().ServeHTTP(rec, req); close(done) }()
	time.Sleep(700 * time.Millisecond) // ~2 items at 400ms apart
	cancel()
	<-done

	body := rec.Body.String()
	for _, want := range []string{
		"data: mode append\n",
		"data: selector #feed-list\n",
		"data: elements <li>item 1",
		"event: datastar-patch-signals\n",
		`data: signals {"status":"appended 1"}`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("feed stream missing %q\n%s", want, body)
		}
	}
}

func TestGreetForm(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/greet", strings.NewReader("name=World"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := do(t, newMux(), req)
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{"data: selector #greeting\n", "data: elements Hello, World!"} {
		if !strings.Contains(body, want) {
			t.Errorf("greet missing %q\n%s", want, body)
		}
	}
}

func TestConfigScriptEmbedsJSON(t *testing.T) {
	rec := do(t, newMux(), httptest.NewRequest(http.MethodGet, "/config.js", nil))
	if ct := rec.Header().Get("Content-Type"); ct != "text/javascript" {
		t.Fatalf("content-type = %q", ct)
	}
	if got := rec.Body.String(); !strings.Contains(got, `window.__APP_CONFIG = {"version":"1.0","count":0};`) {
		t.Errorf("config body = %q", got)
	}
}

func TestFragmentHTML(t *testing.T) {
	rec := do(t, newMux(), httptest.NewRequest(http.MethodGet, "/fragment", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<div id="fragment-target">loaded GET at `) {
		t.Errorf("fragment body = %q", body)
	}
}
