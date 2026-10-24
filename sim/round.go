package sim

import (
	"fmt"
	"math"
	"math/rand"
)

// ---- state ----

type GenState int

const (
	GenUp      GenState = iota
	GenOff              // switched off with its breaker lever
	GenBlown            // drained flat: needs a work order
	GenLeeched          // a leech drone has it shut down
)

type Generator struct {
	Pos     V2
	State   GenState
	Charge  float64
	Low     bool     // below 30% (warned)
	Order   []string // work order (task ids) while blown
	Done    int      // tasks of the order finished
	Leech   int      // id of the drone clamped on it
	Pressed int      // wardens leaning on its spans
	// Lead: the first post of the machine this post belongs to. Corners of your room closer
	// together than the shortest span get a post each but share one generator - one
	// panel, one charge, one state - so the ring can follow your real walls exactly.
	// The lead post holds the machine's state; the others mirror it.
	Lead int
}

// lead: the machine post i belongs to (where its state lives).
func (r *Round) lead(i int) (int, *Generator) {
	l := r.Gens[i].Lead
	return l, &r.Gens[l]
}

// Machines: how many generators there are (posts that share one count once).
func (r *Round) Machines() int {
	n := 0
	for i, g := range r.Gens {
		if g.Lead == i {
			n++
		}
	}
	return n
}

// mirrorGens: every post of a machine shows its lead post's state.
func (r *Round) mirrorGens() {
	for i := range r.Gens {
		if l := r.Gens[i].Lead; l != i {
			pos := r.Gens[i].Pos
			r.Gens[i] = r.Gens[l]
			r.Gens[i].Pos, r.Gens[i].Lead = pos, l
		}
	}
}

type MazeWall struct {
	ID    int
	A, B  V2
	Super float64 // seconds of supercharge left
}

type RobotKind int

const (
	Walker RobotKind = iota
	Heavy
	Drone
)

type RobotState int

const (
	RWalk     RobotState = iota // heading for its span (drones: for a generator)
	RPress                      // leaning on the ring, draining it
	RWait                       // knocked back or waiting for a way through
	RGrab                       // going for the exposed device
	RCarry                      // carrying the device to a door
	RAttached                   // drone clamped on a generator
	RHooked                     // on the harpoon line
	RDead
	RGone // out through a door with the device
	RLurk // no ring left to lean on: closing in round the device, waiting for a chance
	// RRecover: a drone whose tether you yanked off: it limps off outside your ring and
	// patches itself up before it comes back for another go
	RRecover
)

// RecoverSecs: how long an unhooked drone takes to patch itself up.
const RecoverSecs = 55.0

type Robot struct {
	ID      int
	Kind    RobotKind
	DType   string
	Pos     V2
	Prev    V2 // where it was at the start of this step
	Yaw     float64
	State   RobotState
	Path    []V2
	HP      float64
	Burn    bool    // smashing straight through the maze
	Limit   float64 // extra maze walking it puts up with before burning through
	SpanA   int     // generators of the span it leans on
	SpanB   int
	SpanT   float64 // where along the span (0..1)
	Gen     int     // drone: target or clamped generator
	Door    int
	Wait    float64
	DeadT   float64 // seconds since it died
	Cause   string  // wall, super, wire, blast, spear, pluck, green, magenta
	Inside  bool    // lurking: one of the few allowed into your room (the rest wait outside)
	Age     float64
	Moving  float64 // 0..1, for walk animation
	Pinned  bool    // hooked and dragged against the ring
	Dazed   float64 // seconds left reeling from a shock (it just stands there, sparking)
	ZapT    float64 // a live wire just zapped it (a heavy takes a hit at most this often)
	Expo    float64 // seconds it's been lingering in a supercharge's reach (it hurts more)
	wander  V2      // dazed: the way it's stumbling
	wanderT float64
	replan  float64
}

func (b *Robot) Alive() bool { return b.State != RDead && b.State != RGone }

func (b *Robot) Radius() float64 {
	switch b.Kind {
	case Heavy:
		return 0.48
	case Drone:
		return 0.25
	}
	return 0.35
}

// Height of the robot's body (drones: the height they fly at).
func (b *Robot) Height() float64 {
	switch b.Kind {
	case Heavy:
		return 1.2
	case Drone:
		return 1.45
	}
	return 1.95
}

type DevState int

const (
	DevHome    DevState = iota
	DevStolen           // a warden is carrying it to a door
	DevDropped          // lying where its carrier fried
	DevGone             // out the door: round lost
)

type Module struct {
	ID, Name, Game string
	Secs           float64
	Done           bool
}

// Device: the EMP you're building.
type Device struct {
	Pos     V2
	Yaw     float64
	State   DevState
	Held    bool // you have hold of it (the headset side sets this)
	Exposed bool // home but outside the ring
	Carrier int
	Grabber int
	DropT   float64
	Mods    []Module
	Cur     int
	Prog    float64 // 0..1 through the current module
	Fired   bool
	Field   float64 // backup field left (seconds): while it holds, an exposed device can't be taken
	// the power leads: two cables from a box on the floor into the back of the cart. The
	// blast needs both in; a warden making off with the cart, or the cart going further from
	// the box than they reach, pulls them out.
	Leads   int
	LeadBox V2
	// Misfire: the cord was pulled and it didn't go off - the guts under the top need fixing
	// before the cord will work. Misfires: how many times it has (at most twice a round).
	Misfire  bool
	Misfires int
	armed    []bool
}

type ShotKind int

const (
	ShotNone   ShotKind = iota
	ShotFlying          // spear in the air
	ShotRobot           // stuck in a warden: reel it in
	ShotDevice          // stuck in the device or its carrier: reel it home
	ShotAnchor          // stuck in the floor or a wall: the line is a live wire
	ShotLoose           // unhooked from the gun, its end in your hand (dead until tied off)
)

type Harpoon struct {
	Up      bool // turret raised
	Pos     V2
	Gen     int // the generator whose loss would cut it off
	Armed   bool
	Powered bool
	Muzzle  V3 // where the spear leaves (kept up to date by the headset side as you aim)
	Kind    ShotKind
	Tip     V3
	Vel     V3
	Target  int
	Line    float64
	Held    bool // a hand is on the line
	Age     float64
	Super   bool // the loaded spear is supercharged
	// Reels: reel-only spears loaded (the yellow light): the one in it can hook a warden or the
	// cart but makes no live wire (supercharging it makes it a full spear again). Hooking a
	// warden with a full spear loads one of these straight after; some fresh loads are a pair.
	Reels      int
	ReelShot   bool    // the spear in flight is reel-only
	ReelLeft   int     // reel-only spears still to load after the one in flight
	Live       bool    // the shot in flight (or its line) is supercharged
	Beam       bool    // the tractor beam's hauling the dropped cart home
	LineT      float64 // how long the line out has been keeping a magenta wall charged
	prevMuzzle V3      // where the muzzle was last step (a live line swings with it)
	// Traps: old lines you tied off to the turret's base - live wires that stay up after
	// you shoot again
	Traps []Wire
}

// Wire: a live line from A to B (T: how long it's been keeping a magenta wall charged).
type Wire struct {
	A, B V3
	T    float64
}

// MaxTraps: tied-off lines kept up at once (the oldest comes down).
const MaxTraps = 3

type Phase int

const (
	Setup Phase = iota
	Playing
	Won
	Lost
)

type Group struct {
	Door  int
	Kind  RobotKind
	DType string
	N     int
}

// Intel: what the radio says is coming next.
type Intel struct {
	Wave   int
	Groups []Group
}

type Stats struct {
	Kills, Burns, Steals, Saves, GensBlown, Waves, Wires int
}

type spawnReq struct {
	at    float64
	door  int
	kind  RobotKind
	dtype string
}

type Round struct {
	T    *Tuning
	C    *Content
	Seed int64
	rng  *rand.Rand

	Room      []V2 // your real boundary
	Player    V2   // where you're standing (the game keeps it up to date)
	PlayerOK  bool
	Fixtures  []V2 // things standing in your room the turret mustn't rise on (radio, console...)
	Area      float64
	Hall      Hall
	Doors     []Door
	GenLo     int
	GenHi     int
	Gens      []Generator
	Ring      []int // powered generators, in order around the room
	RingPoly  []V2
	RingLen   float64
	Maze      []*MazeWall
	Robots    []*Robot
	Dev       Device
	Harp      Harpoon
	Points    float64
	Wave      int // waves launched so far
	NextWave  float64
	Intel     Intel
	Heard     bool // you listened to the radio for the coming wave
	Time      float64
	Phase     Phase
	Events    []Event
	SuperCD   float64
	SuperRing map[[2]int]float64 // supercharged green spans (generator pair) -> seconds left
	Stats     Stats

	nav      *Nav
	lastDT   float64
	queue    []spawnReq
	nextID   int
	ringKey  string
	mazeCost float64
}

// ---- events (for sound, effects and the HUD) ----

type EventKind int

