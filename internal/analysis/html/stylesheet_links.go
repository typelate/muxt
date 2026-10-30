package analysis

import (
	"errors"
	"fmt"
	"html"
	"io/fs"
	"net/url"
	"strings"

	"github.com/typelate/muxt/internal/css/csstree"
	"github.com/typelate/muxt/internal/template"
)

// StaticAssets is a URL prefix whose files are read from Files.
type StaticAssets struct {
	Prefix     string // "/static/"
	PathMethod string // "ReadStatic": {{.Path.ReadStatic}} and {{$.Path.ReadStatic}} print Prefix
	Files      fs.FS  // the name looked up is the URL path after Prefix
}

// ResolveStylesheets loads the stylesheets trees link to, and those the
// loaded sheets and <style> elements @import, through assets. missing
// reports a link or import no route resolves to a file. Each sheet's
// Filename is its URL path. err is only a read failure other than
// fs.ErrNotExist.
//
// A link is a link element whose static rel holds the token stylesheet.
// Its href is static text, or an action that is exactly .Path.<PathMethod>
// or $.Path.<PathMethod> followed by static text; the action prints that
// route's Prefix. An href resolves against "/", an @import against the URL
// of its sheet ("/" in a <style> element). The longest matching Prefix
// serves the URL. A URL with a scheme or host, one no route serves, and
// one whose file is missing, a directory, or has a name element starting
// with "." or "_" (go:embed leaves those out) are missing.
func ResolveStylesheets(trees []*template.Tree, assets []StaticAssets) (sheets []Stylesheet, missing bool, err error) {
	r := stylesheetResolver{assets: assets, loaded: make(map[string]bool)}
	for _, t := range trees {
		if t == nil || t.Root == nil {
			continue
		}
		template.WalkPath(t, func(_ *template.Tree, _ []template.Node, n template.Node) {
			el, ok := n.(*template.HTMLElementNode)
			if !ok || r.err != nil {
				return
			}
			switch strings.ToLower(el.TagName) {
			case "link":
				r.link(el)
			case "style":
				if len(el.Children) > 0 {
					r.imports(templateCSS(el.Children), "/")
				}
			}
		})
		if r.err != nil {
			return nil, false, r.err
		}
	}
	return r.sheets, r.missing, nil
}

type stylesheetResolver struct {
	assets  []StaticAssets
	loaded  map[string]bool
	sheets  []Stylesheet
	missing bool
	err     error
}

// link loads the stylesheet el links to.
func (r *stylesheetResolver) link(el *template.HTMLElementNode) {
	var rel, href *template.HTMLAttributeNode
	for _, n := range el.Attr {
		switch n := n.(type) {
		case *template.HTMLAttributeNode:
			if n.Namespace != "" {
				continue
			}
			name := staticAttrName(n)
			switch {
			case name == "":
				r.missing = true // a dynamic name may be rel or href
				return
			case strings.EqualFold(name, "rel") && rel == nil:
				rel = n
			case strings.EqualFold(name, "href") && href == nil:
				href = n
			}
		case *template.CommentNode:
		default:
			r.missing = true // an action may add a rel or an href
			return
		}
	}
	if rel == nil {
		return
	}
	relText, action, ok := attrValue(rel.Value)
	if !ok || action != nil {
		r.missing = true
		return
	}
	if !hasToken(relText, "stylesheet") || href == nil {
		return
	}
	ref, ok := r.href(href.Value)
	if !ok {
		r.missing = true
		return
	}
	if ref == "" {
		return
	}
	r.load(ref, "/")
}

// href returns the URL reference an href value names.
func (r *stylesheetResolver) href(value []template.Node) (string, bool) {
	text, action, ok := attrValue(value)
	if !ok {
		return "", false
	}
	text = strings.TrimSpace(text)
	if action == nil {
		return text, true
	}
	assets, ok := r.pathMethod(action)
	if !ok {
		return "", false
	}
	return assets.Prefix + text, true
}

// pathMethod returns the route whose path method action calls.
func (r *stylesheetResolver) pathMethod(action *template.ActionNode) (StaticAssets, bool) {
	pipe := action.Pipe
	if pipe == nil || len(pipe.Decl) > 0 || len(pipe.Commands) != 1 || len(pipe.Commands[0].Args) != 1 {
		return StaticAssets{}, false
	}
	var ident []string
	switch arg := pipe.Commands[0].Args[0].(type) {
	case *template.FieldNode:
		ident = arg.Ident
	case *template.VariableNode:
		if len(arg.Ident) == 0 || arg.Ident[0] != "$" {
			return StaticAssets{}, false
		}
		ident = arg.Ident[1:]
	default:
		return StaticAssets{}, false
	}
	if len(ident) != 2 || ident[0] != "Path" {
		return StaticAssets{}, false
	}
	for _, assets := range r.assets {
		if assets.PathMethod != "" && assets.PathMethod == ident[1] {
			return assets, true
		}
	}
	return StaticAssets{}, false
}

