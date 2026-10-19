//go:build windows

package main

import (
	"math"
	"math/rand"
	"sort"
)

// laser walls with irregular overcharge surges, and the generator posts

type surge struct {
	t0, charge, travel float64
	u0, dir, y0        float64
	trail              []vec3
	seed               float64
	stalls             [3][2]float64
	done, launched     bool
	lastU              float64
}

var (
	wallSurges = map[int][]*surge{}
	surgeNext  = map[int]float64{}
	surgeKeyN  int
)

// The sheet is only projected by the energy blades: from the blade's bottom collar up to
// its top (not the bell cap, not the base machinery). Below the blades, diagonal blade
// extensions telescope out of each generator and down to the floor, so the sheet still
// reaches the deck in front of the bases without being projected through them.
const (
	bladeBot   = 0.44 // where the energy blade starts on the shaft
	bladeTopIn = 0.26 // blade ends this far below the shaft top (the bell)
	extReach   = 0.42 // how far out from the post the extension meets the floor
)

type wallProfile struct {
	L, top, eA, eB float64
	liftA, liftB   float64 // a post hovering (torn free / not yet planted) lifts its blade
	floorY         float64 // where the floor blades stop (higher over a shared base)
}

const (
	floorGap  = 0.045 // the floor blades stop just short of the deck
	floorOver = 0.15  // ...or this high when they'd otherwise dig into a shared base
)

func (w wallProfile) topAt(x float64) float64 {
	u := x / math.Max(1e-6, w.L)
	return w.top + w.liftA + (w.liftB-w.liftA)*u
}

// lo: bottom edge of the sheet at distance x along the wall. It runs from each post's blade
// collar down the floor blade to its shoe, and along the floor between the two shoes.
func (w wallProfile) lo(x float64) float64 {
	reach := math.Min(extReach, w.L*0.48)
	ka, kb := smooth(w.eA), smooth(w.eB)
	ba, bb := bladeBot+w.liftA, bladeBot+w.liftB // bottom of each post's energy blade
	xa := 0.075 + (reach-0.075)*ka
	fy := w.floorY
	if fy == 0 {
		fy = floorGap
	}
	ya := ba + (fy-ba)*ka
	xb := w.L - (0.075 + (reach-0.075)*kb)
	yb := bb + (fy-bb)*kb
	const x0 = 0.075 // the telescope root, at the lower emitter head
	switch {
	case x <= x0:
		return ba
	case x <= xa:
		return ba + (ya-ba)*(x-x0)/math.Max(1e-6, xa-x0)
	case x >= w.L-x0:
		return bb
	case x >= xb:
		return bb + (yb-bb)*(w.L-x0-x)/math.Max(1e-6, w.L-x0-xb)
	}
	return ya + (yb-ya)*(x-xa)/math.Max(1e-6, xb-xa)
}

// breaks: the corners of the bottom edge (telescope root, telescope tip/lens), as fractions
// of the wall length - the sheet mesh and edge line put vertices exactly there so the
// corner stays sharp instead of being cut across above the lens
func (w wallProfile) breaks() []float64 {
	reach := math.Min(extReach, w.L*0.48)
	ka, kb := smooth(w.eA), smooth(w.eB)
	xa := 0.075 + (reach-0.075)*ka
	xb := w.L - (0.075 + (reach-0.075)*kb)
	L := math.Max(1e-6, w.L)
	return []float64{0.075 / L, xa / L, xb / L, 1 - 0.075/L}
}

// edgeSamples: n even steps plus the profile's corners, sorted
func edgeSamples(n int, br []float64) []float64 {
	us := make([]float64, 0, n+1+len(br))
	for k := 0; k <= n; k++ {
		us = append(us, float64(k)/float64(n))
	}
	for _, b := range br {
		if b > 0 && b < 1 {
			us = append(us, b)
		}
	}
	sort.Float64s(us)
	out := us[:1]
	for _, u := range us[1:] {
		if u-out[len(out)-1] > 1e-5 {
			out = append(out, u)
		}
	}
	return out
}

var bladeExt = map[int]float64{}
var bladeExtN int
var bladeExtPos []pt2

// updateBladeExt animates every generator's floor blades toward deployed (planted and powered)
// or retracted (floating, still deploying, or regrowing a base)
func (e *editor) updateBladeExt(dt float64) {
	n := len(e.pts)
	t := fxTime()
	if n != bladeExtN {
		// points were added or removed: carry values over by position
		old, oldPos := bladeExt, bladeExtPos
		bladeExt = map[int]float64{}
		for i, p := range e.pts {
			v := 0.0
			if t < 3 {
				v = 1
			}
			for j, q := range oldPos {
				if q == p {
					v = old[j]
				}
			}
			bladeExt[i] = v
		}
		bladeExtN = n
	}
	bladeExtPos = append(bladeExtPos[:0], e.pts...)
	for i, p := range e.pts {
		target := 1.0
		_, _, floating := e.genMode(i)
		switch {
		case floating:
			target = 0
		case deployAt(p) != nil && t-deployAt(p).t0 < 1.15:
			target = 0
		case regrowAt(p) != nil && t-regrowAt(p).t0 < 1.25:
			target = 0
		}
		v, ok := bladeExt[i]
		if !ok {
			v = 0
		}
		prev := v
		if v < target {
			v = math.Min(target, v+dt*2.2)
		} else {
			v = math.Max(target, v-dt*3)
		}
		if prev < 0.02 && v >= 0.02 {
			e.sfx(sndTelescopeID, fl(p, 0.3), 0.35)
		} else if prev > 0.98 && v <= 0.98 {
			e.sfx(sndHydraulic, fl(p, 0.3), 0.3)
		}
		bladeExt[i] = v
	}
}

// drawBladeExt: the diagonal floor blades - box-section telescoping emitters running from the
// bottom collar of the energy blade down to just above the deck, angled out over the base.
// The sheet comes out of a glowing trench along their upper face (the face toward the sheet).
// extKey: a generator's floor telescopes at rest (stowed or fully out), for the static cache
type extKey struct {
	p, prev, next pt2
	out           bool
	fyA, fyB      float64
}

// cachedChunk: geometry that only changes when its key does is built once into the static
// generator buffer (uploaded to the GPU only when the set of chunks changes)
var (
	chunkCache      = map[any][]float32{}
	chunkFrameKeys  []any
	chunkPrevKeys   []any
	chunkCacheUsed  = map[any]int{}
	chunkFrameCount int
)

func cachedChunk(key any, build func()) {
	if _, ok := chunkCache[key]; !ok {
		start := len(bEnv.v)
		build()
		chunkCache[key] = append([]float32(nil), bEnv.v[start:]...)
		bEnv.v = bEnv.v[:start]
	}
	chunkCacheUsed[key] = chunkFrameCount
	chunkFrameKeys = append(chunkFrameKeys, key)
}

var (
	extCache     = map[extKey][]float32{}
	extFrameKeys []extKey
	extPrevKeys  []extKey
	bxGeom       = true // drawBladeExt: emit the hardware
	bxFX         = true // ...and/or the energy effects
)

// drawBladeExt: the floor telescopes and lower lenses. At rest their hardware is built once
// into the static generator buffer; only the moving ones and the energy effects are drawn
// every frame.
func (e *editor) drawBladeExt(i int, p, prev, next pt2, col rgba) {
	ext := e.extOf(i)
	if deployAt(p) != nil && ext < 0.005 {
		return
	}
	if (ext < 0.005 || ext > 0.999) && e.liftOf(i) == 0 {
		n := len(e.pts)
		key := extKey{p: p, prev: prev, next: next, out: ext > 0.999, fyA: floorGap, fyB: floorGap}
		if e.jointPair(i, (i-1+n)%n) {
			key.fyA = floorOver
		}
		if e.jointPair(i, (i+1)%n) {
			key.fyB = floorOver
		}
		if _, ok := extCache[key]; !ok {
			start := len(bEnv.v)
			bxFX = false
			e.bladeExtDraw(i, p, prev, next, col, ext)
			bxFX = true
			extCache[key] = append([]float32(nil), bEnv.v[start:]...)
			bEnv.v = bEnv.v[:start]
			if len(extCache) > 160 {
				for k := range extCache {
					if k != key {
						delete(extCache, k)
						break
					}
				}
			}
		}
		extFrameKeys = append(extFrameKeys, key)
		bxGeom = false
		e.bladeExtDraw(i, p, prev, next, col, ext)
		bxGeom = true
		return
	}
	e.bladeExtDraw(i, p, prev, next, col, ext)
}

