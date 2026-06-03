# Phase 1 — Request Input Binding (`body` + `unmarshalX`) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **This is a muxt project — use the muxt skills** (`muxt_test-driven-development`, `muxt_explore-from-method`, `muxt_debug-generation-errors`). Tests are txtar fixtures in `cmd/muxt/testdata/`; the runner is `TestDocumentation`/`Test` in `cmd/muxt/main_test.go` (rsc.io/script). Run a single fixture with `go test ./cmd/muxt -run 'Test/<fixture_basename>$'`.

**Goal:** Let a template-name call bind the request body to a method parameter via a `body` (`io.Reader`) identifier and `unmarshalJSON(body)` / `unmarshalForm(body)` wrapper pseudo-functions.

**Architecture:** The call is parsed as a Go expression. `body` joins the reserved scope (typed `io.Reader`, wired to `request.Body`). `unmarshalJSON`/`unmarshalForm` are recognized wrapper names in the `*ast.CallExpr` argument branch of `appendParseArgumentStatements`; they decode into the receiver method's parameter type at that position (or a raw pass-through type for undefined methods). Non-GET `form` is recast to route through the `unmarshalForm` codegen.

**Tech Stack:** Go `go/ast`/`go/types` code generation; `encoding/json` (+ `encoding/json/v2`/`encoding/json/jsontext` under `--output-jsonv2`); txtar tests.

Spec: `docs/superpowers/specs/2026-06-02-call-dsl-phase1-input-binding-design.md`.

---

## File structure

- **`internal/muxt/definition.go`** — `body` scope constant + `patternScope()`; recognize `unmarshalJSON`/`unmarshalForm` wrapper names in `checkArguments`; exported `IsInputWrapper(name string) bool` + the wrapper-name constants.
- **`internal/generate/routes.go`** — `defaultTemplateNameScope` (`body`→`io.Reader`); the `*ast.CallExpr` branch in `appendParseArgumentStatements` (wrapper dispatch); `createMethodSignature` (synthesize target type for undefined methods); recast non-GET `form` to the `unmarshalForm` path; `body` local in the ident special-case switch.
- **`internal/generate/input_binding.go`** (new) — the `unmarshalJSON`/`unmarshalForm` codegen helpers; mirrors `formVariableAssignment`/`appendStructFieldParseStatements` (form) and `datastarMarshalSignalsFunc` (jsonv2 branching) in `internal/generate/datastar_signals.go`.
- **`cmd/muxt/testdata/*.txt`** — new fixtures (one per task).
- **`docs/reference/call-parameters.md`** — document `body` + wrappers.

Commit after every task (each task ends green).

---

## Task 1: `body` reserved identifier (`io.Reader`)

**Files:**
- Modify: `internal/muxt/definition.go` (scope consts + `patternScope`)
- Modify: `internal/generate/routes.go` (`defaultTemplateNameScope`; ident special-case switch ~`:1469`)
- Test: `cmd/muxt/testdata/reference_body_reader.txt`

- [ ] **Step 1: Write the failing fixture**

Create `cmd/muxt/testdata/reference_body_reader.txt`:

```
muxt generate --use-receiver-type=Server
muxt check

exec go test -race -count=1

-- template.gohtml --
{{- define "POST /echo Echo(ctx, body)" -}}{{- .Result -}}{{- end -}}
-- go.mod --
module server

go 1.25
-- server.go --
package server

import (
	"context"
	"embed"
	"html/template"
	"io"
)

//go:embed *.gohtml
var templatesFS embed.FS

var templates = template.Must(template.ParseFS(templatesFS, "*"))

type Server struct{}

func (Server) Echo(ctx context.Context, body io.Reader) string {
	b, _ := io.ReadAll(body)
	return string(b)
}
-- server_test.go --
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEcho(t *testing.T) {
	mux := http.NewServeMux()
	TemplateRoutes(mux, Server{})
	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("hello body"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if got := rec.Body.String(); got != "hello body" {
		t.Fatalf("body = %q, want %q", got, "hello body")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./cmd/muxt -run 'Test/reference_body_reader$'`
Expected: FAIL — generation errors with `failed to determine type for body` (body is not yet a known scope identifier).

