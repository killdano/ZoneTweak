package main

// Leeches. A drone flies in, rises over your beam and hovers above a generator, letting
// down a tether that clamps onto the inside of its shaft - about shoulder height, on your
// side - and starts boring in. Two ways to deal with it:
//   - yank the clamp off: the generator's free at once, but the drone only limps off outside
//     your ring, patches itself up for most of a minute, and comes back;
//   - smash it with the pole (it stands by the radio): swing it through the drone, hard.

import (
	"boundaryvr/sim"
	"math"
	"math/rand"
	"strings"
)

const (
	poleLen  = 1.3  // the pole, end to end
	poleGrip = 0.2  // how far your fist is from its butt end
	smashV   = 1.8  // m/s the pole's head needs to break a drone
	clampY   = 1.62 // where the tether clamps on the shaft
)

// leechPost: which of a machine's posts a leech clamps onto - for a double, the one
// furthest inside your boundary (well clear of the real wall).
func (g *game) leechPost(gi int) int {
	best, bd := gi, math.Inf(-1)
	for _, k := range g.postsOf(gi) {
		if d := sim.EdgeDist(g.r.Room, g.r.Gens[k].Pos); d > bd {
			best, bd = k, d
		}
	}
	return best
}

// leechClamp: where a drone's tether clamps onto generator gi, and the way into the ring.
func (g *game) leechClamp(gi int) (vec3, sim.V2) {
	G := g.r.Gens2()
	pi := g.leechPost(gi)
	in := inwardAt(G, pi)
	return fv(G[pi].Add(in.Mul(0.07)), clampY), in
}

// dronePos: where a drone is drawn (and where the pole has to hit it).
func (g *game) dronePos(b *sim.Robot) (vec3, float64) {
	r := g.r
	t := fxTime()
	ph := float64(b.ID) * 1.7
	h := b.Height() + 0.06*math.Sin(t*2.3+ph)
	pos := fv(b.Pos, h)
	yaw := b.Yaw
	switch {
	case b.State == sim.RAttached && b.Gen >= 0:
		gp := r.Gens[g.leechPost(b.Gen)].Pos
		_, in := g.leechClamp(b.Gen)
		hover := fv(gp, genH+0.5+0.05*math.Sin(t*2.3+ph))
		pos, yaw = hover, math.Atan2(in.Z, in.X)
		if b.Wait < 0 {
			// rising up over the beam from outside, to hover above it
			k := smooth(1 + b.Wait/sim.LeechHop)
			from := fv(gp.Sub(in.Mul(0.62)), h)
			apex := fv(gp.Sub(in.Mul(0.3)), genH+0.75)
			pos = from.mul((1 - k) * (1 - k)).add(apex.mul(2 * k * (1 - k))).add(hover.mul(k * k))
		}
	case b.State == sim.RRecover:
		// limping about outside your ring, patching itself up
		e := sim.RecoverSecs - b.Wait
		pos = fv(b.Pos, 2.1+0.12*math.Sin(t*1.3+ph))
		if e < 1.2 {
			// flung off: from over the generator, tumbling out to where it'll recover
			k := smooth(e / 1.2)
			start := fv(b.Pos, genH+0.5)
			if b.Gen >= 0 && b.Gen < len(r.Gens) {
				start = fv(r.Gens[g.leechPost(b.Gen)].Pos, genH+0.5)
			}
			pos = start.mul(1 - k).add(pos.mul(k)).add(v3(0, 0.5*math.Sin(k*math.Pi), 0))
		}
	}
	return pos, yaw
}

// leechHandle: the tether's clamp on a generator: grab it and yank.
type leechHandle struct {
	g    *game
	id   int
	at   vec3
	grab vec3
}

