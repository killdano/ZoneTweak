package main

import (
	"boundaryvr/sim"
	"math"
	"math/rand"
)

// ---- generators and the green ring ----

func (g *game) drawGenerators() {
	r := g.r
	n := len(r.Gens)
	if n == 0 {
		return
	}
	dt := frameDT
	for i := range r.Gens {
		gen := &r.Gens[i]
		want := 0.0
		if gen.State == sim.GenUp {
			want = 1
		}
		if g.gext[i] < want {
			g.gext[i] = math.Min(want, g.gext[i]+dt*2.2)
		} else {
			g.gext[i] = math.Max(want, g.gext[i]-dt*3)
		}
		if g.gpow[i] < want {
			g.gpow[i] = math.Min(want, g.gpow[i]+dt*1.5)
		} else {
			g.gpow[i] = math.Max(want, g.gpow[i]-dt*4)
		}
		bladeExt[i] = g.gext[i]
	}
	bladeExtN = n
	sidePowerHook = func(gi, side int) float64 {
		if mazeDrawing {
			if side == 0 || side == 1 {
				return mazeSidePow[side]
			}
			return 0
		}
		if gi < 0 || gi >= n {
			return 0
		}
		l := g.linkP[gi]
		if side == 1 {
			l = g.linkN[gi]
		}
		if l < 0 {
			return 0
		}
		return g.gpow[gi]
	}
	for i := range r.Gens {
		gen := &r.Gens[i]
		p := ed.pts[i]
		prev, next := ed.pts[(i-1+n)%n], ed.pts[(i+1)%n]
		col, fault := cEnergy, 0.0
		switch gen.State {
		case sim.GenBlown:
			col, fault = cDead, 1
		case sim.GenOff:
			col = cOffAmber.scale(0.35)
		case sim.GenLeeched:
			col, fault = cDanger, 0.45
		default:
			if k := gen.Charge / r.T.ChargeMax; k < 0.3 {
				fault = (0.3 - k) / 0.3 * 0.5
				col = cEnergy.mix(cAmber, float32((0.3-k)/0.3))
			}
		}
		ed.drawGen(i, p, genH, prev, next, false, col, fault)
		lp, ln := prev, next
		if g.linkP[i] >= 0 {
			lp = ed.pts[g.linkP[i]]
		}
		if g.linkN[i] >= 0 {
			ln = ed.pts[g.linkN[i]]
		}
		ed.drawBladeExt(i, p, lp, ln, col)
		if gen.State == sim.GenBlown && rand.Float64() < frameDT*1.5 {
			spawnSmoke(fl(p, genH-0.2), 2)
		}
	}
}

