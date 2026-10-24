package main

// Each generator carries a service panel on the side facing into your room: a breaker
// lever (off shrinks your room on purpose), a pump handle that tops the charge up, a charge
// gauge, and - when it's blown - a work order and the fixtures its tasks need: a patch cable
// to run from the base to the top, a crank, a fuse to swap, a coolant valve, the breaker.

import (
	"boundaryvr/sim"
	"math"
)

type genPanel struct {
	i         int
	F         xf // origin on the floor in front of the generator, z facing into the room
	brk       *lever
	pump      *slider
	crk       *crank
	valve     *crank
	plug      *prop
	fuseOld   *prop
	fuseNew   *prop
	fuseOut   bool
	turns0    float64 // crank/valve turns when the task started
	task      string
	brkDown   bool
	leechGrab *leechHandle
	beepT     float64  // low-charge warning beep timer
	posts     []int    // the generator posts it runs (two for a pair of close corners)
	cover     *fastSet // a scorched cover screwed or bolted on (for that work order)
}

// inwardAt: the way into the ring at generator i - the inside bisector of its two beams -
// so its panel faces into the room whatever shape the room is.
func inwardAt(G []sim.V2, i int) sim.V2 {
	n := len(G)
	return inwardOf(G[(i-1+n)%n], G[i], G[(i+1)%n], G)
}

// inwardOf: into the ring G at p, between beams from a and to b.
func inwardOf(a, p, b sim.V2, G []sim.V2) sim.V2 {
	e1, e2 := p.Sub(a).Norm(), b.Sub(p).Norm()
	nm := e1.Perp().Add(e2.Perp()).Norm()
	if nm.Len() < 0.5 {
		nm = e2.Perp()
	}
	if !sim.InPoly(G, p.Add(nm.Mul(0.2))) {
		nm = nm.Mul(-1)
	}
	return nm
}

func (g *game) buildPanels() {
	r := g.r
	g.panels_ = nil
	G := r.Gens2()
	for i := range r.Gens {
		i := i
		if r.Gens[i].Lead != i {
			continue // a second post of a generator: it shares the first one's panel
		}
		// one panel per generator, between its posts, facing into the room
		posts := g.postsOf(i)
		p := sim.V2{}
		for _, k := range posts {
			p = p.Add(G[k])
		}
		p = p.Mul(1 / float64(len(posts)))
		in := inwardAt(G, i)
		if len(posts) > 1 {
			n := len(G)
			in = inwardOf(G[(posts[0]-1+n)%n], p, G[(posts[len(posts)-1]+1)%n], G)
		}
		o := p.Add(in.Mul(0.17))
		zx := fv(in, 0)
		xx := zx.cross(v3(0, 1, 0)).mul(-1).norm()
		F := xf{fv(o, 0), xx, v3(0, 1, 0), zx}
		gp := &genPanel{i: i, F: F, posts: posts}
		gp.brk = &lever{piv: F.p(-0.11, 1.22, 0.05), ax: F.x, d0: F.y, L: 0.15, a0: 0.45, a1: 2.6, ang: 0.45, atEnd: 0}
		gp.brk.onEnd = func(end int) { g.breaker(i, end) }
		gp.pump = &slider{base: F.p(0.1, 1.48, 0.08), dir: v3(0, -1, 0), L: 0.3}
		gp.pump.ok = func() bool { s := r.Gens[i].State; return s == sim.GenUp || s == sim.GenOff }
		gp.pump.onStroke = func() { r.Pump(i) }
		gp.crk = &crank{c: F.p(-0.1, 0.72, 0.06), ax: F.z, ref: F.y, R: 0.08}
		gp.crk.ok = func() bool { return gp.task == "crank" }
		gp.valve = &crank{c: F.p(0.02, 0.45, 0.1), ax: F.z, ref: F.y, R: 0.09}
		gp.valve.ok = func() bool { return gp.task == "valve" }
		base := F.p(0.0, 0.22, 0.09)
		gp.plug = &prop{radius: 0.08, hy: 0.03, cordLen: 2.0}
		gp.plug.home = func() (vec3, vec3, vec3, vec3) { return base, F.x, F.y, F.z }
		gp.plug.homeR = 0.06
		gp.plug.docked = true
		gp.plug.anchor = func() vec3 { return F.p(0, 0.18, 0.02) }
		gp.plug.ok = func() bool { return gp.task == "patch" }
		gp.fuseOld = &prop{radius: 0.07, hy: 0.02}
		fh := F.p(0.12, 0.92, 0.07)
		gp.fuseOld.home = func() (vec3, vec3, vec3, vec3) { return fh, F.x, F.y, F.z }
		gp.fuseOld.homeR = 0.0
		gp.fuseOld.docked = true
		gp.fuseOld.ok = func() bool { return gp.task == "fuse" && !gp.fuseOut }
		gp.fuseNew = &prop{radius: 0.07, hy: 0.02}
		fc := F.p(0.17, 0.66, 0.07)
		gp.fuseNew.home = func() (vec3, vec3, vec3, vec3) { return fc, F.x, F.y, F.z }
		gp.fuseNew.homeR = 0.05
		gp.fuseNew.docked = true
		gp.fuseNew.ok = func() bool { return gp.task == "fuse" && gp.fuseOut }
		g.panels_ = append(g.panels_, gp)
	}
}

