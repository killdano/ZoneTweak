package main

import (
	"math"
	"sort"
)

// ---- frustum culling ----

type aabb struct{ mn, mx [3]float32 }

func emptyBox() aabb {
	f := float32(math.MaxFloat32)
	return aabb{[3]float32{f, f, f}, [3]float32{-f, -f, -f}}
}

func (b *aabb) add(x, y, z float32) {
	p := [3]float32{x, y, z}
	for i := 0; i < 3; i++ {
		b.mn[i] = min(b.mn[i], p[i])
		b.mx[i] = max(b.mx[i], p[i])
	}
}

// boxOf: bounds of a vertex array laid out vstride floats per vertex
func boxOf(v []float32) aabb {
	b := emptyBox()
	for i := 0; i+2 < len(v); i += vstride {
		b.add(v[i], v[i+1], v[i+2])
	}
	return b
}

// boxVisible: false only when the box is entirely outside one of the frustum planes of vp
func boxVisible(vp *mat4, b aabb) bool {
	if b.mn[0] > b.mx[0] {
		return false
	}
	var out [6]int
	for k := 0; k < 8; k++ {
		x, y, z := b.mn[0], b.mn[1], b.mn[2]
		if k&1 != 0 {
			x = b.mx[0]
		}
		if k&2 != 0 {
			y = b.mx[1]
		}
		if k&4 != 0 {
			z = b.mx[2]
		}
		cx := vp[0]*x + vp[4]*y + vp[8]*z + vp[12]
		cy := vp[1]*x + vp[5]*y + vp[9]*z + vp[13]
		cz := vp[2]*x + vp[6]*y + vp[10]*z + vp[14]
		cw := vp[3]*x + vp[7]*y + vp[11]*z + vp[15]
		if cx < -cw {
			out[0]++
		}
		if cx > cw {
			out[1]++
		}
		if cy < -cw {
			out[2]++
		}
		if cy > cw {
			out[3]++
		}
		if cz < -cw {
			out[4]++
		}
		if cz > cw {
			out[5]++
		}
	}
	for _, n := range out {
		if n == 8 {
			return false
		}
	}
	return true
}

// a run of whole triangles in a batch, with its bounds
type chunk struct {
	first, count int // in vertices
	box          aabb
}

// chunkify reorders a triangle batch so triangles in the same cell of a cell-sized XZ grid
// sit together, and returns one chunk per occupied cell
func chunkify(v []float32, cell float32) []chunk {
	tri := vstride * 3
	n := len(v) / tri
	if n == 0 {
		return nil
	}
	type key struct{ x, z int32 }
	keys := make([]key, n)
	order := make([]int, n)
	for t := 0; t < n; t++ {
		o := t * tri
		cx := (v[o] + v[o+vstride] + v[o+2*vstride]) / 3
		cz := (v[o+2] + v[o+vstride+2] + v[o+2*vstride+2]) / 3
		keys[t] = key{int32(math.Floor(float64(cx / cell))), int32(math.Floor(float64(cz / cell)))}
		order[t] = t
	}
	sort.SliceStable(order, func(a, b int) bool {
		ka, kb := keys[order[a]], keys[order[b]]
		if ka.x != kb.x {
			return ka.x < kb.x
		}
		return ka.z < kb.z
	})
	src := make([]float32, n*tri)
	copy(src, v[:n*tri])
	var out []chunk
	for i, t := range order {
		copy(v[i*tri:(i+1)*tri], src[t*tri:(t+1)*tri])
		if i == 0 || keys[t] != keys[order[i-1]] {
			out = append(out, chunk{first: i * 3, box: emptyBox()})
		}
		c := &out[len(out)-1]
		c.count += 3
		for k := 0; k < 3; k++ {
			o := i*tri + k*vstride
			c.box.add(v[o], v[o+1], v[o+2])
		}
	}
	return out
}

// visibleRuns: the visible chunks merged into as few contiguous draws as possible
func visibleRuns(vp *mat4, cs []chunk, out [][2]int) [][2]int {
	out = out[:0]
	for _, c := range cs {
		if !boxVisible(vp, c.box) {
			continue
		}
		if k := len(out) - 1; k >= 0 && out[k][0]+out[k][1] == c.first {
			out[k][1] += c.count
			continue
		}
		out = append(out, [2]int{c.first, c.count})
	}
	return out
}

// ---- tiled light lists ----

const (
	tilePx    = 32
	tileIdxW  = 1024 // width of the light-index texture
	lightNear = 0.05
)

type lightTiles struct {
	tx, ty int
	hdr    []uint32 // per tile: offset, count into idx
	idx    []uint8
	cnt    []uint8
	rects  [][4]int
}

