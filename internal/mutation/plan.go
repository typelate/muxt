package mutation

import (
	"fmt"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"text/template/parse"

	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/asteval"
)

// plan is everything decided before a single test is run: which templates
// are reached, with what type of dot, and which variations apply.
//
// Enumerating is fast and running is slow, so the plan is also what a dry
// run reports and what the estimate is built from.
type plan struct {
	mutants    []Mutant
	groups     []Group
	trimmed    []TrimmedTemplate
	templates  int
	complexity int
	runnableN  int
}

func (p *plan) total() int    { return len(p.mutants) }
func (p *plan) runnable() int { return p.runnableN }

func (p *plan) report() *Report {
	trimmed := p.trimmed
	if trimmed == nil {
		trimmed = []TrimmedTemplate{}
	}
	groups := p.groups
	if groups == nil {
		groups = []Group{}
	}
	return &Report{
		Templates:  p.templates,
		Complexity: p.complexity,
		Total:      len(p.mutants),
		// Skips are decided while planning, not while running, so a dry
		// run reports them too.
		Skipped: len(p.mutants) - p.runnableN,
		Groups:  groups,
		Trimmed: trimmed,
	}
}

// newPlan loads the project, walks the templates each ExecuteTemplate
// call reaches, and enumerates the variations available in each.
func newPlan(config Configuration, workingDirectory string) (*plan, error) {
	pl, err := loadPackages(workingDirectory, config.IncludeTests)
	if err != nil {
		return nil, err
	}

	include := func(string) bool { return true }
	if config.TemplatePattern != nil {
		include = config.TemplatePattern.MatchString
	}

	p := new(plan)
	funcs := parseBuiltins()
	seen := make(map[string]struct{})

	for _, templatesVariable := range config.TemplatesVariables {
		lt, err := asteval.LoadTemplates(workingDirectory, templatesVariable, pl)
		if err != nil {
			return nil, err
		}
		for name := range lt.Templates.Functions() {
			funcs[name] = func() string { return "" }
		}

		index, err := buildTreeIndex(lt, workingDirectory, pl, funcs)
		if err != nil {
			return nil, err
		}

		scopes, trimmed := traverse(lt, index)
		for _, t := range trimmed {
			if !config.IncludeTests && isTestFile(t.call.Position.Filename) {
				continue
			}
			p.trimmed = append(p.trimmed, TrimmedTemplate{
				CallSite:    relativePosition(workingDirectory, t.call.Position),
				Template:    t.template,
				DataType:    typeDisplay(t.dataType),
				FirstSeenAt: relativePosition(workingDirectory, t.firstFor.Position),
			})
		}

		for _, sc := range scopes {
			if !config.IncludeTests && isTestFile(sc.call.Position.Filename) {
				// A template rendered only by a test is not rendered
				// in production, and mutating it measures the tests
				// against themselves.
				continue
			}
			if !include(sc.template) {
				continue
			}
			key := sc.template + "\x00" + typeKey(sc.dataType)
			if _, done := seen[key]; done {
				continue
			}
			seen[key] = struct{}{}
			p.add(lt, sc, funcs, workingDirectory)
		}
	}

	if len(p.groups) == 0 && len(p.trimmed) == 0 {
		return nil, &NoCallSitesError{Variables: config.TemplatesVariables}
	}
	return p, nil
}

// add enumerates one template's mutants and files them under the call
// that reaches it.
func (p *plan) add(lt *asteval.LoadedTemplates, sc scope, funcs map[string]any, workingDirectory string) {
	found := mutantsInScope(sc)

	report := TemplateReport{
		Template:   sc.template,
		File:       sc.src.path,
		DataType:   typeDisplay(sc.dataType),
		Via:        sc.via,
		Complexity: complexity(sc.tree.Root),
	}
	p.templates++
	p.complexity += report.Complexity

	for _, mutant := range found {
		result := Result{
			Operator: mutant.Operator,
			Line:     mutant.Line,
			Column:   mutant.Column,
			Original: mutant.Action(),
			Mutated:  mutant.Replacement(),
			Status:   StatusPending,
		}
		if reason, ok := invalid(lt, sc, mutant, funcs); ok {
			result.Status = StatusSkipped
			result.Reason = reason
		} else {
			p.runnableN++
		}
		result.mutantIndex = len(p.mutants)
		p.mutants = append(p.mutants, mutant)
		report.Results = append(report.Results, result)
	}

	site := relativePosition(workingDirectory, sc.call.Position)
	for i := range p.groups {
		if p.groups[i].CallSite == site && p.groups[i].Entry == sc.call.Template {
			p.groups[i].Templates = append(p.groups[i].Templates, report)
			return
		}
	}
	p.groups = append(p.groups, Group{
		CallSite:  site,
		Entry:     sc.call.Template,
		DataType:  typeDisplay(sc.call.DataType),
		Templates: []TemplateReport{report},
	})
}

