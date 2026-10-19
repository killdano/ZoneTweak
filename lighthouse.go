package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
)

type rawBase struct {
	serial          uint64
	pos             vec3
	right, up, back vec3
}

type baseStation struct {
	serial          uint64
	pos             vec3
	right, up, back vec3
}

type lhDB struct {
	BaseStations []struct {
		Config struct {
			Serial uint64 `json:"serialNumber"`
		} `json:"config"`
	} `json:"base_stations"`
	Universes []struct {
		ID    string `json:"id"`
		Bases []struct {
			Serial uint64 `json:"base_serial_number"`
			Target struct {
				Pose     []float64 `json:"pose"`
				Variance float64   `json:"variance"`
			} `json:"target_pose"`
		} `json:"base_stations"`
		Tilt struct {
			Pitch float64 `json:"pitch"`
			Roll  float64 `json:"roll"`
		} `json:"tilt"`
	} `json:"known_universes"`
}

func lighthouseDBPaths() []string {
	var out []string
	if pd := os.Getenv("ProgramData"); pd != "" {
		out = append(out, filepath.Join(pd, "Pimax", "runtime", "config", "lighthouse", "lighthousedb.json"))
	}
	for _, d := range configDirs() {
		out = append(out, filepath.Join(d, "config", "lighthouse", "lighthousedb.json"), filepath.Join(d, "lighthouse", "lighthousedb.json"))
	}
	return out
}

var reUniverse = regexp.MustCompile(`(?:Selected existing|Created new|Using) universe (\d+)`)

func currentLighthouseUniverse() string {
	la := os.Getenv("LOCALAPPDATA")
	if la == "" {
		return ""
	}
	logs, _ := filepath.Glob(filepath.Join(la, "Pimax", "runtime", "pvr_srv_log_*.txt"))
	sort.Strings(logs)
	for i := len(logs) - 1; i >= 0 && i >= len(logs)-4; i-- {
		b, err := os.ReadFile(logs[i])
		if err != nil {
			continue
		}
		m := reUniverse.FindAllSubmatch(b, -1)
		if len(m) > 0 {
			return string(m[len(m)-1][1])
		}
	}
	return ""
}

func rotTo(a, b vec3) [3]vec3 {
	a, b = a.norm(), b.norm()
	v := a.cross(b)
	s := v.length()
	c := a.dot(b)
	I := [3]vec3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
	if s < 1e-9 {
		return I
	}
	K := [3]vec3{{0, -v.z, v.y}, {v.z, 0, -v.x}, {-v.y, v.x, 0}}
	mm := func(A, B [3]vec3) [3]vec3 {
		var C [3]vec3
		for i := 0; i < 3; i++ {
			row := [3]float64{A[i].x, A[i].y, A[i].z}
			col := func(j int) float64 {
				return row[0]*[]float64{B[0].x, B[0].y, B[0].z}[j] + row[1]*[]float64{B[1].x, B[1].y, B[1].z}[j] + row[2]*[]float64{B[2].x, B[2].y, B[2].z}[j]
			}
			C[i] = vec3{col(0), col(1), col(2)}
		}
		return C
	}
	K2 := mm(K, K)
	f := (1 - c) / (s * s)
	var R [3]vec3
	for i := 0; i < 3; i++ {
		R[i] = I[i].add(K[i]).add(K2[i].mul(f))
	}
	return R
}

func matVec(R [3]vec3, v vec3) vec3 { return vec3{R[0].dot(v), R[1].dot(v), R[2].dot(v)} }

func quatRot(q [4]float64, v vec3) vec3 {
	u := vec3{q[0], q[1], q[2]}
	w := q[3]
	t := u.cross(v).add(v.mul(w))
	return v.add(u.cross(t).mul(2))
}

func parseBaseStations(data []byte, uni string) ([]rawBase, string) {
	var db lhDB
	if json.Unmarshal(data, &db) != nil || len(db.Universes) == 0 {
		return nil, ""
	}
	ui := len(db.Universes) - 1
	for i, u := range db.Universes {
		if u.ID == uni {
			ui = i
		}
	}
	u := db.Universes[ui]
	p, r := u.Tilt.Pitch, u.Tilt.Roll
	upB := vec3{-math.Sin(r) * math.Cos(p), math.Cos(r) * math.Cos(p), math.Sin(p)}
	R := rotTo(upB, vec3{0, 1, 0})
	var out []rawBase
	primary := false
	for _, b := range u.Bases {
		ps := b.Target.Pose
		if len(ps) < 7 {
			continue
		}
		ident := ps[0] == 0 && ps[1] == 0 && ps[2] == 0 && ps[4] == 0 && ps[5] == 0 && ps[6] == 0
		if ident {
			if primary {
				continue
			}
			primary = true
		}
		// target_pose is the universe origin expressed in this base station's frame,
		// so the base station's pose in the universe is its inverse.
		qi := [4]float64{-ps[0], -ps[1], -ps[2], ps[3]}
		if ident {
			qi = [4]float64{0, 0, 0, 1}
		}
		pos := quatRot(qi, vec3{ps[4], ps[5], ps[6]}).mul(-1)
		rb := rawBase{serial: b.Serial, pos: matVec(R, pos)}
		rb.right = matVec(R, quatRot(qi, vec3{1, 0, 0})).norm()
		rb.up = matVec(R, quatRot(qi, vec3{0, 1, 0})).norm()
		rb.back = matVec(R, quatRot(qi, vec3{0, 0, 1})).norm()
		out = append(out, rb)
	}
	return out, u.ID
}

func loadRawBaseStations() ([]rawBase, string, string) {
	uni := currentLighthouseUniverse()
	for _, p := range lighthouseDBPaths() {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if bs, id := parseBaseStations(b, uni); len(bs) > 0 {
			return bs, id, p
		}
	}
	return nil, "", ""
}

func (s *store) stagePoint(p vec3) vec3 {
	q := s.adjust().apply(s.toStageMode(s.mode, pt2{p.x, p.z}))
	return vec3{q[0], p.y - s.standY, q[1]}
}

func (s *store) stageDir(o, d vec3) vec3 {
	a := s.stagePoint(o)
	b := s.stagePoint(o.add(d))
	return b.sub(a).norm()
}

func (s *store) stageBases(raw []rawBase) []baseStation {
	out := make([]baseStation, 0, len(raw))
	for _, r := range raw {
		out = append(out, baseStation{
			serial: r.serial,
			pos:    s.stagePoint(r.pos),
			right:  s.stageDir(r.pos, r.right),
			up:     s.stageDir(r.pos, r.up),
			back:   s.stageDir(r.pos, r.back),
		})
	}
	return out
}

func lhbName(serial uint64) string { return "LHB-" + strconv.FormatUint(serial, 16) }
