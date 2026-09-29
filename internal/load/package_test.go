package load

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestTestVariantsFirst(t *testing.T) {
	ids := func(pl []*packages.Package) []string {
		var got []string
		for _, pkg := range pl {
			got = append(got, pkg.ID)
		}
		return got
	}
	in := []*packages.Package{
		{ID: "example.com/a"},
		{ID: "example.com/b [example.com/b.test]"},
		{ID: "example.com/c"},
		{ID: "example.com/a [example.com/a.test]"},
		{ID: "example.com/b.test"},
	}
	want := []string{
		"example.com/b [example.com/b.test]",
		"example.com/a [example.com/a.test]",
		"example.com/a",
		"example.com/c",
		"example.com/b.test",
	}
	if got := ids(testVariantsFirst(in)); !slices.Equal(got, want) {
		t.Errorf("testVariantsFirst() = %q, want %q", got, want)
	}
	if got := ids(in); got[0] != "example.com/a" {
		t.Errorf("testVariantsFirst reordered its argument: %q", got)
	}
}

func TestPackagesWithTests(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":    "module example.com/p\n\ngo 1.24\n",
		"p.go":      "package p\n\nfunc F() int { return 1 }\n",
		"p_test.go": "package p\n\nimport \"testing\"\n\nfunc TestF(t *testing.T) { _ = F() }\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	pl, err := PackagesWithTests(dir, nil)
	if err != nil {
		t.Fatalf("PackagesWithTests() error = %v", err)
	}
	if len(pl) == 0 || !strings.HasSuffix(pl[0].ID, ".test]") {
		t.Fatalf("PackagesWithTests()[0] = %v, want the package compiled with its tests first", pl)
	}
	var files []string
	for _, f := range pl[0].GoFiles {
		files = append(files, filepath.Base(f))
	}
	if !slices.Contains(files, "p_test.go") {
		t.Errorf("first package files = %q, want p_test.go among them", files)
	}
	if got, ok := PackageInDirectory(pl, dir); !ok || got != pl[0] {
		t.Errorf("PackageInDirectory() = %v, %v, want the test variant", got, ok)
	}
}

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
