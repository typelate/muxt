package template

import (
	"io"
	"reflect"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// FuncMap is the type of the map defining the mapping from names to functions.
// Each value must be a function with a single return value, or a function
// with two return values where the second is an error.
type FuncMap map[string]any

// TreeFinder looks up associated templates by name.
type TreeFinder interface {
	FindTree(name string) (*Tree, bool)
}

// FindTreeFunc adapts a function to the TreeFinder interface.
type FindTreeFunc func(name string) (*Tree, bool)

func (f FindTreeFunc) FindTree(name string) (*Tree, bool) {
	return f(name)
}

// Option configures template execution.
type Option func(*config)

type config struct {
	funcs      FuncMap
	finder     TreeFinder
	missingKey MissingKeyMode
	cp         *CoverProfile
	trust      TrustFunc
}

// MissingKeyMode controls how missing map keys and struct fields are handled.
type MissingKeyMode int

const (
	// MissingKeyInvalid is the default: missing struct fields are an error,
	// missing map keys return the zero value.
	MissingKeyInvalid MissingKeyMode = iota
	// MissingKeyZero causes missing struct fields and map keys to return
	// the zero value instead of an error.
	MissingKeyZero
	// MissingKeyError causes both missing struct fields and missing map
	// keys to return an error.
	MissingKeyError
)

// WithFuncs provides custom template functions.
func WithFuncs(funcs FuncMap) Option {
	return func(c *config) {
		c.funcs = funcs
	}
}

// WithFinder provides a TreeFinder for resolving {{template}} calls.
func WithFinder(finder TreeFinder) Option {
	return func(c *config) {
		c.finder = finder
	}
}

// WithMissingKey sets the behavior for missing map keys and struct fields.
func WithMissingKey(mode MissingKeyMode) Option {
	return func(c *config) {
		c.missingKey = mode
	}
}

// WithCoverProfile enables coverage profiling during template execution.
func WithCoverProfile(cp *CoverProfile) Option {
	return func(c *config) {
		c.cp = cp
	}
}

// WithTrust registers a TrustFunc used to detect safe-content types so
// they bypass the relevant contextual escaping in HTML mode.
func WithTrust(f TrustFunc) Option {
	return func(c *config) {
		c.trust = f
	}
}

// maxExecDepth specifies the maximum nesting of {{template}} calls.
const maxExecDepth = 100000

// ExecuteTemplate executes a template tree, writing output to w.
// For HTML trees (root contains HTMLDocumentFragmentNode), it builds an
// html.Node intermediate representation and uses html.Render.
// For plain text trees, it writes directly like text/template.
//
// ExecuteTemplate, ExecuteDocument and ExecuteElement are muxt's
// static-analysis and coverage engine: they run templates so that tests
// and tooling can observe what a template produces and which parts of it
// ran. They are not safe for serving HTTP responses; serve with
// html/template. Their contextual escaping is known to fall short of
// html/template's in three places:
//
//   - An action in attribute-name position (<div {{.Name}}="x">,
//     data-{{.Key}}="x") is in a tag the parser reads as text, so the
//     tag, the action's output included, is written as escaped HTML text
//     (&lt;div ...&gt;) rather than as a tag with a filtered attribute
//     name.
//   - A text (non-HTML) sub-template called from an HTML template is
//     executed without the caller's escaping context.
//   - A style or srcdoc attribute's value is not escaped for the CSS or
//     HTML document it holds.
func ExecuteTemplate(w io.Writer, tree *Tree, data any, opts ...Option) error {
	cfg := config{}
	for _, o := range opts {
		o(&cfg)
	}
	s := &state{
		wr:         w,
		tree:       tree,
		cp:         cfg.cp,
		funcs:      cfg.funcs,
		finder:     cfg.finder,
		missingKey: cfg.missingKey,
		trust:      cfg.trust,
	}
	s.vars = []variable{{"$", reflect.ValueOf(data)}}
	return s.execute(reflect.ValueOf(data))
}

// ExecuteDocument executes an HTML template tree, returning an *html.Node (DocumentNode).
// The tree's root must contain an HTMLDocumentFragmentNode.
// This returns the DOM tree directly without rendering to a writer.
func ExecuteDocument(tree *Tree, data any, opts ...Option) (*html.Node, error) {
	cfg := config{}
	for _, o := range opts {
		o(&cfg)
	}
	s := &state{
		tree:       tree,
		cp:         cfg.cp,
		funcs:      cfg.funcs,
		finder:     cfg.finder,
		missingKey: cfg.missingKey,
		trust:      cfg.trust,
	}
	dot := reflect.ValueOf(data)
	s.vars = []variable{{"$", dot}}
	root := tree.Root
	if root == nil {
		return &html.Node{Type: html.DocumentNode}, nil
	}
	if len(root.Nodes) == 1 {
		if frag, ok := root.Nodes[0].(*HTMLDocumentFragmentNode); ok {
			return s.buildDocument(dot, frag)
		}
	}
	return nil, errorf(tree.Name, "tree is not an HTML template")
}

// ExecuteElement evaluates an HTMLElementNode against data, returning an *html.Node.
// This enables DOM interop with libraries like Cascadia and github.com/typelate/dom.
func ExecuteElement(tree *Tree, elem *HTMLElementNode, data any, opts ...Option) (*html.Node, error) {
	cfg := config{}
	for _, o := range opts {
		o(&cfg)
	}
	s := &state{
		tree:       tree,
		cp:         cfg.cp,
		funcs:      cfg.funcs,
		finder:     cfg.finder,
		missingKey: cfg.missingKey,
		trust:      cfg.trust,
	}
	s.vars = []variable{{"$", reflect.ValueOf(data)}}
	return s.walkHTMLElement(reflect.ValueOf(data), elem)
}

func htmlAtom(tagName string) atom.Atom {
	return atom.Lookup([]byte(tagName))
}
