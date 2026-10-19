//go:build windows

package main

import (
	"math"
	"math/rand"
)

// Salvage of ripped-off generator bases.
// Big robots (Warden: circular saw, Tread: laser cutter) cut around the base, lift it whole
// and carry it out through a door. When there's no room for a big robot, a small Scuttler
// cuts the base into three pieces and hauls them out one per trip.

const (
	toolTorch = iota
	toolSaw
	toolLaser
	toolPlasma
)

const (
	svTravel = iota
	svApproach
	svCut
	svLift
	svCarry
	svReturn
)

type svPlan struct {
	work, op pt2
	wall     int
	small    bool
}

// planFor finds a work spot next to the base. Big robots need ~0.75 m around generators;
// if that doesn't exist a Scuttler is sent instead.
func (e *editor) planFor(ob *oldBase, small bool) (svPlan, bool) {
	clr, dist := 0.75, 0.82
	if small {
		clr, dist = 0.62, 0.52
	}
	inside := len(e.pts) >= 3 && pointInPoly(e.pts, ob.pos[0], ob.pos[1])
	obst := e.obstacles()
	okPt := func(w pt2) bool {
		for _, o := range obst {
			if dist2(o, w) < clr {
				return false
			}
		}
		for _, o := range oldBases {
			if o != ob && !o.gone && dist2(o.pos, w) < clr {
				return false
			}
		}
		return true
	}
	segOK := func(a, b pt2) bool {
		for _, o := range obst {
			if d, _ := segDist(o, a, b); d < clr*0.9 {
				return false
			}
		}
		return true
	}
	best := svPlan{wall: -1}
	bestD := math.Inf(1)
	n := len(e.pts)
	for k := 0; k < 16; k++ {
		a := float64(k) * math.Pi / 8
		w := pt2{ob.pos[0] + math.Cos(a)*dist, ob.pos[1] + math.Sin(a)*dist}
		if !okPt(w) {
			continue
		}
		wIn := n >= 3 && pointInPoly(e.pts, w[0], w[1])
		if wIn != inside {
			continue
		}
		pl := svPlan{work: w, op: w, wall: -1, small: small}
		if inside {
			// walk in through the nearest wall from outside
			bd, bi, bq := math.Inf(1), -1, pt2{}
			for i := 0; i < n; i++ {
				d, u := segDist(w, e.pts[i], e.pts[(i+1)%n])
				if d < bd {
					a, b := e.pts[i], e.pts[(i+1)%n]
					bd, bi, bq = d, i, pt2{a[0] + (b[0]-a[0])*u, a[1] + (b[1]-a[1])*u}
				}
			}
			out := v3(bq[0]-w[0], 0, bq[1]-w[1]).norm()
			op := pt2{bq[0] + out.x*0.8, bq[1] + out.z*0.8}
			if !okPt(op) || !segOK(op, w) {
				continue
			}
			pl.op, pl.wall = op, bi
		} else if outsideDist(e.pts, w) < 0.45 {
			continue
		}
		d := dist2(pl.op, ob.pos)
		if d < bestD {
			best, bestD = pl, d
		}
	}
	return best, !math.IsInf(bestD, 1)
}

// forceSalvKind pins the salvage robot type (tests/preview only; -1 = choose)
var forceSalvKind = -1

func (e *editor) assignSalvage() {
	t := fxTime()
	debrisBots := 0
	for _, r := range robots {
		if r.salv != nil && r.salv.debris && !r.done {
			debrisBots++
		}
	}
	for _, ob := range oldBases {
		if ob.gone || ob.claimed != nil || t-ob.t0 < 2.5 || !world.ok || len(world.doors) == 0 {
			continue
		}
		if ob.debris && (!ob.landed || debrisBots > 0 || ob.fireDebris && fire.phase != fireDone) {
			continue // one Scuttler gathers the chunks, one at a time
		}
		if !ob.debris && debrisNear(ob.pos) {
			continue // clear the broken-off chunks first, then cut what's still bolted down
		}
		if len(robots) >= 5 {
			return
		}
		kind := kindWarden
		if rand.Float64() < 0.5 {
			kind = kindTread
		}
		if forceSalvKind >= 0 {
			kind = forceSalvKind
		}
		if ob.debris {
			kind = kindScuttler
			debrisBots++
		}
		pl, ok := e.planFor(ob, kind == kindScuttler)
		if ok && !e.reachable(pl.op, kind == kindScuttler) {
			ok = false
		}
		if !ok && kind != kindScuttler {
			pl, ok = e.planFor(ob, true)
			kind = kindScuttler
		}
		if !ok {
			continue
		}
		di := rand.Intn(len(world.doors))
		d := world.doors[di]
		r := &robot{pos: d.end, gen: -1, seed: rand.Float64() * 100, kind: kind}
		r.randomize()
		r.salv = ob
		r.svPiece = -1
		r.svTool = map[int]int{kindWarden: toolSaw, kindTread: toolLaser, kindScuttler: toolPlasma}[kind]
		r.svOut, r.svWork, r.svWall = pl.op, pl.work, pl.wall
		ob.claimed = r
		ob.small = pl.small
		r.yaw = math.Atan2(d.mouth[1]-d.end[1], d.mouth[0]-d.end[0])
		r.state, r.sv = rbSalvage, svTravel
		r.path = append([]pt2{d.mouth}, e.salvRoute(d.mouth, pl.op, pl.small)...)
		robots = append(robots, r)
	}
}

