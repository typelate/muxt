package check_test

import (
	"fmt"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/check"
	"github.com/typelate/muxt/internal/template"
)

func TestError_DetailedError(t *testing.T) {
	pkg := types.NewPackage("example.com/web", "web")

	userNamed := types.NewNamed(
		types.NewTypeName(token.NoPos, pkg, "User", nil),
		types.NewStruct([]*types.Var{
			types.NewField(token.NoPos, pkg, "Name", types.Typ[types.String], false),
		}, nil), nil)

	pageNamed := types.NewNamed(
		types.NewTypeName(token.NoPos, pkg, "Page", nil),
		types.NewStruct([]*types.Var{
			types.NewField(token.NoPos, pkg, "Title", types.Typ[types.String], false),
			types.NewField(token.NoPos, pkg, "Owner", userNamed, false),
		}, nil), nil)
	visitSig := types.NewSignatureType(types.NewVar(token.NoPos, pkg, "", pageNamed), nil, nil,
		types.NewTuple(types.NewVar(token.NoPos, pkg, "count", types.Typ[types.Int])),
		types.NewTuple(types.NewVar(token.NoPos, pkg, "", types.Typ[types.String])),
		false)
	pageNamed.AddMethod(types.NewFunc(token.NoPos, pkg, "Visit", visitSig))
	setOwnerSig := types.NewSignatureType(types.NewVar(token.NoPos, pkg, "", pageNamed), nil, nil,
		types.NewTuple(types.NewVar(token.NoPos, pkg, "owner", userNamed)),
		types.NewTuple(types.NewVar(token.NoPos, pkg, "", types.Typ[types.String])),
		false)
	pageNamed.AddMethod(types.NewFunc(token.NoPos, pkg, "SetOwner", setOwnerSig))

	webQualifier := func(p *types.Package) string { return p.Name() }

	execute := func(t *testing.T, text string, data types.Type) *check.Error {
		t.Helper()
		tmpl, err := template.New[*types.Signature]("detail.gohtml").Parse(text)
		require.NoError(t, err)
		global := check.NewGlobal(pkg, token.NewFileSet(), tmpl, check.Functions{})
		checkErr := check.Execute(global, tmpl.Tree(), data)
		require.Error(t, checkErr)
		var e *check.Error
		require.ErrorAs(t, checkErr, &e)
		return e
	}

	t.Run("Error stays a compact single line", func(t *testing.T) {
		e := execute(t, `{{.Missing}}`, pageNamed)
		message := e.Error()
		require.Contains(t, message, "field or method Missing not found on example.com/web.Page")
		require.NotContains(t, message, "available")
		require.NotContains(t, message, "\n")
	})

	t.Run("renders the receiver as an indented type declaration", func(t *testing.T) {
		e := execute(t, `{{.Missing}}`, pageNamed)

		var sb strings.Builder
		require.NoError(t, e.DetailedError(&sb, webQualifier))
		detail := sb.String()

		require.Contains(t, detail, "field or method Missing not found on")
		require.Contains(t, detail, strings.Join([]string{
			"  type Page struct {",
			"    Title string",
			"    Owner User",
			"  }",
			"",
			"  func (p Page) SetOwner(owner User) string { /* ... */ }",
			"  func (p Page) Visit(count int) string { /* ... */ }",
		}, "\n"), "the declaration renders as source in its own package: no selector on same-package types")
	})

	t.Run("pointer receivers are preserved in method declarations", func(t *testing.T) {
		counter := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Counter", nil),
			types.NewStruct([]*types.Var{
				types.NewField(token.NoPos, pkg, "Count", types.Typ[types.Int], false),
			}, nil), nil)
		incrementSig := types.NewSignatureType(types.NewVar(token.NoPos, pkg, "", types.NewPointer(counter)), nil, nil,
			nil, types.NewTuple(types.NewVar(token.NoPos, pkg, "", types.Typ[types.Int])), false)
		counter.AddMethod(types.NewFunc(token.NoPos, pkg, "Increment", incrementSig))

		e := execute(t, `{{.Missing}}`, counter)

		var sb strings.Builder
		require.NoError(t, e.DetailedError(&sb, webQualifier))
		require.Contains(t, sb.String(), "\n  func (c *Counter) Increment() int { /* ... */ }",
			"a pointer receiver changes how a template executes, so the declaration keeps it")
	})

	t.Run("interfaces render as interface declarations", func(t *testing.T) {
		greetSig := types.NewSignatureType(nil, nil, nil, nil,
			types.NewTuple(types.NewVar(token.NoPos, pkg, "", types.Typ[types.String])), false)
		iface := types.NewInterfaceType([]*types.Func{
			types.NewFunc(token.NoPos, pkg, "Greet", greetSig),
		}, nil)
		iface.Complete()
		greeter := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Greeter", nil), iface, nil)

		e := execute(t, `{{.Missing}}`, greeter)

		var sb strings.Builder
		require.NoError(t, e.DetailedError(&sb, webQualifier))
		require.Contains(t, sb.String(), strings.Join([]string{
			"  type Greeter interface {",
			"    Greet() string",
			"  }",
		}, "\n"))
	})

	t.Run("named non-struct types render their underlying type", func(t *testing.T) {
		flags := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Flags", nil), types.Typ[types.Int], nil)
		hasSig := types.NewSignatureType(types.NewVar(token.NoPos, pkg, "", flags), nil, nil, nil,
			types.NewTuple(types.NewVar(token.NoPos, pkg, "", types.Typ[types.Bool])), false)
		flags.AddMethod(types.NewFunc(token.NoPos, pkg, "Has", hasSig))

		e := execute(t, `{{.Missing}}`, flags)

		var sb strings.Builder
		require.NoError(t, e.DetailedError(&sb, webQualifier))
		detail := sb.String()
		require.Contains(t, detail, "\n  type Flags int\n")
		require.Contains(t, detail, "\n  func (f Flags) Has() bool { /* ... */ }")
	})

	t.Run("an empty struct body collapses to struct{}", func(t *testing.T) {
		gadget := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Gadget", nil), types.NewStruct(nil, nil), nil)
		pingSig := types.NewSignatureType(types.NewVar(token.NoPos, pkg, "", gadget), nil, nil, nil,
			types.NewTuple(types.NewVar(token.NoPos, pkg, "", types.Typ[types.String])), false)
		gadget.AddMethod(types.NewFunc(token.NoPos, pkg, "Ping", pingSig))

		e := execute(t, `{{.Missing}}`, gadget)

		var sb strings.Builder
		require.NoError(t, e.DetailedError(&sb, webQualifier))
		require.Contains(t, sb.String(), "\n  type Gadget struct{}\n",
			"no fields to list means no braces spread across lines")
	})

	t.Run("foreign package types keep the qualifier inside declarations", func(t *testing.T) {
		dbPkg := types.NewPackage("example.com/db", "db")
		connNamed := types.NewNamed(types.NewTypeName(token.NoPos, dbPkg, "Conn", nil),
			types.NewStruct(nil, nil), nil)
		account := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Account", nil),
			types.NewStruct([]*types.Var{
				types.NewField(token.NoPos, pkg, "Owner", userNamed, false),
				types.NewField(token.NoPos, pkg, "DB", connNamed, false),
			}, nil), nil)

		e := execute(t, `{{.Missing}}`, account)

		var sb strings.Builder
		require.NoError(t, e.DetailedError(&sb, webQualifier))
		detail := sb.String()
		require.Contains(t, detail, "\n    Owner User\n",
			"Account and User are both in web, so no selector")
		require.Contains(t, detail, "\n    DB    db.Conn\n",
			"Conn lives in another package, so the passed qualifier applies")

		sb.Reset()
		require.NoError(t, e.DetailedError(&sb, nil))
		require.Contains(t, sb.String(), "\n    DB    example.com/db.Conn\n",
			"a nil qualifier prints full paths for foreign packages")
	})

	t.Run("private fields are noted but omitted", func(t *testing.T) {
		data := types.NewStruct([]*types.Var{
			types.NewField(token.NoPos, pkg, "Name", types.Typ[types.String], false),
			types.NewField(token.NoPos, pkg, "secret", types.Typ[types.String], false),
		}, nil)
		e := execute(t, `{{.Missing}}`, data)

		var sb strings.Builder
		require.NoError(t, e.DetailedError(&sb, webQualifier))
		require.Contains(t, sb.String(), strings.Join([]string{
			"  struct {",
			"    Name string",
			"    // private fields omitted",
			"  }",
		}, "\n"))
	})

	t.Run("every line uses the passed qualifier", func(t *testing.T) {
		e := execute(t, `{{.Missing}}`, pageNamed)

		var sb strings.Builder
		require.NoError(t, e.DetailedError(&sb, webQualifier))
		detail := sb.String()

		require.Contains(t, detail, `detail.gohtml:1:3: executing "detail.gohtml" at <.Missing>: field or method Missing not found on web.Page`,
			"the message line should qualify types with the passed qualifier")
		require.NotContains(t, detail, "example.com/web",
			"no line should fall back to the construction-time qualifier")
	})

	t.Run("call argument message lines use the passed qualifier", func(t *testing.T) {
		e := execute(t, `{{.SetOwner "x"}}`, pageNamed)

		var sb strings.Builder
		require.NoError(t, e.DetailedError(&sb, webQualifier))
		detail := sb.String()

		require.Contains(t, detail, "argument 0 has type string expected web.User")
		require.Contains(t, detail, "signature: SetOwner(owner web.User) string")
		require.NotContains(t, detail, "example.com/web")
	})

	t.Run("nil qualifier prints full package paths", func(t *testing.T) {
		e := execute(t, `{{.Missing}}`, pageNamed)

		var sb strings.Builder
		require.NoError(t, e.DetailedError(&sb, nil))
		require.Contains(t, sb.String(), "type Page struct {",
			"the declared name is bare even with a nil qualifier")
		require.Contains(t, sb.String(), "Owner User",
			"same-package field types are bare even with a nil qualifier")
		require.Contains(t, sb.String(), "not found on example.com/web.Page",
			"the message line still prints the full package path")
	})

	t.Run("a memberless type says so", func(t *testing.T) {
		e := execute(t, `{{.Missing}}`, types.NewStruct(nil, nil))

		var sb strings.Builder
		require.NoError(t, e.DetailedError(&sb, webQualifier))
		require.Contains(t, sb.String(), "struct{} has no exported fields or methods")
	})

	t.Run("call errors include the qualified signature and arguments", func(t *testing.T) {
		e := execute(t, `{{.Visit "x"}}`, pageNamed)

		var sb strings.Builder
		require.NoError(t, e.DetailedError(&sb, webQualifier))
		detail := sb.String()
		require.Contains(t, detail, "signature: Visit(count int) string")
		require.Contains(t, detail, "[0] string")
	})

	t.Run("massive inline struct types are elided", func(t *testing.T) {
		bigFields := make([]*types.Var, 8)
		for i := range bigFields {
			bigFields[i] = types.NewField(token.NoPos, pkg, fmt.Sprintf("F%d", i), types.Typ[types.String], false)
		}
		big := types.NewStruct(bigFields, nil)
		data := types.NewStruct([]*types.Var{
			types.NewField(token.NoPos, pkg, "Title", types.Typ[types.String], false),
			types.NewField(token.NoPos, pkg, "Meta", big, false),
		}, nil)

		e := execute(t, `{{.Missing}}`, data)

		require.Contains(t, e.Error(), "F0 string",
			"Error keeps the full inline type")
		leaf := findLeafError(t, e)
		require.True(t, types.Identical(data, leaf.X),
			"X keeps the full type")

		var sb strings.Builder
		require.NoError(t, e.DetailedError(&sb, webQualifier))
		detail := sb.String()
		require.Contains(t, detail, "not found on struct{...}")
		require.Contains(t, detail, "\n  struct {", "an unnamed type renders without a type keyword or name")
		require.Contains(t, detail, "\n    Meta  struct{...}")
		require.NotContains(t, detail, "F0 string",
			"nested fields of an elided struct should not leak into the listing")
	})

	t.Run("method signatures elide massive inline struct parameters", func(t *testing.T) {
		bigFields := make([]*types.Var, 8)
		for i := range bigFields {
			bigFields[i] = types.NewField(token.NoPos, pkg, fmt.Sprintf("F%d", i), types.Typ[types.String], false)
		}
		big := types.NewStruct(bigFields, nil)
		owner := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Widget", nil), types.NewStruct(nil, nil), nil)
		configureSig := types.NewSignatureType(types.NewVar(token.NoPos, pkg, "", owner), nil, nil,
			types.NewTuple(types.NewVar(token.NoPos, pkg, "opts", big)),
			types.NewTuple(types.NewVar(token.NoPos, pkg, "", types.Typ[types.String])),
			false)
		owner.AddMethod(types.NewFunc(token.NoPos, pkg, "Configure", configureSig))

		e := execute(t, `{{.Missing}}`, owner)

		var sb strings.Builder
		require.NoError(t, e.DetailedError(&sb, webQualifier))
		require.Contains(t, sb.String(), "\n  func (w Widget) Configure(struct{...}) string { /* ... */ }")
	})

	t.Run("write errors are propagated", func(t *testing.T) {
		e := execute(t, `{{.Missing}}`, pageNamed)
		require.Error(t, e.DetailedError(failWriter{}, nil))
	})

	t.Run("promoted fields through embedded pointers are listed", func(t *testing.T) {
		base := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Base", nil),
			types.NewStruct([]*types.Var{
				types.NewField(token.NoPos, pkg, "Slug", types.Typ[types.String], false),
			}, nil), nil)
		data := types.NewStruct([]*types.Var{
			types.NewField(token.NoPos, pkg, "Base", types.NewPointer(base), true),
		}, nil)

		e := execute(t, `{{.Missing}}`, data)

		var sb strings.Builder
		require.NoError(t, e.DetailedError(&sb, webQualifier))
		require.Contains(t, sb.String(), "\n    Slug string")
	})

	t.Run("aggregates render one block per failure", func(t *testing.T) {
		e := execute(t, `{{.A}}{{.B}}`, pageNamed)

		var sb strings.Builder
		require.NoError(t, e.DetailedError(&sb, webQualifier))
		blocks := strings.Split(sb.String(), "\n\n")
		require.GreaterOrEqual(t, len(blocks), 2)
		require.Contains(t, sb.String(), "field or method A not found")
		require.Contains(t, sb.String(), "field or method B not found")
	})
}

