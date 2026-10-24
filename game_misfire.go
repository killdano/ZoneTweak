package main

// The misfire. Now and then a good pull of the cord doesn't set the EMP off: its guts need
// fixing (and once in a while, after that, it misfires again). Four screws or bolts hold the
// service hatch on the back half of the cart's top; with them out, the handle in its middle
// lifts the guts up out of the cart - three layers of boards with a mess of wires, thick
// cables, fuses, circuit cards, relays, capacitors and jumpers on them. Nothing about the
// fault shows unless you know where to look: your contact on the radio tells you what to do -
// one or two of: cut a wire, cut a cable and patch it, swap a fuse, the burnt card, the
// cracked relay or the bulging capacitor, pull a jumper. Which one, and where it sits, is
// different every time. The wire cutters and every spare are always on the cart's bottom
// shelf. Do the wrong thing and he starts you over on a different fix (the guts are
// different too). When it's done, push the handle back down and pull the cord again.

import (
	"boundaryvr/sim"
	"math"
	"math/rand"
	"sort"
	"strings"
)

const (
	mfHX, mfHZ = 0.2, 0.1 // the hatch's half size (across, along the cart)
	mfZ        = -0.21    // its middle, along the cart (the back of the top)
	mfBX, mfBZ = 0.19, 0.09
	mfLayers   = 3
	mfRise     = 0.456 // how far the lid comes up
	cutReach   = 0.035 // how close the cutters' jaws have to be to a wire
)

var mfColNames = []string{"red", "blue", "yellow", "green", "white"}
var mfCols = []rgba{{0.85, 0.1, 0.08, 1}, {0.12, 0.3, 0.95, 1}, {0.98, 0.82, 0.08, 1}, {0.12, 0.72, 0.22, 1}, {0.9, 0.9, 0.86, 1}}
var mfCableNames = []string{"orange", "grey", "black", "purple"}
var mfCableCols = []rgba{{0.92, 0.42, 0.04, 1}, {0.42, 0.44, 0.47, 1}, {0.05, 0.05, 0.06, 1}, {0.45, 0.15, 0.6, 1}}
var mfFuseNames = []string{"red", "blue", "green", "yellow", "white", "orange"}
var mfFuseCols = []rgba{{0.85, 0.1, 0.08, 1}, {0.12, 0.3, 0.95, 1}, {0.12, 0.72, 0.22, 1}, {0.98, 0.82, 0.08, 1}, {0.92, 0.92, 0.9, 1}, {0.95, 0.45, 0.05, 1}}
var mfJumperNames = []string{"red", "blue", "green", "yellow"}

// the fixes
const (
	ordWire   = iota // cut a wire
	ordCable         // cut a thick cable and patch it
	ordFuse          // swap a fuse
	ordCard          // swap the burnt card
	ordRelay         // swap the cracked relay
	ordCap           // swap the bulging capacitor
	ordJumper        // pull a jumper
	nOrders
)

// the parts that sit in sockets
const (
	partFuse = iota
	partCard
	partRelay
	partCap
	partJumper
	partPatch // the patch cable (only ever a spare)
)

// how many of each part are on the boards
var mfCount = map[int]int{partFuse: len(mfFuseNames), partCard: 6, partRelay: 4, partCap: 4, partJumper: len(mfJumperNames)}

// mfPt: a point in the guts: on layer k (-1: down in the cart, mfLayers: the lid) and f of the
// way up to the next one, x across and z along from the hatch's middle, up off the surface.
type mfPt struct {
	k       int
	f, x, z float64
	up      float64
}

type mfWire struct {
	pts     []mfPt
	a, b    int // colour; b != a: a with a b stripe (cables: a is the cable colour)
	thick   bool
	cut     bool
	cutU    float64
	patched bool
	world   []vec3 // this frame's path
}

type mfSlot struct {
	kind  int
	col   int // fuses and jumpers: the colour that sits here
	at    mfPt
	holds *mfPart
}

type mfPart struct {
	kind   int
	col    int
	faulty bool // the one the fix is about (blown, burnt, cracked, bulging, the jumper to pull)
	spare  bool
	slot   *mfSlot
	patch  *mfWire // the patch cable: the cut it's bridging
	p      *prop
	seed   int64
	shelf  vec3 // spares: where on the shelf (cart frame)
}

type mfDeco struct {
	at   mfPt
	kind int
	col  rgba
}

type mfOrder struct {
	kind, i int // i: the wire, the colour (fuse, jumper) or the slot (card, relay, capacitor)
	done    bool
}

type mfKit struct {
	g      *game
	rng    *rand.Rand
	kind   int // fastScrew / fastBolt on the hatch's corners
	fs     *fastSet
	lift   float64 // 0 shut .. 1 all the way up
	handle *mfHandle
	// the guts
	wires  []*mfWire
	slots  []*mfSlot
	parts  []*mfPart // the ones on the boards
	deco   []mfDeco
	orders []mfOrder
	heard  bool    // you've had the fix over the radio
	heardT float64 // when (the game's clock)
	helped bool    // he's called back with what to look for
	raised bool    // the guts have been all the way up since this fix began
	told   bool    // "push it back down" said
	reset  bool    // a wrong move: the guts are sinking back in to be shuffled
	// the shelf
	cutters *prop
	spares  []*mfPart
}

// ---- where things are ----

func (k *mfKit) F() xf { return k.g.devFrame() }

// layerY: the height of layer i (-1: down in the cart; mfLayers: the lid) at this lift.
func (k *mfKit) layerY(i int) float64 {
	switch {
	case i < 0:
		return devTop - 0.05
	case i >= mfLayers:
		return devTop + 0.004 + mfRise*k.lift
	}
	shut := devTop - 0.035 + 0.01*float64(i)
	open := devTop + 0.03 + 0.14*float64(i)
	return shut + (open-shut)*k.lift
}

func (k *mfKit) world(p mfPt) vec3 {
	y0, y1 := k.layerY(p.k), k.layerY(p.k+1)
	return k.F().p(p.x, y0+(y1-y0)*p.f+p.up, mfZ+p.z)
}

func (k *mfKit) handlePos() vec3 { return k.F().p(0, k.layerY(mfLayers)+0.055, mfZ) }

func (k *mfKit) open() bool { return k.lift > 0.9 }

// slotPose: a part sitting in slot s.
func (k *mfKit) slotPose(s *mfSlot) (vec3, vec3, vec3, vec3) {
	F := k.F()
	p := k.world(s.at)
	switch s.kind {
	case partFuse: // lying across the cart in its clip
		return p.add(F.y.mul(0.014)), F.z, F.y, F.x.mul(-1)
	case partCard: // standing, its face across the cart
		return p.add(F.y.mul(0.028)), F.x, F.y, F.z
	case partRelay:
		return p.add(F.y.mul(0.014)), F.x, F.y, F.z
	case partCap:
		return p.add(F.y.mul(0.003)), F.x, F.y, F.z
	}
	return p.add(F.y.mul(0.009)), F.x, F.y, F.z // a jumper on its pins
}

