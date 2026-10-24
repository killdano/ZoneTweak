package main

// The screwdriver. The device's access plate is held on by four Phillips screws, and the
// ratcheting driver for them is somewhere in your room - clipped to a generator panel,
// lying by the radio, under the console, or on the floor by a wall. Nothing points at it.
// The cart has an empty outline where it belongs, its chrome catches the light now and
// then, and if you're stuck on the screws for a while your contact on the radio gives you
// an idea where to look.

import (
	"boundaryvr/sim"
	"math"
	"math/rand"
)

const (
	drvHandle = 0.11 // handle length
	drvShaft  = 0.15 // shaft length
)

type driverKit struct {
	p        *prop
	spot     string // where it started: gen, radio, console, floor_north ...
	gen      int    // which generator's clip, for spot "gen"
	everHeld bool
	stuckT   float64 // seconds on the screws without having found it
	hint     int     // radio hints given
	bareT    float64 // "needs a driver" note over the screws (seconds left)
	bareAt   vec3
	glintT   float64
	clip     func() (vec3, vec3, vec3, vec3) // its outline on the cart
	// seated in a screw head this frame: drawn there, upright, turning with your hand
	seated              bool
	seatP, seatX, seatZ vec3
	seatCol             rgba
	call                toolCall // "it's over by..." once you've put it down and need it again
}

// tip: where the driver's Phillips tip is, and the way it points.
func (k *driverKit) tip() (vec3, vec3) {
	p := k.p
	d := p.Z.mul(-1)
	return p.pos.add(d.mul(drvHandle/2 + drvShaft + 0.008)), d
}

func (g *game) buildDriver() {
	k := &driverKit{}
	k.p = &prop{radius: 0.09, hy: 0.017}
	k.p.grip = &[4]vec3{v3(0, 0, 0), v3(1, 0, 0), v3(0, 1, 0), v3(0, 0, 1)}
	k.clip = func() (vec3, vec3, vec3, vec3) {
		F := g.devFrame()
		// hanging on the cart's side under the housing, tip down
		return F.p(devHW+0.035, 0.44, 0.13), F.z, F.x, F.y
	}
	rng := rand.New(rand.NewSource(g.seed*7919 + 17))
	spots := g.toolSpots(rng)
	s := spots[rng.Intn(len(spots))]
	k.spot, k.gen = s.id, s.gen
	k.p.pos, k.p.X, k.p.Y, k.p.Z = s.pos, s.X, s.Y, s.Z
	if s.docked {
		hp, hx, hy, hz := s.pos, s.X, s.Y, s.Z
		k.p.home = func() (vec3, vec3, vec3, vec3) { return hp, hx, hy, hz }
		k.p.docked = true
	} else {
		k.p.resting = true
	}
	// once you've had it, its home is the outline on the cart
	k.p.onRelease = func() {
		hp, _, _, _ := k.clip()
		if hp.sub(k.p.pos).length() < 0.14 {
			k.p.home, k.p.homeR = k.clip, 0.14
			k.p.docked = true
			ed.sfx(sndClang, hp, 0.4)
		} else if k.p.home != nil {
			if hp2, _, _, _ := k.p.home(); hp2.sub(k.p.pos).length() > 0.14 {
				k.p.docked = false
			}
		}
	}
	g.driver = k
}

// toolSpot: somewhere in your room a tool can be left (lying, or in a clip on a panel).
type toolSpot struct {
	id      string
	pos     vec3
	X, Y, Z vec3
	docked  bool
	gen     int
}

