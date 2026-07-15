package muxt

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"html/template"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/astgen"
)

type Argument struct {
	Identifier string
	Type       ArgumentType
	ParamType  types.Type

	template *template.Template

	// sig, args, and isMethod describe a nested call argument
	// (Type == ArgumentTypeCall): the nested call's signature, its own
	// hydrated arguments, and whether it resolves to a receiver method.
	sig      *types.Signature
	args     []Argument
	isMethod bool

	// callbackResult and callbackHasArg describe a validated render-callback
	// argument (Type == ArgumentTypeExecute): the template data type T the
	// callback receives and whether the callback takes that data argument
	// (func(T) error vs func() error, where T = struct{}).
	callbackResult types.Type
	callbackHasArg bool
}

// Signature returns the resolved signature of a nested call argument
// (Type == ArgumentTypeCall), or nil for a leaf argument.
func (a Argument) Signature() *types.Signature { return a.sig }

// IsMethod reports whether a nested call argument resolves to a receiver method
// (as opposed to a package-scope function).
func (a Argument) IsMethod() bool { return a.isMethod }

// Arguments returns the hydrated arguments of a nested call argument.
func (a Argument) Arguments() []Argument { return a.args }

// Template returns the template a render-callback argument (ArgumentTypeExecute)
// renders: the route template for the base execute callback, or the same-named
// template for an sse-prefixed callback (nil if that template does not exist).
func (a Argument) Template() *template.Template { return a.template }

// CallbackSignature returns a render-callback argument's function signature
// (from its parameter type), or nil if the parameter type is not a function.
func (a Argument) CallbackSignature() *types.Signature {
	if a.ParamType == nil {
		return nil
	}
	sig, _ := a.ParamType.Underlying().(*types.Signature)
	return sig
}

// CallbackResultType returns the template data type T a validated
// render-callback argument receives (struct{} for a func() error callback).
func (a Argument) CallbackResultType() types.Type { return a.callbackResult }

// CallbackHasArg reports whether a validated render-callback argument's
// callback takes the template data argument (func(T) error vs func() error).
func (a Argument) CallbackHasArg() bool { return a.callbackHasArg }

type ArgumentType int

const (
	ArgumentTypeUnknown ArgumentType = iota
	ArgumentTypeRequest
	ArgumentTypeResponse
	ArgumentTypeRequestContext
	ArgumentTypeRequestPathValue
	ArgumentTypeRequestForm
	ArgumentTypeRequestMultipartForm
	ArgumentTypeExecute
	ArgumentTypeSendMessage
	ArgumentTypeLastEventID
	ArgumentTypeRequestBodyJSON
	ArgumentTypeCall
)

// ResultShape classifies a handler method's results. It is resolved during
// ResolveCall so the generate package can emit the matching call statements
// without re-deriving the contract.
type ResultShape int

const (
	ResultShapeInvalid ResultShape = iota
	// ResultShapeNone is an sse handler method with no results: func(...)
	ResultShapeNone
	// ResultShapeData is func(...) T
	ResultShapeData
	// ResultShapeDataError is func(...) (T, error)
	ResultShapeDataError
	// ResultShapeDataOK is func(...) (T, bool)
	ResultShapeDataOK
	// ResultShapeError is func(...) error: required for methods receiving the
	// execute callback and permitted for sse handler methods.
	ResultShapeError
)

func ResolveCall(def *Definition, templatesPackage *types.Package, receiver *types.Named, pl []*packages.Package) error {
	if def.call == nil || def.fun == nil {
		return nil
	}
	sig, isMethod, args, err := resolveCall(def, def.call, templatesPackage, receiver, pl)
	if err != nil {
		return err
	}
	def.sig = sig
	def.isMethod = isMethod
	def.Arguments = args
	shape, err := classifyResultShape(def)
	if err != nil {
		return err
	}
	def.resultShape = shape
	return resolveCallbackShapes(def)
}

