//go:build windows

package main

import (
	"log"
	"math"
	"math/rand"
)

const sndScan = 9

type bsState struct {
	next, start float64
	stripes     [][2]float64
}

var (
	rawBases   []rawBase
	bsStates   = map[uint64]*bsState{}
	bsLoadedAt = -100.0
	cScan      = rgba{0.55, 0.92, 1.0, 1}
)

func (e *editor) refreshBases() {
	t := fxTime()
	if t-bsLoadedAt < 30 {
		return
	}
	first := bsLoadedAt < 0
	bsLoadedAt = t
	rb, id, path := loadRawBaseStations()
	if first {
		if len(rb) == 0 {
			log.Printf("base stations: none found")
		} else {
			log.Printf("base stations: %d from %s (universe %s)", len(rb), path, id)
		}
	}
	if len(rb) > 0 {
		rawBases = rb
	}
}

func (s *bsState) schedule(t float64, first bool) {
	if first {
		s.next = t + 4 + rand.Float64()*8
	} else {
		s.next = t + 20 + rand.Float64()*45
	}
}

func (e *editor) roomExit(o, d vec3) float64 {
	if !envHave {
		return 7
	}
	inv := envG.inv()
	q := inv.apply(pt2{o.x, o.z})
	q2 := inv.apply(pt2{o.x + d.x, o.z + d.z})
	dx, dz := q2[0]-q[0], q2[1]-q[1]
	lo := [3]float64{envBox[0] - 2.6, 0, envBox[2] - 2.6}
	hi := [3]float64{envBox[1] + 2.6, envH, envBox[3] + 2.6}
	P := [3]float64{q[0], o.y, q[1]}
	D := [3]float64{dx, d.y, dz}
	tmax := 12.0
	for i := 0; i < 3; i++ {
		if math.Abs(D[i]) < 1e-9 {
			continue
		}
		t1 := (lo[i] - P[i]) / D[i]
		t2 := (hi[i] - P[i]) / D[i]
		tmax = math.Min(tmax, math.Max(t1, t2))
	}
	return math.Max(0.05, tmax)
}

func (e *editor) drawBaseStations() {
	if e.st == nil {
		return
	}
	e.refreshBases()
	if len(rawBases) == 0 {
		return
	}
	t := fxTime()
	for _, b := range e.st.stageBases(rawBases) {
		s := bsStates[b.serial]
		if s == nil {
			s = &bsState{}
			s.schedule(t, true)
			bsStates[b.serial] = s
		}
		e.drawBaseModel(b, t)
		if s.start == 0 && t >= s.next {
			s.start = t
			s.stripes = s.stripes[:0]
			off := 0.0
			for k := 0; k < 6+rand.Intn(3); k++ {
				w := (0.3 + rand.Float64()*1.4) * math.Pi / 180
				s.stripes = append(s.stripes, [2]float64{off, w})
				off += w + (0.4+rand.Float64()*1.8)*math.Pi/180
			}
			e.sfx(sndScan, b.pos, 0.7)
		}
		if s.start > 0 {
			age := t - s.start
			if age > 2.0 {
				s.start = 0
				s.schedule(t, false)
			} else {
				e.drawScan(b, s, age)
			}
		}
	}
}

type bsKey struct {
	serial       uint64
	pos, back, r vec3
}

func (e *editor) drawBaseModel(b baseStation, t float64) {
	seedTex(float64(b.serial % 100000))
	fwd := b.back.mul(-1)
	R, U := b.right, b.up
	defer cleanRoles()()
	F := xf{b.pos, R, U, fwd}
	// the unit and its arm are built once into the static buffer; only the lights animate
	cachedChunk(bsKey{b.serial, b.pos, b.back, b.right}, func() {
		bxFX = false
		bbox(&bEnv, F, 0, 0, -0.032, 0.047, 0.047, 0.03, 0.014, cFrame)
		bbox(&bEnv, F, 0, 0, 0.0, 0.043, 0.043, 0.006, 0.01, rgba{0.02, 0.022, 0.026, 1})
		bbox(&bEnv, F, 0, 0, -0.066, 0.03, 0.03, 0.006, 0.006, cMetalDk)
		for _, sx := range []float64{-1, 1} {
			bbox(&bEnv, F, sx*0.049, 0, -0.03, 0.004, 0.04, 0.026, 0.002, cArmor)
		}
		e.drawBioArm(b, F, t)
		bxFX = true
	})
	bxGeom = false
	e.drawBioArm(b, F, t)
	bxGeom = true

	slot := F.p(0, 0, 0.007)
	a, c := F.p(-0.036, 0, 0.007), F.p(0.036, 0, 0.007)
	ribbon(a, c, 0.012, cScan.alpha(0.18))
	sh := math.Mod(t*7.3, 1)
	sprite(a.add(c.sub(a).mul(sh)), 0.012, cScan.alpha(0.5))
	sprite(slot, 0.05, cScan.alpha(0.06))
	led := F.p(-0.03, 0.032, 0.008)
	sprite(led, 0.008, cEnergy.alpha(0.9))
	sprite(led, 0.025, cEnergy.alpha(0.2))
}

