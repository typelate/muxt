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

//go:generate go run github.com/crhntr/txtarfmt/cmd/txtarfmt -ext=.txt testdata/*

func Test_example(t *testing.T) {
	t.Run("generate", func(t *testing.T) {
		_ = os.Remove(filepath.FromSlash("../../example/hypertext/template_routes.go"))
		ctx := t.Context()
		cmd := exec.CommandContext(ctx, "go", "generate", "./...")
		cmd.Dir = filepath.FromSlash("../../example")
		cmd.Stderr = os.Stdout
		cmd.Stdout = os.Stdout
		require.NoError(t, cmd.Run())
	})
	t.Run("check", func(t *testing.T) {
		ctx := t.Context()
		cmd := exec.CommandContext(ctx, "go", "run", ".", "-C", filepath.FromSlash("../../example/hypertext"), "check", "--receiver-type", "Backend")
		cmd.Dir = "."
		cmd.Stderr = os.Stdout
		cmd.Stdout = os.Stdout
		require.NoError(t, cmd.Run())
	})
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