// postsOf: the posts of generator i's machine (i first).
func (g *game) postsOf(i int) []int {
	l := g.r.Gens[i].Lead
	var out []int
	for k, gen := range g.r.Gens {
		if gen.Lead == l {
			out = append(out, k)
		}
	}
	return out
}

// panelOf: the panel of the generator post i belongs to.
func (g *game) panelOf(i int) *genPanel {
	l := g.r.Gens[i].Lead
	for _, gp := range g.panels_ {
		if gp.i == l {
			return gp
		}
	}
	return nil
}

func (g *game) breaker(i, end int) {
	r := g.r
	gp := g.panelOf(i)
	gen := &r.Gens[i]
	if gen.State == sim.GenBlown && gp.task == "breaker" {
		if end == 1 {
			gp.brkDown = true
		} else if gp.brkDown {
			gp.brkDown = false
			g.taskDone(i)
		}
		return
	}
	r.SetPower(i, end == 0)
}

func (g *game) taskDone(i int) {
	r := g.r
	gp := g.panelOf(i)
	gp.task = ""
	r.RepairTask(i)
	spawnSparks(gp.F.p(0, 1.2, 0.1), gp.F.z, 15, 0.5)
}

// panelFrame: per frame, keep each panel's task fixtures in step with its generator.
func (g *game) panelFrame(dt float64) {
	r := g.r
	// a tool's seated on a head only while some fastener's frame (here or on the cart) says so
	if g.driver != nil {
		g.driver.seated = false
	}
	if g.wrench != nil {
		g.wrench.seated = false
	}
	for _, gp := range g.panels_ {
		i := gp.i
		gen := &r.Gens[i]
		want := ""
		if gen.State == sim.GenBlown && gen.Done < len(gen.Order) {
			want = gen.Order[gen.Done]
		}
		if want != gp.task {
			gp.task = want
			gp.turns0 = 0
			gp.crk.turns, gp.valve.turns = 0, 0
			gp.fuseOut = false
			gp.fuseOld.docked, gp.fuseNew.docked = true, true
			gp.plug.docked = true
			gp.brkDown = false
			gp.cover = nil
			if want == "screws" || want == "bolts" {
				kind := fastScrew
				if want == "bolts" {
					kind = fastBolt
				}
				F := gp.F
				gp.cover = newFastSet(g, kind, 2, func(k int) (vec3, vec3, vec3) {
					return F.p([]float64{0.045, 0.115}[k], 0.765, 0.047), F.z, F.y // on the panel's face, under the gauge
				})
			}
		}
		gp.pump.tick(dt)
		// a generator running low beeps, faster the lower it gets, so you can hear which one
		if k := gen.Charge / r.T.ChargeMax; gen.State == sim.GenUp && k < 0.3 && r.Phase == sim.Playing {
			if gp.beepT -= dt; gp.beepT <= 0 {
				gp.beepT = 0.45 + 2.2*k
				ed.sfx(sndBeep, fv(gen.Pos, genH-0.3), 0.45)
			}
		} else {
			gp.beepT = 0
		}
		for _, p := range []*prop{gp.plug, gp.fuseOld, gp.fuseNew} {
			p.tick(dt)
		}
		// the breaker follows the generator when nobody holds it
		if !gp.brk.held {
			want := gp.brk.a0
			if gen.State == sim.GenOff || gp.brkDown {
				want = gp.brk.a1
			}
			gp.brk.ang += (want - gp.brk.ang) * math.Min(1, dt*8)
			gp.brk.atEnd = 0
			if gen.State == sim.GenOff || gp.brkDown {
				gp.brk.atEnd = 1
			}
		}
		switch gp.task {
		case "screws", "bolts":
			if gp.cover != nil {
				gp.cover.frame(dt)
				if gp.cover.allOut() {
					ed.sfx(sndClang, gp.F.p(0.08, 0.765, 0.05), 0.6)
					spawnSparks(gp.F.p(0.08, 0.765, 0.05), gp.F.z, 12, 0.5)
					gp.cover = nil
					g.taskDone(i)
				}
			}
		case "crank":
			if math.Abs(gp.crk.turns) >= 5 {
				g.taskDone(i)
			}
		case "valve":
			if math.Abs(gp.valve.turns) >= 3 {
				g.taskDone(i)
			}
		case "patch":
			top := gp.F.p(0.0, 1.78, 0.09)
			if !gp.plug.docked && gp.plug.pos.sub(top).length() < 0.06 {
				if h := g.handHolding(gp.plug); h != nil {
					h.held = nil
					gp.plug.held = false
				}
				gp.plug.home = func() (vec3, vec3, vec3, vec3) { return top, gp.F.x, gp.F.y, gp.F.z }
				gp.plug.docked = true
				ed.sfx(sndClang, top, 0.6)
				g.taskDone(i)
				base := gp.F.p(0.0, 0.22, 0.09)
				F := gp.F
				// back to the base socket for next time (after a beat)
				g.later(1.5, func() {
					gp.plug.home = func() (vec3, vec3, vec3, vec3) { return base, F.x, F.y, F.z }
				})
			}
		case "fuse":
			hold := gp.F.p(0.12, 0.92, 0.07)
			if !gp.fuseOut && gp.fuseOld.held && gp.fuseOld.pos.sub(hold).length() > 0.06 {
				gp.fuseOut = true
				ed.sfx(sndRip, hold, 0.4)
			}
			if gp.fuseOut && gp.fuseNew.held && gp.fuseNew.pos.sub(hold).length() < 0.05 {
				if h := g.handHolding(gp.fuseNew); h != nil {
					h.held = nil
					gp.fuseNew.held = false
				}
				gp.fuseNew.docked = true
				ed.sfx(sndClang, hold, 0.6)
				g.taskDone(i)
			}
		}
	}
}

