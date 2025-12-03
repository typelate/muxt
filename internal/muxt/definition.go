package muxt

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"html/template"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/template/parse"

	"github.com/typelate/muxt/internal/astgen"
)

func Definitions(ts *template.Template) ([]Definition, error) {
	var defs []Definition
	patterns := make(map[string]struct{})
	for _, t := range ts.Templates() {
		mt, err, ok := newDefinition(t)
		if !ok {
			continue
		}
		if err != nil {
			return defs, err
		}
		pattern := strings.Join([]string{mt.method, mt.host, mt.path}, " ")
		if _, exists := patterns[pattern]; exists {
			return defs, fmt.Errorf("duplicate route pattern: %s", mt.pattern)
		}

		// Extract source file from ParseName if available
		if t.Tree != nil && t.Tree.ParseName != "" {
			// ParseName contains the filename used when parsing
			mt.sourceFile = t.Tree.ParseName
		}
		// else sourceFile remains empty string for Parse() defined templates

		patterns[pattern] = struct{}{}
		defs = append(defs, mt)
	}
	slices.SortFunc(defs, Definition.byPathThenMethod)
	calculateIdentifiers(defs)

	// Analyze templates to determine which ones can call Redirect
	analyzeRedirectCalls(ts, defs)

	return defs, nil
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

	fileSet *token.FileSet

	template *template.Template

	pathValueTypes map[string]types.Type
	pathValueNames []string

	identifier string

	hasResponseWriterArg bool

	// sourceFile is the base filename (e.g., "index.gohtml") from which this template was parsed.
	// Empty string means the template was defined via Parse() calls rather than from a file.
	sourceFile string

	// canRedirect indicates whether this template (or any template it calls) can call the Redirect method.
	// This is determined by static analysis of the template's action nodes.
	canRedirect bool
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
		pathValueTypes:    make(map[string]types.Type),
		template:          t,
	}
	httpStatusCode := matches[templateNameMux.SubexpIndex("HTTP_STATUS")]
	if httpStatusCode != "" {
		if strings.HasPrefix(httpStatusCode, "http.Status") {
			code, err := astgen.HTTPStatusName(httpStatusCode)
			if err != nil {
				return Definition{}, fmt.Errorf("failed to parse status code: %w", err), true
			}
			def.defaultStatusCode = code
		} else {
			code, err := strconv.Atoi(strings.TrimSpace(httpStatusCode))
			if err != nil {
				return Definition{}, fmt.Errorf("failed to parse status code: %w", err), true
			}
			def.defaultStatusCode = code
		}
	}

	if len(def.path) > 1 {
		segments := strings.Split(def.path[1:], "/")
		for _, segment := range segments {
			if segment == "" {
				return Definition{}, fmt.Errorf("template has an empty path segment: %s", def.name), true
			}
		}
	}

	switch def.method {
	default:
		return def, fmt.Errorf("%s method not allowed", def.method), true
	case "", http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	}

	pathValueNames := def.parsePathValueNames()
	if err := checkPathValueNames(pathValueNames); err != nil {
		return Definition{}, err, true
	}
	def.pathValueNames = pathValueNames

	err := parseHandler(def.fileSet, &def, def.pathValueNames)
	if err != nil {
		return def, err, true
	}

	if def.fun == nil {
		for _, name := range def.pathValueNames {
			def.pathValueTypes[name] = types.Universe.Lookup("string").Type()
		}
	}

	if httpStatusCode != "" && !def.callWriteHeader(nil) {
		return def, fmt.Errorf("you can not use %s as an argument and specify an HTTP status code", TemplateNameScopeIdentifierHTTPResponse), true
	}

	return def, nil, true
}

var (
	pathSegmentPattern = regexp.MustCompile(`/\{([^}]*)}`)
	templateNameMux    = regexp.MustCompile(`^(?P<pattern>(((?P<METHOD>[A-Z]+)\s+)?)(?P<HOST>([^/])*)(?P<PATH>(/(\S)*)))(\s+(?P<HTTP_STATUS>(\d|http\.Status)\S+))?(?P<CALL>.*)?$`)
)

func (def Definition) parsePathValueNames() []string {
	var result []string
	for _, match := range pathSegmentPattern.FindAllStringSubmatch(def.path, strings.Count(def.path, "/")) {
		n := match[1]
		if n == "$" && strings.Count(def.path, "$") == 1 && strings.HasSuffix(def.path, "{$}") {
			continue
		}
		n = strings.TrimSuffix(n, "...")
		result = append(result, n)
	}
	return result
}

