package main

// The harpoon turret: it rises at the weak spot when a device milestone loads a spear. Grab
// its grips and swing it to aim, pull a trigger to fire. If the spear sticks in a warden or
// in the device, crank the winch on its side (or grab the line and yank) to haul it in - a
// warden hauled through your green ring or a magenta wall gets fried. A spent turret takes a
// supercharge (point at it, A) for a fresh spear; a line stuck in the floor can be tied off
// to the cleat on the turret's post and left up as a live trap.

import (
	"boundaryvr/sim"
	"math"
)

type harpGrip struct {
	g    *game
	side float64
}

// the spade grips sit well behind the pivot, so the gun swings like a mounted gun: push
// the grips left and the muzzle goes right, lift them and it dips
const harpGripBack = 0.34

func (q *harpGrip) gPos() vec3 {
	F, _ := q.g.harpoonFrame()
	return F.p(q.side*0.17, -0.02, -harpGripBack)
}
func (q *harpGrip) gRad() float64 { return 0.11 }
func (q *harpGrip) gOK() bool {
	h := &q.g.r.Harp
	return h.Up && q.g.harpUp > 0.95
}
func (q *harpGrip) gGrab(h *gHand)    { q.g.harpN = -1 } // re-anchor on the next hold
func (q *harpGrip) gRelease(h *gHand) { q.g.harpN = -1 }

// angles of a direction: yaw about up, and elevation
func yawEl(v vec3) (float64, float64) {
	return math.Atan2(v.z, v.x), math.Atan2(v.y, math.Hypot(v.x, v.z))
}

func (q *harpGrip) gHold(h *gHand, dt float64) {
	g := q.g
	hp := &g.r.Harp
	// aim: the gun turns as a rigid body about its pivot. Whatever rotation takes your hands
	// (their midpoint) from where they were when you took hold to where they are now turns
	// the barrel the same way.
	F, _ := g.harpoonFrame()
	var sum vec3
	n := 0
	for _, gh := range hands {
		if gh != nil && (gh.held == g.grips[0] || gh.held == g.grips[1]) {
			sum = sum.add(gh.palm)
			n++
		}
	}
	if n > 0 {
		v := sum.mul(1 / float64(n)).sub(F.o)
		if n != g.harpN {
			g.harpN, g.harpV0, g.harpA0 = n, v, g.harpAim
		}
		y0, e0 := yawEl(g.harpV0)
		y1, e1 := yawEl(v)
		ay, ae := yawEl(g.harpA0)
		yaw := ay + angDiff2(y0, y1)
		pitch := math.Max(-0.6, math.Min(0.35, ae-(e1-e0)))
		want := v3(math.Cos(yaw)*math.Cos(pitch), math.Sin(pitch), math.Sin(yaw)*math.Cos(pitch))
		g.harpAim = g.harpAim.add(want.sub(g.harpAim).mul(math.Min(1, dt*20))).norm()
	}
	_, muzzle := g.harpoonFrame()
	g.r.SetMuzzle(s3v(muzzle))
	if h.trigDown {
		if !hp.Armed && g.r.Tractor(s3v(muzzle), s3v(g.harpAim)) {
			h.buzz(80, 0.8)
			ed.sfx(sndPowerUp, muzzle, 0.8)
		} else if g.r.Fire(s3v(muzzle), s3v(g.harpAim)) {
			h.buzz(120, 1)
			for _, o := range hands {
				if o != nil && o != h && (o.held == g.grips[0] || o.held == g.grips[1]) {
					o.buzz(120, 1)
				}
			}
		} else {
			h.buzz(30, 0.3)
			if !hp.Powered {
				g.say("NO POWER", "YOUR RING DOESN'T COVER THE TURRET", cDanger, 1.5)
			} else if !hp.Armed && g.r.Dev.State == sim.DevDropped {
				g.say("AIM AT THE CART", "THE TRACTOR BEAM NEEDS TO BE ON IT", cBeam, 1.5)
			} else if !hp.Armed {
				g.say("EMPTY", "POINT AT IT AND HOLD A TO RELOAD (USES YOUR SUPERCHARGE)", cAmber, 2)
			}
		}
	}
}

