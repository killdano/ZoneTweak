package main

// Physical controls: levers, sliders, cranks, knobs, buttons, pull cords and props. Every
// machine in the game (generators, the device, the harpoon, the radio) is built from these.
// Each keeps its own state; the owner places it in the world every frame (mount frames move
// with the cart, the turret and so on) and reads its values.

import "math"

func always() bool { return true }

func projPerp(v, ax vec3) vec3 { return v.sub(ax.mul(v.dot(ax))) }

// signedAngle: the angle from a to b around axis ax.
func signedAngle(a, b, ax vec3) float64 {
	return math.Atan2(a.cross(b).dot(ax), a.dot(b))
}

// ---- lever ----

type lever struct {
	piv, ax, d0 vec3    // pivot, rotation axis, arm direction at angle 0
	L           float64 // arm length
	a0, a1      float64 // travel
	ang         float64
	ok          func() bool
	onEnd       func(end int)
	atEnd       int
	held        bool
}

func (l *lever) handle() vec3  { return l.piv.add(rotAbout(l.d0, l.ax, l.ang).mul(l.L)) }
func (l *lever) gPos() vec3    { return l.handle() }
func (l *lever) gRad() float64 { return 0.1 }
func (l *lever) gOK() bool {
	return l.ok == nil || l.ok()
}
func (l *lever) gGrab(h *gHand)    { l.held = true }
func (l *lever) gRelease(h *gHand) { l.held = false }
func (l *lever) gHold(h *gHand, dt float64) {
	v := projPerp(h.palm.sub(l.piv), l.ax)
	if v.length() < 1e-4 {
		return
	}
	a := signedAngle(l.d0, v.norm(), l.ax)
	a = math.Max(l.a0, math.Min(l.a1, a))
	l.ang = a
	end := -1
	switch {
	case a <= l.a0+0.03:
		end = 0
	case a >= l.a1-0.03:
		end = 1
	}
	if end >= 0 && end != l.atEnd {
		h.buzz(40, 0.8)
		ed.sfx(sndRelay, l.handle(), 0.5)
		if l.onEnd != nil {
			l.onEnd(end)
		}
	}
	if end >= 0 {
		l.atEnd = end
	} else if l.atEnd >= 0 && math.Abs(a-[]float64{l.a0, l.a1}[l.atEnd]) > 0.15 {
		l.atEnd = -1
	}
}

// value: 0 at a0, 1 at a1
func (l *lever) value() float64 { return (l.ang - l.a0) / (l.a1 - l.a0) }

// ---- slider (the pump handle) ----

type slider struct {
	base, dir vec3 // dir: the way a push goes
	L         float64
	val       float64 // 0..1 along dir
	phase     int
	ok        func() bool
	onStroke  func()
	held      bool
}

func (s *slider) handle() vec3  { return s.base.add(s.dir.mul(s.val * s.L)) }
func (s *slider) gPos() vec3    { return s.handle() }
func (s *slider) gRad() float64 { return 0.1 }
func (s *slider) gOK() bool {
	return s.ok == nil || s.ok()
}
func (s *slider) gGrab(h *gHand)    { s.held = true }
func (s *slider) gRelease(h *gHand) { s.held = false }
func (s *slider) gHold(h *gHand, dt float64) {
	s.val = math.Max(0, math.Min(1, h.palm.sub(s.base).dot(s.dir)/s.L))
	if s.phase == 0 && s.val > 0.85 {
		s.phase = 1
		h.buzz(30, 0.7)
		if s.onStroke != nil {
			s.onStroke()
		}
	}
	if s.phase == 1 && s.val < 0.2 {
		s.phase = 0
		h.buzz(10, 0.3)
	}
}

// spring: an untouched slider eases back to the top
func (s *slider) tick(dt float64) {
	if !s.held {
		s.val = math.Max(0, s.val-dt*1.5)
		if s.val < 0.2 {
			s.phase = 0
		}
	}
}

// ---- crank ----

type crank struct {
	c, ax, ref vec3 // centre, axle, where angle 0 points
	R          float64
	ang        float64
	turns      float64 // signed, accumulated
	ok         func() bool
	held       bool
	lastA      float64
}

