// Ported from lib/syntax/scope/atrulePrelude.js.

package csstree

func atrulePreludeScope() *Recognizer {
	return &Recognizer{
		GetNode: defaultRecognizer,
	}
}
