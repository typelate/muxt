package templatecover

import (
	"errors"
	"fmt"
	"path/filepath"

	"golang.org/x/tools/cover"
)

// Block is a template block's span in its template, as a cover profile
// gives it.
type Block struct {
	StartLine, StartCol, EndLine, EndCol int
}

// Reached is the template blocks the tests executed, by the parse name of
// the template they are in, which is a file's base name: two files with one
// base name share a key.
//
// Nil means what was reached is unknown, and nothing may be read from it;
// an empty Reached means the tests executed no template at all.
type Reached map[string]map[Block]struct{}

// ErrNoProfile is a run in which no test binary wrote a profile.
var ErrNoProfile = errors.New("no test binary ran the templates (no coverage profile was written)")

// ReadProfiles unions every <pid>.cov in dir by block. A block is in the
// union when any test binary executed it.
//
// Every test binary that runs the engine writes its profile as it starts,
// so a profile with no blocks is a binary that executed no template.
// No profile at all is ErrNoProfile: nothing says the engine ran, as when
// a package has no test files, -exec runs the tests elsewhere, or they
// render through a server binary of their own, and reading it as nothing
// reached would skip every mutant.
func ReadProfiles(dir string) (Reached, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.cov"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, ErrNoProfile
	}
	reached := make(Reached)
	for _, name := range files {
		profiles, err := cover.ParseProfiles(name)
		if err != nil {
			return nil, fmt.Errorf("reading template coverage: %w", err)
		}
		for _, p := range profiles {
			for _, b := range p.Blocks {
				if b.Count == 0 {
					continue
				}
				blocks := reached[p.FileName]
				if blocks == nil {
					blocks = make(map[Block]struct{})
					reached[p.FileName] = blocks
				}
				blocks[Block{StartLine: b.StartLine, StartCol: b.StartCol, EndLine: b.EndLine, EndCol: b.EndCol}] = struct{}{}
			}
		}
	}
	return reached, nil
}