// energySheet: a projected wall of light from a to b, between lo(u) and top(u) (u = 0..1
// along it), with drifting filaments, a scan band and bright edges.
func energySheet(a, b pt2, lo, top func(u float64) float64, breaks []float64, col rgba, pulse float32, seed float64) {
	L := dist2(a, b)
	if L < 1e-3 || pulse <= 0.001 {
		return
	}
	t := fxTime()
	at := func(u, y float64) vec3 { return v3(a[0]+(b[0]-a[0])*u, y, a[1]+(b[1]-a[1])*u) }
	addRect(a, b, 0, top(0.5), col, 0.2*float64(pulse), 5)
	fr := []float64{0, 0.1, 0.45, 0.9, 1}
	as := []float32{0.07, 0.018, 0.006, 0.016, 0.05}
	us := edgeSamples(int(math.Max(4, L/0.12)), breaks)
	for cI := 0; cI+1 < len(us); cI++ {
		u0, u1 := us[cI], us[cI+1]
		l0, l1 := lo(u0), lo(u1)
		t0, t1 := top(u0), top(u1)
		for k := 0; k < 4; k++ {
			c0, c1 := col.alpha(as[k]*pulse), col.alpha(as[k+1]*pulse)
			y00, y10 := l0+(t0-l0)*fr[k], l1+(t1-l1)*fr[k]
			y01, y11 := l0+(t0-l0)*fr[k+1], l1+(t1-l1)*fr[k+1]
			bGlowTri.quadC(at(u0, y00), at(u1, y10), at(u1, y11), at(u0, y01), c0, c0, c1, c1)
		}
	}
	nx, nz := -(b[1]-a[1])/L, (b[0]-a[0])/L
	for _, sg := range []float64{-1, 1} {
		o := v3(nx*0.6*sg, 0, nz*0.6*sg)
		c0 := col.alpha(0.08 * pulse)
		bGlowTri.quadC(at(0, 0.002), at(1, 0.002), at(1, 0.002).add(o), at(0, 0.002).add(o), c0, c0, col.alpha(0), col.alpha(0))
	}
	n := int(L / 0.14)
	if potatoMode {
		n /= 2
	}
	n = max(n, 1)
	for k := 0; k <= n; k++ {
		u := (float64(k) + 0.4*math.Sin(t*0.3+float64(k)*1.7)) / float64(n)
		if u < 0 || u > 1 {
			continue
		}
		fi := seed*997 + float64(k)
		edge := math.Max(0, 1-math.Min(u, 1-u)*L/0.25)
		y := lo(u)
		tp := top(u)
		span := tp - y
		for sg := 0; sg < 3; sg++ {
			h := span * (0.15 + 0.45*noise1(fi+float64(sg)*7.1, t*0.45))
			y1 := math.Min(tp, y+h)
			if sg == 2 {
				y1 = tp
			}
			in := noise1(fi*1.3+float64(sg), t*2.2)
			alpha := float32(0.008+0.13*in*in*in*in+0.2*edge) * pulse
			bGlowLine.line(at(u, y), at(u, y1), col.alpha(alpha))
			if k%7 == 0 && in > 0.7 {
				ribbon(at(u, y), at(u, y1), 0.025, col.alpha(alpha*0.3))
			}
			y = y1
			if y >= tp {
				break
			}
		}
	}
	sy := lo(0.5) + math.Mod(t*0.11+seed*0.37, 1)*(top(0.5)-lo(0.5))
	ribbon(at(0, sy), at(1, sy), 0.2, col.alpha(0.035*pulse))
	edgeLine := func(p, q vec3) {
		ribbon(p, q, 0.06, col.alpha(0.4*pulse))
		ribbon(p, q, 0.015, cEnergyHot.mix(col, 0.5).alpha(0.6*pulse))
		bGlowLine.line(p, q, col.alpha(0.9*pulse))
	}
	edgeLine(at(0, top(0)), at(1, top(1)))
	eu := edgeSamples(4, breaks)
	for k := 0; k+1 < len(eu); k++ {
		u0, u1 := eu[k], eu[k+1]
		edgeLine(at(u0, math.Max(0.004, lo(u0))), at(u1, math.Max(0.004, lo(u1))))
	}
}