func (l *leechHandle) gPos() vec3    { return l.at }
func (l *leechHandle) gRad() float64 { return 0.13 }
func (l *leechHandle) gOK() bool {
	b := l.g.robotByID(l.id)
	return b != nil && b.State == sim.RAttached && b.Wait >= 0 // clamped on, not still coming down
}
func (l *leechHandle) gGrab(h *gHand) { l.grab = h.palm }
func (l *leechHandle) gHold(h *gHand, dt float64) {
	h.buzz(10, 0.8)
	if h.palm.sub(l.grab).length() > 0.2 {
		l.g.r.Unhook(l.id)
		spawnSparks(l.at, h.palm.sub(l.grab).norm(), 30, 0.8)
		ed.sfx(sndRip, l.at, 0.8)
		h.buzz(80, 1)
		h.held = nil
	}
}
func (l *leechHandle) gRelease(h *gHand) {}

func (g *game) robotByID(id int) *sim.Robot {
	for _, b := range g.r.Robots {
		if b.ID == id {
			return b
		}
	}
	return nil
}

// ---- the pole ----

type poleKit struct {
	p        *prop
	headPrev vec3
	bumpT    float64
	kills    int  // wardens it's clubbed (it snaps on the poleLife-th)
	broken   bool // snapped: gone for the rest of the round
	// held: the pole's own swing, lagging a touch behind your wrist (it has weight)
	z, w    vec3 // its direction and how fast that's turning
	wasHeld bool
	// dropped or thrown: its two ends as a rigid rod that tumbles and bounces
	loose  bool
	e, ev  [2]vec3 // butt, head and their velocities
	stillT float64
	clankT float64
	zapT   float64 // an electric crack at the tip (seconds left)
	pick   float64 // which generator its bracket's on (0..1 through them; set per round)
}

// how heavy the pole feels: the spring pulling it round after your wrist, and its damping
const (
	poleStiff = 220.0
	poleDamp  = 22.0
)

// poleLife: how many wardens the pole takes down before it snaps (leeches don't wear it)
const poleLife = 3

func (g *game) buildPole() {
	k := &poleKit{pick: rand.New(rand.NewSource(g.seed*7919 + 13)).Float64()}
	k.p = &prop{radius: 0.16, hy: 0.03}
	k.p.grip = &[4]vec3{v3(0, 0, 0), v3(1, 0, 0), v3(0, 1, 0), v3(0, 0, 1)}
	// upright in its bracket on one of the generators, prongs down
	k.p.home = func() (vec3, vec3, vec3, vec3) {
		m := g.poleMount()
		X := fv(m.side, 0)
		Z := v3(0, 1, 0)
		return fv(m.at, poleLen-poleGrip+0.12), X, X.cross(Z), Z
	}
	k.p.homeR = 0.35
	k.p.ok = func() bool { return !k.broken }
	k.p.docked = true
	g.pole = k // (before its first pose: the bracket goes by its pick)
	k.p.pos, k.p.X, k.p.Y, k.p.Z = k.p.home()
}

// poleMnt: the prod's bracket - bolted to a generator post (which one changes from round to
// round, and with how many generators there are), just to one side of its panel, holding
// the prod upright a hand's width out into the room.
type poleMnt struct {
	at, in, side sim.V2 // where the prod stands; into the room; along the wall
	post         sim.V2 // the post it's bolted to
	gen          int
}

