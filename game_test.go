//go:build !windows

package main

// Scripted-hand tests: virtual hands grab, twist, crank, poke and pull every control the
// way a player would, and the rules have to respond. This is how the interactions get
// checked without a headset.

import (
	"boundaryvr/sim"
	"math"
	"strings"
	"testing"
)

const tdt = 1.0 / 60

type tester struct {
	t  *testing.T
	g  *game
	hs []*hand
}

// newTester: a 10x10 ft room. start begins the round with the waves held off (a test that
// wants robots spawns them itself).
func newTester(t *testing.T, start bool) *tester {
	simClockOn = true
	gloveFit = [2]hmdMatrix34{ident34(), ident34()}
	aimFit = [2]hmdMatrix34{ident34(), ident34()}
	w, d := 10/3.28084, 10/3.28084
	room := []pt2{{-w / 2, -d / 2}, {w / 2, -d / 2}, {w / 2, d / 2}, {-w / 2, d / 2}}
	hs := []*hand{{idx: 0, grab: -1, hoverV: -1, hoverE: -1, btn: -1}, {idx: 1, grab: -1, hoverV: -1, hoverE: -1, btn: -1}}
	ed.hands = hs
	initHands(hs)
	G = newGame(room, 3)
	G.boltedPlate = false // the plate's screwed, the compartment bolted (tests that want it otherwise flip it)
	G.r.T.ReelLoad = 0    // every fresh spear's a full (green) one, unless a test wants reel spears
	G.r.BookOrder()       // the cart's steps in the content's order (TestEMPStepOrder checks the shuffle)
	G.r.T.Misfire = 0     // the cord always works (TestMisfire* turn it on)
	G.buildDevice()
	tt := &tester{t: t, g: G, hs: hs}
	tt.setHead(v3(0, 1.6, 0.5), v3(0, 0, -1))
	if start {
		G.r.Start()
		G.r.NextWave = 1e9
	}
	for _, h := range hs {
		h.valid, h.aimValid = true, true
	}
	tt.pose(0, v3(-0.3, 0.9, 0.3), v3(0, 0, -1), v3(0, 1, 0))
	tt.pose(1, v3(0.3, 0.9, 0.3), v3(0, 0, -1), v3(0, 1, 0))
	tt.frames(1)
	return tt
}

func setMat(m *hmdMatrix34, pos, X, Y, Z vec3) {
	for r, v := range [3]vec3{X, Y, Z} {
		m[0][r], m[1][r], m[2][r] = float32(v.x), float32(v.y), float32(v.z)
	}
	m[0][3], m[1][3], m[2][3] = float32(pos.x), float32(pos.y), float32(pos.z)
}

func (tt *tester) setHead(p, fwd vec3) {
	Z := fwd.mul(-1).norm()
	X := v3(0, 1, 0).cross(Z).norm()
	Y := Z.cross(X)
	setMat(&ed.hmd, p, X, Y, Z)
}

// pose: put hand i's palm at palm, pointing along fwd with up roughly up.
func (tt *tester) pose(i int, palm, fwd, up vec3) {
	h := tt.hs[i]
	Z := fwd.mul(-1).norm()
	Y := projPerp(up, Z).norm()
	X := Y.cross(Z).norm()
	setMat(&h.m, palm.add(Y.mul(0.005)).add(Z.mul(0.03)), X, Y, Z)
	setMat(&h.aim, palm.add(fwd.norm().mul(0.05)), X, Y, Z)
}

// shift: move hand i's controller by d (no frame runs).
func (tt *tester) shift(i int, d vec3) {
	h := tt.hs[i]
	for _, m := range []*hmdMatrix34{&h.m, &h.aim} {
		m[0][3] += float32(d.x)
		m[1][3] += float32(d.y)
		m[2][3] += float32(d.z)
	}
}

// placePalm: move hand i's palm to p, keeping its orientation (no frame runs).
func (tt *tester) placePalm(i int, p vec3) {
	h := tt.hs[i]
	X, Y, Z := h.m.axis(0), h.m.axis(1), h.m.axis(2)
	setMat(&h.m, p.add(Y.mul(0.005)).add(Z.mul(0.03)), X, Y, Z)
	setMat(&h.aim, p.sub(Z.mul(0.05)), X, Y, Z)
}

// tipOffset: where hand i's index fingertip sits relative to its palm, finger straight.
func (tt *tester) tipOffset(i int) vec3 {
	hc := *tt.hs[i]
	hc.trig = 0
	tmp := *hands[i]
	tmp.h = &hc
	tmp.prevPalm = vec3{}
	tmp.update(0)
	return tmp.tip.sub(tmp.palm)
}

// poseTip: point hand i along fwd with its index fingertip (finger out) at tip.
func (tt *tester) poseTip(i int, tip, fwd, up vec3) {
	tt.hs[i].trig = 0
	tt.pose(i, v3(0, 0, 0), fwd, up)
	tt.placePalm(i, tip.sub(tt.tipOffset(i)))
}

// poke: press a button whose face centre is c and normal n with hand i's fingertip.
func (tt *tester) poke(i int, c, n vec3) {
	up := v3(0, 1, 0)
	if math.Abs(n.y) > 0.9 {
		up = v3(1, 0, 0)
	}
	tt.poseTip(i, c.add(n.mul(0.04)), n.mul(-1), up)
	tt.frames(2)
	tt.poseTip(i, c.sub(n.mul(0.012)), n.mul(-1), up)
	tt.frames(1)
	tt.poseTip(i, c.add(n.mul(0.05)), n.mul(-1), up)
	tt.frames(16) // let the button's cooldown pass
}

// tap: a quick jab at a button - in, out, and on to the next one (about 12 a second)
func (tt *tester) tap(i int, c, n vec3) {
	up := v3(0, 1, 0)
	if math.Abs(n.y) > 0.9 {
		up = v3(1, 0, 0)
	}
	tt.poseTip(i, c.add(n.mul(0.02)), n.mul(-1), up)
	tt.frames(1)
	tt.poseTip(i, c.sub(n.mul(0.01)), n.mul(-1), up)
	tt.frames(1)
	tt.poseTip(i, c.add(n.mul(0.02)), n.mul(-1), up)
	tt.frames(3)
}

func (tt *tester) grip(i int, on bool) {
	if on {
		tt.hs[i].grip = 1
	} else {
		tt.hs[i].grip = 0
	}
}

// bitB: hold (or let go of) B on hand i.
func (tt *tester) bitB(i int, on bool) {
	h := tt.hs[i]
	h.pressed &^= 1 << bitMenu
	if on {
		h.pressed |= 1 << bitMenu
	}
}

func (tt *tester) bits(i int, trigger, a bool) {
	h := tt.hs[i]
	h.pressed = 0
	if trigger {
		h.pressed |= 1 << bitTrigger
		h.trig = 1
	}
	if a {
		h.pressed |= 1 << bitGrip
	}
}

func (tt *tester) frames(n int) {
	for k := 0; k < n; k++ {
		simClock += tdt
		frameDT = tdt
		G.frame(tdt)
		G.buildScene()
		for _, h := range tt.hs {
			h.prev = h.pressed
		}
		tt.g = G
	}
}

// move: slide hand i's palm from where it is to p over n frames, keeping its orientation.
func (tt *tester) move(i int, p vec3, n int) {
	start := hands[i].palm
	for k := 1; k <= n; k++ {
		tt.placePalm(i, start.add(p.sub(start).mul(float64(k)/float64(n))))
		tt.frames(1)
	}
}

// grabAt: palm to p, close the hand; the hand must catch something.
func (tt *tester) grabAt(i int, p vec3) {
	tt.move(i, p, 6)
	tt.grip(i, true)
	tt.frames(2)
	if hands[i].held == nil {
		tt.t.Fatalf("hand %d grabbed nothing at %.2f,%.2f,%.2f", i, p.x, p.y, p.z)
	}
}

// grabMoving: chase something that's moving (the palm keeps landing on it), then grab.
func (tt *tester) grabMoving(i int, at func() vec3) {
	for k := 0; k < 8; k++ {
		tt.placePalm(i, at())
		tt.frames(1)
	}
	tt.placePalm(i, at())
	tt.grip(i, true)
	tt.frames(1)
	if hands[i].held == nil {
		p := at()
		tt.t.Fatalf("hand %d couldn't catch the moving thing at %.2f,%.2f,%.2f", i, p.x, p.y, p.z)
	}
}

func (tt *tester) release(i int) {
	tt.grip(i, false)
	tt.frames(2)
}

// rotate: turn hand i about axis ax through its palm by a radians (no frame runs).
func (tt *tester) rotate(i int, ax vec3, a float64) {
	h := tt.hs[i]
	// pivot on the controller's own palm point (the glove may be fitted away from it)
	palm := h.m.pos().sub(h.m.axis(1).mul(0.005)).sub(h.m.axis(2).mul(0.03))
	X, Y, Z := rotAbout(h.m.axis(0), ax, a), rotAbout(h.m.axis(1), ax, a), rotAbout(h.m.axis(2), ax, a)
	setMat(&h.m, palm.add(Y.mul(0.005)).add(Z.mul(0.03)), X, Y, Z)
	setMat(&h.aim, palm.sub(Z.mul(0.05)), X, Y, Z)
}

// twist: rotate hand i about ax by total radians over steps frames, palm fixed.
func (tt *tester) twist(i int, ax vec3, total float64, steps int) {
	for k := 0; k < steps; k++ {
		tt.rotate(i, ax, total/float64(steps))
		tt.frames(1)
	}
}

// ratchet: turn a knob through total radians the way a wrist does: grab, twist up to 140
// degrees, let go, turn the hand back, grab again.
func (tt *tester) ratchet(i int, at, ax vec3, total float64) {
	left := total
	for math.Abs(left) > 1e-6 {
		step := math.Max(-2.4, math.Min(2.4, left))
		tt.grabAt(i, at)
		tt.twist(i, ax, step, 10)
		tt.release(i)
		tt.rotate(i, ax, -step)
		tt.frames(1)
		left -= step
	}
}

// circle: move hand i's palm round a crank handle's circle (centre c, axle ax, from ref).
func (tt *tester) circle(i int, c, ax, ref vec3, R, turns float64, perTurn int, a0 float64) {
	n := int(turns * float64(perTurn))
	for k := 1; k <= n; k++ {
		a := a0 + float64(k)/float64(perTurn)*2*math.Pi
		tt.placePalm(i, c.add(rotAbout(ref, ax, a).mul(R)).add(ax.mul(0.05)))
		tt.frames(1)
	}
}

// toModule: finish device modules until the nth (0-based) one with this id is current.
func (tt *tester) toModule(id string, nth int) {
	r := tt.g.r
	for r.Dev.Cur < len(r.Dev.Mods) {
		if r.Dev.Mods[r.Dev.Cur].ID == id {
			if nth == 0 {
				break
			}
			nth--
		}
		r.CompleteModule()
		tt.frames(1)
	}
	if r.Dev.Cur >= len(r.Dev.Mods) || r.Dev.Mods[r.Dev.Cur].ID != id {
		tt.t.Fatalf("no module %q", id)
	}
	tt.frames(2)
}

func (tt *tester) mod() devModule { return tt.g.devVis.mods[tt.g.r.Dev.Cur] }

// ---- setup ----

// The trigger on the floor inside your room (a missed tool pull, say) doesn't start a
// magenta wall: they can't go there anyway.
func TestNoWallFromInsideRoom(t *testing.T) {
	tt := newTester(t, true)
	o := v3(0.3, 1.3, 0.5)
	d := v3(-0.2, 0, -0.4).sub(o).norm()
	tt.pose(1, o.sub(d.mul(0.05)), d, v3(0, 1, 0))
	tt.frames(1)
	tt.bits(1, true, false)
	tt.frames(2)
	if hands[1].hit.kind != 3 || hands[1].building {
		t.Fatalf("pointing at the floor of your room shouldn't start a wall (hit %d, building %v)", hands[1].hit.kind, hands[1].building)
	}
	tt.bits(1, false, false)
	tt.hs[1].trig = 0
	tt.frames(1)
}

// Dropped tools come to rest on whatever they land on: a generator's base and the drum round
// its shaft, the cart's bottom tray, the power box - not inside them.
func TestToolsRestOnThings(t *testing.T) {
	tt := newTester(t, true)
	g := tt.g
	p := g.misfire.cutters
	drop := func(at vec3) float64 {
		p.held, p.docked, p.resting, p.onCart, p.home = false, false, false, false, nil
		p.pos, p.vel = at, vec3{}
		tt.frames(90)
		return p.pos.y - p.hy
	}
	gn := g.r.Gens[0].Pos
	if y := drop(fv(gn, 0.8).add(v3(0.2, 0, 0))); math.Abs(y-0.036) > 0.01 {
		t.Fatalf("on a generator's base it should rest at 0.036, got %.3f", y)
	}
	if y := drop(fv(gn, 0.8)); math.Abs(y-0.135) > 0.01 || sim.Dist(sim.V(p.pos.x, p.pos.z), gn) < 0.09 {
		t.Fatalf("dropped onto the shaft it should sit on the drum beside it (y %.3f, %.2f out)", y, sim.Dist(sim.V(p.pos.x, p.pos.z), gn))
	}
	F := g.devFrame()
	if y := drop(F.p(0.1, 0.45, 0.1)); math.Abs(y-cartShelfY) > 0.01 {
		t.Fatalf("in the cart's tray it should rest at %.3f, got %.3f", cartShelfY, y)
	}
	b := g.r.Dev.LeadBox
	if y := drop(fv(b, 0.5)); math.Abs(y-0.106) > 0.01 {
		t.Fatalf("on the power box it should rest at 0.106, got %.3f", y)
	}
}

// Every place the radio can say a tool is has a recording, for both tools.
func TestToolPlacesRecorded(t *testing.T) {
	for _, w := range []string{"cart", "gen", "radio", "console", "turret", "middle", "north", "east", "south", "west"} {
		for _, tool := range []string{"h_d_at_", "h_w_at_"} {
			if voiceClip(tool+w) == nil {
				t.Errorf("no recording for %s%s", tool, w)
			}
		}
	}
}

// A step needs the screwdriver, which you've had and put down: the radio says where it is.
func TestToolCallSaysWhere(t *testing.T) {
	tt := newTester(t, true)
	g := tt.g
	k := g.driver
	k.everHeld = true
	c := sim.Centroid(g.r.Room)
	k.p.held, k.p.docked, k.p.resting, k.p.home = false, false, true, nil
	k.p.pos = fv(c.Add(sim.V(0, -1.0)), k.p.hy)
	gens := g.r.Gens
	g.r.Gens = nil // (nothing else nearby, for this check)
	rp := g.radio.pos
	g.radio.pos = sim.V(50, 50)
	w := g.whereIs(k.p)
	g.r.Gens, g.radio.pos = gens, rp
	if w != "at_north" && w != "at_console" {
		t.Fatalf("north of the middle should be h_at_north, got %s", w)
	}
	tt.toModule("screws", 0)
	tt.frames(60 * 9)
	found := false
	for _, m := range g.radio.inbox {
		if m.id == "h_d_need" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no call about the screwdriver (need %d, call %+v, held %v, inbox %v)", g.toolNeeded(), k.call, k.p.held, g.radio.inbox)
	}
	if cl := msgClips("h_d_need"); len(cl) != 1 || len(cl[0].data) == 0 || &cl[0].data[0] != &voiceClip("h_d_" + g.whereIs(k.p))[0] {
		t.Fatalf("the call should say where the screwdriver is (%d clips)", len(cl))
	}
}

func TestBuildWallWithLaser(t *testing.T) {
	tt := newTester(t, false)
	r := tt.g.r
	p0 := r.Points
	// face south (the console is to the north) and draw out in the hall
	tt.setHead(v3(0, 1.6, 0), v3(0, 0, 1))
	o := v3(0.3, 1.3, 0.5)
	aimAt := func(x, z float64) {
		d := v3(x, 0, z).sub(o).norm()
		tt.pose(1, o.sub(d.mul(0.05)), d, v3(0, 1, 0))
	}
	aimAt(-1.5, 4.5)
	tt.hs[1].trig = 0.5 // half-pull: the beam shows, nothing is drawn yet
	tt.frames(2)
	if !hands[1].beam || hands[1].hit.kind != 3 {
		t.Fatalf("half-pull should show the beam on the floor (beam %v, hit %d)", hands[1].beam, hands[1].hit.kind)
	}
	for k := 0; k <= 20; k++ {
		aimAt(-1.5+3*float64(k)/20, 4.5)
		tt.bits(1, true, false)
		tt.frames(1)
	}
	tt.bits(1, false, false)
	tt.hs[1].trig = 0
	tt.frames(2)
	if len(r.Maze) != 1 {
		t.Fatalf("expected 1 wall, have %d (points %.0f)", len(r.Maze), r.Points)
	}
	w := r.Maze[0]
	if L := sim.Dist(w.A, w.B); L < 2.5 || L > 3.5 {
		t.Fatalf("wall length %.2f", L)
	}
	if r.Points >= p0 {
		t.Fatal("no points spent")
	}
	// hold the trigger on it to take it down
	d := v3(0, 1.0, 4.5).sub(o).norm()
	tt.pose(1, o.sub(d.mul(0.05)), d, v3(0, 1, 0))
	for k := 0; k < int(eraseSecs*60)+6; k++ {
		tt.bits(1, true, false)
		tt.frames(1)
	}
	tt.bits(1, false, false)
	tt.hs[1].trig = 0
	tt.frames(1)
	if len(r.Maze) != 0 {
		t.Fatalf("wall not removed (%d left)", len(r.Maze))
	}
	if math.Abs(r.Points-p0) > 0.5 || r.MazeSpent() > 0.5 {
		t.Fatalf("setup refund: points %.1f vs %.1f, maze spent %.1f", r.Points, p0, r.MazeSpent())
	}
}

func TestWallInsideRoomRefused(t *testing.T) {
	tt := newTester(t, false)
	r := tt.g.r
	if _, why := r.PlaceWall(sim.V(-0.5, 0), sim.V(0.5, 0)); why == "" {
		t.Fatal("a wall inside the room was allowed")
	}
	if _, why := r.PlaceWall(sim.V(-1.4, -2.2), sim.V(1.4, -2.2)); why != "" {
		t.Fatalf("a wall just outside the room was refused: %s", why)
	}
}

func TestDrawingIgnoresTheConsole(t *testing.T) {
	tt := newTester(t, false)
	r := tt.g.r
	c := tt.g.console
	n0 := len(r.Gens)
	// start the wall beside the console, then sweep the beam across it while drawing
	o := c.pos.add(c.n.mul(1.0)).add(v3(0, 0.1, 0))
	aimAt := func(p vec3) {
		d := p.sub(o).norm()
		tt.pose(1, o.sub(d.mul(0.05)), d, v3(0, 1, 0))
	}
	far := c.pos.sub(c.n.mul(3.2))
	far.y = 0
	aimAt(far.add(c.right.mul(1.6)))
	tt.bits(1, true, false)
	tt.frames(1)
	if !hands[1].building {
		t.Fatalf("didn't start drawing (hit %d)", hands[1].hit.kind)
	}
	for k := 0; k <= 30; k++ {
		aimAt(far.add(c.right.mul(1.6 - 3.2*float64(k)/30)))
		tt.bits(1, true, false)
		tt.frames(1)
	}
	tt.bits(1, false, false)
	tt.hs[1].trig = 0
	tt.frames(2)
	if len(r.Maze) != 1 {
		t.Fatalf("the wall drawn across the console didn't land (%d walls)", len(r.Maze))
	}
	if len(r.Gens) != n0 || r.Phase != sim.Setup {
		t.Fatal("sweeping across the console pressed something on it")
	}
}

func TestConsoleAndLightsOutLever(t *testing.T) {
	tt := newTester(t, false)
	r := tt.g.r
	n0 := len(r.Gens)
	c := tt.g.console
	// poke MORE
	b := c.btns[1]
	tt.poke(1, c.pt(b.x, b.y, 0.008), c.n)
	if r.GenHi > n0 && len(r.Gens) != n0+1 {
		t.Fatalf("MORE didn't add a generator: %d -> %d", n0, len(r.Gens))
	}
	// laser FEWER
	fb := c.btns[0]
	sp := c.pt(fb.x, fb.y, 0)
	o := c.pos.add(c.n.mul(0.8)).add(v3(0.2, 0.1, 0))
	d := sp.sub(o).norm()
	tt.pose(1, o, d, v3(0, 1, 0))
	tt.hs[1].trig = 0.5
	tt.frames(1)
	tt.bits(1, true, false)
	tt.frames(1)
	tt.bits(1, false, false)
	tt.hs[1].trig = 0
	tt.frames(1)
	if len(r.Gens) != n0 {
		t.Fatalf("FEWER by laser: %d generators, want %d", len(r.Gens), n0)
	}
	// throw the lever: grab the red handle and haul it toward you and down
	l := tt.g.lightsOut
	tt.pose(1, l.handle().add(c.n.mul(0.2)), c.n.mul(-1), v3(0, 1, 0))
	tt.frames(1)
	tt.grabAt(1, l.handle())
	for k := 1; k <= 30; k++ {
		a := l.a0 + (l.a1-l.a0)*float64(k)/30
		tt.placePalm(1, l.piv.add(rotAbout(l.d0, l.ax, a).mul(l.L)))
		tt.frames(1)
		if k == 15 && r.Phase != sim.Setup {
			t.Fatal("the round started with the lever only half way")
		}
	}
	if r.Phase != sim.Playing {
		t.Fatalf("the lever didn't start the round (phase %d, lever %.2f)", r.Phase, l.ang)
	}
	tt.frames(3)
	if hands[1].held != nil {
		t.Fatal("still holding the lever after the console went away")
	}
	tt.frames(40)
	if envLampK > 0.3 {
		t.Fatalf("lights still on after the lever (%.2f)", envLampK)
	}
	tt.frames(150)
	if envLampK < 0.4 || envLampTint.g > 0.6 {
		t.Fatalf("emergency lights didn't come up red (k %.2f, tint %+v)", envLampK, envLampTint)
	}
}

