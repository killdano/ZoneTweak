package main

import (
	"fmt"
	"math"
)

func (e *editor) areaHandles() map[int]pt2 {
	g := e.centerPose()
	w, h := e.area[0]/2, e.area[1]/2
	loc := map[int]pt2{
		1: {0, 0}, 2: {w, 0}, 3: {-w, 0}, 4: {0, h}, 5: {0, -h},
		6: {w, h}, 7: {w, -h}, 8: {-w, h}, 9: {-w, -h}, 10: {0, -arcR},
	}
	out := map[int]pt2{}
	for k, v := range loc {
		out[k] = g.apply(v)
	}
	return out
}

const (
	arcR    = 0.55
	arcSpan = 0.9
)

func (e *editor) onArc(c pt2) bool {
	l := e.centerPose().inv().apply(c)
	r := math.Hypot(l[0], l[1])
	if math.Abs(r-arcR) > 0.12 {
		return false
	}
	a := math.Atan2(l[0], -l[1])
	return math.Abs(a) < arcSpan/2+0.1 || math.Abs(math.Remainder(a-math.Pi, 2*math.Pi)) < arcSpan/2+0.1
}

func (e *editor) lockPos() pt2 { return e.centerPose().apply(pt2{0.35, 0.3}) }

func (e *editor) setAreaMode(on bool) {
	e.areaMode = on
	e.matching, e.drawing = false, false
	e.releaseAll()
	for _, h := range e.hands {
		h.areaGrab, h.areaHover = 0, 0
	}
	if on {
		e.say("Play area unlocked. Drag the handles on the floor")
		if e.menuOpen {
			e.page = e.pageArea
		}
		return
	}
	if e.page == e.pageArea {
		e.page = e.pageMain
	}
	if e.centerDirty || e.areaDirty {
		e.dirty = true
		e.say("Play area locked. Press save to keep it")
	} else {
		e.say("Play area locked")
	}
}

func (e *editor) drawLock() {
	g := e.centerPose()
	p := e.lockPos()
	col := cMuted
	if e.areaMode {
		col = cArea
	}
	for _, hd := range e.hands {
		if hd.valid {
			if c, ok := hd.pointer(e.laser); ok && dist2(c, p) < 0.15 {
				col = cAmber
			}
		}
	}
	R := v3(g.a, 0, g.c)
	F := v3(-g.b, 0, -g.d).mul(-1)
	base := fl(p, 0.004)
	bw, bh := 0.07, 0.055
	corner := func(x, z float64) vec3 { return base.add(R.mul(x)).add(F.mul(z)) }
	b0, b1, b2, b3 := corner(-bw, -bh), corner(bw, -bh), corner(bw, bh), corner(-bw, bh)
	bOpaque.quad(b0, b1, b2, b3, col)
	lift := 0.0
	if e.areaMode {
		lift = 0.05
	}
	const n = 12
	for i := 0; i < n; i++ {
		a0, a1 := math.Pi*float64(i)/n, math.Pi*float64(i+1)/n
		q0 := corner(-0.045*math.Cos(a0), bh+lift+0.06*math.Sin(a0))
		q1 := corner(-0.045*math.Cos(a1), bh+lift+0.06*math.Sin(a1))
		neonLine(q0, q1, col)
	}
	neonLine(corner(-0.045, bh), corner(-0.045, bh+lift), col)
	if !e.areaMode {
		neonLine(corner(0.045, bh), corner(0.045, bh+lift), col)
	}
	r, u := billboard(e.hmd.pos(), base)
	label := "UNLOCK PLAY AREA"
	if e.areaMode {
		label = "LOCK PLAY AREA"
	}
	drawTextCentered(&bOpaque, label, base.add(v3(0, 0.1, 0)), r, u, smallPx*1.1, col)
}

