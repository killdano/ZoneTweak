//go:build windows

package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
)

// Moving a generator: the base is a slide mount that lets the shaft wander a little.
// Past ripDist the shaft tears free with sparks, floats while held, then grows a new
// base where it is planted. The torn base stays behind until a robot salvages it.

const (
	slideDist = 0.06 // cables start to strain
	edgeDist  = 0.11 // the mount stops here: the shaft collar never leaves the rubber ring
	pullRip   = 0.11 // extra hand pull past the edge before it tears free
	ripDist   = edgeDist + pullRip
)

var genPull = map[int]float64{}

type oldBase struct {
	joint    bool
	pos      pt2
	ax       vec3
	t0       float64
	claimed  *robot
	progress float64
	gone     bool
	nextFX   float64
	state    int
	cut      float64
	pieces   [3]int
	carrier  *robot
	small    bool
	cutA     float64
	// a bite sheared out by a neighbouring new base (local ax/az coords)
	biteC pt2
	biteR float64
	bites []bite
	// flying / landed broken-off chunk
	debris      bool
	shape       []pt2
	dpos, dvel  vec3
	spinAx      vec3
	spin, spinV float64
	landed      bool
	bounces     int
	liftYaw     float64
	cutSpan     float64
	fireDebris  bool
}

type regrow struct {
	pos      pt2
	t0       float64
	fired    int
	ax       vec3
	reattach bool
	anchor   pt2
	shear    []*oldBase
}

var (
	oldBases  []*oldBase
	regrows   []*regrow
	wallPow   = map[int]float64{}
	wallOff   = map[int]float64{}
	wallKeyN  int
	strainSnd float64
)

// ---- grab hooks ----

var (
	baseAnchor = map[pt2]pt2{}
	jointNbrs  = map[int][]pt2{}
	plates     [][]pt2
	jointGen   = map[pt2]bool{}
)

const jointDist = 0.64

func anchorOf(p pt2) pt2 {
	if a, ok := baseAnchor[p]; ok {
		return a
	}
	return p
}

func (e *editor) grabStart(h *hand, idx int, newPoint bool) {
	h.gAnchor = anchorOf(e.pts[idx])
	h.gJoint = jointGen[h.gAnchor]
	delete(baseAnchor, e.pts[idx])
	h.gRipped = newPoint
	h.gNew = newPoint
}

func (e *editor) grabUpdate(h *hand) {
	if h.grab < 0 || h.grab >= len(e.pts) || h.gNew {
		return
	}
	p := e.pts[h.grab]
	d := dist2(p, h.gAnchor)
	t := fxTime()
	if !h.gRipped {
		pull := math.Max(0, d-edgeDist)
		genPull[h.grab] = pull / pullRip
		if pull > 0 && d < ripDist {
			// the mount bottoms out: the shaft barely gives while the hand keeps pulling
			dir := v3(p[0]-h.gAnchor[0], 0, p[1]-h.gAnchor[1]).norm()
			r := edgeDist + 0.02*(1-math.Exp(-pull/0.04))
			e.pts[h.grab] = pt2{h.gAnchor[0] + dir.x*r, h.gAnchor[1] + dir.z*r}
			if t > h.gBuzz {
				k := pull / pullRip
				h.gBuzz = t + 0.09 - 0.05*k
				hapticAmp(h.idx, 25+int(40*k), float32(0.15+0.45*k))
			}
		}
		if d > slideDist && t > strainSnd {
			k := math.Min(1, (d-slideDist)/(ripDist-slideDist))
			strainSnd = t + 0.6 + rand.Float64()*0.8 - 0.35*k
			c := fl(h.gAnchor, 0.1)
			spawnSparks(c, v3(rand.NormFloat64(), 0.5, rand.NormFloat64()).norm(), 3+int(10*k), 0.3)
			e.sfx(sndCreak, c, 0.4+0.3*k)
		}
		if d > ripDist {
			genPull[h.grab] = 0
			h.gRipped = true
			ax := v3(1, 0, 0)
			if n := len(e.pts); n > 1 {
				ax = baseAxisFor(h.gAnchor, e.pts[(h.grab-1+n)%n], e.pts[(h.grab+1)%n])
			}
			if bp := basePoses[h.gAnchor]; bp != nil {
				ax = bp.ax // the torn base keeps exactly the layout it had
			}
			oldBases = append(oldBases, &oldBase{pos: h.gAnchor, ax: ax, t0: t, joint: h.gJoint})
			c := fl(h.gAnchor, 0.12)
			spawnSparks(c, v3(0, 1, 0), 70, 1.2)
			spawnSmoke(c, 10)
			sparkBursts = append(sparkBursts, sparkBurst{p: c, t: t, k: 1})
			addPoint(c, cSpark, 12, 4)
			e.sfx(sndRip, c, 1.0)
			hapticAmp(h.idx, 220, 1)
		}
	}
}

func (e *editor) grabEnd(h *hand) {
	if h.gRipped && h.grab >= 0 && h.grab < len(e.pts) {
		p := e.pts[h.grab]
		ax := v3(1, 0, 0)
		if n := len(e.pts); n > 1 {
			ax = baseAxisFor(p, e.pts[(h.grab-1+n)%n], e.pts[(h.grab+1)%n])
		}
		if docked, anc, oax, sh := e.plantNearOld(p, ax); docked {
			// the shaft re-docks on its old base, off-centre where it landed
			baseAnchor[p] = anc
			regrows = append(regrows, &regrow{pos: p, t0: fxTime(), ax: oax, reattach: true, anchor: anc})
			e.sfx(sndHydraulic, fl(p, 0.2), 0.9)
			hapticAmp(h.idx, 60, 0.7)
		} else {
			regrows = append(regrows, &regrow{pos: p, t0: fxTime(), ax: ax, shear: sh})
			e.sfx(sndRegrow, fl(p, 0.2), 0.9)
		}
	} else if !h.gNew && h.grab >= 0 && h.grab < len(e.pts) {
		// leave the base where it is: the shaft stays off-centre on its cables
		if p := e.pts[h.grab]; dist2(p, h.gAnchor) > 0.003 {
			baseAnchor[p] = h.gAnchor
		}
	}
	if h.grab >= 0 {
		delete(genPull, h.grab)
	}
	h.gRipped, h.gNew = false, false
}

func (e *editor) idxOf(p pt2) int {
	for i, q := range e.pts {
		if q == p {
			return i
		}
	}
	return -1
}

