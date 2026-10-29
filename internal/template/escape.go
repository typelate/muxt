package template

import (
	_ "embed"
	"encoding/json"
	"reflect"
	"strings"
	"unicode/utf8"
)

// Trust classifies values that should bypass contextual escaping in
// HTML-mode execution. The set mirrors the safe-content types from
// the standard html/template package.
type Trust int

const (
	// TrustNone means the value gets default contextual escaping.
	TrustNone Trust = iota
	// TrustHTML marks a value as a known-safe HTML fragment. In body
	// context (text, not script/style), the value is rendered raw.
	TrustHTML
	// TrustURL marks a value as a known-safe URL. In URL attributes,
	// the safe-scheme filter is skipped.
	TrustURL
	// TrustJS marks a value as a known-safe JavaScript expression. In
	// script body context, JS-string escaping is skipped.
	TrustJS
	// TrustJSStr marks a value as known-safe content for inside a
	// quoted JavaScript string literal. JS-string escaping is skipped
	// in script string context.
	TrustJSStr
	// TrustCSS marks a value as known-safe CSS. In style body context,
	// CSS escaping is skipped.
	TrustCSS
	// TrustHTMLAttr marks a value as a known-safe HTML attribute value.
	// URL filtering and JS-event escaping are skipped.
	TrustHTMLAttr
	// TrustSrcset marks a value as a known-safe srcset attribute value.
	// Per-URL safe-scheme filtering is skipped.
	TrustSrcset
)

// TrustFunc inspects a runtime value and reports its trust classification.
// A nil TrustFunc behaves like one that returns TrustNone.
type TrustFunc func(reflect.Value) Trust

// escapeAction applies context-appropriate escaping to a string based on the
// HTMLContext annotation on the ActionNode and the optional Trust kind.
//
// It returns the escaped text and a "raw" flag. When raw is true, the result
// must be emitted as html.RawNode so the renderer does not HTML-escape it
// again — used for trusted HTML/JS/CSS in body or script/style position.
func escapeAction(s string, ctx *HTMLContext, trust Trust) (string, bool) {
	if ctx == nil {
		return s, false
	}
	switch ctx.State {
	case "attr":
		return escapeAttrValue(s, ctx, trust), false
	case "comment":
		return "", false
	default: // "text"
		switch ctx.Element {
		case "script":
			return escapeScript(s, ctx, trust), true
		case "style":
			if trust == TrustCSS {
				return s, true
			}
			return escapeCSSText(s), true
		default:
			if trust == TrustHTML {
				return s, true
			}
			// html.Render handles escaping for text nodes.
			return s, false
		}
	}
}

// escapeScript escapes a value interpolated into <script> content.
// The HTMLContext.JSContext field tells us whether the action sits inside
// a JS string/template literal (apply jsStrEscape) or a bare expression
// (apply jsValEscape, which JSON-quotes the value).
func escapeScript(s string, ctx *HTMLContext, trust Trust) string {
	if trust == TrustJS {
		return s
	}
	switch ctx.JSContext {
	case "tpl":
		// A template literal is a string, but `${` opens an expression.
		// TrustJSStr is not honored here: it asserts the value is safe
		// between quotes, which says nothing about interpolation.
		return escapeJSTmplLit(s)
	case "str":
		if trust == TrustJSStr {
			return s
		}
		return escapeJS(s)
	default:
		// Expression context: wrap in quotes and JS-escape so the
		// interpolated value is always a JS string literal. This
		// prevents JS injection from non-string template data.
		if trust == TrustJSStr {
			return "\"" + s + "\""
		}
		return jsValEscape(s)
	}
}

