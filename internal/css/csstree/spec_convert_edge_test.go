package csstree

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// FromPlainObject reads numbers of JSON decoded with UseNumber and floats
// holding integers, and reports a field of the wrong kind (Go only: upstream
// fromPlainObject does not check its input).
func TestFromPlainObjectFields(t *testing.T) {
	t.Run("json.Number locations", func(t *testing.T) {
		dec := json.NewDecoder(strings.NewReader(`{"type":"Identifier","name":"a","loc":{"source":"s","start":{"offset":0,"line":1,"column":1},"end":{"offset":1,"line":1,"column":2}}}`))
		dec.UseNumber()
		var object map[string]any
		if err := dec.Decode(&object); err != nil {
			t.Fatal(err)
		}
		node, err := FromPlainObject(object)
		if err != nil {
			t.Fatal(err)
		}
		if loc := node.Location(); loc == nil || loc.Source != "s" || loc.End.Offset != 1 || loc.End.Column != 2 {
			t.Errorf("Location() = %+v", loc)
		}
	})

	for _, tt := range []struct {
		name   string
		object map[string]any
		want   string
	}{
		{name: "type not a string", object: map[string]any{"type": 1.0}, want: `plain object 1 field "type": float64 is not a string`},
		{name: "unknown type", object: map[string]any{"type": "Bogus"}, want: "Unknown node type: Bogus"},
		{name: "string field", object: map[string]any{"type": "Identifier", "name": 1.0}, want: `plain object Identifier field "name": float64 is not a string`},
		{name: "node field", object: map[string]any{"type": "Declaration", "property": "a", "value": "x"}, want: `plain object Declaration field "value": string is not a node`},
		{name: "list field", object: map[string]any{"type": "Value", "children": "x"}, want: `plain object Value field "children": string is not a list of nodes`},
		{name: "list item", object: map[string]any{"type": "Value", "children": []any{"x"}}, want: `plain object Value field "children": []interface {} is not a list of nodes`},
		{name: "loc", object: map[string]any{"type": "Identifier", "name": "a", "loc": "x"}, want: `plain object Identifier field "loc": string is not a location`},
		{name: "position", object: map[string]any{"type": "Identifier", "name": "a", "loc": map[string]any{"start": "x"}}, want: `plain object <nil> field "start": string is not a position`},
		{
			name:   "fractional offset",
			object: map[string]any{"type": "Identifier", "name": "a", "loc": map[string]any{"start": map[string]any{"offset": 1.5, "line": 1.0, "column": 1.0}, "end": map[string]any{"offset": 2.0, "line": 1.0, "column": 2.0}}},
			want:   `plain object <nil> field "offset": float64 is not an integer`,
		},
		{
			name:   "bad json.Number",
			object: map[string]any{"type": "Identifier", "name": "a", "loc": map[string]any{"start": map[string]any{"offset": json.Number("1e400"), "line": 1.0, "column": 1.0}, "end": map[string]any{"offset": 2.0, "line": 1.0, "column": 2.0}}},
			want:   `plain object <nil> field "offset": json.Number is not an integer`,
		},
		{name: "node type of a field", object: map[string]any{"type": "Rule", "prelude": map[string]any{"type": "Raw", "value": "a"}, "block": map[string]any{"type": "Raw", "value": "b"}}, want: `plain object Rule field "block": node type Raw is not *csstree.Block`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			node, err := FromPlainObject(tt.object)
			if err == nil || err.Error() != tt.want {
				t.Errorf("FromPlainObject(%v) = %v, %v; want error %s", tt.object, node, err, tt.want)
			}
		})
	}

	var unknown *UnknownNodeTypeError
	if _, err := FromPlainObject(map[string]any{"type": "Bogus"}); !errors.As(err, &unknown) || unknown.Type != "Bogus" {
		t.Errorf("FromPlainObject(Bogus) = %v, want *UnknownNodeTypeError", err)
	}
}
