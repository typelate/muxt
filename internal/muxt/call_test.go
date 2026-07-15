package muxt

import (
	"go/token"
	"go/types"
	"html/template"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

func TestArgument(t *testing.T) {
	fileSet := token.NewFileSet()
	packageList, err := packages.Load(&packages.Config{
		Fset: fileSet,
		Mode: packages.NeedModule | packages.NeedTypesInfo | packages.NeedName | packages.NeedFiles | packages.NeedTypes | packages.NeedSyntax | packages.NeedEmbedPatterns | packages.NeedEmbedFiles | packages.NeedImports,
		Dir:  "testdata/example",
	}, ".")
	require.NoError(t, err)

	examplePkg := packageList[0].Types
	require.NotNil(t, examplePkg)

	httpPkg := findImport(examplePkg, "net/http")
	require.NotNil(t, httpPkg)
	httpRequestPtrType := types.NewPointer(httpPkg.Scope().Lookup("Request").Type())
	require.NotNil(t, httpRequestPtrType)
	httpResponseWriterType := httpPkg.Scope().Lookup("ResponseWriter").Type()
	require.NotNil(t, httpResponseWriterType)

	contextPkg := findImport(examplePkg, "context")
	require.NotNil(t, contextPkg)
	contextContextType := contextPkg.Scope().Lookup("Context").Type()
	require.NotNil(t, contextContextType)

	netURLPkg := findImport(examplePkg, "net/url")
	require.NotNil(t, contextPkg)
	netURLValuesType := netURLPkg.Scope().Lookup("Values").Type()
	require.NotNil(t, netURLValuesType)

	mimeMultipartPkg := findImport(examplePkg, "mime/multipart")
	require.NotNil(t, mimeMultipartPkg)
	multipartFormType := mimeMultipartPkg.Scope().Lookup("Form").Type()
	require.NotNil(t, multipartFormType)

	// mime/multipart.Form

	serverType := examplePkg.Scope().Lookup("Server").Type().(*types.Named)
	emptyStruct := examplePkg.Scope().Lookup("Empty").Type().(*types.Named)

	for _, tc := range []struct {
		Name     string
		Receiver *types.Named
		Template string
		Expect   func(t *testing.T, defs []Definition, err error)
	}{
		{Name: "no args", Receiver: serverType, Template: `{{define "GET / M()"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.Len(t, defs, 1)
			require.Len(t, defs[0].Arguments, 0)
			require.Equal(t, "M", defs[0].Identifier())
			require.NotNil(t, defs[0].sig)
		}},
		{Name: "receiver method call", Receiver: serverType, Template: `{{define "GET / M()"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.Len(t, defs, 1)
			require.Equal(t, "M", defs[0].Identifier())
			require.Empty(t, defs[0].Arguments)
			require.True(t, defs[0].isMethod, "M is a method on the receiver")
		}},
		{Name: "package function call", Receiver: serverType, Template: `{{define "GET / FunctionContext(ctx)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.Len(t, defs, 1)
			require.Equal(t, "FunctionContext", defs[0].Identifier())
			require.Equal(t, ArgumentTypeRequestContext, defs[0].Arguments[0].Type)
			require.False(t, defs[0].isMethod, "FunctionContext is a package-scope function, not a receiver method")
		}},
		{Name: "request", Receiver: serverType, Template: `{{define "GET / HTTPRequest(request)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.Len(t, defs, 1)
			require.NotNil(t, defs[0].sig)
			require.Len(t, defs[0].Arguments, 1)
			require.Equal(t, "HTTPRequest", defs[0].Identifier())
			require.Equal(t, "request", defs[0].Arguments[0].Identifier)
			require.Equal(t, ArgumentTypeRequest, defs[0].Arguments[0].Type)
			require.True(t, types.Identical(httpRequestPtrType, defs[0].Arguments[0].ParamType))
		}},
		{Name: "context", Receiver: serverType, Template: `{{define "GET / Context(ctx)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.Len(t, defs, 1)
			require.Len(t, defs[0].Arguments, 1)
			require.Equal(t, "Context", defs[0].Identifier())
			require.Equal(t, "ctx", defs[0].Arguments[0].Identifier)
			require.Equal(t, ArgumentTypeRequestContext, defs[0].Arguments[0].Type)
			require.True(t, types.Identical(contextContextType, defs[0].Arguments[0].ParamType))
		}},
		{Name: "response writer", Receiver: serverType, Template: `{{define "GET / HTTPResponseWriter(response)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.Len(t, defs, 1)
			require.Equal(t, "HTTPResponseWriter", defs[0].Identifier())
			require.Equal(t, "response", defs[0].Arguments[0].Identifier)
			require.Equal(t, ArgumentTypeResponse, defs[0].Arguments[0].Type)
			require.True(t, types.Identical(httpResponseWriterType, defs[0].Arguments[0].ParamType))
		}},
		{Name: "form", Receiver: serverType, Template: `{{define "GET / URLValues(form)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.Len(t, defs, 1)
			require.Equal(t, "URLValues", defs[0].Identifier())
			require.Equal(t, "form", defs[0].Arguments[0].Identifier)
			require.Equal(t, ArgumentTypeRequestForm, defs[0].Arguments[0].Type)
			require.True(t, types.Identical(netURLValuesType, defs[0].Arguments[0].ParamType))
		}},
		{Name: "multipart value form param is parsed as a field struct and rejected", Receiver: serverType, Template: `{{define "GET / MultipartForm(multipart)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			// A non-pointer multipart.Form falls into struct field-binding mode,
			// where its map fields are not parseable; raw mode requires
			// *multipart.Form (see "multipart raw pointer").
			require.ErrorContains(t, err, "failed to generate parse statements for multipart field Value: unsupported type: map[string][]string")
		}},
		{Name: "multipart raw pointer", Receiver: serverType, Template: `{{define "GET / MultipartFormPtr(multipart)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.Len(t, defs, 1)
			require.Equal(t, "MultipartFormPtr", defs[0].Identifier())
			require.Equal(t, ArgumentTypeRequestMultipartForm, defs[0].Arguments[0].Type)
			require.True(t, types.Identical(types.NewPointer(multipartFormType), defs[0].Arguments[0].ParamType))
		}},
		{Name: "multipart param neither struct nor pointer", Receiver: serverType, Template: `{{define "GET / String(multipart)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "expected multipart parameter type to be a struct")
		}},
		{Name: "form struct", Receiver: serverType, Template: `{{define "GET / FormStruct(form)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.Len(t, defs, 1)
			require.Equal(t, "FormStruct", defs[0].Identifier())
			require.Equal(t, ArgumentTypeRequestForm, defs[0].Arguments[0].Type)
			require.Equal(t, "In", defs[0].Arguments[0].ParamType.(*types.Named).Obj().Name())
		}},
		{Name: "form param neither struct nor url.Values", Receiver: serverType, Template: `{{define "GET / String(form)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "expected form parameter type to be a struct")
		}},
		{Name: "path value", Receiver: serverType, Template: `{{define "GET /{id} String(id)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.Len(t, defs, 1)
			require.Equal(t, "String", defs[0].Identifier())
			require.Equal(t, "id", defs[0].Arguments[0].Identifier)
			require.Equal(t, ArgumentTypeRequestPathValue, defs[0].Arguments[0].Type)
			basic, ok := defs[0].Arguments[0].ParamType.(*types.Basic)
			require.True(t, ok)
			require.Equal(t, types.String, basic.Kind())
		}},
		{Name: "last event id", Receiver: serverType, Template: `{{define "GET / String(lastEventID)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.Len(t, defs, 1)

			require.Equal(t, "String", defs[0].Identifier())
			require.Equal(t, "lastEventID", defs[0].Arguments[0].Identifier)
			require.Equal(t, ArgumentTypeLastEventID, defs[0].Arguments[0].Type)
			basic, ok := defs[0].Arguments[0].ParamType.(*types.Basic)
			require.True(t, ok)
			require.Equal(t, types.String, basic.Kind())
		}},
		{Name: "nested method call", Receiver: serverType, Template: `{{define "GET / Any(Context(ctx))"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.Len(t, defs, 1)
			// The outer Any(any) receives the result of the inner Context call.

			require.Equal(t, "Any", defs[0].Identifier())

			require.Len(t, defs[0].Arguments, 1)

			require.NotEmpty(t, defs[0].Arguments[0].Identifier)
			isTypeAny(t, defs[0].Arguments[0].ParamType)

			nested := defs[0].Arguments[0]

			require.Equal(t, "Context", defs[0].Arguments[0].Identifier)
			require.True(t, nested.isMethod, "Context is a receiver method")
			require.NotNil(t, nested.sig, "nested call signature")

			require.Equal(t, ArgumentTypeCall, defs[0].Arguments[0].Type)
			require.Equal(t, "Context", defs[0].Arguments[0].Identifier)
			isTypeAny(t, defs[0].Arguments[0].ParamType)

			require.Len(t, defs[0].Arguments[0].args, 1)
			require.Equal(t, ArgumentTypeRequestContext, defs[0].Arguments[0].args[0].Type)
			require.Equal(t, "ctx", defs[0].Arguments[0].args[0].Identifier)
			require.Equal(t, contextContextType, defs[0].Arguments[0].args[0].ParamType)
		}},
		{Name: "nested package function call", Receiver: serverType, Template: `{{define "GET / Any(FunctionContext(ctx))"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.Len(t, defs, 1)

			isTypeAny(t, defs[0].Arguments[0].ParamType)

			requireArgument(t, defs[0].Arguments, 0, "FunctionContext", ArgumentTypeCall, "any")
			nested := defs[0].Arguments[0]
			require.False(t, nested.isMethod, "FunctionContext is a package-scope function")
			requireArgument(t, nested.args, 0, "ctx", ArgumentTypeRequestContext, "context.Context")
		}},
		{Name: "synthesized method", Receiver: emptyStruct, Template: `{{define "GET / DoesNotExist(ctx)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.Len(t, defs, 1)
			// The receiver has no DoesNotExist method and it is not a package
			// function, so its signature is synthesized from the call scope.
			require.NotNil(t, defs[0].sig)
			require.True(t, defs[0].isMethod, "a synthesized call becomes a required receiver method")

			require.Equal(t, "ctx", defs[0].Arguments[0].Identifier)
			require.True(t, types.Identical(contextContextType, defs[0].Arguments[0].ParamType))
		}},
		{Name: "error when argument is not assignable to parameter", Receiver: serverType, Template: `{{define "GET / Context(request)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "method expects type context.Context but request is *http.Request")
		}},
		{Name: "passing context argument when parameter is a string", Receiver: serverType, Template: `{{define "GET / String(ctx)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "method expects type string but ctx is context.Context")
		}},
		{Name: "passing request when parameter is a receiver pointer", Receiver: serverType, Template: `{{define "GET / PtrServer(request)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "method expects type *Server but request is *http.Request")
		}},
		{Name: "passing response when parameter is a string", Receiver: serverType, Template: `{{define "GET / String(response)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "method expects type string but response is http.ResponseWriter")
		}},
		{Name: "too few arguments", Receiver: serverType, Template: `{{define "GET / String()"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, `handler func String(string) any expects 1 arguments but call String() has 0`)
			require.Len(t, defs, 1)
		}},
		{Name: "too many arguments", Receiver: serverType, Template: `{{define "GET /{name} Context(ctx, name)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, `handler func Context(context.Context) any expects 1 arguments but call Context(ctx, name) has 2`)
		}},
		{Name: "execute callback when method takes no parameters", Receiver: serverType, Template: `{{define "GET / NoParams(execute)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "execute argument for NoParams must be a func(...) error")
		}},
		{Name: "wrong argument type in shared field list", Receiver: serverType, Template: `{{define "GET /post/{postID}/comment/{commentID} FieldList(ctx, request, commentID)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "method expects type string but request is *http.Request")
		}},
		{Name: "data result shape", Receiver: serverType, Template: `{{define "GET / M()"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.Equal(t, ResultShapeData, defs[0].ResultShape())
		}},
		{Name: "data and error result shape", Receiver: serverType, Template: `{{define "GET / StringError()"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.Equal(t, ResultShapeDataError, defs[0].ResultShape())
		}},
		{Name: "data and ok result shape", Receiver: serverType, Template: `{{define "GET / StringOK()"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.Equal(t, ResultShapeDataOK, defs[0].ResultShape())
		}},
		{Name: "method with no results", Receiver: serverType, Template: `{{define "GET / NoResults()"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, `method for pattern "GET / NoResults()" has no results it should have one or two`)
		}},
		{Name: "second result must be error or bool", Receiver: serverType, Template: `{{define "GET / TwoResultsSecondNotErrorOrBool()"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "expected last result to be either an error or a bool")
		}},
		{Name: "execute method must return only error", Receiver: serverType, Template: `{{define "GET / ExecuteReturnsValue(execute)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "method ExecuteReturnsValue using the execute callback must return only error")
		}},
		{Name: "sse method must return nothing or an error", Receiver: serverType, Template: `{{define "GET /x sse(SSEReturnsValue(execute))"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "method SSEReturnsValue using the sse callback must return nothing or an error")
		}},
		{Name: "sse method returning nothing", Receiver: serverType, Template: `{{define "GET /x sse(SSEEvents(execute))"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.Equal(t, ResultShapeNone, defs[0].ResultShape())
		}},
		{Name: "nested call with no results", Receiver: serverType, Template: `{{define "GET / Any(NoResults())"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "method NoResults has no results it should have one or two")
		}},
		{Name: "execute callback with data parameter", Receiver: serverType, Template: `{{define "GET / ExecuteTD(execute)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.True(t, defs[0].Arguments[0].CallbackHasArg())
			named, ok := defs[0].Arguments[0].CallbackResultType().(*types.Named)
			require.True(t, ok)
			require.Equal(t, "TD", named.Obj().Name())
		}},
		{Name: "execute callback without data parameter", Receiver: serverType, Template: `{{define "GET / ExecuteNoArg(execute)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.False(t, defs[0].Arguments[0].CallbackHasArg())
			st, ok := defs[0].Arguments[0].CallbackResultType().(*types.Struct)
			require.True(t, ok)
			require.Zero(t, st.NumFields())
		}},
		{Name: "execute callback parameter is not a function", Receiver: serverType, Template: `{{define "GET / ExecuteNotFunc(execute)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "execute argument for ExecuteNotFunc must be a func(...) error")
		}},
		{Name: "execute callback with too many parameters", Receiver: serverType, Template: `{{define "GET / ExecuteMultiArg(execute)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "execute callback must have zero or one parameter; wrap multiple values in a struct")
		}},
		{Name: "sse callback parameter is not a function", Receiver: serverType, Template: `{{define "GET /x sse(SSECallbackNotFunc(execute))"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "execute parameter for SSECallbackNotFunc must be a function")
		}},
		{Name: "sse callback with too many parameters", Receiver: serverType, Template: `{{define "GET /x sse(SSECallbackMultiArg(execute))"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "sse callback must have zero or one parameter; wrap multiple values in a struct")
		}},
		{Name: "execute callback method not defined on receiver", Receiver: emptyStruct, Template: `{{define "GET / NotDefined(execute)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "method NotDefined using the execute callback must be defined on the receiver type")
		}},
		{Name: "sse prefixed callback template missing", Receiver: serverType, Template: `{{define "GET /events sse(SSETwoCallbacks(execute, sseClock))"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, `no template "sseClock" for sse argument sseClock`)
		}},
		{Name: "sse prefixed callback template defined", Receiver: serverType, Template: `{{define "GET /events sse(SSETwoCallbacks(execute, sseClock))"}}{{end}}{{define "sseClock"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.Len(t, defs[0].Arguments, 2)
			require.NotNil(t, defs[0].Arguments[1].Template())
			require.Equal(t, "sseClock", defs[0].Arguments[1].Template().Name())
		}},
		{Name: "sse message template missing", Receiver: serverType, Template: `{{define "GET /x sse(SSECallbackNotFunc(fooMessage))"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, `no template "fooMessage" for sse message argument fooMessage`)
		}},
		{Name: "sse message template defined", Receiver: serverType, Template: `{{define "GET /x sse(SSECallbackNotFunc(fooMessage))"}}{{end}}{{define "fooMessage"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.Equal(t, ArgumentTypeSendMessage, defs[0].Arguments[0].Type)
			require.NotNil(t, defs[0].Arguments[0].Template())
			require.Equal(t, "fooMessage", defs[0].Arguments[0].Template().Name())
		}},
		{Name: "method with three results", Receiver: serverType, Template: `{{define "GET / ThreeResults()"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "method ThreeResults has 3 results it should have one or two")
		}},
		{Name: "nested call with three results", Receiver: serverType, Template: `{{define "GET / Any(ThreeResults())"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "method ThreeResults has 3 results it should have one or two")
		}},
		{Name: "path value with unsupported basic type", Receiver: serverType, Template: `{{define "GET /{id} Float64(id)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "method param type float64 not supported")
		}},
		{Name: "path value with unsupported named type", Receiver: serverType, Template: `{{define "GET /{id} URLParam(id)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "unsupported type: url.URL")
		}},
		{Name: "path value with text unmarshaler", Receiver: serverType, Template: `{{define "GET /{id} TextUnmarshalerParam(id)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.Equal(t, ArgumentTypeRequestPathValue, defs[0].Arguments[0].Type)
		}},
		{Name: "last event id with unsupported basic type", Receiver: serverType, Template: `{{define "GET / Float64(lastEventID)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "method param type float64 not supported")
		}},
		{Name: "form struct with unsupported field type", Receiver: serverType, Template: `{{define "GET / FormUnsupportedField(form)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "failed to generate parse statements for form field href: unsupported type: url.URL")
		}},
		{Name: "multipart struct with file header fields", Receiver: serverType, Template: `{{define "POST / Upload(multipart)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.Equal(t, ArgumentTypeRequestMultipartForm, defs[0].Arguments[0].Type)
		}},
		{Name: "multipart struct with unsupported field type", Receiver: serverType, Template: `{{define "POST / BadUpload(multipart)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "failed to generate parse statements for multipart field File: unsupported type: multipart.File")
		}},
		{Name: "form struct with file header field is unsupported", Receiver: serverType, Template: `{{define "GET / Upload(form)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "failed to generate parse statements for form field File: unsupported type: *multipart.FileHeader")
		}},
		{Name: "form struct field bindings", Receiver: serverType, Template: `{{define "GET / TaggedForm(form)"}}{{end}}{{define "count-template"}}<input name="count-input" minlength="1">{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			fields := defs[0].Arguments[0].FormFields()
			require.Len(t, fields, 2)

			count := fields[0]
			require.Equal(t, "Count", count.Field.Name())
			require.Equal(t, "count-input", count.InputName)
			require.NotNil(t, count.Template)
			require.Equal(t, "count-template", count.Template.Name())
			require.False(t, count.Slice)
			require.False(t, count.FileHeader)
			require.Equal(t, UnmarshalInt, count.Method)
			require.Len(t, count.Validations, 1)
			minLength, ok := count.Validations[0].(MinLengthValidation)
			require.True(t, ok)
			require.Equal(t, "count-input", minLength.Name)
			require.Equal(t, 1, minLength.MinLength)

			tags := fields[1]
			require.Equal(t, "Tags", tags.Field.Name())
			require.Equal(t, "tag", tags.InputName)
			require.Nil(t, tags.Template)
			require.True(t, tags.Slice)
			require.Equal(t, UnmarshalString, tags.Method)
			basic, ok := tags.Elem.(*types.Basic)
			require.True(t, ok)
			require.Equal(t, types.String, basic.Kind())
		}},
		{Name: "form field validation attribute is invalid", Receiver: serverType, Template: `{{define "GET / TaggedForm(form)"}}{{end}}{{define "count-template"}}<input name="count-input" minlength="abc">{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.ErrorContains(t, err, "minlength must be an integer")
		}},
		{Name: "multipart struct field bindings", Receiver: serverType, Template: `{{define "POST / Upload(multipart)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			fields := defs[0].Arguments[0].FormFields()
			require.Len(t, fields, 4)
			require.Equal(t, "Name", fields[0].Field.Name())
			require.False(t, fields[0].FileHeader)
			require.Equal(t, "Tags", fields[1].Field.Name())
			require.True(t, fields[1].Slice)
			require.Equal(t, "File", fields[2].Field.Name())
			require.True(t, fields[2].FileHeader)
			require.False(t, fields[2].Slice)
			require.Equal(t, "Files", fields[3].Field.Name())
			require.True(t, fields[3].FileHeader)
			require.True(t, fields[3].Slice)
		}},
		{Name: "raw form param has no field bindings", Receiver: serverType, Template: `{{define "GET / URLValues(form)"}}{{end}}`, Expect: func(t *testing.T, defs []Definition, err error) {
			require.NoError(t, err)
			require.Empty(t, defs[0].Arguments[0].FormFields())
		}},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ts := template.Must(template.New("").Parse(tc.Template))
			defs, err := Definitions(ts, "templates")
			if err != nil {
				t.Fatal(err)
			}

			for i := range defs {
				err = ResolveCall(&defs[i], examplePkg, tc.Receiver, packageList)
				if err != nil {
					break
				}
			}
			tc.Expect(t, defs, err)
		})
	}
}

func isTypeAny(t *testing.T, tp types.Type) {
	t.Helper()
	anyAliasType, ok := tp.(*types.Alias)
	require.True(t, ok)
	require.Equal(t, "any", anyAliasType.Obj().Name())
}

// requireArgument asserts that the argument at index i has the expected
// identifier, classification, and parameter type (compared by its type string).
func requireArgument(t *testing.T, args []Argument, i int, identifier string, argType ArgumentType, paramType string) {
	t.Helper()
	require.Greater(t, len(args), i, "argument at index %d does not exist", i)
	arg := args[i]
	require.Equal(t, identifier, arg.Identifier, "Argument[%d].Identifier", i)
	require.Equal(t, argType, arg.Type, "Argument[%d].Type", i)
	require.NotNil(t, arg.ParamType, "Argument[%d].ParamType", i)
	require.Equal(t, paramType, arg.ParamType.String(), "Argument[%d].ParamType", i)
}

func findImport(example *types.Package, pkg string) *types.Package {
	for _, p := range example.Imports() {
		if p.Path() == pkg {
			return p
		}
	}
	return nil
}
