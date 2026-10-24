package main

// The floor. Some runtimes work out where the floor is from wherever the headset was when
// the game started - so start it with the headset lying on the desk and the floor ends up
// the desk's height too low, and you're eight feet tall. Nobody's eyes are 2.15 m up, so a
// head that's that high for a second gets the floor put back under your feet. And SET
// FLOOR in the pause menu: put a controller on the floor and pull its trigger.

import (
	"boundaryvr/sim"
	"log"
	"math"
)

// floorOff: how far below your real floor the runtime's is (taken off every tracked height)
var floorOff float64

// focusSeq: counts the times the headset's been put on (the session gaining focus)
var focusSeq int

const (
	tooTall  = 2.15 // eyes higher than this: the floor's wrong
	tooShort = 1.1  // eyes lower than this just after the headset goes on: the floor's wrong
	onWindow = 8.0  // seconds after it goes on that a too-low head counts
	eyeGuess = 1.65 // where to put them when it is
	padOnFlr = 0.05 // a controller lying on the floor: its grip this far up
)

// floorAuto: watching for a head at a height nobody's is (both ZoneTweak and the game).
type floorAuto struct {
	t, highT, lowT, focusT float64
	lastHead               vec3
	focusSeen              int
}

var autoFloor floorAuto

// step: a frame of watching; standing says you're surely on your feet (the game's setup).
// Returns whether it moved the floor.
func (a *floorAuto) step(hp vec3, dt float64, standing bool) bool {
	a.t += dt
	moving := hp.sub(a.lastHead).length() > 0.0005 // on a head, not lying still on a desk
	a.lastHead = hp
	if hp.y > tooTall && moving {
		a.highT += dt
	} else if hp.y <= tooTall {
		a.highT = 0
	}
	// just after the headset goes on you're standing: eyes that low mean the floor's too high
	if focusSeq != a.focusSeen {
		a.focusSeen, a.focusT = focusSeq, a.t
	}
	early := a.t-a.focusT < onWindow || standing
	if hp.y < tooShort && hp.y > -1 && moving && early {
		a.lowT += dt
	} else if hp.y >= tooShort || !early {
		a.lowT = 0
	}
	if a.lowT > 1.5 {
		d := hp.y - eyeGuess
		floorOff += d
		a.lowT = 0
		log.Printf("floor: head only %.2f m up - too short; floor moved down %.2f m (offset %.2f)", hp.y, -d, floorOff)
		return true
	}
	if a.highT > 1.0 {
		d := hp.y - eyeGuess
		floorOff += d
		a.highT = 0
		log.Printf("floor: head %.2f m up - too tall; floor moved up %.2f m (offset %.2f)", hp.y, d, floorOff)
		return true
	}
	return false
}

func (g *game) floorFrame(dt float64) {
	if autoFloor.step(ed.hmd.pos(), dt, g.r.Phase == sim.Setup) {
		g.say("FLOOR ADJUSTED", "NOT RIGHT? PAUSE MENU > SET FLOOR", cBeam, 4)
	}
	if !g.settingFloor {
		return
	}
	for _, gh := range hands {
		if gh.valid() && gh.trigDown {
			y := gh.h.m.pos().y
			floorOff += y - padOnFlr
			g.settingFloor = false
			log.Printf("floor: set by a controller at %.2f m (offset %.2f)", y, floorOff)
			g.say("FLOOR SET", "", cEnergy, 2)
			gh.buzz(60, 0.8)
			gh.trigDown = false // that pull was for the floor, nothing else
			return
		}
	}
}

// startSetFloor: from the pause menu.
func (g *game) startSetFloor() {
	g.menuOpen = false
	g.settingFloor = true
	g.say("SET THE FLOOR", "PUT A CONTROLLER ON THE FLOOR AND PULL ITS TRIGGER", cBeam, 3600)
}

var _ = math.Abs
