package main

// The device (the EMP) on its cart. Nothing on it is a button you just press: you unscrew
// the access plate, wire the capacitor bank, play whatever game its compartment wants, set
// three dials, wind a coil that leaks the moment you let go, read a serial number out of a
// dark slot with a flashlight and key it in, then pull the cord until it starts. The cart
// rolls: grab a push handle and drag it wherever your ring still covers.

import (
	"boundaryvr/sim"
	"math"
	"math/rand"
)

// cart dimensions
const (
	devTop  = 1.04 // top of the housing
	devMidY = 0.88
	devHW   = 0.25 // housing half width (x)
	devHL   = 0.33 // housing half length (z)
	devHH   = 0.16 // housing half height
	handleZ = 0.47
	handleY = 1.0
)

type devVisual struct {
	mods      []devModule
	handles   [2]*cartHandle
	vel       sim.V2 // the cart's own speed (it's heavy: it lags behind your hands)
	rumbleT   float64
	flash     *prop
	litSlot   float64
	lastCur   int
	startedAt []bool
	fired     float64
}

type devModule interface {
	start(g *game)
	frame(g *game, F xf, dt float64)
	draw(g *game, F xf, current, done bool)
	grabbers(out []grabber) []grabber
	pokers(out []poker) []poker
}

// devFrame: the cart's frame (x across, y up, z along it), on the floor.
func (g *game) devFrame() xf {
	d := &g.r.Dev
	f := v3(math.Cos(d.Yaw), 0, math.Sin(d.Yaw))
	s := v3(-f.z, 0, f.x)
	F := xf{fv(d.Pos, 0), s, v3(0, 1, 0), f}
	if k := g.engineShake(); k > 0 && d.State == sim.DevHome {
		// the engine's running: the whole cart judders
		t := fxTime()
		F.o = F.o.add(v3(math.Sin(t*61)*k, math.Abs(math.Sin(t*47))*k*0.6, math.Sin(t*53+1)*k))
	}
	if d.State == sim.DevStolen {
		F.o = F.o.add(v3(0, 0.22, 0))
	}
	if d.State == sim.DevDropped {
		F.y = v3(0, 1, 0).mul(0.25).add(s.mul(0.97)).norm()
		F.x = s.mul(0.25).sub(v3(0, 0.97, 0)).norm()
		F.o = F.o.add(v3(0, 0.27, 0))
	}
	return F
}

func (g *game) buildDevice() {
	propSupport = g.supportAt
	cartLid = func() (xf, float64, float64, float64, bool) {
		if g.r == nil || g.r.Dev.State == sim.DevDropped {
			return xf{}, 0, 0, 0, false
		}
		return g.devFrame(), devHW, devHL, devTop, true
	}
	dv := &g.devVis
	r := g.r
	dv.mods = nil
	for i, m := range r.Dev.Mods {
		var dm devModule
		switch m.ID {
		case "screws":
			dm = &modScrews{}
		case "wires":
			dm = &modWires{}
		case "game":
			dm = &modGame{idx: i}
		case "dials":
			dm = &modDials{}
		case "crank":
			dm = &modCrank{}
		case "serial":
			dm = &modSerial{}
		case "ripcord":
			dm = &modRip{}
		default:
			dm = &modScrews{}
		}
		dv.mods = append(dv.mods, dm)
	}
	dv.startedAt = make([]bool, len(dv.mods))
	dv.lastCur = -1
	dv.handles = [2]*cartHandle{{g: g, end: 1}, {g: g, end: -1}}
	dv.flash = &prop{radius: 0.09, hy: 0.03}
	dv.flash.home = func() (vec3, vec3, vec3, vec3) {
		F := g.devFrame()
		return F.p(devHW+0.06, 0.62, -0.1), F.y.mul(-1), F.x, F.z.mul(-1)
	}
	dv.flash.homeR = 0.12
	dv.flash.docked = true
	dv.flash.X, dv.flash.Y, dv.flash.Z = v3(1, 0, 0), v3(0, 1, 0), v3(0, 0, 1)
}

// deviceFrame: per frame logic: which module is live, the cart handles, the flashlight.
func (g *game) deviceFrame(dt float64) {
	r := g.r
	d := &r.Dev
	dv := &g.devVis
	F := g.devFrame()
	if d.Cur != dv.lastCur {
		if d.Cur < len(dv.mods) {
			// a fresh module, or one the device threw you back into
			dv.mods[d.Cur].start(g)
		}
		dv.lastCur = d.Cur
	}
	for i, m := range dv.mods {
		if i == d.Cur && d.State == sim.DevHome && r.Phase == sim.Playing {
			m.frame(g, F, dt)
		}
	}
	dv.flash.tick(dt)
	g.cartFrame(dt)
	g.driverFrame(dt)
	g.wrenchFrame(dt)
	g.poleFrame(dt)
	g.leadsFrame(dt)
	g.misfireFrame(dt)
	g.engineFrame()
	if d.State == sim.DevHome && d.Fired {
		dv.fired += dt
	}
}

// flashOn: how well the flashlight lights point p from the front (0..1).
func (g *game) flashOn(p vec3) float64 {
	f := g.devVis.flash
	if !f.held {
		return 0
	}
	ld := f.Z.mul(-1)
	to := p.sub(f.pos)
	L := to.length()
	if L > 1.2 || L < 0.02 {
		return 0
	}
	c := to.mul(1 / L).dot(ld)
	return smooth((c - math.Cos(25*math.Pi/180)) / (1 - math.Cos(25*math.Pi/180)) * 3)
}

func (g *game) devGrabbers(out []grabber) []grabber {
	d := &g.r.Dev
	dv := &g.devVis
	out = append(out, dv.handles[0], dv.handles[1], dv.flash)
	if g.driver != nil {
		out = append(out, g.driver.p)
	}
	if g.wrench != nil {
		out = append(out, g.wrench.p)
	}
	out = g.leadGrabbers(out)
	if g.misfire != nil {
		out = g.misfire.grabbers(out)
	}
	if d.State == sim.DevHome && d.Cur < len(dv.mods) && g.r.Phase == sim.Playing {
		out = dv.mods[d.Cur].grabbers(out)
	}
	return out
}

func (g *game) devPokers(out []poker) []poker {
	d := &g.r.Dev
	dv := &g.devVis
	if d.State == sim.DevHome && d.Cur < len(dv.mods) && g.r.Phase == sim.Playing {
		out = dv.mods[d.Cur].pokers(out)
	}
	return out
}

// ---- the cart's push handles (drag it; yank it off a warden) ----

type cartHandle struct {
	g       *game
	end     float64 // +1 front, -1 back
	off     sim.V2  // cart centre relative to the hand when grabbed
	tugT    float64
	tugAt   vec3
	target  sim.V2 // where this hand is hauling the cart's centre to this frame
	pulling bool
}

// The cart is heavy: it follows your hands on a stiff spring with a top speed (much faster
// with both hands on it), and if you try to yank it further than that your grip slips.
const (
	cartSpeed1 = 0.35 // m/s with one hand
	cartSpeed2 = 0.8  // with both
	cartSlip   = 0.57 // how far ahead of the cart your hand can get before it slips off
)

func (c *cartHandle) gPos() vec3 {
	return c.g.devFrame().p(0, handleY, c.end*handleZ)
}
func (c *cartHandle) gRad() float64 { return 0.13 }
func (c *cartHandle) gOK() bool {
	d := &c.g.r.Dev
	if d.State == sim.DevGone || c.g.r.Phase == sim.Won {
		return false
	}
	// it has to be somewhere you can actually stand
	return sim.InPoly(c.g.r.Room, d.Pos)
}
func (c *cartHandle) gGrab(h *gHand) {
	d := &c.g.r.Dev
	c.off = d.Pos.Sub(sim.V(h.palm.x, h.palm.z))
	c.tugAt = h.palm
	if d.State == sim.DevDropped {
		c.g.r.SetDevice(true, d.Pos, d.Yaw)
	} else if d.State == sim.DevHome {
		c.g.r.SetDevice(true, d.Pos, d.Yaw)
	}
}
func (c *cartHandle) gHold(h *gHand, dt float64) {
	g := c.g
	d := &g.r.Dev
	switch d.State {
	case sim.DevStolen:
		// tug of war: yank it away from the warden
		c.tugT -= dt
		carrier := g.robotByID(d.Carrier)
		h.buzz(10, 0.9)
		if carrier != nil && c.tugT <= 0 {
			away := sim.V(h.palm.x, h.palm.z).Sub(carrier.Pos).Norm()
			sp := h.vel.x*away.X + h.vel.z*away.Z
			if sp > 1.1 {
				c.tugT = 0.6
				if g.r.Tug() {
					c.off = d.Pos.Sub(sim.V(h.palm.x, h.palm.z))
					h.buzz(80, 1)
				} else {
					h.buzz(60, 0.6)
				}
			}
		}
	case sim.DevHome:
		c.target, c.pulling = sim.V(h.palm.x, h.palm.z).Add(c.off), true
		if sim.Dist(c.target, d.Pos) > cartSlip {
			// too heavy to be yanked like that: your hand slips off the handle
			h.held = nil
			c.pulling = false
			c.gRelease(h)
			h.buzz(70, 1)
			ed.sfx(sndCreak, c.gPos(), 0.35)
		}
	}
}

// housingBox: the cart's housing, as something cords lie over.
func (g *game) housingBox(F xf) cordBox {
	return cordBox{F: xf{F.p(0, 0, 0), F.x, F.y, F.z}, hx: devHW, hz: devHL, y0: devMidY - devHH, y1: devTop}
}

