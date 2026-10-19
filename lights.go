package main

import (
	"math"
	"sort"
)

type glLight struct {
	kind   int
	a, b   vec3
	col    [3]float64
	rad, h float64
	pri    float64
	score  float64
}

type spotLight struct {
	pos, dir      vec3
	col           [3]float64
	cosOut, cosIn float64
	rng           float64
	vp            mat4
}

var (
	frameLights []glLight
	spots       []spotLight
)

const maxLights = 48

func lin(c rgba, k float64) [3]float64 {
	return [3]float64{math.Pow(float64(c.r), 2.2) * k, math.Pow(float64(c.g), 2.2) * k, math.Pow(float64(c.b), 2.2) * k}
}

func addPoint(p vec3, c rgba, k, rad float64) {
	frameLights = append(frameLights, glLight{kind: 0, a: p, b: p, col: lin(c, k), rad: rad})
}

func addLine(a, b vec3, c rgba, k, rad float64) {
	frameLights = append(frameLights, glLight{kind: 1, a: a, b: b, col: lin(c, k), rad: rad})
}

func addRect(a, b pt2, y0, h float64, c rgba, k, rad float64) {
	frameLights = append(frameLights, glLight{kind: 2, a: vec3{a[0], y0, a[1]}, b: vec3{b[0], y0, b[1]}, col: lin(c, k), rad: rad, h: h, pri: 1})
}

func lightDistTo(l glLight, p vec3) float64 {
	q := l.a
	if l.kind >= 1 {
		ab := l.b.sub(l.a)
		t := 0.0
		if dd := ab.dot(ab); dd > 1e-9 {
			t = math.Max(0, math.Min(1, p.sub(l.a).dot(ab)/dd))
		}
		q = l.a.add(ab.mul(t))
		if l.kind == 2 {
			q.y = math.Max(l.a.y, math.Min(l.a.y+l.h, p.y))
		}
	}
	return q.sub(p).length()
}

// packLights sorts by importance to the viewer and packs 3 vec4 per light
func packLights(cam vec3, out []float32) ([]float32, int) {
	for i := range frameLights {
		l := &frameLights[i]
		d := lightDistTo(*l, cam)
		e := l.col[0] + l.col[1] + l.col[2]
		size := 1.0
		if l.kind >= 1 {
			size = 1 + l.b.sub(l.a).length() + l.h
		}
		l.score = l.pri*1000 + e*size/(d*d+1)
		if d > l.rad+6 {
			l.score -= 2000
		}
	}
	sort.SliceStable(frameLights, func(i, j int) bool { return frameLights[i].score > frameLights[j].score })
	n := len(frameLights)
	if n > maxLights {
		n = maxLights
	}
	out = out[:0]
	for i := 0; i < n; i++ {
		l := frameLights[i]
		out = append(out,
			float32(l.a.x), float32(l.a.y), float32(l.a.z), float32(l.kind),
			float32(l.b.x), float32(l.b.y), float32(l.b.z), float32(l.rad),
			float32(l.col[0]), float32(l.col[1]), float32(l.col[2]), float32(l.h))
	}
	for len(out) < maxLights*12 {
		out = append(out, 0)
	}
	return out, n
}

func makeSpot(pos, dir vec3, c rgba, k, outerDeg, innerDeg, rng float64) spotLight {
	up := vec3{1, 0, 0}
	if math.Abs(dir.x) > 0.9 {
		up = vec3{0, 0, 1}
	}
	view := lookAt4(pos, pos.add(dir), up)
	proj := perspective4(outerDeg*2*math.Pi/180*1.05, 1, 0.1, rng)
	return spotLight{pos: pos, dir: dir.norm(), col: lin(c, k), cosOut: math.Cos(outerDeg * math.Pi / 180), cosIn: math.Cos(innerDeg * math.Pi / 180), rng: rng, vp: mul4(proj, view)}
}

type fogVol struct {
	c          vec3
	r, k, yStr float64
}

var fogVols []fogVol

// potatoMode mirrors the POTATO quality for the scene code (fewer particles, simpler FX)
var potatoMode bool

func addFog(c vec3, r, k, yStr float64) {
	if potatoMode {
		return
	}
	if len(fogVols) < 24 {
		fogVols = append(fogVols, fogVol{c, r, k, yStr})
	}
}

func packFog() ([]float32, []float32, int) {
	a := make([]float32, 0, 96)
	b := make([]float32, 0, 96)
	for _, f := range fogVols {
		a = append(a, float32(f.c.x), float32(f.c.y), float32(f.c.z), float32(f.r))
		b = append(b, float32(f.k), float32(f.yStr), 0, 0)
	}
	n := len(fogVols)
	for len(a) < 96 {
		a = append(a, 0)
		b = append(b, 0)
	}
	return a, b, n
}
