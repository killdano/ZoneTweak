// Package sim holds Contraband's rules: the ring of generators, the wardens and drones,
// the maze, the escape device and the harpoon. It knows nothing about rendering, audio or
// VR hardware, so it runs headless in tests and can be ported to another engine as is.
// Units are metres and seconds; the floor is the XZ plane of the VR stage space.
package sim

import "math"

// V2 is a point or direction on the floor (x, z).
type V2 struct{ X, Z float64 }

// V3 is a point or direction in space (y up).
type V3 struct{ X, Y, Z float64 }

func V(x, z float64) V2               { return V2{x, z} }
func (a V2) Add(b V2) V2              { return V2{a.X + b.X, a.Z + b.Z} }
func (a V2) Sub(b V2) V2              { return V2{a.X - b.X, a.Z - b.Z} }
func (a V2) Mul(s float64) V2         { return V2{a.X * s, a.Z * s} }
func (a V2) Dot(b V2) float64         { return a.X*b.X + a.Z*b.Z }
func (a V2) Cross(b V2) float64       { return a.X*b.Z - a.Z*b.X }
func (a V2) Len() float64             { return math.Hypot(a.X, a.Z) }
func (a V2) Perp() V2                 { return V2{-a.Z, a.X} }
func (a V2) At(y float64) V3          { return V3{a.X, y, a.Z} }
func Dist(a, b V2) float64            { return math.Hypot(a.X-b.X, a.Z-b.Z) }
func Lerp(a, b V2, t float64) V2      { return V2{a.X + (b.X-a.X)*t, a.Z + (b.Z-a.Z)*t} }
func (a V3) Add(b V3) V3              { return V3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a V3) Sub(b V3) V3              { return V3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a V3) Mul(s float64) V3         { return V3{a.X * s, a.Y * s, a.Z * s} }
func (a V3) Dot(b V3) float64         { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func (a V3) Len() float64             { return math.Sqrt(a.Dot(a)) }
func (a V3) XZ() V2                   { return V2{a.X, a.Z} }
func Lerp3(a, b V3, t float64) V3     { return a.Add(b.Sub(a).Mul(t)) }
func clamp(x, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, x)) }

func (a V2) Norm() V2 {
	l := a.Len()
	if l < 1e-12 {
		return V2{}
	}
	return a.Mul(1 / l)
}

func (a V3) Norm() V3 {
	l := a.Len()
	if l < 1e-12 {
		return V3{}
	}
	return a.Mul(1 / l)
}

// PolyArea is the signed area (positive when the points run counter-clockwise in x/z).
func PolyArea(P []V2) float64 {
	a := 0.0
	for i := range P {
		p, q := P[i], P[(i+1)%len(P)]
		a += p.X*q.Z - q.X*p.Z
	}
	return a / 2
}

func PolyPerim(P []V2) float64 {
	l := 0.0
	for i := range P {
		l += Dist(P[i], P[(i+1)%len(P)])
	}
	return l
}

func InPoly(P []V2, p V2) bool {
	c := false
	for i, j := 0, len(P)-1; i < len(P); j, i = i, i+1 {
		a, b := P[i], P[j]
		if (a.Z > p.Z) != (b.Z > p.Z) && p.X < (b.X-a.X)*(p.Z-a.Z)/(b.Z-a.Z)+a.X {
			c = !c
		}
	}
	return c
}

// SegDist: distance from p to segment ab, and where along it (0..1) the closest point is.
func SegDist(p, a, b V2) (float64, float64) {
	d := b.Sub(a)
	L := d.Dot(d)
	t := 0.0
	if L > 0 {
		t = clamp(p.Sub(a).Dot(d)/L, 0, 1)
	}
	return Dist(p, a.Add(d.Mul(t))), t
}

// SegHit: where segments ab and cd cross (t along ab, u along cd), if they do.
func SegHit(a, b, c, d V2) (t, u float64, ok bool) {
	r, s := b.Sub(a), d.Sub(c)
	den := r.Cross(s)
	if math.Abs(den) < 1e-12 {
		return 0, 0, false
	}
	q := c.Sub(a)
	t = q.Cross(s) / den
	u = q.Cross(r) / den
	return t, u, t >= 0 && t <= 1 && u >= 0 && u <= 1
}

// SegSegDist: closest distance between two segments.
func SegSegDist(a, b, c, d V2) float64 {
	if _, _, ok := SegHit(a, b, c, d); ok {
		return 0
	}
	m := math.Inf(1)
	for _, x := range [][3]V2{{a, c, d}, {b, c, d}, {c, a, b}, {d, a, b}} {
		dd, _ := SegDist(x[0], x[1], x[2])
		m = math.Min(m, dd)
	}
	return m
}

// EdgeDist: distance from p to the nearest edge of polygon P.
func EdgeDist(P []V2, p V2) float64 {
	m := math.Inf(1)
	for i := range P {
		d, _ := SegDist(p, P[i], P[(i+1)%len(P)])
		m = math.Min(m, d)
	}
	return m
}

func Centroid(P []V2) V2 {
	var cx, cz, A float64
	for i := range P {
		p, q := P[i], P[(i+1)%len(P)]
		cr := p.X*q.Z - q.X*p.Z
		A += cr
		cx += (p.X + q.X) * cr
		cz += (p.Z + q.Z) * cr
	}
	if math.Abs(A) < 1e-9 {
		var s V2
		for _, p := range P {
			s = s.Add(p)
		}
		return s.Mul(1 / float64(max(1, len(P))))
	}
	return V2{cx / (3 * A), cz / (3 * A)}
}

// DomAngle: the room's main wall direction (mod 90 degrees), weighted by wall length.
func DomAngle(P []V2) float64 {
	s, c := 0.0, 0.0
	for i := range P {
		d := P[(i+1)%len(P)].Sub(P[i])
		L2 := d.Dot(d)
		t := math.Atan2(d.Z, d.X) * 4
		s += L2 * math.Sin(t)
		c += L2 * math.Cos(t)
	}
	return math.Atan2(s, c) / 4
}

// Simplify drops points that sit within tol of the line through their neighbours.
func Simplify(P []V2, tol float64) []V2 {
	Q := append([]V2(nil), P...)
	for len(Q) > 3 {
		bi, bd := -1, tol
		for i := range Q {
			d, _ := SegDist(Q[i], Q[(i-1+len(Q))%len(Q)], Q[(i+1)%len(Q)])
			if d < bd {
				bd, bi = d, i
			}
		}
		if bi < 0 {
			break
		}
		Q = append(Q[:bi], Q[bi+1:]...)
	}
	return Q
}

// Frame is a rotated rectangle frame on the floor: origin plus unit axes.
type Frame struct{ O, X, Z V2 }

func FrameAt(o V2, theta float64) Frame {
	c, s := math.Cos(theta), math.Sin(theta)
	return Frame{o, V2{c, s}, V2{-s, c}}
}
func (f Frame) To(p V2) V2   { d := p.Sub(f.O); return V2{d.Dot(f.X), d.Dot(f.Z)} }
func (f Frame) From(q V2) V2 { return f.O.Add(f.X.Mul(q.X)).Add(f.Z.Mul(q.Z)) }

// Rot: a turned by t radians.
func (a V2) Rot(t float64) V2 {
	c, s := math.Cos(t), math.Sin(t)
	return V2{a.X*c - a.Z*s, a.X*s + a.Z*c}
}
