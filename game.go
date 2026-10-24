package main

// game: the glue between the rules (package sim) and the engine. It owns the round, keeps a
// visual for every warden and drone, turns rule events into sounds and effects, and builds
// the frame. Everything here is portable: the Windows VR build and the Linux preview share it.

import (
	"boundaryvr/sim"
	"math"
	"math/rand"
)

const genH = 2.3 // generator height (top of the energy blade)

var (
	cMagenta    = rgba{1.00, 0.24, 0.84, 1}
	cMagentaHot = rgba{1.00, 0.80, 0.97, 1}
	cWhiteHot   = rgba{0.92, 1.00, 0.96, 1}
	cDead       = rgba{0.22, 0.10, 0.08, 1}
	cOffAmber   = rgba{1.00, 0.55, 0.12, 1}
)

type banner struct {
	text  string
	col   rgba
	t0    float64
	dur   float64
	small string
}

type game struct {
	r            *sim.Round
	room         []pt2
	seed         int64
	bots         map[int]*robot
	gext         []float64 // generator floor blades deployed (0..1)
	gpow         []float64 // generator emitters powered (0..1)
	linkP        []int     // each generator's powered neighbour on either side (-1 if none)
	linkN        []int
	auto         *sim.Bot // autoplay (preview and attract mode)
	t            float64
	banners      []banner
	flash        float64      // whole-room flash (device firing)
	shockFX      []shockFX    // supercharged spear shocks going off
	settingFloor bool         // SET FLOOR: waiting for a controller on the floor
	superPulse   []superPulse // a wall just supercharged: its blast rolling out
	shake        float64
	lastWave     int
	harpAim      vec3 // the turret's barrel direction
	harpUp       float64
	harpN        int  // hands on the grips when the aim was last anchored (-1: re-anchor)
	harpV0       vec3 // pivot to hands at the anchor
	harpA0       vec3 // barrel direction at the anchor
	devVis       devVisual
	panels_      []*genPanel
	leeches      map[int]*leechHandle
	console      *uiPanel
	results      *uiPanel
	menu         *uiPanel
	menuOpen     bool
	radio        *radio
	grips        [2]*harpGrip
	lines        [2]*harpLine
	reel         *crank  // the harpoon's winch: crank a hooked warden in
	reelTurns    float64 // the winch's turns at the last frame
	timers       []timer
	driver       *driverKit             // the screwdriver, somewhere in your room
	wrench       *wrenchKit             // ...and the wrench
	leads        *leadKit               // the cart's power leads from the box on the floor
	misfire      *mfKit                 // the service hatch and the guts under it (a misfire)
	trace        traceKit               // tracing your room by hand, at the setup console
	boltedPlate  bool                   // this round the access plate's bolted (and the compartment screwed)
	pole         *poleKit               // the pole you smash leeches with
	mposts       map[[2]int32]*mazePost // the maze's base stations, by where they stand
	retract      map[int]float64        // magenta walls being taken down by the trigger (0..1)
	mpostSeq     int                    // ...the order they were asked for
	mpostLast    float64                // ...and when the last one started up
	wallFrom     vec3                   // where the last wall was drawn from (its pods fly out of there)
	wallFromT    float64
	fitting      bool // fitting the gloves (from the pause menu; the round waits)
	fitPanel     *uiPanel
	lightsOut    *lever  // the big lever on the setup console that starts the round
	lightsT      float64 // when it was thrown (game time)
}

var G *game

// gameTuning: the rules' numbers (nil: the built-in ones). The VR build loads an override
// file from beside the exe; the difficulty chosen on the console scales the waves on top.
var gameTuning *sim.Tuning

// difficulty: 0 easy, 1 normal, 2 hard (kept across rounds)
var difficulty = 1

var difficultyNames = [3]string{"EASY", "NORMAL", "HARD"}
var difficultyPressure = [3]float64{0.55, 1.0, 1.1}

func tuningFor() *sim.Tuning {
	var t sim.Tuning
	if gameTuning != nil {
		t = *gameTuning
	} else {
		t = *sim.DefaultTuning()
	}
	base := t.Pressure
	if base <= 0 {
		base = 1
	}
	t.Pressure = base * difficultyPressure[difficulty]
	return &t
}

