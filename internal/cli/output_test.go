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
	"golang.org/x/tools/go/packages"
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
	parseError := packages.Error{Kind: packages.ParseError, Pos: "a.go:1:1", Msg: "expected 'package'"}
	typeError := packages.Error{Kind: packages.TypeError, Pos: "a.go:2:1", Msg: "undefined: X"}
	const warning = "warning: package has syntax errors, so these checks ran against a partial AST; run go build for the full picture\n"

	for _, tt := range []struct {
		name      string
		pl        []*packages.Package
		nilLogger bool
		want      string
	}{
		{name: "clean", pl: []*packages.Package{{}}},
		{name: "type errors are expected before generate", pl: []*packages.Package{{Errors: []packages.Error{typeError}}}},
		{name: "syntax errors", pl: []*packages.Package{{Errors: []packages.Error{parseError}}}, want: warning},
		{name: "no logger", pl: []*packages.Package{{Errors: []packages.Error{parseError}}}, nilLogger: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := log.New(&buf, "", 0)
			if tt.nilLogger {
				logger = nil
			}
			warnPartialAST(logger, tt.pl)
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
	for _, args := range [][]string{{"version"}, {"v"}, {"version", "--verbose"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout bytes.Buffer
			err := Commands(t.TempDir(), args, func(string) string { return "" }, &stdout, io.Discard)
			v, ok := cliVersion()
			if !ok {
				assert.EqualError(t, err, "missing CLI version", "muxt %v, want the missing CLI version error", args)
				return
			}
			assert.True(t, strings.HasPrefix(stdout.String(), v+"\n"), "muxt %v printed %q, want it to start with %q", args, stdout.String(), v+"\n")
		})
	}
}

func TestCommandsAreWired(t *testing.T) {
	help := func(t *testing.T, arg string) string {
		t.Helper()
		var stdout bytes.Buffer
		err := Commands(t.TempDir(), []string{arg, "--help"}, func(string) string { return "" }, &stdout, io.Discard)
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
