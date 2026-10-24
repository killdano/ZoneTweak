package main

// Tracing your room by hand, for headsets that don't hand the game a usable boundary (a Quest
// over Link, say, which only gives a rectangle at best). At the setup console, TRACE ROOM:
// point at the floor and pull the trigger at each corner of your play area - your headset's
// own boundary shows you where it is as you get near it - and point back at the first corner
// to close it. B takes the last corner back. The room is saved beside the game and used next
// time on this headset; HEADSET ROOM goes back to the headset's own boundary.

import (
	"boundaryvr/sim"
	"math"
)

type traceKit struct {
	on  bool
	pts []pt2 // corners so far (the game's world)
}

// roomSource: where the room came from ("headset", "rectangle", "traced", "default").
var roomSource = "headset"

// onTraced: the VR build saves a traced room and rebuilds the game in it (nil: just rebuild).
var onTraced func(pts []pt2)

// onHeadsetRoom: the VR build forgets the traced room and goes back to the headset's.
var onHeadsetRoom func()

const traceClose = 0.3 // pointing this near the first corner closes the loop

func (g *game) traceToggle() {
	if g.r.Phase != sim.Setup {
		return
	}
	g.trace.on, g.trace.pts = !g.trace.on, nil
	if g.trace.on {
		g.say("TRACE YOUR ROOM", "POINT AT THE FLOOR, PULL THE TRIGGER AT EACH CORNER", cBeam, 5)
	}
}

// traceInput: a trigger pull on the floor (or B) while tracing. Returns whether it took it.
func (g *game) traceInput(gh *gHand) bool {
	k := &g.trace
	if !k.on {
		return false
	}
	if gh.bDown {
		gh.bOnWall = true // (not the menu)
		if n := len(k.pts); n > 0 {
			k.pts = k.pts[:n-1]
			ed.sfx(sndTick, gh.aim.pos(), 0.5)
		}
		return true
	}
	if !gh.trigDown || gh.hit.kind != 3 {
		return gh.hit.kind == 3
	}
	p := pt2{gh.hit.p.x, gh.hit.p.z}
	if n := len(k.pts); n >= 3 && dist2(p, k.pts[0]) < traceClose {
		g.traceDone()
		return true
	}
	if n := len(k.pts); n > 0 && dist2(p, k.pts[n-1]) < 0.15 {
		return true // the same corner twice
	}
	k.pts = append(k.pts, p)
	gh.buzz(25, 0.6)
	ed.sfx(sndTelescopeID, fl(p, 0.1), 0.4)
	return true
}

func (g *game) traceDone() {
	k := &g.trace
	P := k.pts
	k.on, k.pts = false, nil
	if math.Abs(polyArea(P)) < 2 {
		g.say("TOO SMALL", "A ROOM NEEDS AT LEAST 2 SQUARE METRES", cDanger, 3)
		return
	}
	ed.sfx(sndPowerUp, ed.hmd.pos(), 0.6)
	if onTraced != nil {
		onTraced(P)
		return
	}
	roomSource = "traced"
	G = newGame(P, g.seed+1)
}

func (g *game) drawTrace() {
	k := &g.trace
	if !k.on {
		return
	}
	t := fxTime()
	for i, p := range k.pts {
		c := cBeam
		if i == 0 && len(k.pts) >= 3 {
			c = cEnergy // point back here to close it
			ringSoft(fl(p, 0.01), traceClose, 0.015, c.alpha(float32(0.4+0.3*math.Sin(t*5))), 32)
		}
		useMat(mSteel)
		cyl(&bEnv, fl(p, 0), fl(p, 0.5), 0.02, 0.02, 10, cSteelEnv, true)
		sprite(fl(p, 0.52), 0.06, c.alpha(0.8))
		if i > 0 {
			glowLine(fl(k.pts[i-1], 0.05), fl(p, 0.05), cBeam, 1)
		}
	}
	// where the next corner would go
	for _, gh := range hands {
		if gh.valid() && gh.beam && gh.hit.kind == 3 {
			p := pt2{gh.hit.p.x, gh.hit.p.z}
			ringSoft(fl(p, 0.01), 0.08, 0.02, cBeam.alpha(0.8), 24)
			if n := len(k.pts); n > 0 {
				glowLine(fl(k.pts[n-1], 0.05), fl(p, 0.05), cBeam.alpha(0.4), 0.6)
			}
		}
	}
	cam := ed.hmd.pos()
	q := cam.add(ed.hmd.axis(2).mul(-0.9)).add(v3(0, -0.25, 0))
	rr, uu := billboard(cam, q)
	msg := "POINT AT THE FLOOR AT EACH CORNER OF YOUR PLAY AREA - PULL THE TRIGGER"
	if len(k.pts) >= 3 {
		msg = "POINT BACK AT THE FIRST CORNER TO FINISH"
	}
	drawTextCentered(&bOpaque, msg, q, rr, uu, 0.0011, cBeam)
	drawTextCentered(&bOpaque, "B: TAKE BACK A CORNER    CONSOLE: CANCEL", q.add(v3(0, -0.035, 0)), rr, uu, 0.0009, cMuted)
}

// roomSourceName: where the room came from, for the console.
func roomSourceName() string {
	switch roomSource {
	case "traced":
		return "TRACED"
	case "rectangle":
		return "A RECTANGLE: TRACE IT"
	case "default":
		return "A GUESS: TRACE IT"
	case "edited":
		return "ZONETWEAK'S (UNSAVED)"
	}
	if canLeave {
		return "ZONETWEAK'S BOUNDARY"
	}
	return "FROM YOUR HEADSET"
}

// boundsCandidates: different readings of the headset's boundary, drawn lettered on the floor
// during setup so a playtest can say which one matches the real room.
var boundsCandidates map[string][]pt2

func (g *game) drawCandidates() {
	if len(boundsCandidates) == 0 || g.r.Phase != sim.Setup {
		return
	}
	cols := map[string]rgba{"A": cEnergy, "B": cAmber, "C": cMagenta, "D": cBeam, "E": cWhite, "F": cDanger}
	cam := ed.hmd.pos()
	for k, P := range boundsCandidates {
		c := cols[k]
		for i := range P {
			a, b := P[i], P[(i+1)%len(P)]
			glowLine(fl(a, 0.03), fl(b, 0.03), c, 1.5)
			for _, u := range []float64{0.25, 0.75} {
				q := fl(lerp2(a, b, u), 0.35)
				rr, uu := billboard(cam, q)
				drawTextCentered(&bOpaque, k, q, rr, uu, 0.004, c)
			}
		}
	}
}
