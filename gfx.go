package main

import (
	"math"
	"time"
)

type tbatch struct{ v []float32 }

func (b *tbatch) reset() { b.v = b.v[:0] }

func (b *tbatch) vert(p vec3, u, v float32, c rgba) {
	b.v = append(b.v, float32(p.x), float32(p.y), float32(p.z), u, v, c.r, c.g, c.b, c.a)
}

func (b *tbatch) quad(p0, p1, p2, p3 vec3, uv [4][2]float32, c0, c1, c2, c3 rgba) {
	b.vert(p0, uv[0][0], uv[0][1], c0)
	b.vert(p1, uv[1][0], uv[1][1], c1)
	b.vert(p2, uv[2][0], uv[2][1], c2)
	b.vert(p0, uv[0][0], uv[0][1], c0)
	b.vert(p2, uv[2][0], uv[2][1], c2)
	b.vert(p3, uv[3][0], uv[3][1], c3)
}

var (
	bSoft, bShadow tbatch
	camPos         vec3
	camR           = v3(1, 0, 0)
	camU           = v3(0, 1, 0)
	fillDir        = v3(-0.6, 0.3, -0.5).norm()
	lightDir       = v3(0.35, 0.8, 0.45).norm()
	bounceDir      = v3(0, 1, 0)
	bounceCol      rgba
	bounceAmt      float64
	matSpec        = 0.5
	matGloss       = 28.0
	rimCol         = rgba{0.35, 0.65, 0.75, 1}
	uvSprite       = [4][2]float32{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	uvRibbon       = [4][2]float32{{0.5, 0}, {0.5, 1}, {0.5, 1}, {0.5, 0}}
)

func (c rgba) scale(f float32) rgba { return rgba{c.r * f, c.g * f, c.b * f, c.a} }

func (c rgba) mix(d rgba, t float32) rgba {
	return rgba{c.r + (d.r-c.r)*t, c.g + (d.g-c.g)*t, c.b + (d.b-c.b)*t, c.a + (d.a-c.a)*t}
}

func shadeP(c rgba, n, p vec3) rgba {
	v := camPos.sub(p)
	if l := v.length(); l > 1e-6 {
		v = v.mul(1 / l)
	}
	dk := n.dot(lightDir)
	if dk < 0 {
		dk = 0
	}
	fill := n.dot(fillDir) * 0.22
	if fill < 0 {
		fill = 0
	}
	amb := 0.16 + 0.10*(n.y*0.5+0.5)
	l := float32(amb + 0.72*dk + fill)
	spec := 0.0
	if matSpec > 0 {
		hx, hy, hz := lightDir.x+v.x, lightDir.y+v.y, lightDir.z+v.z
		hl := hx*hx + hy*hy + hz*hz
		if hl > 1e-12 {
			x := (n.x*hx + n.y*hy + n.z*hz) / math.Sqrt(hl)
			if x > 0 {
				x *= x
				x *= x
				x *= x
				x *= x
				if matGloss > 24 {
					x *= x
				}
				spec = x * matSpec
			}
		}
	}
	nv := n.dot(v)
	if nv < 0 {
		nv = 0
	}
	rv := 1 - nv
	rim := float32(rv * rv * rv * 0.28)
	bo := float32(0)
	if bounceAmt > 0 {
		if d := n.dot(bounceDir); d > 0 {
			bo = float32(d * bounceAmt)
		}
	}
	s := float32(spec)
	return rgba{
		c.r*l + s + rimCol.r*rim*c.r*2 + bounceCol.r*bo,
		c.g*l + s + rimCol.g*rim*c.g*2 + bounceCol.g*bo,
		c.b*l + s + rimCol.b*rim*c.b*2 + bounceCol.b*bo,
		1,
	}
}

func shade(base rgba, n vec3) rgba {
	l := float32(0.35 + 0.65*math.Max(0, n.dot(lightDir)))
	return rgba{base.r * l, base.g * l, base.b * l, 1}
}

func (b *batch) vertS(p, n vec3, c rgba) { b.vertL(p, n, c) }

func (b *batch) quadS(p0, p1, p2, p3, n0, n1, n2, n3 vec3, c rgba) {
	b.vertL(p0, n0, c)
	b.vertL(p1, n1, c)
	b.vertL(p2, n2, c)
	b.vertL(p0, n0, c)
	b.vertL(p2, n2, c)
	b.vertL(p3, n3, c)
}

func (b *batch) triS(p0, p1, p2, n vec3, c rgba) {
	b.vertL(p0, n, c)
	b.vertL(p1, n, c)
	b.vertL(p2, n, c)
}

func withTex(f xf) func() {
	save, so := texF, texOff
	texF = f
	nextTexOff()
	return func() { texF, texOff = save, so }
}

func (b *batch) quadF(p0, p1, p2, p3, n vec3, c rgba) { b.quadS(p0, p1, p2, p3, n, n, n, n, c) }

func (b *batch) quadC(p0, p1, p2, p3 vec3, c0, c1, c2, c3 rgba) {
	b.vert(p0, c0)
	b.vert(p1, c1)
	b.vert(p2, c2)
	b.vert(p0, c0)
	b.vert(p2, c2)
	b.vert(p3, c3)
}

func basis(z vec3) (vec3, vec3) {
	up := v3(0, 1, 0)
	if math.Abs(z.y) > 0.9 {
		up = v3(1, 0, 0)
	}
	x := z.cross(up).norm()
	return x, x.cross(z).norm()
}

var trigTabs = map[int][][2]float64{}

func trig(n int) [][2]float64 {
	if tb, ok := trigTabs[n]; ok {
		return tb
	}
	tb := make([][2]float64, n+1)
	for i := 0; i <= n; i++ {
		a := 2 * math.Pi * float64(i) / float64(n)
		tb[i] = [2]float64{math.Cos(a), math.Sin(a)}
	}
	trigTabs[n] = tb
	return tb
}

func cyl(b *batch, a, c vec3, r0, r1 float64, segs int, col rgba, caps bool) {
	d := c.sub(a)
	L := d.length()
	if L < 1e-6 {
		return
	}
	z := d.mul(1 / L)
	x, y := basis(z)
	defer withTex(xf{a, x, y, z})()
	if math.Max(r0, r1) < 0.015 {
		segs = min(segs, lodSegs(segs)) // tiny parts never get extra facets
	} else {
		segs = lodSegs(segs)
	}
	slope := (r0 - r1) / L
	tb := trig(segs)
	for i := 0; i < segs; i++ {
		d0 := x.mul(tb[i][0]).add(y.mul(tb[i][1]))
		d1 := x.mul(tb[i+1][0]).add(y.mul(tb[i+1][1]))
		n0, n1 := d0.add(z.mul(slope)).norm(), d1.add(z.mul(slope)).norm()
		b.quadS(a.add(d0.mul(r0)), a.add(d1.mul(r0)), c.add(d1.mul(r1)), c.add(d0.mul(r1)), n0, n1, n1, n0, col)
		if caps {
			b.triS(c, c.add(d0.mul(r1)), c.add(d1.mul(r1)), z, col)
			b.triS(a, a.add(d1.mul(r0)), a.add(d0.mul(r0)), z.mul(-1), col)
		}
	}
}

// lodSegs: scale a tessellation count by the current level of detail
func lodSegs(n int) int {
	if segMul == 1 {
		return n
	}
	m := int(math.Round(float64(n) * segMul))
	if m < 3 {
		m = 3
	}
	return m
}

// unit sphere / hemisphere direction grids, computed once per tessellation
var sphTabs = map[[3]int][]vec3{}

func sphTab(lat, lon int, hemi bool) []vec3 {
	k := [3]int{lat, lon, 0}
	if hemi {
		k[2] = 1
	}
	if t, ok := sphTabs[k]; ok {
		return t
	}
	t := make([]vec3, (lat+1)*(lon+1))
	for i := 0; i <= lat; i++ {
		a := math.Pi * (float64(i)/float64(lat) - 0.5)
		if hemi {
			a = math.Pi / 2 * float64(i) / float64(lat)
		}
		for j := 0; j <= lon; j++ {
			bb := 2 * math.Pi * float64(j) / float64(lon)
			t[i*(lon+1)+j] = vec3{math.Cos(a) * math.Cos(bb), math.Sin(a), math.Cos(a) * math.Sin(bb)}
		}
	}
	sphTabs[k] = t
	return t
}

func sphere(b *batch, p vec3, r float64, col rgba, lat, lon int) {
	lat, lon = lodSegs(lat), lodSegs(lon)
	defer withTex(xf{p, vec3{1, 0, 0}, vec3{0, 1, 0}, vec3{0, 0, 1}})()
	tb := sphTab(lat, lon, false)
	w := lon + 1
	for i := 0; i < lat; i++ {
		for j := 0; j < lon; j++ {
			n00, n01, n11, n10 := tb[i*w+j], tb[i*w+j+1], tb[(i+1)*w+j+1], tb[(i+1)*w+j]
			b.quadS(p.add(n00.mul(r)), p.add(n01.mul(r)), p.add(n11.mul(r)), p.add(n10.mul(r)), n00, n01, n11, n10, col)
		}
	}
}

func hemi(b *batch, p, up vec3, r float64, col rgba, lat, lon int) {
	x, y := basis(up)
	defer withTex(xf{p, x, y, up})()
	lat, lon = lodSegs(lat), lodSegs(lon)
	tb := sphTab(lat, lon, true)
	w := lon + 1
	// table (cos a cos b, sin a, cos a sin b) -> x*(cos a cos b) + y*(cos a sin b) + up*(sin a)
	n := func(q vec3) vec3 { return x.mul(q.x).add(y.mul(q.z)).add(up.mul(q.y)) }
	for i := 0; i < lat; i++ {
		for j := 0; j < lon; j++ {
			n00, n01, n11, n10 := n(tb[i*w+j]), n(tb[i*w+j+1]), n(tb[(i+1)*w+j+1]), n(tb[(i+1)*w+j])
			b.quadS(p.add(n00.mul(r)), p.add(n01.mul(r)), p.add(n11.mul(r)), p.add(n10.mul(r)), n00, n01, n11, n10, col)
		}
	}
}

type xf struct{ o, x, y, z vec3 }

func (f xf) p(a, b, c float64) vec3 { return f.o.add(f.x.mul(a)).add(f.y.mul(b)).add(f.z.mul(c)) }
func (f xf) d(a, b, c float64) vec3 { return f.x.mul(a).add(f.y.mul(b)).add(f.z.mul(c)).norm() }

func frameAt(o, fwd vec3) xf {
	fwd = fwd.norm()
	s := v3(-fwd.z, 0, fwd.x).norm()
	return xf{o, s, v3(0, 1, 0), fwd}
}

// level of detail switches for things drawn far away (robots in a crowd): plain boxes
// instead of chamfered ones, and no rivets or circuit chips
var (
	lodPlainBox bool
	lodNoTrim   bool
)

func bevelBox(b *batch, c, ax, ay, az vec3, hx, hy, hz, ch float64, col rgba) {
	if lodPlainBox {
		defer withTex(xf{c, ax, ay, az})()
		fastBox(b, xf{c, ax, ay, az}, 0, 0, 0, hx, hy, hz, col)
		return
	}
	A := [3]vec3{ax, ay, az}
	H := [3]float64{hx, hy, hz}
	ch = math.Min(ch, 0.45*math.Min(hx, math.Min(hy, hz)))
	defer withTex(xf{c, ax, ay, az})()
	pt := func(v [3]float64) vec3 { return c.add(A[0].mul(v[0])).add(A[1].mul(v[1])).add(A[2].mul(v[2])) }
	for i := 0; i < 3; i++ {
		j, k := (i+1)%3, (i+2)%3
		for _, s := range []float64{-1, 1} {
			var q [4]vec3
			for m, uv := range [][2]float64{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
				var v [3]float64
				v[i], v[j], v[k] = s*H[i], uv[0]*(H[j]-ch), uv[1]*(H[k]-ch)
				q[m] = pt(v)
			}
			b.quadF(q[0], q[1], q[2], q[3], A[i].mul(s), col)
		}
	}
	for i := 0; i < 3; i++ {
		j, k := (i+1)%3, (i+2)%3
		for _, si := range []float64{-1, 1} {
			for _, sj := range []float64{-1, 1} {
				var v0, v1, v2, v3_ [3]float64
				v0[i], v0[j], v0[k] = si*H[i], sj*(H[j]-ch), -(H[k] - ch)
				v1[i], v1[j], v1[k] = si*H[i], sj*(H[j]-ch), H[k]-ch
				v2[i], v2[j], v2[k] = si*(H[i]-ch), sj*H[j], H[k]-ch
				v3_[i], v3_[j], v3_[k] = si*(H[i]-ch), sj*H[j], -(H[k] - ch)
				n := A[i].mul(si).add(A[j].mul(sj)).norm()
				b.quadF(pt(v0), pt(v1), pt(v2), pt(v3_), n, col)
			}
		}
	}
	for _, sx := range []float64{-1, 1} {
		for _, sy := range []float64{-1, 1} {
			for _, sz := range []float64{-1, 1} {
				s := [3]float64{sx, sy, sz}
				var q [3]vec3
				for i := 0; i < 3; i++ {
					var v [3]float64
					for k := 0; k < 3; k++ {
						v[k] = s[k] * (H[k] - ch)
					}
					v[i] = s[i] * H[i]
					q[i] = pt(v)
				}
				n := A[0].mul(sx).add(A[1].mul(sy)).add(A[2].mul(sz)).norm()
				b.triS(q[0], q[1], q[2], n, col)
			}
		}
	}
}

func bbox(b *batch, f xf, cx, cy, cz, hx, hy, hz, ch float64, col rgba) {
	bevelBox(b, f.p(cx, cy, cz), f.x, f.y, f.z, hx, hy, hz, ch, col)
}

func bezier(a, b, c, d vec3, t float64) vec3 {
	u := 1 - t
	return a.mul(u * u * u).add(b.mul(3 * u * u * t)).add(c.mul(3 * u * t * t)).add(d.mul(t * t * t))
}

func hose(bt *batch, a, ca, cb, b vec3, r float64, col rgba, n int) {
	sm := curMat
	curMat = mRubber
	defer func() { curMat = sm }()
	prev := a
	for k := 1; k <= n; k++ {
		p := bezier(a, ca, cb, b, float64(k)/float64(n))
		cyl(bt, prev, p, r, r, 8, col, false)
		prev = p
	}
}

func rod(a, b vec3, r float64, col rgba) {
	cyl(&bEnv, a, b, r, r, 12, col, true)
}

func ball(p vec3, r float64, col rgba) {
	switch {
	case r < 0.015:
		save := segMul
		segMul = math.Min(segMul, 1)
		sphere(&bEnv, p, r, col, 4, 6) // a pin head: a few facets is all it ever shows
		segMul = save
	case r < 0.04:
		sphere(&bEnv, p, r, col, 5, 8)
	default:
		sphere(&bEnv, p, r, col, 6, 10)
	}
}

func sprite(p vec3, s float64, col rgba) {
	r, u := camR.mul(s), camU.mul(s)
	bSoft.quad(p.sub(r).sub(u), p.add(r).sub(u), p.add(r).add(u), p.sub(r).add(u), uvSprite, col, col, col, col)
}

func ribbonC(a, b vec3, w float64, ca, cb rgba) {
	d := b.sub(a)
	mid := a.add(d.mul(0.5))
	side := d.cross(camPos.sub(mid))
	l := side.length()
	if l < 1e-9 {
		return
	}
	side = side.mul(w / 2 / l)
	bSoft.quad(a.sub(side), a.add(side), b.add(side), b.sub(side), uvRibbon, ca, ca, cb, cb)
}

func ribbon(a, b vec3, w float64, col rgba) { ribbonC(a, b, w, col, col) }

func shadowBlob(p pt2, r float64, a float32) {
	c := rgba{0, 0, 0, a}
	y := 0.0025
	bShadow.quad(v3(p[0]-r, y, p[1]-r), v3(p[0]+r, y, p[1]-r), v3(p[0]+r, y, p[1]+r), v3(p[0]-r, y, p[1]+r), uvSprite, c, c, c, c)
}

func floorGlow(p pt2, r float64, col rgba) {
	y := 0.003
	bSoft.quad(v3(p[0]-r, y, p[1]-r), v3(p[0]+r, y, p[1]-r), v3(p[0]+r, y, p[1]+r), v3(p[0]-r, y, p[1]+r), uvSprite, col, col, col, col)
}

func glowTexture(n int) []byte {
	px := make([]byte, n*n*4)
	e4 := math.Exp(-4)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			dx := (float64(x)+0.5)/float64(n)*2 - 1
			dy := (float64(y)+0.5)/float64(n)*2 - 1
			r2 := dx*dx + dy*dy
			a := 0.0
			if r2 < 1 {
				a = 0.65*math.Max(0, (math.Exp(-4*r2)-e4)/(1-e4)) + 0.35*math.Exp(-28*r2)
			}
			i := (y*n + x) * 4
			px[i], px[i+1], px[i+2], px[i+3] = 255, 255, 255, byte(math.Min(255, a*255))
		}
	}
	return px
}