func (g *game) drawRing() {
	r := g.r
	t := fxTime()
	for _, s := range r.Spans() {
		ia, ib := s[0], s[1]
		a, b := ed.pts[ia], ed.pts[ib]
		L := dist2(a, b)
		prof := wallProfile{L: L, top: genH - bladeTopIn, eA: ed.extOf(ia), eB: ed.extOf(ib)}
		k := math.Min(r.Gens[ia].Charge, r.Gens[ib].Charge) / r.T.ChargeMax
		col := cEnergy
		pulse := float32(0.85+0.15*math.Sin(t*2.4+float64(ia))) * float32(math.Min(g.gpow[ia], g.gpow[ib]))
		pulse *= float32(0.55 + 0.45*k)
		if k < 0.3 && noise1(float64(ia)*3.3, t*11) < (0.3-k)*1.6 {
			pulse *= 0.3
		}
		hot := r.SpanHot(ia, ib)
		if hot > 0 {
			col = cWhiteHot
			pulse *= 2.4
			for q := 0; q < 3; q++ {
				u := rand.Float64()
				arc(fl(pt2{a[0] + (b[0]-a[0])*u, a[1] + (b[1]-a[1])*u}, 0.05), fl(pt2{a[0] + (b[0]-a[0])*rand.Float64(), a[1] + (b[1]-a[1])*rand.Float64()}, prof.top*rand.Float64()), cEnergyHot)
			}
		}
		energySheet(a, b, func(u float64) float64 { return prof.lo(u * L) }, func(u float64) float64 { return prof.topAt(u * L) }, prof.breaks(), col, pulse, float64(ia*31+ib))
		ed.surges(1000+ia*64+ib, a, b, L, genH, col, float64(pulse), 0)
	}
	// wardens leaning on the ring: the sheet flares and crackles where they push
	for _, b := range r.Robots {
		if !b.Alive() || (b.State != sim.RPress && !b.Pinned) || b.SpanA < 0 {
			continue
		}
		A, B := r.Gens[b.SpanA].Pos, r.Gens[b.SpanB].Pos
		_, u := sim.SegDist(b.Pos, A, B)
		c := fv(sim.Lerp(A, B, u), 1.05+0.25*math.Sin(t*3+float64(b.ID)))
		fk := float32(0.6 + 0.4*noise1(float64(b.ID), t*9))
		sprite(c, 0.35, cEnergy.alpha(0.35*fk))
		sprite(c, 0.12, cEnergyHot.alpha(0.8*fk))
		addPoint(c, cEnergyHot, 3*float64(fk), 2.5)
		if rand.Float64() < 0.5 {
			d := v3(rand.NormFloat64(), rand.NormFloat64()*0.7, rand.NormFloat64()).norm().mul(0.25)
			arc(c, c.add(d), cEnergy)
		}
		if rand.Float64() < frameDT*3 {
			spawnSparks(c, fv(b.Pos.Sub(sim.Lerp(A, B, u)).Norm(), 0.3).norm(), 6, 0.5)
		}
	}
}

// ---- the magenta maze ----

func (g *game) drawMaze() {
	r := g.r
	t := fxTime()
	H := r.T.WallHeight
	for _, w := range r.Maze {
		a, b := pv(w.A), pv(w.B)
		// a wall's sheet only lights once the base stations it runs between are up
		k := 1.0
		for _, q := range g.wallPostPts(a, b) {
			if mp := g.mposts[mazePostKey(q)]; mp != nil {
				k = math.Min(k, mp.ready())
			} else {
				k = 0
			}
		}
		// being taken down: the sheet drains down and out first
		f := 1 - clamp01(g.retract[w.ID]/0.4)
		k *= f
		if k <= 0.01 {
			continue
		}
		top := 0.05 + (H-0.27)*f
		col := cMagenta
		pulse := float32((0.75 + 0.1*math.Sin(t*2.1+float64(w.ID))) * k)
		if w.Super > 0 {
			col = cMagentaHot
			pulse *= 2.3
			if rand.Float64() < 0.7 {
				u, v := rand.Float64(), rand.Float64()
				arc(fl(lerp2(a, b, u), 0.05), fl(lerp2(a, b, v), H*rand.Float64()), cMagentaHot)
			}
		}
		energySheet(a, b, func(float64) float64 { return 0.05 }, func(float64) float64 { return top }, nil, col, pulse, float64(w.ID))
	}
}

func lerp2(a, b pt2, u float64) pt2 { return pt2{a[0] + (b[0]-a[0])*u, a[1] + (b[1]-a[1])*u} }

// faceText: text printed on a surface, readable from the side its normal n points to
// (works whatever the handedness of the frame it came from).
func faceText(b *batch, s string, c, n, up vec3, px float64, col rgba) {
	drawTextCentered(b, s, c, up.cross(n).norm(), up, px, col)
}

// topText: text on a level surface, turned to read the right way up from where you are.
func topText(b *batch, s string, c, n vec3, px float64, col rgba) {
	up := projPerp(c.sub(ed.hmd.pos()), n)
	if up.length() < 1e-3 {
		up = projPerp(v3(0, 0, -1), n)
	}
	faceText(b, s, c, n, up.norm(), px, col)
}

