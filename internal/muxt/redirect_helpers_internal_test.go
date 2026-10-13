package muxt

import (
	"testing"
	"text/template/parse"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func firstCommand(t *testing.T, text string) *parse.CommandNode {
	t.Helper()
	_, page := pageIn(t, text)
	action, ok := page.Tree.Root.Nodes[0].(*parse.ActionNode)
	require.True(t, ok, "%q does not start with an action", text)
	return action.Pipe.Cmds[0]
}

func TestContainsRedirectCall(t *testing.T) {
	for _, tt := range []struct {
		template string
		want     bool
	}{
		{template: `{{.Redirect "/x"}}`, want: true},
		{template: `{{.RedirectSeeOther "/x"}}`, want: true},
		{template: `{{.Header.Redirect "/x"}}`, want: true},
		{template: `{{(.A).Redirect}}`, want: true},
		{template: `{{.Name}}`},
		{template: `{{.Header}}`},
		{template: `{{"literal"}}`},
	} {
		t.Run(tt.template, func(t *testing.T) {
			assert.Equal(t, tt.want, containsRedirectCall(firstCommand(t, tt.template)), "containsRedirectCall(%s)", tt.template)
		})
	}

	t.Run("no command", func(t *testing.T) {
		assert.False(t, containsRedirectCall(nil), "containsRedirectCall(nil)")
	})
}

func TestCallsMethodOnTemplateData(t *testing.T) {
	for _, tt := range []struct {
		name     string
		template string
		want     bool
	}{
		{name: "safe method", template: `{{.Path}}`},
		{name: "safe method chained", template: `{{.Request.Method}}`},
		{name: "unknown method", template: `{{.Name}}`, want: true},
		{name: "redirect", template: `{{.Redirect "/x"}}`, want: true},
		{name: "dot passed to a function", template: `{{printf "%v" .}}`, want: true},
		{name: "safe field passed to a function", template: `{{printf "%v" .Path}}`},
		{name: "unknown field passed to a function", template: `{{printf "%v" .Name}}`, want: true},
		{name: "chain passed to a function", template: `{{printf "%v" (.A).B}}`, want: true},
		{name: "literal passed to a function", template: `{{printf "%v" "x"}}`},
		{name: "literal", template: `{{"x"}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, callsMethodOnTemplateData(firstCommand(t, tt.template)), "callsMethodOnTemplateData(%s)", tt.template)
		})
	}

	t.Run("no command", func(t *testing.T) {
		assert.False(t, callsMethodOnTemplateData(nil), "callsMethodOnTemplateData(nil)")
	})
}

// pipeOf is the parenthesised pipeline (arg).
func pipeOf(arg parse.Node) *parse.PipeNode {
	return &parse.PipeNode{Cmds: []*parse.CommandNode{{Args: []parse.Node{arg}}}}
}

func TestChainStartsAtTemplateData(t *testing.T) {
	for _, tt := range []struct {
		name string
		node parse.Node
		dot  bool
		want bool
	}{
		{name: "dot with dot as data", node: &parse.DotNode{}, dot: true, want: true},
		{name: "dot rebound", node: &parse.DotNode{}},
		{name: "root variable", node: &parse.VariableNode{Ident: []string{"$"}}, want: true},
		{name: "root variable with a field", node: &parse.VariableNode{Ident: []string{"$", "A"}}, want: true},
		{name: "local variable", node: &parse.VariableNode{Ident: []string{"$x"}}, dot: true},
		{name: "empty variable", node: &parse.VariableNode{}, dot: true},
		{name: "pipeline of dot with dot as data", node: pipeOf(&parse.DotNode{}), dot: true, want: true},
		{name: "pipeline of dot with dot rebound", node: pipeOf(&parse.DotNode{})},
		{name: "pipeline of the root variable", node: pipeOf(&parse.VariableNode{Ident: []string{"$"}}), want: true},
		{name: "pipeline selecting from dot", node: pipeOf(&parse.FieldNode{Ident: []string{"Result"}}), dot: true},
		{name: "pipeline declaring a variable", node: &parse.PipeNode{Decl: []*parse.VariableNode{{Ident: []string{"$x"}}}, Cmds: pipeOf(&parse.DotNode{}).Cmds}, dot: true},
		{name: "empty pipeline", node: &parse.PipeNode{}, dot: true},
		{name: "literal", node: &parse.StringNode{}, dot: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, chainStartsAtTemplateData(&parse.ChainNode{Node: tt.node}, tt.dot), "chainStartsAtTemplateData(%s, dot=%t)", tt.name, tt.dot)
		})
	}
}
