# How to extend TemplateData with request-aware helpers

Add a method to the generated `TemplateData[R, T]` in a file of your own, such as `template_data.go` ([package layout](../reference/package-layout.md)); `muxt check` type-checks the call. `user` stands for your session package.

```go
func (data *TemplateData[R, T]) CanEditPortfolio(p Portfolio) bool {
	session, ok := user.SessionFromContext(data.Request().Context())
	return ok && session.UserID != "" && p.AuthorID == session.UserID
}
```

```gotmpl
{{if .CanEditPortfolio .Result}}<button hx-get="{{.Path.EditPortfolio .Result.ID}}">Edit</button>{{end}}
```

The generated accessors are listed under [TemplateData](../reference/call-results.md#templatedata). For htmx request headers, `--output-htmx` ([flags](../reference/cli.md#flags)) generates `HXRequest`, `HXBoosted`, and the other `HX*` helpers ([reference_output_htmx.txt](../../cmd/muxt/testdata/reference_output_htmx.txt)).

A worked extension is in [map domain errors to status codes](domain-error-status-codes.md).