func (g *game) poleMount() poleMnt {
	r := g.r
	var leads []int
	for i, gen := range r.Gens {
		if gen.Lead == i {
			leads = append(leads, i)
		}
	}
	k := g.pole
	pick := 0.0
	if k != nil {
		pick = k.pick
	}
	if len(leads) == 0 {
		c := sim.Centroid(r.Room)
		return poleMnt{at: c, in: sim.V(0, 1), side: sim.V(1, 0), post: c, gen: -1}
	}
	G := r.Gens2()
	// the chosen one first; if its bracket won't fit inside your room, the next one round
	for t := 0; t < len(leads); t++ {
		i := leads[(int(pick*float64(len(leads)))+t)%len(leads)]
		posts := g.postsOf(i)
		p := sim.V2{}
		for _, k := range posts {
			p = p.Add(G[k])
		}
		p = p.Mul(1 / float64(len(posts)))
		in := inwardAt(G, i)
		side := sim.V(-in.Z, in.X)
		// beside its panel (the roomier side), or further out if a corner's too tight
		for _, o := range [][2]float64{{0.24, 0.36}, {0.4, 0.3}} {
			best, bd := sim.V2{}, -1.0
			for _, sd := range []float64{1, -1} {
				q := p.Add(in.Mul(o[0])).Add(side.Mul(sd * o[1]))
				if !sim.InPoly(r.Room, q) {
					continue
				}
				if d := sim.EdgeDist(r.Room, q); d > bd {
					best, bd = q, d
				}
			}
			if bd > 0.1 {
				return poleMnt{at: best, in: in, side: side, post: p, gen: i}
			}
		}
	}
	// nowhere beside a panel: out in front of the first one's
	i := leads[int(pick*float64(len(leads)))%len(leads)]
	p := G[i]
	in := inwardAt(G, i)
	return poleMnt{at: p.Add(in.Mul(0.5)), in: in, side: sim.V(-in.Z, in.X), post: p, gen: i}
}

// drawPoleBracket: the bracket - two hazard-striped clamps on arms from the post, a PROD
// plate and a lamp that blinks while the prod's in it (so you notice it from across the room).
func (g *game) drawPoleBracket() {
	k := g.pole
	if k == nil {
		return
	}
	m := g.poleMount()
	t := fxTime()
	c := fv(m.at, 0)
	post := fv(m.post, 0)
	in, side := fv(m.in, 0), fv(m.side, 0)
	docked := k.p.docked && !k.broken
	defer cleanRoles()()
	for _, y := range []float64{0.55, 1.18} {
		cc := c.add(v3(0, y, 0))
		pp := post.add(v3(0, y, 0)).add(in.mul(0.07))
		useMat(mSteel)
		d := cc.sub(pp)
		bevelBox(&bEnv, pp.add(d.mul(0.5)), d.norm(), v3(0, 1, 0), d.norm().cross(v3(0, 1, 0)).norm(), d.length()/2, 0.018, 0.012, 0.004, cSteelEnv.scale(0.8))
		useMat(mHazard)
		cyl(&bEnv, cc.sub(v3(0, 0.025, 0)), cc.add(v3(0, 0.025, 0)), 0.032, 0.032, 14, rgba{0.9, 0.65, 0.1, 1}, true)
		if docked {
			ringSoftAxes(cc, side, in, 0.04, 0.006, cAmber.alpha(float32(0.5+0.3*math.Sin(t*5+y*3))), 20)
		}
		useMat(mCast)
		cyl(&bEnv, pp.sub(v3(0, 0.04, 0)), pp.add(v3(0, 0.04, 0)), 0.03, 0.03, 10, cIron, true)
	}
	// a plate on a stalk over the prod's butt, with a lamp on top, facing into the room
	pc := c.add(v3(0, 1.62, 0)).sub(in.mul(0.03))
	useMat(mSteel)
	cyl(&bEnv, c.add(v3(0, 1.18, 0)).sub(in.mul(0.035)), pc.sub(v3(0, 0.04, 0)), 0.008, 0.008, 6, cSteelEnv, false)
	useMat(mPaint)
	bevelBox(&bEnv, pc, side, v3(0, 1, 0), in, 0.13, 0.06, 0.008, 0.004, rgba{0.12, 0.12, 0.13, 1})
	faceText(&bOpaque, "PROD", pc.add(in.mul(0.009)).add(v3(0, 0.012, 0)), in, v3(0, 1, 0), 0.0036, cAmber)
	faceText(&bOpaque, "KILLS LEECHES", pc.add(in.mul(0.009)).add(v3(0, -0.035, 0)), in, v3(0, 1, 0), 0.0013, cWhite.alpha(0.85))
	for _, sy := range []float64{-1, 1} {
		a, b := pc.add(in.mul(0.009)).add(side.mul(-0.125)).add(v3(0, sy*0.056, 0)), pc.add(in.mul(0.009)).add(side.mul(0.125)).add(v3(0, sy*0.056, 0))
		bGlowLine.line(a, b, cAmber.alpha(0.7))
	}
	lp := pc.add(v3(0, 0.085, 0))
	useMat(mCast)
	cyl(&bEnv, lp.sub(v3(0, 0.025, 0)), lp.sub(v3(0, 0.012, 0)), 0.02, 0.02, 10, cIron, true)
	lc := rgba{0.3, 0.2, 0.05, 1}
	if docked {
		k := 0.5 + 0.5*math.Sin(t*5)
		lc = cAmber.scale(float32(0.6 + 0.4*k))
		sprite(lp, 0.2, cAmber.alpha(float32(0.25+0.35*k)))
		addPoint(lp.add(in.mul(0.1)), cAmber, 2.5*k+0.8, 2.5)
	}
	curEmis = 2
	sphere(&bEnv, lp, 0.022, lc, 4, 8)
	curEmis = 1
	if docked && g.r.Phase == sim.Setup {
		ringSoft(fv(m.at, 0.01), 0.22+0.03*math.Sin(t*3), 0.02, cAmber.alpha(0.5), 28)
	}
}