// cartFrame: the cart rolls toward wherever the hands on its handles are hauling it.
func (g *game) cartFrame(dt float64) {
	d := &g.r.Dev
	dv := &g.devVis
	var tgt sim.V2
	n := 0
	for _, c := range dv.handles {
		if c != nil && c.pulling && g.handHolding(c) != nil {
			tgt = tgt.Add(c.target)
			n++
		}
		if c != nil {
			c.pulling = false
		}
	}
	if n == 0 || d.State != sim.DevHome || dt <= 0 {
		dv.vel = sim.V2{} // no coasting: it stops when you let go
		return
	}
	tgt = tgt.Mul(1 / float64(n))
	k, top := 30.0, cartSpeed1
	if n == 2 {
		k, top = 45.0, cartSpeed2
	}
	dv.vel = dv.vel.Add(tgt.Sub(d.Pos).Mul(k * dt)).Mul(math.Max(0, 1-10*dt))
	if sp := dv.vel.Len(); sp > top {
		dv.vel = dv.vel.Mul(top / sp)
	}
	np := d.Pos.Add(dv.vel.Mul(dt))
	if !sim.InPoly(g.r.Room, np) {
		dv.vel = sim.V2{} // you can't push it through your real wall
		return
	}
	g.r.SetDevice(true, np, d.Yaw)
	// the casters rumble in your hands as it moves
	if sp := dv.vel.Len(); sp > 0.05 {
		dv.rumbleT -= dt
		if dv.rumbleT <= 0 {
			dv.rumbleT = 0.09
			for _, c := range dv.handles {
				if h := g.handHolding(c); h != nil {
					h.buzz(12, float32(math.Min(1, 0.25+sp*1.2)))
				}
			}
		}
	}
}
func (c *cartHandle) gRelease(h *gHand) {
	d := &c.g.r.Dev
	if d.State == sim.DevHome && c.g.handHolding(c.g.devVis.handles[0]) == nil && c.g.handHolding(c.g.devVis.handles[1]) == nil {
		c.g.r.SetDevice(false, d.Pos, d.Yaw)
	}
}

// ---- drawing the cart and the housing ----

func (g *game) drawDevice() {
	r := g.r
	d := &r.Dev
	if d.State == sim.DevGone {
		return
	}
	dv := &g.devVis
	t := fxTime()
	F := g.devFrame()
	seedTex(15000)
	defer withTex(F)()
	wearScale = 0.4 // you work on it up close all round
	defer func() { wearScale = 1 }()
	// casters, frame, shelf
	useMat(mCast)
	for _, sx := range []float64{-1, 1} {
		for _, sz := range []float64{-1, 1} {
			w := F.p(sx*0.22, 0.05, sz*0.3)
			cyl(&bEnv, w.sub(F.x.mul(0.022)), w.add(F.x.mul(0.022)), 0.05, 0.05, 12, cIron, true)
			cyl(&bEnv, w, F.p(sx*0.22, 0.13, sz*0.3), 0.012, 0.012, 6, cSteelEnv, false)
		}
	}
	useMat(mPaint)
	bbox(&bEnv, F, 0, 0.15, 0, 0.27, 0.018, 0.37, 0.008, rgba{0.2, 0.22, 0.2, 1})
	bbox(&bEnv, F, 0, 0.7, 0, 0.27, 0.018, 0.37, 0.008, rgba{0.2, 0.22, 0.2, 1})
	// the bottom shelf is a tray: a lip all round so the spares can't slide off
	for _, s := range []float64{-1, 1} {
		bbox(&bEnv, F, s*0.264, 0.205, 0, 0.006, 0.037, 0.37, 0.003, rgba{0.22, 0.24, 0.22, 1})
		bbox(&bEnv, F, 0, 0.205, s*0.364, 0.27, 0.037, 0.006, 0.003, rgba{0.22, 0.24, 0.22, 1})
	}
	useMat(mSteel)
	for _, sx := range []float64{-1, 1} {
		for _, sz := range []float64{-1, 1} {
			cyl(&bEnv, F.p(sx*0.25, 0.13, sz*0.35), F.p(sx*0.25, 0.71, sz*0.35), 0.014, 0.014, 8, cSteelEnv, false)
		}
	}
	// push handles
	for i, sz := range []float64{1, -1} {
		useMat(mSteel)
		for _, sx := range []float64{-1, 1} {
			cyl(&bEnv, F.p(sx*0.22, 0.71, sz*0.36), F.p(sx*0.22, handleY, sz*handleZ), 0.012, 0.012, 8, cSteelEnv, false)
		}
		useMat(mRubber)
		hc := rgba{0.06, 0.06, 0.06, 1}
		if g.handHolding(dv.handles[i]) != nil {
			hc = rgba{0.15, 0.12, 0.04, 1}
		}
		cyl(&bEnv, F.p(-0.24, handleY, sz*handleZ), F.p(0.24, handleY, sz*handleZ), 0.02, 0.02, 10, hc, true)
	}
	// housing, with the serial compartment let into its right side (+x)
	useMat(mPaint)
	hc := rgba{0.17, 0.21, 0.24, 1}
	const inner = devHW - serialDepth
	bbox(&bEnv, F, -(devHW-inner)/2, devMidY, 0, (devHW+inner)/2, devHH, devHL, 0.025, hc)
	{
		cx := (devHW + inner) / 2
		hw := (devHW - inner) / 2
		z0, z1 := serialZ-serialHW, serialZ+serialHW
		y0, y1 := serialY-serialHH, serialY+serialHH
		bot, top := devMidY-devHH, devMidY+devHH
		bbox(&bEnv, F, cx, devMidY, (-devHL+z0)/2, hw, devHH, (z0+devHL)/2, 0.006, hc) // toward the back end
		bbox(&bEnv, F, cx, devMidY, (z1+devHL)/2, hw, devHH, (devHL-z1)/2, 0.006, hc)  // toward the front end
		bbox(&bEnv, F, cx, (y1+top)/2, serialZ, hw, (top-y1)/2, serialHW, 0.004, hc)   // over the hole
		bbox(&bEnv, F, cx, (bot+y0)/2, serialZ, hw, (y0-bot)/2, serialHW, 0.004, hc)   // under it
	}
	useMat(mHazard)
	bbox(&bEnv, F, 0, devMidY-devHH+0.015, 0, devHW+0.004, 0.015, devHL+0.004, 0.004, rgba{0.8, 0.6, 0.1, 1})
	// antenna with the charge light
	useMat(mSteel)
	cyl(&bEnv, F.p(-devHW+0.05, devTop, devHL-0.05), F.p(-devHW+0.05, devTop+0.42, devHL-0.05), 0.008, 0.004, 6, cSteelEnv, false)
	tip := F.p(-devHW+0.05, devTop+0.44, devHL-0.05)
	k := r.Work() / math.Max(1, r.WorkTotal())
	curEmis = 6
	sphere(&bEnv, tip, 0.02, cBeam, 4, 8)
	curEmis = 1
	sprite(tip, 0.14, cBeam.alpha(float32(0.25+0.5*k)*float32(0.75+0.25*math.Sin(t*(4+8*k)))))
	addPoint(tip, cBeam, 0.8+3*k, 3)
	// progress lamps across the front end
	n := len(d.Mods)
	for i := 0; i < n; i++ {
		x := -0.2 + 0.4*float64(i)/float64(max(1, n-1))
		p := F.p(x, devTop-0.025, devHL+0.003)
		c := cMuted.scale(0.25)
		switch {
		case d.Mods[i].Done:
			c = cEnergy
		case i == d.Cur:
			c = cAmber.scale(float32(0.6 + 0.4*math.Sin(t*6)))
		}
		curEmis = 3
		sphere(&bEnv, p, 0.011, c, 3, 6)
		curEmis = 1
		if d.Mods[i].Done || i == d.Cur {
			sprite(p, 0.035, c.alpha(0.6))
		}
	}
	// the modules
	for i, m := range dv.mods {
		m.draw(g, F, i == d.Cur && d.State == sim.DevHome, d.Mods[i].Done)
	}
	// what to do now, on a strip over the top
	if d.State == sim.DevHome && d.Cur < len(d.Mods) && r.Phase == sim.Playing {
		mod := d.Mods[d.Cur]
		cam := ed.hmd.pos()
		p := F.p(0, devTop+0.3, 0)
		rr, uu := billboard(cam, p)
		name := mod.Name
		plate, door := g.cartFasteners()
		switch mod.ID {
		case "screws":
			name = fastVerb(plate) + " THE ACCESS PLATE"
		case "ripcord":
			rm, _ := dv.mods[d.Cur].(*modRip)
			switch {
			case d.Misfire && g.misfire != nil:
				name = "MISFIRE: " + g.misfire.stage()
			case d.Leads < sim.LeadsNeeded:
				name = "PLUG THE POWER LEADS INTO THE BACK"
			case rm != nil && rm.lit():
				name = "PUSH THE BIG RED BUTTON"
			case rm != nil && rm.running:
				name = "LET IT WARM UP"
			default:
				name = "PULL THE CORD TO START THE ENGINE"
			}
		case "serial":
			if sm, ok := dv.mods[d.Cur].(*modSerial); ok && sm.fs != nil && !sm.fs.allOut() {
				name = fastVerb(door) + " THE COMPARTMENT DOOR"
			}
		}
		drawTextCentered(&bOpaque, name, p, rr, uu, 0.0019, cAmber)
		drawTextCentered(&bOpaque, itoa(int(100*k))+"%", p.add(v3(0, -0.05, 0)), rr, uu, 0.0016, cBeam)
	}
	// the flashlight in its clip (or wherever you left it), the driver's outline
	g.drawFlashlight()
	g.drawDriverOutline(F)
	if g.wrench != nil {
		g.drawWrenchOutline(F)
	}
	if d.Exposed && d.State == sim.DevHome {
		ringSoft(fv(d.Pos, 0.01), 0.62+0.05*math.Sin(t*8), 0.04, cDanger.alpha(0.7), 40)
		if d.Field > 0 {
			g.drawField(d.Field / math.Max(0.1, r.T.FieldMax(r.Machines())))
		}
	}
	if d.State == sim.DevStolen || d.State == sim.DevDropped {
		sprite(F.p(0, devTop, 0), 0.6, cDanger.alpha(float32(0.15+0.1*math.Sin(t*9))))
	}
	shadowBlob(pv(d.Pos), 0.55, 0.6)
}

