package cli

import (
	"fmt"
	"math"

	"github.com/dustin/go-humanize"

	"github.com/typelate/muxt/internal/generate"
)

// byteSize is a pflag.Value that parses a human-readable byte size, such as
// "32MB", "64MiB" or "1GB", into *p. A zero p, or a value of zero or less,
// reads as generate's default size, so a flag that was never given still shows
// the size generate would use.
type byteSize struct{ p *int64 }

func (b byteSize) String() string {
	n := int64(generate.DefaultMultipartMaxMemory)
	if b.p != nil && *b.p > 0 {
		n = *b.p
	}
	return humanize.IBytes(uint64(n))
}

func (b byteSize) Set(v string) error {
	n, err := humanize.ParseBytes(v)
	if err != nil {
		return fmt.Errorf("invalid byte size %q: %w", v, err)
	}
	if n == 0 {
		return fmt.Errorf("multipart max memory must be positive, got %q", v)
	}
	if n > math.MaxInt64 {
		return fmt.Errorf("multipart max memory %q exceeds int64 maximum", v)
	}
	*b.p = int64(n)
	return nil
}

func (byteSize) Type() string { return "bytes" }
