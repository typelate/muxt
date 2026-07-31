package muxt

import (
	"errors"
	"fmt"
	"go/ast"
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

	// callbackHasOptions reports a send-family callback declaring the
	// trailing options variadic (func(T, opts ...SSEEventOption) error).
	callbackHasOptions bool

	// formFields describes how each struct field of a form or multipart
	// argument binds to the request (nil for the raw url.Values /
	// *multipart.Form mode).
	formFields []FieldBinding
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

// CallbackHasOptions reports whether a send-family callback declares the
// trailing per-event options variadic.
func (a Argument) CallbackHasOptions() bool { return a.callbackHasOptions }

// FormFields returns the field bindings of a form or multipart argument in
// struct mode, or nil when the parameter receives the raw request value.
func (a Argument) FormFields() []FieldBinding { return a.formFields }

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
	// ArgumentTypeSendJSON is a marshalJSON-wrapped send callback on an sse
	// route: the callback marshals its argument as the event's JSON data
	// instead of rendering a template.
	ArgumentTypeSendJSON
	ArgumentTypeLastEventID
	ArgumentTypeRequestBody
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
	// ResultShapeSSEChan is an sse method returning a receivable channel;
	// each received value renders one event until the channel closes.
	ResultShapeSSEChan
	// ResultShapeSSESeq is an sse method returning iter.Seq[T]; each yielded
	// value renders one event.
	ResultShapeSSESeq
	// ResultShapeSSESeq2 is an sse method returning iter.Seq2[T, error]; a
	// non-nil yielded error is placed on the event data's error list and the
	// event still renders.
	ResultShapeSSESeq2
)

// StreamResultShape reports whether tp is a supported SSE stream result type
// and returns the stream's element type: a receivable channel's element, an
// iter.Seq[T]'s T, or an iter.Seq2[T, error]'s T.
func StreamResultShape(tp types.Type) (ResultShape, types.Type, bool) {
	switch t := tp.(type) {
	case *types.Chan:
		if t.Dir() == types.SendOnly {
			return ResultShapeInvalid, nil, false
		}
		return ResultShapeSSEChan, t.Elem(), true
	case *types.Named:
		obj := t.Obj()
		if obj.Pkg() == nil || obj.Pkg().Path() != "iter" {
			return ResultShapeInvalid, nil, false
		}
		args := t.TypeArgs()
		switch obj.Name() {
		case "Seq":
			if args.Len() == 1 {
				return ResultShapeSSESeq, args.At(0), true
			}
		case "Seq2":
			if args.Len() == 2 {
				errIface := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
				if types.Implements(args.At(1), errIface) {
					return ResultShapeSSESeq2, args.At(0), true
				}
			}
		}
	}
	return ResultShapeInvalid, nil, false
}

// ResolveOptions carries the generation settings argument resolution and
// synthesis depend on.
type ResolveOptions struct {
	// JSONV2 selects encoding/json/v2 for generated JSON code paths.
	JSONV2 bool
	// SSEEventOptionType is the (possibly flag-renamed) generated per-event
	// option type send-family callbacks may declare a variadic of.
	SSEEventOptionType string
}

func ResolveCall(def *Definition, templatesPackage *types.Package, receiver *types.Named, pl []*packages.Package, opts ResolveOptions) error {
	if def.call == nil || def.fun == nil {
		return nil
	}
	sig, isMethod, args, err := resolveCall(def, def.call, templatesPackage, receiver, pl, opts)
	if err != nil {
		return err
	}
	def.sig = sanitizeOptionVariadics(sig, templatesPackage, opts.SSEEventOptionType)
	def.isMethod = isMethod
	def.Arguments = args
	shape, err := classifyResultShape(def, typeQualifier(receiver.Obj().Pkg()))
	if err != nil {
		return err
	}
	def.resultShape = shape
	return resolveCallbackShapes(def, templatesPackage, opts)
}

