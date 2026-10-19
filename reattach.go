//go:build windows

package main

import (
	"math"
	"math/rand"
)

// Planting a ripped generator next to its old base:
//   - close enough (the shaft lands on the old plate): it re-docks - pistons and cables reach
//     back up and lock on, leaving the shaft off-centre where it landed
//   - near but overlapping: the new base's construction shears the overlapping section off the
//     old plate; those chunks fly, land, and a Scuttler collects them one at a time; the part
//     still bolted to the floor is cut up and removed afterwards as usual
const (
	redockDist = 0.21
	baseR      = 0.3
	biteR      = 0.33
)

type bite struct {
	c pt2
	r float64
}

// remainSpans: the radial spans [r0,r1] of plate still present along local angle th
func remainSpans(bites []bite, th float64) [][2]float64 {
	out := [][2]float64{{0, baseR}}
	u := pt2{math.Cos(th), math.Sin(th)}
	for _, b := range bites {
		ub := u[0]*b.c[0] + u[1]*b.c[1]
		disc := ub*ub - (b.c[0]*b.c[0] + b.c[1]*b.c[1]) + b.r*b.r
		if b.r <= 0 || disc <= 0 {
			continue
		}
		sq := math.Sqrt(disc)
		c0, c1 := ub-sq, ub+sq
		var nx [][2]float64
		for _, s := range out {
			if c1 <= s[0] || c0 >= s[1] {
				nx = append(nx, s)
				continue
			}
			if c0 > s[0]+0.005 {
				nx = append(nx, [2]float64{s[0], c0})
			}
			if c1 < s[1]-0.005 {
				nx = append(nx, [2]float64{c1, s[1]})
			}
		}
		out = nx
	}
	return out
}

// local (ax, az) coordinates on a base
func baseFrame(ax vec3) (vec3, vec3) {
	ax.y = 0
	if ax.length() < 1e-6 {
		ax = v3(1, 0, 0)
	}
	ax = ax.norm()
	return ax, ax.cross(v3(0, 1, 0)).norm()
}

func toLocal(ob *oldBase, p pt2) pt2 {
	ax, az := baseFrame(ob.ax)
	d := v3(p[0]-ob.pos[0], 0, p[1]-ob.pos[1])
	return pt2{d.dot(ax), d.dot(az)}
}

// try to re-dock or shear an old base when a ripped shaft is planted at p
// returns true if it re-docked (no new base grows)
var lastDocked *oldBase

func (e *editor) plantNearOld(p pt2, ax vec3) (docked bool, anchor pt2, oax vec3, shearObs []*oldBase) {
	var best *oldBase
	bd := math.Inf(1)
	for _, ob := range oldBases {
		if ob.gone || ob.debris || ob.state >= 2 {
			continue
		}
		if d := dist2(ob.pos, p); d < bd {
			best, bd = ob, d
		}
	}
	if best == nil {
		return
	}
	ob := best
	if bd < redockDist && ob.state == 0 && ob.biteR == 0 {
		ob.gone = true
		e.releaseClaim(ob)
		lastDocked = ob
		return true, ob.pos, ob.ax, nil
	}
	// every old base the new one would overlap gets sheared when the new base grows into it
	for _, o := range oldBases {
		if !o.gone && !o.debris && o.state < 2 && dist2(o.pos, p) < baseR+biteR {
			shearObs = append(shearObs, o)
		}
	}
	return
}

func (e *editor) releaseClaim(ob *oldBase) {
	if r := ob.claimed; r != nil {
		r.salv, r.carryBase, r.svPiece, r.salvaging = nil, nil, -1, false
		if r.state == rbSalvage {
			e.sendHome(r)
		}
	}
	ob.claimed = nil
}

