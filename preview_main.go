//go:build !windows

package main

// The Linux preview: plays a round with the bot (or a scripted scene), builds one frame
// from a chosen camera and dumps the draw batches; preview/render.py renders them with the
// real shaders. Usage:
//   prevbin "eyeX,eyeY,eyeZ,lookX,lookY,lookZ" seconds
// Environment: SEED, ROOM ("chap" for the captured boundary, or "w,d" in feet), GENS,
// MAZE (fraction of points the bot spends on the maze), SCENE (scripted set-ups).

import (
	"boundaryvr/sim"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"time"
)

func previewRoom() []pt2 {
	spec := os.Getenv("ROOM")
	if spec == "" || spec == "chap" {
		vb, err := os.ReadFile("../testdata/chaperone_info.vrchap")
		if err == nil {
			var doc map[string]any
			json.Unmarshal(vb, &doc)
			if u := findUniverse(doc, "31"); u != nil {
				P, _ := boundsOf(u)
				tx, tz, yaw := standingXZYaw(u)
				st := &store{tx: tx, tz: tz, yaw: yaw, standY: standingY(u)}
				return st.toStageAll(P)
			}
		}
		spec = "10,10"
	}
	var w, d float64
	fmt.Sscanf(spec, "%f,%f", &w, &d)
	w, d = w/3.28084, d/3.28084
	return []pt2{{-w / 2, -d / 2}, {w / 2, -d / 2}, {w / 2, d / 2}, {-w / 2, d / 2}}
}

func writeBatch(name string, v []float32) {
	f, _ := os.Create("dump/" + name + ".bin")
	binary.Write(f, binary.LittleEndian, v)
	f.Close()
}

func setCam(eye, look vec3) mat4 {
	view := lookAt4(eye, look, v3(0, 1, 0))
	iv := inv4(view)
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			ed.hmd[r][c] = float32(iv[c*4+r])
		}
		ed.hmd[r][3] = float32(iv[12+r])
	}
	return view
}

