package main

import (
	"flag"
	"fmt"
	"io"
	"os"
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
		switch args[0] {
		case "generate", "gen", "g":
			return generateCommand(args[1:], wd, getEnv, stdout, stderr)
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
