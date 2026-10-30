package template

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/template/lex"
)

var debug = flag.Bool("debug", false, "show the errors produced by the main tests")

func init() {
	// Use a small stack limit for testing to avoid creating huge expressions.
	maxStackDepth = 3
}

type parseTest struct {
	name   string
	input  string
	ok     bool
	result string // what the user would see in an error message.
	exp    Node   // expected AST root (nil for error cases)
}

const (
	noError  = true
	hasError = false
)

func TestParse(t *testing.T) {
	for _, test := range []parseTest{
		{
			name:  "empty",
			input: "",
			ok:    noError,
			exp:   &ListNode{},
		},
		{
			name:  "comment",
			input: "{{/*\n\n\n*/}}",
			ok:    noError,
			exp:   &ListNode{},
		},
		{
			name:   "spaces",
			input:  " \t\n",
			ok:     noError,
			result: `" \t\n"`,
			exp:    &ListNode{Nodes: []Node{&TextNode{Text: []byte(" \t\n")}}},
		},
		{
			name:   "text",
			input:  "some text",
			ok:     noError,
			result: `"some text"`,
			exp:    &ListNode{Nodes: []Node{&TextNode{Text: []byte("some text")}}},
		},
		{
			name:   "emptyAction",
			input:  "{{}}",
			ok:     hasError,
			result: `{{}}`,
		},
		{
			name:   "field",
			input:  "{{.X}}",
			ok:     noError,
			result: `{{.X}}`,
			exp:    &ListNode{Nodes: []Node{&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"X"}}}}}}}}},
		},
		{
			name:   "simple command",
			input:  "{{printf}}",
			ok:     noError,
			result: `{{printf}}`,
			exp:    &ListNode{Nodes: []Node{&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&IdentifierNode{Ident: "printf"}}}}}}}},
		},
		{
			name:   "$ invocation",
			input:  "{{$}}",
			ok:     noError,
			result: "{{$}}",
			exp:    &ListNode{Nodes: []Node{&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&VariableNode{Ident: []string{"$"}}}}}}}}},
		},
		{
			name:   "variable invocation",
			input:  "{{with $x := 3}}{{$x 23}}{{end}}",
			ok:     noError,
			result: "{{with $x := 3}}{{$x 23}}{{end}}",
			exp: &ListNode{Nodes: []Node{&WithNode{BranchNode: BranchNode{
				Pipe: &PipeNode{Decl: []*VariableNode{{Ident: []string{"$x"}}}, Commands: []*CommandNode{{Args: []Node{&NumberNode{IsInt: true, IsUint: true, IsFloat: true, Int64: 3, Uint64: 3, Float64: 3, Text: "3"}}}}},
				List: &ListNode{Nodes: []Node{&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{
					&VariableNode{Ident: []string{"$x"}},
					&NumberNode{IsInt: true, IsUint: true, IsFloat: true, Int64: 23, Uint64: 23, Float64: 23, Text: "23"},
				}}}}}}},
			}}}},
		},
		{
			name:   "variable with fields",
			input:  "{{$.I}}",
			ok:     noError,
			result: "{{$.I}}",
			exp:    &ListNode{Nodes: []Node{&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&VariableNode{Ident: []string{"$", "I"}}}}}}}}},
		},
		{
			name:   "multi-word command",
			input:  "{{printf `%d` 23}}",
			ok:     noError,
			result: "{{printf `%d` 23}}",
			exp: &ListNode{Nodes: []Node{&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{
				&IdentifierNode{Ident: "printf"},
				&StringNode{Quoted: "`%d`", Text: "%d"},
				&NumberNode{IsInt: true, IsUint: true, IsFloat: true, Int64: 23, Uint64: 23, Float64: 23, Text: "23"},
			}}}}}}},
		},
		{
			name:   "pipeline",
			input:  "{{.X|.Y}}",
			ok:     noError,
			result: `{{.X | .Y}}`,
			exp: &ListNode{Nodes: []Node{&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{
				{Args: []Node{&FieldNode{Ident: []string{"X"}}}},
				{Args: []Node{&FieldNode{Ident: []string{"Y"}}}},
			}}}}},
		},
		{
			name:   "pipeline with decl",
			input:  "{{$x := .X|.Y}}",
			ok:     noError,
			result: `{{$x := .X | .Y}}`,
			exp: &ListNode{Nodes: []Node{&ActionNode{Pipe: &PipeNode{
				Decl: []*VariableNode{{Ident: []string{"$x"}}},
				Commands: []*CommandNode{
					{Args: []Node{&FieldNode{Ident: []string{"X"}}}},
					{Args: []Node{&FieldNode{Ident: []string{"Y"}}}},
				},
			}}}},
		},
		{
			name:   "nested pipeline",
			input:  "{{.X (.Y .Z) (.A | .B .C) (.E)}}",
			ok:     noError,
			result: `{{.X (.Y .Z) (.A | .B .C) (.E)}}`,
			exp: &ListNode{Nodes: []Node{&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{
				&FieldNode{Ident: []string{"X"}},
				&PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Y"}}, &FieldNode{Ident: []string{"Z"}}}}}},
				&PipeNode{Commands: []*CommandNode{
					{Args: []Node{&FieldNode{Ident: []string{"A"}}}},
					{Args: []Node{&FieldNode{Ident: []string{"B"}}, &FieldNode{Ident: []string{"C"}}}},
				}},
				&PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"E"}}}}}},
			}}}}}}},
		},
		{
			name:   "field applied to parentheses",
			input:  "{{(.Y .Z).Field}}",
			ok:     noError,
			result: `{{(.Y .Z).Field}}`,
			exp: &ListNode{Nodes: []Node{&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{
				&ChainNode{Node: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Y"}}, &FieldNode{Ident: []string{"Z"}}}}}}, Field: []string{"Field"}},
			}}}}}}},
		},
		{
			name:   "simple if",
			input:  "{{if .X}}hello{{end}}",
			ok:     noError,
			result: `{{if .X}}"hello"{{end}}`,
			exp: &ListNode{Nodes: []Node{&IfNode{BranchNode: BranchNode{
				Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"X"}}}}}},
				List: &ListNode{Nodes: []Node{&TextNode{Text: []byte("hello")}}},
			}}}},
		},
		{
			name:   "if with else",
			input:  "{{if .X}}true{{else}}false{{end}}",
			ok:     noError,
			result: `{{if .X}}"true"{{else}}"false"{{end}}`,
			exp: &ListNode{Nodes: []Node{&IfNode{BranchNode: BranchNode{
				Pipe:     &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"X"}}}}}},
				List:     &ListNode{Nodes: []Node{&TextNode{Text: []byte("true")}}},
				ElseList: &ListNode{Nodes: []Node{&TextNode{Text: []byte("false")}}},
			}}}},
		},
		{
			name:   "if with else if",
			input:  "{{if .X}}true{{else if .Y}}false{{end}}",
			ok:     noError,
			result: `{{if .X}}"true"{{else}}{{if .Y}}"false"{{end}}{{end}}`,
			exp: &ListNode{Nodes: []Node{&IfNode{BranchNode: BranchNode{
				Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"X"}}}}}},
				List: &ListNode{Nodes: []Node{&TextNode{Text: []byte("true")}}},
				ElseList: &ListNode{Nodes: []Node{&IfNode{BranchNode: BranchNode{
					Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Y"}}}}}},
					List: &ListNode{Nodes: []Node{&TextNode{Text: []byte("false")}}},
				}}}},
			}}}},
		},
		{
			name:   "if else chain",
			input:  "+{{if .X}}X{{else if .Y}}Y{{else if .Z}}Z{{end}}+",
			ok:     noError,
			result: `"+"{{if .X}}"X"{{else}}{{if .Y}}"Y"{{else}}{{if .Z}}"Z"{{end}}{{end}}{{end}}"+"`,
			exp: &ListNode{Nodes: []Node{
				&TextNode{Text: []byte("+")},
				&IfNode{BranchNode: BranchNode{
					Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"X"}}}}}},
					List: &ListNode{Nodes: []Node{&TextNode{Text: []byte("X")}}},
					ElseList: &ListNode{Nodes: []Node{&IfNode{BranchNode: BranchNode{
						Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Y"}}}}}},
						List: &ListNode{Nodes: []Node{&TextNode{Text: []byte("Y")}}},
						ElseList: &ListNode{Nodes: []Node{&IfNode{BranchNode: BranchNode{
							Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Z"}}}}}},
							List: &ListNode{Nodes: []Node{&TextNode{Text: []byte("Z")}}},
						}}}},
					}}}},
				}},
				&TextNode{Text: []byte("+")},
			}},
		},
		{
			name:   "simple range",
			input:  "{{range .X}}hello{{end}}",
			ok:     noError,
			result: `{{range .X}}"hello"{{end}}`,
			exp: &ListNode{Nodes: []Node{&RangeNode{BranchNode: BranchNode{
				Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"X"}}}}}},
				List: &ListNode{Nodes: []Node{&TextNode{Text: []byte("hello")}}},
			}}}},
		},
		{
			name:   "chained field range",
			input:  "{{range .X.Y.Z}}hello{{end}}",
			ok:     noError,
			result: `{{range .X.Y.Z}}"hello"{{end}}`,
			exp: &ListNode{Nodes: []Node{&RangeNode{BranchNode: BranchNode{
				Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"X", "Y", "Z"}}}}}},
				List: &ListNode{Nodes: []Node{&TextNode{Text: []byte("hello")}}},
			}}}},
		},
		{
			name:   "nested range",
			input:  "{{range .X}}hello{{range .Y}}goodbye{{end}}{{end}}",
			ok:     noError,
			result: `{{range .X}}"hello"{{range .Y}}"goodbye"{{end}}{{end}}`,
			exp: &ListNode{Nodes: []Node{&RangeNode{BranchNode: BranchNode{
				Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"X"}}}}}},
				List: &ListNode{Nodes: []Node{
					&TextNode{Text: []byte("hello")},
					&RangeNode{BranchNode: BranchNode{
						Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Y"}}}}}},
						List: &ListNode{Nodes: []Node{&TextNode{Text: []byte("goodbye")}}},
					}},
				}},
			}}}},
		},
		{
			name:   "range with else",
			input:  "{{range .X}}true{{else}}false{{end}}",
			ok:     noError,
			result: `{{range .X}}"true"{{else}}"false"{{end}}`,
			exp: &ListNode{Nodes: []Node{&RangeNode{BranchNode: BranchNode{
				Pipe:     &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"X"}}}}}},
				List:     &ListNode{Nodes: []Node{&TextNode{Text: []byte("true")}}},
				ElseList: &ListNode{Nodes: []Node{&TextNode{Text: []byte("false")}}},
			}}}},
		},
		{
			name:   "range over pipeline",
			input:  "{{range .X|.M}}true{{else}}false{{end}}",
			ok:     noError,
			result: `{{range .X | .M}}"true"{{else}}"false"{{end}}`,
			exp: &ListNode{Nodes: []Node{&RangeNode{BranchNode: BranchNode{
				Pipe: &PipeNode{Commands: []*CommandNode{
					{Args: []Node{&FieldNode{Ident: []string{"X"}}}},
					{Args: []Node{&FieldNode{Ident: []string{"M"}}}},
				}},
				List:     &ListNode{Nodes: []Node{&TextNode{Text: []byte("true")}}},
				ElseList: &ListNode{Nodes: []Node{&TextNode{Text: []byte("false")}}},
			}}}},
		},
		{
			name:   "range []int",
			input:  "{{range .SI}}{{.}}{{end}}",
			ok:     noError,
			result: `{{range .SI}}{{.}}{{end}}`,
			exp: &ListNode{Nodes: []Node{&RangeNode{BranchNode: BranchNode{
				Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"SI"}}}}}},
				List: &ListNode{Nodes: []Node{&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&DotNode{}}}}}}}},
			}}}},
		},
		{
			name:   "range 1 var",
			input:  "{{range $x := .SI}}{{.}}{{end}}",
			ok:     noError,
			result: `{{range $x := .SI}}{{.}}{{end}}`,
			exp: &ListNode{Nodes: []Node{&RangeNode{BranchNode: BranchNode{
				Pipe: &PipeNode{Decl: []*VariableNode{{Ident: []string{"$x"}}}, Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"SI"}}}}}},
				List: &ListNode{Nodes: []Node{&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&DotNode{}}}}}}}},
			}}}},
		},
		{
			name:   "range 2 vars",
			input:  "{{range $x, $y := .SI}}{{.}}{{end}}",
			ok:     noError,
			result: `{{range $x, $y := .SI}}{{.}}{{end}}`,
			exp: &ListNode{Nodes: []Node{&RangeNode{BranchNode: BranchNode{
				Pipe: &PipeNode{Decl: []*VariableNode{{Ident: []string{"$x"}}, {Ident: []string{"$y"}}}, Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"SI"}}}}}},
				List: &ListNode{Nodes: []Node{&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&DotNode{}}}}}}}},
			}}}},
		},
		{
			name:   "range with break",
			input:  "{{range .SI}}{{.}}{{break}}{{end}}",
			ok:     noError,
			result: `{{range .SI}}{{.}}{{break}}{{end}}`,
			exp: &ListNode{Nodes: []Node{&RangeNode{BranchNode: BranchNode{
				Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"SI"}}}}}},
				List: &ListNode{Nodes: []Node{
					&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&DotNode{}}}}}},
					&BreakNode{},
				}},
			}}}},
		},
		{
			name:   "range with continue",
			input:  "{{range .SI}}{{.}}{{continue}}{{end}}",
			ok:     noError,
			result: `{{range .SI}}{{.}}{{continue}}{{end}}`,
			exp: &ListNode{Nodes: []Node{&RangeNode{BranchNode: BranchNode{
				Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"SI"}}}}}},
				List: &ListNode{Nodes: []Node{
					&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&DotNode{}}}}}},
					&ContinueNode{},
				}},
			}}}},
		},
		{
			name:   "constants",
			input:  "{{range .SI 1 -3.2i true false 'a' nil}}{{end}}",
			ok:     noError,
			result: `{{range .SI 1 -3.2i true false 'a' nil}}{{end}}`,
			exp: &ListNode{Nodes: []Node{&RangeNode{BranchNode: BranchNode{
				Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{
					&FieldNode{Ident: []string{"SI"}},
					&NumberNode{IsInt: true, IsUint: true, IsFloat: true, Int64: 1, Uint64: 1, Float64: 1, Text: "1"},
					&NumberNode{IsComplex: true, Complex128: -3.2i, Text: "-3.2i"},
					&BoolNode{True: true},
					&BoolNode{True: false},
					&NumberNode{IsInt: true, IsUint: true, IsFloat: true, Int64: 'a', Uint64: 'a', Float64: float64('a'), Text: "'a'"},
					&NilNode{},
				}}}},
				List: &ListNode{},
			}}}},
		},
		{
			name:   "template",
			input:  "{{template `x`}}",
			ok:     noError,
			result: `{{template "x"}}`,
			exp:    &ListNode{Nodes: []Node{&TemplateNode{Name: "x"}}},
		},
		{
			name:   "template with arg",
			input:  "{{template `x` .Y}}",
			ok:     noError,
			result: `{{template "x" .Y}}`,
			exp:    &ListNode{Nodes: []Node{&TemplateNode{Name: "x", Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Y"}}}}}}}}},
		},
		{
			name:   "with",
			input:  "{{with .X}}hello{{end}}",
			ok:     noError,
			result: `{{with .X}}"hello"{{end}}`,
			exp: &ListNode{Nodes: []Node{&WithNode{BranchNode: BranchNode{
				Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"X"}}}}}},
				List: &ListNode{Nodes: []Node{&TextNode{Text: []byte("hello")}}},
			}}}},
		},
		{
			name:   "with with else",
			input:  "{{with .X}}hello{{else}}goodbye{{end}}",
			ok:     noError,
			result: `{{with .X}}"hello"{{else}}"goodbye"{{end}}`,
			exp: &ListNode{Nodes: []Node{&WithNode{BranchNode: BranchNode{
				Pipe:     &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"X"}}}}}},
				List:     &ListNode{Nodes: []Node{&TextNode{Text: []byte("hello")}}},
				ElseList: &ListNode{Nodes: []Node{&TextNode{Text: []byte("goodbye")}}},
			}}}},
		},
		{
			name:   "with with else with",
			input:  "{{with .X}}hello{{else with .Y}}goodbye{{end}}",
			ok:     noError,
			result: `{{with .X}}"hello"{{else}}{{with .Y}}"goodbye"{{end}}{{end}}`,
			exp: &ListNode{Nodes: []Node{&WithNode{BranchNode: BranchNode{
				Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"X"}}}}}},
				List: &ListNode{Nodes: []Node{&TextNode{Text: []byte("hello")}}},
				ElseList: &ListNode{Nodes: []Node{&WithNode{BranchNode: BranchNode{
					Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Y"}}}}}},
					List: &ListNode{Nodes: []Node{&TextNode{Text: []byte("goodbye")}}},
				}}}},
			}}}},
		},
		{
			name:   "with else chain",
			input:  "{{with .X}}X{{else with .Y}}Y{{else with .Z}}Z{{end}}",
			ok:     noError,
			result: `{{with .X}}"X"{{else}}{{with .Y}}"Y"{{else}}{{with .Z}}"Z"{{end}}{{end}}{{end}}`,
			exp: &ListNode{Nodes: []Node{&WithNode{BranchNode: BranchNode{
				Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"X"}}}}}},
				List: &ListNode{Nodes: []Node{&TextNode{Text: []byte("X")}}},
				ElseList: &ListNode{Nodes: []Node{&WithNode{BranchNode: BranchNode{
					Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Y"}}}}}},
					List: &ListNode{Nodes: []Node{&TextNode{Text: []byte("Y")}}},
					ElseList: &ListNode{Nodes: []Node{&WithNode{BranchNode: BranchNode{
						Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Z"}}}}}},
						List: &ListNode{Nodes: []Node{&TextNode{Text: []byte("Z")}}},
					}}}},
				}}}},
			}}}},
		},
		// Trimming spaces — the lexer preserves text content; trim is deferred to execution.
		{
			name:   "trim left",
			input:  "x \r\n\t{{- 3}}",
			ok:     noError,
			result: `"x \r\n\t"{{3}}`,
			exp: &ListNode{Nodes: []Node{
				&TextNode{Text: []byte("x \r\n\t")},
				&ActionNode{TrimLeft: true, Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&NumberNode{IsInt: true, IsUint: true, IsFloat: true, Int64: 3, Uint64: 3, Float64: 3, Text: "3"}}}}}},
			}},
		},
		{
			name:   "trim right",
			input:  "{{3 -}}\n\n\ty",
			ok:     noError,
			result: `{{3}}"\n\n\ty"`,
			exp: &ListNode{Nodes: []Node{
				&ActionNode{TrimRight: true, Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&NumberNode{IsInt: true, IsUint: true, IsFloat: true, Int64: 3, Uint64: 3, Float64: 3, Text: "3"}}}}}},
				&TextNode{Text: []byte("\n\n\ty")},
			}},
		},
		{
			name:   "trim left and right",
			input:  "x \r\n\t{{- 3 -}}\n\n\ty",
			ok:     noError,
			result: `"x \r\n\t"{{3}}"\n\n\ty"`,
			exp: &ListNode{Nodes: []Node{
				&TextNode{Text: []byte("x \r\n\t")},
				&ActionNode{TrimLeft: true, TrimRight: true, Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&NumberNode{IsInt: true, IsUint: true, IsFloat: true, Int64: 3, Uint64: 3, Float64: 3, Text: "3"}}}}}},
				&TextNode{Text: []byte("\n\n\ty")},
			}},
		},
		{
			name:   "trim with extra spaces",
			input:  "x\n{{-  3   -}}\ny",
			ok:     noError,
			result: `"x\n"{{3}}"\ny"`,
			exp: &ListNode{Nodes: []Node{
				&TextNode{Text: []byte("x\n")},
				&ActionNode{TrimLeft: true, TrimRight: true, Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&NumberNode{IsInt: true, IsUint: true, IsFloat: true, Int64: 3, Uint64: 3, Float64: 3, Text: "3"}}}}}},
				&TextNode{Text: []byte("\ny")},
			}},
		},
		{
			name:   "comment trim left",
			input:  "x \r\n\t{{- /* hi */}}",
			ok:     noError,
			result: `"x \r\n\t"`,
			exp:    &ListNode{Nodes: []Node{&TextNode{Text: []byte("x \r\n\t")}}},
		},
		{
			name:   "comment trim right",
			input:  "{{/* hi */ -}}\n\n\ty",
			ok:     noError,
			result: `"\n\n\ty"`,
			exp:    &ListNode{Nodes: []Node{&TextNode{Text: []byte("\n\n\ty")}}},
		},
		{
			name:   "comment trim left and right",
			input:  "x \r\n\t{{- /* */ -}}\n\n\ty",
			ok:     noError,
			result: `"x \r\n\t""\n\n\ty"`,
			exp:    &ListNode{Nodes: []Node{&TextNode{Text: []byte("x \r\n\t")}, &TextNode{Text: []byte("\n\n\ty")}}},
		},
		{
			name:   "block definition",
			input:  `{{block "foo" .}}hello{{end}}`,
			ok:     noError,
			result: `{{block "foo" .}}hello{{end}}`,
			exp: &ListNode{Nodes: []Node{&BlockNode{
				Name: "foo",
				Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&DotNode{}}}}},
				Body: &ListNode{Nodes: []Node{&TextNode{Text: []byte("hello")}}},
			}}},
		},
		{
			name:   "newline in assignment",
			input:  "{{ $x \n := \n 1 \n }}",
			ok:     noError,
			result: "{{$x := 1}}",
			exp:    &ListNode{Nodes: []Node{&ActionNode{Pipe: &PipeNode{Decl: []*VariableNode{{Ident: []string{"$x"}}}, Commands: []*CommandNode{{Args: []Node{&NumberNode{IsInt: true, IsUint: true, IsFloat: true, Int64: 1, Uint64: 1, Float64: 1, Text: "1"}}}}}}}},
		},
		{
			name:   "newline in empty action",
			input:  "{{\n}}",
			ok:     hasError,
			result: "{{\n}}",
		},
		{
			name:   "newline in pipeline",
			input:  "{{\n\"x\"\n|\nprintf\n}}",
			ok:     noError,
			result: `{{"x" | printf}}`,
			exp: &ListNode{Nodes: []Node{&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{
				{Args: []Node{&StringNode{Quoted: `"x"`, Text: "x"}}},
				{Args: []Node{&IdentifierNode{Ident: "printf"}}},
			}}}}},
		},
		{
			name:  "newline in comment",
			input: "{{/*\nhello\n*/}}",
			ok:    noError,
			exp:   &ListNode{Nodes: []Node{}},
		},
		{
			name:  "newline in comment",
			input: "{{-\n/*\nhello\n*/\n-}}",
			ok:    noError,
			exp:   &ListNode{Nodes: []Node{}},
		},
		{
			name:   "spaces around continue",
			input:  "{{range .SI}}{{.}}{{ continue }}{{end}}",
			ok:     noError,
			result: `{{range .SI}}{{.}}{{continue}}{{end}}`,
			exp: &ListNode{Nodes: []Node{&RangeNode{BranchNode: BranchNode{
				Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"SI"}}}}}},
				List: &ListNode{Nodes: []Node{
					&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&DotNode{}}}}}},
					&ContinueNode{},
				}},
			}}}},
		},
		{
			name:   "spaces around break",
			input:  "{{range .SI}}{{.}}{{ break }}{{end}}",
			ok:     noError,
			result: `{{range .SI}}{{.}}{{break}}{{end}}`,
			exp: &ListNode{Nodes: []Node{&RangeNode{BranchNode: BranchNode{
				Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"SI"}}}}}},
				List: &ListNode{Nodes: []Node{
					&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&DotNode{}}}}}},
					&BreakNode{},
				}},
			}}}},
		},

		// Errors.
		{
			name:  "unclosed action",
			input: "hello{{range",
			ok:    hasError,
		},
		{
			name:  "unmatched end",
			input: "{{end}}",
			ok:    hasError,
		},
		{
			name:  "unmatched else",
			input: "{{else}}",
			ok:    hasError,
		},
		{
			name:  "unmatched else after if",
			input: "{{if .X}}hello{{end}}{{else}}",
			ok:    hasError,
		},
		{
			name:  "multiple else",
			input: "{{if .X}}1{{else}}2{{else}}3{{end}}",
			ok:    hasError,
		},
		{
			name:  "missing end",
			input: "hello{{range .x}}",
			ok:    hasError,
		},
		{
			name:  "missing end after else",
			input: "hello{{range .x}}{{else}}",
			ok:    hasError,
		},
		{
			name:  "undefined function",
			input: "hello{{undefined}}",
			ok:    noError,
			exp: &ListNode{Nodes: []Node{
				&TextNode{Text: []byte("hello")},
				&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&IdentifierNode{Ident: "undefined"}}}}}},
			}},
		},
		{
			name:  "undefined variable",
			input: "{{$x}}",
			ok:    hasError,
		},
		{
			name:  "variable undefined after end",
			input: "{{with $x := 4}}{{end}}{{$x}}",
			ok:    hasError,
		},
		{
			name:  "variable undefined in template",
			input: "{{template $v}}",
			ok:    hasError,
		},
		{
			name:  "declare with field",
			input: "{{with $x.Y := 4}}{{end}}",
			ok:    hasError,
		},
		{
			name:  "template with field ref",
			input: "{{template .X}}",
			ok:    hasError,
		},
		{
			name:  "template with var",
			input: "{{template $v}}",
			ok:    hasError,
		},
		{
			name:  "invalid punctuation",
			input: "{{printf 3, 4}}",
			ok:    hasError,
		},
		{
			name:  "multidecl outside range",
			input: "{{with $v, $u := 3}}{{end}}",
			ok:    hasError,
		},
		{
			name:  "too many decls in range",
			input: "{{range $u, $v, $w := 3}}{{end}}",
			ok:    hasError,
		},
		{
			name:  "dot applied to parentheses",
			input: "{{printf (printf .).}}",
			ok:    hasError,
		},
		{
			name:  "adjacent args",
			input: "{{printf 3`x`}}",
			ok:    hasError,
		},
		{
			name:  "adjacent args with .",
			input: "{{printf `x`.}}",
			ok:    hasError,
		},
		{
			name:  "extra end after if",
			input: "{{if .X}}a{{else if .Y}}b{{end}}{{end}}",
			ok:    hasError,
		},
		{
			name:  "break outside range",
			input: "{{range .}}{{end}} {{break}}",
			ok:    hasError,
		},
		{
			name:  "continue outside range",
			input: "{{range .}}{{end}} {{continue}}",
			ok:    hasError,
		},
		{
			name:  "break in range else",
			input: "{{range .}}{{else}}{{break}}{{end}}",
			ok:    hasError,
		},
		{
			name:  "continue in range else",
			input: "{{range .}}{{else}}{{continue}}{{end}}",
			ok:    hasError,
		},
		// Other kinds of assignments and operators aren't available yet.
		{
			name:   "bug0a",
			input:  "{{$x := 0}}{{$x}}",
			ok:     noError,
			result: "{{$x := 0}}{{$x}}",
			exp: &ListNode{Nodes: []Node{
				&ActionNode{Pipe: &PipeNode{Decl: []*VariableNode{{Ident: []string{"$x"}}}, Commands: []*CommandNode{{Args: []Node{&NumberNode{IsInt: true, IsUint: true, IsFloat: true, Text: "0"}}}}}},
				&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&VariableNode{Ident: []string{"$x"}}}}}}},
			}},
		},
		{
			name:  "bug0b",
			input: "{{$x += 1}}{{$x}}",
			ok:    hasError,
		},
		{
			name:  "bug0c",
			input: "{{$x ! 2}}{{$x}}",
			ok:    hasError,
		},
		{
			name:  "bug0d",
			input: "{{$x % 3}}{{$x}}",
			ok:    hasError,
		},
		// Check the parse fails for := rather than comma.
		{
			name:  "bug0e",
			input: "{{range $x := $y := 3}}{{end}}",
			ok:    hasError,
		},
		// Another bug: variable read must ignore following punctuation.
		{
			name:  "bug1a",
			input: "{{$x:=.}}{{$x!2}}",
			ok:    hasError,
		}, // ! is just illegal here.
		{
			name:  "bug1b",
			input: "{{$x:=.}}{{$x+2}}",
			ok:    hasError,
		}, // $x+2 should not parse as ($x) (+2).
		{
			name:   "bug1c",
			input:  "{{$x:=.}}{{$x +2}}",
			ok:     noError,
			result: "{{$x := .}}{{$x +2}}", // It's OK with a space.
			exp: &ListNode{Nodes: []Node{
				&ActionNode{Pipe: &PipeNode{Decl: []*VariableNode{{Ident: []string{"$x"}}}, Commands: []*CommandNode{{Args: []Node{&DotNode{}}}}}},
				&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&VariableNode{Ident: []string{"$x"}}, &NumberNode{IsInt: true, IsFloat: true, Int64: 2, Float64: 2, Text: "+2"}}}}}},
			}},
		},
		// Check the range handles assignment vs. declaration properly.
		{
			name:   "bug2a",
			input:  "{{range $x := 0}}{{$x}}{{end}}",
			ok:     noError,
			result: "{{range $x := 0}}{{$x}}{{end}}",
			exp: &ListNode{Nodes: []Node{&RangeNode{BranchNode: BranchNode{
				Pipe: &PipeNode{Decl: []*VariableNode{{Ident: []string{"$x"}}}, Commands: []*CommandNode{{Args: []Node{&NumberNode{IsInt: true, IsUint: true, IsFloat: true, Text: "0"}}}}},
				List: &ListNode{Nodes: []Node{&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&VariableNode{Ident: []string{"$x"}}}}}}}}},
			}}}},
		},
		{
			name:   "bug2b",
			input:  "{{range $x = 0}}{{$x}}{{end}}",
			ok:     noError,
			result: "{{range $x = 0}}{{$x}}{{end}}",
			exp: &ListNode{Nodes: []Node{&RangeNode{BranchNode: BranchNode{
				Pipe: &PipeNode{IsAssign: true, Decl: []*VariableNode{{Ident: []string{"$x"}}}, Commands: []*CommandNode{{Args: []Node{&NumberNode{IsInt: true, IsUint: true, IsFloat: true, Text: "0"}}}}},
				List: &ListNode{Nodes: []Node{&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&VariableNode{Ident: []string{"$x"}}}}}}}}},
			}}}},
		},
		// dot following a literal value
		{
			name:  "dot after integer",
			input: "{{1.E}}",
			ok:    hasError,
		},
		{
			name:  "dot after float",
			input: "{{0.1.E}}",
			ok:    hasError,
		},
		{
			name:  "dot after boolean",
			input: "{{true.E}}",
			ok:    hasError,
		},
		{
			name:  "dot after char",
			input: "{{'a'.any}}",
			ok:    hasError,
		},
		{
			name:  "dot after string",
			input: `{{"hello".guys}}`,
			ok:    hasError,
		},
		{
			name:  "dot after dot",
			input: "{{..E}}",
			ok:    hasError,
		},
		{
			name:  "dot after nil",
			input: "{{nil.E}}",
			ok:    hasError,
		},
		// Wrong pipeline
		{
			name:  "wrong pipeline dot",
			input: "{{12|.}}",
			ok:    hasError,
		},
		{
			name:  "wrong pipeline number",
			input: "{{.|12|printf}}",
			ok:    hasError,
		},
		{
			name:  "wrong pipeline string",
			input: "{{.|printf|\"error\"}}",
			ok:    hasError,
		},
		{
			name:  "wrong pipeline char",
			input: "{{12|printf|'e'}}",
			ok:    hasError,
		},
		{
			name:  "wrong pipeline boolean",
			input: "{{.|true}}",
			ok:    hasError,
		},
		{
			name:  "wrong pipeline nil",
			input: "{{'c'|nil}}",
			ok:    hasError,
		},
		{
			name:  "empty pipeline",
			input: `{{printf "%d" ( ) }}`,
			ok:    hasError,
		},
		// Missing pipeline in block
		{
			name:  "block definition",
			input: `{{block "foo"}}hello{{end}}`,
			ok:    hasError,
		},

		// Expression nested depth tests.
		{
			name:   "paren nesting normal",
			input:  "{{ (( 1 )) }}",
			ok:     noError,
			result: "{{((1))}}",
			exp: &ListNode{Nodes: []Node{&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{
				&PipeNode{Commands: []*CommandNode{{Args: []Node{
					&PipeNode{Commands: []*CommandNode{{Args: []Node{&NumberNode{IsInt: true, IsUint: true, IsFloat: true, Int64: 1, Uint64: 1, Float64: 1, Text: "1"}}}}},
				}}}},
			}}}}}}},
		},
		{
			name:   "paren nesting at limit",
			input:  "{{ ((( 1 ))) }}",
			ok:     noError,
			result: "{{(((1)))}}",
			exp: &ListNode{Nodes: []Node{&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{
				&PipeNode{Commands: []*CommandNode{{Args: []Node{
					&PipeNode{Commands: []*CommandNode{{Args: []Node{
						&PipeNode{Commands: []*CommandNode{{Args: []Node{&NumberNode{IsInt: true, IsUint: true, IsFloat: true, Int64: 1, Uint64: 1, Float64: 1, Text: "1"}}}}},
					}}}},
				}}}},
			}}}}}}},
		},
		{
			name:   "paren nesting exceeds limit",
			input:  "{{ (((( 1 )))) }}",
			ok:     hasError,
			result: "template: test:1: max expression depth exceeded",
		},
		{
			name:   "paren nesting in pipeline",
			input:  "{{ ((( 1 ))) | printf }}",
			ok:     noError,
			result: "{{(((1))) | printf}}",
			exp: &ListNode{Nodes: []Node{&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{
				{Args: []Node{
					&PipeNode{Commands: []*CommandNode{{Args: []Node{
						&PipeNode{Commands: []*CommandNode{{Args: []Node{
							&PipeNode{Commands: []*CommandNode{{Args: []Node{&NumberNode{IsInt: true, IsUint: true, IsFloat: true, Int64: 1, Uint64: 1, Float64: 1, Text: "1"}}}}},
						}}}},
					}}}},
				}},
				{Args: []Node{&IdentifierNode{Ident: "printf"}}},
			}}}}},
		},
		{
			name:   "paren nesting in pipeline exceeds limit",
			input:  "{{ (((( 1 )))) | printf }}",
			ok:     hasError,
			result: "template: test:1: max expression depth exceeded",
		},
		{
			name:   "paren nesting with other constructs",
			input:  "{{ if ((( true ))) }}YES{{ end }}",
			ok:     noError,
			result: "{{if (((true)))}}\"YES\"{{end}}",
			exp: &ListNode{Nodes: []Node{&IfNode{BranchNode: BranchNode{
				Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{
					&PipeNode{Commands: []*CommandNode{{Args: []Node{
						&PipeNode{Commands: []*CommandNode{{Args: []Node{
							&PipeNode{Commands: []*CommandNode{{Args: []Node{&BoolNode{True: true}}}}},
						}}}},
					}}}},
				}}}},
				List: &ListNode{Nodes: []Node{&TextNode{Text: []byte("YES")}}},
			}}}},
		},
		{
			name:   "paren nesting with other constructs exceeds limit",
			input:  "{{ if (((( true )))) }}YES{{ end }}",
			ok:     hasError,
			result: "template: test:1: max expression depth exceeded",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			opt := lex.NewOptions()
			opt.ParseHTML = false
			opt.EmitComment = false
			tmpl, err := ParseTree("test.gohtml", test.name, test.input, nil, opt)
			if !test.ok {
				require.Error(t, err)
				if *debug {
					fmt.Printf("%s: %s\n\t%s\n", test.name, test.input, err)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.input, tmpl.Root.String(), "String()")
			requireASTEqual(t, tmpl.Root, test.exp)
		})
	}
}

