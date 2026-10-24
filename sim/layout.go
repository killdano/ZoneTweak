package sim

import (
	"math"
	"sort"
)

// genInset: generators stand this far inside the real boundary, so every handle on them
// is reachable without leaning out over the wall
const genInset = 0.12

// Corners: the room's main corners (small kinks in a hand-traced boundary dropped).
func Corners(boundary []V2, minSpan float64) []V2 {
	C := Simplify(boundary, 0.12)
	// merge corners closer than the shortest allowed span
	for len(C) > 3 {
		bi, bd := -1, minSpan
		for i := range C {
			if d := Dist(C[i], C[(i+1)%len(C)]); d < bd {
				bd, bi = d, i
			}
		}
		if bi < 0 {
			break
		}
		// drop the outer corner of the two (cutting it keeps the ring inside your real
		// walls); if both are outer corners, the one that loses less floor
		j := (bi + 1) % len(C)
		ai := triArea(C[(bi-1+len(C))%len(C)], C[bi], C[j])
		aj := triArea(C[bi], C[j], C[(j+1)%len(C)])
		ci, cj := convexAt(C, bi), convexAt(C, j)
		if ci != cj {
			ai, aj = 0, 0
			if ci {
				aj = 1
			} else {
				ai = 1
			}
		}
		if ai < aj {
			C = append(C[:bi], C[bi+1:]...)
		} else {
			C = append(C[:j], C[j+1:]...)
		}
	}
	return C
}

func triArea(a, b, c V2) float64 { return math.Abs(b.Sub(a).Cross(c.Sub(a))) / 2 }

// convexAt: the outline turns outward at vertex i - an outer corner. Cutting one off moves
// the ring inward; cutting an inner corner would push it out past the real wall.
func convexAt(P []V2, i int) bool {
	n := len(P)
	a, b, c := P[(i-1+n)%n], P[i], P[(i+1)%n]
	cr := b.Sub(a).Cross(c.Sub(b))
	if PolyArea(P) < 0 {
		cr = -cr
	}
	return cr >= 0
}

// PlaceGens: n generator spots around the room. Corners first, so the ring hugs the real
// walls; extra generators split the longest spans evenly; with fewer generators than
// corners, the corners that cost the least floor are dropped (their beam cuts across).
func PlaceGens(boundary []V2, n int, t *Tuning) []V2 {
	pos, _ := PlaceGensGrouped(boundary, n, t)
	return pos
}

// cornerGroups: the room's corners in ring order, those closer together than the shortest
// span grouped into one machine each (nil when the outline is too small to group).
func cornerGroups(boundary []V2, t *Tuning) [][]V2 {
	C := Simplify(boundary, 0.12)
	k := len(C)
	ms := t.minSpan()
	start := -1
	for i := 0; i < k; i++ {
		if Dist(C[(i-1+k)%k], C[i]) >= ms {
			start = i
			break
		}
	}
	if start < 0 {
		return nil
	}
	var groups [][]V2
	for j := 0; j < k; j++ {
		i := (start + j) % k
		if j > 0 && Dist(C[(i-1+k)%k], C[i]) < ms {
			groups[len(groups)-1] = append(groups[len(groups)-1], C[i])
		} else {
			groups = append(groups, []V2{C[i]})
		}
	}
	if len(groups) < 3 {
		return nil
	}
	return groups
}

