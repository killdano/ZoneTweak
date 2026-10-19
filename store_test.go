package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	steamCfg := filepath.Join(tmp, "steam", "config")
	ovr := filepath.Join(tmp, "openvr")
	pimax := filepath.Join(tmp, "Pimax", "runtime")
	os.MkdirAll(pimax, 0755)
	t.Setenv("ProgramData", tmp)
	os.MkdirAll(steamCfg, 0755)
	os.MkdirAll(ovr, 0755)
	os.WriteFile(filepath.Join(ovr, "openvrpaths.vrpath"), []byte(`{"config":["`+steamCfg+`"]}`), 0644)
	src, _ := os.ReadFile("/mnt/user-data/uploads/chaperone_info.vrchap")
	os.WriteFile(filepath.Join(steamCfg, "chaperone_info.vrchap"), src, 0644)
	os.WriteFile(filepath.Join(ovr, "chaperone_info.vrchap"), src, 0644)
	os.WriteFile(filepath.Join(pimax, "chaperone_info.vrchap"), src, 0644)
	os.WriteFile(filepath.Join(pimax, "chaperone_info_openvr.vrchap"), src, 0644)
	t.Setenv("LOCALAPPDATA", tmp)
	s, err := loadStore("30")
	if err != nil {
		t.Fatal(err)
	}
	if s.uniID != "31" || len(s.pts) != 34 || len(s.uniFiles) != 4 || s.source != filepath.Join(pimax, "chaperone_info.vrchap") || s.height != 3 {
		t.Fatalf("bad load uni=%s pts=%d files=%v h=%v", s.uniID, len(s.pts), s.uniFiles, s.height)
	}
	P := squareUp(simplify(s.pts, 0.10), domAngle(s.pts), 0.20)
	w, skipped, err := s.save(P, 2.5, 0.1)
	if err != nil || len(w) != 4 || len(skipped) != 0 {
		t.Fatal(w, skipped, err)
	}
	s2, _ := loadStore("30")
	if len(s2.pts) != 4 || s2.height != 2.5 {
		t.Fatalf("reload %v", s2.pts)
	}
	for _, f := range []string{filepath.Join(ovr, "chaperone_info.vrchap"), filepath.Join(steamCfg, "chaperone_info.vrchap"), filepath.Join(pimax, "chaperone_info.vrchap"), filepath.Join(pimax, "chaperone_info_openvr.vrchap")} {
		doc, _ := readJSON(f)
		u := findUniverse(doc, "31")
		p, _ := boundsOf(u)
		if len(p) != 4 || u["center_calied"] != true {
			t.Fatal("not written", f)
		}
		if p, _ := boundsOf(findUniverse(doc, "30")); len(p) != 4 {
			t.Fatal("30 changed")
		}
	}
	if err := s2.selectUniverse(s2.nextUniverse()); err != nil || s2.uniID != "30" {
		t.Fatal("switch", err, s2.uniID)
	}
}

func TestFrame(t *testing.T) {
	tmp := t.TempDir()
	pimax := filepath.Join(tmp, "Pimax", "runtime")
	os.MkdirAll(pimax, 0755)
	t.Setenv("ProgramData", tmp)
	t.Setenv("LOCALAPPDATA", filepath.Join(tmp, "none"))
	src, _ := os.ReadFile("/mnt/user-data/uploads/chaperone_info.vrchap")
	os.WriteFile(filepath.Join(pimax, "chaperone_info.vrchap"), src, 0644)
	s, err := loadStore("")
	if err != nil {
		t.Fatal(err)
	}
	if !pointInPoly(s.pts, 0, 0) {
		t.Fatal("room center should be inside the boundary after conversion")
	}
	for m := 0; m < frameModes; m++ {
		for _, p := range s.rawPts {
			q := s.toFileMode(m, s.toStageMode(m, p))
			if math.Abs(q[0]-p[0]) > 1e-9 || math.Abs(q[1]-p[1]) > 1e-9 {
				t.Fatal("roundtrip", m)
			}
		}
	}
	P := clonePtsT(s.pts)
	if _, _, err := s.save(P, 3, 0); err != nil {
		t.Fatal(err)
	}
	s2, _ := loadStore("")
	for i := range P {
		if math.Abs(s2.pts[i][0]-P[i][0]) > 1e-5 {
			t.Fatal("save changed position")
		}
	}
	if raw, _ := boundsOf(findUniverse(must(readJSON(filepath.Join(pimax, "chaperone_info.vrchap"))), "31")); math.Abs(raw[0][0]-(-1.454038500785828)) > 1e-5 {
		t.Fatal("raw coords changed", raw[0])
	}
	if P2, from, err := s2.oldestBackup(); err != nil || len(P2) != 34 || from == "" {
		t.Fatal("backup", err)
	}
}

func clonePtsT(P []pt2) []pt2                       { return append([]pt2(nil), P...) }
func must(m map[string]any, _ error) map[string]any { return m }

func TestRigid(t *testing.T) {
	r := rigidFromYaw(0.7, 1.5, -2)
	q := rigidFromYaw(-1.1, 0.3, 0.4)
	p := pt2{0.25, -3}
	a := r.then(q).apply(p)
	b := q.apply(r.apply(p))
	c := r.inv().apply(r.apply(p))
	if math.Abs(a[0]-b[0]) > 1e-12 || math.Abs(a[1]-b[1]) > 1e-12 || math.Abs(c[0]-p[0]) > 1e-12 || math.Abs(c[1]-p[1]) > 1e-12 {
		t.Fatal(a, b, c)
	}
	tmp := t.TempDir()
	pimax := filepath.Join(tmp, "Pimax", "runtime")
	os.MkdirAll(pimax, 0755)
	t.Setenv("ProgramData", tmp)
	t.Setenv("LOCALAPPDATA", filepath.Join(tmp, "none"))
	src, _ := os.ReadFile("/mnt/user-data/uploads/chaperone_info.vrchap")
	os.WriteFile(filepath.Join(pimax, "chaperone_info.vrchap"), src, 0644)
	s, _ := loadStore("")
	moved := r.all(s.pts)
	s.moveStage(r)
	if _, _, err := s.save(moved, 3, 0); err != nil {
		t.Fatal(err)
	}
	raw, _ := boundsOf(findUniverse(must(readJSON(filepath.Join(pimax, "chaperone_info.vrchap"))), "31"))
	if math.Abs(raw[5][0]-2.376723051071167) > 1e-5 || math.Abs(raw[5][1]-(-2.229987621307373)) > 1e-5 {
		t.Fatal("moving the stage changed the physical boundary", raw[5])
	}
}

func TestStandingRigid(t *testing.T) {
	s := &store{tx: 0.096, tz: -2.186, yaw: -2.46}
	p := pt2{1.2, -0.4}
	a := s.standingRigid().apply(p)
	b := s.toFileMode(0, p)
	if math.Abs(a[0]-b[0]) > 1e-9 || math.Abs(a[1]-b[1]) > 1e-9 {
		t.Fatal(a, b)
	}
	st := standingFromRigid(s.standingRigid())
	if math.Abs(st[2]-s.yaw) > 1e-9 || math.Abs(st[0]-s.tx) > 1e-9 {
		t.Fatal(st)
	}
}