func (e *editor) bladeExtDraw(i int, p, prev, next pt2, col rgba, ext float64) {
	if ext < 0.005 {
		e.bottomLenses(i, p, prev, next, 0, col)
		return
	}
	e.bottomLenses(i, p, prev, next, ext, col)
	k := smooth(ext)
	t := fxTime()
	up := v3(0, 1, 0)
	lift := e.liftOf(i)
	c := fl(p, lift)
	seedTex(70000 + float64(i)*31)
	for side, nb := range []pt2{prev, next} {
		L := dist2(p, nb)
		if L < 1e-4 {
			continue
		}
		dv := v3((nb[0]-p[0])/L, 0, (nb[1]-p[1])/L)
		wn := dv.cross(up).norm() // across the sheet
		reach := math.Min(extReach, L*0.48)
		o := c.add(dv.mul(0.075)).add(up.mul(bladeBot))
		fy := floorGap
		if e.jointPair(i, (i+[]int{-1, 1}[side]+len(e.pts))%len(e.pts)) {
			fy = floorOver
		}
		f := fl(p, fy).add(dv.mul(reach))
		tip := o.add(f.sub(o).mul(k))
		d := tip.sub(o)
		Ld := d.length()
		if Ld < 1e-4 {
			continue
		}
		dn := d.mul(1 / Ld)
		nUp := up.sub(dn.mul(up.dot(dn))).norm() // in the sheet's plane, toward the sheet
		// collar housing the arm folds out of
		type sec struct{ hw, hh float64 }
		secs := []sec{{0.032, 0.024}, {0.027, 0.02}, {0.022, 0.016}}
		if !bxGeom {
			secs = nil
		} else {
			useMat(mCast)
			bevelBox(&bEnv, o.sub(nUp.mul(0.03)).add(dv.mul(-0.01)), wn, up, dv, 0.04, 0.045, 0.03, 0.008, cIron)
		}
		// three nested box sections; the top face of each sits on the sheet's edge line
		for si, sc := range secs {
			u0 := float64(si) / 3 * 0.9
			u1 := math.Min(1, u0+0.42)
			a0, a1 := o.add(d.mul(u0)), o.add(d.mul(u1))
			mid := a0.add(a1).mul(0.5).sub(nUp.mul(sc.hh))
			hl := a1.sub(a0).length() / 2
			if si == 0 {
				useMat(mCast)
				bevelBox(&bEnv, mid, wn, nUp, dn, sc.hw, sc.hh, hl, 0.005, cIron)
			} else {
				useMat(mSteel)
				bevelBox(&bEnv, mid, wn, nUp, dn, sc.hw, sc.hh, hl, 0.004, cSteelEnv.scale(0.75))
			}
			// lip/clamp ring where the next section slides out
			useMat(mCast)
			bevelBox(&bEnv, a1.sub(nUp.mul(sc.hh)).sub(dn.mul(0.008)), wn, nUp, dn, sc.hw+0.004, sc.hh+0.004, 0.008, 0.002, cIron.scale(1.2))
			// side bolts
			useMat(mSteel)
			for _, sx := range []float64{-1, 1} {
				b := a0.add(d.mul(0.12 / math.Max(Ld, 0.12) * 0.5)).sub(nUp.mul(sc.hh)).add(wn.mul(sx * (sc.hw + 0.001)))
				cyl(&bEnv, b, b.add(wn.mul(sx*0.004)), 0.005, 0.005, 6, cSteelEnv, true)
			}
			// the trench: a dark slot down the upper face
			curEmis = 1
			useMat(material{0, 0.2, 0.9, 0})
			tw := sc.hw * 0.38
			q0, q1 := a0.add(nUp.mul(0.0008)), a1.add(nUp.mul(0.0008))
			bEnv.quadF(q0.sub(wn.mul(tw)), q1.sub(wn.mul(tw)), q1.add(wn.mul(tw)), q0.add(wn.mul(tw)), nUp, rgba{0.01, 0.012, 0.012, 1})
		}
		if bxGeom {
			// end cap with an emitter lens; it hangs just above the deck, it doesn't stand on it
			useMat(mCast)
			bevelBox(&bEnv, tip.sub(nUp.mul(0.016)).add(dn.mul(0.006)), wn, nUp, dn, 0.024, 0.018, 0.008, 0.004, cIron)
			// a short level mount on the tip carries the lens pushed out from the lower box
			nz := tip.add(dv.mul(0.012))
			bevelBox(&bEnv, nz, wn, up, dv, 0.03, 0.03, 0.014, 0.004, cIron.scale(1.15))
		}
		if bxFX && ext > 0.55 && e.sideWallPower(i, side) > 0.05 {
			g := float32(math.Min(1, (ext-0.55)/0.4) * e.sideWallPower(i, side))
			// energy welling out of the trench along its length
			ribbon(o, tip, 0.018, col.alpha(0.3*g))
			ribbon(o, tip, 0.006, cEnergyHot.alpha(0.75*g))
			// the same flickering hot points as the main blade, jetting up into the sheet
			nPts := int(Ld / 0.045)
			if potatoMode {
				nPts /= 3
			}
			for q := 0; q < nPts; q++ {
				hs := hash1(float64(i)*13.1 + float64(side)*7.7 + float64(q)*3.3)
				u := (float64(q) + 0.5 + 0.3*math.Sin(t*0.4*(0.4+hs)+hs*9)) / float64(nPts)
				pp := o.add(d.mul(u))
				fk := noise1(hs*31, t*0.8*(0.8+hs*0.96))
				br := float32(0.25+0.75*fk*fk) * g
				sz := 0.015 + 0.035*hs*hs
				sprite(pp, sz*(0.8+0.4*fk), cEnergyHot.alpha(0.5*br))
				jl := (0.02 + 0.1*fk*fk) * (0.6 + sz*12)
				// jets run dead level, straight out into the sheet (like the main blade's)
				ribbonC(pp, pp.add(dv.mul(jl)), sz*0.5, cEnergyHot.alpha(0.4*br), cEnergyHot.alpha(0))
			}
			sprite(tip, 0.03, col.alpha(0.6*g))
		}
	}
}

// bottomLensAt: where the lower emitter lens sits for one side - on the face of the lower box
// while the telescope is stowed, riding out on the end of the telescope as it extends
func (e *editor) bottomLensAt(i int, p, nb pt2, side int, ext float64) (vec3, vec3, bool) {
	L := dist2(p, nb)
	if L < 1e-4 {
		return vec3{}, vec3{}, false
	}
	up := v3(0, 1, 0)
	dv := v3((nb[0]-p[0])/L, 0, (nb[1]-p[1])/L)
	c := fl(p, e.liftOf(i))
	box := c.add(dv.mul(headFace)).add(up.mul(bottomHeadY))
	if ext <= 0 {
		return box, dv, true
	}
	k := smooth(ext)
	o := c.add(dv.mul(0.075)).add(up.mul(bladeBot))
	fy := floorGap
	if e.jointPair(i, (i+[]int{-1, 1}[side]+len(e.pts))%len(e.pts)) {
		fy = floorOver
	}
	f := fl(p, fy).add(dv.mul(math.Min(extReach, L*0.48)))
	tip := o.add(f.sub(o).mul(k))
	at := tip.add(dv.mul(0.026)) // centred on the sheet's bottom edge line
	// out of the box face first, then along with the telescope
	u := smooth(math.Min(1, ext/0.15))
	return box.add(at.sub(box).mul(u)), dv, true
}

