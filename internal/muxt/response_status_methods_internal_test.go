package muxt

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"testing"
)

func TestResponseStateMethods(t *testing.T) {
	for _, tt := range []struct {
		name string
		src  string
		want map[string]string
	}{
		{
			name: "calls StatusCode",
			src: `package p
func (data *TemplateData[R, T]) ErrorStatusCode() string { data.StatusCode(404); return "" }`,
			want: map[string]string{"ErrorStatusCode": "StatusCode"},
		},
		{
			name: "calls another method that redirects",
			src: `package p
func (data *TemplateData[R, T]) Outer() string { return data.Inner() }
func (data *TemplateData[R, T]) Inner() string { data.Redirect("/x", 302); return "" }`,
			want: map[string]string{"Outer": "Redirect", "Inner": "Redirect"},
		},
		{
			name: "chained on Header",
			src: `package p
func (d *TemplateData[R, T]) Moved() string { d.Header("A", "b").RedirectFound("/x"); return "" }`,
			want: map[string]string{"Moved": "RedirectFound"},
		},
		{
			name: "only Header",
			src: `package p
func (data *TemplateData[R, T]) Plain() string { data.Header("X", "y"); return "" }`,
			want: map[string]string{},
		},
		{
			name: "mutual recursion that reaches state",
			src: `package p
func (data *TemplateData[R, T]) A() string { data.B(); return "" }
func (data *TemplateData[R, T]) B() string { data.A(); data.StatusCode(500); return "" }`,
			want: map[string]string{"A": "StatusCode", "B": "StatusCode"},
		},
		{
			name: "mutual recursion that reaches nothing",
			src: `package p
func (data *TemplateData[R, T]) A() string { data.B(); return "" }
func (data *TemplateData[R, T]) B() string { data.A(); return "" }`,
			want: map[string]string{},
		},
		{
			name: "other receiver types are ignored",
			src: `package p
func (data *Other) Status() string { data.StatusCode(1); return "" }
func (data *TemplateData[R, T]) Wrap() string { return "" }`,
			want: map[string]string{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "p.go", tt.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			got := responseStateMethods([]*ast.File{file}, "TemplateData")
			if !maps.Equal(got, tt.want) {
				t.Errorf("responseStateMethods() = %v, want %v", got, tt.want)
			}
		})
	}
}
