# Phase 3b — datastar framing + arg-family removal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. **This is a muxt project — dispatched agents must use the muxt skills** (`muxt_test-driven-development`, `muxt_refactoring`, `muxt_explore`, `muxt_datastar`).
>
> **Project preferences (enforce in every task):**
> 1. Generate Go code with functions that **build go/ast nodes and return concrete static node types** (`*ast.FuncDecl`, `*ast.GenDecl`, `[]ast.Decl`). Do NOT introduce `parser.ParseFile`-from-string-const patterns.
> 2. txtar fixtures assert **behavior, not generated source text** — use `muxt check` for static type-validation of template calls and `exec go test` over `httptest` for runtime behavior (status, `Content-Type`, `datastar-*` headers, SSE `event:`/`data:` bytes, JSON bodies). **No `grep`/`! grep` assertions over `template_routes.go`.**
> 3. After edits, run `go test ./...`; format with `gofumpt -w`; rename Go symbols with `gopls rename` (never sed/replace_all). Commit messages end with `Assisted-by: Claude:claude-opus-4-8 [tools]` — NO `Co-Authored-By`/`Signed-off-by`.

**Goal:** Add the `datastar(...)` framing wrapper composing with the Phase 2 `sse()`/`marshalJSON()` representation wrappers, and delete the dev-only `elements`/`signal`/`script` reserved-argument families so datastar codegen is driven by framing + representation only.

**Architecture:** Mirror Phase 3a's `htmx(...)`. A `FramingDatastar` enum value is recognized and stripped in `parseHandler` before the representation wrapper. Three datastar template-data types are emitted per-framing: `DatastarTemplateData` (html render: `.Actions()` + `datastar-selector`/`mode`/`use-view-transition` header setters), `DatastarEventTemplateData` (patch-elements stream, retained), `DatastarSignalsTemplateData` (non-streaming `application/json` body + `datastar-only-if-missing` header). The new path is built and tested via **explicit `datastar(...)` wrappers** (Tasks 3–5, `config.Datastar` false → old arg path dormant, existing fixtures green), then `--use-datastar` is flipped to auto-wrap and the old arg families are deleted in one swap (Task 6).

**Tech Stack:** Go `go/ast`, `go/types`, `go/parser`; muxt generator (`internal/muxt`, `internal/generate`, `internal/cli`); txtar fixtures in `cmd/muxt/testdata/` run by `Test` in `cmd/muxt/main_test.go` (rsc.io/script).

**Test command reference:** one fixture: `go test ./cmd/muxt -run 'Test/<basename>'`; package: `go test ./cmd/muxt`; unit: `go test ./internal/muxt ./internal/cli`; all: `go test ./...`.

**Spec:** `docs/superpowers/specs/2026-06-10-call-dsl-phase3b-datastar-framing-design.md`.

---

## File Structure

- **`internal/muxt/definition.go`** — parser. Task 1 adds `FramingDatastar` + `FramingWrapperDatastar` + extends the framing strip block. Task 6 deletes `IsElementsArgument`/`IsSignalArgument`/`IsScriptArgument`/`IsDatastarArgument`, the `UsesElements/Signal/Script/Datastar` accessors, the `elements`/`signal`/`script` scope constants, and their `checkArguments` handling.
- **`internal/cli/commands.go`** — Task 2 adds three `--output-datastar-*-type` flags (validation + round-trip). Task 6 repoints datastar defaulting onto them.
- **`internal/generate/routes.go`** — `effectiveFraming`/`renderTemplateDataType` datastar branches; per-framing emission datastar branch; `methodHandlerFunc` representation×framing dispatch.
- **`internal/generate/datastar_template_data.go`** (new) — `DatastarTemplateData` render-type builder (`.Actions()` wired in + `.Selector`/`.Mode`/`.UseViewTransition` header setters + base) and `DatastarSignalsTemplateData` builder (`.OnlyIfMissing()`).
- **`internal/generate/datastar_event_template_data.go`** — `DatastarEventTemplateData` (retained).
- **`internal/generate/datastar_signals.go`** — `datastarMarshalSignals` (retained).
- **`internal/generate/datastar_actions.go`** — `.Actions()`/builders (retained; accessor emission moves to per-framing).
- **`internal/generate/datastar_handlers.go`** — Task 5 adds the framing-driven datastar stream/signals handlers; Task 6 deletes the arg-driven `datastarMethodHandlerFunc` dispatch and shape handlers.
- **`cmd/muxt/testdata/`** — new `reference_datastar_framing_*.txt`; Task 6 migrates existing `reference_datastar_*.txt`, deletes `reference_datastar_script.txt`.
- **`docs/reference/`, `docs/explanation/decisions/00007_framing_wrappers.md`** — Task 7.

---

## Task 1: Parser — recognize and strip `datastar(...)`

**Files:**
- Modify: `internal/muxt/definition.go` (the `Framing` enum + `FramingWrapperHTMX` const area; the framing strip block in `parseHandler`)
- Test: `internal/muxt/definition_internal_test.go`

- [ ] **Step 1: Write the failing unit test**

Add to `internal/muxt/definition_internal_test.go` (package `muxt`):

```go
func TestDefinitionFramingDatastar(t *testing.T) {
	for _, tt := range []struct {
		name string
		want Framing
		rep  Representation
		fun  string
	}{
		{name: "GET /a datastar(Index(ctx))", want: FramingDatastar, rep: RepresentationNone, fun: "Index"},
		{name: "GET /b datastar(sse(Stream(ctx, send)))", want: FramingDatastar, rep: RepresentationSSE, fun: "Stream"},
		{name: "GET /c datastar(marshalJSON(Signals(ctx)))", want: FramingDatastar, rep: RepresentationMarshalJSON, fun: "Signals"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			def, err, ok := newDefinition(template.New(tt.name))
			if !ok || err != nil {
				t.Fatalf("newDefinition(%q) ok=%v err=%v", tt.name, ok, err)
			}
			if got := def.Framing(); got != tt.want {
				t.Errorf("Framing() = %v, want %v", got, tt.want)
			}
			if got := def.Representation(); got != tt.rep {
				t.Errorf("Representation() = %v, want %v", got, tt.rep)
			}
			if def.FunctionIdentifier().Name != tt.fun {
				t.Errorf("FunctionIdentifier() = %q, want %q", def.FunctionIdentifier().Name, tt.fun)
			}
		})
	}
}

func TestDefinitionDatastarBadArity(t *testing.T) {
	for _, in := range []string{"GET /a datastar()", "GET /b datastar(Index(ctx), Other(ctx))"} {
		if _, err, _ := newDefinition(template.New(in)); err == nil {
			t.Fatalf("newDefinition(%q): want error for bad datastar arity", in)
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/muxt -run 'TestDefinitionFramingDatastar|TestDefinitionDatastarBadArity'`
Expected: FAIL — `FramingDatastar` undefined.