func (k *crank) handle() vec3 {
	return k.c.add(rotAbout(k.ref, k.ax, k.ang).mul(k.R)).add(k.ax.mul(0.05))
}
func (k *crank) gPos() vec3    { return k.handle() }
func (k *crank) gRad() float64 { return 0.1 }
func (k *crank) gOK() bool {
	return k.ok == nil || k.ok()
}
func (k *crank) gGrab(h *gHand) {
	k.held = true
	k.lastA = k.ang
}
func (k *crank) gRelease(h *gHand) { k.held = false }
func (k *crank) gHold(h *gHand, dt float64) {
	v := projPerp(h.palm.sub(k.c), k.ax)
	if v.length() < 0.02 {
		return
	}
	a := signedAngle(k.ref, v.norm(), k.ax)
	d := angDiff(k.ang, a)
	k.ang = a
	k.turns += d / (2 * math.Pi)
	if math.Floor((k.turns-d/(2*math.Pi))*4) != math.Floor(k.turns*4) {
		h.buzz(8, 0.4)
		ed.sfx(sndTick, k.c, 0.25)
	}
}

// ---- knob (twist your wrist) ----

type knob struct {
	c, ax     vec3
	r         float64 // knob radius (for drawing and reach)
	ang       float64
	lo, hi    float64 // limits (use -inf/+inf for none)
	ok        func() bool
	held      bool
	ref       vec3
	detent    float64 // radians between clicks (0 = smooth)
	lastClick float64
}

func (k *knob) gPos() vec3 { return k.c }
func (k *knob) gRad() float64 {
	return math.Max(0.08, k.r+0.05)
}
func (k *knob) gOK() bool {
	return k.ok == nil || k.ok()
}
func (k *knob) handRef(h *gHand) vec3 {
	X, Y, _ := h.axes()
	v := projPerp(X, k.ax)
	if v.length() < 0.3 {
		v = projPerp(Y, k.ax)
	}
	return v.norm()
}
func (k *knob) gGrab(h *gHand) {
	k.held = true
	k.ref = k.handRef(h)
}
func (k *knob) gRelease(h *gHand) { k.held = false }
func (k *knob) gHold(h *gHand, dt float64) {
	cur := k.handRef(h)
	d := signedAngle(k.ref, cur, k.ax)
	k.ref = cur
	k.ang = math.Max(k.lo, math.Min(k.hi, k.ang+d))
	if k.detent > 0 && math.Abs(k.ang-k.lastClick) >= k.detent {
		k.lastClick = math.Round(k.ang/k.detent) * k.detent
		h.buzz(6, 0.4)
		ed.sfx(sndTick, k.c, 0.2)
	}
}

// ---- push button (poked with a fingertip) ----

type pushBtn struct {
	c, n    vec3
	r       float64
	ok      func() bool
	onPress func()
	press   float64 // animation 1 -> 0
	cool    float64
	armT    float64 // when a fingertip was last just in front of it (going in soon after is a press)
}

// pokeR: the fingertip as a ball (it's the pad of your finger that meets the button)
const pokeR = 0.009

func (b *pushBtn) pokeOK() bool {
	return (b.ok == nil || b.ok()) && b.cool <= 0
}
func (b *pushBtn) pokeTest(prev, tip vec3) bool {
	// the finger pad's depth into the face (negative: in) and how far off centre it is
	d0, d1 := prev.sub(b.c).dot(b.n)-pokeR, tip.sub(b.c).dot(b.n)-pokeR
	lat := func(p vec3) float64 { return projPerp(p.sub(b.c), b.n).length() }
	l1 := lat(tip)
	now := fxTime()
	if d1 > 0.002 && d1 < 0.08 && l1 < b.r*1.6+0.03 {
		b.armT = now // a fingertip in front of it, near enough: ready to press
	}
	armed := now-b.armT < 0.5
	if d1 >= -0.002 || !(armed || d0 >= -0.002) {
		return false
	}
	// where it went through the face (a quick jab can be well past it by the next frame)
	lc := l1
	if d0 > d1 {
		u := math.Max(0, math.Min(1, d0/(d0-d1)))
		lc = lat(prev.add(tip.sub(prev).mul(u)))
	}
	// a generous button, and only a short debounce so you can tap fast
	if math.Min(lc, l1) < b.r*1.35 {
		b.armT = -1
		b.press, b.cool = 1, pokeDebounce
		ed.sfx(sndTick, b.c, 0.4)
		if b.onPress != nil {
			b.onPress()
		}
		return true
	}
	return false
}

// pokeDebounce: the shortest time between two presses of one button (tracking jitter)
const pokeDebounce = 0.06

func (b *pushBtn) tick(dt float64) {
	b.press = math.Max(0, b.press-dt*5)
	b.cool -= dt
}

