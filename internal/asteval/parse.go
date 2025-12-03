package asteval

import (
	"go/token"
	"reflect"
	"strings"
	"text/template/parse"
)

// NewParseNodePosition uses reflection to access private field "text"
func NewParseNodePosition(tree *parse.Tree, n parse.Node) token.Position {
	pos := int(n.Position())
	tr := reflect.ValueOf(tree)
	fullText := tr.Elem().FieldByName("text").String()
	text := fullText[:pos]
	byteNum := strings.LastIndex(text, "\n")
	if byteNum == -1 {
		byteNum = pos // On first line.
	} else {
		byteNum++ // After the newline.
		byteNum = pos - byteNum
	}
	lineNum := 1 + strings.Count(text, "\n")
	return token.Position{
		Filename: tree.ParseName,
		Column:   byteNum,
		Line:     lineNum,
		Offset:   pos,
	}
}
