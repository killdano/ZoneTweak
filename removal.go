package main

import (
	"math"
	"math/rand"
)

// Deleting a generator: the boundary changes at once, while a stand-in of the deleted post
// plays one of four exits, picked at random:
//   remDecommission - powers down, walls unzip into the neighbours, the shaft telescopes down,
//                     pistons and cables let go, the housing sinks into the base; the empty
//                     base is left for the salvage robots
//   remExtract      - a claw drops from the ceiling, clamps the shaft and rips it out through
//                     the roof; the torn base is left for salvage
//   remHatch        - the deck under it splits, the whole post sinks into the dark on its lift
//                     and hatch doors slide shut over it
//   remOverload     - it shorts out: arcs crawl up the shaft, the bell blows off, the shaft
//                     crumples into its base; the stump and the bits are left for salvage
// Undo while it plays brings the post straight back and cancels the stand-in.

const (
	remDecommission = iota
	remExtract
	remHatch
	remOverload
	remKinds
)

type removal struct {
	kind       int
	p, anchor  pt2
	prev, next pt2
	H          float64
	ax         vec3
	nbrs       []pt2
	joint      bool
	t0         float64
	fired      uint32
	ob         *oldBase
	wallIdx    int
	wallSet    bool
}

var (
	removals    []*removal
	lastRemKind = -1
	remForce    = -1 // tests: force one exit
)

func remDur(kind int) float64 {
	switch kind {
	case remDecommission:
		return 2.7
	case remExtract:
		return 2.9
	case remHatch:
		return 2.9
	}
	return 2.5
}

// startRemoval: called just before point idx is cut out of the outline
func (e *editor) startRemoval(idx int) {
	n := len(e.pts)
	if idx < 0 || idx >= n || n < 2 {
		return
	}
	p := e.pts[idx]
	k := rand.Intn(remKinds)
	if k == lastRemKind {
		k = (k + 1 + rand.Intn(remKinds-1)) % remKinds
	}
	if remForce >= 0 {
		k = remForce
	}
	lastRemKind = k
	anc := anchorOf(p)
	r := &removal{kind: k, p: p, anchor: anc, prev: e.pts[(idx-1+n)%n], next: e.pts[(idx+1)%n], H: e.height, t0: fxTime(),
		wallIdx: (idx - 1 + n - 1) % (n - 1), joint: jointGen[anc]}
	r.nbrs = append(r.nbrs, jointNbrs[idx]...)
	if bp := basePoses[anc]; bp != nil {
		r.ax = bp.ax
	} else {
		r.ax = baseAxisFor(anc, r.prev, r.next, r.nbrs...)
	}
	removals = append(removals, r)
}

func (r *removal) at(bit uint, t float64) bool {
	if r.fired&(1<<bit) == 0 && fxTime()-r.t0 >= t {
		r.fired |= 1 << bit
		return true
	}
	return false
}

// leave the base behind for the salvage crews
func (e *editor) remLeaveBase(r *removal) {
	if r.ob != nil {
		return
	}
	r.ob = &oldBase{pos: r.anchor, ax: r.ax, t0: fxTime(), joint: r.joint, intact: r.kind == remDecommission}
	oldBases = append(oldBases, r.ob)
}

// inView: whether anything within rad of p could be on screen (a cone wider than any headset)
func (e *editor) inView(p vec3, rad float64) bool {
	d := p.sub(e.hmd.pos())
	l := d.length()
	if l < rad+0.3 {
		return true
	}
	a := math.Acos(math.Max(-1, math.Min(1, d.dot(e.hmd.axis(2).mul(-1))/l)))
	return a-math.Asin(math.Min(1, rad/l)) < 72*math.Pi/180
}