// ---- pull cord (the rip cord) ----

type pullCord struct {
	a, dir  vec3 // where the cord leaves the housing; its rest direction (up, out of it)
	pull    vec3 // the way it's being pulled now: any way you like but down
	ext     float64
	maxExt  float64
	ok      func() bool
	onPull  func(speed float64)
	held    bool
	armed   bool
	prevExt float64
	speed   float64
}

// cordMinUp: how far above level a pull has to stay (the cord won't go down into the cart)
const cordMinUp = 0.1

func (p *pullCord) way() vec3 {
	if p.pull.length() < 0.5 {
		return p.dir
	}
	return p.pull
}
func (p *pullCord) handle() vec3  { return p.a.add(p.way().mul(p.ext + 0.03)) }
func (p *pullCord) gPos() vec3    { return p.handle() }
func (p *pullCord) gRad() float64 { return 0.1 }
func (p *pullCord) gOK() bool {
	return p.ok == nil || p.ok()
}
func (p *pullCord) gGrab(h *gHand) {
	p.held, p.armed = true, true
}
func (p *pullCord) gRelease(h *gHand) { p.held = false }
func (p *pullCord) gHold(h *gHand, dt float64) {
	p.prevExt = p.ext
	v := h.palm.sub(p.a)
	if v.length() > 0.02 {
		d := v.norm()
		if up := d.dot(p.dir); up < cordMinUp {
			// level-ish at most: lift it back up to just above level
			flat := projPerp(d, p.dir)
			if flat.length() < 1e-3 {
				flat = projPerp(v3(1, 0, 0), p.dir)
			}
			d = flat.norm().mul(math.Sqrt(1 - cordMinUp*cordMinUp)).add(p.dir.mul(cordMinUp))
		}
		p.pull = d
	}
	p.ext = math.Max(0, math.Min(p.maxExt, v.dot(p.way())-0.03))
	if dt > 0 {
		p.speed = p.speed*0.5 + 0.5*(p.ext-p.prevExt)/dt
	}
	h.buzz(5, float32(math.Min(1, 0.2+p.ext)))
	if p.armed && p.ext > p.maxExt*0.8 {
		p.armed = false
		if p.onPull != nil {
			p.onPull(p.speed)
		}
	}
	if p.ext < 0.08 {
		p.armed = true
	}
}
func (p *pullCord) tick(dt float64) {
	if !p.held {
		p.ext = math.Max(0, p.ext-dt*3)
		if p.ext < 0.08 {
			p.armed = true
			p.pull = vec3{} // wound back in: it sits upright again
		}
	}
}

// ---- prop: something you pick up and carry ----

type prop struct {
	pos       vec3
	X, Y, Z   vec3 // orientation
	held      bool
	offP      vec3 // grab offset in the hand frame
	offX      vec3
	offY      vec3
	offZ      vec3
	vel       vec3
	resting   bool
	home      func() (vec3, vec3, vec3, vec3) // holster pose (pos, X, Y, Z); nil = none
	homeR     float64
	docked    bool
	anchor    func() vec3 // cord anchor (nil = no cord)
	cordLen   float64
	radius    float64 // grab reach around pos
	ok        func() bool
	onRelease func()
	hy        float64  // how high its middle sits above the floor when lying down
	grip      *[4]vec3 // a tool's fixed hold in the hand frame (position, X, Y, Z); nil = as grabbed
	onCart    bool     // lying on the cart's lid (it rides along with the cart)
	cartRel   [4]vec3  // ...where, in the cart's frame (position, X, Y, Z)
}

// cartLid: the top of the cart's housing as somewhere dropped things can land (its frame,
// half extents and lid height in that frame); nil when there's no cart. Its bottom shelf (a
// tray) catches things too.
var cartLid func() (F xf, hx, hz, top float64, ok bool)

const cartShelfY, cartShelfHX, cartShelfHZ = 0.168, 0.255, 0.355

// propSupport: the top of whatever else stands under p that a dropped thing can come to rest
// on (if it was above it a moment ago), and a nudge sideways off anything solid it fell into.
var propSupport func(was, p vec3) (top float64, push vec3, ok bool)

func inXf(F xf, v vec3) vec3 { return v3(v.dot(F.x), v.dot(F.y), v.dot(F.z)) }
func outXf(F xf, v vec3) vec3 {
	return F.x.mul(v.x).add(F.y.mul(v.y)).add(F.z.mul(v.z))
}

