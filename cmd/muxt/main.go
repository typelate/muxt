package main

import (
	"fmt"
	"io"
	"os"

	"github.com/typelate/muxt/internal/configuration"
)

func main() {
	wd, err := os.Getwd()
	if err != nil {
		os.Exit(handleError(err))
	}
	if len(os.Args) == 1 {
		writeHelp(os.Stderr)
		return
	}
	os.Exit(handleError(command(wd, os.Args[1:], os.Getenv, os.Stdout, os.Stderr)))
}

func command(wd string, args []string, getEnv func(string) string, stdout, stderr io.Writer) error {
	var err error
	wd, args, err = configuration.Global(wd, args, stderr)
	if err != nil {
		return err
	}
	switch cmd, cmdArgs := args[0], args[1:]; cmd {
	case "generate", "gen", "g":
		return generateCommand(wd, cmdArgs, getEnv, stdout, stderr)
	case "version", "v":
		return versionCommand(stdout)
	case "check", "c", "typelate":
		return checkCommand(wd, cmdArgs, stderr)
	case "documentation", "docs", "d":
		return documentationCommand(wd, cmdArgs, stdout, stderr)
	default:
		return fmt.Errorf("unknown command")
	}
}

func handleError(err error) int {
	if err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		return 1
	}
	return 0
}
