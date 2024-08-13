package fruit_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/crhntr/dom/domtest"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/html/atom"

	"github.com/crhntr/muxt/internal/generate/testdata/test001/fruit"
	"github.com/crhntr/muxt/internal/generate/testdata/test001/fruit/fake"
)

//go:generate counterfeiter -generate
//counterfeiter:generate -fake-name FakeReceiver -o fake/receiver.go . Receiver

func TestIndex(t *testing.T) {
	mux := http.NewServeMux()

	receiver := new(fake.FakeReceiver)

	mux.HandleFunc("/{$}", fruit.Index)
	fruit.TemplateRoutes(mux, receiver)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	res := rec.Result()
	document := domtest.Response(t, res)
	assert.Equal(t, http.StatusOK, res.StatusCode)

	rows := document.QuerySelectorAll(`tbody tr`)
	assert.Equal(t, 4, rows.Length())
	if t.Failed() {
		t.Log(document)
	}
}

func TestFarm(t *testing.T) {
	mux := http.NewServeMux()

	receiver := new(fake.FakeReceiver)

	mux.HandleFunc("/{$}", fruit.Index)
	fruit.TemplateRoutes(mux, receiver)

	req := httptest.NewRequest(http.MethodGet, "/farm", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	res := rec.Result()
	document := domtest.Response(t, res)
	assert.Equal(t, http.StatusOK, res.StatusCode)

	main := document.QuerySelector(`main.container`)
	assert.NotNil(t, main)
	assert.Equal(t, strings.TrimSpace(main.TextContent()), "Hello, farm!")
	if t.Failed() {
		t.Log(document)
	}
}

func TestEditRow(t *testing.T) {
	mux := http.NewServeMux()

	receiver := new(fake.FakeReceiver)

	mux.HandleFunc("/{$}", fruit.Index)
	fruit.TemplateRoutes(mux, receiver)

	req := httptest.NewRequest(http.MethodGet, "/fruits/peach/edit", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	res := rec.Result()
	document := domtest.DocumentFragmentResponse(t, res, atom.Tr)
	assert.Equal(t, http.StatusOK, res.StatusCode)

	if form := document.QuerySelector(`form`); assert.NotNil(t, form) {
		assert.Equal(t, "/fruits/peach", form.GetAttribute("hx-patch"))
	}
	if t.Failed() {
		t.Log(document)
	}
}

func TestUpdateRow(t *testing.T) {
	mux := http.NewServeMux()

	receiver := new(fake.FakeReceiver)
	receiver.EditRowReturns(fruit.Row{
		ID:    "peach",
		Fruit: "Peach",
		Count: 32,
	}, nil)

	mux.HandleFunc("/{$}", fruit.Index)
	fruit.TemplateRoutes(mux, receiver)

	req := httptest.NewRequest(http.MethodPatch, "/fruits/peach", strings.NewReader(url.Values{
		"count": []string{"32"},
	}.Encode()))
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	res := rec.Result()
	fragment := domtest.DocumentFragmentResponse(t, res, atom.Tr)
	assert.Equal(t, http.StatusOK, res.StatusCode)

	children := fragment.Children()
	if assert.Equal(t, 2, children.Length()) {
		assert.True(t, children.Item(0).Matches(`td`), children.Item(0))
		assert.True(t, children.Item(1).Matches(`td[hx-get="/fruits/peach/edit"]`), children.Item(1))
	}
}
