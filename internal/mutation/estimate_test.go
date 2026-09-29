package mutation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
			assert.Equal(t, tt.want, e.total(), "total()")
		})
	}
}

func TestEstimateObserve(t *testing.T) {
	e := estimate{perMutant: 10 * time.Second, remaining: 1, workers: 1}
	e.observe(2 * time.Second)
	e.observe(4 * time.Second)
	assert.Equal(t, 3*time.Second, e.perMutant, "perMutant is the average of what was observed")
	assert.Equal(t, 0, e.remaining, "remaining stops at 0")
}