// ---- generators ----

func TestBreakerAndPump(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	gp := tt.g.panels_[0]
	l := gp.brk
	tt.grabAt(1, l.handle())
	for k := 1; k <= 20; k++ {
		a := l.a0 + (l.a1-l.a0)*float64(k)/20
		tt.move(1, l.piv.add(rotAbout(l.d0, l.ax, a).mul(l.L)), 1)
	}
	if r.Gens[0].State != sim.GenOff {
		t.Fatalf("breaker down didn't switch generator 0 off (state %d, lever %.2f)", r.Gens[0].State, l.ang)
	}
	for k := 1; k <= 20; k++ {
		a := l.a1 - (l.a1-l.a0)*float64(k)/20
		tt.move(1, l.piv.add(rotAbout(l.d0, l.ax, a).mul(l.L)), 1)
	}
	tt.release(1)
	if r.Gens[0].State != sim.GenUp {
		t.Fatalf("breaker up didn't switch it back on (state %d)", r.Gens[0].State)
	}
	r.Gens[0].Charge = 40
	p := gp.pump
	tt.grabAt(1, p.handle())
	for s := 0; s < 3; s++ {
		tt.move(1, p.base.add(p.dir.mul(p.L)), 8)
		tt.move(1, p.base, 8)
	}
	tt.release(1)
	if r.Gens[0].Charge < 40+2.5*r.T.PumpStroke*100-1 {
		t.Fatalf("pumping 3 strokes: charge %.1f", r.Gens[0].Charge)
	}
}

func TestRepairEveryTask(t *testing.T) {
	tt := newTester(t, true)
	g := tt.g
	r := g.r
	i := 1
	gen := &r.Gens[i]
	gen.State, gen.Charge, gen.Order, gen.Done = sim.GenBlown, 0, []string{"patch", "crank", "fuse", "valve", "breaker"}, 0
	tt.frames(2)
	gp := g.panels_[i]
	F := gp.F
	// patch: plug from the base socket up to the top socket
	tt.grabAt(1, gp.plug.pos)
	tt.move(1, F.p(0, 1.0, 0.2), 15)
	tt.move(1, F.p(0, 1.78, 0.09), 15)
	tt.frames(2)
	tt.release(1)
	if gen.Done != 1 {
		t.Fatalf("patch cable: done %d", gen.Done)
	}
	// crank: 5+ turns
	k := gp.crk
	tt.grabAt(1, k.handle())
	tt.circle(1, k.c, k.ax, k.ref, k.R, 5.3, 16, k.ang)
	tt.release(1)
	if gen.Done != 2 {
		t.Fatalf("crank: done %d (turns %.2f)", gen.Done, k.turns)
	}
	// fuse: pull the old one, drop it, seat the new one
	tt.grabAt(1, gp.fuseOld.pos)
	tt.move(1, gp.fuseOld.pos.add(F.z.mul(0.15)), 8)
	tt.release(1)
	if !gp.fuseOut {
		t.Fatal("old fuse didn't come out")
	}
	tt.frames(30)
	tt.grabAt(1, gp.fuseNew.pos)
	tt.move(1, F.p(0.12, 0.92, 0.07), 10)
	tt.frames(2)
	tt.release(1)
	if gen.Done != 3 {
		t.Fatalf("fuse: done %d", gen.Done)
	}
	// valve: 3 turns of the wheel
	v := gp.valve
	tt.grabAt(1, v.handle())
	tt.circle(1, v.c, v.ax, v.ref, v.R, 3.3, 16, v.ang)
	tt.release(1)
	if gen.Done != 4 {
		t.Fatalf("valve: done %d (turns %.2f)", gen.Done, v.turns)
	}
	// breaker: down and back up
	l := gp.brk
	tt.grabAt(1, l.handle())
	for k := 1; k <= 15; k++ {
		tt.move(1, l.piv.add(rotAbout(l.d0, l.ax, l.a0+(l.a1-l.a0)*float64(k)/15).mul(l.L)), 1)
	}
	for k := 1; k <= 15; k++ {
		tt.move(1, l.piv.add(rotAbout(l.d0, l.ax, l.a1-(l.a1-l.a0)*float64(k)/15).mul(l.L)), 1)
	}
	tt.release(1)
	if gen.State != sim.GenUp {
		t.Fatalf("after every task the generator is still state %d (done %d)", gen.State, gen.Done)
	}
}

// ---- the device ----

// fetchDriver: walk the right hand to wherever the driver is and pick it up.
func (tt *tester) fetchDriver() *driverKit {
	k := tt.g.driver
	at := k.p.pos
	tt.pose(1, at.add(v3(0, 0.15, 0)), v3(0, -1, 0), v3(1, 0, 0))
	tt.frames(1)
	tt.grabAt(1, at)
	if hands[1].held != k.p {
		tt.t.Fatal("picked up something other than the driver")
	}
	return k
}

// driveScrew: seat the driver (held in the right hand) in screw i and back it out.
func (tt *tester) driveScrew(m *modScrews, i int) {
	F := tt.g.devFrame()
	head := m.headPos(F, i)
	reach := drvHandle/2 + drvShaft + 0.008
	tt.pose(1, head.add(F.y.mul(reach+0.12)), F.y.mul(-1), F.x)
	tt.frames(1)
	tt.move(1, head.add(F.y.mul(reach)), 6)
	for k := 0; k < 12 && !m.fs.f[i].out; k++ {
		tt.twist(1, F.y, 2.4, 10) // lefty-loosey, as far as a wrist goes
		// lift it off, turn the hand back, set it back on the head
		tt.move(1, hands[1].palm.add(F.y.mul(0.2)), 4)
		tt.twist(1, F.y, -2.4, 4)
		head = m.headPos(F, i)
		tt.move(1, head.add(F.y.mul(reach)), 4)
	}
}

// fetchWrench: the right hand picks up the wrench, wherever it is.
func (tt *tester) fetchWrench() *wrenchKit {
	k := tt.g.wrench
	at := k.p.pos
	tt.pose(1, at.add(v3(0, 0.15, 0)), v3(0, -1, 0), v3(1, 0, 0))
	tt.frames(1)
	tt.grabAt(1, at)
	if hands[1].held != k.p {
		tt.t.Fatal("picked up something other than the wrench")
	}
	return k
}

// driveBolt: put the wrench (in the right hand) over bolt i of set fs and swing it round
// counter-clockwise, lifting it off and setting it back on for each new swing.
func (tt *tester) driveBolt(fs *fastSet, i int) {
	_, n, tan := fs.place(i)
	a0 := 0.0
	for k := 0; k < 14 && !fs.f[i].out; k++ {
		head := fs.headPos(i)
		fwd := rotAbout(tan, n, a0)
		// the box end over the head, the handle back toward the hand
		tt.pose(1, head.sub(fwd.mul(wrenchLen)).add(n.mul(0.1)), fwd, n)
		tt.frames(1)
		tt.placePalm(1, head.sub(fwd.mul(wrenchLen)))
		tt.frames(2)
		for s := 1; s <= 10; s++ {
			a := a0 + 1.4*float64(s)/10
			tt.placePalm(1, head.sub(rotAbout(tan, n, a).mul(wrenchLen)))
			tt.frames(1)
		}
		a0 += 1.4
		tt.move(1, hands[1].palm.add(n.mul(0.25)), 4) // lift it off
	}
}

func TestDeviceScrewsWithDriver(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	tt.toModule("screws", 0)
	m := tt.mod().(*modScrews)
	tt.fetchDriver()
	for i := range m.fs.f {
		tt.driveScrew(m, i)
		if !m.fs.f[i].out {
			t.Fatalf("screw %d still in (%.2f turns)", i, m.fs.f[i].ang/(2*math.Pi))
		}
	}
	tt.frames(40)
	if !r.Dev.Mods[0].Done {
		t.Fatalf("plate didn't come off (cur %d)", r.Dev.Cur)
	}
}

func TestNoWigglingItOut(t *testing.T) {
	tt := newTester(t, true)
	tt.toModule("screws", 0)
	m := tt.mod().(*modScrews)
	tt.fetchDriver()
	F := tt.g.devFrame()
	head := m.headPos(F, 0)
	reach := drvHandle/2 + drvShaft + 0.008
	tt.pose(1, head.add(F.y.mul(reach)), F.y.mul(-1), F.x)
	tt.frames(2)
	// the wrong way: it won't go, it shows red and shakes
	tight := false
	for k := 0; k < 15; k++ {
		tt.twist(1, F.y, -0.2, 1)
		tight = tight || m.fs.feel == feelTight
	}
	if m.fs.f[0].ang != 0 {
		t.Fatalf("turning the wrong way moved the screw (%.2f)", m.fs.f[0].ang)
	}
	if !tight {
		t.Fatal("turning the wrong way didn't show red")
	}
	// the right way: yellow while it breaks loose, then green as it comes out
	var seen []int
	for k := 0; k < 20; k++ {
		tt.twist(1, F.y, 0.1, 1)
		if len(seen) == 0 || seen[len(seen)-1] != m.fs.feel {
			seen = append(seen, m.fs.feel)
		}
	}
	if len(seen) < 2 || seen[0] != feelBreak || seen[len(seen)-1] != feelFree {
		t.Fatalf("the right way should go yellow then green, went %v", seen)
	}
	if m.fs.f[0].ang < 1.2 {
		t.Fatalf("the right way didn't back it out (%.2f)", m.fs.f[0].ang)
	}
	// wiggling it back and forth on the head gets you nowhere...
	was := m.fs.f[0].ang
	for k := 0; k < 6; k++ {
		tt.twist(1, F.y, -0.6, 4)
		tt.twist(1, F.y, 0.6, 4)
	}
	if m.fs.f[0].ang-was > 0.02 {
		t.Fatalf("wiggling the driver backed the screw out (%.2f)", m.fs.f[0].ang-was)
	}
	// ...lift it off, turn the hand back, set it back on: now another turn counts
	tt.move(1, hands[1].palm.add(F.y.mul(0.2)), 4)
	tt.twist(1, F.y, -1.5, 4)
	tt.move(1, head.add(F.y.mul(reach)), 4)
	tt.twist(1, F.y, 1.0, 8)
	if m.fs.f[0].ang-was < 0.8 {
		t.Fatalf("set back on, another turn should back it out further (%.2f)", m.fs.f[0].ang-was)
	}
}

// The tip is magnetic: near enough and roughly downward, it pulls into the head and sits
// there upright; pulled well clear, it lets go.
func TestDriverTipIsMagnetic(t *testing.T) {
	tt := newTester(t, true)
	tt.toModule("screws", 0)
	m := tt.mod().(*modScrews)
	k := tt.fetchDriver()
	F := tt.g.devFrame()
	head := m.headPos(F, 1)
	reach := drvHandle/2 + drvShaft + 0.008
	// 4 cm off the head and tipped 35 degrees
	dir := rotAbout(F.y.mul(-1), F.x, 35*math.Pi/180)
	tt.pose(1, head.add(F.x.mul(0.04)).sub(dir.mul(reach)), dir, F.z)
	tt.frames(3)
	if m.fs.eng != 1 || !k.seated {
		t.Fatalf("the tip didn't pull into the screw (eng %d)", m.fs.eng)
	}
	tip := k.seatP.sub(k.seatZ.mul(drvHandle/2 + drvShaft + 0.008))
	if tip.sub(head).length() > 0.002 || k.seatZ.dot(F.y) < 0.999 {
		t.Fatal("seated, the driver should sit upright in the head")
	}
	// tipping the hand right over doesn't pull it out...
	X, _, _ := hands[1].axes()
	palm := hands[1].palm
	tt.rotate(1, X, 70*math.Pi/180)
	tt.placePalm(1, palm)
	tt.frames(3)
	if m.fs.eng != 1 || !k.seated {
		t.Fatal("tilting the hand pulled the driver out of the screw")
	}
	// ...pulling the hand 15 cm away does
	tt.move(1, hands[1].palm.add(F.y.mul(0.15)), 6)
	if m.fs.eng != -1 || k.seated {
		t.Fatal("pulled well clear, the tip is still stuck in the screw")
	}
}

func TestBareHandScrewWontTurn(t *testing.T) {
	tt := newTester(t, true)
	tt.toModule("screws", 0)
	m := tt.mod().(*modScrews)
	k := tt.g.driver
	F := tt.g.devFrame()
	at := m.headPos(F, 1)
	tt.pose(1, at.add(F.y.mul(0.15)), v3(0, -1, 0), F.x)
	tt.frames(1)
	tt.grabAt(1, at)
	tt.twist(1, F.y, 2.4, 10)
	tt.release(1)
	if m.fs.f[1].ang != 0 {
		t.Fatal("a bare hand turned a screw")
	}
	if k.bareT <= 0 || k.hint != 1 {
		t.Fatalf("no 'needs a driver' feedback (note %.1f, hint %d)", k.bareT, k.hint)
	}
}

func TestDriverHintsEscalate(t *testing.T) {
	tt := newTester(t, true)
	tt.toModule("screws", 0)
	k := tt.g.driver
	for _, want := range []struct {
		at   float64
		hint int
	}{{10, 0}, {20, 1}, {50, 2}, {100, 3}} {
		for k.stuckT < want.at {
			tt.frames(30)
		}
		if k.hint != want.hint {
			t.Fatalf("at %.0f s stuck: hint %d, want %d", k.stuckT, k.hint, want.hint)
		}
	}
	if voiceClip("h_"+k.spot) == nil {
		t.Fatalf("no voice line for the hint about %q", k.spot)
	}
}

func TestDriverSpotsAreReachable(t *testing.T) {
	seen := map[string]bool{}
	for seed := int64(1); seed <= 30; seed++ {
		simClockOn = true
		w, d := 10/3.28084, 10/3.28084
		G = newGame([]pt2{{-w / 2, -d / 2}, {w / 2, -d / 2}, {w / 2, d / 2}, {-w / 2, d / 2}}, seed)
		k := G.driver
		seen[k.spot] = true
		p := sim.V(k.p.pos.x, k.p.pos.z)
		if !sim.InPoly(G.r.Room, p) {
			t.Fatalf("seed %d: the driver (%s) is outside your room", seed, k.spot)
		}
		if k.p.pos.y < 0 || k.p.pos.y > 1.6 {
			t.Fatalf("seed %d: the driver (%s) is out of reach at %.2f m", seed, k.spot, k.p.pos.y)
		}
	}
	if len(seen) < 3 {
		t.Fatalf("the driver only ever turns up in %v", seen)
	}
}

func TestDriverGoesBackOnTheCart(t *testing.T) {
	tt := newTester(t, true)
	k := tt.fetchDriver()
	hp, _, _, _ := k.clip()
	tt.move(1, hp, 20)
	tt.release(1)
	if !k.p.docked {
		t.Fatal("the driver didn't go back in its outline")
	}
	h := tt.g.devVis.handles[0]
	at := h.gPos()
	tt.grabAt(1, at)
	tt.move(1, at.add(v3(0.5, 0, 0)), 20)
	tt.release(1)
	tt.frames(2)
	hp2, _, _, _ := k.clip()
	if k.p.pos.sub(hp2).length() > 0.01 {
		t.Fatal("the docked driver stayed behind when the cart moved")
	}
}

func TestThrownDriverComesBack(t *testing.T) {
	tt := newTester(t, true)
	k := tt.fetchDriver()
	r := tt.g.r
	at := hands[1].palm
	tt.move(1, at.add(v3(0, 0.3, -1.2)), 5) // a hard throw toward the north wall
	tt.release(1)
	k.p.vel = v3(0, 2, -9)
	tt.frames(120)
	if !sim.InPoly(r.Room, sim.V(k.p.pos.x, k.p.pos.z)) {
		t.Fatalf("the driver ended up out of reach at %.2f,%.2f", k.p.pos.x, k.p.pos.z)
	}
}

func TestDeviceWires(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	tt.toModule("wires", 0)
	m := tt.mod().(*modWires)
	cur := r.Dev.Cur
	for i, p := range m.plugs {
		F := tt.g.devFrame()
		tt.pose(1, p.pos.add(F.x.mul(0.2)), F.x.mul(-1), v3(0, 1, 0))
		tt.frames(1)
		tt.grabAt(1, p.pos)
		s := m.socket(F, m.order[i]).add(F.y.mul(0.03))
		tt.move(1, s.add(F.y.mul(0.15)), 12)
		tt.move(1, s, 8)
		tt.frames(2)
		tt.release(1)
		if !m.in[i] {
			t.Fatalf("plug %d didn't seat (dist %.3f)", i, p.pos.sub(s).length())
		}
	}
	tt.frames(2)
	if !r.Dev.Mods[cur].Done {
		t.Fatal("wiring done but the module isn't")
	}
}

func TestDeviceSimon(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	for i := range r.Dev.Mods {
		if r.Dev.Mods[i].ID == "game" {
			r.Dev.Mods[i].Game = "simon"
		}
	}
	tt.toModule("game", 0)
	m := tt.mod().(*modGame)
	cur := r.Dev.Cur
	if m.kind != "simon" {
		t.Fatalf("game is %q", m.kind)
	}
	// nothing plays until you press START
	tt.frames(90)
	if m.show < 0.6-1e-9 || m.input {
		t.Fatal("simon started playing before anyone pressed START")
	}
	m.seq = []int{1, 1, 3} // the same colour twice running, tapped fast
	tt.poke(1, m.go_.c, m.go_.n)
	for round := 0; round < 3; round++ {
		for k := 0; k < 60*10 && !m.input; k++ {
			tt.frames(1)
		}
		if !m.input {
			t.Fatalf("round %d: the sequence never finished playing", round)
		}
		seq := append([]int(nil), m.seq...)
		for _, s := range seq {
			b := m.btn[s]
			tt.tap(1, b.c, b.n)
		}
		if r.Dev.Mods[cur].Done {
			break
		}
		if m.round != round+1 {
			t.Fatalf("round %d not passed (round %d step %d)", round, m.round, m.step)
		}
	}
	if !r.Dev.Mods[cur].Done {
		t.Fatal("simon passed but the module isn't done")
	}
}

func TestDeviceSimonWrongPress(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	for i := range r.Dev.Mods {
		if r.Dev.Mods[i].ID == "game" {
			r.Dev.Mods[i].Game = "simon"
		}
	}
	tt.toModule("game", 0)
	m := tt.mod().(*modGame)
	cur := r.Dev.Cur
	start := r.Work()
	tt.frames(90)
	tt.poke(1, m.go_.c, m.go_.n)
	// get one round right...
	for k := 0; k < 60*10 && !m.input; k++ {
		tt.frames(1)
	}
	for _, s := range append([]int(nil), m.seq...) {
		tt.poke(1, m.btn[s].c, m.btn[s].n)
	}
	tt.frames(2)
	w0 := r.Work()
	if w0 <= start {
		t.Fatal("a round of simon made no progress")
	}
	// ...then press the wrong one
	for k := 0; k < 60*10 && !m.input; k++ {
		tt.frames(1)
	}
	wrong := (m.seq[0] + 1) % 4
	tt.poke(1, m.btn[wrong].c, m.btn[wrong].n)
	tt.frames(3)
	if r.Work() >= w0 {
		t.Fatalf("a wrong press cost nothing (work %.1f -> %.1f)", w0, r.Work())
	}
	if r.Dev.Cur != cur || r.Work() < start-1e-6 {
		t.Fatalf("a wrong press threw you back out of the game (module %d, work %.1f < %.1f)", r.Dev.Cur, r.Work(), start)
	}
	if m := tt.mod().(*modGame); m.kind == "simon" {
		t.Fatalf("a botched game should come back as a different one, got %q", m.kind)
	}
}

// The cart's power leads: in at the start; a warden taking the cart rips them out; the cord
// won't set it off till both are plugged back into the back of the cart.
func TestPowerLeads(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	d := &r.Dev
	k := tt.g.leads
	tt.frames(2)
	if d.Leads != 2 || !k.plugs[0].docked || !k.plugs[1].docked {
		t.Fatalf("leads should start plugged in (%d)", d.Leads)
	}
	tt.toModule("ripcord", 0)
	b := r.Spawn(sim.Walker, 0, "")
	b.Pos = d.Pos.Add(sim.V(0.9, 0))
	r.Steal(b)
	tt.frames(2)
	if d.Leads != 0 || k.plugs[0].docked || k.plugs[1].docked {
		t.Fatalf("taking the cart should rip the leads out (%d)", d.Leads)
	}
	// it's back (the warden let go of it right there)
	d.State, d.Carrier = sim.DevHome, 0
	b.State, b.Wait, b.Pos = sim.RWait, 100, d.Pos.Add(sim.V(5, 0))
	tt.frames(90)
	m := tt.mod().(*modRip)
	fireCart(tt, m)
	if r.Phase != sim.Playing {
		t.Fatal("the cord set it off with the leads out")
	}
	// plug them back in
	for i, p := range k.plugs {
		tt.pose(1, p.pos.add(v3(0, 0.15, 0)), v3(0, -1, 0), v3(0, 0, 1))
		tt.frames(1)
		tt.grabAt(1, p.pos)
		if hands[1].held != p {
			t.Fatalf("lead %d: grabbed something else", i)
		}
		at, _, _, _ := p.home()
		tt.move(1, at.add(hands[1].palm.sub(p.pos)), 12)
		tt.release(1)
		tt.move(1, hands[1].palm.add(v3(0, 0.3, 0)), 4)
		if !p.docked {
			t.Fatalf("lead %d didn't go in (%.3f from its socket)", i, p.pos.sub(at).length())
		}
	}
	tt.frames(2)
	if d.Leads != 2 {
		t.Fatalf("both in but the sim says %d", d.Leads)
	}
	fireCart(tt, m)
	if r.Phase != sim.Won {
		t.Fatalf("leads in, three hard pulls, and no blast (phase %d)", r.Phase)
	}
}

