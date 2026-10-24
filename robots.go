package main

import (
	"math"
	"math/rand"
)

type rbDoor struct{ mouth, end pt2 }

var world struct {
	ok                 bool
	g                  rigid2
	lx0, lx1, lz0, lz1 float64
	doors              []rbDoor
}

var (
	cRobot    = rgba{0.13, 0.14, 0.16, 1}
	cRobotPl  = rgba{0.26, 0.27, 0.30, 1}
	cRobotEye = rgba{1.00, 0.08, 0.04, 1}
	cWeld     = rgba{0.70, 0.80, 1.00, 1}
	cHazard   = rgba{0.85, 0.55, 0.05, 1}
	genFault  []float64
	robots    []*robot
	rbSpawnT  = 3.0
)

const (
	rbWalk = iota
	rbWork
	rbLook
	rbTalk
	rbEvade
	rbSalvage
	rbFire
)

const (
	kindWarden = iota
	kindTread
	kindScuttler
)

type robot struct {
	kind              int
	pos               pt2
	yaw, headYaw      float64
	path              []pt2
	state             int
	gen, jobs         int
	leaving           bool
	timer, total      float64
	phase, moving     float64
	seed              float64
	stepIdx           int
	travel            float64
	torch, claw, tail vec3
	weldOn            bool
	done              bool
	wear              float64
	paint             rgba
	chrome            bool
	partner           *robot
	talkT, talkCool   float64
	nextLine          float64
	speaking          bool
	jointSpark        float64
	vid               int
	prevState         int
	evadeUntil        float64
	salv              *oldBase
	sv, svWall        int
	svOut, svWork     pt2
	salvaging, carry  bool
	crouch, liftK     float64
	yieldT, chkT      float64
	yieldTo, chkPos   pt2
	chkWP             pt2
	chkD              float64
	chkN              int
	yieldDone         bool
	unsticking        bool
	stuckN            int
	stuckAt           pt2
	stuckT            float64
	winT              float64
	winPos            pt2
	yieldFrom         *robot
	backing           bool
	firePh            int
	fireSpot          pt2
	sprayOn           bool
	nextHiss          float64
	exK, rpK          float64 // fire crew: extinguisher drawn (0 stowed .. 1 aimed), repair tool out
	aimUp             float64 // fire crew: horn raised onto the fire (1) or dipped between blasts (0)
	fireT             float64
	fireCrew          bool // carries an extinguisher on its back/hull
	hornTip, hornDir  vec3 // where the CO2 horn was last drawn
	hornT             float64
	// arm pose springs: each tool arm eases between poses instead of snapping
	armOff, armVel [3]vec3
	armL           [3][2]float64
	armInit        [3]bool
	armT           [3]float64
	toolK          float64 // salvage cutter unfolded (0 stowed .. 1 working)
	tdS            vec3
	braceI         int
	svTool         int
	svPiece        int
	svTrips        int
	cutPt          vec3
	carryBase      *oldBase
	svWait         float64
	// set by the game: where the arms work (instead of a generator), and the body falling
	workSet bool
	workAt  pt2
	workY   float64
	tilt    float64 // radians, falling forward
	sink    float64 // metres into the floor
	noWeld  bool    // arms out but no torch (carrying)
}

var paints = []rgba{
	{0.28, 0.29, 0.31, 1}, {0.62, 0.48, 0.12, 1}, {0.42, 0.13, 0.09, 1},
	{0.28, 0.31, 0.20, 1}, {0.70, 0.70, 0.66, 1}, {0.09, 0.09, 0.10, 1},
}

var robotSerial int

func (r *robot) randomize() {
	robotSerial++
	r.vid = robotSerial
	r.paint = paints[rand.Intn(len(paints))]
	switch x := rand.Float64(); {
	case x < 0.22:
		r.wear = 0.04 + rand.Float64()*0.1
	case x < 0.36:
		r.wear = 0.03 + rand.Float64()*0.08
		r.chrome = true
	default:
		r.wear = 0.5 + rand.Float64()*0.45
	}
	r.talkCool = 4 + rand.Float64()*6
}

func (e *editor) pathClear(a, b pt2, r float64) bool { return e.pathClearV(a, b, r, r) }

