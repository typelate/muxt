// Ported from lib/tokenizer/adopt-buffer.js.

package tokenizer

const minSize = 16 * 1024

// adoptBuffer returns buffer when it holds at least size elements, otherwise
// a new zeroed buffer with some headroom, so token and line tables can be
// reused across sources.
func adoptBuffer[T uint64 | int](buffer []T, size int) []T {
	if buffer == nil || len(buffer) < size {
		return make([]T, max(size+1024, minSize))
	}

	return buffer
}