func TestExecute_error_classification(t *testing.T) {
	pkg := types.NewPackage("example.com/app", "app")
	emptyStruct := types.NewStruct(nil, nil)
	boolStruct := types.NewStruct([]*types.Var{
		types.NewField(token.NoPos, pkg, "Ok", types.Typ[types.Bool], false),
	}, nil)

	badMethodOwner := func() types.Type {
		named := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "BadMethod", nil), types.NewStruct(nil, nil), nil)
		results := types.NewTuple(
			types.NewVar(token.NoPos, pkg, "", types.Typ[types.String]),
			types.NewVar(token.NoPos, pkg, "", types.Typ[types.Int]),
		)
		sig := types.NewSignatureType(types.NewVar(token.NoPos, pkg, "", named), nil, nil, nil, results, false)
		named.AddMethod(types.NewFunc(token.NoPos, pkg, "Method", sig))
		return named
	}()

	for _, tt := range []struct {
		Name     string
		Template string
		Data     types.Type
		Want     check.ErrorType
		WantX    string
	}{
		{
			Name:     "field or method not found",
			Template: `{{.Missing}}`,
			Data:     emptyStruct,
			Want:     check.ErrorTypeFieldOrMethodNotFound,
			WantX:    "struct{}",
		},
		{
			Name:     "field not exported",
			Template: `{{.missing}}`,
			Data:     emptyStruct,
			Want:     check.ErrorTypeFieldNotExported,
			WantX:    "struct{}",
		},
		{
			Name:     "constant overflows int",
			Template: `{{18446744073709551615}}`,
			Data:     emptyStruct,
			Want:     check.ErrorTypeConstantOverflow,
		},
		{
			Name:     "template not found",
			Template: `{{template "missing"}}`,
			Data:     emptyStruct,
			Want:     check.ErrorTypeTemplateNotFound,
		},
		{
			Name:     "argument given to non-function",
			Template: `{{"hello" 1}}`,
			Data:     emptyStruct,
			Want:     check.ErrorTypeNotAFunction,
		},
		{
			Name:     "nil is not a command",
			Template: `{{nil}}`,
			Data:     emptyStruct,
			Want:     check.ErrorTypeBadCommand,
		},
		{
			Name:     "bad built-in call argument",
			Template: `{{len .Ok}}`,
			Data:     boolStruct,
			Want:     check.ErrorTypeCallArguments,
		},
		{
			Name:     "range over unsupported type",
			Template: `{{range .Ok}}{{end}}`,
			Data:     boolStruct,
			Want:     check.ErrorTypeRange,
			WantX:    "bool",
		},
		{
			Name:     "bad method signature",
			Template: `{{.Method}}`,
			Data:     badMethodOwner,
			Want:     check.ErrorTypeBadSignature,
		},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			tmpl, err := template.New[*types.Signature]("classify.gohtml").Parse(tt.Template)
			require.NoError(t, err)

			global := check.NewGlobal(pkg, token.NewFileSet(), tmpl, check.Functions{})
			checkErr := check.Execute(global, tmpl.Tree(), tt.Data)

			leaf := findLeafError(t, checkErr)
			require.Equal(t, tt.Want, leaf.Type, "Execute(%q) leaf classified as %v, want %v", tt.Template, leaf.Type, tt.Want)
			require.NotNil(t, leaf.Tree, "leaf error should reference its template.Tree")
			require.NotNil(t, leaf.Node, "leaf error should reference its template.Node")
			if tt.WantX != "" {
				require.NotNil(t, leaf.X, "leaf error should carry the relevant types.Type")
				require.Equal(t, tt.WantX, leaf.X.String())
			}
		})
	}

	t.Run("unknown function", func(t *testing.T) {
		// The template parser does not resolve function names, so an
		// unknown one reaches the checker.
		tmpl, err := template.New[*types.Signature]("unknown-func.gohtml").Parse(`{{foo}}`)
		require.NoError(t, err)
		tree := tmpl.Tree()

		global := check.NewGlobal(pkg, token.NewFileSet(), check.FindTreeFunc(func(string) (*template.Tree, bool) {
			return nil, false
		}), check.Functions{})
		checkErr := check.Execute(global, tree, emptyStruct)

		leaf := findLeafError(t, checkErr)
		require.Equal(t, check.ErrorTypeUnknownFunction, leaf.Type)
	})

	t.Run("aggregate", func(t *testing.T) {
		tmpl, err := template.New[*types.Signature]("classify.gohtml").Parse(`{{.A}}{{.B}}`)
		require.NoError(t, err)

		global := check.NewGlobal(pkg, token.NewFileSet(), tmpl, check.Functions{})
		checkErr := check.Execute(global, tmpl.Tree(), emptyStruct)

		var root *check.Error
		require.ErrorAs(t, checkErr, &root)
		require.Equal(t, check.ErrorTypeAggregate, root.Type)
	})
}

