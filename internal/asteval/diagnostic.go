package asteval

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// NoPackageError explains a failed package lookup at dir: no loaded
// package matched it. The short Error form names the directory; the
// MultiLineError form lists what did load, forwards the loader's own
// errors, and names workspace state when a go.work file may be
// excluding the module, since muxt loads packages like the go command
// and inherits GOWORK, GOFLAGS, and GOROOT.
func NoPackageError(dir string, pl []*packages.Package) error {
	e := &PackageLookupError{Dir: dir}
	if len(pl) == 0 {
		e.Details = append(e.Details, "go/packages loaded no packages")
	} else {
		paths := make([]string, 0, len(pl))
		for _, p := range pl {
			if p.PkgPath != "" {
				paths = append(paths, p.PkgPath)
			}
		}
		e.Details = append(e.Details, fmt.Sprintf("loaded %d packages: %s", len(pl), strings.Join(paths, ", ")))
	}
	const maxLoadErrors = 3
	shown := 0
	for _, p := range pl {
		for _, loadErr := range p.Errors {
			if shown == maxLoadErrors {
				e.Details = append(e.Details, "(more load errors omitted)")
				break
			}
			e.Details = append(e.Details, loadErr.Error())
			shown++
		}
		if shown == maxLoadErrors {
			break
		}
	}
	if note := workspaceNote(dir); note != "" {
		e.Details = append(e.Details, note)
	}
	e.Details = append(e.Details, "muxt loads Go packages like the go command and inherits GOWORK, GOFLAGS, and GOROOT")
	return e
}

// PackageLookupError reports that no loaded package matched a
// directory. Error is the short single-line form; MultiLineError adds
// one detail line each for what did load, the loader's own errors, and
// the workspace state steering resolution.
type PackageLookupError struct {
	// Dir is the directory whose package lookup came up empty.
	Dir string

	// Details are indented under the short form, one line each.
	Details []string
}

func (e *PackageLookupError) Error() string {
	return "no Go package found at " + e.Dir
}

// MultiLineError renders the short form with each detail line
// indented under it.
func (e *PackageLookupError) MultiLineError() string {
	var sb strings.Builder
	sb.WriteString(e.Error())
	for _, detail := range e.Details {
		sb.WriteString("\n\t")
		sb.WriteString(detail)
	}
	return sb.String()
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