// harpLine: the line you grab to haul in (one per hand, each at the point nearest that hand).
type harpLine struct {
	g     *game
	idx   int
	prevD float64
}

func (l *harpLine) ends() (vec3, vec3) {
	_, muzzle := l.g.harpoonFrame()
	return muzzle, v3s(l.g.r.Harp.Tip)
}

func (l *harpLine) gPos() vec3 {
	a, b := l.ends()
	gh := hands[l.idx]
	if gh == nil || !gh.valid() {
		return a
	}
	ab := b.sub(a)
	u := 0.0
	if L2 := ab.dot(ab); L2 > 1e-6 {
		u = math.Max(0, math.Min(1, gh.palm.sub(a).dot(ab)/L2))
	}
	return a.add(ab.mul(u))
}
func (l *harpLine) gRad() float64 { return 0.1 }
func (l *harpLine) gOK() bool {
	k := l.g.r.Harp.Kind
	if k == sim.ShotLoose {
		return l.g.handHolding(l) != nil // only the hand that has it
	}
	return k == sim.ShotRobot || k == sim.ShotDevice || k == sim.ShotAnchor
}
func (l *harpLine) gGrab(h *gHand) {
	g := l.g
	if g.r.Harp.Kind == sim.ShotAnchor && g.r.DetachLine() {
		// a line stuck in the floor: you unhook it from the gun, and it goes dead in your hand
		_, muzzle := g.harpoonFrame()
		ed.sfx(sndPowerDown, muzzle, 0.6)
		ed.sfx(sndClang, muzzle, 0.4)
		spawnSparks(muzzle, v3(0, 1, 0), 15, 0.6)
		h.buzz(50, 0.8)
		return
	}
	_, b := l.ends()
	l.prevD = h.palm.sub(b).length()
	g.r.HoldLine(true)
}
func (l *harpLine) gHold(h *gHand, dt float64) {
	g := l.g
	hp := &g.r.Harp
	if hp.Kind == sim.ShotLoose {
		// bring its end to the glowing cleat on the turret's post to tie it off - live again
		if c := g.harpCleat(); h.palm.sub(c).length() < 0.16 {
			if g.r.TieOff(s3v(c)) {
				h.held = nil
				h.buzz(80, 1)
				ed.sfx(sndClang, c, 0.7)
				ed.sfx(sndPowerUp, c, 0.6)
				spawnSparks(c, v3(0, 1, 0), 25, 0.8)
				g.say("TIED OFF", "IT'S LIVE AGAIN, AND THE GUN'S FREE TO SHOOT", cMagenta, 2.5)
			}
		}
		return
	}
	if hp.Kind == sim.ShotAnchor {
		return
	}
	_, b := l.ends()
	d := h.palm.sub(b).length()
	if pull := d - l.prevD; pull > 0 {
		// a hard yank hauls them in fast
		k := 2.0
		if h.vel.length() > 1.5 {
			k = 4.0
		}
		g.r.Reel(pull * k)
		h.buzz(10, float32(math.Min(1, 0.3+pull*20)))
	}
	l.prevD = d
	g.r.HoldLine(true)
}

// harpCleat: where an old line can be tied off, on the turret's post.
func (g *game) harpCleat() vec3 {
	h := &g.r.Harp
	F, _ := g.harpoonFrame()
	side := v3(F.x.x, 0, F.x.z).norm()
	return fv(h.Pos, 0.78).add(side.mul(0.09))
}

