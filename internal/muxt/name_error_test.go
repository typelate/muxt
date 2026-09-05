package muxt

import (
	"go/token"
	"html/template"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/typelate/check"
)

// definitionAt is a check.DefinitionFinder reporting a fixed name literal position.
type definitionAt struct {
	name string
	pos  token.Position
}

func (d definitionAt) FindDefinition(name string) (check.Definition, bool) {
	if name != d.name {
		return check.Definition{}, false
	}
	return check.Definition{
		Name:         name,
		TemplateName: check.Span{Position: d.pos, Length: len(name) + 2},
	}, true
}

func TestDefinitionsErrorPosition(t *testing.T) {
	const name = "OPTIONS / F()"
	ts := template.Must(template.New("index.gohtml").Parse(`{{define "` + name + `"}}{{end}}`))
	finder := definitionAt{name: name, pos: token.Position{
		Filename: "index.gohtml",
		Offset:   9,
		Line:     1,
		Column:   10,
	}}

	_, err := Definitions(ts, "templates", finder)
	// The name literal's content starts one byte past the opening quote at
	// column 11; the failing METHOD segment starts at the first byte of the name.
	require.EqualError(t, err, "index.gohtml:1:11: OPTIONS method not allowed")

	nameErr, ok := err.(*NameError)
	require.True(t, ok)
	var sb strings.Builder
	require.NoError(t, nameErr.DetailedError(&sb))
	assert.Equal(t, "  OPTIONS / F()\n  ^^^^^^^\n", sb.String())
}

func TestNameErrorDetailedErrorClamps(t *testing.T) {
	for _, tt := range []struct {
		Name           string
		Err            NameError
		ExpectedMarker string
	}{
		{
			Name:           "span past the end of the name",
			Err:            NameError{Name: "GET /", Offset: 4, Length: 10},
			ExpectedMarker: "    ^",
		},
		{
			Name:           "zero length gets one marker",
			Err:            NameError{Name: "GET /", Offset: 0, Length: 0},
			ExpectedMarker: "^",
		},
		{
			Name:           "negative offset clamps to the start",
			Err:            NameError{Name: "GET /", Offset: -3, Length: 3},
			ExpectedMarker: "^^^",
		},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			var sb strings.Builder
			require.NoError(t, tt.Err.DetailedError(&sb))
			assert.Equal(t, "  "+tt.Err.Name+"\n  "+tt.ExpectedMarker+"\n", sb.String())
		})
	}
}
