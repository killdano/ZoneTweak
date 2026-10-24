package main

import (
	"math"
	"math/rand"
)

type particle struct {
	p, v      vec3
	life, max float64
	heat      float64
	kind      int
	size      float64
	bounced   int
	ax        vec3 // CO2: jet axis the puff spins round
}

const (
	pSpark = iota
	pEmber
	pSmoke
	pSteam
	pMist
	pSoot
	pCO2
)

var (
	particles []particle
	bSmoke    tbatch
	bFire     tbatch
)

func spawnSparks(p vec3, n vec3, count int, speed float64) {
	for i := 0; i < count; i++ {
		d := v3(rand.NormFloat64(), rand.NormFloat64()+0.6, rand.NormFloat64()).norm()
		if d.dot(n) < 0 {
			d = d.add(n.mul(-2 * d.dot(n)))
		}
		sp := (0.8 + rand.ExpFloat64()*1.6) * (0.6 + speed)
		life := 0.3 + rand.Float64()*0.9
		kind := pSpark
		if rand.Float64() < 0.12 {
			kind = pEmber
			sp *= 0.4
			life *= 2.2
		}
		particles = append(particles, particle{p: p, v: d.mul(sp), life: life, max: life, heat: 0.8 + 0.2*rand.Float64(), kind: kind})
	}
	if count > 6 {
		sparkBursts = append(sparkBursts, sparkBurst{p: p, t: fxTime(), k: math.Min(1, float64(count)/40)})
	}
	trimParticles()
}

// softN: smoke/steam/mist billboards alive. They're expensive to fill (each is a lit,
// noise-shaded sprite), so past a budget new ones are skipped rather than stacking up.
var softN int

const softBudget = 420

func softFull() bool {
	if potatoMode {
		return softN >= 70
	}
	return softN >= softBudget
}

func spawnSmoke(p vec3, count int) {
	for i := 0; i < count; i++ {
		if softFull() {
			return
		}
		softN++
		v := v3(rand.NormFloat64()*0.06, 0.12+rand.Float64()*0.15, rand.NormFloat64()*0.06)
		life := 4 + rand.Float64()*3.5
		particles = append(particles, particle{p: p, v: v, life: life, max: life, kind: pSmoke, size: 0.06 + rand.Float64()*0.05})
	}
	trimParticles()
}

func spawnSteam(p, dir vec3, count int) {
	for i := 0; i < count; i++ {
		if softFull() {
			return
		}
		softN++
		v := dir.mul(0.7 + rand.Float64()*0.9).add(v3(rand.NormFloat64(), rand.NormFloat64(), rand.NormFloat64()).mul(0.15))
		life := 4 + rand.Float64()*4
		particles = append(particles, particle{p: p, v: v, life: life, max: life, kind: pSteam, size: 0.06 + rand.Float64()*0.06})
	}
	trimParticles()
}

// spawnSoot: thick black smoke from the electrical fire
func spawnSoot(p vec3, count int) {
	for i := 0; i < count; i++ {
		if softFull() {
			return
		}
		softN++
		v := v3(rand.NormFloat64()*0.08, 0.35+rand.Float64()*0.3, rand.NormFloat64()*0.08)
		life := 5 + rand.Float64()*3
		particles = append(particles, particle{p: p, v: v, life: life, max: life, kind: pSoot, size: 0.12 + rand.Float64()*0.08})
	}
}

// spawnCO2: extinguisher gas - fast, then billowing into a big cold white cloud
func spawnCO2(p, dir vec3) {
	if softFull() {
		return
	}
	softN++
	life := 2.5 + rand.Float64()*2
	particles = append(particles, particle{p: p, v: dir.mul(2.5 + rand.Float64()*1.5), life: life, max: life, kind: pCO2, size: 0.08 + rand.Float64()*0.05,
		ax: dir, heat: rand.Float64()})
}

// spawnCO2Jet: one puff of the blast leaving the horn - fast, small, spinning round the jet
// axis so the cloud it builds churns in swirls
func spawnCO2Jet(p, dir, axis vec3) {
	// the blast gets its own head-room over the ambient steam and haze
	if potatoMode && softN >= 110 || !potatoMode && softN >= softBudget+120 {
		return
	}
	softN++
	life := 1.4 + rand.Float64()*1.1
	particles = append(particles, particle{p: p, v: dir.mul(3.4 + rand.Float64()*1.8), life: life, max: life, kind: pCO2, size: 0.035 + rand.Float64()*0.03,
		ax: axis, heat: rand.Float64()})
}

