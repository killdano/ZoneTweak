//go:build windows

package main

import (
	"math"
	"math/rand"
)

const (
	sndEject = 3 + iota
	sndLand
	sndPulse
	sndTelescope
	sndIgnite
	sndTool
)

var (
	toolK      = 0.0
	toolSwitch = -10.0
	cToolDown  = rgba{0.30, 1.00, 0.70, 1}
	deploys    []*deploy
)

type deploy struct {
	pos   pt2
	from  vec3
	t0    float64
	hand  *hand
	fired int
}

func smooth(x float64) float64 {
	x = math.Max(0, math.Min(1, x))
	return x * x * (3 - 2*x)
}

func (e *editor) panAt(p vec3) (float64, float64) {
	d := p.sub(e.hmd.pos())
	l := d.length()
	if l < 1e-6 {
		return 1, 0
	}
	return 1 / (1 + 0.25*l*l), d.mul(1 / l).dot(e.hmd.axis(0))
}

func (e *editor) sfx(kind int, p vec3, amp float64) {
	a, pan := e.panAt(p)
	audioPlay(kind, amp*math.Max(a, 0.35), pan, p.sub(e.hmd.pos()).length())
}

func toolColor() rgba { return cToolDown.mix(cBeam, float32(smooth(toolK))) }

func (e *editor) switchTool(h *hand) {
	e.laser = !e.laser
	e.laserHand = h
	toolSwitch = fxTime()
	haptic(h.idx, 2500)
	e.sfx(sndTool, h.m.pos(), 0.9)
	if e.laser {
		e.say("Pointer mode")
	} else {
		e.say("Floor mode")
	}
}

func (e *editor) updateTool(dt float64) {
	tgt := 0.0
	if e.laser {
		tgt = 1
	}
	if toolK < tgt {
		toolK = math.Min(tgt, toolK+dt/0.45)
	} else if toolK > tgt {
		toolK = math.Max(tgt, toolK-dt/0.45)
	}
}

func (h *hand) toolFrame() xf {
	m := &h.m
	if h.aimValid {
		m = &h.aim
	}
	z := m.axis(2)
	return xf{m.pos().add(z.mul(0.05)), m.axis(0), m.axis(1), z}
}

