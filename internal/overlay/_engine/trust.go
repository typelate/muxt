package template

import (
	"reflect"

	"html/template/internal/engine"
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
func trustOf(v reflect.Value) engine.Trust {
	if !v.IsValid() {
		return engine.TrustNone
	}
	for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return engine.TrustNone
		}
		v = v.Elem()
	}
	switch v.Type() {
	case htmlType:
		return engine.TrustHTML
	case urlType:
		return engine.TrustURL
	case jsType:
		return engine.TrustJS
	case jsStrType:
		return engine.TrustJSStr
	case cssType:
		return engine.TrustCSS
	case htmlAttrType:
		return engine.TrustHTMLAttr
	case srcsetType:
		return engine.TrustSrcset
	}
	return engine.TrustNone
}
