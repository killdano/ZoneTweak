package main

// Switching between ZoneTweak and Contraband. The game borrows the engine's shared state
// (the generator list the walls are drawn from, the hall, the lamps, the glow colours...);
// going in, the editor's own state is put aside, and coming back it's put back and the
// engine's set the way the editor draws.

import "math"

const (
	modeZone = iota // ZoneTweak, the editor
	modeGame        // Contraband
)

var appMode = modeZone

// onPlay: ZoneTweak's PLAY CONTRABAND (the VR build fades to black and starts the game).
var onPlay func()

// screenFade: how black the picture is (0 clear .. 1 black; the final pass darkens by it).
// Switching modes fades to black, swaps everything over, and fades back in.
var screenFade float64

var (
	fadeTo   = -1 // the mode it's fading out to (-1: not switching)
	fadeDir  int  // +1 going black, -1 coming back
	fadeHold int  // frames held at black (the switch can take a frame or two to build)
)

const (
	fadeOut = 0.45 // seconds to black
	fadeIn  = 0.6  // seconds back
)

// startFade: fade out and switch to mode m (ignored while a switch is under way).
func startFade(m int) {
	if fadeDir != 0 {
		return
	}
	fadeTo, fadeDir = m, 1
}

// fadeStep: a frame of the fade. Returns the mode to switch to now (at black), or -1.
func fadeStep(dt float64) int {
	switch fadeDir {
	case 1:
		screenFade = math.Min(1, screenFade+dt/fadeOut)
		if screenFade >= 1 {
			fadeHold++
			if fadeHold >= 2 {
				m := fadeTo
				fadeTo, fadeDir, fadeHold = -1, -1, 0
				return m
			}
		}
	case -1:
		screenFade = math.Max(0, screenFade-dt/fadeIn)
		if screenFade <= 0 {
			fadeDir = 0
		}
	}
	return -1
}

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
}