// shear the part of ob that the new base at p will occupy into flying chunks
func (e *editor) shear(ob *oldBase, p pt2) {
	b := toLocal(ob, p)
	ax, az := baseFrame(ob.ax)
	t := fxTime()
	whole := math.Hypot(b[0], b[1]) < 0.3 // the new base swallows the old centre: it all breaks up
	if whole {
		ob.gone = true
		e.releaseClaim(ob)
	}
	prev := ob.bites
	if !whole {
		if ob.biteR == 0 {
			ob.biteC, ob.biteR = b, biteR
		}
		ob.bites = append(ob.bites, bite{b, biteR})
	}
	// chunks: angular slices of the overlap lens (or the whole plate), as seen from the old centre
	const n = 64
	type span struct{ t0, t1 float64 }
	spanAt := func(th float64) (span, bool) {
		u := pt2{math.Cos(th), math.Sin(th)}
		if whole {
			best := span{}
			for _, s := range remainSpans(prev, th) {
				if a := math.Max(0.04, s[0]); s[1]-a > best.t1-best.t0 {
					best = span{a, s[1]}
				}
			}
			return best, best.t1-best.t0 > 0.01
		}
		ub := u[0]*b[0] + u[1]*b[1]
		disc := ub*ub - (b[0]*b[0] + b[1]*b[1]) + biteR*biteR
		if disc <= 0 {
			return span{}, false
		}
		sq := math.Sqrt(disc)
		t0, t1 := math.Max(0, ub-sq), math.Min(baseR, ub+sq)
		// only what's still there: an earlier bite may already have taken some of it
		best := span{}
		for _, s := range remainSpans(prev, th) {
			a, z := math.Max(t0, s[0]), math.Min(t1, s[1])
			if z-a > best.t1-best.t0 {
				best = span{a, z}
			}
		}
		if best.t1-best.t0 < 0.01 {
			return span{}, false
		}
		return best, true
	}
	var slices [][]float64
	var cur []float64
	for k := 0; k <= n; k++ {
		th := float64(k) * 2 * math.Pi / n
		if _, ok := spanAt(th); ok {
			cur = append(cur, th)
		} else if len(cur) > 0 {
			slices = append(slices, cur)
			cur = nil
		}
	}
	if len(cur) > 0 {
		if len(slices) > 0 && len(slices[0]) > 0 && slices[0][0] == 0 {
			slices[0] = append(cur, slices[0]...)
		} else {
			slices = append(slices, cur)
		}
	}
	world2 := func(l pt2) pt2 {
		return pt2{ob.pos[0] + ax.x*l[0] + az.x*l[1], ob.pos[1] + ax.z*l[0] + az.z*l[1]}
	}
	for _, sl := range slices {
		// split each region into chunks of ~6 angular steps
		per := 6
		if whole {
			per = 11
		}
		for i := 0; i < len(sl)-1; i += per {
			j := i + per
			if j >= len(sl) {
				j = len(sl) - 1
			}
			if j-i < 2 {
				continue
			}
			var outer, inner []pt2
			for k := i; k <= j; k++ {
				sp, _ := spanAt(sl[k])
				jag := 0.012 * (hash1(sl[k]*31+ob.t0) - 0.5)
				u := pt2{math.Cos(sl[k]), math.Sin(sl[k])}
				outer = append(outer, pt2{u[0] * sp.t1, u[1] * sp.t1})
				inner = append(inner, pt2{u[0] * (sp.t0 + jag), u[1] * (sp.t0 + jag)})
			}
			var poly []pt2
			poly = append(poly, outer...)
			for k := len(inner) - 1; k >= 0; k-- {
				poly = append(poly, inner[k])
			}
			// centroid -> local shape around it, in world-aligned axes
			var cx, cz float64
			for _, q := range poly {
				cx += q[0]
				cz += q[1]
			}
			cx /= float64(len(poly))
			cz /= float64(len(poly))
			shape := make([]pt2, len(poly))
			for k, q := range poly {
				w := world2(q)
				c := world2(pt2{cx, cz})
				shape[k] = pt2{w[0] - c[0], w[1] - c[1]}
			}
			c := world2(pt2{cx, cz})
			out := v3(c[0]-p[0], 0, c[1]-p[1]).norm()
			sp := 1.1 + rand.Float64()*0.9
			d := &oldBase{
				pos: c, ax: v3(1, 0, 0), t0: t, debris: true, small: true, state: 3,
				shape: shape, pieces: [3]int{0, 2, 2},
				dpos: fl(c, 0.02), dvel: out.mul(sp).add(v3(rand.NormFloat64()*0.2, 1.6+rand.Float64()*1.2, rand.NormFloat64()*0.2)),
				spinAx: v3(rand.NormFloat64(), rand.NormFloat64()*0.3, rand.NormFloat64()).norm(), spinV: 6 + rand.Float64()*10,
			}
			oldBases = append(oldBases, d)
		}
	}
	c := fl(p, 0.05)
	spawnSparks(c, v3(0, 1, 0), 40, 1.0)
	spawnSmoke(c, 8)
	addPoint(c.add(v3(0, 0.1, 0)), cSpark, 10, 4)
	e.sfx(sndRip, c, 0.8)
	e.sfx(sndClang, fl(ob.pos, 0.05), 0.7)
}

