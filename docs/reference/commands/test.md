# muxt test

Runs `go test` with muxt's template engine in place of `html/template`. With `--output-cover-profile` it writes the template blocks the tests executed as a Go cover profile for `go tool cover -html`.

```text
muxt test [-- go test flags] [flags]
```

To read the profile, see [See which template blocks your tests run](../../how-to/template-coverage.md).

Everything after `--` goes to `go test` as written, after `-count=1`; a later `-count` wins. With nothing after `--`, `go test` gets no package pattern and tests the package in the directory.

`go test`'s stdout and stderr reach yours unchanged. In a healthy run muxt prints nothing of its own.

The files that put the engine in place are written to a temporary directory, removed when the command ends, an interrupt included. The copy of `go.mod` among them may raise its `go` line to the engine's; your `go.mod` stays as written and muxt reports neither. [reference_template_coverage_quiet_go_line.txt](../../../cmd/muxt/testdata/reference_template_coverage_quiet_go_line.txt)

## Refused arguments

muxt refuses these before anything runs:

- An argument before `--`: `muxt test web` fails with `test takes no arguments before --; pass go test flags after --: web`.
- `-overlay`: muxt test puts its engine in place through its own overlay.
- `-mod` and `-modfile`: muxt test sets `-modfile` to a copy of `go.mod` that requires `github.com/typelate/htt`, and `-mod=mod` so the go command can complete the copy's sums.
- A `-coverpkg` that covers `html/template`: by name, as `std` or `all`, or by a `...` pattern that matches it. Coverage instrumentation reads `html/template` from disk, which would undo the engine. `-coverpkg=./...` works.
- A package pattern after `--` together with `--output-cover-profile`: each package needs its own `go test` to be told apart in the profile. Run in the package directory, or list the packages in `muxt.json`.
- `-C`: `go test` reads it only as its first flag. Use muxt's own: `muxt -C sub test`.

## Failures

When `go test` fails, `muxt test` exits `1` and adds nothing to what `go test` printed, except in a run over every entry ([Selection](#selection)). Every failure exits `1`, `go test`'s `2` included.

## Selection

With a [muxt.json](../muxt-json.md) in the working directory, every entry runs, in key order, one `go test` each. Anywhere else the package directory runs alone, as `muxt check` selects it.

A run over every entry writes the entry's key on its own line before that package's output. With `-json` after `--` it writes no key, so the output stays a stream of JSON events.

muxt puts the engine in place for every package before the first `go test` runs. When one package cannot have it, no `go test` runs and the error is `cannot run the tests on muxt's template engine: <reason>`. With a `go.work` in use the reason is `go.work is in use, and the go command refuses muxt test's -modfile in workspace mode: go: -modfile cannot be used in workspace mode`.

The first package that fails ends the run; the entries after it are not tested. Its last line is `Error: <key>: go test failed`.

## Cover profile

`--output-cover-profile FILE` writes the template blocks the tests reached to `FILE` when the run ends, as a Go cover profile (`mode: set`). Each line names a template file by absolute path, so a run over several packages writes one profile with every package's blocks. A run that reaches no template writes the mode line alone. A failing run still writes the profile of what ran.

The profile holds reached blocks only, so `go tool cover -html` shows unreached template text as not tracked rather than uncovered. It leaves out string templates, files read at run time, and templates parsed only in test files or only under build tags.

The directory of `FILE` must exist. When it does not, no `go test` runs and the error is `--output-cover-profile: stat <directory>: no such file or directory`, the directory by absolute path. When it is a file, the error is `--output-cover-profile: <directory> is not a directory`.

A package whose tests wrote no profile, as with no test files, is a warning on stderr and leaves its blocks out; the exit status is `go test`'s:

```text
warning: no test binary in /abs/pkg ran muxt's template engine; the cover profile has nothing from that package
```

A package that does not load, or has no templates variable, cannot be attributed. Its blocks are left out with a warning on stderr, and the exit status is `go test`'s:

```text
warning: cannot read the template files of /abs/pkg (reading the templates: variable templates not found in package example.com/pkg); the cover profile has nothing from that package
```

A parse name is a file's base name. When a local file and a shared fragment from another package's `embed.FS` have the same one, blocks reached under it cannot be placed in one file. They are left out with a warning naming both files, and the exit status is `go test`'s:

```text
warning: /abs/pkg/index.gohtml and /abs/ui/index.gohtml share the parse name index.gohtml; the cover profile leaves out its blocks
```

## Differences from go test

The tests run on muxt's template engine, not on `html/template`; `go test` stays the check of correctness. The engine still escapes script and URL attributes differently, so a test that asserts such output, a Datastar `data-on-click` among them, passes under `go test` and fails here. [reference_template_coverage_escaping_gap.txt](../../../cmd/muxt/testdata/reference_template_coverage_escaping_gap.txt)