// PlaceGensGrouped: n generators round the room with a post on every corner of it, so the
// ring is the shape of your real room. Corners closer together than the shortest span
// share one generator - a post on each, one machine. Extra generators split the longest
// beams. Returns the posts in ring order and, for each post, the index of its machine's
// first post. With fewer generators than the room has corner groups it falls back to
// cutting corners (outer ones only, so the ring stays inside your walls).
func PlaceGensGrouped(boundary []V2, n int, t *Tuning) ([]V2, []int) {
	groups := cornerGroups(boundary, t)
	if groups == nil || n < len(groups) {
		pos := placeMerged(boundary, n, t)
		lead := make([]int, len(pos))
		for i := range lead {
			lead[i] = i
		}
		return pos, lead
	}
	m := len(groups)
	last := func(g []V2) V2 { return g[len(g)-1] }
	parts := make([]int, m)
	for i := range parts {
		parts[i] = 1
	}
	for extra := n - m; extra > 0; extra-- {
		bi, bl := 0, -1.0
		for i := range groups {
			if l := Dist(last(groups[i]), groups[(i+1)%m][0]) / float64(parts[i]); l > bl {
				bl, bi = l, i
			}
		}
		parts[bi]++
	}
	var pos []V2
	var lead []int
	for i, gr := range groups {
		l := len(pos)
		for _, p := range gr {
			pos = append(pos, p)
			lead = append(lead, l)
		}
		a, b := last(gr), groups[(i+1)%m][0]
		for k := 1; k < parts[i]; k++ {
			lead = append(lead, len(pos))
			pos = append(pos, nearestOnPoly(boundary, Lerp(a, b, float64(k)/float64(parts[i]))))
		}
	}
	return insetPts(pos, boundary, genInset), lead
}

// placeMerged: the old way, for outlines too small or with too few generators to give
// every corner a post: close corners merged, the cheapest outer corners cut.
func placeMerged(boundary []V2, n int, t *Tuning) []V2 {
	C := Corners(boundary, t.minSpan())
	for len(C) > n && len(C) > 3 {
		// only outer corners can go: the beam then cuts across inside the room
		bi, ba := -1, math.Inf(1)
		for i := range C {
			a := triArea(C[(i-1+len(C))%len(C)], C[i], C[(i+1)%len(C)])
			if a < ba && convexAt(C, i) {
				ba, bi = a, i
			}
		}
		if bi < 0 {
			break
		}
		C = append(C[:bi], C[bi+1:]...)
	}
	k := len(C)
	parts := make([]int, k)
	for i := range parts {
		parts[i] = 1
	}
	for extra := n - k; extra > 0; extra-- {
		bi, bl := 0, -1.0
		for i := range C {
			if l := Dist(C[i], C[(i+1)%k]) / float64(parts[i]); l > bl {
				bl, bi = l, i
			}
		}
		parts[bi]++
	}
	var out []V2
	for i := range C {
		a, b := C[i], C[(i+1)%k]
		for p := 0; p < parts[i]; p++ {
			q := Lerp(a, b, float64(p)/float64(parts[i]))
			if p > 0 {
				q = nearestOnPoly(boundary, q) // ride the real wall, not the simplified chord
			}
			out = append(out, q)
		}
	}
	return insetPts(out, boundary, genInset)
}

func nearestOnPoly(P []V2, q V2) V2 {
	best, bp := math.Inf(1), q
	for i := range P {
		a, b := P[i], P[(i+1)%len(P)]
		d, u := SegDist(q, a, b)
		if d < best {
			best, bp = d, Lerp(a, b, u)
		}
	}
	return bp
}

// insetPts moves each point d inward along the bisector of its neighbours (staying inside).
func insetPts(G []V2, room []V2, d float64) []V2 {
	n := len(G)
	out := make([]V2, n)
	for i := range G {
		p, a, b := G[i], G[(i-1+n)%n], G[(i+1)%n]
		e1, e2 := p.Sub(a).Norm(), b.Sub(p).Norm()
		n1, n2 := e1.Perp(), e2.Perp()
		nm := n1.Add(n2).Norm()
		if nm.Len() < 0.5 {
			nm = n2
		}
		q := p.Add(nm.Mul(d))
		if !InPoly(room, q) {
			q = p.Sub(nm.Mul(d))
		}
		if !InPoly(room, q) {
			q = p
		}
		out[i] = q
	}
	return out
}

// GenRange: the generator counts a room allows (no span longer than the maximum, none
// shorter than the minimum); hi < lo never happens, the range just collapses to one count.
func GenRange(boundary []V2, t *Tuning) (lo, hi int) {
	lo, hi = -1, -1
	for n := t.MinGens; n <= t.MaxGens; n++ {
		G, L := PlaceGensGrouped(boundary, n, t)
		mx, mn := 0.0, math.Inf(1)
		machines := 0
		for i := range G {
			if L[i] == i {
				machines++
			}
			j := (i + 1) % len(G)
			if L[i] == L[j] {
				continue // two posts of one generator
			}
			d := Dist(G[i], G[j])
			mx, mn = math.Max(mx, d), math.Min(mn, d)
		}
		if mx <= t.maxSpan()+0.05 && lo < 0 {
			lo = n
		}
		if mn >= t.minSpan()-0.05 && machines == n {
			hi = n
		}
	}
	if lo < 0 {
		lo = t.MaxGens
	}
	// a post on every corner, so the ring is the shape of your real room
	if g := cornerGroups(boundary, t); lo < len(g) {
		lo = min(len(g), t.MaxGens)
	}
	if hi < lo {
		hi = lo
	}
	return lo, hi
}