func (e *editor) updateRemovals() {
	t := fxTime()
	out := removals[:0]
	for _, r := range removals {
		back := false
		for _, q := range e.pts {
			if dist2(q, r.p) < 1e-4 {
				back = true
			}
		}
		if back {
			// undone: the post is back, so is its base
			if r.ob != nil && !r.ob.gone {
				e.releaseClaim(r.ob)
				r.ob.gone = true
			}
			continue
		}
		// the wall that now joins the two neighbours powers up once the old ones are gone
		if !r.wallSet && wallKeyN == len(e.pts) && r.wallIdx < len(e.pts) {
			r.wallSet = true
			wallPow[r.wallIdx] = 0
			delay := 0.35
			if r.kind == remDecommission {
				delay = 0.95
			}
			wallOff[r.wallIdx] = r.t0 + delay
		}
		if t-r.t0 > remDur(r.kind) {
			if r.kind != remHatch {
				e.remLeaveBase(r)
				continue
			}
			// the sealed hatch stays in the deck until you're not looking at it
			if !e.inView(fl(r.anchor, 0), 0.45) {
				continue
			}
		}
		out = append(out, r)
	}
	removals = out
}

// ghostWall: what's left of a deleted post's wall, from the post (u=0) to the neighbour (u=1),
// drawn from u0 to 1, at strength k
func ghostWall(a, b pt2, top float64, u0, k float64, col rgba) {
	if k <= 0.01 || u0 >= 1 {
		return
	}
	at := func(u, y float64) vec3 { return v3(a[0]+(b[0]-a[0])*u, y, a[1]+(b[1]-a[1])*u) }
	pa := float32(k)
	n := 6
	for c := 0; c < n; c++ {
		ua := u0 + (1-u0)*float64(c)/float64(n)
		ub := u0 + (1-u0)*float64(c+1)/float64(n)
		lo := floorGap
		bGlowTri.quadC(at(ua, lo), at(ub, lo), at(ub, top), at(ua, top), col.alpha(0.05*pa), col.alpha(0.05*pa), col.alpha(0.015*pa), col.alpha(0.015*pa))
	}
	for _, y := range []float64{floorGap, top} {
		ribbon(at(u0, y), at(1, y), 0.05, col.alpha(0.35*pa))
		bGlowLine.line(at(u0, y), at(1, y), col.alpha(0.9*pa))
	}
	if u0 > 0.001 {
		// the retracting front: a hot seam running back to the neighbour
		f0, f1 := at(u0, floorGap), at(u0, top)
		ribbon(f0, f1, 0.04, cEnergyHot.alpha(0.8*pa))
		bGlowLine.line(f0, f1, cEnergyHot.alpha(pa))
		if rand.Float64() < 0.4 {
			spawnSparks(lerpV(f0, f1, rand.Float64()), v3(0, 1, 0), 1, 0.3)
		}
	}
}

func (e *editor) drawRemovals(col rgba) {
	for _, r := range removals {
		seedTex(91000 + r.t0*7)
		switch r.kind {
		case remDecommission:
			e.remDecommission(r, col)
		case remExtract:
			e.remExtract(r, col)
		case remHatch:
			e.remHatch(r, col)
		case remOverload:
			e.remOverload(r, col)
		}
	}
}

// the walls of a deleted post: unzip back into the neighbours, or flicker out
func (r *removal) walls(col rgba, unzip bool) {
	age := fxTime() - r.t0
	top := r.H - bladeTopIn
	for _, nb := range []pt2{r.prev, r.next} {
		if unzip {
			ghostWall(r.p, nb, top, smooth(age/0.9), 1, col)
			continue
		}
		k := 1 - age/0.3
		if noise1(r.t0*5+nb[0], age*30) < 0.35 {
			k *= 0.2
		}
		ghostWall(r.p, nb, top, 0, k, col)
	}
}

// the base with its pistons out (k2 1) or let go (0); sink lowers the whole base
func (r *removal) base(k2, sink float64) {
	if k2 < 0 {
		k2 = 0
	}
	baseLift = -sink
	reattachK = k2 * 0.6
	if k2 >= 1 {
		reattachK = -1
	}
	cableStow = r.kind == remDecommission // a clean shutdown unplugs its cables, it doesn't snap them
	genBaseGeom(r.anchor, r.p, 1, false, 0, r.ax, r.nbrs...)
	reattachK, cableStow = -1, false
	baseLift = 0
}

