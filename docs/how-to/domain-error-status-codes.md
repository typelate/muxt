# How to map domain errors to HTTP status codes

Muxt never inspects a returned error — a method error renders the template with `.Err` set and responds 500. To respond 404 for a missing record and 403 for a permission failure, give the errors your receiver methods return a `StatusCode() int` method and apply it during rendering with a small `TemplateData` extension.

The error type lives at the boundary between your domain and the hypertext package, and it confines the HTTP knowledge: importing `net/http` for the status constants is fine *here*, in the error type — what stays HTTP-free is everything behind it, the services and repositories whose errors it wraps.

Define the error at that boundary:

```go
type ReadSecurityError struct {
    err error
}

func NewReadSecurityError(err error) *ReadSecurityError {
    if err == nil {
        return nil
    }
    return &ReadSecurityError{err: err}
}

func (r *ReadSecurityError) Error() string {
    if database.IsNotFoundError(r.err) {
        return "security not found"
    }
    return "failed to read security"
}

func (r *ReadSecurityError) StatusCode() int {
    if database.IsNotFoundError(r.err) {
        return http.StatusNotFound
    }
    return http.StatusInternalServerError
}

func (r *ReadSecurityError) Unwrap() error {
    return r.err
}
```

Wrap at the boundary in the receiver method — log the internals, return the user-facing error:

```go
func (s *Server) ReadSecurity(ctx context.Context, id string) (security.Document, error) {
    doc, err := s.db.SecurityDocument(ctx, id)
    if err != nil {
        s.Logger.ErrorContext(ctx, "failed to load security", slog.String("id", id))
        return security.Document{}, NewReadSecurityError(err)
    }
    return doc, nil
}
```

Add the `ErrorStatusCode` extension from [extending TemplateData](extend-template-data.md) and invoke it where the error renders. `TemplateData.String` returns an empty string, so the call renders nothing and only sets the status:

```gotmpl
{{define "GET /security/{id} ReadSecurity(ctx, id)"}}
{{- .ErrorStatusCode -}}
{{if .Err}}<p class="error">{{.Err}}</p>{{else}}...{{end}}
{{end}}
```

Conventions that make this hold up:

- Constructors return `nil` for `nil` input, so wrapping stays one line.
- `Error()` text is user-facing; the wrapped cause is for logs and `errors.Is`/`errors.As`. Don't leak internals through the message.
- `StatusCode()` maps domain states (not found, unauthorized, invalid) to HTTP semantics in one place.

Use this for operations with more than one failure state, and for services consumed by both HTTP and non-HTTP callers — the error type carries the semantics either way.