// obstacles: generator posts and their (possibly off-centre) bases
func (e *editor) obstacles() []pt2 {
	out := make([]pt2, 0, len(e.pts)*2)
	for _, p := range e.pts {
		out = append(out, p)
		if a := anchorOf(p); a != p {
			out = append(out, a)
		}
	}
	return out
}

func (e *editor) pathClearV(a, b pt2, r, rv float64) bool {
	P := e.pts
	if len(P) < 3 || e.drawing {
		return true
	}
	for i := range P {
		if segsCross(a, b, P[i], P[(i+1)%len(P)]) {
			return false
		}
	}
	if r <= 0.06 {
		return true
	}
	if pointInPoly(P, a[0], a[1]) || pointInPoly(P, b[0], b[1]) {
		return false
	}
	if outsideDist(P, a) < r || outsideDist(P, b) < r {
		return false
	}
	// the whole segment, not just its ends: a leg that cuts a corner of the zone would
	// otherwise brush the laser wall half way along
	if L := dist2(a, b); L > 0.2 {
		n := int(L / 0.15)
		for k := 1; k < n; k++ {
			u := float64(k) / float64(n)
			if outsideDist(P, pt2{a[0] + (b[0]-a[0])*u, a[1] + (b[1]-a[1])*u}) < r-0.01 {
				return false
			}
		}
	}
	for _, p := range e.obstacles() {
		if d, _ := segDist(p, a, b); d < rv {
			return false
		}
	}
	return true
}

func (e *editor) edgeOut(i int) pt2 {
	n := len(e.pts)
	a, b := e.pts[i], e.pts[(i+1)%n]
	dx, dz := b[0]-a[0], b[1]-a[1]
	l := math.Hypot(dx, dz)
	if l < 1e-6 {
		return pt2{}
	}
	nx, nz := dz/l, -dx/l
	m := pt2{(a[0]+b[0])/2 + nx*0.01, (a[1]+b[1])/2 + nz*0.01}
	if pointInPoly(e.pts, m[0], m[1]) {
		nx, nz = -nx, -nz
	}
	return pt2{nx, nz}
}

func (e *editor) genWork(i int, d float64) (pt2, bool) {
	n := len(e.pts)
	if n < 3 || i < 0 || i >= n {
		return pt2{}, false
	}
	a, b := e.edgeOut((i-1+n)%n), e.edgeOut(i)
	bx, bz := a[0]+b[0], a[1]+b[1]
	l := math.Hypot(bx, bz)
	if l < 0.2 {
		bx, bz, l = b[0], b[1], 1
	}
	bx, bz = bx/l, bz/l
	p := e.pts[i]
	for _, k := range []float64{1, 1.4, 1.9} {
		q := pt2{p[0] + bx*d*k, p[1] + bz*d*k}
		if !pointInPoly(e.pts, q[0], q[1]) && outsideDist(e.pts, q) > robotWallClear {
			return q, true
		}
	}
	return pt2{}, false
}

func laneLocal(s float64) pt2 {
	w, h := world.lx1-world.lx0, world.lz1-world.lz0
	P := 2 * (w + h)
	s = math.Mod(math.Mod(s, P)+P, P)
	switch {
	case s < w:
		return pt2{world.lx0 + s, world.lz0}
	case s < w+h:
		return pt2{world.lx1, world.lz0 + s - w}
	case s < 2*w+h:
		return pt2{world.lx1 - (s - w - h), world.lz1}
	}
	return pt2{world.lx0, world.lz1 - (s - 2*w - h)}
}

func laneProj(p pt2) pt2 {
	q := world.g.inv().apply(p)
	x := math.Max(world.lx0, math.Min(world.lx1, q[0]))
	z := math.Max(world.lz0, math.Min(world.lz1, q[1]))
	dx0, dx1, dz0, dz1 := math.Abs(x-world.lx0), math.Abs(x-world.lx1), math.Abs(z-world.lz0), math.Abs(z-world.lz1)
	m := math.Min(math.Min(dx0, dx1), math.Min(dz0, dz1))
	switch m {
	case dx0:
		x = world.lx0
	case dx1:
		x = world.lx1
	case dz0:
		z = world.lz0
	default:
		z = world.lz1
	}
	return world.g.apply(pt2{x, z})
}

// robots keep their whole body (shoulders, arms, tracks) clear of laser walls and generators
const (
	robotWallClear = 0.62
	robotPostClear = 0.66
)

func (e *editor) route(from, to pt2) []pt2 { return e.routeC(from, to, robotPostClear) }

