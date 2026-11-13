package cli

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "rewrite the golden files under testdata")

// checkGolden compares got with the file at path, and writes got there when
// the test runs with -update.
func checkGolden(t *testing.T, path, got string) {
	t.Helper()
	if *update {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755), "creating the golden directory")
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644), "writing %s", path)
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "reading %s; rewrite it with -update", path)
	assert.Equal(t, string(want), got, "%s; rewrite it with -update and read the diff", path)
}

// commandTree returns the cobra tree of muxt, as a command's runner sees it.
func commandTree(t *testing.T) *cobra.Command {
	t.Helper()
	var root *cobra.Command
	err := commands(t.TempDir(), []string{"fmt"}, func(string) string { return "" }, cliVersion, strings.NewReader(""), io.Discard, io.Discard, runners{
		format: func(cmd *cobra.Command, _ string, _ formatConfiguration) error {
			root = cmd.Root()
			return nil
		},
	})
	require.NoError(t, err, "muxt fmt")
	require.NotNil(t, root, "root command")
	return root
}

// declaredFlags writes every flag a command declares itself, hidden ones
// included, with each field the user can see or a deprecation warning prints.
func declaredFlags(cmd *cobra.Command) string {
	var b strings.Builder
	cmd.NonInheritedFlags().VisitAll(func(f *pflag.Flag) {
		if f.Name == "help" {
			return
		}
		fmt.Fprintf(&b, "--%s\n", f.Name)
		fmt.Fprintf(&b, "  shorthand: %q\n", f.Shorthand)
		fmt.Fprintf(&b, "  type: %s\n", f.Value.Type())
		fmt.Fprintf(&b, "  default: %q\n", f.DefValue)
		fmt.Fprintf(&b, "  hidden: %t\n", f.Hidden)
		fmt.Fprintf(&b, "  deprecated: %q\n", f.Deprecated)
		fmt.Fprintf(&b, "  usage: %s\n", strconv.Quote(f.Usage))
	})
	return b.String()
}

// TestFlagDeclarations states every flag of every command, hidden ones
// included, and the --help text of each command, so a change to how flags are
// registered shows as a diff here when it changes what a user sees. Rewrite
// the files with -update.
func TestFlagDeclarations(t *testing.T) {
	root := commandTree(t)
	var visit func(cmd *cobra.Command, path []string)
	visit = func(cmd *cobra.Command, path []string) {
		name := strings.Join(path, " ")
		t.Run(name, func(t *testing.T) {
			var help bytes.Buffer
			args := append(append([]string{}, path[1:]...), "--help")
			err := commands(t.TempDir(), args, func(string) string { return "" }, cliVersion, strings.NewReader(""), &help, io.Discard, runners{})
			require.NoError(t, err, "muxt %v", args)
			got := name + "\n\nflags:\n" + declaredFlags(cmd) + "\n--help:\n" + help.String()
			checkGolden(t, filepath.Join("testdata", "flags", strings.Join(path, "_")+".txt"), got)
		})
		for _, sub := range cmd.Commands() {
			if sub.Name() == "completion" || sub.Name() == "help" {
				continue
			}
			visit(sub, append(append([]string{}, path...), sub.Name()))
		}
	}
	visit(root, []string{rootCommandName})
}
