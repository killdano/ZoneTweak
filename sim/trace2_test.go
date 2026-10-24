package sim

import (
	"os"
	"testing"
)

func TestTraceSteal(t *testing.T) {
	if os.Getenv("TRACE2") == "" {
		t.Skip()
	}
	r := NewRound(rectRoom(10, 10), 6, nil, nil)
	r.SetGens(6)
	r.Start()
	next := 150.0
	for r.Phase == Playing && r.Time < 400 {
		r.Step(0.05)
		for _, e := range r.TakeEvents() {
			switch e.Kind {
			case EvGenBlown, EvExposed, EvStolen, EvDropped, EvRecovered, EvLost, EvFieldDown, EvRing, EvZapInside:
				t.Logf("%6.1f ev %d A=%d B=%d %s at %.1f,%.1f", r.Time, e.Kind, e.A, e.B, e.S, e.Pos.X, e.Pos.Z)
			case EvDied:
				if r.Time > 150 {
					t.Logf("%6.1f died %d (%s) at %.1f,%.1f", r.Time, e.A, e.S, e.Pos.X, e.Pos.Z)
				}
			}
		}
		if r.Time >= next {
			next += 10
			st := map[RobotState]int{}
			for _, x := range r.Robots {
				st[x.State]++
			}
			d := r.Dev
			t.Logf("%6.1f robots %v dev state %d pos %.1f,%.1f exposed %v field %.1f grabber %d carrier %d ring %d queue %d", r.Time, st, d.State, d.Pos.X, d.Pos.Z, d.Exposed, d.Field, d.Grabber, d.Carrier, len(r.Ring), len(r.queue))
		}
	}
}
