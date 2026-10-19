package main

import "math"

// column-major 4x4 helpers (m[col*4+row])

func mul4(a, b mat4) mat4 {
	var r mat4
	for c := 0; c < 4; c++ {
		for rr := 0; rr < 4; rr++ {
			var s float32
			for k := 0; k < 4; k++ {
				s += a[k*4+rr] * b[c*4+k]
			}
			r[c*4+rr] = s
		}
	}
	return r
}

func inv4(m mat4) mat4 {
	var a [4][8]float64
	for r := 0; r < 4; r++ {
		for c := 0; c < 4; c++ {
			a[r][c] = float64(m[c*4+r])
		}
		a[r][4+r] = 1
	}
	for c := 0; c < 4; c++ {
		p := c
		for r := c + 1; r < 4; r++ {
			if math.Abs(a[r][c]) > math.Abs(a[p][c]) {
				p = r
			}
		}
		a[c], a[p] = a[p], a[c]
		d := a[c][c]
		if math.Abs(d) < 1e-12 {
			return mat4{}
		}
		for k := 0; k < 8; k++ {
			a[c][k] /= d
		}
		for r := 0; r < 4; r++ {
			if r == c {
				continue
			}
			f := a[r][c]
			for k := 0; k < 8; k++ {
				a[r][k] -= f * a[c][k]
			}
		}
	}
	var out mat4
	for r := 0; r < 4; r++ {
		for c := 0; c < 4; c++ {
			out[c*4+r] = float32(a[r][4+c])
		}
	}
	return out
}

func lookAt4(eye, target, up vec3) mat4 {
	f := target.sub(eye).norm()
	s := f.cross(up).norm()
	u := s.cross(f)
	return mat4{
		float32(s.x), float32(u.x), float32(-f.x), 0,
		float32(s.y), float32(u.y), float32(-f.y), 0,
		float32(s.z), float32(u.z), float32(-f.z), 0,
		float32(-s.dot(eye)), float32(-u.dot(eye)), float32(f.dot(eye)), 1,
	}
}

func perspective4(fovy, aspect, n, f float64) mat4 {
	t := 1 / math.Tan(fovy/2)
	var m mat4
	m[0] = float32(t / aspect)
	m[5] = float32(t)
	m[10] = float32(-(f + n) / (f - n))
	m[11] = -1
	m[14] = float32(-2 * f * n / (f - n))
	return m
}

func xform4(m mat4, p vec3) vec3 {
	x := float64(m[0])*p.x + float64(m[4])*p.y + float64(m[8])*p.z + float64(m[12])
	y := float64(m[1])*p.x + float64(m[5])*p.y + float64(m[9])*p.z + float64(m[13])
	z := float64(m[2])*p.x + float64(m[6])*p.y + float64(m[10])*p.z + float64(m[14])
	w := float64(m[3])*p.x + float64(m[7])*p.y + float64(m[11])*p.z + float64(m[15])
	return vec3{x / w, y / w, z / w}
}