- [ ] **Step 3: Add the enum value and reserved name**

In `internal/muxt/definition.go`, find the `Framing` enum (currently `FramingNone`, `FramingHTMX`) and the `FramingWrapperHTMX = "htmx"` const. Add:

```go
const (
	FramingNone Framing = iota
	FramingHTMX
	FramingDatastar
)
```

```go
// FramingWrapperDatastar is the reserved outer framing-wrapper function name for datastar.
const FramingWrapperDatastar = "datastar"
```

- [ ] **Step 4: Extend the framing strip block in `parseHandler`**

Find the framing strip block added in Phase 3a (it tests `if fun.Name == FramingWrapperHTMX`). Generalize it to recognize both wrapper names. Replace the `if fun.Name == FramingWrapperHTMX { ... }` block with:

```go
	framing := FramingNone
	switch fun.Name {
	case FramingWrapperHTMX:
		framing = FramingHTMX
	case FramingWrapperDatastar:
		framing = FramingDatastar
	}
	if framing != FramingNone {
		if len(call.Args) != 1 {
			return fmt.Errorf("%s takes exactly one argument: the wrapped call", fun.Name)
		}
		inner, ok := call.Args[0].(*ast.CallExpr)
		if !ok {
			return fmt.Errorf("%s argument must be a call", fun.Name)
		}
		innerFun, ok := inner.Fun.(*ast.Ident)
		if !ok {
			return fmt.Errorf("expected function identifier, got: %s", astgen.Format(inner.Fun))
		}
		call, fun = inner, innerFun
	}
	def.framing = framing
```

(Keep the surrounding code — the representation unwrap and `checkArguments` — unchanged; they run on the unwrapped `call`/`fun`.)

- [ ] **Step 5: Run the unit tests**

Run: `go test ./internal/muxt -run 'TestDefinitionFraming|TestDefinitionHTMXBadArity|TestDefinitionDatastarBadArity'`
Expected: PASS (htmx cases still pass; datastar cases now pass).

Run: `go test ./...`
Expected: PASS (additive; nothing dispatches on `FramingDatastar` yet — `effectiveFraming` has no datastar branch, so explicit `datastar(...)` routes would currently render as `FramingNone`; no fixture uses an explicit `datastar(...)` wrapper yet).

- [ ] **Step 6: Commit**

```bash
gofumpt -w internal/muxt/definition.go internal/muxt/definition_internal_test.go
git add internal/muxt/definition.go internal/muxt/definition_internal_test.go
git commit -m "feat(muxt): recognize datastar() framing wrapper

Assisted-by: Claude:claude-opus-4-8 gofumpt"
```

---

## Task 2: CLI — three `--output-datastar-*-type` flags

**Files:**
- Modify: `internal/generate/routes.go` (`RoutesFileConfiguration`: add three fields)
- Modify: `internal/cli/commands.go` (consts, flag registration, `applyDefaults`, identifier validation, `configToArgs`)
- Test: `internal/cli/commands_test.go`

- [ ] **Step 1: Add config fields**

In `RoutesFileConfiguration` (`internal/generate/routes.go`, near `HTMXTemplateDataType`), add:

```go
	DatastarTemplateDataType        string
	DatastarEventTemplateDataType   string
	DatastarSignalsTemplateDataType string
```

- [ ] **Step 2: Write the failing CLI unit test**

Add to `internal/cli/commands_test.go`:

```go
func TestApplyDefaults_DatastarTemplateDataTypes(t *testing.T) {
	newFlagSet := func(cfg *generate.RoutesFileConfiguration) *pflag.FlagSet {
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		addOutputFlagsToFlagSet(fs, cfg)
		return fs
	}
	t.Run("defaults", func(t *testing.T) {
		cfg := generate.RoutesFileConfiguration{OutputExportedDefaultIdentifiers: true}
		fs := newFlagSet(&cfg)
		applyDefaults(&cfg, fs)
		assert.Equal(t, defaultDatastarTemplateDataTypeName, cfg.DatastarTemplateDataType)
		assert.Equal(t, defaultDatastarEventTemplateDataTypeName, cfg.DatastarEventTemplateDataType)
		assert.Equal(t, defaultDatastarSignalsTemplateDataTypeName, cfg.DatastarSignalsTemplateDataType)
	})
	t.Run("round-trip forwards non-default", func(t *testing.T) {
		args := configToArgs(generate.RoutesFileConfiguration{
			DatastarSignalsTemplateDataType: "Sig",
			DatastarTemplateDataType:        defaultDatastarTemplateDataTypeName,
			DatastarEventTemplateDataType:   defaultDatastarEventTemplateDataTypeName,
		})
		assert.Contains(t, args, "--"+outputDatastarSignalsTemplateDataType+"=Sig")
	})
}
```

- [ ] **Step 2b: Run it to verify it fails**

Run: `go test ./internal/cli -run TestApplyDefaults_DatastarTemplateDataTypes`
Expected: FAIL — undefined consts.

- [ ] **Step 3: Add consts and defaults**

In `internal/cli/commands.go`, near `outputHTMXTemplateDataType` and `defaultHTMXTemplateDataTypeName`, add:

```go
	outputDatastarTemplateDataType        = "output-datastar-template-data-type"
	outputDatastarEventTemplateDataType   = "output-datastar-event-template-data-type"
	outputDatastarSignalsTemplateDataType = "output-datastar-signals-template-data-type"
```

```go
	outputDatastarTemplateDataTypeHelp        = "The type name for the template data passed to datastar-framed (text/html) route templates."
	outputDatastarEventTemplateDataTypeHelp   = "The type name for the per-event template data in datastar(sse(...)) patch-elements streams."
	outputDatastarSignalsTemplateDataTypeHelp = "The type name for the template data in non-streaming datastar(marshalJSON(...)) routes."
```

