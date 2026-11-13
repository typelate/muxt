package mutation

import (
	"errors"
	"go/token"
	"go/types"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/typelate/muxt/template"
	"github.com/typelate/muxt/template/html"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/asteval"
	"github.com/typelate/muxt/internal/load/loadtest"
	"github.com/typelate/muxt/internal/source"
)

// newSelector is a selector with nothing to compare with and no template
// pattern: every scope counts as changed.
func newSelector(before revision) selector {
	return selector{
		before:    before,
		seen:      make(map[string]struct{}),
		unchanged: make(map[string]struct{}),
		include:   func(string) bool { return true },
	}
}

// trimOf is a repeat of a template already reached with the same dot,
// reported from one call site having first been reached from another.
func trimOf(template string, dot types.Type, at, first string) trim {
	position := func(file string) callSite {
		return callSite{Position: token.Position{Filename: file, Line: 1, Column: 1}}
	}
	return trim{
		call:     position(at),
		template: template,
		dataType: dot,
		firstFor: position(first),
	}
}

func names(chosen []scope) []string {
	var out []string
	for _, sc := range chosen {
		out = append(out, sc.template)
	}
	return out
}

// TestSelectorSkipsWhatARevisionAlreadyHeld states the rule a --diff run
// selects by: a scope is mutated when its template reads differently than
// it did at the revision, or when it is reached with a type of dot the
// revision did not reach it with.
func TestSelectorSkipsWhatARevisionAlreadyHeld(t *testing.T) {
	str, count := types.Typ[types.String], types.Typ[types.Int]
	before := scopesOf([]scope{scopeOf(t, "page", `<b>{{.}}</b>`, str)})

	for _, tt := range []struct {
		name          string
		scope         scope
		wantMutated   bool
		wantUnchanged string
	}{
		{
			name:          "the same text and dot",
			scope:         scopeOf(t, "page", `<b>{{.}}</b>`, str),
			wantUnchanged: "page string",
		},
		{
			name:        "text that reads differently",
			scope:       scopeOf(t, "page", `<i>{{.}}</i>`, str),
			wantMutated: true,
		},
		{
			name:        "a type of dot it was not reached with",
			scope:       scopeOf(t, "page", `<b>{{.}}</b>`, count),
			wantMutated: true,
		},
		{
			name:        "a template the revision did not hold",
			scope:       scopeOf(t, "footer", `<p>{{.}}</p>`, str),
			wantMutated: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			chosen := newSelector(before).choose([]scope{tt.scope}, nil)

			var wantMutate []string
			if tt.wantMutated {
				wantMutate = []string{tt.scope.template}
			}
			assert.Equal(t, wantMutate, names(chosen.mutate), "mutate")

			var wantUnchanged []string
			if tt.wantUnchanged != "" {
				wantUnchanged = []string{tt.wantUnchanged}
			}
			var got []string
			for _, u := range chosen.unchanged {
				got = append(got, u.Template+" "+u.DataType)
			}
			assert.Equal(t, wantUnchanged, got, "unchanged")
		})
	}
}

// TestSelectorMutatesATemplateOnceAcrossAWholeRun states that a template
// reached from two templates variables is mutated once: the second
// traversal to reach it says nothing about it at all.
func TestSelectorMutatesATemplateOnceAcrossAWholeRun(t *testing.T) {
	str := types.Typ[types.String]
	page := scopeOf(t, "page", `<b>{{.}}</b>`, str)
	sel := newSelector(nil)

	require.Equal(t, []string{"page"}, names(sel.choose([]scope{page}, nil).mutate), "the first traversal chose the page")
	second := sel.choose([]scope{page}, nil)
	assert.Empty(t, second.mutate, "the second traversal chose nothing")
	assert.Empty(t, second.unchanged, "the second traversal left nothing")
}

