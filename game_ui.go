package main

// Panels (the setup console, the results board, the pause menu), the radio with its
// handset, and the per-frame order everything runs in.

import (
	"boundaryvr/sim"
	"fmt"
	"log"
	"math"
	"strings"
)

type uiBtn struct {
	label  func() string
	x, y   float64 // centre, panel coordinates (metres from the panel centre)
	w, h   float64
	act    func()
	on     func() bool
	accent rgba
	poke   *pushBtn
}

type uiPanel struct {
	pos          vec3
	right, up, n vec3 // n faces the viewer
	w, h         float64
	lines        func() []string
	btns         []*uiBtn
	show         func() bool
	hot          int
}

func (p *uiPanel) visible() bool { return p.show == nil || p.show() }

func (p *uiPanel) pt(x, y, z float64) vec3 {
	return p.pos.add(p.right.mul(x)).add(p.up.mul(y)).add(p.n.mul(z))
}

func (p *uiPanel) ray(o, d vec3) (float64, int, bool) {
	if !p.visible() {
		return 0, -1, false
	}
	den := d.dot(p.n)
	if math.Abs(den) < 1e-5 {
		return 0, -1, false
	}
	t := p.pos.sub(o).dot(p.n) / den
	if t <= 0 || t > 8 {
		return 0, -1, false
	}
	q := o.add(d.mul(t)).sub(p.pos)
	x, y := q.dot(p.right), q.dot(p.up)
	if math.Abs(x) > p.w/2 || math.Abs(y) > p.h/2 {
		return 0, -1, false
	}
	for i, b := range p.btns {
		if math.Abs(x-b.x) <= b.w/2 && math.Abs(y-b.y) <= b.h/2 {
			return t, i, true
		}
	}
	return t, -1, true
}

func (p *uiPanel) press(i int) {
	if i >= 0 && i < len(p.btns) && p.btns[i].act != nil {
		audioClick()
		p.btns[i].act()
	}
}

// face: turn the panel toward the viewer (horizontally)
func (p *uiPanel) face(cam vec3) {
	n := cam.sub(p.pos)
	n.y = 0
	if n.length() < 1e-3 {
		return
	}
	p.n = n.norm()
	p.up = v3(0, 1, 0)
	p.right = p.up.cross(p.n).norm()
}

func (p *uiPanel) draw(hot map[*uiPanel]int) {
	if !p.visible() {
		return
	}
	t := fxTime()
	w2, h2 := p.w/2, p.h/2
	useMat(mCast)
	bevelBox(&bEnv, p.pt(0, 0, -0.012), p.right, p.up, p.n, w2+0.015, h2+0.015, 0.008, 0.005, cFrame)
	glassT := rgba{0.03, 0.07, 0.09, 0.92}
	glassB := rgba{0.015, 0.03, 0.04, 0.94}
	bTrans.quadC(p.pt(-w2, -h2, -0.003), p.pt(w2, -h2, -0.003), p.pt(w2, h2, -0.003), p.pt(-w2, h2, -0.003), glassB, glassB, glassT, glassT)
	for _, e := range [][2][2]float64{{{-w2, -h2}, {w2, -h2}}, {{w2, -h2}, {w2, h2}}, {{w2, h2}, {-w2, h2}}, {{-w2, h2}, {-w2, -h2}}} {
		glowLine(p.pt(e[0][0], e[0][1], 0.001), p.pt(e[1][0], e[1][1], 0.001), cBeam.alpha(0.45), 0.3)
	}
	sweep := -h2 + math.Mod(t*0.12, 1)*2*h2
	ribbon(p.pt(-w2, sweep, 0), p.pt(w2, sweep, 0), 0.02, cBeam.alpha(0.05))
	if p.lines != nil {
		for i, l := range p.lines() {
			px, col := 0.0018, cWhite
			switch {
			case i == 0:
				px, col = 0.0034, cEnergy
			case strings.HasPrefix(l, "!"):
				l, col = l[1:], cAmber
			case strings.HasPrefix(l, "~"):
				l, col = l[1:], cMuted
			}
			drawText(&bOpaque, l, p.pt(-w2+0.025, h2-0.04-float64(i)*0.032-boolF(i > 0)*0.015, 0.002), p.right, p.up, px, col)
		}
	}
	for i, b := range p.btns {
		hotB := hot[p] == i
		col := rgba{0.12, 0.15, 0.17, 1}
		if b.on != nil && b.on() {
			col = rgba{0.08, 0.2, 0.14, 1}
		}
		if hotB {
			col = rgba{0.16, 0.26, 0.22, 1}
		}
		z := 0.004
		if b.poke != nil {
			z -= 0.004 * b.poke.press
		}
		useMat(mPaint)
		bevelBox(&bEnv, p.pt(b.x, b.y, z), p.right, p.up, p.n, b.w/2, b.h/2, 0.004, 0.003, col)
		ac := b.accent
		if ac.a == 0 {
			ac = cBeam
		}
		x0, x1, y0, y1 := b.x-b.w/2, b.x+b.w/2, b.y-b.h/2, b.y+b.h/2
		lc := ac.alpha(0.25)
		if hotB {
			lc = ac.alpha(float32(0.7 + 0.3*math.Sin(t*8)))
		}
		for _, e := range [][2][2]float64{{{x0, y0}, {x1, y0}}, {{x1, y0}, {x1, y1}}, {{x1, y1}, {x0, y1}}, {{x0, y1}, {x0, y0}}} {
			glowLine(p.pt(e[0][0], e[0][1], z+0.0045), p.pt(e[1][0], e[1][1], z+0.0045), lc, 0)
		}
		lbl := b.label()
		px := math.Min(0.0026, (b.w-0.02)/float64(textWidth(lbl)))
		drawTextCentered(&bOpaque, lbl, p.pt(b.x, b.y, z+0.0048), p.right, p.up, px, cWhite)
	}
}