// generator draw state lookup for index i
func (e *editor) isNewGrab(i int) bool {
	for _, h := range e.hands {
		if h.grab == i && h.gNew {
			return true
		}
	}
	return false
}

func (e *editor) genMode(i int) (anchor pt2, sliding, floating bool) {
	p := e.pts[i]
	for _, h := range e.hands {
		if h.grab == i {
			if h.gRipped || h.gNew {
				// torn free, or brand new and not planted yet: it hovers until let go
				return p, false, true
			}
			return h.gAnchor, true, false
		}
	}
	a := anchorOf(p)
	return a, false, false
}

// computeJoints groups generators whose bases would overlap into shared plates
func (e *editor) computeJoints() {
	n := len(e.pts)
	prevJoint := jointGen
	jointNbrs = map[int][]pt2{}
	jointGen = map[pt2]bool{}
	plates = plates[:0]
	if n < 2 || e.drawing {
		return
	}
	live := map[pt2]bool{}
	for _, p := range e.pts {
		live[p] = true
	}
	for k := range baseAnchor {
		if !live[k] {
			delete(baseAnchor, k)
		}
	}
	// torn-off bases that were part of a shared slab stay part of it until a robot starts
	// cutting them free
	var obs []*oldBase
	for _, ob := range oldBases {
		if ob.joint && ob.state == 0 && !ob.gone && !ob.debris && ob.biteR == 0 {
			obs = append(obs, ob)
		}
	}
	oldJointNbrs = map[*oldBase][]pt2{}
	N := n + len(obs)
	anc := make([]pt2, N)
	ok := make([]bool, N)
	for i := range e.pts {
		a, _, floating := e.genMode(i)
		if floating || regrowAt(e.pts[i]) != nil || deployAt(e.pts[i]) != nil {
			continue
		}
		anc[i], ok[i] = a, true
	}
	for k, ob := range obs {
		anc[n+k], ok[n+k] = ob.pos, true
	}
	parent := make([]int, N)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	for i := 0; i < N; i++ {
		for j := i + 1; j < N; j++ {
			if ok[i] && ok[j] && dist2(anc[i], anc[j]) < jointDist {
				parent[find(i)] = find(j)
			}
		}
	}
	groups := map[int][]int{}
	for i := 0; i < N; i++ {
		if ok[i] {
			groups[find(i)] = append(groups[find(i)], i)
		}
	}
	t := fxTime()
	seen := map[string]bool{}
	plateAges = plateAges[:0]
	for _, g := range groups {
		if len(g) < 2 {
			continue
		}
		sort.Slice(g, func(a, b int) bool {
			if anc[g[a]][0] != anc[g[b]][0] {
				return anc[g[a]][0] < anc[g[b]][0]
			}
			return anc[g[a]][1] < anc[g[b]][1]
		})
		var pl []pt2
		for _, i := range g {
			pl = append(pl, anc[i])
		}
		key := plateKey(pl)
		seen[key] = true
		born, ok := plateBorn[key]
		if !ok {
			born = t
			if t < 3 {
				born = t - 10 // plates that already exist at start-up don't animate
			}
			for _, p := range pl {
				if prevJoint[p] {
					born = t - plateSwap // a slab growing to take in one more: just the tail of the forge
				}
			}
			plateBorn[key] = born
			plateFX[key] = 0
		}
		age := t - born
		if age >= plateSwap {
			// members switch over to the shared mount once the slab has formed under them
			for _, i := range g {
				jointGen[anc[i]] = true
				for _, j := range g {
					if j == i {
						continue
					}
					if i < n {
						jointNbrs[i] = append(jointNbrs[i], anc[j])
					} else {
						oldJointNbrs[obs[i-n]] = append(oldJointNbrs[obs[i-n]], anc[j])
					}
				}
			}
		}
		plates = append(plates, pl)
		plateAges = append(plateAges, age)
	}
	for k, b := range plateBorn {
		if !seen[k] {
			// a plate that just broke up: let it retract instead of vanishing
			if g := parsePlateKey(k); t-b >= plateForm && !sharesAnchor(g) {
				plateGone = append(plateGone, goneSlab{anchors: g, t0: t})
			}
			delete(plateBorn, k)
			delete(plateFX, k)
		}
	}
}

const (
	plateForm = 1.4  // seconds for a joint slab to forge itself
	plateSwap = 0.85 // when the generators' legs and cables re-route onto it
)

type goneSlab struct {
	anchors []pt2
	t0      float64
}

var (
	oldJointNbrs = map[*oldBase][]pt2{}
	plateBorn    = map[string]float64{}
	plateFX      = map[string]int{}
	plateAges    []float64
	plateGone    []goneSlab
	// plateK < 1 draws a slab mid-forge: thin, hot-edged, no bolts or spines yet
	plateK = 1.0
)

func sharesAnchor(g []pt2) bool {
	for _, p := range g {
		if jointGen[p] {
			return true
		}
	}
	return false
}

func parsePlateKey(k string) []pt2 {
	var out []pt2
	for _, s := range strings.Split(k, ";") {
		var x, z float64
		if n, _ := fmt.Sscanf(s, "%f,%f", &x, &z); n == 2 {
			out = append(out, pt2{x, z})
		}
	}
	return out
}

func regrowAt(p pt2) *regrow {
	for _, r := range regrows {
		if dist2(r.pos, p) < 0.05 {
			return r
		}
	}
	return nil
}

// ---- base geometry ----

// baseLift raises a base off the floor while a robot carries it
var baseLift float64

// reattachK >= 0 animates a re-dock: full-size plate, pistons and cables reconnecting
var reattachK = -1.0

