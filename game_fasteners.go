package main

// Fasteners: Phillips screws (the screwdriver) and hex bolts (the wrench) holding covers on -
// the cart's access plate, its serial compartment door, and some blown generators' covers.
// Which of the cart's two covers is screwed and which is bolted changes every round.
//
// A tool takes hold of a head it comes near (the driver's tip is magnetic; the wrench's box
// end drops over a bolt) and stays on until you pull your hand away. Then turn it
// counter-clockwise - the driver with your wrist, the wrench by swinging its handle round
// the bolt: it shows red and shakes the wrong way (it's as tight as it goes), yellow while
// it breaks loose, green as it comes out. There's no ratchet: twisting back and forth on
// the head gets you nowhere - turn as far as your wrist goes, lift the tool off, set it
// back on and turn again.

import (
	"boundaryvr/sim"
	"math"
)

const (
	fastScrew = iota
	fastBolt
)

// fastSet: a cover's fasteners and the tool working on them.
type fastSet struct {
	g     *game
	kind  int
	place func(i int) (p, n, tan vec3) // fastener i: its foot on the surface, the surface's outward normal, a direction along it
	f     []*screw
	heads []*fastHead
	// the tool in one of them
	eng     int
	ref     vec3
	high    float64 // the furthest this seating has turned it (only going past it counts)
	feel    int
	feelT   float64
	buzzT   float64
	seatAng float64
	seatOff vec3
}

func newFastSet(g *game, kind, n int, place func(i int) (p, n, tan vec3)) *fastSet {
	s := &fastSet{g: g, kind: kind, place: place, eng: -1}
	for i := 0; i < n; i++ {
		s.f = append(s.f, &screw{})
		s.heads = append(s.heads, &fastHead{s: s, i: i})
	}
	return s
}

// rise: how far it stands proud as it backs out
func (s *fastSet) rise(i int) float64 {
	return math.Min(1, s.f[i].ang/(screwTurns*2*math.Pi)) * 0.012
}

func (s *fastSet) headPos(i int) vec3 {
	p, n, _ := s.place(i)
	return p.add(n.mul(0.0145 + s.rise(i)))
}

func (s *fastSet) allOut() bool {
	for _, f := range s.f {
		if !f.out {
			return false
		}
	}
	return true
}

// progress: 0 all in .. 1 all out
func (s *fastSet) progress() float64 {
	done := 0.0
	for _, f := range s.f {
		if f.out {
			done++
		} else {
			done += math.Min(1, f.ang/(screwTurns*2*math.Pi)) * 0.95
		}
	}
	return done / float64(len(s.f))
}

// tool: the prop that works this kind of fastener, if it's in a hand.
func (s *fastSet) tool() (*prop, *gHand) {
	g := s.g
	var p *prop
	if s.kind == fastScrew && g.driver != nil {
		p = g.driver.p
	}
	if s.kind == fastBolt && g.wrench != nil {
		p = g.wrench.p
	}
	if p == nil || !p.held {
		return nil, nil
	}
	return p, g.handHolding(p)
}

// seatedIn: the fastener the held tool is on this frame (-1: none).
func (s *fastSet) seatedIn() int {
	g := s.g
	p, hand := s.tool()
	if p == nil || hand == nil {
		return -1
	}
	// already on one: it stays on until you pull your hand away
	if s.eng >= 0 && s.eng < len(s.f) && !s.f[s.eng].out {
		hp := s.headPos(s.eng)
		_, n, _ := s.place(s.eng)
		if s.kind == fastScrew {
			if p.pos.sub(hp.add(s.seatOff)).length() < unseatR {
				return s.eng
			}
		} else {
			d := hand.palm.sub(hp)
			if math.Abs(d.dot(n)) < 0.2 && math.Abs(projPerp(d, n).length()-wrenchLen) < 0.15 {
				return s.eng
			}
		}
	}
	eng, best := -1, math.Inf(1)
	for i, f := range s.f {
		if f.out {
			continue
		}
		hp := s.headPos(i)
		_, n, _ := s.place(i)
		var d float64
		ok := false
		if s.kind == fastScrew {
			tip, dir := g.driver.tip()
			d = tip.sub(hp).length()
			ok = d < seatR && dir.dot(n.mul(-1)) > seatCone
		} else {
			// the wrench's box end over the head (or pushed through the cover round it), the
			// wrench roughly across the bolt - it snaps flat as it drops on
			w := g.wrench.head().sub(hp)
			along := w.dot(n)
			d = projPerp(w, n).length()
			ok = d < 0.065 && along > -0.07 && along < 0.05 && math.Abs(p.Z.dot(n)) < 0.82 &&
				projPerp(hand.palm.sub(hp), n).length() > 0.06
		}
		if ok && d < best {
			eng, best = i, d
		}
	}
	return eng
}

