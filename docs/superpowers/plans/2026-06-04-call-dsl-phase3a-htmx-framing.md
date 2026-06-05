# Phase 3a — htmx framing + HTMXTemplateData split Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. **This is a muxt project — dispatched agents must use the muxt skills** (`muxt_test-driven-development`, `muxt_refactoring`, `muxt_explore-from-route`).
>
> **Project preference (enforce in every task):** generate Go code with functions that **build go/ast nodes and return concrete static node types** (`*ast.FuncDecl`, `*ast.GenDecl`, `[]ast.Decl`, …). Do NOT introduce `parser.ParseFile`-from-string-const patterns.

**Goal:** Introduce the outer **framing wrapper** mechanism and the `htmx()` frontend: a dedicated `HTMXTemplateData` type carries the HX* helpers (removed from a now-minimal `TemplateData`), `htmx(Method(...))` selects it, and `--use-htmx` auto-wraps every route.

**Architecture:** Mirror Phase 2's representation wrapper. A `Framing` enum (`FramingNone`/`FramingHTMX`) is recognized at the outermost call position and stripped in `parseHandler` *before* the representation wrapper, so `def.call`/`def.fun` stay on the inner method. Template-data generation is refactored from one hard-coded type into a `templateDataDecls(file, config, typeName, withHTMX)` aggregator emitted **per distinct framing used in the file**. A route's non-SSE render uses the type matching its *effective* framing (`def.Framing()`, or htmx when `--use-htmx` is set).

**Tech Stack:** Go `go/ast`, `go/types`, `go/parser`; muxt generator (`internal/muxt`, `internal/generate`, `internal/cli`); txtar fixtures in `cmd/muxt/testdata/` run by `Test` in `cmd/muxt/main_test.go`.

**Test command reference:** one fixture: `go test ./cmd/muxt -run 'Test/<basename>'`; package: `go test ./cmd/muxt`; all: `go test ./...`.

---

## File Structure

- **`internal/muxt/definition.go`** — parser. Add `Framing` enum + `FramingWrapperHTMX` const + `framing` field + `Framing()` accessor; strip `htmx(...)` outermost in `parseHandler` before the representation unwrap; reserve `htmx` as an outer-only name.
- **`internal/generate/template_data.go`** — parameterize the base-type method builders by target type name; add the `templateDataDecls(file, config, typeName, withHTMX) []ast.Decl` aggregator.
- **`internal/generate/routes.go`** — replace the inline template-data block (`:361-383`) with the aggregator; add `effectiveFraming`/`renderTemplateDataType` helpers; emit base types per framing; thread the render type through the two non-SSE render composites (`:635`, `:1204`).
- **`internal/cli/commands.go`** — add `--output-htmx-template-data-type` (default `HTMXTemplateData`); `applyDefaults` default; keep htmx/datastar mutual exclusion.
- **`cmd/muxt/testdata/`** — `reference_htmx_framing.txt`, `reference_htmx_mixed.txt`, `err_htmx_bad_arity.txt`, `reference_htmx_auto_wrap.txt`, `reference_htmx_template_data_minimal.txt`; migrate `reference_htmx_helpers.txt`.
- **`docs/reference/`** — document framing + the split.

---

## Task 1: Parser — recognize and unwrap `htmx(...)`

**Files:**
- Modify: `internal/muxt/definition.go` (`parseHandler`; const/enum area near the Phase-2 `Representation`; accessors near `Representation()`)
- Test: `internal/muxt/definition_internal_test.go`

- [ ] **Step 1: Write the failing unit test**

Add to `internal/muxt/definition_internal_test.go` (package `muxt`):

