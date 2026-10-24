package main

// Five more of the games the device can want, behind the same hatch on the cart's left side
// as Simon and the needle:
//
//   freq    two knobs tune your wave (how fast, how tall) onto the target's, which drifts
//   safe    a dial: feel for the click on each of three numbers and hold it there
//   plug    a switchboard: plug each call's cord into the line its lamp lights, in time
//   valves  three valves, three gauges, each valve bleeding the next gauge: all in the green
//   look    the code shows on a screen on the far side of the cart: go and read it, come
//           back and key it in (key it wrong and it changes)

import (
	"boundaryvr/sim"
	"math"
	"math/rand"
)

// mini: one of these games, in the compartment's frame (c its middle, n out of it, side to
// the right, up).
type mini interface {
	frame(m *modGame, g *game, c, n, side, up vec3, dt float64)
	draw(m *modGame, g *game, c, n, side, up vec3)
	grabbers(out []grabber) []grabber
	pokers(out []poker) []poker
}

func newMini(kind string, g *game) mini {
	switch kind {
	case "freq":
		return newFreq()
	case "safe":
		return newSafe()
	case "plug":
		return newPlug()
	case "valves":
		return newValves()
	case "look":
		return newLook()
	}
	return nil
}

// miniKnob: a knob drawn the usual way (a cast cylinder with a mark on it).
func miniKnob(k *knob, n, up vec3, label string) {
	useMat(mCast)
	cyl(&bEnv, k.c.sub(n.mul(0.022)), k.c.sub(n.mul(0.012)), k.r+0.008, k.r+0.008, 18, cIron, true)
	useMat(mSteel)
	cyl(&bEnv, k.c.sub(n.mul(0.012)), k.c, k.r, k.r*0.92, 18, cSteelEnv, true)
	mk := rotAbout(up, n, k.ang)
	flatLine(&bOpaque, k.c.add(n.mul(0.001)), k.c.add(n.mul(0.001)).add(mk.mul(k.r*0.85)), n, 0.004, cAmber)
	if label != "" {
		faceText(&bOpaque, label, k.c.add(up.mul(-k.r-0.012)).sub(n.mul(0.01)), n, up, 0.0008, cMuted)
	}
}

// miniTitle: the instruction along the top of the compartment.
func miniTitle(c, n, up vec3, s string) {
	faceText(&bOpaque, s, c.add(up.mul(0.105)).add(n.mul(0.03)), n, up, 0.0011, cAmber)
}

// miniBar: the progress bar along the bottom.
func miniBar(c, n, side, up vec3, p float64) {
	bp := c.add(up.mul(-0.112)).add(n.mul(0.025))
	flatLine(&bOpaque, bp.add(side.mul(-0.1)).sub(n.mul(0.001)), bp.add(side.mul(0.1)).sub(n.mul(0.001)), n, 0.008, rgba{0.08, 0.09, 0.09, 1})
	ribbon(bp.add(side.mul(-0.1)), bp.add(side.mul(-0.1+0.2*math.Max(0, math.Min(1, p)))), 0.01, cEnergy.alpha(0.9))
}

// miniScreen: a dark glass screen.
func miniScreen(c, side, up vec3, hw, hh float64) {
	bTrans.quadC(c.add(side.mul(-hw)).add(up.mul(-hh)), c.add(side.mul(hw)).add(up.mul(-hh)), c.add(side.mul(hw)).add(up.mul(hh)), c.add(side.mul(-hw)).add(up.mul(hh)),
		rgba{0.02, 0.06, 0.04, 0.95}, rgba{0.02, 0.06, 0.04, 0.95}, rgba{0.03, 0.09, 0.06, 0.95}, rgba{0.03, 0.09, 0.06, 0.95})
}

// ---- freq: match the wave ----

type miniFreq struct {
	kf, ka       *knob
	ft0, at0     float64 // the target's frequency and height (before its drift)
	ph1, ph2     float64
	t            float64
	prog         float64
	locked       bool
	started      bool
	f, a, ft, at float64
}

func newFreq() *miniFreq {
	m := &miniFreq{ft0: 1.7 + rand.Float64()*1.3, at0: 0.4 + rand.Float64()*0.4, ph1: rand.Float64() * 6, ph2: rand.Float64() * 6}
	m.kf = &knob{r: 0.024, lo: 0, hi: 2 * math.Pi, detent: math.Pi / 24}
	m.ka = &knob{r: 0.024, lo: 0, hi: 2 * math.Pi, detent: math.Pi / 24}
	return m
}

// wave: frequency (waves across the screen) and height from the knobs.
func (q *miniFreq) wave() (float64, float64) {
	return 1 + 2.5*q.kf.ang/(2*math.Pi), 0.15 + 0.85*q.ka.ang/(2*math.Pi)
}

func (q *miniFreq) target() (float64, float64) {
	return q.ft0 + 0.25*math.Sin(0.23*q.t+q.ph1), q.at0 + 0.1*math.Sin(0.31*q.t+q.ph2)
}