// reelFrame: the winch hauls the line in as you crank it, either way round.
func (g *game) reelFrame() {
	k := g.reel
	if k == nil {
		return
	}
	F, _ := g.harpoonFrame()
	k.c, k.ax, k.ref, k.R = F.p(0.2, -0.06, -0.12), F.x, F.y, 0.075
	d := math.Abs(k.turns - g.reelTurns)
	g.reelTurns = k.turns
	if !k.held || d <= 0 {
		return
	}
	g.r.Reel(d * reelPerTurn)
	g.r.HoldLine(true)
	if h := g.handHolding(k); h != nil {
		h.buzz(6, float32(math.Min(1, 0.2+d*6)))
	}
}

const reelPerTurn = 0.9 // metres of line a turn of the winch hauls in
func (l *harpLine) gRelease(h *gHand) {
	if l.g.r.Harp.Kind == sim.ShotLoose {
		l.g.r.DropLine() // let go without tying it off: the line's gone
		return
	}
	if l.g.handHolding(l.g.lines[0]) == nil && l.g.handHolding(l.g.lines[1]) == nil {
		l.g.r.HoldLine(false)
	}
}

func (g *game) harpGrabbers(out []grabber) []grabber {
	if g.grips[0] == nil {
		g.grips = [2]*harpGrip{{g: g, side: -1}, {g: g, side: 1}}
		g.lines = [2]*harpLine{{g: g, idx: 0}, {g: g, idx: 1}}
	}
	if g.reel == nil {
		g.reel = &crank{R: 0.075}
		g.reel.ok = func() bool {
			k := g.r.Harp.Kind
			return g.r.Harp.Up && (k == sim.ShotRobot || k == sim.ShotDevice)
		}
	}
	return append(out, g.grips[0], g.grips[1], g.lines[0], g.lines[1], g.reel)
}

func (g *game) harpoonFrame() (xf, vec3) {
	h := &g.r.Harp
	base := fv(h.Pos, 0)
	rise := smooth(g.harpUp)
	piv := base.add(v3(0, 1.08*rise-0.05, 0))
	aim := g.harpAim
	if aim.length() < 0.5 {
		aim = v3(0, 0, -1)
	}
	fwd := aim.norm()
	side := fwd.cross(v3(0, 1, 0))
	if side.length() < 1e-3 {
		side = v3(1, 0, 0)
	}
	side = side.norm()
	up := side.cross(fwd).norm()
	return xf{piv, side, up, fwd}, piv.add(fwd.mul(0.62))
}

// harpoonTick: when a turret rises somewhere new, it points out of the ring.
func (g *game) harpoonTick(dt float64) {
	h := &g.r.Harp
	if !h.Up {
		return
	}
	g.harpUp = math.Min(1, g.harpUp+dt/1.6)
	if g.harpUp < 0.05 {
		c := sim.Centroid(g.r.Room)
		out := h.Pos.Sub(c).Norm()
		if out.Len() < 0.5 {
			out = sim.V(0, -1)
		}
		g.harpAim = v3(out.X, -0.08, out.Z).norm()
	}
	if g.handHolding(g.grips[0]) == nil && g.handHolding(g.grips[1]) == nil {
		_, muzzle := g.harpoonFrame()
		g.r.SetMuzzle(s3v(muzzle))
	}
	g.reelFrame()
	if g.handHolding(g.lines[0]) == nil && g.handHolding(g.lines[1]) == nil && (g.reel == nil || !g.reel.held) {
		g.r.HoldLine(false)
	}
}