// flying chunks: ballistic with tumble, bounce once, settle flat; stay on the floor until collected
func (e *editor) updateDebris(dt float64) {
	for _, d := range oldBases {
		if !d.debris || d.gone || d.landed {
			continue
		}
		d.dvel.y -= 9.8 * dt
		d.dpos = d.dpos.add(d.dvel.mul(dt))
		d.spin += d.spinV * dt
		if rand.Float64() < dt*20 {
			spawnSmoke(d.dpos, 1)
		}
		// keep out of the laser walls' posts and the new bases: deflect off them
		// never come to rest overlapping a base, another chunk, or out past the room walls
		type blk struct {
			p pt2
			r float64
		}
		var blocks []blk
		for _, o := range e.obstacles() {
			blocks = append(blocks, blk{o, 0.5})
		}
		for _, o := range oldBases {
			if o == d || o.gone || o.state == 2 {
				continue
			}
			if o.debris {
				if o.landed {
					blocks = append(blocks, blk{o.pos, 0.3})
				}
			} else {
				blocks = append(blocks, blk{o.pos, 0.48})
			}
		}
		for _, b := range blocks {
			v := v3(d.dpos.x-b.p[0], 0, d.dpos.z-b.p[1])
			if l := v.length(); l < b.r && l > 1e-6 && d.dpos.y < 0.15 {
				n := v.mul(1 / l)
				d.dpos = d.dpos.add(n.mul(b.r - l))
				if vn := d.dvel.dot(n); vn < 0 {
					d.dvel = d.dvel.sub(n.mul(1.6 * vn))
				}
			}
		}
		if world.ok && !inLaneRect(pt2{d.dpos.x, d.dpos.z}, 0.2) {
			d.dvel.x, d.dvel.z = -d.dvel.x*0.5, -d.dvel.z*0.5
		}
		if d.dpos.y <= 0.0 && d.dvel.y < 0 {
			d.dpos.y = 0
			e.sfx(sndClang, d.dpos, 0.35)
			spawnSparks(d.dpos, v3(0, 1, 0), 6, 0.3)
			if d.bounces == 0 && -d.dvel.y > 1.2 {
				d.bounces++
				d.dvel = v3(d.dvel.x*0.45, -d.dvel.y*0.3, d.dvel.z*0.45)
				d.spinV *= 0.4
			} else {
				d.landed = true
				d.pos = pt2{d.dpos.x, d.dpos.z}
				d.t0 = fxTime() // salvage clock starts once it's settled
			}
		}
	}
}

