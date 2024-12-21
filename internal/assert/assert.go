package assert

import (
	"fmt"
	"os"
)

const pleaseCreateAnIssue = `If you get this error, please file an issue on https://github.com/crhntr/muxt/issues/new with a description of the inputs. I did not expect this to be possible.`

func Exit(description, message string, in ...any) {
	_, _ = fmt.Fprintf(os.Stderr, message, in...)
	_, _ = fmt.Fprintf(os.Stderr, "\n"+description)
	_, _ = fmt.Fprintln(os.Stderr, "\n"+pleaseCreateAnIssue)
	os.Exit(1)
}

func Len[T any](in []T, n int, description string) {
	if len(in) != n {
		Exit(description, "expected length %d got %d\n", n, len(in))
	}
}

func MaxLen[T any](in []T, n int, description string) {
	if len(in) > n {
		Exit(description, "expected length less than %d got %d\n", n, len(in))
	}
}