// TestSelectorHonoursTheTemplatePattern states that --template-pattern
// decides before anything else: a template it does not match is neither
// mutated nor reported as unchanged.
func TestSelectorHonoursTheTemplatePattern(t *testing.T) {
	str := types.Typ[types.String]
	sel := newSelector(nil)
	sel.include = func(name string) bool { return name == "page" }

	chosen := sel.choose([]scope{
		scopeOf(t, "page", `<b>{{.}}</b>`, str),
		scopeOf(t, "footer", `<p>{{.}}</p>`, str),
	}, nil)
	assert.Equal(t, []string{"page"}, names(chosen.mutate), "mutate only the page")
	assert.Empty(t, chosen.unchanged, "the pattern is not a comparison")
}

// TestSelectorReportsTrims states what a trim says and when it is worth
// saying: once per template and dot however many times it repeats, and not
// at all for a template this run mutated nowhere.
func TestSelectorReportsTrims(t *testing.T) {
	str := types.Typ[types.String]
	page := scopeOf(t, "page", `<b>{{.}}</b>`, str)

	t.Run("a repeat is reported once", func(t *testing.T) {
		chosen := newSelector(nil).choose([]scope{page}, []trim{
			trimOf("row", str, "page.go", "page.go"),
			trimOf("row", str, "page.go", "page.go"),
		})
		require.Len(t, chosen.trimmed, 1)
		want := TrimmedTemplate{CallSite: "page.go:1:1", Template: "row", DataType: "string", FirstSeenAt: "page.go:1:1"}
		assert.Equal(t, want, chosen.trimmed[0])
	})

	t.Run("a repeat of a template left alone is not reported", func(t *testing.T) {
		before := scopesOf([]scope{page})
		chosen := newSelector(before).choose([]scope{page}, []trim{trimOf("page", str, "page.go", "page.go")})
		assert.Empty(t, chosen.trimmed, "the page was mutated nowhere")
	})

	t.Run("a repeat of a template the pattern excludes is not reported", func(t *testing.T) {
		sel := newSelector(nil)
		sel.include = func(name string) bool { return name == "page" }
		chosen := sel.choose([]scope{page}, []trim{trimOf("row", str, "page.go", "page.go")})
		assert.Empty(t, chosen.trimmed, "the row is mutated nowhere")
	})
}

// TestSelectorCountsWhatThePatternMatched states that a selection says how
// many of the templates reached the pattern admitted, which is how a
// pattern matching none of them is told from templates with nothing in
// them to vary.
func TestSelectorCountsWhatThePatternMatched(t *testing.T) {
	str := types.Typ[types.String]
	sel := newSelector(nil)
	sel.include = func(name string) bool { return name == "page" }
	chosen := sel.choose([]scope{
		scopeOf(t, "page", `<b>{{.}}</b>`, str),
		scopeOf(t, "footer", `<p>{{.}}</p>`, str),
	}, nil)
	assert.Equal(t, 2, chosen.reached, "reached")
	assert.Equal(t, 1, chosen.matched, "matched")
}

// TestPlanReportCounts states how a plan's mutants are counted: an action
// too wide to enumerate counts once, as skipped, alongside the mutants
// skipped for not type checking.
func TestPlanReportCounts(t *testing.T) {
	p := &plan{mutants: make([]mutant, 5), runnableN: 3, overBudget: 2}
	report := p.report()
	assert.Equal(t, 7, report.Total, "total")
	assert.Equal(t, 4, report.Skipped, "skipped")
}

