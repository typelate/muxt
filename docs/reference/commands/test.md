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
- `-mod` and `-modfile`: muxt test sets `-modfile` to a copy of `go.mod` that requires `github.com/typelate/htt`, and `-mod=mod` so the go command can complete the copy's sums.
- A `-coverpkg` that covers `html/template`: by name, as `std` or `all`, or by a `...` pattern that matches it. Coverage instrumentation reads `html/template` from disk, which would undo the engine. `-coverpkg=./...` works.
- `-C`: `go test` reads it only as its first flag. Use muxt's own: `muxt -C sub test`.

## Failures

When `go test` fails, `muxt test` exits `1` and adds nothing to what `go test` printed, except in a run over every entry ([Selection](#selection)). Every failure exits `1`, `go test`'s `2` included.

## Selection

With a [muxt.json](../muxt-json.md) in the working directory, every entry runs, in key order, one `go test` each. Anywhere else the package directory runs alone, as `muxt check` selects it.

A run over every entry writes the entry's key on its own line before that package's output. With `-json` after `--` it writes no key, so the output stays a stream of JSON events.

muxt puts the engine in place for every package before the first `go test` runs. When one package cannot have it, no `go test` runs and the error is `cannot run the tests on muxt's template engine: <reason>`. With a `go.work` in use the reason is `go.work is in use, and the go command refuses muxt test's -modfile in workspace mode: go: -modfile cannot be used in workspace mode`.

The first package that fails ends the run; the entries after it are not tested. Its last line is `Error: <key>: go test failed`.
