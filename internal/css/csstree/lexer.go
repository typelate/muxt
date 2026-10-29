// Ported from lib/lexer/Lexer.js.

package csstree

import (
	"bytes"
	"errors"
	"iter"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/typelate/muxt/internal/css/csstree/definitionsyntax"
)

// Errors the lexer returns in MatchResult.Error where upstream uses a plain
// Error.
var (
	// ErrVarNotSupported is returned for a value containing var().
	//lint:ignore ST1005 upstream message
	ErrVarNotSupported = errors.New("Matching for a tree with var() is not supported")
	// ErrCustomProperty is returned for a custom property (--*).
	ErrCustomProperty = errors.New("Lexer matching doesn't applicable for custom properties")
	// ErrNotDeclaration is returned by MatchDeclaration for a node that is
	// not a Declaration.
	//lint:ignore ST1005 upstream message
	ErrNotDeclaration = errors.New("Not a Declaration node")
)

// AtruleSyntaxError reports an at-rule prelude or descriptor that the
// at-rule does not allow (upstream a built-in SyntaxError).
type AtruleSyntaxError struct {
	Message string
}

// Error returns Message.
func (e *AtruleSyntaxError) Error() string {
	return e.Message
}

// MatchSyntax is the syntax a match tree node matched: a definition syntax
// node (definitionsyntax.Node: Type, Property, Keyword, ...), or a
// *SyntaxRef naming the syntax at the root of a match.
type MatchSyntax interface {
	Type() string
}

// SyntaxRef names a syntax of the lexer (upstream a descriptor's ref
// {type, name}).
type SyntaxRef struct {
	// Kind is upstream type: "Type", "Property", "AtrulePrelude" or
	// "AtruleDescriptor".
	Kind string
	Name string
}

// Type returns Kind.
func (r *SyntaxRef) Type() string {
	return r.Kind
}

// syntaxName returns the name of a match syntax ("" when it has none).
func syntaxName(syntax MatchSyntax) string {
	switch syntax := syntax.(type) {
	case *SyntaxRef:
		return syntax.Name
	case *definitionsyntax.Type:
		return syntax.Name
	case *definitionsyntax.Property:
		return syntax.Name
	case *definitionsyntax.Keyword:
		return syntax.Name
	case *definitionsyntax.AtKeyword:
		return syntax.Name
	case *definitionsyntax.Function:
		return syntax.Name
	}

	return ""
}

// MatchNode is a node of a match tree. A syntax node has a non-nil Match
// (possibly empty) holding what matched inside Syntax; a token node has a
// nil Match, the Token text and the AST Node it came from (nil for a
// string value or whitespace the generator added). Syntax is nil where
// upstream has null (e.g. tokens matched by generic types).
type MatchNode struct {
	Syntax MatchSyntax
	Match  []*MatchNode
	Token  string
	Node   Node
}

// MatchResult is the result of a match (upstream buildMatchResult). Matched
// is nil and Error is set when the value does not match: a
// *SyntaxMatchError, a *SyntaxReferenceError, an *AtruleSyntaxError, one
// of the Err* values, or an error where upstream throws (a broken syntax
// definition or reference, or a node the generator does not know).
type MatchResult struct {
	Matched    *MatchNode
	Iterations int
	Error      error
}

// Descriptor is a syntax of the lexer: a type, a property, an at-rule
// prelude or an at-rule descriptor (upstream createDescriptor's result).
// Its definition syntax is parsed and its match graph built on first use;
// a Descriptor is safe for concurrent use.
type Descriptor struct {
	// Type is "Type", "Property", "AtrulePrelude" or "AtruleDescriptor".
	Type string
	Name string
	// Parent is the at-rule name of an AtruleDescriptor, else "".
	Parent string
	// Serializable reports whether the syntax has a definition syntax
	// (false for generic types).
	Serializable bool

	syntax   func() (definitionsyntax.Node, error)
	match    func() (*matchGraphNode, error)
	matchRef func() (*matchGraphNode, error)
}

// Syntax returns the definition syntax tree, parsing it on first use; it
// is nil for a generic type.
func (d *Descriptor) Syntax() (definitionsyntax.Node, error) {
	return d.syntax()
}

// AtruleSyntax is the syntax of an at-rule: its prelude (nil when it has
// none) and descriptors (nil when it has no known descriptors).
type AtruleSyntax struct {
	Name        string
	Prelude     *Descriptor
	Descriptors map[string]*Descriptor
}

// LexerDump is the lexer's configuration with syntaxes as definition
// syntax text (upstream dump()); its fields have upstream's JSON keys.
type LexerDump struct {
	Generic         bool                        `json:"generic"`
	CSSWideKeywords []string                    `json:"cssWideKeywords"`
	Units           map[string][]string         `json:"units"`
	Types           map[string]string           `json:"types"`
	Properties      map[string]string           `json:"properties"`
	Atrules         map[string]AtruleDefinition `json:"atrules"`
}

