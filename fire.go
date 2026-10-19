//go:build windows

package main

import (
	"math"
	"math/rand"
	"os"
	"strconv"
)

// A one-off incident per launch, no sooner than 5 minutes in: a wall junction box starts
// sparking, a minute later it blows - debris, fire, alarm. Robots run in with extinguishers,
// smother it in a cloud of suppressant, weld the box back together and a Scuttler clears
// the debris.

type junction struct {
	c, n, x, base vec3 // board centre, outward normal, along-wall axis, floor point below
}

var envJunctions []junction

const (
	fireIdle = iota
	fireSparking
	fireBurning
	fireSmoulder
	fireRepair
	fireDone
)

const (
	fpTravel = iota
	fpSpray
	fpRepair
	fpLeave
	fpStow   // putting the extinguisher away
	fpFinish // folding the repair tool away, a last look over the work
)

var fire struct {
	phase      int
	at, t0     float64
	j          junction
	heat       float64
	scorch     float64
	nextAlarm  float64
	nextSpark  float64
	repairLeft float64
	bots       []*robot
	lastBurst  float64
	// the crew fights it in volleys: blast, hold off, it flares back, blast again
	volleyOn bool
	volleyT  float64
	volleys  int
	pauseT   float64
	readyT   float64
	supp     float64 // smoothed suppression: rises fast under the gas, ebbs after
}

func (e *editor) updateFire(dt float64) {
	t := fxTime()
	if fire.at == 0 {
		fire.at = t + 180 + rand.Float64()*420
		if v, err := strconv.ParseFloat(os.Getenv("ZONETWEAK_FIRE_AT"), 64); err == nil {
			fire.at = t + v
		}
	}
	switch fire.phase {
	case fireIdle:
		if t < fire.at || !world.ok || len(world.doors) == 0 {
			return
		}
		if len(envJunctions) == 0 {
			fire.phase = fireDone
			return
		}
		fire.j = envJunctions[rand.Intn(len(envJunctions))]
		fire.phase, fire.t0 = fireSparking, t
	case fireSparking:
		k := (t - fire.t0) / 60
		if t > fire.nextSpark {
			fire.nextSpark = t + (0.15+rand.Float64()*2.5)*(1.2-k)
			p := fire.j.c.add(fire.j.x.mul((rand.Float64() - 0.5) * 0.2)).add(v3(0, (rand.Float64()-0.5)*0.25, 0)).add(fire.j.n.mul(0.01))
			spawnSparks(p, fire.j.n, 4+int(20*k*rand.Float64()), 0.3+0.5*k)
			sparkBursts = append(sparkBursts, sparkBurst{p: p, t: t, k: 0.3 + 0.5*k})
			e.sfx(sndSpark, p, 0.25+0.35*k)
			if k > 0.5 && rand.Float64() < 0.5 {
				spawnSmoke(p, 2)
			}
		}
		if rand.Float64() < frameDT*k*3 {
			spawnSmoke(fire.j.c.add(fire.j.n.mul(0.05)), 1)
		}
		if k >= 1 {
			e.fireExplode()
		}
	case fireBurning:
		spraying, ready, crew := 0, 0, 0
		for _, r := range fire.bots {
			if r.done || r.state != rbFire {
				continue
			}
			crew++
			if r.firePh == fpSpray && r.exK >= 1 {
				ready++
			}
			if r.sprayOn {
				spraying++
			}
		}
		if ready > 0 && fire.readyT == 0 {
			fire.readyT = t
		}
		final := fire.volleys >= 3
		switch {
		case !fire.volleyOn && ready > 0 && t >= fire.volleyT && (ready >= crew || t-fire.readyT > 6):
			fire.volleyOn = true
			fire.volleys++
			fire.volleyT = t + 2.4 + rand.Float64()*1.3
			e.sfx(sndSteam, fire.j.c.add(fire.j.n.mul(1.0)), 0.9)
		case fire.volleyOn && t >= fire.volleyT && !final:
			fire.volleyOn = false
			fire.pauseT = t
			fire.volleyT = t + 2.6 + rand.Float64()*1.6
		}
		if ready == 0 {
			fire.volleyOn = false
		}
		cap := math.Max(0.2, 1-0.24*float64(fire.volleys))
		if spraying > 0 {
			fire.heat -= dt * 0.085 * float64(spraying)
			fire.supp += (1 - fire.supp) * math.Min(1, dt*3)
		} else {
			fire.heat += dt * 0.07 // left alone, it climbs straight back
			fire.supp -= fire.supp * math.Min(1, dt*0.55)
		}
		fire.heat = math.Min(cap, fire.heat)
		if t > fire.nextAlarm {
			fire.nextAlarm = t + 1.9
			e.sfx(sndAlarm, fl(pt2{fire.j.base.x, fire.j.base.z}, 3.6), 0.9)
		}
		if fire.heat <= 0 {
			fire.heat = 0
			fire.volleyOn = false
			fire.phase, fire.t0 = fireSmoulder, t
			e.sfx(sndSteam, fire.j.c, 0.7)
		}
	case fireSmoulder:
		fire.supp -= fire.supp * math.Min(1, dt*0.4)
		if rand.Float64() < frameDT*6*math.Max(0, 1-(t-fire.t0)/12) {
			spawnSmoke(fire.j.c.add(fire.j.n.mul(0.06)), 1)
		}
		if t-fire.t0 > 4 {
			fire.phase, fire.t0 = fireRepair, t
			fire.repairLeft = 16 + rand.Float64()*6
			for _, r := range fire.bots {
				if !r.done && r.state == rbFire {
					// put the extinguisher away first, then walk over and get the tools out
					r.firePh, r.path = fpStow, nil
					r.fireSpot = repairSpot(r == fire.bots[0])
					if r != fire.bots[0] && dist2(r.fireSpot, fire.bots[0].fireSpot) < 0.95 {
						r.fireSpot = spreadFrom(fire.bots[0].fireSpot, r.fireSpot)
					}
				}
			}
		}
	case fireRepair:
		working := 0
		for _, r := range fire.bots {
			if !r.done && r.state == rbFire && r.firePh == fpRepair && r.rpK >= 1 {
				working++
			}
		}
		fire.repairLeft -= dt * float64(working)
		if working == 0 && t-fire.t0 > 40 {
			fire.repairLeft = 0 // nobody made it: the box sorts itself out eventually
		}
		if fire.repairLeft <= 0 {
			fire.phase = fireDone
			fire.scorch = 0
			e.sfx(sndPowerUp, fire.j.c, 0.6)
			for _, r := range fire.bots {
				if !r.done && r.state == rbFire {
					if r.firePh == fpRepair {
						r.firePh, r.fireT = fpFinish, t
					} else {
						// stow whatever is still out, then head home
						r.firePh, r.path = fpTravel, nil
					}
				}
			}
		}
	}
}

