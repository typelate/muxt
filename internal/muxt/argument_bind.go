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
		if hasPathParameter(def.Segments, argumentIdentifier) {
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
	var err error
	switch arg.Name {
	case TemplateNameScopeIdentifierContext:
		a.Type = ArgumentTypeRequestContext
		err = bindScopeValue(&a, checker, qual)
	case TemplateNameScopeIdentifierForm:
		a.Type = ArgumentTypeRequestForm
		err = bindFormArgument(&a, def, checker, qual, false)
	case TemplateNameScopeIdentifierMultipart:
		a.Type = ArgumentTypeRequestMultipartForm
		err = bindFormArgument(&a, def, checker, qual, true)
	case TemplateNameScopeIdentifierHTTPRequest:
		a.Type = ArgumentTypeRequest
		err = bindScopeValue(&a, checker, qual)
	case TemplateNameScopeIdentifierHTTPResponse:
		a.Type = ArgumentTypeResponse
		err = bindScopeValue(&a, checker, qual)
	case TemplateNameScopeIdentifierLastEventID:
		a.Type = ArgumentTypeLastEventID
		err = bindParsedArgument(&a, checker, qual)
	case TemplateNameScopeIdentifierExecute:
		a.Type = ArgumentTypeExecute
		a.template = def.template
	case TemplateNameScopeIdentifierRequestBody:
		a.Type = ArgumentTypeRequestBody
		err = checkRequestBodyParameter(&a, checker, qual)
	default:
		err = bindNamedArgument(&a, def, checker, qual)
	}
	return a, err
}

// bindNamedArgument binds an identifier that is not reserved: a path value or
// one of the sse callbacks and messages, which exist only on sse routes.
func bindNamedArgument(a *Argument, def *Definition, checker Checker, qual types.Qualifier) error {
	name := a.Identifier
	switch {
	case hasPathParameter(def.Segments, name):
		a.Type = ArgumentTypeRequestPathValue
		return bindParsedArgument(a, checker, qual)
	case isSSEArgument(name):
		// Template existence is validated in resolveCallbackShapes once all
		// arguments are hydrated.
		a.Type = ArgumentTypeExecute
		a.template = def.template.Lookup(name)
		return nil
	case def.isSignalsCallback(name):
		// The remainder before the Signals suffix is only a label, so muxt
		// never derives an identifier from it.
		a.Type = ArgumentTypeSignalsCallback
		return nil
	case def.isSendMessage(name):
		a.Type = ArgumentTypeSendMessage
		a.template = def.template.Lookup(name)
		if a.template == nil {
			return def.missingSSEMessageTemplateError(name)
		}
		return nil
	}
	return errors.New("unknown argument type")
}

func (def *Definition) missingSSEMessageTemplateError(name string) error {
	if suggestion, ok := astgen.NearestString(name, templateNames(def.template)); ok {
		return fmt.Errorf("no template %q for sse message argument %s; did you mean %q?", name, name, suggestion)
	}
	return fmt.Errorf("no template %q for sse message argument %s", name, name)
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

// isSignalsCallback reports whether name is a Signals-suffixed patch-signals
// callback argument on this route. A declared path parameter wins: a path
// value that happens to end in Signals stays a path value.
func (def *Definition) isSignalsCallback(name string) bool {
	return def.Representation == RepresentationSSE &&
		isSignalsCallbackArgument(name) &&
		!hasPathParameter(def.Segments, name)
}

// isSignalsCallbackArgument reports whether name is a datastar patch-signals
// callback argument: a Signals-suffixed identifier (countsSignals) whose
// func(T) error argument is marshaled as the event payload. Only valid on sse
// routes in --output-datastar packages.
func isSignalsCallbackArgument(name string) bool {
	return strings.HasSuffix(name, "Signals") && token.IsIdentifier(name)
}

func (def *Definition) isSendMessage(name string) bool {
	return def.Representation == RepresentationSSE && isSSEMessageArgument(name)
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