func (g *game) drawHarpoon() {
	h := &g.r.Harp
	if !h.Up {
		return
	}
	t := fxTime()
	base := fv(h.Pos, 0)
	seedTex(16000)
	useMat(mCast)
	cyl(&bEnv, base, base.add(v3(0, 0.02, 0)), 0.32, 0.32, 24, cIron.scale(1.2), true)
	useMat(mHazard)
	cyl(&bEnv, base.add(v3(0, 0.003, 0)), base.add(v3(0, 0.018, 0)), 0.34, 0.34, 24, rgba{0.8, 0.6, 0.1, 1}, false)
	F, muzzle := g.harpoonFrame()
	useMat(mSteel)
	cyl(&bEnv, base.add(v3(0, 0.02, 0)), F.o.sub(v3(0, 0.08, 0)), 0.07, 0.06, 16, cSteelEnv.scale(0.8), true)
	useMat(mCast)
	cyl(&bEnv, F.o.sub(v3(0, 0.12, 0)), F.o.sub(v3(0, 0.04, 0)), 0.1, 0.1, 18, cIron, true)
	for _, sx := range []float64{-1, 1} {
		bevelBox(&bEnv, F.o.add(F.x.mul(sx*0.13)).add(v3(0, 0.02, 0)), F.x, v3(0, 1, 0), F.z, 0.015, 0.1, 0.05, 0.005, cIron)
	}
	useMat(mPaint)
	bevelBox(&bEnv, F.p(0, 0, -0.1), F.x, F.y, F.z, 0.1, 0.09, 0.25, 0.02, rgba{0.3, 0.32, 0.28, 1})
	useMat(mSteel)
	cyl(&bEnv, F.p(0, 0.02, 0.1), muzzle, 0.035, 0.03, 14, cSteelEnv, true)
	useMat(mCast)
	cyl(&bEnv, F.p(0, 0.02, 0.5), muzzle, 0.045, 0.045, 14, cIron, true)
	for i, sx := range []float64{-1, 1} {
		gp := F.p(sx*0.17, -0.02, -harpGripBack)
		hc := rgba{0.05, 0.05, 0.05, 1}
		if g.grips[i] != nil && g.handHolding(g.grips[i]) != nil {
			hc = rgba{0.18, 0.14, 0.04, 1}
		}
		useMat(mRubber)
		cyl(&bEnv, gp.sub(F.y.mul(0.07)), gp.add(F.y.mul(0.07)), 0.022, 0.022, 10, hc, true)
		useMat(mCast)
		cyl(&bEnv, F.p(sx*0.1, -0.02, -harpGripBack), gp, 0.012, 0.012, 8, cIron, false)
	}
	useMat(mSteel)
	cyl(&bEnv, F.p(-0.07, -0.1, -0.12), F.p(0.07, -0.1, -0.12), 0.06, 0.06, 16, cSteelEnv.scale(0.6), true)
	// the trigger paddle between the grips
	useMat(mPaint)
	bevelBox(&bEnv, F.p(0, -0.03, -harpGripBack-0.01), F.x, F.y, F.z, 0.04, 0.025, 0.01, 0.004, cDanger.scale(0.7))
	if h.Armed {
		useMat(mChrome)
		cyl(&bEnv, muzzle.sub(F.z.mul(0.05)), muzzle.add(F.z.mul(0.08)), 0.02, 0.002, 10, cChromeEnv, true)
		// a faint sight line while you hold it
		if g.grips[0] != nil && (g.handHolding(g.grips[0]) != nil || g.handHolding(g.grips[1]) != nil) {
			hit := g.rayWorld(muzzle, F.z, false)
			end := muzzle.add(F.z.mul(12))
			if hit.ok {
				end = hit.p
			}
			ribbon(muzzle, end, 0.003, cDanger.alpha(0.35))
			sprite(end, 0.05, cDanger.alpha(0.8))
		}
	}
	lamp := F.p(0, 0.1, -0.15)
	var lc rgba
	switch {
	case !h.Powered:
		lc = cDanger.scale(float32(0.4 + 0.4*math.Max(0, math.Sin(t*5))))
	case h.Armed && h.Reels > 0:
		lc = cYellow // reel-only: it'll hook a robot or the cart, no live wire
	case h.Armed:
		lc = cEnergy // a full spear: a live wire, or a robot to reel in
	default:
		lc = rgba{0.12, 0.12, 0.12, 1} // empty
	}
	curEmis = 4
	sphere(&bEnv, lamp, 0.018, lc, 4, 8)
	// reel-only spears left: a pip each either side of the lamp
	for i := 0; i < h.Reels && i < 2; i++ {
		sphere(&bEnv, lamp.add(F.x.mul(0.035*float64(2*i-1))), 0.008, cYellow, 4, 6)
	}
	curEmis = 1
	sprite(lamp, 0.07, lc.alpha(0.7))
	if h.Armed && h.Powered && g.harpUp > 0.99 {
		addPoint(lamp.add(v3(0, 0.1, 0)), lc, 1.2, 2)
		sprite(muzzle, 0.08, lc.alpha(float32(0.3+0.2*math.Sin(t*6))))
	}
	if h.Super && g.harpUp > 0.5 {
		g.drawHarpField(F, muzzle, t)
	}
	if h.Beam {
		g.drawTractor(muzzle, t)
	}
	// the winch on its side
	if k := g.reel; k != nil {
		useMat(mCast)
		cyl(&bEnv, k.c.sub(k.ax.mul(0.02)), k.c.add(k.ax.mul(0.03)), 0.05, 0.05, 16, cIron, true)
		hd := k.handle()
		useMat(mSteel)
		cyl(&bEnv, k.c.add(k.ax.mul(0.03)), hd.sub(k.ax.mul(0.02)), 0.008, 0.008, 6, cSteelEnv, false)
		useMat(mRubber)
		hc := rgba{0.07, 0.07, 0.07, 1}
		if k.gOK() {
			hc = cAmber.scale(0.7)
		}
		cyl(&bEnv, hd.sub(k.ax.mul(0.02)), hd.add(k.ax.mul(0.04)), 0.016, 0.016, 10, hc, true)
	}
	// the cleat on the post, and the traps tied off to it
	cl := g.harpCleat()
	useMat(mSteel)
	bevelBox(&bEnv, cl, v3(1, 0, 0), v3(0, 1, 0), v3(0, 0, 1), 0.02, 0.012, 0.035, 0.004, cSteelEnv)
	switch h.Kind {
	case sim.ShotAnchor:
		sprite(cl, 0.06, cMagenta.alpha(float32(0.4+0.3*math.Sin(t*6))))
	case sim.ShotLoose:
		// the line's in your hand: the cleat lights up where it ties off
		k := float32(0.6 + 0.4*math.Sin(t*8))
		sprite(cl, 0.16, cMagentaHot.alpha(0.7*k))
		ringSoft(cl, 0.07+0.02*math.Sin(t*8), 0.012, cMagentaHot.alpha(0.9), 20)
		addPoint(cl, cMagenta, 1.5, 1.5)
		cam := ed.hmd.pos()
		q := cl.add(v3(0, 0.14, 0))
		rr, uu := billboard(cam, q)
		drawTextCentered(&bOpaque, "TIE IT HERE", q, rr, uu, 0.0012*math.Max(1, q.sub(cam).length()), cMagentaHot)
	}
	for _, w := range h.Traps {
		g.drawLiveWireAged(v3s(w.A), v3s(w.B), h.Powered, w.T/sim.LineLife)
	}
	// the line
	if h.Kind == sim.ShotNone {
		return
	}
	tip := v3s(h.Tip)
	switch h.Kind {
	case sim.ShotAnchor:
		g.drawLiveWireAged(muzzle, tip, h.Powered, h.LineT/sim.LineLife)
	case sim.ShotLoose:
		// dead, from where it's stuck to the clip in your hand
		end := muzzle
		for _, ln := range g.lines {
			if hh := g.handHolding(ln); hh != nil {
				end = hh.palm
			}
		}
		pts := drapeCord(tip, end, end.sub(tip).length()*1.08, 18, 0.006)
		drawCord(pts, 0.007, rgba{0.08, 0.08, 0.08, 1})
		useMat(mSteel)
		cyl(&bEnv, end.sub(v3(0, 0.03, 0)), end.add(v3(0, 0.03, 0)), 0.012, 0.012, 8, cSteelEnv, true)
		useMat(mChrome)
		d := tip.sub(end).norm()
		cyl(&bEnv, tip.add(d.mul(0.0)), tip.sub(d.mul(0.3)), 0.002, 0.012, 8, cChromeEnv, true)
		return
	case sim.ShotRobot, sim.ShotDevice:
		// it hangs slack until it's hauled tight
		L := math.Max(h.Line, tip.sub(muzzle).length())
		pts := drapeCord(muzzle, tip, L, 18, 0.006)
		drawCord(pts, 0.006, rgba{0.08, 0.08, 0.08, 1})
		if h.Live {
			g.drawCharged(pts, t)
		}
	default:
		useMat(mRubber)
		cyl(&bEnv, muzzle, tip, 0.006, 0.006, 5, rgba{0.08, 0.08, 0.08, 1}, false)
		if h.Live {
			g.drawCharged([]vec3{muzzle, tip}, t)
			sprite(tip, 0.25, cBeam.alpha(0.6))
			addPoint(tip, cBeam, 1.5, 3)
		}
	}
	if h.Kind == sim.ShotFlying || h.Kind == sim.ShotAnchor {
		useMat(mChrome)
		d := tip.sub(muzzle).norm()
		cyl(&bEnv, tip.sub(d.mul(0.4)), tip, 0.012, 0.002, 8, cChromeEnv, true)
	}
}

