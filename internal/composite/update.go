package composite

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"
)

// LockSuffix is appended to a muxt.json path to name the lock file Update holds
// while it rewrites the file. Creating it with O_EXCL is the one lock every
// platform muxt ships for gives without a dependency.
const LockSuffix = ".lock"

// lockPolicy is how long update waits for a lock another run holds: Attempts
// tries to create it, with Pause between two tries. Tests give Pause a stand-in
// so no test sleeps.
type lockPolicy struct {
	Attempts int
	Pause    func()
	// Create makes the lock file and fails with fs.ErrExist when it is there.
	// Nil means createExclusive; a test gives one whose Close fails, which no
	// real file can be made to do on demand.
	Create func(name string) (io.Closer, error)
}

// createExclusive creates name, failing with fs.ErrExist when it is there.
func createExclusive(name string) (io.Closer, error) {
	return os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
}

// Update reads the muxt.json at path, an absolute path, applies change to what
// it read, and saves the result, all while holding path+LockSuffix, so
// concurrent runs each see the others' entries instead of overwriting them.
// change runs against a fresh read and may refuse by returning an error, which
// Update returns as it is without writing. A lock held by another run is
// waited for about five seconds, then Update fails with a *LockedError. The
// lock file is removed before Update returns. keys is as Find takes it.
func Update(path string, keys Keys, change func(*File) error) error {
	return update(path, keys, change, lockPolicy{
		Attempts: 100,
		Pause:    func() { time.Sleep(50 * time.Millisecond) },
	})
}

func update(path string, keys Keys, change func(*File) error, policy lockPolicy) (err error) {
	lock := path + LockSuffix
	if err := acquire(path, lock, policy); err != nil {
		return err
	}
	defer func() {
		if removeErr := os.Remove(lock); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("removing %s: %w", lock, removeErr))
		}
	}()
	f, err := read(path, keys)
	if err != nil {
		return err
	}
	if err := change(f); err != nil {
		return err
	}
	return f.Save()
}

// acquire creates lock, waiting as policy says while it exists. It fails with
// a *LockedError when every attempt finds it held.
func acquire(path, lock string, policy lockPolicy) error {
	create := policy.Create
	if create == nil {
		create = createExclusive
	}
	for attempt := 1; ; attempt++ {
		f, err := create(lock)
		if err == nil {
			if err := f.Close(); err != nil {
				// update has not deferred the removal yet, so a lock left
				// here would stall every later run.
				err = fmt.Errorf("creating %s: %w", lock, err)
				if removeErr := os.Remove(lock); removeErr != nil {
					err = errors.Join(err, fmt.Errorf("removing %s: %w", lock, removeErr))
				}
				return err
			}
			return nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("creating %s: %w", lock, err)
		}
		if attempt >= policy.Attempts {
			return &LockedError{Path: path, Lock: lock}
		}
		policy.Pause()
	}
}