func (e *editor) drawTool(h *hand) {
	seedTex(7000 + float64(h.idx)*50)
	F := h.toolFrame()
	defer withTex(F)()
	k := smooth(toolK)
	t := fxTime()
	since := t - toolSwitch
	col := toolColor()
	fwd := F.z.mul(-1)
	down := v3(0, -1, 0)
	d := down.mul(1 - k).add(fwd.mul(k))
	if d.length() < 1e-3 {
		d = fwd
	}
	d = d.norm()
	morph := 0.0
	if since < 0.45 {
		morph = math.Sin(since / 0.45 * math.Pi)
	}
	defer toolRoles()()
	saveM := curMat
	useMat(material{8, 1, 0.12, 0})
	defer useMat(saveM)
	segMul = 2
	defer func() { segMul = 1 }()
	matSpec, matGloss = 0.8, 40
	bbox(&bEnv, F, 0, -0.004, 0.055, 0.021, 0.02, 0.06, 0.008, rgba{0.10, 0.11, 0.13, 1})
	bbox(&bEnv, F, 0, 0.018, 0.05, 0.012, 0.008, 0.05, 0.004, cMetal)
	for _, s := range []float64{-1, 1} {
		bbox(&bEnv, F, s*0.024, -0.004, 0.06+0.01*morph, 0.005, 0.014, 0.045, 0.003, cMetalDk)
		a, b := F.p(s*0.0295, 0.0, 0.02), F.p(s*0.0295, 0.0, 0.1)
		ribbon(a, b, 0.008, col.alpha(0.55))
		bGlowLine.line(a, b, col.alpha(0.9))
	}
	cyl(&bEnv, F.p(0, -0.004, 0.115), F.p(0, -0.004, 0.13), 0.016, 0.012, 12, cMetalDk, true)
	sprite(F.p(0, -0.004, 0.132), 0.012, col.alpha(0.8))

	pivot := F.p(0, -0.004, -0.008)
	ball(pivot, 0.017, cMetalDk)
	tip := pivot.add(d.mul(0.085))
	cyl(&bEnv, pivot, pivot.add(d.mul(0.06)), 0.014, 0.011, 14, cMetal, false)
	cyl(&bEnv, pivot.add(d.mul(0.06)), tip, 0.011, 0.009, 14, cMetalDk, true)
	for i, s := range []float64{0.018, 0.034, 0.05} {
		c := pivot.add(d.mul(s))
		cyl(&bEnv, c, c.add(d.mul(0.006)), 0.0175, 0.0175, 14, cMetalDk, true)
		gl := float32(0.5 + 0.5*math.Sin(t*9-float64(i)*1.2))
		x, y := basis(d)
		for q := 0; q < 12; q++ {
			a0, a1 := float64(q)*math.Pi/6, float64(q+1)*math.Pi/6
			p0 := c.add(d.mul(0.003)).add(x.mul(0.0185 * math.Cos(a0))).add(y.mul(0.0185 * math.Sin(a0)))
			p1 := c.add(d.mul(0.003)).add(x.mul(0.0185 * math.Cos(a1))).add(y.mul(0.0185 * math.Sin(a1)))
			bGlowLine.line(p0, p1, col.alpha(0.5+0.5*gl))
		}
	}
	x, y := basis(d)
	spread := (1-k)*0.55 + morph*0.5
	spin := morph * 2.5
	for q := 0; q < 3; q++ {
		a := float64(q)*2*math.Pi/3 + spin
		rd := x.mul(math.Cos(a)).add(y.mul(math.Sin(a)))
		b0 := pivot.add(d.mul(0.03)).add(rd.mul(0.018))
		b1 := b0.add(d.mul(0.05 * math.Cos(spread))).add(rd.mul(0.05 * math.Sin(spread)))
		cyl(&bEnv, b0, b1, 0.0045, 0.003, 8, cMetal, true)
		ball(b0, 0.006, cMetalDk)
		sprite(b1, 0.008, col.alpha(0.7))
	}
	muzzle := tip.add(d.mul(0.004))
	h.muzzle, h.muzzleDir = muzzle, d
	sprite(muzzle, 0.018, col.alpha(0.6))
	sprite(muzzle, 0.006, rgba{1, 1, 1, 0.9})
	if since < 0.6 {
		f := float32(1 - since/0.6)
		sprite(pivot, 0.12*float64(f)+0.03, col.alpha(0.5*f))
		for q := 0; q < 6; q++ {
			a := float64(q)*math.Pi/3 + t*8
			rd := x.mul(math.Cos(a)).add(y.mul(math.Sin(a)))
			ribbonC(pivot, pivot.add(rd.mul(0.06*float64(f))), 0.006, col.alpha(0.8*f), col.alpha(0))
		}
	}
	matSpec, matGloss = 0.5, 28
}

func beam(o, end vec3, col rgba, w float64, flow float64) {
	t := fxTime()
	fl := float32(0.85 + 0.15*noise1(o.x*10, t*25))
	ribbon(o, end, w*3.5, col.alpha(0.08*fl))
	ribbon(o, end, w, col.alpha(0.45*fl))
	ribbon(o, end, w*0.3, rgba{0.95, 1, 1, 0.9})
	bGlowLine.line(o, end, rgba{0.9, 1, 1, 0.9})
	d := end.sub(o)
	L := d.length()
	if L < 1e-4 {
		return
	}
	n := int(math.Max(2, L*3))
	for k := 0; k < n; k++ {
		s := math.Mod(t*flow/L+float64(k)/float64(n), 1)
		p := o.add(d.mul(s))
		ribbonC(p, o.add(d.mul(math.Min(1, s+0.05/L))), w*1.6, col.alpha(0), rgba{1, 1, 1, 0.5 * fl})
	}
	sprite(end, w*4, col.alpha(0.5))
	sprite(end, w*1.2, rgba{1, 1, 1, 0.9})
}