// TestIndexTreesKeepsTheTreeTheSetKept states which of several sources
// holding one name the index reads: the one the name's definition is in,
// which is the body text/template kept, and otherwise the last that is
// not empty, which is the one text/template would keep.
func TestIndexTreesKeepsTheTreeTheSetKept(t *testing.T) {
	sourceOf := func(name, text string) *templateSource {
		return newFileSource(name, name, text, "", "")
	}
	for _, tt := range []struct {
		name    string
		sources []*templateSource
		defined string
		want    string
	}{
		{
			name:    "the definition is in the later source",
			sources: []*templateSource{sourceOf("a.gohtml", "{{.A}}"), sourceOf("b.gohtml", "{{.B}}")},
			defined: "b.gohtml",
			want:    "b.gohtml",
		},
		{
			name:    "the definition is in the earlier source",
			sources: []*templateSource{sourceOf("a.gohtml", "{{.A}}"), sourceOf("b.gohtml", "{{.B}}")},
			defined: "a.gohtml",
			want:    "a.gohtml",
		},
		{
			name:    "no definition: a later non-empty tree replaces an earlier one",
			sources: []*templateSource{sourceOf("a.gohtml", "{{.A}}"), sourceOf("b.gohtml", "{{.B}}")},
			want:    "b.gohtml",
		},
		{
			name:    "no definition: a later empty tree does not replace a non-empty one",
			sources: []*templateSource{sourceOf("a.gohtml", "{{.A}}"), sourceOf("b.gohtml", " ")},
			want:    "a.gohtml",
		},
		{
			name:    "no definition: a non-empty tree replaces an empty one",
			sources: []*templateSource{sourceOf("a.gohtml", ""), sourceOf("b.gohtml", "{{.B}}")},
			want:    "b.gohtml",
		},
		{
			name:    "no definition: an empty tree replaces an empty one",
			sources: []*templateSource{sourceOf("a.gohtml", ""), sourceOf("b.gohtml", " ")},
			want:    "b.gohtml",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Each file names the template it holds after its base name, so
			// a shared define is what makes two sources compete for one name.
			defined := make(map[string]*templateSource)
			for _, src := range tt.sources {
				src.rootName = "page"
				if src.path == tt.defined {
					defined["page"] = src
				}
			}
			index, err := indexTrees(tt.sources, defined, nil, true)
			require.NoError(t, err)
			assert.Equal(t, tt.want, index["page"].src.path, "indexTrees(..., true)[page] source")
		})
	}
}

// TestDefinitionsOfListsByName states that the definitions come out in
// name order however the set holds them, so a run that finds several
// unreadable templates names the same one every time.
func TestDefinitionsOfListsByName(t *testing.T) {
	set := html.Must(html.New("c").Parse(`{{define "b"}}{{end}}{{define "e"}}{{end}}{{define "a"}}{{end}}{{define "d"}}{{end}}`))
	definitions := make(map[string]source.Definition)
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		definitions[name] = source.Definition{Name: name}
	}
	lt := &checked{Variable: source.Variable{Set: set, Definitions: definitions}}
	for range 20 {
		var got []string
		for _, definition := range definitionsOf(lt) {
			got = append(got, definition.Name)
		}
		require.Equal(t, []string{"a", "b", "c", "d", "e"}, got, "definitionsOf(...) names")
	}
}

func TestIndexTreesNamesTheSourceThatDoesNotParse(t *testing.T) {
	src := newFileSource("bad.gohtml", "bad.gohtml", "{{if}}", "", "")
	_, err := indexTrees([]*templateSource{src}, nil, nil, true)
	require.Error(t, err, "indexTrees(bad source, true)")
	assert.True(t, strings.HasPrefix(err.Error(), "bad.gohtml: "), "indexTrees(bad source, true) error = %v, want one naming bad.gohtml", err)
}

