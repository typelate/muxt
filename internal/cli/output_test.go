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
			if buf.String() != tt.want {
				t.Errorf("warnPartialAST() wrote %q, want %q", buf.String(), tt.want)
			}
		})
	}
}

func TestPrintMultiLineError(t *testing.T) {
	t.Run("a multi-line error", func(t *testing.T) {
		var stderr bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetErr(&stderr)
		if !printMultiLineError(cmd, fmt.Errorf("wrapped: %w", multiLineError{})) {
			t.Fatal("printMultiLineError() = false, want true")
		}
		if want := "Error:\nline one\nline two\n"; stderr.String() != want {
			t.Errorf("stderr = %q, want %q", stderr.String(), want)
		}
		if !cmd.SilenceErrors {
			t.Error("SilenceErrors = false, want cobra's inline error silenced")
		}
	})
	t.Run("any other error", func(t *testing.T) {
		var stderr bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetErr(&stderr)
		if printMultiLineError(cmd, errors.New("plain")) {
			t.Fatal("printMultiLineError() = true, want false")
		}
		if stderr.Len() != 0 || cmd.SilenceErrors {
			t.Errorf("stderr = %q, SilenceErrors = %v, want untouched", stderr.String(), cmd.SilenceErrors)
		}
	})
}

func TestCheckFailure(t *testing.T) {
	t.Run("a multi-line error is returned as it is", func(t *testing.T) {
		var stderr bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetErr(&stderr)
		in := multiLineError{}
		if got := checkFailure(cmd, in); got != error(in) {
			t.Errorf("checkFailure() = %v, want the error it was given", got)
		}
		if want := "Error:\nline one\nline two\n"; stderr.String() != want {
			t.Errorf("stderr = %q, want %q", stderr.String(), want)
		}
	})
	t.Run("any other error is a fail line", func(t *testing.T) {
		var stderr bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetErr(&stderr)
		got := checkFailure(cmd, errors.New("no such variable"))
		if want := "fail: no such variable"; got == nil || got.Error() != want {
			t.Errorf("checkFailure() = %v, want %q", got, want)
		}
		if stderr.Len() != 0 {
			t.Errorf("stderr = %q, want nothing", stderr.String())
		}
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
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("writeResult() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || buf.String() != tt.want {
				t.Fatalf("writeResult() = %q, %v, want %q", buf.String(), err, tt.want)
			}
		})
	}
}

func TestVersionCommand(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"v"}, {"version", "--verbose"}} {
		var stdout bytes.Buffer
		err := Commands(t.TempDir(), args, func(string) string { return "" }, &stdout, io.Discard)
		v, ok := cliVersion()
		switch {
		case !ok && (err == nil || err.Error() != "missing CLI version"):
			t.Errorf("muxt %v = %v, want the missing CLI version error", args, err)
		case ok && !strings.HasPrefix(stdout.String(), v+"\n"):
			t.Errorf("muxt %v printed %q, want it to start with %q", args, stdout.String(), v+"\n")
		}
	}
}

func TestCommandsAreWired(t *testing.T) {
	for _, name := range []string{
		generateCommandName, versionCommandName, checkCommandName,
		listTemplateCallersCommandName, listTemplateCallsCommandName,
		exploreModuleCommandName, generateFakeServerCommandName, testTemplateMutationsName,
	} {
		var stdout bytes.Buffer
		if err := Commands(t.TempDir(), []string{name, "--help"}, func(string) string { return "" }, &stdout, io.Discard); err != nil {
			t.Errorf("muxt %s --help error = %v", name, err)
		}
		if !strings.Contains(stdout.String(), "muxt "+name) {
			t.Errorf("muxt %s --help = %q, want its usage", name, stdout.String())
		}
	}
	for alias, name := range map[string]string{
		"g": generateCommandName, "gen": generateCommandName, "v": versionCommandName, "c": checkCommandName,
		"callers": listTemplateCallersCommandName, "calls": listTemplateCallsCommandName, "explore": exploreModuleCommandName,
	} {
		var stdout bytes.Buffer
		if err := Commands(t.TempDir(), []string{alias, "--help"}, func(string) string { return "" }, &stdout, io.Discard); err != nil {
			t.Errorf("muxt %s --help error = %v", alias, err)
		}
		if !strings.Contains(stdout.String(), "muxt "+name) {
			t.Errorf("muxt %s --help = %q, want the usage of %s", alias, stdout.String(), name)
		}
	}
}
