package asteval

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// NoPackageError explains a failed package lookup at dir: no loaded
// package matched it. The message lists what did load, forwards the
// loader's own errors, and names workspace state when a go.work file
// may be excluding the module, since muxt loads packages like the go
// command and inherits GOWORK, GOFLAGS, and GOROOT.
func NoPackageError(dir string, pl []*packages.Package) error {
	var b strings.Builder
	fmt.Fprintf(&b, "no Go package found at %s", dir)
	if len(pl) == 0 {
		b.WriteString("\n\tgo/packages loaded no packages")
	} else {
		paths := make([]string, 0, len(pl))
		for _, p := range pl {
			if p.PkgPath != "" {
				paths = append(paths, p.PkgPath)
			}
		}
		fmt.Fprintf(&b, "\n\tloaded %d packages: %s", len(pl), strings.Join(paths, ", "))
	}
	const maxLoadErrors = 3
	shown := 0
	for _, p := range pl {
		for _, loadErr := range p.Errors {
			if shown == maxLoadErrors {
				b.WriteString("\n\t(more load errors omitted)")
				break
			}
			fmt.Fprintf(&b, "\n\t%s", loadErr)
			shown++
		}
		if shown == maxLoadErrors {
			break
		}
	}
	if note := workspaceNote(dir); note != "" {
		fmt.Fprintf(&b, "\n\t%s", note)
	}
	b.WriteString("\n\tmuxt loads Go packages like the go command and inherits GOWORK, GOFLAGS, and GOROOT")
	return errors.New(b.String())
}

// workspaceNote reports the go.work file that governs dir, if any:
// either the file GOWORK names or the nearest go.work in a parent
// directory (the go command's own discovery rule). A workspace that
// does not list dir's module makes every package lookup under dir come
// up empty, which is otherwise invisible from the error.
func workspaceNote(dir string) string {
	switch gowork := os.Getenv("GOWORK"); gowork {
	case "off":
		return ""
	case "", "auto":
		// Empty and "auto" both mean the go command discovers the
		// nearest go.work in a parent directory.
		for d := dir; ; {
			workFile := filepath.Join(d, "go.work")
			if _, err := os.Stat(workFile); err == nil {
				return fmt.Sprintf("a workspace file at %s is in effect; if it does not list this module, run with GOWORK=off or add the module with: go work use %s", workFile, dir)
			}
			parent := filepath.Dir(d)
			if parent == d {
				return ""
			}
			d = parent
		}
	default:
		return fmt.Sprintf("GOWORK=%s is set; if that workspace does not list this module, run with GOWORK=off or add the module with: go work use %s", gowork, dir)
	}
}
