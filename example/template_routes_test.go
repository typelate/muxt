package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/crhntr/dom/domtest"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html/atom"
)

func TestRoutes(t *testing.T) {
	//"PATCH /fruits/{id}"
	t.Run("update fruit with id", func(t *testing.T) {
		mux := http.NewServeMux()
		fake := new(FakeBackend)

		var (
			fruitID int
			form    EditRow
		)
		fake.SubmitFormEditRowFunc = func(fruitIDArg int, formArg EditRow) EditRowPage {
			fruitID, form = fruitIDArg, formArg
			return EditRowPage{Row: Row{ID: 1, Name: "a", Value: 97}, Error: nil}
		}

		routes(mux, fake)

		rec := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodPatch, "/fruits/1", strings.NewReader(url.Values{"count": []string{"5"}}.Encode()))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		mux.ServeHTTP(rec, req)
		res := rec.Result()
		tBody := domtest.DocumentFragmentResponse(t, res, atom.Tbody)
		t.Cleanup(func() {
			if testing.Verbose() && t.Failed() {
				t.Log(tBody)
			}
		})

		require.Equal(t, http.StatusOK, res.StatusCode)
		require.Equal(t, fruitID, 1)
		require.Equal(t, form.Value, 5)

		i := 0
		for el := range tBody.QuerySelectorEach(`td`) {
			switch i {
			case 0:
				require.Equal(t, "a", el.TextContent())
			case 1:
				require.Equal(t, "97", el.TextContent())
			default:
				t.Fatal(el)
			}
			i++
		}
	})

	//"GET /fruits/{id}/edit"
	//"GET /help"
	//"GET /{$}"
}

type FakeBackend struct {
	SubmitFormEditRowFunc func(fruitID int, form EditRow) EditRowPage
	GetFormEditRowFunc    func(fruitID int) EditRowPage
	ListFunc              func(_ context.Context) []Row
}

func (fb *FakeBackend) SubmitFormEditRow(fruitID int, form EditRow) EditRowPage {
	return fb.SubmitFormEditRowFunc(fruitID, form)
}
func (fb *FakeBackend) GetFormEditRow(fruitID int) EditRowPage { return fb.GetFormEditRowFunc(fruitID) }
func (fb *FakeBackend) List(ctx context.Context) []Row         { return fb.ListFunc(ctx) }