// pokers for every button of every visible panel
func (p *uiPanel) pokeList(out []poker) []poker {
	if !p.visible() {
		return out
	}
	for _, b := range p.btns {
		if b.poke == nil {
			b := b
			b.poke = &pushBtn{onPress: func() { audioClick(); b.act() }}
		}
		b.poke.c = p.pt(b.x, b.y, 0.008)
		b.poke.n = p.n
		b.poke.r = math.Min(b.w, b.h) / 2
		out = append(out, b.poke)
	}
	return out
}

// ---- the setup console ----

func (g *game) buildConsole() {
	r := g.r
	c := g.consoleSpot()
	p := &uiPanel{pos: fv(c, 1.3), w: 0.62, h: 0.6, n: v3(0, 0, 1), up: v3(0, 1, 0), right: v3(1, 0, 0)}
	// it's furniture: it faces the middle of your room and stays put
	if mid := sim.Centroid(r.RingPoly); sim.Dist(mid, c) > 0.1 {
		p.face(fv(mid, 1.3))
	}
	p.show = func() bool { return r.Phase == sim.Setup }
	p.lines = func() []string {
		lo, hi := r.GenLo, r.GenHi
		green := r.GreenCostFor(r.Machines())
		room := fmt.Sprintf("YOUR ROOM  %.0f SQ FT, %s", r.Area*sim.SqFtPerM2, roomSourceName())
		if roomSource == "rectangle" || roomSource == "default" {
			room = "!" + room
		}
		return []string{
			"CONTRABAND",
			room,
			fmt.Sprintf("GENERATORS  %d  (THIS ROOM TAKES %d TO %d)", r.Machines(), lo, hi),
			fmt.Sprintf("~GENERATORS AND RING COST %.0f PTS", green),
			fmt.Sprintf("!%.0f POINTS LEFT FOR YOUR MAZE", r.Points),
			"~DRAW MAGENTA WALLS OUT IN THE HALL:",
			"~HALF-PULL THE TRIGGER TO AIM, PULL TO DRAW.",
			"~HOLD THE TRIGGER ON A MAGENTA WALL TO TAKE IT DOWN.",
			"~THE AMBER BUTTON SETS HOW HARD IT GETS.",
			"!READY? PULL THE LIGHTS-OUT LEVER.",
		}
	}
	setN := func(d int) func() {
		return func() {
			if r.SetGens(r.Machines() + d) {
				g.syncGens()
				g.buildPanels()
				ed.sfx(sndTelescopeID, p.pos, 0.5)
			} else {
				ed.sfx(sndAlarm, p.pos, 0.2)
			}
		}
	}
	p.btns = []*uiBtn{
		{label: func() string { return "FEWER" }, x: -0.2, y: -0.18, w: 0.17, h: 0.06, act: setN(-1)},
		{label: func() string { return "MORE" }, x: -0.01, y: -0.18, w: 0.17, h: 0.06, act: setN(1)},
		{label: func() string { return difficultyNames[difficulty] }, x: 0.2, y: -0.18, w: 0.17, h: 0.06, accent: cAmber, act: func() {
			difficulty = (difficulty + 1) % 3
			pr := tuningFor().Pressure
			r.T.Pressure = pr
			ed.sfx(sndRelay, p.pos, 0.5)
		}},
		{label: func() string {
			if g.trace.on {
				return "CANCEL"
			}
			return "TRACE ROOM"
		}, x: -0.13, y: -0.26, w: 0.25, h: 0.06, accent: cBeam, act: func() { g.traceToggle() }},
		{label: func() string {
			if canLeave {
				return "USE BOUNDARY"
			}
			return "HEADSET ROOM"
		}, x: 0.13, y: -0.26, w: 0.25, h: 0.06, act: func() {
			if roomSource == "traced" && onHeadsetRoom != nil {
				onHeadsetRoom()
			} else {
				ed.sfx(sndAlarm, p.pos, 0.2)
			}
		}},
	}
	g.console = p
	// the lights-out lever beside the glass: grab it, haul it toward you and down
	sw := &lever{piv: p.pt(p.w/2+0.14, -0.1, 0.06), ax: p.right, d0: p.up, L: 0.26, a0: -0.15, a1: 2.0, ang: -0.15, atEnd: 0}
	sw.ok = func() bool { return r.Phase == sim.Setup }
	sw.onEnd = func(end int) {
		if end == 1 {
			g.startRound()
		}
	}
	g.lightsOut = sw
}

