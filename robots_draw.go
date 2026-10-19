//go:build windows

package main

import (
	"math"
	"math/rand"
)

var (
	cArmor  = rgba{0.27, 0.28, 0.31, 1}
	cFrame  = rgba{0.11, 0.12, 0.14, 1}
	cChrome = rgba{0.72, 0.74, 0.78, 1}
	cHose   = rgba{0.05, 0.05, 0.06, 1}
)

func (e *editor) separateRobots() {
	for i := 0; i < len(robots); i++ {
		for j := i + 1; j < len(robots); j++ {
			a, b := robots[i], robots[j]
			dx, dz := b.pos[0]-a.pos[0], b.pos[1]-a.pos[1]
			d := math.Hypot(dx, dz)
			minD := 0.9
			if a.carryBase != nil || b.carryBase != nil {
				minD = 1.35
			}
			if d >= minD {
				continue
			}
			if d < 1e-4 {
				dx, dz, d = 1, 0, 1
			}
			push := (minD - d) / 2
			nx, nz := dx/d, dz/d
			ma, mb := 1.0, 1.0
			if a.state == rbWork {
				ma, mb = 0, 2
			}
			if b.state == rbWork {
				mb = 0
				if a.state != rbWork {
					ma = 2
				}
			}
			move := func(r *robot, sx, sz float64) {
				c := pt2{r.pos[0] + sx, r.pos[1] + sz}
				if len(e.pts) >= 3 && !e.drawing && (pointInPoly(e.pts, c[0], c[1]) || outsideDist(e.pts, c) < robotWallClear) {
					return
				}
				r.pos = c
			}
			move(a, -nx*push*ma, -nz*push*ma)
			move(b, nx*push*mb, nz*push*mb)
		}
	}
}

func fastBox(b *batch, f xf, cx, cy, cz, hx, hy, hz float64, col rgba) {
	p := func(sx, sy, sz float64) vec3 { return f.p(cx+sx*hx, cy+sy*hy, cz+sz*hz) }
	b.quadF(p(1, -1, -1), p(1, 1, -1), p(1, 1, 1), p(1, -1, 1), f.x, col)
	b.quadF(p(-1, -1, 1), p(-1, 1, 1), p(-1, 1, -1), p(-1, -1, -1), f.x.mul(-1), col)
	b.quadF(p(-1, 1, -1), p(-1, 1, 1), p(1, 1, 1), p(1, 1, -1), f.y, col)
	b.quadF(p(-1, -1, -1), p(1, -1, -1), p(1, -1, 1), p(-1, -1, 1), f.y.mul(-1), col)
	b.quadF(p(-1, -1, 1), p(1, -1, 1), p(1, 1, 1), p(-1, 1, 1), f.z, col)
	b.quadF(p(-1, -1, -1), p(-1, 1, -1), p(1, 1, -1), p(1, -1, -1), f.z.mul(-1), col)
}

func segFrame(a, b vec3, side vec3) xf {
	z := b.sub(a).norm()
	x := side.sub(z.mul(side.dot(z)))
	if x.length() < 1e-4 {
		x, _ = basis(z)
	}
	x = x.norm()
	y := z.cross(x).norm()
	return xf{a.add(b).mul(0.5), x, y, z}
}

func armorSeg(a, b, side vec3, hx, hy float64, col rgba) {
	f := segFrame(a, b, side)
	bevelBox(&bEnv, f.o, f.x, f.y, f.z, hx, hy, b.sub(a).length()/2, math.Min(hx, hy)*0.35, col)
}

func piston(a, b vec3, r float64) {
	m := a.add(b.sub(a).mul(0.55))
	cyl(&bEnv, a, m, r*1.7, r*1.7, 10, cFrame, true)
	cyl(&bEnv, m.sub(b.sub(a).mul(0.05)), b, r, r, 10, cChrome, true)
}

func axleJoint(p, ax vec3, r, w float64, col rgba) {
	cyl(&bEnv, p.sub(ax.mul(w/2)), p.add(ax.mul(w/2)), r, r, 14, col, true)
	cyl(&bEnv, p.sub(ax.mul(w/2+0.006)), p.add(ax.mul(w/2+0.006)), r*0.45, r*0.45, 10, cChrome, true)
}

func ik(root, target vec3, l1, l2 float64, bend vec3) vec3 {
	d := target.sub(root)
	L := d.length()
	if L < 1e-4 {
		return root.add(bend.norm().mul(l1))
	}
	dir := d.mul(1 / L)
	L = math.Max(math.Abs(l1-l2)+1e-3, math.Min(l1+l2-1e-3, L))
	a := (l1*l1 + L*L - l2*l2) / (2 * L)
	h := math.Sqrt(math.Max(0, l1*l1-a*a))
	bp := bend.sub(dir.mul(bend.dot(dir)))
	if bp.length() < 1e-4 {
		bp = v3(0, 1, 0)
	}
	return root.add(dir.mul(a)).add(bp.norm().mul(h))
}

func reach(root, target vec3, l1, l2 float64) vec3 {
	d := target.sub(root)
	if L := d.length(); L > l1+l2-0.002 {
		return root.add(d.mul((l1 + l2 - 0.002) / L))
	}
	return target
}

func arc(a, b vec3, col rgba) {
	prev := a
	const n = 9
	for k := 1; k <= n; k++ {
		f := float64(k) / n
		p := a.add(b.sub(a).mul(f))
		if k < n {
			p = p.add(v3(rand.NormFloat64(), rand.NormFloat64(), rand.NormFloat64()).mul(0.012))
		}
		bGlowLine.line(prev, p, rgba{1, 1, 1, 1})
		ribbon(prev, p, 0.02, col.alpha(0.5))
		prev = p
	}
}

func (e *editor) weldFX(r *robot, tip vec3, g pt2) {
	t := fxTime()
	on := noise1(r.seed, t*4) > 0.28
	if !on {
		sprite(tip, 0.02, cHazard.alpha(0.8))
		return
	}
	r.weldOn = true
	sp := v3(g[0], tip.y, g[1])
	n := tip.sub(sp)
	n.y = 0
	n = n.norm()
	hit := sp.add(n.mul(0.045))
	arc(tip, hit, cWeld)
	flare(hit, n, 0.05+0.03*rand.Float64(), cWeld)
	addPoint(hit.add(n.mul(0.15)), cWeld, 2.5+3*rand.Float64(), 4)
	sprite(hit, 0.35+0.1*rand.Float64(), cWeld.alpha(0.12))
	if rand.Float64() < 0.6 {
		if rand.Float64() < 0.4 {
			spawnSparks(hit, n, 1+rand.Intn(2), 0.25)
		}
	}
	floorGlow(pt2{hit.x, hit.z}, 0.9, cWeld.alpha(0.08+0.06*float32(rand.Float64())))
}

