package template

import (
	"io"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/typelate/muxt/internal/template/lex"
)

// CoverProfile collects coverage information during template execution.
//
// Usage: create a CoverProfile, pass it to execute via WithCoverProfile,
// then execute templates. Trees are added automatically during execution.
// After execution, call WriteTo to produce a Go coverage profile.
//
// Coverage counters are embedded atomic.Int64 fields on AST nodes.
// Execution increments them via .Add(1); WriteTo reads them via .Load().
type CoverProfile struct {
	mu    sync.Mutex
	trees []*Tree
	seen  map[*Tree]bool
}

// AddTree registers a tree for coverage reporting. Safe to call
// multiple times for the same tree (idempotent). Must be called
// before or during execution so WriteTo includes the tree's counters.
func (cp *CoverProfile) AddTree(tree *Tree) {
	if cp == nil || tree == nil {
		return
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()
	if cp.seen == nil {
		cp.seen = make(map[*Tree]bool)
	}
	if cp.seen[tree] {
		return
	}
	cp.seen[tree] = true
	cp.trees = append(cp.trees, tree)
}

// Init registers the tree for coverage. Called by the execution engine.
func (cp *CoverProfile) Init(tree *Tree) {
	cp.AddTree(tree)
}

type coverBlock struct {
	parseName           string
	startLine, startCol int
	endLine, endCol     int
	numStmt             int
	count               int64
}

// WriteTo writes the coverage profile in the standard Go coverage format.
// It walks each registered tree's AST, reading counters from nodes.
func (cp *CoverProfile) WriteTo(w io.Writer) (int64, error) {
	if cp == nil {
		return 0, nil
	}
	cp.mu.Lock()
	trees := make([]*Tree, len(cp.trees))
	copy(trees, cp.trees)
	cp.mu.Unlock()
	var blocks []coverBlock
	for _, tree := range trees {
		blocks = coverWalkNode(tree, tree.Root, blocks)
	}
	blocks = collapseBlocks(blocks)

	var total int64
	n, err := io.WriteString(w, "mode: set\n")
	total += int64(n)
	if err != nil {
		return total, err
	}
	var buf []byte
	for _, b := range blocks {
		buf = buf[:0]
		buf = append(buf, b.parseName...)
		buf = append(buf, ':')
		buf = strconv.AppendInt(buf, int64(b.startLine), 10)
		buf = append(buf, '.')
		buf = strconv.AppendInt(buf, int64(b.startCol), 10)
		buf = append(buf, ',')
		buf = strconv.AppendInt(buf, int64(b.endLine), 10)
		buf = append(buf, '.')
		buf = strconv.AppendInt(buf, int64(b.endCol), 10)
		buf = append(buf, ' ')
		buf = strconv.AppendInt(buf, int64(b.numStmt), 10)
		buf = append(buf, ' ')
		buf = strconv.AppendInt(buf, b.count, 10)
		buf = append(buf, '\n')
		n, err := w.Write(buf)
		total += int64(n)
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// coverWalkNode recursively walks the AST, emitting coverage blocks for
// coverable nodes. Blocks are flat and non-overlapping.
func coverWalkNode(tree *Tree, node Node, blocks []coverBlock) []coverBlock {
	if node == nil {
		return blocks
	}
	switch n := node.(type) {
	case *ListNode:
		if n == nil {
			return blocks
		}
		for _, child := range n.Nodes {
			blocks = coverWalkNode(tree, child, blocks)
		}
	case *HTMLDocumentFragmentNode:
		for _, child := range n.Children {
			blocks = coverWalkNode(tree, child, blocks)
		}
	case *TextNode:
		blocks = appendLeafBlock(tree, blocks, n.Start(), n.End(), &n.count)
	case *ActionNode:
		blocks = appendLeafBlock(tree, blocks, n.Start(), n.End(), &n.count)
	case *TemplateNode:
		blocks = appendLeafBlock(tree, blocks, n.Start(), n.End(), &n.count)
	case *BreakNode:
		blocks = appendLeafBlock(tree, blocks, n.Start(), n.End(), &n.count)
	case *ContinueNode:
		blocks = appendLeafBlock(tree, blocks, n.Start(), n.End(), &n.count)
	case *HTMLAttributeNode:
		blocks = appendLeafBlock(tree, blocks, n.Start(), n.End(), &n.count)
	case *HTMLCommentNode:
		blocks = appendLeafBlock(tree, blocks, n.Start(), n.End(), &n.count)
	case *HTMLDoctypeNode:
		blocks = appendLeafBlock(tree, blocks, n.Start(), n.End(), &n.count)
	case *IfNode:
		blocks = coverWalkBranch(tree, blocks, &n.BranchNode)
	case *RangeNode:
		blocks = coverWalkBranch(tree, blocks, &n.BranchNode)
	case *WithNode:
		blocks = coverWalkBranch(tree, blocks, &n.BranchNode)
	case *HTMLElementNode:
		blocks = coverWalkElement(tree, blocks, n)
	case *DefineNode:
		blocks = coverWalkNode(tree, n.Body, blocks)
	case *BlockNode:
		blocks = coverWalkNode(tree, n.Body, blocks)
	case *CommentNode:
		// Template comments produce no output — no coverage.
	}
	return blocks
}

// coverWalkBranch emits blocks for a branch node's structural regions,
// then recurses into children.
func coverWalkBranch(tree *Tree, blocks []coverBlock, b *BranchNode) []coverBlock {
	// {{if/range/with ...}} — from branch start to List start
	if b.List != nil {
		blocks = appendBlock(tree, blocks, b.start, b.List.Start(), &b.openCount)
	}
	// Walk the true-branch body
	blocks = coverWalkNode(tree, b.List, blocks)
	// {{else}} — from List end to ElseList start
	if b.ElseList != nil {
		blocks = appendBlock(tree, blocks, b.List.End(), b.ElseList.Start(), &b.elseCount)
		blocks = coverWalkNode(tree, b.ElseList, blocks)
	}
	// {{end}} — from last body end to branch end
	lastEnd := b.start
	if b.ElseList != nil {
		lastEnd = b.ElseList.End()
	} else if b.List != nil {
		lastEnd = b.List.End()
	}
	blocks = appendBlock(tree, blocks, lastEnd, b.end, &b.endCount)
	return blocks
}

// coverWalkElement emits blocks for an element's coverable regions.
// Each block counts as 1 statement:
//   - <tag  — from element start to AttrStart (openCount)
//   - each attribute — walked, producing its own blocks
//   - >    — from last attr end to AttrEnd (attrCount)
//   - each child — walked
//   - </tag> — from EndTagStart to element end (closeCount)
func coverWalkElement(tree *Tree, blocks []coverBlock, elem *HTMLElementNode) []coverBlock {
	// <tag
	blocks = appendBlock(tree, blocks, elem.Start(), elem.AttrStart, &elem.openCount)
	// Walk attribute children; each adds its own blocks.
	for _, attr := range elem.Attr {
		blocks = coverWalkNode(tree, attr, blocks)
	}
	// Closing ">"
	attrCloseStart := elem.AttrStart
	if len(elem.Attr) > 0 {
		attrCloseStart = elem.Attr[len(elem.Attr)-1].End()
	}
	blocks = appendBlock(tree, blocks, attrCloseStart, elem.AttrEnd, &elem.attrCount)
	if !elem.Void {
		// Walk content children.
		for _, child := range elem.Children {
			blocks = coverWalkNode(tree, child, blocks)
		}
		// Close tag </tag>
		blocks = appendBlock(tree, blocks, elem.EndTagStart, elem.end, &elem.closeCount)
	}
	return blocks
}

func appendLeafBlock(tree *Tree, blocks []coverBlock, start, end lex.Pos, count *atomic.Int64) []coverBlock {
	return appendBlock(tree, blocks, start, end, count)
}

func appendBlock(tree *Tree, blocks []coverBlock, start, end lex.Pos, count *atomic.Int64) []coverBlock {
	if start == end {
		return blocks
	}
	startPos := tree.PositionAt(start)
	endPos := tree.PositionAt(end)
	return append(blocks, coverBlock{
		parseName: startPos.ParseName,
		startLine: startPos.Line, startCol: startPos.Column,
		endLine: endPos.Line, endCol: endPos.Column,
		numStmt: 1,
		count:   count.Load(),
	})
}

// collapseBlocks merges consecutive blocks with the same count.
func collapseBlocks(blocks []coverBlock) []coverBlock {
	if len(blocks) == 0 {
		return nil
	}
	var result []coverBlock
	i := 0
	for i < len(blocks) {
		merged := blocks[i]
		j := i + 1
		for j < len(blocks) {
			next := blocks[j]
			if next.count != merged.count {
				break
			}
			merged.numStmt += next.numStmt
			if next.endLine > merged.endLine || (next.endLine == merged.endLine && next.endCol > merged.endCol) {
				merged.endLine = next.endLine
				merged.endCol = next.endCol
			}
			j++
		}
		result = append(result, merged)
		i = j
	}
	return result
}
