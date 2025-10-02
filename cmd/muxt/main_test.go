package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"rsc.io/script"
	"rsc.io/script/scripttest"

	"github.com/typelate/muxt/internal/cli"
)

//go:generate cp main.go ../../
//go:generate go run github.com/crhntr/txtarfmt/cmd/txtarfmt -ext=.txt testdata/*

func TestDocumentation(t *testing.T) {
	const mainPackage = "github.com/typelate/muxt/cmd/muxt"

	t.Run("generate example", func(t *testing.T) {
		ctx := t.Context()
		cmd := exec.CommandContext(ctx, "go", "generate", "./...")
		cmd.Dir = filepath.FromSlash("../../docs/example")
		cmd.Stderr = os.Stdout
		cmd.Stdout = os.Stdout
		require.NoError(t, cmd.Run())
	})
	t.Run("check example", func(t *testing.T) {
		ctx := t.Context()
		cmd := exec.CommandContext(ctx, "go", "run", mainPackage, "-C", filepath.FromSlash("../../docs/example/hypertext"), "check", "--receiver-type", "Backend")
		cmd.Dir = "."
		cmd.Stderr = os.Stdout
		cmd.Stdout = os.Stdout
		require.NoError(t, cmd.Run())
	})
}

func TestEntrypoint(t *testing.T) {
	cmdMainGo, err := os.ReadFile("main.go")
	require.NoError(t, err)
	mainGo, err := os.ReadFile(filepath.FromSlash("../../main.go"))
	require.NoError(t, err)
	require.Equal(t, string(cmdMainGo), string(mainGo))
}

func Test(t *testing.T) {
	e := script.NewEngine()
	e.Quiet = true
	e.Cmds = scripttest.DefaultCmds()
	e.Cmds["muxt"] = scriptCommand()
	ctx := t.Context()
	scripttest.Test(t, ctx, e, nil, filepath.FromSlash("testdata/*.txt"))
}

func scriptCommand() script.Cmd {
	return script.Command(script.CmdUsage{
		Summary: "muxt",
		Args:    "",
	}, func(state *script.State, args ...string) (script.WaitFunc, error) {
		return func(state *script.State) (string, string, error) {
			var stdout, stderr bytes.Buffer
			err := cli.Commands(state.Getwd(), append([]string{"muxt"}, args...), func(s string) string {
				e, _ := state.LookupEnv(s)
				return e
			}, &stdout, &stderr)
			if err != nil {
				stderr.WriteString(err.Error())
			}
			return stdout.String(), stderr.String(), err
		}, nil
	})
}
