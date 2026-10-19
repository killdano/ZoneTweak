//go:build windows

package main

import (
	"math"
	"math/rand"
)

// environment: industrial cell block built once per room layout, animated emitters every frame

type envEmitter struct {
	a, b, n vec3
	col     rgba
	kind    int
	phase   float64
	broken  float64
}

type steamVent struct {
	p, dir vec3
	next   float64
}

var (
	envKey     [8]float64
	envEmit    []envEmitter
	envDoors   []rbDoor
	envVents   []steamVent
	envHave    bool
	envG       rigid2
	envBox     [4]float64
	envFans    []xf
	envLamps   []vec3
	staticVer  int
	cPanelW    = rgba{0.30, 0.32, 0.33, 1}
	cPlateDk   = rgba{0.17, 0.18, 0.19, 1}
	cPipeGreen = rgba{0.22, 0.29, 0.22, 1}
	cPipeRed   = rgba{0.36, 0.12, 0.08, 1}
	cPipeYel   = rgba{0.62, 0.50, 0.12, 1}
	cSteelEnv  = rgba{0.58, 0.60, 0.62, 1}
	cIron      = rgba{0.20, 0.20, 0.21, 1}
)

const envH = 5.2

func (e *editor) buildEnvironment() {
	if !envHave || !e.manipulating() {
		g, x0, x1, z0, z1 := e.roomFrame()
		box := [4]float64{x0, x1, z0, z1}
		switch {
		case !envHave:
			envG, envBox, envHave = g, box, true
		default:
			// the cell only turns if the boundary's main direction swung a long way; otherwise it
			// keeps its frame and just grows or shrinks, and either way only where you can't see
			d := math.Atan2(g.c, g.a) - math.Atan2(envG.c, envG.a)
			d = math.Mod(d+math.Pi/4, math.Pi/2)
			if d < 0 {
				d += math.Pi / 2
			}
			d -= math.Pi / 4
			if math.Abs(d) > 12*math.Pi/180 && e.roomChangeHidden(true, box) {
				envG, envBox = g, box
				break
			}
			ob := [4]float64{}
			ob[0], ob[1], ob[2], ob[3] = boxIn(envG, e.pts)
			// each wall moves on its own, as soon as its own strip is out of view, so a room
			// that grew on two opposite sides doesn't wait for a view that hides both at once
			for i := 0; i < 4; i++ {
				if ob[i] == envBox[i] {
					continue
				}
				nb := envBox
				nb[i] = ob[i]
				if e.roomChangeHidden(false, nb) {
					envBox = nb
				}
			}
		}
	}
	g := envG
	x0, x1, z0, z1 := envBox[0], envBox[1], envBox[2], envBox[3]
	world.g, world.ok = g, true
	world.lx0, world.lx1, world.lz0, world.lz1 = x0-1.35, x1+1.35, z0-1.35, z1+1.35
	m := 2.6
	key := [8]float64{g.a, g.b, g.tx, g.tz, x0 - m, x1 + m, z0 - m, z1 + m}
	if key != envKey {
		envKey = key
		e.buildStatic(g, x0-m, x1+m, z0-m, z1+m)
		staticVer++
		if R != nil {
			R.staticVer = staticVer
		}
	}
	world.doors = envDoors
	e.animateEnv()
}

// boxIn: the boundary's bounds in frame g, rounded out to half metres
func boxIn(g rigid2, pts []pt2) (float64, float64, float64, float64) {
	inv := g.inv()
	x0, x1, z0, z1 := -2.0, 2.0, -2.0, 2.0
	for i, p := range pts {
		q := inv.apply(p)
		if i == 0 {
			x0, x1, z0, z1 = q[0], q[0], q[1], q[1]
		}
		x0, x1 = math.Min(x0, q[0]), math.Max(x1, q[0])
		z0, z1 = math.Min(z0, q[1]), math.Max(z1, q[1])
	}
	return math.Floor(x0*2) / 2, math.Ceil(x1*2) / 2, math.Floor(z0*2) / 2, math.Ceil(z1*2) / 2
}

// roomChangeHidden: true when every part of the cell that the change would touch is out of
// the player's view. A box change only touches strips along the walls that move (plus the
// ends of what spans the room); a change of frame touches everything, so it waits until
// you look straight down at the deck or up at the ceiling.
func (e *editor) roomChangeHidden(major bool, nb [4]float64) bool {
	eye := e.hmd.pos()
	fwd := e.hmd.axis(2).mul(-1)
	if major {
		return math.Abs(fwd.y) > 0.82
	}
	const m = 2.6
	cosLim := math.Cos(72 * math.Pi / 180) // wider than any headset's half-FOV
	g := envG
	visible := func(gx0, gx1, gz0, gz1 float64) bool {
		for i := 0; i <= 6; i++ {
			for j := 0; j <= 6; j++ {
				for _, y := range []float64{0.05, envH * 0.5, envH - 0.1} {
					q := g.apply(pt2{gx0 + (gx1-gx0)*float64(i)/6, gz0 + (gz1-gz0)*float64(j)/6})
					d := v3(q[0], y, q[1]).sub(eye)
					l := d.length()
					if l < 1e-3 || d.dot(fwd)/l > cosLim {
						return true
					}
				}
			}
		}
		return false
	}
	ob := envBox
	lo, hi := math.Min(ob[2], nb[2])-m, math.Max(ob[3], nb[3])+m
	if ob[0] != nb[0] && visible(math.Min(ob[0], nb[0])-m-0.3, math.Max(ob[0], nb[0])-m+1.8, lo, hi) {
		return false
	}
	if ob[1] != nb[1] && visible(math.Min(ob[1], nb[1])+m-1.8, math.Max(ob[1], nb[1])+m+0.3, lo, hi) {
		return false
	}
	lo, hi = math.Min(ob[0], nb[0])-m, math.Max(ob[1], nb[1])+m
	if ob[2] != nb[2] && visible(lo, hi, math.Min(ob[2], nb[2])-m-0.3, math.Max(ob[2], nb[2])-m+1.8) {
		return false
	}
	if ob[3] != nb[3] && visible(lo, hi, math.Min(ob[3], nb[3])+m-1.8, math.Max(ob[3], nb[3])+m+0.3) {
		return false
	}
	return true
}

