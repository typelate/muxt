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
Template Name (with route pattern and method call)
    ↓
Parser (./internal/muxt/parse)
    ↓
Type Checker (go/types ./internal/analysis)
    ↓
Generator (./internal/muxt/generate)
    ↓
HTTP Handler Code
```

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

Update the generator code in order:
1. `internal/muxt/` — Core generation logic
2. `internal/source/` — AST helpers (if needed)
3. `internal/cli/` — CLI handling (if needed)

### 5. Verify Your Changes

```bash
# Check for build/type errors
go test ./cmd/muxt

# Run the formatter
go fmt ./...
gofumpt -w .
```

## Common Tasks

### Exploring Existing Behavior

To explore existing functionality in a smaller scope.
- Create a main.go file in a new directory inside ./cmd/tmp/<some-dir>
- Run the new executable package with `go run ./cmd/tmp/explore-asterr`

**Do not do this in /tmp!**
**Do not add go.mod to ./cmd/tmp directories otherwise running muxt will not work!**

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
- `internal/muxt/` — Generator logic (parse, type check, generate)
- `internal/source/` — AST analysis helpers
- `internal/cli/` — Command-line interface
- `cmd/muxt/` — Command entry point

### Tests & Examples
- `cmd/muxt/testdata/` — Test cases (txtar format) - **START HERE**
- `docs/examples/` — Complete working examples

### Documentation
- `docs/tutorials/` — Getting started guides
- `docs/how-to/` — Task-oriented guides
- `docs/reference/` — Feature reference
- `docs/explanation/` — Design philosophy
- `docs/prompts/` — Prompts for AI assistants

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

- Check `docs/prompts/muxt-guide.md` for template syntax reference
- Review existing test files in `cmd/muxt/testdata/`
- Open an issue with a minimal example