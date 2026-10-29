// Ported from lib/tokenizer/TokenStream.js.

package tokenizer

const (
	offsetMask      = 1<<typeShift - 1
	typeShift       = 58
	blockOpenToken  = 1
	blockCloseToken = 2
)

// balancePair maps a block opener to its closer. It is read-only.
// 32b of memory ought to be enough for anyone (any number of tokens)
var balancePair = [32]TokenType{
	Function:          RightParenthesis,
	LeftParenthesis:   RightParenthesis,
	LeftSquareBracket: RightSquareBracket,
	LeftCurlyBracket:  RightCurlyBracket,
}

// blockTokens marks block openers and closers. It is read-only.
var blockTokens = [32]uint8{
	Function:           blockOpenToken,
	LeftParenthesis:    blockOpenToken,
	LeftSquareBracket:  blockOpenToken,
	LeftCurlyBracket:   blockOpenToken,
	RightParenthesis:   blockCloseToken,
	RightSquareBracket: blockCloseToken,
	RightCurlyBracket:  blockCloseToken,
}

func boundIndex(index, minimum, maximum int) int {
	if index < minimum {
		return minimum
	}
	if index > maximum {
		return maximum
	}
	return index
}

// TokenStream is a cursor over the tokens of a source. Use NewTokenStream or
// call SetSource before using it.
//
// Tokens are stored packed as (type << 58 | end) in uint64 cells. Upstream
// packs (type << 24 | end) into uint32 cells, which silently corrupts offsets
// in sources of 16 MiB or more; 58 offset bits exceed any addressable source.
type TokenStream struct {
	Source string

	// TokenIndex, TokenType, TokenStart and TokenEnd describe the current
	// token; EOF reports that the cursor has moved past the last token.
	EOF        bool
	TokenIndex int
	TokenType  TokenType
	TokenStart int
	TokenEnd   int

	TokenCount int

	// Balance holds, for every token index, the index of the matching
	// block token (or of the enclosing block's end, or TokenCount).
	Balance []int

	firstCharOffset int
	offsetAndType   []uint64
}

// NewTokenStream tokenizes source with tokenize and positions the stream on
// the first token. A nil tokenize produces no tokens.
func NewTokenStream(source string, tokenize func(source string, onToken func(tokenType TokenType, start, end int))) *TokenStream {
	ts := new(TokenStream)
	ts.SetSource(source, tokenize)
	return ts
}

// Reset moves the cursor back before the first token.
func (ts *TokenStream) Reset() {
	ts.EOF = false
	ts.TokenIndex = -1
	ts.TokenType = 0
	ts.TokenStart = ts.firstCharOffset
	ts.TokenEnd = ts.firstCharOffset
}

