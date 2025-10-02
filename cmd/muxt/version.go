package main

import (
	"fmt"
	"io"
	"runtime/debug"
)

func versionCommand(stdout io.Writer) error {
	v, ok := cliVersion()
	if !ok {
		return fmt.Errorf("missing CLI version")
	}
	_, err := fmt.Fprintln(stdout, v)
	return err
}

func cliVersion() (string, bool) {
	bi, ok := debug.ReadBuildInfo()
	if !ok || bi.Main.Version == "" {
		return "", false
	}
	return bi.Main.Version, true
}