func (e *editor) remDecommission(r *removal, col rgba) {
	age := fxTime() - r.t0
	c := fl(r.p, 0)
	up := v3(0, 1, 0)
	if r.at(0, 0) {
		e.sfx(sndPowerDown, fl(r.p, 1.2), 0.7)
	}
	if r.at(1, 0.6) {
		e.sfx(sndHydraulic, fl(r.p, 1), 0.7)
	}
	if r.at(2, 1.05) {
		e.sfx(sndRelay, fl(r.p, 0.2), 0.7)
		e.sfx(sndSteam, fl(r.p, 0.2), 0.5)
		spawnSteam(fl(r.p, 0.1), up, 10)
	}
	if r.at(3, 2.35) {
		e.sfx(sndClang, fl(r.p, 0.1), 0.6)
		spawnSteam(fl(r.p, 0.05), up, 8)
	}
	r.walls(col, true)
	// shaft telescopes down to its housing, the pistons let go, the housing sinks into the base
	Hs := r.H - (r.H-0.7)*smooth((age-0.6)/0.9)
	lift := -0.75 * smooth((age-1.8)/0.6)
	k2 := 1 - smooth((age-1.0)/0.7)
	collarDrop = lift
	r.base(k2, 0)
	collarDrop = 0
	if lift > -0.74 {
		genShaftGeom(r.p, Hs, r.prev, r.next, lift)
	}
	// red beacon on the bell while it shuts down
	if age < 1.6 {
		b := c.add(up.mul(Hs + 0.13 + lift))
		on := float32(0.5 + 0.5*math.Sin(age*14))
		sprite(b, 0.08, rgba{1, 0.12, 0.06, 0.8 * on})
		addPoint(b, rgba{1, 0.1, 0.05, 1}, 1.5*float64(on), 2)
	}
	// the dying blade
	if age < 0.7 {
		fk := float32((1 - age/0.7) * (0.4 + 0.6*noise1(r.t0, age*25)))
		for _, nb := range []pt2{r.prev, r.next} {
			dv := v3(nb[0]-r.p[0], 0, nb[1]-r.p[1]).norm()
			o := c.add(dv.mul(0.071))
			ribbon(o.add(up.mul(0.44)), o.add(up.mul(r.H-0.26)), 0.02, col.alpha(0.5*fk))
		}
	}
	if lift < 0 && lift > -0.7 && rand.Float64() < 0.3 {
		spawnSteam(c.add(up.mul(0.08)), v3(rand.NormFloat64(), 0.4, rand.NormFloat64()).norm(), 1)
	}
}

// ceilingY: where the extraction claw comes from
const ceilingY = 3.9

func drawClaw(hub vec3, open float64) {
	up := v3(0, 1, 0)
	useMat(mCast)
	cyl(&bEnv, hub, hub.add(up.mul(0.14)), 0.095, 0.075, 18, cIron, true)
	useMat(mHazard)
	cyl(&bEnv, hub.add(up.mul(0.03)), hub.add(up.mul(0.06)), 0.098, 0.098, 18, rgba{0.8, 0.6, 0.1, 1}, false)
	useMat(mSteel)
	for k := 0; k < 3; k++ {
		a := float64(k) * 2 * math.Pi / 3
		d := v3(math.Cos(a), 0, math.Sin(a))
		knuckle := hub.add(d.mul(0.09)).add(up.mul(0.01))
		mid := knuckle.add(d.mul(0.05 + 0.08*open)).add(up.mul(-0.12))
		tip := mid.add(d.mul(-0.05 + 0.03*open)).add(up.mul(-0.1))
		ball(knuckle, 0.022, cSteelEnv)
		cyl(&bEnv, knuckle, mid, 0.018, 0.016, 8, cSteelEnv, false)
		ball(mid, 0.017, cSteelEnv)
		cyl(&bEnv, mid, tip, 0.016, 0.008, 8, cIron, true)
	}
}

