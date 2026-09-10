package mutation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// suiteDigest identifies the test suite a verdict was measured against.
//
// A verdict says "the tests caught this mutant", so it is only worth
// reusing while those tests are the same tests. Nothing else in an
// action's identity notices a test changing: the template, the types it
// reads, and the seed can all sit still while an assertion is added,
// weakened or deleted. Without this, adding a test to cover a miss left
// the miss recorded, and the next run reported the gap that the new test
// had just closed.
//
// It covers what decides which assertions run: the package patterns, the
// -run expression, and the contents of every test file the go command
// would compile for them. Paths go in relative to the working directory
// so the digest is the same on another machine.
func suiteDigest(workingDirectory string, packages []string, match *regexp.Regexp) (string, error) {
	files, err := testFiles(workingDirectory, packages)
	if err != nil {
		return "", err
	}

	h := sha256.New()
	fmt.Fprintf(h, "v%d\x00", stateVersion)
	for _, pattern := range packages {
		fmt.Fprintf(h, "pkg\x00%s\x00", pattern)
	}
	if match != nil {
		fmt.Fprintf(h, "run\x00%s\x00", match.String())
	}
	for _, file := range files {
		rel, err := filepath.Rel(workingDirectory, file)
		if err != nil {
			rel = file
		}
		body, err := os.ReadFile(file)
		if err != nil {
			// A file the go command listed and we cannot read is a
			// changed suite as far as this is concerned.
			fmt.Fprintf(h, "file\x00%s\x00unreadable\x00", filepath.ToSlash(rel))
			continue
		}
		fmt.Fprintf(h, "file\x00%s\x00%d\x00", filepath.ToSlash(rel), len(body))
		h.Write(body)
	}
	return hex.EncodeToString(h.Sum(nil))[:32], nil
}

// testFiles lists every test file the go command would compile for the
// given patterns, in a stable order.
//
// go list is asked rather than the directory walked, so the answer
// follows the same build constraints, tags and package selection the test
// run will.
func testFiles(workingDirectory string, packages []string) ([]string, error) {
	args := []string{"list", "-e", "-f", "{{.Dir}}\t{{range .TestGoFiles}}{{.}}\t{{end}}{{range .XTestGoFiles}}{{.}}\t{{end}}"}
	args = append(args, packages...)

	cmd := exec.Command("go", args...)
	cmd.Dir = workingDirectory
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing test files: %w", err)
	}

	var files []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Split(strings.TrimRight(line, "\t"), "\t")
		if len(fields) < 2 || fields[0] == "" {
			continue
		}
		for _, name := range fields[1:] {
			if name == "" {
				continue
			}
			files = append(files, filepath.Join(fields[0], name))
		}
	}
	slices.Sort(files)
	return slices.Compact(files), nil
}