// poleWhere: the radio clip saying where the prod is ("" if it's broken).
func (g *game) poleWhere() string {
	k := g.pole
	if k == nil || k.broken {
		return ""
	}
	if k.p.docked {
		return "p_br_" + strings.TrimPrefix(g.compass(g.poleMount().at), "at_")
	}
	return "p_" + g.whereIs(k.p)
}

// poleEnds: the pole's butt and head (it runs out ahead of your fist).
func (k *poleKit) ends() (vec3, vec3) {
	p := k.p
	return p.pos.add(p.Z.mul(poleGrip)), p.pos.sub(p.Z.mul(poleLen - poleGrip))
}

func (g *game) poleFrame(dt float64) {
	k := g.pole
	if k == nil || k.broken {
		return
	}
	r := g.r
	p := k.p
	hand := g.handHolding(p)
	k.zapT = math.Max(0, k.zapT-dt)
	switch {
	case hand != nil:
		// your fist holds the butt; the rest swings round after it
		target := p.Z
		if !k.wasHeld {
			k.z, k.w = target, vec3{}
		}
		if dt > 0 {
			k.w = k.w.add(k.z.cross(target).mul(poleStiff * dt)).mul(math.Max(0, 1-poleDamp*dt))
			k.z = k.z.add(k.w.cross(k.z).mul(dt)).norm()
		}
		p.Z = k.z
		p.Y = projPerp(p.Y, p.Z).norm()
		p.X = p.Y.cross(p.Z).norm()
		if wl := k.w.length(); wl > 4 && rngF() < dt*20 {
			hand.buzz(8, float32(math.Min(0.6, wl*0.04))) // you feel it swing
		}
		k.loose = false
	case p.held || p.docked:
		k.loose = false // in flight to your hand, or on its stand
	case k.wasHeld || !k.loose && !p.resting:
		// let go (or dropped from a pull): it falls as a stick, ends and all
		butt, head := k.ends()
		k.e = [2]vec3{butt, head}
		vh := p.vel
		if k.wasHeld && dt > 0 {
			vh = head.sub(k.headPrev).mul(1 / dt)
		}
		k.ev = [2]vec3{p.vel, vh}
		k.loose, k.stillT, p.resting, p.onCart = true, 0, false, false
	}
	k.wasHeld = hand != nil
	if k.loose {
		g.poleTumble(k, dt)
	} else if hand == nil {
		p.tick(dt)
	}
	if hand == nil && !sim.InPoly(r.Room, sim.V(p.pos.x, p.pos.z)) {
		// you can't walk out of your room for it: it turns up back on its stand
		p.docked, p.held, k.loose = true, false, false
		p.pos, p.X, p.Y, p.Z = p.home()
	}
	k.bumpT -= dt
	_, head := k.ends()
	if hand == nil || dt <= 0 {
		k.headPrev = head
		return
	}
	v := head.sub(k.headPrev).mul(1 / dt)
	k.headPrev = head
	tip := head.add(p.Z.mul(0.45)) // the business end
	for _, b := range r.Robots {
		if b.Kind != sim.Drone || !b.Alive() {
			continue
		}
		dp, _ := g.dronePos(b)
		if pointSegDist(dp, tip, head.sub(p.Z.mul(0.08))) > 0.24 {
			continue
		}
		if v.length() >= smashV {
			// the prod's tip: a crack of current and it drops, tethered or flying about
			if b.State == sim.RAttached && b.Wait >= 0 {
				r.Pluck(b.ID)
			} else {
				r.Smash(b.ID)
			}
			k.zapT = 0.35
			spawnSparks(dp, v.norm(), 70, 1.4)
			spawnSmoke(dp, 6)
			ed.sfx(sndZap, dp, 0.9)
			ed.sfx(sndBoom, dp, 0.6)
			hand.buzz(120, 1)
		} else if k.bumpT <= 0 {
			k.bumpT = 0.3
			ed.sfx(sndClang, dp, 0.3)
			hand.buzz(30, 0.5)
		}
	}
	g.clubRobots(k, head, head.add(p.Z.mul(0.7)), v)
}

