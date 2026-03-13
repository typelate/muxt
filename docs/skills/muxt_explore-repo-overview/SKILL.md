---
name: muxt-explore-repo-overview
description: "Muxt: Use when new to a Muxt codebase and need the big picture. Maps all routes, templates, and the receiver type."
---

# Explore Repo Overview

Get the big picture of an existing Muxt codebase by mapping all routes, templates, and the receiver type.

## Step 1: Discover All Muxt Packages

```bash
muxt explore-module
```

Or for structured data:

```bash
muxt explore-module --format=json
```

This discovers all packages with muxt-generated files, shows their configuration (routes function, receiver interface, receiver type), and provides the exact commands to drill into each package.

## Step 2: Drill Into Specific Packages

Use the commands from explore-module output to list routes and template relationships:

```bash
muxt explore-module --format=json | jq -r '.packages[].commands.calls' | sh
```

Find HTMX-enabled packages:

```bash
muxt explore-module --format=json | jq -r '.packages[] | select(.config.htmxHelpers) | .path'
```

List external CDN assets:

```bash
muxt explore-module --format=json | jq '.packages[].externalAssets[]'
```

**Warning:** `muxt list-template-calls` and `muxt list-template-callers` can produce very large output in bigger codebases and may clog up the context window. Before running them unfiltered, check the size:

```bash
muxt list-template-calls | wc -l
muxt list-template-callers | wc -l
```

If the output is large, use `--match` to focus on a specific area of interest (see the [explore-from-route](../explore-from-route/SKILL.md) and [explore-from-method](../explore-from-method/SKILL.md) skills). Or page through it:

```bash
muxt list-template-calls | head -50
muxt list-template-callers | head -50
```

Only run the full unfiltered output when the line count is manageable.

## Step 3: Trace Navigation Links in Templates

Scan templates for navigation links:

```bash
bash ${CLAUDE_SKILL_DIR}/scripts/scan-navigation.sh
```

Look for two patterns:

- **Hardcoded paths** like `<a href="/users">` — these reference routes by string and can break silently if routes change
- **Type-checked paths** like `<a href="{{$.Path.ListUsers}}">` — these use the generated `TemplateRoutePaths` helper, so the compiler catches broken links

The `.Path` method is available on `TemplateData` in every route template. Each route with a call expression gets a corresponding method on `TemplateRoutePaths`:

```gotemplate
{{define "GET /{$} Home(ctx)"}}
  <a href="{{$.Path.ListUsers}}">Users</a>
  <a href="{{$.Path.GetUser 42}}">User 42</a>
{{end}}
```

When exploring, note which links use `.Path` (safe) vs hardcoded strings (fragile). This is a good signal for code quality.

## Step 4: Understand the Receiver Type

Use `go doc` or the explore-module output to find the receiver type:

```bash
muxt explore-module --format=json | jq -r '.packages[0] | "go doc \(.path) \(.config.receiverType)"' | sh
```

For deeper exploration, use gopls:

1. **Workspace symbol search** for the receiver type name (e.g., `Server`)
2. **Go to Definition** to read its struct fields and dependencies
3. **Package API** to see all its methods — each method corresponds to a route's call expression

## Step 5: Explore Interactively (Optional)

Generate an httptest exploration server with a counterfeiter fake:

```bash
muxt generate-fake-server path/to/package
```

This generates files in `./cmd/explore-goland/`:

- **`main.go`** — readable entry point: creates the fake receiver, wires routes, starts httptest server
- **`internal/fake/receiver.go`** — counterfeiter-generated fake struct with `*Returns()` and `*CallCount()` methods

Use `--output` to change the output directory:

```bash
muxt generate-fake-server path/to/package --output ./cmd/my-explorer
```

### Set Up Fake Return Values

Edit `main.go` to set up return values on the fake receiver before the server starts. The fake has `*Returns(...)` methods for each interface method:

```go
receiver := new(fake.RoutesReceiver)

// Set up fake data so routes render with realistic content
receiver.ListArticlesReturns([]hypertext.Article{
    {ID: 1, Title: "First Post", Body: "Hello world"},
    {ID: 2, Title: "Second Post", Body: "Another article"},
}, nil)
receiver.GetArticleReturns(hypertext.Article{
    ID: 1, Title: "First Post", Body: "Hello world",
}, nil)
```

This lets you see what the templates render with specific data. Modify the return values and re-run `go run ./cmd/explore-goland/` to test different states (empty lists, error conditions, edge cases).

### Run the Server

```bash
go run ./cmd/explore-goland/
```

The server prints `Explore at: http://127.0.0.1:<port>` and waits for Ctrl-C.

### Browse with Chrome DevTools MCP

Use Chrome DevTools MCP tools to explore the frontend:

```
navigate_page({"url": "http://127.0.0.1:<port>/"})
take_snapshot({})
take_screenshot({})
```

Click through links, inspect the DOM, and take screenshots to understand the page structure. For HTMX-enabled pages, interact with elements and observe the network requests:

```
click({"uid": "<element-uid>"})
take_snapshot({})
list_network_requests({})
```

### Explore the Full Frontend Stack

Chrome DevTools MCP operates a real browser, so HTMX swaps, JavaScript, and CSS all work. Use it to verify the full frontend stack:

1. **Navigate** to a page and take a snapshot to see the rendered DOM
2. **Click** interactive elements (buttons, links, HTMX triggers) and snapshot again to see what changed
3. **Check network requests** to see HTMX fragment fetches and form submissions
4. **Take screenshots** to see the visual result with CSS applied

For HTMX pages, this is the fastest way to verify that `hx-get`, `hx-post`, and swap targets work correctly with the fake data you configured in `main.go`.

## Reference

- [Template Name Syntax](../../reference/template-names.md)
- [Templates Variable](../../reference/templates-variable.md)
- [CLI Commands](../../reference/cli.md)

### Test Cases (`cmd/muxt/testdata/`)

- `reference_package_discovery.txt` — How muxt discovers the package and templates variable
- `reference_template_embed_gen_decl.txt` — `//go:embed` with `var` declaration
- `reference_template_glob.txt` — Template glob patterns
- `reference_template_with_multiple_embeds.txt` — Multiple `//go:embed` directives
- `reference_list_template_calls.txt` — `muxt list-template-calls` output format
- `reference_list_template_callers.txt` — `muxt list-template-callers` output format
- `reference_multiple_generated_routes.txt` — Multiple templates variables in one package
