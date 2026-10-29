package template

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/template/lex"
)

type htmlParseTest struct {
	name   string
	input  string
	result string // expected String() output of the tree root
	exp    Node   // expected AST root (nil to skip structural check)
}

func TestHTMLParseDefineExtract(t *testing.T) {
	input := `{{define "header"}}<h1>{{.Title}}</h1>{{end}}<p>body</p>`
	tmpl, err := ParseTree("test.gohtml", "main", input, nil, lex.NewOptions())
	require.NoError(t, err)

	// The DefineNode should be inline in the AST.
	result := tmpl.Root.String()
	expected := `{{define "header"}}<h1>{{.Title}}</h1>{{end}}<p>body</p>`
	assert.Equal(t, expected, result, "inline AST")
	inlineExp := &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
		&DefineNode{Name: "header", Body: &ListNode{Nodes: []Node{
			&HTMLElementNode{TagName: "h1", Closed: true, Children: []Node{
				&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Title"}}}}}}},
			}},
		}}},
		&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{
			&TextNode{Text: []byte("body")},
		}},
	}}}}
	requireASTEqual(t, tmpl.Root, inlineExp)

	// Extract should pull the define into a separate tree.
	treeSet := make(map[string]*Tree)
	for name, tr := range Extract(tmpl) {
		treeSet[name] = tr
	}
	headerTree, ok := treeSet["header"]
	require.True(t, ok, "Extract: missing header tree")
	headerResult := headerTree.Root.String()
	headerExpected := "<h1>{{.Title}}</h1>"
	assert.Equal(t, headerExpected, headerResult, "Extract header tree")
	headerExp := &ListNode{Nodes: []Node{
		&HTMLElementNode{TagName: "h1", Closed: true, Children: []Node{
			&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Title"}}}}}}},
		}},
	}}
	requireASTEqual(t, headerTree.Root, headerExp)
}