// poleTumble: a loose pole falling as a rigid stick - it bounces, clatters and rolls to a
// stop, and lands on the cart's lid if it comes down on it.
func (g *game) poleTumble(k *poleKit, dt float64) {
	p := k.p
	const r0, steps = 0.02, 3
	h := dt / steps
	for s := 0; s < steps && dt > 0; s++ {
		for i := range k.e {
			old := k.e[i]
			k.ev[i] = k.ev[i].add(v3(0, -9.8*h, 0)).mul(1 - 0.15*h)
			k.e[i] = k.e[i].add(k.ev[i].mul(h))
			ground := r0
			if cartLid != nil {
				if F, hx, hz, top, ok := cartLid(); ok {
					q, was := inXf(F, k.e[i].sub(F.o)), inXf(F, old.sub(F.o))
					if math.Abs(q.x) < hx && math.Abs(q.z) < hz && was.y >= top-0.03 {
						ground = F.o.y + top + r0
					}
				}
			}
			if k.e[i].y < ground {
				k.e[i].y = ground
				if vy := -k.ev[i].y; vy > 0 {
					if vy > 0.7 && k.clankT <= 0 {
						k.clankT = 0.07
						ed.sfx(sndClang, k.e[i], math.Min(0.7, 0.12*vy))
					}
					k.ev[i].y = vy * 0.38
					if k.ev[i].y < 0.25 {
						k.ev[i].y = 0
					}
				}
				k.ev[i].x *= 1 - math.Min(1, 6*h)
				k.ev[i].z *= 1 - math.Min(1, 6*h)
			}
		}
		// it's rigid: hold the two ends a pole's length apart
		d := k.e[1].sub(k.e[0])
		L := d.length()
		if L < 1e-6 {
			d, L = v3(1, 0, 0), 1
		}
		n := d.mul(1 / L)
		c := (L - poleLen) / 2
		k.e[0], k.e[1] = k.e[0].add(n.mul(c)), k.e[1].sub(n.mul(c))
		rel := k.ev[1].sub(k.ev[0]).dot(n) / 2
		k.ev[0], k.ev[1] = k.ev[0].add(n.mul(rel)), k.ev[1].sub(n.mul(rel))
	}
	k.clankT -= dt
	// where the prop is: its fist point and its line
	Z := k.e[0].sub(k.e[1]).norm()
	p.Z = Z
	p.pos = k.e[0].sub(Z.mul(poleGrip))
	p.Y = projPerp(p.Y, Z)
	if p.Y.length() < 0.1 {
		p.Y = projPerp(v3(0, 1, 0), Z)
		if p.Y.length() < 0.1 {
			p.Y = projPerp(v3(1, 0, 0), Z)
		}
	}
	p.Y = p.Y.norm()
	p.X = p.Y.cross(Z).norm()
	p.vel = k.ev[0].add(k.ev[1]).mul(0.5)
	if k.ev[0].length() < 0.06 && k.ev[1].length() < 0.06 {
		k.stillT += dt
	} else {
		k.stillT = 0
	}
	if k.stillT > 0.3 {
		// come to rest (riding the cart if it's lying on its lid)
		k.loose, p.resting, p.vel = false, true, vec3{}
		if cartLid != nil {
			if F, hx, hz, top, ok := cartLid(); ok {
				q := inXf(F, p.pos.sub(F.o))
				if q.y > top-0.02 && math.Abs(q.x) < hx+poleLen && math.Abs(q.z) < hz+poleLen && (k.e[0].y > 0.3 || k.e[1].y > 0.3) {
					p.onCart = true
					p.cartRel = [4]vec3{q, inXf(F, p.X), inXf(F, p.Y), inXf(F, p.Z)}
				}
			}
		}
	}
}

