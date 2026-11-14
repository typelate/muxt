package composite_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/composite"
)

// addEntry is a change that adds the entry key, the way generate package does.
func addEntry(key string) func(*composite.File) error {
	return func(f *composite.File) error {
		return f.Add(key, composite.Entry{})
	}
}

// TestUpdateLetsOneRunAddAKey states that of two runs adding the same key,
// the second, reading the file the first wrote, fails with an *ExistsError
// naming the file and the key, and writes nothing.
func TestUpdateLetsOneRunAddAKey(t *testing.T) {
	const key = "example.com/m/same"
	dir := writeFile(t, `{"packages": {}}`)
	path := filepath.Join(dir, composite.FileName)
	policy := composite.LockPolicy{Attempts: 1, Pause: runtime.Gosched}
	require.NoError(t, composite.UpdateWith(path, testKeys(), addEntry(key), policy), "first Update")
	afterFirst, err := os.ReadFile(path)
	require.NoError(t, err, "reading muxt.json")

	err = composite.UpdateWith(path, testKeys(), addEntry(key), policy)

	var exists *composite.ExistsError
	require.ErrorAs(t, err, &exists, "second Update error %v", err)
	assert.Equal(t, composite.ExistsError{Path: path, Where: `packages["example.com/m/same"]`}, *exists, "error")
	afterSecond, err := os.ReadFile(path)
	require.NoError(t, err, "reading muxt.json")
	assert.Equal(t, string(afterFirst), string(afterSecond), "muxt.json after the refused run")
}

// TestUpdateKeepsEveryConcurrentEntry states that goroutines each adding a
// different entry through Update leave all of them in the file: each reads the
// file fresh under the lock, so none overwrites another's.
func TestUpdateKeepsEveryConcurrentEntry(t *testing.T) {
	const n = 16
	dir := writeFile(t, `{"packages": {}}`)
	path := filepath.Join(dir, composite.FileName)
	// A lock held for a moment is waited for by yielding, not sleeping.
	policy := composite.LockPolicy{Attempts: 1_000_000, Pause: runtime.Gosched}
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			errs[i] = composite.UpdateWith(path, testKeys(), addEntry(fmt.Sprintf("example.com/m/p%d", i)), policy)
		})
	}
	wg.Wait()

	require.NoError(t, errors.Join(errs...), "Update")
	f, found, err := composite.Find(dir, testKeys())
	require.NoError(t, err, "Find")
	require.True(t, found, "found")
	assert.Len(t, f.Packages, n, "entries in muxt.json: %v", f.Keys())
}

// TestUpdateFailsWhileLockIsHeld states that Update, finding muxt.json.lock,
// waits the bounded number of attempts and then fails with a *LockedError
// naming the file and the lock by absolute path, changing nothing and leaving
// the other process's lock.
func TestUpdateFailsWhileLockIsHeld(t *testing.T) {
	const file = `{"packages": {}}`
	dir := writeFile(t, file)
	path := filepath.Join(dir, composite.FileName)
	lock := path + ".lock"
	require.NoError(t, os.WriteFile(lock, nil, 0o644))
	pauses := 0
	policy := composite.LockPolicy{Attempts: 3, Pause: func() { pauses++ }}
	called := false

	err := composite.UpdateWith(path, testKeys(), func(*composite.File) error {
		called = true
		return nil
	}, policy)

	locked, ok := errors.AsType[*composite.LockedError](err)
	require.True(t, ok, "Update = %v, want *composite.LockedError", err)
	assert.Equal(t, path, locked.Path, "path")
	assert.Equal(t, lock, locked.Lock, "lock")
	assert.Equal(t, path+" is locked by "+lock+": another muxt run is writing it; delete the lock file if none is", err.Error(), "message")
	assert.Equal(t, 2, pauses, "pauses between 3 attempts")
	assert.False(t, called, "change called without the lock")
	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, file, string(got), "muxt.json changed")
	assert.FileExists(t, lock, "another run's lock was removed")
}

// failingClose is a lock file whose Close fails after it was created.
type failingClose struct{ err error }

func (c failingClose) Close() error { return c.err }

// TestUpdateRemovesLockWhenClosingItFails states that a lock Update created
// but could not close is removed, so a later run does not wait on a lock no
// run holds, and that the error says why and changes nothing.
func TestUpdateRemovesLockWhenClosingItFails(t *testing.T) {
	const file = `{"packages": {}}`
	dir := writeFile(t, file)
	path := filepath.Join(dir, composite.FileName)
	closeErr := errors.New("close failed")
	policy := composite.LockPolicy{Attempts: 1, Pause: func() {}, Create: func(name string) (io.Closer, error) {
		if err := os.WriteFile(name, nil, 0o644); err != nil {
			return nil, err
		}
		return failingClose{err: closeErr}, nil
	}}

	err := composite.UpdateWith(path, testKeys(), addEntry("example.com/m/a"), policy)

	assert.ErrorIs(t, err, closeErr, "Update")
	assert.NoFileExists(t, path+".lock", "lock file outlived Update")
	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, file, string(got), "muxt.json changed")
}

// TestUpdateRemovesLock states that muxt.json.lock is gone after Update
// succeeds and after the change fails, and that a failing change writes
// nothing.
func TestUpdateRemovesLock(t *testing.T) {
	failure := errors.New("refused")
	for _, tt := range []struct {
		name    string
		change  func(*composite.File) error
		wantErr error
		want    string
	}{
		{name: "success", change: addEntry("example.com/m/a"), want: "example.com/m/a"},
		{name: "failing change", change: func(f *composite.File) error {
			f.Packages["example.com/m/a"] = composite.Entry{}
			return failure
		}, wantErr: failure},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeFile(t, `{"packages": {}}`)
			path := filepath.Join(dir, composite.FileName)

			err := composite.Update(path, testKeys(), tt.change)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr, "Update")
			} else {
				require.NoError(t, err, "Update")
			}
			assert.NoFileExists(t, path+".lock", "lock file outlived Update")
			f, _, findErr := composite.Find(dir, testKeys())
			require.NoError(t, findErr, "Find")
			if tt.want == "" {
				assert.Empty(t, f.Packages, "a failing change was saved")
				return
			}
			assert.Contains(t, f.Keys(), tt.want, "entries")
		})
	}
}