// envBays: wall bays on a fixed grid in the room frame (so walls that don't move keep exactly
// the same bays when the cell grows or shrinks); returns [c0,c1] ranges and grid indices
func envBays(lo, hi float64) ([][2]float64, []int) {
	const bw = 1.6
	var cuts []float64
	cuts = append(cuts, lo)
	for k := math.Ceil((lo-bw/2)/bw + 1e-9); bw*k+bw/2 < hi-1e-9; k++ {
		c := bw*k + bw/2
		if c > lo+1e-9 {
			cuts = append(cuts, c)
		}
	}
	cuts = append(cuts, hi)
	// fold slivers at the ends into their neighbours
	if len(cuts) > 2 && cuts[1]-cuts[0] < 0.8 {
		cuts = append(cuts[:1], cuts[2:]...)
	}
	if n := len(cuts); n > 2 && cuts[n-1]-cuts[n-2] < 0.8 {
		cuts = append(cuts[:n-2], cuts[n-1])
	}
	var out [][2]float64
	var idx []int
	for i := 0; i+1 < len(cuts); i++ {
		out = append(out, [2]float64{cuts[i], cuts[i+1]})
		idx = append(idx, int(math.Round((cuts[i]+cuts[i+1])/2/bw)))
	}
	return out, idx
}

