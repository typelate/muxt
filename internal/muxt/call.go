package muxt

import (
	"fmt"
	"go/ast"
	"go/types"
	"html/template"

	"github.com/typelate/muxt/internal/asteval"
	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/source"
)

// ResolveCall resolves def's call against the receiver type and the package's
// functions, asking checker what it needs to know about the standard library.
func ResolveCall(def *Definition, pkg source.Package, receiver *types.Named, checker Checker) error {
	if def.call == nil {
		return nil
	}
	sig, isMethod, args, err := resolveCall(def, def.call, pkg, receiver, checker)
	if err != nil {
		return def.finishNameError(err, def.handlerSpan())
	}
	def.sig = sig
	def.isMethod = isMethod
	def.Arguments = args
	if err := linkArguments(def, typeQualifier(receiver.Obj().Pkg())); err != nil {
		return def.finishNameError(err, def.handlerSpan())
	}
	shape, err := classifyResultShape(def, typeQualifier(receiver.Obj().Pkg()))
	if err != nil {
		// Result-shape errors are about the method contract, so the
		// marker points at the method identifier.
		return def.finishNameError(errAtNode(def.fun, err), def.handlerSpan())
	}
	def.resultShape = shape
	if err := resolveCallbackShapes(def); err != nil {
		return def.finishNameError(err, def.handlerSpan())
	}
	def.resultStatusCode = statusCodeSource(def.resultType(), pkg.Types)
	return nil
}

// definedHere returns a "file:line:col: name is defined here" note for
// object, or "" when its source position is unknown (synthesized
// methods, for instance, have no position).
func definedHere(pkg source.Package, object types.Object) string {
	if object == nil || !object.Pos().IsValid() || pkg.Fset == nil {
		return ""
	}
	position := pkg.Fset.Position(object.Pos())
	if position.Filename == "" {
		return ""
	}
	return fmt.Sprintf("%s: %s is defined here", position, object.Name())
}

// resolveCall resolves a single call expression (top-level or nested) against
// the receiver and templates package. It returns the call's signature, whether
// it is a receiver method (as opposed to a package-scope function), and the
// hydrated arguments mapping each call argument to its method/func parameter.
//
// When the call identifier is neither a receiver method nor a package-scope
// function, its signature is synthesized from the call scope and attached to
// the receiver so it appears in the generated RoutesReceiver interface.
func resolveCall(def *Definition, call *ast.CallExpr, pkg source.Package, receiver *types.Named, checker Checker) (*types.Signature, bool, []Argument, error) {
	fun, ok := call.Fun.(*ast.Ident)
	if !ok {
		return nil, false, nil, errAt(call.Fun, "expected a function identifier, got: %s", astgen.Format(call.Fun))
	}
	object, isMethod, err := lookupCallee(def, call, fun, pkg, receiver, checker)
	if err != nil {
		return nil, false, nil, err
	}
	if call == def.call {
		if note := definedHere(pkg, object); note != "" {
			def.related = append(def.related, note)
		}
	}
	sig := object.Type().(*types.Signature)
	qual := typeQualifier(receiver.Obj().Pkg())
	if len(call.Args) != sig.Params().Len() {
		return nil, false, nil, argumentCountError(call, fun, sig, qual)
	}
	args, err := hydrateArguments(def, call, sig, pkg, receiver, checker)
	if err != nil {
		return nil, false, nil, err
	}
	return sig, isMethod, args, nil
}

// lookupCallee finds what fun names: a receiver method, else a package-scope
// function, else a method synthesized from the call's arguments.
func lookupCallee(def *Definition, call *ast.CallExpr, fun *ast.Ident, pkg source.Package, receiver *types.Named, checker Checker) (types.Object, bool, error) {
	if object, _, _ := types.LookupFieldOrMethod(receiver, true, receiver.Obj().Pkg(), fun.Name); object != nil {
		return object, true, nil
	}
	if function, ok := packageScopeFunc(pkg.Types, fun); ok {
		return function, false, nil
	}
	sig, err := synthesizeCallSignature(def, call, pkg, receiver, checker)
	if err != nil {
		return nil, false, err
	}
	method := types.NewFunc(0, receiver.Obj().Pkg(), fun.Name, sig)
	receiver.AddMethod(method)
	def.synthesizedMethods = append(def.synthesizedMethods, signatureString(fun.Name, sig, typeQualifier(receiver.Obj().Pkg())))
	return method, true, nil
}

