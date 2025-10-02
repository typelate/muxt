package main

import (
	"os"

	"github.com/typelate/muxt/internal/cli"
)

func main() {
	wd, err := os.Getwd()
	if err != nil {
		os.Exit(handleError(err))
	}
	os.Exit(handleError(cli.Commands(wd, os.Args, os.Getenv, os.Stdout, os.Stderr)))
}

func handleError(err error) int {
	if err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		return 1
	}
	return 0
}
