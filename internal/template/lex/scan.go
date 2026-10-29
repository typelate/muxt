// Package lex provides the template and HTML lexer, source positions, and
// parse options used by the muxt template parser.
package lex

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const eof = -1

// Trimming spaces.
// If the action begins "{{- " rather than "{{", then all space/tab/newlines
// preceding the action are trimmed; conversely if it ends " -}}" the
// leading spaces are trimmed. This is done entirely in the lexer; the
// parser never sees it happen. We require an ASCII space (' ', \t, \r, \n)
// to be present to avoid ambiguity with things like "{{-3}}". It reads
// better with the space present anyway. For simplicity, only ASCII
// does the job.
const (
	trimMarker    = '-'        // Attached to left/right delimiter, trims trailing spaces from preceding/following text.
	trimMarkerLen = Pos(1 + 1) // marker plus space before or after
)

// stateFn represents the state of the scanner as a function that returns the next state.
type stateFn func(*Scanner) stateFn

// Scanner holds the state of the scanner.
type Scanner struct {
	input           string // the string being scanned
	pos             Pos    // current position in the input
	start           Pos    // start position of this item
	atEOF           bool   // we have hit the end of input and returned eof
	parenDepth      int    // nesting depth of ( ) exprs
	line            int    // 1+number of newlines seen
	startLine       int    // start line of this item
	item            Token  // item to return to parser
	insideAction    bool   // are we inside an action?
	Options         Options
	htmlReturnState stateFn // HTML state to resume after a {{ ... }} action
	htmlRawTag      string  // lowercased tag name when inside a raw text element; empty otherwise
	commentTrimLeft bool    // left trim marker was present on comment's opening delimiter
}

// Options control behavior of the lexer. All default to false.
type Options struct {
	EmitComment  bool // emit TokenTypeComment tokens.
	BreakOK      bool // break keyword allowed
	ContinueOK   bool // continue keyword allowed
	ParseHTML    bool
	LeftDelim    string // start of action marker
	RightDelim   string // end of action marker
	MaxHTMLDepth int    // maximum nesting depth for HTML elements; 0 means use default (1024)
}

func NewOptions(funcMaps ...map[string]any) Options {
	return Options{
		EmitComment:  true,
		ContinueOK:   !hasFunction("continue", funcMaps...),
		BreakOK:      !hasFunction("break", funcMaps...),
		LeftDelim:    "{{",
		RightDelim:   "}}",
		ParseHTML:    true,
		MaxHTMLDepth: 1024,
	}
}

func hasFunction(name string, funcMaps ...map[string]any) bool {
	for _, funcMap := range funcMaps {
		if funcMap == nil {
			continue
		}
		if funcMap[name] != nil {
			return true
		}
	}
	return false
}

func (l *Scanner) next() rune {
	if int(l.pos) >= len(l.input) {
		l.atEOF = true
		return eof
	}
	r, w := utf8.DecodeRuneInString(l.input[l.pos:])
	l.pos += Pos(w)
	if r == '\n' {
		l.line++
	}
	return r
}

func (l *Scanner) peek() rune {
	r := l.next()
	l.backup()
	return r
}

func (l *Scanner) backup() {
	if !l.atEOF && l.pos > 0 {
		r, w := utf8.DecodeLastRuneInString(l.input[:l.pos])
		l.pos -= Pos(w)
		// Correct newline count.
		if r == '\n' {
			l.line--
		}
	}
}

func (l *Scanner) thisItem(t TokenType) Token {
	i := Token{Type: t, Pos: l.start, End: l.pos, Value: l.input[l.start:l.pos], Line: l.startLine}
	l.start = l.pos
	l.startLine = l.line
	return i
}

func (l *Scanner) emit(t TokenType) stateFn {
	return l.emitItem(l.thisItem(t))
}

func (l *Scanner) emitItem(i Token) stateFn {
	l.item = i
	return nil
}

// ignore skips over the pending input before this point.
// It tracks newlines in the ignored text, so use it only
// for text that is skipped without calling l.next.
func (l *Scanner) ignore() {
	l.line += strings.Count(l.input[l.start:l.pos], "\n")
	l.start = l.pos
	l.startLine = l.line
}

func (l *Scanner) accept(valid string) bool {
	if strings.ContainsRune(valid, l.next()) {
		return true
	}
	l.backup()
	return false
}

func (l *Scanner) acceptRun(valid string) {
	for strings.ContainsRune(valid, l.next()) {
	}
	l.backup()
}

