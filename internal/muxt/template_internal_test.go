package muxt

import (
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateName_ByPathThenMethod(t *testing.T) {
	for _, tt := range []struct {
		Name    string
		In, Exp []Template
	}{
		{
			Name: "sort by path then method",
			In: mustNewTemplateName(
				"GET /b",
				"POST /a",
				"GET /a",
			),
			Exp: mustNewTemplateName(
				"GET /a",
				"POST /a",
				"GET /b",
			),
		},
		{
			Name: "sort just paths",
			In: mustNewTemplateName(
				"/b",
				"/c",
				"/a",
			),
			Exp: mustNewTemplateName(
				"/a",
				"/b",
				"/c",
			),
		},
		{
			Name: "sort just methods",
			In: mustNewTemplateName(
				"DELETE /",
				"POST /",
				"GET /",
				"PATCH /",
			),
			Exp: mustNewTemplateName(
				"DELETE /",
				"GET /",
				"PATCH /",
				"POST /",
			),
		},
		{
			// this is blocked higher up in parsing templates but this is lower down so if a
			// caller does not use TemplatePatterns they get consistent results
			Name: "method and path are the same",
			In: mustNewTemplateName(
				"GET / F2()",
				"GET / F3()",
				"GET / F1()",
			),
			Exp: mustNewTemplateName(
				"GET / F1()",
				"GET / F2()",
				"GET / F3()",
			),
		},
		{
			// this is blocked higher up in parsing templates but this is lower down so if a
			// caller does not use TemplatePatterns they get consistent results
			Name: "method and path are the same",
			In: mustNewTemplateName(
				"GET / F2()",
				"GET / F3()",
				"GET / F1()",
			),
			Exp: mustNewTemplateName(
				"GET / F1()",
				"GET / F2()",
				"GET / F3()",
			),
		},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			slices.SortFunc(tt.In, Template.byPathThenMethod)
			assert.Equal(t, stringList(tt.Exp), stringList(tt.In))
		})
	}
}

func stringList[T fmt.Stringer](in []T) []string {
	out := make([]string, 0, len(in))
	for _, e := range in {
		out = append(out, e.String())
	}
	return out
}

func mustNewTemplateName(in ...string) []Template {
	var result []Template
	for _, n := range in {
		p, err, _ := newTemplate(n)
		if err != nil {
			panic(err)
		}
		result = append(result, p)
	}
	return result
}

