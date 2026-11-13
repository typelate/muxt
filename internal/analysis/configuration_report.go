package analysis

import (
	"bytes"
	"encoding/json/v2"
	"fmt"
	"io"
)

// ConfigurationReport is the Configuration section of the route listing: every
// generate setting, once. It is the --format json value of the listing's
// Configuration member, an array of groups, each a Source and its Settings in
// order, each a Key as muxt.json writes it and its Value:
//
//	"Configuration": [
//		{"Source": "defaults", "Settings": [{"Key": "output_file", "Value": "template_routes.go"}]}
//	]
//
// The two groups from a muxt.json, packages["<key>"] and package_defaults, also
// have a File member holding that file's absolute path, so an editor can open
// it from any directory. Text writes that path once above both, since a reader sees the
// file first and then its entry. The groups and their settings are arrays, not
// objects, because their order is part of what the report says.
type ConfigurationReport []SettingGroup

// SettingGroup is the settings that come from one source. File is the muxt.json
// the source is in, empty for flags and defaults.
type SettingGroup struct {
	Source   string
	Settings []ReportLine
	File     string `json:",omitzero"`
}

// ReportLine is one muxt.json key and the value the run uses for it.
type ReportLine struct {
	Key   string
	Value ReportValue
}

// ReportValue is the value of one setting: a string, a boolean or a list of
// strings, the kinds a muxt.json key holds. It is a closed type so the report
// needs no any, and it writes as the JSON value of that kind.
type ReportValue struct {
	text string
	flag bool
	list []string
	kind reportKind
}

type reportKind int

const (
	textKind reportKind = iota
	flagKind
	listKind
)

// TextValue is the report value of a string setting.
func TextValue(s string) ReportValue { return ReportValue{kind: textKind, text: s} }

// FlagValue is the report value of a boolean setting.
func FlagValue(b bool) ReportValue { return ReportValue{kind: flagKind, flag: b} }

// ListValue is the report value of a list setting.
func ListValue(l []string) ReportValue { return ReportValue{kind: listKind, list: l} }

// MarshalJSON writes the value as the JSON of its kind.
func (v ReportValue) MarshalJSON() ([]byte, error) {
	switch v.kind {
	case flagKind:
		return json.Marshal(v.flag)
	case listKind:
		return json.Marshal(v.list)
	default:
		return json.Marshal(v.text)
	}
}

// WriteTo writes the section as text: one heading per source, a line per
// setting written so it can be pasted into muxt.json, and a blank line after.
// The file heading of a muxt.json's groups is written once, before the first of
// them, and the groups and their settings are indented under it.
func (r ConfigurationReport) WriteTo(w io.Writer) (int64, error) {
	var buf bytes.Buffer
	buf.WriteString("Configuration:\n")
	var file string
	for _, group := range r {
		indent := "  "
		if group.File != "" {
			if group.File != file {
				fmt.Fprintf(&buf, "  %s:\n", group.File)
				file = group.File
			}
			indent = "    "
		}
		fmt.Fprintf(&buf, "%s%s:\n", indent, group.Source)
		for _, s := range group.Settings {
			value, err := json.Marshal(s.Value)
			if err != nil {
				return 0, fmt.Errorf("writing %s: %w", s.Key, err)
			}
			fmt.Fprintf(&buf, "%s  %q: %s\n", indent, s.Key, value)
		}
	}
	buf.WriteString("\n")
	return io.Copy(w, &buf)
}
