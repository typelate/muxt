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

	"github.com/typelate/muxt/example/hypertext"
	"github.com/typelate/muxt/example/hypertext/internal/fake"
)

// TemplateRoutePaths is a local alias because the muxt generate expects to write the tests in the package scope.
type TemplateRoutePaths = hypertext.TemplateRoutePaths

func TestTemplateRoutes(t *testing.T) {
	// The Given, When, Then, and Case structures and the runCase function are only generated once.
	// You may add fields to any structure. Do not alter the signature of any Given, When, or Then function on Case.
	// You may edit the body of runCase (and the for case loop body).
	//
	// Consider if you want your collaborator test seam to be RoutesReceiver or your interface implementation's
	// collaborators. This generated test function works well either way. If you use RoutesReceiver as a seam, consider
	// using a mock generator like https://pkg.go.dev/github.com/maxbrunsfeld/counterfeiter/v6 or https://pkg.go.dev/github.com/ryanmoran/faux
	// If you decide to cover your RoutesReceiver testing with this test function. Add the receiver's collaborator
	// test doubles to the Given and Then structures so you can configure and make assertions in the respective
	// Given and Then test hooks for each case.
	type (
		// Given is the scope used for setting up test case collaborators.
		Given struct {
			receiver *fake.Backend
		}

		// When is the scope used to create HTTP Requests. It is unlikely you will need to add additional fields.
		When struct{}

		// Then is the scope used for test case assertions. It will likely have collaborator test doubles.
		Then struct {
			receiver *fake.Backend
		}

		Case struct {
			// The Name, by default a generated identifier, you may change this.
			Name string

			// The Template field is the route template being tested. It is used by the test generator to detect
			// which templates are being tested. Do not change this.
			Template string

			// The "Given" function MAY set up collaborators.
			// The code generator does not add this field in newly generated test cases.
			Given func(t *testing.T, given Given)

			// The "When" function MUST set up an HTTP Request.
			// The generated function will call httptest.NewRequest using the appropriate method and
			// the generated TemplateRoutePaths path constructor method.
			When func(t *testing.T, when When) *http.Request

			// The "Then" function MAY make assertions on response or any configured collaborators.
			// The generated function will assert that the response.StatusCode matches the expected status code.
			//
			// Consider using https://pkg.go.dev/github.com/stretchr/testify for assertions
			// and https://pkg.go.dev/github.com/crhntr/dom/domtest for interacting with the HTML body.
			Then func(t *testing.T, then Then, response *http.Response)
		}
	)

	runCase := func(t *testing.T, tc Case) {
		if tc.When == nil {
			t.Fatal("test case field When must not be nil")
		}
		if tc.Then == nil {
			t.Fatal("test case field Then must not be nil")
		}
		if tc.Template == "" {
			t.Fatal("test case field Template must not be empty")
		}

		// If you need to do universal setup of your receiver, do that here.

		receiver := new(fake.Backend)

		mux := http.NewServeMux()
		hypertext.TemplateRoutes(mux, receiver)
		if tc.Given != nil {
			tc.Given(t, Given{
				receiver: receiver,
			})
		}
		request := tc.When(t, When{})
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)

		// If you want to do universal assertions of all your endpoints, consider writing a helper function
		// and calling it here.

		if tc.Then != nil {
			tc.Then(t, Then{
				receiver: receiver,
			}, recorder.Result())
		}
	}

	for _, tc := range []Case{{
		Name:     "SubmitFormEditRow",
		Template: "PATCH /fruits/{id} SubmitFormEditRow(id, form)",
		Given: func(t *testing.T, given Given) {
			given.receiver.SubmitFormEditRowReturns(hypertext.Row{ID: 1, Name: "a", Value: 97}, nil)
		},
		When: func(t *testing.T, when When) *http.Request {
			body := strings.NewReader(url.Values{"count": []string{"5"}}.Encode())
			request := httptest.NewRequest("PATCH", TemplateRoutePaths{}.SubmitFormEditRow(1), body)
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			return request
		},
		Then: func(t *testing.T, then Then, response *http.Response) {
			require.Equal(t, http.StatusOK, response.StatusCode)

			domtest.Fragment(atom.Tbody, func(t *testing.T, fragment spec.DocumentFragment, _ *fake.Backend) {
				require.Equal(t, 1, fragment.ChildElementCount())
				tdList := fragment.QuerySelectorAll(`tr td`)
				require.Equal(t, 2, tdList.Length())
				require.Equal(t, "a", tdList.Item(0).TextContent())
				require.Equal(t, "97", tdList.Item(1).TextContent())
			})(t, response, then.receiver)

			if assert.Equal(t, 1, then.receiver.SubmitFormEditRowCallCount()) {
				id, form := then.receiver.SubmitFormEditRowArgsForCall(0)
				require.EqualValues(t, 1, id)
				require.Equal(t, hypertext.EditRow{Value: 5}, form)
			}
		},
	}, {
		Name:     "GetFormEditRow",
		Template: "GET /fruits/{id}/edit GetFormEditRow(id)",
		Given: func(t *testing.T, given Given) {
			given.receiver.GetFormEditRowReturns(hypertext.Row{ID: 1, Name: "a", Value: 97}, nil)
		},
		When: func(t *testing.T, when When) *http.Request {
			request := httptest.NewRequest("GET", TemplateRoutePaths{}.GetFormEditRow(1), nil)
			return request
		},
		Then: func(t *testing.T, then Then, response *http.Response) {
			require.Equal(t, http.StatusOK, response.StatusCode)

			domtest.Fragment(atom.Tbody, func(t *testing.T, fragment spec.DocumentFragment, _ *fake.Backend) {
				require.Equal(t, 1, fragment.ChildElementCount())
				tdList := fragment.QuerySelectorAll(`tr td`)
				require.Equal(t, 2, tdList.Length())
				require.Equal(t, "a", tdList.Item(0).TextContent())

				input := tdList.Item(1).QuerySelector(`input[name='count']`)
				require.Equal(t, input.GetAttribute("value"), "97")
			})(t, response, then.receiver)
		},
	}, {
		Name:     "ReadHelp",
		Template: "GET /help",
		When: func(t *testing.T, when When) *http.Request {
			request := httptest.NewRequest("GET", TemplateRoutePaths{}.ReadHelp(), nil)
			return request
		},
		Then: func(t *testing.T, then Then, response *http.Response) {
			require.Equal(t, http.StatusOK, response.StatusCode)
		},
	}, {
		Name:     "List",
		Template: "GET /{$} List(ctx)",
		When: func(t *testing.T, when When) *http.Request {
			request := httptest.NewRequest("GET", TemplateRoutePaths{}.List(), nil)
			return request
		},
		Then: func(t *testing.T, then Then, response *http.Response) {
			if expected, got := http.StatusOK, response.StatusCode; expected != got {
				t.Errorf("unexpected status code: got %d expected %d", got, expected)
			}
		},
	}} {
		t.Run(tc.Name, func(t *testing.T) { runCase(t, tc) })
	}
}
