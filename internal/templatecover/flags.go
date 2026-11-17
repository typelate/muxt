package templatecover

import (
	"errors"
	"fmt"
	"iter"
	"regexp"
	"strings"
)

// Flag is one step of a walk over go test arguments.
type Flag struct {
	// Name is the flag's name without its dashes, "" for an argument that
	// is not a flag, and "args" for the -args that hands the rest to the
	// test binary.
	Name string

	// Value is the flag's value, after its = or, for a flag that takes one,
	// the next argument; HasValue says whether it has one.
	Value    string
	HasValue bool

	// Args are the arguments the step spans: the flag and its value, or for
	// -args, everything from it on.
	Args []string
}

// takesValue reports whether a go test or build flag takes a value,
// which without an = is the next argument. Any other flag, the test
// binary's own among them, is read as one that takes none, as go test
// reads a flag it does not know.
func takesValue(name string) bool {
	switch name {
	case "asmflags", "bench", "benchtime", "blockprofile", "blockprofilerate",
		"buildmode", "compiler", "count", "covermode", "coverpkg",
		"coverprofile", "cpu", "cpuprofile", "exec", "fuzz",
		"fuzzminimizetime", "fuzztime", "gccgoflags", "gcflags",
		"installsuffix", "ldflags", "list", "memprofile", "memprofilerate",
		"mod", "modfile", "mutexprofile", "mutexprofilefraction", "o",
		"outputdir", "overlay", "p", "parallel", "pgo", "pkgdir",
		"run", "shuffle", "skip", "tags", "timeout", "toolexec",
		"trace", "vet":
		return true
	}
	return false
}

// Flags walks go test arguments as go test reads them, one flag, with its
// value, at a time. It stops at -args, whose arguments belong to the test
// binary, and yields them as one last step.
func Flags(args []string) iter.Seq[Flag] {
	return func(yield func(Flag) bool) {
		for i := 0; i < len(args); i++ {
			arg := args[i]
			if arg == "-args" || arg == "--args" {
				yield(Flag{Name: "args", Args: args[i:]})
				return
			}
			if !strings.HasPrefix(arg, "-") {
				// A value of a flag it did not know to take one, such
				// as a test binary's own.
				if !yield(Flag{Args: args[i : i+1]}) {
					return
				}
				continue
			}
			name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
			end := i + 1
			if !hasValue && takesValue(name) && end < len(args) {
				value, hasValue = args[end], true
				end++
			}
			if !yield(Flag{Name: name, Value: value, HasValue: hasValue, Args: args[i:end]}) {
				return
			}
			i = end - 1
		}
	}
}

// Refuse reports why a go test flag cannot be passed through beside the
// engine, or nil when it can.
//
// -overlay is how a mutant reaches the build. A second one would either be
// ignored or replace the mutant, and either way the run would measure the
// templates as written rather than as mutated.
//
// -mod and -modfile are refused because the engine sets both: -modfile
// names a copy of go.mod that requires github.com/typelate/htt, whose
// template package the html/template shim forwards to, and -mod=mod lets
// the go command complete that copy's go.sum.
//
// -coverpkg is refused when it covers html/template, by name, by a pattern
// matching it, or as std or all: the engine replaces that package through
// the overlay, and coverage instrumentation reads the packages it
// instruments from disk, which would undo it.
func Refuse(f Flag) error {
	switch f.Name {
	case "overlay":
		return errors.New("go test flag -overlay cannot be passed through: muxt uses -overlay to deliver each mutant")
	case "mod", "modfile":
		return fmt.Errorf("go test flag -%s cannot be passed through: the coverage pre-pass sets -mod=mod and -modfile to require github.com/typelate/htt, muxt's template engine, in a copy of go.mod", f.Name)
	case "coverpkg":
		if pattern, ok := coversHTMLTemplate(f.Value); ok {
			return fmt.Errorf("go test flag -coverpkg cannot cover html/template (%q does): muxt replaces html/template with its own template engine to learn which template actions the tests reach; use -coverpkg with your own packages, such as ./... instead", pattern)
		}
	}
	return nil
}

// coversHTMLTemplate returns the first of a -coverpkg value's
// comma-separated patterns that takes in html/template.
func coversHTMLTemplate(value string) (string, bool) {
	for pattern := range strings.SplitSeq(value, ",") {
		switch {
		case pattern == "std", pattern == "all":
			return pattern, true
		case strings.Contains(pattern, "..."):
			parts := strings.Split(pattern, "...")
			for j := range parts {
				parts[j] = regexp.QuoteMeta(parts[j])
			}
			if regexp.MustCompile("^" + strings.Join(parts, ".*") + "$").MatchString("html/template") {
				return pattern, true
			}
		case pattern == "html/template":
			return pattern, true
		}
	}
	return "", false
}
