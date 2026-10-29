package template

import (
	"fmt"
	"reflect"
	"strings"
)

// builtins returns the default set of built-in functions.
func builtins() FuncMap {
	return FuncMap{
		"and":      and,
		"call":     call,
		"html":     htmlEscaper,
		"index":    index,
		"slice":    slice,
		"js":       jsEscaper,
		"len":      length,
		"not":      not,
		"or":       or,
		"print":    fmt.Sprint,
		"printf":   fmt.Sprintf,
		"println":  fmt.Sprintln,
		"urlquery": urlQueryEscaper,

		// Comparisons
		"eq": eq,
		"ge": ge,
		"gt": gt,
		"le": le,
		"lt": lt,
		"ne": ne,
	}
}

// IsTrue reports whether the value is 'true', in the sense of not the zero of its type,
// and whether the value is valid. It is for use by packages that wrap htt.
func IsTrue(val any) (truth, ok bool) {
	return isTrue(reflect.ValueOf(val))
}

// isTrue reports whether the value is 'true', in the sense of not the zero of its type,
// and whether the value is valid.
func isTrue(val reflect.Value) (truth, ok bool) {
	if !val.IsValid() {
		return false, true
	}
	switch val.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		truth = val.Len() > 0
	case reflect.Bool:
		truth = val.Bool()
	case reflect.Complex64, reflect.Complex128:
		truth = val.Complex() != 0
	case reflect.Chan, reflect.Func, reflect.Pointer, reflect.Interface:
		truth = !val.IsNil()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		truth = val.Int() != 0
	case reflect.Float32, reflect.Float64:
		truth = val.Float() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		truth = val.Uint() != 0
	case reflect.Struct:
		truth = true // Struct values are always true.
	default:
		return
	}
	return truth, true
}

// and and or are retained for the builtins()/FuncMap surface (name presence
// and signature), but are not invoked at runtime: the executor intercepts the
// built-in and/or in evalFunction and evaluates them lazily via evalAndOr, so
// argument evaluation short-circuits. These functions still encode the
// value-selection semantics that evalAndOr mirrors.
func and(arg0 reflect.Value, args ...reflect.Value) reflect.Value {
	if !truth(arg0) {
		return arg0
	}
	for i := range args {
		arg0 = args[i]
		if !truth(arg0) {
			break
		}
	}
	return arg0
}

func or(arg0 reflect.Value, args ...reflect.Value) reflect.Value {
	if truth(arg0) {
		return arg0
	}
	for i := range args {
		arg0 = args[i]
		if truth(arg0) {
			break
		}
	}
	return arg0
}

func not(arg reflect.Value) bool {
	return !truth(arg)
}

func truth(arg reflect.Value) bool {
	t, _ := isTrue(arg)
	return t
}

func length(item reflect.Value) (int, error) {
	item, isNil := indirect(item)
	if isNil {
		return 0, fmt.Errorf("len of nil pointer")
	}
	switch item.Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
		return item.Len(), nil
	default:
		return 0, fmt.Errorf("len of type %s", item.Type())
	}
}

func call(fn reflect.Value, args ...reflect.Value) (reflect.Value, error) {
	typ := fn.Type()
	if typ.Kind() != reflect.Func {
		return reflect.Value{}, fmt.Errorf("non-function of type %s", typ)
	}
	if !goodFunc(typ) {
		return reflect.Value{}, fmt.Errorf("function called with %d args; should be 1 or 2", typ.NumOut())
	}
	numIn := typ.NumIn()
	var dddType reflect.Type
	if typ.IsVariadic() {
		if len(args) < numIn-1 {
			return reflect.Value{}, fmt.Errorf("wrong number of args: got %d want at least %d", len(args), numIn-1)
		}
		dddType = typ.In(numIn - 1).Elem()
	} else {
		if len(args) != numIn {
			return reflect.Value{}, fmt.Errorf("wrong number of args: got %d want %d", len(args), numIn)
		}
	}
	argv := make([]reflect.Value, len(args))
	for i, arg := range args {
		arg = prepareArg(arg, argType(typ, i, numIn, dddType))
		argv[i] = arg
	}
	return safeCall(fn, argv)
}

