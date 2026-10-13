package generate

import (
	"go/ast"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/typelate/muxt/internal/astgen"
)

// TestPrefixedPath states how a pattern path is spelled under a path
// prefix. path.Join drops a trailing slash, which in a pattern means the
// subtree, so any path that ends in one gets it back, not only "/".
func TestPrefixedPath(t *testing.T) {
	for _, tt := range []struct {
		name string
		path string
		want string
	}{
		{
			name: "a path without a trailing slash is joined",
			path: "/users",
			want: `path.Join(pathsPrefix, "/users")`,
		},
		{
			name: "the root keeps its trailing slash",
			path: "/",
			want: `strings.TrimSuffix(path.Join(pathsPrefix, "/"), "/") + "/"`,
		},
		{
			name: "a subtree keeps its trailing slash",
			path: "/static/",
			want: `strings.TrimSuffix(path.Join(pathsPrefix, "/static/"), "/") + "/"`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			file := outputFile()
			got := concatenation(prefixedPath(file, ast.NewIdent("pathsPrefix"), tt.path)...)
			assert.Equal(t, tt.want, astgen.Format(got), "prefixedPath(%q)", tt.path)
		})
	}
}
