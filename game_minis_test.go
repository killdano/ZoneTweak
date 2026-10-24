package main

import (
	"boundaryvr/sim"
	"math"
	"testing"
)

// miniOn: the device's game stage, as this game, open and ready.
func miniOn(t *testing.T, kind string) (*tester, *modGame, int) {
	tt := newTester(t, true)
	r := tt.g.r
	for i := range r.Dev.Mods {
		if r.Dev.Mods[i].ID == "game" {
			r.Dev.Mods[i].Game = kind
		}
	}
	tt.toModule("game", 0)
	m := tt.mod().(*modGame)
	if m.kind != kind || m.mini == nil {
		t.Fatalf("game is %q", m.kind)
	}
	tt.frames(60) // the lid swings down
	return tt, m, r.Dev.Cur
}

// knobHand: a hand turning a knob toward an angle a little each frame, letting go and
// taking a new grip when the wrist runs out (as a real hand does).
type knobHand struct {
	tt    *tester
	i     int
	k     *knob
	wrist float64
	held  bool
}

func (kh *knobHand) step(want float64) {
	tt, k := kh.tt, kh.k
	if !kh.held {
		tt.pose(kh.i, k.c.add(k.ax.mul(0.15)), k.ax.mul(-1), v3(0, 1, 0))
		tt.frames(1)
		tt.grabAt(kh.i, k.c)
		kh.held, kh.wrist = true, 0
		if hands[kh.i].held != k {
			tt.t.Fatalf("hand %d took the wrong thing", kh.i)
		}
	}
	d := math.Max(-0.05, math.Min(0.05, want-k.ang))
	if math.Abs(kh.wrist+d) > 1.7 {
		tt.release(kh.i)
		tt.rotate(kh.i, k.ax, -kh.wrist)
		tt.frames(1)
		tt.grabAt(kh.i, k.c)
		kh.wrist = 0
		return
	}
	tt.rotate(kh.i, k.ax, d)
	kh.wrist += d
}

func (kh *knobHand) letGo() {
	if kh.held {
		kh.tt.release(kh.i)
		kh.held = false
	}
}

// Frequency lock: the two knobs steer your wave onto the drifting target and, held there,
// it fills; nothing drifts before you take a knob.
func TestMiniFreq(t *testing.T) {
	tt, m, cur := miniOn(t, "freq")
	q := m.mini.(*miniFreq)
	tt.frames(300)
	if q.t != 0 || q.prog != 0 {
		t.Fatal("it ran before anyone took a knob")
	}
	f, a := &knobHand{tt: tt, i: 1, k: q.kf}, &knobHand{tt: tt, i: 0, k: q.ka}
	for k := 0; k < 60*60 && !tt.g.r.Dev.Mods[cur].Done; k++ {
		ft, at := q.target()
		f.step((ft - 1) / 2.5 * 2 * math.Pi)
		a.step((at - 0.15) / 0.85 * 2 * math.Pi)
		tt.frames(1)
	}
	if !tt.g.r.Dev.Mods[cur].Done {
		t.Fatalf("frequency lock not done (progress %.2f)", q.prog)
	}
}

// The safe: three numbers found by turning to each and holding it there.
func TestMiniSafe(t *testing.T) {
	tt, m, cur := miniOn(t, "safe")
	q := m.mini.(*miniSafe)
	kh := &knobHand{tt: tt, i: 1, k: q.dial}
	step := 2 * math.Pi / safeNums
	for d := 0; d < 3; d++ {
		target := q.comb[d]
		// the nearest angle that puts it under the mark
		want := -float64(target) * step
		for math.Abs(want-q.dial.ang) > math.Pi {
			want += math.Copysign(2*math.Pi, q.dial.ang-want)
		}
		for k := 0; k < 60*20 && q.got == d; k++ {
			kh.step(want)
			tt.frames(1)
		}
		if q.got != d+1 && !tt.g.r.Dev.Mods[cur].Done {
			t.Fatalf("number %d (%d) not found: dial on %d", d, target, q.number())
		}
	}
	if !tt.g.r.Dev.Mods[cur].Done {
		t.Fatal("the safe's open but the stage isn't done")
	}
}

