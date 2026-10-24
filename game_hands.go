package main

// Your hands: armoured gloves (no body), a grab system everything physical plugs into, and
// the laser in each glove that builds the maze and supercharges walls.

import (
	"boundaryvr/sim"
	"log"
	"math"
)

// grabber: anything a glove can take hold of.
type grabber interface {
	gPos() vec3     // where the hand latches on
	gRad() float64  // how close the palm must be
	gOK() bool      // available right now
	gGrab(h *gHand) // just grabbed
	gHold(h *gHand, dt float64)
	gRelease(h *gHand)
}

// poker: a button pressed with the index fingertip.
type poker interface {
	pokeTest(prev, tip vec3) bool // the fingertip went through it this frame
	pokeOK() bool
}

type rayHit struct {
	ok    bool
	t     float64
	p     vec3
	kind  int // 1 green span, 2 magenta wall, 3 floor, 4 panel
	a, b  int // generators of a span
	id    int // magenta wall id
	panel *uiPanel
	btn   int
}

type gHand struct {
	h          *hand
	side       float64 // -1 left, +1 right
	grip, trig float64
	closed     bool // fist closed (with hysteresis)
	grabDown   bool
	grabUp     bool
	trigDown   bool
	trigUp     bool
	trigHeld   bool
	aDown      bool
	aHeld      bool
	bDown      bool // B: magenta walls (and, pressed at nothing, the menu)
	bHeld      bool
	bUp        bool
	bOnWall    bool // this press of B started on a magenta wall
	held       grabber
	hover      grabber
	palm, tip  vec3
	prevPalm   vec3
	prevTip    vec3
	vel        vec3
	holdT      float64
	// laser
	beam     bool
	hit      rayHit // where the beam lands: a panel or the floor
	wallHit  rayHit // the wall the beam picks out: green (for A) or magenta (for B)
	building bool
	bA       pt2
	aT       float64 // A held on a green span or the turret (charging; -1: done)
	bT       float64 // B held on a magenta wall (charging; -1: done)
	erasing  bool    // the trigger held on a magenta wall: it's folding away
	eraseID  int     // ...which wall
	eraseHit rayHit  // the magenta wall the trigger would take down
	wasHit   rayHit
	lostT    float64 // how long the wall being charged has been out of the beam
	buzzT    float64
	// the glove as you see it: the controller's grip pose with your fit applied
	m               hmdMatrix34
	aim             hmdMatrix34 // the laser's pose, riding with the fitted glove
	forceOK         bool        // the controller reports squeeze force (Index)
	fitDrag         int         // fit mode: the other glove this hand has hold of (-1: none)
	fitRel          hmdMatrix34 // that glove's pose relative to this controller when taken
	fitAim          int         // fit mode: the other hand's laser this hand is re-aiming (-1: none)
	pull            pullSt      // force pull: the tool this hand's arc is locked onto
	fore            vec3        // forearm direction (wrist toward elbow), smoothed
	shoulder, elbow vec3        // where the arm was worked out to be
}

var hands [2]*gHand

func initHands(hs []*hand) {
	for i := 0; i < 2 && i < len(hs); i++ {
		side := -1.0
		if i == 1 {
			side = 1
		}
		hands[i] = &gHand{h: hs[i], side: side, fitDrag: -1, fitAim: -1, m: ident34()}
	}
}

func (gh *gHand) valid() bool { return gh != nil && gh.h != nil && gh.h.valid }

// local: a point in the glove's own frame (x mirrored for the left hand) to world space.
func (gh *gHand) local(x, y, z float64) vec3 { return gh.m.xform(v3(x*gh.side, y, z)) }

func (gh *gHand) axes() (vec3, vec3, vec3) {
	return gh.m.axis(0).mul(gh.side), gh.m.axis(1), gh.m.axis(2)
}

// fitGlove: the glove's frame for this frame (controller pose with your fit)
func (gh *gHand) fitGlove() {
	i := gh.h.idx
	if i < 0 || i > 1 {
		i = 0
	}
	gh.m = mul34(gh.h.m, gloveFit[i])
	// the laser comes off the glove, so it moves with your fit too, turned by its own re-aim
	gh.aim = mul34(gh.aimBase(), aimFit[i])
}

// finger joint chain: root, direction, curl for each of 3 segments (radians), lengths
func (gh *gHand) finger(root vec3, curl float64, lens [3]float64) []vec3 {
	X, Y, Z := gh.axes()
	pts := []vec3{root}
	d := Z.mul(-1)
	p := root
	ang := 0.0
	for k, L := range lens {
		ang += curl * []float64{0.9, 1.4, 1.1}[k]
		// curl toward the palm (-X in the glove frame) and back toward the wrist
		dir := d.mul(math.Cos(ang)).add(X.mul(-math.Sin(ang)))
		dir = dir.add(Y.mul(-0.12 * curl)).norm()
		p = p.add(dir.mul(L))
		pts = append(pts, p)
	}
	return pts
}

func (gh *gHand) indexChain() []vec3 {
	return gh.finger(gh.local(0.012, 0.024, -0.052), gh.trig*0.95, [3]float64{0.032, 0.024, 0.019})
}

