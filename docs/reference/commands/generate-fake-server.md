# muxt generate-fake-server

Generate a fake server for interactively exploring routes. Creates an httptest server with a counterfeiter fake implementation of the receiver interface.

**WARNING:** The generated fake interface is unstable and should not be relied upon. This command is intended for exploratory use only.

```bash
muxt generate-fake-server path/to/package
```

## Output

Generates two files:

- **`<output>/main.go`** — Entry point that creates the fake receiver, wires routes, and starts an httptest server
- **`<output>/internal/fake/receiver.go`** — Counterfeiter-generated fake struct with `*Returns()` and `*CallCount()` methods

Default output directory: `./cmd/explore-goland/`

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--output`, `-o` | string | `./cmd/explore-goland` | Output directory for the generated files. |

## Arguments

Positional arguments are target package directories (relative or absolute). Multiple packages are supported. If no arguments are given, uses the working directory.

```bash
# Single package
muxt generate-fake-server hypertext

# Multiple packages
muxt generate-fake-server hypertext admin

# Working directory
muxt generate-fake-server

# Custom output
muxt generate-fake-server hypertext --output ./cmd/my-explorer
```

## Usage

### 1. Generate

```bash
muxt generate-fake-server path/to/package
```

### 2. Set Up Fake Data

Edit `main.go` to configure return values on the fake receiver:

```go
receiver := new(fake.RoutesReceiver)

receiver.ListArticlesReturns([]hypertext.Article{
    {ID: 1, Title: "First Post", Body: "Hello world"},
    {ID: 2, Title: "Second Post", Body: "Another article"},
}, nil)
```

### 3. Run

```bash
go run ./cmd/explore-goland/
```

The server prints `Explore at: http://127.0.0.1:<port>` and waits for Ctrl-C.

### 4. Browse

Use Chrome DevTools MCP to explore the rendered pages:

```
navigate_page({"url": "http://127.0.0.1:<port>/"})
take_snapshot({})
take_screenshot({})
```

## Requirements

- The target package must be a **library** (not `package main`)
- The target package must have a muxt-generated file (created by `muxt generate`)

## Related

- [muxt generate](generate.md) — Generate handlers from templates
- [muxt explore-module](explore-module.md) — Discover all muxt packages in the module
- [Explore](../../skills/muxt_explore/SKILL.md) — Full exploration workflow (pick an entry point: route, method, error, or fresh repo)
