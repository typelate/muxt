// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// Modifications copyright 2026 christopher hunter. all rights reserved.

package template

import (
	"iter"

	"github.com/typelate/muxt/internal/template/lex"
)

// Extract walks the parsed AST and yields {{define}} nodes (DefineNode)
// as separate trees. Each yielded pair is (template name, *Tree).
// The source tree itself is not yielded — only defines found within it.
func Extract(src *Tree) iter.Seq2[string, *Tree] {
	return func(yield func(string, *Tree) bool) {
		extractDefines(yield, src, src.Root)
	}
}

// extractDefines recursively walks the AST looking for DefineNode instances.
// Each DefineNode is converted into a separate Tree and yielded.
// Returns false if the yield function returned false (caller wants to stop).
func extractDefines(yield func(string, *Tree) bool, parent *Tree, n Node) bool {
	if n == nil {
		return true
	}
	switch n := n.(type) {
	case *DefineNode:
		dt := &Tree{
			Name: n.Name,
		}
		dt.Root = n.Body
		dt.ParseName = parent.ParseName
		dt.Filepath = parent.Filepath
		dt.text = parent.text
		dt.Diagnostics = diagnosticsInRange(parent.Diagnostics, n.Body.Start(), n.Body.End())
		dt.RootTrimFirst = n.StartTrimRight
		dt.RootTrimLast = n.EndTrimLeft
		return yield(n.Name, dt)
	case *BlockNode:
		// Root-level trim (RootTrimFirst/RootTrimLast) is not set here: a
		// {{block}}'s canonical Tree — the one actually executed at
		// runtime and reused by check/internal/asteval's static loader —
		// is built once, in blockControl (parse.go), and registered
		// directly into the parser's treeSet; this Extract-produced Tree
		// is a second, independent reconstruction that existing callers
		// (formatter, coverage) use only for source ranges and structure,
		// never for execution.
		dt := &Tree{
			Name: n.Name,
		}
		dt.Root = n.Body
		dt.ParseName = parent.ParseName
		dt.Filepath = parent.Filepath
		dt.text = parent.text
		dt.Diagnostics = diagnosticsInRange(parent.Diagnostics, n.Body.Start(), n.Body.End())
		return yield(n.Name, dt)
	case *ListNode:
		if n != nil {
			for _, child := range n.Nodes {
				if !extractDefines(yield, parent, child) {
					return false
				}
			}
		}
	case *HTMLDocumentFragmentNode:
		for _, child := range n.Children {
			if !extractDefines(yield, parent, child) {
				return false
			}
		}
	case *HTMLElementNode:
		for _, attr := range n.Attr {
			if !extractDefines(yield, parent, attr) {
				return false
			}
		}
		for _, child := range n.Children {
			if !extractDefines(yield, parent, child) {
				return false
			}
		}
	case *IfNode:
		if !extractDefines(yield, parent, n.List) {
			return false
		}
		if !extractDefines(yield, parent, n.ElseList) {
			return false
		}
	case *RangeNode:
		if !extractDefines(yield, parent, n.List) {
			return false
		}
		if !extractDefines(yield, parent, n.ElseList) {
			return false
		}
	case *WithNode:
		if !extractDefines(yield, parent, n.List) {
			return false
		}
		if !extractDefines(yield, parent, n.ElseList) {
			return false
		}
	}
	return true
}

// diagnosticsInRange returns the diagnostics from ds whose position
// falls within [start, end) — the extracted body's source range. A
// diagnostic recorded against the parent tree during parsing belongs
// to an extracted define/block tree iff it was recorded somewhere
// inside that body.
func diagnosticsInRange(ds Diagnostics, start, end lex.Pos) Diagnostics {
	var out Diagnostics
	for _, d := range ds {
		if d.Pos >= start && d.Pos < end {
			out = append(out, d)
		}
	}
	return out
}
