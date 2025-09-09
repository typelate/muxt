package cli

import (
	"flag"
	"io"
	"path/filepath"
)

func Global(wd string, args []string, stdout io.Writer) (string, []string, error) {
	var changeDir string
	global := flag.NewFlagSet("muxt global", flag.ExitOnError)
	global.SetOutput(stdout)
	global.StringVar(&changeDir, "C", "", "change root directory")
	if err := global.Parse(args); err != nil {
		return "", nil, err
	}
	if filepath.IsAbs(changeDir) {
		return changeDir, global.Args(), nil
	}
	cd, err := filepath.Abs(filepath.Join(wd, changeDir))
	if err != nil {
		return "", nil, err
	}
	return cd, global.Args(), nil
}
