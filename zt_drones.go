package main

// ZoneTweak's roaming leech drones: a few of Contraband's drones going about the hall
// outside your boundary at about head height, the way the robots do - door to door, now and
// then out of one door and, a while later, back in through another, stopping now and then to
// look at you. Set dressing only.
//
// They never clip anything: they fly the same routes the robots walk (clear of your laser
// walls and generators), and another drone or a robot in the way doesn't block them - they
// rise up over it and drop back down past it.

import (
	"math"
	"math/rand"
)

type zDrone struct {
	pos, vel pt2
	h        float64
	path     []pt2
	yaw      float64
	ph       float64
	waitT    float64 // hovering (or, away, gone) for this long yet
	away     bool    // out through a door, not back yet
	leaving  bool    // on its way out
	replanT  float64
	moving   float64
}

var zDrones []*zDrone

const (
	zDroneN       = 3
	zDroneH       = 1.5  // cruising height: about five feet
	zDroneSpeed   = 0.75 // m/s
	zDroneR       = 0.3  // its own size, rotors and all
	zDroneGap     = 0.75 // from another drone, side to side
	zDroneBotGap  = 1.0  // from a robot, side to side
	zDroneOverBot = 2.75 // passing over a robot (they stand about 2.2 m)
	zDroneOverOne = 0.55 // passing over another drone: this much higher
)

func (e *editor) dronesFrame(dt float64) {
	if len(e.pts) < 3 || !envHave || !world.ok || len(world.doors) == 0 || dt <= 0 {
		return
	}
	for len(zDrones) < zDroneN {
		// they turn up a few seconds apart, each through a door
		d := &zDrone{away: true, waitT: 1 + 4*float64(len(zDrones)) + rand.Float64()*2, ph: rand.Float64() * 6, h: zDroneH}
		zDrones = append(zDrones, d)
	}
	for i, d := range zDrones {
		if d.away {
			d.waitT -= dt
			if d.waitT <= 0 {
				// back in through a door - one with nothing else coming or going through it
				dr := world.doors[rand.Intn(len(world.doors))]
				busy := false
				for _, o := range zDrones {
					if o != d && !o.away && (dist2(o.pos, dr.end) < 2.5 || dist2(o.pos, dr.mouth) < 2) {
						busy = true
					}
				}
				for _, rb := range robots {
					if dist2(rb.pos, dr.end) < 2 || dist2(rb.pos, dr.mouth) < 1.5 {
						busy = true
					}
				}
				if busy {
					d.waitT = 1
					continue
				}
				d.away, d.leaving, d.pos, d.vel, d.h = false, false, dr.end, pt2{}, zDroneH
				d.path = []pt2{dr.mouth}
				d.waitT = 0
			}
			continue
		}
		if len(d.path) == 0 {
			d.waitT -= dt
			if d.leaving {
				// out of the door: gone for a while
				d.away, d.waitT = true, 5+rand.Float64()*12
				continue
			}
			if d.waitT <= 0 {
				e.droneNext(d)
			}
		}
		// every couple of seconds, the rest of the route again (you might have moved a wall)
		d.replanT -= dt
		if d.replanT <= 0 && len(d.path) > 0 && !d.leaving {
			d.replanT = 2
			if p := e.route(d.pos, d.path[len(d.path)-1]); p != nil {
				d.path = p
			}
		}
		// along the route
		want := pt2{}
		if len(d.path) > 0 {
			wp := d.path[0]
			to := pt2{wp[0] - d.pos[0], wp[1] - d.pos[1]}
			L := math.Hypot(to[0], to[1])
			if L < 0.2 {
				d.path = d.path[1:]
				if len(d.path) == 0 {
					d.waitT = 2 + rand.Float64()*5
				}
			} else {
				k := zDroneSpeed * math.Min(1, (L+0.3)/0.8) / L
				want = pt2{to[0] * k, to[1] * k}
			}
		}
		// something in the way at its height (a robot, or a drone that goes first): it climbs
		// over the top, and holds back until it's up there
		hTo, hold := zDroneH, false
		for _, r := range robots {
			if l := dist2(d.pos, r.pos); l < zDroneBotGap+0.5 {
				hTo = math.Max(hTo, zDroneOverBot)
				hold = hold || l < zDroneBotGap+0.2 && d.h < zDroneOverBot-0.1
			}
		}
		for j, o := range zDrones {
			if j == i || o.away {
				continue
			}
			l := dist2(d.pos, o.pos)
			if j < i && l < zDroneGap+0.6 {
				hTo = math.Max(hTo, o.h+zDroneOverOne)
				hold = hold || l < zDroneGap+0.25 && d.h < o.h+zDroneOverOne-0.1
			}
			if j > i && l < zDroneGap+0.25 && o.h < d.h+zDroneOverOne-0.1 {
				hold = true // the one going over hasn't got up yet: wait for it
			}
		}
		if !e.droneClear(d.pos) {
			hTo = math.Max(hTo, e.height+0.5) // caught inside a moved wall: over the top of it
		}
		rate := 1.2 * dt
		d.h += math.Max(-rate, math.Min(rate, hTo-d.h))
		if hold {
			want = pt2{}
		}
		// and edge away from anything at its own height that's close
		away := func(p pt2, gap float64) {
			dx, dz := d.pos[0]-p[0], d.pos[1]-p[1]
			if l := math.Hypot(dx, dz); l < gap && l > 1e-6 {
				k := (gap - l) / l * 1.5
				want[0], want[1] = want[0]+dx*k, want[1]+dz*k
			}
		}
		for j, o := range zDrones {
			if j != i && !o.away && math.Abs(o.h-d.h) < zDroneOverOne {
				away(o.pos, zDroneGap+0.3)
			}
		}
		for _, rb := range robots {
			if d.h < zDroneOverBot-0.1 {
				away(rb.pos, zDroneBotGap+0.2)
			}
		}
		d.vel[0] += (want[0] - d.vel[0]) * math.Min(1, dt*3)
		d.vel[1] += (want[1] - d.vel[1]) * math.Min(1, dt*3)
		next := pt2{d.pos[0] + d.vel[0]*dt, d.pos[1] + d.vel[1]*dt}
		// never into your boundary or close to its walls and generators
		if e.droneClear(next) || !e.droneClear(d.pos) {
			d.pos = next
		} else {
			d.vel = pt2{}
		}
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
		d.yaw += angDiff(d.yaw, look) * math.Min(1, dt*1.8)
	}
}