// flatLine: a thin painted stroke from a to c, lying flat in the surface with normal n
// (triangles, so it can share a triangle batch with text).
func flatLine(b *batch, a, c, n vec3, w float64, col rgba) {
	side := c.sub(a).cross(n).norm().mul(w / 2)
	b.quad(a.sub(side), c.sub(side), c.add(side), a.add(side), col)
}

// arcRing: a progress arc round c in the plane of right and up, k of the way round.
func arcRing(c, right, up vec3, R, k float64, col rgba) {
	n := int(48 * k)
	for i := 0; i < n; i++ {
		a0, a1 := float64(i)/48*2*math.Pi, float64(i+1)/48*2*math.Pi
		p0 := c.add(up.mul(R * math.Cos(a0))).add(right.mul(R * math.Sin(a0)))
		p1 := c.add(up.mul(R * math.Cos(a1))).add(right.mul(R * math.Sin(a1)))
		ribbon(p0, p1, 0.02, col.alpha(0.9))
	}
}

// ---- door names ----

// drawDoorSigns: each door's name stencilled big over it in glow paint, so "SOUTH DOOR" on
// the radio (and "the north wall" when your contact helps you find the driver) is
// something you can see.
func (g *game) drawDoorSigns() {
	up := v3(0, 1, 0)
	for _, d := range g.r.Doors {
		out := fv(d.Out, 0).norm()
		right := out.cross(up).norm()
		// a sign board hung out from the wall on brackets, clear of the pipes and lamps
		wall := fv(d.Wall, 3.3)
		c := wall.sub(out.mul(0.5))
		F := xf{c, right, up, out.mul(-1)}
		px := 0.06
		hw, hh := float64(textWidth(d.Name))*px/2+0.14, 3.5*px+0.1
		useMat(mCast)
		bbox(&bEnv, F, 0, 0, -0.03, hw, hh, 0.03, 0.01, rgba{0.05, 0.05, 0.055, 1})
		useMat(mSteel)
		for _, sx := range []float64{-1, 1} {
			a := F.p(sx*(hw-0.1), hh*0.4, -0.06)
			cyl(&bEnv, a, a.add(out.mul(0.48)), 0.018, 0.018, 8, cSteelEnv, true)
			b := F.p(sx*(hw-0.1), -hh*0.4, -0.06)
			cyl(&bEnv, b, b.add(out.mul(0.48)), 0.018, 0.018, 8, cSteelEnv, true)
		}
		col := rgba{1, 0.7, 0.15, 1}
		curEmis = 3
		drawTextCentered(&bOpaque, d.Name, F.p(0, 0, 0.004), right, up, px, col)
		curEmis = 1
		addPoint(F.p(0, 0, 0.4), col, 0.6, 2.5)
	}
}

// ---- your room on the floor ----

func (g *game) drawRoomFloor() {
	r := g.r
	t := fxTime()
	n := len(g.room)
	for i := range g.room {
		a, b := g.room[i], g.room[(i+1)%n]
		bFloorL.line(fl(a, 0.004), fl(b, 0.004), cAmber.alpha(0.35))
	}
	// floor your ring no longer covers: red hatching
	if len(r.RingPoly) < 3 {
		return
	}
	h := r.Hall
	rb := h.RoomBox
	k := float32(0.25 + 0.1*math.Sin(t*4))
	for x := rb[0] + 0.05; x < rb[1]; x += 0.18 {
		var prev pt2
		inside := false
		for z := rb[2]; z <= rb[3]; z += 0.05 {
			q := h.F.From(sim.V(x, z))
			lost := sim.InPoly(r.Room, q) && !sim.InPoly(r.RingPoly, q)
			p := pv(q)
			if lost && !inside {
				prev, inside = p, true
			}
			if !lost && inside {
				bFloorL.line(fl(prev, 0.005), fl(p, 0.005), cDanger.alpha(k))
				inside = false
			}
		}
	}
}

