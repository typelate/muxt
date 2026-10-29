package main

import (
	"strings"
	"testing"
)

func TestTransform(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		isFork  bool
		want    []string
		wantNot []string
		wantErr bool
	}{
		{
			name:    "aliased root import",
			src:     "package x\nimport httpkg \"github.com/typelate/htt\"\nvar _ httpkg.Tree\n",
			want:    []string{`"github.com/typelate/muxt/internal/template"`, "template.Tree"},
			wantNot: []string{"httpkg"},
		},
		{
			name:   "fork with root import",
			src:    "package template\nimport (\n\t\"github.com/typelate/htt\"\n\t\"github.com/typelate/htt/template\"\n)\nvar _ htt.Tree\nvar _ template.Template\n",
			isFork: true,
			want:   []string{"package html", "template.Tree", "html.Template", `internal/template/html"`},
		},
		{
			name:    "root package name",
			src:     "package htt_test\n",
			want:    []string{"package template_test"},
			wantNot: []string{"htt"},
		},
		{
			name:    "dot import fails",
			src:     "package x\nimport . \"github.com/typelate/htt\"\n",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := transform("in.go", []byte(tt.src), tt.isFork)
			if (err != nil) != tt.wantErr {
				t.Fatalf("transform error = %v, wantErr %v", err, tt.wantErr)
			}
			for _, w := range tt.want {
				if !strings.Contains(string(out), w) {
					t.Errorf("output missing %q:\n%s", w, out)
				}
			}
			for _, w := range tt.wantNot {
				if strings.Contains(string(out), w) {
					t.Errorf("output contains %q:\n%s", w, out)
				}
			}
		})
	}
}
