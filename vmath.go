package main

import "math"

type vec3 struct{ x, y, z float64 }
type rgba struct{ r, g, b, a float32 }
type mat4 [16]float32
type pt2 [2]float64

func v3(x, y, z float64) vec3     { return vec3{x, y, z} }
func (a vec3) add(b vec3) vec3    { return vec3{a.x + b.x, a.y + b.y, a.z + b.z} }
func (a vec3) sub(b vec3) vec3    { return vec3{a.x - b.x, a.y - b.y, a.z - b.z} }
func (a vec3) mul(s float64) vec3 { return vec3{a.x * s, a.y * s, a.z * s} }
func (a vec3) dot(b vec3) float64 { return a.x*b.x + a.y*b.y + a.z*b.z }
func (a vec3) cross(b vec3) vec3 {
	return vec3{a.y*b.z - a.z*b.y, a.z*b.x - a.x*b.z, a.x*b.y - a.y*b.x}
}
func (a vec3) length() float64 { return math.Sqrt(a.dot(a)) }
func (a vec3) norm() vec3 {
	l := a.length()
	if l < 1e-9 {
		return a
	}
	return a.mul(1 / l)
}
func fl(p pt2, y float64) vec3      { return vec3{p[0], y, p[1]} }
func (c rgba) alpha(a float32) rgba { c.a = a; return c }

type hmdMatrix34 [3][4]float32

func (m *hmdMatrix34) pos() vec3 { return vec3{float64(m[0][3]), float64(m[1][3]), float64(m[2][3])} }
func (m *hmdMatrix34) axis(i int) vec3 {
	return vec3{float64(m[0][i]), float64(m[1][i]), float64(m[2][i])}
}
func (m *hmdMatrix34) xform(p vec3) vec3 {
	return m.axis(0).mul(p.x).add(m.axis(1).mul(p.y)).add(m.axis(2).mul(p.z)).add(m.pos())
}

func mul34(a, b hmdMatrix34) hmdMatrix34 {
	var r hmdMatrix34
	for i := 0; i < 3; i++ {
		for j := 0; j < 4; j++ {
			var s float32
			for k := 0; k < 3; k++ {
				s += a[i][k] * b[k][j]
			}
			if j == 3 {
				s += a[i][3]
			}
			r[i][j] = s
		}
	}
	return r
}

func inv34(m hmdMatrix34) hmdMatrix34 {
	var r hmdMatrix34
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			r[i][j] = m[j][i]
		}
	}
	for i := 0; i < 3; i++ {
		r[i][3] = -(r[i][0]*m[0][3] + r[i][1]*m[1][3] + r[i][2]*m[2][3])
	}
	return r
}

func mat4From34(m hmdMatrix34) mat4 {
	return mat4{
		m[0][0], m[1][0], m[2][0], 0,
		m[0][1], m[1][1], m[2][1], 0,
		m[0][2], m[1][2], m[2][2], 0,
		m[0][3], m[1][3], m[2][3], 1,
	}
}

func projFromRaw(l, r, t, b, n, f float32) mat4 {
	tg, bg := b, t
	var m mat4
	m[0] = 2 / (r - l)
	m[5] = 2 / (tg - bg)
	m[8] = (r + l) / (r - l)
	m[9] = (tg + bg) / (tg - bg)
	m[10] = -(f + n) / (f - n)
	m[11] = -1
	m[14] = -2 * f * n / (f - n)
	return m
}

func projFromFov(f xrFov, n, far float32) mat4 {
	l, r := float32(math.Tan(float64(f.l))), float32(math.Tan(float64(f.r)))
	u, d := float32(math.Tan(float64(f.u))), float32(math.Tan(float64(f.d)))
	var m mat4
	m[0] = 2 / (r - l)
	m[5] = 2 / (u - d)
	m[8] = (r + l) / (r - l)
	m[9] = (u + d) / (u - d)
	m[10] = -(far + n) / (far - n)
	m[11] = -1
	m[14] = -2 * far * n / (far - n)
	return m
}