func (e *editor) remExtract(r *removal, col rgba) {
	age := fxTime() - r.t0
	up := v3(0, 1, 0)
	if r.at(0, 0) {
		e.sfx(sndServo, fl(r.p, 3), 0.7)
	}
	if r.at(1, 0.72) {
		e.sfx(sndHydraulic, fl(r.p, r.H), 0.8)
		e.sfx(sndClang, fl(r.p, r.H), 0.6)
		spawnSparks(fl(r.p, r.H), up, 12, 0.4)
	}
	if r.at(2, 0.95) {
		e.sfx(sndCreak, fl(r.p, 0.3), 0.8)
	}
	if r.at(3, 1.3) {
		e.sfx(sndRip, fl(r.p, 0.15), 1)
		c := fl(r.anchor, 0.12)
		spawnSparks(c, up, 60, 1.1)
		spawnSmoke(c, 8)
		sparkBursts = append(sparkBursts, sparkBurst{p: c, t: fxTime(), k: 1})
		e.remLeaveBase(r)
	}
	if r.at(4, 1.45) {
		e.sfx(sndHydraulic, fl(r.p, 3), 0.9)
	}
	r.walls(col, false)
	// the claw: drops, clamps the cap, holds while the mount strains, then hauls away
	capY := r.H + 0.1
	hubY := ceilingY
	switch {
	case age < 0.7:
		hubY = ceilingY + (capY-ceilingY)*smooth(age/0.7)
	default:
		hubY = capY
	}
	lift := 0.0
	shake := pt2{}
	if age > 0.95 && age < 1.3 {
		s := (age - 0.95) / 0.35
		shake = pt2{0.012 * s * math.Sin(age*80), 0.012 * s * math.Cos(age*73)}
	}
	if age > 1.3 {
		u := age - 1.3
		lift = 3.2 * u * u
		hubY += lift
	}
	open := 1.0
	if age > 0.6 {
		open = 1 - smooth((age-0.6)/0.15)
	}
	hub := fl(r.p, hubY).add(v3(shake[0], 0, shake[1]))
	// cable up to the roof
	cTop := fl(r.p, ceilingY+0.5)
	if hub.y < cTop.y {
		useMat(mSteel)
		cyl(&bEnv, hub.add(up.mul(0.14)), cTop, 0.012, 0.012, 8, cSteelEnv, false)
	}
	drawClaw(hub, open)
	if age < 1.3 {
		k2 := 1.0
		if age > 1.0 {
			k2 = 1 - smooth((age-1.0)/0.3)
		}
		r.base(k2, 0)
	}
	if hub.y < ceilingY+r.H+0.5 {
		sp := pt2{r.p[0] + shake[0], r.p[1] + shake[1]}
		genShaftGeom(sp, r.H, r.prev, r.next, lift)
		if lift > 0 {
			if rand.Float64() < 0.4 {
				spawnSparks(fl(r.p, lift+0.05), up.mul(-1), 2, 0.4)
			}
			if lift > ceilingY-r.H-0.3 && rand.Float64() < 0.5 {
				spawnSmoke(fl(r.p, ceilingY-0.1), 1) // dust where it punches through
			}
		}
	}
	if age < 0.7 {
		// warning strobe on the claw on its way down
		on := float32(0.5 + 0.5*math.Sin(age*20))
		sprite(hub.add(up.mul(0.15)), 0.07, rgba{1, 0.6, 0.1, on})
	}
}

