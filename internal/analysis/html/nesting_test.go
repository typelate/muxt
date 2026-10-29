package analysis

import (
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

func TestCheckNesting(t *testing.T) {
	for _, test := range []struct {
		name, input string
		nDiags      int
		contains    string
	}{
		{"nested form", `<form><fieldset><form>x</form></fieldset></form>`, 1, "browsers ignore this <form>"},
		{"form in template ok", `<form><template><form>x</form></template></form>`, 0, ""},
		{"sibling forms ok", `<form>a</form><form>b</form>`, 0, ""},
		{"conditional nested form", `<form>{{if .X}}<form>x</form>{{end}}</form>`, 1, "browsers ignore this <form>"},
		{"nested a", `<a href="/x">x<a href="/y">y</a></a>`, 1, "close the outer <a>"},
		{"a in td inside linked table ok", `<a href="/x"><table><tr><td><a href="/y">y</a></td></tr></table></a>`, 0, ""},
		{"nested button", `<button>x<button>y</button></button>`, 1, "close the outer <button>"},
		{"button in a ok", `<a href="/x"><button>y</button></a>`, 0, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			tree, err := template.ParseTree("test.gohtml", test.name, test.input, nil, lex.NewOptions())
			if err != nil {
				t.Fatalf("ParseTree error: %v", err)
			}
			var diags Diagnostics
			checkNesting(tree, &diags)
			if len(diags) != test.nDiags {
				t.Fatalf("checkNesting(%q) = %d diagnostics %v, want %d", test.input, len(diags), diags, test.nDiags)
			}
			if test.contains != "" && !strings.Contains(diags[0].Message, test.contains) {
				t.Errorf("message %q does not contain %q", diags[0].Message, test.contains)
			}
		})
	}
}

func TestCheckNesting_Severity(t *testing.T) {
	t.Run("nested form is an error", func(t *testing.T) {
		tree, err := template.ParseTree("test.gohtml", t.Name(), `<form><fieldset><form>x</form></fieldset></form>`, nil, lex.NewOptions())
		if err != nil {
			t.Fatalf("ParseTree error: %v", err)
		}
		var diags Diagnostics
		checkNesting(tree, &diags)
		if len(diags) != 1 {
			t.Fatalf("checkNesting = %d diagnostics, want 1", len(diags))
		}
		if diags[0].Severity != SeverityError {
			t.Errorf("Severity = %v, want %v", diags[0].Severity, SeverityError)
		}
	})

	t.Run("conditionally nested form is a warning", func(t *testing.T) {
		tree, err := template.ParseTree("test.gohtml", t.Name(), `<form>{{if .X}}<form>x</form>{{end}}</form>`, nil, lex.NewOptions())
		if err != nil {
			t.Fatalf("ParseTree error: %v", err)
		}
		var diags Diagnostics
		checkNesting(tree, &diags)
		if len(diags) != 1 {
			t.Fatalf("checkNesting = %d diagnostics, want 1", len(diags))
		}
		if diags[0].Severity != SeverityWarning {
			t.Errorf("Severity = %v, want %v", diags[0].Severity, SeverityWarning)
		}
	})
}