func (e *editor) fireExplode() {
	t := fxTime()
	j := fire.j
	c := j.c.add(j.n.mul(0.05))
	fire.phase, fire.t0, fire.heat, fire.scorch = fireBurning, t, 1, 1
	e.sfx(sndBoom, c, 1)
	e.sfx(sndRip, c, 0.8)
	e.sfx(sndHit, c, 0.7)
	spawnSparks(c, j.n, 140, 1.6)
	spawnSmoke(c, 12)
	sparkBursts = append(sparkBursts, sparkBurst{p: c, t: t, k: 1})
	for k := 0; k < 3; k++ {
		sparkBursts = append(sparkBursts, sparkBurst{p: c.add(j.n.mul(0.3)), t: t + 0.05*float64(k), k: 1})
	}
	// debris: torn cover, board fragments, conduit
	for k := 0; k < 7; k++ {
		var shape []pt2
		r0 := 0.04 + rand.Float64()*0.07
		nv := 5 + rand.Intn(3)
		for v := 0; v < nv; v++ {
			a := float64(v) * 2 * math.Pi / float64(nv)
			r := r0 * (0.6 + 0.6*rand.Float64())
			shape = append(shape, pt2{math.Cos(a) * r, math.Sin(a) * r})
		}
		vel := j.n.mul(1.6 + rand.Float64()*2.4).add(j.x.mul(rand.NormFloat64() * 1.2)).add(v3(0, 0.5+rand.Float64()*2, 0))
		d := &oldBase{
			pos: pt2{c.x, c.z}, ax: v3(1, 0, 0), t0: t, debris: true, small: true, state: 3,
			shape: shape, pieces: [3]int{0, 2, 2}, fireDebris: true,
			dpos: c, dvel: vel,
			spinAx: v3(rand.NormFloat64(), rand.NormFloat64(), rand.NormFloat64()).norm(), spinV: 8 + rand.Float64()*14,
		}
		oldBases = append(oldBases, d)
	}
	// fire crews: a Warden and a Tread come running with extinguishers
	for k, kind := range []int{kindWarden, kindTread} {
		d := world.doors[k%len(world.doors)]
		r := &robot{pos: d.end, gen: -1, seed: rand.Float64() * 100, kind: kind}
		r.randomize()
		r.state, r.firePh, r.svPiece = rbFire, fpTravel, -1
		r.fireCrew = true
		r.yaw = math.Atan2(d.mouth[1]-d.end[1], d.mouth[0]-d.end[0])
		r.fireSpot = sprayStand(k)
		if k == 1 && dist2(r.fireSpot, fire.bots[0].fireSpot) < 1.0 {
			r.fireSpot = spreadFrom(fire.bots[0].fireSpot, r.fireSpot)
		}
		r.path = append([]pt2{d.mouth}, e.fireRoute(d.mouth, r.fireSpot)...)
		robots = append(robots, r)
		fire.bots = append(fire.bots, r)
	}
}

