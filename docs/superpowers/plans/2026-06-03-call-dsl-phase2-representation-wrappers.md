# Phase 2 — Representation Wrappers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. **This is a muxt project — dispatched agents must use the muxt skills** (`muxt_test-driven-development`, `muxt_explore-from-route`, `muxt_explore-from-method`, `muxt_refactoring`).

**Goal:** Add the two response-side representation wrappers to the template-name call DSL — `sse(Method(...))` for Server-Sent Events (callback `send`/`sendX` or iterator/channel returns) and `marshalJSON(Method(...))` for `application/json` responses — replacing the dev-only `sse`/`sseX` argument family.

**Architecture:** Recognize `sse` / `marshalJSON` as reserved pseudo-functions at the **outermost** call position. Unwrap them at parse time (`parseHandler`) so `Definition.call`/`Definition.fun` already point at the inner method call and all existing arg-parsing/type-checking works unchanged; a `Definition.representation` enum records which wrapper. Dispatch on the enum in `methodHandlerFunc`. The SSE path reuses the existing streaming transport (`streamMethodHandlerFunc`, `sseClosure`, `SSETemplateData.WriteTo`, buffer pool, flusher, mutex); only the trigger (arg → wrapper), the callback names (`sse`/`sseX` → `send`/`sendX`, with `sendX` → template `X` verbatim) and the new iterator/channel return path are new. `marshalJSON` is a new file mirroring `datastar_signals.go`'s JSONV2 branching.

**Tech Stack:** Go `go/ast`, `go/types`, `go/parser`; muxt generator (`internal/muxt`, `internal/generate`, `internal/astgen`); txtar fixtures in `cmd/muxt/testdata/` run by `Test` in `cmd/muxt/main_test.go` (rsc.io/script); `encoding/json` + `encoding/json/v2`; `iter`.

---

## File Structure

- **`internal/muxt/definition.go`** — parser. Add the `Representation` enum + `Representation()` accessor + `representation` field; unwrap the outer `sse(...)`/`marshalJSON(...)` wrapper in `parseHandler`; add `IsSendArgument` and the `send` reserved identifier; gate `send`/`sendX` (and `marshalJSON(sendX)`) in `checkArguments` behind an `allowSend` flag. Later remove the obsolete `sse` arg family (`IsSSEArgument`, `TemplateNameScopeIdentifierSSE`, `UsesSSE`).
- **`internal/generate/routes.go`** — dispatch. Add the `Representation()` switch in `methodHandlerFunc`; add `sseWrapperHandlerFunc` (callback + return sub-modes); retarget away from `sseMethodHandlerFunc`/`sseArg`. Add `iter.Seq`/`iter.Seq2`/channel return detection + range codegen. Later delete `sseMethodHandlerFunc`, `sseArg`, `validateSSEMethodResults`.
- **`internal/generate/marshal_json.go`** (new) — the `marshalJSON(Method(...))` response handler, the `marshalJSON(sendX)` send-callback closure, and the shared `writeJSONResponse` package-level helper (JSONV2-aware), mirroring `internal/generate/datastar_signals.go`.
- **`cmd/muxt/testdata/`** — new `reference_sse_wrapper.txt`, `reference_sse_sendx.txt`, `reference_sse_iter_seq.txt`, `reference_sse_chan.txt`, `reference_sse_iter_seq2_error.txt`, `reference_sse_marshal_send.txt`, `reference_marshal_json.txt`, `reference_marshal_json_jsonv2.txt`, `err_sse_iter_and_send.txt`, `err_sse_unwrapped_iter.txt`, `err_marshal_json_no_value.txt`; migrate existing `reference_sse*.txt` / `err_sse*.txt`.
- **`docs/reference/call-parameters.md`**, **`docs/reference/datastar.md`** — document the wrappers.

**Test command reference** (run from repo root): a single fixture is `go test ./cmd/muxt -run 'Test/<basename-without-.txt>'`; the package is `go test ./cmd/muxt`; everything is `go test ./...`.

---

## Task 1: Parser — recognize and unwrap `sse()` / `marshalJSON()`, reserve `send`/`sendX`

**Files:**
- Modify: `internal/muxt/definition.go` (`parseHandler` ~339-374; `checkArguments` ~456-482; const block ~484-503; accessors near ~140)
- Test: `internal/muxt/definition_test.go`

- [ ] **Step 1: Write the failing unit test**

Add to `internal/muxt/definition_test.go` (package `muxt`, so `newDefinition` is reachable):

```go
func TestDefinitionRepresentation(t *testing.T) {
	for _, tt := range []struct {
		name string
		want Representation
		fun  string
	}{
		{name: "GET /a Plain(ctx)", want: RepresentationNone, fun: "Plain"},
		{name: "GET /b sse(Stream(ctx, send))", want: RepresentationSSE, fun: "Stream"},
		{name: "GET /c sse(Stream(ctx, send, sendClock))", want: RepresentationSSE, fun: "Stream"},
		{name: "GET /d marshalJSON(GetUser(ctx))", want: RepresentationMarshalJSON, fun: "GetUser"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			def, err, ok := newDefinition(template.New(tt.name))
			if !ok || err != nil {
				t.Fatalf("newDefinition(%q) ok=%v err=%v", tt.name, ok, err)
			}
			if got := def.Representation(); got != tt.want {
				t.Errorf("Representation() = %v, want %v", got, tt.want)
			}
			if def.FunctionIdentifier().Name != tt.fun {
				t.Errorf("FunctionIdentifier() = %q, want %q (wrapper must be unwrapped)", def.FunctionIdentifier().Name, tt.fun)
			}
		})
	}
}

func TestDefinitionSendOnlyInsideSSE(t *testing.T) {
	// bare send/sendX outside sse(...) is an unknown argument
	_, err, _ := newDefinition(template.New("GET /e Plain(ctx, send)"))
	if err == nil {
		t.Fatal("want error for send argument outside sse() wrapper")
	}
}
```

Ensure `definition_test.go` imports `"text/template"`.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/muxt -run 'TestDefinitionRepresentation|TestDefinitionSendOnlyInsideSSE'`
Expected: FAIL — `def.Representation undefined`, `RepresentationSSE undefined`.

- [ ] **Step 3: Add the enum, field, accessor, and `send` reserved name**

In `internal/muxt/definition.go`, add to the const block (~498):

```go
	// Phase 2 SSE send render-callback family, valid only inside sse(...).
	TemplateNameScopeIdentifierSend = "send"
```

Add reserved outer-wrapper names and the enum near the other consts:

```go
// Representation names the optional outermost wrapper of a handler call.
type Representation int

