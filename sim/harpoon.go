package sim

import "math"

// The harpoon: finishing a quarter, half and three quarters of the device each loads one
// spear (you can only hold one). The turret rises well inside the ring, toward the
// generator most likely to go next.
// Whatever the spear sticks into goes live: a warden gets reeled in hand over hand (drag it
// through a supercharged wall and it fries), the device or its carrier gets hauled home, and
// a floor or wall turns the line into a live wire that fries whatever walks into it.

// freshSpear: what a new load is - a full spear (green), or now and then a pair of
// reel-only ones (yellow).
func (r *Round) freshSpear() string {
	h := &r.Harp
	h.Armed, h.Reels, h.ReelLeft = true, 0, 0
	if r.rng.Float64() < r.T.ReelLoad {
		h.Reels = 2
		return "reel"
	}
	return ""
}

func (r *Round) armHarpoon() {
	h := &r.Harp
	if h.Armed && (h.Reels == 0 || h.Super) {
		return
	}
	if h.Up {
		// already standing: it just loads a spear where it is (a line that's out stays out;
		// a reel-only spear in it makes way for the new one)
		r.emit(EvArmed, h.Pos, h.Gen, 1, r.freshSpear())
		return
	}
	r.releaseShot()
	p, g, ok := r.weakSpot()
	if !ok {
		return
	}
	h.Up, h.Pos, h.Gen = true, p, g
	h.Muzzle = p.At(1.15)
	h.Powered = true
	r.emit(EvArmed, p, g, 0, r.freshSpear())
}

func (r *Round) releaseShot() {
	h := &r.Harp
	if h.Kind == ShotRobot {
		if b := r.robot(h.Target); b != nil && b.Alive() && b.State == RHooked {
			b.Pinned = false
			r.planWalker(b, false)
		}
	}
	h.Kind, h.Target, h.Held, h.Line, h.Live, h.LineT = ShotNone, 0, false, 0, false, 0
}

// weakSpot: toward the generator that looks most likely to go next, but well in from the
// ring - you stand behind the turret and swing it about, so it wants room round it (as much
// as the room has: it backs off to a tighter spot in a small room).
func (r *Round) weakSpot() (V2, int, bool) {
	n := len(r.Ring)
	if n < 3 {
		return V2{}, -1, false
	}
	// the generator that looks most likely to go next
	gw, bs := -1, math.Inf(-1)
	for _, gi := range r.Ring {
		g := &r.Gens[gi]
		if sc := (r.T.ChargeMax - g.Charge) + 25*float64(g.Pressed) + 10*r.rng.Float64(); sc > bs {
			bs, gw = sc, gi
		}
	}
	G := r.Gens[gw].Pos
	lo, hi := r.RingPoly[0], r.RingPoly[0]
	for _, p := range r.RingPoly {
		lo, hi = V(math.Min(lo.X, p.X), math.Min(lo.Z, p.Z)), V(math.Max(hi.X, p.X), math.Max(hi.Z, p.Z))
	}
	ok := func(s V2, in, dev float64) bool {
		if !InPoly(r.RingPoly, s) || EdgeDist(r.RingPoly, s) < in || Dist(s, r.Dev.Pos) < dev {
			return false
		}
		for _, f := range r.Fixtures {
			if Dist(s, f) < 0.75 {
				return false
			}
		}
		return true
	}
	// as far in as the room allows, then as near that generator as it can be
	for _, lv := range [][2]float64{{TurretRoom, 1.0}, {0.9, 1.0}, {0.65, 0.9}, {0.4, 0.75}, {0.3, 0.65}} {
		best, bd := V2{}, math.Inf(1)
		for x := lo.X; x <= hi.X; x += 0.1 {
			for z := lo.Z; z <= hi.Z; z += 0.1 {
				s := V(x, z)
				if d := Dist(s, G); d < bd && d >= 0.6 && ok(s, lv[0], lv[1]) {
					best, bd = s, d
				}
			}
		}
		if !math.IsInf(bd, 1) {
			return best, gw, true
		}
	}
	return V2{}, -1, false
}

// TurretRoom: how far in from the ring the turret stands when the room allows (m)
const TurretRoom = 1.2

// SetMuzzle: the headset side keeps the spear's exit point up to date as you aim.
func (r *Round) SetMuzzle(p V3) { r.Harp.Muzzle = p }