// LexerValidation lists the broken syntaxes Validate found.
type LexerValidation struct {
	Errors     []string `json:"errors"`
	Types      []string `json:"types"`
	Properties []string `json:"properties"`
}

func dumpMapSyntax(names []string, dict map[string]*Descriptor, compact bool) (map[string]string, error) {
	result := make(map[string]string)

	for _, name := range names {
		syntax, err := dict[name].Syntax()
		if err != nil {
			return nil, err
		}
		if syntax != nil {
			result[name] = definitionsyntax.Generate(syntax, definitionsyntax.GenerateOptions{Compact: compact})
		}
	}

	return result, nil
}

func dumpAtruleMapSyntax(names []string, dict map[string]*AtruleSyntax, compact bool) (map[string]AtruleDefinition, error) {
	result := make(map[string]AtruleDefinition)

	for _, name := range names {
		atrule := dict[name]
		var definition AtruleDefinition

		if atrule.Prelude != nil {
			syntax, err := atrule.Prelude.Syntax()
			if err != nil {
				return nil, err
			}
			if syntax != nil {
				definition.Prelude = definitionsyntax.Generate(syntax, definitionsyntax.GenerateOptions{Compact: compact})
			}
		}

		if atrule.Descriptors != nil {
			descriptors, err := dumpMapSyntax(sortedKeys(atrule.Descriptors), atrule.Descriptors, compact)
			if err != nil {
				return nil, err
			}
			definition.Descriptors = descriptors
		}

		result[name] = definition
	}

	return result, nil
}

func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}

func valueHasVar(tokens []matchToken) bool {
	for i := 0; i < len(tokens); i++ {
		if toLowerCase(tokens[i].value) == "var(" || tokenNameIs(&tokens[i], "var(") {
			return true
		}
	}

	return false
}

func syntaxHasTopLevelCommaMultiplier(syntax definitionsyntax.Node) bool {
	group, ok := syntax.(*definitionsyntax.Group)
	if !ok || len(group.Terms) == 0 {
		return false
	}
	singleTerm, ok := group.Terms[0].(*definitionsyntax.Multiplier)

	return !group.Explicit &&
		len(group.Terms) == 1 &&
		ok &&
		singleTerm.Comma
}

func buildMatchResult(matched *MatchNode, err error, iterations int) *MatchResult {
	return &MatchResult{
		Matched:    matched,
		Iterations: iterations,
		Error:      err,
	}
}

func matchSyntax(lexer *Lexer, syntax *Descriptor, value matchInput, useCSSWideKeywords bool) *MatchResult {
	tokens, err := prepareTokens(value, lexer.syntax)
	if err != nil {
		return buildMatchResult(nil, err, 0)
	}
	var result *matchResult

	if valueHasVar(tokens) {
		return buildMatchResult(nil, ErrVarNotSupported, 0)
	}

	if useCSSWideKeywords {
		result, err = matchAsTree(tokens, lexer.cssWideKeywordsSyntax, lexer)
		if err != nil {
			return buildMatchResult(nil, err, 0)
		}
	}

	if !useCSSWideKeywords || result.tree == nil {
		match, err := syntax.match()
		if err != nil {
			return buildMatchResult(nil, err, 0)
		}
		result, err = matchAsTree(tokens, match, lexer)
		if err != nil {
			return buildMatchResult(nil, err, 0)
		}
		if result.tree == nil {
			definition, err := syntax.Syntax()
			if err != nil {
				return buildMatchResult(nil, err, 0)
			}
			return buildMatchResult(
				nil,
				newSyntaxMatchError(result.reason, definition, value.node, result),
				result.iterations,
			)
		}
	}

	return buildMatchResult(result.tree, nil, result.iterations)
}

// Lexer matches CSS values against the syntaxes of types, properties and
// at-rules (upstream class Lexer). Its matching, checking and getter
// methods are safe for concurrent use; AddAtrule_, AddProperty_ and
// AddType_ are not, and must not run concurrently with any other method.
//
// Values to match are ASTs (a nil Node is upstream null); the methods with
// a String suffix take CSS text instead, which is tokenized rather than
// parsed (upstream takes either in the same argument).
type Lexer struct {
	cssWideKeywords []string
	syntax          *Syntax
	generic         bool
	units           map[string][]string
	atrules         map[string]*AtruleSyntax
	atruleNames     []string
	properties      map[string]*Descriptor
	propertyNames   []string
	types           map[string]*Descriptor
	typeNames       []string
	structure       map[string]*nodeStructure

	cssWideKeywordsSyntax *matchGraphNode
}

// NewLexer returns a lexer for the lexer keys of config: Generic,
// CSSWideKeywords, Units, Types, Atrules, Properties and Node (whose
// Structure gives the node structures CheckStructure checks). syntax
// generates and walks the ASTs the lexer matches; nil means the default
// syntax. It fails when a node type has no or a bad Structure, or when
// the CSS-wide keywords do not parse as a definition syntax.
//
// Config maps are read in key order, which decides the order of Types,
// Properties, Atrules and of Validate's results.
func NewLexer(config *Config, syntax *Syntax) (*Lexer, error) {
	if syntax == nil {
		syntax = defaultParseSyntax()
	}

	return newLexer(config, syntax, nil)
}