// update: read this frame's buttons and work out palm and fingertip.
func (gh *gHand) update(dt float64) {
	if !gh.valid() {
		return
	}
	h := gh.h
	gh.fitGlove()
	gh.grip, gh.trig = h.grip, h.trig
	if gh.grip == 0 && h.held(bitGrab) {
		gh.grip = 1
	}
	was := gh.closed
	if h.force > 0.02 && !gh.forceOK {
		gh.forceOK = true
		log.Printf("hand %d reports squeeze force: grabbing by squeezing", h.idx)
	}
	switch {
	case gh.forceOK:
		// Index: the squeeze value only says how much of your hand is touching the grip -
		// it sits high just holding the controller, which left hands stuck shut. Grabbing
		// is squeezing.
		if !gh.closed && h.force > 0.2 {
			gh.closed = true
		} else if gh.closed && h.force < 0.08 {
			gh.closed = false
		}
	case !gh.closed && gh.grip > 0.55:
		gh.closed = true
	case gh.closed && gh.grip < 0.35:
		gh.closed = false
	}
	gh.grabDown, gh.grabUp = gh.closed && !was, !gh.closed && was
	gh.trigDown, gh.trigUp, gh.trigHeld = h.down(bitTrigger), !h.held(bitTrigger) && h.prev&(1<<bitTrigger) != 0, h.held(bitTrigger)
	gh.aDown, gh.aHeld = h.down(bitGrip), h.held(bitGrip)
	gh.bDown, gh.bHeld, gh.bUp = h.down(bitMenu), h.held(bitMenu), !h.held(bitMenu) && h.prev&(1<<bitMenu) != 0
	gh.prevPalm, gh.prevTip = gh.palm, gh.tip
	gh.palm = gh.local(0, -0.005, -0.03)
	ch := gh.indexChain()
	gh.tip = ch[len(ch)-1]
	if gh.prevPalm == (vec3{}) {
		gh.prevPalm, gh.prevTip = gh.palm, gh.tip
	}
	if dt > 0 {
		v := gh.palm.sub(gh.prevPalm).mul(1 / dt)
		gh.vel = gh.vel.add(v.sub(gh.vel).mul(math.Min(1, dt*15)))
	}
}

func (gh *gHand) buzz(ms int, amp float32) { hapticAmp(gh.h.idx, ms, amp) }

// ---- grabbing ----

func (g *game) handFrame(dt float64) {
	gs := g.grabbers()
	ps := g.pokers()
	for _, gh := range hands {
		if !gh.valid() {
			continue
		}
		gh.update(dt)
		if gh.held != nil {
			gh.holdT += dt
			if gh.grabUp || !gh.held.gOK() {
				gh.held.gRelease(gh)
				gh.held = nil
				gh.buzz(20, 0.3)
			} else {
				gh.held.gHold(gh, dt)
			}
			continue
		}
		// nearest grabbable within reach of the palm
		var best grabber
		bd := math.Inf(1)
		for _, o := range gs {
			if !o.gOK() || g.heldByOther(o, gh) {
				continue
			}
			d := o.gPos().sub(gh.palm).length()
			if d < o.gRad() && d < bd {
				bd, best = d, o
			}
		}
		if best != gh.hover && best != nil {
			gh.buzz(8, 0.15)
		}
		gh.hover = best
		if gh.grabDown && best != nil {
			gh.held, gh.holdT = best, 0
			gh.hover = nil
			best.gGrab(gh)
			gh.buzz(25, 0.5)
			continue
		}
		// fingertip pokes (with the index finger out)
		if gh.trig < 0.8 {
			for _, p := range ps {
				if p.pokeOK() && p.pokeTest(gh.prevTip, gh.tip) {
					gh.buzz(18, 0.6)
					break
				}
			}
		}
	}
}

func (g *game) heldByOther(o grabber, me *gHand) bool {
	for _, gh := range hands {
		if gh != nil && gh != me && gh.held == o {
			return true
		}
	}
	return false
}

func (g *game) handHolding(o grabber) *gHand {
	for _, gh := range hands {
		if gh != nil && gh.held == o {
			return gh
		}
	}
	return nil
}

// ---- the laser ----

// rayWorld: what a ray from o along d hits first: a panel, a wall (when walls is set) or the floor.
func (g *game) rayWorld(o, d vec3, walls bool) rayHit { return g.rayCast(o, d, walls, true) }