func (e *editor) bottomLenses(i int, p, prev, next pt2, ext float64, col rgba) {
	up := v3(0, 1, 0)
	for side, nb := range []pt2{prev, next} {
		fc, dv, ok := e.bottomLensAt(i, p, nb, side, ext)
		if !ok {
			continue
		}
		if bxGeom {
			seedTex(71000 + float64(i)*17 + float64(side))
			emitterLens(fc, dv, dv.cross(up).norm(), up)
		}
		pw := float32(e.sideWallPower(i, side))
		if bxFX && pw > 0.05 {
			lp := fc.add(dv.mul(0.04))
			fl := float32(0.75 + 0.25*math.Sin(fxTime()*11+float64(i)*3+float64(side)))
			sprite(lp, 0.075, col.alpha(0.55*fl*pw))
			sprite(lp, 0.03, cEnergyHot.alpha(fl*pw))
		}
	}
}

// extOf: how far generator i's floor blades are deployed (0 retracted .. 1 on the floor)
func (e *editor) extOf(i int) float64 {
	if e.drawing {
		return 1
	}
	if v, ok := bladeExt[i]; ok {
		return v
	}
	return 0
}

func (e *editor) profileOf(i int, a, b pt2, H float64) wallProfile {
	n := len(e.pts)
	w := wallProfile{L: dist2(a, b), top: H - bladeTopIn, eA: 1, eB: 1}
	if !e.drawing && n > 0 {
		w.eA, w.eB = e.extOf(i%n), e.extOf((i+1)%n)
		w.liftA, w.liftB = e.liftOf(i%n), e.liftOf((i+1)%n)
		if e.jointPair(i%n, (i+1)%n) {
			w.floorY = floorOver
		}
	}
	return w
}

// jointPair: generators i and j stand on the same shared base
func (e *editor) jointPair(i, j int) bool {
	if i < 0 || j < 0 || i >= len(e.pts) || j >= len(e.pts) {
		return false
	}
	// a generator being dragged (or torn off) keeps its old base's spot for this test, so
	// its neighbours on the shared base don't suddenly drop their floor blades
	aj := anchorOf(e.pts[j])
	ai := anchorOf(e.pts[i])
	for _, h := range e.hands {
		if h.grab == j && !h.gNew {
			aj = h.gAnchor
		}
		if h.grab == i && !h.gNew {
			ai = h.gAnchor
		}
	}
	for _, a := range jointNbrs[i] {
		if a == aj {
			return true
		}
	}
	for _, a := range jointNbrs[j] {
		if a == ai {
			return true
		}
	}
	// the dragged one itself: its own neighbour list may be gone while it floats
	for _, h := range e.hands {
		if (h.grab == i || h.grab == j) && !h.gNew && h.gJoint && dist2(ai, aj) < jointDist {
			return true
		}
	}
	return false
}

// liftOf: how high generator i is hovering right now (same numbers drawGen/drawDeploy use)
func (e *editor) liftOf(i int) float64 {
	if i < 0 || i >= len(e.pts) {
		return 0
	}
	t := fxTime()
	p := e.pts[i]
	if _, _, floating := e.genMode(i); floating {
		return 0.12 + 0.015*math.Sin(t*5)
	}
	if rg := regrowAt(p); rg != nil {
		return math.Max(0, 0.12*(1-(t-rg.t0)/0.35))
	}
	return 0
}

func (e *editor) buildLaserWall(i int, a, b pt2, H, level float64, hot bool) {
	t := fxTime()
	L := dist2(a, b)
	if L < 1e-3 {
		return
	}
	prof := e.profileOf(i, a, b, H)
	top := prof.top
	lo := func(u float64) float64 { return prof.lo(u * L) }
	vis := math.Min(deployVis(a), deployVis(b))
	if !e.drawing && i < len(e.pts) {
		vis *= e.wallVisible(i)
	}
	if vis <= 0 {
		return
	}
	col := e.wallTint(level, hot)
	boost := float32(1 + 1.5*level)
	pulse := float32(0.85+0.15*math.Sin(t*2.4+float64(i))) * boost * float32(vis)
	fault := 0.0
	if !e.drawing && len(genFault) == len(e.pts) && len(e.pts) > 0 {
		fault = math.Max(genFault[i%len(e.pts)], genFault[(i+1)%len(e.pts)])
		if fault > 0 && noise1(float64(i)*3.3, t*11) < fault*0.55 {
			pulse *= 0.25
		}
	}
	// brownout: the whole wall sags now and then
	sag := noise1(float64(i)*7.7, t*0.9)
	if sag < 0.12 {
		pulse *= float32(0.55 + 3*sag)
	}
	at := func(u, y float64) vec3 { return v3(a[0]+(b[0]-a[0])*u, y, a[1]+(b[1]-a[1])*u) }

	addRect(a, b, 0, top, col, 0.2*float64(pulse), 5)
	e.wallReact(i, a, b, H, col)

	fr := []float64{0, 0.1, 0.45, 0.9, 1}
	as := []float32{0.07, 0.018, 0.006, 0.016, 0.05}
	cols := int(math.Max(4, L/0.12))
	us := edgeSamples(cols, prof.breaks())
	for cI := 0; cI+1 < len(us); cI++ {
		u0, u1 := us[cI], us[cI+1]
		l0, l1 := lo(u0), lo(u1)
		for k := 0; k < 4; k++ {
			c0, c1 := col.alpha(as[k]*pulse), col.alpha(as[k+1]*pulse)
			t0, t1 := prof.topAt(u0*L), prof.topAt(u1*L)
			y00, y10 := l0+(t0-l0)*fr[k], l1+(t1-l1)*fr[k]
			y01, y11 := l0+(t0-l0)*fr[k+1], l1+(t1-l1)*fr[k+1]
			bGlowTri.quadC(at(u0, y00), at(u1, y10), at(u1, y11), at(u0, y01), c0, c0, c1, c1)
		}
	}
	nx, nz := -(b[1]-a[1])/L, (b[0]-a[0])/L
	for _, sg := range []float64{-1, 1} {
		o := v3(nx*0.6*sg, 0, nz*0.6*sg)
		c0 := col.alpha(0.08 * pulse)
		bGlowTri.quadC(at(0, 0.002), at(1, 0.002), at(1, 0.002).add(o), at(0, 0.002).add(o), c0, c0, col.alpha(0), col.alpha(0))
	}

	// fine vertical filaments, uneven and drifting
	n := int(L / 0.14)
	if potatoMode {
		n /= 2
	}
	if n < 1 {
		n = 1
	}
	for k := 0; k <= n; k++ {
		u := (float64(k) + 0.4*math.Sin(t*0.3+float64(k)*1.7)) / float64(n)
		if u < 0 || u > 1 {
			continue
		}
		fi := float64(i*997 + k)
		edge := math.Max(0, 1-math.Min(u, 1-u)*L/0.25)
		y := lo(u)
		top := prof.topAt(u * L)
		span := top - y
		for seg := 0; seg < 3; seg++ {
			h := span * (0.15 + 0.45*noise1(fi+float64(seg)*7.1, t*0.45))
			y1 := math.Min(top, y+h)
			if seg == 2 {
				y1 = top
			}
			in := noise1(fi*1.3+float64(seg), t*2.2)
			alpha := float32(0.008+0.13*in*in*in*in+0.2*edge) * pulse
			bGlowLine.line(at(u, y), at(u, y1), col.alpha(alpha))
			if k%7 == 0 && in > 0.7 {
				ribbon(at(u, y), at(u, y1), 0.025, col.alpha(alpha*0.3))
			}
			y = y1
			if y >= top {
				break
			}
		}
	}
	// a single slow scan band
	sy := bladeBot + math.Mod(t*0.11+float64(i)*0.37, 1)*(top-bladeBot)
	ribbon(at(0, sy), at(1, sy), 0.2, col.alpha(0.035*pulse))

	// bright edges: along the top, and along the bottom profile (floor, then up the blades)
	edgeLine := func(p, q vec3) {
		ribbon(p, q, 0.06, col.alpha(0.4*pulse))
		ribbon(p, q, 0.015, cEnergyHot.alpha(0.6*pulse))
		bGlowLine.line(p, q, col.alpha(0.9*pulse))
	}
	edgeLine(at(0, prof.topAt(0)), at(1, prof.topAt(L)))
	eu := edgeSamples(4, prof.breaks()) // the edge is straight between corners
	for k := 0; k+1 < len(eu); k++ {
		u0, u1 := eu[k], eu[k+1]
		edgeLine(at(u0, math.Max(0.004, lo(u0))), at(u1, math.Max(0.004, lo(u1))))
	}
	e.surges(i, a, b, L, H, col, float64(pulse), fault)
}