func (p *prop) gPos() vec3 { return p.pos }
func (p *prop) gRad() float64 {
	return math.Max(0.07, p.radius)
}
func (p *prop) gOK() bool {
	return p.ok == nil || p.ok()
}

func handXf(h *gHand) xf {
	X, Y, Z := h.axes()
	return xf{h.palm, X, Y, Z}
}

func (p *prop) gGrab(h *gHand) {
	F := handXf(h)
	d := p.pos.sub(F.o)
	p.offP = v3(d.dot(F.x), d.dot(F.y), d.dot(F.z))
	loc := func(v vec3) vec3 { return v3(v.dot(F.x), v.dot(F.y), v.dot(F.z)) }
	p.offX, p.offY, p.offZ = loc(p.X), loc(p.Y), loc(p.Z)
	// pull the prop into the palm a little so it doesn't float off the glove
	p.offP = p.offP.mul(0.4)
	if p.grip != nil {
		// tools sit in the hand the way you hold them
		p.offP, p.offX, p.offY, p.offZ = p.grip[0], p.grip[1], p.grip[2], p.grip[3]
	}
	p.held, p.docked, p.resting, p.onCart = true, false, false, false
}

func (p *prop) gHold(h *gHand, dt float64) {
	F := handXf(h)
	w := func(v vec3) vec3 { return F.x.mul(v.x).add(F.y.mul(v.y)).add(F.z.mul(v.z)) }
	np := F.o.add(w(p.offP))
	if dt > 0 {
		p.vel = np.sub(p.pos).mul(1 / dt)
	}
	p.pos = np
	p.X, p.Y, p.Z = w(p.offX), w(p.offY), w(p.offZ)
}

func (p *prop) gRelease(h *gHand) {
	p.held = false
	if p.home != nil {
		hp, _, _, _ := p.home()
		if hp.sub(p.pos).length() < p.homeR {
			p.docked = true
		}
	}
	if p.onRelease != nil {
		p.onRelease()
	}
}

// tick: docked props sit in their holster; dropped ones fall; corded ones stay on the cord.
func (p *prop) tick(dt float64) {
	if p.held {
		return
	}
	if p.docked && p.home != nil {
		p.pos, p.X, p.Y, p.Z = p.home()
		p.vel = vec3{}
		return
	}
	if p.resting && p.onCart {
		// lying on the cart: it goes where the cart goes
		if cartLid == nil {
			p.resting, p.onCart = false, false
		} else if F, _, _, _, ok := cartLid(); ok {
			p.pos = F.o.add(outXf(F, p.cartRel[0]))
			p.X, p.Y, p.Z = outXf(F, p.cartRel[1]), outXf(F, p.cartRel[2]), outXf(F, p.cartRel[3])
		} else {
			p.resting, p.onCart = false, false
		}
	}
	if !p.resting {
		p.vel = p.vel.add(v3(0, -9.8*dt, 0)).mul(math.Max(0, 1-dt*0.8))
		old := p.pos
		p.pos = p.pos.add(p.vel.mul(dt))
		// the ground under it: the floor, or the cart's lid if it's coming down onto it
		ground, lid := p.hy, false
		var LF xf
		if cartLid != nil {
			if F, hx, hz, top, ok := cartLid(); ok {
				q := inXf(F, p.pos.sub(F.o))
				was := inXf(F, old.sub(F.o))
				if math.Abs(q.x) < hx+0.02 && math.Abs(q.z) < hz+0.02 && was.y >= top-0.03 {
					ground, lid, LF = F.o.y+top+p.hy, true, F
				} else if math.Abs(q.x) < cartShelfHX && math.Abs(q.z) < cartShelfHZ && was.y >= cartShelfY-0.03 && was.y < top-0.3 {
					ground, lid, LF = F.o.y+cartShelfY+p.hy, true, F // in the tray on the bottom shelf
				}
			}
		}
		if !lid && propSupport != nil {
			if top, push, ok := propSupport(old, p.pos); ok {
				if top+p.hy > ground {
					ground = top + p.hy
				}
				p.pos = p.pos.add(push)
			}
		}
		if p.pos.y < ground {
			p.pos.y = ground
			p.vel = p.vel.mul(0.3)
			p.vel.y = -p.vel.y * 0.2
			if p.vel.length() < 0.15 {
				p.resting, p.vel = true, vec3{}
				// lie down
				p.Y = v3(0, 1, 0)
				p.Z = projPerp(p.Z, p.Y)
				if p.Z.length() < 0.1 {
					p.Z = v3(0, 0, 1)
				}
				p.Z = p.Z.norm()
				p.X = p.Y.cross(p.Z).norm()
				if lid {
					p.onCart = true
					p.cartRel = [4]vec3{inXf(LF, p.pos.sub(LF.o)), inXf(LF, p.X), inXf(LF, p.Y), inXf(LF, p.Z)}
				}
			}
		}
	}
	if p.anchor != nil {
		a := p.anchor()
		d := p.pos.sub(a)
		if L := d.length(); L > p.cordLen {
			p.pos = a.add(d.mul(p.cordLen / L))
			p.vel = p.vel.mul(0.5)
			p.resting, p.onCart = false, false
			if p.pos.y <= p.hy+0.001 {
				p.resting = true
			}
		}
	}
}

