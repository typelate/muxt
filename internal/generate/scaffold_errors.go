package generate

import "fmt"

// DuplicateDefinitionError reports a route template the package's templates
// already define. It carries where, so a caller can point at the existing
// definition instead of adding a second one.
type DuplicateDefinitionError struct {
	// Name is the template name asked for.
	Name string
	// File is where Name is defined, an absolute path an editor can open, or
	// empty when the definition has no known location.
	File string
	// Line is the line of the definition in File, or 0 when File is empty.
	Line int
}

func (e *DuplicateDefinitionError) Error() string {
	if e.File == "" {
		return fmt.Sprintf("template %q is already defined", e.Name)
	}
	return fmt.Sprintf("template %q is already defined at %s:%d", e.Name, e.File, e.Line)
}
