# How to handle expired sessions in htmx requests

When a session expires, the next htmx request must not get the login page back. A redirect to `/login` is followed inside the XMLHttpRequest, so htmx receives a 200 login page and swaps it into whatever small element asked, leaving a page inside the page. Detect htmx requests in middleware and answer them in a way htmx acts on. Keep background polling from extending the session, and show a lock dialog when it ends.

`sessions` stands for your session package: `Lookup` finds the request's session, `Touch` extends its expiry, and `ExpiresAt` is when it ends.

## Wrap every route with middleware

Generate with `--output-routes-func-with-middleware-param` ([generate](../reference/commands/generate.md)) and pass the middleware to the routes function:

```go
TemplateRoutes(mux, srv, requireSession(store))
```

The middleware runs after the mux has matched, so `r.Pattern` holds the pattern of the matched route, as written in the template name. It wraps the static assets handler too.

```go
func requireSession(store *sessions.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Pattern {
			case "GET /login", "POST /login", "GET /static/":
				next.ServeHTTP(w, r)
				return
			case "GET /session/status":
				// The status route reads the session itself and never extends it.
				next.ServeHTTP(w, r)
				return
			}
			session, ok := store.Lookup(r)
			if !ok {
				refuseExpired(w, r)
				return
			}
			if r.Header.Get("X-Background") != "true" {
				store.Touch(w, session)
			}
			ctx, cancel := context.WithDeadline(r.Context(), session.ExpiresAt)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
```

The login routes and static assets are exempt, or the login page could not load its own stylesheet. `GET /static/` is the pattern `--output-static-assets-path=/static` registers ([static assets](../reference/commands/generate.md#static-assets)). Matching on `r.Pattern` names each route the way its template does.

## Answer an htmx request with HX-Redirect

```go
func refuseExpired(w http.ResponseWriter, r *http.Request) {
	login := "/login?next=" + url.QueryEscape(nextPath(r))
	switch {
	case r.Header.Get("HX-History-Restore-Request") == "true":
		w.WriteHeader(http.StatusUnauthorized)
	case r.Header.Get("HX-Request") == "true":
		w.Header().Set("HX-Redirect", login)
		w.WriteHeader(http.StatusUnauthorized)
	default:
		http.Redirect(w, r, login, http.StatusSeeOther)
	}
}

// nextPath is the page the user was on: HX-Current-URL for an htmx request,
// the request itself otherwise. Only the path and query are kept, so the
// login handler never redirects to another host.
func nextPath(r *http.Request) string {
	if current, err := url.Parse(r.Header.Get("HX-Current-URL")); err == nil && current.Path != "" {
		if p := current.RequestURI(); strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "//") {
			return p
		}
	}
	return r.URL.RequestURI()
}
```

htmx 2 reads `HX-Redirect` before it looks at the status code, sets `location.href`, and swaps nothing, so the 401 still navigates the whole tab. Any client that is not htmx sees a 401 instead of a 200. `next` comes from `HX-Current-URL`, the page htmx was on, not from the fragment URL it requested. Check `next` again in the login handler before redirecting to it: it is input from the client.

`HX-Refresh: true` in place of `HX-Redirect` reloads the page instead. The reload is a plain request that the `default` case answers with a 303 to the login page, with `next` set to the page URL. That costs a round trip and needs no `HX-Current-URL`.

htmx reads no response headers when it restores history: when the user goes back to a pushed URL that htmx has no snapshot of, it requests the URL with `HX-History-Restore-Request: true` and only swaps responses below 400. The 401 there makes htmx fire `htmx:historyCacheMissLoadError`; the script below reloads the page on it.

## Or show a lock dialog on 401

To keep the page in place and show your own lock screen, drop the `HX-Redirect` line from `refuseExpired`, answer a bare 401, and listen for `htmx:responseError`, which htmx fires for any response of 400 or above:

```html
<dialog id="session-lock">
	<p>Your session has ended.</p>
	<a href="/login">Sign in again</a>
</dialog>
<script>
	htmx.on("htmx:responseError", function (event) {
		if (event.detail.xhr.status === 401) {
			document.getElementById("session-lock").showModal();
		}
	});
	htmx.on("htmx:historyCacheMissLoadError", function () {
		location.reload();
	});
</script>
```

Put both in the layout so every page has them. htmx does not swap a 401 body by default, but an element with `hx-target-401` or `hx-target-4*` from the response-targets extension does, so keep the 401 body empty.

## Keep polling from extending the session

A poll that counts as activity keeps the session alive for as long as the tab is open. Mark background requests and skip `Touch` for them, as the middleware above does. A polling element that is not user activity sends the header with `hx-headers`:

```html
<section hx-get="/notifications" hx-trigger="every 30s" hx-headers='{"X-Background": "true"}'></section>
```

A route that only background requests use can be matched on `r.Pattern` instead, as the middleware does for `GET /session/status`.

## Poll for the session's end

A heartbeat route lets the page show the lock dialog before the user clicks anything. `SessionStatus` takes the request ([call parameters](../reference/call-parameters.md)) and reports whether the session is gone:

```go
type SessionState struct{ Expired bool }

func (s *Server) SessionStatus(r *http.Request) SessionState {
	_, ok := s.sessions.Lookup(r)
	return SessionState{Expired: !ok}
}
```

```gotmpl
{{define "GET /session/status SessionStatus(request)"}}
{{- if .Result.Expired -}}
{{.StatusCode 286}}<dialog id="session-lock" hx-swap-oob="true" hx-on::load="this.showModal()">
	<p>Your session has ended.</p>
	<a href="/login">Sign in again</a>
</dialog>
{{- end -}}
{{end}}
```

```gotmpl
<div hx-get="{{.Path.SessionStatus}}" hx-trigger="every 60s" hx-swap="none"></div>
```

While the session lasts the template renders nothing, so the route answers 204 ([status codes](../reference/call-results.md#status-code-control)) and htmx swaps nothing. Once it has ended the route answers 286: htmx swaps the response, which with `hx-swap="none"` means only the out-of-band dialog replaces the `#session-lock` in the layout, and 286 stops the polling. `hx-on::load` runs once htmx has processed the new dialog and opens it as a modal.

## Datastar

Datastar has no counterpart to `HX-Redirect`, so a header cannot move the tab to the login page. Datastar actions send `Datastar-Request: true`; for those, answer 401, or answer 200 with a `text/html` body holding the `<dialog id="session-lock">`, which Datastar patches into the page by id.

A Server-Sent Events route can stay open long after the session ends. The `context.WithDeadline` in the middleware cancels the request's context at `ExpiresAt`. The generated `sse` callbacks check the request context before they write, and a receiver method that takes `ctx` should return when `ctx.Done()` closes, so the stream ends with the session ([Datastar live view](datastar-live-view.md)).