func (e *editor) surges(i int, a, b pt2, L, H float64, col rgba, pulse, fault float64) {
	t := fxTime()
	if len(e.pts) != surgeKeyN {
		surgeKeyN = len(e.pts)
		wallSurges = map[int][]*surge{}
		surgeNext = map[int]float64{}
	}
	at := func(u, y float64) vec3 { return v3(a[0]+(b[0]-a[0])*u, y, a[1]+(b[1]-a[1])*u) }
	nx, nz := -(b[1]-a[1])/L, (b[0]-a[0])/L
	wn := v3(nx, 0, nz)
	if _, ok := surgeNext[i]; !ok {
		surgeNext[i] = t + 1 + rand.Float64()*6
	}
	if t > surgeNext[i] && pulse > 0.3 {
		rate := 25.2 + 36.5*rand.Float64()
		if fault > 0 {
			rate *= 0.4
		}
		surgeNext[i] = t + rate*(0.3+rand.ExpFloat64()*0.7)
		s := &surge{t0: t, charge: 0.12 + rand.Float64()*0.35, travel: 0.35 + rand.Float64()*0.9, seed: rand.Float64() * 100}
		if rand.Float64() < 0.5 {
			s.u0, s.dir = 0.02+rand.Float64()*0.1, 1
		} else {
			s.u0, s.dir = 0.98-rand.Float64()*0.1, -1
		}
		if rand.Float64() < 0.3 {
			s.u0 = 0.2 + rand.Float64()*0.6
		}
		s.y0 = bladeBot + 0.1 + (H-bladeTopIn-bladeBot-0.2)*math.Pow(rand.Float64(), 1.6)
		for k := range s.stalls {
			s.stalls[k] = [2]float64{rand.Float64(), rand.Float64() * 0.3}
		}
		s.lastU = s.u0
		wallSurges[i] = append(wallSurges[i], s)
		e.sfx(sndSurge, at(s.u0, 0.1), 0.35)
	}
	live := wallSurges[i][:0]
	for _, s := range wallSurges[i] {
		age := t - s.t0
		base := at(s.u0, 0.01)
		if age < s.charge {
			k := age / s.charge
			fl := 0.6 + 0.4*noise1(s.seed, t*40)
			sprite(base.add(v3(0, 0.05, 0)), 0.1+0.35*k, cEnergyHot.alpha(float32(0.5*fl)))
			sprite(base, 0.04+0.08*k, rgba{1, 1, 1, float32(0.9 * fl)})
			col2 := cEnergyHot.alpha(float32(0.5 * k * fl))
			ribbonC(base, at(s.u0, s.y0+0.2), 0.05+0.06*k, col2, col2.alpha(0))
			for q := 0; q < 2; q++ {
				arc(base, at(s.u0+(rand.Float64()-0.5)*0.08, rand.Float64()*s.y0), cEnergy)
			}
			if rand.Float64() < 0.144 {
				spawnSparks(base, wn.mul(float64(rand.Intn(2)*2-1)), 2+rand.Intn(4), 0.5)
			}
			addPoint(base.add(v3(0, 0.2, 0)), cEnergyHot, 3*k*fl, 3)
			live = append(live, s)
			continue
		}
		ta := age - s.charge
		if !s.launched {
			s.launched = true
			e.sfx(sndZip, base, 0.45)
			spawnSparks(base, v3(0, 1, 0), 18, 0.8)
		}
		k := ta / s.travel
		pu := k
		for _, st := range s.stalls {
			if k > st[0] {
				pu -= math.Min(k-st[0], st[1]) * 0.6
			}
		}
		prog := math.Pow(math.Max(0, pu), 2.3)
		target := 1.0
		if s.dir < 0 {
			target = 0
		}
		u := s.u0 + (target-s.u0)*prog
		y := s.y0 + 0.12*math.Sin(ta*13+s.seed) + 0.05*(noise1(s.seed, ta*25)-0.5)
		y = math.Max(0.05, math.Min(H-0.05, y))
		head := at(u, y)
		if !s.done {
			s.trail = append(s.trail, head)
			if len(s.trail) > 26 {
				s.trail = s.trail[1:]
			}
		}
		fade := 1.0
		if prog >= 1 {
			if !s.done {
				s.done = true
				end := at(target, y)
				spawnSparks(end, wn, 25, 0.9)
				e.sfx(sndZap, end, 0.4)
			}
			fade = math.Max(0, 1-(ta-s.travel)/0.35)
		}
		if fade <= 0 {
			continue
		}
		f := float32(fade)
		for q := 1; q < len(s.trail); q++ {
			w := float64(q) / float64(len(s.trail))
			c := cEnergyHot.alpha(float32(0.75*w*w) * f)
			jit := v3(0, (rand.Float64()-0.5)*0.03*w, 0)
			ribbon(s.trail[q-1], s.trail[q].add(jit), 0.02+0.06*w, c)
			bGlowLine.line(s.trail[q-1], s.trail[q], rgba{1, 1, 1, float32(w) * f})
		}
		if !s.done {
			sprite(head, 0.32, col.alpha(0.35))
			sprite(head, 0.12, cEnergyHot.alpha(0.8))
			sprite(head, 0.035, rgba{1, 1, 1, 1})
			if rand.Float64() < 0.144 {
				ty := 0.0
				if rand.Float64() < 0.5 {
					ty = H
				}
				arc(head, at(u-s.dir*0.03, ty), cEnergy)
			}
			addPoint(head, cEnergyHot, 4.5, 3.5)
		}
		live = append(live, s)
	}
	wallSurges[i] = live
}

// shaftNoCap: draw the shaft with its bell blown off
var shaftNoCap bool

