package csstree_test

import (
	"fmt"
	"log"

	"github.com/typelate/muxt/internal/css/csstree"
)

func Example() {
	ast, err := csstree.Parse(`.a { color: red; margin: 0 auto }`, csstree.ParseOptions{})
	if err != nil {
		log.Fatal(err)
	}

	err = csstree.Walk(ast, csstree.WalkOptions{
		Visit: "Declaration",
		Enter: func(_ *csstree.WalkContext, node csstree.Node, _ *csstree.ListItem[csstree.Node], _ *csstree.List[csstree.Node]) csstree.WalkAction {
			fmt.Println("property:", node.(*csstree.Declaration).Property)
			return csstree.WalkContinue
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	css, err := csstree.Generate(ast, csstree.GenerateOptions{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(css)

	lexer := csstree.DefaultLexer()
	fmt.Println(lexer.MatchPropertyString("color", "red").Error == nil)
	fmt.Println(lexer.MatchPropertyString("color", "10px").Error)

	// Output:
	// property: color
	// property: margin
	// .a{color:red;margin:0 auto}
	// true
	// Mismatch
	//   syntax: <color>
	//    value: 10px
	//   --------^
}