// ---- wardens and drones ----

func (g *game) drawRobots() {
	r := g.r
	t := fxTime()
	seen := map[int]bool{}
	cam := ed.hmd.pos()
	n := 0
	for _, b := range r.Robots {
		if b.Kind != sim.Drone && b.DeadT < 6 {
			n++
		}
	}
	robotCrowdK = math.Max(1, math.Sqrt(float64(n)/8))
	defer func() { robotCrowdK = 1 }()
	viewFwd = ed.hmd.axis(2).mul(-1)
	viewFwd.y = 0
	if viewFwd.length() < 1e-3 {
		viewFwd = v3(0, 0, -1)
	}
	viewFwd = viewFwd.norm()
	for _, b := range r.Robots {
		seen[b.ID] = true
		if b.Kind == sim.Drone {
			g.drawDrone(b)
			continue
		}
		v := g.bots[b.ID]
		if v == nil {
			v = &robot{gen: -1, seed: rand.Float64() * 100, svPiece: -1}
			v.randomize()
			v.kind = kindWarden
			if b.Kind == sim.Heavy {
				v.kind = kindTread
			}
			v.pos, v.yaw = pv(b.Pos), b.Yaw
			g.bots[b.ID] = v
		}
		np := pv(b.Pos)
		v.phase += dist2(np, v.pos) / 0.5 * math.Pi
		v.pos, v.yaw, v.moving = np, b.Yaw, b.Moving
		v.workSet, v.noWeld, v.state = false, false, rbWalk
		v.tilt, v.sink = 0, 0
		switch b.State {
		case sim.RPress:
			A, B := r.Gens[b.SpanA].Pos, r.Gens[b.SpanB].Pos
			_, u := sim.SegDist(b.Pos, A, B)
			v.state, v.workSet, v.workAt = rbWork, true, pv(sim.Lerp(A, B, u))
			v.workY = 1.0 + 0.15*math.Sin(t*0.7+v.seed)
		case sim.RHooked:
			v.moving = 1
			v.phase += frameDT * 7
			if b.Pinned && b.SpanA >= 0 {
				A, B := r.Gens[b.SpanA].Pos, r.Gens[b.SpanB].Pos
				_, u := sim.SegDist(b.Pos, A, B)
				v.state, v.workSet, v.workAt, v.workY = rbWork, true, pv(sim.Lerp(A, B, u)), 1.0
			}
		case sim.RCarry:
			f := sim.V(math.Cos(b.Yaw), math.Sin(b.Yaw))
			v.state, v.workSet, v.noWeld = rbWork, true, true
			v.workAt, v.workY = pv(b.Pos.Add(f.Mul(0.5))), 0.75
		case sim.RLurk:
			// closing in round the device: while its field holds they burn at the dome
			d := &r.Dev
			if d.State == sim.DevHome && d.Exposed && d.Field > 0 && sim.Dist(b.Pos, d.Pos) < 1.5 && len(b.Path) == 0 {
				to := d.Pos.Sub(b.Pos).Norm()
				v.state, v.workSet = rbWork, true
				v.workAt = pv(d.Pos.Sub(to.Mul(0.8)))
				v.workY = 0.9 + 0.2*math.Sin(t*0.9+v.seed)
			}
		case sim.RDead:
			v.moving = 0
			v.tilt = math.Min(1.45, b.DeadT*b.DeadT*2.5)
			v.sink = math.Max(0, (b.DeadT-4.5)*0.35)
			if b.DeadT < 3 && rand.Float64() < frameDT*4 {
				spawnSmoke(fl(np, 0.8), 2)
			}
			if b.DeadT < 1.2 && rand.Float64() < 0.3 {
				spawnSparks(fl(np, 0.9+rand.Float64()*0.6), v3(0, 1, 0), 4, 0.6)
			}
		}
		if b.Alive() {
			look := math.Atan2(cam.z-np[1], cam.x-np[0])
			v.headYaw += (math.Max(-1.1, math.Min(1.1, angDiff(v.yaw, look))) - v.headYaw) * math.Min(1, frameDT*3)
			ed.robotSteps(v)
		}
		// robots well behind you aren't built at all (even a very wide headset doesn't see
		// past about 100 degrees either side); close ones always are
		if to := v3(np[0]-cam.x, 0, np[1]-cam.z); to.length() > 1.6 && to.norm().dot(viewFwd) < -0.2 {
			continue
		}
		ed.drawRobot(v)
		if b.Alive() && b.Kind != sim.Drone {
			g.drawPower(b, np, t)
		}
		if b.Dazed > 0 && b.Alive() {
			// reeling from a shock: wobbling on the spot, sparks crawling over it
			v.tilt = 0.12 * math.Sin(t*7+v.seed)
			h := fl(np, 1.0+0.5*rand.Float64())
			if rand.Float64() < 0.5 {
				arc(h, h.add(v3(rand.Float64()-0.5, rand.Float64()-0.5, rand.Float64()-0.5).mul(0.6)), cBeam)
			}
			if rand.Float64() < frameDT*6 {
				spawnSparks(h, v3(0, 1, 0), 5, 0.7)
			}
			sprite(fl(np, 1.3), 0.6, cBeam.alpha(float32(0.1+0.08*math.Sin(t*19))))
		}
		if b.Burn && b.Alive() && b.State == sim.RWalk {
			g.drawBreacher(b, np, t)
		}
	}
	for id := range g.bots {
		if !seen[id] {
			delete(g.bots, id)
		}
	}
}

