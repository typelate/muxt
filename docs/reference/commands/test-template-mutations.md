# muxt test-template-mutations

Varies each dynamic and control-flow action in your templates, one at a time, and re-runs the tests against each variation.

```bash
muxt test-template-mutations -v --seed 1
```

```text
3 mutants across 1 template (complexity 2, seed 1)
baseline ok

template.go:20:9 ExecuteTemplate "greeting" (dot: server.Greeting)
  "greeting" templates.gohtml (complexity 2, dot: server.Greeting)
    KILL 1:29 action-zero
    MISS 1:39 if-false
    KILL 1:39 if-true

3 mutants, 2 killed, 1 missed, 0 skipped
```

The [tutorial](../../tutorials/find-untested-template-behavior.md) turns a miss into an assertion.

## Usage

```text
muxt test-template-mutations [package-dir] [-- go test flags] [flags]
```

The package directory defaults to the working directory and is resolved like `-C`; `go test ./...` runs from it ([reference_package_directory_argument.txt](../../../cmd/muxt/testdata/reference_package_directory_argument.txt)). Everything after `--` is passed to `go test` as written, except `-overlay`. To bound each mutant's run, pass `go test`'s own `-timeout`, for example `-- -timeout=2m`.

```bash
muxt test-template-mutations --template-pattern '^GET /users' --run TestUsers ./web -- -tags=integration -race
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--dry-run` | `false` | Enumerate and report the mutants without running tests. |
| `--verbose`, `-v` | `false` | Report every mutant, not only misses, and stream progress. |
| `--template-pattern` | all | Mutate only templates whose name matches this regular expression. |
| `--run` | all | Passed to `go test -run`. |
| `--include-test-callers` | `false` | Also start from `ExecuteTemplate` calls in `_test.go` files. |
| `--seed` | random; printed in the preamble | Seed for the values substituted by `operands`. |
| `--max-cases` | `8` | Most operand combinations one action may contribute. |
| `--workers` | `1` | Mutants to run at once, each in its own `go test`; the tests must tolerate running beside themselves. |
| `--diff` | none | Mutate only templates that changed since this git revision. |