func (e *editor) buildStatic(g rigid2, x0, x1, z0, z1 float64) {
	bStatic.reset()
	bStaticL.reset()
	bFloorS.reset()
	bFloorQ.reset()
	envEmit = envEmit[:0]
	envDoors = envDoors[:0]
	envVents = envVents[:0]
	envFans = envFans[:0]
	envLamps = envLamps[:0]
	envJunctions = envJunctions[:0]
	primCounter = 1000
	stableTex = true
	defer func() { stableTex = false }()
	W := func(x, z float64) pt2 { return g.apply(pt2{x, z}) }
	gx := g.apply(pt2{1, 0})
	X := v3(gx[0]-g.tx, 0, gx[1]-g.tz)
	gz := g.apply(pt2{0, 1})
	Z := v3(gz[0]-g.tx, 0, gz[1]-g.tz)
	up := v3(0, 1, 0)
	H := envH
	texF = xf{vec3{}, vec3{1, 0, 0}, vec3{0, 1, 0}, vec3{0, 0, 1}}
	texOff = vec3{}

	o := fl(W(0, 0), 0) // anchored to the room frame, not its corner: resizing doesn't slide the deck
	floorFrame = xf{o, X, up, Z}
	useMat(mSteel)
	bFloorQ.quadF(fl(W(x0-0.3, z0-0.3), 0), fl(W(x1+0.3, z0-0.3), 0), fl(W(x1+0.3, z1+0.3), 0), fl(W(x0-0.3, z1+0.3), 0), up, rgba{0.2, 0.2, 0.2, 1})

	// ceiling deck + trusses
	useMat(mWall)
	bStatic.quadF(fl(W(x0, z0), H), fl(W(x1, z0), H), fl(W(x1, z1), H), fl(W(x0, z1), H), v3(0, -1, 0), cPlateDk)
	// trusses on a fixed 2 m grid, built in 0.6 m sections so their ends are the only thing
	// that changes when the cell resizes
	for x := 2 * math.Ceil((x0+0.6)/2); x < x1-0.6; x += 2 {
		f := xf{fl(W(x, 0), 0), X, up, Z}
		for za := 0.6 * math.Floor(z0/0.6); za < z1; za += 0.6 {
			a, b := math.Max(za, z0), math.Min(za+0.6, z1)
			if b-a < 1e-3 {
				continue
			}
			zm, hh := (a+b)/2, (b-a)/2
			useMat(mCast)
			bbox(&bStatic, f, 0, H-0.03, zm, 0.11, 0.03, hh, 0.008, cIron)
			bbox(&bStatic, f, 0, H-0.42, zm, 0.11, 0.025, hh, 0.008, cIron)
			bbox(&bStatic, f, 0, H-0.225, zm, 0.012, 0.18, hh, 0.004, cIron)
			if a == za && b == za+0.6 {
				ya, yb := H-0.4, H-0.05
				if int(math.Round(za/0.6))%2 != 0 {
					ya, yb = yb, ya
				}
				for _, sd := range []float64{-0.07, 0.07} {
					pa := f.p(sd, ya, za)
					pb := f.p(sd, yb, za+0.6)
					fr := segFrame(pa, pb, X)
					bevelBox(&bStatic, fr.o, fr.x, fr.y, fr.z, 0.012, 0.02, pb.sub(pa).length()/2, 0.005, cIron)
				}
				useMat(mSteel)
				for _, sd := range []float64{-1, 1} {
					bp := f.p(sd*0.085, H-0.39, za+0.3)
					cyl(&bStatic, bp, bp.add(up.mul(-0.012)), 0.01, 0.01, 6, cSteelEnv, true)
				}
			}
		}
	}
	// cable trays near ceiling along X, in 1.2 m sections on a fixed grid
	for ti, zz := range []float64{z0 + 0.9, z1 - 0.9} {
		f := xf{fl(W(0, zz), 0), X, up, Z}
		for xa := 1.2 * math.Floor(x0/1.2); xa < x1; xa += 1.2 {
			a, b := math.Max(xa, x0), math.Min(xa+1.2, x1)
			if b-a < 1e-3 {
				continue
			}
			xm, hh := (a+b)/2, (b-a)/2
			useMat(mSteel)
			bbox(&bStatic, f, xm, H-0.75, 0, hh, 0.008, 0.18, 0.003, cSteelEnv.scale(0.7))
			for _, sd := range []float64{-1, 1} {
				bbox(&bStatic, f, xm, H-0.71, sd*0.18, hh, 0.04, 0.006, 0.003, cSteelEnv.scale(0.7))
			}
			if xa < x0+0.4 || xa+1.2 > x1-0.4 {
				continue
			}
			for _, sd := range []float64{-1, 1} {
				cyl(&bStatic, f.p(xa, H-0.75, sd*0.17), f.p(xa, H-0.42, sd*0.17), 0.008, 0.008, 6, cSteelEnv, false)
			}
			useMat(mRubber)
			k := int(math.Round(xa / 1.2))
			cr := rand.New(rand.NewSource(int64(ti*100000 + k*37 + 777)))
			for c := 0; c < 5; c++ {
				zoff := -0.12 + float64(c)*0.06
				sag := 0.04 + 0.12*cr.Float64()
				if c == 4 && cr.Float64() < 0.3 {
					sag = 0.5 + 0.4*cr.Float64()
				}
				pa := f.p(xa, H-0.72, zoff)
				pb := f.p(xa+1.2, H-0.72, zoff)
				d := up.mul(-sag * 1.33)
				hose(&bStatic, pa, pa.add(pb.sub(pa).mul(0.3)).add(d), pa.add(pb.sub(pa).mul(0.7)).add(d), pb, 0.012+0.006*float64(c%2), rgba{0.04 + 0.02*float32(c%3), 0.04, 0.04, 1}, 10)
			}
		}
	}

	type side struct {
		a, b pt2
		in   pt2
	}
	sides := []side{
		{pt2{x0, z0}, pt2{x1, z0}, pt2{0, 1}},
		{pt2{x1, z0}, pt2{x1, z1}, pt2{-1, 0}},
		{pt2{x1, z1}, pt2{x0, z1}, pt2{0, -1}},
		{pt2{x0, z1}, pt2{x0, z0}, pt2{1, 0}},
	}
	for si, s := range sides {
		L := dist2(s.a, s.b)
		dx, dz := (s.b[0]-s.a[0])/L, (s.b[1]-s.a[1])/L
		inw := X.mul(s.in[0]).add(Z.mul(s.in[1]))
		along := X.mul(dx).add(Z.mul(dz))
		P := func(u, d float64) pt2 { return W(s.a[0]+dx*u+s.in[0]*d, s.a[1]+dz*u+s.in[1]*d) }
		pipeCols := []rgba{cPipeGreen, cPipeRed, cPipeYel}
		// the side's own coordinate in the room frame (x for sides 0/2, z for 1/3)
		lo, hi := s.a[0], s.b[0]
		if si%2 == 1 {
			lo, hi = s.a[1], s.b[1]
		}
		rev := hi < lo
		if rev {
			lo, hi = hi, lo
		}
		bays, gidx := envBays(lo, hi)
		doorBay := -1
		for bi, b := range bays {
			if (si == 0 || si == 2) && b[0] <= 0 && b[1] > 0 {
				doorBay = bi
			}
		}
		for bi, b := range bays {
			u0, u1 := b[0]-lo, b[1]-lo
			if rev {
				u0, u1 = hi-b[1], hi-b[0]
			}
			um := (u0 + u1) / 2
			hw := (u1 - u0) / 2
			k := gidx[bi]
			rng := rand.New(rand.NewSource(int64(si*100000 + k*131 + 4242)))
			fr := xf{fl(P(um, 0), 0), along, up, inw}
			texF = xf{vec3{}, vec3{1, 0, 0}, vec3{0, 1, 0}, vec3{0, 0, 1}}
			door := bi == doorBay
			if door {
				e.buildDoor(fr, hw, H)
			} else if ((k+si)%3+3)%3 == 1 {
				e.buildAlcove(fr, hw, H, rng)
			} else {
				e.buildPanelBay(fr, hw, H, rng, si, k)
			}
			// I-beam pillar
			useMat(mCast)
			bbox(&bStatic, fr, -hw, H/2, 0.012, 0.012, H/2, 0.09, 0.004, cIron)
			for _, sz := range []float64{-1, 1} {
				_ = sz
			}
			bbox(&bStatic, fr, -hw, H/2, 0.105, 0.075, H/2, 0.012, 0.004, cIron)
			bbox(&bStatic, fr, -hw, H/2, -0.075, 0.075, H/2, 0.012, 0.004, cIron)
			useMat(mSteel)
			for y := 0.25; y < H-0.2; y += 0.42 {
				for _, sx := range []float64{-0.05, 0.05} {
					bp := fr.p(-hw+sx, y, 0.117)
					cyl(&bStatic, bp, bp.add(inw.mul(0.012)), 0.011, 0.009, 6, cSteelEnv, true)
				}
			}
			bbox(&bStatic, fr, -hw, 0.06, 0.05, 0.12, 0.06, 0.12, 0.01, cIron)
			// pipe runs with flanges at the pillars
			for pi, py := range []float64{0.55, 3.55, 4.35} {
				r := []float64{0.06, 0.08, 0.045}[pi]
				d := 0.26 + float64(pi)*0.04
				col := pipeCols[(pi+si)%3]
				useMat(material{0, 0.3, 0.55, 0.65})
				if door && py < 3 {
					// the low run climbs up and over the doorway so nothing crosses the robots' path
					dw := hw - 0.15
					xo, yo := dw+0.3, 3.08
					cyl(&bStatic, fr.p(-hw, py, d), fr.p(-xo, py, d), r, r, 16, col, false)
					cyl(&bStatic, fr.p(-xo, py, d), fr.p(-xo, yo, d), r, r, 16, col, false)
					cyl(&bStatic, fr.p(-xo, yo, d), fr.p(xo, yo, d), r, r, 16, col, false)
					cyl(&bStatic, fr.p(xo, yo, d), fr.p(xo, py, d), r, r, 16, col, false)
					cyl(&bStatic, fr.p(xo, py, d), fr.p(hw, py, d), r, r, 16, col, false)
					useMat(mCast)
					for _, q := range [][2]float64{{-xo, py}, {-xo, yo}, {xo, yo}, {xo, py}} {
						sphere(&bStatic, fr.p(q[0], q[1], d), r*1.35, cIron, 8, 12)
					}
					for _, sx := range []float64{-1, 1} {
						bbox(&bStatic, fr, sx*xo, 1.8, d/2, 0.015, 0.025, d/2, 0.005, cIron)
					}
				} else {
					cyl(&bStatic, fr.p(-hw, py, d), fr.p(hw, py, d), r, r, 16, col, false)
				}
				useMat(mCast)
				cyl(&bStatic, fr.p(-hw+0.02, py, d), fr.p(-hw+0.06, py, d), r*1.5, r*1.5, 16, cIron, true)
				bbox(&bStatic, fr, -hw+0.04, py, d/2, 0.015, 0.025, d/2, 0.005, cIron)
				useMat(mSteel)
				for b := 0; b < 6; b++ {
					a := float64(b) * math.Pi / 3
					bp := fr.p(-hw+0.065, py+math.Sin(a)*r*1.25, d+math.Cos(a)*r*1.25)
					cyl(&bStatic, bp, bp.add(along.mul(0.012)), 0.008, 0.008, 6, cSteelEnv, true)
				}
			}
			useMat(mCast)
			bbox(&bStatic, fr, 0, H-0.15, 0.1, hw, 0.15, 0.1, 0.02, cIron)
		}
	}
	// two shadowed ceiling lamps (with a fan under one)
	cx, cz := 0.0, 0.0 // the room frame's origin, so the lamps stay put when it resizes
	for i, off := range []float64{-1.3, 1.3} {
		p := fl(W(cx+off, cz), H-0.75)
		envLamps = append(envLamps, p)
		useMat(mSteel)
		for c := 0; c < 6; c++ {
			a := p.add(up.mul(0.75 - float64(c)*0.11))
			cyl(&bStatic, a, a.add(up.mul(-0.09)), 0.008, 0.008, 6, cIron, true)
		}
		useMat(mPaint)
		cyl(&bStatic, p.add(up.mul(0.08)), p.add(up.mul(0.0)), 0.06, 0.08, 18, cPipeGreen.scale(0.7), true)
		cyl(&bStatic, p.add(up.mul(0.0)), p.add(up.mul(-0.16)), 0.08, 0.32, 24, cPipeGreen.scale(0.7), false)
		curEmis = 14
		sphere(&bStatic, p.add(up.mul(-0.07)), 0.06, rgba{1, 0.93, 0.8, 1}, 6, 12)
		curEmis = 1
		if i == 0 {
			envFans = append(envFans, xf{p.add(up.mul(-0.75)), X, up, Z})
		}
	}
}

