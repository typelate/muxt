package cli

import "testing"

func TestCompilePatterns(t *testing.T) {
	t.Run("none stays nil", func(t *testing.T) {
		got, err := compilePatterns(nil)
		if err != nil || got != nil {
			t.Fatalf("compilePatterns(nil) = %v, %v, want nil, nil", got, err)
		}
	})
	t.Run("in order", func(t *testing.T) {
		got, err := compilePatterns([]string{`^GET `, `Home$`})
		if err != nil {
			t.Fatalf("compilePatterns() error = %v", err)
		}
		if len(got) != 2 || got[0].String() != `^GET ` || got[1].String() != `Home$` {
			t.Fatalf("compilePatterns() = %v, want both patterns in order", got)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		_, err := compilePatterns([]string{`ok`, `(`})
		want := "error parsing regexp: missing closing ): `(`"
		if err == nil || err.Error() != want {
			t.Fatalf("compilePatterns() error = %v, want %q", err, want)
		}
	})
}

func TestCheckTemplatesVariables(t *testing.T) {
	for _, tt := range []struct {
		name    string
		in      []string
		wantErr string
	}{
		{name: "none"},
		{name: "identifiers", in: []string{"templates", "_pages2"}},
		{name: "empty is left to fixTemplateVariables", in: []string{""}},
		{name: "not an identifier", in: []string{"ok", "not-ok"}, wantErr: "variable not-ok value must be a well-formed Go identifier"},
		{name: "keyword", in: []string{"range"}, wantErr: "variable range value must be a well-formed Go identifier"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := checkTemplatesVariables(tt.in)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("checkTemplatesVariables(%q) = %v, want no error", tt.in, err)
			case tt.wantErr != "" && (err == nil || err.Error() != tt.wantErr):
				t.Fatalf("checkTemplatesVariables(%q) = %v, want %q", tt.in, err, tt.wantErr)
			}
		})
	}
}
