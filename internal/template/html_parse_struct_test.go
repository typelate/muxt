package template

import (
	"testing"

	"github.com/typelate/muxt/internal/template/lex"
)

// TestHTMLParseStructural verifies that the unified parser produces a
// faithful DocumentFragment representation of source HTML. Unlike
// x/net/html which rewrites input into a well-formed HTML5 document
// (adding <html>, <head>, <body>, moving misplaced elements), our parser
// preserves source structure exactly. These tests verify we capture all
// elements, attributes, and text as-written.
//
// Inputs are drawn from webkit test data and other structural edge cases.
// We do NOT compare against x/net/html output because the two parsers have
// fundamentally different goals (well-formed DOM vs source-faithful AST).
func TestHTMLParseStructural(t *testing.T) {
	tests := []struct {
		name  string
		input string
		exp   Node
	}{
		{
			name:  "text only",
			input: "FOO BAR",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&TextNode{Text: []byte("FOO BAR")},
			}}}},
		},
		{
			name:  "comment with text",
			input: "FOO<!-- BAR -->BAZ",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&TextNode{Text: []byte("FOO")},
				&HTMLCommentNode{Children: []Node{&TextNode{Text: []byte(" BAR ")}}},
				&TextNode{Text: []byte("BAZ")},
			}}}},
		},
		{
			name:  "simple element",
			input: "<p>Hello</p>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{
					&TextNode{Text: []byte("Hello")},
				}},
			}}}},
		},
		{
			name:  "nested elements",
			input: "<div><p>Hello</p></div>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "div", Closed: true, Children: []Node{
					&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{
						&TextNode{Text: []byte("Hello")},
					}},
				}},
			}}}},
		},
		{
			name:  "void element",
			input: "<br>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "br", Void: true},
			}}}},
		},
		{
			name:  "doctype",
			input: "<!DOCTYPE html>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLDoctypeNode{Text: "<!DOCTYPE html>"},
			}}}},
		},
		{
			name:  "full page structure",
			input: "<!DOCTYPE html><html><head><title>Test</title></head><body><p>Hello</p></body></html>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLDoctypeNode{Text: "<!DOCTYPE html>"},
				&HTMLElementNode{TagName: "html", Closed: true, Children: []Node{
					&HTMLElementNode{TagName: "head", Closed: true, Children: []Node{
						&HTMLElementNode{TagName: "title", Closed: true, Children: []Node{
							&TextNode{Text: []byte("Test")},
						}},
					}},
					&HTMLElementNode{TagName: "body", Closed: true, Children: []Node{
						&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{
							&TextNode{Text: []byte("Hello")},
						}},
					}},
				}},
			}}}},
		},
		{
			name:  "attributes preserved",
			input: `<div class="foo" id="bar">text</div>`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "div", Closed: true, Attr: []Node{
					&HTMLAttributeNode{
						HasValue: true,
						Key:      []Node{&TextNode{Text: []byte("class")}},
						Value:    []Node{&TextNode{Text: []byte(`"foo"`)}},
					},
					&HTMLAttributeNode{
						HasValue: true,
						Key:      []Node{&TextNode{Text: []byte("id")}},
						Value:    []Node{&TextNode{Text: []byte(`"bar"`)}},
					},
				}, Children: []Node{
					&TextNode{Text: []byte("text")},
				}},
			}}}},
		},
		{
			name:  "boolean attribute",
			input: "<input disabled>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "input", Void: true, Attr: []Node{
					&HTMLAttributeNode{Key: []Node{&TextNode{Text: []byte("disabled")}}},
				}},
			}}}},
		},
		{
			name:  "self-closing",
			input: `<img src="test.png" />`,
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "img", Void: true, Attr: []Node{
					&HTMLAttributeNode{
						HasValue: true,
						Key:      []Node{&TextNode{Text: []byte("src")}},
						Value:    []Node{&TextNode{Text: []byte(`"test.png"`)}},
					},
				}},
			}}}},
		},
		{
			name:  "sibling elements",
			input: "<p>One</p><p>Two</p>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{
					&TextNode{Text: []byte("One")},
				}},
				&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{
					&TextNode{Text: []byte("Two")},
				}},
			}}}},
		},
		{
			name:  "mixed text and elements",
			input: "before<p>inside</p>after",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&TextNode{Text: []byte("before")},
				&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{
					&TextNode{Text: []byte("inside")},
				}},
				&TextNode{Text: []byte("after")},
			}}}},
		},
		// Unclosed elements — we parse as-written, not recovering.
		{
			name:  "unclosed element",
			input: "<p>Hello",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "p", Children: []Node{
					&TextNode{Text: []byte("Hello")},
				}},
			}}}},
		},
		{
			name:  "mismatched end tag",
			input: "<div><p>Hello</div>",
			// Like x/net/html: </div> matches ancestor <div>, so <p> is
			// implicitly closed (without a matching end tag) and </div>
			// closes the <div>.
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "div", Closed: true, Children: []Node{
					&HTMLElementNode{TagName: "p", Children: []Node{
						&TextNode{Text: []byte("Hello")},
					}},
				}},
			}}}},
		},
		// Raw text elements.
		{
			name:  "script element",
			input: "<script>var x = 1;</script>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "script", Closed: true, Children: []Node{
					&TextNode{Text: []byte("var x = 1;")},
				}},
			}}}},
		},
		{
			name:  "style element",
			input: "<style>body { color: red; }</style>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "style", Closed: true, Children: []Node{
					&TextNode{Text: []byte("body { color: red; }")},
				}},
			}}}},
		},
		// Template actions interleaved.
		{
			name:  "template in text",
			input: "<p>Hello {{.Name}}</p>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{
					&TextNode{Text: []byte("Hello ")},
					&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Name"}}}}}}},
				}},
			}}}},
		},
		{
			name:  "template in attribute",
			input: `<div class="{{.Class}}">text</div>`,
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
					&TextNode{Text: []byte("text")},
				}},
			}}}},
		},
		{
			name:  "control structure wrapping HTML",
			input: "{{range .Items}}<li>{{.}}</li>{{end}}",
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
		// Case-insensitive tag matching.
		{
			name:  "case-insensitive tag matching",
			input: "<DIV>hello</div>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "DIV", Closed: true, Children: []Node{
					&TextNode{Text: []byte("hello")},
				}},
			}}}},
		},
		// Mixed case tag name.
		{
			name:  "mixed case tag name",
			input: "<Div>hello</Div>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "Div", Closed: true, Children: []Node{
					&TextNode{Text: []byte("hello")},
				}},
			}}}},
		},
		// Case-insensitive void element.
		{
			name:  "case-insensitive void",
			input: "<BR>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "BR", Void: true},
			}}}},
		},
		// Orphan end tag in children — preserved verbatim, siblings correct.
		{
			name:  "orphan end tag in children",
			input: "<div><p>text</b></p></div>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "div", Closed: true, Children: []Node{
					&HTMLElementNode{TagName: "p", Closed: true, Children: []Node{
						&TextNode{Text: []byte("text")},
						&TextNode{Text: []byte("</b>"), Verbatim: true},
					}},
				}},
			}}}},
		},
		// Implicit close on ancestor end tag.
		{
			name:  "implicit close on ancestor",
			input: "<div><span><p>text</div>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "div", Closed: true, Children: []Node{
					&HTMLElementNode{TagName: "span", Children: []Node{
						&HTMLElementNode{TagName: "p", Children: []Node{
							&TextNode{Text: []byte("text")},
						}},
					}},
				}},
			}}}},
		},
		// Textarea raw text preserves child tags as text.
		{
			name:  "textarea raw text with tags",
			input: "<textarea><b>bold</b></textarea>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "textarea", Closed: true, Children: []Node{
					&TextNode{Text: []byte("<b>bold</b>")},
				}},
			}}}},
		},
		// Textarea contains action
		{
			name:  "textarea raw text with tags",
			input: "<textarea>Hello: {{.Value}}!</textarea>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "textarea", Closed: true, Children: []Node{
					&TextNode{Text: []byte("Hello: ")},
					&ActionNode{Pipe: &PipeNode{Commands: []*CommandNode{{Args: []Node{&FieldNode{Ident: []string{"Value"}}}}}}},
					&TextNode{Text: []byte("!")},
				}},
			}}}},
		},
		// Title raw text.
		{
			name:  "title raw text",
			input: "<title>Page <b>Title</b></title>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "title", Closed: true, Children: []Node{
					&TextNode{Text: []byte("Page <b>Title</b>")},
				}},
			}}}},
		},
		// Script with less-than character.
		{
			name:  "script with less-than",
			input: "<script>if (a < b) { x(); }</script>",
			exp: &ListNode{Nodes: []Node{&HTMLDocumentFragmentNode{Children: []Node{
				&HTMLElementNode{TagName: "script", Closed: true, Children: []Node{
					&TextNode{Text: []byte("if (a < b) { x(); }")},
				}},
			}}}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tmpl, err := ParseTree("test.gohtml", test.name, test.input, nil, lex.NewOptions())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			requireASTEqual(t, tmpl.Root, test.exp)
		})
	}
}