// frame: the tool turning a fastener out.
func (s *fastSet) frame(dt float64) {
	g := s.g
	s.feelT = math.Max(0, s.feelT-dt)
	s.buzzT -= dt
	eng := s.seatedIn()
	_, hand := s.tool()
	if eng >= 0 && hand != nil {
		f := s.f[eng]
		hp := s.headPos(eng)
		_, n, tan := s.place(eng)
		// what turns it: your wrist's roll for the driver, the handle's swing round the
		// bolt for the wrench
		var cur, ax vec3
		if s.kind == fastScrew {
			X, _, Z := hand.axes()
			cur, ax = projPerp(X, Z), Z
		} else {
			cur, ax = projPerp(hand.palm.sub(hp), n), n
		}
		if cur.length() > 0.02 {
			cur = cur.norm()
			if eng != s.eng {
				hand.buzz(25, 0.85)
				ed.sfx(sndTick, hp, 0.5)
				ed.sfx(sndClang, hp, 0.12)
				s.feel, s.feelT, s.seatAng, s.high = feelSeat, 0, 0, 0
				p, _ := s.tool()
				s.seatOff = p.pos.sub(hp)
			} else {
				d := signedAngle(projPerp(s.ref, ax), cur, ax)
				s.seatAng += d
				tight := s.seatAng < 0
				s.seatAng = math.Max(0, s.seatAng) // it won't go any tighter: the tool doesn't turn
				// only turning it further than this seating has already turned it counts:
				// wiggling back and forth on the head does nothing
				gain := math.Max(0, s.seatAng-s.high)
				s.high = math.Max(s.high, s.seatAng)
				switch {
				case gain > 1e-4:
					// lefty-loosey: the right way
					if f.loose < 1 {
						f.loose = math.Min(1, f.loose+gain/breakAway)
						s.feel, s.feelT = feelBreak, 0.4
						if s.buzzT <= 0 {
							hand.buzz(30, 0.55)
							s.buzzT = 0.05
						}
						if f.loose >= 1 {
							hand.buzz(45, 1)
							ed.sfx(sndCreak, hp, 0.45)
						}
					} else {
						before := f.ang
						f.ang += gain
						s.feel, s.feelT = feelFree, 0.5
						if math.Floor(before/(math.Pi/4)) != math.Floor(f.ang/(math.Pi/4)) {
							hand.buzz(8, 0.4)
							ed.sfx(sndTick, hp, 0.18)
						}
					}
				case d < -1e-4 && tight:
					// the wrong way: it's as tight as it goes
					s.feel, s.feelT = feelTight, 0.45
					if s.buzzT <= 0 {
						hand.buzz(70, 1)
						ed.sfx(sndGrind, hp, 0.22)
						s.buzzT = 0.09
					}
				}
			}
			s.ref = cur
		}
		if s.feelT <= 0 {
			s.feel = feelSeat
			if f.loose >= 1 {
				s.feel = feelFree
			}
		}
		// the tool sits on the head, turning with your hand
		if s.kind == fastScrew {
			k := g.driver
			k.seated = true
			k.seatZ = n
			k.seatX = rotAbout(tan, n, s.seatAng)
			k.seatP = hp.add(n.mul(drvHandle/2 + drvShaft + 0.008))
			k.seatCol = feelColor(s.feel)
		} else {
			w := g.wrench
			w.seated = true
			w.seatHead, w.seatDir, w.seatN = hp, cur, n
			w.seatCol = feelColor(s.feel)
		}
		if f.ang >= screwTurns*2*math.Pi && !f.out {
			f.out = true
			ed.sfx(sndClang, hp, 0.5)
			spawnSparks(hp, n, 5, 0.3)
			hand.buzz(40, 0.8)
			eng = -1
			if g.driver != nil {
				g.driver.seated = false
			}
			if g.wrench != nil {
				g.wrench.seated = false
			}
		}
	}
	s.eng = eng
}

