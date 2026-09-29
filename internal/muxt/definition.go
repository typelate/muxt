package muxt

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"html/template"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/source"
)

// Definitions parses route definitions from the template names in a
// templates variable's set. When the variable locates a template's
// definition, errors about its name carry a file position.
func Definitions(variable source.Variable) ([]Definition, error) {
	ts, templatesVariable := variable.Set, variable.Name
	var (
		defs     []Definition
		failures []nameFailure
	)
	for _, t := range ts.Templates() {
		mt, err, ok := newDefinition(t)
		if !ok {
			continue
		}
		if pos, found := variable.NamePosition(t.Name()); found {
			mt.namePosition = pos
		}
		if err != nil {
			// Collect every malformed name so one run reports them all.
			mt.sourceFile = templateSourceFile(t)
			failures = append(failures, nameFailure{def: mt, err: mt.finishNameError(err, mt.handlerSpan())})
			continue
		}
		if t.Tree != nil && t.Tree.ParseName != "" {
			mt.sourceFile = t.Tree.ParseName
		}
		mt.templatesVariable = templatesVariable

		defs = append(defs, mt)
	}
	if len(failures) > 0 {
		return defs, combineNameFailures(failures)
	}
	slices.SortFunc(defs, Definition.byPathThenMethod)
	calculateIdentifiers(defs)

	analyzeRedirectCalls(ts, defs)

	if err := checkResponseWriterConflicts(ts, defs); err != nil {
		return defs, err
	}

	return defs, nil
}

type nameFailure struct {
	def Definition
	err error
}

// combineNameFailures joins the errors in one order. The template set
// iterates in map order; sorting keeps the report stable across runs.
func combineNameFailures(failures []nameFailure) error {
	slices.SortFunc(failures, func(a, b nameFailure) int {
		if n := cmp.Compare(a.def.sourceFile, b.def.sourceFile); n != 0 {
			return n
		}
		if n := cmp.Compare(a.def.namePosition.Offset, b.def.namePosition.Offset); n != 0 {
			return n
		}
		return cmp.Compare(a.def.name, b.def.name)
	})
	errs := make([]error, 0, len(failures))
	for _, failure := range failures {
		errs = append(errs, failure.err)
	}
	return CombineErrors(errs)
}

// templateSourceFile returns the file t was parsed from, or "". ParseFS and
// ParseFiles record the file name as the tree's ParseName; a template defined
// directly with Parse carries its own name there instead, so a ParseName
// matching the template name does not identify a file.
func templateSourceFile(t *template.Template) string {
	if t.Tree == nil || t.Tree.ParseName == t.Name() {
		return ""
	}
	return t.Tree.ParseName
}

// definitionLocation renders where this definition's name literal was
// written: the full position when the define clause was located, the
// file name alone when only that is known, or "".
func (def Definition) definitionLocation() string {
	if def.namePosition.IsValid() {
		return def.namePosition.String()
	}
	return def.sourceFile
}

type Definition struct {
	// name has the full unaltered template name
	name string

	// method, host, path, and pattern are parsed sub-parts of the string passed to mux.Handle
	method, host, path, pattern string

	// handler is used to generate the method interface
	handler string

	// defaultStatusCode is the status code to use in the response header for this template endpoint
	defaultStatusCode int

	fun  *ast.Ident
	call *ast.CallExpr
	sig  *types.Signature

	isMethod    bool
	resultShape ResultShape

	fileSet *token.FileSet

	template *template.Template

	identifier string

	resultStatusCode ResultStatusCode

	hasResponseWriterArg bool

	// sourceFile is the base filename (e.g., "index.gohtml") from which this template was parsed.
	// Empty string means the template was defined via Parse() calls rather than from a file.
	sourceFile string

	// canRedirect is whether this template, or one it calls, may call a redirect method.
	canRedirect bool

	// templatesVariable is the name of the package-level *template.Template
	// variable that contains this template (e.g., "templates", "adminTemplates")
	templatesVariable string

	usesSignals     bool
	signalsCallback string

	// spans records the byte offsets of the matched name segments and
	// handlerOffset the offset of the trimmed handler expression, so
	// errors can point at the failing part of the name. namePosition is
	// where the name literal's content begins in the defining file, when
	// the definition was found.
	spans         nameSpans
	handlerOffset int
	namePosition  token.Position

	// related holds "file:line:col: note" lines that give errors about
	// this definition context, such as where the handler method is
	// defined in Go source.
	related []string

	// synthesizedMethods lists the signatures ResolveCall inferred for
	// methods the receiver does not define yet, so generation can tell
	// the user typed checking is deferred until the method exists.
	synthesizedMethods []string

	Representation Representation

	Segments  []Segment
	Arguments []Argument
}