const (
	RepresentationNone Representation = iota
	RepresentationSSE
	RepresentationMarshalJSON
)

// Reserved outer representation-wrapper function names.
const (
	RepresentationWrapperSSE         = "sse"
	RepresentationWrapperMarshalJSON = "marshalJSON"
)

// IsSendArgument reports whether name is an SSE send render-callback argument:
// the reserved "send" identifier, or a camelCase "send"-prefixed name (sendClock,
// sendStatus, ...). A "send"-prefixed name renders the same-named template
// (sendClock -> "Clock").
func IsSendArgument(name string) bool {
	return isReservedOrPrefixed(name, TemplateNameScopeIdentifierSend)
}
```

Add the field to `Definition` (~107, after `templatesVariable`):

```go
	// representation records the outermost call wrapper (none, sse, marshalJSON).
	representation Representation
```

Add the accessor near `CallExpression` (~141):

```go
func (def Definition) Representation() Representation { return def.representation }
```

- [ ] **Step 4: Unwrap the outer wrapper in `parseHandler`**

In `parseHandler` (`internal/muxt/definition.go`), after `fun, ok := call.Fun.(*ast.Ident)` succeeds and the ellipsis check (~360), before the `scope`/`checkArguments` block, insert:

```go
	// Recognize an optional outermost representation wrapper: sse(Method(...)) or
	// marshalJSON(Method(...)). Unwrap it so the rest of the parser and the whole
	// generator operate on the inner method call unchanged.
	representation := RepresentationNone
	switch fun.Name {
	case RepresentationWrapperSSE:
		representation = RepresentationSSE
	case RepresentationWrapperMarshalJSON:
		representation = RepresentationMarshalJSON
	}
	if representation != RepresentationNone {
		if len(call.Args) != 1 {
			return fmt.Errorf("%s takes exactly one argument: the method call", fun.Name)
		}
		inner, ok := call.Args[0].(*ast.CallExpr)
		if !ok {
			return fmt.Errorf("%s argument must be a method call", fun.Name)
		}
		innerFun, ok := inner.Fun.(*ast.Ident)
		if !ok {
			return fmt.Errorf("expected function identifier, got: %s", astgen.Format(inner.Fun))
		}
		call, fun = inner, innerFun
	}
	def.representation = representation
```

Then change the `checkArguments` call to pass whether `send` is allowed:

```go
	scope := append(patternScope(), pathParameterNames...)
	slices.Sort(scope)
	if err := checkArguments(scope, call, representation == RepresentationSSE); err != nil {
		return err
	}
```

- [ ] **Step 5: Gate `send`/`sendX` in `checkArguments`**

Change the signature and the `*ast.Ident` case in `checkArguments` (~456):

```go
func checkArguments(identifiers []string, call *ast.CallExpr, allowSend bool) error {
	for i, a := range call.Args {
		switch exp := a.(type) {
		case *ast.Ident:
			known := false
			if _, ok := slices.BinarySearch(identifiers, exp.Name); ok {
				known = true
			}
			if IsSSEArgument(exp.Name) || IsDatastarArgument(exp.Name) {
				known = true
			}
			if allowSend && IsSendArgument(exp.Name) {
				known = true
			}
			if !known {
				return fmt.Errorf("unknown argument %s at index %d", exp.Name, i)
			}
		case *ast.CallExpr:
			if id, ok := exp.Fun.(*ast.Ident); ok && IsInputWrapper(id.Name) {
				if len(exp.Args) != 1 {
					return fmt.Errorf("%s takes exactly one argument", id.Name)
				}
				inner, ok := exp.Args[0].(*ast.Ident)
				if !ok || inner.Name != TemplateNameScopeIdentifierBody {
					return fmt.Errorf("%s argument must be %s", id.Name, TemplateNameScopeIdentifierBody)
				}
				continue
			}
			if err := checkArguments(identifiers, exp, allowSend); err != nil {
				return fmt.Errorf("call %s argument error: %w", astgen.Format(call.Fun), err)
			}
		default:
			return fmt.Errorf("expected only identifier or call expressions as arguments, argument at index %d is: %s", i, astgen.Format(a))
		}
	}
	return nil
}
```

(Other call sites of `checkArguments`, if any, pass `false`.)

- [ ] **Step 6: Run the unit tests to verify they pass**

Run: `go test ./internal/muxt -run 'TestDefinitionRepresentation|TestDefinitionSendOnlyInsideSSE'`
Expected: PASS.

Run: `go test ./...`
Expected: PASS (additive change; nothing dispatches on the new enum yet, the old `sse` arg path is untouched).

- [ ] **Step 7: Commit**

```bash
gofumpt -w internal/muxt/definition.go internal/muxt/definition_test.go
git add internal/muxt/definition.go internal/muxt/definition_test.go
git commit -m "feat(muxt): recognize sse()/marshalJSON() representation wrappers

Assisted-by: Claude:claude-opus-4-8 gofumpt"
```

---

## Task 2: `sse()` callback mode — `send` and `sendX` callbacks

**Files:**
- Modify: `internal/generate/routes.go` (`methodHandlerFunc` ~1126; add `sseWrapperHandlerFunc`)
- Test: `cmd/muxt/testdata/reference_sse_wrapper.txt`, `cmd/muxt/testdata/reference_sse_sendx.txt`

- [ ] **Step 1: Write the failing fixtures**

Create `cmd/muxt/testdata/reference_sse_wrapper.txt`:

```
muxt generate --use-receiver-type=Server
muxt check

exec go test -race -count=1

-- template.gohtml --
{{- define "GET /events sse(Stream(ctx, lastEventID, send))" -}}{{- .Result -}}{{- end -}}
-- go.mod --
module server

go 1.24
-- server.go --
package server

import (
	"context"
	"embed"
	"html/template"
)

//go:embed *.gohtml
var templatesFS embed.FS

var templates = template.Must(template.ParseFS(templatesFS, "*"))

type Server struct{}

// Stream echoes the Last-Event-Id once via the send callback and returns.
func (Server) Stream(ctx context.Context, lastEventID string, send func(data string) error) {
	_ = send("hello-" + lastEventID)
}
-- server_test.go --
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStream(t *testing.T) {
	mux := http.NewServeMux()
	TemplateRoutes(mux, Server{})
	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	req.Header.Set("Last-Event-Id", "42")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}
	if got := rec.Body.String(); !strings.Contains(got, "data: hello-42\n\n") {
		t.Fatalf("body = %q, want it to contain %q", got, "data: hello-42\n\n")
	}
}
```

Create `cmd/muxt/testdata/reference_sse_sendx.txt` (bare `send` → the define body; `sendClock` → template `Clock`):

```
muxt generate --use-receiver-type=Server
muxt check