func (e *editor) reachable(p pt2, small bool) bool {
	rv := 0.7
	if small {
		rv = 0.6
	}
	for _, d := range world.doors {
		if e.routeC(d.mouth, p, rv) != nil {
			return true
		}
	}
	return false
}

func (e *editor) salvRoute(from, to pt2, small bool) []pt2 {
	rv := 0.72
	if small {
		rv = 0.6
	}
	if p := e.routeC(from, to, rv); p != nil {
		return p
	}
	if p := e.route(from, to); p != nil {
		return p
	}
	return []pt2{to}
}

func (e *editor) planSalvage(r *robot) {
	if r.salv == nil || r.salv.gone {
		e.sendHome(r)
		return
	}
	r.state = rbSalvage
	r.backing = false
	switch {
	case r.carryBase != nil || r.svPiece >= 0 && r.sv == svCarry:
		r.sv = svCarry
		d := world.doors[rand.Intn(len(world.doors))]
		r.path = append(e.salvRoute(r.pos, d.mouth, r.kind == kindScuttler), d.end)
	default:
		pl, ok := e.planFor(r.salv, r.kind == kindScuttler)
		if ok {
			r.svOut, r.svWork, r.svWall = pl.op, pl.work, pl.wall
		}
		r.sv = svTravel
		r.path = e.salvRoute(r.pos, r.svOut, r.kind == kindScuttler)
	}
}

func (r *robot) walkTo(tg pt2, speed, dt float64) bool {
	dx, dz := tg[0]-r.pos[0], tg[1]-r.pos[1]
	l := math.Hypot(dx, dz)
	if l < 0.05 {
		return true
	}
	ty := math.Atan2(dz, dx)
	df := angDiff(r.yaw, ty)
	r.yaw += df * math.Min(1, dt*3)
	st := math.Min(l, speed*math.Max(0.15, math.Cos(df))*dt)
	r.pos = pt2{r.pos[0] + dx/l*st, r.pos[1] + dz/l*st}
	r.travel += st
	stride := 0.62
	if r.kind != kindWarden {
		stride = 0.4
	}
	r.phase += st / stride * math.Pi
	return false
}

func (e *editor) holdWall(r *robot) {
	if r.svWall >= 0 && r.svWall < len(e.pts) {
		wallOff[r.svWall] = fxTime() + 0.6
	}
}