func TestParseWithComments(t *testing.T) {
	tests := [...]parseTest{
		{
			name:   "comment",
			input:  "{{/*\n\n\n*/}}",
			ok:     noError,
			result: "{{/*\n\n\n*/}}",
		},
		{
			name:   "comment trim left",
			input:  "x \r\n\t{{- /* hi */}}",
			ok:     noError,
			result: "x \r\n\t{{- /* hi */}}",
		},
		{
			name:   "comment trim right",
			input:  "{{/* hi */ -}}\n\n\ty",
			ok:     noError,
			result: "{{/* hi */ -}}\n\n\ty",
		},
		{
			name:   "comment trim left and right",
			input:  "x \r\n\t{{- /* */ -}}\n\n\ty",
			ok:     noError,
			result: "x \r\n\t{{- /* */ -}}\n\n\ty",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// ParseComments
			tmpl, err := ParseTree("test.gohtml", test.name, test.input, nil, lex.NewOptions())
			require.NoError(t, err)
			if tmpl != nil && tmpl.Root != nil {
				assert.Equal(t, test.result, tmpl.Root.String())
			}
		})
	}
}

func TestKeywordsAndFuncs(t *testing.T) {
	// Check collisions between functions and new keywords like 'break'. When a
	// break function is provided, the parser should treat 'break' as a function,
	// not a keyword.

	inp := `{{range .X}}{{break 20}}{{end}}`
	{
		// 'break' is a defined function, don't treat it as a keyword: it should
		// accept an argument successfully.
		funcsWithKeywordFunc := map[string]any{
			"break": func(in any) any { return in },
		}
		tmpl, err := ParseTree("test.gohtml", "", inp, nil, lex.NewOptions(funcsWithKeywordFunc))
		require.NoError(t, err, "with break func")
		require.NotNil(t, tmpl, "with break func")
	}

	{
		// No function called 'break'; treat it as a keyword. Results in a parse
		// error.
		_, err := ParseTree("test.gohtml", "", inp, nil, lex.NewOptions())
		require.Error(t, err, "without break func: expected error")
	}
}