// SetSource tokenizes source with tokenize (usually Tokenize), reusing the
// stream's buffers, and positions the stream on the first token. A nil
// tokenize produces no tokens.
func (ts *TokenStream) SetSource(source string, tokenize func(source string, onToken func(tokenType TokenType, start, end int))) {
	if tokenize == nil {
		tokenize = func(string, func(TokenType, int, int)) {}
	}

	sourceLength := len(source)
	offsetAndType := adoptBuffer(ts.offsetAndType, len(source)+1) // +1 because of eof-token
	balance := adoptBuffer(ts.Balance, len(source)+1)
	tokenCount := 0
	firstCharOffset := -1
	balanceCloseType := TokenType(0)
	balanceStart := len(source)

	// capture buffers
	ts.offsetAndType = nil
	ts.Balance = nil
	clear(balance)

	tokenize(source, func(tokenType TokenType, start, end int) {
		index := tokenCount
		tokenCount++

		// type & offset
		offsetAndType[index] = uint64(tokenType)<<typeShift | uint64(end)

		if firstCharOffset == -1 {
			firstCharOffset = start
		}

		// balance
		balance[index] = balanceStart

		if tokenType == balanceCloseType {
			prevBalanceStart := balance[balanceStart]

			// set reference to balance end for a block opener
			balance[balanceStart] = index

			// pop state
			balanceStart = prevBalanceStart
			balanceCloseType = balancePair[offsetAndType[prevBalanceStart]>>typeShift]
		} else if ts.IsBlockOpenerTokenType(tokenType) { // check for FunctionToken, <(-token>, <[-token> and <{-token>
			// push state
			balanceStart = index
			balanceCloseType = balancePair[tokenType]
		}
	})

	// finalize buffers
	offsetAndType[tokenCount] = uint64(EOF)<<typeShift | uint64(sourceLength) // <EOF-token>
	balance[tokenCount] = tokenCount                                          // prevents false positive balance match with any token

	// reverse references from balance start to end
	// tokens
	//   token:   a ( [ b c ] d e ) {
	//   index:   0 1 2 3 4 5 6 7 8 9
	// before
	//   balance: 0 8 5 2 2 2 1 1 1 0
	//            - > > < < < < < < -
	// after
	//   balance: 9 8 5 5 5 2 8 8 1 9
	//            > > > > > < > > < >
	for i := range tokenCount {
		balanceStart := balance[i]

		if balanceStart <= i {
			balanceEnd := balance[balanceStart]

			if balanceEnd != i {
				balance[i] = balanceEnd
			}
		} else if balanceStart > tokenCount {
			balance[i] = tokenCount
		}
	}

	// balance[0] = tokenCount;

	ts.Source = source
	if firstCharOffset == -1 {
		ts.firstCharOffset = 0
	} else {
		ts.firstCharOffset = firstCharOffset
	}
	ts.TokenCount = tokenCount
	ts.offsetAndType = offsetAndType
	ts.Balance = balance

	ts.Reset()
	ts.Next()
}

// packed reads offsetAndType[index], returning 0 out of range as a JavaScript
// typed array read of undefined does once masked or shifted.
func (ts *TokenStream) packed(index int) uint64 {
	if index < 0 || index >= len(ts.offsetAndType) {
		return 0
	}
	return ts.offsetAndType[index]
}

// LookupType returns the type of the token offset tokens after the current
// one, or EOF.
func (ts *TokenStream) LookupType(offset int) TokenType {
	offset += ts.TokenIndex

	if offset < ts.TokenCount {
		return TokenType(ts.packed(offset) >> typeShift)
	}

	return EOF
}

// LookupTypeNonSC returns the type of the idx-th token from the current one
// that is not whitespace or a comment, or EOF.
func (ts *TokenStream) LookupTypeNonSC(idx int) TokenType {
	for offset := ts.TokenIndex; offset < ts.TokenCount; offset++ {
		tokenType := TokenType(ts.packed(offset) >> typeShift)

		if tokenType != WhiteSpace && tokenType != Comment {
			if idx == 0 {
				return tokenType
			}
			idx--
		}
	}

	return EOF
}

// LookupOffset returns the start offset of the token offset tokens after the
// current one, or len(Source).
func (ts *TokenStream) LookupOffset(offset int) int {
	offset += ts.TokenIndex

	if offset < ts.TokenCount {
		return int(ts.packed(offset-1) & offsetMask)
	}

	return len(ts.Source)
}

// LookupOffsetNonSC returns how many tokens after the current one the idx-th
// token that is not whitespace or a comment is, or 0 (EOF).
func (ts *TokenStream) LookupOffsetNonSC(idx int) int {
	for offset := ts.TokenIndex; offset < ts.TokenCount; offset++ {
		tokenType := TokenType(ts.packed(offset) >> typeShift)

		if tokenType != WhiteSpace && tokenType != Comment {
			if idx == 0 {
				return offset - ts.TokenIndex
			}
			idx--
		}
	}

	return int(EOF)
}

// LookupValue reports whether the token offset tokens after the current one
// is an ASCII case-insensitive match for referenceStr (which must be
// lowercase).
func (ts *TokenStream) LookupValue(offset int, referenceStr string) bool {
	offset += ts.TokenIndex

	if offset < ts.TokenCount {
		return CmpStr(
			ts.Source,
			int(ts.packed(offset-1)&offsetMask),
			int(ts.packed(offset)&offsetMask),
			referenceStr,
		)
	}

	return false
}