// The safe gives itself away: the signal meter's full on the number and empty far off.
func TestMiniSafeSignal(t *testing.T) {
	q := newSafe()
	step := 2 * math.Pi / safeNums
	q.dial.ang = -float64(q.comb[0]) * step
	if q.number() != q.comb[0] || q.signal() != 1 {
		t.Fatalf("on the number the signal should be full (%.2f)", q.signal())
	}
	q.dial.ang = -float64((q.comb[0]+10)%safeNums) * step
	if q.signal() != 0 {
		t.Fatal("ten off, no signal")
	}
}

// The switchboard: each call's cord into the line it wants; two lost calls start it over.
func TestMiniPlug(t *testing.T) {
	tt, m, cur := miniOn(t, "plug")
	q := m.mini.(*miniPlug)
	for c := 0; c < plugCalls; c++ {
		for k := 0; k < 300 && q.caller < 0; k++ {
			tt.frames(1)
		}
		if q.caller < 0 {
			t.Fatalf("call %d never rang", c)
		}
		p := q.plugs[q.caller]
		tt.pose(1, p.pos.add(q.n.mul(0.12)), q.n.mul(-1), v3(0, 1, 0))
		tt.frames(1)
		tt.grabAt(1, p.pos)
		if hands[1].held != p {
			t.Fatalf("call %d: took the wrong plug", c)
		}
		tt.move(1, q.jacks[q.line].add(q.n.mul(0.012)), 10)
		tt.release(1)
		if q.done != c+1 && !tt.g.r.Dev.Mods[cur].Done {
			t.Fatalf("call %d: plugged in, not put through (in %d)", c, p.in)
		}
		tt.move(1, p.rest.add(q.n.mul(0.2)), 4)
	}
	if !tt.g.r.Dev.Mods[cur].Done {
		t.Fatal("all calls put through but the stage isn't done")
	}
}

func TestMiniPlugLostCalls(t *testing.T) {
	tt, m, cur := miniOn(t, "plug")
	q := m.mini.(*miniPlug)
	for k := 0; k < 300 && q.caller < 0; k++ {
		tt.frames(1)
	}
	// pick up a plug (the clock starts), put it back
	p := q.plugs[(q.caller+1)%4]
	tt.pose(1, p.pos.add(q.n.mul(0.12)), q.n.mul(-1), v3(0, 1, 0))
	tt.frames(1)
	tt.grabAt(1, p.pos)
	tt.move(1, p.pos.add(q.n.mul(0.2)), 4)
	tt.release(1)
	tt.frames(int((plugTime + plugCallGap + 1) / tdt))
	if q.misses != 1 || tt.g.r.Dev.Cur != cur {
		t.Fatalf("one call rang out: one miss, still on (misses %d)", q.misses)
	}
	tt.frames(int((plugTime + plugCallGap + 1) / tdt))
	if m := tt.mod().(*modGame); m.kind == "plug" && m.mini.(*miniPlug) == q {
		t.Fatal("two lost calls should start the game over (as another game)")
	}
}

// Valves: open them so all three gauges sit in the green, hold it 3 s.
func TestMiniValves(t *testing.T) {
	tt, m, cur := miniOn(t, "valves")
	q := m.mini.(*miniValves)
	// the openings that put every gauge mid-band (solve v_i - 0.45 v_(i-1) = 0.335 - off_i)
	mid := (vlLo + vlHi) / 2
	var v [3]float64
	for it := 0; it < 60; it++ {
		for i := range v {
			v[i] = mid - 0.2 - q.off[i] + 0.45*v[(i+2)%3]
		}
	}
	hs := []*knobHand{{tt: tt, i: 1, k: q.kn[0]}, {tt: tt, i: 1, k: q.kn[1]}, {tt: tt, i: 1, k: q.kn[2]}}
	for i, kh := range hs {
		want := v[i] * 3 * math.Pi
		for k := 0; k < 60*15 && math.Abs(q.kn[i].ang-want) > 0.01; k++ {
			kh.step(want)
			tt.frames(1)
		}
		kh.letGo()
		if tt.g.r.Dev.Cur != cur {
			t.Fatalf("valve %d: blew a gauge on the way (p %v)", i, q.p)
		}
	}
	tt.move(1, q.kn[0].c.add(q.kn[0].ax.mul(0.3)), 4)
	for k := 0; k < 60*6 && !tt.g.r.Dev.Mods[cur].Done; k++ {
		tt.frames(1)
	}
	if !tt.g.r.Dev.Mods[cur].Done {
		t.Fatalf("all three set, not done (p %.2f %.2f %.2f, hold %.1f)", q.p[0], q.p[1], q.p[2], q.hold)
	}
}