func argType(typ reflect.Type, i, numIn int, dddType reflect.Type) reflect.Type {
	if !typ.IsVariadic() || i < numIn-1 {
		return typ.In(i)
	}
	return dddType
}

func safeCall(fun reflect.Value, args []reflect.Value) (val reflect.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("%v", r)
			}
		}
	}()
	ret := fun.Call(args)
	if len(ret) == 2 && !ret[1].IsNil() {
		return ret[0], ret[1].Interface().(error)
	}
	return ret[0], nil
}

// index returns the result of indexing its first argument by the following arguments.
func index(item reflect.Value, indexes ...reflect.Value) (reflect.Value, error) {
	item = indirectInterface(item)
	if !item.IsValid() {
		return reflect.Value{}, fmt.Errorf("index of untyped nil")
	}
	for _, idx := range indexes {
		idx = indirectInterface(idx)
		var isNil bool
		if item, isNil = indirect(item); isNil {
			return reflect.Value{}, fmt.Errorf("index of nil pointer")
		}
		switch item.Kind() {
		case reflect.Array, reflect.Slice, reflect.String:
			x, err := indexArg(idx, item.Len())
			if err != nil {
				return reflect.Value{}, err
			}
			item = item.Index(x)
		case reflect.Map:
			idx, err := prepareArgErr(idx, item.Type().Key())
			if err != nil {
				return reflect.Value{}, err
			}
			elemType := item.Type().Elem()
			item = item.MapIndex(idx)
			if !item.IsValid() {
				item = reflect.Zero(elemType)
			}
		default:
			return reflect.Value{}, fmt.Errorf("can't index item of type %s", item.Type())
		}
	}
	return item, nil
}

func indexArg(idx reflect.Value, length int) (int, error) {
	var x int64
	switch idx.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		x = idx.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		x = int64(idx.Uint())
	case reflect.Float32, reflect.Float64:
		x = int64(idx.Float())
	default:
		return 0, fmt.Errorf("cannot index slice/array with type %s", idx.Type())
	}
	if x < 0 || int(x) >= length {
		return 0, fmt.Errorf("index out of range: %d", x)
	}
	return int(x), nil
}

// slice returns the result of slicing its first argument.
func slice(item reflect.Value, indexes ...reflect.Value) (reflect.Value, error) {
	item = indirectInterface(item)
	if !item.IsValid() {
		return reflect.Value{}, fmt.Errorf("slice of untyped nil")
	}
	if len(indexes) > 3 {
		return reflect.Value{}, fmt.Errorf("too many slice indexes: %d", len(indexes))
	}
	var cap int
	switch item.Kind() {
	case reflect.String:
		if len(indexes) == 3 {
			return reflect.Value{}, fmt.Errorf("cannot 3-index slice a string")
		}
		cap = item.Len()
	case reflect.Array, reflect.Slice:
		cap = item.Cap()
	default:
		return reflect.Value{}, fmt.Errorf("can't slice item of type %s", item.Type())
	}
	// Set defaults
	idx := [3]int{0, item.Len(), cap}
	for i, idxVal := range indexes {
		x, err := indexArg(idxVal, cap+1)
		if err != nil {
			return reflect.Value{}, err
		}
		idx[i] = x
	}
	if len(indexes) < 3 {
		return item.Slice(idx[0], idx[1]), nil
	}
	return item.Slice3(idx[0], idx[1], idx[2]), nil
}

// Comparison functions