// debrisFrame: orientation of a chunk (tumbling in flight, flat once landed)
func debrisFrame(d *oldBase) (c, x, y, z vec3) {
	c = d.dpos
	if d.landed {
		c = fl(d.pos, 0)
	}
	x, y, z = v3(1, 0, 0), v3(0, 1, 0), v3(0, 0, 1)
	if !d.landed {
		x, y, z = rotAbout(x, d.spinAx, d.spin), rotAbout(y, d.spinAx, d.spin), rotAbout(z, d.spinAx, d.spin)
	}
	return
}

func rotAbout(v, k vec3, a float64) vec3 {
	c, s := math.Cos(a), math.Sin(a)
	return v.mul(c).add(k.cross(v).mul(s)).add(k.mul(k.dot(v) * (1 - c)))
}

// drawDebris: a loose piece - a jagged slab chunk, or a blown-off bell cap lying on its side
func drawDebris(ob *oldBase, c, x, y, z vec3) {
	if !ob.bell {
		drawChunk(ob.shape, c, x, y, z)
		return
	}
	defer withTex(xf{c, x, y, z})()
	m := c.add(y.mul(0.072))
	useMat(material{0, 0.3, 0.45, 0.45})
	cyl(&bEnv, m.sub(x.mul(0.07)), m.add(x.mul(0.07)), 0.072, 0.072, 20, rgba{0.4, 0.32, 0.09, 1}, false)
	hemi(&bEnv, m.add(x.mul(0.07)), x, 0.072, rgba{0.4, 0.32, 0.09, 1}, 4, 20)
	useMat(mCast)
	cyl(&bEnv, m.sub(x.mul(0.07)), m.sub(x.mul(0.085)), 0.072, 0.06, 20, rgba{0.05, 0.05, 0.05, 1}, true) // scorched, torn rim
}

// drawChunk: an extruded jagged slab of base plate, shape in its own (x,z) plane
func drawChunk(shape []pt2, c, x, y, z vec3) {
	const h = 0.028
	if len(shape) < 3 {
		return
	}
	at := func(q pt2, hh float64) vec3 { return c.add(x.mul(q[0])).add(z.mul(q[1])).add(y.mul(hh)) }
	defer withTex(xf{c, x, y, z})()
	useMat(mCast)
	ctr := at(pt2{}, h)
	n := len(shape)
	for i := 0; i < n; i++ {
		a, b := shape[i], shape[(i+1)%n]
		bEnv.triS(ctr, at(b, h), at(a, h), y, cIron.scale(1.2))
		bEnv.triS(at(pt2{}, 0), at(a, 0), at(b, 0), y.mul(-1), cIron.scale(0.7))
		side := at(b, 0).sub(at(a, 0)).cross(y).norm()
		bEnv.quadF(at(a, 0), at(b, 0), at(b, h), at(a, h), side, cIron)
	}
}