func TestVerifyReadable(t *testing.T) {
	treeOf := func(text string) *template.Tree {
		trees, err := asteval.ParseTrees("page", text, "", "", nil, true)
		require.NoError(t, err)
		return trees["page"]
	}
	definition := func(name, text string) source.Definition {
		return source.Definition{
			Name:   name,
			Tree:   treeOf(text),
			Define: source.Span{Position: token.Position{Filename: filepath.Join("/work", name+".gohtml")}},
		}
	}
	located := func(path, text string) treeLocation {
		return treeLocation{src: &templateSource{path: path}, tree: treeOf(text)}
	}

	for _, tt := range []struct {
		name      string
		defs      []source.Definition
		index     map[string]treeLocation
		want      bool
		wantPath  string
		wantAbout string
	}{
		{
			name:  "read with actions",
			defs:  []source.Definition{definition("page", "{{.A}}")},
			index: map[string]treeLocation{"page": located("page.gohtml", "{{.A}}")},
		},
		{
			name:  "a static definition needs nothing",
			defs:  []source.Definition{definition("page", "text")},
			index: map[string]treeLocation{},
		},
		{
			name:  "a definition without a tree needs nothing",
			defs:  []source.Definition{{Name: "page"}},
			index: map[string]treeLocation{},
		},
		{
			name:      "indexed without actions is reported at its source",
			defs:      []source.Definition{definition("page", "{{.A}}")},
			index:     map[string]treeLocation{"page": located("web/page.gohtml", "text")},
			want:      true,
			wantPath:  "web/page.gohtml",
			wantAbout: "page",
		},
		{
			name:      "absent is reported at its define clause",
			defs:      []source.Definition{definition("page", "{{.A}}")},
			index:     map[string]treeLocation{},
			want:      true,
			wantPath:  "../page.gohtml",
			wantAbout: "page",
		},
		{
			name: "the first unreadable definition is reported",
			defs: []source.Definition{definition("ok", "{{.A}}"), definition("one", "{{.A}}"), definition("two", "{{.A}}")},
			index: map[string]treeLocation{
				"ok": located("ok.gohtml", "{{.A}}"),
			},
			want:      true,
			wantPath:  "../one.gohtml",
			wantAbout: "one",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := verifyReadable(tt.defs, tt.index, filepath.Join("/work", "app"))
			if !tt.want {
				require.Nil(t, got, "verifyReadable(...)")
				return
			}
			require.NotNil(t, got, "verifyReadable(...)")
			assert.Equal(t, tt.wantPath, got.Path, "verifyReadable(...) path")
			assert.Equal(t, tt.wantAbout, got.Template, "verifyReadable(...) template")
		})
	}
}

func TestPlanValidate(t *testing.T) {
	var (
		group     = []Group{{}}
		mutants   = make([]mutant, 1)
		unchanged = []UnchangedTemplate{{}}
		trimmed   = []TrimmedTemplate{{}}
	)
	pattern := regexp.MustCompile("^footer$")
	for _, tt := range []struct {
		name          string
		plan          plan
		wantNoCalls   bool
		wantNoMutant  bool
		wantNoMatches bool
	}{
		{name: "empty plan reached no call site", plan: plan{}, wantNoCalls: true},
		{name: "templates counted without a group still reached no call site", plan: plan{templates: 1}, wantNoCalls: true},
		{name: "reached templates without mutants", plan: plan{groups: group, templates: 2}, wantNoMutant: true},
		{name: "mutants", plan: plan{groups: group, templates: 1, mutants: mutants}},
		{name: "only over budget actions", plan: plan{groups: group, templates: 1, overBudget: 1}},
		{name: "only unchanged templates", plan: plan{unchanged: unchanged}},
		{name: "only trimmed templates", plan: plan{trimmed: trimmed}},
		{name: "no templates counted", plan: plan{groups: group}},
		{
			name: "templates a pattern selected hold no actions",
			plan: plan{groups: group, templates: 1, pattern: pattern, reached: 2, matched: 1},
		},
		{
			name: "templates a --diff run selected hold no actions",
			plan: plan{groups: group, templates: 1, diff: "main"},
		},
		{
			name:          "a pattern matching no template reached",
			plan:          plan{trimmed: trimmed, pattern: pattern, reached: 2},
			wantNoMatches: true,
		},
		{
			name:          "a pattern matching no template reached and nothing trimmed",
			plan:          plan{pattern: pattern, reached: 1},
			wantNoMatches: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.plan.validate([]string{"templates"})
			noCalls, isNoCalls := errors.AsType[*NoCallSitesError](err)
			noMutations, isNoMutations := errors.AsType[*NoMutationsError](err)
			noMatches, isNoMatches := errors.AsType[*NoTemplateMatchesError](err)
			require.Equal(t, tt.wantNoCalls, isNoCalls, "validate() = %v: no call sites", err)
			require.Equal(t, tt.wantNoMutant, isNoMutations, "validate() = %v: no mutations", err)
			require.Equal(t, tt.wantNoMatches, isNoMatches, "validate() = %v: no template matches", err)
			require.Equal(t, tt.wantNoCalls || tt.wantNoMutant || tt.wantNoMatches, err != nil, "validate() = %v: any error", err)
			if isNoMatches {
				assert.Equal(t, "^footer$", noMatches.Pattern, "validate() pattern")
			}
			if isNoCalls {
				assert.Equal(t, []string{"templates"}, noCalls.Variables, "validate() variables")
			}
			if isNoMutations {
				assert.Equal(t, tt.plan.templates, noMutations.Templates, "validate() templates")
			}
		})
	}
}

