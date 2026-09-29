package cli

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log"

	"github.com/spf13/cobra"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/load"
	"github.com/typelate/muxt/internal/muxt"
)

// warnPartialAST says when the checks that follow ran against source the
// parser could not fully read.
//
// muxt is not the gate on a package compiling -- go build is -- and its
// template checks are worth running on broken Go, so this does not stop
// the command. What it stops is the silence: without it, a package go
// build rejects gets the same clean output as one that passes.
func warnPartialAST(logger *log.Logger, pl []*packages.Package) {
	if logger == nil || len(load.ParseErrors(pl)) == 0 {
		return
	}
	logger.Printf("warning: package has syntax errors, so these checks ran against a partial AST; run go build for the full picture")
}

// checkFailure reports why muxt check could not finish: a multi-line
// error prints as its own diagram and is returned as it is, and anything
// else -- a package that would not load, a templates variable that does
// not evaluate -- is returned as a fail line.
func checkFailure(cmd *cobra.Command, err error) error {
	if printMultiLineError(cmd, err) {
		return err
	}
	return fmt.Errorf("fail: %s", err)
}

// printMultiLineError renders err's verbose form to stderr — an
// "Error:" heading then the multi-line rendering (markers, positions,
// one location per line) — and silences cobra's inline prefix so every
// path-first line stays clickable. It reports whether err had a
// multi-line form.
func printMultiLineError(cmd *cobra.Command, err error) bool {
	multiLine, ok := errors.AsType[muxt.MultiLineError](err)
	if !ok {
		return false
	}
	cmd.SilenceErrors = true
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Error:")
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), multiLine.MultiLineError())
	return true
}

// resultJSON writes a --format=json result as encoding/json wrote it before
// muxt moved to encoding/json/v2, so a script reading it sees no change:
// map members sorted by key, a nil list or map as null, and <, >, &, U+2028
// and U+2029 escaped.
var resultJSON = json.JoinOptions(
	jsontext.WithIndent("\t"),
	json.Deterministic(true),
	json.FormatNilSliceAsNull(true),
	json.FormatNilMapAsNull(true),
	jsontext.EscapeForHTML(true),
	jsontext.EscapeForJS(true),
)

func writeResult(cmd *cobra.Command, w io.Writer, result io.WriterTo) error {
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	switch format {
	case "json":
		buf, err := json.Marshal(result, resultJSON)
		if err != nil {
			return err
		}
		_, err = w.Write(append(buf, '\n'))
		return err
	case "text":
		_, err := result.WriteTo(w)
		return err
	default:
		return fmt.Errorf("unknown format: %s", format)
	}
}
