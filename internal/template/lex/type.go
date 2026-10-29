package lex

import "fmt"

// TokenType identifies the type of lex items.
type TokenType int

const (
	TokenTypeError        TokenType = iota // error occurred; value is text of error
	TokenTypeBool                          // boolean constant
	TokenTypeChar                          // printable ASCII character; grab bag for comma etc.
	TokenTypeCharConstant                  // character constant
	TokenTypeComment                       // comment text
	TokenTypeComplex                       // complex constant (1+2i); imaginary is just a number
	TokenTypeAssign                        // equals ('=') introducing an assignment
	TokenTypeDeclare                       // colon-equals (':=') introducing a declaration
	TokenTypeEOF
	TokenTypeField      // alphanumeric identifier starting with '.'
	TokenTypeIdentifier // alphanumeric identifier not starting with '.'
	TokenTypeLeftDelim  // left action delimiter
	TokenTypeLeftParen  // '(' inside action
	TokenTypeNumber     // simple number, including imaginary
	TokenTypePipe       // pipe symbol
	TokenTypeRawString  // raw quoted string (includes quotes)
	TokenTypeRightDelim // right action delimiter
	TokenTypeRightParen // ')' inside action
	TokenTypeSpace      // run of spaces separating arguments
	TokenTypeString     // quoted string (includes quotes)
	TokenTypeText       // plain text
	TokenTypeVariable   // variable starting with '$', such as '$' or  '$1' or '$hello'
	// Keywords appear after all the rest.
	TokenTypeKeyword  // used only to delimit the keywords
	TokenTypeBlock    // block keyword
	TokenTypeBreak    // break keyword
	TokenTypeContinue // continue keyword
	TokenTypeDot      // the cursor, spelled '.'
	TokenTypeDefine   // define keyword
	TokenTypeElse     // else keyword
	TokenTypeEnd      // end keyword
	TokenTypeIf       // if keyword
	TokenTypeNil      // the untyped nil constant, easiest to treat as a keyword
	TokenTypeRange    // range keyword
	TokenTypeTemplate // template keyword
	TokenTypeWith     // with keyword
)

var key = map[string]TokenType{
	".":        TokenTypeDot,
	"block":    TokenTypeBlock,
	"break":    TokenTypeBreak,
	"continue": TokenTypeContinue,
	"define":   TokenTypeDefine,
	"else":     TokenTypeElse,
	"end":      TokenTypeEnd,
	"if":       TokenTypeIf,
	"range":    TokenTypeRange,
	"nil":      TokenTypeNil,
	"template": TokenTypeTemplate,
	"with":     TokenTypeWith,
}

var tokenName = map[TokenType]string{
	TokenTypeError:        "error",
	TokenTypeBool:         "bool",
	TokenTypeChar:         "char",
	TokenTypeCharConstant: "charconst",
	TokenTypeComment:      "comment",
	TokenTypeComplex:      "complex",
	TokenTypeDeclare:      ":=",
	TokenTypeEOF:          "EOF",
	TokenTypeField:        "field",
	TokenTypeIdentifier:   "identifier",
	TokenTypeLeftDelim:    "left delim",
	TokenTypeLeftParen:    "(",
	TokenTypeNumber:       "number",
	TokenTypePipe:         "pipe",
	TokenTypeRawString:    "raw string",
	TokenTypeRightDelim:   "right delim",
	TokenTypeRightParen:   ")",
	TokenTypeSpace:        "space",
	TokenTypeString:       "string",
	TokenTypeVariable:     "variable",

	// keywords
	TokenTypeDot:      ".",
	TokenTypeBlock:    "block",
	TokenTypeBreak:    "break",
	TokenTypeContinue: "continue",
	TokenTypeDefine:   "define",
	TokenTypeElse:     "else",
	TokenTypeIf:       "if",
	TokenTypeEnd:      "end",
	TokenTypeNil:      "nil",
	TokenTypeRange:    "range",
	TokenTypeTemplate: "template",
	TokenTypeWith:     "with",

	TokenTypeHTMLText:         "HTMLText",
	TokenTypeHTMLStartTag:     "HTMLStartTag",
	TokenTypeHTMLEndTag:       "HTMLEndTag",
	TokenTypeHTMLTagOpen:      "HTMLTagOpen",
	TokenTypeHTMLTagEnd:       "HTMLTagEnd",
	TokenTypeHTMLSelfClose:    "HTMLSelfClose",
	TokenTypeHTMLEndTagOpen:   "HTMLEndTagOpen",
	TokenTypeHTMLAttrName:     "HTMLAttrName",
	TokenTypeHTMLAttrEquals:   "HTMLAttrEquals",
	TokenTypeHTMLAttrValue:    "HTMLAttrValue",
	TokenTypeHTMLComment:      "HTMLComment",
	TokenTypeHTMLCommentOpen:  "HTMLCommentOpen",
	TokenTypeHTMLCommentClose: "HTMLCommentClose",
	TokenTypeHTMLDoctype:      "HTMLDoctype",
}

func (i TokenType) String() string {
	s := tokenName[i]
	if s == "" {
		return fmt.Sprintf("TokenType(%d)", int(i))
	}
	return s
}
