package sim

import (
	"container/heap"
	"math"
)

// Nav: a walking grid over the hall. Each cell keeps its distance to three kinds of
// obstacle: the fixed ones (hall walls, generator bases), the green ring (inside it counts
// as solid), and the magenta maze. A robot of radius r can stand where all three are at
// least r away; a burning walker ignores the magenta.
type Nav struct {
	Hall         Hall
	Cell         float64
	NX, NZ       int
	static, ring []float32
	mag          []float32
	gens         []V2
	ringPoly     []V2
	maze         []Seg
	// A* scratch
	g    []float32
	from []int32
	seen []uint32
	done []uint32
	gen  uint32
}

type Seg struct{ A, B V2 }

const navCell = 0.2

func NewNav(h Hall, gens []V2) *Nav {
	n := &Nav{Hall: h, Cell: navCell}
	n.NX, n.NZ = int(math.Ceil(h.W/navCell)), int(math.Ceil(h.H/navCell))
	N := n.NX * n.NZ
	n.static, n.ring, n.mag = make([]float32, N), make([]float32, N), make([]float32, N)
	n.g, n.from, n.seen, n.done = make([]float32, N), make([]int32, N), make([]uint32, N), make([]uint32, N)
	n.gens = append([]V2(nil), gens...)
	for i := 0; i < N; i++ {
		p := n.center(i)
		d := h.WallDist(p)
		for _, g := range gens {
			d = math.Min(d, Dist(p, g)-0.3)
		}
		n.static[i] = float32(d)
		n.ring[i], n.mag[i] = 1e9, 1e9
	}
	return n
}

func (n *Nav) center(i int) V2 {
	ix, iz := i%n.NX, i/n.NX
	return n.Hall.F.From(V((float64(ix)+0.5)*n.Cell, (float64(iz)+0.5)*n.Cell))
}

func (n *Nav) cellOf(p V2) (int, bool) {
	q := n.Hall.F.To(p)
	ix, iz := int(math.Floor(q.X/n.Cell)), int(math.Floor(q.Z/n.Cell))
	if ix < 0 || iz < 0 || ix >= n.NX || iz >= n.NZ {
		return -1, false
	}
	return iz*n.NX + ix, true
}

// SetRing: the powered ring changed shape.
func (n *Nav) SetRing(poly []V2) {
	n.ringPoly = append(n.ringPoly[:0], poly...)
	for i := range n.ring {
		if len(poly) < 3 {
			n.ring[i] = 1e9
			continue
		}
		p := n.center(i)
		if InPoly(poly, p) {
			n.ring[i] = 0
		} else {
			n.ring[i] = float32(EdgeDist(poly, p))
		}
	}
}

// SetMaze: the magenta walls changed.
func (n *Nav) SetMaze(segs []Seg) {
	n.maze = append(n.maze[:0], segs...)
	for i := range n.mag {
		p := n.center(i)
		m := 1e9
		for _, s := range segs {
			d, _ := SegDist(p, s.A, s.B)
			m = math.Min(m, d-0.05)
		}
		n.mag[i] = float32(m)
	}
}

func (n *Nav) clear(i int, r float64, burn bool) float64 {
	d := math.Min(float64(n.static[i]), float64(n.ring[i]))
	if !burn {
		d = math.Min(d, float64(n.mag[i]))
	}
	return d - r
}

func (n *Nav) Walkable(p V2, r float64, burn bool) bool {
	i, ok := n.cellOf(p)
	return ok && n.clear(i, r, burn) >= 0
}

// Clearance at p (how far past radius r the nearest obstacle is; negative = blocked).
func (n *Nav) Clearance(p V2, r float64, burn bool) float64 {
	i, ok := n.cellOf(p)
	if !ok {
		return -1
	}
	return n.clear(i, r, burn)
}

// nearestFree: the closest standable cell to i (breadth first, up to ~3 m away).
func (n *Nav) nearestFree(i int, r float64, burn bool) int {
	if n.clear(i, r, burn) >= 0 {
		return i
	}
	n.gen++
	q := []int{i}
	n.seen[i] = n.gen
	for k := 0; k < len(q) && k < 2500; k++ {
		c := q[k]
		if n.clear(c, r, burn) >= 0 {
			return c
		}
		cx, cz := c%n.NX, c/n.NX
		for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			x, z := cx+d[0], cz+d[1]
			if x < 0 || z < 0 || x >= n.NX || z >= n.NZ {
				continue
			}
			j := z*n.NX + x
			if n.seen[j] != n.gen {
				n.seen[j] = n.gen
				q = append(q, j)
			}
		}
	}
	return -1
}

type pqItem struct {
	i int32
	f float32
}
type pq []pqItem

