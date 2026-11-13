package composite_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// TestFindRejectsAWrongOuterShape states that a value that is not an object
// where the file needs one is a *composite.ShapeError naming the place and the
// position of the value, without the json package's Go type names.
func TestFindRejectsAWrongOuterShape(t *testing.T) {
	for _, tt := range []struct {
		name, file, want string
		line, col        int
	}{
		{
			name: "file is a list",
			file: `[]`,
			want: `muxt.json must be a JSON object`,
			line: 1, col: 1,
		},
		{
			name: "file is a string after a newline",
			file: "\n  \"x\"",
			want: `muxt.json must be a JSON object`,
			line: 2, col: 3,
		},
		{
			name: "packages is a list",
			file: `{"packages": []}`,
			want: `packages must be an object keyed by import path`,
			line: 1, col: 14,
		},
		{
			name: "package defaults is a number",
			file: `{"package_defaults": 3}`,
			want: `package_defaults must be a JSON object`,
			line: 1, col: 22,
		},
		{
			name: "entry is a list",
			file: `{"packages": {"example.com/m": []}}`,
			want: `packages["example.com/m"] must be a JSON object`,
			line: 1, col: 32,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeFile(t, tt.file)

			_, _, err := composite.Find(dir, testKeys())

			var typed *composite.ShapeError
			require.ErrorAs(t, err, &typed, "Find error %v", err)
			path := filepath.Join(dir, composite.FileName)
			assert.Equal(t, fmt.Sprintf("%s:%d:%d: %s", path, tt.line, tt.col, tt.want), err.Error(), "message")
			assert.Equal(t, path, typed.Path, "path")
		})
	}
}

// TestFindRejectsARepeatedKey states that a key written twice in one object,
// at the top, in packages, in package_defaults or in an entry, is an error
// naming the key and the position of the second one, not a silent last-wins.
func TestFindRejectsARepeatedKey(t *testing.T) {
	for _, tt := range []struct {
		name, file, at, want string
	}{
		{
			name: "top-level key",
			file: `{"packages": {}, "packages": {}}`,
			at:   `"packages": {}}`,
			want: `key "packages" is repeated`,
		},
		{
			name: "packages key",
			file: `{"packages": {"example.com/m": {}, "example.com/m": {}}}`,
			at:   `"example.com/m": {}}}`,
			want: `packages: key "example.com/m" is repeated`,
		},
		{
			name: "entry key",
			file: "{\"packages\": {\"example.com/m\": {\n\t\"output_htmx\": true,\n\t\"output_htmx\": false}}}",
			at:   `"output_htmx": false`,
			want: `packages["example.com/m"]: key "output_htmx" is repeated`,
		},
		{
			name: "package defaults key",
			file: `{"package_defaults": {"output_htmx": true, "output_htmx": true}, "packages": {}}`,
			at:   `"output_htmx": true}`,
			want: `package_defaults: key "output_htmx" is repeated`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeFile(t, tt.file)

			_, _, err := composite.Find(dir, testKeys())

			var typed *composite.RepeatedKeyError
			require.ErrorAs(t, err, &typed, "Find error %v", err)
			offset := strings.Index(tt.file, tt.at)
			line := 1 + strings.Count(tt.file[:offset], "\n")
			col := offset - strings.LastIndex(tt.file[:offset], "\n")
			path := filepath.Join(dir, composite.FileName)
			assert.Equal(t, fmt.Sprintf("%s:%d:%d: %s", path, line, col, tt.want), err.Error(), "message")
		})
	}
}

// TestFindAcceptsNullWhereAnObjectGoes states that null is an absent object
// everywhere the shape check wants one.
func TestFindAcceptsNullWhereAnObjectGoes(t *testing.T) {
	dir := writeFile(t, `{"package_defaults": null, "packages": {"example.com/m": null}}`)

	_, _, err := composite.Find(dir, testKeys())

	require.NoError(t, err, "Find")
}

// TestFindRejectsANullKeyItDoesNotAccept states that a key read drops as null
// is still checked: unknown, dashed and deprecated keys are errors whatever
// their value.
func TestFindRejectsANullKeyItDoesNotAccept(t *testing.T) {
	keys := testKeys()
	keys.Deprecated = map[string]string{"receiver_type": "use_receiver_type"}
	for _, tt := range []struct {
		name, file, want string
	}{
		{
			name: "unknown",
			file: `{"packages": {"example.com/m": {"no_such_key": null}}}`,
			want: `packages["example.com/m"]: unknown key "no_such_key"`,
		},
		{
			name: "dashed",
			file: `{"package_defaults": {"output-htmx": null}, "packages": {"example.com/m": {}}}`,
			want: `package_defaults: key "output-htmx" uses dashes; write "output_htmx"`,
		},
		{
			name: "deprecated",
			file: `{"packages": {"example.com/m": {"receiver_type": null}}}`,
			want: `packages["example.com/m"]: key "receiver_type" is deprecated; write "use_receiver_type"`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeFile(t, tt.file)

			_, _, err := composite.Find(dir, keys)

			require.Error(t, err, "Find")
			path := filepath.Join(dir, composite.FileName)
			assert.Equal(t, path+": "+tt.want, err.Error(), "message")
		})
	}
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
	assert.JSONEq(t, "true", string(entry["output_htmx"]), "output_htmx of the resolved entry")
	assert.NotContains(t, entry, "use_receiver_type", "the resolved entry")
}

// TestResolveKeepsAnEntryListWhole states that a list an entry holds replaces
// the default's list, and that an empty one still does.
func TestResolveKeepsAnEntryListWhole(t *testing.T) {
	dir := writeFile(t, `{"package_defaults": {"use_templates": ["a", "b"], "output_htmx": true}, "packages": {"example.com/m": {"use_templates": []}}}`)
	keys := composite.Keys{Kinds: map[string]composite.Kind{"use_templates": composite.KindList, "output_htmx": composite.KindBool}}

	f, _, err := composite.Find(dir, keys)

	require.NoError(t, err, "Find")
	entry, ok := f.Resolve("example.com/m")
	require.True(t, ok, "entry")
	assert.JSONEq(t, "[]", string(entry["use_templates"]), "use_templates of the resolved entry")
	assert.JSONEq(t, "true", string(entry["output_htmx"]), "output_htmx of the resolved entry")
	assert.JSONEq(t, `["a", "b"]`, string(f.Defaults["use_templates"]), "the defaults are not changed")
}
