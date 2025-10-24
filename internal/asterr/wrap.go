package asterr

import (
	"fmt"
	"go/token"
	"path/filepath"
)

func WrapWithFilename(workingDirectory string, set *token.FileSet, pos token.Pos, err error) error {
	p := set.Position(pos)
	p.Filename, _ = filepath.Rel(workingDirectory, p.Filename)
	return fmt.Errorf("%s: %w", p, err)
}