func newLexer(config *Config, syntax *Syntax, structure map[string]*nodeStructure) (*Lexer, error) {
	l := &Lexer{
		cssWideKeywords: cssWideKeywords(),
		syntax:          syntax,
		generic:         false,
		units:           defaultUnits(),
		atrules:         make(map[string]*AtruleSyntax),
		properties:      make(map[string]*Descriptor),
		types:           make(map[string]*Descriptor),
		structure:       structure,
	}
	if l.structure == nil {
		var err error
		l.structure, err = getStructureFromConfig(config)
		if err != nil {
			return nil, err
		}
	}

	if config != nil {
		if config.CSSWideKeywords != nil {
			l.cssWideKeywords = config.CSSWideKeywords
		}

		if config.Units != nil {
			for _, group := range unitGroupNames() {
				if units, ok := config.Units[group]; ok && units != nil {
					l.units[group] = units
				}
			}
		}

		if config.Types != nil {
			for _, name := range sortedKeys(config.Types) {
				l.AddType_(name, config.Types[name])
			}
		}

		if config.Generic != nil && *config.Generic {
			l.generic = true
			for _, generic := range createGenericTypes(l.units) {
				l.addGenericType(generic.name, generic.fn)
			}
		}

		if config.Atrules != nil {
			for _, name := range sortedKeys(config.Atrules) {
				l.AddAtrule_(name, config.Atrules[name])
			}
		}

		if config.Properties != nil {
			for _, name := range sortedKeys(config.Properties) {
				l.AddProperty_(name, config.Properties[name])
			}
		}
	}

	cssWideKeywordsSyntax, err := buildMatchGraphFromString(strings.Join(l.cssWideKeywords, " |  "), nil)
	if err != nil {
		return nil, err
	}
	l.cssWideKeywordsSyntax = cssWideKeywordsSyntax

	return l, nil
}

// CheckStructure checks that every node of ast has the fields its node
// type's Structure describes, with values of the allowed kinds. It checks
// the plain object form of ast (see ToPlainObject), in which a node is a
// map and a list an array; the warnings refer to those maps. It returns
// nil when the structure is correct (upstream false).
func (l *Lexer) CheckStructure(ast Node) ([]StructureWarning, error) {
	plain, err := l.syntax.ToPlainObject(ast)
	if err != nil {
		return nil, err
	}

	return l.CheckPlainObjectStructure(plain), nil
}

// CheckPlainObjectStructure is CheckStructure for an AST in its plain
// object form, such as JSON decoded into map[string]any.
func (l *Lexer) CheckPlainObjectStructure(ast map[string]any) []StructureWarning {
	structure := l.structure
	var warns []StructureWarning

	collectWarning := func(node any, message string) {
		warns = append(warns, StructureWarning{Node: node, Message: message})
	}

	l.walkPlainObject(ast, func(node any) {
		nodeType := "undefined"
		if object, ok := node.(map[string]any); ok {
			if typ, ok := object["type"]; ok {
				nodeType = jsString(typ)
			}
		}

		if nodeStructure, ok := structure[nodeType]; ok {
			nodeStructure.check(node, collectWarning)
		} else {
			collectWarning(node, "Unknown node type `"+nodeType+"`")
		}
	})

	return warns
}

// walkPlainObject calls enter for node and its descendants in the order
// the syntax's walker visits a parsed AST (upstream checkStructure uses
// syntax.walk, which walks plain objects too): the node and list fields of
// a node type's Structure, in order. Null fields and items are skipped.
func (l *Lexer) walkPlainObject(node any, enter func(node any)) {
	if node == nil {
		return
	}

	enter(node)

	object, ok := node.(map[string]any)
	if !ok {
		return
	}
	nodeType, ok := object["type"].(string)
	if !ok {
		return
	}

	for _, field := range l.syntax.config.Node[nodeType].Structure {
		isNode := false
		isList := false
		for _, fieldType := range field.Types {
			switch fieldType.Kind {
			case FieldNode:
				isNode = true
			case FieldList:
				isList = true
			}
		}

		ref := object[field.Name]
		switch {
		case isList:
			if items, ok := ref.([]any); ok {
				for _, item := range items {
					l.walkPlainObject(item, enter)
				}
			}
		case isNode:
			l.walkPlainObject(ref, enter)
		}
	}
}

// StructureDocs returns the allowed values of each field of a node type
// as text, such as "Identifier | null" (upstream
// lexer.structure[nodeType].docs), or nil for an unknown node type.
func (l *Lexer) StructureDocs(nodeType string) map[string]string {
	structure, ok := l.structure[nodeType]
	if !ok {
		return nil
	}

	return maps.Clone(structure.docs)
}

