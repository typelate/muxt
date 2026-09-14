// Package configjson says how muxt reads and writes a command's
// configuration as JSON.
//
// It is how the snapshot archives in internal/{generate,analysis,mutation}
// hold the configuration they run with, so a configuration read back is the
// one a command line produced: a field a command line left alone is null
// rather than an empty list, and a member the configuration does not
// declare is an error rather than a typo nothing reports.
//
// Patterns say nothing here: regexp.Regexp reads and writes itself as the
// text it was compiled from.
package configjson

import "encoding/json/v2"

// Options reads and writes a configuration the way muxt holds one.
func Options() json.Options {
	return json.JoinOptions(
		json.FormatNilSliceAsNull(true),
		json.FormatNilMapAsNull(true),
		json.RejectUnknownMembers(true),
	)
}
