package template

import (
	"io"
	"strings"

	"golang.org/x/net/html"
)

// render writes the HTML representation of the node tree to w.
// It matches html/template's output format:
//   - Void elements use <br> not <br/>
//   - Attributes with empty values are rendered bare (e.g., disabled not disabled="")
//   - Text content and attribute values are properly escaped
func render(w io.Writer, n *html.Node) error {
	return renderNode(w, n)
}

func renderNode(w io.Writer, n *html.Node) error {
	switch n.Type {
	case html.DocumentNode:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if err := renderNode(w, c); err != nil {
				return err
			}
		}
		return nil

	case html.TextNode:
		_, err := io.WriteString(w, htmlEscapeString(n.Data))
		return err

	case html.ElementNode:
		return renderElement(w, n)

	case html.CommentNode:
		if _, err := io.WriteString(w, "<!--"); err != nil {
			return err
		}
		if _, err := io.WriteString(w, n.Data); err != nil {
			return err
		}
		_, err := io.WriteString(w, "-->")
		return err

	case html.DoctypeNode:
		if _, err := io.WriteString(w, "<!DOCTYPE "); err != nil {
			return err
		}
		if _, err := io.WriteString(w, n.Data); err != nil {
			return err
		}
		_, err := io.WriteString(w, ">")
		return err

	case html.RawNode:
		_, err := io.WriteString(w, n.Data)
		return err

	default:
		return nil
	}
}

func renderElement(w io.Writer, n *html.Node) error {
	if _, err := io.WriteString(w, "<"); err != nil {
		return err
	}
	if _, err := io.WriteString(w, n.Data); err != nil {
		return err
	}
	for _, a := range n.Attr {
		if err := renderAttr(w, a); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, ">"); err != nil {
		return err
	}

	if isVoidElement(n.Data) {
		return nil
	}

	rawText := isRawTextElement(n.Data)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if rawText && c.Type == html.TextNode {
			if _, err := io.WriteString(w, c.Data); err != nil {
				return err
			}
			continue
		}
		if err := renderNode(w, c); err != nil {
			return err
		}
	}

	if _, err := io.WriteString(w, "</"); err != nil {
		return err
	}
	if _, err := io.WriteString(w, n.Data); err != nil {
		return err
	}
	_, err := io.WriteString(w, ">")
	return err
}

func renderAttr(w io.Writer, a html.Attribute) error {
	if _, err := io.WriteString(w, " "); err != nil {
		return err
	}
	if a.Namespace != "" {
		if _, err := io.WriteString(w, a.Namespace); err != nil {
			return err
		}
		if _, err := io.WriteString(w, ":"); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, a.Key); err != nil {
		return err
	}
	if a.Val == "" {
		return nil
	}
	if _, err := io.WriteString(w, `="`); err != nil {
		return err
	}
	if _, err := io.WriteString(w, htmlEscapeAttrVal(a.Val)); err != nil {
		return err
	}
	_, err := io.WriteString(w, `"`)
	return err
}

// htmlEscaper escapes the five HTML special characters plus \r,
// matching x/net/html's escape function used by html.Render.
var renderTextEscaper = strings.NewReplacer(
	`&`, "&amp;",
	`'`, "&#39;",
	`<`, "&lt;",
	`>`, "&gt;",
	`"`, "&#34;",
	"\r", "&#13;",
)

// htmlEscapeString escapes special HTML characters in text content.
func htmlEscapeString(s string) string {
	return renderTextEscaper.Replace(s)
}

// attrEscaper escapes HTML special characters in attribute values,
// including newlines which must be escaped in attribute contexts.
// Single quotes are NOT escaped because we always use double-quoted
// attributes, and this matches html/template's output for static
// attribute content.
var attrEscaper = strings.NewReplacer(
	`&`, "&amp;",
	`<`, "&lt;",
	`>`, "&gt;",
	`"`, "&#34;",
	"\n", "&#10;",
	"\r", "&#13;",
)

// htmlEscapeAttrVal escapes special characters in attribute values.
func htmlEscapeAttrVal(s string) string {
	return attrEscaper.Replace(s)
}

// isVoidElement reports whether the named element is a void element
// (one that cannot have child nodes).
func isVoidElement(tag string) bool {
	switch tag {
	case "area", "base", "br", "col", "embed", "hr", "img", "input",
		"link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}

// IsVoidElement reports whether the named element is a void element
// (no closing tag). Exported for tooling such as the LSP.
func IsVoidElement(tag string) bool {
	return isVoidElement(tag)
}

// IsRawTextElement reports whether the named element holds raw text
// whose contents are not HTML-escaped. Exported for tooling such as
// the LSP.
func IsRawTextElement(tag string) bool {
	return isRawTextElement(tag)
}

// isRawTextElement reports whether the named element holds raw text content
// (no HTML entity expansion). Children of these elements must be emitted
// verbatim — HTML-escaping their text would corrupt the script or style
// payload, since the browser does not entity-decode raw-text elements.
func isRawTextElement(tag string) bool {
	switch tag {
	case "script", "style":
		return true
	}
	return false
}