func genShaftGeom(p pt2, H float64, prev, next pt2, lift float64) {
	c := fl(p, lift)
	defer withTex(xf{c, v3(1, 0, 0), v3(0, 1, 0), v3(0, 0, 1)})()
	up := v3(0, 1, 0)
	fd := v3(next[0]-p[0], 0, next[1]-p[1])
	if fd.length() < 1e-6 {
		fd = v3(1, 0, 0)
	}
	useMat(material{0, 0.3, 0.45, 0.45})
	cyl(&bEnv, c.add(up.mul(0.04)), c.add(up.mul(0.1)), 0.12, 0.105, 28, rgba{0.55, 0.45, 0.12, 1}, true)
	useMat(mHazard)
	cyl(&bEnv, c.add(up.mul(0.1)), c.add(up.mul(0.135)), 0.108, 0.108, 28, rgba{0.8, 0.6, 0.1, 1}, true)
	useMat(mCast)
	cyl(&bEnv, c.add(up.mul(0.135)), c.add(up.mul(0.32)), 0.082, 0.077, 28, cIron.scale(1.4), false)
	for k := 0; k < 12; k++ {
		a := float64(k) * math.Pi / 6
		dv := v3(math.Cos(a), 0, math.Sin(a))
		fr := frameAt(c.add(dv.mul(0.083)), dv.cross(up))
		bevelBox(&bEnv, fr.p(0, 0.225, 0), dv, up, dv.cross(up), 0.006, 0.08, 0.008, 0.003, cIron)
	}
	cyl(&bEnv, c.add(up.mul(0.32)), c.add(up.mul(0.36)), 0.09, 0.09, 28, cIron, true)
	useMat(mSteel)
	cyl(&bEnv, c.add(up.mul(0.36)), c.add(up.mul(0.42)), 0.072, 0.046, 28, cSteelEnv, false)
	useMat(material{1, 1, 0.22, 0.15})
	cyl(&bEnv, c.add(up.mul(0.42)), c.add(up.mul(H-0.22)), 0.042, 0.042, 28, cSteelEnv, false)
	useMat(mCast)
	for y := 0.6; y < H-0.3; y += 0.32 {
		cyl(&bEnv, c.add(up.mul(y)), c.add(up.mul(y+0.03)), 0.054, 0.054, 24, cIron.scale(1.5), true)
		useMat(mSteel)
		for b := 0; b < 4; b++ {
			a := float64(b)*math.Pi/2 + 0.4
			bp := c.add(up.mul(y + 0.015)).add(v3(math.Cos(a), 0, math.Sin(a)).mul(0.054))
			cyl(&bEnv, bp, bp.add(v3(math.Cos(a), 0, math.Sin(a)).mul(0.008)), 0.006, 0.005, 6, cSteelEnv, true)
		}
		useMat(mCast)
	}
	cyl(&bEnv, c.add(up.mul(H-0.24)), c.add(up.mul(H-0.2)), 0.06, 0.072, 28, cIron, false)
	if !shaftNoCap {
		useMat(material{0, 0.3, 0.45, 0.45})
		cyl(&bEnv, c.add(up.mul(H-0.2)), c.add(up.mul(H-0.02)), 0.072, 0.072, 28, rgba{0.55, 0.45, 0.12, 1}, false)
		hemi(&bEnv, c.add(up.mul(H-0.02)), up, 0.072, rgba{0.55, 0.45, 0.12, 1}, 6, 28)
		useMat(mSteel)
		cyl(&bEnv, c.add(up.mul(H+0.04)), c.add(up.mul(H+0.12)), 0.012, 0.008, 10, cSteelEnv, true)
	} else {
		// blown open: a scorched, jagged stump where the bell was
		useMat(mCast)
		cyl(&bEnv, c.add(up.mul(H-0.2)), c.add(up.mul(H-0.16)), 0.072, 0.07, 28, rgba{0.05, 0.045, 0.04, 1}, true)
	}
	for _, nb := range []pt2{prev, next} {
		dx, dz := nb[0]-p[0], nb[1]-p[1]
		l := math.Hypot(dx, dz)
		if l < 1e-6 {
			continue
		}
		rf := frameAt(c, v3(dx/l, 0, dz/l))
		useMat(mCast)
		bbox(&bEnv, rf, 0, (H-0.24+0.44)/2, 0.05, 0.024, (H-0.68)/2, 0.018, 0.008, cIron)
		for _, s := range []float64{-1, 1} {
			bbox(&bEnv, rf, s*0.02, (H-0.24+0.44)/2, 0.062, 0.004, (H-0.68)/2, 0.008, 0.002, cIron.scale(1.6))
		}
		// emitter heads: the top one keeps its lens in the socket; the bottom one's lens rides
		// out on the end of the floor telescope (drawn per frame in bottomLenses)
		emitterHead(rf, bottomHeadY)
		emitterHead(rf, H-0.26)
		emitterLens(rf.p(0, H-0.26, headFace), rf.z, rf.x, rf.y)
	}
	useMat(mCircuit)
	hf := frameAt(c, fd.cross(up))
	hf.o = hf.p(0, 0, 0.0855)
	restore := withTex(hf)
	bStaticQuadOn(&bEnv, hf, 0.03, 0.17, 0.035, 0.08, rgba{0.1, 0.4, 0.15, 1})
	restore()
}

const (
	bottomHeadY = bladeBot // the lower lens sits on the sheet's bottom edge line
	headFace    = 0.096    // lens socket face, along the wall direction from the post axis
)

// emitterHead: cast housing with cooling fins, a steel face plate and a recessed round lens
// socket the lens seats into
func emitterHead(rf xf, y float64) {
	useMat(mCast)
	bevelBox(&bEnv, rf.p(0, y, 0.058), rf.x, rf.y, rf.z, 0.034, 0.046, 0.03, 0.008, cIron)
	useMat(mSteel)
	bbox(&bEnv, rf, 0, y, 0.0895, 0.031, 0.043, 0.0025, 0.002, cSteelEnv.scale(0.85))
	for _, sx := range []float64{-1, 1} {
		for k := 0; k < 4; k++ {
			bbox(&bEnv, rf, sx*0.037, y-0.03+float64(k)*0.02, 0.058, 0.004, 0.0025, 0.024, 0.001, cSteelEnv.scale(0.7))
		}
	}
	// the socket: a cast ring round a dark recess
	fc := rf.p(0, y, 0.092)
	useMat(mCast)
	cyl(&bEnv, fc, fc.add(rf.z.mul(headFace-0.092)), 0.033, 0.032, 20, cIron.scale(1.3), false)
	useMat(mRubber)
	cyl(&bEnv, fc.sub(rf.z.mul(0.001)), fc.add(rf.z.mul(0.0015)), 0.028, 0.028, 20, rgba{0.02, 0.022, 0.022, 1}, true)
	// conduit from the head back into the post
	useMat(mChrome)
	cyl(&bEnv, rf.p(0, y+0.046, 0.05), rf.p(0, y+0.07, 0.03), 0.006, 0.006, 8, cChromeEnv, false)
}

// emitterLens: knurled bezel, chrome focusing collar, four screws and a green glass dome,
// facing fw from the mounting face at fc (ax, ay span the face)
func emitterLens(fc, fw, ax, ay vec3) {
	useMat(mCast)
	cyl(&bEnv, fc.sub(fw.mul(0.002)), fc.add(fw.mul(0.006)), 0.029, 0.029, 20, cIron, true)
	useMat(mChrome)
	cyl(&bEnv, fc.add(fw.mul(0.006)), fc.add(fw.mul(0.014)), 0.025, 0.023, 20, cChromeEnv, true)
	for q := 0; q < 4; q++ {
		a := float64(q)*math.Pi/2 + math.Pi/4
		bp := fc.add(ax.mul(0.0235 * math.Cos(a))).add(ay.mul(0.0235 * math.Sin(a)))
		cyl(&bEnv, bp, bp.add(fw.mul(0.008)), 0.003, 0.003, 6, cSteelEnv, true)
	}
	useMat(mGlass)
	hemi(&bEnv, fc.add(fw.mul(0.012)), fw, 0.018, rgba{0.25, 0.75, 0.42, 1}, 5, 20)
}

// baseAxisFor: orient a base so its legs and cable runs sit in the gaps between the two wall
// directions, leaving a clear path for the diagonal floor blades
func baseAxisFor(p, prev, next pt2, nbrs ...pt2) vec3 {
	dn := math.Atan2(next[1]-p[1], next[0]-p[0])
	if math.Hypot(next[0]-p[0], next[1]-p[1]) < 1e-6 {
		dn = 0
	}
	dp := math.Atan2(prev[1]-p[1], prev[0]-p[0])
	hasPrev := math.Hypot(prev[0]-p[0], prev[1]-p[1]) > 1e-6
	// hydraulics and tethers sit at 30+60k deg from the axis
	dist := func(rel float64) float64 { // angular distance (deg) to the nearest leg or cable
		m := math.Mod(rel*180/math.Pi-30, 60)
		if m < 0 {
			m += 60
		}
		return math.Min(m, 60-m)
	}
	best, bs := 0.0, -1.0
	for d := -30.0; d <= 30; d += 1.5 {
		psi := d * math.Pi / 180
		ax := dn + psi
		s := dist(dn - ax)
		if hasPrev {
			s = math.Min(s, dist(dp-ax))
		}
		for _, nb := range nbrs {
			if math.Hypot(nb[0]-p[0], nb[1]-p[1]) > 1e-6 {
				// joined neighbours count too, a bit less than the walls
				s = math.Min(s, 4+dist(math.Atan2(nb[1]-p[1], nb[0]-p[0])-ax))
			}
		}
		if s > bs+1e-9 {
			best, bs = psi, s
		}
	}
	a := dn + best
	return v3(math.Cos(a), 0, math.Sin(a))
}