// GetTokenStart returns the start offset of the token at tokenIndex.
func (ts *TokenStream) GetTokenStart(tokenIndex int) int {
	if tokenIndex == ts.TokenIndex {
		return ts.TokenStart
	}

	if tokenIndex > 0 {
		if tokenIndex < ts.TokenCount {
			return int(ts.offsetAndType[tokenIndex-1] & offsetMask)
		}
		return int(ts.offsetAndType[ts.TokenCount] & offsetMask)
	}

	return ts.firstCharOffset
}

// GetTokenEnd returns the end offset of the token at tokenIndex.
func (ts *TokenStream) GetTokenEnd(tokenIndex int) int {
	if tokenIndex == ts.TokenIndex {
		return ts.TokenEnd
	}

	return int(ts.offsetAndType[boundIndex(tokenIndex, 0, ts.TokenCount)] & offsetMask)
}

// GetTokenType returns the type of the token at tokenIndex.
func (ts *TokenStream) GetTokenType(tokenIndex int) TokenType {
	if tokenIndex == ts.TokenIndex {
		return ts.TokenType
	}

	return TokenType(ts.offsetAndType[boundIndex(tokenIndex, 0, ts.TokenCount)] >> typeShift)
}

// SubstrToCursor returns Source[start:TokenStart].
func (ts *TokenStream) SubstrToCursor(start int) string {
	return substring(ts.Source, start, ts.TokenStart)
}

// IsBlockOpenerTokenType reports whether tokenType opens a block: a
// function, (, [ or { token.
func (ts *TokenStream) IsBlockOpenerTokenType(tokenType TokenType) bool {
	return blockTokens[tokenType] == blockOpenToken
}

// IsBlockCloserTokenType reports whether tokenType closes a block: a ), ] or
// } token.
func (ts *TokenStream) IsBlockCloserTokenType(tokenType TokenType) bool {
	return blockTokens[tokenType] == blockCloseToken
}

// GetBlockTokenPairIndex returns the index of the token that closes (or
// opens) the block token at tokenIndex, or -1 when it is not a block token
// or has no pair.
func (ts *TokenStream) GetBlockTokenPairIndex(tokenIndex int) int {
	tokenType := ts.GetTokenType(tokenIndex)

	if blockTokens[tokenType] == 1 {
		// block open token
		pairIndex, ok := ts.balanceAt(tokenIndex)
		if !ok {
			return -1 // upstream: no cell, so the pair's type is 0 and matches no closer
		}
		closeType := ts.GetTokenType(pairIndex)

		if balancePair[tokenType] == closeType {
			return pairIndex
		}
		return -1
	} else if blockTokens[tokenType] == 2 {
		// block close token
		pairIndex, ok := ts.balanceAt(tokenIndex)
		if !ok {
			return -1 // upstream: no cell, so the pair's type is 0 and opens nothing
		}
		openType := ts.GetTokenType(pairIndex)

		if balancePair[openType] == tokenType {
			return pairIndex
		}
		return -1
	}

	return -1
}

// IsBalanceEdge reports whether the current token's balance points before
// tokenIndex.
func (ts *TokenStream) IsBalanceEdge(tokenIndex int) bool {
	balance, ok := ts.balanceAt(ts.TokenIndex)
	// upstream compares undefined (no cell, as after Reset), which is false
	return ok && balance < tokenIndex
}

// balanceAt reads Balance[index]; ok is false out of range, where a
// JavaScript typed array read gives undefined.
func (ts *TokenStream) balanceAt(index int) (balance int, ok bool) {
	if index < 0 || index >= len(ts.Balance) {
		return 0, false
	}
	return ts.Balance[index], true
}

// IsDelim reports whether the token offset tokens after the current one (the
// current one when offset is 0) is a delim token for code.
func (ts *TokenStream) IsDelim(code, offset int) bool {
	if offset != 0 {
		return ts.LookupType(offset) == Delim &&
			CharCodeAt(ts.Source, ts.LookupOffset(offset)) == code
	}

	return ts.TokenType == Delim &&
		CharCodeAt(ts.Source, ts.TokenStart) == code
}

