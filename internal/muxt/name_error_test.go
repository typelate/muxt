package muxt

import (
	"errors"
	"go/token"
	"html/template"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/source"
)

func TestDefinitionsErrorPosition(t *testing.T) {
	const name = "OPTIONS / F()"
	ts := template.Must(template.New("index.gohtml").Parse(`{{define "` + name + `"}}{{end}}`))
	variable := source.Variable{Name: "templates", Set: ts, Definitions: map[string]source.Definition{
		name: {
			Name: name,
			// The quoted name starts at column 10, so the name itself
			// starts one byte in, at column 11.
			TemplateName: source.Span{Position: token.Position{Filename: "index.gohtml", Offset: 9, Line: 1, Column: 10}, Length: len(name) + 2},
		},
	}}

	_, err := Definitions(variable)
	// The failing METHOD segment starts at the first byte of the name.
	require.EqualError(t, err, "index.gohtml:1:11: OPTIONS method not allowed; allowed methods: GET, POST, PUT, PATCH, and DELETE")

	nameErr, ok := err.(*NameError)
	require.True(t, ok)
	assert.Equal(t, "  OPTIONS / F()\n  ^^^^^^^\n"+nameErr.Error(), nameErr.MultiLineError())
}

func TestNameErrorMultiLineErrorClamps(t *testing.T) {
	for _, tt := range []struct {
		Name           string
		Err            NameError
		ExpectedMarker string
	}{
		{
			Name:           "span past the end of the name",
			Err:            NameError{Name: "GET /", Offset: 4, Length: 10, err: errors.New("boom")},
			ExpectedMarker: "    ^",
		},
		{
			Name:           "zero length gets one marker",
			Err:            NameError{Name: "GET /", Offset: 0, Length: 0, err: errors.New("boom")},
			ExpectedMarker: "^",
		},
		{
			Name:           "negative offset clamps to the start",
			Err:            NameError{Name: "GET /", Offset: -3, Length: 3, err: errors.New("boom")},
			ExpectedMarker: "^^^",
		},
		{
			// Handler identifiers may contain non-ASCII letters; the
			// marker is measured in runes so it stays aligned. The
			// prefix "GET /é " is 8 bytes but 7 display columns, and
			// "Héllo()" is 8 bytes but 7 runes.
			Name:           "multi-byte runes in the name",
			Err:            NameError{Name: "GET /é Héllo()", Offset: 8, Length: 8, err: errors.New("boom")},
			ExpectedMarker: "       ^^^^^^^",
		},
		{
			Name:           "every span is marked on one line",
			Err:            NameError{Name: "GET /{id} F(id, id)", Offset: 12, Length: 2, Also: [][2]int{{16, 18}}, err: errors.New("boom")},
			ExpectedMarker: "            ^^  ^^",
		},
		{
			Name:           "a span before the primary one is marked too",
			Err:            NameError{Name: "GET /{id} F(id, id)", Offset: 16, Length: 2, Also: [][2]int{{12, 14}}, err: errors.New("boom")},
			ExpectedMarker: "            ^^  ^^",
		},
		{
			Name:           "extra spans are clamped to the name",
			Err:            NameError{Name: "GET /", Offset: 0, Length: 1, Also: [][2]int{{3, 99}, {-4, -1}}, err: errors.New("boom")},
			ExpectedMarker: "^  ^^",
		},
		{
			Name:           "related locations follow the short form",
			Err:            NameError{Name: "GET /", Offset: 0, Length: 3, err: errors.New("boom"), Related: []string{"main.go:4:5: F is defined here"}},
			ExpectedMarker: "^^^",
		},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			expected := "  " + tt.Err.Name + "\n  " + tt.ExpectedMarker + "\n" + tt.Err.Error()
			for _, related := range tt.Err.Related {
				expected += "\n" + related
			}
			assert.Equal(t, expected, tt.Err.MultiLineError())
		})
	}
}

func TestFindIdents(t *testing.T) {
	for _, tt := range []struct {
		expr      string
		name      string
		wantPos   []token.Pos
		wantFirst token.Pos
	}{
		{expr: `F(a, G(a), b)`, name: "a", wantPos: []token.Pos{3, 8}, wantFirst: 3},
		{expr: `F(G(a), a)`, name: "a", wantPos: []token.Pos{5, 9}, wantFirst: 5},
		{expr: `F(b)`, name: "a"},
		{expr: `F()`, name: "a"},
	} {
		t.Run(tt.expr, func(t *testing.T) {
			call := mustParseCall(t, tt.expr)
			var got []token.Pos
			for _, node := range findIdents(call, tt.name) {
				got = append(got, node.Pos())
			}
			assert.Equal(t, tt.wantPos, got, "findIdents(%q, %q)", tt.expr, tt.name)

			first := findIdent(call, tt.name)
			if tt.wantFirst == token.NoPos {
				assert.Nil(t, first, "findIdent(%q, %q)", tt.expr, tt.name)
				return
			}
			require.NotNil(t, first, "findIdent(%q, %q)", tt.expr, tt.name)
			assert.Equal(t, tt.wantFirst, first.Pos(), "findIdent(%q, %q)", tt.expr, tt.name)
		})
	}
	assert.Nil(t, findIdents(nil, "a"), "findIdents(nil, a)")
}