func (e *editor) robotLighting(r *robot) {
	bounceAmt = 0
	if len(e.pts) < 3 || e.drawing {
		return
	}
	best, bq := math.Inf(1), pt2{}
	for i := range e.pts {
		a, b := e.pts[i], e.pts[(i+1)%len(e.pts)]
		d, u := segDist(r.pos, a, b)
		if d < best {
			best, bq = d, pt2{a[0] + (b[0]-a[0])*u, a[1] + (b[1]-a[1])*u}
		}
	}
	bounceDir = v3(bq[0]-r.pos[0], 0.3, bq[1]-r.pos[1]).norm()
	bounceCol = cEnergy
	bounceAmt = 0.45 * math.Exp(-best/0.9)
}

func (e *editor) robotRoles(r *robot) {
	w := float32(r.wear)
	armorMat := material{0, 0.12, 0.42, w}
	armorCol := r.paint
	if r.chrome {
		armorMat = material{1, 1, 0.05, w * 0.5}
		armorCol = rgba{0.88, 0.89, 0.92, 1}
	}
	roles = []role{
		{cArmor, armorCol, armorMat},
		{cFrame, rgba{0.15, 0.15, 0.16, 1}, material{7, 0.85, 0.5, w}},
		{cChrome, rgba{0.86, 0.87, 0.9, 1}, material{1, 1, 0.07, w * 0.4}},
		{cMetalDk, rgba{0.32, 0.33, 0.35, 1}, material{1, 0.95, 0.38, w}},
		{cMetal, rgba{0.6, 0.62, 0.64, 1}, material{1, 1, 0.3, w}},
		{cHose, cHose, material{2, 0, 0.7, w}},
		{cHazard, rgba{0.8, 0.6, 0.1, 1}, material{6, 0.1, 0.6, w}},
	}
}

func rivetRow(a, b, n vec3, count int, r float64) {
	for k := 0; k < count; k++ {
		p := a.add(b.sub(a).mul((float64(k) + 0.5) / float64(count)))
		cyl(&bEnv, p, p.add(n.mul(r*0.6)), r, r*0.7, 5, cMetal, true)
	}
}

func circuitPanel(c, ax, ay, n vec3, hw, hh float64, wear float32) {
	defer withTex(xf{c, ax, ay, n})() // board pattern fixed to the board, not the world
	sm := curMat
	curMat = material{4, 0.1, 0.45, wear}
	p := func(x, y float64) vec3 { return c.add(ax.mul(x)).add(ay.mul(y)).add(n.mul(0.002)) }
	bEnv.quadF(p(-hw, -hh), p(hw, -hh), p(hw, hh), p(-hw, hh), n, rgba{0.1, 0.4, 0.15, 1})
	curMat = mSteel
	for k := 0; k < 3; k++ {
		x := -hw*0.6 + float64(k)*hw*0.6
		cc := p(x, hh*0.5)
		bevelBox(&bEnv, cc.add(n.mul(0.006)), ax, ay, n, hw*0.18, hh*0.15, 0.005, 0.002, rgba{0.05, 0.05, 0.06, 1})
	}
	curMat = sm
}

func (e *editor) jointSparks(r *robot, p, n vec3) {
	if r.wear < 0.55 || r.moving < 0.3 {
		return
	}
	if rand.Float64() < frameDT*0.05*(r.wear-0.5)*2 {
		spawnSparks(p, n, 8+rand.Intn(12), 0.5)
		e.sfx(sndSpark, p, 0.35)
		sparkBursts = append(sparkBursts, sparkBurst{p: p, t: fxTime(), k: 0.6})
	}
}

func (e *editor) drawRobot(r *robot) {
	e.robotLighting(r)
	e.robotRoles(r)
	segMul = 1
	if dist2(r.pos, pt2{camPos.x, camPos.z}) < 3.5 {
		if !potatoMode {
			segMul = 2
		}
	}
	saveC := primCounter
	primCounter = float64(r.vid) * 977
	defer func() { roles, segMul, primCounter = nil, 1, saveC }()
	f := v3(math.Cos(r.yaw), 0, math.Sin(r.yaw))
	s := v3(-f.z, 0, f.x)
	up := v3(0, 1, 0)
	base := fl(r.pos, 0)
	F := xf{base, s, up, f}
	defer withTex(F)() // textures ride with the robot instead of sliding over it
	hf := f.mul(math.Cos(r.headYaw)).add(s.mul(math.Sin(r.headYaw)))
	hs := v3(-hf.z, 0, hf.x)
	var gp pt2
	working := r.state == rbWork && r.gen >= 0 && r.gen < len(e.pts)
	if working {
		gp = e.pts[r.gen]
	}
	t := fxTime()
	wy := 0.55 + 1.0*(0.5+0.5*math.Sin(t*0.35+r.seed))
	armBlend = 1
	if r.state == rbFire && (r.firePh == fpRepair || r.firePh == fpFinish) && r.rpK > 0 {
		// the repair tools swing out to the box (and back) instead of snapping there
		working = true
		armBlend = smooth(r.rpK)
		gp = pt2{fire.j.base.x, fire.j.base.z}
		wy = fire.j.c.y + 0.12*math.Sin(t*0.8+r.seed)
	}
	switch r.kind {
	case kindScuttler:
		shadowBlob(r.pos, 0.38, 0.6)
		e.drawScuttler(r, F, hf, hs)
	case kindWarden:
		shadowBlob(r.pos, 0.55, 0.6)
		e.drawWarden(r, F, hf, hs, working, gp, wy)
	default:
		shadowBlob(r.pos, 0.6, 0.65)
		e.drawTread(r, F, hf, hs, working, gp, wy)
	}
}

func (e *editor) eyeLaser(r *robot, eye vec3) {
	t := fxTime()
	tg := e.hmd.pos().add(v3(0, -0.35, 0))
	switch {
	case r.state == rbLook:
	case r.state == rbTalk && r.partner != nil && r.speaking && noise1(r.seed*3, t*0.7) > 0.7:
		h := 1.55
		if r.partner.kind == kindTread {
			h = 0.55
		}
		tg = fl(r.partner.pos, h)
	default:
		return
	}
	dir := tg.sub(eye)
	if dl := dir.length(); dl > 0.5 {
		end := eye.add(dir.mul((dl - 0.15) / dl))
		a := float32(0.35 + 0.15*noise1(r.seed, t*9))
		ribbon(eye, end, 0.006, cRobotEye.alpha(a))
		bGlowLine.line(eye, end, cRobotEye.alpha(a))
		sprite(end, 0.03, cRobotEye.alpha(0.6))
	}
}