// viewFwd: where you're facing, flat (for skipping robots behind you)
var viewFwd = vec3{0, 0, -1}

func (g *game) drawDrone(b *sim.Robot) {
	r := g.r
	t := fxTime()
	ph := float64(b.ID) * 1.7
	h := b.Height() + 0.06*math.Sin(t*2.3+ph)
	pos, yaw := g.dronePos(b)
	attached := b.State == sim.RAttached
	g.drawLeechTether(b, pos)
	g.drawRecovering(b, pos)
	if b.State == sim.RRecover && sim.RecoverSecs-b.Wait < 1.2 {
		yaw += (sim.RecoverSecs - b.Wait) * 9 // tumbling as it's flung off
	}
	if !b.Alive() {
		k := math.Min(1, b.DeadT/0.6)
		pos.y = h*(1-k*k) + 0.06*k*k
		if b.DeadT > 5 {
			return
		}
	}
	f := v3(math.Cos(yaw), 0, math.Sin(yaw))
	s := v3(-f.z, 0, f.x)
	up := v3(0, 1, 0)
	if !b.Alive() {
		up = up.add(s.mul(0.6)).norm()
	} else if !attached {
		// lean into the direction of travel
		up = up.add(f.mul(0.18 * b.Moving)).norm()
	}
	F := xf{pos, s, up, f}
	defer withTex(F)()
	useMat(mPaint)
	bevelBox(&bEnv, pos, s, up, f, 0.09, 0.045, 0.12, 0.02, rgba{0.16, 0.16, 0.18, 1})
	useMat(mHazard)
	bevelBox(&bEnv, F.p(0, 0.05, -0.02), s, up, f, 0.06, 0.012, 0.07, 0.006, rgba{0.8, 0.6, 0.1, 1})
	useMat(mCast)
	for k := 0; k < 4; k++ {
		sx, sz := []float64{-1, 1, -1, 1}[k], []float64{-1, -1, 1, 1}[k]
		hub := F.p(sx*0.17, 0.03, sz*0.15)
		cyl(&bEnv, F.p(sx*0.06, 0.02, sz*0.06), hub, 0.012, 0.01, 6, cIron, false)
		cyl(&bEnv, hub.sub(up.mul(0.03)), hub.add(up.mul(0.02)), 0.022, 0.022, 10, cIron, true)
		if b.Alive() {
			// spinning rotor: a faint disc and a blade flicker
			for q := 0; q < 2; q++ {
				a := t*60 + float64(q)*math.Pi/2 + float64(k)
				d := s.mul(math.Cos(a)).add(f.mul(math.Sin(a)))
				ribbon(hub.add(up.mul(0.025)).sub(d.mul(0.09)), hub.add(up.mul(0.025)).add(d.mul(0.09)), 0.02, rgba{0.6, 0.6, 0.65, 0.25})
			}
		}
	}
	// the eye
	eye := F.p(0, 0, 0.125)
	curEmis = 4
	hemi(&bEnv, eye, f, 0.028, cRobotEye, 3, 10)
	curEmis = 1
	if b.Alive() {
		fk := float32(0.7 + 0.3*math.Sin(t*9+ph))
		sprite(eye.add(f.mul(0.02)), 0.08, cRobotEye.alpha(0.5*fk))
		addPoint(eye.add(f.mul(0.1)), cRobotEye, 0.8, 2)
	}
	// claws: open in flight, clamped round the generator when it's on one
	open := 0.6
	if attached {
		open = -0.1
	}
	useMat(mSteel)
	for k := 0; k < 3; k++ {
		a := float64(k)*2*math.Pi/3 + math.Pi/2
		d := s.mul(math.Cos(a)).add(up.mul(math.Sin(a))).mul(1)
		root := F.p(0, -0.02, 0.1).add(d.mul(0.03))
		tip := root.add(f.mul(0.09)).add(d.mul(0.04 * open))
		cyl(&bEnv, root, tip, 0.008, 0.006, 6, cSteelEnv, true)
	}
	if attached && b.Alive() && r.Gens[b.Gen].State == sim.GenLeeched {
		// draining: red pulses down a cable into the generator
		gp := fv(r.Gens[g.leechPost(b.Gen)].Pos, 1.1)
		k := 0.5 + 0.5*math.Sin(t*6+ph)
		ribbon(eye, gp, 0.012, cRobotEye.alpha(float32(0.4*k)))
		sprite(pos, 0.25, cDanger.alpha(float32(0.18*k)))
		addPoint(pos, cDanger, 1.5*k, 2)
		if rand.Float64() < frameDT*4 {
			spawnSparks(gp, f.mul(-1), 4, 0.4)
		}
	}
	if !b.Alive() && b.DeadT < 2 && rand.Float64() < frameDT*6 {
		spawnSmoke(pos, 1)
	}
	shadowBlob(pv(b.Pos), 0.2, float32(0.5*math.Max(0, 1-(h-0.2)/2)))
}