// stand points in front of the box, clamped to where robots can walk
func laneClamp(p pt2) pt2 {
	if !world.ok {
		return p
	}
	q := world.g.inv().apply(p)
	q[0] = math.Max(world.lx0+0.2, math.Min(world.lx1-0.2, q[0]))
	q[1] = math.Max(world.lz0+0.2, math.Min(world.lz1-0.2, q[1]))
	return world.g.apply(q)
}

func sprayStand(k int) pt2 {
	j := fire.j
	side := []float64{-0.55, 0.55}[k%2]
	p := j.base.add(j.n.mul(1.25)).add(j.x.mul(side))
	return laneClamp(pt2{p.x, p.z})
}

func repairSpot(first bool) pt2 {
	j := fire.j
	side := 0.32
	if !first {
		side = -0.32
	}
	p := j.base.add(j.n.mul(0.75)).add(j.x.mul(side))
	return laneClamp(pt2{p.x, p.z})
}

// spreadFrom: a second stand spot at least a body-width from the first, still walkable
func spreadFrom(a, b pt2) pt2 {
	for _, d := range []vec3{fire.j.x, fire.j.x.mul(-1), fire.j.n, fire.j.n.mul(-1)} {
		for _, s := range []float64{1.0, 1.3} {
			p := laneClamp(pt2{a[0] + d.x*s, a[1] + d.z*s})
			if dist2(p, a) >= 0.95 {
				return p
			}
		}
	}
	return b
}

func (e *editor) fireRoute(from, to pt2) []pt2 {
	if p := e.route(from, to); p != nil {
		return p
	}
	return []pt2{to}
}

