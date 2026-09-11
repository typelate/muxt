package mutation

import (
	"testing"
	"time"
)

func TestEstimateTotal(t *testing.T) {
	for _, tt := range []struct {
		name      string
		remaining int
		workers   int
		want      time.Duration
	}{
		{name: "nothing left", remaining: 0, workers: 1, want: 0},
		{name: "one at a time", remaining: 3, workers: 1, want: 3 * time.Second},
		{name: "a partial last round still takes a round", remaining: 3, workers: 2, want: 2 * time.Second},
		{name: "full rounds", remaining: 4, workers: 2, want: 2 * time.Second},
		{name: "more slots than work", remaining: 1, workers: 8, want: time.Second},
		{name: "no workers given means one", remaining: 3, workers: 0, want: 3 * time.Second},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := estimate{perMutant: time.Second, remaining: tt.remaining, workers: tt.workers}
			if got := e.total(); got != tt.want {
				t.Errorf("total() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEstimateObserve(t *testing.T) {
	e := estimate{perMutant: 10 * time.Second, remaining: 1, workers: 1}
	e.observe(2 * time.Second)
	e.observe(4 * time.Second)
	if e.perMutant != 3*time.Second {
		t.Errorf("perMutant = %v, want the average of what was observed, 3s", e.perMutant)
	}
	if e.remaining != 0 {
		t.Errorf("remaining = %d, want it to stop at 0", e.remaining)
	}
}