type isEmptyTest struct {
	name  string
	input string
	empty bool
}

func TestIsEmpty(t *testing.T) {
	assert.True(t, IsEmptyTree(nil), "nil tree should be empty")
	for _, test := range []isEmptyTest{
		{
			name:  "empty",
			input: ``, empty: true,
		},
		{
			name:  "nonempty",
			input: `hello`, empty: false,
		},
		{
			name:  "spaces only",
			input: " \t\n \t\n", empty: true,
		},
		{
			name:  "comment only",
			input: "{{/* comment */}}", empty: true,
		},
		{
			name:  "definition",
			input: `{{define "x"}}something{{end}}`, empty: true,
		},
		{
			name:  "empty definition",
			input: `{{define "x"}}{{end}}`, empty: true,
		},
		{
			name:  "definitions and space",
			input: "{{define `x`}}something{{end}}\n\n{{define `y`}}something{{end}}\n\n", empty: true,
		},
		{
			name:  "definitions and text",
			input: "{{define `x`}}something{{end}}\nx\n{{define `y`}}something{{end}}\ny\n", empty: false,
		},
		{
			name:  "definition and action",
			input: "{{define `x`}}something{{end}}{{if 3}}foo{{end}}", empty: false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tree, err := ParseTree("test.gohtml", "root", test.input, make(map[string]*Tree), lex.NewOptions())
			require.NoError(t, err)
			assert.Equal(t, test.empty, IsEmptyTree(tree.Root))
		})
	}
}

