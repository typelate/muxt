package muxt

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"slices"
	"sync"
)

type Options struct {
	logger   *slog.Logger
	receiver any
	execute  ExecuteFunc[any]
	error    ExecuteFunc[error]
}

func newOptions() Options {
	return Options{
		logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
			Level: slog.LevelError,
		})),
		receiver: nil,
		execute:  defaultExecute,
		error:    internalServerErrorErrorFunc,
	}
}

func WithStructuredLogger(log *slog.Logger) Options { return newOptions().WithStructuredLogger(log) }
func WithReceiver(r any) Options                    { return newOptions().WithReceiver(r) }
func WithDataFunc(ex ExecuteFunc[any]) Options      { return newOptions().WithDataFunc(ex) }
func WithErrorFunc(ex ExecuteFunc[error]) Options   { return newOptions().WithErrorFunc(ex) }
func WithNoopErrorFunc() Options                    { return newOptions().WithNoopErrorFunc() }
func With500ErrorFunc() Options                     { return newOptions().With500ErrorFunc() }

func (o Options) WithStructuredLogger(log *slog.Logger) Options {
	o.logger = log
	return o
}

func (o Options) WithReceiver(r any) Options {
	o.receiver = r
	return o
}

func (o Options) WithDataFunc(ex ExecuteFunc[any]) Options {
	o.execute = ex
	return o
}

func (o Options) WithErrorFunc(ex ExecuteFunc[error]) Options {
	o.error = ex
	return o
}

func (o Options) WithNoopErrorFunc() Options {
	o.error = noopErrorFunc
	return o
}

func (o Options) With500ErrorFunc() Options {
	o.error = internalServerErrorErrorFunc
	return o
}

func noopErrorFunc(http.ResponseWriter, *http.Request, *template.Template, *slog.Logger, error) {}

func internalServerErrorErrorFunc(res http.ResponseWriter, _ *http.Request, t *template.Template, logger *slog.Logger, err error) {
	logger.Error("handler error", "error", err, "template", t.Name())
	http.Error(res, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func applyOptions(options []Options) *Options {
	result := newOptions()
	for _, o := range options {
		if o.logger != nil {
			result.logger = o.logger
		}
		if o.receiver != nil {
			result.receiver = o.receiver
		}
		if o.execute != nil {
			result.execute = o.execute
		}
		if o.error != nil {
			result.error = o.error
		}
	}
	return &result
}

func Handlers(mux *http.ServeMux, ts *template.Template, options ...Options) error {
	o := applyOptions(options)
	for _, t := range ts.Templates() {
		pattern, err, match := NewTemplateName(t.Name())
		if !match {
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to parse NewPattern for template %q: %w", t.Name(), err)
		}
		if pattern.Handler == "" {
			mux.HandleFunc(pattern.Pattern, simpleTemplateHandler(o.execute, t, o.logger))
			continue
		}
		ex, err := parser.ParseExpr(pattern.Handler)
		if err != nil {
			return fmt.Errorf("failed to parse handler expression: %w", err)
		}
		switch exp := ex.(type) {
		case *ast.CallExpr:
			h, err := callMethodHandler(o, t, pattern, exp)
			if err != nil {
				return fmt.Errorf("failed to create handler for %q: %w", pattern.Pattern+" "+pattern.Handler, err)
			}
			mux.HandleFunc(pattern.Pattern, h)
		default:
			return fmt.Errorf("unexpected handler expression %v", pattern.Handler)
		}
	}
	return nil
}

func callMethodHandler(o *Options, t *template.Template, pattern TemplateName, call *ast.CallExpr) (http.HandlerFunc, error) {
	pathParams, err := pattern.PathParameters()
	if err != nil {
		return nil, err
	}
	switch function := call.Fun.(type) {
	default:
		return nil, fmt.Errorf("expected method call on receiver")
	case *ast.Ident:
		return createSelectorHandler(o, t, call, function, pathParams)
	}
}

func createSelectorHandler(o *Options, t *template.Template, call *ast.CallExpr, method *ast.Ident, pathParams []string) (http.HandlerFunc, error) {
	m, err := serviceMethod(o, call, method)
	if err != nil {
		return nil, err
	}
	inputs, err := generateInputsFunction(o, t, m.Type(), call, pathParams)
	if err != nil {
		return nil, err
	}
	return generateOutputsFunction(o, t, m, inputs)
}

type inputsFunc = func(res http.ResponseWriter, req *http.Request) []reflect.Value

func generateOutputsFunction(o *Options, t *template.Template, method reflect.Value, inputs inputsFunc) (http.HandlerFunc, error) {
	methodType := method.Type()
	switch num := methodType.NumOut(); num {
	case 1:
		return valueResultHandler(o, t, method, inputs), nil
	case 2:
		if !methodType.Out(1).AssignableTo(reflect.TypeFor[error]()) {
			return nil, fmt.Errorf("the second result must be an error")
		}
		return valuesResultHandler(o, t, method, inputs), nil
	default:
		return nil, fmt.Errorf("method must either return (T) or (T, error)")
	}
}

func valueResultHandler(o *Options, t *template.Template, method reflect.Value, inputs inputsFunc) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		in := inputs(res, req)
		out := method.Call(in)
		o.execute(res, req, t, o.logger, out[0].Interface())
	}
}

func valuesResultHandler(o *Options, t *template.Template, method reflect.Value, inputs inputsFunc) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		in := inputs(res, req)
		out := method.Call(in)
		callRes, callErr := out[0], out[1]
		if !callErr.IsNil() {
			err := callErr.Interface().(error)
			o.error(res, req, t, o.logger, err)
			return
		}
		o.execute(res, req, t, o.logger, callRes.Interface())
	}
}