// stepFireBot drives a fire-crew robot; returns (want walking, head yaw target)
func (e *editor) stepFireBot(r *robot, dt float64) (float64, float64) {
	defer func() {
		want := 0.0
		if r.sprayOn || r.firePh == fpSpray && fire.volleyOn {
			want = 1
		}
		r.aimUp += (want - r.aimUp) * math.Min(1, dt*4)
	}()
	r.sprayOn = false
	j := fire.j
	t := fxTime()
	face := func() {
		ty := math.Atan2(j.base.z-r.pos[1], j.base.x-r.pos[0])
		r.yaw += angDiff(r.yaw, ty) * math.Min(1, dt*3)
	}
	speed := 1.15 // running
	if r.kind == kindTread {
		speed = 1.4
	}
	switch r.firePh {
	case fpTravel:
		r.rpK = math.Max(0, r.rpK-dt/0.9)
		if r.exK > 0 {
			// never run with the horn out: stow it before setting off
			r.exK = math.Max(0, r.exK-dt/1.2)
			return 0, 0
		}
		if len(r.path) == 0 {
			switch fire.phase {
			case fireRepair:
				r.firePh = fpRepair
			case fireDone:
				r.firePh = fpLeave
				e.sendHome(r)
			default:
				r.firePh = fpSpray
			}
			return 0, 0
		}
		if r.walkTo(r.path[0], speed, dt) {
			r.path = r.path[1:]
		}
		return 1, 0
	case fpSpray:
		face()
		if fire.phase == fireBurning {
			r.exK = math.Min(1, r.exK+dt/1.4) // unhook the bottle, bring the horn up
		}
		if fire.phase != fireBurning || r.exK < 1 || !fire.volleyOn || r.aimUp < 0.8 {
			return 0, 0.15 * math.Sin(t*0.9+r.seed) // between blasts: watching it
		}
		r.sprayOn = true
		co2Emit(r, t, dt)
		if t > r.nextHiss {
			r.nextHiss = t + 0.9
			e.sfx(sndSteam, fl(r.pos, 1), 0.5)
		}
		return 0, 0
	case fpStow:
		face()
		r.exK = math.Max(0, r.exK-dt/1.3)
		if r.exK <= 0 {
			r.firePh = fpTravel
			if len(r.path) == 0 {
				r.path = e.fireRoute(r.pos, r.fireSpot)
			}
		}
		return 0, 0
	case fpRepair:
		face()
		switch fire.phase {
		case fireRepair:
			r.rpK = math.Min(1, r.rpK+dt/1.2) // cutter and welder unfold out of the arm
			r.weldOn = r.rpK >= 1
		case fireDone:
			r.firePh, r.fireT = fpFinish, t
		}
		return 0, 0
	case fpFinish:
		face()
		r.rpK = math.Max(0, r.rpK-dt/1.0)
		age := t - r.fireT
		if age > 2.8 {
			r.firePh = fpLeave
			e.sendHome(r)
			return 0, 0
		}
		if r.rpK <= 0 {
			// a last scan over the patched box before heading off
			return 0, 0.45 * math.Sin((age-1)*3.2)
		}
		return 0, 0
	}
	r.exK = math.Max(0, r.exK-dt/1.2)
	r.rpK = math.Max(0, r.rpK-dt/0.9)
	if len(r.path) == 0 {
		r.done = true
		return 0, 0
	}
	if r.walkTo(r.path[0], 0.85, dt) {
		r.path = r.path[1:]
	}
	return 1, 0
}

// sprayFX: a CO2 blast from the horn - a roaring cone of cold gas that boils into a swirling
// white cloud over the fire. No beam: it is all gas, spawned as puffs that spin round the jet.
func (e *editor) sprayFX(r *robot, tip, aim vec3) {
	t := fxTime()
	tgt := fire.j.c.add(fire.j.n.mul(0.05))
	r.hornTip, r.hornDir, r.hornT = tip, aim, t
	// frost glow at the horn mouth and a cold light in the cloud
	sprite(tip.add(aim.mul(0.03)), 0.06, rgba{0.92, 0.96, 1, 0.55})
	L := tgt.sub(tip).length()
	addPoint(tip.add(aim.mul(L*0.55)), rgba{0.85, 0.92, 1, 1}, 3, 2.6)
}

// co2Emit: the blast itself, run from the sim so the cloud builds even while nobody looks
func co2Emit(r *robot, t, dt float64) {
	tip, aim := r.hornTip, r.hornDir
	if t-r.hornT > 0.5 || aim.length() < 0.5 {
		// not drawn lately: estimate the horn from the body
		tip = fl(r.pos, 1.0)
		if r.kind == kindTread {
			tip = fl(r.pos, 0.75)
		}
		aim = fire.j.c.sub(tip).norm()
		tip = tip.add(aim.mul(0.45))
	}
	n := dt * 26
	if fire.volleyT-t < 0.3 {
		n *= 0.5 // valve closing: the jet sputters
	}
	for n > 0 {
		if n < 1 && rand.Float64() > n {
			break
		}
		n--
		cone := v3(rand.NormFloat64(), rand.NormFloat64(), rand.NormFloat64()).mul(0.09)
		spawnCO2Jet(tip.add(aim.mul(0.02)), aim.add(cone).norm(), aim)
	}
	if rand.Float64() < dt*8 {
		tgt := fire.j.c.add(fire.j.n.mul(0.05))
		spawnCO2(tgt, fire.j.n.add(v3(0, 0.5, 0)).add(v3(rand.NormFloat64(), 0, rand.NormFloat64()).mul(0.5)).norm())
	}
}