// spawnMist: a slow, long-lived bank of vapour that just hangs and drifts
func spawnMist(p vec3, size float64) {
	if softN >= softBudget-80 || potatoMode {
		return // mist is ambience: it gives way first
	}
	softN++
	v := v3(rand.NormFloat64()*0.03, 0.005+rand.Float64()*0.02, rand.NormFloat64()*0.03)
	life := 10 + rand.Float64()*8
	particles = append(particles, particle{p: p, v: v, life: life, max: life, kind: pMist, size: size * (0.7 + 0.6*rand.Float64()), heat: rand.Float64()})
	trimParticles()
}

func trimParticles() {
	if len(particles) > 2500 {
		particles = particles[len(particles)-2500:]
	}
}

type sparkBurst struct {
	p vec3
	t float64
	k float64
}

var sparkBursts []sparkBurst

// blackbody-ish ramp for cooling metal
func heatColor(h float64) rgba {
	h = math.Max(0, math.Min(1, h))
	switch {
	case h > 0.75:
		k := float32((h - 0.75) / 0.25)
		return rgba{1, 0.85 + 0.15*k, 0.55 + 0.45*k, 1}
	case h > 0.45:
		k := float32((h - 0.45) / 0.3)
		return rgba{1, 0.45 + 0.4*k, 0.12 + 0.43*k, 1}
	}
	k := float32(h / 0.45)
	return rgba{0.55 + 0.45*k, 0.08 + 0.37*k, 0.02 + 0.1*k, 1}
}

