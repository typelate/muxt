package load

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// NoPackageError explains a failed package lookup at dir: no loaded
// package matched it. The short Error form names the directory; the
// MultiLineError form lists what did load, forwards the loader's own
// errors, and names workspace state when a go.work file may be
// excluding the module, since muxt loads packages like the go command
// and inherits GOWORK, GOFLAGS, and GOROOT. The workspace is the one the
// go command reports in the process's environment.
func NoPackageError(dir string, pl []*packages.Package) error {
	e := &PackageLookupError{Dir: dir}
	if len(pl) == 0 {
		e.Details = append(e.Details, "go/packages loaded no packages")
	} else {
		var listing string
		listing, e.LoadedWithErrors = describeLoaded(dir, pl)
		e.Details = append(e.Details, listing)
	}
	e.Details = append(e.Details, loadErrorDetails(pl)...)
	if note := workspaceNote(dir, nil); note != "" {
		e.Details = append(e.Details, note)
	}
	e.Details = append(e.Details, "muxt loads Go packages like the go command and inherits GOWORK, GOFLAGS, and GOROOT")
	return e
}

// describeLoaded lists the loaded package paths. It also reports whether the
// package at dir loaded with errors: a package loaded by directory pattern
// keeps the directory as its path when module resolution fails, and saying
// "no Go package found" above a list containing that very directory reads
// self-contradictory.
func describeLoaded(dir string, pl []*packages.Package) (string, bool) {
	paths := make([]string, 0, len(pl))
	loadedWithErrors := false
	for _, p := range pl {
		if p.PkgPath != "" {
			paths = append(paths, p.PkgPath)
		}
		if p.PkgPath == dir && len(p.Errors) > 0 {
			loadedWithErrors = true
		}
	}
	return fmt.Sprintf("loaded %d packages: %s", len(pl), strings.Join(paths, ", ")), loadedWithErrors
}

// loadErrorDetails forwards the loader's own errors, up to a limit.
func loadErrorDetails(pl []*packages.Package) []string {
	const maxLoadErrors = 3
	var details []string
	for _, p := range pl {
		for _, loadErr := range p.Errors {
			if len(details) == maxLoadErrors {
				return append(details, "(more load errors omitted)")
			}
			details = append(details, loadErr.Error())
		}
	}
	return details
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

// workspaceNote reports the go.work file that governs dir, if any. A
// workspace that does not list dir's module makes every package lookup
// under dir come up empty, which is otherwise invisible from the error.
//
// The go command is asked, in env (nil for the process's own), the one
// the load ran in: GOWORK may be set there, in the GOENV file, or not at
// all, with the nearest go.work in a parent directory in effect.
func workspaceNote(dir string, env []string) string {
	cmd := exec.Command("go", "env", "GOWORK")
	cmd.Dir = existingDir(dir)
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		// The hint is a guess at a cause; the failure it explains is
		// reported either way.
		return ""
	}
	workFile := strings.TrimSpace(string(out))
	if workFile == "" || workFile == "off" {
		return ""
	}
	// go work use wants the module root, not the package directory.
	return fmt.Sprintf("the workspace file %s is in effect; if it does not list this module, run with GOWORK=off or add the module with: go work use %s", workFile, moduleRoot(dir))
}

// existingDir returns dir, or its nearest ancestor that exists: a lookup
// can name a directory nothing was ever written to.
func existingDir(dir string) string {
	for d := dir; ; {
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return d
		}
		d = parent
	}
}

// loadFailedError frames a packages.Load failure. The short form carries
// the go command's own message, since it is the cause and most commands
// print only the short form; the workspace guidance a failed lookup gets
// goes in the details.
func loadFailedError(dir string, env []string, err error) error {
	e := &PackageLookupError{
		Dir:     dir,
		Summary: "failed to load Go packages from " + dir + ": " + goMessage(err),
	}
	if note := workspaceNote(dir, env); note != "" {
		e.Details = append(e.Details, note)
	}
	e.Details = append(e.Details, "muxt loads Go packages like the go command and inherits GOWORK, GOFLAGS, and GOROOT")
	return e
}

// goMessage returns what the go command wrote to stderr when the go list
// driver failed. The driver wraps it as "err: <exit status>: stderr: <go
// message>"; only that leading plumbing is removed, so a go message that
// itself contains "stderr: " is kept whole.
func goMessage(err error) string {
	msg := err.Error()
	if rest, ok := strings.CutPrefix(msg, "err: "); ok {
		if _, stderr, ok := strings.Cut(rest, ": stderr: "); ok {
			msg = stderr
		}
	} else if stderr, ok := strings.CutPrefix(msg, "stderr: "); ok {
		msg = stderr
	}
	return strings.TrimSpace(msg)
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