func (e *editor) stepSalvage(r *robot, dt float64) (want float64, headTo float64) {
	ob := r.salv
	t := fxTime()
	speed := 0.9
	if r.kind == kindScuttler {
		speed = 1.15
	}
	if r.carryBase != nil {
		speed *= 0.7
	}
	if ob == nil {
		e.sendHome(r)
		return 0, 0
	}
	followPath := func() bool {
		if len(r.path) == 0 {
			return true
		}
		if r.backing {
			tg := r.path[0]
			dx, dz := tg[0]-r.pos[0], tg[1]-r.pos[1]
			l := math.Hypot(dx, dz)
			st := math.Min(l, speed*0.5*dt)
			if l > 1e-6 {
				r.pos = pt2{r.pos[0] + dx/l*st, r.pos[1] + dz/l*st}
				r.travel -= st
				r.phase -= st / 0.4 * math.Pi
			}
			if l < 0.05 {
				r.backing = false
				r.path = r.path[1:]
			}
			return len(r.path) == 0
		}
		if r.walkTo(r.path[0], speed, dt) {
			r.path = r.path[1:]
		}
		return len(r.path) == 0
	}
	if r.sv != svCut {
		r.toolK = math.Max(0, r.toolK-dt/0.7) // fold the cutter away
	}
	switch r.sv {
	case svTravel:
		if followPath() {
			r.sv = svApproach
			r.svWait = 0.7
			if r.svWall >= 0 {
				e.sfx(sndSpeech, fl(r.pos, 0.8), 0.35)
			}
		}
		return 1, 0
	case svApproach:
		e.holdWall(r)
		r.svWait -= dt
		if r.svWait > 0 {
			return 0, 0
		}
		if r.walkTo(r.svWork, speed*0.8, dt) {
			if ob.state == 3 {
				r.sv = svLift
				r.svWait = 1.8
			} else {
				r.sv = svCut
				if ob.state == 0 && len(oldJointNbrs[ob]) > 0 {
					// first cut severs it from the shared slab: a hot seam flashes across the join
					for _, nb := range oldJointNbrs[ob] {
						a, b := fl(ob.pos, 0.03), fl(nb, 0.03)
						m := a.add(b).mul(0.5)
						spawnSparks(m, v3(0, 1, 0), 18, 0.6)
						spawnSteam(m, v3(0, 1, 0), 6)
						addPoint(m.add(v3(0, 0.1, 0)), cSpark, 8, 3)
					}
					e.sfx(sndRip, fl(ob.pos, 0.05), 0.55)
					e.sfx(sndHydraulic, fl(ob.pos, 0.1), 0.5)
				}
				if ob.state == 0 {
					ob.cutA = math.Atan2(r.pos[1]-ob.pos[1], r.pos[0]-ob.pos[0])
					if ob.biteR > 0 {
						// only the rim that's still there: start and end at the sheared edges
						b := math.Hypot(ob.biteC[0], ob.biteC[1])
						cw := (baseR*baseR + b*b - ob.biteR*ob.biteR) / (2 * baseR * math.Max(b, 1e-6))
						if cw > -1 && cw < 1 {
							w := math.Acos(cw)
							ax, az := baseFrame(ob.ax)
							bd := ax.mul(ob.biteC[0]).add(az.mul(ob.biteC[1]))
							phi := math.Atan2(bd.z, bd.x)
							ob.cutA, ob.cutSpan = phi+w, 2*math.Pi-2*w
						}
					}
				}
				ob.state = 1
			}
		}
		return 1, 0
	case svCut:
		e.holdWall(r)
		dur := 7.0
		if ob.small {
			dur = 5.5
		}
		g := ob.pos
		r.yaw += angDiff(r.yaw, math.Atan2(g[1]-r.pos[1], g[0]-r.pos[0])) * math.Min(1, dt*3)
		r.salvaging = true
		r.cutPt = e.cutPoint(ob, r)
		if r.toolK < 1 {
			// get the cutter out and onto the base before it starts biting
			r.toolK = math.Min(1, r.toolK+dt/0.9)
			return 0, 0
		}
		ob.cut = math.Min(1, ob.cut+dt/dur)
		if t > ob.nextFX {
			ob.nextFX = t + 0.5 + rand.Float64()*0.6
			snd := sndGrind
			if r.svTool != toolSaw {
				snd = sndSpark
			}
			e.sfx(snd, r.cutPt, 0.45)
		}
		if rand.Float64() < frameDT*12 {
			spawnSmoke(r.cutPt.add(v3(0, 0.05, 0)), 1)
		}
		if ob.cut >= 1 {
			if ob.small {
				ob.state = 3
				r.sv = svLift
				r.svWait = 1.8
			} else {
				ob.state = 2
				r.sv = svLift
				r.svWait = liftTime
				e.sfx(sndHydraulic, fl(ob.pos, 0.2), 0.6)
			}
			e.sfx(sndClang, fl(ob.pos, 0.1), 0.5)
		}
		return 0, 0
	case svLift:
		e.holdWall(r)
		r.svWait -= dt
		if ob.small {
			if r.svPiece < 0 {
				for k := range ob.pieces {
					if ob.pieces[k] == 0 {
						ob.pieces[k] = 1
						r.svPiece = k
						e.sfx(sndRelay, fl(ob.pos, 0.1), 0.6)
						break
					}
				}
			}
			r.liftK = math.Max(0, math.Min(1, 1-r.svWait/1.8))
		} else {
			if ob.carrier != r {
				ob.liftYaw = r.yaw
			}
			ob.carrier = r
			r.carryBase = ob
			r.liftK = math.Max(0, math.Min(1, 1-r.svWait/liftTime))
			if r.liftK > 0.38 && r.liftK-dt/liftTime <= 0.38 {
				e.sfx(sndRelay, fl(ob.pos, 0.1), 0.7)
				e.sfx(sndHydraulic, fl(ob.pos, 0.2), 0.6)
			}
		}
		if r.svWait <= 0 {
			r.liftK = 1
			r.sv = svCarry
			r.path = nil
			from := r.svOut
			if !ob.small {
				// reverse straight back with the load before turning, so it can't swing into a post
				f := v3(math.Cos(r.yaw), 0, math.Sin(r.yaw))
				back := pt2{r.pos[0] - f.x*0.7, r.pos[1] - f.z*0.7}
				r.path = append(r.path, back)
				r.backing = true
				if r.svWall < 0 {
					from = back
				}
			}
			if r.svWall >= 0 {
				r.path = append(r.path, r.svOut)
			}
			d := world.doors[rand.Intn(len(world.doors))]
			if rt := e.routeC(from, d.mouth, 1.05); !ob.small && rt != nil {
				// carrying a whole base out front: keep extra clearance from the posts
				r.path = append(r.path, rt...)
			} else {
				r.path = append(r.path, e.salvRoute(from, d.mouth, ob.small)...)
			}
			r.path = append(r.path, d.end)
		}
		return 0, 0
	case svCarry:
		if len(r.path) > 0 && r.svWall >= 0 && dist2(r.pos, r.svOut) > 0.1 && pointInPoly(e.pts, r.pos[0], r.pos[1]) {
			e.holdWall(r)
		}
		if followPath() {
			if ob.small {
				if r.svPiece >= 0 {
					ob.pieces[r.svPiece] = 2
					r.svPiece = -1
				}
				left := false
				for _, s := range ob.pieces {
					if s == 0 {
						left = true
					}
				}
				if !left {
					ob.gone = true
					if ob.debris && e.nextDebris(r) {
						return 1, 0
					}
					r.done = true
					return 0, 0
				}
				r.sv = svReturn
				r.svWait = 1.5 + rand.Float64()
				r.path = append(e.salvRoute(r.pos, r.svOut, true), r.svOut)
			} else {
				ob.gone = true
				r.carryBase = nil
				r.done = true
			}
		}
		return 1, 0
	case svReturn:
		r.svWait -= dt
		if r.svWait > 0 {
			return 0, 0
		}
		if followPath() {
			r.sv = svApproach
			r.svWait = 0.4
		}
		return 1, 0
	}
	return 0, 0
}

