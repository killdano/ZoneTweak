package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func vrchap(t *testing.T, path string, unis map[string][]pt2, times map[string]string) {
	var us []any
	for id, P := range unis {
		us = append(us, map[string]any{"universeID": id, "time": times[id], "collision_bounds": makeBounds(P, 2.43),
			"play_area": []any{2.0, 2.0}, "standing": map[string]any{"translation": []any{0.0, 0.0, 0.0}, "yaw": 0.0}})
	}
	b, _ := json.Marshal(map[string]any{"jsonid": "chaperone_info", "universes": us, "version": 5})
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestSteamUniverse(t *testing.T) {
	tmp := t.TempDir()
	steamCfg := filepath.Join(tmp, "steam", "config")
	ovrDir := filepath.Join(tmp, "openvr")
	t.Setenv("ProgramData", tmp)
	t.Setenv("LOCALAPPDATA", tmp)
	os.MkdirAll(ovrDir, 0755)
	os.WriteFile(filepath.Join(ovrDir, "openvrpaths.vrpath"), []byte(`{"config":[`+string(must2(json.Marshal(steamCfg)))+`]}`), 0644)
	room := []pt2{{-1, -1}, {2, -1}, {2, 1.5}, {-1, 1.5}}
	other := []pt2{{-2, -2}, {2, -2}, {2, 2}, {-2, 2}, {-2.5, 0}}
	small := []pt2{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	vrchap(t, filepath.Join(steamCfg, "chaperone_info.vrchap"), map[string][]pt2{"100": room, "200": other, "300": small},
		map[string]string{"100": "2026-01-01 10:00:00", "200": "2026-09-01 10:00:00", "300": "2025-01-01 10:00:00"})
	vrchap(t, filepath.Join(tmp, "Pimax", "runtime", "chaperone_info.vrchap"), map[string][]pt2{"900": other}, map[string]string{"900": "2026-10-01 10:00:00"})
	s, err := loadStore("")
	if err != nil {
		t.Fatal(err)
	}
	// SteamVR shows the 100 room, moved and turned into its standing frame
	live := rigidFromYaw(0.6, 0.4, -0.3).all(room)
	if id := s.steamUniverse(live); id != "100" {
		t.Fatal("live match picked", id)
	}
	// nothing live: the newest universe in SteamVR's own file, never Pimax's
	if id := s.steamUniverse(nil); id != "200" {
		t.Fatal("newest picked", id)
	}
	if !isPimaxFile(filepath.Join(tmp, "Pimax", "runtime", "x.vrchap")) || isPimaxFile(filepath.Join(steamCfg, "x.vrchap")) {
		t.Fatal("isPimaxFile")
	}
}

// A Pimax headset under SteamVR: SteamVR's own boundary is Pimax's 1 m placeholder square
// (Pimax's compositor draws the real one), so the Pimax boundary stays. A real SteamVR room
// still wins.
func TestSteamPlaceholderKeepsPimax(t *testing.T) {
	tmp := t.TempDir()
	steamCfg := filepath.Join(tmp, "steam", "config")
	ovrDir := filepath.Join(tmp, "openvr")
	t.Setenv("ProgramData", tmp)
	t.Setenv("LOCALAPPDATA", tmp)
	os.MkdirAll(ovrDir, 0755)
	os.WriteFile(filepath.Join(ovrDir, "openvrpaths.vrpath"), []byte(`{"config":[`+string(must2(json.Marshal(steamCfg)))+`]}`), 0644)
	square := []pt2{{-0.5, -0.5}, {-0.5, 0.5}, {0.5, 0.5}, {0.5, -0.5}}
	room := []pt2{{-1.5, -2}, {1.5, -2}, {1.6, 2.4}, {-1.4, 2.5}, {-1.6, 0}}
	vrchap(t, filepath.Join(steamCfg, "chaperone_info.vrchap"), map[string][]pt2{"30": square}, map[string]string{"30": "2026-10-02 15:00:55"})
	pim := filepath.Join(tmp, "Pimax", "runtime", "chaperone_info_openvr.vrchap")
	b, _ := json.Marshal(map[string]any{"jsonid": "chaperone_info", "version": 5, "universes": []any{
		map[string]any{"universeID": "30", "collision_bounds": makeBounds(square, 2.43), "standing": map[string]any{"translation": []any{0.0, 0.0, 0.0}, "yaw": 0.0}},
		map[string]any{"universeID": "31", "center_calied": true, "floor_calied": true, "collision_bounds": makeBounds(room, 2.43), "standing": map[string]any{"translation": []any{0.0, 0.0, 0.0}, "yaw": 0.0}},
	}})
	os.MkdirAll(filepath.Dir(pim), 0755)
	os.WriteFile(pim, b, 0644)
	s, err := loadStore("")
	if err != nil {
		t.Fatal(err)
	}
	if s.uniID != "31" {
		t.Fatal("should start on the Pimax boundary, got", s.uniID)
	}
	if id := s.useSteamUniverse(square); id != "31" || len(s.pts) != len(room) {
		t.Fatalf("SteamVR's placeholder square replaced the Pimax boundary (%s, %d points)", id, len(s.pts))
	}
	// a real room in SteamVR's own file: that's the one
	vrchap(t, filepath.Join(steamCfg, "chaperone_info.vrchap"), map[string][]pt2{"30": square, "40": room}, map[string]string{"30": "2026-10-02 15:00:55", "40": "2026-10-03 10:00:00"})
	s, _ = loadStore("")
	if id := s.useSteamUniverse(room); id != "40" {
		t.Fatal("a real SteamVR room should be used, got", id)
	}
}

func must2(b []byte, _ error) []byte { return b }

func TestSteamZone(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "steamvr.vrsettings"), []byte(`{"collisionBounds":{"CollisionBoundsFadeDistance":0.55,"CollisionBoundsColorGammaA":180}}`), 0644)
	var calls []string
	z := newSteamZone([]string{filepath.Join(tmp, "nope"), tmp}, func(key, kind string, v float64) error {
		calls = append(calls, key+" "+kind)
		return nil
	})
	if v, ok := z.warnDistance(); !ok || v != 0.55 {
		t.Fatal("read fade", v, ok)
	}
	if v, _ := z.setWarnDistance(0.62, false); v != 0.62 {
		t.Fatal("set fade", v)
	}
	if v, _ := z.warnDistance(); v != 0.62 {
		t.Fatal("cached fade", v)
	}
	if !z.zoneOn() {
		t.Fatal("zone should be on")
	}
	z.setZoneOn(false)
	if z.zoneOn() {
		t.Fatal("zone should be off")
	}
	z.setZoneOn(true)
	if z.alphaNow() != 180 {
		t.Fatal("alpha restore", z.alphaNow())
	}
	z.setWarnDistance(0, true)
	if _, ok := z.warnDistance(); ok {
		t.Fatal("cleared fade")
	}
	want := []string{"CollisionBoundsFadeDistance float", "CollisionBoundsColorGammaA int", "CollisionBoundsColorGammaA int", "CollisionBoundsFadeDistance remove"}
	if len(calls) != len(want) {
		t.Fatal(calls)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatal(calls)
		}
	}
}