// clubRobots: a hard swing into a warden or a heavy takes it down - and wears the pole.
func (g *game) clubRobots(k *poleKit, head, tip, v vec3) {
	r := g.r
	hand := g.handHolding(k.p)
	for _, b := range r.Robots {
		if b.Kind == sim.Drone || !b.Alive() {
			continue
		}
		// the body: an upright column round its position
		hit, at := false, vec3{}
		for q := 0; q <= 6; q++ {
			p := head.add(tip.sub(head).mul(float64(q) / 6))
			if p.y < 0.25 || p.y > b.Height()+0.1 {
				continue
			}
			if math.Hypot(p.x-b.Pos.X, p.z-b.Pos.Z) < b.Radius()+0.06 {
				hit, at = true, p
				break
			}
		}
		if !hit {
			continue
		}
		if v.length() < smashV {
			if k.bumpT <= 0 {
				k.bumpT = 0.3
				ed.sfx(sndClang, at, 0.4)
				if hand != nil {
					hand.buzz(30, 0.5)
				}
			}
			continue
		}
		if !r.Smash(b.ID) {
			continue
		}
		k.kills++
		spawnSparks(at, v.norm(), 90, 1.6)
		spawnSmoke(at, 8)
		ed.sfx(sndBoom, at, 0.8)
		ed.sfx(sndClang, at, 1)
		if hand != nil {
			hand.buzz(150, 1)
		}
		if k.kills >= poleLife {
			g.snapPole(k, at)
			return
		}
		left := poleLife - k.kills
		msg := itoa(left) + " MORE AND IT SNAPS"
		if left == 1 {
			msg = "ONE MORE AND IT SNAPS"
		}
		g.say("SMASHED", msg, cAmber, 2)
		k.bumpT = 0.3
	}
}

// snapPole: the pole breaks in two and it's gone for the round.
func (g *game) snapPole(k *poleKit, at vec3) {
	k.broken = true
	if hand := g.handHolding(k.p); hand != nil {
		hand.held = nil
	}
	k.p.held, k.p.docked = false, false
	ed.sfx(sndRip, at, 1)
	ed.sfx(sndClang, at, 0.8)
	for q := 0; q < 5; q++ {
		p := k.p.pos.sub(k.p.Z.mul(poleLen * float64(q) / 5))
		spawnSparks(p, v3(0, 1, 0), 15, 1)
	}
	spawnSmoke(at, 10)
	g.say("THE POLE SNAPPED", "NO MORE SMASHING LEECHES - YANK THEIR TETHERS", cDanger, 3)
}

func pointSegDist(p, a, b vec3) float64 {
	ab := b.sub(a)
	L := ab.dot(ab)
	u := 0.0
	if L > 1e-9 {
		u = math.Max(0, math.Min(1, p.sub(a).dot(ab)/L))
	}
	return p.sub(a.add(ab.mul(u))).length()
}

