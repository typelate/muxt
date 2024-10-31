package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"

	"rsc.io/script"
)

func main() {
	flag.Parse()
	wd, err := os.Getwd()
	if err != nil {
		os.Exit(handleError(err))
	}
	os.Exit(handleError(command(wd, flag.Args(), os.Getenv, os.Stdout, os.Stderr)))
}

func command(wd string, args []string, getEnv func(string) string, stdout, stderr io.Writer) error {
	if len(args) > 0 {
		switch cmd, cmdArgs := args[0], args[1:]; cmd {
		case "generate", "gen", "g":
			return generateCommand(cmdArgs, wd, getEnv, stdout, stderr)
		case "version", "v":
			return versionCommand(stdout)
		}
	}
	return fmt.Errorf("unknown command")
}

func handleError(err error) int {
	if err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		return 1
	}
	return 0
}

func scriptCommand() script.Cmd {
	return script.Command(script.CmdUsage{
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
}
