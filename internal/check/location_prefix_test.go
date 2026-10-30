package check_test

import (
	"go/token"
	"go/types"
	"testing"

	"github.com/typelate/muxt/internal/check"
	"github.com/typelate/muxt/internal/template"
)

// TestErrorLocationPrefix states the location and context an error is
// reported with, as upstream check printed them over text/template trees:
// no file name when the set was parsed under the empty name, and the
// context rendered the way text/template's String rendered the node, with
// trim markers dropped and the spacing inside the action normalized. The
// wants were printed by upstream check v0.8.3 for the same input.
func TestErrorLocationPrefix(t *testing.T) {
	pkg := types.NewPackage("example.com/p", "p")
	inner := types.NewStruct([]*types.Var{types.NewField(0, pkg, "C", types.Typ[types.Int], false)}, nil)
	page := types.NewStruct([]*types.Var{
		types.NewField(0, pkg, "Title", types.Typ[types.String], false),
		types.NewField(0, pkg, "A", inner, false),
	}, nil)
	for _, tt := range []struct {
		name, setName, text, want string
	}{
		{
			name: "a set parsed under the empty name", setName: "",
			text: `{{define "p"}}<p>{{template "missing" .}}</p>{{end}}`,
			want: `1:29: executing "p" at <{{template "missing" .}}>: template "missing" not found`,
		},
		{
			name: "a template call with a trim marker and extra spaces", setName: "t",
			text: `{{define "p"}}<p>{{-  template   "missing" .}}</p>{{end}}`,
			want: `t:1:34: executing "p" at <{{template "missing" .}}>: template "missing" not found`,
		},
		{
			name: "a template call with its name on the next line", setName: "t",
			text: "{{define \"p\"}}<p>{{template\n  \"missing\"}}</p>{{end}}",
			want: `t:2:3: executing "p" at <{{template "missing"}}>: template "missing" not found`,
		},
		{
			name: "a field in a spaced out command", setName: "t",
			text: `{{define "p"}}<p>{{ printf  "%d"   .Missing }}</p>{{end}}`,
			want: `t:1:36: executing "p" at <.Missing>: field or method Missing not found on struct{Title string; A struct{C int}}`,
		},
		{
			name: "a chained field", setName: "t",
			text: `{{define "p"}}<p>{{- .A.B -}}</p>{{end}}`,
			want: `t:1:24: executing "p" at <.A.B>: field or method B not found on struct{C int}`,
		},
		{
			name: "a field in a spaced out if", setName: "t",
			text: `{{define "p"}}<p>{{if   eq  .Missing 1 }}x{{end}}</p>{{end}}`,
			want: `t:1:29: executing "p" at <.Missing>: field or method Missing not found on struct{Title string; A struct{C int}}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ts, err := template.New[*types.Signature](tt.setName).Parse(tt.text)
			if err != nil {
				t.Fatal(err)
			}
			tree, _ := ts.FindTree("p")
			global := check.NewGlobal(pkg, token.NewFileSet(), ts, check.DefaultFunctions(pkg))
			err = check.Execute(global, tree, page)
			if err == nil || err.Error() != tt.want {
				t.Errorf("Execute(%q) =\n\t%v\nwant\n\t%s", tt.text, err, tt.want)
			}
		})
	}
}

// TestErrorLocationPrefixOfACommand states that a command's context is the
// command rendered with single spaces between its words, as text/template
// rendered it, with quoted strings kept as written.
func TestErrorLocationPrefixOfACommand(t *testing.T) {
	pkg := types.NewPackage("example.com/p", "p")
	page := types.NewStruct([]*types.Var{types.NewField(0, pkg, "Title", types.Typ[types.String], false)}, nil)
	for _, tt := range []struct{ text, want string }{
		{
			text: `{{define "p"}}<p>{{ undefined  "%d"   .Title  -}}</p>{{end}}`,
			want: `t:1:21: executing "p" at <undefined "%d" .Title>: unknown function: undefined`,
		},
		{
			text: "{{define \"p\"}}<p>{{ undefined  \"a  b\"  `c  d`   .Title  -}}</p>{{end}}",
			want: "t:1:21: executing \"p\" at <undefined \"a  b\" `c  d` .Title>: unknown function: undefined",
		},
	} {
		ts, err := template.New[*types.Signature]("t").Parse(tt.text)
		if err != nil {
			t.Fatal(err)
		}
		tree, _ := ts.FindTree("p")
		global := check.NewGlobal(pkg, token.NewFileSet(), ts, check.Functions{})
		err = check.Execute(global, tree, page)
		if err == nil || err.Error() != tt.want {
			t.Errorf("Execute(%q) =\n\t%v\nwant\n\t%s", tt.text, err, tt.want)
		}
	}
}
