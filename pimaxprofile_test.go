package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPimaxProfile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LOCALAPPDATA", tmp)
	os.MkdirAll(filepath.Join(tmp, "Pimax", "runtime"), 0755)
	src := `{
   "" : null,
   "Pimax P3C9" : { "fov_level" : 0 },
   "disable_room_boundary_global" : 0,
   "ipd" : 0.06127000227570534
}`
	os.WriteFile(filepath.Join(tmp, "Pimax", "runtime", "profile.json"), []byte(src), 0644)
	p := newPimaxProfile()
	if _, ok := p.warnDistance(); ok || !p.zoneOn() {
		t.Fatal("initial")
	}
	if v, err := p.setWarnDistance(0.73, false); err != nil || v != 0.73 {
		t.Fatal(v, err)
	}
	if v, ok := p.warnDistance(); !ok || v != 0.73 {
		t.Fatal("readback", v)
	}
	if v, _ := p.setWarnDistance(9, false); v != warnDistMax {
		t.Fatal("clamp")
	}
	p.setZoneOn(false)
	if p.zoneOn() {
		t.Fatal("zone")
	}
	p.setWarnDistance(0, true)
	b, _ := os.ReadFile(p.path)
	s := string(b)
	if strings.Contains(s, keyWarnDist) || !strings.Contains(s, `"ipd": 0.06127000227570534`) || !strings.Contains(s, `"fov_level": 0`) || !strings.Contains(s, `"": null`) {
		t.Fatal(s)
	}
	if m, _ := filepath.Glob(p.path + ".bak-*"); len(m) != 1 {
		t.Fatal("backup")
	}
}