var (
	bEnv, bEnvL, bFloor, bFloorL, bGlowTri, bGlowLine, bGlowWide batch
	bStatic, bStaticL, bFloorS, bFloorQ                          batch
	floorFrame                                                   = xf{vec3{}, vec3{1, 0, 0}, vec3{0, 1, 0}, vec3{0, 0, 1}}
	frameDT                                                      = 1.0 / 90
	fxStart                                                      = time.Now()
	cEnergy                                                      = rgba{0.30, 1.00, 0.45, 1}
	cEnergyHot                                                   = rgba{0.75, 1.00, 0.80, 1}
	cBeam                                                        = rgba{0.35, 0.85, 1.00, 1}
	cSpark                                                       = rgba{1.00, 0.70, 0.25, 1}
	cWallPanel                                                   = rgba{0.10, 0.13, 0.17, 1}
	cWallDk                                                      = rgba{0.06, 0.08, 0.11, 1}
	cLight                                                       = rgba{0.85, 0.95, 1.00, 1}
	cMetal                                                       = rgba{0.55, 0.60, 0.66, 1}
	cMetalDk                                                     = rgba{0.22, 0.25, 0.29, 1}
)

// simClockOn: the preview drives time itself (deterministic screenshots)
var (
	simClockOn bool
	simClock   float64
)

func fxTime() float64 {
	if simClockOn {
		return simClock
	}
	return time.Since(fxStart).Seconds()
}