func updateParticles(dt float64) {
	out := particles[:0]
	for _, q := range particles {
		q.life -= dt
		if q.life <= 0 {
			continue
		}
		switch q.kind {
		case pSpark, pEmber:
			q.v.y -= 9.8 * dt
			drag := 1 - dt*0.6
			q.v = q.v.mul(drag)
			q.p = q.p.add(q.v.mul(dt))
			q.heat -= dt * (0.9 + 0.6*rand.Float64())
			if q.kind == pEmber {
				q.heat += dt * 0.6
			}
			if q.p.y < 0 {
				q.p.y = 0.001
				q.bounced++
				q.v.y = -q.v.y * (0.25 + 0.2*rand.Float64())
				q.v.x = q.v.x*0.55 + rand.NormFloat64()*0.3
				q.v.z = q.v.z*0.55 + rand.NormFloat64()*0.3
				if q.bounced == 1 && rand.Float64() < 0.25 && len(particles) < 2400 {
					for s := 0; s < 2; s++ {
						c := q
						c.v = v3(rand.NormFloat64()*0.6, 0.6+rand.Float64(), rand.NormFloat64()*0.6)
						c.life *= 0.5
						c.max = c.life
						out = append(out, c)
					}
				}
			}
		case pSmoke:
			q.v = q.v.mul(1 - dt*0.4)
			q.v.y += dt * 0.05
			q.p = q.p.add(q.v.mul(dt)).add(v3(math.Sin(q.p.y*3+q.life), 0, math.Cos(q.p.x*3)).mul(dt * 0.05))
			q.size += dt * 0.07
		case pSteam:
			q.v = q.v.mul(1 - dt*1.4)
			q.v.y += dt * 0.12
			q.p = q.p.add(q.v.mul(dt)).add(v3(math.Sin(q.p.y*2+q.max), 0, math.Cos(q.p.x*2+q.max)).mul(dt * 0.04))
			q.size += dt * 0.12 * (q.life / q.max)
		case pSoot:
			q.v = q.v.mul(1 - dt*0.3)
			q.v.y += dt * 0.08
			q.p = q.p.add(q.v.mul(dt)).add(v3(math.Sin(q.p.y*2+q.max), 0, math.Cos(q.p.x*2+q.max)).mul(dt * 0.08))
			q.size += dt * 0.16
		case pCO2:
			q.v = q.v.mul(1 - dt*2.4)
			// spin round the jet axis: the perpendicular drift is rotated, so puffs corkscrew
			if q.ax.length() > 0.5 {
				along := q.ax.mul(q.v.dot(q.ax))
				perp := q.v.sub(along)
				w := (2.5 + 3*q.heat) * dt
				if q.heat < 0.5 {
					w = -w
				}
				perp = perp.add(q.ax.cross(perp).mul(w))
				q.v = along.add(perp)
				// turbulence: each puff wanders off the axis as it slows
				ag := 1 - q.life/q.max
				q.v = q.v.add(v3(math.Sin(q.p.y*7+q.heat*20+fxTime()*2), math.Cos(q.p.x*6+q.heat*13), math.Sin(q.p.z*7-fxTime()*1.7)).mul(dt * 0.9 * ag))
			}
			// cold, heavy gas: it sinks a little once the blast lets go of it
			q.v.y -= dt * 0.06 * (1 - q.life/q.max)
			q.p = q.p.add(q.v.mul(dt))
			// the blast slams into the wall round the fire and fans out across it
			if fire.phase != fireIdle {
				n := fire.j.n
				if d := q.p.sub(fire.j.c).dot(n); d < 0.05 && d > -0.6 {
					q.p = q.p.add(n.mul(0.05 - d))
					if vn := q.v.dot(n); vn < 0 {
						q.v = q.v.sub(n.mul(vn * 1.3))
						q.size += 0.05
					}
				}
			}
			q.size = math.Min(0.55, q.size+dt*(0.12+0.5*q.life/q.max))
		case pMist:
			q.p = q.p.add(q.v.mul(dt)).add(v3(math.Sin(fxTime()*0.2+q.heat*9), 0, math.Cos(fxTime()*0.17+q.heat*7)).mul(dt * 0.025))
			q.size += dt * 0.02
		}
		out = append(out, q)
	}
	particles = out
	softN = 0
	for _, q := range particles {
		if q.kind >= pSmoke {
			softN++
		}
	}
	for _, q := range particles {
		f := q.life / q.max
		switch q.kind {
		case pSpark:
			c := heatColor(q.heat)
			sp := q.v.length()
			tail := q.p.sub(q.v.mul(math.Min(0.045, 0.02+sp*0.004)))
			a := float32(math.Min(1, f*1.6))
			ribbonC(tail, q.p, 0.006+0.004*q.heat, c.alpha(0), c.alpha(a))
			bGlowLine.line(tail, q.p, c.mix(rgba{1, 1, 1, 1}, float32(q.heat*0.5)).alpha(a))
		case pEmber:
			c := heatColor(q.heat * (0.7 + 0.3*math.Sin(q.life*30)))
			sprite(q.p, 0.012, c.alpha(float32(f)))
		case pSmoke:
			a := float32(math.Min(1, (1-f)*6) * f * 0.45)
			smokeSprite(q.p, q.size, rgba{0.16, 0.155, 0.15, a})
		case pSteam:
			a := float32(math.Min(1, (1-f)*5) * f * 0.26)
			smokeSprite(q.p, q.size, rgba{0.8, 0.84, 0.86, a})
		case pSoot:
			a := float32(math.Min(1, (1-f)*5) * f * 0.75)
			smokeSprite(q.p, q.size, rgba{0.03, 0.028, 0.026, a})
		case pCO2:
			// colour (0.95, 0.97, 1) flags the swirling-gas look in smoke.fs
			a := float32(math.Min(1, (1-f)*10) * math.Pow(f, 0.7) * 0.65)
			smokeSprite(q.p, q.size, rgba{0.95, 0.97, 1, a})
		case pMist:
			a := float32(math.Sin(f*math.Pi) * 0.09)
			smokeSprite(q.p, q.size, rgba{0.78, 0.82, 0.84, a})
		}
	}
	t := fxTime()
	live := sparkBursts[:0]
	for _, b := range sparkBursts {
		age := t - b.t
		if age > 0.6 {
			continue
		}
		k := (1 - age/0.6) * b.k
		addPoint(b.p.add(v3(0, 0.08, 0)), rgba{1, 0.7, 0.35, 1}, 9*k*k, 3.5)
		live = append(live, b)
	}
	sparkBursts = live
	if len(sparkBursts) > 6 {
		sparkBursts = sparkBursts[len(sparkBursts)-6:]
	}
}

func smokeSprite(p vec3, s float64, col rgba) {
	r, u := camR.mul(s), camU.mul(s)
	bSmoke.quad(p.sub(r).sub(u), p.add(r).sub(u), p.add(r).add(u), p.sub(r).add(u), uvSprite, col, col, col, col)
}

func flare(p, n vec3, size float64, col rgba) {
	t := fxTime()
	r, u := basis(n)
	sprite(p, size*2.2, col.alpha(0.22))
	sprite(p, size*0.9, col.alpha(0.6))
	sprite(p, size*0.3, rgba{1, 1, 1, 1})
	for k := 0; k < 10; k++ {
		a := t*3 + float64(k)*2*math.Pi/10 + rand.Float64()*0.3
		l := size * (0.6 + 1.2*rand.Float64())
		dir := r.mul(math.Cos(a)).add(u.mul(math.Sin(a)))
		ribbonC(p, p.add(dir.mul(l)), 0.006, rgba{1, 1, 1, 0.9}, col.alpha(0))
	}
	// anamorphic streak
	ribbonC(p.sub(camR.mul(size*3)), p, 0.01, col.alpha(0), col.alpha(0.5))
	ribbonC(p, p.add(camR.mul(size*3)), 0.01, col.alpha(0.5), col.alpha(0))
}
