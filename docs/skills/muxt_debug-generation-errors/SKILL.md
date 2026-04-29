---
name: muxt-debug-generation-errors
description: "Muxt: Use when `muxt generate` or `muxt check` fails with an error. Covers reading error messages, diagnosing common error categories, and the fix-and-rerun workflow."
---

# Debugging muxt Generation Errors

When `muxt generate` or `muxt check` fails, use this workflow to diagnose and fix.

## When to use this skill

- `muxt generate` returns an error.
- `muxt check` reports a problem (template field error, unused template, dead code).
- Compiler error after `go generate` claims a method is missing on the receiver — that's actually a muxt issue.

## Reading error messages

Muxt errors include:
- **File position** — which `.gohtml` or `.go` file and line.
- **Template name** — the template that triggered the error.
- **Expected vs actual** — what muxt expected and what it found.

Errors from `muxt generate` include the affected type or template and the package path. Template-body errors from `muxt check` include file position and field name.

## `muxt check` vs `muxt generate`

| Command | What | Writes files? |
|---------|------|---------------|
| `muxt generate` | Type-checks AND generates handler code | Yes |
| `muxt check` | Type-checks only (read-only) | No |

`muxt check` only accepts `--use-templates-variable` and `--verbose`. It discovers types from the **already-generated code**, so run `muxt generate` first, then `muxt check`.

`muxt check` also detects:
- Unused templates (defined but never called as routes).
- Dead code outside `{{define}}` blocks.
- Template-body type errors (accessing nonexistent fields).

## Fix workflow

1. **Read the error** — file, template, problem.
2. **Find the template** — open the `.gohtml` at the reported position.
3. **Check the method signature** — receiver method exists with right parameters and returns?
4. **Fix the mismatch** — template name, method, or both.
5. **Re-run:**
   ```bash
   muxt check          # fast validation
   muxt generate       # generate + validate
   go test ./...
   ```

## Quick diagnosis lookup

`references/error-catalog.md` has the full table. Summary by symptom:

| Symptom | Likely category |
|---------|-----------------|
| `could not find receiver type` | Wrong `--use-receiver-type` value |
| `could not find templates variable` | Missing package-level `var templates` |
| Compiler: "does not implement RoutesReceiver" | Method not found on receiver — inferred but not implemented |
| `expected N arguments but got M` | Param count mismatch |
| `argument type mismatch`, `unsupported type` | Param type unsupported |
| `method ... has no results` | Method returns nothing — needs at least one return value |
| `unsupported type: url.URL`, `unsupported composite type` | Form field uses unsupported Go type |
| `unsupported return type with form`, `bool return with form` | Form method needs `(T, error)` or `T` |
| `duplicate route pattern: GET /` | Two templates with the same method+path |
| `.Result.WrongField` field not found | Template body accesses missing struct field |
| `unused template`, `dead code outside define` | Cleanup needed (remove or wrap in `{{define}}`) |
| `TemplateRoutePaths method name collision` | Two handlers differ only in first-letter case |
| `cannot export identifier "_..."` | Handler name starts with non-letter |
| `unknown command`, `unknown flag` | CLI typo |

Each category has fix steps and a corresponding test case in `cmd/muxt/testdata/err_*.txt` — see `references/error-catalog.md`.

## Reference files

- `references/error-catalog.md` — every error category with full message, cause, fix steps, and the test file demonstrating the fix.
