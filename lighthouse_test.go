package main

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

func TestBaseStationsReal(t *testing.T) {
	b, err := os.ReadFile("testdata/lighthousedb.json")
	if err != nil {
		t.Skip("no data")
	}
	raw, id := parseBaseStations(b, "1789111411")
	if len(raw) != 2 || id != "1789111411" {
		t.Fatalf("got %d bases in %s", len(raw), id)
	}
	vb, _ := os.ReadFile("testdata/chaperone_info.vrchap")
	var doc map[string]any
	json.Unmarshal(vb, &doc)
	u := findUniverse(doc, "31")
	P, _ := boundsOf(u)
	tx, tz, yaw := standingXZYaw(u)
	s := &store{tx: tx, tz: tz, yaw: yaw, standY: standingY(u)}
	stP := s.toStageAll(P)
	for _, bs := range s.stageBases(raw) {
		in := pointInPoly(stP, bs.pos.x, bs.pos.z)
		d := outsideDist(stP, pt2{bs.pos.x, bs.pos.z})
		fwd := bs.back.mul(-1)
		t.Logf("%s pos %.2f %.2f %.2f inside=%v distToZone=%.2f fwd %.2f %.2f %.2f up %.2f", lhbName(bs.serial), bs.pos.x, bs.pos.y, bs.pos.z, in, d, fwd.x, fwd.y, fwd.z, bs.up.y)
		if bs.pos.y < 1.5 || bs.pos.y > 3 || fwd.y > 0 || bs.up.y < 0.85 || math.IsNaN(bs.pos.x) {
			t.Errorf("implausible base pose")
		}
	}
	var cx, cz float64
	for _, p := range stP {
		cx += p[0] / float64(len(stP))
		cz += p[1] / float64(len(stP))
	}
	t.Logf("zone centroid %.2f %.2f", cx, cz)
}
