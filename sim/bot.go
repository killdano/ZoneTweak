package sim

import (
	"math"
	"math/rand"
)

// Bot plays a round the way the Balance Lab's simulated player does: it walks around your
// room at real walking speed and always does the most urgent thing first. It drives the
// same calls the headset does, so headless tests exercise the real rules.
type Bot struct {
	R     *Round
	Pos   V2
	Speed float64 // m/s
	Fail  float64 // device-game failures per second of game work
	Aim   float64 // aiming error, radians
	React float64 // seconds to notice something and turn to it (a person, not a machine)
	Slow  float64 // how much longer hands-on jobs take than their book time (fumbling in VR)
	busy  float64
	done  func()
	Job   string
	rng   *rand.Rand
	fixT  float64 // time spent on a misfire so far
}

func NewBot(r *Round, seed int64) *Bot {
	return &Bot{R: r, Pos: Centroid(r.Room), Speed: 0.9, Fail: 0.4 / 40, Aim: 0.03, React: 0.9, Slow: 1.3, rng: rand.New(rand.NewSource(seed))}
}

// BuildMaze: baffles across the approach to each door, alternating sides, spending about
// frac of the points (the Balance Lab player builds the walkers' maze to ~80% of kamikaze).
func (b *Bot) BuildMaze(frac float64) {
	r := b.R
	budget := r.Points * frac / float64(len(r.Doors))
	h := r.Hall
	for di, d := range r.Doors {
		spend := 0.0
		in := d.Out.Mul(-1)
		side := d.Out.Perp()
		// the gap between the hall wall and your room on this side
		gap := 0.0
		for k := 0.5; k < 30; k += 0.25 {
			p := d.Mouth.Add(in.Mul(k))
			if InPoly(r.Room, p) || EdgeDist(r.Room, p) < 1.0 {
				break
			}
			gap = k
		}
		row := 0
		for k := 1.6; k < gap-0.8 && spend < budget; k += 1.5 {
			c := d.Mouth.Add(in.Mul(k))
			w := math.Min(4.0, (budget-spend)/r.T.MazeCost())
			if w < 1.0 {
				break
			}
			off := 0.9
			if (row+di)%2 == 1 {
				off = -0.9
			}
			a := c.Add(side.Mul(off - w/2))
			e := c.Add(side.Mul(off + w/2))
			for w >= 1.0 {
				if !h.Inside(a, 0.6) || !h.Inside(e, 0.6) {
					w -= 0.5
					a, e = c.Add(side.Mul(off-w/2)), c.Add(side.Mul(off+w/2))
					continue
				}
				if _, why := r.PlaceWall(a, e); why == "" {
					spend += Dist(a, e) * r.T.MazeCost()
					break
				}
				w -= 0.5
				a, e = c.Add(side.Mul(off-w/2)), c.Add(side.Mul(off+w/2))
			}
			row++
		}
	}
}

func (b *Bot) walk(to V2) float64 {
	t := Dist(b.Pos, to) / b.Speed
	b.Pos = to
	return t
}

func (b *Bot) do(job string, secs float64, f func()) {
	if job != "work" && job != "reel" {
		secs = secs*b.Slow + b.React
	}
	b.Job, b.busy, b.done = job, secs, f
}

func (b *Bot) Step(dt float64) {
	r := b.R
	if r.Phase != Playing {
		return
	}
	if r.Harp.Kind == ShotRobot || r.Harp.Kind == ShotDevice {
		r.HoldLine(true)
		r.Reel(0.7 * dt)
		if b.Job == "reel" {
			return
		}
	} else {
		r.HoldLine(false)
		if b.Job == "reel" {
			b.Job, b.busy = "", 0
		}
	}
	if d := &r.Dev; d.State == DevHome && d.Exposed && !d.Held && b.Job != "carry" && b.busy > 0 && len(r.RingPoly) >= 3 {
		// a real player drops everything for this
		b.busy, b.done, b.Job = 0, nil, ""
	}
	if b.busy > 0 {
		b.busy -= dt
		if b.Job == "work" {
			b.workTick(dt)
		}
		if b.busy <= 0 && b.done != nil {
			f := b.done
			b.done = nil
			f()
		}
		return
	}
	b.decide()
}

func (b *Bot) safeSpot() V2 {
	r := b.R
	if len(r.RingPoly) < 3 {
		return r.Dev.Pos
	}
	return Centroid(r.RingPoly)
}