// cordPts: a hanging cord from a to b (sag grows with slack), n segments.
func cordPts(a, b vec3, length float64, n int) []vec3 {
	L := b.sub(a).length()
	slack := math.Max(0, length-L)
	sag := math.Min(0.6, slack*0.5)
	pts := make([]vec3, n+1)
	for k := 0; k <= n; k++ {
		u := float64(k) / float64(n)
		p := a.add(b.sub(a).mul(u))
		p.y -= sag * 4 * u * (1 - u)
		if p.y < 0.01 {
			p.y = 0.01
		}
		pts[k] = p
	}
	return pts
}

// cordBox: a solid box a cord can't pass through: x within hx, y from y0 to y1, z within
// hz, in frame F.
type cordBox struct {
	F      xf
	hx, hz float64
	y0, y1 float64
}

// pushOut: q moved out of the box (to its nearest face) if it's inside, r clear of it.
func (o cordBox) pushOut(q vec3, r float64) vec3 {
	d := q.sub(o.F.o)
	x, y, z := d.dot(o.F.x), d.dot(o.F.y), d.dot(o.F.z)
	if math.Abs(x) >= o.hx+r || math.Abs(z) >= o.hz+r || y <= o.y0-r || y >= o.y1+r {
		return q
	}
	// the smallest way out; resting on top wins a near tie (slack lies on things)
	top := o.y1 + r - y
	best, how := top*0.8, 0
	if v := y - (o.y0 - r); v < best {
		best, how = v, 1
	}
	if v := o.hx + r - math.Abs(x); v < best {
		best, how = v, 2
	}
	if v := o.hz + r - math.Abs(z); v < best {
		best, how = v, 3
	}
	switch how {
	case 0:
		y = o.y1 + r
	case 1:
		y = o.y0 - r
	case 2:
		x = math.Copysign(o.hx+r, x)
	case 3:
		z = math.Copysign(o.hz+r, z)
	}
	return o.F.o.add(o.F.x.mul(x)).add(o.F.y.mul(y)).add(o.F.z.mul(z))
}

// drapeCord: a cord of a given length from a to b that hangs under its own weight and
// lies over the boxes in its way (and on the floor) instead of passing through them.
func drapeCord(a, b vec3, length float64, n int, r float64, boxes ...cordBox) []vec3 {
	pts := cordPts(a, b, length, n)
	seg := length / float64(n)
	for it := 0; it < 30; it++ {
		for k := 1; k < n; k++ {
			pts[k].y -= 0.006 // weight
		}
		for pass := 0; pass < 3; pass++ {
			for k := 0; k < n; k++ {
				d := pts[k+1].sub(pts[k])
				L := d.length()
				if L <= seg || L < 1e-9 {
					continue
				}
				c := d.mul((L - seg) / L)
				switch {
				case k == 0:
					pts[k+1] = pts[k+1].sub(c)
				case k+1 == n:
					pts[k] = pts[k].add(c)
				default:
					pts[k] = pts[k].add(c.mul(0.5))
					pts[k+1] = pts[k+1].sub(c.mul(0.5))
				}
			}
		}
		for k := 1; k < n; k++ {
			for _, o := range boxes {
				pts[k] = o.pushOut(pts[k], r)
			}
			if pts[k].y < r {
				pts[k].y = r
			}
		}
	}
	return pts
}

func drawCord(pts []vec3, r float64, col rgba) {
	saveM := curMat
	useMat(mRubber)
	for k := 0; k+1 < len(pts); k++ {
		cyl(&bEnv, pts[k], pts[k+1], r, r, 6, col, false)
	}
	useMat(saveM)
}
