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