func tether(o, p vec3, col rgba) {
	t := fxTime()
	mid := o.add(p).mul(0.5).add(v3(0, 0.08, 0))
	prev := o
	const n = 16
	for k := 1; k <= n; k++ {
		s := float64(k) / n
		u := 1 - s
		q := o.mul(u * u).add(mid.mul(2 * u * s)).add(p.mul(s * s))
		jit := v3(math.Sin(t*30+s*20), math.Cos(t*27+s*17), math.Sin(t*23+s*13)).mul(0.004 * math.Sin(s*math.Pi))
		q = q.add(jit)
		ribbon(prev, q, 0.02, col.alpha(0.35))
		bGlowLine.line(prev, q, col.alpha(0.9))
		prev = q
	}
	for k := 0; k < 3; k++ {
		s := math.Mod(t*1.5+float64(k)/3, 1)
		u := 1 - s
		sprite(o.mul(u*u).add(mid.mul(2*u*s)).add(p.mul(s*s)), 0.03, rgba{1, 1, 1, 0.6})
	}
	sprite(p, 0.09, col.alpha(0.5))
}

func (e *editor) drawToolBeams(h *hand) (wallBuzz bool, hit wallHit) {
	if h.muzzle == (vec3{}) {
		return
	}
	col := toolColor()
	o := h.muzzle
	if !e.laser {
		c := h.cursor()
		end := fl(c, 0.003)
		beam(o, end, col, 0.006, 1.2)
		addLine(o, end, col, 0.35, 2)
		ringSoft(end, 0.05+0.01*math.Sin(fxTime()*6), 0.02, col.alpha(0.8), 28)
		r := e.grabR()
		ringSoft(fl(c, 0.002), r, 0.015, col.alpha(0.18), 48)
	} else if h == e.laserHand && h.aimValid {
		dir := h.aim.axis(2).mul(-1)
		end := o.add(dir.mul(15)) // long enough to reach any wall in the room
		if h.menuRay {
			end = h.menuPt.add(e.menuN.mul(0.004))
		} else if h.rayHitOK {
			end = h.rayHit
			flare(end, o.sub(end).norm(), 0.05, col)
		} else {
			floorHit := false
			if c, ok := h.pointer(true); ok {
				end = fl(c, 0.004)
				floorHit = true
			}
			if wh := e.rayWalls(h.aim.pos(), dir); wh.ok && wh.p.sub(o).length() < end.sub(o).length() {
				// the beam stops on the laser wall: no floor marker beyond it
				end = wh.p
				wallBuzz, hit = true, wh
				e.touchWall(wh.wall, wh.p, 0.25)
				floorHit = false
			}
			if floorHit {
				ringSoft(end, 0.06, 0.02, col.alpha(0.7), 28)
			}
		}
		beam(o, end, col, 0.005, 2.5)
		addLine(o, end, col, 0.35, 2)
		addPoint(end, col, 0.6, 1.5)
	}
	g := h.grab
	if g < 0 {
		g = h.hoverV
	}
	if g >= 0 && g < len(e.pts) && !e.drawing {
		p := e.pts[g]
		y := math.Max(0.45, math.Min(e.height-0.3, o.y))
		tc := col
		if h.grab >= 0 {
			tc = cAmber
		}
		if e.laser && h.rayHitOK {
			y = h.rayHit.y
		} else {
			tether(o, v3(p[0], y, p[1]), tc)
		}
		ringSoft(v3(p[0], y, p[1]), 0.075, 0.02, tc.alpha(0.9), 24)
		ringSoft(v3(p[0], y, p[1]), 0.09+0.01*math.Sin(fxTime()*10), 0.01, tc.alpha(0.5), 24)
	}
	return
}

func (e *editor) grabR() float64 {
	if e.laser {
		return grabRadius
	}
	return 0.45
}

func (e *editor) startDeploy(h *hand, p pt2, track bool) {
	from := h.muzzle
	if from == (vec3{}) {
		from = h.m.pos()
	}
	d := &deploy{pos: p, from: from, t0: fxTime()}
	if track {
		d.hand = h
	}
	deploys = append(deploys, d)
	e.sfx(sndEject, from, 0.8)
	haptic(h.idx, 1500)
}

