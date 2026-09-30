package muxt

import (
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckRepeatedArgument(t *testing.T) {
	request := types.NewNamed(types.NewTypeName(token.NoPos, types.NewPackage("net/http", "http"), "Request", nil), types.NewStruct(nil, nil), nil)
	otherRequest := types.NewNamed(types.NewTypeName(token.NoPos, types.NewPackage("example.com/other", "other"), "Request", nil), types.NewStruct(nil, nil), nil)
	str, number := types.Typ[types.String], types.Typ[types.Int]
	anyType := types.Universe.Lookup("any").Type()

	for _, tt := range []struct {
		name          string
		first, second Argument
		use           int
		wantErr       string
	}{
		{
			name:   "the same type",
			first:  Argument{paramType: str},
			second: Argument{paramType: str},
			use:    2,
		},
		{
			name:   "identical pointer types that were built separately",
			first:  Argument{paramType: types.NewPointer(request), direct: true},
			second: Argument{paramType: types.NewPointer(request), direct: true},
			use:    2,
		},
		{
			name:    "same-named types from different packages differ",
			first:   Argument{paramType: types.NewPointer(request), direct: true},
			second:  Argument{paramType: types.NewPointer(otherRequest), direct: true},
			use:     2,
			wantErr: "id is passed more than once with different types: *net/http.Request at the first use and *example.com/other.Request at use 2",
		},
		{
			name:    "different types both passed directly",
			first:   Argument{paramType: str, direct: true},
			second:  Argument{paramType: anyType, direct: true},
			use:     2,
			wantErr: "id is passed more than once with different types: string at the first use and any at use 2",
		},
		{
			name:    "different types, only the first passed directly",
			first:   Argument{paramType: str, direct: true},
			second:  Argument{paramType: number},
			use:     2,
			wantErr: "id is passed more than once with different types: string at the first use and int at use 2",
		},
		{
			name:    "different types, only the second passed directly",
			first:   Argument{paramType: str},
			second:  Argument{paramType: number, direct: true},
			use:     2,
			wantErr: "id is passed more than once with different types: string at the first use and int at use 2",
		},
		{
			name:    "the use that differs is named",
			first:   Argument{paramType: number},
			second:  Argument{paramType: str},
			use:     3,
			wantErr: "id is passed more than once with different types: int at the first use and string at use 3",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.first.Identifier, tt.second.Identifier = "id", "id"
			err := checkRepeatedArgument(&Definition{}, nil, &tt.first, &tt.second, tt.use)
			if tt.wantErr == "" {
				assert.NoError(t, err, "checkRepeatedArgument()")
				return
			}
			assert.EqualError(t, err, tt.wantErr, "checkRepeatedArgument()")
		})
	}
}

// TestDeclaresLocal states, for every argument kind, whether the generated
// handler declares one local for it, which is what makes repeated uses share
// a value and so need identical parameter types. A new kind fails here until
// it is classified.
func TestDeclaresLocal(t *testing.T) {
	want := map[ArgumentType]bool{
		ArgumentTypeUnknown:              false,
		ArgumentTypeRequest:              false,
		ArgumentTypeResponse:             false,
		ArgumentTypeRequestContext:       true,
		ArgumentTypeRequestPathValue:     true,
		ArgumentTypeRequestForm:          true,
		ArgumentTypeRequestMultipartForm: true,
		ArgumentTypeExecute:              false,
		ArgumentTypeSendMessage:          false,
		ArgumentTypeSignalsCallback:      false,
		ArgumentTypeLastEventID:          true,
		ArgumentTypeRequestBody:          true,
		ArgumentTypeRequestBodyJSON:      false,
		ArgumentTypeCall:                 false,
	}
	for kind := ArgumentTypeUnknown; kind <= ArgumentTypeCall; kind++ {
		got, ok := want[kind]
		if !assert.True(t, ok, "ArgumentType %d is not classified", kind) {
			continue
		}
		assert.Equal(t, got, declaresLocal(kind), "declaresLocal(%d)", kind)
	}
}