// shelfPose: a spare in its place on the cart's bottom shelf.
func (k *mfKit) shelfPose(pt *mfPart) (vec3, vec3, vec3, vec3) {
	F := k.F()
	p := F.p(pt.shelf.x, pt.shelf.y, pt.shelf.z)
	switch pt.kind {
	case partFuse:
		return p, F.x, F.y, F.z
	case partCard: // lying flat
		return p, F.y.mul(-1), F.x, F.z
	case partPatch:
		return p, F.x, F.y, F.z
	}
	return p, F.x, F.y, F.z
}

// patchPose: the patch cable bridging a cut cable.
func (k *mfKit) patchPose(w *mfWire) (vec3, vec3, vec3, vec3) {
	F := k.F()
	a, b := w.cutEnds()
	m := a.add(b).mul(0.5)
	z := b.sub(a)
	if z.length() < 1e-4 {
		z = F.x
	}
	z = z.norm()
	x := F.y.cross(z)
	if x.length() < 0.1 {
		x = F.x.cross(z)
	}
	x = x.norm()
	return m, x, z.cross(x), z
}

// patchSnap: let go of the patch cable this near a cut cable and it clamps across it.
const patchSnap = 0.12

// needsPatch: a cable the fix wants patched that's been cut and not patched yet (nil: none).
func (k *mfKit) needsPatch() *mfWire {
	for _, o := range k.orders {
		if o.kind == ordCable && o.i < len(k.wires) {
			if w := k.wires[o.i]; w.cut && !w.patched {
				return w
			}
		}
	}
	return nil
}

// patchSpare: the patch cable from the tray.
func (k *mfKit) patchSpare() *mfPart {
	for _, pt := range k.spares {
		if pt.kind == partPatch {
			return pt
		}
	}
	return nil
}

// drawPatchHint: a cut cable waiting for its patch - the cut ends spit and glow yellow with a
// ghost of the patch across them, the patch cable on the tray glows too, and once it's in your
// hand a line runs from it to the cut ("LET GO" when it's near enough to clamp on).
func (k *mfKit) drawPatchHint(t float64) {
	w := k.needsPatch()
	if w == nil || !k.open() || k.reset {
		return
	}
	pt := k.patchSpare()
	if pt == nil {
		return
	}
	yel := rgba{1, 0.85, 0.15, 1}
	pulse := float32(0.55 + 0.45*math.Sin(t*6))
	a, b := w.cutEnds()
	m := a.add(b).mul(0.5)
	d := b.sub(a)
	ga, gb := a.sub(d.mul(0.6)), b.add(d.mul(0.6))
	held := k.g.handHolding(pt.p) != nil
	near := held && pt.p.pos.sub(m).length() < patchSnap
	ghost := yel.alpha(0.35 * pulse)
	if near {
		ghost = yel.alpha(0.9)
	}
	up := k.F().y
	bow := up.mul(0.03)
	const n = 10
	prev := ga
	for i := 1; i <= n; i++ {
		q := bezier(ga, ga.add(gb.sub(ga).mul(0.3)).add(bow), ga.add(gb.sub(ga).mul(0.7)).add(bow), gb, float64(i)/n)
		glowLine(prev, q, ghost, 0.5)
		prev = q
	}
	for _, e := range []vec3{a, b} {
		sprite(e, 0.025, yel.alpha(0.8*pulse))
		if rand.Float64() < 0.04 {
			spawnSparks(e, up, 2, 0.15)
		}
	}
	ringSoftAxes(m, k.F().x, k.F().z, patchSnap*0.6, 0.004, yel.alpha(0.5*pulse), 28)
	cam := ed.hmd.pos()
	lp := m.add(up.mul(0.07))
	rr, uu := billboard(cam, lp)
	label := "PATCH CABLE HERE"
	if near {
		label = "LET GO"
	}
	drawTextCentered(&bOpaque, label, lp, rr, uu, 0.0008, yel)
	if held {
		if !near {
			glowLine(pt.p.pos, m, yel.alpha(0.4+0.3*pulse), 0.3)
		}
		return
	}
	// on the tray: look here
	sp := pt.p.pos
	sprite(sp, 0.09, yel.alpha(0.35*pulse))
	tp := sp.add(up.mul(0.06))
	rr, uu = billboard(cam, tp)
	drawTextCentered(&bOpaque, "PATCH CABLE", tp, rr, uu, 0.0007, yel)
}

// cutEnds: the two ends of a cut wire.
func (w *mfWire) cutEnds() (vec3, vec3) {
	n := len(w.world) - 1
	if n < 1 {
		return vec3{}, vec3{}
	}
	i := min(n-1, max(0, int(w.cutU*float64(n))))
	return w.world[i], w.world[i+1]
}

// ---- building ----

func (g *game) buildMisfire() {
	k := &mfKit{g: g, rng: rand.New(rand.NewSource(g.seed*7919 + 5))}
	k.kind = fastScrew
	if k.rng.Intn(2) == 1 {
		k.kind = fastBolt
	}
	k.fs = newFastSet(g, k.kind, 4, func(i int) (vec3, vec3, vec3) {
		F := k.F()
		x := []float64{-1, 1, -1, 1}[i] * (mfHX - 0.025)
		z := mfZ + []float64{-1, -1, 1, 1}[i]*(mfHZ-0.02)
		return F.p(x, devTop+0.006, z), F.y, F.x
	})
	k.handle = &mfHandle{k: k}
	// the cutters, lying on the shelf
	k.cutters = &prop{radius: 0.08, hy: 0.01, homeR: 0.12}
	k.cutters.grip = &[4]vec3{v3(0, 0, 0), v3(1, 0, 0), v3(0, 1, 0), v3(0, 0, 1)}
	k.cutters.home = func() (vec3, vec3, vec3, vec3) {
		F := k.F()
		return F.p(0.15, 0.185, 0.18), F.y.cross(F.x).norm(), F.y, F.x
	}
	k.cutters.docked = true
	k.cutters.pos, k.cutters.X, k.cutters.Y, k.cutters.Z = k.cutters.home()
	// the spares: a fuse of every colour, a card, a relay, a capacitor, a patch cable
	for c := range mfFuseNames {
		k.addSpare(&mfPart{kind: partFuse, col: c, shelf: v3(-0.2+0.032*float64(c), 0.176, 0.24)})
	}
	k.addSpare(&mfPart{kind: partCard, seed: 99, shelf: v3(-0.12, 0.172, 0.02)})
	k.addSpare(&mfPart{kind: partRelay, seed: 98, shelf: v3(0.04, 0.182, 0.02)})
	k.addSpare(&mfPart{kind: partCap, seed: 97, shelf: v3(0.12, 0.17, 0.02)})
	k.addSpare(&mfPart{kind: partPatch, shelf: v3(0, 0.178, -0.2)})
	g.misfire = k
	k.generate(1)
}