// smoothArm: the arm's target is its natural (swinging) pose plus a job-specific offset; the
// offset rides a critically damped spring in the robot's own frame, so switching job - cut,
// lift, carry, weld, rest - is a smooth move, never a jump. Arm lengths ease the same way.
func (r *robot) smoothArm(slot int, F xf, rest, target vec3, L1, L2 float64) (vec3, float64, float64) {
	d := target.sub(rest)
	loc := v3(d.dot(F.x), d.dot(F.y), d.dot(F.z))
	t := fxTime()
	if !r.armInit[slot] {
		r.armInit[slot] = true
		r.armOff[slot], r.armVel[slot] = loc, vec3{}
		r.armL[slot] = [2]float64{L1, L2}
		r.armT[slot] = t
	} else if t > r.armT[slot] {
		dt := math.Min(0.05, t-r.armT[slot])
		r.armT[slot] = t
		const w = 9.0
		acc := loc.sub(r.armOff[slot]).mul(w * w).sub(r.armVel[slot].mul(2 * w))
		r.armVel[slot] = r.armVel[slot].add(acc.mul(dt))
		r.armOff[slot] = r.armOff[slot].add(r.armVel[slot].mul(dt))
		k := math.Min(1, dt*6)
		r.armL[slot][0] += (L1 - r.armL[slot][0]) * k
		r.armL[slot][1] += (L2 - r.armL[slot][1]) * k
	}
	o := r.armOff[slot]
	return rest.add(F.x.mul(o.x)).add(F.y.mul(o.y)).add(F.z.mul(o.z)), r.armL[slot][0], r.armL[slot][1]
}

// segDist3: closest distance between segments ab and cd (sampled; plenty for clearance tests)
func segDist3(a, b, c, d vec3) float64 {
	best := 1e9
	for i := 0; i <= 8; i++ {
		p := lerpV(a, b, float64(i)/8)
		for j := 0; j <= 8; j++ {
			best = math.Min(best, p.sub(lerpV(c, d, float64(j)/8)).length())
		}
	}
	return best
}

// braceHand: where a Warden's claw holds the base down while its saw arm works the rim.
// Candidates sit on the plate in front of the robot; the one giving the most clearance from
// the saw arm, boom and blade wins, with a little stickiness so it doesn't dither.
func (r *robot) braceHand(T xf, s, up, cp, tow vec3) vec3 {
	g := fl(r.salv.pos, 0.06)
	toR := fl(r.pos, 0.06).sub(g).norm()
	side := toR.cross(up).norm()
	if side.dot(s) > 0 {
		side = side.mul(-1) // +side points to the robot's left
	}
	shL, shR := T.p(-0.34, 0.15, 0), T.p(0.34, 0.15, 0)
	sawHand := cp.add(up.mul(0.42)).add(tow.mul(0.12))
	hub := cp.add(up.mul(0.075))
	elR := lerpV(shR, sawHand, 0.5).add(s.mul(0.12))
	var cs []vec3
	for _, a := range []float64{0, 0.6, 1.1, -0.6, -1.1} {
		for _, rr := range []float64{0.1, 0.2} {
			d := toR.mul(math.Cos(a)).add(side.mul(math.Sin(a)))
			cs = append(cs, g.add(d.mul(rr)))
		}
	}
	bestI, bestS := 0, -1e9
	for k, p := range cs {
		if p.sub(shL).length() > 1.05 {
			continue
		}
		elL := lerpV(shL, p, 0.5).add(s.mul(-0.12))
		cl := math.Min(segDist3(shL, elL, shR, elR), segDist3(elL, p, shR, elR))
		cl = math.Min(cl, segDist3(elL, p, elR, sawHand))
		cl = math.Min(cl, segDist3(elL, p, sawHand, hub))
		cl = math.Min(cl, p.sub(cp).length()-0.12) // keep fingers off the blade
		sc := cl - 0.15*p.sub(g.add(toR.mul(0.2)).add(side.mul(0.1))).length()
		if k == r.braceI {
			sc += 0.04
		}
		if sc > bestS {
			bestI, bestS = k, sc
		}
	}
	r.braceI = bestI
	return cs[bestI]
}

// armBlend: how far a fire-crew robot's tool arm has swung from its carry pose to the job
var armBlend = 1.0

func lerpV(a, b vec3, k float64) vec3 { return a.add(b.sub(a).mul(k)) }

// co2Aim: where a fire-crew horn points from p - at the fire while blasting, dipped between
func co2Aim(r *robot, p vec3) vec3 {
	tgt := fire.j.c.add(fire.j.n.mul(0.05))
	d := tgt.sub(p).norm()
	return d.add(v3(0, -0.45*(1-r.aimUp), 0)).norm()
}

