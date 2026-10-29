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
