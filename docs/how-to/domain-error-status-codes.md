# How to map domain errors to HTTP status codes

A returned error responds 500. To respond 404 for a missing record, give the error a `StatusCode() int` method and apply it from the template with an `ErrorStatusCode` method on `TemplateData`, defined below. muxt consults `StatusCode` itself only for a method that returns only an error whose declared type has it ([status codes](../reference/call-results.md#status-code-control)); a `(T, error)` method, the usual shape for a lookup, always responds 500 unless the template sets another code.

Define the error in the package that holds the templates so the services it wraps stay HTTP-free. `database` and `security` stand for your own packages.

```go
type ReadSecurityError struct{ err error }

func (r *ReadSecurityError) Error() string { return "failed to read security" }
func (r *ReadSecurityError) Unwrap() error { return r.err }

func (r *ReadSecurityError) StatusCode() int {
	if database.IsNotFoundError(r.err) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}
```

Return it from the receiver method:

```go
func (s *Server) ReadSecurity(ctx context.Context, id string) (security.Document, error) {
	doc, err := s.db.SecurityDocument(ctx, id)
	if err != nil {
		return security.Document{}, &ReadSecurityError{err: err}
	}
	return doc, nil
}
```

Add an `ErrorStatusCode` method to `TemplateData` ([extend TemplateData](extend-template-data.md)) and call it where the error renders. It prints nothing ([`.String`](../reference/call-results.md#templatedata)):

```go
func (data *TemplateData[R, T]) ErrorStatusCode() *TemplateData[R, T] {
	var sc interface{ StatusCode() int }
	if errors.As(data.Err(), &sc) {
		return data.StatusCode(sc.StatusCode())
	}
	return data
}
```

```gotmpl
{{define "GET /security/{id} ReadSecurity(ctx, id)"}}
{{- .ErrorStatusCode -}}
{{if .Err}}<p class="error">{{.Err}}</p>{{else}}...{{end}}
{{end}}
```

Keep `Error()` text user-facing and the wrapped cause for logs and `errors.Is`.

The template approach works only on routes whose method does not take `response`. When it does, muxt writes the template buffer without applying any status code, so a missing record renders at 200 and `ErrorStatusCode` is silently ignored. See [routes that take `response`](../reference/call-parameters.md#routes-that-take-response).
