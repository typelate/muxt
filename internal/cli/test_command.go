package cli

// testConfiguration is what the command line gives muxt test.
type testConfiguration struct {
	// GoTestArgs is everything after --, as written.
	GoTestArgs []string
}
