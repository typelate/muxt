//go:build grammarfuzz

package format

import (
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/template/format/internal/rendering"
)

// TestGrammar formats random templates from a small HTML and template
// grammar and checks that each output is idempotent and renders the same
// page as its input under every dot from fuzzDots. Unlike the byte-level
// fuzz targets it reaches nested control flow, tables, hidden elements
// and trim markers in a few seconds.
//
//	go test -tags grammarfuzz -run TestGrammar ./internal/template/format
//
// GRAMMAR_N sets the number of inputs (default 20000), GRAMMAR_SEED the
// generator seed (default 0), and GRAMMAR_MISORDER=1 adds misordered end
// tags such as <div><b>x</div>y</b>.
func TestGrammar(t *testing.T) {
	n := grammarEnvInt(t, "GRAMMAR_N", 20000)
	seed := grammarEnvInt(t, "GRAMMAR_SEED", 0)
	g := gram{
		r:        rand.New(rand.NewPCG(uint64(seed), 1)),
		misorder: os.Getenv("GRAMMAR_MISORDER") != "",
	}
	idem, rend, parseErrs, shown := 0, 0, 0, 0
	for range n {
		input := g.list(3)
		out, err := Source("g.gohtml", input)
		if err != nil {
			// The generator writes only templates that parse, so an
			// error here is a generator or parser bug worth seeing.
			parseErrs++
			if parseErrs <= 5 {
				t.Logf("Source(input) error: %v\n  input: %q", err, input)
			}
			continue
		}
		again, err := Source("g.gohtml", out)
		if err != nil || again != out {
			idem++
			if shown < 10 {
				shown++
				t.Errorf("not idempotent\n  input: %q\n  -> %q\n  -> %q", input, out, again)
			}
			continue
		}
		for _, dot := range fuzzDots() {
			want, err := render(input, "", dot)
			if err != nil {
				continue
			}
			if _, err := rendering.Tokens(want, rendering.Body, nil); err != nil {
				continue
			}
			got, err := render(out, "", dot)
			if err != nil {
				rend++
				if shown < 40 {
					shown++
					t.Errorf("output does not execute with %v: %v\n  in:  %q\n  out: %q", dot, err, input, out)
				}
				break
			}
			diff, err := rendering.Diff(want, got, rendering.Body, nil)
			if err != nil || diff != "" {
				rend++
				if shown < 40 {
					shown++
					t.Errorf("rendering changed with %v: %s %v\n  in:  %q\n  out: %q", dot, diff, err, input, out)
				}
				break
			}
		}
	}
	t.Logf("grammar: seed %d, misorder %v: %d inputs, %d parse errors, %d idempotency, %d rendering", seed, g.misorder, n, parseErrs, idem, rend)
}

func grammarEnvInt(t *testing.T, name string, fallback int) int {
	t.Helper()
	s := os.Getenv(name)
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return n
}

type gram struct {
	r        *rand.Rand
	misorder bool
}

var (
	gramWS   = []string{"", "", " ", "\n", "\n  ", "\t", "  ", "\n\n"}
	gramTags = []string{"div", "p", "span", "li", "ul", "b", "h1", "em", "section", "a", "dd", "dt", "dl", "address"}
)

func (g gram) ws() string { return gramWS[g.r.IntN(len(gramWS))] }

func (g gram) list(depth int) string {
	var sb strings.Builder
	n := g.r.IntN(4)
	sb.WriteString(g.ws())
	for range n {
		sb.WriteString(g.item(depth))
		sb.WriteString(g.ws())
	}
	return sb.String()
}

func (g gram) trimL() string {
	if g.r.IntN(6) == 0 {
		return "- "
	}
	return ""
}

func (g gram) trimR() string {
	if g.r.IntN(6) == 0 {
		return " -"
	}
	return ""
}

func (g gram) item(depth int) string {
	k := g.r.IntN(12)
	if depth <= 0 {
		k = g.r.IntN(3)
	}
	switch k {
	case 0:
		return []string{"a", "x y", "b"}[g.r.IntN(3)]
	case 1:
		return "{{" + g.trimL() + ".X" + g.trimR() + "}}"
	case 2:
		return []string{"<br>", "<img src=x>", "{{/* c */}}", "<!-- c -->"}[g.r.IntN(4)]
	case 3:
		body := g.list(depth - 1)
		s := "{{" + g.trimL() + "if .Show" + g.trimR() + "}}" + body
		if g.r.IntN(2) == 0 {
			s += "{{" + g.trimL() + "else" + g.trimR() + "}}" + g.list(depth-1)
		}
		return s + "{{" + g.trimL() + "end" + g.trimR() + "}}"
	case 4:
		return "{{range .Items}}" + g.list(depth-1) + "{{end}}"
	case 5:
		return "<span hidden>" + g.list(depth-1) + "</span>"
	case 6:
		return "<table>" + g.ws() + "<tr>" + g.ws() + "<td>" + g.list(depth-1) + "</td>" + g.ws() + "</tr>" + g.ws() + "</table>"
	case 8:
		// foster-parented content directly in table, tbody or tr
		f := func() string {
			if g.r.IntN(2) == 0 {
				return g.list(0)
			}
			return g.item(depth - 1)
		}
		switch g.r.IntN(3) {
		case 0:
			return "<table>" + f() + "<tr><td>" + g.list(depth-1) + "</td></tr></table>"
		case 1:
			return "<table><tbody>" + f() + "<tr><td>t</td></tr></tbody></table>"
		default:
			return "<table><tr>" + f() + "<td>t</td>" + g.ws() + "</tr></table>"
		}
	case 9:
		if !g.misorder {
			break
		}
		// misordered end tags
		a := gramTags[g.r.IntN(len(gramTags))]
		b := []string{"b", "em", "a", "span"}[g.r.IntN(4)]
		return "<" + a + ">" + g.list(depth-1) + "<" + b + ">" + g.list(depth-1) + "</" + a + ">" + g.list(depth-1) + "</" + b + ">"
	default:
		t := gramTags[g.r.IntN(len(gramTags))]
		return "<" + t + ">" + g.list(depth-1) + "</" + t + ">"
	}
	return "<div>" + g.list(depth-1) + "</div>"
}