// genGrabbers: everything on the generators you can take hold of.
func (g *game) genGrabbers(out []grabber) []grabber {
	for _, gp := range g.panels_ {
		out = append(out, gp.brk, gp.pump, gp.crk, gp.valve, gp.plug, gp.fuseOld, gp.fuseNew)
		if gp.cover != nil {
			out = gp.cover.grabbers(out)
		}
	}
	// leeches: grab one's tether and yank it off
	for _, b := range g.r.Robots {
		if b.Kind == sim.Drone && b.State == sim.RAttached {
			lh := g.leeches[b.ID]
			if lh == nil {
				lh = &leechHandle{g: g, id: b.ID}
				g.leeches[b.ID] = lh
			}
			lh.at, _ = g.leechClamp(b.Gen)
			out = append(out, lh)
		}
	}
	return out
}

// ---- drawing ----

func (g *game) drawPanels() {
	r := g.r
	t := fxTime()
	for _, gp := range g.panels_ {
		i := gp.i
		gen := &r.Gens[i]
		F := gp.F
		seedTex(12000 + float64(i)*90)
		// the panel hangs off the post on two arms
		useMat(mPaint)
		bbox(&bEnv, F, 0, 1.12, 0.03, 0.17, 0.42, 0.012, 0.006, rgba{0.2, 0.22, 0.2, 1})
		useMat(mCast)
		for _, y := range []float64{0.8, 1.45} {
			if len(gp.posts) > 1 {
				// a pair of posts: arms out to each
				for _, k := range gp.posts {
					cyl(&bEnv, F.p(0, y, 0.02), fv(r.Gens[k].Pos, y), 0.012, 0.012, 8, cIron, false)
				}
				continue
			}
			cyl(&bEnv, F.p(0, y, 0.02), F.p(0, y, -0.15), 0.012, 0.012, 8, cIron, false)
		}
		useMat(mHazard)
		bbox(&bEnv, F, 0, 1.55, 0.04, 0.17, 0.012, 0.004, 0.002, rgba{0.8, 0.6, 0.1, 1})
		// charge gauge
		k := gen.Charge / r.T.ChargeMax
		for s := 0; s < 10; s++ {
			y := 0.84 + float64(s)*0.05
			c := cMuted.scale(0.15)
			if float64(s)+0.5 <= k*10 {
				c = cEnergy
				if k < 0.3 {
					c = cDanger
				}
				if gen.State != sim.GenUp {
					c = c.scale(0.4)
				}
			}
			p := F.p(-0.02, y, 0.045)
			curEmis = 2
			useMat(mPaint)
			bbox(&bEnv, F, -0.02, y, 0.045, 0.025, 0.016, 0.004, 0.002, c)
			curEmis = 1
			if c != cMuted.scale(0.15) {
				sprite(p.add(F.z.mul(0.01)), 0.03, c.alpha(0.35))
			}
		}
		// status text
		label, lc := "ONLINE", cEnergy
		switch gen.State {
		case sim.GenOff:
			label, lc = "SWITCHED OFF", cOffAmber
		case sim.GenBlown:
			label, lc = "BLOWN", cDanger
		case sim.GenLeeched:
			label, lc = "LEECHED: RIP THE TETHER OFF", cDanger
		}
		if gen.Leech != 0 && gen.State == sim.GenUp {
			label, lc = "LEECH! RIP THE TETHER OFF", cDanger
		}
		if gen.Pressed > 0 && gen.State == sim.GenUp {
			label, lc = "UNDER ATTACK", cAmber
		}
		fl := float32(1)
		if gen.State != sim.GenUp || gen.Leech != 0 {
			fl = float32(0.6 + 0.4*math.Sin(t*6))
		}
		drawTextCentered(&bOpaque, label, F.p(0, 1.5, 0.045), F.x, F.y, 0.0016, lc.alpha(fl))
		// breaker lever
		g.drawLever(gp.brk, cDanger)
		drawTextCentered(&bOpaque, "ON", F.p(-0.11, 1.42, 0.045), F.x, F.y, 0.0011, cEnergy)
		drawTextCentered(&bOpaque, "OFF", F.p(-0.11, 0.98, 0.045), F.x, F.y, 0.0011, cOffAmber)
		// pump
		pu := gp.pump
		useMat(mSteel)
		cyl(&bEnv, pu.base.add(v3(0, 0.04, 0)), pu.base.add(pu.dir.mul(pu.L+0.02)), 0.012, 0.012, 8, cSteelEnv, true)
		h := pu.handle()
		useMat(mRubber)
		cyl(&bEnv, h.sub(F.x.mul(0.05)), h.add(F.x.mul(0.05)), 0.016, 0.016, 10, rgba{0.05, 0.05, 0.05, 1}, true)
		useMat(mCast)
		cyl(&bEnv, h, h.add(v3(0, 0.05, 0)), 0.01, 0.01, 8, cIron, false)
		if gen.State == sim.GenUp && k < 0.5 {
			sprite(h, 0.05, cAmber.alpha(float32(0.3+0.3*math.Sin(t*5))))
		}
		drawTextCentered(&bOpaque, "PUMP", F.p(0.1, 1.53, 0.045), F.x, F.y, 0.001, cWhite)
		if gp.cover != nil {
			// the scorched cover, held on by its screws or bolts
			useMat(mSteel)
			bbox(&bEnv, F, 0.08, 0.765, 0.0445, 0.06, 0.03, 0.0025, 0.002, rgba{0.22, 0.18, 0.15, 1})
			if rngF() < frameDT*2 {
				spawnSmoke(F.p(0.08, 0.79, 0.05), 1)
			}
			gp.cover.draw(true)
		}
		if gen.State == sim.GenBlown {
			g.drawWorkOrder(gp, gen)
		}
		// the patch cable lives at the base whether or not it's needed
		g.drawPlug(gp)
		if gp.task == "crank" {
			g.drawCrank(gp.crk, 0.08, cAmber)
		}
		if gp.task == "valve" {
			g.drawValve(gp.valve)
		}
		if gp.task == "fuse" {
			g.drawFuse(gp.fuseOld, gp.fuseOut, true)
			g.drawFuse(gp.fuseNew, false, false)
			hold := F.p(0.12, 0.92, 0.07)
			useMat(mCast)
			bbox(&bEnv, F, 0.12, 0.92, 0.055, 0.02, 0.04, 0.012, 0.004, cIron)
			if gp.fuseOut {
				sprite(hold, 0.05, cAmber.alpha(float32(0.5+0.4*math.Sin(t*8))))
			}
		}
	}
}