```go
func TestDefinitionFraming(t *testing.T) {
	for _, tt := range []struct {
		name string
		want Framing
		rep  Representation
		fun  string
	}{
		{name: "GET /a Plain(ctx)", want: FramingNone, rep: RepresentationNone, fun: "Plain"},
		{name: "GET /b htmx(Index(ctx))", want: FramingHTMX, rep: RepresentationNone, fun: "Index"},
		{name: "GET /c htmx(sse(Stream(ctx, send)))", want: FramingHTMX, rep: RepresentationSSE, fun: "Stream"},
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

func TestDefinitionHTMXBadArity(t *testing.T) {
	for _, in := range []string{"GET /a htmx()", "GET /b htmx(Index(ctx), Other(ctx))"} {
		if _, err, _ := newDefinition(template.New(in)); err == nil {
			t.Fatalf("newDefinition(%q): want error for bad htmx arity", in)
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/muxt -run 'TestDefinitionFraming|TestDefinitionHTMXBadArity'`
Expected: FAIL — `def.Framing undefined`, `FramingHTMX undefined`.

- [ ] **Step 3: Add the enum, field, accessor, reserved name**

In `internal/muxt/definition.go`, near the Phase-2 `Representation` enum, add:

```go
// Framing names the optional outermost frontend wrapper of a handler call.
type Framing int

const (
	FramingNone Framing = iota
	FramingHTMX
)

// FramingWrapperHTMX is the reserved outer framing-wrapper function name.
const FramingWrapperHTMX = "htmx"
```

Add a field to `Definition` (next to `representation`):

```go
	// framing records the outermost frontend wrapper (none, htmx).
	framing Framing
```

Add the accessor next to `Representation()`:

```go
func (def Definition) Framing() Framing { return def.framing }
```

- [ ] **Step 4: Strip `htmx(...)` first in `parseHandler`**

In `parseHandler`, after `fun, ok := call.Fun.(*ast.Ident)` and the ellipsis check, and **before** the existing representation-wrapper detection block (the `switch fun.Name { case RepresentationWrapperSSE ... }`), insert:

```go
	// Strip the optional outermost framing wrapper (htmx(...)) first, so the
	// representation wrapper and method call are detected on what remains.
	framing := FramingNone
	if fun.Name == FramingWrapperHTMX {
		framing = FramingHTMX
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

The existing representation block then runs on the unwrapped `call`/`fun`, and `checkArguments` after it. (`htmx` is thus reserved only as an outer name; a bare `htmx` argument is already an "unknown argument" because it is not in `patternScope()`.)

- [ ] **Step 5: Run the unit tests to verify they pass**

Run: `go test ./internal/muxt -run 'TestDefinitionFraming|TestDefinitionHTMXBadArity'`
Expected: PASS.

Run: `go test ./...`
Expected: PASS (additive; nothing dispatches on `Framing` yet).

- [ ] **Step 6: Commit**

```bash
gofumpt -w internal/muxt/definition.go internal/muxt/definition_internal_test.go
git add internal/muxt/definition.go internal/muxt/definition_internal_test.go
git commit -m "feat(muxt): recognize htmx() framing wrapper