func (q *miniFreq) frame(m *modGame, g *game, c, n, side, up vec3, dt float64) {
	open := func() bool { return m.open > 0.9 }
	q.kf.ok, q.ka.ok = open, open
	q.kf.c, q.kf.ax = c.add(side.mul(-0.09)).add(up.mul(-0.07)).add(n.mul(0.03)), n
	q.ka.c, q.ka.ax = c.add(side.mul(0.09)).add(up.mul(-0.07)).add(n.mul(0.03)), n
	if !q.started {
		q.started = q.kf.held || q.ka.held
	}
	if q.started {
		q.t += dt // the target only drifts once you're at it
	}
	q.f, q.a = q.wave()
	q.ft, q.at = q.target()
	err := math.Max(math.Abs(q.f-q.ft)/0.18, math.Abs(q.a-q.at)/0.09)
	was := q.locked
	q.locked = err < 1
	if q.locked != was && q.started {
		ed.sfx(sndRelay, q.kf.c, 0.25)
		for _, k := range []*knob{q.kf, q.ka} {
			if h := g.handHolding(k); h != nil {
				h.buzz(25, 0.7)
			}
		}
	}
	if q.locked && q.started {
		q.prog += dt / 10
	} else {
		q.prog = math.Max(0, q.prog-dt/30)
	}
	g.r.SetModuleProgress(q.prog)
	if q.prog >= 1 {
		g.r.CompleteModule()
	}
}

func (q *miniFreq) draw(m *modGame, g *game, c, n, side, up vec3) {
	sc := c.add(up.mul(0.03)).add(n.mul(0.02))
	miniScreen(sc, side, up, 0.12, 0.055)
	t := fxTime()
	line := func(f, a float64, col rgba, w float32) {
		const segs = 48
		var prev vec3
		for i := 0; i <= segs; i++ {
			u := float64(i) / segs
			y := a * 0.045 * math.Sin(2*math.Pi*f*u+t*3)
			p := sc.add(side.mul(-0.11 + 0.22*u)).add(up.mul(y)).add(n.mul(0.002))
			if i > 0 {
				glowLine(prev, p, col, w)
			}
			prev = p
		}
	}
	tcol := cEnergy.alpha(0.35)
	ycol := cAmber
	if q.locked && q.started {
		tcol, ycol = cEnergy.alpha(0.8), cEnergyHot
	}
	line(q.ft, q.at, tcol, 0.6)
	line(q.f, q.a, ycol, 0.4)
	miniKnob(q.kf, n, up, "HOW FAST")
	miniKnob(q.ka, n, up, "HOW TALL")
	msg := "TURN THE KNOBS: YOUR WAVE ONTO THE GREEN ONE"
	if !q.started {
		msg = "TAKE A KNOB TO START"
	} else if q.locked {
		msg = "LOCKED - HOLD IT THERE"
	}
	miniTitle(c, n, up, msg)
	miniBar(c, n, side, up, q.prog)
}

func (q *miniFreq) grabbers(out []grabber) []grabber { return append(out, q.kf, q.ka) }
func (q *miniFreq) pokers(out []poker) []poker       { return out }

// ---- safe: crack the dial ----

const safeNums = 40

type miniSafe struct {
	dial  *knob
	comb  [3]int
	got   int     // numbers found
	hold  float64 // how long it's sat on the current number
	last  int     // the number at the mark last frame
	clunk float64 // the click flash
}

func newSafe() *miniSafe {
	m := &miniSafe{dial: &knob{r: 0.048, lo: math.Inf(-1), hi: math.Inf(1), detent: 2 * math.Pi / safeNums}}
	for i := range m.comb {
		for {
			v := rand.Intn(safeNums)
			ok := v != 0
			for j := 0; j < i; j++ {
				ok = ok && safeDist(v, m.comb[j]) >= 6
			}
			if ok {
				m.comb[i] = v
				break
			}
		}
	}
	return m
}

func safeDist(a, b int) int {
	d := (a - b + safeNums) % safeNums
	return min(d, safeNums-d)
}

// number: what's under the mark (turning it clockwise counts up).
func (q *miniSafe) number() int {
	k := int(math.Round(-q.dial.ang / (2 * math.Pi / safeNums)))
	return ((k % safeNums) + safeNums) % safeNums
}

// signal: how close the dial is to the number you're after (1 on it, 0 four or more off).
func (q *miniSafe) signal() float64 {
	if q.got >= 3 {
		return 0
	}
	return math.Max(0, 1-float64(safeDist(q.number(), q.comb[q.got]))/4)
}