// resolveCallbackShapes validates each render-callback argument against the
// callback contract — func() error (T = struct{}) or func(T) error — and
// records T and whether the callback takes the data argument. On sse routes
// every callback argument is checked; on html routes only the base execute
// argument is (sse-prefixed callbacks are inert there).
func resolveCallbackShapes(def *Definition, templatesPackage *types.Package, opts ResolveOptions) error {
	errIface := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	for i := range def.Arguments {
		a := &def.Arguments[i]
		if a.Type != ArgumentTypeExecute && a.Type != ArgumentTypeSendJSON {
			continue
		}
		if def.Representation != RepresentationSSE && a.Identifier != TemplateNameScopeIdentifierExecute {
			continue
		}
		callback := a.CallbackSignature()
		if callback == nil || callback.Results().Len() != 1 || !types.Implements(callback.Results().At(0).Type(), errIface) {
			if def.Representation == RepresentationSSE {
				return fmt.Errorf("%s callback for %s must be a function", a.Identifier, def.fun.Name)
			}
			return fmt.Errorf("execute argument for %s must be a func(...) error", def.fun.Name)
		}
		params, dataParams := callback.Params(), callback.Params().Len()
		if callback.Variadic() {
			if def.Representation != RepresentationSSE {
				return fmt.Errorf("execute callback for %s takes no options; its shape is func(T) error or func() error", def.fun.Name)
			}
			elem := params.At(params.Len() - 1).Type().(*types.Slice).Elem()
			if err := checkSendOptionType(elem, templatesPackage, opts.SSEEventOptionType, typeQualifier(templatesPackage)); err != nil {
				return err
			}
			a.callbackHasOptions = true
			dataParams--
		}
		switch dataParams {
		case 0:
			a.callbackResult = types.NewStruct(nil, nil)
			a.callbackHasArg = false
		case 1:
			a.callbackResult = params.At(0).Type()
			a.callbackHasArg = true
		default:
			if def.Representation == RepresentationSSE {
				return errors.New("send callback must have zero or one parameter; wrap multiple values in a struct")
			}
			return errors.New("execute callback must have zero or one parameter; wrap multiple values in a struct")
		}
		if def.Representation == RepresentationSSE && a.Type == ArgumentTypeExecute && a.template == nil {
			return fmt.Errorf("no template %q for sse send callback %s", strings.TrimPrefix(a.Identifier, TemplateNameScopeIdentifierSend), a.Identifier)
		}
	}
	return nil
}

// checkSendOptionType verifies a send callback's variadic element is the
// generated per-event option type from the routes package.
func checkSendOptionType(elem types.Type, templatesPackage *types.Package, optionTypeName string, qual types.Qualifier) error {
	if basic, ok := elem.(*types.Basic); ok && basic.Kind() == types.Invalid {
		// The referenced type does not resolve yet — the option type is about
		// to be generated (first run). The Go build after generation enforces
		// that the name matches.
		return nil
	}
	if named, ok := elem.(*types.Named); ok {
		obj := named.Obj()
		if obj.Name() == optionTypeName && obj.Pkg() != nil && obj.Pkg().Path() == templatesPackage.Path() {
			return nil
		}
	}
	return fmt.Errorf("send callback options variadic must have element type %s, got %s", optionTypeName, types.TypeString(elem, qual))
}