// cutPoint: where the tool is touching the base this frame
func (e *editor) cutPoint(ob *oldBase, r *robot) vec3 {
	c := fl(ob.pos, 0.02)
	if ob.small {
		// three radial cuts, centre to rim, splitting the plate into thirds
		k := math.Min(ob.cut*3, 2.999)
		line := math.Floor(k)
		u := k - line
		ang := math.Atan2(ob.ax.z, ob.ax.x) + line*2*math.Pi/3
		d := v3(math.Cos(ang), 0, math.Sin(ang))
		return c.add(d.mul(0.31 * u)).add(v3(0, 0.01, 0))
	}
	// all the way around the rim, starting at the side nearest the robot
	return c.add(rimDir(ob, ob.cut).mul(0.31))
}

// glowing kerf + sparks + smoke for whatever tool is working
func (e *editor) cutFX(r *robot, tip vec3) {
	ob := r.salv
	if ob == nil {
		return
	}
	cp := r.cutPt
	t := fxTime()
	switch r.svTool {
	case toolSaw:
		tan := cp.sub(fl(ob.pos, cp.y)).cross(v3(0, 1, 0)).norm()
		for k := 0; k < 4; k++ {
			d := tan.mul(0.8).add(v3(rand.NormFloat64()*0.3, 0.4+rand.Float64()*0.4, rand.NormFloat64()*0.3)).norm()
			spawnSparks(cp, d, 1, 1.2)
		}
		addPoint(cp.add(v3(0, 0.15, 0)), cSpark, 3+2*rand.Float64(), 3)
	case toolLaser, toolPlasma:
		col := rgba{1, 0.35, 0.15, 1}
		if r.svTool == toolPlasma {
			col = rgba{0.5, 0.75, 1, 1}
		}
		beam(tip, cp, col, 0.005, 3)
		sprite(cp, 0.09, cSpark.alpha(0.6))
		sprite(cp, 0.025, rgba{1, 1, 0.9, 1})
		if rand.Float64() < 0.5 {
			spawnSparks(cp, v3(0, 1, 0), 1+rand.Intn(2), 0.4)
		}
		addPoint(cp.add(v3(0, 0.12, 0)), col, 2.5+1.5*rand.Float64(), 2.5)
		r.weldOn = true
	}
	_ = t
}

func (e *editor) drawKerf(ob *oldBase) {
	if ob.cut <= 0 || ob.state == 2 {
		return
	}
	c := fl(ob.pos, 0.031)
	hot := func(age float64) rgba { return heatColor(math.Max(0, 0.95-age*0.6)) }
	if ob.small {
		for line := 0; line < 3; line++ {
			k := math.Min(1, ob.cut*3-float64(line))
			if k <= 0 {
				continue
			}
			ang := math.Atan2(ob.ax.z, ob.ax.x) + float64(line)*2*math.Pi/3
			d := v3(math.Cos(ang), 0, math.Sin(ang))
			a, b := c.add(v3(0, 0.012, 0)), c.add(d.mul(0.31*k)).add(v3(0, 0.012, 0))
			age := (ob.cut*3 - float64(line) - k) * 2
			ribbon(a, b, 0.01, hot(age+0.4).alpha(0.7))
			bGlowLine.line(a.add(d.mul(0.31*k*0.6)), b, hot(0.1).alpha(0.8))
		}
		return
	}
	const N = 56
	segs := int(N * ob.cut)
	for k := 0; k < segs; k++ {
		age := (ob.cut - float64(k)/N) * 6
		p0 := c.add(rimDir(ob, float64(k)/N).mul(0.31))
		p1 := c.add(rimDir(ob, float64(k+1)/N).mul(0.31))
		ribbon(p0, p1, 0.014, hot(age).alpha(float32(math.Max(0.12, 1-age*0.25))))
		if age < 0.3 {
			bGlowLine.line(p0, p1, hot(age))
		}
	}
}

// rimDir: direction from the base centre to the kerf at progress u (0..1 = full turn)
func rimDir(ob *oldBase, u float64) vec3 {
	span := ob.cutSpan
	if span == 0 {
		span = 2 * math.Pi
	}
	a := ob.cutA + u*span
	return v3(math.Cos(a), 0, math.Sin(a))
}

// a whole base held by a robot: baseLift raises the regular base geometry
func drawCarriedBase(ob *oldBase, c vec3, f vec3) {
	if ob.biteR > 0 {
		drawBittenPlate(c, f, ob.bites, 0, 2*math.Pi, true)
	} else {
		baseLift = c.y
		genBaseGeom(pt2{c.x, c.z}, pt2{c.x, c.z}, 1, true, 0, f)
		baseLift = 0
	}
	shadowBlob(pt2{c.x, c.z}, 0.32, 0.35)
	if rand.Float64() < frameDT*1.5 {
		spawnSmoke(c.add(v3(0, 0.08, 0)), 1)
	}
}

