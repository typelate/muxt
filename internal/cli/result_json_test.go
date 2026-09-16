package cli

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/astgen"
)

// jsonResult has the shapes a --format=json result is made of: a list and
// a map a command may leave nil, the import map a listing carries, and
// template source.
type jsonResult struct {
	Names   []string
	Counts  map[string]int
	Imports *astgen.TypeFormatter
	Source  string
}

func (jsonResult) WriteTo(io.Writer) (int64, error) { return 0, nil }

// TestWriteResultJSON states that --format=json writes a result as
// encoding/json did before muxt moved to encoding/json/v2: map members,
// including a listing's imports, sorted by key on every run; a nil list or
// map as null; and <, >, & and U+2028 escaped.
func TestWriteResultJSON(t *testing.T) {
	imports := astgen.NewTypeFormatter("example.com/server")
	for i := range 16 {
		imports.Imports[fmt.Sprintf("example.com/pkg%02d", 15-i)] = fmt.Sprintf("pkg%02d", 15-i)
	}
	result := jsonResult{Imports: imports, Source: "<p>{{.A}} & {{.B}}</p> "}

	var want strings.Builder
	want.WriteString("{\n\t\"Names\": null,\n\t\"Counts\": null,\n\t\"Imports\": {\n")
	for i := range 16 {
		separator := ","
		if i == 15 {
			separator = ""
		}
		fmt.Fprintf(&want, "\t\t\"example.com/pkg%02d\": \"pkg%02d\"%s\n", i, i, separator)
	}
	want.WriteString("\t},\n\t\"Source\": \"\\u003cp\\u003e{{.A}} \\u0026 {{.B}}\\u003c/p\\u003e\\u2028\"\n}\n")

	cmd := &cobra.Command{}
	cmd.Flags().String("format", "json", "")
	// Map iteration order changes run to run, so one lucky ordering must not
	// pass.
	for range 20 {
		var got bytes.Buffer
		require.NoError(t, writeResult(cmd, &got, result))
		assert.Equal(t, want.String(), got.String())
	}
}
