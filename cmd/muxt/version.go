package main

import (
	"io"
	"runtime/debug"
)

func versionCommand(stdout io.Writer) error {
	v, _ := cliVersion()
	_, err := io.WriteString(stdout, v)
	return err
}

func cliVersion() (string, bool) {
	bi, ok := debug.ReadBuildInfo()
	if !ok || bi.Main.Version == "" {
		return "", false
	}
	return bi.Main.Version, true
}
