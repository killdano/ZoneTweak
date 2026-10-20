package main

import (
	"math"
	"os"
	"path/filepath"
	"time"
)

// zoneCfg: the boundary system's own safety settings (Pimax's profile, or SteamVR's settings)
type zoneCfg interface {
	sysName() string
	defaultDist() float64
	warnDistance() (float64, bool)
	setWarnDistance(v float64, clear bool) (float64, error)
	zoneOn() bool
	setZoneOn(on bool) error
	needsRestart() bool
}

func (p *pimaxProfile) sysName() string      { return "Pimax" }
func (p *pimaxProfile) defaultDist() float64 { return 0.5 }
func (p *pimaxProfile) needsRestart() bool   { return p.changed }

// boundarySys names whoever owns the boundary on this machine, for messages and tips
var boundarySys = "Pimax"

func steamMode() bool { return boundarySys == "SteamVR" }

const (
	svrSection     = "collisionBounds"
	svrFade        = "CollisionBoundsFadeDistance"
	svrAlpha       = "CollisionBoundsColorGammaA"
	svrDefaultFade = 0.7
)

// steamZone maps the zone settings onto SteamVR's chaperone settings: the warning distance is
// the fade distance, and the zone switch is the bounds' alpha. Values are read from
// steamvr.vrsettings and written live through SteamVR (set), which saves the file itself.
type steamZone struct {
	path      string
	set       func(key, kind string, v float64) error
	cache     map[string]any
	cachedAt  time.Time
	fade      *float64
	fadeKnown bool
	alpha     *int
	prevAlpha int
}

func newSteamZone(configDirs []string, set func(key, kind string, v float64) error) *steamZone {
	s := &steamZone{set: set}
	for _, d := range configDirs {
		p := filepath.Join(d, "steamvr.vrsettings")
		if _, err := os.Stat(p); err == nil {
			s.path = p
			break
		}
	}
	return s
}

func (s *steamZone) sysName() string      { return "SteamVR" }
func (s *steamZone) defaultDist() float64 { return svrDefaultFade }
func (s *steamZone) needsRestart() bool   { return false }

func (s *steamZone) section() map[string]any {
	if s.cache == nil || time.Since(s.cachedAt) > time.Second {
		s.cache, s.cachedAt = map[string]any{}, time.Now()
		if s.path != "" {
			if m, err := readJSON(s.path); err == nil {
				if sec, ok := m[svrSection].(map[string]any); ok {
					s.cache = sec
				}
			}
		}
	}
	return s.cache
}

func (s *steamZone) warnDistance() (float64, bool) {
	if s.fadeKnown {
		if s.fade == nil {
			return 0, false
		}
		return *s.fade, true
	}
	v, ok := s.section()[svrFade]
	if !ok || v == nil {
		return 0, false
	}
	return num(v), true
}

func (s *steamZone) setWarnDistance(v float64, clear bool) (float64, error) {
	if clear {
		if err := s.set(svrFade, "remove", 0); err != nil {
			return 0, err
		}
		s.fade, s.fadeKnown = nil, true
		return 0, nil
	}
	v = math.Round(math.Max(warnDistMin, math.Min(warnDistMax, v))*100) / 100
	if err := s.set(svrFade, "float", v); err != nil {
		return v, err
	}
	s.fade, s.fadeKnown = &v, true
	return v, nil
}

func (s *steamZone) alphaNow() int {
	if s.alpha != nil {
		return *s.alpha
	}
	if v, ok := s.section()[svrAlpha]; ok && v != nil {
		return int(num(v))
	}
	return 255
}

func (s *steamZone) zoneOn() bool { return s.alphaNow() > 0 }

func (s *steamZone) setZoneOn(on bool) error {
	if !on {
		if a := s.alphaNow(); a > 0 {
			s.prevAlpha = a
		}
		if err := s.set(svrAlpha, "int", 0); err != nil {
			return err
		}
		z := 0
		s.alpha = &z
		return nil
	}
	a := s.prevAlpha
	if a <= 0 || a == 255 {
		if err := s.set(svrAlpha, "remove", 0); err != nil {
			return err
		}
		a = 255
	} else if err := s.set(svrAlpha, "int", float64(a)); err != nil {
		return err
	}
	s.alpha = &a
	return nil
}
