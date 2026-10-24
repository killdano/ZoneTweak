package main

import "testing"

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