// rayCast: rayWorld with the panels optional (a wall being drawn ignores them).
func (g *game) rayCast(o, d vec3, walls, panels bool) rayHit {
	best := rayHit{t: math.Inf(1)}
	r := g.r
	for _, pn := range g.panels() {
		if !panels {
			break
		}
		if t, btn, ok := pn.ray(o, d); ok && t < best.t {
			best = rayHit{ok: true, t: t, p: o.add(d.mul(t)), kind: 4, panel: pn, btn: btn}
		}
	}
	if walls {
		vq := func(a, b pt2, y0, y1 float64) (float64, bool) {
			d2 := sim.V(d.x, d.z)
			ab := sim.V(b[0]-a[0], b[1]-a[1])
			den := d2.Cross(ab)
			if math.Abs(den) < 1e-9 {
				return 0, false
			}
			q := sim.V(a[0]-o.x, a[1]-o.z)
			t := q.Cross(ab) / den
			u := q.Cross(d2) / den
			if t <= 0 || u < 0 || u > 1 {
				return 0, false
			}
			y := o.y + d.y*t
			return t, y >= y0 && y <= y1
		}
		_ = o
		for _, s := range r.Spans() {
			if t, ok := vq(ed.pts[s[0]], ed.pts[s[1]], 0, genH); ok && t < best.t {
				best = rayHit{ok: true, t: t, p: o.add(d.mul(t)), kind: 1, a: s[0], b: s[1]}
			}
		}
		for _, w := range r.Maze {
			if t, ok := vq(pv(w.A), pv(w.B), 0, r.T.WallHeight); ok && t < best.t {
				best = rayHit{ok: true, t: t, p: o.add(d.mul(t)), kind: 2, id: w.ID}
			}
		}
	}
	if d.y < -0.01 {
		t := -o.y / d.y
		if t > 0 && t < laserReach && t < best.t {
			best = rayHit{ok: true, t: t, p: o.add(d.mul(t)), kind: 3}
		}
	}
	return best
}

// wallTarget: the wall the A button acts on along a beam that lands at land: the first
// magenta wall it passes through, or failing that, the green span it crosses. (From inside
// your room every beam crosses your ring first, so magenta comes first or you could never
// point at it.)
func (g *game) wallTarget(o, d vec3, land rayHit, kind int) rayHit {
	r := g.r
	maxT := laserReach
	if land.ok {
		maxT = land.t
	}
	// pad: how far past each end the beam still counts (metres); top: how high
	cross := func(a, b pt2, pad, top, maxT float64) (float64, bool) {
		d2 := sim.V(d.x, d.z)
		ab := sim.V(b[0]-a[0], b[1]-a[1])
		den := d2.Cross(ab)
		if math.Abs(den) < 1e-9 {
			return 0, false
		}
		q := sim.V(a[0]-o.x, a[1]-o.z)
		t := q.Cross(ab) / den
		u := q.Cross(d2) / den
		e := pad / math.Max(ab.Len(), 1e-6)
		if t <= 0 || t > maxT || u < -e || u > 1+e {
			return 0, false
		}
		y := o.y + d.y*t
		return t, y >= 0 && y <= top
	}
	best := rayHit{t: math.Inf(1)}
	if kind == 2 {
		// magenta walls are easy to pick out: their base stations count, a beam just over
		// the top counts, and so does one that lands on the floor just short of a wall
		mt := maxT
		if land.ok && land.kind == 3 {
			mt = land.t + 0.8
		}
		for _, w := range r.Maze {
			if t, ok := cross(pv(w.A), pv(w.B), 0.35, r.T.WallHeight+0.4, mt); ok && t < best.t {
				best = rayHit{ok: true, t: t, p: o.add(d.mul(t)), kind: 2, id: w.ID}
			}
		}
		if !best.ok && land.ok && land.kind == 3 {
			bd := 0.7
			for _, w := range r.Maze {
				if dd, _ := sim.SegDist(sim.V(land.p.x, land.p.z), w.A, w.B); dd < bd {
					bd = dd
					best = rayHit{ok: true, t: land.t, p: land.p, kind: 2, id: w.ID}
				}
			}
		}
		if !best.ok {
			return rayHit{}
		}
		return best
	}
	for _, sp := range r.Spans() {
		if t, ok := cross(ed.pts[sp[0]], ed.pts[sp[1]], 0.15, genH+0.5, maxT+0.5); ok && t < best.t {
			best = rayHit{ok: true, t: t, p: o.add(d.mul(t)), kind: 1, a: sp[0], b: sp[1]}
		}
	}
	if !best.ok {
		return rayHit{}
	}
	return best
}

// snapEnd: magenta wall ends snap onto existing posts so walls join up.
func (g *game) snapEnd(p pt2) pt2 {
	best, bp := 0.35, p
	for _, w := range g.r.Maze {
		for _, q := range []sim.V2{w.A, w.B} {
			if d := dist2(p, pv(q)); d < best {
				best, bp = d, pv(q)
			}
		}
	}
	return bp
}