// draw: the heads still in (a Phillips cross or a hex), turned as far as they've turned.
func (s *fastSet) draw(current bool) {
	for i, f := range s.f {
		if f.out {
			continue
		}
		p, up, tan := s.place(i)
		h := s.rise(i)
		top := p.add(up.mul(0.0148 + h))
		useMat(mChrome)
		if s.kind == fastScrew {
			cyl(&bEnv, p.add(up.mul(-0.01)), p.add(up.mul(0.006+h)), 0.006, 0.006, 8, cChromeEnv, false)
			cyl(&bEnv, p.add(up.mul(0.006+h)), p.add(up.mul(0.0145+h)), 0.012, 0.011, 12, cChromeEnv, true)
			a := rotAbout(tan, up, f.ang)
			b := rotAbout(up.cross(tan).norm(), up, f.ang)
			bEnvL.line(top.sub(a.mul(0.008)), top.add(a.mul(0.008)), rgba{0.03, 0.03, 0.03, 1})
			bEnvL.line(top.sub(b.mul(0.008)), top.add(b.mul(0.008)), rgba{0.03, 0.03, 0.03, 1})
		} else {
			// washer, threaded stud, hex head
			useMat(mSteel)
			cyl(&bEnv, p.add(up.mul(-0.001)), p.add(up.mul(0.002)), 0.016, 0.016, 14, cSteelEnv, true)
			cyl(&bEnv, p.add(up.mul(-0.01)), p.add(up.mul(0.006+h)), 0.007, 0.007, 8, cSteelEnv, false)
			useMat(mCast)
			hexHead(p.add(up.mul(0.003+h)), up, rotAbout(tan, up, f.ang), 0.012, 0.0115, rgba{0.4, 0.42, 0.45, 1})
		}
		if current && s.eng == i {
			col := feelColor(s.feel)
			ringSoftAxes(top, tan, up.cross(tan).norm(), 0.02, 0.005, col.alpha(0.95), 20)
			ringSoftAxes(top, tan, up.cross(tan).norm(), 0.03, 0.003, col.alpha(0.45), 20)
			sprite(top, 0.045, col.alpha(0.3))
		}
	}
}

// hexHead: a bolt's hex head, standing on base along up, one flat facing x.
func hexHead(base, up, x vec3, r, hgt float64, col rgba) {
	y := up.cross(x).norm()
	var ring [6]vec3
	for k := 0; k < 6; k++ {
		a := float64(k)*math.Pi/3 + math.Pi/6
		ring[k] = x.mul(r * math.Cos(a)).add(y.mul(r * math.Sin(a)))
	}
	top := base.add(up.mul(hgt))
	for k := 0; k < 6; k++ {
		a, b := ring[k], ring[(k+1)%6]
		n := a.add(b).norm()
		bEnv.quadF(base.add(a), base.add(b), top.add(b), top.add(a), n, col)
		bEnv.quadF(top.add(a), top.add(b), top, top, up, col.scale(1.1))
	}
}

func (s *fastSet) grabbers(out []grabber) []grabber {
	for i, f := range s.f {
		if !f.out {
			out = append(out, s.heads[i])
		}
	}
	return out
}

// fastHead: what a bare hand finds on a screw or a bolt (it won't turn).
type fastHead struct {
	s *fastSet
	i int
}

func (h *fastHead) gPos() vec3    { return h.s.headPos(h.i) }
func (h *fastHead) gRad() float64 { return 0.06 }
func (h *fastHead) gOK() bool {
	p, _ := h.s.tool()
	return !h.s.f[h.i].out && p == nil
}
func (h *fastHead) gGrab(gh *gHand) {
	gh.buzz(45, 0.7)
	if h.s.kind == fastScrew {
		h.s.g.bareScrew(h.gPos())
	} else {
		h.s.g.bareBolt(h.gPos())
	}
}
func (h *fastHead) gHold(gh *gHand, dt float64) {}
func (h *fastHead) gRelease(gh *gHand)          {}