Assisted-by: Claude:claude-opus-4-8 gofumpt"
```

---

## Task 2: Refactor template-data generation into a parameterized aggregator (behavior-preserving)

**Files:**
- Modify: `internal/generate/template_data.go` (parameterize `templateRedirect`, `templateRedirectHelperMethod`, `templateRedirectHelperMethods`, `templateDataMuxtVersionMethod`, `templateDataPathMethod` by target type name; add `templateDataDecls`)
- Modify: `internal/generate/routes.go` (replace the inline block `:361-383` with one aggregator call)

This task changes NO generated output — it restructures so Task 4 can emit per framing. Verify by `go test ./...` staying green (the existing `reference_*`/`reference_htmx_helpers` golden/grep fixtures are the regression guard).

- [ ] **Step 1: Parameterize the config-reading builders by type name**

Five builders currently read `config.TemplateDataType` internally. Add a `typeName string` parameter to each and use it instead:
- `templateDataMuxtVersionMethod(config RoutesFileConfiguration)` → `templateDataMuxtVersionMethod(config RoutesFileConfiguration, typeName string)` (use `typeName` at the `templateDataMethodReceiver(...)` call, ~line 215).
- `templateDataPathMethod(config RoutesFileConfiguration)` → `templateDataPathMethod(config RoutesFileConfiguration, typeName string)` (receiver + any `config.TemplateDataType` use, ~line 240).
- `templateRedirect(file *File, config RoutesFileConfiguration)` → `templateRedirect(file *File, config RoutesFileConfiguration, typeName string)` (receiver ~122 and the `IndexListExpr` result ~129 use `typeName`).
- `templateRedirectHelperMethod(file, config, methodName, statusCode)` → add `typeName string` (receiver ~179, result ~185).
- `templateRedirectHelperMethods(file, config)` → add `typeName string`; pass it through to each `templateRedirectHelperMethod` call.

Each must still return its current concrete type (`*ast.FuncDecl` / `[]*ast.FuncDecl`).

- [ ] **Step 2: Add the `templateDataDecls` aggregator**

Add to `internal/generate/template_data.go`:

```go
// templateDataDecls returns the base template-data type named typeName and its
// methods. When withHTMX is true the HX* helper methods are included (this is the
// HTMXTemplateData variant); otherwise the type is the minimal base.
func templateDataDecls(file *File, config RoutesFileConfiguration, typeName string, withHTMX bool) []ast.Decl {
	decls := []ast.Decl{
		templateDataType(file, typeName, ast.NewIdent(config.ReceiverInterface)),
		templateDataMuxtVersionMethod(config, typeName),
		templateDataPathMethod(config, typeName),
		templateDataResultMethod(typeName),
		templateDataRequestMethod(file, typeName),
		templateDataStatusCodeMethod(typeName),
		templateDataHeaderMethod(typeName),
		templateDataOkay(typeName),
		templateDataError(file, typeName),
		templateDataReceiver(ast.NewIdent(config.ReceiverInterface), typeName),
		templateRedirect(file, config, typeName),
	}
	for _, m := range templateRedirectHelperMethods(file, config, typeName) {
		decls = append(decls, m)
	}
	decls = append(decls, templateDataStringMethod(typeName))
	if withHTMX {
		for _, m := range templateDataHTMXHelperMethods(typeName) {
			decls = append(decls, m)
		}
	}
	return decls
}
```

- [ ] **Step 3: Replace the inline block in `routes.go`**

In the `decls := []ast.Decl{...}` literal (`internal/generate/routes.go:346-407`), remove the template-data lines `:361-382` (from `templateDataType(...)` through the `if config.HTMXHelpers { ... }` block and the `templateDataStringMethod` append) and replace with a single append right after `routesFunc`:

```go
	decls := []ast.Decl{
		importDecl,
		&ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{Name: ast.NewIdent(config.ReceiverInterface), Type: receiverInterface}}},
		routesFunc,
	}
	decls = append(decls, templateDataDecls(file, config, config.TemplateDataType, config.HTMXHelpers)...)
```

(Keep the subsequent datastar/sse/marshalJSON/routePath appends exactly as they are. The `templateRedirectHelperMethods` loop that was inline is now inside `templateDataDecls`.) Confirm there is no leftover reference to the removed inline lines.

- [ ] **Step 4: Verify behavior is unchanged**

Run: `go test ./...`
Expected: PASS — generated output is byte-identical (same type name `config.TemplateDataType`, HX* still gated by `config.HTMXHelpers`). The `reference_htmx_helpers.txt` and all golden fixtures are the regression guard.

- [ ] **Step 5: Commit**

```bash
gofumpt -w internal/generate/template_data.go internal/generate/routes.go
git add internal/generate/template_data.go internal/generate/routes.go
git commit -m "refactor(generate): parameterize template-data generation by type name