// Fire the harpoon from muzzle along dir. Needs a loaded spear and power.
func (r *Round) Fire(muzzle, dir V3) bool {
	h := &r.Harp
	if r.Phase != Playing || !h.Up || !h.Armed || !h.Powered {
		return false
	}
	r.releaseShot()
	h.Armed = false
	h.ReelShot, h.ReelLeft = h.Reels > 0 && !h.Super, 0
	if h.ReelShot {
		h.ReelLeft = h.Reels - 1
	}
	h.Reels = 0
	h.Kind, h.Muzzle, h.Tip, h.Vel, h.Age = ShotFlying, muzzle, muzzle, dir.Norm().Mul(30), 0
	h.Live, h.Super = h.Super, false
	r.emit(EvFire, muzzle.XZ(), 0, 0, "")
	return true
}

// Reel: you pulled d metres of line in, hand over hand.
func (r *Round) Reel(d float64) {
	h := &r.Harp
	if h.Kind == ShotRobot || h.Kind == ShotDevice {
		h.Line = math.Max(0.3, h.Line-d)
	}
}

// HoldLine: whether a hand is gripping the line (so the target can't pull it back out).
func (r *Round) HoldLine(held bool) { r.Harp.Held = held }

type spearHit struct {
	kind int // 0 warden, 1 device, 2 floor/wall, 3 drone
	p    V3
	b    *Robot
}

// The cart's box as the spear sees it (it matches the drawn housing, a little generous).
const (
	DevHalfW = 0.25 // across the cart
	DevHalfL = 0.33 // along it
	DevTop   = 1.04 // the lid
)

// inCart: whether p is inside the cart's housing (carried a little off the floor, or lying
// on its side when it's been dropped).
func (r *Round) inCart(p V3) bool {
	d := &r.Dev
	f := V(math.Cos(d.Yaw), math.Sin(d.Yaw))
	s := V(-f.Z, f.X)
	q := p.XZ().Sub(d.Pos)
	lx, lz := math.Abs(q.Dot(s)), math.Abs(q.Dot(f))
	const pad = 0.06
	switch d.State {
	case DevDropped:
		return lx < DevTop*0.6+pad && lz < DevHalfL+pad && p.Y >= 0 && p.Y <= 2*DevHalfW+0.3
	case DevStolen:
		return lx < DevHalfW+pad && lz < DevHalfL+pad && p.Y >= 0.1 && p.Y <= DevTop+0.22+pad
	}
	return lx < DevHalfW+pad && lz < DevHalfL+pad && p.Y >= 0 && p.Y <= DevTop+pad
}

// SpearHitsCart: whether a spear at p would stick in the cart (for tests).
func (r *Round) SpearHitsCart(p V3) bool { return r.inCart(p) }

func (r *Round) spearCheck(p V3) *spearHit {
	d := &r.Dev
	if (d.State == DevStolen || d.State == DevDropped) && r.inCart(p) {
		return &spearHit{kind: 1, p: p}
	}
	for _, b := range r.Robots {
		if !b.Alive() {
			continue
		}
		if b.Kind == Drone {
			if p.Sub(b.Pos.At(b.Height())).Len() < 0.3 {
				return &spearHit{kind: 3, p: p, b: b}
			}
			continue
		}
		if Dist(p.XZ(), b.Pos) < b.Radius()+0.12 && p.Y >= 0 && p.Y <= b.Height()+0.1 {
			if b.State == RCarry {
				return &spearHit{kind: 1, p: p, b: b}
			}
			return &spearHit{kind: 0, p: p, b: b}
		}
	}
	if p.Y <= 0.01 {
		p.Y = 0
		return &spearHit{kind: 2, p: p}
	}
	if p.Y >= 5.0 || !r.Hall.Inside(p.XZ(), 0.02) {
		return &spearHit{kind: 2, p: p}
	}
	return nil
}