// fireCart: start the engine (hard pulls till it catches), let it rev till the button
// lights, push it.
func fireCart(tt *tester, m *modRip) {
	for i := 0; i < 3 && !m.running; i++ {
		m.c.onPull(2)
		tt.frames(2)
	}
	for k := 0; k < 60*8 && !m.lit() && m.running; k++ {
		tt.frames(1)
	}
	m.push(tt.g)
	tt.frames(2)
}

// misfired: through to the cord and pull it: it misfires.
func misfired(tt *tester) (*mfKit, *modRip) {
	r := tt.g.r
	r.T.Misfire = 1
	tt.toModule("ripcord", 0)
	m := tt.mod().(*modRip)
	fireCart(tt, m)
	if !r.Dev.Misfire || r.Phase != sim.Playing {
		tt.t.Fatalf("the cord should have misfired (misfire %v, phase %d)", r.Dev.Misfire, r.Phase)
	}
	return tt.g.misfire, m
}

// openHatch: the hatch's four corners out with the right tool, then the handle pulled up.
func openHatch(tt *tester, k *mfKit) {
	if k.kind == fastScrew {
		tt.fetchDriver()
		for i := range k.fs.f {
			tt.driveFast(k.fs, i)
		}
	} else {
		tt.fetchWrench()
		for i := range k.fs.f {
			tt.driveBolt(k.fs, i)
		}
	}
	if !k.fs.allOut() {
		tt.t.Fatalf("the hatch's corners are still in (%.2f)", k.fs.progress())
	}
	// put the tool down out of the way
	tt.move(1, v3(0.6, 0.9, 0.6), 6)
	tt.release(1)
	hp := k.handlePos()
	tt.pose(0, hp.add(v3(0, 0.12, 0)), v3(0, -1, 0), v3(1, 0, 0))
	tt.frames(1)
	tt.grabAt(0, hp)
	if hands[0].held != k.handle {
		tt.t.Fatal("didn't get the hatch handle")
	}
	tt.move(0, hands[0].palm.add(v3(0, 0.5, 0)), 15)
	tt.release(0)
	tt.frames(2)
	if !k.raised || !k.open() {
		tt.t.Fatalf("the guts didn't come up (lift %.2f)", k.lift)
	}
}

// snipAt: the cutters (in the right hand) with their jaws on p, squeezed.
func snipAt(tt *tester, p vec3) {
	fwd := v3(0, -1, 0.0001).norm()
	tt.pose(1, p.sub(fwd.mul(0.135)), fwd, v3(1, 0, 0))
	tt.frames(1)
	tt.bits(1, true, false)
	tt.frames(1)
	tt.bits(1, false, false)
	tt.hs[1].trig = 0
	tt.frames(1)
}

// setFix: the fix, set for a test (the faulty flags to match).
func setFix(k *mfKit, orders ...mfOrder) {
	for _, pt := range k.parts {
		pt.faulty = false
	}
	for _, o := range orders {
		switch o.kind {
		case ordFuse:
			k.colSlot(partFuse, o.i).holds.faulty = true
		case ordJumper:
			k.colSlot(partJumper, o.i).holds.faulty = true
		case ordCard, ordRelay, ordCap:
			k.slots[o.i].holds.faulty = true
		}
	}
	k.orders = orders
}

// slotOf: the index of the first slot of this kind.
func slotOf(k *mfKit, kind int) int {
	for i, s := range k.slots {
		if s.kind == kind {
			return i
		}
	}
	return -1
}

func spareOf(k *mfKit, kind, col int) *mfPart {
	for _, pt := range k.spares {
		if pt.kind == kind && (kind != partFuse || pt.col == col) {
			return pt
		}
	}
	return nil
}

// carry: hand i takes p's prop and lets go of it at "at".
func carry(tt *tester, i int, p *prop, at vec3) {
	tt.pose(i, p.pos.add(v3(0, 0.1, 0)), v3(0, -1, 0), v3(1, 0, 0))
	tt.frames(1)
	tt.grabAt(i, p.pos)
	if hands[i].held != p {
		tt.t.Fatal("grabbed the wrong thing")
	}
	tt.move(i, at.add(hands[i].palm.sub(p.pos)), 12)
	tt.release(i)
}

// The misfire: the corners off the hatch, the guts up, a wire cut and a fuse swapped as the
// radio says, the handle back down - and then the cord sets it off.
func TestMisfireRepair(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	k, m := misfired(tt)
	openHatch(tt, k)
	setFix(k, mfOrder{kind: ordWire, i: 2}, mfOrder{kind: ordFuse, i: 1})
	// the cutters off the shelf
	tt.pose(1, k.cutters.pos.add(v3(0, 0.12, 0)), v3(0, -1, 0), v3(1, 0, 0))
	tt.frames(1)
	tt.grabAt(1, k.cutters.pos)
	if hands[1].held != k.cutters {
		t.Fatal("didn't get the cutters")
	}
	w := k.wires[2]
	snipAt(tt, w.world[len(w.world)/2])
	if !w.cut || !k.orders[0].done {
		t.Fatalf("the wire didn't cut (cut %v)", w.cut)
	}
	// out with the blown fuse, in with the blue spare
	f := k.colSlot(partFuse, 1).holds
	carry(tt, 0, f.p, f.p.pos.add(v3(0.3, 0.1, 0.3)))
	if k.orders[0].kind != ordWire || !k.orders[0].done {
		t.Fatal("pulling the blown fuse counted as a mistake")
	}
	at, _, _, _ := k.slotPose(k.colSlot(partFuse, 1))
	carry(tt, 0, spareOf(k, partFuse, 1).p, at)
	if !k.allDone() {
		t.Fatalf("the fix isn't done: %+v", k.orders)
	}
	// the handle back down
	tt.move(1, v3(0.6, 0.9, 0.6), 6)
	tt.release(1)
	hp := k.handlePos()
	tt.pose(0, hp.add(v3(0, 0.12, 0)), v3(0, -1, 0), v3(1, 0, 0))
	tt.frames(1)
	tt.grabAt(0, hp)
	tt.move(0, hands[0].palm.add(v3(0, -0.5, 0)), 15)
	tt.release(0)
	tt.frames(2)
	if r.Dev.Misfire {
		t.Fatalf("closed up with the fix done, still misfired (lift %.2f)", k.lift)
	}
	r.T.Misfire2 = 0
	fireCart(tt, m)
	if r.Phase != sim.Won {
		t.Fatalf("fixed, and the cord still doesn't set it off (phase %d)", r.Phase)
	}
}

// The wrong wire, or a part that's fine: he starts you over on a different fix. The burnt
// card, the cracked relay and the bulging capacitor: out, and a spare in. A cable: cut, then
// patched. A jumper: just pulled.
func TestMisfireWrongAndParts(t *testing.T) {
	tt := newTester(t, true)
	k, _ := misfired(tt)
	openHatch(tt, k)
	setFix(k, mfOrder{kind: ordWire, i: 0})
	tt.pose(1, k.cutters.pos.add(v3(0, 0.12, 0)), v3(0, -1, 0), v3(1, 0, 0))
	tt.frames(1)
	tt.grabAt(1, k.cutters.pos)
	w := k.wires[3]
	snipAt(tt, w.world[len(w.world)/2])
	if !k.reset || k.wires[3] != w {
		t.Fatal("cutting the wrong wire should sink the guts back in before anything changes")
	}
	tt.frames(int(2 / tdt))
	if k.reset || k.lift != 0 || k.wires[3] == w || !tt.g.r.Dev.Misfire || k.heard || k.raised {
		t.Fatalf("back inside, it should be a fresh fix (reset %v, lift %.2f)", k.reset, k.lift)
	}
	liftGuts(tt, k)
	// a cable: cut it, patch it
	setFix(k, mfOrder{kind: ordCable, i: 8})
	c := k.wires[8]
	snipAt(tt, c.world[len(c.world)/2])
	if !c.cut || k.allDone() {
		t.Fatalf("cut the cable: it should want patching now (cut %v)", c.cut)
	}
	tt.move(1, v3(0.6, 0.9, 0.6), 6)
	tt.release(1)
	a, b := c.cutEnds()
	carry(tt, 0, spareOf(k, partPatch, 0).p, a.add(b).mul(0.5))
	if !c.patched || !k.allDone() {
		t.Fatalf("the patch didn't go on (patched %v)", c.patched)
	}
	// each part that goes bad: out with it, a spare in
	for _, kind := range []int{partCard, partRelay, partCap} {
		k.generate(1)
		k.raised = true
		si := slotOf(k, kind)
		setFix(k, mfOrder{kind: map[int]int{partCard: ordCard, partRelay: ordRelay, partCap: ordCap}[kind], i: si})
		bad := k.slots[si].holds
		carry(tt, 0, bad.p, bad.p.pos.add(v3(0.3, 0.1, 0.3)))
		if k.slots[si].holds != nil || k.allDone() || len(k.orders) != 1 {
			t.Fatalf("part %d: pulling the bad one should leave the slot empty", kind)
		}
		at, _, _, _ := k.slotPose(k.slots[si])
		carry(tt, 0, spareOf(k, kind, 0).p, at)
		if !k.allDone() {
			t.Fatalf("part %d: the spare in its slot should finish it: %+v", kind, k.orders)
		}
	}
	// a jumper: pulling it is the fix
	k.generate(1)
	k.raised = true
	setFix(k, mfOrder{kind: ordJumper, i: 2})
	j := k.colSlot(partJumper, 2).holds
	carry(tt, 0, j.p, j.p.pos.add(v3(0.3, 0.1, 0.3)))
	if !k.allDone() {
		t.Fatal("pulling the jumper should be the fix")
	}
	// a fine card pulled: wrong
	k.generate(1)
	k.raised = true
	setFix(k, mfOrder{kind: ordFuse, i: 0})
	good := k.slots[slotOf(k, partCard)].holds
	tt.pose(0, good.p.pos.add(v3(0, 0.1, 0)), v3(0, -1, 0), v3(1, 0, 0))
	tt.frames(1)
	tt.move(0, good.p.pos, 4)
	tt.grip(0, true)
	tt.frames(3)
	tt.release(0)
	tt.frames(int(2 / tdt))
	if k.slots[slotOf(k, partCard)].holds == good || hands[0].held != nil {
		t.Fatal("pulling a card that's fine should start a fresh fix (and the old card's gone)")
	}
}

// liftGuts: pull the handle back up (the hatch already open).
func liftGuts(tt *tester, k *mfKit) {
	tt.move(1, v3(0.6, 0.9, 0.6), 6)
	hp := k.handlePos()
	tt.pose(0, hp.add(v3(0, 0.12, 0)), v3(0, -1, 0), v3(1, 0, 0))
	tt.frames(1)
	tt.grabAt(0, hp)
	tt.move(0, hands[0].palm.add(v3(0, 0.5, 0)), 15)
	tt.release(0)
	tt.frames(2)
	if !k.raised || !k.open() {
		tt.t.Fatalf("the guts didn't come back up (lift %.2f)", k.lift)
	}
}

// Every fix the radio can ask for has a recording.
func TestMisfireClipsExist(t *testing.T) {
	tt := newTester(t, true)
	k := tt.g.misfire
	ids := []string{"x_mf_start", "x_mf_again", "x_mf_up", "x_mf_wrong", "x_mf_patch", "x_mf_done", "mf_orders", "mf_also", "mf_cutters"}
	for i := range k.wires {
		kind := ordWire
		if k.wires[i].thick {
			kind = ordCable
		}
		ids = append(ids, k.orderClip(mfOrder{kind: kind, i: i}))
	}
	for c := range mfFuseNames {
		ids = append(ids, k.orderClip(mfOrder{kind: ordFuse, i: c}))
	}
	for c := range mfJumperNames {
		ids = append(ids, k.orderClip(mfOrder{kind: ordJumper, i: c}))
	}
	for _, kd := range []int{ordCard, ordRelay, ordCap} {
		ids = append(ids, k.orderClip(mfOrder{kind: kd}))
	}
	// every wire colour pair, not just this board's
	for a := range mfColNames {
		for b := a; b < len(mfColNames); b++ {
			ids = append(ids, k.orderClip(mfOrder{kind: ordWire, i: 0}))
			k.wires[0].a, k.wires[0].b = a, b
		}
	}
	for _, id := range ids {
		if voiceClip(id) == nil {
			t.Errorf("no recording for %q", id)
		}
	}
}

// The fix call: the orders first, then what each thing looks like; and if you're still at
// it 45 s after hearing it, he calls back with what to look for.
func TestMisfireDetails(t *testing.T) {
	tt := newTester(t, true)
	k, _ := misfired(tt)
	openHatch(tt, k)
	setFix(k, mfOrder{kind: ordFuse, i: 2}, mfOrder{kind: ordRelay, i: slotOf(k, partRelay)})
	cl := k.orderClips()
	want := []string{"mf_orders", "mf_f_green", "mf_also", "mf_relay", "mf_look", "mf_d_fuse", "mf_d_relay"}
	if len(cl) != len(want) {
		t.Fatalf("the call has %d parts, want %d", len(cl), len(want))
	}
	for i, id := range want {
		if &cl[i].data[0] != &voiceClip(id)[0] {
			t.Fatalf("part %d should be %s", i, id)
		}
	}
	k.heard, k.heardT = true, tt.g.t
	tt.frames(60 * 46)
	found := false
	for _, m := range tt.g.radio.inbox {
		found = found || m.id == "x_mf_help"
	}
	if !found || len(msgClips("x_mf_help")) != 3 {
		t.Fatalf("no call back with the details (found %v)", found)
	}
}

// Once in a while it misfires again: the handle back up, and a two-part fix.
func TestMisfireTwice(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	k, m := misfired(tt)
	r.T.Misfire2 = 1
	k.orders = []mfOrder{{kind: ordJumper, i: 0, done: true}}
	r.FixMisfire()
	tt.frames(1)
	fireCart(tt, m)
	if !r.Dev.Misfire || r.Dev.Misfires != 2 || len(k.orders) != 2 {
		t.Fatalf("it should misfire a second time with a two-part fix (misfire %v, %d times, %d orders)", r.Dev.Misfire, r.Dev.Misfires, len(k.orders))
	}
	k.orders = []mfOrder{{kind: ordJumper, i: 0, done: true}}
	r.FixMisfire()
	fireCart(tt, m)
	if r.Phase != sim.Won {
		t.Fatalf("never a third misfire (phase %d)", r.Phase)
	}
}

// The cart's steps come in a different order each round: the plate before the capacitor
// bank under it, the cord always last, the two games never back to back.
func TestEMPStepOrder(t *testing.T) {
	seen := map[string]bool{}
	for seed := int64(1); seed <= 40; seed++ {
		r := sim.NewRound([]sim.V2{sim.V(-1.5, -1.5), sim.V(1.5, -1.5), sim.V(1.5, 1.5), sim.V(-1.5, 1.5)}, seed, nil, nil)
		ms := r.Dev.Mods
		var ids []string
		pos := map[string]int{}
		games := 0
		for i, m := range ms {
			ids = append(ids, m.ID)
			if _, ok := pos[m.ID]; !ok {
				pos[m.ID] = i
			}
			if m.ID == "game" {
				want := "THE DEVICE WANTS A GAME"
				if games > 0 {
					want = "THE DEVICE WANTS ANOTHER GAME"
				}
				if m.Name != want {
					t.Fatalf("seed %d: game %d named %q", seed, games, m.Name)
				}
				games++
				if i > 0 && ms[i-1].ID == "game" {
					t.Fatalf("seed %d: games back to back: %v", seed, ids)
				}
			}
		}
		if ms[len(ms)-1].ID != "ripcord" || pos["wires"] < pos["screws"] {
			t.Fatalf("seed %d: bad order %v", seed, ids)
		}
		seen[strings.Join(ids, ",")] = true
	}
	if len(seen) < 15 {
		t.Fatalf("only %d different orders in 40 rounds", len(seen))
	}
}

// Dan's loop: the wires went in fine, then the game after them failed on its own before he
// got round to it, threw him back into the wires and popped his plugs out.
func TestBotchedGameKeepsTheWiresIn(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	for i := range r.Dev.Mods {
		if r.Dev.Mods[i].ID == "game" {
			r.Dev.Mods[i].Game = "needle"
		}
	}
	tt.toModule("wires", 0)
	wires := tt.mod().(*modWires)
	for i := range wires.in {
		wires.in[i] = true
	}
	tt.frames(3)
	m := tt.mod().(*modGame)
	cur := r.Dev.Cur
	// nobody touches the knob for 20 s: the needle just waits
	for k := 0; k < 60*20; k++ {
		tt.frames(1)
	}
	if r.Dev.Cur != cur || m.needle != 0 {
		t.Fatalf("the needle game ran away before anyone took the knob (module %d, needle %.2f)", r.Dev.Cur, m.needle)
	}
	// and when a game is botched, the wires stay done
	r.Setback(r.T.Setback, "needle")
	tt.frames(3)
	if r.Dev.Cur != cur || !r.Dev.Mods[cur-1].Done {
		t.Fatalf("a botched game undid the wires (module %d)", r.Dev.Cur)
	}
}

// Trying a plug in the wrong colour's socket tells you so; near its own, it pulls in.
func TestWrongSocketWarnsRightSocketPulls(t *testing.T) {
	tt := newTester(t, true)
	tt.toModule("wires", 0)
	m := tt.mod().(*modWires)
	p := m.plugs[0]
	F := tt.g.devFrame()
	tt.pose(1, p.pos.add(F.x.mul(0.2)), F.x.mul(-1), v3(0, 1, 0))
	tt.frames(1)
	tt.grabAt(1, p.pos)
	wrong := (m.order[0] + 1) % 3
	ws := m.socket(F, wrong).add(F.y.mul(0.03))
	tt.move(1, ws.add(F.y.mul(0.12)), 10)
	tt.move(1, ws, 6)
	if m.in[0] || m.wrongT <= 0 || m.wrongAt != wrong {
		t.Fatal("the wrong socket neither refused the plug nor said so")
	}
	rs := m.socket(F, m.order[0]).add(F.y.mul(0.03))
	tt.move(1, rs.add(F.y.mul(0.12)), 10)
	tt.move(1, rs.add(F.y.mul(0.07)), 6)
	if !m.in[0] {
		t.Fatal("7 cm over its own socket, the plug didn't pull in")
	}
}

func TestDeviceNeedle(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	for i := range r.Dev.Mods {
		if r.Dev.Mods[i].ID == "game" {
			r.Dev.Mods[i].Game = "needle"
		}
	}
	tt.toModule("game", 0)
	m := tt.mod().(*modGame)
	cur := r.Dev.Cur
	tt.frames(60)
	ax := m.kn.ax
	at := m.kn.c
	tt.pose(1, at.add(ax.mul(0.15)), ax.mul(-1), v3(0, 1, 0))
	tt.frames(1)
	tt.grabAt(1, at)
	wrist := 0.0
	worst := 0.0
	for k := 0; k < 60*60 && !r.Dev.Mods[cur].Done; k++ {
		if r.Dev.Cur != cur {
			t.Fatalf("the needle got away (setback) at %.1f s", float64(k)/60)
		}
		// a steady hand: turn against the needle, a little at a time
		want := math.Max(-0.06, math.Min(0.06, -m.needle/0.9*0.35))
		if math.Abs(wrist+want) > 1.7 {
			// out of wrist: let go, turn back, take a new grip
			tt.release(1)
			tt.rotate(1, ax, -wrist)
			tt.frames(1)
			tt.grabAt(1, at)
			wrist = 0
			continue
		}
		tt.rotate(1, ax, want)
		wrist += want
		tt.frames(1)
		worst = math.Max(worst, math.Abs(m.needle))
	}
	if !r.Dev.Mods[cur].Done {
		t.Fatalf("needle game not finished (progress %.2f, worst needle %.2f)", m.prog, worst)
	}
}

func TestDeviceDials(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	tt.toModule("dials", 0)
	m := tt.mod().(*modDials)
	cur := r.Dev.Cur
	F := tt.g.devFrame()
	for i, k := range m.kn {
		at := k.c
		tt.pose(1, at.add(F.z.mul(0.15)), F.z.mul(-1), v3(0, 1, 0))
		tt.frames(1)
		for tries := 0; tries < 6 && math.Abs(k.ang-m.target[i]) > 0.05; tries++ {
			tt.ratchet(1, at, F.z, m.target[i]-k.ang)
		}
		if math.Abs(k.ang-m.target[i]) > 0.17 {
			t.Fatalf("dial %d at %.2f, target %.2f", i, k.ang, m.target[i])
		}
	}
	tt.frames(80)
	if !r.Dev.Mods[cur].Done {
		t.Fatalf("dials set but the module isn't done (good %.2f)", m.good)
	}
}

func TestDeviceCoilCrank(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	tt.toModule("crank", 0)
	m := tt.mod().(*modCrank)
	cur := r.Dev.Cur
	k := m.k
	tt.pose(1, k.handle().add(k.ax.mul(0.15)), k.ax.mul(-1), v3(0, 1, 0))
	tt.frames(1)
	tt.grabAt(1, k.handle())
	tt.circle(1, k.c, k.ax, k.ref, k.R, 16, 30, k.ang)
	tt.release(1)
	if !r.Dev.Mods[cur].Done {
		t.Fatalf("coil not charged (charge %.2f, turns %.1f)", m.charge, k.turns)
	}
	// and the leak: a coil half wound and let go drains
	m2 := &modCrank{}
	m2.start(tt.g)
	m2.charge = 0.5
	F := tt.g.devFrame()
	m2.frame(tt.g, F, 0)
	for s := 0; s < 120; s++ {
		m2.frame(tt.g, F, tdt)
	}
	if m2.charge > 0.5-0.07 {
		t.Fatalf("an unattended coil didn't leak (%.2f)", m2.charge)
	}
}

