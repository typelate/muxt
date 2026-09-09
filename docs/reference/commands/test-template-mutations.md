# muxt test-template-mutations

Vary each dynamic and control flow action in your templates, one at a time, and re-run the tests against each variation.

A variation the tests still pass through is a **miss**: nothing the suite asserts on depends on what that action does. A variation that makes a test fail is **caught**, which is the outcome to want.

```bash
muxt test-template-mutations
```

```
20 mutants across 6 templates (complexity 10)
baseline ok

template_routes.go:38:13 ExecuteTemplate "/ Count()" (dot: *main.TemplateData[main.RoutesReceiver, int64])
  "/ Count()" template.gohtml (complexity 1, dot: *main.TemplateData[main.RoutesReceiver, int64])
    MISS 14:2 template-drop
  "count" template.gohtml via {{template}} (complexity 1, dot: int64)
    MISS 62:19 action-zero

20 mutants, 18 killed, 2 missed, 0 skipped
```

Each `MISS` line names a template, a place in it, and the variation nothing noticed — enough to write the missing assertion.

## Mutation Happens in the Context of a Render

A template action means nothing on its own. `{{.Total}}` is a field access only because of what dot is where the template is rendered, so traversal starts at each `templates.ExecuteTemplate` call and walks `{{template}}` invocations depth first from there, carrying dot along.

Depth first is what keeps the report readable: everything about one template, then the partials it renders, before moving to the next call site.

Two consequences:

- **A template nothing renders is not mutated.** If no `ExecuteTemplate` call is found at all, the command says so rather than reporting an empty run.
- **A template is mutated once per type of dot**, across the whole run. Two calls rendering it with the same input would produce the same mutants and the same verdicts, so the second subtree is trimmed. `-v` lists what was trimmed and where it was first reached.

Calls in `_test.go` files are ignored by default — a template rendered only by a test is not rendered in production, and mutating it measures the tests against themselves. Pass `--include-test-callers` to opt in.

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | `false` | Enumerate and report the mutants without running any tests. |
| `--verbose`, `-v` | bool | `false` | Report every mutant, not only the misses, and stream progress with an ETA. |
| `--template-pattern` | string | _(all)_ | Only mutate templates whose name matches this regular expression. |
| `--run` | string | _(all)_ | Only run tests matching this regular expression. Passed to `go test -run`. |
| `--include-test-callers` | bool | `false` | Also start from `ExecuteTemplate` calls in `_test.go` files. |
| `--use-templates-variable` | string[] | `templates` | Global `*template.Template` variable name(s) to read templates from. |
| `--format` | string | `text` | `text` or `json`. |

Package patterns may be passed as arguments. The default is `./...`, so a mutation caught only by a test in another package is still reported as caught.

```bash
muxt test-template-mutations --template-pattern '^GET /users' --run TestUsers ./web/...
```

## Knowing How Long It Will Take