func (e *editor) routeC(from, to pt2, rv float64) []pt2 {
	nodes := []pt2{from, to}
	for i := range e.pts {
		if e.drawing {
			break
		}
		if w, ok := e.genWork(i, math.Max(0.7, rv+0.2)); ok {
			nodes = append(nodes, w, laneProj(w))
		}
	}
	for k := 0; k < 4; k++ {
		c := []pt2{{world.lx0, world.lz0}, {world.lx1, world.lz0}, {world.lx1, world.lz1}, {world.lx0, world.lz1}}[k]
		nodes = append(nodes, world.g.apply(c))
	}
	for _, d := range world.doors {
		nodes = append(nodes, d.mouth, laneProj(d.mouth))
	}
	nodes = append(nodes, laneProj(from), laneProj(to))
	N := len(nodes)
	rEnd := 0.05
	if len(e.pts) >= 3 && !e.drawing {
		rEnd = math.Max(0.05, math.Min(robotWallClear-0.02, math.Min(outsideDist(e.pts, from), outsideDist(e.pts, to))-0.02))
	}
	obst := e.obstacles()
	clrOf := func(p pt2) float64 {
		m := math.Inf(1)
		for _, o := range obst {
			m = math.Min(m, dist2(o, p))
		}
		return m
	}
	clrFrom, clrTo := clrOf(from), clrOf(to)
	dist := make([]float64, N)
	prev := make([]int, N)
	done := make([]bool, N)
	for i := range dist {
		dist[i], prev[i] = math.Inf(1), -1
	}
	dist[0] = 0
	for {
		u := -1
		for i := 0; i < N; i++ {
			if !done[i] && !math.IsInf(dist[i], 1) && (u < 0 || dist[i] < dist[u]) {
				u = i
			}
		}
		if u < 0 || u == 1 {
			break
		}
		done[u] = true
		for v := 0; v < N; v++ {
			if done[v] || v == u {
				continue
			}
			c := dist[u] + dist2(nodes[u], nodes[v])
			if c >= dist[v] {
				continue
			}
			r := robotWallClear
			if u == 0 || v == 1 {
				r = rEnd
			}
			rr := math.Max(r, rv)
			if u == 0 {
				rr = math.Max(r, math.Min(rr, clrFrom-0.02))
			}
			if v == 1 {
				rr = math.Max(r, math.Min(rr, clrTo-0.02))
			}
			if e.pathClearV(nodes[u], nodes[v], math.Min(r, robotWallClear), rr) {
				dist[v], prev[v] = c, u
			}
		}
	}
	if prev[1] < 0 {
		return nil
	}
	var out []pt2
	for v := 1; v > 0; v = prev[v] {
		out = append([]pt2{nodes[v]}, out...)
	}
	return out
}

func (e *editor) pickGen(r *robot) int {
	n := len(e.pts)
	if n < 3 || e.drawing {
		return -1
	}
	taken := map[int]bool{}
	for _, o := range robots {
		if o != r && o.gen >= 0 {
			taken[o.gen] = true
		}
	}
	tot := 0.0
	w := make([]float64, n)
	for i := 0; i < n; i++ {
		if taken[i] || i == r.gen {
			continue
		}
		w[i] = 1
		if i < len(genFault) {
			w[i] += 8 * genFault[i]
		}
		tot += w[i]
	}
	if tot == 0 {
		return -1
	}
	x := rand.Float64() * tot
	for i := range w {
		x -= w[i]
		if x <= 0 && w[i] > 0 {
			return i
		}
	}
	return -1
}

func (e *editor) sendToGen(r *robot) bool {
	for try := 0; try < 4; try++ {
		g := e.pickGen(r)
		if g < 0 {
			return false
		}
		w, ok := e.genWork(g, 0.7)
		if !ok {
			continue
		}
		if p := e.route(r.pos, w); p != nil {
			r.gen, r.path, r.state = g, p, rbWalk
			return true
		}
	}
	return false
}

func (e *editor) sendHome(r *robot) {
	r.leaving, r.gen, r.state = true, -1, rbWalk
	d := world.doors[rand.Intn(len(world.doors))]
	p := e.route(r.pos, d.mouth)
	if p == nil {
		p = []pt2{d.mouth}
	}
	r.path = append(p, d.end)
}

