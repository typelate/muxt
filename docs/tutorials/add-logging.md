# Add Logging

Give generated handlers a `*slog.Logger` and test the entries they write.

Prerequisite: the project from the [Quick Start](quick-start.md), through step 4.

## Step 1: Add the flag

In `main.go`, change the directive to:

```go
//go:generate go tool muxt generate --output-routes-func-with-logger-param
```

Run `go generate`. The routes function gains a parameter:

```go
func TemplateRoutes(mux *http.ServeMux, receiver RoutesReceiver, logger *slog.Logger) TemplateRoutePaths
```

## Step 2: Pass a logger

Add `log/slog` and `os` to the imports of `main.go`. In `main`, replace the `TemplateRoutes(mux, nil)` line with:

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
TemplateRoutes(mux, nil, logger)
```

Run `go run .` and open <http://localhost:8080>. Each request logs at debug level before the template runs:

```json
{"time":"...","level":"DEBUG","msg":"handling request","pattern":"GET /","path":"/","method":"GET"}
```

Stop the server with Ctrl-C. The logger's other entry, `ERROR` on a failed render, is listed under [Routes function](../reference/commands/generate.md#routes-function).

## Step 3: Test it

Create `logging_test.go`. It writes the logger to a buffer and decodes the entry:

```go
package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mux := http.NewServeMux()
	TemplateRoutes(mux, nil, logger)
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

```bash
go test ./...
```

The test passes. A fuller suite, with a receiver and error entries, is [reference_structured_logging.txt](../../cmd/muxt/testdata/reference_structured_logging.txt).
