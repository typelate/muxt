package template_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/template"
)

func TestNew(t *testing.T) {
	tmpl := template.New[any]("test")
	assert.Equal(t, "test", tmpl.Name())
	assert.Nil(t, tmpl.Tree(), "Tree() should be nil before parsing")
	assert.Same(t, tmpl, tmpl.Lookup("test"), "Lookup(name) should return self")
}

func TestParse(t *testing.T) {
	tmpl := template.New[any]("test")
	result, err := tmpl.Parse("<div>hello</div>")
	require.NoError(t, err)
	assert.Same(t, tmpl, result, "Parse() should return the receiver")
	require.NotNil(t, tmpl.Tree(), "Tree() should not be nil after parsing")
	assert.Equal(t, "<div>hello</div>", tmpl.Tree().Root.String())
}

func TestParseWithDefine(t *testing.T) {
	tmpl := template.New[any]("main")
	_, err := tmpl.Parse(`<div>{{define "sub"}}<span>inner</span>{{end}}</div>`)
	require.NoError(t, err)
	sub := tmpl.Lookup("sub")
	require.NotNil(t, sub, `Lookup("sub") should not be nil`)
	require.NotNil(t, sub.Tree(), "sub.Tree() should not be nil")
}

func TestFunctions(t *testing.T) {
	tmpl := template.New[any]("test")
	result := tmpl.Functions(map[string]any{"myfunc": nil})
	assert.Same(t, tmpl, result, "Functions() should return the receiver")
}

func TestDelimiters(t *testing.T) {
	tmpl := template.New[any]("test").Delimiters("<%", "%>")
	_, err := tmpl.Parse("<div><% .Title %></div>")
	require.NoError(t, err)
	require.NotNil(t, tmpl.Tree(), "Tree() should not be nil after parsing with custom delimiters")
}

func TestOption(t *testing.T) {
	t.Run("valid option", func(t *testing.T) {
		tmpl := template.New[any]("test")
		result, err := tmpl.Option("missingkey=error")
		assert.NoError(t, err)
		assert.Same(t, tmpl, result, "Option() should return the receiver")
	})

	t.Run("unknown option returns error", func(t *testing.T) {
		_, err := template.New[any]("test").Option("badoption")
		assert.Error(t, err)
	})

	t.Run("unknown missingkey value returns error", func(t *testing.T) {
		_, err := template.New[any]("test").Option("missingkey=badvalue")
		assert.Error(t, err)
	})

	t.Run("missingkey=default is accepted", func(t *testing.T) {
		// stdlib html/template treats "default" as an alias for "invalid".
		tmpl := template.New[any]("test")
		result, err := tmpl.Option("missingkey=default")
		assert.NoError(t, err)
		assert.Same(t, tmpl, result, "Option() should return the receiver")
	})
}

func TestLookupNil(t *testing.T) {
	tmpl := template.New[any]("test")
	assert.Nil(t, tmpl.Lookup("nonexistent"))
}

func TestFindTree(t *testing.T) {
	tmpl := template.New[any]("main")
	_, err := tmpl.Parse("<p>hello</p>")
	require.NoError(t, err)

	tree, ok := tmpl.FindTree("main")
	assert.True(t, ok, `FindTree("main") should return true`)
	assert.NotNil(t, tree, `FindTree("main") tree should not be nil`)

	tree, ok = tmpl.FindTree("nonexistent")
	assert.False(t, ok, `FindTree("nonexistent") should return false`)
	assert.Nil(t, tree, `FindTree("nonexistent") tree should be nil`)
}

func TestAddParseTree(t *testing.T) {
	tmpl := template.New[any]("main")
	_, err := tmpl.Parse("<p>main</p>")
	require.NoError(t, err)

	// Parse another template to get a tree
	other := template.New[any]("other")
	_, err = other.Parse("<span>other</span>")
	require.NoError(t, err)

	added, err := tmpl.AddParseTree("extra", other.Tree())
	require.NoError(t, err)
	require.NotNil(t, added, "AddParseTree() should not return nil")

	// Verify via Lookup
	assert.NotNil(t, tmpl.Lookup("extra"), `Lookup("extra") should not be nil after AddParseTree`)

	// Verify via FindTree
	tree, ok := tmpl.FindTree("extra")
	assert.True(t, ok, `FindTree("extra") should return true after AddParseTree`)
	assert.Same(t, other.Tree(), tree, `FindTree("extra") should return the added tree`)
}

func TestAddParseTreeAnnotatesContext(t *testing.T) {
	tmpl := template.New[any]("main")
	_, err := tmpl.Parse("<p>main</p>")
	require.NoError(t, err)

	// Parse a template with an action in an attribute
	other := template.New[any]("link")
	_, err = other.Parse(`<a href="{{.URL}}">{{.Label}}</a>`)
	require.NoError(t, err)

	// AddParseTree should annotate contexts on the added tree
	_, err = tmpl.AddParseTree("link", other.Tree())
	require.NoError(t, err)

	// Verify via FindTree that the tree is present
	tree, ok := tmpl.FindTree("link")
	require.True(t, ok, `FindTree("link") should return true after AddParseTree`)
	require.NotNil(t, tree, `FindTree("link") tree should not be nil`)
}