func (e *editor) remHatch(r *removal, col rgba) {
	age := fxTime() - r.t0
	up := v3(0, 1, 0)
	const R = 0.44
	if r.at(0, 0) {
		e.sfx(sndRelay, fl(r.p, 0.1), 0.7)
	}
	if r.at(1, 0.4) {
		e.sfx(sndClang, fl(r.p, 0.05), 0.5)
		e.sfx(sndHydraulic, fl(r.p, 0.1), 0.8)
	}
	if r.at(2, 0.9) {
		e.sfx(sndServo, fl(r.p, 0.1), 0.7)
	}
	if r.at(3, 2.05) {
		e.sfx(sndHydraulic, fl(r.p, 0.05), 0.6)
	}
	if r.at(4, 2.4) {
		e.sfx(sndClang, fl(r.p, 0.02), 0.8)
		e.sfx(sndSteam, fl(r.p, 0.05), 0.5)
		spawnSteam(fl(r.p, 0.03), up, 12)
	}
	r.walls(col, false)
	c := fl(r.anchor, 0)
	ax, az := baseFrame(r.ax)
	// warning: the seam round the hatch lights amber and pulses
	if age < 2.4 {
		on := float32(0.5 + 0.5*math.Sin(age*12))
		ringSoft(c.add(up.mul(0.006)), R, 0.012, cAmber.alpha(0.8*on), 40)
	}
	open := smooth((age - 0.4) / 0.4)
	sink := 0.0
	if age > 0.9 {
		u := (age - 0.9) / 1.1
		sink = 2.6 * math.Pow(math.Min(1, u), 1.6)
	}
	closeK := smooth((age - 2.0) / 0.4)
	// the pit: dark below the deck once the floor plates open
	if open > 0 && closeK < 1 {
		useMat(mRubber)
		pr := R * open
		const segs = 28
		for k := 0; k < segs; k++ {
			a0, a1 := float64(k)*2*math.Pi/segs, float64(k+1)*2*math.Pi/segs
			p0 := c.add(ax.mul(pr * math.Cos(a0))).add(az.mul(pr * math.Sin(a0))).add(up.mul(0.002))
			p1 := c.add(ax.mul(pr * math.Cos(a1))).add(az.mul(pr * math.Sin(a1))).add(up.mul(0.002))
			bEnv.triS(c.add(up.mul(0.002)), p1, p0, up, rgba{0.004, 0.004, 0.005, 1})
		}
		floorGlow(r.anchor, R*1.2, cAmber.alpha(float32(0.25*open*(1-closeK))))
	}
	// post and base ride the lift down; anything below the deck isn't drawn
	if sink < 2.55 {
		r.base(1, sink)
		genShaftGeom(r.p, r.H, r.prev, r.next, -sink)
	}
	// hatch doors slide in from under the deck edge and seal it, then settle flush
	if age > 1.9 {
		settle := smooth((age - 2.55) / 0.3)
		y := 0.006 - 0.004*settle // settles flush with the deck
		for _, sd := range []float64{-1, 1} {
			off := ax.mul(sd * R * (1 - closeK))
			useMat(mHazard)
			const segs = 14
			for k := 0; k < segs; k++ {
				a0 := -math.Pi/2 + float64(k)*math.Pi/segs
				a1 := a0 + math.Pi/segs
				d0 := ax.mul(sd * math.Cos(a0)).add(az.mul(math.Sin(a0)))
				d1 := ax.mul(sd * math.Cos(a1)).add(az.mul(math.Sin(a1)))
				o := c.add(off).add(up.mul(y))
				bEnv.triS(o, o.add(d1.mul(R)), o.add(d0.mul(R)), up, rgba{0.3, 0.28, 0.24, 1})
			}
			useMat(mSteel)
			e0 := c.add(off).add(az.mul(-R)).add(up.mul(y + 0.003))
			e1 := c.add(off).add(az.mul(R)).add(up.mul(y + 0.003))
			cyl(&bEnv, e0, e1, 0.006, 0.006, 6, rgba{0.8, 0.6, 0.1, 1}, false)
		}
	}
}

