package muxt

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"reflect"
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
		pat, err, match := NewPattern(t.Name())
		if !match {
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to parse NewPattern for template %q: %w", t.Name(), err)
		}
		if pat.Handler == "" {
			mux.HandleFunc(pat.Pattern, simpleTemplateHandler(o.execute, t, o.logger))
			continue
		}
		handler, err := pat.ParseHandler()
		if err != nil {
			return err
		}
		h, err := newReflectHandlerFunc(o, t, handler, handler.Ident)
		if err != nil {
			return fmt.Errorf("failed to create handler for %q: %w", pat.String(), err)
		}
		mux.HandleFunc(pat.Pattern, h)
	}
	return nil
}

func newReflectHandlerFunc(o *Options, t *template.Template, h *Handler, method *ast.Ident) (http.HandlerFunc, error) {
	m, err := serviceMethod(o, h.Call, method)
	if err != nil {
		return nil, err
	}
	inputs, err := generateInputsFunction(o, t, m.Type(), h)
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

func generateInputsFunction(o *Options, t *template.Template, method reflect.Type, h *Handler) (inputsFunc, error) {
	if method.NumIn() != len(h.Args) {
		return nil, fmt.Errorf("wrong number of arguments")
	}
	if len(h.Args) == 0 {
		return func(http.ResponseWriter, *http.Request) []reflect.Value {
			return nil
		}, nil
	}
	var args []string
	for i, exp := range h.Args {
		arg, err := typeCheckMethodParameters(method.In(i), exp)
		if err != nil {
			return nil, fmt.Errorf("method argument at index %d: %w", i, err)
		}
		args = append(args, arg)
	}
	return func(res http.ResponseWriter, req *http.Request) []reflect.Value {
		var in []reflect.Value
		for _, arg := range args {
			switch arg {
			case PatternScopeIdentifierHTTPResponse:
				in = append(in, reflect.ValueOf(res))
			case PatternScopeIdentifierHTTPRequest:
				in = append(in, reflect.ValueOf(req))
			case PatternScopeIdentifierContext:
				in = append(in, reflect.ValueOf(req.Context()))
			case PatternScopeIdentifierLogger:
				in = append(in, reflect.ValueOf(o.logger))
			case PatternScopeIdentifierTemplate:
				in = append(in, reflect.ValueOf(t))
			default:
				in = append(in, reflect.ValueOf(req.PathValue(arg)))
			}
		}
		return in
	}, nil
}

var argumentType = sync.OnceValue(func() func(argName string) (reflect.Type, error) {
	requestType := reflect.TypeFor[*http.Request]()
	contextType := reflect.TypeFor[context.Context]()
	responseType := reflect.TypeFor[http.ResponseWriter]()
	loggerType := reflect.TypeFor[*slog.Logger]()
	templateType := reflect.TypeFor[*template.Template]()
	stringType := reflect.TypeFor[string]()
	return func(argName string) (reflect.Type, error) {
		var argType reflect.Type
		switch argName {
		case PatternScopeIdentifierHTTPRequest:
			argType = requestType
		case PatternScopeIdentifierContext:
			argType = contextType
		case PatternScopeIdentifierHTTPResponse:
			argType = responseType
		case PatternScopeIdentifierLogger:
			argType = loggerType
		case PatternScopeIdentifierTemplate:
			argType = templateType
		default:
			argType = stringType
		}
		return argType, nil
	}
})

func typeCheckMethodParameters(paramType reflect.Type, arg *ast.Ident) (string, error) {
	argType, err := argumentType()(arg.Name)
	if err != nil {
		return arg.Name, err
	}
	if !argType.AssignableTo(paramType) {
		return arg.Name, fmt.Errorf("argument %s %s is not assignable to parameter type %s", arg.Name, argType, paramType)
	}
	return arg.Name, nil
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
