# muxt documentation

Prints the reference, how-to, tutorial, and explanation pages embedded in the binary, so the rules are readable without the muxt source.

```bash
muxt documentation [path]
```

Aliases: `docs`, `d`.

| Invocation | Output |
|---|---|
| `muxt documentation` | The path of every embedded file, one per line. |
| `muxt documentation <path>` | The file's content. A markdown page prints verbatim; any other file prints under its path line in a code fence. |
| `muxt documentation <directory>` | The path of each file under the directory, one per line. |

## Paths

A path is a file's or directory's repo-relative path, such as `docs/reference/template-names.md`. The `docs/` prefix may be left off, and a leading `./`, a trailing `/`, or a `#fragment` is ignored, so `reference/template-names.md#route-names` also prints the page.

With no argument the path is the directory `docs`, so the command prints the path of every embedded file, one per line. A directory path, such as `docs/how-to`, prints the path of each file under it, one per line.

Links to other pages are printed as repo-relative paths that `muxt documentation` accepts, so a link such as `../type-checking.md#preconditions` prints as `docs/reference/type-checking.md#preconditions`. A link that leaves docs/ is printed as a GitHub URL for the version this muxt was built from: a release tag, the commit of a development build, or `main` when neither applies.

`docs/README.md` is the top-level page and is embedded like the others. Example files under `docs/examples` print inside a code fence under their path, so `muxt documentation docs/examples/simple/main.go` prints the path, then the file in a `go` fence.

## Edge cases

- **Breaking change.** Names such as `template-names` no longer print a page; they fail as unknown paths. The `--format` flag is gone.
- **Unknown path.** A path that names no file or directory fails. The error lists up to five of the closest paths, such as `did you mean: docs/reference/cli.md`, or says to run `muxt documentation` to list every path when none is close.
