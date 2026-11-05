package html

import (
	"reflect"

	"github.com/typelate/muxt/template"
)

// contentTrust is the Trust of each safe-content type, by name. The names
// are the same in this package and in the standard library's
// html/template.
var contentTrust = map[string]template.Trust{
	"HTML":     template.TrustHTML,
	"URL":      template.TrustURL,
	"JS":       template.TrustJS,
	"JSStr":    template.TrustJSStr,
	"CSS":      template.TrustCSS,
	"HTMLAttr": template.TrustHTMLAttr,
	"Srcset":   template.TrustSrcset,
}

// pkgPath is this package's import path, as its types report it.
var pkgPath = reflect.TypeFor[HTML]().PkgPath()

// stdlibPkgPath is the standard library's html/template. Code running
// this package in process may pass its content types, which are distinct
// from this package's; they are recognized by name, since importing
// html/template here would make it import itself where muxt lays this
// package over it.
const stdlibPkgPath = "html/template"

// trustOf classifies a value's Trust kind for the htt engine. It recognizes
// this package's safe-content types and the standard library's
// html/template ones so they bypass the matching contextual escaper,
// restoring the trust-type semantics promised by the html/template API.
func trustOf(v reflect.Value) template.Trust {
	if !v.IsValid() {
		return template.TrustNone
	}
	for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return template.TrustNone
		}
		v = v.Elem()
	}
	t := v.Type()
	if t.Kind() != reflect.String || (t.PkgPath() != pkgPath && t.PkgPath() != stdlibPkgPath) {
		return template.TrustNone
	}
	return contentTrust[t.Name()] // TrustNone for any other name
}
