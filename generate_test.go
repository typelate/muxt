package muxt_test

import (
	"encoding/json"
	"go/format"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/txtar"

	"github.com/crhntr/muxt"
)

func TestGenerate(t *testing.T) {
	matches, err := filepath.Glob(filepath.FromSlash("testdata/*.txtar"))
	require.NoError(t, err)
	for _, match := range matches {
		testGenerate(t, match)
	}
}

func testGenerate(t *testing.T, fileName string) {
	testName := strings.TrimSuffix(path.Base(filepath.ToSlash(fileName)), ".txtar")

	type configuration struct {
		Filename string `json:"filename"`
		Line     int    `json:"line"`
	}

	t.Run(testName, func(t *testing.T) {
		t.Helper()
		dir := t.TempDir()

		archive, err := txtar.ParseFile(fileName)
		require.NoError(t, err, "failed to read testdata")

		var config configuration
		require.NoError(t, json.Unmarshal(archive.Comment, &config), "test configuration parse failed")

		for _, f := range archive.Files {
			if path.Ext(f.Name) == ".expect" {
				continue
			}
			p := filepath.FromSlash(f.Name)
			output := filepath.Join(dir, p)
			require.NoErrorf(t, os.WriteFile(output, f.Data, 0666), "failed to write %s", p)
		}

		require.NoError(t, muxt.Generate(dir, config.Filename, config.Line, []string{"--function=Handlers"}))

		for _, exp := range archive.Files {
			ext := path.Ext(exp.Name)
			if ext != ".expect" {
				continue
			}
			exp.Name = strings.TrimSuffix(exp.Name, ext)

			exp.Data, err = format.Source(exp.Data)
			require.NoError(t, err)

			p := filepath.FromSlash(exp.Name)
			fp := filepath.Join(dir, p)
			got, err := os.ReadFile(fp)
			require.NoError(t, err)
			assert.Equal(t, string(exp.Data), string(got))
		}
	})
}