const (
	EvWave EventKind = iota
	EvIntel
	EvSpawn
	EvGenBlown
	EvGenUp
	EvGenOff
	EvLeech
	EvPluck
	EvRing
	EvDied
	EvBurn
	EvExposed
	EvStolen
	EvDropped
	EvRecovered
	EvTugFail
	EvLost
	EvArmed
	EvFire
	EvHit
	EvWireZap
	EvSuper
	EvModule
	EvSetback
	EvWon
	EvKnock
	EvZapInside
	EvUnhook    // a drone's tether yanked off: A generator, B drone
	EvDragFry   // a hooked warden hauled through a beam: A robot, B kind
	EvTiedOff   // an old harpoon line tied off as a trap: A traps up
	EvHarpSuper // the harpoon supercharged (loading a spear if it was spent)
	EvShock     // a supercharged spear's shock: A robot killed, B kind (dazes those near)
	EvTractor   // the harpoon's tractor beam locked onto the dropped cart
	EvLineGone  // a line that kept a magenta wall charged burned out: Pos its middle
	EvMiniEMP   // the spear went into the cart: a little EMP left the wardens round it dazed
	EvDud       // a reel-only spear hit the floor or a wall: no live wire, it just drops
	EvLeadsOut  // the cart's power leads got pulled out (A: how many were in)
	EvNoPower   // the cord was pulled with the power leads out: no blast
	EvMisfire   // the cord was pulled and it didn't go off (B: 1 if it was already broken)
	EvFixed     // the misfire's fixed: the cord will work now
	EvLineFree  // the anchored line unhooked from the gun (it's in your hand, dead)
	EvLineDrop  // ...and let go of without tying it off
	EvPress
	EvWallGone
	EvPump
	EvTask
	EvFieldDown // an exposed device's backup field ran out: it can be taken
	EvLeechBite // a leech that wasn't ripped off in time has shut its generator down
	EvGenLow    // a generator under attack dropped below 30% charge
	EvQueued    // a robot is waiting behind a door (the hall is full)
)

type Event struct {
	Kind EventKind
	Pos  V2
	A, B int
	S    string
}

func (r *Round) emit(k EventKind, p V2, a, b int, s string) {
	r.Events = append(r.Events, Event{k, p, a, b, s})
}

// ---- setup ----

// NewRound: a round in your room (the real boundary, in stage space).
func NewRound(room []V2, seed int64, t *Tuning, c *Content) *Round {
	if t == nil {
		t = DefaultTuning()
	}
	if c == nil {
		c = DefaultContent()
	}
	r := &Round{T: t, C: c, Seed: seed, rng: rand.New(rand.NewSource(seed)), SuperRing: map[[2]int]float64{}}
	r.Room = append([]V2(nil), room...)
	if PolyArea(r.Room) < 0 {
		for i, j := 0, len(r.Room)-1; i < j; i, j = i+1, j-1 {
			r.Room[i], r.Room[j] = r.Room[j], r.Room[i]
		}
	}
	r.Area = PolyArea(r.Room)
	r.Hall, r.Doors = MakeHall(r.Room, t.HallMargin, c.DoorNames)
	r.GenLo, r.GenHi = GenRange(r.Room, t)
	n := int(math.Max(float64(r.GenLo), math.Min(float64(r.GenHi), 6)))
	r.Dev.Mods = r.freshModules()
	r.Dev.armed = make([]bool, len(t.HarpoonAt))
	r.SetGens(n)
	r.Dev.Pos = Centroid(r.RingPoly)
	r.Dev.LeadBox, r.Dev.Leads = r.leadBoxSpot(), LeadsNeeded
	r.Dev.Field = t.FieldMax(r.Machines())
	return r
}

func (r *Round) freshModules() []Module {
	var ms []Module
	// empWork scales the modules' book times (how the work is weighted and how long the
	// simulated player takes; your hands take as long as they take)
	tot := 0.0
	for _, m := range r.C.EMPModules {
		tot += m.Secs
	}
	k := 1.0
	if tot > 0 && r.T.EMPWork > 0 {
		k = r.T.EMPWork / tot
	}
	var last []Module
	used := map[string]bool{}
	for _, m := range r.C.EMPModules {
		mod := Module{ID: m.ID, Name: m.Name, Secs: m.Secs * k}
		if m.ID == "game" && len(r.C.Games) > 0 {
			// a different game each time the device wants one
			for tries := 0; tries < 20; tries++ {
				mod.Game = r.C.Games[r.rng.Intn(len(r.C.Games))]
				if !used[mod.Game] || len(used) >= len(r.C.Games) {
					break
				}
			}
			used[mod.Game] = true
		}
		if m.ID == "ripcord" {
			last = append(last, mod) // pulling the cord sets it off: always the end
			continue
		}
		ms = append(ms, mod)
	}
	shuffleModules(ms, r.rng)
	ms = append(ms, last...)
	// the games read in the order they come
	ng := 0
	for i := range ms {
		if ms[i].ID == "game" {
			ms[i].Name = []string{"THE DEVICE WANTS A GAME", "THE DEVICE WANTS ANOTHER GAME"}[min(ng, 1)]
			ng++
		}
	}
	return ms
}

// BookOrder: the device's steps back in the order the content lists them (setup only; tests
// that walk through them in turn use it).
func (r *Round) BookOrder() {
	if r.Phase != Setup {
		return
	}
	used := make([]bool, len(r.Dev.Mods))
	var out []Module
	for _, c := range r.C.EMPModules {
		for i, m := range r.Dev.Mods {
			if !used[i] && m.ID == c.ID {
				used[i] = true
				m.Name = c.Name
				out = append(out, m)
				break
			}
		}
	}
	r.Dev.Mods = out
}

// shuffleModules: the steps come in a different order every round - except the capacitor
// bank's sockets are under the access plate (the plate comes off first), and the two games
// aren't back to back.
func shuffleModules(ms []Module, rng *rand.Rand) {
	at := func(id string) int {
		for i, m := range ms {
			if m.ID == id {
				return i
			}
		}
		return -1
	}
	for try := 0; try < 40; try++ {
		rng.Shuffle(len(ms), func(i, j int) { ms[i], ms[j] = ms[j], ms[i] })
		if p, w := at("screws"), at("wires"); p >= 0 && w >= 0 && w < p {
			ms[p], ms[w] = ms[w], ms[p]
		}
		ok := true
		for i := 1; i < len(ms); i++ {
			if ms[i].ID == "game" && ms[i-1].ID == "game" {
				ok = false
			}
		}
		if ok {
			return
		}
	}
}

// SetGens: choose how many generators (setup only). Returns false if not allowed.
func (r *Round) SetGens(n int) bool {
	if r.Phase != Setup || n < r.GenLo || n > r.GenHi {
		return false
	}
	pos, lead := PlaceGensGrouped(r.Room, n, r.T)
	cost := r.T.GreenCost(n, PolyPerim(pos))
	if r.T.Budget(r.Area)-cost-r.mazeCost < -1e-6 && len(r.Gens) > 0 {
		return false
	}
	r.Gens = make([]Generator, len(pos))
	for i, p := range pos {
		r.Gens[i] = Generator{Pos: p, State: GenUp, Charge: r.T.ChargeMax, Lead: lead[i]}
	}
	r.nav = NewNav(r.Hall, pos)
	r.ringKey = ""
	r.updateRing()
	r.nav.SetMaze(r.mazeSegs())
	r.Points = r.T.Budget(r.Area) - cost - r.mazeCost
	r.Dev.Field = r.T.FieldMax(n)
	if len(r.RingPoly) >= 3 && !InPoly(r.RingPoly, r.Dev.Pos) {
		r.Dev.Pos = Centroid(r.RingPoly)
		r.Dev.LeadBox = r.leadBoxSpot()
	}
	return true
}

// GreenCostFor: what n generators would cost in this room (for the setup console).
func (r *Round) GreenCostFor(n int) float64 {
	pos, _ := PlaceGensGrouped(r.Room, n, r.T)
	return r.T.GreenCost(n, PolyPerim(pos))
}

// Start pulls the lever: the round is on.
func (r *Round) Start() {
	if r.Phase != Setup {
		return
	}
	r.Phase = Playing
	r.Time = 0
	r.NextWave = r.T.FirstWave
	r.Dev.LeadBox, r.Dev.Leads = r.leadBoxSpot(), LeadsNeeded
	r.Intel = r.makeIntel(0)
	r.emit(EvIntel, V2{}, 1, 0, "")
}

// ---- the step ----

// TakeEvents: everything that happened since the last call (the caller plays the sounds).
func (r *Round) TakeEvents() []Event {
	ev := append([]Event(nil), r.Events...)
	r.Events = r.Events[:0]
	return ev
}

func (r *Round) Step(dt float64) {
	r.lastDT = dt
	if r.Phase != Playing {
		return
	}
	r.Time += dt
	r.SuperCD = math.Max(0, r.SuperCD-dt)
	if d := &r.Dev; d.Leads > 0 && (d.State != DevHome || Dist(d.Pos, d.LeadBox) > LeadReach) {
		r.pullLeads() // dragged further than they reach
	}
	for _, w := range r.Maze {
		w.Super = math.Max(0, w.Super-dt)
	}
	for _, k := range r.superKeys() {
		if v := r.SuperRing[k] - dt; v > 0 {
			r.SuperRing[k] = v
		} else {
			delete(r.SuperRing, k)
		}
	}
	r.superBlast(dt)
	if r.Time >= r.NextWave {
		r.launchWave()
	}
	for len(r.queue) > 0 && r.queue[0].at <= r.Time {
		q := r.queue[0]
		if q.kind != Drone && r.T.MaxRobots > 0 && r.walkersAlive() >= r.T.MaxRobots {
			// the hall is full: everyone still to come waits behind the doors a moment longer
			for i := range r.queue {
				r.queue[i].at += 1.5
			}
			break
		}
		r.queue = r.queue[1:]
		r.spawn(q)
	}
	for i := range r.Gens {
		g := &r.Gens[i]
		g.Pressed = 0
		if g.Lead != i {
			continue
		}
		if g.State == GenOff {
			g.Charge = math.Min(r.T.ChargeMax, g.Charge+r.T.OffRecharge*dt)
		}
	}
	for _, b := range r.Robots {
		b.Prev = b.Pos
		if b.Alive() {
			b.Age += dt
			r.stepRobot(b, dt)
		} else {
			b.DeadT += dt
		}
	}
	r.separate()
	r.keepClear()
	for _, b := range r.Robots {
		if b.Alive() {
			r.contactChecks(b)
		}
	}
	for i := range r.Gens {
		g := &r.Gens[i]
		if g.Lead != i {
			continue
		}
		if g.State == GenUp && !g.Low && g.Charge < 0.3*r.T.ChargeMax {
			g.Low = true
			r.emit(EvGenLow, g.Pos, i, 0, "")
		} else if g.Low && g.Charge > 0.4*r.T.ChargeMax {
			g.Low = false
		}
		if g.State == GenUp && g.Charge <= 0 {
			g.Charge = 0
			g.State = GenBlown
			g.Order = r.workOrder()
			g.Done = 0
			r.Stats.GensBlown++
			r.emit(EvGenBlown, g.Pos, i, 0, "")
		}
	}
	r.mirrorGens()
	r.updateRing()
	r.stepDevice(dt)
	r.stepHarpoon(dt)
	// drop the dead after a while (the headset side animates them meanwhile)
	live := r.Robots[:0]
	for _, b := range r.Robots {
		if b.Alive() || b.DeadT < 8 {
			live = append(live, b)
		}
	}
	r.Robots = live
}

