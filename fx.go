package main

import (
	"math"
	"math/rand"
)

func glowLine(a, b vec3, col rgba, wide float32) {
	bGlowLine.line(a, b, col)
	w := 0.008 + 0.03*float64(wide)
	ribbon(a, b, w, col.alpha(col.a*0.45))
}

func neonLine(a, b vec3, col rgba) { glowLine(a, b, col, 0.5) }

func ringSoft(c vec3, r, w float64, col rgba, segs int) {
	for s := 0; s < segs; s++ {
		a0, a1 := 2*math.Pi*float64(s)/float64(segs), 2*math.Pi*float64(s+1)/float64(segs)
		p0 := v3(c.x+r*math.Cos(a0), c.y, c.z+r*math.Sin(a0))
		p1 := v3(c.x+r*math.Cos(a1), c.y, c.z+r*math.Sin(a1))
		bGlowLine.line(p0, p1, col)
		ribbon(p0, p1, w, col.alpha(col.a*0.4))
	}
}

// ringSoftAxes: ringSoft in any plane (u and w span it).
func ringSoftAxes(c, u, w vec3, r, wd float64, col rgba, segs int) {
	at := func(a float64) vec3 { return c.add(u.mul(r * math.Cos(a))).add(w.mul(r * math.Sin(a))) }
	for s := 0; s < segs; s++ {
		p0, p1 := at(2*math.Pi*float64(s)/float64(segs)), at(2*math.Pi*float64(s+1)/float64(segs))
		bGlowLine.line(p0, p1, col)
		ribbon(p0, p1, wd, col.alpha(col.a*0.4))
	}
}

func ringGlow(c vec3, r float64, col rgba) { ringSoft(c, r, 0.025, col, 40) }

func noise1(i, t float64) float64 {
	s := math.Sin(i*12.9898+t*(1.7+math.Mod(i*0.37, 1)*3.1)) * math.Sin(i*4.13+t*2.3+math.Sin(t*0.9+i))
	return 0.5 + 0.5*s
}

func hash1(x float64) float64 { return math.Mod(math.Abs(math.Sin(x*127.1)*43758.5453), 1) }

func (e *editor) roomFrame() (rigid2, float64, float64, float64, float64) {
	th := domAngle(e.pts)
	var cx, cz float64
	for _, p := range e.pts {
		cx += p[0]
		cz += p[1]
	}
	if len(e.pts) > 0 {
		cx /= float64(len(e.pts))
		cz /= float64(len(e.pts))
	}
	g := rigidFromYaw(th, cx, cz)
	inv := g.inv()
	x0, x1, z0, z1 := -2.0, 2.0, -2.0, 2.0
	for i, p := range e.pts {
		q := inv.apply(p)
		if i == 0 {
			x0, x1, z0, z1 = q[0], q[0], q[1], q[1]
		}
		x0, x1 = math.Min(x0, q[0]), math.Max(x1, q[0])
		z0, z1 = math.Min(z0, q[1]), math.Max(z1, q[1])
	}
	q := func(v float64, up bool) float64 {
		if up {
			return math.Ceil(v*2) / 2
		}
		return math.Floor(v*2) / 2
	}
	return g, q(x0, false), q(x1, true), q(z0, false), q(z1, true)
}

func (e *editor) manipulating() bool {
	if e.drawing {
		return true
	}
	for _, h := range e.hands {
		if h.grab >= 0 || h.areaGrab != 0 {
			return true
		}
	}
	return false
}

func (e *editor) wallTint(level float64, hot bool) rgba {
	c := cEnergy
	if hot {
		c = cAmber
	}
	if level > 0 {
		l := float32(level)
		c = rgba{c.r + (cDanger.r-c.r)*l, c.g + (cDanger.g-c.g)*l, c.b + (cDanger.b-c.b)*l, 1}
	}
	return c
}

type wallHit struct {
	ok   bool
	p, n vec3
	wall int
}

func (e *editor) rayWalls(o, d vec3) wallHit {
	best := wallHit{}
	bt := math.Inf(1)
	n := len(e.pts)
	for i := 0; i < n; i++ {
		if v, ok := wallPow[i]; ok && v < 0.05 && n == wallKeyN {
			continue // switched off: nothing there to hit
		}
		a, b := e.pts[i], e.pts[(i+1)%n]
		wn := v3(-(b[1] - a[1]), 0, b[0]-a[0]).norm()
		den := d.dot(wn)
		if math.Abs(den) < 1e-6 {
			continue
		}
		t := fl(a, 0).sub(o).dot(wn) / den
		if t <= 0.02 || t >= bt {
			continue
		}
		p := o.add(d.mul(t))
		_, u := segDist(pt2{p.x, p.z}, a, b)
		q := pt2{a[0] + (b[0]-a[0])*u, a[1] + (b[1]-a[1])*u}
		if dist2(q, pt2{p.x, p.z}) > 1e-3 || p.y < 0 || p.y > e.height {
			continue
		}
		if den > 0 {
			wn = wn.mul(-1)
		}
		bt, best = t, wallHit{true, p, wn, i}
	}
	return best
}

func (e *editor) updateWallContacts(dt float64) (touching bool) {
	if len(e.pts) < 3 || e.drawing {
		return false
	}
	for _, h := range e.hands {
		if !h.valid {
			h.touching = false
			continue
		}
		tp := h.tip()
		vel := 0.0
		if dt > 0 && h.prevTip != (vec3{}) {
			vel = tp.sub(h.prevTip).length() / dt
		}
		h.prevTip = tp
		if h.grab >= 0 || h.hoverV >= 0 || h.areaGrab != 0 || h.lineGrab {
			h.touching = false
			continue
		}
		hit := false
		var hp, hn vec3
		hw := -1
		for i := range e.pts {
			a, b := e.pts[i], e.pts[(i+1)%len(e.pts)]
			d, u := segDist(pt2{tp.x, tp.z}, a, b)
			if d < 0.035 && tp.y > 0 && tp.y < e.height {
				hit = true
				hw = i
				q := pt2{a[0] + (b[0]-a[0])*u, a[1] + (b[1]-a[1])*u}
				hp = v3(q[0], tp.y, q[1])
				hn = v3(tp.x-q[0], 0, tp.z-q[1]).norm()
			}
		}
		if hit {
			touching = true
			if !h.touching {
				strength := math.Min(1, 0.25+vel/2.5)
				spawnSparks(hp, hn, int(20+60*strength), strength)
				audioImpact(strength)
				haptic(h.idx, int(2000+6000*strength))
			} else if rand.Float64() < 0.5 {
				spawnSparks(hp, hn, 2, 0.2)
			}
			flare(hp, hn, 0.06, cSpark)
			e.touchWall(hw, hp, math.Min(1, 0.4+vel/2))
		}
		h.touching = hit
	}
	return touching
}

func (e *editor) humFor() (float64, float64) {
	if len(e.pts) < 3 {
		return 0, 0
	}
	hp := e.hmd.pos()
	best, bq := math.Inf(1), pt2{}
	for i := range e.pts {
		a, b := e.pts[i], e.pts[(i+1)%len(e.pts)]
		d, u := segDist(pt2{hp.x, hp.z}, a, b)
		if d < best {
			best, bq = d, pt2{a[0] + (b[0]-a[0])*u, a[1] + (b[1]-a[1])*u}
		}
	}
	close := math.Max(0, 1-best/1.8)
	vol := 0.05 + 0.20*math.Pow(close, 1.6)
	dir := v3(bq[0]-hp.x, 0, bq[1]-hp.z).norm()
	pan := dir.dot(e.hmd.axis(0))
	return vol, pan
}