func (q *miniSafe) frame(m *modGame, g *game, c, n, side, up vec3, dt float64) {
	q.dial.ok = func() bool { return m.open > 0.9 && q.got < 3 }
	q.dial.c, q.dial.ax = c.add(side.mul(-0.03)).add(up.mul(-0.02)).add(n.mul(0.03)), n
	q.clunk -= dt
	if q.got >= 3 {
		return
	}
	num := q.number()
	h := g.handHolding(q.dial)
	if num != q.last {
		q.last, q.hold = num, 0
		if h != nil {
			// the nearer the number, the heavier each click; on it, a clunk
			switch d := safeDist(num, q.comb[q.got]); {
			case d == 0:
				h.buzz(70, 1)
				ed.sfx(sndRelay, q.dial.c, 0.6)
				q.clunk = 0.25
			case d <= 3:
				h.buzz(10+12*(4-d), float32(0.4+0.15*float64(4-d)))
				ed.sfx(sndTick, q.dial.c, 0.25+0.1*float64(4-d))
			}
		}
	}
	if num == q.comb[q.got] && h != nil {
		q.hold += dt
		if q.hold > 1 {
			q.got++
			q.hold = 0
			ed.sfx(sndPowerUp, q.dial.c, 0.5)
			h.buzz(120, 1)
			if q.got >= 3 {
				g.r.CompleteModule()
				return
			}
		}
	} else {
		q.hold = 0
	}
	g.r.SetModuleProgress((float64(q.got) + math.Min(1, q.hold)) / 3)
}

func (q *miniSafe) draw(m *modGame, g *game, c, n, side, up vec3) {
	k := q.dial
	// the dial: numbers round its face, a mark above it
	useMat(mCast)
	cyl(&bEnv, k.c.sub(n.mul(0.025)), k.c.sub(n.mul(0.006)), k.r+0.012, k.r+0.012, 28, cIron, true)
	useMat(material{0, 0.05, 0.5, 0}) // enamel
	cyl(&bEnv, k.c.sub(n.mul(0.006)), k.c, k.r, k.r, 28, rgba{0.86, 0.84, 0.76, 1}, true)
	for i := 0; i < safeNums; i++ {
		a := k.ang + float64(i)*2*math.Pi/safeNums
		d := rotAbout(up, n, a)
		r0 := k.r * 0.84
		if i%5 == 0 {
			r0 = k.r * 0.74
			p := k.c.add(d.mul(k.r * 0.56)).add(n.mul(0.0015))
			drawTextCentered(&bOpaque, itoa(i), p, d.cross(n).norm(), d, 0.0011, rgba{0.05, 0.05, 0.05, 1})
		}
		flatLine(&bOpaque, k.c.add(d.mul(r0)).add(n.mul(0.0015)), k.c.add(d.mul(k.r*0.98)).add(n.mul(0.0015)), n, 0.0022, rgba{0.05, 0.05, 0.05, 1})
	}
	mk := k.c.add(up.mul(k.r + 0.016)).add(n.mul(0.001))
	col := cAmber
	if q.clunk > 0 {
		col = cEnergyHot
		sprite(mk, 0.04, cEnergy.alpha(0.6))
	}
	flatLine(&bOpaque, mk, mk.add(up.mul(-0.01)), n, 0.004, col)
	// three lamps, one per number found; a listening meter beside them
	for i := 0; i < 3; i++ {
		p := c.add(side.mul(0.075)).add(up.mul(0.05 - 0.03*float64(i))).add(n.mul(0.025))
		lc := rgba{0.25, 0.08, 0.06, 1}
		if i < q.got {
			lc = cEnergy
			sprite(p, 0.035, cEnergy.alpha(0.6))
		}
		useMat(mCast)
		cyl(&bEnv, p.sub(n.mul(0.01)), p.sub(n.mul(0.004)), 0.012, 0.012, 12, cIron, true)
		curEmis = 2
		useMat(mPaint)
		cyl(&bEnv, p.sub(n.mul(0.004)), p, 0.008, 0.008, 12, lc, true)
		curEmis = 1
	}
	mb := c.add(side.mul(0.12)).add(up.mul(-0.07)).add(n.mul(0.025))
	s := q.signal()
	flatLine(&bOpaque, mb.sub(n.mul(0.001)), mb.add(up.mul(0.12)).sub(n.mul(0.001)), n, 0.014, rgba{0.08, 0.09, 0.09, 1})
	if s > 0 {
		ribbon(mb, mb.add(up.mul(0.12*s)), 0.012, cAmber.mix(cEnergy, float32(s)).alpha(0.9))
	}
	faceText(&bOpaque, "SIGNAL", mb.add(up.mul(-0.012)).sub(n.mul(0.004)), n, up, 0.0007, cMuted)
	msg := "TURN THE DIAL. FEEL FOR THE CLUNK, STOP ON IT"
	if g.handHolding(k) == nil && q.got == 0 {
		msg = "TAKE THE DIAL"
	} else if q.hold > 0 {
		msg = "HOLD IT THERE"
	}
	miniTitle(c, n, up, msg)
	miniBar(c, n, side, up, (float64(q.got)+math.Min(1, q.hold))/3)
}