// resolveCallbackShapes validates each render-callback argument against the
// callback contract — func() error (T = struct{}) or func(T) error — and
// records T and whether the callback takes the data argument. On sse routes
// every callback argument is checked; on html routes only the base execute
// argument is (sse-prefixed callbacks are inert there).
func resolveCallbackShapes(def *Definition) error {
	errIface := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	for i := range def.Arguments {
		a := &def.Arguments[i]
		if a.Type != ArgumentTypeExecute {
			continue
		}
		if def.Representation != RepresentationSSE && a.Identifier != TemplateNameScopeIdentifierExecute {
			continue
		}
		callback := a.CallbackSignature()
		if callback == nil || callback.Results().Len() != 1 || !types.Implements(callback.Results().At(0).Type(), errIface) {
			if def.Representation == RepresentationSSE {
				return fmt.Errorf("execute parameter for %s must be a function", def.fun.Name)
			}
			return fmt.Errorf("execute argument for %s must be a func(...) error", def.fun.Name)
		}
		switch callback.Params().Len() {
		case 0:
			a.callbackResult = types.NewStruct(nil, nil)
			a.callbackHasArg = false
		case 1:
			a.callbackResult = callback.Params().At(0).Type()
			a.callbackHasArg = true
		default:
			if def.Representation == RepresentationSSE {
				return errors.New("sse callback must have zero or one parameter; wrap multiple values in a struct")
			}
			return errors.New("execute callback must have zero or one parameter; wrap multiple values in a struct")
		}
		if def.Representation == RepresentationSSE && a.template == nil {
			return fmt.Errorf("no template %q for sse argument %s", a.Identifier, a.Identifier)
		}
	}
	return nil
}

// classifyResultShape validates def's method results against its contract:
// sse methods return nothing or an error, methods receiving the execute
// callback return only error, and all other methods return a value plus an
// optional error or bool.
func classifyResultShape(def *Definition) (ResultShape, error) {
	results := def.sig.Results()
	errIface := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	if def.Representation == RepresentationSSE {
		switch {
		case results.Len() == 0:
			return ResultShapeNone, nil
		case results.Len() == 1 && types.Implements(results.At(0).Type(), errIface):
			return ResultShapeError, nil
		default:
			return ResultShapeInvalid, fmt.Errorf("method %s using the sse callback must return nothing or an error", def.fun.Name)
		}
	}
	if slices.ContainsFunc(def.Arguments, func(a Argument) bool {
		return a.Type == ArgumentTypeExecute && a.Identifier == TemplateNameScopeIdentifierExecute
	}) {
		if results.Len() != 1 || !types.Implements(results.At(0).Type(), errIface) {
			return ResultShapeInvalid, fmt.Errorf("method %s using the execute callback must return only error", def.fun.Name)
		}
		return ResultShapeError, nil
	}
	switch results.Len() {
	case 1:
		return ResultShapeData, nil
	case 2:
		last := results.At(1).Type()
		if types.Implements(last, errIface) {
			return ResultShapeDataError, nil
		}
		if basic, ok := last.(*types.Basic); ok && basic.Kind() == types.Bool {
			return ResultShapeDataOK, nil
		}
		return ResultShapeInvalid, errors.New("expected last result to be either an error or a bool")
	case 0:
		return ResultShapeInvalid, fmt.Errorf("method for pattern %q has no results it should have one or two", def.name)
	default:
		return ResultShapeInvalid, fmt.Errorf("method %s has no results it should have one or two", def.fun.Name)
	}
}

// checkNestedCallResultShape validates a nested call's results: one value,
// optionally followed by an error or bool.
func checkNestedCallResultShape(name string, sig *types.Signature) error {
	results := sig.Results()
	errIface := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	switch results.Len() {
	case 1:
		return nil
	case 2:
		last := results.At(1).Type()
		if types.Implements(last, errIface) {
			return nil
		}
		if basic, ok := last.(*types.Basic); ok && basic.Kind() == types.Bool {
			return nil
		}
		return errors.New("expected last result to be either an error or a bool")
	default:
		return fmt.Errorf("method %s has no results it should have one or two", name)
	}
}

