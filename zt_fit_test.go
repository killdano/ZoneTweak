package main

import (
	"math"
	"testing"
)

// FIT TOOL: the other hand grabs a tool and moves it; that tool keeps the new place on its
// controller (and the other tool mirrors it), and B saves it.
func TestToolFit(t *testing.T) {
	saveDir, saveHands := gloveDir, ed.hands
	defer func() { gloveDir, ed.hands, toolFit = saveDir, saveHands, [2]hmdMatrix34{ident34(), ident34()} }()
	gloveDir = t.TempDir()
	toolFit = [2]hmdMatrix34{ident34(), ident34()}
	at := func(x, y, z float64) hmdMatrix34 {
		m := ident34()
		m[0][3], m[1][3], m[2][3] = float32(x), float32(y), float32(z)
		return m
	}
	l := &hand{idx: 0, valid: true, aimValid: true}
	r := &hand{idx: 1, valid: true, aimValid: true}
	ed.hands = []*hand{l, r}
	pose := func(h *hand, raw hmdMatrix34) {
		h.rawM, h.rawAim = raw, raw
		h.m, h.aim = fitTool(h.idx, raw, raw)
	}
	pose(l, at(-0.1, 1, -0.3))
	pose(r, at(0.0, 1, -0.3)) // right hand right beside the left tool
	ed.startToolFit()
	r.prev, r.pressed = 0, 1<<bitGrab
	ed.toolFitFrame()
	if r.fitOther != l {
		t.Fatal("a grip next to the other tool should take hold of it")
	}
	// carry it 5 cm up with the right hand
	r.prev = r.pressed
	pose(r, at(0.0, 1.05, -0.3))
	ed.toolFitFrame()
	pose(l, at(-0.1, 1, -0.3))
	if d := toolFit[0].pos(); math.Abs(d.y-0.05) > 1e-3 {
		t.Fatalf("the left tool should have moved 5 cm up on its controller (%v)", d)
	}
	if m := mirrorFit(toolFit[0]); m != toolFit[1] {
		t.Fatal("the other tool should mirror it")
	}
	if l.aim.pos().y < 1.049 {
		t.Fatalf("the left tool's pose should carry the fit (%.3f)", l.aim.pos().y)
	}
	// let go, B: saved
	r.pressed = 0
	ed.toolFitFrame()
	r.prev, r.pressed = 0, 1<<bitMenu
	ed.toolFitFrame()
	if ed.toolFitting {
		t.Fatal("B should finish fitting")
	}
	kept := toolFit
	toolFit = [2]hmdMatrix34{ident34(), ident34()}
	loadToolFit()
	for i := range kept {
		for r := 0; r < 3; r++ {
			for c := 0; c < 4; c++ {
				if math.Abs(float64(kept[i][r][c]-toolFit[i][r][c])) > 1e-5 {
					t.Fatal("the fit should load back as it was saved")
				}
			}
		}
	}
}