func (q *miniSafe) grabbers(out []grabber) []grabber { return append(out, q.dial) }
func (q *miniSafe) pokers(out []poker) []poker       { return out }

// ---- plug: the switchboard ----

const (
	plugCalls   = 4   // calls to put through
	plugMisses  = 2   // calls let ring out (or put through wrong) before it starts over
	plugTime    = 9.0 // seconds a call rings before it's lost
	plugSnap    = 0.045
	plugCallGap = 1.2
)

var plugCols = [4]rgba{{1, 0.25, 0.2, 1}, {0.25, 0.55, 1, 1}, {1, 0.85, 0.2, 1}, {0.35, 1, 0.45, 1}}

type miniPlug struct {
	g        *game
	plugs    [4]*swPlug
	jacks    [4]vec3 // the line jacks (A-D)
	n        vec3
	caller   int // the call ringing now (-1: between calls)
	line     int // the line it wants
	ring     float64
	wait     float64
	done     int
	misses   int
	started  bool
	failFlas float64
	lastLine int
}

type swPlug struct {
	q      *miniPlug
	i      int
	rest   vec3 // its holder
	anchor vec3 // where its cord comes out
	pos    vec3
	off    vec3
	in     int // the line jack it's in (-1: none)
	held   bool
	backT  float64 // after a call's put through it pops back out
	ok     func() bool
}

func (p *swPlug) gPos() vec3     { return p.pos }
func (p *swPlug) gRad() float64  { return 0.06 }
func (p *swPlug) gOK() bool      { return p.ok == nil || p.ok() }
func (p *swPlug) gGrab(h *gHand) { p.held, p.in, p.off = true, -1, p.pos.sub(h.palm); h.buzz(20, 0.5) }
func (p *swPlug) gHold(h *gHand, dt float64) {
	p.pos = h.palm.add(p.off)
	// near a jack it lines up with it
	for j, jp := range p.q.jacks {
		if jp.sub(p.pos).length() < plugSnap {
			p.pos = jp.add(p.q.n.mul(0.012))
			if p.q.lastLine != j {
				p.q.lastLine = j
				h.buzz(15, 0.6)
			}
			return
		}
	}
	p.q.lastLine = -1
}
func (p *swPlug) gRelease(h *gHand) {
	p.held = false
	for j, jp := range p.q.jacks {
		if jp.sub(p.pos).length() < plugSnap*1.4 {
			p.in, p.pos = j, jp.add(p.q.n.mul(0.012))
			ed.sfx(sndClang, jp, 0.35)
			h.buzz(40, 0.8)
			p.q.plugged(p, j)
			return
		}
	}
	p.in, p.pos = -1, p.rest
	ed.sfx(sndTick, p.rest, 0.3)
}

func newPlug() *miniPlug {
	q := &miniPlug{caller: -1, wait: 0.5, lastLine: -1}
	for i := range q.plugs {
		q.plugs[i] = &swPlug{q: q, i: i, in: -1}
	}
	return q
}

// plugged: a plug's gone into a line jack.
func (q *miniPlug) plugged(p *swPlug, j int) {
	g := q.g
	if q.caller < 0 || g == nil {
		p.backT = 0.6 // nobody calling: it doesn't stay
		return
	}
	if p.i == q.caller && j == q.line {
		q.done++
		ed.sfx(sndPowerUp, q.jacks[j], 0.4)
		p.backT = 1.0
		q.caller, q.wait = -1, plugCallGap
		if q.done >= plugCalls {
			g.r.CompleteModule()
		}
		return
	}
	q.miss(g, p.pos)
	p.backT = 0.5
}

func (q *miniPlug) miss(g *game, at vec3) {
	q.misses++
	q.failFlas = 0.6
	ed.sfx(sndAlarm, at, 0.35)
	if q.misses >= plugMisses {
		g.r.Setback(g.r.T.Setback, "plug")
		return
	}
	g.say("LOST THAT CALL", "ONE MORE AND IT STARTS OVER", cDanger, 2)
	q.caller, q.wait = -1, plugCallGap
}

