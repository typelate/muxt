# web-platform-tests white-space fixtures

Copied from https://github.com/web-platform-tests/wpt at commit
`8548e92382323b94b16a0ce524b5675d1b464a74`, under 3-Clause BSD (see `LICENSE.md`).
Each `.html` file has a provenance comment prepended as its first line;
nothing else was changed. They are verified byte-identical (apart from that
line) to the copies the htt brainstorm fetched earlier.

Fetch command, per file:

    curl -fsSL https://raw.githubusercontent.com/web-platform-tests/wpt/8548e92382323b94b16a0ce524b5675d1b464a74/css/css-text/white-space/<file>

- `reference/<file>` comes from `css/css-text/white-space/reference/<file>`.
- `top/<file>` comes from `css/reference/<file>` (the `../../reference/` target of the tests).
- `LICENSE.md` comes from the repository root.
- `white-space-empty-text-sibling.html` is not vendored (scripted).

Used by `internal/rendering` `TestWPTOracle` (the oracle) and the formatter cases.

Oracle coverage and exclusions:

- Checked: `white-space-collapse-000/001`, `seg-break-transformation-000.tentative`,
  `line-edge-white-space-collapse-001/002`, `trailing-space-before-br-001`,
  `pre-line-with-space-and-newline`.
- `trailing-space-before-br-001`: rows ending in `<br>` are compared with each
  other, not with the `<br>`-less row 1. A `<br>` is content to the oracle; WPT
  measures widths, where a trailing `<br>` adds no width.
- `seg-break-transformation-000.tentative`: asserts CSS Text 4 segment break to
  space; the oracle keeps U+000A distinct from U+0020, so the pairs are not equal.
- Other files are layout-measurement or pre/pre-wrap reftests (the pair differ
  structurally or by styling), not whitespace-equivalence pairs; they are
  inputs for formatter cases only.