// drawLightsOut: a wall-style power lever on its own plate beside the console glass.
func (g *game) drawLightsOut() {
	p, l := g.console, g.lightsOut
	if p == nil || l == nil {
		return
	}
	t := fxTime()
	R, U, N := p.right, p.up, p.n
	at := func(x, y, z float64) vec3 { return l.piv.add(R.mul(x)).add(U.mul(y)).add(N.mul(z)) }
	useMat(mPaint)
	bevelBox(&bEnv, at(0, 0.02, -0.06), R, U, N, 0.1, 0.22, 0.01, 0.004, rgba{0.2, 0.21, 0.2, 1})
	useMat(mCast)
	bevelBox(&bEnv, at(-0.115, 0, -0.06), R, U, N, 0.02, 0.02, 0.008, 0.003, cIron) // strut to the console
	useMat(mHazard)
	bevelBox(&bEnv, at(0, -0.185, -0.049), R, U, N, 0.1, 0.02, 0.002, 0.001, rgba{0.8, 0.6, 0.1, 1})
	// the ears that hold the pivot pin
	useMat(mCast)
	for _, sx := range []float64{-1, 1} {
		bevelBox(&bEnv, at(sx*0.045, 0, -0.025), R, U, N, 0.008, 0.045, 0.035, 0.003, cIron)
	}
	cyl(&bEnv, at(-0.056, 0, 0), at(0.056, 0, 0), 0.012, 0.012, 10, cSteelEnv, true)
	// how far it has been pulled, on the outer face of the left ear
	for k := 0; k < 16; k++ {
		a0 := l.a0 + (l.a1-l.a0)*float64(k)/16
		a1 := l.a0 + (l.a1-l.a0)*float64(k+1)/16
		c := cMuted.alpha(0.5)
		if a1 <= l.ang+0.01 {
			c = cDanger
		}
		q0 := l.piv.add(rotAbout(l.d0, l.ax, a0).mul(0.2)).add(R.mul(-0.06))
		q1 := l.piv.add(rotAbout(l.d0, l.ax, a1).mul(0.2)).add(R.mul(-0.06))
		bGlowLine.line(q0, q1, c)
	}
	// arm and a fat red crossbar handle
	h := l.handle()
	useMat(mSteel)
	cyl(&bEnv, l.piv, h, 0.013, 0.011, 10, cSteelEnv, false)
	useMat(mRubber)
	cyl(&bEnv, h.sub(R.mul(0.07)), h.add(R.mul(0.07)), 0.022, 0.022, 12, cDanger.scale(0.8), true)
	if !l.held {
		sprite(h, 0.1, cDanger.alpha(float32(0.25+0.2*math.Sin(t*4))))
	}
	lp := at(0, -0.1, -0.048)
	drawTextCentered(&bOpaque, "LIGHTS OUT", lp, R, U, 0.0012, cDanger)
	drawTextCentered(&bOpaque, "PULL TO START", lp.add(U.mul(-0.03)), R, U, 0.0008, cWhite)
}