func genBaseGeom(anchor, shaftAt pt2, grow float64, torn bool, sink float64, ax vec3, nbrs ...pt2) {
	if grow <= 0.001 {
		return
	}
	up := v3(0, 1, 0)
	c := fl(anchor, baseLift)
	sc := (1 - sink*0.85)
	c.y -= sink * 0.04
	if ax.length() < 0.5 {
		ax = v3(1, 0, 0)
	}
	az := ax.cross(up).norm()
	defer withTex(xf{c, ax, up, az})()
	R := 0.3 * smooth(grow) * sc
	if reattachK >= 0 {
		R = 0.3 * sc
	}
	joint := len(nbrs) > 0
	if !joint {
		useMat(mCast)
		cyl(&bEnv, c, c.add(up.mul(0.028*sc)), R, R*0.97, 36, cIron.scale(1.2), true)
		useMat(mHazard)
		cyl(&bEnv, c.add(up.mul(0.004)), c.add(up.mul(0.022*sc)), R+0.008, R+0.008, 36, rgba{0.8, 0.6, 0.1, 1}, false)
	}
	useMat(mRubber)
	cyl(&bEnv, c.add(up.mul(0.028*sc)), c.add(up.mul(0.034*sc)), R*0.86, R*0.86, 36, rgba{0.05, 0.05, 0.055, 1}, true)
	useMat(mSteel)
	cyl(&bEnv, c.add(up.mul(0.03*sc)), c.add(up.mul(0.036*sc)), R*0.62, R*0.62, 36, cSteelEnv.scale(0.8), true)
	for k := 0; k < 12; k++ {
		a := float64(k) * math.Pi / 6
		d := ax.mul(math.Cos(a)).add(az.mul(math.Sin(a)))
		p := c.add(d.mul(R * 0.74)).add(up.mul(0.03 * sc))
		cyl(&bEnv, p, p.add(up.mul(0.008)), 0.012*sc, 0.012*sc, 6, cSteelEnv, true)
	}
	s := fl(shaftAt, baseLift)
	pose := curPose
	if pose == nil {
		pose = &basePose{reach: slotReaches(anchor, ax, nbrs)}
	}
	for k := 0; k < 6; k++ {
		if slotIsCable(k) {
			continue
		}
		d := slotDir(ax, az, k)
		rad, ps := slotRadius(R*0.92, pose.reach[k])
		ps *= sc
		foot := c.add(d.mul(rad)).add(up.mul(0.03 * sc))
		useMat(mRubber)
		cyl(&bEnv, foot.sub(up.mul(0.006)), foot.add(up.mul(0.02*ps)), 0.04*ps, 0.034*ps, 14, rgba{0.06, 0.06, 0.06, 1}, true)
		useMat(mCast)
		bevelBox(&bEnv, foot.add(up.mul(0.035*ps)), d, up, d.cross(up), 0.03*ps, 0.018*ps, 0.022*ps, 0.006, cIron)
		ball(foot.add(up.mul(0.055*ps)), 0.016*ps, cChromeEnv)
		if torn {
			stub := foot.add(up.mul(0.06 * ps)).add(d.mul(-0.04)).add(up.mul(0.07))
			useMat(mChrome)
			cyl(&bEnv, foot.add(up.mul(0.055*ps)), stub, 0.012, 0.012, 10, cChromeEnv, false)
			useMat(mCast)
			cyl(&bEnv, foot.add(up.mul(0.055*ps)), foot.add(up.mul(0.055*ps)).add(stub.sub(foot).mul(0.45)), 0.022, 0.022, 12, cIron, false)
			continue
		}
		attach := s.add(d.mul(0.105)).add(up.mul(0.24))
		k2 := smooth((grow - 0.25) / 0.6)
		if reattachK >= 0 {
			k2 = smooth(reattachK / 0.6)
		}
		base := foot.add(up.mul(0.055 * ps))
		top := base.add(attach.sub(base).mul(k2))
		if k2 > 0.01 {
			pistonEnv(base, top, 0.012)
		}
	}
	if reattachK >= 0 && reattachK < 0.62 {
		genCables(c, s, ax, az, R, sc, 1, true, pose) // still dangling until they snap back in
	} else {
		genCables(c, s, ax, az, R, sc, grow, torn, pose)
	}
	if torn {
		useMat(mCast)
		cyl(&bEnv, c.add(up.mul(0.035*sc)), c.add(up.mul(0.075*sc)), 0.105*sc, 0.1*sc, 24, cIron, false)
		const n = 18
		for k := 0; k < n; k++ {
			a0 := float64(k) * 2 * math.Pi / n
			a1 := float64(k+1) * 2 * math.Pi / n
			h := (0.03 + 0.05*hash1(float64(k)*3.7+anchor[0]*11)) * sc
			p0 := c.add(ax.mul(0.1 * sc * math.Cos(a0))).add(az.mul(0.1 * sc * math.Sin(a0))).add(up.mul(0.075 * sc))
			p1 := c.add(ax.mul(0.1 * sc * math.Cos(a1))).add(az.mul(0.1 * sc * math.Sin(a1))).add(up.mul(0.075 * sc))
			tip := p0.add(p1).mul(0.5).add(up.mul(h)).add(p0.sub(c).mul(0.15 * (hash1(float64(k)) - 0.3)))
			n0 := p0.sub(c)
			n0.y = 0
			bEnv.triS(p0, p1, tip, n0.norm(), cIron.scale(1.3))
		}
		useMat(mCircuit)
		cyl(&bEnv, c.add(up.mul(0.04*sc)), c.add(up.mul(0.06*sc)), 0.07*sc, 0.07*sc, 18, rgba{0.1, 0.4, 0.15, 1}, true)
	}
}

var cChromeEnv = rgba{0.85, 0.86, 0.88, 1}

func pistonEnv(a, b vec3, r float64) {
	useMat(mCast)
	m := a.add(b.sub(a).mul(0.5))
	cyl(&bEnv, a, m, r*1.7, r*1.7, 12, cIron, true)
	useMat(mChrome)
	cyl(&bEnv, m.sub(b.sub(a).mul(0.05)), b, r, r, 12, cChromeEnv, true)
	useMat(mCast)
	ball(b, r*1.6, cIron)
}

// ---- per-frame drawing for a closed-outline generator ----