// descriptorSource is the syntax createDescriptor takes: definition syntax
// text, a definition syntax tree or a generic function (upstream a string,
// an object or a function).
type descriptorSource struct {
	text    string
	isText  bool
	tree    definitionsyntax.Node
	generic genericFunc
}

func (l *Lexer) createDescriptor(source descriptorSource, typ, name, parent string) *Descriptor {
	ref := &SyntaxRef{
		Kind: typ,
		Name: name,
	}
	descriptor := &Descriptor{
		Type:         typ,
		Name:         name,
		Parent:       parent,
		Serializable: source.isText || source.tree != nil,
	}

	if source.generic != nil {
		descriptor.syntax = func() (definitionsyntax.Node, error) { return nil, nil }
		match := buildGenericMatchGraphRoot(source.generic, ref)
		descriptor.match = func() (*matchGraphNode, error) { return match, nil }
		descriptor.matchRef = func() (*matchGraphNode, error) { return nil, nil }

		return descriptor
	}

	if source.isText {
		// lazy parsing on first access
		descriptor.syntax = sync.OnceValues(func() (definitionsyntax.Node, error) {
			group, err := definitionsyntax.Parse(source.text)
			if err != nil {
				return nil, err
			}
			return group, nil
		})
	} else {
		descriptor.syntax = func() (definitionsyntax.Node, error) { return source.tree, nil }
	}

	// lazy graph build on first access
	descriptor.match = sync.OnceValues(func() (*matchGraphNode, error) {
		syntax, err := descriptor.syntax()
		if err != nil {
			return nil, err
		}
		if syntax == nil {
			// upstream: buildMatchGraph(null) throws a TypeError
			//lint:ignore ST1005 upstream message
			return nil, errors.New("Unknown node type:")
		}
		return buildMatchGraph(syntax, ref)
	})

	descriptor.matchRef = func() (*matchGraphNode, error) { return nil, nil }
	if typ == "Property" {
		descriptor.matchRef = sync.OnceValues(func() (*matchGraphNode, error) {
			syntax, err := descriptor.syntax()
			if err != nil {
				return nil, err
			}
			if !syntaxHasTopLevelCommaMultiplier(syntax) {
				return nil, nil
			}

			group := syntax.(*definitionsyntax.Group)
			return buildMatchGraph(&definitionsyntax.Group{
				Terms:         []definitionsyntax.Node{group.Terms[0].(*definitionsyntax.Multiplier).Term},
				Combinator:    group.Combinator,
				DisallowEmpty: group.DisallowEmpty,
				Explicit:      group.Explicit,
			}, ref)
		})
	}

	return descriptor
}

// CreateDescriptor returns a descriptor of type typ ("Type", "Property",
// "AtrulePrelude" or "AtruleDescriptor") for a definition syntax, without
// adding it to the lexer; pass it to MatchDescriptor. parent is the
// at-rule of an AtruleDescriptor.
func (l *Lexer) CreateDescriptor(syntax, typ, name, parent string) *Descriptor {
	return l.createDescriptor(descriptorSource{text: syntax, isText: true}, typ, name, parent)
}

// CreateDescriptorFromTree is CreateDescriptor for a parsed definition
// syntax.
func (l *Lexer) CreateDescriptorFromTree(syntax definitionsyntax.Node, typ, name, parent string) *Descriptor {
	return l.createDescriptor(descriptorSource{tree: syntax}, typ, name, parent)
}

// AddAtrule_ adds or replaces the syntax of the at-rule name.
func (l *Lexer) AddAtrule_(name string, syntax AtruleDefinition) {
	atrule := &AtruleSyntax{
		Name: name,
	}
	if syntax.Prelude != "" {
		atrule.Prelude = l.CreateDescriptor(syntax.Prelude, "AtrulePrelude", name, "")
	}
	if syntax.Descriptors != nil {
		atrule.Descriptors = make(map[string]*Descriptor)
		for _, descName := range sortedKeys(syntax.Descriptors) {
			atrule.Descriptors[descName] = l.createDescriptor(
				descriptorSourceFromText(syntax.Descriptors[descName]),
				"AtruleDescriptor",
				descName,
				name,
			)
		}
	}

	if _, ok := l.atrules[name]; !ok {
		l.atruleNames = append(l.atruleNames, name)
	}
	l.atrules[name] = atrule
}

// descriptorSourceFromText returns the source of a syntax; "" is upstream
// null, a descriptor without syntax.
func descriptorSourceFromText(syntax string) descriptorSource {
	if syntax == "" {
		return descriptorSource{}
	}
	return descriptorSource{text: syntax, isText: true}
}

// AddProperty_ adds or replaces the syntax of the property name; an empty
// syntax is ignored.
func (l *Lexer) AddProperty_(name, syntax string) {
	if syntax == "" {
		return
	}

	if _, ok := l.properties[name]; !ok {
		l.propertyNames = append(l.propertyNames, name)
	}
	l.properties[name] = l.CreateDescriptor(syntax, "Property", name, "")
}