func (e *editor) remOverload(r *removal, col rgba) {
	age := fxTime() - r.t0
	up := v3(0, 1, 0)
	c := fl(r.p, 0)
	if r.at(0, 0) {
		e.sfx(sndZap, fl(r.p, 1), 0.6)
	}
	if r.at(1, 0.45) {
		e.sfx(sndZap, fl(r.p, 1.6), 0.7)
	}
	if r.at(2, 1.0) {
		top := fl(r.p, r.H-0.1)
		e.sfx(sndBoom, top, 0.55)
		e.sfx(sndRip, top, 0.6)
		spawnSparks(top, up, 70, 1.4)
		spawnSmoke(top, 10)
		sparkBursts = append(sparkBursts, sparkBurst{p: top, t: fxTime(), k: 1})
		// the bell cap goes flying; it lands as a piece of debris for the Scuttlers
		oldBases = append(oldBases, &oldBase{pos: pt2{top.x, top.z}, ax: v3(1, 0, 0), t0: fxTime(), debris: true, small: true, state: 3,
			shape: []pt2{{-0.07, -0.07}, {0.07, -0.07}, {0.07, 0.07}, {-0.07, 0.07}}, pieces: [3]int{0, 2, 2}, dpos: top.add(up.mul(0.1)),
			dvel: v3(rand.NormFloat64()*0.8, 3.2, rand.NormFloat64()*0.8), bell: true,
			spinAx: v3(rand.NormFloat64(), rand.NormFloat64(), rand.NormFloat64()).norm(), spinV: 6 + rand.Float64()*6})
		// shrapnel: a few chunks of the bell for the Scuttlers to pick up later
		for k := 0; k < 3; k++ {
			var shape []pt2
			r0 := 0.03 + rand.Float64()*0.03
			for v := 0; v < 5; v++ {
				a := float64(v) * 2 * math.Pi / 5
				shape = append(shape, pt2{math.Cos(a) * r0 * (0.6 + 0.6*rand.Float64()), math.Sin(a) * r0 * (0.6 + 0.6*rand.Float64())})
			}
			vel := v3(rand.NormFloat64()*1.5, 1.5+rand.Float64()*2, rand.NormFloat64()*1.5)
			oldBases = append(oldBases, &oldBase{pos: pt2{top.x, top.z}, ax: v3(1, 0, 0), t0: fxTime(), debris: true, small: true, state: 3,
				shape: shape, pieces: [3]int{0, 2, 2}, dpos: top, dvel: vel,
				spinAx: v3(rand.NormFloat64(), rand.NormFloat64(), rand.NormFloat64()).norm(), spinV: 8 + rand.Float64()*12})
		}
	}
	if r.at(3, 1.15) {
		e.sfx(sndCreak, fl(r.p, 0.6), 0.9)
		e.sfx(sndClang, fl(r.p, 0.3), 0.5)
	}
	if r.at(4, 1.9) {
		e.sfx(sndClang, fl(r.p, 0.15), 0.6)
		spawnSmoke(fl(r.p, 0.3), 6)
		e.remLeaveBase(r)
	}
	// walls thrash, then die with the blast
	if age < 1.0 {
		top := r.H - bladeTopIn
		for _, nb := range []pt2{r.prev, r.next} {
			k := 0.3 + 0.9*noise1(r.t0+nb[0]*3, age*40)
			ghostWall(r.p, nb, top, 0, k, col)
		}
	}
	// the shaft rattles under the surge, then folds down into its base
	sh := 0.006 * math.Min(1, age/0.8)
	sp := pt2{r.p[0] + sh*math.Sin(age*90), r.p[1] + sh*math.Cos(age*83)}
	Hs := r.H
	if age > 1.0 {
		Hs = r.H - (r.H-0.7)*smooth((age-1.0)/0.9)
	}
	k2 := 1.0
	if age > 1.1 {
		k2 = 1 - smooth((age-1.1)/0.5)
	}
	if age < 1.9 {
		r.base(k2, 0)
		shaftNoCap = age >= 1.0
		genShaftGeom(sp, Hs, r.prev, r.next, 0)
		shaftNoCap = false
	}
	// arcs crawling up the column
	if age < 1.1 && rand.Float64() < 0.7 {
		y0 := 0.3 + rand.Float64()*(Hs-0.5)
		a0 := c.add(up.mul(y0)).add(v3(rand.NormFloat64(), 0, rand.NormFloat64()).norm().mul(0.06))
		a1 := a0.add(v3(rand.NormFloat64()*0.1, 0.12+rand.Float64()*0.25, rand.NormFloat64()*0.1))
		arc(a0, a1, rgba{0.75, 0.85, 1, 1})
		addPoint(a0, rgba{0.7, 0.8, 1, 1}, 4, 2)
		if rand.Float64() < 0.15 {
			e.sfx(sndSpark, a0, 0.4)
		}
	}
	if age > 1.0 && age < 1.9 && rand.Float64() < 0.5 {
		spawnSparks(c.add(up.mul(Hs-0.2)), v3(rand.NormFloat64(), 0.5, rand.NormFloat64()).norm(), 2, 0.5)
		if rand.Float64() < 0.3 {
			spawnSmoke(c.add(up.mul(Hs-0.1)), 1)
		}
	}
}
