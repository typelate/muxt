package generate_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/generate"
)

func TestNamespace(t *testing.T) {
	for _, tc := range []struct {
		importPath    string
		hubImportPath string
		want          string
		wantErr       bool
	}{
		{
			importPath:    "example.com/m/internal/hypertext/blog",
			hubImportPath: "example.com/m/internal/hypertext",
			want:          "Blog",
		},
		{
			importPath:    "example.com/m/internal/hypertext/admin_v2",
			hubImportPath: "example.com/m/internal/hypertext",
			want:          "AdminV2",
		},
		{
			importPath:    "example.com/m/internal/hypertext/admin/users",
			hubImportPath: "example.com/m/internal/hypertext",
			want:          "AdminUsers",
		},
		{
			importPath:    "example.com/m/internal/hypertext/api",
			hubImportPath: "example.com/m/internal/hypertext",
			want:          "API",
		},
		{
			importPath:    "example.com/m/blog",
			hubImportPath: "example.com/m",
			want:          "Blog",
		},
		{
			importPath:    "example.com/other/blog",
			hubImportPath: "example.com/m/internal/hypertext",
			wantErr:       true,
		},
		{
			importPath:    "example.com/m/internal/hypertext/api/2024",
			hubImportPath: "example.com/m/internal/hypertext",
			want:          "API2024",
		},
	} {
		got, err := generate.Namespace(tc.importPath, tc.hubImportPath)
		if tc.wantErr {
			assert.Error(t, err, "Namespace(%q, %q) should fail", tc.importPath, tc.hubImportPath)
			continue
		}
		require.NoError(t, err, "Namespace(%q, %q)", tc.importPath, tc.hubImportPath)
		assert.Equal(t, tc.want, got, "Namespace(%q, %q) = %q, want %q", tc.importPath, tc.hubImportPath, got, tc.want)
	}
}

func TestNamespaceIsNotExportedIdentifier(t *testing.T) {
	const hub = "example.com/m"
	for _, tc := range []struct {
		importPath string
		namespace  string
	}{
		{importPath: "example.com/m/_1", namespace: "1"},
		{importPath: "example.com/m/123", namespace: "123"},
	} {
		_, err := generate.Namespace(tc.importPath, hub)
		var nsErr *generate.InvalidNamespaceError
		require.ErrorAs(t, err, &nsErr, "Namespace(%q, %q)", tc.importPath, hub)
		assert.Equal(t, tc.importPath, nsErr.ImportPath)
		assert.Equal(t, tc.namespace, nsErr.Namespace)
		assert.Equal(t,
			fmt.Sprintf(`package %q gets the route paths namespace %q, which is not an exported Go identifier; rename its directory`, tc.importPath, tc.namespace),
			err.Error())
	}
}

func TestCheckNamespaces(t *testing.T) {
	const hub = "example.com/m"
	clashing := []string{"example.com/m/admin_users", "example.com/m/admin/users"}
	for _, tc := range []struct {
		name        string
		importPaths []string
		wantErr     bool
	}{
		{
			name:        "no clash",
			importPaths: []string{"example.com/m/blog", "example.com/m/admin/users"},
		},
		{
			name:        "clash in given order",
			importPaths: clashing,
			wantErr:     true,
		},
		{
			name:        "clash in reverse order",
			importPaths: []string{clashing[1], clashing[0]},
			wantErr:     true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := generate.CheckNamespaces(tc.importPaths, hub)
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			var sameErr *generate.SameNamespaceError
			require.ErrorAs(t, err, &sameErr)
			assert.Equal(t, "AdminUsers", sameErr.Namespace)
			assert.Equal(t, [2]string{"example.com/m/admin/users", "example.com/m/admin_users"}, sameErr.ImportPaths)
			assert.Equal(t,
				`packages "example.com/m/admin/users" and "example.com/m/admin_users" both get the route paths namespace "AdminUsers"; rename one of them`,
				err.Error())
		})
	}
}
