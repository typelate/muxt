package astgen

import (
	"fmt"
	"go/ast"
	"go/types"

	"github.com/typelate/muxt/internal/source"
)

// ConvertToString formats variable, whose type is tp, as a string.
func ConvertToString(im ImportManager, variable ast.Expr, tp source.Type) (ast.Expr, error) {
	kind, ok := tp.Basic()
	if !ok {
		return nil, fmt.Errorf("unsupported type for path parameters")
	}
	switch kind {
	case types.Bool, types.UntypedBool:
		return Call(im, "", "strconv", "FormatBool", ConvertIdent("bool", variable)), nil
	case types.Int, types.UntypedInt:
		return StrconvItoaCall(im, variable), nil
	case types.Int8, types.Int16, types.Int32, types.Int64:
		return formatInteger(im, "FormatInt", "int64", variable), nil
	case types.Uint, types.Uint8, types.Uint16, types.Uint32:
		return formatInteger(im, "FormatUint", "uint64", variable), nil
	case types.Uint64:
		return Call(im, "", "strconv", "FormatUint", variable, Int(10)), nil
	case types.String:
		return variable, nil
	default:
		return nil, fmt.Errorf("unsupported basic type for path parameters")
	}
}

// formatInteger calls the strconv function format on variable converted to
// the widest type it takes.
func formatInteger(im ImportManager, format, widest string, variable ast.Expr) *ast.CallExpr {
	return Call(im, "", "strconv", format, ConvertIdent(widest, variable), Int(10))
}

func StrconvAtoiCall(im ImportManager, expr ast.Expr) *ast.CallExpr {
	return Call(im, "", "strconv", "Atoi", expr)
}

func StrconvItoaCall(im ImportManager, expr ast.Expr) *ast.CallExpr {
	return Call(im, "", "strconv", "Itoa", expr)
}

func StrconvParseIntCall(im ImportManager, expr ast.Expr, base, size int) *ast.CallExpr {
	return Call(im, "", "strconv", "ParseInt", expr, Int(base), Int(size))
}

func StrconvParseUintCall(im ImportManager, expr ast.Expr, base, size int) *ast.CallExpr {
	return Call(im, "", "strconv", "ParseUint", expr, Int(base), Int(size))
}

func StrconvParseFloatCall(im ImportManager, expr ast.Expr, size int) *ast.CallExpr {
	return Call(im, "", "strconv", "ParseFloat", expr, Int(size))
}

func StrconvParseBoolCall(im ImportManager, expr ast.Expr) *ast.CallExpr {
	return Call(im, "", "strconv", "ParseBool", expr)
}