// fastName: what a cover held on with this kind of fastener says to do.
func fastVerb(kind int) string {
	if kind == fastBolt {
		return "UNBOLT"
	}
	return "UNSCREW"
}

// cartFasteners: which of the cart's covers is screwed and which bolted (it changes every
// round): the access plate, then the serial compartment.
func (g *game) cartFasteners() (plate, door int) {
	if g.boltedPlate {
		return fastBolt, fastScrew
	}
	return fastScrew, fastBolt
}

// toolNeeded: the tool the current stage of the cart needs, if it's a fastener stage and
// there's still something to undo (-1 if none).
func (g *game) toolNeeded() int {
	r := g.r
	d := &r.Dev
	if d.State != sim.DevHome || d.Cur >= len(g.devVis.mods) {
		return -1
	}
	if k := g.misfire; k != nil && k.active() && !k.fs.allOut() {
		return k.kind
	}
	switch m := g.devVis.mods[d.Cur].(type) {
	case *modScrews:
		if m.fs != nil && !m.fs.allOut() {
			return m.fs.kind
		}
	case *modSerial:
		if m.fs != nil && !m.fs.allOut() {
			return m.fs.kind
		}
	}
	return g.genToolNeeded()
}

// toolCall: the next step needs a tool you've had but put down somewhere: after a few
// seconds without it (or straight away if you try a fastener bare-handed) your contact calls
// to say where it's lying now.
type toolCall struct {
	t    float64
	said bool
}

func (g *game) toolCallFrame(c *toolCall, kind int, p *prop, everHeld bool, id string, dt float64) {
	if g.toolNeeded() != kind || g.handHolding(p) != nil || g.r.Phase != sim.Playing {
		c.t, c.said = 0, false
		return
	}
	if c.t += dt; everHeld && !c.said && c.t > 8 {
		c.said = true
		radioSay(id, false)
	}
}

// bareCall: a fastener tried bare-handed: if the tool's been put down somewhere, say where.
func (g *game) bareCall(c *toolCall, everHeld bool, id string) {
	if everHeld && !c.said && g.r.Phase == sim.Playing {
		c.said = true
		radioSay(id, false)
	}
}

// genToolNeeded: the tool a blown generator's scorched cover needs (-1 if none).
func (g *game) genToolNeeded() int {
	for _, gp := range g.panels_ {
		if gp.cover != nil && !gp.cover.allOut() {
			return gp.cover.kind
		}
	}
	return -1
}

// whereIs: where a tool is lying right now, as the radio says it ("at_cart", "at_north"...).
func (g *game) whereIs(p *prop) string {
	r := g.r
	q := sim.V(p.pos.x, p.pos.z)
	if F := g.devFrame(); r.Dev.State == sim.DevHome {
		if c := inXf(F, p.pos.sub(F.o)); math.Abs(c.x) < devHW+0.15 && math.Abs(c.z) < devHL+0.15 {
			return "at_cart"
		}
	}
	for _, gn := range r.Gens {
		if sim.Dist(q, gn.Pos) < 0.6 {
			return "at_gen"
		}
	}
	if g.radio != nil && sim.Dist(q, g.radio.pos) < 0.6 {
		return "at_radio"
	}
	if sim.Dist(q, g.consoleSpot()) < 0.6 {
		return "at_console"
	}
	if r.Harp.Up && sim.Dist(q, r.Harp.Pos) < 0.7 {
		return "at_turret"
	}
	return g.compass(q)
}

// compass: which part of your room a spot's in ("at_middle", "at_north"...; north is the
// NORTH door's way).
func (g *game) compass(q sim.V2) string {
	d := q.Sub(sim.Centroid(g.r.Room))
	switch {
	case d.Len() < 0.7:
		return "at_middle"
	case math.Abs(d.Z) >= math.Abs(d.X) && d.Z < 0:
		return "at_north"
	case math.Abs(d.Z) >= math.Abs(d.X):
		return "at_south"
	case d.X > 0:
		return "at_east"
	}
	return "at_west"
}