func TestConfigurationMaxCases(t *testing.T) {
	for _, tt := range []struct{ configured, want int }{
		{configured: -1, want: DefaultMaxCases},
		{configured: 0, want: DefaultMaxCases},
		{configured: 1, want: 1},
		{configured: 20, want: 20},
	} {
		t.Run(strconv.Itoa(tt.configured), func(t *testing.T) {
			assert.Equal(t, tt.want, (Configuration{MaxCases: tt.configured}).maxCases(), "Configuration{MaxCases: %d}.maxCases()", tt.configured)
		})
	}
}

func TestTemplateFilter(t *testing.T) {
	for _, tt := range []struct {
		name    string
		pattern *regexp.Regexp
		in      string
		want    bool
	}{
		{name: "no pattern admits everything", pattern: nil, in: "anything", want: true},
		{name: "matching name", pattern: regexp.MustCompile(`^page$`), in: "page", want: true},
		{name: "other name", pattern: regexp.MustCompile(`^page$`), in: "footer", want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, templateFilter(tt.pattern)(tt.in), "templateFilter(%v)(%q)", tt.pattern, tt.in)
		})
	}
}

func TestHasRoot(t *testing.T) {
	for _, tt := range []struct {
		name string
		tree *template.Tree
		want bool
	}{
		{name: "nil tree", tree: nil, want: false},
		{name: "tree without root", tree: &template.Tree{}, want: false},
		{name: "tree with root", tree: &template.Tree{Root: &template.ListNode{}}, want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hasRoot(tt.tree), "hasRoot(%s)", tt.name)
		})
	}
}

func TestRelativePath(t *testing.T) {
	dir := filepath.Join(string(filepath.Separator), "work", "app")
	for _, tt := range []struct {
		name, dir, file, want string
	}{
		{name: "inside", dir: dir, file: filepath.Join(dir, "web", "page.gohtml"), want: "web/page.gohtml"},
		{name: "outside", dir: dir, file: filepath.Join(dir, "..", "other", "a.gohtml"), want: "../other/a.gohtml"},
		{name: "no relative path", dir: "relative", file: filepath.Join(dir, "a.gohtml"), want: filepath.Join(dir, "a.gohtml")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, relativePath(tt.dir, tt.file), "relativePath(%q, %q)", tt.dir, tt.file)
		})
	}
}

// sameBaseNameGo parses two files named row.gohtml, from two directories,
// into two template sets. Both are parsed under the name row.gohtml, so a
// cover profile cannot say which of the two a block is in.
const sameBaseNameGo = `package server

import (
	"embed"
	"html/template"
	"io"
)

//go:embed a/row.gohtml
var aFiles embed.FS

//go:embed b/row.gohtml
var bFiles embed.FS

var (
	aTemplates = template.Must(template.ParseFS(aFiles, "a/row.gohtml"))
	bTemplates = template.Must(template.ParseFS(bFiles, "b/row.gohtml"))
)

func RenderA(w io.Writer, name string) error { return aTemplates.ExecuteTemplate(w, "row.gohtml", name) }

func RenderB(w io.Writer, name string) error { return bTemplates.ExecuteTemplate(w, "row.gohtml", name) }
`