func deployAt(p pt2) *deploy {
	var best *deploy
	bd := 0.08
	for _, d := range deploys {
		if dd := dist2(d.pos, p); dd < bd {
			best, bd = d, dd
		}
	}
	return best
}

func deployVis(p pt2) float64 {
	d := deployAt(p)
	if d == nil {
		return 1
	}
	age := fxTime() - d.t0
	switch {
	case age < 1.7:
		return 0
	case age < 2.4:
		s := (age - 1.7) / 0.7
		if noise1(d.t0*3.7, age*14) > 0.25+0.6*s {
			return 0
		}
		return 0.5 + 0.8*rand.Float64()
	}
	return 1
}

func (e *editor) updateDeploys() {
	now := fxTime()
	out := deploys[:0]
	for _, d := range deploys {
		age := now - d.t0
		if age > 3 {
			continue
		}
		if d.hand != nil && d.hand.grab >= 0 && d.hand.grab < len(e.pts) && age < 2.6 {
			d.pos = e.pts[d.hand.grab]
		}
		p := fl(d.pos, 0.1)
		steps := []struct {
			at   float64
			kind int
			amp  float64
		}{{0.45, sndLand, 0.9}, {0.47, sndPulse, 0.8}, {0.9, sndTelescope, 0.8}, {1.7, sndIgnite, 0.8}}
		for i, s := range steps {
			if d.fired&(1<<i) == 0 && age >= s.at {
				d.fired |= 1 << i
				e.sfx(s.kind, p, s.amp)
				if s.kind == sndLand {
					spawnSparks(fl(d.pos, 0.02), v3(0, 1, 0), 14, 0.2)
				}
			}
		}
		out = append(out, d)
	}
	deploys = out
}