Common flags (`--format`, `--use-templates-variable`, `-C`): [cli.md](../cli.md#flags).

## What is mutated

Traversal starts at each `ExecuteTemplate` call that meets the [type-checking preconditions](../type-checking.md#preconditions) and walks `{{template}}` calls depth first, carrying dot along.

- A template nothing renders is not mutated.
- A template is mutated once per type of dot. A second subtree reached with the same dot is trimmed; `-v` lists what was trimmed. [reference_test_template_mutations_trimmed.txt](../../../cmd/muxt/testdata/reference_test_template_mutations_trimmed.txt)
- Calls in `_test.go` files are ignored unless `--include-test-callers` is set. [reference_test_template_mutations_test_callers.txt](../../../cmd/muxt/testdata/reference_test_template_mutations_test_callers.txt)
- Templates written as Go string literals are mutated too. Positions are reported in the `.go` file. [reference_test_template_mutations_string_literal.txt](../../../cmd/muxt/testdata/reference_test_template_mutations_string_literal.txt)
- Templates parsed with custom `Delims` are mutated like any other. [reference_test_template_mutations_custom_delimiters.txt](../../../cmd/muxt/testdata/reference_test_template_mutations_custom_delimiters.txt)

Mutants reach the build through `go test -overlay`, which also reaches `//go:embed` content, so the working tree is never written to.

## Operators

One mutant per applicable action. No operator renames a template, so a mutant never moves a route.

| Operator | Applies to | Variation |
|----------|-----------|-----------|
| `action-zero` | An action whose type resolved to a string, bool, integer or float | Substitutes the type's zero value: `""`, `0`, `false`. |
| `action-empty` | Any other action | Prints nothing. |
| `if-true` | `{{if}}` | Takes the then branch. |
| `if-false` | `{{if}}` | Takes the else branch, or nothing. |
| `with-empty` | `{{with}}` | Treats the value as absent. |
| `range-never` | `{{range}}` | Iterates zero times. |
| `template-drop` | `{{template}}` | Removes the call. `{{block}}` is never dropped. |
| `operands` | An action reading two or more inputs | Substitutes drawn values into one combination of its leaf operands. |
| `condition` | One condition of an `and`/`or`/`not` decision | Forces that condition true or false while the others read real data. |
| `condition-dead` | A condition the simplifier removed | Reported as `SKIP`, never run. |

A pipeline that declares variables keeps the declarations; only the value is replaced.

### Type resolution

A pipeline is typed from its last command. Field paths off dot resolve through struct fields, no-argument methods and pointers. Calls resolve from the registered function's signature; `len` is `int`, comparisons and `not` are `bool`, the print and escape families are `string`. `and`, `or`, `index`, `slice`, `call` and variables are left unresolved. The `html/template` safe string types (`HTML`, `JS`, `URL`, ...) resolve but have no literal. All of these get `action-empty`. [reference_test_template_mutations_function_types.txt](../../../cmd/muxt/testdata/reference_test_template_mutations_function_types.txt)

### Operands

`operands` reports which input a test depends on:

```text
MISS 12:5 operands .First="nard"
KILL 12:5 operands .First="nard" .Last="xeqr"
KILL 12:5 operands .Last="xeqr"
```

Leaf operands are the field accesses, variables and dots a pipeline reads. Function names and literals are not operands. [reference_test_template_mutations_operands.txt](../../../cmd/muxt/testdata/reference_test_template_mutations_operands.txt)

An action needing more than `--max-cases` combinations is skipped with the arithmetic:

```text
SKIP 1:22 operands (2 operands need 3 cases, over --max-cases=2)
```

### Conditions

A decision written with `and`, `or` and `not` is simplified first (flattening, constant folding, idempotence, complement, absorption), then gets one mutant per condition. A condition that cannot change the outcome is reported dead:

```text
SKIP 1:50 condition-dead (.Loud cannot change the decision)
```

Comparisons, calls and multi-command pipelines fall back to `operands`. [reference_test_template_mutations_conditions.txt](../../../cmd/muxt/testdata/reference_test_template_mutations_conditions.txt)

## Statuses

| Status | Meaning |
|--------|---------|
| `KILL` | A test failed. |
| `MISS` | Every test passed: a missing assertion, an untested branch, or an equivalent mutant whose output cannot differ. |
| `SKIP` | Not run: the mutant does not parse or type check (else its render error would count as a kill), is `condition-dead`, or exceeds `--max-cases`. [reference_test_template_mutations_skipped.txt](../../../cmd/muxt/testdata/reference_test_template_mutations_skipped.txt) |
| `PEND` | Enumerated but not run (`--dry-run`). |

## Exit status

The command exits non-zero when:

- any mutant is a `MISS`. The report is written first, and stderr ends with `Error: N mutants missed`. Skipped mutants and `--dry-run` do not fail.
  [reference_test_template_mutations_go_test_flags.txt](../../../cmd/muxt/testdata/reference_test_template_mutations_go_test_flags.txt)
- the baseline (unmutated) test run fails. Nothing is mutated.
  [err_test_template_mutations_baseline_fails.txt](../../../cmd/muxt/testdata/err_test_template_mutations_baseline_fails.txt)
- `go test` cannot run, or a mutant's run fails without a test failing (a package that does not build, an error from the `go` command). Its output is printed.
- the templates reached hold no dynamic or control-flow action, unless `--template-pattern` or `--diff` narrowed them; then the report says there is nothing to mutate.
  [err_test_template_mutations_no_actions.txt](../../../cmd/muxt/testdata/err_test_template_mutations_no_actions.txt)
- `--template-pattern` matches no template reached.
- no `ExecuteTemplate` call is found; the type of dot comes from the call site.
- `--diff` names a revision git does not know.

## Timing

The preamble counts mutants, templates and total complexity (per template, one plus one per `if`, `range` and `with`, plus one per `and` or `or` command in their pipelines, summed). After the baseline run, stderr reports the baseline duration and an estimate for the whole run:

```text
20 mutants across 6 templates (complexity 10), baseline 1.9s, estimated 38s
```

With `-v` each mutant prints its duration and the time left:

```text
[ 2/20] MISS template.gohtml:17:2 "/ Count()" template-drop (1.8s, ~34.3s left)
```

## `--diff`

```bash
muxt test-template-mutations --diff origin/main
```

The unit is a template rendered with one type of dot. It is mutated when its text changed as parsed (reformatting inside an action or moving the file is not a change) or when it is reached with a dot type it was not reached with at the revision. Types are compared by name.

```text
3 mutants across 2 templates (complexity 2, seed 1)
1 template unchanged since origin/main
```

The revision is read with `git archive`. If the templates cannot be read there, every template counts as changed and the preamble says why. [reference_test_template_mutations_diff.txt](../../../cmd/muxt/testdata/reference_test_template_mutations_diff.txt)

## JSON

`--format json` carries the text report plus each mutant's source, replacement and duration:

```json
{
	"dry_run": true,
	"seed": 1,
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

A run adds `seconds` per result and a top-level `baseline` object with `passed` and `seconds`. A `--diff` run adds `diff`, plus `unchanged` when any template was left alone or `diff_error` when the revision could not be read. [reference_test_template_mutations_json.txt](../../../cmd/muxt/testdata/reference_test_template_mutations_json.txt)

## Limitations

- `eq`, `ne`, `lt`, `le`, `gt` and `ge` are checked for arity, not operand comparability, so a mutant that breaks a comparison runs and is recorded as killed by the render error.
- Each mutant is a full `go test -count=1` run.
- A source parsed with `Delims` that holds no `{{define}}` is read with the default delimiters.

Related: [muxt check](check.md) type-checks templates without running them; [muxt list-template-callers](list-template-callers.md) lists the call sites traversal starts from.
