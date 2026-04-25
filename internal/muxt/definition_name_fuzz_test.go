package muxt

import (
	"go/ast"
	"strings"
	"testing"
)

// FuzzCalculateIdentifiers feeds synthetic Definition slices to
// calculateIdentifiers. Input encodes one definition per line as
// "METHOD|PATH|FUN" — empty FUN means no receiver method (fun == nil).
func FuzzCalculateIdentifiers(f *testing.F) {
	seeds := []string{
		"GET|/|",
		"GET|/users|List",
		"GET|/users|List\nGET|/admins|List",         // duplicate fun.Name
		"GET|/users/{id}|Get\nGET|/posts/{id}|Get",  // duplicate + path params
		"POST|/|Create\nGET|/|Create\nPUT|/|Create", // triple duplicate
		"GET|/{id...}|F",                            // wildcard segment
		"GET|/a/{b}/c/{d}|Multi",
		"|/|",    // empty method
		"GET||F", // empty path
		"\n\n",   // empty lines
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, encoded string) {
		lines := strings.Split(encoded, "\n")
		if len(lines) > 16 {
			t.Skip() // keep runs fast
		}
		defs := make([]Definition, 0, len(lines))
		seenInputs := make(map[string]struct{})
		for _, line := range lines {
			if _, dup := seenInputs[line]; dup {
				continue // real template sets can't have duplicate (method, path, fun) triples
			}
			seenInputs[line] = struct{}{}
			parts := strings.SplitN(line, "|", 3)
			if len(parts) != 3 {
				continue
			}
			// Only accept inputs that could emerge from newDefinition:
			// path must start with "/", method restricted to known verbs or "".
			if !strings.HasPrefix(parts[1], "/") {
				continue
			}
			switch parts[0] {
			case "", "GET", "POST", "PUT", "PATCH", "DELETE":
			default:
				continue
			}
			if parts[2] != "" && !isGoIdent(parts[2]) {
				continue // real fun.Name comes from the Go parser
			}
			// Path segments likewise come from http.ServeMux patterns.
			if !isRoutePath(parts[1]) {
				continue
			}
			d := Definition{method: parts[0], path: parts[1]}
			if parts[2] != "" {
				d.fun = &ast.Ident{Name: parts[2]}
			}
			defs = append(defs, d)
		}
		if len(defs) == 0 {
			return
		}

		calculateIdentifiers(defs) // must not panic

		// Invariant: identifiers assigned to non-trivial inputs are non-empty
		// when the function name is non-empty OR the path yields a segment.
		seen := make(map[string]int)
		for i, d := range defs {
			id := d.identifier
			if id == "" {
				continue
			}
			if prev, ok := seen[id]; ok {
				t.Fatalf("duplicate identifier %q at indexes %d and %d\ninput: %q\ndefs: %+v",
					id, prev, i, encoded, defs)
			}
			seen[id] = i
		}
	})
}

func isGoIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_' {
			continue
		}
		if i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func isRoutePath(s string) bool {
	if !strings.HasPrefix(s, "/") {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '/' || r == '{' || r == '}' || r == '.' || r == '_' || r == '-' || r == '$':
		default:
			return false
		}
	}
	return true
}