// Door: where wardens and drones come in (and where a stolen device goes out).
type Door struct {
	Name  string
	Mouth V2 // just inside the hall wall
	Out   V2 // unit vector out through the door
	Wall  V2 // the doorway in the wall itself
}

// Hall: the prison hall around your room, in its own frame (X along the room's main walls).
type Hall struct {
	F       Frame   // origin at the hall's corner
	W, H    float64 // size along F.X and F.Z
	RoomBox [4]float64
	Theta   float64
}

func (h Hall) Inside(p V2, margin float64) bool {
	q := h.F.To(p)
	return q.X >= margin && q.X <= h.W-margin && q.Z >= margin && q.Z <= h.H-margin
}

func (h Hall) WallDist(p V2) float64 {
	q := h.F.To(p)
	return math.Min(math.Min(q.X, h.W-q.X), math.Min(q.Z, h.H-q.Z))
}

// Corners of the hall, counter-clockwise in the hall frame.
func (h Hall) Corners() []V2 {
	return []V2{h.F.From(V(0, 0)), h.F.From(V(h.W, 0)), h.F.From(V(h.W, h.H)), h.F.From(V(0, h.H))}
}

// MakeHall builds the hall around a room: the room's main direction sets the walls, and
// there's margin metres of open floor on every side. Doors sit in the middle of each wall:
// NORTH on the -Z side (the way you faced when you set your boundary up), then EAST, SOUTH, WEST.
func MakeHall(boundary []V2, margin float64, names []string) (Hall, []Door) {
	th := DomAngle(boundary)
	// pick the 90-degree turn of the frame whose -Z wall faces stage -Z, so NORTH is ahead
	best, bth := -2.0, th
	for k := 0; k < 4; k++ {
		a := th + float64(k)*math.Pi/2
		f := FrameAt(V(0, 0), a)
		if d := f.Z.Dot(V(0, 1)); d > best {
			best, bth = d, a
		}
	}
	th = bth
	f0 := FrameAt(V(0, 0), th)
	x0, x1, z0, z1 := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	for _, p := range boundary {
		q := f0.To(p)
		x0, x1 = math.Min(x0, q.X), math.Max(x1, q.X)
		z0, z1 = math.Min(z0, q.Z), math.Max(z1, q.Z)
	}
	o := f0.From(V(x0-margin, z0-margin))
	h := Hall{F: Frame{o, f0.X, f0.Z}, W: x1 - x0 + 2*margin, H: z1 - z0 + 2*margin, Theta: th}
	h.RoomBox = [4]float64{margin, margin + x1 - x0, margin, margin + z1 - z0}
	name := func(i int) string {
		if i < len(names) {
			return names[i]
		}
		return string(rune('A' + i))
	}
	// room centre in hall coordinates, so doors line up with the room, not the hall's middle
	cx, cz := margin+(x1-x0)/2, margin+(z1-z0)/2
	mk := func(i int, wall, out V2) Door {
		return Door{Name: name(i), Wall: h.F.From(wall), Out: h.F.X.Mul(out.X).Add(h.F.Z.Mul(out.Z)),
			Mouth: h.F.From(wall.Sub(out.Mul(0.7)))}
	}
	doors := []Door{
		mk(0, V(cx, 0), V(0, -1)),
		mk(1, V(h.W, cz), V(1, 0)),
		mk(2, V(cx, h.H), V(0, 1)),
		mk(3, V(0, cz), V(-1, 0)),
	}
	return h, doors
}

// sortedKeys: deterministic iteration over int-keyed maps
func sortedKeys[T any](m map[int]T) []int {
	ks := make([]int, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Ints(ks)
	return ks
}
