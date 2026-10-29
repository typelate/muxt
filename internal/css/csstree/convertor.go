// Ported from lib/convertor/create.js.
//
// Upstream converts in place between an AST whose children are Lists and a
// JSON-like AST whose children are arrays, by walking the AST. A Go AST is
// typed, so here the plain object form is a separate value: maps, slices,
// strings, booleans, numbers and nil, exactly what decoding the JSON of an
// AST with encoding/json gives.

package csstree

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

// Convertor converts between ASTs and their plain object form (upstream
// createConvertor's { fromPlainObject, toPlainObject }).
type Convertor struct {
	types map[string]FromPlainObjectFunc
}

// CreateConvertor returns a convertor for the node types of config
// (upstream createConvertor(walk) takes the walker of the config).
func CreateConvertor(config *Config) *Convertor {
	types := make(map[string]FromPlainObjectFunc)

	if config != nil {
		for name, nodeType := range config.Node {
			if nodeType.FromPlainObject != nil {
				types[name] = nodeType.FromPlainObject
			}
		}
	}

	return &Convertor{types: types}
}

// FromPlainObject builds an AST from its plain object form, such as the
// result of ToPlainObject or of decoding the JSON of an AST into an any. It
// fails on a value of the wrong kind or a node of a type without a
// FromPlainObject function.
func (c *Convertor) FromPlainObject(ast map[string]any) (Node, error) {
	r := &PlainObjectReader{types: c.types}
	node := r.node(ast)
	if r.err != nil {
		return nil, r.err
	}
	return node, nil
}

// ToPlainObject returns the plain object form of ast: the value of
// JSON.parse(JSON.stringify(ast)), a map per node with "type", "loc" and
// the node fields, and a slice per list. Nodes are encoded with
// encoding/json, so a node type added through Syntax.Fork needs a
// MarshalJSON method writing "type".
func (c *Convertor) ToPlainObject(ast Node) (map[string]any, error) {
	data, err := json.Marshal(ast)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// PlainObjectReader reads the fields of plain object nodes for the
// FromPlainObject functions of node types. Its methods record the first
// error (a field of the wrong kind) and return zero values after it.
type PlainObjectReader struct {
	types map[string]FromPlainObjectFunc
	err   error
}

// Fail records err as the failure of the conversion unless one was
// already recorded.
func (r *PlainObjectReader) Fail(err error) {
	if r.err == nil {
		r.err = err
	}
}

// Err returns the first error recorded.
func (r *PlainObjectReader) Err() error {
	return r.err
}

// fieldError records that object[key] has the wrong kind of value.
func (r *PlainObjectReader) fieldError(object map[string]any, key, want string) {
	r.Fail(fmt.Errorf("plain object %v field %q: %T is not %s", object["type"], key, object[key], want))
}

// node converts a plain object node.
func (r *PlainObjectReader) node(object map[string]any) Node {
	if r.err != nil || object == nil {
		return nil
	}

	nodeType, ok := object["type"].(string)
	if !ok {
		r.fieldError(object, "type", "a string")
		return nil
	}

	fn, ok := r.types[nodeType]
	if !ok {
		r.Fail(&UnknownNodeTypeError{Type: nodeType})
		return nil
	}

	return fn(r, object)
}

// Node reads object[key] as a node; a missing key or nil is nil.
func (r *PlainObjectReader) Node(object map[string]any, key string) Node {
	switch value := object[key].(type) {
	case nil:
		return nil
	case map[string]any:
		return r.node(value)
	default:
		r.fieldError(object, key, "a node")
		return nil
	}
}

// PlainObjectNode reads object[key] as a node of type T, which a field
// holding exactly one node type needs; a missing key or nil is the zero
// T.
func PlainObjectNode[T Node](r *PlainObjectReader, object map[string]any, key string) T {
	var zero T

	node := r.Node(object, key)
	if node == nil {
		return zero
	}

	result, ok := node.(T)
	if !ok {
		r.Fail(fmt.Errorf("plain object %v field %q: node type %s is not %T", object["type"], key, node.Type(), zero))
		return zero
	}

	return result
}

// List reads object[key] as a list of nodes; a missing key or nil is nil.
func (r *PlainObjectReader) List(object map[string]any, key string) *List[Node] {
	switch value := object[key].(type) {
	case nil:
		return nil
	case []any:
		list := new(List[Node])
		for _, data := range value {
			switch data := data.(type) {
			case nil:
				list.AppendData(nil)
			case map[string]any:
				list.AppendData(r.node(data))
			default:
				r.fieldError(object, key, "a list of nodes")
				return nil
			}
		}
		return list
	default:
		r.fieldError(object, key, "a list of nodes")
		return nil
	}
}

// String reads object[key] as a string; a missing key or nil (upstream
// null) is "".
func (r *PlainObjectReader) String(object map[string]any, key string) string {
	switch value := object[key].(type) {
	case nil:
		return ""
	case string:
		return value
	default:
		r.fieldError(object, key, "a string")
		return ""
	}
}

// Location reads object["loc"]; a missing key or nil is nil.
func (r *PlainObjectReader) Location(object map[string]any) *Location {
	switch value := object["loc"].(type) {
	case nil:
		return nil
	case map[string]any:
		loc := &Location{
			Source: r.String(value, "source"),
			Start:  r.position(value, "start"),
			End:    r.position(value, "end"),
		}
		if r.err != nil {
			return nil
		}
		return loc
	default:
		r.fieldError(object, "loc", "a location")
		return nil
	}
}

func (r *PlainObjectReader) position(object map[string]any, key string) tokenizer.Position {
	value, ok := object[key].(map[string]any)
	if !ok {
		r.fieldError(object, key, "a position")
		return tokenizer.Position{}
	}

	return tokenizer.Position{
		Offset: r.int(value, "offset"),
		Line:   r.int(value, "line"),
		Column: r.int(value, "column"),
	}
}

func (r *PlainObjectReader) int(object map[string]any, key string) int {
	switch value := object[key].(type) {
	case int:
		return value
	case float64:
		if value == math.Trunc(value) && math.Abs(value) <= 1<<53 {
			return int(value)
		}
	case json.Number:
		n, err := value.Int64()
		if err == nil {
			return int(n)
		}
	}

	r.fieldError(object, key, "an integer")
	return 0
}
