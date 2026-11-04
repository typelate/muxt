package mutation

import (
	"cmp"
	"context"
	"fmt"
	"go/token"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/typelate/muxt/internal/template"

	"github.com/typelate/muxt/internal/check"

	"github.com/typelate/muxt/internal/asteval"
	"github.com/typelate/muxt/internal/source"
)

// plan is everything decided before a single test is run: which templates
// are reached, with what type of dot, and which variations apply.
//
// Enumerating is fast and running is slow, so the plan is also what a dry
// run reports and what the estimate is built from.
type plan struct {
	mutants    []mutant
	groups     []Group
	trimmed    []TrimmedTemplate
	templates  int
	complexity int
	runnableN  int
	overBudget int
	seed       uint64
	draw       *values
	maxCases   int

	// diff is the revision a --diff run compares with, diffError why the
	// templates there could not be read, and unchanged the templates
	// left alone because nothing about them changed since.
	diff      string
	diffError string
	unchanged []UnchangedTemplate

	// pattern is --template-pattern, nil when every template is mutated.
	// reached counts the templates the calls reach and matched those the
	// pattern admitted, which is how a pattern that matches nothing is
	// told from templates that hold nothing to vary.
	pattern          *regexp.Regexp
	reached, matched int

	// coverage is the template blocks the tests executed, from the
	// coverage pre-pass; nil when it is unknown.
	coverage coverage

	// candidates are the runnable mutants coverage may show no test
	// reaches, and unreached the ones it does once every template set
	// is planned: whether a parse name is ambiguous is only known then.
	candidates []candidate
	unreached  []UnreachedMutant

	// includeUnreached runs the unreached mutants rather than skipping
	// them: --include-uncovered-nodes.
	includeUnreached bool

	// parsedFrom is the texts each parse name was given to, which says
	// whether a block filed under one is ambiguous.
	parsedFrom parseNameSources

	// prePass is how long the coverage pre-pass took, zero when it did
	// not run.
	prePass time.Duration
}

// candidate is a runnable mutant in an html/template set, with the name
// its template's text is parsed under at run time, which is what a cover
// profile keys its blocks on.
type candidate struct {
	mutant    int
	parseName string
	result    Result
	file      string
}

// narrowed reports whether the run was asked to mutate only some of the
// templates it reaches.
func (p *plan) narrowed() bool { return p.pattern != nil || p.diff != "" }

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
	var note string
	if len(p.mutants) == 0 && p.overBudget == 0 {
		// Only a narrowed run gets this far with nothing to mutate; an
		// unnarrowed one is a NoMutationsError.
		note = "nothing to mutate: the selected templates hold no dynamic or control flow actions"
	}
	return &Report{
		Note:       note,
		Templates:  p.templates,
		Complexity: p.complexity,
		Seed:       p.seed,
		Total:      len(p.mutants) + p.overBudget,
		// Skips are decided while planning, not while running, so a dry
		// run reports them too.
		Skipped:   len(p.mutants) - p.runnableN + p.overBudget,
		Groups:    groups,
		Trimmed:   trimmed,
		Diff:      p.diff,
		DiffError: p.diffError,
		Unchanged: p.unchanged,

		CoverageKnown: p.coverage != nil,
		Unreached:     p.unreached,

		IncludeUncoveredNodes: p.includeUnreached,
	}
}

// selection is what a run decided about the scopes one templates variable
// reached: which to mutate, which a --diff run left alone, and which
// repeats to report as trimmed.
type selection struct {
	mutate    []scope
	unchanged []UnchangedTemplate
	trimmed   []TrimmedTemplate

	// reached counts the scopes the traversal found and matched the ones
	// the template pattern admitted.
	reached, matched int
}

// selector decides what a run mutates. It holds what that decision needs
// and nothing else: no loader, no filesystem, no git.
//
// seen and unchanged carry across the templates variables of one run, so
// that a template reached from two of them is mutated once, and a repeat
// of a template left alone is reported as trimmed nowhere.
type selector struct {
	// before is what the templates read like at the --diff revision. Nil
	// means there is nothing to compare with, so every scope counts as
	// changed.
	before revision

	seen      map[string]struct{}
	unchanged map[string]struct{}
	include   func(string) bool

	// wd is the directory positions are reported relative to.
	wd string
}