// consoleSpot: inside your ring, a little way from the device.
func (g *game) consoleSpot() sim.V2 {
	r := g.r
	c := sim.Centroid(r.RingPoly)
	for _, d := range []sim.V2{sim.V(0, -0.9), sim.V(0.9, 0), sim.V(-0.9, 0), sim.V(0, 0.9), sim.V(0, -0.6), sim.V(0.6, 0)} {
		q := c.Add(d)
		if sim.InPoly(r.RingPoly, q) && sim.EdgeDist(r.RingPoly, q) > 0.35 {
			return q
		}
	}
	return c
}

func (g *game) startRound() {
	if g.r.Phase != sim.Setup {
		return
	}
	g.r.Start()
	g.lightsT = g.t
	at := g.console.pos
	ed.sfx(sndBoom, at, 0.5)
	ed.sfx(sndPowerDown, fv(sim.Centroid(g.r.Room), 5), 1)
	g.later(1.4, func() { ed.sfx(sndAlarm, at, 0.6) })
	g.later(1.8, func() { ed.sfx(sndPowerUp, at, 0.7) })
	g.later(2.4, func() { radioSay("x_start", true) })
	g.say("LIGHTS OUT. HERE THEY COME", "BUILD THE DEVICE. HOLD THE RING.", cEnergy, 3)
}

// ---- results ----

func (g *game) buildResults() {
	r := g.r
	p := &uiPanel{w: 0.7, h: 0.5, n: v3(0, 0, 1), up: v3(0, 1, 0), right: v3(1, 0, 0)}
	p.show = func() bool { return r.Phase == sim.Won || r.Phase == sim.Lost }
	p.lines = func() []string {
		head := "YOU'RE OUT"
		if r.Phase == sim.Lost {
			head = "THEY TOOK IT"
		}
		mins := int(r.Time) / 60
		secs := int(r.Time) % 60
		return []string{
			head,
			fmt.Sprintf("TIME %d:%02d   WAVES %d", mins, secs, r.Wave),
			fmt.Sprintf("WARDENS DOWN %d   WALKERS BURNED THROUGH %d", r.Stats.Kills, r.Stats.Burns),
			fmt.Sprintf("GENERATORS BLOWN %d   DEVICE GRABBED %d   SAVES %d", r.Stats.GensBlown, r.Stats.Steals, r.Stats.Saves),
			fmt.Sprintf("DEVICE %d%%", int(100*r.Work()/r.WorkTotal())),
		}
	}
	p.btns = []*uiBtn{
		{label: func() string { return "AGAIN" }, x: 0, y: -0.16, w: 0.4, h: 0.09, accent: cEnergy, act: func() { g.restart() }},
	}
	if canLeave {
		p.h = 0.6
		p.btns = append(p.btns, &uiBtn{label: func() string { return "BACK TO ZONETWEAK" }, x: 0, y: -0.25, w: 0.4, h: 0.06, accent: cBeam, act: leaveToZone})
	}
	g.results = p
}

// restart: a new round in the same room, with the same number of generators. The maze
// isn't kept: you build it as they come.
func (g *game) restart() {
	old := g.r
	n := old.Machines()
	var R []pt2
	for _, p := range old.Room {
		R = append(R, pv(p))
	}
	log.Printf("restart: %d machines, room %v", n, R)
	ng := newGame(R, g.seed+1)
	if ng.r.SetGens(n) {
		ng.syncGens()
		ng.buildPanels()
	}
	for _, gh := range hands {
		if gh != nil {
			gh.held, gh.hover, gh.building = nil, nil, false
		}
	}
	G = ng
}

// ---- the pause menu ----

// leaveToZone: back to ZoneTweak, the editor (after this frame).
func leaveToZone() {
	if onLeave != nil {
		onLeave()
	}
}

