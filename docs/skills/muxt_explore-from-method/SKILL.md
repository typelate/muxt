---
name: muxt-explore-from-method
description: "Muxt: Use when exploring a Muxt codebase starting from a receiver method name. Traces from method to templates and routes that use it."
---

# Explore from Method

Start from a receiver method name and find which templates and routes use it.

## Step 1: Find the Method

Use gopls workspace symbol search for the method name:

```
go_search({"query": "GetUser"})
```

Go to Definition to read its implementation, signature, and return types.

## Step 2: Find References

Use gopls Find References to see where the method is called:

```
go_symbol_references({"file": "/path/to/receiver.go", "symbol": "Server.GetUser"})
```

This shows the generated handler code that calls the method, confirming it's wired up.

## Step 3: Find the Route Template

Use `muxt list-template-callers` with `--match` to find which route template(s) reference the method. The `--match` flag takes a regular expression matched against the full template name.

Template names follow this pattern:

```
[METHOD ][HOST]/PATH[ STATUS][ CALL]
```

Examples: `GET /users/{id} GetUser(ctx, id)`, `POST /article 201 CreateArticle(ctx, form)`, `/help`

Match by call expression to find templates that invoke the method:

```bash
muxt list-template-callers --match "GetUser"
```

This shows the template name containing the call expression, any templates that call it, and any Go `ExecuteTemplate` calls that invoke it.

## Step 4: Read the Template

Open the `.gohtml` file(s) to see how the method's return value is rendered:

- Which fields of the return type are accessed
- How errors are handled (`.Err`)
- What sub-templates are called with the data

## Reference

- [Template Name Syntax](../reference/template-names.md)
- [Call Parameters](../reference/call-parameters.md)
- [Call Results](../reference/call-results.md)
- [CLI Commands](../reference/cli.md)

### Test Cases (`cmd/muxt/testdata/`)

- `reference_list_template_callers.txt` — `muxt list-template-callers` output format
- `howto_list_template_callers.txt` — Using `--match` flag to filter template callers
