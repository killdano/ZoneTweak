package main

// The way into Contraband from ZoneTweak: a big red lever on a post just inside your play
// space, under a sign that says PLAY CONTRABAND. Grab it (the grip or the trigger, with the
// controller on the handle) and haul it toward you and down: your controllers turn into
// gloves and the game starts in the room you've just shaped. The pause menu brings you back.

import "math"

type zoneLever struct {
	l      lever
	at     pt2   // where the post stands
	face   pt2   // which way its front faces (into the room)
	placed []pt2 // the boundary it was placed for
	by     *hand // the hand on it
	firedT float64
	fired  bool
}

var zLever zoneLever

// onPlay: the VR build starts the game (the lever's been pulled).
var onPlay func()

const (
	zlPivotY = 1.0  // the lever's pivot height
	zlArm    = 0.3  // pivot to handle
	zlGrab   = 0.13 // a controller this near the handle can take it
	zlInset  = 0.5  // how far inside the boundary the post stands
)

// leverSpot: where the post goes for this boundary - half a metre in from the middle of the
// longest wall it fits by, facing into the room.
func leverSpot(P []pt2) (pt2, pt2, bool) {
	n := len(P)
	if n < 3 {
		return pt2{}, pt2{}, false
	}
	best, bestL := -1, 0.0
	var at, face pt2
	for i := range P {
		a, b := P[i], P[(i+1)%n]
		L := dist2(a, b)
		if L < 0.6 || L <= bestL {
			continue
		}
		m := lerp2(a, b, 0.5)
		d := pt2{(b[0] - a[0]) / L, (b[1] - a[1]) / L}
		nrm := pt2{-d[1], d[0]}
		if !pointInPoly(P, m[0]+nrm[0]*0.1, m[1]+nrm[1]*0.1) {
			nrm = pt2{d[1], -d[0]}
		}
		q := pt2{m[0] + nrm[0]*zlInset, m[1] + nrm[1]*zlInset}
		if !pointInPoly(P, q[0], q[1]) || distToBoundary(P, q) < zlInset*0.6 {
			continue
		}
		best, bestL, at, face = i, L, q, nrm
	}
	return at, face, best >= 0
}

// place: (re)stand the post for the boundary as it is now, unless you're in the middle of
// moving something (it waits till you let go).
func (z *zoneLever) place(P []pt2, busy bool) {
	if busy || z.by != nil || samePts(P, z.placed) {
		return
	}
	z.placed = clonePts(P)
	at, face, ok := leverSpot(P)
	if !ok {
		z.placed = nil // nowhere it fits: no lever until the boundary changes
		z.at, z.face = pt2{}, pt2{}
		return
	}
	z.at, z.face = at, face
	n := v3(face[0], 0, face[1])
	right := worldUp.cross(n).norm()
	z.l = lever{piv: fl(at, zlPivotY).add(n.mul(0.06)), ax: right, d0: worldUp, L: zlArm, a0: -0.15, a1: 2.0, ang: -0.15, atEnd: 0}
}

func (z *zoneLever) ok() bool { return z.face != (pt2{}) }

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

// handle: a controller working the lever. Returns whether it took this frame's input.
func (z *zoneLever) handle(h *hand) bool {
	if !z.ok() || z.fired {
		return false
	}
	l := &z.l
	p := h.m.pos()
	if z.by == nil {
		if p.sub(l.handle()).length() < zlGrab && (h.down(bitTrigger) || h.down(bitGrab)) {
			z.by = h
			haptic(h.idx, 1500)
			ed.sfx(sndTick, l.handle(), 0.5)
			return true
		}
		return false
	}
	if z.by != h {
		return false
	}
	if !h.held(bitTrigger) && !h.held(bitGrab) {
		z.by = nil
		// let go part way: it springs back up
		l.ang = l.a0
		return true
	}
	v := projPerp(p.sub(l.piv), l.ax)
	if v.length() > 1e-4 {
		was := l.ang
		l.ang = math.Max(l.a0, math.Min(l.a1, signedAngle(l.d0, v.norm(), l.ax)))
		if math.Floor(was*6) != math.Floor(l.ang*6) {
			haptic(h.idx, 400)
			ed.sfx(sndTick, l.handle(), 0.25)
		}
	}
	if l.ang >= l.a1-0.05 {
		z.fired, z.firedT, z.by = true, fxTime(), nil
		haptic(h.idx, 4000)
		ed.sfx(sndRelay, l.handle(), 0.7)
		ed.sfx(sndPowerUp, l.handle(), 0.8)
	}
	return true
}

// frame: after the pull, a beat for the flash, then the game.
func (z *zoneLever) frame() {
	if z.fired && fxTime()-z.firedT > 0.7 {
		z.fired = false
		z.l.ang = z.l.a0
		if onPlay != nil {
			onPlay()
		}
	}
}

// near: a controller close enough to take it.
func (z *zoneLever) near(hs []*hand) bool {
	for _, h := range hs {
		if h.valid && h.m.pos().sub(z.l.handle()).length() < zlGrab {
			return true
		}
	}
	return false
}