// drawField: the device's backup field, a flickering dome that thins out as it runs down.
func (g *game) drawField(k float64) {
	d := &g.r.Dev
	t := fxTime()
	c := fv(d.Pos, 0)
	const R = 0.8
	// it stutters harder the less there is left
	on := 0.55 + 0.45*k
	if math.Sin(t*(9+30*(1-k))) > 0.6+0.35*k {
		on *= 0.35
	}
	const H = 1.4 // tall enough to cover the cart and its antenna
	col := cBeam.alpha(float32(0.55 * on))
	for lat := 0; lat < 6; lat++ {
		a := float64(lat) / 6 * math.Pi / 2
		ringSoft(c.add(v3(0, H*math.Sin(a), 0)), R*math.Cos(a), 0.012, col, 40)
	}
	for m := 0; m < 10; m++ {
		az := float64(m)/10*2*math.Pi + t*0.4
		prev := c.add(v3(R*math.Cos(az), 0, R*math.Sin(az)))
		for s := 1; s <= 8; s++ {
			a := float64(s) / 8 * math.Pi / 2
			p := c.add(v3(R*math.Cos(a)*math.Cos(az), H*math.Sin(a), R*math.Cos(a)*math.Sin(az)))
			bGlowLine.line(prev, p, col)
			prev = p
		}
	}
	sprite(c.add(v3(0, 0.7, 0)), 1.8, cBeam.alpha(float32(0.06*on)))
	cam := ed.hmd.pos()
	p := c.add(v3(0, H+0.15, 0))
	rr, uu := billboard(cam, p)
	drawTextCentered(&bOpaque, "FIELD "+itoa(int(math.Ceil(d.Field))), p, rr, uu, 0.0022, cBeam)
	drawTextCentered(&bOpaque, "DRAG IT BACK IN", p.add(v3(0, -0.06, 0)), rr, uu, 0.0012, cWhite)
}

func (g *game) drawFlashlight() {
	f := g.devVis.flash
	// a flashlight in a hand is a real, shadow-casting light (it borrows the second lamp's slot)
	if f.held {
		ld := f.Z.mul(-1)
		sp := makeSpot(f.pos.add(ld.mul(0.1)), ld, rgba{1, 0.95, 0.85, 1}, 18, 22, 10, 7)
		if len(spots) >= 2 {
			spots[1] = sp
		} else {
			spots = append(spots, sp)
		}
	}
	useMat(mCast)
	a, b := f.pos.add(f.Z.mul(0.09)), f.pos.sub(f.Z.mul(0.08))
	cyl(&bEnv, a, b, 0.018, 0.018, 12, rgba{0.12, 0.12, 0.13, 1}, true)
	cyl(&bEnv, b, b.sub(f.Z.mul(0.03)), 0.026, 0.026, 14, cIron, true)
	lens := b.sub(f.Z.mul(0.031))
	if f.held {
		curEmis = 8
		hemi(&bEnv, lens, f.Z.mul(-1), 0.022, rgba{1, 0.95, 0.85, 1}, 3, 12)
		curEmis = 1
		sprite(lens, 0.06, rgba{1, 0.95, 0.85, 0.6})
		ribbonC(lens, lens.sub(f.Z.mul(0.8)), 0.25, rgba{1, 0.95, 0.85, 0.06}, rgba{1, 0.95, 0.85, 0})
	} else {
		useMat(mGlass)
		hemi(&bEnv, lens, f.Z.mul(-1), 0.022, rgba{0.4, 0.4, 0.38, 1}, 3, 12)
	}
	if f.docked {
		F := g.devFrame()
		useMat(mSteel)
		bbox(&bEnv, F, devHW+0.04, 0.62, -0.1, 0.012, 0.05, 0.03, 0.004, cSteelEnv)
	}
}

// ---- module: unscrew the access plate (with the driver you have to find) ----

const (
	screwTurns = 0.8  // turns to back a screw out
	seatR      = 0.06 // the driver's magnetic tip pulls into a screw head this close...
	seatCone   = 0.64 // ...pointing within 50 degrees of straight down
	unseatR    = 0.13 // and stays in until your hand is pulled this far off (tilting it doesn't)
	breakAway  = 0.45 // turning it takes before a screw breaks loose (radians)
)

// what turning the driver feels like (and the colour it shows)
const (
	feelSeat  = iota // seated, not turning
	feelTight        // turning the wrong way: it won't go any tighter
	feelBreak        // the right way, breaking it loose
	feelFree         // the right way, and it's coming out
)

type screw struct {
	ang   float64 // how far it's been backed out (radians)
	loose float64 // how far it's been worked loose (1: it's broken free)
	out   bool
}

type modScrews struct {
	fs    *fastSet // four screws or four bolts (whichever the plate has this round)
	F     xf
	plate float64 // 0 on .. 1 gone
}

func feelColor(f int) rgba {
	switch f {
	case feelTight:
		return cDanger
	case feelBreak:
		return cAmber
	case feelFree:
		return cEnergy
	}
	return cBeam
}

func (m *modScrews) start(g *game) {
	kind, _ := g.cartFasteners()
	m.fs = newFastSet(g, kind, 4, func(i int) (vec3, vec3, vec3) {
		return m.pos(m.F, i), m.F.y, m.F.x
	})
	m.plate = 0
}

func (m *modScrews) pos(F xf, i int) vec3 {
	x := []float64{-1, 1, -1, 1}[i] * 0.1
	z := []float64{-1, -1, 1, 1}[i] * 0.07
	return F.p(x, devTop+0.012, z)
}

// headPos: where fastener i's head is (tests reach for it)
func (m *modScrews) headPos(F xf, i int) vec3 {
	m.F = F
	return m.fs.headPos(i)
}

func (m *modScrews) frame(g *game, F xf, dt float64) {
	if m.fs == nil {
		m.start(g)
	}
	m.F = F
	m.fs.frame(dt)
	done := m.fs.progress()
	g.r.SetModuleProgress(done)
	if m.fs.allOut() {
		m.plate += dt * 2
		if m.plate >= 1 {
			g.r.CompleteModule()
		}
	}
}

func (m *modScrews) draw(g *game, F xf, current, done bool) {
	if done {
		return // the plate is off
	}
	if m.fs == nil {
		m.start(g)
	}
	m.F = F
	lift := m.plate * 0.3
	useMat(mSteel)
	bbox(&bEnv, F, 0, devTop+0.004+lift, 0, 0.13, 0.004, 0.1, 0.002, cSteelEnv.scale(0.7))
	topText(&bOpaque, "ACCESS", F.p(0, devTop+0.009+lift, 0), F.y, 0.0015, cMuted)
	m.fs.draw(current)
}

func (m *modScrews) grabbers(out []grabber) []grabber {
	if m.fs == nil {
		return out
	}
	return m.fs.grabbers(out)
}
func (m *modScrews) pokers(out []poker) []poker { return out }

// ---- module: wire the capacitor bank (three plugs into matching sockets) ----

var wireCols = [3]rgba{{0.85, 0.12, 0.08, 1}, {0.95, 0.75, 0.1, 1}, {0.15, 0.35, 0.95, 1}}

type modWires struct {
	plugs   [3]*prop
	in      [3]bool
	order   [3]int  // which socket each colour goes to
	wrongT  float64 // a plug was just tried in the wrong socket (seconds the warning shows)
	wrongAt int     // ...which socket
}

// over: q is over socket j (within half the gap to the next socket, no higher than top)
func (m *modWires) over(F xf, j int, q vec3, top float64) bool {
	d := q.sub(m.socket(F, j))
	h := d.dot(F.y)
	return projPerp(d, F.y).length() < 0.034 && h > -0.02 && h < top
}

// taken: a plug is already in socket j
func (m *modWires) taken(j int) bool {
	for c := range m.order {
		if m.order[c] == j && m.in[c] {
			return true
		}
	}
	return false
}

func (m *modWires) start(g *game) {
	for i := range m.plugs {
		i := i
		p := &prop{radius: 0.07, hy: 0.03, cordLen: 0.75}
		p.anchor = func() vec3 { return g.devFrame().p(devHW, devMidY-0.05, -0.15+float64(i)*0.15) }
		p.home = func() (vec3, vec3, vec3, vec3) {
			F := g.devFrame()
			return F.p(devHW+0.05, devMidY-0.08, -0.15+float64(i)*0.15), F.x, F.y, F.z
		}
		p.homeR = 0.0
		p.docked = true
		m.plugs[i] = p
		m.in[i] = false
	}
	m.order = [3]int{0, 1, 2}
	rand.Shuffle(3, func(a, b int) { m.order[a], m.order[b] = m.order[b], m.order[a] })
}

func (m *modWires) socket(F xf, i int) vec3 { return F.p(-0.07+float64(i)*0.07, devTop-0.005, 0.0) }

