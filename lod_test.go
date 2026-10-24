//go:build !windows

package main

import (
	"boundaryvr/sim"
	"testing"
	"time"
)

func TestRobotCost(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	tt := newTester(t, true)
	r := tt.g.r
	for _, d := range []float64{2, 4, 7, 12} {
		for _, k := range []sim.RobotKind{sim.Walker, sim.Heavy} {
			r.Robots = nil
			b := r.Spawn(k, 0, "")
			b.Pos = sim.V(0, -d)
			b.State, b.Path, b.Wait = sim.RWait, nil, 100
			tt.g.bots = map[int]*robot{}
			resetBatches()
			tt.g.drawRobots()
			n0 := len(bEnv.v) / vstride
			t0 := time.Now()
			for i := 0; i < 50; i++ {
				resetBatches()
				tt.g.drawRobots()
			}
			t.Logf("kind %d at %4.1f m: %6d verts, %.3f ms", k, d, n0, float64(time.Since(t0).Microseconds())/1000/50)
		}
	}
}
