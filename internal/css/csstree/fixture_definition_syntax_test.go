// Ported from lib/__tests/fixture/definition-syntax.js and the parts of
// lib/__tests/helpers/JsonLocator.js it uses (the definitionsyntax package
// has the same loader for its own tests; forEachTest,
// forEachAtrulePreludeTest and forEachAtruleDescriptorTest need a lexer, so
// they live here).

package csstree

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

const definitionSyntaxFixtureDir = "testdata/definition-syntax"

// definitionSyntaxLexerConfig is the `lexer` of a fixture test, passed to
// createLexer.
type definitionSyntaxLexerConfig struct {
	Generic         *bool               `json:"generic"`
	CSSWideKeywords []string            `json:"cssWideKeywords"`
	Units           map[string][]string `json:"units"`
	Types           map[string]string   `json:"types"`
	Properties      map[string]string   `json:"properties"`
}

// definitionSyntaxTest is one entry of a fixture file.
type definitionSyntaxTest struct {
	Key      string
	Name     string
	Test     string
	Syntax   string
	Property string
	Type     string
	Lexer    *definitionSyntaxLexerConfig
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

		locator, err := newJSONLocator(path.Join(definitionSyntaxFixtureDir, fn), data)
		if err != nil {
			t.Fatal(err)
		}

		var raw map[string]struct {
			Test     string                       `json:"test"`
			Syntax   *string                      `json:"syntax"`
			Property *string                      `json:"property"`
			Type     *string                      `json:"type"`
			Lexer    *definitionSyntaxLexerConfig `json:"lexer"`
			Valid    []string                     `json:"valid"`
			Invalid  []string                     `json:"invalid"`
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
				Test:    test.Test,
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

// definitionSyntaxFactory receives a fixture value: testType is "valid" or
// "invalid", testState the fixture's `test` ("skip", "only" or ""), syntax
// the definition syntax to match ("" to match property instead).
type definitionSyntaxFactory func(t *testing.T, testType, testState, name string, lexer *Lexer, property, value, syntax string)

// runDefinitionSyntaxValue runs one fixture value as a subtest named name,
// skipping it when the fixture says so (upstream it.skip).
func runDefinitionSyntaxValue(t *testing.T, testState, name string, fn func(t *testing.T)) {
	t.Run(name, func(t *testing.T) {
		if testState == "skip" {
			t.Skip("skipped by fixture")
		}
		fn(t)
	})
}

// forEachDefinitionSyntaxTest mirrors forEachTest.
func forEachDefinitionSyntaxTest(t *testing.T, factory definitionSyntaxFactory) {
	t.Helper()

	for _, file := range loadDefinitionSyntaxTests(t) {
		for _, test := range file.Tests {
			syntax := test.Syntax
			lexer := DefaultLexer()
			if test.Lexer != nil {
				var err error
				lexer, err = CreateLexer(&Config{
					Generic:         test.Lexer.Generic,
					CSSWideKeywords: test.Lexer.CSSWideKeywords,
					Units:           test.Lexer.Units,
					Types:           test.Lexer.Types,
					Properties:      test.Lexer.Properties,
				})
				if err != nil {
					t.Fatalf("%s: CreateLexer() error = %v", test.Name, err)
				}
			}

			for _, field := range []struct {
				name   string
				values []string
			}{
				{"valid", test.Valid},
				{"invalid", test.Invalid},
			} {
				for idx, value := range field.values {
					factory(
						t,
						field.name,
						test.Test,
						fmt.Sprintf("%s %s#%d", test.Name, field.name, idx),
						lexer,
						test.Property,
						value,
						syntax,
					)
				}
			}
		}
	}
}

// atruleTest is an entry of atrules.json.
type atruleTest struct {
	Name        string
	Test        string
	Prelude     *atruleTestValues
	Descriptors map[string]atruleTestValues
}

type atruleTestValues struct {
	Valid   []string `json:"valid"`
	Invalid []string `json:"invalid"`
}

// loadAtruleTests returns atrules.json's tests in key order (upstream
// atruleTests).
func loadAtruleTests(t *testing.T) ([]string, map[string]atruleTest) {
	t.Helper()

	filename := filepath.Join(definitionSyntaxFixtureDir, "atrules.json")
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	locator, err := newJSONLocator(path.Join(definitionSyntaxFixtureDir, "atrules.json"), data)
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]struct {
		Test        string                      `json:"test"`
		Prelude     *atruleTestValues           `json:"prelude"`
		Descriptors map[string]atruleTestValues `json:"descriptors"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("%s: %v", filename, err)
	}

	tests := make(map[string]atruleTest)
	for _, key := range locator.keys {
		name, err := locator.get(key)
		if err != nil {
			t.Fatal(err)
		}
		tests[key] = atruleTest{
			Name:        name,
			Test:        raw[key].Test,
			Prelude:     raw[key].Prelude,
			Descriptors: raw[key].Descriptors,
		}
	}

	return locator.keys, tests
}

// forEachAtrulePreludeTest mirrors forEachAtrulePreludeTest.
func forEachAtrulePreludeTest(t *testing.T, factory func(t *testing.T, testType, testState, name string, lexer *Lexer, atruleName, value string)) {
	t.Helper()

	atruleNames, atruleTests := loadAtruleTests(t)
	for _, atruleName := range atruleNames {
		testset := atruleTests[atruleName]
		if testset.Prelude == nil {
			continue
		}

		test := testset.Prelude
		for _, field := range []struct {
			name   string
			values []string
		}{
			{"valid", test.Valid},
			{"invalid", test.Invalid},
		} {
			for idx, value := range field.values {
				factory(
					t,
					field.name,
					testset.Test,
					fmt.Sprintf("%s @%s prelude %s#%d", testset.Name, atruleName, field.name, idx),
					DefaultLexer(),
					atruleName,
					value,
				)
			}
		}
	}
}

// forEachAtruleDescriptorTest mirrors forEachAtruleDescriptorTest;
// descriptors are visited in name order.
func forEachAtruleDescriptorTest(t *testing.T, factory func(t *testing.T, testType, testState, name string, lexer *Lexer, atruleName, descriptorName, value string)) {
	t.Helper()

	atruleNames, atruleTests := loadAtruleTests(t)
	for _, atruleName := range atruleNames {
		testset := atruleTests[atruleName]
		if testset.Descriptors == nil {
			continue
		}

		for _, descriptorName := range sortedKeys(testset.Descriptors) {
			test := testset.Descriptors[descriptorName]
			for _, field := range []struct {
				name   string
				values []string
			}{
				{"valid", test.Valid},
				{"invalid", test.Invalid},
			} {
				for idx, value := range field.values {
					factory(
						t,
						field.name,
						testset.Test,
						fmt.Sprintf("%s @%s descriptor %s %s#%d", testset.Name, atruleName, descriptorName, field.name, idx),
						DefaultLexer(),
						atruleName,
						descriptorName,
						value,
					)
				}
			}
		}
	}
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
