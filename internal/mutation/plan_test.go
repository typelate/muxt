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
	"text/template/parse"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/asteval"
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
}

// TestPlanReportCounts states how a plan's mutants are counted: an action
// too wide to enumerate counts once, as skipped, alongside the mutants
// skipped for not type checking.
func TestPlanReportCounts(t *testing.T) {
	p := &plan{mutants: make([]Mutant, 5), runnableN: 3, overBudget: 2}
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
			index, err := indexTrees(tt.sources, defined, nil)
			require.NoError(t, err)
			assert.Equal(t, tt.want, index["page"].src.path, "indexTrees(...)[page] source")
		})
	}
}

func TestIndexTreesNamesTheSourceThatDoesNotParse(t *testing.T) {
	src := newFileSource("bad.gohtml", "bad.gohtml", "{{if}}", "", "")
	_, err := indexTrees([]*templateSource{src}, nil, nil)
	require.Error(t, err, "indexTrees(bad source)")
	assert.True(t, strings.HasPrefix(err.Error(), "bad.gohtml: "), "indexTrees(bad source) error = %v, want one naming bad.gohtml", err)
}

func TestVerifyReadable(t *testing.T) {
	treeOf := func(text string) *parse.Tree {
		trees, err := asteval.ParseTrees("page", text, "", "", nil)
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
		mutants   = make([]Mutant, 1)
		unchanged = []UnchangedTemplate{{}}
		trimmed   = []TrimmedTemplate{{}}
	)
	for _, tt := range []struct {
		name         string
		plan         plan
		wantNoCalls  bool
		wantNoMutant bool
	}{
		{name: "empty plan reached no call site", plan: plan{}, wantNoCalls: true},
		{name: "templates counted without a group still reached no call site", plan: plan{templates: 1}, wantNoCalls: true},
		{name: "reached templates without mutants", plan: plan{groups: group, templates: 2}, wantNoMutant: true},
		{name: "mutants", plan: plan{groups: group, templates: 1, mutants: mutants}},
		{name: "only over budget actions", plan: plan{groups: group, templates: 1, overBudget: 1}},
		{name: "only unchanged templates", plan: plan{unchanged: unchanged}},
		{name: "only trimmed templates", plan: plan{trimmed: trimmed}},
		{name: "no templates counted", plan: plan{groups: group}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.plan.validate([]string{"templates"})
			noCalls, isNoCalls := errors.AsType[*NoCallSitesError](err)
			noMutations, isNoMutations := errors.AsType[*NoMutationsError](err)
			require.Equal(t, tt.wantNoCalls, isNoCalls, "validate() = %v: no call sites", err)
			require.Equal(t, tt.wantNoMutant, isNoMutations, "validate() = %v: no mutations", err)
			require.Equal(t, tt.wantNoCalls || tt.wantNoMutant, err != nil, "validate() = %v: any error", err)
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
		tree *parse.Tree
		want bool
	}{
		{name: "nil tree", tree: nil, want: false},
		{name: "tree without root", tree: &parse.Tree{}, want: false},
		{name: "tree with root", tree: &parse.Tree{Root: &parse.ListNode{}}, want: true},
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
