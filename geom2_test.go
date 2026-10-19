package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestInsetAndFit(t *testing.T) {
	sq := []pt2{{-2, -1.5}, {2, -1.5}, {2, 1.5}, {-2, 1.5}}
	in := insetPoly(sq, 0.5)
	if math.Abs(in[0][0]+1.5) > 1e-9 || math.Abs(in[0][1]+1) > 1e-9 {
		t.Fatal("inset", in)
	}
	rev := []pt2{sq[3], sq[2], sq[1], sq[0]}
	if in2 := insetPoly(rev, 0.5); math.Abs(math.Abs(in2[0][0])-1.5) > 1e-9 {
		t.Fatal("inset reversed", in2)
	}
	if d := distToBoundary(sq, pt2{1.7, 0}); math.Abs(d-0.3) > 1e-9 {
		t.Fatal(d)
	}
	if distToBoundary(sq, pt2{5, 0}) != 0 {
		t.Fatal("outside")
	}
	w, h, ok := fitArea(sq, 0)
	if !ok || math.Abs(w-4) > 0.01 || math.Abs(h-3) > 0.01 {
		t.Fatal("fit", w, h)
	}
	L := []pt2{{-2, -2}, {2, -2}, {2, 0}, {0, 0}, {0, 2}, {-2, 2}}
	w, h, ok = fitArea(rigidFromYaw(0, 0.5, -0.5).inv().all(L), 0)
	if !ok || w <= 0 || h <= 0 {
		t.Fatal("L fit", w, h)
	}
	if rectFits(rigidFromYaw(0, 0.5, -0.5).inv().all(L), w/2+0.01, h/2+0.01) {
		t.Fatal("fit not maximal")
	}

	tmp := t.TempDir()
	pimax := filepath.Join(tmp, "Pimax", "runtime")
	os.MkdirAll(pimax, 0755)
	t.Setenv("ProgramData", tmp)
	t.Setenv("LOCALAPPDATA", filepath.Join(tmp, "none"))
	src, _ := os.ReadFile("/mnt/user-data/uploads/chaperone_info.vrchap")
	os.WriteFile(filepath.Join(pimax, "chaperone_info.vrchap"), src, 0644)
	s, _ := loadStore("")
	w, h, ok = fitArea(s.pts, 0.1)
	t.Logf("real room fit %.2f x %.2f ok=%v", w, h, ok)
	if !ok {
		t.Fatal("real room fit")
	}

	center := rigidFromYaw(0.4, 0.3, -0.2)
	m := center.then(s.adjust().inv())
	ns := standingFromRigid(m.then(s.standingRigid()))
	s.newStand = &ns
	area := [2]float64{w, h}
	s.newArea = &area
	if _, _, err := s.save(s.pts, 3, 0); err != nil {
		t.Fatal(err)
	}
	s.setAdj(center)
	s2, _ := loadStore("")
	for i := range s2.pts {
		back := center.apply(s2.pts[i])
		if dist2(back, s.pts[i]) > 1e-4 {
			t.Fatal("center change moved the boundary", i, back, s.pts[i])
		}
	}
	if math.Abs(s2.playArea[0]-w) > 1e-5 {
		t.Fatal("play area not saved")
	}
	raw, _ := boundsOf(findUniverse(must(readJSON(filepath.Join(pimax, "chaperone_info.vrchap"))), "31"))
	if math.Abs(raw[0][0]-(-1.454038500785828)) > 1e-5 {
		t.Fatal("raw boundary moved")
	}
}
