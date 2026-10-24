package main

// Fitting the gloves to your real hands. Controllers sit in hands differently, so from the
// pause menu you can grab either glove with your other hand and move it onto where your
// real hand is. The fit is stored per hand as an offset from the controller's grip pose; the
// laser rides the fitted glove, and grabbing the ball on a laser re-aims it (aimFit).

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
)

var gloveFit = [2]hmdMatrix34{ident34(), ident34()}

// aimFit: a turn of each laser in its own frame (re-aimed by hand in fit mode)
var aimFit = [2]hmdMatrix34{ident34(), ident34()}

// aimOrb: how far along the laser its grab ball sits; aimMax: how far it can be turned
const (
	aimOrb = 0.3
	aimMax = 70 * math.Pi / 180
)

// gloveDir: where the fit is saved (the VR build sets it; empty means don't save)
var gloveDir string

func ident34() hmdMatrix34 {
	return hmdMatrix34{{1, 0, 0, 0}, {0, 1, 0, 0}, {0, 0, 1, 0}}
}

func gloveFile() string { return filepath.Join(gloveDir, "spaces", "gloves.txt") }

func saveGloves() {
	if gloveDir == "" {
		return
	}
	var b strings.Builder
	for _, m := range append(gloveFit[:], aimFit[:]...) {
		for r := 0; r < 3; r++ {
			for c := 0; c < 4; c++ {
				fmt.Fprintf(&b, "%.6f ", m[r][c])
			}
		}
		b.WriteString("\n")
	}
	os.MkdirAll(filepath.Dir(gloveFile()), 0755)
	os.WriteFile(gloveFile(), []byte(b.String()), 0644)
	logGloves("saved")
}

// logGloves: the fit into contraband.log (so a log carries it too)
func logGloves(why string) {
	names := []string{"left glove", "right glove", "left laser", "right laser"}
	for i, m := range append(gloveFit[:], aimFit[:]...) {
		var b strings.Builder
		for r := 0; r < 3; r++ {
			for c := 0; c < 4; c++ {
				fmt.Fprintf(&b, "%.6f ", m[r][c])
			}
		}
		log.Printf("fit %s %s: %s", why, names[i], strings.TrimSpace(b.String()))
	}
}

// gloveSaved: a fit was loaded from the save file (so no controller default applies)
var gloveSaved bool

// defaultFits: a starting fit for a controller type, until you fit your own (profile
// suffix -> left glove, right glove, left laser, right laser).
var defaultFits = map[string][4]hmdMatrix34{}

// onProfile: hand i turned out to be this controller: with no saved fit, use its default.
func onProfile(i int, prof string) {
	if gloveSaved {
		return
	}
	for suffix, f := range defaultFits {
		if strings.HasSuffix(prof, suffix) {
			gloveFit[i], aimFit[i] = f[i], f[2+i]
			log.Printf("hand %d: default fit for %s", i, suffix)
		}
	}
}

func loadGloves() {
	if gloveDir == "" {
		return
	}
	bs, err := os.ReadFile(gloveFile())
	if err != nil {
		return
	}
	gloveSaved = true
	defer logGloves("loaded")
	lines := strings.Split(strings.TrimSpace(string(bs)), "\n")
	for i := 0; i < 4 && i < len(lines); i++ {
		var v [12]float32
		n, _ := fmt.Sscan(lines[i], &v[0], &v[1], &v[2], &v[3], &v[4], &v[5], &v[6], &v[7], &v[8], &v[9], &v[10], &v[11])
		if n != 12 {
			continue
		}
		var m hmdMatrix34
		for k := 0; k < 12; k++ {
			m[k/4][k%4] = v[k]
		}
		// only sane fits: a rotation, and the glove within 25 cm of the controller
		off := math.Abs(float64(m[0][3])) + math.Abs(float64(m[1][3])) + math.Abs(float64(m[2][3]))
		switch {
		case !orthoOK(m):
		case i < 2 && off < 0.25:
			gloveFit[i] = m
		case i >= 2 && off < 1e-4 && float64(m[2][2]) > math.Cos(aimMax)-0.01:
			aimFit[i-2] = m
		}
	}
}

