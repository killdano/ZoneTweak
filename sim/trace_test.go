package sim

import (
	"fmt"
	"os"
	"testing"
)

func TestTrace(t *testing.T) {
	if os.Getenv("TRACE") == "" {
		t.Skip()
	}
	n := 4
	fmt.Sscan(os.Getenv("TRACE"), &n)
	rw, rd := 10.0, 10.0
	if v := os.Getenv("TRACEROOM"); v != "" {
		fmt.Sscanf(v, "%f,%f", &rw, &rd)
	}
	r := NewRound(rectRoom(rw, rd), 5, nil, nil)
	r.SetGens(n)
	b := NewBot(r, 9)
	b.BuildMaze(0.8)
	t.Logf("points left %.0f, maze walls %d, gens %d", r.Points, len(r.Maze), len(r.Gens))
	for _, w := range r.Maze {
		t.Logf("  wall %.2f,%.2f -> %.2f,%.2f", w.A.X, w.A.Z, w.B.X, w.B.Z)
	}
	r.Start()
	dt := 0.05
	next := 0.0
	for r.Phase == Playing && r.Time < 600 {
		r.Step(dt)
		b.Step(dt)
		for _, e := range r.TakeEvents() {
			switch e.Kind {
			case EvGenBlown, EvGenUp, EvLeech, EvPluck, EvExposed, EvStolen, EvDropped, EvRecovered, EvArmed, EvHit, EvLost, EvWon, EvWave, EvBurn, EvSetback:
				t.Logf("%6.1f ev %d A=%d B=%d %s at %.1f,%.1f", r.Time, e.Kind, e.A, e.B, e.S, e.Pos.X, e.Pos.Z)
			}
		}
		if r.Time >= next {
			next += 10
			st := map[RobotState]int{}
			for _, x := range r.Robots {
				st[x.State]++
			}
			gs := ""
			for _, g := range r.Gens {
				gs += fmt.Sprintf(" %d:%.0f", g.State, g.Charge)
			}
			t.Logf("%6.1f wave %d robots %v gens%s dev %d exp %v work %.0f job %s pts %.0f", r.Time, r.Wave, st, gs, r.Dev.State, r.Dev.Exposed, r.Work(), b.Job, r.Points)
		}
	}
	t.Logf("end phase %d at %.1f stats %+v", r.Phase, r.Time, r.Stats)
}
