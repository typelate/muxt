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
			Name:      "simple",
			Templates: `{{define "GET /"}}Hello, world!{{end}}`,
			ExpectedFile: `package main

import (
	"bytes"
	"net/http"
)

type RoutesReceiver interface {
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /", func(response http.ResponseWriter, request *http.Request) {
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET /", struct {
			Data struct {
			}
			Request *http.Request
		}{Data: struct {
		}{}, Request: request}); err != nil {
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
			Name:      "simple call",
			Templates: `{{define "GET / F()"}}Hello, world!{{end}}`,
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
		data := receiver.F()
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET / F()", struct {
			Data    any
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
			Name:      "multiple arguments no static receiver",
			Templates: `{{define "GET /project/{projectID}/task/{taskID} F(ctx, response, request, projectID, taskID)"}}Hello, world!{{end}}`,
			ExpectedFile: `package main

import (
	"bytes"
	"context"
	"net/http"
)

type RoutesReceiver interface {
	F(ctx context.Context, response http.ResponseWriter, request *http.Request, projectID string, taskID string) any
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /project/{projectID}/task/{taskID}", func(response http.ResponseWriter, request *http.Request) {
		ctx := request.Context()
		projectID := request.PathValue("projectID")
		taskID := request.PathValue("taskID")
		data := receiver.F(ctx, response, request, projectID, taskID)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET /project/{projectID}/task/{taskID} F(ctx, response, request, projectID, taskID)", struct {
			Data    any
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = buf.WriteTo(response)
	})
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
	"bytes"
	"net/http"
)

