package muxt_test

import (
	"cmp"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/txtar"

	"github.com/crhntr/muxt/internal/muxt"
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
		Interface       string

		ExpectedError string
		ExpectedFile  string
	}{
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
	"bytes"
	"net/http"
)

type RoutesReceiver interface {
	F(username string) int
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /age/{username}", func(response http.ResponseWriter, request *http.Request) {
		type ResponseData struct {
			Data    int
			Request *http.Request
		}
		username := request.PathValue("username")
		data := receiver.F(username)
		buf := bytes.NewBuffer(nil)
		rd := ResponseData{Data: data, Request: request}
		if err := templates.ExecuteTemplate(buf, "GET /age/{username} F(username)", rd); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
}
`,
		},
		{
			Name:      "when the default interface name is overwritten",
			Templates: `{{define "GET / F()"}}Hello{{end}}`,
			Interface: "Server",
			ExpectedFile: `package main

import (
	"bytes"
	"net/http"
)

type Server interface {
	F() any
}

func routes(mux *http.ServeMux, receiver Server) {
	mux.HandleFunc("GET /", func(response http.ResponseWriter, request *http.Request) {
		type ResponseData struct {
			Data    any
			Request *http.Request
		}
		data := receiver.F()
		buf := bytes.NewBuffer(nil)
		rd := ResponseData{Data: data, Request: request}
		if err := templates.ExecuteTemplate(buf, "GET / F()", rd); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
}
`,
		},
		{
			Name:      "F returns a value and a boolean",
			Templates: `{{define "GET /age/{username} F(username)"}}Hello, {{.}}!{{end}}`,
			ReceiverPackage: `
-- receiver.go --
package main

type T struct{}

func (T) F(username string) (int, bool) { return 30, true }
`,
			Receiver: "T",
			ExpectedFile: `package main

import (
	"bytes"
	"net/http"
)

type RoutesReceiver interface {
	F(username string) (int, bool)
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /age/{username}", func(response http.ResponseWriter, request *http.Request) {
		type ResponseData struct {
			Data    bool
			Request *http.Request
		}
		username := request.PathValue("username")
		data, ok := receiver.F(username)
		if !ok {
			return
		}
		buf := bytes.NewBuffer(nil)
		rd := ResponseData{Data: data, Request: request}
		if err := templates.ExecuteTemplate(buf, "GET /age/{username} F(username)", rd); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
}
`,
		},
		{
			Name:      "F returns a value and an unsupported type",
			Templates: `{{define "GET /{$} F()"}}Hello, {{.}}!{{end}}`,
			ReceiverPackage: `
-- receiver.go --
package main

type T struct{}

func (T) F() (int, float64) { return 30, true }
`,
			Receiver:      "T",
			ExpectedError: "expected last result to be either an error or a bool",
		},
		{
			Name:      "F returns a value and an unsupported type",
			Templates: `{{define "GET /{$} F()"}}Hello, {{.}}!{{end}}`,
			ReceiverPackage: `
-- receiver.go --
package main

type T struct{}

func (T) F() (int, []error) { return 30, nil }
`,
			Receiver:      "T",
			ExpectedError: "expected last result to be either an error or a bool",
		},
		{
			Name:      "method receiver is a pointer",
			Templates: `{{define "GET /age/{username} F(username)"}}Hello, {{.}}!{{end}}`,
			ReceiverPackage: `
-- receiver.go --
package main

type T struct{}

func (*T) F(username string) int { return 30 }

`,
			Receiver: "T",
			ExpectedFile: `package main

import (
	"bytes"
	"net/http"
)

type RoutesReceiver interface {
	F(username string) int
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /age/{username}", func(response http.ResponseWriter, request *http.Request) {
		type ResponseData struct {
			Data    int
			Request *http.Request
		}
		username := request.PathValue("username")
		data := receiver.F(username)
		buf := bytes.NewBuffer(nil)
		rd := ResponseData{Data: data, Request: request}
		if err := templates.ExecuteTemplate(buf, "GET /age/{username} F(username)", rd); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
}
`,
		},
		{
			Name:      "call method with two returns",
			Templates: `{{define "GET /age/{username} F(username)"}}Hello, {{.}}!{{end}}`,
			ReceiverPackage: `
-- receiver.go --
package main

import "net/http"

type T struct{}

func (T) F(username string) (int, error) { return 30, error }
`,
			Receiver: "T",
			ExpectedFile: `package main

import (
	"bytes"
	"net/http"
)

type RoutesReceiver interface {
	F(username string) (int, error)
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /age/{username}", func(response http.ResponseWriter, request *http.Request) {
		type ResponseData struct {
			Data    error
			Request *http.Request
		}
		username := request.PathValue("username")
		data, err := receiver.F(username)
		if err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		buf := bytes.NewBuffer(nil)
		rd := ResponseData{Data: data, Request: request}
		if err := templates.ExecuteTemplate(buf, "GET /age/{username} F(username)", rd); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
}
`,
		},
		{
			Name:      "error wrong argument type",
			Templates: `{{define "GET / F(request)"}}Hello, world!{{end}}`,
			ReceiverPackage: `
-- receiver.go --
package main

import "context"

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
	"bytes"
	"context"
	"net/http"
)

type RoutesReceiver interface {
	F(ctx context.Context, username string) int
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /age/{username}", func(response http.ResponseWriter, request *http.Request) {
		type ResponseData struct {
			Data    int
			Request *http.Request
		}
		ctx := request.Context()
		username := request.PathValue("username")
		data := receiver.F(ctx, username)
		buf := bytes.NewBuffer(nil)
		rd := ResponseData{Data: data, Request: request}
		if err := templates.ExecuteTemplate(buf, "GET /age/{username} F(ctx, username)", rd); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
}
`,
		},
		{
			Name:      "form has no fields",
			Templates: `{{define "GET / F(form)"}}Hello, {{.}}!{{end}}`,
			ReceiverPackage: `
-- in.go --
package main

type T struct{}

type In struct{}

func (T) F(form In) any { return nil }
`,
			Receiver: "T",
			ExpectedFile: `package main

import (
	"bytes"
	"net/http"
)

type RoutesReceiver interface {
	F(form In) any
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /", func(response http.ResponseWriter, request *http.Request) {
		type ResponseData struct {
			Data    any
			Request *http.Request
		}
		request.ParseForm()
		var form In
		data := receiver.F(form)
		buf := bytes.NewBuffer(nil)
		rd := ResponseData{Data: data, Request: request}
		if err := templates.ExecuteTemplate(buf, "GET / F(form)", rd); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
}
`,
		},
		{
			Name:      "F is not defined and a form field is passed",
			Templates: `{{define "GET / F(form)"}}Hello, {{.}}!{{end}}`,
			ReceiverPackage: `
-- in.go --
package main

type T struct{}
`,
			Receiver: "T",
			ExpectedFile: `package main

import (
	"bytes"
	"net/http"
	"net/url"
)

type RoutesReceiver interface {
	F(form url.Values) any
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /", func(response http.ResponseWriter, request *http.Request) {
		type ResponseData struct {
			Data    any
			Request *http.Request
		}
		request.ParseForm()
		var form url.Values = request.Form
		data := receiver.F(form)
		buf := bytes.NewBuffer(nil)
		rd := ResponseData{Data: data, Request: request}
		if err := templates.ExecuteTemplate(buf, "GET / F(form)", rd); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
}
`,
		},
		{
			Name:      "F is defined and form type is a struct",
			Templates: `{{define "GET / F(form)"}}Hello, {{.}}!{{end}}`,
			ReceiverPackage: `
-- in.go --
package main

type (
	T struct{}
	In struct{
		field string
	}
)

func (T) F(form In) int { return 0 }

`,
			Receiver: "T",
			ExpectedFile: `package main

import (
	"bytes"
	"net/http"
)

type RoutesReceiver interface {
	F(form In) int
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /", func(response http.ResponseWriter, request *http.Request) {
		type ResponseData struct {
			Data    int
			Request *http.Request
		}
		request.ParseForm()
		var form In
		form.field = request.FormValue("field")
		data := receiver.F(form)
		buf := bytes.NewBuffer(nil)
		rd := ResponseData{Data: data, Request: request}
		if err := templates.ExecuteTemplate(buf, "GET / F(form)", rd); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
}
`,
		},
		{
			Name:      "form html has a cromulent min attribute",
			Templates: `{{define "GET / F(form)"}}<input type="number" name="field" min="13">{{end}}`,
			ReceiverPackage: `
-- in.go --
package main

type (
	T struct{}
	In struct{
		field int
	}
)

func (T) F(form In) int { return 0 }
`,
			Receiver: "T",
			ExpectedFile: `package main

import (
	"bytes"
	"net/http"
	"strconv"
)

type RoutesReceiver interface {
	F(form In) int
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /", func(response http.ResponseWriter, request *http.Request) {
		type ResponseData struct {
			Data    int
			Request *http.Request
		}
		request.ParseForm()
		var form In
		{
			value, err := strconv.Atoi(request.FormValue("field"))
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			if value < 13 {
				http.Error(response, "field must not be less than 13", http.StatusBadRequest)
				return
			}
			form.field = value
		}
		data := receiver.F(form)
		buf := bytes.NewBuffer(nil)
		rd := ResponseData{Data: data, Request: request}
		if err := templates.ExecuteTemplate(buf, "GET / F(form)", rd); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
}
`,
		},
		{
			Name:      "F is defined and form field has an input tag",
			Templates: `{{define "GET / F(form)"}}Hello, {{.}}!{{end}}`,
			ReceiverPackage: `
-- in.go --
package main

type (
	T struct{}
	In struct{
		field string ` + "`name:\"some-field\"`" + `
	}
)

func (T) F(form In) int { return 0 }
`,
			Receiver: "T",
			ExpectedFile: `package main

import (
	"bytes"
	"net/http"
)

type RoutesReceiver interface {
	F(form In) int
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /", func(response http.ResponseWriter, request *http.Request) {
		type ResponseData struct {
			Data    int
			Request *http.Request
		}
		request.ParseForm()
		var form In
		form.field = request.FormValue("some-field")
		data := receiver.F(form)
		buf := bytes.NewBuffer(nil)
		rd := ResponseData{Data: data, Request: request}
		if err := templates.ExecuteTemplate(buf, "GET / F(form)", rd); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
}
`,
		},
		{
			Name:      "F is defined and form slice field",
			Templates: `{{define "GET / F(form)"}}Hello, {{.}}!{{end}}`,
			ReceiverPackage: `
-- in.go --
package main

import "net/http"

type (
	T struct{}
	In struct{
		field []string
	}
)

func (T) F(form In) int { return 0 }
`,
			Receiver: "T",
			ExpectedFile: `package main

import (
	"bytes"
	"net/http"
)

type RoutesReceiver interface {
	F(form In) int
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /", func(response http.ResponseWriter, request *http.Request) {
		type ResponseData struct {
			Data    int
			Request *http.Request
		}
		request.ParseForm()
		var form In
		for _, val := range request.Form["field"] {
			form.field = append(form.field, val)
		}
		data := receiver.F(form)
		buf := bytes.NewBuffer(nil)
		rd := ResponseData{Data: data, Request: request}
		if err := templates.ExecuteTemplate(buf, "GET / F(form)", rd); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
}
`,
		},
		{
			Name:      "F is defined and form has unsupported field type",
			Templates: `{{define "GET / F(form)"}}Hello, {{.}}!{{end}}`,
			ReceiverPackage: `
-- in.go --
package main

import (
	"net/http"
	"net/url"
)

type (
	T struct{}
	In struct{
		href url.URL
	}
)

func (T) F(form In) int { return 0 }

func execute(response http.ResponseWriter, request *http.Request, writeHeader bool, name string, code int, data any) {}
`,
			Receiver:      "T",
			ExpectedError: "failed to generate parse statements for form field href: unsupported type: url.URL",
		},
		{
			Name:        "call F",
			Templates:   `{{define "GET / F()"}}Hello, world!{{end}}`,
			Receiver:    "T",
			PackageName: "main",
			ReceiverPackage: `-- in.go --
package main

type T struct{}
`,
			ExpectedFile: `package main

import (
	"bytes"
	"net/http"
)

type RoutesReceiver interface {
	F() any
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /", func(response http.ResponseWriter, request *http.Request) {
		type ResponseData struct {
			Data    any
			Request *http.Request
		}
		data := receiver.F()
		buf := bytes.NewBuffer(nil)
		rd := ResponseData{Data: data, Request: request}
		if err := templates.ExecuteTemplate(buf, "GET / F()", rd); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
}
`,
		},
		{
			Name:        "no handler",
			Templates:   `{{define "GET /"}}Hello, world!{{end}}`,
			Receiver:    "T",
			PackageName: "main",
			ReceiverPackage: `-- in.go --
package main

type T struct{}
`,
			ExpectedFile: `package main

import (
	"bytes"
	"net/http"
)

type RoutesReceiver interface {
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /", func(response http.ResponseWriter, request *http.Request) {
		type ResponseData struct {
			Request *http.Request
		}
		buf := bytes.NewBuffer(nil)
		rd := ResponseData{Request: request}
		if err := templates.ExecuteTemplate(buf, "GET /", rd); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
}
`,
		},
		{
			Name:      "call F with argument response",
			Templates: `{{define "GET / F(response)"}}{{end}}`,
			Receiver:  "T",
			ReceiverPackage: `-- in.go --
package main

import "net/http"

type T struct{}

func (T) F(response http.ResponseWriter) any {return nil}
`,
			ExpectedFile: `package main

import (
	"bytes"
	"net/http"
)

type RoutesReceiver interface {
	F(response http.ResponseWriter) any
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /", func(response http.ResponseWriter, request *http.Request) {
		type ResponseData struct {
			Data    any
			Request *http.Request
		}
		data := receiver.F(response)
		buf := bytes.NewBuffer(nil)
		rd := ResponseData{Data: data, Request: request}
		if err := templates.ExecuteTemplate(buf, "GET / F(response)", rd); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = buf.WriteTo(response)
	})
}
`,
		},
		{
			Name:      "call F with argument context",
			Templates: `{{define "GET / F(ctx)"}}{{end}}`,
			Receiver:  "T",
			ReceiverPackage: `-- in.go --
package main

import "context"

type T struct{}

func (T) F(ctx context.Context) any {return nil}
`,
			ExpectedFile: `package main

import (
	"bytes"
	"context"
	"net/http"
)

type RoutesReceiver interface {
	F(ctx context.Context) any
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /", func(response http.ResponseWriter, request *http.Request) {
		type ResponseData struct {
			Data    any
			Request *http.Request
		}
		ctx := request.Context()
		data := receiver.F(ctx)
		buf := bytes.NewBuffer(nil)
		rd := ResponseData{Data: data, Request: request}
		if err := templates.ExecuteTemplate(buf, "GET / F(ctx)", rd); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
}
`,
		},
		{
			Name:      "call F with argument path param",
			Templates: `{{define "GET /{param} F(param)"}}{{end}}`,
			Receiver:  "T",
			ReceiverPackage: `-- in.go --
package main

type T struct{}

func (T) F(param string) any {return nil}
`,
			ExpectedFile: `package main

import (
	"bytes"
	"net/http"
)

type RoutesReceiver interface {
	F(param string) any
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /{param}", func(response http.ResponseWriter, request *http.Request) {
		type ResponseData struct {
			Data    any
			Request *http.Request
		}
		param := request.PathValue("param")
		data := receiver.F(param)
		buf := bytes.NewBuffer(nil)
		rd := ResponseData{Data: data, Request: request}
		if err := templates.ExecuteTemplate(buf, "GET /{param} F(param)", rd); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
}
`,
		},
		{
			Name:      "call F with multiple arguments",
			Templates: `{{define "GET /{userName} F(ctx, userName)"}}{{end}}`,
			Receiver:  "T",
			ReceiverPackage: `-- in.go --
package main

import "context"

type T struct{}

func (T) F(ctx context.Context, userName string) any {return nil}
`,
			ExpectedFile: `package main

import (
	"bytes"
	"context"
	"net/http"
)

type RoutesReceiver interface {
	F(ctx context.Context, userName string) any
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /{userName}", func(response http.ResponseWriter, request *http.Request) {
		type ResponseData struct {
			Data    any
			Request *http.Request
		}
		ctx := request.Context()
		userName := request.PathValue("userName")
		data := receiver.F(ctx, userName)
		buf := bytes.NewBuffer(nil)
		rd := ResponseData{Data: data, Request: request}
		if err := templates.ExecuteTemplate(buf, "GET /{userName} F(ctx, userName)", rd); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
}
`,
		},
		{
			Name:      "missing arguments",
			Templates: `{{define "GET / F()"}}{{end}}`,
			Receiver:  "T",
			ReceiverPackage: `-- in.go --
package main

type T struct{}

func (T) F(string) any {return nil}

func execute(response http.ResponseWriter, request *http.Request, writeHeader bool, name string, code int, data any) {}
`,

			ExpectedError: "handler func F(string) any expects 1 arguments but call F() has 0",
		},
		{
			Name:      "extra arguments",
			Templates: `{{define "GET /{name} F(ctx, name)"}}{{end}}`,
			Receiver:  "T",
			ReceiverPackage: `-- in.go --
package main

import ( 
	"context"
	"net/http"
)

type T struct{}

func (T) F(context.Context) any {return nil}

func execute(response http.ResponseWriter, request *http.Request, writeHeader bool, name string, code int, data any) {}
`,
			ExpectedError: "handler func F(context.Context) any expects 1 arguments but call F(ctx, name) has 2",
		},
		{
			Name:      "wrong argument type request",
			Templates: `{{define "GET / F(request)"}}{{end}}`,
			Receiver:  "T",
			ReceiverPackage: `-- in.go --
package main

import ( 
	"context"
	"net/http"
)

type T struct{}

func (T) F(string) any {return nil}

func execute(response http.ResponseWriter, request *http.Request, writeHeader bool, name string, code int, data any) {}
`,
			ExpectedError: "method expects type string but request is *http.Request",
		},
		{
			Name:      "wrong argument type ctx",
			Templates: `{{define "GET / F(ctx)"}}{{end}}`,
			Receiver:  "T",
			ReceiverPackage: `-- in.go --
package main

import "net/http"

type T struct{}

func (T) F(string) any {return nil}

func execute(response http.ResponseWriter, request *http.Request, writeHeader bool, name string, code int, data any) {}
`,
			ExpectedError: "method expects type string but ctx is context.Context",
		},
		{
			Name:      "wrong argument type response",
			Templates: `{{define "GET / F(response)"}}{{end}}`,
			Receiver:  "T",
			ReceiverPackage: `-- in.go --
package main

import "net/http"

type T struct{}

func (T) F(string) any {return nil}

func execute(response http.ResponseWriter, request *http.Request, writeHeader bool, name string, code int, data any) {}
`,
			ExpectedError: "method expects type string but response is http.ResponseWriter",
		},
		{
			Name:      "method missing a result",
			Templates: `{{define "GET / F()"}}{{end}}`,
			Receiver:  "T",
			ReceiverPackage: `-- t.go --
package main

type T struct{}

func (T) F() {}

`,
			ExpectedError: `method for pattern "GET / F()" has no results it should have one or two`,
		},
		{
			Name:      "wrong argument type path value",
			Templates: `{{define "GET /{name} F(name)"}}{{end}}`,
			Receiver:  "T",
			ReceiverPackage: `-- in.go --
package main

import "net/http"

type T struct{}

func (T) F(float64) any {return nil}

func execute(response http.ResponseWriter, request *http.Request, writeHeader bool, name string, code int, data any) {}
`,
			ExpectedError: "method param type float64 not supported",
		},
		{
			Name:      "wrong argument type request ptr",
			Templates: `{{define "GET / F(request)"}}{{end}}`,
			Receiver:  "T",
			ReceiverPackage: `-- in.go --
package main

import "net/http"

type T struct{}

func (T) F(*T) any {return nil}

func execute(response http.ResponseWriter, request *http.Request, writeHeader bool, name string, code int, data any) {}
`,
			ExpectedError: "method expects type *T but request is *http.Request",
		},
		{
			Name:      "wrong argument type in field list",
			Templates: `{{define "GET /post/{postID}/comment/{commentID} F(ctx, request, commentID)"}}{{end}}`,
			Receiver:  "T",
			ReceiverPackage: `-- in.go --
package main

import (
	"context"
	"net/http"
)

type T struct{}

func (T) F(context.Context, string, string) any {return nil}

func execute(response http.ResponseWriter, request *http.Request, writeHeader bool, name string, code int, data any) {}
`,
			ExpectedError: "method expects type string but request is *http.Request",
		},
		{
			Name:      "call expression argument with bool last result",
			Templates: `{{define "GET /{id} F(ctx, Session(response, request), id)"}}{{end}}`,
			Receiver:  "T",
			ReceiverPackage: `-- in.go --
package main

import (
	"bytes"
	"context"
	"net/http"
)

type (
	T struct{}
	S struct{}
)

func (T) F(context.Context, S, int) any {return nil}

func (T) Session(http.ResponseWriter, *http.Request) (S, bool) {return Session{}, false}
`,
			ExpectedFile: `package main

import (
	"bytes"
	"context"
	"net/http"
	"strconv"
)

type RoutesReceiver interface {
	Session(http.ResponseWriter, *http.Request) (S, bool)
	F(context.Context, S, int) any
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /{id}", func(response http.ResponseWriter, request *http.Request) {
		type ResponseData struct {
			Data    any
			Request *http.Request
		}
		ctx := request.Context()
		result0, ok := receiver.Session(response, request)
		if !ok {
			return
		}
		idParsed, err := strconv.Atoi(request.PathValue("id"))
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		id := idParsed
		data := receiver.F(ctx, result0, id)
		buf := bytes.NewBuffer(nil)
		rd := ResponseData{Data: data, Request: request}
		if err := templates.ExecuteTemplate(buf, "GET /{id} F(ctx, Session(response, request), id)", rd); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = buf.WriteTo(response)
	})
}
`,
		},
		{
			Name:      "call expression argument with error last result",
			Templates: `{{define "GET /{id} F(ctx, Author(id), id)"}}{{end}}`,
			Receiver:  "T",
			ReceiverPackage: `-- in.go --
package main

import (
	"context"
	"net/http"
)

type (
	T struct{}
	Session struct{}
)

func (T) F(context.Context, Session, int) any {return nil}

func (T) Author(int) (Session, error) {return Session{}, nil}
`,
			ExpectedFile: `package main

import (
	"bytes"
	"context"
	"net/http"
	"strconv"
)

type RoutesReceiver interface {
	Author(int) (Session, error)
	F(context.Context, Session, int) any
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /{id}", func(response http.ResponseWriter, request *http.Request) {
		type ResponseData struct {
			Data    any
			Request *http.Request
		}
		ctx := request.Context()
		idParsed, err := strconv.Atoi(request.PathValue("id"))
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		id := idParsed
		result0, err := receiver.Author(id)
		if err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		data := receiver.F(ctx, result0, id)
		buf := bytes.NewBuffer(nil)
		rd := ResponseData{Data: data, Request: request}
		if err := templates.ExecuteTemplate(buf, "GET /{id} F(ctx, Author(id), id)", rd); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
}
`,
		},
		{
			Name:      "call expression argument",
			Templates: `{{define "GET / F(ctx, LoadConfiguration())"}}{{end}}`,
			Receiver:  "T",
			ReceiverPackage: `-- in.go --
package main

import (
	"bytes"
	"context"
	"net/http"
)

type (
	T struct{}
	Configuration struct{}
)

func (T) F(context.Context, Configuration) any {return nil}

func (T) LoadConfiguration() Configuration { return }
`,
			ExpectedFile: `package main

import (
	"bytes"
	"context"
	"net/http"
)

type RoutesReceiver interface {
	LoadConfiguration() Configuration
	F(context.Context, Configuration) any
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /", func(response http.ResponseWriter, request *http.Request) {
		type ResponseData struct {
			Data    any
			Request *http.Request
		}
		ctx := request.Context()
		result0 := receiver.LoadConfiguration()
		data := receiver.F(ctx, result0)
		buf := bytes.NewBuffer(nil)
		rd := ResponseData{Data: data, Request: request}
		if err := templates.ExecuteTemplate(buf, "GET / F(ctx, LoadConfiguration())", rd); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
}
`,
		},
		{
			Name:      "call expression argument with response argument",
			Templates: `{{define "GET / F(ctx, Headers(response))"}}{{end}}`,
			Receiver:  "T",
			ReceiverPackage: `-- in.go --
package main

import (
	"context"
	"net/http"
)

type (
	T struct{}
	Configuration struct{}
)

func (T) F(context.Context, any) any {return nil}

func (T) Headers(response http.ResponseWriter) any { return }
`,
			ExpectedFile: `package main

import (
	"bytes"
	"context"
	"net/http"
)

type RoutesReceiver interface {
	Headers(response http.ResponseWriter) any
	F(context.Context, any) any
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /", func(response http.ResponseWriter, request *http.Request) {
		type ResponseData struct {
			Data    any
			Request *http.Request
		}
		ctx := request.Context()
		result0 := receiver.Headers(response)
		data := receiver.F(ctx, result0)
		buf := bytes.NewBuffer(nil)
		rd := ResponseData{Data: data, Request: request}
		if err := templates.ExecuteTemplate(buf, "GET / F(ctx, Headers(response))", rd); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = buf.WriteTo(response)
	})
}
`,
		},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			archive := txtar.Parse([]byte(tt.ReceiverPackage))
			archiveDir, err := txtar.FS(archive)
			require.NoError(t, err)

			dir := t.TempDir()
			require.NoError(t, os.CopyFS(dir, archiveDir))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com\n\ngo 1.20\n"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "template.gohtml"), []byte(tt.Templates), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "templates.go"), []byte(fmt.Sprintf(`package %s

import (
	"embed"
	"html/template"
)

//go:embed template.gohtml
var templatesDir embed.FS

var templates = template.Must(template.ParseFS(templatesDir, "template.gohtml"))
`, cmp.Or(tt.PackageName, "main"))), 0o644))
			logger := log.New(io.Discard, "", 0)
			out, err := muxt.TemplateRoutesFile(dir, logger, muxt.RoutesFileConfiguration{
				ReceiverInterface: tt.Interface,
				PackageName:       tt.PackageName,
				TemplatesVariable: tt.TemplatesVar,
				RoutesFunction:    tt.RoutesFunc,
				PackagePath:       "example.com",
				ReceiverType:      tt.Receiver,
				OutputFileName:    "template_routes.go",
			})
			if tt.ExpectedError == "" {
				assert.NoError(t, err)
				assert.Equal(t, tt.ExpectedFile, out)
			} else {
				assert.ErrorContains(t, err, tt.ExpectedError)
			}
		})
	}
}
