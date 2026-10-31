package format

import (
	"errors"
	"strings"
	"testing"
)

func TestDirectives(t *testing.T) {
	for _, tt := range []struct {
		name    string
		input   string
		want    string // the output; input when empty
		wantErr string
		kept    []string // substrings that must appear in the output as written
	}{
		{
			name:  "off and on keep the region",
			input: "<div>\n{{/* muxt:fmt off */}}\n<p>  a  </p>\n   <p>b</p>\n{{/* muxt:fmt on */}}\n<p>c</p>\n</div>\n",
			want:  "<div>\n\t{{/* muxt:fmt off */}}\n<p>  a  </p>\n   <p>b</p>\n{{/* muxt:fmt on */}}\n\t<p>c</p>\n</div>\n",
			kept:  []string{"\n<p>  a  </p>\n   <p>b</p>\n"},
		},
		{
			name:  "off without on runs to the end of its parent",
			input: "<div>\n{{/* muxt:fmt off */}}\n<p>  a  </p>\n   <p>b</p>\n</div>\n<p>\n  c\n</p>\n",
			want:  "<div>\n\t{{/* muxt:fmt off */}}\n<p>  a  </p>\n   <p>b</p>\n</div>\n<p>\n\tc\n</p>\n",
			kept:  []string{"\n<p>  a  </p>\n   <p>b</p>\n</div>"},
		},
		{
			name:  "off first in the file keeps the file",
			input: "  {{/* muxt:fmt off */}}\n<div>\n<p>  a  </p>\n      <p>b</p>\n</div>\n",
		},
		{
			name:  "preserve first in the file keeps the file",
			input: "{{/* muxt:fmt preserve */}}\n<div>\n<p>  a  </p>\n      <p>b</p>\n</div>\n<ul>\n<li>x</li></ul>\n",
		},
		{
			name:  "off first and ended by on is a region",
			input: "{{/* muxt:fmt off */}}\n<p>  a  </p>\n{{/* muxt:fmt on */}}\n<div>\n<p>b</p>\n      </div>\n",
			want:  "{{/* muxt:fmt off */}}\n<p>  a  </p>\n{{/* muxt:fmt on */}}\n<div>\n\t<p>b</p>\n</div>\n",
			kept:  []string{"\n<p>  a  </p>\n"},
		},
		{
			name:  "preserve keeps the next element",
			input: "<div>\n{{/* muxt:fmt preserve */}}\n  <ul>\n     <li>a</li>   <li>b</li>\n</ul>\n<p>c</p>\n</div>\n",
			want:  "<div>\n\t{{/* muxt:fmt preserve */}}\n  <ul>\n     <li>a</li>   <li>b</li>\n</ul>\n\t<p>c</p>\n</div>\n",
			kept:  []string{"\n  <ul>\n     <li>a</li>   <li>b</li>\n</ul>"},
		},
		{
			name:  "preserve skips comments to the next node",
			input: "<div>\n{{/* muxt:fmt preserve */}}\n{{/* note */}}<!-- html -->\n<p>  a  </p>\n<p>b</p>\n</div>\n",
			want:  "<div>\n\t{{/* muxt:fmt preserve */}}\n{{/* note */}}<!-- html -->\n<p>  a  </p>\n\t<p>b</p>\n</div>\n",
			kept:  []string{"\n{{/* note */}}<!-- html -->\n<p>  a  </p>"},
		},
		{
			name:  "preserve keeps an if through its end",
			input: "<div>\n{{/* muxt:fmt preserve */}}\n{{if   .X}}\n <p>  a </p>\n{{else}}<p>b</p>{{end}}\n<p>c</p>\n</div>\n",
			want:  "<div>\n\t{{/* muxt:fmt preserve */}}\n{{if   .X}}\n <p>  a </p>\n{{else}}<p>b</p>{{end}}\n\t<p>c</p>\n</div>\n",
			kept:  []string{"\n{{if   .X}}\n <p>  a </p>\n{{else}}<p>b</p>{{end}}"},
		},
		{
			name:  "preserve keeps an unclosed element and its content",
			input: "<ul>\n{{/* muxt:fmt preserve */}}\n  <li>a   b\n  <li>c\n</ul>\n",
			want:  "<ul>\n\t{{/* muxt:fmt preserve */}}\n  <li>a   b\n  <li>c\n</ul>\n",
			kept:  []string{"\n  <li>a   b\n  <li>c\n</ul>"},
		},
		{
			name:  "trim-marked directives are recognized",
			input: "<div>\n{{- /* muxt:fmt off */ -}}\n<p>  a  </p>\n{{- /* muxt:fmt on */ -}}\n<p>c</p>\n</div>\n",
			want:  "<div>\n\t{{- /* muxt:fmt off */ -}}\n<p>  a  </p>\n{{- /* muxt:fmt on */ -}}\n\t<p>c</p>\n</div>\n",
			kept:  []string{"\n<p>  a  </p>\n"},
		},
		{
			name:  "inside a branch body",
			input: "<div>\n{{if .X}}\n{{/* muxt:fmt off */}}\n<p>  a  </p>\n{{/* muxt:fmt on */}}\n<p>b</p>\n{{end}}\n</div>\n",
			want:  "<div>\n\t{{if .X}}\n\t\t{{/* muxt:fmt off */}}\n<p>  a  </p>\n{{/* muxt:fmt on */}}\n\t\t<p>b</p>\n\t{{end}}\n</div>\n",
			kept:  []string{"\n<p>  a  </p>\n"},
		},
		{
			name:  "inside a define body",
			input: "{{define \"x\"}}\n{{/* muxt:fmt off */}}\n<p>  a  </p>\n{{end}}\n",
			want:  "{{define \"x\"}}\n\t{{/* muxt:fmt off */}}\n<p>  a  </p>\n{{end}}\n",
			kept:  []string{"\n<p>  a  </p>\n{{end}}"},
		},
		{
			name:  "off in a define body with no gap at its edges",
			input: "{{define \"x\"}}<p>a</p>{{/* muxt:fmt off */}} <b>a  b</b> {{end}}\n",
			want:  "{{define \"x\"}}\n\t<p>a</p>\n\t{{/* muxt:fmt off */}} <b>a  b</b> {{end}}\n",
		},
		{
			name:  "off after content at the root runs to the end of the file",
			input: "<div>\n<p>a</p>\n</div>\n{{/* muxt:fmt off */}}\n<p>  b </p>\n  <p>c</p>",
			want:  "<div>\n\t<p>a</p>\n</div>\n{{/* muxt:fmt off */}}\n<p>  b </p>\n  <p>c</p>",
		},
		{
			name:  "directive in an inline run",
			input: "<p>a {{/* muxt:fmt off */}}  b   c {{/* muxt:fmt on */}} d</p>\n",
		},
		{
			name:  "preserve keeps text",
			input: "<p>{{/* muxt:fmt preserve */}}  a    b   </p>\n",
		},
		{
			name:  "ordinary comments are not directives",
			input: "<div>\n    {{/* just a comment */}}\n<!-- muxt:fmt off -->\n  <p>  a  </p>\n</div>\n",
			want:  "<div>\n\t{{/* just a comment */}}\n\t<!-- muxt:fmt off -->\n\t<p>a</p>\n</div>\n",
		},
		{
			name:    "misspelled verb",
			input:   "<div>\n{{/* muxt:fmt of */}}\n</div>\n",
			wantErr: `t.gohtml:2:1: unknown directive "muxt:fmt of": want muxt:fmt off, muxt:fmt on, or muxt:fmt preserve`,
		},
		{
			name:    "extra words",
			input:   "<div>\n  {{/* muxt:fmt off please */}}\n</div>\n",
			wantErr: `t.gohtml:2:3: unknown directive "muxt:fmt off please": want muxt:fmt off, muxt:fmt on, or muxt:fmt preserve`,
		},
		{
			name:    "no verb",
			input:   "<div>{{- /* muxt:fmt */}}</div>\n",
			wantErr: `t.gohtml:1:6: unknown directive "muxt:fmt": want muxt:fmt off, muxt:fmt on, or muxt:fmt preserve`,
		},
		{
			name:    "on without off",
			input:   "<div>\n{{/* muxt:fmt on */}}\n</div>\n",
			wantErr: "t.gohtml:2:1: muxt:fmt on without muxt:fmt off before it in the same element",
		},
		{
			name:    "off and on straddle parents",
			input:   "<div>{{/* muxt:fmt off */}}</div>{{/* muxt:fmt on */}}\n",
			wantErr: "t.gohtml:1:34: muxt:fmt on without muxt:fmt off before it in the same element",
		},
		{
			name:    "preserve with nothing after it",
			input:   "<p>{{/* muxt:fmt preserve */}}</p>\n",
			wantErr: "t.gohtml:1:4: muxt:fmt preserve has no node after it",
		},
		{
			name:    "preserve followed only by whitespace and comments",
			input:   "<p>x{{/* muxt:fmt preserve */}}  {{/* c */}}\n</p>\n",
			wantErr: "t.gohtml:1:5: muxt:fmt preserve has no node after it",
		},
		{
			name:    "error in a file with no elements",
			input:   "{{/* muxt:fmt of */}}\n",
			wantErr: `t.gohtml:1:1: unknown directive "muxt:fmt of": want muxt:fmt off, muxt:fmt on, or muxt:fmt preserve`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Source("t.gohtml", tt.input)
			if tt.wantErr != "" {
				var de *DirectiveError
				if !errors.As(err, &de) {
					t.Fatalf("Source error = %v, want a *DirectiveError", err)
				}
				if de.Error() != tt.wantErr {
					t.Errorf("Source error = %q, want %q", de.Error(), tt.wantErr)
				}
				if got != "" {
					t.Errorf("Source returned %q with an error, want nothing", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Source error = %v", err)
			}
			want := tt.want
			if want == "" {
				want = tt.input
			}
			if got != want {
				t.Errorf("Source(%q) =\n%q\nwant\n%q", tt.input, got, want)
			}
			for _, k := range tt.kept {
				if !strings.Contains(got, k) {
					t.Errorf("output lost the region %q:\n%q", k, got)
				}
			}
			again, err := Source("t.gohtml", got)
			if err != nil {
				t.Fatalf("Source of the output error = %v", err)
			}
			if again != got {
				t.Errorf("not idempotent:\nfirst  %q\nsecond %q", got, again)
			}
		})
	}
}