func (g *game) drawWorkOrder(gp *genPanel, gen *sim.Generator) {
	F := gp.F
	// a paper card clipped to the side of the panel
	c := F.p(0.32, 1.25, 0.04)
	bTrans.quadC(c.add(F.x.mul(-0.12)).add(F.y.mul(-0.16)), c.add(F.x.mul(0.12)).add(F.y.mul(-0.16)), c.add(F.x.mul(0.12)).add(F.y.mul(0.16)), c.add(F.x.mul(-0.12)).add(F.y.mul(0.16)),
		rgba{0.85, 0.82, 0.7, 1}, rgba{0.85, 0.82, 0.7, 1}, rgba{0.9, 0.88, 0.76, 1}, rgba{0.9, 0.88, 0.76, 1})
	ink := rgba{0.1, 0.1, 0.12, 1}
	drawText(&bOpaque, "WORK ORDER", c.add(F.x.mul(-0.11)).add(F.y.mul(0.13)).add(F.z.mul(0.002)), F.x, F.y, 0.0011, ink)
	for k, id := range gen.Order {
		name := taskName(g.r, id)
		col := ink
		mark := "[ ] "
		if k < gen.Done {
			mark = "[X] "
			col = rgba{0.2, 0.45, 0.2, 1}
		} else if k == gen.Done {
			col = rgba{0.6, 0.1, 0.05, 1}
		}
		lines := wrapWords(mark+name, 22)
		for li, l := range lines {
			drawText(&bOpaque, l, c.add(F.x.mul(-0.11)).add(F.y.mul(0.09-float64(k)*0.07-float64(li)*0.017)).add(F.z.mul(0.002)), F.x, F.y, 0.0008, col)
		}
	}
}