// droneClear: whether a drone at p is well clear of your boundary's walls and generators.
func (e *editor) droneClear(p pt2) bool {
	if e.drawing || len(e.pts) < 3 {
		return true
	}
	return !pointInPoly(e.pts, p[0], p[1]) && outsideDist(e.pts, p) >= robotWallClear-0.05
}

// droneNext: somewhere new - another door (sometimes out of it), or along the hall.
func (e *editor) droneNext(d *zDrone) {
	for try := 0; try < 6; try++ {
		var to pt2
		leave := false
		switch x := rand.Float64(); {
		case x < 0.3:
			dr := world.doors[rand.Intn(len(world.doors))]
			to, leave = dr.mouth, true
		case x < 0.65:
			to = world.doors[rand.Intn(len(world.doors))].mouth
		default:
			P := 2 * (world.lx1 - world.lx0 + world.lz1 - world.lz0)
			to = world.g.apply(laneLocal(rand.Float64() * P))
		}
		if dist2(to, d.pos) < 1 {
			continue
		}
		crowded := false
		for _, o := range zDrones {
			if o != d && !o.away && (dist2(o.pos, to) < 1.5 || len(o.path) > 0 && dist2(o.path[len(o.path)-1], to) < 1.5) {
				crowded = true
			}
		}
		for _, rb := range robots {
			if dist2(rb.pos, to) < 1.5 {
				crowded = true
			}
		}
		if crowded {
			continue
		}
		p := e.route(d.pos, to)
		if p == nil {
			continue
		}
		if leave {
			for _, dr := range world.doors {
				if dr.mouth == to {
					p = append(p, dr.end)
				}
			}
		}
		d.path, d.leaving, d.replanT = p, leave, 2
		return
	}
	d.waitT = 1
}

func (e *editor) drawDrones() {
	if len(e.pts) < 3 || !envHave {
		return
	}
	t := fxTime()
	for _, d := range zDrones {
		if d.away {
			continue
		}
		pos := fl(d.pos, d.h+0.05*math.Sin(t*2.3+d.ph))
		f := v3(math.Cos(d.yaw), 0, math.Sin(d.yaw))
		s := v3(-f.z, 0, f.x)
		up := v3(0, 1, 0).add(f.mul(0.18 * d.moving)).norm()
		drawDroneBody(pos, s, up, f, true, false, d.ph)
		shadowBlob(d.pos, 0.22, float32(0.5*math.Max(0, 1-(d.h-0.2)/2)))
	}
}
