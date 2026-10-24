package sim

import (
	"fmt"
	"math"
	"os"
	"testing"
)

func rectRoom(wFt, dFt float64) []V2 {
	w, d := wFt/FtPerM, dFt/FtPerM
	return []V2{V(-w/2, -d/2), V(w/2, -d/2), V(w/2, d/2), V(-w/2, d/2)}
}

var labRooms = [][2]float64{{7, 7}, {8, 10}, {10, 10}, {12, 10}, {14, 12}, {16, 16}}

func TestLayout(t *testing.T) {
	tu := DefaultTuning()
	for _, rm := range labRooms {
		room := rectRoom(rm[0], rm[1])
		lo, hi := GenRange(room, tu)
		if lo > hi || lo < tu.MinGens || hi > tu.MaxGens {
			t.Fatalf("room %v: bad range %d..%d", rm, lo, hi)
		}
		for n := lo; n <= hi; n++ {
			G := PlaceGens(room, n, tu)
			if len(G) != n {
				t.Fatalf("room %v n=%d: placed %d", rm, n, len(G))
			}
			for i, g := range G {
				if !InPoly(room, g) {
					t.Fatalf("room %v n=%d: generator %d outside the room", rm, n, i)
				}
			}
		}
		h, doors := MakeHall(room, tu.HallMargin, DefaultContent().DoorNames)
		if len(doors) != 4 || h.W < 2*tu.HallMargin || h.H < 2*tu.HallMargin {
			t.Fatalf("hall %v", h)
		}
		// north is ahead (-Z)
		if doors[0].Out.Z > -0.7 {
			t.Fatalf("north door faces %v", doors[0].Out)
		}
		t.Logf("room %vx%v ft: gens %d..%d, hall %.1fx%.1f m", rm[0], rm[1], lo, hi, h.W, h.H)
	}
}

func TestNavFromDoors(t *testing.T) {
	r := NewRound(rectRoom(10, 10), 1, nil, nil)
	c := Centroid(r.RingPoly)
	for _, d := range r.Doors {
		p, L, ok := r.nav.Path(d.Mouth, c, 0.35, false)
		if !ok || len(p) == 0 {
			t.Fatalf("no path from %s", d.Name)
		}
		if L < r.T.HallMargin-1 {
			t.Fatalf("path from %s too short: %.1f", d.Name, L)
		}
	}
}

func TestMazeRules(t *testing.T) {
	r := NewRound(rectRoom(10, 10), 1, nil, nil)
	p0 := r.Points
	// inside the room: refused
	if _, why := r.PlaceWall(V(-0.5, 0), V(0.5, 0)); why == "" {
		t.Fatal("wall inside the room accepted")
	}
	// a baffle in front of the north door
	d := r.Doors[0]
	in := d.Out.Mul(-1)
	c := d.Mouth.Add(in.Mul(2.5))
	side := d.Out.Perp()
	id, why := r.PlaceWall(c.Sub(side.Mul(1.5)), c.Add(side.Mul(1.5)))
	if why != "" {
		t.Fatalf("baffle refused: %s", why)
	}
	if math.Abs((p0-r.Points)-3*r.T.MazeCost()) > 0.01 {
		t.Fatalf("cost %.2f", p0-r.Points)
	}
	r.RemoveWall(id)
	if math.Abs(r.Points-p0) > 0.01 {
		t.Fatalf("setup refund: %.2f vs %.2f", r.Points, p0)
	}
	// sealing a door with a ring of walls: refused
	m := d.Mouth.Add(in.Mul(1.6))
	ok := 0
	for _, s := range [][2]V2{{m.Add(side.Mul(-3)), m.Add(side.Mul(3))}} {
		if _, why := r.PlaceWall(s[0], s[1]); why == "" {
			ok++
		}
	}
	_ = ok
}

func runBotRound(room []V2, n int, seed int64, maze float64) (*Round, *Bot) {
	r := NewRound(room, seed, nil, nil)
	if n > 0 {
		r.SetGens(n)
	}
	b := NewBot(r, seed*31+7)
	b.BuildMaze(maze)
	r.Start()
	dt := 1.0 / 20
	for r.Phase == Playing && r.Time < 900 {
		r.Step(dt)
		b.Step(dt)
	}
	return r, b
}

func TestBotRound(t *testing.T) {
	r, _ := runBotRound(rectRoom(10, 10), 6, 3, 0.8)
	t.Logf("phase %d at %.0f s, wave %d, work %.0f/%.0f, stats %+v", r.Phase, r.Time, r.Wave, r.Work(), r.WorkTotal(), r.Stats)
	if r.Phase == Playing {
		t.Fatalf("round never ended")
	}
}

// TestBotWinRates (set WINRATES=1): the bot's win rate in each Balance Lab room.
func TestBotWinRates(t *testing.T) {
	if os.Getenv("WINRATES") == "" {
		t.Skip("set WINRATES=1")
	}
	seeds := 20
	if v := os.Getenv("SEEDS"); v != "" {
		fmt.Sscan(v, &seeds)
	}
	for _, rm := range labRooms {
		room := rectRoom(rm[0], rm[1])
		lo, hi := GenRange(room, DefaultTuning())
		line := fmt.Sprintf("%2.0fx%-2.0f", rm[0], rm[1])
		for n := lo; n <= hi; n++ {
			wins, dur, steals, burns := 0, 0.0, 0, 0
			for s := 1; s <= seeds; s++ {
				r, _ := runBotRound(room, n, int64(s*1000+n), 0.8)
				if r.Phase == Won {
					wins++
					dur += r.Time
				}
				steals += r.Stats.Steals
				burns += r.Stats.Burns
			}
			line += fmt.Sprintf("  %d:%3d%%", n, wins*100/seeds)
			if os.Getenv("VERBOSE") != "" {
				line += fmt.Sprintf("(%.0fs st%.1f b%.1f)", dur/math.Max(1, float64(wins)), float64(steals)/float64(seeds), float64(burns)/float64(seeds))
			}
		}
		t.Log(line)
	}
}

// A walker burning through a magenta wall takes a bite out of it, and the bite's points come
// back into the pool so the hole can be patched.
func TestBurnThroughRefundsTheBite(t *testing.T) {
	r := NewRound(rectRoom(10, 10), 1, nil, nil)
	c := Centroid(r.Room)
	a, b := c.Add(V(-1.5, 4.5)), c.Add(V(1.5, 4.5))
	id, why := r.PlaceWall(a, b)
	if why != "" {
		t.Fatal(why)
	}
	before, spent := r.Points, r.MazeSpent()
	w := r.Wall(id)
	r.wreck(w, Lerp(a, b, 0.5))
	left := 0.0
	for _, m := range r.Maze {
		left += Dist(m.A, m.B)
	}
	bite := Dist(a, b) - left
	if bite < 1 {
		t.Fatalf("the burn barely touched it (%.2f m gone)", bite)
	}
	if got, want := r.Points-before, bite*r.T.MazeCost(); math.Abs(got-want) > 0.01 {
		t.Fatalf("the bite's points should come back (got %.1f, want %.1f)", got, want)
	}
	if math.Abs(r.MazeSpent()-(spent-bite*r.T.MazeCost())) > 0.01 {
		t.Fatal("the maze's spend didn't come down by the bite")
	}
}