func TestHTMLParse(t *testing.T) {
	for _, test := range []htmlParseTest{
		{
			name:   "empty",
			input:  "",
			result: "",
			exp:    &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{}}},
		},
		{
			name:   "text only",
			input:  "hello world",
			result: "hello world",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&TextNode{Text: []byte("hello world")},
			}}}},
		},
		{
			name:   "simple element",
			input:  "<p>hello</p>",
			result: "<p>hello</p>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{
					&TextNode{Text: []byte("hello")},
				}},
			}}}},
		},
		{
			name:   "nested elements",
			input:  "<div><p>hello</p></div>",
			result: "<div><p>hello</p></div>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "div", Closed: true, Children: []Node{
					&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{
						&TextNode{Text: []byte("hello")},
					}},
				}},
			}}}},
		},
		{
			name:   "void element",
			input:  "<br>",
			result: "<br>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "br", Void: true},
			}}}},
		},
		{
			name:   "self-closing",
			input:  "<br/>",
			result: "<br/>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "br", Void: true},
			}}}},
		},
		{
			name: "element with attr", input: `<div class="foo">hello</div>`,
			result: `<div class="foo">hello</div>`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "div", Closed: true, Attr: []Node{
					&HTMLAttributeNode{
						HasValue: true,
						Key:      []Node{&TextNode{Text: []byte("class")}},
						Value:    []Node{&TextNode{Text: []byte(`"foo"`)}},
					},
				}, Children: []Node{
					&TextNode{Text: []byte("hello")},
				}},
			}}}},
		},
		{
			name:   "boolean attr",
			input:  "<input disabled>",
			result: "<input disabled>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "input", Void: true, Attr: []Node{
					&HTMLAttributeNode{Key: []Node{&TextNode{Text: []byte("disabled")}}},
				}},
			}}}},
		},
		{
			name: "multiple attrs", input: `<a href="/" class="link">click</a>`,
			result: `<a href="/" class="link">click</a>`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "a", Closed: true, Attr: []Node{
					&HTMLAttributeNode{
						HasValue: true,
						Key:      []Node{&TextNode{Text: []byte("href")}},
						Value:    []Node{&TextNode{Text: []byte(`"/"`)}},
					},
					&HTMLAttributeNode{
						HasValue: true,
						Key:      []Node{&TextNode{Text: []byte("class")}},
						Value:    []Node{&TextNode{Text: []byte(`"link"`)}},
					},
				}, Children: []Node{
					&TextNode{Text: []byte("click")},
				}},
			}}}},
		},
		{
			name:   "template in text",
			input:  "<p>{{.Name}}</p>",
			result: "<p>{{.Name}}</p>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{
					&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Name"}}}}}}},
				}},
			}}}},
		},
		{
			name: "template in attr", input: `<div class="{{.Class}}">x</div>`,
			result: `<div class="{{.Class}}">x</div>`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "div", Closed: true, Attr: []Node{
					&HTMLAttributeNode{
						HasValue: true,
						Key:      []Node{&TextNode{Text: []byte("class")}},
						Value: []Node{
							&TextNode{Text: []byte(`"`)},
							&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Class"}}}}}}},
							&TextNode{Text: []byte(`"`)},
						},
					},
				}, Children: []Node{
					&TextNode{Text: []byte("x")},
				}},
			}}}},
		},
		{
			name: "mixed attr value", input: `<div class="base {{.X}}">x</div>`,
			result: `<div class="base {{.X}}">x</div>`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "div", Closed: true, Attr: []Node{
					&HTMLAttributeNode{
						HasValue: true,
						Key:      []Node{&TextNode{Text: []byte("class")}},
						Value: []Node{
							&TextNode{Text: []byte(`"base `)},
							&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"X"}}}}}}},
							&TextNode{Text: []byte(`"`)},
						},
					},
				}, Children: []Node{
					&TextNode{Text: []byte("x")},
				}},
			}}}},
		},
		{
			name:   "html comment",
			input:  "<!-- hello -->",
			result: "<!-- hello -->",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLCommentNode{Children: []Node{&TextNode{Text: []byte(" hello ")}}},
			}}}},
		},
		{
			name:   "doctype",
			input:  "<!DOCTYPE html>",
			result: "<!DOCTYPE html>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLDoctypeNode{Text: "<!DOCTYPE html>"},
			}}}},
		},
		{
			name:   "range in html",
			input:  "{{range .Items}}<li>{{.}}</li>{{end}}",
			result: `{{range .Items}}<li>{{.}}</li>{{end}}`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&RangeNode{BranchNode: BranchNode{
					Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Items"}}}}}},
					List: &ListNode{Nodes: []Node{
						&HTMLElementNode{TagName: "li", Closed: true, Children: []Node{
							&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&DotNode{}}}}}},
						}},
					}},
				}},
			}}}},
		},
		{
			name: "define inline", input: `{{define "header"}}<h1>{{.Title}}</h1>{{end}}<p>body</p>`,
			result: `{{define "header"}}<h1>{{.Title}}</h1>{{end}}<p>body</p>`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&DefineNode{Name: "header", Body: &ListNode{Nodes: []Node{
					&HTMLElementNode{TagName: "h1", Closed: true, Children: []Node{
						&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Title"}}}}}}},
					}},
				}}},
				&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{
					&TextNode{Text: []byte("body")},
				}},
			}}}},
		},
		// Single-quoted attribute.
		{
			name: "single-quoted attr", input: "<div class='foo'>hello</div>",
			result: "<div class='foo'>hello</div>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "div", Closed: true, Attr: []Node{
					&HTMLAttributeNode{
						HasValue: true,
						Key:      []Node{&TextNode{Text: []byte("class")}},
						Value:    []Node{&TextNode{Text: []byte("'foo'")}},
					},
				}, Children: []Node{
					&TextNode{Text: []byte("hello")},
				}},
			}}}},
		},
		// Unquoted attribute.
		{
			name: "unquoted attr", input: "<input type=text>",
			result: "<input type=text>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "input", Void: true, Attr: []Node{
					&HTMLAttributeNode{
						HasValue: true,
						Key:      []Node{&TextNode{Text: []byte("type")}},
						Value:    []Node{&TextNode{Text: []byte("text")}},
					},
				}},
			}}}},
		},
		// Boolean attr then valued attr.
		{
			name: "boolean attr then valued attr", input: `<input required type="text">`,
			result: `<input required type="text">`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "input", Void: true, Attr: []Node{
					&HTMLAttributeNode{Key: []Node{&TextNode{Text: []byte("required")}}},
					&HTMLAttributeNode{
						HasValue: true,
						Key:      []Node{&TextNode{Text: []byte("type")}},
						Value:    []Node{&TextNode{Text: []byte(`"text"`)}},
					},
				}},
			}}}},
		},
		// Multiple void elements.
		{
			name:   "multiple void elements",
			input:  `<br><hr><img src="x.png">`,
			result: `<br><hr><img src="x.png">`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "br", Void: true},
				&HTMLElementNode{TagName: "hr", Void: true},
				&HTMLElementNode{TagName: "img", Void: true, Attr: []Node{
					&HTMLAttributeNode{
						HasValue: true,
						Key:      []Node{&TextNode{Text: []byte("src")}},
						Value:    []Node{&TextNode{Text: []byte(`"x.png"`)}},
					},
				}},
			}}}},
		},
		// Empty comment.
		{
			name:   "empty comment",
			input:  "<!---->",
			result: "<!---->",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLCommentNode{},
			}}}},
		},
		// Abrupt comment close <!-->.
		{
			name:   "abrupt comment close <!-->",
			input:  "<!-->text",
			result: "<!-->text",
		},
		// Abrupt comment close <!--->.
		{
			name:   "abrupt comment close <!--->",
			input:  "<!--->text",
			result: "<!--->text",
		},
		// Self-closing non-void element.
		{
			name:   "self-closing non-void",
			input:  "<div/>text",
			result: "<div/>text",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "div", Void: true},
				&TextNode{Text: []byte("text")},
			}}}},
		},
		// Deeply nested HTML.
		{
			name:   "deeply nested HTML",
			input:  "<div><section><article><p><span>x</span></p></article></section></div>",
			result: "<div><section><article><p><span>x</span></p></article></section></div>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "div", Closed: true, Children: []Node{
					&HTMLElementNode{TagName: "section", Closed: true, Children: []Node{
						&HTMLElementNode{TagName: "article", Closed: true, Children: []Node{
							&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{
								&HTMLElementNode{TagName: "span", Closed: true, Children: []Node{
									&TextNode{Text: []byte("x")},
								}},
							}},
						}},
					}},
				}},
			}}}},
		},
		// Orphan end tag at top level — exercises htmlParseEndTag.
		{
			name:   "orphan end tag at top level",
			input:  "</div>",
			result: "</div>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&TextNode{Text: []byte("</div>"), Verbatim: true},
			}}}},
		},
		// HTML entities preserved.
		{
			name:   "HTML entities preserved",
			input:  `<p title="&amp;">&lt;hello&gt;</p>`,
			result: `<p title="&amp;">&lt;hello&gt;</p>`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "p", Closed: true, Attr: []Node{
					&HTMLAttributeNode{
						HasValue: true,
						Key:      []Node{&TextNode{Text: []byte("title")}},
						Value:    []Node{&TextNode{Text: []byte(`"&amp;"`)}},
					},
				}, Children: []Node{
					&TextNode{Text: []byte("&lt;hello&gt;")},
				}},
			}}}},
		},
		// data-* attr with template.
		{
			name: "data-* attr with template", input: `<tr data-asset-index="{{.Index}}">x</tr>`,
			result: `<tr data-asset-index="{{.Index}}">x</tr>`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "tr", Closed: true, Attr: []Node{
					&HTMLAttributeNode{
						HasValue: true,
						Key:      []Node{&TextNode{Text: []byte("data-asset-index")}},
						Value: []Node{
							&TextNode{Text: []byte(`"`)},
							&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Index"}}}}}}},
							&TextNode{Text: []byte(`"`)},
						},
					},
				}, Children: []Node{
					&TextNode{Text: []byte("x")},
				}},
			}}}},
		},
		// --- Raw text elements (script, style, textarea, title) ---
		// WPT-inspired: basic script content.
		{
			name:   "raw text: script with text",
			input:  "<script>'Hello'</script>",
			result: "<script>'Hello'</script>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "script", Closed: true, Children: []Node{
					&TextNode{Text: []byte("'Hello'")},
				}},
			}}}},
		},
		// WPT-inspired: empty script.
		{
			name:   "raw text: empty script",
			input:  "<script></script>",
			result: "<script></script>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "script", Closed: true},
			}}}},
		},
		// WPT-inspired: bare < in script (the key bug this feature fixes).
		{
			name:   "raw text: bare < in script",
			input:  "<script>if (a < b) {}</script>",
			result: "<script>if (a < b) {}</script>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "script", Closed: true, Children: []Node{
					&TextNode{Text: []byte("if (a < b) {}")},
				}},
			}}}},
		},
		// WPT-inspired: <! in script.
		{
			name:   "raw text: <! in script",
			input:  "<script>'<!'</script>",
			result: "<script>'<!'</script>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "script", Closed: true, Children: []Node{
					&TextNode{Text: []byte("'<!'")},
				}},
			}}}},
		},
		// WPT-inspired: <!-- in script (comment-like, but raw text).
		{
			name:   "raw text: <!-- in script",
			input:  "<script>'<!--'</script>",
			result: "<script>'<!--'</script>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "script", Closed: true, Children: []Node{
					&TextNode{Text: []byte("'<!--'")},
				}},
			}}}},
		},
		// WPT-inspired: </scriptx> is NOT a matching end tag.
		{
			name:   "raw text: non-matching end tag </scriptx>",
			input:  "<script></scriptx></script>",
			result: "<script></scriptx></script>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "script", Closed: true, Children: []Node{
					&TextNode{Text: []byte("</scriptx>")},
				}},
			}}}},
		},
		// WPT-inspired: template action in script.
		{
			name:   "raw text: template action in script",
			input:  "<script>var x = {{.X}};</script>",
			result: "<script>var x = {{.X}};</script>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "script", Closed: true, Children: []Node{
					&TextNode{Text: []byte("var x = ")},
					&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"X"}}}}}}},
					&TextNode{Text: []byte(";")},
				}},
			}}}},
		},
		// Style element with bare <.
		{
			name:   "raw text: style with bare <",
			input:  "<style>a < b { color: red }</style>",
			result: "<style>a < b { color: red }</style>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "style", Closed: true, Children: []Node{
					&TextNode{Text: []byte("a < b { color: red }")},
				}},
			}}}},
		},
		// Textarea with HTML tags as raw content.
		{
			name:   "raw text: textarea with tags",
			input:  "<textarea><div>not a real div</div></textarea>",
			result: "<textarea><div>not a real div</div></textarea>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "textarea", Closed: true, Children: []Node{
					&TextNode{Text: []byte("<div>not a real div</div>")},
				}},
			}}}},
		},
		// Title with tags as raw content.
		{
			name:   "raw text: title with tags",
			input:  "<title><b>K&amp;R C</b></title>",
			result: "<title><b>K&amp;R C</b></title>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "title", Closed: true, Children: []Node{
					&TextNode{Text: []byte("<b>K&amp;R C</b>")},
				}},
			}}}},
		},
		// Script with attributes — attributes are still parsed normally.
		{
			name:   "raw text: script with attributes",
			input:  `<script type="text/javascript">x</script>`,
			result: `<script type="text/javascript">x</script>`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "script", Closed: true, Attr: []Node{
					&HTMLAttributeNode{
						HasValue: true,
						Key:      []Node{&TextNode{Text: []byte("type")}},
						Value:    []Node{&TextNode{Text: []byte(`"text/javascript"`)}},
					},
				}, Children: []Node{
					&TextNode{Text: []byte("x")},
				}},
			}}}},
		},
		// WPT-inspired: case-insensitive end tag.
		{
			name:   "raw text: case-insensitive end tag",
			input:  "<SCRIPT>x</SCRipt>",
			result: "<SCRIPT>x</SCRipt>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "SCRIPT", Closed: true, Children: []Node{
					&TextNode{Text: []byte("x")},
				}},
			}}}},
		},
		// WPT-inspired: multiple < and </ in script content.
		{
			name:   "raw text: multiple bare < in script",
			input:  "<script>a < b && c < d</script>",
			result: "<script>a < b && c < d</script>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "script", Closed: true, Children: []Node{
					&TextNode{Text: []byte("a < b && c < d")},
				}},
			}}}},
		},
		// Raw text element followed by normal content.
		{
			name:   "raw text: script then normal",
			input:  "<script>x</script><p>y</p>",
			result: "<script>x</script><p>y</p>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "script", Closed: true, Children: []Node{
					&TextNode{Text: []byte("x")},
				}},
				&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{
					&TextNode{Text: []byte("y")},
				}},
			}}}},
		},
		// Control structure inside quoted attribute value.
		{
			name:   "if in attr value",
			input:  `<input value="{{if .X}}yes{{end}}">`,
			result: `<input value="{{if .X}}yes{{end}}">`,
		},
		{
			name:   "if-else in attr value",
			input:  `<input value="{{if .X}}yes{{else}}no{{end}}">`,
			result: `<input value="{{if .X}}yes{{else}}no{{end}}">`,
		},
		{
			name:   "with-else in attr value",
			input:  `<input value="{{with .X}}{{.}}{{else}}default{{end}}">`,
			result: `<input value="{{with .X}}{{.}}{{else}}default{{end}}">`,
		},
		{
			name:   "control structure in attr value followed by conditional attr",
			input:  `<input value="{{with .Name}}{{.}}{{else}}My Portfolio{{end}}" {{if .Err}}aria-invalid="true"{{end}}>`,
			result: `<input value="{{with .Name}}{{.}}{{else}}My Portfolio{{end}}" {{if .Err}}aria-invalid="true"{{end}}>`,
		},
		// Single-quoted control structures in attr values.
		{
			name:   "sq if in attr value",
			input:  `<input value='{{if .X}}yes{{end}}'>`,
			result: `<input value='{{if .X}}yes{{end}}'>`,
		},
		{
			name:   "sq if-else in attr value",
			input:  `<input value='{{if .X}}yes{{else}}no{{end}}'>`,
			result: `<input value='{{if .X}}yes{{else}}no{{end}}'>`,
		},
		{
			name:   "sq with-else in attr value",
			input:  `<input value='{{with .X}}{{.}}{{else}}default{{end}}'>`,
			result: `<input value='{{with .X}}{{.}}{{else}}default{{end}}'>`,
		},
		// Range in attr value.
		{
			name:   "dq range in attr value",
			input:  `<input value="{{range .Items}}{{.}} {{end}}">`,
			result: `<input value="{{range .Items}}{{.}} {{end}}">`,
		},
		{
			name:   "sq range in attr value",
			input:  `<input value='{{range .Items}}{{.}} {{end}}'>`,
			result: `<input value='{{range .Items}}{{.}} {{end}}'>`,
		},
		// Mixed text + control in attr value.
		{
			name:   "dq prefix + if",
			input:  `<input value="prefix-{{if .X}}yes{{end}}">`,
			result: `<input value="prefix-{{if .X}}yes{{end}}">`,
		},
		{
			name:   "dq if + suffix",
			input:  `<input value="{{if .X}}yes{{end}}-suffix">`,
			result: `<input value="{{if .X}}yes{{end}}-suffix">`,
		},
		{
			name:   "dq prefix + if-else + suffix",
			input:  `<input value="prefix-{{if .X}}yes{{else}}no{{end}}-suffix">`,
			result: `<input value="prefix-{{if .X}}yes{{else}}no{{end}}-suffix">`,
		},
		{
			name:   "sq prefix + with-else",
			input:  `<input value='hi {{with .X}}{{.}}{{else}}fallback{{end}}'>`,
			result: `<input value='hi {{with .X}}{{.}}{{else}}fallback{{end}}'>`,
		},
		// Nested control structures in attr value.
		{
			name:   "dq nested if-if",
			input:  `<input value="{{if .X}}{{if .Y}}deep{{end}}{{end}}">`,
			result: `<input value="{{if .X}}{{if .Y}}deep{{end}}{{end}}">`,
		},
		{
			name:   "dq nested if-with",
			input:  `<input value="{{if .X}}{{with .Y}}{{.}}{{end}}{{end}}">`,
			result: `<input value="{{if .X}}{{with .Y}}{{.}}{{end}}{{end}}">`,
		},
		{
			name:   "dq nested with-range",
			input:  `<input value="{{with .Items}}{{range .}}{{.}} {{end}}{{end}}">`,
			result: `<input value="{{with .Items}}{{range .}}{{.}} {{end}}{{end}}">`,
		},
		// No space after attr value before conditional attr.
		{
			name:   "no space after value before cond attr",
			input:  `<input value="{{.X}}"{{if .Y}} class="c"{{end}}>`,
			result: `<input value="{{.X}}"{{if .Y}} class="c"{{end}}>`,
		},
		// with wrapping attr name=value.
		{
			name:   "with wrapping attr",
			input:  `<input {{with .X}}value="{{.}}"{{end}}>`,
			result: `<input {{with .X}}value="{{.}}"{{end}}>`,
		},
		// Complicated embedded control flow in attr values.
		{
			name:   "triple nested if-with-range in val",
			input:  `<div class="{{if .X}}{{with .Items}}{{range .}}{{.}}{{end}}{{end}}{{end}}">x</div>`,
			result: `<div class="{{if .X}}{{with .Items}}{{range .}}{{.}}{{end}}{{end}}{{end}}">x</div>`,
		},
		{
			name:   "chained else-if in val",
			input:  `<div class="{{if eq .I 1}}one{{else if eq .I 7}}seven{{else}}other{{end}}">x</div>`,
			result: `<div class="{{if eq .I 1}}one{{else if eq .I 7}}seven{{else}}other{{end}}">x</div>`,
		},
		{
			name:   "range index var in val",
			input:  `<div data-x="{{range $i, $v := .Items}}{{$i}}:{{$v}} {{end}}">x</div>`,
			result: `<div data-x="{{range $i, $v := .Items}}{{$i}}:{{$v}} {{end}}">x</div>`,
		},
		{
			name:   "var decl in if in val",
			input:  `<div class="{{if .X}}{{$x := "on"}}{{$x}}{{end}}">x</div>`,
			result: `<div class="{{if .X}}{{$x := "on"}}{{$x}}{{end}}">x</div>`,
		},
		{
			name:   "call in if in val",
			input:  `<div class="{{if .X}}{{call .Fn "y"}}{{end}}">x</div>`,
			result: `<div class="{{if .X}}{{call .Fn "y"}}{{end}}">x</div>`,
		},
		{
			name:   "multiple controls concat in val",
			input:  `<div class="{{if .X}}a{{end}}-{{with .Name}}{{.}}{{end}}-{{range .Items}}{{.}}{{end}}">x</div>`,
			result: `<div class="{{if .X}}a{{end}}-{{with .Name}}{{.}}{{end}}-{{range .Items}}{{.}}{{end}}">x</div>`,
		},
		{
			name:   "if eq in val",
			input:  `<input value="{{if eq .Name "World"}}match{{else}}no{{end}}">`,
			result: `<input value="{{if eq .Name "World"}}match{{else}}no{{end}}">`,
		},
		{
			name:   "if and in val",
			input:  `<input value="{{if and .X .Name}}both{{else}}nope{{end}}">`,
			result: `<input value="{{if and .X .Name}}both{{else}}nope{{end}}">`,
		},
		{
			name:   "if len in val",
			input:  `<input value="{{if gt (len .Items) 0}}has{{else}}empty{{end}}">`,
			result: `<input value="{{if gt (len .Items) 0}}has{{else}}empty{{end}}">`,
		},
		{
			name:   "4-deep nesting in val",
			input:  `<div class="{{if .X}}{{if .Name}}{{if gt .I 0}}{{with .U}}{{.V}}{{end}}{{end}}{{end}}{{end}}">x</div>`,
			result: `<div class="{{if .X}}{{if .Name}}{{if gt .I 0}}{{with .U}}{{.V}}{{end}}{{end}}{{end}}{{end}}">x</div>`,
		},
		{
			name:   "with dollar in val",
			input:  `<div class="{{with .U}}{{.V}}-{{$.Name}}{{end}}">x</div>`,
			result: `<div class="{{with .U}}{{.V}}-{{$.Name}}{{end}}">x</div>`,
		},
		{
			name:   "range dollar in val",
			input:  `<div data-x="{{range .Items}}{{.}}-{{$.Name}} {{end}}">x</div>`,
			result: `<div data-x="{{range .Items}}{{.}}-{{$.Name}} {{end}}">x</div>`,
		},
		{
			name:   "printf in range in val",
			input:  `<div data-x="{{range .Nums}}{{printf "%03d" .}} {{end}}">x</div>`,
			result: `<div data-x="{{range .Nums}}{{printf "%03d" .}} {{end}}">x</div>`,
		},
		{
			name:   "kitchen sink val",
			input:  `<div class="{{with .Name}}n:{{.}}{{else}}anon{{end}} {{range .Items}}{{if ne . "b"}}{{.}}{{end}}{{end}}">x</div>`,
			result: `<div class="{{with .Name}}n:{{.}}{{else}}anon{{end}} {{range .Items}}{{if ne . "b"}}{{.}}{{end}}{{end}}">x</div>`,
		},
		// Control flow wrapping entire key=value in attr position.
		{
			name:   "if wrapping key=val",
			input:  `<input {{if .X}}name="x"{{end}}>`,
			result: `<input {{if .X}}name="x"{{end}}>`,
		},
		{
			name:   "if-else wrapping attrs",
			input:  `<input {{if .X}}class="on"{{else}}class="off"{{end}}>`,
			result: `<input {{if .X}}class="on"{{else}}class="off"{{end}}>`,
		},
		{
			name:   "chained else-if wrapping attrs",
			input:  `<div {{if eq .I 1}}class="one"{{else if eq .I 7}}class="seven"{{else}}class="other"{{end}}>x</div>`,
			result: `<div {{if eq .I 1}}class="one"{{else if eq .I 7}}class="seven"{{else}}class="other"{{end}}>x</div>`,
		},
		{
			name:   "nested if in attr-position with val",
			input:  `<div {{if .X}}class="{{if eq .I 7}}lucky{{else}}unlucky{{end}}"{{end}}>x</div>`,
			result: `<div {{if .X}}class="{{if eq .I 7}}lucky{{else}}unlucky{{end}}"{{end}}>x</div>`,
		},
		{
			name:   "if wrapping bool attr",
			input:  `<input type="checkbox" {{if .X}}checked{{end}}>`,
			result: `<input type="checkbox" {{if .X}}checked{{end}}>`,
		},
		{
			name:   "with wrapping bool attr",
			input:  `<input type="checkbox" {{with .X}}checked{{end}}>`,
			result: `<input type="checkbox" {{with .X}}checked{{end}}>`,
		},
		{
			name:   "multiple ifs in attr position",
			input:  `<input {{if .X}}disabled{{end}} {{if .Name}}title="{{.Name}}"{{end}}>`,
			result: `<input {{if .X}}disabled{{end}} {{if .Name}}title="{{.Name}}"{{end}}>`,
		},
		{
			name:   "if wrapping href attr",
			input:  `<a {{if .X}}href="{{.Name}}"{{end}}>link</a>`,
			result: `<a {{if .X}}href="{{.Name}}"{{end}}>link</a>`,
		},
		{
			name:   "range producing attrs",
			input:  `<ul>{{range $k, $v := .M}}<li data-key="{{$k}}">{{$v}}</li>{{end}}</ul>`,
			result: `<ul>{{range $k, $v := .M}}<li data-key="{{$k}}">{{$v}}</li>{{end}}</ul>`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tmpl, err := ParseTree("test.gohtml", test.name, test.input, nil, lex.NewOptions())
			require.NoError(t, err)
			assert.Equal(t, test.result, tmpl.Root.String())
			requireASTEqual(t, tmpl.Root, test.exp)
		})
	}
}

