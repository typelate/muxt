---
name: muxt-explore-from-error
description: "Muxt: Use when tracing an error message or log line back through a Muxt codebase to find the handler and template that produced it."
---

# Explore from Error

Start from an error message or log line and trace back to the receiver method, handler, and template.

## Step 1: Find the Error Source

Grep the codebase for the error string to locate the receiver method that produces it:

```bash
grep -rn "user not found" --include='*.go' .
```

This identifies the method and file where the error originates.

## Step 2: Find the Generated Handler

Use gopls Find References on the method to find the generated handler that calls it:

```
go_symbol_references({"file": "/path/to/receiver.go", "symbol": "Server.GetUser"})
```

Read the generated handler to understand how errors flow through `TemplateData[R, T]`.

## Step 3: Find the Route Template

Use `muxt list-template-callers` with `--match` to find the route template. The `--match` flag takes a regular expression matched against the full template name (`[METHOD ][HOST]/PATH[ STATUS][ CALL]`):

```bash
muxt list-template-callers --match "GetUser"
```

## Step 4: Check Error Rendering

Read the template to see how `.Err` is rendered:

- Is there a `{{with .Err}}` block?
- Does the template display the error message?
- Are there missing error handling paths?

If the template lacks error handling, this explains why errors may be swallowed silently.

## Step 5: Enable Handler Logging

Muxt-generated handlers log via `log/slog`. Without any flags, they call `slog.ErrorContext` on the global logger when template execution fails.

To also see a debug-level log line for every incoming request, enable `--output-routes-func-with-logger-param`. This adds a `*slog.Logger` parameter to `TemplateRoutes` and generates both log levels:

- **Debug**: every request (pattern, path, method)
- **Error**: template execution failures (pattern, path, error message)

```go
//go:generate muxt generate --use-receiver-type=Server --output-routes-func-with-logger-param
```

Make sure the logger's level is set low enough to see debug output:

```go
// Development: text output, debug level
logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
}))
TemplateRoutes(mux, receiver, logger)
```

For production, use JSON output and environment-based level:

```go
level := slog.LevelError
if os.Getenv("ENV") == "development" {
    level = slog.LevelDebug
}
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: level,
}))
```

See Go's [log/slog documentation](https://pkg.go.dev/log/slog) and [structured logging blog post](https://go.dev/blog/slog).

## Reference

- [Call Results](../../reference/call-results.md)
- [Template Name Syntax](../../reference/template-names.md)
- [CLI Commands](../../reference/cli.md)

### Test Cases (`cmd/muxt/testdata/`)

- `reference_structured_logging.txt` — Structured logging with JSON parsing and slog field assertions
- `reference_cli_logger_flag.txt` — `--output-routes-func-with-logger-param` flag
