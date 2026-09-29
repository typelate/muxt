package muxt

import (
	"fmt"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/typelate/muxt/internal/astgen"
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

// ResultStatusCode is where a route's result offers a status code: a
// StatusCode() int method, a StatusCode field, or nowhere.
type ResultStatusCode int

const (
	ResultStatusCodeNone ResultStatusCode = iota
	ResultStatusCodeMethod
	ResultStatusCodeField
)

// resultType is the type the template data's Result field has: the
// execute callback's parameter when the call takes one, otherwise the
// call's first result.
func (def *Definition) resultType() types.Type {
	if i, ok := def.ExecuteArgumentIndex(); ok {
		return def.Arguments[i].callbackResult
	}
	switch def.resultShape {
	case ResultShapeData, ResultShapeDataError, ResultShapeDataOK:
		return def.sig.Results().At(0).Type()
	}
	return nil
}

func statusCodeSource(tp types.Type, pkg *types.Package) ResultStatusCode {
	if tp == nil {
		return ResultStatusCodeNone
	}
	if types.Implements(tp, statusCoder) {
		return ResultStatusCodeMethod
	}
	if obj, _, _ := types.LookupFieldOrMethod(tp, true, pkg, "StatusCode"); obj != nil {
		return ResultStatusCodeField
	}
	return ResultStatusCodeNone
}

var statusCoder = types.NewInterfaceType([]*types.Func{
	types.NewFunc(token.NoPos, nil, "StatusCode", types.NewSignatureType(nil, nil, nil,
		types.NewTuple(),
		types.NewTuple(types.NewVar(token.NoPos, nil, "", types.Typ[types.Int])),
		false,
	)),
}, nil).Complete()

// resolveCallbackShapes validates each render-callback argument against the
// callback contract — func() error (T = struct{}) or func(T) error — and
// records T and whether the callback takes the data argument. On sse routes
// every callback argument is checked; on html routes only the base execute
// argument is (sse-prefixed callbacks are inert there).
func resolveCallbackShapes(def *Definition) error {
	errIface := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	for i := range def.Arguments {
		a := &def.Arguments[i]
		if a.Type == ArgumentTypeSignalsCallback {
			// The generated closure has type func(T) error, so the result must
			// be exactly error — an error implementation would not compile at
			// the receiver call.
			callback := a.callbackSignature()
			if callback == nil || callback.Params().Len() != 1 || callback.Results().Len() != 1 || !types.Identical(callback.Results().At(0).Type(), types.Universe.Lookup("error").Type()) {
				return def.argErrorf(a.Identifier, "the %s signals callback must be a func(T) error; T is marshaled as the patch-signals payload", a.Identifier)
			}
			a.callbackResult = callback.Params().At(0).Type()
			a.callbackHasArg = true
			continue
		}
		if a.Type != ArgumentTypeExecute {
			continue
		}
		if def.Representation != RepresentationSSE && a.Identifier != TemplateNameScopeIdentifierExecute {
			continue
		}
		callback := a.callbackSignature()
		if callback == nil || callback.Results().Len() != 1 || !types.Implements(callback.Results().At(0).Type(), errIface) {
			if def.Representation == RepresentationSSE {
				return def.argErrorf(a.Identifier, "execute parameter for %s must be a function", def.fun.Name)
			}
			return def.argErrorf(a.Identifier, "execute argument for %s must be a func(...) error", def.fun.Name)
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
				return def.argErrorf(a.Identifier, "sse callback must have zero or one parameter; wrap multiple values in a struct")
			}
			return def.argErrorf(a.Identifier, "execute callback must have zero or one parameter; wrap multiple values in a struct")
		}
		if def.Representation == RepresentationSSE && a.template == nil {
			if suggestion, ok := astgen.NearestString(a.Identifier, templateNames(def.template)); ok {
				return def.argErrorf(a.Identifier, "no template %q for sse argument %s; did you mean %q?", a.Identifier, a.Identifier, suggestion)
			}
			return def.argErrorf(a.Identifier, "no template %q for sse argument %s", a.Identifier, a.Identifier)
		}
	}
	return nil
}

// classifyResultShape validates def's method results against its contract:
// marshalJSON methods return a value to marshal plus an optional error, sse
// methods return nothing or an error, methods receiving the execute callback
// return only error, and all other methods return a value plus an optional
// error or bool.
func classifyResultShape(def *Definition, qual types.Qualifier) (ResultShape, error) {
	results := def.sig.Results()
	errIface := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	sigStr := def.fun.Name + strings.TrimPrefix(types.TypeString(def.sig, qual), "func")
	if def.Representation == RepresentationMarshalJSON {
		return classifyMarshalJSONResultShape(def, results, errIface, qual)
	}
	if def.Representation == RepresentationSSE {
		switch {
		case results.Len() == 0:
			return ResultShapeNone, nil
		case results.Len() == 1 && types.Implements(results.At(0).Type(), errIface):
			return ResultShapeError, nil
		default:
			return ResultShapeInvalid, fmt.Errorf("sse handler method %s must return nothing or a single error", sigStr)
		}
	}
	if slices.ContainsFunc(def.Arguments, func(a Argument) bool {
		return a.Type == ArgumentTypeExecute && a.Identifier == TemplateNameScopeIdentifierExecute
	}) {
		if results.Len() != 1 || !types.Implements(results.At(0).Type(), errIface) {
			return ResultShapeInvalid, fmt.Errorf("method %s receiving the execute callback must return only error", sigStr)
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
		return ResultShapeInvalid, fmt.Errorf("the second result of %s must be an error or a bool, got %s", sigStr, types.TypeString(last, qual))
	case 0:
		return ResultShapeInvalid, fmt.Errorf("method %s has no results; it should have one or two", sigStr)
	default:
		return ResultShapeInvalid, fmt.Errorf("method %s has %d results; it should have one or two", sigStr, results.Len())
	}
}

// classifyNestedCallResultShape validates a nested call's results: one
// value, optionally followed by an error or bool.
func classifyNestedCallResultShape(name string, sig *types.Signature, qual types.Qualifier) (ResultShape, error) {
	results := sig.Results()
	errIface := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	sigStr := name + strings.TrimPrefix(types.TypeString(sig, qual), "func")
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
		return ResultShapeInvalid, fmt.Errorf("the second result of %s must be an error or a bool, got %s", sigStr, types.TypeString(last, qual))
	case 0:
		return ResultShapeInvalid, fmt.Errorf("method %s has no results; it should have one or two", sigStr)
	default:
		return ResultShapeInvalid, fmt.Errorf("method %s has %d results; it should have one or two", sigStr, results.Len())
	}
}

// classifyMarshalJSONResultShape enforces the marshalJSON contract: the
// wrapped method must produce exactly one non-error value to marshal,
// optionally followed by an error.
func classifyMarshalJSONResultShape(def *Definition, results *types.Tuple, errIface *types.Interface, qual types.Qualifier) (ResultShape, error) {
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
		if first := results.At(0).Type(); types.Implements(first, errIface) {
			return ResultShapeInvalid, fmt.Errorf("marshalJSON requires a non-error first result to marshal but %s returns an error value", sigStr)
		}
		if last := results.At(1).Type(); !types.Implements(last, errIface) {
			return ResultShapeInvalid, fmt.Errorf("marshalJSON requires the second result of %s to be an error, got %s", sigStr, types.TypeString(last, qual))
		}
		return ResultShapeDataError, nil
	default:
		return ResultShapeInvalid, fmt.Errorf("marshalJSON allows at most two results but %s has %d", sigStr, results.Len())
	}
}