func sv(p pt2) sim.V2             { return sim.V2{X: p[0], Z: p[1]} }
func pv(v sim.V2) pt2             { return pt2{v.X, v.Z} }
func v3s(v sim.V3) vec3           { return vec3{v.X, v.Y, v.Z} }
func s3v(v vec3) sim.V3           { return sim.V3{X: v.x, Y: v.y, Z: v.z} }
func fv(v sim.V2, y float64) vec3 { return vec3{v.X, y, v.Z} }

// harpExplained: your contact has talked you through the harpoon's tricks (once a session)
var harpExplained bool

// reelExplained: your contact has told you what the yellow light means (once a session)
var reelExplained bool

// cleanWins: rounds won in a row this session without a misfire
var cleanWins int

func newGame(room []pt2, seed int64) *game {
	g := &game{room: room, seed: seed, bots: map[int]*robot{}}
	radioLast = map[string]float64{}
	var R []sim.V2
	for _, p := range room {
		R = append(R, sv(p))
	}
	g.r = sim.NewRound(R, seed, tuningFor(), nil)
	// a win or two without a misfire makes one likelier, then certain
	g.r.T.Misfire = []float64{g.r.T.Misfire, math.Max(g.r.T.Misfire, 0.75), 1}[min(cleanWins, 2)]
	g.room = nil
	for _, p := range g.r.Room {
		g.room = append(g.room, pv(p))
	}
	g.setupEnv()
	g.syncGens()
	g.harpAim = v3(0, 0, -1)
	// which of the cart's covers is bolted (the other's screwed) changes from round to round
	g.boltedPlate = (uint64(seed)*2654435761>>13)&1 == 1
	g.init()
	return g
}

// setupEnv: the prison hall around your room, with a door in the middle of each wall.
func (g *game) setupEnv() {
	h := g.r.Hall
	envG = rigid2{a: h.F.X.X, b: h.F.Z.X, c: h.F.X.Z, d: h.F.Z.Z, tx: h.F.O.X, tz: h.F.O.Z}
	envBox = [4]float64{0, h.W, 0, h.H}
	envHave = true
	envDoorsOn = true
	rb := h.RoomBox
	cx, cz := (rb[0]+rb[1])/2, (rb[2]+rb[3])/2
	envDoorAt = [4]float64{cx, cz, cx, cz}
	envLampC = pt2{cx, cz}
	envH = 6.0
	ed.buildStatic(envG, 0, h.W, 0, h.H)
	staticVer++
	staticChanged(staticVer)
}

// syncGens: the engine draws generators at ed.pts.
func (g *game) syncGens() {
	n := len(g.r.Gens)
	ed.pts = make([]pt2, n)
	for i, gen := range g.r.Gens {
		ed.pts[i] = pv(gen.Pos)
	}
	ed.height = genH
	if len(g.gext) != n {
		g.gext, g.gpow = make([]float64, n), make([]float64, n)
		for i := range g.gext {
			g.gext[i], g.gpow[i] = 1, 1
		}
		bladeExt = map[int]float64{}
		bladeExtN = n
	}
	g.linkP, g.linkN = make([]int, n), make([]int, n)
	g.links()
}

func (g *game) links() {
	n := len(g.r.Gens)
	for i := range g.linkP {
		g.linkP[i], g.linkN[i] = -1, -1
	}
	for _, s := range g.r.Spans() {
		g.linkN[s[0]], g.linkP[s[1]] = s[1], s[0]
	}
	_ = n
}

// step: advance the rules and turn what happened into sounds and effects.
func (g *game) step(dt float64) {
	if g.auto != nil {
		g.auto.Step(dt)
	}
	if hp := ed.hmd.pos(); hp.y > 0.5 {
		g.r.Player, g.r.PlayerOK = sv(pt2{hp.x, hp.z}), true
	}
	g.r.Fixtures = g.r.Fixtures[:0]
	if g.radio != nil {
		g.r.Fixtures = append(g.r.Fixtures, g.radio.pos)
	}
	if g.pole != nil {
		hp, _, _, _ := g.pole.p.home()
		g.r.Fixtures = append(g.r.Fixtures, sim.V(hp.x, hp.z))
	}
	g.r.Fixtures = append(g.r.Fixtures, g.consoleSpot())
	g.r.Step(dt)
	g.t += dt
	for _, e := range g.r.TakeEvents() {
		g.onEvent(e)
	}
	g.links()
	g.flash = math.Max(0, g.flash-dt*1.5)
}

