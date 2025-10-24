package asteval

import (
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStringWithType(t *testing.T) {
	for _, tt := range []struct {
		Name          string
		Value         string
		Type          types.Type
		ErrorContains string
	}{
		{Name: "invalid int", Value: "abc", Type: types.Universe.Lookup("int").Type(), ErrorContains: `parsing "abc": invalid syntax`},
		{Name: "valid int", Value: "32", Type: types.Universe.Lookup("int").Type()},
		{Name: "valid int8", Value: "32", Type: types.Universe.Lookup("int8").Type()},
		{Name: "valid int16", Value: "32", Type: types.Universe.Lookup("int16").Type()},
		{Name: "valid int32", Value: "32", Type: types.Universe.Lookup("int32").Type()},
		{Name: "valid int64", Value: "32", Type: types.Universe.Lookup("int64").Type()},
		{Name: "valid uint", Value: "32", Type: types.Universe.Lookup("uint").Type()},
		{Name: "valid uint8", Value: "32", Type: types.Universe.Lookup("uint8").Type()},
		{Name: "valid uint16", Value: "32", Type: types.Universe.Lookup("uint16").Type()},
		{Name: "valid uint32", Value: "32", Type: types.Universe.Lookup("uint32").Type()},
		{Name: "valid uint64", Value: "32", Type: types.Universe.Lookup("uint64").Type()},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			_, err := ParseWithType(tt.Value, tt.Type)
			if tt.ErrorContains != "" {
				assert.Contains(t, err.Error(), tt.ErrorContains)
			} else if err != nil {
				require.NoError(t, err)
			}
		})
	}
}
