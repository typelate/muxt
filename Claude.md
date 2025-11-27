# Developing Muxt

**Mission:** Make server-rendered HTML the primary, strongly-typed surface for HTTP handlers.

## Goals

- Templates are the **single source of truth** for routes
- Catch mismatches at **compile time** via `go/types`
- Generated code is **deterministic and idiomatic Go**
- **TDD workflow**: test first, implement, refactor

## Development Workflow

1. Learn from `cmd/muxt/testdata/` txtar files
2. Update generator (`internal/muxt`) and tests together
3. Add AST helpers in `internal/source` if needed

### Iterating on Tests

Tests in `cmd/muxt/testdata/` are txtar files:

```bash
# Extract and work on a test
mkdir -p ./cmd/muxt/testdata/some-test
cd ./cmd/muxt/testdata/some-test && txtar -extract ../some-test.txt

# Run Go commands in working directory
go -C ./cmd/muxt/testdata/some-test test

# Clean up when done (already gitignored)
rm -rf ./cmd/muxt/testdata/some-test
```

## Test Naming Convention

Format: `[category]_[feature]_with_[details].txt`

| Category | Purpose | Example |
|----------|---------|---------|
| `tutorial_*` | Complete learning examples | `tutorial_blog_example.txt` |
| `howto_*` | Task-oriented guides | `howto_form_basic.txt` |
| `reference_*` | Feature documentation (happy path) | `reference_status_codes.txt` |
| `err_*` | Error condition documentation | `err_duplicate_pattern.txt` |

```bash
# Find tests by category
ls cmd/muxt/testdata/tutorial_*.txt
ls cmd/muxt/testdata/reference_*form*.txt
ls cmd/muxt/testdata/err_*.txt
```

## Key Directories

- `internal/muxt/` — Generator code
- `internal/cli/` — CLI handling
- `internal/source/` — AST helpers
- `cmd/muxt/testdata/` — Test cases (txtar)
- `docs/prompts/` — User-facing prompts for Claude Code

## Notes

- `//go:embed` requires explicit patterns per directory level: `*.gohtml */*.gohtml`
- Run `go generate ./...` to regenerate after changes
- See `docs/prompts/muxt-guide.md` for template syntax reference