func (p pq) Len() int           { return len(p) }
func (p pq) Less(a, b int) bool { return p[a].f < p[b].f }
func (p pq) Swap(a, b int)      { p[a], p[b] = p[b], p[a] }
func (p *pq) Push(x any)        { *p = append(*p, x.(pqItem)) }
func (p *pq) Pop() any          { o := *p; x := o[len(o)-1]; *p = o[:len(o)-1]; return x }

// Path: a walkable route from a to b for a robot of radius r, smoothed into straight legs.
// It returns the waypoints after a (ending at b, or the free spot nearest b) and the length.
func (n *Nav) Path(a, b V2, r float64, burn bool) ([]V2, float64, bool) {
	ia, ok1 := n.cellOf(a)
	ib, ok2 := n.cellOf(b)
	if !ok1 || !ok2 {
		return nil, 0, false
	}
	ia = n.nearestFree(ia, r, burn)
	ib0 := ib
	ib = n.nearestFree(ib, r, burn)
	if ia < 0 || ib < 0 {
		return nil, 0, false
	}
	n.gen++
	h := func(i int) float32 {
		dx := math.Abs(float64(i%n.NX - ib%n.NX))
		dz := math.Abs(float64(i/n.NX - ib/n.NX))
		return float32((math.Max(dx, dz) + (math.Sqrt2-1)*math.Min(dx, dz)) * n.Cell)
	}
	open := &pq{}
	n.g[ia], n.from[ia], n.seen[ia] = 0, -1, n.gen
	heap.Push(open, pqItem{int32(ia), h(ia)})
	found := false
	dirs := [8][3]float64{{1, 0, 1}, {-1, 0, 1}, {0, 1, 1}, {0, -1, 1}, {1, 1, math.Sqrt2}, {1, -1, math.Sqrt2}, {-1, 1, math.Sqrt2}, {-1, -1, math.Sqrt2}}
	for open.Len() > 0 {
		it := heap.Pop(open).(pqItem)
		c := int(it.i)
		if n.done[c] == n.gen {
			continue
		}
		n.done[c] = n.gen
		if c == ib {
			found = true
			break
		}
		cx, cz := c%n.NX, c/n.NX
		for _, d := range dirs {
			x, z := cx+int(d[0]), cz+int(d[1])
			if x < 0 || z < 0 || x >= n.NX || z >= n.NZ {
				continue
			}
			j := z*n.NX + x
			if n.clear(j, r, burn) < 0 {
				continue
			}
			if d[2] > 1 && (n.clear(cz*n.NX+x, r, burn) < 0 || n.clear(z*n.NX+cx, r, burn) < 0) {
				continue // no cutting corners past an obstacle
			}
			ng := n.g[c] + float32(d[2]*n.Cell)
			if n.seen[j] == n.gen && ng >= n.g[j] {
				continue
			}
			n.seen[j], n.g[j], n.from[j] = n.gen, ng, int32(c)
			heap.Push(open, pqItem{int32(j), ng + h(j)})
		}
	}
	if !found {
		return nil, 0, false
	}
	var cells []int
	for c := ib; c >= 0; c = int(n.from[c]) {
		cells = append(cells, c)
		if c == ia {
			break
		}
	}
	// reverse into a -> b order
	for i, j := 0, len(cells)-1; i < j; i, j = i+1, j-1 {
		cells[i], cells[j] = cells[j], cells[i]
	}
	pts := make([]V2, len(cells))
	for i, c := range cells {
		pts[i] = n.center(c)
	}
	pts[0] = a
	if ib == ib0 {
		pts[len(pts)-1] = b
	}
	sm := n.smooth(pts, r, burn)
	L := 0.0
	prev := a
	for _, p := range sm[1:] {
		L += Dist(prev, p)
		prev = p
	}
	return sm[1:], L, true
}

// smooth: string-pull the cell path into the fewest straight legs that stay walkable.
func (n *Nav) smooth(P []V2, r float64, burn bool) []V2 {
	if len(P) <= 2 {
		return P
	}
	out := []V2{P[0]}
	i := 0
	for i < len(P)-1 {
		j := len(P) - 1
		for j > i+1 && !n.lineFree(P[i], P[j], r, burn) {
			j--
		}
		out = append(out, P[j])
		i = j
	}
	return out
}

func (n *Nav) lineFree(a, b V2, r float64, burn bool) bool {
	L := Dist(a, b)
	steps := int(L/(n.Cell*0.5)) + 1
	for k := 1; k < steps; k++ {
		p := Lerp(a, b, float64(k)/float64(steps))
		i, ok := n.cellOf(p)
		if !ok || n.clear(i, r, burn) < -0.02 {
			return false
		}
	}
	return true
}

// Reachable: can a robot of radius r get from a to b at all.
func (n *Nav) Reachable(a, b V2, r float64, burn bool) bool {
	_, _, ok := n.Path(a, b, r, burn)
	return ok
}