exec go test -race -count=1

-- template.gohtml --
{{- define "GET /events sse(Stream(ctx, send, sendClock))" -}}body:{{- .Result -}}{{- end -}}
{{- define "Clock" -}}clock:{{- .Result -}}{{- end -}}
-- go.mod --
module server

go 1.24
-- server.go --
package server

import (
	"context"
	"embed"
	"html/template"
)

//go:embed *.gohtml
var templatesFS embed.FS

var templates = template.Must(template.ParseFS(templatesFS, "*"))

type Server struct{}

// Stream sends one body event and one Clock event, then returns.
func (Server) Stream(ctx context.Context, send func(data string) error, sendClock func(data string) error) {
	_ = send("hi")
	_ = sendClock("tick")
}
-- server_test.go --
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStreamSendX(t *testing.T) {
	mux := http.NewServeMux()
	TemplateRoutes(mux, Server{})
	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	got := rec.Body.String()
	if !strings.Contains(got, "data: body:hi\n\n") {
		t.Fatalf("body = %q, want it to contain %q", got, "data: body:hi\n\n")
	}
	if !strings.Contains(got, "data: clock:tick\n\n") {
		t.Fatalf("body = %q, want it to contain %q", got, "data: clock:tick\n\n")
	}
}
```

- [ ] **Step 2: Run the fixtures to verify they fail**

Run: `go test ./cmd/muxt -run 'Test/reference_sse_wrapper|Test/reference_sse_sendx'`
Expected: FAIL — the `sse()` wrapper is parsed and unwrapped (Task 1) but nothing dispatches on `RepresentationSSE`, so the route is generated as an ordinary handler and the test (text/event-stream, `data:` frames) fails.

- [ ] **Step 3: Add the dispatch and `sseWrapperHandlerFunc`**

In `internal/generate/routes.go`, inside `methodHandlerFunc`, immediately after `sig` is obtained (after the `sig, ok := sigs[...]` block, ~1126) and **before** the `config.Datastar && def.UsesDatastar()` branch, insert:

```go
	switch def.Representation() {
	case muxt.RepresentationSSE:
		return sseWrapperHandlerFunc(file, config, def, sigs, receiver, sig, receiverInterfaceName)
	case muxt.RepresentationMarshalJSON:
		return marshalJSONHandlerFunc(file, config, def, sigs, receiver, sig, receiverInterfaceName)
	}
```

(`marshalJSONHandlerFunc` lands in Task 4; to keep this task compiling, add a temporary stub at the bottom of `routes.go` that returns `nil, fmt.Errorf("marshalJSON not yet implemented")` — Task 4 replaces it. The `reference_marshal_json` fixtures do not exist until Task 4, so the stub is never exercised.)

Add `sseWrapperHandlerFunc` near `sseMethodHandlerFunc` (~925). For Task 2 it handles **callback mode** only; Task 3 prepends the return-mode branch:

```go
// sseWrapperHandlerFunc builds the streaming handler for a route wrapped in
// sse(...). In callback mode the method takes send/sendX callbacks: bare `send`
// renders the route's define body, and a `sendX` name renders template `X`
// (the remainder after the "send" prefix, verbatim).
func sseWrapperHandlerFunc(file *File, config RoutesFileConfiguration, def muxt.Definition, sigs map[string]*types.Signature, receiver *types.Named, sig *types.Signature, receiverInterfaceName string) (*ast.FuncLit, error) {
	return streamMethodHandlerFunc(file, config, def, sigs, receiver, sig, muxt.TemplateNameScopeIdentifierSend, "sse handler returned an error",
		func(i int, id *ast.Ident, cb *types.Signature) (ast.Expr, bool, error) {
			if !muxt.IsSendArgument(id.Name) {
				return nil, false, nil
			}
			resultType, hasArg, err := validateSSECallbackShape(def.FunctionIdentifier().Name, cb)
			if err != nil {
				return nil, false, err
			}
			templateName := def.Name()
			if id.Name != muxt.TemplateNameScopeIdentifierSend {
				templateName = strings.TrimPrefix(id.Name, muxt.TemplateNameScopeIdentifierSend)
				if def.Template() == nil || def.Template().Lookup(templateName) == nil {
					return nil, false, fmt.Errorf("no template %q for send argument %s", templateName, id.Name)
				}
			}
			closure, err := sseClosure(file, config, def, templateName, resultType, hasArg, receiverInterfaceName, streamFlusherIdent, streamMutexIdent)
			return closure, true, err
		})
}
```

This mirrors the existing `sseMethodHandlerFunc` (routes.go:925) exactly, differing only in: keyed on `IsSendArgument` not `IsSSEArgument`; the `send` base identifier; and `strings.TrimPrefix(id.Name, "send")` so `sendClock` → template `Clock` (the existing sse path used the whole `sseClock` as the template name). Confirm `strings` is imported in `routes.go` (it is).

- [ ] **Step 4: Run the fixtures to verify they pass**

Run: `go test ./cmd/muxt -run 'Test/reference_sse_wrapper|Test/reference_sse_sendx'`
Expected: PASS.

- [ ] **Step 5: Run the full suite**

Run: `go test ./...`
Expected: PASS (old `sse` arg path still present and unchanged; new wrapper path added).

- [ ] **Step 6: Commit**

```bash
gofumpt -w internal/generate/routes.go
git add internal/generate/routes.go cmd/muxt/testdata/reference_sse_wrapper.txt cmd/muxt/testdata/reference_sse_sendx.txt
git commit -m "feat(generate): sse() wrapper callback mode with send/sendX

Assisted-by: Claude:claude-opus-4-8 gofumpt"
```

---

## Task 3: `sse()` return mode — `iter.Seq`, `iter.Seq2`, channels

**Files:**
- Modify: `internal/generate/routes.go` (`sseWrapperHandlerFunc`; add a return-mode helper and return-type detection)
- Test: `cmd/muxt/testdata/reference_sse_iter_seq.txt`, `reference_sse_chan.txt`, `reference_sse_iter_seq2_error.txt`, `err_sse_iter_and_send.txt`, `err_sse_unwrapped_iter.txt`

- [ ] **Step 1: Write the failing fixtures**

`cmd/muxt/testdata/reference_sse_iter_seq.txt` — method returns `iter.Seq[T]`; one event per value:

```
muxt generate --use-receiver-type=Server
muxt check

exec go test -race -count=1

-- template.gohtml --
{{- define "GET /events sse(Ticks(ctx))" -}}{{- .Result -}}{{- end -}}
-- go.mod --
module server

go 1.24
-- server.go --
package server

