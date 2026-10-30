package analysis

import _ "embed"

// uaCSS is the HTML Standard's user-agent style sheet, reduced to the
// display and white-space declarations (see ua.css for its provenance). It
// is the default origin of the whitespace classifier's cascade.
//
// ua.css is generated from the Rendering section of the HTML Standard.
// Generation needs the network, so there is no go:generate line; to
// regenerate, run from the repository root:
//
//	curl -fsSL https://html.spec.whatwg.org/multipage/rendering.html -o /tmp/rendering.html
//	go run ./internal/analysis/html/uasheet -i /tmp/rendering.html -o internal/analysis/html/ua.css -date $(date +%F)
//
//go:embed ua.css
var uaCSS string
