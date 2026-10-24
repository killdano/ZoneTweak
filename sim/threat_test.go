package sim

import (
	"os"
	"testing"
)

// A player who only works on the device and never defends: the wardens have to win.
func TestPassivePlayerLoses(t *testing.T) {
	room := rectRoom(10, 10)
	for s := int64(1); s <= 6; s++ {
		r := NewRound(room, s, nil, nil)
		r.SetGens(6)
		r.Start()
		blown, steal, lostAt := 0, 0.0, 0.0
		for r.Phase == Playing && r.Time < 900 {
			r.Step(0.05)
			for _, e := range r.TakeEvents() {
				switch e.Kind {
				case EvGenBlown:
					blown++
				case EvStolen:
					if steal == 0 {
						steal = r.Time
					}
				case EvLost:
					lostAt = r.Time
				}
			}
		}
		if os.Getenv("VERBOSE") != "" || r.Phase != Lost {
			t.Logf("seed %d: phase %d at %.0f s, blown %d, first steal %.0f s, lost %.0f s", s, r.Phase, r.Time, blown, steal, lostAt)
		}
		if r.Phase != Lost {
			t.Errorf("seed %d: nobody defending and the wardens never took the device", s)
		}
	}
}
