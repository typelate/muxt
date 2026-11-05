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

The package directory defaults to the working directory and is resolved like `-C`; `go test ./...` runs from it ([reference_package_directory_argument.txt](../../../cmd/muxt/testdata/reference_package_directory_argument.txt)). Everything after `--` is passed to `go test` as written, except these, which muxt refuses before anything runs:

- `-overlay`: muxt delivers each mutant through its own overlay.
- `-json`: muxt reads `go test`'s text output to tell a failed test from a failed build, and the JSON stream has none of those lines.
- `-mod` and `-modfile`: the [coverage pre-pass](#how-the-pre-pass-works) sets both.
- A `-coverpkg` that would cover `html/template`; the [coverage pre-pass limitations](#template-coverage) explain why.

To bound each mutant's run, pass `go test`'s own `-timeout`, for example `-- -timeout=2m`.

```bash
muxt test-template-mutations --template-pattern '^GET /users' --run TestUsers ./web -- -tags=integration -race
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--dry-run` | `false` | Enumerate and report the mutants without running tests. |
| `--include-uncovered-nodes` | `false` | Run the mutants in template nodes no test reaches instead of skipping them. See [coverage pre-pass](#coverage-pre-pass). |
| `--verbose`, `-v` | `false` | Report every mutant, not only misses, and stream progress: a line per template and a line per mutant. |
| `--template-pattern` | all | Mutate only templates whose name matches this regular expression. |
| `--run` | all | Passed to `go test -run`. |
| `--include-test-callers` | `false` | Also start from `ExecuteTemplate` calls in `_test.go` files. |
| `--seed` | random; printed in the preamble | Seed for the values substituted by `operands`. |
| `--max-cases` | `8` | Most operand combinations one action may contribute. |
| `--workers` | `1` | Mutants to run at once, each in its own `go test`; the tests must tolerate running beside themselves. Above 1, every kill is [repeated alone](#flaky-kills). |
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
| `SKIP` | Not run: the mutant does not parse or type check (else its render error would count as a kill), is `condition-dead`, exceeds `--max-cases`, or is in a node no test reaches. See [coverage pre-pass](#coverage-pre-pass). [reference_test_template_mutations_skipped.txt](../../../cmd/muxt/testdata/reference_test_template_mutations_skipped.txt) |
| `FLAKY` | A test failed, then passed when run again alone (`--workers` above 1 only). Neither a kill nor a miss; the reason names the tests. See [flaky kills](#flaky-kills). |
| `PEND` | Enumerated but not run (`--dry-run`). |

### Flaky kills

Mutants run with `--workers` above 1 share whatever the tests share: a port, a file, the CPU. A test can fail from that contention alone, which would count as a kill. So once all mutants have run, each `KILL` is repeated, one at a time with nothing else running, against the same mutant and running only the tests that failed (all the tests when a package failed without naming one, and exactly the tests `--run` or a `-run` after `--` selected when you gave one). If they fail again it stays a `KILL`. If they pass it becomes `FLAKY`, listed without `-v` like a `MISS`, and counted on the summary line, which gains `N flaky` only when there is one:

```text
    FLAKY 1:39 if-true (TestGreeting failed beside other mutants, passed when run alone)

3 mutants, 1 killed, 1 flaky, 1 missed, 0 skipped
```

A flaky mutant is unknown, not caught, so close the gap by making the suite tolerate `--workers`, or run with `--workers 1`. It does not fail the exit status. With one worker nothing is repeated.

The repeat is serial on purpose, since contention is what it checks for, and it costs up to one more run of the suite per kill. On a suite that kills most mutants it takes back much of what `--workers` saved. On a terminal, or with `-v`, each repeat writes a line: `[recheck 3/12] templates.gohtml "page" 1:39 if-true: killed`. The template lines written while the mutants run count kills as `(unconfirmed)` for the same reason. A repeat that cannot run keeps the kill and says why in its reason. `--format json` adds a top-level `flaky` count when it is not zero.

## Coverage pre-pass

muxt skips the mutants that no test reaches, because no test could kill them. A mutant is unreached when no test executes the template action it is in. muxt reports it as a `SKIP` with the reason `no test reaches it` and counts it among the skipped mutants, so it does not fail the run. The summary line gains `N unreached` when there is at least one. With `-v`, muxt also lists the unreached mutants after the templates:

```text
    SKIP 1:49 action-zero (no test reaches it)

no test reaches 1 mutant:
  "page" templates.gohtml 1:49 action-zero

4 mutants, 2 killed, 1 missed, 1 skipped, 1 unreached
```

Each listed mutant needs a test that renders its branch. A stronger assertion in the existing tests cannot catch it. [reference_test_template_mutations_unreached.txt](../../../cmd/muxt/testdata/reference_test_template_mutations_unreached.txt)

`--include-uncovered-nodes` runs the unreached mutants instead of skipping them. Each one is almost certainly a `MISS`, and a miss fails the run. muxt still lists them under `no test reaches`, so you can tell which misses are untested branches, and the summary marks them `(run)`:

```text
4 mutants, 2 killed, 2 missed, 0 skipped, 1 unreached (run)
```

[reference_test_template_mutations_include_uncovered.txt](../../../cmd/muxt/testdata/reference_test_template_mutations_include_uncovered.txt)

### How the pre-pass works

The pre-pass runs after the baseline passes, and only when there is a mutant to run, so a failing baseline stops the run before the engine is tried. It runs your tests one extra time, unmutated, with muxt's own template engine in place of `html/template`. The engine records each template block the tests execute. The kill runs that follow use the real `html/template`, so the engine never decides whether a mutant is killed.

The command is `go test -count=1 -v -mod=mod -modfile=<scratch>/go.mod -overlay=<scratch>/overlay.json` with your `go test` flags. Before it runs, muxt writes these under its scratch directory for the run:

- `html-template/muxt_shim.go`: a shim of `html/template` that aliases each of its types to, and forwards each of its functions and constants to, [`github.com/typelate/muxt/template/html`](../template-engine.md). `FuncMap` stays `text/template`'s. The overlay deletes the real files of GOROOT's `html/template` and puts the shim in their place.
- `go.mod` and `go.sum`: copies of yours, with `github.com/typelate/muxt` required at the version of the muxt you ran. When your `go.mod` already requires muxt, as a [`tool` dependency](../../tutorials/quick-start.md) does, the copy keeps your version when it is the same and uses the CLI's when it is not. In muxt's own repository the copy requires nothing, since the engine is already there.

Your `go.mod` and `go.sum` are never written. `-mod=mod` lets the go command add the sums muxt's requirements need to the copy. The go command takes muxt from the module cache, where `go install` or `go get` left it, and fetches it from the module proxy otherwise. With `-v`, the [timing](#timing) preamble names the module the engine came from, as in `coverage pre-pass 2.4s (engine github.com/typelate/muxt@v0.23.0)`, and adds `, not your go.mod's v0.22.0` when the copy overrode your version. muxt removes the scratch directory when the run ends.

A development build of muxt has no published version. A build of a commit with no uncommitted changes requires the pseudo-version of that commit, which the go command can fetch once the commit is pushed. Any other development build stops the pre-pass with `muxt is a modified development build; set MUXT_MODULE_DIR to its source directory`, and coverage is unknown. Set `MUXT_MODULE_DIR` to the directory of muxt's source, and the copy replaces `github.com/typelate/muxt` with it. muxt's own tests do this with the repository, so they fetch nothing.

Each test binary that runs the engine writes its profile, `<pid>.cov`, as it starts, before it renders anything. A profile with no blocks is a test binary that executed no template, and every mutant it would have reached is unreached. No profile at all means no test binary ran the engine, and coverage is unknown.

No package is added under GOROOT, so vet runs in the pre-pass as in your normal test run. The `-v` flag lets a test binary that stopped recording coverage say so in its output.

The pre-pass never fails the run. When it cannot tell which actions the tests reach, it writes a warning and the run goes on without it. [Template coverage](#template-coverage) under Limitations lists when that happens, along with the other edge cases.

## Exit status

The command exits non-zero when:

- any mutant is a `MISS`. A `FLAKY` one does not fail the run. The report is written first, and stderr ends with `Error: N mutants missed`. A skipped mutant does not fail the run, even when it is unreached, and `--dry-run` does not fail either.
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

The preamble counts mutants, templates and total complexity (per template, one plus one per `if`, `range` and `with`, plus one per `and` or `or` command in their pipelines, summed). After the baseline and the [coverage pre-pass](#coverage-pre-pass), stderr reports the baseline duration, the pre-pass duration when the pre-pass ran, and an estimate for the rest of the run. The pre-pass is one more run of the suite, and the estimate leaves it out because it has already finished:

```text
20 mutants across 6 templates (complexity 10), baseline 1.9s, coverage pre-pass 2.4s, estimated 38s
```

With `-v` the pre-pass's part also names the module the engine came from: `coverage pre-pass 2.4s (engine github.com/typelate/muxt@v0.23.0)`. A development build that `MUXT_MODULE_DIR` replaces has no version to name, so the note is `engine github.com/typelate/muxt => <dir>`.

Progress goes to stderr only, so `--format json` on stdout stays machine-readable. When stderr is a terminal, or with `-v`, one line is written as each template finishes: its position among the templates that have mutants, its mutant counts and the time its mutants took. With `--workers` above 1 templates finish in whatever order their last mutant does.

```text
[ 1/6] templates.gohtml "greeting": 8 mutants, 7 killed, 1 missed, 3.2s
[ 2/6] templates.gohtml "footer": 3 mutants, 2 killed, 0 missed, 1 skipped, 1.1s
```

`missed` counts `MISS` results, as the summary line does. A skipped count appears only when there is one. Piped without `-v`, stderr carries the preamble and nothing else, so a log stays short; the line per template is what to tail to see a long run is alive.

With `-v` each mutant also prints its duration and the time left:

```text
[ 2/20] MISS template.gohtml:17:2 "/ Count()" template-drop (1.8s, ~34.3s left)
```

With `--dry-run` each template's heading says how many mutants it holds, which is where a long run's time will go:

```text
  "greeting" templates.gohtml (complexity 2, 3 mutants, dot: server.Greeting)
```

## `--diff`

```bash
muxt test-template-mutations --diff origin/main
```

The unit is a template rendered with one type of dot. It is mutated when its text changed as parsed (reformatting inside an action or moving the file is not a change) or when it is reached with a dot type it was not reached with at the revision. Types are compared by name, looking through type aliases wherever they appear (including in the type arguments of a generic dot): retargeting `type Page = page.Data` to another type counts as a new type of dot.

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

A run adds `seconds` per result and a top-level `baseline` object with `passed` and `seconds`. When the pre-pass learned the coverage, a run adds `"coverage_known": true`. When any mutant is unreached, it also adds `unreached`, which lists each unreached mutant's template, file, line, column, operator and replacement. A run with `--include-uncovered-nodes` adds `"include_uncovered_nodes": true`. A `--diff` run adds `diff`, plus `unchanged` when any template was left alone or `diff_error` when the revision could not be read. [reference_test_template_mutations_json.txt](../../../cmd/muxt/testdata/reference_test_template_mutations_json.txt)

## Limitations

- `eq`, `ne`, `lt`, `le`, `gt` and `ge` are checked for arity, not operand comparability, so a mutant that breaks a comparison runs and is recorded as killed by the render error.
- Each mutant is a full `go test -count=1` run.
- A source parsed with `Delims` that holds no `{{define}}` is read with the default delimiters.

### Template coverage

These edge cases apply to the [coverage pre-pass](#coverage-pre-pass).

- **Coverage unknown.** When the pre-pass cannot tell which actions the tests reach, stderr gets one line that starts with `warning: template coverage unknown:` and gives the reason. muxt then reports no mutant as unreached and skips none as unreached, with or without `--include-uncovered-nodes`. The run goes on as it would without the pre-pass.
- **The overlay is refused.** muxt runs a probe first to check that the go command accepts an overlay of GOROOT's `html/template` that imports `github.com/typelate/muxt/template/html`, with muxt required as in the pre-pass. When the go command refuses or ignores that overlay, coverage is unknown, and the warning quotes the go command's own reason. It suggests `GOTOOLCHAIN=local` only when that reason is about the toolchain or the module cache.
- **muxt cannot be fetched.** When the go command cannot get the `github.com/typelate/muxt` version the copy requires, the probe fails, coverage is unknown, and the warning says `the go command cannot get github.com/typelate/muxt@<version>` with the go command's reason. A muxt installed with `go install` is in the module cache already.
- **Development builds.** A muxt built from uncommitted changes, or without version control information, has no version to require. Coverage is unknown until `MUXT_MODULE_DIR` names its source directory; see [How the pre-pass works](#how-the-pre-pass-works).
- **Another muxt version in your `go.mod`.** The copy requires the CLI's version in place of yours, so the pre-pass builds the engine of the muxt you ran. `-v` says so in the preamble. When another requirement of yours needs a newer muxt, minimal version selection keeps the newer one, and the engine is that version's, not the CLI's.
- **Workspaces.** In workspace mode (a `go.work` in use), the go command refuses `-modfile`, so the probe fails, the warning says `go.work is in use`, and coverage is unknown. muxt does not set `GOWORK=off`, because your tests would then build against a different set of modules than the baseline and the kill runs. Set `GOWORK=off` yourself if your module builds without the workspace.
- **Shared module versions.** The pre-pass builds your tests against the copy's module graph, not yours. `-mod=mod` ignores `vendor/`, so your dependencies come from the module cache, downloaded when they are not there. And since the copy requires muxt, the go command selects the highest version any module requires, so a module you share with muxt, such as `golang.org/x/net`, can be built at a newer version than your `go.mod` requires. When any is, the run warns once, naming each module and both versions, and still uses the coverage:

  ```text
  warning: the coverage pre-pass selected newer versions of modules your go.mod requires: golang.org/x/net v0.30.0 → v0.62.0; tests exercising them may take other paths
  ```

  The baseline and the kill runs build with your `go.mod` and `vendor/` as written.
- **Offline or without a proxy.** The first pre-pass in a module may need modules your build never did: muxt and its requirements, such as `golang.org/x/net`. Without a module proxy (`GOPROXY=off`) or a network, and with those modules not in the module cache, `go test` cannot set up. Coverage is then unknown, and the warning quotes the go command's reason, for example `module lookup disabled by GOPROXY=off`. A proxy without a reachable checksum database fails the same way, because the sums of muxt's requirements cannot be verified; whether to set `GONOSUMDB` or `GOSUMDB=off` is your call.
- **An older `go` line.** The pre-pass compiles your module at muxt's go line when yours is lower. The go command raises the `go` line in the copy of your `go.mod`, because muxt's module needs it. Your packages then compile at that language version in the pre-pass, and the pre-pass coverage decides which mutants are skipped as unreached. The newer version can change what your code does:
  - When your `go` line is below `go 1.22`, your loops share one variable across iterations. In the pre-pass each iteration gets its own, so a closure that captures the loop variable can see a different value than in your normal run.
  - The `GODEBUG` defaults follow the `go` line.

  The run then warns once and still uses the coverage:

  ```text
  warning: template coverage was computed at go 1.27 (your go.mod says go 1.24): branches that depend on the older language semantics may be reported unreached
  ```

  All of this applies only to the pre-pass. The baseline and the kill runs build with your `go.mod` as written.
- **The tests fail under the engine.** When the tests fail, or do not build, with the engine in place, coverage is unknown and the warning names the first failure:

  ```text
  warning: template coverage unknown: tests fail under muxt's template engine (coverage pre-pass): ./render_test.go:20:25: cannot use templates.Lookup("greeting").Tree (variable of type *"github.com/typelate/muxt/template".Tree) as *parse.Tree value in variable declaration; known gaps: Template.Tree, AddParseTree, Error.Node
  ```

  The baseline still decides whether the tests pass. [reference_test_template_mutations_engine_gap.txt](../../../cmd/muxt/testdata/reference_test_template_mutations_engine_gap.txt)
- **Known engine gaps.** The shim declares every identifier of `html/template`, and three of them differ from the standard library's because the shim's types are the engine's. `Template.Tree` holds a `*template.Tree` of `github.com/typelate/muxt/template`, not a `*parse.Tree`. `AddParseTree` takes that tree, not a `*parse.Tree`. `Error.Node` holds a value with only a `String` method, not a `parse.Node`. Code that uses any of them does not build under the engine, so the pre-pass reports coverage unknown for that run.
- **No test binary ran the engine.** Coverage is known only from a profile, so when no test binary writes one, the warning is `warning: template coverage unknown: no test binary ran the templates (no coverage profile was written)` and every mutant runs. That happens when the packages have no `_test.go` files, when `-exec` runs the test binaries somewhere their profiles do not reach, and when the tests render through a server binary of their own, built without the overlay, and the test binary does not link `html/template` itself. A test binary that does link it, and renders nothing in its own process, writes an empty profile, so every mutant reads as unreached and is skipped. [reference_test_template_mutations_no_test_files.txt](../../../cmd/muxt/testdata/reference_test_template_mutations_no_test_files.txt), [reference_test_template_mutations_never_rendered.txt](../../../cmd/muxt/testdata/reference_test_template_mutations_never_rendered.txt)
- **One parse name, several packages.** Profiles are unioned by the name a template's text was parsed under, a file's base name, across every test binary, so a block reached in one package's `index.gohtml` also counts as reached in another package's `index.gohtml`. The error only goes one way: such a mutant is run instead of skipped.
- **Coverage export is disabled.** A test binary that cannot write its coverage prints `muxt: template coverage export disabled:` and the reason. The pre-pass reads that as coverage unknown, not as templates no test reached.
- **Downloaded toolchains.** When the `go` toolchain in use was downloaded for a `toolchain` line in `go.mod`, its GOROOT is in the module cache, and the go command refuses overlays there, so coverage is unknown. Set `GOTOOLCHAIN=local` to use the installed toolchain, or install the toolchain outside the module cache.
- **A `-coverpkg` that covers `html/template`.** Coverage instrumentation reads the packages it instruments from disk, which would undo the overlay. So muxt refuses a `-coverpkg` after `--` that takes in `html/template`, and it does so before anything runs. The refused values are `html/template` itself, `std`, `all`, and any `...` pattern that matches `html/template`. A `-coverpkg` of your own packages, such as `-coverpkg=./...`, works.

  ```text
  Error: go test flag -coverpkg cannot cover html/template ("std" does): muxt replaces html/template with its own template engine to learn which template actions the tests reach; use -coverpkg with your own packages, such as ./... instead
  ```

  [err_test_template_mutations_coverpkg_refused.txt](../../../cmd/muxt/testdata/err_test_template_mutations_coverpkg_refused.txt)
- **Coverage flags.** The pre-pass leaves out your `-cover`, `-covermode`, `-coverprofile` and `-coverpkg` flags, so only the baseline writes a coverage profile.
- **Dry runs.** `--dry-run` runs no tests, with or without `--diff`, so it skips the pre-pass. It lists every mutant and never reports one as unreached.
- **Cost.** No flag turns the pre-pass off. It costs one extra run of the suite, plus a one-time build of the engine and of the `html/template` shim, which later runs take from the build cache. The [timing](#timing) preamble reports how long the pre-pass took.

Related: [muxt check](check.md) type-checks templates without running them; [muxt list-template-callers](list-template-callers.md) lists the call sites traversal starts from.