// sendHomeAvoiding: leave by whichever door's route stays furthest from a trouble spot
func (e *editor) sendHomeAvoiding(r *robot, bad pt2) {
	var best []pt2
	bs := -1.0
	for _, d := range world.doors {
		p := e.route(r.pos, d.mouth)
		if p == nil {
			continue
		}
		m := 9.0
		prev := r.pos
		for _, q := range p {
			dd, _ := segDist(bad, prev, q)
			m = math.Min(m, dd)
			prev = q
		}
		if m > bs {
			bs, best = m, append(p, d.end)
		}
	}
	r.leaving, r.gen, r.state = true, -1, rbWalk
	if best == nil {
		e.sendHome(r)
		return
	}
	r.path = best
}

func (e *editor) spawnRobot() {
	if !world.ok || len(world.doors) < 2 {
		return
	}
	di := rand.Intn(len(world.doors))
	d := world.doors[di]
	r := &robot{pos: d.end, gen: -1, seed: rand.Float64() * 100}
	r.randomize()
	if rand.Float64() < 0.35 {
		r.kind = kindTread
	}
	dx, dz := d.mouth[0]-d.end[0], d.mouth[1]-d.end[1]
	r.yaw = math.Atan2(dz, dx)
	r.pos = d.end
	if rand.Float64() < 0.7 && len(e.pts) >= 3 && !e.drawing {
		r.jobs = 1 + rand.Intn(3)
		r.pos = d.mouth
		if e.sendToGen(r) {
			r.path = append([]pt2{d.mouth}, r.path...)
			r.pos = d.end
			robots = append(robots, r)
			return
		}
		r.pos = d.end
	}
	o := world.doors[(di+1+rand.Intn(len(world.doors)-1))%len(world.doors)]
	p := e.route(d.mouth, o.mouth)
	if p == nil {
		return
	}
	r.leaving = true
	r.path = append(append([]pt2{d.mouth}, p...), o.end)
	robots = append(robots, r)
}

func angDiff(a, b float64) float64 {
	d := math.Mod(b-a+math.Pi, 2*math.Pi)
	if d < 0 {
		d += 2 * math.Pi
	}
	return d - math.Pi
}

func (e *editor) updateFaults(dt float64) {
	n := len(e.pts)
	if e.drawing {
		n = 0
	}
	if len(genFault) != n {
		genFault = make([]float64, n)
		for _, r := range robots {
			if r.gen >= n {
				r.gen = -1
			}
		}
	}
	if n > 0 && rand.Float64() < dt/150 {
		genFault[rand.Intn(n)] = 0.5 + 0.5*rand.Float64()
	}
}

func (e *editor) updateRobots(dt float64) {
	e.updateFaults(dt)
	if !world.ok {
		return
	}
	e.assignSalvage()
	rbSpawnT -= dt
	if rbSpawnT <= 0 {
		rbSpawnT = 7 + rand.Float64()*18
		if len(robots) < 3 {
			e.spawnRobot()
		}
	}
	cam := e.hmd.pos()
	weld, weldPan, servo, servoPan := 0.0, 0.0, 0.0, 0.0
	alive := robots[:0]
	for _, r := range robots {
		e.stepRobot(r, dt)
		if r.done {
			continue
		}
		alive = append(alive, r)
		p := fl(r.pos, 1)
		d := p.sub(cam).length()
		att := 1 / (1 + 0.35*d*d)
		pan := p.sub(cam).norm().dot(e.hmd.axis(0))
		if r.weldOn {
			if w := 0.5 * att; w > weld {
				weld, weldPan = w, pan
			}
		}
		if s := r.moving * att * 0.5; s > servo {
			servo, servoPan = s, pan
		}
		e.drawRobot(r)
	}
	robots = alive
	e.separateRobots()
	audioRobots(weld, weldPan, servo, servoPan)
}