func (e *editor) drawWarden(r *robot, F xf, hf, hs vec3, working bool, gp pt2, wy float64) {
	t := fxTime()
	m := r.moving
	s, up, f := F.x, F.y, F.z
	base := F.o
	bob := 0.035 * m * math.Abs(math.Sin(r.phase))
	cutting := r.salv != nil && (r.salvaging || r.toolK > 0)
	if cutting || r.carryBase != nil && r.liftK < 1 {
		r.crouch += (0.3 - r.crouch) * math.Min(1, frameDT*2)
	} else {
		r.crouch += (0 - r.crouch) * math.Min(1, frameDT*2)
	}
	bob += r.crouch
	pel := F.p(0, 1.02-bob, -0.02)
	bevelBox(&bEnv, pel, s, up, f, 0.19, 0.07, 0.12, 0.025, cArmor)
	fastBox(&bEnv, xf{pel, s, up, f}, 0, -0.05, 0.11, 0.08, 0.04, 0.02, cFrame)

	for i, sg := range []float64{-1, 1} {
		ph := r.phase + float64(i)*math.Pi
		fz := 0.26 * math.Sin(ph) * m
		lift := math.Max(0, math.Cos(ph)) * 0.15 * m
		hip := F.p(sg*0.2, 1.0-bob, -0.02)
		ankle := F.p(sg*0.22, 0.13+lift, fz-0.04)
		knee := ik(hip, ankle, 0.5, 0.5, f.mul(-1).add(up.mul(-0.1)).add(f.mul(r.crouch*2)))
		axleJoint(hip, s, 0.07, 0.13, cFrame)
		armorSeg(hip, knee, s, 0.055, 0.065, cArmor)
		piston(hip.add(f.mul(0.06)).add(up.mul(-0.04)), knee.add(f.mul(0.05)).add(up.mul(-0.05)), 0.011)
		axleJoint(knee, s, 0.055, 0.12, cFrame)
		e.jointSparks(r, knee, f.mul(-1))
		bevelBox(&bEnv, knee.add(f.mul(-0.05)), s, up, f, 0.045, 0.05, 0.03, 0.012, cArmor)
		sprite(knee.add(f.mul(-0.085)), 0.015, cRobotEye.alpha(0.9))
		cyl(&bEnv, knee, ankle, 0.036, 0.028, 14, cFrame, false)
		armorSeg(knee.add(ankle.sub(knee).mul(0.1)), knee.add(ankle.sub(knee).mul(0.7)).add(f.mul(0.03)), s, 0.03, 0.025, cArmor)
		ball(ankle, 0.042, cMetalDk)
		foot := F.p(sg*0.22, 0.04+lift*0.7, fz)
		ff := xf{foot, s, up, f}
		for _, toe := range []float64{-0.06, 0, 0.06} {
			a := ff.p(toe*0.5, 0.02, 0.0)
			b := ff.p(toe, -0.015, 0.2)
			armorSeg(a, b, s, 0.018, 0.016, cMetalDk)
			bevelBox(&bEnv, b.add(f.mul(0.02)), s, up, f, 0.012, 0.01, 0.025, 0.005, cChrome)
		}
		armorSeg(ff.p(0, 0.02, 0), ff.p(0, -0.01, -0.13), s, 0.02, 0.018, cMetalDk)
		cyl(&bEnv, ankle, ff.p(0, 0.02, 0.02), 0.024, 0.024, 10, cChrome, false)
	}

	p := 0.32
	ay := up.mul(math.Cos(p)).add(f.mul(math.Sin(p)))
	az := f.mul(math.Cos(p)).sub(up.mul(math.Sin(p)))
	for k := 0; k < 4; k++ {
		a := pel.add(ay.mul(0.06 + float64(k)*0.045))
		cyl(&bEnv, a, a.add(ay.mul(0.035)), 0.085-0.004*float64(k), 0.085-0.004*float64(k), 16, cFrame, true)
	}
	tc := pel.add(ay.mul(0.42))
	T := xf{tc, s, ay, az}
	bevelBox(&bEnv, tc, s, ay, az, 0.27, 0.22, 0.15, 0.05, cFrame)
	for _, sg := range []float64{-1, 1} {
		pc := T.p(sg*0.12, 0.05, 0.13)
		bevelBox(&bEnv, pc, T.d(1, 0, sg*0.25), ay, T.d(-sg*0.25, 0, 1), 0.12, 0.13, 0.04, 0.02, cArmor)
	}
	bbox(&bEnv, T, 0, -0.14, 0.12, 0.16, 0.05, 0.035, 0.015, cArmor)
	core := T.p(0, 0.03, 0.165)
	cyl(&bEnv, core.sub(az.mul(0.01)), core.add(az.mul(0.012)), 0.05, 0.05, 18, cMetalDk, true)
	cp := float32(0.6 + 0.4*math.Sin(t*3+r.seed))
	sprite(core.add(az.mul(0.015)), 0.06, cRobotEye.alpha(0.8*cp))
	sprite(core.add(az.mul(0.015)), 0.018, rgba{1, 0.7, 0.6, cp})
	ringSoft(core.add(az.mul(0.014)), 0.04, 0.01, cRobotEye.alpha(0.6*cp), 16)

	for _, yy := range []float64{-0.2, 0.2} {
		rivetRow(T.p(-0.24, yy, 0.151), T.p(0.24, yy, 0.151), az, 9, 0.008)
	}
	circuitPanel(T.p(0.19, -0.05, 0.151), s, ay, az, 0.05, 0.07, float32(r.wear))
	eb0 := 0.8 + 0.2*math.Sin(t*7)
	if r.wear > 0.6 && noise1(r.seed, t*3) < 0.15 {
		eb0 = 0.15
	}
	addPoint(T.p(0, 0.03, 0.2), cRobotEye, 0.5*eb0, 2)

	bp := T.p(0, 0.02, -0.21)
	bevelBox(&bEnv, bp, s, ay, az, 0.18, 0.19, 0.08, 0.03, cArmor)
	circuitPanel(T.p(-0.07, 0.02, -0.292), s, ay, az.mul(-1), 0.07, 0.12, float32(r.wear))
	fc := T.p(0.09, 0.08, -0.292)
	cyl(&bEnv, fc, fc.add(az.mul(-0.012)), 0.055, 0.055, 20, cMetalDk, false)
	for k := 0; k < 5; k++ {
		a := t*18 + float64(k)*2*math.Pi/5
		d := s.mul(math.Cos(a)).add(ay.mul(math.Sin(a)))
		tn := d.cross(az)
		bevelBox(&bEnv, fc.add(d.mul(0.027)).add(az.mul(-0.006)), d, tn.mul(0.9).add(az.mul(0.3)).norm(), az.mul(0.9).sub(tn.mul(0.3)).norm(), 0.024, 0.012, 0.002, 0.001, cMetal)
	}
	rivetRow(T.p(-0.17, 0.2, -0.29), T.p(0.17, 0.2, -0.29), az.mul(-1), 7, 0.007)
	for k := 0; k < 4; k++ {
		y := -0.12 + float64(k)*0.07
		a, b := T.p(-0.12, 0.02+y, -0.295), T.p(0.12, 0.02+y, -0.295)
		ribbon(a, b, 0.018, cSpark.alpha(0.35))
		bGlowLine.line(a, b, cSpark.alpha(0.7))
	}
	for _, sg := range []float64{-1, 1} {
		ex := T.p(sg*0.12, 0.2, -0.25)
		cyl(&bEnv, ex, ex.add(ay.mul(0.12)).add(az.mul(-0.03)), 0.035, 0.03, 14, cMetalDk, true)
		sprite(ex.add(ay.mul(0.13)).add(az.mul(-0.03)), 0.05, cSpark.alpha(float32(0.3+0.2*noise1(sg, t*6))))
	}
	ant := T.p(0.14, 0.2, -0.2)
	cyl(&bEnv, ant, ant.add(up.mul(0.32)), 0.008, 0.004, 6, cChrome, true)
	if math.Mod(t+r.seed, 1.2) < 0.15 {
		sprite(ant.add(up.mul(0.33)), 0.04, cRobotEye)
	}

	neck := T.p(0, 0.2, 0.1)
	head := neck.add(hf.mul(0.13)).add(up.mul(0.05))
	cyl(&bEnv, neck, head, 0.04, 0.035, 12, cChrome, false)
	for _, sg := range []float64{-1, 1} {
		a := T.p(sg*0.08, 0.17, -0.12)
		hose(&bEnv, a, a.add(up.mul(0.12)), head.add(hs.mul(sg*0.08)).add(up.mul(0.08)), head.add(hs.mul(sg*0.07)).add(hf.mul(-0.08)), 0.012, cHose, 6)
	}
	H := xf{head, hs, up, hf}
	bevelBox(&bEnv, head, hs, up, hf, 0.12, 0.075, 0.14, 0.03, cFrame)
	bbox(&bEnv, H, 0, 0.07, -0.01, 0.105, 0.025, 0.15, 0.015, cArmor)
	bbox(&bEnv, H, 0, -0.07, 0.03, 0.08, 0.025, 0.1, 0.012, cArmor)
	eye := H.p(0, 0.005, 0.143)
	eb := float32(0.75 + 0.25*math.Sin(t*7))
	if r.state == rbLook {
		eb = 1
	}
	a, b := H.p(-0.095, 0.005, 0.142), H.p(0.095, 0.005, 0.142)
	ribbon(a, b, 0.02, cRobotEye.alpha(eb))
	ribbon(a, b, 0.006, rgba{1, 0.8, 0.7, eb})
	if r.speaking {
		eb = float32(0.6 + 0.4*noise1(r.seed, t*30))
	}
	if r.wear > 0.6 && noise1(r.seed*2, t*2.5) < 0.12 {
		eb *= 0.1
	}
	sprite(eye, 0.12, cRobotEye.alpha(0.18*eb))
	addPoint(eye.add(hf.mul(0.05)), cRobotEye, 0.9*float64(eb), 2.6)
	for _, sg := range []float64{-1, 1} {
		pod := H.p(sg*0.13, 0.0, 0.05)
		cyl(&bEnv, pod.sub(hs.mul(sg*0.02)), pod.add(hs.mul(sg*0.03)), 0.03, 0.03, 12, cMetalDk, true)
		cyl(&bEnv, pod.add(hf.mul(0.0)), pod.add(hf.mul(0.05)), 0.016, 0.014, 10, cMetalDk, true)
		sprite(pod.add(hf.mul(0.055)), 0.02, cRobotEye.alpha(0.9))
	}
	e.eyeLaser(r, eye)
	var carryC vec3
	if r.carryBase != nil {
		carryC = liftPos(r, F.p(0, 0.86, 0.45))
		drawCarriedBase(r.carryBase, carryC, carryAxis(r))
	}

	// the CO2 bottle: on the back while stowed, carried in the claw while fighting the fire
	fireK := 0.0
	var bottleNeck vec3
	if r.fireCrew {
		if r.state == rbFire {
			fireK = r.exK
		}
		mount := F.p(-0.16, 1.02, -0.3)
		bb := mount
		if fireK > 0.35 {
			u := smooth((fireK - 0.35) / 0.35)
			bb = lerpV(mount, F.p(-0.12, 0.66, 0.36), u).add(up.mul(0.12 * math.Sin(u*math.Pi)))
		}
		drawExtinguisher(bb, up)
		bottleNeck = bb.add(up.mul(0.36))
		if fireK <= 0.7 {
			// horn clipped to the side of the bottle on its coiled hose
			hb := bb.add(up.mul(0.24)).add(s.mul(0.08))
			co2Hose(bottleNeck.add(up.mul(0.02)), hb, up.mul(-1))
			drawCO2Horn(hb, up.mul(-1))
		}
	}
	for i, sg := range []float64{-1, 1} {
		shp := T.p(sg*0.34, 0.15, 0)
		ball(shp, 0.06, cFrame)
		bevelBox(&bEnv, shp.add(up.mul(0.07)).add(s.mul(sg*0.02)), T.d(1, sg*0.35, 0), T.d(-sg*0.35, 1, 0), az, 0.11, 0.055, 0.14, 0.03, cArmor)
		bevelBox(&bEnv, shp.add(up.mul(0.0)).add(s.mul(sg*0.09)), T.d(1, sg*0.6, 0), T.d(-sg*0.6, 1, 0), az, 0.04, 0.07, 0.12, 0.015, cArmor)
		swing := math.Sin(r.phase+float64(i)*math.Pi) * 0.14 * m
		hand := shp.add(up.mul(-0.62)).add(f.mul(0.22 + swing)).add(s.mul(sg * 0.06))
		rest := hand
		if working {
			g := v3(gp[0], 0, gp[1])
			tow := base.sub(g)
			tow.y = 0
			tow = tow.norm()
			ts := v3(-tow.z, 0, tow.x)
			wh := hand
			_ = ts
			// each hand stays on its own side of the body so the arms never cross
			if i == 1 {
				wh = g.add(tow.mul(0.14)).add(s.mul(0.03 + 0.03*math.Sin(t*2.3))).add(up.mul(wy + 0.04*math.Sin(t*5)))
			} else {
				wh = g.add(tow.mul(0.1)).add(s.mul(-0.09)).add(up.mul(wy + 0.28))
			}
			hand = lerpV(hand, wh, armBlend)
		}
		var hornAim vec3
		hornK := 0.0
		if fireK > 0 {
			if i == 0 {
				// claw: reach back to the bottle on the back, then carry it round in front
				if fireK < 0.35 {
					hand = lerpV(hand, bottleNeck, smooth(fireK/0.35))
				} else {
					hand = bottleNeck
				}
			} else {
				// tool hand: take the horn off the bottle, then bring it up to aim
				grab := bottleNeck.add(s.mul(0.08)).add(up.mul(-0.04))
				if fireK > 0.7 {
					hand = lerpV(hand, grab, smooth((fireK-0.7)/0.12))
				}
				if fireK > 0.82 {
					hornK = smooth((fireK - 0.82) / 0.18)
					hornAim = co2Aim(r, shp)
					hand = lerpV(grab, shp.add(hornAim.mul(0.58)).add(s.mul(-0.05)), hornK)
				}
			}
		}
		L1, L2 := 0.4, 0.46
		if cutting {
			cp := r.cutPt
			tow := fl(r.pos, cp.y).sub(cp).norm()
			if i == 1 {
				// saw boom held over the kerf
				hand = cp.add(up.mul(0.42)).add(tow.mul(0.12))
				L1, L2 = 0.52, 0.62
			} else {
				// the bracing hand: pick a spot on the plate that keeps this arm well clear of
				// the saw arm and the blade wherever the cut has got to round the rim
				hand = r.braceHand(T, s, up, cp, tow)
				L1, L2 = 0.52, 0.62
			}
		}
		if r.carryBase != nil {
			hand = carryC.add(s.mul(sg * 0.29)).add(up.mul(0.03))
			L1, L2 = 0.48, 0.55
		}
		hand, L1, L2 = r.smoothArm(i, F, rest, hand, L1, L2)
		hand = reach(shp, hand, L1, L2)
		elbow := ik(shp, hand, L1, L2, f.mul(-1).add(s.mul(sg*0.6)).add(up.mul(-0.3)))
		cyl(&bEnv, shp, elbow, 0.04, 0.035, 12, cFrame, false)
		armorSeg(shp.add(elbow.sub(shp).mul(0.15)), shp.add(elbow.sub(shp).mul(0.8)), s, 0.05, 0.05, cArmor)
		axleJoint(elbow, s, 0.045, 0.1, cFrame)
		e.jointSparks(r, elbow, f.mul(-1).add(up.mul(-0.5)).norm())
		armorSeg(elbow, hand, s, 0.05, 0.058, cArmor)
		ed := hand.sub(elbow).norm()
		piston(elbow.add(up.mul(0.05)), hand.sub(ed.mul(0.06)).add(up.mul(0.05)), 0.009)
		hose(&bEnv, bp.add(s.mul(sg*0.15)), bp.add(s.mul(sg*0.3)).add(up.mul(0.1)), elbow.add(up.mul(0.15)), elbow.add(f.mul(-0.03)), 0.011, cHose, 6)
		fs, fu := basis(ed)
		if i == 1 {
			cyl(&bEnv, hand, hand.add(ed.mul(0.1)), 0.028, 0.024, 14, cMetalDk, true)
			for k := 0; k < 3; k++ {
				c := hand.add(ed.mul(0.02 + float64(k)*0.03))
				cyl(&bEnv, c, c.add(ed.mul(0.008)), 0.032, 0.032, 14, cChrome, true)
			}
			nz := hand.add(ed.mul(0.1))
			torch := 0.06
			if r.fireCrew {
				torch = 0.06 * armBlend // the torch head slides out of the tool body
				if !working {
					torch = 0
				}
			}
			if cutting {
				torch *= 1 - smooth(r.toolK) // the welding tip folds away as the saw comes out
			}
			tip := nz.add(ed.mul(torch))
			if cutting {
				drawSaw(r, hand)
			}
			if torch > 0.004 {
				cyl(&bEnv, nz, tip, 0.018, 0.007+0.011*(1-torch/0.06), 12, cChrome, true)
			}
			if fireK > 0.7 {
				// the CO2 horn in the tool hand, on its hose from the bottle valve
				hd := ed
				if hornK > 0 {
					hd = ed.mul(1 - hornK).add(hornAim.mul(hornK)).norm()
				}
				hb := hand.add(ed.mul(0.05)).add(fs.mul(0.03))
				co2Hose(bottleNeck.add(up.mul(0.02)), hb, hd)
				htip := drawCO2Horn(hb, hd)
				r.hornTip, r.hornDir, r.hornT = htip, hd, fxTime()
				if r.sprayOn {
					e.sprayFX(r, htip, hd)
				}
			}
			if cutting || r.sprayOn {
			} else if working && armBlend >= 0.999 {
				e.weldFX(r, tip, gp)
			} else if torch > 0.004 {
				sprite(tip, 0.015, cWeld.alpha(0.7))
			}
		} else {
			bevelBox(&bEnv, hand.add(ed.mul(0.03)), fs, fu, ed, 0.035, 0.035, 0.03, 0.01, cMetalDk)
			open := 0.5 + 0.3*math.Sin(t*1.7+r.seed)
			for k := 0; k < 3; k++ {
				a := float64(k)*2*math.Pi/3 + 0.4
				off := fs.mul(math.Cos(a)).add(fu.mul(math.Sin(a)))
				k0 := hand.add(ed.mul(0.05)).add(off.mul(0.028))
				k1 := k0.add(ed.mul(0.06)).add(off.mul(0.03 * open))
				k2 := k1.add(ed.mul(0.055)).add(off.mul(-0.03))
				ball(k0, 0.011, cChrome)
				cyl(&bEnv, k0, k1, 0.01, 0.009, 8, cMetalDk, false)
				ball(k1, 0.01, cChrome)
				cyl(&bEnv, k1, k2, 0.009, 0.003, 8, cMetalDk, true)
			}
		}
	}
}

