package cli

import (
	"bytes"
	"fmt"
	"go/token"
	"go/types"
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

// receiverResult has the shape of the route listing's receiver: a go/types
// named type, which has no exported fields.
type receiverResult struct {
	Receiver *types.Named
}

func (receiverResult) WriteTo(io.Writer) (int64, error) { return 0, nil }

// TestWriteResultJSONReceiver states that a go/types named type is written
// as encoding/json wrote it: an empty object, or null when there is none.
// encoding/json/v2 refuses a struct with no exported fields.
func TestWriteResultJSONReceiver(t *testing.T) {
	named := types.NewNamed(types.NewTypeName(token.NoPos, types.NewPackage("example.com/server", "server"), "Server", nil), types.NewStruct(nil, nil), nil)
	cmd := &cobra.Command{}
	cmd.Flags().String("format", "json", "")
	for _, tt := range []struct {
		name   string
		result receiverResult
		want   string
	}{
		{name: "a receiver", result: receiverResult{Receiver: named}, want: "{\n\t\"Receiver\": {}\n}\n"},
		{name: "no receiver", result: receiverResult{}, want: "{\n\t\"Receiver\": null\n}\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got bytes.Buffer
			require.NoError(t, writeResult(cmd, &got, tt.result))
			assert.Equal(t, tt.want, got.String())
		})
	}
}

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