func (q *miniPlug) frame(m *modGame, g *game, c, n, side, up vec3, dt float64) {
	q.g, q.n = g, n
	for j := range q.jacks {
		q.jacks[j] = c.add(side.mul(-0.105 + 0.07*float64(j))).add(up.mul(0.0)).add(n.mul(0.02))
	}
	for i, p := range q.plugs {
		p.ok = func() bool { return m.open > 0.9 }
		p.rest = c.add(side.mul(-0.105 + 0.07*float64(i))).add(up.mul(-0.075)).add(n.mul(0.03))
		p.anchor = c.add(side.mul(-0.105 + 0.07*float64(i))).add(up.mul(-0.105)).add(n.mul(0.005))
		if !p.held && p.in < 0 {
			p.pos = p.rest
		}
		if p.in >= 0 {
			p.pos = q.jacks[p.in].add(n.mul(0.012))
			if p.backT > 0 {
				p.backT -= dt
				if p.backT <= 0 {
					p.in, p.pos = -1, p.rest
					ed.sfx(sndTick, p.rest, 0.3)
				}
			}
		}
		if p.held {
			q.started = true
		}
	}
	q.failFlas -= dt
	if m.open < 0.9 || q.done >= plugCalls {
		return
	}
	if q.caller < 0 {
		q.wait -= dt
		if q.wait <= 0 {
			// a new call: a caller and a line, neither the plug that's still in somewhere
			q.caller = rand.Intn(4)
			for k := 0; k < 8 && q.plugs[q.caller].in >= 0; k++ {
				q.caller = rand.Intn(4)
			}
			q.line = rand.Intn(4)
			q.ring = plugTime
			ed.sfx(sndTone2, c, 0.4)
		}
	} else if q.started {
		q.ring -= dt // the clock only runs once you've picked up a cord
		if q.ring <= 0 {
			q.miss(g, c)
			return
		}
	}
	p := float64(q.done) / plugCalls
	g.r.SetModuleProgress(p)
}

func (q *miniPlug) draw(m *modGame, g *game, c, n, side, up vec3) {
	t := fxTime()
	blink := func(rate float64) bool { return math.Mod(t*rate, 1) < 0.6 }
	rate := 2.0
	if q.caller >= 0 && q.ring < 4 {
		rate = 6 // hurry
	}
	for i := 0; i < 4; i++ {
		// the caller lamps (one over each cord) and the line jacks with their lamps
		x := -0.105 + 0.07*float64(i)
		cl := c.add(side.mul(x)).add(up.mul(-0.045)).add(n.mul(0.025))
		lc := plugCols[i].scale(0.25)
		if q.caller == i && blink(rate) {
			lc = plugCols[i]
			sprite(cl, 0.035, plugCols[i].alpha(0.7))
		}
		plugLamp(cl, n, lc)
		faceText(&bOpaque, itoa(i+1), cl.add(side.mul(0.016)).sub(n.mul(0.004)), n, up, 0.0008, plugCols[i])
		jp := q.jacks[i]
		ll := jp.add(up.mul(0.035)).add(n.mul(0.005))
		lc = rgba{0.2, 0.2, 0.2, 1}
		if q.caller >= 0 && q.line == i && blink(rate) {
			lc = plugCols[q.caller]
			sprite(ll, 0.035, lc.alpha(0.7))
		}
		plugLamp(ll, n, lc)
		useMat(mChrome)
		cyl(&bEnv, jp.sub(n.mul(0.01)), jp, 0.011, 0.011, 12, rgba{0.75, 0.75, 0.72, 1}, true)
		cyl(&bEnv, jp.sub(n.mul(0.004)), jp.add(n.mul(0.0005)), 0.005, 0.005, 10, rgba{0.02, 0.02, 0.02, 1}, true)
		faceText(&bOpaque, string(rune('A'+i)), jp.add(up.mul(-0.02)).sub(n.mul(0.004)), n, up, 0.0009, cWhite)
	}
	for _, p := range q.plugs {
		col := plugCols[p.i]
		mid := p.anchor.add(p.pos).mul(0.5).add(up.mul(-0.03)).add(n.mul(0.02))
		useMat(mRubber)
		hose(&bEnv, p.anchor, p.anchor.add(n.mul(0.03)), mid, p.pos.sub(n.mul(0.01)), 0.0035, col.scale(0.8), 10)
		useMat(mChrome)
		cyl(&bEnv, p.pos.add(n.mul(0.02)), p.pos.sub(n.mul(0.004)), 0.007, 0.0045, 10, rgba{0.8, 0.8, 0.78, 1}, true)
		useMat(mRubber)
		cyl(&bEnv, p.pos.add(n.mul(0.055)), p.pos.add(n.mul(0.02)), 0.011, 0.01, 12, col, true)
	}
	msg := "PLUG THE LIT CORD INTO THE LIT LINE"
	if q.caller >= 0 {
		msg = "CALL " + itoa(q.caller+1) + " WANTS LINE " + string(rune('A'+q.line))
	}
	if !q.started {
		msg += " - GRAB A PLUG"
	}
	miniTitle(c, n, up, msg)
	if q.failFlas > 0 {
		sprite(c.add(n.mul(0.04)), 0.25, cDanger.alpha(float32(0.4*q.failFlas)))
	}
	miniBar(c, n, side, up, float64(q.done)/plugCalls)
}

func plugLamp(p, n vec3, col rgba) {
	useMat(mCast)
	cyl(&bEnv, p.sub(n.mul(0.01)), p.sub(n.mul(0.004)), 0.01, 0.01, 12, cIron, true)
	curEmis = 2
	useMat(mPaint)
	cyl(&bEnv, p.sub(n.mul(0.004)), p, 0.007, 0.007, 12, col, true)
	curEmis = 1
}