func TestHTMLParseErrors(t *testing.T) {
	for _, test := range []struct {
		name   string
		input  string
		errStr string
	}{
		{
			name:   "top-level end",
			input:  `<p>hello</p>{{end}}`,
			errStr: "unexpected",
		},
		{
			name:   "top-level else",
			input:  `<p>hello</p>{{else}}`,
			errStr: "unexpected",
		},
		{
			name:   "bare end",
			input:  `{{end}}`,
			errStr: "unexpected",
		},
		{
			name:   "bare else",
			input:  `{{else}}`,
			errStr: "unexpected",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseTree("test.gohtml", test.name, test.input, nil, lex.NewOptions())
			require.Error(t, err)
			assert.ErrorContains(t, err, test.errStr)
		})
	}
}

func TestHTMLParseDepthLimit(t *testing.T) {
	t.Run("NewOptions default MaxHTMLDepth", func(t *testing.T) {
		opts := lex.NewOptions()
		assert.Equal(t, 1024, opts.MaxHTMLDepth)
	})

	t.Run("exceeds default depth", func(t *testing.T) {
		input := strings.Repeat("<div>", 2000)
		_, err := ParseTree("test.gohtml", "deep", input, nil, lex.NewOptions())
		require.Error(t, err)
		assert.ErrorContains(t, err, "nesting depth exceeded")
	})

	t.Run("at default depth limit", func(t *testing.T) {
		// 1024 nested elements should succeed at the default limit.
		input := strings.Repeat("<div>", 1024) + strings.Repeat("</div>", 1024)
		_, err := ParseTree("test.gohtml", "at-limit", input, nil, lex.NewOptions())
		require.NoError(t, err)
	})

	t.Run("custom MaxHTMLDepth respected", func(t *testing.T) {
		opts := lex.NewOptions()
		opts.MaxHTMLDepth = 5
		input := strings.Repeat("<div>", 10) + strings.Repeat("</div>", 10)
		_, err := ParseTree("test.gohtml", "custom", input, nil, opts)
		require.Error(t, err)
		assert.ErrorContains(t, err, "nesting depth exceeded")
	})

	t.Run("within custom limit", func(t *testing.T) {
		opts := lex.NewOptions()
		opts.MaxHTMLDepth = 5
		input := strings.Repeat("<div>", 5) + strings.Repeat("</div>", 5)
		_, err := ParseTree("test.gohtml", "within-custom", input, nil, opts)
		require.NoError(t, err)
	})
}