// errorf returns an error token and terminates the scan by passing
// back a nil pointer that will be the next state, terminating l.nextItem.
func (l *Scanner) errorf(format string, args ...any) stateFn {
	l.item = Token{Type: TokenTypeError, Pos: l.start, End: l.pos, Value: fmt.Sprintf(format, args...), Line: l.startLine}
	l.start = 0
	l.pos = 0
	l.input = l.input[:0]
	return nil
}

func (l *Scanner) Next() Token {
	l.item = Token{Type: TokenTypeEOF, Pos: l.pos, End: l.pos, Value: "EOF", Line: l.startLine}
	var state stateFn
	if l.insideAction {
		state = scanInsideAction
	} else if l.Options.ParseHTML {
		if l.htmlReturnState != nil {
			state = l.htmlReturnState
			l.htmlReturnState = nil
		} else {
			state = lexHTMLText
		}
	} else {
		state = scanText
	}
	for {
		state = state(l)
		if state == nil {
			return l.item
		}
	}
}

func New(input string, options Options) *Scanner {
	if options.LeftDelim == "" {
		options.LeftDelim = leftDelim
	}
	if options.RightDelim == "" {
		options.RightDelim = rightDelim
	}
	l := &Scanner{
		input:        input,
		Options:      options,
		line:         1,
		startLine:    1,
		insideAction: false,
	}
	return l
}

const (
	leftDelim    = "{{"
	rightDelim   = "}}"
	leftComment  = "/*"
	rightComment = "*/"
)

// scanText scans until an opening action delimiter, "{{".
func scanText(l *Scanner) stateFn {
	if x := strings.Index(l.input[l.pos:], l.Options.LeftDelim); x >= 0 {
		if x > 0 {
			l.pos += Pos(x)
			l.line += strings.Count(l.input[l.start:l.pos], "\n")
			i := l.thisItem(TokenTypeText)
			l.ignore()
			if len(i.Value) > 0 {
				return l.emitItem(i)
			}
		}
		return scanLeftDelim
	}
	l.pos = Pos(len(l.input))
	// Correctly reached EOF.
	if l.pos > l.start {
		l.line += strings.Count(l.input[l.start:l.pos], "\n")
		return l.emit(TokenTypeText)
	}
	return l.emit(TokenTypeEOF)
}

// atRightDelim reports whether the lexer is at a right delimiter, possibly preceded by a trim marker.
func (l *Scanner) atRightDelim() (delim, trimSpaces bool) {
	if hasRightTrimMarker(l.input[l.pos:]) && strings.HasPrefix(l.input[l.pos+trimMarkerLen:], l.Options.RightDelim) { // With trim marker.
		return true, true
	}
	if strings.HasPrefix(l.input[l.pos:], l.Options.RightDelim) { // Without trim marker.
		return true, false
	}
	return false, false
}

// scanLeftDelim scans the left delimiter, which is known to be present, possibly with a trim marker.
func scanLeftDelim(l *Scanner) stateFn {
	l.pos += Pos(len(l.Options.LeftDelim))
	trimSpace := hasLeftTrimMarker(l.input[l.pos:])
	afterMarker := Pos(0)
	if trimSpace {
		afterMarker = trimMarkerLen
	}
	if strings.HasPrefix(l.input[l.pos+afterMarker:], leftComment) {
		l.commentTrimLeft = trimSpace
		l.pos += afterMarker
		l.ignore()
		return scanComment
	}
	i := l.thisItem(TokenTypeLeftDelim)
	i.TrimLeft = trimSpace
	l.insideAction = true
	l.pos += afterMarker
	l.ignore()
	l.parenDepth = 0
	return l.emitItem(i)
}

// scanComment scans a comment. The left comment marker is known to be present.
func scanComment(l *Scanner) stateFn {
	l.pos += Pos(len(leftComment))
	x := strings.Index(l.input[l.pos:], rightComment)
	if x < 0 {
		return l.errorf("unclosed comment")
	}
	l.pos += Pos(x + len(rightComment))
	delim, trimSpace := l.atRightDelim()
	if !delim {
		return l.errorf("comment ends before closing delimiter")
	}
	l.line += strings.Count(l.input[l.start:l.pos], "\n")
	i := l.thisItem(TokenTypeComment)
	i.TrimLeft = l.commentTrimLeft
	i.TrimRight = trimSpace
	l.commentTrimLeft = false
	if trimSpace {
		l.pos += trimMarkerLen
	}
	l.pos += Pos(len(l.Options.RightDelim))
	l.ignore()
	if l.Options.EmitComment {
		return l.emitItem(i)
	}
	if l.Options.ParseHTML {
		if l.htmlReturnState != nil {
			return l.htmlReturnState
		}
		return lexHTMLText
	}
	return scanText
}

