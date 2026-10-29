// Ported from lib/__tests/fixture/definition-syntax.js and the parts of
// lib/__tests/helpers/JsonLocator.js it uses.
//
// forEachTest, forEachAtrulePreludeTest and forEachAtruleDescriptorTest need a
// lexer; they belong with the lexer tests in the root package.

package definitionsyntax_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"testing"
)

const definitionSyntaxFixtureDir = "../testdata/definition-syntax"

// definitionSyntaxTest is one entry of a fixture file.
type definitionSyntaxTest struct {
	Key      string
	Name     string
	Syntax   string
	Property string
	Type     string
	Lexer    json.RawMessage
	Valid    []string
	Invalid  []string
}

type definitionSyntaxTestFile struct {
	Filename string
	Tests    []definitionSyntaxTest
}

// loadDefinitionSyntaxTests mirrors the `tests` export upstream: every fixture
// file except atrules.json, in directory order, with tests in key order.
func loadDefinitionSyntaxTests(t *testing.T) []definitionSyntaxTestFile {
	t.Helper()

	entries, err := os.ReadDir(definitionSyntaxFixtureDir)
	if err != nil {
		t.Fatal(err)
	}

	var result []definitionSyntaxTestFile
	for _, entry := range entries {
		fn := entry.Name()
		if filepath.Ext(fn) != ".json" || fn == "atrules.json" {
			continue
		}

		filename := filepath.Join(definitionSyntaxFixtureDir, fn)
		data, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}

		// named as upstream names it, relative to the module root
		locator, err := newJSONLocator(path.Join("testdata", "definition-syntax", fn), data)
		if err != nil {
			t.Fatal(err)
		}

		var raw map[string]struct {
			Syntax   *string         `json:"syntax"`
			Property *string         `json:"property"`
			Type     *string         `json:"type"`
			Lexer    json.RawMessage `json:"lexer"`
			Valid    []string        `json:"valid"`
			Invalid  []string        `json:"invalid"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatalf("%s: %v", filename, err)
		}

		file := definitionSyntaxTestFile{Filename: filename}
		for _, key := range locator.keys {
			test := raw[key]
			name, err := locator.get(key)
			if err != nil {
				t.Fatal(err)
			}

			item := definitionSyntaxTest{
				Key:     key,
				Name:    name,
				Lexer:   test.Lexer,
				Valid:   test.Valid,
				Invalid: test.Invalid,
			}
			if test.Syntax != nil {
				item.Syntax = *test.Syntax
			}
			if test.Property != nil {
				item.Property = *test.Property
			}
			if test.Type != nil {
				item.Type = *test.Type
			}

			if item.Syntax == "" && item.Property == "" && item.Type == "" {
				item.Syntax = key
			}

			file.Tests = append(file.Tests, item)
		}

		result = append(result, file)
	}

	return result
}

// jsonLocator mirrors JsonLocator: it remembers where each top-level key of a
// JSON object starts and rejects duplicate keys at any depth.
type jsonLocator struct {
	filename string
	data     []byte
	keys     []string
	offsets  map[string]int
}

func newJSONLocator(filename string, data []byte) (*jsonLocator, error) {
	locator := &jsonLocator{
		filename: filename,
		data:     data,
		offsets:  make(map[string]int),
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}
	if tok != json.Delim('{') {
		return locator, nil
	}

	for dec.More() {
		start := locator.nextTokenStart(int(dec.InputOffset()))
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filename, err)
		}
		key, _ := tok.(string)
		if _, ok := locator.offsets[key]; ok {
			return nil, fmt.Errorf("duplicate key `%s` at %s", key, locator.getLocation(start))
		}
		locator.offsets[key] = start
		locator.keys = append(locator.keys, key)

		if err := locator.checkForDuplicateKeys(dec); err != nil {
			return nil, err
		}
	}

	return locator, nil
}

// nextTokenStart skips whitespace and separators the decoder has not consumed yet.
func (l *jsonLocator) nextTokenStart(offset int) int {
	for offset < len(l.data) {
		switch l.data[offset] {
		case ' ', '\t', '\n', '\r', ',', ':':
			offset++
		default:
			return offset
		}
	}
	return offset
}

// checkForDuplicateKeys consumes one JSON value from dec.
func (l *jsonLocator) checkForDuplicateKeys(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("%s: %w", l.filename, err)
	}

	switch tok {
	case json.Delim('{'):
		var keys []string
		for dec.More() {
			start := l.nextTokenStart(int(dec.InputOffset()))
			tok, err := dec.Token()
			if err != nil {
				return fmt.Errorf("%s: %w", l.filename, err)
			}
			key, _ := tok.(string)
			if slices.Contains(keys, key) {
				return fmt.Errorf("duplicate key `%s` at %s", key, l.getLocation(start))
			}
			keys = append(keys, key)
			if err := l.checkForDuplicateKeys(dec); err != nil {
				return err
			}
		}
		return l.closeDelim(dec)
	case json.Delim('['):
		for dec.More() {
			if err := l.checkForDuplicateKeys(dec); err != nil {
				return err
			}
		}
		return l.closeDelim(dec)
	}

	return nil
}

func (l *jsonLocator) closeDelim(dec *json.Decoder) error {
	if _, err := dec.Token(); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("%s: %w", l.filename, err)
	}
	return nil
}

func (l *jsonLocator) getLocation(offset int) string {
	line := 1 + bytes.Count(l.data[:offset], []byte("\n"))
	column := 1 + offset - (bytes.LastIndexByte(l.data[:offset], '\n') + 1)
	return fmt.Sprintf("%s:%d:%d", l.filename, line, column)
}

func (l *jsonLocator) get(name string) (string, error) {
	offset, ok := l.offsets[name]
	if !ok {
		return "", fmt.Errorf("key `%s` not found in %s", name, l.filename)
	}
	return l.getLocation(offset) + " (" + name + ")", nil
}