func (e *editor) buildPanelBay(fr xf, hw, H float64, rng *rand.Rand, si, k int) {
	inw, along, up := fr.z, fr.x, fr.y
	useMat(mWall)
	bStatic.quadF(fr.p(-hw, 0, 0), fr.p(hw, 0, 0), fr.p(hw, H, 0), fr.p(-hw, H, 0), inw, cPanelW.scale(0.8))
	useMat(material{3, 0.4, 0.5, 0.75})
	bbox(&bStatic, fr, 0, 0.45, 0.04, hw-0.01, 0.45, 0.04, 0.012, cPlateDk)
	for y := 0.12; y < 0.85; y += 0.24 {
		bbox(&bStatic, fr, 0, y, 0.083, hw-0.06, 0.008, 0.004, 0.002, cPlateDk.scale(0.8))
	}
	useMat(mWall)
	bbox(&bStatic, fr, 0, 1.45, 0.015, hw-0.03, 0.52, 0.015, 0.008, cPanelW)
	useMat(mCast)
	bbox(&bStatic, fr, 0, 2.22, 0.05, hw-0.01, 0.22, 0.05, 0.02, cIron)
	for _, y := range []float64{2.12, 2.32} {
		a, b := fr.p(-hw+0.14, y, 0.103), fr.p(hw-0.14, y, 0.103)
		curEmis = 5
		bStatic.quadC(a.add(up.mul(-0.022)), b.add(up.mul(-0.022)), b.add(up.mul(0.022)), a.add(up.mul(0.022)), cLight, cLight, cLight, cLight)
		curEmis = 1
		br := 0.0
		if rng.Float64() < 0.22 {
			br = 0.5 + rng.Float64()*0.5
		}
		envEmit = append(envEmit, envEmitter{a: a, b: b, n: inw, col: cLight, kind: 0, phase: float64(k) + float64(si)*1.7 + rng.Float64(), broken: br})
	}
	useMat(mWall)
	for _, pr := range [][2]float64{{2.5, 3.3}, {3.3, 4.9}} {
		y0, y1 := pr[0], pr[1]
		hy := (y1 - y0) / 2
		bbox(&bStatic, fr, 0, y0+hy, 0.014, hw-0.06, hy-0.02, 0.014, 0.008, cPanelW.scale(1.05))
		for gg := -2; gg <= 2; gg++ {
			bbox(&bStatic, fr, float64(gg)*(hw-0.08)/2, y0+hy, 0.032, 0.012, hy-0.05, 0.006, 0.003, cPlateDk)
		}
	}
	// wall vent low
	if rng.Float64() < 0.4 {
		useMat(mCast)
		vx := (rng.Float64() - 0.5) * (hw - 0.4)
		bbox(&bStatic, fr, vx, 0.22, 0.09, 0.22, 0.12, 0.01, 0.004, cIron)
		for s := 0; s < 6; s++ {
			bbox(&bStatic, fr, vx, 0.13+float64(s)*0.035, 0.105, 0.2, 0.006, 0.012, 0.002, cIron.scale(1.3))
		}
		if rng.Float64() < 0.6 {
			envVents = append(envVents, steamVent{p: fr.p(vx, 0.22, 0.12), dir: inw.add(up.mul(0.3)).norm(), next: rng.Float64() * 8})
		}
	}
	// junction box with an exposed board
	if rng.Float64() < 0.45 {
		jx := -hw + 0.3 + rng.Float64()*(2*hw-0.6)
		jy := 1.2 + rng.Float64()*0.5
		useMat(material{0, 0.2, 0.5, 0.7})
		bbox(&bStatic, fr, jx, jy, 0.06, 0.16, 0.2, 0.05, 0.012, cPipeGreen.scale(0.6))
		useMat(mCircuit)
		bStatic.quadF(fr.p(jx-0.13, jy-0.17, 0.112), fr.p(jx+0.13, jy-0.17, 0.112), fr.p(jx+0.13, jy+0.17, 0.112), fr.p(jx-0.13, jy+0.17, 0.112), inw, rgba{0.1, 0.4, 0.15, 1})
		useMat(material{0, 0.2, 0.5, 0.7})
		door := fr.p(jx+0.16, jy, 0.11)
		dn := inw.mul(math.Cos(1.1)).add(along.mul(math.Sin(1.1)))
		bevelBox(&bStatic, door.add(dn.mul(0.16)), dn, up, dn.cross(up).mul(-1), 0.16, 0.2, 0.006, 0.004, cPipeGreen.scale(0.6))
		useMat(mRubber)
		for c := 0; c < 3; c++ {
			a := fr.p(jx-0.08+float64(c)*0.06, jy-0.2, 0.08)
			b := fr.p(jx-0.12+float64(c)*0.09, 0.62, 0.3)
			hose(&bStatic, a, a.add(up.mul(-0.2)), b.add(up.mul(0.15)), b, 0.01, rgba{0.05, 0.05 + 0.04*float32(c), 0.05, 1}, 8)
		}
		envEmit = append(envEmit, envEmitter{a: fr.p(jx+0.1, jy+0.15, 0.115), b: fr.p(jx+0.1, jy+0.15, 0.115), n: inw, col: cEnergy, kind: 3, phase: jx * 7})
		envJunctions = append(envJunctions, junction{c: fr.p(jx, jy, 0.113), n: inw, x: along, base: fl(pt2{fr.p(jx, 0, 0).x, fr.p(jx, 0, 0).z}, 0)})
	}
	// valve on the lower pipe
	if rng.Float64() < 0.35 {
		vx := -hw + 0.3 + rng.Float64()*(2*hw-0.6)
		c := fr.p(vx, 0.55, 0.26)
		useMat(mCast)
		cyl(&bStatic, c.add(inw.mul(-0.0)), c.add(inw.mul(0.14)), 0.03, 0.03, 10, cIron, true)
		useMat(material{0, 0.3, 0.45, 0.8})
		wc := c.add(inw.mul(0.15))
		const seg = 14
		for s := 0; s < seg; s++ {
			a0, a1 := float64(s)*2*math.Pi/seg, float64(s+1)*2*math.Pi/seg
			p0 := wc.add(along.mul(0.09 * math.Cos(a0))).add(up.mul(0.09 * math.Sin(a0)))
			p1 := wc.add(along.mul(0.09 * math.Cos(a1))).add(up.mul(0.09 * math.Sin(a1)))
			cyl(&bStatic, p0, p1, 0.011, 0.011, 6, cPipeRed, false)
		}
		for s := 0; s < 4; s++ {
			a := float64(s) * math.Pi / 2
			cyl(&bStatic, wc, wc.add(along.mul(0.09*math.Cos(a))).add(up.mul(0.09*math.Sin(a))), 0.007, 0.007, 6, cPipeRed, false)
		}
	}
	useMat(mHazard)
	bbox(&bStatic, fr, 0, 0.02, 0.33, hw, 0.02, 0.035, 0.006, rgba{0.8, 0.6, 0.1, 1})
}