// failWriter always fails so tests can observe write-error propagation.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, fmt.Errorf("sink closed") }

func TestError_Decl(t *testing.T) {
	pkg := types.NewPackage("example.com/web", "web")
	fset := token.NewFileSet()
	file := fset.AddFile("page.go", -1, 100)
	declPos := file.Pos(10)

	pageNamed := types.NewNamed(
		types.NewTypeName(declPos, pkg, "Page", nil),
		types.NewStruct(nil, nil), nil)

	tmpl, err := template.New[*types.Signature]("decl.gohtml").Parse(`{{.Missing}}`)
	require.NoError(t, err)
	global := check.NewGlobal(pkg, fset, tmpl, check.Functions{})
	checkErr := check.Execute(global, tmpl.Tree(), pageNamed)

	leaf := findLeafError(t, checkErr)
	require.True(t, leaf.Decl.IsValid(), "Decl should point at the receiver type declaration")
	require.Equal(t, fset.Position(declPos), leaf.Decl)
}

func TestExecute_secondary_errors(t *testing.T) {
	pkg := types.NewPackage("example.com/web", "web")
	emptyStruct := types.NewStruct(nil, nil)

	tmpl, err := template.New[*types.Signature]("secondary.gohtml").Parse(`{{$x := .Missing}}{{$x}}`)
	require.NoError(t, err)
	global := check.NewGlobal(pkg, token.NewFileSet(), tmpl, check.Functions{})
	checkErr := check.Execute(global, tmpl.Tree(), emptyStruct)

	var root *check.Error
	require.ErrorAs(t, checkErr, &root)
	var leaves []*check.Error
	for e := range root.All {
		if e.Type != check.ErrorTypeAggregate {
			leaves = append(leaves, e)
		}
	}
	require.Len(t, leaves, 2)

	require.Equal(t, check.ErrorTypeFieldOrMethodNotFound, leaves[0].Type)
	require.False(t, leaves[0].Secondary, "the root cause is not secondary")

	require.Equal(t, check.ErrorTypeVariableNotFound, leaves[1].Type)
	require.True(t, leaves[1].Secondary,
		"a variable lookup that failed because its declaration pipeline failed is a follow-on")
}

