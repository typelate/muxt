package astgen

import "testing"

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
			if err != nil {
				t.Fatalf("HTTPStatusName(%q) = %v", tt.name, err)
			}
			if got != tt.want {
				t.Errorf("HTTPStatusName(%q) = %d, want %d", tt.name, got, tt.want)
			}
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
			if err == nil {
				t.Fatalf("HTTPStatusName(%q) = %d, want an error", tt.name, code)
			}
			if err.Error() != tt.want {
				t.Errorf("HTTPStatusName(%q) = %q, want %q", tt.name, err, tt.want)
			}
			if code != 0 {
				t.Errorf("HTTPStatusName(%q) = %d, want no code beside the error", tt.name, code)
			}
		})
	}
}