func (r *Round) superKeys() [][2]int {
	ks := make([][2]int, 0, len(r.SuperRing))
	for k := range r.SuperRing {
		ks = append(ks, k)
	}
	// deterministic order
	for i := 1; i < len(ks); i++ {
		for j := i; j > 0 && (ks[j][0] < ks[j-1][0] || ks[j][0] == ks[j-1][0] && ks[j][1] < ks[j-1][1]); j-- {
			ks[j], ks[j-1] = ks[j-1], ks[j]
		}
	}
	return ks
}

// ---- waves ----

func sround(rng *rand.Rand, x float64) int {
	f := math.Floor(x)
	if rng.Float64() < x-f {
		f++
	}
	return int(f)
}

func (r *Round) makeIntel(k int) Intel {
	t := r.T
	ws := t.waveScale(r.Area)
	nw := max(1, sround(r.rng, (t.WalkersBase+t.WalkersPer*float64(k))*ws))
	nh := sround(r.rng, math.Floor(float64(k)/2)*t.HeaviesPer2*ws)
	nd := sround(r.rng, (t.DronesBase+math.Floor(t.DronesPer*float64(k)))*ws*t.droneScale(r.Machines(), r.Area))
	in := Intel{Wave: k + 1}
	nd0 := len(r.Doors)
	d1 := r.rng.Intn(nd0)
	if nw >= 3 {
		d2 := (d1 + 1 + r.rng.Intn(nd0-1)) % nd0
		a := (nw + 1) / 2
		in.Groups = append(in.Groups, Group{Door: d1, Kind: Walker, N: a}, Group{Door: d2, Kind: Walker, N: nw - a})
	} else {
		in.Groups = append(in.Groups, Group{Door: d1, Kind: Walker, N: nw})
	}
	if nh > 0 {
		in.Groups = append(in.Groups, Group{Door: r.rng.Intn(nd0), Kind: Heavy, N: nh})
	}
	if nd > 0 {
		in.Groups = append(in.Groups, Group{Door: r.rng.Intn(nd0), Kind: Drone, DType: r.droneType(k), N: nd})
	}
	return in
}

func (r *Round) droneType(k int) string {
	tot := 0.0
	for _, d := range r.C.Drones {
		if d.From <= k {
			tot += d.Weight
		}
	}
	x := r.rng.Float64() * tot
	for _, d := range r.C.Drones {
		if d.From <= k {
			x -= d.Weight
			if x <= 0 {
				return d.ID
			}
		}
	}
	return "leech"
}

func (r *Round) launchWave() {
	if r.Wave > 0 {
		r.Points += r.T.WaveBonus
		r.Stats.Waves++
	}
	base := r.Time
	for _, g := range r.Intel.Groups {
		gap := 1.2
		if g.Kind == Drone {
			gap = 1.5
		}
		for i := 0; i < g.N; i++ {
			r.queue = append(r.queue, spawnReq{at: base + float64(i)*gap, door: g.Door, kind: g.Kind, dtype: g.DType})
		}
	}
	// keep the queue in time order
	for i := 1; i < len(r.queue); i++ {
		for j := i; j > 0 && r.queue[j].at < r.queue[j-1].at; j-- {
			r.queue[j], r.queue[j-1] = r.queue[j-1], r.queue[j]
		}
	}
	r.Wave++
	r.emit(EvWave, V2{}, r.Wave, 0, "")
	r.NextWave = r.Time + r.T.WaveEvery
	r.Intel = r.makeIntel(r.Wave)
	r.Heard = false
	r.emit(EvIntel, V2{}, r.Wave+1, 0, "")
}

func (r *Round) spawn(q spawnReq) {
	r.nextID++
	d := r.Doors[q.door]
	side := d.Out.Perp()
	b := &Robot{ID: r.nextID, Kind: q.kind, DType: q.dtype, Door: q.door, SpanA: -1, SpanB: -1, Gen: -1}
	b.Pos = d.Mouth.Add(side.Mul((r.rng.Float64() - 0.5) * 0.8))
	b.Prev = b.Pos
	b.Yaw = math.Atan2(-d.Out.Z, -d.Out.X)
	switch q.kind {
	case Heavy:
		b.HP = r.T.HeavyHP
	case Drone:
		b.HP = 1
	default:
		b.HP = r.T.WalkerHP
	}
	b.Limit = r.T.KamikazeFt / FtPerM * (r.T.BurnMin + (r.T.BurnMax-r.T.BurnMin)*r.rng.Float64())
	r.Robots = append(r.Robots, b)
	r.emit(EvSpawn, b.Pos, b.ID, int(q.kind), q.dtype)
	if q.kind == Drone {
		r.planDrone(b)
	} else {
		r.planWalker(b, true)
	}
}

// Spawn: send one robot in through a door right now (tests, tutorials, practice).
func (r *Round) Spawn(kind RobotKind, door int, dtype string) *Robot {
	if kind == Drone && dtype == "" {
		dtype = "leech"
	}
	r.spawn(spawnReq{at: r.Time, door: door, kind: kind, dtype: dtype})
	return r.Robots[len(r.Robots)-1]
}

func (r *Round) walkersAlive() int {
	n := 0
	for _, b := range r.Robots {
		if b.Alive() && b.Kind != Drone {
			n++
		}
	}
	return n
}

func (r *Round) robot(id int) *Robot {
	for _, b := range r.Robots {
		if b.ID == id {
			return b
		}
	}
	return nil
}

// ---- the ring ----

func (r *Round) updateRing() {
	var ring []int
	key := ""
	for i, g := range r.Gens {
		if g.State == GenUp {
			ring = append(ring, i)
			key += fmt.Sprint(i, ",")
		}
	}
	if key == r.ringKey && r.ringKey != "" {
		return
	}
	first := r.ringKey == ""
	r.ringKey = key
	old := r.RingPoly
	r.Ring = ring
	r.RingPoly = nil
	r.RingLen = 0
	if len(ring) >= 3 {
		for _, i := range ring {
			r.RingPoly = append(r.RingPoly, r.Gens[i].Pos)
		}
		r.RingLen = PolyPerim(r.RingPoly)
	}
	r.nav.SetRing(r.RingPoly)
	if first {
		return
	}
	r.emit(EvRing, V2{}, len(ring), 0, "")
	// whatever got caught inside a ring that just grew back gets zapped out
	for _, b := range r.Robots {
		if !b.Alive() || b.State == RAttached || len(r.RingPoly) < 3 {
			continue
		}
		if InPoly(r.RingPoly, b.Pos) && (len(old) < 3 || !InPoly(old, b.Pos)) {
			r.emit(EvZapInside, b.Pos, b.ID, 0, "")
			if b.Kind == Heavy {
				b.HP -= 18
				b.Pos = r.pushOut(b.Pos, b.Radius()+0.1)
				if b.HP <= 0 {
					r.kill(b, "green")
				}
			} else {
				r.kill(b, "green")
			}
		}
	}
	// wardens on spans that no longer exist move on; drones heading for dead generators retarget
	for _, b := range r.Robots {
		if !b.Alive() {
			continue
		}
		switch b.State {
		case RPress, RWalk:
			if b.Kind != Drone && !r.spanValid(b.SpanA, b.SpanB) {
				r.planWalker(b, false)
			}
		}
	}
}

func (r *Round) pushOut(p V2, d float64) V2 {
	best, bp := math.Inf(1), p
	P := r.RingPoly
	for i := range P {
		a, b := P[i], P[(i+1)%len(P)]
		dd, u := SegDist(p, a, b)
		if dd < best {
			best = dd
			q := Lerp(a, b, u)
			n := r.outward(a, b)
			bp = q.Add(n.Mul(d))
		}
	}
	return bp
}

// spanValid: a and b are neighbours in the powered ring.
func (r *Round) spanValid(a, b int) bool {
	n := len(r.Ring)
	if n < 3 || a < 0 || b < 0 {
		return false
	}
	for k := 0; k < n; k++ {
		if r.Ring[k] == a && r.Ring[(k+1)%n] == b {
			return true
		}
	}
	return false
}

// outward: the unit normal of ring edge a->b pointing out of the ring.
func (r *Round) outward(a, b V2) V2 {
	n := b.Sub(a).Perp().Norm()
	if len(r.RingPoly) >= 3 && InPoly(r.RingPoly, Lerp(a, b, 0.5).Add(n.Mul(0.05))) {
		n = n.Mul(-1)
	}
	return n
}

