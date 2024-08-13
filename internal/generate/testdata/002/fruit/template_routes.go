package fruit

import (
	"bytes"
	"html/template"
	"net/http"
)

type Receiver interface {
	EditRow(response http.ResponseWriter, request *http.Request, fruit string) (any, error)
}

func TemplateRoutes(mux *http.ServeMux, receiver Receiver) {
	mux.HandleFunc("GET /farm", func(response http.ResponseWriter, request *http.Request) {
		execute(response, request, templates.Lookup("GET /farm"), http.StatusOK, request)
	})
	mux.HandleFunc("PATCH /fruits/{fruit}", func(response http.ResponseWriter, request *http.Request) {
		fruit := request.PathValue("fruit")
		data, err := receiver.EditRow(response, request, fruit)
		if err != nil {
			handleError(response, request, templates, err)
			return
		}
		execute(response, request, templates.Lookup("PATCH /fruits/{fruit} EditRow(response, request, fruit)"), http.StatusOK, data)
	})
	mux.HandleFunc("GET /fruits/{fruit}/edit", func(response http.ResponseWriter, request *http.Request) {
		execute(response, request, templates.Lookup("GET /fruits/{fruit}/edit"), http.StatusOK, request)
	})
}

// execute is a default implementation add a function with the same signature to the package and this function will not be generated
func execute(res http.ResponseWriter, _ *http.Request, t *template.Template, code int, data any) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	res.WriteHeader(code)
	_, _ = buf.WriteTo(res)
}

// handleError is a default implementation add a function with the same signature to the package and this function will not be generated
func handleError(res http.ResponseWriter, _ *http.Request, _ *template.Template, err error) {
	http.Error(res, err.Error(), http.StatusInternalServerError)
}
