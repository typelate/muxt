---
name: muxt-debug-generation-errors
description: "Muxt: Use when `muxt generate` or `muxt check` fails with an error. Covers reading error messages, diagnosing common error categories, and the fix-and-rerun workflow."
---

# Debugging Generation Errors

When `muxt generate` or `muxt check` fails, use this workflow to diagnose and fix the error.

## Reading Error Messages

Muxt errors include:
- **File position** — which `.gohtml` or `.go` file and line
- **Template name** — the template that triggered the error
- **Expected vs actual** — what Muxt expected and what it found

Example error:

```
Error: could not find receiver type server in example.com/internal/hypertext
```

Errors from `muxt generate` include the affected type or template and the package path. Template body errors from `muxt check` include file position and field name.

## `muxt check` vs `muxt generate`

| Command | What it does | Writes files? |
|---------|-------------|---------------|
| `muxt generate` | Type-checks and generates handler code | Yes |
| `muxt check` | Type-checks only (read-only validation) | No |

Use `muxt check` for fast feedback during development. Use `muxt generate` when you're ready to produce the handler code.

**Note:** `muxt check` only accepts `--use-templates-variable` and `--verbose` flags. It does not accept `--use-receiver-type` — it discovers types from the generated code. Run `muxt generate` first to produce the generated code, then `muxt check` to validate template body types.

`muxt check` also detects issues that `muxt generate` does not:
- Unused templates (defined but never called as routes)
- Dead code outside `{{define}}` blocks
- Template body type errors (accessing nonexistent fields)

## Fix Workflow

1. **Read the error** — identify the file, template, and problem
2. **Find the template** — open the `.gohtml` file at the reported position
3. **Check the method signature** — ensure the receiver method exists with the right parameters and return types
4. **Fix the mismatch** — update the template name, method, or both
5. **Re-run** — `muxt check` or `muxt generate` to verify the fix

```bash
muxt check          # fast validation
muxt generate       # generate + validate
go test ./...       # run tests after fixing
```

## Common Error Categories

### Missing or Wrong Receiver Type

**Error:** `could not find receiver type`

**Cause:** The `--use-receiver-type` flag names a type that doesn't exist in the package.

**Fix:** Check the flag value matches your type name exactly (case-sensitive):

```bash
# Wrong
muxt generate --use-receiver-type=server

# Right
muxt generate --use-receiver-type=Server
```

**Test file:** `err_missing_receiver_type.txt`

### Missing Templates Variable

**Error:** `could not find templates variable`

**Cause:** Muxt can't find a package-level `var templates` declaration.

**Fix:** Ensure you have a package-level variable that parses templates:

```go
//go:embed *.gohtml
var templateFS embed.FS

var templates = template.Must(template.ParseFS(templateFS, "*.gohtml"))
```

The variable must be at package scope (not inside a function).

**Test file:** `err_missing_templates_variable.txt`

### Method Not Found on Receiver

When a method in the template call expression doesn't exist on the receiver type, `muxt generate` does **not** produce an error. Instead, it infers a method signature from the call expression and adds it to the generated `RoutesReceiver` interface. The Go compiler then fails because the receiver type doesn't satisfy the interface:

```
*Server does not implement RoutesReceiver (missing method GetUser)
```

**Fix:** Either add the method to the receiver type or fix the template name:

```go
// Template says: GetUser(ctx, id)
// Add this method:
func (s Server) GetUser(ctx context.Context, id int) (User, error) { ... }
```

Run `go generate ./...` followed by `go build` or `go test` to see the compiler error.

### Method Signature Mismatches

#### Wrong Parameter Count

**Error:** `expected 2 arguments but got 3` (or similar)

**Cause:** The template call expression has a different number of arguments than the method.

**Fix:** Match the template arguments to the method parameters (or vice versa based on what was changed more recently):

```gotemplate
{{define "GET /article/{id} GetArticle(ctx, id)"}}
```

```go
func (s Server) GetArticle(ctx context.Context, id int) (Article, error)
```

**Test files:** `err_missing_args.txt`, `err_extra_args.txt`

#### Wrong Parameter Types

**Error:** `argument type mismatch` or `unsupported type`

**Cause:** A template argument maps to a parameter type that Muxt can't parse.

**Fix:** Use supported types: `context.Context`, `*http.Request`, `http.ResponseWriter`, `string`, `int`, `bool`, or a type implementing `encoding.TextUnmarshaler`.

**Test files:** `err_arg_type_mismatch.txt`, `err_arg_context_type.txt`, `err_arg_request_type.txt`, `err_arg_response_type.txt`, `err_arg_path_value_type.txt`

#### No Return Value

**Error:** `method for pattern "GET / Home()" has no results it should have one or two`