func TestIsEmptyTreeBreakContinue(t *testing.T) {
	tr := &Tree{Name: "test", text: "{{break}}"}
	breakNode := tr.newBreak(0, 9, 1)
	list := tr.newList(0)
	list.Nodes = append(list.Nodes, breakNode)
	assert.False(t, IsEmptyTree(list), "ListNode containing BreakNode should not be empty")
	assert.False(t, IsEmptyTree(breakNode), "BreakNode should not be empty")

	tr2 := &Tree{Name: "test", text: "{{continue}}"}
	continueNode := tr2.newContinue(0, 12, 1)
	list2 := tr2.newList(0)
	list2.Nodes = append(list2.Nodes, continueNode)
	assert.False(t, IsEmptyTree(list2), "ListNode containing ContinueNode should not be empty")
	assert.False(t, IsEmptyTree(continueNode), "ContinueNode should not be empty")
}

func TestErrors(t *testing.T) {
	for _, test := range []parseTest{
		// Check line numbers are accurate.
		{
			name:   "unclosed1",
			input:  "line1\n{{",
			ok:     hasError,
			result: `test.gohtml:2: unclosed action`,
		},
		{
			name:   "unclosed2",
			input:  "line1\n{{define `x`}}line2\n{{",
			ok:     hasError,
			result: `test.gohtml:3: unclosed action`,
		},
		{
			name:   "unclosed3",
			input:  "line1\n{{\"x\"\n\"y\"\n",
			ok:     hasError,
			result: `test.gohtml:4: unclosed action started at test.gohtml:2`,
		},
		{
			name:   "unclosed4",
			input:  "{{\n\n\n\n\n",
			ok:     hasError,
			result: `test.gohtml:6: unclosed action started at test.gohtml:1`,
		},
		// Specific errors.
		{
			name:   "comment1",
			input:  "{{/*}}",
			ok:     hasError,
			result: `test.gohtml:1: unclosed comment`,
		},
		{
			name:   "comment2",
			input:  "{{/*\nhello\n}}",
			ok:     hasError,
			result: `test.gohtml:1: unclosed comment`,
		},
		{
			name:   "lparen",
			input:  "{{.X (1 2 3}}",
			ok:     hasError,
			result: `unclosed left paren`,
		},
		{
			name:   "rparen",
			input:  "{{.X 1 2 3 ) }}",
			ok:     hasError,
			result: "unexpected right paren",
		},
		{
			name:   "rparen2",
			input:  "{{(.X 1 2 3",
			ok:     hasError,
			result: `unclosed action`,
		},
		{
			name:   "space",
			input:  "{{`x`3}}",
			ok:     hasError,
			result: `in operand`,
		},
		{
			name:   "idchar",
			input:  "{{a#}}",
			ok:     hasError,
			result: `'#'`,
		},
		{
			name:   "charconst",
			input:  "{{'a}}",
			ok:     hasError,
			result: `unterminated character constant`,
		},
		{
			name:   "stringconst",
			input:  `{{"a}}`,
			ok:     hasError,
			result: `unterminated quoted string`,
		},
		{
			name:   "rawstringconst",
			input:  "{{`a}}",
			ok:     hasError,
			result: `unterminated raw quoted string`,
		},
		{
			name:   "number",
			input:  "{{0xi}}",
			ok:     hasError,
			result: `number syntax`,
		},
		{
			name:   "multidefine",
			input:  "{{define `a`}}a{{end}}{{define `a`}}b{{end}}",
			ok:     hasError,
			result: `multiple definition of template`,
		},
		{
			name:   "eof",
			input:  "{{range .X}}",
			ok:     hasError,
			result: `unexpected EOF`,
		},
		{
			name:   "variable",
			input:  "{{$x := 23}}{{with $x.y := 3}}{{$x 23}}{{end}}",
			ok:     hasError,
			result: `unexpected ":="`,
		},
		{
			name:   "multidecl",
			input:  "{{$a,$b,$c := 23}}",
			ok:     hasError,
			result: `too many declarations`,
		},
		{
			name:   "undefvar",
			input:  "{{$a}}",
			ok:     hasError,
			result: `undefined variable`,
		},
		{
			name:   "wrongdot",
			input:  "{{true.any}}",
			ok:     hasError,
			result: `unexpected . after term`,
		},
		{
			name:   "wrongpipeline",
			input:  "{{12|false}}",
			ok:     hasError,
			result: `non executable command in pipeline`,
		},
		{
			name:   "emptypipeline",
			input:  `{{ ( ) }}`,
			ok:     hasError,
			result: `missing value for parenthesized pipeline`,
		},
		{
			name:   "multilinerawstring",
			input:  "{{ $v := `\n` }} {{",
			ok:     hasError,
			result: `test.gohtml:2: unclosed action`,
		},
		{
			name:   "rangeundefvar",
			input:  "{{range $k}}{{end}}",
			ok:     hasError,
			result: `undefined variable`,
		},
		{
			name:   "rangeundefvars",
			input:  "{{range $k, $v}}{{end}}",
			ok:     hasError,
			result: `undefined variable`,
		},
		{
			name:   "rangemissingvalue1",
			input:  "{{range $k,}}{{end}}",
			ok:     hasError,
			result: `missing value for range`,
		},
		{
			name:   "rangemissingvalue2",
			input:  "{{range $k, $v := }}{{end}}",
			ok:     hasError,
			result: `missing value for range`,
		},
		{
			name:   "rangenotvariable1",
			input:  "{{range $k, .}}{{end}}",
			ok:     hasError,
			result: `range can only initialize variables`,
		},
		{
			name:   "rangenotvariable2",
			input:  "{{range $k, 123 := .}}{{end}}",
			ok:     hasError,
			result: `range can only initialize variables`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			opt := lex.NewOptions()
			opt.ParseHTML = false
			_, err := ParseTree("test.gohtml", test.name, test.input, make(map[string]*Tree), opt)
			require.Error(t, err)
			assert.ErrorContains(t, err, test.result)
		})
	}
}

