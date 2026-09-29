package muxt

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/typelate/muxt/internal/astgen"
)

// linkArguments marks the first occurrence, depth first, of each request
// value as the one that declares its local, links each wildcard segment to
// its first occurrence, and rejects a repeat that needs a different value
// than the first.
func linkArguments(def *Definition, qual types.Qualifier) error {
	return linkArgumentsSeen(def, qual, def.Arguments, make(map[string]*Argument))
}

func linkArgumentsSeen(def *Definition, qual types.Qualifier, args []Argument, seen map[string]*Argument) error {
	for i := range args {
		arg := &args[i]
		switch arg.Type {
		case ArgumentTypeCall:
			if err := linkArgumentsSeen(def, qual, arg.args, seen); err != nil {
				return err
			}
		case ArgumentTypeRequestPathValue, ArgumentTypeLastEventID, ArgumentTypeRequestForm, ArgumentTypeRequestMultipartForm,
			ArgumentTypeRequestContext, ArgumentTypeRequestBody:
			first, ok := seen[arg.Identifier]
			if !ok {
				seen[arg.Identifier] = arg
				arg.declares = true
				if arg.Type == ArgumentTypeRequestPathValue {
					for j := range def.Segments {
						if def.Segments[j].IsWildcard() && def.Segments[j].value == arg.Identifier {
							def.Segments[j].argument = arg
						}
					}
				}
				continue
			}
			if first.direct && arg.direct {
				continue
			}
			if types.Identical(first.paramType, arg.paramType) {
				continue
			}
			return def.argUsesErrorf(arg.Identifier, "%s is passed more than once with different types: %s and %s",
				arg.Identifier, types.TypeString(first.paramType, qual), types.TypeString(arg.paramType, qual))
		}
	}
	return nil
}

// defaultScopeType returns the type an argument identifier binds to: a
// reserved identifier's, as checker reports it, or string for lastEventID and
// a path value.
func defaultScopeType(checker Checker, def *Definition, argumentIdentifier string) (types.Type, bool) {
	switch argumentIdentifier {
	case TemplateNameScopeIdentifierLastEventID:
		return types.Universe.Lookup("string").Type(), true
	case TemplateNameScopeIdentifierHTTPRequest, TemplateNameScopeIdentifierHTTPResponse, TemplateNameScopeIdentifierContext,
		TemplateNameScopeIdentifierForm, TemplateNameScopeIdentifierMultipart, TemplateNameScopeIdentifierRequestBody:
		tp, err := checker.ScopeType(argumentIdentifier)
		return tp, err == nil
	default:
		if _, ok := pathParameter(def.Segments, argumentIdentifier); ok {
			return types.Universe.Lookup("string").Type(), true
		}
		return nil, false
	}
}