// resolveCall resolves a single call expression (top-level or nested) against
// the receiver and templates package. It returns the call's signature, whether
// it is a receiver method (as opposed to a package-scope function), and the
// hydrated arguments mapping each call argument to its method/func parameter.
//
// When the call identifier is neither a receiver method nor a package-scope
// function, its signature is synthesized from the call scope and attached to
// the receiver so it appears in the generated RoutesReceiver interface.
func resolveCall(def *Definition, call *ast.CallExpr, templatesPackage *types.Package, receiver *types.Named, pl []*packages.Package) (*types.Signature, bool, []Argument, error) {
	fun, ok := call.Fun.(*ast.Ident)
	if !ok {
		return nil, false, nil, fmt.Errorf("expected call to be a function identifier")
	}
	isMethod := true
	object, _, _ := types.LookupFieldOrMethod(receiver, true, receiver.Obj().Pkg(), fun.Name)
	if object == nil {
		if m, ok := packageScopeFunc(templatesPackage, fun); ok {
			object = m
			isMethod = false
		} else {
			ms, err := synthesizeCallSignature(def, call, templatesPackage, receiver, pl)
			if err != nil {
				return nil, false, nil, err
			}
			fn := types.NewFunc(0, receiver.Obj().Pkg(), fun.Name, ms)
			receiver.AddMethod(fn)
			object = fn
		}
	}
	sig := object.Type().(*types.Signature)
	args := make([]Argument, 0, len(call.Args))
	qual := typeQualifier(receiver.Obj().Pkg())

	if paramCount := sig.Params().Len(); len(call.Args) != sig.Params().Len() {
		// An execute callback that cannot map to a func parameter gets its
		// contract error rather than the generic argument count mismatch.
		for i, a := range call.Args {
			id, ok := a.(*ast.Ident)
			if !ok || id.Name != TemplateNameScopeIdentifierExecute {
				continue
			}
			if i >= paramCount {
				return nil, false, nil, fmt.Errorf("execute argument for %s must be a func(...) error", fun.Name)
			}
			if _, ok := sig.Params().At(i).Type().Underlying().(*types.Signature); !ok {
				return nil, false, nil, fmt.Errorf("execute argument for %s must be a func(...) error", fun.Name)
			}
		}
		sigStr := fun.Name + strings.TrimPrefix(types.TypeString(sig, qual), "func")
		return nil, false, nil, fmt.Errorf("handler func %s expects %d arguments but call %s has %d", sigStr, paramCount, astgen.Format(call), len(call.Args))
	}

	for i, a := range call.Args {
		var paramType types.Type
		if i < sig.Params().Len() {
			paramType = sig.Params().At(i).Type()
		}
		switch argument := a.(type) {
		case *ast.Ident:
			if paramType == nil && !IsSSEArgument(argument.Name) {
				args = append(args, Argument{Identifier: argument.Name})
				continue
			}
			arg, err := newArgumentFromIdentifier(def, pl, argument, paramType, qual)
			if err != nil {
				return nil, false, nil, err
			}
			args = append(args, arg)
		case *ast.CallExpr:
			var name string
			if fun, ok := argument.Fun.(*ast.Ident); ok {
				name = fun.Name
			}
			if paramType == nil {
				args = append(args, Argument{Identifier: name})
				continue
			}
			nestedSig, nestedIsMethod, nestedArgs, err := resolveCall(def, argument, templatesPackage, receiver, pl)
			if err != nil {
				return nil, false, nil, err
			}
			if err := checkNestedCallResultShape(name, nestedSig); err != nil {
				return nil, false, nil, err
			}
			args = append(args, Argument{
				Identifier: name,
				Type:       ArgumentTypeCall,
				ParamType:  paramType,
				sig:        nestedSig,
				isMethod:   nestedIsMethod,
				args:       nestedArgs,
			})
		}
	}
	return sig, isMethod, args, nil
}

// synthesizeCallSignature builds a signature for a call whose method is not yet
// defined on the receiver, inferring each parameter type from the argument
// scope. Nested calls are resolved (so their own methods are synthesized too)
// but do not contribute a parameter, mirroring the pre-hydration generator.
func synthesizeCallSignature(def *Definition, call *ast.CallExpr, templatesPackage *types.Package, receiver *types.Named, pl []*packages.Package) (*types.Signature, error) {
	var params []*types.Var
	hasSSE := false
	for _, a := range call.Args {
		switch arg := a.(type) {
		case *ast.Ident:
			if arg.Name == TemplateNameScopeIdentifierExecute {
				return nil, fmt.Errorf("method %s using the execute callback must be defined on the receiver type", call.Fun.(*ast.Ident).Name)
			}
			if IsSSEArgument(arg.Name) {
				hasSSE = true
				params = append(params, types.NewVar(0, receiver.Obj().Pkg(), arg.Name, sseCallbackSignature()))
				continue
			}
			tp, ok := DefaultScopeType(pl, def, arg.Name)
			if !ok {
				return nil, fmt.Errorf("could not determine a type for %s", arg.Name)
			}
			params = append(params, types.NewVar(0, receiver.Obj().Pkg(), arg.Name, tp))
		case *ast.CallExpr:
			if _, _, _, err := resolveCall(def, arg, templatesPackage, receiver, pl); err != nil {
				return nil, err
			}
		}
	}
	results := types.NewTuple(types.NewVar(0, nil, "", types.Universe.Lookup("any").Type()))
	if hasSSE {
		results = types.NewTuple()
	}
	return types.NewSignatureType(types.NewVar(0, nil, "", receiver.Obj().Type()), nil, nil, types.NewTuple(params...), results, false), nil
}

