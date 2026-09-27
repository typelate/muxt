# Package Layout

A muxt package past a few routes settles into this shape:

```text
internal/hypertext/
├── server.go                  # Server type and the interfaces it depends on
├── template.go                # templates variable, go:generate directive
├── functions.go               # template functions
├── template_data.go           # TemplateData method extensions
├── errors.go                  # domain errors with StatusCode() methods
├── portfolio.go               # receiver methods, one file per domain
├── portfolio_list.gohtml      # route templates, prefixed by domain
├── _header.gohtml             # shared partials, prefixed with _ (a bare-directory //go:embed skips them; use a glob or all:)
├── template_routes.go         # generated
├── portfolio_list_template_routes_gen.go   # generated with --output-multiple-files
├── portfolio_test.go          # receiver tests against fakes, route tests through httptest
└── internal/fake/             # counterfeiter fakes of the interfaces in server.go
```

| File | See |
|------|-----|
| `portfolio.go` | [Call Results](call-results.md) |
| `template_data.go` | [Extend TemplateData](../how-to/extend-template-data.md) |
| `functions.go` | [Template functions](../how-to/template-functions.md) |
| `errors.go` | [Domain error status codes](../how-to/domain-error-status-codes.md) |
| `portfolio_test.go` | [Structure a project for testing](../how-to/receiver-package-and-testing.md) |

Commit the generated files.
