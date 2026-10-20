//go:build windows

package main

import (
	"math"
	"math/rand"
)

// Pushing a post into the cell walls: the wall resists first (the post slows to a stop a
// hand's width short of it, the controller buzzes harder, the wall lights amber), and if you
// keep pushing it gives way: the whole wall slides back on its rails to make room.
//
// The slide is drawn by the vertex shaders: the room is rebuilt at its new size straight away
// and, for the length of the slide, everything near the moving wall is mapped back toward its
// old place (see slideMap, mirrored in lit.vs and shadow.vs).

type wallSlide struct {
	old, new [4]float64 // wall coordinates x0, x1, z0, z1 in the room frame
	t0       float64
	on       bool
	side     int
	clanked  bool
}

var (
	slide     wallSlide
	strainT   float64
	wallPushK [4]float64 // how hard each wall is being pushed this frame, for its glow
	wallPushP [4]pt2     // where, in the room frame
)

func wallsOf(b [4]float64) [4]float64 {
	return [4]float64{b[0] - wallMargin, b[1] + wallMargin, b[2] - wallMargin, b[3] + wallMargin}
}

func slideK() float64 {
	if !slide.on {
		return 1
	}
	return smooth((fxTime() - slide.t0) / slideDur)
}

func slideActive() bool { return slide.on && fxTime()-slide.t0 < slideDur }

// slideMap: where a point of the (already rebuilt) room is drawn during a wall slide
func slideMap(p vec3) vec3 {
	if !slideActive() {
		return p
	}
	k := slideK()
	inv := envG.inv()
	q := inv.apply(pt2{p.x, p.z})
	x := slideEdge(q[0], slide.old[1], slide.new[1], 1, k)
	x = slideEdge(x, slide.old[0], slide.new[0], -1, k)
	z := slideEdge(q[1], slide.old[3], slide.new[3], 1, k)
	z = slideEdge(z, slide.old[2], slide.new[2], -1, k)
	w := envG.apply(pt2{x, z})
	return v3(w[0], p.y, w[1])
}

// pushWall moves wall i (0 x0, 1 x1, 2 z0, 3 z1) so the boundary box edge sits at v
func (e *editor) pushWall(i int, v float64) {
	if slideActive() {
		// already moving: carry on from where it is drawn now
		k := slideK()
		for j := 0; j < 4; j++ {
			slide.old[j] = slide.old[j] + (slide.new[j]-slide.old[j])*k
		}
	} else {
		slide.old = wallsOf(envBox)
	}
	envBox[i] = v
	slide.new = wallsOf(envBox)
	slide.t0, slide.on, slide.side, slide.clanked = fxTime(), true, i, false
	c := e.wallPoint(i, wallPushP[i], slide.old)
	e.sfx(sndHydraulic, fl(c, 1.2), 1)
	e.sfx(sndCreak, fl(c, 0.3), 0.8)
	e.sfx(sndServo, fl(c, 2.5), 0.6)
}

// wallPoint: the world point on wall i (wall coordinates w) nearest the room-frame point q
func (e *editor) wallPoint(i int, q pt2, w [4]float64) pt2 {
	switch i {
	case 0, 1:
		q[0] = w[i]
	default:
		q[1] = w[i]
	}
	return envG.apply(q)
}

// wallPush: a dragged post's target position, held off the walls with a soft stop; pushing
// hard enough moves the wall
func (e *editor) wallPush(h *hand, p pt2) pt2 {
	if !envHave {
		return p
	}
	q := envG.inv().apply(p)
	for i := 0; i < 4; i++ {
		ax, sgn := i/2, 1.0
		if i%2 == 0 {
			sgn = -1
		}
		stop := (wallsOf(envBox)[i] - sgn*wallClear) * sgn // in the push direction
		u := q[ax] * sgn
		over := u - stop
		if over <= 0 {
			continue
		}
		if over > breakPush {
			// it gives: the wall slides back far enough for the post plus a bit
			nb := (u + wallClear + 0.4 - wallMargin) * sgn
			if sgn > 0 {
				nb = math.Ceil(nb*2) / 2
			} else {
				nb = math.Floor(nb*2) / 2
			}
			wallPushP[i] = q
			e.pushWall(i, nb)
			hapticAmp(h.idx, 160, 1)
			continue
		}
		k := over / breakPush
		q[ax] = (stop + resistR*(1-math.Exp(-over/resistR))) * sgn
		wallPushK[i] = math.Max(wallPushK[i], k)
		wallPushP[i] = q
		t := fxTime()
		if t > h.gBuzz {
			h.gBuzz = t + 0.1 - 0.06*k
			hapticAmp(h.idx, 20+int(40*k), float32(0.2+0.5*k))
		}
		if t > strainT {
			strainT = t + 0.9 - 0.5*k
			e.sfx(sndCreak, fl(e.wallPoint(i, q, wallsOf(envBox)), 0.4), 0.3+0.4*k)
		}
	}
	return envG.apply(q)
}

// drawWallPush: the amber strain glow where a post is being shoved at a wall, and the steam
// and clank of a wall sliding on its rails
func (e *editor) drawWallPush() {
	t := fxTime()
	for i := 0; i < 4; i++ {
		k := wallPushK[i]
		wallPushK[i] = 0
		if k <= 0 || !envHave {
			continue
		}
		c := e.wallPoint(i, wallPushP[i], wallsOf(envBox))
		fk := float32(k * (0.7 + 0.3*math.Sin(t*20)))
		for _, y := range []float64{0.05, 0.6, 1.2} {
			sprite(fl(c, y), 0.35+0.3*k, cAmber.alpha(0.35*fk))
		}
		floorGlow(c, 0.6, cAmber.alpha(0.4*fk))
		addPoint(fl(c, 0.5), cAmber, 3*k, 3)
		if rand.Float64() < k*0.3 {
			spawnSparks(fl(c, 0.1+rand.Float64()*0.4), v3(0, 0.5, 0), 3, 0.3)
		}
	}
	if !slide.on {
		return
	}
	age := t - slide.t0
	if age > slideDur+0.5 {
		slide.on = false
		return
	}
	if age < slideDur {
		// steam venting along the rail at the foot of the moving wall
		k := slideK()
		w := slide.old
		for j := range w {
			w[j] += (slide.new[j] - w[j]) * k
		}
		c := e.wallPoint(slide.side, wallPushP[slide.side], w)
		if rand.Float64() < 0.6 {
			off := (rand.Float64() - 0.5) * 6
			q := envG.inv().apply(c)
			if slide.side < 2 {
				q[1] += off
			} else {
				q[0] += off
			}
			sp := envG.apply(q)
			spawnSteam(fl(sp, 0.05), v3(0, 0.6, 0), 2)
		}
	} else if !slide.clanked {
		slide.clanked = true
		c := e.wallPoint(slide.side, wallPushP[slide.side], slide.new)
		e.sfx(sndClang, fl(c, 1), 1)
		spawnSteam(fl(c, 0.1), v3(0, 1, 0), 30)
	}
}
