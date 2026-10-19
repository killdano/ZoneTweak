package main

import "math"

type oneEuro struct {
	minCutoff, beta, dCutoff float64
	x, dx                    float64
	init                     bool
}

func alpha(cutoff, dt float64) float64 {
	tau := 1 / (2 * math.Pi * cutoff)
	return 1 / (1 + tau/dt)
}

func (f *oneEuro) step(v, dt float64) float64 {
	if !f.init || dt <= 0 {
		f.x, f.dx, f.init = v, 0, true
		return v
	}
	d := (v - f.x) / dt
	ad := alpha(f.dCutoff, dt)
	f.dx += ad * (d - f.dx)
	a := alpha(f.minCutoff+f.beta*math.Abs(f.dx), dt)
	f.x += a * (v - f.x)
	return f.x
}

type pointFilter struct{ fx, fz oneEuro }

func newPointFilter() *pointFilter {
	mk := func() oneEuro { return oneEuro{minCutoff: 0.7, beta: 1.5, dCutoff: 1.0} }
	return &pointFilter{fx: mk(), fz: mk()}
}

func (p *pointFilter) step(c pt2, dt float64) pt2 {
	return pt2{p.fx.step(c[0], dt), p.fz.step(c[1], dt)}
}

func (p *pointFilter) reset() { p.fx.init, p.fz.init = false, false }