// AddType_ adds or replaces the syntax of the type name; an empty syntax
// is ignored.
func (l *Lexer) AddType_(name, syntax string) {
	if syntax == "" {
		return
	}

	l.addType(name, l.CreateDescriptor(syntax, "Type", name, ""))
}

func (l *Lexer) addGenericType(name string, fn genericFunc) {
	l.addType(name, l.createDescriptor(descriptorSource{generic: fn}, "Type", name, ""))
}

func (l *Lexer) addType(name string, descriptor *Descriptor) {
	if _, ok := l.types[name]; !ok {
		l.typeNames = append(l.typeNames, name)
	}
	l.types[name] = descriptor
}

// CheckAtruleName returns a *SyntaxReferenceError when the at-rule is
// unknown, else nil.
func (l *Lexer) CheckAtruleName(atruleName string) error {
	if l.GetAtrule(atruleName, true) == nil {
		return newSyntaxReferenceError("Unknown at-rule", "@"+atruleName)
	}

	return nil
}

// CheckAtrulePrelude checks that the at-rule exists and that prelude
// (nil for none) is present when, and only when, the at-rule requires
// one.
func (l *Lexer) CheckAtrulePrelude(atruleName string, prelude Node) error {
	return l.checkAtrulePrelude(atruleName, nodeInput(prelude))
}

// CheckAtrulePreludeString is CheckAtrulePrelude for prelude text ("" for
// none).
func (l *Lexer) CheckAtrulePreludeString(atruleName, prelude string) error {
	return l.checkAtrulePrelude(atruleName, cssInput(prelude))
}

func (l *Lexer) checkAtrulePrelude(atruleName string, prelude matchInput) error {
	if err := l.CheckAtruleName(atruleName); err != nil {
		return err
	}

	atrule := l.GetAtrule(atruleName, true)

	if atrule.Prelude == nil && prelude.truthy() {
		return &AtruleSyntaxError{Message: "At-rule `@" + atruleName + "` should not contain a prelude"}
	}

	if atrule.Prelude != nil && !prelude.truthy() {
		if matchSyntax(l, atrule.Prelude, cssInput(""), false).Matched == nil {
			return &AtruleSyntaxError{Message: "At-rule `@" + atruleName + "` should contain a prelude"}
		}
	}

	return nil
}

// CheckAtruleDescriptorName checks that the at-rule exists and has the
// descriptor (with or without a vendor prefix).
func (l *Lexer) CheckAtruleDescriptorName(atruleName, descriptorName string) error {
	if err := l.CheckAtruleName(atruleName); err != nil {
		return err
	}

	atrule := l.GetAtrule(atruleName, true)
	descriptor := Keyword(descriptorName)

	if atrule.Descriptors == nil {
		return &AtruleSyntaxError{Message: "At-rule `@" + atruleName + "` has no known descriptors"}
	}

	if atrule.Descriptors[descriptor.Name] == nil &&
		atrule.Descriptors[descriptor.Basename] == nil {
		return newSyntaxReferenceError("Unknown at-rule descriptor", descriptorName)
	}

	return nil
}

// CheckPropertyName returns a *SyntaxReferenceError when the property is
// unknown, else nil.
func (l *Lexer) CheckPropertyName(propertyName string) error {
	if l.GetProperty(propertyName, true) == nil {
		return newSyntaxReferenceError("Unknown property", propertyName)
	}

	return nil
}

// MatchAtrulePrelude matches an at-rule prelude (nil for none) against the
// at-rule's prelude syntax.
func (l *Lexer) MatchAtrulePrelude(atruleName string, prelude Node) *MatchResult {
	return l.matchAtrulePrelude(atruleName, nodeInput(prelude))
}

// MatchAtrulePreludeString is MatchAtrulePrelude for prelude text ("" for
// none).
func (l *Lexer) MatchAtrulePreludeString(atruleName, prelude string) *MatchResult {
	return l.matchAtrulePrelude(atruleName, cssInput(prelude))
}

func (l *Lexer) matchAtrulePrelude(atruleName string, prelude matchInput) *MatchResult {
	if err := l.checkAtrulePrelude(atruleName, prelude); err != nil {
		return buildMatchResult(nil, err, 0)
	}

	atrule := l.GetAtrule(atruleName, true)

	if atrule.Prelude == nil {
		return buildMatchResult(nil, nil, 0)
	}

	if !prelude.truthy() {
		prelude = cssInput("")
	}

	return matchSyntax(l, atrule.Prelude, prelude, false)
}

// MatchAtruleDescriptor matches the value of an at-rule descriptor.
func (l *Lexer) MatchAtruleDescriptor(atruleName, descriptorName string, value Node) *MatchResult {
	return l.matchAtruleDescriptor(atruleName, descriptorName, nodeInput(value))
}

// MatchAtruleDescriptorString is MatchAtruleDescriptor for value text.
func (l *Lexer) MatchAtruleDescriptorString(atruleName, descriptorName, value string) *MatchResult {
	return l.matchAtruleDescriptor(atruleName, descriptorName, cssInput(value))
}