// invalid reports whether a mutant would break the template rather than
// change its behaviour, and why.
//
// A mutation that stops the template parsing or type checking would make
// the tests fail with a render error. That failure would be recorded as
// the mutation being caught, which is a lie: nothing asserted on the
// behaviour, the template just stopped working.
func invalid(lt *asteval.LoadedTemplates, sc scope, mutant Mutant, funcs map[string]any) (string, bool) {
	mutated := sc.src.mutatedText(mutant)
	trees, err := parse.Parse(sc.src.rootName, mutated, "", "", funcs)
	if err != nil {
		return "does not parse", true
	}
	tree, ok := trees[sc.template]
	if !ok || tree == nil || tree.Root == nil {
		return "template not found after mutation", true
	}
	if !checks(lt, tree, sc.dataType) {
		return "does not type check against " + typeDisplay(sc.dataType), true
	}
	return "", false
}

// buildTreeIndex parses every text the template set was loaded from and
// indexes the templates it defines by name.
//
// The trees are parsed here rather than taken from the template set so
// that every node position is an offset into text this package holds,
// which is what a mutation is spliced into.
func buildTreeIndex(lt *asteval.LoadedTemplates, workingDirectory string, pl []*packages.Package, funcs map[string]any) (map[string]treeLocation, error) {
	collector := newSourceCollector(workingDirectory, pl)
	for _, t := range lt.HTML.Templates() {
		definition, ok := lt.Templates.FindDefinition(t.Name())
		if !ok {
			continue
		}
		if err := collector.add(definition); err != nil {
			return nil, err
		}
	}

	index := make(map[string]treeLocation)
	for _, src := range collector.sorted() {
		trees, err := parse.Parse(src.rootName, src.text, "", "", funcs)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", src.path, err)
		}
		for name, tree := range trees {
			if tree == nil || tree.Root == nil {
				continue
			}
			if existing, ok := index[name]; ok && !parse.IsEmptyTree(existing.tree.Root) {
				continue
			}
			index[name] = treeLocation{src: src, tree: tree}
		}
	}
	return index, nil
}

// loadPackages loads the working directory's package, optionally
// including its test files so that ExecuteTemplate calls written in tests
// are visible.
func loadPackages(workingDirectory string, includeTests bool) ([]*packages.Package, error) {
	if !includeTests {
		_, pl, err := asteval.LoadPackages(workingDirectory)
		return pl, err
	}
	fileSet := token.NewFileSet()
	pl, err := packages.Load(&packages.Config{
		Fset:  fileSet,
		Tests: true,
		Mode: packages.NeedModule | packages.NeedTypesInfo | packages.NeedName |
			packages.NeedFiles | packages.NeedTypes | packages.NeedSyntax |
			packages.NeedEmbedPatterns | packages.NeedEmbedFiles | packages.NeedImports,
		Dir: workingDirectory,
	}, workingDirectory, "encoding", "fmt", "net/http")
	if err != nil {
		return nil, err
	}
	// With Tests set, go list reports the package twice: once as it is
	// written and once compiled with its in-package test files. The
	// second is the one holding a test's ExecuteTemplate calls, so it
	// has to come first when a package is picked by directory.
	slices := make([]*packages.Package, 0, len(pl))
	for _, pkg := range pl {
		if strings.HasSuffix(pkg.ID, ".test]") {
			slices = append(slices, pkg)
		}
	}
	for _, pkg := range pl {
		if !strings.HasSuffix(pkg.ID, ".test]") {
			slices = append(slices, pkg)
		}
	}
	return slices, nil
}

func isTestFile(filename string) bool {
	return strings.HasSuffix(filename, "_test.go")
}

func relativePosition(workingDirectory string, position token.Position) string {
	if !position.IsValid() {
		return "?"
	}
	path := position.Filename
	if rel, err := filepath.Rel(workingDirectory, path); err == nil {
		path = filepath.ToSlash(rel)
	}
	return path + ":" + strconv.Itoa(position.Line) + ":" + strconv.Itoa(position.Column)
}
