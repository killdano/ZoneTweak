package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type playSpace struct {
	Name     string       `json:"name"`
	Universe string       `json:"universe"`
	Height   float64      `json:"height"`
	Points   [][2]float64 `json:"points"`
	Frame    string       `json:"frame,omitempty"`
	Saved    string       `json:"saved"`
}

type spaceLib struct{ dir string }

func (l spaceLib) fileFor(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "space"
	}
	return filepath.Join(l.dir, s+".json")
}

func (l spaceLib) list() []playSpace {
	m, _ := filepath.Glob(filepath.Join(l.dir, "*.json"))
	var out []playSpace
	for _, f := range m {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var p playSpace
		if json.Unmarshal(b, &p) == nil && len(p.Points) >= 3 && p.Name != "" {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func (l spaceLib) find(name string) (playSpace, bool) {
	for _, p := range l.list() {
		if strings.EqualFold(strings.TrimSpace(p.Name), strings.TrimSpace(name)) {
			return p, true
		}
	}
	return playSpace{}, false
}

func (l spaceLib) save(name, universe string, pts []pt2, h float64, frame string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("name the space first")
	}
	if len(pts) < 3 {
		return errors.New("a space needs at least 3 points")
	}
	if err := os.MkdirAll(l.dir, 0755); err != nil {
		return err
	}
	p := playSpace{Name: name, Universe: universe, Height: h, Frame: frame, Saved: time.Now().Format(time.RFC3339)}
	for _, q := range pts {
		p.Points = append(p.Points, [2]float64{q[0], q[1]})
	}
	b, _ := json.MarshalIndent(p, "", "  ")
	return os.WriteFile(l.fileFor(name), b, 0644)
}

func (l spaceLib) remove(name string) error { return os.Remove(l.fileFor(name)) }

func (l spaceLib) active() string {
	b, _ := os.ReadFile(filepath.Join(l.dir, "active.txt"))
	return strings.TrimSpace(string(b))
}

func (l spaceLib) setActive(name string) {
	os.MkdirAll(l.dir, 0755)
	os.WriteFile(filepath.Join(l.dir, "active.txt"), []byte(name), 0644)
}

func (p playSpace) pts() []pt2 {
	out := make([]pt2, len(p.Points))
	for i, q := range p.Points {
		out[i] = pt2{q[0], q[1]}
	}
	return out
}

func (l spaceLib) frameMode() int {
	b, _ := os.ReadFile(filepath.Join(l.dir, "align.txt"))
	switch strings.TrimSpace(string(b)) {
	case "1":
		return 1
	case "2":
		return 2
	}
	return 0
}

func (l spaceLib) setFrameMode(m int) {
	os.MkdirAll(l.dir, 0755)
	os.WriteFile(filepath.Join(l.dir, "align.txt"), []byte(strings.TrimSpace(string(rune('0'+m)))), 0644)
}