func eq(arg1 reflect.Value, arg2 ...reflect.Value) (bool, error) {
	arg1 = indirectInterface(arg1)
	if len(arg2) == 0 {
		return false, fmt.Errorf("missing argument for comparison")
	}
	k1, _ := basicKind(arg1)
	for _, a := range arg2 {
		a = indirectInterface(a)
		k2, _ := basicKind(a)
		truth, err := doComparison(k1, k2, arg1, a)
		if err != nil {
			return false, err
		}
		if truth {
			return true, nil
		}
	}
	return false, nil
}

func ne(arg1, arg2 reflect.Value) (bool, error) {
	equal, err := eq(arg1, arg2)
	return !equal, err
}

func lt(arg1, arg2 reflect.Value) (bool, error) {
	arg1 = indirectInterface(arg1)
	arg2 = indirectInterface(arg2)
	k1, _ := basicKind(arg1)
	k2, _ := basicKind(arg2)
	return doLt(k1, k2, arg1, arg2)
}

func le(arg1, arg2 reflect.Value) (bool, error) {
	lessThan, err := lt(arg1, arg2)
	if lessThan || err != nil {
		return lessThan, err
	}
	equal, err := eq(arg1, arg2)
	return equal, err
}

func gt(arg1, arg2 reflect.Value) (bool, error) {
	lessOrEqual, err := le(arg1, arg2)
	if err != nil {
		return false, err
	}
	return !lessOrEqual, nil
}

func ge(arg1, arg2 reflect.Value) (bool, error) {
	lessThan, err := lt(arg1, arg2)
	if err != nil {
		return false, err
	}
	return !lessThan, nil
}

type kind int

const (
	invalidKind kind = iota
	boolKind
	complexKind
	intKind
	floatKind
	stringKind
	uintKind
)

func basicKind(v reflect.Value) (kind, error) {
	switch v.Kind() {
	case reflect.Bool:
		return boolKind, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return intKind, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return uintKind, nil
	case reflect.Float32, reflect.Float64:
		return floatKind, nil
	case reflect.Complex64, reflect.Complex128:
		return complexKind, nil
	case reflect.String:
		return stringKind, nil
	}
	return invalidKind, fmt.Errorf("uncomparable type %s: %v", v.Type(), v)
}

func doComparison(k1, k2 kind, v1, v2 reflect.Value) (bool, error) {
	if k1 != k2 {
		switch {
		case k1 == intKind && k2 == uintKind:
			return v1.Int() >= 0 && uint64(v1.Int()) == v2.Uint(), nil
		case k1 == uintKind && k2 == intKind:
			return v2.Int() >= 0 && v1.Uint() == uint64(v2.Int()), nil
		case k1 == intKind && k2 == floatKind:
			return float64(v1.Int()) == v2.Float(), nil
		case k1 == floatKind && k2 == intKind:
			return v1.Float() == float64(v2.Int()), nil
		case k1 == uintKind && k2 == floatKind:
			return float64(v1.Uint()) == v2.Float(), nil
		case k1 == floatKind && k2 == uintKind:
			return v1.Float() == float64(v2.Uint()), nil
		default:
			return false, fmt.Errorf("incompatible types for comparison")
		}
	}
	switch k1 {
	case boolKind:
		return v1.Bool() == v2.Bool(), nil
	case complexKind:
		return v1.Complex() == v2.Complex(), nil
	case floatKind:
		return v1.Float() == v2.Float(), nil
	case intKind:
		return v1.Int() == v2.Int(), nil
	case stringKind:
		return v1.String() == v2.String(), nil
	case uintKind:
		return v1.Uint() == v2.Uint(), nil
	}
	return false, fmt.Errorf("uncomparable types")
}

