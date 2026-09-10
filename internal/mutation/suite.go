package mutation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// TestSuite is what a run's verdicts were measured against.
//
// A verdict says the tests caught a mutant, so it is only an answer while
// those tests are the same tests. Recording the suite test by test is
// what lets the next run tell an added test from an edited one: an added
// test can only turn a miss into a kill, so every kill still stands.
type TestSuite struct {
	// Packages is keyed by import path.
	Packages map[string]TestPackage `json:"packages,omitempty"`
}

// TestPackage is one package's tests, and everything they are built on.
type TestPackage struct {
	// Support digests everything in the package's test files that is not
	// a test function: imports, helpers, fixtures, package level state.
	// A test's own digest cannot see a helper it calls change, so a
	// change here unsettles every test in the package.
	Support string `json:"support"`

	// Tests maps each test function's name to a digest of its source.
	Tests map[string]string `json:"tests,omitempty"`
}

// suiteDelta is what changed between the suite a verdict was reached
// under and the one about to run.
type suiteDelta struct {
	// unsettled names tests that were edited or removed. A kill those
	// tests reached is no longer evidence of anything.
	unsettled map[string]bool

	// grew reports that some test was added or edited, so a miss might
	// not be a miss any more. A deletion alone cannot catch a mutant
	// nothing was catching.
	grew bool
}

// qualify names one test within the suite.
func qualify(pkg, test string) string { return pkg + "." + test }

// delta compares the suite that produced a state file with this one.
func (s TestSuite) delta(previous TestSuite) suiteDelta {
	d := suiteDelta{unsettled: make(map[string]bool)}

	for path, was := range previous.Packages {
		now, stillThere := s.Packages[path]
		if !stillThere {
			// Every test it held is gone, so any kill naming one has to
			// be reached again.
			for test := range was.Tests {
				d.unsettled[qualify(path, test)] = true
			}
			continue
		}
		if was.Support != now.Support {
			// A helper or fixture changed, and a test's own digest
			// cannot see that, so none of them can be trusted.
			for test := range was.Tests {
				d.unsettled[qualify(path, test)] = true
			}
			d.grew = true
		}
		for test, digest := range was.Tests {
			switch current, kept := now.Tests[test]; {
			case !kept:
				d.unsettled[qualify(path, test)] = true
			case current != digest:
				d.unsettled[qualify(path, test)] = true
				d.grew = true
			}
		}
		for test := range now.Tests {
			if _, existed := was.Tests[test]; !existed {
				d.grew = true
			}
		}
	}

	for path := range s.Packages {
		if _, existed := previous.Packages[path]; !existed {
			d.grew = true
		}
	}
	return d
}

// reusable reports whether a recorded verdict still answers for this run.
//
// A kill needs one failing test, so it stands while the test that reached
// it is untouched, however much the rest of the suite moved. A miss needs
// the whole suite to pass, so anything added or edited could have closed
// it. A skip was decided before any test ran.
func (d suiteDelta) reusable(result StateResult) bool {
	switch result.Status {
	case StatusKilled:
		if len(result.Killers) == 0 {
			// Nothing recorded which test caught it, so nothing says it
			// still would.
			return !d.grew && len(d.unsettled) == 0
		}
		for _, killer := range result.Killers {
			if d.unsettled[killer] {
				return false
			}
		}
		return true
	case StatusMissed:
		return !d.grew
	default:
		return true
	}
}

// readTestSuite records every test the go command would compile for the
// given patterns.
//
// go list is asked rather than the directory walked, so the answer
// follows the same build constraints, tags and package selection the test
// run will.
func readTestSuite(workingDirectory string, packages []string) (TestSuite, error) {
	listed, err := listTestPackages(workingDirectory, packages)
	if err != nil {
		return TestSuite{}, err
	}

	suite := TestSuite{Packages: make(map[string]TestPackage, len(listed))}
	for _, pkg := range listed {
		files := slices.Concat(pkg.TestGoFiles, pkg.XTestGoFiles)
		slices.Sort(files)

		support := sha256.New()
		tests := make(map[string]string)
		for _, name := range files {
			if err := digestTestFile(filepath.Join(pkg.Dir, name), name, support, tests); err != nil {
				return TestSuite{}, err
			}
		}
		if len(files) == 0 {
			continue
		}
		suite.Packages[pkg.ImportPath] = TestPackage{
			Support: hex.EncodeToString(support.Sum(nil))[:32],
			Tests:   tests,
		}
	}
	return suite, nil
}

