package template_test

import (
	"strings"
	"testing"

	"github.com/typelate/muxt/template"
)

func TestPipeNode_HeadCommands(t *testing.T) {
	for _, tt := range []struct {
		name   string
		action string
		want   []string
	}{
		{name: "bare field", action: `{{.X}}`, want: []string{`.X`}},
		{name: "parenthesized dot", action: `{{(.)}}`, want: []string{`.`}},
		{name: "parenthesized call", action: `{{(and .A .B)}}`, want: []string{`and .A .B`}},
		{name: "nested parentheses", action: `{{((.X))}}`, want: []string{`.X`}},
		{name: "inner pipeline", action: `{{(.X | printf "%v")}}`, want: []string{`.X`, `printf "%v"`}},
		{name: "later stages kept", action: `{{(.X) | printf "%v" | print}}`, want: []string{`.X`, `printf "%v"`, `print`}},
		{name: "head of inner pipeline", action: `{{((.X | print) | printf "%v")}}`, want: []string{`.X`, `print`, `printf "%v"`}},
		{name: "outer declaration", action: `{{$x := (.X)}}`, want: []string{`.X`}},
		{name: "inner declaration kept", action: `{{($y := .X)}}`, want: []string{`($y := .X)`}},
		{name: "argument kept", action: `{{print (.X)}}`, want: []string{`print (.X)`}},
		{name: "head argument with more args kept", action: `{{(.F) 1}}`, want: []string{`(.F) 1`}},
		{name: "later stage kept", action: `{{.X | (print)}}`, want: []string{`.X`, `(print)`}},
		{name: "field chain kept", action: `{{(.X).Y}}`, want: []string{`(.X).Y`}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tree := parseText(t, tt.name, tt.action)
			action, ok := tree.Root.Nodes[0].(*template.ActionNode)
			if !ok {
				t.Fatalf("node = %T, want *template.ActionNode", tree.Root.Nodes[0])
			}
			var got []string
			for _, cmd := range action.Pipe.HeadCommands() {
				got = append(got, cmd.String())
			}
			if strings.Join(got, " | ") != strings.Join(tt.want, " | ") {
				t.Errorf("HeadCommands of %s = %q, want %q", tt.action, got, tt.want)
			}
		})
	}
}

func TestPipeNode_HeadCommands_doesNotAliasInnerCommands(t *testing.T) {
	const action = `{{(.X | print) | printf "%v"}}`
	tree := parseText(t, "t", action)
	pipe := tree.Root.Nodes[0].(*template.ActionNode).Pipe
	head := pipe.HeadCommands()
	for i := range head {
		head[i] = nil
	}
	inner := pipe.Commands[0].Args[0].(*template.PipeNode)
	for i, cmd := range inner.Commands {
		if cmd == nil {
			t.Errorf("inner.Commands[%d] = nil after clearing HeadCommands", i)
		}
	}
	if pipe.Commands[1] == nil {
		t.Error("pipe.Commands[1] = nil after clearing HeadCommands")
	}
}