func (l *Lexer) matchAtruleDescriptor(atruleName, descriptorName string, value matchInput) *MatchResult {
	if err := l.CheckAtruleDescriptorName(atruleName, descriptorName); err != nil {
		return buildMatchResult(nil, err, 0)
	}

	atrule := l.GetAtrule(atruleName, true)
	descriptor := Keyword(descriptorName)

	syntax := atrule.Descriptors[descriptor.Name]
	if syntax == nil {
		syntax = atrule.Descriptors[descriptor.Basename]
	}

	return matchSyntax(l, syntax, value, false)
}

// MatchDeclaration matches the value of a Declaration node against its
// property's syntax.
func (l *Lexer) MatchDeclaration(node Node) *MatchResult {
	declaration, ok := node.(*Declaration)
	if !ok {
		return buildMatchResult(nil, ErrNotDeclaration, 0)
	}

	return l.MatchProperty(declaration.Property, declaration.Value)
}

// MatchProperty matches value against the syntax of the property, also
// accepting the CSS-wide keywords.
func (l *Lexer) MatchProperty(propertyName string, value Node) *MatchResult {
	return l.matchProperty(propertyName, nodeInput(value))
}

// MatchPropertyString is MatchProperty for value text.
func (l *Lexer) MatchPropertyString(propertyName, value string) *MatchResult {
	return l.matchProperty(propertyName, cssInput(value))
}

func (l *Lexer) matchProperty(propertyName string, value matchInput) *MatchResult {
	// don't match syntax for a custom property at the moment
	if Property(propertyName).Custom {
		return buildMatchResult(nil, ErrCustomProperty, 0)
	}

	if err := l.CheckPropertyName(propertyName); err != nil {
		return buildMatchResult(nil, err, 0)
	}

	return matchSyntax(l, l.GetProperty(propertyName, true), value, true)
}

// MatchType matches value against the syntax of the type typeName.
func (l *Lexer) MatchType(typeName string, value Node) *MatchResult {
	return l.matchType(typeName, nodeInput(value))
}

// MatchTypeString is MatchType for value text.
func (l *Lexer) MatchTypeString(typeName, value string) *MatchResult {
	return l.matchType(typeName, cssInput(value))
}

func (l *Lexer) matchType(typeName string, value matchInput) *MatchResult {
	typeSyntax := l.GetType(typeName)

	if typeSyntax == nil {
		return buildMatchResult(nil, newSyntaxReferenceError("Unknown type", typeName), 0)
	}

	return matchSyntax(l, typeSyntax, value, false)
}

// Match matches value against a definition syntax, such as
// "fn( <number># )" (upstream match with a string syntax).
func (l *Lexer) Match(syntax string, value Node) *MatchResult {
	return l.MatchDescriptor(l.CreateDescriptor(syntax, "Type", "anonymous", ""), value)
}

// MatchString is Match for value text.
func (l *Lexer) MatchString(syntax, value string) *MatchResult {
	return l.MatchDescriptorString(l.CreateDescriptor(syntax, "Type", "anonymous", ""), value)
}

// MatchDescriptor matches value against a descriptor, such as one GetType
// returns (upstream match with a descriptor). A nil descriptor is a "Bad
// syntax" *SyntaxReferenceError.
func (l *Lexer) MatchDescriptor(syntax *Descriptor, value Node) *MatchResult {
	return l.match(syntax, nodeInput(value))
}

// MatchDescriptorString is MatchDescriptor for value text.
func (l *Lexer) MatchDescriptorString(syntax *Descriptor, value string) *MatchResult {
	return l.match(syntax, cssInput(value))
}

func (l *Lexer) match(syntax *Descriptor, value matchInput) *MatchResult {
	if syntax == nil {
		return buildMatchResult(nil, newSyntaxReferenceError("Bad syntax", ""), 0)
	}

	return matchSyntax(l, syntax, value, false)
}

// FindValueFragments returns the runs of nodes of value that matched the
// syntax named name of type typ ("Type", "Property", "Keyword", ...) when
// matching value against the property's syntax.
func (l *Lexer) FindValueFragments(propertyName string, value Node, typ, name string) ([]Fragment, error) {
	return matchFragments(l, value, l.MatchProperty(propertyName, value), typ, name)
}

// FindDeclarationValueFragments is FindValueFragments for the property
// and value of a Declaration node.
func (l *Lexer) FindDeclarationValueFragments(declaration Node, typ, name string) ([]Fragment, error) {
	var value Node
	if declaration, ok := declaration.(*Declaration); ok {
		value = declaration.Value
	}

	return matchFragments(l, value, l.MatchDeclaration(declaration), typ, name)
}