func orthoOK(m hmdMatrix34) bool {
	for i := 0; i < 3; i++ {
		a := m.axis(i)
		if math.Abs(a.length()-1) > 0.02 {
			return false
		}
		for j := i + 1; j < 3; j++ {
			if math.Abs(a.dot(m.axis(j))) > 0.02 {
				return false
			}
		}
	}
	return true
}

// mirrorFit: the same fit for the other hand - mirrored left to right in the controller's
// frame (both controllers share one axis convention; your hands are mirror images).
func mirrorFit(m hmdMatrix34) hmdMatrix34 {
	sgn := [3]float32{-1, 1, 1}
	var o hmdMatrix34
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			o[r][c] = sgn[r] * m[r][c] * sgn[c]
		}
		o[r][3] = sgn[r] * m[r][3]
	}
	return o
}

// turnTo: the rotation that turns unit vector a onto unit vector b (about their common
// perpendicular), as a pose with no offset.
func turnTo(a, b vec3) hmdMatrix34 {
	k := a.cross(b)
	sn, c := k.length(), a.dot(b)
	if sn < 1e-9 {
		return ident34()
	}
	k = k.mul(1 / sn)
	kk := [3]float64{k.x, k.y, k.z}
	K := [3][3]float64{{0, -k.z, k.y}, {k.z, 0, -k.x}, {-k.y, k.x, 0}}
	var m hmdMatrix34
	for r := 0; r < 3; r++ {
		for q := 0; q < 3; q++ {
			v := sn*K[r][q] + (1-c)*kk[r]*kk[q]
			if r == q {
				v += c
			}
			m[r][q] = float32(v)
		}
	}
	return m
}

// orthonormalize: keep a dragged fit a clean rotation (float32 round-off)
func orthonormalize(m hmdMatrix34) hmdMatrix34 {
	z := m.axis(2).norm()
	x := m.axis(1).cross(z).norm()
	y := z.cross(x)
	for r, v := range [3]vec3{x, y, z} {
		m[0][r], m[1][r], m[2][r] = float32(v.x), float32(v.y), float32(v.z)
	}
	return m
}

// ---- fit mode ----

func (g *game) buildFitPanel() {
	p := &uiPanel{w: 0.62, h: 0.38}
	p.show = func() bool { return g.fitting }
	p.lines = func() []string {
		return []string{
			"FIT YOUR GLOVES",
			"GRAB A GLOVE WITH YOUR OTHER HAND",
			"AND MOVE IT ONTO YOUR REAL HAND.",
			"~ROTATE IT TOO. LET GO TO KEEP IT.",
			"~THE OTHER GLOVE MIRRORS IT, LASER AND ALL.",
			"~GRAB THE BALL ON A LASER TO RE-AIM IT.",
		}
	}
	p.btns = []*uiBtn{
		{label: func() string { return "DONE" }, x: -0.15, y: -0.12, w: 0.2, h: 0.07, accent: cEnergy, act: func() { g.endFit() }},
		{label: func() string { return "RESET" }, x: 0.15, y: -0.12, w: 0.2, h: 0.07, accent: cAmber, act: func() {
			gloveFit = [2]hmdMatrix34{ident34(), ident34()}
			aimFit = [2]hmdMatrix34{ident34(), ident34()}
			saveGloves()
		}},
	}
	g.fitPanel = p
}

func (g *game) startFit() {
	g.menuOpen = false
	g.fitting = true
	for _, gh := range hands {
		if gh != nil {
			gh.held, gh.hover, gh.building, gh.fitDrag, gh.fitAim = nil, nil, false, -1, -1
		}
	}
	cam := ed.hmd.pos()
	fwd := ed.hmd.axis(2).mul(-1)
	fwd.y = 0
	if fwd.length() < 0.1 {
		fwd = v3(0, 0, -1)
	}
	g.fitPanel.pos = cam.add(fwd.norm().mul(0.75)).add(v3(0, 0.1, 0))
	g.fitPanel.face(cam)
}

func (g *game) endFit() {
	g.fitting = false
	for _, gh := range hands {
		if gh != nil {
			gh.fitDrag, gh.fitAim = -1, -1
		}
	}
	saveGloves()
}

