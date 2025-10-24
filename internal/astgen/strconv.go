package astgen

import (
	"fmt"
	"go/ast"
	"go/types"
)

// ConvertToString converts a variable to its string representation based on its basic kind
func ConvertToString(im ImportManager, variable ast.Expr, kind types.BasicKind) (ast.Expr, error) {
	switch kind {
	case types.Bool, types.UntypedBool:
		return FormatBool(im, variable), nil
	case types.Int, types.UntypedInt:
		return FormatInt(im, variable), nil
	case types.Int8:
		return FormatInt8(im, variable), nil
	case types.Int16:
		return FormatInt16(im, variable), nil
	case types.Int32:
		return FormatInt32(im, variable), nil
	case types.Int64:
		return FormatInt64(im, variable), nil
	case types.Uint:
		return FormatUint(im, variable), nil
	case types.Uint8:
		return FormatUint8(im, variable), nil
	case types.Uint16:
		return FormatUint16(im, variable), nil
	case types.Uint32:
		return FormatUint32(im, variable), nil
	case types.Uint64:
		return FormatUint64(im, variable), nil
	case types.String:
		return variable, nil
	default:
		return nil, fmt.Errorf("unsupported basic type for path parameters")
	}
}

// StrconvAtoiCall creates a strconv.Atoi call expression
func StrconvAtoiCall(im ImportManager, expr ast.Expr) *ast.CallExpr {
	return Call(im, "", "strconv", "Atoi", []ast.Expr{expr})
}

// StrconvItoaCall creates a strconv.Itoa call expression
func StrconvItoaCall(im ImportManager, expr ast.Expr) *ast.CallExpr {
	return Call(im, "", "strconv", "Itoa", []ast.Expr{expr})
}

// StrconvParseIntCall creates a strconv.ParseInt call expression
func StrconvParseIntCall(im ImportManager, expr ast.Expr, base, size int) *ast.CallExpr {
	return Call(im, "", "strconv", "ParseInt", []ast.Expr{expr, Int(base), Int(size)})
}

// StrconvParseUintCall creates a strconv.ParseUint call expression
func StrconvParseUintCall(im ImportManager, expr ast.Expr, base, size int) *ast.CallExpr {
	return Call(im, "", "strconv", "ParseUint", []ast.Expr{expr, Int(base), Int(size)})
}

// StrconvParseFloatCall creates a strconv.ParseFloat call expression
func StrconvParseFloatCall(im ImportManager, expr ast.Expr, size int) *ast.CallExpr {
	return Call(im, "", "strconv", "ParseFloat", []ast.Expr{expr, Int(size)})
}

// StrconvParseBoolCall creates a strconv.ParseBool call expression
func StrconvParseBoolCall(im ImportManager, expr ast.Expr) *ast.CallExpr {
	return Call(im, "", "strconv", "ParseBool", []ast.Expr{expr})
}

// StrconvParseInt8Call creates a strconv.ParseInt call for int8
func StrconvParseInt8Call(im ImportManager, in ast.Expr) *ast.CallExpr {
	return StrconvParseIntCall(im, in, 10, 8)
}

// StrconvParseInt16Call creates a strconv.ParseInt call for int16
func StrconvParseInt16Call(im ImportManager, in ast.Expr) *ast.CallExpr {
	return StrconvParseIntCall(im, in, 10, 16)
}

// StrconvParseInt32Call creates a strconv.ParseInt call for int32
func StrconvParseInt32Call(im ImportManager, in ast.Expr) *ast.CallExpr {
	return StrconvParseIntCall(im, in, 10, 32)
}

// StrconvParseInt64Call creates a strconv.ParseInt call for int64
func StrconvParseInt64Call(im ImportManager, in ast.Expr) *ast.CallExpr {
	return StrconvParseIntCall(im, in, 10, 64)
}

// StrconvParseUint0Call creates a strconv.ParseUint call for uint
func StrconvParseUint0Call(im ImportManager, in ast.Expr) *ast.CallExpr {
	return StrconvParseUintCall(im, in, 10, 0)
}