func (g *game) laserFrame(dt float64) {
	r := g.r
	// walls you stopped taking down unfold back to full size
	if g.retract == nil {
		g.retract = map[int]float64{}
	}
	for id, k := range g.retract {
		held := false
		for _, gh := range hands {
			held = held || gh != nil && gh.erasing && gh.eraseID == id
		}
		if !held {
			if k -= dt / (eraseSecs * 0.5); k <= 0 {
				delete(g.retract, id)
			} else {
				g.retract[id] = k
			}
		}
	}
	for _, gh := range hands {
		if !gh.valid() {
			continue
		}
		h := gh.h
		gh.beam = false
		if gh.erasing && (!gh.trigHeld || gh.held != nil) {
			gh.erasing = false // let go: the wall unfolds again
		}
		if gh.held != nil || !h.aimValid || (gh.hover != nil && !gh.building) {
			// a hand on something can't draw (a wall half drawn when you grab is dropped)
			gh.building = false
			continue
		}
		o := gh.aim.pos()
		d := gh.aim.axis(2).mul(-1)
		// the beam: the trigger, or a thumb on A or B (it shows what that button will act on
		// before you press it). aT/bT: a button just let go - a tap.
		aiming := gh.trig > 0.12 || gh.trigHeld || gh.aHeld || gh.bHeld || gh.h.aTouch || gh.h.bTouch || gh.aT > 0 || gh.bT > 0 || gh.building
		if !aiming {
			gh.aT = 0
			continue
		}
		gh.beam = true
		// the beam passes through energy walls to the floor (that's where the trigger draws);
		// the first wall it crosses is what the A button works on. A wall being drawn looks
		// straight through the panels.
		gh.hit = g.rayCast(o, d, false, !gh.building)
		gh.wallHit = rayHit{}
		gh.eraseHit = rayHit{}
		if !gh.building && gh.hit.kind != 4 {
			gh.eraseHit = g.eraseTarget(o, d, gh.hit)
		}
		if !gh.building {
			// A acts on the green ring, B on magenta walls: whichever thumb is on its
			// button picks which the beam looks for
			switch {
			case gh.bHeld || gh.h.bTouch || gh.bT > 0:
				gh.wallHit = g.wallTarget(o, d, gh.hit, 2)
			case gh.aHeld || gh.h.aTouch || gh.aT > 0:
				gh.wallHit = g.wallTarget(o, d, gh.hit, 1)
				// a spent harpoon turret takes a supercharge too
				if th := g.turretTarget(o, d); th.ok && (!gh.wallHit.ok || th.t < gh.wallHit.t) {
					gh.wallHit = th
				}
			}
		}
		if gh.bDown {
			gh.bOnWall = gh.wallHit.ok && gh.wallHit.kind == 2
		}
		if gh.hit.kind == 4 && gh.trigDown {
			gh.hit.panel.press(gh.hit.btn)
			gh.buzz(15, 0.5)
			continue
		}
		if gh.hit.kind == 4 {
			continue
		}
		if r.Phase != sim.Setup && r.Phase != sim.Playing || g.fitting || g.menuOpen || g.settingFloor {
			continue
		}
		if g.traceInput(gh) {
			continue
		}
		// the trigger held on a magenta wall: it folds away, and when it's all the way down
		// it's gone and its points are back in your pool (let go early: it unfolds again)
		if gh.erasing {
			w := r.Wall(gh.eraseID)
			if !gh.trigHeld || w == nil {
				gh.erasing = false
			} else {
				k := g.retract[gh.eraseID] + dt/eraseSecs
				g.retract[gh.eraseID] = k
				gh.chargeBuzz(k * superHold)
				if k >= 1 {
					p := fl(lerp2(pv(w.A), pv(w.B), 0.5), 0.3)
					r.RemoveWall(gh.eraseID)
					gh.erasing = false
					gh.buzz(60, 0.8)
					ed.sfx(sndPowerDown, p, 0.6)
					spawnSparks(p, v3(0, 1, 0), 20, 0.5)
				}
			}
			continue
		}
		if gh.trigDown && gh.eraseHit.ok && gh.pull.phase == pullNone {
			gh.erasing, gh.eraseID = true, gh.eraseHit.id
			ed.sfx(sndHydraulic, gh.eraseHit.p, 0.4)
			continue
		}
		// building
		// (not from inside your room: magenta can't go there, so the floor there's not for drawing)
		if gh.trigDown && gh.hit.kind == 3 && gh.pull.phase == pullNone && !sim.InPoly(r.Room, sim.V(gh.hit.p.x, gh.hit.p.z)) {
			gh.building = true
			gh.bA = g.snapEnd(pt2{gh.hit.p.x, gh.hit.p.z})
		}
		if gh.building {
			b := pt2{gh.hit.p.x, gh.hit.p.z}
			if gh.hit.kind == 3 {
				b = g.snapEnd(b)
			}
			if !gh.trigHeld {
				gh.building = false
				if dist2(gh.bA, b) >= r.T.MinWall {
					if _, why := r.PlaceWall(sv(gh.bA), sv(b)); why != "" {
						g.say(why, "", cDanger, 1.6)
						gh.buzz(60, 0.8)
						ed.sfx(sndAlarm, fl(b, 1), 0.25)
					} else {
						gh.buzz(30, 0.6)
						ed.sfx(sndPowerUp, fl(b, 1), 0.5)
						spawnSparks(fl(b, 0.05), v3(0, 1, 0), 10, 0.4)
						g.wallFrom, g.wallFromT = gh.aim.pos(), fxTime()
					}
				}
			}
			continue
		}
		// Supercharging takes a deliberate hold: A on the green ring (or the harpoon turret),
		// B on a magenta wall, held until the charge ring fills. Let go early and nothing's
		// spent. (Taking a magenta wall down is the trigger's job.)
		wh := gh.wallHit
		// a shaky aim loses the wall for a moment: keep charging on the one it was on
		if wh.ok {
			gh.lostT = 0
		} else if gh.wasHit.ok && (gh.aT > 0 || gh.bT > 0) {
			if gh.lostT += dt; gh.lostT < 0.35 {
				wh = gh.wasHit
			}
		}
		if wh.ok && gh.wasHit.ok && (wh.kind != gh.wasHit.kind || wh.id != gh.wasHit.id || wh.a != gh.wasHit.a || wh.b != gh.wasHit.b) {
			// moved onto another wall: start over
			gh.aT, gh.bT = math.Min(gh.aT, 0), math.Min(gh.bT, 0)
		}
		if wh.ok || gh.lostT >= 0.35 {
			gh.wasHit = wh
		}
		playing := r.Phase == sim.Playing
		aOn := wh.ok && (wh.kind == 1 || wh.kind == 5) && playing
		bOn := wh.ok && wh.kind == 2 && gh.bOnWall
		cancel := func(k float64) {
			if k > 0.15 {
				gh.buzz(20, 0.3)
				ed.sfx(sndPowerDown, wh.p, 0.25)
				g.say("CANCELLED", "HOLD IT TILL THE RING FILLS", cMuted, 1)
			}
		}
		recharging := func() {
			gh.buzz(40, 0.3)
			g.say("SUPERCHARGE RECHARGING", fmtSecs(r.SuperCD), cAmber, 1.2)
		}
		// A: charge the green span or the turret
		switch {
		case !gh.aHeld:
			if gh.aT > 0 && gh.aT < superHold {
				cancel(gh.aT)
			}
			gh.aT = 0
		case gh.aT < 0: // done: waiting for you to let go
		case !aOn:
			gh.aT = 0
		case r.SuperCD > 0:
			recharging()
			gh.aT = -1
		default:
			gh.aT += dt
			gh.chargeBuzz(gh.aT)
			if gh.aT >= superHold {
				ok := false
				if wh.kind == 5 {
					ok = r.SuperHarpoon()
				} else {
					ok = r.SuperSpan(wh.a, wh.b)
				}
				if ok {
					gh.buzz(100, 1)
				}
				gh.aT = -1
			}
		}
		// B: charge the magenta wall
		switch {
		case !gh.bHeld:
			if gh.bT > 0 && gh.bT < superHold {
				cancel(gh.bT)
			}
			gh.bT = 0
		case gh.bT < 0:
		case !bOn || !playing:
			gh.bT = 0
		case r.SuperCD > 0:
			recharging()
			gh.bT = -1
		default:
			gh.bT += dt
			gh.chargeBuzz(gh.bT)
			if gh.bT >= superHold {
				if r.SuperWall(wh.id) {
					gh.buzz(100, 1)
				}
				gh.bT = -1
			}
		}
	}
}

