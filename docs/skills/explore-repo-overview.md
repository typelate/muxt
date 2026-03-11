---
name: muxt-explore-repo-overview
description: "Muxt: Use when new to a Muxt codebase and need the big picture. Maps all routes, templates, and the receiver type."
---

# Explore Repo Overview

Get the big picture of an existing Muxt codebase by mapping all routes, templates, and the receiver type.

## Step 1: Find Directories Where muxt generate Runs

Locate every `//go:generate` directive that invokes muxt:

```bash
grep -rn '//go:generate.*muxt' --include='*.go' .
```

Each match is a directory where muxt generates code. Note the flags:
- `--use-receiver-type=X` — the receiver type name
- `--use-templates-variable=Y` — the templates variable name (default: `templates`)
- `--output-routes-func=Z` — the generated routes function name (default: `TemplateRoutes`)

Read each file to find the templates variable and `//go:embed` directive showing which `.gohtml` files are included.

Use `go doc` to inspect the generated function and types in that package:

```bash
# Show the TemplateRoutes function signature and doc comment
go doc example.com/hypertext TemplateRoutes

# Show the generated receiver interface (lists all methods the templates expect)
go doc example.com/hypertext RoutesReceiver

# Show the generated path helper type and its methods
go doc example.com/hypertext TemplateRoutePaths
```

Replace `example.com/hypertext` with the actual package path from Step 1.

## Step 2: List Routes and Template Functions

From each directory found in Step 1, run `muxt` with no subcommand:

```bash
muxt
```

This lists all route templates and template functions in that directory. This is usually concise enough for an overview.

**Warning:** `muxt list-template-calls` and `muxt list-template-callers` can produce very large output in bigger codebases and may clog up the context window. Before running them unfiltered, check the size:

```bash
muxt list-template-calls | wc -l
muxt list-template-callers | wc -l
```

If the output is large, use `--match` to focus on a specific area of interest (see the [explore-from-route](explore-from-route.md) and [explore-from-method](explore-from-method.md) skills). Or page through it:

```bash
muxt list-template-calls | head -50
muxt list-template-callers | head -50
```

Only run the full unfiltered output when the line count is manageable.

## Step 3: Trace Navigation Links in Templates

Scan `.gohtml` files for HTML elements that link between routes:

```bash
grep -rn 'href=\|action=' --include='*.gohtml' .
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

Use `go doc` to see the receiver type's exported API:

```bash
# Show the type definition and its methods
go doc example.com/hypertext Server

# Show a specific method's signature and doc comment
go doc example.com/hypertext Server.GetArticle
```

For deeper exploration, use gopls:

1. **Workspace symbol search** for the receiver type name (e.g., `Server`)
2. **Go to Definition** to read its struct fields and dependencies
3. **Package API** to see all its methods — each method corresponds to a route's call expression

## Step 5: Explore Interactively (Optional)

Create `./cmd/explore-gohtml/` with a counterfeiter mock and httptest server for visual exploration:

### 4a: Generate a counterfeiter mock

```bash
cd ./cmd/explore-gohtml/ || exit 1
go install github.com/maxbrunsfeld/counterfeiter/v6 # only if not found by `which counterfeiter`
counterfeiter --fake-name RoutesReceiver -o ./internal/fake/routes_receiver.go example.com/internal/hypertext.RoutesReceiver
```

Run `go generate ./...` to create the fake.

### 4b: Wire an httptest server

Run the server in the background and send SIGINT to stop it cleanly.

```go
package main

import (
    "context"
    "fmt"
    "net/http"
    "net/http/httptest"
    "os/signal"
    "syscall"
	
    "example.com/internal/hypertext"
	"example.com/cmd/explore-gohtml/internal/fake"
)

func main() {
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    receiver := new(fake.RoutesReceiver)
    // Configure stub returns to produce representative HTML

    mux := http.NewServeMux()
    hypertext.TemplateRoutes(mux, receiver)

    server := httptest.NewServer(mux)
    defer server.Close()
    fmt.Println("Explore at:", server.URL)

    <-ctx.Done()
}
```

### 4c: Browse with Chrome DevTools MCP

Once the httptest server is running:

```
navigate_page({"url": "http://127.0.0.1:<port>/"})
take_snapshot({})
take_screenshot({})
```

## Reference

- [Template Name Syntax](../reference/template-names.md)
- [Templates Variable](../reference/templates-variable.md)
- [CLI Commands](../reference/cli.md)

### Test Cases (`cmd/muxt/testdata/`)

- `reference_package_discovery.txt` — How muxt discovers the package and templates variable
- `reference_template_embed_gen_decl.txt` — `//go:embed` with `var` declaration
- `reference_template_glob.txt` — Template glob patterns
- `reference_template_with_multiple_embeds.txt` — Multiple `//go:embed` directives
- `reference_list_template_calls.txt` — `muxt list-template-calls` output format
- `reference_list_template_callers.txt` — `muxt list-template-callers` output format
- `reference_multiple_generated_routes.txt` — Multiple templates variables in one package
