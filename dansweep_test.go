//go:build !windows

package main

import (
	"boundaryvr/sim"
	"fmt"
	"os"
	"testing"
)

// DANSWEEP=1: win rate in the captured real room over a grid of drain and pressure.
func TestDanRoomSweep(t *testing.T) {
	if os.Getenv("DANSWEEP") == "" {
		t.Skip()
	}
	os.Chdir("preview")
	os.Setenv("ROOM", "chap")
	poly := previewRoom()
	os.Chdir("..")
	var R []sim.V2
	for _, p := range poly {
		R = append(R, sv(p))
	}
	for _, dr := range []float64{0.8, 1.0, 1.2} {
		for _, pr := range []float64{0.35, 0.45, 0.55} {
			tu := sim.DefaultTuning()
			tu.Drain, tu.Pressure = dr, pr
			line := fmt.Sprintf("drain %.1f pressure %.2f:", dr, pr)
			for _, n := range []int{8, 10} {
				w, blown, steals := 0, 0, 0
				const seeds = 14
				for s := 1; s <= seeds; s++ {
					r := sim.NewRound(R, int64(s*1000+n), tu, nil)
					r.SetGens(n)
					b := sim.NewBot(r, int64(s*31+n))
					b.BuildMaze(0.8)
					r.Start()
					for r.Phase == sim.Playing && r.Time < 900 {
						r.Step(0.05)
						b.Step(0.05)
					}
					if r.Phase == sim.Won {
						w++
					}
					blown += r.Stats.GensBlown
					steals += r.Stats.Steals
				}
				line += fmt.Sprintf("  %d gens %3d%% (blown %.1f, steals %.1f)", n, w*100/seeds, float64(blown)/seeds, float64(steals)/seeds)
			}
			t.Log(line)
		}
	}
}