// drawSaw: a boom from the Warden's wrist down to a spinning circular saw sat in the kerf
func drawSaw(r *robot, hand vec3) {
	ob := r.salv
	cp := r.cutPt
	up := v3(0, 1, 0)
	radial := cp.sub(fl(ob.pos, cp.y))
	radial.y = 0
	radial = radial.norm()
	tan := up.cross(radial).norm() // blade plane = (tan, up)
	// the saw unfolds from the hand and swings down onto the kerf (toolK), and folds back
	k := smooth(r.toolK)
	if k <= 0.01 {
		return
	}
	hub := lerpV(hand.add(radial.mul(0.08)), cp.add(up.mul(0.075)), k)
	R := 0.105 * (0.25 + 0.75*k)
	working := k >= 0.999 && r.salvaging
	useMat(mSteel)
	// boom + gearbox
	cyl(&bEnv, hand, hub.add(radial.mul(0.06)).add(up.mul(0.05)), 0.022, 0.02, 12, cMetalDk, true)
	gb := hub.add(radial.mul(0.045))
	bevelBox(&bEnv, gb, tan, up, radial, 0.05, 0.05, 0.025, 0.01, cArmor)
	cyl(&bEnv, gb, hub.sub(radial.mul(0.01)), 0.022, 0.022, 12, cChrome, true)
	// guard over the top half
	useMat(mHazard)
	const gs = 12
	for k := 0; k < gs; k++ {
		a0 := math.Pi * float64(k) / gs
		a1 := math.Pi * float64(k+1) / gs
		d0 := tan.mul(math.Cos(a0)).add(up.mul(math.Sin(a0)))
		d1 := tan.mul(math.Cos(a1)).add(up.mul(math.Sin(a1)))
		p0, p1 := hub.add(d0.mul(R+0.015)), hub.add(d1.mul(R+0.015))
		bEnv.quadF(p0.sub(radial.mul(0.012)), p1.sub(radial.mul(0.012)), p1.add(radial.mul(0.02)), p0.add(radial.mul(0.02)), d0.add(d1).norm(), rgba{0.75, 0.55, 0.1, 1})
		bEnv.quadF(p0.add(radial.mul(0.02)), p1.add(radial.mul(0.02)), hub.add(d1.mul(R*0.5)).add(radial.mul(0.02)), hub.add(d0.mul(R*0.5)).add(radial.mul(0.02)), radial, rgba{0.75, 0.55, 0.1, 1})
	}
	// blade: a thin disc with teeth, spun so fast it's mostly a blur
	useMat(mChrome)
	spin := fxTime() * 47 * k
	const teeth = 28
	for k := 0; k < teeth; k++ {
		a0 := spin + 2*math.Pi*float64(k)/teeth
		a1 := a0 + 2*math.Pi/teeth
		d0 := tan.mul(math.Cos(a0)).add(up.mul(math.Sin(a0)))
		d1 := tan.mul(math.Cos(a1)).add(up.mul(math.Sin(a1)))
		bEnv.triS(hub, hub.add(d0.mul(R)), hub.add(d1.mul(R*0.93)), radial.mul(-1), cChromeEnv)
		bEnv.triS(hub, hub.add(d1.mul(R*0.93)), hub.add(d0.mul(R)), radial, cChromeEnv)
	}
	ringSoft(hub.sub(radial.mul(0.004)), R*0.97, 0.008, rgba{0.9, 0.9, 0.95, 0.25}, 32)
	if !working {
		return
	}
	// glowing contact + spray of sparks thrown along the tangent
	sprite(cp.add(up.mul(0.02)), 0.06, cSpark.alpha(0.7))
	addPoint(cp.add(up.mul(0.12)), cSpark, 2.5+2*rand.Float64(), 3)
	if rand.Float64() < 0.55 {
		d := tan.mul(-0.9).add(up.mul(0.35 + 0.3*rand.Float64())).add(radial.mul(rand.NormFloat64() * 0.25)).norm()
		spawnSparks(cp.add(up.mul(0.015)), d, 1+rand.Intn(2), 1.1)
	}
	r.weldOn = true
}

// the bite of the base whose pieces are being drawn (set by the caller)
var pieceBite []bite