// Spans: the powered ring's spans as generator pairs.
func (r *Round) Spans() [][2]int {
	n := len(r.Ring)
	if n < 3 {
		return nil
	}
	out := make([][2]int, n)
	for k := 0; k < n; k++ {
		out[k] = [2]int{r.Ring[k], r.Ring[(k+1)%n]}
	}
	return out
}

func (r *Round) strength() float64 { return r.T.strength(r.RingLen) }

// ---- wardens ----

func (r *Round) speedOf(b *Robot) float64 {
	switch {
	case b.State == RCarry:
		return r.T.CarrySpeed / FtPerM
	case b.Kind == Heavy:
		return r.T.HeavySpeed / FtPerM
	case b.Kind == Drone:
		return r.T.DroneSpeed / FtPerM
	}
	return r.T.WalkerSpeed / FtPerM
}

func (r *Round) pickSpan(b *Robot) (int, int, bool) {
	spans := r.Spans()
	if len(spans) == 0 {
		return -1, -1, false
	}
	count := map[[2]int]int{}
	for _, o := range r.Robots {
		if o != b && o.Alive() && o.Kind != Drone && o.SpanA >= 0 {
			count[[2]int{o.SpanA, o.SpanB}]++
		}
	}
	best, bs := -1, math.Inf(1)
	for k, s := range spans {
		if r.Gens[s[0]].Lead == r.Gens[s[1]].Lead {
			continue // the short beam between two posts of one generator: nothing to lean on
		}
		m := Lerp(r.Gens[s[0]].Pos, r.Gens[s[1]].Pos, 0.5)
		score := Dist(b.Pos, m) + 2.5*float64(count[s]) + 1.5*r.rng.Float64()
		if score < bs {
			bs, best = score, k
		}
	}
	if best < 0 {
		return -1, -1, false
	}
	return spans[best][0], spans[best][1], true
}

// pressPoint: where a warden of radius rad stands to lean on span a-b at t.
func (r *Round) pressPoint(a, b int, t, rad float64) V2 {
	A, B := r.Gens[a].Pos, r.Gens[b].Pos
	return Lerp(A, B, t).Add(r.outward(A, B).Mul(rad + 0.08))
}

func (r *Round) spanSpot(b *Robot, a, c int) float64 {
	best, bd := 0.5, -1.0
	for k := 0; k <= 12; k++ {
		t := 0.22 + 0.56*float64(k)/12
		d := 9.0
		for _, o := range r.Robots {
			if o != b && o.Alive() && o.SpanA == a && o.SpanB == c {
				d = math.Min(d, math.Abs(o.SpanT-t))
			}
		}
		d += 0.01 * r.rng.Float64()
		if d > bd {
			bd, best = d, t
		}
	}
	return best
}

// planWalker: pick a span and a route to it. On the first plan the warden decides whether
// your maze is worth walking, or whether it burns straight through.
func (r *Round) planWalker(b *Robot, first bool) {
	a, c, ok := r.pickSpan(b)
	if !ok {
		// nothing left to lean on: come for the device
		r.planLurk(b)
		return
	}
	b.SpanA, b.SpanB = a, c
	b.SpanT = r.spanSpot(b, a, c)
	goal := r.pressPoint(a, c, b.SpanT, b.Radius())
	rad := b.Radius()
	if first {
		pm, lm, okm := r.nav.Path(b.Pos, goal, rad, false)
		pd, ld, okd := r.nav.Path(b.Pos, goal, rad, true)
		switch {
		case okm && (!okd || lm-ld <= b.Limit):
			b.Path, b.Burn = pm, false
		case okd:
			b.Path, b.Burn = pd, true
			r.Stats.Burns++
		default:
			b.State, b.Wait = RWait, 1.5
			return
		}
	} else {
		p, _, ok := r.nav.Path(b.Pos, goal, rad, b.Burn)
		if !ok {
			p, _, ok = r.nav.Path(b.Pos, goal, rad, true)
			if !ok {
				b.State, b.Wait = RWait, 1.5
				return
			}
			b.Burn = true
		}
		b.Path = p
	}
	b.State = RWalk
}

// planLurk: with your ring down, a warden closes in on the device and waits round it
// (the nearest one takes it once its field fails; the rest crowd you).
func (r *Round) planLurk(b *Robot) {
	b.SpanA, b.SpanB = -1, -1
	d := &r.Dev
	if d.State == DevGone {
		b.State, b.Wait = RWait, 2
		return
	}
	// only a couple come right into your room; the rest wait just outside it, so you can
	// still see what you're doing
	a := float64(b.ID) * 2.39996
	in := 0
	for _, o := range r.Robots {
		if o != b && o.Alive() && o.Kind != Drone && o.State == RLurk && o.Inside {
			in++
		}
	}
	b.Inside = in < MaxInside
	spot := d.Pos.Add(V(math.Cos(a), math.Sin(a)).Mul(0.85 + 0.25*float64(b.ID%3)))
	if !b.Inside {
		spot = r.outsideSpot(b.Pos, 0.6+0.3*float64(b.ID%3))
	}
	if d.State == DevStolen && b.Inside {
		// follow the carrier out, a step behind
		if c := r.robot(d.Carrier); c != nil {
			spot = c.Pos.Sub(V(math.Cos(c.Yaw), math.Sin(c.Yaw)).Mul(1.2)).Add(V(math.Cos(a), math.Sin(a)).Mul(0.6))
		}
	}
	p, _, ok := r.nav.Path(b.Pos, spot, b.Radius(), b.Burn)
	if !ok {
		p, _, ok = r.nav.Path(b.Pos, spot, b.Radius(), true)
	}
	if !ok {
		b.State, b.Wait = RWait, 1.5
		return
	}
	b.Path, b.State, b.replan = p, RLurk, 2
}

// MaxInside: how many wardens come right into your room when your ring is down.
const MaxInside = 2

// outsideSpot: a place just outside your real boundary (d beyond it), near p.
func (r *Round) outsideSpot(p V2, d float64) V2 {
	sign := 1.0
	if PolyArea(r.Room) < 0 {
		sign = -1
	}
	best, bd, bn := p, math.Inf(1), V2{}
	for i := range r.Room {
		a, c := r.Room[i], r.Room[(i+1)%len(r.Room)]
		dd, u := SegDist(p, a, c)
		if dd < bd {
			e := c.Sub(a).Norm()
			best, bd, bn = Lerp(a, c, u), dd, V(e.Z, -e.X).Mul(sign) // the outward side
		}
	}
	q := best.Add(bn.Mul(d))
	for k := 0; k < 8 && InPoly(r.Room, q); k++ {
		q = q.Add(bn.Mul(0.25))
	}
	return q
}

// keepClear: wardens can't stand where you're standing - they step round you.
func (r *Round) keepClear() {
	if !r.PlayerOK {
		return
	}
	for _, b := range r.Robots {
		if !b.Alive() || b.Kind == Drone || b.State == RAttached || b.State == RHooked {
			continue
		}
		min := b.Radius() + PersonalSpace
		d := Dist(b.Pos, r.Player)
		if d >= min {
			continue
		}
		dir := b.Pos.Sub(r.Player)
		if d < 1e-4 {
			dir = V(math.Cos(b.Yaw), math.Sin(b.Yaw))
		}
		// straight out from you, or slipping round you if something's in the way
		for _, turn := range []float64{0, 0.7, -0.7, 1.4, -1.4} {
			np := r.Player.Add(dir.Norm().Rot(turn).Mul(min))
			if r.nav.Walkable(np, b.Radius()*0.8, true) {
				b.Pos = np
				break
			}
		}
	}
}

// PersonalSpace: how close a warden's edge gets to the middle of you.
const PersonalSpace = 0.45

func (r *Round) planDrone(b *Robot) {
	var ups []int
	for _, i := range r.Ring {
		if r.Gens[i].Leech == 0 && r.Gens[i].Lead == i {
			ups = append(ups, i)
		}
	}
	if len(ups) == 0 {
		b.State, b.Wait = RWait, 3
		return
	}
	g := ups[r.rng.Intn(len(ups))]
	b.Gen = g
	goal := r.droneSpot(g)
	p, _, ok := r.nav.Path(b.Pos, goal, b.Radius(), false)
	if !ok {
		b.State, b.Wait = RWait, 2
		return
	}
	b.Path, b.State = p, RWalk
}

// droneSpot: just outside generator g, where a drone hovers before clamping on.
// LeechHop: seconds a drone takes to hop over the beam onto a generator's inner face
// (before its bite starts counting).
const LeechHop = 1.0

func (r *Round) droneSpot(g int) V2 {
	p := r.Gens[g].Pos
	n := len(r.Gens)
	a, c := r.Gens[(g-1+n)%n].Pos, r.Gens[(g+1)%n].Pos
	out := r.outward(a, p).Add(r.outward(p, c)).Norm()
	if out.Len() < 0.5 {
		out = p.Sub(Centroid(r.Gens2())).Norm()
	}
	return p.Add(out.Mul(0.62))
}

func (r *Round) Gens2() []V2 {
	out := make([]V2, len(r.Gens))
	for i, g := range r.Gens {
		out[i] = g.Pos
	}
	return out
}

// follow: walk the path; returns true on arrival.
func (r *Round) follow(b *Robot, speed, dt float64) bool {
	step := speed * dt
	for step > 0 && len(b.Path) > 0 {
		tgt := b.Path[0]
		d := Dist(b.Pos, tgt)
		if d <= step {
			b.Pos = tgt
			b.Path = b.Path[1:]
			step -= d
			continue
		}
		dir := tgt.Sub(b.Pos).Mul(1 / d)
		b.Pos = b.Pos.Add(dir.Mul(step))
		want := math.Atan2(dir.Z, dir.X)
		b.Yaw += angDiff(b.Yaw, want) * math.Min(1, dt*6)
		step = 0
	}
	b.Moving += (1 - b.Moving) * math.Min(1, dt*5)
	return len(b.Path) == 0
}

