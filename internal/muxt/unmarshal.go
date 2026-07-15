package muxt

import (
	"fmt"
	"go/types"

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

// checkFormArgument permits a form or multipart parameter to either receive
// the raw request value (url.Values / *multipart.Form) or be a struct whose
// fields parse from the submitted form. Struct fields must be a supported
// scalar or slice of scalars; multipart structs may also bind
// *multipart.FileHeader and []*multipart.FileHeader fields.
func checkFormArgument(pl []*packages.Package, paramType types.Type, argName, packagePath, identifier string, pointer bool, qual types.Qualifier, allowFileFields bool) error {
	at, err := stdlibType(pl, packagePath, identifier, pointer)
	if err != nil {
		return err
	}
	if types.AssignableTo(at, paramType) {
		return nil
	}
	st, ok := paramType.Underlying().(*types.Struct)
	if !ok {
		return fmt.Errorf("expected %s parameter type to be a struct", argName)
	}
	return checkFormStructFields(pl, st, argName, qual, allowFileFields)
}

func checkFormStructFields(pl []*packages.Package, st *types.Struct, argName string, qual types.Qualifier, allowFileFields bool) error {
	var fileHeaderPtr types.Type
	if allowFileFields {
		if mp, ok := findPackageTypes(pl, "mime/multipart"); ok {
			if obj := mp.Scope().Lookup("FileHeader"); obj != nil {
				fileHeaderPtr = types.NewPointer(obj.Type())
			}
		}
	}
	for i := 0; i < st.NumFields(); i++ {
		field := st.Field(i)
		ft := field.Type()
		if fileHeaderPtr != nil && (types.Identical(ft, fileHeaderPtr) || types.Identical(ft, types.NewSlice(fileHeaderPtr))) {
			continue
		}
		elem := ft
		if slice, ok := ft.(*types.Slice); ok {
			elem = slice.Elem()
		}
		if err := checkUnmarshalable(pl, elem, qual); err != nil {
			return fmt.Errorf("failed to generate parse statements for %s field %s: %w", argName, field.Name(), err)
		}
	}
	return nil
}
