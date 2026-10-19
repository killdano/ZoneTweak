package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type uniInfo struct {
	id     string
	pimax  bool
	points int
	files  []string
	newest time.Time
	source string
}

type store struct {
	files    []string
	unis     map[string]*uniInfo
	ids      []string
	uniID    string
	uniFiles []string
	source   string
	pts      []pt2
	height   float64
	standY   float64
	tx, tz   float64
	yaw      float64
	mode     int
	rawPts   []pt2
	adj      rigid2
	playArea [2]float64
	newStand *[3]float64
	newArea  *[2]float64
	backedUp map[string]bool
}

func configDirs() []string {
	var dirs []string
	pd := os.Getenv("ProgramData")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	dirs = append(dirs, filepath.Join(pd, "Pimax", "runtime"))
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		if b, err := os.ReadFile(filepath.Join(la, "openvr", "openvrpaths.vrpath")); err == nil {
			var v struct {
				Config []string `json:"config"`
			}
			if json.Unmarshal(b, &v) == nil {
				dirs = append(dirs, v.Config...)
			}
		}
		dirs = append(dirs, filepath.Join(la, "openvr"))
	}
	dirs = append(dirs, steamConfigDirs()...)
	seen := map[string]bool{}
	var out []string
	for _, d := range dirs {
		d = filepath.Clean(d)
		k := strings.ToLower(d)
		if seen[k] {
			continue
		}
		seen[k] = true
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			out = append(out, d)
		}
	}
	return out
}

func readJSON(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var m map[string]any
	if err := d.Decode(&m); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return m, nil
}

func num(v any) float64 {
	switch t := v.(type) {
	case json.Number:
		f, _ := t.Float64()
		return f
	case float64:
		return t
	}
	return 0
}

