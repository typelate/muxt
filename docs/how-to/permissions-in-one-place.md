# How to keep permissions in one place

A permission rule shows up twice in a web app: the server refuses the action, and the page hides the button for it. When the two copies drift, users see buttons that fail, or the server lets through a request the page never offered. Write the rule once as a Go function, call it where the action is enforced, and call the same function where the template reads it.

`viewer(ctx)` stands for your session lookup. It returns the signed-in `User`, the zero `User` when nobody is signed in.

## Write the rule as a function

```go
// canEdit is the one place the edit rule lives.
func canEdit(u User, doc Document) bool {
	return u.ID != "" && u.ID == doc.OwnerID
}
```

## Enforce it in the receiver method

The method that does the work checks the rule before it does anything, and returns an error when the viewer may not:

```go
type forbiddenError struct{}

func (forbiddenError) Error() string   { return "you cannot edit this document" }
func (forbiddenError) StatusCode() int { return http.StatusForbidden }

var ErrForbidden error = forbiddenError{}

func (s *Server) EditDocument(ctx context.Context, id int) (Document, error) {
	doc, err := s.find(id)
	if err != nil {
		return Document{}, err
	}
	if !canEdit(viewer(ctx), doc) {
		return Document{}, ErrForbidden
	}
	return doc, nil
}
```

A `(T, error)` method responds 500 unless the template sets the code, so the template applies the error's `StatusCode` with the `ErrorStatusCode` method from [map domain errors to status codes](domain-error-status-codes.md):

```gotmpl
{{define "GET /documents/{id}/edit EditDocument(ctx, id)"}}
{{- .ErrorStatusCode -}}
{{- if .Err}}<p class="error">{{.Err}}</p>{{else}}<form>{{.Result.Title}}</form>{{end}}
{{- end}}
```

A request from someone who is not the owner gets a 403 whether or not the page showed them a link.

## Show it through a result field

The page that links to the edit form computes the same rule into its result:

```go
type DocumentPage struct {
	Document Document
	CanEdit  bool
}

func (s *Server) GetDocument(ctx context.Context, id int) (DocumentPage, error) {
	doc, err := s.find(id)
	if err != nil {
		return DocumentPage{}, err
	}
	return DocumentPage{Document: doc, CanEdit: canEdit(viewer(ctx), doc)}, nil
}
```

```gotmpl
{{define "GET /documents/{id} GetDocument(ctx, id)"}}
{{- .ErrorStatusCode -}}
{{- if .Err}}<p class="error">{{.Err}}</p>
{{- else}}<article>
<h1>{{.Result.Document.Title}}</h1>
{{- if .Result.CanEdit}}
<a href="{{.Path.EditDocument .Result.Document.ID}}">Edit</a>
{{- end}}
</article>
{{- end}}
{{- end}}
```

A field suits a page about one record: the method already has the record and the viewer, and a test of the method can assert `CanEdit` without rendering anything.

## Or show it through a TemplateData method

When the template needs the rule for many records, such as each row of a list, a field per record is awkward. Add a method to `TemplateData` ([extend TemplateData](extend-template-data.md)) that reads the viewer from the request and calls the same function:

```go
func (data *TemplateData[R, T]) CanEdit(doc Document) bool {
	return canEdit(viewer(data.Request().Context()), doc)
}
```

```gotmpl
{{define "GET /documents List(ctx)"}}{{template "nav" .}}<ul>
{{- range .Result}}<li>{{.Title}}{{if $.CanEdit .}} <a href="{{$.Path.EditDocument .ID}}">Edit</a>{{end}}</li>{{end}}
</ul>{{end}}
```

Inside `{{range}}` the dot is the document, so the method is called on `$`.

## Enforce route-wide rules in middleware

A rule that depends only on the route and the viewer, such as "only admins reach `/admin/`", fits in middleware. Generate with `--output-routes-func-with-middleware-param` ([generate](../reference/commands/generate.md)) and pass it to the routes function, `TemplateRoutes(mux, srv, authorize)`. `r.Pattern` holds the matched pattern as written in the template name:

```go
func isAdmin(u User) bool { return u.ID == "root" }

func authorize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Pattern, "GET /admin/") && !isAdmin(viewer(r.Context())) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (data *TemplateData[R, T]) IsAdmin() bool {
	return isAdmin(viewer(data.Request().Context()))
}
```

```gotmpl
{{define "GET /admin/users Users(ctx)"}}<ul>{{range .Result}}<li>{{.ID}}</li>{{end}}</ul>{{end}}

{{define "nav"}}<nav><a href="/documents">Documents</a>{{if .IsAdmin}} <a href="{{.Path.Users}}">Users</a>{{end}}</nav>{{end}}
```

The middleware and the template both call `isAdmin`, so the link and the route agree.

## Test both sides of each permission branch

A test that renders the page only as the owner passes whether or not the `{{if .Result.CanEdit}}` is there. `muxt test-template-mutations` ([reference](../reference/commands/test-template-mutations.md)) finds that branch. With the `GET /documents/{id}` template above at the top of `template.gohtml` and a test that only visits as the owner:

```bash
muxt test-template-mutations --seed 1 --template-pattern '^GET /documents/\{id\} '
```

```text
  "GET /documents/{id} GetDocument(ctx, id)" template.gohtml (complexity 3, dot: *main.TemplateData[main.RoutesReceiver, main.DocumentPage])
    MISS 6:1 if-true
```

Line 6 is `{{- if .Result.CanEdit}}`. The `if-true` mutant always shows the link, and no test noticed. Add a test that visits as someone else and asserts there is no edit link, and the mutant becomes a `KILL`:

```go
func TestDocumentOtherViewerSeesNoEdit(t *testing.T) {
	mux := http.NewServeMux()
	TemplateRoutes(mux, newServer(), authorize)
	req := httptest.NewRequest(http.MethodGet, "/documents/1", nil)
	req = req.WithContext(withViewer(req.Context(), User{ID: "bo"}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if body := rec.Body.String(); strings.Contains(body, `/edit"`) {
		t.Errorf("GET /documents/1 as bo = %q, want no edit link", body)
	}
}
```

`withViewer` stands for however your tests sign a user in. The mutation run covers the template only. The check in `EditDocument` is Go, so it needs its own test that a non-owner gets the 403.

## Edge cases

- **`SSETemplateData` does not get your methods.** A fragment rendered by an `sse` callback has `*SSETemplateData` as its dot, not `*TemplateData`, so a method added to `TemplateData` is not there and `muxt check` reports `field or method CanEdit not found on SSETemplateData`. Add the method to both types, each calling the same function:

  ```go
  func (m *SSETemplateData[R, T]) CanEdit(doc Document) bool {
  	return canEdit(viewer(m.Request().Context()), doc)
  }
  ```

  ```gotmpl
  {{define "POST /documents/{id}/watch sse(Watch(ctx, id, sseDocument))"}}{{end}}

  {{define "sseDocument"}}<li id="doc-{{.Result.ID}}">{{.Result.Title}}{{if .CanEdit .Result}} <a href="{{.Path.EditDocument .Result.ID}}">Edit</a>{{end}}</li>{{end}}
  ```

- **The template only displays.** Hiding a button stops no one: a request can be sent without the page. A permission read in the template is for display, never enforcement. Every route that changes or reveals something checks the rule in its receiver method or in middleware.
- **A field is a snapshot.** `CanEdit` is computed when the method runs. A page left open keeps showing the link after the permission is revoked, and the receiver method refuses the click.
- **Middleware sees the pattern, not the record.** Middleware runs before the receiver method loads anything, so a rule that depends on the record, like `canEdit`, belongs in the method.