func (q *miniPlug) grabbers(out []grabber) []grabber {
	for _, p := range q.plugs {
		out = append(out, p)
	}
	return out
}
func (q *miniPlug) pokers(out []poker) []poker { return out }

// ---- valves: balance the pressures ----

const (
	vlLo, vlHi = 0.45, 0.62 // the green band
	vlHold     = 3.0        // seconds all three must stay in it
	vlBurst    = 0.93       // over this the gauge is in the red
)

type miniValves struct {
	kn      [3]*knob
	p, off  [3]float64
	hold    float64
	over    float64
	started bool
	t       float64
}

func newValves() *miniValves {
	m := &miniValves{}
	for i := range m.kn {
		m.kn[i] = &knob{r: 0.026, lo: 0, hi: 3 * math.Pi, detent: math.Pi / 12}
		m.off[i] = (rand.Float64()*2 - 1) * 0.08
		m.p[i] = 0.2 + m.off[i]
	}
	return m
}

// opening: how far valve i's open (0..1).
func (q *miniValves) opening(i int) float64 { return q.kn[i].ang / (3 * math.Pi) }

// want: where gauge i is heading: its own valve raises it, the one before it bleeds it.
func (q *miniValves) want(i int) float64 {
	drift := 0.0
	if q.started {
		drift = 0.03 * math.Sin(0.4*q.t+float64(i)*2.1)
	}
	return math.Max(0, math.Min(1, 0.2+q.opening(i)-0.45*q.opening((i+2)%3)+q.off[i]+drift))
}

func (q *miniValves) inBand(i int) bool { return q.p[i] >= vlLo && q.p[i] <= vlHi }

func (q *miniValves) frame(m *modGame, g *game, c, n, side, up vec3, dt float64) {
	for i, k := range q.kn {
		k.ok = func() bool { return m.open > 0.9 }
		k.c, k.ax = c.add(side.mul(-0.09+0.09*float64(i))).add(up.mul(-0.07)).add(n.mul(0.03)), n
		q.started = q.started || k.held
	}
	if q.started {
		q.t += dt
	}
	all, n3, hot := true, 0, false
	for i := range q.p {
		q.p[i] += (q.want(i) - q.p[i]) * math.Min(1, dt*1.5)
		if q.inBand(i) {
			n3++
		} else {
			all = false
		}
		hot = hot || q.p[i] > vlBurst
	}
	if hot {
		q.over += dt
		if math.Mod(q.over, 0.4) < dt {
			ed.sfx(sndAlarm, c, 0.25)
		}
		if q.over > 1.5 {
			g.r.Setback(g.r.T.Setback, "valves")
			return
		}
	} else {
		q.over = 0
	}
	if all {
		was := q.hold
		q.hold += dt
		if math.Floor(was) != math.Floor(q.hold) {
			ed.sfx(sndTick, c, 0.5)
		}
		if q.hold >= vlHold {
			g.r.CompleteModule()
			return
		}
	} else {
		q.hold = 0
	}
	g.r.SetModuleProgress(0.3*float64(n3)/3 + 0.7*q.hold/vlHold)
}