func TestNewTemplateName(t *testing.T) {
	for _, tt := range []struct {
		Name         string
		In           string
		ExpMatch     bool
		TemplateName func(t *testing.T, pat Template)
		Error        func(t *testing.T, err error)
	}{
		{
			Name:     "get root",
			In:       "GET /",
			ExpMatch: true,
			TemplateName: func(t *testing.T, pat Template) {
				assert.Equal(t, http.MethodGet, pat.method)
				assert.Equal(t, "", pat.host)
				assert.Equal(t, "/", pat.path)
				assert.Equal(t, "GET /", pat.pattern)
				assert.Equal(t, "", pat.handler)
			},
		},
		{
			Name:     "multiple spaces after method",
			In:       "GET  /",
			ExpMatch: true,
			TemplateName: func(t *testing.T, pat Template) {
				assert.Equal(t, http.MethodGet, pat.method)
				assert.Equal(t, "", pat.host)
				assert.Equal(t, "/", pat.path)
				assert.Equal(t, "GET  /", pat.pattern)
				assert.Equal(t, "", pat.handler)
			},
		},
		{
			Name:     "post root",
			In:       "POST /",
			ExpMatch: true,
			TemplateName: func(t *testing.T, pat Template) {
				assert.Equal(t, http.MethodPost, pat.method)
				assert.Equal(t, "", pat.host)
				assert.Equal(t, "/", pat.path)
				assert.Equal(t, "POST /", pat.pattern)
				assert.Equal(t, "", pat.handler)
			},
		},
		{
			Name:     "patch root",
			In:       "PATCH /",
			ExpMatch: true,
			TemplateName: func(t *testing.T, pat Template) {
				assert.Equal(t, http.MethodPatch, pat.method)
				assert.Equal(t, "", pat.host)
				assert.Equal(t, "/", pat.path)
				assert.Equal(t, "PATCH /", pat.pattern)
				assert.Equal(t, "", pat.handler)
			},
		},
		{
			Name:     "delete root",
			In:       "DELETE /",
			ExpMatch: true,
			TemplateName: func(t *testing.T, pat Template) {
				assert.Equal(t, http.MethodDelete, pat.method)
				assert.Equal(t, "", pat.host)
				assert.Equal(t, "/", pat.path)
				assert.Equal(t, "DELETE /", pat.pattern)
				assert.Equal(t, "", pat.handler)
			},
		},
		{
			Name:     "put root",
			In:       "PUT /",
			ExpMatch: true,
			TemplateName: func(t *testing.T, pat Template) {
				assert.Equal(t, http.MethodPut, pat.method)
				assert.Equal(t, "", pat.host)
				assert.Equal(t, "/", pat.path)
				assert.Equal(t, "PUT /", pat.pattern)
				assert.Equal(t, "", pat.handler)
			},
		},
		{
			Name:     "with end of path wildcard",
			In:       "PUT /ping/pong/{$}",
			ExpMatch: true,
			TemplateName: func(t *testing.T, pat Template) {
				assert.Equal(t, http.MethodPut, pat.method)
				assert.Equal(t, "", pat.host)
				assert.Equal(t, "/ping/pong/{$}", pat.path)
				assert.Equal(t, "PUT /ping/pong/{$}", pat.pattern)
				assert.Equal(t, "", pat.handler)
			},
		},
		{
			Name:     "put root",
			In:       "OPTIONS /",
			ExpMatch: true,
			Error: func(t *testing.T, err error) {
				assert.ErrorContains(t, err, "OPTIONS method not allowed")
			},
		},
		{
			Name:     "path parameter is not an identifier",
			In:       "GET /{123} F(123)",
			ExpMatch: true,
			Error: func(t *testing.T, err error) {
				assert.ErrorContains(t, err, `path parameter name not permitted: "123" is not a Go identifier`)
			},
		},
		{
			Name:     "path end sentential in the middle is not permitted",
			In:       "GET /x/{$}/y F()",
			ExpMatch: true,
			Error: func(t *testing.T, err error) {
				assert.ErrorContains(t, err, `path parameter name not permitted: "$" is not a Go identifier`)
			},
		},
		{
			Name:         "path end sentential in the middle is not permitted",
			In:           "GET /x/{$} F()",
			ExpMatch:     true,
			TemplateName: func(t *testing.T, pat Template) {},
		},
		{
			Name:     "duplicate path parameter name",
			In:       "GET /{name}/{name} F()",
			ExpMatch: true,
			Error: func(t *testing.T, err error) {
				assert.ErrorContains(t, err, `forbidden repeated path parameter names: found at least 2 path parameters with name "name"`)
			},
		},
		{
			Name:     "with status code",
			In:       "POST / 202",
			ExpMatch: true,
			TemplateName: func(t *testing.T, pat Template) {
				assert.Equal(t, http.StatusAccepted, pat.statusCode)
			},
		},
		{
			Name:     "without status code",
			In:       "POST /",
			ExpMatch: true,
			TemplateName: func(t *testing.T, pat Template) {
				assert.Equal(t, http.StatusOK, pat.statusCode)
			},
		},
		{
			Name:     "with status code and handler",
			In:       "POST / 202 F()",
			ExpMatch: true,
			TemplateName: func(t *testing.T, pat Template) {
				assert.Equal(t, http.StatusAccepted, pat.statusCode)
			},
		},
		{
			Name:     "with status code constant",
			In:       "POST / http.StatusTeapot F()",
			ExpMatch: true,
			TemplateName: func(t *testing.T, pat Template) {
				assert.Equal(t, http.StatusTeapot, pat.statusCode)
			},
		},
		{
			Name:     "with status code constant",
			In:       "POST / http.StatusBANANA F()",
			ExpMatch: true,
			Error: func(t *testing.T, err error) {
				assert.ErrorContains(t, err, "failed to parse status code: unknown http.StatusBANANA")
			},
		},
		{
			Name:     "with call expression parameter",
			In:       "GET /{id} F(S(response, request), id)",
			ExpMatch: true,
		},
		{
			Name:     "with call expression parameter and status code",
			In:       "GET /{id} 200 F(S(response, request), id)",
			ExpMatch: true,
		},
		{
			Name:     "with call expression parameter",
			In:       "GET / F(S())",
			ExpMatch: true,
		},
		{
			Name:     "when the path parameter is already in scope",
			In:       "GET /{response} 200 F(response)",
			ExpMatch: true,
			Error: func(t *testing.T, err error) {
				assert.ErrorContains(t, err, "the name response is not allowed as a path parameter it is already in scope")
			},
		},
		{
			Name:     "when the expression is not a call",
			In:       "GET / F",
			ExpMatch: true,
			Error: func(t *testing.T, err error) {
				assert.ErrorContains(t, err, "expected call expression, got: F")
			},
		},
		{
			Name:     "when an identifier is not defined",
			In:       "GET / F(unknown)",
			ExpMatch: true,
			Error: func(t *testing.T, err error) {
				assert.ErrorContains(t, err, "unknown argument unknown")
			},
		},
		{
			Name:     "wrong argument expression type",
			In:       "GET / F(1+2)",
			ExpMatch: true,
			Error: func(t *testing.T, err error) {
				assert.ErrorContains(t, err, "expected only identifier or call expressions as arguments, argument at index 0 is: 1 + 2")
			},
		},
		{
			Name:     "wrong argument call argument expression type",
			In:       "GET / F(G(1+2))",
			ExpMatch: true,
			Error: func(t *testing.T, err error) {
				assert.ErrorContains(t, err, "expected only identifier or call expressions as arguments, argument at index 0 is: 1 + 2")
			},
		},
		{
			Name:     "status identifier",
			In:       "/ http.StatusTeapot",
			ExpMatch: true,
			TemplateName: func(t *testing.T, pat Template) {
				assert.Equal(t, "", pat.method)
				assert.Equal(t, "", pat.host)
				assert.Equal(t, "/", pat.path)
				assert.Equal(t, "/", pat.pattern)
				assert.Equal(t, 418, pat.statusCode)
				assert.Equal(t, "", pat.handler)
			},
		},
		{
			Name:     "path and status",
			In:       "/ 418",
			ExpMatch: true,
			TemplateName: func(t *testing.T, pat Template) {
				assert.Equal(t, "", pat.method)
				assert.Equal(t, "", pat.host)
				assert.Equal(t, "/", pat.path)
				assert.Equal(t, "/", pat.pattern)
				assert.Equal(t, 418, pat.statusCode)
				assert.Equal(t, "", pat.handler)
			},
		},
		{
			Name:     "path status and call",
			In:       "/ 418 f()",
			ExpMatch: true,
			TemplateName: func(t *testing.T, pat Template) {
				assert.Equal(t, "", pat.method)
				assert.Equal(t, "", pat.host)
				assert.Equal(t, "/", pat.path)
				assert.Equal(t, "/", pat.pattern)
				assert.Equal(t, 418, pat.statusCode)
				assert.Equal(t, "f()", pat.handler)
			},
		},
		{
			Name:     "path and call",
			In:       "/ f()",
			ExpMatch: true,
			TemplateName: func(t *testing.T, pat Template) {
				assert.Equal(t, "", pat.method)
				assert.Equal(t, "", pat.host)
				assert.Equal(t, "/", pat.path)
				assert.Equal(t, "/", pat.pattern)
				assert.Equal(t, http.StatusOK, pat.statusCode)
				assert.Equal(t, "f()", pat.handler)
			},
		},
		{
			Name:     "no arg post",
			In:       "GET / F()",
			ExpMatch: true,
		},
		{
			Name:     "no arg get",
			In:       "POST / F()",
			ExpMatch: true,
		},
		{
			Name:     "float64 as handler",
			In:       "POST / 1.2",
			ExpMatch: true,
			Error: func(t *testing.T, err error) {
				require.ErrorContains(t, err, `failed to parse status code: strconv.Atoi: parsing "1.2": invalid syntax`)
			},
		},
		{
			Name:     "status constant",
			In:       "POST / http.StatusOK",
			ExpMatch: true,
		},
		{
			Name:     "not an expression",
			In:       "GET / package main",
			ExpMatch: true,
			Error: func(t *testing.T, err error) {
				require.ErrorContains(t, err, "failed to parse handler expression: ")
			},
		},
		{
			Name:     "function literal",
			In:       "GET / func() {} ()",
			ExpMatch: true,
			Error: func(t *testing.T, err error) {
				require.ErrorContains(t, err, "expected function identifier")
			},
		},
		{
			Name:     "call ellipsis",
			In:       "GET /{fileName} F(fileName...)",
			ExpMatch: true,
			Error: func(t *testing.T, err error) {
				require.ErrorContains(t, err, "unexpected ellipsis")
			},
		},
		{
			Name:     "response arg and status code",
			In:       "GET /{fileName} 201 F(response)",
			ExpMatch: true,
			Error: func(t *testing.T, err error) {
				require.ErrorContains(t, err, "you can not use response as an argument and specify an HTTP status code")
			},
		},
		{
			Name:     "empty middle path segment",
			In:       "/x//y",
			ExpMatch: true,
			Error: func(t *testing.T, err error) {
				require.ErrorContains(t, err, "template has an empty path segment:")
			},
		},
		{
			Name:     "empty first path segment",
			In:       "//x/y",
			ExpMatch: true,
			Error: func(t *testing.T, err error) {
				require.ErrorContains(t, err, "template has an empty path segment:")
			},
		},
		{
			Name:     "empty final path segment",
			In:       "/x//",
			ExpMatch: true,
			Error: func(t *testing.T, err error) {
				require.ErrorContains(t, err, "template has an empty path segment:")
			},
		},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			pat, err, match := newTemplate(tt.In)
			require.Equal(t, tt.ExpMatch, match)
			if tt.Error != nil {
				tt.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.TemplateName != nil {
					tt.TemplateName(t, pat)
				}
			}
		})
	}
}