// fitFrame: a closed hand near the other glove takes hold of it; the glove then rides with
// that hand until it lets go, and its offset from its own controller is what's kept.
func (g *game) fitFrame() {
	for i, a := range hands {
		if !a.valid() {
			continue
		}
		o := hands[1-i]
		if o == nil || !o.valid() {
			continue
		}
		switch {
		case a.grabDown && a.fitDrag < 0 && a.fitAim < 0 && o.aimBall().sub(a.palm).length() < 0.08:
			// the ball on the other hand's laser: re-aim it
			a.fitAim = 1 - i
			a.buzz(30, 0.6)
			o.buzz(30, 0.6)
		case a.fitAim >= 0 && !a.closed:
			a.fitAim = -1
			a.buzz(20, 0.4)
			saveGloves()
		case a.fitAim >= 0:
			// the laser points from where it leaves the glove toward your grabbing hand
			B := o.aimBase()
			want := a.palm.sub(B.pos())
			if want.length() < 0.05 {
				break
			}
			inv := inv34(B)
			dl := inv.xform(B.pos().add(want.norm())).sub(inv.xform(B.pos())).norm()
			fwd := v3(0, 0, -1)
			if ang := math.Acos(math.Max(-1, math.Min(1, fwd.dot(dl)))); ang > aimMax {
				// as far as it goes in that direction
				side := dl.sub(fwd.mul(fwd.dot(dl))).norm()
				dl = fwd.mul(math.Cos(aimMax)).add(side.mul(math.Sin(aimMax)))
			}
			R := turnTo(fwd, dl)
			aimFit[a.fitAim] = R
			aimFit[1-a.fitAim] = mirrorFit(R)
			o.fitGlove()
			a.fitGlove()
		case a.grabDown && a.fitDrag < 0 && o.palm.sub(a.palm).length() < 0.13:
			a.fitDrag = 1 - i
			a.fitRel = mul34(inv34(a.h.m), o.m)
			a.buzz(30, 0.6)
			o.buzz(30, 0.6)
		case a.fitDrag >= 0 && !a.closed:
			a.fitDrag = -1
			a.buzz(20, 0.4)
			saveGloves()
		case a.fitDrag >= 0:
			world := mul34(a.h.m, a.fitRel)
			fit := orthonormalize(mul34(inv34(o.h.m), world))
			// keep it believable: the glove can't wander off the controller
			if d := fit.pos(); d.length() < 0.25 {
				gloveFit[a.fitDrag] = fit
				gloveFit[1-a.fitDrag] = mirrorFit(fit) // the other hand is the mirror image
			}
		}
	}
}

// drawFit: in fit mode, a faint outline round each glove you can take hold of, and each
// laser shown with the ball you grab to re-aim it.
func (g *game) drawFit() {
	if !g.fitting {
		return
	}
	t := fxTime()
	for i, a := range hands {
		if !a.valid() {
			continue
		}
		o := hands[1-i]
		col := cBeam.alpha(0.5)
		if o != nil && o.fitDrag == i {
			col = cAmber.alpha(0.9)
		}
		ringSoft(a.palm, 0.11+0.005*math.Sin(t*6+float64(i)), 0.008, col, 32)
		if a.held != nil || a.beam {
			continue
		}
		bc := cBeam
		if o != nil && o.fitAim == i {
			bc = cAmber
		}
		p0 := a.aim.pos()
		ball := a.aimBall()
		beam(p0, p0.add(a.aim.axis(2).mul(-0.9)), bc.alpha(0.6), 0.003, 2.5)
		sphere(&bEnv, ball, 0.022, bc, 6, 10)
		ringSoft(ball, 0.04+0.004*math.Sin(t*7+float64(i)), 0.006, bc.alpha(0.8), 20)
	}
}

// aimBase: the laser's pose before its own re-aim (riding the fitted glove)
func (gh *gHand) aimBase() hmdMatrix34 { return mul34(gh.m, mul34(inv34(gh.h.m), gh.h.aim)) }

// aimBall: where the grab ball on this hand's laser sits
func (gh *gHand) aimBall() vec3 { return gh.aim.pos().add(gh.aim.axis(2).mul(-aimOrb)) }