// unreachedMutants names each mutant a plan reports no test reaching.
func unreachedMutants(p *plan) []string {
	var names []string
	for _, u := range p.report().Unreached {
		names = append(names, u.Template+" "+u.File+" "+strconv.Itoa(u.Line)+":"+strconv.Itoa(u.Column)+" "+Result{Operator: u.Operator, Mutated: u.Mutated}.label())
	}
	return names
}

// TestPlanReportsUnreachedMutants states which mutants the coverage
// pre-pass says no test reaches: those whose action no executed block of
// their template overlaps. The greeting is a Go string literal, so its
// blocks are located in the literal's text while its mutants are located
// in the Go file, and the template is parsed under its own name.
//
// "Hello, {{.Name}}!{{if .Loud}} !!!{{end}}" has the name action at
// columns 8 to 16 of its text and the if at 18 to 29.
func TestPlanReportsUnreachedMutants(t *testing.T) {
	t.Parallel()
	var (
		hello   = coverBlock{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 8}
		name    = coverBlock{StartLine: 1, StartCol: 8, EndLine: 1, EndCol: 17}
		ifOpen  = coverBlock{StartLine: 1, StartCol: 18, EndLine: 1, EndCol: 30}
		whole   = coverBlock{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 41}
		nameMut = []string{`greeting template.go 8:70 action-zero`}
		ifMuts  = []string{`greeting template.go 8:80 if-false`, `greeting template.go 8:80 if-true`}
		all     = append(append([]string{}, nameMut...), ifMuts...)
	)
	greeting := map[string]string{"go.mod": goMod, "template.go": greetingGo}
	textGreeting := map[string]string{"go.mod": goMod, "template.go": strings.Replace(greetingGo, `"html/template"`, `"text/template"`, 1)}
	sameBaseName := map[string]string{
		"go.mod": goMod, "template.go": sameBaseNameGo,
		"a/row.gohtml": "<b>{{.}}</b>", "b/row.gohtml": "<i>{{.}}</i>",
	}
	for _, tt := range []struct {
		name      string
		files     map[string]string
		variables []string
		coverage  coverage
		known     bool
		want      []string
	}{
		{name: "coverage unknown", files: greeting},
		{name: "no template executed", files: greeting, coverage: coverage{}, known: true, want: all},
		{name: "only another template executed", files: greeting, coverage: coverage{"other": {whole: {}}}, known: true, want: all},
		{name: "only text before the actions executed", files: greeting, coverage: coverage{"greeting": {hello: {}}}, known: true, want: all},
		{name: "the name executed", files: greeting, coverage: coverage{"greeting": {hello: {}, name: {}}}, known: true, want: ifMuts},
		{name: "every action executed", files: greeting, coverage: coverage{"greeting": {name: {}, ifOpen: {}}}, known: true},
		{name: "a block spanning the actions", files: greeting, coverage: coverage{"greeting": {whole: {}}}, known: true},
		{name: "two templates parsed under one name", files: sameBaseName, variables: []string{"aTemplates", "bTemplates"}, coverage: coverage{}, known: true},
		{name: "a text/template set", files: textGreeting, coverage: coverage{}, known: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := module(t, tt.files)
			config := greetingConfig()
			if tt.variables != nil {
				config.TemplatesVariables = tt.variables
			}
			p, err := newPlan(t.Context(), config, dir)
			require.NoError(t, err)
			p.cover(tt.coverage, 0)
			assert.Equal(t, tt.want, unreachedMutants(p), "unreached mutants")
			assert.Equal(t, tt.known, p.report().CoverageKnown, "coverage known")
		})
	}
}