import (
	"context"
	"embed"
	"html/template"
	"iter"
)

//go:embed *.gohtml
var templatesFS embed.FS

var templates = template.Must(template.ParseFS(templatesFS, "*"))

type Server struct{}

// Ticks yields three values; each is rendered as one SSE event via the define body.
func (Server) Ticks(ctx context.Context) iter.Seq[string] {
	return func(yield func(string) bool) {
		for _, v := range []string{"a", "b", "c"} {
			if !yield(v) {
				return
			}
		}
	}
}
-- server_test.go --
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTicks(t *testing.T) {
	mux := http.NewServeMux()
	TemplateRoutes(mux, Server{})
	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}
	want := "data: a\n\ndata: b\n\ndata: c\n\n"
	if got := rec.Body.String(); got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}
```

`cmd/muxt/testdata/reference_sse_chan.txt` — method returns `<-chan T`:

```
muxt generate --use-receiver-type=Server
muxt check

exec go test -race -count=1

-- template.gohtml --
{{- define "GET /events sse(Ticks(ctx))" -}}{{- .Result -}}{{- end -}}
-- go.mod --
module server

go 1.24
-- server.go --
package server

import (
	"context"
	"embed"
	"html/template"
)

//go:embed *.gohtml
var templatesFS embed.FS

var templates = template.Must(template.ParseFS(templatesFS, "*"))

type Server struct{}

// Ticks returns a channel that yields two values then closes.
func (Server) Ticks(ctx context.Context) <-chan string {
	ch := make(chan string, 2)
	ch <- "x"
	ch <- "y"
	close(ch)
	return ch
}
-- server_test.go --
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTicksChan(t *testing.T) {
	mux := http.NewServeMux()
	TemplateRoutes(mux, Server{})
	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	want := "data: x\n\ndata: y\n\n"
	if got := rec.Body.String(); got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}
```

`cmd/muxt/testdata/reference_sse_iter_seq2_error.txt` — `iter.Seq2[T, error]`; a yielded error reaches the template-data error list (the define body renders `.Error`):

```
muxt generate --use-receiver-type=Server
muxt check

exec go test -race -count=1

-- template.gohtml --
{{- define "GET /events sse(Ticks(ctx))" -}}{{- if .Error -}}err:{{- range .Error -}}{{- . -}}{{- end -}}{{- else -}}ok:{{- .Result -}}{{- end -}}{{- end -}}
-- go.mod --
module server

go 1.24
-- server.go --
package server

import (
	"context"
	"embed"
	"errors"
	"html/template"
	"iter"
)

//go:embed *.gohtml
var templatesFS embed.FS

var templates = template.Must(template.ParseFS(templatesFS, "*"))

type Server struct{}

// Ticks yields one good value and then an error.
func (Server) Ticks(ctx context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		if !yield("good", nil) {
			return
		}
		yield("", errors.New("boom"))
	}
}
-- server_test.go --
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTicksSeq2(t *testing.T) {
	mux := http.NewServeMux()
	TemplateRoutes(mux, Server{})
	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	got := rec.Body.String()
	if !strings.Contains(got, "data: ok:good\n\n") {
		t.Fatalf("body = %q, want it to contain %q", got, "data: ok:good\n\n")
	}
	if !strings.Contains(got, "data: err:boom\n\n") {
		t.Fatalf("body = %q, want it to contain %q", got, "data: err:boom\n\n")
	}
}
```

`cmd/muxt/testdata/err_sse_iter_and_send.txt` — both an iterator return and a send callback:

```
! muxt generate --use-receiver-type=Server
stderr 'cannot use both a send callback and an iterator or channel return'

-- template.gohtml --
{{- define "GET /events sse(Ticks(ctx, send))" -}}{{- .Result -}}{{- end -}}
-- go.mod --
module server

go 1.24
-- server.go --
package server

import (
	"context"
	"embed"
	"html/template"
	"iter"
)

//go:embed *.gohtml
var templatesFS embed.FS

var templates = template.Must(template.ParseFS(templatesFS, "*"))

type Server struct{}

func (Server) Ticks(ctx context.Context, send func(string) error) iter.Seq[string] {
	return func(yield func(string) bool) {}
}
```

`cmd/muxt/testdata/err_sse_unwrapped_iter.txt` — iterator return without `sse(...)`:

```
! muxt generate --use-receiver-type=Server
stderr 'returns an iterator or channel; wrap the call in sse\(...\)'

-- template.gohtml --
{{- define "GET /events Ticks(ctx)" -}}{{- .Result -}}{{- end -}}
-- go.mod --
module server

go 1.24
-- server.go --
package server

import (
	"context"
	"embed"
	"html/template"
	"iter"
)

//go:embed *.gohtml
var templatesFS embed.FS

var templates = template.Must(template.ParseFS(templatesFS, "*"))

type Server struct{}

func (Server) Ticks(ctx context.Context) iter.Seq[string] {
	return func(yield func(string) bool) {}
}
```

- [ ] **Step 2: Run the fixtures to verify they fail**

Run: `go test ./cmd/muxt -run 'Test/reference_sse_iter_seq$|Test/reference_sse_chan|Test/reference_sse_iter_seq2_error|Test/err_sse_iter_and_send|Test/err_sse_unwrapped_iter'`
Expected: FAIL — return-mode handling and the two new errors don't exist yet.

- [ ] **Step 3: Add return-type detection**

Add a helper in `internal/generate/routes.go` that classifies the inner method's single result type. Use `go/types`:

```go
// sseReturnKind classifies an sse() method result type as a stream source.
type sseReturnKind int

const (
	sseReturnNone  sseReturnKind = iota // not a stream (error/nothing -> callback mode)
	sseReturnChan                       // <-chan T or chan T
	sseReturnSeq                        // iter.Seq[T]
	sseReturnSeq2                        // iter.Seq2[T, error]
)

// classifySSEReturn inspects sig's first result (if any) and returns the stream
// kind plus the element type T (and, for Seq2, that T is the value type).
func classifySSEReturn(sig *types.Signature) (sseReturnKind, types.Type) {
	if sig.Results().Len() != 1 {
		return sseReturnNone, nil
	}
	rt := sig.Results().At(0).Type()
	if ch, ok := rt.Underlying().(*types.Chan); ok {
		return sseReturnChan, ch.Elem()
	}
	named, ok := types.Unalias(rt).(*types.Named)
	if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != "iter" {
		return sseReturnNone, nil
	}
	args := named.TypeArgs()
	switch named.Obj().Name() {
	case "Seq":
		if args != nil && args.Len() == 1 {
			return sseReturnSeq, args.At(0)
		}
	case "Seq2":
		if args != nil && args.Len() == 2 {
			return sseReturnSeq2, args.At(0)
		}
	}
	return sseReturnNone, nil
}
```

- [ ] **Step 4: Branch `sseWrapperHandlerFunc` into return mode vs callback mode**

At the top of `sseWrapperHandlerFunc`, before the `streamMethodHandlerFunc` call, add:

```go
	kind, elem := classifySSEReturn(sig)
	hasSend := def.usesSend() // see Step 5
	if kind != sseReturnNone {
		if hasSend {
			return nil, fmt.Errorf("call %s cannot use both a send callback and an iterator or channel return", def.FunctionIdentifier().Name)
		}
		return sseReturnHandlerFunc(file, config, def, sigs, receiver, sig, receiverInterfaceName, kind, elem)
	}
