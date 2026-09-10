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
| `--seed` | uint64 | _(drawn)_ | Seed the values substituted for an action's operands. Drawn and reported when not given. |
| `--max-cases` | int | `8` | Most operand combinations one action may contribute. |
| `--state` | string | `testdata/template-mutations.json` | Record verdicts here and reuse them for unchanged actions. Empty disables. |
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
| `action-zero` | An action whose type resolved | Substitutes that type's zero value: `""`, `0`, `false`. |
| `action-empty` | An action whose type did not | Prints nothing. |
| `if-true` | `{{if}}` | Takes the then branch unconditionally. |
| `if-false` | `{{if}}` | Takes the else branch, or none, unconditionally. |
| `with-empty` | `{{with}}` | Behaves as though the value were absent, leaving the else branch. |
| `range-never` | `{{range}}` | Iterates zero times, leaving the else branch. |
| `template-drop` | `{{template}}` | Removes the call. |
| `operands` | An action reading two or more inputs | Substitutes drawn values into one combination of its leaf operands. |
| `condition` | A condition of an `and`/`or`/`not` decision | Forces that one condition true or false, leaving the others reading real data. |
| `condition-dead` | A condition simplification removed | Reported, never run: it cannot change the decision. |

A pipeline that declares variables keeps its declarations and has only its value replaced, so `{{range $i, $item := .Items}}` is mutated without leaving `$item` undefined.

`{{block}}` is not dropped: a block defines its body in place, so removing its call would orphan the body's `{{end}}`.

`with-empty` and `range-never` replace the whole construct rather than the pipeline. Substituting `false` into a `{{with}}` would rebind dot to a boolean and stop every field access in the body from type checking, and there is no literal for an empty sequence.

## Is Each Input Coupled to a Failure?

Emptying a whole action says only that *something* about it is watched. An action reading more than one input also gets a mutant per combination of its leaf operands, which says *which* of them nothing is watching:

```
MISS 12:5 operands .First="nard"
KILL 12:5 operands .First="nard" .Last="xeqr"
KILL 12:5 operands .Last="xeqr"
```

Here nothing depends on `.First` on its own. The values are drawn per type from a seeded generator; the seed is reported, and drawn when `--seed` is not given, so any run can be repeated exactly.

Leaf operands are the field accesses, variables and dots a pipeline reads, including inside a nested pipeline. A function name is not one and neither is a literal — neither carries data into the template.

### Boolean Decisions

Forcing every condition of a decision at once only ever produces the then branch or the else branch, which `if-true` and `if-false` already cover. So a decision written with `and`, `or` and `not` gets one mutant per condition, forcing **that one** true or false while the others keep reading real data. The decision is then still a function of the data, so whether a test notices depends on what it renders with — which is what says whether anything is coupled to that condition. It is also linear in the number of conditions rather than exponential.

```
MISS 1:18 condition .Admin=false
KILL 1:18 condition .Admin=true
MISS 1:18 condition .Owner=false
MISS 1:18 condition .Owner=true
```

`.Owner` survives both: with `{{if and .Admin .Owner}}` and a test that never renders with `.Admin` true, nothing can observe `.Owner` at all.

The decision is simplified first — flattening, constant folding, idempotence, complement and absorption. A condition simplification removes is reported `condition-dead` and never run, because nothing could have been coupled to something that cannot change the outcome:

```
SKIP 1:50 condition-dead (.Loud cannot change the decision)
```

`{{or .Banned (and .Banned .Loud)}}` is just `.Banned`.

Anything the model does not cover exactly — a comparison, a call, a multi-command pipeline — falls back to the general operand combinations.

### Cost

Every case is a full test run, so `--max-cases` bounds what one action may contribute. An action over the bound contributes nothing and says so, rather than quietly turning a two-minute run into an hour:

```
SKIP 18:3 operands (7 operands need 127 cases, over --max-cases=8)
```

## Only Run What Changed

A run records its verdicts in `testdata/template-mutations.json` and reuses them next time for actions that have not changed:

```
2 mutants, 2 killed, 0 missed, 0 skipped, 1 reused
```

An action is identified by a hash of its template's source, the action itself, the fully resolved type of dot and of every operand, and the seed. The source alone would not be enough: a field changing from a `string` to an `int` changes what a mutation substitutes without changing a byte of the template, and the dot type's name stays the same either way.

The file belongs to the tests, which is why it sits in `testdata` — it is the record of which template behaviour the suite was shown to cover, and it should be reviewed and committed alongside the tests that produced it. Pass `--state ""` to turn it off, and a different `--seed` retries everything.

A dry run neither reads nor writes it.

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

## Nothing To Mutate

If every template reached holds only static text, the command exits non-zero rather than reporting a run of zero mutants:

```
Error: no mutations available: the 3 template(s) reached hold no dynamic or control flow actions, so a run would report every mutant killed without testing anything
```

A report of `0 mutants, 0 missed` reads as the tests catching everything. It is an error so that a project whose templates were never read cannot be mistaken for one whose tests are thorough.

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

## How an Action's Type Is Resolved

A pipeline's value is whatever its **last** command produces, so `{{.Name | printf "%s"}}` is typed from `printf`, not from `.Name`.

- **A field path off dot** — `{{.User.Email}}` — is resolved structurally, through struct fields and no-argument methods, following pointers.
- **A call** is resolved from the function's signature. The template set carries one for every function it registered, so a project's own `{{upper .Name}}` is typed from `upper`. The builtins the checker verifies by shape rather than by signature are answered from their fixed result types: `len` is an `int`, the comparisons and `not` are `bool`, the print and escape families are `string`.
- **`and`, `or`, `index`, `slice` and `call` are deliberately left unresolved.** What they produce depends on their arguments, and guessing would put a wrong replacement in a mutant.
- **A pipeline reading a variable** — `{{$item}}` — is left unresolved.

Unresolved means `action-empty` rather than `action-zero`; the mutation still happens, it just substitutes an empty string instead of a type-directed value.

`html/template`'s safe string types — `HTML`, `HTMLAttr`, `CSS`, `JS`, `JSStr`, `Srcset`, `URL` — are named types over `string`, and get `action-empty` too. A template has no way to write a literal of one: `""` in template source is an ordinary string the escaper treats differently, so calling it that type's zero value would claim more than the substitution delivers.

## Limitations

- A template set built with `Delims` is mutated like any other. The delimiters are not exposed by `text/template`, so they are read back from the `{{end}}` clause of a definition, whose span runs from one delimiter through the other. They are resolved per parsed source, not per file, so a construction chain that calls `Delims` more than once — or one Go file holding several literals parsed differently — reads each source with its own pair. A source whose only template has no define clause has no such clause to read and falls back to `{{` and `}}`; if that leaves a template the set can see actions in and this command cannot, the run fails rather than measuring fewer templates than it was given.
- `eq`, `ne`, `lt`, `le`, `gt` and `ge` are checked for arity but not for whether their operands are comparable, so a mutant that breaks a comparison type-checks, runs, and is recorded as caught by the render error it causes.
- Each mutant is a full `go test -count=1` run. Narrow it with `--template-pattern`, `--run`, and a package argument, and use `--dry-run` first to see the size of the job.

## Related

- [Find Untested Template Behavior](../../tutorials/find-untested-template-behavior.md) — a walkthrough: read a miss, write the assertion it asks for, watch it turn green
- [HTML is the API](../../explanation/html-is-the-api.md) — why the rendered markup is the contract these mutations probe
- [muxt check](check.md) — Type-check templates without running them
- [muxt list-template-callers](list-template-callers.md) — List callers of a template
