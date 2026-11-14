package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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
	examples, err := os.ReadDir(filepath.FromSlash("../../docs/examples"))
	require.NoError(t, err)
	for _, example := range examples {
		if !example.IsDir() {
			continue
		}
		t.Run("check example "+example.Name(), func(t *testing.T) {
			ctx := t.Context()
			buf := bytes.NewBuffer(nil)
			cmd := exec.CommandContext(ctx, "go", "run", mainPackage, "-C", filepath.FromSlash("../../docs/examples/"+example.Name()), "check")
			cmd.Dir = "."
			cmd.Stderr = buf
			cmd.Stdout = buf
			require.NoError(t, cmd.Run(), buf.String())
		})
	}
}

func TestEntrypoint(t *testing.T) {
	cmdMainGo, err := os.ReadFile("main.go")
	require.NoError(t, err)
	mainGo, err := os.ReadFile(filepath.FromSlash("../../main.go"))
	require.NoError(t, err)
	require.Equal(t, string(cmdMainGo), string(mainGo))
}

// scriptWaitDelay is how long a script's exec waits for a subprocess's output
// to drain after it exits, and, when the script is cancelled, between the
// interrupt and the kill.
const scriptWaitDelay = 10 * time.Second

func Test(t *testing.T) {
	// The scripts extract scratch modules that are never part of a
	// workspace; a GOWORK from the invoking environment must not leak
	// into their package loading.
	t.Setenv("GOWORK", "off")
	e := script.NewEngine()
	e.Quiet = true
	e.Cmds = scripttest.DefaultCmds()
	// The engine reads a subprocess's output through a pipe and, once the
	// subprocess has exited, waits only scriptWaitDelay for that pipe to
	// drain before failing the script with "exec: WaitDelay expired before
	// I/O complete". The default of 100ms is too short when this process is
	// starved of CPU, as it is when CI runs many scripts at once under -race:
	// a script whose go test has already passed then fails.
	e.Cmds["exec"] = script.Exec(func(cmd *exec.Cmd) error { return cmd.Process.Signal(os.Interrupt) }, scriptWaitDelay)
	var inputs sync.Map // *script.State to the standard input of its next muxt command
	e.Cmds["stdin"] = stdinCommand(&inputs)
	e.Cmds["muxt"] = scriptCommand(&inputs)
	e.Cmds["count-matches"] = countRedirectBlocksCommand()
	ctx := t.Context()
	scripttest.Test(t, ctx, e, nil, filepath.FromSlash("testdata/*.txt"))
}

// stdinCommand sets the standard input of the script's next muxt command
// to a file's contents, as the go command's script engine does for exec.
func stdinCommand(inputs *sync.Map) script.Cmd {
	return script.Command(script.CmdUsage{
		Summary: "set the standard input of the next muxt command to the file's contents",
		Args:    "file",
	}, func(state *script.State, args ...string) (script.WaitFunc, error) {
		if len(args) != 1 {
			return nil, script.ErrUsage
		}
		data, err := os.ReadFile(state.Path(args[0]))
		if err != nil {
			return nil, err
		}
		inputs.Store(state, string(data))
		return nil, nil
	})
}

// scriptCommand runs muxt in the script's working directory. Its standard
// input is the file the last stdin command named, or empty.
func scriptCommand(inputs *sync.Map) script.Cmd {
	return script.Command(script.CmdUsage{
		Summary: "muxt",
		Args:    "",
	}, func(state *script.State, args ...string) (script.WaitFunc, error) {
		var stdin string
		if v, ok := inputs.LoadAndDelete(state); ok {
			stdin = v.(string)
		}
		return func(state *script.State) (string, string, error) {
			var stdout, stderr bytes.Buffer
			err := cli.Commands(state.Getwd(), args, func(s string) string {
				e, _ := state.LookupEnv(s)
				return e
			}, strings.NewReader(stdin), &stdout, &stderr)
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
