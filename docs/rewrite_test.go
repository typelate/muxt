package docs_test

import (
	"testing"

	"github.com/typelate/muxt/docs"
)

func TestRewriteLinks(t *testing.T) {
	for _, tt := range []struct {
		page string
		in   string
		want string
	}{
		{
			page: "docs/reference/commands/check.md",
			in:   "[Type Checking](../type-checking.md#preconditions)",
			want: "[Type Checking](docs/reference/type-checking.md#preconditions)",
		},
		{
			page: "docs/how-to/x.md",
			in:   "[cli](../reference/cli.md)",
			want: "[cli](docs/reference/cli.md)",
		},
		{
			page: "docs/README.md",
			in:   "[Decisions](explanation/decisions/)",
			want: "[Decisions](docs/explanation/decisions/)",
		},
		{
			page: "docs/examples/simple/README.md",
			in:   "[main.go](main.go)",
			want: "[main.go](docs/examples/simple/main.go)",
		},
		{
			page: "docs/reference/cli.md",
			in:   "[a](#flags)",
			want: "[a](#flags)",
		},
		{
			page: "docs/reference/cli.md",
			in:   "[b](https://go.dev/doc)",
			want: "[b](https://go.dev/doc)",
		},
		{
			page: "docs/reference/cli.md",
			in:   "[c](mailto:x@example.com)",
			want: "[c](mailto:x@example.com)",
		},
		{
			page: "docs/how-to/x.md",
			in:   "[a](../reference/a.md) and [b](../reference/b.md)",
			want: "[a](docs/reference/a.md) and [b](docs/reference/b.md)",
		},
		{
			page: "docs/reference/cli.md",
			in:   "```\n[x](./y.md)\n```\n[x](./y.md)\n",
			want: "```\n[x](./y.md)\n```\n[x](docs/reference/y.md)\n",
		},
		{
			page: "docs/reference/cli.md",
			in:   "![alt](img/a.png)",
			want: "![alt](docs/reference/img/a.png)",
		},
		{
			page: "docs/reference/commands/check.md",
			in:   "[t](../../../cmd/muxt/testdata/x.txt)",
			want: "[t](../../../cmd/muxt/testdata/x.txt)",
		},
	} {
		got := string(docs.RewriteLinks(tt.page, []byte(tt.in), "main"))
		if got != tt.want {
			t.Errorf("RewriteLinks(%q, %q) = %q, want %q", tt.page, tt.in, got, tt.want)
		}
	}
}
