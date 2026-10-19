package main

import "math"

func polyArea(P []pt2) float64 {
	a := 0.0
	for i := range P {
		p, q := P[i], P[(i+1)%len(P)]
		a += p[0]*q[1] - q[0]*p[1]
	}
	return a / 2
}

func polyPerim(P []pt2) float64 {
	l := 0.0
	for i := range P {
		l += dist2(P[i], P[(i+1)%len(P)])
	}
	return l
}

func dist2(a, b pt2) float64 { return math.Hypot(b[0]-a[0], b[1]-a[1]) }

func pointInPoly(P []pt2, x, z float64) bool {
	c := false
	for i, j := 0, len(P)-1; i < len(P); j, i = i, i+1 {
		a, b := P[i], P[j]
		if (a[1] > z) != (b[1] > z) && x < (b[0]-a[0])*(z-a[1])/(b[1]-a[1])+a[0] {
			c = !c
		}
	}
	return c
}

func segDist(p, a, b pt2) (float64, float64) {
	dx, dz := b[0]-a[0], b[1]-a[1]
	L := dx*dx + dz*dz
	t := 0.0
	if L > 0 {
		t = ((p[0]-a[0])*dx + (p[1]-a[1])*dz) / L
	}
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(p[0]-a[0]-t*dx, p[1]-a[1]-t*dz), t
}

func domAngle(P []pt2) float64 {
	s, c := 0.0, 0.0
	for i := range P {
		a, b := P[i], P[(i+1)%len(P)]
		dx, dz := b[0]-a[0], b[1]-a[1]
		L2 := dx*dx + dz*dz
		t := math.Atan2(dz, dx) * 4
		s += L2 * math.Sin(t)
		c += L2 * math.Cos(t)
	}
	return math.Atan2(s, c) / 4
}

type frame struct{ c, s float64 }

func newFrame(theta float64) frame { return frame{math.Cos(theta), math.Sin(theta)} }
func (f frame) to(p pt2) pt2       { return pt2{p[0]*f.c + p[1]*f.s, -p[0]*f.s + p[1]*f.c} }
func (f frame) from(q pt2) pt2     { return pt2{q[0]*f.c - q[1]*f.s, q[0]*f.s + q[1]*f.c} }

func simplify(P []pt2, tol float64) []pt2 {
	Q := append([]pt2(nil), P...)
	for len(Q) > 4 {
		bi, bd := -1, tol
		for i := range Q {
			d, _ := segDist(Q[i], Q[(i-1+len(Q))%len(Q)], Q[(i+1)%len(Q)])
			if d < bd {
				bd, bi = d, i
			}
		}
		if bi < 0 {
			break
		}
		Q = append(Q[:bi], Q[bi+1:]...)
	}
	return Q
}

type wallGroup struct {
	k, o  int
	L, cw float64
}

func squareUp(P []pt2, theta, minLen float64) []pt2 {
	f := newFrame(theta)
	n := len(P)
	var g []wallGroup
	for i := 0; i < n; i++ {
		a, b := f.to(P[i]), f.to(P[(i+1)%n])
		du, dv := b[0]-a[0], b[1]-a[1]
		L := math.Hypot(du, dv)
		if L < 1e-6 {
			continue
		}
		k := ((int(math.Round(math.Atan2(dv, du)/(math.Pi/2))) % 4) + 4) % 4
		o := k % 2
		c := (a[1] + b[1]) / 2
		if o == 1 {
			c = (a[0] + b[0]) / 2
		}
		g = append(g, wallGroup{k, o, L, c * L})
	}
	for guard := 0; guard < 10000; guard++ {
		m := len(g)
		if m < 4 {
			return nil
		}
		merged := false
		for i := 0; i < m; i++ {
			j := (i + 1) % m
			if g[i].k == g[j].k {
				g[i] = wallGroup{g[i].k, g[i].o, g[i].L + g[j].L, g[i].cw + g[j].cw}
				g = append(g[:j], g[j+1:]...)
				merged = true
				break
			}
		}
		if merged {
			continue
		}
		wi, wl := -1, math.Inf(1)
		for i := 0; i < m; i++ {
			j := (i + 1) % m
			if g[i].o == g[j].o {
				s := i
				if g[j].L < g[i].L {
					s = j
				}
				if g[s].L < wl {
					wl, wi = g[s].L, s
				}
			}
		}
		if wi < 0 {
			for i := 0; i < m; i++ {
				if g[i].L < minLen && g[i].L < wl {
					wl, wi = g[i].L, i
				}
			}
		}
		if wi < 0 {
			break
		}
		g = append(g[:wi], g[wi+1:]...)
	}
	if len(g) < 4 {
		return nil
	}
	out := make([]pt2, 0, len(g))
	for i := range g {
		a, b := g[i], g[(i+1)%len(g)]
		ca, cb := a.cw/a.L, b.cw/b.L
		if a.o == 0 {
			out = append(out, f.from(pt2{cb, ca}))
		} else {
			out = append(out, f.from(pt2{ca, cb}))
		}
	}
	return out
}

func snapToNeighbors(p pt2, prev, next *pt2, theta float64) pt2 {
	f := newFrame(theta)
	q := f.to(p)
	t := math.Tan(7 * math.Pi / 180)
	for _, nb := range []*pt2{prev, next} {
		if nb == nil {
			continue
		}
		r := f.to(*nb)
		du, dv := q[0]-r[0], q[1]-r[1]
		if math.Abs(du) < math.Abs(dv)*t {
			q[0] = r[0]
		} else if math.Abs(dv) < math.Abs(du)*t {
			q[1] = r[1]
		}
	}
	return f.from(q)
}

type rigid2 struct{ a, b, c, d, tx, tz float64 }

func identity2() rigid2 { return rigid2{a: 1, d: 1} }

func (r rigid2) apply(p pt2) pt2 {
	return pt2{r.a*p[0] + r.b*p[1] + r.tx, r.c*p[0] + r.d*p[1] + r.tz}
}

func (r rigid2) inv() rigid2 {
	return rigid2{a: r.a, b: r.c, c: r.b, d: r.d,
		tx: -(r.a*r.tx + r.c*r.tz), tz: -(r.b*r.tx + r.d*r.tz)}
}

func (r rigid2) then(s rigid2) rigid2 {
	return rigid2{
		a: s.a*r.a + s.b*r.c, b: s.a*r.b + s.b*r.d,
		c: s.c*r.a + s.d*r.c, d: s.c*r.b + s.d*r.d,
		tx: s.a*r.tx + s.b*r.tz + s.tx, tz: s.c*r.tx + s.d*r.tz + s.tz,
	}
}

func (r rigid2) all(P []pt2) []pt2 {
	out := make([]pt2, len(P))
	for i, p := range P {
		out[i] = r.apply(p)
	}
	return out
}

func rigidFromYaw(yaw, x, z float64) rigid2 {
	c, s := math.Cos(yaw), math.Sin(yaw)
	return rigid2{a: c, b: -s, c: s, d: c, tx: x, tz: z}
}
