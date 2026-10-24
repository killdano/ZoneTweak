package main

// The wrench. Whichever of the cart's covers isn't screwed on this round is bolted, and some
// blown generators' covers are too. Like the screwdriver it's somewhere in your room; its
// box end drops over a bolt head when you bring it close, lying across the bolt, and stays
// on until you pull your hand away. Swing the handle round the bolt counter-clockwise to
// back it out (the ratchet lets you swing back for another go).

import (
	"boundaryvr/sim"
	"math"
	"math/rand"
)

const wrenchLen = 0.2 // from your fist to the middle of its box end

type wrenchKit struct {
	p        *prop
	spot     string
	gen      int
	everHeld bool
	stuckT   float64
	hint     int
	bareT    float64
	bareAt   vec3
	glintT   float64
	clip     func() (vec3, vec3, vec3, vec3)
	// on a bolt this frame: drawn there, its handle swinging round with your hand
	seated                   bool
	seatHead, seatDir, seatN vec3
	seatCol                  rgba
	call                     toolCall
}

// head: the middle of the box end (it runs out ahead of your fist).
func (k *wrenchKit) head() vec3 { return k.p.pos.sub(k.p.Z.mul(wrenchLen)) }

func (g *game) buildWrench() {
	k := &wrenchKit{}
	k.p = &prop{radius: 0.09, hy: 0.012}
	k.p.grip = &[4]vec3{v3(0, 0, 0), v3(1, 0, 0), v3(0, 1, 0), v3(0, 0, 1)}
	k.clip = func() (vec3, vec3, vec3, vec3) {
		F := g.devFrame()
		// on its outline on the cart's side, lying along it, box end forward
		return F.p(devHW+0.03, 0.27, -0.06), F.y.mul(-1), F.x, F.z
	}
	rng := rand.New(rand.NewSource(g.seed*104729 + 31))
	spots := g.toolSpots(rng)
	// not where the screwdriver is
	var ok []toolSpot
	for _, s := range spots {
		if g.driver == nil || s.id != g.driver.spot {
			ok = append(ok, s)
		}
	}
	if len(ok) == 0 {
		ok = spots
	}
	s := ok[rng.Intn(len(ok))]
	k.spot, k.gen = s.id, s.gen
	k.p.pos, k.p.X, k.p.Y, k.p.Z = s.pos, s.X, s.Y, s.Z
	if s.docked {
		hp, hx, hy, hz := s.pos, s.X, s.Y, s.Z
		k.p.home = func() (vec3, vec3, vec3, vec3) { return hp, hx, hy, hz }
		k.p.docked = true
	} else {
		k.p.resting = true
		// lying flat: its box end out ahead along the floor
		k.p.pos = k.p.pos.add(k.p.Z.mul(wrenchLen / 2))
	}
	k.p.onRelease = func() {
		hp, _, _, _ := k.clip()
		if hp.sub(k.p.pos).length() < 0.14 {
			k.p.home, k.p.homeR = k.clip, 0.14
			k.p.docked = true
			ed.sfx(sndClang, hp, 0.4)
		}
	}
	g.wrench = k
}

// wrenchFrame: its physics, where it may lie, and the hints.
func (g *game) wrenchFrame(dt float64) {
	k := g.wrench
	if k == nil {
		return
	}
	r := g.r
	p := k.p
	if p.held {
		k.everHeld = true
	}
	p.tick(dt)
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
	g.toolCallFrame(&k.call, fastBolt, p, k.everHeld, "h_w_need", dt)
	// stuck on bolts without it: the contact gives you an idea where it is
	if g.toolNeeded() == fastBolt && r.Phase == sim.Playing && !k.everHeld {
		k.stuckT += dt
		switch {
		case k.hint == 0 && k.stuckT > 18:
			k.hint = 1
			radioSay("h_wneed", true)
		case k.hint == 1 && k.stuckT > 45:
			k.hint = 2
			radioSay("h_"+k.spot, true)
		case k.hint == 2 && k.stuckT > 95:
			k.hint = 3
			radioSay("h_"+k.spot, true)
		}
	}
}

// bareBolt: what grabbing a bolt with an empty hand gets you.
func (g *game) bareBolt(at vec3) {
	k := g.wrench
	if k == nil {
		return
	}
	if k.bareT <= 0 {
		ed.sfx(sndGrind, at, 0.35)
	}
	k.bareT, k.bareAt = 1.6, at
	if k.hint == 0 && !k.everHeld && g.r.Phase == sim.Playing && g.toolNeeded() == fastBolt {
		k.hint = 1
		radioSay("h_wneed", true)
	}
	g.bareCall(&k.call, k.everHeld, "h_w_need")
}

