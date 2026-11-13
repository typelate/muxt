package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type multiLineError struct{}

func (multiLineError) Error() string          { return "short form" }
func (multiLineError) MultiLineError() string { return "line one\nline two" }

type textResult string

func (r textResult) WriteTo(w io.Writer) (int64, error) {
	n, err := io.WriteString(w, string(r))
	return int64(n), err
}

func TestWarnPartialAST(t *testing.T) {
	const warning = "warning: package has syntax errors, so these checks ran against a partial AST; run go build for the full picture\n"

	for _, tt := range []struct {
		name           string
		hasParseErrors bool
		nilLogger      bool
		want           string
	}{
		{name: "clean"},
		{name: "syntax errors", hasParseErrors: true, want: warning},
		{name: "no logger", hasParseErrors: true, nilLogger: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := log.New(&buf, "", 0)
			if tt.nilLogger {
				logger = nil
			}
			warnPartialAST(logger, tt.hasParseErrors)
			assert.Equal(t, tt.want, buf.String(), "warnPartialAST() wrote")
		})
	}
}

func TestPrintMultiLineError(t *testing.T) {
	t.Run("a multi-line error", func(t *testing.T) {
		var stderr bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetErr(&stderr)
		require.True(t, printMultiLineError(cmd, fmt.Errorf("wrapped: %w", multiLineError{})), "printMultiLineError()")
		assert.Equal(t, "Error:\nline one\nline two\n", stderr.String(), "stderr")
		assert.True(t, cmd.SilenceErrors, "SilenceErrors, want cobra's inline error silenced")
	})
	t.Run("any other error", func(t *testing.T) {
		var stderr bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetErr(&stderr)
		require.False(t, printMultiLineError(cmd, errors.New("plain")), "printMultiLineError()")
		assert.Empty(t, stderr.String(), "stderr, want untouched")
		assert.False(t, cmd.SilenceErrors, "SilenceErrors, want untouched")
	})
}

func TestCheckFailure(t *testing.T) {
	t.Run("a multi-line error is returned as it is", func(t *testing.T) {
		var stderr bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetErr(&stderr)
		in := multiLineError{}
		assert.Equal(t, error(in), checkFailure(cmd, in), "checkFailure() want the error it was given")
		assert.Equal(t, "Error:\nline one\nline two\n", stderr.String(), "stderr")
	})
	t.Run("any other error is a fail line", func(t *testing.T) {
		var stderr bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetErr(&stderr)
		got := checkFailure(cmd, errors.New("no such variable"))
		assert.EqualError(t, got, "fail: no such variable", "checkFailure()")
		assert.Empty(t, stderr.String(), "stderr, want nothing")
	})
}

func TestWriteResultFormats(t *testing.T) {
	newCmd := func(format string) *cobra.Command {
		cmd := &cobra.Command{}
		if format != "" {
			cmd.Flags().String("format", format, "")
		}
		return cmd
	}
	for _, tt := range []struct {
		name    string
		format  string
		want    string
		wantErr string
	}{
		{name: "text", format: "text", want: "hello"},
		{name: "json", format: "json", want: "\"hello\"\n"},
		{name: "unknown", format: "yaml", wantErr: "unknown format: yaml"},
		{name: "no format flag", wantErr: "flag accessed but not defined: format"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := writeResult(newCmd(tt.format), &buf, textResult("hello"))
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr, "writeResult()")
				return
			}
			require.NoError(t, err, "writeResult()")
			require.Equal(t, tt.want, buf.String(), "writeResult()")
		})
	}
}

func TestVersionCommand(t *testing.T) {
	run := func(t *testing.T, args []string, version func() (string, bool)) (string, error) {
		t.Helper()
		var stdout bytes.Buffer
		err := commands(t.TempDir(), args, func(string) string { return "" }, version, strings.NewReader(""), &stdout, io.Discard, runners{})
		return stdout.String(), err
	}
	for _, args := range [][]string{{"version"}, {"v"}, {"version", "--verbose"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, err := run(t, args, func() (string, bool) { return "v1.2.3", true })
			require.NoError(t, err, "muxt %v", args)
			assert.True(t, strings.HasPrefix(stdout, "v1.2.3\n"), "muxt %v printed %q, want it to start with %q", args, stdout, "v1.2.3\n")
		})
	}
	t.Run("no version in the build", func(t *testing.T) {
		_, err := run(t, []string{"version"}, func() (string, bool) { return "", false })
		assert.EqualError(t, err, "missing CLI version", "muxt version")
	})
}

func TestCommandsAreWired(t *testing.T) {
	help := func(t *testing.T, arg string) string {
		t.Helper()
		var stdout bytes.Buffer
		err := Commands(t.TempDir(), []string{arg, "--help"}, func(string) string { return "" }, strings.NewReader(""), &stdout, io.Discard)
		assert.NoError(t, err, "muxt %s --help", arg)
		return stdout.String()
	}

	for _, name := range []string{
		generateCommandName, versionCommandName, checkCommandName,
		listTemplateCallersCommandName, listTemplateCallsCommandName,
		exploreModuleCommandName, generateFakeServerCommandName, testTemplateMutationsName,
	} {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, help(t, name), "muxt "+name, "muxt %s --help want its usage", name)
		})
	}
	for alias, name := range map[string]string{
		"g": generateCommandName, "gen": generateCommandName, "v": versionCommandName, "c": checkCommandName,
		"callers": listTemplateCallersCommandName, "calls": listTemplateCallsCommandName, "explore": exploreModuleCommandName,
	} {
		t.Run("alias "+alias, func(t *testing.T) {
			assert.Contains(t, help(t, alias), "muxt "+name, "muxt %s --help want the usage of %s", alias, name)
		})
	}
}
