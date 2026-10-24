package main

// The maze's base stations. Every magenta wall stands on a base station at each end, built
// and animated like your generators, just burning magenta: when you draw a wall, a pod
// flies out of your hand to each end, lands, unfolds into its base, the shaft telescopes
// up, the floor blades swing out and the emitter flickers on - and only then does the
// sheet light between them. Take a wall down and its stations fold their blades away and
// sink back into the deck. (On a restart the pods drop from the ceiling instead.)

import (
	"math"
	"math/rand"
	"sort"
)

type mazePost struct {
	p       pt2
	from    vec3    // where its pod is launched from
	dep     deploy  // its pod and deploy animation
	vis     float64 // how lit its emitter is (it flickers on)
	nbs     []pt2   // the posts next to it along its walls
	pending bool    // waiting its turn to go up
	order   int     // ...and its place in the queue
	born    float64 // when it went up
	gone    float64 // when its last wall came down (0 while it stands)
	ext     float64 // floor blades out (0..1)
	retract float64 // its walls being taken down by the trigger (0..1; shared stations stay)
	fired   int     // animation sounds already played
	seen    bool
}

// Stations go up one after another along a wall, a beat apart, and only a few animate at
// once (a growing station is built fresh every frame), so a whole maze coming back on a
// restart ripples up over a few seconds instead of stalling the frame.
const (
	mazeBirthGap   = 0.07
	mazeMaxGrowing = 8
)

// busy: still animating (its geometry is built every frame)
func (m *mazePost) busy(t float64) bool {
	return !m.pending && (m.gone > 0 || t-m.born < 1.75 || m.ext < 0.999)
}

// ready: how far its sheet can be lit (blades nearly out)
func (m *mazePost) ready() float64 {
	if m.gone > 0 || m.pending {
		return 0
	}
	return math.Min(m.vis, clamp01((m.ext-0.6)/0.4))
}

func clamp01(x float64) float64 { return math.Max(0, math.Min(1, x)) }

func mazePostKey(p pt2) [2]int32 {
	return [2]int32{int32(math.Round(p[0] * 50)), int32(math.Round(p[1] * 50))}
}

// wallPostPts: where a wall's stations stand: one at each end.
func (g *game) wallPostPts(a, b pt2) []pt2 { return []pt2{a, b} }

type mshaftKey struct {
	p, prev, next pt2
	H             float64
}

var (
	mazeDrawing bool       // the generator drawing code is drawing a maze station
	mazeSidePow [2]float64 // ...and how lit its two emitter sides are
)

// drawMazePosts: runs between beginGenFrame and endGenFrame (the stations' static geometry
// joins the cached generator buffer).
func (g *game) drawMazePosts() {
	r := g.r
	t := fxTime()
	H := r.T.WallHeight
	if g.mposts == nil {
		g.mposts = map[[2]int32]*mazePost{}
	}
	for _, mp := range g.mposts {
		mp.seen = false
	}
	for _, w := range r.Maze {
		pts := g.wallPostPts(pv(w.A), pv(w.B))
		rk := g.retract[w.ID]
		for i, p := range pts {
			key := mazePostKey(p)
			mp := g.mposts[key]
			if mp == nil || mp.gone > 0 {
				g.mpostSeq++
				mp = &mazePost{p: p, pending: true, order: g.mpostSeq, from: fl(p, envH-0.4)}
				if g.wallFromT > 0 && t-g.wallFromT < 0.5 {
					mp.from = g.wallFrom // the wall you just drew: out of your hand
				}
				g.mposts[key] = mp
			}
			if !mp.seen {
				mp.seen, mp.nbs, mp.retract = true, mp.nbs[:0], rk
			}
			mp.retract = math.Min(mp.retract, rk) // a station another wall still uses stays up
			for _, j := range []int{i - 1, i + 1} {
				if j >= 0 && j < len(pts) {
					mp.nbs = append(mp.nbs, pts[j])
				}
			}
		}
	}
	busy := 0
	var wait []*mazePost
	for key, mp := range g.mposts {
		switch {
		case !mp.seen && mp.pending:
			delete(g.mposts, key) // its wall came down before it went up
			continue
		case !mp.seen && mp.gone == 0 && mp.retract > 0.98:
			delete(g.mposts, key) // folded all the way away already
			continue
		case !mp.seen && mp.gone == 0:
			mp.gone = t
			ed.sfx(sndHydraulic, fl(mp.p, 0.3), 0.35)
		case mp.gone > 0 && t-mp.gone > 1.0:
			delete(g.mposts, key)
			continue
		}
		if mp.pending {
			wait = append(wait, mp)
		} else if mp.busy(t) {
			busy++
		}
	}
	sort.Slice(wait, func(i, j int) bool { return wait[i].order < wait[j].order })
	for _, mp := range wait {
		if busy >= mazeMaxGrowing || t-g.mpostLast < mazeBirthGap {
			break
		}
		mp.pending, mp.born = false, t
		mp.dep = deploy{pos: mp.p, from: mp.from, t0: t}
		ed.sfx(sndEject, mp.from, 0.5)
		g.mpostLast = t
		busy++
	}
	for _, mp := range g.mposts {
		if !mp.pending {
			g.drawMazePost(mp, H, t)
		}
	}
	glowCol, glowHot, glowLite, mazeDrawing = cEnergy, cEnergyHot, false, false
}