// toolSpots: where the screwdriver and the wrench may turn up: on the floor by a wall, down
// by the radio, under the console, or in a tool clip on a generator panel.
func (g *game) toolSpots(rng *rand.Rand) []toolSpot {
	r := g.r
	var spots []toolSpot
	clear := func(q sim.V2) bool {
		if !sim.InPoly(r.RingPoly, q) || sim.EdgeDist(r.RingPoly, q) < 0.3 {
			return false
		}
		if sim.Dist(q, r.Dev.Pos) < 0.75 || sim.Dist(q, g.consoleSpot()) < 0.5 || sim.Dist(q, g.radio.pos) < 0.45 {
			return false
		}
		for _, gn := range r.Gens {
			if sim.Dist(q, gn.Pos) < 0.5 {
				return false
			}
		}
		return true
	}
	lying := func(id string, q sim.V2, yaw float64) toolSpot {
		z := v3(math.Cos(yaw), 0, math.Sin(yaw))
		return toolSpot{id: id, pos: fv(q, 0.017), Y: v3(0, 1, 0), Z: z, X: v3(0, 1, 0).cross(z).norm()}
	}
	// on the floor by a wall
	c := sim.Centroid(r.RingPoly)
	for _, w := range []struct {
		id  string
		dir sim.V2
	}{{"floor_north", sim.V(0, -1)}, {"floor_east", sim.V(1, 0)}, {"floor_south", sim.V(0, 1)}, {"floor_west", sim.V(-1, 0)}} {
		var last sim.V2
		ok := false
		for t := 0.3; t < 8; t += 0.05 {
			q := c.Add(w.dir.Mul(t))
			if !sim.InPoly(r.RingPoly, q) || sim.EdgeDist(r.RingPoly, q) < 0.45 {
				break
			}
			last, ok = q, true
		}
		if ok && clear(last) {
			spots = append(spots, lying(w.id, last, rng.Float64()*2*math.Pi))
		}
	}
	// by the radio's feet
	for k := 0; k < 8; k++ {
		a := float64(k) * math.Pi / 4
		q := g.radio.pos.Add(sim.V(math.Cos(a), math.Sin(a)).Mul(0.42))
		if sim.InPoly(r.RingPoly, q) && sim.EdgeDist(r.RingPoly, q) > 0.25 && sim.Dist(q, r.Dev.Pos) > 0.7 {
			spots = append(spots, lying("radio", q, a+math.Pi/2))
			break
		}
	}
	// on the console's base plate
	if g.console != nil {
		cb := sim.V(g.console.pos.x, g.console.pos.z)
		back := sim.V(-g.console.n.x, -g.console.n.z)
		q := cb.Add(back.Mul(0.13))
		s := lying("console", q, math.Atan2(g.console.right.z, g.console.right.x))
		s.pos.y = 0.03 + 0.017
		spots = append(spots, s)
	}
	// in a tool clip on a generator panel, hanging tip down
	if len(g.panels_) > 0 {
		// not one the setup console stands in front of
		var ok []int
		for i, gp := range g.panels_ {
			if g.console == nil || gp.F.p(-0.215, 0.86, 0.06).sub(g.console.pos).length() > 0.8 {
				ok = append(ok, i)
			}
		}
		if len(ok) > 0 {
			gi := ok[rng.Intn(len(ok))]
			F := g.panels_[gi].F
			spots = append(spots, toolSpot{id: "gen", pos: F.p(-0.215, 0.86, 0.06), X: F.x, Y: F.z, Z: v3(0, 1, 0), docked: true, gen: gi})
		}
	}
	return spots
}

// driverFrame: the driver's physics, where it may lie, and the hints.
func (g *game) driverFrame(dt float64) {
	k := g.driver
	if k == nil {
		return
	}
	r := g.r
	p := k.p
	if p.held {
		k.everHeld = true
	}
	p.tick(dt)
	// anything thrown out of reach comes back in (you can't walk out of your room for it)
	if !p.held && !sim.InPoly(r.Room, sim.V(p.pos.x, p.pos.z)) {
		c := sim.Centroid(r.Room)
		q := sim.V(p.pos.x, p.pos.z)
		for i := 0; i < 40 && !sim.InPoly(r.Room, q); i++ {
			q = sim.Lerp(q, c, 0.1)
		}
		q = sim.Lerp(q, c, 0.1)
		p.pos, p.vel, p.resting = fv(q, 0.3), vec3{}, false
		ed.sfx(sndClang, p.pos, 0.3)
	}
	k.bareT = math.Max(0, k.bareT-dt)
	g.toolCallFrame(&k.call, fastScrew, p, k.everHeld, "h_d_need", dt)
	// stuck on the screws without it: the contact gives you an idea, a little more each time
	if g.toolNeeded() == fastScrew && r.Phase == sim.Playing && !k.everHeld {
		k.stuckT += dt
		switch {
		case k.hint == 0 && k.stuckT > 18:
			k.hint = 1
			radioSay("h_need", true)
		case k.hint == 1 && k.stuckT > 45:
			k.hint = 2
			radioSay("h_"+k.spot, true)
		case k.hint == 2 && k.stuckT > 95:
			k.hint = 3
			radioSay("h_"+k.spot, true)
		}
	}
}

// bareScrew: what touching a screw with an empty hand gets you.
func (g *game) bareScrew(at vec3) {
	k := g.driver
	if k == nil {
		return
	}
	if k.bareT <= 0 {
		ed.sfx(sndGrind, at, 0.35)
	}
	k.bareT, k.bareAt = 1.6, at
	if k.hint == 0 && !k.everHeld && g.r.Phase == sim.Playing {
		k.hint = 1
		radioSay("h_need", true)
	}
	g.bareCall(&k.call, k.everHeld, "h_d_need")
}

