package muxt_test

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/crhntr/dom/domtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html/atom"

	"github.com/crhntr/muxt"
	"github.com/crhntr/muxt/internal/example"
	"github.com/crhntr/muxt/internal/fake"
)

//go:generate counterfeiter -generate
//counterfeiter:generate -o ./internal/fake/receiver.go --fake-name Receiver . receiver
var _ receiver = (*fake.Receiver)(nil)

//counterfeiter:generate -o ./internal/fake/response_writer.go --fake-name ResponseWriter net/http.ResponseWriter

type (
	receiver interface {
		ListArticles(ctx context.Context) ([]example.Article, error)
		ToUpper(in ...rune) string
		Parse(string) []string
		GetComment(ctx context.Context, articleID, commentID int) (string, error)
		SomeString(ctx context.Context, x string) (string, error)
		TooManyResults() (int, int, int)
		NumAuthors() int
		CheckAuth(req *http.Request) (string, error)
		Handler(http.ResponseWriter, *http.Request) template.HTML
		ErrorHandler(http.ResponseWriter, *http.Request) (template.HTML, error)
		LogLines(*slog.Logger) int
		Template(*template.Template) template.HTML
		Type(any) string
		Tuple() (string, string)
	}
)

func TestRoutes(t *testing.T) {
	t.Run("GET index", func(t *testing.T) {
		//
		ts := template.Must(template.New("simple path").Parse(
			/* language=gotemplate */
			`{{define "GET /" }}<h1>Hello, friend!</h1>{{end}}`,
		))
		mux := http.NewServeMux()
		err := muxt.Handlers(mux, ts)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		res := rec.Result()

		assert.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("when a handler is registered", func(t *testing.T) {
		ts := template.Must(template.New("simple path").Parse(
			/* language=gotemplate */
			`{{define "GET /articles ListArticles(ctx)" }}<ul>{{range .}}<li data-id="{{.ID}}">{{.Title}}</li>{{end}}</ul>{{end}}`,
		))
		as := new(fake.Receiver)
		articles := []example.Article{
			{ID: 1, Title: "Hello"},
			{ID: 2, Title: "Goodbye"},
		}
		as.ListArticlesReturns(articles, nil)
		mux := http.NewServeMux()
		err := muxt.Handlers(mux, ts, muxt.WithReceiver(as))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/articles", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		assert.Equal(t, 1, as.ListArticlesCallCount())

		res := rec.Result()

		assert.Equal(t, http.StatusOK, res.StatusCode)
		fragment := domtest.DocumentFragmentResponse(t, res, atom.Body)
		listItems := fragment.QuerySelectorAll(`[data-id]`)
		assert.Equal(t, len(articles), listItems.Length())
		for i := 0; i < listItems.Length(); i++ {
			li := listItems.Item(i)
			assert.Equal(t, articles[i].Title, li.TextContent())
			assert.Equal(t, strconv.Itoa(articles[i].ID), li.GetAttribute("data-id"))
		}
	})

	t.Run("unexpected method", func(t *testing.T) {
		//
		ts := template.Must(template.New("simple path").Parse(`{{define "CONNECT /articles" }}{{.}}{{end}}`))
		mux := http.NewServeMux()
		err := muxt.Handlers(mux, ts)
		require.ErrorContains(t, err, `failed to parse NewPattern for template "CONNECT /articles": CONNECT method not allowed`)
	})

	t.Run("no method", func(t *testing.T) {
		//
		ts := template.Must(template.New("simple path").Parse(`{{define "/x/y" }}{{.Method}}{{end}}`))
		mux := http.NewServeMux()
		err := muxt.Handlers(mux, ts)
		require.NoError(t, err)

		for _, method := range []string{http.MethodGet, http.MethodPost} {
			req := httptest.NewRequest(method, "/x/y", nil)
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)
			res := rec.Result()

			assert.Equal(t, http.StatusOK, res.StatusCode)
			body, _ := io.ReadAll(res.Body)
			assert.Equal(t, method, string(body))
		}
	})

	t.Run("selector must be an expression", func(t *testing.T) {
		ts := template.Must(template.New("simple path").Parse(`{{define "GET / var x int" }}{{.}}{{end}}`))
		mux := http.NewServeMux()
		err := muxt.Handlers(mux, ts, muxt.WithReceiver(new(fake.Receiver)))
		require.ErrorContains(t, err, "failed to parse handler expression")
	})

	t.Run("function must be an identifier", func(t *testing.T) {
		ts := template.Must(template.New("simple path").Parse(`{{define "GET / func().Method(req)" }}{{.}}{{end}}`))
		mux := http.NewServeMux()
		rec := new(fake.Receiver)
		err := muxt.Handlers(mux, ts, muxt.WithReceiver(rec))
		require.ErrorContains(t, err, `expected function identifier`)
	})

	t.Run("receiver is nil and a method is expected", func(t *testing.T) {
		ts := template.Must(template.New("simple path").Parse(`{{define "GET / Method(req)" }}{{.}}{{end}}`))
		mux := http.NewServeMux()
		err := muxt.Handlers(mux, ts)
		require.ErrorContains(t, err, "receiver is nil")
	})

	t.Run("method not found on basic type", func(t *testing.T) {
		ts := template.Must(template.New("simple path").Parse(`{{define "GET / Foo(req)" }}{{.}}{{end}}`))
		mux := http.NewServeMux()
		err := muxt.Handlers(mux, ts, muxt.WithReceiver(100))
		require.ErrorContains(t, err, `method Foo not found on int`)
	})

	t.Run("ellipsis not allowed", func(t *testing.T) {
		ts := template.Must(template.New("simple path").Parse(`{{define "GET /{name} ToUpper(name...)" }}{{.}}{{end}}`))
		mux := http.NewServeMux()
		s := new(fake.Receiver)
		err := muxt.Handlers(mux, ts, muxt.WithReceiver(s))
		require.ErrorContains(t, err, `ellipsis call not allowed`)
	})

	t.Run("duplicate path param identifier", func(t *testing.T) {
		ts := template.Must(template.New("simple path").Parse(`{{define "GET /articles/{id}/comment/{id} GetComment(ctx, id, id)" }}{{.}}{{end}}`))
		mux := http.NewServeMux()
		s := new(fake.Receiver)
		err := muxt.Handlers(mux, ts, muxt.WithReceiver(s))
		require.ErrorContains(t, err, `path parameter id defined at least twice`)
	})

	t.Run("path param is not an identifier ", func(t *testing.T) {
		ts := template.Must(template.New("simple path").Parse(`{{define "GET /{key-id} SomeString(ctx, key-id)"}}KEY{{end}}`))
		mux := http.NewServeMux()
		s := new(fake.Receiver)
		err := muxt.Handlers(mux, ts, muxt.WithReceiver(s))
		require.ErrorContains(t, err, `path parameter name not permitted: "key-id" is not a Go identifier`)
	})

	for _, name := range []string{
		"request",
		"ctx",
		"response",
		"logger",
		"template",
	} {
		t.Run(name+" can not be used as a path parameter identifier", func(t *testing.T) {
			ts := template.Must(template.New("simple path").Parse(fmt.Sprintf(`{{define "GET /{%[1]s} Type(%[1]s)"}}{{.}}{{end}}`, name)))
			mux := http.NewServeMux()
			s := new(fake.Receiver)
			err := muxt.Handlers(mux, ts, muxt.WithReceiver(s))
			require.ErrorContains(t, err, fmt.Sprintf(`identfier %s is already defined`, name))
		})

		t.Run(name+" can be used when no handler is defined", func(t *testing.T) {
			ts := template.Must(template.New("simple path").Parse(fmt.Sprintf(`{{define "GET /{%[1]s}"}}{{.}}{{end}}`, name)))
			mux := http.NewServeMux()
			s := new(fake.Receiver)
			err := muxt.Handlers(mux, ts, muxt.WithReceiver(s))
			require.NoError(t, err)
		})
	}

	t.Run("template execution fails", func(t *testing.T) {
		ts := template.Must(template.New("simple path").Funcs(template.FuncMap{
			"errorNow": func() (string, error) { return "", fmt.Errorf("BANANA") },
		}).Parse(`{{define "GET / ListArticles(ctx)"}}{{ errorNow }}{{end}}`))
		mux := http.NewServeMux()
		s := new(fake.Receiver)
		err := muxt.Handlers(mux, ts, muxt.WithReceiver(s))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		res := rec.Result()
		assert.Equal(t, http.StatusInternalServerError, res.StatusCode)
	})

	t.Run("write fails", func(t *testing.T) {
		ts := template.Must(template.New("simple path").Parse(`{{define "GET /"}}{{printf "%d" 199}}{{end}}`))
		logBuffer := bytes.NewBuffer(nil)
		logger := slog.New(slog.NewTextHandler(logBuffer, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		mux := http.NewServeMux()
		err := muxt.Handlers(mux, ts, muxt.WithStructuredLogger(logger))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(errorWriter{ResponseWriter: rec}, req)
		res := rec.Result()
		assert.Equal(t, http.StatusOK, res.StatusCode)
		assert.Contains(t, logBuffer.String(), "failed to write full response")
	})

	t.Run("too many results", func(t *testing.T) {
		ts := template.Must(template.New("simple path").Parse(`{{define "GET / TooManyResults()" }}{{.}}{{end}}"`))
		mux := http.NewServeMux()
		err := muxt.Handlers(mux, ts, muxt.WithReceiver(new(fake.Receiver)))
		require.ErrorContains(t, err, `method must either return (T) or (T, error)`)
	})

	t.Run("call fails", func(t *testing.T) {
		ts := template.Must(template.New("simple path").Parse(`{{define "GET /number-of-articles ListArticles(ctx)"}}{{len .}}{{end}}`))
		logBuffer := bytes.NewBuffer(nil)
		logger := slog.New(slog.NewTextHandler(logBuffer, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		mux := http.NewServeMux()
		s := new(fake.Receiver)
		s.ListArticlesReturns(nil, fmt.Errorf("banana"))
		err := muxt.Handlers(mux, ts, muxt.WithReceiver(s).WithStructuredLogger(logger))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/number-of-articles", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		res := rec.Result()
		assert.Equal(t, http.StatusInternalServerError, res.StatusCode)

		body, _ := io.ReadAll(res.Body)
		assert.NotContains(t, string(body), "banana")

		assert.Contains(t, logBuffer.String(), "banana")
	})

	t.Run("not a function call", func(t *testing.T) {
		ts := template.Must(template.New("simple path").Parse(`{{define "GET /number-of-articles <-c"}}{{.}}{{end}}`))
		mux := http.NewServeMux()
		s := new(fake.Receiver)
		err := muxt.Handlers(mux, ts, muxt.WithReceiver(s))
		require.ErrorContains(t, err, "expected call")
	})

	t.Run("single return", func(t *testing.T) {
		ts := template.Must(template.New("simple path").Parse(`{{define "GET /number-of-authors NumAuthors()"}}{{.}}{{end}}`))
		mux := http.NewServeMux()
		s := new(fake.Receiver)
		s.NumAuthorsReturns(234)
		err := muxt.Handlers(mux, ts, muxt.WithReceiver(s))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/number-of-authors", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		res := rec.Result()
		assert.Equal(t, http.StatusOK, res.StatusCode)

		body, _ := io.ReadAll(res.Body)

		assert.Equal(t, string(body), "234")
	})

	t.Run("request as a parameter", func(t *testing.T) {
		ts := template.Must(template.New("simple path").Parse(`{{define "GET /auth CheckAuth(request)"}}OK{{end}}`))
		mux := http.NewServeMux()
		s := new(fake.Receiver)
		s.NumAuthorsReturns(234)
		err := muxt.Handlers(mux, ts, muxt.WithReceiver(s))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/auth", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		res := rec.Result()
		assert.Equal(t, http.StatusOK, res.StatusCode)

		body, _ := io.ReadAll(res.Body)

		assert.Equal(t, string(body), "OK")
	})

	t.Run("non identifier params", func(t *testing.T) {
		ts := template.Must(template.New("simple path").Parse(`{{define "GET /site-owner GetComment(ctx, 3, 1+2)"}}OK{{end}}`))
		mux := http.NewServeMux()
		s := new(fake.Receiver)
		s.NumAuthorsReturns(234)
		err := muxt.Handlers(mux, ts, muxt.WithReceiver(s))
		require.ErrorContains(t, err, `method argument at index 1: argument is not an identifier got 3`)
	})

	t.Run("query param", func(t *testing.T) {
		ts := template.Must(template.New("simple path").Parse(`{{define "GET /input/{in} Parse(in)"}}{{.}}{{end}}`))
		mux := http.NewServeMux()
		s := new(fake.Receiver)
		s.NumAuthorsReturns(234)
		err := muxt.Handlers(mux, ts, muxt.WithReceiver(s))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/input/peach", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		v := s.ParseArgsForCall(0)
		assert.Equal(t, "peach", v)
	})

	t.Run("unknown identifier", func(t *testing.T) {
		ts := template.Must(template.New("simple path").Parse(`{{define "GET / Parse(enemy)"}}@{{end}}`))
		mux := http.NewServeMux()
		s := new(fake.Receiver)
		s.NumAuthorsReturns(234)
		err := muxt.Handlers(mux, ts, muxt.WithReceiver(s))
		require.ErrorContains(t, err, `method argument at index 0: unknown argument type for enemy`)
	})

	t.Run("full handler func signature", func(t *testing.T) {
		ts := template.Must(template.New("simple path").Parse(`{{define "GET / Handler(response, request)"}}{{.}}{{end}}`))
		logBuffer := bytes.NewBuffer(nil)
		logger := slog.New(slog.NewTextHandler(logBuffer, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		mux := http.NewServeMux()
		s := new(fake.Receiver)

		s.HandlerStub = func(writer http.ResponseWriter, request *http.Request) template.HTML {
			writer.WriteHeader(http.StatusCreated)

			return "<strong>Progressive</strong>"
		}

		err := muxt.Handlers(mux, ts, muxt.WithReceiver(s).WithStructuredLogger(logger))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/input/peach", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		res := rec.Result()
		assert.Equal(t, http.StatusCreated, res.StatusCode)
	})

	t.Run("method receives a template", func(t *testing.T) {
		ts := template.Must(template.New("simple path").Parse(`{{define "GET / Template(template)"}}{{.}}{{end}}`))
		mux := http.NewServeMux()
		s := new(fake.Receiver)

		err := muxt.Handlers(mux, ts, muxt.WithReceiver(s))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/input/peach", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		res := rec.Result()
		assert.Equal(t, http.StatusOK, res.StatusCode)

		if assert.Equal(t, 1, s.TemplateCallCount()) {
			arg := s.TemplateArgsForCall(0)
			assert.Equal(t, "GET / Template(template)", arg.Name())
		}
	})

	t.Run("wrong parameter type", func(t *testing.T) {
		ts := template.Must(template.New("simple path").Parse(`{{define "GET / Template(request)"}}{{.}}{{end}}`))
		mux := http.NewServeMux()
		s := new(fake.Receiver)

		err := muxt.Handlers(mux, ts, muxt.WithReceiver(s))
		require.ErrorContains(t, err, "method argument at index 0: argument request *http.Request is not assignable to parameter type *template.Template")
	})

	t.Run("handler uses a logger", func(t *testing.T) {
		ts := template.Must(template.New("simple path").Parse(`{{define "POST /stdin LogLines(logger)"}}{{printf "lines: %d" .}}{{end}}`))
		logBuffer := bytes.NewBuffer(nil)
		logger := slog.New(slog.NewTextHandler(logBuffer, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		mux := http.NewServeMux()
		s := new(fake.Receiver)

		s.LogLinesStub = func(logger *slog.Logger) int {
			logger.Info("some message")
			return 42
		}

		err := muxt.Handlers(mux, ts, muxt.WithStructuredLogger(logger).WithReceiver(s))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/stdin", strings.NewReader(""))
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		res := rec.Result()
		assert.Equal(t, http.StatusOK, res.StatusCode)

		assert.Contains(t, logBuffer.String(), "some message")
	})

	t.Run("wrong number of arguments", func(t *testing.T) {
		ts := template.Must(template.New("simple path").Parse(`{{define "POST /stdin LogLines(ctx, logger)"}}{{printf "lines: %d" .}}{{end}}`))
		mux := http.NewServeMux()
		err := muxt.Handlers(mux, ts, muxt.WithReceiver(new(fake.Receiver)))
		require.ErrorContains(t, err, "wrong number of arguments")
	})

	t.Run("custom execute function", func(t *testing.T) {
		//
		ts := template.Must(template.New("simple path").Parse(
			/* language=gotemplate */
			`{{define "GET /" }}<h1>Hello, friend!</h1>{{end}}`,
		))
		mux := http.NewServeMux()
		err := muxt.Handlers(mux, ts, muxt.WithDataFunc(func(res http.ResponseWriter, req *http.Request, t *template.Template, logger *slog.Logger, data any) {
			res.WriteHeader(http.StatusBadRequest)
		}))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		res := rec.Result()

		assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	})
	t.Run("custom execute function", func(t *testing.T) {
		//
		ts := template.Must(template.New("simple path").Parse(
			`{{define "GET / Tuple()" }}<h1>{{.}}</h1>{{end}}`,
		))
		mux := http.NewServeMux()
		err := muxt.Handlers(mux, ts, muxt.WithReceiver(new(fake.Receiver)))
		require.ErrorContains(t, err, "the second result must be an error")
	})

	t.Run("when the error handler is overwritten", func(t *testing.T) {
		//
		ts := template.Must(template.New("simple path").Parse(
			`{{define "GET / ListArticles(ctx)" }}<h1>{{len .}}</h1>{{end}}`,
		))
		mux := http.NewServeMux()
		s := new(fake.Receiver)
		listErr := fmt.Errorf("banana")
		s.ListArticlesReturns(nil, listErr)
		const userFacingError = "🍌"
		err := muxt.Handlers(mux, ts, muxt.WithErrorFunc(func(res http.ResponseWriter, req *http.Request, ts *template.Template, logger *slog.Logger, err error) {
			assert.Equal(t, "GET / ListArticles(ctx)", ts.Name())
			assert.NotNil(t, logger)
			assert.Error(t, err)
			assert.Equal(t, err, listErr)
			res.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(res, userFacingError)
		}).WithReceiver(s))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		res := rec.Result()
		assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("when the noop handler error func is configures", func(t *testing.T) {
		//
		ts := template.Must(template.New("simple path").Parse(
			`{{define "GET / ErrorHandler(response, request)" }}{{.}}{{end}}`,
		))
		mux := http.NewServeMux()
		s := new(fake.Receiver)

		const body = `<p id="error">Excuse You</p>`
		s.ErrorHandlerStub = func(res http.ResponseWriter, _ *http.Request) (template.HTML, error) {
			res.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(res, body)
			return "", fmt.Errorf("banana")
		}

		logBuffer := bytes.NewBuffer(nil)
		logger := slog.New(slog.NewTextHandler(logBuffer, &slog.HandlerOptions{Level: slog.LevelDebug}))

		err := muxt.Handlers(mux, ts, muxt.WithNoopErrorFunc().WithReceiver(s).WithStructuredLogger(logger))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		res := new(fake.ResponseWriter)
		mux.ServeHTTP(res, req)

		assert.Equal(t, 1, res.WriteHeaderCallCount())
		assert.Equal(t, http.StatusBadRequest, res.WriteHeaderArgsForCall(0))
		assert.Equal(t, body, string(res.WriteArgsForCall(0)))
		assert.Empty(t, logBuffer.String())
	})

	t.Run("when the 500 handler error func is configured", func(t *testing.T) {
		//
		ts := template.Must(template.New("simple path").Parse(
			`{{define "GET / ErrorHandler(response, request)" }}{{.}}{{end}}`,
		))
		mux := http.NewServeMux()
		s := new(fake.Receiver)

		s.ErrorHandlerStub = func(res http.ResponseWriter, _ *http.Request) (template.HTML, error) {
			return "", fmt.Errorf("banana")
		}

		logBuffer := bytes.NewBuffer(nil)
		logger := slog.New(slog.NewTextHandler(logBuffer, &slog.HandlerOptions{Level: slog.LevelDebug}))

		err := muxt.Handlers(mux, ts, muxt.With500ErrorFunc().WithReceiver(s).WithStructuredLogger(logger))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		res := new(fake.ResponseWriter)
		res.HeaderReturns(make(http.Header))
		mux.ServeHTTP(res, req)

		assert.Equal(t, 1, res.WriteHeaderCallCount())
		assert.Equal(t, http.StatusInternalServerError, res.WriteHeaderArgsForCall(0))
		assert.Equal(t, http.StatusText(http.StatusInternalServerError)+"\n", string(res.WriteArgsForCall(0)))
		assert.Contains(t, logBuffer.String(), "error=banana")
	})

	t.Run("when the path has an end of path delimiter", func(t *testing.T) {
		//
		ts := template.Must(template.New("simple path").Parse(
			`{{define "GET /{$} ListArticles(ctx)" }}{{len .}}{{end}}`,
		))
		mux := http.NewServeMux()
		s := new(fake.Receiver)

		err := muxt.Handlers(mux, ts, muxt.WithReceiver(s))
		require.NoError(t, err)
	})
}

type errorWriter struct {
	http.ResponseWriter
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("banna")
}