func (e *editor) drawGen(i int, p pt2, H float64, prev, next pt2, hot bool, col rgba, fault float64) {
	seedTex(9000 + float64(i)*300)
	anchor, sliding, floating := e.genMode(i)
	rg := regrowAt(p)
	nb := jointNbrs[i]
	pose := basePoseFor(anchor, prev, next, nb) // the base never turns under its own sliding shaft
	ax := pose.ax
	if !sliding && !floating && rg == nil {
		e.buildGenerator(p, H, prev, next, hot, col, fault, anchor, nb)
		return
	}
	t := fxTime()
	lift := 0.0
	if floating {
		lift = 0.12 + 0.015*math.Sin(t*5)
		c := fl(p, lift)
		for k := 0; k < 3; k++ {
			sprite(c.add(v3(0, 0.02, 0)), 0.18, cEnergyHot.alpha(0.25))
		}
		ringSoft(fl(p, 0.01), 0.18+0.03*math.Sin(t*9), 0.03, cEnergy.alpha(0.6), 32)
		beam(c.add(v3(0, 0.03, 0)), fl(p, 0.005), cEnergy, 0.02, 2)
		if rand.Float64() < 0.08 {
			spawnSparks(c.add(v3(0, 0.03, 0)), v3(0, -1, 0), 2, 0.3)
		}
		addPoint(c, cEnergyHot, 2.5, 2.5)
		if !e.isNewGrab(i) {
			severedStubs(fl(p, lift), ax, t)
		}
	}
	if rg != nil && rg.reattach {
		// re-dock: shaft settles, pistons reach back up and lock, cables reconnect one by one
		age := t - rg.t0
		lift = math.Max(0, 0.12*(1-age/0.4))
		reattachK = math.Min(1, math.Max(0, (age-0.3)/1.1))
		genBaseGeom(rg.anchor, p, 1, false, 0, rg.ax, nb...)
		reattachK = -1
		if age < 1.6 {
			k := math.Sin(math.Min(1, age/1.5) * math.Pi)
			beam(fl(rg.anchor, 0.04), fl(p, 0.06+lift), cEnergy, 0.012, 1)
			sprite(fl(p, 0.1), 0.3*k+0.05, cEnergy.alpha(float32(0.4*k)))
			marks := []struct {
				at   float64
				kind int
			}{{0.4, sndClang}, {0.75, sndRelay}, {1.0, sndRelay}, {1.25, sndPowerUp}}
			for mi, m := range marks {
				if rg.fired&(1<<mi) == 0 && age >= m.at {
					rg.fired |= 1 << mi
					e.sfx(m.kind, fl(rg.anchor, 0.1), 0.6)
					if m.kind == sndRelay {
						spawnSparks(fl(rg.anchor, 0.08), v3(0, 1, 0), 6, 0.25)
					}
				}
			}
		}
	} else if rg != nil {
		age := t - rg.t0
		lift = math.Max(0, 0.12*(1-age/0.35))
		grow := smooth((age - 0.2) / 1.2)
		if len(rg.shear) > 0 && grow > 0.55 {
			for _, ob := range rg.shear {
				if !ob.gone && ob.state < 2 {
					e.shear(ob, p)
				}
			}
			rg.shear = nil
		}
		genBaseGeom(p, p, grow, false, 0, rg.ax)
		if age < 1.6 {
			k := math.Sin(math.Min(1, age/1.4) * math.Pi)
			ringSoft(fl(p, 0.01), 0.05+0.32*grow, 0.04, cEnergyHot.alpha(float32(0.8*k)), 40)
			sprite(fl(p, 0.1), 0.4*k+0.05, cEnergy.alpha(float32(0.5*k)))
			addPoint(fl(p, 0.2), cEnergyHot, 6*k, 3)
			if rand.Float64() < 0.5 {
				a := rand.Float64() * 2 * math.Pi
				q := fl(p, 0.03).add(v3(math.Cos(a), 0, math.Sin(a)).mul(0.3 * grow))
				spawnSparks(q, v3(0, 1, 0), 3, 0.3)
			}
			marks := []struct {
				at   float64
				kind int
			}{{0.45, sndLandID}, {0.8, sndHydraulic}, {1.15, sndClang}}
			for mi, m := range marks {
				if rg.fired&(1<<mi) == 0 && age >= m.at {
					rg.fired |= 1 << mi
					e.sfx(m.kind, fl(p, 0.1), 0.7)
				}
			}
		}
	} else if sliding {
		curPose = pose
		defer func() { curPose = nil }()
		d := dist2(p, anchor)
		pl := genPull[e.idxOf(p)]
		cableStrain = math.Min(1, 0.75*smooth(d/edgeDist)+0.25*pl)
		genBaseGeom(anchor, p, 1, false, 0, ax, nb...)
		cableStrain = 0
		strain := math.Max(0, (d-slideDist)/(edgeDist-slideDist)) * 0.5
		if pl > 0 {
			strain = 0.5 + 0.5*pl
		}
		if strain > 0 {
			c := fl(anchor, 0.08)
			sprite(c, 0.12+0.2*strain, cSpark.alpha(float32(0.3*strain)))
			for k := 0; k < 6; k++ {
				if slotIsCable(k) {
					continue
				}
				a := slotAngle(k)
				dd := ax.mul(math.Cos(a)).add(ax.cross(v3(0, 1, 0)).mul(math.Sin(a)))
				sprite(fl(anchor, 0.06).add(dd.mul(0.28)), 0.05, cDanger.alpha(float32(0.8*strain*(0.5+0.5*math.Sin(t*30)))))
			}
		}
		beam(fl(anchor, 0.04), fl(p, 0.06), cEnergy, 0.012, 1)
	}
	genShaftGeom(p, H, prev, next, lift)
	e.genGlow(p, H, prev, next, hot, col, fault, lift)
}

// ---- salvage + wall interlocks + evasion ----

func (e *editor) wallVisible(i int) float64 {
	n := len(e.pts)
	if n != wallKeyN {
		wallKeyN = n
		wallPow = map[int]float64{}
		wallOff = map[int]float64{}
	}
	t := fxTime()
	want := 1.0
	if wallOff[i] > t {
		want = 0
	}
	p, ok := wallPow[i]
	if !ok {
		p = 1
	}
	if want < p {
		if p > 0.99 {
			e.sfx(sndPowerDown, fl(e.pts[i], 1), 0.5)
		}
		p = math.Max(0, p-frameDT*3)
	} else if want > p {
		if p < 0.01 {
			e.sfx(sndPowerUp, fl(e.pts[i], 1), 0.5)
		}
		p = math.Min(1, p+frameDT*1.2)
	}
	wallPow[i] = p
	if p > 0 && p < 1 {
		if noise1(float64(i)*5.1, t*24) > p {
			return 0.05
		}
	}
	return p
}

func (e *editor) wallInterlocks() {
	n := len(e.pts)
	if n < 3 || e.drawing {
		return
	}
	t := fxTime()
	for _, r := range robots {
		for i := 0; i < n; i++ {
			d, _ := segDist(r.pos, e.pts[i], e.pts[(i+1)%n])
			if d < 0.34 {
				wallOff[i] = t + 0.6
			}
		}
	}
}

