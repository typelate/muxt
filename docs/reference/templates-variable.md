# Making Template Source Files Discoverable

`muxt check` finds function calls and evaluates the template name and parameter type to do static analysis of your template actions.
Generate HTTP handler functions comply with this expectation by using string literals in `ExecuteTemplate` calls.
This heuristic to map template names to their data parameter only works for checking.

To map template names to expected handler behavior, `muxt generate` needs to find an assignment expression where the templates are parsed.
This configuration is much more brittle than the scanning heuristic used by `muxt check` so you may encounter problems when you deviate from the given example.
Please file a GitHub issue if you encounter problems with your configuration.

`muxt generate` expects you to use a global variable with type `*template.Template` initialized by an assignment expression.
The assignment expression must call ParseFS with a variable of type `embed.FS` as the first argument.
`muxt generate` will then parse the embedded files and find the templates that match the expected template name pattern.

Here is a minimal example of a Go source file that `muxt generate` can use to find templates:
```go
package server

import (
	"embed"
	"html/template"
)

//go:embed *.gohtml
var goTemplateHypertextFiles embed.FS

var myTemplates = template.Must(template.ParseFS(goTemplateHypertextFiles, "*"))
```

*[(See Muxt CLI Test/templates_and_embed_in_gen_decl)](../../cmd/muxt/testdata/templates_and_embed_in_gen_decl.txt)*

The right-hand side of the assignment expression may include any of the following template function calls:

- [`Must`](https://pkg.go.dev/html/template#Must)
- [`Parse`](https://pkg.go.dev/html/template#Parse)
- [`New`](https://pkg.go.dev/html/template#New)
- [`ParseFS`](https://pkg.go.dev/html/template#ParseFS)

or methods:

- [`Template.Parse`](https://pkg.go.dev/html/template#Template.Parse)
- [`Template.New`](https://pkg.go.dev/html/template#Template.New)
- [`Template.ParseFS`](https://pkg.go.dev/html/template#Template.ParseFS)
- [`Template.Delims`](https://pkg.go.dev/html/template#Template.Delims)
- [`Template.Option`](https://pkg.go.dev/html/template#Template.Option)
- [`Template.Funcs`](https://pkg.go.dev/html/template#Template.Option)

*[(See Muxt CLI Test/templates_multiple_parsefs)](../../cmd/muxt/testdata/templates_multiple_parsefs.txt)* for using multiple ParseFS calls.

You can use a different variable name for the `*template.Template` by invoking `muxt generate` with the
`--templates-variable=someOtherName` flag
and ensure you have a globally scoped variable someOtherName where the right-hand side of the expression is
`template.Must()` with some parse calls.