func (q *miniValves) draw(m *modGame, g *game, c, n, side, up vec3) {
	for i, k := range q.kn {
		// the gauge over each valve: a green band, a red end
		gc := c.add(side.mul(-0.09 + 0.09*float64(i))).add(up.mul(0.03)).add(n.mul(0.02))
		useMat(mCast)
		cyl(&bEnv, gc.sub(n.mul(0.01)), gc.sub(n.mul(0.001)), 0.038, 0.038, 20, cIron, true)
		bTrans.quadC(gc.add(side.mul(-0.032)).add(up.mul(-0.03)), gc.add(side.mul(0.032)).add(up.mul(-0.03)), gc.add(side.mul(0.032)).add(up.mul(0.034)), gc.add(side.mul(-0.032)).add(up.mul(0.034)),
			rgba{0.85, 0.83, 0.75, 1}, rgba{0.85, 0.83, 0.75, 1}, rgba{0.9, 0.88, 0.8, 1}, rgba{0.9, 0.88, 0.8, 1})
		piv := gc.add(up.mul(-0.018)).add(n.mul(0.001))
		ang := func(v float64) float64 { return (v*2 - 1) * 1.1 }
		for s := 0; s <= 20; s++ {
			v := float64(s) / 20
			d := up.mul(math.Cos(ang(v))).add(side.mul(math.Sin(ang(v))))
			col := rgba{0.25, 0.25, 0.25, 1}
			switch {
			case v >= vlLo && v <= vlHi:
				col = rgba{0.1, 0.55, 0.15, 1}
			case v > vlBurst:
				col = rgba{0.75, 0.1, 0.05, 1}
			}
			flatLine(&bOpaque, piv.add(d.mul(0.038)), piv.add(d.mul(0.046)), n, 0.0025, col)
		}
		d := up.mul(math.Cos(ang(q.p[i]))).add(side.mul(math.Sin(ang(q.p[i]))))
		ribbon(piv, piv.add(d.mul(0.043)), 0.002, rgba{0.8, 0.1, 0.05, 1})
		if q.inBand(i) {
			sprite(gc.add(up.mul(0.045)), 0.02, cEnergy.alpha(0.8))
		}
		// the valve: a spoked handwheel
		useMat(mPaint)
		wc := rgba{0.6, 0.12, 0.08, 1}
		for s := 0; s < 3; s++ {
			a := k.ang + float64(s)*2*math.Pi/3
			dd := rotAbout(up, n, a)
			cyl(&bEnv, k.c, k.c.add(dd.mul(k.r)), 0.003, 0.003, 6, wc, false)
		}
		const segs = 16
		for s := 0; s < segs; s++ {
			a0, a1 := float64(s)*2*math.Pi/segs, float64(s+1)*2*math.Pi/segs
			cyl(&bEnv, k.c.add(rotAbout(up, n, a0).mul(k.r)), k.c.add(rotAbout(up, n, a1).mul(k.r)), 0.0045, 0.0045, 6, wc, false)
		}
		useMat(mCast)
		cyl(&bEnv, k.c.sub(n.mul(0.02)), k.c, 0.007, 0.007, 8, cIron, true)
	}
	msg := "ALL THREE IN THE GREEN, AND HOLD"
	switch {
	case !q.started:
		msg = "TURN THE VALVES. EACH ONE BLEEDS THE NEXT GAUGE"
	case q.over > 0:
		msg = "TOO MUCH PRESSURE!"
	case q.hold > 0:
		msg = "HOLD IT... " + itoa(int(math.Ceil(vlHold-q.hold)))
	}
	miniTitle(c, n, up, msg)
	miniBar(c, n, side, up, q.hold/vlHold)
}

func (q *miniValves) grabbers(out []grabber) []grabber {
	for _, k := range q.kn {
		out = append(out, k)
	}
	return out
}
func (q *miniValves) pokers(out []poker) []poker { return out }

// ---- look: the code's on the other side ----

const lookLen = 5

type miniLook struct {
	btn     [6]*pushBtn
	code    [lookLen]int
	got     int
	changed float64 // the code just changed (a flash on both screens)
	rise    float64 // the far screen coming up out of the cart's top
}

func newLook() *miniLook {
	m := &miniLook{}
	m.shuffle()
	return m
}

func (q *miniLook) shuffle() {
	for i := range q.code {
		for {
			v := rand.Intn(6)
			if i == 0 || v != q.code[i-1] {
				q.code[i] = v
				break
			}
		}
	}
	q.got = 0
}

func (q *miniLook) press(g *game, s int) {
	if q.got >= lookLen {
		return
	}
	if s == q.code[q.got] {
		q.got++
		ed.sfx(sndTone1, q.btn[s].c, 0.4)
		if q.got >= lookLen {
			ed.sfx(sndPowerUp, q.btn[s].c, 0.5)
			g.r.CompleteModule()
		}
		return
	}
	// wrong: a new code, and you'll have to go round and read it again
	ed.sfx(sndAlarm, q.btn[s].c, 0.4)
	q.shuffle()
	q.changed = 1.2
	g.say("WRONG SYMBOL", "THE CODE CHANGED - GO ROUND AND READ IT AGAIN", cDanger, 3)
}

// farScreen: the code screen, up out of the cart's top at the far (+x) edge, facing away.
func (q *miniLook) farScreen(g *game) (vec3, vec3, vec3, vec3) {
	F := g.devFrame()
	lift := 0.11 * smooth(q.rise)
	return F.p(devHW-0.02, devTop-0.06+lift, 0.05), F.x, F.z.mul(-1), F.y
}

func (q *miniLook) frame(m *modGame, g *game, c, n, side, up vec3, dt float64) {
	for i := range q.btn {
		if q.btn[i] == nil {
			i := i
			q.btn[i] = &pushBtn{r: 0.022}
			q.btn[i].onPress = func() { q.press(g, i) }
		}
		b := q.btn[i]
		b.ok = func() bool { return m.open > 0.9 && q.got < lookLen }
		b.c = c.add(side.mul(-0.075 + 0.075*float64(i%3))).add(up.mul(-0.005 - 0.07*float64(i/3))).add(n.mul(0.03))
		b.n = n
		b.tick(dt)
	}
	q.rise = math.Min(1, q.rise+dt*0.8)
	q.changed -= dt
	g.r.SetModuleProgress(float64(q.got) / lookLen)
}