func (e *editor) drawArcs(hot bool) {
	g := e.centerPose()
	col := cYellow
	if hot {
		col = cAmber
	}
	for _, mid := range []float64{0, math.Pi} {
		pts := []pt2{}
		const n = 24
		for i := 0; i <= n; i++ {
			a := mid - arcSpan/2 + arcSpan*float64(i)/n
			pts = append(pts, g.apply(pt2{arcR * math.Sin(a), -arcR * math.Cos(a)}))
		}
		for _, off := range []float64{0.004, 0.012} {
			for i := 0; i < n; i++ {
				neonLine(fl(pts[i], off), fl(pts[i+1], off), col)
			}
		}
		for i := 0; i < n; i += 2 {
			bGlowTri.quad(fl(pts[i], 0.003), fl(pts[i+1], 0.003), fl(pts[i+1], 0.03), fl(pts[i], 0.03), col.alpha(0.35))
		}
		for _, end := range []float64{-1, 1} {
			a := mid + end*arcSpan/2
			tip := pt2{arcR * math.Sin(a+end*0.12), -arcR * math.Cos(a+end*0.12)}
			in := pt2{(arcR - 0.08) * math.Sin(a), -(arcR - 0.08) * math.Cos(a)}
			out := pt2{(arcR + 0.08) * math.Sin(a), -(arcR + 0.08) * math.Cos(a)}
			bOpaque.tri(fl(g.apply(in), 0.006), fl(g.apply(out), 0.006), fl(g.apply(tip), 0.006), col)
		}
	}
}

func (e *editor) pickHandle(c pt2) int {
	if e.onArc(c) {
		for _, k := range []int{6, 7, 8, 9, 2, 3, 4, 5} {
			if dist2(e.areaHandles()[k], c) < 0.18 {
				return k
			}
		}
		return 10
	}
	hs := e.areaHandles()
	best, bd := 0, 0.0
	for _, k := range []int{6, 7, 8, 9, 2, 3, 4, 5, 1} {
		r := 0.18
		if k == 1 {
			r = 0.25
		}
		if d := dist2(hs[k], c); d < r && (best == 0 || d < bd) {
			best, bd = k, d
		}
	}
	return best
}

func (e *editor) turnArea(rad float64) {
	g := e.centerPose()
	e.center = rigidFromYaw(math.Atan2(g.c, g.a)+rad, g.tx, g.tz)
	e.centerDirty = true
}

func (e *editor) drawArea() {
	g := e.centerPose()
	o := g.apply(pt2{0, 0})
	fwd := g.apply(pt2{0, -0.45})
	col := cYellow
	neonLine(fl(g.apply(pt2{-0.25, 0}), 0.003), fl(g.apply(pt2{0.25, 0}), 0.003), col)
	neonLine(fl(g.apply(pt2{0, 0.25}), 0.003), fl(fwd, 0.003), col)
	neonLine(fl(fwd, 0.003), fl(g.apply(pt2{-0.08, -0.35}), 0.003), col)
	neonLine(fl(fwd, 0.003), fl(g.apply(pt2{0.08, -0.35}), 0.003), col)
	r, u := billboard(e.hmd.pos(), fl(o, 0))
	name := "PLAY CENTER"
	if e.centerDirty || e.areaDirty {
		name += " (UNSAVED)"
	}
	drawTextCentered(&bOpaque, name, fl(o, 0.14), r, u, smallPx*1.5, col)

	w, h := e.area[0], e.area[1]
	cs := e.areaCorners(g, w, h)
	ac := cArea
	if !e.areaMode {
		ac = cArea.alpha(0.45)
	}
	for i := 0; i < 4; i++ {
		neonLine(fl(cs[i], 0.005), fl(cs[(i+1)%4], 0.005), ac)
	}
	e.drawLock()
	if !e.areaMode {
		return
	}
	for i := 0; i < 4; i++ {
		bGlowTri.quad(fl(cs[i], 0), fl(cs[(i+1)%4], 0), fl(cs[(i+1)%4], 0.03), fl(cs[i], 0.03), cArea.alpha(0.25))
	}
	hot := map[int]bool{}
	for _, hd := range e.hands {
		if hd.valid {
			if hd.areaGrab != 0 {
				hot[hd.areaGrab] = true
			} else if hd.areaHover != 0 {
				hot[hd.areaHover] = true
			}
		}
	}
	hs := e.areaHandles()
	for k, p := range hs {
		if k == 10 {
			continue
		}
		size, hc := 0.035, cArea
		switch {
		case k == 1:
			size, hc = 0.06, cYellow
		case k == 10:
			size, hc = 0.045, cYellow
		case k >= 6:
			size = 0.05
		}
		if hot[k] {
			hc, size = cAmber, size*1.3
		}
		post(&bOpaque, fl(p, 0), size, 0.06, hc)
	}
	e.drawArcs(hot[10])
	for _, hd := range e.hands {
		k := hd.areaHover
		if hd.areaGrab != 0 {
			k = hd.areaGrab
		}
		if !hd.valid || k == 0 {
			continue
		}
		txt := map[int]string{1: "MOVE PLAY CENTER", 10: "DRAG ALONG THE ARROW TO TURN"}[k]
		if txt == "" {
			txt = "STRETCH PLAY AREA"
		}
		p := fl(hs[k], 0.22)
		rr, uu := billboard(e.hmd.pos(), p)
		drawTextCentered(&bOpaque, txt, p, rr, uu, smallPx*1.3, cAmber)
	}
	lp := fl(g.apply(pt2{0, h/2 + 0.2}), 0.25)
	r2, u2 := billboard(e.hmd.pos(), lp)
	drawTextCentered(&bOpaque, fmt.Sprintf("PLAY AREA %s X %s", e.fmtLen(w), e.fmtLen(h)), lp, r2, u2, smallPx*1.4, cArea)
	drawTextCentered(&bOpaque, "FORWARD", fl(g.apply(pt2{0, -0.6}), 0.05), r2, u2, smallPx*1.1, cYellow.alpha(0.7))
}

