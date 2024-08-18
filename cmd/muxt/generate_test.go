package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"rsc.io/script"
	"rsc.io/script/scripttest"
)

func TestGenerate(t *testing.T) {
	e := script.NewEngine()
	e.Quiet = true
	e.Cmds = scripttest.DefaultCmds()
	e.Cmds["muxt"] = script.Command(script.CmdUsage{
		Summary: "muxt",
		Args:    "",
	}, func(state *script.State, args ...string) (script.WaitFunc, error) {
		return func(state *script.State) (string, string, error) {
			var stdout, stderr bytes.Buffer
			err := command(state.Getwd(), args, func(s string) string {
				e, _ := state.LookupEnv(s)
				return e
			}, &stdout, &stderr)
			if err != nil {
				stderr.WriteString(err.Error())
			}
			return stdout.String(), stderr.String(), err
		}, nil
	})
	testFiles, err := filepath.Glob(filepath.FromSlash("testdata/generate/*.txtar"))
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