// geometry of one third of a torn plate
func genPiece(c vec3, ax vec3, k int) {
	{
		_, az := baseFrame(ax)
		defer withTex(xf{c, ax, v3(0, 1, 0), az})()
	}
	if len(pieceBite) > 0 {
		a0 := float64(k) * 2 * math.Pi / 3
		drawBittenPlate(c, ax, pieceBite, a0, a0+2*math.Pi/3, false)
		return
	}
	up := v3(0, 1, 0)
	az := ax.cross(up).norm()
	a0 := math.Atan2(ax.z, ax.x) + float64(k)*2*math.Pi/3
	a1 := a0 + 2*math.Pi/3
	const segs = 12
	R := 0.3
	h := 0.028
	pt := func(a, r float64) vec3 { return c.add(v3(math.Cos(a), 0, math.Sin(a)).mul(r)) }
	useMat(mCast)
	for s := 0; s < segs; s++ {
		b0 := a0 + (a1-a0)*float64(s)/segs
		b1 := a0 + (a1-a0)*float64(s+1)/segs
		bEnv.triS(c.add(up.mul(h)), pt(b0, R).add(up.mul(h)), pt(b1, R).add(up.mul(h)), up, cIron.scale(1.2))
		n := v3(math.Cos((b0+b1)/2), 0, math.Sin((b0+b1)/2))
		bEnv.quadF(pt(b0, R), pt(b1, R), pt(b1, R).add(up.mul(h)), pt(b0, R).add(up.mul(h)), n, cIron)
	}
	for _, a := range []float64{a0, a1} {
		n := v3(-math.Sin(a), 0, math.Cos(a))
		bEnv.quadF(c, pt(a, R), pt(a, R).add(up.mul(h)), c.add(up.mul(h)), n, cIron.scale(0.8))
		// glowing fresh cut edge
		bGlowLine.line(c.add(up.mul(h)), pt(a, R).add(up.mul(h)), heatColor(0.35).alpha(0.4))
	}
	// the hardware that stood on this third: two of the six slots (a hydraulic foot or a
	// tether plug), so the pieces match the base they came from
	for j := 0; j < 2; j++ {
		sk := (2*k + j) % 6
		a := math.Atan2(ax.z, ax.x) + slotAngle(sk)
		d := v3(math.Cos(a), 0, math.Sin(a))
		if slotIsCable(sk) {
			pl := pt(a, R*0.78).add(up.mul(h))
			useMat(mCast)
			bevelBox(&bEnv, pl.add(up.mul(0.012)), d, up, d.cross(up), 0.03, 0.018, 0.026, 0.006, cIron)
			useMat(mHazard)
			cyl(&bEnv, pl.add(up.mul(0.024)), pl.add(up.mul(0.05)), 0.022, 0.022, 12, rgba{0.8, 0.6, 0.1, 1}, true)
			continue
		}
		foot := pt(a, R*0.92).add(up.mul(h))
		useMat(mRubber)
		cyl(&bEnv, foot.sub(up.mul(0.006)), foot.add(up.mul(0.02)), 0.04, 0.034, 14, rgba{0.06, 0.06, 0.06, 1}, true)
		useMat(mCast)
		bevelBox(&bEnv, foot.add(up.mul(0.035)), d, up, d.cross(up), 0.03, 0.018, 0.022, 0.006, cIron)
		useMat(mChrome)
		cyl(&bEnv, foot.add(up.mul(0.05)), foot.add(up.mul(0.09)).add(d.mul(-0.025)), 0.012, 0.012, 10, cChromeEnv, false)
	}
	useMat(mCast)
	if k == 0 {
		cyl(&bEnv, c.add(up.mul(h)), c.add(up.mul(h+0.04)), 0.07, 0.065, 14, cIron, true)
	}
	_ = az
}

func debrisNear(p pt2) bool {
	for _, d := range oldBases {
		if d.debris && !d.gone && dist2(d.pos, p) < 2.0 {
			return true
		}
	}
	return false
}

// nextDebris: a Scuttler that just dropped off a chunk heads straight back for the next one
func (e *editor) nextDebris(r *robot) bool {
	for _, d := range oldBases {
		if !d.debris || d.gone || !d.landed || d.claimed != nil || d.fireDebris && fire.phase != fireDone {
			continue
		}
		pl, ok := e.planFor(d, true)
		if !ok {
			continue
		}
		r.salv, r.svPiece = d, -1
		d.claimed = r
		r.svOut, r.svWork, r.svWall = pl.op, pl.work, pl.wall
		r.sv = svReturn
		r.svWait = 1 + rand.Float64()
		r.path = append(e.salvRoute(r.pos, pl.op, true), pl.op)
		return true
	}
	return false
}

func (e *editor) drawOldBases() {
	t := fxTime()
	e.updateDebris(frameDT)
	live := oldBases[:0]
	for _, ob := range oldBases {
		if ob.gone {
			continue
		}
		live = append(live, ob)
		seedTex(15000 + ob.t0*17)
		smokeRate := 1.2
		switch ob.state {
		case 0, 1:
			if ob.biteR > 0 {
				drawBittenPlate(fl(ob.pos, 0), ob.ax, ob.bites, 0, 2*math.Pi, true)
			} else {
				// still part of a shared slab until the robot cuts it free
				genBaseGeom(ob.pos, ob.pos, 1, true, 0, ob.ax, oldJointNbrs[ob]...)
			}
			e.drawKerf(ob)
			shadowBlob(ob.pos, 0.35, 0.4)
			if ob.state == 1 {
				smokeRate = 6
			}
		case 2:
			// drawn by the carrying robot, so it moves exactly with its hands
			continue
		case 3:
			if ob.debris {
				if ob.pieces[0] == 0 {
					c, x, y, z := debrisFrame(ob)
					drawDebris(ob, c, x, y, z)
					if ob.landed {
						shadowBlob(ob.pos, 0.14, 0.4)
					}
				}
				if !ob.landed {
					continue
				}
				smokeRate = 0.5
				break
			}
			c := fl(ob.pos, 0)
			pieceBite = ob.bites
			for k := 0; k < 3; k++ {
				if ob.pieces[k] == 0 {
					genPiece(c, ob.ax, k)
				}
			}
			pieceBite = nil
		}
		pos := fl(ob.pos, 0.08)
		if ob.state == 2 && ob.carrier != nil {
			pos = fl(ob.carrier.pos, 0.6)
		}
		if rand.Float64() < frameDT*smokeRate {
			spawnSmoke(pos, 1)
		}
		if ob.state == 0 && rand.Float64() < frameDT*0.3 {
			spawnSparks(pos, v3(rand.NormFloat64(), 0.6, rand.NormFloat64()).norm(), 3+rand.Intn(5), 0.3)
		}
		if ob.state == 0 {
			fl := float32(0.5 + 0.5*math.Sin(t*6))
			sprite(pos, 0.06, cDanger.alpha(0.7*fl))
		}
	}
	oldBases = live
	if len(regrows) > 0 {
		lr := regrows[:0]
		for _, r := range regrows {
			if t-r.t0 < 2 {
				lr = append(lr, r)
			}
		}
		regrows = lr
	}
}