// escapeAttrValue applies pre-rendering sanitization for HTML attribute values.
// TrustHTMLAttr passes through verbatim (still HTML-attr-escaped by the
// renderer). The other trust kinds are scoped to specific attribute
// classes — TrustURL bypasses URL filtering, TrustSrcset bypasses srcset
// filtering, TrustJS/TrustJSStr bypass JS-event JS escape but the value is
// still wrapped so it forms a single quoted JS string in the resulting
// inline handler.
func escapeAttrValue(s string, ctx *HTMLContext, trust Trust) string {
	if trust == TrustHTMLAttr {
		return s
	}
	switch {
	case isSrcsetAttr(ctx.Attr):
		if trust == TrustSrcset {
			return s
		}
		return filterSrcset(s)
	case isURLAttr(ctx.Attr):
		if trust == TrustURL {
			return s
		}
		return filterURL(s)
	case isJSEventAttr(ctx.Attr):
		if trust == TrustJS || trust == TrustJSStr {
			return "\"" + s + "\""
		}
		return jsValEscape(s)
	default:
		return s
	}
}

// escapeCSSText escapes a value interpolated into <style> content using a
// CSS-aware replacement table so values cannot break out of declarations,
// rules, comments, or strings.
func escapeCSSText(s string) string {
	return jsReplace(s, cssReplacementTable)
}

// escapeJS applies JS string escaping using the same replacement table approach
// as html/template's jsStrEscaper.
func escapeJS(s string) string {
	return jsReplace(s, jsStrReplacementTable)
}

// escapeJSTmplLit applies JS escaping for content interpolated into a
// backtick template literal. It matches html/template's jsTmplLitEscaper.
func escapeJSTmplLit(s string) string {
	return jsReplace(s, jsTmplLitReplacementTable)
}

// jsValEscape matches html/template's jsValEscaper for string values:
// the content is JS-escaped and wrapped in double quotes.
func jsValEscape(s string) string {
	return "\"" + escapeJS(s) + "\""
}

// jsExprJSONMarshal renders v (a non-string action result landing directly
// in a <script> expression position, e.g. {{.Result}} where Result is a
// struct or slice) as a JSON literal, matching html/template's
// jsValEscaper: any value there is json.Marshal-ed, not just quoted as a
// string, so a struct produces a JS object literal instead of Go's %v
// syntax wrapped in quotes.
//
// encoding/json's Marshal already HTML-escapes '<', '>', and '&' to
// </>/& by default, which is what keeps a marshaled
// "</script>" from being able to close the surrounding script element —
// the same property html/template's jsValEscaper relies on. Reports false
// (falling back to the caller's normal string path) when v is nil or
// itself a string/[]byte: those already render correctly through the
// existing jsValEscape path, and routing them here as well is unnecessary
// surface for no behavior change.
func jsExprJSONMarshal(v any) (string, bool) {
	if v == nil {
		return "", false
	}
	switch v.(type) {
	case string, []byte:
		return "", false
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.String:
		return "", false
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return "", false
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", false
	}
	return string(b), true
}

// jsReplace replaces each rune r of s with replacementTable[r], provided that
// r < len(replacementTable). If replacementTable[r] is the empty string then
// no replacement is made. It also replaces runes U+2028 and U+2029.
func jsReplace(s string, replacementTable []string) string {
	var b strings.Builder
	r, w, written := rune(0), 0, 0
	for i := 0; i < len(s); i += w {
		r, w = utf8.DecodeRuneInString(s[i:])
		var repl string
		switch {
		case int(r) < len(lowUnicodeReplacementTable) && lowUnicodeReplacementTable[r] != "":
			repl = lowUnicodeReplacementTable[r]
		case int(r) < len(replacementTable) && replacementTable[r] != "":
			repl = replacementTable[r]
		case r == ' ':
			repl = "\\u2028"
		case r == ' ':
			repl = "\\u2029"
		default:
			continue
		}
		if written == 0 {
			b.Grow(len(s))
		}
		b.WriteString(s[written:i])
		b.WriteString(repl)
		written = i + w
	}
	if written == 0 {
		return s
	}
	b.WriteString(s[written:])
	return b.String()
}