// build: which of the n packed lights (12 floats each, as packLights writes them) can reach
// each 32 px tile of a w x h view, from the screen rectangle of each light's bounding box.
// Returns false (and builds nothing) when the lights cover so much of the view that the
// lists would hardly cull anything: then the plain loop over all lights is cheaper.
func (lt *lightTiles) build(vp *mat4, lights []float32, n int, w, h int) bool {
	lt.tx, lt.ty = (w+tilePx-1)/tilePx, (h+tilePx-1)/tilePx
	nt := lt.tx * lt.ty
	lt.rects = lt.rects[:0]
	area := 0
	for i := 0; i < n; i++ {
		L := lights[i*12 : i*12+12]
		kind := int(L[3] + 0.5)
		rad := L[7]
		b := emptyBox()
		b.add(L[0], L[1], L[2])
		if kind >= 1 {
			b.add(L[4], L[5], L[6])
		}
		if kind == 2 {
			b.add(L[0], L[1]+L[11], L[2])
		}
		for k := 0; k < 3; k++ {
			b.mn[k] -= rad
			b.mx[k] += rad
		}
		r, ok := screenRect(vp, b, w, h)
		if !ok {
			lt.rects = append(lt.rects, [4]int{0, 0, -1, -1})
			continue
		}
		r = [4]int{r[0] / tilePx, r[1] / tilePx, r[2] / tilePx, r[3] / tilePx}
		lt.rects = append(lt.rects, r)
		area += (r[2] - r[0] + 1) * (r[3] - r[1] + 1)
	}
	if n == 0 || float64(area) > 0.7*float64(nt*n) {
		return false
	}
	if cap(lt.cnt) < nt {
		lt.cnt = make([]uint8, nt)
		lt.hdr = make([]uint32, nt*2)
	}
	lt.cnt, lt.hdr = lt.cnt[:nt], lt.hdr[:nt*2]
	for i := range lt.cnt {
		lt.cnt[i] = 0
	}
	for _, r := range lt.rects {
		for y := r[1]; y <= r[3]; y++ {
			for x := r[0]; x <= r[2]; x++ {
				lt.cnt[y*lt.tx+x]++
			}
		}
	}
	off := 0
	for t := 0; t < nt; t++ {
		lt.hdr[t*2], lt.hdr[t*2+1] = uint32(off), 0
		off += int(lt.cnt[t])
	}
	rows := (off + tileIdxW - 1) / tileIdxW
	if rows < 1 {
		rows = 1
	}
	if cap(lt.idx) < rows*tileIdxW {
		lt.idx = make([]uint8, rows*tileIdxW)
	}
	lt.idx = lt.idx[:rows*tileIdxW]
	for i, r := range lt.rects {
		for y := r[1]; y <= r[3]; y++ {
			for x := r[0]; x <= r[2]; x++ {
				t := y*lt.tx + x
				lt.idx[lt.hdr[t*2]+lt.hdr[t*2+1]] = uint8(i)
				lt.hdr[t*2+1]++
			}
		}
	}
	return true
}

// screenRect: the pixel rectangle a box covers, clamped to the view; the whole view when the
// box reaches behind the eye, false when it is off screen
func screenRect(vp *mat4, b aabb, w, h int) ([4]int, bool) {
	if !boxVisible(vp, b) {
		return [4]int{}, false
	}
	x0, y0, x1, y1 := float32(1), float32(1), float32(-1), float32(-1)
	for k := 0; k < 8; k++ {
		x, y, z := b.mn[0], b.mn[1], b.mn[2]
		if k&1 != 0 {
			x = b.mx[0]
		}
		if k&2 != 0 {
			y = b.mx[1]
		}
		if k&4 != 0 {
			z = b.mx[2]
		}
		cw := vp[3]*x + vp[7]*y + vp[11]*z + vp[15]
		if cw < lightNear {
			return [4]int{0, 0, w - 1, h - 1}, true
		}
		cx := (vp[0]*x + vp[4]*y + vp[8]*z + vp[12]) / cw
		cy := (vp[1]*x + vp[5]*y + vp[9]*z + vp[13]) / cw
		x0, x1 = min(x0, cx), max(x1, cx)
		y0, y1 = min(y0, cy), max(y1, cy)
	}
	px := func(v float32, s int) int {
		p := int(math.Floor(float64((v*0.5 + 0.5) * float32(s))))
		return max(0, min(s-1, p))
	}
	return [4]int{px(x0, w), px(y0, h), px(x1, w), px(y1, h)}, true
}

// ---- fixed foveation ----

// rateMap: a shading-rate palette index per tw x th texel of a w x h view: 0 full rate near
// the lens centre, 1 (2x2) further out, 2 (4x4) at the edges. Angles are measured from the
// view axis using the projection, so it follows each eye's off-centre lens.
func rateMap(proj *mat4, w, h, tw, th int, in, mid float64) ([]uint8, int, int) {
	nx, ny := (w+tw-1)/tw, (h+th-1)/th
	out := make([]uint8, nx*ny)
	// NDC of the view axis, and NDC per unit of tangent
	cx, cy := float64(-proj[8]), float64(-proj[9])
	sx, sy := float64(proj[0]), float64(proj[5])
	ti, tm := math.Tan(in*math.Pi/180), math.Tan(mid*math.Pi/180)
	for y := 0; y < ny; y++ {
		for x := 0; x < nx; x++ {
			ndx := (float64(x)+0.5)*float64(tw)/float64(w)*2 - 1
			ndy := (float64(y)+0.5)*float64(th)/float64(h)*2 - 1
			t := math.Hypot((ndx-cx)/sx, (ndy-cy)/sy)
			switch {
			case t < ti:
				out[y*nx+x] = 0
			case t < tm:
				out[y*nx+x] = 1
			default:
				out[y*nx+x] = 2
			}
		}
	}
	return out, nx, ny
}
