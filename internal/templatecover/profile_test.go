package templatecover_test

import (
	"bytes"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/typelate/htt"

	"github.com/typelate/muxt/internal/source"
	"github.com/typelate/muxt/internal/templatecover"
)

// definedIn is a definition of a template parsed as parseName from file.
func definedIn(file, parseName string) source.Definition {
	return source.Definition{
		Define: source.Span{Position: token.Position{Filename: file}},
		Tree:   &htt.Tree{ParseName: parseName, Filepath: file},
	}
}

func packageWith(definitions map[string]source.Definition) source.Package {
	return source.Package{Variables: []source.Variable{{Name: "templates", Definitions: definitions}}}
}

func written(t *testing.T, p *templatecover.Profile) string {
	t.Helper()
	var buf bytes.Buffer
	_, err := p.WriteTo(&buf)
	require.NoError(t, err)
	return buf.String()
}

// TestProfileNamesTheFileTheTemplateWasParsedFrom states that a block the
// tests reached under a parse name is written under the absolute path of
// the file that definition was parsed from.
func TestProfileNamesTheFileTheTemplateWasParsedFrom(t *testing.T) {
	sources := templatecover.Sources(packageWith(map[string]source.Definition{
		"index.gohtml": definedIn("/abs/pkg/index.gohtml", "index.gohtml"),
	}))
	reached := templatecover.Reached{"index.gohtml": {{StartLine: 2, StartCol: 1, EndLine: 2, EndCol: 9}: {}}}

	var p templatecover.Profile
	p.Add(reached, sources)

	assert.Equal(t, "mode: set\n/abs/pkg/index.gohtml:2.1,2.9 1 1\n", written(t, &p))
}

// TestProfileLeavesOutBlocksOfATemplateWithNoFile states that a parse name
// no definition's file carries is left out: a template parsed from a string
// or a file read at run time has no file to put its blocks in.
func TestProfileLeavesOutBlocksOfATemplateWithNoFile(t *testing.T) {
	sources := templatecover.Sources(packageWith(map[string]source.Definition{
		"index.gohtml": definedIn("/abs/pkg/index.gohtml", "index.gohtml"),
		"inline":       definedIn("/abs/pkg/page.go", "inline"),
	}))
	block := map[templatecover.Block]struct{}{{StartLine: 2, StartCol: 1, EndLine: 2, EndCol: 9}: {}}
	reached := templatecover.Reached{"inline": block, "read-at-run-time.gohtml": block}

	var p templatecover.Profile
	p.Add(reached, sources)

	assert.Equal(t, "mode: set\n", written(t, &p))
}