func main() {
	seed := int64(7)
	if v := os.Getenv("SEED"); v != "" {
		fmt.Sscan(v, &seed)
	}
	rand.Seed(seed)
	simClockOn = true
	room := previewRoom()
	G = newGame(room, seed)
	if v := os.Getenv("GENS"); v != "" {
		var n int
		fmt.Sscan(v, &n)
		if G.r.SetGens(n) {
			G.syncGens()
		}
	}
	if v := os.Getenv("GENTASK"); v != "" {
		// every generator blown, stuck on this repair step, for a look at its fixtures
		for i := range G.r.Gens {
			G.r.Gens[i].State, G.r.Gens[i].Order, G.r.Gens[i].Done = sim.GenBlown, []string{v}, 0
		}
	}
	maze := 0.8
	if v := os.Getenv("MAZE"); v != "" {
		fmt.Sscan(v, &maze)
	}
	bot := sim.NewBot(G.r, seed)
	bot.BuildMaze(maze)
	wallAt := os.Getenv("WALLAT") // "ax,az,bx,bz": a wall drawn from the right hand at the start
	G.auto = bot
	if os.Getenv("NOSTART") == "" {
		G.r.Start()
	}
	var cx, cy, cz, lx, ly, lz float64
	fmt.Sscanf(os.Args[1], "%f,%f,%f,%f,%f,%f", &cx, &cy, &cz, &lx, &ly, &lz)
	secs := 30.0
	if len(os.Args) > 2 {
		fmt.Sscan(os.Args[2], &secs)
	}
	eye, look := v3(cx, cy, cz), v3(lx, ly, lz)
	view := setCam(eye, look)
	dt := 1.0 / 45
	frames := int(secs / dt)
	t0 := time.Now()
	var tb time.Duration
	nb := 0
	for f := 0; f < frames; f++ {
		simClock += dt
		frameDT = dt
		if f == 0 && wallAt != "" {
			var ax, az, bx, bz float64
			fmt.Sscanf(wallAt, "%f,%f,%f,%f", &ax, &az, &bx, &bz)
			if _, why := G.r.PlaceWall(sim.V(ax, az), sim.V(bx, bz)); why != "" {
				fmt.Fprintln(os.Stderr, "wall:", why)
			}
			fwd := look.sub(eye).norm()
			G.wallFrom = eye.add(fwd.mul(0.42)).add(fwd.cross(v3(0, 1, 0)).norm().mul(0.16)).add(v3(0, -0.2, 0))
			G.wallFromT = fxTime()
		}
		G.lampFrame()
		G.panelFrame(dt)
		G.radioFrame(dt)
		G.harpoonTick(dt)
		G.step(dt)
		if f >= frames-60 || f%6 == 0 {
			t1 := time.Now()
			G.buildScene()
			tb += time.Since(t1)
			nb++
		} else {
			updateParticles(dt)
		}
		if os.Getenv("STOPAT") != "" && G.r.Phase != sim.Playing && f < frames-60 {
			frames = f + 60
		}
	}
	if v := os.Getenv("MODULE"); v != "" {
		// jump the device to a module (by id) and open it, for a look at it
		G.auto = nil
		isGame := false
		for _, gm := range G.r.C.Games {
			isGame = isGame || gm == v
		}
		if isGame {
			for i := range G.r.Dev.Mods {
				if G.r.Dev.Mods[i].ID == "game" {
					G.r.Dev.Mods[i].Game = v
				}
			}
			v = "game"
		}
		for G.r.Dev.Cur < len(G.r.Dev.Mods) && G.r.Dev.Mods[G.r.Dev.Cur].ID != v {
			G.r.CompleteModule()
		}

		for k := 0; k < 150; k++ {
			simClock += dt
			frameDT = dt
			G.deviceFrame(dt)
			G.step(dt)
			if k == 10 && os.Getenv("ENGINE") != "" {
				// the engine running and the big red button lit
				for _, dm := range G.devVis.mods {
					if rm, ok := dm.(*modRip); ok {
						rm.running, rm.revT = true, 10
					}
				}
			}
		}
	}
	if v := os.Getenv("MISFIRE"); v != "" {
		// the cord misfired: the hatch off and the guts up (MISFIRE=shut: still bolted down;
		// MISFIRE=burnt: a card to find with the flashlight)
		G.auto = nil
		for G.r.Dev.Cur < len(G.r.Dev.Mods)-1 {
			G.r.CompleteModule()
		}
		G.r.T.Misfire = 1
		G.r.SetLeads(2)
		G.r.CompleteModule()
		k := G.misfire
		G.step(dt) // the misfire itself
		if v != "shut" {
			for _, f := range k.fs.f {
				f.out = true
			}
			k.lift = 1
		}
		for n := 0; n < 200 && v == "burnt" && !(len(k.orders) > 0 && k.orders[0].kind == ordCard); n++ {
			k.generate(1)
		}
		for f := 0; f < 450; f++ {
			simClock += dt
			frameDT = dt
			G.deviceFrame(dt)
			G.step(dt)
			if f%6 == 0 {
				updateParticles(dt * 6)
			}
		}
		for _, c := range k.parts {
			if c.faulty {
				p := c.p.pos
				fmt.Fprintf(os.Stderr, "faulty part %d at %.3f,%.3f,%.3f\n", c.kind, p.x, p.y, p.z)
			}
		}
		fmt.Fprintf(os.Stderr, "misfire %v lift %.2f orders %d: %v\n", G.r.Dev.Misfire, k.lift, len(k.orders), k.orders)
	}
	if os.Getenv("LEECH") != "" {
		// a leech hovering over a generator, tethered on
		G.auto = nil
		b := G.r.Spawn(sim.Drone, 0, "leech")
		for k := 0; k < 45*60 && !(b.State == sim.RAttached && b.Wait > 0.3); k++ {
			simClock += dt
			frameDT = dt
			G.step(dt)
		}
		if os.Getenv("LEECH") == "off" {
			b.Wait = 1
			G.r.Unhook(b.ID)
			for k := 0; k < 30; k++ {
				simClock += dt
				frameDT = dt
				G.step(dt)
			}
		}
		fmt.Fprintf(os.Stderr, "leech state %d gen %d at %.2f,%.2f\n", b.State, b.Gen, b.Pos.X, b.Pos.Z)
	}
	if v := os.Getenv("SERIALSPOT"); v != "" {
		for _, m := range G.devVis.mods {
			if ms, ok := m.(*modSerial); ok {
				fmt.Sscan(v, &ms.spot)
			}
		}
	}
	if os.Getenv("FLASH") != "" {
		// the flashlight in hand, shining from the camera at the serial slot
		F := G.devFrame()
		at := F.p(devHW-0.06, serialY, serialZ)
		f := G.devVis.flash
		f.held = true
		f.pos = at.add(F.x.mul(0.35)).add(F.z.mul(-0.25)).add(v3(0, 0.15, 0))
		f.Z = f.pos.sub(at).norm()
		f.X = v3(0, 1, 0).cross(f.Z).norm()
		f.Y = f.Z.cross(f.X)
	}
	fmt.Fprintf(os.Stderr, "device at %.2f,%.2f box %.2f,%.2f frame x %.2f,%.2f,%.2f z %.2f,%.2f,%.2f\n", G.r.Dev.Pos.X, G.r.Dev.Pos.Z, G.r.Dev.LeadBox.X, G.r.Dev.LeadBox.Z, G.devFrame().x.x, G.devFrame().x.y, G.devFrame().x.z, G.devFrame().z.x, G.devFrame().z.y, G.devFrame().z.z)
	if os.Getenv("FIELD") != "" {
		// knock out two neighbouring generators so the ring shrinks off the device
		G.auto = nil
		for i := range G.r.Gens {
			if G.r.Dev.Exposed {
				break
			}
			G.r.SetPower(i, false)
			for k := 0; k < 3; k++ {
				simClock += dt
				frameDT = dt
				G.step(dt)
			}
		}
		for k := 0; k < 40; k++ {
			simClock += dt
			frameDT = dt
			G.step(dt)
		}
	}
	if v := os.Getenv("BREACH"); v != "" {
		// a warden charging a magenta wall with its breacher out: "x,z" where it stands
		var x, z float64
		fmt.Sscanf(v, "%f,%f", &x, &z)
		b := G.r.Spawn(sim.Walker, 0, "")
		b.Pos, b.Burn, b.State = sim.V(x, z), true, sim.RWalk
		b.Path = []sim.V2{sim.V(x, z+3)}
		b.Yaw = math.Pi / 2
		b.Moving = 1
	}
	if os.Getenv("SUPERWALL") != "" {
		// every magenta wall supercharged this instant (anything near it gets the blast)
		G.r.Phase = sim.Playing
		for _, w := range G.r.Maze {
			w.Super = 5
		}
		G.superPulse = append(G.superPulse, superPulse{t: G.t - 0.2})
		for k := 0; k < 3; k++ {
			simClock += dt
			frameDT = dt
			G.step(dt)
		}
	}
	if v := os.Getenv("RETRACT"); v != "" {
		// every magenta wall part way through being taken down
		var k float64
		fmt.Sscan(v, &k)
		G.retract = map[int]float64{}
		for _, w := range G.r.Maze {
			G.retract[w.ID] = k
		}
	}
	if os.Getenv("SUPERHARP") != "" && G.r.Harp.Up {
		G.r.SuperCD = 0
		G.r.SuperHarpoon()
		G.harpUp = 1
	}
	if os.Getenv("FIRE") != "" && G.r.Harp.Armed {
		// shoot the nearest warden (or the floor out in the hall) and let the line settle
		_, muzzle := G.harpoonFrame()
		tgt := fv(G.r.Harp.Pos.Add(G.r.Harp.Pos.Sub(sim.Centroid(G.r.Room)).Norm().Mul(4)), 0)
		bd := math.Inf(1)
		for _, b := range G.r.Robots {
			if b.Alive() && b.Kind == sim.Walker {
				if d := sim.Dist(b.Pos, G.r.Harp.Pos); d < bd && os.Getenv("FIRE") != "floor" {
					bd, tgt = d, fv(b.Pos, 1.0)
				}
			}
		}
		dir := tgt.sub(muzzle).norm()
		G.harpAim, G.harpUp = dir, 1
		_, muzzle = G.harpoonFrame()
		G.r.SetMuzzle(s3v(muzzle))
		G.r.Fire(s3v(muzzle), s3v(dir))
		for k := 0; k < 30; k++ {
			simClock += dt
			frameDT = dt
			G.harpoonTick(dt)
			G.step(dt)
		}
	}
	if G.r.Phase == sim.Won || G.r.Phase == sim.Lost {
		fwd := look.sub(eye)
		fwd.y = 0
		G.results.pos = eye.add(fwd.norm().mul(0.9)).add(v3(0, -0.15, 0))
		G.results.face(eye)
	}
	simClock += dt
	if os.Getenv("HANDS") != "" {
		previewHands(eye, look)
	}
	if os.Getenv("PULLLOCK") != "" && hands[1] != nil {
		// the right hand's arc locked onto the screwdriver
		G.driver.p.docked, G.driver.p.resting = false, true
		G.driver.p.pos = eye.add(look.sub(eye).norm().mul(1.6)).add(look.sub(eye).cross(v3(0, 1, 0)).norm().mul(0.5))
		G.driver.p.pos.y = G.driver.p.hy
		hands[1].fitGlove()
		hands[1].pull = pullSt{p: G.driver.p, phase: pullLocked}
	}
	if os.Getenv("FITMODE") != "" {
		// the glove-fitting view: lasers with their re-aim balls
		G.startFit()
		for _, gh := range hands {
			if gh.valid() {
				gh.fitGlove()
				gh.palm = gh.local(0, -0.005, -0.03)
			}
		}
	}
	armDebug = os.Getenv("ARMDBG") != ""
	if v := os.Getenv("ZONE"); v != "" {
		// ZoneTweak's own view of this room (the game put away); ZONE=menu opens its menu
		resetEngineForZone()
		ed.st = &store{uniID: "31", pts: clonePts(room), height: 2.4, playArea: [2]float64{2.4, 2.4}, unis: map[string]*uniInfo{}}
		ed.lib = spaceLib{dir: os.TempDir()}
		ed.prof = newPimaxProfile()
		ed.zone = ed.prof
		ed.loadFromStore()
		ed.buildPages()
		if v == "menu" {
			ed.openMenu()
		}
		for k := 0; k < 90; k++ {
			simClock += dt
			frameDT = dt
			ed.cur = ed.page.buttons()
			ed.buildScene()
		}
		for _, d := range zDrones {
			fmt.Fprintf(os.Stderr, "drone at %.2f,%.2f,%.2f\n", d.pos[0], d.h, d.pos[1])
		}
	} else {
		G.buildScene()
	}

	if v := os.Getenv("THIRD"); v != "" {
		// render from somewhere else than the head (to look at the arms from outside)
		var a, b vec3
		fmt.Sscanf(v, "%f,%f,%f,%f,%f,%f", &a.x, &a.y, &a.z, &b.x, &b.y, &b.z)
		view = lookAt4(a, b, v3(0, 1, 0))
		eye, cx, cy, cz = a, a.x, a.y, a.z
	}
	fmt.Fprintf(os.Stderr, "sim %.1fs in %v, buildScene avg %.2f ms, env verts %d, robots %d, phase %d wave %d work %.0f/%.0f\n",
		G.r.Time, time.Since(t0), float64(tb.Microseconds())/1000/float64(max(1, nb)), len(bEnv.v)/vstride, len(G.r.Robots), G.r.Phase, G.r.Wave, G.r.Work(), G.r.WorkTotal())
	for i, gp := range G.panels_ {
		if i < 2 {
			fmt.Fprintf(os.Stderr, "panel %d at %.2f,%.2f,%.2f facing %.2f,%.2f\n", gp.i, gp.F.o.x, gp.F.o.y, gp.F.o.z, gp.F.z.x, gp.F.z.z)
		}
	}
	if G.wrench != nil {
		w := G.wrench.p.pos
		fmt.Fprintf(os.Stderr, "wrench %s at %.2f,%.2f,%.2f\n", G.wrench.spot, w.x, w.y, w.z)
	}
	if G.pole != nil {
		_, hd := G.pole.ends()
		m := G.poleMount()
		fmt.Fprintf(os.Stderr, "pole head %.2f,%.2f,%.2f, bracket on generator %d at %.2f,%.2f\n", hd.x, hd.y, hd.z, m.gen, m.at.X, m.at.Z)
	}
	for _, d := range G.r.Doors {
		fmt.Fprintf(os.Stderr, "door %s wall %.2f,%.2f out %.2f,%.2f\n", d.Name, d.Wall.X, d.Wall.Z, d.Out.X, d.Out.Z)
	}
	h := G.r.Harp
	fmt.Fprintf(os.Stderr, "harpoon up %v at %.2f,%.2f armed %v powered %v shot %d tip %.2f,%.2f,%.2f; device %.2f,%.2f state %d module %d exposed %v field %.1f ring %d\n",
		h.Up, h.Pos.X, h.Pos.Z, h.Armed, h.Powered, h.Kind, h.Tip.X, h.Tip.Y, h.Tip.Z, G.r.Dev.Pos.X, G.r.Dev.Pos.Z, G.r.Dev.State, G.r.Dev.Cur, G.r.Dev.Exposed, G.r.Dev.Field, len(G.r.Ring))
	os.MkdirAll("dump", 0755)
	bGen.reset()
	for _, gp := range genParts {
		bGen.v = append(bGen.v, gp.v...)
	}
	for n, b := range map[string]*batch{"static": &bStatic, "env": &bEnv, "floorq": &bFloorQ, "gen": &bGen, "floors": &bFloorS, "glowtri": &bGlowTri,
		"glowline": &bGlowLine, "glowwide": &bGlowWide, "floor": &bFloor, "envl": &bEnvL, "opaque": &bOpaque, "lines": &bLines, "trans": &bTrans, "floorl": &bFloorL, "staticl": &bStaticL} {
		writeBatch(n, b.v)
	}
	for n, b := range map[string]*tbatch{"soft": &bSoft, "shadow": &bShadow, "smoke": &bSmoke, "fire": &bFire} {
		writeBatch(n, b.v)
	}
	lights, nl := packLights(eye, nil)
	var sp []map[string]any
	for _, s := range spots {
		sp = append(sp, map[string]any{"pos": []float64{s.pos.x, s.pos.y, s.pos.z}, "dir": []float64{s.dir.x, s.dir.y, s.dir.z}, "col": s.col, "cosOut": s.cosOut, "cosIn": s.cosIn, "rng": s.rng, "vp": s.vp})
	}
	sg := staticGlow()
	meta := map[string]any{"emisTint": []float64{sg.x, sg.y, sg.z}, "lights": lights, "nl": nl, "spots": sp, "eye": []float64{cx, cy, cz}, "view": view,
		"fo": []float64{floorFrame.o.x, floorFrame.o.y, floorFrame.o.z}, "fx": []float64{floorFrame.x.x, floorFrame.x.y, floorFrame.x.z}, "fz": []float64{floorFrame.z.x, floorFrame.z.y, floorFrame.z.z}, "time": fxTime()}
	jb, _ := json.Marshal(meta)
	os.WriteFile("dump/meta.json", jb, 0644)
	_ = math.Pi
}