// drawHarpField: a supercharged turret: a crackling shell of energy round its head.
func (g *game) drawHarpField(F xf, muzzle vec3, t float64) {
	c := F.o
	R := 0.42 + 0.02*math.Sin(t*9)
	for k := 0; k < 3; k++ {
		ax := []vec3{F.x, F.y, F.z}[k]
		bx := []vec3{F.y, F.z, F.x}[k]
		ph := t*(1.3+0.4*float64(k)) + float64(k)
		u := ax.mul(math.Cos(ph)).add(bx.mul(math.Sin(ph)))
		w := ax.cross(bx).norm()
		ringSoftAxes(c, u, w, R, 0.012, cBeam.alpha(0.55), 40)
	}
	sprite(c, 1.0, cBeam.alpha(float32(0.12+0.05*math.Sin(t*13))))
	sprite(muzzle, 0.18, cBeam.alpha(float32(0.5+0.3*math.Sin(t*17))))
	addPoint(c.add(v3(0, 0.2, 0)), cBeam, 2.2, 3)
	if rngF() < 0.5 {
		d := v3(rngF()-0.5, rngF()-0.5, rngF()-0.5).norm()
		arc(c.add(d.mul(0.12)), c.add(d.mul(R)), cBeam)
	}
	if rngF() < 0.3 {
		arc(muzzle, muzzle.add(v3(rngF()-0.5, rngF()-0.5, rngF()-0.5).mul(0.25)), cBeam)
	}
}