func DefaultScopeType(pl []*packages.Package, def *Definition, argumentIdentifier string) (types.Type, bool) {
	stdlibType := func(pkgPath, name string, pointer bool) (types.Type, bool) {
		pkg, ok := findPackageTypes(pl, pkgPath)
		if !ok {
			return nil, false
		}
		t := pkg.Scope().Lookup(name).Type()
		if pointer {
			t = types.NewPointer(t)
		}
		return t, true
	}
	switch argumentIdentifier {
	case TemplateNameScopeIdentifierHTTPRequest:
		return stdlibType("net/http", "Request", true)
	case TemplateNameScopeIdentifierHTTPResponse:
		return stdlibType("net/http", "ResponseWriter", false)
	case TemplateNameScopeIdentifierContext:
		return stdlibType("context", "Context", false)
	case TemplateNameScopeIdentifierForm:
		return stdlibType("net/url", "Values", false)
	case TemplateNameScopeIdentifierMultipart:
		return stdlibType("mime/multipart", "Form", true)
	case TemplateNameScopeIdentifierLastEventID:
		return types.Universe.Lookup("string").Type(), true
	default:
		if slices.Contains(def.PathValueIdentifiers(), argumentIdentifier) {
			return types.Universe.Lookup("string").Type(), true
		}
		return nil, false
	}
}

func findPackageTypes(pl []*packages.Package, pkgPath string) (*types.Package, bool) {
	for _, pkg := range pl {
		if pkg.Types.Path() == pkgPath {
			return pkg.Types, true
		}
	}
	for _, pkg := range pl {
		if p, ok := searchImports(pkg.Types, pkgPath); ok {
			return p, true
		}
	}
	return nil, false
}

func searchImports(pt *types.Package, pkgPath string) (*types.Package, bool) {
	for _, pkg := range pt.Imports() {
		if pkg.Path() == pkgPath {
			return pkg, true
		}
	}
	for _, pkg := range pt.Imports() {
		if p, ok := searchImports(pkg, pkgPath); ok {
			return p, true
		}
	}
	return nil, false
}

// sseCallbackSignature is the func(any) error type synthesized for an sse
// argument when the receiver method is not already defined.
func sseCallbackSignature() *types.Signature {
	anyType := types.Universe.Lookup("any").Type()
	errType := types.Universe.Lookup("error").Type()
	return types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewVar(0, nil, "", anyType)),
		types.NewTuple(types.NewVar(0, nil, "", errType)),
		false)
}

// typeQualifier renders types the way they read in the receiver's package:
// types from that package are unqualified and all others use the package name
// (*http.Request, not *net/http.Request).
func typeQualifier(receiverPkg *types.Package) types.Qualifier {
	return func(p *types.Package) string {
		if p == receiverPkg {
			return ""
		}
		return p.Name()
	}
}

