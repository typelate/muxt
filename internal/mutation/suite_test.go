package mutation

import "testing"

func suiteOf(support string, tests map[string]string) TestSuite {
	return TestSuite{Packages: map[string]TestPackage{
		"server": {Support: support, Tests: tests},
	}}
}

// TestSuiteDeltaDecidesWhatStillAnswers states the rule the whole reuse
// mechanism turns on.
//
// A kill needs one failing test, so it stands while the test that reached
// it is untouched, however much the rest of the suite moved. A miss needs
// the whole suite to pass, so anything added or edited could have closed
// it -- but a deletion alone cannot catch a mutant nothing was catching.
//
// Getting either direction wrong is silent. Reusing a kill too eagerly
// reports coverage the suite no longer has; refusing to reuse costs only
// time, which is why the doubtful cases here fall that way.
func TestSuiteDeltaDecidesWhatStillAnswers(t *testing.T) {
	const (
		greetDigest    = "aaaa"
		farewellDigest = "bbbb"
		support        = "ssss"
	)
	before := suiteOf(support, map[string]string{
		"TestGreet":    greetDigest,
		"TestFarewell": farewellDigest,
	})

	killedByGreet := StateResult{Status: StatusKilled, Killers: []string{"server.TestGreet"}}
	missed := StateResult{Status: StatusMissed}

	for _, tt := range []struct {
		name       string
		now        TestSuite
		reuseKill  bool
		reuseMiss  bool
		reuseSkip  bool
		reuseBlind bool // a kill with no recorded killer
	}{
		{
			name:      "nothing changed",
			now:       before,
			reuseKill: true, reuseMiss: true, reuseSkip: true, reuseBlind: true,
		},
		{
			name: "a test was added",
			now: suiteOf(support, map[string]string{
				"TestGreet": greetDigest, "TestFarewell": farewellDigest, "TestNew": "cccc",
			}),
			// The new test cannot un-kill anything, but it could close
			// the miss.
			reuseKill: true, reuseMiss: false, reuseSkip: true, reuseBlind: false,
		},
		{
			name: "the test that caught it was edited",
			now: suiteOf(support, map[string]string{
				"TestGreet": "edited", "TestFarewell": farewellDigest,
			}),
			reuseKill: false, reuseMiss: false, reuseSkip: true, reuseBlind: false,
		},
		{
			name: "a different test was edited",
			now: suiteOf(support, map[string]string{
				"TestGreet": greetDigest, "TestFarewell": "edited",
			}),
			// The witness to this kill is untouched.
			reuseKill: true, reuseMiss: false, reuseSkip: true, reuseBlind: false,
		},
		{
			name: "the test that caught it was deleted",
			now:  suiteOf(support, map[string]string{"TestFarewell": farewellDigest}),
			// Nothing was added, so the miss is still a miss.
			reuseKill: false, reuseMiss: true, reuseSkip: true, reuseBlind: false,
		},
		{
			name:      "a different test was deleted",
			now:       suiteOf(support, map[string]string{"TestGreet": greetDigest}),
			reuseKill: true, reuseMiss: true, reuseSkip: true, reuseBlind: false,
		},
		{
			name: "a helper the tests are built on changed",
			now: suiteOf("edited", map[string]string{
				"TestGreet": greetDigest, "TestFarewell": farewellDigest,
			}),
			// A test's own digest cannot see its helpers, so none of
			// them can be trusted.
			reuseKill: false, reuseMiss: false, reuseSkip: true, reuseBlind: false,
		},
		{
			name:      "the package went away",
			now:       TestSuite{Packages: map[string]TestPackage{}},
			reuseKill: false, reuseMiss: true, reuseSkip: true, reuseBlind: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := tt.now.delta(before)

			if got := d.reusable(killedByGreet); got != tt.reuseKill {
				t.Errorf("reusable(kill caught by TestGreet) = %t, want %t", got, tt.reuseKill)
			}
			if got := d.reusable(missed); got != tt.reuseMiss {
				t.Errorf("reusable(miss) = %t, want %t", got, tt.reuseMiss)
			}
			if got := d.reusable(StateResult{Status: StatusSkipped}); got != tt.reuseSkip {
				t.Errorf("reusable(skip) = %t, want %t", got, tt.reuseSkip)
			}
			// A kill recorded before killers were kept names no witness,
			// so it can only be trusted while nothing moved at all.
			if got := d.reusable(StateResult{Status: StatusKilled}); got != tt.reuseBlind {
				t.Errorf("reusable(kill with no recorded killer) = %t, want %t", got, tt.reuseBlind)
			}
		})
	}
}

// TestKillersReadsFailingTestsFromTheEventStream states that the tests
// which caught a mutant are taken from the go command's own report.
//
// They are what makes a kill reusable later, so reading them wrongly
// either throws away every kill or keeps one whose witness has gone.
func TestKillersReadsFailingTestsFromTheEventStream(t *testing.T) {
	const stream = `{"Action":"run","Package":"server","Test":"TestGreet"}
{"Action":"output","Package":"server","Test":"TestGreet","Output":"    greeting = \"\"\n"}
{"Action":"fail","Package":"server","Test":"TestGreet/lowercase"}
{"Action":"fail","Package":"server","Test":"TestGreet"}
{"Action":"pass","Package":"server","Test":"TestFarewell"}
{"Action":"fail","Package":"server/admin","Test":"TestDashboard"}
{"Action":"fail","Package":"server"}
{"Action":"pass","Package":"server/admin"}
`

	got := killers(stream)
	want := []string{"server.TestGreet", "server/admin.TestDashboard"}

	if len(got) != len(want) {
		t.Fatalf("killers = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("killers = %v, want %v", got, want)
		}
	}
}
