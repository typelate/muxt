package cli

import (
	"flag"
	"io"
	"path/filepath"
)

func global(wd string, args []string, stdout io.Writer) (string, []string, error) {
	var changeDir string
	flagSet := flag.NewFlagSet("muxt global", flag.ExitOnError)
	flagSet.SetOutput(stdout)
	flagSet.StringVar(&changeDir, "C", "", "change root directory")
	if err := flagSet.Parse(args); err != nil {
		return "", nil, err
	}
	if filepath.IsAbs(changeDir) {
		return changeDir, flagSet.Args(), nil
	}
	cd, err := filepath.Abs(filepath.Join(wd, changeDir))
	if err != nil {
		return "", nil, err
	}
	return cd, flagSet.Args(), nil
}
