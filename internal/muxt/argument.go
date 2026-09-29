package muxt

import (
	"go/types"
	"html/template"

	"github.com/typelate/muxt/internal/source"
)

type Argument struct {
	Identifier string
	Type       ArgumentType
	paramType  types.Type

	template *template.Template

	// sig, args, and isMethod describe a nested call argument
	// (Type == ArgumentTypeCall): the nested call's signature, its own
	// hydrated arguments, and whether it resolves to a receiver method.
	sig         *types.Signature
	args        []Argument
	isMethod    bool
	resultShape ResultShape

	// callbackResult and callbackHasArg describe a validated render-callback
	// argument (Type == ArgumentTypeExecute): the template data type T the
	// callback receives and whether the callback takes that data argument
	// (func(T) error vs func() error, where T = struct{}).
	callbackResult types.Type
	callbackHasArg bool

	// formFields describes how each struct field of a form or multipart
	// argument binds to the request (nil for the raw url.Values /
	// *multipart.Form mode).
	formFields []FieldBinding

	// scopeType, direct and method describe a request value argument: the
	// type the identifier binds to, whether that value is assignable to the
	// parameter as it is, and otherwise how it parses from its string form.
	scopeType types.Type
	direct    bool
	method    UnmarshalMethod

	textMarshaler bool
	declares      bool
}

// ScopeType returns the type a request value argument binds to before any
// parsing: *http.Request for request, string for a path value, and so on.
func (a Argument) ScopeType() source.Type { return source.NewType(a.scopeType) }

// Direct reports whether a request value argument is assignable to its
// parameter as it is. A path value or lastEventID that is not assignable
// parses with UnmarshalMethod; a form or multipart argument that is not
// assignable binds a struct through FormFields.
func (a Argument) Direct() bool { return a.direct }

// UnmarshalMethod returns how a path value or lastEventID argument that is
// not Direct parses from its string form.
func (a Argument) UnmarshalMethod() UnmarshalMethod { return a.method }

// TextMarshaler reports whether a parsed path value or lastEventID
// argument's parameter type implements encoding.TextMarshaler, so a route
// path formats it back with MarshalText.
func (a Argument) TextMarshaler() bool { return a.textMarshaler }

// Declares reports whether this is the first occurrence of a request value
// the generator declares a local for; a later occurrence reuses it.
func (a Argument) Declares() bool { return a.declares }

// Signature returns the resolved signature of a nested call argument
// (Type == ArgumentTypeCall), or nil for a leaf argument.
func (a Argument) Signature() source.Type {
	if a.sig == nil {
		return source.Type{}
	}
	return source.NewType(a.sig)
}

// IsMethod reports whether a nested call argument resolves to a receiver method
// (as opposed to a package-scope function).
func (a Argument) IsMethod() bool { return a.isMethod }

func (a Argument) Arguments() []Argument { return a.args }

func (a Argument) ParamType() source.Type { return source.NewType(a.paramType) }

func (a Argument) ResultShape() ResultShape { return a.resultShape }

// Template returns the template a render-callback argument (ArgumentTypeExecute)
// renders: the route template for the base execute callback, or the same-named
// template for an sse-prefixed callback (nil if that template does not exist).
func (a Argument) Template() *template.Template { return a.template }

// callbackSignature returns a render-callback argument's function signature
// (from its parameter type), or nil if the parameter type is not a function.
func (a Argument) callbackSignature() *types.Signature {
	if a.paramType == nil {
		return nil
	}
	sig, _ := a.paramType.Underlying().(*types.Signature)
	return sig
}

// CallbackResultType returns the template data type T a validated
// render-callback argument receives (struct{} for a func() error callback).
func (a Argument) CallbackResultType() source.Type { return source.NewType(a.callbackResult) }

// CallbackHasArg reports whether a validated render-callback argument's
// callback takes the template data argument (func(T) error vs func() error).
func (a Argument) CallbackHasArg() bool { return a.callbackHasArg }

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
	ArgumentTypeSendMessage
	ArgumentTypeSignalsCallback
	ArgumentTypeLastEventID
	ArgumentTypeRequestBody
	ArgumentTypeRequestBodyJSON
	ArgumentTypeCall
)

const (
	TemplateNameScopeIdentifierContext      = "ctx"
	TemplateNameScopeIdentifierForm         = "form"
	TemplateNameScopeIdentifierMultipart    = "multipart"
	TemplateNameScopeIdentifierHTTPRequest  = "request"
	TemplateNameScopeIdentifierHTTPResponse = "response"
	TemplateNameScopeIdentifierExecute      = "execute"
	TemplateNameScopeIdentifierLastEventID  = "lastEventID"
	TemplateNameScopeIdentifierRequestBody  = "body"

	// TemplateNameScopeIdentifierSignals is shorthand for
	// unmarshalJSON(body); it is rewritten during parsing and generation
	// rejects it unless the package targets Datastar.
	TemplateNameScopeIdentifierSignals = "signals"
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
