# Add Logging to Muxt Handlers

Add structured logging to your generated HTTP handlers using Go's `log/slog`.

## Prerequisites

- A working Muxt project with `muxt generate`

## Step 1: Enable the Logger Parameter

Add `--output-routes-func-with-logger-param` to your `go:generate` directive:

```go
//go:generate muxt generate --use-receiver-type=Server --output-routes-func-with-logger-param
```

Run `go generate ./...` to regenerate. The generated `TemplateRoutes` function now accepts a `*slog.Logger` parameter:

```go
func TemplateRoutes(mux *http.ServeMux, receiver RoutesReceiver, logger *slog.Logger) TemplateRoutePaths
```

## Step 2: Create a Logger and Pass It

### Development

Use text output with debug level to see all request logs:

```go
logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
}))
TemplateRoutes(mux, server, logger)
```

Output for each request:

```
time=2026-03-11T10:30:00.000-07:00 level=DEBUG msg="handling request" pattern="GET /" path=/ method=GET
```

### Production

Use JSON output with environment-based level:

```go
level := slog.LevelError
if os.Getenv("ENV") == "development" {
    level = slog.LevelDebug
}
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: level,
}))
TemplateRoutes(mux, server, logger)
```

## What Gets Logged

The generated handlers produce two kinds of log entries:

### DEBUG: Every Request

Logged before template execution. Fields: `pattern`, `path`, `method`.

```json
{
  "level": "DEBUG",
  "msg": "handling request",
  "pattern": "GET /",
  "path": "/",
  "method": "GET",
  "time": "2026-03-11T10:30:00.000-07:00"
}
```

At `slog.LevelInfo` or higher, these are filtered out — only errors appear.

### ERROR: Template Execution Failure

Logged when `templates.ExecuteTemplate` returns an error. Fields: `pattern`, `path`, `error`.

```json
{
  "level": "ERROR",
  "msg": "failed to render page",
  "pattern": "GET /",
  "path": "/",
  "error": "template: ...: can't evaluate field Foo in type *main.TemplateData[...]",
  "time": "2026-03-11T10:30:00.000-07:00"
}
```

The handler also returns HTTP 500 with the body `failed to render page`.

### What Is Not Logged

Receiver method errors are **not** logged. They are stored in `TemplateData` and rendered by the template's `{{if .Err}}` block. This is by design: templates handle application errors, the logger handles infrastructure errors (broken templates).

## Without the Logger Parameter

Even without `--output-routes-func-with-logger-param`, generated handlers call `slog.ErrorContext` on the **default logger** when template execution fails:

```go
slog.ErrorContext(request.Context(), "failed to render page",
    slog.String("path", request.URL.Path),
    slog.String("pattern", request.Pattern),
    slog.String("error", err.Error()))
```

The flag adds:
- An explicit `*slog.Logger` parameter (instead of the global default)
- Debug-level request logging for every request

## Step 3: Test Logging Output

Parse log output as JSON in tests to assert on structured fields:

```go
func TestRequestLogging(t *testing.T) {
    var logBuf bytes.Buffer
    logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{
        Level: slog.LevelDebug,
    }))

    mux := http.NewServeMux()
    TemplateRoutes(mux, server, logger)

    rec := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/", nil)
    mux.ServeHTTP(rec, req)

    var entry struct {
        Level   string `json:"level"`
        Msg     string `json:"msg"`
        Pattern string `json:"pattern"`
        Path    string `json:"path"`
        Method  string `json:"method"`
    }
    if err := json.Unmarshal(logBuf.Bytes(), &entry); err != nil {
        t.Fatalf("failed to parse log: %v", err)
    }

    if entry.Level != "DEBUG" {
        t.Errorf("got level %q, want DEBUG", entry.Level)
    }
    if entry.Msg != "handling request" {
        t.Errorf("got msg %q, want \"handling request\"", entry.Msg)
    }
    if entry.Pattern != "GET /" {
        t.Errorf("got pattern %q, want \"GET /\"", entry.Pattern)
    }
}
```

To test that errors at INFO level produce no debug output:

```go
func TestNoDebugLogsAtInfoLevel(t *testing.T) {
    var logBuf bytes.Buffer
    logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))

    mux := http.NewServeMux()
    TemplateRoutes(mux, server, logger)

    rec := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/", nil)
    mux.ServeHTTP(rec, req)

    if logBuf.Len() > 0 {
        t.Errorf("expected no logs at INFO level, got: %s", logBuf.String())
    }
}
```

See `cmd/muxt/testdata/reference_structured_logging.txt` for the full test suite.

## Related

- [`muxt generate` flags](../reference/commands/generate.md) - All output flags including logger
- [Go slog documentation](https://pkg.go.dev/log/slog) - Standard library reference
- [Structured Logging with slog](https://go.dev/blog/slog) - Go blog post