// Tractor: with no spear loaded, the harpoon can still haul the cart home when it's lying
// on the floor (its carrier died): aim at it, pull the trigger.
func (r *Round) Tractor(muzzle, dir V3) bool {
	h := &r.Harp
	d := &r.Dev
	if r.Phase != Playing || !h.Up || h.Beam || d.State != DevDropped || h.Kind == ShotFlying || h.Kind == ShotDevice {
		return false
	}
	dir = dir.Norm()
	to := d.Pos.At(0.35).Sub(muzzle)
	t := to.Dot(dir)
	if t <= 0 || t > 25 || to.Sub(dir.Mul(t)).Len() > 0.6+0.08*t {
		return false
	}
	h.Beam = true
	r.emit(EvTractor, d.Pos, 0, 0, "")
	return true
}

// TractorSpeed: how fast the beam hauls the cart (m/s); the wardens can't take it from it
const TractorSpeed = 1.1

func (r *Round) stepTractor(dt float64) {
	h := &r.Harp
	d := &r.Dev
	if !h.Beam {
		return
	}
	if !h.Up || d.State != DevDropped || r.Phase != Playing {
		h.Beam = false
		return
	}
	m := h.Pos
	if dd := Dist(d.Pos, m); dd > 0.05 {
		d.Pos = d.Pos.Add(m.Sub(d.Pos).Norm().Mul(math.Min(dd, TractorSpeed*dt)))
	}
	d.DropT = math.Max(d.DropT, 0.5) // nobody picks it up out of the beam
	home := len(r.RingPoly) >= 3 && InPoly(r.RingPoly, d.Pos)
	if len(r.RingPoly) < 3 && InPoly(r.Room, d.Pos) && Dist(d.Pos, m) < 1.1 {
		home = true
	}
	if home {
		d.State, d.Carrier, d.Held, d.Exposed, d.Grabber = DevHome, 0, false, false, 0
		r.Stats.Saves++
		r.emit(EvRecovered, d.Pos, 0, 2, "")
		h.Beam = false
	}
}

// sweepLine: a live line strung from the muzzle: swing the turret and the line swings with
// it - anything it sweeps through or touches gets zapped.
func (r *Round) sweepLine() {
	h := &r.Harp
	m0, m1, tip := h.prevMuzzle.XZ(), h.Muzzle.XZ(), h.Tip.XZ()
	moved := h.prevMuzzle != (V3{}) && Dist(m0, m1) > 1e-4
	h.prevMuzzle = h.Muzzle
	if !h.Powered {
		return
	}
	for _, b := range r.Robots {
		if !b.Alive() || b.State == RHooked {
			continue
		}
		dd, u := SegDist(b.Pos, m1, tip)
		hit := dd < b.Radius()+0.04
		if !hit && moved && inTri(b.Pos, m0, m1, tip) {
			_, u = SegDist(b.Pos, m1, tip)
			hit = true
		}
		if !hit {
			continue
		}
		y := h.Muzzle.Y + (h.Tip.Y-h.Muzzle.Y)*u
		lo, hi := 0.05, b.Height()+0.05
		if b.Kind == Drone {
			lo, hi = b.Height()-0.35, b.Height()+0.35
		}
		if y >= lo && y <= hi {
			r.wireZap(b, Lerp(m1, tip, u))
		}
	}
}

// inTri: whether p lies inside triangle abc (either winding).
func inTri(p, a, b, c V2) bool {
	d1 := b.Sub(a).Cross(p.Sub(a))
	d2 := c.Sub(b).Cross(p.Sub(b))
	d3 := a.Sub(c).Cross(p.Sub(c))
	neg := d1 < 0 || d2 < 0 || d3 < 0
	pos := d1 > 0 || d2 > 0 || d3 > 0
	return !(neg && pos)
}

