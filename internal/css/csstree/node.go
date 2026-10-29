// Ported from lib/syntax/node/index.js and the node type definition shape
// (name, structure, walkContext, generate) shared by lib/syntax/node/*.js.

package csstree

import (
	"bytes"
	"encoding/json"

	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

// Node is a CSS AST node: a pointer to one of the node structs of this
// package (StyleSheet, Rule, Selector, ...) or to a node type added by an
// extension.
type Node interface {
	// Type returns upstream node.type, e.g. "Selector".
	Type() string
	// Location returns upstream node.loc, nil when positions are off.
	Location() *Location
}

// Location is a node's range in the source (upstream node.loc).
type Location = tokenizer.Location

// FieldKind is the kind of one alternative of a node structure field.
type FieldKind int

const (
	// FieldString is upstream String: the field holds a string.
	FieldString FieldKind = iota + 1
	// FieldBoolean is upstream Boolean: the field holds a boolean.
	FieldBoolean
	// FieldNull is upstream null: the field may be null.
	FieldNull
	// FieldNode is a node type name: the field holds a node of type FieldType.Node.
	FieldNode
	// FieldList is upstream [[...]]: the field holds a list of nodes whose
	// types are in FieldType.List (any type when empty).
	FieldList
)

// FieldType is one alternative of a node structure field.
type FieldType struct {
	Kind FieldKind
	// Node is the node type for FieldNode.
	Node string
	// List holds the allowed item node types for FieldList; empty means any.
	List []string
}

// StructureField describes one field of a node type (upstream
// structure[name] = [...types]). Fields are kept in upstream order.
type StructureField struct {
	Name  string
	Types []FieldType
}

// GenerateFunc writes node to g (upstream generate(node) with this bound to
// the generator's public API).
type GenerateFunc func(g *Generator, node Node)

// WalkFieldsFunc calls walkNode for each non-nil node field and walkList
// for each non-nil list field of node, in structure order or, when reverse
// is set, in reverse structure order. It returns true as soon as walkNode
// or walkList does (upstream: the type iterator the walker builds from
// structure by reading node[field.name]).
type WalkFieldsFunc func(node Node, reverse bool, walkNode func(Node) bool, walkList func(*List[Node]) bool) bool

// CloneFunc returns a deep copy of node, copying its child nodes with
// clone (upstream clone copies any object generically).
type CloneFunc func(node Node, clone func(Node) Node) Node

// FromPlainObjectFunc builds a node from its plain object form (upstream
// fromPlainObject turns the arrays of a JSON-like AST back into Lists in
// place). Fields are read with the methods of r, which records the first
// error.
type FromPlainObjectFunc func(r *PlainObjectReader, object map[string]any) Node

// NodeConfig describes a node type (upstream config.node[name]: the exports
// of lib/syntax/node/<Name>.js except parse, which is a *Parser method).
//
// Walk, Clone and FromPlainObject are Go-only: upstream reads and writes
// node fields by name, which a typed AST cannot do without reflection, so
// each node type supplies these functions. A node type added through
// Syntax.Fork supplies its own; without them the walker does not descend
// into its nodes, Clone shares them and FromPlainObject rejects them.
type NodeConfig struct {
	Name        string
	Structure   []StructureField
	WalkContext string
	Generate    GenerateFunc

	Walk            WalkFieldsFunc
	Clone           CloneFunc
	FromPlainObject FromPlainObjectFunc
}

// nodeConfigs returns every node type in upstream index.js order.
func nodeConfigs() map[string]NodeConfig {
	result := make(map[string]NodeConfig)
	for _, nodeType := range []NodeConfig{
		anPlusBNodeType(),
		atruleNodeType(),
		atrulePreludeNodeType(),
		attributeSelectorNodeType(),
		blockNodeType(),
		bracketsNodeType(),
		cdcNodeType(),
		cdoNodeType(),
		classSelectorNodeType(),
		combinatorNodeType(),
		commentNodeType(),
		conditionNodeType(),
		declarationNodeType(),
		declarationListNodeType(),
		dimensionNodeType(),
		featureNodeType(),
		featureFunctionNodeType(),
		featureRangeNodeType(),
		functionNodeType(),
		generalEnclosedNodeType(),
		hashNodeType(),
		identifierNodeType(),
		idSelectorNodeType(),
		layerNodeType(),
		layerListNodeType(),
		mediaQueryNodeType(),
		mediaQueryListNodeType(),
		nestingSelectorNodeType(),
		nthNodeType(),
		numberNodeType(),
		operatorNodeType(),
		parenthesesNodeType(),
		percentageNodeType(),
		pseudoClassSelectorNodeType(),
		pseudoElementSelectorNodeType(),
		ratioNodeType(),
		rawNodeType(),
		ruleNodeType(),
		scopeNodeType(),
		selectorNodeType(),
		selectorListNodeType(),
		stringNodeType(),
		styleSheetNodeType(),
		supportsDeclarationNodeType(),
		typeSelectorNodeType(),
		unicodeRangeNodeType(),
		urlNodeType(),
		valueNodeType(),
		whiteSpaceNodeType(),
	} {
		result[nodeType.Name] = nodeType
	}
	return result
}

// structure field helpers

func stringField() FieldType  { return FieldType{Kind: FieldString} }
func booleanField() FieldType { return FieldType{Kind: FieldBoolean} }
func nullField() FieldType    { return FieldType{Kind: FieldNull} }

func nodeField(nodeType string) FieldType {
	return FieldType{Kind: FieldNode, Node: nodeType}
}

func listField(nodeTypes ...string) FieldType {
	return FieldType{Kind: FieldList, List: nodeTypes}
}

// JSON encoding

// jsonField is one field of a node's JSON object after "type" and "loc".
type jsonField struct {
	name  string
	value any
}

// marshalNode encodes a node as upstream JSON.stringify does: "type" first,
// then "loc", then fields in structure order.
func marshalNode(node Node, fields ...jsonField) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(`{"type":`)
	if err := writeJSON(&buf, node.Type()); err != nil {
		return nil, err
	}
	buf.WriteString(`,"loc":`)
	if err := writeJSON(&buf, node.Location()); err != nil {
		return nil, err
	}
	for _, field := range fields {
		buf.WriteByte(',')
		if err := writeJSON(&buf, field.name); err != nil {
			return nil, err
		}
		buf.WriteByte(':')
		if err := writeJSON(&buf, field.value); err != nil {
			return nil, err
		}
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// writeJSON appends the JSON encoding of value without escaping <, > and &,
// as JSON.stringify does not.
func writeJSON(buf *bytes.Buffer, value any) error {
	if list, ok := value.(*List[Node]); ok && list != nil {
		// List.MarshalJSON uses json.Marshal, which escapes HTML
		buf.WriteByte('[')
		for item := list.Head(); item != nil; item = item.Next {
			if item != list.Head() {
				buf.WriteByte(',')
			}
			if err := writeJSON(buf, item.Data); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
		return nil
	}

	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return err
	}
	buf.Truncate(buf.Len() - 1) // Encode appends a newline
	return nil
}

// nullableString returns nil for "" (upstream null) and s otherwise.
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