func (g *game) drawMazePost(mp *mazePost, H, t float64) {
	p := mp.p
	if len(mp.nbs) == 0 {
		return
	}
	prev := mp.nbs[0]
	next := pt2{2*p[0] - prev[0], 2*p[1] - prev[1]} // an end station: its back emitter faces nothing
	pw := [2]float64{1, 0}
	if len(mp.nbs) > 1 {
		next = mp.nbs[1]
		pw[1] = 1
	}
	age := t - mp.born
	// the pod's sounds: lands, opens, telescopes, ignites
	steps := []struct {
		at   float64
		kind int
		amp  float64
	}{{0.45, sndLand, 0.6}, {0.47, sndPulse, 0.5}, {0.9, sndTelescope, 0.5}, {1.7, sndIgnite, 0.5}}
	for i, s := range steps {
		if mp.gone == 0 && mp.fired&(1<<i) == 0 && age >= s.at {
			mp.fired |= 1 << i
			ed.sfx(s.kind, fl(p, 0.1), s.amp)
			if s.kind == sndLand {
				spawnSparks(fl(p, 0.02), v3(0, 1, 0), 14, 0.2)
			}
		}
	}
	// blades out once the shaft is up; in again when it's coming down
	rk := mp.retract
	switch {
	case rk > 0 && mp.gone == 0 && age > 1.15:
		// being taken down: blades fold in as the trigger's held (and out again if let go)
		mp.ext = math.Min(mp.ext, 1-smooth(clamp01((rk-0.25)/0.35)))
		if rk < 0.25 {
			mp.ext = math.Min(1, mp.ext+frameDT*2.2)
		}
	case mp.gone > 0:
		mp.ext = math.Max(0, mp.ext-frameDT*3.5)
	case age > 1.15:
		was := mp.ext
		mp.ext = math.Min(1, mp.ext+frameDT*2.2)
		if was < 0.02 && mp.ext >= 0.02 {
			ed.sfx(sndTelescopeID, fl(p, 0.3), 0.3)
		}
	}
	// the emitter flickers on after ignition, like a fresh generator's
	switch {
	case mp.gone > 0 || age < 1.7:
		mp.vis = 0
	case age < 2.4:
		s := (age - 1.7) / 0.7
		mp.vis = 0
		if noise1(mp.born*3.7, age*14) <= 0.25+0.6*s {
			mp.vis = 0.5 + 0.5*rand.Float64()
		}
	default:
		mp.vis = 1
	}
	mp.vis *= 1 - clamp01(rk/0.35)
	k := mp.ready()
	mazeDrawing = true
	mazeSidePow = [2]float64{pw[0] * (0.15 + 0.85*k), pw[1] * (0.15 + 0.85*k)}
	glowCol, glowHot = cMagenta, cMagentaHot
	cam := ed.hmd.pos()
	glowLite = cam.sub(fl(p, 1)).length() > 4.5
	col := cMagenta.scale(float32((0.35 + 0.65*k) * mp.vis))
	ax := basePoseFor(p, prev, next, nil).ax
	// the same texture seeds as the finished (cached) station
	primCounter = math.Floor(p[0]*131+p[1]*71) + 5000
	switch {
	case mp.gone > 0:
		// blades away first, then the whole station sinks into the deck
		kk := t - mp.gone
		sink := smooth(clamp01((kk - 0.3) / 0.6))
		if sink < 0.999 {
			genBaseGeom(p, p, 1-sink, false, 0, ax)
			if sink == 0 {
				cachedChunk(mshaftKey{p, prev, next, H}, func() { genShaftGeom(p, H, prev, next, 0) })
			} else {
				genShaftGeom(p, H, prev, next, -sink*(H+0.2))
			}
		}
	case rk > 0.55 && age >= 1.7:
		// the last of taking it down: the station sinks into the deck
		sink := smooth(clamp01((rk - 0.55) / 0.45))
		if sink < 0.999 {
			genBaseGeom(p, p, 1-sink, false, 0, ax)
			genShaftGeom(p, H, prev, next, -sink*(H+0.2))
		}
	case age < 1.7:
		// the pod: flies in, lands, unfolds, the shaft telescopes up
		ed.drawDeploy(&mp.dep, p, H, prev, next)
	default:
		ed.buildGenerator(p, H, prev, next, false, col, 0, p, nil)
	}
	if mp.gone > 0 || age > 1.15 {
		g.mazeBlades(mp, prev, next, col)
	}
}

// mazeBlades: a station's diagonal floor blades, cached when they're all the way in or out.
func (g *game) mazeBlades(mp *mazePost, prev, next pt2, col rgba) {
	ext := mp.ext
	p := mp.p
	if (ext < 0.005 || ext > 0.999) && mp.gone == 0 {
		key := extKey{p: p, prev: prev, next: next, out: ext > 0.999, fyA: floorGap, fyB: floorGap}
		if _, ok := extCache[key]; !ok {
			start := len(bEnv.v)
			bxFX = false
			ed.bladeExtDraw(-1, p, prev, next, col, ext)
			bxFX = true
			extCache[key] = append([]float32(nil), bEnv.v[start:]...)
			bEnv.v = bEnv.v[:start]
		}
		extFrameKeys = append(extFrameKeys, key)
		bxGeom = false
		ed.bladeExtDraw(-1, p, prev, next, col, ext)
		bxGeom = true
		return
	}
	if ext > 0.005 {
		ed.bladeExtDraw(-1, p, prev, next, col, ext)
	}
}