func (r *Round) stepHarpoon(dt float64) {
	h := &r.Harp
	h.Powered = h.Up // it has its own power now: it works wherever your ring is
	r.stepTractor(dt)
	if h.Kind == ShotAnchor {
		r.sweepLine()
	} else {
		h.prevMuzzle = V3{}
	}
	r.lineCharge(dt)
	switch h.Kind {
	case ShotFlying:
		h.Age += dt
		steps := int(h.Vel.Len()*dt/0.08) + 1
		sdt := dt / float64(steps)
		for s := 0; s < steps; s++ {
			np := h.Tip.Add(h.Vel.Mul(sdt))
			h.Vel.Y -= 1.5 * sdt
			hit := r.spearCheck(np)
			h.Tip = np
			if hit == nil {
				continue
			}
			switch hit.kind {
			case 3:
				r.kill(hit.b, "spear")
				r.emit(EvHit, np.XZ(), 3, hit.b.ID, "")
				continue // straight through it
			case 0:
				b := hit.b
				if h.Live {
					// a supercharged spear: the shock kills it outright and the spear flies
					// on through it
					r.shock(b, np)
					continue
				}
				if r.Dev.Grabber == b.ID {
					r.Dev.Grabber = 0
				}
				b.State, b.Path, b.SpanA, b.SpanB, b.Pinned = RHooked, nil, -1, -1, false
				h.Kind, h.Target, h.Line = ShotRobot, b.ID, Dist(h.Pos, b.Pos)
				r.emit(EvHit, np.XZ(), 0, b.ID, "")
				// reeling one in locks the gun into reel mode for one more go
				if !h.ReelShot {
					h.ReelLeft = 1
				}
				r.reloadReel()
			case 1:
				// the spear going into the cart sets off a little EMP of its own: every
				// warden near it is left stumbling about for a good while (a supercharged
				// spear fries the one hauling it outright; otherwise it just lets go)
				if c := r.robot(r.Dev.Carrier); c != nil && c.Alive() {
					if h.Live {
						r.emit(EvShock, c.Pos, c.ID, int(c.Kind), "")
						r.kill(c, "shock")
					} else {
						r.dropCart(c)
					}
				}
				r.emit(EvMiniEMP, r.Dev.Pos, 0, 0, "")
				r.dazeAround(r.Dev.Pos, 0, RescueDaze, RescueReach)
				h.Kind, h.Line = ShotDevice, Dist(h.Pos, r.Dev.Pos)
				r.emit(EvHit, np.XZ(), 1, 0, "")
				r.reloadReel()
			default:
				r.anchorSpear(np)
			}
			return
		}
		if h.Age > 2.5 {
			h.Tip.Y = 0
			r.anchorSpear(h.Tip)
		}
	case ShotRobot:
		b := r.robot(h.Target)
		if b == nil || !b.Alive() || b.State != RHooked {
			h.Kind = ShotNone
			return
		}
		if !h.Held {
			pull := 0.25
			if b.Kind == Heavy {
				pull = 0.5
			}
			h.Line = math.Min(32, h.Line+pull*dt)
		}
		old := b.Pos
		r.drag(b, h.Line, dt)
		b.Prev = old
		r.contactChecks(b)
		h.Tip = b.Pos.At(b.Height() * 0.6)
	case ShotDevice:
		d := &r.Dev
		carrier := r.robot(d.Carrier)
		if d.State == DevStolen && carrier != nil && carrier.Alive() && !h.Held {
			h.Line = math.Min(32, h.Line+r.T.CarrySpeed/FtPerM*0.5*dt)
		}
		m := h.Pos // the winch reels it in to the turret's base, inside your ring
		if h.Live {
			// the line's live: anything in the way of it fries
			for _, b := range r.Robots {
				if b.Alive() && b.Kind != Drone && b.State != RHooked {
					if dd, _ := SegDist(b.Pos, h.Muzzle.XZ(), d.Pos); dd < b.Radius()+0.1 {
						r.shock(b, b.Pos.At(1))
					}
				}
			}
			carrier = r.robot(d.Carrier)
		}
		if dd := Dist(m, d.Pos); dd > h.Line {
			step := math.Min(dd-h.Line, 1.2*dt+0.02)
			d.Pos = d.Pos.Add(m.Sub(d.Pos).Norm().Mul(step))
			if carrier != nil && carrier.Alive() && d.State == DevStolen {
				old := carrier.Pos
				f := V(math.Cos(carrier.Yaw), math.Sin(carrier.Yaw))
				carrier.Pos = d.Pos.Sub(f.Mul(carrier.Radius() + 0.12))
				carrier.Prev = old
				r.contactChecks(carrier)
			}
		} else if d.State == DevStolen && carrier != nil && carrier.Alive() && !h.Held {
			// the carrier walks off along its route, dragging the line out
			r.follow(carrier, r.T.CarrySpeed/FtPerM*0.5, dt)
			r.carryDevice(carrier)
		}
		home := len(r.RingPoly) >= 3 && InPoly(r.RingPoly, d.Pos)
		if len(r.RingPoly) < 3 && InPoly(r.Room, d.Pos) && Dist(d.Pos, m) < 1.1 {
			home = true // no ring left: back beside the turret is as home as it gets
		}
		if home {
			if carrier != nil && carrier.Alive() {
				if carrier.Kind == Heavy {
					carrier.State, carrier.Wait = RWait, 3
				} else {
					r.kill(carrier, "green")
				}
			}
			d.State, d.Carrier, d.Held, d.Exposed, d.Grabber = DevHome, 0, false, false, 0
			r.Stats.Saves++
			r.emit(EvRecovered, d.Pos, 0, 2, "")
			h.Kind = ShotNone
			return
		}
		h.Tip = d.Pos.At(0.6)
	}
}