func genGeometry(p pt2, H float64, prev, next, anchor pt2, nbrs []pt2, pose *basePose) {
	curPose = pose
	genBaseGeom(anchor, p, 1, false, 0, pose.ax, nbrs...)
	curPose = nil
	genShaftGeom(p, H, prev, next, 0)
}

func bStaticQuadOn(b *batch, f xf, cx, cy, hw, hh float64, col rgba) {
	b.quadF(f.p(cx-hw, cy-hh, 0), f.p(cx+hw, cy-hh, 0), f.p(cx+hw, cy+hh, 0), f.p(cx-hw, cy+hh, 0), f.z, col)
}

type genKey struct {
	p, prev, next pt2
	H             float64
	anchor, nbSum pt2
	nbN           int
	pose          [4]int32
}

type genCached struct {
	v    []float32
	used float64
}

var (
	genCache     = map[genKey]*genCached{}
	genFrameKeys []genKey
	genPrevKeys  []genKey
	bGen         batch
	genVer       int
)

func beginGenFrame() {
	chunkFrameCount++
	if chunkFrameCount%600 == 0 {
		for k, f := range chunkCacheUsed {
			if chunkFrameCount-f > 300 {
				delete(chunkCache, k)
				delete(chunkCacheUsed, k)
			}
		}
	}
	chunkFrameKeys = chunkFrameKeys[:0]
	extFrameKeys = extFrameKeys[:0]
	genFrameKeys = genFrameKeys[:0]
	plateFrameKeys = plateFrameKeys[:0]
}

// endGenFrame rebuilds the shared generator buffer only when the set of posts changed
// genPart: one cached piece of static generator geometry; each gets its own GPU buffer, so a
// change (a post moved, a plate forged) re-uploads only the pieces that changed
type genPart struct {
	key any
	v   []float32
}

var (
	genParts    []genPart
	genPartKeys []any
	genPartsVer int // bumps only when the set of static pieces changes
)

func endGenFrame() {
	sort.Strings(plateFrameKeys) // plates come out of a map in random order
	genParts = genParts[:0]
	for _, k := range genFrameKeys {
		if gc := genCache[k]; gc != nil && len(gc.v) > 0 {
			genParts = append(genParts, genPart{k, gc.v})
		}
	}
	for _, k := range plateFrameKeys {
		if v := plateCache[k]; len(v) > 0 {
			genParts = append(genParts, genPart{k, v})
		}
	}
	for _, k := range extFrameKeys {
		if v := extCache[k]; len(v) > 0 {
			genParts = append(genParts, genPart{k, v})
		}
	}
	for _, k := range chunkFrameKeys {
		if v := chunkCache[k]; len(v) > 0 {
			genParts = append(genParts, genPart{k, v})
		}
	}
	same := len(genParts) == len(genPartKeys)
	for i := 0; same && i < len(genParts); i++ {
		same = genParts[i].key == genPartKeys[i]
	}
	if !same {
		genPartKeys = genPartKeys[:0]
		for _, p := range genParts {
			genPartKeys = append(genPartKeys, p.key)
		}
		genPartsVer++
	}
	genVer++
}

func (e *editor) buildGenerator(p pt2, H float64, prev, next pt2, hot bool, col rgba, fault float64, anchor pt2, nbrs []pt2) {
	t := fxTime()
	pose := basePoseFor(anchor, prev, next, nbrs)
	key := genKey{p: p, prev: prev, next: next, H: H, anchor: anchor, nbN: len(nbrs), pose: pose.key()}
	for _, nb := range nbrs {
		key.nbSum[0] += nb[0]
		key.nbSum[1] += nb[1]
	}
	gc := genCache[key]
	if gc == nil {
		start := len(bEnv.v)
		save := primCounter
		primCounter = math.Floor(p[0]*131+p[1]*71) + 5000
		genGeometry(p, H, prev, next, anchor, nbrs, pose)
		primCounter = save
		gc = &genCached{v: append([]float32(nil), bEnv.v[start:]...)}
		bEnv.v = bEnv.v[:start]
		genCache[key] = gc
	}
	genFrameKeys = append(genFrameKeys, key)
	gc.used = t
	if len(genCache) > 80 {
		for k, v := range genCache {
			if t-v.used > 0.5 {
				delete(genCache, k)
			}
		}
	}
	e.genGlow(p, H, prev, next, hot, col, fault, 0)
}

// ---- the energy sheet reacting where something touches it ----

type wallTouch struct {
	wall     int
	u, y     float64
	t0, last float64
	k        float64
}

var wallTouches []*wallTouch

type flutter struct {
	wall                       int
	u, y, t0, life, seed, size float64
	stray                      bool
}

var (
	flutters     []flutter
	lastFlutterP vec3
	lastFlutterT float64
)

// drawFlutters: interference left in the sheet where the pointer passed - small patches of
// jittering horizontal scanlines, and a harsh salt of TV-static pixels that hangs on a little
// longer than the lines. Reseeded every few frames so it never repeats. Stray flutters are
// lone lines thrown out around the main trail.
func drawFlutters(i int, a, b pt2, col rgba, onSheet func(vec3) bool) {
	t := fxTime()
	L := dist2(a, b)
	along := v3((b[0]-a[0])/L, 0, (b[1]-a[1])/L)
	up := v3(0, 1, 0)
	hot := cEnergyHot
	white := rgba{0.85, 1, 0.9, 1}
	for _, f := range flutters {
		if f.wall != i {
			continue
		}
		age := (t - f.t0) / f.life
		if age < 0 || age >= staticLinger {
			continue
		}
		c := v3(a[0]+(b[0]-a[0])*f.u, f.y, a[1]+(b[1]-a[1])*f.u)
		frame := math.Floor(t * 24) // static re-rolls ~24 times a second
		w := f.size * (1.4 + hash1(f.seed*1.3))
		hgt := f.size * (0.9 + hash1(f.seed*2.1))
		P := func(x, y float64) vec3 { return c.add(along.mul(x)).add(up.mul(y)) }
		if age < 1 {
			// no fade: every artifact flickers at full strength; as the patch runs out of time
			// fewer and fewer of them turn up until none are left
			lines := 3 + int(hash1(f.seed+frame)*5)
			if f.stray {
				lines = 1 + int(hash1(f.seed+frame)*1.6)
			}
			for l := 0; l < lines; l++ {
				r := hash1(f.seed*7 + frame*1.7 + float64(l)*3.1)
				y := (r - 0.5) * hgt
				x0 := (hash1(f.seed*3+frame+float64(l)) - 0.5) * w
				ln := w * (0.15 + 0.6*hash1(f.seed*5+frame*0.3+float64(l)*1.9))
				p0, p1 := P(x0-ln/2, y), P(x0+ln/2, y)
				if !onSheet(p0) || !onSheet(p1) {
					continue
				}
				if hash1(f.seed*19+frame*1.1+float64(l)*7.3) < age {
					continue // thinning out
				}
				br := hash1(f.seed*9 + frame*2.3 + float64(l))
				if br < 0.25 {
					continue // dropout
				}
				cc := col
				if br > 0.85 {
					cc = hot
				}
				bGlowLine.line(p0, p1, cc.alpha(float32(0.25+0.45*br)))
			}
		}
		if f.stray {
			continue
		}
		// TV static: hard little pixels, re-rolled at 30 Hz, outliving the lines by 10%
		sa := age / staticLinger
		sframe := math.Floor(t * 30)
		n := 7 + int(hash1(f.seed*2.7+sframe)*12)      // 20% more pixels...
		px := (0.0028 + 0.002*hash1(f.seed*1.9)) * 0.8 // ...each 20% smaller
		for q := 0; q < n; q++ {
			h1 := hash1(f.seed*11 + sframe*3.3 + float64(q)*1.7)
			h2 := hash1(f.seed*13 + sframe*2.1 + float64(q)*5.3)
			h3 := hash1(f.seed*17 + sframe*1.3 + float64(q)*2.9)
			if hash1(f.seed*23+sframe*0.7+float64(q)*4.1) < sa {
				continue // thinning out
			}
			x := (h1 - 0.5) * w * 1.3
			y := (h2 - 0.5) * hgt * 1.3
			o := P(x, y)
			if !onSheet(o) {
				continue
			}
			sz := px * (0.7 + 1.6*h3*h3)
			cc := col
			if h3 > 0.6 {
				cc = white
			}
			al := float32(0.4 + 0.6*h3)
			bGlowTri.quad(o.add(along.mul(-sz)).add(up.mul(-sz*0.7)), o.add(along.mul(sz)).add(up.mul(-sz*0.7)),
				o.add(along.mul(sz)).add(up.mul(sz*0.7)), o.add(along.mul(-sz)).add(up.mul(sz*0.7)), cc.alpha(al))
		}
	}
}