```

(Keep the existing callback-mode `streamMethodHandlerFunc` call as the fallthrough.)

Add the unwrapped-iterator error: in `methodHandlerFunc`, after the `Representation()` switch and after the existing `sig.Results().Len() == 0` check region, add a guard so a non-wrapped iterator/channel method is a clear error rather than a confusing downstream failure. Place it right after the `Representation()` switch:

```go
	if def.Representation() == muxt.RepresentationNone {
		if kind, _ := classifySSEReturn(sig); kind != sseReturnNone {
			return nil, fmt.Errorf("call %s returns an iterator or channel; wrap the call in sse(...)", def.FunctionIdentifier().Name)
		}
	}
```

- [ ] **Step 5: Add `usesSend` to the parser**

In `internal/muxt/definition.go`, add (near `UsesSSE`):

```go
// usesSend reports whether the (unwrapped) call uses a send/sendX render callback.
func (def Definition) usesSend() bool {
	return def.usesArgument(IsSendArgument)
}
```

Export it as `UsesSend` if `internal/generate` cannot reach an unexported method (it is a different package, so it MUST be exported):

```go
// UsesSend reports whether the sse() call uses a send/sendX render callback.
func (def Definition) UsesSend() bool {
	return def.usesArgument(IsSendArgument)
}
```

Use `def.UsesSend()` in Step 4.

- [ ] **Step 6: Implement `sseReturnHandlerFunc`**

Add to `internal/generate/routes.go`. It reuses the stream scaffold from `streamMethodHandlerFunc` (headers, flusher, mutex, ctx/path parsing) but instead of replacing callback args, it **ranges over the method's returned iterator/channel**, rendering each value via an `sseClosure`-shaped render func. Build it by mirroring `streamMethodHandlerFunc` (routes.go:786-920); the difference is the tail:

- Build `render := sseClosure(file, config, def, def.Name(), elem, true /*hasArg*/, receiverInterfaceName, streamFlusherIdent, streamMutexIdent)` and assign it to a local `sseRender` (a `func(T) error`).
- For `sseReturnSeq` / `sseReturnChan`, emit:

```go
for v := range RECEIVER.Method(args...) {
	if err := sseRender(v); err != nil {
		slog... // executeTemplateFailedLogLine(file, "sse handler returned an error", errIdent)
		return
	}
}
```

- For `sseReturnSeq2`, also build an error-render func `sseRenderErr` that sets the template-data error list. The simplest correct approach: extend `sseClosure` with an optional error parameter, OR add a sibling `sseClosureWithError` that takes `(result T, iterErr error)` and seeds the `SSETemplateData` `TemplateDataFieldIdentifierError` field with `[]error{iterErr}` when non-nil. Then emit:

```go
for v, iterErr := range RECEIVER.Method(args...) {
	if err := sseRender(v, iterErr); err != nil {
		slog...
		return
	}
}
```

  where the render seeds the td as `SSETemplateData[Recv, T]{..., result: v, errors: []error{iterErr}}` when `iterErr != nil` (omit the field when nil so non-error events render normally). The td error field identifier is `TemplateDataFieldIdentifierError` (type `[]error`), confirmed at `internal/generate/sse_template_data.go:95`.

Implementation note: to keep the buffer/mutex/flush logic DRY, factor the per-value render body out of `sseClosure` into a helper that both the callback closure and the range body can call, OR generate the closure and invoke it inline as shown. Either is acceptable; prefer reusing `sseClosure` for `Seq`/`chan` and a small variant for `Seq2`.

The method call expression is `RECEIVER.Method(parsedArgs...)` — reuse the `callFun` construction from `streamMethodHandlerFunc` (routes.go:799-804) and the parsed-arg statements already appended by `appendParseArgumentStatements`. For return mode there are no callback args to replace, so the call args are the parsed locals as usual.

- [ ] **Step 7: Run the fixtures to verify they pass**

Run: `go test ./cmd/muxt -run 'Test/reference_sse_iter_seq$|Test/reference_sse_chan|Test/reference_sse_iter_seq2_error|Test/err_sse_iter_and_send|Test/err_sse_unwrapped_iter'`
Expected: PASS.

- [ ] **Step 8: Run the full suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
gofumpt -w internal/generate/routes.go internal/muxt/definition.go
git add internal/generate/routes.go internal/muxt/definition.go cmd/muxt/testdata/reference_sse_iter_seq.txt cmd/muxt/testdata/reference_sse_chan.txt cmd/muxt/testdata/reference_sse_iter_seq2_error.txt cmd/muxt/testdata/err_sse_iter_and_send.txt cmd/muxt/testdata/err_sse_unwrapped_iter.txt
git commit -m "feat(generate): sse() return mode for iter.Seq, iter.Seq2, channels

Assisted-by: Claude:claude-opus-4-8 gofumpt"
```

---

## Task 4: `marshalJSON(Method(...))` response wrapper

**Files:**
- Create: `internal/generate/marshal_json.go`
- Modify: `internal/generate/routes.go` (replace the Task 2 stub with the real `marshalJSONHandlerFunc`)
- Test: `cmd/muxt/testdata/reference_marshal_json.txt`, `reference_marshal_json_jsonv2.txt`, `err_marshal_json_no_value.txt`

- [ ] **Step 1: Write the failing fixtures**

`cmd/muxt/testdata/reference_marshal_json.txt` — default `encoding/json`:

```
muxt generate --use-receiver-type=Server
muxt check

exec go test -race -count=1

-- template.gohtml --
{{- define "GET /api/user marshalJSON(GetUser(ctx))" -}}{{- end -}}
-- go.mod --
module server

go 1.24
-- server.go --
package server

import (
	"context"
	"embed"
	"html/template"
)

//go:embed *.gohtml
var templatesFS embed.FS

var templates = template.Must(template.ParseFS(templatesFS, "*"))

type Server struct{}

type User struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func (Server) GetUser(ctx context.Context) (User, error) {
	return User{Name: "Ada", Age: 36}, nil
}
-- server_test.go --
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetUserJSON(t *testing.T) {
	mux := http.NewServeMux()
	TemplateRoutes(mux, Server{})
	req := httptest.NewRequest(http.MethodGet, "/api/user", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"name":"Ada","age":36}` {
		t.Fatalf("body = %q, want %q", got, `{"name":"Ada","age":36}`)
	}
}
```

`cmd/muxt/testdata/reference_marshal_json_jsonv2.txt` — generation-only, asserts the v2 path:

```
muxt generate --use-receiver-type=Server --output-jsonv2

grep 'json.MarshalWrite' template_routes.go
! grep 'go-json-experiment' template_routes.go
grep 'encoding/json/v2' template_routes.go

-- template.gohtml --
{{- define "GET /api/user marshalJSON(GetUser(ctx))" -}}{{- end -}}
-- go.mod --
module server

go 1.25
-- server.go --
package server

import (
	"context"
	"embed"
	"html/template"
)

//go:embed *.gohtml
var templatesFS embed.FS

var templates = template.Must(template.ParseFS(templatesFS, "*"))

type Server struct{}

type User struct {
	Name string `json:"name"`
}

func (Server) GetUser(ctx context.Context) (User, error) {
	return User{Name: "Ada"}, nil
}
```

`cmd/muxt/testdata/err_marshal_json_no_value.txt` — method with no return value:

```
! muxt generate --use-receiver-type=Server
stderr 'marshalJSON method GetNothing must return a value to marshal'

-- template.gohtml --
{{- define "GET /api/x marshalJSON(GetNothing(ctx))" -}}{{- end -}}
-- go.mod --
module server

go 1.24
-- server.go --
package server

import (
	"context"
	"embed"
	"html/template"
)

//go:embed *.gohtml
var templatesFS embed.FS

var templates = template.Must(template.ParseFS(templatesFS, "*"))

type Server struct{}

func (Server) GetNothing(ctx context.Context) {}
```

- [ ] **Step 2: Run the fixtures to verify they fail**

Run: `go test ./cmd/muxt -run 'Test/reference_marshal_json$|Test/reference_marshal_json_jsonv2|Test/err_marshal_json_no_value'`
Expected: FAIL — `marshalJSONHandlerFunc` is still the stub.

- [ ] **Step 3: Create `internal/generate/marshal_json.go`**

Add the package-level JSON helper (mirror `datastarMarshalSignalsFunc`, datastar_signals.go:36), the response handler, and the decl-emission hook. The helper:

```go
package generate

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"net/http"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
)

// marshalJSONResponseFuncName is the generated helper that marshals a value as
// JSON to the response writer. Emitted when at least one route uses marshalJSON.
const marshalJSONResponseFuncName = "marshalJSONResponse"

// marshalJSONResponseDecls returns the marshalJSONResponse helper.
func marshalJSONResponseDecls(file *File, config RoutesFileConfiguration) []ast.Decl {
	return []ast.Decl{marshalJSONResponseFunc(file, config)}
}

// marshalJSONResponseFunc builds:
//
//	func marshalJSONResponse(response http.ResponseWriter, v any) {
//		response.Header().Set("Content-Type", "application/json")
//		// --output-jsonv2:
//		if err := json.MarshalWrite(response, v); err != nil {
//			http.Error(response, err.Error(), http.StatusInternalServerError)
//		}
//		// default:
//		b, err := json.Marshal(v)
//		if err != nil {
//			http.Error(response, err.Error(), http.StatusInternalServerError)
//			return
//		}
//		_, _ = response.Write(b)
//	}
func marshalJSONResponseFunc(file *File, config RoutesFileConfiguration) *ast.FuncDecl { /* mirror datastarMarshalSignalsFunc, writing to response and setting the header */ }
```

The response handler `marshalJSONHandlerFunc(file, config, def, sigs, receiver, sig, receiverInterfaceName)` must:

1. Validate the inner method returns at least one value: `if sig.Results().Len() == 0 { return nil, fmt.Errorf("marshalJSON method %s must return a value to marshal", def.FunctionIdentifier().Name) }`. The first result is the value `T`; an optional second result of type `error` is handled.
2. Build the `http.HandlerFunc` literal (`astgen.HTTPHandlerFuncType`, response/request idents).
3. Bind ctx / path params / `unmarshalX(body)` args by reusing `appendParseArgumentStatements` exactly as `methodHandlerFunc` does (routes.go:1202) — but with a `parseErrBlock` that writes `http.Error(response, ..., 400)` and returns (model after `streamMethodHandlerFunc`'s `parseErrBlock`, routes.go:841, since there is no template-data error list here).
4. Call the receiver method capturing `(v)` or `(v, err)`. If the method returns an error, emit `if err != nil { http.Error(response, err.Error(), http.StatusInternalServerError); return }`.
5. Emit `marshalJSONResponse(response, v)`.

Use `callFun` construction like `streamMethodHandlerFunc` (routes.go:799-804). For the no-error single-return case, assign `v := RECEIVER.Method(args...)`.

- [ ] **Step 4: Emit the helper decl and replace the stub**

Find where the datastar signals helper is conditionally emitted (search `datastarSignalsDecls` in `internal/generate/routes.go`) and add a parallel emission: when at least one definition has `def.Representation() == muxt.RepresentationMarshalJSON`, append `marshalJSONResponseDecls(file, config)...` to the file decls. (Mirror the existing `datastarSignalsDecls` gating predicate — it scans definitions for a usage.)

Replace the Task 2 stub `marshalJSONHandlerFunc` with the real implementation from Step 3.

- [ ] **Step 5: Run the fixtures to verify they pass**

Run: `go test ./cmd/muxt -run 'Test/reference_marshal_json$|Test/reference_marshal_json_jsonv2|Test/err_marshal_json_no_value'`
Expected: PASS.

- [ ] **Step 6: Run the full suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
gofumpt -w internal/generate/marshal_json.go internal/generate/routes.go
git add internal/generate/marshal_json.go internal/generate/routes.go cmd/muxt/testdata/reference_marshal_json.txt cmd/muxt/testdata/reference_marshal_json_jsonv2.txt cmd/muxt/testdata/err_marshal_json_no_value.txt
git commit -m "feat(generate): marshalJSON() response wrapper

Assisted-by: Claude:claude-opus-4-8 gofumpt"
```

---

## Task 5: `marshalJSON(sendX)` — marshal a send callback's value