func TestDeviceSerialWithFlashlight(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	tt.toModule("serial", 0)
	m := tt.mod().(*modSerial)
	cur := r.Dev.Cur
	F := tt.g.devFrame()
	slot := m.slot(F)
	if tt.g.flashOn(slot) > 0 {
		t.Fatal("the slot is lit before anyone picked up the light")
	}
	// take the flashlight off its clip
	fl := tt.g.devVis.flash
	tt.pose(1, fl.pos.add(F.x.mul(0.2)), F.x.mul(-1), v3(0, 1, 0))
	tt.frames(1)
	tt.grabAt(1, fl.pos)
	// shine it into the slot from 35 cm out
	tt.move(1, slot.add(F.x.mul(0.35)), 15)
	ld := fl.Z.mul(-1)
	want := slot.sub(fl.pos).norm()
	ax := ld.cross(want)
	ang := math.Atan2(ax.length(), ld.dot(want))
	if ax.length() > 1e-6 {
		tt.rotate(1, ax.norm(), ang)
	}
	tt.frames(3)
	if lit := tt.g.flashOn(slot); lit < 0.5 {
		t.Fatalf("the flashlight doesn't light the slot (%.2f)", lit)
	}
	// key it in with the other hand, fast: a wrong code first, then a typo and a backspace
	keyIdx := func(ch byte) int {
		return strings.IndexByte(keyLabels, ch)
	}
	press := func(ch byte) {
		b := m.keys[keyIdx(ch)]
		tt.tap(0, b.c, b.n)
	}
	wrong := byte('0' + (m.code[0]-'0'+1)%10)
	for i := 0; i < len(m.code); i++ {
		press(wrong)
	}
	if r.Dev.Mods[cur].Done || m.err <= 0 {
		t.Fatal("a wrong code should show WRONG and not open anything")
	}
	press(wrong)
	press('<')
	if m.typed != "" || m.err > 0 {
		t.Fatalf("typing after a wrong code should start over (typed %q)", m.typed)
	}
	for i := 0; i < len(m.code); i++ {
		press(m.code[i])
	}
	if !r.Dev.Mods[cur].Done {
		t.Fatalf("right serial keyed in (%q), module not done", m.typed)
	}
}

func TestDeviceRipCord(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	tt.toModule("ripcord", 0)
	m := tt.mod().(*modRip)
	cur := r.Dev.Cur
	// a lazy pull doesn't count
	at := m.c.handle()
	tt.pose(1, at.add(v3(0, 0.15, 0)), v3(0, -1, 0), v3(1, 0, 0))
	tt.frames(1)
	tt.grabAt(1, at)
	tt.move(1, at.add(v3(0, 0.6, 0)), 60)
	tt.release(1)
	if m.good != 0 {
		t.Fatalf("a slow pull counted (%d)", m.good)
	}
	for k := 0; k < 3; k++ {
		tt.frames(30) // the cord winds back in
		at := m.c.handle()
		tt.grabAt(1, at)
		tt.move(1, at.add(v3(0, 0.62, 0)), 9)
		tt.release(1)
	}
	if !m.running || m.lit() {
		t.Fatalf("three hard pulls: the engine should be running and revving (running %v)", m.running)
	}
	pushRed(tt, m)
	if r.Phase == sim.Won {
		t.Fatal("pushed the button before it lit: it should do nothing")
	}
	for k := 0; k < 60*6 && !m.lit(); k++ {
		tt.frames(1)
	}
	pushRed(tt, m)
	if !r.Dev.Mods[cur].Done || r.Phase != sim.Won {
		t.Fatalf("the lit button should set it off (round %d)", r.Phase)
	}
}

// pushRed: a fingertip jab down through the big red button.
func pushRed(tt *tester, m *modRip) {
	b := m.btn
	tt.poseTip(0, b.top().add(b.up.mul(0.07)), b.up.mul(-1), v3(0, 0, 1))
	tt.frames(1)
	for k := 1; k <= 6; k++ {
		tt.poseTip(0, b.top().add(b.up.mul(0.07-0.013*float64(k))), b.up.mul(-1), v3(0, 0, 1))
		tt.frames(1)
	}
	tt.poseTip(0, b.top().add(b.up.mul(0.2)), b.up.mul(-1), v3(0, 0, 1))
	tt.frames(2)
}

// The big red button goes off for a fingertip that just touches the top of its cap (off
// centre, at an angle), and for a palm slapped down on it.
func TestBigRedButtonEasy(t *testing.T) {
	for _, how := range []string{"tip", "palm"} {
		tt := newTester(t, true)
		r := tt.g.r
		tt.toModule("ripcord", 0)
		m := tt.mod().(*modRip)
		for i := 0; i < 3 && !m.running; i++ {
			m.c.onPull(2)
			tt.frames(2)
		}
		for k := 0; k < 60*8 && !m.lit(); k++ {
			tt.frames(1)
		}
		b := m.btn
		side := projPerp(v3(1, 0, 0.3), b.up).norm()
		if how == "tip" {
			at := b.top().add(side.mul(0.03))
			dir := b.up.mul(-1).add(side.mul(0.5)).norm()
			tt.poseTip(0, at.sub(dir.mul(0.1)), dir, v3(0, 0, 1))
			tt.frames(1)
			tt.poseTip(0, at.add(b.up.mul(0.004)), dir, v3(0, 0, 1))
			tt.frames(2)
		} else {
			at := b.top().add(side.mul(0.02)).add(b.up.mul(0.025))
			tt.pose(1, at.add(b.up.mul(0.2)), v3(0, 0, -1), b.up)
			tt.frames(1)
			tt.move(1, at.sub(b.up.mul(0.005)), 4)
			tt.frames(2)
		}
		if r.Phase != sim.Won {
			t.Fatalf("%s: the lit button should have gone off", how)
		}
	}
}

// The cord pulls out any way but down: three hard pulls off at an angle start it too, and a
// pull downward only gets it to level.
func TestRipCordAnyAngle(t *testing.T) {
	tt := newTester(t, true)
	tt.toModule("ripcord", 0)
	m := tt.mod().(*modRip)
	ways := []vec3{v3(0.7, 0.7, 0), v3(-0.5, 0.45, 0.6).norm(), v3(0, 0.35, -0.95).norm()}
	for _, w := range ways {
		tt.frames(30)
		at := m.c.handle()
		tt.pose(1, at.add(v3(0, 0.12, 0)), v3(0, -1, 0), v3(1, 0, 0))
		tt.frames(1)
		tt.grabAt(1, at)
		tt.move(1, at.add(w.mul(0.62)), 9)
		tt.release(1)
	}
	if !m.running {
		t.Fatalf("three hard pulls at an angle should start it (good %d)", m.good)
	}
	// down: it stops just above level
	tt2 := newTester(t, true)
	tt2.toModule("ripcord", 0)
	m2 := tt2.mod().(*modRip)
	at := m2.c.handle()
	tt2.pose(1, at.add(v3(0, 0.12, 0)), v3(0, -1, 0), v3(1, 0, 0))
	tt2.frames(1)
	tt2.grabAt(1, at)
	tt2.move(1, at.add(v3(0.3, -0.4, 0)), 9)
	if h := m2.c.handle(); h.y < m2.c.a.y {
		t.Fatalf("the cord went down into the cart (handle %.3f below its hole %.3f)", h.y, m2.c.a.y)
	}
	tt2.release(1)
}

func TestCartDrag(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	h := tt.g.devVis.handles[0]
	p0 := r.Dev.Pos
	at := h.gPos()
	tt.grabAt(1, at)
	// it's heavy: a steady walking pull drags it along
	tt.move(1, at.add(v3(0, 0, 0.5)), 150)
	tt.frames(30)
	moved := sim.Dist(p0, r.Dev.Pos)
	if moved < 0.4 {
		t.Fatalf("the cart only moved %.2f m", moved)
	}
	// try to shove it out through your real wall
	tt.move(1, hands[1].palm.add(v3(0, 0, 3)), 600)
	tt.release(1)
	if !sim.InPoly(r.Room, r.Dev.Pos) {
		t.Fatalf("the cart went through the wall to %.2f,%.2f", r.Dev.Pos.X, r.Dev.Pos.Z)
	}
}

func TestTugOfWar(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	r.T.TugWalker = 1 // the mechanics, not the odds: a good yank always works here
	b := r.Spawn(sim.Walker, 0, "")
	d := &r.Dev
	b.Pos = d.Pos.Add(sim.V(0.9, 0))
	r.Steal(b)
	tt.frames(1)
	if d.State != sim.DevStolen {
		t.Fatalf("steal didn't take (state %d)", d.State)
	}
	// grab the handle farther from the warden and yank it back, again until it gives
	var hd *cartHandle
	best := -1.0
	for _, c := range tt.g.devVis.handles {
		if dd := sim.Dist(sim.V(c.gPos().x, c.gPos().z), b.Pos); dd > best {
			best, hd = dd, c
		}
	}
	got := false
	for k := 0; k < 8 && !got; k++ {
		tt.grabMoving(1, hd.gPos)
		at := hands[1].palm
		away := sim.V(at.x, at.z).Sub(b.Pos).Norm()
		tt.move(1, at.add(v3(away.X, 0, away.Z).mul(0.35)), 10)
		tt.release(1)
		tt.frames(30)
		got = d.State == sim.DevHome
	}
	if !got {
		t.Fatalf("eight hard yanks and the warden still has it (state %d)", d.State)
	}
}

// ---- the harpoon ----

func armTurret(tt *tester) *sim.Harpoon {
	r := tt.g.r
	for r.Work() < 0.25*r.WorkTotal() {
		r.CompleteModule()
		tt.frames(1)
	}
	h := &r.Harp
	if !h.Up || !h.Armed {
		tt.t.Fatal("a quarter of the device done and no harpoon")
	}
	tt.frames(120) // it rises
	return h
}

// holdTurret: both hands on the grips, then swing it to aim along dir. The gun turns as a
// rigid body, so the hands' midpoint has to go round the pivot the opposite way.
func holdTurret(tt *tester, dir vec3) {
	g := tt.g
	for i, side := range []float64{-1, 1} {
		F, _ := g.harpoonFrame()
		gp := F.p(side*0.17, -0.02, -harpGripBack)
		tt.pose(i, gp.sub(F.z.mul(0.15)), F.z, v3(0, 1, 0))
		tt.frames(1)
		tt.move(i, gp, 4)
		tt.grip(i, true)
		tt.frames(2)
		if hands[i].held != g.grips[i] {
			tt.t.Fatalf("hand %d didn't catch its grip", i)
		}
	}
	F, _ := g.harpoonFrame()
	mid := hands[0].palm.add(hands[1].palm).mul(0.5)
	v0 := mid.sub(F.o)
	y0, e0 := yawEl(v0)
	ay, ae := yawEl(g.harpAim)
	dy, de := yawEl(dir)
	y1, e1 := y0+angDiff2(ay, dy), e0-(de-ae)
	v1 := v3(math.Cos(y1)*math.Cos(e1), math.Sin(e1), math.Sin(y1)*math.Cos(e1)).mul(v0.length())
	delta := v1.sub(v0)
	p0 := [2]vec3{hands[0].palm, hands[1].palm}
	for k := 1; k <= 40; k++ {
		for i := 0; i < 2; i++ {
			tt.placePalm(i, p0[i].add(delta.mul(float64(k)/40)))
		}
		tt.frames(1)
	}
	tt.frames(20)
	if a := g.harpAim.dot(dir.norm()); a < 0.995 {
		tt.t.Fatalf("the gun didn't come round to the target (cos %.3f)", a)
	}
}

func TestHarpoonHookAndReel(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	h := armTurret(tt)
	_, muzzle := tt.g.harpoonFrame()
	// a warden standing in the hall a few metres out from the turret
	out := h.Pos.Sub(sim.Centroid(r.Room)).Norm()
	b := r.Spawn(sim.Walker, 0, "")
	b.Pos = h.Pos.Add(out.Mul(3.2))
	b.Path, b.State, b.Wait = nil, sim.RWait, 100
	tt.frames(1)
	target := fv(b.Pos, 1.0)
	holdTurret(tt, target.sub(muzzle).norm())
	tt.bits(1, true, false)
	tt.frames(1)
	tt.bits(1, false, false)
	tt.hs[1].trig = 0
	for k := 0; k < 60 && h.Kind == sim.ShotFlying; k++ {
		tt.frames(1)
	}
	if h.Kind != sim.ShotRobot || h.Target != b.ID {
		t.Fatalf("the spear didn't stick in the warden (kind %d, target %d)", h.Kind, h.Target)
	}
	// let go of the turret and haul the line in hand over hand
	tt.release(0)
	tt.release(1)
	d0 := sim.Dist(b.Pos, h.Pos)
	_, muzzle = tt.g.harpoonFrame()
	for pull := 0; pull < 8 && b.Alive(); pull++ {
		i := pull % 2
		along := v3s(h.Tip).sub(muzzle).norm()
		at := muzzle.add(along.mul(0.35))
		tt.pose(i, at.add(v3(0, 0.1, 0)), along, v3(0, 1, 0))
		tt.frames(1)
		tt.grabAt(i, at)
		tt.move(i, at.sub(along.mul(0.45)), 8)
		tt.release(i)
	}
	d1 := sim.Dist(b.Pos, h.Pos)
	if b.Alive() && d1 > d0-1.0 && !b.Pinned {
		t.Fatalf("reeling didn't bring it in (%.2f -> %.2f m)", d0, d1)
	}
}

// shootTurret: aim the held turret at p and pull the trigger; wait for the spear to land.
func shootTurret(tt *tester, p vec3) {
	h := &tt.g.r.Harp
	_, muzzle := tt.g.harpoonFrame()
	holdTurret(tt, p.sub(muzzle).norm())
	tt.bits(1, true, false)
	tt.frames(1)
	tt.bits(1, false, false)
	tt.hs[1].trig = 0
	for k := 0; k < 90 && h.Kind == sim.ShotFlying; k++ {
		tt.frames(1)
	}
	tt.release(0)
	tt.release(1)
	tt.frames(2)
}

// Reel mode: hooking a warden with a full (green) spear loads a reel-only (yellow) one - it
// hooks another, but the floor gets no live wire out of it unless it's supercharged. Some
// fresh loads come yellow from the start, a pair of them.
func TestHarpoonReelMode(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	h := armTurret(tt)
	if h.Reels != 0 {
		t.Fatal("the first load should be green here")
	}
	out := h.Pos.Sub(sim.Centroid(r.Room)).Norm()
	side := sim.V(-out.Z, out.X)
	warden := func(off float64) *sim.Robot {
		b := r.Spawn(sim.Walker, 0, "")
		b.Pos = h.Pos.Add(out.Mul(3.2)).Add(side.Mul(off))
		b.Path, b.State, b.Wait = nil, sim.RWait, 100
		tt.frames(1)
		return b
	}
	b1 := warden(-0.8)
	shootTurret(tt, fv(b1.Pos, 1.0))
	if h.Kind != sim.ShotRobot || h.Target != b1.ID {
		t.Fatalf("green spear didn't hook the warden (kind %d)", h.Kind)
	}
	if !h.Armed || h.Reels != 1 {
		t.Fatalf("hooking one should load one reel spear (armed %v reels %d)", h.Armed, h.Reels)
	}
	// the yellow one into the floor: no wire
	shootTurret(tt, fv(h.Pos.Add(out.Mul(4)), 0))
	if h.Kind == sim.ShotAnchor || h.Armed {
		t.Fatalf("a reel spear made a live wire or left the gun loaded (kind %d armed %v)", h.Kind, h.Armed)
	}
	// a supercharged reel spear is a full one again
	h.Armed, h.Reels = true, 1
	r.SuperCD = 0
	if !r.SuperHarpoon() || h.Reels != 0 {
		t.Fatal("supercharging didn't make it a full spear")
	}
	h.Super, h.Armed = false, false
	// a yellow load from the get-go: two reel-ins
	r.T.ReelLoad = 1
	r.CompleteModule()
	for r.Work() < 0.5*r.WorkTotal() {
		r.CompleteModule()
	}
	tt.frames(2)
	if !h.Armed || h.Reels != 2 {
		t.Fatalf("a yellow load should be a pair of reel spears (armed %v reels %d)", h.Armed, h.Reels)
	}
	b2 := warden(0.8)
	shootTurret(tt, fv(b2.Pos, 1.0))
	if h.Kind != sim.ShotRobot || h.Target != b2.ID || !h.Armed || h.Reels != 1 {
		t.Fatalf("first yellow: kind %d target %d armed %v reels %d", h.Kind, h.Target, h.Armed, h.Reels)
	}
	b3 := warden(0)
	shootTurret(tt, fv(b3.Pos, 1.0))
	if h.Kind != sim.ShotRobot || h.Target != b3.ID || h.Armed {
		t.Fatalf("second yellow: kind %d target %d armed %v", h.Kind, h.Target, h.Armed)
	}
}

func TestHarpoonLiveWire(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	h := armTurret(tt)
	_, muzzle := tt.g.harpoonFrame()
	out := h.Pos.Sub(sim.Centroid(r.Room)).Norm()
	// shoot the floor out in the hall: the line becomes a live wire
	floor := fv(h.Pos.Add(out.Mul(4)), 0)
	holdTurret(tt, floor.sub(muzzle).norm())
	tt.bits(1, true, false)
	tt.frames(1)
	tt.bits(1, false, false)
	for k := 0; k < 120 && h.Kind == sim.ShotFlying; k++ {
		tt.frames(1)
	}
	if h.Kind != sim.ShotAnchor {
		t.Fatalf("spear in the floor should anchor (kind %d)", h.Kind)
	}
	tt.release(0)
	tt.release(1)
	// a warden walking across the line fries
	_, muzzle = tt.g.harpoonFrame()
	mid := sim.Lerp(sim.V(muzzle.x, muzzle.z), h.Tip.XZ(), 0.6)
	perp := sim.V(-out.Z, out.X)
	b := r.Spawn(sim.Walker, 0, "")
	b.Pos = mid.Add(perp.Mul(1.0))
	b.Path, b.State = []sim.V2{mid.Sub(perp.Mul(1.0))}, sim.RWalk
	b.SpanA, b.SpanB = r.Spans()[0][0], r.Spans()[0][1]
	for k := 0; k < 60*5 && b.Alive(); k++ {
		tt.frames(1)
	}
	if b.Alive() {
		t.Fatalf("a warden walked through the live wire (at %.2f,%.2f)", b.Pos.X, b.Pos.Z)
	}
}

// ---- the radio ----

func TestRadioToTheEar(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	rd := tt.g.radio
	if r.Heard {
		t.Fatal("heard before listening")
	}
	at := rd.set.pos
	tt.pose(1, at.add(v3(0, 0.15, 0)), v3(0, -1, 0), v3(1, 0, 0))
	tt.frames(1)
	tt.grabAt(1, at)
	// to your ear
	head := ed.hmd.pos()
	ear := head.add(ed.hmd.axis(0).mul(0.09))
	tt.move(1, ear, 20)
	tt.frames(10)
	if !rd.listening || !r.Heard {
		t.Fatalf("handset at the ear but not listening (dist %.2f)", rd.set.pos.sub(ear).length())
	}
	tt.move(1, ear.add(v3(0.3, -0.4, -0.2)), 10)
	tt.release(1)
	tt.frames(60)
	if rd.listening {
		t.Fatal("still listening with the handset dropped")
	}
}

// ---- supercharge ----

func TestSuperchargeFriesThePresser(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	b := r.Spawn(sim.Walker, 0, "")
	for k := 0; k < 60*60 && b.State != sim.RPress; k++ {
		tt.frames(1)
	}
	if b.State != sim.RPress {
		t.Fatalf("the walker never reached the ring (state %d)", b.State)
	}
	// from the middle of the room, point at the span it leans on and hit A
	A, B := r.Gens[b.SpanA].Pos, r.Gens[b.SpanB].Pos
	q := sim.Lerp(A, B, b.SpanT)
	c := sim.Centroid(r.Room)
	o := fv(c, 1.3)
	tgt := fv(q, 0.9)
	d := tgt.sub(o).norm()
	tt.pose(1, o.sub(d.mul(0.05)), d, v3(0, 1, 0))
	tt.hs[1].aTouch = true // thumb on A: the beam shows which span it'll hit
	tt.frames(2)
	wh := hands[1].wallHit
	if !wh.ok || wh.kind != 1 {
		t.Fatalf("pointing at the span doesn't target it (%+v)", wh)
	}
	// a quick press isn't enough: let go early and nothing's spent
	tt.bits(1, false, true)
	tt.frames(30)
	tt.bits(1, false, false)
	tt.frames(2)
	if r.SuperCD > 0 || !b.Alive() {
		t.Fatal("letting go of A before the charge filled still supercharged")
	}
	// hold it till the ring fills
	tt.bits(1, false, true)
	tt.frames(int(superHold*60) + 4)
	tt.bits(1, false, false)
	tt.hs[1].trig = 0
	tt.frames(30)
	if b.Alive() {
		t.Fatalf("supercharged span and the presser lives (state %d hp %.1f)", b.State, b.HP)
	}
	if r.SuperCD <= 0 {
		t.Fatal("no cooldown after a supercharge")
	}
}

// ---- the radio voice ----

