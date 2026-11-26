package cli

import (
	"fmt"
	"io"

	"github.com/typelate/muxt/internal/diagnostic"
)

// warningLine is how a command prints a warning: the diagnostic's text after
// the word "warning: ". The severity stays out of the text, so an error and a
// warning of the same words differ by this prefix alone.
func warningLine(d diagnostic.Diagnostic) string {
	return "warning: " + d.String()
}

// writeWarning writes d as a warning line to w.
func writeWarning(w io.Writer, d diagnostic.Diagnostic) error {
	if _, err := fmt.Fprintln(w, warningLine(d)); err != nil {
		return fmt.Errorf("writing a warning: %w", err)
	}
	return nil
}