// Skip moves the cursor tokenCount tokens forward (or backward when
// negative), or to EOF.
func (ts *TokenStream) Skip(tokenCount int) {
	next := ts.TokenIndex + tokenCount

	if next < ts.TokenCount {
		ts.TokenIndex = next
		ts.TokenStart = int(ts.packed(next-1) & offsetMask)
		item := ts.packed(next)
		ts.TokenType = TokenType(item >> typeShift)
		ts.TokenEnd = int(item & offsetMask)
	} else {
		ts.TokenIndex = ts.TokenCount
		ts.Next()
	}
}

// Next moves the cursor to the next token, or to EOF.
func (ts *TokenStream) Next() {
	next := ts.TokenIndex + 1

	if next < ts.TokenCount {
		ts.TokenIndex = next
		ts.TokenStart = ts.TokenEnd
		item := ts.offsetAndType[next]
		ts.TokenType = TokenType(item >> typeShift)
		ts.TokenEnd = int(item & offsetMask)
	} else {
		ts.EOF = true
		ts.TokenIndex = ts.TokenCount
		ts.TokenType = EOF
		ts.TokenStart = len(ts.Source)
		ts.TokenEnd = len(ts.Source)
	}
}

// SkipSC skips whitespace and comment tokens.
func (ts *TokenStream) SkipSC() {
	for ts.TokenType == WhiteSpace || ts.TokenType == Comment {
		ts.Next()
	}
}

// SkipUntilBalanced moves the cursor forward from startToken to the end of
// the enclosing block, stopping early where stopConsume, given the first
// char code of a token, returns 1 (stop before the token) or 2 (stop after
// it). Blocks opened on the way are skipped whole.
func (ts *TokenStream) SkipUntilBalanced(startToken int, stopConsume func(code int) int) {
	cursor := startToken
	balanceEnd := 0
	offset := 0

loop:
	for ; cursor < ts.TokenCount; cursor++ {
		balanceEnd = ts.Balance[cursor]

		// stop scanning on balance edge that points to offset before start token
		if balanceEnd < startToken {
			break loop
		}

		if cursor > 0 {
			offset = int(ts.offsetAndType[cursor-1] & offsetMask)
		} else {
			offset = ts.firstCharOffset
		}

		// check stop condition
		switch stopConsume(CharCodeAt(ts.Source, offset)) {
		case 1: // just stop
			break loop

		case 2: // stop & included
			cursor++
			break loop

		default:
			// fast forward to the end of balanced block for an open block tokens
			if ts.IsBlockOpenerTokenType(TokenType(ts.offsetAndType[cursor] >> typeShift)) {
				cursor = balanceEnd
			}
		}
	}

	ts.Skip(cursor - ts.TokenIndex)
}

// ForEachToken calls fn for every token in order.
func (ts *TokenStream) ForEachToken(fn func(tokenType TokenType, start, end, index int)) {
	for i, offset := 0, ts.firstCharOffset; i < ts.TokenCount; i++ {
		start := offset
		item := ts.offsetAndType[i]
		end := int(item & offsetMask)
		tokenType := TokenType(item >> typeShift)

		offset = end

		fn(tokenType, start, end, i)
	}
}

// DumpToken is one entry of Dump.
type DumpToken struct {
	Idx     int    `json:"idx"`
	Type    string `json:"type"`
	Chunk   string `json:"chunk"`
	Balance int    `json:"balance"`
}

// Dump returns every token with its name, text and balance, for debugging.
func (ts *TokenStream) Dump() []DumpToken {
	tokens := make([]DumpToken, ts.TokenCount)

	ts.ForEachToken(func(tokenType TokenType, start, end, index int) {
		tokens[index] = DumpToken{
			Idx:     index,
			Type:    TokenNames[tokenType],
			Chunk:   substring(ts.Source, start, end),
			Balance: ts.Balance[index],
		}
	})

	return tokens
}

// substring mirrors String.prototype.substring: it clamps both ends to
// [0, len(s)] and swaps them when start > end.
func substring(s string, start, end int) string {
	start = boundIndex(start, 0, len(s))
	end = boundIndex(end, 0, len(s))
	if start > end {
		start, end = end, start
	}
	return s[start:end]
}
