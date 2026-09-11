package mutation

import (
	"errors"
	"strings"
)

// CheckGoTestArgs refuses a pass-through flag muxt sets itself.
//
// -overlay is how a mutant reaches the build. A second one would either be
// ignored or replace the mutant, and either way the run would measure the
// templates as written rather than as mutated.
func CheckGoTestArgs(args []string) error {
	for _, arg := range args {
		name, _, _ := strings.Cut(arg, "=")
		if strings.TrimLeft(name, "-") == "overlay" {
			return errors.New("go test flag -overlay cannot be passed through: muxt uses -overlay to deliver each mutant")
		}
	}
	return nil
}
