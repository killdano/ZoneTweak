package main

// core: the shared world and hand types, colours and small helpers used by every part of
// the engine (portable: the Windows VR build and the Linux preview both compile this)

import (
	"fmt"
	"log"
	"math"
	"strings"
	"sync/atomic"
	"time"
)

const (
	bitMenu    = 1
	bitGrip    = 2
	bitStick   = 32
	bitTrigger = 33
	bitTool    = 3
	bitGrab    = 4

	grabRadius  = 0.20
	edgeRadius  = 0.15
	closeRadius = 0.25
)

var (
	cGreen  = rgba{0.24, 0.86, 0.52, 1}
	cAmber  = rgba{1.00, 0.70, 0.25, 1}
	cGhost  = rgba{0.42, 0.48, 0.45, 1}
	cGrid   = rgba{0.13, 0.18, 0.15, 1}
	cGrid1  = rgba{0.20, 0.27, 0.23, 1}
	cYellow = rgba{1.00, 0.84, 0.28, 1}
	cWhite  = rgba{0.90, 0.94, 0.92, 1}
	cMuted  = rgba{0.58, 0.65, 0.61, 1}
	cPanel  = rgba{0.06, 0.08, 0.07, 0.92}
	cButton = rgba{0.13, 0.17, 0.15, 1}
	cBtnHot = rgba{0.24, 0.86, 0.52, 1}
	cBtnOn  = rgba{0.10, 0.30, 0.20, 1}
	cBlue   = rgba{0.45, 0.70, 1.00, 1}
	cInk    = rgba{0.03, 0.08, 0.05, 1}
	worldUp = v3(0, 1, 0)
)

type hand struct {
	matchG0        rigid2
	idx            int
	valid          bool
	m              hmdMatrix34
	pressed, prev  uint64
	grab           int
	hoverV, hoverE int
	btn            int
	aim            hmdMatrix34
	aimValid       bool
	lineGrab       bool
	areaGrab       int
	areaHover      int
	grabOff        pt2
	menuRay        bool
	menuPt         vec3
	lf             *pointFilter
	touching       bool
	prevTip        vec3
	muzzle         vec3
	muzzleDir      vec3
	rayGrab        bool
	rayDist        float64
	rf             *pointFilter
	rayHit         vec3
	rayHitOK       bool
	prevBtn        int
	gAnchor        pt2
	gRipped, gNew  bool
	gBuzz          float64
	gJoint         bool
	lc             pt2
	lok            bool
	grip, trig     float64 // analog squeeze (0..1)
	force          float64 // Index: squeeze force (0 when the controller doesn't report it)
	aTouch, bTouch bool    // thumb resting on A / B
}

func (h *hand) down(b uint) bool { return h.pressed&(1<<b) != 0 && h.prev&(1<<b) == 0 }
func (h *hand) held(b uint) bool { return h.pressed&(1<<b) != 0 }
func (h *hand) cursor() pt2      { p := h.m.pos(); return pt2{p.x, p.z} }
func (h *hand) tip() vec3        { return h.m.xform(v3(0, 0, -0.09)) }

type button struct {
	label func() string
	act   func()
	span  int
	on    func() bool
}

type page struct {
	cols    int
	keyH    float64
	header  func() []string
	buttons func() []button
}

type editor struct {
	st                           *store
	lib                          spaceLib
	prof                         *pimaxProfile
	zone                         zoneCfg
	liveFailed                   atomic.Bool
	matching                     bool
	matchHand                    *hand
	matchAdj0                    rigid2
	matchPts0, matchOrig0        []pt2
	pageZone, pageArea, pageGfx  *page
	proxTest, laser              bool
	lineDrag                     float64
	center, center0              rigid2
	centerDirty, areaDirty       bool
	areaMode                     bool
	floorY                       float64
	floorAt                      time.Time
	floorWarned                  bool
	area, area0                  [2]float64
	laserHand                    *hand
	savedAny                     bool
	restartAfter                 bool
	pts, orig                    []pt2
	height                       float64
	theta                        float64
	undo, redo                   [][]pt2
	snap, ghost, feet            bool
	drawing                      bool
	draft                        []pt2
	draftHand                    *hand
	menuOpen                     bool
	menuOpenT                    float64
	menuCloseT                   float64
	menuWasOpen                  bool
	menuPos, menuR, menuU, menuN vec3
	page                         *page
	pageMain, pageSpaces, pageKb *page
	kbText                       string
	confirmDel                   time.Time
	status                       string
	statusAt                     time.Time
	dirty                        bool
	exitArmed                    time.Time
	quit                         bool
	hands                        []*hand
	hmd                          hmdMatrix34
	cur                          []button
}