func (g *game) drawPole() {
	k := g.pole
	if k == nil || k.broken {
		return
	}
	butt, head := k.ends()
	Z := k.p.Z
	useMat(mSteel)
	cyl(&bEnv, butt, head.add(Z.mul(0.3)), 0.014, 0.014, 10, cSteelEnv, true)
	useMat(mRubber)
	cyl(&bEnv, butt, butt.sub(Z.mul(0.4)), 0.019, 0.019, 10, rgba{0.06, 0.06, 0.06, 1}, true)
	useMat(mHazard)
	cyl(&bEnv, head.add(Z.mul(0.3)), head.add(Z.mul(0.02)), 0.03, 0.026, 12, rgba{0.85, 0.6, 0.08, 1}, true)
	useMat(mCast)
	cyl(&bEnv, head.add(Z.mul(0.02)), head.sub(Z.mul(0.02)), 0.036, 0.03, 12, cIron, true)
	// the prod: two copper prongs with current crackling between them
	t := fxTime()
	X := k.p.X
	useMat(mSteel)
	var pr [2]vec3
	for i, sx := range []float64{-1, 1} {
		a := head.sub(Z.mul(0.02)).add(X.mul(sx * 0.014))
		pr[i] = a.sub(Z.mul(0.06))
		cyl(&bEnv, a, pr[i], 0.005, 0.004, 6, rgba{0.75, 0.45, 0.25, 1}, true)
	}
	hot := 0.5
	if k.p.held {
		hot = 1
	}
	if k.zapT > 0 {
		hot = 3
	}
	for q := 0; q < 2; q++ {
		arc(pr[0], pr[1], cBeam)
	}
	mid := pr[0].add(pr[1]).mul(0.5)
	sprite(mid, 0.06*hot+0.02*math.Sin(t*31), cBeam.alpha(float32(0.5+0.3*math.Sin(t*23))))
	addPoint(mid.sub(Z.mul(0.05)), cBeam, 0.4*hot, 1.2)
	if k.zapT > 0 || rngF() < 0.08*hot {
		d := v3(rngF()-0.5, rngF()-0.5, rngF()-0.5).norm()
		arc(mid, mid.add(d.mul(0.12+0.25*k.zapT)).sub(Z.mul(0.05)), cBeam)
	}
	// every warden it's clubbed leaves a split in it
	for q := 0; q < k.kills; q++ {
		c := head.add(Z.mul(0.5 + 0.12*float64(q)))
		useMat(mCast)
		cyl(&bEnv, c.sub(Z.mul(0.02)), c.add(Z.mul(0.02)), 0.0155, 0.0155, 10, rgba{0.05, 0.04, 0.03, 1}, false)
		if q == poleLife-2 {
			curEmis = 2
			cyl(&bEnv, c.sub(Z.mul(0.006)), c.add(Z.mul(0.006)), 0.016, 0.016, 10, cDanger.scale(0.7), false)
			curEmis = 1
		}
	}
	if !k.p.held {
		// a stripe of glow paint so you can find it in the dark
		curEmis = 2
		cyl(&bEnv, head.add(Z.mul(0.2)), head.add(Z.mul(0.16)), 0.031, 0.031, 12, cAmber.scale(0.6), false)
		curEmis = 1
	}
}