func (z *zoneLever) draw(hs []*hand) {
	if !z.ok() {
		return
	}
	t := fxTime()
	l := &z.l
	n := v3(z.face[0], 0, z.face[1])
	R, U := l.ax, worldUp
	base := fl(z.at, 0)
	defer cleanRoles()()
	clean := material{0, 0.2, 0.5, 0.03} // fresh paint, no rust
	// the post on its floor plate, running on up to the sign
	useMat(mCast)
	bevelBox(&bEnv, base.add(v3(0, 0.012, 0)), R, U, n, 0.16, 0.012, 0.16, 0.005, cIron)
	useMat(clean)
	cyl(&bEnv, base.add(v3(0, 0.02, 0)), fl(z.at, zlPivotY-0.12), 0.045, 0.04, 14, rgba{0.22, 0.23, 0.24, 1}, true)
	useMat(mSteel)
	cyl(&bEnv, fl(z.at, zlPivotY-0.12).add(n.mul(-0.04)), fl(z.at, 1.6).add(n.mul(-0.04)), 0.018, 0.018, 10, cSteelEnv, true)
	// the lever's housing: a plate with hazard stripes, the ears holding the pivot pin
	at := func(x, y, zz float64) vec3 { return l.piv.add(R.mul(x)).add(U.mul(y)).add(n.mul(zz)) }
	useMat(clean)
	bevelBox(&bEnv, at(0, -0.05, -0.05), R, U, n, 0.1, 0.13, 0.03, 0.006, rgba{0.2, 0.21, 0.2, 1})
	useMat(mHazard)
	bevelBox(&bEnv, at(0, -0.155, -0.019), R, U, n, 0.1, 0.02, 0.002, 0.001, rgba{0.8, 0.6, 0.1, 1})
	useMat(mCast)
	for _, sx := range []float64{-1, 1} {
		bevelBox(&bEnv, at(sx*0.045, 0, -0.01), R, U, n, 0.008, 0.045, 0.035, 0.003, cIron)
	}
	cyl(&bEnv, at(-0.056, 0, 0), at(0.056, 0, 0), 0.012, 0.012, 10, cSteelEnv, true)
	// how far it's been pulled, glowing down the ear
	for k := 0; k < 16; k++ {
		a0 := l.a0 + (l.a1-l.a0)*float64(k)/16
		a1 := l.a0 + (l.a1-l.a0)*float64(k+1)/16
		c := cMuted.alpha(0.5)
		if a1 <= l.ang+0.01 {
			c = cDanger
		}
		q0 := l.piv.add(rotAbout(l.d0, l.ax, a0).mul(0.2)).add(R.mul(-0.06))
		q1 := l.piv.add(rotAbout(l.d0, l.ax, a1).mul(0.2)).add(R.mul(-0.06))
		bGlowLine.line(q0, q1, c)
	}
	// the arm and its fat red handle
	hp := l.handle()
	useMat(mSteel)
	cyl(&bEnv, l.piv, hp, 0.014, 0.012, 10, cSteelEnv, false)
	useMat(mRubber)
	cyl(&bEnv, hp.sub(R.mul(0.075)), hp.add(R.mul(0.075)), 0.024, 0.024, 12, cDanger.scale(0.8), true)
	if z.by == nil {
		k := 0.25 + 0.2*math.Sin(t*4)
		if z.near(hs) {
			k = 0.7
			ringSoftAxes(hp, R, n, 0.09, 0.008, cDanger.alpha(0.8), 24)
		}
		sprite(hp, 0.11, cDanger.alpha(float32(k)))
	}
	addPoint(hp.add(n.mul(0.1)), cDanger, 0.6, 1.2)
	faceText(&bOpaque, "PULL", at(0, -0.1, -0.019), n, U, 0.0022, cWhite)
	// the sign on top of the post
	sc := fl(z.at, 1.75).add(n.mul(-0.04))
	useMat(clean)
	bevelBox(&bEnv, sc, R, U, n, 0.4, 0.15, 0.015, 0.008, rgba{0.05, 0.05, 0.06, 1})
	f := sc.add(n.mul(0.017))
	flick := float32(0.85 + 0.15*noise1(3, t*6))
	faceText(&bOpaque, "PLAY", f.add(U.mul(0.085)), n, U, 0.0042, cAmber.alpha(flick))
	faceText(&bOpaque, "CONTRABAND", f.add(U.mul(0.02)), n, U, 0.0062, cDanger.alpha(flick))
	faceText(&bOpaque, "TWEAK YOUR ZONE. THEN DEFEND IT.", f.add(U.mul(-0.055)), n, U, 0.0019, cWhite.alpha(0.75))
	faceText(&bOpaque, "GRAB THE LEVER, PULL IT DOWN", f.add(U.mul(-0.1)), n, U, 0.0019, cAmber.alpha(0.9))
	// a strip of light round its edge and a glow on the floor in front
	for _, sy := range []float64{-1, 1} {
		a, b := f.add(R.mul(-0.39)).add(U.mul(sy*0.14)), f.add(R.mul(0.39)).add(U.mul(sy*0.14))
		bGlowLine.line(a, b, cDanger.alpha(0.6*flick))
	}
	addPoint(f.add(n.mul(0.25)), cAmber, 1.2*float64(flick), 2)
	// the pull: a flash
	if z.fired {
		k := 1 - math.Min(1, (t-z.firedT)/0.7)
		cam := ed.hmd.pos()
		sprite(cam.add(ed.hmd.axis(2).mul(-0.3)), 2, rgba{1, 0.95, 0.9, float32(0.9 * (1 - k*k))})
		addPoint(hp, cWhiteHot, 30*k, 6)
	}
}