func TestVoiceLinesExist(t *testing.T) {
	var ids []string
	for _, k := range []string{"walker", "heavy", "leech"} {
		for n := 1; n <= 12; n++ {
			ids = append(ids, "c_"+k+"_"+itoa(n))
		}
		ids = append(ids, "c_"+k+"_many")
	}
	for _, d := range []string{"north", "east", "south", "west"} {
		ids = append(ids, "d_"+d)
	}
	for w := 1; w <= 15; w++ {
		ids = append(ids, "w_"+itoa(w))
	}
	ids = append(ids, "w_next", "out_1", "out_2", "out_3", "x_start", "x_genblown", "x_leech", "x_stolen",
		"x_exposed", "x_armed", "x_recovered", "x_pickup", "x_won", "x_lost", "x_livewire",
		"h_need", "h_floor_north", "h_floor_east", "h_floor_south", "h_floor_west", "h_gen", "h_radio", "h_console",
		"x_genlow", "x_ringdown", "n_pickup_1", "n_pickup_2", "n_urgent",
		"h_wneed", "x_harp_super", "x_harp_tie", "x_tractor", "x_cart_spear")
	for _, id := range ids {
		if c := voiceClip(id); len(c) < 4000 {
			t.Errorf("voice line %q missing or too short (%d samples)", id, len(c))
		}
	}
}

// Warnings wait on the handset: the speaker only plays its recorded prompt to pick up, and
// at your ear the waiting lines play (stale ones are dropped).
func TestRadioSpeakerOnlyPromptsYouToPickUp(t *testing.T) {
	tt := newTester(t, true)
	g := tt.g
	rd := g.radio
	r := g.r
	// a leech on one generator, the others running low
	b := waitForLeech(tt, 1)
	for i := range r.Gens {
		if i != b.Gen {
			r.Gens[i].Charge = 0.1 * r.T.ChargeMax
		}
	}
	r.Heard = true // the intel's been heard: only the warnings are waiting
	rd.inbox, rd.nagLeft, rd.nagAt = nil, 0, 0
	eng.mu.Lock()
	eng.clips, eng.cpend = nil, nil
	eng.cend = [len(eng.cend)]float64{}
	eng.mu.Unlock()
	radioSay("x_leech", false)
	radioSay("x_genlow", false)
	radioSay("x_genlow", false) // twice: still one message
	if len(rd.inbox) != 2 {
		t.Fatalf("two warnings should be waiting on the handset, %d are", len(rd.inbox))
	}
	tt.frames(40)
	eng.mu.Lock()
	var speaker, handset int
	for _, c := range append(append([]clipPlay(nil), eng.cpend...), eng.clips...) {
		switch {
		case c.src == srcRadio && len(c.data) == len(voiceClip("n_urgent")):
			speaker++
		case c.src == srcRadio:
			t.Errorf("the speaker played something other than its prompt (%d samples)", len(c.data))
		case c.src == srcHandset:
			handset++
		}
	}
	eng.mu.Unlock()
	if speaker != 1 {
		t.Fatalf("the speaker should prompt once (urgent) for the leech, played %d", speaker)
	}
	if handset != 0 {
		t.Fatal("the contact talked before anyone picked up")
	}
	// stale ones drop out: a minute-old message, and the leech once it's gone
	rd.inbox = append(rd.inbox, radioMsg{id: "x_recovered", at: g.t - 60, ttl: 30})
	r.Pluck(b.ID)
	at := rd.set.pos
	tt.pose(1, at.add(v3(0, 0.15, 0)), v3(0, -1, 0), v3(1, 0, 0))
	tt.frames(1)
	tt.grabAt(1, at)
	ear := ed.hmd.pos().add(ed.hmd.axis(0).mul(0.09))
	tt.move(1, ear, 20)
	tt.frames(2)
	if len(rd.inbox) != 0 {
		t.Fatal("picking up should play what was waiting")
	}
	eng.mu.Lock()
	var got []int
	for _, c := range append(append([]clipPlay(nil), eng.clips...), eng.cpend...) {
		if c.src == srcHandset {
			got = append(got, len(c.data))
		}
	}
	eng.mu.Unlock()
	if len(got) < 1 || got[0] != len(voiceClip("x_genlow")) {
		t.Fatalf("at the ear the warning that's still true should play first, got %v", got)
	}
	for _, n := range got {
		if n == len(voiceClip("x_recovered")) || n == len(voiceClip("x_leech")) {
			t.Fatal("a message that's no longer true played")
		}
	}
	// while you're listening a new one comes straight through
	radioSay("x_armed", false)
	if len(rd.inbox) != 0 {
		t.Fatal("with the handset at your ear a new line should play, not wait")
	}
}

// Drop the handset and look away: it's back on its cradle.
func TestHandsetGoesBackWhenYouLookAway(t *testing.T) {
	tt := newTester(t, true)
	rd := tt.g.radio
	s := rd.set
	at := s.pos
	tt.pose(1, at.add(v3(0, 0.15, 0)), v3(0, -1, 0), v3(1, 0, 0))
	tt.frames(1)
	tt.grabAt(1, at)
	// carry it off and drop it on the floor in front of you
	tt.setHead(v3(0, 1.6, 0), s.pos.sub(v3(0, 1.6, 0)))
	tt.move(1, s.pos.add(v3(0, -0.4, 0)), 10)
	tt.release(1)
	tt.frames(90)
	if s.docked {
		t.Fatal("it went back while you were looking at it")
	}
	// turn away
	away := v3(0, 1.6, 0).sub(s.pos)
	away.y = 0
	tt.setHead(v3(0, 1.6, 0), away.norm())
	tt.frames(60)
	if !s.docked {
		t.Fatal("the handset should be back on the radio once you look away")
	}
}

// A screwdriver dropped onto the cart lands on its lid (and rides along when it moves).
func TestDroppedOnTheCartStaysOnIt(t *testing.T) {
	tt := newTester(t, true)
	k := tt.g.driver
	F := tt.g.devFrame()
	p := k.p
	tt.pose(1, p.pos.add(v3(0, 0.15, 0)), v3(0, -1, 0), v3(1, 0, 0))
	tt.frames(1)
	tt.grabAt(1, p.pos)
	if !p.held {
		t.Fatal("couldn't pick up the screwdriver")
	}
	tt.move(1, F.p(0.05, devTop+0.3, -0.1), 30)
	tt.frames(15) // hold it still over the lid, then let go
	tt.release(1)
	tt.frames(120)
	if !p.resting || !p.onCart {
		t.Fatalf("it should be lying on the cart (y %.2f, lid %.2f)", p.pos.y, F.o.y+devTop)
	}
	if p.pos.y < F.o.y+devTop {
		t.Fatalf("it went through the lid (y %.2f)", p.pos.y)
	}
	before := p.pos.sub(F.o)
	tt.g.r.Dev.Pos = tt.g.r.Dev.Pos.Add(sim.V(0.3, 0))
	tt.frames(2)
	F = tt.g.devFrame()
	if d := p.pos.sub(F.o).sub(before).length(); d > 0.01 {
		t.Fatalf("it didn't ride along with the cart (%.3f off)", d)
	}
}

func TestIntelPlaysInTheHandset(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	items := intelLines(r)
	want := 2 + 2*len(r.Intel.Groups)
	if len(items) != want {
		t.Fatalf("intel for %d groups has %d lines, want %d", len(r.Intel.Groups), len(items), want)
	}
	rd := tt.g.radio
	at := rd.set.pos
	tt.pose(1, at.add(v3(0, 0.15, 0)), v3(0, -1, 0), v3(1, 0, 0))
	tt.frames(1)
	tt.grabAt(1, at)
	ear := ed.hmd.pos().add(ed.hmd.axis(0).mul(0.09))
	tt.move(1, ear, 20)
	tt.frames(5)
	buf := make([]float64, 2*4800)
	for k := 0; k < 5; k++ {
		eng.render(buf)
	}
	eng.mu.Lock()
	n, amp := len(eng.clips), eng.ccur[srcHandset].amp
	var pos float64
	if n > 0 {
		pos = eng.clips[0].pos
	}
	eng.mu.Unlock()
	if n == 0 || pos <= 0 {
		t.Fatalf("no intel playing (%d clips, pos %.0f)", n, pos)
	}
	if amp < 0.9 {
		t.Fatalf("handset at the ear but the voice is at %.2f", amp)
	}
}

// ---- round flow ----

// laserPress: point hand 1 at a panel button and click the trigger.
func (tt *tester) laserPress(p *uiPanel, i int) {
	b := p.btns[i]
	at := p.pt(b.x, b.y, 0)
	o := at.add(p.n.mul(0.8)).add(v3(0.1, 0.1, 0))
	d := at.sub(o).norm()
	tt.pose(1, o, d, v3(0, 1, 0))
	tt.hs[1].trig = 0.5
	tt.frames(1)
	tt.bits(1, true, false)
	tt.frames(1)
	tt.bits(1, false, false)
	tt.hs[1].trig = 0
	tt.frames(2)
}

func TestRoundFlowWinAndAgain(t *testing.T) {
	tt := newTester(t, false)
	r := tt.g.r
	// a maze to carry over
	if _, why := r.PlaceWall(sim.V(-1.4, -2.4), sim.V(1.4, -2.4)); why != "" {
		t.Fatal(why)
	}
	tt.g.startRound()
	r.NextWave = 1e9
	for r.Phase == sim.Playing {
		r.CompleteModule()
		tt.frames(1)
	}
	if r.Phase != sim.Won {
		t.Fatalf("phase %d after the last module", r.Phase)
	}
	tt.frames(30)
	res := tt.g.results
	if !res.visible() || res.pos == (vec3{}) {
		t.Fatal("no results board after the win")
	}
	old := tt.g
	tt.laserPress(res, 0) // AGAIN
	if G == old || G.r.Phase != sim.Setup {
		t.Fatal("AGAIN didn't start a new round")
	}
	if len(G.r.Maze) != 0 {
		t.Fatalf("the maze shouldn't carry over: %d walls", len(G.r.Maze))
	}
	if envLampK != 1 {
		tt.frames(1)
		if envLampK != 1 {
			t.Fatalf("lights still out in setup (%.2f)", envLampK)
		}
	}
}

func TestRoundFlowLoseAndNewMaze(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	r.PlaceWall(sim.V(-1.4, -2.4), sim.V(1.4, -2.4))
	b := r.Spawn(sim.Walker, 0, "")
	b.Pos = r.Dev.Pos.Add(sim.V(0.6, 0))
	r.Steal(b)
	for k := 0; k < 60*120 && r.Phase == sim.Playing; k++ {
		tt.frames(1)
	}
	if r.Phase != sim.Lost {
		t.Fatalf("the warden never got out with it (phase %d, device %d)", r.Phase, r.Dev.State)
	}
	tt.frames(10)
	tt.laserPress(tt.g.results, 0) // AGAIN
	if G.r.Phase != sim.Setup || len(G.r.Maze) != 0 {
		t.Fatalf("new maze: phase %d, %d walls", G.r.Phase, len(G.r.Maze))
	}
}

func TestPauseMenu(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	t0 := r.Time
	tt.hs[0].pressed = 1 << bitMenu
	tt.frames(1)
	tt.hs[0].pressed = 0
	tt.frames(1) // it opens as you let go of B
	if !tt.g.menuOpen {
		t.Fatal("B didn't open the menu")
	}
	tt.frames(60)
	if r.Time > t0+3*tdt+1e-9 {
		t.Fatalf("the round ran on behind the pause menu (%.2f -> %.2f)", t0, r.Time)
	}
	tt.laserPress(tt.g.menu, 0) // RESUME
	if tt.g.menuOpen {
		t.Fatal("RESUME didn't close the menu")
	}
	tt.frames(30)
	if r.Time < t0+0.4 {
		t.Fatal("the round didn't resume")
	}
}

func TestDeviceFieldHoldsThenFails(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	d := &r.Dev
	// shut two neighbouring generators: the ring shrinks off the device
	r.SetPower(0, false)
	r.SetPower(1, false)
	tt.frames(2)
	if !d.Exposed {
		t.Skip("this layout still covers the device")
	}
	b := r.Spawn(sim.Walker, 0, "")
	b.Pos = d.Pos.Add(sim.V(0.5, 0))
	b.State = sim.RGrab
	d.Grabber = b.ID
	tt.frames(60)
	if d.State != sim.DevHome {
		t.Fatalf("taken while the field was up (field %.1f)", d.Field)
	}
	tt.frames(int(r.T.FieldSecs*60) + 30)
	if d.State == sim.DevHome {
		t.Fatalf("the field never failed (field %.1f, exposed %v)", d.Field, d.Exposed)
	}
}

// Dragging the cart takes every part of the device with it, whichever stage you're on.
func TestCartCarriesEveryPart(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	tt.toModule("screws", 0)
	dv := &tt.g.devVis
	h := dv.handles[0]
	at := h.gPos()
	tt.grabAt(1, at)
	tt.move(1, at.add(v3(0.6, 0, 0.4)), 30)
	tt.release(1)
	tt.frames(2)
	F := tt.g.devFrame()
	local := func(p vec3) vec3 {
		d := p.sub(F.o)
		return v3(d.dot(F.x), d.dot(F.y), d.dot(F.z))
	}
	inCart := func(name string, p vec3) {
		q := local(p)
		if math.Abs(q.x) > devHW+0.2 || math.Abs(q.z) > devHL+0.25 || q.y < 0 || q.y > devTop+0.8 {
			t.Errorf("%s left behind: %.2f,%.2f,%.2f in cart space", name, q.x, q.y, q.z)
		}
	}
	for i, m := range dv.mods {
		switch mm := m.(type) {
		case *modCrank:
			inCart("coil crank", mm.k.c)
		case *modRip:
			inCart("rip cord", mm.c.a)
		case *modWires:
			for k, p := range mm.plugs {
				inCart("wire plug "+itoa(k), p.pos)
			}
		}
		_ = i
	}
	inCart("flashlight", dv.flash.pos)
	_ = r
}

// ---- fitting the gloves ----

func TestFitGloves(t *testing.T) {
	tt := newTester(t, true)
	g := tt.g
	// B opens the menu, FIT GLOVES by laser
	tt.hs[0].pressed = 1 << bitMenu
	tt.frames(1)
	tt.hs[0].pressed = 0
	tt.frames(1)
	tt.laserPress(g.menu, 2)
	if !g.fitting {
		t.Fatal("FIT GLOVES didn't start fitting")
	}
	t0 := g.r.Time
	// hands out in front, the left one reaches over and takes the right glove
	tt.pose(1, v3(0.15, 1.2, 0.1), v3(0, 0, -1), v3(0, 1, 0))
	tt.pose(0, v3(-0.15, 1.2, 0.1), v3(0, 0, -1), v3(0, 1, 0))
	tt.frames(2)
	aim0 := tt.hs[1].aim
	palm0 := hands[1].palm
	tt.move(0, palm0.add(v3(0, 0.02, 0)), 8)
	tt.grip(0, true)
	tt.frames(2)
	if hands[0].fitDrag != 1 {
		t.Fatal("the left hand didn't take hold of the right glove")
	}
	// slide it 4 cm forward and roll it 25 degrees, then let go (moving the controller
	// itself: the left glove's own fit changes under it as the mirror)
	laser0 := hands[1].aim.pos().sub(palm0)
	for k := 0; k < 10; k++ {
		tt.shift(0, v3(0, 0, -0.004))
		tt.frames(1)
	}
	tt.rotate(0, v3(0, 0, -1), 25*math.Pi/180)
	tt.frames(3)
	tt.release(0)
	moved := hands[1].palm.sub(palm0)
	if math.Abs(moved.z+0.04) > 0.015 {
		t.Fatalf("the glove should sit 4 cm further forward, moved %.3f,%.3f,%.3f", moved.x, moved.y, moved.z)
	}
	if tt.hs[1].aim != aim0 {
		t.Fatal("the controller's own aim changed")
	}
	if d := hands[1].aim.pos().sub(hands[1].palm).sub(rotAbout(laser0, v3(0, 0, -1), 25*math.Pi/180)).length(); d > 0.01 {
		t.Fatalf("the laser didn't move with the glove (%.3f off)", d)
	}
	if gloveFit[0] != mirrorFit(gloveFit[1]) {
		t.Fatal("the left glove isn't the mirror of the right one")
	}
	// it stays fitted with the hand: move the right controller, the glove keeps its offset
	off := hands[1].palm.sub(tt.hs[1].m.pos())
	tt.move(1, hands[1].palm.add(v3(0.1, 0, 0)), 5)
	if d := hands[1].palm.sub(tt.hs[1].m.pos()).sub(off).length(); d > 0.003 {
		t.Fatalf("the fit didn't stay with the controller (%.3f)", d)
	}
	if g.r.Time != t0 {
		t.Fatal("the round ran while fitting")
	}
	tt.laserPress(g.fitPanel, 0) // DONE
	if g.fitting {
		t.Fatal("DONE didn't finish fitting")
	}
	tt.frames(10)
	if g.r.Time <= t0 {
		t.Fatal("the round didn't carry on after fitting")
	}
}

// In fit mode each laser has a ball on it: take it with the other hand and the laser
// swings to point at your grabbing hand; the other laser mirrors it.
func TestFitReaimsTheLaser(t *testing.T) {
	tt := newTester(t, true)
	g := tt.g
	g.startFit()
	tt.pose(1, v3(0.15, 1.2, 0.1), v3(0, 0, -1), v3(0, 1, 0))
	tt.pose(0, v3(-0.25, 1.1, 0.3), v3(0, 0, -1), v3(0, 1, 0))
	tt.frames(2)
	raw := tt.hs[1].aim
	d0 := hands[1].aim.axis(2).mul(-1)
	o := hands[1].aim.pos()
	tt.move(0, hands[1].aimBall(), 10)
	tt.grip(0, true)
	tt.frames(2)
	if hands[0].fitAim != 1 {
		t.Fatal("the left hand didn't take the ball on the right laser")
	}
	if hands[0].fitDrag >= 0 {
		t.Fatal("taking the laser's ball grabbed the glove instead")
	}
	// lift it: the laser should tip up toward the hand, about 25 degrees
	up := o.add(d0.mul(aimOrb)).add(v3(0, aimOrb*math.Tan(25*math.Pi/180), 0))
	tt.move(0, up, 10)
	tt.frames(2)
	tt.release(0)
	tt.frames(2)
	d1 := hands[1].aim.axis(2).mul(-1)
	if ang := math.Acos(math.Min(1, d0.dot(d1))) * 180 / math.Pi; math.Abs(ang-25) > 4 {
		t.Fatalf("the laser should have tipped about 25 degrees, turned %.1f", ang)
	}
	if d1.y <= d0.y {
		t.Fatal("the laser should have tipped up toward the hand")
	}
	if hands[1].aim.pos().sub(o).length() > 0.005 {
		t.Fatal("re-aiming moved where the laser leaves the glove")
	}
	if tt.hs[1].aim != raw {
		t.Fatal("the controller's own aim changed")
	}
	if aimFit[0] != mirrorFit(aimFit[1]) {
		t.Fatal("the left laser isn't the mirror of the right one")
	}
	// grabbing the glove itself still moves the glove, not the laser
	tt.move(0, hands[1].palm.add(v3(0, 0.02, 0)), 10)
	tt.grip(0, true)
	tt.frames(2)
	if hands[0].fitDrag != 1 || hands[0].fitAim >= 0 {
		t.Fatal("next to the glove, the grab should take the glove")
	}
	tt.release(0)
	g.endFit()
	// and the round's laser points the new way
	tt.hs[1].trig = 0.5
	tt.frames(2)
	if !hands[1].beam || hands[1].aim.axis(2).mul(-1).sub(d1).length() > 0.01 {
		t.Fatal("after fitting, the laser doesn't point the re-aimed way")
	}
	tt.hs[1].trig = 0
}

func TestFitSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	gloveDir = dir
	defer func() { gloveDir = "" }()
	gloveFit[1] = mul34(ident34(), hmdMatrix34{{1, 0, 0, 0.01}, {0, 1, 0, -0.02}, {0, 0, 1, 0.03}})
	aimFit[1] = turnTo(v3(0, 0, -1), v3(0, 0.3, -1).norm())
	want := aimFit[1]
	saveGloves()
	gloveFit = [2]hmdMatrix34{ident34(), ident34()}
	aimFit = [2]hmdMatrix34{ident34(), ident34()}
	loadGloves()
	if gloveFit[1][2][3] < 0.029 || gloveFit[1][1][3] > -0.019 {
		t.Fatalf("the saved fit didn't come back: %v", gloveFit[1])
	}
	if math.Abs(float64(aimFit[1][1][2]-want[1][2])) > 1e-4 {
		t.Fatalf("the saved laser aim didn't come back: %v", aimFit[1])
	}
	aimFit = [2]hmdMatrix34{ident34(), ident34()}
}

func TestWristBendsOnItsOwn(t *testing.T) {
	tt := newTester(t, true)
	gh := hands[1]
	tt.pose(1, v3(0.25, 1.1, -0.2), v3(0, 0, -1), v3(0, 1, 0))
	tt.frames(10)
	wrist := gh.local(0, -0.008, 0.045)
	f0 := gh.forearmDir(wrist)
	_, _, Z0 := gh.axes()
	// bend the hand 45 degrees down at the wrist, wrist where it was
	X, _, _ := gh.axes()
	tt.rotate(1, X, -45*math.Pi/180)
	tt.frames(10)
	w := gh.local(0, -0.008, 0.045)
	tt.placePalm(1, hands[1].palm.add(wrist.sub(w)))
	tt.frames(10)
	f1 := gh.forearmDir(gh.local(0, -0.008, 0.045))
	_, _, Z1 := gh.axes()
	if math.Acos(math.Max(-1, math.Min(1, Z0.dot(Z1)))) < 0.6 {
		t.Fatal("the hand didn't bend")
	}
	if a := math.Acos(math.Max(-1, math.Min(1, f0.dot(f1)))); a > 0.15 {
		t.Fatalf("the forearm swung %.0f degrees with the wrist", a*180/math.Pi)
	}
}