func taskName(r *sim.Round, id string) string {
	for _, t := range r.C.RepairTasks {
		if t.ID == id {
			return t.Name
		}
	}
	return id
}

func (g *game) drawLever(l *lever, col rgba) {
	useMat(mCast)
	cyl(&bEnv, l.piv.sub(l.ax.mul(0.03)), l.piv.add(l.ax.mul(0.03)), 0.022, 0.022, 12, cIron, true)
	h := l.handle()
	useMat(mSteel)
	cyl(&bEnv, l.piv, h, 0.011, 0.011, 8, cSteelEnv, false)
	useMat(mPaint)
	cyl(&bEnv, h.sub(l.ax.mul(0.04)), h.add(l.ax.mul(0.04)), 0.018, 0.018, 10, col, true)
}

func (g *game) drawCrank(k *crank, rad float64, col rgba) {
	useMat(mCast)
	cyl(&bEnv, k.c.sub(k.ax.mul(0.02)), k.c.add(k.ax.mul(0.02)), 0.025, 0.025, 12, cIron, true)
	arm := k.c.add(rotAbout(k.ref, k.ax, k.ang).mul(k.R)).add(k.ax.mul(0.02))
	useMat(mSteel)
	cyl(&bEnv, k.c.add(k.ax.mul(0.02)), arm, 0.01, 0.01, 8, cSteelEnv, false)
	h := k.handle()
	useMat(mRubber)
	cyl(&bEnv, arm, h, 0.014, 0.014, 10, rgba{0.05, 0.05, 0.05, 1}, true)
	// progress ring
	tt := math.Min(1, math.Abs(k.turns)/5)
	for s := 0; s < 20; s++ {
		if float64(s)/20 >= tt {
			break
		}
		a0, a1 := float64(s)/20*2*math.Pi, float64(s+1)/20*2*math.Pi
		p0 := k.c.add(rotAbout(k.ref, k.ax, a0).mul(rad + 0.03))
		p1 := k.c.add(rotAbout(k.ref, k.ax, a1).mul(rad + 0.03))
		bGlowLine.line(p0, p1, col)
	}
	sprite(h, 0.05, col.alpha(0.3))
}