type Representation string

const (
	// RepresentationTextHTML Representation = ""

	RepresentationSSE Representation = "sse"

	// RepresentationMarshalJSON responds application/json with the marshaled
	// method result; the define body executes only for its side effects.
	RepresentationMarshalJSON Representation = "marshalJSON"
)

func (def Definition) SourceFile() string { return def.sourceFile }
func (def Definition) RawPattern() string { return def.pattern }

// Pattern returns a normalized http.ServeMux pattern.
func (def Definition) Pattern() string {
	var sb strings.Builder

	if m := def.HTTPMethod(); m != "" {
		sb.WriteString(m)
		sb.WriteString(" ")
	}

	if h := def.Host(); h != "" {
		sb.WriteString(h)
	}
	sb.WriteString(def.Path())

	return sb.String()
}

func (def Definition) Name() string { return def.name }
func (def Definition) Path() string { return strings.TrimSpace(def.path) }
func (def Definition) Host() string { return strings.ToLower(strings.TrimSpace(def.host)) }

// HTTPMethod does normalization based on the convention (not requirement) that method characters are ASCII and uppercase
func (def Definition) HTTPMethod() string { return strings.ToUpper(strings.TrimSpace(def.method)) }

func (def Definition) DefaultStatusCode() int         { return def.defaultStatusCode }
func (def Definition) MayRedirect() bool              { return def.canRedirect }
func (def Definition) Template() *template.Template   { return def.template }
func (def Definition) FunctionIdentifier() *ast.Ident { return def.fun }
func (def Definition) CallExpression() *ast.CallExpr  { return cloneCall(def.call) }
func (def Definition) HasResponseWriterArg() bool     { return def.hasResponseWriterArg }
func (def Definition) Identifier() string             { return def.identifier }
func (def Definition) TemplatesVariable() string      { return def.templatesVariable }
func (def Definition) Signature() source.Type {
	if def.sig == nil {
		return source.Type{}
	}
	return source.NewType(def.sig)
}
func (def Definition) IsMethod() bool                     { return def.isMethod }
func (def Definition) ResultShape() ResultShape           { return def.resultShape }
func (def Definition) ResultStatusCode() ResultStatusCode { return def.resultStatusCode }

// ResultType is the type of the template data's Result field.
func (def Definition) ResultType() source.Type { return source.NewType(def.resultType()) }
func (def Definition) UsesSignals() bool       { return def.usesSignals }

func (def Definition) IsIndex() bool {
	p := def.Path()
	return p == "/" || p == "/{$}"
}

// HasPathEndWildcard reports when the special path has the "{$}" wildcard
func (def Definition) HasPathEndWildcard() bool {
	return strings.HasSuffix(def.Path(), "{$}")
}

// SignalsCallback returns the first Signals-suffixed callback argument name,
// if the route has one.
func (def Definition) SignalsCallback() (string, bool) {
	return def.signalsCallback, def.signalsCallback != ""
}

// SynthesizedMethods lists the signatures ResolveCall inferred for
// handler methods the receiver does not define. The results are
// untyped (any), so template field checks are deferred until the
// method exists.
func (def Definition) SynthesizedMethods() []string { return def.synthesizedMethods }

func (def Definition) String() string { return def.name }

func (def Definition) functionName() string {
	if def.fun == nil {
		return ""
	}
	return def.fun.Name
}

// IsRouteDefinitionName reports whether name has the shape of a route
// template definition: an HTTP pattern optionally followed by a status
// code and handler call.
func IsRouteDefinitionName(name string) bool {
	return templateNameMux.MatchString(name)
}

