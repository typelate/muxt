package generate

import (
	"strings"
	"testing"
)

func TestFileImport(t *testing.T) {
	file := scalarTestFile(t)

	if got := file.Import("", "net/http"); got != "http" {
		t.Errorf("Import(net/http) = %q, want http", got)
	}
	if got := file.Import("", "net/http"); got != "http" {
		t.Errorf("Import(net/http) again = %q, want the same name", got)
	}
	aliased := file.Import("", "example.com/other/http")
	if !strings.HasPrefix(aliased, "http") || aliased == "http" || len(aliased) != len("http")+12 {
		t.Errorf("Import(example.com/other/http) = %q, want http followed by a 12 character hash", aliased)
	}
	if got := file.Import("", "example.com/other/http"); got != aliased {
		t.Errorf("Import(example.com/other/http) again = %q, want %q", got, aliased)
	}

	var paths []string
	for _, spec := range file.ImportSpecs() {
		paths = append(paths, spec.Path.Value)
	}
	if want := `"example.com/other/http" "net/http"`; strings.Join(paths, " ") != want {
		t.Errorf("ImportSpecs paths = %v, want %s", paths, want)
	}
}