The default-name consts already exist (`defaultDatastarTemplateDataTypeName = "DatastarTemplateData"`, `defaultDatastarEventTemplateDataTypeName = "DatastarEventTemplateData"`). Add the signals one near them:

```go
	defaultDatastarSignalsTemplateDataTypeName = "DatastarSignalsTemplateData"
```

- [ ] **Step 4: Register flags**

In the function that registers output flags (where `--output-htmx-template-data-type` is registered, `addOutputFlagsToFlagSet`), add (empty-string default; `applyDefaults` controls the effective default):

```go
	flagSet.StringVar(&g.DatastarTemplateDataType, outputDatastarTemplateDataType, "", outputDatastarTemplateDataTypeHelp)
	flagSet.StringVar(&g.DatastarEventTemplateDataType, outputDatastarEventTemplateDataType, "", outputDatastarEventTemplateDataTypeHelp)
	flagSet.StringVar(&g.DatastarSignalsTemplateDataType, outputDatastarSignalsTemplateDataType, "", outputDatastarSignalsTemplateDataTypeHelp)
```

- [ ] **Step 5: Default in `applyDefaults`**

In `applyDefaults`, after the htmx defaulting, mirror it for the three datastar flags using `cmp.Or` against the default-name consts (exported branch), and the `strcase.ToGoCamel(...)` gated form in the private/camelCase branch — match exactly how `HTMXTemplateDataType` is defaulted in both branches. Example for the exported branch:

```go
	config.DatastarTemplateDataType = cmp.Or(config.DatastarTemplateDataType, defaultDatastarTemplateDataTypeName)
	config.DatastarEventTemplateDataType = cmp.Or(config.DatastarEventTemplateDataType, defaultDatastarEventTemplateDataTypeName)
	config.DatastarSignalsTemplateDataType = cmp.Or(config.DatastarSignalsTemplateDataType, defaultDatastarSignalsTemplateDataTypeName)
```

(In the private branch, gate each on `!flagSet.Changed(<flag>)` and apply `strcase.ToGoCamel(<defaultName>)`, identical to `HTMXTemplateDataType`.)

- [ ] **Step 6: Validate as identifiers + round-trip**

In the identifier-validation block (where `config.HTMXTemplateDataType` is checked with `token.IsIdentifier`), add three sibling checks:

```go
	if config.DatastarTemplateDataType != "" && !token.IsIdentifier(config.DatastarTemplateDataType) {
		return fmt.Errorf(outputDatastarTemplateDataType + errIdentSuffix)
	}
	if config.DatastarEventTemplateDataType != "" && !token.IsIdentifier(config.DatastarEventTemplateDataType) {
		return fmt.Errorf(outputDatastarEventTemplateDataType + errIdentSuffix)
	}
	if config.DatastarSignalsTemplateDataType != "" && !token.IsIdentifier(config.DatastarSignalsTemplateDataType) {
		return fmt.Errorf(outputDatastarSignalsTemplateDataType + errIdentSuffix)
	}
```

In `configToArgs`, after the `HTMXTemplateDataType` forwarding, add:

```go
	if config.DatastarTemplateDataType != defaultDatastarTemplateDataTypeName {
		args = append(args, "--"+outputDatastarTemplateDataType+"="+config.DatastarTemplateDataType)
	}
	if config.DatastarEventTemplateDataType != defaultDatastarEventTemplateDataTypeName {
		args = append(args, "--"+outputDatastarEventTemplateDataType+"="+config.DatastarEventTemplateDataType)
	}
	if config.DatastarSignalsTemplateDataType != defaultDatastarSignalsTemplateDataTypeName {
		args = append(args, "--"+outputDatastarSignalsTemplateDataType+"="+config.DatastarSignalsTemplateDataType)
	}
```

- [ ] **Step 7: Verify**

Run: `go test ./internal/cli -run TestApplyDefaults_DatastarTemplateDataTypes` → PASS.
Run: `go test ./...` → PASS (additive; codegen does not read these fields yet).

- [ ] **Step 8: Commit**

```bash
gofumpt -w internal/cli/commands.go internal/generate/routes.go internal/cli/commands_test.go
git add internal/cli/commands.go internal/generate/routes.go internal/cli/commands_test.go
git commit -m "feat(cli): add --output-datastar-*-template-data-type flags

Assisted-by: Claude:claude-opus-4-8 gofumpt"
```

---

## Task 3: `DatastarTemplateData` render type + explicit `datastar(Method(ctx))` render

This task wires the **html render** datastar path for an **explicit** `datastar(...)` wrapper (no `--use-datastar`), so the old arg path stays dormant and existing fixtures stay green.

**Files:**
- Create: `internal/generate/datastar_template_data.go`
- Modify: `internal/generate/routes.go` (`effectiveFraming`, `renderTemplateDataType`, per-framing emission)
- Test: `cmd/muxt/testdata/reference_datastar_framing_render.txt`

- [ ] **Step 1: Write the failing behavioral fixture**

`cmd/muxt/testdata/reference_datastar_framing_render.txt`:

```
muxt generate --use-receiver-type=Server
muxt check

exec go test -race -count=1

-- template.gohtml --
{{- define "GET /widget Widget(ctx)" -}}{{- .Selector "#target" -}}{{- .Mode "inner" -}}{{- .UseViewTransition true -}}<div>{{- .Result -}}</div>{{- end -}}
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

func (Server) Widget(ctx context.Context) string { return "hi" }
-- server_test.go --
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDatastarRenderHeaders(t *testing.T) {
	mux := http.NewServeMux()
	TemplateRoutes(mux, Server{})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/widget", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("datastar-selector"); got != "#target" {
		t.Errorf("datastar-selector = %q, want %q", got, "#target")
	}
	if got := rec.Header().Get("datastar-mode"); got != "inner" {
		t.Errorf("datastar-mode = %q, want %q", got, "inner")
	}
	if got := rec.Header().Get("datastar-use-view-transition"); got != "true" {
		t.Errorf("datastar-use-view-transition = %q, want %q", got, "true")
	}
	if body := rec.Body.String(); !strings.Contains(body, "hi") {
		t.Errorf("body = %q, want it to contain %q", body, "hi")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./cmd/muxt -run 'Test/reference_datastar_framing_render'`
Expected: FAIL — `muxt check` errors that `.Selector`/`.Mode`/`.UseViewTransition` are not found on the render type (the route renders with minimal `TemplateData` because `effectiveFraming` has no datastar branch and `DatastarTemplateData` is not emitted).