// scanRightDelim scans the right delimiter, which is known to be present, possibly with a trim marker.
func scanRightDelim(l *Scanner) stateFn {
	_, trimSpace := l.atRightDelim()
	if trimSpace {
		l.pos += trimMarkerLen
		l.ignore()
	}
	l.pos += Pos(len(l.Options.RightDelim))
	i := l.thisItem(TokenTypeRightDelim)
	i.TrimRight = trimSpace
	l.insideAction = false
	return l.emitItem(i)
}

func scanInsideAction(l *Scanner) stateFn {
	// Either number, quoted string, or identifier.
	// Spaces separate arguments; runs of spaces turn into TokenTypeSpace.
	// Pipe symbols separate and are emitted.
	delim, _ := l.atRightDelim()
	if delim {
		if l.parenDepth == 0 {
			return scanRightDelim
		}
		return l.errorf("unclosed left paren")
	}
	switch r := l.next(); {
	case r == eof:
		return l.errorf("unclosed action")
	case isSpace(r):
		l.backup() // Put space back in case we have " -}}".
		return scanSpace
	case r == '=':
		return l.emit(TokenTypeAssign)
	case r == ':':
		if l.next() != '=' {
			return l.errorf("expected :=")
		}
		return l.emit(TokenTypeDeclare)
	case r == '|':
		return l.emit(TokenTypePipe)
	case r == '"':
		return scanQuote
	case r == '`':
		return scanRawQuote
	case r == '$':
		return scanVariable
	case r == '\'':
		return lexChar
	case r == '.':
		// special look-ahead for ".field" so we don't break l.backup().
		if l.pos < Pos(len(l.input)) {
			r := l.input[l.pos]
			if r < '0' || '9' < r {
				return scanField
			}
		}
		fallthrough // '.' can start a number.
	case r == '+' || r == '-' || ('0' <= r && r <= '9'):
		l.backup()
		return scanNumber
	case isAlphaNumeric(r):
		l.backup()
		return scanIdentifier
	case r == '(':
		l.parenDepth++
		return l.emit(TokenTypeLeftParen)
	case r == ')':
		l.parenDepth--
		if l.parenDepth < 0 {
			return l.errorf("unexpected right paren")
		}
		return l.emit(TokenTypeRightParen)
	case r <= unicode.MaxASCII && unicode.IsPrint(r):
		return l.emit(TokenTypeChar)
	default:
		return l.errorf("unrecognized character in action: %#U", r)
	}
}

// scanSpace scans a run of space characters.
// We have not consumed the first space, which is known to be present.
// Take care if there is a trim-marked right delimiter, which starts with a space.
func scanSpace(l *Scanner) stateFn {
	var r rune
	var numSpaces int
	for {
		r = l.peek()
		if !isSpace(r) {
			break
		}
		l.next()
		numSpaces++
	}
	// Be careful about a trim-marked closing delimiter, which has a minus
	// after a space. We know there is a space, so check for the '-' that might follow.
	if hasRightTrimMarker(l.input[l.pos-1:]) && strings.HasPrefix(l.input[l.pos-1+trimMarkerLen:], l.Options.RightDelim) {
		l.backup() // Before the space.
		if numSpaces == 1 {
			return scanRightDelim // On the delim, so go right to that.
		}
	}
	return l.emit(TokenTypeSpace)
}

func scanIdentifier(l *Scanner) stateFn {
	for {
		switch r := l.next(); {
		case isAlphaNumeric(r):
			// absorb.
		default:
			l.backup()
			word := l.input[l.start:l.pos]
			if !l.atTerminator() {
				return l.errorf("bad character %#U", r)
			}
			switch {
			case key[word] > TokenTypeKeyword:
				item := key[word]
				if item == TokenTypeBreak && !l.Options.BreakOK || item == TokenTypeContinue && !l.Options.ContinueOK {
					return l.emit(TokenTypeIdentifier)
				}
				return l.emit(item)
			case word[0] == '.':
				return l.emit(TokenTypeField)
			case word == "true", word == "false":
				return l.emit(TokenTypeBool)
			default:
				return l.emit(TokenTypeIdentifier)
			}
		}
	}
}

// scanField scans a field: .Alphanumeric.
// The . has been scanned.
func scanField(l *Scanner) stateFn {
	return scanFieldOrVariable(l, TokenTypeField)
}

// scanVariable scans a Variable: $Alphanumeric.
// The $ has been scanned.
func scanVariable(l *Scanner) stateFn {
	if l.atTerminator() { // Nothing interesting follows -> "$".
		return l.emit(TokenTypeVariable)
	}
	return scanFieldOrVariable(l, TokenTypeVariable)
}

