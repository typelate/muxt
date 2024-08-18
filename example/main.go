package main

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
)

//go:embed *.gohtml
var templateSource embed.FS

var templates = template.Must(template.ParseFS(templateSource, "*"))

type Backend struct {
	data []Row
}

type EditRowPage struct {
	Row   Row
	Error error
}

func (b *Backend) SubmitFormEditRow(request *http.Request, fruit string) EditRowPage {
	count, err := strconv.Atoi(request.FormValue("count"))
	if err != nil {
		return EditRowPage{Error: err, Row: Row{Name: fruit}}
	}
	for i := range b.data {
		if b.data[i].Name == fruit {
			b.data[i].Value = count
			return EditRowPage{Error: nil, Row: b.data[i]}
		}
	}
	return EditRowPage{Error: fmt.Errorf("fruit not found")}
}

func (b *Backend) GetFormEditRow(fruit string) EditRowPage {
	for i := range b.data {
		if b.data[i].Name == fruit {
			return EditRowPage{Error: nil, Row: b.data[i]}
		}
	}
	return EditRowPage{Error: fmt.Errorf("fruit not found")}
}

type Row struct {
	Name  string
	Value int
}

func (b *Backend) List(_ context.Context) []Row { return b.data }

//go:generate muxt generate --receiver Backend

func main() {
	backend := &Backend{
		data: []Row{
			{Name: "Peach", Value: 10},
			{Name: "Plum", Value: 20},
			{Name: "Pineapple", Value: 2},
		},
	}
	mux := http.NewServeMux()
	Routes(mux, backend)
	log.Fatal(http.ListenAndServe(":8080", mux))
}