func (g *game) drawValve(k *crank) {
	useMat(mPaint)
	const seg = 18
	for s := 0; s < seg; s++ {
		a0, a1 := k.ang+float64(s)*2*math.Pi/seg, k.ang+float64(s+1)*2*math.Pi/seg
		p0 := k.c.add(rotAbout(k.ref, k.ax, a0).mul(k.R))
		p1 := k.c.add(rotAbout(k.ref, k.ax, a1).mul(k.R))
		cyl(&bEnv, p0, p1, 0.012, 0.012, 6, cPipeRed, false)
	}
	for s := 0; s < 4; s++ {
		cyl(&bEnv, k.c, k.c.add(rotAbout(k.ref, k.ax, k.ang+float64(s)*math.Pi/2).mul(k.R)), 0.008, 0.008, 6, cPipeRed, false)
	}
	useMat(mCast)
	cyl(&bEnv, k.c.sub(k.ax.mul(0.1)), k.c, 0.02, 0.02, 10, cIron, true)
	tt := math.Min(1, math.Abs(k.turns)/3)
	sprite(k.handle(), 0.05, cAmber.alpha(0.4))
	drawTextCentered(&bOpaque, itoa(int(tt*100))+"%", k.c.add(k.ax.mul(0.01)).add(v3(0, k.R+0.04, 0)), k.ax.cross(v3(0, 1, 0)).mul(-1).norm(), v3(0, 1, 0), 0.0012, cAmber)
}

func (g *game) drawPlug(gp *genPanel) {
	p := gp.plug
	F := gp.F
	top := F.p(0.0, 1.78, 0.09)
	// sockets
	useMat(mCast)
	for _, s := range []vec3{F.p(0, 0.22, 0.075), F.p(0, 1.78, 0.075)} {
		cyl(&bEnv, s.sub(F.z.mul(0.02)), s, 0.03, 0.03, 12, cIron, true)
	}
	// the top socket's on a bracket clamped round the shaft (it's above the panel)
	useMat(mSteel)
	bbox(&bEnv, F, 0, 1.78, 0.0275, 0.026, 0.034, 0.0275, 0.004, cSteelEnv)
	cyl(&bEnv, F.p(0, 1.745, -0.02), F.p(0, 1.815, -0.02), 0.062, 0.062, 16, cSteelEnv, true)
	topText(&bOpaque, "PATCH", F.p(0, 1.825, 0.06), F.y, 0.0009, cMuted)
	if gp.task == "patch" {
		t := fxTime()
		sprite(top, 0.06, cAmber.alpha(float32(0.4+0.4*math.Sin(t*6))))
		ringSoft(top, 0.05, 0.01, cAmber.alpha(0.8), 20)
	}
	drawCord(cordPts(p.anchor(), p.pos, p.cordLen, 12), 0.008, rgba{0.85, 0.5, 0.05, 1})
	useMat(mPaint)
	bevelBox(&bEnv, p.pos, p.X, p.Y, p.Z, 0.016, 0.016, 0.03, 0.004, rgba{0.85, 0.5, 0.05, 1})
	useMat(mChrome)
	cyl(&bEnv, p.pos.add(p.Z.mul(-0.03)), p.pos.add(p.Z.mul(-0.05)), 0.006, 0.006, 6, cChromeEnv, true)
}

func (g *game) drawFuse(p *prop, out, burnt bool) {
	if p.docked && burnt && out {
		return
	}
	useMat(mGlass)
	col := rgba{0.6, 0.75, 0.8, 1}
	if burnt {
		col = rgba{0.2, 0.18, 0.15, 1}
	}
	cyl(&bEnv, p.pos.sub(p.Y.mul(0.035)), p.pos.add(p.Y.mul(0.035)), 0.012, 0.012, 10, col, true)
	useMat(mChrome)
	for _, s := range []float64{-1, 1} {
		cyl(&bEnv, p.pos.add(p.Y.mul(s*0.03)), p.pos.add(p.Y.mul(s*0.045)), 0.013, 0.013, 10, cChromeEnv, true)
	}
}