// findLeafError walks the error tree depth first and returns the first
// non-aggregate error.
func findLeafError(t *testing.T, err error) *check.Error {
	t.Helper()
	require.Error(t, err)
	var root *check.Error
	require.ErrorAs(t, err, &root)
	for e := range root.All {
		if e.Type != check.ErrorTypeAggregate {
			return e
		}
	}
	t.Fatalf("no leaf error found in %v", err)
	return nil
}

func TestExecute_multiple_errors(t *testing.T) {
	pkg := types.NewPackage("example.com/app", "app")
	emptyStruct := types.NewStruct(nil, nil)

	for _, tt := range []struct {
		Name     string
		Template string
		Data     types.Type
		Contains []string
	}{
		{
			Name:     "each bad action in a list is reported",
			Template: `{{.Name}}{{.Age}}`,
			Data:     emptyStruct,
			Contains: []string{
				"field or method Name not found",
				"field or method Age not found",
			},
		},
		{
			Name:     "the if pipeline and both branches are reported",
			Template: `{{if .Missing}}{{.A}}{{else}}{{.B}}{{end}}`,
			Data:     emptyStruct,
			Contains: []string{
				"field or method Missing not found",
				"field or method A not found",
				"field or method B not found",
			},
		},
		{
			Name:     "the range body and else list are reported",
			Template: `{{range .Items}}{{.A}}{{else}}{{.B}}{{end}}`,
			Data: types.NewStruct([]*types.Var{
				types.NewField(token.NoPos, pkg, "Items", types.NewSlice(emptyStruct), false),
			}, nil),
			Contains: []string{
				"field or method A not found",
				"field or method B not found",
			},
		},
		{
			Name:     "each bad attribute and child of an HTML element is reported",
			Template: `<p class="{{.A}}">{{.B}}</p>`,
			Data:     emptyStruct,
			Contains: []string{
				"field or method A not found",
				"field or method B not found",
			},
		},
		{
			Name:     "the block pipeline and body are reported",
			Template: `{{block "b" .Missing}}{{.A}}{{end}}`,
			Data:     emptyStruct,
			Contains: []string{
				"field or method Missing not found",
			},
		},
		{
			Name:     "the with pipeline and else list are reported",
			Template: `{{with .Missing}}{{.A}}{{else}}{{.B}}{{end}}`,
			Data:     emptyStruct,
			Contains: []string{
				"field or method Missing not found",
				"field or method B not found",
			},
		},
		{
			Name:     "each bad argument in a command is reported",
			Template: `{{index .A .B}}`,
			Data:     emptyStruct,
			Contains: []string{
				"field or method A not found",
				"field or method B not found",
			},
		},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			tmpl, err := template.New[*types.Signature]("multi.gohtml").Parse(tt.Template)
			require.NoError(t, err)

			global := check.NewGlobal(pkg, token.NewFileSet(), tmpl, check.Functions{})
			checkErr := check.Execute(global, tmpl.Tree(), tt.Data)

			require.Error(t, checkErr)
			var e *check.Error
			require.ErrorAs(t, checkErr, &e)

			message := checkErr.Error()
			for _, want := range tt.Contains {
				require.Contains(t, message, want)
			}
			gotLines := strings.Count(message, "\n") + 1
			require.Equal(t, len(tt.Contains), gotLines,
				"Execute(%q) should report one line per failure, got:\n%s", tt.Template, message)
		})
	}
}