// ---- banners: the big messages floating in front of you ----

func (g *game) drawBanners() {
	cam := ed.hmd.pos()
	fwd := ed.hmd.axis(2).mul(-1)
	fwd.y = 0
	if fwd.length() < 1e-3 {
		fwd = v3(0, 0, -1)
	}
	fwd = fwd.norm()
	live := g.banners[:0]
	for i, bn := range g.banners {
		age := g.t - bn.t0
		if age > bn.dur {
			continue
		}
		live = append(live, bn)
		a := float32(math.Min(1, math.Min(age/0.25, (bn.dur-age)/0.5)))
		p := cam.add(fwd.mul(1.3)).add(v3(0, 0.12-float64(len(g.banners)-1-i)*0.16, 0))
		rr, uu := billboard(cam, p)
		drawTextCentered(&bOpaque, bn.text, p, rr, uu, textPx*2.6, bn.col.alpha(a))
		if bn.small != "" {
			drawTextCentered(&bOpaque, bn.small, p.add(v3(0, -0.065, 0)), rr, uu, textPx*1.3, cWhite.alpha(a))
		}
	}
	g.banners = live
}

// drawBreacher: a warden that's lost patience with your maze charges a magenta wall with a
// shield breacher held out in front of it - a spinning emitter throwing a cone of white-hot
// anti-energy ahead, brighter the closer it gets to the wall it's about to punch through.
func (g *game) drawBreacher(b *sim.Robot, np pt2, t float64) {
	r := g.r
	f := v3(math.Cos(b.Yaw), 0, math.Sin(b.Yaw))
	s := v3(-f.z, 0, f.x)
	c := fl(np, 1.15).add(f.mul(b.Radius() + 0.18))
	// how close the next wall on its way is
	near := 0.0
	for _, w := range r.Maze {
		if d, _ := sim.SegDist(b.Pos, w.A, w.B); d < 2.5 {
			near = math.Max(near, 1-d/2.5)
		}
	}
	useMat(mCast)
	cyl(&bEnv, c.sub(f.mul(0.05)), c.add(f.mul(0.04)), 0.11, 0.09, 16, cIron, true)
	useMat(mSteel)
	for _, sx := range []float64{-1, 1} {
		cyl(&bEnv, c.add(s.mul(sx*0.09)).sub(f.mul(0.02)), fl(np, 1.1).add(s.mul(sx*0.22)).add(f.mul(0.12)), 0.012, 0.012, 6, cSteelEnv, false)
	}
	hot := rgba{1, 0.85, 1, 1}
	curEmis = 3
	sphere(&bEnv, c.add(f.mul(0.05)), 0.05, hot, 4, 8)
	curEmis = 1
	for k := 0; k < 3; k++ {
		a := t*(6+float64(k)*2) + float64(k)*2.1
		u := s.mul(math.Cos(a)).add(v3(0, 1, 0).mul(math.Sin(a)))
		w := f.cross(u).norm()
		ringSoftAxes(c.add(f.mul(0.06)), u, w, 0.08+0.02*float64(k), 0.006, cMagentaHot.alpha(0.8), 18)
	}
	// the cone it throws ahead
	reach := 0.5 + 1.0*near
	tip := c.add(f.mul(reach))
	for k := 0; k < 4; k++ {
		off := s.mul((rngF() - 0.5) * 0.5 * reach).add(v3(0, (rngF()-0.5)*0.5*reach, 0))
		arc(c.add(f.mul(0.06)), tip.add(off), hot)
	}
	ribbon(c, tip, 0.12+0.2*near, rgba{1, 0.6, 1, float32(0.15 + 0.25*near)})
	sprite(c.add(f.mul(0.06)), 0.3+0.3*near, rgba{1, 0.7, 1, float32(0.5 + 0.3*math.Sin(t*20))})
	addPoint(c.add(f.mul(0.15)), cMagentaHot, 1+2*near, 2.5)
	sprite(fl(np, 1.2), 0.7, cMagenta.alpha(0.12+0.06*float32(math.Sin(t*14))))
}