func TestOrphanEndTagDiagnostics(t *testing.T) {
	for _, test := range []struct {
		name, input string
		wantMsgs    []string
	}{
		{"top level", `orphan</b>end`, []string{`unmatched </b> is ignored by browsers`}},
		{"inside element", `<div>x</b></div>`, []string{`unmatched </b> is ignored by browsers`}},
		{"matching tag is fine", `<b>x</b>`, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			tree, err := ParseTree("test.gohtml", test.name, test.input, nil, lex.NewOptions())
			require.NoError(t, err)
			var got []string
			for _, d := range tree.Diagnostics {
				if d.Code == DiagOrphanEndTag {
					got = append(got, d.Message)
				}
			}
			assert.Equal(t, test.wantMsgs, got)
		})
	}
}

func TestImplicitCloseDiagnostics(t *testing.T) {
	for _, test := range []struct {
		name, input string
		wantCodes   []DiagnosticCode
		wantMsg     string // contains-match on first diagnostic, "" to skip
	}{
		{"adoption agency b/i", `<b><i>x</b>y`, []DiagnosticCode{DiagMisnestedTag}, `</b> also closes <i>; browsers reopen a copy of <i> after it`},
		{"li implied close is fine", `<ul><li>a<li>b</ul>`, nil, ""},
		{"p implied close is fine", `<div><p>a</div>`, nil, ""},
		{"span left to unclosed check", `<div><span></div>`, nil, ""},
		{"properly nested", `<b><i>x</i></b>`, nil, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			tree, err := ParseTree("test.gohtml", test.name, test.input, nil, lex.NewOptions())
			require.NoError(t, err)
			var codes []DiagnosticCode
			for _, d := range tree.Diagnostics {
				codes = append(codes, d.Code)
			}
			assert.Equal(t, test.wantCodes, codes)
			if test.wantMsg != "" {
				require.NotEmpty(t, tree.Diagnostics)
				assert.Contains(t, tree.Diagnostics[0].Message, test.wantMsg)
			}
		})
	}
}