- [ ] **Step 3: Add the `body` scope constant and register it**

In `internal/muxt/definition.go`, add the constant beside the other scope identifiers (the `TemplateNameScopeIdentifier*` block) and to `patternScope()`:

```go
TemplateNameScopeIdentifierBody = "body"
```

Add `TemplateNameScopeIdentifierBody` to the slice returned by `patternScope()`.

- [ ] **Step 4: Type `body` as `io.Reader` and wire the local**

In `internal/generate/routes.go` `defaultTemplateNameScope`, add a case (alongside the `context`/`form` cases):

```go
case muxt.TemplateNameScopeIdentifierBody:
	pkg, ok := file.Types("io")
	if !ok {
		return nil, false
	}
	return pkg.Scope().Lookup("Reader").Type(), true
```

In `appendParseArgumentStatements`, in the `case *ast.Ident:` block's inner `switch arg.Name` (the one with `case muxt.TemplateNameScopeIdentifierContext:` ~`:1482`), add:

```go
case muxt.TemplateNameScopeIdentifierBody:
	statements = append(statements, singleAssignment(token.DEFINE, ast.NewIdent(muxt.TemplateNameScopeIdentifierBody))(
		&ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("Body")},
	))
```

This emits `body := request.Body`. (`io.Reader` is assignable from `*request.Body`’s type, so the `types.AssignableTo` branch at `:1466` is taken.)

- [ ] **Step 5: Run it to verify it passes**

Run: `go test ./cmd/muxt -run 'Test/reference_body_reader$'`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
gofumpt -w internal/ && go test ./internal/... ./cmd/muxt -run 'Test/reference_body_reader$' -count=1
git add internal/muxt/definition.go internal/generate/routes.go cmd/muxt/testdata/reference_body_reader.txt
git commit -m "feat(generate): add body (io.Reader) request argument"
```

---

## Task 2: `unmarshalJSON(body)` into a defined parameter (encoding/json)

**Files:**
- Modify: `internal/muxt/definition.go` (`checkArguments`, `IsInputWrapper`, wrapper consts)
- Modify: `internal/generate/routes.go` (`*ast.CallExpr` branch ~`:1417`)
- Create: `internal/generate/input_binding.go`
- Test: `cmd/muxt/testdata/reference_unmarshal_json.txt`

- [ ] **Step 1: Write the failing fixture**

Create `cmd/muxt/testdata/reference_unmarshal_json.txt`:

```
muxt generate --use-receiver-type=Server
muxt check

grep '"encoding/json"' template_routes.go
! grep 'go-json-experiment' template_routes.go

exec go test -race -count=1

-- template.gohtml --
{{- define "POST /add Add(ctx, unmarshalJSON(body))" -}}{{- .Result -}}{{- end -}}
-- go.mod --
module server

go 1.25
-- server.go --
package server

import (
	"context"
	"embed"
	"html/template"
	"strconv"
)

//go:embed *.gohtml
var templatesFS embed.FS

var templates = template.Must(template.ParseFS(templatesFS, "*"))

type Server struct{}

type Pair struct {
	A int `json:"a"`
	B int `json:"b"`
}

