package main

import (
	"math"
	"testing"
)

// The lever into the game stands inside the boundary, half a metre in from its longest wall,
// facing into the room; a controller can take it and haul it down, and that starts the game.
func TestZoneLever(t *testing.T) {
	simClockOn, simClock = true, 100
	defer func() { simClockOn = false }()
	P := []pt2{{-1.5, -1}, {1.5, -1}, {1.5, 1}, {-1.5, 1}}
	z := &zoneLever{}
	z.place(P, false)
	if !z.ok() || !pointInPoly(P, z.at[0], z.at[1]) || math.Abs(distToBoundary(P, z.at)-zlInset) > 0.01 {
		t.Fatalf("the post should stand %.1f m inside: at %v", zlInset, z.at)
	}
	c := pt2{z.at[0] + z.face[0], z.at[1] + z.face[1]}
	if !pointInPoly(P, c[0], c[1]) {
		t.Fatalf("it should face into the room (%v)", z.face)
	}
	// busy: it doesn't move under you
	z.place([]pt2{{-3, -1}, {3, -1}, {3, 1}, {-3, 1}}, true)
	if !samePts(z.placed, P) {
		t.Fatal("re-placed while something was being moved")
	}
	h := &hand{idx: 1, valid: true, grab: -1}
	at := func(p vec3) {
		h.m = ident34()
		h.m[0][3], h.m[1][3], h.m[2][3] = float32(p.x), float32(p.y), float32(p.z)
	}
	at(z.l.handle())
	h.prev, h.pressed = 0, 1<<bitGrab
	if !z.handle(h) || z.by != h {
		t.Fatal("a controller on the handle with the grip should take it")
	}
	played := false
	onPlay = func() { played = true }
	defer func() { onPlay = nil }()
	h.prev = h.pressed
	// haul it toward you and down in steps
	for k := 0; k <= 20 && !z.fired; k++ {
		a := z.l.a0 + (z.l.a1-z.l.a0)*float64(k)/20
		at(z.l.piv.add(rotAbout(z.l.d0, z.l.ax, a).mul(z.l.L)))
		z.handle(h)
	}
	if !z.fired {
		t.Fatalf("pulled all the way down, it should fire (angle %.2f)", z.l.ang)
	}
	z.frame()
	if played {
		t.Fatal("the game starts after the flash, not on the same frame")
	}
	simClock += 1
	z.frame()
	if !played {
		t.Fatal("the pull should start the game")
	}
	// let go half way: it springs back
	z2 := &zoneLever{}
	z2.place(P, false)
	at(z2.l.handle())
	h.prev, h.pressed = 0, 1<<bitTrigger
	z2.handle(h)
	h.prev = h.pressed
	at(z2.l.piv.add(rotAbout(z2.l.d0, z2.l.ax, 1).mul(z2.l.L)))
	z2.handle(h)
	h.pressed = 0
	z2.handle(h)
	if z2.by != nil || z2.l.ang != z2.l.a0 || z2.fired {
		t.Fatal("let go part way, it should spring back up without starting anything")
	}
}

// No room for it (a sliver of a boundary): no lever, and no crash.
func TestZoneLeverNoRoom(t *testing.T) {
	z := &zoneLever{}
	z.place([]pt2{{0, 0}, {0.5, 0}, {0.5, 0.4}}, false)
	if z.ok() {
		t.Fatal("a tiny boundary has nowhere to stand it")
	}
	z.draw(nil)
}
