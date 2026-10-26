# muxt documentation

Prints the reference, how-to, tutorial, and explanation pages embedded in the binary, so the rules are readable without the muxt source.

```bash
muxt documentation [topic]
```

Alias: `docs`.

| Invocation | Output |
|---|---|
| `muxt documentation` | An index: one `topic-id — title` line per page, grouped by section. |
| `muxt documentation <topic>` | The page's markdown, verbatim. |
| `muxt documentation --format json` | The index as `[{"id", "title", "section"}]`. |

## Topic IDs

A topic ID is the page's path under `docs/` without `.md`. Pages under `reference/` drop that prefix.

| Section | Example ID |
|---|---|
| `reference` | `template-names`, `call-parameters`, `commands/generate` |
| `how-to` | `how-to/template-functions` |
| `tutorials` | `tutorials/quick-start` |
| `explanation` | `explanation/manifesto`, `explanation/decisions/00002_use_code_generation_instead_of_reflection` |

`reference/template-names` and `template-names.md` also resolve.

Links inside a page are relative to the muxt repository's `docs/` directory. They are not URLs; resolve them to another topic ID.

An unknown topic exits 1 and lists up to five topics whose ID contains the input (or one of its words).

`docs/examples` is not embedded.