func treadPath(s float64) (float64, float64, float64, float64) {
	const L, R = 0.6, 0.1
	P := 2*L + 2*math.Pi*R
	s = math.Mod(math.Mod(s, P)+P, P)
	switch {
	case s < L:
		return -L/2 + s, 0.2, 1, 0
	case s < L+math.Pi*R:
		a := (s - L) / R
		return L/2 + R*math.Sin(a), 0.1 + R*math.Cos(a), math.Cos(a), -math.Sin(a)
	case s < 2*L+math.Pi*R:
		return L/2 - (s - L - math.Pi*R), 0.0, -1, 0
	}
	a := (s - 2*L - math.Pi*R) / R
	return -L/2 - R*math.Sin(a), 0.1 - R*math.Cos(a), -math.Cos(a), math.Sin(a)
}

func (e *editor) drawTread(r *robot, F xf, hf, hs vec3, working bool, gp pt2, wy float64) {
	t := fxTime()
	s, up, f := F.x, F.y, F.z
	base := F.o
	P := 2*0.6 + 2*math.Pi*0.1
	for _, sg := range []float64{-1, 1} {
		x := sg * 0.29
		bbox(&bEnv, F, x-sg*0.045, 0.1, 0, 0.012, 0.085, 0.33, 0.01, cArmor)
		bbox(&bEnv, F, x+sg*0.045, 0.1, 0, 0.012, 0.075, 0.3, 0.01, cFrame)
		for _, wz := range []float64{-0.3, -0.1, 0.1, 0.3} {
			rr := 0.085
			if math.Abs(wz) > 0.2 {
				rr = 0.095
			}
			c := F.p(x, 0.1, wz)
			cyl(&bEnv, c.sub(s.mul(0.04)), c.add(s.mul(0.04)), rr, rr, 16, cMetalDk, true)
			cyl(&bEnv, c.sub(s.mul(0.046)), c.add(s.mul(0.046)), 0.03, 0.03, 10, cChrome, true)
		}
		const links = 30
		off := r.travel * 1.0
		if sg > 0 && r.state == rbWalk {
			off *= 1.0
		}
		for k := 0; k < links; k++ {
			z, y, tz, ty := treadPath(off + float64(k)*P/links)
			tan := f.mul(tz).add(up.mul(ty))
			nrm := tan.cross(s).mul(-1)
			if nrm.dot(F.p(x, y, z).sub(F.p(x, 0.1, 0))) < 0 {
				nrm = nrm.mul(-1)
			}
			fastBox(&bEnv, xf{F.p(x, y, z), s, nrm, tan}, 0, 0.004, 0, 0.055, 0.01, 0.022, cFrame.scale(1.4))
		}
	}
	hull := xf{F.p(0, 0.33, -0.02), s, up, f}
	bbox(&bEnv, hull, 0, 0, 0, 0.21, 0.09, 0.33, 0.03, cFrame)
	bevelBox(&bEnv, hull.p(0, 0.07, 0.22), s, up.mul(0.8).add(f.mul(0.6)).norm(), f.mul(0.8).sub(up.mul(0.6)).norm(), 0.2, 0.02, 0.13, 0.01, cArmor)
	bbox(&bEnv, hull, 0, 0.1, -0.08, 0.19, 0.02, 0.2, 0.01, cArmor)
	for _, sg := range []float64{-1, 1} {
		for k := 0; k < 6; k++ {
			c := cHazard
			if k%2 == 1 {
				c = rgba{0.05, 0.05, 0.05, 1}
			}
			z := -0.25 + float64(k)*0.09
			bbox(&bEnv, hull, sg*0.215, 0.0, z, 0.006, 0.05, 0.045, 0.003, c)
		}
	}
	for k := 0; k < 3; k++ {
		a, b := hull.p(-0.12, 0.125, -0.2+float64(k)*0.045), hull.p(0.12, 0.125, -0.2+float64(k)*0.045)
		ribbon(a, b, 0.02, cSpark.alpha(0.4))
		bGlowLine.line(a, b, cSpark.alpha(0.8))
	}
	sprite(hull.p(0, 0.2, -0.16), 0.18, cSpark.alpha(float32(0.05+0.04*noise1(r.seed, t*5))))

	neck := hull.p(0, 0.12, 0.1)
	cyl(&bEnv, neck, neck.add(up.mul(0.1)), 0.06, 0.05, 16, cMetalDk, true)
	hc := neck.add(up.mul(0.16))
	H := xf{hc, hs, up, hf}
	bevelBox(&bEnv, hc, hs, up, hf, 0.11, 0.065, 0.1, 0.03, cArmor)
	bbox(&bEnv, H, 0, 0, 0.1, 0.075, 0.045, 0.03, 0.015, cFrame)
	for _, sg := range []float64{-1, 1} {
		bevelBox(&bEnv, H.p(sg*0.13, 0.01, -0.01), H.d(1, 0, sg*0.3), up, H.d(-sg*0.3, 0, 1), 0.02, 0.06, 0.08, 0.008, cFrame)
	}
	eb := float32(0.75 + 0.25*math.Sin(t*6+r.seed))
	if r.speaking {
		eb = float32(0.6 + 0.4*noise1(r.seed, t*30))
	}
	if r.state == rbLook {
		eb = 1
	}
	var eye vec3
	for k, o := range [][2]float64{{-0.035, 0.012}, {0.035, 0.012}, {0, -0.022}} {
		ep := H.p(o[0], o[1], 0.13)
		cyl(&bEnv, ep.sub(hf.mul(0.01)), ep.add(hf.mul(0.02)), 0.017, 0.015, 12, cMetalDk, true)
		sprite(ep.add(hf.mul(0.022)), 0.03, cRobotEye.alpha(eb))
		sprite(ep.add(hf.mul(0.022)), 0.008, rgba{1, 0.8, 0.7, eb})
		if k == 2 {
			eye = ep.add(hf.mul(0.025))
		}
	}
	sprite(H.p(0, 0, 0.16), 0.12, cRobotEye.alpha(0.12*eb))
	addPoint(H.p(0, 0, 0.2), cRobotEye, 0.7*float64(eb), 2.2)
	circuitPanel(hull.p(0.214, 0.0, 0.18), f, up, s, 0.08, 0.05, float32(r.wear))
	circuitPanel(hull.p(-0.214, 0.0, 0.18), f.mul(-1), up, s.mul(-1), 0.08, 0.05, float32(r.wear))
	for _, sg := range []float64{-1, 1} {
		rivetRow(hull.p(sg*0.212, 0.075, -0.3), hull.p(sg*0.212, 0.075, 0.3), s.mul(sg), 10, 0.007)
	}
	if r.wear > 0.5 && rand.Float64() < frameDT*2*r.moving {
		spawnSmoke(hull.p(-0.12, 0.25, -0.3), 1)
	}
	e.eyeLaser(r, eye)

	// hydraulic clamp arms on the bow: folded when idle, reach out and grip a base's rim to carry it
	var carryC vec3
	grab := 0.0
	if r.carryBase != nil {
		carryC = liftPos(r, F.p(0, 0.4, 0.8))
		grab = 1
		if r.liftK < 0.38 {
			grab = smooth(r.liftK / 0.38)
		}
		drawCarriedBase(r.carryBase, carryC, carryAxis(r))
	}
	for _, sg := range []float64{-1, 1} {
		sh := hull.p(sg*0.19, 0.02, 0.3)
		stow := hull.p(sg*0.17, 0.0, 0.5).add(up.mul(0.02 * math.Sin(t*2+sg)))
		claw := stow
		jaw := 0.2
		if r.carryBase != nil {
			rim := carryC.add(s.mul(sg * 0.335)).add(up.mul(0.015))
			claw = stow.add(rim.sub(stow).mul(grab))
			jaw = 1 - grab
			if r.liftK >= 0.38 {
				jaw = 0
			}
		}
		claw = reach(sh, claw, 0.3, 0.34)
		el := ik(sh, claw, 0.3, 0.34, up.add(s.mul(sg*0.5)))
		axleJoint(sh, s, 0.045, 0.07, cFrame)
		armorSeg(sh, el, s, 0.03, 0.034, cArmor)
		axleJoint(el, s, 0.034, 0.06, cFrame)
		armorSeg(el, claw, s, 0.024, 0.026, cMetalDk)
		piston(sh.add(up.mul(0.05)), el.add(up.mul(0.045)), 0.009)
		e.jointSparks(r, el, up)
		// C-clamp jaws pinching the rim from above and below, facing inward
		in := s.mul(-sg)
		cd := claw.sub(el).norm()
		bevelBox(&bEnv, claw, cd, up, cd.cross(up), 0.03, 0.03, 0.03, 0.008, cFrame)
		for _, q := range []float64{-1, 1} {
			gap := 0.022 + 0.03*jaw
			j0 := claw.add(up.mul(q * gap))
			j1 := j0.add(in.mul(0.06))
			cyl(&bEnv, j0, j1, 0.01, 0.008, 8, cChrome, true)
			bevelBox(&bEnv, j1, in, up, in.cross(up), 0.012, 0.008, 0.016, 0.003, cMetalDk)
		}
	}

	tb := hull.p(0, 0.13, -0.22)
	cyl(&bEnv, tb.sub(up.mul(0.02)), tb.add(up.mul(0.04)), 0.07, 0.065, 16, cMetalDk, true)
	t1 := tb.add(up.mul(0.42)).add(f.mul(-0.1))
	tip := hull.p(0, 0.3+0.08*math.Sin(t*1.4+r.seed), 0.45).add(s.mul(0.1 * math.Sin(t*0.9+r.seed)))
	rest := tip
	cutting := r.salv != nil && (r.salvaging || r.toolK > 0)
	if cutting {
		tow := fl(r.pos, 0).sub(fl(pt2{r.cutPt.x, r.cutPt.z}, 0)).norm()
		tip = r.cutPt.add(up.mul(0.3)).add(tow.mul(0.12))
	} else if working {
		g := v3(gp[0], 0, gp[1])
		tow := base.sub(g)
		tow.y = 0
		tip = lerpV(tip, g.add(tow.norm().mul(0.13)).add(up.mul(math.Min(wy, 1.15))), armBlend)
	}
	// CO2: the bottle rides on the hull; the arm picks the horn off it and aims it
	fireK := 0.0
	var bottleNeck, hornAim vec3
	hornK := 0.0
	if r.fireCrew {
		if r.state == rbFire {
			fireK = r.exK
		}
		bb := F.p(-0.2, 0.45, -0.28)
		drawExtinguisher(bb, up)
		bottleNeck = bb.add(up.mul(0.36))
		grab := bottleNeck.add(s.mul(0.08)).add(up.mul(0.02))
		if fireK > 0 {
			tip = lerpV(tip, grab, smooth(math.Min(1, fireK/0.45)))
		}
		if fireK > 0.45 {
			hornK = smooth((fireK - 0.45) / 0.55)
			hornAim = co2Aim(r, t1)
			tip = lerpV(grab, t1.add(hornAim.mul(0.78)), hornK)
		}
		if fireK <= 0.42 {
			hb := bb.add(up.mul(0.24)).add(s.mul(0.08))
			co2Hose(bottleNeck.add(up.mul(0.02)), hb, up.mul(-1))
			drawCO2Horn(hb, up.mul(-1))
		}
	}
	armorSeg(tb, t1, s, 0.035, 0.045, cArmor)
	piston(tb.add(f.mul(0.06)), t1.add(f.mul(0.04)).add(up.mul(-0.1)), 0.01)
	axleJoint(t1, s, 0.045, 0.1, cFrame)
	tip, _, _ = r.smoothArm(0, F, rest, tip, 0.5, 0.55)
	tip = reach(t1, tip, 0.5, 0.55)
	tk := ik(t1, tip, 0.5, 0.55, up.add(f.mul(-0.3)))
	cyl(&bEnv, t1, tk, 0.03, 0.026, 12, cFrame, false)
	armorSeg(t1.add(tk.sub(t1).mul(0.15)), t1.add(tk.sub(t1).mul(0.85)), s, 0.035, 0.035, cArmor)
	axleJoint(tk, s, 0.04, 0.09, cFrame)
	armorSeg(tk, tip, s, 0.028, 0.03, cArmor)
	piston(t1.add(up.mul(0.05)), tk.add(up.mul(0.04)), 0.008)
	hose(&bEnv, tb.add(s.mul(0.06)), tb.add(up.mul(0.3)).add(s.mul(0.1)), tk.add(up.mul(0.1)).add(s.mul(0.05)), tk.add(s.mul(0.04)), 0.01, cHose, 7)
	td := tip.sub(tk).norm()
	torch := 0.05
	if r.fireCrew {
		torch = 0.05 * armBlend // welding head slides out for the repair
		if !working {
			torch = 0
		}
	}
	nz := tip.add(td.mul(0.05 + torch))
	cyl(&bEnv, tip, tip.add(td.mul(0.05)), 0.024, 0.022, 12, cMetalDk, true)
	if torch > 0.004 {
		cyl(&bEnv, tip.add(td.mul(0.05)), nz, 0.016, 0.006+0.01*(1-torch/0.05), 12, cChrome, true)
	}
	r.tail = nz
	if fireK > 0.42 {
		hd := td
		if hornK > 0 {
			hd = td.mul(1 - hornK).add(hornAim.mul(hornK)).norm()
		}
		hb := tip.add(td.mul(0.03)).add(s.mul(0.035))
		co2Hose(bottleNeck.add(up.mul(0.02)), hb, hd)
		htip := drawCO2Horn(hb, hd)
		r.hornTip, r.hornDir, r.hornT = htip, hd, fxTime()
		if r.sprayOn {
			e.sprayFX(r, htip, hd)
		}
	}
	if cutting && r.salvaging && r.toolK >= 1 {
		e.cutFX(r, nz)
	} else if cutting && r.toolK > 0 {
		// the cutting head spinning up / winding down
		sprite(nz, 0.02+0.03*r.toolK, rgba{1, 0.35, 0.15, float32(0.5 * r.toolK)})
	}
	if r.carry {
		cc := hull.p(0, 0.2, -0.05)
		useMat(mCast)
		cyl(&bEnv, cc, cc.add(up.mul(0.07)), 0.11, 0.1, 20, cIron, true)
		cyl(&bEnv, cc.add(up.mul(0.07)), cc.add(up.mul(0.1)), 0.06, 0.05, 14, cIron.scale(1.3), true)
		for k := 0; k < 3; k++ {
			a := float64(k) * 2 * math.Pi / 3
			d := f.mul(math.Cos(a)).add(s.mul(math.Sin(a)))
			cyl(&bEnv, cc.add(d.mul(0.1)).add(up.mul(0.04)), cc.add(d.mul(0.2)).add(up.mul(0.1)), 0.012, 0.01, 8, cChrome, true)
		}
		if rand.Float64() < frameDT*0.4 {
			spawnSparks(cc.add(up.mul(0.1)), up, 3, 0.2)
		}
	}
	if cutting || r.sprayOn {
	} else if working && armBlend >= 0.999 {
		e.weldFX(r, nz, gp)
	} else if torch > 0.004 {
		sprite(nz, 0.015, cWeld.alpha(0.7))
	}
}