// drawBittenPlate: an old base with a bite sheared out of it. Polar spans from the centre:
// the plate is drawn where it isn't inside the bite circle; fresh broken edges glow faintly.
func drawBittenPlate(c vec3, ax vec3, bites []bite, a0, a1 float64, full bool) {
	ax, az := baseFrame(ax)
	defer withTex(xf{c, ax, v3(0, 1, 0), az})()
	up := v3(0, 1, 0)
	const h = 0.028
	steps := int(48 * (a1 - a0) / (2 * math.Pi))
	if steps < 4 {
		steps = 4
	}
	dir := func(th float64) vec3 { return ax.mul(math.Cos(th)).add(az.mul(math.Sin(th))) }
	spans := func(th float64) [][2]float64 { return remainSpans(bites, th) }
	useMat(mCast)
	for i := 0; i < steps; i++ {
		th0 := a0 + (a1-a0)*float64(i)/float64(steps)
		th1 := a0 + (a1-a0)*float64(i+1)/float64(steps)
		s0, s1 := spans(th0), spans(th1)
		if len(s0) != len(s1) {
			s1 = s0
		}
		d0, d1 := dir(th0), dir(th1)
		for k := range s0 {
			p := func(d vec3, r, y float64) vec3 { return c.add(d.mul(r)).add(up.mul(y)) }
			bEnv.quadF(p(d0, s0[k][0], h), p(d0, s0[k][1], h), p(d1, s1[k][1], h), p(d1, s1[k][0], h), up, cIron.scale(1.2))
			// outer rim (with its hazard band) or a broken face
			if s0[k][1] >= baseR-0.001 {
				nn := d0.add(d1).norm()
				bEnv.quadF(p(d0, baseR, 0), p(d1, baseR, 0), p(d1, baseR, h), p(d0, baseR, h), nn, cIron)
				useMat(mHazard)
				bEnv.quadF(p(d0, baseR+0.006, 0.004), p(d1, baseR+0.006, 0.004), p(d1, baseR+0.006, 0.022), p(d0, baseR+0.006, 0.022), nn, rgba{0.8, 0.6, 0.1, 1})
				useMat(mCast)
			} else {
				nn := d0.add(d1).norm()
				bEnv.quadF(p(d0, s0[k][1], 0), p(d1, s1[k][1], 0), p(d1, s1[k][1], h), p(d0, s0[k][1], h), nn, cIron.scale(0.8))
				bGlowLine.line(p(d0, s0[k][1], h), p(d1, s1[k][1], h), heatColor(0.3).alpha(0.35))
			}
			if s0[k][0] > 0.001 {
				nn := d0.add(d1).norm().mul(-1)
				bEnv.quadF(p(d1, s1[k][0], 0), p(d0, s0[k][0], 0), p(d0, s0[k][0], h), p(d1, s1[k][0], h), nn, cIron.scale(0.8))
				bGlowLine.line(p(d0, s0[k][0], h), p(d1, s1[k][0], h), heatColor(0.3).alpha(0.35))
			}
		}
	}
	// straight cut faces when this is one sector of a cut-up plate
	if !full {
		for _, th := range []float64{a0, a1} {
			for _, sp := range spans(th) {
				d := dir(th)
				nn := up.cross(d)
				if th == a1 {
					nn = nn.mul(-1)
				}
				p0, p1 := c.add(d.mul(sp[0])), c.add(d.mul(sp[1]))
				bEnv.quadF(p0, p1, p1.add(up.mul(h)), p0.add(up.mul(h)), nn, cIron.scale(0.8))
			}
		}
	}
	if !full {
		return
	}
	inBite := func(l pt2) bool {
		for _, b := range bites {
			if math.Hypot(l[0]-b.c[0], l[1]-b.c[1]) < b.r+0.04 {
				return true
			}
		}
		return false
	}
	// surviving feet and the torn central crown
	for k := 0; k < 6; k++ {
		if slotIsCable(k) {
			continue
		}
		a := slotAngle(k)
		l := pt2{math.Cos(a) * baseR * 0.92, math.Sin(a) * baseR * 0.92}
		if inBite(l) {
			continue
		}
		foot := c.add(dir(a).mul(baseR * 0.92)).add(up.mul(0.03))
		useMat(mRubber)
		cyl(&bEnv, foot.sub(up.mul(0.006)), foot.add(up.mul(0.02)), 0.04, 0.034, 14, rgba{0.06, 0.06, 0.06, 1}, true)
		useMat(mCast)
		bevelBox(&bEnv, foot.add(up.mul(0.035)), dir(a), up, dir(a).cross(up), 0.03, 0.018, 0.022, 0.006, cIron)
	}
	if !inBite(pt2{}) {
		useMat(mCast)
		cyl(&bEnv, c.add(up.mul(h)), c.add(up.mul(0.075)), 0.105, 0.1, 24, cIron, false)
		useMat(mCircuit)
		cyl(&bEnv, c.add(up.mul(0.04)), c.add(up.mul(0.06)), 0.07, 0.07, 18, rgba{0.1, 0.4, 0.15, 1}, true)
	}
}