func TestErrorList(t *testing.T) {
	first := &NameError{Name: "GET / F(a)", Offset: 0, Length: 3, err: errors.New("first problem")}
	second := &NameError{Name: "GET / G(b)", Offset: 0, Length: 3, err: errors.New("second problem")}

	t.Run("one error keeps its own type", func(t *testing.T) {
		err := CombineErrors([]error{first})
		require.Same(t, first, err)
	})
	t.Run("no errors is nil", func(t *testing.T) {
		require.NoError(t, CombineErrors(nil))
	})
	t.Run("the short form is one line naming the first failure", func(t *testing.T) {
		err := CombineErrors([]error{first, second})
		require.Equal(t, "first problem (and 1 more error)", err.Error())
		require.NotContains(t, err.Error(), "\n")
	})
	t.Run("the long form renders every member", func(t *testing.T) {
		err := CombineErrors([]error{first, second})
		multiLine, ok := err.(MultiLineError)
		require.True(t, ok)
		rendered := multiLine.MultiLineError()
		require.Contains(t, rendered, "first problem")
		require.Contains(t, rendered, "second problem")
		require.Contains(t, rendered, "\n\n", "members are separated by a blank line")
	})
	t.Run("members stay reachable through errors.As", func(t *testing.T) {
		err := CombineErrors([]error{first, second})
		nameErr, ok := errors.AsType[*NameError](err)
		require.True(t, ok)
		require.Same(t, first, nameErr)
	})
}

func TestNameErrorMultiLineErrorIgnoresAnInvertedSpan(t *testing.T) {
	err := NameError{Name: "GET /", Offset: 0, Length: 1, Also: [][2]int{{4, 2}, {3, 3}}, err: errors.New("boom")}
	assert.Equal(t, "  GET /\n  ^\nboom", err.MultiLineError())
}

func TestNameErrorErrorPrefix(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  NameError
		want string
	}{
		{name: "position", err: NameError{Position: token.Position{Filename: "a.gohtml", Line: 2, Column: 3}, SourceFile: "b.gohtml", err: errors.New("boom")}, want: "a.gohtml:2:3: boom"},
		{name: "source file", err: NameError{SourceFile: "b.gohtml", err: errors.New("boom")}, want: "b.gohtml: boom"},
		{name: "neither", err: NameError{err: errors.New("boom")}, want: "boom"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.err.Error())
		})
	}
}

func TestNewNameSpans(t *testing.T) {
	t.Run("a match spans its groups", func(t *testing.T) {
		const name = "GET /a F()"
		spans := newNameSpans(templateNameMux.FindStringSubmatchIndex(name))
		assert.Equal(t, [2]int{0, 3}, spans.method)
		assert.Equal(t, [2]int{4, 6}, spans.path)
		assert.Equal(t, [2]int{-1, -1}, spans.status)
	})
	t.Run("no match spans nothing", func(t *testing.T) {
		assert.Equal(t, nameSpans{[2]int{-1, -1}, [2]int{-1, -1}, [2]int{-1, -1}, [2]int{-1, -1}, [2]int{-1, -1}}, newNameSpans(nil))
	})
	t.Run("a group cut off by a short index spans nothing", func(t *testing.T) {
		// The index has the group's start but not its end.
		short := make([]int, 2*templateNameMux.SubexpIndex("METHOD")+1)
		assert.Equal(t, [2]int{-1, -1}, newNameSpans(short).method)
	})
}

func TestDefinitionSpanErrorf(t *testing.T) {
	def := &Definition{name: "GET /a F()"}
	for _, tt := range []struct {
		name       string
		span       [2]int
		wantOffset int
		wantLength int
	}{
		{name: "matched segment", span: [2]int{4, 6}, wantOffset: 4, wantLength: 2},
		{name: "segment at the start", span: [2]int{0, 3}, wantOffset: 0, wantLength: 3},
		{name: "unmatched segment marks the whole name", span: [2]int{-1, -1}, wantOffset: 0, wantLength: len("GET /a F()")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			nameErr, ok := def.spanErrorf(tt.span, "boom").(*NameError)
			require.True(t, ok)
			assert.Equal(t, tt.wantOffset, nameErr.Offset)
			assert.Equal(t, tt.wantLength, nameErr.Length)
		})
	}
}