func newArgumentFromIdentifier(def *Definition, pl []*packages.Package, arg *ast.Ident, param types.Type, qual types.Qualifier) (Argument, error) {
	a := Argument{
		Identifier: arg.Name,
		ParamType:  param,
	}
	switch arg.Name {
	case TemplateNameScopeIdentifierContext:
		a.Type = ArgumentTypeRequestContext
		if err := isAssignable(pl, param, arg.Name, "context", "Context", false, qual); err != nil {
			return a, err
		}
	case TemplateNameScopeIdentifierForm:
		a.Type = ArgumentTypeRequestForm
		if err := checkFormArgument(pl, param, arg.Name, "net/url", "Values", false, qual, false); err != nil {
			return a, err
		}
	case TemplateNameScopeIdentifierMultipart:
		a.Type = ArgumentTypeRequestMultipartForm
		if err := checkFormArgument(pl, param, arg.Name, "mime/multipart", "Form", true, qual, true); err != nil {
			return a, err
		}
	case TemplateNameScopeIdentifierHTTPRequest:
		a.Type = ArgumentTypeRequest
		if err := isAssignable(pl, param, arg.Name, "net/http", "Request", true, qual); err != nil {
			return a, err
		}
	case TemplateNameScopeIdentifierHTTPResponse:
		a.Type = ArgumentTypeResponse
		if err := isAssignable(pl, param, arg.Name, "net/http", "ResponseWriter", false, qual); err != nil {
			return a, err
		}
	case TemplateNameScopeIdentifierLastEventID:
		a.Type = ArgumentTypeLastEventID
		if err := checkParsedArgument(pl, param, qual); err != nil {
			return a, err
		}
	case TemplateNameScopeIdentifierExecute:
		a.Type = ArgumentTypeExecute
		a.template = def.template
	default:
		if slices.Contains(def.pathValueNames, arg.Name) {
			a.Type = ArgumentTypeRequestPathValue
			if err := checkParsedArgument(pl, param, qual); err != nil {
				return a, err
			}
			return a, nil
		}
		if IsSSEArgument(arg.Name) {
			// An sse-prefixed render callback (sseClock, sseMetrics, ...) renders
			// the same-named template. Template existence is validated in
			// resolveCallbackShapes once all arguments are hydrated.
			a.Type = ArgumentTypeExecute
			a.template = def.template.Lookup(arg.Name)
			return a, nil
		}
		if isSendMessage(def, arg) {
			a.Type = ArgumentTypeSendMessage

			t := def.template.Lookup(arg.Name)
			if t == nil {
				return Argument{}, fmt.Errorf("template %q was not found for execute argument", def.pathValueNames)
			}
			a.template = t

			return a, nil
		}
		return Argument{}, errors.New("unknown argument type")
	}
	return a, nil
}

func stdlibType(pl []*packages.Package, pkgPath, name string, pointer bool) (types.Type, error) {
	pkg, ok := findPackageTypes(pl, pkgPath)
	if !ok {
		return nil, fmt.Errorf("could not find package %q for %s", pkgPath, name)
	}
	t := pkg.Scope().Lookup(name).Type()
	if pointer {
		t = types.NewPointer(t)
	}
	return t, nil
}

func isAssignable(pl []*packages.Package, paramType types.Type, argName, packagePath, identifier string, pointer bool, qual types.Qualifier) error {
	at, err := stdlibType(pl, packagePath, identifier, pointer)
	if err != nil {
		return err
	}
	if !types.AssignableTo(at, paramType) {
		return fmt.Errorf("method expects type %s but %s is %s", types.TypeString(paramType, qual), argName, types.TypeString(at, qual))
	}
	return nil
}

func isSendMessage(def *Definition, arg *ast.Ident) bool {
	return def.Representation == RepresentationSSE && strings.HasSuffix(arg.Name, "Message") && token.IsIdentifier(arg.Name)
}

func packageScopeFunc(pkg *types.Package, fun *ast.Ident) (types.Object, bool) {
	obj := pkg.Scope().Lookup(fun.Name)
	if obj == nil {
		return nil, false
	}
	sig, ok := obj.Type().(*types.Signature)
	if !ok {
		return nil, false
	}
	if sig.Recv() != nil {
		return nil, false
	}
	return obj, true
}

const (
	TemplateNameScopeIdentifierContext      = "ctx"
	TemplateNameScopeIdentifierForm         = "form"
	TemplateNameScopeIdentifierMultipart    = "multipart"
	TemplateNameScopeIdentifierHTTPRequest  = "request"
	TemplateNameScopeIdentifierHTTPResponse = "response"
	TemplateNameScopeIdentifierExecute      = "execute"
	TemplateNameScopeIdentifierLastEventID  = "lastEventID"
)

func patternScope() []string {
	return []string{
		TemplateNameScopeIdentifierHTTPRequest,
		TemplateNameScopeIdentifierHTTPResponse,
		TemplateNameScopeIdentifierContext,
		TemplateNameScopeIdentifierForm,
		TemplateNameScopeIdentifierMultipart,
		TemplateNameScopeIdentifierExecute,
		TemplateNameScopeIdentifierLastEventID,
	}
}