func hasHTTPResponseWriterArgument(call *ast.CallExpr) bool {
	for _, a := range call.Args {
		switch arg := a.(type) {
		case *ast.Ident:
			if arg.Name == TemplateNameScopeIdentifierHTTPResponse {
				return true
			}
		case *ast.CallExpr:
			if hasHTTPResponseWriterArgument(arg) {
				return true
			}
		}
	}
	return false
}

func checkPathValueNames(in []string) error {
	for i, n := range in {
		if !token.IsIdentifier(n) {
			return fmt.Errorf("path parameter name not permitted: %q is not a Go identifier", n)
		}
		if slices.Contains(in[:i], n) {
			return fmt.Errorf("forbidden repeated path parameter names: found at least 2 path parameters with name %q", n)
		}
		if slices.Contains(patternScope(), n) {
			return fmt.Errorf("the name %s is not allowed as a path parameter it is already in scope", n)
		}
	}
	return nil
}

func (def Definition) String() string { return def.name }

func (def Definition) Method() string {
	if def.fun == nil {
		return ""
	}
	return def.fun.Name
}

func (def Definition) Template() *template.Template {
	return def.template
}

func (def Definition) byPathThenMethod(d Definition) int {
	if n := cmp.Compare(def.path, d.path); n != 0 {
		return n
	}
	if m := cmp.Compare(def.method, d.method); m != 0 {
		return m
	}
	return cmp.Compare(def.handler, d.handler)
}

func parseHandler(fileSet *token.FileSet, def *Definition, pathParameterNames []string) error {
	if def.handler == "" {
		return nil
	}
	e, err := parser.ParseExprFrom(fileSet, "template_name.go", []byte(def.handler), 0)
	if err != nil {
		loc, _ := def.template.Tree.ErrorContext(def.template.Tree.Root)
		return fmt.Errorf("failed to parse handler expression %s: %v", loc, err)
	}
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return fmt.Errorf("expected call expression, got: %s", astgen.Format(e))
	}
	fun, ok := call.Fun.(*ast.Ident)
	if !ok {
		return fmt.Errorf("expected function identifier, got got: %s", astgen.Format(call.Fun))
	}
	if call.Ellipsis != token.NoPos {
		return fmt.Errorf("unexpected ellipsis")
	}

	scope := append(patternScope(), pathParameterNames...)
	slices.Sort(scope)
	if err := checkArguments(scope, call); err != nil {
		return err
	}

	def.fun = fun
	def.call = call

	def.hasResponseWriterArg = hasHTTPResponseWriterArgument(call)

	return nil
}

func (def Definition) callWriteHeader(receiverInterfaceType *ast.InterfaceType) bool {
	if def.call == nil {
		return true
	}
	return !hasIdentArgument(def.call.Args, TemplateNameScopeIdentifierHTTPResponse, receiverInterfaceType, 1, 1)
}

func hasIdentArgument(args []ast.Expr, ident string, receiverInterfaceType *ast.InterfaceType, depth, maxDepth int) bool {
	if depth > maxDepth {
		return false
	}
	for _, arg := range args {
		switch exp := arg.(type) {
		case *ast.Ident:
			if exp.Name == ident {
				return true
			}
		case *ast.CallExpr:
			methodIdent, ok := exp.Fun.(*ast.Ident)
			if ok && receiverInterfaceType != nil {
				field, ok := astgen.FindFieldWithName(receiverInterfaceType.Methods, methodIdent.Name)
				if ok {
					funcType, ok := field.Type.(*ast.FuncType)
					if ok {
						if funcType.Results.NumFields() == 1 {
							if hasIdentArgument(exp.Args, ident, receiverInterfaceType, depth+1, maxDepth+1) {
								return true
							}
						}
					}
				}
			}
		}
	}
	return false
}

func checkArguments(identifiers []string, call *ast.CallExpr) error {
	for i, a := range call.Args {
		switch exp := a.(type) {
		case *ast.Ident:
			if _, ok := slices.BinarySearch(identifiers, exp.Name); !ok {
				return fmt.Errorf("unknown argument %s at index %d", exp.Name, i)
			}
		case *ast.CallExpr:
			if err := checkArguments(identifiers, exp); err != nil {
				return fmt.Errorf("call %s argument error: %w", astgen.Format(call.Fun), err)
			}
		default:
			return fmt.Errorf("expected only identifier or call expressions as arguments, argument at index %d is: %s", i, astgen.Format(a))
		}
	}
	return nil
}

const (
	TemplateNameScopeIdentifierHTTPRequest  = "request"
	TemplateNameScopeIdentifierHTTPResponse = "response"
	TemplateNameScopeIdentifierContext      = "ctx"
	TemplateNameScopeIdentifierForm         = "form"

	TemplateDataFieldIdentifierResult        = "result"
	TemplateDataFieldIdentifierOkay          = "okay"
	TemplateDataFieldIdentifierRedirectURL   = "redirectURL"
	TemplateDataFieldIdentifierError         = "errList"
	TemplateDataFieldIdentifierReceiver      = "receiver"
	TemplateDataFieldIdentifierStatusCode    = "statusCode"
	TemplateDataFieldIdentifierErrStatusCode = "errStatusCode"
)