func (m *modWires) frame(g *game, F xf, dt float64) {
	if m.plugs[0] == nil {
		m.start(g)
	}
	m.wrongT = math.Max(0, m.wrongT-dt)
	n := 0
	for i, p := range m.plugs {
		if m.in[i] {
			n++
			continue
		}
		if p.held || !p.docked {
			p.docked = false
		}
		p.tick(dt)
		s := m.socket(F, m.order[i])
		if p.held {
			// near another colour's socket: it won't go in, and you can tell
			for j := 0; j < 3; j++ {
				if j == m.order[i] || m.taken(j) {
					continue
				}
				if m.over(F, j, p.pos, 0.08) {
					if m.wrongT <= 0 {
						if h := g.handHolding(p); h != nil {
							h.buzz(60, 1)
						}
						ed.sfx(sndGrind, m.socket(F, j), 0.3)
					}
					m.wrongT, m.wrongAt = 0.5, j
				}
			}
		}
		// its own socket pulls the plug in from anywhere over it, up to a hand's width above
		if p.held && m.over(F, m.order[i], p.pos, 0.13) {
			if h := g.handHolding(p); h != nil {
				h.held = nil
			}
			p.held = false
			m.in[i] = true
			ed.sfx(sndClang, s, 0.6)
			spawnSparks(s, F.y, 8, 0.3)
			n++
		}
	}
	g.r.SetModuleProgress(float64(n) / 3)
	if n == 3 {
		g.r.CompleteModule()
	}
}

func (m *modWires) draw(g *game, F xf, current, done bool) {
	if m.plugs[0] == nil {
		m.start(g)
	}
	t := fxTime()
	for i := 0; i < 3; i++ {
		// sockets: coloured rings on the top
		s := m.socket(F, i)
		owner := 0
		for c := range m.order {
			if m.order[c] == i {
				owner = c
			}
		}
		useMat(mCast)
		cyl(&bEnv, s.sub(F.y.mul(0.01)), s.add(F.y.mul(0.006)), 0.02, 0.02, 12, cIron, true)
		col := wireCols[owner]
		if current && !m.in[owner] {
			sprite(s.add(F.y.mul(0.01)), 0.04, col.alpha(float32(0.4+0.3*math.Sin(t*6+float64(i)))))
		}
		ringSoft(s.add(F.y.mul(0.007)), 0.022, 0.005, col.alpha(0.9), 16)
		if current && m.wrongT > 0 && m.wrongAt == i {
			// wrong colour: a red flash, and the colour it wants
			ringSoft(s.add(F.y.mul(0.01)), 0.035, 0.006, cDanger.alpha(float32(math.Min(1, m.wrongT*3))), 20)
			cam := ed.hmd.pos()
			q := s.add(F.y.mul(0.07))
			rr, uu := billboard(cam, q)
			drawTextCentered(&bOpaque, "WRONG COLOUR", q, rr, uu, 0.0012, cDanger.alpha(float32(math.Min(1, m.wrongT*3))))
		}
	}
	for i, p := range m.plugs {
		col := wireCols[i]
		if p.docked && !p.held {
			p.pos, p.X, p.Y, p.Z = p.home() // hanging on the cart, wherever the cart is
		}
		a := p.anchor()
		end := p.pos
		if m.in[i] || done {
			end = m.socket(F, m.order[i]).add(F.y.mul(0.03))
		}
		drawCord(drapeCord(a, end, p.cordLen, 14, 0.007, g.housingBox(F)), 0.007, col.scale(0.7))
		useMat(mPaint)
		X, Y, Z := p.X, p.Y, p.Z
		if m.in[i] || done {
			X, Y, Z = F.x, F.y, F.z
		}
		bevelBox(&bEnv, end, X, Y, Z, 0.014, 0.025, 0.014, 0.004, col)
	}
}

func (m *modWires) grabbers(out []grabber) []grabber {
	for i, p := range m.plugs {
		if p != nil && !m.in[i] {
			out = append(out, p)
		}
	}
	return out
}
func (m *modWires) pokers(out []poker) []poker { return out }

// ---- module: the device wants a game ----

type modGame struct {
	idx  int
	kind string
	open float64
	// simon
	btn   [4]*pushBtn
	seq   []int
	round int
	step  int
	show  float64 // playing the sequence back
	lit   int
	litT  float64
	input bool
	go_   *pushBtn // START: nothing plays until you press it (and it plays the round again)
	ready bool     // START has been pressed
	// needle
	kn      *knob
	needle  float64
	vel     float64
	prog    float64
	failT   float64
	kickT   float64
	lastKn  float64
	phase   float64
	started bool // the needle only starts wandering once you've taken the knob
	// the others (game_minis.go)
	mini mini
}

func (m *modGame) start(g *game) {
	m.kind = g.r.Dev.Mods[m.idx].Game
	m.open = 0
	m.round, m.step, m.input = 0, 0, false
	m.seq = nil
	m.lit, m.litT = -1, 0
	m.show = 0.6
	m.ready = false
	m.go_ = &pushBtn{r: 0.022}
	m.go_.ok = func() bool { return m.kind == "simon" && m.open >= 1 }
	m.go_.onPress = func() {
		// play this round's sequence (again, if you were part way through it)
		m.ready, m.input, m.step, m.show = true, false, 0, 0.5
		ed.sfx(sndRelay, m.go_.c, 0.5)
	}
	for i := range m.btn {
		i := i
		m.btn[i] = &pushBtn{r: 0.035}
		m.btn[i].ok = func() bool { return m.kind == "simon" && m.input }
		m.btn[i].onPress = func() { m.simonPress(g, i) }
	}
	m.kn = &knob{r: 0.03, lo: math.Inf(-1), hi: math.Inf(1), detent: math.Pi / 16}
	m.kn.ok = func() bool { return m.kind == "needle" && m.open > 0.9 }
	m.needle, m.vel, m.prog, m.failT, m.lastKn = 0, 0, 0, 0, 0
	m.kickT, m.phase, m.started = 3, rand.Float64()*6, false
	m.mini = newMini(m.kind, g)
	ed.sfx(sndHydraulic, fv(g.r.Dev.Pos, 1), 0.6)
}

// the compartment is on the cart's left side (-x)
func (m *modGame) face(F xf) (vec3, vec3, vec3) {
	return F.p(-devHW-0.003, devMidY+0.01, 0), F.x.mul(-1), F.z.mul(-1)
}

var simonCols = [4]rgba{{0.1, 0.9, 0.3, 1}, {0.95, 0.15, 0.1, 1}, {0.15, 0.4, 1, 1}, {1, 0.85, 0.1, 1}}
var simonPitch = [4]int{sndTone0, sndTone1, sndTone2, sndTone3}

func (m *modGame) simonPress(g *game, i int) {
	m.lit, m.litT = i, 0.3
	ed.sfx(simonPitch[i], fv(g.r.Dev.Pos, 1), 0.6)
	if i != m.seq[m.step] {
		m.input = false
		g.r.Setback(g.r.T.Setback, "simon")
		return
	}
	m.step++
	if m.step >= len(m.seq) {
		m.round++
		m.input = false
		if m.round >= 3 {
			g.r.CompleteModule()
			return
		}
		m.show = 1.0
		m.seq = append(m.seq, rand.Intn(4))
		m.step = 0
	}
}

func (m *modGame) frame(g *game, F xf, dt float64) {
	if m.btn[0] == nil {
		m.start(g)
	}
	m.open = math.Min(1, m.open+dt*1.5)
	c, n, side := m.face(F)
	up := F.y
	for i, b := range m.btn {
		x := []float64{-1, 1, -1, 1}[i] * 0.06
		y := []float64{1, 1, -1, -1}[i] * 0.05
		b.c, b.n = c.add(side.mul(x)).add(up.mul(y)).add(n.mul(0.03)), n
		b.tick(dt)
	}
	m.kn.c, m.kn.ax = c.add(side.mul(0.09)).add(up.mul(-0.07)).add(n.mul(0.03)), n
	m.go_.c, m.go_.n = c.add(side.mul(0.122)).add(up.mul(-0.085)).add(n.mul(0.03)), n
	m.go_.tick(dt)
	m.litT -= dt
	if m.litT <= 0 {
		m.lit = -1
	}
	if m.mini != nil {
		m.mini.frame(m, g, c, n, side, up, dt)
		return
	}
	switch m.kind {
	case "simon":
		if m.seq == nil {
			m.seq = []int{rand.Intn(4), rand.Intn(4), rand.Intn(4)}
		}
		if !m.input && m.open >= 1 && m.ready {
			m.show -= dt
			if m.show < 0 {
				k := int(-m.show / 0.6)
				if k >= len(m.seq) {
					m.input, m.step = true, 0
				} else if frac := -m.show/0.6 - float64(k); frac < 0.05 {
					if m.lit != m.seq[k] || m.litT < 0.2 {
						m.lit, m.litT = m.seq[k], 0.42
						ed.sfx(simonPitch[m.seq[k]], fv(g.r.Dev.Pos, 1), 0.5)
					}
				}
			}
		}
		g.r.SetModuleProgress((float64(m.round) + float64(m.step)/float64(len(m.seq))) / 3)
	case "needle":
		// it waits for you: nothing moves until you first take hold of the knob
		if !m.started {
			if !m.kn.held {
				m.needle, m.vel, m.lastKn = 0, 0, m.kn.ang
				g.r.SetModuleProgress(m.prog)
				break
			}
			m.started = true
			m.kickT = 3
		}
		// the needle wanders and now and then kicks; turning the knob pushes it back (it spins
		// freely, so let go and take a new grip when your wrist runs out). Keep it in the green.
		m.vel += 0.22*math.Sin(g.t*0.7+m.phase)*dt + rand.NormFloat64()*0.3*math.Sqrt(dt)
		m.kickT -= dt
		if m.kickT <= 0 {
			m.kickT = 2.2 + rand.Float64()*2.8
			m.vel += (rand.Float64()*2 - 1) * 0.9
			ed.sfx(sndTick, m.kn.c, 0.5)
		}
		m.vel *= math.Max(0, 1-dt*0.8)
		m.needle += m.vel*dt + (m.kn.ang-m.lastKn)*0.9
		m.lastKn = m.kn.ang
		if math.Abs(m.needle) < 0.3 {
			m.prog += dt / 14
		}
		if math.Abs(m.needle) > 1 {
			m.failT += dt
			if m.failT > 0.6 {
				g.r.Setback(g.r.T.Setback, "needle")
				return
			}
		} else {
			m.failT = 0
		}
		m.needle = math.Max(-1.2, math.Min(1.2, m.needle))
		g.r.SetModuleProgress(m.prog)
		if m.prog >= 1 {
			g.r.CompleteModule()
		}
	}
}