func (g *game) say(text, small string, col rgba, dur float64) {
	g.banners = append(g.banners, banner{text: text, small: small, col: col, t0: g.t, dur: dur})
	if len(g.banners) > 3 {
		g.banners = g.banners[len(g.banners)-3:]
	}
}

func (g *game) onEvent(e sim.Event) {
	r := g.r
	p := fv(e.Pos, 1.0)
	switch e.Kind {
	case sim.EvWave:
		g.say(fmtWave(e.A), "", cDanger, 2.5)
		for _, d := range r.Doors {
			ed.sfx(sndAlarm, fv(d.Wall, 2.5), 0.5)
			spawnSteam(fv(d.Wall, 2.6), fv(d.Out, 0).mul(-1).add(v3(0, -0.4, 0)).norm(), 30)
		}
	case sim.EvIntel:
		ed.sfx(sndTick, g.radioPos(), 0.6)
	case sim.EvSpawn:
		ed.sfx(sndClang, p, 0.5)
	case sim.EvGenBlown:
		top := fv(e.Pos, genH)
		spawnSparks(top, v3(0, 1, 0), 60, 1.2)
		spawnSmoke(top, 14)
		sparkBursts = append(sparkBursts, sparkBurst{p: top, t: fxTime(), k: 1})
		ed.sfx(sndBoom, top, 0.9)
		ed.sfx(sndPowerDown, top, 0.7)
		g.say("GENERATOR DOWN", "YOUR ROOM JUST SHRANK", cDanger, 2.5)
		radioSay("x_genblown", false)
	case sim.EvGenUp:
		ed.sfx(sndPowerUp, fv(e.Pos, 1.2), 0.7)
		spawnSparks(fv(e.Pos, 0.3), v3(0, 1, 0), 20, 0.6)
	case sim.EvGenOff:
		ed.sfx(sndPowerDown, fv(e.Pos, 1.2), 0.6)
	case sim.EvLeech:
		ed.sfx(sndClang, fv(e.Pos, 1.4), 0.8)
		spawnSparks(fv(e.Pos, 1.4), v3(0, 1, 0), 30, 0.7)
		g.say("LEECH ON A GENERATOR", "YANK ITS TETHER OFF - OR SMASH IT WITH THE POLE", cDanger, 2.5)
		radioSay("x_leech", false)
	case sim.EvPluck:
		ed.sfx(sndRip, fv(e.Pos, 1.4), 0.8)
	case sim.EvDied:
		hy := 1.2
		if e.B == int(sim.Drone) {
			hy = 1.45
		}
		q := fv(e.Pos, hy)
		spawnSparks(q, v3(0, 1, 0), 45, 1.0)
		spawnSmoke(q, 10)
		sparkBursts = append(sparkBursts, sparkBurst{p: q, t: fxTime(), k: 0.8})
		switch e.S {
		case "super", "wire", "green":
			ed.sfx(sndZap, q, 0.9)
		case "blast":
		default:
			ed.sfx(sndZap, q, 0.6)
		}
		ed.sfx(sndBoom, q, 0.35)
	case sim.EvBurn:
		// the breacher punching its hole: a white-hot burst where it goes through
		q := fv(e.Pos, 1.0)
		spawnSparks(q, v3(0, 1, 0), 90, 1.6)
		spawnSmoke(q, 8)
		ed.sfx(sndZap, q, 1)
		ed.sfx(sndRip, q, 0.8)
		ed.sfx(sndBoom, q, 0.5)
		g.shockFX = append(g.shockFX, shockFX{p: q, t: g.t, col: rgba{1, 0.6, 1, 1}})
	case sim.EvExposed:
		ed.sfx(sndAlarm, fv(e.Pos, 0.8), 0.7)
		g.say("THE DEVICE IS OUTSIDE YOUR LINE", "ITS FIELD WON'T HOLD LONG. DRAG IT BACK IN", cDanger, 3)
		ed.sfx(sndSurge, fv(e.Pos, 0.8), 0.6)
		radioSay("x_exposed", false)
	case sim.EvLeadsOut:
		g.unplugAll()
		if g.r.Dev.State == sim.DevHome {
			g.say("POWER LEADS PULLED OUT", "PLUG THEM BACK INTO THE CART - IT WON'T GO OFF WITHOUT THEM", cDanger, 3)
		}
		radioSay("x_leads", false)
	case sim.EvMisfire:
		p := fv(e.Pos, 1.1)
		if e.B == 0 {
			ed.sfx(sndPowerDown, p, 1)
			ed.sfx(sndBoom, p, 0.5)
			spawnSparks(p, v3(0, 1, 0), 80, 1.6)
			spawnSmoke(p, 12)
			if k := g.misfire; k != nil {
				// the first time one thing or two; again, two more
				k.generate(map[bool]int{true: 2, false: 0}[e.A > 1])
				k.raised = false
			}
			if e.A > 1 {
				g.say("MISFIRED AGAIN", "OPEN IT BACK UP - PICK UP THE RADIO", cDanger, 4)
				radioSay("x_mf_again", true)
			} else {
				g.say("MISFIRE", "IT DIDN'T GO OFF - PICK UP THE RADIO", cDanger, 4)
				radioSay("x_mf_start", true)
			}
		}
		for _, m := range g.devVis.mods {
			if rm, ok := m.(*modRip); ok {
				rm.good = 0
			}
		}
	case sim.EvFixed:
		ed.sfx(sndPowerUp, fv(e.Pos, 1.1), 0.9)
		g.say("IT'S READY AGAIN", "START IT UP - IT'S WARM, ONE GOOD PULL - THEN THE BUTTON", cEnergy, 3)
	case sim.EvNoPower:
		ed.sfx(sndGrind, fv(e.Pos, 0.9), 0.5)
		g.say("NO POWER", "PLUG BOTH LEADS INTO THE BACK OF THE CART", cDanger, 2.5)
	case sim.EvStolen:
		ed.sfx(sndAlarm, fv(e.Pos, 0.8), 1)
		sub := "GRAB IT, OR HARPOON IT"
		if g.r.Harp.Up && g.r.SuperCD <= 0 {
			sub = "SUPERCHARGE THE HARPOON (HOLD A ON IT) AND SPEAR THE CART"
		}
		g.say("THEY'VE GOT THE DEVICE", sub, cDanger, 3)
		radioSay("x_stolen", true)
	case sim.EvDropped:
		ed.sfx(sndClang, fv(e.Pos, 0.4), 0.8)
		if h := &g.r.Harp; h.Up && g.r.Dev.State == sim.DevDropped && !h.Armed {
			radioSay("x_tractor", true)
		}
		if h := &g.r.Harp; h.Up && g.r.Dev.State == sim.DevDropped {
			sub := "AIM THE HARPOON AT IT AND PULL THE TRIGGER: TRACTOR BEAM"
			if h.Armed {
				sub = "SPEAR IT WITH THE HARPOON AND WIND IT HOME"
			}
			g.say("IT'S ON THE FLOOR", sub, cBeam, 3)
		}
	case sim.EvLineFree:
		g.say("LINE UNHOOKED", "TIE IT TO THE GLOWING CLEAT ON THE GUN'S POST TO KEEP IT LIVE - OR LET GO", cMagenta, 3)
	case sim.EvLineDrop:
		ed.sfx(sndPowerDown, fv(e.Pos, 0.2), 0.5)
		spawnSparks(fv(e.Pos, 0.1), v3(0, 1, 0), 12, 0.4)
	case sim.EvLineGone:
		p := fv(e.Pos, 0.8)
		ed.sfx(sndZap, p, 0.9)
		ed.sfx(sndRip, p, 0.6)
		spawnSparks(p, v3(0, 1, 0), 60, 1.2)
		spawnSmoke(p, 6)
		g.say("A LINE BURNED OUT", "A MINUTE KEEPING A WALL CHARGED IS ALL IT HAS", cMagenta, 2.5)
	case sim.EvMiniEMP:
		p := fv(e.Pos, 0.7)
		ed.sfx(sndSurge, p, 0.9)
		ed.sfx(sndZap, p, 0.7)
		spawnSparks(p, v3(0, 1, 0), 60, 1.4)
		g.flash = math.Max(g.flash, 0.4)
		g.shockFX = append(g.shockFX, shockFX{p: p, t: g.t})
		g.say("MINI EMP", "THEY'RE STUMBLING ABOUT - WIND IT IN", cBeam, 2.5)
	case sim.EvTractor:
		ed.sfx(sndHydraulic, fv(e.Pos, 0.5), 0.6)
		g.say("TRACTOR BEAM", "IT'S COMING HOME", cBeam, 2)
	case sim.EvRecovered:
		ed.sfx(sndRelay, fv(e.Pos, 0.6), 0.8)
		if g.r.Dev.Leads < sim.LeadsNeeded {
			g.say("GOT IT BACK", "PLUG THE POWER LEADS BACK INTO IT", cEnergy, 3)
		} else {
			g.say("GOT IT BACK", "", cEnergy, 2)
		}
		radioSay("x_recovered", false)
		radioSay("x_leads", false)
	case sim.EvTugFail:
		ed.sfx(sndGrind, fv(e.Pos, 0.6), 0.6)
	case sim.EvLost:
		ed.sfx(sndAlarm, fv(e.Pos, 1), 1)
		g.say("THEY CARRIED IT OUT", "ROUND LOST", cDanger, 30)
		radioSay("x_lost", true)
	case sim.EvArmed:
		ed.sfx(sndPowerUp, fv(e.Pos, 1), 0.8)
		reel := e.S == "reel"
		switch {
		case e.B == 0 && reel:
			g.harpUp = 0 // it rises out of the floor
			g.say("HARPOON ARMED - REEL SPEARS", "YELLOW: HOOK ROBOTS (OR THE CART) TWICE - NO LIVE WIRE", cYellow, 4)
		case e.B == 0:
			g.harpUp = 0
			g.say("HARPOON ARMED", "IT'S UP IN YOUR ROOM", cAmber, 3)
		case e.B == 2:
			g.say("ONE MORE REEL", "YELLOW: HOOK ANOTHER ONE - NO LIVE WIRE UNLESS YOU SUPERCHARGE IT", cYellow, 3)
		case reel:
			g.say("HARPOON LOADED - REEL SPEARS", "YELLOW: HOOK ROBOTS (OR THE CART) TWICE - NO LIVE WIRE", cYellow, 3)
		default:
			g.say("HARPOON LOADED", "", cAmber, 2)
		}
		if reel && !reelExplained {
			reelExplained = true
			radioSay("x_reel_mode", false)
		}
		radioSay("x_armed", false)
		if !harpExplained {
			// the first time: your contact talks you through what it can do
			harpExplained = true
			radioSay("x_harp_super", false)
			radioSay("x_harp_tie", false)
			radioSay("x_cart_spear", false)
		}
	case sim.EvHarpSuper:
		p := fv(e.Pos, 1.1)
		ed.sfx(sndPowerUp, p, 1)
		ed.sfx(sndZap, p, 0.8)
		spawnSparks(p, v3(0, 1, 0), 50, 1.2)
		g.say("HARPOON SUPERCHARGED", "THE NEXT SPEAR SHOCKS WHATEVER IT HITS", cBeam, 2.5)
	case sim.EvShock:
		p := fv(e.Pos, 1.0)
		ed.sfx(sndZap, p, 1)
		ed.sfx(sndBoom, p, 0.7)
		spawnSparks(p, v3(0, 1, 0), 120, 2.2)
		spawnSmoke(p, 10)
		g.flash = math.Max(g.flash, 0.6)
		g.shockFX = append(g.shockFX, shockFX{p: p, t: g.t})
	case sim.EvFire:
		ed.sfx(sndZip, fv(e.Pos, 1.1), 1)
		ed.sfx(sndHydraulic, fv(e.Pos, 1.1), 0.6)
	case sim.EvHit:
		ed.sfx(sndClang, fv(e.Pos, 1), 0.8)
		spawnSparks(fv(e.Pos, 0.8), v3(0, 1, 0), 25, 0.8)
		if e.A == 2 {
			g.say("LIVE WIRE", "", cMagenta, 2)
		}
	case sim.EvDud:
		q := fv(e.Pos, 0.1)
		ed.sfx(sndClang, q, 0.6)
		spawnSparks(q, v3(0, 1, 0), 8, 0.4)
		g.say("NO WIRE", "A YELLOW SPEAR ONLY HOOKS - SUPERCHARGE IT FOR A LIVE WIRE", cYellow, 2.5)
	case sim.EvWireZap:
		q := fv(e.Pos, 1.0)
		spawnSparks(q, v3(0, 1, 0), 60, 1.2)
		ed.sfx(sndZap, q, 1)
	case sim.EvSuper:
		ed.sfx(sndSurge, fv(e.Pos, 1.2), 1)
		g.superPulse = append(g.superPulse, superPulse{t: g.t})
	case sim.EvModule:
		ed.sfx(sndRelay, fv(e.Pos, 0.9), 0.7)
		ed.sfx(sndPowerUp, fv(e.Pos, 0.9), 0.4)
	case sim.EvSetback:
		// a botched game starts over (as a different game); it never undoes a stage you'd done
		if d := &r.Dev; e.S != "tug" && d.Cur < len(d.Mods) && d.Mods[d.Cur].ID == "game" {
			g.devVis.lastCur = -1
		}
		switch e.S {
		case "simon":
			ed.sfx(sndAlarm, fv(e.Pos, 0.9), 0.5)
			g.say("WRONG BUTTON", "THE GAME STARTS OVER", cDanger, 2.5)
		case "needle":
			ed.sfx(sndAlarm, fv(e.Pos, 0.9), 0.5)
			g.say("THE NEEDLE LEFT THE GREEN", "THE GAME STARTS OVER", cDanger, 2.5)
		case "plug":
			ed.sfx(sndAlarm, fv(e.Pos, 0.9), 0.5)
			g.say("TOO MANY CALLS LOST", "THE GAME STARTS OVER", cDanger, 2.5)
		case "valves":
			ed.sfx(sndAlarm, fv(e.Pos, 0.9), 0.5)
			g.say("A GAUGE BLEW", "THE GAME STARTS OVER", cDanger, 2.5)
		case "tug":
			// wrenched back out of a warden's grip: "GOT IT BACK" says it
		default:
			ed.sfx(sndAlarm, fv(e.Pos, 0.9), 0.5)
			g.say("WRONG", "THE GAME STARTS OVER", cDanger, 2)
		}
	case sim.EvWon:
		g.flash = 1
		ed.sfx(sndBoom, fv(e.Pos, 1), 1)
		ed.sfx(sndBoom, fv(e.Pos, 2), 1)
		g.say("EMP FIRED", "YOU'RE OUT", cEnergy, 30)
		if g.r.Dev.Misfires == 0 {
			cleanWins++
		} else {
			cleanWins = 0
		}
		g.later(1.2, func() { radioSay("x_won", true) })
	case sim.EvKnock:
		ed.sfx(sndSurge, fv(e.Pos, 1.2), 0.8)
	case sim.EvZapInside:
		ed.sfx(sndZap, fv(e.Pos, 1.0), 0.8)
	case sim.EvGenLow:
		ed.sfx(sndBeep, fv(e.Pos, genH-0.3), 0.8)
		g.say("GENERATOR LOW", "PUMP IT OR THEY'LL BLOW IT", cAmber, 1.8)
		radioSay("x_genlow", false)
	case sim.EvRing:
		if e.A < 3 && r.Phase == sim.Playing {
			ed.sfx(sndPowerDown, fv(sim.Centroid(r.Room), 3), 1)
			g.say("YOUR RING IS DOWN", "THEY'RE COMING FOR THE DEVICE", cDanger, 3)
			radioSay("x_ringdown", true)
		}
	case sim.EvFieldDown:
		ed.sfx(sndPowerDown, fv(e.Pos, 0.8), 0.9)
		g.say("FIELD DOWN", "THEY CAN TAKE IT NOW", cDanger, 2.5)
	case sim.EvLeechBite:
		ed.sfx(sndPowerDown, fv(e.Pos, 1.4), 0.8)
		ed.sfx(sndZap, fv(e.Pos, 1.4), 0.7)
		g.say("THE LEECH BIT THROUGH", "GENERATOR SHUT DOWN. YANK THE TETHER OR SMASH IT", cDanger, 2.5)
	case sim.EvPump:
		ed.sfx(sndHydraulic, fv(e.Pos, 1.0), 0.35)
	case sim.EvTask:
		ed.sfx(sndRelay, fv(e.Pos, 1.0), 0.5)
	case sim.EvWallGone:
		ed.sfx(sndPowerDown, fv(e.Pos, 1.0), 0.4)
	}
}

