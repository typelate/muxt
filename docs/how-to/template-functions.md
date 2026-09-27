# How to add custom template functions

Register formatting functions with `Funcs` before `ParseFS`, passing an inline `template.FuncMap` literal ([supported initializers](../reference/templates-variable.md#supported-initializers)); `muxt check` resolves them.

```go
//go:embed *.gohtml
var templateFS embed.FS

var templates = template.Must(
	template.New("").
		Funcs(template.FuncMap{"dollars": dollars, "dateOnly": dateOnly}).
		ParseFS(templateFS, "*.gohtml"),
)

func dollars(v float64) string     { return fmt.Sprintf("$%.2f", v) }
func dateOnly(t time.Time) string { return t.Format(time.DateOnly) }
```

```gotmpl
<p>Balance: {{.Result.Balance | dollars}} on {{.Result.Date | dateOnly}}</p>
```

Keep functions pure; logic that needs the request or the result belongs in a [`TemplateData` extension](extend-template-data.md).
