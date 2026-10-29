// Ported from lib/data.js and lib/data-patch.js.

package csstree

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

var (
	//go:embed data/mdn/at-rules.json
	mdnAtrulesJSON []byte
	//go:embed data/mdn/properties.json
	mdnPropertiesJSON []byte
	//go:embed data/mdn/syntaxes.json
	mdnSyntaxesJSON []byte
	//go:embed data/patch.json
	patchJSON []byte
)

var (
	extendSyntax = regexp.MustCompile(`^\s*\|\s*`)

	atruleBlock   = regexp.MustCompile(`\{(?:.|\s)+\}`)
	atrulePrelude = regexp.MustCompile(`^@\S+\s+([^;\{]*)`)
)

// definitions holds the syntaxes of the default lexer (upstream data.js's
// default export).
type definitions struct {
	Types      map[string]string
	Atrules    map[string]AtruleDefinition
	Properties map[string]string
}

// mdnSyntax is an entry of mdn-data's properties.json or syntaxes.json, or
// of an at-rule's descriptors; only the syntax is used.
type mdnSyntax struct {
	Syntax string `json:"syntax"`
}

type mdnAtrule struct {
	Syntax      string               `json:"syntax"`
	Descriptors map[string]mdnSyntax `json:"descriptors"`
}

// patchEntry is an entry of patch.json's types or properties, or a
// descriptor of an at-rule patch after preprocessPatchAtrulesDescritors.
type patchEntry struct {
	Syntax string `json:"syntax"`
}

// atrulePatch is an entry of patch.json's atrules. Prelude is nil when
// the key is absent and "null" when it is null.
type atrulePatch struct {
	Prelude     json.RawMessage            `json:"prelude"`
	Descriptors map[string]json.RawMessage `json:"descriptors"`
}

// patch is patch.json (upstream data-patch.js).
type patch struct {
	Atrules    map[string]*atrulePatch `json:"atrules"`
	Properties map[string]patchEntry   `json:"properties"`
	Types      map[string]patchEntry   `json:"types"`
}

// atruleEntry is an at-rule after preprocessAtrules: an empty prelude is
// upstream null, and so is a nil descriptors map.
type atruleEntry struct {
	prelude     string
	descriptors map[string]string
}

func preprocessAtrules(dict map[string]mdnAtrule) (map[string]atruleEntry, error) {
	result := make(map[string]atruleEntry)

	for atruleName, atrule := range dict {
		var descriptors map[string]string

		if atrule.Descriptors != nil {
			descriptors = make(map[string]string)

			for name, descriptor := range atrule.Descriptors {
				descriptors[name] = descriptor.Syntax
			}
		}

		match := atrulePrelude.FindStringSubmatch(replaceFirst(atruleBlock, strings.TrimSpace(atrule.Syntax), ""))
		if match == nil {
			// upstream: reading [1] of null throws a TypeError
			return nil, fmt.Errorf("at-rule %s: no prelude in syntax %q", atruleName, atrule.Syntax)
		}

		result[atruleName[1:]] = atruleEntry{
			prelude:     strings.TrimSpace(match[1]),
			descriptors: descriptors,
		}
	}

	return result, nil
}

// replaceFirst is JavaScript's str.replace(re, replacement) for a regexp
// without the g flag: only the first match is replaced.
func replaceFirst(re *regexp.Regexp, str, replacement string) string {
	loc := re.FindStringIndex(str)
	if loc == nil {
		return str
	}
	return str[:loc[0]] + replacement + str[loc[1]:]
}

func patchDictionary(dict map[string]string, patchDict map[string]patchEntry) map[string]string {
	result := make(map[string]string)

	// copy all syntaxes for an original dict
	for key, value := range dict {
		if value != "" {
			result[key] = value
		}
	}

	// apply a patch
	for key, entry := range patchDict {
		if _, ok := dict[key]; ok {
			if entry.Syntax != "" {
				if extendSyntax.MatchString(entry.Syntax) {
					result[key] = result[key] + " " + strings.TrimSpace(entry.Syntax)
				} else {
					result[key] = entry.Syntax
				}
			} else {
				delete(result, key)
			}
		} else {
			if entry.Syntax != "" {
				result[key] = replaceFirst(extendSyntax, entry.Syntax, "")
			}
		}
	}

	return result
}

func preprocessPatchAtrulesDescritors(declarations map[string]json.RawMessage) (map[string]patchEntry, error) {
	result := make(map[string]patchEntry)

	for key, value := range declarations {
		var syntax string
		if err := json.Unmarshal(value, &syntax); err == nil {
			result[key] = patchEntry{Syntax: syntax}
			continue
		}

		var entry patchEntry
		if err := json.Unmarshal(value, &entry); err != nil {
			return nil, fmt.Errorf("descriptor %s: %w", key, err)
		}
		result[key] = entry
	}

	return result, nil
}

