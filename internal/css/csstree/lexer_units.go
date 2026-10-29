// Ported from lib/lexer/units.js.

package csstree

// unitGroupNames lists the unit groups in the order upstream's units
// module exports them (the key order of upstream's lexer.units).
func unitGroupNames() []string {
	return []string{
		"length",
		"angle",
		"time",
		"frequency",
		"resolution",
		"flex",
		"decibel",
		"semitones",
	}
}

// defaultUnits returns the units of each unit group (upstream
// `import * as units from './units.js'`).
func defaultUnits() map[string][]string {
	return map[string][]string{
		"length": {
			// absolute length units https://www.w3.org/TR/css-values-3/#lengths
			"cm", "mm", "q", "in", "pt", "pc", "px",
			// font-relative length units https://drafts.csswg.org/css-values-4/#font-relative-lengths
			"em", "rem",
			"ex", "rex",
			"cap", "rcap",
			"ch", "rch",
			"ic", "ric",
			"lh", "rlh",
			// viewport-percentage lengths https://drafts.csswg.org/css-values-4/#viewport-relative-lengths
			"vw", "svw", "lvw", "dvw",
			"vh", "svh", "lvh", "dvh",
			"vi", "svi", "lvi", "dvi",
			"vb", "svb", "lvb", "dvb",
			"vmin", "svmin", "lvmin", "dvmin",
			"vmax", "svmax", "lvmax", "dvmax",
			// container relative lengths https://drafts.csswg.org/css-contain-3/#container-lengths
			"cqw", "cqh", "cqi", "cqb", "cqmin", "cqmax",
		},
		"angle":      {"deg", "grad", "rad", "turn"}, // https://www.w3.org/TR/css-values-3/#angles
		"time":       {"s", "ms"},                    // https://www.w3.org/TR/css-values-3/#time
		"frequency":  {"hz", "khz"},                  // https://www.w3.org/TR/css-values-3/#frequency
		"resolution": {"dpi", "dpcm", "dppx", "x"},   // https://www.w3.org/TR/css-values-3/#resolution
		"flex":       {"fr"},                         // https://drafts.csswg.org/css-grid/#fr-unit
		"decibel":    {"db"},                         // https://www.w3.org/TR/css3-speech/#mixing-props-voice-volume
		"semitones":  {"st"},                         // https://www.w3.org/TR/css3-speech/#voice-props-voice-pitch
	}
}