The preamble reports how many mutants there are, across how many templates, and their total [cyclomatic complexity](https://en.wikipedia.org/wiki/Cyclomatic_complexity) — one, plus one per branch point. Complexity and mutant count move together, because every branch is somewhere a mutation applies.

After the unmutated run, the command prints its duration and an estimate for the whole run to stderr:

```
20 mutants across 6 templates (complexity 10), baseline 1.9s, estimated 38s
```

That is the number to set a CI timeout from, and the signal for whether to wait or walk away. With `-v` the estimate is reprinted after each mutant and converges on what the suite actually costs:

```
[ 1/20] KILL template.gohtml:14:2 "/ Count()" template-drop (1.9s, ~36.1s left)
[ 2/20] MISS template.gohtml:17:2 "/ Count()" template-drop (1.8s, ~34.3s left)
```

`--dry-run` does the whole enumeration — including type checking every mutant — and runs nothing, which takes about as long as `muxt check`.

## Operators

One mutant is produced per applicable action. No operator renames a template or changes which templates exist, so a mutant never moves a route.

| Operator | Applies to | Variation |
|----------|-----------|-----------|
| `action-zero` | `{{.Field}}` whose type resolved | Substitutes that type's zero value: `""`, `0`, `false`. |
| `action-empty` | `{{.Field}}`, `{{printf ...}}` | Prints nothing, where the type could not be resolved from dot. |
| `if-true` | `{{if}}` | Takes the then branch unconditionally. |
| `if-false` | `{{if}}` | Takes the else branch, or none, unconditionally. |
| `with-empty` | `{{with}}` | Behaves as though the value were absent, leaving the else branch. |
| `range-never` | `{{range}}` | Iterates zero times, leaving the else branch. |
| `template-drop` | `{{template}}` | Removes the call. |

A pipeline that declares variables keeps its declarations and has only its value replaced, so `{{range $i, $item := .Items}}` is mutated without leaving `$item` undefined.

`{{block}}` is not dropped: a block defines its body in place, so removing its call would orphan the body's `{{end}}`.

`with-empty` and `range-never` replace the whole construct rather than the pipeline. Substituting `false` into a `{{with}}` would rebind dot to a boolean and stop every field access in the body from type checking, and there is no literal for an empty sequence.

## Skipped Mutants

A mutation that stops the template parsing or type checking is reported as `SKIP` and never run:

```
SKIP 1:18 action-empty (does not type check against server.Page)
```

Running it would fail the tests with a render error, and that failure would be recorded as the mutation being caught — a kill it did not earn. Knowing the type of dot is what makes this decidable before anything runs.

## Baseline

The tests run once unmutated before anything is varied. If that baseline fails, nothing is mutated and the command exits non-zero:

```
Error: baseline tests failed before mutation; fix them first:
--- FAIL: TestIndex (0.00s)
```

Every mutant would otherwise be recorded as caught — by the failure that was already there.

## How Variations Are Delivered

Mutants reach the test run through the go command's `-overlay` flag, which replaces a file for the build without writing to your working tree. The overlay reaches `//go:embed` content, so an embedded template is varied without touching the checkout.

Templates written as Go string literals are mutated too: the variation is spliced into the literal's text and the literal is re-encoded, so the overlay replaces the `.go` file. Positions are reported in that file.

> `golang.org/x/tools/go/packages` has an `Overlay` field of its own, but it does not reach `//go:embed` content — `go list` reports the original path for an embedded file, so anything reading that path still sees the unmutated bytes.

## Interpreting a Report

A miss is not automatically a bug. It means one of:

- **A missing assertion.** The common case: the test renders the template but never checks the part that action controls.
- **An untested branch.** `if-false` surviving usually means no test renders with that condition false.
- **An equivalent mutant.** The variation genuinely cannot change observable output.

Work down the misses; each one closed is an assertion the suite did not have.

## JSON Output

`--format json` carries the same report, plus the mutated action's source, its replacement, and per-mutant durations:

```json
{
	"dry_run": true,
	"templates": 1,
	"complexity": 2,
	"total": 3,
	"killed": 0,
	"missed": 0,
	"skipped": 0,
	"groups": [
		{
			"call_site": "template.go:20:9",
			"entry": "greeting",
			"data_type": "server.Greeting",
			"templates": [
				{
					"template": "greeting",
					"file": "templates.gohtml",
					"data_type": "server.Greeting",
					"via_template_call": false,
					"complexity": 2,
					"results": [
						{
							"status": "PEND",
							"operator": "if-false",
							"line": 1,
							"column": 39,
							"original": "{{if .Loud}}",
							"mutated": "false"
						}
					]
				}
			]
		}
	],
	"trimmed": []
}
```

Statuses are `KILL`, `MISS`, `SKIP`, and `PEND` for a mutant that was enumerated but not run.

## Limitations

- Custom delimiters are not supported; templates are scanned with `{{` and `}}`.
- A pipeline that calls a function or reads a variable has no resolved type, so it gets `action-empty` rather than a type-directed replacement.
- Each mutant is a full `go test -count=1` run. Narrow it with `--template-pattern`, `--run`, and a package argument, and use `--dry-run` first to see the size of the job.

## Related

- [muxt check](check.md) — Type-check templates without running them
- [muxt list-template-callers](list-template-callers.md) — List callers of a template