// gameShown: which game stage's hatch is drawn - both use the same compartment, so the one
// you're on, else the next one to do (else the last).
func gameShown(g *game) int {
	d := &g.r.Dev
	if d.Cur < len(d.Mods) && d.Mods[d.Cur].ID == "game" && d.State == sim.DevHome {
		return d.Cur
	}
	last := -1
	for i, md := range d.Mods {
		if md.ID != "game" {
			continue
		}
		if !md.Done {
			return i
		}
		last = i
	}
	return last
}

func (m *modGame) draw(g *game, F xf, current, done bool) {
	if gameShown(g) != m.idx {
		return
	}
	c, n, side := m.face(F)
	up := F.y
	open := 0.0
	if current {
		open = m.open
	}
	// the lid swings down
	lidAng := open * 1.9
	hinge := c.add(up.mul(-0.12))
	lu := rotAbout(up, side, -lidAng)
	ln := rotAbout(n, side, -lidAng)
	lc := hinge.add(lu.mul(0.12)).add(ln.mul(0.006))
	useMat(mPaint)
	bevelBox(&bEnv, lc, side, lu, ln, 0.15, 0.12, 0.006, 0.003, rgba{0.22, 0.26, 0.28, 1})
	if open < 0.2 {
		label := "GAME"
		if done {
			label = "DONE"
		}
		faceText(&bOpaque, label, lc.add(ln.mul(0.007)), ln, lu, 0.0016, cMuted)
		return
	}
	// inside the compartment
	useMat(mCast)
	if m.mini != nil {
		m.mini.draw(m, g, c, n, side, up)
		return
	}
	switch m.kind {
	case "simon":
		for i, b := range m.btn {
			if b == nil {
				continue
			}
			col := simonCols[i].scale(0.35)
			if m.lit == i {
				col = simonCols[i]
				addPoint(b.c.add(n.mul(0.05)), simonCols[i], 2, 1.5)
				sprite(b.c.add(n.mul(0.01)), 0.09, simonCols[i].alpha(0.5))
			}
			curEmis = 2
			useMat(mPaint)
			bevelBox(&bEnv, b.c.sub(n.mul(0.01+0.006*b.press)), side, up, n, 0.032, 0.032, 0.012, 0.006, col)
			curEmis = 1
		}
		msg := "WATCH"
		if m.input {
			msg = "YOUR TURN"
		} else if !m.ready {
			msg = "PRESS START"
		}
		// START
		gb := m.go_
		gc := cAmber.scale(0.5)
		if !m.ready || m.input {
			gc = cAmber.scale(float32(0.6 + 0.4*math.Sin(fxTime()*5)))
		}
		useMat(mCast)
		cyl(&bEnv, gb.c.sub(n.mul(0.03)), gb.c.sub(n.mul(0.012)), 0.028, 0.028, 16, cIron, true)
		curEmis = 2
		useMat(mPaint)
		cyl(&bEnv, gb.c.sub(n.mul(0.012)), gb.c.sub(n.mul(0.004+0.006*gb.press)), 0.02, 0.02, 16, gc, true)
		curEmis = 1
		faceText(&bOpaque, "START", gb.c.add(up.mul(-0.033)).sub(n.mul(0.01)), n, up, 0.0008, cWhite)
		faceText(&bOpaque, msg+"  "+itoa(m.round+1)+"/3", c.add(up.mul(0.105)).add(n.mul(0.03)), n, up, 0.0012, cAmber)
	case "needle":
		// gauge face
		gc := c.add(up.mul(0.03)).add(n.mul(0.02))
		bTrans.quadC(gc.add(side.mul(-0.1)).add(up.mul(-0.06)), gc.add(side.mul(0.1)).add(up.mul(-0.06)), gc.add(side.mul(0.1)).add(up.mul(0.07)), gc.add(side.mul(-0.1)).add(up.mul(0.07)),
			rgba{0.85, 0.83, 0.75, 1}, rgba{0.85, 0.83, 0.75, 1}, rgba{0.9, 0.88, 0.8, 1}, rgba{0.9, 0.88, 0.8, 1})
		piv := gc.add(up.mul(-0.05)).add(n.mul(0.002))
		for s := -10; s <= 10; s++ {
			a := float64(s) / 10 * 1.0
			d := up.mul(math.Cos(a)).add(side.mul(math.Sin(a)))
			col := rgba{0.1, 0.5, 0.15, 1}
			if math.Abs(float64(s)) > 3 {
				col = rgba{0.2, 0.2, 0.2, 1}
			}
			if math.Abs(float64(s)) > 9 {
				col = rgba{0.7, 0.1, 0.05, 1}
			}
			flatLine(&bOpaque, piv.add(d.mul(0.09)), piv.add(d.mul(0.105)), n, 0.003, col)
		}
		a := m.needle
		nd := up.mul(math.Cos(a)).add(side.mul(math.Sin(a)))
		ribbon(piv, piv.add(nd.mul(0.1)), 0.003, rgba{0.8, 0.1, 0.05, 1})
		// the knob
		kc := m.kn.c
		useMat(mCast)
		cyl(&bEnv, kc.sub(n.mul(0.02)), kc, 0.03, 0.03, 16, cIron, true)
		mk := rotAbout(up, n, m.kn.ang)
		bEnvL.line(kc.add(n.mul(0.001)), kc.add(n.mul(0.001)).add(mk.mul(0.028)), cAmber)
		msg := "HOLD IT IN THE GREEN"
		if !m.started {
			msg = "TAKE THE KNOB TO START"
		}
		faceText(&bOpaque, msg, c.add(up.mul(0.105)).add(n.mul(0.03)), n, up, 0.0011, cAmber)
		// progress bar
		bp := c.add(up.mul(-0.1)).add(n.mul(0.025))
		ribbon(bp.add(side.mul(-0.1)), bp.add(side.mul(-0.1+0.2*m.prog)), 0.01, cEnergy.alpha(0.9))
	}
}

func (m *modGame) grabbers(out []grabber) []grabber {
	if m.mini != nil {
		return m.mini.grabbers(out)
	}
	if m.kind == "needle" && m.kn != nil {
		out = append(out, m.kn)
	}
	return out
}

func (m *modGame) pokers(out []poker) []poker {
	if m.mini != nil {
		return m.mini.pokers(out)
	}
	if m.kind == "simon" {
		for _, b := range m.btn {
			if b != nil {
				out = append(out, b)
			}
		}
		if m.go_ != nil {
			out = append(out, m.go_)
		}
	}
	return out
}

// ---- module: set the three dials ----

type modDials struct {
	kn     [3]*knob
	target [3]float64
	good   float64
}

func (m *modDials) start(g *game) {
	for i := range m.kn {
		m.kn[i] = &knob{r: 0.03, lo: -math.Pi * 0.75, hi: math.Pi * 0.75, detent: math.Pi / 24}
		m.kn[i].ang = (rand.Float64()*2 - 1) * 1.5
		m.target[i] = (rand.Float64()*2 - 1) * 1.8
		for math.Abs(m.target[i]-m.kn[i].ang) < 0.6 {
			m.target[i] = (rand.Float64()*2 - 1) * 1.8
		}
	}
	m.good = 0
}

// the dials are on the front end (+z)
func (m *modDials) at(F xf, i int) vec3 { return F.p(-0.15+float64(i)*0.15, devMidY+0.02, devHL+0.003) }

func (m *modDials) frame(g *game, F xf, dt float64) {
	if m.kn[0] == nil {
		m.start(g)
	}
	ok := 0
	for i, k := range m.kn {
		k.c, k.ax = m.at(F, i).add(F.z.mul(0.025)), F.z
		if math.Abs(k.ang-m.target[i]) < 0.17 {
			ok++
		}
	}
	if ok == 3 {
		m.good += dt
	} else {
		m.good = 0
	}
	g.r.SetModuleProgress(float64(ok)/3*0.9 + 0.1*math.Min(1, m.good))
	if m.good > 1 {
		g.r.CompleteModule()
	}
}

func (m *modDials) draw(g *game, F xf, current, done bool) {
	if m.kn[0] == nil {
		m.start(g)
	}
	for i, k := range m.kn {
		c := m.at(F, i)
		n := F.z
		up := F.y
		side := F.x
		// face with the target notch
		bTrans.quadC(c.add(side.mul(-0.055)).add(up.mul(-0.055)).add(n.mul(0.001)), c.add(side.mul(0.055)).add(up.mul(-0.055)).add(n.mul(0.001)), c.add(side.mul(0.055)).add(up.mul(0.055)).add(n.mul(0.001)), c.add(side.mul(-0.055)).add(up.mul(0.055)).add(n.mul(0.001)),
			rgba{0.1, 0.12, 0.12, 1}, rgba{0.1, 0.12, 0.12, 1}, rgba{0.12, 0.14, 0.14, 1}, rgba{0.12, 0.14, 0.14, 1})
		td := rotAbout(up, n, m.target[i])
		ok := math.Abs(k.ang-m.target[i]) < 0.17
		tc := cAmber
		if ok {
			tc = cEnergy
		}
		if !current && !done {
			tc = cMuted.scale(0.4)
		}
		ribbon(c.add(n.mul(0.002)).add(td.mul(0.038)), c.add(n.mul(0.002)).add(td.mul(0.052)), 0.008, tc)
		useMat(mCast)
		cyl(&bEnv, c, c.add(n.mul(0.025)), 0.03, 0.028, 16, cIron, true)
		pd := rotAbout(up, n, k.ang)
		tip := c.add(n.mul(0.026))
		ribbon(tip, tip.add(pd.mul(0.026)), 0.005, rgba{0.9, 0.9, 0.85, 1})
	}
	if current {
		faceText(&bOpaque, "MATCH THE MARKS", F.p(0, devTop-0.06, devHL+0.004), F.z, F.y, 0.0012, cAmber)
	}
}

