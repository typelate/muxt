package html

import (
	"reflect"

	"github.com/typelate/muxt/internal/template"
)

// Cached reflect.Type values for the html/template safe-content types.
// These are defined string types from the standard library, re-exported by
// content.go via type aliases — so reflect.TypeOf returns the same Type
// regardless of which alias the caller uses.
var (
	htmlType     = reflect.TypeOf(HTML(""))
	urlType      = reflect.TypeOf(URL(""))
	jsType       = reflect.TypeOf(JS(""))
	jsStrType    = reflect.TypeOf(JSStr(""))
	cssType      = reflect.TypeOf(CSS(""))
	htmlAttrType = reflect.TypeOf(HTMLAttr(""))
	srcsetType   = reflect.TypeOf(Srcset(""))
)

// trustOf classifies a value's Trust kind for the htt engine. It recognizes
// the standard html/template safe-content types so they bypass the matching
// contextual escaper, restoring the trust-type semantics promised by the
// html/template API.
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
	switch v.Type() {
	case htmlType:
		return template.TrustHTML
	case urlType:
		return template.TrustURL
	case jsType:
		return template.TrustJS
	case jsStrType:
		return template.TrustJSStr
	case cssType:
		return template.TrustCSS
	case htmlAttrType:
		return template.TrustHTMLAttr
	case srcsetType:
		return template.TrustSrcset
	}
	return template.TrustNone
}
