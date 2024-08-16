package generate_test

import (
	"bytes"
	"context"
	"log"
	"path/filepath"
	"strings"
	"testing"

	"rsc.io/script"
	"rsc.io/script/scripttest"

	"github.com/crhntr/muxt/internal/generate"
)

func Test(t *testing.T) {
	e := script.NewEngine()
	e.Quiet = true
	e.Cmds = scripttest.DefaultCmds()
	e.Cmds["generate"] = script.Command(script.CmdUsage{
		Summary: "executes muxt generate",
		Args:    "",
	}, func(state *script.State, args ...string) (script.WaitFunc, error) {
		return func(state *script.State) (string, string, error) {
			var buf bytes.Buffer
			logger := log.New(&buf, "", 0)
			err := generate.Command(args, state.Getwd(), logger, state.LookupEnv)
			if err != nil {
				buf.WriteString(err.Error())
			}
			return buf.String(), "", err
		}, nil
	})
	testFiles, err := filepath.Glob(filepath.FromSlash("testdata/*.txtar"))
	if err != nil {
		t.Fatal(err)
	}

	for _, filePath := range testFiles {
		name := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			scripttest.Test(t, ctx, e, nil, filePath)
		})
	}
}
