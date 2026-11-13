package composite

import "strings"

// Key returns the muxt.json key of a flag name: the name with each "-" written
// as "_". Keys use underscores so the file's spelling matches the identifiers
// a Go developer already types, and the flag names stay as the command line
// spells them.
func Key(flag string) string {
	return strings.ReplaceAll(flag, "-", "_")
}

// Kind is the JSON type a key takes.
type Kind uint8

const (
	// KindString is a JSON string.
	KindString Kind = iota + 1
	// KindBool is a JSON boolean.
	KindBool
	// KindList is a JSON array of strings.
	KindList
)

// Keys is what an entry may hold: each key's kind and each deprecated key's
// replacement. composite does not know the settings; the command that owns
// them derives Keys and hands it to Find, so one declaration per setting
// drives the file as well as the flags.
type Keys struct {
	// Kinds maps each key an entry accepts to the JSON type it takes.
	Kinds map[string]Kind
	// Deprecated maps an old key to the key that replaces it.
	Deprecated map[string]string
}
