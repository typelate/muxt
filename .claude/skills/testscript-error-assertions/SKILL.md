---
name: testscript-error-assertions
description: How to assert full error messages containing absolute paths in rsc.io/script test files under ./cmd/muxt/testdata/*.txt. Use whenever writing, editing, or debugging a .txt script in this repo, or whenever an assertion involves an error message or a file path — even if scripttest isn't mentioned by name.
---

# Asserting full error messages in rsc.io/script

Tests live in `./cmd/muxt/testdata/*.txt`, run by `scripttest.Test`.

`stdout`/`stderr` are unanchored regexp searches — they prove a substring
appeared somewhere, which is not "this is the error message." Use `cmpenv`: it
expands `$VAR` in both sides, then compares byte-exact. Args are
`cmpenv <actual> <expected>`, where actual may be `stdout` or `stderr`.

Absolute paths go in the golden file as `$WORK` — `scripttest` sets it to the
test's `t.TempDir()`, which is also the script's cwd.

```txt
! muxt check
cmpenv stderr want-err.txt

-- template.gohtml --
{{define "GET /x F()"}}{{end}}
-- want-err.txt --
$WORK/template.gohtml:1:1: F not found in package
Error: fail: 1 error
```

The final `Error: <summary>` line is cobra's error printer — muxt's stderr
always ends with it, newline included.

- `stderr`, not `stdout` — check where the tool actually writes.
- `cmpenv` doesn't check the exit code; keep the leading `!`.
- There is no `-update` mode. Golden files are written by hand; never paste a
  real `/tmp/...` path into one.
- Use `${/}` for the path separator when a golden file must pass on Windows
  (`${:}` is the list separator). Both are built in — but see the quoting
  rule below: they only expand where expansion happens at all.
- macOS: `$WORK` is `/var/folders/...` and rsc.io/script does **not** resolve
  symlinks (unlike `go-internal/testscript`). A `/private/var` vs `/var`
  mismatch means the tool is resolving the path itself — fix it there, don't
  paper over it in the assertion.

For one line inside nondeterministic output, an anchored `stderr` is fine,
but mind the quoting rule: **variables never expand inside single quotes** —
a `$WORK` or `${/}` in a quoted pattern stays literal and silently never
matches. Close the quote, let the variable sit unquoted, then reopen it;
unquoted variables are `regexp.QuoteMeta`'d during expansion, so the value is
matched literally while the rest of the pattern stays a regexp (Go's RE2 has
no `\Q...\E`):

```txt
stderr '(?m)^'$WORK'/template\.gohtml:1:1: F not found in package$'
stderr '(?m)^probe'${/}'probe\.go:[0-9]+:[0-9]+: undefined: server\.TemplateData$'
```