// digestTestFile splits one test file into the tests it declares and the
// support everything else provides.
func digestTestFile(path, name string, support io.Writer, tests map[string]string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading test file: %w", err)
	}
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, src, parser.SkipObjectResolution)
	if err != nil {
		// A test file that does not parse cannot be split up, so all of
		// it counts as support: any test in the package is unsettled
		// until it does.
		fmt.Fprintf(support, "unparsed\x00%s\x00", name)
		support.Write(src)
		return nil
	}

	fmt.Fprintf(support, "file\x00%s\x00package\x00%s\x00", name, file.Name.Name)
	for _, decl := range file.Decls {
		from := fileSet.Position(decl.Pos()).Offset
		to := fileSet.Position(decl.End()).Offset
		if from < 0 || to > len(src) || from >= to {
			continue
		}
		body := src[from:to]

		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || !isTestFunction(fn) {
			support.Write(body)
			continue
		}
		h := sha256.New()
		h.Write(body)
		tests[fn.Name.Name] = hex.EncodeToString(h.Sum(nil))[:32]
	}
	return nil
}

// isTestFunction reports whether a declaration is a test the go command
// would run: func TestXxx(*testing.T), with no receiver.
//
// Benchmarks, examples and fuzz targets are left to the support digest.
// They can still fail a run, but they are not what a mutation report is
// measured against, and treating them as support is the safe side.
func isTestFunction(fn *ast.FuncDecl) bool {
	if fn.Recv != nil || fn.Name == nil || !fn.Name.IsExported() {
		return false
	}
	if !strings.HasPrefix(fn.Name.Name, "Test") {
		return false
	}
	return fn.Type.Params != nil && len(fn.Type.Params.List) == 1
}

// listedPackage is what go list reports about one package's tests.
type listedPackage struct {
	Dir          string
	ImportPath   string
	TestGoFiles  []string
	XTestGoFiles []string
}

func listTestPackages(workingDirectory string, packages []string) ([]listedPackage, error) {
	args := append([]string{"list", "-e", "-json=Dir,ImportPath,TestGoFiles,XTestGoFiles"}, packages...)
	cmd := exec.Command("go", args...)
	cmd.Dir = workingDirectory
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing test files: %w", err)
	}

	var listed []listedPackage
	decoder := json.NewDecoder(strings.NewReader(string(out)))
	for decoder.More() {
		var pkg listedPackage
		if err := decoder.Decode(&pkg); err != nil {
			return nil, fmt.Errorf("listing test files: %w", err)
		}
		listed = append(listed, pkg)
	}
	slices.SortFunc(listed, func(a, b listedPackage) int {
		return strings.Compare(a.ImportPath, b.ImportPath)
	})
	return listed, nil
}

// killers reports which tests failed, qualified by package, from the
// go command's JSON event stream.
//
// These are the tests that caught a mutant. Recording them is what lets
// a later run tell a kill that still stands from one whose only witness
// has changed.
func killers(jsonOutput string) []string {
	type event struct {
		Action  string `json:"Action"`
		Package string `json:"Package"`
		Test    string `json:"Test"`
	}

	found := make(map[string]bool)
	decoder := json.NewDecoder(strings.NewReader(jsonOutput))
	for decoder.More() {
		var e event
		if err := decoder.Decode(&e); err != nil {
			// The stream carries the test binary's own output too, and a
			// line that is not an event is not a reason to stop.
			break
		}
		if e.Action != "fail" || e.Test == "" || e.Package == "" {
			continue
		}
		// A failing subtest fails its parent too, and the parent is what
		// the suite record and -run both name.
		name, _, _ := strings.Cut(e.Test, "/")
		found[qualify(e.Package, name)] = true
	}

	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// suiteScope is the package patterns and -run expression a run tests
// under, recorded so a verdict is not reused for a different selection.
func suiteScope(packages []string, match *regexp.Regexp) string {
	h := sha256.New()
	for _, pattern := range packages {
		fmt.Fprintf(h, "pkg\x00%s\x00", pattern)
	}
	if match != nil {
		fmt.Fprintf(h, "run\x00%s\x00", match.String())
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}