// static pixels hang on 10% longer than the scanlines
const staticLinger = 1.1

// touchWall records contact at world point p on wall i (k 0..1); called every frame of contact
func (e *editor) touchWall(i int, p vec3, k float64) {
	n := len(e.pts)
	if i < 0 || i >= n {
		return
	}
	a, b := e.pts[i], e.pts[(i+1)%n]
	_, u := segDist(pt2{p.x, p.z}, a, b)
	t := fxTime()
	if k < 0.5 {
		// the pointer drags a wake of little energy flutters behind it across the sheet
		if p.sub(lastFlutterP).length() > 0.03 || t-lastFlutterT > 0.14 {
			lastFlutterP, lastFlutterT = p, t
			L := math.Max(0.1, dist2(a, b))
			for c := 0; c < 1+rand.Intn(2); c++ {
				flutters = append(flutters, flutter{wall: i, u: u + (rand.Float64()-0.5)*0.02/L, y: p.y + (rand.Float64()-0.5)*0.03,
					t0: t, life: 0.24 + rand.Float64()*0.44, seed: rand.Float64() * 1000, size: 0.025 + 0.07*rand.Float64()*rand.Float64()})
			}
			// a wide, thinning fall-off: lone lines scattered further and further from the trail
			for c := 0; c < 2+rand.Intn(2); c++ {
				r := 0.06 + 0.34*math.Pow(rand.Float64(), 1.8)
				ang := rand.Float64() * 2 * math.Pi
				flutters = append(flutters, flutter{wall: i, u: u + r*math.Cos(ang)/L, y: p.y + r*0.7*math.Sin(ang),
					t0: t + rand.Float64()*0.12, life: 0.16 + rand.Float64()*0.28, seed: rand.Float64() * 1000,
					size: 0.012 + 0.03*rand.Float64(), stray: true})
			}
			if len(flutters) > 220 {
				flutters = flutters[len(flutters)-220:]
			}
		}
	}
	for _, w := range wallTouches {
		if w.wall == i && math.Abs(w.u-u)*dist2(a, b) < 0.25 && math.Abs(w.y-p.y) < 0.25 && t-w.last < 0.2 {
			w.u, w.y, w.last = u, p.y, t
			w.k = math.Max(w.k, k)
			return
		}
	}
	wallTouches = append(wallTouches, &wallTouch{wall: i, u: u, y: p.y, t0: t, last: t, k: k})
	if len(wallTouches) > 12 {
		wallTouches = wallTouches[1:]
	}
}

func (e *editor) wallReact(i int, a, b pt2, H float64, col rgba) {
	t := fxTime()
	L := dist2(a, b)
	along := v3((b[0]-a[0])/L, 0, (b[1]-a[1])/L)
	up := v3(0, 1, 0)
	a3 := fl(a, 0)
	prof := e.profileOf(i, a, b, H)
	onSheet := func(p vec3) bool {
		x := p.sub(a3).dot(along)
		return x >= 0 && x <= L && p.y >= prof.lo(x) && p.y <= prof.topAt(x)
	}
	drawFlutters(i, a, b, col, onSheet)
	if i == 0 {
		lf := flutters[:0]
		for _, f := range flutters {
			if t-f.t0 < f.life {
				lf = append(lf, f)
			}
		}
		flutters = lf
		live := wallTouches[:0]
		for _, w := range wallTouches {
			if t-w.last <= 1.6 {
				live = append(live, w)
			}
		}
		wallTouches = live
	}
	for _, w := range wallTouches {
		if w.wall != i || t-w.last > 1.6 {
			continue
		}
		c := v3(a[0]+(b[0]-a[0])*w.u, w.y, a[1]+(b[1]-a[1])*w.u)
		on := t-w.last < 0.1
		k := w.k             // ~0.25 for the pointer, up to 1 for a hard hand touch
		R := 0.08 + 0.55*k*k // how far the disturbance spreads
		fade := 1.0
		if !on {
			fade = math.Max(0, 1-(t-w.last)/0.8)
		}
		seed := float64(w.wall)*7.3 + w.t0*1.7
		hot := cEnergyHot
		// energy flowing away from the contact like water round a stone: meandering streams of
		// soft, transparent glow drifting outward and dissolving
		streams := 6 + int(8*k)
		for sIdx := 0; sIdx < streams; sIdx++ {
			hs := hash1(seed + float64(sIdx)*3.17)
			th0 := hs*2*math.Pi + 0.3*math.Sin(t*0.7+hs*9)
			speed := (0.25 + 0.35*hash1(seed+float64(sIdx)*1.9)) * (0.5 + k)
			// the stream's channel: a faint curving filament whose brightness flows outward
			const ns = 9
			var prevP vec3
			var prevA float32
			for q := 0; q <= ns; q++ {
				r := R * float64(q) / ns
				th := th0 + 1.1*(noise1(hs*13+r*6, t*0.9)-0.5)
				pp := c.add(along.mul(math.Cos(th) * r)).add(up.mul(math.Sin(th) * r * 1.15))
				flow := 0.5 + 0.5*math.Sin((r/R-t*speed*1.6)*2*math.Pi*1.5+hs*20)
				al := float32(flow * flow * (1 - r/R) * fade * (0.05 + 0.12*k))
				if q > 0 && onSheet(pp) && onSheet(prevP) {
					ribbonC(prevP, pp, 0.006+0.014*k, col.alpha(prevA), col.alpha(al))
				}
				prevP, prevA = pp, al
			}
			const beads = 7
			for bIdx := 0; bIdx < beads; bIdx++ {
				ph := math.Mod(t*speed/R+float64(bIdx)/beads+hs, 1)
				r := ph * R
				// curl: the path bends with noise as it travels
				th := th0 + 1.1*(noise1(hs*13+r*6, t*0.9)-0.5)
				p := c.add(along.mul(math.Cos(th) * r)).add(up.mul(math.Sin(th) * r * 1.15))
				if !onSheet(p) {
					continue
				}
				life := ph * (1 - ph) * 4
				al := float32(life * fade * (0.08 + 0.18*k) * (0.6 + 0.4*noise1(hs*5+float64(bIdx), t*3)))
				sz := (0.012 + 0.03*k) * (0.6 + 0.8*hash1(hs*7+float64(bIdx)))
				sprite(p, sz, col.alpha(al))
				if hash1(hs*11+float64(bIdx)) > 0.7 {
					sprite(p, sz*0.4, hot.alpha(al*1.4))
				}
			}
		}
		// a soft, broken shimmer front rather than a solid ring
		front := math.Mod(t-w.t0, 0.9) / 0.9
		if !on {
			front = math.Min(1, (t-w.last)/0.8)
		}
		const segs = 22
		for sg := 0; sg < segs; sg++ {
			a0 := float64(sg) * 2 * math.Pi / segs
			gap := noise1(seed+float64(sg)*0.9, t*2.2)
			if gap < 0.45 {
				continue
			}
			rr := R * front * (0.85 + 0.3*noise1(float64(sg)*1.3, t*4))
			p := c.add(along.mul(math.Cos(a0) * rr)).add(up.mul(math.Sin(a0) * rr * 1.15))
			if !onSheet(p) {
				continue
			}
			sprite(p, 0.02+0.04*k, col.alpha(float32((1-front)*(gap-0.45)*0.5*fade*(0.3+0.7*k))))
		}
		if !on {
			continue
		}
		// contact point: a small hot core, brighter for a real touch
		sprite(c, 0.04+0.12*k, col.alpha(float32(0.15+0.25*k)))
		sprite(c, 0.015+0.03*k, hot.alpha(float32(0.5+0.3*k)))
		addPoint(c, col, 0.6+4*k*k, 1+2*k)
		if k > 0.5 && rand.Float64() < 0.2 {
			d := along.mul(rand.NormFloat64()).add(up.mul(rand.NormFloat64() * 0.7)).norm()
			end := c.add(d.mul(0.12 + 0.3*rand.Float64()*k))
			end.y = math.Max(0.02, math.Min(H, end.y))
			if onSheet(end) {
				arc(c, end, hot.alpha(0.6))
			}
		}
	}
}

