# Contributing to Muxt

**Mission:** Make server-rendered HTML the primary, strongly-typed surface for HTTP handlers.

## Getting Started

1. **Read the README.md** — Understand what Muxt does and the core concept
2. **Read this file** — You're here!
3. **Run `go test`** — Verify the project works on your machine
4. **Explore `cmd/muxt/testdata/`** — Test files (txtar format) show all features and behavior

## Project Goals

- Templates are the **single source of truth** for routes
- Catch mismatches at **compile time** via `go/types`
- Generated code is **deterministic and idiomatic Go**
- **TDD workflow**: test first, implement, refactor

## Architecture Overview

```
go list (golang.org/x/tools/go/packages)      ./internal/load
    ↓  load.Package, load.GenerateSource, load.RoutesSource
source.Package: types + templates variables    ./internal/source
    ↓  muxt.Definitions, muxt.ResolveCall
Resolved routes (muxt.Definition)              ./internal/muxt
    ↓
Generated files / check reports                ./internal/generate, ./internal/analysis
```

**The package load stops at `internal/load`.** It is the only package that
calls `packages.Load` (the go command, seconds per run). It hydrates a
command's configuration into a `source.Package`: plain data holding the
package's types, and each templates variable's template set, functions,
definitions and ExecuteTemplate calls. Route resolution, generation and the
template checks read only that, so they can be handed values built in memory.

**The standard library is asked, not copied.** Route resolution asks a
`muxt.Checker` what a reserved argument binds to and which types marshal to
and from text. `load.StandardLibrary` is the one implementation, answering from
the official standard library a run loaded. Tests of resolution and generation
use `internal/muxt/muxttest`, which builds the counterfeiter fake in
`internal/muxt/muxtfakes` over stand-in types declared in the test's own
source (`muxttest.StandInChecker(t, pkg)`, or `muxttest.NewChecker()` with
`Binds`, `ParsesFromText` and `FormatsAsText`), so they state muxt's rules
rather than one library version's shape. Regenerate the fake with
`go generate ./internal/muxt`. Tests that need real types use
`internal/load/loadtest`, which type checks package source against the
official standard library's export data without loading the package graph.

**Key concept:** Muxt reads template names like `"GET /{id} GetUser(ctx, id)"` and generates `http.Handler` implementations that:
- Parse URL parameters to the correct Go types
- Call the receiver method with parsed args
- Handle errors and render the template with results

## Development Workflow

### 1. Understand the Scope

For **feature additions or bug fixes**, locate relevant test files:

```bash
# Find all tests for a specific feature
ls cmd/muxt/testdata/ | grep your-feature

# If it doesn't exist, you're adding a new feature
```

### 2. Test-First: Add or Update Tests

Tests are `txtar` files (text archive format) in `cmd/muxt/testdata/`:

```bash
# Extract a test to inspect it
mkdir -p ./cmd/your-test
cd ./cmd/your-test
txtar -extract ../muxt/testdata/your-test.txt
# Files are extracted, edit them normally, but make test changes in the original txtar file.


# Run tests in the extracted directory
go -C ./cmd/muxt/testdata/your-test test

# Clean up when done (already gitignored)
rm -rf ./cmd/muxt/testdata/your-test
```

### 3. Run Tests Frequently

```bash
# Test a single package
go test ./cmd/muxt

# Test a specific test
go test ./cmd/muxt -run TestName

# Run all tests (only when making cross-package changes)
go test ./...
```

### 4. Implement Changes

Update the code in order:
1. `internal/muxt/` — Route name parsing and call resolution
2. `internal/generate/` or `internal/analysis/` — What is written or reported
3. `internal/load/` — Only if a run needs something new from the loaded packages
4. `internal/cli/` — CLI handling (if needed)

Before adding an integration script, see whether a unit test can state it,
at the layer that owns the behavior:
- **Flags:** `internal/cli/configurations_test.go` states what a command line
  parses into, and which command lines are rejected, without loading anything.
- **What a command does with a valid configuration:**
  `internal/{generate,analysis}/testdata/<command>/*.txtar` snapshot generated
  files, check reports and the route and template listings, from packages
  loaded in memory in milliseconds. The directory names the command; each
  archive's `config.json` is the configuration the command line in its header
  parses into. Run one with
  `go test ./internal/analysis -run TestSnapshots/list-template-calls/calls`,
  and rewrite them with
  `go test ./internal/{generate,analysis} -run TestSnapshots -update`, then
  review the diff.

Integration scripts are for what needs the go command: generated code
compiling and serving requests, and files on disk.

### 5. Verify Your Changes