func (m *modDials) grabbers(out []grabber) []grabber {
	for _, k := range m.kn {
		if k != nil {
			out = append(out, k)
		}
	}
	return out
}
func (m *modDials) pokers(out []poker) []poker { return out }

// ---- module: charge the coil (it leaks) ----

type modCrank struct {
	k      *crank
	charge float64
	lastT  float64
}

func (m *modCrank) start(g *game) {
	m.k = &crank{R: 0.09}
	m.charge = g.r.Dev.Prog
}

// the crank is on the back end (-z)
func (m *modCrank) frame(g *game, F xf, dt float64) {
	if m.k == nil {
		m.start(g)
	}
	m.k.c, m.k.ax, m.k.ref = F.p(0, devMidY, -devHL-0.01), F.z.mul(-1), F.y
	d := math.Abs(m.k.turns - m.lastT)
	m.lastT = m.k.turns
	m.charge += d / 12
	if !m.k.held {
		m.charge -= dt * 0.04 // it leaks the moment you let go
	} else {
		m.charge -= dt * 0.008
	}
	m.charge = math.Max(0, m.charge)
	g.r.SetModuleProgress(m.charge)
	if m.charge >= 1 {
		g.r.CompleteModule()
	}
}

func (m *modCrank) draw(g *game, F xf, current, done bool) {
	if m.k == nil {
		m.start(g)
	}
	// it rides with the cart whether or not it's the stage you're on
	m.k.c, m.k.ax, m.k.ref = F.p(0, devMidY, -devHL-0.01), F.z.mul(-1), F.y
	g.drawCrank(m.k, 0.09, cBeam)
	// coil gauge beside it
	k := m.charge
	if done {
		k = 1
	}
	base := F.p(0.17, devMidY-0.1, -devHL-0.004)
	ribbon(base, base.add(F.y.mul(0.2)), 0.02, rgba{0.05, 0.05, 0.05, 1})
	col := cBeam
	if current && !m.k.held && k > 0.01 && k < 1 {
		col = cDanger
		sprite(base.add(F.y.mul(0.2*k)), 0.04, cDanger.alpha(0.5))
	}
	ribbon(base, base.add(F.y.mul(0.2*k)), 0.016, col.alpha(0.9))
	if current {
		faceText(&bOpaque, "WIND IT", F.p(0, devTop-0.03, -devHL-0.004), F.z.mul(-1), F.y, 0.0013, cAmber)
	}
}

func (m *modCrank) grabbers(out []grabber) []grabber {
	if m.k != nil {
		out = append(out, m.k)
	}
	return out
}
func (m *modCrank) pokers(out []poker) []poker { return out }

// ---- module: read the serial in the dark slot, key it in ----

type modSerial struct {
	fs    *fastSet // the door's two screws or bolts (whichever the plate hasn't got)
	F     xf
	code  string
	typed string
	keys  []*pushBtn
	err   float64 // a wrong code was just entered (seconds it shows)
	open  float64 // the compartment door (0 shut .. 1 swung open)
	spot  int     // where in there the serial is printed
}

func (m *modSerial) start(g *game) {
	m.code = ""
	for i := 0; i < 4; i++ {
		m.code += string(rune('0' + rand.Intn(10)))
	}
	m.typed = ""
	m.keys = nil
	m.open = 0
	m.fs = nil
	m.ensureFS(g)
	m.spot = rand.Intn(len(serialSpots))
	for i := 0; i < len(keyLabels); i++ {
		ch := keyLabels[i]
		b := &pushBtn{r: 0.016}
		b.onPress = func() { m.key(g, ch) }
		m.keys = append(m.keys, b)
	}
}

// ensureFS: the compartment door's fasteners, on its face near the free edge.
func (m *modSerial) ensureFS(g *game) {
	if m.fs != nil {
		return
	}
	_, kind := g.cartFasteners()
	m.fs = newFastSet(g, kind, 2, func(i int) (vec3, vec3, vec3) {
		D := m.doorFrame(m.F)
		return D.p(0.0045, []float64{-0.055, 0.055}[i], 2*serialHW-0.03), D.x, D.y
	})
}

// keyLabels: the keypad, three across (no ENTER: the right code takes itself)
const keyLabels = "123456789<0"

func (m *modSerial) key(g *game, ch byte) {
	switch ch {
	case '<':
		if len(m.typed) > 0 {
			m.typed = m.typed[:len(m.typed)-1]
		}
		return
	}
	if len(m.typed) >= len(m.code) {
		m.typed = "" // starting over after a wrong code
	}
	m.err = 0
	m.typed += string(ch)
	if len(m.typed) < len(m.code) {
		return
	}
	if m.typed == m.code {
		ed.sfx(sndRelay, fv(g.r.Dev.Pos, 1), 0.6)
		g.r.CompleteModule()
		return
	}
	m.err = 1.2
	ed.sfx(sndAlarm, fv(g.r.Dev.Pos, 1), 0.4)
}

// the serial compartment: a dark recess in the right side (+x) behind a hinged door
const (
	serialZ     = -0.12 // its middle, along the cart
	serialY     = 0.885
	serialHW    = 0.1  // half the opening's width (along the cart)
	serialHH    = 0.09 // ...and height
	serialDepth = 0.17 // how deep it goes in
)

// where the serial can be printed, in the compartment's frame (x out, y up, z along)
var serialSpots = []struct {
	id      string
	p, n, u vec3
}{
	// each a label stuck just proud of its surface, so nothing in there buries it
	{"board", v3(-serialDepth+0.01, -0.03, -0.02), v3(1, 0, 0), v3(0, 1, 0)}, // the circuit board, under the chips
	{"cap", v3(-0.07+0.025, -0.045, 0.068), v3(1, 0, 0), v3(0, 1, 0)},        // the big capacitor's front
	{"wall", v3(-0.09, 0.02, -serialHW+0.004), v3(0, 0, 1), v3(0, 1, 0)},     // the side wall, in the shadow
	{"floor", v3(-0.06, -serialHH+0.003, -0.02), v3(0, 1, 0), v3(-1, 0, 0)},  // the floor, under the wiring
	{"door", v3(-0.007, 0, serialHW-0.03), v3(-1, 0, 0), v3(0, 1, 0)},        // the inside of the door
}

// slot: the middle of the compartment's opening; keypad beside it
func (m *modSerial) slot(F xf) vec3 { return F.p(devHW, serialY, serialZ) }

// cf: the compartment's frame (origin at the opening's middle, x out of the side)
func cf(F xf) xf { return xf{F.p(devHW, serialY, serialZ), F.x, F.y, F.z} }
func (m *modSerial) keyAt(F xf, i int) vec3 {
	col, row := i%3, i/3
	return F.p(devHW+0.004, devMidY+0.09-float64(row)*0.04, 0.05+float64(col)*0.045)
}

func (m *modSerial) frame(g *game, F xf, dt float64) {
	if m.keys == nil {
		m.start(g)
	}
	for i, b := range m.keys {
		b.c, b.n = m.keyAt(F, i).add(F.x.mul(0.01)), F.x
		b.tick(dt)
	}
	m.F = F
	m.ensureFS(g)
	if !m.fs.allOut() {
		// the door's held shut until its screws (or bolts) are out
		m.open = 0
		m.fs.frame(dt)
		g.r.SetModuleProgress(0.35 * m.fs.progress())
		return
	}
	if m.open == 0 {
		ed.sfx(sndHydraulic, m.slot(F), 0.5)
	}
	m.open = math.Min(1, m.open+dt*1.2)
	if m.err > 0 {
		if m.err -= dt; m.err <= 0 && len(m.typed) >= len(m.code) {
			m.typed = "" // the wrong code clears itself
		}
	}
	g.r.SetModuleProgress(0.35 + 0.65*float64(len(m.typed))/5)
}

// serialAt: where the serial is printed (world), its facing and its up
func (m *modSerial) serialAt(F xf) (vec3, vec3, vec3) {
	C := cf(F)
	sp := serialSpots[m.spot]
	if sp.id == "door" {
		D := m.doorFrame(F)
		return D.p(sp.p.x, sp.p.y, sp.p.z), D.x.mul(-1), D.y
	}
	w := func(v vec3) vec3 { return C.x.mul(v.x).add(C.y.mul(v.y)).add(C.z.mul(v.z)) }
	return C.p(sp.p.x, sp.p.y, sp.p.z), w(sp.n), w(sp.u)
}

// doorFrame: the door's own frame, swung about its hinge on the back edge of the opening
func (m *modSerial) doorFrame(F xf) xf {
	C := cf(F)
	ang := smooth(m.open) * 1.85
	hinge := C.p(0.004, 0, -serialHW)
	along := rotAbout(C.z, C.y, -ang) // the door's width, from the hinge
	out := rotAbout(C.x, C.y, -ang)
	return xf{hinge, out, C.y, along}
}

