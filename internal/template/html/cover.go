package html

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/typelate/muxt/internal/template"
)

// coverEnv names the environment variable that turns on node coverage
// export. muxt sets it to a directory when it runs a package's tests with
// this engine laid over html/template; each test binary then appends the
// template nodes it executes to <dir>/<pid>.cov in CoverProfile.WriteTo's
// format, and muxt unions the files afterward.
const coverEnv = "MUXT_TEMPLATE_COVER"

// processCover is the process's coverage export: nil, the normal case,
// unless coverEnv named a directory when the package was initialized. It
// is this package's only mutable state. The environment can be read only
// once per process, as there is no exit hook in which to flush a profile
// collected in memory, so the export appends as it goes instead.
var processCover = coverExportFor(os.Getenv(coverEnv))

// coverExport appends the blocks of executed template nodes to a cover
// profile file the first time each is reached. Mode set is monotone, so
// a block is written once; every write is synchronous, so nothing is lost
// when the test binary exits, and no goroutine or timer is involved.
type coverExport struct {
	path string

	mu      sync.Mutex
	file    *os.File
	written map[string]struct{}
}

func coverExportFor(dir string) *coverExport {
	if dir == "" {
		return nil
	}
	return &coverExport{
		path:    filepath.Join(dir, strconv.Itoa(os.Getpid())+".cov"),
		written: make(map[string]struct{}),
	}
}

// execute runs tree with a cover profile attached, then appends the blocks
// the run reached for the first time. A profile per execute, rather than
// one for the process, keeps the export from holding every tree ever
// parsed and walking all of them after each execute; c.written carries
// what is already in the file. An export failure is returned along with
// any execution error so a broken coverage run cannot pass silently.
func (c *coverExport) execute(wr io.Writer, tree *template.Tree, data any, opts []template.Option) error {
	cp := new(template.CoverProfile)
	err := template.ExecuteTemplate(wr, tree, data, append(opts, template.WithCoverProfile(cp))...)
	if exportErr := c.append(cp); exportErr != nil {
		return errors.Join(err, fmt.Errorf("html/template: exporting template coverage: %w", exportErr))
	}
	return err
}

func (c *coverExport) append(cp *template.CoverProfile) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.file == nil {
		f, err := os.OpenFile(c.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(f, "mode: set\n"); err != nil {
			return errors.Join(err, f.Close())
		}
		// The file stays open for the life of the process; the operating
		// system closes it at exit, and every write before then has
		// already reached it.
		c.file = f
	}
	_, err := cp.WriteNewlyCovered(c.file, c.written)
	return err
}