func TestBlock(t *testing.T) {
	const (
		input = `a{{block "inner" .}}bar{{.}}baz{{end}}b`
		inner = `bar{{.}}baz`
	)
	treeSet := make(map[string]*Tree)
	tmpl, err := ParseTree("test.gohtml", "outer", input, treeSet, lex.NewOptions())
	require.NoError(t, err)
	assert.Equal(t, input, tmpl.Root.String(), "outer template")
	inTmpl := treeSet["inner"]
	require.NotNil(t, inTmpl, "block did not define template")
	assert.Equal(t, inner, inTmpl.Root.String(), "inner template")
}

func TestLineNum(t *testing.T) {
	// const count = 100
	const count = 3
	text := strings.Repeat("{{printf 1234}}\n", count)
	opt := lex.NewOptions()
	opt.ParseHTML = false
	tree, err := ParseTree("test.gohtml", "bench", text, nil, opt)
	require.NoError(t, err)
	// Check the line numbers. Each line is an action containing a template, followed by text.
	// That's two nodes per line.
	nodes := tree.Root.Nodes
	for i := 0; i < len(nodes); i += 2 {
		line := 1 + i/2
		// Action first.
		action := nodes[i].(*ActionNode)
		assert.Equal(t, line, action.Line, "line %d: action", line)
		assert.Equal(t, line, action.Pipe.Line, "line %d: pipe", line)
	}
}

