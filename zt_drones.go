package main

// ZoneTweak's roaming leech drones: a few of Contraband's drones drifting about the cell up
// above your generators, now and then stopping to look down at you. Set dressing only.
// They stay over the top of your generators and walls (and the robots on the floor), under
// the ceiling lamps, inside the cell, and apart from each other.

import (
	"math"
	"math/rand"
)

type zDrone struct {
	pos, vel, to pt2 // in the world, on the floor plane
	h, hTo       float64
	yaw          float64
	ph           float64
	waitT        float64
	moving       float64
}

var zDrones []*zDrone

const (
	zDroneN     = 3
	zDroneApart = 1.5 // from each other
	zDroneSpeed = 0.6 // m/s
	zDroneOver  = 0.6 // above the generators' tops
	zDroneLamp  = 1.3 // from under a ceiling lamp
)

// droneSpot: somewhere in the cell, clear of the other drones' spots.
func (e *editor) droneSpot(self *zDrone) pt2 {
	b := envBox
	var p pt2
	for k := 0; k < 20; k++ {
		p = envG.apply(pt2{b[0] + 0.6 + rand.Float64()*(b[1]-b[0]-1.2), b[2] + 0.6 + rand.Float64()*(b[3]-b[2]-1.2)})
		ok := true
		for _, lp := range envLamps {
			if dist2(p, pt2{lp.x, lp.z}) < zDroneLamp {
				ok = false
			}
		}
		for _, o := range zDrones {
			if o != self && dist2(p, o.to) < zDroneApart {
				ok = false
			}
		}
		if ok {
			break
		}
	}
	return p
}

// droneHeights: how low and how high they fly (over the generators, under the lamps).
func (e *editor) droneHeights() (float64, float64) {
	lo := math.Max(e.height+zDroneOver, 2.8) // (and over a robot's head)
	hi := math.Max(lo+0.2, math.Min(lo+1.0, envH-1.3))
	return lo, hi
}

func (e *editor) dronesFrame(dt float64) {
	if len(e.pts) < 3 || !envHave || dt <= 0 {
		return
	}
	lo, hi := e.droneHeights()
	for len(zDrones) < zDroneN {
		d := &zDrone{ph: rand.Float64() * 6, waitT: rand.Float64() * 2}
		d.pos = e.droneSpot(d)
		d.to = e.droneSpot(d)
		d.h = lo + rand.Float64()*(hi-lo)
		d.hTo = d.h
		zDrones = append(zDrones, d)
	}
	b := envBox
	for _, d := range zDrones {
		// pick somewhere new once it's got there (and hung about a moment)
		if dist2(d.pos, d.to) < 0.3 {
			d.waitT -= dt
			if d.waitT <= 0 {
				d.to = e.droneSpot(d)
				d.hTo = lo + rand.Float64()*(hi-lo)
				d.waitT = 1 + rand.Float64()*4
			}
		}
		to := pt2{d.to[0] - d.pos[0], d.to[1] - d.pos[1]}
		L := math.Hypot(to[0], to[1])
		want := pt2{}
		if L > 1e-6 {
			k := zDroneSpeed * math.Min(1, L/1.2) / L
			want = pt2{to[0] * k, to[1] * k}
		}
		// keep apart: a drone that's close pushes the other away
		for _, o := range zDrones {
			if o == d {
				continue
			}
			dx, dz := d.pos[0]-o.pos[0], d.pos[1]-o.pos[1]
			if l := math.Hypot(dx, dz); l < zDroneApart && l > 1e-6 && math.Abs(d.h-o.h) < 0.6 {
				k := (zDroneApart - l) / l * 1.2
				want[0], want[1] = want[0]+dx*k, want[1]+dz*k
			}
		}
		// and clear of the ceiling lamps (and the fan under one)
		for _, lp := range envLamps {
			dx, dz := d.pos[0]-lp.x, d.pos[1]-lp.z
			if l := math.Hypot(dx, dz); l < zDroneLamp && l > 1e-6 {
				k := (zDroneLamp - l) / l * 1.5
				want[0], want[1] = want[0]+dx*k, want[1]+dz*k
			}
		}
		d.vel[0] += (want[0] - d.vel[0]) * math.Min(1, dt*1.5)
		d.vel[1] += (want[1] - d.vel[1]) * math.Min(1, dt*1.5)
		d.pos[0] += d.vel[0] * dt
		d.pos[1] += d.vel[1] * dt
		// never out through the cell's walls (it can shrink under them)
		q := envG.inv().apply(d.pos)
		q[0] = math.Max(b[0]+0.4, math.Min(b[1]-0.4, q[0]))
		q[1] = math.Max(b[2]+0.4, math.Min(b[3]-0.4, q[1]))
		d.pos = envG.apply(q)
		// height: over the generators, under the lamps; a robot working below lifts it
		hTo := d.hTo
		for _, r := range robots {
			if dist2(d.pos, r.pos) < 1.2 {
				hTo = hi
			}
		}
		hTo = math.Max(lo, math.Min(hi, hTo))
		d.h += (hTo - d.h) * math.Min(1, dt*0.6)
		d.h = math.Max(d.h, lo-0.1)
		sp := math.Hypot(d.vel[0], d.vel[1])
		d.moving = math.Min(1, sp/zDroneSpeed)
		// face where it's going; hovering, it turns to look at you
		look := d.yaw
		if sp > 0.15 {
			look = math.Atan2(d.vel[1], d.vel[0])
		} else {
			cam := ed.hmd.pos()
			look = math.Atan2(cam.z-d.pos[1], cam.x-d.pos[0])
		}
		d.yaw += angDiff(d.yaw, look) * math.Min(1, dt*1.5)
	}
}

func (e *editor) drawDrones() {
	if len(e.pts) < 3 || !envHave {
		return
	}
	t := fxTime()
	for _, d := range zDrones {
		pos := fl(d.pos, d.h+0.06*math.Sin(t*2.3+d.ph))
		f := v3(math.Cos(d.yaw), 0, math.Sin(d.yaw))
		s := v3(-f.z, 0, f.x)
		up := v3(0, 1, 0).add(f.mul(0.18 * d.moving)).norm()
		drawDroneBody(pos, s, up, f, true, false, d.ph)
		shadowBlob(d.pos, 0.25, 0.25)
	}
}