func angDiff(a, b float64) float64 {
	d := math.Mod(b-a+math.Pi, 2*math.Pi)
	if d < 0 {
		d += 2 * math.Pi
	}
	return d - math.Pi
}

func (r *Round) stepRobot(b *Robot, dt float64) {
	b.Dazed = math.Max(0, b.Dazed-dt)
	b.ZapT = math.Max(0, b.ZapT-dt)
	if b.Dazed > 0 && b.State == RWait && b.Kind != Drone {
		// dazed: it stumbles about with no idea where it's going
		if b.wanderT -= dt; b.wanderT <= 0 || b.wander.Len() < 0.5 {
			a := r.rng.Float64() * 2 * math.Pi
			b.wander, b.wanderT = V(math.Cos(a), math.Sin(a)), 0.8+1.4*r.rng.Float64()
		}
		np := b.Pos.Add(b.wander.Mul(r.speedOf(b) * 0.35 * dt))
		if r.nav.Walkable(np, b.Radius()*0.8, false) {
			b.Pos = np
			b.Yaw = math.Atan2(b.wander.Z, b.wander.X)
			b.Moving = 0.5
		} else {
			b.wanderT = 0
		}
	}
	if b.State != RWalk && b.State != RGrab && b.State != RCarry {
		b.Moving *= math.Max(0, 1-dt*4)
	}
	switch b.State {
	case RWait:
		b.Wait -= dt
		if b.Wait <= 0 {
			if b.Kind == Drone {
				r.planDrone(b)
			} else {
				r.planWalker(b, false)
			}
		}
	case RWalk:
		if b.Kind == Drone {
			if r.Gens[b.Gen].State != GenUp || r.Gens[b.Gen].Leech != 0 {
				r.planDrone(b)
				return
			}
			if r.follow(b, r.speedOf(b), dt) {
				g := &r.Gens[b.Gen]
				if g.State == GenUp && g.Leech == 0 {
					// it clamps on and starts boring in: a few seconds to rip it off before
					// the generator shuts down
					// it rises up over the beam, hovers above the generator and lets down a
					// tether that clamps onto the inside of its shaft (where you can reach it)
					g.Leech = b.ID
					b.State, b.Wait = RAttached, -LeechHop
					b.Pos = g.Pos // hovering right over it, its tether clamped on
					r.emit(EvLeech, g.Pos, b.Gen, b.ID, "")
				} else {
					r.planDrone(b)
				}
			}
			return
		}
		if !r.spanValid(b.SpanA, b.SpanB) {
			r.planWalker(b, false)
			return
		}
		if r.follow(b, r.speedOf(b), dt) {
			b.State = RPress
			A, B := r.Gens[b.SpanA].Pos, r.Gens[b.SpanB].Pos
			n := r.outward(A, B)
			b.Yaw = math.Atan2(-n.Z, -n.X)
			r.emit(EvPress, b.Pos, b.ID, 0, "")
		}
	case RPress:
		if !r.spanValid(b.SpanA, b.SpanB) {
			r.planWalker(b, false)
			return
		}
		r.press(b, b.SpanA, b.SpanB, dt)
	case RGrab:
		d := &r.Dev
		if d.State == DevHome && !d.Exposed || d.State == DevStolen || d.State == DevGone {
			d.Grabber = 0
			r.planWalker(b, false)
			return
		}
		b.replan -= dt
		if b.replan <= 0 {
			b.replan = 1
			p, _, ok := r.nav.Path(b.Pos, d.Pos, b.Radius(), b.Burn)
			if !ok {
				p, _, ok = r.nav.Path(b.Pos, d.Pos, b.Radius(), true)
			}
			if ok {
				b.Path = p
			}
		}
		r.follow(b, r.speedOf(b), dt)
		if Dist(b.Pos, d.Pos) < b.Radius()+0.45 {
			r.steal(b)
		}
	case RCarry:
		if r.Harp.Kind == ShotDevice {
			return // the harpoon line holds it; stepHarpoon drags things around
		}
		if r.follow(b, r.speedOf(b), dt) {
			// out through the door
			b.State = RGone
			r.Dev.State = DevGone
			r.Dev.Pos = r.Doors[b.Door].Wall
			r.Phase = Lost
			r.emit(EvLost, b.Pos, b.ID, 0, "")
			return
		}
		r.carryDevice(b)
	case RLurk:
		b.replan -= dt
		if b.replan <= 0 {
			// back to the ring as soon as there's one; otherwise keep closing in
			if _, _, ok := r.pickSpan(b); ok {
				r.planWalker(b, false)
				return
			}
			r.planLurk(b)
			return
		}
		if len(b.Path) > 0 {
			r.follow(b, r.speedOf(b), dt)
		} else {
			b.Moving *= math.Max(0, 1-dt*4)
			to := r.Dev.Pos.Sub(b.Pos)
			b.Yaw += angDiff(b.Yaw, math.Atan2(to.Z, to.X)) * math.Min(1, dt*3)
		}
	case RRecover:
		b.Wait -= dt
		if b.Wait <= 0 {
			r.planDrone(b)
		}
	case RAttached:
		g := &r.Gens[b.Gen]
		if g.Leech != b.ID {
			r.kill(b, "pluck")
			break
		}
		b.Wait += dt
		if g.State == GenUp && b.Wait >= r.T.LeechBite {
			g.State = GenLeeched
			r.emit(EvLeechBite, g.Pos, b.Gen, b.ID, "")
			r.mirrorGens()
			r.updateRing()
		}
	case RHooked:
		// stepHarpoon moves it
		if b.Pinned && r.spanValid(b.SpanA, b.SpanB) {
			r.press(b, b.SpanA, b.SpanB, dt)
		}
	}
}

func (r *Round) press(b *Robot, a, c int, dt float64) {
	s := r.strength()
	k := 1.0
	if b.Kind == Heavy {
		k = r.T.HeavyDrain
	}
	rate := r.T.Drain * k / (0.3 + 2*s) * dt
	la, lc := r.Gens[a].Lead, r.Gens[c].Lead
	for _, gi := range []int{la, lc} {
		g := &r.Gens[gi]
		g.Pressed++
		if g.State == GenUp && !(gi == lc && la == lc) { // the two ends of one machine: drained once
			g.Charge -= rate
		}
	}
	if _, hot := r.SuperRing[[2]int{a, c}]; hot {
		if b.Kind == Heavy {
			b.HP -= 8 * dt
		} else {
			b.HP = 0
		}
		if b.HP <= 0 {
			r.kill(b, "super")
		}
		return
	}
	// draining your ring costs it its own power: a warden that's already been hit by a
	// supercharge doesn't have long before it drops
	b.HP -= r.T.Sting*s*dt + rate*DrainCost
	if b.HP <= 0 {
		r.kill(b, "wall")
	}
}

// DrainCost: power a warden spends per unit of charge it drains from your ring (a fresh
// walker drains about a third of a generator before it burns itself out)
const DrainCost = 0.12

// MaxHP: a robot's power when it came in.
func (r *Round) MaxHP(b *Robot) float64 {
	switch b.Kind {
	case Heavy:
		return r.T.HeavyHP
	case Drone:
		return 1
	}
	return r.T.WalkerHP
}

func (r *Round) kill(b *Robot, cause string) {
	if !b.Alive() {
		return
	}
	prev := b.State
	b.State, b.Cause, b.DeadT, b.Path = RDead, cause, 0, nil
	r.Stats.Kills++
	r.emit(EvDied, b.Pos, b.ID, int(b.Kind), cause)
	d := &r.Dev
	if d.Grabber == b.ID {
		d.Grabber = 0
	}
	if prev == RCarry && d.State == DevStolen && d.Carrier == b.ID {
		d.Carrier = 0
		d.State = DevDropped
		d.DropT = r.T.Regrab
		r.emit(EvDropped, d.Pos, 0, 0, "")
		if len(r.RingPoly) >= 3 && InPoly(r.RingPoly, d.Pos) {
			d.State = DevHome
			r.emit(EvRecovered, d.Pos, 0, 0, "")
		}
	}
	if b.Kind == Drone && b.Gen >= 0 && r.Gens[b.Gen].Leech == b.ID {
		g := &r.Gens[b.Gen]
		g.Leech = 0
		if g.State == GenLeeched {
			g.State = GenUp
			if g.Charge <= 0 {
				g.State = GenBlown
				g.Order = r.workOrder()
			}
		}
		r.mirrorGens()
		r.emit(EvPluck, g.Pos, b.Gen, b.ID, "")
	}
	if r.Harp.Target == b.ID && r.Harp.Kind == ShotRobot {
		r.Harp.Kind = ShotNone
	}
}

// separate: walking robots don't pile into one another.
func (r *Round) separate() {
	for i, a := range r.Robots {
		if !a.Alive() || a.State == RAttached || a.State == RHooked {
			continue
		}
		for _, b := range r.Robots[i+1:] {
			if !b.Alive() || b.State == RAttached || b.State == RHooked || (a.Kind == Drone) != (b.Kind == Drone) {
				continue
			}
			minD := a.Radius() + b.Radius() + 0.05
			d := Dist(a.Pos, b.Pos)
			if d >= minD || d < 1e-6 {
				continue
			}
			push := b.Pos.Sub(a.Pos).Mul((minD - d) / d / 2)
			ma, mb := 1.0, 1.0
			if a.State == RPress {
				ma, mb = 0.2, 1.8
			}
			if b.State == RPress {
				mb = 0.2
				if a.State != RPress {
					ma = 1.8
				}
			}
			// a warden carrying the device shoulders everyone else out of its way
			if a.State == RCarry {
				ma, mb = 0, 2
			} else if b.State == RCarry {
				ma, mb = 2, 0
			}
			na, nb := a.Pos.Sub(push.Mul(ma)), b.Pos.Add(push.Mul(mb))
			if r.nav.Walkable(na, a.Radius()*0.8, true) {
				a.Pos = na
			}
			if r.nav.Walkable(nb, b.Radius()*0.8, true) {
				b.Pos = nb
			}
		}
	}
}