func BenchmarkParseLarge(b *testing.B) {
	text := strings.Repeat("{{1234}}\n", 10000)
	for i := 0; i < b.N; i++ {
		_, err := ParseTree("test.gohtml", "bench", text, nil, lex.NewOptions())
		if err != nil {
			b.Fatal(err)
		}
	}
}

var sinkv, sinkl string

func BenchmarkVariableString(b *testing.B) {
	v := &VariableNode{
		Ident: []string{"$", "A", "BB", "CCC", "THIS_IS_THE_VARIABLE_BEING_PROCESSED"},
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkv = v.String()
	}
	if sinkv == "" {
		b.Fatal("Benchmark was not run")
	}
}

func BenchmarkListString(b *testing.B) {
	text := `
{{(printf .Field1.Field2.Field3).Value}}
{{$x := (printf .Field1.Field2.Field3).Value}}
{{$y := (printf $x.Field1.Field2.Field3).Value}}
{{$z := $y.Field1.Field2.Field3}}
{{if contains $y $z}}
	{{printf "%q" $y}}
{{else}}
	{{printf "%q" $x}}
{{end}}
{{with $z.Field1 | contains "boring"}}
	{{printf "%q" . | printf "%s"}}
{{else}}
	{{printf "%d %d %d" 11 11 11}}
	{{printf "%d %d %s" 22 22 $x.Field1.Field2.Field3 | printf "%s"}}
	{{printf "%v" (contains $z.Field1.Field2 $y)}}
{{end}}
`
	tree, err := ParseTree("test.gohtml", "bench", text, make(map[string]*Tree), lex.NewOptions())
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkl = tree.Root.String()
	}
	if sinkl == "" {
		b.Fatal("Benchmark was not run")
	}
}

func TestTemplateFile(t *testing.T) {
	// Minimal smoke test: the assets_report.gohtml file parses without error.
	// Deeper round-trip and AST assertions live in TestReferenceTestdataFiles.
	buf, err := os.ReadFile(filepath.FromSlash("testdata/assets_report.gohtml"))
	require.NoError(t, err)
	_, err = ParseTree("test.gohtml", "testdata/assets_report.gohtml", string(buf), make(map[string]*Tree), lex.NewOptions())
	require.NoError(t, err)
}

// TestHTMLModeDoesNotAffectTemplateOnly verifies that template-only content
// (no HTML tags) parsed in HTML mode produces functionally equivalent output.
// In HTML mode, text nodes are represented differently (no quoting in String()),
// but the template actions should be identical.
func TestHTMLModeDoesNotAffectTemplateOnly(t *testing.T) {
	templateOnlyCases := []struct {
		name   string
		input  string
		result string // expected String() in HTML mode
		exp    Node   // expected AST root (nil to skip structural check)
	}{
		{
			name: "field", input: "{{.X}}", result: "{{.X}}",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"X"}}}}}}},
			}}}},
		},
		{
			name: "pipeline", input: "{{.X | .Y}}", result: "{{.X | .Y}}",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{
					{Args: []Node{&FieldNode{Ident: []string{"X"}}}},
					{Args: []Node{&FieldNode{Ident: []string{"Y"}}}},
				}}},
			}}}},
		},
		{
			name: "if", input: "{{if .X}}yes{{end}}", result: "{{if .X}}yes{{end}}",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&IfNode{BranchNode: BranchNode{
					Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"X"}}}}}},
					List: &ListNode{Nodes: []Node{&TextNode{Text: []byte("yes")}}},
				}},
			}}}},
		},
		{
			name: "if else", input: "{{if .X}}yes{{else}}no{{end}}", result: "{{if .X}}yes{{else}}no{{end}}",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&IfNode{BranchNode: BranchNode{
					Pipe:     &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"X"}}}}}},
					List:     &ListNode{Nodes: []Node{&TextNode{Text: []byte("yes")}}},
					ElseList: &ListNode{Nodes: []Node{&TextNode{Text: []byte("no")}}},
				}},
			}}}},
		},
		{
			name: "range", input: "{{range .X}}item{{end}}", result: "{{range .X}}item{{end}}",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&RangeNode{BranchNode: BranchNode{
					Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"X"}}}}}},
					List: &ListNode{Nodes: []Node{&TextNode{Text: []byte("item")}}},
				}},
			}}}},
		},
		{
			name: "with", input: "{{with .X}}val{{end}}", result: "{{with .X}}val{{end}}",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&WithNode{BranchNode: BranchNode{
					Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"X"}}}}}},
					List: &ListNode{Nodes: []Node{&TextNode{Text: []byte("val")}}},
				}},
			}}}},
		},
		{
			name: "template", input: `{{template "x"}}`, result: `{{template "x"}}`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&TemplateNode{Name: "x"},
			}}}},
		},
		{
			name: "template with arg", input: `{{template "x" .Y}}`, result: `{{template "x" .Y}}`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&TemplateNode{Name: "x", Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Y"}}}}}}},
			}}}},
		},
		{
			name: "nested", input: "{{range .X}}{{if .Y}}z{{end}}{{end}}", result: "{{range .X}}{{if .Y}}z{{end}}{{end}}",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&RangeNode{BranchNode: BranchNode{
					Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"X"}}}}}},
					List: &ListNode{Nodes: []Node{
						&IfNode{BranchNode: BranchNode{
							Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Y"}}}}}},
							List: &ListNode{Nodes: []Node{&TextNode{Text: []byte("z")}}},
						}},
					}},
				}},
			}}}},
		},
	}
	for _, test := range templateOnlyCases {
		t.Run(test.name, func(t *testing.T) {
			tmpl, err := ParseTree("test.gohtml", test.name, test.input, nil, lex.NewOptions())
			require.NoError(t, err)
			assert.Equal(t, test.result, tmpl.Root.String())
			requireASTEqual(t, tmpl.Root, test.exp)
		})
	}
}