func serviceMethod(o *Options, call *ast.CallExpr, method *ast.Ident) (reflect.Value, error) {
	if o.receiver == nil {
		return reflect.Value{}, fmt.Errorf("receiver is nil")
	}
	if call.Ellipsis != token.NoPos {
		return reflect.Value{}, fmt.Errorf("ellipsis call not allowed")
	}
	r := reflect.ValueOf(o.receiver)
	m := r.MethodByName(method.Name)
	if !m.IsValid() {
		return reflect.Value{}, fmt.Errorf("method %s not found on %s", method.Name, r.Type())
	}
	return m, nil
}

func printNode(node ast.Node) string {
	buf := bytes.NewBuffer(nil)
	_ = format.Node(buf, token.NewFileSet(), node)
	return buf.String()
}

const (
	// requestArgumentIdentifier identifies an *http.Request
	requestArgumentIdentifier = "request"

	// contextArgumentIdentifier identifies a context.Context off of *http.Request
	contextArgumentIdentifier = "ctx"

	// responseArgumentIdentifier identifies an http.ResponseWriter
	responseArgumentIdentifier = "response"

	// loggerArgumentIdentifier identifies an *slog.Logger
	loggerArgumentIdentifier = "logger"

	// loggerArgumentTemplate identifies a *template.Template
	loggerArgumentTemplate = "template"
)

func rootScope() []string {
	return []string{
		requestArgumentIdentifier,
		contextArgumentIdentifier,
		responseArgumentIdentifier,
		loggerArgumentIdentifier,
		loggerArgumentTemplate,
	}
}

func generateInputsFunction(o *Options, t *template.Template, method reflect.Type, call *ast.CallExpr, pathParams []string) (inputsFunc, error) {
	if method.NumIn() != len(call.Args) {
		return nil, fmt.Errorf("wrong number of arguments")
	}
	if len(call.Args) == 0 {
		return func(http.ResponseWriter, *http.Request) []reflect.Value {
			return nil
		}, nil
	}
	for _, pp := range pathParams {
		if slices.Contains(rootScope(), pp) {
			return nil, fmt.Errorf("identfier %s is already defined", pp)
		}
	}
	var args []string
	for i, exp := range call.Args {
		arg, err := typeCheckMethodParameters(pathParams, method.In(i), exp)
		if err != nil {
			return nil, fmt.Errorf("method argument at index %d: %w", i, err)
		}
		args = append(args, arg)
	}
	return func(res http.ResponseWriter, req *http.Request) []reflect.Value {
		var in []reflect.Value
		for _, arg := range args {
			switch arg {
			case requestArgumentIdentifier:
				in = append(in, reflect.ValueOf(req))
			case contextArgumentIdentifier:
				in = append(in, reflect.ValueOf(req.Context()))
			case responseArgumentIdentifier:
				in = append(in, reflect.ValueOf(res))
			case loggerArgumentIdentifier:
				in = append(in, reflect.ValueOf(o.logger))
			case loggerArgumentTemplate:
				in = append(in, reflect.ValueOf(t))
			default:
				if slices.Index(pathParams, arg) >= 0 {
					in = append(in, reflect.ValueOf(req.PathValue(arg)))
				}
			}
		}
		return in
	}, nil
}

var argumentType = sync.OnceValue(func() func(argName string, pathParams []string) (reflect.Type, error) {
	requestType := reflect.TypeFor[*http.Request]()
	contextType := reflect.TypeFor[context.Context]()
	responseType := reflect.TypeFor[http.ResponseWriter]()
	loggerType := reflect.TypeFor[*slog.Logger]()
	templateType := reflect.TypeFor[*template.Template]()
	stringType := reflect.TypeFor[string]()
	return func(argName string, pathParams []string) (reflect.Type, error) {
		var argType reflect.Type
		switch argName {
		case requestArgumentIdentifier:
			argType = requestType
		case contextArgumentIdentifier:
			argType = contextType
		case responseArgumentIdentifier:
			argType = responseType
		case loggerArgumentIdentifier:
			argType = loggerType
		case loggerArgumentTemplate:
			argType = templateType
		default:
			if !slices.Contains(pathParams, argName) {
				return nil, fmt.Errorf("unknown argument type for %s", argName)
			}
			argType = stringType
		}
		return argType, nil
	}
})

func typeCheckMethodParameters(pathParams []string, paramType reflect.Type, exp ast.Expr) (string, error) {
	switch arg := exp.(type) {
	case *ast.Ident:
		argType, err := argumentType()(arg.Name, pathParams)
		if err != nil {
			return arg.Name, err
		}
		if !argType.AssignableTo(paramType) {
			return arg.Name, fmt.Errorf("argument %s %s is not assignable to parameter type %s", arg.Name, argType, paramType)
		}
		return arg.Name, nil
	default:
		return "", fmt.Errorf("argument is not an identifier got %s", printNode(exp))
	}
}

func simpleTemplateHandler(ex ExecuteFunc[any], t *template.Template, logger *slog.Logger) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		ex(res, req, t, logger, req)
	}
}

type ExecuteFunc[T any] func(http.ResponseWriter, *http.Request, *template.Template, *slog.Logger, T)

func defaultExecute(res http.ResponseWriter, req *http.Request, t *template.Template, logger *slog.Logger, data any) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		logger.Error("failed to render page", "method", req.Method, "path", req.URL.Path, "error", err)
		http.Error(res, "failed to render page", http.StatusInternalServerError)
		return
	}
	if _, err := buf.WriteTo(res); err != nil {
		logger.Error("failed to write full response", "method", req.Method, "path", req.URL.Path, "error", err)
		return
	}
}