// contactChecks: what a robot's move this step ran into (smashed magenta, a live wire, a
// supercharged wall).
func (r *Round) contactChecks(b *Robot) {
	if b.State == RAttached {
		return
	}
	if b.Pos != b.Prev {
		for _, w := range r.Maze {
			if _, _, hit := SegHit(b.Prev, b.Pos, w.A, w.B); !hit {
				continue
			}
			if w.Super > 0 {
				r.kill(b, "super")
				return
			}
			if b.Kind != Drone && (b.Burn || b.State == RHooked) {
				r.wreck(w, b.Pos)
				// burning through costs it: a walker usually doesn't make it out the other side
				if b.State != RHooked {
					die := BurnDeathWalker
					if b.Kind == Heavy {
						die = BurnDeathHeavy
					}
					if r.rng.Float64() < die {
						r.kill(b, "magenta")
						return
					}
				}
				if b.State == RHooked {
					b.HP -= 4
					if b.HP <= 0 {
						r.kill(b, "magenta")
						return
					}
				}
			}
			break
		}
		wires := r.Harp.Traps
		if r.Harp.Kind == ShotAnchor {
			wires = append(append([]Wire(nil), wires...), Wire{A: r.Harp.Muzzle, B: r.Harp.Tip})
		}
		for _, w := range wires {
			if !r.Harp.Powered {
				break
			}
			m, tip := w.A, w.B
			// its body touching the line (or its step crossing it)
			_, u, hit := SegHit(b.Prev, b.Pos, m.XZ(), tip.XZ())
			if dd, uu := SegDist(b.Pos, m.XZ(), tip.XZ()); !hit && dd < b.Radius()+0.04 {
				u, hit = uu, true
			}
			if hit {
				h := m.Y + (tip.Y-m.Y)*u
				lo, hi := 0.05, b.Height()+0.05
				if b.Kind == Drone {
					lo, hi = b.Height()-0.35, b.Height()+0.35
				}
				if h >= lo && h <= hi {
					r.wireZap(b, Lerp(m.XZ(), tip.XZ(), u))
					return
				}
			}
		}
	}
	// standing against a hot magenta wall
	for _, w := range r.Maze {
		if w.Super > 0 {
			if d, _ := SegDist(b.Pos, w.A, w.B); d < b.Radius()+0.06 {
				r.kill(b, "super")
				return
			}
		}
	}
}

// wireZap: a live harpoon line zaps b where it touches it (at): a walker or drone fries, a
// heavy takes a hit (at most every half second).
func (r *Round) wireZap(b *Robot, at V2) {
	if b.ZapT > 0 || !b.Alive() {
		return
	}
	b.ZapT = 0.5
	r.Stats.Wires++
	r.emit(EvWireZap, at, b.ID, 0, "")
	if b.Kind == Heavy {
		b.HP -= 18
		if b.HP <= 0 {
			r.kill(b, "wire")
		}
		return
	}
	r.kill(b, "wire")
}

// wreck: a burning walker smashes through wall w at p, taking a wreckFt bite out of it.
// The points the bite was worth go back into your pool, so you can patch the hole.
func (r *Round) wreck(w *MazeWall, p V2) {
	half := r.T.WreckFt / FtPerM / 2
	L := Dist(w.A, w.B)
	_, u := SegDist(p, w.A, w.B)
	c := u * L
	r.removeWall(w.ID, 1)
	r.emit(EvBurn, p, w.ID, 0, "")
	keep := func(a, b V2) {
		r.addWall(a, b, w.Super)
		cost := r.WallCost(a, b)
		r.Points -= cost
		r.mazeCost += cost
	}
	if c-half > 0.3 {
		keep(w.A, Lerp(w.A, w.B, (c-half)/L))
	}
	if L-(c+half) > 0.3 {
		keep(Lerp(w.A, w.B, (c+half)/L), w.B)
	}
	r.nav.SetMaze(r.mazeSegs())
}

// BurnDeathWalker, BurnDeathHeavy: the chance burning through a magenta wall kills it
const (
	BurnDeathWalker = 0.75
	BurnDeathHeavy  = 0.35
)

// ---- the device ----

func (r *Round) steal(b *Robot) {
	d := &r.Dev
	// your hands on it don't stop them: they wrench it off you (yank it back, or harpoon them)
	if d.State != DevHome && d.State != DevDropped || d.State == DevHome && d.Field > 0 {
		return
	}
	if d.State == DevDropped && r.Harp.Beam {
		return // it's in the tractor beam
	}
	d.State, d.Carrier, d.Grabber, d.Held = DevStolen, b.ID, 0, false
	b.State = RCarry
	r.pullLeads()
	r.Stats.Steals++
	r.emit(EvStolen, d.Pos, b.ID, 0, "")
	// out by whichever door is the shortest walk (they follow your maze with it)
	best, bl := -1, math.Inf(1)
	var bp []V2
	for i, door := range r.Doors {
		p, L, ok := r.nav.Path(b.Pos, door.Mouth, b.Radius(), false)
		if !ok {
			p, L, ok = r.nav.Path(b.Pos, door.Mouth, b.Radius(), true)
		}
		if ok && L < bl {
			best, bl, bp = i, L, p
		}
	}
	if best < 0 {
		best = 0
		bp = []V2{r.Doors[0].Mouth}
	}
	b.Door = best
	b.Path = bp
}

// Steal: robot b takes the device right now, field or no field (tests and practice).
func (r *Round) Steal(b *Robot) {
	r.Dev.Field = 0
	r.steal(b)
}

func (r *Round) carryDevice(b *Robot) {
	d := &r.Dev
	f := V(math.Cos(b.Yaw), math.Sin(b.Yaw))
	d.Pos = b.Pos.Add(f.Mul(b.Radius() + 0.12))
	d.Yaw = b.Yaw
	// brushing past the maze with an armful of electronics
	for _, w := range r.Maze {
		if dd, _ := SegDist(b.Pos, w.A, w.B); dd < b.Radius()+0.35 {
			if w.Super > 0 || r.rng.Float64() < r.T.FryPerSec*frameDT(r) {
				r.kill(b, "magenta")
			}
			break
		}
	}
}

// frameDT: the step length (tracked so per-second chances work at any step size)
func frameDT(r *Round) float64 { return r.lastDT }

func (r *Round) stepDevice(dt float64) {
	d := &r.Dev
	switch d.State {
	case DevHome:
		safe := len(r.RingPoly) >= 3 && InPoly(r.RingPoly, d.Pos)
		if !safe && !d.Exposed {
			r.emit(EvExposed, d.Pos, 0, 0, "")
		}
		d.Exposed = !safe
		// the device's own backup field buys you a few seconds to drag it back in
		if safe {
			d.Field = math.Min(r.T.FieldMax(r.Machines()), d.Field+r.T.FieldCharge*dt)
		} else if d.Field > 0 {
			if d.Field -= dt; d.Field <= 0 {
				d.Field = 0
				r.emit(EvFieldDown, d.Pos, 0, 0, "")
			}
		}
		if d.Exposed && d.Grabber == 0 {
			r.sendGrabber()
		}
	case DevDropped:
		if len(r.RingPoly) >= 3 && InPoly(r.RingPoly, d.Pos) {
			d.State, d.Exposed = DevHome, false
			r.emit(EvRecovered, d.Pos, 0, 0, "")
			return
		}
		d.DropT -= dt
		if d.DropT <= 0 && d.Grabber == 0 && !d.Held {
			r.sendGrabber()
		}
	}
	// harpoon milestones
	tot := r.WorkTotal()
	for i, f := range r.T.HarpoonAt {
		if !d.armed[i] && r.Work() >= f*tot-1e-9 {
			d.armed[i] = true
			r.armHarpoon()
		}
	}
}

func (r *Round) sendGrabber() {
	d := &r.Dev
	var best *Robot
	bd := math.Inf(1)
	for _, b := range r.Robots {
		if !b.Alive() || b.Kind == Drone || b.State == RHooked || b.State == RCarry || b.Dazed > 0 {
			continue
		}
		if dd := Dist(b.Pos, d.Pos); dd < bd {
			bd, best = dd, b
		}
	}
	if best == nil {
		return
	}
	d.Grabber = best.ID
	best.State, best.SpanA, best.SpanB, best.replan = RGrab, -1, -1, 0
}

// Work: seconds of device work done so far; WorkTotal: all of it.
func (r *Round) Work() float64 {
	w := 0.0
	for i, m := range r.Dev.Mods {
		if m.Done {
			w += m.Secs
		} else if i == r.Dev.Cur {
			w += m.Secs * r.Dev.Prog
		}
	}
	return w
}

func (r *Round) WorkTotal() float64 {
	w := 0.0
	for _, m := range r.Dev.Mods {
		w += m.Secs
	}
	return w
}

// Module: the device module being worked on (nil once it has fired).
func (r *Round) Module() *Module {
	if r.Dev.Cur >= len(r.Dev.Mods) {
		return nil
	}
	return &r.Dev.Mods[r.Dev.Cur]
}

// SetModuleProgress: how far through the current module the hands-on work is (0..1).
func (r *Round) SetModuleProgress(p float64) {
	if r.Phase == Playing && r.Dev.State == DevHome {
		r.Dev.Prog = clamp(p, 0, 0.999)
	}
}

