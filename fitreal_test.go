package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestFitRealCentered(t *testing.T) {
	tmp := t.TempDir()
	pimax := filepath.Join(tmp, "Pimax", "runtime")
	os.MkdirAll(pimax, 0755)
	t.Setenv("ProgramData", tmp)
	t.Setenv("LOCALAPPDATA", filepath.Join(tmp, "none"))
	src, _ := os.ReadFile("/mnt/user-data/uploads/chaperone_info.vrchap")
	os.WriteFile(filepath.Join(pimax, "chaperone_info.vrchap"), src, 0644)
	s, _ := loadStore("")
	P := s.pts
	var cx, cz, A float64
	for i := range P {
		p, q := P[i], P[(i+1)%len(P)]
		cr := p[0]*q[1] - q[0]*p[1]
		A += cr
		cx += (p[0] + q[0]) * cr
		cz += (p[1] + q[1]) * cr
	}
	cx, cz = cx/(3*A), cz/(3*A)
	g := rigidFromYaw(domAngle(P), cx, cz)
	w, h, ok := fitArea(g.inv().all(P), 0.1)
	t.Logf("centered+squared fit %.2f x %.2f %v (area %.1f of %.1f)", w, h, ok, w*h, math.Abs(polyArea(P)))
	if !ok || w*h < 6 {
		t.Fatal("expected a big fit")
	}
}
