package check_test

import (
	"go/types"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/check"
	"github.com/typelate/muxt/internal/template"
)

// recursionNode is the dot for templates that render a tree by invoking
// themselves on each child.
//
//lint:ignore U1000 it is used in a template
type recursionNode struct {
	Name     string
	Children []recursionNode
}

// recursionOuter and recursionInner alternate as a template descends, so a
// template invoking itself on .Child sees a different dot on each level.
//
//lint:ignore U1000 it is used in a template
type recursionOuter struct {
	Name  string
	Child recursionInner
}

//lint:ignore U1000 it is used in a template
type recursionInner struct {
	Child *recursionOuter
}

// TestExecute_recursiveTemplate checks templates that invoke themselves,
// directly or through a cycle, which used to recurse until the stack
// overflowed.
func TestExecute_recursiveTemplate(t *testing.T) {
	testPkg := find(t, loadPkg(), func(p *packages.Package) bool {
		return p.Name == packageName
	})
	lookup := func(t *testing.T, name string) types.Type {
		t.Helper()
		obj := testPkg.Types.Scope().Lookup(name)
		if obj == nil {
			t.Fatalf("type %s not found", name)
		}
		return obj.Type()
	}

	for _, tt := range []struct {
		name     string
		source   string
		template string
		dot      string
		// wantErr lists substrings that must each appear exactly once in
		// the error; empty means the template must check.
		wantErr []string
	}{
		{
			name:     "invokes itself with the same dot",
			source:   `{{define "node"}}{{.Name}}{{range .Children}}{{template "node" .}}{{end}}{{end}}`,
			template: "node",
			dot:      "recursionNode",
		},
		{
			name:     "two templates invoke each other with the same dot",
			source:   `{{define "a"}}{{.Name}}{{range .Children}}{{template "b" .}}{{end}}{{end}}{{define "b"}}[{{template "a" .}}]{{end}}`,
			template: "a",
			dot:      "recursionNode",
		},
		{
			name:     "invokes itself from inside a block body",
			source:   `{{define "outer"}}{{.Name}}{{block "inner" .}}{{range .Children}}{{template "outer" .}}{{end}}{{end}}{{end}}`,
			template: "outer",
			dot:      "recursionNode",
		},
		{
			name:     "an error in a recursive template is reported once",
			source:   `{{define "bad"}}{{.Missing}}{{range .Children}}{{template "bad" .}}{{end}}{{end}}`,
			template: "bad",
			dot:      "recursionNode",
			wantErr:  []string{"Missing not found"},
		},
		{
			name:     "each distinct dot type is checked",
			source:   `{{define "show"}}{{.Name}}{{template "show" .Child}}{{end}}`,
			template: "show",
			dot:      "recursionOuter",
			wantErr:  []string{"Name not found on github.com/typelate/muxt/internal/check_test.recursionInner"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ts, err := template.New[*types.Signature](tt.template).Parse(tt.source)
			require.NoError(t, err)
			global := check.NewGlobal(testPkg.Types, testPkg.Fset, ts, check.DefaultFunctions(testPkg.Types))
			tree, ok := ts.FindTree(tt.template)
			require.True(t, ok)

			err = check.Execute(global, tree, lookup(t, tt.dot))

			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Fatalf("Execute(%q) = %v, want nil", tt.template, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Execute(%q) = nil, want an error containing %q", tt.template, tt.wantErr)
			}
			for _, want := range tt.wantErr {
				if got := strings.Count(err.Error(), want); got != 1 {
					t.Errorf("Execute(%q) error contains %q %d times, want once\n%v", tt.template, want, got, err)
				}
			}
		})
	}
}

// TestExecute_rootTreeNotServedByName checks that a root tree is not
// mistaken for the template its name resolves to: when the tree finder
// serves a different body under that name, invoking the name must still
// check the served body.
func TestExecute_rootTreeNotServedByName(t *testing.T) {
	testPkg := find(t, loadPkg(), func(p *packages.Package) bool {
		return p.Name == packageName
	})
	root, err := template.New[*types.Signature]("node").Parse(`{{range .Children}}{{template "node" .}}{{end}}`)
	require.NoError(t, err)
	served, err := template.New[*types.Signature]("node").Parse(`{{.Missing}}`)
	require.NoError(t, err)
	global := check.NewGlobal(testPkg.Types, testPkg.Fset, served, check.DefaultFunctions(testPkg.Types))
	dot := testPkg.Types.Scope().Lookup("recursionNode").Type()

	err = check.Execute(global, root.Tree(), dot)

	if err == nil || !strings.Contains(err.Error(), "Missing not found") {
		t.Fatalf("Execute(root) = %v, want an error containing %q from the served node body", err, "Missing not found")
	}
}