func (m *modSerial) draw(g *game, F xf, current, done bool) {
	if m.keys == nil {
		m.start(g)
	}
	n, up, along := F.x, F.y, F.z
	C := cf(F)
	dark := rgba{0.035, 0.035, 0.04, 1}
	at := func(x, y, z float64) vec3 { return C.p(x, y, z) }
	// the recess: dark walls, floor, ceiling and back
	useMat(mCast)
	bbox(&bEnv, C, -serialDepth-0.005, 0, 0, 0.005, serialHH, serialHW, 0.001, dark)
	for _, sy := range []float64{-1, 1} {
		bbox(&bEnv, C, -serialDepth/2, sy*(serialHH+0.003), 0, serialDepth/2, 0.003, serialHW, 0.001, dark)
	}
	for _, sz := range []float64{-1, 1} {
		bbox(&bEnv, C, -serialDepth/2, 0, sz*(serialHW+0.003), serialDepth/2, serialHH, 0.003, 0.001, dark)
	}
	// what's in it: a circuit board on the back, chips, capacitors, a relay, and wiring
	useMat(mCircuit)
	bbox(&bEnv, C, -serialDepth+0.003, 0.02, -0.02, 0.002, 0.06, 0.07, 0.001, rgba{0.05, 0.22, 0.1, 1})
	useMat(mPaint)
	for i, q := range [][2]float64{{0.035, -0.045}, {0.035, 0.01}, {-0.01, -0.03}, {-0.01, 0.03}} {
		bbox(&bEnv, C, -serialDepth+0.007, q[0], q[1], 0.003, 0.012, 0.016+0.004*float64(i%2), 0.001, rgba{0.04, 0.04, 0.045, 1})
	}
	useMat(mSteel)
	cyl(&bEnv, at(-0.07, -serialHH, 0.068), at(-0.07, -serialHH+0.09, 0.068), 0.022, 0.022, 14, rgba{0.15, 0.25, 0.55, 1}, true)
	cyl(&bEnv, at(-0.11, -serialHH, 0.04), at(-0.11, -serialHH+0.06, 0.04), 0.014, 0.014, 12, rgba{0.5, 0.42, 0.12, 1}, true)
	cyl(&bEnv, at(-0.04, -serialHH, -0.07), at(-0.04, -serialHH+0.05, -0.07), 0.012, 0.012, 12, rgba{0.12, 0.12, 0.13, 1}, true)
	useMat(mPaint)
	bbox(&bEnv, C, -0.12, -0.05, -0.055, 0.025, 0.025, 0.02, 0.003, rgba{0.55, 0.12, 0.08, 1}) // a relay
	wires := []struct {
		a, b vec3
		col  rgba
	}{
		{at(-serialDepth+0.01, 0.07, -0.08), at(-0.02, -serialHH+0.01, 0.05), rgba{0.8, 0.1, 0.05, 1}},
		{at(-serialDepth+0.01, 0.06, 0.07), at(-0.03, -serialHH+0.01, -0.06), rgba{0.9, 0.75, 0.1, 1}},
		{at(-0.13, 0.08, 0.0), at(-0.05, -serialHH+0.01, -0.01), rgba{0.15, 0.35, 0.9, 1}},
		{at(-serialDepth+0.01, -0.02, -0.09), at(-0.02, 0.05, 0.09), rgba{0.12, 0.6, 0.15, 1}},
		{at(-0.1, 0.085, -0.06), at(-0.12, -serialHH+0.01, 0.08), rgba{0.08, 0.08, 0.08, 1}},
	}
	for _, w := range wires {
		drawCord(cordPts(w.a, w.b, w.a.sub(w.b).length()*1.25, 8), 0.0035, w.col)
	}
	// the door: screwed (or bolted) shut until this stage, then it swings open on its hinge
	m.F = F
	m.ensureFS(g)
	if !current && !done {
		m.open = 0
	} else if done {
		m.open = 1
	}
	if m.open < 0.01 {
		m.fs.draw(current)
	}
	D := m.doorFrame(F)
	useMat(mPaint)
	bbox(&bEnv, D, 0, 0, serialHW, 0.004, serialHH+0.006, serialHW+0.006, 0.002, rgba{0.2, 0.24, 0.27, 1})
	useMat(mSteel)
	cyl(&bEnv, D.p(0.012, -0.03, 2*serialHW-0.02), D.p(0.012, 0.03, 2*serialHW-0.02), 0.006, 0.006, 8, cSteelEnv, true)
	for _, y := range []float64{-0.07, 0.07} {
		cyl(&bEnv, D.p(0, y-0.012, 0), D.p(0, y+0.012, 0), 0.006, 0.006, 8, cSteelEnv, true)
	}
	if m.open < 0.05 {
		faceText(&bOpaque, "MAINT.", D.p(0.0045, 0.0, serialHW), D.x, D.y, 0.0012, cMuted)
	}
	if current {
		// the serial: somewhere in there, too dark to read without the flashlight
		sp, sn, su := m.serialAt(F)
		lit := g.flashOn(sp)
		// the label: a scrap of paper you can just make out in the dark; the number on it
		// only reads in the flashlight's beam
		sr := su.cross(sn).norm()
		pc := rgba{0.5, 0.48, 0.42, 1}.scale(float32(0.25 + 0.75*lit))
		lw, lh := 0.026, 0.011
		useMat(mPaint)
		bEnv.quadF(sp.sub(sr.mul(lw)).sub(su.mul(lh)), sp.add(sr.mul(lw)).sub(su.mul(lh)), sp.add(sr.mul(lw)).add(su.mul(lh)), sp.sub(sr.mul(lw)).add(su.mul(lh)), sn, pc)
		if lit > 0.05 && m.open > 0.7 {
			faceText(&bOpaque, m.code, sp.add(sn.mul(0.0008)), sn, su, 0.0016, rgba{0.08, 0.07, 0.06, 1})
			addPoint(sp.add(sn.mul(0.05)), rgba{1, 0.95, 0.85, 1}, 0.4*lit, 0.4)
		}
		faceText(&bOpaque, "FIND THE SERIAL (USE THE LIGHT)", at(0.002, -serialHH-0.02, 0), n, up, 0.0008, cMuted)
	}
	// keypad
	labels := keyLabels
	for i, b := range m.keys {
		c := m.keyAt(F, i)
		col := rgba{0.25, 0.27, 0.28, 1}
		if current {
			col = rgba{0.35, 0.38, 0.36, 1}
		}
		useMat(mPaint)
		bevelBox(&bEnv, c.sub(n.mul(0.004*b.press)), along, up, n, 0.017, 0.015, 0.008, 0.004, col)
		faceText(&bOpaque, string(labels[i]), c.add(n.mul(0.0085-0.004*b.press)), n, up, 0.0012, cWhite)
	}
	// display
	dc := F.p(devHW+0.002, devMidY+0.13, 0.095)
	txt := m.typed
	for len(txt) < 4 {
		txt += "-"
	}
	tc := cEnergy
	if m.err > 0 {
		tc, txt = cDanger, "WRONG"
	}
	if current || done {
		faceText(&bOpaque, txt, dc.add(n.mul(0.002)), n, up, 0.0018, tc)
	}
}

func (m *modSerial) grabbers(out []grabber) []grabber {
	if m.fs == nil {
		return out
	}
	return m.fs.grabbers(out)
}
func (m *modSerial) pokers(out []poker) []poker {
	for _, b := range m.keys {
		out = append(out, b)
	}
	return out
}

// ---- module: pull the cord ----

// The last stage: a pull-start gas engine and a big red button. Pull the cord hard: the
// first two pulls turn it over but it doesn't catch; the third, it fires up, shakes the cart
// and revs - and once it's up to speed the big red button lights. Push it to set the EMP off.
// (A misfire kills the engine; once it's fixed it's warm and starts on the first pull.)
type modRip struct {
	c       *pullCord
	btn     *bigButton
	good    int     // hard pulls since it was last off
	need    int     // hard pulls it takes to catch (three cold, one warm)
	crankT  float64 // turning over without catching: how long left
	running bool
	revT    float64 // how long it's been running
	spin    float64
	puffT   float64
}

// engine timings
const (
	ripCrank = 0.9 // a pull that doesn't catch turns it over this long
	ripRev   = 4.0 // running this long before the button lights
)

func (m *modRip) btnAt(F xf) vec3 { return F.p(-0.09, devTop+0.032, 0.2) }

// lit: the button's live.
func (m *modRip) lit() bool { return m.running && m.revT >= ripRev }

func (m *modRip) start(g *game) {
	m.c = &pullCord{maxExt: 0.55}
	m.good, m.need, m.running, m.revT, m.crankT = 0, 3, false, 0, 0
	m.btn = &bigButton{}
	m.btn.onPress = func() { m.push(g) }
	m.c.onPull = func(speed float64) {
		p := fv(g.r.Dev.Pos, 1.1)
		if g.r.Dev.Misfire {
			ed.sfx(sndCreak, p, 0.6)
			g.say("MISFIRE", "FIX THE GUTS FIRST - THE RADIO SAYS HOW", cDanger, 2.5)
			return
		}
		if m.running {
			ed.sfx(sndCreak, p, 0.4)
			return // it's going already
		}
		if speed <= 1.3 {
			ed.sfx(sndCreak, p, 0.6)
			return
		}
		m.good++
		m.spin = 1
		ed.sfx(sndGrind, p, 0.5)
		if m.good >= m.need {
			// it catches
			m.running, m.revT, m.crankT = true, 0, 0
			ed.sfx(sndIgniteID, p, 0.9)
			spawnSmoke(m.exhaust(g.devFrame()), 6)
			return
		}
		m.crankT = ripCrank // turns over... and doesn't catch
	}
}

