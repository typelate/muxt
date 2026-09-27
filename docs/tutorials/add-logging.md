# Add Logging

Give generated handlers a `*slog.Logger`.

Prerequisites: a project where `muxt generate` runs cleanly. `server` below is its receiver.

## Step 1: Add the flag

Add `--output-routes-func-with-logger-param` to the `go:generate` directive and run `go generate ./...`. The routes function gains a parameter:

```go
func TemplateRoutes(mux *http.ServeMux, receiver RoutesReceiver, logger *slog.Logger) TemplateRoutePaths
```

## Step 2: Pass a logger

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
TemplateRoutes(mux, server, logger)
```

Each request now logs at debug level before the template runs:

```json
{"time":"...","level":"DEBUG","msg":"handling request","pattern":"GET /","path":"/","method":"GET"}
```

The logger's other entry, `ERROR` on a failed render, is listed under [Routes function](../reference/commands/generate.md#routes-function).

## Step 3: Test it

Write the logger to a buffer and decode the entry:

```go
func TestRequestLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mux := http.NewServeMux()
	TemplateRoutes(mux, server, logger)
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	var entry struct {
		Msg, Pattern string
	}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Msg != "handling request" || entry.Pattern != "GET /" {
		t.Errorf("got %+v", entry)
	}
}
```

The full suite is in [reference_structured_logging.txt](../../cmd/muxt/testdata/reference_structured_logging.txt).