// patchPrelude reads an at-rule patch prelude; null is "".
func patchPrelude(raw json.RawMessage) (string, error) {
	var prelude *string
	if err := json.Unmarshal(raw, &prelude); err != nil {
		return "", err
	}
	if prelude == nil {
		return "", nil
	}
	return *prelude, nil
}

func patchAtrules(dict map[string]atruleEntry, patchDict map[string]*atrulePatch) (map[string]AtruleDefinition, error) {
	result := make(map[string]AtruleDefinition)

	// copy all syntaxes for an original dict
	for key, entry := range dict {
		entryPatch, inPatch := patchDict[key]
		if inPatch && entryPatch == nil {
			continue
		}

		if entryPatch == nil {
			entryPatch = &atrulePatch{}
		}

		prelude := entry.prelude
		if inPatch && entryPatch.Prelude != nil {
			var err error
			prelude, err = patchPrelude(entryPatch.Prelude)
			if err != nil {
				return nil, fmt.Errorf("at-rule %s: %w", key, err)
			}
		}

		descriptors, err := preprocessPatchAtrulesDescritors(entryPatch.Descriptors)
		if err != nil {
			return nil, fmt.Errorf("at-rule %s: %w", key, err)
		}

		result[key] = AtruleDefinition{
			Prelude:     prelude,
			Descriptors: patchDictionary(entry.descriptors, descriptors),
		}
	}

	// apply a patch
	for key, atrulePatch := range patchDict {
		if _, ok := dict[key]; atrulePatch != nil && !ok {
			var prelude string
			if atrulePatch.Prelude != nil {
				var err error
				prelude, err = patchPrelude(atrulePatch.Prelude)
				if err != nil {
					return nil, fmt.Errorf("at-rule %s: %w", key, err)
				}
			}

			var descriptors map[string]string
			if atrulePatch.Descriptors != nil {
				patchDescriptors, err := preprocessPatchAtrulesDescritors(atrulePatch.Descriptors)
				if err != nil {
					return nil, fmt.Errorf("at-rule %s: %w", key, err)
				}
				descriptors = patchDictionary(map[string]string{}, patchDescriptors)
			}

			result[key] = AtruleDefinition{
				Prelude:     prelude,
				Descriptors: descriptors,
			}
		}
	}

	return result, nil
}

// syntaxesOf returns the syntax of each entry of an mdn-data dictionary
// (upstream reads value.syntax in patchDictionary).
func syntaxesOf(dict map[string]mdnSyntax) map[string]string {
	result := make(map[string]string, len(dict))
	for key, value := range dict {
		result[key] = value.Syntax
	}
	return result
}

// loadDefinitions decodes the embedded mdn-data and patch.json and applies
// the patch (upstream data.js, evaluated on import).
func loadDefinitions() (*definitions, error) {
	var mdnAtrules map[string]mdnAtrule
	if err := json.Unmarshal(mdnAtrulesJSON, &mdnAtrules); err != nil {
		return nil, fmt.Errorf("at-rules.json: %w", err)
	}
	var mdnProperties map[string]mdnSyntax
	if err := json.Unmarshal(mdnPropertiesJSON, &mdnProperties); err != nil {
		return nil, fmt.Errorf("properties.json: %w", err)
	}
	var mdnSyntaxes map[string]mdnSyntax
	if err := json.Unmarshal(mdnSyntaxesJSON, &mdnSyntaxes); err != nil {
		return nil, fmt.Errorf("syntaxes.json: %w", err)
	}
	var patch patch
	if err := json.Unmarshal(patchJSON, &patch); err != nil {
		return nil, fmt.Errorf("patch.json: %w", err)
	}

	atrules, err := preprocessAtrules(mdnAtrules)
	if err != nil {
		return nil, err
	}
	patchedAtrules, err := patchAtrules(atrules, patch.Atrules)
	if err != nil {
		return nil, err
	}

	return &definitions{
		Types:      patchDictionary(syntaxesOf(mdnSyntaxes), patch.Types),
		Atrules:    patchedAtrules,
		Properties: patchDictionary(syntaxesOf(mdnProperties), patch.Properties),
	}, nil
}

// defaultDefinitions returns the definitions, loading them on first use.
// The result is shared: callers must not change it.
var defaultDefinitions = sync.OnceValues(loadDefinitions)