func (b *Bot) decide() {
	r := b.R
	d := &r.Dev
	reach := func(p V2) bool { return InPoly(r.Room, p) && EdgeDist(r.Room, p) > 0.2 }
	harpReady := r.Harp.Up && r.Harp.Armed && r.Harp.Powered && r.Harp.Kind != ShotFlying
	fireAt := func(tgt V3) {
		t := b.walk(r.Harp.Pos) + 1.2
		b.do("aim", t, func() {
			m := r.Harp.Pos.At(1.15)
			dir := tgt.Sub(m).Norm()
			dir.X += b.rng.NormFloat64() * b.Aim
			dir.Y += b.rng.NormFloat64() * b.Aim
			dir.Z += b.rng.NormFloat64() * b.Aim
			if r.Fire(m, dir) {
				b.Job, b.busy = "reel", 0
			}
		})
	}
	switch {
	case d.State == DevHome && d.Exposed && !d.Held && len(r.RingPoly) >= 3 && reach(d.Pos):
		to := b.safeSpot()
		t := b.walk(d.Pos) + Dist(d.Pos, to)/b.Speed + 1
		b.Pos = to
		b.do("carry", t, func() {
			if d.State == DevHome && d.Exposed {
				r.SetDevice(true, to, 0)
				r.SetDevice(false, to, 0)
			}
		})
		return
	case d.State == DevStolen:
		if c := r.robot(d.Carrier); c != nil && reach(c.Pos) {
			b.do("tug", b.walk(c.Pos)+1.2, func() {
				if r.Tug() {
					to := b.safeSpot()
					r.SetDevice(true, to, 0)
					r.SetDevice(false, to, 0)
				}
			})
			return
		}
		if harpReady {
			fireAt(d.Pos.At(0.5))
			return
		}
	case d.State == DevDropped:
		if reach(d.Pos) {
			b.do("fetch", b.walk(d.Pos)+1, func() {
				to := b.safeSpot()
				r.SetDevice(true, to, 0)
				r.SetDevice(false, to, 0)
			})
			return
		}
		if harpReady {
			fireAt(d.Pos.At(0.5))
			return
		}
	}
	// supercharge the span the most wardens are leaning on (no walking: it's the laser)
	if r.SuperCD <= 0 {
		count := map[[2]int]int{}
		for _, o := range r.Robots {
			if o.Alive() && (o.State == RPress || o.State == RHooked && o.Pinned) && o.SpanA >= 0 {
				w := 1
				if o.Kind == Heavy {
					w = 2
				}
				count[[2]int{o.SpanA, o.SpanB}] += w
			}
		}
		var best [2]int
		bn := 0
		for k, n := range count {
			if n > bn || n == bn && (k[0] < best[0] || k[0] == best[0] && k[1] < best[1]) {
				bn, best = n, k
			}
		}
		if bn >= 2 {
			a, c := best[0], best[1]
			b.do("super", 1.4, func() { r.SuperSpan(a, c) }) // aim, then hold A till it fills
			return
		}
	}
	// generators: drones first, then repairs (first of all with no ring left), then top-ups
	noRing := len(r.RingPoly) < 3
	best, bp := -1, 0.0
	for i, g := range r.Gens {
		p := 0.0
		switch {
		case g.Leech != 0 && g.State == GenUp:
			p = 3.5 // clamped on but not through yet: rip it off before it bites
		case g.State == GenLeeched:
			p = 3
		case g.State == GenBlown:
			p = 2
			if noRing {
				p = 4
			}
		case g.State == GenUp && g.Charge < 0.3*r.T.ChargeMax:
			p = 1 + (0.3 - g.Charge/r.T.ChargeMax)
			if d.Cur == len(d.Mods)-1 && d.State == DevHome {
				p = 0 // one stage from the end: finishing it beats topping up
			}
		}
		if p > bp {
			bp, best = p, i
		}
	}
	if best >= 0 {
		g := &r.Gens[best]
		at := g.Pos.Add(b.safeSpot().Sub(g.Pos).Norm().Mul(0.4))
		switch {
		case g.Leech != 0:
			id := g.Leech
			b.do("pluck", b.walk(at)+2.5, func() { r.Pluck(id) })
		case g.State == GenBlown:
			secs := 0.0
			for _, t := range g.Order {
				secs += r.taskSecs(t)
			}
			gi := best
			b.do("repair", b.walk(at)+secs, func() {
				for r.Gens[gi].State == GenBlown {
					r.RepairTask(gi)
				}
			})
		default:
			gi := best
			b.do("pump", b.walk(at)+2.5, func() {
				for k := 0; k < 12 && r.Gens[gi].Charge < r.T.ChargeMax; k++ {
					r.Pump(gi)
				}
			})
		}
		return
	}
	if d.State == DevHome {
		t := b.walk(d.Pos)
		if d.Leads < LeadsNeeded {
			// plug the power leads back in (pushing the cart back toward their box first if
			// it's out of their reach)
			b.do("plug", t+4, func() {
				if d.State != DevHome {
					return
				}
				if Dist(d.Pos, d.LeadBox) > LeadReach-0.3 {
					to := d.LeadBox.Add(d.Pos.Sub(d.LeadBox).Norm().Mul(1.0))
					if len(r.RingPoly) >= 3 && !InPoly(r.RingPoly, to) {
						to = d.LeadBox.Add(b.safeSpot().Sub(d.LeadBox).Norm().Mul(math.Min(1.0, Dist(d.LeadBox, b.safeSpot()))))
					}
					r.SetDevice(true, to, d.Yaw)
					r.SetDevice(false, to, d.Yaw)
				}
				r.SetLeads(LeadsNeeded)
			})
			return
		}
		b.do("work", t+0.25, nil)
	}
}

func (r *Round) taskSecs(id string) float64 {
	for _, t := range r.C.RepairTasks {
		if t.ID == id {
			return t.Secs
		}
	}
	return 3
}

// workTick: hands on the device. Games sometimes go wrong.
func (b *Bot) workTick(dt float64) {
	r := b.R
	m := r.Module()
	if m == nil || r.Dev.State != DevHome || Dist(b.Pos, r.Dev.Pos) > 0.8 {
		return
	}
	if m.ID == "game" && b.rng.Float64() < b.Fail*dt {
		r.Setback(r.T.Setback, "bot")
		return
	}
	if r.Dev.Misfire {
		// the top off, the guts up, what the radio says, closed up again
		if b.fixT += dt / b.Slow; b.fixT >= r.T.MisfireSecs {
			b.fixT = 0
			r.FixMisfire()
		}
		return
	}
	p := r.Dev.Prog + dt/(m.Secs*b.Slow)
	if p >= 1 {
		r.CompleteModule()
	} else {
		r.SetModuleProgress(p)
	}
}