// drawLeechTether: a clamped drone's tether, its clamp on the shaft, and how close it is to
// biting through.
func (g *game) drawLeechTether(b *sim.Robot, pos vec3) {
	r := g.r
	if b.State != sim.RAttached || b.Gen < 0 {
		return
	}
	t := fxTime()
	clamp, in := g.leechClamp(b.Gen)
	tk := 1.0
	if b.Wait < 0 {
		tk = smooth((1 + b.Wait/sim.LeechHop - 0.6) / 0.4)
	}
	if tk <= 0.01 {
		return
	}
	end := pos.add(clamp.sub(pos).mul(tk))
	from := pos.add(v3(0, -0.05, 0))
	pts := drapeCord(from, end, end.sub(from).length()*1.08, 10, 0.006)
	drawCord(pts, 0.008, rgba{0.08, 0.08, 0.09, 1})
	for q := 0; q+1 < len(pts); q++ {
		ribbon(pts[q], pts[q+1], 0.014, cDanger.alpha(0.35)) // it glows a little, so you can see it in the dark
	}
	if tk < 1 {
		return
	}
	// the clamp: jaws round the shaft
	inv := fv(in, 0)
	side := inv.cross(v3(0, 1, 0)).norm()
	useMat(mSteel)
	bevelBox(&bEnv, clamp.add(inv.mul(0.02)), side, v3(0, 1, 0), inv, 0.03, 0.025, 0.02, 0.006, cSteelEnv)
	for _, sx := range []float64{-1, 1} {
		cyl(&bEnv, clamp.add(side.mul(sx*0.028)), clamp.add(side.mul(sx*0.03)).sub(inv.mul(0.06)), 0.007, 0.005, 6, cSteelEnv, true)
	}
	gen := r.Gens[b.Gen]
	if gen.State == sim.GenUp {
		// boring in: pulses run down the tether, and a ring round the clamp fills to the bite
		k := math.Min(1, b.Wait/math.Max(0.1, r.T.LeechBite))
		for q := 0; q < 3; q++ {
			u := math.Mod(t*1.6+float64(q)/3, 1)
			i := int(u * float64(len(pts)-1))
			sprite(pts[i], 0.035, cDanger.alpha(0.8))
		}
		cam := ed.hmd.pos()
		rr, uu := billboard(cam, clamp)
		const seg = 24
		for q := 0; q < seg; q++ {
			a0, a1 := float64(q)/seg*2*math.Pi, float64(q+1)/seg*2*math.Pi
			c := cMuted.alpha(0.5)
			if float64(q)/seg < k {
				c = cDanger
			}
			p0 := clamp.add(rr.mul(0.08 * math.Sin(a0))).add(uu.mul(0.08 * math.Cos(a0)))
			p1 := clamp.add(rr.mul(0.08 * math.Sin(a1))).add(uu.mul(0.08 * math.Cos(a1)))
			bGlowLine.line(p0, p1, c)
		}
		if rand.Float64() < frameDT*(4+20*k) {
			spawnSparks(clamp, inv, 3, 0.4)
		}
	}
}

// drawRecovering: an unhooked drone patching itself up outside your ring.
func (g *game) drawRecovering(b *sim.Robot, pos vec3) {
	if b.State != sim.RRecover {
		return
	}
	t := fxTime()
	e := sim.RecoverSecs - b.Wait
	if e < 1.5 && rand.Float64() < 0.5 {
		spawnSmoke(pos, 1)
	}
	if rand.Float64() < frameDT*2 {
		spawnSparks(pos, v3(rand.NormFloat64(), 0.5, rand.NormFloat64()).norm(), 4, 0.3)
	}
	// a welding flicker where it's mending
	if math.Sin(t*23+float64(b.ID)) > 0.3 {
		sprite(pos.add(v3(0, -0.06, 0)), 0.06, cBeam.alpha(0.8))
		addPoint(pos, cBeam, 0.8, 1.5)
	}
	// how far along it is
	k := math.Min(1, e/sim.RecoverSecs)
	cam := ed.hmd.pos()
	rr, uu := billboard(cam, pos)
	n := int(32 * k)
	for q := 0; q < n; q++ {
		a0, a1 := float64(q)/32*2*math.Pi, float64(q+1)/32*2*math.Pi
		bGlowLine.line(pos.add(rr.mul(0.22*math.Sin(a0))).add(uu.mul(0.22*math.Cos(a0))), pos.add(rr.mul(0.22*math.Sin(a1))).add(uu.mul(0.22*math.Cos(a1))), cAmber.alpha(0.7))
	}
}
