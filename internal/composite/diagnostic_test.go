package composite_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/typelate/muxt/internal/composite"
	"github.com/typelate/muxt/internal/diagnostic"
)

// diagnosed is what every error type of the package that has been given a
// diagnostic offers.
type diagnosed interface {
	error
	Diagnostic() diagnostic.Diagnostic
}

// TestDiagnosticText states, for every error type of the package, that the
// one-line text of its diagnostic is its message. A row with a reason is a type
// whose Error() the text cannot reproduce byte for byte, so Error() stays as it
// was written: want is that message, text the diagnostic's.
func TestDiagnosticText(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    diagnosed
		want   string
		text   string
		reason string
	}{
		{
			name: "ValueError",
			err:  &composite.ValueError{Key: "output_max_size", Err: errors.New("invalid byte size")},
			want: "output_max_size: invalid byte size",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.err.Error(), "Error()")
			if tt.reason == "" {
				assert.Equal(t, tt.want, tt.err.Diagnostic().String(), "diagnostic text")
				return
			}
			assert.Equal(t, tt.text, tt.err.Diagnostic().String(), "diagnostic text")
			assert.NotEqual(t, tt.want, tt.text, "a row with a reason differs from its message")
		})
	}
}