func (e *editor) movingGen() (pt2, bool) {
	for _, h := range e.hands {
		if h.grab >= 0 && h.grab < len(e.pts) {
			return e.pts[h.grab], true
		}
	}
	return pt2{}, false
}

// returns true when the robot is busy evading this frame
func (e *editor) evade(r *robot, dt float64) bool {
	t := fxTime()
	g, moving := e.movingGen()
	inside := len(e.pts) >= 3 && !e.drawing && pointInPoly(e.pts, r.pos[0], r.pos[1]) && r.state != rbSalvage
	danger := inside
	if moving {
		if dist2(g, r.pos) < 1.8 {
			danger = true
		}
		n := len(e.pts)
		for _, h := range e.hands {
			if h.grab >= 0 && h.grab < n {
				for _, j := range []int{(h.grab - 1 + n) % n, h.grab} {
					if d, _ := segDist(r.pos, e.pts[j], e.pts[(j+1)%n]); d < 0.9 {
						danger = true
					}
				}
			}
		}
	}
	if danger {
		if r.state != rbEvade {
			r.prevState = r.state
			e.sfx(sndSpeech, fl(r.pos, 1.4), 0.4)
		}
		r.state = rbEvade
		r.evadeUntil = t + 1.2
	}
	if r.state != rbEvade {
		return false
	}
	if t > r.evadeUntil && !moving && !inside {
		r.state = rbWalk
		r.path = nil
		switch {
		case r.prevState == rbFire && r.firePh != fpLeave:
			r.state, r.firePh = rbFire, fpTravel
			r.path = e.fireRoute(r.pos, r.fireSpot)
		case r.salv != nil:
			e.planSalvage(r)
		case r.prevState == rbWork && r.gen >= 0 && r.gen < len(e.pts):
			if !e.sendToGen(r) {
				e.sendHome(r)
			}
		default:
			e.sendHome(r)
		}
		return false
	}
	// flee: away from the moving generator, out of the zone, keep clear of walls
	away := v3(r.pos[0], 0, r.pos[1])
	if moving {
		away = away.sub(fl(g, 0))
	} else {
		away = v3(0, 0, 0)
	}
	if len(e.pts) >= 3 {
		best, bq := math.Inf(1), pt2{}
		n := len(e.pts)
		for i := 0; i < n; i++ {
			d, u := segDist(r.pos, e.pts[i], e.pts[(i+1)%n])
			if d < best {
				a, b := e.pts[i], e.pts[(i+1)%n]
				best, bq = d, pt2{a[0] + (b[0]-a[0])*u, a[1] + (b[1]-a[1])*u}
			}
		}
		out := v3(r.pos[0]-bq[0], 0, r.pos[1]-bq[1])
		if inside {
			out = out.mul(-1)
		}
		if out.length() > 1e-6 {
			away = away.add(out.norm().mul(1.5))
		}
	}
	if away.length() < 1e-6 {
		away = v3(math.Cos(r.yaw), 0, math.Sin(r.yaw)).mul(-1)
	}
	away = away.norm()
	ty := math.Atan2(away.z, away.x)
	df := angDiff(r.yaw, ty)
	r.yaw += df * math.Min(1, dt*6)
	sp := 1.7
	if r.kind == kindTread {
		sp = 2.0
	}
	step := sp * math.Max(0.3, math.Cos(df)) * dt
	np := pt2{r.pos[0] + away.x*step, r.pos[1] + away.z*step}
	if world.ok {
		q := world.g.inv().apply(np)
		q[0] = math.Max(world.lx0-1.0, math.Min(world.lx1+1.0, q[0]))
		q[1] = math.Max(world.lz0-1.0, math.Min(world.lz1+1.0, q[1]))
		np = world.g.apply(q)
	}
	r.travel += dist2(np, r.pos)
	r.phase += dist2(np, r.pos) / 0.5 * math.Pi
	r.pos = np
	r.moving += (1 - r.moving) * math.Min(1, dt*5)
	return true
}

const cableSlack = 0.22

// 0..1 while a shaft is dragged on its mount: the tethers draw in and go tight before they snap
var cableStrain float64

func cablePoints(c, s, ax, az vec3, R, sc float64, k int) (vec3, vec3, vec3) {
	return cablePointsR(c, s, ax, az, R*0.78, sc, slotAngle(cableSlots[k]))
}

func cablePointsR(c, s, ax, az vec3, rad, sc, a float64) (vec3, vec3, vec3) {
	up := v3(0, 1, 0)
	d := ax.mul(math.Cos(a)).add(az.mul(math.Sin(a)))
	plug := c.add(d.mul(rad)).add(up.mul(0.036 * sc))
	drum := s.add(d.mul(0.118)).add(up.mul(0.2))
	return plug, drum, d
}

func cableRest(R, sc float64) float64 {
	c0 := v3(0, 0, 0)
	p, q, _ := cablePoints(c0, c0, v3(1, 0, 0), v3(0, 0, 1), R, sc, 0)
	return p.sub(q).length() + cableSlack
}