func (g *game) buildMenu() {
	p := &uiPanel{w: 0.5, h: 0.42}
	p.show = func() bool { return g.menuOpen }
	p.lines = func() []string { return []string{"PAUSED"} }
	p.h = 0.58
	if canLeave {
		p.h = 0.66
	}
	p.btns = []*uiBtn{
		{label: func() string { return "RESUME" }, x: 0, y: 0.14, w: 0.4, h: 0.065, accent: cEnergy, act: func() { g.menuOpen = false }},
		{label: func() string { return "RESTART THIS ROUND" }, x: 0, y: 0.065, w: 0.4, h: 0.065, act: func() { g.menuOpen = false; g.restart() }},
		{label: func() string { return "FIT GLOVES" }, x: 0, y: -0.01, w: 0.4, h: 0.065, accent: cAmber, act: func() { g.startFit() }},
		{label: func() string { return "SET FLOOR" }, x: 0, y: -0.085, w: 0.4, h: 0.065, accent: cBeam, act: func() { g.startSetFloor() }},
		{label: func() string { return "GRAPHICS: " + qualityName() }, x: 0, y: -0.16, w: 0.4, h: 0.065, act: cycleQuality},
		{label: func() string { return "QUIT" }, x: 0, y: -0.235, w: 0.4, h: 0.065, accent: cDanger, act: func() { ed.quit = true }},
	}
	if canLeave {
		// back to the editor, just above QUIT
		q := p.btns[5]
		p.btns = append(p.btns[:5], &uiBtn{label: func() string { return "BACK TO ZONETWEAK" }, w: 0.4, h: 0.065, accent: cBeam, act: func() {
			g.menuOpen = false
			leaveToZone()
		}}, q)
		for i, b := range p.btns {
			b.y = 0.18 - 0.075*float64(i)
		}
	}
	g.menu = p
}

func (g *game) openMenu() {
	if g.fitting {
		g.endFit() // B finishes fitting
		return
	}
	g.menuOpen = !g.menuOpen
	if g.menuOpen {
		cam := ed.hmd.pos()
		fwd := ed.hmd.axis(2).mul(-1)
		fwd.y = 0
		if fwd.length() < 0.1 {
			fwd = v3(0, 0, -1)
		}
		fwd = fwd.norm()
		g.menu.pos = cam.add(fwd.mul(0.6)).add(v3(0, -0.2, 0))
		g.menu.face(cam)
	}
}

// ---- the radio ----

type radio struct {
	pos       sim.V2
	set       *prop
	listening bool
	heard     int
	talkT     float64
	text      string
	intelWave int     // the wave the current intel is about
	intelAt   float64 // when it came in (game time)
	nagged    int     // the wave we last prompted you to pick up for
	inbox     []radioMsg
	nagAt     float64 // when the speaker may prompt next
	nagLeft   int     // prompts left for what's waiting now
	nags      int
	awayT     float64 // the handset's been left off its cradle out of your sight this long
}

func (g *game) radioSpot() sim.V2 {
	r := g.r
	c := sim.Centroid(r.RingPoly)
	best, bp := -1.0, c
	for k := 0; k < 16; k++ {
		a := float64(k) * math.Pi / 8
		for _, rad := range []float64{1.3, 1.0, 0.8, 0.6} {
			q := c.Add(sim.V(math.Cos(a), math.Sin(a)).Mul(rad))
			if !sim.InPoly(r.RingPoly, q) || sim.EdgeDist(r.RingPoly, q) < 0.4 {
				continue
			}
			s := sim.Dist(q, r.Dev.Pos) + 0.6*sim.Dist(q, g.consoleSpot())
			if s > best {
				best, bp = s, q
			}
			break
		}
	}
	return bp
}

func (g *game) buildRadio() {
	rd := &radio{pos: g.radioSpot()}
	cradle := func() (vec3, vec3, vec3, vec3) {
		return fv(rd.pos, 1.07).add(v3(0, 0, 0)), v3(1, 0, 0), v3(0, 1, 0), v3(0, 0, 1)
	}
	rd.set = &prop{radius: 0.1, hy: 0.03, cordLen: 1.6, home: cradle, homeR: 0.15, docked: true}
	rd.set.anchor = func() vec3 { return fv(rd.pos, 1.06) } // the top of the set, by the cradle
	rd.set.X, rd.set.Y, rd.set.Z = v3(1, 0, 0), v3(0, 1, 0), v3(0, 0, 1)
	g.radio = rd
}

func intelText(r *sim.Round) []string {
	in := r.Intel
	out := []string{"WAVE " + itoa(in.Wave)}
	for _, gr := range in.Groups {
		what := "WALKER"
		switch gr.Kind {
		case sim.Heavy:
			what = "HEAVY"
		case sim.Drone:
			what = strings.ToUpper(gr.DType)
		}
		if gr.N != 1 {
			if what == "HEAVY" {
				what = "HEAVIES"
			} else {
				what += "S"
			}
		}
		out = append(out, fmt.Sprintf("%d %s, %s DOOR", gr.N, what, r.Doors[gr.Door].Name))
	}
	return out
}

