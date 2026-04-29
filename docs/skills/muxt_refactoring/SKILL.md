---
name: muxt-refactoring
description: "Muxt: Use when renaming receiver methods, changing route patterns, moving templates between files, splitting packages, or adding/removing method parameters in a Muxt codebase. Guides safe refactoring through Muxt's template-method coupling chain."
---

# Refactoring Muxt Routes

Muxt couples templates, route patterns, and receiver methods by name. Refactoring any one requires updating the others. This skill walks the coupling chain safely.

## When to use this skill

- Renaming a receiver method.
- Changing a route pattern (path, method, params).
- Moving templates between `.gohtml` files.
- Splitting a Muxt package into separate packages.
- Adding/removing method parameters.

## Safe refactoring loop

After every change, run:

```bash
go generate ./...    # regenerate handler code
muxt check           # type-check templates against generated code
go test ./...        # run tests
```

`muxt check` validates against already-generated code, so it must run after `go generate`. Stop and fix errors before the next change.

## Renaming a receiver method

Order matters because gopls won't rename the concrete method while the generated interface still references the old name.

1. Find references — `muxt list-template-callers --match "GetArticle"` and `gopls references` / `gopls implementation` on the method's file:line:column.
2. Update the template name(s) — `{{define "GET /article/{id} FetchArticle(ctx, id)"}}`.
3. **Regenerate first** — `go generate ./...` — so the interface uses the new name.
4. Rename the concrete method — `gopls rename -w main.go:19:20 FetchArticle` (or via the generated interface method to cascade to all implementations).
5. Update tests (counterfeiter call counts: `app.FetchArticleCallCount()`).
6. Regenerate + `go test`.

Full commands and `gopls rename -d` dry-run example in `references/examples.md`.

## Changing a route pattern

1. Update the template name (path, method, param names — change types where needed).
2. Update `$.Path` callers in templates that reference the route.
3. Update tests to use `TemplateRoutePaths` methods (`paths.GetArticle("my-post")`) instead of hardcoded paths.
4. Update the method signature if param types changed.
5. Regenerate + test.

Full before/after examples in `references/examples.md`.

## Moving templates between files

Muxt is file-agnostic within a package. Move a `{{define}}` block between `.gohtml` files freely — no code change. **File order may affect template overriding.** Ensure `//go:embed *.gohtml` covers the new location; add subdirectory patterns if needed (`//go:embed *.gohtml partials/*.gohtml`). Run `muxt check`.

## Splitting into multiple packages

Use `--use-receiver-type-package` for cross-package receivers:

```bash
muxt generate --use-receiver-type=Handler --use-receiver-type-package=example.com/internal/app
```

Move the receiver type and methods → update `//go:generate` → `go generate` → fix import issues in generated code.

## Adding/removing parameters

Add or remove the parameter in *both* the template call expression and the method signature, then regenerate. For tests that depended on the removed parameter, update or delete the assertion. Full before/after in `references/examples.md`.

## Cleanup

Switching between single-file and multi-file output, or changing the output file name? Muxt cleans up orphaned generated files on the next `go generate`. If you switch strategies and want immediate cleanup, delete the old generated files manually.

## Analyzing coupling before a refactor

`muxt list-template-calls` and `muxt list-template-callers` with `--format json` + jq can find duplicate route patterns (same method + path), sub-templates shared by multiple routes (a hidden coupling), all callers of a specific sub-template, and what a given route calls. Worked jq queries in `references/examples.md`.

## Reference files

- `references/examples.md` — full before/after for every refactoring (rename, route change, parameter add/remove), gopls command lines, embed-directive updates, jq queries for coupling analysis.

## External reference

- [Call Parameters](../../reference/call-parameters.md), [Call Results](../../reference/call-results.md), [Template Name Syntax](../../reference/template-names.md)
- [Debug Generation Errors](../muxt_debug-generation-errors/SKILL.md) — when refactoring triggers errors.

### Test cases (`cmd/muxt/testdata/`)

| Feature | Test file |
|---------|-----------|
| Receiver in different package | `reference_receiver_with_different_package.txt` |
| Multiple generated route files | `reference_multiple_generated_routes.txt` |
| Multiple template files | `reference_multiple_template_files.txt` |
| Cleanup: orphaned files | `reference_cleanup_orphaned_files.txt` |
| Cleanup: routes-func change | `reference_cleanup_routes_func_change.txt`, `reference_cleanup_different_routes_func.txt` |
| Cleanup: switch single ↔ multi | `reference_cleanup_switch_to_multiple_files.txt`, `reference_cleanup_switch_to_single_file.txt` |
| Receiver with pointer | `reference_receiver_with_pointer.txt` |
| Receiver with embedded method | `reference_receiver_with_embedded_method.txt` |
