package lex

import (
	"strings"
	"testing"
)

// collectHTML gathers the emitted items from an HTML-mode lexer.
func collectHTML(t *lexTest) (items []Token) {
	l := New(t.input, Options{
		ParseHTML:   true,
		EmitComment: true,
		BreakOK:     true,
		ContinueOK:  true,
	})
	for {
		item := l.Next()
		items = append(items, item)
		if item.Type == TokenTypeEOF || item.Type == TokenTypeError {
			break
		}
	}
	return
}

var htmlLexTests = []lexTest{
	// Plain text
	{name: "html empty", input: "", items: []Token{tEOF}},
	{name: "html plain text", input: "hello world", items: []Token{
		{Type: TokenTypeHTMLText, Value: "hello world"},
		tEOF,
	}},
	{name: "html text with newlines", input: "hello\nworld", items: []Token{
		{Type: TokenTypeHTMLText, Value: "hello\nworld"},
		tEOF,
	}},

	// Template actions in text
	{name: "html template in text", input: "hello {{.Name}} world", items: []Token{
		{Type: TokenTypeHTMLText, Value: "hello "},
		tLeft,
		{Type: TokenTypeField, Value: ".Name"},
		tRight,
		{Type: TokenTypeHTMLText, Value: " world"},
		tEOF,
	}},
	{name: "html template only", input: "{{.X}}", items: []Token{
		tLeft,
		{Type: TokenTypeField, Value: ".X"},
		tRight,
		tEOF,
	}},

	// Simple tags
	{name: "html simple tag", input: "<div>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "div"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},
	{name: "html end tag", input: "</div>", items: []Token{
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "div"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},
	{name: "html self-closing tag", input: "<br/>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "br"},
		{Type: TokenTypeHTMLSelfClose, Value: "/>"},
		tEOF,
	}},
	{name: "html self-closing with space", input: "<br />", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "br"},
		{Type: TokenTypeHTMLSelfClose, Value: "/>"},
		tEOF,
	}},

	// Tags with text content
	{name: "html tag with text", input: "<p>hello</p>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "p"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLText, Value: "hello"},
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "p"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Attributes
	{name: "html attr no value", input: "<input disabled>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "input"},
		{Type: TokenTypeHTMLAttrName, Value: "disabled"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},
	{name: "html attr with value", input: `<div class="foo">`, items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "div"},
		{Type: TokenTypeHTMLAttrName, Value: "class"},
		{Type: TokenTypeHTMLAttrEquals, Value: "="},
		{Type: TokenTypeHTMLAttrValue, Value: `"foo"`},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},
	{name: "html attr single quoted", input: "<div class='foo'>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "div"},
		{Type: TokenTypeHTMLAttrName, Value: "class"},
		{Type: TokenTypeHTMLAttrEquals, Value: "="},
		{Type: TokenTypeHTMLAttrValue, Value: "'foo'"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},
	{name: "html multiple attrs", input: `<a href="/" class="link">`, items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "a"},
		{Type: TokenTypeHTMLAttrName, Value: "href"},
		{Type: TokenTypeHTMLAttrEquals, Value: "="},
		{Type: TokenTypeHTMLAttrValue, Value: `"/"`},
		{Type: TokenTypeHTMLAttrName, Value: "class"},
		{Type: TokenTypeHTMLAttrEquals, Value: "="},
		{Type: TokenTypeHTMLAttrValue, Value: `"link"`},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Template actions in attributes
	{name: "html template in attr value", input: `<div class="base {{.Class}}">`, items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "div"},
		{Type: TokenTypeHTMLAttrName, Value: "class"},
		{Type: TokenTypeHTMLAttrEquals, Value: "="},
		{Type: TokenTypeHTMLAttrValue, Value: `"base `},
		tLeft,
		{Type: TokenTypeField, Value: ".Class"},
		tRight,
		{Type: TokenTypeHTMLAttrValue, Value: `"`},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Comments
	{name: "html comment", input: "<!-- hello -->", items: []Token{
		{Type: TokenTypeHTMLCommentOpen, Value: "<!--"},
		{Type: TokenTypeHTMLComment, Value: " hello "},
		{Type: TokenTypeHTMLCommentClose, Value: "-->"},
		tEOF,
	}},
	{name: "html comment with template", input: "<!-- {{.Version}} -->", items: []Token{
		{Type: TokenTypeHTMLCommentOpen, Value: "<!--"},
		{Type: TokenTypeHTMLComment, Value: " "},
		tLeft,
		{Type: TokenTypeField, Value: ".Version"},
		tRight,
		{Type: TokenTypeHTMLComment, Value: " "},
		{Type: TokenTypeHTMLCommentClose, Value: "-->"},
		tEOF,
	}},

	// Doctype
	{name: "html doctype", input: "<!DOCTYPE html>", items: []Token{
		{Type: TokenTypeHTMLDoctype, Value: "<!DOCTYPE html>"},
		tEOF,
	}},

	// Mixed content
	{name: "html mixed", input: `<!DOCTYPE html><html><body><p>{{.Content}}</p></body></html>`, items: []Token{
		{Type: TokenTypeHTMLDoctype, Value: "<!DOCTYPE html>"},
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "html"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "body"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "p"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tLeft,
		{Type: TokenTypeField, Value: ".Content"},
		tRight,
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "p"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "body"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "html"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Template control in HTML
	{name: "html range with items", input: `{{range .Items}}<li>{{.}}</li>{{end}}`, items: []Token{
		tLeft,
		{Type: TokenTypeRange, Value: "range"},
		{Type: TokenTypeSpace, Value: " "},
		{Type: TokenTypeField, Value: ".Items"},
		tRight,
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "li"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tLeft,
		{Type: TokenTypeDot, Value: "."},
		tRight,
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "li"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tLeft,
		{Type: TokenTypeEnd, Value: "end"},
		tRight,
		tEOF,
	}},

	// Template action in attribute position
	{name: "html template in attr position", input: `<div {{if .Show}}class="visible"{{end}}>`, items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "div"},
		tLeft,
		{Type: TokenTypeIf, Value: "if"},
		{Type: TokenTypeSpace, Value: " "},
		{Type: TokenTypeField, Value: ".Show"},
		tRight,
		{Type: TokenTypeHTMLAttrName, Value: "class"},
		{Type: TokenTypeHTMLAttrEquals, Value: "="},
		{Type: TokenTypeHTMLAttrValue, Value: `"visible"`},
		tLeft,
		{Type: TokenTypeEnd, Value: "end"},
		tRight,
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Cases ported from internal/html/token_test.go.
	// Our lexer produces raw tokens (no entity decoding, no case normalization,
	// no error recovery), so only well-formed cases that don't rely on those
	// features are ported.

	// Tags (from "tags" test case)
	{name: "html port: tags", input: "<a>b<c/>d</e>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "a"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLText, Value: "b"},
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "c"},
		{Type: TokenTypeHTMLSelfClose, Value: "/>"},
		{Type: TokenTypeHTMLText, Value: "d"},
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "e"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Basic raw text (from "basic raw text")
	// The lexer recognizes raw text elements and emits inner content as HTMLText.
	{name: "html port: basic raw text", input: "<script><a></b></script>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "script"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLText, Value: "<a></b>"},
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "script"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Proper DOCTYPE (from "Proper DOCTYPE")
	{name: "html port: proper DOCTYPE", input: "<!DOCTYPE html>", items: []Token{
		{Type: TokenTypeHTMLDoctype, Value: "<!DOCTYPE html>"},
		tEOF,
	}},

	// Comment with surrounding text (from "comment0")
	{name: "html port: comment0", input: "abc<b><!-- skipme --></b>def", items: []Token{
		{Type: TokenTypeHTMLText, Value: "abc"},
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "b"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLCommentOpen, Value: "<!--"},
		{Type: TokenTypeHTMLComment, Value: " skipme "},
		{Type: TokenTypeHTMLCommentClose, Value: "-->"},
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "b"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLText, Value: "def"},
		tEOF,
	}},

	// Empty attribute (from "Empty attribute")
	{name: "html port: empty attribute", input: "<input disabled FOO>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "input"},
		{Type: TokenTypeHTMLAttrName, Value: "disabled"},
		{Type: TokenTypeHTMLAttrName, Value: "FOO"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Unquoted attribute value (from "Unquoted attribute value")
	{name: "html port: unquoted attr value", input: "<input value=yes FOO=BAR>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "input"},
		{Type: TokenTypeHTMLAttrName, Value: "value"},
		{Type: TokenTypeHTMLAttrEquals, Value: "="},
		{Type: TokenTypeHTMLAttrValue, Value: "yes"},
		{Type: TokenTypeHTMLAttrName, Value: "FOO"},
		{Type: TokenTypeHTMLAttrEquals, Value: "="},
		{Type: TokenTypeHTMLAttrValue, Value: "BAR"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Single-quoted attribute value (from "Single-quoted attribute value")
	{name: "html port: single-quoted attr", input: "<input value='yes' FOO='BAR'>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "input"},
		{Type: TokenTypeHTMLAttrName, Value: "value"},
		{Type: TokenTypeHTMLAttrEquals, Value: "="},
		{Type: TokenTypeHTMLAttrValue, Value: "'yes'"},
		{Type: TokenTypeHTMLAttrName, Value: "FOO"},
		{Type: TokenTypeHTMLAttrEquals, Value: "="},
		{Type: TokenTypeHTMLAttrValue, Value: "'BAR'"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Double-quoted attribute value (from "Double-quoted attribute value")
	{name: "html port: double-quoted attr", input: `<input value="I'm an attribute" FOO="BAR">`, items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "input"},
		{Type: TokenTypeHTMLAttrName, Value: "value"},
		{Type: TokenTypeHTMLAttrEquals, Value: "="},
		{Type: TokenTypeHTMLAttrValue, Value: `"I'm an attribute"`},
		{Type: TokenTypeHTMLAttrName, Value: "FOO"},
		{Type: TokenTypeHTMLAttrEquals, Value: "="},
		{Type: TokenTypeHTMLAttrValue, Value: `"BAR"`},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Attribute name with hyphens (from "Attribute name characters")
	{name: "html port: attr name with hyphens", input: `<meta http-equiv="content-type">`, items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "meta"},
		{Type: TokenTypeHTMLAttrName, Value: "http-equiv"},
		{Type: TokenTypeHTMLAttrEquals, Value: "="},
		{Type: TokenTypeHTMLAttrValue, Value: `"content-type"`},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Mixed attributes (from "Mixed attributes")
	{name: "html port: mixed attrs", input: `a<P V="0 1" w='2' X=3 y>z`, items: []Token{
		{Type: TokenTypeHTMLText, Value: "a"},
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "P"},
		{Type: TokenTypeHTMLAttrName, Value: "V"},
		{Type: TokenTypeHTMLAttrEquals, Value: "="},
		{Type: TokenTypeHTMLAttrValue, Value: `"0 1"`},
		{Type: TokenTypeHTMLAttrName, Value: "w"},
		{Type: TokenTypeHTMLAttrEquals, Value: "="},
		{Type: TokenTypeHTMLAttrValue, Value: `'2'`},
		{Type: TokenTypeHTMLAttrName, Value: "X"},
		{Type: TokenTypeHTMLAttrEquals, Value: "="},
		{Type: TokenTypeHTMLAttrValue, Value: "3"},
		{Type: TokenTypeHTMLAttrName, Value: "y"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLText, Value: "z"},
		tEOF,
	}},

	// Nested tags (from "tags" family)
	{name: "html port: nested tags", input: "<div><p>hello</p></div>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "div"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "p"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLText, Value: "hello"},
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "p"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "div"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Textarea with tag (from "textarea with tag") — raw text element
	{name: "html port: textarea with tag", input: "<textarea><div></textarea>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "textarea"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLText, Value: "<div>"},
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "textarea"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Title with tag (from "title with tag and entity") — raw text element
	{name: "html port: title with tags", input: "<title><b>K&amp;R C</b></title>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "title"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLText, Value: "<b>K&amp;R C</b>"},
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "title"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// DOCTYPE with two spaces (from "DOCTYPE with two spaces")
	{name: "html port: DOCTYPE two spaces", input: "<!doctype  html>", items: []Token{
		{Type: TokenTypeHTMLDoctype, Value: "<!doctype  html>"},
		tEOF,
	}},

	// Style element (raw text)
	{name: "html port: style element", input: "<style>body { color: red; }</style>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "style"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLText, Value: "body { color: red; }"},
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "style"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Non-standard / edge cases ported from internal/html/token_test.go.
	// Where the HTML5 tokenizer recovers from malformed input, our lexer
	// reads the markup it cannot lex as a tag as text, as text/template
	// does. These tests document our lexer's behavior.

	// Bare < (from "not a tag #0")
	{name: "html port: bare <", input: "<", items: []Token{
		{Type: TokenTypeHTMLText, Value: "<"},
		tEOF,
	}},

	// Bare </ (from "not a tag #1")
	{name: "html port: bare </", input: "</", items: []Token{
		{Type: TokenTypeHTMLText, Value: "</"},
		tEOF,
	}},

	// < in text (from "not a tag #7")
	{name: "html port: < in text", input: "a < b", items: []Token{
		{Type: TokenTypeHTMLText, Value: "a < b"},
		tEOF,
	}},

	// Multiple < (from "not a tag #9")
	{name: "html port: multiple <", input: "a<<<b>>>c", items: []Token{
		{Type: TokenTypeHTMLText, Value: "a<<"},
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "b"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLText, Value: ">>c"},
		tEOF,
	}},

	// </> (from "not a tag #2")
	{name: "html port: </>", input: "</>", items: []Token{
		{Type: TokenTypeHTMLText, Value: "</>"},
		tEOF,
	}},

	// a</>b (from "not a tag #3")
	{name: "html port: a</>b", input: "a</>b", items: []Token{
		{Type: TokenTypeHTMLText, Value: "a</>b"},
		tEOF,
	}},

	// </ > (from "not a tag #4")
	{name: "html port: </ >", input: "</ >", items: []Token{
		{Type: TokenTypeHTMLText, Value: "</ >"},
		tEOF,
	}},

	// </.> (from "not a tag #6")
	{name: "html port: </.>", input: "</.>", items: []Token{
		{Type: TokenTypeHTMLText, Value: "</.>"},
		tEOF,
	}},

	// <<p> (from "not a tag #11")
	{name: "html port: <<p>", input: "<<p>", items: []Token{
		{Type: TokenTypeHTMLText, Value: "<"},
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "p"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// EOF in tag name (from "tag name eof #0")
	{name: "html port: EOF in tag name", input: "<a", items: []Token{
		{Type: TokenTypeHTMLText, Value: "<a"},
		tEOF,
	}},

	// EOF in tag with trailing space (from "tag name eof #1")
	{name: "html port: EOF in tag space", input: "<a ", items: []Token{
		{Type: TokenTypeHTMLText, Value: "<a "},
		tEOF,
	}},

	// Text then EOF in tag (from "tag name eof #2")
	{name: "html port: text then EOF tag", input: "a<b", items: []Token{
		{Type: TokenTypeHTMLText, Value: "a"},
		{Type: TokenTypeHTMLText, Value: "<b"},
		tEOF,
	}},

	// Tag then EOF in tag (from "tag name eof #3")
	{name: "html port: tag then EOF tag", input: "<a><b", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "a"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLText, Value: "<b"},
		tEOF,
	}},

	// EOF in attribute (from "tag name eof #4")
	{name: "html port: EOF in attr", input: "<a x", items: []Token{
		{Type: TokenTypeHTMLText, Value: "<a x"},
		tEOF,
	}},

	// EOF after attr name (from "malformed tag #2")
	{name: "html port: EOF after attr name", input: "<p id", items: []Token{
		{Type: TokenTypeHTMLText, Value: "<p id"},
		tEOF,
	}},

	// EOF after attr equals (from "malformed tag #3")
	{name: "html port: EOF after attr equals", input: "<p id=", items: []Token{
		{Type: TokenTypeHTMLText, Value: "<p id="},
		tEOF,
	}},

	// Empty attr value (from "malformed tag #4")
	{name: "html port: empty attr value", input: "<p id=>", items: []Token{
		{Type: TokenTypeHTMLText, Value: "<p id=>"},
		tEOF,
	}},

	// EOF in unquoted attr value (from "malformed tag #5")
	{name: "html port: EOF in unquoted attr", input: "<p id=0", items: []Token{
		{Type: TokenTypeHTMLText, Value: "<p id=0"},
		tEOF,
	}},

	// Junk in end tag (from "malformed tag #9")
	{name: "html port: junk in end tag", input: "<p></p id", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "p"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLText, Value: "</p id"},
		tEOF,
	}},

	// Bogus comment <!> treated as doctype (from "comment5")
	{name: "html port: bogus <!>", input: "a<!>z", items: []Token{
		{Type: TokenTypeHTMLText, Value: "a"},
		{Type: TokenTypeHTMLDoctype, Value: "<!>"},
		{Type: TokenTypeHTMLText, Value: "z"},
		tEOF,
	}},

	// Bogus comment <!-> treated as doctype (from "comment6")
	{name: "html port: bogus <!->", input: "a<!->z", items: []Token{
		{Type: TokenTypeHTMLText, Value: "a"},
		{Type: TokenTypeHTMLDoctype, Value: "<!->"},
		{Type: TokenTypeHTMLText, Value: "z"},
		tEOF,
	}},

	// Unclosed comment (from "comment8")
	{name: "html port: unclosed comment", input: "a<!--z", items: []Token{
		{Type: TokenTypeHTMLText, Value: "a<!--z"},
		tEOF,
	}},

	// Comment with content (from "comment3")
	{name: "html port: comment with >", input: "a<!--x>-->z", items: []Token{
		{Type: TokenTypeHTMLText, Value: "a"},
		{Type: TokenTypeHTMLCommentOpen, Value: "<!--"},
		{Type: TokenTypeHTMLComment, Value: "x>"},
		{Type: TokenTypeHTMLCommentClose, Value: "-->"},
		{Type: TokenTypeHTMLText, Value: "z"},
		tEOF,
	}},

	// Empty comment (from "comment1" area)
	{name: "html port: empty comment", input: "<!---->", items: []Token{
		{Type: TokenTypeHTMLCommentOpen, Value: "<!--"},
		{Type: TokenTypeHTMLCommentClose, Value: "-->"},
		tEOF,
	}},

	// Comment surrounding text (from "comment0" — already tested above, but
	// including an isolated version here)
	{name: "html port: comment in text", input: "x<!-- hello -->y", items: []Token{
		{Type: TokenTypeHTMLText, Value: "x"},
		{Type: TokenTypeHTMLCommentOpen, Value: "<!--"},
		{Type: TokenTypeHTMLComment, Value: " hello "},
		{Type: TokenTypeHTMLCommentClose, Value: "-->"},
		{Type: TokenTypeHTMLText, Value: "y"},
		tEOF,
	}},

	// XML processing instruction treated as doctype (from "XML processing instruction")
	{name: "html port: XML PI", input: "<?xml?>", items: []Token{
		{Type: TokenTypeHTMLDoctype, Value: "<?xml?>"},
		tEOF,
	}},

	// Trim markers in HTML text — whitespace before {{- and after -}} is trimmed.
	// The delimiter tokens are still just "{{" and "}}" (trim marker is consumed
	// but not included in the token value, matching stdlib text/template behavior).
	// Text content is NOT trimmed by the lexer — trim is deferred to execution.
	{name: "html trim markers in text", input: "hello {{- .Foo -}} world", items: []Token{
		{Type: TokenTypeHTMLText, Value: "hello "},
		tLeft,
		{Type: TokenTypeField, Value: ".Foo"},
		tRight,
		{Type: TokenTypeHTMLText, Value: " world"},
		tEOF,
	}},

	// Unquoted attribute value with template action — exercises
	// lexHTMLAttrValueUnquoted template-action path.
	{name: "html unquoted attr with template action", input: `<div class={{.Class}}>`, items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "div"},
		{Type: TokenTypeHTMLAttrName, Value: "class"},
		{Type: TokenTypeHTMLAttrEquals, Value: "="},
		tLeft,
		{Type: TokenTypeField, Value: ".Class"},
		tRight,
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Single-quoted attribute value with template action — exercises
	// lexHTMLAttrValueSingleQuotedInner action path.
	{name: "html single-quoted attr with template action", input: `<div class='{{.Class}}'>`, items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "div"},
		{Type: TokenTypeHTMLAttrName, Value: "class"},
		{Type: TokenTypeHTMLAttrEquals, Value: "="},
		{Type: TokenTypeHTMLAttrValue, Value: "'"},
		tLeft,
		{Type: TokenTypeField, Value: ".Class"},
		tRight,
		{Type: TokenTypeHTMLAttrValue, Value: "'"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// data-* attribute name with hyphens.
	{name: "html data-* attribute", input: `<div data-value="x">`, items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "div"},
		{Type: TokenTypeHTMLAttrName, Value: "data-value"},
		{Type: TokenTypeHTMLAttrEquals, Value: "="},
		{Type: TokenTypeHTMLAttrValue, Value: `"x"`},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Upper case tag name — preserved as-is.
	{name: "html upper case tag", input: "<DIV></DIV>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "DIV"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "DIV"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Unquoted value terminated at whitespace, then more attrs follow.
	{name: "html unquoted value then more attrs", input: `<input type=text name="n">`, items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "input"},
		{Type: TokenTypeHTMLAttrName, Value: "type"},
		{Type: TokenTypeHTMLAttrEquals, Value: "="},
		{Type: TokenTypeHTMLAttrValue, Value: "text"},
		{Type: TokenTypeHTMLAttrName, Value: "name"},
		{Type: TokenTypeHTMLAttrEquals, Value: "="},
		{Type: TokenTypeHTMLAttrValue, Value: `"n"`},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Raw text elements: bare < in content (previously caused lexer error).
	{name: "html raw text: bare < in script", input: "<script>if (a < b) { x(); }</script>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "script"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLText, Value: "if (a < b) { x(); }"},
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "script"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Raw text element with template action inside.
	{name: "html raw text: template action in script", input: "<script>var data = {{.X}};</script>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "script"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLText, Value: "var data = "},
		tLeft,
		{Type: TokenTypeField, Value: ".X"},
		tRight,
		{Type: TokenTypeHTMLText, Value: ";"},
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "script"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Raw text: non-matching end tag (</scripting>) is consumed as text.
	{name: "html raw text: non-matching end tag", input: "<script></scripting></script>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "script"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLText, Value: "</scripting>"},
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "script"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Raw text: case-insensitive end tag matching (per WHATWG).
	{name: "html raw text: case insensitive end tag", input: "<SCRIPT>x</SCRipt>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "SCRIPT"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLText, Value: "x"},
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "SCRipt"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Raw text: end tag with whitespace before > (per WHATWG, whitespace is a terminator).
	{name: "html raw text: end tag with whitespace", input: "<script>x</script >", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "script"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLText, Value: "x"},
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "script"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Raw text: empty content.
	{name: "html raw text: empty script", input: "<script></script>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "script"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "script"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Raw text: unclosed at EOF.
	{name: "html raw text: unclosed script", input: "<script>hello", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "script"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLText, Value: "hello"},
		tEOF,
	}},

	// Raw text: multiple template actions.
	{name: "html raw text: multiple actions", input: "<script>{{.A}}+{{.B}}</script>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "script"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tLeft,
		{Type: TokenTypeField, Value: ".A"},
		tRight,
		{Type: TokenTypeHTMLText, Value: "+"},
		tLeft,
		{Type: TokenTypeField, Value: ".B"},
		tRight,
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "script"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Raw text: trim markers work inside raw text.
	// Text content is NOT trimmed by the lexer — trim is deferred to execution.
	{name: "html raw text: trim markers", input: "<script>x {{- .Y -}} z</script>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "script"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLText, Value: "x "},
		tLeft,
		{Type: TokenTypeField, Value: ".Y"},
		tRight,
		{Type: TokenTypeHTMLText, Value: " z"},
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "script"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Raw text: style element.
	{name: "html raw text: style with < in content", input: "<style>a < b { color: red }</style>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "style"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLText, Value: "a < b { color: red }"},
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "style"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Raw text: script with attributes.
	{name: "html raw text: script with attrs", input: `<script type="text/javascript">x</script>`, items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "script"},
		{Type: TokenTypeHTMLAttrName, Value: "type"},
		{Type: TokenTypeHTMLAttrEquals, Value: "="},
		{Type: TokenTypeHTMLAttrValue, Value: `"text/javascript"`},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLText, Value: "x"},
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "script"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},

	// Dynamic attribute name and value with template actions.
	// The lexer splits the attribute name across HTMLAttrName and template
	// action tokens. It does not group them into a single attribute name;
	// that grouping (if desired) would be done by the parser.
	{name: "html dynamic attr name and value", input: `<p data-{{.Key}}_xy-z='foo {{.Foo}}{{if .Cond}}c{{end}} '>`, items: []Token{
		{Type: TokenTypeHTMLText, Value: "<p data-"},
		tLeft,
		{Type: TokenTypeField, Value: ".Key"},
		tRight,
		{Type: TokenTypeHTMLText, Value: "_xy-z='foo "},
		tLeft,
		{Type: TokenTypeField, Value: ".Foo"},
		tRight,
		tLeft,
		{Type: TokenTypeIf, Value: "if"},
		tSpace,
		{Type: TokenTypeField, Value: ".Cond"},
		tRight,
		{Type: TokenTypeHTMLText, Value: "c"},
		tLeft,
		{Type: TokenTypeEnd, Value: "end"},
		tRight,
		{Type: TokenTypeHTMLText, Value: " '>"},
		tEOF,
	}},

	// Raw text: template action followed by content containing < and >.
	{name: "html raw text: action then bare angle brackets", input: "<style>a {{.X}} b < c > d</style>", items: []Token{
		{Type: TokenTypeHTMLTagOpen, Value: "<"},
		{Type: TokenTypeHTMLStartTag, Value: "style"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		{Type: TokenTypeHTMLText, Value: "a "},
		tLeft,
		{Type: TokenTypeField, Value: ".X"},
		tRight,
		{Type: TokenTypeHTMLText, Value: " b < c > d"},
		{Type: TokenTypeHTMLEndTagOpen, Value: "</"},
		{Type: TokenTypeHTMLEndTag, Value: "style"},
		{Type: TokenTypeHTMLTagEnd, Value: ">"},
		tEOF,
	}},
}

func TestHTMLLex(t *testing.T) {
	for _, test := range htmlLexTests {
		t.Run(test.name, func(t *testing.T) {
			items := collectHTML(&test)
			if !equalSkipPosCheck(items, test.items) {
				t.Errorf("got\n\t%+v\nexpected\n\t%v", items, test.items)
			}
		})
	}
}

// TestHTMLLexEquivalence maps token_test.go cases from internal/html to the
// unified HTML-mode lexer. Our lexer is a raw tokenizer: it does not decode
// entities, normalize case, or perform error recovery. This test documents
// which cases our lexer handles and which produce errors.
//
// Cases are categorized as:
//   - "match": our lexer produces tokens that structurally match the HTML tokenizer
//   - "error": our lexer produces an error (expected for malformed inputs)
//   - Skip: the case requires features our lexer intentionally omits
//     (entity decoding, case normalization, error recovery)
func TestHTMLLexEquivalence(t *testing.T) {
	type expectationType int

	const (
		Match expectationType = iota
		Error
		Skip
	)

	type equivTest struct {
		desc   string // from tokenTest.desc
		html   string // from tokenTest.html
		expect expectationType
		reason string // why skip or error
	}

	tests := []equivTest{
		// Well-formed cases — our lexer produces equivalent structure.
		{desc: "empty", html: "", expect: Match, reason: ""},
		{desc: "text", html: "foo  bar", expect: Match, reason: ""},
		{desc: "tags", html: "<a>b<c/>d</e>", expect: Match, reason: ""},
		{desc: "Proper DOCTYPE", html: "<!DOCTYPE html>", expect: Match, reason: ""},
		{desc: "comment0", html: "abc<b><!-- skipme --></b>def", expect: Match, reason: ""},
		{desc: "Empty attribute", html: "<input disabled FOO>", expect: Match, reason: ""},
		{desc: "Unquoted attribute value", html: "<input value=yes FOO=BAR>", expect: Match, reason: ""},
		{desc: "Single-quoted attribute value", html: "<input value='yes' FOO='BAR'>", expect: Match, reason: ""},
		{desc: "Double-quoted attribute value", html: `<input value="I'm an attribute" FOO="BAR">`, expect: Match, reason: ""},
		{desc: "Attribute name characters", html: `<meta http-equiv="content-type">`, expect: Match, reason: ""},
		{desc: "basic raw text", html: "<script><a></b></script>", expect: Match, reason: ""},
		{desc: "textarea with tag", html: "<textarea><div></textarea>", expect: Match, reason: ""},
		{desc: "title with tag and entity", html: "<title><b>K&amp;R C</b></title>", expect: Match, reason: ""},
		{desc: "'>' completes script end tag", html: "<SCRIPT>a</SCRipt>", expect: Match, reason: ""},

		// Malformed markup. text/template and html/template parse it, so our
		// lexer reads what it cannot lex as a tag as text (see lexHTMLText).
		{desc: "not a tag #0", html: "<", expect: Match, reason: "text: bare < at EOF"},
		{desc: "not a tag #1", html: "</", expect: Match, reason: "text: bare </ at EOF"},
		{desc: "not a tag #2", html: "</>", expect: Match, reason: "text: empty end tag"},
		{desc: "not a tag #3", html: "a</>b", expect: Match, reason: "text: empty end tag after text"},
		{desc: "not a tag #4", html: "</ >", expect: Match, reason: "text: space after </"},
		{desc: "not a tag #5", html: "</.", expect: Match, reason: "text: non-alpha after </"},
		{desc: "not a tag #6", html: "</.>", expect: Match, reason: "text: non-alpha after </"},
		{desc: "not a tag #7", html: "a < b", expect: Match, reason: "text: bare < in text"},
		{desc: "not a tag #8", html: "<.>", expect: Match, reason: "text: non-alpha after <"},
		{desc: "not a tag #9", html: "a<<<b>>>c", expect: Match, reason: "text: multiple < signs"},
		{desc: "not a tag #10", html: "if x<0 and y < 0 then x*y>0", expect: Match, reason: "text: < in expression"},
		{desc: "not a tag #11", html: "<<p>", expect: Match, reason: "text: doubled <"},
		{desc: "tag name eof #0", html: "<a", expect: Match, reason: "text: EOF in tag"},
		{desc: "tag name eof #1", html: "<a ", expect: Match, reason: "text: EOF after tag name"},
		{desc: "tag name eof #2", html: "a<b", expect: Match, reason: "text: EOF in tag after text"},
		{desc: "tag name eof #3", html: "<a><b", expect: Match, reason: "text: EOF in second tag"},
		{desc: "tag name eof #4", html: "<a x", expect: Match, reason: "text: EOF in attribute"},
		{desc: "malformed tag #0", html: "<p</p>", expect: Match, reason: "text: < in tag name"},
		{desc: "malformed tag #1", html: "<p </p>", expect: Match, reason: "text: < in attributes"},
		{desc: "malformed tag #2", html: "<p id", expect: Match, reason: "text: EOF after attr name"},
		{desc: "malformed tag #3", html: "<p id=", expect: Match, reason: "text: EOF after ="},
		{desc: "malformed tag #4", html: "<p id=>", expect: Match, reason: "text: empty value after ="},
		{desc: "malformed tag #5", html: "<p id=0", expect: Match, reason: "text: EOF in unquoted value"},
		{desc: "malformed tag #6", html: "<p id=0</p>", expect: Match, reason: "lexer treats 0</p as unquoted value"},
		{desc: "malformed tag #7", html: `<p id="0</p>`, expect: Match, reason: "text: EOF in quoted value"},
		{desc: "malformed tag #8", html: `<p id="0"</p>`, expect: Match, reason: "text: < after quoted value"},
		{desc: "malformed tag #9", html: "<p></p id", expect: Match, reason: "text: junk in end tag"},

		// Cases that require features our lexer intentionally omits.

		// golang.org/x/net/html token_test.go:51
		{desc: "entity", html: "one &lt; two", expect: Skip, reason: "entity decoding"},

		// golang.org/x/net/html token_test.go:515
		{desc: "tricky", html: "<p \t\n iD=\"a&quot;B\"  foo=\"bar\"><EM>te&lt;&amp;;xt</em></p>", expect: Skip, reason: "entity decoding + case normalization"},

		// golang.org/x/net/html token_test.go:522
		{desc: "noSuchEntity", html: `<a b="c&noSuchEntity;d">&lt;&alsoDoesntExist;&`, expect: Skip, reason: "entity decoding"},

		// golang.org/x/net/html token_test.go:527
		{desc: "entity without semicolon", html: `&notit;&notin;<a b="q=z&amp=5&notice=hello&not;=world">`, expect: Skip, reason: "entity decoding"},

		// golang.org/x/net/html token_test.go:532
		{desc: "entity with digits", html: "&frac12;", expect: Skip, reason: "entity decoding"},

		// golang.org/x/net/html token_test.go:508
		// Backslash does not escape quotes in HTML. Our lexer treats '\' as
		// literal — the first '"' after '\' terminates the value, matching the
		// reference tokenizer's behavior. Subsequent characters parse slightly
		// differently (attr name "b\"" vs "b") but both are valid raw
		// tokenizations of malformed input.
		{desc: "backslash", html: `<p id="a\"b">`, expect: Match, reason: ""},
		{desc: "Mixed attributes", html: `a<P V="0 1" w='2' X=3 y>z`, expect: Match, reason: ""},

		// golang.org/x/net/html token_test.go:589
		// Single quotes are allowed in unquoted attribute values per HTML5.
		// Our lexer correctly includes them — the value terminates at '>' or
		// whitespace, not at quotes. The reference tokenizer entity-encodes
		// quotes in output (can&#39;t) but the structural tokenization matches.
		{desc: "Attributes with a solitary single quote", html: `<p id=can't><p id=won't>`, expect: Match, reason: ""},

		// golang.org/x/net/html token_test.go:315
		// Per HTML5 WHATWG 13.2.5.43, <!--> is an "abruptly closed comment"
		// (parse error but valid). html/template handles this at runtime via
		// golang.org/x/net/html, so tooling must accept it.
		{desc: "comment1", html: "a<!-->z", expect: Match, reason: ""},

		// golang.org/x/net/html token_test.go:320
		// Per HTML5 WHATWG 13.2.5.44, <!---> is an "abruptly closed comment"
		// (parse error but valid). Same rationale as comment1.
		{desc: "comment2", html: "a<!--->z", expect: Match, reason: ""},
		{desc: "comment3", html: "a<!--x>-->z", expect: Match, reason: ""},
		{desc: "comment5", html: "a<!>z", expect: Match, reason: ""},
		{desc: "comment6", html: "a<!->z", expect: Match, reason: ""},
		{desc: "comment8", html: "a<!--z", expect: Match, reason: "text: unclosed comment"},
		{desc: "DOCTYPE with no space", html: "<!doctypehtml>", expect: Match, reason: ""},
		{desc: "DOCTYPE with two spaces", html: "<!doctype  html>", expect: Match, reason: ""},
		{desc: "looks like DOCTYPE but isn't", html: "<!DOCUMENT html>", expect: Match, reason: ""},
		{desc: "DOCTYPE at EOF", html: "<!DOCtype", expect: Match, reason: "text: EOF in doctype"},
		{desc: "XML processing instruction", html: "<?xml?>", expect: Match, reason: ""},

		// golang.org/x/net/html token_test.go:625
		// In <p a=/>, our unquoted value scanner treats '/' as a literal
		// character and '>' as the terminator, producing value "/" then tag
		// close '>'. The reference tokenizer produces the same structure:
		// <p a="/">. Both agree '/' is the value, not a self-closing marker.
		{desc: "self-closing tag with attribute", html: "<p a=/>", expect: Match, reason: ""},

		// golang.org/x/net/html token_test.go:595
		// The reference tokenizer applies WHATWG 13.2.5.32 error recovery,
		// treating '=' as an attribute name. Our lexer requires attribute names
		// to start with a non-delimiter character and reads the tag as text.
		{desc: "equals sign before attribute name", html: "<p  =>", expect: Match, reason: "text: = as attr name"},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			lt := lexTest{name: test.desc, input: test.html}
			items := collectHTML(&lt)

			hasError := false
			for _, item := range items {
				if item.Type == TokenTypeError {
					hasError = true
					break
				}
			}

			switch test.expect {
			case Match:
				if hasError {
					t.Errorf("expected successful lex, got error: %v", items)
				}
			case Error:
				if !hasError {
					t.Errorf("expected error, got tokens: %v", items)
				}
			case Skip:
				t.Skipf("skipped: %s", test.reason)
			}
		})
	}
}

// TestHTMLLexRoundTrip verifies that for HTML inputs without inter-token
// whitespace (which our lexer consumes during tag scanning), the
// concatenation of all token values reconstructs the original input.
// Our lexer is not a passthrough tokenizer — whitespace between tag name
// and attributes, and before />, is consumed and not emitted as tokens.
func TestHTMLLexRoundTrip(t *testing.T) {
	inputs := []string{
		"",
		"hello world",
		"<p>hello</p>",
		"<div><p>hello</p></div>",
		"<br/>",
		// Note: "<br />" does NOT round-trip because the space before /> is consumed.
		"<!-- hello -->",
		"<!DOCTYPE html>",
		"{{range .Items}}<li>{{.}}</li>{{end}}",
		"{{.X}}",
	}

	for _, input := range inputs {
		t.Run(truncate(input, 40), func(t *testing.T) {
			lt := lexTest{name: "roundtrip", input: input}
			items := collectHTML(&lt)

			var sb strings.Builder
			for _, item := range items {
				if item.Type == TokenTypeEOF || item.Type == TokenTypeError {
					break
				}
				sb.WriteString(item.Value)
			}

			if got := sb.String(); got != input {
				t.Errorf("round-trip failed:\n  got:  %q\n  want: %q", got, input)
			}
		})
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		if s == "" {
			return "(empty)"
		}
		return s
	}
	return s[:n] + "..."
}

func TestScanCommentRespectsHTMLReturnState(t *testing.T) {
	// When EmitComment is false and ParseHTML is true, a Go template comment
	// inside an HTML tag (e.g. in attribute position) must resume the tag-scanning
	// state, not fall back to lexHTMLText.
	l := New(`<div {{/* comment */}} id="x">`, Options{
		ParseHTML:   true,
		EmitComment: false,
		BreakOK:     true,
		ContinueOK:  true,
	})
	var items []Token
	for {
		item := l.Next()
		items = append(items, item)
		if item.Type == TokenTypeEOF || item.Type == TokenTypeError {
			break
		}
	}
	// After the comment is silently consumed, the lexer should still be inside the tag
	// and produce an attribute name token for "id".
	foundAttrName := false
	for _, item := range items {
		if item.Type == TokenTypeHTMLAttrName && item.Value == "id" {
			foundAttrName = true
			break
		}
	}
	if !foundAttrName {
		t.Errorf("expected TokenTypeHTMLAttrName with value \"id\", got tokens:\n\t%+v", items)
	}
}

func TestIsHTMLTagNameCharRejectsNonASCIIDigit(t *testing.T) {
	// U+0660 is Arabic-Indic digit zero. WHATWG HTML spec requires ASCII-only
	// digits in tag names.
	if isHTMLTagNameChar('\u0660') {
		t.Error("isHTMLTagNameChar accepted non-ASCII digit U+0660, want false")
	}
	// Verify ASCII digits are still accepted.
	for r := '0'; r <= '9'; r++ {
		if !isHTMLTagNameChar(r) {
			t.Errorf("isHTMLTagNameChar rejected ASCII digit %c", r)
		}
	}
}

func TestLexHTMLUnclosedEndTagIsText(t *testing.T) {
	// An end tag the input ends inside of is text, in raw text too, so the
	// lexer reports no error for it.
	for _, input := range []string{"</div", "<script></script", "<script></script x>"} {
		l := New(input, Options{
			ParseHTML:  true,
			BreakOK:    true,
			ContinueOK: true,
		})
		for {
			item := l.Next()
			if item.Type == TokenTypeError {
				t.Errorf("New(%q) lexed error %q", input, item.Value)
			}
			if item.Type == TokenTypeEOF || item.Type == TokenTypeError {
				break
			}
		}
	}
}