func patternScope() []string {
	return []string{
		TemplateNameScopeIdentifierHTTPRequest,
		TemplateNameScopeIdentifierHTTPResponse,
		TemplateNameScopeIdentifierContext,
		TemplateNameScopeIdentifierForm,
	}
}

func (def Definition) matchReceiver(funcDecl *ast.FuncDecl, receiverTypeIdent string) bool {
	if funcDecl == nil || funcDecl.Name == nil || funcDecl.Name.Name != def.fun.Name ||
		funcDecl.Recv == nil || len(funcDecl.Recv.List) < 1 {
		return false
	}
	exp := funcDecl.Recv.List[0].Type
	if star, ok := exp.(*ast.StarExpr); ok {
		exp = star.X
	}
	ident, ok := exp.(*ast.Ident)
	return ok && ident.Name == receiverTypeIdent
}

func (def Definition) callHandleFunc(file *File, handlerFuncLit *ast.FuncLit, config RoutesFileConfiguration) *ast.ExprStmt {
	pattern := ast.Expr(astgen.String(def.pattern))
	if config.PathPrefix {
		i := strings.Index(def.pattern, "/")
		pattern = &ast.BinaryExpr{
			X:  astgen.String(def.pattern[:i]),
			Op: token.ADD,
			Y:  astgen.Call(file, "path", "path", "Join", ast.NewIdent(pathPrefixPathsStructFieldName), astgen.String(def.pattern[i:])),
		}
	}
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(muxVarIdent),
			Sel: ast.NewIdent(httpHandleFuncIdent),
		},
		Args: []ast.Expr{pattern, handlerFuncLit},
	}}
}

// analyzeRedirectCalls performs static analysis on all templates to determine
// which ones can call the Redirect method. It updates the canRedirect field
// on each Definition in the templates slice.
func analyzeRedirectCalls(ts *template.Template, defs []Definition) {
	// Build a map from template name to template index for quick lookup
	templateMap := make(map[string]int)
	for i := range defs {
		templateMap[defs[i].name] = i
	}

	// For each template, check if it can redirect
	for i := range defs {
		t := ts.Lookup(defs[i].name)
		if t == nil || t.Tree == nil {
			continue
		}
		visited := make(map[string]bool)
		defs[i].canRedirect = canTemplateRedirect(t.Tree.Root, ts, templateMap, defs, visited)
	}
}

// canTemplateRedirect recursively checks if a template tree can call Redirect.
// It returns true if:
// 1. The template directly calls .Redirect
// 2. The template calls another template that can redirect
// 3. The template passes TemplateData to a function (conservatively assume it might redirect)
// 4. The template calls a non-default method on TemplateData (conservatively assume it might redirect)
// The visited map tracks templates currently being analyzed to prevent infinite recursion on circular references.
func canTemplateRedirect(node parse.Node, ts *template.Template, templateMap map[string]int, defs []Definition, visited map[string]bool) bool {
	if node == nil {
		return false
	}

	switch n := node.(type) {
	case *parse.ListNode:
		if n == nil {
			return false
		}
		for _, child := range n.Nodes {
			if canTemplateRedirect(child, ts, templateMap, defs, visited) {
				return true
			}
		}

	case *parse.ActionNode:
		if n.Pipe != nil {
			for _, cmd := range n.Pipe.Cmds {
				if containsRedirectCall(cmd) {
					return true
				}
				// Check if TemplateData is passed as argument to a function
				if callsMethodOnTemplateData(cmd) {
					return true
				}
			}
		}

	case *parse.IfNode:
		if canTemplateRedirect(n.Pipe, ts, templateMap, defs, visited) {
			return true
		}
		if canTemplateRedirect(n.List, ts, templateMap, defs, visited) {
			return true
		}
		if canTemplateRedirect(n.ElseList, ts, templateMap, defs, visited) {
			return true
		}

	case *parse.RangeNode:
		if canTemplateRedirect(n.Pipe, ts, templateMap, defs, visited) {
			return true
		}
		if canTemplateRedirect(n.List, ts, templateMap, defs, visited) {
			return true
		}
		if canTemplateRedirect(n.ElseList, ts, templateMap, defs, visited) {
			return true
		}

	case *parse.WithNode:
		if canTemplateRedirect(n.Pipe, ts, templateMap, defs, visited) {
			return true
		}
		if canTemplateRedirect(n.List, ts, templateMap, defs, visited) {
			return true
		}
		if canTemplateRedirect(n.ElseList, ts, templateMap, defs, visited) {
			return true
		}

	case *parse.TemplateNode:
		// Check if the called template can redirect
		// Prevent infinite recursion on circular template references
		if visited[n.Name] {
			return false
		}
		visited[n.Name] = true
		defer delete(visited, n.Name)

		// Look up the template in the full template set (not just routes)
		calledTemplate := ts.Lookup(n.Name)
		if calledTemplate != nil && calledTemplate.Tree != nil {
			if canTemplateRedirect(calledTemplate.Tree.Root, ts, templateMap, defs, visited) {
				return true
			}
		}

	case *parse.PipeNode:
		if n != nil {
			for _, cmd := range n.Cmds {
				if containsRedirectCall(cmd) {
					return true
				}
				if callsMethodOnTemplateData(cmd) {
					return true
				}
			}
		}
	}

	return false
}