// scanFieldOrVariable scans a field or variable: [.$]Alphanumeric.
// The . or $ has been scanned.
func scanFieldOrVariable(l *Scanner, typ TokenType) stateFn {
	if l.atTerminator() { // Nothing interesting follows -> "." or "$".
		if typ == TokenTypeVariable {
			return l.emit(TokenTypeVariable)
		}
		return l.emit(TokenTypeDot)
	}
	var r rune
	for {
		r = l.next()
		if !isAlphaNumeric(r) {
			l.backup()
			break
		}
	}
	if !l.atTerminator() {
		return l.errorf("bad character %#U", r)
	}
	return l.emit(typ)
}

// atTerminator reports whether the input is at valid termination character to
// appear after an identifier. Breaks .X.Y into two pieces. Also catches cases
// like "$x+2" not being acceptable without a space, in case we decide one
// day to implement arithmetic.
func (l *Scanner) atTerminator() bool {
	r := l.peek()
	if isSpace(r) {
		return true
	}
	switch r {
	case eof, '.', ',', '|', ':', ')', '(':
		return true
	}
	return strings.HasPrefix(l.input[l.pos:], l.Options.RightDelim)
}

// lexChar scans a character constant. The initial quote is already
// scanned. Syntax checking is done by the parser.
func lexChar(l *Scanner) stateFn {
Loop:
	for {
		switch l.next() {
		case '\\':
			if r := l.next(); r != eof && r != '\n' {
				break
			}
			fallthrough
		case eof, '\n':
			return l.errorf("unterminated character constant")
		case '\'':
			break Loop
		}
	}
	return l.emit(TokenTypeCharConstant)
}

// scanNumber scans a number: decimal, octal, hex, float, or imaginary. This
// isn't a perfect number scanner - for instance it accepts "." and "0x0.2"
// and "089" - but when it's wrong the input is invalid and the parser (via
// strconv) will notice.
func scanNumber(l *Scanner) stateFn {
	if !l.scanNumber() {
		return l.errorf("bad number syntax: %q", l.input[l.start:l.pos])
	}
	if sign := l.peek(); sign == '+' || sign == '-' {
		// Complex: 1+2i. No spaces, must end in 'i'.
		if !l.scanNumber() || l.input[l.pos-1] != 'i' {
			return l.errorf("bad number syntax: %q", l.input[l.start:l.pos])
		}
		return l.emit(TokenTypeComplex)
	}
	return l.emit(TokenTypeNumber)
}

func (l *Scanner) scanNumber() bool {
	// Optional leading sign.
	l.accept("+-")
	// Is it hex?
	digits := "0123456789_"
	if l.accept("0") {
		// Note: Leading 0 does not mean octal in floats.
		if l.accept("xX") {
			digits = "0123456789abcdefABCDEF_"
		} else if l.accept("oO") {
			digits = "01234567_"
		} else if l.accept("bB") {
			digits = "01_"
		}
	}
	l.acceptRun(digits)
	if l.accept(".") {
		l.acceptRun(digits)
	}
	if len(digits) == 10+1 && l.accept("eE") {
		l.accept("+-")
		l.acceptRun("0123456789_")
	}
	if len(digits) == 16+6+1 && l.accept("pP") {
		l.accept("+-")
		l.acceptRun("0123456789_")
	}
	// Is it imaginary?
	l.accept("i")
	// Next thing mustn't be alphanumeric.
	if isAlphaNumeric(l.peek()) {
		l.next()
		return false
	}
	return true
}

func scanQuote(l *Scanner) stateFn {
Loop:
	for {
		switch l.next() {
		case '\\':
			if r := l.next(); r != eof && r != '\n' {
				break
			}
			fallthrough
		case eof, '\n':
			return l.errorf("unterminated quoted string")
		case '"':
			break Loop
		}
	}
	return l.emit(TokenTypeString)
}

func scanRawQuote(l *Scanner) stateFn {
Loop:
	for {
		switch l.next() {
		case eof:
			return l.errorf("unterminated raw quoted string")
		case '`':
			break Loop
		}
	}
	return l.emit(TokenTypeRawString)
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n'
}

func isAlphaNumeric(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func hasLeftTrimMarker(s string) bool {
	return len(s) >= 2 && s[0] == trimMarker && isSpace(rune(s[1]))
}

func hasRightTrimMarker(s string) bool {
	return len(s) >= 2 && isSpace(rune(s[0])) && s[1] == trimMarker
}
