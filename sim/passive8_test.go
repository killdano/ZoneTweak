package sim

import (
	"os"
	"testing"
)

func TestPassiveDanLike(t *testing.T) {
	if os.Getenv("PASS8") == "" {
		t.Skip()
	}
	room := rectRoom(13, 14)
	for s := int64(1); s <= 6; s++ {
		r := NewRound(room, s, nil, nil)
		r.SetGens(8)
		r.Start()
		blown, steal := 0, 0.0
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
				}
			}
		}
		t.Logf("seed %d: phase %d at %.0f s, blown %d, first steal %.0f s", s, r.Phase, r.Time, blown, steal)
	}
}
