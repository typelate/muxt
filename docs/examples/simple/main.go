package main

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"sync"
)

type Backend struct {
	sync.RWMutex
	data []Row
}

func (b *Backend) List(_ context.Context) []Row {
	b.RLock()
	defer b.RUnlock()
	return slices.Clone(b.data)
}

func (b *Backend) SubmitFormEditRow(fruitID int, form EditRow) (Row, error) {
	return b.findRow(fruitID, func(row *Row) { row.Value = form.Value })
}

func (b *Backend) GetFormEditRow(fruitID int) (Row, error) { return b.findRow(fruitID, nil) }

func (b *Backend) findRow(fruitID int, update func(row *Row)) (Row, error) {
	b.RLock()
	defer b.RUnlock()
	index := slices.IndexFunc(b.data, func(row Row) bool {
		return row.ID == fruitID
	})
	if index < 0 {
		return Row{}, fmt.Errorf("fruit not found")
	}
	if update != nil {
		update(&b.data[index])
	}
	return b.data[index], nil
}

func main() {
	backend := &Backend{
		data: []Row{
			{ID: 1, Name: "Peach", Value: 10},
			{ID: 2, Name: "Plum", Value: 20},
			{ID: 3, Name: "Pineapple", Value: 2},
		},
	}
	mux := http.NewServeMux()
	TemplateRoutes(mux, backend)
	log.Fatal(http.ListenAndServe(":"+cmp.Or(os.Getenv("PORT"), "8080"), mux))
}
