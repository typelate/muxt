package muxt

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/template"

	"github.com/crhntr/muxt/internal/source"
)

type Case[F any] struct {
	generated  bool
	start, end int
	Name       string
	Template   string
	GivenFunc  F
	WhenFunc   F
	ThenFunc   F
}

func generateTests(wd string, config RoutesFileConfiguration, templates []Template) (string, error) {
	fileName := filepath.Join(wd, config.TestsFileName)
	if config.PreviousTests == "" {
		config.PreviousTests = fmt.Sprintf(defaultTestFile, config.PackageName, config.RoutesFunction)
	}
	fileSet := token.NewFileSet()

	fileBuffer := []byte(config.PreviousTests)

	testFile, err := parser.ParseFile(fileSet, fileName, fileBuffer, parser.AllErrors|parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return "", err
	}

	buf := bytes.NewBuffer(nil)

	testFuncName := "Test" + config.RoutesFunction

	for _, decl := range testFile.Decls {
		testFunc, ok := decl.(*ast.FuncDecl)
		if !ok || testFunc.Name.Name != testFuncName {
			continue
		}
		for _, stmt := range testFunc.Body.List {
			cl, ok := findCasesLoop(stmt)
			if !ok {
				continue
			}
			ec := existingCases(fileSet, cl)
			if err := generateNewTestCases(buf, config, templates, fileSet, ec); err != nil {
				return "", err
			}
			insertNewAt := fileSet.Position(cl.End()).Offset - 1
			fileBuffer = slices.Insert(fileBuffer, insertNewAt, []byte(buf.String())...)
			return string(fileBuffer), nil
		}
	}

	return "", fmt.Errorf("function %s not found in %s", testFuncName, config.TestsFileName)
}

func existingCases(fileSet *token.FileSet, cl *ast.CompositeLit) []Case[*ast.FuncLit] {
	result := make([]Case[*ast.FuncLit], 0, len(cl.Elts))
	for _, elt := range cl.Elts {
		caseLit, ok := elt.(*ast.CompositeLit)
		if !ok {
			continue
		}
		result = append(result, parseExistingCase(fileSet, elt, caseLit))
	}
	return result
}

func findCasesLoop(stmt ast.Stmt) (*ast.CompositeLit, bool) {
	rs, ok := stmt.(*ast.RangeStmt)
	if !ok {
		return nil, false
	}
	cl, ok := rs.X.(*ast.CompositeLit)
	if !ok {
		return nil, false
	}
	at, ok := cl.Type.(*ast.ArrayType)
	if !ok || at.Elt.(*ast.Ident).Name != "Case" {
		return nil, false
	}
	return cl, true
}

func generateNewTestCases(buf *bytes.Buffer, config RoutesFileConfiguration, templates []Template, fileSet *token.FileSet, existingCases []Case[*ast.FuncLit]) error {
	buf.Reset()
	var newCases []Case[*ast.FuncLit]
	templatesWithTests := make(map[string]struct{})
	for _, testCase := range existingCases {
		templatesWithTests[testCase.Template] = struct{}{}
	}

	for _, t := range templates {
		if _, ok := templatesWithTests[t.name]; ok {
			continue
		}
		newCases = append(newCases, newCase(config, t))
	}

	strCases := make([]string, 0, len(newCases))
	for _, tc := range newCases {
		strCase, err := renderCaseFunctions(buf, fileSet, tc)
		if err != nil {
			return err
		}
		if err := renderCase(buf, strCase); err != nil {
			return err
		}
		strCases = append(strCases, buf.String())
	}

	joinedNewCases := strings.Join(strCases, ", ")
	if len(templatesWithTests) != 0 && joinedNewCases != "" {
		joinedNewCases = ", " + joinedNewCases
	}

	buf.Reset()
	buf.WriteString(joinedNewCases)
	return nil
}

func parseExistingCase(fileSet *token.FileSet, elt ast.Expr, caseLit *ast.CompositeLit) Case[*ast.FuncLit] {
	c := Case[*ast.FuncLit]{
		start: fileSet.Position(elt.Pos()).Offset,
		end:   fileSet.Position(elt.End()).Offset,
	}
	for _, fieldElt := range caseLit.Elts {
		kv, ok := fieldElt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		keyIdent := kv.Key.(*ast.Ident)
		switch keyIdent.Name {
		case "Name":
			c.Name, _ = strconv.Unquote(kv.Value.(*ast.BasicLit).Value)
		case "Template":
			c.Template, _ = strconv.Unquote(kv.Value.(*ast.BasicLit).Value)
		case "Given":
			c.GivenFunc = kv.Value.(*ast.FuncLit)
		case "When":
			c.WhenFunc = kv.Value.(*ast.FuncLit)
		case "Then":
			c.ThenFunc = kv.Value.(*ast.FuncLit)
		}
	}
	return c
}