// previewHands: both gloves in front of the camera, the right one half-pulling its trigger
// at the floor (AIM="x,z" picks the spot; default: where the camera looks).
func previewHands(eye, look vec3) {
	hs := []*hand{{idx: 0, grab: -1, hoverV: -1, hoverE: -1, btn: -1}, {idx: 1, grab: -1, hoverV: -1, hoverE: -1, btn: -1}}
	ed.hands = hs
	initHands(hs)
	fwd := look.sub(eye).norm()
	right := fwd.cross(v3(0, 1, 0)).norm()
	up := right.cross(fwd).norm()
	aim := v3(look.x, 0, look.z)
	if v := os.Getenv("AIM"); v != "" {
		fmt.Sscanf(v, "%f,%f", &aim.x, &aim.z)
	}
	set := func(h *hand, palm, f, u vec3) {
		Z := f.mul(-1).norm()
		Y := projPerp(u, Z).norm()
		X := Y.cross(Z).norm()
		put := func(m *hmdMatrix34, pos vec3) {
			for r, a := range [3]vec3{X, Y, Z} {
				m[0][r], m[1][r], m[2][r] = float32(a.x), float32(a.y), float32(a.z)
			}
			m[0][3], m[1][3], m[2][3] = float32(pos.x), float32(pos.y), float32(pos.z)
		}
		put(&h.m, palm.add(Y.mul(0.005)).add(Z.mul(0.03)))
		put(&h.aim, palm.add(f.norm().mul(0.05)))
		h.valid, h.aimValid = true, true
	}
	rp := eye.add(fwd.mul(0.42)).add(right.mul(0.16)).add(up.mul(-0.2))
	rf, ru := aim.sub(rp).norm(), v3(0, 1, 0)
	lvl := v3(fwd.x, 0, fwd.z).norm()
	lr := lvl.cross(v3(0, 1, 0)).norm()
	switch os.Getenv("POSE") {
	case "reach": // arm out in front, pointing ahead
		rp, rf, ru = eye.add(lvl.mul(0.55)).add(lr.mul(0.18)).add(v3(0, -0.25, 0)), lvl, v3(0, 1, 0)
	case "down": // pointing straight down at the cart in front of you
		rp, rf, ru = eye.add(lvl.mul(0.4)).add(lr.mul(0.12)).add(v3(0, -0.6, 0)), v3(0, -1, 0), lvl
	case "ear": // handset at your ear
		rp, rf, ru = eye.add(lr.mul(0.12)).add(lvl.mul(0.02)).add(v3(0, -0.04, 0)), v3(0, 1, 0).add(lvl.mul(0.3)).norm(), lr.mul(-1)
	case "up": // reaching up
		rp, rf, ru = eye.add(lvl.mul(0.2)).add(lr.mul(0.2)).add(v3(0, 0.25, 0)), v3(0, 1, 0).add(lvl.mul(0.4)).norm(), lvl.mul(-1)
	case "chest": // across your chest, pointing left
		rp, rf, ru = eye.add(lvl.mul(0.3)).add(lr.mul(-0.05)).add(v3(0, -0.4, 0)), lr.mul(-1), v3(0, 1, 0)
	case "side": // out to the right
		rp, rf, ru = eye.add(lr.mul(0.55)).add(v3(0, -0.35, 0)), lr, v3(0, 1, 0)
	case "palmup": // in front, palm up (as if holding something out)
		rp, rf, ru = eye.add(lvl.mul(0.4)).add(lr.mul(0.15)).add(v3(0, -0.45, 0)), lvl, lr.mul(-1)
	}
	set(hs[1], rp, rf, ru)
	hs[1].trig = 0.45
	lp := eye.add(fwd.mul(0.34)).add(right.mul(-0.17)).add(up.mul(-0.24))
	set(hs[0], lp, fwd.add(right.mul(0.5)).norm(), v3(0, 1, 0).add(right.mul(0.3)))
	hs[0].grip = 0.3
	for k := 0; k < 3; k++ {
		G.handFrame(1.0 / 45)
		G.laserFrame(1.0 / 45)
	}
	if os.Getenv("SCENE") == "driver" {
		previewDriver(hs)
	}
}