func (e *editor) buildAlcove(fr xf, hw, H float64, rng *rand.Rand) {
	inw, along, up := fr.z, fr.x, fr.y
	useMat(mWall)
	bStatic.quadF(fr.p(-hw, 0, -0.35), fr.p(hw, 0, -0.35), fr.p(hw, H, -0.35), fr.p(-hw, H, -0.35), inw, cPlateDk)
	bStatic.quadF(fr.p(-hw, 0, 0), fr.p(-hw, 0, -0.35), fr.p(-hw, H, -0.35), fr.p(-hw, H, 0), along, cPlateDk)
	bStatic.quadF(fr.p(hw, 0, -0.35), fr.p(hw, 0, 0), fr.p(hw, H, 0), fr.p(hw, H, -0.35), along.mul(-1), cPlateDk)
	bStatic.quadF(fr.p(-hw, 0.001, 0), fr.p(hw, 0.001, 0), fr.p(hw, 0.001, -0.35), fr.p(-hw, 0.001, -0.35), up, cPlateDk.scale(0.7))
	useMat(mCast)
	for x := -hw + 0.12; x < hw; x += 0.16 {
		bbox(&bStatic, fr, x, H/2, -0.33, 0.025, H/2, 0.02, 0.006, cIron)
	}
	useMat(material{1, 0.9, 0.45, 0.6})
	for _, cx := range []float64{-hw * 0.5, hw * 0.5} {
		cyl(&bStatic, fr.p(cx, 0, -0.22), fr.p(cx, H, -0.22), 0.035, 0.035, 12, cSteelEnv.scale(0.8), false)
		for y := 0.4; y < H; y += 0.9 {
			bbox(&bStatic, fr, cx, y, -0.27, 0.05, 0.02, 0.06, 0.006, cIron)
		}
	}
	for li, y := range []float64{1.35 + rng.Float64()*0.3, 3.4 + rng.Float64()*0.5} {
		lx := (rng.Float64() - 0.5) * hw * 0.6
		c := fr.p(lx, y, -0.24)
		useMat(mCast)
		bbox(&bStatic, fr, lx, y, -0.31, 0.09, 0.09, 0.03, 0.01, cIron)
		cyl(&bStatic, c.add(inw.mul(-0.06)), c.add(inw.mul(0.0)), 0.075, 0.07, 20, cIron, true)
		curEmis = 6
		hemi(&bStatic, c, inw, 0.055, rgba{1, 0.72, 0.4, 1}, 4, 16)
		curEmis = 1
		useMat(mSteel)
		for k := 0; k < 6; k++ {
			a := float64(k) * math.Pi / 3
			d0 := along.mul(math.Cos(a)).add(up.mul(math.Sin(a)))
			hose2 := c.add(d0.mul(0.072))
			cyl(&bStatic, hose2, c.add(d0.mul(0.03)).add(inw.mul(0.075)), 0.005, 0.005, 6, cSteelEnv, false)
		}
		for _, rr := range []float64{0.072, 0.05} {
			const seg = 16
			for k := 0; k < seg; k++ {
				a0, a1 := float64(k)*2*math.Pi/seg, float64(k+1)*2*math.Pi/seg
				z := 0.035
				if rr < 0.06 {
					z = 0.068
				}
				cyl(&bStatic, c.add(along.mul(rr*math.Cos(a0))).add(up.mul(rr*math.Sin(a0))).add(inw.mul(z)), c.add(along.mul(rr*math.Cos(a1))).add(up.mul(rr*math.Sin(a1))).add(inw.mul(z)), 0.004, 0.004, 5, cSteelEnv, false)
			}
		}
		br := 0.0
		if rng.Float64() < 0.3 || li == 1 && rng.Float64() < 0.3 {
			br = 0.6 + 0.4*rng.Float64()
		}
		envEmit = append(envEmit, envEmitter{a: c.add(inw.mul(0.05)), b: c.add(inw.mul(0.05)), n: inw, col: rgba{1, 0.72, 0.4, 1}, kind: 1, phase: y + rng.Float64()*3, broken: br})
	}
	useMat(mHazard)
	bbox(&bStatic, fr, 0, 0.03, 0.02, hw, 0.03, 0.02, 0.006, rgba{0.8, 0.6, 0.1, 1})
}

