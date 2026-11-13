package cli

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/composite"
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

// TestSettingsNamesAreDistinct states that no setting name, deprecated
// spelling or muxt.json key is claimed twice: a second claim would register a
// flag twice, which panics, or make one muxt.json key mean two settings.
func TestSettingsNamesAreDistinct(t *testing.T) {
	spellings := map[string]string{}
	keys := map[string]string{}
	claim := func(spelling, owner string) {
		if other, ok := spellings[spelling]; ok {
			t.Errorf("spelling %q is claimed by %s and by %s", spelling, other, owner)
		}
		spellings[spelling] = owner
		key := composite.Key(spelling)
		if other, ok := keys[key]; ok && other != owner {
			t.Errorf("muxt.json key %q is claimed by %s and by %s", key, other, owner)
		}
		keys[key] = owner
	}
	for _, s := range generateSettings(new(generate.RoutesFileConfiguration)) {
		claim(s.name, s.name)
		for _, spelling := range s.deprecated {
			claim(spelling, s.name+" (deprecated spelling)")
		}
	}
	claim(deprecatedTemplatesVariable, useTemplatesVariable+" (deprecated spelling)")
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