// lowUnicodeReplacementTable contains replacements for all control characters
// (U+0000 through U+001F). Matches html/template's lowUnicodeReplacementTable.
var lowUnicodeReplacementTable = []string{
	0:    "\\u0000",
	0x01: "\\u0001",
	0x02: "\\u0002",
	0x03: "\\u0003",
	0x04: "\\u0004",
	0x05: "\\u0005",
	0x06: "\\u0006",
	'\a': "\\u0007",
	'\b': "\\u0008",
	'\t': "\\t",
	'\n': "\\n",
	'\v': "\\u000b", // "\v" == "v" on IE 6.
	'\f': "\\f",
	'\r': "\\r",
	0x0e: "\\u000e",
	0x0f: "\\u000f",
	0x10: "\\u0010",
	0x11: "\\u0011",
	0x12: "\\u0012",
	0x13: "\\u0013",
	0x14: "\\u0014",
	0x15: "\\u0015",
	0x16: "\\u0016",
	0x17: "\\u0017",
	0x18: "\\u0018",
	0x19: "\\u0019",
	0x1a: "\\u001a",
	0x1b: "\\u001b",
	0x1c: "\\u001c",
	0x1d: "\\u001d",
	0x1e: "\\u001e",
	0x1f: "\\u001f",
}

// jsStrReplacementTable contains replacements for JS string special characters.
// Matches html/template's jsStrReplacementTable.
var jsStrReplacementTable = []string{
	0:    "\\u0000",
	'\t': "\\t",
	'\n': "\\n",
	'\v': "\\u000b",
	'\f': "\\f",
	'\r': "\\r",
	// Encode HTML specials as hex so the output can be embedded
	// in HTML attributes without further encoding.
	'"':  "\\u0022",
	'`':  "\\u0060",
	'&':  "\\u0026",
	'\'': "\\u0027",
	'+':  "\\u002b",
	'/':  "\\/",
	'<':  "\\u003c",
	'>':  "\\u003e",
	'\\': "\\\\",
}

// jsTmplLitReplacementTable is jsStrReplacementTable extended with the
// characters that are special inside a backtick template literal: `$` and
// `{` (which together open an interpolation) and `}` (which closes one).
// Matches html/template's jsBqStrReplacementTable.
var jsTmplLitReplacementTable = []string{
	0:    "\\u0000",
	'\t': "\\t",
	'\n': "\\n",
	'\v': "\\u000b",
	'\f': "\\f",
	'\r': "\\r",
	// Encode HTML specials as hex so the output can be embedded
	// in HTML attributes without further encoding.
	'"':  "\\u0022",
	'`':  "\\u0060",
	'&':  "\\u0026",
	'\'': "\\u0027",
	'+':  "\\u002b",
	'/':  "\\/",
	'<':  "\\u003c",
	'>':  "\\u003e",
	'\\': "\\\\",
	'$':  "\\u0024",
	'{':  "\\u007b",
	'}':  "\\u007d",
}

// cssReplacementTable contains replacements for characters that are special
// in CSS. Anything that could let an interpolated value break out of a CSS
// declaration, rule, comment, or string is hex-escaped. CSS hex escapes
// terminate at the first non-hex character so each replacement is followed
// by a space.
var cssReplacementTable = []string{
	0:    "\\0 ",
	'\t': "\\9 ",
	'\n': "\\a ",
	'\v': "\\b ",
	'\f': "\\c ",
	'\r': "\\d ",
	'"':  "\\22 ",
	'\'': "\\27 ",
	'(':  "\\28 ",
	')':  "\\29 ",
	'*':  "\\2a ",
	'+':  "\\2b ",
	',':  "\\2c ",
	'/':  "\\2f ",
	':':  "\\3a ",
	';':  "\\3b ",
	'<':  "\\3c ",
	'=':  "\\3d ",
	'>':  "\\3e ",
	'@':  "\\40 ",
	'\\': "\\\\",
	'{':  "\\7b ",
	'}':  "\\7d ",
	'`':  "\\60 ",
	'&':  "\\26 ",
}

// filterURL checks a URL for safe schemes. If the URL has a dangerous scheme
// (like javascript:), it is replaced with "#ZgotmplZ" (matching html/template).
func filterURL(s string) string {
	s = strings.TrimSpace(s)
	if isSafeURL(s) {
		return s
	}
	return "#ZgotmplZ"
}

