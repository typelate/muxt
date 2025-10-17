# How to Add Logging to Generated Handlers

Learn how to add structured logging to your Muxt-generated HTTP handlers using the `--output-routes-func-with-logger-param` flag.

## Goal

Add observability to your handlers by logging request details and errors using Go's `log/slog` package.

## Prerequisites

- A Muxt project with templates defined
- Go 1.21+ (for `log/slog` support)

## Steps

### 1. Generate with the Logger Flag

Add `--output-routes-func-with-logger-param` to your `muxt generate` command:

```bash
muxt generate --use-receiver-type=App --output-routes-func-with-logger-param
```

Or in your `go:generate` directive:

```go
//go:generate muxt generate --use-receiver-type=App --output-routes-func-with-logger-param
```

> **Note:** The `--logger` flag is deprecated. Use `--output-routes-func-with-logger-param` instead.

### 2. Update Your Routes Function Call

The generated `TemplateRoutes` function now requires a `*slog.Logger` parameter:

```go
package main

import (
    "log/slog"
    "net/http"
    "os"
)

func main() {
    // Create a logger
    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelDebug,
    }))

    mux := http.NewServeMux()
    receiver := &App{}

    // Pass the logger to TemplateRoutes
    TemplateRoutes(mux, receiver, logger)

    http.ListenAndServe(":8080", mux)
}
```

### 3. Configure Log Levels

The generated handlers log at two levels:

**Debug Level** - Request handling:
```go
logger.DebugContext(request.Context(), "handling request",
    slog.String("pattern", "GET /users/{id}"),
    slog.String("path", request.URL.Path),
    slog.String("method", request.Method))
```

**Error Level** - Template execution failures:
```go
logger.ErrorContext(request.Context(), "failed to render page",
    slog.String("pattern", "GET /users/{id}"),
    slog.String("path", request.URL.Path),
    slog.String("error", err.Error()))
```

Set your handler's log level based on your needs:

```go
// Development: see all requests
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
}))

// Production: only errors
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelError,
}))
```

## Examples

### Development Logging

```go
package main

import (
    "log/slog"
    "net/http"
    "os"
)

func main() {
    // Human-readable text output for development
    logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelDebug,
        AddSource: true,
    }))

    mux := http.NewServeMux()
    TemplateRoutes(mux, &App{}, logger)

    if err := http.ListenAndServe(":8080", mux); err != nil {
		slog.Error("server failed", slog.String("error", err.Error()))
	}
}
```

Output:
```
time=2025-10-02T15:23:48.123Z level=DEBUG msg="handling request" pattern="GET /users/{id}" path=/users/123 method=GET
```

### Production Logging

```go
package main

import (
    "log/slog"
    "net/http"
    "os"
)

func main() {
    // JSON output for production (works with log aggregators)
    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelError,
    }))

    mux := http.NewServeMux()
    TemplateRoutes(mux, &App{}, logger)

    http.ListenAndServe(":8080", mux)
}
```

Output (only on errors):
```json
{"time":"2025-10-02T15:23:48.123Z","level":"ERROR","msg":"failed to render page","pattern":"GET /users/{id}","path":"/users/123","error":"template: users.gohtml: executing \"GET /users/{id}\" at <.User.Name>: can't evaluate field Name in type *User"}
```

### Custom Logger with Attributes

```go
package main

import (
    "log/slog"
    "net/http"
    "os"
)

func main() {
    // Add service-level attributes
    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With(
        slog.String("service", "my-app"),
        slog.String("version", "1.0.0"),
        slog.String("environment", "production"),
    )

    mux := http.NewServeMux()
    TemplateRoutes(mux, &App{}, logger)

    http.ListenAndServe(":8080", mux)
}
```

## Without the Logger Flag

If you don't use `--output-routes-func-with-logger-param`, the generated code uses the global `slog` default logger:

```go
// Generated without --output-routes-func-with-logger-param
func TemplateRoutes(mux *http.ServeMux, receiver RoutesReceiver) TemplateRoutePaths {
    // ...
    if err := templates.ExecuteTemplate(buf, "GET /", &td); err != nil {
        slog.ErrorContext(request.Context(), "failed to render page", ...)
        // ...
    }
}
```

This still logs errors, but:
- No debug logging for requests
- Uses the global logger (can't configure per-handler)
- No `logger` parameter in function signature

## Troubleshooting

### "logger" Undefined Error

```
undefined: logger
```

**Solution**: Pass a `*slog.Logger` to `TemplateRoutes`:

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
TemplateRoutes(mux, receiver, logger)
```

### No Debug Logs Appearing

**Cause**: Log level is too high (e.g., `LevelError`)

**Solution**: Set level to `LevelDebug`:

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
}))
```

### Too Many Logs in Production

**Cause**: Debug level is enabled

**Solution**: Use `LevelError` or `LevelWarn` in production:

```go
level := slog.LevelError
if os.Getenv("ENV") == "development" {
    level = slog.LevelDebug
}

logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: level,
}))
```

## Next Steps

- **[Test Your Handlers](test-handlers.md)** - Write tests for your logged handlers
- **[Write Receiver Methods](write-receiver-methods.md)** - Add logging inside your receiver methods
- **[CLI Reference](../reference/cli.md)** - See all available flags

## Related

- [Go `log/slog` documentation](https://pkg.go.dev/log/slog)
- [Structured logging best practices](https://go.dev/blog/slog)
