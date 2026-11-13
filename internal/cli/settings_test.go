package cli

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/generate"
)

// TestSettingsBindEveryField states that each field of the generate
// configuration is bound by exactly one setting, or is set by code and not by
// a person. A field with no setting cannot be given on the command line or in
// muxt.json; a field with two is written twice.
func TestSettingsBindEveryField(t *testing.T) {
	codeSet := map[string]bool{
		"PackageName":                true,
		"PackagePath":                true,
		"MuxtVersion":                true,
		"SilenceHTTPResponseWarning": true,
		"Verbose":                    true,
	}
	var c generate.RoutesFileConfiguration
	settings := generateSettings(&c)

	v := reflect.ValueOf(&c).Elem()
	for i := range v.NumField() {
		name := v.Type().Field(i).Name
		addr := v.Field(i).Addr().Interface()
		var bound []string
		for _, s := range settings {
			if s.field == addr {
				bound = append(bound, s.name)
			}
		}
		switch {
		case codeSet[name]:
			assert.Empty(t, bound, "field %s is set by code but settings %v bind it", name, bound)
		default:
			assert.Len(t, bound, 1, "field %s is bound by settings %v, want exactly one", name, bound)
		}
	}
}

// exampleOf returns the command line argument and the muxt.json value that
// give s its Example.
func exampleOf(t *testing.T, s setting) (arg string, value any) {
	t.Helper()
	require.NotEmpty(t, s.example, "setting %s has no Example", s.name)
	arg = "--" + s.name + "=" + s.example
	switch s.field.(type) {
	case *bool:
		b, err := strconv.ParseBool(s.example)
		require.NoError(t, err, "setting %s Example %q", s.name, s.example)
		return arg, b
	case *[]string:
		return arg, []string{s.example}
	default:
		return arg, s.example
	}
}