// lookSymbol: one of the six symbols, drawn as lines in the plane (r, u).
func lookSymbol(s int, c, r, u vec3, sz float64, line func(a, b vec3)) {
	at := func(x, y float64) vec3 { return c.add(r.mul(x * sz)).add(u.mul(y * sz)) }
	poly := func(pts [][2]float64) {
		for i := range pts {
			a, b := pts[i], pts[(i+1)%len(pts)]
			line(at(a[0], a[1]), at(b[0], b[1]))
		}
	}
	switch s {
	case 0: // circle
		var pts [][2]float64
		for i := 0; i < 14; i++ {
			a := float64(i) / 14 * 2 * math.Pi
			pts = append(pts, [2]float64{math.Cos(a), math.Sin(a)})
		}
		poly(pts)
	case 1: // triangle
		poly([][2]float64{{0, 1}, {0.95, -0.75}, {-0.95, -0.75}})
	case 2: // square
		poly([][2]float64{{-0.85, -0.85}, {0.85, -0.85}, {0.85, 0.85}, {-0.85, 0.85}})
	case 3: // cross
		line(at(-0.9, -0.9), at(0.9, 0.9))
		line(at(-0.9, 0.9), at(0.9, -0.9))
	case 4: // diamond
		poly([][2]float64{{0, 1}, {0.8, 0}, {0, -1}, {-0.8, 0}})
	default: // three bars
		for _, y := range []float64{-0.7, 0, 0.7} {
			line(at(-0.9, y), at(0.9, y))
		}
	}
}

func (q *miniLook) draw(m *modGame, g *game, c, n, side, up vec3) {
	t := fxTime()
	for i, b := range q.btn {
		if b == nil {
			continue
		}
		useMat(mPaint)
		bevelBox(&bEnv, b.c.sub(n.mul(0.01+0.005*b.press)), side, up, n, 0.026, 0.026, 0.01, 0.005, rgba{0.16, 0.17, 0.19, 1})
		face := b.c.sub(n.mul(0.0045 * b.press)).add(n.mul(0.0012))
		lookSymbol(i, face, side, up, 0.012, func(a, bb vec3) { flatLine(&bOpaque, a, bb, n, 0.002, cBeam) })
	}
	// what you've keyed so far
	for k := 0; k < lookLen; k++ {
		p := c.add(side.mul(-0.1 + 0.05*float64(k))).add(up.mul(0.065)).add(n.mul(0.022))
		miniScreen(p, side, up, 0.019, 0.016)
		if k < q.got {
			lookSymbol(q.code[k], p.add(n.mul(0.002)), side, up, 0.009, func(a, bb vec3) { glowLine(a, bb, cEnergy, 0.3) })
		} else if k == q.got && math.Mod(t*2, 1) < 0.5 {
			glowLine(p.add(side.mul(-0.008)).add(up.mul(-0.01)), p.add(side.mul(0.008)).add(up.mul(-0.01)), cAmber, 0.3)
		}
	}
	msg := "THE CODE'S ON THE OTHER SIDE OF THE CART"
	if q.changed > 0 {
		msg = "THE CODE CHANGED: GO AND LOOK AGAIN"
	}
	miniTitle(c, n, up, msg)
	miniBar(c, n, side, up, float64(q.got)/lookLen)
	// the far screen
	sc, sn, sr, su := q.farScreen(g)
	useMat(mPaint)
	bevelBox(&bEnv, sc.sub(sn.mul(0.008)), sr, su, sn, 0.1, 0.055, 0.007, 0.004, rgba{0.1, 0.11, 0.12, 1})
	cyl(&bEnv, sc.add(su.mul(-0.055)).sub(sn.mul(0.008)), sc.add(su.mul(-0.12)).sub(sn.mul(0.008)), 0.008, 0.008, 8, cSteelEnv, false)
	miniScreen(sc, sr, su, 0.092, 0.047)
	faceText(&bOpaque, "CODE", sc.add(su.mul(0.035)).add(sn.mul(0.002)), sn, su, 0.0009, cMuted)
	flick := float32(0.8 + 0.2*noise1(9, t*7))
	if q.changed > 0 && math.Mod(t*6, 1) < 0.5 {
		flick = 0.2
	}
	for k := 0; k < lookLen; k++ {
		p := sc.add(sr.mul(-0.072 + 0.036*float64(k))).add(su.mul(-0.006)).add(sn.mul(0.002))
		col := cBeam
		if k < q.got {
			col = cEnergy.alpha(0.5)
		}
		lookSymbol(q.code[k], p, sr, su, 0.013, func(a, bb vec3) { glowLine(a, bb, col.alpha(col.a*flick), 0.4) })
	}
	// on the back, facing the hatch: where to look
	faceText(&bOpaque, "CODE ON THE OTHER SIDE", sc.sub(sn.mul(0.0165)), sn.mul(-1), su, 0.0008, cAmber)
}

func (q *miniLook) grabbers(out []grabber) []grabber { return out }
func (q *miniLook) pokers(out []poker) []poker {
	for _, b := range q.btn {
		if b != nil {
			out = append(out, b)
		}
	}
	return out
}

var _ = sim.V
