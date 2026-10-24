package main

// The cart's power leads: two heavy cables from a box on the floor plug into the back of the
// cart. The blast needs both in. A warden making off with the cart rips them out (so does
// dragging the cart further than they reach): plug them back in before you pull the cord.

import (
	"boundaryvr/sim"
	"math"
)

const leadCord = sim.LeadReach + 0.5 // the cable from the box to a plug (m)

var cLead = rgba{0.8, 0.36, 0.05, 1}

type leadKit struct {
	plugs [2]*prop
	sayT  float64
}

// leadSocket: where lead i plugs into the back of the cart, and the way out of it.
func (g *game) leadSocket(i int) (vec3, vec3) {
	F := g.devFrame()
	return F.p([]float64{-0.17, 0.17}[i], devMidY-0.1, -devHL-0.004), F.z.mul(-1)
}

// leadOutlet: where lead i's cable leaves the box on the floor.
func (g *game) leadOutlet(i int) vec3 {
	return fv(g.r.Dev.LeadBox, 0.11).add(v3([]float64{-0.045, 0.045}[i], 0, 0))
}

func (g *game) buildLeads() {
	k := &leadKit{}
	for i := range k.plugs {
		i := i
		p := &prop{radius: 0.08, hy: 0.025, cordLen: leadCord, homeR: 0.09}
		p.home = func() (vec3, vec3, vec3, vec3) {
			s, out := g.leadSocket(i)
			F := g.devFrame()
			return s.add(out.mul(0.035)), F.x, F.y, out
		}
		p.anchor = func() vec3 { return g.leadOutlet(i) }
		p.onRelease = func() {
			if p.docked && !g.leadCanPlug() {
				p.docked = false // nothing to plug it into just now
			}
			if p.docked {
				s, _ := g.leadSocket(i)
				ed.sfx(sndClang, s, 0.5)
				ed.sfx(sndRelay, s, 0.5)
				spawnSparks(s, v3(0, 1, 0), 6, 0.3)
				for _, h := range hands {
					if h != nil && h.held == nil && h.palm.sub(s).length() < 0.25 {
						h.buzz(40, 0.8)
					}
				}
			}
		}
		p.pos, p.X, p.Y, p.Z = p.home()
		p.docked = true
		k.plugs[i] = p
	}
	g.leads = k
}

// leadCanPlug: whether the cart's there to plug into (home, and within the leads' reach).
func (g *game) leadCanPlug() bool {
	d := &g.r.Dev
	return d.State == sim.DevHome && sim.Dist(d.Pos, d.LeadBox) <= sim.LeadReach
}

func (g *game) leadsFrame(dt float64) {
	k := g.leads
	if k == nil {
		return
	}
	n := 0
	for _, p := range k.plugs {
		if p.docked && !g.leadCanPlug() {
			g.popLead(p)
		}
		p.tick(dt)
		if p.docked {
			n++
		}
	}
	if g.r.Phase == sim.Playing && n != g.r.Dev.Leads {
		g.r.SetLeads(n)
	}
	k.sayT = math.Max(0, k.sayT-dt)
}

// popLead: a lead yanked out of the cart: it flies off back toward its box.
func (g *game) popLead(p *prop) {
	if !p.docked {
		return
	}
	p.docked, p.resting, p.onCart = false, false, false
	away := g.leadOutlet(0).sub(p.pos)
	away.y = 0
	if away.length() > 0.01 {
		away = away.norm()
	}
	p.vel = away.mul(1.6).add(v3(0, 1.2, 0))
	spawnSparks(p.pos, v3(0, 1, 0), 18, 0.7)
	ed.sfx(sndZap, p.pos, 0.6)
}

// unplugAll: the sim says the leads came out (a warden took the cart, or it went too far).
func (g *game) unplugAll() {
	if g.leads == nil {
		return
	}
	for _, p := range g.leads.plugs {
		if g.handHolding(p) == nil {
			g.popLead(p)
		}
	}
}

func (g *game) leadGrabbers(out []grabber) []grabber {
	if g.leads == nil {
		return out
	}
	for _, p := range g.leads.plugs {
		out = append(out, p)
	}
	return out
}