type RoutesReceiver interface {
	F(username string) int
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /age/{username}", func(response http.ResponseWriter, request *http.Request) {
		username := request.PathValue("username")
		data := receiver.F(username)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET /age/{username} F(username)", struct {
			Data    int
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
		data := receiver.F()
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET / F()", struct {
			Data    any
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
		username := request.PathValue("username")
		data, ok := receiver.F(username)
		if !ok {
			return
		}
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET /age/{username} F(username)", struct {
			Data    bool
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
		username := request.PathValue("username")
		data := receiver.F(username)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET /age/{username} F(username)", struct {
			Data    int
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
		username := request.PathValue("username")
		data, err := receiver.F(username)
		if err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET /age/{username} F(username)", struct {
			Data    error
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
		ctx := request.Context()
		username := request.PathValue("username")
		data := receiver.F(ctx, username)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET /age/{username} F(ctx, username)", struct {
			Data    int
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
			Name:     "when using param parsers",
			Receiver: "T",
			Templates: `
{{- define "GET /bool/{value}   PassBool(value)"   -}} <p>{{- printf "%[1]#v %[1]T" . -}}</p> {{- end -}}
{{- define "GET /int/{value}    PassInt(value)"    -}} <p>{{- printf "%[1]#v %[1]T" . -}}</p> {{- end -}}
{{- define "GET /int16/{value}  PassInt16(value)"  -}} <p>{{- printf "%[1]#v %[1]T" . -}}</p> {{- end -}}
{{- define "GET /int32/{value}  PassInt32(value)"  -}} <p>{{- printf "%[1]#v %[1]T" . -}}</p> {{- end -}}
{{- define "GET /int64/{value}  PassInt64(value)"  -}} <p>{{- printf "%[1]#v %[1]T" . -}}</p> {{- end -}}
{{- define "GET /int8/{value}   PassInt8(value)"   -}} <p>{{- printf "%[1]#v %[1]T" . -}}</p> {{- end -}}
{{- define "GET /uint/{value}   PassUint(value)"   -}} <p>{{- printf "%[1]#v %[1]T" . -}}</p> {{- end -}}
{{- define "GET /uint16/{value} PassUint16(value)" -}} <p>{{- printf "%[1]#v %[1]T" . -}}</p> {{- end -}}
{{- define "GET /uint32/{value} PassUint32(value)" -}} <p>{{- printf "%[1]#v %[1]T" . -}}</p> {{- end -}}
{{- define "GET /uint64/{value} PassUint64(value)" -}} <p>{{- printf "%[1]#v %[1]T" . -}}</p> {{- end -}}
{{- define "GET /uint8/{value}  PassUint8(value)"  -}} <p>{{- printf "%[1]#v %[1]T" . -}}</p> {{- end -}}
`,
			ReceiverPackage: `
-- t.go --
package main

import (
	"embed"
)

//go:embed *.gohtml
var formHTML embed.FS

var templates = template.Must(template.ParseFS(formHTML, "*"))

type T struct{}

func (T) PassInt(in int) int          { return in }
func (T) PassInt64(in int64) int64    { return in }
func (T) PassInt32(in int32) int32    { return in }
func (T) PassInt16(in int16) int16    { return in }
func (T) PassInt8(in int8) int8       { return in }
func (T) PassUint(in uint) uint       { return in }
func (T) PassUint64(in uint64) uint64 { return in }
func (T) PassUint16(in uint16) uint16 { return in }
func (T) PassUint32(in uint32) uint32 { return in }
func (T) PassUint64(in uint16) uint16 { return in }
func (T) PassUint8(in uint8) uint8    { return in }
func (T) PassBool(in bool) bool       { return in }
func (T) PassByte(in byte) byte       { return in }
func (T) PassRune(in rune) rune       { return in }
`,
			ExpectedFile: `package main

import (
	"bytes"
	"net/http"
	"strconv"
)

type RoutesReceiver interface {
	PassBool(in bool) bool
	PassInt(in int) int
	PassInt16(in int16) int16
	PassInt32(in int32) int32
	PassInt64(in int64) int64
	PassInt8(in int8) int8
	PassUint(in uint) uint
	PassUint16(in uint16) uint16
	PassUint32(in uint32) uint32
	PassUint64(in uint64) uint64
	PassUint8(in uint8) uint8
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /bool/{value}", func(response http.ResponseWriter, request *http.Request) {
		valueParsed, err := strconv.ParseBool(request.PathValue("value"))
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		value := valueParsed
		data := receiver.PassBool(value)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET /bool/{value}   PassBool(value)", struct {
			Data    bool
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
	mux.HandleFunc("GET /int/{value}", func(response http.ResponseWriter, request *http.Request) {
		valueParsed, err := strconv.Atoi(request.PathValue("value"))
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		value := valueParsed
		data := receiver.PassInt(value)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET /int/{value}    PassInt(value)", struct {
			Data    int
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
	mux.HandleFunc("GET /int16/{value}", func(response http.ResponseWriter, request *http.Request) {
		valueParsed, err := strconv.ParseInt(request.PathValue("value"), 10, 16)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		value := int16(valueParsed)
		data := receiver.PassInt16(value)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET /int16/{value}  PassInt16(value)", struct {
			Data    int16
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
	mux.HandleFunc("GET /int32/{value}", func(response http.ResponseWriter, request *http.Request) {
		valueParsed, err := strconv.ParseInt(request.PathValue("value"), 10, 32)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		value := int32(valueParsed)
		data := receiver.PassInt32(value)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET /int32/{value}  PassInt32(value)", struct {
			Data    int32
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
	mux.HandleFunc("GET /int64/{value}", func(response http.ResponseWriter, request *http.Request) {
		valueParsed, err := strconv.ParseInt(request.PathValue("value"), 10, 64)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		value := valueParsed
		data := receiver.PassInt64(value)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET /int64/{value}  PassInt64(value)", struct {
			Data    int64
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
	mux.HandleFunc("GET /int8/{value}", func(response http.ResponseWriter, request *http.Request) {
		valueParsed, err := strconv.ParseInt(request.PathValue("value"), 10, 8)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		value := int8(valueParsed)
		data := receiver.PassInt8(value)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET /int8/{value}   PassInt8(value)", struct {
			Data    int8
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
	mux.HandleFunc("GET /uint/{value}", func(response http.ResponseWriter, request *http.Request) {
		valueParsed, err := strconv.ParseUint(request.PathValue("value"), 10, 0)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		value := uint(valueParsed)
		data := receiver.PassUint(value)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET /uint/{value}   PassUint(value)", struct {
			Data    uint
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
	mux.HandleFunc("GET /uint16/{value}", func(response http.ResponseWriter, request *http.Request) {
		valueParsed, err := strconv.ParseUint(request.PathValue("value"), 10, 16)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		value := uint16(valueParsed)
		data := receiver.PassUint16(value)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET /uint16/{value} PassUint16(value)", struct {
			Data    uint16
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
	mux.HandleFunc("GET /uint32/{value}", func(response http.ResponseWriter, request *http.Request) {
		valueParsed, err := strconv.ParseUint(request.PathValue("value"), 10, 32)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		value := uint32(valueParsed)
		data := receiver.PassUint32(value)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET /uint32/{value} PassUint32(value)", struct {
			Data    uint32
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
	mux.HandleFunc("GET /uint64/{value}", func(response http.ResponseWriter, request *http.Request) {
		valueParsed, err := strconv.ParseUint(request.PathValue("value"), 10, 64)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		value := valueParsed
		data := receiver.PassUint64(value)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET /uint64/{value} PassUint64(value)", struct {
			Data    uint64
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Header().Set("content-type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(response)
	})
	mux.HandleFunc("GET /uint8/{value}", func(response http.ResponseWriter, request *http.Request) {
		valueParsed, err := strconv.ParseUint(request.PathValue("value"), 10, 8)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		value := uint8(valueParsed)
		data := receiver.PassUint8(value)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET /uint8/{value}  PassUint8(value)", struct {
			Data    uint8
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
		request.ParseForm()
		var form In
		data := receiver.F(form)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET / F(form)", struct {
			Data    any
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
		request.ParseForm()
		var form url.Values = request.Form
		data := receiver.F(form)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET / F(form)", struct {
			Data    any
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
		request.ParseForm()
		var form In
		form.field = request.FormValue("field")
		data := receiver.F(form)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET / F(form)", struct {
			Data    int
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
		if err := templates.ExecuteTemplate(buf, "GET / F(form)", struct {
			Data    int
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
		request.ParseForm()
		var form In
		form.field = request.FormValue("some-field")
		data := receiver.F(form)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET / F(form)", struct {
			Data    int
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
			Name:      "form argument has typed parameters",
			Templates: `{{define "GET / F(form)"}}Hello, {{.}}!{{end}}`,
			ReceiverPackage: `
-- in.go --
package main

import (
	"net/http"
"time"
)

type (
	T struct{}
	In struct{
		fieldInt    int
		fieldInt64  int64
		fieldInt32  int32
		fieldInt16  int16
		fieldInt8   int8
		fieldUint   uint
		fieldUint64 uint64
		fieldUint32 uint32
		fieldUint16 uint16
		fieldUint8  uint8
		fieldBool   bool
		fieldTime   time.Time
	}
)

func (T) F(form In) int { return 0 }

func execute(response http.ResponseWriter, request *http.Request, writeHeader bool, name string, code int, data any) {}
`,
			Receiver: "T",
			ExpectedFile: `package main

import (
	"bytes"
	"net/http"
	"strconv"
	"time"
)

type RoutesReceiver interface {
	F(form In) int
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /", func(response http.ResponseWriter, request *http.Request) {
		request.ParseForm()
		var form In
		{
			value, err := strconv.Atoi(request.FormValue("fieldInt"))
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldInt = value
		}
		{
			value, err := strconv.ParseInt(request.FormValue("fieldInt64"), 10, 64)
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldInt64 = value
		}
		{
			value, err := strconv.ParseInt(request.FormValue("fieldInt32"), 10, 32)
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldInt32 = int32(value)
		}
		{
			value, err := strconv.ParseInt(request.FormValue("fieldInt16"), 10, 16)
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldInt16 = int16(value)
		}
		{
			value, err := strconv.ParseInt(request.FormValue("fieldInt8"), 10, 8)
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldInt8 = int8(value)
		}
		{
			value, err := strconv.ParseUint(request.FormValue("fieldUint"), 10, 0)
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldUint = uint(value)
		}
		{
			value, err := strconv.ParseUint(request.FormValue("fieldUint64"), 10, 64)
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldUint64 = value
		}
		{
			value, err := strconv.ParseUint(request.FormValue("fieldUint32"), 10, 32)
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldUint32 = uint32(value)
		}
		{
			value, err := strconv.ParseUint(request.FormValue("fieldUint16"), 10, 16)
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldUint16 = uint16(value)
		}
		{
			value, err := strconv.ParseUint(request.FormValue("fieldUint8"), 10, 8)
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldUint8 = uint8(value)
		}
		{
			value, err := strconv.ParseBool(request.FormValue("fieldBool"))
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldBool = value
		}
		{
			value, err := time.Parse("2006-01-02", request.FormValue("fieldTime"))
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldTime = value
		}
		data := receiver.F(form)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET / F(form)", struct {
			Data    int
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
			Name:      "F is defined and form has two two names for a single field",
			Templates: `{{define "GET / F(form)"}}Hello, {{.}}!{{end}}`,
			ReceiverPackage: `
-- in.go --
package main

import "net/http"

type (
	T struct{}
	In struct{
		field1, field2 string
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
		request.ParseForm()
		var form In
		form.field1 = request.FormValue("field1")
		form.field2 = request.FormValue("field2")
		data := receiver.F(form)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET / F(form)", struct {
			Data    int
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
		request.ParseForm()
		var form In
		for _, val := range request.Form["field"] {
			form.field = append(form.field, val)
		}
		data := receiver.F(form)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET / F(form)", struct {
			Data    int
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
			Name:      "F is defined and form has typed slice fields",
			Templates: `{{define "GET / F(form)"}}Hello, {{.}}!{{end}}`,
			ReceiverPackage: `
-- in.go --
package main

import "time"

type (
	T struct{}
	In struct{
		fieldInt    []int
		fieldInt64  []int64
		fieldInt32  []int32
		fieldInt16  []int16
		fieldInt8   []int8
		fieldUint   []uint
		fieldUint64 []uint64
		fieldUint32 []uint32
		fieldUint16 []uint16
		fieldUint8  []uint8
		fieldBool   []bool
		fieldTime   []time.Time
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
	"time"
)

type RoutesReceiver interface {
	F(form In) int
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /", func(response http.ResponseWriter, request *http.Request) {
		request.ParseForm()
		var form In
		for _, val := range request.Form["fieldInt"] {
			value, err := strconv.Atoi(val)
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldInt = append(form.fieldInt, value)
		}
		for _, val := range request.Form["fieldInt64"] {
			value, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldInt64 = append(form.fieldInt64, value)
		}
		for _, val := range request.Form["fieldInt32"] {
			value, err := strconv.ParseInt(val, 10, 32)
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldInt32 = append(form.fieldInt32, int32(value))
		}
		for _, val := range request.Form["fieldInt16"] {
			value, err := strconv.ParseInt(val, 10, 16)
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldInt16 = append(form.fieldInt16, int16(value))
		}
		for _, val := range request.Form["fieldInt8"] {
			value, err := strconv.ParseInt(val, 10, 8)
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldInt8 = append(form.fieldInt8, int8(value))
		}
		for _, val := range request.Form["fieldUint"] {
			value, err := strconv.ParseUint(val, 10, 0)
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldUint = append(form.fieldUint, uint(value))
		}
		for _, val := range request.Form["fieldUint64"] {
			value, err := strconv.ParseUint(val, 10, 64)
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldUint64 = append(form.fieldUint64, value)
		}
		for _, val := range request.Form["fieldUint32"] {
			value, err := strconv.ParseUint(val, 10, 32)
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldUint32 = append(form.fieldUint32, uint32(value))
		}
		for _, val := range request.Form["fieldUint16"] {
			value, err := strconv.ParseUint(val, 10, 16)
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldUint16 = append(form.fieldUint16, uint16(value))
		}
		for _, val := range request.Form["fieldUint8"] {
			value, err := strconv.ParseUint(val, 10, 8)
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldUint8 = append(form.fieldUint8, uint8(value))
		}
		for _, val := range request.Form["fieldBool"] {
			value, err := strconv.ParseBool(val)
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldBool = append(form.fieldBool, value)
		}
		for _, val := range request.Form["fieldTime"] {
			value, err := time.Parse("2006-01-02", val)
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			form.fieldTime = append(form.fieldTime, value)
		}
		data := receiver.F(form)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET / F(form)", struct {
			Data    int
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
		data := receiver.F()
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET / F()", struct {
			Data    any
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET /", struct {
			Data struct {
			}
			Request *http.Request
		}{Data: struct {
		}{}, Request: request}); err != nil {
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
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET /", struct {
			Data struct {
			}
			Request *http.Request
		}{Data: struct {
		}{}, Request: request}); err != nil {
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
		data := receiver.F(response)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET / F(response)", struct {
			Data    any
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
		ctx := request.Context()
		data := receiver.F(ctx)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET / F(ctx)", struct {
			Data    any
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
		param := request.PathValue("param")
		data := receiver.F(param)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET /{param} F(param)", struct {
			Data    any
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
		ctx := request.Context()
		userName := request.PathValue("userName")
		data := receiver.F(ctx, userName)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET /{userName} F(ctx, userName)", struct {
			Data    any
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
		if err := templates.ExecuteTemplate(buf, "GET /{id} F(ctx, Session(response, request), id)", struct {
			Data    any
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
		if err := templates.ExecuteTemplate(buf, "GET /{id} F(ctx, Author(id), id)", struct {
			Data    any
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
		ctx := request.Context()
		result0 := receiver.LoadConfiguration()
		data := receiver.F(ctx, result0)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET / F(ctx, LoadConfiguration())", struct {
			Data    any
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
		ctx := request.Context()
		result0 := receiver.Headers(response)
		data := receiver.F(ctx, result0)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET / F(ctx, Headers(response))", struct {
			Data    any
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = buf.WriteTo(response)
	})
}
`,
		},
		{
			Name:      "use embedded field methods",
			Templates: `{{define "GET / F()"}}{{end}}`,
			Receiver:  "Server",
			ReceiverPackage: `
-- server.go --
package main

type T struct{}

func (T) F() int { return 0 }

type Server struct {
	T
}
`,
			ExpectedFile: `package main

import (
	"bytes"
	"net/http"
)

type RoutesReceiver interface {
	F() int
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /", func(response http.ResponseWriter, request *http.Request) {
		data := receiver.F()
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET / F()", struct {
			Data    int
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
			Name:      "use embedded field methods from another package",
			Templates: `{{define "GET / F()"}}{{end}}`,
			Receiver:  "Server",
			ReceiverPackage: `
-- another/t.go --
package another

type T struct{}

func (T) F() int { return 0 }

-- server.go --
package main

import "example.com/another"

type Server struct {
	another.T
}
`,
			ExpectedFile: `package main

import (
	"bytes"
	"net/http"
)

type RoutesReceiver interface {
	F() int
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /", func(response http.ResponseWriter, request *http.Request) {
		data := receiver.F()
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET / F()", struct {
			Data    int
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
			Name:      "use text encoding",
			Templates: `{{define "GET /{id} F(id)"}}{{.}}{{end}}`,
			Receiver:  "Server",
			ReceiverPackage: `-- f.go --
package main

type ID int

func (id *ID) UnmarshalText(text []byte) error {
	n, err := strconv.ParseUint(string(text), 2, 64)
	if err != nil {
		return err
	}
	*id = n
	return nil
}

type Server struct{}

func (Server) F(id ID) int { return int(id) }
`,
			ExpectedFile: `package main

import (
	"bytes"
	"net/http"
)

type RoutesReceiver interface {
	F(id ID) int
}

func routes(mux *http.ServeMux, receiver RoutesReceiver) {
	mux.HandleFunc("GET /{id}", func(response http.ResponseWriter, request *http.Request) {
		var idParsed ID
		if err := idParsed.UnmarshalText([]byte(request.PathValue("id"))); err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		id := idParsed
		data := receiver.F(id)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET /{id} F(id)", struct {
			Data    int
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
			Name:        "package function",
			Templates:   `{{define "GET / function(ctx)" }}{{.}}{{end}}`,
			PackageName: "main",
			ReceiverPackage: `
-- f.go --
package main

import "context"

func function(ctx context.Context) int { return 32 }
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
		ctx := request.Context()
		data := function(ctx)
		buf := bytes.NewBuffer(nil)
		if err := templates.ExecuteTemplate(buf, "GET / function(ctx)", struct {
			Data    any
			Request *http.Request
		}{Data: data, Request: request}); err != nil {
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
				require.NoError(t, err)
				assert.Equal(t, tt.ExpectedFile, out)
			} else {
				assert.ErrorContains(t, err, tt.ExpectedError)
			}
		})
	}
}