// ---- the maze's base stations ----

func wallReady(g *game, w *sim.MazeWall) float64 {
	k := 1.0
	for _, q := range g.wallPostPts(pv(w.A), pv(w.B)) {
		mp := g.mposts[mazePostKey(q)]
		if mp == nil {
			return 0
		}
		k = math.Min(k, mp.ready())
	}
	return k
}

func TestMazeStationsGrowThenLight(t *testing.T) {
	tt := newTester(t, false)
	g := tt.g
	a, b := sim.V2{X: 2.6, Z: 2.5}, sim.V2{X: -2.6, Z: 2.5}
	if _, why := g.r.PlaceWall(a, b); why != "" {
		t.Fatal("wall refused:", why)
	}
	w := g.r.Maze[len(g.r.Maze)-1]
	pts := g.wallPostPts(pv(a), pv(b))
	if len(pts) != 2 {
		t.Fatalf("a wall should stand on a station at each end, got %d", len(pts))
	}
	tt.frames(1)
	var born []float64
	for f := 0; f < 60*4; f++ {
		if f < 30 && wallReady(g, w) > 0 {
			t.Fatal("the sheet lit before its stations were up")
		}
		tt.frames(1)
		for i, q := range pts {
			mp := g.mposts[mazePostKey(q)]
			if mp != nil && !mp.pending && i >= len(born) {
				born = append(born, mp.born)
			}
		}
	}
	if len(born) != 2 {
		t.Fatalf("only %d of 2 stations went up", len(born))
	}
	for i := 1; i < 2; i++ {
		if born[i] <= born[i-1] {
			t.Errorf("stations should go up one after another along the wall: %v", born)
		}
	}
	if k := wallReady(g, w); k < 0.999 {
		t.Errorf("4 s after it was drawn the wall is only %.2f lit", k)
	}
	for _, q := range pts {
		if mp := g.mposts[mazePostKey(q)]; mp.busy(fxTime()) {
			t.Error("a finished station is still being rebuilt every frame")
		}
	}
	// take it down: the stations fold away and sink, then they're gone
	g.r.RemoveWall(w.ID)
	tt.frames(2)
	for _, q := range pts {
		if mp := g.mposts[mazePostKey(q)]; mp == nil || mp.gone == 0 {
			t.Fatal("a station vanished instead of sinking")
		}
	}
	tt.frames(75)
	if len(g.mposts) != 0 {
		t.Errorf("%d stations still standing after their wall came down", len(g.mposts))
	}
}

func checkBatches(t *testing.T, stage string) {
	t.Helper()
	for n, b := range map[string]*batch{"env": &bEnv, "opaque": &bOpaque, "trans": &bTrans, "glowtri": &bGlowTri, "floor": &bFloor, "static": &bStatic} {
		if c := b.count(); c%3 != 0 {
			t.Errorf("%s: %s batch has %d vertices (not whole triangles)", stage, n, c)
		}
	}
	for n, b := range map[string]*batch{"lines": &bLines, "glowline": &bGlowLine, "envl": &bEnvL, "floorl": &bFloorL, "staticl": &bStaticL} {
		if c := b.count(); c%2 != 0 {
			t.Errorf("%s: %s batch has %d vertices (not whole lines)", stage, n, c)
		}
	}
}

func TestBatchesStayAligned(t *testing.T) {
	for _, game := range []string{"simon", "needle", "freq", "safe", "plug", "valves", "look"} {
		tt := newTester(t, true)
		g := tt.g
		for i := range g.r.Dev.Mods {
			if g.r.Dev.Mods[i].ID == "game" {
				g.r.Dev.Mods[i].Game = game
			}
		}
		tt.frames(5)
		checkBatches(t, "start")
		for g.r.Dev.Cur < len(g.r.Dev.Mods) {
			id := g.r.Dev.Mods[g.r.Dev.Cur].ID
			tt.frames(40)
			checkBatches(t, game+" module "+id)
			g.r.CompleteModule()
		}
		g.openMenu()
		tt.frames(3)
		checkBatches(t, "menu")
		g.startFit()
		tt.frames(3)
		checkBatches(t, "fit")
		g.endFit()
	}
}

// The forearm runs from the wrist back toward the body for wherever the hand goes, and the
// wrist never bends past what a real one does.
func TestForearmFollowsTheArm(t *testing.T) {
	tt := newTester(t, true)
	head := v3(0, 1.6, 0.5)
	tt.setHead(head, v3(0, 0, -1))
	gh := hands[1]
	poses := []struct {
		name          string
		palm, fwd, up vec3
		elbowLow      bool
	}{
		{"reach", v3(0.18, 1.35, -0.05), v3(0, 0, -1), v3(0, 1, 0), true},
		{"down at the cart", v3(0.12, 1.0, 0.1), v3(0, -1, 0), v3(0, 0, -1), false},
		{"handset at the ear", v3(0.12, 1.56, 0.48), v3(0, 1, -0.3), v3(-1, 0, 0), true},
		{"reaching up", v3(0.2, 1.85, 0.3), v3(0, 1, -0.4), v3(0, 0, 1), false},
		{"across the chest", v3(-0.05, 1.2, 0.2), v3(-1, 0, 0), v3(0, 1, 0), true},
		{"out to the side", v3(0.55, 1.25, 0.5), v3(1, 0, 0), v3(0, 1, 0), true},
		{"palm up", v3(0.15, 1.15, 0.1), v3(0, 0, -1), v3(-1, 0, 0), true},
		{"wrist bent right back", v3(0.18, 1.2, 0.0), v3(0, 1, 0.3), v3(0, 0, 1), true},
	}
	for _, p := range poses {
		tt.pose(1, p.palm, p.fwd, p.up)
		tt.frames(20)
		wrist := gh.local(0, -0.008, 0.045)
		fd := gh.forearmDir(wrist)
		_, _, Z := gh.axes()
		bend := math.Acos(math.Max(-1, math.Min(1, fd.mul(-1).dot(Z.mul(-1)))))
		if bend > maxWristK+0.02 {
			t.Errorf("%s: the wrist is bent %.0f degrees", p.name, bend*180/math.Pi)
		}
		sh := shoulderOf(1)
		if fd.dot(sh.sub(wrist).norm()) < -0.2 {
			t.Errorf("%s: the forearm points away from the body", p.name)
		}
		if el := wrist.add(fd.mul(foreArmLen)); p.elbowLow && el.y > sh.y+0.05 {
			t.Errorf("%s: the elbow is up at %.2f, above the shoulder (%.2f)", p.name, el.y, sh.y)
		}
	}
}

// Looking round doesn't swing your shoulders with your head; turning your body does.
func TestShouldersDontFollowAGlance(t *testing.T) {
	tt := newTester(t, true)
	tt.setHead(v3(0, 1.6, 0.5), v3(0, 0, -1))
	tt.frames(30)
	f0, _ := torsoAxes()
	tt.setHead(v3(0, 1.6, 0.5), v3(math.Sin(0.4), 0, -math.Cos(0.4)))
	tt.frames(10)
	f1, _ := torsoAxes()
	if math.Acos(math.Min(1, f0.dot(f1))) > 0.15 {
		t.Error("a 23 degree glance turned the shoulders")
	}
	tt.setHead(v3(0, 1.6, 0.5), v3(1, 0, 0))
	tt.frames(10)
	f2, _ := torsoAxes()
	if f2.dot(v3(1, 0, 0)) < 0.8 {
		t.Error("turning to face the side left the shoulders behind")
	}
}

// A works on the green ring and B on magenta walls - a tap of B supercharges a magenta
// wall, holding B takes it down - so pointing through your ring at the maze, each button
// does what you meant.
// A magenta wall is easy to pick out with B: aiming at a base station past its end, just
// over its top, or at the floor a little short of it all count.
func TestMagentaWallIsEasyToAimAt(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	if _, why := r.PlaceWall(sim.V(-1.5, 4.5), sim.V(1.5, 4.5)); why != "" {
		t.Fatal(why)
	}
	tt.setHead(v3(0, 1.6, 0), v3(0, 0, 1))
	o := v3(0.3, 1.3, 0.5)
	for _, c := range []struct {
		why string
		at  vec3
	}{
		{"the base station past the end", v3(1.75, 0.4, 4.5)},
		{"just over the top", v3(0, r.T.WallHeight+0.25, 4.5)},
		{"the floor just short of it", v3(0, 0, 3.9)},
	} {
		d := c.at.sub(o).norm()
		tt.pose(1, o.sub(d.mul(0.05)), d, v3(0, 1, 0))
		tt.hs[1].bTouch = true
		tt.frames(2)
		if wh := hands[1].wallHit; !wh.ok || wh.kind != 2 {
			t.Errorf("aiming at %s doesn't pick out the wall (%+v)", c.why, wh)
		}
	}
	// well off to the side it doesn't
	d := v3(3.5, 0.5, 4.5).sub(o).norm()
	tt.pose(1, o.sub(d.mul(0.05)), d, v3(0, 1, 0))
	tt.frames(2)
	if wh := hands[1].wallHit; wh.ok && wh.kind == 2 {
		t.Error("a beam two metres past the wall's end still picks it out")
	}
	tt.hs[1].bTouch = false
}

func TestHoldBSuperchargesTriggerRemoves(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	if _, why := r.PlaceWall(sim.V(-1.5, 4.5), sim.V(1.5, 4.5)); why != "" {
		t.Fatal(why)
	}
	tt.setHead(v3(0, 1.6, 0), v3(0, 0, 1))
	o := v3(0.3, 1.3, 0.5)
	d := v3(0, 1.0, 4.5).sub(o).norm()
	tt.pose(1, o.sub(d.mul(0.05)), d, v3(0, 1, 0))
	// thumb on A: it picks out the green span the beam crosses, not the wall behind
	tt.hs[1].aTouch = true
	tt.frames(2)
	if wh := hands[1].wallHit; !wh.ok || wh.kind != 1 {
		t.Fatalf("A should pick out the green span (%+v)", wh)
	}
	tt.hs[1].aTouch = false
	// thumb on B: the magenta wall
	tt.hs[1].bTouch = true
	tt.frames(2)
	if wh := hands[1].wallHit; !wh.ok || wh.kind != 2 {
		t.Fatalf("B should pick out the magenta wall (%+v)", wh)
	}
	holdB := func(n int, trig bool) {
		for k := 0; k < n; k++ {
			tt.bitB(1, true)
			if trig {
				tt.hs[1].pressed |= 1 << bitTrigger
				tt.hs[1].trig = 1
			}
			tt.frames(1)
		}
		tt.bitB(1, false)
		tt.hs[1].pressed &^= 1 << bitTrigger
		tt.hs[1].trig = 0
		tt.frames(2)
	}
	// let go early: cancelled, nothing spent
	holdB(30, false)
	if r.SuperCD > 0 || len(r.Maze) != 1 || r.Maze[0].Super > 0 || tt.g.menuOpen {
		t.Fatal("a short press of B on the wall did something")
	}
	// held till it fills: supercharged
	holdB(int(superHold*60)+4, false)
	if r.SuperCD <= 0 || len(r.Maze) != 1 {
		t.Fatalf("holding B should supercharge the wall (cooldown %.1f, walls %d)", r.SuperCD, len(r.Maze))
	}
}

// Holding the trigger on a magenta wall folds it away; let go early and it unfolds again,
// hold it all the way and it's gone with every point back in your pool.
func TestTriggerHoldRemovesWall(t *testing.T) {
	for _, playing := range []bool{false, true} {
		tt := newTester(t, playing)
		r := tt.g.r
		before := r.Points
		id, why := r.PlaceWall(sim.V(-1.5, 4.5), sim.V(1.5, 4.5))
		if why != "" {
			t.Fatal(why)
		}
		spent := before - r.Points
		tt.setHead(v3(0, 1.6, 0), v3(0, 0, 1))
		o := v3(0.3, 1.3, 0.5)
		d := v3(0.4, 1.2, 4.5).sub(o).norm()
		tt.pose(1, o.sub(d.mul(0.05)), d, v3(0, 1, 0))
		tt.hs[1].trig = 0.5
		tt.frames(2)
		if eh := hands[1].eraseHit; !eh.ok || eh.id != id {
			t.Fatalf("pointing at the wall, the trigger doesn't pick it out (%+v)", eh)
		}
		pull := func(n int) {
			for k := 0; k < n; k++ {
				tt.hs[1].pressed |= 1 << bitTrigger
				tt.hs[1].trig = 1
				tt.frames(1)
			}
			tt.hs[1].pressed &^= 1 << bitTrigger
			tt.hs[1].trig = 0
		}
		pull(int(eraseSecs * 60 / 2))
		if k := tt.g.retract[id]; k < 0.3 || k > 0.7 || r.Wall(id) == nil {
			t.Fatalf("half way through it should be half folded and still there (%.2f)", k)
		}
		tt.frames(int(eraseSecs * 60))
		if tt.g.retract[id] > 0 || r.Wall(id) == nil {
			t.Fatal("let go early, it should unfold back to full size")
		}
		if len(r.Maze) != 1 {
			t.Fatal("the trigger on the wall drew a new one")
		}
		pull(int(eraseSecs*60) + 4)
		tt.frames(2)
		if r.Wall(id) != nil {
			t.Fatal("held all the way, the wall should be gone")
		}
		if math.Abs(r.Points-before) > 0.01 {
			t.Fatalf("its points should all be back (spent %.1f, %.1f short)", spent, before-r.Points)
		}
		if r.SuperCD > 0 {
			t.Fatal("taking it down used the supercharge")
		}
	}
}

// Holding the cart doesn't make it safe: a warden wrenches it out of your hands.
func TestWardenTakesTheCartOutOfYourHands(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	d := &r.Dev
	for i := range r.Gens {
		if d.Exposed {
			break
		}
		r.SetPower(i, false)
		tt.frames(2)
	}
	if !d.Exposed {
		t.Skip("this layout still covers the device")
	}
	hd := tt.g.devVis.handles[0]
	tt.grabMoving(1, hd.gPos)
	if !d.Held {
		t.Fatal("holding the handle didn't count as holding the cart")
	}
	d.Field = 0
	b := r.Spawn(sim.Walker, 0, "")
	b.Pos = d.Pos.Add(sim.V(1.2, 0.3))
	for k := 0; k < 60*20 && d.State != sim.DevStolen; k++ {
		tt.placePalm(1, hd.gPos())
		tt.frames(1)
	}
	if d.State != sim.DevStolen {
		t.Fatalf("with the field down a warden should take it even from your hands (state %d)", d.State)
	}
}

// With your ring down, only a couple of wardens come right into your room - the rest wait
// outside - and none of them stands on top of you.
func TestInvasionStaysThin(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	for i := range r.Gens {
		r.SetPower(i, false)
	}
	for k := 0; k < 10; k++ {
		r.Spawn(sim.Walker, k%4, "")
	}
	tt.setHead(v3(0.2, 1.6, 0.3), v3(0, 0, -1))
	worstIn, worstNear := 0, 9.0
	for f := 0; f < 60*45; f++ {
		tt.frames(1)
		if f < 60*15 {
			continue // give them time to arrive
		}
		in := 0
		for _, b := range r.Robots {
			if !b.Alive() || b.Kind == sim.Drone {
				continue
			}
			if sim.InPoly(r.Room, b.Pos) && b.State == sim.RLurk {
				in++
			}
			if d := sim.Dist(b.Pos, r.Player) - b.Radius(); d < worstNear {
				worstNear = d
				if d < sim.PersonalSpace-0.05 {
					t.Logf("t %.1f warden %d state %d at %.2f,%.2f, %.2f from you", r.Time, b.ID, b.State, b.Pos.X, b.Pos.Z, d)
				}
			}
		}
		worstIn = max(worstIn, in)
	}
	if worstIn > sim.MaxInside {
		t.Errorf("%d wardens hanging about inside your room (at most %d should)", worstIn, sim.MaxInside)
	}
	if worstNear < sim.PersonalSpace-0.05 {
		t.Errorf("a warden got within %.2f m of you", worstNear)
	}
}

// Index: just holding the controller reads a high squeeze value; the hand still opens as
// soon as you stop squeezing.
func TestIndexGripOpensWhenYouStopSqueezing(t *testing.T) {
	tt := newTester(t, true)
	h, gh := tt.hs[1], hands[1]
	h.grip, h.force = 0.9, 0 // fingers wrapped round the handle, not squeezing
	tt.frames(2)
	h.force = 0.5 // squeeze
	tt.frames(2)
	if !gh.closed {
		t.Fatal("squeezing didn't close the hand")
	}
	h.force = 0.03 // let go of the squeeze, fingers still on the grip
	tt.frames(2)
	if gh.closed {
		t.Fatal("the hand stayed shut after the squeeze ended")
	}
}

// The cart is heavy: yank a handle fast and your grip slips instead of the cart flying.
func TestCartIsHeavy(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	h := tt.g.devVis.handles[0]
	p0 := r.Dev.Pos
	at := h.gPos()
	tt.grabAt(1, at)
	tt.move(1, at.add(v3(0, 0, 0.9)), 30) // 1.8 m/s
	if hands[1].held != nil {
		t.Fatal("yanked hard, the hand should have slipped off the handle")
	}
	if moved := sim.Dist(p0, r.Dev.Pos); moved > 0.3 {
		t.Fatalf("a yank shot the cart %.2f m", moved)
	}
}

// The cords lie over the cart (and the radio) instead of passing through them.
func TestCordsDrapeOverTheCart(t *testing.T) {
	tt := newTester(t, true)
	tt.toModule("wires", 0)
	m := tt.mod().(*modWires)
	F := tt.g.devFrame()
	box := tt.g.housingBox(F)
	for i, p := range m.plugs {
		for _, end := range []vec3{m.socket(F, m.order[i]).add(F.y.mul(0.03)), p.pos} {
			pts := drapeCord(p.anchor(), end, p.cordLen, 14, 0.007, box)
			for k, q := range pts[1 : len(pts)-1] {
				if box.pushOut(q, 0.005) != q {
					t.Fatalf("plug %d's cord passes through the cart at point %d", i, k+1)
				}
			}
		}
	}
}

// waitForLeech: a drone sent at generator 0's side of the room, hovering and clamped on.
func waitForLeech(tt *tester, door int) *sim.Robot {
	r := tt.g.r
	b := r.Spawn(sim.Drone, door, "leech")
	for k := 0; k < 60*40 && b.State != sim.RAttached; k++ {
		tt.frames(1)
	}
	if b.State != sim.RAttached {
		tt.t.Fatalf("the leech never clamped on (state %d)", b.State)
	}
	tt.frames(int(sim.LeechHop*60) + 2) // up over the beam, tether down
	return b
}

// A leech hovers over the generator; its tether clamps on the inside of the shaft, in reach.
// Yank the clamp: the generator's free, and the drone goes off to patch itself up before
// it comes back.
func TestLeechUnhookedComesBack(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	b := waitForLeech(tt, 0)
	gi := b.Gen
	pos, _ := tt.g.dronePos(b)
	if pos.y < genH+0.3 {
		t.Fatalf("the leech should hover above the generator, it's at %.2f m", pos.y)
	}
	lh := tt.g.leeches[b.ID]
	at := sim.V(lh.at.x, lh.at.z)
	if !sim.InPoly(r.RingPoly, at) || !sim.InPoly(r.Room, at) || lh.at.y > 1.8 {
		t.Fatal("the tether's clamp isn't on the inside, in reach")
	}
	tt.frames(int(r.T.LeechBite*60) + 10)
	if r.Gens[gi].State != sim.GenLeeched {
		t.Fatalf("the leech never bit through (state %d)", r.Gens[gi].State)
	}
	tt.grabAt(1, lh.at)
	tt.move(1, lh.at.add(v3(0, -0.3, 0)), 8)
	tt.release(1)
	if b.State != sim.RRecover {
		t.Fatalf("yanking the clamp should send it off to recover (state %d, cause %q, held %T)", b.State, b.Cause, hands[1].held)
	}
	if r.Gens[gi].State != sim.GenUp {
		t.Fatalf("the generator didn't come back when the tether came off (state %d)", r.Gens[gi].State)
	}
	if bp := b.Pos; sim.InPoly(r.RingPoly, bp) {
		t.Fatal("it should recover outside your ring")
	}
	tt.frames(int(sim.RecoverSecs*60) + 60)
	if b.State == sim.RRecover || !b.Alive() {
		t.Fatalf("it never came back for another go (state %d)", b.State)
	}
}

// The pole by the radio smashes a leech for good - if you swing it.
func TestLeechSmashedWithThePole(t *testing.T) {
	tt := newTester(t, true)
	b := waitForLeech(tt, 1)
	k := tt.g.pole
	at := k.p.pos
	tt.pose(1, at.add(v3(0, 0.15, 0)), v3(0, 0, -1), v3(0, 1, 0))
	tt.frames(1)
	tt.grabAt(1, at)
	if hands[1].held != k.p {
		t.Fatal("couldn't pick up the pole")
	}
	dp, _ := tt.g.dronePos(b)
	// stand under it and bring the pole's head up through it
	dir := v3(-0.4, 0.8, -0.2).norm()
	tt.pose(1, dp.sub(dir.mul(poleLen-poleGrip+0.42)), dir, v3(0, 1, 0)) // its tip 35 cm short
	tt.g.pole.wasHeld = false                                            // the scripted hand jumped: no swing from where it was
	tt.frames(2)
	if !b.Alive() {
		t.Fatal("the leech broke before the pole moved")
	}
	tt.move(1, hands[1].palm.add(dir.mul(0.6)), 7) // a hard jab, 5 m/s, through it
	if b.Alive() {
		t.Fatalf("a hard hit with the pole should break it (state %d)", b.State)
	}
}

