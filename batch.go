package main

import "math"

// vertex: pos3 nrm3 col4 mat4 tex3 = 17 floats.
// col.a holds the material kind for lit vertices, alpha for unlit ones.
// mat = metallic, roughness, wear, emissive (emissive > 0 or zero normal = unlit).
const vstride = 17

type batch struct{ v []float32 }

type material struct {
	kind               int
	metal, rough, wear float32
}

var (
	mPaint    = material{0, 0.15, 0.5, 0.4}
	mSteel    = material{1, 1, 0.38, 0.25}
	mChrome   = material{1, 1, 0.07, 0.0}
	mRubber   = material{2, 0, 0.75, 0.3}
	mWall     = material{3, 0.35, 0.55, 0.45}
	mCircuit  = material{4, 0.1, 0.45, 0.1}
	mGlass    = material{5, 0, 0.05, 0}
	mHazard   = material{6, 0.1, 0.6, 0.5}
	mCast     = material{7, 0.8, 0.6, 0.5}
	curMat    = mPaint
	curEmis   = float32(1)
	texF      = xf{vec3{}, vec3{1, 0, 0}, vec3{0, 1, 0}, vec3{0, 0, 1}}
	texOff    vec3
	wearScale = float32(1)
)

func useMat(m material) { curMat = m }

func (b *batch) reset() { b.v = b.v[:0] }

func (b *batch) vert(p vec3, c rgba) {
	b.v = append(b.v, float32(p.x), float32(p.y), float32(p.z), 0, 0, 0, c.r, c.g, c.b, c.a, 0, 1, 0, curEmis, 0, 0, 0)
}

type role struct {
	key, col rgba
	mat      material
}

var (
	roles  []role
	segMul = 1.0 // tessellation scale for cylinders/spheres (level of detail)
)

func (b *batch) vertL(p, n vec3, c rgba) {
	m := curMat
	for i := range roles {
		if roles[i].key == c {
			c, m = roles[i].col, roles[i].mat
			break
		}
	}
	d := p.sub(texF.o)
	t := vec3{d.dot(texF.x), d.dot(texF.y), d.dot(texF.z)}.add(texOff)
	w := m.wear * wearScale
	if w > 1 {
		w = 1
	}
	b.v = append(b.v, float32(p.x), float32(p.y), float32(p.z), float32(n.x), float32(n.y), float32(n.z),
		c.r, c.g, c.b, float32(m.kind)/16,
		m.metal, m.rough, w, 0,
		float32(t.x), float32(t.y), float32(t.z))
}

func (b *batch) line(a, c vec3, col rgba) { b.vert(a, col); b.vert(c, col) }

func (b *batch) tri(a, c, d vec3, col rgba) { b.vert(a, col); b.vert(c, col); b.vert(d, col) }

func (b *batch) quad(a, c, d, e vec3, col rgba) { b.tri(a, c, d, col); b.tri(a, d, e, col) }

func (b *batch) count() int { return len(b.v) / vstride }

var primCounter float64

// seedTex makes the procedural-texture offsets of the following primitives stable
// frame to frame (otherwise rust/scratches would re-randomise every frame)
func seedTex(x float64) { primCounter = math.Floor(x) }

// stableTex: texture offsets come from where a part is, not how many parts came before it,
// so rebuilding the cell leaves untouched parts looking exactly the same
var stableTex bool

func nextTexOff() {
	primCounter++
	seed := primCounter
	if stableTex {
		o := texF.o
		seed = math.Floor(o.x*173.3) + math.Floor(o.y*91.7)*1.37 + math.Floor(o.z*137.1)*2.11
	}
	v := math.Sin(seed*12.9898) * 43758.5453
	h := v - math.Floor(v)
	f1, f2 := h*7.13, h*3.71
	texOff = vec3{h * 31, (f1 - math.Floor(f1)) * 29, (f2 - math.Floor(f2)) * 37}
}