func (e *editor) drawScan(b baseStation, s *bsState, age float64) {
	fwd := b.back.mul(-1)
	R, U := b.right, b.up
	const hfov, vfov = 75 * math.Pi / 180, 55 * math.Pi / 180
	var tilt, ph float64
	switch {
	case age < 0.9:
		tilt, ph = math.Pi/6, age/0.9
	case age >= 1.0 && age < 1.9:
		tilt, ph = -math.Pi/6, (age-1.0)/0.9
	default:
		return
	}
	env := float32(math.Sin(ph * math.Pi))
	fr := float32(math.Min(1, age/0.15) * math.Min(1, (2-age)/0.2))
	for _, sg := range [][2]float64{{-hfov, -vfov}, {hfov, -vfov}, {hfov, vfov}, {-hfov, vfov}} {
		d := fwd.mul(math.Cos(sg[0]) * math.Cos(sg[1])).add(R.mul(math.Sin(sg[0]) * math.Cos(sg[1]))).add(U.mul(math.Sin(sg[1])))
		L := e.roomExit(b.pos, d)
		ribbonC(b.pos, b.pos.add(d.mul(L)), 0.01, cScan.alpha(0.25*fr), cScan.alpha(0))
	}
	sprite(b.pos.add(fwd.mul(0.01)), 0.14, cScan.alpha(0.5*env))
	addPoint(b.pos.add(fwd.mul(0.1)), cScan, 2.5*float64(env), 4)
	sprite(b.pos.add(fwd.mul(0.01)), 0.03, rgba{1, 1, 1, 0.9 * env})
	base := -hfov + 2*hfov*ph
	const segs = 16
	for _, st := range s.stripes {
		phi := base + st[0]
		f := fwd.mul(math.Cos(phi)).add(R.mul(math.Sin(phi)))
		side := f.cross(U).norm()
		up := U.mul(math.Cos(tilt)).add(side.mul(math.Sin(tilt))).norm()
		var prev vec3
		var prevOK bool
		for i := 0; i <= segs; i++ {
			th := -vfov + 2*vfov*float64(i)/segs
			d := f.mul(math.Cos(th)).add(up.mul(math.Sin(th))).norm()
			L := e.roomExit(b.pos, d)
			p := b.pos.add(d.mul(L))
			if prevOK {
				near := cScan.alpha(0.16 * env)
				far := cScan.alpha(0.03 * env)
				bGlowTri.vert(b.pos, near)
				bGlowTri.vert(prev, far)
				bGlowTri.vert(p, far)
				bGlowLine.line(prev, p, cScan.alpha(0.9*env))
				ribbon(prev, p, 0.02+0.01*st[1]*180/math.Pi, cScan.alpha(0.35*env))
			}
			prev, prevOK = p, true
		}
	}
}

var cVein = rgba{1.0, 0.32, 0.12, 1}