func fmtWave(n int) string {
	return "WAVE " + itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// radioPos: where the radio stands (by the setup console's spot until it has one)
func (g *game) radioPos() vec3 {
	c := sim.Centroid(g.r.Room)
	return fv(c, 1.0)
}

// ---- the frame ----

func resetBatches() {
	bOpaque.reset()
	bLines.reset()
	bTrans.reset()
	for _, b := range []*batch{&bEnv, &bEnvL, &bFloor, &bFloorL, &bGlowTri, &bGlowLine, &bGlowWide} {
		b.reset()
	}
	bSoft.reset()
	bShadow.reset()
	bSmoke.reset()
	bFire.reset()
	frameLights = frameLights[:0]
	fogVols = fogVols[:0]
	seedTex(0)
}

func (g *game) buildScene() {
	resetBatches()
	cam := ed.hmd.pos()
	camPos, camR, camU = cam, ed.hmd.axis(0), ed.hmd.axis(1)
	ed.animateEnv()
	beginGenFrame()
	g.drawGenerators()
	g.drawMazePosts()
	endGenFrame()
	g.drawRing()
	g.drawMaze()
	g.drawDoorSigns()
	g.drawRoomFloor()
	g.drawPanels()
	g.drawRobots()
	g.drawDevice()
	g.drawLeads()
	g.drawMisfire()
	g.drawTrace()
	g.drawDriver()
	g.drawWrench()
	g.drawPole()
	g.drawPoleBracket()
	g.drawHarpoon()
	g.drawShocks()
	g.drawSuperReach()
	g.drawRadio()
	g.drawUI()
	g.drawHands()
	g.drawBanners()
	updateMist()
	updateParticles(frameDT)
	if g.flash > 0 {
		addPoint(fv(g.r.Dev.Pos, 1.5), cWhiteHot, 60*g.flash, 14)
	}
	_ = rand.Float64
}

func rngF() float64 { return rand.Float64() }

// angDiff2: the signed angle from a to b, wrapped to -pi..pi.
func angDiff2(a, b float64) float64 {
	d := math.Mod(b-a+math.Pi, 2*math.Pi)
	if d < 0 {
		d += 2 * math.Pi
	}
	return d - math.Pi
}

// lampFrame: the hall lamps. Throwing the lights-out lever kills them with a clunk; a moment
// later the emergency circuit brings them back dim and red for the rest of the round.
func (g *game) lampFrame() {
	white := rgba{1, 1, 1, 1}
	red := rgba{1, 0.42, 0.32, 1}
	switch g.r.Phase {
	case sim.Setup:
		envLampK, envLampTint = 1, white
		return
	case sim.Won:
		// the EMP took the robots out; the building's own power comes back
		envLampK += (1 - envLampK) * math.Min(1, frameDT*0.6)
		envLampTint = envLampTint.mix(white, float32(math.Min(1, frameDT*0.6)))
		return
	}
	t := g.t - g.lightsT
	switch {
	case t < 0.12:
		envLampK, envLampTint = 1-t/0.12, white
	case t < 1.7:
		envLampK = 0
		if math.Mod(t*13, 1) < 0.15 && t > 1.1 {
			envLampK = 0.25 // the emergency circuit catching
		}
		envLampTint = red
	case t < 2.6:
		envLampK, envLampTint = 0.5*smooth((t-1.7)/0.9), red
	default:
		envLampK, envLampTint = 0.5, red
	}
}