// the CO2 bottle: red cylinder, chrome valve and carry handle
func drawExtinguisher(p, up vec3) {
	useMat(mPaint)
	cyl(&bEnv, p, p.add(up.mul(0.28)), 0.055, 0.055, 14, rgba{0.65, 0.06, 0.04, 1}, true)
	hemi(&bEnv, p.add(up.mul(0.28)), up, 0.055, rgba{0.65, 0.06, 0.04, 1}, 3, 14)
	useMat(mChrome)
	cyl(&bEnv, p.add(up.mul(0.32)), p.add(up.mul(0.37)), 0.015, 0.015, 8, cChromeEnv, true)
	side := up.cross(v3(0.3, 0, 1)).norm()
	cyl(&bEnv, p.add(up.mul(0.36)), p.add(up.mul(0.36)).add(side.mul(0.06)), 0.007, 0.007, 6, cChromeEnv, false)
}

// the discharge horn: a black flared cone on a short chrome swivel
func drawCO2Horn(base, dir vec3) vec3 {
	useMat(mChrome)
	cyl(&bEnv, base, base.add(dir.mul(0.04)), 0.012, 0.012, 8, cChromeEnv, true)
	useMat(mRubber)
	m := base.add(dir.mul(0.04))
	tip := m.add(dir.mul(0.13))
	cyl(&bEnv, m, tip, 0.018, 0.045, 14, rgba{0.04, 0.04, 0.045, 1}, false)
	return tip
}

// the hose from the bottle valve to the horn
func co2Hose(a, b, bdir vec3) {
	sag := math.Max(0.05, a.sub(b).length()*0.35)
	hose(&bEnv, a, a.add(v3(0, -sag, 0)), b.sub(bdir.mul(0.08)).add(v3(0, -sag*0.6, 0)), b, 0.009, rgba{0.05, 0.05, 0.055, 1}, 10)
}

// flameBB: one vertical flame billboard turned to face the viewer (shape and motion are done
// in flame.fs; r of the colour carries a seed, alpha the intensity)
// flameBBShape: shape 0 = campfire (wide root), 1 = electrical (narrow source, wide middle)
func flameBBShape(base vec3, w, h, inten, seed, shape float64) {
	flameShape = shape
	flameBB(base, w, h, inten, seed)
	flameShape = 0
}

var flameShape float64

func flameBB(base vec3, w, h, inten, seed float64) {
	if inten <= 0.01 {
		return
	}
	up := v3(0, 1, 0)
	toCam := camPos.sub(base)
	toCam.y = 0
	if toCam.length() < 1e-4 {
		toCam = v3(0, 0, 1)
	}
	right := up.cross(toCam.norm()).norm().mul(w / 2)
	c := rgba{float32(seed), float32(flameShape), 0, float32(math.Min(1, inten))}
	a, b := base.sub(right), base.add(right)
	bFire.quad(a, b, b.add(up.mul(h)), a.add(up.mul(h)), uvSprite, c, c, c, c)
}

