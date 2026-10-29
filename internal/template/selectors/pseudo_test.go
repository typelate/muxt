package selectors_test

import (
	"testing"

	"github.com/typelate/muxt/internal/template/selectors"
)

func TestSelectorMatch_PseudoClasses(t *testing.T) {
	tree := parseHTML(t, `<ul><li id="a">a</li>{{if .X}}<li id="b"></li>{{end}}<li id="c"><span></span></li></ul>`+
		`<ol>{{if .Y}}<li id="m1"></li>{{end}}<li id="m2"></li></ol>`+
		`<div id="empty"></div><div id="ws">   </div><div id="dyn">{{.Z}}</div>`)

	for _, tt := range []struct {
		name     string
		selector string
		nodeID   string
		want     selectors.Trit
	}{
		{":first-child certain", "li:first-child", "a", selectors.TritYes},
		{":first-child certain predecessor", "li:first-child", "c", selectors.TritNo},
		{":first-child only conditional before", ":first-child", "m2", selectors.TritMaybe},

		{":last-child certain", "li:last-child", "c", selectors.TritYes},
		{":last-child certain follower", "li:last-child", "a", selectors.TritNo},

		{":empty truly empty", "div:empty", "empty", selectors.TritYes},
		{":empty whitespace only", "div:empty", "ws", selectors.TritYes},
		{":empty has element child", "li:empty", "c", selectors.TritNo},
		{":empty dynamic content", "div:empty", "dyn", selectors.TritMaybe},

		{":not different type", "li:not(p)", "a", selectors.TritYes},
		{":not self type", "li:not(li)", "a", selectors.TritNo},
		{":not nested pseudo", "li:not(:first-child)", "c", selectors.TritYes},
		{":not nested pseudo on first", "li:not(:first-child)", "a", selectors.TritNo},
	} {
		t.Run(tt.name, func(t *testing.T) {
			node := elementByID(t, tree, tt.nodeID)
			sel := selectors.MustCompile(tt.selector)
			if _, got := sel.Matches(node); got != tt.want {
				t.Errorf("MustCompile(%q).Match(#%s) = %s, want %s",
					tt.selector, tt.nodeID, got, tt.want)
			}
		})
	}
}
