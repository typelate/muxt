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
			// A package loaded by directory pattern keeps the directory as
			// its path when module resolution fails. Saying "no Go package
			// found" above a list containing that very directory reads
			// self-contradictory, so report the truth: it loaded, broken.
			if p.PkgPath == dir && len(p.Errors) > 0 {
				e.LoadedWithErrors = true
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

	// Summary overrides the short form when set.
	Summary string

	// LoadedWithErrors reports that the directory's package did load but
	// carried loader errors, so "no Go package found" would be wrong.
	LoadedWithErrors bool

	// Details are indented under the short form, one line each.
	Details []string
}

func (e *PackageLookupError) Error() string {
	if e.Summary != "" {
		return e.Summary
	}
	if e.LoadedWithErrors {
		return "the Go package at " + e.Dir + " loaded, but with errors"
	}
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
	// go work use wants the module root, not the package directory.
	module := moduleRoot(dir)
	switch gowork := os.Getenv("GOWORK"); gowork {
	case "off":
		return ""
	case "", "auto":
		// Empty and "auto" both mean the go command discovers the
		// nearest go.work in a parent directory.
		for d := dir; ; {
			workFile := filepath.Join(d, "go.work")
			if _, err := os.Stat(workFile); err == nil {
				return fmt.Sprintf("a workspace file at %s is in effect; if it does not list this module, run with GOWORK=off or add the module with: go work use %s", workFile, module)
			}
			parent := filepath.Dir(d)
			if parent == d {
				return ""
			}
			d = parent
		}
	default:
		return fmt.Sprintf("GOWORK=%s is set; if that workspace does not list this module, run with GOWORK=off or add the module with: go work use %s", gowork, module)
	}
}

// loadFailedError frames a packages.Load failure. The driver's own
// error arrives triple-wrapped ("err: exit status 1: stderr: go: …"),
// so the go error is unwrapped from the plumbing and gets the same
// workspace guidance a failed lookup gets.
func loadFailedError(dir string, err error) error {
	msg := err.Error()
	if idx := strings.LastIndex(msg, "stderr: "); idx >= 0 {
		msg = strings.TrimSpace(msg[idx+len("stderr: "):])
	}
	e := &PackageLookupError{
		Dir:     dir,
		Summary: "failed to load Go packages from " + dir,
		Details: []string{msg},
	}
	if note := workspaceNote(dir); note != "" {
		e.Details = append(e.Details, note)
	}
	e.Details = append(e.Details, "muxt loads Go packages like the go command and inherits GOWORK, GOFLAGS, and GOROOT")
	return e
}

// moduleRoot walks up from dir to the nearest directory containing a
// go.mod, falling back to dir when none is found.
func moduleRoot(dir string) string {
	for d := dir; ; {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return dir
		}
		d = parent
	}
}
