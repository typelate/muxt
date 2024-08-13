package fruit

import (
	"context"
	"net/http"
)

type Receiver interface {
	Taste(ctx context.Context) (any, error)
	EditRow(response http.ResponseWriter, request *http.Request, fruit string) (any, error)
}

func TemplateRoutes(mux *http.ServeMux, receiver Receiver) {
	mux.HandleFunc("GET /farm", func(response http.ResponseWriter, request *http.Request) {
		execute(response, request, templates.Lookup("GET /farm"), http.StatusOK, request)
	})
	mux.HandleFunc("PATCH /fruits/taste", func(response http.ResponseWriter, request *http.Request) {
		data, err := receiver.Taste(request.Context())
		if err != nil {
			handleError(response, request, templates, err)
			return
		}
		execute(response, request, templates.Lookup("PATCH /fruits/taste Taste(ctx)"), http.StatusOK, data)
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
