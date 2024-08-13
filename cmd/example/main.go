package main

import (
	"bytes"
	"cmp"
	"embed"
	_ "embed"
	"html/template"
	"log"
	"net/http"
	"os"
)

//go:embed *.gohtml
var formHTML embed.FS

//go:generate go run github.com/crhntr/muxt/cmd/muxt
var templates = template.Must(template.ParseFS(formHTML, "*"))

func main() {
	mux := http.NewServeMux()

	var rec Receiver

	TemplateRoutes(mux, rec)

	log.Fatal(http.ListenAndServe(":"+cmp.Or(os.Getenv("PORT"), "8080"), mux))
}

func execute(res http.ResponseWriter, _ *http.Request, t *template.Template, code int, data any) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	res.WriteHeader(code)
	_, _ = buf.WriteTo(res)
}

func handleError(res http.ResponseWriter, _ *http.Request, _ *template.Template, err error) {
	http.Error(res, err.Error(), http.StatusInternalServerError)
}