func (g *game) drawWrench() {
	k := g.wrench
	if k == nil {
		return
	}
	p := k.p
	t := fxTime()
	// its line: from the fist to the box end, and which way its flat faces
	fist, Z, N := p.pos, p.Z, p.Y
	head := k.head()
	if k.seated {
		head, N = k.seatHead.add(k.seatN.mul(0.006)), k.seatN
		Z = k.seatDir // the handle runs from the bolt out toward your hand
		fist = head.add(Z.mul(wrenchLen))
	}
	X := N.cross(Z).norm()
	saveM := curMat
	defer useMat(saveM)
	// the handle: a flat chrome-vanadium bar, a rubber grip at the end you hold
	useMat(mChrome)
	mid := fist.add(head).mul(0.5)
	bevelBox(&bEnv, mid, X, N, Z, 0.009, 0.004, wrenchLen/2-0.015, 0.002, cChromeEnv)
	useMat(mRubber)
	bevelBox(&bEnv, fist.add(Z.mul(0.02)), X, N, Z, 0.012, 0.007, 0.05, 0.003, rgba{0.75, 0.12, 0.08, 1})
	// the box end: a ring round the bolt, and an open jaw on the other end
	useMat(mChrome)
	const segs = 12
	for s := 0; s < segs; s++ {
		a0, a1 := float64(s)*2*math.Pi/segs, float64(s+1)*2*math.Pi/segs
		q0 := head.add(X.mul(0.019 * math.Cos(a0))).add(Z.mul(0.019 * math.Sin(a0)))
		q1 := head.add(X.mul(0.019 * math.Cos(a1))).add(Z.mul(0.019 * math.Sin(a1)))
		cyl(&bEnv, q0, q1, 0.0045, 0.0045, 6, cChromeEnv, false)
	}
	jaw := fist.add(Z.mul(0.075))
	for _, sx := range []float64{-1, 1} {
		cyl(&bEnv, jaw.add(X.mul(sx*0.01)), jaw.add(X.mul(sx*0.016)).add(Z.mul(0.03)), 0.004, 0.004, 6, cChromeEnv, true)
	}
	if k.seated {
		sprite(head, 0.035, k.seatCol.alpha(0.7))
		ringSoftAxes(head, X, Z, 0.026, 0.004, k.seatCol.alpha(0.9), 16)
	}
	if !p.held {
		k.glintT -= frameDT
		lit := g.flashOn(p.pos)
		if k.glintT <= 0 {
			k.glintT = 2.6 + 1.1*math.Abs(math.Sin(t*1.1))
		}
		if k.glintT > 2.45 || lit > 0.3 {
			sprite(mid, 0.05+0.05*lit, rgba{1, 1, 0.95, 0.9})
			sprite(mid, 0.16+0.1*lit, rgba{1, 0.95, 0.8, 0.22})
		}
	}
	if k.spot == "gen" && k.p.docked && k.gen < len(g.panels_) {
		F := g.panels_[k.gen].F
		useMat(mSteel)
		bbox(&bEnv, F, -0.215, 0.86, 0.045, 0.024, 0.012, 0.012, 0.003, cSteelEnv)
	}
	if k.bareT > 0 {
		cam := ed.hmd.pos()
		q := k.bareAt.add(v3(0, 0.08, 0))
		rr, uu := billboard(cam, q)
		drawTextCentered(&bOpaque, "NEEDS A WRENCH", q, rr, uu, 0.0012, cAmber.alpha(float32(math.Min(1, k.bareT*2))))
	}
}

// drawWrenchOutline: its painted outline on the cart's side, under the driver's board.
func (g *game) drawWrenchOutline(F xf) {
	k := g.wrench
	c, _, _, _ := k.clip()
	c = c.sub(F.x.mul(0.018))
	n, up, along := F.x, F.y, F.z
	useMat(mPaint)
	bevelBox(&bEnv, c.sub(n.mul(0.006)), along, up, n, 0.15, 0.035, 0.004, 0.002, rgba{0.14, 0.15, 0.15, 1})
	oc := rgba{0.85, 0.85, 0.8, 1}
	if k.p.docked && k.p.home != nil {
		if hp, _, _, _ := k.p.home(); hp.sub(c).length() < 0.1 {
			oc = cEnergy
		}
	}
	at := func(x, y float64) vec3 { return c.add(along.mul(x)).add(up.mul(y)).add(n.mul(0.0015)) }
	outline := [][2]float64{{-0.1, 0.008}, {0.08, 0.008}, {0.08, 0.02}, {0.12, 0.02}, {0.12, -0.02}, {0.08, -0.02}, {0.08, -0.008}, {-0.1, -0.008}}
	for i := range outline {
		a, b := outline[i], outline[(i+1)%len(outline)]
		flatLine(&bOpaque, at(a[0], a[1]), at(b[0], b[1]), n, 0.0022, oc)
	}
	faceText(&bOpaque, "WRENCH", at(-0.02, 0.024), n, up, 0.0008, oc)
}
