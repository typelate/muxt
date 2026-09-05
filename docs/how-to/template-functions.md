# How to add custom template functions

Templates need domain-specific formatting — currency, percentages, dates — that Go's standard template functions don't provide. Register pure formatting functions before parsing so every template can call them in pipelines.

```go
//go:embed *.gohtml
var fs embed.FS

//go:generate muxt generate --use-receiver-type=Server
var templates = template.Must(
    template.New("").
        Funcs(template.FuncMap{
            "percent":  percentFn,
            "dollars":  dollarsFn,
            "dateOnly": dateOnlyFn,
        }).
        ParseFS(fs, "*.gohtml"),
)

func percentFn(multiply bool, value float64) string {
    if multiply {
        value *= 100
    }
    return fmt.Sprintf("%.2f%%", value)
}

func dollarsFn(value float64) string {
    return message.NewPrinter(language.English).Sprintf("$%0.2f", value)
}

func dateOnlyFn(t time.Time) string {
    return t.Format(time.DateOnly)
}
```

```gotmpl
{{define "GET /report/{id} GetReport(ctx, id)"}}
<p>Return: {{.Result.Return | percent true}}</p>
<p>Balance: {{.Result.Balance | dollars}}</p>
<p>Date: {{.Result.Date | dateOnly}}</p>
{{end}}
```

`muxt check` resolves functions registered in the `templates` variable's construction chain, so a wrong argument type in a pipeline fails the check.

Keep functions pure — input to output, no side effects, no I/O. That boundary decides where logic lives:

| Logic | Home |
|-------|------|
| Pure formatting and conversion | Template function |
| Needs the request or the result | [`TemplateData` extension](extend-template-data.md) |
| Business rules | Receiver method |

Good candidates: locale-aware numbers, date display, markdown rendering, truncation, URL encoding.