// StrconvParseUint8Call creates a strconv.ParseUint call for uint8
func StrconvParseUint8Call(im ImportManager, in ast.Expr) *ast.CallExpr {
	return StrconvParseUintCall(im, in, 10, 8)
}

// StrconvParseUint16Call creates a strconv.ParseUint call for uint16
func StrconvParseUint16Call(im ImportManager, in ast.Expr) *ast.CallExpr {
	return StrconvParseUintCall(im, in, 10, 16)
}

// StrconvParseUint32Call creates a strconv.ParseUint call for uint32
func StrconvParseUint32Call(im ImportManager, in ast.Expr) *ast.CallExpr {
	return StrconvParseUintCall(im, in, 10, 32)
}

// StrconvParseUint64Call creates a strconv.ParseUint call for uint64
func StrconvParseUint64Call(im ImportManager, in ast.Expr) *ast.CallExpr {
	return StrconvParseUintCall(im, in, 10, 64)
}

// FormatInt creates a strconv.Itoa call expression
func FormatInt(im ImportManager, in ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  ExportedIdentifier(im, "", "strconv", "Itoa"),
		Args: []ast.Expr{in},
	}
}

// FormatInt8 creates a strconv.FormatInt call for int8
func FormatInt8(im ImportManager, in ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  ExportedIdentifier(im, "", "strconv", "FormatInt"),
		Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("int64"), Args: []ast.Expr{in}}, Int(10)},
	}
}

// FormatInt16 creates a strconv.FormatInt call for int16
func FormatInt16(im ImportManager, in ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  ExportedIdentifier(im, "", "strconv", "FormatInt"),
		Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("int64"), Args: []ast.Expr{in}}, Int(10)},
	}
}

// FormatInt32 creates a strconv.FormatInt call for int32
func FormatInt32(im ImportManager, in ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  ExportedIdentifier(im, "", "strconv", "FormatInt"),
		Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("int64"), Args: []ast.Expr{in}}, Int(10)},
	}
}

// FormatInt64 creates a strconv.FormatInt call for int64
func FormatInt64(im ImportManager, in ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  ExportedIdentifier(im, "", "strconv", "FormatInt"),
		Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("int64"), Args: []ast.Expr{in}}, Int(10)},
	}
}

// FormatUint creates a strconv.FormatUint call for uint
func FormatUint(im ImportManager, in ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  ExportedIdentifier(im, "", "strconv", "FormatUint"),
		Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("uint64"), Args: []ast.Expr{in}}, Int(10)},
	}
}

// FormatUint8 creates a strconv.FormatUint call for uint8
func FormatUint8(im ImportManager, in ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  ExportedIdentifier(im, "", "strconv", "FormatUint"),
		Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("uint64"), Args: []ast.Expr{in}}, Int(10)},
	}
}

// FormatUint16 creates a strconv.FormatUint call for uint16
func FormatUint16(im ImportManager, in ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  ExportedIdentifier(im, "", "strconv", "FormatUint"),
		Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("uint64"), Args: []ast.Expr{in}}, Int(10)},
	}
}

// FormatUint32 creates a strconv.FormatUint call for uint32
func FormatUint32(im ImportManager, in ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  ExportedIdentifier(im, "", "strconv", "FormatUint"),
		Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("uint64"), Args: []ast.Expr{in}}, Int(10)},
	}
}

// FormatUint64 creates a strconv.FormatUint call for uint64
func FormatUint64(im ImportManager, in ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  ExportedIdentifier(im, "", "strconv", "FormatUint"),
		Args: []ast.Expr{in, Int(10)},
	}
}

// FormatBool creates a strconv.FormatBool call expression
func FormatBool(im ImportManager, in ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun:  ExportedIdentifier(im, "", "strconv", "FormatBool"),
		Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("bool"), Args: []ast.Expr{in}}},
	}
}
