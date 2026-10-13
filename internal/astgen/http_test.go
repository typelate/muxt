package astgen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHTTPStatusName states which names resolve to a status code. A
// template name may write the constant with or without its package
// qualifier, and the code it resolves to is what the generated handler
// passes to WriteHeader.
func TestHTTPStatusName(t *testing.T) {
	for _, tt := range []struct {
		name string
		want int
	}{
		{name: "http.StatusOK", want: 200},
		{name: "StatusOK", want: 200},
		{name: "http.StatusNoContent", want: 204},
		{name: "http.StatusFound", want: 302},
		{name: "http.StatusNotFound", want: 404},
		{name: "http.StatusTeapot", want: 418},
		{name: "http.StatusInternalServerError", want: 500},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := HTTPStatusName(tt.name)
			require.NoError(t, err, "HTTPStatusName(%q)", tt.name)
			assert.Equal(t, tt.want, got, "HTTPStatusName(%q)", tt.name)
		})
	}
}

// TestHTTPStatusNameRejectsWhatIsNotAConstant states how a name nothing
// resolves is refused. The message is the whole of what a reader gets, and
// the caller writes the name in front of it, so neither half repeats it.
func TestHTTPStatusNameRejectsWhatIsNotAConstant(t *testing.T) {
	for _, tt := range []struct {
		name string
		want string
	}{
		{name: "http.StatusCreted", want: "did you mean http.StatusCreated?"},
		{name: "StatusNotFund", want: "did you mean http.StatusNotFound?"},
		{name: "http.Bananas", want: "not an http.Status constant"},
		{name: "", want: "not an http.Status constant"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			code, err := HTTPStatusName(tt.name)
			require.Error(t, err, "HTTPStatusName(%q) = %d, want an error", tt.name, code)
			assert.Equal(t, tt.want, err.Error(), "HTTPStatusName(%q) error", tt.name)
			assert.Zero(t, code, "HTTPStatusName(%q) code beside the error", tt.name)
		})
	}
}

// TestHTTPStatusNameSuggestsTheSameNameEveryTime states that a name two
// constants are equally near gets one suggestion, the same on every run:
// StatusGo is two edits from both StatusGone and StatusOK.
func TestHTTPStatusNameSuggestsTheSameNameEveryTime(t *testing.T) {
	const want = "did you mean http.StatusGone?"
	for range 50 {
		_, err := HTTPStatusName("StatusGo")
		require.Error(t, err)
		require.Equal(t, want, err.Error(), "HTTPStatusName(%q) error", "StatusGo")
	}
}