// containsRedirectCall checks if a command node contains a call to .Redirect
func containsRedirectCall(cmd *parse.CommandNode) bool {
	if cmd == nil || len(cmd.Args) == 0 {
		return false
	}

	for _, arg := range cmd.Args {
		if field, ok := arg.(*parse.FieldNode); ok {
			// Check if this is a .Redirect call
			if len(field.Ident) > 0 && field.Ident[len(field.Ident)-1] == "Redirect" {
				return true
			}
			// Also check if any part of the chain is Redirect
			for _, ident := range field.Ident {
				if ident == "Redirect" {
					return true
				}
			}
		}
		// Check for chain nodes like .field.Redirect or (.Redirect ...).Header
		if chain, ok := arg.(*parse.ChainNode); ok {
			// Check if any field in the chain is Redirect
			for _, field := range chain.Field {
				if field == "Redirect" {
					return true
				}
			}
			// Also recursively check the Node that the chain starts from
			if chainNode, ok := chain.Node.(*parse.PipeNode); ok {
				for _, chainCmd := range chainNode.Cmds {
					if containsRedirectCall(chainCmd) {
						return true
					}
				}
			}
		}
	}
	return false
}

func callsMethodOnTemplateData(cmd *parse.CommandNode) bool {
	if cmd == nil || len(cmd.Args) == 0 {
		return false
	}
	firstArg := cmd.Args[0]
	if _, ok := firstArg.(*parse.IdentifierNode); ok {
		if len(cmd.Args) > 1 {
			// This is a function call with arguments
			// Check if any argument is bare TemplateData (.) or calls unsafe methods
			for i := 1; i < len(cmd.Args); i++ {
				switch arg := cmd.Args[i].(type) {
				case *parse.DotNode:
					// Bare . is being passed - this is the full TemplateData
					// Be conservative: function might call methods on it
					return true
				case *parse.FieldNode:
					// Check if it's a safe method call
					if !isAllSafeMethods(arg.Ident) {
						return true
					}
				case *parse.ChainNode:
					// A chain is being passed, be conservative
					return true
				}
			}
		}
	}

	// Check for direct method calls on TemplateData (not passed to a function)
	for _, arg := range cmd.Args {
		if field, ok := arg.(*parse.FieldNode); ok {
			// Check if all methods in the chain are safe
			if !isAllSafeMethods(field.Ident) {
				return true
			}
		}
	}

	return false
}

// isAllSafeMethods checks if all identifiers in a field chain are safe methods
func isAllSafeMethods(idents []string) bool {
	if len(idents) == 0 {
		return true
	}
	// First identifier must be a safe TemplateData method
	if !isSafeTemplateDataMethod(idents[0]) {
		return false
	}
	// If there are more identifiers, we're chaining off the result
	// e.g. `.Request.Method` - this is safe if Request is safe
	// (subsequent fields/methods are on the returned type, not TemplateData)
	return true
}

// isSafeTemplateDataMethod returns true for TemplateData methods that definitely
// don't set redirectURL (i.e., don't call Redirect internally)
func isSafeTemplateDataMethod(methodName string) bool {
	safeMethodsSet := map[string]bool{
		"Path":        true, // returns TemplateRoutePaths
		"Result":      true, // returns T (the result type)
		"Request":     true, // returns *http.Request
		"Receiver":    true, // returns R (the receiver type)
		"Ok":          true, // returns bool
		"Err":         true, // returns error
		"MuxtVersion": true, // returns string
		"StatusCode":  true, // sets statusCode field, returns *TemplateData but doesn't set redirectURL
		"Header":      true, // sets response headers, returns *TemplateData but doesn't set redirectURL
	}
	return safeMethodsSet[methodName]
}
