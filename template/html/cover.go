package html

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/typelate/muxt/template"
)

// CoverEnv names the environment variable that turns on node coverage
// export. muxt sets it to a directory when it runs a package's tests with
// this engine laid over html/template; each test binary then appends the
// template nodes it executes to <dir>/<pid>.cov in CoverProfile.WriteTo's
// format, and muxt unions the files afterward.
const CoverEnv = "MUXT_TEMPLATE_COVER"

// CoverDisabledMarker starts the one line a test binary prints on stderr
// when its coverage export fails and stops; the error follows it. muxt
// reads a run with this line in its output as coverage unknown.
const CoverDisabledMarker = "muxt: template coverage export disabled:"

// processCover is the process's coverage export: nil, the normal case,
// unless coverEnv named a directory when the package was initialized. It
// is this package's only mutable state. The environment can be read only
// once per process, as there is no exit hook in which to flush a profile
// collected in memory, so the export appends as it goes instead.
var processCover = coverExportFor(os.Getenv(CoverEnv), os.Stderr)

// coverExport appends the blocks of executed template nodes to a cover
// profile file the first time each is reached. Mode set is monotone, so
// a block is written once; every write is synchronous, so nothing is lost
// when the test binary exits, and no goroutine or timer is involved.
//
// The file, with its mode line, is created as the export is made, when
// the package is initialized, before any template runs: its existence is
// what tells muxt that a test binary ran this engine, so that a profile
// with no blocks reads as templates no test reached, and no profile at
// all as coverage unknown.
//
// Coverage is a by-product of the user's tests, so it never fails them:
// the first error opening or writing the file is reported once on stderr
// and turns the export off for the rest of the process.
type coverExport struct {
	path   string
	stderr io.Writer

	mu       sync.Mutex
	disabled bool
	file     *os.File
	written  map[string]struct{}
}

// coverExportFor is the export to <dir>/<pid>.cov, which it opens, nil
// when dir is empty. A file it cannot open is reported on stderr, and the
// export it returns is off.
func coverExportFor(dir string, stderr io.Writer) *coverExport {
	if dir == "" {
		return nil
	}
	c := &coverExport{
		path:    filepath.Join(dir, strconv.Itoa(os.Getpid())+".cov"),
		stderr:  stderr,
		written: make(map[string]struct{}),
	}
	f, err := openProfile(c.path)
	if err != nil {
		c.disable(err)
		return c
	}
	// The file stays open for the life of the process; the operating
	// system closes it at exit, and every write before then has already
	// reached it.
	c.file = f
	return c
}

// disable turns the export off and says why, once. The caller holds mu,
// or is the only one with c.
func (c *coverExport) disable(err error) {
	c.disabled = true
	// The export is off from here on; a failure to report that has
	// nowhere better to go.
	_, _ = fmt.Fprintf(c.stderr, "%s %v\n", CoverDisabledMarker, err)
}

// execute runs tree with a cover profile attached, then appends the blocks
// the run reached for the first time. A profile per execute, rather than
// one for the process, keeps the export from holding every tree ever
// parsed and walking all of them after each execute; c.written carries
// what is already in the file. It returns only the execution error.
func (c *coverExport) execute(wr io.Writer, tree *template.Tree, data any, opts []template.Option) error {
	cp := new(template.CoverProfile)
	err := template.ExecuteTemplate(wr, tree, data, append(opts, template.WithCoverProfile(cp))...)
	c.append(cp)
	return err
}

func (c *coverExport) append(cp *template.CoverProfile) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.disabled {
		return
	}
	if _, err := cp.WriteNewlyCovered(c.file, c.written); err != nil {
		c.disable(err)
	}
}

// openProfile opens name for appending, writing the mode line only if the
// file is empty: a predecessor with the same pid, as after syscall.Exec,
// may already have written to it.
func openProfile(name string) (*os.File, error) {
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		return nil, errors.Join(err, f.Close())
	}
	if info.Size() == 0 {
		if _, err := io.WriteString(f, "mode: set\n"); err != nil {
			return nil, errors.Join(err, f.Close())
		}
	}
	return f, nil
}
