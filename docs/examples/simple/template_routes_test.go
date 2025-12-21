package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/typelate/dom/domtest"
	"golang.org/x/net/html/atom"
)

func TestRoutes(t *testing.T) {
	type Case struct {
		Name  string
		Given func(*testing.T, *BackendMock)
		When  func(*testing.T) *http.Request
		Then  func(*testing.T, *http.Response, *BackendMock)
	}

	run := func(tt Case) func(t *testing.T) {
		return func(t *testing.T) {
			backend := new(BackendMock)
			if tt.Given != nil {
				tt.Given(t, backend)
			}
			req := tt.When(t)

			mux := http.NewServeMux()
			TemplateRoutes(mux, backend)
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			res := rec.Result()

			if tt.Then != nil {
				tt.Then(t, res, backend)
			}
		}
	}

	for _, tt := range []Case{
		{
			Name: "when the row edit form is submitted",
			Given: func(t *testing.T, f *BackendMock) {
				f.On("SubmitFormEditRow", 1, EditRow{5}).Return(Row{ID: 1, Name: "a", Value: 97}, nil)
			},
			When: func(t *testing.T) *http.Request {
				req := httptest.NewRequest(http.MethodPatch, TemplateRoutePaths{}.SubmitFormEditRow(1), strings.NewReader(url.Values{"count": []string{"5"}}.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				return req
			},
			Then: func(t *testing.T, res *http.Response, f *BackendMock) {
				assert.Equal(t, http.StatusOK, res.StatusCode)

				fragment := domtest.ParseResponseDocumentFragment(t, res, atom.Tbody)
				require.Equal(t, 1, fragment.ChildElementCount())
				tdList := fragment.QuerySelectorAll(`tr td`)
				require.Equal(t, 2, tdList.Length())
				require.Equal(t, "a", tdList.Item(0).TextContent())
				require.Equal(t, "97", tdList.Item(1).TextContent())

				f.AssertExpectations(t)
			},
		},
		{
			Name: "when the row edit form is requested",
			Given: func(t *testing.T, f *BackendMock) {
				f.On("GetFormEditRowReturns", 1).Return(Row{ID: 1, Name: "a", Value: 97}, nil)
			},
			When: func(t *testing.T) *http.Request {
				return httptest.NewRequest(http.MethodGet, TemplateRoutePaths{}.GetFormEditRow(1), nil)
			},
			Then: func(t *testing.T, res *http.Response, f *BackendMock) {
				assert.Equal(t, http.StatusOK, res.StatusCode)

				fragment := domtest.ParseResponseDocumentFragment(t, res, atom.Tbody)
				t.Log(fragment)
				require.Equal(t, 1, fragment.ChildElementCount())
				tdList := fragment.QuerySelectorAll(`tr td`)
				require.Equal(t, 2, tdList.Length())
				require.Equal(t, "a", tdList.Item(0).TextContent())

				input := tdList.Item(1).QuerySelector(`input[name='count']`)
				require.Equal(t, input.GetAttribute("value"), "97")
			},
		},
	} {
		t.Run(tt.Name, run(tt))
	}
}

type BackendMock struct {
	mock.Mock
}

func (s *BackendMock) List(ctx context.Context) []Row {
	return s.Mock.Called(ctx)[0].([]Row)
}

func (s *BackendMock) SubmitFormEditRow(fruitID int, form EditRow) (Row, error) {
	res := s.Mock.Called(fruitID, form)
	return res.Get(0).(Row), res.Get(1).(error)
}

func (s *BackendMock) GetFormEditRow(fruitID int) (Row, error) {
	res := s.Mock.Called(fruitID)
	return res.Get(0).(Row), res.Get(1).(error)
}