func (e *editor) buildDoor(fr xf, hw, H float64) {
	const dh, dd = 2.7, 2.6
	inw, side, up := fr.z, fr.x, fr.y
	useMat(mWall)
	bStatic.quadF(fr.p(-hw, dh, 0), fr.p(hw, dh, 0), fr.p(hw, H, 0), fr.p(-hw, H, 0), inw, cPanelW.scale(0.8))
	dw := hw - 0.15
	bStatic.quadF(fr.p(-hw, 0, 0), fr.p(-dw, 0, 0), fr.p(-dw, dh, 0), fr.p(-hw, dh, 0), inw, cPanelW.scale(0.8))
	bStatic.quadF(fr.p(dw, 0, 0), fr.p(hw, 0, 0), fr.p(hw, dh, 0), fr.p(dw, dh, 0), inw, cPanelW.scale(0.8))
	dark := cPlateDk.scale(0.5)
	bStatic.quadF(fr.p(-dw, 0, 0), fr.p(-dw, 0, -dd), fr.p(-dw, dh, -dd), fr.p(-dw, dh, 0), side, dark)
	bStatic.quadF(fr.p(dw, 0, -dd), fr.p(dw, 0, 0), fr.p(dw, dh, 0), fr.p(dw, dh, -dd), side.mul(-1), dark)
	bStatic.quadF(fr.p(-dw, dh, 0), fr.p(-dw, dh, -dd), fr.p(dw, dh, -dd), fr.p(dw, dh, 0), up.mul(-1), dark)
	bStatic.quadF(fr.p(-dw, 0, -dd), fr.p(dw, 0, -dd), fr.p(dw, dh, -dd), fr.p(-dw, dh, -dd), inw, rgba{0.02, 0.02, 0.025, 1})
	useMat(mSteel)
	bStatic.quadF(fr.p(-dw, 0.001, 0), fr.p(dw, 0.001, 0), fr.p(dw, 0.001, -dd), fr.p(-dw, 0.001, -dd), up, cPlateDk)
	useMat(mCast)
	for _, sx := range []float64{-1, 1} {
		bbox(&bStatic, fr, sx*(dw+0.07), dh/2, 0.06, 0.08, dh/2, 0.06, 0.02, cIron)
		for y := 0.3; y < dh; y += 0.5 {
			bbox(&bStatic, fr, sx*(dw+0.07), y, 0.13, 0.05, 0.08, 0.02, 0.006, cIron.scale(1.3))
		}
		// hydraulic door rams
		useMat(mChrome)
		cyl(&bStatic, fr.p(sx*(dw+0.03), dh+0.1, 0.18), fr.p(sx*(dw-0.2), dh-0.25, 0.18), 0.02, 0.02, 10, cSteelEnv, true)
		useMat(mCast)
		cyl(&bStatic, fr.p(sx*(dw+0.08), dh+0.2, 0.18), fr.p(sx*(dw+0.03), dh+0.1, 0.18), 0.04, 0.04, 12, cIron, true)
	}
	useMat(mHazard)
	bbox(&bStatic, fr, 0, dh+0.09, 0.06, dw+0.15, 0.09, 0.06, 0.02, rgba{0.8, 0.6, 0.1, 1})
	bbox(&bStatic, fr, 0, 0.005, 0.4, dw, 0.005, 0.2, 0.002, rgba{0.8, 0.6, 0.1, 1})
	for _, sx := range []float64{-1, 1} {
		p := fr.p(sx*(dw+0.07), dh+0.35, 0.12)
		useMat(mCast)
		cyl(&bStatic, p.sub(up.mul(0.06)), p, 0.05, 0.05, 12, cIron, true)
		curEmis = 3
		hemi(&bStatic, p, up, 0.045, cRobotEye, 3, 12)
		curEmis = 1
		envEmit = append(envEmit, envEmitter{a: p.add(up.mul(0.02)), b: p.add(up.mul(0.02)), n: inw, col: cRobotEye, kind: 4, phase: sx})
	}
	for d := 0.4; d < dd; d += 0.7 {
		a, b := fr.p(-dw+0.05, dh-0.02, -d), fr.p(dw-0.05, dh-0.02, -d)
		curEmis = 4
		c := cRobotEye.scale(0.8)
		bStatic.quadC(a.add(inw.mul(-0.03)), b.add(inw.mul(-0.03)), b.add(inw.mul(0.03)), a.add(inw.mul(0.03)), c, c, c, c)
		curEmis = 1
		envEmit = append(envEmit, envEmitter{a: a, b: b, n: up.mul(-1), col: cRobotEye, kind: 5, phase: d})
	}
	mo, en := fr.p(0, 0, 0.3), fr.p(0, 0, -dd+0.4)
	envDoors = append(envDoors, rbDoor{mouth: pt2{mo.x, mo.z}, end: pt2{en.x, en.z}})
}