func (Server) Add(ctx context.Context, in Pair) string {
	return strconv.Itoa(in.A + in.B)
}
-- server_test.go --
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdd(t *testing.T) {
	mux := http.NewServeMux()
	TemplateRoutes(mux, Server{})
	req := httptest.NewRequest(http.MethodPost, "/add", strings.NewReader(`{"a":2,"b":3}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if got := rec.Body.String(); got != "5" {
		t.Fatalf("body = %q, want %q", got, "5")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./cmd/muxt -run 'Test/reference_unmarshal_json$'`
Expected: FAIL — `unknown argument unmarshalJSON` (parser rejects it), or a generation error.

- [ ] **Step 3: Recognize the wrapper names in the parser**

In `internal/muxt/definition.go` add:

```go
const (
	InputWrapperUnmarshalJSON = "unmarshalJSON"
	InputWrapperUnmarshalForm = "unmarshalForm"
)

// IsInputWrapper reports whether name is a recognized request-body decode wrapper.
func IsInputWrapper(name string) bool {
	return name == InputWrapperUnmarshalJSON || name == InputWrapperUnmarshalForm
}
```

In `checkArguments`, the `*ast.CallExpr` case currently recurses into the call. Add, before recursing, a branch: if `exp.Fun` is an `*ast.Ident` and `IsInputWrapper(funIdent.Name)`, require exactly one argument and that it is the `body` identifier; otherwise error. Do **not** recurse into a wrapper call (its inner `body` is not a receiver-method argument):

```go
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
	if err := checkArguments(identifiers, exp); err != nil {
		return fmt.Errorf("call %s argument error: %w", astgen.Format(call.Fun), err)
	}
```

- [ ] **Step 4: Generate the decode for `unmarshalJSON(body)` (defined param type)**

Create `internal/generate/input_binding.go` with a helper that returns the statements to decode `request.Body` into a fresh local `varIdent` of type `targetType`, plus the local identifier to substitute for the argument. For `encoding/json` (the `--output-jsonv2`=false path):

```go
package generate

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/typelate/muxt/internal/astgen"
)

// unmarshalJSONStatements decodes request.Body into a new local `varIdent` of
// targetType and returns the statements. On error it runs parseErrBlock.
// bodyExpr is the io.Reader to read (request.Body).
func unmarshalJSONStatements(file *File, config RoutesFileConfiguration, varIdent string, targetType types.Type, bodyExpr ast.Expr, parseErrBlock *ast.BlockStmt) ([]ast.Stmt, error) {
	typeExpr, err := file.TypeASTExpression(targetType)
	if err != nil {
		return nil, err
	}
	// var v T
	decl := &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{
		&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(varIdent)}, Type: typeExpr},
	}}}
	ifErr := func(call ast.Expr) ast.Stmt {
		return &ast.IfStmt{
			Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}},
			Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
			Body: parseErrBlock,
		}
	}
	if config.JSONV2 {
		// if err := json.UnmarshalRead(body, &v); err != nil { <parseErrBlock> }
		return []ast.Stmt{decl, ifErr(astgen.Call(file, "json", "encoding/json/v2", "UnmarshalRead",
			bodyExpr, &ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(varIdent)}))}, nil
	}
	// b, err := io.ReadAll(body); if err != nil {…}; if err := json.Unmarshal(b, &v); err != nil {…}
	const bIdent = "bodyBytes"
	readAll := &ast.IfStmt{
		Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(bIdent), ast.NewIdent(errIdent)}, Tok: token.DEFINE,
			Rhs: []ast.Expr{astgen.Call(file, "", "io", "ReadAll", bodyExpr)}},
		Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
		Body: parseErrBlock,
	}
	return []ast.Stmt{decl, readAll, ifErr(astgen.Call(file, "", "encoding/json", "Unmarshal",
		ast.NewIdent(bIdent), &ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(varIdent)}))}, nil
}
```

Wire it into `appendParseArgumentStatements`. At the very top of the `case *ast.CallExpr:` branch (`routes.go:1417`), before the existing nested-call handling, add:

```go
if id, ok := arg.Fun.(*ast.Ident); ok && muxt.IsInputWrapper(id.Name) {
	varIdent := "input" + strconv.Itoa(resultCount)
	resultCount++
	bodyExpr := &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("Body")}
	var s []ast.Stmt
	var err error
	switch id.Name {
	case muxt.InputWrapperUnmarshalJSON:
		s, err = unmarshalJSONStatements(file, config, varIdent, param.Type(), bodyExpr, parseErrBlock())
	case muxt.InputWrapperUnmarshalForm:
		// Task 5
	}
	if err != nil {
		return nil, err
	}
	statements = append(statements, s...)
	call.Args[i] = ast.NewIdent(varIdent)
	continue
}
```

(`param` is `signature.Params().At(i)` at `:1412`; `parseErrBlock` is the 400 factory passed into the function.)

- [ ] **Step 5: Run it to verify it passes**

Run: `go test ./cmd/muxt -run 'Test/reference_unmarshal_json$'`
Expected: PASS (body `{"a":2,"b":3}` → `5`; generated file imports `encoding/json`, no backport).

- [ ] **Step 6: Commit**

```bash
gofumpt -w internal/ && go test ./internal/... ./cmd/muxt -run 'Test/reference_unmarshal_json$' -count=1
git add internal/muxt/definition.go internal/generate/routes.go internal/generate/input_binding.go cmd/muxt/testdata/reference_unmarshal_json.txt
git commit -m "feat(generate): unmarshalJSON(body) request input binding"
```

---

## Task 3: `--output-jsonv2` path (`json.UnmarshalRead`)

**Files:**
- Test: `cmd/muxt/testdata/reference_unmarshal_json_jsonv2.txt`

(The codegen branch already exists from Task 4 of the helper — `config.JSONV2`. This task only adds a generation-only fixture, like `reference_datastar_signals_jsonv2.txt`, because building the output needs a `GOEXPERIMENT=jsonv2` toolchain.)

- [ ] **Step 1: Write the generation-only fixture**

Create `cmd/muxt/testdata/reference_unmarshal_json_jsonv2.txt`:

```
muxt generate --use-receiver-type=Server --output-jsonv2

grep 'json "encoding/json/v2"' template_routes.go
grep 'json.UnmarshalRead' template_routes.go
! grep 'go-json-experiment' template_routes.go

-- template.gohtml --
{{- define "POST /add Add(ctx, unmarshalJSON(body))" -}}{{- .Result -}}{{- end -}}
-- go.mod --
module server

go 1.25
-- server.go --
package server

import (
	"context"
	"embed"
	"html/template"
	"strconv"
)

//go:embed *.gohtml
var templatesFS embed.FS

var templates = template.Must(template.ParseFS(templatesFS, "*"))

type Server struct{}

type Pair struct {
	A int `json:"a"`
	B int `json:"b"`
}

func (Server) Add(ctx context.Context, in Pair) string { return strconv.Itoa(in.A + in.B) }
```

- [ ] **Step 2: Run it to verify it passes**

Run: `go test ./cmd/muxt -run 'Test/reference_unmarshal_json_jsonv2$'`
Expected: PASS (grep finds `json "encoding/json/v2"` and `json.UnmarshalRead`; no backport).

- [ ] **Step 3: Commit**

```bash
git add cmd/muxt/testdata/reference_unmarshal_json_jsonv2.txt
git commit -m "test(generate): assert unmarshalJSON --output-jsonv2 uses encoding/json/v2"
```

---

## Task 4: Undefined-method pass-through (`json.RawMessage` / `*jsontext.Decoder`)

**Files:**
- Modify: `internal/generate/routes.go` (`createMethodSignature`)
- Modify: `internal/generate/input_binding.go` (pass-through codegen for raw target types)
- Test: `cmd/muxt/testdata/reference_unmarshal_json_undefined.txt`

- [ ] **Step 1: Write the failing fixture**

Create `cmd/muxt/testdata/reference_unmarshal_json_undefined.txt` (no `--use-receiver-type`, so `Lookup` is undefined and its signature is synthesized; assert it round-trips raw JSON):

```
muxt generate

grep 'json.RawMessage' template_routes.go
! grep 'go-json-experiment' template_routes.go

exec go test -race -count=1

-- template.gohtml --
{{- define "POST /raw Lookup(unmarshalJSON(body))" -}}{{- printf "%s" .Result -}}{{- end -}}
-- go.mod --
module server

go 1.25
-- server.go --
package server

import (
	"embed"
	"html/template"
)

//go:embed *.gohtml
var templatesFS embed.FS

var templates = template.Must(template.ParseFS(templatesFS, "*"))
-- server_test.go --
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type server struct{}

func (server) Lookup(raw []byte) []byte { return raw }

func TestRaw(t *testing.T) {
	mux := http.NewServeMux()
	TemplateRoutes(mux, server{})
	req := httptest.NewRequest(http.MethodPost, "/raw", strings.NewReader(`{"x":1}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if got := rec.Body.String(); got != `{"x":1}` {
		t.Fatalf("body = %q", got)
	}
}
```

Note: the receiver interface method's parameter is synthesized as `json.RawMessage`; the hand-written `server.Lookup([]byte)` satisfies it because `json.RawMessage` is `[]byte`.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./cmd/muxt -run 'Test/reference_unmarshal_json_undefined$'`
Expected: FAIL — `createMethodSignature` errors `could not determine a type for unmarshalJSON` (the `*ast.CallExpr` arg has no inferred type yet).

- [ ] **Step 3: Synthesize the raw target type for undefined methods**

In `createMethodSignature` (`routes.go`), the loop currently handles only `*ast.Ident` args; add an `*ast.CallExpr` case that recognizes the input wrappers and appends a parameter of the raw target type:

```go
case *ast.CallExpr:
	if id, ok := arg.Fun.(*ast.Ident); ok && muxt.IsInputWrapper(id.Name) {
		tp, err := inputWrapperTargetType(file, config, id.Name)
		if err != nil {
			return nil, err
		}
		params = append(params, types.NewVar(0, receiver.Obj().Pkg(), "", tp))
		continue
	}
	if err := ensureMethodSignature(file, config, signatures, def, receiver, receiverInterface, arg, templatesPackage); err != nil {
		return nil, err
	}
```

Add `inputWrapperTargetType` to `internal/generate/input_binding.go`:

```go
func inputWrapperTargetType(file *File, config RoutesFileConfiguration, wrapper string) (types.Type, error) {
	switch wrapper {
	case muxt.InputWrapperUnmarshalForm:
		pkg, ok := file.Types("net/url")
		if !ok {
			return nil, fmt.Errorf(`the "net/url" package must be loaded`)
		}
		return pkg.Scope().Lookup("Values").Type(), nil
	default: // unmarshalJSON
		if config.JSONV2 {
			pkg, ok := file.Types("encoding/json/jsontext")
			if !ok {
				return nil, fmt.Errorf(`the "encoding/json/jsontext" package must be loaded`)
			}
			return types.NewPointer(pkg.Scope().Lookup("Decoder").Type()), nil
		}
		pkg, ok := file.Types("encoding/json")
		if !ok {
			return nil, fmt.Errorf(`the "encoding/json" package must be loaded`)
		}
		return pkg.Scope().Lookup("RawMessage").Type(), nil
	}
}
```

- [ ] **Step 4: Pass-through codegen when the target is the raw type**

In `unmarshalJSONStatements`, branch on the target type: if it is `encoding/json.RawMessage`, emit `bodyBytes, err := io.ReadAll(body); …; var v json.RawMessage = bodyBytes`; if it is `*encoding/json/jsontext.Decoder`, emit `v := jsontext.NewDecoder(body)` (no unmarshal). Detect via the type's string/object identity:

```go
if isRawJSONTarget(targetType) { // json.RawMessage
	return []ast.Stmt{ /* bodyBytes, err := io.ReadAll(body); if err {…}; v := json.RawMessage(bodyBytes) */ }, nil
}
if config.JSONV2 && isJSONTextDecoderTarget(targetType) { // *jsontext.Decoder
	return []ast.Stmt{ /* v := jsontext.NewDecoder(body) */ }, nil
}
```

Implement `isRawJSONTarget`/`isJSONTextDecoderTarget` by comparing the named type's package path + name (`encoding/json`.`RawMessage`; `encoding/json/jsontext`.`Decoder`). Write the two code branches in full mirroring the `io.ReadAll` block already in the helper and `astgen.Call(file, "json", "encoding/json/jsontext", "NewDecoder", bodyExpr)`.

- [ ] **Step 5: Run it to verify it passes**

Run: `go test ./cmd/muxt -run 'Test/reference_unmarshal_json_undefined$'`
Expected: PASS (raw `{"x":1}` echoed; generated file references `json.RawMessage`).

- [ ] **Step 6: Commit**

```bash
gofumpt -w internal/ && go test ./internal/... ./cmd/muxt -run 'Test/reference_unmarshal_json' -count=1
git add internal/generate/routes.go internal/generate/input_binding.go cmd/muxt/testdata/reference_unmarshal_json_undefined.txt
git commit -m "feat(generate): unmarshalJSON pass-through (RawMessage/jsontext.Decoder) for undefined methods"
```

---

## Task 5: `unmarshalForm(body)` reusing the form binding

**Files:**
- Modify: `internal/generate/input_binding.go` / `routes.go` (the `InputWrapperUnmarshalForm` branch in the `*ast.CallExpr` handler)
- Test: `cmd/muxt/testdata/reference_unmarshal_form.txt`

- [ ] **Step 1: Write the failing fixture**

Create `cmd/muxt/testdata/reference_unmarshal_form.txt`:

```
muxt generate --use-receiver-type=Server
muxt check

exec go test -race -count=1

-- template.gohtml --
{{- define "POST /greet Greet(ctx, unmarshalForm(body))" -}}Hello, {{ .Result.Name }}!{{- end -}}
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

type GreetForm struct {
	Name string `name:"name"`
}

func (Server) Greet(ctx context.Context, form GreetForm) GreetForm { return form }
-- server_test.go --
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGreet(t *testing.T) {
	mux := http.NewServeMux()
	TemplateRoutes(mux, Server{})
	req := httptest.NewRequest(http.MethodPost, "/greet", strings.NewReader("name=World"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if got := rec.Body.String(); got != "Hello, World!" {
		t.Fatalf("body = %q", got)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./cmd/muxt -run 'Test/reference_unmarshal_form$'`
Expected: FAIL — the `InputWrapperUnmarshalForm` branch is empty (no statements), so the arg local is undefined.

- [ ] **Step 3: Implement the form branch by reusing the existing binding**

In the `case muxt.InputWrapperUnmarshalForm:` of the `*ast.CallExpr` wrapper dispatch, generate the same statements the `form` arg produces. The form code path is `appendParseFormToStructStatements` (struct, `routes.go:1534`) and `formVariableAssignment` (`net/url.Values`). Build a synthetic `*ast.Ident{Name: varIdent}` and `param` (a `types.Var` of `param.Type()`) and call the existing helpers, e.g. for a struct target:

```go
case muxt.InputWrapperUnmarshalForm:
	target := types.NewVar(0, nil, varIdent, param.Type())
	s, err = appendParseFormToStructStatements(nil, def, file, resultType, ast.NewIdent(varIdent), target, validationFailureBlock, rdIdent)
```

(For a `net/url.Values` target, mirror the `formVariableAssignment` + `callParseForm()` path instead. Inspect `appendParseFormToStructStatements`/`formVariableAssignment` signatures and pass the synthetic ident/param so the generated local is `varIdent`.)

- [ ] **Step 4: Run it to verify it passes**

Run: `go test ./cmd/muxt -run 'Test/reference_unmarshal_form$'`
Expected: PASS (`name=World` → `Hello, World!`).

- [ ] **Step 5: Commit**

```bash
gofumpt -w internal/ && go test ./internal/... ./cmd/muxt -run 'Test/reference_unmarshal_form$' -count=1
git add internal/generate/routes.go internal/generate/input_binding.go cmd/muxt/testdata/reference_unmarshal_form.txt
git commit -m "feat(generate): unmarshalForm(body) reusing the form binding"
```

---

## Task 6: Recast non-GET `form` to share the `unmarshalForm` codegen

**Files:**
- Modify: `internal/generate/routes.go` (the `form` handling in `appendParseArgumentStatements`)
- Test: `cmd/muxt/testdata/reference_form_equals_unmarshal_form.txt`

- [ ] **Step 1: Write the fixture asserting equivalence**

Create `cmd/muxt/testdata/reference_form_equals_unmarshal_form.txt` — two routes, one using `form` and one using `unmarshalForm(body)` on POST, asserting identical behavior:

```
muxt generate --use-receiver-type=Server
muxt check

exec go test -race -count=1

-- template.gohtml --
{{- define "POST /a A(ctx, form)" -}}{{ .Result.Name }}{{- end -}}
{{- define "POST /b B(ctx, unmarshalForm(body))" -}}{{ .Result.Name }}{{- end -}}
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

type F struct {
	Name string `name:"name"`
}

func (Server) A(ctx context.Context, form F) F { return form }
func (Server) B(ctx context.Context, form F) F { return form }
-- server_test.go --
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func post(t *testing.T, mux *http.ServeMux, path string) string {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("name=Z"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Body.String()
}

func TestFormEquivalence(t *testing.T) {
	mux := http.NewServeMux()
	TemplateRoutes(mux, Server{})
	if a, b := post(t, mux, "/a"), post(t, mux, "/b"); a != "Z" || a != b {
		t.Fatalf("a=%q b=%q", a, b)
	}
}
```

- [ ] **Step 2: Run it to verify it passes (or fails)**

Run: `go test ./cmd/muxt -run 'Test/reference_form_equals_unmarshal_form$'`
Expected: With Task 5 done, both routes already produce `Z` — this fixture is a regression guard. If it passes immediately, the recast in Step 3 is the cleanup; if behaviors differ, Step 3 reconciles them.

- [ ] **Step 3: Route non-GET `form` through the shared helper**

Confirm the `form` arg (`appendParseFormToStructStatements` / `formVariableAssignment`) and the `unmarshalForm` branch call the **same** helper functions so the two are literally one code path (they already are, since Task 5 reused them). If any divergence remains (e.g. the `form` path special-cases GET), leave the GET branch untouched and ensure non-GET `form` and `unmarshalForm(body)` share the helper. No behavior change for GET `form`.

- [ ] **Step 4: Run the whole datastar/form fixture set**

Run: `go test ./cmd/muxt -run 'Test/(reference_form|reference_unmarshal|howto_form)' -count=1`
Expected: PASS (existing `howto_form_*` fixtures still pass — no regression to GA `form`).

- [ ] **Step 5: Commit**

```bash
gofumpt -w internal/ && go test ./cmd/muxt -count=1
git add internal/generate/routes.go cmd/muxt/testdata/reference_form_equals_unmarshal_form.txt
git commit -m "refactor(generate): non-GET form shares unmarshalForm codegen"
```

---

## Task 7: Error fixture + docs

**Files:**
- Create: `cmd/muxt/testdata/err_unmarshal_json_bad_arg.txt`
- Modify: `docs/reference/call-parameters.md`

- [ ] **Step 1: Error fixture for a non-`body` wrapper argument**

Create `cmd/muxt/testdata/err_unmarshal_json_bad_arg.txt`:

```
! muxt generate --use-receiver-type=Server
stderr 'unmarshalJSON argument must be body'

-- template.gohtml --
{{- define "POST /x F(ctx, unmarshalJSON(ctx))" -}}{{- .Result -}}{{- end -}}
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

func (Server) F(ctx context.Context, v any) any { return v }
```

- [ ] **Step 2: Run it to verify it passes**

Run: `go test ./cmd/muxt -run 'Test/err_unmarshal_json_bad_arg$'`
Expected: PASS (the `checkArguments` validation from Task 2 produces the error).

- [ ] **Step 3: Document the wrappers**

In `docs/reference/call-parameters.md`, add rows/section for `body` (`io.Reader`, source `request.Body`) and the `unmarshalJSON(body)` / `unmarshalForm(body)` wrappers (note: target type from the method parameter; undefined → `json.RawMessage` / `*jsontext.Decoder`; `form` non-GET is sugar for `unmarshalForm(body)`; `--output-jsonv2` uses `UnmarshalRead`).

- [ ] **Step 4: Verify docs + full suite**

Run: `go test ./docs/ ./cmd/muxt ./internal/... -count=1`
Expected: PASS (docs link check + all fixtures).

- [ ] **Step 5: Commit**

```bash
git add cmd/muxt/testdata/err_unmarshal_json_bad_arg.txt docs/reference/call-parameters.md
git commit -m "docs(generate): document body and unmarshalJSON/unmarshalForm; add bad-arg error fixture"
```

---

## Final verification

- [ ] `gofumpt -l internal/ docs/` is clean; `go vet ./...` clean.
- [ ] `go test ./...` green.
- [ ] No stray `datastar-counter` binary staged (the examples are `package main`; `rm -f docs/examples/datastar-counter/datastar-counter` before committing if present).
- [ ] Spec coverage: `body` (Task 1), `unmarshalJSON` defined (2) + jsonv2 (3) + undefined pass-through (4), `unmarshalForm` (5), `form` recast (6), bad-arg error + docs (7). `signals` intentionally deferred.