// chargeBuzz: a supercharge building up in your hand
func (gh *gHand) chargeBuzz(t float64) {
	if int(t*12) != int((t-frameDT)*12) {
		gh.buzz(10, float32(0.15+0.6*math.Min(1, t/superHold)))
	}
}

const (
	superHold = 1.0 // A or B held this long on a wall: it's supercharged
	eraseSecs = 1.5 // the trigger held this long on a magenta wall: it's taken down
)

// eraseTarget: the magenta wall a trigger pull would take down - the beam passes through
// its sheet (or its base stations, or just over the top) before it lands. Aiming at the
// floor right by a station is drawing a new wall from it instead.
func (g *game) eraseTarget(o, d vec3, land rayHit) rayHit {
	r := g.r
	maxT := laserReach
	if land.ok {
		maxT = land.t + 0.05
	}
	best := rayHit{t: math.Inf(1)}
	d2 := sim.V(d.x, d.z)
	q0 := sim.V(o.x, o.z)
	for _, w := range r.Maze {
		ab := w.B.Sub(w.A)
		den := d2.Cross(ab)
		if math.Abs(den) < 1e-9 {
			continue
		}
		q := w.A.Sub(q0)
		t := q.Cross(ab) / den
		u := q.Cross(d2) / den
		e := 0.25 / math.Max(ab.Len(), 1e-6)
		if t <= 0 || t > maxT || u < -e || u > 1+e || t >= best.t {
			continue
		}
		if y := o.y + d.y*t; y < 0 || y > r.T.WallHeight+0.3 {
			continue
		}
		best = rayHit{ok: true, t: t, p: o.add(d.mul(t)), kind: 2, id: w.ID}
	}
	if !best.ok {
		return rayHit{}
	}
	if land.ok && land.kind == 3 {
		lp := pt2{land.p.x, land.p.z}
		if sp := g.snapEnd(lp); sp != lp && dist2(sp, pt2{best.p.x, best.p.z}) < 0.45 {
			return rayHit{} // the floor at a station's foot: drawing from it
		}
	}
	return best
}

// highlightWall: the wall a button is about to act on, picked out along its edges.
func (g *game) highlightWall(wh rayHit) {
	r := g.r
	k := float32(0.6 + 0.4*math.Sin(fxTime()*10))
	var a, b pt2
	var h float64
	col := cEnergyHot
	switch wh.kind {
	case 1:
		a, b, h = ed.pts[wh.a], ed.pts[wh.b], genH
	case 2:
		for _, w := range r.Maze {
			if w.ID == wh.id {
				a, b = pv(w.A), pv(w.B)
			}
		}
		h, col = r.T.WallHeight, cMagentaHot
	case 5:
		ringSoft(wh.p, 0.3, 0.025, cEnergyHot.alpha(0.9*k), 32)
		return
	default:
		return
	}
	for _, y := range []float64{0.04, h - 0.05} {
		ribbon(fl(a, y), fl(b, y), 0.03, col.alpha(0.9*k))
	}
	for _, p := range []pt2{a, b} {
		ribbon(fl(p, 0.04), fl(p, h-0.05), 0.03, col.alpha(0.9*k))
	}
}

