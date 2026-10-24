//go:build !windows

package main

import (
	"boundaryvr/sim"
	"os"
	"testing"
)

// Nothing you have to touch is outside your real boundary (there may be a wall there), and
// the green ring never runs outside it either - in your captured room and the lab rooms.
func TestEverythingReachableInside(t *testing.T) {
	rooms := map[string][]pt2{}
	for _, spec := range []string{"chap", "10,10", "7,7", "16,16", "12,10"} {
		os.Chdir("preview")
		os.Setenv("ROOM", spec)
		rooms[spec] = previewRoom()
		os.Chdir("..")
	}
	for name, room := range rooms {
		var R []sim.V2
		for _, p := range room {
			R = append(R, sv(p))
		}
		lo, hi := sim.GenRange(R, tuningFor())
		for n := lo; n <= hi; n++ {
			g := newGame(room, 3)
			if g.r.SetGens(n) {
				g.syncGens()
				g.buildPanels()
			}
			in := func(what string, p vec3) {
				q := sim.V(p.x, p.z)
				if !sim.InPoly(g.r.Room, q) || sim.EdgeDist(g.r.Room, q) < 0.04 {
					t.Errorf("%s, %d gens: %s at %.2f,%.2f is outside your boundary", name, n, what, p.x, p.z)
				}
			}
			G := g.r.Gens2()
			for _, gp := range g.panels_ {
				F := gp.F
				in("breaker", gp.brk.piv)
				in("pump", gp.pump.base)
				in("crank", gp.crk.c)
				in("valve", gp.valve.c)
				in("patch socket", F.p(0, 1.78, 0.09))
				for _, x := range []float64{-0.2, 0.2} {
					in("panel edge", F.p(x, 1, 0.05))
				}
				// where a leech's tether clamps on
				c, _ := g.leechClamp(gp.i)
				in("leech clamp", c)
			}
			for i := range G {
				a, b := G[i], G[(i+1)%len(G)]
				for k := 0; k <= 10; k++ {
					q := sim.Lerp(a, b, float64(k)/10)
					if !sim.InPoly(g.r.Room, q) {
						t.Errorf("%s, %d gens: the ring runs outside your boundary at %.2f,%.2f", name, n, q.X, q.Z)
						break
					}
				}
			}
			in("device", fv(g.r.Dev.Pos, 1))
			in("radio", fv(g.radio.pos, 1))
			in("pole", g.pole.p.pos)
		}
	}
}

// The harpoon turret stands well in from the ring (you stand behind it and swing it about),
// as far as the room allows, and never on the radio, the pole's stand or the console.
func TestTurretHasRoomAroundIt(t *testing.T) {
	for _, c := range []struct {
		spec string
		in   float64
	}{{"chap", 0.9}, {"16,16", 1.15}, {"12,10", 0.6}, {"10,10", 0.5}, {"7,7", 0.3}} {
		os.Chdir("preview")
		os.Setenv("ROOM", c.spec)
		room := previewRoom()
		os.Chdir("..")
		for seed := int64(1); seed <= 4; seed++ {
			g := newGame(room, seed)
			g.step(0.01)
			r := g.r
			r.Start()
			g.step(0.01)
			r.CompleteModule()
			r.CompleteModule()
			r.CompleteModule()
			for k := 0; k < 10 && !r.Harp.Up; k++ {
				g.step(0.05)
			}
			h := r.Harp
			if !h.Up {
				t.Errorf("%s seed %d: no turret after three stages", c.spec, seed)
				continue
			}
			if d := sim.EdgeDist(r.RingPoly, h.Pos); d < c.in {
				t.Errorf("%s seed %d: the turret is only %.2f m in from the ring (want %.2f)", c.spec, seed, d, c.in)
			}
			for _, f := range r.Fixtures {
				if sim.Dist(f, h.Pos) < 0.7 {
					t.Errorf("%s seed %d: the turret rose on top of a fixture", c.spec, seed)
				}
			}
		}
	}
}

// A leech on a double generator clamps onto whichever of its two posts is further inside
// your boundary.
func TestLeechClampsOnTheInnerPost(t *testing.T) {
	os.Chdir("preview")
	os.Setenv("ROOM", "chap")
	room := previewRoom()
	os.Chdir("..")
	found := false
	for n := 8; n <= 10; n++ {
		g := newGame(room, 3)
		if g.r.SetGens(n) {
			g.syncGens()
			g.buildPanels()
		}
		for _, gp := range g.panels_ {
			posts := g.postsOf(gp.i)
			if len(posts) < 2 {
				continue
			}
			found = true
			c, _ := g.leechClamp(gp.i)
			q := sim.V(c.x, c.z)
			for _, k := range posts {
				if sim.EdgeDist(g.r.Room, g.r.Gens[k].Pos) > sim.EdgeDist(g.r.Room, g.r.Gens[g.leechPost(gp.i)].Pos)+1e-9 {
					t.Errorf("%d gens: the leech clamps on the outer post of a double", n)
				}
			}
			if sim.EdgeDist(g.r.Room, q) < 0.15 {
				t.Errorf("%d gens: the clamp is only %.2f m in from your boundary", n, sim.EdgeDist(g.r.Room, q))
			}
		}
	}
	if !found {
		t.Skip("no double generators in this room")
	}
}