// FindAllFragments is FindDeclarationValueFragments for every declaration
// in ast.
func (l *Lexer) FindAllFragments(ast Node, typ, name string) ([]Fragment, error) {
	var result []Fragment
	var findErr error

	err := l.syntax.Walk(ast, WalkOptions{
		Visit: "Declaration",
		Enter: func(_ *WalkContext, declaration Node, _ *ListItem[Node], _ *List[Node]) WalkAction {
			fragments, err := l.FindDeclarationValueFragments(declaration, typ, name)
			if err != nil {
				findErr = err
				return WalkBreak
			}
			result = append(result, fragments...)
			return WalkContinue
		},
	})
	if err != nil {
		return nil, err
	}
	if findErr != nil {
		return nil, findErr
	}

	return result, nil
}

// GetAtrule returns the syntax of an at-rule, nil when it is unknown. With
// fallbackBasename (upstream's default), a vendor-prefixed name falls back
// to the name without the prefix.
func (l *Lexer) GetAtrule(atruleName string, fallbackBasename bool) *AtruleSyntax {
	atrule := Keyword(atruleName)
	var atruleEntry *AtruleSyntax
	if atrule.Vendor != "" && fallbackBasename {
		atruleEntry = l.atrules[atrule.Name]
		if atruleEntry == nil {
			atruleEntry = l.atrules[atrule.Basename]
		}
	} else {
		atruleEntry = l.atrules[atrule.Name]
	}

	return atruleEntry
}

// GetAtrulePrelude returns the prelude syntax of an at-rule, nil when the
// at-rule is unknown or has no prelude.
func (l *Lexer) GetAtrulePrelude(atruleName string, fallbackBasename bool) *Descriptor {
	atrule := l.GetAtrule(atruleName, fallbackBasename)

	if atrule == nil {
		return nil
	}

	return atrule.Prelude
}

// GetAtruleDescriptor returns nil. Upstream calls hasOwnProperty on its
// prototype-less at-rule map, which throws a TypeError, and would read
// `this.atrules.declarators`, a field that does not exist, after it; the
// method is kept for the API (see AtruleSyntax Descriptors instead).
func (l *Lexer) GetAtruleDescriptor(atruleName, name string) *Descriptor {
	return nil
}

// GetProperty returns the syntax of a property, nil when it is unknown.
// With fallbackBasename (upstream's default), a vendor-prefixed name falls
// back to the name without the prefix.
func (l *Lexer) GetProperty(propertyName string, fallbackBasename bool) *Descriptor {
	property := Property(propertyName)
	var propertyEntry *Descriptor
	if property.Vendor != "" && fallbackBasename {
		propertyEntry = l.properties[property.Name]
		if propertyEntry == nil {
			propertyEntry = l.properties[property.Basename]
		}
	} else {
		propertyEntry = l.properties[property.Name]
	}

	return propertyEntry
}

// GetType returns the syntax of a type, nil when it is unknown.
func (l *Lexer) GetType(name string) *Descriptor {
	return l.types[name]
}

// Validate checks that every syntax refers only to types and properties
// that exist and are valid themselves. It returns nil when all are valid
// (upstream null), and an error when a syntax does not parse.
func (l *Lexer) Validate() (*LexerValidation, error) {
	syntaxRef := func(name string, isType bool) string {
		if isType {
			return "<" + name + ">"
		}
		return "<'" + name + "'>"
	}

	var errs []string
	brokenTypes := newBrokenMap()
	brokenProperties := newBrokenMap()

	var validate func(name string, broken *brokenMap, descriptor *Descriptor) (bool, error)
	validate = func(name string, broken *brokenMap, descriptor *Descriptor) (bool, error) {
		if value, ok := broken.get(name); ok {
			return value, nil
		}

		broken.set(name, false)
		syntax, err := descriptor.Syntax()
		if err != nil {
			return false, err
		}
		if syntax != nil {
			var walkErr error
			err := definitionsyntax.Walk(syntax, definitionsyntax.WalkOptions{
				Enter: func(node definitionsyntax.Node) {
					if walkErr != nil {
						return
					}

					var nodeName string
					switch node := node.(type) {
					case *definitionsyntax.Type:
						nodeName = node.Name
					case *definitionsyntax.Property:
						nodeName = node.Name
					default:
						return
					}
					isType := node.Type() == "Type"

					dict := l.properties
					brokenDict := brokenProperties
					if isType {
						dict = l.types
						brokenDict = brokenTypes
					}

					if _, ok := dict[nodeName]; !ok {
						errs = append(errs, syntaxRef(name, broken == brokenTypes)+" used missed syntax definition "+syntaxRef(nodeName, isType))
						broken.set(name, true)
					} else {
						isBroken, err := validate(nodeName, brokenDict, dict[nodeName])
						if err != nil {
							walkErr = err
							return
						}
						if isBroken {
							errs = append(errs, syntaxRef(name, broken == brokenTypes)+" used broken syntax definition "+syntaxRef(nodeName, isType))
							broken.set(name, true)
						}
					}
				},
			})
			if err != nil {
				return false, err
			}
			if walkErr != nil {
				return false, walkErr
			}
		}

		// upstream returns undefined here: a name being validated is not
		// (yet) broken for the syntaxes that refer to it
		return false, nil
	}

	for _, key := range l.typeNames {
		if _, err := validate(key, brokenTypes, l.types[key]); err != nil {
			return nil, err
		}
	}

	for _, key := range l.propertyNames {
		if _, err := validate(key, brokenProperties, l.properties[key]); err != nil {
			return nil, err
		}
	}

	brokenTypesArray := brokenTypes.brokenNames()
	brokenPropertiesArray := brokenProperties.brokenNames()

	if len(brokenTypesArray) > 0 || len(brokenPropertiesArray) > 0 {
		return &LexerValidation{
			Errors:     errs,
			Types:      brokenTypesArray,
			Properties: brokenPropertiesArray,
		}, nil
	}

	return nil, nil
}