func renderCase(buf *bytes.Buffer, strCase Case[string]) error {
	buf.Reset()
	return template.Must(template.New("").Funcs(template.FuncMap{
		"stringsRepeat": strings.Repeat,
		"prefixLines": func(prefix, in string) string {
			lines := strings.Split(in, "\n")
			for i, l := range lines {
				lines[i] = prefix + l
			}
			return strings.Join(lines, "\n")
		},
		"stringTrimSpace": strings.TrimSpace,
	}).Parse( /* language=go */ `{
		Name: {{printf "%q" .Name}},
		Template: {{printf "%q" .Template}},
		{{- if .GivenFunc}}
		Given: {{.ThenFunc | prefixLines (stringsRepeat "\t" 2) | stringTrimSpace}},
		{{- end}}
		{{- if .WhenFunc}}
		When: {{.WhenFunc | prefixLines (stringsRepeat "\t" 2) | stringTrimSpace}},
		{{- end}}
		{{- if .ThenFunc}}
		Then: {{.ThenFunc | prefixLines (stringsRepeat "\t" 2) | stringTrimSpace}},
		{{- end}}
	}`)).Execute(buf, strCase)
}

func renderCaseFunctions(buf *bytes.Buffer, fileSet *token.FileSet, astCase Case[*ast.FuncLit]) (Case[string], error) {
	buf.Reset()
	defer buf.Reset()
	strCase := Case[string]{
		start:     astCase.start,
		end:       astCase.end,
		generated: astCase.generated,

		Name:     astCase.Name,
		Template: astCase.Template,
	}

	if astCase.GivenFunc != nil {
		buf.Reset()
		if err := format.Node(buf, fileSet, astCase.GivenFunc); err != nil {
			return strCase, fmt.Errorf("failed to format Given function: %w", err)
		}
		strCase.GivenFunc = buf.String()
	}

	if astCase.WhenFunc != nil {
		buf.Reset()
		if err := format.Node(buf, fileSet, astCase.WhenFunc); err != nil {
			return strCase, fmt.Errorf("failed to format When function: %w", err)
		}
		strCase.WhenFunc = buf.String()
	}

	if astCase.ThenFunc != nil {
		buf.Reset()
		if err := format.Node(buf, fileSet, astCase.ThenFunc); err != nil {
			return strCase, fmt.Errorf("failed to format Then function: %w", err)
		}
		strCase.ThenFunc = buf.String()
	}

	return strCase, nil
}

func newCase(config RoutesFileConfiguration, template Template) Case[*ast.FuncLit] {
	whenLit := &ast.FuncLit{
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{
				{Names: []*ast.Ident{ast.NewIdent("t")}, Type: &ast.StarExpr{X: &ast.SelectorExpr{
					X:   ast.NewIdent("testing"),
					Sel: ast.NewIdent("T"),
				}}},
				{Names: []*ast.Ident{ast.NewIdent("when")}, Type: ast.NewIdent("When")},
			}},
			Results: &ast.FieldList{List: []*ast.Field{
				{Type: &ast.StarExpr{X: &ast.SelectorExpr{
					X:   ast.NewIdent("http"),
					Sel: ast.NewIdent("Request"),
				}}},
			}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Tok: token.DEFINE,
				Rhs: []ast.Expr{&ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X:   ast.NewIdent("httptest"),
						Sel: ast.NewIdent("NewRequest"),
					},
					Args: []ast.Expr{
						source.String(template.method),
						&ast.CallExpr{
							Fun: &ast.SelectorExpr{
								X: &ast.CompositeLit{
									Type: ast.NewIdent(config.TemplateRoutePathsTypeName),
									Elts: []ast.Expr{},
								},
								Sel: ast.NewIdent(template.identifier),
							},
							Args: []ast.Expr{},
						},
						source.Nil(),
					},
				}},
				Lhs: []ast.Expr{ast.NewIdent("request")},
			},
			&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent("request")}},
		}},
	}
	thenLit := &ast.FuncLit{
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{
				{Names: []*ast.Ident{ast.NewIdent("t")}, Type: &ast.StarExpr{X: &ast.SelectorExpr{
					X:   ast.NewIdent("testing"),
					Sel: ast.NewIdent("T"),
				}}},
				{Names: []*ast.Ident{ast.NewIdent("then")}, Type: ast.NewIdent("Then")},
				{Names: []*ast.Ident{ast.NewIdent("response")}, Type: &ast.StarExpr{X: &ast.SelectorExpr{
					X:   ast.NewIdent("http"),
					Sel: ast.NewIdent("Response"),
				}}},
			}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.IfStmt{
				Init: &ast.AssignStmt{
					Tok: token.DEFINE,
					Lhs: []ast.Expr{
						ast.NewIdent("expected"),
						ast.NewIdent("got"),
					},
					Rhs: []ast.Expr{
						source.HTTPStatusCode("http", template.defaultStatusCode),
						&ast.SelectorExpr{
							X:   ast.NewIdent("response"),
							Sel: ast.NewIdent("StatusCode"),
						},
					},
				},
				Cond: &ast.BinaryExpr{X: ast.NewIdent("expected"), Op: token.NEQ, Y: ast.NewIdent("got")},
				Body: &ast.BlockStmt{List: []ast.Stmt{
					// t.Fatal("test case field When must not be nil")
					&ast.ExprStmt{X: &ast.CallExpr{
						Fun:  &ast.SelectorExpr{X: ast.NewIdent("t"), Sel: ast.NewIdent("Errorf")},
						Args: []ast.Expr{source.String("unexpected status code: got %d expected %d"), ast.NewIdent("got"), ast.NewIdent("expected")},
					}},
				}},
			},
		}},
	}

	return Case[*ast.FuncLit]{
		generated: true,
		Name:      template.identifier,
		Template:  template.name,
		GivenFunc: nil,
		WhenFunc:  whenLit,
		ThenFunc:  thenLit,
	}
}