**Files:**
- Modify: `internal/muxt/definition.go` (`checkArguments` — accept `marshalJSON(sendX)` when `allowSend`)
- Modify: `internal/generate/routes.go` (`sseWrapperHandlerFunc` buildClosure — recognize a wrapped send arg) and `internal/generate/marshal_json.go` (the marshal-send closure)
- Test: `cmd/muxt/testdata/reference_sse_marshal_send.txt`

- [ ] **Step 1: Write the failing fixture**

`cmd/muxt/testdata/reference_sse_marshal_send.txt` — `marshalJSON(sendStatus)` emits a JSON `data:` event:

```
muxt generate --use-receiver-type=Server
muxt check

exec go test -race -count=1

-- template.gohtml --
{{- define "GET /events sse(Stream(ctx, marshalJSON(sendStatus)))" -}}{{- end -}}
-- go.mod --
module server

go 1.24
-- server.go --
package server

import (
	"context"
	"embed"
	"html/template"
)

//go:embed *.gohtml
var templatesFS embed.FS

var templates = template.Must(template.ParseFS(templatesFS, "*"))

type Server struct{}

type Status struct {
	OK bool `json:"ok"`
}

// Stream sends one signal value marshaled as JSON, then returns.
func (Server) Stream(ctx context.Context, sendStatus func(Status) error) {
	_ = sendStatus(Status{OK: true})
}
-- server_test.go --
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStreamMarshalSend(t *testing.T) {
	mux := http.NewServeMux()
	TemplateRoutes(mux, Server{})
	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if got := rec.Body.String(); !strings.Contains(got, `data: {"ok":true}`+"\n\n") {
		t.Fatalf("body = %q, want it to contain %q", got, `data: {"ok":true}`+"\n\n")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./cmd/muxt -run 'Test/reference_sse_marshal_send'`
Expected: FAIL — `checkArguments` rejects the `marshalJSON(sendStatus)` arg (inner is not `body`), or generation does not produce a JSON event.

- [ ] **Step 3: Accept `marshalJSON(sendX)` in `checkArguments`**

In `internal/muxt/definition.go` `checkArguments`, extend the `*ast.CallExpr` case so that, when `allowSend` and the call's `Fun` is `RepresentationWrapperMarshalJSON` with a single `send`/`sendX` ident arg, it is accepted (before the input-wrapper check fallthrough that requires `body`):

```go
case *ast.CallExpr:
	if id, ok := exp.Fun.(*ast.Ident); ok {
		if allowSend && id.Name == RepresentationWrapperMarshalJSON {
			if len(exp.Args) == 1 {
				if inner, ok := exp.Args[0].(*ast.Ident); ok && IsSendArgument(inner.Name) {
					continue
				}
			}
			return fmt.Errorf("marshalJSON send wrapper takes exactly one send callback argument")
		}
		if IsInputWrapper(id.Name) {
			// ... existing body check ...
		}
	}
	if err := checkArguments(identifiers, exp, allowSend); err != nil {
		return fmt.Errorf("call %s argument error: %w", astgen.Format(call.Fun), err)
	}
```

- [ ] **Step 4: Generate the marshal-send closure**

The send-callback wiring in `streamMethodHandlerFunc` only replaces **ident** args (routes.go:887-905 iterates `def.CallExpression().Args` and skips non-idents). A `marshalJSON(sendStatus)` arg is a `*ast.CallExpr`, so extend the arg loop in `streamMethodHandlerFunc` to also pass `*ast.CallExpr` args to `buildClosure` (or handle them in a parallel branch). Pass the wrapped send ident and a flag indicating "marshal" to the closure builder.

In `sseWrapperHandlerFunc`'s `buildClosure`, detect the wrapped form: if the arg is `marshalJSON(sendX)`, build a **marshal closure** instead of `sseClosure`. Add `marshalSendClosure(file, config, resultType, flusherIdent, mutexIdent)` to `internal/generate/marshal_json.go`. It mirrors `sseClosure` (routes.go:967) but, instead of `templates.ExecuteTemplate`, marshals the value into the pooled buffer with the JSONV2-aware path, then frames it through `SSETemplateData.WriteTo` (so the bytes become `data:` lines) under the mutex and flushes. Reuse the buffer→`SSETemplateData{data: buf}`→`WriteTo` tail from `sseClosure`; only the "fill the buffer" step differs (marshal vs execute-template). For the marshal step reuse the same JSONV2 branch as `marshalJSONResponseFunc` (a shared internal helper `appendMarshalIntoBuffer(file, config, bufIdent, valueIdent)` returning `[]ast.Stmt` keeps it DRY between the response handler and this closure).

To extract the callback's value type `T` for the closure parameter: the wrapped send callback's signature is `sig.Params().At(i)` (a `func(T) error`); take `cb.Params().At(0).Type()`.

- [ ] **Step 5: Run the fixture to verify it passes**

Run: `go test ./cmd/muxt -run 'Test/reference_sse_marshal_send'`
Expected: PASS.

- [ ] **Step 6: Run the full suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
gofumpt -w internal/muxt/definition.go internal/generate/routes.go internal/generate/marshal_json.go
git add internal/muxt/definition.go internal/generate/routes.go internal/generate/marshal_json.go cmd/muxt/testdata/reference_sse_marshal_send.txt
git commit -m "feat(generate): marshalJSON(sendX) marshals a send value as a JSON event