func vertebrae(a, b vec3, r0, r1 float64, t, phase float64) {
	d := b.sub(a)
	L := d.length()
	if L < 1e-4 {
		return
	}
	z := d.mul(1 / L)
	n := int(L / 0.055)
	if !bxGeom {
		n = -1
	} else {
		cyl(&bEnv, a, b, r1*0.45, r1*0.45, 10, cChrome.scale(0.7), false)
	}
	for k := 0; k <= n; k++ {
		f := float64(k) / float64(n+1)
		r := r0 + (r1-r0)*f
		c := a.add(d.mul(f))
		col := cFrame
		if k%2 == 1 {
			col = cArmor.scale(0.8)
		}
		cyl(&bEnv, c, c.add(z.mul(0.032)), r, r*0.92, 12, col, true)
	}
	x, y := basis(z)
	for i, off := range []vec3{x.mul(r0 * 1.15), x.mul(-r0 * 1.15)} {
		bulge := y.mul(r0 * 0.6)
		if bxGeom {
			hose(&bEnv, a.add(off), a.add(d.mul(0.33)).add(off.mul(1.4)).add(bulge), a.add(d.mul(0.66)).add(off.mul(1.4)).add(bulge), b.add(off.mul(0.8)), 0.007, cHose, 8)
		}
		if !bxFX {
			continue
		}
		pulse := float32(0.35 + 0.35*math.Max(0, math.Sin(t*2.2-float64(i)*0.8+phase)))
		va, vb := a.add(y.mul(-r0*1.05)), b.add(y.mul(-r1*1.05))
		ribbon(va, vb, 0.006, cVein.alpha(0.18*pulse))
		s := math.Mod(t*0.7+phase+float64(i)*0.5, 1)
		sprite(va.add(vb.sub(va).mul(s)), 0.025, cVein.alpha(0.35*pulse))
	}
}

func (e *editor) drawBioArm(b baseStation, F xf, t float64) {
	up := vec3{0, 1, 0}
	hb := vec3{b.back.x, 0, b.back.z}
	if hb.length() < 1e-3 {
		hb = vec3{1, 0, 0}
	}
	hb = hb.norm()
	ph := float64(b.serial%97) * 0.3
	A := vec3{b.pos.x + hb.x*0.75, envH, b.pos.z + hb.z*0.75}
	G := bxGeom
	if G {
		bevelBox(&bEnv, A.sub(up.mul(0.03)), vec3{hb.z, 0, -hb.x}, up, hb, 0.16, 0.03, 0.16, 0.015, cMetalDk)
		for _, s := range []float64{-1, 1} {
			for _, q := range []float64{-1, 1} {
				bp := A.add(vec3{hb.z, 0, -hb.x}.mul(s * 0.12)).add(hb.mul(q * 0.12)).sub(up.mul(0.065))
				cyl(&bEnv, bp, bp.add(up.mul(0.01)), 0.012, 0.012, 6, cChrome, true)
			}
		}
	}
	S := A.sub(up.mul(0.14))
	W := b.pos.add(up.mul(0.1))
	L := W.sub(S).length()
	l1, l2 := math.Max(0.5, L*0.6), math.Max(0.45, L*0.55)
	E := ik(S, W, l1, l2, hb.add(up.mul(0.6)))
	if G {
		cyl(&bEnv, A.sub(up.mul(0.06)), S, 0.05, 0.06, 14, cFrame, true)
		ball(S, 0.07, cArmor)
	}
	vertebrae(S, E, 0.05, 0.042, t, ph)
	if G {
		ball(E, 0.055, cArmor)
		bevelBox(&bEnv, E.add(hb.mul(0.03)), vec3{hb.z, 0, -hb.x}, up, hb, 0.05, 0.05, 0.035, 0.015, cFrame)
	}
	if bxFX {
		sprite(E.add(hb.mul(0.07)), 0.025, cVein.alpha(float32(0.4+0.3*math.Sin(t*2.2+ph))))
	}
	vertebrae(E, W, 0.04, 0.03, t, ph+1)
	if G {
		ball(W, 0.035, cChrome)
		cyl(&bEnv, W, W.sub(up.mul(0.035)), 0.03, 0.045, 12, cFrame, true)
	}
	for k := 0; k < 3; k++ {
		a := float64(k)*2*math.Pi/3 + math.Pi/2
		rd := F.x.mul(math.Cos(a)).add(F.z.mul(math.Sin(a))).norm()
		k0 := W.sub(up.mul(0.03)).add(rd.mul(0.03))
		k1 := b.pos.add(rd.mul(0.068)).add(up.mul(0.04))
		k2 := b.pos.add(rd.mul(0.054)).sub(up.mul(0.015))
		if G {
			ball(k0, 0.011, cChrome)
		}
		vertebrae(k0, k1, 0.012, 0.01, t, ph+float64(k))
		if G {
			ball(k1, 0.01, cChrome)
			cyl(&bEnv, k1, k2, 0.009, 0.004, 8, cArmor, true)
		}
	}
}
