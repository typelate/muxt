package muxt_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/crhntr/muxt"
)

func TestNewPattern(t *testing.T) {
	for _, tt := range []struct {
		Name         string
		TemplateName string
		ExpMatch     bool
		Pattern      func(t *testing.T, pat muxt.Pattern)
		Error        func(t *testing.T, err error)
	}{
		{
			Name:         "get root",
			TemplateName: "GET /",
			ExpMatch:     true,
			Pattern: func(t *testing.T, pat muxt.Pattern) {
				assert.EqualExportedValues(t, muxt.Pattern{
					Method:  http.MethodGet,
					Host:    "",
					Path:    "/",
					Pattern: "GET /",
					Handler: "",
				}, pat)
			},
		},
		{
			Name:         "multiple spaces after method",
			TemplateName: "GET  /",
			ExpMatch:     true,
			Pattern: func(t *testing.T, pat muxt.Pattern) {
				assert.EqualExportedValues(t, muxt.Pattern{
					Method:  http.MethodGet,
					Host:    "",
					Path:    "/",
					Pattern: "GET  /",
					Handler: "",
				}, pat)
			},
		},
		{
			Name:         "post root",
			TemplateName: "POST /",
			ExpMatch:     true,
			Pattern: func(t *testing.T, pat muxt.Pattern) {
				assert.EqualExportedValues(t, muxt.Pattern{
					Method:  http.MethodPost,
					Host:    "",
					Path:    "/",
					Pattern: "POST /",
					Handler: "",
				}, pat)
			},
		},
		{
			Name:         "patch root",
			TemplateName: "PATCH /",
			ExpMatch:     true,
			Pattern: func(t *testing.T, pat muxt.Pattern) {
				assert.EqualExportedValues(t, muxt.Pattern{
					Method:  http.MethodPatch,
					Host:    "",
					Path:    "/",
					Pattern: "PATCH /",
					Handler: "",
				}, pat)
			},
		},
		{
			Name:         "delete root",
			TemplateName: "DELETE /",
			ExpMatch:     true,
			Pattern: func(t *testing.T, pat muxt.Pattern) {
				assert.EqualExportedValues(t, muxt.Pattern{
					Method:  http.MethodDelete,
					Host:    "",
					Path:    "/",
					Pattern: "DELETE /",
					Handler: "",
				}, pat)
			},
		},
		{
			Name:         "put root",
			TemplateName: "PUT /",
			ExpMatch:     true,
			Pattern: func(t *testing.T, pat muxt.Pattern) {
				assert.EqualExportedValues(t, muxt.Pattern{
					Method:  http.MethodPut,
					Host:    "",
					Path:    "/",
					Pattern: "PUT /",
					Handler: "",
				}, pat)
			},
		},
		{
			Name:         "with end of path wildcard",
			TemplateName: "PUT /ping/pong/{$}",
			ExpMatch:     true,
			Pattern: func(t *testing.T, pat muxt.Pattern) {
				assert.EqualExportedValues(t, muxt.Pattern{
					Method:  http.MethodPut,
					Host:    "",
					Path:    "/ping/pong/{$}",
					Pattern: "PUT /ping/pong/{$}",
					Handler: "",
				}, pat)
			},
		},
		{
			Name:         "put root",
			TemplateName: "OPTIONS /",
			ExpMatch:     true,
			Error: func(t *testing.T, err error) {
				assert.ErrorContains(t, err, "OPTIONS method not allowed")
			},
		},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			pat, err, match := muxt.NewPattern(tt.TemplateName)
			require.Equal(t, tt.ExpMatch, match)
			if tt.Error != nil {
				tt.Error(t, err)
			} else if tt.Pattern != nil {
				assert.NoError(t, err)
				tt.Pattern(t, pat)
			}
		})
	}
}

func TestTemplateName_ByPathThenMethod(t *testing.T) {
	for _, tt := range []struct {
		Name    string
		In, Exp []muxt.Pattern
	}{
		{
			Name: "sort by path then method",
			In: []muxt.Pattern{
				mustNewTemplateName("GET /b"),
				mustNewTemplateName("POST /a"),
				mustNewTemplateName("GET /a"),
			},
			Exp: []muxt.Pattern{
				mustNewTemplateName("GET /a"),
				mustNewTemplateName("POST /a"),
				mustNewTemplateName("GET /b"),
			},
		},
		{
			Name: "sort just paths",
			In: []muxt.Pattern{
				mustNewTemplateName("/b"),
				mustNewTemplateName("/c"),
				mustNewTemplateName("/a"),
			},
			Exp: []muxt.Pattern{
				mustNewTemplateName("/a"),
				mustNewTemplateName("/b"),
				mustNewTemplateName("/c"),
			},
		},
		{
			Name: "sort just methods",
			In: []muxt.Pattern{
				mustNewTemplateName("DELETE /"),
				mustNewTemplateName("POST /"),
				mustNewTemplateName("GET /"),
				mustNewTemplateName("PATCH /"),
			},
			Exp: []muxt.Pattern{
				mustNewTemplateName("DELETE /"),
				mustNewTemplateName("GET /"),
				mustNewTemplateName("PATCH /"),
				mustNewTemplateName("POST /"),
			},
		},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			slices.SortFunc(tt.In, muxt.Pattern.ByPathThenMethod)
			assert.Equal(t, tt.Exp, tt.In)
		})
	}
}

func TestPattern_parseHandler(t *testing.T) {
	for _, tt := range []struct {
		Name   string
		In     string
		ExpErr string
	}{
		{
			Name: "no arg post",
			In:   "GET / F()",
		},
		{
			Name: "no arg get",
			In:   "POST / F()",
		},
		{
			Name:   "int as handler",
			In:     "POST / 1",
			ExpErr: "expected call, got: 1",
		},
		{
			Name:   "not an expression",
			In:     "GET / package main",
			ExpErr: "failed to parse handler expression: ",
		},
		{
			Name:   "function literal",
			In:     "GET / func() {} ()",
			ExpErr: "expected function identifier",
		},
		{
			Name:   "call ellipsis",
			In:     "GET /{fileName} F(fileName...)",
			ExpErr: "unexpected ellipsis",
		},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			p, err, ok := muxt.NewPattern(tt.In)
			require.True(t, ok)
			require.NotZero(t, p.Handler)
			if tt.ExpErr != "" {
				assert.ErrorContains(t, err, tt.ExpErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func mustNewTemplateName(in string) muxt.Pattern {
	p, err, _ := muxt.NewPattern(in)
	if err != nil {
		panic(err)
	}
	return p
}
