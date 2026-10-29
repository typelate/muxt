// Ported from lib/__tests/fixture/ast.js.
//
// Test names are "<file> <key>" (with "#<n>" for array entries) instead of
// the JsonLocator "file:line:column (key)" names upstream builds.

package csstree

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const astFixtureDir = "testdata/ast"

// astFixtureOptions are the parse options a fixture may set.
type astFixtureOptions struct {
	Context             string `json:"context"`
	Atrule              string `json:"atrule"`
	Positions           bool   `json:"positions"`
	ParseAtrulePrelude  *bool  `json:"parseAtrulePrelude"`
	ParseRulePrelude    *bool  `json:"parseRulePrelude"`
	ParseValue          *bool  `json:"parseValue"`
	ParseCustomProperty *bool  `json:"parseCustomProperty"`
}

// astFixturePosition is upstream's test.position.
type astFixturePosition struct {
	Offset int
	Line   int
	Column int
}

// astFixtureTest is one fixture entry after upstream's processTest.
type astFixtureTest struct {
	Name     string
	Source   string
	Generate *string
	AST      any
	// OrderedAST is AST with the keys of objects in source order
	// (*orderedObject), for tests that depend on upstream's
	// Object.values order.
	OrderedAST    any
	Error         string
	Position      astFixturePosition
	Options       astFixtureOptions
	Skip          bool
	SkipRoundtrip bool
}

// ParseOptions converts the fixture options to ParseOptions; throwOnError
// mirrors the `onParseError: error => { throw error; }` that upstream adds
// for error fixtures.
func (test astFixtureTest) ParseOptions(throwOnError bool) ParseOptions {
	o := test.Options
	options := ParseOptions{
		Context:   o.Context,
		Atrule:    o.Atrule,
		Positions: o.Positions,
	}
	if o.ParseAtrulePrelude != nil {
		options.NoParseAtrulePrelude = !*o.ParseAtrulePrelude
	}
	if o.ParseRulePrelude != nil {
		options.NoParseRulePrelude = !*o.ParseRulePrelude
	}
	if o.ParseValue != nil {
		options.NoParseValue = !*o.ParseValue
	}
	if o.ParseCustomProperty != nil {
		options.ParseCustomProperty = *o.ParseCustomProperty
	}
	if throwOnError {
		options.OnParseError = func(err error, _ Node) error {
			return err
		}
	}
	return options
}

// astFixtureFile is upstream's result[filename] = { scope, tests, errors }.
type astFixtureFile struct {
	Filename string
	Scope    string
	Tests    []astFixtureTest
	Errors   []astFixtureTest
}

var astWrapper = map[string]func(ast any) any{
	"stylesheet": func(ast any) any {
		return map[string]any{
			"type":     "StyleSheet",
			"children": []any{ast},
		}
	},
	"mediaQuery": func(ast any) any {
		return map[string]any{
			"type":      "MediaQuery",
			"modifier":  nil,
			"mediaType": nil,
			"condition": ast,
		}
	},
	"rule": func(ast any) any {
		return map[string]any{
			"type": "Rule",
			"prelude": map[string]any{
				"type":     "SelectorList",
				"children": []any{},
			},
			"block": ast,
		}
	},
	"selector": func(ast any) any {
		return map[string]any{
			"type":     "Selector",
			"children": []any{ast},
		}
	},
	"value": func(ast any) any {
		return map[string]any{
			"type":     "Value",
			"children": []any{ast},
		}
	},
}

// forEachASTTest mirrors forEachTest(factory, errors).
func forEachASTTest(t *testing.T, errors bool, factory func(t *testing.T, test astFixtureTest)) {
	t.Helper()
	for _, file := range loadASTFixtures(t, nil) {
		forEachASTFileTest(t, file, errors, factory)
	}
}

// forEachSelectorASTTest mirrors forEachSelectorTest(factory, errors).
func forEachSelectorASTTest(t *testing.T, errors bool, factory func(t *testing.T, test astFixtureTest)) {
	t.Helper()
	for _, file := range loadASTFixtures(t, []string{"selector", "selectorList"}) {
		forEachASTFileTest(t, file, errors, factory)
	}
}