- [ ] **Step 3: Build the `DatastarTemplateData` render-type decls**

Create `internal/generate/datastar_template_data.go`. Mirror the existing HTML patch-header setters on `DatastarEventTemplateData` (`datastar_event_template_data.go`, methods `.Selector`/`.Mode`/`.UseViewTransition`) but emit **response headers** via the base `.Header(name, value)` helper (the same pattern `htmxHeaderSetterMethod` uses in `template_data.go`). The render type is the base template-data decls (from `templateDataDecls(file, config, typeName, false)`) plus `.Actions()` (via `datastarActionsAccessorMethod`) plus the three header setters:

```go
package generate

import (
	"go/ast"

	"github.com/typelate/muxt/internal/astgen"
)

// datastarRenderHeaderSetterMethod builds a chainable method that sets a
// datastar-* response header from a string argument and returns the receiver,
// rendering as "" via String() (same pattern as htmxHeaderSetterMethod).
func datastarRenderHeaderSetterMethod(typeName, methodName, headerName, paramName string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(typeName),
		Name: ast.NewIdent(methodName),
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(paramName)}, Type: ast.NewIdent("string")}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: &ast.StarExpr{X: &ast.IndexListExpr{
				X:       ast.NewIdent(typeName),
				Indices: []ast.Expr{ast.NewIdent("R"), ast.NewIdent("T")},
			}}}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(templateDataReceiverName), Sel: ast.NewIdent("Header")},
			Args: []ast.Expr{astgen.String(headerName), ast.NewIdent(paramName)},
		}}}}},
	}
}
```

Add a `.UseViewTransition(bool)` variant: same shape, param type `bool`, body sets the header to `strconv.FormatBool(useViewTransition)` — reuse the existing helper that the `DatastarEventTemplateData.UseViewTransition` setter uses to stringify a bool (read `datastar_event_template_data.go` for the exact expression and mirror it; if it inlines `"true"`, accept a bool and call `strconv.FormatBool`). Then the aggregator:

```go
// datastarTemplateDataDecls returns the datastar html-render template-data type
// (typeName): the base template-data methods plus .Actions() and the
// datastar-selector / datastar-mode / datastar-use-view-transition header setters.
func datastarTemplateDataDecls(file *File, config RoutesFileConfiguration, typeName string) []ast.Decl {
	decls := templateDataDecls(file, config, typeName, false)
	decls = append(decls,
		datastarActionsAccessorMethod(config, typeName),
		datastarRenderHeaderSetterMethod(typeName, "Selector", "datastar-selector", "selector"),
		datastarRenderHeaderSetterMethod(typeName, "Mode", "datastar-mode", "mode"),
		datastarRenderUseViewTransitionMethod(typeName),
	)
	return decls
}
```

NOTE: `datastarActionsAccessorMethod` currently takes only `config` and reads `config.TemplateDataType` for its receiver (`datastar_actions.go:61`). Add a `typeName string` parameter and use it for the receiver, mirroring the Phase 3a `typeName` parameterization of the template-data builders. Update its existing call site in `datastarActionsDecls` to pass `config.TemplateDataType` (preserving current behavior for the old path until Task 6).

- [ ] **Step 4: Wire framing selection + emission in `routes.go`**

In `effectiveFraming`, add the datastar branch (keep explicit-wins first):

```go
func effectiveFraming(config RoutesFileConfiguration, def muxt.Definition) muxt.Framing {
	if def.Framing() != muxt.FramingNone {
		return def.Framing()
	}
	if config.HTMXHelpers {
		return muxt.FramingHTMX
	}
	// NOTE: the config.Datastar auto-wrap branch is added in Task 6, after the old
	// arg-driven datastar path is removed. Until then only explicit datastar(...)
	// wrappers reach the datastar framing.
	return muxt.FramingNone
}
```

In `renderTemplateDataType`, add:

```go
	if framing == muxt.FramingDatastar {
		return config.DatastarTemplateDataType
	}
```

In the per-framing emission block (the non-`config.Datastar` else-branch from Phase 3a), add a datastar arm using the same `slices.ContainsFunc(routeDefinitions, ...)` pattern:

```go
		usesDatastar := slices.ContainsFunc(routeDefinitions, func(d muxt.Definition) bool { return effectiveFraming(config, d) == muxt.FramingDatastar })
		if usesDatastar {
			decls = append(decls, datastarTemplateDataDecls(file, config, config.DatastarTemplateDataType)...)
		}
```

(Leave the existing `if config.Datastar { ... }` arm and the old `datastarActionsDecls`/`datastarEventTemplateDataDecls`/`datastarSignalsDecls` appends untouched — they serve the old arg path until Task 6. Explicit-wrapper fixtures here have `config.Datastar == false`, so the old arm is dormant and only the new datastar emission fires.)

The plain-render dispatch already uses `ast.NewIdent(renderTemplateDataType(config, effectiveFraming(config, def)))` (Phase 3a). So a `datastar(Method(ctx))` route now renders with `DatastarTemplateData`. No `methodHandlerFunc` change needed for the render path.

- [ ] **Step 5: Run the fixture**

Run: `go test ./cmd/muxt -run 'Test/reference_datastar_framing_render'`
Expected: PASS.

- [ ] **Step 6: Full suite**

Run: `go test ./...`
Expected: PASS — existing `reference_datastar_*` fixtures use `--use-datastar` (no explicit wrapper), so their routes are `FramingNone`, the new datastar emission does not fire, and the old path is unchanged.

- [ ] **Step 7: Commit**

```bash
gofumpt -w internal/generate/datastar_template_data.go internal/generate/routes.go internal/generate/datastar_actions.go
git add internal/generate/datastar_template_data.go internal/generate/routes.go internal/generate/datastar_actions.go cmd/muxt/testdata/reference_datastar_framing_render.txt
git commit -m "feat(generate): datastar(Method) renders DatastarTemplateData with patch headers

Assisted-by: Claude:claude-opus-4-8 gofumpt"
```

---

## Task 4: `DatastarSignalsTemplateData` + `datastar(marshalJSON(Method(...)))`