// filterSrcset filters a srcset attribute value. Each comma-separated entry is
// of the form "url" or "url descriptor"; the URL portion is passed through
// filterURL and unsafe entries are replaced with "#ZgotmplZ".
func filterSrcset(s string) string {
	parts := strings.Split(s, ",")
	for i, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		urlPart, descriptor, hasDesc := strings.Cut(trimmed, " ")
		filtered := filterURL(urlPart)
		if hasDesc {
			parts[i] = filtered + " " + strings.TrimSpace(descriptor)
		} else {
			parts[i] = filtered
		}
	}
	return strings.Join(parts, ", ")
}

// isSafeURL reports whether s is a relative URL or has a safe protocol scheme.
// Only http, https, and mailto are considered safe, matching html/template.
func isSafeURL(s string) bool {
	if protocol, _, ok := strings.Cut(s, ":"); ok && !strings.Contains(protocol, "/") {
		if !strings.EqualFold(protocol, "http") && !strings.EqualFold(protocol, "https") && !strings.EqualFold(protocol, "mailto") {
			return false
		}
	}
	return true
}

// isURLAttr reports whether an attribute typically contains a URL.
func isURLAttr(attr string) bool {
	switch strings.ToLower(attr) {
	case "href", "src", "action", "formaction", "cite", "data", "poster",
		"codebase", "background", "longdesc", "usemap", "manifest", "icon",
		"ping", "xlink:href":
		return true
	}
	return false
}

// isSrcsetAttr reports whether an attribute is a srcset attribute that
// contains a comma-separated list of URLs.
func isSrcsetAttr(attr string) bool {
	switch strings.ToLower(attr) {
	case "srcset", "imagesrcset":
		return true
	}
	return false
}

// isJSEventAttr reports whether an attribute is a JavaScript event handler.
// The set is the union of html/template's attrTypeMap event handlers and
// common modern events. Attributes that merely start with "on" but are not
// real event handlers (such as "only") are not treated as JS context.
func isJSEventAttr(attr string) bool {
	_, ok := jsEventAttrs[strings.ToLower(attr)]
	return ok
}

//go:embed js_event_attrs.json
var jsEventAttrsJSON []byte

// jsEventAttrs is loaded from js_event_attrs.json so the canonical list
// can be edited without touching Go source.
var jsEventAttrs = loadJSEventAttrs()

func loadJSEventAttrs() map[string]struct{} {
	var names []string
	if err := json.Unmarshal(jsEventAttrsJSON, &names); err != nil {
		panic("htt: js_event_attrs.json is malformed: " + err.Error())
	}
	out := make(map[string]struct{}, len(names))
	for _, n := range names {
		out[strings.ToLower(n)] = struct{}{}
	}
	return out
}

// EscapeJS is the exported form of escapeJS for testing.
func EscapeJS(s string) string {
	return escapeJS(s)
}

// EscapeCSS is the exported form of escapeCSSText for testing.
func EscapeCSS(s string) string {
	return escapeCSSText(s)
}

// FilterURL is the exported form of filterURL for testing.
func FilterURL(s string) string {
	return filterURL(s)
}

// IsURLAttr reports whether the named attribute typically holds a URL,
// meaning an interpolated value is filtered for dangerous schemes. It
// is the exported form of the classifier the escaper uses, so tooling
// (such as the LSP) can describe escaping without drifting from it.
func IsURLAttr(attr string) bool {
	return isURLAttr(attr)
}

// IsJSEventAttr reports whether the named attribute is a JavaScript
// event handler, meaning an interpolated value is JS-escaped. It is
// the exported form of the classifier the escaper uses.
func IsJSEventAttr(attr string) bool {
	return isJSEventAttr(attr)
}

// FilterSrcset is the exported form of filterSrcset for testing.
func FilterSrcset(s string) string {
	return filterSrcset(s)
}