// drawFire: the junction box sparking, burning, scorched and finally patched
func (e *editor) drawFire() {
	if fire.phase == fireIdle {
		return
	}
	t := fxTime()
	j := fire.j
	up := v3(0, 1, 0)
	seedTex(51000)
	if fire.phase == fireSparking {
		k := (t - fire.t0) / 60
		if noise1(3.3, t*9) > 0.8-0.3*k {
			sprite(j.c.add(j.n.mul(0.02)), 0.06+0.1*k, cSpark.alpha(0.8))
			addPoint(j.c.add(j.n.mul(0.1)), cSpark, 1+3*k, 2)
		}
		return
	}
	// scorch: blackened, blistered board and soot streaks up the wall
	if fire.phase != fireDone {
		useMat(material{2, 0, 0.95, 0})
		c := j.c.add(j.n.mul(0.004))
		q := func(x, y float64) vec3 { return c.add(j.x.mul(x)).add(up.mul(y)) }
		bEnv.quadF(q(-0.2, -0.24), q(0.2, -0.24), q(0.2, 0.24), q(-0.2, 0.24), j.n, rgba{0.025, 0.022, 0.02, 1})
		bEnv.quadF(q(-0.26, 0.24), q(0.26, 0.24), q(0.14, 0.9), q(-0.14, 0.9), j.n, rgba{0.035, 0.03, 0.028, 1})
		// glowing embers in the char while it's hot
		if fire.phase == fireBurning || fire.phase == fireSmoulder {
			for k := 0; k < 9; k++ {
				p := q(-0.15+0.3*hash1(float64(k)*3.1), -0.2+0.4*hash1(float64(k)*7.7)).add(j.n.mul(0.002))
				g := 0.5 + 0.5*noise1(float64(k), t*2)
				if fire.phase == fireSmoulder {
					g *= math.Max(0, 1-(t-fire.t0)/4)
				}
				sprite(p, 0.025, heatColor(0.35+0.4*g).alpha(float32(0.8*g)))
			}
		}
	}
	switch fire.phase {
	case fireBurning:
		h := math.Max(0.15, fire.heat)
		// an electrical fire: it bursts out of the box itself - narrow at the source, billowing
		// wide in the middle, tapering above - choked with smoke, and it comes and goes as the
		// shorted board arcs and spits.
		base := j.c.add(j.n.mul(0.06)).add(up.mul(-0.1))
		surge := math.Pow(noise1(4.4, t*0.9), 3)*1.5 + 0.3*noise1(8.8, t*3.3)
		// it breathes: bursts of flame, then it chokes down to a smoulder pouring black smoke,
		// then the smoke ignites and it bursts again. Spraying suppressant pushes it to the low side.
		burst := smooth((noise1(21.3, t*0.33) + 0.25*noise1(5.7, t*1.1) - 0.45) / 0.25)
		if fire.volleys > 0 && !fire.volleyOn {
			// a pause in the blasting: the fire claws its way back through the cloud
			burst = math.Max(burst, smooth((t-fire.pauseT-0.9)/1.1)*(0.55+0.45*noise1(2.2, t*1.7)))
		}
		burst *= 1 - 0.9*fire.supp
		if burst > 0.6 && fire.lastBurst <= 0.6 {
			// re-ignition: the smoke flashes over
			pp := base.add(up.mul(0.25))
			spawnSparks(pp, j.n, 30+rand.Intn(25), 1.3)
			sparkBursts = append(sparkBursts, sparkBurst{p: pp, t: t, k: 0.9})
			e.sfx(sndBoom, pp, 0.35)
			e.sfx(sndZap, pp, 0.5)
		}
		fire.lastBurst = burst
		// dense black soot, heaviest while the flames are down
		if rand.Float64() < frameDT*(8+36*(1-burst))*h/(1+2*fire.supp) {
			sp := base.add(up.mul(0.15 + 0.4*rand.Float64())).add(j.n.mul(0.1)).add(j.x.mul((rand.Float64() - 0.5) * 0.3))
			spawnSoot(sp, 1)
		}
		for k := 0; k < 6; k++ {
			hk := hash1(float64(k)*3.7 + 1)
			// each tongue flares up and dies back on its own rhythm
			gate := smooth((noise1(hk*23, t*(0.9+hk))-0.32)/0.3) * (0.15 + 0.85*burst)
			if gate < 0.02 {
				continue
			}
			x := (hk - 0.5) * 0.22
			y := (hash1(float64(k)*9.1) - 0.5) * 0.14
			wk := (0.35 + 0.3*hash1(float64(k)*5.5)) * (0.7 + 0.5*h)
			hgt := (0.55 + 0.5*hash1(float64(k)*7.3)) * (0.6 + 0.6*h) * (0.75 + 0.5*surge) * (0.45 + 0.75*burst)
			jit := 0.7 + 0.6*noise1(hk*17, t*(3+2*hk))
			flameBBShape(base.add(j.x.mul(x)).add(up.mul(y)).add(j.n.mul(0.03+0.05*hk)), wk*(0.85+0.3*jit), hgt*jit, 0.55*h*gate*(0.8+0.4*jit), math.Mod(0.11+hk*0.77, 1), 1)
		}

		// the short: arcs crawling out of the box, blue-white flashes, crackle
		if rand.Float64() < frameDT*(3+6*h) {
			a0 := j.c.add(j.n.mul(0.03)).add(j.x.mul((rand.Float64() - 0.5) * 0.2)).add(up.mul((rand.Float64() - 0.5) * 0.25))
			dir := j.x.mul(rand.NormFloat64()).add(up.mul(rand.NormFloat64())).add(j.n.mul(0.3 * rand.Float64())).norm()
			end := a0.add(dir.mul(0.12 + 0.35*rand.Float64()))
			arc(a0, end, rgba{0.75, 0.85, 1, 1})
			addPoint(a0.add(j.n.mul(0.15)), rgba{0.7, 0.8, 1, 1}, 6+6*rand.Float64(), 3)
			sprite(a0, 0.1, rgba{0.8, 0.9, 1, 0.6})
			if rand.Float64() < 0.3 {
				e.sfx(sndSpark, a0, 0.35)
			}
		}
		// pops: a component blows and throws burning bits and sparks out of the fire
		if rand.Float64() < frameDT*0.6*h {
			pp := base.add(up.mul(0.1))
			spawnSparks(pp, j.n, 18+rand.Intn(20), 1.1)
			spawnSmoke(pp, 3)
			sparkBursts = append(sparkBursts, sparkBurst{p: pp, t: t, k: 0.6})
			e.sfx(sndZap, pp, 0.4)
		}
		sprite(base, 0.14, rgba{1, 0.85, 0.5, float32(0.25 * h)})
		// burning debris on the floor below
		for _, d := range oldBases {
			if d.fireDebris && !d.gone && d.landed && dist2(d.pos, pt2{j.base.x, j.base.z}) < 3 {
				dp := fl(d.pos, 0.03)
				flameBB(dp, 0.16, 0.28, h*0.8, math.Mod(d.t0*0.37, 1))
				if rand.Float64() < frameDT*2*h {
					spawnSmoke(dp.add(up.mul(0.15)), 1)
				}
			}
		}
		fl := 0.75 + 0.25*noise1(1.7, t*12)
		addPoint(base.add(j.n.mul(0.35)).add(up.mul(0.2)), rgba{1, 0.55, 0.2, 1}, (6+6*h)*fl, 5)
		floorGlow(pt2{j.base.x + j.n.x*0.4, j.base.z + j.n.z*0.4}, 1.2, rgba{1, 0.45, 0.1, float32(0.18 * h * fl)})
		if rand.Float64() < frameDT*6*h {
			spawnSparks(base.add(up.mul(0.2)), up, 2, 0.4)
		}
		// rotating red alarm beacons up on the ceiling trusses
		for k, off := range []float64{0, 2.5} {
			bp := j.base.add(j.n.mul(1.2 + off)).add(up.mul(3.7))
			a := t*5 + float64(k)*1.3
			d := v3(math.Cos(a), -0.25, math.Sin(a)).norm()
			useMat(mPaint)
			cyl(&bEnv, bp.add(up.mul(0.06)), bp.add(up.mul(-0.04)), 0.05, 0.05, 12, rgba{0.2, 0.2, 0.22, 1}, true)
			curEmis = 3
			hemi(&bEnv, bp.add(up.mul(-0.04)), up.mul(-1), 0.045, rgba{1, 0.1, 0.05, 1}, 3, 12)
			curEmis = 1
			ribbonC(bp, bp.add(d.mul(2.2)), 0.28, rgba{1, 0.08, 0.04, 0.22}, rgba{1, 0.08, 0.04, 0})
			addPoint(bp.add(d.mul(0.6)), rgba{1, 0.1, 0.05, 1}, 4, 6)
		}
	case fireSmoulder:
		s := math.Max(0, 1-(t-fire.t0)/4)
		addPoint(j.c.add(j.n.mul(0.2)), rgba{1, 0.4, 0.15, 1}, 2*s, 2.5)
	case fireDone:
		// the patched-up box: a fresh plate bolted over the damage
		useMat(mSteel)
		F := xf{j.c.add(j.n.mul(0.012)), j.x, up, j.n}
		bevelBox(&bEnv, F.o, j.x, up, j.n, 0.2, 0.25, 0.008, 0.004, cSteelEnv.scale(0.9))
		for _, sx := range []float64{-0.17, 0.17} {
			for _, sy := range []float64{-0.21, 0.21} {
				b := F.p(sx, sy, 0.009)
				cyl(&bEnv, b, b.add(j.n.mul(0.006)), 0.01, 0.01, 6, cChromeEnv, true)
			}
		}
		sprite(F.p(0.12, 0.17, 0.014), 0.012, cEnergy.alpha(float32(0.6+0.4*math.Sin(t*3))))
	}
}