// drawTractor: the tractor beam - a pale tube of rings streaming from the cart back to the
// turret's muzzle.
func (g *game) drawTractor(muzzle vec3, t float64) {
	d := &g.r.Dev
	c := fv(d.Pos, 0.4)
	ax := muzzle.sub(c)
	L := ax.length()
	if L < 0.05 {
		return
	}
	ax = ax.mul(1 / L)
	u := ax.cross(v3(0, 1, 0))
	if u.length() < 0.1 {
		u = v3(1, 0, 0)
	}
	u = u.norm()
	w := ax.cross(u).norm()
	for k := 0; k < 9; k++ {
		f := math.Mod(t*0.9+float64(k)/9, 1)
		R := 0.06 + 0.35*(1-f)
		ringSoftAxes(c.add(ax.mul(L*f)), u, w, R, 0.01, cBeam.alpha(float32(0.55*(1-f)+0.15)), 24)
	}
	ribbon(c, muzzle, 0.25, cBeam.alpha(0.08))
	ribbon(c, muzzle, 0.06, cBeam.alpha(0.2))
	sprite(c, 0.9, cBeam.alpha(0.18))
	addPoint(c.add(v3(0, 0.3, 0)), cBeam, 2, 3)
	cam := ed.hmd.pos()
	q := c.add(v3(0, 0.85, 0))
	rr, uu := billboard(cam, q)
	drawTextCentered(&bOpaque, "TRACTOR BEAM", q, rr, uu, 0.0014*math.Max(1, q.sub(cam).length()), cBeam)
}

