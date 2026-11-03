package rendering

import "strconv"

// Caller is a synthetic template that calls a {{define}} from one
// context, so the oracle can render the define's body where a real call
// site might put it.
type Caller struct {
	Name    string  // a short label for messages
	Source  string  // the caller's template source
	Context Context // the context its output is parsed in

	// CallSite reports that the define's body renders in a mode other
	// than collapse here (pre-line, preserve, an attribute value,
	// RCDATA), so its line breaks or bytes must stay exact. The formatter can keep them only when it
	// sees this call site, so the oracle formats the define together
	// with the caller. A collapse caller needs no such help: an uncalled
	// define is formatted as if called from collapse (spec §2).
	CallSite bool
}

// Callers returns the synthetic callers of the define named name:
// collapse <div>, inline between text in a <p>, a <tbody> when the
// body's first element is a <tr> (firstElement, lower case), a
// white-space:pre-line <div> (which keeps the LF count), a
// white-space:pre-wrap <div>, an attribute value and a <textarea>.
func Callers(name, firstElement string) []Caller {
	call := `{{template ` + strconv.Quote(name) + ` .}}`
	callers := []Caller{
		{Name: "div", Source: "<div>" + call + "</div>", Context: Body},
		{Name: "inline", Source: "<p>a" + call + "b</p>", Context: Body},
	}
	if firstElement == "tr" {
		callers = append(callers, Caller{Name: "table", Source: "<table><tbody>" + call + "</tbody></table>", Context: TableBody})
	}
	return append(callers,
		Caller{Name: "pre-line", Source: `<div style="white-space:pre-line">` + call + `</div>`, Context: Body, CallSite: true},
		Caller{Name: "pre-wrap", Source: `<div style="white-space:pre-wrap">` + call + `</div>`, Context: Body, CallSite: true},
		Caller{Name: "attribute", Source: `<p title="` + call + `"></p>`, Context: Body, CallSite: true},
		Caller{Name: "textarea", Source: "<textarea>" + call + "</textarea>", Context: Body, CallSite: true},
	)
}