**Cause:** The method returns nothing. Muxt needs at least one return value to render the template.

**Fix:** Add a return type. Use a named empty struct `type HomePage struct{}` if no data is needed:

```go
// Wrong: no return
func (s Server) Home(ctx context.Context) { ... }

// Right: returns data for the template
func (s Server) Home(ctx context.Context) HomePage { ... }
```

**Test file:** `err_method_no_return.txt`

### Form Field Type Errors

**Error:** `unsupported type: url.URL` or `unsupported composite type`

**Cause:** A form struct field uses a type that Muxt can't parse from form data.

**Fix:** Use supported field types: `string`, `int`, `bool`, `[]string`, `[]int`. For complex types, parse them manually in your method.

**Test files:** `err_form_unsupported_field_type.txt`, `err_form_unsupported_composite.txt`

### Form Return Type Errors

**Error:** `unsupported return type with form` or `bool return with form`

**Cause:** The method's return type is incompatible with form handling.

**Fix:** Use `(T, error)` or `T` as the return type for form methods.

**Test files:** `err_form_unsupported_return.txt`, `err_form_bool_return.txt`

### Duplicate Patterns

**Error:** `duplicate route pattern: GET /`

**Cause:** Two templates define the same HTTP method + path combination.

**Fix:** Each route pattern must be unique. Rename or remove the duplicate:

```gotemplate
{{/* These conflict: */}}
{{define "GET / Greetings()"}}...{{end}}
{{define "GET / Welcome()"}}...{{end}}

{{/* Fix: use different paths or remove one */}}
{{define "GET / Welcome()"}}...{{end}}
{{define "GET /greetings Greetings()"}}...{{end}}
```

**Test file:** `err_duplicate_pattern.txt`

### Template Type Errors (`muxt check`)

**Error:** `.Result.WrongField` — field not found

**Cause:** The template body accesses a field that doesn't exist on the method's return type.

**Fix:** Check the return type's fields and fix the template:

```go
type Article struct {
    Title string
    Body  string
}
```

```gotemplate
{{/* Wrong: */}}
<h1>{{.Result.Name}}</h1>

{{/* Right: */}}
<h1>{{.Result.Title}}</h1>
```

**Test file:** `err_check_with_wrong_field.txt`

### Dead Code / Unused Templates (`muxt check`)

**Error:** `unused template "sidebar"` or `dead code outside define`

**Cause:** A template is defined but never referenced, or there's content outside any `{{define}}` block.

**Fix:** Remove unused templates or wrap content in a `{{define}}` block.

**Test files:** `err_check_with_unused_template.txt`, `err_check_with_dead_code_outside_define.txt`

### CLI Errors

**Error:** `unknown command` or `unknown flag`

**Cause:** Typo in the command or flag name.

**Fix:** Check `muxt --help` for available commands and flags.

**Test files:** `err_cli_unknown_command.txt`, `err_unknown_flag.txt`, `err_invalid_identifier_flag.txt`, `err_invalid_output_filename.txt`

## Reference

### All Error Test Cases (`cmd/muxt/testdata/err_*`)

| Error | Test File |
|-------|-----------|
| Missing receiver type | `err_missing_receiver_type.txt` |
| Missing templates variable | `err_missing_templates_variable.txt` |
| Undefined form method (inferred) | `err_form_with_undefined_method.txt` |
| Method has no return | `err_method_no_return.txt` |
| Missing arguments | `err_missing_args.txt` |
| Extra arguments | `err_extra_args.txt` |
| Argument type mismatch | `err_arg_type_mismatch.txt` |
| Context type error | `err_arg_context_type.txt` |
| Request type error | `err_arg_request_type.txt` |
| Request pointer type error | `err_arg_request_ptr_type.txt` |
| Response type error | `err_arg_response_type.txt` |
| Path value type error | `err_arg_path_value_type.txt` |
| Field list type error | `err_arg_field_list_type.txt` |
| Unsupported form field type | `err_form_unsupported_field_type.txt` |
| Unsupported form composite | `err_form_unsupported_composite.txt` |
| Unsupported form return | `err_form_unsupported_return.txt` |
| Bool return with form | `err_form_bool_return.txt` |
| Duplicate route pattern | `err_duplicate_pattern.txt` |
| Wrong template field | `err_check_with_wrong_field.txt` |
| Unused template | `err_check_with_unused_template.txt` |
| Dead code outside define | `err_check_with_dead_code_outside_define.txt` |
| Unknown CLI command | `err_cli_unknown_command.txt` |
| Unknown flag | `err_unknown_flag.txt` |
| Invalid identifier flag | `err_invalid_identifier_flag.txt` |
| Invalid output filename | `err_invalid_output_filename.txt` |
