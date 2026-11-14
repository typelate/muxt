package composite

// LockPolicy is lockPolicy for the tests of package composite_test, which
// state the wait on a held lock without a real sleep.
type LockPolicy = lockPolicy

// UpdateWith is update for the tests of package composite_test.
func UpdateWith(path string, keys Keys, change func(*File) error, policy LockPolicy) error {
	return update(path, keys, change, policy)
}
