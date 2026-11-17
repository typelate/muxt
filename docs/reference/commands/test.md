# muxt test

Runs `go test` with muxt's template engine in place of `html/template`.

```text
muxt test [-- go test flags] [flags]
```

Everything after `--` goes to `go test` as written, after `-count=1`; a later `-count` wins. With nothing after `--`, `go test` gets no package pattern and tests the package in the directory.

`go test`'s stdout and stderr reach yours unchanged. In a healthy run muxt prints nothing of its own.

The files that put the engine in place are written to a temporary directory, removed when the command ends, an interrupt included.

## Refused arguments

muxt refuses these before anything runs:

- An argument before `--`: `muxt test web` fails with `test takes no arguments before --; pass go test flags after --: web`.
- `-overlay`: muxt test puts its engine in place through its own overlay.

## Failures

When `go test` fails, `muxt test` exits `1` and adds nothing to what `go test` printed. Every failure exits `1`, `go test`'s `2` included.