func forEachASTFileTest(t *testing.T, file astFixtureFile, errors bool, factory func(t *testing.T, test astFixtureTest)) {
	tests := file.Tests
	if errors {
		tests = file.Errors
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			factory(t, test)
		})
	}
}

// loadASTFixtures reads every scope directory of testdata/ast (or only the
// given scopes).
func loadASTFixtures(t *testing.T, scopes []string) []astFixtureFile {
	t.Helper()

	if scopes == nil {
		entries, err := os.ReadDir(astFixtureDir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			scopes = append(scopes, entry.Name())
		}
	}

	var result []astFixtureFile
	for _, scope := range scopes {
		result = readASTTestDir(t, filepath.Join(astFixtureDir, scope), result, scope)
	}
	return result
}

func readASTTestDir(t *testing.T, dir string, result []astFixtureFile, scope string) []astFixtureFile {
	t.Helper()

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		return result
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		filename := filepath.Join(dir, entry.Name())

		if entry.IsDir() {
			result = readASTTestDir(t, filename, result, scope)
			continue
		}

		if filepath.Ext(filename) != ".json" {
			continue
		}

		data, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}

		keys, values, err := decodeOrderedObject(data)
		if err != nil {
			t.Fatalf("%s: %v", filename, err)
		}

		file := astFixtureFile{
			Filename: filename,
			Scope:    scope,
		}
		displayName := path.Join(strings.TrimPrefix(filepath.ToSlash(dir), "testdata/ast/"), entry.Name())

		processTest := func(raw json.RawMessage, storeKey string) {
			test, isError, err := processASTTest(raw, scope)
			if err != nil {
				t.Fatalf("%s %s: %v", filename, storeKey, err)
			}
			test.Name = displayName + " " + storeKey
			if isError {
				file.Errors = append(file.Errors, test)
			} else {
				file.Tests = append(file.Tests, test)
			}
		}

		for i, key := range keys {
			value := bytes.TrimSpace(values[i])
			if len(value) > 0 && value[0] == '[' {
				var list []json.RawMessage
				if err := json.Unmarshal(value, &list); err != nil {
					t.Fatalf("%s %s: %v", filename, key, err)
				}
				for idx, raw := range list {
					processTest(raw, fmt.Sprintf("%s#%d", key, idx+1))
				}
			} else {
				processTest(value, key)
			}
		}

		result = append(result, file)
	}

	return result
}

