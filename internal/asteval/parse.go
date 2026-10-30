package asteval

import (
	"cmp"
	"go/types"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// ParseTrees parses template text into the trees it defines, the way
// muxt's template loading (the html/template fork's Template.Parse)
// parses it.
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
func ParseTrees(name, text, leftDelim, rightDelim string, functions map[string]*types.Signature) (map[string]*template.Tree, error) {
	_, hasBreak := functions["break"]
	_, hasContinue := functions["continue"]
	treeSet := make(map[string]*template.Tree)
	root, err := template.ParseTree(name, name, text, treeSet, lex.Options{
		EmitComment: true,
		ParseHTML:   true,
		LeftDelim:   cmp.Or(leftDelim, "{{"),
		RightDelim:  cmp.Or(rightDelim, "}}"),
		BreakOK:     !hasBreak,
		ContinueOK:  !hasContinue,
	})
	if err != nil {
		return nil, err
	}
	// The parser registered the top-level tree and any blocks; the define
	// clauses it kept inline are taken out here. As text/template does,
	// a tree replaces one already registered under its name only when
	// that one is empty, so a top-level template holding nothing but
	// define clauses gives way to a define of its own name.
	delete(treeSet, name)
	add := func(name string, tree *template.Tree) {
		if existing, ok := treeSet[name]; !ok || template.IsEmptyTree(existing.Root) {
			treeSet[name] = tree
		}
	}
	for definedName, tree := range template.Extract(root) {
		add(definedName, tree)
	}
	add(name, root)
	for _, tree := range treeSet {
		tree.HTMLMode = true
		template.AnnotateHTMLContexts(tree.Root)
	}
	return treeSet, nil
}