**Files:**
- Modify: `internal/generate/datastar_template_data.go` (add `DatastarSignalsTemplateData` builder)
- Modify: `internal/generate/routes.go` (per-framing emission; `methodHandlerFunc` representation×framing dispatch for marshalJSON+datastar)
- Modify: `internal/generate/datastar_handlers.go` (new framing-driven signals handler)
- Test: `cmd/muxt/testdata/reference_datastar_framing_signals.txt`

- [ ] **Step 1: Write the failing behavioral fixture**

`cmd/muxt/testdata/reference_datastar_framing_signals.txt`:

```
muxt generate --use-receiver-type=Server
muxt check

exec go test -race -count=1

-- template.gohtml --
{{- define "POST /signals datastar(marshalJSON(Signals(ctx)))" -}}{{- .OnlyIfMissing -}}{{- end -}}
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

type Counts struct {
	N int `json:"n"`
}

func (Server) Signals(ctx context.Context) Counts { return Counts{N: 7} }
-- server_test.go --
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDatastarSignals(t *testing.T) {
	mux := http.NewServeMux()
	TemplateRoutes(mux, Server{})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/signals", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
	if got := rec.Header().Get("datastar-only-if-missing"); got != "true" {
		t.Fatalf("datastar-only-if-missing = %q, want true", got)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != `{"n":7}` {
		t.Fatalf("body = %q, want %q", body, `{"n":7}`)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./cmd/muxt -run 'Test/reference_datastar_framing_signals'`
Expected: FAIL — `.OnlyIfMissing` not found on the render type / no datastar-signals handler.

- [ ] **Step 3: Build `DatastarSignalsTemplateData`**

In `internal/generate/datastar_template_data.go`, add a builder for the minimal signals type. It is the base template-data shape (so `.Result`/`.Request`/`.Receiver`/`.Header` are available for conditions) plus a single nullary chainable `.OnlyIfMissing()` setter that sets the `datastar-only-if-missing: true` header (mirror `htmxRefreshMethod` in `template_data.go` — a nullary setter returning the receiver):

```go
func datastarOnlyIfMissingMethod(typeName string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(typeName),
		Name: ast.NewIdent("OnlyIfMissing"),
		Type: &ast.FuncType{Results: &ast.FieldList{List: []*ast.Field{{Type: &ast.StarExpr{X: &ast.IndexListExpr{
			X:       ast.NewIdent(typeName),
			Indices: []ast.Expr{ast.NewIdent("R"), ast.NewIdent("T")},
		}}}}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(templateDataReceiverName), Sel: ast.NewIdent("Header")},
			Args: []ast.Expr{astgen.String("datastar-only-if-missing"), astgen.String("true")},
		}}}}},
	}
}

func datastarSignalsTemplateDataDecls(file *File, config RoutesFileConfiguration, typeName string) []ast.Decl {
	decls := templateDataDecls(file, config, typeName, false)
	return append(decls, datastarOnlyIfMissingMethod(typeName))
}
```

- [ ] **Step 4: Emit the type per-framing**

In `routes.go`, in the per-framing datastar arm (Task 3 Step 4), also emit the signals type when any datastar route uses the marshalJSON representation:

```go
		usesDatastarSignals := slices.ContainsFunc(routeDefinitions, func(d muxt.Definition) bool {
			return effectiveFraming(config, d) == muxt.FramingDatastar && d.Representation() == muxt.RepresentationMarshalJSON
		})
		if usesDatastarSignals {
			decls = append(decls, datastarSignalsTemplateDataDecls(file, config, config.DatastarSignalsTemplateDataType)...)
		}
```

- [ ] **Step 5: Dispatch the signals handler**

In `methodHandlerFunc`, the Phase 2 marshalJSON path returns early for `def.Representation() == muxt.RepresentationMarshalJSON`. Before that return, branch on framing: when `effectiveFraming(config, def) == muxt.FramingDatastar`, generate the **datastar signals handler** instead of the plain `application/json` handler. The datastar signals handler:

1. evaluates the route's `define` body once with a `DatastarSignalsTemplateData` value (so `.OnlyIfMissing` can set the header) — render into an ignored buffer, but call before writing the body;
2. marshals the method result to JSON (reuse the Phase 2 marshalJSON body-writing — `encoding/json` or `encoding/json/v2` under `--output-jsonv2`);
3. sets `Content-Type: application/json` and writes the JSON.

Implement `datastarSignalsHandlerFunc(file, config, def, ...)` in `datastar_handlers.go` modeled on the existing Phase 2 marshalJSON handler builder (find it — it builds the `application/json` write) but inserting the template-execution-for-header-side-effects step using `DatastarSignalsTemplateData`. Reuse the existing template-execution AST helper used by the normal render path (the call that runs `templates.ExecuteTemplate(buf, def.templateName, data)`), passing a `config.DatastarSignalsTemplateDataType` value and discarding `buf`. Header writes must precede the body write (set the header, run the template for side effects, then `w.Header().Set("Content-Type", "application/json")` and write).

Wire the dispatch in `methodHandlerFunc`:

```go
	if def.Representation() == muxt.RepresentationMarshalJSON {
		if effectiveFraming(config, def) == muxt.FramingDatastar {
			return datastarSignalsHandlerFunc(file, config, def, /* same args the marshalJSON builder takes */)
		}
		return marshalJSONMethodHandlerFunc(/* existing */)
	}
```

(Use the actual existing function name for the Phase 2 marshalJSON handler builder; read `routes.go`/`marshal_json.go` to confirm.)

- [ ] **Step 6: Run the fixture, then full suite**

Run: `go test ./cmd/muxt -run 'Test/reference_datastar_framing_signals'` → PASS.
Run: `go test ./...` → PASS (existing fixtures unaffected; `config.Datastar` false here).

- [ ] **Step 7: Commit**

```bash
gofumpt -w internal/generate/datastar_template_data.go internal/generate/routes.go internal/generate/datastar_handlers.go
git add internal/generate/datastar_template_data.go internal/generate/routes.go internal/generate/datastar_handlers.go cmd/muxt/testdata/reference_datastar_framing_signals.txt
git commit -m "feat(generate): datastar(marshalJSON) emits JSON body + only-if-missing header

Assisted-by: Claude:claude-opus-4-8 gofumpt"
```

---

## Task 5: `datastar(sse(...))` patch-elements stream + inline patch-signals

This is the largest task: route the Phase 2 SSE stream transport through the datastar marshaler (`DatastarEventTemplateData` for `send`/`sendX` element frames; `datastarMarshalSignals` for `marshalJSON(sendX)` signal frames).

