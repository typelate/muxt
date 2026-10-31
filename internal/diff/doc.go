// Package diff computes an anchored line diff in unified diff format.
//
// diff.go, diff_test.go and testdata are copied from the Go distribution's
// $GOROOT/src/internal/diff (Go 1.27), which the go command uses for
// gofmt -d style output. diff.go is verbatim. diff_test.go differs in one
// line: it imports golang.org/x/tools/txtar in place of internal/txtar,
// which code outside the standard library cannot import.
//
// To resync, copy the three from $(go env GOROOT)/src/internal/diff,
// change the txtar import in diff_test.go to golang.org/x/tools/txtar,
// and run go test ./internal/diff.
package diff
