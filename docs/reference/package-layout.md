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
├── _header.gohtml             # shared partials, prefixed with _
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

## Hub layout

With `route_paths` in `muxt.json`, the directory of `muxt.json` is the hub and the muxt packages sit below it:

```text
internal/hypertext/
├── muxt.json                  # route_paths and packages
├── hypertext.go               # TemplateSource embed.FS, shared path parameter types
├── nav.gohtml                 # shared template
├── route_paths.go             # generated: the path helpers of every listed package
├── admin/
│   ├── admin.go
│   ├── index.gohtml
│   └── template_routes.go     # generated, imports the hub
└── blog/
    ├── blog.go
    ├── blog.gohtml
    ├── assets/
    └── template_routes.go     # generated, imports the hub
```

Packages import the hub; the hub imports none of them ([steps](../how-to/link-to-a-route-in-another-package.md), [`route_paths`](muxt-json.md#route_paths)).

## Edge cases

- **Partials prefixed with `_`.** A bare-directory `//go:embed` skips them, so embed them with a glob or `all:`.