func (e *editor) stepRobot(r *robot, dt float64) {
	r.weldOn = false
	r.salvaging = false
	if e.evade(r, dt) {
		cam := e.hmd.pos()
		look := math.Atan2(cam.z-r.pos[1], cam.x-r.pos[0])
		r.headYaw += (math.Max(-1.2, math.Min(1.2, angDiff(r.yaw, look))) - r.headYaw) * math.Min(1, dt*4)
		e.robotSteps(r)
		return
	}
	if e.yieldCheck(r, dt) {
		return
	}
	if r.state == rbFire {
		want, headTo := e.stepFireBot(r, dt)
		r.moving += (want - r.moving) * math.Min(1, dt*5)
		r.headYaw += (headTo - r.headYaw) * math.Min(1, dt*4)
		e.robotSteps(r)
		return
	}
	if r.state == rbSalvage {
		want, headTo := e.stepSalvage(r, dt)
		r.moving += (want - r.moving) * math.Min(1, dt*5)
		r.headYaw += (headTo - r.headYaw) * math.Min(1, dt*4)
		e.robotSteps(r)
		return
	}
	speed := 0.85
	stride := 0.62
	if r.kind == kindTread {
		speed, stride = 1.25, 0.42
	}
	want := 0.0
	headTo := 0.0
	cam := e.hmd.pos()
	look := math.Atan2(cam.z-r.pos[1], cam.x-r.pos[0])
	r.talkCool -= dt
	if r.state == rbWalk && r.talkCool <= 0 && len(r.path) > 1 {
		for _, o := range robots {
			if o == r || o.done || o.state != rbWalk || o.talkCool > 0 || len(o.path) < 2 {
				continue
			}
			if dist2(r.pos, o.pos) < 1.9 {
				dur := 3.5 + rand.Float64()*3.5
				for _, x := range []*robot{r, o} {
					x.state, x.timer = rbTalk, dur
					x.speaking = false
				}
				r.partner, o.partner = o, r
				r.speaking = true
				r.nextLine = fxTime() + 0.3
				o.nextLine = r.nextLine
				break
			}
		}
	}
	switch r.state {
	case rbTalk:
		p := r.partner
		if p == nil || p.done || p.partner != r {
			r.state, r.partner = rbWalk, nil
			break
		}
		toP := math.Atan2(p.pos[1]-r.pos[1], p.pos[0]-r.pos[0])
		r.yaw += angDiff(r.yaw, toP) * math.Min(1, dt*2)
		headTo = math.Max(-1, math.Min(1, angDiff(r.yaw, toP))) + 0.15*math.Sin(fxTime()*2+r.seed)
		r.timer -= dt
		if r.vid < p.vid && fxTime() >= r.nextLine {
			sp := r
			if !r.speaking {
				sp = p
			}
			e.sfx(sndSpeech, fl(sp.pos, 1.6), 0.55)
			gap := 0.45 + rand.Float64()*0.9
			r.nextLine = fxTime() + gap
			r.speaking, p.speaking = !r.speaking, r.speaking
		}
		if r.timer <= 0 {
			for _, x := range []*robot{r, p} {
				x.state, x.partner, x.speaking = rbWalk, nil, false
				x.talkCool = 14 + rand.Float64()*20
			}
		}
	case rbWalk:
		if len(r.path) == 0 {
			if r.leaving {
				r.done = true
				return
			}
			if r.gen >= 0 && r.gen < len(e.pts) {
				r.state = rbWork
				r.total = 6 + rand.Float64()*9
				r.timer = r.total
			} else {
				e.sendHome(r)
			}
			break
		}
		tgt := r.path[0]
		dx, dz := tgt[0]-r.pos[0], tgt[1]-r.pos[1]
		l := math.Hypot(dx, dz)
		if l < 0.06 {
			r.path = r.path[1:]
			break
		}
		dirx, dirz := dx/l, dz/l
		sx, sz, slow := 0.0, 0.0, 1.0
		for _, o := range robots {
			if o == r || o.done {
				continue
			}
			ox, oz := r.pos[0]-o.pos[0], r.pos[1]-o.pos[1]
			d := math.Hypot(ox, oz)
			if d < 1e-4 || d > 1.4 {
				continue
			}
			w := (1.4 - d) / 1.4
			sx += ox / d * w * 1.4
			sz += oz / d * w * 1.4
			if ahead := -(ox*dirx + oz*dirz) / d; ahead > 0.4 && d < 1.1 {
				if o.state != rbWalk || o.seed < r.seed {
					slow = math.Min(slow, math.Max(0.08, (d-0.7)/0.4))
				}
				px, pz := -dirz, dirx
				if px*ox+pz*oz < 0 {
					px, pz = -px, -pz
				}
				sx += px * w
				sz += pz * w
			}
		}
		mx, mz := dirx+sx, dirz+sz
		if ml := math.Hypot(mx, mz); ml > 1e-6 {
			mx, mz = mx/ml, mz/ml
		}
		ty := math.Atan2(mz, mx)
		df := angDiff(r.yaw, ty)
		r.yaw += df * math.Min(1, dt*3.5)
		v := speed * math.Max(0.15, math.Cos(df)) * slow
		step := math.Min(l, v*dt)
		cand := pt2{r.pos[0] + mx*step, r.pos[1] + mz*step}
		if len(e.pts) >= 3 && !e.drawing && (pointInPoly(e.pts, cand[0], cand[1]) || outsideDist(e.pts, cand) < robotWallClear) {
			cand = pt2{r.pos[0] + dirx*step, r.pos[1] + dirz*step}
			// never step closer to a laser wall than we already are once inside the margin
			allow := math.Min(robotWallClear, outsideDist(e.pts, tgt)-0.02)
			if od := outsideDist(e.pts, cand); pointInPoly(e.pts, cand[0], cand[1]) || od < allow && od < outsideDist(e.pts, r.pos) {
				cand = r.pos
			}
		}
		if len(e.pts) >= 3 && !e.drawing {
			for _, o := range e.obstacles() {
				if dc := dist2(o, cand); dc < math.Min(robotPostClear, dist2(o, tgt)-0.02) && dc < dist2(o, r.pos) {
					// slide round a generator instead of brushing it
					t := v3(cand[0]-o[0], 0, cand[1]-o[1]).norm()
					cand = pt2{o[0] + t.x*robotPostClear, o[1] + t.z*robotPostClear}
					if pointInPoly(e.pts, cand[0], cand[1]) || outsideDist(e.pts, cand) < robotWallClear-0.05 {
						cand = r.pos
					}
				}
			}
		}
		r.pos = cand
		r.travel += step
		r.phase += step / stride * math.Pi
		want = 1
		if rand.Float64() < dt*0.035 && !r.leaving && outsideDist(e.pts, r.pos) < 3 {
			r.state, r.timer = rbLook, 2+rand.Float64()*2
		}
		headTo = 0.35 * math.Sin(fxTime()*0.7+r.seed)
		if ad := angDiff(r.yaw, look); math.Abs(ad) < 1.2 && outsideDist(e.pts, r.pos) < 1.6 {
			headTo = ad
		}
	case rbLook:
		r.timer -= dt
		headTo = math.Max(-1.4, math.Min(1.4, angDiff(r.yaw, look)))
		if math.Abs(angDiff(r.yaw, look)) > 1.3 {
			r.yaw += angDiff(r.yaw, look) * math.Min(1, dt*1.5)
		}
		if r.timer <= 0 {
			r.state = rbWalk
		}
	case rbWork:
		if r.gen < 0 || r.gen >= len(e.pts) {
			r.state = rbWalk
			e.sendHome(r)
			break
		}
		g := e.pts[r.gen]
		ty := math.Atan2(g[1]-r.pos[1], g[0]-r.pos[0])
		r.yaw += angDiff(r.yaw, ty) * math.Min(1, dt*3)
		r.timer -= dt
		if r.gen < len(genFault) {
			genFault[r.gen] = math.Max(0, genFault[r.gen]-dt/r.total*1.2)
		}
		headTo = 0.15 * math.Sin(fxTime()*1.3+r.seed)
		if r.timer <= 0 {
			if r.gen < len(genFault) {
				genFault[r.gen] = 0
			}
			r.jobs--
			if r.jobs <= 0 || rand.Float64() < 0.2 || !e.sendToGen(r) {
				e.sendHome(r)
			}
		}
	}
	r.moving += (want - r.moving) * math.Min(1, dt*5)
	r.headYaw += (headTo - r.headYaw) * math.Min(1, dt*4)
	e.robotSteps(r)

}