func (g *game) radioFrame(dt float64) {
	rd := g.radio
	r := g.r
	rd.set.tick(dt)
	// left lying about: once you look away it's back on top of the radio
	if s := rd.set; !s.held && !s.docked {
		fwd := ed.hmd.axis(2).mul(-1)
		if s.pos.sub(ed.hmd.pos()).norm().dot(fwd) < 0.45 {
			rd.awayT += dt
		} else {
			rd.awayT = 0
		}
		if rd.awayT > 0.5 {
			s.docked, s.resting, s.onCart, s.vel = true, false, false, vec3{}
			s.tick(0)
			rd.awayT = 0
		}
	} else {
		rd.awayT = 0
	}
	ear := false
	if rd.set.held {
		cam := ed.hmd.pos()
		side := ed.hmd.axis(0)
		for _, s := range []float64{-1, 1} {
			if rd.set.pos.sub(cam.add(side.mul(0.09*s))).length() < 0.16 {
				ear = true
			}
		}
	}
	if ear && !rd.listening {
		ed.sfx(sndTick, rd.set.pos, 0.6)
		rd.listening = true
		rd.playWaiting(g.t)
		// a misfire's fix: every time you pick up, till it's done
		if k := g.misfire; k != nil && k.ordersOpen() {
			eng.queueClips(srcHandset, k.orderClips(), false)
			if !k.heard {
				k.heard, k.heardT = true, g.t
			}
		}
	}
	rd.listening = ear
	if ear && r.Phase == sim.Playing {
		rd.talkT += dt
		if rd.heard != r.Intel.Wave {
			rd.heard = r.Intel.Wave
			rd.talkT = 0
			eng.queueClips(srcHandset, intelLines(r), false)
		}
		r.Heard = true
	}
	// nobody's listening: the speaker prompts you to pick up - when something new comes in
	// and once more a while later
	if r.Intel.Wave != rd.intelWave {
		rd.intelWave, rd.intelAt = r.Intel.Wave, g.t
	}
	if r.Phase == sim.Playing && !r.Heard && g.t-rd.intelAt > 6 && rd.nagged != r.Intel.Wave {
		rd.nagged = r.Intel.Wave
		rd.nagLeft = 2
		rd.nagAt = math.Min(rd.nagAt, g.t)
	}
	msgs := rd.waiting(g.t)
	unheard := r.Phase == sim.Playing && !r.Heard && g.t-rd.intelAt > 6
	if r.Phase != sim.Playing || len(msgs) == 0 && !unheard {
		rd.nagLeft = 0
	}
	if rd.nagLeft > 0 && !rd.set.held && g.t >= rd.nagAt && eng.clipBusyFor(srcRadio) <= 0 {
		urgent := false
		for _, m := range msgs {
			urgent = urgent || m.urgent
		}
		rd.nag(urgent)
		rd.nagLeft--
		rd.nagAt = g.t + 15
		if urgent {
			rd.nagAt = g.t + 10
		}
	}
	g.radioAudio()
}

