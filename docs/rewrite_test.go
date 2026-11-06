package docs_test

import (
	"testing"

	"github.com/typelate/muxt/docs"
)

func TestRewriteLinks(t *testing.T) {
	for _, tt := range []struct {
		page string
		ref  string
		in   string
		want string
	}{
		{
			page: "docs/reference/commands/check.md",
			ref:  "main",
			in:   "[Type Checking](../type-checking.md#preconditions)",
			want: "[Type Checking](docs/reference/type-checking.md#preconditions)",
		},
		{
			page: "docs/how-to/x.md",
			ref:  "main",
			in:   "[cli](../reference/cli.md)",
			want: "[cli](docs/reference/cli.md)",
		},
		{
			page: "docs/README.md",
			ref:  "main",
			in:   "[Decisions](explanation/decisions/)",
			want: "[Decisions](docs/explanation/decisions/)",
		},
		{
			page: "docs/examples/simple/README.md",
			ref:  "main",
			in:   "[main.go](main.go)",
			want: "[main.go](docs/examples/simple/main.go)",
		},
		{
			page: "docs/reference/cli.md",
			ref:  "main",
			in:   "[a](#flags)",
			want: "[a](#flags)",
		},
		{
			page: "docs/reference/cli.md",
			ref:  "main",
			in:   "[b](https://go.dev/doc)",
			want: "[b](https://go.dev/doc)",
		},
		{
			page: "docs/reference/cli.md",
			ref:  "main",
			in:   "[c](mailto:x@example.com)",
			want: "[c](mailto:x@example.com)",
		},
		{
			page: "docs/how-to/x.md",
			ref:  "main",
			in:   "[a](../reference/a.md) and [b](../reference/b.md)",
			want: "[a](docs/reference/a.md) and [b](docs/reference/b.md)",
		},
		{
			page: "docs/reference/cli.md",
			ref:  "main",
			in:   "```\n[x](./y.md)\n```\n[x](./y.md)\n",
			want: "```\n[x](./y.md)\n```\n[x](docs/reference/y.md)\n",
		},
		{
			page: "docs/reference/cli.md",
			ref:  "main",
			in:   "![alt](img/a.png)",
			want: "![alt](docs/reference/img/a.png)",
		},
		{
			page: "docs/reference/commands/check.md",
			ref:  "v1.2.3",
			in:   "[t](../../../cmd/muxt/testdata/x.txt)",
			want: "[t](https://github.com/typelate/muxt/blob/v1.2.3/cmd/muxt/testdata/x.txt)",
		},
		{
			page: "docs/reference/commands/check.md",
			ref:  "main",
			in:   "[t](../../../cmd/muxt/testdata/x.txt)",
			want: "[t](https://github.com/typelate/muxt/blob/main/cmd/muxt/testdata/x.txt)",
		},
		{
			page: "docs/reference/commands/check.md",
			ref:  "main",
			in:   "[t](../../../cmd/muxt/testdata/x.txt#L3)",
			want: "[t](https://github.com/typelate/muxt/blob/main/cmd/muxt/testdata/x.txt#L3)",
		},
		{
			page: "docs/README.md",
			ref:  "main",
			in:   "[r](../README.md)",
			want: "[r](https://github.com/typelate/muxt/blob/main/README.md)",
		},
	} {
		got := string(docs.RewriteLinks(tt.page, []byte(tt.in), tt.ref))
		if got != tt.want {
			t.Errorf("RewriteLinks(%q, %q, %q) = %q, want %q", tt.page, tt.in, tt.ref, got, tt.want)
		}
	}
}