```bash
# Check for build/type errors
go test ./cmd/muxt

# Run the formatter
go fmt ./...
gofumpt -w .
```

## Common Tasks

### Adding a New Feature

1. Create a test file: `cmd/muxt/testdata/reference_my_feature.txt`
2. Define the expected input (template) and output (generated code)
3. Run the test to see it fail
4. Update `internal/muxt/` generator functions
5. Run `go test ./cmd/muxt` until it passes

### Fixing a Bug

1. Create a test file: `cmd/muxt/testdata/err_bug_description.txt` or update an existing test
2. Reproduce the bug in the test
3. Run `go test ./cmd/muxt` to confirm failure
4. Fix the bug in `internal/muxt/`
5. Run `go test ./cmd/muxt` to confirm the fix

### Adding Error Detection

1. Create a test: `cmd/muxt/testdata/err_error_name.txt`
2. Define input that should produce an error
3. Add validation logic to `internal/muxt/`
4. Verify the error message is clear

### Improving Documentation

- User-facing docs: Update files in `docs/`
- Developer docs: Update this file or add inline code comments
- Generator behavior: Update test names and comments in `cmd/muxt/testdata/`

## Test Naming Convention

Format: `[category]_[feature]_with_[details].txt`

| Category | Purpose | Example |
|----------|---------|---------|
| `tutorial_*` | Complete learning examples | `tutorial_blog_example.txt` |
| `howto_*` | Task-oriented guides | `howto_form_basic.txt` |
| `reference_*` | Feature documentation (happy path) | `reference_status_codes.txt` |
| `err_*` | Error condition documentation | `err_duplicate_pattern.txt` |

Find tests by category:
```bash
ls cmd/muxt/testdata/tutorial_*.txt
ls cmd/muxt/testdata/reference_*form*.txt
ls cmd/muxt/testdata/err_*.txt
```

## Key Files and Directories

### Source Code
- `internal/load/` — Package loading (the only `go/packages` caller), hydration into `source.Package`, and load diagnostics
- `internal/source/` — The loaded package as plain data: what everything after the load reads
- `internal/muxt/` — Template name parsing and route resolution against go/types
- `internal/generate/` — Routes file generation
- `internal/analysis/` — `muxt check` and the template listings
- `internal/muxt/muxtfakes/` — The counterfeiter fake of `muxt.Checker`, generated by `go generate ./internal/muxt`
- `internal/muxt/muxttest/` — Builds that fake from what a test says the standard library looks like, plus import-free type checking
- `internal/load/loadtest/` — A loaded package type checked against the official standard library, for tests that go through `internal/load`
- `internal/cli/` — Command-line interface
- `cmd/muxt/` — Command entry point

### Tests & Examples
- `cmd/muxt/testdata/` — Test cases (txtar format) - **START HERE**
- `docs/examples/` — Complete working examples

### Documentation
- `docs/reference/` — Feature reference
- `docs/explanation/` — Design philosophy
- `docs/skills/` — Claude Code skills for AI assistants

## Important Implementation Details

### Template Embedding
`//go:embed` requires explicit patterns per directory level:
```go
//go:embed *.gohtml */*.gohtml
var templateFS embed.FS
```

### Regenerating Code
After changes to generator code, regenerate test outputs:
```bash
go generate ./...
```

### Testing Generated Code
Each test extracts to a temporary directory with a valid Go module:
```bash
go -C ./cmd/muxt/testdata/test-name test
```

## Debugging Tips

### Understand Test Structure
Open a txtar file to see the structure:
```bash
cat cmd/muxt/testdata/reference_basic.txt
```

Each txtar contains:
- Template files (`.gohtml`)
- Go files (`.go`)
- Expected output (`_gen_` files)
- Optional error assertions

### Run Specific Tests
```bash
# Extract a test
mkdir -p ./cmd/muxt/testdata/debug-test
cd ./cmd/muxt/testdata/debug-test
txtar -extract ../your-test.txt

# Edit files, run tests
cd ../../../..
go -C ./cmd/muxt/testdata/debug-test test -v
```

## Performance Considerations

- Generator should be fast (run during `go generate`)
- Generated code has no runtime reflection
- All type checking happens at generation time

## Pull Request Checklist

- [ ] Tests pass: `go test ./...`
- [ ] Code formatted: `go fmt ./...` and `gofumpt -w .`
- [ ] New features have test files with clear naming
- [ ] Error conditions are documented with `err_*` tests
- [ ] No unnecessary changes to generated output
- [ ] Documentation updated if user-facing behavior changed

## Questions?

- Check `docs/reference/template-names.md` for template syntax reference
- Review existing test files in `cmd/muxt/testdata/`
- Open an issue with a minimal example