func (g *game) drawDriver() {
	k := g.driver
	if k == nil {
		return
	}
	p := k.p
	t := fxTime()
	pos, Z := p.pos, p.Z
	if k.seated {
		pos, Z = k.seatP, k.seatZ
	}
	back := pos.add(Z.mul(drvHandle / 2))
	front := pos.sub(Z.mul(drvHandle / 2))
	tipBase := front.sub(Z.mul(drvShaft))
	tip := tipBase.sub(Z.mul(0.012))
	saveM := curMat
	defer useMat(saveM)
	// handle: yellow with black grip ribs
	useMat(mPaint)
	cyl(&bEnv, back, front, 0.0155, 0.0175, 14, rgba{0.85, 0.62, 0.06, 1}, true)
	useMat(mRubber)
	for s := 0; s < 5; s++ {
		a := back.add(Z.mul(-0.015 - float64(s)*0.018))
		cyl(&bEnv, a, a.sub(Z.mul(0.008)), 0.0185, 0.0185, 14, rgba{0.05, 0.05, 0.05, 1}, true)
	}
	cyl(&bEnv, front, front.sub(Z.mul(0.01)), 0.012, 0.008, 12, rgba{0.05, 0.05, 0.05, 1}, true)
	// chrome shaft and the Phillips tip
	useMat(mChrome)
	cyl(&bEnv, front, tipBase, 0.0036, 0.0036, 8, cChromeEnv, false)
	cyl(&bEnv, tipBase, tip, 0.0036, 0.0006, 8, cChromeEnv, true)
	// a red stripe down the handle, so you can see it turn
	X := p.X
	if k.seated {
		X = k.seatX
	}
	X = projPerp(X, Z)
	if X.length() > 0.1 {
		X = X.norm()
		useMat(mPaint)
		bevelBox(&bEnv, pos.add(X.mul(0.0172)), X, Z.cross(X), Z, 0.002, 0.0035, drvHandle*0.42, 0.001, rgba{0.75, 0.1, 0.06, 1})
	}
	if k.seated {
		// seated: the tip glows the colour of how it's turning
		sprite(tip, 0.03, k.seatCol.alpha(0.7))
		ringSoft(tipBase, 0.012, 0.003, k.seatCol.alpha(0.9), 14)
	}
	// the glint that gives it away in the dark
	if !p.held {
		k.glintT -= frameDT
		lit := g.flashOn(p.pos)
		if k.glintT <= 0 {
			k.glintT = 2.4 + 1.0*math.Abs(math.Sin(t*1.3))
		}
		if k.glintT > 2.25 || lit > 0.3 {
			q := front.add(tipBase).mul(0.5)
			sprite(q, 0.05+0.05*lit, rgba{1, 1, 0.95, 0.9})
			sprite(q, 0.16+0.1*lit, rgba{1, 0.95, 0.8, 0.22})
		}
	}
	// the tool clip it hangs in on a generator panel
	if k.spot == "gen" && k.p.docked && k.gen < len(g.panels_) {
		F := g.panels_[k.gen].F
		useMat(mSteel)
		bbox(&bEnv, F, -0.215, 0.86, 0.045, 0.024, 0.012, 0.012, 0.003, cSteelEnv)
	}
	// "needs a driver", over the screw you tried with a bare hand
	if k.bareT > 0 {
		cam := ed.hmd.pos()
		q := k.bareAt.add(v3(0, 0.08, 0))
		rr, uu := billboard(cam, q)
		drawTextCentered(&bOpaque, "NEEDS A DRIVER", q, rr, uu, 0.0012, cAmber.alpha(float32(math.Min(1, k.bareT*2))))
	}
}

// drawDriverOutline: the cart's shadow board - an empty outline where the driver belongs.
func (g *game) drawDriverOutline(F xf) {
	k := g.driver
	c := F.p(devHW+0.012, 0.44, 0.13)
	n, up, along := F.x, F.y, F.z
	useMat(mPaint)
	bevelBox(&bEnv, c.sub(n.mul(0.006)), along, up, n, 0.05, 0.17, 0.004, 0.002, rgba{0.14, 0.15, 0.15, 1})
	// the silhouette, painted on: handle above, shaft below
	oc := rgba{0.85, 0.85, 0.8, 1}
	if k != nil && k.p.docked && k.p.home != nil {
		if hp, _, _, _ := k.p.home(); hp.sub(c).length() < 0.1 {
			oc = cEnergy // it's home
		}
	}
	at := func(x, y float64) vec3 { return c.add(along.mul(x)).add(up.mul(y)).add(n.mul(0.0015)) }
	outline := [][2]float64{{-0.018, 0.115}, {0.018, 0.115}, {0.018, 0.005}, {0.004, -0.005}, {0.004, -0.15}, {-0.004, -0.15}, {-0.004, -0.005}, {-0.018, 0.005}}
	for i := range outline {
		a, b := outline[i], outline[(i+1)%len(outline)]
		flatLine(&bOpaque, at(a[0], a[1]), at(b[0], b[1]), n, 0.0022, oc)
	}
	faceText(&bOpaque, "DRIVER", at(0, 0.145), n, up, 0.0008, oc)
	useMat(mSteel)
	bevelBox(&bEnv, at(0, 0.06).add(n.mul(0.006)), along, up, n, 0.026, 0.006, 0.006, 0.002, cSteelEnv)
}