const defaultTestFile = `package %[1]s

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test%[2]s(t *testing.T) {
	// The Given, When, Then, and Case structures and the runCase function are only generated once.
	// You may add fields to any structure. Do not alter the signature of any Given, When, or Then function on Case.
	// You may edit the body of runCase (and the for case loop body).
	//
	// Consider if you want your collaborator test seam to be RoutesReceiver or your interface implementation's
	// collaborators. This generated test function works well either way. If you use RoutesReceiver as a seam, consider
	// using a mock generator like https://pkg.go.dev/github.com/maxbrunsfeld/counterfeiter/v6 or https://pkg.go.dev/github.com/ryanmoran/faux
	// If you decide to cover your RoutesReceiver testing with this test function. Add the receiver's collaborator
	// test doubles to the Given and Then structures so you can configure and make assertions in the respective
	// Given and Then test hooks for each case.
	type (
		// Given is the scope used for setting up test case collaborators.
		Given struct{}

		// When is the scope used to create HTTP Requests. It is unlikely you will need to add additional fields.
		When struct{}

		// Then is the scope used for test case assertions. It will likely have collaborator test doubles.
		Then struct{}

		Case struct {
			// The Name, by default a generated identifier, you may change this.
			Name string

			// The Template field is the route template being tested. It is used by the test generator to detect
			// which templates are being tested. Do not change this.
			Template string

			// The "Given" function MAY set up collaborators.
			// The code generator does not add this field in newly generated test cases.
			Given func(t *testing.T, given Given)

			// The "When" function MUST set up an HTTP Request.
			// The generated function will call httptest.NewRequest using the appropriate method and
			// the generated TemplateRoutePaths path constructor method.
			When func(t *testing.T, when When) *http.Request

			// The "Then" function MAY make assertions on response or any configured collaborators.
			// The generated function will assert that the response.StatusCode matches the expected status code.
			//
			// Consider using https://pkg.go.dev/github.com/stretchr/testify for assertions
			// and https://pkg.go.dev/github.com/crhntr/dom/domtest for interacting with the HTML body.
			Then func(t *testing.T, then Then, response *http.Response)
		}
	)

	runCase := func(t *testing.T, tc Case) {
		if tc.When == nil {
			t.Fatal("test case field When must not be nil")
		}
		if tc.Then == nil {
			t.Fatal("test case field Then must not be nil")
		}
		if tc.Template == "" {
			t.Fatal("test case field Template must not be empty")
		}

		// If you need to do universal setup of your receiver, do that here.

		var receiver RoutesReceiver = nil
		mux := http.NewServeMux()
		%[2]s(mux, receiver)
		if tc.Given != nil {
			tc.Given(t, Given{})
		}
		request := tc.When(t, When{})
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)

		// If you want to do universal assertions of all your endpoints, consider writing a helper function
		// and calling it here.

		if tc.Then != nil {
			tc.Then(t, Then{}, recorder.Result())
		}
	}

	for _, tc := range []Case{} {
		t.Run(tc.Name, func(t *testing.T) { runCase(t, tc) })
	}
}
`
