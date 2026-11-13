package composite

import (
	"bytes"
	"encoding/json/jsontext"
	"fmt"
)

// checkShape walks data, which is valid JSON, and returns a *ShapeError for
// the first value in document order that is not an object where the file needs
// one. It reads tokens, not Go types, so a message never names a struct of
// this package. A null is an absent object and passes.
func checkShape(path string, data []byte) error {
	w := shapeWalker{
		path: path,
		data: data,
		dec:  jsontext.NewDecoder(bytes.NewReader(data), jsontext.AllowInvalidUTF8(true), jsontext.AllowDuplicateNames(true)),
	}
	return w.file()
}

// shapeWalker reads the tokens of data once, from the start.
type shapeWalker struct {
	path string
	data []byte
	dec  *jsontext.Decoder
}

// file walks the top-level object.
func (w *shapeWalker) file() error {
	return w.object("muxt.json", "a JSON object", func(name string) error {
		switch name {
		case "package_defaults":
			return w.object("package_defaults", "a JSON object", w.skipMember)
		case "packages":
			return w.object("packages", "an object keyed by import path", func(key string) error {
				return w.object(fmt.Sprintf("packages[%q]", key), "a JSON object", w.skipMember)
			})
		default:
			return w.skipMember(name)
		}
	})
}

// skipMember reads past the value of a member without looking at it.
func (w *shapeWalker) skipMember(string) error {
	if err := w.dec.SkipValue(); err != nil {
		return fmt.Errorf("reading %s: %w", w.path, err)
	}
	return nil
}

// object reads the next value, which must be an object or null, and calls
// member with the name of each member, before its value. member reads that
// value. where and want name the value in the error a different kind gets.
func (w *shapeWalker) object(where, want string, member func(name string) error) error {
	switch w.dec.PeekKind() {
	case '{':
	case 'n':
		return w.skipMember("")
	case 0:
		return fmt.Errorf("reading %s: %w", w.path, w.dec.SkipValue())
	default:
		line, col := position(w.data, w.valueStart()+1)
		return &ShapeError{Path: w.path, Line: line, Col: col, Where: where, Want: want}
	}
	if _, err := w.dec.ReadToken(); err != nil {
		return fmt.Errorf("reading %s: %w", w.path, err)
	}
	for w.dec.PeekKind() != '}' {
		name, err := w.dec.ReadToken()
		if err != nil {
			return fmt.Errorf("reading %s: %w", w.path, err)
		}
		if err := member(name.String()); err != nil {
			return err
		}
	}
	if _, err := w.dec.ReadToken(); err != nil {
		return fmt.Errorf("reading %s: %w", w.path, err)
	}
	return nil
}

// valueStart returns the offset of the first byte of the next value: the
// decoder is left after the previous token, before any space, comma or colon.
func (w *shapeWalker) valueStart() int64 {
	off := w.dec.InputOffset()
	for off < int64(len(w.data)) && bytes.IndexByte([]byte(" \t\r\n,:"), w.data[off]) >= 0 {
		off++
	}
	return off
}
