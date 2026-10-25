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

// ZoneTweak's roaming drones go about the hall at head height without ever getting near
// your walls and generators, each other or a robot (they go over the top), and come and go
// through the doors.
func TestZoneDrones(t *testing.T) {
	simClockOn, simClock = true, 0
	saveR := robots
	defer func() { simClockOn = false; zDrones = nil; robots = saveR }()
	zDrones = nil
	P := []pt2{{-1.5, -1.2}, {1.5, -1.2}, {1.5, 1.2}, {-1.5, 1.2}}
	ed.pts, ed.orig, ed.height = clonePts(P), clonePts(P), 2.4
	ed.drawing = false
	resetEngineForZone()
	ed.buildEnvironment()
	if len(world.doors) == 0 {
		t.Fatal("no doors")
	}
	// a robot standing in the hall by a door, in the drones' way
	bot := &robot{pos: world.doors[0].mouth, gen: -1}
	robots = []*robot{bot}
	trips, wasAway := 0, make([]bool, zDroneN)
	low, flown := 0, 0.0
	last := map[*zDrone]pt2{}
	for k := 0; k < 90*240; k++ {
		simClock += 1.0 / 90
		ed.dronesFrame(1.0 / 90)
		for i, d := range zDrones {
			if p, ok := last[d]; ok && !d.away && dist2(p, d.pos) < 0.5 {
				flown += dist2(p, d.pos)
			}
			last[d] = d.pos
			if d.away {
				wasAway[i] = true
				continue
			}
			if wasAway[i] {
				trips++
				wasAway[i] = false
			}
			if d.h < 1.8 {
				low++
			}
			if d.h < ed.height+0.3 && !ed.droneClear(d.pos) {
				t.Fatalf("drone %d at %.2f m, in among your walls (%.2f from the boundary)", i, d.h, outsideDist(P, d.pos))
			}
			if dist2(d.pos, bot.pos) < 0.8 && d.h < 2.6 {
				t.Fatalf("drone %d flew into the robot (%.2f away at %.2f m)", i, dist2(d.pos, bot.pos), d.h)
			}
			for j, o := range zDrones {
				if j > i && !o.away && dist2(d.pos, o.pos) < 0.6 && math.Abs(d.h-o.h) < 0.4 {
					t.Fatalf("drones %d and %d ran into each other at %.1f s: %v h %.2f path %d hold? / %v h %.2f path %d", i, j, simClock, d.pos, d.h, len(d.path), o.pos, o.h, len(o.path))
				}
			}
		}
	}
	if trips < 3 {
		t.Fatalf("only %d trips in through a door in four minutes", trips)
	}
	if flown < 30 {
		t.Fatalf("they should get about (flew %.0f m between them in four minutes)", flown)
	}
	if low == 0 {
		t.Fatal("they should mostly cruise at head height")
	}
}
