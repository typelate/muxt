# muxt test-template-mutations

Vary each dynamic and control flow action in your templates, one at a time, and re-run the tests against each variation.

A variation the tests still pass through is a **miss**: nothing the suite asserts on depends on what that action does. A variation that makes a test fail is **caught**, which is the outcome to want.

```bash
muxt test-template-mutations
```

```
baseline ok
KILL index.gohtml:12:5 "GET /{$} Index()" action-empty
MISS index.gohtml:18:3 "GET /{$} Index()" if-false
KILL index.gohtml:24:1 "GET /{$} Index()" range-never
3 mutants, 2 killed, 1 missed
```

Each `MISS` line names a template, a place in it, and the variation nothing noticed — which is enough to write the missing assertion.

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--template-pattern` | string | _(all)_ | Only mutate templates whose name matches this regular expression. |
| `--run` | string | _(all)_ | Only run tests matching this regular expression. Passed to `go test -run`. |
| `--use-templates-variable` | string[] | `templates` | Global `*template.Template` variable name(s) to read templates from. |
| `--format` | string | `text` | `text` or `json`. |

Package patterns may be passed as arguments. The default is `./...`, so a mutation caught only by a test in another package is still reported as caught.

```bash
muxt test-template-mutations --template-pattern '^GET /users' --run TestUsers ./web/...
```

## Operators

One mutant is produced per applicable action. No operator renames a template or changes which templates exist, so a mutant never moves a route.

| Operator | Applies to | Variation |
|----------|-----------|-----------|
| `action-empty` | `{{.Field}}`, `{{printf ...}}` | Prints nothing. |
| `if-true` | `{{if}}` | Takes the then branch unconditionally. |
| `if-false` | `{{if}}` | Takes the else branch, or none, unconditionally. |
| `with-empty` | `{{with}}` | Behaves as though the value were absent. |
| `range-never` | `{{range}}` | Iterates zero times, leaving the else branch. |
| `template-drop` | `{{template}}` | Removes the call. |

A pipeline that declares variables keeps its declarations and has only its value replaced, so `{{range $i, $item := .Items}}` is mutated without leaving `$item` undefined.

`{{block}}` is not dropped: a block defines its body in place, so removing its call would orphan the body's `{{end}}`.

## Baseline

The tests run once unmutated before anything is varied. If that baseline fails, nothing is mutated and the command exits non-zero:

```
Error: baseline tests failed before mutation; fix them first:
--- FAIL: TestIndex (0.00s)
```

Every mutant would otherwise be recorded as caught — by the failure that was already there rather than by the mutation.

## How Variations Are Delivered

Mutants reach the test run through the go command's [`-overlay`](https://pkg.go.dev/cmd/go#hdr-Build_and_test_caching) flag, which replaces a file for the build without writing to your working tree. The overlay reaches `//go:embed` content, so an embedded template is varied without touching the checkout.

Templates written as Go string literals are mutated too: the variation is spliced into the literal's text and the literal is re-encoded, so the overlay replaces the `.go` file. Positions are reported in that file.

> `golang.org/x/tools/go/packages` has an `Overlay` field of its own, but it does not reach `//go:embed` content — `go list` reports the original path for an embedded file, so anything reading that path still sees the unmutated bytes.

## Interpreting a Report

A miss is not automatically a bug. It means one of:

- **A missing assertion.** The common case: the test renders the template but never checks the part that action controls.
- **An untested branch.** `if-false` surviving usually means no test renders with that condition false.
- **An equivalent mutant.** The variation genuinely cannot change observable output. Rare, but real for actions whose value is always empty in every case worth testing.

Work down the misses; each one closed is an assertion the suite did not have.

## JSON Output

`--format json` carries the mutated action's source and its replacement, which the text form leaves out:

```json
{
	"baseline": {
		"passed": true
	},
	"results": [
		{
			"status": "MISS",
			"operator": "if-false",
			"template": "greeting",
			"file": "templates.gohtml",
			"line": 1,
			"column": 39,
			"original": "{{if .Loud}}",
			"mutated": "false"
		}
	],
	"total": 1,
	"killed": 0,
	"missed": 1
}
```

## Limitations

- Custom delimiters are not supported; templates are scanned with `{{` and `}}`.
- Each mutant is a full `go test -count=1` run, so a large suite takes a while. Narrow it with `--template-pattern`, `--run`, and a package argument.

## Related

- [muxt check](check.md) — Type-check templates without running them
- [muxt list-template-callers](list-template-callers.md) — List callers of a template