func (g *game) drawRadio() {
	rd := g.radio
	r := g.r
	t := fxTime()
	base := fv(rd.pos, 0)
	seedTex(17000)
	// tripod stand
	useMat(mSteel)
	for k := 0; k < 3; k++ {
		a := float64(k) * 2 * math.Pi / 3
		foot := base.add(v3(math.Cos(a)*0.22, 0, math.Sin(a)*0.22))
		cyl(&bEnv, foot, base.add(v3(0, 0.82, 0)), 0.01, 0.01, 6, cSteelEnv, false)
	}
	// the set
	F := xf{base.add(v3(0, 0.92, 0)), v3(1, 0, 0), v3(0, 1, 0), v3(0, 0, 1)}
	cam := ed.hmd.pos()
	to := cam.sub(F.o)
	to.y = 0
	if to.length() > 0.1 {
		F.z = to.norm()
		F.x = v3(0, 1, 0).cross(F.z).norm()
	}
	useMat(mPaint)
	bbox(&bEnv, F, 0, 0, 0, 0.2, 0.11, 0.12, 0.015, rgba{0.24, 0.3, 0.22, 1})
	useMat(mCast)
	bbox(&bEnv, F, 0, 0.115, 0, 0.12, 0.012, 0.05, 0.005, cIron)
	cyl(&bEnv, F.p(0.15, 0.1, -0.05), F.p(0.2, 0.55, -0.08), 0.005, 0.004, 6, cSteelEnv, false)
	// screen
	sc := F.p(-0.05, 0.01, 0.121)
	unheard := r.Phase == sim.Playing && (!r.Heard || len(rd.inbox) > 0)
	bTrans.quadC(sc.add(F.x.mul(-0.12)).add(F.y.mul(-0.07)), sc.add(F.x.mul(0.12)).add(F.y.mul(-0.07)), sc.add(F.x.mul(0.12)).add(F.y.mul(0.07)), sc.add(F.x.mul(-0.12)).add(F.y.mul(0.07)),
		rgba{0.02, 0.05, 0.02, 1}, rgba{0.02, 0.05, 0.02, 1}, rgba{0.03, 0.08, 0.03, 1}, rgba{0.03, 0.08, 0.03, 1})
	if r.Phase == sim.Playing {
		if n := len(rd.inbox); n > 0 && !rd.listening {
			msg := "1 MESSAGE - PICK UP"
			if n > 1 {
				msg = itoa(n) + " MESSAGES - PICK UP"
			}
			drawTextCentered(&bOpaque, msg, sc.add(F.z.mul(0.001)), F.x, F.y, 0.0012, cAmber.alpha(float32(0.6+0.4*math.Sin(t*5))))
		} else if k := g.misfire; k != nil && k.ordersOpen() && k.heard {
			drawText(&bOpaque, "THE FIX:", sc.add(F.x.mul(-0.11)).add(F.y.mul(0.055)).add(F.z.mul(0.001)), F.x, F.y, 0.0012, cAmber)
			for i, o := range k.orders {
				c := cEnergy
				if o.done {
					c = cMuted
				}
				drawText(&bOpaque, k.orderText(o), sc.add(F.x.mul(-0.11)).add(F.y.mul(0.033-float64(i)*0.022)).add(F.z.mul(0.001)), F.x, F.y, 0.00085, c)
			}
		} else if r.Heard || rd.listening {
			for i, l := range intelText(r) {
				drawText(&bOpaque, l, sc.add(F.x.mul(-0.11)).add(F.y.mul(0.055-float64(i)*0.022)).add(F.z.mul(0.001)), F.x, F.y, 0.0012, cEnergy)
			}
		} else {
			drawTextCentered(&bOpaque, "PICK UP AND LISTEN", sc.add(F.z.mul(0.001)), F.x, F.y, 0.0013, cAmber.alpha(float32(0.6+0.4*math.Sin(t*5))))
		}
		drawText(&bOpaque, "NEXT "+itoa(int(math.Ceil(r.NextWave-r.Time)))+"S", sc.add(F.x.mul(-0.11)).add(F.y.mul(-0.06)).add(F.z.mul(0.001)), F.x, F.y, 0.001, cDanger)
	}
	lamp := F.p(0.15, 0.06, 0.121)
	lc := cMuted.scale(0.3)
	if unheard {
		lc = cAmber.scale(float32(math.Max(0, math.Sin(t*7))))
		addPoint(lamp.add(F.z.mul(0.05)), cAmber, 1.2*math.Max(0, math.Sin(t*7)), 1.5)
	}
	curEmis = 4
	sphere(&bEnv, lamp, 0.012, lc, 3, 6)
	curEmis = 1
	// handset and its coiled cord
	s := rd.set
	if s.docked {
		s.pos, s.X, s.Y, s.Z = F.p(0, 0.15, 0), F.x, F.y, F.z
		s.home = func() (vec3, vec3, vec3, vec3) { return F.p(0, 0.15, 0), F.x, F.y, F.z }
	}
	a := s.anchor()
	pts := drapeCord(a, s.pos, s.cordLen, 14, 0.008, cordBox{F: F, hx: 0.2, hz: 0.12, y0: -0.11, y1: 0.127})
	for k := range pts {
		if k > 0 && k < len(pts)-1 {
			// coils
			ph := float64(k) * 2.2
			pts[k] = pts[k].add(v3(math.Cos(ph)*0.008, math.Sin(ph)*0.008, 0))
		}
	}
	drawCord(pts, 0.006, rgba{0.05, 0.05, 0.05, 1})
	useMat(mPaint)
	bevelBox(&bEnv, s.pos, s.X, s.Y, s.Z, 0.11, 0.02, 0.025, 0.01, rgba{0.1, 0.1, 0.1, 1})
	for _, sx := range []float64{-1, 1} {
		cyl(&bEnv, s.pos.add(s.X.mul(sx*0.1)), s.pos.add(s.X.mul(sx*0.1)).add(s.Y.mul(-0.03)), 0.022, 0.022, 10, rgba{0.1, 0.1, 0.1, 1}, true)
	}
	if rd.listening && r.Phase == sim.Playing {
		// what you hear, as subtitles near the handset
		p := s.pos.add(v3(0, 0.12, 0))
		rr, uu := billboard(cam, p)
		for i, l := range intelText(r) {
			drawTextCentered(&bOpaque, l, p.add(v3(0, -float64(i)*0.03, 0)), rr, uu, 0.0014, cEnergy)
		}
	}
}

