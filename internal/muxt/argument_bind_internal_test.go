package muxt

import (
	"go/types"
	"testing"
)

func TestCheckRepeatedArgument(t *testing.T) {
	for _, tt := range []struct {
		name          string
		first, second Argument
		wantErr       string
	}{
		{
			name:   "same type",
			first:  Argument{paramType: types.Typ[types.String]},
			second: Argument{paramType: types.Typ[types.String]},
		},
		{
			name:   "different types both passed directly",
			first:  Argument{paramType: types.Typ[types.String], direct: true},
			second: Argument{paramType: types.Typ[types.Int], direct: true},
		},
		{
			name:    "different types, only the first passed directly",
			first:   Argument{paramType: types.Typ[types.String], direct: true},
			second:  Argument{paramType: types.Typ[types.Int]},
			wantErr: "id is passed more than once with different types: string and int",
		},
		{
			name:    "different types, only the second passed directly",
			first:   Argument{paramType: types.Typ[types.String]},
			second:  Argument{paramType: types.Typ[types.Int], direct: true},
			wantErr: "id is passed more than once with different types: string and int",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.first.Identifier, tt.second.Identifier = "id", "id"
			err := checkRepeatedArgument(&Definition{}, nil, &tt.first, &tt.second)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("checkRepeatedArgument() = %v, want nil", err)
			case tt.wantErr != "" && (err == nil || err.Error() != tt.wantErr):
				t.Errorf("checkRepeatedArgument() = %v, want %q", err, tt.wantErr)
			}
		})
	}
}