func (e *editor) handleArea(h *hand, c pt2) {
	if h.areaGrab != 0 {
		if !h.held(bitTrigger) {
			h.areaGrab = 0
			haptic(h.idx, 800)
			return
		}
		g0 := e.center0
		l := g0.inv().apply(c)
		w0, h0 := e.area0[0], e.area0[1]
		moveAxis := func(val, half, sign float64) (float64, float64) {
			fixed := -sign * half
			moved := fixed + sign*math.Max(0.5, math.Min(20, sign*(val-fixed)))
			return math.Abs(moved - fixed), (moved + fixed) / 2
		}
		k := h.areaGrab
		switch k {
		case 1:
			e.center = rigid2{a: g0.a, b: g0.b, c: g0.c, d: g0.d, tx: c[0] - h.grabOff[0], tz: c[1] - h.grabOff[1]}
			e.centerDirty = true
			return
		case 10:
			a0 := math.Atan2(h.grabOff[1], h.grabOff[0])
			a1 := math.Atan2(c[1]-g0.tz, c[0]-g0.tx)
			e.center = rigidFromYaw(math.Atan2(g0.c, g0.a)+a1-a0, g0.tx, g0.tz)
			e.centerDirty = true
			return
		}
		cx, cz, w, hh := 0.0, 0.0, w0, h0
		switch k {
		case 2, 6, 7:
			w, cx = moveAxis(l[0], w0/2, 1)
		case 3, 8, 9:
			w, cx = moveAxis(l[0], w0/2, -1)
		}
		switch k {
		case 4, 6, 8:
			hh, cz = moveAxis(l[1], h0/2, 1)
		case 5, 7, 9:
			hh, cz = moveAxis(l[1], h0/2, -1)
		}
		np := g0.apply(pt2{cx, cz})
		e.center = rigid2{a: g0.a, b: g0.b, c: g0.c, d: g0.d, tx: np[0], tz: np[1]}
		e.area = [2]float64{w, hh}
		e.centerDirty, e.areaDirty = true, true
		return
	}
	h.areaHover = e.pickHandle(c)
	if !h.down(bitTrigger) || h.areaHover == 0 {
		return
	}
	h.areaGrab = h.areaHover
	e.center0 = e.centerPose()
	e.area0 = e.area
	h.grabOff = pt2{c[0] - e.center0.tx, c[1] - e.center0.tz}
	haptic(h.idx, 1200)
}
