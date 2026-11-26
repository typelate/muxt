package generate

import (
	"log"

	"github.com/typelate/muxt/internal/diagnostic"
)

// logWarning prints d to logger as a warning: the diagnostic's text after
// the word "warning: ".
func logWarning(logger *log.Logger, d diagnostic.Diagnostic) {
	logger.Print("warning: " + d.String())
}