func genCables(c, s, ax, az vec3, R, sc, grow float64, torn bool, pose *basePose) {
	up := v3(0, 1, 0)
	L0 := cableRest(0.3, 1)
	for k, slot := range cableSlots {
		rad, ps := slotRadius(R*0.78, pose.reach[slot])
		plug, drum, d := cablePointsR(c, s, ax, az, rad, sc, slotAngle(slot))
		ps *= sc
		useMat(mCast)
		bevelBox(&bEnv, plug.add(up.mul(0.018)), d, up, d.cross(up), 0.03*ps, 0.018*ps, 0.026*ps, 0.006, cIron)
		useMat(mHazard)
		cyl(&bEnv, plug.add(up.mul(0.03*ps)), plug.add(up.mul(0.06*ps)), 0.022*ps, 0.022*ps, 12, rgba{0.8, 0.6, 0.1, 1}, true)
		cr := 0.019 * sc
		if torn {
			// a snapped stub flopping out of the plug, frayed copper at the end
			end := plug.add(up.mul(0.12)).add(d.mul(0.1)).add(d.cross(up).mul(0.05 * math.Sin(float64(k)*2.1)))
			hose(&bEnv, plug.add(up.mul(0.06*ps)), plug.add(up.mul(0.14)), end.add(d.mul(-0.02)), end, cr, rgba{0.04, 0.04, 0.045, 1}, 6)
			useMat(material{1, 1, 0.35, 0.3})
			for w := 0; w < 5; w++ {
				a := float64(w) * 1.3
				tip := end.add(d.mul(0.03)).add(v3(math.Cos(a), 0.4, math.Sin(a)).mul(0.025))
				cyl(&bEnv, end, tip, 0.003, 0.0015, 4, rgba{0.85, 0.5, 0.25, 1}, false)
			}
			if rand.Float64() < frameDT*0.25 {
				spawnSparks(end, d, 3, 0.2)
			}
			continue
		}
		if grow < 0.7 {
			continue
		}
		useMat(mCast)
		bevelBox(&bEnv, drum, d, up, d.cross(up), 0.016, 0.026, 0.02, 0.005, cIron)
		chord := drum.sub(plug).length()
		taut := math.Min(1, chord/L0)
		sag := math.Sqrt(math.Max(0, L0*L0-chord*chord)) * 0.5
		sag *= 1 - 0.85*cableStrain
		if cableStrain > 0.5 {
			taut = math.Max(taut, 0.86+0.14*(cableStrain-0.5)/0.5)
		}
		out := d.mul(sag * 0.9)
		ca := plug.add(up.mul(0.06 + sag*0.2)).add(out)
		cb := drum.add(out.mul(1.1)).sub(up.mul(sag * 0.6))
		if cableStrain > 0.3 {
			// a tight line hums: a small fast quiver in the middle
			j := 0.004 * (cableStrain - 0.3) * math.Sin(fxTime()*70+float64(k)*1.9)
			sd := d.cross(up)
			ca = ca.add(sd.mul(j))
			cb = cb.sub(sd.mul(j))
		}
		hose(&bEnv, plug.add(up.mul(0.06*ps)), ca, cb, drum.add(d.mul(0.012)), cr, rgba{0.04, 0.04, 0.045, 1}, 12)
		for b := 0.25; b < 0.8; b += 0.27 {
			q := bezier(plug.add(up.mul(0.06*ps)), ca, cb, drum, b)
			q2 := bezier(plug.add(up.mul(0.06*ps)), ca, cb, drum, b+0.03)
			useMat(mHazard)
			cyl(&bEnv, q, q2, cr*1.12, cr*1.12, 10, rgba{0.8, 0.6, 0.1, 1}, false)
		}
		if taut > 0.86 {
			k2 := (taut - 0.86) / 0.14
			sprite(drum, 0.03+0.04*k2, cSpark.alpha(float32(0.8*k2)))
			sprite(plug.add(up.mul(0.05)), 0.03+0.04*k2, cSpark.alpha(float32(0.8*k2)))
		}
	}
}

// dangling severed cable ends under a ripped, floating shaft
func severedStubs(s vec3, ax vec3, t float64) {
	up := v3(0, 1, 0)
	az := ax.cross(up).norm()
	for k := range cableSlots {
		a := slotAngle(cableSlots[k])
		d := ax.mul(math.Cos(a)).add(az.mul(math.Sin(a)))
		top := s.add(d.mul(0.118)).add(up.mul(0.2))
		sw := 0.03 * math.Sin(t*3+float64(k)*2)
		end := top.add(up.mul(-0.2)).add(d.mul(0.05 + sw)).add(d.cross(up).mul(sw))
		hose(&bEnv, top, top.add(d.mul(0.04)), end.add(up.mul(0.06)), end, 0.019, rgba{0.04, 0.04, 0.045, 1}, 6)
		if rand.Float64() < 0.05 {
			spawnSparks(end, up.mul(-1), 2, 0.2)
		}
		sprite(end, 0.025, cSpark.alpha(float32(0.4+0.4*rand.Float64())))
	}
}

// ---- shared plates for generators standing close together ----

var (
	plateCache     = map[string][]float32{}
	plateFrameKeys []string
	platePrevKeys  []string
)

func plateKey(a []pt2) string {
	b := make([]byte, 0, len(a)*24)
	for _, p := range a {
		b = append(b, []byte(fmt.Sprintf("%.4f,%.4f;", p[0], p[1]))...)
	}
	return string(b)
}

func (e *editor) drawPlates() {
	t := fxTime()
	for pi, pl := range plates {
		k := plateKey(pl)
		if age := plateAges[pi]; age < plateForm {
			e.formPlate(k, pl, age)
			continue
		}
		if _, ok := plateCache[k]; !ok {
			start := len(bEnv.v)
			seedTex(40000 + pl[0][0]*97 + pl[0][1]*31)
			genPlateGeom(pl)
			plateCache[k] = append([]float32(nil), bEnv.v[start:]...)
			bEnv.v = bEnv.v[:start]
			if len(plateCache) > 64 {
				for kk := range plateCache {
					if kk != k {
						delete(plateCache, kk)
						break
					}
				}
			}
		}
		plateFrameKeys = append(plateFrameKeys, k)
		for _, a := range pl {
			shadowBlob(a, 0.32, 0.3)
		}
	}
	// slabs retracting after their generators were pulled apart
	live := plateGone[:0]
	for _, g := range plateGone {
		age := t - g.t0
		if age > 0.7 {
			continue
		}
		live = append(live, g)
		plateK = 1 - smooth(age/0.7)
		seedTex(40000 + g.anchors[0][0]*97 + g.anchors[0][1]*31)
		genPlateGeom(g.anchors)
		plateK = 1
	}
	plateGone = live
}