// TestCoverageReaches states how one action is read against the blocks
// executed in its template's text: reached when an executed block
// overlaps it, and always reached at a {{block}} call, which the engine
// gives no block of its own.
func TestCoverageReaches(t *testing.T) {
	const text = "ab\n{{.X}}cd\n{{block \"b\" .}}{{end}}"
	src := newFileSource("page.gohtml", "page.gohtml", text, "", "")
	action, block := src.regions[0], src.regions[1]
	require.Equal(t, "block", block.keyword, "second region")
	for _, tt := range []struct {
		name   string
		blocks map[coverBlock]struct{}
		r      region
		want   bool
	}{
		{name: "no blocks", r: action},
		{name: "the action's own block", blocks: map[coverBlock]struct{}{{StartLine: 2, StartCol: 1, EndLine: 2, EndCol: 7}: {}}, r: action, want: true},
		{name: "a block containing the action", blocks: map[coverBlock]struct{}{{StartLine: 1, StartCol: 1, EndLine: 3, EndCol: 1}: {}}, r: action, want: true},
		{name: "a block ending where the action starts", blocks: map[coverBlock]struct{}{{StartLine: 1, StartCol: 1, EndLine: 2, EndCol: 1}: {}}, r: action},
		{name: "a block starting where the action ends", blocks: map[coverBlock]struct{}{{StartLine: 2, StartCol: 7, EndLine: 2, EndCol: 9}: {}}, r: action},
		{name: "a block past the end of the text", blocks: map[coverBlock]struct{}{{StartLine: 9, StartCol: 1, EndLine: 9, EndCol: 4}: {}}, r: action},
		{name: "a block call", r: block, want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cov := coverage{"page.gohtml": tt.blocks}
			got := cov.reaches("page.gohtml", newLineIndex(src.text), tt.r)
			assert.Equal(t, tt.want, got, "reaches(%q)", src.text[tt.r.start:tt.r.end])
		})
	}
}

// sharedChainGo defines "own" and, in the same set, "shared", which "own"
// renders. Each holds one action that mutates.
const sharedChainGo = `package server

import (
	"html/template"
	"io"
)

var templates = template.Must(template.Must(template.New("own").Parse(` + "`{{if .}}a{{end}}{{template \"shared\" .}}`" + `)).New("shared").Parse(` + "`{{if .}}b{{end}}`" + `))

func Render(w io.Writer, on bool) error {
	return templates.ExecuteTemplate(w, "own", on)
}
`

// TestPlanMutatesOnlyTemplatesTheVariableOwns states that no mutant is
// planned for a template defined in another package's files, which that
// package owns, while the package's own templates are mutated.
func TestPlanMutatesOnlyTemplatesTheVariableOwns(t *testing.T) {
	for _, tt := range []struct {
		name         string
		sharedFrom   string
		wantTemplate []string
	}{
		{name: "both defined here", wantTemplate: []string{"own bool", "shared bool"}},
		{name: "shared defined in another package", sharedFrom: "example.com/other", wantTemplate: []string{"own bool"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			in, err := inputFrom(dir, loadtest.Package(t, dir, "example.com/server", map[string]string{
				"go.mod":      goMod,
				"template.go": sharedChainGo,
			}), []string{"templates"})
			require.NoError(t, err)
			require.Len(t, in.pkg.Variables, 1)
			variable := in.pkg.Variables[0]
			if tt.sharedFrom != "" {
				definition, ok := variable.Definitions["shared"]
				require.True(t, ok, "definitions %v", variable.Definitions)
				definition.Package = tt.sharedFrom
				variable.Definitions["shared"] = definition
			}

			p, err := planFrom(Configuration{TemplatesVariables: []string{"templates"}, Seed: 1, SeedSet: true}, in, nil, "")

			require.NoError(t, err)
			assert.ElementsMatch(t, tt.wantTemplate, mutatedTemplates(p), "mutated templates")
		})
	}
}
