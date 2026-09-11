package mutation

import (
	"cmp"
	"fmt"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/template/parse"

	"github.com/typelate/check"
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
	overBudget int
	seed       uint64
	draw       *values
	maxCases   int
}

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
		Seed:       p.seed,
		Total:      len(p.mutants) + p.overBudget,
		// Skips are decided while planning, not while running, so a dry
		// run reports them too.
		Skipped: len(p.mutants) - p.runnableN + p.overBudget,
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

	p := &plan{
		seed:     config.Seed,
		draw:     newValues(config.Seed),
		maxCases: config.MaxCases,
	}
	if p.maxCases <= 0 {
		p.maxCases = DefaultMaxCases
	}
	seen := make(map[string]struct{})

	for _, templatesVariable := range config.TemplatesVariables {
		lt, err := asteval.LoadTemplates(workingDirectory, templatesVariable, pl)
		if err != nil {
			return nil, err
		}
		functions := lt.Templates.Functions()

		index, err := buildTreeIndex(lt, workingDirectory, pl, functions)
		if err != nil {
			return nil, err
		}

		scopes, trimmed := traverse(lt, index)
		reported := make(map[TrimmedTemplate]struct{})
		for _, t := range trimmed {
			entry := TrimmedTemplate{
				CallSite:    relativePosition(workingDirectory, t.call.Position),
				Template:    t.template,
				DataType:    typeDisplay(t.dataType),
				FirstSeenAt: relativePosition(workingDirectory, t.firstFor.Position),
			}
			if _, said := reported[entry]; said {
				// One template may invoke a partial several times. That
				// is one thing to say once, not once per invocation.
				continue
			}
			reported[entry] = struct{}{}
			p.trimmed = append(p.trimmed, entry)
		}

		for _, sc := range scopes {
			if !include(sc.template) {
				continue
			}
			key := executionKey(sc.template, sc.dataType)
			if _, done := seen[key]; done {
				continue
			}
			seen[key] = struct{}{}
			p.add(lt, sc, functions, workingDirectory)
		}
	}

	if len(p.groups) == 0 && len(p.trimmed) == 0 {
		return nil, &NoCallSitesError{Variables: config.TemplatesVariables}
	}
	if p.templates > 0 && len(p.mutants) == 0 && p.overBudget == 0 {
		// Templates were reached and held nothing to mutate. A template
		// that is entirely static is possible, but a whole run of them
		// means the actions were not read, and reporting that as a pass
		// would say the tests catch everything.
		return nil, &NoMutationsError{Templates: p.templates}
	}
	return p, nil
}

// add enumerates one template's mutants and files them under the call
// that reaches it.
func (p *plan) add(lt *asteval.LoadedTemplates, sc scope, functions check.Functions, workingDirectory string) {
	found, notes := mutantsInScope(sc, functions, p.draw, p.maxCases)

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
		switch reason, broken := invalid(lt, sc, mutant, functions); {
		case mutant.Operator == OperatorConditionDead:
			// Simplification already proved this condition cannot change
			// the decision, so no test could be coupled to it and there
			// is nothing to learn from running it.
			result.Status = StatusSkipped
			result.Reason = mutant.Replacement()
			result.Mutated = ""
		case broken:
			result.Status = StatusSkipped
			result.Reason = reason
		default:
			p.runnableN++
		}
		result.mutantIndex = len(p.mutants)
		p.mutants = append(p.mutants, mutant)
		report.Results = append(report.Results, result)
	}

	p.overBudget += len(notes)
	for _, note := range notes {
		// An action too wide to enumerate is reported where its mutants
		// would have been, so the gap is visible in the same place a
		// reader is already looking.
		report.Results = append(report.Results, Result{
			Status:   StatusSkipped,
			Operator: OperatorOperands,
			Line:     note.line,
			Column:   note.column,
			Reason:   note.reason(),
		})
	}
	slices.SortFunc(report.Results, func(a, b Result) int {
		return cmp.Or(
			cmp.Compare(a.Line, b.Line),
			cmp.Compare(a.Column, b.Column),
			cmp.Compare(a.Operator, b.Operator),
			cmp.Compare(a.Mutated, b.Mutated),
		)
	})

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
func invalid(lt *asteval.LoadedTemplates, sc scope, mutant Mutant, functions check.Functions) (string, bool) {
	mutated := sc.src.mutatedText(mutant.edits)
	trees, err := asteval.ParseTrees(sc.src.rootName, mutated, sc.src.leftDelim, sc.src.rightDelim, functions)
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
func buildTreeIndex(lt *asteval.LoadedTemplates, workingDirectory string, pl []*packages.Package, functions check.Functions) (map[string]treeLocation, error) {
	// The definitions are gathered before the collector is built: a
	// source scans its actions as it is constructed, and it can only do
	// that once the delimiters its file was written with are known,
	// which is something the definitions say.
	var defs []check.Definition
	for _, t := range lt.HTML.Templates() {
		definition, ok := lt.Templates.FindDefinition(t.Name())
		if !ok {
			continue
		}
		defs = append(defs, definition)
	}

	collector := newSourceCollector(workingDirectory, pl, defs)
	for _, definition := range defs {
		if _, err := collector.add(definition); err != nil {
			return nil, err
		}
	}

	index := make(map[string]treeLocation)
	for _, src := range collector.sorted() {
		trees, err := asteval.ParseTrees(src.rootName, src.text, src.leftDelim, src.rightDelim, functions)
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

	for _, definition := range defs {
		if definition.Tree == nil || countActions(definition.Tree.Root) == 0 {
			continue
		}
		location, indexed := index[definition.Name]
		// Absent is the same failure as present and empty: read with the
		// wrong delimiters, the define clause is not recognised as one,
		// so no tree is produced under that name at all.
		if !indexed || countActions(location.tree.Root) == 0 {
			path := collector.relative(definition.Define.Position.Filename)
			if indexed {
				path = location.src.path
			}
			// The template set found actions here and this re-parse found
			// none, so the text was read with delimiters it was not
			// written in. Left alone the template contributes no mutants
			// and the run reports a smaller job rather than a problem.
			return nil, &UnreadableTemplateError{
				Template: definition.Name,
				Path:     path,
			}
		}
	}
	return index, nil
}

// countActions reports how many dynamic or control flow actions a tree
// holds, which is what a template has to offer a mutation.
func countActions(node parse.Node) int {
	switch n := node.(type) {
	case *parse.ListNode:
		if n == nil {
			return 0
		}
		total := 0
		for _, child := range n.Nodes {
			total += countActions(child)
		}
		return total
	case *parse.ActionNode, *parse.TemplateNode:
		return 1
	case *parse.IfNode:
		return 1 + countActions(n.List) + countActions(n.ElseList)
	case *parse.WithNode:
		return 1 + countActions(n.List) + countActions(n.ElseList)
	case *parse.RangeNode:
		return 1 + countActions(n.List) + countActions(n.ElseList)
	default:
		return 0
	}
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