// drawCharged: a supercharged line - electric blue crackle along it.
func (g *game) drawCharged(pts []vec3, t float64) {
	for k := 1; k < len(pts); k++ {
		j := v3(rngF()-0.5, rngF()-0.5, rngF()-0.5).mul(0.025)
		ribbon(pts[k-1], pts[k].add(j), 0.035, cBeam.alpha(0.45))
		bGlowLine.line(pts[k-1], pts[k].add(j), cBeam)
	}
	addLine(pts[0], pts[len(pts)-1], cBeam, 2.5, 3)
	if rngF() < 0.5 {
		q := pts[int(rngF()*float64(len(pts)-1))]
		arc(q, q.add(v3(rngF()-0.5, rngF()-0.5, rngF()-0.5).mul(0.3)), cBeam)
	}
}

// drawLiveWireAged: a live line that may be burning out (life 0..1: the last stretch of a
// line keeping a magenta wall charged - it sputters, then it's gone).
func (g *game) drawLiveWireAged(a, b vec3, live bool, life float64) {
	if live && life > 0.83 && rngF() < (life-0.83)*4 {
		// sputtering: a beat dark
		useMat(mRubber)
		cyl(&bEnv, a, b, 0.006, 0.006, 5, rgba{0.1, 0.06, 0.1, 1}, false)
		if rngF() < 0.5 {
			q := a.add(b.sub(a).mul(rngF()))
			spawnSparks(q, v3(0, 1, 0), 3, 0.4)
		}
		return
	}
	g.drawLiveWire(a, b, live)
}

// drawLiveWire: a harpoon line strung tight between two points - crackling magenta while the
// turret has power, a dead black line when it hasn't.
func (g *game) drawLiveWire(a, b vec3, live bool) {
	prev := a
	const segs = 18
	sag := 0.03
	if !live {
		sag = 0.25
	}
	for k := 1; k <= segs; k++ {
		u := float64(k) / segs
		p := a.add(b.sub(a).mul(u)).add(v3(0, -sag*4*u*(1-u), 0))
		if live {
			j := v3((rngF()-0.5)*0.02, (rngF()-0.5)*0.02, (rngF()-0.5)*0.02)
			ribbon(prev, p.add(j), 0.03, cMagenta.alpha(0.5))
			bGlowLine.line(prev, p.add(j), cMagentaHot)
		} else {
			useMat(mRubber)
			cyl(&bEnv, prev, p, 0.006, 0.006, 5, rgba{0.08, 0.08, 0.08, 1}, false)
		}
		prev = p
	}
	if live {
		addLine(a, b, cMagenta, 2.5, 3)
		if rngF() < 0.3 {
			u := rngF()
			q := a.add(b.sub(a).mul(u))
			arc(q, q.add(v3(rngF()-0.5, rngF()-0.5, rngF()-0.5).mul(0.2)), cMagentaHot)
		}
	}
}

// shockFX: a supercharged spear's shock going off at p (game time t).
type shockFX struct {
	p   vec3
	t   float64
	col rgba // blue unless set
}