func (k *mfKit) addSpare(pt *mfPart) {
	pt.spare = true
	k.newPartProp(pt)
	k.spares = append(k.spares, pt)
}

func (k *mfKit) newPartProp(pt *mfPart) {
	p := &prop{radius: 0.05, hy: 0.008, homeR: 0.08}
	if pt.kind == partPatch {
		p.radius, p.homeR = 0.09, 0.12
	}
	pt.p = p
	p.home = func() (vec3, vec3, vec3, vec3) {
		switch {
		case pt.slot != nil:
			return k.slotPose(pt.slot)
		case pt.patch != nil:
			return k.patchPose(pt.patch)
		case pt.spare:
			return k.shelfPose(pt)
		}
		return p.pos, p.X, p.Y, p.Z
	}
	if !pt.spare {
		p.homeR = 0
	}
	p.onRelease = func() { k.dropPart(pt) }
	p.docked = true
	p.pos, p.X, p.Y, p.Z = p.home()
}

// generate: a fresh set of guts and a fresh fix (n: how many things it takes; 0: one or two).
func (k *mfKit) generate(n int) {
	rng := k.rng
	// let go of anything off the old boards
	for _, pt := range k.parts {
		if h := k.g.handHolding(pt.p); h != nil {
			h.held = nil
		}
	}
	k.wires, k.slots, k.parts, k.deco = nil, nil, nil, nil
	// every spare back on the shelf (unless it's in a hand)
	for _, pt := range k.spares {
		pt.slot, pt.patch = nil, nil
		if k.g.handHolding(pt.p) == nil {
			pt.p.docked = true
		}
	}
	// a grid of spots on each layer: 5 across, 2 along
	type cell struct {
		k    int
		x, z float64
	}
	var cells []cell
	for l := 0; l < mfLayers; l++ {
		for i := 0; i < 5; i++ {
			for j := 0; j < 2; j++ {
				cells = append(cells, cell{l, -0.152 + 0.076*float64(i), -0.045 + 0.09*float64(j)})
			}
		}
	}
	rng.Shuffle(len(cells), func(i, j int) { cells[i], cells[j] = cells[j], cells[i] })
	for _, kind := range []int{partFuse, partCard, partRelay, partCap, partJumper} {
		for i := 0; i < mfCount[kind]; i++ {
			cl := cells[0]
			cells = cells[1:]
			s := &mfSlot{kind: kind, col: i, at: mfPt{k: cl.k, x: cl.x, z: cl.z, up: 0.003}}
			pt := &mfPart{kind: kind, col: i, slot: s, seed: rng.Int63()}
			s.holds = pt
			k.newPartProp(pt)
			k.slots = append(k.slots, s)
			k.parts = append(k.parts, pt)
		}
	}
	// the rest: chips, coils, heat sinks
	for _, cl := range cells {
		k.deco = append(k.deco, mfDeco{at: mfPt{k: cl.k, x: cl.x, z: cl.z, up: 0.003}, kind: rng.Intn(3),
			col: []rgba{{0.1, 0.1, 0.12, 1}, {0.15, 0.3, 0.65, 1}, {0.7, 0.45, 0.15, 1}}[rng.Intn(3)]})
	}
	// wires: seven, every one a different colour or colour-and-stripe; then the thick cables
	var combos [][2]int
	for a := range mfColNames {
		for b := a; b < len(mfColNames); b++ {
			combos = append(combos, [2]int{a, b})
		}
	}
	rng.Shuffle(len(combos), func(i, j int) { combos[i], combos[j] = combos[j], combos[i] })
	route := func() []mfPt {
		ka := rng.Intn(mfLayers+1) - 1
		kb := rng.Intn(mfLayers+1) - 1
		if kb < ka {
			ka, kb = kb, ka
		}
		pt := func(l int) mfPt {
			return mfPt{k: l, x: (rng.Float64()*2 - 1) * (mfBX - 0.02), z: (rng.Float64()*2 - 1) * (mfBZ - 0.015), up: 0.004}
		}
		a, b := pt(ka), pt(kb)
		if ka < 0 {
			a.up = 0
		}
		if kb < 0 {
			b.up = 0
		}
		ra, rb := a, b
		ra.f, rb.f = 0.25+0.35*rng.Float64(), 0.25+0.35*rng.Float64()
		ra.up, rb.up = 0, 0
		mid := pt(ka + rng.Intn(kb-ka+1))
		mid.f, mid.up = 0.3+0.5*rng.Float64(), 0
		// out past the edge now and then, looping round
		if rng.Intn(3) == 0 {
			mid.x = math.Copysign(mfBX+0.015, mid.x)
		}
		return []mfPt{a, ra, mid, rb, b}
	}
	for i := 0; i < 7; i++ {
		k.wires = append(k.wires, &mfWire{pts: route(), a: combos[i][0], b: combos[i][1]})
	}
	for i := range mfCableNames {
		k.wires = append(k.wires, &mfWire{pts: route(), a: i, b: i, thick: true})
	}
	// the fix: one or two different kinds of thing
	kinds := make([]int, nOrders)
	for i := range kinds {
		kinds[i] = i
	}
	rng.Shuffle(len(kinds), func(i, j int) { kinds[i], kinds[j] = kinds[j], kinds[i] })
	if n == 0 {
		n = 1
		if rng.Float64() < 0.45 {
			n = 2
		}
	}
	k.orders = nil
	pick := func(kind int) int {
		var c []int
		for i, s := range k.slots {
			if s.kind == kind {
				c = append(c, i)
			}
		}
		i := c[rng.Intn(len(c))]
		k.slots[i].holds.faulty = true
		return i
	}
	for _, kd := range kinds[:n] {
		o := mfOrder{kind: kd}
		switch kd {
		case ordWire:
			o.i = rng.Intn(7)
		case ordCable:
			o.i = 7 + rng.Intn(len(mfCableNames))
		case ordFuse:
			o.i = k.slots[pick(partFuse)].col
		case ordJumper:
			o.i = k.slots[pick(partJumper)].col
		case ordCard:
			o.i = pick(partCard)
		case ordRelay:
			o.i = pick(partRelay)
		case ordCap:
			o.i = pick(partCap)
		}
		k.orders = append(k.orders, o)
	}
	sort.Slice(k.orders, func(i, j int) bool { return k.orders[i].kind < k.orders[j].kind })
	k.heard, k.told, k.helped = false, false, false
}

