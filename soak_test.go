//go:build !windows

package main

import (
	"boundaryvr/sim"
	"os"
	"testing"
)

// SOAK=1: a whole round through the per-frame game code (hands in view, bot playing the
// rules), watching for panics anywhere in the frame or the scene build.
func TestSoakRound(t *testing.T) {
	if os.Getenv("SOAK") == "" {
		t.Skip("set SOAK=1")
	}
	rooms := [][2]float64{{10, 10}, {7, 7}, {16, 16}}
	if os.Getenv("SOAK") == "chap" {
		rooms = [][2]float64{{0, 0}} // the captured real boundary
	}
	for _, room := range rooms {
		tt := newTester(t, false)
		w, d := room[0]/3.28084, room[1]/3.28084
		poly := []pt2{{-w / 2, -d / 2}, {w / 2, -d / 2}, {w / 2, d / 2}, {-w / 2, d / 2}}
		if room[0] == 0 {
			os.Chdir("preview")
			os.Setenv("ROOM", "chap")
			poly = previewRoom()
			os.Chdir("..")
		}
		G = newGame(poly, 11)
		tt.g = G
		bot := sim.NewBot(G.r, 5)
		bot.BuildMaze(0.8)
		G.auto = bot
		G.startRound()
		tt.hs[1].trig = 0.3 // a beam out the whole time
		n := 0
		for G.r.Phase == sim.Playing && G.r.Time < 420 {
			tt.frames(1)
			n++
			if n%600 == 0 {
				// wave the right hand about so the beam sweeps walls and panels
				tt.pose(1, v3(0.3, 1.3, 0), v3(float64(n%7)-3, -1, float64(n%5)-2).norm(), v3(0, 1, 0))
			}
		}
		tt.frames(120) // the results board and its lamps
		t.Logf("%vx%v: %d frames, phase %d at %.0f s, robots %d, stats %+v", room[0], room[1], n, G.r.Phase, G.r.Time, len(G.r.Robots), G.r.Stats)
	}
}
