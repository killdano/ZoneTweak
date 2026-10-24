package sim

import (
	"fmt"
	"os"
	"testing"
)

// WHY=1: how rounds end for the person-like bot (won / lost to a steal / still going at
// 900 s), and how much the wardens actually do to the ring.
func TestWhyRoundsEnd(t *testing.T) {
	if os.Getenv("WHY") == "" {
		t.Skip()
	}
	for _, rm := range [][2]float64{{10, 10}, {14, 12}} {
		room := rectRoom(rm[0], rm[1])
		tu := DefaultTuning()
		lo, hi := GenRange(room, tu)
		for _, n := range []int{lo, hi} {
			won, lost, timeout := 0, 0, 0
			var blown, steals, drainSum, leeches float64
			for s := 1; s <= 10; s++ {
				r := NewRound(room, int64(s*1000+n), tu, nil)
				r.SetGens(n)
				b := NewBot(r, int64(s*31+n))
				b.BuildMaze(0.8)
				r.Start()
				for r.Phase == Playing && r.Time < 900 {
					before := 0.0
					for _, g := range r.Gens {
						before += g.Charge
					}
					r.Step(0.05)
					for _, e := range r.Events {
						if e.Kind == EvLeech {
							leeches++
						}
					}
					b.Step(0.05)
					after := 0.0
					for _, g := range r.Gens {
						after += g.Charge
					}
					if after < before {
						drainSum += before - after
					}
				}
				switch r.Phase {
				case Won:
					won++
				case Lost:
					lost++
				default:
					timeout++
				}
				blown += float64(r.Stats.GensBlown)
				steals += float64(r.Stats.Steals)
			}
			t.Logf("%vx%v %d gens: won %d lost %d timeout %d | per round: gens blown %.1f, steals %.1f, leeches %.1f, charge drained %.0f",
				rm[0], rm[1], n, won, lost, timeout, blown/10, steals/10, leeches/10, drainSum/10)
		}
	}
	_ = fmt.Sprint
}