// drawPower: a warden's power, once it's lost some: a short bar over its head (green to
// red), and sparks and smoke as it runs low.
func (g *game) drawPower(b *sim.Robot, np pt2, t float64) {
	max := g.r.MaxHP(b)
	k := clamp01(b.HP / max)
	if k > 0.98 {
		return
	}
	top := fl(np, robotTop(b)+0.25)
	cam := ed.hmd.pos()
	rr, _ := billboard(cam, top)
	w := 0.22
	if b.Kind == sim.Heavy {
		w = 0.32
	}
	col := rgba{float32(1 - k), float32(0.25 + 0.75*k), 0.15, 1}
	a, e := top.sub(rr.mul(w)), top.add(rr.mul(w))
	ribbon(a, e, 0.03, rgba{0.05, 0.05, 0.05, 0.7})
	m := a.add(e.sub(a).mul(k))
	ribbon(a, m, 0.022, col.alpha(0.95))
	bGlowLine.line(a, m, col)
	if k < 0.5 && rngF() < frameDT*(4-6*k) {
		spawnSparks(fl(np, 1.0+0.6*rngF()), v3(0, 1, 0), 4, 0.6)
	}
	if k < 0.3 && rngF() < frameDT*3 {
		spawnSmoke(fl(np, 1.3), 1)
	}
}

func robotTop(b *sim.Robot) float64 { return b.Height() }
