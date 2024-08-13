package muxt_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/crhntr/muxt"
)

func Test_endpoint(t *testing.T) {
	for _, tt := range []struct {
		Name         string
		TemplateName string
		ExpMatch     bool
		Pattern      func(t *testing.T, pat muxt.TemplateName)
		Error        func(t *testing.T, err error)
	}{
		{
			Name:         "get root",
			TemplateName: "GET /",
			ExpMatch:     true,
			Pattern: func(t *testing.T, pat muxt.TemplateName) {
				assert.EqualExportedValues(t, muxt.TemplateName{
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
			Pattern: func(t *testing.T, pat muxt.TemplateName) {
				assert.EqualExportedValues(t, muxt.TemplateName{
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
			Pattern: func(t *testing.T, pat muxt.TemplateName) {
				assert.EqualExportedValues(t, muxt.TemplateName{
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
			Pattern: func(t *testing.T, pat muxt.TemplateName) {
				assert.EqualExportedValues(t, muxt.TemplateName{
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
			Pattern: func(t *testing.T, pat muxt.TemplateName) {
				assert.EqualExportedValues(t, muxt.TemplateName{
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
			Pattern: func(t *testing.T, pat muxt.TemplateName) {
				assert.EqualExportedValues(t, muxt.TemplateName{
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
			Pattern: func(t *testing.T, pat muxt.TemplateName) {
				assert.EqualExportedValues(t, muxt.TemplateName{
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
			pat, err, match := muxt.NewTemplateName(tt.TemplateName)
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