// argumentCountError reports a call whose arguments do not match sig's
// parameters. An execute callback that cannot map to a func parameter gets its
// contract error rather than the generic argument count mismatch.
func argumentCountError(call *ast.CallExpr, fun *ast.Ident, sig *types.Signature, qual types.Qualifier) error {
	for i, a := range call.Args {
		id, ok := a.(*ast.Ident)
		if !ok || id.Name != TemplateNameScopeIdentifierExecute {
			continue
		}
		if i >= sig.Params().Len() {
			return errAt(id, "execute argument for %s must be a func(...) error", fun.Name)
		}
		if _, ok := sig.Params().At(i).Type().Underlying().(*types.Signature); !ok {
			return errAt(id, "execute argument for %s must be a func(...) error", fun.Name)
		}
	}
	return fmt.Errorf("handler func %s expects %d arguments but call %s has %d", signatureString(fun.Name, sig, qual), sig.Params().Len(), astgen.Format(call), len(call.Args))
}

// hydrateArguments maps each argument of call, which has as many arguments as
// sig has parameters, to the parameter it binds to.
func hydrateArguments(def *Definition, call *ast.CallExpr, sig *types.Signature, pkg source.Package, receiver *types.Named, checker Checker) ([]Argument, error) {
	qual := typeQualifier(receiver.Obj().Pkg())
	args := make([]Argument, 0, len(call.Args))
	for i, a := range call.Args {
		paramType := sig.Params().At(i).Type()
		switch argument := a.(type) {
		case *ast.Ident:
			arg, err := newArgumentFromIdentifier(def, checker, argument, paramType, qual)
			if err != nil {
				return nil, errAtNode(argument, err)
			}
			args = append(args, arg)
		case *ast.CallExpr:
			arg, err := hydrateCallArgument(def, argument, paramType, pkg, receiver, checker)
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
		}
	}
	return args, nil
}

func hydrateCallArgument(def *Definition, call *ast.CallExpr, paramType types.Type, pkg source.Package, receiver *types.Named, checker Checker) (Argument, error) {
	var name string
	if fun, ok := call.Fun.(*ast.Ident); ok {
		name = fun.Name
	}
	if name == callWrapperUnmarshalJSON {
		// The decode target is the method parameter's type; any type
		// encoding/json can unmarshal into is permitted, so there is
		// no assignability constraint to check here.
		return Argument{
			Identifier: TemplateNameScopeIdentifierRequestBody,
			Type:       ArgumentTypeRequestBodyJSON,
			paramType:  paramType,
		}, nil
	}
	sig, isMethod, args, err := resolveCall(def, call, pkg, receiver, checker)
	if err != nil {
		return Argument{}, err
	}
	shape, err := classifyNestedCallResultShape(name, sig, typeQualifier(receiver.Obj().Pkg()))
	if err != nil {
		return Argument{}, errAtNode(call.Fun, err)
	}
	return Argument{
		Identifier:  name,
		Type:        ArgumentTypeCall,
		paramType:   paramType,
		sig:         sig,
		isMethod:    isMethod,
		args:        args,
		resultShape: shape,
	}, nil
}

// synthesizeCallSignature builds a signature for a call whose method is not yet
// defined on the receiver, inferring each parameter type from the argument
// scope. Nested calls are resolved (so their own methods are synthesized too)
// but do not contribute a parameter, mirroring the pre-hydration generator.
func synthesizeCallSignature(def *Definition, call *ast.CallExpr, pkg source.Package, receiver *types.Named, checker Checker) (*types.Signature, error) {
	params := synthesizedParameters{
		callName: call.Fun.(*ast.Ident).Name,
		pkg:      receiver.Obj().Pkg(),
		seen:     make(map[string]bool),
	}
	for _, a := range call.Args {
		var err error
		switch arg := a.(type) {
		case *ast.Ident:
			err = params.addIdentifier(def, checker, arg)
		case *ast.CallExpr:
			err = params.addCall(def, arg, pkg, receiver, checker)
		}
		if err != nil {
			return nil, err
		}
	}
	return params.signature(receiver), nil
}