func doLt(k1, k2 kind, v1, v2 reflect.Value) (bool, error) {
	if k1 != k2 {
		switch {
		case k1 == intKind && k2 == uintKind:
			return v1.Int() < 0 || uint64(v1.Int()) < v2.Uint(), nil
		case k1 == uintKind && k2 == intKind:
			return v2.Int() >= 0 && v1.Uint() < uint64(v2.Int()), nil
		case k1 == intKind && k2 == floatKind:
			return float64(v1.Int()) < v2.Float(), nil
		case k1 == floatKind && k2 == intKind:
			return v1.Float() < float64(v2.Int()), nil
		case k1 == uintKind && k2 == floatKind:
			return float64(v1.Uint()) < v2.Float(), nil
		case k1 == floatKind && k2 == uintKind:
			return v1.Float() < float64(v2.Uint()), nil
		default:
			return false, fmt.Errorf("incompatible types for comparison")
		}
	}
	switch k1 {
	case boolKind, complexKind:
		return false, fmt.Errorf("< not defined on bool or complex")
	case floatKind:
		return v1.Float() < v2.Float(), nil
	case intKind:
		return v1.Int() < v2.Int(), nil
	case stringKind:
		return v1.String() < v2.String(), nil
	case uintKind:
		return v1.Uint() < v2.Uint(), nil
	}
	return false, fmt.Errorf("uncomparable types")
}

// HTML, JS, and URL-query escaping

func htmlEscaper(args ...any) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&#34;",
		"'", "&#39;",
	).Replace(fmt.Sprint(args...))
}

func jsEscaper(args ...any) string {
	return strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"'", `\'`,
		"<", `\x3C`,
		">", `\x3E`,
		"&", `\x26`,
	).Replace(fmt.Sprint(args...))
}

func urlQueryEscaper(args ...any) string {
	return urlEscape(fmt.Sprint(args...))
}

func urlEscape(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if shouldEscape(c) {
			fmt.Fprintf(&b, "%%%02X", c)
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
}

func shouldEscape(c byte) bool {
	if 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' {
		return false
	}
	switch c {
	case '-', '_', '.', '~':
		return false
	}
	return true
}

// prepareArg checks and converts an argument value to the required type.
func prepareArg(value reflect.Value, typ reflect.Type) reflect.Value {
	if !value.IsValid() {
		if canBeNil(typ) {
			value = reflect.Zero(typ)
		} else {
			value = reflect.Zero(typ)
		}
	}
	if value.Type().AssignableTo(typ) {
		return value
	}
	if intLike(value.Kind()) && intLike(typ.Kind()) && value.Type().ConvertibleTo(typ) {
		value = value.Convert(typ)
	}
	return value
}

func prepareArgErr(value reflect.Value, typ reflect.Type) (reflect.Value, error) {
	if !value.IsValid() {
		if canBeNil(typ) {
			return reflect.Zero(typ), nil
		}
		return reflect.Zero(typ), fmt.Errorf("value is nil; should be of type %s", typ)
	}
	if value.Type().AssignableTo(typ) {
		return value, nil
	}
	if intLike(value.Kind()) && intLike(typ.Kind()) && value.Type().ConvertibleTo(typ) {
		return value.Convert(typ), nil
	}
	return reflect.Value{}, fmt.Errorf("value has type %s; should be %s", value.Type(), typ)
}

func intLike(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return true
	}
	return false
}

func canBeNil(typ reflect.Type) bool {
	switch typ.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return true
	}
	return false
}

// goodFunc reports whether the function has the right return signature.
func goodFunc(typ reflect.Type) bool {
	switch {
	case typ.NumOut() == 1:
		return true
	case typ.NumOut() == 2 && typ.Out(1) == errorType:
		return true
	}
	return false
}

var errorType = reflect.TypeFor[error]()

// indirect returns the value dereferencing through any pointer and reports whether the value is nil.
func indirect(v reflect.Value) (rv reflect.Value, isNil bool) {
	for ; v.Kind() == reflect.Pointer; v = v.Elem() {
		if v.IsNil() {
			return v, true
		}
	}
	return v, false
}

// indirectInterface returns the concrete value in an interface value.
func indirectInterface(v reflect.Value) reflect.Value {
	if v.Kind() != reflect.Interface {
		return v
	}
	if v.IsNil() {
		return reflect.Value{}
	}
	return v.Elem()
}