// brokenMap is the Map Validate keeps per dictionary: names in insertion
// order with whether they are broken.
type brokenMap struct {
	names  []string
	broken map[string]bool
}

func newBrokenMap() *brokenMap {
	return &brokenMap{broken: make(map[string]bool)}
}

func (m *brokenMap) get(name string) (bool, bool) {
	value, ok := m.broken[name]
	return value, ok
}

func (m *brokenMap) set(name string, value bool) {
	if _, ok := m.broken[name]; !ok {
		m.names = append(m.names, name)
	}
	m.broken[name] = value
}

func (m *brokenMap) brokenNames() []string {
	var result []string
	for _, name := range m.names {
		if m.broken[name] {
			result = append(result, name)
		}
	}
	return result
}

// Dump returns the lexer's configuration with its syntaxes generated as
// definition syntax text, compact unless pretty (upstream dump(false,
// pretty)). Generic types, which have no definition syntax, are left out.
// It fails when a syntax does not parse.
func (l *Lexer) Dump(pretty bool) (*LexerDump, error) {
	types, err := dumpMapSyntax(l.typeNames, l.types, !pretty)
	if err != nil {
		return nil, err
	}
	properties, err := dumpMapSyntax(l.propertyNames, l.properties, !pretty)
	if err != nil {
		return nil, err
	}
	atrules, err := dumpAtruleMapSyntax(l.atruleNames, l.atrules, !pretty)
	if err != nil {
		return nil, err
	}

	return &LexerDump{
		Generic:         l.generic,
		CSSWideKeywords: slices.Clone(l.cssWideKeywords),
		Units:           l.Units(),
		Types:           types,
		Properties:      properties,
		Atrules:         atrules,
	}, nil
}

// String returns the JSON of Dump(false) (upstream toString), or the
// error text when a syntax does not parse.
func (l *Lexer) String() string {
	dump, err := l.Dump(false)
	if err != nil {
		return err.Error()
	}

	var buf bytes.Buffer
	if err := writeJSON(&buf, dump); err != nil {
		return err.Error()
	}

	return buf.String()
}

// Generic reports whether the lexer has the generic types (upstream
// lexer.generic).
func (l *Lexer) Generic() bool {
	return l.generic
}

// CSSWideKeywords returns the CSS-wide keywords every property accepts.
func (l *Lexer) CSSWideKeywords() []string {
	return slices.Clone(l.cssWideKeywords)
}

// Units returns the units of each unit group (upstream lexer.units).
func (l *Lexer) Units() map[string][]string {
	units := make(map[string][]string, len(l.units))
	for group, list := range l.units {
		units[group] = slices.Clone(list)
	}
	return units
}

// Types returns the types in the order they were added.
func (l *Lexer) Types() iter.Seq2[string, *Descriptor] {
	return func(yield func(string, *Descriptor) bool) {
		for _, name := range l.typeNames {
			if !yield(name, l.types[name]) {
				return
			}
		}
	}
}

// Properties returns the properties in the order they were added.
func (l *Lexer) Properties() iter.Seq2[string, *Descriptor] {
	return func(yield func(string, *Descriptor) bool) {
		for _, name := range l.propertyNames {
			if !yield(name, l.properties[name]) {
				return
			}
		}
	}
}

// Atrules returns the at-rules in the order they were added.
func (l *Lexer) Atrules() iter.Seq2[string, *AtruleSyntax] {
	return func(yield func(string, *AtruleSyntax) bool) {
		for _, name := range l.atruleNames {
			if !yield(name, l.atrules[name]) {
				return
			}
		}
	}
}

// lookupMatch implements matchSyntaxes.
func (l *Lexer) lookupMatch(isType bool, name string) (*matchGraphNode, error) {
	dict := l.properties
	if isType {
		dict = l.types
	}

	dictSyntax, ok := dict[name]
	if !ok || dictSyntax == nil {
		return nil, nil
	}

	match, err := dictSyntax.match()
	if err != nil || match == nil {
		return nil, err
	}

	matchRef, err := dictSyntax.matchRef()
	if err != nil {
		return nil, err
	}
	if matchRef != nil {
		return matchRef, nil
	}

	return match, nil
}
