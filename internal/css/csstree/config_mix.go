// Ported from lib/syntax/config/mix.js.

package csstree

import (
	"maps"
	"slices"
	"strings"
)

// jsWhiteSpace is the ASCII part of the JavaScript regexp class \s.
const jsWhiteSpace = " \t\n\v\f\r"

// appendOrSet returns a + b when b starts with "|" (after whitespace) and
// a is set, b without that "|" when a is not set, and b otherwise. "" is
// upstream null.
func appendOrSet(a, b string) string {
	if trimmed := strings.TrimLeft(b, jsWhiteSpace); strings.HasPrefix(trimmed, "|") {
		if a != "" {
			return a + b
		}
		return strings.TrimLeft(trimmed[1:], jsWhiteSpace)
	}

	return b
}

// mergeRecognizer is upstream's { ...base, ...ext } for a scope: fields of
// ext that are set replace those of base, and Functions are merged by
// name.
func mergeRecognizer(base, ext *Recognizer) *Recognizer {
	result := new(Recognizer)
	if base != nil {
		*result = *base
		result.Functions = maps.Clone(base.Functions)
	}

	if ext.GetNode != nil {
		result.GetNode = ext.GetNode
	}
	if ext.OnWhiteSpace != nil {
		result.OnWhiteSpace = ext.OnWhiteSpace
	}
	if len(ext.Functions) > 0 {
		if result.Functions == nil {
			result.Functions = make(map[string]func(p *Parser, recognizer *Recognizer) *List[Node])
		}
		maps.Copy(result.Functions, ext.Functions)
	}

	return result
}

// mergeNodeConfig is upstream's { ...base, ...extractProps(ext, ['name',
// 'structure', 'parse', 'generate', 'walkContext']) }: fields of ext that
// are set replace those of base. The Go-only Walk, Clone and
// FromPlainObject are merged the same way.
func mergeNodeConfig(base, ext NodeConfig) NodeConfig {
	result := base

	if ext.Name != "" {
		result.Name = ext.Name
	}
	if ext.Structure != nil {
		result.Structure = ext.Structure
	}
	if ext.Generate != nil {
		result.Generate = ext.Generate
	}
	if ext.WalkContext != "" {
		result.WalkContext = ext.WalkContext
	}
	if ext.Walk != nil {
		result.Walk = ext.Walk
	}
	if ext.Clone != nil {
		result.Clone = ext.Clone
	}
	if ext.FromPlainObject != nil {
		result.FromPlainObject = ext.FromPlainObject
	}

	return result
}

// mix returns a new config: dest with the keys set in src mixed in.
// Upstream copies src keys that are present; here a key is present when
// its field is not the zero value (nil map, nil slice, nil Generic).
func mix(dest, src *Config) *Config {
	if dest == nil {
		dest = new(Config)
	}
	if src == nil {
		src = new(Config)
	}
	result := *dest

	if src.Generic != nil {
		result.Generic = new(*src.Generic)
	}

	if src.CSSWideKeywords != nil {
		if dest.CSSWideKeywords != nil {
			result.CSSWideKeywords = slices.Concat(dest.CSSWideKeywords, src.CSSWideKeywords)
		} else {
			result.CSSWideKeywords = slices.Clone(src.CSSWideKeywords)
		}
	}

	if src.Units != nil {
		result.Units = maps.Clone(dest.Units)
		if result.Units == nil {
			result.Units = make(map[string][]string)
		}
		for name, patch := range src.Units {
			if patch == nil {
				patch = []string{}
			}
			result.Units[name] = patch
		}
	}

	if src.Atrules != nil {
		result.Atrules = maps.Clone(dest.Atrules)
		if result.Atrules == nil {
			result.Atrules = make(map[string]AtruleDefinition)
		}

		for name, atrule := range src.Atrules {
			exists := result.Atrules[name]
			current := AtruleDefinition{
				Prelude:     exists.Prelude,
				Descriptors: maps.Clone(exists.Descriptors),
			}
			if current.Descriptors == nil {
				current.Descriptors = make(map[string]string)
			}

			if atrule.Prelude != "" {
				current.Prelude = appendOrSet(current.Prelude, atrule.Prelude)
			}

			for descriptorName, descriptorValue := range atrule.Descriptors {
				if descriptorValue != "" {
					current.Descriptors[descriptorName] = appendOrSet(current.Descriptors[descriptorName], descriptorValue)
				} else {
					current.Descriptors[descriptorName] = ""
				}
			}

			if len(current.Descriptors) == 0 {
				current.Descriptors = nil
			}

			result.Atrules[name] = current
		}
	}

	if src.Types != nil {
		result.Types = mixSyntaxes(dest.Types, src.Types)
	}

	if src.Properties != nil {
		result.Properties = mixSyntaxes(dest.Properties, src.Properties)
	}

	if src.ParseContext != nil {
		result.ParseContext = maps.Clone(dest.ParseContext)
		if result.ParseContext == nil {
			result.ParseContext = make(map[string]func(p *Parser, options ParseOptions) Node)
		}
		maps.Copy(result.ParseContext, src.ParseContext)
	}

	if src.Scope != nil {
		result.Scope = maps.Clone(dest.Scope)
		if result.Scope == nil {
			result.Scope = make(map[string]*Recognizer)
		}
		for key, props := range src.Scope {
			if props == nil {
				props = new(Recognizer)
			}
			result.Scope[key] = mergeRecognizer(result.Scope[key], props)
		}
	}

	if src.Features != nil {
		result.Features = maps.Clone(dest.Features)
		if result.Features == nil {
			result.Features = make(map[string]map[string]func(p *Parser) Node)
		}
		for key, props := range src.Features {
			merged := maps.Clone(result.Features[key])
			if merged == nil {
				merged = make(map[string]func(p *Parser) Node)
			}
			maps.Copy(merged, props)
			result.Features[key] = merged
		}
	}

	if src.Atrule != nil {
		result.Atrule = maps.Clone(dest.Atrule)
		if result.Atrule == nil {
			result.Atrule = make(map[string]AtruleParser)
		}
		maps.Copy(result.Atrule, src.Atrule)
	}

	if src.Pseudo != nil {
		result.Pseudo = maps.Clone(dest.Pseudo)
		if result.Pseudo == nil {
			result.Pseudo = make(map[string]func(p *Parser) *List[Node])
		}
		maps.Copy(result.Pseudo, src.Pseudo)
	}

	if src.Node != nil {
		result.Node = maps.Clone(dest.Node)
		if result.Node == nil {
			result.Node = make(map[string]NodeConfig)
		}
		for key, props := range src.Node {
			result.Node[key] = mergeNodeConfig(result.Node[key], props)
		}
	}

	return &result
}

// mixSyntaxes is the types and properties case of mix.
func mixSyntaxes(dest, src map[string]string) map[string]string {
	result := maps.Clone(dest)
	if result == nil {
		result = make(map[string]string)
	}
	for name, syntax := range src {
		result[name] = appendOrSet(result[name], syntax)
	}
	return result
}
