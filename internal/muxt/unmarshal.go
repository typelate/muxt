package muxt

import (
	"fmt"
	"go/types"
	"html/template"
	"reflect"
	"strings"

	"github.com/typelate/dom"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/typelate/muxt/internal/source"
)

// UnmarshalMethod identifies how a request value (path value, lastEventID
// header, or form field) parses from its string form.
type UnmarshalMethod int

const (
	UnmarshalUnsupported UnmarshalMethod = iota
	UnmarshalString
	UnmarshalBool
	UnmarshalInt
	UnmarshalInt8
	UnmarshalInt16
	UnmarshalInt32
	UnmarshalInt64
	UnmarshalUint
	UnmarshalUint8
	UnmarshalUint16
	UnmarshalUint32
	UnmarshalUint64
	UnmarshalFloat32
	UnmarshalFloat64
	UnmarshalTextUnmarshaler
)

var basicUnmarshalMethods = map[string]UnmarshalMethod{
	"string":  UnmarshalString,
	"bool":    UnmarshalBool,
	"int":     UnmarshalInt,
	"int8":    UnmarshalInt8,
	"int16":   UnmarshalInt16,
	"int32":   UnmarshalInt32,
	"int64":   UnmarshalInt64,
	"uint":    UnmarshalUint,
	"uint8":   UnmarshalUint8,
	"uint16":  UnmarshalUint16,
	"uint32":  UnmarshalUint32,
	"uint64":  UnmarshalUint64,
	"float32": UnmarshalFloat32,
	"float64": UnmarshalFloat64,
}

// unmarshalMethodFor classifies how tp parses from its string form: a basic
// type parsed with strconv (matched by name, so the byte and rune aliases are
// not supported), or a named type whose pointer implements
// encoding.TextUnmarshaler, which checker decides. An alias parses like the
// type it names.
func unmarshalMethodFor(checker Checker, tp types.Type) UnmarshalMethod {
	switch t := types.Unalias(tp).(type) {
	case *types.Basic:
		return basicUnmarshalMethods[t.Name()]
	case *types.Named:
		if checker.TextUnmarshaler(t) {
			return UnmarshalTextUnmarshaler
		}
	}
	return UnmarshalUnsupported
}

// supportedUnmarshalTypes names the types a path value or lastEventID
// parameter parses into, for error messages about everything else.
// Floats are excluded by design: they parse ambiguously and format
// lossily, which route paths cannot afford.
const supportedUnmarshalTypes = "string, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, or a type whose pointer implements encoding.TextUnmarshaler"

// supportedUnmarshalFieldTypes names the types a form or multipart
// struct field parses into: fields also accept floats.
const supportedUnmarshalFieldTypes = "float64, float32, " + supportedUnmarshalTypes

// unsupportedTypeError matches the error wording of the pre-hydration
// generator: unsupported basic types name the type directly, other
// types render in Go syntax. Both wordings list the supported set so
// the fix needs no doc lookup.
func unsupportedTypeError(tp types.Type, qual types.Qualifier, supported string) error {
	if _, ok := types.Unalias(tp).(*types.Basic); ok {
		return fmt.Errorf("method param type %s not supported (supported: %s; bind as string and parse it yourself for other values)", tp.String(), supported)
	}
	return fmt.Errorf("unsupported type: %s (supported: %s)", types.TypeString(tp, qual), supported)
}

// checkUnmarshalable reports whether tp parses from a form field's
// string value.
func checkUnmarshalable(checker Checker, tp types.Type, qual types.Qualifier) error {
	if unmarshalMethodFor(checker, tp) != UnmarshalUnsupported {
		return nil
	}
	return unsupportedTypeError(tp, qual, supportedUnmarshalFieldTypes)
}

// isStringAssignable reports whether a request string can be passed to a
// parameter of type tp without parsing it.
func isStringAssignable(tp types.Type) bool {
	return types.AssignableTo(types.Universe.Lookup("string").Type(), tp)
}

// bindParsedArgument validates a path value or lastEventID parameter: it
// either receives the raw string or parses from one. Floats are rejected
// here even though form fields accept them.
func bindParsedArgument(a *Argument, checker Checker, qual types.Qualifier) error {
	a.scopeType = types.Universe.Lookup("string").Type()
	if isStringAssignable(a.paramType) {
		a.direct = true
		return nil
	}
	a.method = unmarshalMethodFor(checker, a.paramType)
	switch a.method {
	case UnmarshalUnsupported, UnmarshalFloat32, UnmarshalFloat64:
		return unsupportedTypeError(a.paramType, qual, supportedUnmarshalTypes)
	default:
		a.textMarshaler = checker.TextMarshaler(a.paramType)
		return nil
	}
}

const (
	// InputAttributeNameStructTag renames the form input a struct field binds
	// to (e.g. `name:"count-input"`).
	InputAttributeNameStructTag = "name"
	// InputAttributeTemplateStructTag names the template whose input element
	// attributes (minlength, maxlength, ...) generate validations for the field.
	InputAttributeTemplateStructTag = "template"
)

