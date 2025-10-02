# How to Integrate Muxt into an Existing Project

Add Muxt to your existing Go web app without breaking everything.

## Goal

Get Muxt working alongside your current routes. No big rewrite. No disruption.

## Prerequisites

- A Go project that's already running
- You're using `html/template` (or want to start)

## Option 1: Add Type Checking to Existing Templates

If you already use `html/template` and want to add static analysis without changing your architecture:

### Step 1: Make Templates Discoverable

Ensure your `*template.Template` variable is initialized as a global declaration using embedded files (maybe in a new test file):

```go
package server

import (
	"embed"
	"html/template"
)

//go:embed *.gohtml
var templateSource embed.FS

var templates = template.Must(template.ParseFS(templateSource, "*.gohtml"))
```

### Step 2: Use String Literals in ExecuteTemplate Calls

Update all `templates.ExecuteTemplate` calls to use:
- String literals for the template name
- Static types for the data argument (avoid `any` or `interface{}`)

```go
// Good
var data database.UserRow
templates.ExecuteTemplate(w, "user-profile", data)

// Avoid - Muxt can't analyze these
var data any
templates.ExecuteTemplate(w, templateName, data)
```

### Step 3: Run Type Checking

```bash
go install github.com/typelate/muxt@latest
muxt check
```

This validates your templates without generating any code.

## Option 2: Generate Routes in a Separate Package

For generating HTTP handlers alongside existing routes, create an isolated package for Muxt-managed templates.

### Step 1: Create a Hypertext Package

```bash
mkdir -p internal/hypertext
```

**Why a separate package?**
- Clear separation of concerns
- Keeps generated code isolated
- Easier to test and maintain

### Step 2: Set Up Template Parsing

Create `internal/hypertext/templates.go`:

```go
package hypertext

import (
	"embed"
	"html/template"
)

//go:embed *.gohtml
var templatesDir embed.FS

//go:generate muxt generate --receiver-type=Server --receiver-type-package=example.com/internal/domain --routes-func=Routes
var templates = template.Must(template.ParseFS(templatesDir, "*"))
```

*[(See Muxt CLI Test/receiver_and_routes_are_in_different_packages)](../../cmd/muxt/testdata/receiver_and_routes_are_in_different_packages.txt)*

### Step 3: Add Your Templates

Create `internal/hypertext/templates/index.gohtml`:

```gotemplate
{{define "GET /dashboard Dashboard(ctx)" -}}
<!DOCTYPE html>
<html>
<head><title>Dashboard</title></head>
<body>
  <h1>Welcome, {{.Username}}</h1>
</body>
</html>
{{- end}}
```

### Step 4: Generate Routes

```bash
cd internal/hypertext
go generate
```

This creates `template_routes.go` with a `Routes` function.

### Step 5: Register Routes in Main

Update your `main.go` to register both old and new routes:

```go
package main

import (
	"log"
	"net/http"

	"example.com/internal/api"      // Your existing routes
	"example.com/internal/hypertext" // New Muxt routes
	"example.com/internal/domain"
)

func main() {
	mux := http.NewServeMux()

	srv := domain.New()

	// Existing routes
	api.RegisterRoutes(mux, srv)

	// Muxt-generated routes
	hypertext.Routes(mux, srv)

	log.Fatal(http.ListenAndServe(":8080", mux))
}
```

## Verify Integration

Run `muxt check` to ensure templates are correctly typed:

```bash
muxt check
```

If you see errors, check:
- Template names match the expected pattern
- Method signatures match template parameters
- The `--receiver-type` flag points to the correct type

## Common Issues

**`muxt check` can't find templates**

Your templates variable needs to be package-level and use `ParseFS` with an `embed.FS`. Muxt looks for this pattern.

**Route conflicts**

`http.ServeMux` panics on duplicate routes. If you get a panic at startup, you've registered the same path twice. Pick different paths or consolidate.

**Generated interface doesn't match**

Check your `--receiver-type` flag. It should point to the actual type that has your methods. If you're getting weird errors, this is usually why.

## Next Steps

- [Write receiver methods](write-receiver-methods.md) for your generated handlers
- [Test your handlers](test-handlers.md) using `domtest`
- Review [template name syntax](../reference/template-names.md) for advanced routing patterns