func TestMiniValvesBurst(t *testing.T) {
	tt, m, cur := miniOn(t, "valves")
	q := m.mini.(*miniValves)
	kh := &knobHand{tt: tt, i: 1, k: q.kn[0]}
	for k := 0; k < 60*15 && q.kn[0].ang < 3*math.Pi-0.01; k++ {
		kh.step(3 * math.Pi)
		tt.frames(1)
	}
	kh.letGo()
	tt.frames(60 * 3)
	if tt.g.r.Dev.Cur == cur && tt.mod().(*modGame).mini == q {
		t.Fatalf("one valve wide open and the gauge in the red should blow it (p %.2f)", q.p[0])
	}
}

// The code on the far side: keyed right it's done; keyed wrong the code changes.
func TestMiniLook(t *testing.T) {
	tt, m, cur := miniOn(t, "look")
	q := m.mini.(*miniLook)
	old := q.code
	wrong := (q.code[0] + 1) % 6
	tt.poke(1, q.btn[wrong].c, q.btn[wrong].n)
	if q.got != 0 || q.changed <= 0 {
		t.Fatal("a wrong symbol should change the code")
	}
	_ = old
	for k := 0; k < lookLen; k++ {
		s := q.code[k]
		tt.poke(1, q.btn[s].c, q.btn[s].n)
	}
	if !tt.g.r.Dev.Mods[cur].Done {
		t.Fatalf("keyed the code, not done (got %d)", q.got)
	}
	// the screen faces away from the keypad: you can't read it from the hatch
	sc, sn, _, _ := q.farScreen(tt.g)
	c, n, _ := m.face(tt.g.devFrame())
	if sn.dot(n) > -0.5 || sc.sub(c).dot(n) > 0 {
		t.Fatal("the code screen should face the other way, on the far side")
	}
}

// Every game the device can want draws without breaking the batches, and each round's two
// game stages are different games.
func TestMiniGamesDiffer(t *testing.T) {
	for seed := int64(1); seed < 30; seed++ {
		R := []sim.V2{sim.V(-1.5, -1.5), sim.V(1.5, -1.5), sim.V(1.5, 1.5), sim.V(-1.5, 1.5)}
		r := sim.NewRound(R, seed, tuningFor(), nil)
		var gs []string
		for _, md := range r.Dev.Mods {
			if md.ID == "game" {
				gs = append(gs, md.Game)
			}
		}
		if len(gs) == 2 && gs[0] == gs[1] {
			t.Fatalf("seed %d: the same game twice (%s)", seed, gs[0])
		}
	}
}

// The prod starts in a bracket on one of the generators - a different one from round to
// round - inside your room, and the leech warning goes on to say where it is.
func TestProdBracket(t *testing.T) {
	gens := map[int]bool{}
	for seed := int64(1); seed <= 12; seed++ {
		P := []pt2{{-1.8, -1.5}, {1.8, -1.5}, {1.8, 1.5}, {-1.8, 1.5}}
		g := newGame(P, seed)
		G = g
		m := g.poleMount()
		if m.gen < 0 {
			t.Fatalf("seed %d: no generator for the bracket", seed)
		}
		gens[m.gen] = true
		hp, _, _, _ := g.pole.p.home()
		if !g.pole.p.docked || hp.sub(g.pole.p.pos).length() > 1e-6 {
			t.Fatalf("seed %d: the prod should start in its bracket", seed)
		}
		if !sim.InPoly(g.r.Room, sim.V(hp.x, hp.z)) || sim.Dist(sim.V(hp.x, hp.z), g.r.Gens[m.gen].Pos) > 0.6 {
			t.Fatalf("seed %d: the bracket should be inside the room, at its generator", seed)
		}
		cl := msgClips("x_leech")
		if len(cl) != 2 || voiceClip(g.poleWhere()) == nil {
			t.Fatalf("seed %d: the leech warning should say where the prod is (%q)", seed, g.poleWhere())
		}
	}
	if len(gens) < 3 {
		t.Fatalf("the bracket should move round the generators from round to round (only %v)", gens)
	}
}