// cleanRoles: well-kept machined parts (tool, menu console, base stations)
func cleanRoles() func() {
	save := roles
	roles = []role{
		{rgba{0.10, 0.11, 0.13, 1}, rgba{0.10, 0.11, 0.13, 1}, material{0, 0.3, 0.32, 0.03}},
		{cFrame, rgba{0.12, 0.12, 0.13, 1}, material{7, 0.8, 0.42, 0.05}},
		{cArmor, rgba{0.30, 0.31, 0.33, 1}, material{0, 0.3, 0.35, 0.05}},
		{cChrome, rgba{0.86, 0.87, 0.9, 1}, material{1, 1, 0.06, 0}},
		{cMetalDk, rgba{0.30, 0.31, 0.33, 1}, material{1, 1, 0.3, 0.05}},
		{cMetal, rgba{0.62, 0.64, 0.66, 1}, material{1, 1, 0.22, 0.03}},
		{cHose, cHose, material{2, 0, 0.7, 0.1}},
		{rgba{0.02, 0.022, 0.026, 1}, rgba{0.02, 0.022, 0.026, 1}, material{5, 0, 0.05, 0}},
	}
	return func() { roles = save }
}

// toolRoles: the hand tool is clean polished metal throughout - gunmetal body, chrome trim
func toolRoles() func() {
	save := roles
	pol := func(r, g, b, rough float32) material { return material{8, 1, rough, 0} }
	roles = []role{
		{rgba{0.10, 0.11, 0.13, 1}, rgba{0.34, 0.35, 0.38, 1}, pol(0, 0, 0, 0.16)},
		{cFrame, rgba{0.30, 0.31, 0.34, 1}, pol(0, 0, 0, 0.2)},
		{cArmor, rgba{0.58, 0.6, 0.64, 1}, pol(0, 0, 0, 0.1)},
		{cChrome, rgba{0.9, 0.91, 0.94, 1}, pol(0, 0, 0, 0.04)},
		{cMetalDk, rgba{0.26, 0.27, 0.3, 1}, pol(0, 0, 0, 0.14)},
		{cMetal, rgba{0.75, 0.77, 0.8, 1}, pol(0, 0, 0, 0.07)},
		{cHose, cHose, material{2, 0, 0.6, 0}},
		{rgba{0.02, 0.022, 0.026, 1}, rgba{0.02, 0.022, 0.026, 1}, material{5, 0, 0.05, 0}},
	}
	return func() { roles = save }
}

// carryAxis: a carried base keeps the orientation it had on the floor, turning with the robot
func carryAxis(r *robot) vec3 {
	ob := r.carryBase
	a := math.Atan2(ob.ax.z, ob.ax.x) + angDiff(ob.liftYaw, r.yaw)
	return v3(math.Cos(a), 0, math.Sin(a))
}

// liftPos eases a cut-free base from the floor up to where the robot holds it
func liftPos(r *robot, hold vec3) vec3 {
	if r.salv == nil || r.liftK >= 1 {
		return hold
	}
	// first ~40% of the lift is the grab: the base stays put while the grippers close on it
	k := smooth(math.Max(0, (r.liftK-0.38)/0.62))
	a := fl(r.salv.pos, 0)
	p := a.add(hold.sub(a).mul(k))
	p.y += math.Sin(k*math.Pi) * 0.05
	return p
}
