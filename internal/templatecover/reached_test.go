package templatecover_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/templatecover"
)

// TestReadProfilesUnionsTheBlocksAnyBinaryReached states that a block is
// reached when any test binary's profile counts it, and one no binary
// counts is not.
func TestReadProfilesUnionsTheBlocksAnyBinaryReached(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "1.cov"), []byte("mode: set\nindex.gohtml:1.1,1.8 1 1\nindex.gohtml:1.30,1.34 1 0\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "2.cov"), []byte("mode: set\nindex.gohtml:2.1,2.5 1 1\n"), 0o600))

	got, err := templatecover.ReadProfiles(dir)
	require.NoError(t, err)

	assert.Equal(t, templatecover.Reached{"index.gohtml": {
		{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 8}: {},
		{StartLine: 2, StartCol: 1, EndLine: 2, EndCol: 5}: {},
	}}, got)
}

// TestReadProfilesWithNoProfile states that a directory with no profile is
// ErrNoProfile, not an empty Reached.
func TestReadProfilesWithNoProfile(t *testing.T) {
	_, err := templatecover.ReadProfiles(t.TempDir())
	assert.ErrorIs(t, err, templatecover.ErrNoProfile)
}