type pulseKey struct{ p, nb pt2 }

type railPt struct{ h, size, ph, f float64 }

var railPts = map[pulseKey][]railPt{}

// sideWallPower: power level of the wall leaving generator gi on side 0 (prev) or 1 (next)
func (e *editor) sideWallPower(gi, side int) float64 {
	n := len(e.pts)
	if gi < 0 || n < 2 || e.drawing {
		return 1
	}
	w := gi
	if side == 0 {
		w = (gi - 1 + n) % n
	}
	if v, ok := wallPow[w]; ok && n == wallKeyN {
		return v
	}
	return 1
}

func (e *editor) genGlow(p pt2, H float64, prev, next pt2, hot bool, col rgba, fault, lift float64) {
	t := fxTime()
	// damp vapour pooling round the base, like moisture boiling off the coils
	addFog(v3(p[0], 0.3, p[1]), 1.05, 0.8+0.2*math.Sin(t*0.4+p[0]*3), 0.45)
	if rand.Float64() < frameDT*0.5 {
		a := rand.Float64() * 2 * math.Pi
		r := 0.15 + rand.Float64()*0.55
		spawnMist(v3(p[0]+math.Cos(a)*r, 0.08+rand.Float64()*0.35, p[1]+math.Sin(a)*r), 0.4)
	}
	c := fl(p, lift)
	up := v3(0, 1, 0)
	flick := float32(1)
	statusCol := cEnergy
	if fault > 0 {
		statusCol = cDanger
		if noise1(p[0]*13+p[1]*7, t*9) < fault*0.7 {
			flick = 0.12
			col = cDanger
		}
		if rand.Float64() < fault*frameDT*0.96 {
			sp := v3(p[0], 0.4+rand.Float64()*(H-0.8), p[1])
			n := v3(rand.NormFloat64(), 0, rand.NormFloat64()).norm()
			spawnSparks(sp.add(n.mul(0.05)), n, 10+rand.Intn(14), 0.5)
			spawnSmoke(sp, 3)
			sprite(sp, 0.3, cSpark.alpha(0.7))
			addPoint(sp, cSpark, 6, 3)
			e.sfx(sndSpark, sp, 0.4*fault)
		}
	}
	ring := col
	if hot {
		ring = cAmber
	}
	shadowBlob(p, 0.3, 0.35)
	for k := 0; k < 12; k++ {
		a := float64(k)*math.Pi/6 + math.Pi/12
		dv := v3(math.Cos(a), 0, math.Sin(a))
		ribbon(c.add(dv.mul(0.083)).add(up.mul(0.16)), c.add(dv.mul(0.083)).add(up.mul(0.29)), 0.012, ring.alpha(0.5*flick))
	}
	blink := 1.0
	if fault > 0 {
		blink = math.Max(0, math.Sin(t*8))
	}
	sprite(c.add(up.mul(H+0.13)), 0.1, statusCol.alpha(float32(0.8*blink)))
	sprite(c.add(up.mul(H+0.13)), 0.02, rgba{1, 1, 1, float32(0.9 * blink)})
	if blink > 0.1 {
		addPoint(c.add(up.mul(H+0.13)), statusCol, 0.8*blink, 2)
	}
	gi := e.idxOf(p)
	for side, nb := range []pt2{prev, next} {
		dx, dz := nb[0]-p[0], nb[1]-p[1]
		l := math.Hypot(dx, dz)
		if l < 1e-6 {
			continue
		}
		dv := v3(dx/l, 0, dz/l)
		o := c.add(dv.mul(0.071))
		// a blade facing a wall a robot has switched off goes dark with it
		wp := float32(e.sideWallPower(gi, side))
		fl := float32(0.75+0.25*math.Sin(t*11+dx*7)) * flick * (0.12 + 0.88*wp)
		// the lens on the top emitter box glows where the top edge of the sheet leaves it
		lp := c.add(dv.mul(headFace + 0.04)).add(up.mul(H - 0.26))
		sprite(lp, 0.075, cEnergy.alpha(0.6*fl))
		sprite(lp, 0.03, cEnergyHot.alpha(1.0*fl))
		ribbon(lp, lp.add(dv.mul(0.09)), 0.006, cEnergyHot.alpha(0.6*fl))
		ribbon(o.add(up.mul(0.44)), o.add(up.mul(H-0.26)), 0.03, ring.alpha(0.45*fl))
		ribbon(o.add(up.mul(0.44)), o.add(up.mul(H-0.26)), 0.008, cEnergyHot.alpha(0.8*fl))
		// hot points held along the blade, like energy bleeding off it into the sheet: they
		// hold their place, wander a little, flicker, and spit short jets sideways
		key := pulseKey{p, nb}
		pts, ok := railPts[key]
		if !ok {
			if len(railPts) > 400 {
				railPts = map[pulseKey][]railPt{}
			}
			cnt := 13 + rand.Intn(6)
			for q := 0; q < cnt; q++ {
				sz := 0.02 + 0.05*math.Pow(rand.Float64(), 2.2)
				pts = append(pts, railPt{h: rand.Float64(), size: sz, ph: rand.Float64() * 100, f: 0.4 + rand.Float64()*1.1})
			}
			railPts[key] = pts
		}
		span := (H - 0.26) - 0.44
		side := dv
		for qi, q := range pts {
			if potatoMode && qi%3 != 0 {
				continue
			}
			y := 0.44 + span*q.h + 0.03*math.Sin(t*0.8*q.f+q.ph) + 0.012*(noise1(q.ph, t*1.36)-0.5)
			pp := o.add(up.mul(y))
			fk := noise1(q.ph*3.1, t*0.8*(0.8+0.8*q.f)) // pulse rate x0.8
			br := float32(0.25 + 0.75*fk*fk)
			sprite(pp, q.size*(0.8+0.4*fk), cEnergyHot.alpha(0.55*br*fl))
			if q.size > 0.045 {
				sprite(pp, q.size*0.35, rgba{0.9, 1, 0.92, 0.7 * br * fl})
			}
			// jet into the sheet: length and strength flicker with the point
			if wp < 0.3 {
				continue
			}
			jl := (0.03 + 0.16*fk*fk) * (0.6 + q.size*12) * float64(wp)
			ribbonC(pp, pp.add(side.mul(jl)), q.size*0.5, cEnergyHot.alpha(0.45*br*fl), cEnergyHot.alpha(0))
		}
	}
	for _, y := range []float64{0.12, 0.34, H - 0.22} {
		pa := float32(0.6 + 0.4*math.Sin(t*5+y*3))
		ringSoft(c.add(up.mul(y)), 0.112, 0.015, ring.alpha(0.45*pa*flick), 32)
	}
	if hot {
		floorGlow(p, 0.35, cAmber.alpha(0.35))
		addPoint(c.add(up.mul(0.3)), cAmber, 1.5, 2)
	}
	floorGlow(p, 0.25, ring.alpha(0.1*flick))
}