// previewDriver: the right hand holding the driver seated in the first screw, turned the
// way FEEL says (tight: the wrong way; break: just starting the right way; free: coming out).
func previewDriver(hs []*hand) {
	m, ok := G.devVis.mods[G.r.Dev.Cur].(*modScrews)
	k := G.driver
	if !ok || k == nil {
		return
	}
	F := G.devFrame()
	head := m.headPos(F, 0)
	reach := drvHandle/2 + drvShaft + 0.008
	dir := rotAbout(F.y.mul(-1), F.x, 0.35)
	palm := head.add(F.x.mul(0.02)).sub(dir.mul(reach))
	Z := dir.mul(-1)
	Y := projPerp(F.z.mul(-1), Z).norm()
	X := Y.cross(Z).norm()
	put := func(m *hmdMatrix34, pos vec3) {
		for r, a := range [3]vec3{X, Y, Z} {
			m[0][r], m[1][r], m[2][r] = float32(a.x), float32(a.y), float32(a.z)
		}
		m[0][3], m[1][3], m[2][3] = float32(pos.x), float32(pos.y), float32(pos.z)
	}
	gh := hands[1]
	hs[1].grip = 1
	turn := map[string]float64{"tight": -0.06, "break": 0.03, "free": 0.06}[os.Getenv("FEEL")]
	if os.Getenv("FEEL") == "free" {
		m.fs.f[0].loose = 1
	}
	for f := 0; f < 12; f++ {
		if f > 2 {
			X, Y = rotAbout(X, F.y, turn), rotAbout(Y, F.y, turn)
			Z = rotAbout(Z, F.y, turn)
		}
		put(&hs[1].m, palm.add(Y.mul(0.005)).add(Z.mul(0.03)))
		put(&hs[1].aim, palm.add(dir.mul(0.05)))
		gh.fitGlove()
		gh.update(1.0 / 45)
		if gh.held == nil {
			gh.held = k.p
			k.p.gGrab(gh)
		}
		k.p.gHold(gh, 1.0/45)
		simClock += 1.0 / 45
		frameDT = 1.0 / 45
		G.deviceFrame(1.0 / 45)
	}
}