**Files:**
- Modify: `internal/generate/routes.go` (per-framing emission of `DatastarEventTemplateData`; SSE handler dispatch parameterized by framing)
- Modify: `internal/generate/datastar_handlers.go` / the Phase 2 SSE handler builder (inject the datastar event type + signal marshaler)
- Test: `cmd/muxt/testdata/reference_datastar_framing_elements.txt`, `reference_datastar_framing_signals_stream.txt`

- [ ] **Step 1: Write the failing behavioral fixtures**

`cmd/muxt/testdata/reference_datastar_framing_elements.txt` — `datastar(sse(Stream(ctx, send)))`; `send` renders the define body as a `datastar-patch-elements` frame; `.Selector`/`.Mode` set the frame metadata lines:

```
muxt generate --use-receiver-type=Server
muxt check

exec go test -race -count=1

-- template.gohtml --
{{- define "GET /stream datastar(sse(Stream(ctx, send)))" -}}{{- .Selector "#list" -}}{{- .Mode "append" -}}<li>{{- .Result -}}</li>{{- end -}}
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

func (Server) Stream(ctx context.Context, send func(item string) error) {
	_ = send("a")
}
-- server_test.go --
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDatastarElementsStream(t *testing.T) {
	mux := http.NewServeMux()
	TemplateRoutes(mux, Server{})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/stream", nil))

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"event: datastar-patch-elements",
		"data: selector #list",
		"data: mode append",
		"data: elements <li>a</li>",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want it to contain %q", body, want)
		}
	}
}
```

`cmd/muxt/testdata/reference_datastar_framing_signals_stream.txt` — inline `marshalJSON(sendSig)` send with the per-frame `onlyIfMissing` bool:

```
muxt generate --use-receiver-type=Server
muxt check

exec go test -race -count=1

-- template.gohtml --
{{- define "GET /mixed datastar(sse(Mixed(ctx, send, marshalJSON(sendSignals))))" -}}<li>{{- .Result -}}</li>{{- end -}}
{{- define "sendSignals" -}}{{- end -}}
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

type Sig struct {
	Count int `json:"count"`
}

// Mixed sends one element frame and one signals frame (onlyIfMissing=true).
func (Server) Mixed(ctx context.Context, send func(item string) error, sendSignals func(v Sig, onlyIfMissing bool) error) {
	_ = send("x")
	_ = sendSignals(Sig{Count: 1}, true)
}
-- server_test.go --
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDatastarMixedStream(t *testing.T) {
	mux := http.NewServeMux()
	TemplateRoutes(mux, Server{})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/mixed", nil))

	body := rec.Body.String()
	for _, want := range []string{
		"event: datastar-patch-elements",
		"data: elements <li>x</li>",
		"event: datastar-patch-signals",
		`data: signals {"count":1}`,
		"data: onlyIfMissing true",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want it to contain %q", body, want)
		}
	}
}
```

NOTE: verify the exact patch-signals data-line syntax (`data: signals <json>` and the onlyIfMissing line) emitted by the existing datastar signal stream code in `datastar_handlers.go` (`signalEventClosure`) and match the fixture assertions to what the retained marshaler actually writes. Adjust the `want` substrings to the real output.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./cmd/muxt -run 'Test/reference_datastar_framing_elements|Test/reference_datastar_framing_signals_stream'`
Expected: FAIL — datastar SSE framing not wired; the routes currently stream via the generic `SSETemplateData` marshaler (plain `data:` lines), and `.Selector`/`.Mode` aren't on the generic SSE event type.

- [ ] **Step 3: Emit `DatastarEventTemplateData` per-framing**

In `routes.go`, in the per-framing datastar arm, emit the event type when any datastar route uses the sse representation:

```go
		usesDatastarStream := slices.ContainsFunc(routeDefinitions, func(d muxt.Definition) bool {
			return effectiveFraming(config, d) == muxt.FramingDatastar && d.Representation() == muxt.RepresentationSSE
		})
		if usesDatastarStream {
			decls = append(decls, datastarEventTemplateDataDecls(file, config)...)
		}
```

`datastarEventTemplateDataDecls` currently reads the event type name from config — confirm it uses `config.DatastarEventTemplateDataType` (add the parameter / repoint it from any old name source so it uses the Task 2 field). The retained `datastarMarshalSignals` helper must also be emitted when `usesDatastarStream` (any stream may carry signal frames) OR specifically when a stream route has a `marshalJSON(sendX)` send — emit it whenever `usesDatastarStream` to keep it simple.

- [ ] **Step 4: Parameterize the SSE stream handler by framing**

The Phase 2 SSE handler builder selects the per-event template-data type (`SSETemplateData`) and the event marshaler (`SSETemplateData.WriteTo`, plain `data:` lines). Parameterize it so that under `effectiveFraming(config, def) == muxt.FramingDatastar`:

- the per-event template-data type is `config.DatastarEventTemplateDataType` (so `send`/`sendX` callbacks render templates whose `.` exposes `.Selector`/`.Mode`/`.Namespace`/`.UseViewTransition`, and `WriteTo` emits `event: datastar-patch-elements`); and
- a `marshalJSON(sendX)` send is generated as a `func(T, bool) error` that calls `datastarMarshalSignals` and writes a `datastar-patch-signals` frame with the per-frame `onlyIfMissing` bool — reuse the existing `signalEventClosure` body from the old datastar stream handler (`datastar_handlers.go`) for the frame-writing, adapting it to the Phase 2 send-callback wiring.

Concretely: locate the Phase 2 SSE handler builder (the function `methodHandlerFunc` dispatches to for `RepresentationSSE`). Add a parameter or internal branch keyed on `effectiveFraming(config, def)`:
- `FramingDatastar` → event type = `config.DatastarEventTemplateDataType`, element-send marshaler = patch-elements `WriteTo`, signal-send (`marshalJSON(sendX)`) marshaler = patch-signals via `datastarMarshalSignals` with the `func(T, bool) error` shape;
- otherwise → existing generic `SSETemplateData` behavior (unchanged for htmx/none).

This is the representation×framing seam ADR 9 describes: the SSE transport is generic, only the marshaler/event-type differs by framing. Keep the generic path byte-identical for non-datastar routes.

- [ ] **Step 5: Run the fixtures, then full suite**

Run: `go test ./cmd/muxt -run 'Test/reference_datastar_framing_elements|Test/reference_datastar_framing_signals_stream'` → PASS.
Run: `go test ./...` → PASS. Existing generic `reference_sse_*` fixtures must remain green (the generic marshaler path is unchanged); existing `reference_datastar_*` fixtures (old arg path) still green.

- [ ] **Step 6: Commit**

```bash
gofumpt -w internal/generate/routes.go internal/generate/datastar_handlers.go internal/generate/datastar_event_template_data.go
git add internal/generate/routes.go internal/generate/datastar_handlers.go internal/generate/datastar_event_template_data.go cmd/muxt/testdata/reference_datastar_framing_elements.txt cmd/muxt/testdata/reference_datastar_framing_signals_stream.txt
git commit -m "feat(generate): datastar(sse) streams patch-elements and inline patch-signals