// TestError_locationMatchesTextTemplate checks that Error reports a field
// chain where text/template does: at the field after the first segment,
// which is where text/template's parser places a chained node.
func TestError_locationMatchesTextTemplate(t *testing.T) {
	pkg := types.NewPackage("example.com/app", "app")
	inner := types.NewStruct(nil, nil)
	data := types.NewStruct([]*types.Var{
		types.NewField(token.NoPos, pkg, "Result", inner, false),
	}, nil)

	for _, tt := range []struct {
		Name     string
		Template string
		Want     string
	}{
		{
			Name:     "a single field is located at its dot",
			Template: `{{.Missing}}`,
			Want:     `location.gohtml:1:3: executing "location.gohtml" at <.Missing>: field or method Missing not found on struct{Result struct{}}`,
		},
		{
			Name:     "a field chain is located at its second field",
			Template: `<h1>Fruit {{.Result.Name}}</h1>`,
			Want:     `location.gohtml:1:20: executing "location.gohtml" at <.Result.Name>: field or method Name not found on struct{}`,
		},
		{
			Name:     "a variable chain is located at its first field",
			Template: `{{$x := .}}{{$x.Missing}}`,
			Want:     `location.gohtml:1:16: executing "location.gohtml" at <$x.Missing>: field or method Missing not found on struct{Result struct{}}`,
		},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			tmpl, err := template.New[*types.Signature]("location.gohtml").Parse(tt.Template)
			require.NoError(t, err)
			global := check.NewGlobal(pkg, token.NewFileSet(), tmpl, check.Functions{})

			checkErr := check.Execute(global, tmpl.Tree(), data)

			require.EqualError(t, checkErr, tt.Want)
		})
	}
}

