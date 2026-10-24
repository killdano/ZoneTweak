package main

// Force pull: point at a loose tool (the screwdriver, the pole, the flashlight) and ease the
// trigger in - an electric arc locks onto it - then flick your hand back toward you and it
// flies over to your hand. Squeeze to catch it (it hangs there a moment waiting for you).

import "math"

const (
	pullMinD  = 0.55 // closer than this you just reach for it
	pullReach = 7.0  // how far the arc reaches
	pullFlick = 1.1  // m/s of hand speed back toward you (or upward) that throws the pull
	pullWait  = 1.0  // seconds it hangs at your hand waiting to be caught
	pullKeep  = 0.5  // how far off the lock can drift (cos of the angle) before it lets go
)

// pullSt: one hand's pull: what it's locked onto and where that is in its flight.
type pullSt struct {
	p      *prop
	phase  int // pullNone, pullLocked, pullFlying, pullWaiting
	t, dur float64
	from   vec3
	axes   [3]vec3 // its orientation when it took off
	buzzT  float64
}

const (
	pullNone = iota
	pullLocked
	pullFlying
	pullWaiting
)

// pullables: the loose tools you can pull to you.
func (g *game) pullables() []*prop {
	var out []*prop
	if g.driver != nil {
		out = append(out, g.driver.p)
	}
	if g.pole != nil && !g.pole.broken {
		out = append(out, g.pole.p)
	}
	if g.devVis.flash != nil {
		out = append(out, g.devVis.flash)
	}
	if g.wrench != nil {
		out = append(out, g.wrench.p)
	}
	if g.misfire != nil {
		out = append(out, g.misfire.cutters)
	}
	return out
}

// pullTarget: the tool the beam from o along d points at, if any (generous: the further
// away, the wider the cone).
func (g *game) pullTarget(gh *gHand, o, d vec3) *prop {
	var best *prop
	bs := 1.0
	for _, p := range g.pullables() {
		if p.held || !p.gOK() || g.handHolding(p) != nil {
			continue
		}
		c := p.pos
		if p == g.pole.p {
			_, head := g.pole.ends()
			c = p.pos.add(head).mul(0.5) // the middle of the pole
		}
		if c.sub(gh.palm).length() < pullMinD {
			continue
		}
		t := c.sub(o).dot(d)
		if t <= 0 || t > pullReach {
			continue
		}
		off := c.sub(o.add(d.mul(t))).length()
		if s := off / (0.1 + 0.05*t); s < bs {
			bs, best = s, p
		}
	}
	return best
}

// heldPose: where a tool would sit in this hand (its fixed hold, or as it is).
func heldPose(gh *gHand, p *prop) (vec3, [3]vec3) {
	F := handXf(gh)
	w := func(v vec3) vec3 { return F.x.mul(v.x).add(F.y.mul(v.y)).add(F.z.mul(v.z)) }
	if p.grip != nil {
		return F.o.add(w(p.grip[0])), [3]vec3{w(p.grip[1]), w(p.grip[2]), w(p.grip[3])}
	}
	return F.o, [3]vec3{p.X, p.Y, p.Z}
}

