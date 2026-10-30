package analysis

import (
	"testing"

	"github.com/typelate/muxt/internal/css/csstree"
	"github.com/typelate/muxt/internal/template/selectors"
)

func TestUASheetSelectorsCompile(t *testing.T) {
	ast, err := csstree.Parse(uaCSS, csstree.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	nodes := csstree.FindAll(ast, func(_ *csstree.WalkContext, node csstree.Node, _ *csstree.ListItem[csstree.Node], _ *csstree.List[csstree.Node]) bool {
		return node.Type() == "Selector"
	})
	count := len(nodes)
	for _, node := range nodes {
		s, err := csstree.Generate(node, csstree.GenerateOptions{})
		if err != nil {
			t.Errorf("generating selector: %v", err)
			continue
		}
		if _, err := selectors.Compile(s); err != nil {
			t.Errorf("selectors.Compile(%q): %v", s, err)
		}
	}
	if count < 100 {
		t.Errorf("found %d selectors in ua.css, want at least 100", count)
	}
}