const liftTime = 2.4

// pieceAt: where a Scuttler's piece is right now (its sector centre, axis), and where its
// magnet grips it. While lifting it travels from the floor up and over onto the bed.
func pieceAt(r *robot, bed, f vec3) (c, ax, grip vec3) {
	ob := r.salv
	c, ax = bed.add(f.mul(0.1)), f
	if r.sv == svLift && r.liftK < 1 {
		k := smooth(math.Max(0, (r.liftK-0.3)/0.7))
		a := fl(ob.pos, 0)
		c = a.add(c.sub(a).mul(k))
		c.y += math.Sin(k*math.Pi) * 0.3
		ax = ob.ax.mul(1 - k).add(f.mul(k))
		if ax.length() < 1e-3 {
			ax = f
		}
		ax = ax.norm()
	}
	am := math.Atan2(ax.z, ax.x) + float64(r.svPiece)*2*math.Pi/3 + math.Pi/3
	grip = c.add(v3(math.Cos(am), 0, math.Sin(am)).mul(0.15)).add(v3(0, 0.05, 0))
	if ob.debris {
		grip = c.add(v3(0, 0.05, 0))
	}
	return
}

// a carried piece on a Scuttler's cargo bed
func (e *editor) drawCarriedPiece(r *robot, bed vec3, f vec3) {
	if r.salv == nil || r.svPiece < 0 {
		return
	}
	baseLift = 0
	c, ax, _ := pieceAt(r, bed, f)
	if r.salv.debris {
		_, az := baseFrame(ax)
		drawDebris(r.salv, c, ax, v3(0, 1, 0), az)
	} else {
		pieceBite = r.salv.bites
		genPiece(c, ax, r.svPiece)
		pieceBite = nil
	}
	if rand.Float64() < frameDT*2 {
		spawnSmoke(bed.add(v3(0, 0.05, 0)), 1)
	}
}