// fuseSlot / jumperSlot: where the fuse or jumper of colour c sits.
func (k *mfKit) colSlot(kind, c int) *mfSlot {
	for _, s := range k.slots {
		if s.kind == kind && s.col == c {
			return s
		}
	}
	return nil
}

// ---- the orders ----

func (k *mfKit) wireName(w *mfWire) string {
	if w.thick {
		return mfCableNames[w.a]
	}
	if w.a == w.b {
		return mfColNames[w.a]
	}
	return mfColNames[w.a] + "_" + mfColNames[w.b]
}

func (k *mfKit) orderText(o mfOrder) string {
	switch o.kind {
	case ordWire:
		return "CUT THE " + strings.ToUpper(strings.ReplaceAll(k.wireName(k.wires[o.i]), "_", " AND ")) + " WIRE"
	case ordCable:
		return "CUT THE " + strings.ToUpper(k.wireName(k.wires[o.i])) + " CABLE, PATCH IT"
	case ordFuse:
		return "REPLACE THE " + strings.ToUpper(mfFuseNames[o.i]) + " FUSE"
	case ordJumper:
		return "PULL THE " + strings.ToUpper(mfJumperNames[o.i]) + " JUMPER"
	case ordRelay:
		return "REPLACE THE CRACKED RELAY"
	case ordCap:
		return "REPLACE THE BULGING CAPACITOR"
	}
	return "REPLACE THE BURNT CIRCUIT CARD"
}

func (k *mfKit) orderClip(o mfOrder) string {
	switch o.kind {
	case ordWire:
		return "mf_w_" + k.wireName(k.wires[o.i])
	case ordCable:
		return "mf_c_" + k.wireName(k.wires[o.i])
	case ordFuse:
		return "mf_f_" + mfFuseNames[o.i]
	case ordJumper:
		return "mf_j_" + mfJumperNames[o.i]
	case ordRelay:
		return "mf_relay"
	case ordCap:
		return "mf_cap"
	}
	return "mf_card"
}

// orderClips: what your contact says when you pick up: the fix, and where the cutters are if
// you need them.
func (k *mfKit) orderClips() []clipItem {
	var out []clipItem
	add := func(id string, gap float64) {
		if c := voiceClip(id); c != nil {
			out = append(out, clipItem{c, gap})
		}
	}
	add("mf_orders", 0.3)
	cut, n := false, 0
	for _, o := range k.orders {
		if o.done {
			continue
		}
		if n > 0 {
			add("mf_also", 0.2)
		}
		n++
		add(k.orderClip(o), 0.35)
		cut = cut || o.kind == ordWire || o.kind == ordCable
	}
	if cut {
		add("mf_cutters", 0.3)
	}
	// then, for whoever's still listening, what each thing looks like and how it goes
	add("mf_look", 0.3)
	return append(out, k.detailClips()...)
}

// detailClips: what to look for, and how, for each kind of thing still to do.
func (k *mfKit) detailClips() []clipItem {
	var out []clipItem
	seen := map[int]bool{}
	for _, o := range k.orders {
		if o.done || seen[o.kind] {
			continue
		}
		seen[o.kind] = true
		if c := voiceClip("mf_d_" + []string{"wire", "cable", "fuse", "card", "relay", "cap", "jumper"}[o.kind]); c != nil {
			out = append(out, clipItem{c, 0.4})
		}
	}
	return out
}

func (k *mfKit) allDone() bool {
	for _, o := range k.orders {
		if !o.done {
			return false
		}
	}
	return len(k.orders) > 0
}

// active: the cart's misfired and the guts are what you're working on.
func (k *mfKit) active() bool {
	d := &k.g.r.Dev
	return d.Misfire && d.State == sim.DevHome && k.g.r.Phase == sim.Playing
}

// ordersOpen: the guts are up and there's a fix to hear.
func (k *mfKit) ordersOpen() bool { return k.active() && k.raised && !k.allDone() }

// wrong: you did something the fix didn't ask for: he starts you over on a different one.
func (k *mfKit) wrong(at vec3) {
	if k.reset {
		return
	}
	ed.sfx(sndAlarm, at, 0.45)
	spawnSparks(at, v3(0, 1, 0), 30, 0.9)
	spawnSmoke(at, 4)
	k.g.say("WRONG ONE", "IT'S RESETTING - LIFT IT BACK UP FOR A NEW FIX", cDanger, 3)
	radioSay("x_mf_wrong", true)
	// the guts sink back into the cart; once they're in, the circuit's shuffled (resetDone)
	k.reset = true
	if h := k.g.handHolding(k.handle); h != nil {
		h.held = nil
	}
	ed.sfx(sndHydraulic, k.handlePos(), 0.6)
}

// resetDone: the guts are back inside after a wrong move: a different circuit, a new fix.
func (k *mfKit) resetDone() {
	k.reset = false
	k.generate(len(k.orders))
	k.raised, k.told = false, false
	ed.sfx(sndRelay, k.handlePos(), 0.7)
	ed.sfx(sndServo, k.handlePos(), 0.4)
}

func (k *mfKit) checkDone(at vec3) {
	if k.allDone() && !k.told {
		k.told = true
		ed.sfx(sndPowerUp, at, 0.6)
		k.g.say("THAT'S IT", "PUSH THE HANDLE BACK DOWN, THEN START IT UP AGAIN", cEnergy, 3)
		radioSay("x_mf_done", false)
	}
}

// wants: whether the fix includes this.
func (k *mfKit) wants(kind, i int) bool {
	for _, o := range k.orders {
		if o.kind == kind && o.i == i {
			return true
		}
	}
	return false
}

func (k *mfKit) updateOrders() {
	for i := range k.orders {
		o := &k.orders[i]
		was := o.done
		switch o.kind {
		case ordWire:
			o.done = k.wires[o.i].cut
		case ordCable:
			o.done = k.wires[o.i].cut && k.wires[o.i].patched
		case ordFuse:
			s := k.colSlot(partFuse, o.i)
			o.done = s.holds != nil && s.holds.spare && s.holds.col == o.i
		case ordJumper:
			o.done = k.colSlot(partJumper, o.i).holds == nil
		case ordCard, ordRelay, ordCap:
			s := k.slots[o.i]
			o.done = s.holds != nil && s.holds.spare
		}
		if o.done && !was {
			ed.sfx(sndRelay, k.handlePos(), 0.35)
			k.checkDone(k.handlePos())
		}
	}
}

// ---- the frame ----

