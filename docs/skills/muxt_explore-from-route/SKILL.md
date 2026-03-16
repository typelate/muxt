---
name: muxt-explore-from-route
description: "Muxt: Use when exploring a Muxt codebase starting from a URL path or route pattern. Traces from route to template to receiver method."
---

# Explore from Route

Start from a URL path or route pattern and trace through the template to the receiver method.

## Step 1: Find the Route Template

Use `muxt list-template-calls` with `--match` to find the route template. The `--match` flag takes a regular expression matched against the full template name.

Template names follow this pattern:

```
[METHOD ][HOST]/PATH[ STATUS][ CALL]
```

Examples: `GET /users/{id} GetUser(ctx, id)`, `POST /article 201 CreateArticle(ctx, form)`, `/help`

Craft your `--match` regex to target any part of the name:

```bash
# Match by path
muxt list-template-calls --match "/users"

# Match by method and path
muxt list-template-calls --match "^GET /users"

# Match by call expression
muxt list-template-calls --match "GetUser"

# Match a specific path parameter pattern
muxt list-template-calls --match "/users/\\{id\\}"
```

This shows the template name and any sub-templates it calls.

Use `muxt list-template-callers` with the same `--match` flag to see which other templates (and Go `ExecuteTemplate` calls) reference this one:

```bash
muxt list-template-callers --match "^GET /users"
```

## Step 2: Read the Template

Open the `.gohtml` file containing the matched template. Look for:

- The **call expression** in the template name (e.g., `GetUser(ctx, id)`)
- **Sub-template calls** via `{{template "name" .}}` or `{{block "name" .}}`
- How the **return value** is rendered (`.Result`, `.Err`, fields)

## Step 3: Trace the Receiver Method

Use gopls to navigate to the method named in the call expression:

1. **Workspace symbol search** for the method name (e.g., `GetUser`)
2. **Go to Definition** on the method to read its implementation
3. Read the method signature to understand parameter parsing and return types

## Step 4: Trace Types Used in the Template

Use gopls to inspect types referenced in the template:

1. **Go to Definition** on the return type to see its fields (these are what `.Result.FieldName` accesses)
2. **Go to Definition** on form struct types if the template handles `POST` with form binding
3. **Package API** for any imported types used as parameters or return values

## Reference

- [Template Name Syntax](../../reference/template-names.md)
- [Call Parameters](../../reference/call-parameters.md)
- [Call Results](../../reference/call-results.md)
- [CLI Commands](../../reference/cli.md)

### Test Cases (`cmd/muxt/testdata/`)

- `reference_list_template_calls.txt` — `muxt list-template-calls` output format
- `howto_list_template_calls.txt` — Using `--match` flag to filter template calls
