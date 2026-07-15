package muxt

import (
	"fmt"
	"go/types"
	"html/template"
	"reflect"

	"golang.org/x/tools/go/packages"
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
	UnmarshalTextUnmarshaler
)

// UnmarshalMethodFor classifies how tp parses from its string form: a basic
// type parsed with strconv (matched by name, so the byte and rune aliases are
// not supported), or a named type whose pointer implements
// encoding.TextUnmarshaler. TextUnmarshaler detection requires the encoding
// package to be reachable in the load graph.
func UnmarshalMethodFor(pl []*packages.Package, tp types.Type) UnmarshalMethod {
	switch t := tp.(type) {
	case *types.Basic:
		switch t.Name() {
		case "string":
			return UnmarshalString
		case "bool":
			return UnmarshalBool
		case "int":
			return UnmarshalInt
		case "int8":
			return UnmarshalInt8
		case "int16":
			return UnmarshalInt16
		case "int32":
			return UnmarshalInt32
		case "int64":
			return UnmarshalInt64
		case "uint":
			return UnmarshalUint
		case "uint8":
			return UnmarshalUint8
		case "uint16":
			return UnmarshalUint16
		case "uint32":
			return UnmarshalUint32
		case "uint64":
			return UnmarshalUint64
		}
	case *types.Named:
		if encPkg, ok := findPackageTypes(pl, "encoding"); ok {
			textUnmarshaler := encPkg.Scope().Lookup("TextUnmarshaler").Type().Underlying().(*types.Interface)
			if types.Implements(types.NewPointer(t), textUnmarshaler) {
				return UnmarshalTextUnmarshaler
			}
		}
	}
	return UnmarshalUnsupported
}

// checkUnmarshalable reports whether tp parses from a string, matching the
// error wording of the pre-hydration generator: unsupported basic types name
// the type directly, other types render in Go syntax.
func checkUnmarshalable(pl []*packages.Package, tp types.Type, qual types.Qualifier) error {
	if UnmarshalMethodFor(pl, tp) != UnmarshalUnsupported {
		return nil
	}
	if _, ok := tp.(*types.Basic); ok {
		return fmt.Errorf("method param type %s not supported", tp.String())
	}
	return fmt.Errorf("unsupported type: %s", types.TypeString(tp, qual))
}

// checkParsedArgument validates a path value or lastEventID parameter: it
// either receives the raw string or parses from one.
func checkParsedArgument(pl []*packages.Package, paramType types.Type, qual types.Qualifier) error {
	if types.AssignableTo(types.Universe.Lookup("string").Type(), paramType) {
		return nil
	}
	return checkUnmarshalable(pl, paramType, qual)
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
	// Field is the bound struct field.
	Field *types.Var
	// InputName is the form input name: the name struct tag or the field name.
	InputName string
	// Template is the field's validation template (template struct tag), or
	// nil when the tag is absent or names an undefined template.
	Template *template.Template
	// Elem is the type parsed from one string value: the field type, or the
	// slice element type when Slice is set. Undefined for FileHeader fields.
	Elem types.Type
	// Slice binds every request value for InputName, not just the first.
	Slice bool
	// FileHeader binds the field from request.MultipartForm.File instead of a
	// text value: *multipart.FileHeader or (with Slice) []*multipart.FileHeader.
	FileHeader bool
	// Method is how Elem parses from a string. Undefined for FileHeader fields.
	Method UnmarshalMethod
}

// checkFormArgument permits a form or multipart parameter to either receive
// the raw request value (url.Values / *multipart.Form) or be a struct whose
// fields parse from the submitted form, returning one FieldBinding per struct
// field (nil in raw mode). Struct fields must be a supported scalar or slice
// of scalars; multipart structs may also bind *multipart.FileHeader and
// []*multipart.FileHeader fields.
func checkFormArgument(def *Definition, pl []*packages.Package, paramType types.Type, argName, packagePath, identifier string, pointer bool, qual types.Qualifier, allowFileFields bool) ([]FieldBinding, error) {
	at, err := stdlibType(pl, packagePath, identifier, pointer)
	if err != nil {
		return nil, err
	}
	if types.AssignableTo(at, paramType) {
		return nil, nil
	}
	st, ok := paramType.Underlying().(*types.Struct)
	if !ok {
		return nil, fmt.Errorf("expected %s parameter type to be a struct", argName)
	}
	return formStructBindings(def, pl, st, argName, qual, allowFileFields)
}

func formStructBindings(def *Definition, pl []*packages.Package, st *types.Struct, argName string, qual types.Qualifier, allowFileFields bool) ([]FieldBinding, error) {
	var fileHeaderPtr types.Type
	if allowFileFields {
		if mp, ok := findPackageTypes(pl, "mime/multipart"); ok {
			if obj := mp.Scope().Lookup("FileHeader"); obj != nil {
				fileHeaderPtr = types.NewPointer(obj.Type())
			}
		}
	}
	bindings := make([]FieldBinding, 0, st.NumFields())
	for i := 0; i < st.NumFields(); i++ {
		field, tags := st.Field(i), reflect.StructTag(st.Tag(i))
		fb := FieldBinding{
			Field:     field,
			InputName: field.Name(),
		}
		if name, found := tags.Lookup(InputAttributeNameStructTag); found {
			fb.InputName = name
		}
		ft := field.Type()
		if fileHeaderPtr != nil && (types.Identical(ft, fileHeaderPtr) || types.Identical(ft, types.NewSlice(fileHeaderPtr))) {
			fb.FileHeader = true
			fb.Slice = types.Identical(ft, types.NewSlice(fileHeaderPtr))
			bindings = append(bindings, fb)
			continue
		}
		if name, found := tags.Lookup(InputAttributeTemplateStructTag); found {
			fb.Template = def.template.Lookup(name)
		}
		fb.Elem = ft
		if slice, ok := ft.(*types.Slice); ok {
			fb.Slice = true
			fb.Elem = slice.Elem()
		}
		if err := checkUnmarshalable(pl, fb.Elem, qual); err != nil {
			return nil, fmt.Errorf("failed to generate parse statements for %s field %s: %w", argName, field.Name(), err)
		}
		fb.Method = UnmarshalMethodFor(pl, fb.Elem)
		bindings = append(bindings, fb)
	}
	return bindings, nil
}
