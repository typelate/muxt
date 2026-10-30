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
//
// A sheet is Conditional unless some way to it is unconditional. A link is
// conditional in a template branch, in <template> or <noscript>, with a
// media attribute other than "" or "all", with rel alternate, or with
// disabled; an @import is conditional with a media query, supports() or
// layer after its URL, or in a conditional sheet or <style> element.
//
// Sheets are in first-reference order, a sheet before those it imports,
// which is not cascade order: ComputeWhitespace makes a tie between rules
// of different sheets uncertain (rank.mayBeat), so the order of sheets
// never decides a property.
func ResolveStylesheets(trees []*template.Tree, assets []StaticAssets) (sheets []Stylesheet, missing bool, err error) {
	r := stylesheetResolver{assets: assets, loaded: make(map[string]int)}
	for _, t := range trees {
		if t == nil || t.Root == nil {
			continue
		}
		template.WalkPath(t, func(_ *template.Tree, path []template.Node, n template.Node) {
			el, ok := n.(*template.HTMLElementNode)
			if !ok || r.err != nil {
				return
			}
			switch strings.ToLower(el.TagName) {
			case "link":
				r.link(el, optionalPlace(path))
			case "style":
				if len(el.Children) > 0 {
					r.imports(templateCSS(el.Children), "/", optionalPlace(path) || conditionalMedia(el))
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
	assets []StaticAssets
	// loaded maps each URL path read to its index in sheets, or -1 when
	// no file is there.
	loaded  map[string]int
	sheets  []Stylesheet
	missing bool
	err     error
}

// link loads the stylesheet el links to; conditional reports that el is
// somewhere the link may not apply.
func (r *stylesheetResolver) link(el *template.HTMLElementNode, conditional bool) {
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
			case strings.EqualFold(name, "disabled"):
				conditional = true
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
	conditional = conditional || hasToken(relText, "alternate") || conditionalMedia(el)
	r.load(ref, "/", conditional)
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
// imports. A sheet loaded before as conditional becomes unconditional
// when this way to it is, and so do the sheets it imports unconditionally.
func (r *stylesheetResolver) load(ref, base string, conditional bool) {
	urlPath, ok := resolveStylesheetURL(ref, base)
	if !ok {
		r.missing = true
		return
	}
	if i, ok := r.loaded[urlPath]; ok {
		if i >= 0 && r.sheets[i].Conditional && !conditional {
			r.sheets[i].Conditional = false
			r.imports(cssText{text: r.sheets[i].Text}, urlPath, false)
		}
		return
	}
	r.loaded[urlPath] = -1
	text, ok, err := r.read(urlPath)
	if err != nil {
		r.err = err
		return
	}
	if !ok {
		r.missing = true
		return
	}
	r.loaded[urlPath] = len(r.sheets)
	r.sheets = append(r.sheets, Stylesheet{Filename: urlPath, Text: text, Conditional: conditional})
	r.imports(cssText{text: text}, urlPath, conditional)
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

// imports loads the sheets css imports; base is the URL of its sheet, and
// conditional reports that the sheet may not apply.
func (r *stylesheetResolver) imports(css cssText, base string, conditional bool) {
	sheet, ok := parseStylesheet(css)
	if !ok {
		return // ComputeWhitespace makes a sheet that does not parse unknown
	}
	for item := sheet.Children.Head(); item != nil && r.err == nil; item = item.Next {
		rule, ok := item.Data.(*csstree.Atrule)
		if !ok || !strings.EqualFold(rule.Name, "import") {
			continue
		}
		target, condition := atImport(rule)
		if target == "" || (rule.Loc != nil && css.dynamic(rule.Loc.Start.Offset, rule.Loc.End.Offset)) {
			r.missing = true
			continue
		}
		r.load(target, base, conditional || isCondition(condition))
	}
}

// atImportTarget returns the URL an @import rule names, or "".
func atImportTarget(rule *csstree.Atrule) string {
	target, _ := atImport(rule)
	return target
}

// atImport returns the URL an @import rule names, or "", and the rest of
// its prelude: the layer, supports() and media query conditions.
func atImport(rule *csstree.Atrule) (target, condition string) {
	if rule.Prelude == nil {
		return "", ""
	}
	prelude, err := csstree.Generate(rule.Prelude, csstree.GenerateOptions{})
	if err != nil {
		return "", ""
	}
	target = importTarget(prelude)
	if target == "" {
		return "", ""
	}
	prelude = strings.TrimSpace(prelude)
	if strings.HasPrefix(strings.ToLower(prelude), "url(") {
		return target, prelude[strings.IndexByte(prelude, ')')+1:]
	}
	return target, prelude[1+len(target)+1:]
}

// isCondition reports whether condition, a media query list or the rest of
// an @import prelude, may not hold.
func isCondition(condition string) bool {
	condition = strings.TrimSpace(condition)
	return condition != "" && !strings.EqualFold(condition, "all")
}

// optionalPlace reports whether an element at path may not be in the
// document: it is in a template branch, or in <template> or <noscript>,
// whose content does not apply when scripting is on.
func optionalPlace(path []template.Node) bool {
	for _, n := range path {
		if isBranch(n) {
			return true
		}
		if el, ok := n.(*template.HTMLElementNode); ok && (strings.EqualFold(el.TagName, "template") || strings.EqualFold(el.TagName, "noscript")) {
			return true
		}
	}
	return false
}

// conditionalMedia reports whether el has a media attribute that may not
// match: one that is dynamic, or other than "" and "all".
func conditionalMedia(el *template.HTMLElementNode) bool {
	for _, n := range el.Attr {
		attr, ok := n.(*template.HTMLAttributeNode)
		if !ok || attr.Namespace != "" || !strings.EqualFold(staticAttrName(attr), "media") {
			continue
		}
		text, action, ok := attrValue(attr.Value)
		return !ok || action != nil || isCondition(text)
	}
	return false
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