Assisted-by: Claude:claude-opus-4-8 gofumpt"
```

---

## Task 6: Flip `--use-datastar` auto-wrap; delete the arg families; migrate fixtures

Now that the framing-driven path handles all three datastar shapes via explicit wrappers, switch `--use-datastar` to auto-wrap and delete the dev-only arg machinery and the old handler path. Migrate the existing `reference_datastar_*` fixtures.

**Files:**
- Modify: `internal/generate/routes.go` (`effectiveFraming` datastar branch; remove the old `config.Datastar` emission arm and old dispatch; repoint datastar default type names)
- Modify: `internal/cli/commands.go` (`applyDefaults`: stop forcing `config.TemplateDataType`/`SSETemplateDataType` to datastar names under `--use-datastar`; datastar render now uses the dedicated flags)
- Modify: `internal/muxt/definition.go` (delete arg-family recognition + accessors + constants + `checkArguments` handling)
- Modify: `internal/generate/datastar_handlers.go` (delete `datastarMethodHandlerFunc` arg-driven dispatch + elements/signal/script shape handlers; keep the frame-writing helpers reused by Task 5)
- Modify: `internal/generate/fake_server.go`, `datastar_event_template_data.go`, `datastar_signals.go` as needed to drop `UsesDatastar`/arg references
- Tests: migrate `cmd/muxt/testdata/reference_datastar_actions.txt`, `reference_datastar_elements.txt`, `reference_datastar_signals.txt`, `reference_datastar_signals_jsonv2.txt`, `reference_datastar_signals_stream.txt`, `reference_datastar_multiple_callbacks.txt`, `reference_datastar_args_only_reserved_with_flag.txt`; delete `reference_datastar_script.txt`; add `err_datastar_script_removed.txt`

- [ ] **Step 1: Flip auto-wrap**

In `effectiveFraming` (`routes.go`), replace the Task-3 NOTE comment with the real branch:

```go
	if config.Datastar {
		return muxt.FramingDatastar
	}
```

- [ ] **Step 2: Remove the old datastar emission arm + repoint defaults**

In `routes.go`, delete the `if config.Datastar { decls = append(decls, templateDataDecls(file, config, config.TemplateDataType, false)...) }` arm and the unconditional old `datastarActionsDecls`/`datastarEventTemplateDataDecls(UsesElements)`/`datastarSignalsDecls(UsesSignal)` appends. The per-framing datastar arm (Tasks 3–5) now emits everything (it fires because `--use-datastar` makes routes `FramingDatastar`). Ensure the per-framing emission else-branch runs for `config.Datastar` too (remove the `if config.Datastar { ... } else { <per-framing> }` split so the per-framing block always runs).

In `internal/cli/commands.go` `applyDefaults`, remove the block that, under `config.Datastar`, overrides `config.TemplateDataType`/`config.SSETemplateDataType` with the datastar default names. Datastar routes now render via `config.DatastarTemplateDataType` / `config.DatastarEventTemplateDataType` (defaulted in Task 2). `--output-template-data-type`/`--output-sse-template-data-type` revert to their generic defaults (harmless: under `--use-datastar` every route is `FramingDatastar`, so minimal `TemplateData`/`SSETemplateData` are not emitted).

- [ ] **Step 3: Delete the arg-family recognition (definition.go)**

Use `gopls` to find references first: `go test ./...` will surface every break. Delete `IsElementsArgument`, `IsSignalArgument`, `IsScriptArgument`, `IsDatastarArgument`, the `UsesElements`/`UsesSignal`/`UsesScript`/`UsesDatastar` methods, the `TemplateNameScopeIdentifierElements`/`Signal`/`Script` constants, and any `checkArguments` branch that treated them as reserved. After removal, a bare `elements`/`signal`/`script` identifier is an ordinary argument (path value or method param) — i.e. an unknown reserved name is just resolved like any other argument.

- [ ] **Step 4: Delete the old arg-driven handler path (datastar_handlers.go)**

Delete `datastarMethodHandlerFunc` and the arg-shape handlers it dispatched to (`datastarStreamHandlerFunc`, `datastarSignalHandlerFunc`, `datastarScriptHandlerFunc`, `datastarResponseHandlerFunc` and their `signalCallbackSignature`/`validateSignalCallbackShape`/`signalResponseClosure` helpers) **except** the frame-writing pieces reused by Task 5 (the patch-signals frame emitter adapted into the Phase 2 send path, and `datastarMarshalSignals`). In `routes.go`, delete the `if config.Datastar && def.UsesDatastar() { return datastarMethodHandlerFunc(...) }` dispatch (the framing×representation dispatch from Tasks 4–5 replaces it). Delete `script` handling entirely — there is no replacement.

- [ ] **Step 5: Migrate the existing fixtures to wrapper syntax (behavioral)**

Rewrite each, keeping behavioral assertions (no grep):
- `reference_datastar_actions.txt` — `--use-datastar` plain routes calling `.Actions` now auto-wrap to `FramingDatastar` rendering `DatastarTemplateData`; verify it passes unchanged or adjust template-data-type references. Assert the rendered `@verb('url')` action expression in the body (behavioral).
- `reference_datastar_elements.txt` → use `datastar(sse(Method(ctx, send)))` (or keep `--use-datastar` and convert the `elements` callback to `send`); assert the `datastar-patch-elements` frame bytes.
- `reference_datastar_signals.txt` → `datastar(marshalJSON(Method(ctx)))`; assert `application/json` body.
- `reference_datastar_signals_jsonv2.txt` → same with `--output-jsonv2`; assert the jsonv2-marshaled body.
- `reference_datastar_signals_stream.txt` → `datastar(sse(Method(ctx, send, marshalJSON(sendX))))`; assert mixed frames.
- `reference_datastar_multiple_callbacks.txt` → multiple `sendX` element callbacks under `datastar(sse(...))`.
- `reference_datastar_args_only_reserved_with_flag.txt` → its premise (args not reserved without the flag) is now always true; either delete it or repurpose it to assert a method param named `signal`/`elements` works as an ordinary argument under `datastar(...)`.
- Delete `reference_datastar_script.txt`.
- Add `err_datastar_script_removed.txt`: a route whose call uses `script` as a render callback under `--use-datastar` now fails (script is an unknown/ordinary argument with no method param to bind) — assert the actual `! muxt generate`/`! muxt check` error and a `stderr` substring of the real message. Verify the exact behavior by running it and matching the regex to what is emitted.

- [ ] **Step 6: Run the full suite**

Run: `go test ./...`
Expected: PASS. Iterate on each migrated fixture until green. Run `gofumpt -w` on every modified `.go` file and `go run github.com/crhntr/txtarfmt/cmd/txtarfmt -ext=.txt cmd/muxt/testdata/reference_datastar_*.txt cmd/muxt/testdata/err_datastar_*.txt` so fixtures are formatter-clean.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat(generate): --use-datastar auto-wraps datastar(); remove elements/signal/script args

Datastar codegen is now driven by framing + representation. The dev-only
elements/signal/script reserved arguments and their handler path are deleted;
script has no replacement. Existing datastar fixtures migrate to wrapper syntax.

Assisted-by: Claude:claude-opus-4-8 gofumpt txtarfmt"
```