// CompleteModule: the current module is done. The last one fires the device: you win.
func (r *Round) CompleteModule() {
	d := &r.Dev
	if r.Phase != Playing || d.State != DevHome || d.Cur >= len(d.Mods) {
		return
	}
	if d.Cur == len(d.Mods)-1 && d.Leads < LeadsNeeded {
		r.emit(EvNoPower, d.Pos, d.Leads, 0, "")
		return // no blast without power
	}
	if d.Cur == len(d.Mods)-1 && d.Misfire {
		r.emit(EvMisfire, d.Pos, d.Misfires, 1, "")
		return
	}
	if d.Cur == len(d.Mods)-1 && d.Misfires < 2 && r.rng.Float64() < []float64{r.T.Misfire, r.T.Misfire2}[d.Misfires] {
		// it spins up... and nothing
		d.Misfire, d.Prog = true, 0
		d.Misfires++
		r.emit(EvMisfire, d.Pos, d.Misfires, 0, "")
		// a weak pulse is all it puts out: the wardens near it stagger about for a while
		r.dazeAround(d.Pos, 0, r.T.MisfireDaze, MisfireReach)
		return
	}
	d.Mods[d.Cur].Done = true
	r.emit(EvModule, d.Pos, d.Cur, 0, d.Mods[d.Cur].ID)
	d.Cur++
	d.Prog = 0
	if d.Cur >= len(d.Mods) {
		d.Fired = true
		r.Phase = Won
		for _, b := range r.Robots {
			r.kill(b, "blast")
		}
		r.emit(EvWon, d.Pos, 0, 0, "")
	}
}

// Setback: a botched device game (or the device wrenched out of a warden's hands) costs
// you this many seconds of work - but only on the stage you're on: you never lose a stage
// you'd already finished. A game you botch comes back as a different game. why says what
// did it ("simon", "needle", "tug", "bot").
func (r *Round) Setback(secs float64, why string) {
	d := &r.Dev
	start := 0.0
	for i := 0; i < d.Cur && i < len(d.Mods); i++ {
		start += d.Mods[i].Secs
	}
	target := math.Max(start, r.Work()-secs)
	acc := 0.0
	for i := range d.Mods {
		m := &d.Mods[i]
		if acc+m.Secs > target+1e-9 {
			for j := i; j < len(d.Mods); j++ {
				mj := &d.Mods[j]
				if mj.Done && mj.ID == "game" && len(r.C.Games) > 1 {
					old := mj.Game
					for mj.Game == old {
						mj.Game = r.C.Games[r.rng.Intn(len(r.C.Games))]
					}
				}
				mj.Done = false
			}
			if d.Mods[i].ID == "game" && len(r.C.Games) > 1 && i == d.Cur {
				old := m.Game
				for m.Game == old {
					m.Game = r.C.Games[r.rng.Intn(len(r.C.Games))]
				}
			}
			d.Cur = i
			d.Prog = (target - acc) / m.Secs
			r.emit(EvSetback, d.Pos, i, 0, why)
			return
		}
		acc += m.Secs
	}
}

// SetDevice: you're carrying (or let go of) the device. Position is where it sits on the floor.
func (r *Round) SetDevice(held bool, p V2, yaw float64) {
	d := &r.Dev
	switch d.State {
	case DevHome:
		d.Held = held
		if held {
			d.Pos, d.Yaw = p, yaw
		}
	case DevDropped:
		if held {
			d.State, d.Held, d.Pos, d.Yaw = DevHome, true, p, yaw
			d.Grabber = 0
			r.Stats.Saves++
			r.emit(EvRecovered, p, 0, 0, "")
		}
	default:
		d.Held = false
	}
}

// Power leads.
const (
	LeadsNeeded = 2
	LeadReach   = 2.4 // how far from its box the cart can be with the leads still in (m)
)

// leadBoxSpot: where the leads' box sits on the floor - just behind the cart where it
// starts, or beside or in front of it if that's out of the room.
func (r *Round) leadBoxSpot() V2 {
	d := &r.Dev
	f := V(math.Cos(d.Yaw), math.Sin(d.Yaw))
	s := V(-f.Z, f.X)
	for _, dir := range []V2{f.Mul(-1), s, s.Mul(-1), f} {
		p := d.Pos.Add(dir.Mul(0.75))
		if InPoly(r.Room, p) && EdgeDist(r.Room, p) > 0.2 {
			return p
		}
	}
	return d.Pos.Add(f.Mul(-0.5))
}

// MisfireReach: how far a misfire's weak pulse reaches (m)
const MisfireReach = 6.0

// FixMisfire: the guts are fixed and closed up again: the cord will work.
func (r *Round) FixMisfire() {
	if d := &r.Dev; d.Misfire {
		// the cord's still most of the way wound: a few good pulls
		d.Misfire, d.Prog = false, 0.75
		r.emit(EvFixed, d.Pos, 0, 0, "")
	}
}

// SetLeads: how many power leads are plugged into the cart (the headset side reports it).
func (r *Round) SetLeads(n int) {
	d := &r.Dev
	if d.State != DevHome || Dist(d.Pos, d.LeadBox) > LeadReach {
		n = 0
	}
	d.Leads = max(0, min(LeadsNeeded, n))
}

// pullLeads: the leads come out of the cart.
func (r *Round) pullLeads() {
	if d := &r.Dev; d.Leads > 0 {
		r.emit(EvLeadsOut, d.Pos, d.Leads, 0, "")
		d.Leads = 0
	}
}

// TugGrace: seconds of field the device gets back when you yank it off a warden.
const TugGrace = 3.0

// Tug: you yank the device while a warden has it. Returns whether it came loose.
func (r *Round) Tug() bool {
	d := &r.Dev
	if d.State != DevStolen {
		return false
	}
	b := r.robot(d.Carrier)
	if b == nil {
		return false
	}
	ch := r.T.TugWalker
	if b.Kind == Heavy {
		ch = r.T.TugHeavy
	}
	if r.rng.Float64() >= ch {
		r.emit(EvTugFail, d.Pos, b.ID, 0, "")
		return false
	}
	d.State, d.Carrier, d.Held = DevHome, 0, true
	b.State, b.Wait = RWait, 3
	// wrenched back: its field flickers up again, a moment to drag it home before they regrab
	d.Field = math.Max(d.Field, TugGrace)
	r.Stats.Saves++
	r.Setback(r.T.TugCost, "tug")
	r.emit(EvRecovered, d.Pos, b.ID, 1, "")
	return true
}

// ---- generators ----

func (r *Round) workOrder() []string {
	steps := r.C.RepairSteps[r.rng.Intn(len(r.C.RepairSteps))]
	var out []string
	used := map[string]bool{}
	for len(out) < steps && len(out) < len(r.C.RepairTasks) {
		t := r.C.RepairTasks[r.rng.Intn(len(r.C.RepairTasks))].ID
		if !used[t] {
			used[t] = true
			out = append(out, t)
		}
	}
	return out
}

// SetPower: throw generator i's breaker lever (off shrinks your ring on purpose).
func (r *Round) SetPower(i int, on bool) bool {
	if i < 0 || i >= len(r.Gens) {
		return false
	}
	i, g := r.lead(i)
	defer r.mirrorGens()
	switch {
	case on && g.State == GenOff && g.Charge > 0:
		g.State = GenUp
		r.emit(EvGenUp, g.Pos, i, 0, "")
	case !on && g.State == GenUp:
		g.State = GenOff
		r.emit(EvGenOff, g.Pos, i, 0, "")
	default:
		return false
	}
	if r.Phase == Playing || r.Phase == Setup {
		r.updateRing()
	}
	return true
}

// RepairTask: one task of a blown generator's work order is done.
func (r *Round) RepairTask(i int) {
	i, g := r.lead(i)
	if g.State != GenBlown {
		return
	}
	defer r.mirrorGens()
	g.Done++
	r.emit(EvTask, g.Pos, i, g.Done, "")
	if g.Done >= len(g.Order) {
		g.State, g.Charge, g.Order, g.Done = GenUp, r.T.RepairFrac*r.T.ChargeMax, nil, 0
		r.emit(EvGenUp, g.Pos, i, 1, "")
		r.mirrorGens()
		r.updateRing()
	}
}

// Pump: one stroke of generator i's charge handle. Filling it to the top shoves the wardens
// leaning on it back.
func (r *Round) Pump(i int) {
	i, g := r.lead(i)
	if g.State != GenUp && g.State != GenOff {
		return
	}
	defer r.mirrorGens()
	was := g.Charge
	g.Charge = math.Min(r.T.ChargeMax, g.Charge+r.T.PumpStroke*r.T.ChargeMax)
	r.emit(EvPump, g.Pos, i, 0, "")
	if was < r.T.ChargeMax-1e-6 && g.Charge >= r.T.ChargeMax-1e-6 && g.State == GenUp {
		for _, b := range r.Robots {
			if b.Alive() && b.State == RPress && (r.Gens[b.SpanA].Lead == i || r.Gens[b.SpanB].Lead == i) {
				b.State, b.Wait = RWait, r.T.Knockback
				b.Pos = b.Pos.Add(b.Pos.Sub(g.Pos).Norm().Mul(0.3))
			}
		}
		r.emit(EvKnock, g.Pos, i, 0, "")
	}
}