var ed = &editor{ghost: true}

func (e *editor) say(format string, a ...any) {
	e.status = strings.ToUpper(fmt.Sprintf(format, a...))
	e.statusAt = time.Now()
	log.Println(e.status)
	setTitle("ZoneTweak - " + e.status)
}

func clonePts(P []pt2) []pt2 { return append([]pt2(nil), P...) }

func onoff(b bool) string {
	if b {
		return "ON"
	}
	return "OFF"
}

func ring(b *batch, c vec3, r float64, col rgba) {
	const n = 28
	for i := 0; i < n; i++ {
		a0, a1 := float64(i)/n*2*math.Pi, float64(i+1)/n*2*math.Pi
		b.line(c.add(v3(math.Cos(a0)*r, 0, math.Sin(a0)*r)), c.add(v3(math.Cos(a1)*r, 0, math.Sin(a1)*r)), col)
	}
}

func box(b *batch, m *hmdMatrix34, hx, hy, hz float64, col rgba) {
	var c [8]vec3
	for i := 0; i < 8; i++ {
		x, y, z := hx, hy, hz
		if i&1 == 0 {
			x = -x
		}
		if i&2 == 0 {
			y = -y
		}
		if i&4 == 0 {
			z = -z
		}
		c[i] = m.xform(v3(x, y, z))
	}
	for _, k := range [][2]int{{0, 1}, {2, 3}, {4, 5}, {6, 7}, {0, 2}, {1, 3}, {4, 6}, {5, 7}, {0, 4}, {1, 5}, {2, 6}, {3, 7}} {
		b.line(c[k[0]], c[k[1]], col)
	}
}

func post(b *batch, p vec3, s, h float64, col rgba) {
	a, c, d, f := p.add(v3(-s, 0, -s)), p.add(v3(s, 0, -s)), p.add(v3(s, 0, s)), p.add(v3(-s, 0, s))
	u := v3(0, h, 0)
	b.quad(a, c, c.add(u), a.add(u), col)
	b.quad(c, d, d.add(u), c.add(u), col)
	b.quad(d, f, f.add(u), d.add(u), col)
	b.quad(f, a, a.add(u), f.add(u), col)
	b.quad(a.add(u), c.add(u), d.add(u), f.add(u), col)
}

func billboard(cam, p vec3) (vec3, vec3) {
	f := cam.sub(p)
	f.y = 0
	if f.length() < 1e-4 {
		f = v3(0, 0, 1)
	}
	return worldUp.cross(f.norm()).norm(), worldUp
}

func boolF(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func wrapWords(s string, n int) []string {
	if len(s) <= n {
		return []string{s}
	}
	var out []string
	line := ""
	for _, w := range strings.Fields(s) {
		switch {
		case line == "":
			line = w
		case len(line)+1+len(w) <= n:
			line += " " + w
		default:
			out = append(out, line)
			line = w
		}
	}
	if line != "" || len(out) == 0 {
		out = append(out, clip(line, n))
	}
	return out
}

var bOpaque, bLines, bTrans batch

const (
	panelW   = 0.56
	btnGap   = 0.010
	headerH0 = 0.085
	footerH  = 0.032
	textPx   = 0.0027
	smallPx  = 0.0021
	panelPad = 0.02
)

var (
	cWarn   = rgba{1.00, 0.55, 0.15, 1}
	cDanger = rgba{1.00, 0.22, 0.20, 1}
	cArea   = rgba{1.00, 0.30, 0.85, 1}
	cLaser  = rgba{0.75, 0.95, 1.00, 1}
)

// laserReach: how far down the hall the hand lasers reach (the maze is built out there)
const laserReach = 30.0

func (h *hand) rawLaser() (pt2, bool) {
	if !h.aimValid {
		return pt2{}, false
	}
	o := h.aim.pos()
	dir := h.aim.axis(2).mul(-1)
	if dir.y > -0.01 {
		return pt2{}, false
	}
	t := -o.y / dir.y
	if t > laserReach || t < 0 {
		return pt2{}, false
	}
	return pt2{o.x + dir.x*t, o.z + dir.z*t}, true
}

func (h *hand) updateLaser(dt float64) {
	if h.lf == nil {
		h.lf = newPointFilter()
	}
	raw, ok := h.rawLaser()
	if !ok {
		h.lf.reset()
		h.lok = false
		return
	}
	h.lc, h.lok = h.lf.step(raw, dt), true
}

func (h *hand) pointer(laser bool) (pt2, bool) {
	if !laser {
		return h.cursor(), true
	}
	return h.lc, h.lok
}
