package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"rsc.io/script"
	"rsc.io/script/scripttest"
)

func commandTest(t *testing.T, pattern string) {
	e := script.NewEngine()
	e.Quiet = true
	e.Cmds = scripttest.DefaultCmds()
	e.Cmds["muxt"] = scriptCommand()
	ctx := context.Background()
	scripttest.Test(t, ctx, e, nil, pattern)
}

func Test_example(t *testing.T) {
	require.NoError(t, os.Remove(filepath.FromSlash("../../example/template_routes.go")))

	ctx := context.TODO()
	t.Run("generate", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, "go", "generate")
		cmd.Dir = filepath.FromSlash("../../example")
		cmd.Stderr = os.Stdout
		cmd.Stdout = os.Stdout
		require.NoError(t, cmd.Run())
	})
	t.Run("check", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, "go", "run", ".", "-C", filepath.FromSlash("../../example"), "check", "--receiver-type", "Backend")
		cmd.Dir = "."
		cmd.Stderr = os.Stdout
		cmd.Stdout = os.Stdout
		require.NoError(t, cmd.Run())
	})
}