// The pole clubs wardens too - a hard jab takes one down - but the third one snaps it.
func TestPoleClubsWardensAndSnaps(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	k := tt.g.pole
	tt.pose(1, k.p.pos.add(v3(0, 0.15, 0)), v3(0, 0, -1), v3(0, 1, 0))
	tt.frames(1)
	tt.grabAt(1, k.p.pos)
	if hands[1].held != k.p {
		t.Fatal("couldn't pick up the pole")
	}
	dir := v3(0, 0, -1)
	for n := 1; n <= poleLife; n++ {
		b := r.Spawn(sim.Walker, 0, "")
		b.Pos = sim.V(0.3*float64(n)-0.6, -0.9)
		b.Path, b.State, b.Wait = nil, sim.RWait, 100
		body := fv(b.Pos, 1.0)
		// the pole's head 40 cm short of it, then a hard jab
		tt.pose(1, body.sub(dir.mul(poleLen-poleGrip+b.Radius()+0.4)), dir, v3(0, 1, 0))
		tt.g.pole.wasHeld = false // the scripted hand jumped: no swing from where it was
		tt.frames(2)
		if !b.Alive() {
			t.Fatalf("warden %d went down before the pole moved", n)
		}
		tt.move(1, hands[1].palm.add(dir.mul(0.6)), 6)
		if b.Alive() {
			t.Fatalf("a hard jab should take warden %d down", n)
		}
		if b.Cause != "pole" {
			t.Fatalf("warden %d died of %q", n, b.Cause)
		}
		if n < poleLife && (k.broken || hands[1].held != k.p) {
			t.Fatalf("the pole broke after only %d", n)
		}
	}
	if !k.broken || hands[1].held == k.p || k.p.gOK() {
		t.Fatal("the third warden should snap the pole and it's gone")
	}
	// and with it gone, a leech has to be yanked off
	tt.release(1)
	b := waitForLeech(tt, 1)
	dp, _ := tt.g.dronePos(b)
	tt.pose(1, dp.sub(v3(0, 1, 0)), v3(0, 1, 0), v3(0, 0, 1))
	tt.frames(1)
	tt.move(1, dp, 6)
	if !b.Alive() {
		t.Fatal("a broken pole still smashed a leech")
	}
}

// A gentle nudge with the pole doesn't break it.
func TestLeechShrugsOffANudge(t *testing.T) {
	tt := newTester(t, true)
	b := waitForLeech(tt, 1)
	k := tt.g.pole
	tt.pose(1, k.p.pos.add(v3(0, 0.15, 0)), v3(0, 0, -1), v3(0, 1, 0))
	tt.frames(1)
	tt.grabAt(1, k.p.pos)
	dp, _ := tt.g.dronePos(b)
	dir := v3(-0.4, 0.8, -0.2).norm()
	tt.pose(1, dp.sub(dir.mul(poleLen-poleGrip+0.42)), dir, v3(0, 1, 0))
	tt.g.pole.wasHeld = false // the scripted hand jumped: no swing from where it was
	tt.frames(2)
	tt.move(1, hands[1].palm.add(dir.mul(0.6)), 72) // 0.5 m/s
	if !b.Alive() {
		t.Fatal("a slow nudge with the pole broke it")
	}
}

// hookWarden: arm the turret and spear a warden standing a few metres out in the hall.
func hookWarden(tt *tester) (*sim.Harpoon, *sim.Robot) {
	r := tt.g.r
	h := armTurret(tt)
	_, muzzle := tt.g.harpoonFrame()
	out := h.Pos.Sub(sim.Centroid(r.Room)).Norm()
	b := r.Spawn(sim.Walker, 0, "")
	b.Pos = h.Pos.Add(out.Mul(3.2))
	b.Path, b.State, b.Wait = nil, sim.RWait, 100
	tt.frames(1)
	holdTurret(tt, fv(b.Pos, 1.0).sub(muzzle).norm())
	tt.bits(1, true, false)
	tt.frames(1)
	tt.bits(1, false, false)
	tt.hs[1].trig = 0
	for k := 0; k < 60 && h.Kind == sim.ShotFlying; k++ {
		tt.frames(1)
	}
	if h.Kind != sim.ShotRobot || h.Target != b.ID {
		tt.t.Fatalf("the spear didn't stick in the warden (kind %d)", h.Kind)
	}
	tt.release(0)
	tt.release(1)
	return h, b
}

// Crank the winch on the turret's side: the warden's hauled in, and dragged through your
// green ring it fries.
func TestHarpoonWinchHaulsThemThroughTheRing(t *testing.T) {
	tt := newTester(t, true)
	_, b := hookWarden(tt)
	k := tt.g.reel
	tt.frames(1)
	at := k.handle()
	tt.pose(1, at.add(k.ax.mul(0.15)), k.ax.mul(-1), v3(0, 1, 0))
	tt.frames(1)
	tt.grabAt(1, at)
	if hands[1].held != k {
		t.Fatal("couldn't take hold of the winch")
	}
	for turn := 0; turn < 12 && b.Alive(); turn++ {
		tt.circle(1, k.c, k.ax, k.ref, k.R, 1, 40, k.ang)
	}
	if b.Alive() {
		t.Fatalf("twelve turns of the winch and it's still out there (%.2f m)", sim.Dist(b.Pos, tt.g.r.Harp.Pos))
	}
	if b.Cause != "dragged" {
		t.Fatalf("it should have been fried hauled through the ring, died of %q", b.Cause)
	}
}

// A line stuck in the floor ties off to the cleat and stays up as a trap; pointing at the
// spent turret and tapping A spends the supercharge on a fresh spear.
func TestHarpoonTrapAndRecharge(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	h := armTurret(tt)
	_, muzzle := tt.g.harpoonFrame()
	out := h.Pos.Sub(sim.Centroid(r.Room)).Norm()
	holdTurret(tt, fv(h.Pos.Add(out.Mul(4)), 0).sub(muzzle).norm())
	tt.bits(1, true, false)
	tt.frames(1)
	tt.bits(1, false, false)
	tt.hs[1].trig = 0
	for k := 0; k < 120 && h.Kind == sim.ShotFlying; k++ {
		tt.frames(1)
	}
	if h.Kind != sim.ShotAnchor || h.Armed {
		t.Fatalf("the spear should be in the floor and the turret spent (kind %d)", h.Kind)
	}
	tt.release(0)
	tt.release(1)
	// grab the line near the muzzle and bring it down to the cleat
	_, muzzle = tt.g.harpoonFrame()
	along := v3s(h.Tip).sub(muzzle).norm()
	at := muzzle.add(along.mul(0.3))
	tt.pose(1, at.add(v3(0, 0.1, 0)), along, v3(0, 1, 0))
	tt.frames(1)
	tt.grabAt(1, at)
	tt.move(1, tt.g.harpCleat(), 15)
	tt.release(1)
	if h.Kind != sim.ShotNone || len(h.Traps) != 1 {
		t.Fatalf("the line wasn't tied off (kind %d, traps %d)", h.Kind, len(h.Traps))
	}
	// point at the turret, thumb on A, tap
	F, _ := tt.g.harpoonFrame()
	in := sim.Centroid(r.Room).Sub(h.Pos).Norm()
	o := F.o.add(v3(in.X, 0, in.Z).mul(1.2)).add(v3(0, 0.25, 0))
	tt.pose(1, o, F.o.sub(o).norm(), v3(0, 1, 0))
	tt.hs[1].aTouch = true
	tt.frames(2)
	if wh := hands[1].wallHit; !wh.ok || wh.kind != 5 {
		t.Fatalf("thumb on A, pointing at the spent turret, doesn't pick it out (%+v)", wh)
	}
	tt.bits(1, false, true)
	tt.frames(int(superHold*60) + 4)
	tt.bits(1, false, false)
	tt.frames(2)
	if !h.Armed || r.SuperCD <= 0 {
		t.Fatalf("holding A should spend the supercharge on a new spear (armed %v, cooldown %.1f)", h.Armed, r.SuperCD)
	}
}

// superTurret: point at the turret with the thumb on A and tap A.
func superTurret(tt *tester) {
	r := tt.g.r
	h := &r.Harp
	F, _ := tt.g.harpoonFrame()
	in := sim.Centroid(r.Room).Sub(h.Pos)
	if in.Len() < 0.3 {
		in = sim.V(0, 1)
	}
	in = in.Norm()
	o := F.o.add(v3(in.X, 0, in.Z).mul(1.0)).add(v3(0, 0.25, 0))
	tt.pose(1, o, F.o.sub(o).norm(), v3(0, 1, 0))
	tt.hs[1].aTouch = true
	tt.frames(2)
	tt.bits(1, false, true)
	tt.frames(int(superHold*60) + 4)
	tt.bits(1, false, false)
	tt.hs[1].aTouch = false
	tt.frames(2)
}

// They've got the EMP: supercharge the turret, spear the cart. The warden hauling it fries,
// so does one standing on the line, the others nearby are left reeling, and the winch
// brings the cart home.
func TestSuperHarpoonGetsTheEMPBack(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	h := armTurret(tt)
	r.SuperCD = 0
	superTurret(tt)
	if !h.Super || !h.Armed || r.SuperCD <= 0 {
		t.Fatalf("tapping A at the turret should supercharge it (super %v armed %v)", h.Super, h.Armed)
	}
	if tt.g.harpUp < 0.99 {
		t.Fatal("supercharging sank the turret back into the floor")
	}
	// the cart, out in the hall, in a warden's hands
	d := &r.Dev
	out := h.Pos.Sub(sim.Centroid(r.Room)).Norm()
	side := sim.V(-out.Z, out.X)
	d.Pos = h.Pos.Add(out.Mul(3.4))
	carrier := r.Spawn(sim.Walker, 0, "")
	carrier.Pos = d.Pos.Add(out.Mul(0.5))
	carrier.State, carrier.Path = sim.RCarry, []sim.V2{r.Doors[0].Mouth}
	cs := r.T.CarrySpeed
	r.T.CarrySpeed = 0.01 // it's barely getting anywhere while you line up
	defer func() { r.T.CarrySpeed = cs }()
	d.State, d.Carrier, d.Held = sim.DevStolen, carrier.ID, false
	inWay := r.Spawn(sim.Walker, 0, "")
	inWay.Pos = h.Pos.Add(out.Mul(2.2))
	inWay.State, inWay.Wait, inWay.Path = sim.RWait, 100, nil
	near := r.Spawn(sim.Walker, 0, "")
	near.Pos = d.Pos.Add(side.Mul(1.6))
	near.State, near.Wait, near.Path = sim.RWait, 100, nil
	tt.frames(1)
	piv, _ := tt.g.harpoonFrame()
	holdTurret(tt, fv(d.Pos, 0.5).sub(piv.o).norm()) // the muzzle's on the line from the pivot
	tt.bits(1, true, false)
	tt.frames(1)
	tt.bits(1, false, false)
	tt.hs[1].trig = 0
	for k := 0; k < 60 && h.Kind == sim.ShotFlying; k++ {
		tt.frames(1)
	}
	if h.Kind != sim.ShotDevice {
		t.Fatalf("the spear should be in the cart (kind %d, carrier %v %q, in the way %v %q, dev state %d)", h.Kind, carrier.Alive(), carrier.Cause, inWay.Alive(), inWay.Cause, d.State)
	}
	if carrier.Alive() || carrier.Cause != "shock" {
		t.Fatalf("the warden hauling it should be shocked dead (%v, %q)", carrier.Alive(), carrier.Cause)
	}
	if !near.Alive() || near.Dazed <= 0 {
		t.Fatalf("the warden near it should be left reeling (alive %v, dazed %.1f)", near.Alive(), near.Dazed)
	}
	tt.release(0)
	tt.release(1)
	tt.frames(2)
	if inWay.Alive() {
		t.Fatal("the warden standing on the live line should have fried")
	}
	// crank it home
	k := tt.g.reel
	at := k.handle()
	tt.pose(1, at.add(k.ax.mul(0.15)), k.ax.mul(-1), v3(0, 1, 0))
	tt.frames(1)
	tt.grabAt(1, at)
	for turn := 0; turn < 12 && d.State != sim.DevHome; turn++ {
		tt.circle(1, k.c, k.ax, k.ref, k.R, 1, 40, k.ang)
	}
	if d.State != sim.DevHome {
		t.Fatalf("twelve turns and the cart isn't home (state %d, %.2f m out)", d.State, sim.Dist(d.Pos, h.Pos))
	}
}

// The turret works wherever your ring is - even with the ring gone.
func TestHarpoonFiresWithoutTheRing(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	h := armTurret(tt)
	for i := range r.Gens {
		r.SetPower(i, false)
	}
	tt.frames(5)
	if !h.Powered {
		t.Fatal("the turret lost power with the ring")
	}
	_, muzzle := tt.g.harpoonFrame()
	out := h.Pos.Sub(sim.Centroid(r.Room)).Norm()
	holdTurret(tt, fv(h.Pos.Add(out.Mul(4)), 0).sub(muzzle).norm())
	tt.bits(1, true, false)
	tt.frames(2)
	tt.bits(1, false, false)
	if h.Kind == sim.ShotNone {
		t.Fatal("it wouldn't fire with the ring down")
	}
}

// Force pull: point at the screwdriver across the room with the trigger eased in, flick the
// hand back and it flies over; squeeze and it's in your hand.
func TestForcePullTheScrewdriver(t *testing.T) {
	tt := newTester(t, true)
	k := tt.g.driver
	p := k.p
	// it's lying on the floor a couple of metres off
	p.docked, p.resting, p.held = false, true, false
	p.pos = v3(-0.9, p.hy, -0.9)
	tt.frames(1)
	// somewhere to stand with nothing else in reach of the hand
	var o, d vec3
	for _, c := range []vec3{{0.9, 1.4, 1.0}, {-0.2, 1.5, 1.1}, {1.1, 1.5, -0.3}, {0.6, 1.2, 0.6}} {
		o, d = c, p.pos.sub(c).norm()
		tt.pose(1, o, d, v3(0, 1, 0))
		tt.frames(2)
		if hands[1].hover == nil {
			break
		}
	}
	tt.frames(30) // hold still
	if hands[1].pull.phase != pullNone {
		t.Fatal("it locked on without the trigger")
	}
	tt.hs[1].trig = 0.4
	tt.frames(12)
	if hands[1].pull.phase != pullLocked || hands[1].pull.p != p {
		t.Fatalf("easing the trigger in while pointing at it should lock on (phase %d, beam %v, hover %v, driver at %.2f,%.2f,%.2f)", hands[1].pull.phase, hands[1].beam, hands[1].hover != nil, p.pos.x, p.pos.y, p.pos.z)
	}
	// the flick: back toward you, fast
	tt.move(1, hands[1].palm.sub(d.mul(0.3)), 5)
	tt.hs[1].trig = 0
	if hands[1].pull.phase != pullFlying && hands[1].pull.phase != pullWaiting {
		t.Fatalf("a flick should send it flying (phase %d)", hands[1].pull.phase)
	}
	for i := 0; i < 60 && hands[1].pull.phase == pullFlying; i++ {
		tt.frames(1)
	}
	if hands[1].pull.phase != pullWaiting || p.pos.sub(hands[1].palm).length() > 0.2 {
		t.Fatalf("it should be at your hand waiting to be caught (phase %d, %.2f m off)", hands[1].pull.phase, p.pos.sub(hands[1].palm).length())
	}
	tt.grip(1, true)
	tt.frames(2)
	if hands[1].held != p {
		t.Fatal("squeezing should catch it")
	}
	tt.release(1)
}

// Not caught, it drops; and the pole comes over the same way.
func TestForcePullDropsIfNotCaught(t *testing.T) {
	tt := newTester(t, true)
	pole := tt.g.pole.p
	_, head := tt.g.pole.ends()
	c := pole.pos.add(head).mul(0.5)
	// from out in the room (the prod's bracket is on a generator by a wall)
	mid := sim.Centroid(tt.g.r.Room)
	o := c.add(fv(mid.Sub(sim.V(c.x, c.z)).Norm(), 0).mul(1.5)).add(v3(0, 0.3, 0))
	d := c.sub(o).norm()
	tt.pose(1, o, d, v3(0, 1, 0))
	tt.frames(30)
	tt.hs[1].trig = 0.4
	tt.frames(12)
	if hands[1].pull.p != pole {
		t.Fatalf("pointing at the pole should lock onto it (phase %d)", hands[1].pull.phase)
	}
	tt.move(1, hands[1].palm.add(v3(0, 0.3, 0)), 5) // an upward flick works too
	tt.hs[1].trig = 0
	tt.frames(90)
	if hands[1].held == pole || hands[1].pull.phase != pullNone {
		t.Fatal("not caught, it should have dropped")
	}
	if pole.held || pole.docked {
		t.Fatal("the pole should be lying where it fell")
	}
}

// The prod's tip kills a leech that's just flying about, too.
func TestPoleZapsAFlyingLeech(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	k := tt.g.pole
	tt.pose(1, k.p.pos.add(v3(0, 0.15, 0)), v3(0, 0, -1), v3(0, 1, 0))
	tt.frames(1)
	tt.grabAt(1, k.p.pos)
	b := r.Spawn(sim.Drone, 0, "leech")
	b.Pos, b.State, b.Wait, b.Path = sim.V(0.4, -0.6), sim.RWait, 100, nil
	tt.frames(1)
	dp, _ := tt.g.dronePos(b)
	dir := v3(0, 0, -1)
	tt.pose(1, dp.sub(dir.mul(poleLen-poleGrip+0.42)), dir, v3(0, 1, 0))
	tt.g.pole.wasHeld = false
	tt.frames(2)
	if !b.Alive() {
		t.Fatal("the leech went down before the pole moved")
	}
	tt.move(1, hands[1].palm.add(dir.mul(0.6)), 7)
	if b.Alive() {
		t.Fatalf("a hard jab should zap a flying leech (state %d)", b.State)
	}
}

// Dropped, the pole falls as a stick: it lands on the floor (not through it), bounces about
// and settles lying flat.
func TestDroppedPoleTumblesAndSettles(t *testing.T) {
	tt := newTester(t, true)
	k := tt.g.pole
	p := k.p
	tt.pose(1, p.pos.add(v3(0, 0.15, 0)), v3(0, 0, -1), v3(0, 1, 0))
	tt.frames(1)
	tt.grabAt(1, p.pos)
	// hold it up and out, then let go
	tt.move(1, v3(0.8, 1.5, 0.9), 30)
	tt.frames(20)
	tt.release(1)
	tt.frames(1)
	if !k.loose {
		t.Fatal("let go, the pole should be falling as a loose stick")
	}
	lowest, bounced := 9.0, false
	prevVy := 0.0
	for i := 0; i < 60*6 && k.loose; i++ {
		tt.frames(1)
		for _, e := range k.e {
			lowest = math.Min(lowest, e.y)
		}
		vy := k.ev[0].y + k.ev[1].y
		if prevVy < -0.5 && vy > 0.2 {
			bounced = true
		}
		prevVy = vy
	}
	if k.loose || !p.resting {
		t.Fatal("the pole never settled")
	}
	if lowest < 0 {
		t.Fatalf("it went through the floor (%.2f)", lowest)
	}
	if !bounced {
		t.Error("it landed dead, without a bounce")
	}
	butt, head := k.ends()
	if math.Abs(butt.y-head.y) > 0.1 || butt.y > 0.1 {
		t.Fatalf("it should be lying flat on the floor (ends at %.2f and %.2f)", butt.y, head.y)
	}
}

// The pole has weight: a sudden turn of the wrist and its head lags behind, then catches up.
func TestPoleSwingHasWeight(t *testing.T) {
	tt := newTester(t, true)
	k := tt.g.pole
	p := k.p
	tt.pose(1, p.pos.add(v3(0, 0.15, 0)), v3(0, 0, -1), v3(0, 1, 0))
	tt.frames(1)
	tt.grabAt(1, p.pos)
	tt.frames(30)
	tt.rotate(1, v3(0, 1, 0), 60*math.Pi/180) // a 60 degree flick in one frame
	tt.frames(1)
	want := hands[1].m.axis(2)
	if a := math.Acos(math.Min(1, p.Z.dot(want))); a < 20*math.Pi/180 {
		t.Fatalf("the pole followed the flick instantly (%.0f degrees behind)", a*180/math.Pi)
	}
	tt.frames(40)
	if a := math.Acos(math.Min(1, p.Z.dot(hands[1].m.axis(2)))); a > 3*math.Pi/180 {
		t.Fatalf("the pole never caught up with the wrist (%.0f degrees off)", a*180/math.Pi)
	}
}

// The cart's on the floor out in the hall (its carrier died) and the turret has no spear:
// aim at the cart and pull the trigger, and the tractor beam hauls it home - the wardens
// can't take it out of the beam.
func TestTractorBeamBringsTheCartHome(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	h := armTurret(tt)
	h.Armed = false
	d := &r.Dev
	out := h.Pos.Sub(sim.Centroid(r.Room)).Norm()
	d.Pos, d.State, d.DropT, d.Carrier = h.Pos.Add(out.Mul(3.2)), sim.DevDropped, 4, 0
	w := r.Spawn(sim.Walker, 0, "")
	w.Pos = d.Pos.Add(out.Mul(0.9))
	tt.frames(1)
	piv, _ := tt.g.harpoonFrame()
	holdTurret(tt, fv(d.Pos, 0.35).sub(piv.o).norm())
	tt.bits(1, true, false)
	tt.frames(1)
	tt.bits(1, false, false)
	tt.hs[1].trig = 0
	if !h.Beam {
		t.Fatal("the trigger with no spear, aimed at the dropped cart, should lock the tractor beam on")
	}
	tt.release(0)
	tt.release(1)
	for k := 0; k < 60*10 && d.State == sim.DevDropped; k++ {
		tt.frames(1)
	}
	if d.State != sim.DevHome {
		t.Fatalf("the beam should have brought the cart home (state %d, %.2f m out)", d.State, sim.Dist(d.Pos, h.Pos))
	}
	if h.Beam {
		t.Fatal("the beam should let go once it's home")
	}
}