func (g *game) pullFrame(dt float64) {
	for _, gh := range hands {
		if !gh.valid() {
			continue
		}
		ps := &gh.pull
		p := ps.p
		// someone took hold of it (this hand catching it, or the other one): done
		if p != nil && g.handHolding(p) != nil {
			if g.handHolding(p) == gh {
				gh.buzz(40, 0.7)
			}
			*ps = pullSt{}
			continue
		}
		switch ps.phase {
		case pullNone:
			if gh.held != nil || !gh.beam || gh.building || gh.trig < 0.12 || g.fitting || g.menuOpen {
				continue
			}
			if q := g.pullTarget(gh, gh.aim.pos(), gh.aim.axis(2).mul(-1)); q != nil {
				*ps = pullSt{p: q, phase: pullLocked}
				gh.buzz(25, 0.5)
				ed.sfx(sndZap, q.pos, 0.3)
			}
		case pullLocked:
			o, d := gh.aim.pos(), gh.aim.axis(2).mul(-1)
			to := p.pos.sub(o)
			if gh.held != nil || gh.trig < 0.08 && !gh.trigHeld || p.held || to.norm().dot(d) < pullKeep {
				*ps = pullSt{}
				continue
			}
			ps.t += dt
			ps.buzzT -= dt
			if ps.buzzT <= 0 {
				ps.buzzT = 0.1
				gh.buzz(8, 0.25)
			}
			// the flick: the hand snapping back toward you, or up
			back := to.norm().mul(-1)
			if ps.t > 0.15 && (gh.vel.dot(back) > pullFlick || gh.vel.y > pullFlick*1.2) {
				ps.phase, ps.t, ps.from = pullFlying, 0, p.pos
				ps.axes = [3]vec3{p.X, p.Y, p.Z}
				ps.dur = math.Max(0.25, math.Min(0.7, to.length()/5))
				p.held, p.docked, p.resting, p.onCart, p.vel = true, false, false, false, vec3{}
				gh.buzz(60, 1)
				ed.sfx(sndZip, p.pos, 0.8)
				spawnSparks(p.pos, v3(0, 1, 0), 25, 0.8)
			}
		case pullFlying:
			ps.t += dt
			u := math.Min(1, ps.t/ps.dur)
			at, ax := heldPose(gh, p)
			// a quick arc over to the hand, turning to sit the way you'll hold it
			mid := ps.from.add(at).mul(0.5).add(v3(0, 0.15+0.08*ps.from.sub(at).length(), 0))
			e := 1 - (1-u)*(1-u)
			q := ps.from.mul((1 - e) * (1 - e)).add(mid.mul(2 * e * (1 - e))).add(at.mul(e * e))
			if dt > 0 {
				p.vel = q.sub(p.pos).mul(1 / dt)
			}
			p.pos = q
			k := smooth(u)
			for i := range ax {
				ax[i] = ps.axes[i].mul(1 - k).add(ax[i].mul(k)).norm()
			}
			p.Z = ax[2]
			p.X = ax[1].cross(p.Z).norm()
			p.Y = p.Z.cross(p.X).norm()
			if rngF() < 0.5 {
				spawnSparks(p.pos, p.vel.mul(-1).norm(), 2, 0.3)
			}
			if u >= 1 {
				ps.phase, ps.t = pullWaiting, 0
				if gh.closed && gh.held == nil {
					g.catchPulled(gh, p)
				}
			}
		case pullWaiting:
			ps.t += dt
			at, ax := heldPose(gh, p)
			p.pos = at
			p.X, p.Y, p.Z = ax[0], ax[1], ax[2]
			switch {
			case gh.held == nil && gh.grabDown:
				g.catchPulled(gh, p)
			case gh.held != nil || ps.t > pullWait:
				// not caught: it drops
				p.held, p.vel = false, gh.vel.mul(0.5)
				*ps = pullSt{}
			}
		}
	}
}

// catchPulled: the pulled tool lands in this hand.
func (g *game) catchPulled(gh *gHand, p *prop) {
	p.held = false
	gh.held, gh.holdT, gh.hover = p, 0, nil
	p.gGrab(gh)
	gh.buzz(50, 0.8)
	ed.sfx(sndTick, p.pos, 0.5)
	gh.pull = pullSt{}
}

// drawPulls: the arc a locked pull hangs on, and its tool glowing.
func (g *game) drawPulls() {
	t := fxTime()
	for _, gh := range hands {
		if !gh.valid() || gh.pull.p == nil {
			continue
		}
		ps := &gh.pull
		p := ps.p
		switch ps.phase {
		case pullLocked:
			o := gh.aim.pos()
			for q := 0; q < 3; q++ {
				arc(o, p.pos, cBeam)
			}
			ribbon(o, p.pos, 0.05, cBeam.alpha(0.18))
			addLine(o, p.pos, cBeam, 1.5, 2)
			sprite(p.pos, 0.18+0.03*math.Sin(t*20), cBeam.alpha(0.6))
			ringSoft(p.pos.add(v3(0, 0.02, 0)), 0.12+0.01*math.Sin(t*9), 0.01, cBeam.alpha(0.8), 24)
			addPoint(p.pos, cBeam, 0.8, 1.5)
			cam := ed.hmd.pos()
			q := p.pos.add(v3(0, 0.2, 0))
			rr, uu := billboard(cam, q)
			drawTextCentered(&bOpaque, "FLICK TO PULL", q, rr, uu, 0.0012*math.Max(1, q.sub(cam).length()), cBeam)
		case pullFlying:
			sprite(p.pos, 0.12, cBeam.alpha(0.5))
			if rngF() < 0.5 {
				arc(gh.palm, p.pos, cBeam)
			}
		case pullWaiting:
			sprite(p.pos, 0.1+0.02*math.Sin(t*12), cBeam.alpha(0.4))
		}
	}
}