// TestReferenceTemplateInputs verifies that template inputs drawn from the
// reference testdata directories (text/template and html/template examples,
// .tmpl files) parse without error in both standard and HTML modes.
//
// These inputs exercise patterns found in real-world template usage that go
// beyond the unit-level parseTests and htmlParseTests.
func TestReferenceTemplateInputs(t *testing.T) {
	// Inputs from testdata/reference/text_template/testdata/*.tmpl
	// and testdata/reference/html_template/testdata/*.tmpl.
	//
	// file1.tmpl: multiple defines
	// file2.tmpl: define with nested template call
	// tmpl1.tmpl / tmpl2.tmpl: cross-referencing templates
	tmplFileInputs := []struct {
		name  string
		input string
	}{
		{name: "file1.tmpl: multiple defines", input: "{{define \"x\"}}TEXT{{end}}\n{{define \"dotV\"}}{{.V}}{{end}}\n"},
		{name: "file2.tmpl: define with nested template", input: "{{define \"dot\"}}{{.}}{{end}}\n{{define \"nested\"}}{{template \"dot\" .}}{{end}}\n"},
		{name: "tmpl1.tmpl: text with define and template call", input: "template1\n{{define \"x\"}}x{{end}}\n{{template \"y\"}}\n"},
		{name: "tmpl2.tmpl: text with define and template call", input: "template2\n{{define \"y\"}}y{{end}}\n{{template \"x\"}}\n"},
	}

	// Inputs from reference example_test.go files.
	exampleInputs := []struct {
		name  string
		input string
	}{
		{name: "text/template letter example", input: "\nDear {{.Name}},\n{{if .Attended}}\nIt was a pleasure to see you at the wedding.\n{{- else}}\nIt is a shame you couldn't make it to the wedding.\n{{- end}}\n{{with .Gift -}}\nThank you for the lovely {{.}}.\n{{end}}\nBest wishes,\nJosie\n"},
		{name: "text/template block master", input: `Names:{{block "list" .}}{{"\n"}}{{range .}}{{println "-" .}}{{end}}{{end}}`},
		{name: "text/template block overlay", input: `{{define "list"}} {{join . ", "}}{{end}} `},
		{name: "html/template autoescaping define", input: `{{define "T"}}Hello, {{.}}!{{end}}`},
	}

	modes := []struct {
		name    string
		options lex.Options
	}{
		{"standard mode", standardModeOptions()},
		{"HTML mode", lex.NewOptions()}, // NewOptions() defaults ParseHTML to true.
	}

	allInputs := append(append([]struct {
		name  string
		input string
	}{}, tmplFileInputs...), exampleInputs...)

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			for _, tc := range allInputs {
				t.Run(tc.name, func(t *testing.T) {
					_, err := ParseTree("test.gohtml", "test", tc.input, nil, mode.options)
					require.NoError(t, err)
				})
			}
		})
	}
}

// standardModeOptions returns lex options with ParseHTML disabled, used by the
// reference tests that want to exercise the plain text/template parser path.
func standardModeOptions() lex.Options {
	opt := lex.NewOptions()
	opt.ParseHTML = false
	return opt
}

// TestReferenceHTMLDocumentParse verifies that full HTML documents from the
// html/template reference example parse correctly in HTML mode.
func TestReferenceHTMLDocumentParse(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "html/template Example() full document", input: "\n<!DOCTYPE html>\n<html>\n\t<head>\n\t\t<meta charset=\"UTF-8\">\n\t\t<title>{{.Title}}</title>\n\t</head>\n\t<body>\n\t\t{{range .Items}}<div>{{ . }}</div>{{else}}<div><strong>no rows</strong></div>{{end}}\n\t</body>\n</html>"},
		{name: "doctype with html head body", input: "<!DOCTYPE html><html><head><title>T</title></head><body><p>Hello</p></body></html>"},
		{name: "nested elements with multiple attrs", input: `<html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width"></head><body><main><h1>Title</h1></main></body></html>`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpl, err := ParseTree("test.gohtml", "test", tc.input, nil, lex.NewOptions())
			require.NoError(t, err)
			require.NotNil(t, tmpl.Root)
			require.NotEmpty(t, tmpl.Root.Nodes, "empty root")
			_, ok := tmpl.Root.Nodes[0].(*HTMLDocumentFragmentNode)
			assert.True(t, ok, "expected HTMLDocumentFragmentNode, got %T", tmpl.Root.Nodes[0])
		})
	}
}

// TestReferenceRawTextElements verifies that raw text elements (script, style,
// textarea, title) with embedded template actions parse correctly.
func TestReferenceRawTextElements(t *testing.T) {
	tests := []htmlParseTest{
		{name: "script with template action", input: "<script>var x = {{.Value}};</script>", result: "<script>var x = {{.Value}};</script>"},
		{name: "style with template action", input: "<style>.cls { color: {{.Color}}; }</style>", result: "<style>.cls { color: {{.Color}}; }</style>"},
		{name: "textarea with template action", input: "<textarea>{{.Content}}</textarea>", result: "<textarea>{{.Content}}</textarea>"},
		{name: "title with template action", input: "<title>{{.Title}}</title>", result: "<title>{{.Title}}</title>"},
		{name: "script with if", input: "<script>{{if .Debug}}console.log('debug');{{end}}</script>", result: "<script>{{if .Debug}}console.log('debug');{{end}}</script>"},
		{name: "script with range", input: "<script>{{range .Scripts}}loadScript('{{.}}');{{end}}</script>", result: "<script>{{range .Scripts}}loadScript('{{.}}');{{end}}</script>"},
		{name: "script with less-than and action", input: "<script>if (x < 3) { {{.Code}} }</script>", result: "<script>if (x < 3) { {{.Code}} }</script>"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpl, err := ParseTree("test.gohtml", tc.name, tc.input, nil, lex.NewOptions())
			require.NoError(t, err)
			assert.Equal(t, tc.result, tmpl.Root.String())
		})
	}
}

// TestReferenceHTMLCommentWithAction verifies HTML comments containing
// template actions parse correctly.
func TestReferenceHTMLCommentWithAction(t *testing.T) {
	tests := []htmlParseTest{
		{name: "comment with action", input: "<!-- {{.Note}} -->", result: "<!-- {{.Note}} -->"},
		{name: "comment with if", input: "<!-- {{if .A}}a <b>{{.B}}</b>{{else}}c{{end}} -->", result: "<!-- {{if .A}}a <b>{{.B}}</b>{{else}}c{{end}} -->"},
		{name: "comment with range and with", input: "<!-- {{range .A}}<i>{{with .B}}{{.}}{{end}}</i>{{end}} -->", result: "<!-- {{range .A}}<i>{{with .B}}{{.}}{{end}}</i>{{end}} -->"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpl, err := ParseTree("test.gohtml", tc.name, tc.input, nil, lex.NewOptions())
			require.NoError(t, err)
			assert.Equal(t, tc.result, tmpl.Root.String())
		})
	}
}

// TestReferenceCustomDelimiters verifies parsing with non-default delimiters,
// drawn from the html/template ExampleTemplate_Delims test.
func TestReferenceCustomDelimiters(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		leftDelim  string
		rightDelim string
	}{
		{name: "angle bracket delimiters", input: "<<.Greeting>> {{.Name}}", leftDelim: "<<", rightDelim: ">>"},
		{name: "dollar-at delimiters", input: "$$.X@@", leftDelim: "$$", rightDelim: "@@"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opt := lex.NewOptions()
			opt.LeftDelim, opt.RightDelim = tc.leftDelim, tc.rightDelim
			_, err := ParseTree("test.gohtml", "test", tc.input, nil, opt)
			require.NoError(t, err)
		})
	}
}

// TestReferenceBlockDefine verifies the block/overlay pattern used in both
// text/template and html/template examples.
func TestReferenceBlockDefine(t *testing.T) {
	// ParseTree master template with block.
	master := `Names:{{block "list" .}}{{"\n"}}{{range .}}{{println "-" .}}{{end}}{{end}}`
	treeSet := make(map[string]*Tree)
	tmpl, err := ParseTree("test.gohtml", "master", master, treeSet, lex.NewOptions())
	require.NoError(t, err, "master parse")
	require.NotNil(t, tmpl.Root, "master root")
	// The block should have created a "list" template in the tree set.
	assert.Contains(t, treeSet, "list", "block did not create 'list' template")
	// The master template should also be in the tree set.
	assert.Contains(t, treeSet, "master", "master template not in tree set")

	// ParseTree overlay that redefines "list" into a fresh tree set.
	// In real usage, Clone() is used to copy the tree set before redefining.
	// Parsing it into the same tree set is a second definition of "list",
	// which text/template reports.
	overlay := `{{define "list"}} {{join . ", "}}{{end}} `
	overlaySet := make(map[string]*Tree)
	_, err = ParseTree("test.gohtml", "overlay", overlay, overlaySet, lex.NewOptions())
	require.NoError(t, err, "overlay parse")
	assert.Contains(t, overlaySet, "list", "overlay did not create 'list' template")
	_, err = ParseTree("test.gohtml", "overlay", overlay, treeSet, lex.NewOptions())
	assert.EqualError(t, err, `template: test.gohtml:1: template: multiple definition of template "list"`, "overlay parsed into the master's tree set")
}