// bigButton: a mushroom button you can't miss: a fingertip or a palm coming down onto its
// cap (anywhere on it, or just past its rim) pushes it.
type bigButton struct {
	base, up vec3
	onPress  func()
	press    float64 // animation 1 -> 0
	cool     float64
	was      [2]bool // each hand's palm was on it last frame
}

const (
	bigTop = 0.066 // the cap's top above the base
	bigR   = 0.07  // how far out from its middle a hand still pushes it
)

func (b *bigButton) top() vec3 { return b.base.add(b.up.mul(bigTop)) }

// on: whether p is down on the cap - within its reach and no higher than just over it.
func (b *bigButton) on(p vec3) bool {
	d := p.sub(b.base)
	h := d.dot(b.up)
	return projPerp(d, b.up).length() < bigR && h < bigTop+0.012 && h > -0.03
}

func (b *bigButton) hit() {
	if b.cool > 0 {
		return
	}
	b.press, b.cool = 1, 0.5
	ed.sfx(sndClang, b.top(), 0.5)
	if b.onPress != nil {
		b.onPress()
	}
}

func (b *bigButton) pokeOK() bool { return b.cool <= 0 }
func (b *bigButton) pokeTest(prev, tip vec3) bool {
	if b.on(tip) && !b.on(prev) {
		b.hit()
		return true
	}
	return false
}

// slap: a palm (an open hand or a fist) coming down on it.
func (b *bigButton) slap() {
	for i, h := range hands {
		if i > 1 || h == nil || !h.valid() {
			continue
		}
		on := b.on(h.palm.sub(b.up.mul(0.025))) // the underside of the hand, not its middle
		if on && !b.was[i] && h.held == nil {
			b.hit()
			h.buzz(60, 1)
		}
		b.was[i] = on
	}
}

func (b *bigButton) tick(dt float64) {
	b.press = math.Max(0, b.press-dt*4)
	b.cool -= dt
}

// push: the big red button.
func (m *modRip) push(g *game) {
	p := m.btnAt(g.devFrame())
	if !m.lit() {
		ed.sfx(sndTick, p, 0.5)
		return
	}
	if g.r.Dev.Leads < sim.LeadsNeeded {
		g.r.CompleteModule() // (it says why not)
		return
	}
	g.r.CompleteModule()
	if g.r.Dev.Misfire {
		// it coughs and dies
		m.running, m.revT, m.good, m.need = false, 0, 0, 1
		ed.sfx(sndPowerDown, p, 0.8)
		spawnSmoke(m.exhaust(g.devFrame()), 10)
	}
}

// exhaust: where the engine's pipe comes out (low on the cart's left side, toward the back).
func (m *modRip) exhaust(F xf) vec3 { return F.p(-devHW-0.06, devMidY-0.11, -0.2) }

// rpm: how fast it's turning now (0: not at all).
func (m *modRip) rpm() float64 {
	switch {
	case m.running && m.revT < 0.6:
		return 1100 + 1500*m.revT // catching
	case m.running && m.revT < ripRev:
		u := (m.revT - 0.6) / (ripRev - 0.6)
		return 1000 + 3400*math.Sin(math.Pi*u)*(1-0.3*u) + 500*u // revving
	case m.running:
		return 1500 // a fast idle, waiting for you
	case m.crankT > 0:
		u := 1 - m.crankT/ripCrank
		return 300 + 700*math.Sin(math.Pi*math.Min(1, u*1.4)) // turns over, sputters out
	}
	return 0
}

func (m *modRip) frame(g *game, F xf, dt float64) {
	if m.c == nil {
		m.start(g)
	}
	m.c.a, m.c.dir = F.p(0.12, devTop+0.02, 0.18), F.y
	m.c.tick(dt)
	m.btn.base, m.btn.up = m.btnAt(F), F.y
	m.btn.tick(dt)
	m.btn.slap()
	m.spin = math.Max(0, m.spin-dt*0.7)
	m.crankT = math.Max(0, m.crankT-dt)
	if m.running {
		was := m.lit()
		m.revT += dt
		if m.lit() && !was {
			ed.sfx(sndPowerUp, m.btnAt(F), 0.8)
			ed.sfx(sndBeep, m.btnAt(F), 0.6)
		}
		// exhaust
		if m.puffT -= dt; m.puffT <= 0 {
			m.puffT = 0.25 - 0.18*math.Min(1, m.rpm()/4000)
			spawnSmoke(m.exhaust(F), 1)
		}
		// the cart shaking under your hands
		for _, h := range hands {
			if h != nil && h.held != nil && (h.held == g.devVis.handles[0] || h.held == g.devVis.handles[1]) {
				h.buzz(15, float32(0.15+0.25*math.Min(1, m.rpm()/4000)))
			}
		}
	}
	k := float64(m.good) / float64(m.need+1)
	if m.running {
		k = 0.6 + 0.35*math.Min(1, m.revT/ripRev)
	}
	g.r.SetModuleProgress(math.Min(0.95, k))
}

// engineFrame: the engine's sound (and whether it's still going), whatever stage it's on.
func (g *game) engineFrame() {
	var m *modRip
	for _, dm := range g.devVis.mods {
		if rm, ok := dm.(*modRip); ok {
			m = rm
		}
	}
	if m == nil {
		return
	}
	d := &g.r.Dev
	if m.running && (d.State != sim.DevHome || g.r.Phase != sim.Playing) {
		// they've got the cart (or it's over): it stalls
		m.running, m.revT, m.good, m.need = false, 0, 0, 1
	}
	rpm := m.rpm()
	if rpm <= 0 || g.r.Phase != sim.Playing {
		eng.motorSet(0, 0, 0)
		return
	}
	a, pan := ed.panAt(fv(d.Pos, 0.8))
	vol := 0.55
	if !m.running {
		vol = 0.4 * math.Min(1, m.crankT/0.25) // sputtering out
	}
	eng.motorSet(vol*math.Max(a, 0.3), rpm, pan)
}

// shake: how much the engine's shaking the cart right now (m).
func (g *game) engineShake() float64 {
	for _, dm := range g.devVis.mods {
		if rm, ok := dm.(*modRip); ok && (rm.running || rm.crankT > 0) {
			return 0.0012 + 0.0035*math.Min(1, rm.rpm()/4000)
		}
	}
	return 0
}

func (m *modRip) draw(g *game, F xf, current, done bool) {
	if m.c == nil {
		m.start(g)
	}
	t := fxTime()
	m.c.a, m.c.dir = F.p(0.12, devTop+0.02, 0.18), F.y
	a := m.c.a
	useMat(mCast)
	cyl(&bEnv, a.sub(F.y.mul(0.02)), a, 0.03, 0.025, 14, cIron, true)
	h := m.c.handle()
	ribbon(a, h, 0.004, rgba{0.85, 0.8, 0.6, 1})
	useMat(mPaint)
	cyl(&bEnv, h.sub(F.x.mul(0.05)), h.add(F.x.mul(0.05)), 0.014, 0.014, 10, cDanger, true)
	// the exhaust pipe
	ex := m.exhaust(F)
	useMat(mSteel)
	cyl(&bEnv, F.p(-devHW, devMidY-0.11, -0.2), ex, 0.014, 0.016, 10, cSteelEnv.scale(0.6), true)
	// the big red button
	b := m.btnAt(F)
	useMat(mHazard)
	cyl(&bEnv, b.sub(F.y.mul(0.012)), b.add(F.y.mul(0.006)), 0.05, 0.05, 20, rgba{0.8, 0.6, 0.1, 1}, true)
	cap := rgba{0.35, 0.03, 0.02, 1}
	if m.lit() {
		cap = cDanger
		curEmis = 2.5 + 1.5*float32(0.5+0.5*math.Sin(t*6))
	}
	useMat(mPaint)
	top := b.add(F.y.mul(0.03 - 0.008*m.btn.press))
	cyl(&bEnv, b.add(F.y.mul(0.006)), top, 0.026, 0.026, 18, cap.scale(0.8), false)
	hemi(&bEnv, top, F.y, 0.036, cap, 4, 18)
	curEmis = 1
	if m.lit() {
		sprite(top.add(F.y.mul(0.02)), 0.12, cDanger.alpha(float32(0.35+0.25*math.Sin(t*6))))
		addPoint(top.add(F.y.mul(0.05)), cDanger, 1.2, 1.5)
	}
	if !current {
		return
	}
	sprite(h, 0.06, cDanger.alpha(float32(0.3+0.3*math.Sin(t*7))))
	rr, uu := billboard(ed.hmd.pos(), a.add(F.y.mul(0.12)))
	msg, mc := "PULL HARD TO START IT", cAmber
	switch {
	case g.r.Dev.Misfire:
		msg, mc = "MISFIRE - FIX IT FIRST", cDanger
	case m.lit():
		msg, mc = "", cDanger
	case m.running:
		msg, mc = "WARMING UP...", cEnergy
	case m.crankT > 0:
		msg = "IT DIDN'T CATCH - AGAIN"
	}
	if msg != "" {
		drawTextCentered(&bOpaque, msg, a.add(F.y.mul(0.12)), rr, uu, 0.0012, mc)
	}
	if m.lit() {
		q := top.add(F.y.mul(0.08))
		r2, u2 := billboard(ed.hmd.pos(), q)
		drawTextCentered(&bOpaque, "PUSH", q, r2, u2, 0.0016, cDanger)
	}
	if m.spin > 0 {
		sprite(F.p(0, devTop+0.1, 0), 0.5*m.spin, cBeam.alpha(float32(0.3*m.spin)))
	}
}

func (m *modRip) grabbers(out []grabber) []grabber {
	if m.c != nil {
		out = append(out, m.c)
	}
	return out
}
func (m *modRip) pokers(out []poker) []poker {
	if m.btn != nil {
		out = append(out, m.btn)
	}
	return out
}
