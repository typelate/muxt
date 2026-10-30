package check

import (
	"go/types"
	"strings"

	"github.com/typelate/muxt/internal/template"
)

// checkPrintf validates a literal printf format string against the types of
// the arguments that follow it. It runs only when the format (the first
// argument) is a string literal; a dynamic format is left unchecked. It
// mirrors the safe subset of go vet's printf analysis over template argument
// types: it reports a verb whose argument is definitely the wrong kind (a
// string for %d, say) and a mismatch between the number of verbs and
// arguments. It is deliberately conservative — %v, interfaces, fmt.Formatter
// and fmt.Stringer values, and every aggregate type are accepted.
func checkPrintf(g *Global, argNodes []template.Node, argTypes []types.Type) error {
	if len(argNodes) == 0 {
		return nil
	}
	lit, ok := argNodes[0].(*template.StringNode)
	if !ok {
		return nil // dynamic format string; cannot check statically
	}
	format := lit.Text
	args := argTypes[1:] // the format arguments (types); may include a piped value
	used := 0
	next := func() (types.Type, bool) {
		if used >= len(args) {
			return nil, false
		}
		t := args[used]
		used++
		return t, true
	}

	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		i++
		if i >= len(format) {
			break // trailing '%'
		}
		switch format[i] {
		case '%':
			continue // "%%" is a literal percent, no argument
		case '[':
			return nil // explicit argument indexes ("%[2]d") — leave unchecked
		}
		// Flags.
		for i < len(format) && strings.IndexByte("+-# 0", format[i]) >= 0 {
			i++
		}
		// Width: a literal number, or '*' consuming an int argument.
		if star, err := scanStarWidth(g, format, &i, next, "width"); err != nil {
			return err
		} else if !star {
			skipDigits(format, &i)
		}
		// Precision: '.' then a number or '*'.
		if i < len(format) && format[i] == '.' {
			i++
			if star, err := scanStarWidth(g, format, &i, next, "precision"); err != nil {
				return err
			} else if !star {
				skipDigits(format, &i)
			}
		}
		if i >= len(format) {
			break
		}
		verb := format[i]
		t, ok := next()
		if !ok {
			return errorf(ErrorTypeCallArguments, "printf: not enough arguments for format %q", format)
		}
		if want := verbMismatch(rune(verb), t); want != "" {
			return errorf(ErrorTypeCallArguments, "printf: %%%c needs %s but the argument is %s", verb, want, t).withX(t)
		}
	}
	if used < len(args) {
		return errorf(ErrorTypeCallArguments, "printf: too many arguments for format %q", format)
	}
	return nil
}

// scanStarWidth consumes a '*' width or precision at format[*i], validating
// that the consumed argument is an integer. It reports whether a '*' was
// present so the caller can otherwise skip literal digits.
func scanStarWidth(g *Global, format string, i *int, next func() (types.Type, bool), what string) (bool, error) {
	if *i >= len(format) || format[*i] != '*' {
		return false, nil
	}
	t, ok := next()
	if !ok {
		return true, errorf(ErrorTypeCallArguments, "printf: not enough arguments for format %q", format)
	}
	if verbMismatch('d', t) != "" {
		return true, errorf(ErrorTypeCallArguments, "printf: %s (*) needs an integer but the argument is %s", what, t).withX(t)
	}
	*i++
	return true, nil
}

func skipDigits(format string, i *int) {
	for *i < len(format) && format[*i] >= '0' && format[*i] <= '9' {
		*i++
	}
}

// verbMismatch returns a short description of what a printf verb needs when
// t is definitely the wrong kind for it, or "" when t is acceptable or cannot
// be judged confidently. Only basic (non-aggregate) kinds are judged; any
// interface, any value with a Format/String/Error method, and every aggregate
// type are accepted, keeping false positives out.
func verbMismatch(verb rune, t types.Type) string {
	if t == nil || types.IsInterface(t) {
		return ""
	}
	if hasMethod(t, "Format") { // fmt.Formatter handles any verb
		return ""
	}
	b, ok := t.Underlying().(*types.Basic)
	if !ok {
		return "" // aggregate types (slices, structs, pointers, ...) are left alone
	}
	info := b.Info()
	isInt := info&types.IsInteger != 0
	isFloat := info&(types.IsFloat|types.IsComplex) != 0
	isStr := info&types.IsString != 0
	isBool := info&types.IsBoolean != 0
	switch verb {
	case 'd', 'b', 'o', 'O', 'c', 'U':
		if !isInt {
			return "an integer"
		}
	case 'e', 'E', 'f', 'F', 'g', 'G':
		if !isFloat {
			return "a floating-point number"
		}
	case 't':
		if !isBool {
			return "a bool"
		}
	case 's':
		if !isStr && !hasMethod(t, "String") && !hasMethod(t, "Error") {
			return "a string"
		}
	case 'q':
		if !isStr && !isInt && !hasMethod(t, "String") && !hasMethod(t, "Error") {
			return "a string or integer"
		}
	case 'x', 'X':
		if !isInt && !isFloat && !isStr {
			return "an integer, float, or string"
		}
	}
	return ""
}

func hasMethod(t types.Type, name string) bool {
	obj, _, _ := types.LookupFieldOrMethod(t, true, nil, name)
	_, ok := obj.(*types.Func)
	return ok
}

// isPrintfLike reports whether fn has the shape func(string, ...T) — a
// format-string-then-varargs signature — so a user-supplied "printf" that is
// not fmt.Sprintf is not format-checked as one.
func isPrintfLike(fn *types.Signature) bool {
	if fn == nil || !fn.Variadic() || fn.Params().Len() != 2 {
		return false
	}
	b, ok := fn.Params().At(0).Type().Underlying().(*types.Basic)
	return ok && b.Info()&types.IsString != 0
}
