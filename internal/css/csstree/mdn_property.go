package csstree

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"sync"
)

// MDNProperty is the reference data mdn/data records for a CSS property.
// It is not part of upstream csstree, which only reads property syntaxes.
type MDNProperty struct {
	Syntax string
	// Initial is the initial value. For a shorthand it names the longhands
	// whose initial values it takes, joined with ", ", and Shorthand is true.
	Initial   string
	Shorthand bool
	Inherited bool
	// Status is "standard", "nonstandard", "experimental" or "obsolete".
	Status string
	// URL is the property's MDN page, or "" when mdn/data has none.
	URL string
}

// mdnPropertyJSON is an entry of mdn/data's properties.json.
type mdnPropertyJSON struct {
	Syntax    string          `json:"syntax"`
	Initial   json.RawMessage `json:"initial"`
	Inherited bool            `json:"inherited"`
	Status    string          `json:"status"`
	URL       string          `json:"mdn_url"`
}

var mdnPropertyReference = sync.OnceValues(func() (map[string]mdnPropertyJSON, error) {
	var properties map[string]mdnPropertyJSON
	err := json.Unmarshal(mdnPropertiesJSON, &properties)
	return properties, err
})

// MDNPropertyNames returns the names of the properties mdn/data records,
// sorted.
func MDNPropertyNames() []string {
	properties, err := mdnPropertyReference()
	if err != nil {
		return nil
	}
	return slices.Sorted(maps.Keys(properties))
}

// LookupMDNProperty returns the mdn/data reference for the CSS property
// name. It reports false for unknown and custom properties.
func LookupMDNProperty(name string) (MDNProperty, bool) {
	properties, err := mdnPropertyReference()
	if err != nil {
		return MDNProperty{}, false
	}
	p, ok := properties[strings.ToLower(name)]
	if !ok {
		return MDNProperty{}, false
	}
	property := MDNProperty{Syntax: p.Syntax, Inherited: p.Inherited, Status: p.Status, URL: p.URL}
	var longhands []string
	if err := json.Unmarshal(p.Initial, &property.Initial); err != nil {
		if err := json.Unmarshal(p.Initial, &longhands); err == nil {
			property.Initial = strings.Join(longhands, ", ")
			property.Shorthand = true
		}
	}
	return property, true
}
