# How to structure a muxt project for testing

Put the receiver type, templates, and generated routes in a library package and keep `package main` to wiring. Nothing can import `package main`, so a receiver there cannot be faked from another package. [Package layout](../reference/package-layout.md) names where each file lives.

```go
package hypertext

//go:embed *.gohtml
var templatesFS embed.FS

//go:generate muxt generate --use-receiver-type=Server
var templates = template.Must(template.ParseFS(templatesFS, "*.gohtml"))

type Database interface {
	CreateIncident(context.Context, database.CreateIncidentParams) (database.Incident, error)
}

type UsersService interface {
	Portfolio(context.Context, string) (Portfolio, error)
}

type Server struct {
	Database Database
	Users    UsersService
}
```

```go
hypertext.TemplateRoutes(mux, hypertext.Server{Database: queries, Users: users})
```

## Test through the generated routes

Register the routes on a fresh mux per test and drive it with `httptest`. An in-memory sqlite database (`modernc.org/sqlite`) keeps tests hermetic. The route `POST /incidents 201 CreateIncident(ctx, form)` sets the status in its name:

```go
func TestCreateIncident(t *testing.T) {
	// openMemoryDB: sql.Open("sqlite", ":memory:") plus the schema.
	srv := Server{Database: database.New(openMemoryDB(t))}
	mux := http.NewServeMux()
	TemplateRoutes(mux, srv)
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

## Choose what to fake

Where the fake sits decides how much each test covers:

| Fake | Covers | Use for |
|------|--------|---------|
| `RoutesReceiver` | wiring and template branches, driven by canned results | rendering states: empty list, error, populated |
| Service interfaces on `Server` | receiver methods and domain errors | most suites |
| The services' own collaborators, such as a sqlc `Querier` | whole call paths | end-to-end paths, paired with real query tests |

Routing and parameter parsing are covered by muxt's own tests.

Fake the small interfaces on `Server` with [counterfeiter](https://github.com/maxbrunsfeld/counterfeiter):

```go
//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o internal/fake/users_service.go . UsersService
```

The fake imports `hypertext`, so the test is `package hypertext_test`:

```go
func TestGetPortfolio(t *testing.T) {
	users := &fake.UsersService{}
	users.PortfolioReturns(hypertext.Portfolio{ID: "123", Name: "Growth"}, nil)
	result, err := hypertext.Server{Users: users}.GetPortfolio(context.Background(), "123")
	require.NoError(t, err)
	assert.Equal(t, "Growth", result.Name)
}
```

To assert on the returned markup, see [HTML is the API](../explanation/html-is-the-api.md). To find assertions the suite still lacks, see [Find Untested Template Behavior](../tutorials/find-untested-template-behavior.md).
