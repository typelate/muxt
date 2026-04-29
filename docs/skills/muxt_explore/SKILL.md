---
name: muxt-explore
description: "Muxt: Use when exploring a Muxt codebase. Pick the entry point that matches your starting point — a URL/route, a receiver method, an error message/log line, or a fresh repo. Each entry traces through the template-method-route chain."
---

# Explore a Muxt Codebase

Muxt's template-method-route coupling chain has three anchors: the **template** (defines the route + call expression), the **receiver method** (provides the data), and the **route** (the URL pattern). Pick the entry point matching what you have:

| You have | Section |
|---|---|
| A URL path or route pattern | [From a route](#from-a-route) |
| A receiver method name | [From a method](#from-a-method) |
| An error message or log line | [From an error](#from-an-error) |
| Nothing — fresh codebase | [Repo overview](#repo-overview) |

## From a route

Start from a URL path / route pattern; trace through template → method.

1. **Find the template.** `muxt list-template-calls --match "<regex>"` — regex is matched against the full template name `[METHOD ][HOST]/PATH[ STATUS][ CALL]`. Examples:
   ```bash
   muxt list-template-calls --match "/users"          # by path
   muxt list-template-calls --match "^GET /users"     # method + path
   muxt list-template-calls --match "GetUser"         # by call expression
   muxt list-template-calls --match "/users/\\{id\\}" # path-param pattern
   ```
   `muxt list-template-callers --match ...` shows which other templates and Go `ExecuteTemplate` calls reference this one.

2. **Read the template.** Open the matched `.gohtml`. Note: the call expression in the template name, `{{template "name" .}}` or `{{block ...}}` sub-template calls, how `.Result` / `.Err` / fields are rendered.

3. **Trace the receiver method.** gopls workspace symbol search for the method named in the call expression. Go to Definition for implementation; read signature for parameter parsing and return types.

4. **Trace types used in the template.** Go to Definition on the return type (fields = `.Result.FieldName`), on form structs for POST routes, and Package API for imported types.

## From a method

Start from a receiver method name; find templates and routes that use it.

1. **Find the method.** `go_search({"query": "GetUser"})` then Go to Definition for implementation, signature, return types.
2. **Find references.** `go_symbol_references({"file": "/path/to/receiver.go", "symbol": "Server.GetUser"})` — shows the generated handler that calls it.
3. **Find the route template.** `muxt list-template-callers --match "GetUser"` matches the regex against the full template name. The output shows the template containing the call, templates that call it, and Go `ExecuteTemplate` invocations.
4. **Read the template** to see how the return value renders — which fields, how errors are handled, sub-templates invoked.

## From an error

Start from an error message or log line; find the receiver method and template.

1. **Find the error source.** Grep for the error string:
   ```bash
   grep -rn "user not found" --include='*.go' .
   ```
2. **Find the generated handler.** `go_symbol_references` on the method shows the generated handler that calls it. Read it to understand how errors flow through `TemplateData[R, T]`.
3. **Find the route template.** `muxt list-template-callers --match "<MethodName>"`.
4. **Check error rendering** in the template — `{{with .Err}}` block? Does it display the error message? Missing handling can swallow errors silently.
5. **Enable handler logging** if errors aren't surfacing — see `references/structured-logging.md` for slog setup.

## Repo overview

You're new to the codebase. Map all routes, templates, and the receiver type.

1. **Discover all muxt packages.** `muxt explore-module` (or `--format=json` for structured data). Shows package config (routes function, receiver interface, receiver type) and exact drill-in commands.

2. **Drill into specific packages** using the commands from explore-module output:
   ```bash
   muxt explore-module --format=json | jq -r '.packages[].commands.calls' | sh
   muxt explore-module --format=json | jq -r '.packages[] | select(.config.htmxHelpers) | .path'
   ```

3. **Watch context size.** `muxt list-template-calls` and `list-template-callers` can produce huge output. Check first:
   ```bash
   muxt list-template-calls | wc -l
   ```
   If large, use `--match` to focus, or page (`| head -50`).

4. **Trace navigation.** Run `bash ${CLAUDE_SKILL_DIR}/scripts/scan-navigation.sh`. Look for hardcoded paths (`<a href="/users">`, fragile) vs type-checked paths (`<a href="{{$.Path.ListUsers}}">`, safe).

5. **Understand the receiver type.** `go doc` it; gopls workspace symbol search; Go to Definition for fields and dependencies.

6. **(Optional) Explore interactively.** Generate an httptest exploration server with counterfeiter-faked receiver: see `references/interactive-exploration.md` for the full workflow (`muxt generate-fake-server`, fake return setup, Chrome DevTools MCP).

## Reference files

- `references/structured-logging.md` — `--output-routes-func-with-logger-param` setup and slog level conventions for surfacing handler errors.
- `references/interactive-exploration.md` — `muxt generate-fake-server`, configuring fake return values, browsing with Chrome DevTools MCP.

## External reference

- [Template Name Syntax](../../reference/template-names.md)
- [Call Parameters](../../reference/call-parameters.md)
- [Call Results](../../reference/call-results.md)
- [Templates Variable](../../reference/templates-variable.md)
- [CLI Commands](../../reference/cli.md)

### Test Cases (`cmd/muxt/testdata/`)

- `reference_list_template_calls.txt`, `howto_list_template_calls.txt` — `list-template-calls` output and `--match`
- `reference_list_template_callers.txt`, `howto_list_template_callers.txt` — `list-template-callers` output and `--match`
- `reference_package_discovery.txt`, `reference_template_embed_gen_decl.txt`, `reference_template_glob.txt`, `reference_template_with_multiple_embeds.txt`, `reference_multiple_generated_routes.txt` — package and template discovery
- `reference_structured_logging.txt`, `reference_cli_logger_flag.txt` — logger setup
