# Structured logging in muxt handlers

Muxt-generated handlers log via `log/slog`. Without any flags, they call `slog.ErrorContext` on the global logger when template execution fails.

To also see a debug-level log line for every incoming request, enable `--output-routes-func-with-logger-param`. This adds a `*slog.Logger` parameter to `TemplateRoutes` and generates both log levels:

- **Debug** — every request (pattern, path, method).
- **Error** — template execution failures (pattern, path, error message).

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

For production, use JSON output and an environment-based level:

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
