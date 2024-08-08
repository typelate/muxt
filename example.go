package muxt

import (
	"bytes"
	"html/template"
	"net/http"
	"strconv"
)

func execute(res http.ResponseWriter, req *http.Request, code int, t *template.Template, data any) {
	b := bytes.NewBuffer(nil)
	if err := t.Execute(b, data); err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	res.Header().Set("content-type", "text/html; charset=utf-8")
	res.Header().Set("content-length", strconv.Itoa(b.Len()))
	res.WriteHeader(code)
	_, _ = b.WriteTo(res)
}

func HTMLRoutes(mux *http.ServeMux) {

}