// ---- the frame ----

func (g *game) init() {
	g.leeches = map[int]*leechHandle{}
	g.buildPanels()
	g.buildDevice()
	g.buildConsole()
	g.buildResults()
	g.buildMenu()
	g.buildRadio()
	g.buildDriver()
	g.buildWrench()
	g.buildLeads()
	g.buildMisfire()
	g.buildPole()
	g.buildFitPanel()
	g.grips = [2]*harpGrip{{g: g, side: -1}, {g: g, side: 1}}
	g.lines = [2]*harpLine{{g: g, idx: 0}, {g: g, idx: 1}}
}

func (g *game) panels() []*uiPanel {
	return []*uiPanel{g.console, g.results, g.menu, g.fitPanel}
}

func (g *game) grabbers() []grabber {
	var out []grabber
	if g.menuOpen || g.fitting {
		return out
	}
	if g.lightsOut != nil && g.r.Phase == sim.Setup {
		out = append(out, g.lightsOut)
	}
	out = g.genGrabbers(out)
	out = g.devGrabbers(out)
	out = g.harpGrabbers(out)
	if g.pole != nil {
		out = append(out, g.pole.p)
	}
	out = append(out, g.radio.set)
	return out
}

func (g *game) pokers() []poker {
	var out []poker
	for _, p := range g.panels() {
		out = p.pokeList(out)
	}
	if g.menuOpen || g.fitting {
		return out
	}
	out = g.devPokers(out)
	return out
}

// frame: one VR frame of the game: hands first (they drive the machines), then the rules.
func (g *game) frame(dt float64) {
	// the menu: a press of B that wasn't aimed at a magenta wall (on letting go), or a
	// click of the stick
	for _, gh := range hands {
		if !gh.valid() {
			continue
		}
		h := gh.h
		bUp := !h.held(bitMenu) && h.prev&(1<<bitMenu) != 0
		if h.down(bitStick) || bUp && (!gh.bOnWall || g.menuOpen || g.fitting) {
			gh.bOnWall = false
			g.openMenu()
		}
	}
	cam := ed.hmd.pos()
	g.lampFrame()
	if g.results != nil && (g.r.Phase == sim.Won || g.r.Phase == sim.Lost) && g.results.pos == (vec3{}) {
		fwd := ed.hmd.axis(2).mul(-1)
		fwd.y = 0
		if fwd.length() < 0.1 {
			fwd = v3(0, 0, -1)
		}
		g.results.pos = cam.add(fwd.norm().mul(0.9)).add(v3(0, -0.15, 0))
		g.results.face(cam)
	}
	g.handFrame(dt)
	g.floorFrame(dt)
	g.laserFrame(dt)
	g.pullFrame(dt)
	if g.fitting {
		g.fitFrame()
	}
	if !g.menuOpen && !g.fitting && !g.settingFloor {
		g.panelFrame(dt)
		g.deviceFrame(dt)
		g.radioFrame(dt)
		g.harpoonTick(dt)
		g.step(dt)
	}
	for i := 0; i < len(g.timers); i++ {
		if g.t >= g.timers[i].at {
			f := g.timers[i].fn
			g.timers = append(g.timers[:i], g.timers[i+1:]...)
			i--
			f()
		}
	}
}

type timer struct {
	at float64
	fn func()
}

func (g *game) later(s float64, fn func()) { g.timers = append(g.timers, timer{g.t + s, fn}) }

func (g *game) drawUI() {
	hot := map[*uiPanel]int{}
	for _, p := range g.panels() {
		hot[p] = -1
	}
	for _, gh := range hands {
		if gh.valid() && gh.beam && gh.hit.kind == 4 {
			hot[gh.hit.panel] = gh.hit.btn
		}
	}
	for _, p := range g.panels() {
		p.draw(hot)
	}
	if g.r.Phase == sim.Setup && g.console != nil {
		// the console pedestal
		b := g.console.pos
		useMat(mCast)
		cyl(&bEnv, v3(b.x, 0, b.z), v3(b.x, 0.03, b.z), 0.25, 0.25, 20, cIron, true)
		useMat(mSteel)
		cyl(&bEnv, v3(b.x, 0.03, b.z), v3(b.x, b.y-0.27, b.z), 0.05, 0.04, 14, cSteelEnv.scale(0.7), true)
		g.drawLightsOut()
	}
}