// TestReferenceMultiDefineFile verifies that inputs with multiple {{define}}
// blocks (like the .tmpl files in testdata) correctly populate the tree set.
func TestReferenceMultiDefineFile(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedTrees []string
	}{
		{name: "file1.tmpl defines x and dotV", input: "{{define \"x\"}}TEXT{{end}}\n{{define \"dotV\"}}{{.V}}{{end}}\n", expectedTrees: []string{"x", "dotV"}},
		{name: "file2.tmpl defines dot and nested", input: "{{define \"dot\"}}{{.}}{{end}}\n{{define \"nested\"}}{{template \"dot\" .}}{{end}}\n", expectedTrees: []string{"dot", "nested"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			treeSet := make(map[string]*Tree)
			opt := lex.NewOptions()
			opt.ParseHTML = false
			_, err := ParseTree("test.gohtml", "root", tc.input, treeSet, opt)
			require.NoError(t, err)
			for _, name := range tc.expectedTrees {
				assert.Contains(t, treeSet, name, "expected template %q in tree set", name)
			}
		})
	}
}

// TestReferenceMultiDefineFileHTML verifies multi-define inputs in HTML mode.
// In HTML mode, {{define}} creates inline DefineNodes; Extract() is needed
// to pull them into a tree set.
func TestReferenceMultiDefineFileHTML(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedTrees []string
	}{
		{name: "multiple defines with HTML", input: `{{define "header"}}<h1>{{.Title}}</h1>{{end}}{{define "footer"}}<footer>{{.Copy}}</footer>{{end}}<main>{{template "header" .}}content{{template "footer" .}}</main>`, expectedTrees: []string{"header", "footer"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr, err := ParseTree("test.gohtml", "main", tc.input, nil, lex.NewOptions())
			require.NoError(t, err)
			// In HTML mode, defines are inline. Use Extract to get them.
			treeSet := make(map[string]*Tree)
			for name, dt := range Extract(tr) {
				treeSet[name] = dt
			}
			for _, name := range tc.expectedTrees {
				assert.Contains(t, treeSet, name, "expected template %q in extracted tree set", name)
			}
		})
	}
}

// TestExtractDiagnostics verifies that Extract copies parse-time
// diagnostics into each extracted tree, filtered to the extracted
// body's position range, so checking a define/block tree in
// isolation (e.g. via ExecuteTemplate) still surfaces the findings
// the parser recorded for its body.
func TestExtractDiagnostics(t *testing.T) {
	tr, err := ParseTree("test.gohtml", "main", `{{define "row"}}<tr><td>x</b></td></tr>{{end}}{{define "clean"}}<p>ok</p>{{end}}`, nil, lex.NewOptions())
	require.NoError(t, err)
	require.NotEmpty(t, tr.Diagnostics, "expected parent tree to record the orphan </b> diagnostic")

	trees := make(map[string]*Tree)
	for name, dt := range Extract(tr) {
		trees[name] = dt
	}
	require.Contains(t, trees, "row")
	require.Contains(t, trees, "clean")

	rowDiags := trees["row"].Diagnostics
	require.Len(t, rowDiags, 1, "expected exactly one diagnostic copied onto the %q tree, got %v", "row", rowDiags)
	assert.Equal(t, DiagOrphanEndTag, rowDiags[0].Code)

	assert.Empty(t, trees["clean"].Diagnostics, "expected no diagnostics copied onto the issue-free %q tree", "clean")
}

// TestReferenceErrorsInHTMLMode verifies that template errors from the
// reference error test cases are still caught in HTML mode. This ensures
// the error-return refactoring didn't break error propagation.
func TestReferenceErrorsInHTMLMode(t *testing.T) {
	// Select error cases that are meaningful in HTML mode (template-level
	// errors, not lexer-level). These should fail identically in both modes.
	errorCases := []struct {
		name   string
		input  string
		errStr string
	}{
		{name: "undefined variable", input: "{{$x}}", errStr: "undefined variable"},
		{name: "empty pipeline", input: `{{ ( ) }}`, errStr: "missing value"},
		{name: "unclosed action", input: "hello{{range", errStr: "unclosed action"},
		{name: "missing end", input: "hello{{range .x}}", errStr: "unexpected EOF"},
		{name: "multiple else", input: "{{if .X}}1{{else}}2{{else}}3{{end}}", errStr: "expected end"},
		{name: "break outside range html", input: "<p>{{break}}</p>", errStr: "outside"},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseTree("test.gohtml", tc.name, tc.input, nil, lex.NewOptions())
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.errStr)
		})
	}
}

// TestReferenceTestdataFiles parses real-world .gohtml files in testdata/ and
// verifies round-trip fidelity plus any file-specific structural expectations.
// Each file exercises a different parser path:
//   - assets_report.gohtml: production template with define blocks
//   - raw_text_elements.gohtml: script, style, textarea, title with actions
//     and content that would break without raw-text handling (bare <, CSS >)
//   - script_heavy.gohtml: multiple script blocks, heavy < and > operators,
//     conditional wrapping of raw text elements
func TestReferenceTestdataFiles(t *testing.T) {
	cases := []struct {
		file       string // relative to testdata/
		wantDefine bool   // root.Children must contain at least one DefineNode
	}{
		{file: "assets_report.gohtml", wantDefine: true},
		{file: "raw_text_elements.gohtml", wantDefine: true},
		{file: "script_heavy.gohtml", wantDefine: false},
	}

	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.FromSlash("testdata/" + tc.file))
			require.NoError(t, err, "read testdata file")
			input := string(data)

			tmpl, err := ParseTree(tc.file, tc.file, input, nil, lex.NewOptions())
			require.NoError(t, err)
			assert.Equal(t, input, tmpl.Root.String(), "round-trip String()")

			if !tc.wantDefine {
				return
			}
			require.NotNil(t, tmpl.Root)
			require.NotEmpty(t, tmpl.Root.Nodes, "empty root")
			frag, ok := tmpl.Root.Nodes[0].(*HTMLDocumentFragmentNode)
			require.True(t, ok, "expected HTMLDocumentFragmentNode, got %T", tmpl.Root.Nodes[0])
			assert.True(t, containsDefine(frag.Children),
				"expected a DefineNode in the document fragment")
		})
	}
}

// containsDefine reports whether any top-level child is a *DefineNode.
func containsDefine(children []Node) bool {
	for _, child := range children {
		if _, ok := child.(*DefineNode); ok {
			return true
		}
	}
	return false
}

func TestParseErrorType(t *testing.T) {
	const src = "hello {{end}}"
	_, err := ParseTree("page.gohtml", "page", src, nil, lex.NewOptions())
	require.Error(t, err)

	var pe *ParseError
	require.ErrorAs(t, err, &pe)

	// Error() keeps the historical "template: <name>:<line>:" prefix.
	assert.True(t, strings.HasPrefix(pe.Error(), "template: page.gohtml:"), pe.Error())
	// Unwrap gives the bare, prefix-free message, and Error() still ends with it.
	require.NotNil(t, pe.Unwrap())
	assert.NotContains(t, pe.Unwrap().Error(), "template:")
	assert.Contains(t, pe.Error(), pe.Unwrap().Error())
	// Position pins an ordered byte span inside the offending {{end}} region
	// (the exact token the parser stops on is internal, so assert
	// containment rather than a fixed offset).
	start, end := pe.Position(), pe.EndPosition()
	assert.Equal(t, "page.gohtml", start.Filename)
	assert.GreaterOrEqual(t, start.Offset, strings.Index(src, "{{"))
	assert.LessOrEqual(t, end.Offset, strings.Index(src, "}}")+len("}}"))
	assert.Greater(t, end.Offset, start.Offset)
	assert.Positive(t, start.Line)
}

func TestBlockNodeTrimMarkers(t *testing.T) {
	type trims struct{ startLeft, startRight, endLeft, endRight bool }
	for _, tt := range []struct {
		name string
		src  string
		html bool
		want trims
	}{
		{name: "none", src: `{{block "b" .}}x{{end}}`},
		{name: "all", src: `{{- block "b" . -}}x{{- end -}}`, want: trims{true, true, true, true}},
		{name: "inner only", src: `{{block "b" . -}}x{{- end}}`, want: trims{startRight: true, endLeft: true}},
		{name: "html all", src: `<p>{{- block "b" . -}}x{{- end -}}</p>`, html: true, want: trims{true, true, true, true}},
		{name: "html none", src: `<p>{{block "b" .}}x{{end}}</p>`, html: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			opt := lex.NewOptions()
			opt.ParseHTML = tt.html
			tree, err := ParseTree("t", "t", tt.src, nil, opt)
			require.NoError(t, err)
			var block *BlockNode
			Walk(tree, func(_ *Tree, n Node) {
				if b, ok := n.(*BlockNode); ok && block == nil {
					block = b
				}
			})
			require.NotNil(t, block, "no BlockNode in %q", tt.src)
			got := trims{block.StartTrimLeft, block.StartTrimRight, block.EndTrimLeft, block.EndTrimRight}
			if got != tt.want {
				t.Errorf("BlockNode(%q) trims = %v, want %v", tt.src, got, tt.want)
			}
		})
	}
}