// The spear hits the cart anywhere on its housing - right up to the lid.
func TestSpearHitsTheCartsWholeBox(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	d := &r.Dev
	d.State = sim.DevDropped
	if !r.SpearHitsCart(sim.V3{X: d.Pos.X, Y: 0.4, Z: d.Pos.Z}) {
		t.Fatal("dropped: the middle of the cart isn't a hit")
	}
	d.State = sim.DevStolen
	f := sim.V(math.Cos(d.Yaw), math.Sin(d.Yaw))
	top := d.Pos.Add(f.Mul(0.3)).At(sim.DevTop + 0.2)
	if !r.SpearHitsCart(top) {
		t.Fatal("carried: the spear goes through the top corner of the cart")
	}
	if r.SpearHitsCart(d.Pos.Add(f.Mul(0.8)).At(0.5)) {
		t.Fatal("half a metre past the cart's end still counts as a hit")
	}
}

// A supercharged wall's blast reaches out: right up against it a warden fries at once;
// further out it takes damage that falls off (a heavy burns down slowly); well away it's
// untouched. And a warden that's been hit doesn't last long draining your ring.
func TestSuperchargeReachesNearbyRobots(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	id, why := r.PlaceWall(sim.V(-1.5, 4.5), sim.V(1.5, 4.5))
	if why != "" {
		t.Fatal(why)
	}
	put := func(kind sim.RobotKind, x, z float64) *sim.Robot {
		b := r.Spawn(kind, 0, "")
		b.Pos, b.State, b.Wait, b.Path = sim.V(x, z), sim.RWait, 100, nil
		return b
	}
	close := put(sim.Walker, 0.3, 4.95) // its body nearly touching
	near := put(sim.Walker, -0.6, 5.4)  // 0.9 m off the wall
	edge := put(sim.Walker, 1.0, 6.2)   // 1.7 m off: the edge of the blast
	far := put(sim.Walker, 0.5, 7.5)    // 3 m off
	tt.frames(1)
	r.SuperCD = 0
	if !r.SuperWall(id) {
		t.Fatal("couldn't supercharge the wall")
	}
	tt.frames(2)
	if close.Alive() {
		t.Fatal("a warden right up against the supercharged wall should fry at once")
	}
	if !near.Alive() {
		t.Fatal("a metre off it should take damage, not fry at once")
	}
	tt.frames(60 * 4)
	if near.Alive() {
		t.Fatal("a warden standing a metre off it the whole time should burn down")
	}
	if !edge.Alive() || edge.HP >= r.MaxHP(edge) {
		t.Fatalf("at the edge of the blast it should be hurt but alive (alive %v, power %.1f)", edge.Alive(), edge.HP)
	}
	if !far.Alive() || far.HP < r.MaxHP(far) {
		t.Fatal("a warden three metres off it was touched")
	}
}

// Swing the turret with a live line strung from it and the line swings too - a warden it
// sweeps through fries.
func TestSwingingTheLiveLineZaps(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	h := armTurret(tt)
	out := h.Pos.Sub(sim.Centroid(r.Room)).Norm()
	side := sim.V(-out.Z, out.X)
	piv, _ := tt.g.harpoonFrame()
	floor := fv(h.Pos.Add(out.Mul(2)), 0)
	holdTurret(tt, floor.sub(piv.o).norm())
	tt.bits(1, true, false)
	tt.frames(1)
	tt.bits(1, false, false)
	tt.hs[1].trig = 0
	for k := 0; k < 120 && h.Kind == sim.ShotFlying; k++ {
		tt.frames(1)
	}
	if h.Kind != sim.ShotAnchor {
		t.Fatalf("the spear should be in the floor (kind %d)", h.Kind)
	}
	// a warden standing well clear of the line, off to one side near the turret
	b := r.Spawn(sim.Walker, 0, "")
	b.Pos = h.Pos.Add(out.Mul(0.8)).Add(side.Mul(0.5))
	b.State, b.Wait, b.Path = sim.RWait, 100, nil
	tt.frames(5)
	if !b.Alive() {
		t.Fatal("the warden fried before the line moved")
	}
	// swing the gun round toward it: the muzzle end of the line sweeps across
	piv, _ = tt.g.harpoonFrame()
	holdTurret(tt, fv(h.Pos.Add(out.Mul(0.2)).Add(side.Mul(2)), 1.0).sub(piv.o).norm())
	tt.frames(5)
	if b.Alive() {
		t.Fatal("the line swept through the warden and it lived")
	}
}

// Take hold of a line stuck in the floor: it unhooks from the gun and goes dead in your
// hand. Let go away from the cleat and it's gone.
func TestUnhookedLineDropsIfNotTied(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	h := armTurret(tt)
	out := h.Pos.Sub(sim.Centroid(r.Room)).Norm()
	piv, _ := tt.g.harpoonFrame()
	holdTurret(tt, fv(h.Pos.Add(out.Mul(3)), 0).sub(piv.o).norm())
	tt.bits(1, true, false)
	tt.frames(1)
	tt.bits(1, false, false)
	tt.hs[1].trig = 0
	for k := 0; k < 120 && h.Kind == sim.ShotFlying; k++ {
		tt.frames(1)
	}
	tt.release(0)
	tt.release(1)
	_, muzzle := tt.g.harpoonFrame()
	along := v3s(h.Tip).sub(muzzle).norm()
	at := muzzle.add(along.mul(0.3))
	tt.pose(1, at.add(v3(0, 0.1, 0)), along, v3(0, 1, 0))
	tt.frames(1)
	tt.grabAt(1, at)
	if h.Kind != sim.ShotLoose {
		t.Fatalf("grabbing the line should unhook it from the gun (kind %d)", h.Kind)
	}
	tt.move(1, hands[1].palm.add(v3(0.3, 0.2, 0)), 10)
	tt.release(1)
	if h.Kind != sim.ShotNone || len(h.Traps) != 0 {
		t.Fatalf("let go untied, the line should be gone (kind %d, traps %d)", h.Kind, len(h.Traps))
	}
}

// A live line strung through a magenta wall keeps the wall supercharged - for a minute,
// then the whole line burns out.
func TestLineThroughAWallChargesItForAMinute(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	h := armTurret(tt)
	out := h.Pos.Sub(sim.Centroid(r.Room)).Norm()
	side := sim.V(-out.Z, out.X)
	// a magenta wall across the line's path, out in the hall
	mid := h.Pos.Add(out.Mul(2.6))
	id, why := r.PlaceWall(mid.Sub(side.Mul(1)), mid.Add(side.Mul(1)))
	if why != "" {
		t.Fatal(why)
	}
	piv, _ := tt.g.harpoonFrame()
	holdTurret(tt, fv(h.Pos.Add(out.Mul(4)), 0).sub(piv.o).norm())
	tt.bits(1, true, false)
	tt.frames(1)
	tt.bits(1, false, false)
	tt.hs[1].trig = 0
	for k := 0; k < 120 && h.Kind == sim.ShotFlying; k++ {
		tt.frames(1)
	}
	if h.Kind != sim.ShotAnchor {
		t.Fatalf("the spear should be in the floor beyond the wall (kind %d)", h.Kind)
	}
	tt.frames(60 * 20)
	if w := r.Wall(id); w == nil || w.Super <= 0 {
		t.Fatal("twenty seconds on, the line should be keeping the wall supercharged")
	}
	tt.frames(60 * 45)
	if h.Kind != sim.ShotNone {
		t.Fatal("after a minute charging the wall the line should have burned out")
	}
	tt.frames(60)
	if w := r.Wall(id); w == nil || w.Super > 0 {
		t.Fatal("with the line gone the wall should cool down")
	}
}

// A plain spear into the stolen cart sets off a mini EMP: the warden hauling it lets go,
// the ones round it stumble about for a good while, and you wind the cart home.
func TestSpearInTheCartIsAMiniEMP(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	h := armTurret(tt)
	d := &r.Dev
	out := h.Pos.Sub(sim.Centroid(r.Room)).Norm()
	side := sim.V(-out.Z, out.X)
	d.Pos = h.Pos.Add(out.Mul(3.4))
	carrier := r.Spawn(sim.Walker, 0, "")
	carrier.Pos = d.Pos.Add(out.Mul(0.5))
	carrier.State, carrier.Path = sim.RCarry, []sim.V2{r.Doors[0].Mouth}
	cs := r.T.CarrySpeed
	r.T.CarrySpeed = 0.01
	defer func() { r.T.CarrySpeed = cs }()
	d.State, d.Carrier, d.Held = sim.DevStolen, carrier.ID, false
	near := r.Spawn(sim.Walker, 0, "")
	near.Pos = d.Pos.Add(side.Mul(1.8))
	near.State, near.Wait, near.Path = sim.RWait, 0.5, nil
	tt.frames(1)
	piv, _ := tt.g.harpoonFrame()
	holdTurret(tt, fv(d.Pos, 0.5).sub(piv.o).norm())
	tt.bits(1, true, false)
	tt.frames(1)
	tt.bits(1, false, false)
	tt.hs[1].trig = 0
	for k := 0; k < 60 && h.Kind == sim.ShotFlying; k++ {
		tt.frames(1)
	}
	if h.Kind != sim.ShotDevice {
		t.Fatalf("the spear should be in the cart (kind %d)", h.Kind)
	}
	if !carrier.Alive() || d.State != sim.DevDropped {
		t.Fatalf("the warden hauling it should let go and live (alive %v, cart state %d)", carrier.Alive(), d.State)
	}
	if near.Dazed < 10 {
		t.Fatalf("the wardens round it should be dazed for a good while (%.1f s)", near.Dazed)
	}
	p0 := near.Pos
	tt.release(0)
	tt.release(1)
	tt.frames(60 * 3)
	if sim.Dist(p0, near.Pos) < 0.2 {
		t.Fatal("a dazed warden should stumble about, not stand still")
	}
	if near.State != sim.RWait {
		t.Fatal("a dazed warden went back to work")
	}
	k := tt.g.reel
	at := k.handle()
	tt.pose(1, at.add(k.ax.mul(0.15)), k.ax.mul(-1), v3(0, 1, 0))
	tt.frames(1)
	tt.grabAt(1, at)
	for turn := 0; turn < 12 && d.State != sim.DevHome; turn++ {
		tt.circle(1, k.c, k.ax, k.ref, k.R, 1, 40, k.ang)
	}
	if d.State != sim.DevHome {
		t.Fatalf("wound in, the cart should be home (state %d)", d.State)
	}
}

// The compartment door's held shut by two bolts (when the plate's screwed): it doesn't open
// until the wrench has them out.
func TestSerialDoorBoltedShut(t *testing.T) {
	tt := newTester(t, true)
	tt.toModule("serial", 0)
	m := tt.mod().(*modSerial)
	tt.frames(60)
	if m.fs == nil || m.fs.kind != fastBolt {
		t.Fatal("with the plate screwed, the compartment should be bolted")
	}
	if m.open > 0 {
		t.Fatal("the door opened with its bolts still in")
	}
	tt.fetchWrench()
	for i := range m.fs.f {
		tt.driveBolt(m.fs, i)
		if !m.fs.f[i].out {
			t.Fatalf("bolt %d still in (%.2f turns)", i, m.fs.f[i].ang/(2*math.Pi))
		}
	}
	tt.frames(90)
	if m.open < 0.9 {
		t.Fatal("with its bolts out the door should swing open")
	}
}

// Another round the plate's the one that's bolted.
func TestPlateCanBeBolted(t *testing.T) {
	tt := newTester(t, true)
	tt.g.boltedPlate = true
	r := tt.g.r
	tt.toModule("screws", 0)
	m := tt.mod().(*modScrews)
	m.start(tt.g)
	tt.frames(2)
	if m.fs.kind != fastBolt {
		t.Fatal("the plate should be bolted this round")
	}
	// the screwdriver does nothing to a bolt
	tt.fetchDriver()
	tt.driveScrew(m, 0)
	if m.fs.f[0].ang > 0 {
		t.Fatal("the screwdriver turned a bolt")
	}
	tt.release(1)
	tt.g.driver.p.held = false
	tt.fetchWrench()
	for i := range m.fs.f {
		tt.driveBolt(m.fs, i)
	}
	tt.frames(40)
	if !r.Dev.Mods[0].Done {
		t.Fatal("bolts out, the plate didn't come off")
	}
}

// driveFast: the driver (right hand) into screw i of set fs - any facing - backed out a
// wrist-turn at a time, lifted off and set back on between turns.
func (tt *tester) driveFast(fs *fastSet, i int) {
	_, n, tan := fs.place(i)
	reach := drvHandle/2 + drvShaft + 0.008
	head := fs.headPos(i)
	tt.pose(1, head.add(n.mul(reach+0.12)), n.mul(-1), tan)
	tt.frames(1)
	tt.move(1, head.add(n.mul(reach)), 6)
	for k := 0; k < 12 && !fs.f[i].out; k++ {
		tt.twist(1, n, 2.4, 10)
		tt.move(1, hands[1].palm.add(n.mul(0.2)), 4)
		tt.twist(1, n, -2.4, 4)
		head = fs.headPos(i)
		tt.move(1, head.add(n.mul(reach)), 4)
	}
}

// The wrench drops onto a generator's cover bolt held the way you'd reach for it - tilted in
// toward the panel, not lying dead flat - and once on, its handle runs back to your hand.
func TestWrenchSeatsTiltedOnPanel(t *testing.T) {
	tt := newTester(t, true)
	r := tt.g.r
	gp := tt.g.panels_[0]
	gen := &r.Gens[gp.i]
	gen.State, gen.Charge, gen.Order, gen.Done = sim.GenBlown, 0, []string{"bolts"}, 0
	tt.frames(2)
	fs := gp.cover
	if fs == nil || fs.kind != fastBolt {
		t.Fatal("no bolted cover")
	}
	_, n, tan := fs.place(0)
	head := fs.headPos(0)
	// the cover's on the panel's face, not hanging off below it
	if d := head.sub(gp.F.p(0, 1.12, 0.045)); math.Abs(d.dot(gp.F.y)) > 0.42 || math.Abs(d.dot(gp.F.x)) > 0.17 || math.Abs(d.dot(gp.F.z)) > 0.03 {
		t.Fatalf("cover bolt off the panel: %v from its middle", d)
	}
	tt.fetchWrench()
	fwd := tan.mul(math.Cos(0.8)).sub(n.mul(math.Sin(0.8))).norm() // ~45 degrees into the panel
	tt.pose(1, head.sub(fwd.mul(wrenchLen)).add(n.mul(0.12)), fwd, n)
	tt.frames(1)
	tt.move(1, head.sub(fwd.mul(wrenchLen)).add(n.mul(0.01)), 5)
	tt.frames(2)
	w := tt.g.wrench
	if fs.eng != 0 || !w.seated {
		t.Fatalf("wrench didn't drop on (eng %d)", fs.eng)
	}
	if toHand := projPerp(hands[1].palm.sub(head), n).norm(); w.seatDir.dot(toHand) < 0.9 {
		t.Fatalf("seated handle points %.2f away from the hand", w.seatDir.dot(toHand))
	}
	// a swing counter-clockwise loosens it
	for s := 1; s <= 10; s++ {
		tt.placePalm(1, head.add(rotAbout(fwd.mul(-1), n, 1.2*float64(s)/10).mul(wrenchLen)))
		tt.frames(1)
	}
	if fs.f[0].loose <= 0 {
		t.Fatalf("swinging the tilted wrench did nothing (eng %d)", fs.eng)
	}
}

// Some blown generators' work orders have a scorched cover to get off - screwed or bolted.
func TestGeneratorCoverWorkOrders(t *testing.T) {
	for _, task := range []string{"screws", "bolts"} {
		tt := newTester(t, true)
		r := tt.g.r
		gp := tt.g.panels_[0]
		gen := &r.Gens[gp.i]
		gen.State, gen.Charge, gen.Order, gen.Done = sim.GenBlown, 0, []string{task}, 0
		tt.frames(2)
		if gp.cover == nil {
			t.Fatalf("%s: no cover on the panel", task)
		}
		if task == "screws" {
			tt.fetchDriver()
			for i := range gp.cover.f {
				tt.driveFast(gp.cover, i)
			}
		} else {
			tt.fetchWrench()
			for i := range gp.cover.f {
				tt.driveBolt(gp.cover, i)
			}
		}
		tt.frames(2)
		if gen.State != sim.GenUp {
			t.Fatalf("%s: the cover should be off and the generator back up (state %d, cover %v)", task, gen.State, gp.cover != nil)
		}
	}
}

// Presses register: a quick jab that's well through the button by the next frame, one that
// comes in at an angle and ends up off its edge, and one that comes in from the side just
// after hovering in front of it.
func TestButtonPressesRegister(t *testing.T) {
	tt := newTester(t, true)
	tt.toModule("serial", 0)
	m := tt.mod().(*modSerial)
	tt.frames(2)
	b := m.keys[4]
	up := v3(0, 1, 0)
	side := b.n.cross(up).norm()
	try := func(why string, path []vec3) {
		before := len(m.typed)
		for _, p := range path {
			tt.poseTip(0, p, b.n.mul(-1), up)
			tt.frames(1)
		}
		tt.poseTip(0, b.c.add(b.n.mul(0.08)), b.n.mul(-1), up)
		tt.frames(8)
		if len(m.typed) != before+1 {
			t.Errorf("%s: the press didn't register", why)
		}
		m.typed = ""
	}
	try("a fast jab", []vec3{b.c.add(b.n.mul(0.05)), b.c.sub(b.n.mul(0.03))})
	try("at an angle", []vec3{b.c.add(b.n.mul(0.03)).sub(side.mul(0.02)), b.c.sub(b.n.mul(0.02)).add(side.mul(0.03))})
	try("hover then in", []vec3{b.c.add(b.n.mul(0.015)), b.c.add(b.n.mul(0.012)), b.c.sub(b.n.mul(0.01))})
}

// Started with the headset on the desk, some runtimes put the floor too low and you're
// eight feet tall: a head that high for a second gets the floor put back; and SET FLOOR
// takes a controller on the floor.
func TestFloorPutBackUnderYourFeet(t *testing.T) {
	tt := newTester(t, true)
	defer func() { floorOff = 0 }()
	floorOff = 0
	for k := 0; k < 80; k++ {
		tt.setHead(v3(0.001*float64(k%7), 2.6, 0.5), v3(0, 0, -1))
		tt.frames(1)
	}
	if math.Abs(floorOff-(2.6-eyeGuess)) > 0.01 {
		t.Fatalf("an eight-foot head should move the floor up %.2f m, moved %.2f", 2.6-eyeGuess, floorOff)
	}
	floorOff = 0
	// lying still on a desk at that height (it isn't on anyone's head) doesn't count
	for k := 0; k < 80; k++ {
		tt.setHead(v3(0, 2.6, 0.5), v3(0, 0, -1))
		tt.frames(1)
	}
	if floorOff != 0 {
		t.Fatal("a headset lying still moved the floor")
	}
	tt.setHead(v3(0, 1.6, 0.5), v3(0, 0, -1))
	// SET FLOOR: a controller lying on the real floor, 30 cm up as the runtime sees it
	tt.g.startSetFloor()
	tt.pose(1, v3(0.3, 0.3, 0), v3(0, 0, -1), v3(0, 1, 0))
	tt.frames(1)
	tt.bits(1, true, false)
	tt.frames(1)
	tt.bits(1, false, false)
	tt.hs[1].trig = 0
	if tt.g.settingFloor || math.Abs(floorOff-(tt.hs[1].m.pos().y-padOnFlr)) > 0.02 {
		t.Fatalf("the controller on the floor should set it (offset %.2f)", floorOff)
	}
}

// Tracing your room by hand at the setup console: corners on the floor, back to the first to
// close it, and the game rebuilds in that room.
func TestTraceRoom(t *testing.T) {
	tt := newTester(t, false)
	defer func() { roomSource = "headset" }()
	g := tt.g
	g.traceToggle()
	if !g.trace.on {
		t.Fatal("tracing didn't start")
	}
	corners := []pt2{{-1.2, -1.0}, {1.3, -1.1}, {1.4, 1.2}, {-0.2, 1.6}, {-1.3, 0.9}, {-1.18, -0.95}}
	o := v3(0.1, 1.4, 0.3)
	for i, c := range corners {
		d := v3(c[0], 0, c[1]).sub(o).norm()
		tt.pose(1, o.sub(d.mul(0.05)), d, v3(0, 1, 0))
		tt.hs[1].trig = 0.5
		tt.frames(2)
		tt.bits(1, true, false)
		tt.frames(1)
		tt.bits(1, false, false)
		tt.hs[1].trig = 0
		tt.frames(1)
		if i < 5 && len(G.trace.pts) != i+1 && G == g {
			t.Fatalf("corner %d didn't go down (%d corners, hit %d)", i, len(g.trace.pts), hands[1].hit.kind)
		}
	}
	if G == g || roomSource != "traced" {
		t.Fatal("closing the loop should rebuild the game in the traced room")
	}
	if a := math.Abs(G.r.Area); a < 6 || a > 9 {
		t.Fatalf("traced room area %.1f m2", a)
	}
}
