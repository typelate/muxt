# How to structure a muxt project for testing

Keep your receiver type, templates, and generated routes in an importable
library package, and keep `package main` a thin wrapper. Go cannot import
`package main`, so a receiver defined there cannot be faked (counterfeiter,
moq) or tested from an external test package.

## Layout

```
myapp/
├── go.mod
├── main.go                 // package main: wiring only
└── hypertext/
    ├── templates.go        // package hypertext: templates var + receiver
    ├── index.gohtml
    ├── template_routes.go  // generated
    └── routes_test.go
```

`hypertext/templates.go`:

```go
package hypertext

import (
	"embed"
	"html/template"
)

//go:embed *.gohtml
var templatesFS embed.FS

var templates = template.Must(template.ParseFS(templatesFS, "*.gohtml"))

//go:generate muxt generate --use-receiver-type=Server

type Server struct{ db *database.Queries }
```

`main.go` only builds the dependencies and registers routes:

```go
mux := http.NewServeMux()
hypertext.TemplateRoutes(mux, hypertext.NewServer(queries))
log.Fatal(http.ListenAndServe(":8080", mux))
```

## Wire sqlc storage

Point sqlc at its own package (for example `internal/database`) and let the
receiver methods call the generated queries. A sqlc row struct works directly
as a method result, and a sqlc params struct works directly as a `form`
argument — its fields parse from the submitted form by name (add `name:"..."`
tags when input names differ from field names).

```go
func (s Server) CreateIncident(ctx context.Context, f database.CreateIncidentParams) (database.Incident, error) {
	return s.db.CreateIncident(ctx, f)
}
```

## Test through the generated routes

Register the routes on a fresh mux per test and drive it with `httptest`; an
in-memory sqlite database (`modernc.org/sqlite`, DSN `:memory:`) keeps tests
hermetic.

```go
func newTestMux(t *testing.T) *http.ServeMux {
	t.Helper()
	db := openMemoryDB(t) // sql.Open("sqlite", ":memory:") + schema
	mux := http.NewServeMux()
	TemplateRoutes(mux, NewServer(database.New(db)))
	return mux
}

func TestCreateIncident(t *testing.T) {
	mux := newTestMux(t)
	form := url.Values{"title": {"disk full"}, "severity": {"2"}}
	req := httptest.NewRequest(http.MethodPost, "/incidents", strings.NewReader(form.Encode()))
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
}
```

To test handler behavior without a database, generate a fake of the
`RoutesReceiver` interface (this is why the receiver lives in a library
package) and register the routes with the fake.

The [simple example](../examples/simple) shows this layout end to end.
