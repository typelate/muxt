package analysis

import (
	"encoding/json/v2"
	"go/token"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/source"
)

// TestNewRoutesListsOverrides states how a name the construction chain
// defined twice is listed: in text as an Overrides section before the routes,
// in JSON as an Overrides member, each naming the dropped and the kept define
// clause. With no override neither the section nor the member appears.
func TestNewRoutesListsOverrides(t *testing.T) {
	override := source.Override{
		Name:     "footer",
		Replaced: source.Site{Package: "example.com/m/fragments", Position: token.Position{Filename: "/m/fragments/footer.gohtml", Line: 1, Column: 1}},
		Kept:     source.Site{Position: token.Position{Filename: "/m/web/page.gohtml", Line: 2, Column: 1}},
	}
	for _, tt := range []struct {
		name      string
		overrides []source.Override
		wantText  string
		wantJSON  []Override
	}{
		{
			name:      "one override",
			overrides: []source.Override{override},
			wantText:  "Overrides:\n  - footer\n      replaced: /m/fragments/footer.gohtml:1:1\n      kept:     /m/web/page.gohtml:2:1\n\nTemplate Routes:\n",
			wantJSON:  []Override{{Name: "footer", Replaced: "/m/fragments/footer.gohtml:1:1", Kept: "/m/web/page.gohtml:2:1"}},
		},
		{
			name:     "none",
			wantText: "Template Routes:\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ts := parseTemplates(t, `{{define "footer"}}f{{end}}`)
			pkg := source.Package{Variables: []source.Variable{{Name: "templates", Set: ts, Overrides: tt.overrides}}}

			results, err := NewRoutes(pkg, nil)

			require.NoError(t, err)
			require.Len(t, results, 1)
			var text strings.Builder
			_, err = results[0].WriteTo(&text)
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(text.String(), tt.wantText), "text = %q, want prefix %q", text.String(), tt.wantText)
			encoded, err := json.Marshal(results[0])
			require.NoError(t, err)
			var decoded struct {
				Overrides []Override
			}
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			assert.Equal(t, tt.wantJSON, decoded.Overrides, "json Overrides in %s", encoded)
			assert.Equal(t, tt.wantJSON != nil, strings.Contains(string(encoded), `"Overrides"`), "json has Overrides member: %s", encoded)
		})
	}
}
