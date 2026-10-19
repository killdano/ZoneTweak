//go:build windows

package main

import (
	"fmt"
	"math"
	"time"
)

const assumedWarnDist = 0.5

var (
	cWarn   = rgba{1.00, 0.55, 0.15, 1}
	cDanger = rgba{1.00, 0.22, 0.20, 1}
	cArea   = rgba{1.00, 0.30, 0.85, 1}
	cLaser  = rgba{0.75, 0.95, 1.00, 1}
)

func (e *editor) warnDist() (float64, bool) {
	if v, ok := e.prof.warnDistance(); ok {
		return v, true
	}
	return assumedWarnDist, false
}

func (e *editor) proximityLevel() float64 {
	if !e.proxTest || len(e.pts) < 3 {
		return 0
	}
	d, _ := e.warnDist()
	lvl := 0.0
	probe := func(p vec3) {
		dist := distToBoundary(e.pts, pt2{p.x, p.z})
		if dist < d {
			lvl = math.Max(lvl, 1-dist/d)
		}
	}
	probe(e.hmd.pos())
	for _, h := range e.hands {
		if h.valid {
			probe(h.m.pos())
		}
	}
	return lvl
}

func (h *hand) rawLaser() (pt2, bool) {
	if !h.aimValid {
		return pt2{}, false
	}
	o := h.aim.pos()
	dir := h.aim.axis(2).mul(-1)
	if dir.y > -0.03 {
		return pt2{}, false
	}
	t := -o.y / dir.y
	if t > 15 || t < 0 {
		return pt2{}, false
	}
	return pt2{o.x + dir.x*t, o.z + dir.z*t}, true
}

func (h *hand) updateLaser(dt float64) {
	if h.lf == nil {
		h.lf = newPointFilter()
	}
	raw, ok := h.rawLaser()
	if !ok {
		h.lf.reset()
		h.lok = false
		return
	}
	h.lc, h.lok = h.lf.step(raw, dt), true
}

func (h *hand) pointer(laser bool) (pt2, bool) {
	if !laser {
		return h.cursor(), true
	}
	return h.lc, h.lok
}

func (e *editor) rayPanel(h *hand) (vec3, bool) {
	if !e.menuOpen || !h.aimValid {
		return vec3{}, false
	}
	o := h.aim.pos()
	dir := h.aim.axis(2).mul(-1)
	den := dir.dot(e.menuN)
	if math.Abs(den) < 1e-4 {
		return vec3{}, false
	}
	t := e.menuPos.sub(o).dot(e.menuN) / den
	if t <= 0 || t > 6 {
		return vec3{}, false
	}
	p := o.add(dir.mul(t))
	_, H := e.layout()
	d := p.sub(e.menuPos)
	if math.Abs(d.dot(e.menuR)) > panelW/2+0.02 || math.Abs(d.dot(e.menuU)) > H/2+0.02 {
		return vec3{}, false
	}
	return p, true
}

func (e *editor) handleWarnLine(h *hand, c pt2) bool {
	if !e.proxTest {
		return false
	}
	if h.lineGrab {
		if !h.held(bitTrigger) {
			h.lineGrab = false
			d := math.Max(warnDistMin, math.Min(warnDistMax, distToBoundary(e.pts, c)))
			nv, err := e.prof.setWarnDistance(d, false)
			if err != nil {
				e.say("Couldn't write Pimax settings: %v", err)
			} else {
				e.say("Warning distance %.2f M saved", nv)
			}
			haptic(h.idx, 1500)
			return true
		}
		e.lineDrag = math.Max(warnDistMin, math.Min(warnDistMax, distToBoundary(e.pts, c)))
		return true
	}
	if h.hoverV >= 0 || h.hoverE >= 0 || !h.down(bitTrigger) {
		return false
	}
	d, _ := e.warnDist()
	if math.Abs(distToBoundary(e.pts, c)-d) < 0.12 {
		h.lineGrab = true
		e.lineDrag = d
		haptic(h.idx, 1200)
		return true
	}
	return false
}

func (e *editor) currentWarnLine() float64 {
	for _, h := range e.hands {
		if h.lineGrab {
			return e.lineDrag
		}
	}
	d, _ := e.warnDist()
	return d
}

func (e *editor) drawProximity(level float64) {
	if !e.proxTest || len(e.pts) < 3 {
		return
	}
	d := e.currentWarnLine()
	in := insetPoly(e.pts, d)
	col := cWarn
	for _, h := range e.hands {
		if h.lineGrab {
			col = cAmber
		}
	}
	for i := range in {
		a, b := in[i], in[(i+1)%len(in)]
		for k := 0.0; k < 1; k += 0.1 {
			p := pt2{a[0] + (b[0]-a[0])*k, a[1] + (b[1]-a[1])*k}
			q := pt2{a[0] + (b[0]-a[0])*(k+0.06), a[1] + (b[1]-a[1])*(k+0.06)}
			bLines.line(fl(p, 0.006), fl(q, 0.006), col)
		}
		bTrans.quad(fl(a, 0), fl(b, 0), fl(b, 0.04), fl(a, 0.04), col.alpha(0.25))
	}
	label := fmt.Sprintf("WARNING DISTANCE %s", e.fmtLen(d))
	if _, ok := e.prof.warnDistance(); !ok {
		label += " (PIMAX DEFAULT, ASSUMED)"
	}
	if len(in) > 0 {
		mid := fl(pt2{(in[0][0] + in[1%len(in)][0]) / 2, (in[0][1] + in[1%len(in)][1]) / 2}, 0.35)
		r, u := billboard(e.hmd.pos(), mid)
		drawTextCentered(&bOpaque, label, mid, r, u, smallPx*1.4, col)
	}
	if level > 0 {
		pulse := 0.75 + 0.25*math.Sin(float64(time.Now().UnixMilli())/90)
		cam := e.hmd.pos()
		fwd := e.hmd.axis(2).mul(-1)
		fwd.y = 0
		fwd = fwd.norm()
		p := cam.add(fwd.mul(0.7)).add(v3(0, 0.12, 0))
		r, u := billboard(cam, p)
		drawTextCentered(&bOpaque, "SAFETY ZONE WOULD SHOW HERE", p, r, u, textPx*1.2, cDanger.alpha(float32(pulse)))
	}
}

func wallColors(level float64) (rgba, rgba) {
	if level <= 0 {
		return cGreen.alpha(0.10), cGreen
	}
	pulse := 0.8 + 0.2*math.Sin(float64(time.Now().UnixMilli())/90)
	l := float32(level * pulse)
	mix := rgba{cGreen.r + (cDanger.r-cGreen.r)*l, cGreen.g + (cDanger.g-cGreen.g)*l, cGreen.b + (cDanger.b-cGreen.b)*l, 1}
	return mix.alpha(0.12 + 0.5*l), mix
}

func (e *editor) centerPose() rigid2 {
	if e.center == (rigid2{}) {
		e.center = e.st.adjust()
	}
	return e.center
}

func (e *editor) areaCorners(g rigid2, w, h float64) [4]pt2 {
	return [4]pt2{g.apply(pt2{-w / 2, -h / 2}), g.apply(pt2{w / 2, -h / 2}), g.apply(pt2{w / 2, h / 2}), g.apply(pt2{-w / 2, h / 2})}
}