// turretTarget: the beam passing through a spent harpoon turret (what A recharges).
func (g *game) turretTarget(o, d vec3) rayHit {
	h := &g.r.Harp
	if !h.Up || h.Super || h.Kind == sim.ShotFlying || g.harpUp < 0.95 {
		return rayHit{}
	}
	F, _ := g.harpoonFrame()
	t := F.o.sub(o).dot(d)
	if t <= 0 || o.add(d.mul(t)).sub(F.o).length() > 0.3 {
		return rayHit{}
	}
	return rayHit{ok: true, t: t, p: F.o, kind: 5}
}

func fmtSecs(s float64) string { return itoa(int(math.Ceil(s))) + " S" }

// ---- drawing ----

func (g *game) drawHands() {
	t := fxTime()
	g.drawFit()
	g.drawPulls()
	// just in from ZoneTweak: the white of the lever's flash clears, and the controllers you
	// had are gloves now, still glinting
	in := t - gloveIn
	if in >= 0 && in < 0.6 {
		k := 1 - in/0.6
		sprite(ed.hmd.pos().add(ed.hmd.axis(2).mul(-0.3)), 2, rgba{1, 0.95, 0.9, float32(0.9 * k * k)})
	}
	for _, gh := range hands {
		if !gh.valid() {
			continue
		}
		if in >= 0 && in < 1.6 {
			k := 1 - in/1.6
			p := gh.local(0, 0, -0.04)
			sprite(p, 0.12+0.1*k, cBeam.alpha(float32(0.6*k)))
			ringSoftAxes(p, gh.m.axis(0), gh.m.axis(2), 0.06+0.12*(1-k), 0.006, cBeam.alpha(float32(0.8*k)), 24)
			if rngF() < 0.4*k {
				spawnSparks(p, v3(0, 1, 0), 2, 0.2)
			}
		}
		g.drawGlove(gh)
		if gh.hover != nil {
			p := gh.hover.gPos()
			ringSoft(p, 0.05+0.006*math.Sin(t*10), 0.01, cBeam.alpha(0.8), 24)
			sprite(p, 0.06, cBeam.alpha(0.25))
		}
		if gh.beam {
			g.drawBeam(gh)
		}
	}
}

func (g *game) drawGlove(gh *gHand) {
	X, Y, Z := gh.axes()
	seedTex(7000 + float64(gh.h.idx)*50)
	defer withTex(xf{gh.local(0, 0, 0), X, Y, Z})()
	saveM := curMat
	defer useMat(saveM)
	segMul = 1.4
	wearScale = 0.12 // the gloves are always right in front of your eyes: keep them clean
	defer func() { segMul, wearScale = 1, 1 }()
	plate := rgba{0.16, 0.17, 0.19, 1}
	steel := rgba{0.45, 0.47, 0.5, 1}
	// the forearm cuff runs up your arm, not along the hand: the hand bends at the wrist
	wrist := gh.local(0, -0.008, 0.045)
	fd := gh.forearmDir(wrist)
	fu := projPerp(Y, fd)
	if fu.length() < 0.1 {
		fu = projPerp(X, fd)
	}
	fu = fu.norm()
	fs := fd.cross(fu).norm()
	useMat(mCast)
	ball(wrist, 0.03, plate.scale(1.1))
	cyl(&bEnv, wrist.add(fd.mul(0.012)), wrist.add(fd.mul(0.135)), 0.036, 0.043, 16, plate, true)
	useMat(mSteel)
	cyl(&bEnv, wrist.add(fd.mul(0.115)), wrist.add(fd.mul(0.135)), 0.045, 0.045, 16, steel, true)
	// the armoured sleeve on up the forearm to the elbow
	useMat(mCast)
	cyl(&bEnv, wrist.add(fd.mul(0.135)), wrist.add(fd.mul(0.235)), 0.043, 0.05, 16, plate.scale(0.9), false)
	useMat(mSteel)
	cyl(&bEnv, wrist.add(fd.mul(0.225)), wrist.add(fd.mul(0.25)), 0.052, 0.051, 16, steel, true)
	useMat(mCast)
	hemi(&bEnv, wrist.add(fd.mul(0.25)), fd, 0.051, plate, 3, 16)
	drawArmDebug(gh)
	ring := cEnergy
	if gh.held != nil {
		ring = cAmber
	}
	c := wrist.add(fd.mul(0.03))
	for k := 0; k < 16; k++ {
		a0, a1 := float64(k)*math.Pi/8, float64(k+1)*math.Pi/8
		p0 := c.add(fs.mul(0.039 * math.Cos(a0))).add(fu.mul(0.039 * math.Sin(a0)))
		p1 := c.add(fs.mul(0.039 * math.Cos(a1))).add(fu.mul(0.039 * math.Sin(a1)))
		bGlowLine.line(p0, p1, ring.alpha(0.8))
	}
	useMat(mCast)
	// palm and back plate
	useMat(mPaint)
	bevelBox(&bEnv, gh.local(-0.002, 0, -0.012), X, Y, Z, 0.022, 0.04, 0.045, 0.01, plate)
	useMat(mSteel)
	bevelBox(&bEnv, gh.local(0.024, 0.002, -0.015), X, Y, Z, 0.008, 0.042, 0.05, 0.006, steel)
	useMat(mHazard)
	bevelBox(&bEnv, gh.local(0.031, 0.002, -0.01), X, Y, Z, 0.002, 0.03, 0.03, 0.001, rgba{0.8, 0.6, 0.1, 1})
	// fingers
	useMat(mCast)
	ys := []float64{0.024, 0.008, -0.008, -0.024}
	for k, y := range ys {
		curl := gh.grip
		lens := [3]float64{0.03, 0.022, 0.018}
		if k == 0 {
			curl = gh.trig * 0.95
			lens = [3]float64{0.032, 0.024, 0.019}
		}
		if k == 3 {
			lens = [3]float64{0.024, 0.018, 0.015}
		}
		ch := gh.finger(gh.local(0.012, y, -0.052), curl, lens)
		for s := 0; s+1 < len(ch); s++ {
			r := 0.0095 - float64(s)*0.0012
			cyl(&bEnv, ch[s], ch[s+1], r, r*0.92, 8, plate.scale(1.2), true)
			ball(ch[s+1], r*0.95, steel)
		}
		if k == 0 {
			sprite(ch[3], 0.008, cBeam.alpha(0.6))
		}
	}
	// thumb
	troot := gh.local(-0.006, 0.042, -0.02)
	td := Z.mul(-0.8).add(Y.mul(0.35)).add(X.mul(-0.35)).norm()
	tc := gh.grip * 0.6
	t1 := troot.add(td.mul(0.03))
	td2 := td.mul(math.Cos(tc)).add(X.mul(-math.Sin(tc))).norm()
	t2 := t1.add(td2.mul(0.026))
	cyl(&bEnv, troot, t1, 0.011, 0.01, 8, plate.scale(1.2), true)
	cyl(&bEnv, t1, t2, 0.01, 0.009, 8, plate.scale(1.2), true)
	ball(t1, 0.01, steel)
	// laser emitter on the back of the hand
	em := gh.local(0.03, 0.02, -0.04)
	curEmis = 3
	sphere(&bEnv, em, 0.006, cBeam, 3, 6)
	curEmis = 1
	if gh.h.idx == 0 {
		g.drawWrist(gh, wrist, fd, fu)
	}
}