func processASTTest(raw json.RawMessage, scope string) (astFixtureTest, bool, error) {
	var fields struct {
		Source        string            `json:"source"`
		Generate      *string           `json:"generate"`
		AST           any               `json:"ast"`
		Error         string            `json:"error"`
		Offset        any               `json:"offset"`
		Options       astFixtureOptions `json:"options"`
		Skip          any               `json:"skip"`
		SkipRoundtrip any               `json:"skipRoundtrip"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return astFixtureTest{}, false, err
	}
	var rawFields struct {
		AST json.RawMessage `json:"ast"`
	}
	if err := json.Unmarshal(raw, &rawFields); err != nil {
		return astFixtureTest{}, false, err
	}
	var orderedAST any
	if len(rawFields.AST) > 0 {
		var err error
		orderedAST, err = decodeOrdered(rawFields.AST)
		if err != nil {
			return astFixtureTest{}, false, err
		}
	}

	test := astFixtureTest{
		Source:        fields.Source,
		Generate:      fields.Generate,
		AST:           fields.AST,
		OrderedAST:    orderedAST,
		Error:         fields.Error,
		Options:       fields.Options,
		Skip:          truthy(fields.Skip),
		SkipRoundtrip: truthy(fields.SkipRoundtrip),
	}
	test.Options.Context = scope

	if test.Error != "" {
		if offsetMarker, ok := fields.Offset.(string); ok {
			offset := strings.IndexByte(offsetMarker, '^')
			// test.source.substr(0, offset).split(/\r|\r\n|\n|\f/g): the \r
			// alternative comes first, so \r\n counts as two breaks
			before := test.Source[:offset]
			line := 1
			lastBreak := -1
			for i := 0; i < len(before); i++ {
				switch before[i] {
				case '\r', '\n', '\f':
					line++
					lastBreak = i
				}
			}
			test.Position = astFixturePosition{
				Offset: offset,
				Line:   line,
				Column: len(before) - (lastBreak + 1) + 1,
			}
		}
		return test, true, nil
	}

	if ast, ok := test.AST.(map[string]any); ok {
		if typ, _ := ast["type"].(string); !strings.EqualFold(typ, scope) {
			if wrap, ok := astWrapper[scope]; ok {
				test.AST = wrap(test.AST)
				test.OrderedAST = orderedASTWrapper[scope](test.OrderedAST)
			}
		}
	}

	return test, false, nil
}

func truthy(v any) bool {
	switch v := v.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		return v != ""
	default:
		return true
	}
}

// decodeOrderedObject returns the keys and raw values of a JSON object in
// source order.
func decodeOrderedObject(data []byte) ([]string, []json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, nil, fmt.Errorf("expected an object")
	}
	var (
		keys   []string
		values []json.RawMessage
	)
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, nil, fmt.Errorf("expected a key")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, nil, err
		}
		keys = append(keys, key)
		values = append(values, value)
	}
	return keys, values, nil
}

// stripLoc removes every "loc" key, as upstream's
// JSON.stringify(ast, (key, value) => key !== 'loc' ? value : undefined).
func stripLoc(v any) any {
	switch v := v.(type) {
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, value := range v {
			if key == "loc" {
				continue
			}
			result[key] = stripLoc(value)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, value := range v {
			result[i] = stripLoc(value)
		}
		return result
	default:
		return v
	}
}

// toPlainJSON encodes v (an AST node or any JSON value) and decodes it back
// into maps and slices.
func toPlainJSON(t *testing.T, v any) any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var result any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal(%s): %v", data, err)
	}
	return result
}

// assertASTEqualNoLoc is assert.strictEqual(stringifyWithNoLoc(actual),
// stringifyWithNoLoc(expected)).
func assertASTEqualNoLoc(t *testing.T, source string, actual, expected any) {
	t.Helper()
	got := stripLoc(toPlainJSON(t, actual))
	want := stripLoc(toPlainJSON(t, expected))
	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "    ")
		wantJSON, _ := json.MarshalIndent(want, "", "    ")
		t.Errorf("Parse(%q) =\n%s\nwant\n%s", source, gotJSON, wantJSON)
	}
}

// orderedObject is a JSON object with its keys in source order.
type orderedObject struct {
	keys   []string
	values map[string]any
}

func newOrderedObject(keysAndValues ...any) *orderedObject {
	object := &orderedObject{values: make(map[string]any)}
	for i := 0; i < len(keysAndValues); i += 2 {
		key := keysAndValues[i].(string)
		object.keys = append(object.keys, key)
		object.values[key] = keysAndValues[i+1]
	}
	return object
}

// orderedASTWrapper is astWrapper building *orderedObject values with
// upstream's key order.
var orderedASTWrapper = map[string]func(ast any) any{
	"stylesheet": func(ast any) any {
		return newOrderedObject("type", "StyleSheet", "children", []any{ast})
	},
	"mediaQuery": func(ast any) any {
		return newOrderedObject("type", "MediaQuery", "modifier", nil, "mediaType", nil, "condition", ast)
	},
	"rule": func(ast any) any {
		return newOrderedObject("type", "Rule", "prelude", newOrderedObject("type", "SelectorList", "children", []any{}), "block", ast)
	},
	"selector": func(ast any) any {
		return newOrderedObject("type", "Selector", "children", []any{ast})
	},
	"value": func(ast any) any {
		return newOrderedObject("type", "Value", "children", []any{ast})
	},
}

// decodeOrdered decodes JSON like json.Unmarshal into an any, except that
// objects are *orderedObject.
func decodeOrdered(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	return decodeOrderedValue(dec)
}

func decodeOrderedValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch tok {
	case json.Delim('{'):
		object := &orderedObject{values: make(map[string]any)}
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyTok.(string)
			if !ok {
				return nil, fmt.Errorf("expected a key")
			}
			value, err := decodeOrderedValue(dec)
			if err != nil {
				return nil, err
			}
			object.keys = append(object.keys, key)
			object.values[key] = value
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return object, nil
	case json.Delim('['):
		array := []any{}
		for dec.More() {
			value, err := decodeOrderedValue(dec)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return array, nil
	default:
		return tok, nil
	}
}