func (g *game) misfireFrame(dt float64) {
	k := g.misfire
	if k == nil {
		return
	}
	act := k.active()
	if act && k.lift < 0.02 {
		k.fs.frame(dt)
	}
	if k.reset {
		was := k.lift
		k.lift = math.Max(0, k.lift-dt*0.7)
		if math.Floor(was*8) != math.Floor(k.lift*8) {
			ed.sfx(sndServo, k.handlePos(), 0.25)
		}
		if k.lift <= 0 {
			k.resetDone()
		}
	}
	if !act || !k.fs.allOut() || k.reset {
		if h := g.handHolding(k.handle); h != nil {
			h.held = nil
		}
		if !act && !k.reset {
			// it drops shut when you're not on it (or they've taken the cart)
			k.lift = math.Max(0, k.lift-dt*3)
		}
	}
	if k.lift > 0.98 && act && !k.raised {
		k.raised = true
		ed.sfx(sndHydraulic, k.handlePos(), 0.6)
		if !k.allDone() {
			radioSay("x_mf_up", true)
		}
	}
	// shut with the fix done: it's ready to go again
	if act && k.lift < 0.02 && k.allDone() {
		g.r.FixMisfire()
		ed.sfx(sndRelay, k.handlePos(), 0.7)
		k.raised = false
	}
	// still at it a while after hearing the fix: he calls back with what to look for
	if act && k.heard && !k.helped && !k.allDone() && g.t-k.heardT > 45 {
		k.helped = true
		radioSay("x_mf_help", false)
	}
	k.cutters.tick(dt)
	if k.lift > 0.01 {
		for _, w := range k.wires {
			k.trace(w)
		}
	}
	for _, pt := range k.spares {
		// a spare taken back out of a slot or off a cut
		if pt.slot != nil && !pt.p.docked {
			pt.slot.holds, pt.slot = nil, nil
		}
		if pt.patch != nil && !pt.p.docked {
			pt.patch.patched, pt.patch = false, nil
		}
		pt.p.tick(dt)
	}
	for _, pt := range k.parts {
		// pulled out of its slot
		if pt.slot != nil && !pt.p.docked {
			s := pt.slot
			pt.slot, s.holds = nil, nil
			if act && !pt.faulty && !k.reset {
				k.wrong(pt.p.pos)
				return
			}
			ed.sfx(sndTick, pt.p.pos, 0.5)
		}
		pt.p.tick(dt)
	}
	k.updateOrders()
	// the cutters: squeeze the trigger with the jaws on a wire
	if h := g.handHolding(k.cutters); h != nil && h.trigDown {
		k.snip(h)
	}
}

func (k *mfKit) jaw() vec3 { return k.cutters.pos.sub(k.cutters.Z.mul(0.135)) }

func (k *mfKit) snip(h *gHand) {
	j := k.jaw()
	ed.sfx(sndTick, j, 0.7)
	h.buzz(20, 0.6)
	if !k.active() || !k.open() {
		return
	}
	best, bd, bu := -1, cutReach, 0.0
	for i, w := range k.wires {
		if w.cut || len(w.world) < 2 {
			continue
		}
		L := 0.0
		segL := make([]float64, len(w.world)-1)
		for s := 0; s+1 < len(w.world); s++ {
			segL[s] = w.world[s+1].sub(w.world[s]).length()
			L += segL[s]
		}
		acc := 0.0
		for s := 0; s+1 < len(w.world); s++ {
			a, b := w.world[s], w.world[s+1]
			ab := b.sub(a)
			t := 0.0
			if l2 := ab.dot(ab); l2 > 1e-9 {
				t = math.Max(0, math.Min(1, j.sub(a).dot(ab)/l2))
			}
			r := 0.003
			if w.thick {
				r = 0.008
			}
			if d := j.sub(a.add(ab.mul(t))).length() - r; d < bd {
				best, bd, bu = i, d, (acc+t*segL[s])/math.Max(1e-6, L)
			}
			acc += segL[s]
		}
	}
	if best < 0 {
		return
	}
	w := k.wires[best]
	w.cut, w.cutU = true, math.Max(0.06, math.Min(0.94, bu))
	ed.sfx(sndSpark, j, 0.8)
	ed.sfx(sndClang, j, 0.2)
	spawnSparks(j, v3(0, 1, 0), 14, 0.5)
	h.buzz(45, 1)
	kind := ordWire
	if w.thick {
		kind = ordCable
	}
	if !k.wants(kind, best) {
		k.wrong(j)
		return
	}
	k.updateOrders()
	if w.thick {
		radioSay("x_mf_patch", true)
	}
}

// dropPart: a part let go of: into the empty slot of its kind it's next to (or, the patch
// cable, across the cut it's next to).
func (k *mfKit) dropPart(pt *mfPart) {
	if pt.p.docked && pt.spare && pt.slot == nil && pt.patch == nil {
		return // back on the shelf
	}
	if k.open() && pt.kind == partPatch {
		for _, w := range k.wires {
			if !w.thick || !w.cut || w.patched {
				continue
			}
			a, b := w.cutEnds()
			if a.add(b).mul(0.5).sub(pt.p.pos).length() < patchSnap {
				pt.patch, w.patched = w, true
				pt.p.docked = true
				ed.sfx(sndClang, a, 0.4)
				spawnSparks(a, v3(0, 1, 0), 8, 0.3)
				k.updateOrders()
				return
			}
		}
	}
	for _, s := range k.slots {
		if s.holds != nil || s.kind != pt.kind || !k.open() {
			continue
		}
		if p, _, _, _ := k.slotPose(s); p.sub(pt.p.pos).length() < 0.06 {
			pt.slot, s.holds = s, pt
			pt.p.docked = true
			ed.sfx(sndClang, p, 0.35)
			// a fuse of the wrong colour in it
			if k.active() && pt.spare && pt.kind == partFuse && pt.col != s.col {
				k.wrong(p)
				return
			}
			k.updateOrders()
			return
		}
	}
	if !pt.spare {
		pt.p.docked = false
	}
}

func (k *mfKit) grabbers(out []grabber) []grabber {
	out = append(out, k.cutters)
	for _, pt := range k.spares {
		out = append(out, pt.p)
	}
	if k.active() && !k.reset {
		out = append(out, k.handle)
		if k.lift < 0.02 {
			out = k.fs.grabbers(out)
		}
		if k.open() {
			for _, pt := range k.parts {
				out = append(out, pt.p)
			}
		}
	}
	return out
}

// stage: what the strip over the cart says while it's misfired.
func (k *mfKit) stage() string {
	switch {
	case k.reset:
		return "RESETTING..."
	case !k.fs.allOut():
		return fastVerb(k.kind) + " THE HATCH ON TOP"
	case k.allDone():
		return "PUSH THE HANDLE BACK DOWN"
	case k.raised && k.needsPatch() != nil:
		return "LAY THE YELLOW PATCH CABLE ACROSS THE CUT"
	case !k.raised:
		return "PULL THE HANDLE UP"
	case !k.heard:
		return "PICK UP THE RADIO"
	}
	return "DO WHAT THE RADIO SAYS"
}

