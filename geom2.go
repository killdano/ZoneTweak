package main

import "math"

func distToBoundary(P []pt2, c pt2) float64 {
	if len(P) < 3 || !pointInPoly(P, c[0], c[1]) {
		return 0
	}
	best := math.Inf(1)
	for i := range P {
		d, _ := segDist(c, P[i], P[(i+1)%len(P)])
		best = math.Min(best, d)
	}
	return best
}

func insetPoly(P []pt2, d float64) []pt2 {
	n := len(P)
	if n < 3 {
		return nil
	}
	sg := 1.0
	if polyArea(P) < 0 {
		sg = -1
	}
	type line struct{ p, dir, nrm pt2 }
	ls := make([]line, n)
	for i := 0; i < n; i++ {
		a, b := P[i], P[(i+1)%n]
		dx, dz := b[0]-a[0], b[1]-a[1]
		L := math.Hypot(dx, dz)
		if L < 1e-9 {
			L = 1e-9
		}
		nx, nz := -dz/L*sg, dx/L*sg
		ls[i] = line{pt2{a[0] + nx*d, a[1] + nz*d}, pt2{dx / L, dz / L}, pt2{nx, nz}}
	}
	out := make([]pt2, n)
	for i := 0; i < n; i++ {
		l1, l2 := ls[(i-1+n)%n], ls[i]
		den := l1.dir[0]*l2.dir[1] - l1.dir[1]*l2.dir[0]
		fallback := pt2{P[i][0] + (l1.nrm[0]+l2.nrm[0])/2*d, P[i][1] + (l1.nrm[1]+l2.nrm[1])/2*d}
		if math.Abs(den) < 1e-6 {
			out[i] = fallback
			continue
		}
		wx, wz := l2.p[0]-l1.p[0], l2.p[1]-l1.p[1]
		t := (wx*l2.dir[1] - wz*l2.dir[0]) / den
		q := pt2{l1.p[0] + l1.dir[0]*t, l1.p[1] + l1.dir[1]*t}
		if dist2(q, P[i]) > 3*d {
			q = fallback
		}
		out[i] = q
	}
	return out
}

func segsCross(a, b, c, d pt2) bool {
	o := func(p, q, r pt2) float64 { return (q[0]-p[0])*(r[1]-p[1]) - (q[1]-p[1])*(r[0]-p[0]) }
	d1, d2, d3, d4 := o(c, d, a), o(c, d, b), o(a, b, c), o(a, b, d)
	return ((d1 > 0) != (d2 > 0)) && ((d3 > 0) != (d4 > 0))
}

func rectFits(P []pt2, hw, hh float64) bool {
	cs := [4]pt2{{-hw, -hh}, {hw, -hh}, {hw, hh}, {-hw, hh}}
	for _, p := range cs {
		if !pointInPoly(P, p[0], p[1]) {
			return false
		}
	}
	for i := range P {
		a, b := P[i], P[(i+1)%len(P)]
		for k := 0; k < 4; k++ {
			if segsCross(a, b, cs[k], cs[(k+1)%4]) {
				return false
			}
		}
		if math.Abs(a[0]) < hw && math.Abs(a[1]) < hh {
			return false
		}
	}
	return true
}

func fitArea(P []pt2, margin float64) (float64, float64, bool) {
	if len(P) < 3 || !pointInPoly(P, 0, 0) {
		return 0, 0, false
	}
	bestA, bw, bh, bestE := 0.0, 0.0, 0.0, 0.0
	try := func(ex float64) {
		asp := math.Pow(10, ex)
		lo, hi := 0.0, 15.0
		for k := 0; k < 40; k++ {
			mid := (lo + hi) / 2
			if rectFits(P, mid, mid*asp) {
				lo = mid
			} else {
				hi = mid
			}
		}
		if a := lo * lo * asp; a > bestA {
			bestA, bw, bh, bestE = a, 2*lo, 2*lo*asp, ex
		}
	}
	for i := 0; i <= 60; i++ {
		try(-0.7 + 1.4*float64(i)/60)
	}
	step := 1.4 / 60
	for r := 0; r < 3; r++ {
		c := bestE
		for i := -10; i <= 10; i++ {
			try(c + step*float64(i)/10)
		}
		step /= 10
	}
	if bestA <= 0 {
		return 0, 0, false
	}
	return math.Max(0.5, bw-2*margin), math.Max(0.5, bh-2*margin), true
}

func outsideDist(P []pt2, c pt2) float64 {
	best := math.Inf(1)
	for i := range P {
		d, _ := segDist(c, P[i], P[(i+1)%len(P)])
		best = math.Min(best, d)
	}
	return best
}