// drag: pull a hooked warden toward the turret until it's within the line's length. Haul
// it into your green ring or through a magenta wall and the beam fries it.
func (r *Round) drag(b *Robot, line, dt float64) {
	m := r.Harp.Pos // the line runs back to the winch on the turret's base, inside the ring
	d := Dist(m, b.Pos)
	if d <= line {
		return
	}
	want := math.Min(d-line, HaulSpeed*dt+0.02)
	dir := m.Sub(b.Pos).Norm()
	moved := 0.0
	for moved < want {
		st := math.Min(0.04, want-moved)
		np := b.Pos.Add(dir.Mul(st))
		through := len(r.RingPoly) >= 3 && InPoly(r.RingPoly, np) && !InPoly(r.RingPoly, b.Pos)
		for _, w := range r.Maze {
			if _, _, hit := SegHit(b.Pos, np, w.A, w.B); hit {
				through = true
			}
		}
		if through {
			// dragged through a beam: fried
			r.emit(EvDragFry, np, b.ID, int(b.Kind), "")
			r.kill(b, "dragged")
			return
		}
		b.Pos = np
		moved += st
	}
	b.Yaw = math.Atan2(-dir.Z, -dir.X)
}

// HaulSpeed: how fast a hooked warden can be hauled in (m/s) - a hard yank shoves it.
const HaulSpeed = 4.0

// shock: a supercharged spear's jolt - b fries, and every warden near it is left reeling.
func (r *Round) shock(b *Robot, at V3) {
	r.emit(EvShock, b.Pos, b.ID, int(b.Kind), "")
	r.kill(b, "shock")
	r.dazeAround(at.XZ(), b.ID, DazeSecs, ShockDaze)
}

// shockAround: wardens near p are dazed for a few seconds.
func (r *Round) shockAround(p V2, except int) { r.dazeAround(p, except, DazeSecs, ShockDaze) }

// dazeAround: wardens within reach of p are dazed for secs (they drop whatever they were
// doing, and a grab on the cart is let go).
func (r *Round) dazeAround(p V2, except int, secs, reach float64) {
	for _, o := range r.Robots {
		if !o.Alive() || o.ID == except || o.Kind == Drone || o.State == RHooked {
			continue
		}
		if Dist(o.Pos, p) > reach {
			continue
		}
		if r.Dev.Grabber == o.ID {
			r.Dev.Grabber = 0
		}
		if o.State == RCarry && r.Dev.Carrier == o.ID {
			continue // the one hauling the cart: the spear's for it
		}
		o.State, o.Wait, o.Path, o.Pinned = RWait, secs, nil, false
		o.SpanA, o.SpanB = -1, -1
		o.Dazed = secs
	}
}

// ShockDaze: how far a shock's daze reaches (m); DazeSecs: how long they reel
const (
	ShockDaze = 2.5
	DazeSecs  = 5.0
	// shooting the stolen cart with a supercharged spear dazes them far longer and wider:
	// you need the time to get it home
	RescueDaze  = 14.0
	RescueReach = 4.0
)

// dropCart: the warden hauling the cart lets go of it (it's left on the floor).
func (r *Round) dropCart(c *Robot) {
	d := &r.Dev
	if d.State != DevStolen || d.Carrier != c.ID {
		return
	}
	d.Carrier, d.State, d.DropT = 0, DevDropped, r.T.Regrab
	c.State, c.Wait, c.Path = RWait, 1, nil
	r.emit(EvDropped, d.Pos, 0, 0, "")
}