func (e *editor) drawDeploy(d *deploy, p pt2, H float64, prev, next pt2) bool {
	seedTex(8000 + d.t0*13)
	age := fxTime() - d.t0
	if age >= 1.7 {
		return false
	}
	up := v3(0, 1, 0)
	col := cEnergy
	defer cleanRoles()()
	matSpec, matGloss = 0.7, 34
	defer func() { matSpec, matGloss = 0.5, 28 }()
	if age < 0.45 {
		s := age / 0.45
		end := fl(p, 0.075)
		if d.hand != nil {
			end.y += 0.12 // hand-placed: comes to a hover, it doesn't touch down yet
		}
		dist := end.sub(d.from).length()
		pos := d.from.add(end.sub(d.from).mul(s)).add(up.mul(math.Sin(math.Pi*s) * (0.12 + 0.12*dist)))
		ax := v3(math.Cos(age*17), math.Sin(age*11)*0.6, math.Sin(age*17)).norm()
		if s > 0.8 {
			ax = ax.mul(1 - (s-0.8)*5).add(up.mul((s - 0.8) * 5)).norm()
		}
		a, b := pos.sub(ax.mul(0.065)), pos.add(ax.mul(0.065))
		cyl(&bEnv, a, b, 0.055, 0.055, 16, cMetal, false)
		hemi(&bEnv, b, ax, 0.055, cMetalDk, 3, 16)
		hemi(&bEnv, a, ax.mul(-1), 0.055, cMetalDk, 3, 16)
		x, y := basis(ax)
		for q := 0; q < 16; q++ {
			a0, a1 := float64(q)*math.Pi/8, float64(q+1)*math.Pi/8
			p0 := pos.add(x.mul(0.057 * math.Cos(a0))).add(y.mul(0.057 * math.Sin(a0)))
			p1 := pos.add(x.mul(0.057 * math.Cos(a1))).add(y.mul(0.057 * math.Sin(a1)))
			bGlowLine.line(p0, p1, col)
			ribbon(p0, p1, 0.012, col.alpha(0.5))
		}
		sprite(pos, 0.1, col.alpha(0.25))
		for k := 1; k <= 5; k++ {
			ss := math.Max(0, s-float64(k)*0.04)
			q := d.from.add(end.sub(d.from).mul(ss)).add(up.mul(math.Sin(math.Pi*ss) * (0.12 + 0.12*dist)))
			sprite(q, 0.04, col.alpha(float32(0.3-0.05*float64(k))))
		}
		shadowBlob(pt2{pos.x, pos.z}, 0.12, float32(0.5*s))
		return true
	}
	// landed: the real base unfolds out of the capsule, then the real shaft telescopes up
	c := fl(p, 0)
	shadowBlob(p, 0.3, 0.55)
	ax := baseAxisFor(p, prev, next)
	held := d.hand != nil && d.hand.grab >= 0 && d.hand.gNew
	lift := 0.0
	if d.hand != nil {
		// placed by hand: it hovers while held and only grows its floor mount once let go
		if held {
			lift = 0.12 + 0.015*math.Sin(fxTime()*5)
			ringSoft(fl(p, 0.01), 0.18+0.03*math.Sin(fxTime()*9), 0.03, cEnergy.alpha(0.6), 32)
			beam(fl(p, lift+0.03), fl(p, 0.005), cEnergy, 0.02, 2)
			addPoint(fl(p, lift), cEnergyHot, 2.5, 2.5)
		} else if rg := regrowAt(p); rg != nil {
			ra := math.Max(0, fxTime()-rg.t0)
			lift = math.Max(0, 0.12*(1-ra/0.35))
			genBaseGeom(p, p, smooth((ra-0.2)/1.2), false, 0, ax)
		}
	} else {
		grow := math.Min(1, (age-0.45)/0.6)
		genBaseGeom(p, p, math.Max(0.02, grow), false, 0, ax)
	}
	ext := 0.0
	if age > 0.9 {
		ext = smooth((age - 0.9) / 0.8)
	}
	if age < 0.9 {
		s := (age - 0.45) / 0.45
		ringSoft(c.add(up.mul(0.01)), 0.1+1.0*s, 0.06, col.alpha(float32(0.9*(1-s))), 48)
		addPoint(c.add(up.mul(0.2)), cEnergyHot, 8*(1-s), 3.5)
		ringSoft(c.add(up.mul(0.01)), 0.05+0.6*s, 0.03, cEnergyHot.alpha(float32(0.7*(1-s))), 36)
		floorGlow(p, 0.6*(1-s)+0.1, col.alpha(float32(0.5*(1-s))))
		sprite(c.add(up.mul(0.15)), 0.3*(1-s)+0.05, cEnergyHot.alpha(float32(0.6*(1-s))))
		if s < 0.15 {
			sprite(c.add(up.mul(0.1)), 0.5, rgba{1, 1, 1, float32(0.7 * (1 - s/0.15))})
		}
	}
	if age < 0.62 {
		// the capsule shell still sitting in the cradle while the base opens round it
		k := (age - 0.45) / 0.17
		hemi(&bEnv, c.add(up.mul(0.075+lift)), up, 0.055*(1-k*0.6), cMetalDk, 3, 16)
		return true
	}
	// shaft: the full detailed column, growing from housing height to full height
	const h0 = 0.7
	pop := math.Min(1, (age-0.62)/0.15)
	Hs := h0 + (H-h0)*ext
	genShaftGeom(p, Hs, prev, next, lift)
	if pop < 1 {
		// the housing snaps up out of the base in a burst of light and steam
		sprite(c.add(up.mul(0.3)), 0.5*(1-pop)+0.1, cEnergyHot.alpha(float32(0.8*(1-pop))))
		addPoint(c.add(up.mul(0.3)), cEnergyHot, 6*(1-pop), 3)
		if rand.Float64() < 0.3 {
			spawnSteam(c.add(up.mul(0.05)), v3(rand.NormFloat64(), 0.2, rand.NormFloat64()).norm(), 2)
		}
	}
	topY := math.Max(Hs, h0)
	if ext > 0 && ext < 1 {
		// the moving collar where the next stage slides out
		y := 0.42 + (topY-0.64)*ext
		ringSoft(c.add(up.mul(y)), 0.06, 0.02, col.alpha(0.8), 20)
		if rand.Float64() < 0.08 {
			spawnSparks(c.add(up.mul(y)), v3(rand.NormFloat64(), 0.3, rand.NormFloat64()).norm(), 3, 0.2)
		}
	}
	sprite(c.add(up.mul(topY+0.09)), 0.06, col.alpha(0.8))
	return true
}