func TestDefinitionPathParamErrorf(t *testing.T) {
	// The path "/{a}/{b}/{a}/{a}" starts at byte 4 of the name.
	def := &Definition{name: "GET /{a}/{b}/{a}/{a}", path: "/{a}/{b}/{a}/{a}", spans: nameSpans{path: [2]int{4, 20}}}
	for _, tt := range []struct {
		name       string
		param      string
		occurrence int
		wantOffset int
		wantLength int
	}{
		{name: "first occurrence", param: "a", occurrence: 0, wantOffset: 6, wantLength: 1},
		{name: "second occurrence", param: "a", occurrence: 1, wantOffset: 14, wantLength: 1},
		{name: "third occurrence", param: "a", occurrence: 2, wantOffset: 18, wantLength: 1},
		{name: "other parameter", param: "b", occurrence: 0, wantOffset: 10, wantLength: 1},
		{name: "no such occurrence marks the path", param: "a", occurrence: 3, wantOffset: 4, wantLength: 16},
		{name: "no such parameter marks the path", param: "c", occurrence: 0, wantOffset: 4, wantLength: 16},
	} {
		t.Run(tt.name, func(t *testing.T) {
			nameErr, ok := def.pathParamErrorf(tt.param, tt.occurrence, "boom").(*NameError)
			require.True(t, ok)
			assert.Equal(t, tt.wantOffset, nameErr.Offset, "offset")
			assert.Equal(t, tt.wantLength, nameErr.Length, "length")
		})
	}
}

func TestDefinitionHandlerSpan(t *testing.T) {
	t.Run("the handler expression", func(t *testing.T) {
		def := &Definition{handler: "Save()", handlerOffset: 10, spans: nameSpans{call: [2]int{9, 20}}}
		assert.Equal(t, [2]int{10, 16}, def.handlerSpan())
	})
	t.Run("the call segment without a handler", func(t *testing.T) {
		def := &Definition{spans: nameSpans{call: [2]int{9, 20}}}
		assert.Equal(t, [2]int{9, 20}, def.handlerSpan())
	})
}

func TestDefinitionFinishNameErrorFallback(t *testing.T) {
	def := &Definition{name: "GET /a Save()"}
	for _, tt := range []struct {
		name       string
		fallback   [2]int
		wantOffset int
		wantLength int
	}{
		{name: "a segment", fallback: [2]int{4, 6}, wantOffset: 4, wantLength: 2},
		{name: "a segment at the start", fallback: [2]int{0, 3}, wantOffset: 0, wantLength: 3},
		{name: "no segment marks the whole name", fallback: [2]int{-1, -1}, wantOffset: 0, wantLength: len("GET /a Save()")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			nameErr, ok := def.finishNameError(errors.New("boom"), tt.fallback).(*NameError)
			require.True(t, ok)
			assert.Equal(t, tt.wantOffset, nameErr.Offset, "offset")
			assert.Equal(t, tt.wantLength, nameErr.Length, "length")
		})
	}
}

func TestErrorListForms(t *testing.T) {
	list := ErrorList{errors.New("a"), errors.New("b"), errors.New("c")}
	assert.Equal(t, "a (and 2 more errors)", list.Error())
	assert.Equal(t, "a\n\nb\n\nc", list.MultiLineError())
	assert.Equal(t, "a", ErrorList{errors.New("a")}.MultiLineError())
}

func TestDefinitionPathParamErrorfAdjacentNames(t *testing.T) {
	// Only the second "{a" is the whole parameter: the first is followed
	// by more of a name, so the one missing occurrence marks the path.
	def := &Definition{name: "{a{a}", path: "{a{a}", spans: nameSpans{path: [2]int{0, 5}}}
	for occurrence, want := range []int{3, 0} {
		nameErr, ok := def.pathParamErrorf("a", occurrence, "boom").(*NameError)
		require.True(t, ok)
		assert.Equal(t, want, nameErr.Offset, "offset of occurrence %d", occurrence)
	}
}

// TestDefinitionPathParamErrorfWholeNames states that a parameter is
// found by its whole name, not by a name it is the prefix of.
func TestDefinitionPathParamErrorfWholeNames(t *testing.T) {
	for _, tt := range []struct {
		path       string
		param      string
		occurrence int
		want       int
	}{
		{path: "/{formx}/{form}", param: "form", want: len("/{formx}/{")},
		{path: "/{formx}/{form...}", param: "form", want: len("/{formx}/{")},
		{path: "/{ab}/{a}/{a}", param: "a", occurrence: 1, want: len("/{ab}/{a}/{")},
		{path: "/{abc/x", param: "abc", want: len("/{")},
		{path: "/{abcd/{abc", param: "abc", want: len("/{abcd/{")},
	} {
		t.Run(tt.path, func(t *testing.T) {
			def := &Definition{name: tt.path, path: tt.path, spans: nameSpans{path: [2]int{0, len(tt.path)}}}
			nameErr, ok := def.pathParamErrorf(tt.param, tt.occurrence, "boom").(*NameError)
			require.True(t, ok)
			assert.Equal(t, tt.want, nameErr.Offset, "offset of %s occurrence %d", tt.param, tt.occurrence)
			assert.Equal(t, len(tt.param), nameErr.Length, "length")
		})
	}
	t.Run("through Definitions", func(t *testing.T) {
		const name = "GET /{formx}/{form} F(formx, form)"
		_, err := Definitions(source.Variable{Name: "templates", Set: template.Must(template.New("").Parse(`{{define "` + name + `"}}{{end}}`))})
		nameErr, ok := errors.AsType[*NameError](err)
		require.True(t, ok, "Definitions() = %v, want a *NameError", err)
		assert.Equal(t, len("GET /{formx}/{"), nameErr.Offset, "offset")
	})
}