// ---- the handle ----

type mfHandle struct {
	k            *mfKit
	grabY, grabL float64
}

func (h *mfHandle) gPos() vec3     { return h.k.handlePos() }
func (h *mfHandle) gRad() float64  { return 0.09 }
func (h *mfHandle) gOK() bool      { return h.k.active() && h.k.fs.allOut() && !h.k.reset }
func (h *mfHandle) gGrab(g *gHand) { h.grabY, h.grabL = g.palm.y, h.k.lift; g.buzz(30, 0.6) }
func (h *mfHandle) gHold(g *gHand, dt float64) {
	k := h.k
	was := k.lift
	k.lift = math.Max(0, math.Min(1, h.grabL+(g.palm.y-h.grabY)/mfRise))
	if math.Floor(was*8) != math.Floor(k.lift*8) {
		g.buzz(10, 0.4)
		ed.sfx(sndServo, h.gPos(), 0.25)
	}
	if was < 1 && k.lift >= 1 || was > 0 && k.lift <= 0 {
		g.buzz(40, 0.9)
		ed.sfx(sndClang, h.gPos(), 0.4)
	}
}
func (h *mfHandle) gRelease(g *gHand) {
	// let go part way: it settles open if it's more than half up, shut if not
	if k := h.k; k.lift > 0.5 {
		k.lift = 1
	} else {
		k.lift = 0
	}
}

// ---- drawing ----

func (g *game) drawMisfire() {
	k := g.misfire
	if k == nil {
		return
	}
	F := k.F()
	t := fxTime()
	act := k.active()
	// the hatch on the cart's top: its lid, the corners
	lidY := k.layerY(mfLayers)
	if k.lift > 0.005 {
		// the hole it came up out of
		useMat(mPaint)
		bbox(&bEnv, F, 0, devTop+0.0012, mfZ, mfHX-0.006, 0.001, mfHZ-0.006, 0.0005, rgba{0.02, 0.02, 0.02, 1})
	}
	useMat(mSteel)
	bbox(&bEnv, F, 0, lidY+0.003, mfZ, mfHX-0.005, 0.004, mfHZ-0.005, 0.002, cSteelEnv.scale(0.62))
	topText(&bOpaque, "SERVICE", F.p(0, lidY+0.0075, mfZ+0.055), F.y, 0.0011, cMuted)
	if k.lift < 0.02 {
		k.fs.draw(act)
	}
	// the handle
	hp := k.handlePos()
	useMat(mSteel)
	for _, sx := range []float64{-1, 1} {
		cyl(&bEnv, F.p(sx*0.06, lidY+0.006, mfZ), F.p(sx*0.06, lidY+0.055, mfZ), 0.007, 0.007, 8, cSteelEnv, false)
	}
	useMat(mRubber)
	hc := rgba{0.08, 0.08, 0.08, 1}
	if act && k.fs.allOut() {
		hc = cAmber.scale(0.7)
	}
	cyl(&bEnv, F.p(-0.065, lidY+0.055, mfZ), F.p(0.065, lidY+0.055, mfZ), 0.013, 0.013, 10, hc, true)
	if act && k.fs.allOut() && !k.raised && g.handHolding(k.handle) == nil {
		sprite(hp, 0.08, cAmber.alpha(float32(0.3+0.25*math.Sin(t*6))))
	}
	if k.lift > 0.01 {
		k.drawGuts(F, t)
	}
	// the shelf: cutters and spares
	k.drawCutters()
	for _, pt := range k.spares {
		if pt.kind == partPatch {
			k.drawPatch(pt)
		} else {
			k.drawPart(pt, t)
		}
	}
	// a bin for the fuses, a rack for the card, a box for the relay and capacitor
	useMat(mPaint)
	bin := rgba{0.12, 0.13, 0.14, 1}
	bbox(&bEnv, F, -0.12, 0.172, 0.24, 0.105, 0.004, 0.03, 0.002, bin)
	for i := 0; i <= len(mfFuseNames); i++ {
		bbox(&bEnv, F, -0.216+0.032*float64(i), 0.185, 0.24, 0.0015, 0.013, 0.03, 0.0008, bin)
	}
	for _, s := range []float64{-1, 1} {
		bbox(&bEnv, F, -0.12, 0.185, 0.24+s*0.03, 0.105, 0.013, 0.0015, 0.0008, bin)
		bbox(&bEnv, F, 0.08, 0.183, 0.02+s*0.03, 0.08, 0.011, 0.0015, 0.0008, bin)
		bbox(&bEnv, F, 0.08+s*0.08, 0.183, 0.02, 0.0015, 0.011, 0.03, 0.0008, bin)
		bbox(&bEnv, F, -0.12, 0.178, 0.02+s*0.038, 0.04, 0.006, 0.0015, 0.0008, bin)
	}
	faceText(&bOpaque, "SPARES", F.p(0, 0.215, 0.371), F.z, F.y, 0.0014, cMuted)
	faceText(&bOpaque, "SPARES", F.p(0, 0.215, -0.371), F.z.mul(-1), F.y, 0.0014, cMuted)
}