// classifyResultShape validates def's method results against its contract:
// sse methods return nothing or an error, methods receiving the execute
// callback return only error, and all other methods return a value plus an
// optional error or bool.
func classifyResultShape(def *Definition, qual types.Qualifier) (ResultShape, error) {
	results := def.sig.Results()
	errIface := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	if def.Representation == RepresentationMarshalJSON {
		// The wrapped method must produce exactly one non-error value to
		// marshal, optionally followed by an error.
		sigStr := def.fun.Name + strings.TrimPrefix(types.TypeString(def.sig, qual), "func")
		switch results.Len() {
		case 0:
			return ResultShapeInvalid, fmt.Errorf("marshalJSON requires a result to marshal but %s returns nothing", sigStr)
		case 1:
			if types.Implements(results.At(0).Type(), errIface) {
				return ResultShapeInvalid, fmt.Errorf("marshalJSON requires a non-error result but %s only returns an error", sigStr)
			}
			return ResultShapeData, nil
		case 2:
			if last := results.At(1).Type(); !types.Implements(last, errIface) {
				return ResultShapeInvalid, fmt.Errorf("marshalJSON requires the second result of %s to be an error, got %s", sigStr, types.TypeString(last, qual))
			}
			return ResultShapeDataError, nil
		default:
			return ResultShapeInvalid, fmt.Errorf("marshalJSON allows at most two results but %s has %d", sigStr, results.Len())
		}
	}
	if def.Representation == RepresentationSSE {
		if results.Len() == 1 {
			if shape, _, ok := StreamResultShape(results.At(0).Type()); ok {
				if slices.ContainsFunc(def.Arguments, func(a Argument) bool { return a.Type == ArgumentTypeExecute || a.Type == ArgumentTypeSendJSON }) {
					return ResultShapeInvalid, fmt.Errorf("method %s mixes send callbacks with a stream result; use send callbacks or return a stream, not both", def.fun.Name)
				}
				return shape, nil
			}
		}
		switch {
		case results.Len() == 0:
			return ResultShapeNone, nil
		case results.Len() == 1 && types.Implements(results.At(0).Type(), errIface):
			return ResultShapeError, nil
		default:
			return ResultShapeInvalid, fmt.Errorf("method %s using the sse callback must return nothing, an error, or a stream (<-chan T, iter.Seq[T], or iter.Seq2[T, error])", def.fun.Name)
		}
	}
	if results.Len() >= 1 {
		if _, _, ok := StreamResultShape(results.At(0).Type()); ok {
			return ResultShapeInvalid, fmt.Errorf("method %s returns a stream; wrap the call in sse(...) to stream it as server-sent events", def.fun.Name)
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
		return ResultShapeInvalid, fmt.Errorf("method %s has %d results it should have one or two", def.fun.Name, results.Len())
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
	case 0:
		return fmt.Errorf("method %s has no results it should have one or two", name)
	default:
		return fmt.Errorf("method %s has %d results it should have one or two", name, results.Len())
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
func resolveCall(def *Definition, call *ast.CallExpr, templatesPackage *types.Package, receiver *types.Named, pl []*packages.Package, opts ResolveOptions) (*types.Signature, bool, []Argument, error) {
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
			ms, err := synthesizeCallSignature(def, call, templatesPackage, receiver, pl, opts)
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
			if paramType == nil && !IsSendArgument(argument.Name) {
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
			if def.Representation == RepresentationSSE && name == string(RepresentationMarshalJSON) {
				inner, ok := singleSendArgument(argument)
				if !ok {
					return nil, false, nil, fmt.Errorf("marshalJSON inside sse(...) must wrap a single send callback, for example marshalJSON(sendStatus)")
				}
				args = append(args, Argument{
					Identifier: inner.Name,
					Type:       ArgumentTypeSendJSON,
					ParamType:  paramType,
				})
				continue
			}
			if name == callWrapperUnmarshalJSON {
				// The decode target is the method parameter's type; any type
				// encoding/json can unmarshal into is permitted, so there is
				// no assignability constraint to check here.
				args = append(args, Argument{
					Identifier: TemplateNameScopeIdentifierRequestBody,
					Type:       ArgumentTypeRequestBodyJSON,
					ParamType:  paramType,
				})
				continue
			}
			nestedSig, nestedIsMethod, nestedArgs, err := resolveCall(def, argument, templatesPackage, receiver, pl, opts)
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
func synthesizeCallSignature(def *Definition, call *ast.CallExpr, templatesPackage *types.Package, receiver *types.Named, pl []*packages.Package, opts ResolveOptions) (*types.Signature, error) {
	var params []*types.Var
	hasSSE := false
	for _, a := range call.Args {
		switch arg := a.(type) {
		case *ast.Ident:
			if arg.Name == TemplateNameScopeIdentifierExecute {
				return nil, fmt.Errorf("method %s using the execute callback must be defined on the receiver type", call.Fun.(*ast.Ident).Name)
			}
			if IsSendArgument(arg.Name) {
				hasSSE = true
				params = append(params, types.NewVar(0, receiver.Obj().Pkg(), arg.Name, sseCallbackSignature(templatesPackage, opts.SSEEventOptionType)))
				continue
			}
			tp, ok := DefaultScopeType(pl, def, arg.Name)
			if !ok {
				return nil, fmt.Errorf("could not determine a type for %s", arg.Name)
			}
			params = append(params, types.NewVar(0, receiver.Obj().Pkg(), arg.Name, tp))
		case *ast.CallExpr:
			if fn, ok := arg.Fun.(*ast.Ident); ok && def.Representation == RepresentationSSE && fn.Name == string(RepresentationMarshalJSON) {
				if inner, ok := singleSendArgument(arg); ok {
					hasSSE = true
					params = append(params, types.NewVar(0, receiver.Obj().Pkg(), inner.Name, sseCallbackSignature(templatesPackage, opts.SSEEventOptionType)))
					continue
				}
			}
			if fn, ok := arg.Fun.(*ast.Ident); ok && fn.Name == callWrapperUnmarshalJSON {
				// Template-first iteration: without a defined method the decode
				// target is unknown, so pass the raw payload through.
				pkgPath, typeName, pointer := "encoding/json", "RawMessage", false
				if opts.JSONV2 {
					pkgPath, typeName, pointer = "encoding/json/jsontext", "Decoder", true
				}
				tp, err := stdlibType(pl, pkgPath, typeName, pointer)
				if err != nil {
					return nil, err
				}
				params = append(params, types.NewVar(0, receiver.Obj().Pkg(), TemplateNameScopeIdentifierRequestBody, tp))
				continue
			}
			if _, _, _, err := resolveCall(def, arg, templatesPackage, receiver, pl, opts); err != nil {
				return nil, err
			}
		}
	}
	results := types.NewTuple(types.NewVar(0, nil, "", types.Universe.Lookup("any").Type()))
	if hasSSE || def.Representation == RepresentationSSE {
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
	case TemplateNameScopeIdentifierRequestBody:
		return stdlibType("io", "Reader", false)
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

// sseCallbackSignature is the func(any, opts ...SSEEventOption) error type
// synthesized for a send callback when the receiver method is not already
// defined, exposing the full per-event options surface for template-first
// iteration.
func sseCallbackSignature(templatesPackage *types.Package, optionTypeName string) *types.Signature {
	anyType := types.Universe.Lookup("any").Type()
	errType := types.Universe.Lookup("error").Type()
	// Both parameters are named: mixing named and unnamed parameters would
	// print an invalid Go func type into the receiver interface.
	return types.NewSignatureType(nil, nil, nil,
		types.NewTuple(
			types.NewVar(0, nil, "result", anyType),
			types.NewVar(0, nil, "opts", types.NewSlice(sseEventOptionType(templatesPackage, optionTypeName))),
		),
		types.NewTuple(types.NewVar(0, nil, "", errType)),
		true)
}

// sseEventOptionType resolves the generated per-event option type in the
// routes package, or synthesizes a placeholder named type when generation has
// not produced it yet (first run) so synthesized signatures print its name.
func sseEventOptionType(templatesPackage *types.Package, optionTypeName string) types.Type {
	if obj := templatesPackage.Scope().Lookup(optionTypeName); obj != nil {
		if tn, ok := obj.(*types.TypeName); ok {
			return tn.Type()
		}
	}
	tn := types.NewTypeName(0, templatesPackage, optionTypeName, nil)
	return types.NewNamed(tn, types.NewSignatureType(nil, nil, nil, nil, nil, false), nil)
}

// sanitizeOptionVariadics rewrites callback parameters whose options-variadic
// element failed to resolve — the option type is about to be generated on the
// first run — so the printed receiver interface names the generated type
// instead of an invalid type.
func sanitizeOptionVariadics(sig *types.Signature, templatesPackage *types.Package, optionTypeName string) *types.Signature {
	params := sig.Params()
	vars := make([]*types.Var, params.Len())
	changed := false
	for i := range params.Len() {
		p := params.At(i)
		vars[i] = p
		cb, ok := p.Type().Underlying().(*types.Signature)
		if !ok || !cb.Variadic() || cb.Params().Len() == 0 {
			continue
		}
		last := cb.Params().At(cb.Params().Len() - 1)
		slice, ok := last.Type().(*types.Slice)
		if !ok {
			continue
		}
		basic, ok := slice.Elem().(*types.Basic)
		if !ok || basic.Kind() != types.Invalid {
			continue
		}
		cbParams := make([]*types.Var, cb.Params().Len())
		for j := range cb.Params().Len() {
			cbParams[j] = cb.Params().At(j)
		}
		cbParams[len(cbParams)-1] = types.NewVar(0, last.Pkg(), last.Name(), types.NewSlice(sseEventOptionType(templatesPackage, optionTypeName)))
		vars[i] = types.NewVar(0, p.Pkg(), p.Name(), types.NewSignatureType(nil, nil, nil, types.NewTuple(cbParams...), cb.Results(), true))
		changed = true
	}
	if !changed {
		return sig
	}
	return types.NewSignatureType(sig.Recv(), nil, nil, types.NewTuple(vars...), sig.Results(), sig.Variadic())
}

// singleSendArgument returns the send-callback identifier when call has
// exactly one argument and it is a send-family name.
func singleSendArgument(call *ast.CallExpr) (*ast.Ident, bool) {
	if len(call.Args) != 1 {
		return nil, false
	}
	inner, ok := call.Args[0].(*ast.Ident)
	if !ok || !IsSendArgument(inner.Name) {
		return nil, false
	}
	return inner, true
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
		bindings, err := checkFormArgument(def, pl, param, arg.Name, "net/url", "Values", false, qual, false)
		if err != nil {
			return a, err
		}
		a.formFields = bindings
	case TemplateNameScopeIdentifierMultipart:
		a.Type = ArgumentTypeRequestMultipartForm
		bindings, err := checkFormArgument(def, pl, param, arg.Name, "mime/multipart", "Form", true, qual, true)
		if err != nil {
			return a, err
		}
		a.formFields = bindings
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
	case TemplateNameScopeIdentifierRequestBody:
		a.Type = ArgumentTypeRequestBody
		// The request body is a single-use stream; the parameter must be
		// exactly io.Reader so the method cannot assume more than one read.
		readerType, err := stdlibType(pl, "io", "Reader", false)
		if err != nil {
			return a, err
		}
		if !types.Identical(param, readerType) {
			return a, fmt.Errorf("%s parameter must have type io.Reader, got %s", arg.Name, types.TypeString(param, qual))
		}
	default:
		if slices.Contains(def.pathValueNames, arg.Name) {
			a.Type = ArgumentTypeRequestPathValue
			if err := checkParsedArgument(pl, param, qual); err != nil {
				return a, err
			}
			return a, nil
		}
		if IsSendArgument(arg.Name) {
			// The base send callback renders the route's define body; a
			// send-prefixed callback (sendClock, sendMetrics, ...) renders the
			// template named by its suffix. Template existence is validated in
			// resolveCallbackShapes once all arguments are hydrated.
			a.Type = ArgumentTypeExecute
			if arg.Name == TemplateNameScopeIdentifierSend {
				a.template = def.template
			} else {
				a.template = def.template.Lookup(strings.TrimPrefix(arg.Name, TemplateNameScopeIdentifierSend))
			}
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
	TemplateNameScopeIdentifierRequestBody  = "body"
	TemplateNameScopeIdentifierSend         = "send"
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
		TemplateNameScopeIdentifierRequestBody,
	}
}