// drawScuttler: a low four-wheeled salvage rover with a plasma cutter arm and a cargo bed
func (e *editor) drawScuttler(r *robot, F xf, hf, hs vec3) {
	t := fxTime()
	s, up, f := F.x, F.y, F.z
	m := r.moving
	rock := 0.008 * m * math.Sin(r.travel*22)
	ch := xf{F.p(0, 0.2+rock, 0), s, up, f}
	// wheels on trailing arms
	for _, sg := range []float64{-1, 1} {
		for _, wz := range []float64{-0.2, 0.2} {
			c := F.p(sg*0.21, 0.085, wz)
			piv := ch.p(sg*0.15, -0.03, wz*0.6)
			armorSeg(piv, c, s, 0.02, 0.02, cFrame)
			cyl(&bEnv, c.sub(s.mul(0.035)), c.add(s.mul(0.035)), 0.085, 0.085, 18, rgba{0.05, 0.05, 0.055, 1}, true)
			cyl(&bEnv, c.sub(s.mul(0.04)), c.add(s.mul(0.04)), 0.045, 0.045, 12, cMetalDk, true)
			// lugs turning with travel
			for k := 0; k < 6; k++ {
				a := -r.travel/0.085 + float64(k)*math.Pi/3
				d := f.mul(math.Cos(a)).add(up.mul(math.Sin(a)))
				p := c.add(d.mul(0.082)).add(s.mul(sg * 0.036))
				bevelBox(&bEnv, p, s, d, d.cross(s), 0.008, 0.008, 0.012, 0.002, cChrome)
			}
		}
	}
	// chassis tub
	bevelBox(&bEnv, ch.o, s, up, f, 0.16, 0.06, 0.27, 0.02, cArmor)
	fastBox(&bEnv, ch, 0, -0.06, 0, 0.13, 0.02, 0.24, cFrame)
	rivetRow(ch.p(-0.14, 0.061, -0.24), ch.p(-0.14, 0.061, 0.24), up, 8, 0.006)
	rivetRow(ch.p(0.14, 0.061, -0.24), ch.p(0.14, 0.061, 0.24), up, 8, 0.006)
	circuitPanel(ch.p(0.161, 0, 0.05), up, f, s, 0.04, 0.09, float32(r.wear))
	// cargo bed with side rails
	bed := ch.p(0, 0.065, -0.11)
	fastBox(&bEnv, ch, 0, 0.066, -0.11, 0.15, 0.005, 0.15, cMetalDk)
	for _, sg := range []float64{-1, 1} {
		a, b := ch.p(sg*0.15, 0.065, -0.26), ch.p(sg*0.15, 0.065, 0.04)
		cyl(&bEnv, a.add(up.mul(0.05)), b.add(up.mul(0.05)), 0.008, 0.008, 6, cChrome, true)
		for _, z := range []float64{-0.25, -0.11, 0.03} {
			p := ch.p(sg*0.15, 0.065, z)
			cyl(&bEnv, p, p.add(up.mul(0.05)), 0.007, 0.007, 6, cChrome, false)
		}
	}
	if r.svPiece >= 0 && r.salv != nil && (r.sv == svCarry || r.sv == svLift) {
		e.drawCarriedPiece(r, bed, f)
	}
	// sensor mast + head
	mast := ch.p(-0.09, 0.06, 0.18)
	head := mast.add(up.mul(0.22))
	cyl(&bEnv, mast, head, 0.014, 0.012, 10, cChrome, false)
	H := xf{head, hs, up, hf}
	bevelBox(&bEnv, head, hs, up, hf, 0.06, 0.035, 0.05, 0.012, cFrame)
	bbox(&bEnv, H, 0, 0.032, 0, 0.055, 0.01, 0.055, 0.005, cArmor)
	eb := float32(0.75 + 0.25*math.Sin(t*9+r.seed))
	if r.speaking {
		eb = float32(0.5 + 0.5*noise1(r.seed, t*30))
	}
	for _, sg := range []float64{-1, 1} {
		ey := H.p(sg*0.028, 0, 0.052)
		cyl(&bEnv, ey.sub(hf.mul(0.01)), ey.add(hf.mul(0.008)), 0.013, 0.011, 10, cMetalDk, true)
		sprite(ey.add(hf.mul(0.01)), 0.018, cRobotEye.alpha(0.95*eb))
	}
	sprite(H.p(0, 0, 0.06), 0.09, cRobotEye.alpha(0.12*eb))
	addPoint(H.p(0, 0, 0.1), cRobotEye, 0.5*float64(eb), 1.6)
	ant := ch.p(0.1, 0.06, -0.24)
	cyl(&bEnv, ant, ant.add(up.mul(0.28)), 0.005, 0.003, 5, cChrome, true)
	if math.Mod(t*1.3+r.seed, 1) < 0.12 {
		sprite(ant.add(up.mul(0.29)), 0.03, cHazard)
	}
	// plasma cutter arm
	sh := ch.p(0.06, 0.07, 0.2)
	tip := ch.p(0.06, 0.2+0.03*math.Sin(t*1.1+r.seed), 0.42)
	rest := tip
	cutting := r.salv != nil && (r.salvaging || r.toolK > 0)
	gripping := false
	var grip vec3
	if cutting {
		tow := fl(r.pos, 0).sub(fl(pt2{r.cutPt.x, r.cutPt.z}, 0)).norm()
		tip = r.cutPt.add(up.mul(0.13)).add(tow.mul(0.03))
	} else if r.sv == svLift && r.svPiece >= 0 && r.salv != nil && r.liftK < 1 {
		_, _, grip = pieceAt(r, bed, f)
		gripping = true
		if r.liftK < 0.3 {
			// reach out to the piece first
			k := smooth(r.liftK / 0.3)
			tip = tip.add(grip.add(up.mul(0.06)).sub(tip).mul(k))
		} else {
			tip = grip.add(up.mul(0.06))
		}
	}
	ball(sh, 0.03, cFrame)
	L1, L2 := 0.28, 0.3
	if gripping || cutting {
		L1, L2 = 0.36, 0.4 // telescoping forearm
	}
	tip, L1, L2 = r.smoothArm(0, F, rest, tip, L1, L2)
	tip = reach(sh, tip, L1, L2)
	el := ik(sh, tip, L1, L2, up.add(f.mul(-0.5)))
	armorSeg(sh, el, s, 0.02, 0.024, cArmor)
	axleJoint(el, s, 0.025, 0.06, cFrame)
	armorSeg(el, tip, s, 0.017, 0.02, cArmor)
	piston(sh.add(up.mul(0.03)), el.add(up.mul(0.025)), 0.006)
	hose(&bEnv, ch.p(0.1, 0.06, 0.05), ch.p(0.12, 0.15, 0.1), el.add(up.mul(0.06)), el.add(s.mul(0.02)), 0.007, cHose, 6)
	td := tip.sub(el).norm()
	if cutting {
		td = r.cutPt.sub(tip).norm()
	}
	if gripping {
		td = v3(0, -1, 0)
	}
	// the head swivels to its new heading rather than snapping
	if r.tdS.length() < 0.5 {
		r.tdS = td
	}
	r.tdS = r.tdS.add(td.sub(r.tdS).mul(math.Min(1, frameDT*7))).norm()
	td = r.tdS
	nz := tip.add(td.mul(0.07))
	cyl(&bEnv, tip, tip.add(td.mul(0.04)), 0.017, 0.015, 10, cMetalDk, true)
	if gripping {
		// electromagnet pad clamped on the piece
		pad := tip.add(td.mul(0.05))
		cyl(&bEnv, tip.add(td.mul(0.02)), pad, 0.012, 0.012, 8, cChrome, true)
		cyl(&bEnv, pad, pad.add(td.mul(0.012)), 0.04, 0.04, 16, cMetalDk, true)
		if r.liftK > 0.3 {
			ringSoft(pad.add(td.mul(0.013)), 0.042, 0.008, rgba{0.5, 0.75, 1, 0.7}, 16)
		}
	} else {
		cyl(&bEnv, tip.add(td.mul(0.04)), nz, 0.011, 0.004, 10, cChrome, true)
	}
	e.jointSparks(r, el, up)
	if cutting && r.salvaging && r.toolK >= 1 {
		e.cutFX(r, nz)
	} else if cutting && r.toolK > 0 {
		sprite(nz, 0.015+0.03*r.toolK, rgba{0.5, 0.75, 1, float32(0.6 * r.toolK)})
	} else if !gripping {
		sprite(nz, 0.012, rgba{0.5, 0.75, 1, 0.6})
	}
}