func (g *game) drawBeam(gh *gHand) {
	o := gh.aim.pos()
	d := gh.aim.axis(2).mul(-1)
	end := o.add(d.mul(8))
	col := cBeam
	if gh.hit.ok {
		end = gh.hit.p
	}
	r := g.r
	switch {
	case gh.building:
		a, b := gh.bA, pt2{end.x, end.z}
		if gh.hit.kind == 3 {
			b = g.snapEnd(b)
		}
		cost, why := r.CheckWall(sv(a), sv(b))
		wc := cMagenta
		if why != "" {
			wc = cDanger
		}
		energySheet(a, b, func(float64) float64 { return 0.05 }, func(float64) float64 { return r.T.WallHeight }, nil, wc, 0.35, 999)
		mid := fl(lerp2(a, b, 0.5), r.T.WallHeight+0.25)
		rr, uu := billboard(ed.hmd.pos(), mid)
		label := itoa(int(math.Round(cost))) + " PTS"
		if why != "" {
			label = why
		}
		drawTextCentered(&bOpaque, label, mid, rr, uu, textPx*1.6, wc)
		drawTextCentered(&bOpaque, itoa(int(r.Points))+" LEFT", mid.add(v3(0, -0.07, 0)), rr, uu, textPx*1.1, cWhite)
		col = cMagenta
	case gh.hit.kind == 3 && !g.trace.on && !sim.InPoly(r.Room, sv(pt2{end.x, end.z})):
		// where a magenta wall would start (none in your room, so no marker there)
		ringSoft(fl(g.snapEnd(pt2{end.x, end.z}), 0.01), 0.07, 0.02, cMagenta.alpha(0.8), 24)
	}
	// the trigger on a magenta wall: hold it and the wall folds away
	if eh := gh.eraseHit; (gh.erasing || eh.ok && !gh.wallHit.ok) && !gh.building {
		wp := eh.p
		if gh.erasing {
			if w := r.Wall(gh.eraseID); w != nil {
				_, u := sim.SegDist(sv(pt2{end.x, end.z}), w.A, w.B)
				wp = fl(pv(sim.Lerp(w.A, w.B, u)), math.Min(1.6, math.Max(0.6, end.y)))
			}
		}
		cam := ed.hmd.pos()
		px := 0.0011 * math.Max(1, wp.sub(cam).length())
		q := wp.add(v3(0, 0.12+12*px, 0))
		rr, uu := billboard(cam, q)
		if gh.erasing {
			k := g.retract[gh.eraseID]
			ringSoft(wp, 0.05+0.15*k, 0.02, cDanger.alpha(0.9), 24)
			arcRing(wp, rr, uu, 0.22, k, cDanger)
			msg := "REMOVING..."
			if w := r.Wall(gh.eraseID); w != nil {
				msg += "  +" + itoa(int(math.Round(r.WallCost(w.A, w.B)))) + " PTS"
			}
			drawTextCentered(&bOpaque, msg, q, rr, uu, px, cDanger)
			col = cDanger
		} else {
			ringSoft(wp, 0.07, 0.012, cDanger.alpha(0.5), 24)
			drawTextCentered(&bOpaque, "HOLD TRIGGER: REMOVE", q, rr, uu, px, cWhite.alpha(0.8))
		}
	}
	// where the beam crosses an energy wall: that's what A supercharges (or, held, takes down)
	if wh := gh.wallHit; wh.ok && !gh.building {
		wp := wh.p
		if r.Phase == sim.Playing {
			if r.SuperCD > 0 {
				ringSoft(wp, 0.08, 0.015, cMuted.alpha(0.8), 24)
			} else {
				ringSoft(wp, 0.08+0.02*math.Sin(fxTime()*12), 0.02, cAmber.alpha(0.9), 24)
				sprite(wp, 0.12, cAmber.alpha(0.3))
			}
		} else if wh.kind == 2 {
			ringSoft(wp, 0.07, 0.012, cDanger.alpha(0.5), 24)
		}
		// what the button under your thumb will do to the wall it's picked out
		cam := ed.hmd.pos()
		dist := wp.sub(cam).length()
		px := 0.0011 * math.Max(1, dist)
		q := wp.add(v3(0, 0.12+12*px, 0))
		rr, uu := billboard(cam, q)
		msg, col := "", cWhite.alpha(0.85)
		charge := func(k float64, c rgba) {
			k = math.Min(1, k/superHold)
			ringSoft(wp, 0.05+0.18*k, 0.02, c.alpha(0.9), 24)
			arcRing(wp, rr, uu, 0.22, k, c)
			if rngF() < k {
				arc(o.add(end.sub(o).mul(0.6)), wp, c)
			}
		}
		switch {
		case wh.kind == 2 && gh.bT > 0:
			msg, col = "CHARGING...", cMagentaHot
			charge(gh.bT, cMagentaHot)
		case (wh.kind == 1 || wh.kind == 5) && gh.aT > 0:
			msg, col = "CHARGING...", cEnergyHot
			if wh.kind == 5 {
				col = cBeam
			}
			charge(gh.aT, col)
		case wh.kind == 2 && r.Phase == sim.Playing:
			msg = "HOLD B: SUPERCHARGE   HOLD TRIGGER: REMOVE"
		case wh.kind == 2:
			msg = "HOLD THE TRIGGER TO REMOVE IT"
		case wh.kind == 1 && r.Phase == sim.Playing:
			msg = "HOLD A: SUPERCHARGE"
		case wh.kind == 5:
			msg = "HOLD A: SUPERCHARGE THE HARPOON"
		}
		if msg != "" {
			drawTextCentered(&bOpaque, msg, q, rr, uu, px, col)
		}
		g.highlightWall(wh)
	}
	beam(o, end, col, 0.004, 2.5)
	addLine(o, end, col, 0.3, 2)
	flare(end, o.sub(end).norm(), 0.04, col)
}

