package asteval

import (
	"go/token"
	"go/types"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckParses(t *testing.T) {
	basic := func(name string) types.Type { return types.Universe.Lookup(name).Type() }
	age := types.NewNamed(types.NewTypeName(token.NoPos, nil, "Age", nil), basic("int8"), nil)
	for _, tt := range []struct {
		name    string
		val     string
		tp      types.Type
		wantErr string
	}{
		{name: "int", val: "32", tp: basic("int")},
		{name: "int negative", val: "-32", tp: basic("int")},
		{name: "int8", val: "127", tp: basic("int8")},
		{name: "int8 too large", val: "128", tp: basic("int8"), wantErr: `parsing "128": value out of range`},
		{name: "int8 too small", val: "-129", tp: basic("int8"), wantErr: `parsing "-129": value out of range`},
		{name: "int16", val: "32767", tp: basic("int16")},
		{name: "int16 too large", val: "32768", tp: basic("int16"), wantErr: "value out of range"},
		{name: "int32", val: "2147483647", tp: basic("int32")},
		{name: "int32 too large", val: "2147483648", tp: basic("int32"), wantErr: "value out of range"},
		{name: "int64", val: "9223372036854775807", tp: basic("int64")},
		{name: "int64 too large", val: "9223372036854775808", tp: basic("int64"), wantErr: "value out of range"},
		{name: "int is as wide as the platform int", val: strconv.FormatInt(1<<(strconv.IntSize-1)-1, 10), tp: basic("int")},
		{name: "int too large for the platform", val: "9223372036854775808", tp: basic("int"), wantErr: "value out of range"},
		{name: "uint", val: "32", tp: basic("uint")},
		{name: "uint negative", val: "-1", tp: basic("uint"), wantErr: `parsing "-1": invalid syntax`},
		{name: "uint8", val: "255", tp: basic("uint8")},
		{name: "uint8 too large", val: "256", tp: basic("uint8"), wantErr: "value out of range"},
		{name: "uint16", val: "65535", tp: basic("uint16")},
		{name: "uint16 too large", val: "65536", tp: basic("uint16"), wantErr: "value out of range"},
		{name: "uint32", val: "4294967295", tp: basic("uint32")},
		{name: "uint32 too large", val: "4294967296", tp: basic("uint32"), wantErr: "value out of range"},
		{name: "uint64", val: "18446744073709551615", tp: basic("uint64")},
		{name: "uint64 too large", val: "18446744073709551616", tp: basic("uint64"), wantErr: "value out of range"},
		{name: "malformed int", val: "abc", tp: basic("int"), wantErr: `parsing "abc": invalid syntax`},
		{name: "malformed uint", val: "abc", tp: basic("uint"), wantErr: `parsing "abc": invalid syntax`},
		{name: "empty", val: "", tp: basic("int"), wantErr: `parsing "": invalid syntax`},
		{name: "hexadecimal is not base 10", val: "0x10", tp: basic("int"), wantErr: "invalid syntax"},
		{name: "named type parses as its underlying type", val: "127", tp: age},
		{name: "named type range follows its underlying type", val: "128", tp: age, wantErr: "value out of range"},
		{name: "float64", val: "1.5", tp: basic("float64"), wantErr: "type float64 unknown"},
		{name: "string", val: "x", tp: basic("string"), wantErr: "type string unknown"},
		{name: "bool", val: "true", tp: basic("bool"), wantErr: "type bool unknown"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckParses(tt.val, tt.tp)
			if tt.wantErr == "" {
				assert.NoError(t, err, "CheckParses(%q, %s)", tt.val, tt.tp)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr, "CheckParses(%q, %s)", tt.val, tt.tp)
		})
	}
}
