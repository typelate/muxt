package main

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// committedEngine is the copy go generate ./internal/overlay writes, relative
// to this package's directory.
const committedEngine = "../../_engine"

func TestEngineInSync(t *testing.T) {
	sources, err := listSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := generate(sources, dir); err != nil {
		t.Fatal(err)
	}

	want := readTree(t, dir)
	got := readTree(t, committedEngine)
	for name, w := range want {
		g, ok := got[name]
		if !ok {
			t.Errorf("%s is missing from the committed engine; run go generate ./internal/overlay", name)
			continue
		}
		if !bytes.Equal(g, w) {
			t.Errorf("%s differs from a fresh copy; run go generate ./internal/overlay", name)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("%s is in the committed engine but not in a fresh copy; run go generate ./internal/overlay", name)
		}
	}
}

func TestNoticeCarriesXNetLicense(t *testing.T) {
	sources, err := listSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var module string
	for _, s := range sources {
		if s.ImportPath == "golang.org/x/net/html" {
			module = s.Module.Dir
		}
	}
	if module == "" {
		t.Fatal("golang.org/x/net/html has no module directory")
	}
	license, err := os.ReadFile(filepath.Join(module, "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	notice, err := os.ReadFile("../../../../NOTICE.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(notice, bytes.TrimSpace(license)) {
		t.Errorf("NOTICE.txt does not reproduce %s", filepath.Join(module, "LICENSE"))
	}
	if !bytes.Contains(notice, []byte("internal/overlay/_engine/internal/xhtml")) {
		t.Error("NOTICE.txt does not name the x/net copy under internal/overlay/_engine/internal/xhtml")
	}
}

func TestRewriteFile(t *testing.T) {
	rules := rules{
		paths: map[string]string{
			"example.com/m/internal/template":     "html/template/internal/engine",
			"example.com/m/internal/template/lex": "html/template/internal/lex",
		},
		renames: map[string]rename{
			"html/template/internal/engine": {from: "template", to: "engine"},
		},
	}
	for _, tt := range []struct {
		name, clause, src, want string
		drop                    bool
	}{
		{
			name: "renamed import and its references",
			src: `package html

import (
	"fmt"

	"example.com/m/internal/template"
	"example.com/m/internal/template/lex"
)

func f(template2 int) (*template.Tree, lex.Pos) {
	fmt.Println(template2)
	return template.New(), 0
}
`,
			clause: "template",
			want: `package template

import (
	"fmt"

	"html/template/internal/engine"
	"html/template/internal/lex"
)

func f(template2 int) (*engine.Tree, lex.Pos) {
	fmt.Println(template2)
	return engine.New(), 0
}
`,
		},
		{
			name: "a local variable named like the import is left alone",
			src: `package html

import "example.com/m/internal/template"

func f() int {
	template := struct{ X int }{}
	_ = engineUse
	return template.X
}

var engineUse = template.X
`,
			want: `package html

import "html/template/internal/engine"

func f() int {
	template := struct{ X int }{}
	_ = engineUse
	return template.X
}

var engineUse = engine.X
`,
		},
		{
			name: "stdlib html/template aliases become definitions",
			src: `package html

import (
	"html/template"
	"strconv"
)

// HTML is safe.
type HTML = template.HTML

// ErrorCode classifies errors.
type ErrorCode = template.ErrorCode

var _ = strconv.Itoa
`,
			want: `package html

import (
	"strconv"
)

// HTML is safe.
type HTML string

// ErrorCode classifies errors.
type ErrorCode int

var _ = strconv.Itoa
`,
		},
		{
			name: "renamed package clause",
			src: `package template

func New() {}
`,
			clause: "engine",
			want: `package engine

func New() {}
`,
		},
		{
			name: "ignore build constraint drops the file",
			src: `//go:build ignore

package main
`,
			drop: true,
		},
		{
			name: "other build constraints are kept",
			src: `//go:build linux

package lex
`,
			want: `//go:build linux

package lex
`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, keep, err := rules.rewriteFile("f.go", []byte(tt.src), tt.clause)
			if err != nil {
				t.Fatal(err)
			}
			if keep == tt.drop {
				t.Fatalf("rewriteFile kept = %t, want %t", keep, !tt.drop)
			}
			if tt.drop {
				return
			}
			if string(got) != tt.want {
				t.Errorf("rewriteFile =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestRewriteFileRejectsUnknownStdlibAlias(t *testing.T) {
	src := `package html

import "html/template"

type Template = template.Template
`
	if _, _, err := (rules{}).rewriteFile("f.go", []byte(src), ""); err == nil {
		t.Fatal("rewriteFile accepted an alias to html/template.Template, which has no known definition")
	}
}

func readTree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := make(map[string][]byte)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = b
		return nil
	})
	if err != nil {
		t.Fatalf("reading %s: %v", root, err)
	}
	return files
}