func (e *editor) rayGen(o, d vec3) (int, float64) {
	best, bt := -1, math.Inf(1)
	const R = 0.13
	a := d.x*d.x + d.z*d.z
	if a < 1e-8 {
		return -1, 0
	}
	for i, p := range e.pts {
		fx, fz := o.x-p[0], o.z-p[1]
		b := 2 * (fx*d.x + fz*d.z)
		c := fx*fx + fz*fz - R*R
		disc := b*b - 4*a*c
		if disc < 0 {
			continue
		}
		sq := math.Sqrt(disc)
		for _, tt := range []float64{(-b - sq) / (2 * a), (-b + sq) / (2 * a)} {
			if tt <= 0.02 || tt >= bt {
				continue
			}
			y := o.y + d.y*tt
			if y < -0.05 || y > e.height+0.2 {
				continue
			}
			best, bt = i, tt
			break
		}
	}
	return best, bt
}

func (e *editor) handleRayGen(h *hand) bool {
	if !h.aimValid || len(e.pts) < 3 {
		h.rayGrab = false
		return false
	}
	o := h.aim.pos()
	d := h.aim.axis(2).mul(-1)
	if h.rayGrab {
		if !h.held(bitTrigger) || h.grab < 0 || h.grab >= len(e.pts) {
			e.grabEnd(h)
			h.rayGrab = false
			h.grab = -1
			e.theta = domAngle(e.pts)
			haptic(h.idx, 800)
			e.sfx(4, h.aim.pos(), 0.25)
			return true
		}
		raw := o.add(d.mul(h.rayDist))
		if h.rf == nil {
			h.rf = newPointFilter()
		}
		q := h.rf.step(pt2{raw.x, raw.z}, frameDT)
		e.pts[h.grab] = e.wallPush(h, e.snapped(h.grab, pt2{q[0] + h.grabOff[0], q[1] + h.grabOff[1]}))
		e.grabUpdate(h)
		gp := e.pts[h.grab]
		h.rayHit, h.rayHitOK = v3(gp[0], math.Max(0.1, math.Min(e.height, raw.y)), gp[1]), true
		h.hoverV, h.hoverE = h.grab, -1
		return true
	}
	gi, tt := e.rayGen(o, d)
	if gi < 0 {
		return false
	}
	if c, ok := h.pointer(true); ok && fl(c, 0).sub(o).length() < tt-0.05 {
		return false
	}
	hp := o.add(d.mul(tt))
	if h.hoverV != gi {
		haptic(h.idx, 400)
	}
	h.hoverV, h.hoverE = gi, -1
	h.rayHit, h.rayHitOK = hp, true
	switch {
	case h.down(bitTrigger):
		for _, ot := range e.hands {
			if ot != h && ot.grab == gi {
				return true
			}
		}
		e.push()
		gp := e.pts[gi]
		h.grab, h.rayGrab, h.rayDist = gi, true, tt
		e.grabStart(h, gi, false)
		h.grabOff = pt2{gp[0] - hp.x, gp[1] - hp.z}
		h.rf = newPointFilter()
		h.rf.step(pt2{hp.x, hp.z}, frameDT)
		haptic(h.idx, 1500)
		e.sfx(sndTool, hp, 0.3)
	case h.down(bitGrip):
		if len(e.pts) <= 3 {
			e.say("A boundary needs at least 3 points")
			return true
		}
		e.push()
		e.releaseAll()
		e.startRemoval(gi)
		e.pts = append(e.pts[:gi], e.pts[gi+1:]...)
		e.theta = domAngle(e.pts)
		haptic(h.idx, 2500)
		e.say("Point deleted")
	case h.down(bitStick):
		e.doUndo()
	}
	return true
}
