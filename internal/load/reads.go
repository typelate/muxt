package load

import (
	"fmt"
	"go/ast"
	"go/token"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

// UnreadFileError reports a template file, by absolute path, that the
// templates variable does not read: no //go:embed pattern on the embed.FS it
// parses matches the file together with a ParseFS pattern for that FS. Adding
// a define to such a file would leave the route silently missing.
type UnreadFileError struct {
	File          string
	Variable      string
	EmbedPatterns []string
	ParsePatterns []string
}

// Error returns the file, the variable and both pattern lists on one line.
func (e *UnreadFileError) Error() string {
	return fmt.Sprintf("%s is not read by %s: its //go:embed patterns are %s and its ParseFS patterns are %s; name a file both match",
		e.File, e.Variable, quoteAll(e.EmbedPatterns), quoteAll(e.ParsePatterns))
}

func quoteAll(patterns []string) string {
	quoted := make([]string, len(patterns))
	for i, p := range patterns {
		quoted[i] = strconv.Quote(p)
	}
	return strings.Join(quoted, ", ")
}

// CheckFileRead returns an *UnreadFileError when none of the variables, in
// the package in wd, reads the file named name in wd, or would once it exists.
// It reads only the variable's initializer: a ParseFS call whose arguments it
// cannot read there (an FS from another package, a pattern that is not a
// literal), or a variable with no ParseFS call, means it cannot tell, and the
// file is accepted.
func CheckFileRead(wd string, pl []*packages.Package, variables []string, name string) error {
	pkg, ok := packageInDirectory(pl, wd)
	if !ok {
		return nil
	}
	var first *UnreadFileError
	for _, variable := range variables {
		calls, known := parseFSCalls(pkg, variable)
		if !known {
			return nil
		}
		unread := &UnreadFileError{File: filepath.Join(wd, name), Variable: variable}
		for _, call := range calls {
			unread.EmbedPatterns = append(unread.EmbedPatterns, call.embed...)
			unread.ParsePatterns = append(unread.ParsePatterns, call.parse...)
			if anyMatches(call.embed, name) && anyMatches(call.parse, name) {
				return nil
			}
		}
		if first == nil {
			first = unread
		}
	}
	if first == nil {
		return nil
	}
	return first
}

// fsRead is one ParseFS call: the //go:embed patterns of the FS it reads and
// its own patterns.
type fsRead struct{ embed, parse []string }

// parseFSCalls returns the ParseFS calls in the initializer of variable in
// pkg. known is false when there is none, or when any of them cannot be read
// from this package's syntax alone.
func parseFSCalls(pkg *packages.Package, variable string) (calls []fsRead, known bool) {
	var value ast.Expr
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, ident := range vs.Names {
					if ident.Name == variable && i < len(vs.Values) {
						value = vs.Values[i]
					}
				}
			}
		}
	}
	if value == nil {
		return nil, false
	}
	readable := true
	ast.Inspect(value, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "ParseFS" {
			return true
		}
		read, ok := readParseFS(pkg, call)
		if !ok {
			readable = false
			return true
		}
		calls = append(calls, read)
		return true
	})
	return calls, readable && len(calls) > 0
}

func readParseFS(pkg *packages.Package, call *ast.CallExpr) (fsRead, bool) {
	if len(call.Args) < 2 {
		return fsRead{}, false
	}
	fsIdent, ok := call.Args[0].(*ast.Ident)
	if !ok {
		return fsRead{}, false
	}
	var read fsRead
	for _, arg := range call.Args[1:] {
		lit, ok := arg.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return fsRead{}, false
		}
		pattern, err := strconv.Unquote(lit.Value)
		if err != nil {
			return fsRead{}, false
		}
		read.parse = append(read.parse, pattern)
	}
	embed, ok := embedPatterns(pkg, fsIdent.Name)
	if !ok {
		return fsRead{}, false
	}
	read.embed = embed
	return read, true
}

// embedPatterns returns the patterns of the //go:embed lines on the package
// variable name.
func embedPatterns(pkg *packages.Package, name string) ([]string, bool) {
	const prefix = "//go:embed"
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				vs := spec.(*ast.ValueSpec)
				declares := false
				for _, ident := range vs.Names {
					declares = declares || ident.Name == name
				}
				if !declares {
					continue
				}
				var patterns []string
				for _, group := range []*ast.CommentGroup{gen.Doc, vs.Doc} {
					if group == nil {
						continue
					}
					for _, line := range group.List {
						rest, ok := strings.CutPrefix(line.Text, prefix)
						if !ok {
							continue
						}
						for _, field := range strings.Fields(rest) {
							if unquoted, err := strconv.Unquote(field); err == nil {
								field = unquoted
							}
							patterns = append(patterns, field)
						}
					}
				}
				return patterns, len(patterns) > 0
			}
		}
	}
	return nil, false
}

// anyMatches reports whether a pattern matches the file name in the package
// directory. An all: prefix only changes how a directory is walked, which a
// file in the directory itself never is.
func anyMatches(patterns []string, name string) bool {
	for _, pattern := range patterns {
		pattern = strings.TrimPrefix(pattern, "all:")
		if ok, err := path.Match(pattern, name); err == nil && ok {
			return true
		}
	}
	return false
}