func (e *editor) doorPass(r *robot) {
	t := fxTime()
	for i, d := range world.doors {
		if dist2(r.pos, d.mouth) < 0.7 && t > doorSteamNext[i%2]-5 {
			doorSteamNext[i%2] = t + 6 + rand.Float64()*6
			e.doorSteam(i)
		}
	}
}

func (e *editor) robotSteps(r *robot) {
	e.doorPass(r)
	cam := e.hmd.pos()
	if si := int(math.Floor(r.phase / math.Pi)); si != r.stepIdx {
		r.stepIdx = si
		if r.moving > 0.3 {
			p := fl(r.pos, 0)
			d := p.sub(cam).length()
			att := 1 / (1 + 0.4*d*d)
			pan := p.sub(cam).norm().dot(e.hmd.axis(0))
			if r.kind == kindWarden {
				audioStep(0.9*att, pan, sndHeavyStep)
				if rand.Float64() < 0.35 {
					audioStep(0.5*att, pan, sndHydraulic)
				}
			} else {
				audioStep(0.45*att, pan, sndTreadClank)
			}
		}
	}
}

// ---- deadlock breaking: when two robots block each other, the lower-priority one steps aside ----

func (r *robot) priority() int {
	switch {
	case r.carryBase != nil || r.svPiece >= 0 && r.sv == svCarry:
		return 3
	case r.state == rbSalvage:
		return 2
	}
	return 1
}

