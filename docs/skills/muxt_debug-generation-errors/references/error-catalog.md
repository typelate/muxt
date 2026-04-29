# muxt error catalog

Each entry: error message, cause, fix, and the txtar test case in `cmd/muxt/testdata/`.

## Missing or wrong receiver type

**Error:** `could not find receiver type`

**Cause:** `--use-receiver-type` flag names a type that doesn't exist (case-sensitive).

**Fix:** Match the type name exactly:

```bash
# Wrong
muxt generate --use-receiver-type=server
# Right
muxt generate --use-receiver-type=Server
```

**Test:** `err_missing_receiver_type.txt`

## Missing templates variable

**Error:** `could not find templates variable`

**Cause:** No package-level `var templates` declaration.

**Fix:**

```go
//go:embed *.gohtml
var templateFS embed.FS

var templates = template.Must(template.ParseFS(templateFS, "*.gohtml"))
```

The variable must be at package scope, not inside a function.

**Test:** `err_missing_templates_variable.txt`

## Method not found on receiver

`muxt generate` does NOT produce an error directly. It infers a method signature from the call expression and adds it to `RoutesReceiver`. The Go compiler then fails:

```
*Server does not implement RoutesReceiver (missing method GetUser)
```

**Fix:** add the method, or fix the template name:

```go
// Template says: GetUser(ctx, id)
func (s Server) GetUser(ctx context.Context, id int) (User, error) { ... }
```

Run `go generate ./...` then `go build` or `go test` to surface the compiler error.

## Wrong parameter count

**Error:** `expected 2 arguments but got 3` (or similar)

**Fix:** match arguments to method parameters:

```gotmpl
{{define "GET /article/{id} GetArticle(ctx, id)"}}
```

```go
func (s Server) GetArticle(ctx context.Context, id int) (Article, error)
```

**Tests:** `err_missing_args.txt`, `err_extra_args.txt`

## Wrong parameter types

**Error:** `argument type mismatch` or `unsupported type`

**Fix:** use supported types — `context.Context`, `*http.Request`, `http.ResponseWriter`, `string`, `int`, `bool`, `encoding.TextUnmarshaler`.

**Tests:** `err_arg_type_mismatch.txt`, `err_arg_context_type.txt`, `err_arg_request_type.txt`, `err_arg_response_type.txt`, `err_arg_path_value_type.txt`

## No return value

**Error:** `method for pattern "GET / Home()" has no results it should have one or two`

**Fix:** add a return type. Use a named empty struct if no data is needed:

```go
// Wrong
func (s Server) Home(ctx context.Context) { ... }

// Right
func (s Server) Home(ctx context.Context) HomePage { ... }
```

**Test:** `err_method_no_return.txt`

## Form field type errors

**Error:** `unsupported type: url.URL` or `unsupported composite type`

**Fix:** use supported field types — `string`, `int`, `bool`, `[]string`, `[]int`, or types implementing `encoding.TextUnmarshaler`. For complex types, parse manually.

**Tests:** `err_form_unsupported_field_type.txt`, `err_form_unsupported_composite.txt`

## Form return type errors

**Error:** `unsupported return type with form` or `bool return with form`

**Fix:** use `(T, error)` or `T` for form methods.

**Tests:** `err_form_unsupported_return.txt`, `err_form_bool_return.txt`

## Duplicate patterns

**Error:** `duplicate route pattern: GET /`

**Fix:** each route pattern must be unique. Rename or remove a duplicate:

```gotmpl
{{/* Conflicts: */}}
{{define "GET / Greetings()"}}...{{end}}
{{define "GET / Welcome()"}}...{{end}}

{{/* Fix: */}}
{{define "GET / Welcome()"}}...{{end}}
{{define "GET /greetings Greetings()"}}...{{end}}
```

**Test:** `err_duplicate_pattern.txt`

## Template type errors (`muxt check`)

**Error:** `.Result.WrongField` — field not found

**Fix:** match template access to the actual return type:

```go
type Article struct { Title, Body string }
```

```gotmpl
{{/* Wrong */}} <h1>{{.Result.Name}}</h1>
{{/* Right */}} <h1>{{.Result.Title}}</h1>
```

**Test:** `err_check_with_wrong_field.txt`

## Dead code / unused templates (`muxt check`)

**Error:** `unused template "sidebar"` or `dead code outside define`

**Fix:** remove unused templates or wrap content in `{{define}}`.

**Tests:** `err_check_with_unused_template.txt`, `err_check_with_dead_code_outside_define.txt`

## Path method name collision

**Error:** `TemplateRoutePaths method name collision: handlers list and List both produce method List`

**Cause:** two handler methods differ only in case of the first letter. `TemplateRoutePaths` methods are always exported, so both produce the same method name.

**Fix:** rename one method.

**Test:** `err_path_method_collision.txt`

## Unexportable path method identifier

**Error:** `cannot export identifier "_list" for TemplateRoutePaths method: first character '_' has no uppercase form`

**Fix:** rename the handler method to start with a letter.

**Test:** `err_path_method_unexportable.txt`

## CLI errors

**Error:** `unknown command` or `unknown flag`

**Fix:** `muxt --help`.

**Tests:** `err_cli_unknown_command.txt`, `err_unknown_flag.txt`, `err_invalid_identifier_flag.txt`, `err_invalid_output_filename.txt`

## All error test cases summary

| Error | Test |
|-------|------|
| Missing receiver type | `err_missing_receiver_type.txt` |
| Missing templates variable | `err_missing_templates_variable.txt` |
| Undefined form method (inferred) | `err_form_with_undefined_method.txt` |
| Method has no return | `err_method_no_return.txt` |
| Missing / extra arguments | `err_missing_args.txt`, `err_extra_args.txt` |
| Argument type mismatch (general / context / request / request-pointer / response / path-value / field-list) | `err_arg_type_mismatch.txt`, `err_arg_context_type.txt`, `err_arg_request_type.txt`, `err_arg_request_ptr_type.txt`, `err_arg_response_type.txt`, `err_arg_path_value_type.txt`, `err_arg_field_list_type.txt` |
| Unsupported form field / composite / return / bool return | `err_form_unsupported_field_type.txt`, `err_form_unsupported_composite.txt`, `err_form_unsupported_return.txt`, `err_form_bool_return.txt` |
| Duplicate route pattern | `err_duplicate_pattern.txt` |
| Wrong template field | `err_check_with_wrong_field.txt` |
| Unused template / dead code outside define | `err_check_with_unused_template.txt`, `err_check_with_dead_code_outside_define.txt` |
| Unknown CLI command / flag / invalid identifier / invalid output filename | `err_cli_unknown_command.txt`, `err_unknown_flag.txt`, `err_invalid_identifier_flag.txt`, `err_invalid_output_filename.txt` |
| Path method name collision / unexportable | `err_path_method_collision.txt`, `err_path_method_unexportable.txt` |
