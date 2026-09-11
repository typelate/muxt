package mutation

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/typelate/muxt/internal/asteval"
)

// revision is what the templates looked like at an earlier commit: the
// text of each template, keyed by the template and the type of dot it was
// reached with there.
//
// A template is mutated once per type of dot, so that pair is also what
// decides whether mutating it again has anything new to say. The text is
// compared as the parser reads it, so a template that only moved to
// another file has not changed.
type revision map[string]string

// changed reports whether a scope holds anything the revision did not:
// text that reads differently, or a type of dot the template was not
// reached with.
//
// A type is compared by name. A template that still receives the same
// named type is unchanged even when that type gained or lost fields; the
// template text decides what it reads from them.
func (r revision) changed(sc scope) bool {
	text, reached := r[executionKey(sc.template, sc.dataType)]
	return !reached || text != sc.tree.Root.String()
}

// templatesAt reads the templates in dir, a copy of the working directory
// at the revision being compared with.
func templatesAt(config Configuration, dir string) (revision, error) {
	pl, err := loadPackages(dir, config.IncludeTests)
	if err != nil {
		return nil, err
	}
	before := make(revision)
	for _, templatesVariable := range config.TemplatesVariables {
		lt, err := asteval.LoadTemplates(dir, templatesVariable, pl)
		if err != nil {
			return nil, err
		}
		index, err := buildTreeIndex(lt, dir, pl, lt.Templates.Functions())
		if err != nil {
			return nil, err
		}
		scopes, _ := traverse(lt, index)
		for _, sc := range scopes {
			before[executionKey(sc.template, sc.dataType)] = sc.tree.Root.String()
		}
	}
	return before, nil
}

// checkout writes the repository's tree at ref into a temporary directory
// and returns the directory in it that corresponds to workingDirectory,
// along with a function that removes the copy.
//
// git archive writes nothing into the repository, unlike a worktree, so a
// run that is interrupted leaves nothing behind in it.
func checkout(workingDirectory, ref string) (string, func(), error) {
	commit, err := git(workingDirectory, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return "", nil, fmt.Errorf("--diff %s: %w", ref, err)
	}
	top, err := git(workingDirectory, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", nil, fmt.Errorf("--diff %s: %w", ref, err)
	}
	prefix, err := git(workingDirectory, "rev-parse", "--show-prefix")
	if err != nil {
		return "", nil, fmt.Errorf("--diff %s: %w", ref, err)
	}

	root, err := os.MkdirTemp("", "muxt-diff-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(root) }

	// Run from a subdirectory, git archive writes only that subdirectory.
	// The whole tree is needed: the go.mod the package loads with, and
	// anything else it imports, may sit above it.
	archive := exec.Command("git", "archive", "--format=tar", commit)
	archive.Dir = top
	stream, err := archive.StdoutPipe()
	if err != nil {
		cleanup()
		return "", nil, err
	}
	if err := archive.Start(); err != nil {
		cleanup()
		return "", nil, err
	}
	if err := extract(stream, root); err != nil {
		_ = archive.Process.Kill()
		_ = archive.Wait()
		cleanup()
		return "", nil, fmt.Errorf("--diff %s: %w", ref, err)
	}
	if err := archive.Wait(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("--diff %s: git archive: %w", ref, err)
	}
	return filepath.Join(root, filepath.FromSlash(prefix)), cleanup, nil
}

// git runs a git command in dir and returns its output, or what git said
// when it failed.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		// Output keeps what git wrote to stderr, which says why far better
		// than its exit status does.
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// extract writes the files in a tar stream under dir.
func extract(r io.Reader, dir string) error {
	archive := tar.NewReader(r)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if !filepath.IsLocal(header.Name) {
			return fmt.Errorf("archive entry %q is outside the tree", header.Name)
		}
		path := filepath.Join(dir, filepath.FromSlash(header.Name))
		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(path, 0o700)
		case tar.TypeReg:
			err = writeArchived(path, archive, header.FileInfo().Mode().Perm())
		case tar.TypeSymlink:
			if err = os.MkdirAll(filepath.Dir(path), 0o700); err == nil {
				err = os.Symlink(header.Linkname, path)
			}
		}
		// Anything else, such as the global header git writes to name the
		// commit, holds no file.
		if err != nil {
			return err
		}
	}
}

func writeArchived(path string, r io.Reader, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm|0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
