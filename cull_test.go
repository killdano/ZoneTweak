package main

import (
	"math"
	"testing"
)

func testVP() mat4 {
	proj := projFromFov(xrFov{l: -0.9, r: 0.8, u: 0.8, d: -0.85}, 0.05, 60)
	return proj // camera at origin looking down -z
}

func TestBoxVisible(t *testing.T) {
	vp := testVP()
	in := aabb{[3]float32{-0.1, -0.1, -2.1}, [3]float32{0.1, 0.1, -1.9}}
	behind := aabb{[3]float32{-0.1, -0.1, 1.9}, [3]float32{0.1, 0.1, 2.1}}
	side := aabb{[3]float32{50, -0.1, -2.1}, [3]float32{51, 0.1, -1.9}}
	if !boxVisible(&vp, in) || boxVisible(&vp, behind) || boxVisible(&vp, side) {
		t.Fatal("frustum test wrong")
	}
}

func TestChunkify(t *testing.T) {
	var v []float32
	tri := func(x, z float32) {
		for k := 0; k < 3; k++ {
			vv := make([]float32, vstride)
			vv[0], vv[2] = x+float32(k)*0.1, z
			vv[3] = x // tag
			v = append(v, vv...)
		}
	}
	tri(5, 5)
	tri(0, 0)
	tri(5.5, 5.2)
	tri(-3, 0)
	cs := chunkify(v, 2)
	if len(cs) != 3 {
		t.Fatalf("chunks %d", len(cs))
	}
	tot := 0
	for _, c := range cs {
		tot += c.count
	}
	if tot != 12 {
		t.Fatal("lost triangles")
	}
	// every triangle stays intact
	for i := 0; i < 4; i++ {
		o := i * 3 * vstride
		if v[o+3] != v[o+vstride+3] || v[o+3] != v[o+2*vstride+3] {
			t.Fatal("triangle torn apart")
		}
	}
}

func TestTilesMatchBrute(t *testing.T) {
	vp := testVP()
	w, h := 640, 480
	var L []float32
	add := func(x, y, z, rad float32) {
		L = append(L, x, y, z, 0, 0, 0, 0, rad, 1, 1, 1, 0)
	}
	add(0, 0, -3, 0.5)
	add(1, 0.5, -2, 0.3)
	add(0, 0, 0, 0.2)   // around the eye: everywhere
	add(0, 0, 5, 0.5)   // behind: nowhere
	add(-3, 0, -2, 0.4) // off to the side
	var lt lightTiles
	if !lt.build(&vp, L, 5, w, h) {
		t.Fatal("expected tile lists for mostly-local lights")
	}
	// brute force: for sampled pixels, any light whose sphere hits the ray segment must be listed
	inv := inv4(vp)
	for py := 0; py < h; py += 7 {
		for px := 0; px < w; px += 7 {
			t0 := (py/tilePx)*lt.tx + px/tilePx
			has := map[int]bool{}
			for k := 0; k < int(lt.hdr[t0*2+1]); k++ {
				has[int(lt.idx[int(lt.hdr[t0*2])+k])] = true
			}
			nx := (float32(px)+0.5)/float32(w)*2 - 1
			ny := (float32(py)+0.5)/float32(h)*2 - 1
			for i := 0; i < 5; i++ {
				c := v3(float64(L[i*12]), float64(L[i*12+1]), float64(L[i*12+2]))
				r := float64(L[i*12+7])
				for s := 0.0; s <= 1; s += 0.01 {
					d := float32(-1 + 2*s)
					x := inv[0]*nx + inv[4]*ny + inv[8]*d + inv[12]
					y := inv[1]*nx + inv[5]*ny + inv[9]*d + inv[13]
					z := inv[2]*nx + inv[6]*ny + inv[10]*d + inv[14]
					ww := inv[3]*nx + inv[7]*ny + inv[11]*d + inv[15]
					p := v3(float64(x/ww), float64(y/ww), float64(z/ww))
					if p.sub(c).length() < r && !has[i] {
						t.Fatalf("pixel %d,%d misses light %d", px, py, i)
					}
				}
			}
		}
	}
	if lt.rects[3][2] >= 0 {
		t.Fatal("light behind the eye was listed")
	}
}

func TestRateMap(t *testing.T) {
	p := testVP()
	m, nx, ny := rateMap(&p, 640, 480, 16, 16, 30, 45)
	cx := int((-float64(p[8])*0.5 + 0.5) * 640 / 16)
	cy := int((-float64(p[9])*0.5 + 0.5) * 480 / 16)
	if m[cy*nx+cx] != 0 || m[0] != 2 || len(m) != nx*ny {
		t.Fatal("rate map wrong", m[cy*nx+cx], m[0])
	}
	_ = math.Pi
}
