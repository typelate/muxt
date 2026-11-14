package muxt

import (
	"testing"

	"github.com/typelate/htt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func firstCommand(t *testing.T, text string) *htt.CommandNode {
	t.Helper()
	_, page := pageIn(t, text)
	action, ok := page.Tree.Root.Nodes[0].(*htt.ActionNode)
	require.True(t, ok, "%q does not start with an action", text)
	return action.Pipe.Commands[0]
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
		{template: `{{$.Redirect "/x"}}`, want: true},
		{template: `{{$.RedirectFound "/x"}}`, want: true},
		{template: `{{$.Name}}`},
		{template: `{{$}}`},
		{template: `{{.Name}}`},
		{template: `{{.Header}}`},
		{template: `{{"literal"}}`},
	} {
		t.Run(tt.template, func(t *testing.T) {
			assert.Equal(t, tt.want, containsRedirectCall(firstCommand(t, tt.template), true), "containsRedirectCall(%s, true)", tt.template)
		})
	}

	// Where dot is not TemplateData, a redirect named on it is a field of
	// something else; one named on the root or a variable still counts.
	for _, tt := range []struct {
		template string
		want     bool
	}{
		{template: `{{.Redirect "/x"}}`},
		{template: `{{.Header.Redirect "/x"}}`},
		{template: `{{$.Redirect "/x"}}`, want: true},
		{template: `{{(.A).Redirect}}`, want: true},
	} {
		t.Run("rebound "+tt.template, func(t *testing.T) {
			assert.Equal(t, tt.want, containsRedirectCall(firstCommand(t, tt.template), false), "containsRedirectCall(%s, false)", tt.template)
		})
	}

	t.Run("no command", func(t *testing.T) {
		assert.False(t, containsRedirectCall(nil, true), "containsRedirectCall(nil, true)")
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
		{name: "safe method from root", template: `{{$.Path}}`},
		{name: "unknown method from root", template: `{{$.Name}}`, want: true},
		{name: "root passed to a function", template: `{{printf "%v" $}}`, want: true},
		{name: "root alone", template: `{{$}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, callsMethodOnTemplateData(firstCommand(t, tt.template), true), "callsMethodOnTemplateData(%s, true)", tt.template)
		})
	}

	for _, tt := range []struct {
		name     string
		template string
		want     bool
	}{
		{name: "unknown method on a rebound dot", template: `{{.Name}}`},
		{name: "rebound dot passed to a function", template: `{{printf "%v" .}}`},
		{name: "unknown field of a rebound dot passed to a function", template: `{{printf "%v" .Name}}`},
		{name: "unknown method from root under a rebound dot", template: `{{$.Name}}`, want: true},
		{name: "root passed to a function under a rebound dot", template: `{{printf "%v" $}}`, want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, callsMethodOnTemplateData(firstCommand(t, tt.template), false), "callsMethodOnTemplateData(%s, false)", tt.template)
		})
	}

	t.Run("no command", func(t *testing.T) {
		assert.False(t, callsMethodOnTemplateData(nil, true), "callsMethodOnTemplateData(nil, true)")
	})
}

// pipeOf is the parenthesised pipeline (arg).
func pipeOf(arg htt.Node) *htt.PipeNode {
	return &htt.PipeNode{Commands: []*htt.CommandNode{{Args: []htt.Node{arg}}}}
}

func TestChainStartsAtTemplateData(t *testing.T) {
	for _, tt := range []struct {
		name string
		node htt.Node
		dot  bool
		want bool
	}{
		{name: "dot with dot as data", node: &htt.DotNode{}, dot: true, want: true},
		{name: "dot rebound", node: &htt.DotNode{}},
		{name: "root variable", node: &htt.VariableNode{Ident: []string{"$"}}, want: true},
		{name: "root variable with a field", node: &htt.VariableNode{Ident: []string{"$", "A"}}, want: true},
		{name: "local variable", node: &htt.VariableNode{Ident: []string{"$x"}}, dot: true},
		{name: "empty variable", node: &htt.VariableNode{}, dot: true},
		{name: "pipeline of dot with dot as data", node: pipeOf(&htt.DotNode{}), dot: true, want: true},
		{name: "pipeline of dot with dot rebound", node: pipeOf(&htt.DotNode{})},
		{name: "pipeline of the root variable", node: pipeOf(&htt.VariableNode{Ident: []string{"$"}}), want: true},
		{name: "pipeline selecting from dot", node: pipeOf(&htt.FieldNode{Ident: []string{"Result"}}), dot: true},
		{name: "pipeline declaring a variable", node: &htt.PipeNode{Decl: []*htt.VariableNode{{Ident: []string{"$x"}}}, Commands: pipeOf(&htt.DotNode{}).Commands}, dot: true},
		{name: "empty pipeline", node: &htt.PipeNode{}, dot: true},
		{name: "literal", node: &htt.StringNode{}, dot: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, chainStartsAtTemplateData(&htt.ChainNode{Node: tt.node}, tt.dot), "chainStartsAtTemplateData(%s, dot=%t)", tt.name, tt.dot)
		})
	}
}
