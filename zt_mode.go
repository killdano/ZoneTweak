package main

// Switching between ZoneTweak and Contraband. The game borrows the engine's shared state
// (the generator list the walls are drawn from, the hall, the lamps, the glow colours...);
// going in, the editor's own state is put aside, and coming back it's put back and the
// engine's set the way the editor draws.

import "math"

// wantMode: a switch asked for this frame (-1: none), done between frames by the main loop.
var wantMode = -1

// onLeave: the game's pause menu asks to go back to ZoneTweak (the VR build sets it);
// canLeave says there's an editor to go back to (there isn't with no boundary files).
var (
	onLeave  func()
	canLeave bool
)

// gloveIn: when the controllers turned into gloves (the game's first moments glint).
var gloveIn = -10.0

type zoneSnap struct {
	pts, orig, draft         []pt2
	undo, redo               [][]pt2
	height, theta            float64
	dirty                    bool
	drawing, matching, areaM bool
}

func takeZoneSnap() zoneSnap {
	e := ed
	s := zoneSnap{pts: clonePts(e.pts), orig: clonePts(e.orig), draft: clonePts(e.draft),
		undo: e.undo, redo: e.redo, height: e.height, theta: e.theta, dirty: e.dirty,
		drawing: e.drawing, matching: e.matching, areaM: e.areaMode}
	e.menuOpen, e.drawing, e.matching, e.areaMode = false, false, false, false
	e.releaseAll()
	zLever.by, zLever.fired = nil, false
	audioSet(0, 0, 0, 0, 0)
	audioRobots(0, 0, 0, 0)
	return s
}

func (s zoneSnap) restore() {
	e := ed
	e.pts, e.orig, e.draft = s.pts, s.orig, s.draft
	e.undo, e.redo = s.undo, s.redo
	e.height, e.theta, e.dirty = s.height, s.theta, s.dirty
	e.drawing, e.matching, e.areaMode = s.drawing, s.matching, s.areaM
	e.releaseAll()
	e.menuOpen = false
}

// resetEngineForZone: the engine back the way the editor draws (after the game).
func resetEngineForZone() {
	envHave, envKey = false, [8]float64{}
	envDoorsOn = false
	envDoorAt = [4]float64{math.NaN(), math.NaN(), math.NaN(), math.NaN()}
	envLampC, envLampK, envLampTint, envH = pt2{}, 1, rgba{1, 1, 1, 1}, 5.2
	glowCol, glowHot, glowLite = cEnergy, cEnergyHot, false
	sidePowerHook = nil
	robotCrowdK = 1
	lodPlainBox, lodNoTrim = false, false
	bladeExt, bladeExtN, bladeExtPos = map[int]float64{}, 0, nil
	if eng != nil {
		eng.motorSet(0, 0, 0)
		eng.stopClips()
	}
	audioSet(0, 0, 0, 0, 0)
	audioRobots(0, 0, 0, 0)
	for _, gh := range hands {
		if gh != nil {
			gh.held, gh.hover, gh.building, gh.beam = nil, nil, false, false
		}
	}
	zLever.placed = nil // stand it again for the boundary as it is
}