func universes(doc map[string]any) []map[string]any {
	arr, _ := doc["universes"].([]any)
	var out []map[string]any
	for _, u := range arr {
		if m, ok := u.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func uniID(u map[string]any) string { return fmt.Sprint(u["universeID"]) }

func isPimax(u map[string]any) bool {
	_, a := u["center_calied"]
	_, b := u["floor_calied"]
	return a || b
}

func boundsOf(u map[string]any) ([]pt2, float64) {
	arr, _ := u["collision_bounds"].([]any)
	var pts []pt2
	h := 0.0
	for _, q := range arr {
		quad, _ := q.([]any)
		if len(quad) < 4 {
			continue
		}
		v0, _ := quad[0].([]any)
		if len(v0) < 3 {
			continue
		}
		pts = append(pts, pt2{num(v0[0]), num(v0[2])})
		for _, vv := range quad {
			if v, ok := vv.([]any); ok && len(v) >= 3 {
				h = math.Max(h, num(v[1]))
			}
		}
	}
	if h <= 0 {
		h = 2.43
	}
	return pts, h
}

func standingY(u map[string]any) float64 {
	st, _ := u["standing"].(map[string]any)
	tr, _ := st["translation"].([]any)
	if len(tr) >= 2 {
		return num(tr[1])
	}
	return 0
}

func findUniverse(doc map[string]any, id string) map[string]any {
	for _, u := range universes(doc) {
		if uniID(u) == id {
			return u
		}
	}
	return nil
}

func loadStore(runtimeUni string) (*store, error) {
	s := &store{unis: map[string]*uniInfo{}, backedUp: map[string]bool{}}
	seen := map[string]bool{}
	for _, d := range configDirs() {
		m, _ := filepath.Glob(filepath.Join(d, "chaperone_info*.vrchap"))
		sort.Strings(m)
		for _, f := range m {
			if k := strings.ToLower(f); !seen[k] {
				seen[k] = true
				s.files = append(s.files, f)
			}
		}
	}
	if len(s.files) == 0 {
		return nil, errors.New("no chaperone_info.vrchap found in the SteamVR config folders; run room setup once first")
	}
	for _, f := range s.files {
		doc, err := readJSON(f)
		if err != nil {
			continue
		}
		fi, _ := os.Stat(f)
		for _, u := range universes(doc) {
			id := uniID(u)
			inf := s.unis[id]
			if inf == nil {
				inf = &uniInfo{id: id}
				s.unis[id] = inf
				s.ids = append(s.ids, id)
			}
			p, _ := boundsOf(u)
			inf.pimax = inf.pimax || isPimax(u)
			inf.files = append(inf.files, f)
			pimaxFile := strings.Contains(strings.ToLower(f), `pimax`)
			if fi != nil && (inf.source == "" || (pimaxFile && !strings.Contains(strings.ToLower(inf.source), `pimax`)) || (pimaxFile == strings.Contains(strings.ToLower(inf.source), `pimax`) && fi.ModTime().After(inf.newest))) {
				inf.newest, inf.source, inf.points = fi.ModTime(), f, len(p)
			}
		}
	}
	if len(s.ids) == 0 {
		return nil, errors.New("the chaperone files have no universes; run room setup once first")
	}
	sort.Strings(s.ids)
	pick := ""
	for _, id := range s.ids {
		if s.unis[id].pimax && (pick == "" || s.unis[id].points > s.unis[pick].points) {
			pick = id
		}
	}
	if pick == "" && s.unis[runtimeUni] != nil {
		pick = runtimeUni
	}
	if pick == "" {
		for _, id := range s.ids {
			if pick == "" || s.unis[id].points > s.unis[pick].points {
				pick = id
			}
		}
	}
	return s, s.selectUniverse(pick)
}

func (s *store) selectUniverse(id string) error {
	inf := s.unis[id]
	if inf == nil {
		return fmt.Errorf("universe %s not found", id)
	}
	doc, err := readJSON(inf.source)
	if err != nil {
		return err
	}
	u := findUniverse(doc, id)
	if u == nil {
		return fmt.Errorf("universe %s missing from %s", id, inf.source)
	}
	s.uniID = id
	s.uniFiles = inf.files
	s.source = inf.source
	s.rawPts, s.height = boundsOf(u)
	s.standY = standingY(u)
	s.tx, s.tz, s.yaw = standingXZYaw(u)
	s.playArea = [2]float64{1, 1}
	if pa, ok := u["play_area"].([]any); ok && len(pa) >= 2 {
		s.playArea = [2]float64{num(pa[0]), num(pa[1])}
	}
	s.pts = s.toStageAll(s.rawPts)
	return nil
}

func (s *store) nextUniverse() string {
	for i, id := range s.ids {
		if id == s.uniID {
			return s.ids[(i+1)%len(s.ids)]
		}
	}
	return s.uniID
}

func makeBounds(P []pt2, h float64) []any {
	r := func(v float64) float64 { return math.Round(v*1e6) / 1e6 }
	out := make([]any, 0, len(P))
	for i, p := range P {
		q := P[(i+1)%len(P)]
		out = append(out, []any{
			[]any{r(p[0]), 0.0, r(p[1])},
			[]any{r(p[0]), h, r(p[1])},
			[]any{r(q[0]), h, r(q[1])},
			[]any{r(q[0]), 0.0, r(q[1])},
		})
	}
	return out
}

func (s *store) save(P []pt2, h, floorDelta float64) ([]string, []string, error) {
	stamp := time.Now().Format("20060102-150405")
	var written, skipped []string
	P = s.toFileAll(P)
	for _, path := range s.uniFiles {
		doc, err := readJSON(path)
		if err != nil {
			skipped = append(skipped, path)
			continue
		}
		u := findUniverse(doc, s.uniID)
		if u == nil {
			skipped = append(skipped, path)
			continue
		}
		u["collision_bounds"] = makeBounds(P, h)
		r6 := func(v float64) float64 { return math.Round(v*1e6) / 1e6 }
		if s.newStand != nil {
			if st, ok := u["standing"].(map[string]any); ok {
				if tr, ok := st["translation"].([]any); ok && len(tr) >= 3 {
					tr[0], tr[2] = r6(s.newStand[0]), r6(s.newStand[1])
				}
				st["yaw"] = r6(s.newStand[2])
			}
		}
		if s.newArea != nil {
			u["play_area"] = []any{r6(s.newArea[0]), r6(s.newArea[1])}
		}
		if floorDelta != 0 {
			if st, ok := u["standing"].(map[string]any); ok {
				if tr, ok := st["translation"].([]any); ok && len(tr) >= 2 {
					tr[1] = math.Round((num(tr[1])+floorDelta)*1e6) / 1e6
				}
			}
		}
		if !s.backedUp[strings.ToLower(path)] {
			if b, err := os.ReadFile(path); err == nil {
				if err := os.WriteFile(path+".bak-"+stamp, b, 0644); err != nil {
					return written, skipped, fmt.Errorf("couldn't back up %s: %w", path, err)
				}
			}
			s.backedUp[strings.ToLower(path)] = true
		}
		out, err := json.MarshalIndent(doc, "", "   ")
		if err != nil {
			return written, skipped, err
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, append(out, '\n'), 0644); err != nil {
			return written, skipped, fmt.Errorf("couldn't write %s: %w", path, err)
		}
		if err := os.Rename(tmp, path); err != nil {
			os.Remove(tmp)
			return written, skipped, fmt.Errorf("couldn't replace %s: %w", path, err)
		}
		written = append(written, path)
	}
	if len(written) == 0 {
		return nil, skipped, errors.New("no chaperone file contained this universe")
	}
	s.standY += floorDelta
	s.rawPts = P
	if s.newStand != nil {
		s.tx, s.tz, s.yaw = s.newStand[0], s.newStand[1], s.newStand[2]
		s.newStand = nil
	}
	if s.newArea != nil {
		s.playArea = *s.newArea
		s.newArea = nil
	}
	return written, skipped, nil
}

func standingXZYaw(u map[string]any) (float64, float64, float64) {
	st, _ := u["standing"].(map[string]any)
	tr, _ := st["translation"].([]any)
	if len(tr) < 3 {
		return 0, 0, 0
	}
	return num(tr[0]), num(tr[2]), num(st["yaw"])
}

const frameModes = 3

func frameModeName(m int) string {
	return [...]string{"A", "B", "OFF"}[m%frameModes]
}

func (s *store) rot(mode int) (float64, float64, bool) {
	switch mode % frameModes {
	case 0:
		return math.Cos(s.yaw), math.Sin(s.yaw), true
	case 1:
		return math.Cos(-s.yaw), math.Sin(-s.yaw), true
	}
	return 1, 0, false
}

func (s *store) toStageMode(mode int, p pt2) pt2 {
	c, sn, on := s.rot(mode)
	if !on {
		return p
	}
	dx, dz := p[0]-s.tx, p[1]-s.tz
	return pt2{c*dx - sn*dz, sn*dx + c*dz}
}

func (s *store) toFileMode(mode int, q pt2) pt2 {
	c, sn, on := s.rot(mode)
	if !on {
		return q
	}
	return pt2{c*q[0] + sn*q[1] + s.tx, -sn*q[0] + c*q[1] + s.tz}
}

func (s *store) adjust() rigid2 {
	if s.adj == (rigid2{}) {
		s.adj = identity2()
	}
	return s.adj
}

func (s *store) toStageAll(P []pt2) []pt2 {
	out := make([]pt2, len(P))
	for i, p := range P {
		out[i] = s.adjust().apply(s.toStageMode(s.mode, p))
	}
	return out
}

func (s *store) toFileAll(P []pt2) []pt2 {
	inv := s.adjust().inv()
	out := make([]pt2, len(P))
	for i, p := range P {
		out[i] = s.toFileMode(s.mode, inv.apply(p))
	}
	return out
}

func (s *store) standingRigid() rigid2 { return rigidFromYaw(-s.yaw, s.tx, s.tz) }

func standingFromRigid(r rigid2) [3]float64 {
	return [3]float64{r.tx, r.tz, -math.Atan2(r.c, r.a)}
}

func (s *store) setAdj(r rigid2) {
	s.adj = r
	s.pts = s.toStageAll(s.rawPts)
}

func (s *store) moveStage(r rigid2) {
	s.adj = s.adjust().then(r)
	s.pts = r.all(s.pts)
}

func (s *store) setMode(m int, P []pt2) []pt2 {
	out := make([]pt2, len(P))
	for i, p := range P {
		out[i] = s.toStageMode(m, s.toFileMode(s.mode, p))
	}
	s.mode = m % frameModes
	s.pts = s.toStageAll(s.rawPts)
	return out
}

func (s *store) oldestBackup() ([]pt2, string, error) {
	var baks []string
	for _, f := range s.uniFiles {
		m, _ := filepath.Glob(f + ".bak-*")
		baks = append(baks, m...)
	}
	if len(baks) == 0 {
		return nil, "", errors.New("no backups found")
	}
	sort.Slice(baks, func(i, j int) bool {
		return baks[i][strings.LastIndex(baks[i], ".bak-"):] < baks[j][strings.LastIndex(baks[j], ".bak-"):]
	})
	for _, b := range baks {
		doc, err := readJSON(b)
		if err != nil {
			continue
		}
		if u := findUniverse(doc, s.uniID); u != nil {
			p, _ := boundsOf(u)
			if len(p) >= 3 {
				return s.toStageAll(p), b, nil
			}
		}
	}
	return nil, "", errors.New("no backup has this universe")
}