func newDefinition(t *template.Template) (Definition, error, bool) {
	in := t.Name()
	if !templateNameMux.MatchString(in) {
		return Definition{}, nil, false
	}
	matches := templateNameMux.FindStringSubmatch(in)
	def := Definition{
		name:              in,
		method:            matches[templateNameMux.SubexpIndex("METHOD")],
		host:              matches[templateNameMux.SubexpIndex("HOST")],
		path:              matches[templateNameMux.SubexpIndex("PATH")],
		handler:           strings.TrimSpace(matches[templateNameMux.SubexpIndex("CALL")]),
		pattern:           matches[templateNameMux.SubexpIndex("pattern")],
		fileSet:           token.NewFileSet(),
		defaultStatusCode: http.StatusOK,
		template:          t,
		spans:             newNameSpans(templateNameMux.FindStringSubmatchIndex(in)),
	}
	if def.handler != "" && def.spans.call[0] >= 0 {
		def.handlerOffset = def.spans.call[0] + strings.Index(in[def.spans.call[0]:], def.handler)
	}
	httpStatusCode := matches[templateNameMux.SubexpIndex("HTTP_STATUS")]
	if httpStatusCode != "" {
		if err := def.parseStatusCode(httpStatusCode); err != nil {
			return def, err, true
		}
	}
	if err := def.checkPathAndMethod(); err != nil {
		return def, err, true
	}
	if err := def.initializeSegments(); err != nil {
		return def, err, true
	}
	if err := parseHandler(def.fileSet, &def, def.Segments); err != nil {
		return def, err, true
	}
	if httpStatusCode != "" && !def.callWriteHeader() {
		return def, def.statusCodeConflictError(), true
	}
	return def, nil, true
}

func (def *Definition) parseStatusCode(text string) error {
	if strings.HasPrefix(text, "http.Status") {
		code, err := astgen.HTTPStatusName(text)
		if err != nil {
			return def.spanErrorf(def.spans.status, "invalid status code %s: %v", text, err)
		}
		def.defaultStatusCode = code
		return nil
	}
	code, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return def.spanErrorf(def.spans.status, "invalid status code %q: expected an integer like 201 or a constant name like http.StatusCreated", strings.TrimSpace(text))
	}
	def.defaultStatusCode = code
	return nil
}

func (def Definition) checkPathAndMethod() error {
	if len(def.path) > 1 {
		if idx := strings.Index(def.path, "//"); idx >= 0 {
			return def.nameErrorf(def.spans.path[0]+idx+1, 1, "path has an empty segment")
		}
		if strings.HasSuffix(def.path, "/") {
			return def.nameErrorf(def.spans.path[0]+len(def.path)-1, 1, "path has an empty segment")
		}
	}
	switch def.method {
	case "", http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return nil
	}
	return def.spanErrorf(def.spans.method, "%s method not allowed; allowed methods: GET, POST, PUT, PATCH, and DELETE", def.method)
}

func (def Definition) statusCodeConflictError() error {
	const message = "cannot use %s as an argument and also set an HTTP status code in the template name; the handler writes the header through %[1]s"
	if node := findIdent(def.call, TemplateNameScopeIdentifierHTTPResponse); node != nil {
		return errAt(node, message, TemplateNameScopeIdentifierHTTPResponse)
	}
	return fmt.Errorf(message, TemplateNameScopeIdentifierHTTPResponse)
}

var templateNameMux = regexp.MustCompile(`^(?P<pattern>((?P<METHOD>[A-Z]+)\s+)?(?P<HOST>([^/])*)(?P<PATH>(/(\S)*)))(\s+(?P<HTTP_STATUS>(\d|http\.Status)\S+))?(?P<CALL>.*)?$`)

// callWriteHeader reports whether muxt writes the status code: it does not when
// the handler takes the response as a direct argument.
func (def Definition) callWriteHeader() bool {
	if def.call == nil {
		return true
	}
	return !slices.ContainsFunc(def.call.Args, func(arg ast.Expr) bool {
		ident, ok := arg.(*ast.Ident)
		return ok && ident.Name == TemplateNameScopeIdentifierHTTPResponse
	})
}

func (def Definition) ExecuteArgumentIndex() (int, bool) {
	for i, arg := range def.Arguments {
		if arg.Type == ArgumentTypeExecute &&
			arg.Identifier == TemplateNameScopeIdentifierExecute {
			return i, true
		}
	}
	return 0, false
}

func cloneCall(call *ast.CallExpr) *ast.CallExpr {
	clone := *call
	clone.Args = make([]ast.Expr, len(call.Args))
	for i, arg := range call.Args {
		switch arg := arg.(type) {
		case *ast.CallExpr:
			clone.Args[i] = cloneCall(arg)
		case *ast.Ident:
			ident := *arg
			clone.Args[i] = &ident
		default:
			clone.Args[i] = arg
		}
	}
	return &clone
}