// formPlate: the shared slab forges itself under neighbouring generators - welding arcs trace
// the outline, the slab swells up out of the deck glowing hot and cools, then the spines and
// bolts lock in as the generators' legs re-route onto it
func (e *editor) formPlate(key string, pl []pt2, age float64) {
	k := smooth(math.Min(1, (age-0.15)/0.8))
	plateK = math.Max(0.06, k)
	seedTex(40000 + pl[0][0]*97 + pl[0][1]*31)
	genPlateGeom(pl)
	plateK = 1
	var samp []pt2
	var c pt2
	for _, a := range pl {
		c[0] += a[0] / float64(len(pl))
		c[1] += a[1] / float64(len(pl))
		for s := 0; s < 40; s++ {
			th := float64(s) * 2 * math.Pi / 40
			samp = append(samp, pt2{a[0] + 0.315*math.Cos(th), a[1] + 0.315*math.Sin(th)})
		}
	}
	hull := convexHull(samp)
	n := len(hull)
	y := 0.004 + 0.024*plateK + 0.002
	// the outline being traced (first 0.45 s), then a hot rim that cools
	trace := math.Min(1, age/0.45)
	heat := math.Max(0, 1-(age-0.3)/1.0)
	for i := 0; i < int(float64(n)*trace); i++ {
		a, b := fl(hull[i], y), fl(hull[(i+1)%n], y)
		col := heatColor(0.35 + 0.6*heat)
		ribbon(a, b, 0.012, col.alpha(float32(0.25+0.6*heat)))
		bGlowLine.line(a, b, col.alpha(float32(0.4+0.6*heat)))
	}
	if trace < 1 {
		i := int(float64(n) * trace)
		tip := fl(hull[i%n], y)
		flare(tip, v3(0, 1, 0), 0.04, cSpark)
		addPoint(tip.add(v3(0, 0.1, 0)), cSpark, 4, 3)
		if rand.Float64() < 0.5 {
			spawnSparks(tip, v3(0, 1, 0), 2, 0.4)
		}
	}
	if heat > 0 {
		addPoint(fl(c, 0.12), heatColor(0.6), 3*heat, 2.5)
	}
	// steam as it cools, sounds at each stage
	if rand.Float64() < frameDT*10*heat {
		h := hull[rand.Intn(n)]
		spawnSteam(fl(h, 0.04), v3(0, 1, 0), 1)
	}
	marks := []struct {
		at   float64
		kind int
	}{{0.0, sndZip}, {0.2, sndGrind}, {0.6, sndSteam}, {plateSwap, sndHydraulic}, {plateSwap + 0.15, sndClang}}
	for mi, m := range marks {
		if plateFX[key]&(1<<mi) == 0 && age >= m.at {
			plateFX[key] |= 1 << mi
			e.sfx(m.kind, fl(c, 0.1), 0.45)
			if m.kind == sndHydraulic {
				for _, a := range pl {
					spawnSteam(fl(a, 0.08), v3(0, 1, 0), 6)
					spawnSparks(fl(a, 0.06), v3(0, 1, 0), 8, 0.3)
				}
			}
		}
	}
	for _, a := range pl {
		shadowBlob(a, 0.32, 0.3)
	}
}

func convexHull(pts []pt2) []pt2 {
	sort.Slice(pts, func(i, j int) bool {
		if pts[i][0] != pts[j][0] {
			return pts[i][0] < pts[j][0]
		}
		return pts[i][1] < pts[j][1]
	})
	cross := func(o, a, b pt2) float64 { return (a[0]-o[0])*(b[1]-o[1]) - (a[1]-o[1])*(b[0]-o[0]) }
	var h []pt2
	for _, p := range pts {
		for len(h) >= 2 && cross(h[len(h)-2], h[len(h)-1], p) <= 0 {
			h = h[:len(h)-1]
		}
		h = append(h, p)
	}
	lo := len(h) + 1
	for i := len(pts) - 2; i >= 0; i-- {
		p := pts[i]
		for len(h) >= lo && cross(h[len(h)-2], h[len(h)-1], p) <= 0 {
			h = h[:len(h)-1]
		}
		h = append(h, p)
	}
	return h[:len(h)-1]
}

func genPlateGeom(anchors []pt2) {
	up := v3(0, 1, 0)
	var samp []pt2
	var cx, cz float64
	for _, a := range anchors {
		cx += a[0] / float64(len(anchors))
		cz += a[1] / float64(len(anchors))
		for k := 0; k < 40; k++ {
			t := float64(k) * 2 * math.Pi / 40
			samp = append(samp, pt2{a[0] + 0.31*math.Cos(t), a[1] + 0.31*math.Sin(t)})
		}
	}
	hull := convexHull(samp)
	n := len(hull)
	c := v3(cx, 0, cz)
	h0, h1, lip := 0.0, 0.004+0.024*plateK, 0.012*math.Min(1, plateK*1.5)
	inset := func(p pt2, d float64) pt2 {
		v := pt2{p[0] - cx, p[1] - cz}
		l := math.Hypot(v[0], v[1])
		return pt2{p[0] - v[0]/l*d, p[1] - v[1]/l*d}
	}
	useMat(mCast)
	for i := 0; i < n; i++ {
		a, b := hull[i], hull[(i+1)%n]
		ai, bi := inset(a, lip), inset(b, lip)
		bEnv.triS(fl(ai, h1), fl(bi, h1), c.add(up.mul(h1)), up, cIron.scale(1.2))
		nrm := v3(b[1]-a[1], 0, -(b[0] - a[0])).norm()
		if nrm.dot(fl(a, 0).sub(c)) < 0 {
			nrm = nrm.mul(-1)
		}
		sl := nrm.add(up).norm()
		bEnv.quadF(fl(a, h1-lip*0.6), fl(b, h1-lip*0.6), fl(bi, h1), fl(ai, h1), sl, cIron.scale(1.2))
		bEnv.quadF(fl(a, h0), fl(b, h0), fl(b, h1-lip*0.6), fl(a, h1-lip*0.6), nrm, cIron.scale(1.1))
		useMat(mHazard)
		o := pt2{nrm.x * 0.004, nrm.z * 0.004}
		ao, bo := pt2{a[0] + o[0], a[1] + o[1]}, pt2{b[0] + o[0], b[1] + o[1]}
		bEnv.quadF(fl(ao, 0.004), fl(bo, 0.004), fl(bo, 0.016), fl(ao, 0.016), nrm, rgba{0.8, 0.6, 0.1, 1})
		useMat(mCast)
	}
	if plateK < 0.999 {
		return
	}
	// bolts around the rim
	useMat(mSteel)
	per := 0.0
	for i := 0; i < n; i++ {
		a, b := hull[i], hull[(i+1)%n]
		L := dist2(a, b)
		for s := math.Mod(0.12-per, 0.12); s < L; s += 0.12 {
			p := inset(pt2{a[0] + (b[0]-a[0])*s/L, a[1] + (b[1]-a[1])*s/L}, 0.04)
			bp := fl(p, h1)
			cyl(&bEnv, bp, bp.add(up.mul(0.008)), 0.011, 0.011, 6, cSteelEnv, true)
		}
		per = math.Mod(per+L, 0.12)
	}
	// spine housings + conduits along a minimum spanning tree between sockets
	inTree := []int{0}
	used := map[int]bool{0: true}
	for len(inTree) < len(anchors) {
		bi, bj, bd := -1, -1, math.Inf(1)
		for _, i := range inTree {
			for j := range anchors {
				if used[j] {
					continue
				}
				if d := dist2(anchors[i], anchors[j]); d < bd {
					bi, bj, bd = i, j, d
				}
			}
		}
		used[bj] = true
		inTree = append(inTree, bj)
		a, b := fl(anchors[bi], 0), fl(anchors[bj], 0)
		d := b.sub(a)
		L := d.length()
		if L < 0.22 {
			continue
		}
		dn := d.mul(1 / L)
		sd := dn.cross(up).norm()
		p0, p1 := a.add(dn.mul(0.13)), b.sub(dn.mul(0.13))
		mid := p0.add(p1).mul(0.5)
		hl := p1.sub(p0).length() / 2
		useMat(mCast)
		bevelBox(&bEnv, mid.add(up.mul(h1+0.018)), sd, up, dn, 0.05, 0.018, hl, 0.008, cIron)
		for s := -hl + 0.03; s < hl; s += 0.035 {
			bevelBox(&bEnv, mid.add(up.mul(h1+0.037)).add(dn.mul(s)), sd, up, dn, 0.04, 0.003, 0.006, 0.001, cIron.scale(1.4))
		}
		curEmis = 2
		gl := mid.add(up.mul(h1 + 0.0365))
		bEnv.quad(gl.add(sd.mul(-0.03)).add(dn.mul(-hl*0.9)), gl.add(sd.mul(0.03)).add(dn.mul(-hl*0.9)), gl.add(sd.mul(0.03)).add(dn.mul(hl*0.9)), gl.add(sd.mul(-0.03)).add(dn.mul(hl*0.9)), rgba{0.15, 0.5, 0.22, 1})
		curEmis = 1
		for _, s := range []float64{-1, 1} {
			q0 := p0.add(sd.mul(s * 0.075)).add(up.mul(h1 + 0.012))
			q1 := p1.add(sd.mul(s * 0.075)).add(up.mul(h1 + 0.012))
			hose(&bEnv, q0, q0.add(dn.mul(hl*0.6)).add(up.mul(0.02)), q1.sub(dn.mul(hl*0.6)).add(up.mul(0.02)), q1, 0.013, rgba{0.04, 0.04, 0.045, 1}, 8)
		}
	}
}