// load reads the sheet ref names, relative to base, and the sheets it
// imports.
func (r *stylesheetResolver) load(ref, base string) {
	urlPath, ok := resolveStylesheetURL(ref, base)
	if !ok {
		r.missing = true
		return
	}
	if r.loaded[urlPath] {
		return
	}
	r.loaded[urlPath] = true
	text, ok, err := r.read(urlPath)
	if err != nil {
		r.err = err
		return
	}
	if !ok {
		r.missing = true
		return
	}
	r.sheets = append(r.sheets, Stylesheet{Filename: urlPath, Text: text})
	r.imports(cssText{text: text}, urlPath)
}

// read returns the file the route with the longest matching prefix serves
// at urlPath; ok is false when it serves none.
func (r *stylesheetResolver) read(urlPath string) (text string, ok bool, err error) {
	var route *StaticAssets
	for i, assets := range r.assets {
		if strings.HasPrefix(urlPath, assets.Prefix) && (route == nil || len(assets.Prefix) > len(route.Prefix)) {
			route = &r.assets[i]
		}
	}
	if route == nil {
		return "", false, nil
	}
	name := strings.TrimPrefix(urlPath, route.Prefix)
	if name == "" || !fs.ValidPath(name) {
		return "", false, nil
	}
	for element := range strings.SplitSeq(name, "/") {
		if strings.HasPrefix(element, ".") || strings.HasPrefix(element, "_") {
			return "", false, nil
		}
	}
	info, err := fs.Stat(route.Files, name)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("reading stylesheet %s: %w", urlPath, err)
	}
	if info.IsDir() {
		return "", false, nil
	}
	data, err := fs.ReadFile(route.Files, name)
	if err != nil {
		return "", false, fmt.Errorf("reading stylesheet %s: %w", urlPath, err)
	}
	return string(data), true, nil
}

// imports loads the sheets css imports; base is the URL of its sheet.
func (r *stylesheetResolver) imports(css cssText, base string) {
	sheet, ok := parseStylesheet(css)
	if !ok {
		return // ComputeWhitespace makes a sheet that does not parse unknown
	}
	for item := sheet.Children.Head(); item != nil && r.err == nil; item = item.Next {
		rule, ok := item.Data.(*csstree.Atrule)
		if !ok || !strings.EqualFold(rule.Name, "import") {
			continue
		}
		target := atImportTarget(rule)
		if target == "" || (rule.Loc != nil && css.dynamic(rule.Loc.Start.Offset, rule.Loc.End.Offset)) {
			r.missing = true
			continue
		}
		r.load(target, base)
	}
}

// atImportTarget returns the URL an @import rule names, or "".
func atImportTarget(rule *csstree.Atrule) string {
	if rule.Prelude == nil {
		return ""
	}
	prelude, err := csstree.Generate(rule.Prelude, csstree.GenerateOptions{})
	if err != nil {
		return ""
	}
	return importTarget(prelude)
}

// resolveStylesheetURL returns the URL path ref names relative to the URL
// path base. A reference with a scheme or a host is not served by the
// package.
func resolveStylesheetURL(ref, base string) (string, bool) {
	u, err := url.Parse(ref)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Opaque != "" {
		return "", false
	}
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	return (&url.URL{Path: base}).ResolveReference(u).Path, true
}

// attrValue returns the text of an attribute value without its quotes and
// with character references decoded, and the action that leads it, if any.
// ok is false when the value holds another action.
func attrValue(value []template.Node) (text string, action *template.ActionNode, ok bool) {
	const actionMark = "\x00"
	var sb strings.Builder
	for _, n := range value {
		switch n := n.(type) {
		case *template.TextNode:
			sb.Write(n.Text)
		case *template.ActionNode:
			if action != nil {
				return "", nil, false
			}
			action = n
			sb.WriteString(actionMark)
		default:
			return "", nil, false
		}
	}
	raw := unquoteAttrValue([]byte(sb.String()))
	if action != nil {
		rest, leads := strings.CutPrefix(raw, actionMark)
		if !leads {
			return "", nil, false
		}
		raw = rest
	}
	return html.UnescapeString(raw), action, true
}

// hasToken reports whether the space-separated list s holds token, ASCII
// case-insensitively.
func hasToken(s, token string) bool {
	for _, t := range strings.FieldsFunc(s, isASCIIWhitespace) {
		if strings.EqualFold(t, token) {
			return true
		}
	}
	return false
}

func isASCIIWhitespace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\f', '\r':
		return true
	}
	return false
}
