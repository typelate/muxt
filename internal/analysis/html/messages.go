package analysis

// Diagnostic message formats. Centralized for the message-quality
// pass (plan Task 10). Keep stdlib template error style: lowercase,
// no trailing period, evidence in <angle brackets> or %q.
const (
	msgFosteredElement = `browsers move <%s> out of <%s>, to before the <table>`
	msgFosteredText    = `browsers move text in <%s> to before the <table>`
	msgNestedForm      = `browsers ignore this <form>; it is already inside a <form>`
	msgNestedLink      = `browsers close the outer <a> when this <a> starts`
	msgNestedButton    = `browsers close the outer <button> when this <button> starts`
	msgAutoClosed      = `browsers close the open <%s> when <%s> starts; the DOM won't match this nesting`
	conditionalSuffix  = ` (in some branches)`
	repeatedSuffix     = ` (in a loop body)`
)

// Style diagnostic message formats.
const (
	msgUnknownCSSProperty = `unknown CSS property %q; browsers drop this declaration`
	msgInvalidCSSValue    = `%q is not a valid value for CSS property %q; want %s`
	msgUnknownCSSAtRule   = `unknown CSS at-rule %q; browsers ignore it and its block`
	msgUnusedCSSRule      = `no element in the templates matches %q; the rule is unused`
	msgUnusedCSSSelector  = `no element in the templates matches %q`

	msgUndefinedCSSCustomProperty = `custom property %q is never declared; var() without a fallback makes this declaration invalid`
)

// Comment diagnostic message formats.
const (
	msgCommentInRCDATA  = `html/template writes this <!-- as text inside <%s>; browsers show it`
	msgEscapedMarkup    = `html/template escapes this %q, so browsers show it as text; comment with {{/* */}} or <!-- -->`
	msgCommentDelimiter = `browsers end this comment at the %q at %s but html/template strips through the "-->" at %s, hiding the markup between`
	msgCommentInRawText = `html/template strips this comment, but browsers read <%s> content as raw text and keep it`
	msgEscapedInRawText = `html/template escapes this %q as "&lt;", but browsers read <%s> content as raw text and show the "&lt;"`
	msgEscaperRefuses   = `html/template refuses this template: %s`
	msgUnclosedComment  = `a comment that never closes`
)