---

## Task 7: Documentation

**Files:**
- Modify: `docs/reference/call-parameters.md` (datastar framing subsection), `docs/reference/datastar.md` (rewrite around the wrapper model), `docs/reference/cli.md` (the three new flags), `docs/explanation/decisions/00007_framing_wrappers.md` (status)

- [ ] **Step 1: Document datastar framing**

In `docs/reference/call-parameters.md`, extend the "Frontend Framing" section: `datastar(Method(ctx))` → `DatastarTemplateData` (`.Actions()` + `datastar-selector`/`mode`/`use-view-transition` header setters); `datastar(sse(...))` → patch-elements stream (`send`/`sendX` render frames; `marshalJSON(sendX)` → inline patch-signals with per-frame `onlyIfMissing`); `datastar(marshalJSON(...))` → `application/json` body + `.OnlyIfMissing` (`datastar-only-if-missing`) on `DatastarSignalsTemplateData`; `--use-datastar` auto-wraps every route (no opt-out); `script` is removed. Every example must match a passing fixture from Tasks 3–6.

In `docs/reference/datastar.md`, replace the `elements`/`signal`/`script` argument documentation with the wrapper model. In `docs/reference/cli.md`, add the three `--output-datastar-*-template-data-type` rows (Use/Output tables) and note any change to how `--use-datastar` interacts with `--output-template-data-type`.

- [ ] **Step 2: ADR status**

In `docs/explanation/decisions/00007_framing_wrappers.md`, update Status from `Implemented (Phase 3a: htmx; datastar framing in 3b)` to `Implemented (Phase 3a: htmx; Phase 3b: datastar)`.

- [ ] **Step 3: Verify & commit**

Run: `go test ./docs/...` (link validation) and `go test ./...`.

```bash
git add docs/
git commit -m "docs: document datastar() framing and the arg-family removal

Assisted-by: Claude:claude-opus-4-8"
```

---

## Self-Review

**Spec coverage:**
- `FramingDatastar` + `datastar()` recognition/strip → Task 1. ✓
- `--use-datastar` auto-wrap (mirror htmx) → Task 6 (after old path removed). ✓
- `DatastarTemplateData` (`.Actions()` + selector/mode/use-view-transition headers) → Task 3. ✓
- `DatastarEventTemplateData` retained; patch-elements stream via send/sendX → Task 5. ✓
- `DatastarSignalsTemplateData` (`.OnlyIfMissing`, application/json body, template evaluated for header) → Task 4. ✓
- Streaming `marshalJSON(sendX)` = `func(T, bool) error` per-frame onlyIfMissing → Task 5. ✓
- `.Actions()` on datastar render type, per-framing → Task 3 (accessor parameterized) + Task 6 (old unconditional emission removed). ✓
- Remove elements/signal/script + script dropped → Task 6. ✓
- Three `--output-datastar-*-type` flags (validate + round-trip) → Task 2. ✓
- Behavioral fixtures, no grep → Tasks 3–6. ✓
- Docs + ADR status → Task 7. ✓

**Sequencing / green-at-each-commit:** Tasks 1–2 additive. Tasks 3–5 build the new path via **explicit `datastar(...)` wrappers** with `config.Datastar` false, so the old arg path stays dormant and all existing `reference_datastar_*`/`reference_sse_*` fixtures stay green. Task 6 flips auto-wrap, deletes the old path, and migrates fixtures atomically. Task 7 docs. The one large task (6) is mostly deletion + repointing because the new path is already proven in 3–5.

**Type/name consistency:** `FramingDatastar`, `FramingWrapperDatastar`, `DatastarTemplateDataType`/`DatastarEventTemplateDataType`/`DatastarSignalsTemplateDataType`, `datastarTemplateDataDecls`/`datastarSignalsTemplateDataDecls`/`datastarEventTemplateDataDecls`, `datastarRenderHeaderSetterMethod`/`datastarOnlyIfMissingMethod`, `effectiveFraming`/`renderTemplateDataType` used consistently. `datastarActionsAccessorMethod` gains a `typeName` parameter in Task 3.

**Open verification points flagged for the implementer:** exact patch-signals data-line syntax in `signalEventClosure` (Task 5 Step 1 NOTE); the real Phase 2 marshalJSON handler builder name (Task 4 Step 5); the bool-stringify expression in the existing `UseViewTransition` setter (Task 3 Step 3); the real `script`-removed error text (Task 6 Step 5).

**Out of scope (correctly deferred):** `script`/`text/javascript` ergonomics; non-datastar use of the `datastar-*` HTML headers; the `datastarActionBuilderSource` string-const → go/ast refactor; `.Actions` check-side path-arg validation.