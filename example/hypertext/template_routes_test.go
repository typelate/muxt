package hypertext_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/crhntr/dom/domtest"
	"github.com/crhntr/dom/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html/atom"

	"github.com/crhntr/muxt/example/hypertext"
	"github.com/crhntr/muxt/example/hypertext/internal/fake"
)

func TestRoutes(t *testing.T) {
	for _, tt := range []domtest.Case[*testing.T, *fake.Backend]{
		{
			Name: "when the row edit form is submitted",
			Given: domtest.GivenPtr(func(t *testing.T, f *fake.Backend) {
				f.SubmitFormEditRowReturns(hypertext.EditRowPage{Row: hypertext.Row{ID: 1, Name: "a", Value: 97}, Error: nil})
			}),
			When: func(t *testing.T) *http.Request {
				req := httptest.NewRequest(http.MethodPatch, hypertext.TemplateRoutePath().SubmitFormEditRow(1), strings.NewReader(url.Values{"count": []string{"5"}}.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				return req
			},
			Then: func(t *testing.T, res *http.Response, f *fake.Backend) {
				assert.Equal(t, http.StatusOK, res.StatusCode)

				domtest.Fragment(atom.Tbody, func(t *testing.T, fragment spec.DocumentFragment, _ *fake.Backend) {
					require.Equal(t, 1, fragment.ChildElementCount())
					tdList := fragment.QuerySelectorAll(`tr td`)
					require.Equal(t, 2, tdList.Length())
					require.Equal(t, "a", tdList.Item(0).TextContent())
					require.Equal(t, "97", tdList.Item(1).TextContent())
				})(t, res, f)

				if assert.Equal(t, 1, f.SubmitFormEditRowCallCount()) {
					id, form := f.SubmitFormEditRowArgsForCall(0)
					require.EqualValues(t, 1, id)
					require.Equal(t, hypertext.EditRow{Value: 5}, form)
				}
			},
		},
		{
			Name: "when the row edit form is requested",
			Given: domtest.GivenPtr(func(t *testing.T, f *fake.Backend) {
				f.GetFormEditRowReturns(hypertext.EditRowPage{Row: hypertext.Row{ID: 1, Name: "a", Value: 97}, Error: nil})
			}),
			When: func(t *testing.T) *http.Request {
				return httptest.NewRequest(http.MethodGet, hypertext.TemplateRoutePath().GetFormEditRow(1), nil)
			},
			Then: func(t *testing.T, res *http.Response, f *fake.Backend) {
				assert.Equal(t, http.StatusOK, res.StatusCode)

				domtest.Fragment(atom.Tbody, func(t *testing.T, fragment spec.DocumentFragment, _ *fake.Backend) {
					t.Log(fragment)
					require.Equal(t, 1, fragment.ChildElementCount())
					tdList := fragment.QuerySelectorAll(`tr td`)
					require.Equal(t, 2, tdList.Length())
					require.Equal(t, "a", tdList.Item(0).TextContent())

					input := tdList.Item(1).QuerySelector(`input[name='count']`)
					require.Equal(t, input.GetAttribute("value"), "97")
				})(t, res, f)
			},
		},
	} {
		t.Run(tt.Name, tt.Run(func(f *fake.Backend) http.Handler {
			mux := http.NewServeMux()
			hypertext.TemplateRoutes(mux, f)
			return mux
		}))
	}
}
