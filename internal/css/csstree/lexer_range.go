package csstree

import (
	"cmp"
	"math"
	"math/big"
	"strconv"

	"github.com/typelate/muxt/internal/css/csstree/definitionsyntax"
)

// unitFactor is a unit of a group whose units convert to one another: a
// value in the unit is ratio (or float, for rad) canonical units.
type unitFactor struct {
	group string
	ratio *big.Rat // nil for rad
	float float64
}

// convertibleUnit returns the factor of a lowercase unit that converts to
// the canonical unit of its group (CSS Values 4 §6.2 absolute lengths,
// §7.1 angles, §7.2 durations, §7.3 frequencies, §7.4 resolutions).
func convertibleUnit(unit string) (unitFactor, bool) {
	rat := func(group string, num, den int64) (unitFactor, bool) {
		r := big.NewRat(num, den)
		f, _ := r.Float64()
		return unitFactor{group: group, ratio: r, float: f}, true
	}

	switch unit {
	// absolute lengths, in px: 1in = 2.54cm = 96px = 72pt = 6pc, 1cm = 40Q
	case "px":
		return rat("length", 1, 1)
	case "cm":
		return rat("length", 9600, 254)
	case "mm":
		return rat("length", 960, 254)
	case "q":
		return rat("length", 240, 254)
	case "in":
		return rat("length", 96, 1)
	case "pt":
		return rat("length", 4, 3)
	case "pc":
		return rat("length", 16, 1)
	// angles, in deg
	case "deg":
		return rat("angle", 1, 1)
	case "grad":
		return rat("angle", 9, 10)
	case "turn":
		return rat("angle", 360, 1)
	case "rad":
		return unitFactor{group: "angle", float: 180 / math.Pi}, true
	// durations, in s
	case "s":
		return rat("time", 1, 1)
	case "ms":
		return rat("time", 1, 1000)
	// frequencies, in hz
	case "hz":
		return rat("frequency", 1, 1)
	case "khz":
		return rat("frequency", 1000, 1)
	// resolutions, in dppx
	case "dppx", "x":
		return rat("resolution", 1, 1)
	case "dpi":
		return rat("resolution", 1, 96)
	case "dpcm":
		return rat("resolution", 254, 9600)
	}

	return unitFactor{}, false
}

// compareToBound returns -1, 0 or +1 as num, of the lowercase unit ("" for
// a number), is below, at or above bound, and false when the two cannot be
// compared when parsing (CSS Values 4 §5.1: "<angle [0,180deg]> indicates
// an angle between 0deg and 180deg (expressed in any unit)").
//
//   - A bound without a unit is compared with the number alone, as upstream
//     does; CSS writes 0 and ±∞ without a unit.
//   - A bound in the unit of the value, or in a unit that converts to it,
//     is compared exactly, in the canonical unit.
//   - Otherwise, as for a value in em or vw and a bound in px, the units
//     are only known at computed-value time. Every CSS unit is a
//     non-negative multiple of the canonical unit, so a value whose sign
//     differs from the bound's is on the side of the bound its sign says
//     (-1em is below 1px, 0 is below 1px); a value of the same sign cannot
//     be compared and is taken.
func compareToBound(num float64, unit string, bound *definitionsyntax.RangeBound) (int, bool) {
	boundUnit := asciiLowerCase(bound.Unit)

	if boundUnit == "" || boundUnit == unit {
		return cmp.Compare(num, bound.Value), true
	}

	if math.IsInf(num, 0) {
		return cmp.Compare(num, 0), true
	}

	numFactor, numOK := convertibleUnit(unit)
	boundFactor, boundOK := convertibleUnit(boundUnit)
	if numOK && boundOK && numFactor.group == boundFactor.group {
		if numFactor.ratio == nil || boundFactor.ratio == nil {
			return cmp.Compare(num*numFactor.float, bound.Value*boundFactor.float), true
		}
		// the decimal numbers as written, so that 2.54cm is exactly 1in
		numRat, numOK := decimalRat(num)
		boundRat, boundOK := decimalRat(bound.Value)
		if !numOK || !boundOK {
			return cmp.Compare(num*numFactor.float, bound.Value*boundFactor.float), true
		}
		return numRat.Mul(numRat, numFactor.ratio).Cmp(boundRat.Mul(boundRat, boundFactor.ratio)), true
	}

	numSign, boundSign := sign(num), sign(bound.Value)
	if numSign != boundSign {
		return cmp.Compare(numSign, boundSign), true
	}

	return 0, false
}

// decimalRat returns the exact value of the shortest decimal that reads as
// x: 2.54 for the float64 nearest to 2.54, not that float64's exact binary
// value.
func decimalRat(x float64) (*big.Rat, bool) {
	return new(big.Rat).SetString(strconv.FormatFloat(x, 'g', -1, 64))
}

func sign(x float64) int {
	switch {
	case x < 0:
		return -1
	case x > 0:
		return 1
	}
	return 0
}