// anchorSpear: the spear's stuck in the floor or a wall at p: a live wire - unless it was a
// reel-only spear, which just drops (and the next reel-only one loads).
func (r *Round) anchorSpear(p V3) {
	h := &r.Harp
	if h.ReelShot {
		h.Kind, h.Tip = ShotNone, p
		r.emit(EvDud, p.XZ(), 0, 0, "")
		r.reloadReel()
		return
	}
	h.Kind = ShotAnchor
	r.emit(EvHit, p.XZ(), 2, 0, "")
}

// reloadReel: load the next reel-only spear, if this run of them has one left.
func (r *Round) reloadReel() {
	h := &r.Harp
	h.ReelShot = false
	if h.ReelLeft > 0 && !h.Armed {
		h.Armed, h.Reels, h.ReelLeft = true, h.ReelLeft, 0
		r.emit(EvArmed, h.Pos, h.Gen, 2, "reel")
	}
	h.ReelLeft = 0
}

// SuperHarpoon: spend your supercharge on the turret - it loads a spear if it was spent,
// and the next one it fires is supercharged.
func (r *Round) SuperHarpoon() bool {
	h := &r.Harp
	if r.Phase != Playing || !h.Up || h.Super || r.SuperCD > 0 || h.Kind == ShotFlying {
		return false
	}
	h.Armed, h.Super, h.Reels = true, true, 0 // a supercharged spear's a full one
	r.SuperCD = r.T.SuperCool
	r.emit(EvHarpSuper, h.Pos, h.Gen, 0, "")
	return true
}

// TieOff: unhook the line from a spear stuck in the floor or a wall and tie it to the
// turret's base: it stays up as a live trap, and the turret is free to shoot again.
func (r *Round) TieOff(base V3) bool {
	h := &r.Harp
	if h.Kind != ShotAnchor && h.Kind != ShotLoose {
		return false
	}
	h.Traps = append(h.Traps, Wire{base, h.Tip, h.LineT})
	if len(h.Traps) > MaxTraps {
		h.Traps = h.Traps[1:]
	}
	h.Kind, h.Line, h.LineT = ShotNone, 0, 0
	r.emit(EvTiedOff, base.XZ(), len(h.Traps), 0, "")
	return true
}

// DetachLine: you take hold of a line stuck in the floor and unhook it from the gun - it
// goes dead in your hand until you tie it off at the base (it can't go back on the gun).
func (r *Round) DetachLine() bool {
	h := &r.Harp
	if h.Kind != ShotAnchor {
		return false
	}
	h.Kind = ShotLoose
	r.emit(EvLineFree, h.Pos, 0, 0, "")
	return true
}

// DropLine: let go of an unhooked line without tying it off: it's gone.
func (r *Round) DropLine() {
	h := &r.Harp
	if h.Kind != ShotLoose {
		return
	}
	h.Kind, h.LineT = ShotNone, 0
	r.emit(EvLineDrop, h.Tip.XZ(), 0, 0, "")
}

// LineLife: a live line strung through a magenta wall keeps the wall supercharged this long
// (seconds), then the whole line burns out.
const LineLife = 60.0

func (r *Round) lineCharge(dt float64) {
	h := &r.Harp
	if !h.Powered {
		return
	}
	charges := func(a, b V3) bool {
		hit := false
		for _, w := range r.Maze {
			if t, _, ok := SegHit(a.XZ(), b.XZ(), w.A, w.B); ok {
				if y := a.Y + (b.Y-a.Y)*t; y >= 0 && y <= r.T.WallHeight+0.1 {
					w.Super = math.Max(w.Super, 0.3)
					hit = true
				}
			}
		}
		return hit
	}
	if h.Kind == ShotAnchor && charges(h.Muzzle, h.Tip) {
		if h.LineT += dt; h.LineT >= LineLife {
			r.emit(EvLineGone, Lerp(h.Muzzle.XZ(), h.Tip.XZ(), 0.5), 0, 0, "")
			h.Kind, h.LineT = ShotNone, 0
		}
	}
	keep := h.Traps[:0]
	for _, w := range h.Traps {
		if charges(w.A, w.B) {
			if w.T += dt; w.T >= LineLife {
				r.emit(EvLineGone, Lerp(w.A.XZ(), w.B.XZ(), 0.5), 1, 0, "")
				continue
			}
		}
		keep = append(keep, w)
	}
	h.Traps = keep
}
