package asteval

import (
	"fmt"
	"go/types"
	"strconv"
)

type integerKind struct {
	bitSize int
	signed  bool
}

var integerKinds = map[string]integerKind{
	"int":    {bitSize: strconv.IntSize, signed: true},
	"int8":   {bitSize: 8, signed: true},
	"int16":  {bitSize: 16, signed: true},
	"int32":  {bitSize: 32, signed: true},
	"int64":  {bitSize: 64, signed: true},
	"uint":   {bitSize: strconv.IntSize},
	"uint8":  {bitSize: 8},
	"uint16": {bitSize: 16},
	"uint32": {bitSize: 32},
	"uint64": {bitSize: 64},
}

// CheckParses reports whether val, in base 10, fits the integer type that
// tp's underlying type is. Any other type is an error.
func CheckParses(val string, tp types.Type) error {
	kind, ok := integerKinds[tp.Underlying().String()]
	if !ok {
		return fmt.Errorf("type %s unknown", tp.String())
	}
	var err error
	if kind.signed {
		_, err = strconv.ParseInt(val, 10, kind.bitSize)
	} else {
		_, err = strconv.ParseUint(val, 10, kind.bitSize)
	}
	return err
}