func flickerOf(em envEmitter, t float64) float64 {
	if em.broken <= 0 {
		return 1
	}
	n := noise1(em.phase*3.1, t*7)
	if n < em.broken*0.55 {
		return 0.05
	}
	if noise1(em.phase*5.7, t*31) < em.broken*0.25 {
		return 0.3
	}
	return 1
}

func (e *editor) animateEnv() {
	t := fxTime()
	cam := camPos
	for _, l := range envEmit {
		d := l.b.sub(l.a)
		L := d.length()
		fl := flickerOf(l, t)
		near := l.a.sub(cam).length() < 12
		switch l.kind {
		case 0:
			pulse := (0.85 + 0.15*math.Sin(t*0.8+l.phase)) * fl
			ribbon(l.a.add(l.n.mul(0.01)), l.b.add(l.n.mul(0.01)), 0.07, l.col.alpha(float32(0.12*pulse)))
			if near {
				addLine(l.a.add(l.n.mul(0.05)), l.b.add(l.n.mul(0.05)), l.col, 1.2*pulse, 5)
			}
			if fl < 0.5 && rand.Float64() < 0.0012 {
				spawnSparks(l.a.add(d.mul(rand.Float64())), l.n, 6, 0.3)
				e.sfx(sndSpark, l.a, 0.25)
			}
		case 1:
			pulse := (0.9 + 0.1*math.Sin(t*1.3+l.phase*2)) * fl
			sprite(l.a, 0.22, l.col.alpha(float32(0.12*pulse)))
			if near {
				addPoint(l.a.add(l.n.mul(0.08)), l.col, 1.4*pulse, 4)
			}
		case 3:
			c := l.col
			if math.Mod(t*0.7+l.phase*0.37, 1) >= 0.5 {
				c = cAmber
			}
			sprite(l.a, 0.03, c.alpha(0.6))
			sprite(l.a, 0.008, rgba{1, 1, 1, 0.8})
		case 4:
			b := math.Max(0, math.Sin(t*3+l.phase))
			sprite(l.a, 0.25, l.col.alpha(float32(0.3*b)))
			ang := t*4 + l.phase
			dir := vec3{math.Cos(ang), -0.15, math.Sin(ang)}.norm()
			ribbonC(l.a, l.a.add(dir.mul(1.4)), 0.25, l.col.alpha(0.12), l.col.alpha(0))
			addPoint(l.a.add(dir.mul(0.2)), l.col, 1.5*(0.4+0.6*b), 5)
		case 5:
			ribbon(l.a, l.b, 0.05, l.col.alpha(0.15))
		}
		_ = L
	}
	for i := range envVents {
		v := &envVents[i]
		if t > v.next {
			v.next = t + 5 + rand.Float64()*12.5
			spawnSteam(v.p, v.dir, 20)
			e.sfx(sndSteam, v.p, 0.4)
		}
	}
	for fi, f := range envFans {
		seedTex(30000 + float64(fi)*500)
		a := t * 3.4
		useMat(mCast)
		cyl(&bEnv, f.o.add(f.y.mul(0.05)), f.o.add(f.y.mul(-0.08)), 0.08, 0.08, 16, cIron, true)
		cyl(&bEnv, f.o.add(f.y.mul(0.05)), f.o.add(f.y.mul(0.75)), 0.015, 0.015, 8, cIron, false)
		useMat(material{0, 0.3, 0.55, 0.7})
		for k := 0; k < 4; k++ {
			b := a + float64(k)*math.Pi/2
			dir := f.x.mul(math.Cos(b)).add(f.z.mul(math.Sin(b)))
			tan := dir.cross(f.y)
			c := f.o.add(dir.mul(0.42))
			bevelBox(&bEnv, c, dir, f.y.mul(0.97).add(tan.mul(0.25)).norm(), tan.mul(0.97).sub(f.y.mul(0.25)).norm(), 0.36, 0.008, 0.12, 0.004, cPipeGreen.scale(0.5))
		}
		useMat(mSteel)
		const seg = 32
		for s := 0; s < seg; s++ {
			a0, a1 := float64(s)*2*math.Pi/seg, float64(s+1)*2*math.Pi/seg
			cyl(&bEnv, f.o.add(f.x.mul(0.82*math.Cos(a0))).add(f.z.mul(0.82*math.Sin(a0))), f.o.add(f.x.mul(0.82*math.Cos(a1))).add(f.z.mul(0.82*math.Sin(a1))), 0.008, 0.008, 5, cSteelEnv, false)
		}
	}
	// drifting haze patches and steam pooled at the robot doors
	if envHave {
		g := envG
		x0, x1, z0, z1 := envBox[0]-2.0, envBox[1]+2.0, envBox[2]-2.0, envBox[3]+2.0
		for k := 0; k < 4; k++ {
			hx := x0 + (x1-x0)*hash1(float64(k)*3.3+1.1) + 0.6*math.Sin(t*0.05+float64(k))
			hz := z0 + (z1-z0)*hash1(float64(k)*7.7+2.2) + 0.6*math.Cos(t*0.04+float64(k)*2)
			q := g.apply(pt2{hx, hz})
			hy := 0.4 + 1.0*hash1(float64(k)*5.1)
			hr := 1.4 + 0.8*hash1(float64(k))
			addFog(v3(q[0], hy, q[1]), hr, 0.3, 0.7)
			if rand.Float64() < frameDT*0.35 {
				a, r := rand.Float64()*2*math.Pi, rand.Float64()*hr*0.7
				spawnMist(v3(q[0]+math.Cos(a)*r, hy+(rand.Float64()-0.5)*0.8, q[1]+math.Sin(a)*r), 0.95)
			}
		}
		for i, d := range envDoors {
			addFog(v3(d.mouth[0], 0.9, d.mouth[1]), 1.6, 0.6, 1.2)
			// a constant low roll of steam out of the door mouth
			if rand.Float64() < frameDT*1.2 {
				dir := v3(d.mouth[0]-d.end[0], 0, d.mouth[1]-d.end[1]).norm()
				side := v3(-dir.z, 0, dir.x)
				spawnMist(fl(d.mouth, 0.2+rand.Float64()*1.8).add(side.mul((rand.Float64()-0.5)*1.4)).sub(dir.mul(0.3)), 0.7)
			}
			if t > doorSteamNext[i%2] {
				doorSteamNext[i%2] = t + 8.75 + rand.Float64()*11.25
				e.doorSteam(i)
			}
		}
	}
	spots = spots[:0]
	for i, p := range envLamps {
		k := 70.0
		if i == 1 {
			k *= 0.4 + 0.6*flickerOf(envEmitter{phase: 9.3, broken: 0.35}, t)
		}
		spots = append(spots, makeSpot(p.add(vec3{0, -0.2, 0}), vec3{0, -1, 0}, rgba{1, 0.9, 0.75, 1}, k, 60, 28, 10))
		sprite(p.add(vec3{0, -0.1, 0}), 0.4, rgba{1, 0.9, 0.75, 0.2})
	}
}

var doorSteamNext [2]float64

func (e *editor) doorSteam(i int) {
	if i >= len(envDoors) {
		return
	}
	d := envDoors[i]
	dir := v3(d.mouth[0]-d.end[0], 0, d.mouth[1]-d.end[1]).norm()
	side := v3(-dir.z, 0, dir.x)
	for _, s := range []float64{-0.55, 0.55} {
		p := fl(d.mouth, 2.55).add(side.mul(s)).sub(dir.mul(0.25))
		spawnSteam(p, dir.mul(0.6).add(v3(0, -0.5, 0)).norm(), 35)
		e.sfx(sndSteam, p, 0.36)
	}
}
