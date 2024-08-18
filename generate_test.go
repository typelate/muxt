package muxt_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"html/template"
	"io"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/txtar"

	"github.com/crhntr/muxt"
	"github.com/crhntr/muxt/internal/source"
)

func TestGenerate(t *testing.T) {
	for _, tt := range []struct {
		Name            string
		Templates       string
		Receiver        string
		ReceiverPackage string
		PackageName     string
		TemplatesVar    string
		RoutesFunc      string
		Imports         []string
		Method          *ast.FuncType

		ExpectedError string
		ExpectedFile  string
	}{
		{
			Name:      "simple",
			Templates: `{{define "GET /"}}Hello, world!{{end}}`,
			ExpectedFile: `package main

import (
	"net/http"
	"bytes"
	"html/template"
)

type RoutesReceiver interface {
}

func Routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /", func(response http.ResponseWriter, request *http.Request) {
		execute(response, request, templates.Lookup("GET /"), http.StatusOK, request)
	})
}
func execute(response http.ResponseWriter, request *http.Request, t *template.Template, code int, data any) {
	buf := bytes.NewBuffer(nil)
	if err := t.Execute(buf, data); err != nil {
		http.Error(response, err.Error(), http.StatusOK)
		return
	}
	response.WriteHeader(code)
	_, _ = buf.WriteTo(response)
}
`,
		},
		{
			Name:      "simple call",
			Templates: `{{define "GET / F()"}}Hello, world!{{end}}`,
			ExpectedFile: `package main

import (
	"net/http"
	"bytes"
	"html/template"
)

type RoutesReceiver interface {
	F() any
}

func Routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /", func(response http.ResponseWriter, request *http.Request) {
		data := receiver.F()
		execute(response, request, templates.Lookup("GET / F()"), http.StatusOK, data)
	})
}
func execute(response http.ResponseWriter, request *http.Request, t *template.Template, code int, data any) {
	buf := bytes.NewBuffer(nil)
	if err := t.Execute(buf, data); err != nil {
		http.Error(response, err.Error(), http.StatusOK)
		return
	}
	response.WriteHeader(code)
	_, _ = buf.WriteTo(response)
}
`,
		},
		{
			Name:      "multiple arguments no static receiver",
			Templates: `{{define "GET /project/{projectID}/task/{taskID} F(ctx, response, request, projectID, taskID)"}}Hello, world!{{end}}`,
			ExpectedFile: `package main

import (
	"context"
	"net/http"
	"bytes"
	"html/template"
)

type RoutesReceiver interface {
	F(ctx context.Context, response http.ResponseWriter, request *http.Request, projectID string, taskID string) any
}

func Routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /project/{projectID}/task/{taskID}", func(response http.ResponseWriter, request *http.Request) {
		ctx := request.Context()
		projectID := request.PathValue("projectID")
		taskID := request.PathValue("taskID")
		data := receiver.F(ctx, response, request, projectID, taskID)
		execute(response, request, templates.Lookup("GET /project/{projectID}/task/{taskID} F(ctx, response, request, projectID, taskID)"), http.StatusOK, data)
	})
}
func execute(response http.ResponseWriter, request *http.Request, t *template.Template, code int, data any) {
	buf := bytes.NewBuffer(nil)
	if err := t.Execute(buf, data); err != nil {
		http.Error(response, err.Error(), http.StatusOK)
		return
	}
	response.WriteHeader(code)
	_, _ = buf.WriteTo(response)
}
`,
		},
		{
			Name:      "simple call with static one argument",
			Templates: `{{define "GET /age/{username} F(username)"}}Hello, {{.}}!{{end}}`,
			ReceiverPackage: `
-- receiver.go --
package main

type T struct{}

func (T) F(username string) int { return 30 }
`,
			Receiver: "T",
			ExpectedFile: `package main

import (
	"net/http"
	"bytes"
	"html/template"
)

type RoutesReceiver interface {
	F(username string) int
}

func Routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /age/{username}", func(response http.ResponseWriter, request *http.Request) {
		username := request.PathValue("username")
		data := receiver.F(username)
		execute(response, request, templates.Lookup("GET /age/{username} F(username)"), http.StatusOK, data)
	})
}
func execute(response http.ResponseWriter, request *http.Request, t *template.Template, code int, data any) {
	buf := bytes.NewBuffer(nil)
	if err := t.Execute(buf, data); err != nil {
		http.Error(response, err.Error(), http.StatusOK)
		return
	}
	response.WriteHeader(code)
	_, _ = buf.WriteTo(response)
}
`,
		},
		{
			Name:      "error wrong argument type",
			Templates: `{{define "GET / F(request)"}}Hello, world!{{end}}`,
			ReceiverPackage: `
-- receiver.go --
package main

type T struct{}

func (T) F(ctx context.Context) int { return 30 }
`,
			Receiver:      "T",
			ExpectedError: "method expects type context.Context but request is *http.Request",
		},
		{
			Name:      "simple call larger receiver with larger package",
			Templates: `{{define "GET /age/{username} F(ctx, username)"}}Hello, {{.}}!{{end}}`,
			ReceiverPackage: `
-- receiver.go --
package main

type (
	T0 struct{}

	T struct{}
)

-- f.go --
package main

import "context"

func F(string) int { return 20 }

func (T0) F(ctx context.Context) int { return 30 }

func (T) F1(ctx context.Context, username string) int { return 30 }

func (T) F(ctx context.Context, username string) int { return 30 }
`,
			Receiver: "T",
			ExpectedFile: `package main

import (
	"context"
	"net/http"
	"bytes"
	"html/template"
)

type RoutesReceiver interface {
	F(ctx context.Context, username string) int
}

func Routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /age/{username}", func(response http.ResponseWriter, request *http.Request) {
		ctx := request.Context()
		username := request.PathValue("username")
		data := receiver.F(ctx, username)
		execute(response, request, templates.Lookup("GET /age/{username} F(ctx, username)"), http.StatusOK, data)
	})
}
func execute(response http.ResponseWriter, request *http.Request, t *template.Template, code int, data any) {
	buf := bytes.NewBuffer(nil)
	if err := t.Execute(buf, data); err != nil {
		http.Error(response, err.Error(), http.StatusOK)
		return
	}
	response.WriteHeader(code)
	_, _ = buf.WriteTo(response)
}
`,
		},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			ts := template.Must(template.New(tt.Name).Parse(tt.Templates))
			patterns, err := muxt.TemplatePatterns(ts)
			require.NoError(t, err)
			logs := log.New(io.Discard, "", 0)
			set := token.NewFileSet()
			goFiles := methodFuncTypeLoader(t, set, tt.ReceiverPackage)
			out, err := muxt.Generate(patterns, tt.PackageName, tt.TemplatesVar, tt.RoutesFunc, tt.Receiver, set, goFiles, goFiles, logs)
			if tt.ExpectedError == "" {
				assert.NoError(t, err)
				assert.Equal(t, tt.ExpectedFile, out)
			} else {
				assert.ErrorContains(t, err, tt.ExpectedError)
			}
		})
	}
}

func methodFuncTypeLoader(t *testing.T, set *token.FileSet, in string) source.GoFiles {
	t.Helper()
	archive := txtar.Parse([]byte(in))
	var files []*ast.File
	for _, file := range archive.Files {
		f, err := parser.ParseFile(set, file.Name, file.Data, parser.AllErrors)
		require.NoError(t, err)
		files = append(files, f)
	}
	return files
}
