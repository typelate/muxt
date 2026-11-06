# muxt documentation

Prints the documentation pages and example apps embedded in the binary, by path, so the docs for the version you run are readable without the muxt source or a network.

```bash
muxt documentation [path]
```

Aliases: `docs`, `d`.

| Invocation | Output |
|---|---|
| `muxt documentation` | The path of every embedded file, one per line. |
| `muxt documentation <path>` | The file's content. A markdown page prints with its links rewritten (see [Paths](#paths)); any other file prints under its path line in a code fence. |
| `muxt documentation <directory>` | The path of each file under the directory, one per line. |
| `muxt documentation --query <text>` | Every line that holds the text, as `path:line: text`. |

## Search

`--query <text>` (`-q`) prints every line of the embedded docs that holds the text, ignoring case, as `path:line: text`. The text is matched literally, not as a pattern, so `-q ExecuteTemplate` and `-q executetemplate` find the same lines. A path after the flag narrows the search to that file or directory, so `muxt documentation --query htmx docs/how-to` prints only matches under `docs/how-to/`.

## Paths

A path is a file's or directory's repo-relative path, such as `docs/reference/template-names.md`. The `docs/` prefix is optional, and a leading `./`, a trailing `/`, or a `#fragment` is ignored, so `reference/template-names.md#route-names` also prints the page.

Links to other pages are printed as repo-relative paths that `muxt documentation` accepts, so a link such as `../type-checking.md#preconditions` prints as `docs/reference/type-checking.md#preconditions`. A link that leaves docs/ is printed as a GitHub URL for the version this muxt was built from: a release tag, the commit of a development build, or `main` when neither applies.

`docs/README.md` is the top-level page and is embedded like the others. Example files under `docs/examples` print inside a code fence under their path, so `muxt documentation docs/examples/simple/main.go` prints the path, then the file in a `go` fence.

## Edge cases

- **Breaking change.** Names such as `template-names` no longer print a page; they fail as unknown paths. The `--format` flag is gone.
- **No match.** A query that matches no line fails with `no match for "<text>" under <path>`, such as `no match for "muxt rocks" under docs`, and prints nothing to stdout. An empty `--query ""` fails with `--query needs text to search for`.
- **Unknown path.** A path that names no file or directory fails. The error lists up to five of the closest paths, such as `did you mean: docs/reference/cli.md`, or says to run `muxt documentation` to list every path when none is close.
