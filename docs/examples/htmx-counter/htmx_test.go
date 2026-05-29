package main

import (
	"bytes"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTemplateData_HTMX(t *testing.T) {
	requireEqual := func(t *testing.T, expected, actual string) {
		t.Helper()
		if expected != actual {
			t.Errorf("\nExpected: %s\nGot: %s", expected, actual)
		}
	}
	checkHeader := func(t *testing.T, ts *template.Template) http.Header {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		data := newTemplateData(any(nil), rec, req, struct{}{}, true)
		if err := ts.ExecuteTemplate(io.Discard, "", data); err != nil {
			t.Fatal(err)
		}
		return rec.Header()
	}

	t.Run("HXLocation", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{.HXLocation "/abc"}}`))
		h := checkHeader(t, ts)
		requireEqual(t, "/abc", h.Get("HX-Location"))
	})

	t.Run("HXPushURL", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{.HXPushURL "/abc"}}`))
		h := checkHeader(t, ts)
		requireEqual(t, "/abc", h.Get("HX-Push-Url"))
	})

	t.Run("HXRedirect", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{.HXRedirect "/"}}`))
		h := checkHeader(t, ts)
		requireEqual(t, "/", h.Get("HX-Redirect"))
	})

	t.Run("HXRefresh", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{.HXRefresh}}`))
		h := checkHeader(t, ts)
		requireEqual(t, "true", h.Get("HX-Refresh"))
	})

	t.Run("HXReplaceURL", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{.HXReplaceURL "/abc"}}`))
		h := checkHeader(t, ts)
		requireEqual(t, "/abc", h.Get("HX-Replace-Url"))
	})

	t.Run("HXReswap", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{and (.HXReswap "outerhtml") (.HXReswap "innerhtml")}}`))
		h := checkHeader(t, ts)
		requireEqual(t, "innerhtml", h.Get("HX-Reswap"))
	})

	t.Run("HXRetarget", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{.HXRetarget "main"}}`))
		h := checkHeader(t, ts)
		requireEqual(t, "main", h.Get("HX-Retarget"))
	})

	t.Run("HXReselect", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{.HXReselect "main"}}`))
		h := checkHeader(t, ts)
		requireEqual(t, "main", h.Get("HX-Reselect"))
	})

	t.Run("HXTrigger", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{.HXTrigger "MsgEvent"}}`))
		h := checkHeader(t, ts)
		requireEqual(t, "MsgEvent", h.Get("HX-Trigger"))
	})

	t.Run("HXTriggerAfterSettle", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{.HXTriggerAfterSettle "SomeEvent"}}`))
		h := checkHeader(t, ts)
		requireEqual(t, "SomeEvent", h.Get("HX-Trigger-After-Settle"))
	})

	t.Run("HXTriggerAfterSwap", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{.HXTriggerAfterSwap "SomeEvent"}}`))
		h := checkHeader(t, ts)
		requireEqual(t, "SomeEvent", h.Get("HX-Trigger-After-Swap"))
	})

	checkBody := func(t *testing.T, ts *template.Template, req *http.Request) string {
		t.Helper()
		rec := httptest.NewRecorder()
		data := newTemplateData(any(nil), rec, req, struct{}{}, true)
		var buf bytes.Buffer
		if err := ts.ExecuteTemplate(&buf, "", data); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}

	t.Run("HXBoosted", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{if .HXBoosted}}BOOSTED{{end}}`))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("HX-Boosted", "true")
		body := checkBody(t, ts, req)
		requireEqual(t, body, "BOOSTED")
	})

	t.Run("HXCurrentURL", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{.HXCurrentURL}}`))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("HX-Current-url", "http://example.com/?test=HXCurrentURL")
		body := checkBody(t, ts, req)
		requireEqual(t, body, "http://example.com/?test=HXCurrentURL")
	})

	t.Run("HXHistoryRestoreRequest not set", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{.HXHistoryRestoreRequest}}`))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		body := checkBody(t, ts, req)
		requireEqual(t, body, "false")
	})

	t.Run("HXHistoryRestoreRequest true", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{.HXHistoryRestoreRequest}}`))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("HX-History-Restore-Request", "true")
		body := checkBody(t, ts, req)
		requireEqual(t, body, "true")
	})

	t.Run("HXPrompt", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{.HXPrompt}}`))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("HX-Prompt", "Duh!")
		body := checkBody(t, ts, req)
		requireEqual(t, body, "Duh!")
	})

	t.Run("HXRequest not set", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{.HXRequest}}`))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		body := checkBody(t, ts, req)
		requireEqual(t, body, "false")
	})

	t.Run("HXRequest true", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{.HXRequest}}`))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("HX-Request", "true")
		body := checkBody(t, ts, req)
		requireEqual(t, body, "true")
	})

	t.Run("HXTargetElementID", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{.HXTargetElementID}}`))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("HX-Target", "some-target")
		body := checkBody(t, ts, req)
		requireEqual(t, body, "some-target")
	})

	t.Run("HXTriggerName", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{.HXTriggerName}}`))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("HX-Trigger-Name", "some-name")
		body := checkBody(t, ts, req)
		requireEqual(t, body, "some-name")
	})

	t.Run("HXTriggerElementID", func(t *testing.T) {
		ts := template.Must(template.New("").Parse(`{{.HXTriggerElementID}}`))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("HX-Trigger", "some-id")
		body := checkBody(t, ts, req)
		requireEqual(t, body, "some-id")
	})
}