// ---- base slot layout: six slots 60 deg apart, four hydraulic legs and two tethers ----
// slots sit at 30+60k deg from the base axis; the layout is symmetric under a half turn.

var cableSlots = [2]int{1, 4} // 90 and 270 deg

func slotIsCable(k int) bool { return k == 1 || k == 4 }

func slotAngle(k int) float64 { return math.Pi/6 + float64(k)*math.Pi/3 }

func slotDir(ax, az vec3, k int) vec3 {
	a := slotAngle(k)
	return ax.mul(math.Cos(a)).add(az.mul(math.Sin(a)))
}

// basePose: the (smoothed) axis of a planted base and how far each slot may reach out
// before it would run into a joined neighbour's hardware
type basePose struct {
	ax    vec3
	reach [6]float64
	t     float64
	ang   float64
}

var (
	basePoses = map[pt2]*basePose{}
	curPose   *basePose
)

// slotReaches: fraction of the nominal radius each slot can use without crossing the
// half-way line to a neighbour (continuous in the axis, so nothing ever pops)
func slotReaches(anchor pt2, ax vec3, nbrs []pt2) [6]float64 {
	var r [6]float64
	az := ax.cross(v3(0, 1, 0)).norm()
	for k := 0; k < 6; k++ {
		d := slotDir(ax, az, k)
		nom := 0.3 * 0.92
		if slotIsCable(k) {
			nom = 0.3 * 0.78
		}
		f := 1.0
		for _, nb := range nbrs {
			v := v3(nb[0]-anchor[0], 0, nb[1]-anchor[1])
			D := v.length()
			if D < 1e-6 {
				continue
			}
			cs := d.dot(v.mul(1 / D))
			if cs > 0.02 {
				f = math.Min(f, (D/2-0.05)/cs/nom)
			}
		}
		r[k] = math.Max(0, f)
	}
	return r
}

// slotRadius: radius and part scale for a slot; parts squeezed against a neighbour tuck in
// toward the shaft and shrink a little rather than vanish
func slotRadius(nom, f float64) (float64, float64) {
	const minF = 0.55
	ps := 1.0
	if f < minF {
		ps = math.Max(0.45, f/minF)
		f = minF
	}
	return nom * math.Min(1, f), ps
}

// basePoseFor: the base turns smoothly toward its best heading (and its parts slide in or
// out smoothly) when a neighbour moves, instead of snapping
func basePoseFor(anchor, prev, next pt2, nbrs []pt2) *basePose {
	t := fxTime()
	tgt := baseAxisFor(anchor, prev, next, nbrs...)
	ta := math.Atan2(tgt.z, tgt.x)
	p := basePoses[anchor]
	if p == nil {
		p = &basePose{ang: ta, t: t}
		p.ax = tgt
		p.reach = slotReaches(anchor, tgt, nbrs)
		basePoses[anchor] = p
		if len(basePoses) > 160 {
			for k, v := range basePoses {
				if t-v.t > 5 {
					delete(basePoses, k)
				}
			}
		}
		return p
	}
	if t > p.t {
		dt := math.Min(0.1, t-p.t)
		p.t = t
		diff := math.Mod(ta-p.ang, math.Pi)
		if diff > math.Pi/2 {
			diff -= math.Pi
		} else if diff < -math.Pi/2 {
			diff += math.Pi
		}
		step := diff * math.Min(1, dt*5)
		lim := dt * 2.5
		step = math.Max(-lim, math.Min(lim, step))
		p.ang += step
		p.ax = v3(math.Cos(p.ang), 0, math.Sin(p.ang))
		want := slotReaches(anchor, p.ax, nbrs)
		for k := range want {
			p.reach[k] += (want[k] - p.reach[k]) * math.Min(1, dt*6)
		}
	}
	return p
}

// poseKey: quantised pose, part of the static-geometry cache key
func (p *basePose) key() [4]int32 {
	var k [4]int32
	k[0] = int32(math.Round(p.ang * 2000))
	for i := 0; i < 6; i++ {
		k[1+i%3] = k[1+i%3]*131 + int32(math.Round(p.reach[i]*400))
	}
	return k
}
