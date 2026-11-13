package composite_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/composite"
)

// testKeys is the keys the tests of this package read: composite does not
// know the settings, so a test says which keys exist.
func testKeys() composite.Keys {
	return composite.Keys{Kinds: map[string]composite.Kind{
		"use_receiver_type": composite.KindString,
		"output_htmx":       composite.KindBool,
	}}
}

// writeFile writes a go.mod for example.com/m and a muxt.json holding file in
// a temporary directory, and returns the directory.
func writeFile(t *testing.T, file string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/m\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(file), 0o644))
	return dir
}

// TestFindRejectsWrongJSONType states that a value of the wrong JSON type is
// an error naming the file, the entry, the key and the type it takes, before
// the module and key checks. With several, the first in sorted order is named;
// a null value is absent, and an unknown key is reported as unknown.
func TestFindRejectsWrongJSONType(t *testing.T) {
	keys := composite.Keys{Kinds: map[string]composite.Kind{
		"output_htmx":       composite.KindBool,
		"use_receiver_type": composite.KindString,
		"use_templates":     composite.KindList,
	}}
	for _, tt := range []struct {
		name, file, want string
	}{
		{
			name: "string where a boolean goes",
			file: `{"packages": {"example.com/m": {"output_htmx": "yes"}}}`,
			want: `packages["example.com/m"]: key "output_htmx" takes a JSON boolean`,
		},
		{
			name: "boolean where a string goes",
			file: `{"packages": {"example.com/m": {"use_receiver_type": true}}}`,
			want: `packages["example.com/m"]: key "use_receiver_type" takes a JSON string`,
		},
		{
			name: "string where a list goes",
			file: `{"packages": {"example.com/m": {"use_templates": "views"}}}`,
			want: `packages["example.com/m"]: key "use_templates" takes a JSON array of strings`,
		},
		{
			name: "list of numbers",
			file: `{"packages": {"example.com/m": {"use_templates": [1]}}}`,
			want: `packages["example.com/m"]: key "use_templates" takes a JSON array of strings`,
		},
		{
			name: "package defaults",
			file: `{"package_defaults": {"output_htmx": 1}, "packages": {"example.com/m": {}}}`,
			want: `package_defaults: key "output_htmx" takes a JSON boolean`,
		},
		{
			name: "first key in sorted order",
			file: `{"packages": {"example.com/m": {"use_receiver_type": 1, "output_htmx": 1}}}`,
			want: `packages["example.com/m"]: key "output_htmx" takes a JSON boolean`,
		},
		{
			name: "defaults before packages",
			file: `{"packages": {"example.com/m": {"output_htmx": 1}}, "package_defaults": {"use_receiver_type": 1}}`,
			want: `package_defaults: key "use_receiver_type" takes a JSON string`,
		},
		{
			name: "wrong kind before the module check",
			file: `{"packages": {"example.org/other": {"output_htmx": 1}}}`,
			want: `packages["example.org/other"]: key "output_htmx" takes a JSON boolean`,
		},
		{
			name: "unknown key is not a kind error",
			file: `{"packages": {"example.com/m": {"zzzzzzzz": 1}}}`,
			want: `packages["example.com/m"]: unknown key "zzzzzzzz"`,
		},
		{
			name: "null is absent",
			file: `{"packages": {"example.com/m": {"output_htmx": null}}}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeFile(t, tt.file)

			_, _, err := composite.Find(dir, keys)

			if tt.want == "" {
				require.NoError(t, err, "Find")
				return
			}
			require.Error(t, err, "Find")
			path := filepath.Join(dir, composite.FileName)
			assert.Equal(t, path+": "+tt.want, err.Error(), "message")
		})
	}
}

// TestFindRejectsWrongJSONTypeWithAnError states that the wrong-kind error is
// a *composite.ValueTypeError, so a caller can say what the key takes.
func TestFindRejectsWrongJSONTypeWithAnError(t *testing.T) {
	dir := writeFile(t, `{"packages": {"example.com/m": {"output_htmx": "yes"}}}`)

	_, _, err := composite.Find(dir, testKeys())

	var typed *composite.ValueTypeError
	require.ErrorAs(t, err, &typed, "Find error %v", err)
	assert.Equal(t, "output_htmx", typed.Key, "key")
	assert.Equal(t, composite.KindBool, typed.Want, "kind")
}

// TestFindKeepsTheJSONMessageForAWrongOuterShape states that a wrong shape
// outside the entries, such as a list of packages, reads as the json package
// says it.
func TestFindKeepsTheJSONMessageForAWrongOuterShape(t *testing.T) {
	dir := writeFile(t, `{"packages": []}`)

	_, _, err := composite.Find(dir, testKeys())

	require.Error(t, err, "Find")
	path := filepath.Join(dir, composite.FileName)
	assert.Equal(t, "parsing "+path+": json: cannot unmarshal array into Go struct field File.packages of type map[string]composite.Entry", err.Error(), "message")
}

// TestFindReadsNullAsAbsent states that a null value is the same as a key that
// is not there: a default fills it.
func TestFindReadsNullAsAbsent(t *testing.T) {
	dir := writeFile(t, `{"package_defaults": {"output_htmx": true}, "packages": {"example.com/m": {"output_htmx": null, "use_receiver_type": null}}}`)

	f, found, err := composite.Find(dir, testKeys())

	require.NoError(t, err, "Find")
	require.True(t, found, "found")
	entry, ok := f.Resolve("example.com/m")
	require.True(t, ok, "entry")
	require.NotNil(t, entry.OutputHTMX, "output_htmx of the resolved entry")
	assert.True(t, *entry.OutputHTMX, "output_htmx of the resolved entry")
	assert.Nil(t, entry.UseReceiverType, "use_receiver_type of the resolved entry")
}
