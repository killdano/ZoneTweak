//go:build windows

package main

// Contraband's side of the VR build: which room the game is played in, following the play
// space when the runtime re-centres it, and rooms traced by hand.

import (
	"encoding/json"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// gameRoom: the room Contraband is played in - one you traced by hand on this headset, or
// else the boundary you've got in ZoneTweak now (saved or not), or the runtime's own.
func gameRoom(dir string, st *store) []pt2 {
	if P := loadTraced(dir); len(P) >= 3 {
		roomSource = "traced"
		log.Printf("room: traced by hand on this headset, %d corners", len(P))
		return P
	}
	if st != nil && len(ed.pts) >= 3 && !placeholder(ed.pts) {
		roomSource = "headset"
		if ed.dirty {
			roomSource = "edited"
		}
		log.Printf("room: the %s boundary being edited, %d points", boundarySys, len(ed.pts))
		return clonePts(ed.pts)
	}
	return pickRoom(dir, st)
}

// movedSay: the play space was re-centred and the game's followed it.
func movedSay() {
	if G != nil {
		G.say("PLAY SPACE RE-CENTRED", "THE GAME MOVED WITH IT", cAmber, 3)
	}
}

// boundaryWatch: every couple of seconds, whether the runtime has rewritten the boundary file
// (it does when it re-centres) - and if so, where your room is in its coordinates now.
type boundaryWatch struct {
	st    *store
	files map[string]time.Time
	next  time.Time
}

func newBoundaryWatch(st *store) *boundaryWatch {
	w := &boundaryWatch{st: st, files: map[string]time.Time{}}
	if st == nil {
		return w
	}
	for _, f := range st.uniFiles {
		if fi, err := os.Stat(f); err == nil {
			w.files[f] = fi.ModTime()
		}
	}
	return w
}

func (w *boundaryWatch) check() bool {
	if w.st == nil || time.Now().Before(w.next) {
		return false
	}
	w.next = time.Now().Add(2 * time.Second)
	changed := false
	for f, t := range w.files {
		if fi, err := os.Stat(f); err == nil && !fi.ModTime().Equal(t) {
			w.files[f], changed = fi.ModTime(), true
		}
	}
	if !changed {
		return false
	}
	ns, err := loadStore("")
	if err != nil || ns.unis[w.st.uniID] == nil || ns.selectUniverse(w.st.uniID) != nil {
		log.Printf("boundary file changed, couldn't re-read universe %s: %v", w.st.uniID, err)
		return false
	}
	log.Printf("boundary file changed: universe %s, %d points", ns.uniID, len(ns.pts))
	return followStage(ns.pts)
}

// pickRoom: your room, from the best source this headset has - a room you traced by hand on
// it; Pimax's boundary or SteamVR's (when one of those runtimes is in charge and it's a real
// room, not a placeholder); the runtime's play-area rectangle; or, failing all that, a guess
// you'll want to trace over. Sets roomSource.
func pickRoom(dir string, st *store) []pt2 {
	rt := strings.ToLower(xs.runtimeName)
	log.Printf("OpenXR runtime %q, headset %q", xs.runtimeName, xs.systemName)
	if P := loadTraced(dir); len(P) >= 3 {
		roomSource = "traced"
		log.Printf("room: traced by hand on this headset, %d corners", len(P))
		return P
	}
	if strings.Contains(rt, "steamvr") {
		boundarySys = "SteamVR"
		live, err := steamvrLive()
		log.Printf("SteamVR live bounds: %d points, %.1f m2, %v", len(live), math.Abs(polyArea(live)), err)
		if len(live) >= 3 && !placeholder(live) {
			// SteamVR has a real room (a Quest's guardian, an Index's room) - its bounds come
			// in raw tracking space: into the stage's terms with SteamVR's standing pose
			log.Printf("SteamVR standing pose (ok %v): %v", steamStandingOK, steamStanding)
			for i, p := range live {
				log.Printf("  raw corner %d: %.2f, %.2f", i, p[0], p[1])
			}
			if steamStandingOK {
				// which reading of SteamVR's numbers matches your real room? (drawn on the
				// floor during setup, lettered, for a playtest to tell us)
				m := steamStanding
				fwd := make([]pt2, len(live))
				for i, p := range live {
					fwd[i] = pt2{m[0][0]*p[0] + m[0][2]*p[1] + m[0][3], m[2][0]*p[0] + m[2][2]*p[1] + m[2][3]}
				}
				boundsCandidates = map[string][]pt2{"A": rawToStanding(live), "B": live, "C": fwd}
				if w, d, ok := xrStageBounds(); ok {
					rect := []pt2{{-w / 2, -d / 2}, {w / 2, -d / 2}, {w / 2, d / 2}, {-w / 2, d / 2}}
					boundsCandidates["D"] = rect
					e := make([]pt2, 4)
					for i, p := range rect {
						e[i] = pt2{m[0][0]*p[0] + m[0][2]*p[1] + m[0][3], m[2][0]*p[0] + m[2][2]*p[1] + m[2][3]}
					}
					boundsCandidates["E"] = e
					boundsCandidates["F"] = rawToStanding(rect)
				}
				for k, P := range boundsCandidates {
					log.Printf("  candidate %s: %v", k, P)
				}
				P := rawToStanding(live)
				for i, p := range P {
					log.Printf("  stage corner %d: %.2f, %.2f", i, p[0], p[1])
				}
				roomSource = "headset"
				if len(P) == 4 {
					roomSource = "rectangle"
				}
				log.Printf("room: SteamVR's live boundary in stage terms, %d points", len(P))
				return P
			}
			if w, d, ok := xrStageBounds(); ok && w*d >= 1.5 {
				roomSource = "rectangle"
				log.Printf("room: SteamVR's %.2f x %.2f play area", w, d)
				return []pt2{{-w / 2, -d / 2}, {w / 2, -d / 2}, {w / 2, d / 2}, {-w / 2, d / 2}}
			}
		}
		if st != nil {
			st.useSteamUniverse(live)
		}
	}
	if st != nil && (strings.Contains(rt, "pimax") || strings.Contains(rt, "steamvr")) {
		for _, id := range st.ids {
			if u := st.unis[id]; u != nil {
				log.Printf("  universe %s: %d points, pimax %v, from %s (%s)", id, u.points, u.pimax, u.source, u.newest.Format("2006-01-02 15:04"))
			}
		}
		if !placeholder(st.pts) {
			roomSource = "headset"
			log.Printf("room: %s boundary, universe %s, %d points", boundarySys, st.uniID, len(st.pts))
			for i, p := range st.pts {
				log.Printf("  room corner %d: %.2f, %.2f", i, p[0], p[1])
			}
			return append([]pt2(nil), st.pts...)
		}
	}
	if w, d, ok := xrStageBounds(); ok && w*d >= 1.5 {
		roomSource = "rectangle"
		log.Printf("room: the runtime's %.2f x %.2f play area", w, d)
		return []pt2{{-w / 2, -d / 2}, {w / 2, -d / 2}, {w / 2, d / 2}, {-w / 2, d / 2}}
	}
	roomSource = "default"
	log.Printf("room: no usable boundary - a 2.5 m square to start (trace your room)")
	return []pt2{{-1.25, -1.25}, {1.25, -1.25}, {1.25, 1.25}, {-1.25, 1.25}}
}

// tracedKey: rooms traced by hand are kept per runtime and headset.
func tracedKey() string { return xs.runtimeName + " | " + xs.systemName }

func loadTraced(dir string) []pt2 {
	var m map[string][]pt2
	b, err := os.ReadFile(filepath.Join(dir, "spaces", "traced.json"))
	if err != nil || json.Unmarshal(b, &m) != nil {
		return nil
	}
	return m[tracedKey()]
}

// saveTraced: keep (or, P nil, forget) the room traced on this headset.
func saveTraced(dir string, P []pt2) {
	path := filepath.Join(dir, "spaces", "traced.json")
	m := map[string][]pt2{}
	if b, err := os.ReadFile(path); err == nil {
		json.Unmarshal(b, &m)
	}
	if P == nil {
		delete(m, tracedKey())
	} else {
		m[tracedKey()] = P
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(path, b, 0644); err != nil {
		log.Printf("couldn't save the traced room: %v", err)
	}
}
