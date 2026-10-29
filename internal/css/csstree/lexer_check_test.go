// Ported from lib/__tests/lexer-check-atrule-descriptor.js,
// lexer-check-atrule-name.js, lexer-check-atrule-prelude.js and
// lexer-check-property-name.js.

package csstree

import (
	"errors"
	"testing"
)

// assertSyntaxReferenceError checks that err is a *SyntaxReferenceError
// (upstream error.name === 'SyntaxReferenceError') with message.
func assertSyntaxReferenceError(t *testing.T, call string, err error, message string) {
	t.Helper()
	var referenceError *SyntaxReferenceError
	if !errors.As(err, &referenceError) {
		t.Fatalf("%s = %v, want a *SyntaxReferenceError", call, err)
	}
	if err.Error() != message {
		t.Errorf("%s = %q, want %q", call, err.Error(), message)
	}
}

func assertErrorMessage(t *testing.T, call string, err error, message string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s = nil, want %q", call, message)
	}
	if err.Error() != message {
		t.Errorf("%s = %q, want %q", call, err.Error(), message)
	}
}

func assertNoError(t *testing.T, call string, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("%s = %v, want nil", call, err)
	}
}

func TestLexerCheckAtruleDescriptorName(t *testing.T) {
	lexer := DefaultLexer()

	t.Run("should fail on invalid atrule", func(t *testing.T) {
		assertSyntaxReferenceError(t, `CheckAtruleDescriptorName("foo", "")`, lexer.CheckAtruleDescriptorName("foo", ""), "Unknown at-rule `@foo`")
	})

	t.Run("should fail when at-rule has no descriptors", func(t *testing.T) {
		assertErrorMessage(t, `CheckAtruleDescriptorName("import", "test")`, lexer.CheckAtruleDescriptorName("import", "test"), "At-rule `@import` has no known descriptors")
	})

	t.Run("should fail when at-rule has no descriptor", func(t *testing.T) {
		assertErrorMessage(t, `CheckAtruleDescriptorName("font-face", "color")`, lexer.CheckAtruleDescriptorName("font-face", "color"), "Unknown at-rule descriptor `color`")
	})

	t.Run("should pass on correct descriptor", func(t *testing.T) {
		assertNoError(t, `CheckAtruleDescriptorName("font-face", "font-family")`, lexer.CheckAtruleDescriptorName("font-face", "font-family"))
	})
}

func TestLexerCheckAtruleName(t *testing.T) {
	lexer := DefaultLexer()

	t.Run("should pass correct atrule", func(t *testing.T) {
		assertNoError(t, `CheckAtruleName("media")`, lexer.CheckAtruleName("media"))
	})

	t.Run("should pass correct vendor atrule", func(t *testing.T) {
		assertNoError(t, `CheckAtruleName("-webkit-media")`, lexer.CheckAtruleName("-webkit-media")) // FIXME: should fail because never exists
		assertNoError(t, `CheckAtruleName("-webkit-keyframes")`, lexer.CheckAtruleName("-webkit-keyframes"))
	})

	t.Run("should fail on invalid atrule", func(t *testing.T) {
		assertSyntaxReferenceError(t, `CheckAtruleName("foo")`, lexer.CheckAtruleName("foo"), "Unknown at-rule `@foo`")
	})
}

func TestLexerCheckAtrulePrelude(t *testing.T) {
	lexer := DefaultLexer()

	t.Run("should fail on invalid atrule", func(t *testing.T) {
		assertSyntaxReferenceError(t, `CheckAtrulePrelude("foo", nil)`, lexer.CheckAtrulePrelude("foo", nil), "Unknown at-rule `@foo`")
	})

	t.Run("should fail when prelude is set for at-rule with no prelude", func(t *testing.T) {
		assertErrorMessage(t, `CheckAtrulePreludeString("font-face", "hi")`, lexer.CheckAtrulePreludeString("font-face", "hi"), "At-rule `@font-face` should not contain a prelude")
	})

	// upstream passes '', null and undefined; nil Node and "" text cover them
	t.Run("should pass when no prelude for at-rule with no prelude", func(t *testing.T) {
		assertNoError(t, `CheckAtrulePreludeString("font-face", "")`, lexer.CheckAtrulePreludeString("font-face", ""))
		assertNoError(t, `CheckAtrulePrelude("font-face", nil)`, lexer.CheckAtrulePrelude("font-face", nil))
	})

	t.Run("should pass when prelude is not defined and syntax allows it", func(t *testing.T) {
		assertNoError(t, `CheckAtrulePreludeString("page", "")`, lexer.CheckAtrulePreludeString("page", ""))
		assertNoError(t, `CheckAtrulePrelude("page", nil)`, lexer.CheckAtrulePrelude("page", nil))
	})

	t.Run("should fail when prelude is not set for at-rule with prelude", func(t *testing.T) {
		assertErrorMessage(t, `CheckAtrulePreludeString("keyframes", "")`, lexer.CheckAtrulePreludeString("keyframes", ""), "At-rule `@keyframes` should contain a prelude")
		assertErrorMessage(t, `CheckAtrulePrelude("keyframes", nil)`, lexer.CheckAtrulePrelude("keyframes", nil), "At-rule `@keyframes` should contain a prelude")
	})

	t.Run("should pass when prelude for at-rule with prelude", func(t *testing.T) {
		assertNoError(t, `CheckAtrulePreludeString("keyframes", "test")`, lexer.CheckAtrulePreludeString("keyframes", "test"))
	})
}

func TestLexerCheckPropertyName(t *testing.T) {
	lexer := DefaultLexer()

	t.Run("should pass correct property", func(t *testing.T) {
		assertNoError(t, `CheckPropertyName("color")`, lexer.CheckPropertyName("color"))
	})

	t.Run("should pass correct vendor property", func(t *testing.T) {
		assertNoError(t, `CheckPropertyName("-webkit-color")`, lexer.CheckPropertyName("-webkit-color")) // FIXME: should fail because never exists
		assertNoError(t, `CheckPropertyName("-webkit-border-radius")`, lexer.CheckPropertyName("-webkit-border-radius"))
	})

	t.Run("should fail on invalid property", func(t *testing.T) {
		assertSyntaxReferenceError(t, `CheckPropertyName("foo")`, lexer.CheckPropertyName("foo"), "Unknown property `foo`")
	})
}
