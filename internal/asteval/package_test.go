package asteval

import (
	"slices"
	"testing"

	"golang.org/x/tools/go/packages"
)

func loadError(kind packages.ErrorKind, pos, msg string) packages.Error {
	return packages.Error{Pos: pos, Msg: msg, Kind: kind}
}

// TestParseErrors states what a caller is warned about: source the parser
// could not read, and nothing else.
//
// A type error is how a package looks before muxt generate has written its
// handlers -- a main.go calling a TemplateRoutes that does not exist yet --
// so warning about those would fire on an ordinary first run.
func TestParseErrors(t *testing.T) {
	broken := loadError(packages.ParseError, "server.go:4:1", "expected declaration, found 'return'")

	for _, tt := range []struct {
		name string
		pl   []*packages.Package
		want []string
	}{
		{
			name: "a package that loaded cleanly",
			pl:   []*packages.Package{{PkgPath: "server"}},
		},
		{
			name: "source the parser could not read",
			pl:   []*packages.Package{{PkgPath: "server", Errors: []packages.Error{broken}}},
			want: []string{"server.go:4:1: expected declaration, found 'return'"},
		},
		{
			name: "a type error, which generate has not fixed yet",
			pl: []*packages.Package{{PkgPath: "server", Errors: []packages.Error{
				loadError(packages.TypeError, "main.go:9:2", "undefined: TemplateRoutes"),
			}}},
		},
		{
			name: "a package the loader could not list",
			pl: []*packages.Package{{PkgPath: "server", Errors: []packages.Error{
				loadError(packages.ListError, "", "no Go files"),
			}}},
		},
		{
			name: "one broken file, read by two packages",
			pl: []*packages.Package{
				{PkgPath: "server", Errors: []packages.Error{broken}},
				{PkgPath: "server.test", Errors: []packages.Error{broken}},
			},
			want: []string{"server.go:4:1: expected declaration, found 'return'"},
		},
		{
			name: "two broken files",
			pl: []*packages.Package{{PkgPath: "server", Errors: []packages.Error{
				broken,
				loadError(packages.ParseError, "routes.go:1:1", "expected 'package', found 'func'"),
			}}},
			want: []string{
				"server.go:4:1: expected declaration, found 'return'",
				"routes.go:1:1: expected 'package', found 'func'",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, e := range ParseErrors(tt.pl) {
				got = append(got, e.Error())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("ParseErrors = %q, want %q", got, tt.want)
			}
		})
	}
}
