package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourceRef(t *testing.T) {
	for _, tt := range []struct {
		version string
		want    string
	}{
		{version: "v1.2.3", want: "v1.2.3"},
		{version: "v0.22.0-dev.4", want: "v0.22.0-dev.4"},
		{version: "v0.22.1-0.20261105181332-775c5f71f012", want: "775c5f71f012"},
		{version: "v0.22.0-dev.4.0.20261105181332-775c5f71f012", want: "775c5f71f012"},
		{version: "v1.2.3+dirty", want: "main"},
		{version: "(devel)", want: "main"},
		{version: "", want: "main"},
	} {
		got := sourceRef(tt.version)
		if got != tt.want {
			t.Errorf("sourceRef(%q) = %q, want %q", tt.version, got, tt.want)
		}
	}
}

func TestDocumentationQueryPrintsMatchingLines(t *testing.T) {
	var stdout bytes.Buffer
	err := commands(t.TempDir(), []string{"documentation", "--query", "ExecuteTemplate"}, func(string) string { return "" }, func() (string, bool) { return "", false }, strings.NewReader(""), &stdout, io.Discard, runners{})
	require.NoError(t, err, "muxt documentation --query ExecuteTemplate")
	assert.Regexp(t, `(?m)^docs/[^:]+:\d+: .*ExecuteTemplate`, stdout.String(), "muxt documentation --query ExecuteTemplate")
}

func TestDocumentationLinksLeavingDocsNameTheBuildVersion(t *testing.T) {
	for _, tt := range []struct {
		name    string
		version func() (string, bool)
		want    string
	}{
		{
			name:    "release",
			version: func() (string, bool) { return "v1.2.3", true },
			want:    "https://github.com/typelate/muxt/blob/v1.2.3/cmd/muxt/testdata/err_check_before_generate.txt",
		},
		{
			name:    "no version in the build",
			version: func() (string, bool) { return "", false },
			want:    "https://github.com/typelate/muxt/blob/main/cmd/muxt/testdata/err_check_before_generate.txt",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			err := commands(t.TempDir(), []string{"documentation", "docs/reference/commands/check.md"}, func(string) string { return "" }, tt.version, strings.NewReader(""), &stdout, io.Discard, runners{})
			require.NoError(t, err, "muxt documentation docs/reference/commands/check.md")
			assert.Contains(t, stdout.String(), tt.want, "muxt documentation docs/reference/commands/check.md")
		})
	}
}
