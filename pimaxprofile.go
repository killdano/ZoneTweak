package main

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"time"
)

const (
	keyWarnDist = "room_boundary_warning_distance"
	keyZoneOff  = "disable_room_boundary_global"
	warnDistMin = 0.05
	warnDistMax = 2.0
)

type pimaxProfile struct {
	path     string
	backedUp bool
	cache    map[string]any
	cachedAt time.Time
	changed  bool
	onSet    func(key string, v float64, isFloat bool)
}

func (p *pimaxProfile) live(key string, v float64, isFloat bool) {
	if p.onSet != nil {
		p.onSet("runtime_"+key, v, isFloat)
	}
}

func newPimaxProfile() *pimaxProfile {
	la := os.Getenv("LOCALAPPDATA")
	return &pimaxProfile{path: filepath.Join(la, "Pimax", "runtime", "profile.json")}
}

func (p *pimaxProfile) read() (map[string]any, error) {
	if p.cache != nil && time.Since(p.cachedAt) < time.Second {
		return p.cache, nil
	}
	m, err := p.load()
	if err == nil {
		p.cache, p.cachedAt = m, time.Now()
	}
	return m, err
}

func (p *pimaxProfile) load() (map[string]any, error) {
	b, err := os.ReadFile(p.path)
	if err != nil {
		return nil, err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var m map[string]any
	if err := d.Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

func (p *pimaxProfile) write(m map[string]any) error {
	if !p.backedUp {
		if b, err := os.ReadFile(p.path); err == nil {
			os.WriteFile(p.path+".bak-"+time.Now().Format("20060102-150405"), b, 0644)
		}
		p.backedUp = true
	}
	out, err := json.MarshalIndent(m, "", "   ")
	if err != nil {
		return err
	}
	p.cache = nil
	p.changed = true
	tmp := p.path + ".tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, p.path)
}

func (p *pimaxProfile) warnDistance() (float64, bool) {
	m, err := p.read()
	if err != nil {
		return 0, false
	}
	v, ok := m[keyWarnDist]
	if !ok || v == nil {
		return 0, false
	}
	return num(v), true
}

func (p *pimaxProfile) setWarnDistance(v float64, clear bool) (float64, error) {
	m, err := p.load()
	if err != nil {
		return 0, err
	}
	if clear {
		delete(m, keyWarnDist)
		return 0, p.write(m)
	}
	v = math.Round(math.Max(warnDistMin, math.Min(warnDistMax, v))*100) / 100
	m[keyWarnDist] = v
	if err := p.write(m); err != nil {
		return v, err
	}
	p.live(keyWarnDist, v, true)
	return v, nil
}

func (p *pimaxProfile) zoneOn() bool {
	m, err := p.read()
	if err != nil {
		return true
	}
	return num(m[keyZoneOff]) == 0
}

func (p *pimaxProfile) setZoneOn(on bool) error {
	m, err := p.load()
	if err != nil {
		return err
	}
	val := 1.0
	if on {
		val = 0
	}
	m[keyZoneOff] = int(val)
	if err := p.write(m); err != nil {
		return err
	}
	p.live(keyZoneOff, val, false)
	return nil
}

const keyNoRecenter = "disable_recenter"

func (p *pimaxProfile) recenterOn() bool {
	m, err := p.read()
	if err != nil {
		return true
	}
	return num(m[keyNoRecenter]) == 0
}

func (p *pimaxProfile) setRecenterOn(on bool) error {
	m, err := p.load()
	if err != nil {
		return err
	}
	val := 1.0
	if on {
		val = 0
	}
	m[keyNoRecenter] = int(val)
	if err := p.write(m); err != nil {
		return err
	}
	p.live(keyNoRecenter, val, false)
	return nil
}
