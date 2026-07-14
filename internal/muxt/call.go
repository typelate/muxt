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
	return nil
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
		if err := isAssignableOrStruct(pl, param, arg.Name, "net/url", "Values", false); err != nil {
			return a, err
		}
	case TemplateNameScopeIdentifierMultipart:
		a.Type = ArgumentTypeRequestMultipartForm
		if err := isAssignableOrStruct(pl, param, arg.Name, "mime/multipart", "Form", true); err != nil {
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
	case TemplateNameScopeIdentifierExecute:
		a.Type = ArgumentTypeExecute
		a.template = def.template
	default:
		if slices.Contains(def.pathValueNames, arg.Name) {
			a.Type = ArgumentTypeRequestPathValue
			return a, nil
		}
		if IsSSEArgument(arg.Name) {
			// An sse-prefixed render callback (sseClock, sseMetrics, ...) renders
			// the same-named template. Template existence is validated during
			// generation, so a missing template is not an error here.
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

// isAssignableOrStruct permits a form or multipart parameter to either receive
// the raw request value (url.Values / *multipart.Form) or be a struct whose
// fields are parsed from the submitted form.
func isAssignableOrStruct(pl []*packages.Package, paramType types.Type, argName, packagePath, identifier string, pointer bool) error {
	at, err := stdlibType(pl, packagePath, identifier, pointer)
	if err != nil {
		return err
	}
	if types.AssignableTo(at, paramType) {
		return nil
	}
	if _, ok := paramType.Underlying().(*types.Struct); ok {
		return nil
	}
	return fmt.Errorf("expected %s parameter type to be a struct", argName)
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
