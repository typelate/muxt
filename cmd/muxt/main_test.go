package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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
		cmd.Dir = filepath.FromSlash("../../docs/examples/simple")
		cmd.Stderr = os.Stdout
		cmd.Stdout = os.Stdout
		require.NoError(t, cmd.Run())
	})
	t.Run("check example", func(t *testing.T) {
		ctx := t.Context()
		buf := bytes.NewBuffer(nil)
		cmd := exec.CommandContext(ctx, "go", "run", mainPackage, "-C", filepath.FromSlash("../../docs/examples/htmx"), "check")
		cmd.Dir = "."
		cmd.Stderr = buf
		cmd.Stdout = buf
		require.NoError(t, cmd.Run(), buf.String())
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
	e.Cmds["count-matches"] = countRedirectBlocksCommand()
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

func countRedirectBlocksCommand() script.Cmd {
	return script.Command(script.CmdUsage{
		Summary: "count-matches <pattern> <filename> <expected-count>",
		Args:    "pattern filename expected-count",
	}, func(state *script.State, args ...string) (script.WaitFunc, error) {
		if len(args) != 3 {
			return nil, script.ErrUsage
		}
		pattern := args[0]
		filename := args[1]
		expectedCount := args[2]

		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}

		return func(state *script.State) (string, string, error) {
			filePath := filepath.Join(state.Getwd(), filename)
			content, err := os.ReadFile(filePath)
			if err != nil {
				return "", err.Error(), err
			}
			matches := re.FindAll(content, -1)
			count := len(matches)
			expectedNum, err := strconv.Atoi(expectedCount)
			if err != nil {
				return "", "expected-count must be a number", script.ErrUsage
			}
			if count != expectedNum {
				return "", fmt.Sprintf("expected %d redirect blocks, found %d", expectedNum, count), script.ErrUsage
			}
			return fmt.Sprintf("found %d redirect blocks in %s", count, filename), "", nil
		}, nil
	})
}