// drawWrist: the left forearm carries a little display: next wave, points, device, supercharge.
// It sits on top of the cuff (the thumb side), facing up, reading toward the hand.
func (g *game) drawWrist(gh *gHand, wrist, fd, fu vec3) {
	r := g.r
	c := wrist.add(fd.mul(0.075)).add(fu.mul(0.045))
	n := fu
	right := fd.mul(-1)
	up := n.cross(right).norm()
	useMat(mCast)
	bevelBox(&bEnv, c.sub(n.mul(0.004)), right, up, n, 0.032, 0.022, 0.004, 0.002, cFrame)
	bTrans.quadC(c.add(right.mul(-0.03)).add(up.mul(-0.02)), c.add(right.mul(0.03)).add(up.mul(-0.02)), c.add(right.mul(0.03)).add(up.mul(0.02)), c.add(right.mul(-0.03)).add(up.mul(0.02)),
		rgba{0.02, 0.05, 0.04, 0.95}, rgba{0.02, 0.05, 0.04, 0.95}, rgba{0.03, 0.07, 0.05, 0.95}, rgba{0.03, 0.07, 0.05, 0.95})
	px := 0.00062
	line := func(i int, s string, col rgba) {
		drawText(&bOpaque, s, c.add(right.mul(-0.027)).add(up.mul(0.012-float64(i)*0.0075)).add(n.mul(0.001)), right, up, px, col)
	}
	switch r.Phase {
	case sim.Setup:
		line(0, "SETUP", cAmber)
		line(1, itoa(int(r.Points))+" PTS", cWhite)
	case sim.Playing:
		line(0, "WAVE "+itoa(r.Wave+1)+" IN "+itoa(int(math.Ceil(r.NextWave-r.Time))), cDanger)
		line(1, itoa(int(r.Points))+" PTS", cWhite)
		line(2, "EMP "+itoa(int(100*r.Work()/r.WorkTotal()))+"%", cBeam)
		if r.SuperCD > 0 {
			line(3, "SUPER "+itoa(int(math.Ceil(r.SuperCD))), cMuted)
		} else {
			line(3, "SUPER READY", cAmber)
		}
	default:
		line(0, "ROUND OVER", cAmber)
	}
}