// drawLeads: the box, its two cables, the plugs and the sockets in the back of the cart.
func (g *game) drawLeads() {
	k := g.leads
	if k == nil {
		return
	}
	r := g.r
	d := &r.Dev
	t := fxTime()
	// the box
	b := fv(d.LeadBox, 0)
	useMat(mPaint)
	bevelBox(&bEnv, b.add(v3(0, 0.05, 0)), v3(1, 0, 0), v3(0, 1, 0), v3(0, 0, 1), 0.1, 0.05, 0.07, 0.006, rgba{0.2, 0.22, 0.2, 1})
	useMat(mHazard)
	bevelBox(&bEnv, b.add(v3(0, 0.103, 0)), v3(1, 0, 0), v3(0, 1, 0), v3(0, 0, 1), 0.1, 0.004, 0.07, 0.002, rgba{0.8, 0.6, 0.1, 1})
	topText(&bOpaque, "POWER", b.add(v3(0, 0.108, 0.045)), v3(0, 1, 0), 0.0011, cMuted)
	// the sockets in the back of the cart
	want := r.Phase == sim.Playing && d.State == sim.DevHome
	for i, p := range k.plugs {
		s, out := g.leadSocket(i)
		useMat(mCast)
		cyl(&bEnv, s.sub(out.mul(0.01)), s.add(out.mul(0.004)), 0.026, 0.026, 12, cIron, true)
		if want {
			c := cEnergy
			if !p.docked {
				c = cDanger.scale(float32(0.5 + 0.5*math.Sin(t*7)))
			}
			curEmis = 3
			sphere(&bEnv, s.add(v3(0, 0.045, 0)).add(out.mul(0.002)), 0.007, c, 3, 6)
			curEmis = 1
			if !p.docked {
				sprite(s.add(out.mul(0.01)), 0.07, cDanger.alpha(float32(0.25+0.2*math.Sin(t*7))))
			}
		}
		// the cable, then the plug on its end
		tail := p.pos.add(p.Z.mul(0.03))
		drawCord(cordPts(g.leadOutlet(i), tail, leadCord, 18), 0.011, cLead)
		useMat(mRubber)
		bevelBox(&bEnv, p.pos, p.X, p.Y, p.Z, 0.022, 0.022, 0.03, 0.005, rgba{0.1, 0.1, 0.1, 1})
		useMat(mPaint)
		bevelBox(&bEnv, p.pos.add(p.Z.mul(0.012)), p.X, p.Y, p.Z, 0.0235, 0.0235, 0.004, 0.002, rgba{0.85, 0.15, 0.1, 1})
		useMat(mChrome)
		for _, sx := range []float64{-1, 1} {
			q := p.pos.add(p.X.mul(sx * 0.009))
			cyl(&bEnv, q.sub(p.Z.mul(0.03)), q.sub(p.Z.mul(0.05)), 0.003, 0.003, 6, cChromeEnv, false)
		}
	}
	F := g.devFrame()
	faceText(&bOpaque, "POWER", F.p(0, devMidY-0.1, -devHL-0.006), F.z.mul(-1), F.y, 0.0012, cMuted)
	// out of reach of the box: say so on the cart's back
	if want && !g.leadCanPlug() {
		q := F.p(0, devMidY-0.16, -devHL-0.006)
		faceText(&bOpaque, "TOO FAR FROM THE POWER BOX", q, F.z.mul(-1), F.y, 0.0009, cDanger)
	}
}

// supportAt: what a dropped tool or part comes to rest on besides the floor and the cart: a
// generator's base (and the drum round its shaft), the turret's foot, the power box, the top
// of the radio. Anything that lands in a generator's shaft is nudged out onto its drum.
func (g *game) supportAt(was, p vec3) (float64, vec3, bool) {
	best, ok := 0.0, false
	var push vec3
	try := func(top float64, in bool) {
		if in && was.y >= top-0.03 && top > best {
			best, ok = top, true
		}
	}
	for _, gn := range g.r.Gens {
		dx, dz := p.x-gn.Pos.X, p.z-gn.Pos.Z
		d := math.Hypot(dx, dz)
		try(0.036, d < 0.3)
		try(0.135, d < 0.12)
		if d < 0.1 && p.y < 0.45 {
			ux, uz := 1.0, 0.0
			if d > 1e-4 {
				ux, uz = dx/d, dz/d
			}
			push = v3(ux*(0.11-d), 0, uz*(0.11-d))
			ok = true
		}
	}
	if h := &g.r.Harp; h.Up {
		try(0.02, sim.Dist(sim.V(p.x, p.z), h.Pos) < 0.33)
	}
	if b := g.r.Dev.LeadBox; g.leads != nil {
		try(0.106, math.Abs(p.x-b.X) < 0.1 && math.Abs(p.z-b.Z) < 0.07)
	}
	if rd := g.radio; rd != nil {
		try(1.03, math.Abs(p.x-rd.pos.X) < 0.2 && math.Abs(p.z-rd.pos.Z) < 0.12)
	}
	return best, push, ok
}
