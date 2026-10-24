package main

import (
	"math"
	"testing"
)

// Into the game and back: the editor's boundary, history and state come back as they were,
// and the engine draws the editor's way again (no game doors, lamps or glow colours left).
func TestZoneSnapRoundTrip(t *testing.T) {
	P := []pt2{{-1.5, -1}, {1.5, -1}, {1.5, 1}, {-1.5, 1}}
	ed.pts, ed.orig, ed.height = clonePts(P), clonePts(P), 2.4
	ed.undo = [][]pt2{clonePts(P)}
	ed.dirty = true
	s := takeZoneSnap()
	G = newGame(clonePts(P), 7)
	if samePts(ed.pts, P) {
		t.Fatal("the game should have put its generators in the engine's list")
	}
	if !envDoorsOn {
		t.Fatal("the game's hall should be up")
	}
	s.restore()
	resetEngineForZone()
	G = nil
	if !samePts(ed.pts, P) || ed.height != 2.4 || len(ed.undo) != 1 || !ed.dirty {
		t.Fatalf("the editor's state should be back (%d pts, height %.2f)", len(ed.pts), ed.height)
	}
	if envDoorsOn || envHave || sidePowerHook != nil || glowCol != cEnergy || envLampK != 1 || bladeExtN != 0 {
		t.Fatal("the engine should be back to drawing the editor's way")
	}
}

// Switching modes fades to black, switches once it's black, and fades back in.
func TestModeFade(t *testing.T) {
	defer func() { screenFade, fadeDir, fadeTo, fadeHold = 0, 0, -1, 0 }()
	startFade(modeGame)
	switched, at := -1, 0.0
	for k := 0; k < 200 && (fadeDir != 0 || switched < 0); k++ {
		if m := fadeStep(1.0 / 90); m >= 0 {
			switched, at = m, screenFade
		}
	}
	if switched != modeGame || at != 1 {
		t.Fatalf("it should switch to the game at full black (switched %d at %.2f)", switched, at)
	}
	if screenFade != 0 || fadeDir != 0 {
		t.Fatalf("and fade back in afterwards (fade %.2f)", screenFade)
	}
}

func samePts(a, b []pt2) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ZoneTweak's roaming drones keep over the generators' tops, inside the cell, clear of the
// lamps and apart from each other.
func TestZoneDrones(t *testing.T) {
	simClockOn, simClock = true, 0
	defer func() { simClockOn = false; zDrones = nil }()
	zDrones = nil
	P := []pt2{{-1.5, -1.2}, {1.5, -1.2}, {1.5, 1.2}, {-1.5, 1.2}}
	ed.pts, ed.orig, ed.height = clonePts(P), clonePts(P), 2.4
	ed.drawing = false
	resetEngineForZone()
	ed.buildEnvironment()
	lo, _ := ed.droneHeights()
	worst := math.Inf(1)
	for k := 0; k < 90*120; k++ {
		simClock += 1.0 / 90
		ed.dronesFrame(1.0 / 90)
		for i, d := range zDrones {
			if d.h < lo-0.15 || d.h < ed.height+0.4 {
				t.Fatalf("drone %d down at %.2f m, among the generators", i, d.h)
			}
			q := envG.inv().apply(d.pos)
			if q[0] < envBox[0] || q[0] > envBox[1] || q[1] < envBox[2] || q[1] > envBox[3] {
				t.Fatalf("drone %d out through the cell's walls", i)
			}
			for j, o := range zDrones {
				if j > i {
					worst = math.Min(worst, math.Hypot(dist2(d.pos, o.pos), d.h-o.h))
				}
			}
		}
	}
	if len(zDrones) != zDroneN || worst < 0.5 {
		t.Fatalf("%d drones, closest pair %.2f m apart", len(zDrones), worst)
	}
}