// Pluck: you ripped a leech drone off.
// Unhook: you yanked a drone's tether off its generator. The generator's free again, and
// the drone limps off outside your ring to patch itself up (RecoverSecs) before it comes
// back. (Pluck - smashing it - is for good.)
func (r *Round) Unhook(id int) {
	b := r.robot(id)
	if b == nil || b.State != RAttached || b.Wait < 0 {
		return
	}
	g := &r.Gens[b.Gen]
	if g.Leech == b.ID {
		g.Leech = 0
		if g.State == GenLeeched {
			g.State = GenUp
			if g.Charge <= 0 {
				g.State = GenBlown
				g.Order = r.workOrder()
			}
		}
	}
	r.mirrorGens()
	b.Pos = r.outsideSpot(g.Pos, 1.1) // out past your real walls
	b.State, b.Wait, b.Path = RRecover, RecoverSecs, nil
	r.emit(EvUnhook, g.Pos, b.Gen, b.ID, "")
	r.updateRing()
}

// Smash: a warden, heavy or drone clubbed hard with the pole.
func (r *Round) Smash(id int) bool {
	b := r.robot(id)
	if b == nil || !b.Alive() {
		return false
	}
	attached := b.State == RAttached
	r.kill(b, "pole")
	if attached {
		r.updateRing()
	}
	return true
}

func (r *Round) Pluck(id int) {
	if b := r.robot(id); b != nil && b.State == RAttached && b.Wait >= 0 { // not mid-hop
		r.kill(b, "pluck")
		r.updateRing()
	}
}

// ---- the maze ----

func (r *Round) mazeSegs() []Seg {
	out := make([]Seg, len(r.Maze))
	for i, w := range r.Maze {
		out[i] = Seg{w.A, w.B}
	}
	return out
}

func (r *Round) addWall(a, b V2, super float64) *MazeWall {
	r.nextID++
	w := &MazeWall{ID: r.nextID, A: a, B: b, Super: super}
	r.Maze = append(r.Maze, w)
	return w
}

func (r *Round) removeWall(id int, refund float64) {
	for i, w := range r.Maze {
		if w.ID == id {
			r.Maze = append(r.Maze[:i], r.Maze[i+1:]...)
			cost := Dist(w.A, w.B) * r.T.MazeCost()
			r.Points += cost * refund
			r.mazeCost -= cost * refund // what's not refunded stays spent
			r.emit(EvWallGone, Lerp(w.A, w.B, 0.5), id, 0, "")
			return
		}
	}
}

// WallCost: what a magenta wall from a to b costs.
func (r *Round) WallCost(a, b V2) float64 { return Dist(a, b) * r.T.MazeCost() }

// CheckWall: can a magenta wall go from a to b? (Quick checks only: no route test.)
func (r *Round) CheckWall(a, b V2) (float64, string) {
	cost := r.WallCost(a, b)
	if r.Phase != Setup && r.Phase != Playing {
		return cost, "ROUND OVER"
	}
	if Dist(a, b) < r.T.MinWall {
		return cost, "TOO SHORT"
	}
	if !r.Hall.Inside(a, 0.5) || !r.Hall.Inside(b, 0.5) {
		return cost, "TOO CLOSE TO THE HALL WALL"
	}
	if InPoly(r.Room, a) || InPoly(r.Room, b) {
		return cost, "NO MAZE INSIDE YOUR ROOM"
	}
	for i := range r.Room {
		if SegSegDist(a, b, r.Room[i], r.Room[(i+1)%len(r.Room)]) < r.T.RoomGap {
			return cost, "TOO CLOSE TO YOUR ROOM"
		}
	}
	for _, d := range r.Doors {
		if dd, _ := SegDist(d.Mouth, a, b); dd < 1.1 {
			return cost, "KEEP THE DOORWAY CLEAR"
		}
	}
	if cost > r.Points+1e-6 {
		return cost, "NOT ENOUGH POINTS"
	}
	return cost, ""
}

// PlaceWall: build a magenta wall, as long as every door can still reach your ring.
func (r *Round) PlaceWall(a, b V2) (int, string) {
	cost, why := r.CheckWall(a, b)
	if why != "" {
		return 0, why
	}
	w := r.addWall(a, b, 0)
	r.nav.SetMaze(r.mazeSegs())
	if !r.routesOpen() {
		r.Maze = r.Maze[:len(r.Maze)-1]
		r.nav.SetMaze(r.mazeSegs())
		return 0, "THAT SEALS A DOOR OFF"
	}
	r.Points -= cost
	r.mazeCost += cost
	// wardens on their way re-route around it (a burning one just keeps going)
	for _, o := range r.Robots {
		if o.Alive() && (o.State == RWalk || o.State == RGrab || o.State == RCarry || o.State == RLurk) && !o.Burn {
			prev := o.Pos
			for _, p := range o.Path {
				if _, _, hit := SegHit(prev, p, a, b); hit {
					if o.State == RLurk {
						o.replan = 0
					} else if o.State == RWalk && o.Kind != Drone {
						r.planWalker(o, false)
					} else if o.Kind == Drone {
						r.planDrone(o)
					} else {
						o.replan = 0
						if o.State == RCarry {
							r.reroute(o)
						}
					}
					break
				}
				prev = p
			}
		}
	}
	return w.ID, ""
}

func (r *Round) reroute(b *Robot) {
	if p, _, ok := r.nav.Path(b.Pos, r.Doors[b.Door].Mouth, b.Radius(), false); ok {
		b.Path = p
	}
}

// RemoveWall: take a magenta wall down (a full refund before the round, half during it).
// Wall: the magenta wall with this id (nil if it's gone).
func (r *Round) Wall(id int) *MazeWall {
	for _, w := range r.Maze {
		if w.ID == id {
			return w
		}
	}
	return nil
}

// RemoveWall: take a magenta wall down; its points go back into your pool.
func (r *Round) RemoveWall(id int) {
	r.removeWall(id, 1)
	r.nav.SetMaze(r.mazeSegs())
}

// routesOpen: every door still has a walkable way to the ring.
func (r *Round) routesOpen() bool {
	if len(r.RingPoly) < 3 {
		return true
	}
	c := Centroid(r.RingPoly)
	for _, d := range r.Doors {
		// aim at the ring: the free spot nearest its middle is just outside it
		if !r.nav.Reachable(d.Mouth, c, 0.35, false) {
			return false
		}
	}
	return true
}

// MazeSpent: points spent on the maze so far.
func (r *Round) MazeSpent() float64 { return r.mazeCost }

// ---- supercharge ----

// SuperSpan / SuperWall: overcharge a green span or a magenta wall with your laser. Anything
// touching it fries for a few seconds; then there's a cooldown.
func (r *Round) SuperSpan(a, b int) bool {
	if r.Phase != Playing || r.SuperCD > 0 || !r.spanValid(a, b) {
		return false
	}
	r.SuperRing[[2]int{a, b}] = r.T.SuperSecs
	r.SuperCD = r.T.SuperCool
	r.emit(EvSuper, Lerp(r.Gens[a].Pos, r.Gens[b].Pos, 0.5), 0, a, "")
	return true
}

func (r *Round) SuperWall(id int) bool {
	if r.Phase != Playing || r.SuperCD > 0 {
		return false
	}
	for _, w := range r.Maze {
		if w.ID == id {
			w.Super = r.T.SuperSecs
			r.SuperCD = r.T.SuperCool
			r.emit(EvSuper, Lerp(w.A, w.B, 0.5), 1, id, "")
			return true
		}
	}
	return false
}

// A supercharged wall's blast: right up against it a warden fries outright; further out it
// takes damage that falls off with distance (it may limp on, but with little power left
// to drain your ring). Leeches hovering over its generators drop.
const (
	SuperKill  = 0.25 // within this of its body, it's fried
	SuperReach = 1.6  // ...and out to this it takes damage
	SuperDmg   = 9.0  // power a second at the near edge of the reach (heavies take it too)
)

// HotSegs: the supercharged walls right now (green spans and magenta walls).
func (r *Round) HotSegs() []Seg {
	var out []Seg
	for _, k := range r.superKeys() {
		out = append(out, Seg{r.Gens[k[0]].Pos, r.Gens[k[1]].Pos})
	}
	for _, w := range r.Maze {
		if w.Super > 0 {
			out = append(out, Seg{w.A, w.B})
		}
	}
	return out
}

// SuperDist: how far b's body is from the nearest supercharged wall (and whether any is in
// reach).
func (r *Round) SuperDist(b *Robot) (float64, bool) {
	best := math.Inf(1)
	for _, s := range r.HotSegs() {
		if d, _ := SegDist(b.Pos, s.A, s.B); d-b.Radius() < best {
			best = d - b.Radius()
		}
	}
	return math.Max(0, best), best < SuperReach
}

func (r *Round) superBlast(dt float64) {
	if len(r.SuperRing) == 0 {
		hot := false
		for _, w := range r.Maze {
			hot = hot || w.Super > 0
		}
		if !hot {
			return
		}
	}
	for _, b := range r.Robots {
		if !b.Alive() {
			continue
		}
		d, in := r.SuperDist(b)
		if !in {
			b.Expo = math.Max(0, b.Expo-dt)
			continue
		}
		// the longer it hangs about in the blast, the harder it bites
		b.Expo += dt
		ramp := 1 + b.Expo/3
		wasLeech := b.State == RAttached
		switch {
		case b.Kind == Drone || d < SuperKill && b.Kind != Heavy:
			r.kill(b, "super")
		case d < SuperKill:
			b.HP -= 4 * SuperDmg * ramp * dt
		default:
			b.HP -= SuperDmg * ramp * (1 - (d-SuperKill)/(SuperReach-SuperKill)) * dt
		}
		if b.Alive() && b.HP <= 0 {
			r.kill(b, "super")
		}
		if wasLeech && !b.Alive() {
			r.updateRing()
		}
	}
}

// SpanHot: seconds of supercharge left on span a-b.
func (r *Round) SpanHot(a, b int) float64 { return r.SuperRing[[2]int{a, b}] }
