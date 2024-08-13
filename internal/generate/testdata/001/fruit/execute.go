package fruit

import (
	"bytes"
	"embed"
	_ "embed"
	"html/template"
	"net/http"
)

//go:embed *.gohtml
var formHTML embed.FS

//go:generate go run github.com/crhntr/muxt/cmd/muxt
var templates = template.Must(template.ParseFS(formHTML, "*"))

func execute(res http.ResponseWriter, _ *http.Request, t *template.Template, code int, data any) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	res.WriteHeader(code)
	_, _ = buf.WriteTo(res)
}

func handleError(res http.ResponseWriter, _ *http.Request, _ *template.Template, code int, err error) {
	http.Error(res, err.Error(), code)
}

type Row struct {
	ID    string
	Fruit string
	Count int
}

func Index(res http.ResponseWriter, req *http.Request) {
	execute(res, req, templates.Lookup("form.gohtml"), http.StatusOK, []Row{
		{ID: "pear", Fruit: "Pear", Count: 72},
		{ID: "plum", Fruit: "Plum", Count: 71},
		{ID: "peach", Fruit: "Peach", Count: 70},
		{ID: "pineapple", Fruit: "Pineapple", Count: 69},
	})
}