// synthesizedParameters collects the parameters of a synthesized method.
// Each argument becomes a parameter named after it, so a repeated argument
// would synthesize a method whose parameter names collide.
type synthesizedParameters struct {
	callName string
	pkg      *types.Package
	seen     map[string]bool
	vars     []*types.Var
	hasSSE   bool
}

func (p *synthesizedParameters) add(node ast.Node, name string, tp types.Type) error {
	if p.seen[name] {
		return errAt(node, "cannot infer a signature for %s: the %s argument is passed more than once; define the method on the receiver to use repeated arguments", p.callName, name)
	}
	p.seen[name] = true
	p.vars = append(p.vars, types.NewVar(0, p.pkg, name, tp))
	return nil
}

func (p *synthesizedParameters) addIdentifier(def *Definition, checker Checker, arg *ast.Ident) error {
	switch {
	case arg.Name == TemplateNameScopeIdentifierExecute:
		return errAt(arg, "method %s using the execute callback must be defined on the receiver type", p.callName)
	case isSSEArgument(arg.Name):
		p.hasSSE = true
		return p.add(arg, arg.Name, sseCallbackSignature())
	case def.isSignalsCallback(arg.Name):
		return p.add(arg, arg.Name, sseCallbackSignature())
	}
	tp, ok := defaultScopeType(checker, def, arg.Name)
	if !ok {
		return errAt(arg, "could not determine a type for %s", arg.Name)
	}
	return p.add(arg, arg.Name, tp)
}

func (p *synthesizedParameters) addCall(def *Definition, arg *ast.CallExpr, pkg source.Package, receiver *types.Named, checker Checker) error {
	if !isCallTo(arg, callWrapperUnmarshalJSON) {
		_, _, _, err := resolveCall(def, arg, pkg, receiver, checker)
		return err
	}
	// Template-first iteration: without a defined method the decode
	// target is unknown, so pass the raw payload through.
	tp, err := checker.RawJSON()
	if err != nil {
		return err
	}
	return p.add(arg, TemplateNameScopeIdentifierRequestBody, tp)
}

func (p *synthesizedParameters) signature(receiver *types.Named) *types.Signature {
	results := types.NewTuple(types.NewVar(0, nil, "", types.Universe.Lookup("any").Type()))
	if p.hasSSE {
		results = types.NewTuple()
	}
	return types.NewSignatureType(types.NewVar(0, nil, "", receiver.Obj().Type()), nil, nil, types.NewTuple(p.vars...), results, false)
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

// templateNames lists the names in ts's template set, candidates for
// suggesting a fix to a misspelled template name.
func templateNames(ts *template.Template) []string {
	if ts == nil {
		return nil
	}
	all := ts.Templates()
	names := make([]string, 0, len(all))
	for _, t := range all {
		names = append(names, t.Name())
	}
	return names
}

// ResolveDefinitions parses the route definitions of every templates variable in pkg
// and resolves each call against receiver, or against an empty struct named
// Receiver when there is none, so handler methods are inferred.
func ResolveDefinitions(pkg source.Package, receiver *types.Named, checker Checker) ([]Definition, error) {
	if receiver == nil {
		receiver = asteval.NamedEmptyStruct("Receiver", pkg.Types)
	}
	var (
		result []Definition
		errs   []error
	)
	for _, variable := range pkg.Variables {
		defs, err := Definitions(variable)
		if err != nil {
			return nil, err
		}
		for i := range defs {
			if err := ResolveCall(&defs[i], pkg, receiver, checker); err != nil {
				errs = append(errs, err)
			}
		}
		result = append(result, defs...)
	}
	return result, CombineErrors(errs)
}
