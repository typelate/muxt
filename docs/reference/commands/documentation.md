# muxt documentation

Prints the reference, how-to, tutorial, and explanation pages embedded in the binary, so the rules are readable without the muxt source.

```bash
muxt documentation [path]
```

Aliases: `docs`, `d`.

| Invocation | Output |
|---|---|
| `muxt documentation` | An index: one `topic-id — title` line per page, grouped by section. |
| `muxt documentation <path>` | The page's markdown, verbatim. |
| `muxt documentation <directory>` | The path of each file under the directory, one per line. |
| `muxt documentation --format json` | The index as `[{"id", "title", "section"}]`. |

## Paths

A path is a page's repo-relative path, such as `docs/reference/template-names.md`. The `docs/` prefix may be left off, and a leading `./`, a trailing `/`, or a `#fragment` is ignored, so `reference/template-names.md#route-names` also prints the page. The index still prints topic IDs, so build the path from an ID: a `reference` page is `docs/reference/<id>.md`, and any other page is `docs/<id>.md`.

A directory path, such as `docs/how-to`, prints the path of each file under it, one per line.

Links inside a page are relative to the page's own directory. They are not URLs; resolve them from there and pass the result to `muxt documentation`.

`docs/README.md` is the top-level page and is embedded like the others. Example files under `docs/examples` print inside a code fence under their path, so `muxt documentation docs/examples/simple/main.go` prints the path, then the file in a `go` fence.

## Edge cases

- **Topic IDs.** `muxt documentation template-names` and the other ID forms no longer print a page; they fail as unknown paths. This is a breaking change from earlier versions.