// drawShocks: each shock a burst of blue rings and arcs out to the wardens it dazed.
func (g *game) drawShocks() {
	keep := g.shockFX[:0]
	for _, s := range g.shockFX {
		age := g.t - s.t
		if age > 0.9 {
			continue
		}
		keep = append(keep, s)
		k := age / 0.9
		a := float32(1 - k)
		c := cBeam
		if s.col.a > 0 {
			c = s.col
			sprite(s.p, 1.2*(1-k)+0.2, c.alpha(0.7*a))
			ringSoft(v3(s.p.x, 1.0, s.p.z), 0.2+0.9*k, 0.03, c.alpha(0.7*a), 32)
			addPoint(s.p, c, 4*float64(a), 4)
			continue
		}
		ringSoft(v3(s.p.x, 0.05, s.p.z), 0.3+sim.ShockDaze*k, 0.05, c.alpha(0.9*a), 48)
		ringSoft(v3(s.p.x, 1.0, s.p.z), 0.2+1.2*k, 0.03, c.alpha(0.7*a), 32)
		sprite(s.p, 1.5*(1-k)+0.3, c.alpha(0.6*a))
		addPoint(s.p, c, 4*float64(a), 5)
		for _, b := range g.r.Robots {
			if b.Alive() && b.Dazed > sim.DazeSecs-0.6 && rngF() < 0.6 {
				arc(s.p, fv(b.Pos, 1.1), cBeam)
			}
		}
	}
	g.shockFX = keep
}

// superPulse: a wall just supercharged (its blast rolls out over its reach).
type superPulse struct{ t float64 }

// drawSuperReach: a supercharged wall's blast - a sheet of current rolling out either side
// when it goes hot, and arcs reaching out to every warden within its reach while it lasts.
func (g *game) drawSuperReach() {
	r := g.r
	hot := r.HotSegs()
	keep := g.superPulse[:0]
	for _, p := range g.superPulse {
		if g.t-p.t < 0.6 {
			keep = append(keep, p)
		}
	}
	g.superPulse = keep
	if len(hot) == 0 {
		return
	}
	for _, s := range hot {
		a, b := pv(s.A), pv(s.B)
		d := sim.V(s.B.X-s.A.X, s.B.Z-s.A.Z)
		if d.Len() < 1e-6 {
			continue
		}
		n := sim.V(-d.Z, d.X).Norm()
		// the blast rolling out when it goes hot
		for _, p := range g.superPulse {
			k := (g.t - p.t) / 0.6
			for _, sd := range []float64{-1, 1} {
				off := n.Mul(sd * sim.SuperReach * k)
				pa, pb := fl(pt2{a[0] + off.X, a[1] + off.Z}, 0.6), fl(pt2{b[0] + off.X, b[1] + off.Z}, 0.6)
				ribbon(pa, pb, 0.6*(1-k), cBeam.alpha(float32(0.35*(1-k))))
				bGlowLine.line(pa, pb, cBeam.alpha(float32(1-k)))
			}
		}
		// arcs out to whatever's in reach
		for _, bot := range r.Robots {
			if !bot.Alive() && (bot.Cause != "super" || bot.DeadT > 0.4) {
				continue
			}
			dd, u := sim.SegDist(bot.Pos, s.A, s.B)
			if dd > sim.SuperReach+bot.Radius() {
				continue
			}
			// bolts out of the wall into it
			to := fv(bot.Pos, 0.6+rngF()*0.8)
			if bot.Kind == sim.Drone {
				to, _ = g.dronePos(bot)
			}
			for q := 0; q < 2; q++ {
				from := fl(lerp2(a, b, math.Max(0, math.Min(1, u+(rngF()-0.5)*0.15))), 0.3+rngF()*1.3)
				arc(from, to.add(v3(rngF()-0.5, rngF()-0.5, rngF()-0.5).mul(0.25)), cBeam)
				ribbon(from, to, 0.04, cBeam.alpha(0.25))
			}
			sprite(to, 0.35+0.15*rngF(), cBeam.alpha(0.55))
			addPoint(to, cBeam, 1.5, 2.5)
			if rngF() < 0.3 {
				spawnSparks(to, v3(0, 1, 0), 6, 0.8)
			}
		}
	}
}