func inLaneRect(p pt2, margin float64) bool {
	q := world.g.inv().apply(p)
	return q[0] > world.lx0-margin && q[0] < world.lx1+margin && q[1] > world.lz0-margin && q[1] < world.lz1+margin
}

func (e *editor) yieldCheck(r *robot, dt float64) bool {
	t := fxTime()
	if r.yieldT > 0 {
		r.yieldT -= dt
		if !r.yieldDone {
			if r.walkTo(r.yieldTo, 0.7, dt) {
				r.yieldDone = true
			}
			r.moving += (1 - r.moving) * math.Min(1, dt*5)
		} else {
			if r.yieldFrom != nil {
				ty := math.Atan2(r.yieldFrom.pos[1]-r.pos[1], r.yieldFrom.pos[0]-r.pos[0])
				r.yaw += angDiff(r.yaw, ty) * math.Min(1, dt*2)
			}
			r.moving += (0 - r.moving) * math.Min(1, dt*5)
		}
		r.headYaw += (0 - r.headYaw) * math.Min(1, dt*4)
		if r.yieldT <= 0 {
			r.chkT, r.chkPos = t+2.5, r.pos
			r.yieldFrom = nil
			wasStuck := r.unsticking
			r.unsticking = false
			if wasStuck && r.stuckN >= 2 && r.state != rbSalvage {
				// still wedged after backing off: give up on that way and go the other way round
				r.stuckN = 0
				r.gen = -1
				e.sendHomeAvoiding(r, r.stuckAt)
			} else if r.state == rbSalvage {
				e.planSalvage(r)
			} else if len(r.path) > 0 {
				if p := e.route(r.pos, r.path[len(r.path)-1]); p != nil {
					r.path = p
				}
			}
			r.chkN = len(r.path)
		}
		e.robotSteps(r)
		return true
	}
	travelling := len(r.path) > 0 && (r.state == rbWalk || r.state == rbSalvage && (r.sv == svTravel || r.sv == svCarry || r.sv == svReturn) && !r.backing)
	if !travelling {
		r.chkT, r.chkPos = t+1.5, r.pos
		r.winT, r.winPos = t, r.pos
		return false
	}
	// safety net: whatever the cause, a robot that's meant to be walking but hasn't got
	// half a metre anywhere in 8 s backs off and heads off a different way
	if t > r.winT+8 {
		if r.winT > 0 && dist2(r.pos, r.winPos) < 1.2 {
			r.winT, r.winPos = t, r.pos
			r.stuckN, r.stuckAt, r.stuckT = 1, r.pos, t
			if e.unstickForce(r) {
				return true
			}
		}
		r.winT, r.winPos = t, r.pos
	}
	if t < r.chkT {
		return false
	}
	// progress = getting closer to the current waypoint (wandering on the spot doesn't count)
	wp := r.path[0]
	dNow := dist2(r.pos, wp)
	progress := wp != r.chkWP || r.chkD-dNow > 0.2
	r.chkT, r.chkPos, r.chkWP, r.chkD, r.chkN = t+2.0, r.pos, wp, dNow, len(r.path)
	if progress {
		return false
	}
	for _, o := range robots {
		if o == r || o.done || dist2(o.pos, r.pos) > 1.2 {
			continue
		}
		if !o.travelling() && o.yieldT <= 0 {
			// it's busy (working, talking, looking) and won't move: we go round the other way
			return e.unstick(r)
		}
		if o.yieldT > 0 || r.priority() < o.priority() || r.priority() == o.priority() && r.seed < o.seed {
			if o.yieldT > 0 {
				continue // the other one is already getting out of the way
			}
		} else {
			continue // the other one should yield
		}
		away := v3(r.pos[0]-o.pos[0], 0, r.pos[1]-o.pos[1]).norm()
		side := v3(-away.z, 0, away.x)
		best, bs := pt2{}, math.Inf(-1)
		for _, c := range []vec3{away, away.add(side).norm(), away.sub(side).norm(), side, side.mul(-1)} {
			for _, dd := range []float64{1.0, 0.7} {
				p := pt2{r.pos[0] + c.x*dd, r.pos[1] + c.z*dd}
				if !inLaneRect(p, 0.05) || !e.pathClearV(r.pos, p, robotWallClear-0.05, robotPostClear-0.05) {
					continue
				}
				score := dist2(p, o.pos)
				for _, x := range robots {
					if x != r && x != o && !x.done {
						score = math.Min(score, dist2(p, x.pos))
					}
				}
				if score > bs {
					best, bs = p, score
				}
			}
		}
		if math.IsInf(bs, -1) {
			continue
		}
		r.yieldTo, r.yieldT, r.yieldDone, r.yieldFrom = best, 2.5+rand.Float64()*1.5, false, o
		return true
	}
	// no robot in the way: wedged against a wall or a generator. Like a roomba - back off
	// into open space, turn round and try another way.
	return e.unstick(r)
}