// choose decides what to do with the scopes and trims of one traversal.
func (s selector) choose(scopes []scope, trims []trim) selection {
	var chosen selection
	for _, sc := range scopes {
		chosen.reached++
		if !s.include(sc.template) {
			continue
		}
		chosen.matched++
		key := executionKey(sc.template, sc.dataType)
		if _, done := s.seen[key]; done {
			continue
		}
		s.seen[key] = struct{}{}
		if s.before != nil && !s.before.changed(sc) {
			s.unchanged[key] = struct{}{}
			chosen.unchanged = append(chosen.unchanged, UnchangedTemplate{
				Template: sc.template,
				File:     sc.src.path,
				DataType: typeDisplay(sc.dataType),
			})
			continue
		}
		chosen.mutate = append(chosen.mutate, sc)
	}

	reported := make(map[TrimmedTemplate]struct{})
	for _, t := range trims {
		if !s.include(t.template) {
			// A template the pattern excludes is mutated nowhere, so
			// there is no first mutation for the trim to point at.
			continue
		}
		if _, skipped := s.unchanged[executionKey(t.template, t.dataType)]; skipped {
			// Unchanged means mutated nowhere too.
			continue
		}
		entry := TrimmedTemplate{
			CallSite:    relativePosition(s.wd, t.call.Position),
			Template:    t.template,
			DataType:    typeDisplay(t.dataType),
			FirstSeenAt: relativePosition(s.wd, t.firstFor.Position),
		}
		if _, said := reported[entry]; said {
			// One template may invoke a partial several times. That is one
			// thing to say once, not once per invocation.
			continue
		}
		reported[entry] = struct{}{}
		chosen.trimmed = append(chosen.trimmed, entry)
	}
	return chosen
}

