# Agent Prompt for Developing Muxt

**Mission:**
Make server-rendered HTML the primary, strongly-typed surface for HTTP handlers — safe, testable, and reproducible with predictable generation and minimal runtime complexity.

---

## Goals and Philosophy

* Treat templates as the **single source of truth**: declare routes and parameters in template names, generate Go glue automatically.
* Catch mismatches **at compile time** using `go/types`.
* Ensure all generated code is **deterministic, reproducible, and idiomatic Go**.
* Prioritize **clarity, simplicity, and maintainability** in generated code, test cases, and documentation.
* Apply **small, iterative steps**: design tests first, implement, pass tests, refactor.

---

## Workflow Guidance

* Explore the codebase using Go tooling (`go doc`, `go test`, `go mod why`).
* Examine Git history to understand project evolution.
* Learn from `cmd/muxt/testdata/` — use script-driven tests to understand CLI behavior.
* Create examples in `docs/example` and experiment with `go run github.com/typelate/muxt`.
* Update generator code (`internal/muxt`) and tests (`cmd/muxt/testdata/`) together using TDD:

    1. Write a test demonstrating the desired template behavior.
    2. Implement generator changes in `internal/muxt` and CLI handling in `internal/cli`.
    3. Ensure all tests pass.
    4. Refactor code for readability, maintainability, and minimal duplication.
    5. Add AST helpers in `internal/source` if needed.

---

## Template Name Syntax

```
[METHOD ][HOST]/[PATH][ HTTP_STATUS][ CALL]
```

* `METHOD` — optional (e.g., `GET`, `POST`), defaults to all
* `HOST` — optional, matches a specific host
* `PATH` — required, may include `{param}` placeholders
* `HTTP_STATUS` — optional expected success code (e.g., `200`)
* `CALL` — optional Go method invocation with typed parameters

**Example:**

```gohtml
{{define "GET /greet/{language} 200 Greeting(ctx, language)"}} ... {{end}}
```

---

## Call Parameters

* **`ctx`** → `context.Context`
* **`request`** → `*http.Request`
* **`response`** → `http.ResponseWriter`
* **Path parameters** → extracted from `{name}` segments, typed as `string` by default
* **Form/query parameters** → matched by name to method arguments, parsed to appropriate types (`int`, `bool`, `string`)
* **No `CALL`** → execute the template directly

**Example method signatures:**

```go
Greeting(ctx context.Context, language string) error
Login(ctx context.Context, username, password string) (User, error)
Upload(response http.ResponseWriter, request *http.Request) error
```

---

## Core Principles

* Templates are **contracts**, Go code implements behavior.
* Generation ensures **compile-time safety and reproducibility**.
* Adding a new endpoint is **writing a template**; wiring is automatic.
* Generated files are **deterministic artifacts**.
* Script tests and instrumentation provide **observability and validation**.
* Runtime is **lightweight**; generation performs heavy lifting.

---

## Generated File Structure

Muxt generates multiple files based on template source files:

* **`template_routes.go`** — Main file containing:
  - `RoutesReceiver` interface (embeds per-file interfaces)
  - `TemplateRoutes()` function (orchestrates per-file functions)
  - `TemplateData[T]` type and methods
  - `TemplateRoutePaths` type with path helper methods

* **`*_template_routes_gen.go`** — One file per `.gohtml` source file:
  - File-scoped receiver interface (e.g., `IndexRoutesReceiver`)
  - File-specific route function (e.g., `IndexTemplateRoutes()`)
  - HTTP handlers for templates in that file

Templates from `template.Parse()` calls (no source file) remain in main `template_routes.go`.

---

## Documentation Approach

Follow [Diátaxis](http://diataxis.fr/):

* **Tutorials** — step-by-step guides to accomplish real tasks.
* **How-to guides** — actionable recipes (e.g., add a route, test a template).
* **Reference** — complete CLI, template syntax, and generated code specifications.
* **Explanation** — design reasoning, tradeoffs, and Go tooling integration.

**Tone and style:**

* Clear, concise, and direct
* Actionable with practical next steps
* Compact and repeatable
* Pragmatic, focused on working examples
* Precise, technical, and approachable

---

## Test-Driven Development for Agents

* Start by **writing a test first** to define desired behavior.
* Ensure tests **fail initially** to validate that they exercise functionality.
* Implement **minimal code** to pass the test.
* Refactor code and tests for clarity and maintainability.
* Repeat in small, incremental steps.

**XP-style principles:**

* Seek **early feedback** through tests
* Implement **simplest possible solutions**
* Use tests to create **shared understanding**
* Refactor confidently with tests as a **safety net**

**Agent guidance:**

* Work in `cmd/muxt/testdata/` for rapid verification
* Encode requested behavior in tests before implementing
* Take small, incremental steps
* Use examples to demonstrate progress (failing → passing)
* `// go:embed` does not support double star glob patterns you must add every level of directory as a separate field `go:embd *.gohtml */*.gohtml */*/*.gohtml`

---

## Test File Naming Convention

Tests in `cmd/muxt/testdata/` follow Diátaxis-inspired naming:

**Format:** `[category]_[feature]_with_[details].txt`

**Categories:**

* **`tutorial_*`** — Learning-oriented examples (3 tests)
  - Complete, working examples that teach concepts
  - Examples: `tutorial_blog_example.txt`, `tutorial_basic_route.txt`

* **`howto_*`** — Task-oriented guides (15 tests)
  - Show how to accomplish specific tasks
  - Examples: `howto_arg_context.txt`, `howto_form_basic.txt`, `howto_call_with_multiple_args.txt`

* **`reference_*`** — Feature documentation (40+ tests)
  - Document what features exist and their syntax (happy path)
  - Examples: `reference_status_codes.txt`, `reference_form_field_types.txt`, `reference_call_with_two_returns.txt`

* **`err_*`** — Error condition reference (19 tests)
  - Reference documentation for error cases (non-zero exit, stderr output)
  - Show what error messages developers get for various mistakes
  - Examples: `err_duplicate_pattern.txt`, `err_form_with_undefined_method.txt`, `err_arg_context_type.txt`

**Naming patterns:**

* Use `with` to improve readability: `reference_call_with_bool_return.txt`
* Use underscores consistently: `reference_receiver_with_embedded_method.txt`
* Be descriptive: `reference_form_with_slice_field.txt` (not `reference_form_slice.txt`)

**Finding tests:**

```bash
# List by category
ls cmd/muxt/testdata/tutorial_*.txt
ls cmd/muxt/testdata/howto_*.txt
ls cmd/muxt/testdata/reference_*.txt
ls cmd/muxt/testdata/err_*.txt

# Find specific features
ls cmd/muxt/testdata/*form*.txt        # All form-related tests
ls cmd/muxt/testdata/*call*.txt        # All call-related tests
ls cmd/muxt/testdata/reference_*arg*.txt  # Reference for arguments
```

**When adding new tests:**

1. **Tutorial** - Is this a complete, working example teaching a concept? Probably not needed.
2. **How-to** - Does this show how to accomplish a specific task? → `howto_[task].txt`
3. **Reference (happy path)** - Does this document a feature that works? → `reference_[feature]_with_[detail].txt`
4. **Reference (error path)** - Does this document an error condition? → `err_[condition].txt`