func (e *editor) clearance(p pt2, self *robot) float64 {
	c := 9.0
	if len(e.pts) >= 3 && !e.drawing {
		if pointInPoly(e.pts, p[0], p[1]) {
			return -1
		}
		c = outsideDist(e.pts, p)
	}
	for _, o := range e.obstacles() {
		c = math.Min(c, dist2(o, p))
	}
	for _, x := range robots {
		if x != self && !x.done {
			c = math.Min(c, dist2(x.pos, p)*0.8)
		}
	}
	return c
}

func (e *editor) staticClear(p pt2) float64 {
	c := 9.0
	if len(e.pts) >= 3 && !e.drawing {
		c = outsideDist(e.pts, p)
		if pointInPoly(e.pts, p[0], p[1]) {
			c = -1
		}
	}
	for _, o := range e.obstacles() {
		c = math.Min(c, dist2(o, p))
	}
	return c
}

func (r *robot) travelling() bool {
	return len(r.path) > 0 && (r.state == rbWalk || r.state == rbSalvage && (r.sv == svTravel || r.sv == svCarry || r.sv == svReturn))
}

var robotDebug func(format string, a ...any)

// unstickForce: back off, then (for walkers) always pick a new way home
func (e *editor) unstickForce(r *robot) bool {
	if !e.unstick(r) {
		return false
	}
	r.stuckN = 2
	return true
}

func (e *editor) unstick(r *robot) bool {
	if robotDebug != nil {
		robotDebug("unstick seed %.1f pos %.2f,%.2f n=%d", r.seed, r.pos[0], r.pos[1], r.stuckN)
	}
	f := v3(math.Cos(r.yaw), 0, math.Sin(r.yaw))
	best, bs := pt2{}, math.Inf(-1)
	for k := 0; k < 16; k++ {
		a := float64(k) * math.Pi / 8
		d := v3(math.Cos(a), 0, math.Sin(a))
		for _, dd := range []float64{0.9, 0.6} {
			p := pt2{r.pos[0] + d.x*dd, r.pos[1] + d.z*dd}
			if world.ok && !inLaneRect(p, 0.05) {
				continue
			}
			c := e.clearance(p, r)
			if c < 0 || e.staticClear(p) < math.Min(robotWallClear-0.05, e.staticClear(r.pos)+0.02) {
				continue
			}
			// prefer open space, and going back the way we came
			score := c - 0.15*d.dot(f)
			if score > bs {
				best, bs = p, score
			}
		}
	}
	if math.IsInf(bs, -1) {
		return false
	}
	t := fxTime()
	if r.stuckN > 0 && dist2(r.pos, r.stuckAt) < 2.5 && t-r.stuckT < 40 {
		r.stuckN++ // wedged again in the same area
	} else {
		r.stuckN, r.stuckAt = 1, r.pos
	}
	r.stuckT = t
	r.yieldTo, r.yieldT, r.yieldDone, r.yieldFrom = best, 1.8+rand.Float64(), false, nil
	r.unsticking = true
	return true
}