// FieldBinding describes how one struct field of a form or multipart
// parameter binds to the request.
type FieldBinding struct {
	// Name is the bound struct field's name.
	Name string
	// InputName is the form input name: the name struct tag or the field name.
	InputName string
	// Template is the field's validation template (template struct tag), or
	// nil when the tag is absent or names an undefined template.
	Template *template.Template
	elem     types.Type
	// Slice binds every request value for InputName, not just the first.
	Slice bool
	// FileHeader binds the field from request.MultipartForm.File instead of a
	// text value: *multipart.FileHeader or (with Slice) []*multipart.FileHeader.
	FileHeader bool
	// Method is how Elem parses from a string. Undefined for FileHeader fields.
	Method UnmarshalMethod
	// Validations are the constraints parsed from the field's <input> element
	// in Template (the element whose name attribute equals InputName).
	Validations []InputValidation
}

// bindFormArgument permits a form or multipart parameter to either receive
// the raw request value (url.Values / *multipart.Form) or be a struct whose
// fields parse from the submitted form, recording one FieldBinding per struct
// field (none in raw mode). Struct fields must be a supported scalar or slice
// of scalars; multipart structs may also bind *multipart.FileHeader and
// []*multipart.FileHeader fields.
func bindFormArgument(a *Argument, def *Definition, checker Checker, qual types.Qualifier, allowFileFields bool) error {
	at, err := checker.ScopeType(a.Identifier)
	if err != nil {
		return err
	}
	a.scopeType = at
	if types.AssignableTo(at, a.paramType) {
		a.direct = true
		return nil
	}
	st, ok := a.paramType.Underlying().(*types.Struct)
	if !ok {
		return fmt.Errorf("expected %s parameter type to be a struct", a.Identifier)
	}
	bindings, err := formStructBindings(def, checker, st, a.Identifier, qual, allowFileFields)
	if err != nil {
		return err
	}
	a.formFields = bindings
	return nil
}

func formStructBindings(def *Definition, checker Checker, st *types.Struct, argName string, qual types.Qualifier, allowFileFields bool) ([]FieldBinding, error) {
	var fileHeaderPtr types.Type
	if allowFileFields {
		if fileHeader, err := checker.FileHeader(); err == nil {
			fileHeaderPtr = fileHeader
		}
	}
	bindings := make([]FieldBinding, 0, st.NumFields())
	for i := 0; i < st.NumFields(); i++ {
		fb, err := formFieldBinding(def, checker, st, i, argName, qual, fileHeaderPtr)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, fb)
	}
	return bindings, nil
}

// formFieldBinding binds field i of st. A nil fileHeaderPtr means file
// fields are not allowed.
func formFieldBinding(def *Definition, checker Checker, st *types.Struct, i int, argName string, qual types.Qualifier, fileHeaderPtr types.Type) (FieldBinding, error) {
	field, tags := st.Field(i), reflect.StructTag(st.Tag(i))
	fb := FieldBinding{
		Name:      field.Name(),
		InputName: field.Name(),
	}
	if name, found := tags.Lookup(InputAttributeNameStructTag); found {
		fb.InputName = name
	}
	ft := field.Type()
	if fileHeaderPtr != nil {
		if isSlice := types.Identical(ft, types.NewSlice(fileHeaderPtr)); isSlice || types.Identical(ft, fileHeaderPtr) {
			fb.FileHeader = true
			fb.Slice = isSlice
			return fb, nil
		}
	}
	if name, found := tags.Lookup(InputAttributeTemplateStructTag); found {
		fb.Template = def.template.Lookup(name)
	}
	fb.elem = ft
	if slice, ok := types.Unalias(ft).(*types.Slice); ok {
		fb.Slice = true
		fb.elem = slice.Elem()
	}
	validations, err := fieldTemplateValidations(fb)
	if err != nil {
		return FieldBinding{}, err
	}
	fb.Validations = validations
	if err := checkUnmarshalable(checker, fb.elem, qual); err != nil {
		return FieldBinding{}, fmt.Errorf("failed to generate parse statements for %s field %s: %w", argName, field.Name(), err)
	}
	fb.Method = unmarshalMethodFor(checker, fb.elem)
	return fb, nil
}

// fieldTemplateValidations parses the constraint attributes of the <input>
// or <textarea> element bound to fb in its field template: a textarea takes
// minlength and maxlength. Fields without a template tag, or whose template
// binds the name only to another element (a <select>, a <button>), have no
// validations.
func fieldTemplateValidations(fb FieldBinding) ([]InputValidation, error) {
	if fb.Template == nil || fb.Template.Tree == nil {
		return nil, nil
	}
	nodes, err := html.ParseFragment(strings.NewReader(fb.Template.Tree.Root.String()), &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Body,
		Data:     atom.Body.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("parsing template %s for field %s: %w", fb.Template.Name(), fb.Name, err)
	}
	element := dom.NewDocumentFragment(nodes).QuerySelector(fmt.Sprintf("input[name=%[1]q], textarea[name=%[1]q]", fb.InputName))
	if element == nil {
		return nil, nil
	}
	if strings.EqualFold(element.TagName(), atom.Textarea.String()) {
		return parseLengthValidations(fb.InputName, element)
	}
	return ParseInputValidations(fb.InputName, element, fb.elem)
}

// Elem is the type parsed from one string value: the field type, or the
// slice element type when Slice is set. Undefined for FileHeader fields.
func (fb FieldBinding) Elem() source.Type { return source.NewType(fb.elem) }
