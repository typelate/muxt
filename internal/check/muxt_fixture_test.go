package check_test

import (
	"bytes"
	"errors"
	"fmt"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/tools/go/packages"
	"rsc.io/script"
	"rsc.io/script/scripttest"
	"text/template/parse"

	upstream "github.com/typelate/check"

	"github.com/typelate/muxt/internal/check"
	"github.com/typelate/muxt/internal/cli"
)

// TestMuxtFixturesMatchUpstream runs every muxt check fixture script with
// its `muxt check` command wrapped: before the real command runs, the
// package is checked with both upstream github.com/typelate/check and this
// copy, and the sorted error lines are compared.
//
// Temporary: Task 14 of the parser/check cutover deletes it.
func TestMuxtFixturesMatchUpstream(t *testing.T) {
	t.Setenv("GOWORK", "off")
	fixtures, err := filepath.Glob(filepath.FromSlash("../../cmd/muxt/testdata/*_check_*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var selected []string
	for _, f := range fixtures {
		base := filepath.Base(f)
		if strings.HasPrefix(base, "err_check_") || strings.HasPrefix(base, "reference_check_") {
			selected = append(selected, f)
		}
	}
	if len(selected) == 0 {
		t.Fatal("no fixtures found")
	}
	for _, fixture := range selected {
		t.Run(strings.TrimSuffix(filepath.Base(fixture), ".txt"), func(t *testing.T) {
			e := script.NewEngine()
			e.Quiet = true
			e.Cmds = scripttest.DefaultCmds()
			e.Cmds["exec"] = script.Exec(func(cmd *exec.Cmd) error { return cmd.Process.Signal(os.Interrupt) }, 10*time.Second)
			e.Cmds["muxt"] = comparingMuxt(t, filepath.Base(fixture))
			e.Cmds["count-matches"] = script.Command(script.CmdUsage{Summary: "unused"}, func(*script.State, ...string) (script.WaitFunc, error) {
				return nil, script.ErrUsage
			})
			scripttest.Test(t, t.Context(), e, nil, fixture)
		})
	}
}

func comparingMuxt(t *testing.T, fixture string) script.Cmd {
	invocation := 0
	return script.Command(script.CmdUsage{Summary: "muxt", Args: ""}, func(state *script.State, args ...string) (script.WaitFunc, error) {
		return func(state *script.State) (string, string, error) {
			if len(args) > 0 && (args[0] == "check" || args[0] == "c") {
				invocation++
				compareCheckers(t, fixture, invocation, state)
			}
			var stdout, stderr bytes.Buffer
			err := cli.Commands(state.Getwd(), args, func(s string) string {
				v, _ := state.LookupEnv(s)
				return v
			}, &stdout, &stderr)
			return stdout.String(), stderr.String(), err
		}, nil
	})
}

func compareCheckers(t *testing.T, fixture string, invocation int, state *script.State) {
	t.Helper()
	dir := state.Getwd()
	fset := token.NewFileSet()
	pl, err := packages.Load(&packages.Config{
		Fset: fset,
		Mode: packages.NeedModule | packages.NeedTypesInfo | packages.NeedName | packages.NeedFiles | packages.NeedTypes | packages.NeedSyntax | packages.NeedEmbedPatterns | packages.NeedEmbedFiles | packages.NeedImports,
		Dir:  dir,
		Env:  state.Environ(),
	}, ".")
	if err != nil || len(pl) == 0 {
		t.Errorf("%s: check #%d: loading package: %v", fixture, invocation, err)
		return
	}
	pkg := pl[0]
	want := normalize(dir, upstreamErrors(pkg))
	got := normalize(dir, copyErrors(pkg))
	t.Logf("%s: check #%d: %d upstream lines, %d copy lines", fixture, invocation, len(want), len(got))
	if slices.Equal(want, got) {
		return
	}
	t.Errorf("%s: check #%d differs\n--- upstream (%d lines)\n%s\n--- copy (%d lines)\n%s", fixture, invocation,
		len(want), strings.Join(want, "\n"), len(got), strings.Join(got, "\n"))
}

func normalize(dir string, lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.ReplaceAll(l, dir, "$WORK")
	}
	slices.Sort(out)
	return out
}

// leafLines flattens err into one line per leaf: check errors contribute
// their non-aggregate leaves, anything else its own text.
func leafLines[E interface {
	error
	Unwrap() []error
}](err error, asCheck func(error) (E, bool), leaves func(E) []string) []string {
	if err == nil {
		return nil
	}
	if ce, ok := asCheck(err); ok {
		return leaves(ce)
	}
	return []string{err.Error()}
}

func upstreamErrors(pkg *packages.Package) []string {
	lt, err := upstream.LoadTemplates(pkg, "templates")
	if err != nil {
		return []string{"load: " + err.Error()}
	}
	var lines []string
	for c := range lt.ExecuteTemplateCalls() {
		global := upstream.NewGlobal(pkg.Types, pkg.Fset, upstream.FindTreeFunc(func(name string) (*parse.Tree, bool) {
			return lt.FindTree(name)
		}), upstream.Functions(lt.Functions()))
		tree, ok := lt.FindTree(c.TemplateName)
		if !ok {
			lines = append(lines, fmt.Sprintf("template %q not found", c.TemplateName))
			continue
		}
		if c.DataType == nil {
			lines = append(lines, "no data type")
			continue
		}
		lines = append(lines, leafLines(upstream.Execute(global, tree, c.DataType),
			func(err error) (*upstream.Error, bool) { return errors.AsType[*upstream.Error](err) },
			func(e *upstream.Error) []string {
				var out []string
				for leaf := range e.All {
					if leaf.Type == upstream.ErrorTypeAggregate {
						continue
					}
					out = append(out, leaf.Error())
				}
				return out
			})...)
	}
	return lines
}

func copyErrors(pkg *packages.Package) []string {
	lt, err := check.LoadTemplates(pkg, "templates")
	if err != nil {
		return []string{"load: " + err.Error()}
	}
	var lines []string
	for c := range lt.ExecuteTemplateCalls() {
		global := check.NewGlobal(pkg.Types, pkg.Fset, check.FindTreeFunc(lt.FindTree), check.Functions(lt.Functions()))
		tree, ok := lt.FindTree(c.TemplateName)
		if !ok {
			lines = append(lines, fmt.Sprintf("template %q not found", c.TemplateName))
			continue
		}
		if c.DataType == nil {
			lines = append(lines, "no data type")
			continue
		}
		lines = append(lines, leafLines(check.Execute(global, tree, c.DataType),
			func(err error) (*check.Error, bool) { return errors.AsType[*check.Error](err) },
			func(e *check.Error) []string {
				var out []string
				for leaf := range e.All {
					if leaf.Type == check.ErrorTypeAggregate {
						continue
					}
					out = append(out, leaf.Error())
				}
				return out
			})...)
	}
	return lines
}
