package overlay_test

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	stdtemplate "html/template"
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/typelate/htt/template"

	"github.com/typelate/muxt/internal/overlay"
)

// TestShimDeclaresHTMLTemplate states that the shim declares every
// exported identifier of the standard library's html/template, as the
// same kind of declaration, and nothing else exported: laid over
// html/template, it is all the API a test target's code can see. An
// identifier a new Go release adds fails this test.
func TestShimDeclaresHTMLTemplate(t *testing.T) {
	std, err := importer.Default().Import("html/template")
	if err != nil {
		t.Fatalf("importing html/template: %v", err)
	}
	want := make(map[string]string)
	for _, name := range std.Scope().Names() {
		obj := std.Scope().Lookup(name)
		if !obj.Exported() {
			continue
		}
		want[name] = objectKind(obj)
	}

	f, err := parser.ParseFile(token.NewFileSet(), "shim.go", overlay.Shim, 0)
	if err != nil {
		t.Fatalf("parsing the shim: %v", err)
	}
	if f.Name.Name != "template" {
		t.Errorf("the shim is package %s, want template", f.Name.Name)
	}
	got := shimDeclarations(f)

	for _, name := range slices.Sorted(maps.Keys(want)) {
		if kind, ok := got[name]; !ok {
			t.Errorf("the shim does not declare %s %s", want[name], name)
		} else if kind != want[name] {
			t.Errorf("the shim declares %s as a %s, html/template as a %s", name, kind, want[name])
		}
	}
	for _, name := range slices.Sorted(maps.Keys(got)) {
		if _, ok := want[name]; !ok {
			t.Errorf("the shim declares %s %s, which html/template does not", got[name], name)
		}
	}
}

// TestShimTypesHaveHTMLTemplateMethods states that each type the shim
// aliases has every exported method html/template's has, by name. The
// engine's types may have more: its Template also has text/template's
// methods and its tree's, which code written for html/template cannot name.
func TestShimTypesHaveHTMLTemplateMethods(t *testing.T) {
	for _, tt := range []struct{ std, engine reflect.Type }{
		{reflect.TypeFor[*stdtemplate.Template](), reflect.TypeFor[*template.Template]()},
		{reflect.TypeFor[*stdtemplate.Error](), reflect.TypeFor[*template.Error]()},
		{reflect.TypeFor[stdtemplate.ErrorCode](), reflect.TypeFor[template.ErrorCode]()},
		{reflect.TypeFor[stdtemplate.HTML](), reflect.TypeFor[template.HTML]()},
	} {
		got := methodNames(tt.engine)
		for _, name := range methodNames(tt.std) {
			if !slices.Contains(got, name) {
				t.Errorf("%v has no method %s, which %v has", tt.engine, name, tt.std)
			}
		}
	}
}

func methodNames(typ reflect.Type) []string {
	var names []string
	for m := range typ.Methods() {
		names = append(names, m.Name)
	}
	return names
}

// objectKind names a package-level object's kind as shimDeclarations
// does.
func objectKind(obj any) string {
	switch reflect.TypeOf(obj).String() {
	case "*types.TypeName":
		return "type"
	case "*types.Func":
		return "func"
	case "*types.Const":
		return "const"
	case "*types.Var":
		return "var"
	}
	return reflect.TypeOf(obj).String()
}

// shimDeclarations is the kind of each exported package-level identifier
// a file declares.
func shimDeclarations(f *ast.File) map[string]string {
	got := make(map[string]string)
	for _, decl := range f.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			if decl.Recv == nil && decl.Name.IsExported() {
				got[decl.Name.Name] = "func"
			}
		case *ast.GenDecl:
			kind := map[token.Token]string{token.TYPE: "type", token.CONST: "const", token.VAR: "var"}[decl.Tok]
			for _, spec := range decl.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					if spec.Name.IsExported() {
						got[spec.Name.Name] = kind
					}
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						if name.IsExported() {
							got[name.Name] = kind
						}
					}
				}
			}
		}
	}
	return got
}
