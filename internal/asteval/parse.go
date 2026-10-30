package asteval

import (
	"cmp"
	"go/types"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// ParseTrees parses template text into the trees it defines, the way
// muxt's template loading (the html/template fork's Template.Parse)
// parses it. html selects html/template's parse, with HTML structure;
// false parses as text/template does, so a text/template set's JSON or
// prose with a '<' in it parses.
//
// The result maps each template name to its tree: the top-level
// template under name, and one entry per define and block clause. The
// top-level tree keeps its define and block clauses inline as
// DefineNode and BlockNode; the parser does not remove them.
//
// Node positions in the returned trees are byte offsets into text, which
// is what lets a caller relate a node back to the source it was written
// in.
//
// functions supplies the names the template set registered beyond the
// builtins; it may be nil. The parser does not check that a called
// function exists. The names matter only because registering a function
// named break or continue turns that keyword off, as it does in
// Template.Parse.
func ParseTrees(name, text, leftDelim, rightDelim string, functions map[string]*types.Signature, html bool) (map[string]*template.Tree, error) {
	_, hasBreak := functions["break"]
	_, hasContinue := functions["continue"]
	treeSet := make(map[string]*template.Tree)
	root, err := template.ParseTree(name, name, text, treeSet, lex.Options{
		EmitComment: true,
		ParseHTML:   html,
		LeftDelim:   cmp.Or(leftDelim, "{{"),
		RightDelim:  cmp.Or(rightDelim, "}}"),
		BreakOK:     !hasBreak,
		ContinueOK:  !hasContinue,
	})
	if err != nil {
		return nil, err
	}
	// The parser registered the top-level tree and each define and block,
	// with text/template's precedence between definitions of one name:
	// a second definition that is not empty is a parse error, and a top-level
	// template holding nothing but define clauses gives way to a define of
	// its own name.
	treeSet = template.DefinedTrees(root, treeSet)
	if html {
		for _, tree := range treeSet {
			tree.HTMLMode = true
			template.AnnotateHTMLContexts(tree.Root)
		}
	}
	return treeSet, nil
}
