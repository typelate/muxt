// Ported from lib/syntax/create.js, lib/syntax/index.js and lib/index.js.

package csstree

import (
	"sync"

	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

// Syntax is a CSS syntax: the parser, generator, walker, convertor and
// lexer of one configuration (upstream the object createSyntax returns).
// A Syntax is safe for concurrent use; the ASTs it returns are not.
type Syntax struct {
	// Lexer is upstream syntax.lexer: the lexer of the lexer keys of the
	// config (Generic, CSSWideKeywords, Units, Types, Atrules, Properties,
	// Node).
	Lexer *Lexer

	config    *Config
	parse     ParseFunc
	walker    *Walker
	generate  func(node Node, options GenerateOptions) (string, error)
	convertor *Convertor
	clone     func(node Node) Node
}

// SyntaxExtension is what Fork mixes into a copy of a syntax's config:
// a *Config, whose set fields are mixed in as upstream mix does, or an
// ExtendFunc, which changes the copy and returns the config to use
// (upstream fork(extension) takes an object or a function).
type SyntaxExtension interface {
	extend(base *Config) *Config
}

func (c *Config) extend(base *Config) *Config {
	return mix(base, c)
}

// ExtendFunc is the function form of a SyntaxExtension: it receives a
// copy of the syntax's config, which it may change, and returns the
// config of the new syntax.
type ExtendFunc func(base *Config) *Config

func (fn ExtendFunc) extend(base *Config) *Config {
	return fn(base)
}

func createSyntax(config *Config) (*Syntax, error) {
	if config == nil {
		config = new(Config)
	}
	parse := CreateParser(config)
	walker, err := CreateWalker(config)
	if err != nil {
		return nil, err
	}
	generate := CreateGenerator(config)
	convertor := CreateConvertor(config)

	syntax := &Syntax{
		config:    config,
		parse:     parse,
		walker:    walker,
		generate:  generate,
		convertor: convertor,
		clone:     cloneWith(cloneTypesFromConfig(config)),
	}

	lexer, err := newLexer(&Config{
		Generic:         config.Generic,
		CSSWideKeywords: config.CSSWideKeywords,
		Units:           config.Units,
		Types:           config.Types,
		Atrules:         config.Atrules,
		Properties:      config.Properties,
		Node:            config.Node,
	}, syntax, nil)
	if err != nil {
		return nil, err
	}
	syntax.Lexer = lexer

	return syntax, nil
}

// CreateLexer returns a new lexer for the lexer keys of config, sharing
// this syntax and its node structures (upstream createLexer). It fails when
// the CSS-wide keywords do not parse as a definition syntax.
func (s *Syntax) CreateLexer(config *Config) (*Lexer, error) {
	return newLexer(config, s, s.Lexer.structure)
}

// CreateSyntax returns the syntax of a copy of config (upstream
// createSyntax). It fails when a node type has no Structure or a bad one,
// or when the CSS-wide keywords do not parse as a definition syntax.
func CreateSyntax(config *Config) (*Syntax, error) {
	return createSyntax(mix(nil, config))
}

// Tokenize splits source into tokens, calling onToken for each.
func (s *Syntax) Tokenize(source string, onToken func(tokenType tokenizer.TokenType, start, end int)) {
	tokenizer.Tokenize(source, onToken)
}

// Parse parses source into an AST.
func (s *Syntax) Parse(source string, options ParseOptions) (Node, error) {
	return s.parse(source, options)
}

// Generate turns an AST into CSS text.
func (s *Syntax) Generate(node Node, options GenerateOptions) (string, error) {
	return s.generate(node, options)
}

// Walk traverses an AST (see Walker.Walk).
func (s *Syntax) Walk(root Node, options WalkOptions) error {
	return s.walker.Walk(root, options)
}

// Find returns the first node for which fn returns true, or nil (see
// Walker.Find).
func (s *Syntax) Find(ast Node, fn func(context *WalkContext, node Node, item *ListItem[Node], list *List[Node]) bool) Node {
	return s.walker.Find(ast, fn)
}

// FindLast returns the last node for which fn returns true, or nil (see
// Walker.FindLast).
func (s *Syntax) FindLast(ast Node, fn func(context *WalkContext, node Node, item *ListItem[Node], list *List[Node]) bool) Node {
	return s.walker.FindLast(ast, fn)
}

// FindAll returns every node for which fn returns true (see
// Walker.FindAll).
func (s *Syntax) FindAll(ast Node, fn func(context *WalkContext, node Node, item *ListItem[Node], list *List[Node]) bool) []Node {
	return s.walker.FindAll(ast, fn)
}

// FromPlainObject builds an AST from its plain object form (see
// Convertor.FromPlainObject).
func (s *Syntax) FromPlainObject(ast map[string]any) (Node, error) {
	return s.convertor.FromPlainObject(ast)
}

// ToPlainObject returns the plain object form of an AST (see
// Convertor.ToPlainObject).
func (s *Syntax) ToPlainObject(ast Node) (map[string]any, error) {
	return s.convertor.ToPlainObject(ast)
}

// Clone returns a deep copy of node, copying nodes of every type of the
// syntax that has a Clone function (Go only; see Clone).
func (s *Syntax) Clone(node Node) Node {
	return s.clone(node)
}

// Fork returns a new syntax whose config is a copy of this syntax's
// config extended by extension (a *Config or an ExtendFunc). The syntax
// it is forked from is not affected. A nil extension forks an unchanged
// copy.
func (s *Syntax) Fork(extension SyntaxExtension) (*Syntax, error) {
	base := mix(nil, s.config) // copy of config

	if extension == nil {
		return createSyntax(base)
	}

	return createSyntax(extension.extend(base))
}

// defaultSyntax is upstream lib/syntax/index.js:
// createSyntax({ ...lexerConfig, ...parserConfig, ...walkerConfig }).
var defaultSyntax = sync.OnceValue(func() *Syntax {
	lexer, err := lexerConfig()
	if err != nil {
		// the embedded data is decoded by the tests
		panic("csstree: default syntax: " + err.Error())
	}

	return createDefaultSyntax(lexer)
})

// defaultParseSyntax is defaultSyntax without the definitions of data.go
// (types, properties, at-rules): the package-level Parse, Generate, Walk,
// Find, FindLast, FindAll, ToPlainObject, FromPlainObject and Tokenize use
// it, so that a program that only parses does not decode mdn-data. It
// parses, generates and walks exactly as defaultSyntax does.
var defaultParseSyntax = sync.OnceValue(func() *Syntax {
	return createDefaultSyntax(lexerConfigWithoutDefinitions())
})

func createDefaultSyntax(lexer *Config) *Syntax {
	parser := parserConfig()
	walker := walkerConfig()

	syntax, err := CreateSyntax(&Config{
		ParseContext: parser.ParseContext,
		Features:     parser.Features,
		Scope:        parser.Scope,
		Atrule:       parser.Atrule,
		Pseudo:       parser.Pseudo,
		Node:         walker.Node,

		Generic:         lexer.Generic,
		CSSWideKeywords: lexer.CSSWideKeywords,
		Units:           lexer.Units,
		Types:           lexer.Types,
		Atrules:         lexer.Atrules,
		Properties:      lexer.Properties,
	})
	if err != nil {
		// every node type of this package has a valid structure
		panic("csstree: default syntax: " + err.Error())
	}

	return syntax
}

// DefaultSyntax returns the default syntax, with the lexer of the CSS
// definitions of mdn-data (decoded on first use). The package-level
// functions (Parse, Generate, Walk, ...) behave as its methods do.
func DefaultSyntax() *Syntax {
	return defaultSyntax()
}

// DefaultLexer returns the lexer of the default syntax (upstream lexer).
func DefaultLexer() *Lexer {
	return defaultSyntax().Lexer
}

// CreateLexer returns a new lexer for the lexer keys of config with the
// default syntax (see Syntax.CreateLexer).
func CreateLexer(config *Config) (*Lexer, error) {
	return defaultSyntax().CreateLexer(config)
}

// Tokenize splits source into tokens with the default syntax.
func Tokenize(source string, onToken func(tokenType tokenizer.TokenType, start, end int)) {
	defaultParseSyntax().Tokenize(source, onToken)
}

// Walk traverses an AST with the default syntax (see Walker.Walk).
func Walk(root Node, options WalkOptions) error {
	return defaultParseSyntax().Walk(root, options)
}

// Find returns the first node for which fn returns true, or nil, with the
// default syntax.
func Find(ast Node, fn func(context *WalkContext, node Node, item *ListItem[Node], list *List[Node]) bool) Node {
	return defaultParseSyntax().Find(ast, fn)
}

// FindLast returns the last node for which fn returns true, or nil, with
// the default syntax.
func FindLast(ast Node, fn func(context *WalkContext, node Node, item *ListItem[Node], list *List[Node]) bool) Node {
	return defaultParseSyntax().FindLast(ast, fn)
}

// FindAll returns every node for which fn returns true, with the default
// syntax.
func FindAll(ast Node, fn func(context *WalkContext, node Node, item *ListItem[Node], list *List[Node]) bool) []Node {
	return defaultParseSyntax().FindAll(ast, fn)
}

// FromPlainObject builds an AST from its plain object form with the
// default syntax.
func FromPlainObject(ast map[string]any) (Node, error) {
	return defaultParseSyntax().FromPlainObject(ast)
}

// ToPlainObject returns the plain object form of an AST with the default
// syntax.
func ToPlainObject(ast Node) (map[string]any, error) {
	return defaultParseSyntax().ToPlainObject(ast)
}

// Fork returns a new syntax extending the default syntax (see
// Syntax.Fork).
func Fork(extension SyntaxExtension) (*Syntax, error) {
	return defaultSyntax().Fork(extension)
}