Assisted-by: Claude:claude-opus-4-8 gofumpt"
```

---

## Task 6: Remove the dev-only `sse`/`sseX` argument family; migrate fixtures

**Files:**
- Modify: `internal/muxt/definition.go` (delete `TemplateNameScopeIdentifierSSE`, `IsSSEArgument`, `UsesSSE`; remove `sse` from `patternScope`; drop `IsSSEArgument` from `checkArguments`)
- Modify: `internal/generate/routes.go` (delete `sseArg`, `sseMethodHandlerFunc`, `validateSSEMethodResults`; remove the old SSE branch and the `config.Datastar && def.UsesSSE()` guard from `methodHandlerFunc`; rename `validateSSECallbackShape`'s diagnostic from "sse argument" to "send argument" / pass a label)
- Modify/rename: `cmd/muxt/testdata/reference_sse.txt`, `reference_sse_no_arg.txt`, `reference_sse_error_return.txt`, `reference_sse_multiple_callbacks.txt`, `reference_sse_synthesized_method.txt`, `reference_sse_unexported.txt`; `err_sse_with_execute.txt`, `err_sse_with_response.txt`, `err_sse_method_returns_value.txt`, `err_sse_callback_not_func.txt`, `err_sse_missing_template.txt`

- [ ] **Step 1: Migrate the existing SSE fixtures to the wrapper**

For each `reference_sse*.txt` and `err_sse*.txt`, rewrite the template name from the `sse` arg form to the `sse()` wrapper + `send`/`sendX`:
- `Stream(ctx, lastEventID, sse)` → `sse(Stream(ctx, lastEventID, send))`
- multiple callbacks `Stream(ctx, sse, sseClock)` → `sse(Stream(ctx, send, sendClock))` **and** rename the corresponding `{{define "sseClock"}}` template to `{{define "Clock"}}` (the wrapper strips the `send` prefix). Update the method parameter names to `send`/`sendClock` and any test assertions.
- `err_sse_with_execute.txt`: `execute` is a non-SSE callback; combining it with `sse()` should error. Update the expected error to match the new path (the `sse()` method validation rejects an `execute` arg, or the unknown-argument path — pick the error the new code actually produces and assert it).
- `err_sse_with_response.txt`: same treatment for the `response` arg.

Run each migrated fixture as you go: `go test ./cmd/muxt -run 'Test/reference_sse$'` etc. They should still pass **before** deleting the old code, because the wrapper path (Tasks 2–3) already handles them.

- [ ] **Step 2: Verify migrated fixtures pass on the new path**

Run: `go test ./cmd/muxt -run 'Test/reference_sse|Test/err_sse'`
Expected: PASS (now exercising `sse()` + `send`/`sendX`).

- [ ] **Step 3: Delete the old `sse` arg code**

Remove from `internal/muxt/definition.go`: `TemplateNameScopeIdentifierSSE` const, `IsSSEArgument`, `UsesSSE`, the `sse` entry in `patternScope()`, and the `IsSSEArgument(...)` term in `checkArguments`. Remove from `internal/generate/routes.go`: `sseArg`, `sseMethodHandlerFunc`, `validateSSEMethodResults` (if now unused), the `if _, _, hasSSE := sseArg(...)` branch and the `config.Datastar && def.UsesSSE()` guard in `methodHandlerFunc`. Update `validateSSECallbackShape` error text and `validateStreamMethodResults` callers to use the `send` label.

Use `gopls rename`/`go_symbol_references` (via the muxt skills) to confirm no remaining references before deleting each symbol.

- [ ] **Step 4: Run the full suite**

Run: `go test ./...`
Expected: PASS. Then `go vet ./...` — clean (no unused functions).

- [ ] **Step 5: Commit**

```bash
gofumpt -w internal/muxt/definition.go internal/generate/routes.go
git add -A
git commit -m "refactor(generate): remove dev-only sse/sseX arg family; migrate to sse() wrapper

Assisted-by: Claude:claude-opus-4-8 gopls gofumpt"
```

---

## Task 7: Documentation

**Files:**
- Modify: `docs/reference/call-parameters.md`, `docs/reference/datastar.md`
- Modify (status): `docs/explanation/decisions/00008_sse_wrapper_and_send_callbacks.md`, `00009_event_framing_is_a_custom_marshaler.md` (note "Implemented in Phase 2" under Status if the ADR convention tracks it; otherwise leave Status "Decided")

- [ ] **Step 1: Document the wrappers**

In `docs/reference/call-parameters.md`, add a "Response representation" section covering: `sse(Method(...))` (callback mode with `send`/`sendX`; `sendX` → template `X`; return mode with `<-chan T` / `iter.Seq[T]` / `iter.Seq2[T,error]`; the iter.Seq2 error → `.Error`); `marshalJSON(Method(...))` (application/json; `--output-jsonv2`); and `marshalJSON(sendX)` inside `sse(...)`. Note that `sse`/`sseX` arguments are removed and replaced by `sse(...)` + `send`/`sendX`.

In `docs/reference/datastar.md`, update any reference to the `sse` argument to the `sse()` wrapper and note that datastar `patch-elements` / `patch-signals` framing of `sse()` arrives in Phase 3 (the existing `elements`/`signal`/`script` args remain for now).

- [ ] **Step 2: Verify docs match behavior**

Cross-check each documented example's template name against a passing fixture (or run a quick `muxt generate` in a scratch extract). Fix any mismatch.

- [ ] **Step 3: Commit**

```bash
git add docs/reference/call-parameters.md docs/reference/datastar.md docs/explanation/decisions/00008_sse_wrapper_and_send_callbacks.md docs/explanation/decisions/00009_event_framing_is_a_custom_marshaler.md
git commit -m "docs: document sse()/marshalJSON() representation wrappers

Assisted-by: Claude:claude-opus-4-8"
```

---

## Self-Review

**Spec coverage:**
- `sse()` recognition + unwrap → Task 1. ✓
- `send`/`sendX` callbacks (`sendX` → template `X` verbatim) → Task 2. ✓
- iterator/channel returns (`<-chan T`, `iter.Seq[T]`, `iter.Seq2[T,error]`; iter.Seq2 error → error list) → Task 3. ✓
- mutually-exclusive callback vs return; unwrapped iterator error → Task 3. ✓
- `marshalJSON(Method(...))` (default + `--output-jsonv2`; no-value error; 500 on error) → Task 4. ✓
- `marshalJSON(sendX)` → Task 5. ✓
- `--use-datastar` interaction (generic frames; datastar args untouched) → Task 2 dispatch is placed before the datastar branch and is frontend-agnostic; the datastar arg families are not modified. ✓
- `execute` unchanged → not touched by any task. ✓
- remove `sse`/`sseX` cleanly + migrate fixtures → Task 6. ✓
- `muxt check` passes in references → each `reference_*` fixture runs `muxt check`. ✓
- docs → Task 7. ✓

**Type consistency:** `Representation`/`RepresentationNone|SSE|MarshalJSON`, `RepresentationWrapperSSE|MarshalJSON`, `TemplateNameScopeIdentifierSend`, `IsSendArgument`, `UsesSend`, `Representation()`, `sseWrapperHandlerFunc`, `sseReturnHandlerFunc`, `classifySSEReturn`/`sseReturnKind`, `marshalJSONHandlerFunc`, `marshalJSONResponseFunc`/`marshalJSONResponseFuncName`, `marshalSendClosure`, `appendMarshalIntoBuffer` are used consistently across tasks. `checkArguments` gains the `allowSend bool` parameter in Task 1 and is used with it in Tasks 1 and 5.

**Sequencing / green-at-each-commit:** Task 1 additive (old path intact). Task 2 adds the new dispatch + a temporary `marshalJSONHandlerFunc` stub (never exercised until Task 4). Task 3 adds return mode. Task 4 replaces the stub. Task 5 adds the wrapped-send form. Task 6 deletes the old `sse` arg family only after fixtures are migrated to the wrapper. Each task ends with `go test ./...` green.
