package template

import (
	"testing"

	"github.com/typelate/muxt/internal/template/lex"
)

func TestHTMLTemplate(t *testing.T) {
	// htmlTemplateTests are comprehensive integration tests mixing HTML and
	// template syntax. Each test verifies that ParseTree round-trips the input
	// through the AST's String() method.
	for _, test := range []htmlParseTest{
		// Basic control structures wrapping HTML.
		{
			name:   "if wrapping element",
			input:  "{{if .Show}}<p>visible</p>{{end}}",
			result: "{{if .Show}}<p>visible</p>{{end}}",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&IfNode{BranchNode: BranchNode{
					Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Show"}}}}}},
					List: &ListNode{Nodes: []Node{
						&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{
							&TextNode{Text: []byte("visible")},
						}},
					}},
				}},
			}}}},
		},
		{
			name:   "if else wrapping element",
			input:  "{{if .Show}}<p>yes</p>{{else}}<p>no</p>{{end}}",
			result: "{{if .Show}}<p>yes</p>{{else}}<p>no</p>{{end}}",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&IfNode{BranchNode: BranchNode{
					Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Show"}}}}}},
					List: &ListNode{Nodes: []Node{
						&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{&TextNode{Text: []byte("yes")}}},
					}},
					ElseList: &ListNode{Nodes: []Node{
						&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{&TextNode{Text: []byte("no")}}},
					}},
				}},
			}}}},
		},
		{
			name:   "with wrapping element",
			input:  "{{with .Data}}<span>{{.Name}}</span>{{end}}",
			result: "{{with .Data}}<span>{{.Name}}</span>{{end}}",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&WithNode{BranchNode: BranchNode{
					Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Data"}}}}}},
					List: &ListNode{Nodes: []Node{
						&HTMLElementNode{TagName: "span", Closed: true, Children: []Node{
							&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Name"}}}}}}},
						}},
					}},
				}},
			}}}},
		},
		{
			name:   "range over list items",
			input:  "{{range .Items}}<li>{{.}}</li>{{end}}",
			result: "{{range .Items}}<li>{{.}}</li>{{end}}",
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
			name:   "range with else",
			input:  "{{range .Items}}<li>{{.}}</li>{{else}}<li>none</li>{{end}}",
			result: "{{range .Items}}<li>{{.}}</li>{{else}}<li>none</li>{{end}}",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&RangeNode{BranchNode: BranchNode{
					Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Items"}}}}}},
					List: &ListNode{Nodes: []Node{
						&HTMLElementNode{TagName: "li", Closed: true, Children: []Node{
							&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&DotNode{}}}}}},
						}},
					}},
					ElseList: &ListNode{Nodes: []Node{
						&HTMLElementNode{TagName: "li", Closed: true, Children: []Node{&TextNode{Text: []byte("none")}}},
					}},
				}},
			}}}},
		},

		// Nested control structures.
		{
			name:   "nested if in range",
			input:  "{{range .Items}}<li>{{if .Active}}<b>{{.Name}}</b>{{end}}</li>{{end}}",
			result: "{{range .Items}}<li>{{if .Active}}<b>{{.Name}}</b>{{end}}</li>{{end}}",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&RangeNode{BranchNode: BranchNode{
					Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Items"}}}}}},
					List: &ListNode{Nodes: []Node{
						&HTMLElementNode{TagName: "li", Closed: true, Children: []Node{
							&IfNode{BranchNode: BranchNode{
								Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Active"}}}}}},
								List: &ListNode{Nodes: []Node{
									&HTMLElementNode{TagName: "b", Closed: true, Children: []Node{
										&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Name"}}}}}}},
									}},
								}},
							}},
						}},
					}},
				}},
			}}}},
		},
		{
			name:   "range in div",
			input:  "<div>{{range .Items}}<p>{{.}}</p>{{end}}</div>",
			result: "<div>{{range .Items}}<p>{{.}}</p>{{end}}</div>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "div", Closed: true, Children: []Node{
					&RangeNode{BranchNode: BranchNode{
						Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Items"}}}}}},
						List: &ListNode{Nodes: []Node{
							&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{
								&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&DotNode{}}}}}},
							}},
						}},
					}},
				}},
			}}}},
		},

		// Template actions in various positions.
		{
			name:   "action in element text",
			input:  "<p>Hello, {{.Name}}!</p>",
			result: "<p>Hello, {{.Name}}!</p>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{
					&TextNode{Text: []byte("Hello, ")},
					&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Name"}}}}}}},
					&TextNode{Text: []byte("!")},
				}},
			}}}},
		},
		{
			name:   "action as entire text",
			input:  "<p>{{.Content}}</p>",
			result: "<p>{{.Content}}</p>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{
					&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Content"}}}}}}},
				}},
			}}}},
		},
		{
			name:   "multiple actions in text",
			input:  "<p>{{.First}} {{.Last}}</p>",
			result: "<p>{{.First}} {{.Last}}</p>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{
					&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"First"}}}}}}},
					&TextNode{Text: []byte(" ")},
					&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Last"}}}}}}},
				}},
			}}}},
		},
		{
			name: "action in attr value", input: `<a href="{{.URL}}">link</a>`,
			result: `<a href="{{.URL}}">link</a>`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "a", Closed: true, Attr: []Node{
					&HTMLAttributeNode{
						HasValue: true,
						Key:      []Node{&TextNode{Text: []byte("href")}},
						Value: []Node{
							&TextNode{Text: []byte(`"`)},
							&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"URL"}}}}}}},
							&TextNode{Text: []byte(`"`)},
						},
					},
				}, Children: []Node{
					&TextNode{Text: []byte("link")},
				}},
			}}}},
		},
		{
			name: "mixed static and dynamic attr", input: `<div class="base {{.Class}}">x</div>`,
			result: `<div class="base {{.Class}}">x</div>`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "div", Closed: true, Attr: []Node{
					&HTMLAttributeNode{
						HasValue: true,
						Key:      []Node{&TextNode{Text: []byte("class")}},
						Value: []Node{
							&TextNode{Text: []byte(`"base `)},
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
			name: "multiple dynamic attrs", input: `<input type="{{.Type}}" value="{{.Val}}" />`,
			result: `<input type="{{.Type}}" value="{{.Val}}" />`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "input", Void: true, Attr: []Node{
					&HTMLAttributeNode{
						HasValue: true,
						Key:      []Node{&TextNode{Text: []byte("type")}},
						Value: []Node{
							&TextNode{Text: []byte(`"`)},
							&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Type"}}}}}}},
							&TextNode{Text: []byte(`"`)},
						},
					},
					&HTMLAttributeNode{
						HasValue: true,
						Key:      []Node{&TextNode{Text: []byte("value")}},
						Value: []Node{
							&TextNode{Text: []byte(`"`)},
							&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Val"}}}}}}},
							&TextNode{Text: []byte(`"`)},
						},
					},
				}},
			}}}},
		},

		// Conditional attributes.
		{
			name: "if in attr position", input: `<div {{if .Show}}class="visible"{{end}}>x</div>`,
			result: `<div {{if .Show}}class="visible"{{end}}>x</div>`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "div", Closed: true, Attr: []Node{
					&IfNode{BranchNode: BranchNode{
						Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Show"}}}}}},
						List: &ListNode{Nodes: []Node{
							&HTMLAttributeNode{
								HasValue: true,
								Key:      []Node{&TextNode{Text: []byte("class")}},
								Value:    []Node{&TextNode{Text: []byte(`"visible"`)}},
							},
						}},
					}},
				}, Children: []Node{
					&TextNode{Text: []byte("x")},
				}},
			}}}},
		},
		{
			name: "if else in attr position", input: `<div {{if .Show}}class="visible"{{else}}class="hidden"{{end}}>x</div>`,
			result: `<div {{if .Show}}class="visible"{{else}}class="hidden"{{end}}>x</div>`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "div", Closed: true, Attr: []Node{
					&IfNode{BranchNode: BranchNode{
						Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Show"}}}}}},
						List: &ListNode{Nodes: []Node{
							&HTMLAttributeNode{
								HasValue: true,
								Key:      []Node{&TextNode{Text: []byte("class")}},
								Value:    []Node{&TextNode{Text: []byte(`"visible"`)}},
							},
						}},
						ElseList: &ListNode{Nodes: []Node{
							&HTMLAttributeNode{
								HasValue: true,
								Key:      []Node{&TextNode{Text: []byte("class")}},
								Value:    []Node{&TextNode{Text: []byte(`"hidden"`)}},
							},
						}},
					}},
				}, Children: []Node{
					&TextNode{Text: []byte("x")},
				}},
			}}}},
		},
		{
			name: "range in attr position", input: `<div {{range .Classes}}class="{{.}}"{{end}}>x</div>`,
			result: `<div {{range .Classes}}class="{{.}}"{{end}}>x</div>`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "div", Closed: true, Attr: []Node{
					&RangeNode{BranchNode: BranchNode{
						Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Classes"}}}}}},
						List: &ListNode{Nodes: []Node{
							&HTMLAttributeNode{
								HasValue: true,
								Key:      []Node{&TextNode{Text: []byte("class")}},
								Value: []Node{
									&TextNode{Text: []byte(`"`)},
									&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&DotNode{}}}}}},
									&TextNode{Text: []byte(`"`)},
								},
							},
						}},
					}},
				}, Children: []Node{
					&TextNode{Text: []byte("x")},
				}},
			}}}},
		},

		// Template invocation.
		{
			name: "template call", input: `<div>{{template "header"}}</div>`,
			result: `<div>{{template "header"}}</div>`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "div", Closed: true, Children: []Node{
					&TemplateNode{Name: "header"},
				}},
			}}}},
		},
		{
			name: "template call with dot", input: `<div>{{template "header" .}}</div>`,
			result: `<div>{{template "header" .}}</div>`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "div", Closed: true, Children: []Node{
					&TemplateNode{Name: "header", Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&DotNode{}}}}}},
				}},
			}}}},
		},

		// Define inline.
		{
			name: "define inline", input: `{{define "item"}}<li>{{.}}</li>{{end}}`,
			result: `{{define "item"}}<li>{{.}}</li>{{end}}`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&DefineNode{Name: "item", Body: &ListNode{Nodes: []Node{
					&HTMLElementNode{TagName: "li", Closed: true, Children: []Node{
						&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&DotNode{}}}}}},
					}},
				}}},
			}}}},
		},
		{
			name: "define and use", input: `{{define "greeting"}}<b>Hello</b>{{end}}<p>{{template "greeting"}}</p>`,
			result: `{{define "greeting"}}<b>Hello</b>{{end}}<p>{{template "greeting"}}</p>`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&DefineNode{Name: "greeting", Body: &ListNode{Nodes: []Node{
					&HTMLElementNode{TagName: "b", Closed: true, Children: []Node{
						&TextNode{Text: []byte("Hello")},
					}},
				}}},
				&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{
					&TemplateNode{Name: "greeting"},
				}},
			}}}},
		},

		// Pipeline in HTML.
		{
			name:   "pipeline in text",
			input:  "<p>{{.X | printf}}</p>",
			result: "<p>{{.X | printf}}</p>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{
					&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{
						{Args: []Node{&FieldNode{Ident: []string{"X"}}}},
						{Args: []Node{&IdentifierNode{Ident: "printf"}}},
					}}},
				}},
			}}}},
		},

		// Void elements with dynamic attrs.
		{
			name: "void with action", input: `<img src="{{.URL}}" />`,
			result: `<img src="{{.URL}}" />`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "img", Void: true, Attr: []Node{
					&HTMLAttributeNode{
						HasValue: true,
						Key:      []Node{&TextNode{Text: []byte("src")}},
						Value: []Node{
							&TextNode{Text: []byte(`"`)},
							&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"URL"}}}}}}},
							&TextNode{Text: []byte(`"`)},
						},
					},
				}},
			}}}},
		},
		{
			name:   "void boolean attr",
			input:  "<input disabled />",
			result: "<input disabled />",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "input", Void: true, Attr: []Node{
					&HTMLAttributeNode{Key: []Node{&TextNode{Text: []byte("disabled")}}},
				}},
			}}}},
		},

		// Comments alongside HTML.
		{
			name:   "html comment before element",
			input:  "<!-- nav --><nav>x</nav>",
			result: "<!-- nav --><nav>x</nav>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLCommentNode{Children: []Node{&TextNode{Text: []byte(" nav ")}}},
				&HTMLElementNode{TagName: "nav", Closed: true, Children: []Node{
					&TextNode{Text: []byte("x")},
				}},
			}}}},
		},
		{
			name:   "template comment in html",
			input:  "<p>{{/* hidden */}}visible</p>",
			result: "<p>{{/* hidden */}}visible</p>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{
					&CommentNode{Text: "/* hidden */"},
					&TextNode{Text: []byte("visible")},
				}},
			}}}},
		},

		// DOCTYPE with HTML.
		{
			name:   "doctype and html",
			input:  "<!DOCTYPE html><html><body>{{.Content}}</body></html>",
			result: "<!DOCTYPE html><html><body>{{.Content}}</body></html>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLDoctypeNode{Text: "<!DOCTYPE html>"},
				&HTMLElementNode{TagName: "html", Closed: true, Children: []Node{
					&HTMLElementNode{TagName: "body", Closed: true, Children: []Node{
						&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Content"}}}}}}},
					}},
				}},
			}}}},
		},

		// Multiple top-level elements.
		{
			name:   "sibling elements",
			input:  "<h1>Title</h1><p>Body</p>",
			result: "<h1>Title</h1><p>Body</p>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "h1", Closed: true, Children: []Node{&TextNode{Text: []byte("Title")}}},
				&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{&TextNode{Text: []byte("Body")}}},
			}}}},
		},

		// Empty elements.
		{
			name:   "empty element",
			input:  "<div></div>",
			result: "<div></div>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "div", Closed: true},
			}}}},
		},
		{
			name: "empty with attr", input: `<div id="x"></div>`,
			result: `<div id="x"></div>`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "div", Closed: true, Attr: []Node{
					&HTMLAttributeNode{
						HasValue: true,
						Key:      []Node{&TextNode{Text: []byte("id")}},
						Value:    []Node{&TextNode{Text: []byte(`"x"`)}},
					},
				}},
			}}}},
		},

		// Variable declaration in range.
		{
			name:   "range with var",
			input:  "{{range $i, $v := .Items}}<li>{{$i}}: {{$v}}</li>{{end}}",
			result: "{{range $i, $v := .Items}}<li>{{$i}}: {{$v}}</li>{{end}}",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&RangeNode{BranchNode: BranchNode{
					Pipe: &PipeNode{
						Decl:     []*VariableNode{{Ident: []string{"$i"}}, {Ident: []string{"$v"}}},
						Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Items"}}}}},
					},
					List: &ListNode{Nodes: []Node{
						&HTMLElementNode{TagName: "li", Closed: true, Children: []Node{
							&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&VariableNode{Ident: []string{"$i"}}}}}}},
							&TextNode{Text: []byte(": ")},
							&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&VariableNode{Ident: []string{"$v"}}}}}}},
						}},
					}},
				}},
			}}}},
		},

		// With variable.
		{
			name:   "with var",
			input:  "{{with $x := .Name}}<span>{{$x}}</span>{{end}}",
			result: "{{with $x := .Name}}<span>{{$x}}</span>{{end}}",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&WithNode{BranchNode: BranchNode{
					Pipe: &PipeNode{
						Decl:     []*VariableNode{{Ident: []string{"$x"}}},
						Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Name"}}}}},
					},
					List: &ListNode{Nodes: []Node{
						&HTMLElementNode{TagName: "span", Closed: true, Children: []Node{
							&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&VariableNode{Ident: []string{"$x"}}}}}}},
						}},
					}},
				}},
			}}}},
		},
		// HTMX-style attribute with template value.
		{
			name: "hx-get attr with template value", input: `<div hx-get="/view/{{.ID}}/report">x</div>`,
			result: `<div hx-get="/view/{{.ID}}/report">x</div>`,
		},
		// Multiple actions in single attr value.
		{
			name: "multiple actions in single attr", input: `<div style="color: {{.Color}}; font-size: {{.Size}}px">x</div>`,
			result: `<div style="color: {{.Color}}; font-size: {{.Size}}px">x</div>`,
		},
		// Conditional style attribute using eq.
		{
			name: "conditional style attribute", input: `<td {{if eq .I .J}}style="font-weight: bold;"{{end}}>x</td>`,
			result: `<td {{if eq .I .J}}style="font-weight: bold;"{{end}}>x</td>`,
		},
		// Complex pipeline in content.
		{
			name: "complex pipeline in content", input: `<td>{{index .Matrix .I .J | printf "%.2f"}}</td>`,
			result: `<td>{{index .Matrix .I .J | printf "%.2f"}}</td>`,
		},
		// Deep field chain.
		{
			name:   "deep field chain",
			input:  "<p>{{.Result.Report.Assets}}</p>",
			result: "<p>{{.Result.Report.Assets}}</p>",
		},
		// Function call with parens: gt (len .Assets) 0
		{
			name:   "function gt len",
			input:  `{{if gt (len .Assets) 0}}<p>yes</p>{{end}}`,
			result: `{{if gt (len .Assets) 0}}<p>yes</p>{{end}}`,
		},
		// eq with string literal.
		{
			name:   "function eq string comparison",
			input:  `{{if eq .Type "private"}}<span>private</span>{{end}}`,
			result: `{{if eq .Type "private"}}<span>private</span>{{end}}`,
		},
		// Template call with field arg.
		{
			name: "template call with field arg", input: `<td>{{template "item" .Spec}}</td>`,
			result: `<td>{{template "item" .Spec}}</td>`,
		},
		// Void element inside td.
		{
			name:   "br inside td",
			input:  "<td>text<br>more</td>",
			result: "<td>text<br>more</td>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "td", Closed: true, Children: []Node{
					&TextNode{Text: []byte("text")},
					&HTMLElementNode{TagName: "br", Void: true},
					&TextNode{Text: []byte("more")},
				}},
			}}}},
		},
		// Select with conditional selected attribute.
		{
			name:   "select with conditional selected",
			input:  `<select><option {{if eq . "a"}}selected{{end}}>{{.}}</option></select>`,
			result: `<select><option {{if eq . "a"}}selected{{end}}>{{.}}</option></select>`,
		},
		// Action in script raw text element.
		{
			name:   "json in script tag",
			input:  "<script>var data = {{.Datasets}};</script>",
			result: "<script>var data = {{.Datasets}};</script>",
		},
		// Whitespace trimming with HTML.
		{
			name:   "whitespace trimming with HTML",
			input:  "{{- range .Items}}<li>{{.}}</li>{{end -}}",
			result: "{{- range .Items}}<li>{{.}}</li>{{end -}}",
		},
		// Block with HTML content.
		{
			name:   "block with HTML content",
			input:  `{{block "sidebar" .}}<nav>default</nav>{{end}}`,
			result: `{{block "sidebar" .}}<nav>default</nav>{{end}}`,
		},
		// Else-if chain in HTML.
		{
			name:   "else if chain in HTML",
			input:  "{{if .A}}<p>A</p>{{else if .B}}<p>B</p>{{else}}<p>C</p>{{end}}",
			result: "{{if .A}}<p>A</p>{{else if .B}}<p>B</p>{{else}}<p>C</p>{{end}}",
		},
		// Else-with chain in HTML.
		{
			name:   "else with chain in HTML",
			input:  "{{with .X}}<p>X</p>{{else with .Y}}<p>Y</p>{{end}}",
			result: "{{with .X}}<p>X</p>{{else with .Y}}<p>Y</p>{{end}}",
		},
		// Break in range with HTML.
		{
			name:   "break in range with HTML",
			input:  "{{range .Items}}<li>{{if .Done}}{{break}}{{end}}{{.Name}}</li>{{end}}",
			result: "{{range .Items}}<li>{{if .Done}}{{break}}{{end}}{{.Name}}</li>{{end}}",
		},
		// Continue in range with HTML.
		{
			name:   "continue in range with HTML",
			input:  "{{range .Items}}<li>{{if .Skip}}{{continue}}{{end}}{{.Name}}</li>{{end}}",
			result: "{{range .Items}}<li>{{if .Skip}}{{continue}}{{end}}{{.Name}}</li>{{end}}",
		},
		// Nested range with $.
		{
			name:   "nested range with $",
			input:  "{{range $i, $v := .X}}<div>{{range $.Y}}<span>{{$i}}</span>{{end}}</div>{{end}}",
			result: "{{range $i, $v := .X}}<div>{{range $.Y}}<span>{{$i}}</span>{{end}}</div>{{end}}",
		},
		// Multi-line indented template.
		{
			name:   "multi-line indented template",
			input:  "<div>\n  <p>\n    {{.Name}}\n  </p>\n</div>",
			result: "<div>\n  <p>\n    {{.Name}}\n  </p>\n</div>",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tmpl, err := ParseTree("test.gohtml", test.name, test.input, nil, lex.NewOptions())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			result := tmpl.Root.String()
			if result != test.result {
				t.Errorf("got\n\t%s\nexpected\n\t%s", result, test.result)
			}
			requireASTEqual(t, tmpl.Root, test.exp)
		})
	}
}