// TestExecute_interfaceFieldAccess checks that, by default, a field looked
// up on an interface value is an error, as upstream reports it; the
// warning is opt-in through Global.WarnInterfaceFieldAccess.
func TestExecute_interfaceFieldAccess(t *testing.T) {
	pkg := types.NewPackage("example.com/app", "app")
	data := types.NewStruct([]*types.Var{
		types.NewField(token.NoPos, pkg, "Result", types.Universe.Lookup("any").Type(), false),
	}, nil)
	tmpl, err := template.New[*types.Signature]("iface.gohtml").Parse(`{{.Result.Name}}`)
	require.NoError(t, err)

	t.Run("an error by default", func(t *testing.T) {
		global := check.NewGlobal(pkg, token.NewFileSet(), tmpl, check.Functions{})
		checkErr := check.Execute(global, tmpl.Tree(), data)
		require.EqualError(t, checkErr, `iface.gohtml:1:10: executing "iface.gohtml" at <.Result.Name>: field or method Name not found on any`)
		require.Equal(t, check.ErrorTypeFieldOrMethodNotFound, findLeafError(t, checkErr).Type)
	})

	t.Run("a warning when opted in", func(t *testing.T) {
		global := check.NewGlobal(pkg, token.NewFileSet(), tmpl, check.Functions{})
		global.WarnInterfaceFieldAccess = true
		var ws []check.Warning
		global.InspectWarning = func(w check.Warning) { ws = append(ws, w) }
		require.NoError(t, check.Execute(global, tmpl.Tree(), data))
		require.Len(t, ws, 1)
		require.Equal(t, check.WarnInterfaceFieldAccess, ws[0].Code)
	})
}