func newPlan(ctx context.Context, config Configuration, workingDirectory string, cov coverage) (*plan, error) {
	in, err := loadInput(workingDirectory, config, config.env)
	if err != nil {
		return nil, err
	}
	// Loading takes no context, so a run interrupted while loading
	// stops here instead.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// before is what the templates looked like at the --diff revision.
	// Nil means there is nothing to compare with, so every template
	// counts as changed.
	var (
		before    revision
		diffError string
	)
	if config.Diff != "" {
		dir, cleanup, err := checkout(ctx, workingDirectory, config.Diff)
		if err != nil {
			return nil, err
		}
		defer cleanup()
		if before, err = templatesAt(config, dir); err != nil {
			diffError = err.Error()
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	p, err := planFrom(config, in, before, diffError)
	if err != nil {
		return nil, err
	}
	p.cover(cov, 0)
	return p, nil
}

// cover records what the coverage pre-pass saw the tests execute, nil
// when it is unknown, and how long it took, and finds the mutants it
// shows no test reaches, which it skips unless the run includes them. It
// comes after planning, so that a run with nothing to mutate, or one that
// cannot plan at all, never waits for the pre-pass.
func (p *plan) cover(cov coverage, took time.Duration) {
	p.coverage = cov
	p.prePass = took
	p.unreached = p.findUnreached(p.parsedFrom)
	if !p.includeUnreached {
		p.skipUnreached()
	}
}

// unreachedReason is why a mutant no test reaches was not run.
const unreachedReason = "no test reaches it"

// skipUnreached marks every unreached mutant skipped, so it is not run
// and counts among the skips. A mutation nothing executes is all but
// certain to be missed, and running it costs a run of the suite to learn
// what the pre-pass already said.
func (p *plan) skipUnreached() {
	if len(p.unreached) == 0 {
		return
	}
	unreached := make(map[int]struct{}, len(p.unreached))
	for _, u := range p.unreached {
		unreached[u.mutantIndex] = struct{}{}
	}
	for i := range p.groups {
		for j := range p.groups[i].Templates {
			results := p.groups[i].Templates[j].Results
			for k := range results {
				if _, ok := unreached[results[k].mutantIndex]; !ok || results[k].Status != StatusPending {
					continue
				}
				results[k].Status = StatusSkipped
				results[k].Reason = unreachedReason
				p.runnableN--
			}
		}
	}
}

// planFrom walks the templates each ExecuteTemplate call in the input
// reaches and enumerates the variations available in each.
//
// It is everything a plan decides, from inputs that hold no loader: before
// is what the templates read like at the --diff revision, nil when there
// is none, and diffError why that revision could not be read.
func planFrom(config Configuration, in input, before revision, diffError string) (*plan, error) {
	p := &plan{
		seed:      config.Seed,
		draw:      newValues(config.Seed),
		maxCases:  config.maxCases(),
		diff:      config.Diff,
		diffError: diffError,
		pattern:   config.TemplatePattern,

		includeUnreached: config.IncludeUncoveredNodes,
	}
	sel := selector{
		before:    before,
		seen:      make(map[string]struct{}),
		unchanged: make(map[string]struct{}),
		include:   templateFilter(config.TemplatePattern),
		wd:        in.dir,
	}
	parsedFrom := make(parseNameSources)

	for _, variable := range in.pkg.Variables {
		lt := newChecked(in.pkg, variable)

		index, err := buildTreeIndex(lt, in.dir)
		if err != nil {
			return nil, err
		}
		parsedFrom.add(lt, index)

		chosen := sel.choose(traverse(lt, index))
		for _, sc := range chosen.mutate {
			p.add(lt, sc, in.dir)
		}
		p.unchanged = append(p.unchanged, chosen.unchanged...)
		p.trimmed = append(p.trimmed, chosen.trimmed...)
		p.reached += chosen.reached
		p.matched += chosen.matched
	}
	p.parsedFrom = parsedFrom

	if err := p.validate(config.TemplatesVariables); err != nil {
		return nil, err
	}
	return p, nil
}

func (c Configuration) maxCases() int {
	if c.MaxCases <= 0 {
		return DefaultMaxCases
	}
	return c.MaxCases
}

// templateFilter admits every template name when pattern is nil.
func templateFilter(pattern *regexp.Regexp) func(string) bool {
	if pattern == nil {
		return func(string) bool { return true }
	}
	return pattern.MatchString
}

func (p *plan) validate(variables []string) error {
	if p.pattern != nil && p.reached > 0 && p.matched == 0 {
		return &NoTemplateMatchesError{Pattern: p.pattern.String()}
	}
	if len(p.groups) == 0 && len(p.trimmed) == 0 && len(p.unchanged) == 0 {
		return &NoCallSitesError{Variables: variables}
	}
	if p.templates > 0 && len(p.mutants) == 0 && p.overBudget == 0 && !p.narrowed() {
		// A template that is entirely static is possible, but a whole run
		// of them means the actions were not read, and reporting that as
		// a pass would say the tests catch everything. A run narrowed to
		// some templates may well have picked only static ones.
		return &NoMutationsError{Templates: p.templates}
	}
	return nil
}

// add enumerates one template's mutants and files them under the call
// that reaches it.
func (p *plan) add(lt *checked, sc scope, workingDirectory string) {
	found, notes := mutantsInScope(sc, lt.Functions, p.draw, p.maxCases)

	report := TemplateReport{
		Template:   sc.template,
		File:       sc.src.path,
		DataType:   typeDisplay(sc.dataType),
		Via:        sc.via,
		Complexity: complexity(sc.tree.Root),
	}
	p.templates++
	p.complexity += report.Complexity

	for _, m := range found {
		result := Result{
			Operator: m.Operator,
			Line:     m.Line,
			Column:   m.Column,
			Original: m.action,
			Mutated:  m.detail,
			Status:   StatusPending,
		}
		if m.Operator == OperatorConditionDead {
			// Simplification already proved this condition cannot change
			// the decision, so no test could be coupled to it and there
			// is nothing to learn from running it, or from parsing and
			// type checking it first.
			result.Status = StatusSkipped
			result.Reason = m.detail
			result.Mutated = ""
		} else if reason, broken := invalid(lt, sc, m); broken {
			result.Status = StatusSkipped
			result.Reason = reason
		} else {
			p.runnableN++
			if name := parseNameOf(lt, sc.template); name != "" {
				p.candidates = append(p.candidates, candidate{mutant: len(p.mutants), parseName: name, result: result, file: report.File})
			}
		}
		result.mutantIndex = len(p.mutants)
		p.mutants = append(p.mutants, m)
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
	// Stable, so results that tie keep the order they were enumerated in
	// rather than whatever order the sort leaves them.
	slices.SortStableFunc(report.Results, func(a, b Result) int {
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

// parseNameOf is the name a template's text is parsed under when the
// tests execute it, which is the name a cover profile files its blocks
// under: a file's base name, or the name of the template a Go string was
// parsed into. The set was loaded the way the tests load it, so its tree
// carries that name.
//
// It is empty, meaning unknown, for a text/template set: the coverage
// pre-pass lays muxt's engine over html/template only, so the tests never
// report executing one.
func parseNameOf(lt *checked, name string) string {
	if lt.Text {
		return ""
	}
	t := lt.Set.Lookup(name)
	if t == nil || t.Tree == nil {
		return ""
	}
	return t.Tree.ParseName
}

// parseNameSources is the texts each parse name was given to, across
// every template set in the package, by file and offset of the literal
// in it.
type parseNameSources map[string]map[sourceKey]struct{}

func (s parseNameSources) add(lt *checked, index map[string]treeLocation) {
	for name, location := range index {
		parseName := parseNameOf(lt, name)
		if parseName == "" {
			continue
		}
		if s[parseName] == nil {
			s[parseName] = make(map[sourceKey]struct{})
		}
		s[parseName][sourceKey{file: location.src.file, litStart: location.src.litStart}] = struct{}{}
	}
}

// ambiguous reports whether more than one text was parsed under a name,
// in which case a block filed under it may be in either.
func (s parseNameSources) ambiguous(parseName string) bool { return len(s[parseName]) > 1 }

// findUnreached lists the runnable mutants the coverage pre-pass saw no
// test reach. A mutant is unreached only when coverage is known, its
// template's parse name holds no other text in the package, and no block
// executed under that name overlaps the action the mutant changes. A
// template the tests never executed has no blocks, so every mutant in it
// is unreached.
//
// Finding them skips nothing; cover decides whether they run.
func (p *plan) findUnreached(parsedFrom parseNameSources) []UnreachedMutant {
	if p.coverage == nil {
		return nil
	}
	lines := make(map[*templateSource]lineIndex)
	var unreached []UnreachedMutant
	for _, c := range p.candidates {
		if parsedFrom.ambiguous(c.parseName) {
			continue
		}
		m := p.mutants[c.mutant]
		_, r, ok := regionAt(m.src.regions, m.start())
		if !ok {
			continue
		}
		index, ok := lines[m.src]
		if !ok {
			index = newLineIndex(m.src.text)
			lines[m.src] = index
		}
		if p.coverage.reaches(c.parseName, index, r) {
			continue
		}
		unreached = append(unreached, UnreachedMutant{
			Template: m.Template,
			File:     c.file,
			Line:     c.result.Line,
			Column:   c.result.Column,
			Operator: c.result.Operator,
			Mutated:  c.result.Mutated,

			mutantIndex: c.mutant,
		})
	}
	return unreached
}

// reaches reports whether a block executed in the template parsed under
// parseName overlaps the action r, located in the text lines indexes.
// Blocks are spans of the parsed text, end exclusive, as the engine's
// cover profile writes them.
//
// A {{block}} call is always reached: the engine records no block for the
// call itself, only for the body, which an override may replace.
func (c coverage) reaches(parseName string, lines lineIndex, r region) bool {
	if r.keyword == "block" {
		return true
	}
	for b := range c[parseName] {
		start, startOK := lines.offset(b.StartLine, b.StartCol)
		end, endOK := lines.offset(b.EndLine, b.EndCol)
		if !startOK || !endOK {
			// Not a span of this text: the block is in another one
			// parsed under the same name.
			continue
		}
		if start < r.end && r.start < end {
			return true
		}
	}
	return false
}

// invalid reports whether a mutant would break the template rather than
// change its behaviour, and why.
//
// A mutation that stops the template parsing or type checking would make
// the tests fail with a render error. That failure would be recorded as
// the mutation being caught, which is a lie: nothing asserted on the
// behaviour, the template just stopped working.
func invalid(lt *checked, sc scope, m mutant) (string, bool) {
	mutated := sc.src.mutatedText(m.edits)
	trees, err := asteval.ParseTrees(sc.src.rootName, mutated, sc.src.leftDelim, sc.src.rightDelim, lt.Functions, !lt.Text)
	if err != nil {
		return "does not parse", true
	}
	tree, ok := trees[sc.template]
	if !ok || !hasRoot(tree) {
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
func buildTreeIndex(lt *checked, workingDirectory string) (map[string]treeLocation, error) {
	defs := definitionsOf(lt)
	collector := newSourceCollector(workingDirectory, defs)
	defined := make(map[string]*templateSource, len(defs))
	for _, definition := range defs {
		src, err := collector.add(definition)
		if err != nil {
			return nil, err
		}
		if src != nil {
			defined[definition.Name] = src
		}
	}

	index, err := indexTrees(collector.sorted(), defined, lt.Functions, !lt.Text)
	if err != nil {
		return nil, err
	}
	if unreadable := verifyReadable(defs, index, workingDirectory); unreadable != nil {
		return nil, unreadable
	}
	return index, nil
}

// definitionsOf lists the definitions of the templates in the set. They
// are gathered before the collector is built: a source scans its actions
// as it is constructed, and it can only do that once the delimiters its
// file was written with are known, which is something the definitions say.
//
// They are sorted by name: the set holds its templates in a map, and an
// error naming the first unreadable one has to name the same one every run.
func definitionsOf(lt *checked) []source.Definition {
	var defs []source.Definition
	for _, t := range lt.Set.Templates() {
		if definition, ok := lt.Definitions[t.Name()]; ok {
			defs = append(defs, definition)
		}
	}
	slices.SortFunc(defs, func(a, b source.Definition) int { return cmp.Compare(a.Name, b.Name) })
	return defs
}

// indexTrees parses each source once and indexes the trees by template
// name.
//
// A name several sources hold is taken from the source its definition
// is in, which is the one the template set kept: text/template keeps the
// last body that is not empty, so a {{block}} default overridden by a
// later file's define is never rendered, and mutating it would measure
// nothing. defined names that source for each template the loader
// located.
//
// A name with no located definition keeps the last tree that is not
// empty, the way the template set would have. html is false for a
// text/template set, whose sources parse as text.
func indexTrees(sources []*templateSource, defined map[string]*templateSource, functions check.Functions, html bool) (map[string]treeLocation, error) {
	index := make(map[string]treeLocation)
	for _, src := range sources {
		trees, err := asteval.ParseTrees(src.rootName, src.text, src.leftDelim, src.rightDelim, functions, html)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", src.path, err)
		}
		for name, tree := range trees {
			if !hasRoot(tree) {
				continue
			}
			if owner, ok := defined[name]; ok {
				if owner == src {
					index[name] = treeLocation{src: src, tree: tree}
				}
				continue
			}
			if existing, ok := index[name]; ok && template.IsEmptyTree(tree.Root) && !template.IsEmptyTree(existing.tree.Root) {
				continue
			}
			index[name] = treeLocation{src: src, tree: tree}
		}
	}
	return index, nil
}

// verifyReadable reports the first definition holding actions that the
// index does not.
//
// The template set found actions there and the re-parse found none, so
// the text was read with delimiters it was not written in. Left alone the
// template contributes no mutants and the run reports a smaller job rather
// than a problem. Absent from the index is the same failure as present and
// empty: with the wrong delimiters the define clause is not recognised as
// one, so no tree is produced under that name at all.
func verifyReadable(defs []source.Definition, index map[string]treeLocation, workingDirectory string) *UnreadableTemplateError {
	for _, definition := range defs {
		if definition.Tree == nil || countActions(definition.Tree.Root) == 0 {
			continue
		}
		location, indexed := index[definition.Name]
		if indexed && countActions(location.tree.Root) > 0 {
			continue
		}
		path := relativePath(workingDirectory, definition.Define.Position.Filename)
		if indexed {
			path = location.src.path
		}
		return &UnreadableTemplateError{Template: definition.Name, Path: path}
	}
	return nil
}

func hasRoot(tree *template.Tree) bool { return tree != nil && tree.Root != nil }

// countActions reports how many dynamic or control flow actions a tree
// holds, which is what a template has to offer a mutation.
func countActions(node template.Node) int {
	switch n := node.(type) {
	case *template.ListNode:
		if n == nil {
			return 0
		}
		total := 0
		for _, child := range n.Nodes {
			total += countActions(child)
		}
		return total
	case *template.ActionNode, *template.TemplateNode, *template.BlockNode:
		// A block counts as the {{template}} call text/template made of it;
		// its body is a tree of its own.
		return 1
	case *template.IfNode:
		return 1 + countActions(n.List) + countActions(n.ElseList)
	case *template.WithNode:
		return 1 + countActions(n.List) + countActions(n.ElseList)
	case *template.RangeNode:
		return 1 + countActions(n.List) + countActions(n.ElseList)
	case *template.HTMLDocumentFragmentNode:
		return countActionsIn(n.Children)
	case *template.HTMLElementNode:
		return countActionsIn(n.Attr) + countActionsIn(n.Children)
	case *template.HTMLAttributeNode:
		return countActionsIn(n.Key) + countActionsIn(n.Value)
	case *template.HTMLCommentNode:
		return countActionsIn(n.Children)
	default:
		return 0
	}
}

// countActionsIn counts the actions among the children of an HTML node,
// which the copied parser builds where text/template kept flat text.
func countActionsIn(nodes []template.Node) int {
	total := 0
	for _, child := range nodes {
		total += countActions(child)
	}
	return total
}

func relativePosition(workingDirectory string, position token.Position) string {
	if !position.IsValid() {
		return "?"
	}
	path := relativePath(workingDirectory, position.Filename)
	return path + ":" + strconv.Itoa(position.Line) + ":" + strconv.Itoa(position.Column)
}

// offset turns a one based line and column, counting bytes, back into an
// offset, reporting false for a line the text does not have or a column
// past the end of its line.
func (l lineIndex) offset(line, column int) (int, bool) {
	if line < 1 || line > len(l) || column < 1 {
		return 0, false
	}
	offset := l[line-1] + column - 1
	if line < len(l) && offset >= l[line] {
		return 0, false
	}
	return offset, true
}
