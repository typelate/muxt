package composite_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/typelate/muxt/internal/composite"
)

// TestConfiguredDirectoryErrorMessage states that the message names each flag
// given with the muxt.json key to write instead, sorted by flag name, and that
// it is the command and the file alone when no flag is carried.
func TestConfiguredDirectoryErrorMessage(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  *composite.ConfiguredDirectoryError
		want string
	}{
		{
			name: "flags are sorted and each names its key",
			err: &composite.ConfiguredDirectoryError{
				Path: "/m/muxt.json", Command: "generate",
				Flags: []string{"use-receiver-type", "output-htmx"},
				Keys:  map[string]string{"output-htmx": "output_htmx", "use-receiver-type": "use_receiver_type"},
			},
			want: "configure generate with fields in /m/muxt.json instead of flags: --output-htmx (output_htmx), --use-receiver-type (use_receiver_type)",
		},
		{
			name: "a deprecated spelling names the current key",
			err: &composite.ConfiguredDirectoryError{
				Path: "/m/muxt.json", Command: "check",
				Flags: []string{"templates-variable"},
				Keys:  map[string]string{"templates-variable": "use_templates_variable"},
			},
			want: "configure check with fields in /m/muxt.json instead of flags: --templates-variable (use_templates_variable)",
		},
		{
			name: "a flag with no key is named alone",
			err:  &composite.ConfiguredDirectoryError{Path: "/m/muxt.json", Command: "check", Flags: []string{"x"}},
			want: "configure check with fields in /m/muxt.json instead of flags: --x",
		},
		{
			name: "no flags",
			err:  &composite.ConfiguredDirectoryError{Path: "/m/muxt.json", Command: "check"},
			want: "configure check with fields in /m/muxt.json instead of flags",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.err.Error(), "message")
		})
	}
}

// TestSeveralPackagesErrorMessage states that a command that tests one
// package, run where a muxt.json lists several, names the file, the command and
// the directories to pass, on one line.
func TestSeveralPackagesErrorMessage(t *testing.T) {
	err := &composite.SeveralPackagesError{
		Path:    "/m/muxt.json",
		Command: "test-template-mutations",
		Dirs:    []string{"contacts", "inventory", "tasks"},
	}

	assert.Equal(t, "/m/muxt.json: test-template-mutations tests one package; pass one of: contacts inventory tasks", err.Error())
}
