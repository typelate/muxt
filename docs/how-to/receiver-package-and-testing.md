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

## Mock service interfaces, not RoutesReceiver

For most suites, compose the server from focused interfaces and fake those
instead of `RoutesReceiver`:

```go
type Database interface {
	Portfolio(ctx context.Context, id string) (Portfolio, error)
	InsertPortfolio(ctx context.Context, meta Metadata) (Portfolio, error)
}

type UsersService interface {
	SessionUserID(ctx context.Context) (string, error)
}

type Server struct {
	Logger   *slog.Logger
	Database Database
	Users    UsersService
}
```

```go
//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o internal/fake/database.go . Database

func TestGetPortfolio(t *testing.T) {
	fakeDB := &fake.Database{}
	fakeDB.PortfolioReturns(Portfolio{ID: "123", Name: "Growth"}, nil)

	server := Server{Database: fakeDB}
	result, err := server.GetPortfolio(context.Background(), "123")

	require.NoError(t, err)
	assert.Equal(t, "Growth", result.Name)
	assert.Equal(t, 1, fakeDB.PortfolioCallCount())
}
```

Faking the service layer makes the tested layer thicker: each test exercises
receiver methods, domain errors, and `TemplateData` extensions. Faking
`RoutesReceiver` only verifies that generated handlers call the right method —
routing and parameter parsing muxt's own tests already cover.

Three thicknesses, by where the fake sits:

1. **Thin — fake `RoutesReceiver`.** Verifies wiring only. Use when
   specifically testing handler generation behavior.
2. **Medium — fake the service interfaces** (above). Fast, no I/O, covers
   most business logic. The default.
3. **Thick — fake the services' own collaborators**, e.g. a
   [sqlc](https://docs.sqlc.dev/en/latest/reference/config.html#go) `Querier`
   with `emit_interface: true`. Each test covers a whole call path; more
   verbose. Pair it with [real query tests](https://github.com/peterldowns/pgtestdb).

The [simple example](../examples/simple) shows this layout end to end, and
[package layout](../reference/package-layout.md) names where each file lives.