func newArgumentFromIdentifier(def *Definition, checker Checker, arg *ast.Ident, param types.Type, qual types.Qualifier) (Argument, error) {
	a := Argument{
		Identifier: arg.Name,
		paramType:  param,
	}
	switch arg.Name {
	case TemplateNameScopeIdentifierContext:
		a.Type = ArgumentTypeRequestContext
		if err := bindScopeValue(&a, checker, qual); err != nil {
			return a, err
		}
	case TemplateNameScopeIdentifierForm:
		a.Type = ArgumentTypeRequestForm
		if err := bindFormArgument(&a, def, checker, qual, false); err != nil {
			return a, err
		}
	case TemplateNameScopeIdentifierMultipart:
		a.Type = ArgumentTypeRequestMultipartForm
		if err := bindFormArgument(&a, def, checker, qual, true); err != nil {
			return a, err
		}
	case TemplateNameScopeIdentifierHTTPRequest:
		a.Type = ArgumentTypeRequest
		if err := bindScopeValue(&a, checker, qual); err != nil {
			return a, err
		}
	case TemplateNameScopeIdentifierHTTPResponse:
		a.Type = ArgumentTypeResponse
		if err := bindScopeValue(&a, checker, qual); err != nil {
			return a, err
		}
	case TemplateNameScopeIdentifierLastEventID:
		a.Type = ArgumentTypeLastEventID
		if err := bindParsedArgument(&a, checker, qual); err != nil {
			return a, err
		}
	case TemplateNameScopeIdentifierExecute:
		a.Type = ArgumentTypeExecute
		a.template = def.template
	case TemplateNameScopeIdentifierRequestBody:
		a.Type = ArgumentTypeRequestBody
		if err := checkRequestBodyParameter(&a, checker, qual); err != nil {
			return a, err
		}
	default:
		if _, ok := pathParameter(def.Segments, arg.Name); ok {
			a.Type = ArgumentTypeRequestPathValue
			if err := bindParsedArgument(&a, checker, qual); err != nil {
				return a, err
			}
			return a, nil
		}
		if isSSEArgument(arg.Name) {
			// An sse-prefixed render callback (sseClock, sseMetrics, ...) renders
			// the same-named template. Template existence is validated in
			// resolveCallbackShapes once all arguments are hydrated.
			a.Type = ArgumentTypeExecute
			a.template = def.template.Lookup(arg.Name)
			return a, nil
		}
		if isSignalsCallback(def, arg) {
			// The callback contract is validated in resolveCallbackShapes; the
			// remainder before the Signals suffix is only a label, so muxt
			// never derives an identifier from it.
			a.Type = ArgumentTypeSignalsCallback
			return a, nil
		}
		if isSendMessage(def, arg) {
			a.Type = ArgumentTypeSendMessage

			t := def.template.Lookup(arg.Name)
			if t == nil {
				if suggestion, ok := astgen.NearestString(arg.Name, templateNames(def.template)); ok {
					return Argument{}, fmt.Errorf("no template %q for sse message argument %s; did you mean %q?", arg.Name, arg.Name, suggestion)
				}
				return Argument{}, fmt.Errorf("no template %q for sse message argument %s", arg.Name, arg.Name)
			}
			a.template = t

			return a, nil
		}
		return Argument{}, errors.New("unknown argument type")
	}
	return a, nil
}

// bindScopeValue requires a request value -- ctx, request or response -- to
// be assignable to its parameter as it is.
func bindScopeValue(a *Argument, checker Checker, qual types.Qualifier) error {
	at, err := checker.ScopeType(a.Identifier)
	if err != nil {
		return err
	}
	a.scopeType = at
	if !types.AssignableTo(at, a.paramType) {
		return fmt.Errorf("method expects type %s but %s is %s", types.TypeString(a.paramType, qual), a.Identifier, types.TypeString(at, qual))
	}
	a.direct = true
	return nil
}

func isSignalsCallback(def *Definition, arg *ast.Ident) bool {
	return def.IsSignalsCallback(arg.Name)
}

// IsSignalsCallback reports whether name is a Signals-suffixed patch-signals
// callback argument on this route. A declared path parameter wins: a path
// value that happens to end in Signals stays a path value.
func (def *Definition) IsSignalsCallback(name string) bool {
	_, isPathParameter := pathParameter(def.Segments, name)
	return def.Representation == RepresentationSSE &&
		isSignalsCallbackArgument(name) &&
		!isPathParameter
}

// isSignalsCallbackArgument reports whether name is a datastar patch-signals
// callback argument: a Signals-suffixed identifier (countsSignals) whose
// func(T) error argument is marshaled as the event payload. Only valid on sse
// routes in --output-datastar packages.
func isSignalsCallbackArgument(name string) bool {
	return strings.HasSuffix(name, "Signals") && token.IsIdentifier(name)
}

func isSendMessage(def *Definition, arg *ast.Ident) bool {
	return def.Representation == RepresentationSSE && isSSEMessageArgument(arg.Name)
}

// isSSEMessageArgument reports whether name is an sse send-message template
// argument: a Message-suffixed identifier (fooMessage) naming the template the
// message renders with. Only valid on sse routes.
func isSSEMessageArgument(name string) bool {
	return strings.HasSuffix(name, "Message") && token.IsIdentifier(name)
}

// checkRequestBodyParameter requires the parameter bound to the reserved body
// argument to be exactly io.Reader. The request body is a single-use stream,
// so the method must not be able to assume more than one read.
func checkRequestBodyParameter(a *Argument, checker Checker, qual types.Qualifier) error {
	readerType, err := checker.ScopeType(TemplateNameScopeIdentifierRequestBody)
	if err != nil {
		return err
	}
	a.scopeType = readerType
	if !types.Identical(a.paramType, readerType) {
		return fmt.Errorf("%s parameter must have type io.Reader, got %s", TemplateNameScopeIdentifierRequestBody, types.TypeString(a.paramType, qual))
	}
	a.direct = true
	return nil
}