Assisted-by: Claude:claude-opus-4-8 gofumpt"
```

---

## Task 3: CLI — `--output-htmx-template-data-type`

**Files:**
- Modify: `internal/generate/routes.go` (`RoutesFileConfiguration`: add `HTMXTemplateDataType string`)
- Modify: `internal/cli/commands.go` (flag registration; `applyDefaults`)
- Test: `cmd/muxt/testdata/reference_htmx_template_data_type_flag.txt`

- [ ] **Step 1: Write the failing fixture**

`cmd/muxt/testdata/reference_htmx_template_data_type_flag.txt` (generation-only grep; proves the flag renames the htmx type — relies on Task 4's auto-wrap emission, but the flag wiring is added here and the default `HTMXTemplateData` is asserted by Task 5's fixtures, so this fixture asserts the OVERRIDE name appears):

```
muxt generate --use-receiver-type=Server --use-htmx --output-htmx-template-data-type=Widget

grep 'type Widget\[' template_routes.go

-- template.gohtml --
{{- define "GET / Index(ctx)" -}}<p>{{- .Result -}}</p>{{- end -}}
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

func (Server) Index(ctx context.Context) string { return "hi" }
```

NOTE: this fixture depends on Task 4's auto-wrap emission to produce `type Widget[`. If you are running tasks strictly in order, mark this fixture's first run as expected-fail until Task 4, OR (preferred) move this fixture's creation into Task 5. To keep Task 3 self-contained, instead assert only flag PARSING here with a unit test:

Replace the fixture with a CLI unit test `internal/cli/commands_internal_test.go` (or the existing test file) asserting that parsing `--output-htmx-template-data-type=Widget` sets `config.HTMXTemplateDataType == "Widget"`, and that the default is `HTMXTemplateData` after `applyDefaults` when `--use-htmx` is set. (Find how existing flags are unit-tested in `internal/cli`; mirror that.) If `internal/cli` has no such test harness, create the fixture in Task 5 instead and make Task 3 a pure additive flag-wiring change verified by `go test ./...`.

- [ ] **Step 2: Add the config field**

In `RoutesFileConfiguration` (`internal/generate/routes.go`, near `TemplateDataType`):

```go
	HTMXTemplateDataType string
```

- [ ] **Step 3: Register the flag and default**

In `internal/cli/commands.go`, near the `--output-template-data-type` registration, add a const and flag:

```go
	outputHTMXTemplateDataType = "output-htmx-template-data-type"
	outputHTMXTemplateDataTypeHelp = "the type name for the generated HTMX template data type (htmx framing)"
```

```go
	flagSet.StringVar(&g.HTMXTemplateDataType, outputHTMXTemplateDataType, "", outputHTMXTemplateDataTypeHelp)
```

In `applyDefaults`, after the existing template-data-type defaulting, set the htmx default:

```go
	config.HTMXTemplateDataType = cmp.Or(config.HTMXTemplateDataType, "HTMXTemplateData")
```

(Keep the `--use-htmx`/`--use-datastar` mutual-exclusion check unchanged.)

- [ ] **Step 4: Verify**

Run: `go test ./...`
Expected: PASS (additive; nothing reads `HTMXTemplateDataType` yet).

- [ ] **Step 5: Commit**

```bash
gofumpt -w internal/cli/commands.go internal/generate/routes.go
git add internal/cli/commands.go internal/generate/routes.go
git commit -m "feat(cli): add --output-htmx-template-data-type flag

Assisted-by: Claude:claude-opus-4-8 gofumpt"
```

---

## Task 4: `htmx()` explicit framing — render-type selection + per-framing emission

**Files:**
- Modify: `internal/generate/routes.go` (add `effectiveFraming`, `renderTemplateDataType`; per-framing emission for the non-datastar path; thread the render type into the two non-SSE render composites at `:635` and `:1204`)
- Test: `cmd/muxt/testdata/reference_htmx_framing.txt`, `reference_htmx_mixed.txt`, `err_htmx_bad_arity.txt`

- [ ] **Step 1: Write the failing fixtures**

`cmd/muxt/testdata/reference_htmx_framing.txt` — explicit `htmx(Index(ctx))`, no flag; renders with `HTMXTemplateData` and `.HXRedirect` works:

```
muxt generate --use-receiver-type=Server
muxt check

exec go test -race -count=1

-- template.gohtml --
{{- define "GET /set htmx(Set(ctx))" -}}{{- .HXRedirect "/next" -}}done{{- end -}}
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

func (Server) Set(ctx context.Context) string { return "" }
-- server_test.go --
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTMXFraming(t *testing.T) {
	mux := http.NewServeMux()
	TemplateRoutes(mux, Server{})
	req := httptest.NewRequest(http.MethodGet, "/set", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if got := rec.Header().Get("HX-Redirect"); got != "/next" {
		t.Fatalf("HX-Redirect = %q, want /next", got)
	}
}
```

`cmd/muxt/testdata/reference_htmx_mixed.txt` — one `htmx(...)` route and one unframed route in a no-flag file; assert BOTH `HTMXTemplateData` and `TemplateData` are generated:

```
muxt generate --use-receiver-type=Server
muxt check

exec go test -race -count=1
grep 'type TemplateData\[' template_routes.go
grep 'type HTMXTemplateData\[' template_routes.go

-- template.gohtml --
{{- define "GET /plain Plain(ctx)" -}}<p>{{- .Result -}}</p>{{- end -}}
{{- define "GET /framed htmx(Framed(ctx))" -}}<p>{{- .Result -}}</p>{{- end -}}
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

func (Server) Plain(ctx context.Context) string { return "plain" }
func (Server) Framed(ctx context.Context) string { return "framed" }
-- server_test.go --
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMixed(t *testing.T) {
	mux := http.NewServeMux()
	TemplateRoutes(mux, Server{})
	for _, tc := range []struct{ path, want string }{{"/plain", "plain"}, {"/framed", "framed"}} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if got := rec.Body.String(); !strings.Contains(got, tc.want) {
			t.Fatalf("%s body = %q, want contains %q", tc.path, got, tc.want)
		}
	}
}
```

`cmd/muxt/testdata/err_htmx_bad_arity.txt`:

```
! muxt generate --use-receiver-type=Server
stderr 'htmx takes exactly one argument'

-- template.gohtml --
{{- define "GET / Index(ctx, extra)" -}}{{- end -}}
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

func (Server) Index(ctx context.Context) string { return "" }
```

WAIT — `err_htmx_bad_arity.txt` must actually use `htmx()`/`htmx(a, b)`. The parse error fires in `parseHandler` (Task 1). Use the template name `{{- define "GET / htmx()" -}}` (zero args). Correct the template line to:

```
-- template.gohtml --
{{- define "GET / htmx()" -}}{{- end -}}
```

(Task 1 already produces the error `htmx takes exactly one argument: the wrapped call`; the `stderr` regex `htmx takes exactly one argument` matches.) Since Task 1 added this error, this fixture could also live in Task 1; placing it here keeps the htmx generation fixtures together.

- [ ] **Step 2: Run the fixtures to verify they fail**

Run: `go test ./cmd/muxt -run 'Test/reference_htmx_framing|Test/reference_htmx_mixed|Test/err_htmx_bad_arity'`
Expected: `err_htmx_bad_arity` PASSES already (Task 1). `reference_htmx_framing`/`reference_htmx_mixed` FAIL — htmx framing isn't wired: `.HXRedirect` isn't on the render type and `HTMXTemplateData` isn't emitted.

- [ ] **Step 3: Add framing helpers**

In `internal/generate/routes.go`:

```go
// effectiveFraming is a route's framing after applying the --use-htmx flag, which
// auto-wraps every otherwise-unframed route (ADR 7: no per-route opt-out).
func effectiveFraming(config RoutesFileConfiguration, def muxt.Definition) muxt.Framing {
	if def.Framing() != muxt.FramingNone {
		return def.Framing()
	}
	if config.HTMXHelpers {
		return muxt.FramingHTMX
	}
	return muxt.FramingNone
}

// renderTemplateDataType is the non-SSE render template-data type name for a
// framing: HTMXTemplateData for htmx, the configured base type otherwise.
func renderTemplateDataType(config RoutesFileConfiguration, framing muxt.Framing) string {
	if framing == muxt.FramingHTMX {
		return config.HTMXTemplateDataType
	}
	return config.TemplateDataType
}
```

- [ ] **Step 4: Per-framing type emission (non-datastar path)**

In the `decls` assembly, replace the unconditional `decls = append(decls, templateDataDecls(file, config, config.TemplateDataType, config.HTMXHelpers)...)` (from Task 2) with framing-aware emission. Under `--use-datastar` keep the existing single emission; otherwise emit per framing actually used:

```go
	if config.Datastar {
		decls = append(decls, templateDataDecls(file, config, config.TemplateDataType, false)...)
	} else {
		usesNone := slices.ContainsFunc(routeDefinitions, func(d muxt.Definition) bool { return effectiveFraming(config, d) == muxt.FramingNone })
		usesHTMX := slices.ContainsFunc(routeDefinitions, func(d muxt.Definition) bool { return effectiveFraming(config, d) == muxt.FramingHTMX })
		if usesNone {
			decls = append(decls, templateDataDecls(file, config, config.TemplateDataType, false)...)
		}
		if usesHTMX {
			decls = append(decls, templateDataDecls(file, config, config.HTMXTemplateDataType, true)...)
		}
	}
```

(This must be placed where the Task-2 single call was — after `routesFunc`, before the datastar/sse blocks. Ensure `routeDefinitions` is the in-scope slice of definitions; it is the same slice used by the datastar `slices.ContainsFunc` checks below.)

- [ ] **Step 5: Thread the render type into the non-SSE render composites**

Two sites build the render composite with `X: ast.NewIdent(config.TemplateDataType)`:
- `internal/generate/routes.go:635` (the no-receiver-method handler, `noReceiverMethodCall`).
- `internal/generate/routes.go:1204` (`methodHandlerFunc` main path).

In each, compute the render type from the route's effective framing and use it. In `methodHandlerFunc` (which has `def`), add near the top (after the `Representation()` switch returns for sse/marshalJSON, so this only affects non-SSE/non-JSON renders):

```go
	renderType := renderTemplateDataType(config, effectiveFraming(config, def))
```

and replace the `ast.NewIdent(config.TemplateDataType)` at `:1204` with `ast.NewIdent(renderType)`. Do the same in `noReceiverMethodCall` (it has `def` in scope — confirm; if not, pass the computed `renderType` in). Leave the datastar handlers (`datastar_handlers.go:422`) and `.Actions()` (`datastar_actions.go:63`) untouched — datastar is 3b.

NOTE: under `--use-datastar`, `effectiveFraming` returns `FramingNone` (the htmx flag is off and these routes have no `htmx()` wrapper), so `renderTemplateDataType` returns `config.TemplateDataType` (= `DatastarTemplateData`) — datastar render behavior is unchanged. Verify this holds.

- [ ] **Step 6: Run the fixtures to verify they pass**

Run: `go test ./cmd/muxt -run 'Test/reference_htmx_framing|Test/reference_htmx_mixed|Test/err_htmx_bad_arity'`
Expected: PASS.

- [ ] **Step 7: Full suite**

Run: `go test ./...`
Expected: PASS. The existing `reference_htmx_helpers.txt` (which uses `--use-htmx`) still passes here because `--use-htmx` now makes `usesHTMX` true and emits `HTMXTemplateData` with HX* — BUT its render type also becomes `HTMXTemplateData` and the minimal `TemplateData` is no longer emitted under the flag. If `reference_htmx_helpers.txt` greps for HX* on a `TemplateData` receiver or greps `type TemplateData`, it will FAIL here. **If it fails, do NOT fix it in this task** — it is migrated in Task 5. Instead, temporarily confirm the failure is only that fixture and proceed; Task 5 migrates it. (If you prefer a clean green here, move the Task-5 migration of `reference_htmx_helpers.txt` forward into this task.)

To keep the suite green at this commit, migrate `reference_htmx_helpers.txt` now as part of Step 7 if it fails (apply the Task 5 Step 1 migration), and note it in the commit.

- [ ] **Step 8: Commit**

```bash
gofumpt -w internal/generate/routes.go
git add internal/generate/routes.go cmd/muxt/testdata/reference_htmx_framing.txt cmd/muxt/testdata/reference_htmx_mixed.txt cmd/muxt/testdata/err_htmx_bad_arity.txt
git commit -m "feat(generate): htmx() framing selects HTMXTemplateData; emit base types per framing

Assisted-by: Claude:claude-opus-4-8 gofumpt"
```

---

## Task 5: `--use-htmx` auto-wrap semantics + the HTMXTemplateData split made observable

**Files:**
- Test: `cmd/muxt/testdata/reference_htmx_auto_wrap.txt`, `reference_htmx_template_data_minimal.txt`; migrate `reference_htmx_helpers.txt`
- Modify (only if needed): nothing in generator — Task 4 already wired `effectiveFraming` to honor `--use-htmx`. This task LOCKS IN the behavior with fixtures and migrates the legacy one.

- [ ] **Step 1: Migrate `reference_htmx_helpers.txt`**

Read the current fixture. It runs `muxt generate --use-receiver-type=Server --use-htmx` and greps the 17–19 HX* method names. After Phase 3a, those methods are generated on the `HTMXTemplateData` receiver and the route renders with it. Update any grep that names the receiver type from `TemplateData` to `HTMXTemplateData` (e.g. `grep 'func (data \*TemplateData\[.*\]) HXRedirect'` → `... \*HTMXTemplateData\[...`). Grep lines that only match the method NAME (e.g. `grep HXRedirect`) need no change. Keep behavior/wire assertions identical. Run `go test ./cmd/muxt -run 'Test/reference_htmx_helpers'` → PASS.

- [ ] **Step 2: Write `reference_htmx_auto_wrap.txt`**

A plain route under `--use-htmx` auto-wraps to htmx framing; `.HXRedirect` works and the render type is `HTMXTemplateData`:

```
muxt generate --use-receiver-type=Server --use-htmx
muxt check

exec go test -race -count=1
grep 'type HTMXTemplateData\[' template_routes.go
! grep 'type TemplateData\[' template_routes.go

-- template.gohtml --
{{- define "GET /go Go(ctx)" -}}{{- .HXRedirect "/done" -}}ok{{- end -}}
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

func (Server) Go(ctx context.Context) string { return "" }
-- server_test.go --
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAutoWrap(t *testing.T) {
	mux := http.NewServeMux()
	TemplateRoutes(mux, Server{})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/go", nil))
	if got := rec.Header().Get("HX-Redirect"); got != "/done" {
		t.Fatalf("HX-Redirect = %q, want /done", got)
	}
}
```

- [ ] **Step 3: Write `reference_htmx_template_data_minimal.txt`**

An unframed route whose template calls `.HXRedirect` must FAIL `muxt check` (HX* no longer on minimal `TemplateData`):

```
muxt generate --use-receiver-type=Server
! muxt check
stderr 'HXRedirect'

-- template.gohtml --
{{- define "GET / Index(ctx)" -}}{{- .HXRedirect "/x" -}}{{- end -}}
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

func (Server) Index(ctx context.Context) string { return "" }
```

VERIFY the exact `muxt check` error text for a missing method and adjust the `stderr` regex to a substring it actually emits (it should mention `HXRedirect`). If `muxt generate` itself succeeds but `muxt check` reports the missing method, the two-line `! muxt check` + `stderr` form above is correct; if instead generation fails, restructure to `! muxt generate` + the matching stderr.

- [ ] **Step 4: Run the fixtures**

Run: `go test ./cmd/muxt -run 'Test/reference_htmx_helpers|Test/reference_htmx_auto_wrap|Test/reference_htmx_template_data_minimal'`
Expected: PASS.

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/muxt/testdata/reference_htmx_helpers.txt cmd/muxt/testdata/reference_htmx_auto_wrap.txt cmd/muxt/testdata/reference_htmx_template_data_minimal.txt
git commit -m "test(generate): htmx auto-wrap + HTMXTemplateData split fixtures

Assisted-by: Claude:claude-opus-4-8"
```

---

## Task 6: Documentation

**Files:**
- Modify: `docs/reference/call-parameters.md` (framing section); `docs/reference/datastar.md` if it references htmx; ADR `00007_framing_wrappers.md` Status (only if the Status section tracks implementation).

- [ ] **Step 1: Document framing**

In `docs/reference/call-parameters.md`, add a "Frontend framing" subsection (matching the file's heading style) covering: `htmx(Method(...))` selects `HTMXTemplateData` (the HX* helpers live there, not on `TemplateData`); `--use-htmx` auto-wraps every route (no per-route opt-out; mix by omitting the flag and wrapping explicitly); the htmx render-type name is `HTMXTemplateData` (override `--output-htmx-template-data-type`); `htmx(sse(...))` uses the generic SSE marshaler; an unframed route cannot call `.HXRedirect` (it's on `HTMXTemplateData`). Note `datastar()` framing arrives in 3b. Verify every example's template-name syntax against a passing fixture.

- [ ] **Step 2: Update the ADR status (if applicable)**

If `docs/explanation/decisions/00007_framing_wrappers.md` has a `## Status` that tracks implementation, note "Implemented (Phase 3a: htmx; datastar in 3b)". Otherwise leave it.

- [ ] **Step 3: Verify & commit**

Run: `go test ./...` (confirm no fixture broke).

```bash
git add docs/reference/call-parameters.md docs/explanation/decisions/00007_framing_wrappers.md
git commit -m "docs: document htmx() framing and the HTMXTemplateData split

Assisted-by: Claude:claude-opus-4-8"
```

---

## Self-Review

**Spec coverage:**
- `Framing` enum + `htmx()` recognition + accessor → Task 1. ✓
- Strip framing before representation; `htmx(sse(...))` → Task 1 (verified by `TestDefinitionFraming`). ✓
- `HTMXTemplateData` = base + HX*; `TemplateData` minimal; per-framing emission → Task 2 (aggregator) + Task 4 (per-framing). ✓
- `htmx()` selects render type; `htmx(sse(...))` keeps generic marshaler (only non-SSE render type changes) → Task 4 (render composites; SSE path untouched). ✓
- `--use-htmx` auto-wrap, no opt-out → Task 4 (`effectiveFraming`) + Task 5 (fixtures). ✓
- `--output-htmx-template-data-type` default `HTMXTemplateData` → Task 3. ✓
- Backward compat (templates still call `.HX*`) + migrate `reference_htmx_helpers` → Task 5. ✓
- Edge cases: bad arity (Task 1/4), mixed file emits both types (Task 4), minimal `TemplateData` check-fail (Task 5). ✓
- `muxt check` mode-awareness → covered by `reference_htmx_template_data_minimal` (negative) and the htmx reference fixtures (positive). ✓
- Docs → Task 6. ✓

**Type/name consistency:** `Framing`/`FramingNone`/`FramingHTMX`, `FramingWrapperHTMX`, `Framing()`, `HTMXTemplateDataType`, `templateDataDecls`, `effectiveFraming`, `renderTemplateDataType` are used consistently across tasks. The five parameterized builders gain a `typeName string` param in Task 2 and are called with it from `templateDataDecls`.

**Sequencing / green-at-each-commit:** Task 1 additive (parser only). Task 2 behavior-preserving refactor (identical output). Task 3 additive flag. Task 4 wires htmx framing (and, if the legacy fixture breaks under the new `--use-htmx` emission, migrates it in Step 7 to stay green). Task 5 adds the auto-wrap/split fixtures (no generator change). Task 6 docs. The one ordering risk — `reference_htmx_helpers.txt` breaking at Task 4 — is explicitly handled in Task 4 Step 7.

**Out of scope (correctly deferred):** `datastar()` framing + `FramingDatastar` + moving `.Actions()` to `DatastarTemplateData` (3b); removing the datastar `elements`/`signal` arg families (3c); `script` (blocked).