func (k *mfKit) drawGuts(F xf, t float64) {
	// the corner posts the layers ride on
	useMat(mSteel)
	for _, sx := range []float64{-1, 1} {
		for _, sz := range []float64{-1, 1} {
			a := F.p(sx*(mfBX-0.005), devTop-0.04, mfZ+sz*(mfBZ-0.005))
			b := F.p(sx*(mfBX-0.005), k.layerY(mfLayers), mfZ+sz*(mfBZ-0.005))
			cyl(&bEnv, a, b, 0.005, 0.005, 6, cSteelEnv, false)
		}
	}
	// the layers
	for l := 0; l < mfLayers; l++ {
		y := k.layerY(l)
		useMat(mPaint)
		bbox(&bEnv, F, 0, y, mfZ, mfBX, 0.003, mfBZ, 0.001, rgba{0.05, 0.2, 0.1, 1})
		for i := 0; i < 5; i++ {
			z := mfZ - mfBZ + 0.03 + float64(i)*0.033
			ribbon(F.p(-mfBX+0.01, y+0.0035, z), F.p(mfBX-0.01, y+0.0035, z+0.01*math.Sin(float64(i*7+l))), 0.0015, rgba{0.7, 0.5, 0.2, 0.5})
		}
	}
	for _, dc := range k.deco {
		p := k.world(dc.at)
		useMat(mPaint)
		switch dc.kind {
		case 0: // a chip
			bevelBox(&bEnv, p.add(F.y.mul(0.003)), F.x, F.y, F.z, 0.018, 0.003, 0.012, 0.001, rgba{0.06, 0.06, 0.07, 1})
		case 1: // a coil
			cyl(&bEnv, p.add(F.x.mul(-0.016)).add(F.y.mul(0.01)), p.add(F.x.mul(0.016)).add(F.y.mul(0.01)), 0.01, 0.01, 10, rgba{0.75, 0.42, 0.15, 1}, true)
		default: // a heat sink
			for i := -2; i <= 2; i++ {
				bbox(&bEnv, xf{p, F.x, F.y, F.z}, 0.006*float64(i), 0.01, 0, 0.0012, 0.01, 0.014, 0.0005, cSteelEnv.scale(0.8))
			}
		}
	}
	// the sockets
	for _, s := range k.slots {
		p := k.world(s.at)
		switch s.kind {
		case partFuse:
			useMat(mSteel)
			for _, sd := range []float64{-1, 1} {
				q := p.add(F.x.mul(0.022 * sd))
				cyl(&bEnv, q, q.add(F.y.mul(0.016)), 0.003, 0.003, 6, cSteelEnv, false)
			}
		case partCard:
			useMat(mPaint)
			bbox(&bEnv, xf{p, F.x, F.y, F.z}, 0, 0.003, 0, 0.004, 0.003, 0.034, 0.001, rgba{0.1, 0.1, 0.1, 1})
		case partJumper:
			useMat(mChrome)
			for _, sd := range []float64{-1, 1} {
				q := p.add(F.z.mul(0.004 * sd))
				cyl(&bEnv, q, q.add(F.y.mul(0.012)), 0.0012, 0.0012, 4, rgba{0.85, 0.7, 0.3, 1}, false)
			}
		default:
			useMat(mPaint)
			bbox(&bEnv, xf{p, F.x, F.y, F.z}, 0, 0.002, 0, 0.014, 0.002, 0.016, 0.001, rgba{0.12, 0.12, 0.12, 1})
		}
	}
	for _, pt := range k.parts {
		k.drawPart(pt, t)
	}
	for _, w := range k.wires {
		k.drawWire(w)
	}
	for _, pt := range k.spares {
		if pt.kind == partPatch && pt.patch != nil {
			k.drawPatch(pt)
		}
	}
	k.drawPatchHint(t)
}

// catmull: a smooth path through pts.
func catmull(pts []vec3, per int) []vec3 {
	if len(pts) < 2 {
		return pts
	}
	var out []vec3
	for i := 0; i+1 < len(pts); i++ {
		p0, p1, p2, p3 := pts[max(0, i-1)], pts[i], pts[i+1], pts[min(len(pts)-1, i+2)]
		for s := 0; s < per; s++ {
			u := float64(s) / float64(per)
			u2, u3 := u*u, u*u*u
			q := p1.mul(2).add(p2.sub(p0).mul(u)).add(p0.mul(2).sub(p1.mul(5)).add(p2.mul(4)).sub(p3).mul(u2)).add(p1.mul(3).sub(p0).sub(p2.mul(3)).add(p3).mul(u3)).mul(0.5)
			out = append(out, q)
		}
	}
	return append(out, pts[len(pts)-1])
}

// trace: where the wire runs this frame.
func (k *mfKit) trace(w *mfWire) {
	var ctl []vec3
	for _, p := range w.pts {
		ctl = append(ctl, k.world(p))
	}
	w.world = catmull(ctl, 6)
}

func (k *mfKit) drawWire(w *mfWire) {
	k.trace(w)
	n := len(w.world) - 1
	r := 0.003
	if w.thick {
		r = 0.008
	}
	useMat(mRubber)
	cutI := -1
	if w.cut {
		cutI = min(n-1, max(0, int(w.cutU*float64(n))))
	}
	for s := 0; s < n; s++ {
		if s == cutI {
			continue
		}
		a, b := w.world[s], w.world[s+1]
		col := mfCols[w.a]
		if w.thick {
			col = mfCableCols[w.a]
		} else if w.b != w.a && s%3 == 1 {
			col = mfCols[w.b]
		}
		cyl(&bEnv, a, b, r, r, 6, col, false)
	}
	if cutI >= 0 {
		// bare copper at the cut ends
		a, b := w.world[cutI], w.world[cutI+1]
		d := b.sub(a)
		useMat(mChrome)
		cyl(&bEnv, a, a.add(d.mul(0.3)), r*0.6, r*0.3, 5, rgba{0.85, 0.5, 0.25, 1}, false)
		cyl(&bEnv, b, b.sub(d.mul(0.3)), r*0.6, r*0.3, 5, rgba{0.85, 0.5, 0.25, 1}, false)
	}
	useMat(mSteel)
	for _, e := range []vec3{w.world[0], w.world[n]} {
		sphere(&bEnv, e, r*1.4, cSteelEnv, 3, 6)
	}
}

// drawPatch: the patch cable - coiled on the shelf, or bridging a cut with a clamp each end.
func (k *mfKit) drawPatch(pt *mfPart) {
	p := pt.p
	col := rgba{0.95, 0.8, 0.1, 1}
	var a, b vec3
	if w := pt.patch; w != nil {
		a, b = w.cutEnds()
		d := b.sub(a)
		a, b = a.sub(d.mul(0.6)), b.add(d.mul(0.6))
	} else {
		a, b = p.pos.sub(p.Z.mul(0.06)), p.pos.add(p.Z.mul(0.06))
	}
	bow := p.Y.mul(0.03)
	useMat(mRubber)
	hose(&bEnv, a, a.add(b.sub(a).mul(0.3)).add(bow), a.add(b.sub(a).mul(0.7)).add(bow), b, 0.007, col, 10)
	useMat(mSteel)
	for _, e := range []vec3{a, b} {
		cyl(&bEnv, e.sub(p.Z.mul(0.008)), e.add(p.Z.mul(0.008)), 0.011, 0.011, 8, cSteelEnv, true)
	}
}

func (k *mfKit) drawPart(pt *mfPart, t float64) {
	p := pt.p
	rng := rand.New(rand.NewSource(pt.seed))
	switch pt.kind {
	case partFuse:
		useMat(mGlass)
		cyl(&bEnv, p.pos.sub(p.Z.mul(0.016)), p.pos.add(p.Z.mul(0.016)), 0.006, 0.006, 10, rgba{0.75, 0.82, 0.85, 1}, false)
		useMat(mChrome)
		if pt.faulty {
			// blown: the filament snapped, a little smudge on the glass
			cyl(&bEnv, p.pos.sub(p.Z.mul(0.015)), p.pos.sub(p.Z.mul(0.004)), 0.0008, 0.0008, 4, rgba{0.3, 0.3, 0.3, 1}, false)
			cyl(&bEnv, p.pos.add(p.Z.mul(0.015)), p.pos.add(p.Z.mul(0.006)), 0.0008, 0.0008, 4, rgba{0.3, 0.3, 0.3, 1}, false)
		} else {
			cyl(&bEnv, p.pos.sub(p.Z.mul(0.015)), p.pos.add(p.Z.mul(0.015)), 0.0008, 0.0008, 4, rgba{0.85, 0.85, 0.8, 1}, false)
		}
		useMat(mPaint)
		for _, s := range []float64{-1, 1} {
			c := p.pos.add(p.Z.mul(0.019 * s))
			cyl(&bEnv, c.sub(p.Z.mul(0.004)), c.add(p.Z.mul(0.004)), 0.0068, 0.0068, 10, mfFuseCols[pt.col], true)
		}
	case partCard:
		useMat(mPaint)
		bevelBox(&bEnv, p.pos, p.X, p.Y, p.Z, 0.0015, 0.024, 0.032, 0.0008, rgba{0.07, 0.28, 0.14, 1})
		for i := 0; i < 4; i++ {
			q := p.pos.add(p.Y.mul(-0.012 + 0.024*rng.Float64())).add(p.Z.mul(-0.022 + 0.044*rng.Float64()))
			for _, s := range []float64{-1, 1} {
				bevelBox(&bEnv, q.add(p.X.mul(0.003*s)), p.X, p.Y, p.Z, 0.0015, 0.004, 0.006, 0.0005, rgba{0.05, 0.05, 0.06, 1})
			}
		}
		bevelBox(&bEnv, p.pos.sub(p.Y.mul(0.023)), p.X, p.Y, p.Z, 0.0017, 0.003, 0.028, 0.0005, rgba{0.85, 0.65, 0.2, 1})
		if pt.faulty {
			// the charring: dark blotches on one corner, hard to make out
			for i := 0; i < 5; i++ {
				u, v := -0.002+0.012*rng.Float64(), 0.006+0.018*rng.Float64()
				for _, s := range []float64{-1, 1} {
					q := p.pos.add(p.Y.mul(u)).add(p.Z.mul(v)).add(p.X.mul(0.0017 * s))
					bevelBox(&bEnv, q, p.X, p.Y, p.Z, 0.0004, 0.004+0.004*rng.Float64(), 0.005+0.004*rng.Float64(), 0.0002, rgba{0.035, 0.05, 0.035, 1})
				}
			}
			k.embers(p.pos.add(p.Z.mul(0.015)), p.Y.mul(0.01), p.Z.mul(0.012), t, rng)
		}
	case partRelay:
		// a little clear-topped cube
		useMat(mPaint)
		bevelBox(&bEnv, p.pos, p.X, p.Y, p.Z, 0.011, 0.012, 0.015, 0.002, rgba{0.12, 0.14, 0.2, 1})
		useMat(mGlass)
		bevelBox(&bEnv, p.pos.add(p.Y.mul(0.0125)), p.X, p.Y, p.Z, 0.0105, 0.0008, 0.0145, 0.0004, rgba{0.6, 0.65, 0.7, 1})
		if pt.faulty {
			// a hairline crack across the top
			useMat(mPaint)
			a := p.pos.add(p.Y.mul(0.0134)).add(p.X.mul(-0.009)).add(p.Z.mul(-0.006))
			m := a.add(p.X.mul(0.008)).add(p.Z.mul(0.004))
			b := m.add(p.X.mul(0.009)).add(p.Z.mul(0.007))
			cyl(&bEnv, a, m, 0.0004, 0.0004, 3, rgba{0.02, 0.02, 0.02, 1}, false)
			cyl(&bEnv, m, b, 0.0004, 0.0004, 3, rgba{0.02, 0.02, 0.02, 1}, false)
		}
	case partCap:
		useMat(mPaint)
		top := p.pos.add(p.Y.mul(0.03))
		cyl(&bEnv, p.pos, top, 0.009, 0.009, 12, rgba{0.15, 0.3, 0.65, 1}, false)
		useMat(mChrome)
		if pt.faulty {
			// bulged: the top domes up a hair
			hemi(&bEnv, top, p.Y, 0.009, cChromeEnv, 3, 12)
			sphere(&bEnv, top.add(p.Y.mul(0.0015)), 0.0062, cChromeEnv, 3, 8)
		} else {
			cyl(&bEnv, top, top.add(p.Y.mul(0.0008)), 0.009, 0.0088, 12, cChromeEnv, true)
		}
	case partJumper:
		useMat(mPaint)
		bevelBox(&bEnv, p.pos, p.X, p.Y, p.Z, 0.003, 0.004, 0.0065, 0.001, mfFuseCols[pt.col])
	}
}

// embers: in a flashlight's beam, char glints - the only giveaway.
func (k *mfKit) embers(c, du, dv vec3, t float64, rng *rand.Rand) {
	f := k.g.devVis.flash
	if !f.held {
		return
	}
	to := c.sub(f.pos)
	if to.length() > 2 || to.norm().dot(f.Z.mul(-1)) < math.Cos(0.38) {
		return
	}
	for i := 0; i < 3; i++ {
		q := c.add(du.mul(rng.Float64()*2 - 1)).add(dv.mul(rng.Float64()*2 - 1))
		fl := 0.5 + 0.5*math.Sin(t*(7+3*float64(i))+float64(i)*2)
		sprite(q, 0.006+0.004*fl, rgba{1, 0.45, 0.1, float32(0.5 + 0.4*fl)})
	}
	if rngF() < frameDT*2 {
		spawnSmoke(c, 1)
	}
}

func (k *mfKit) drawCutters() {
	p := k.cutters
	squeeze := 0.0
	if h := k.g.handHolding(p); h != nil {
		squeeze = h.trig
	}
	open := 0.32 * (1 - squeeze)
	piv := p.pos.sub(p.Z.mul(0.075))
	useMat(mRubber)
	for _, s := range []float64{-1, 1} {
		dir := rotAbout(p.Z, p.Y, s*open*0.5)
		a := piv.add(dir.mul(0.075)).add(p.X.mul(0.012 * s))
		cyl(&bEnv, piv.add(p.X.mul(0.004*s)), a, 0.006, 0.008, 8, rgba{0.75, 0.1, 0.06, 1}, true)
	}
	useMat(mChrome)
	cyl(&bEnv, piv.sub(p.Y.mul(0.006)), piv.add(p.Y.mul(0.006)), 0.006, 0.006, 8, cChromeEnv, true)
	for _, s := range []float64{-1, 1} {
		dir := rotAbout(p.Z.mul(-1), p.Y, s*open*0.5)
		cyl(&bEnv, piv, piv.add(dir.mul(0.06)), 0.005, 0.0015, 6, cChromeEnv, false)
	}
	if k.g.handHolding(p) == nil && p.docked && k.active() && k.raised && !k.allDone() {
		sprite(p.pos, 0.05, cChrome.alpha(float32(0.2+0.2*math.Sin(fxTime()*4))))
	}
}
