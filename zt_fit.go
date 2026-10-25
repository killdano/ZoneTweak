package main

// Fitting ZoneTweak's tool to your controller, the way Contraband's gloves are fitted: menu
// > FIT TOOL, then grab one tool with your other hand (the grip) and move and turn it until
// it sits where you want it in your hand - the other one mirrors it. The beam comes out of
// the tool, so it follows. B finishes, a thumbstick click puts both back. Saved for next
// time (spaces/toolfit.txt).

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// toolFit: each tool's offset from its controller's aim pose (where the tool is drawn from)
var toolFit = [2]hmdMatrix34{ident34(), ident34()}

func toolFitFile() string { return filepath.Join(gloveDir, "spaces", "toolfit.txt") }

func saveToolFit() {
	if gloveDir == "" {
		return
	}
	var b strings.Builder
	for _, m := range toolFit {
		for r := 0; r < 3; r++ {
			for c := 0; c < 4; c++ {
				fmt.Fprintf(&b, "%.6f ", m[r][c])
			}
		}
		b.WriteString("\n")
	}
	os.MkdirAll(filepath.Dir(toolFitFile()), 0755)
	os.WriteFile(toolFitFile(), []byte(b.String()), 0644)
}

func loadToolFit() {
	if gloveDir == "" {
		return
	}
	bs, err := os.ReadFile(toolFitFile())
	if err != nil {
		return
	}
	for i, l := range strings.Split(strings.TrimSpace(string(bs)), "\n") {
		if i > 1 {
			break
		}
		var v [12]float32
		if n, _ := fmt.Sscan(l, &v[0], &v[1], &v[2], &v[3], &v[4], &v[5], &v[6], &v[7], &v[8], &v[9], &v[10], &v[11]); n != 12 {
			continue
		}
		var m hmdMatrix34
		for k := 0; k < 12; k++ {
			m[k/4][k%4] = v[k]
		}
		if orthoOK(m) && m.pos().length() < 0.25 {
			toolFit[i] = m
		}
	}
}

// fitTool: a hand's poses with its tool fit applied (raw: as the controller reports them).
func fitTool(i int, rawM, rawAim hmdMatrix34) (hmdMatrix34, hmdMatrix34) {
	aim := mul34(rawAim, toolFit[i])
	return mul34(aim, mul34(inv34(rawAim), rawM)), aim
}

// toolCenter: the middle of a hand's tool (what the other hand grabs).
func (h *hand) toolCenter() vec3 { return h.toolFrame().o.add(h.toolFrame().z.mul(0.01)) }

func (e *editor) startToolFit() {
	e.menuOpen, e.toolFitting = false, true
	e.releaseAll()
	for _, h := range e.hands {
		h.fitOther = nil
	}
	e.say("Fit the tool: grab one with your other hand. B when done")
}

func (e *editor) endToolFit() {
	e.toolFitting = false
	for _, h := range e.hands {
		h.fitOther = nil
	}
	saveToolFit()
	e.say("Tool fit saved")
}

// toolFitFrame: in fit mode instead of editing. A grip next to the other tool takes hold of
// it; it rides with that hand until the grip lets go, and its offset from its own controller
// is what's kept (mirrored onto the other).
func (e *editor) toolFitFrame() {
	for _, a := range e.hands {
		if !a.valid {
			continue
		}
		if a.down(bitMenu) {
			e.endToolFit()
			return
		}
		if a.down(bitStick) {
			toolFit = [2]hmdMatrix34{ident34(), ident34()}
			saveToolFit()
			e.say("Tool fit reset")
			haptic(a.idx, 1500)
			continue
		}
		var o *hand
		for _, h := range e.hands {
			if h != a && h.valid {
				o = h
			}
		}
		if o == nil {
			continue
		}
		switch {
		case a.fitOther == nil && a.down(bitGrab) && a.aimValid && o.aimValid && a.toolCenter().sub(o.toolCenter()).length() < 0.15:
			a.fitOther = o
			a.fitRel = mul34(inv34(a.rawAim), o.aim)
			haptic(a.idx, 1200)
			haptic(o.idx, 1200)
		case a.fitOther != nil && !a.held(bitGrab):
			a.fitOther = nil
			haptic(a.idx, 800)
			saveToolFit()
		case a.fitOther == o:
			world := mul34(a.rawAim, a.fitRel)
			fit := orthonormalize(mul34(inv34(o.rawAim), world))
			if fit.pos().length() < 0.25 {
				toolFit[o.idx] = fit
				toolFit[a.idx] = mirrorFit(fit)
			}
		}
	}
}

// drawToolFit: the instructions, and a ring round each tool you can take hold of.
func (e *editor) drawToolFit() {
	if !e.toolFitting {
		return
	}
	t := fxTime()
	for _, h := range e.hands {
		if !h.valid {
			continue
		}
		col := cBeam.alpha(0.5)
		for _, o := range e.hands {
			if o.fitOther == h {
				col = cAmber.alpha(0.9)
			}
		}
		ringSoft(h.toolCenter(), 0.09+0.005*math.Sin(t*6+float64(h.idx)), 0.008, col, 32)
	}
	cam := e.hmd.pos()
	fwd := e.hmd.axis(2).mul(-1)
	fwd.y = 0
	if fwd.length() < 0.1 {
		fwd = v3(0, 0, -1)
	}
	p := cam.add(fwd.norm().mul(0.9)).add(v3(0, 0.12, 0))
	rr, uu := billboard(cam, p)
	lines := []string{"FIT THE TOOL", "GRAB ONE TOOL WITH YOUR OTHER HAND (GRIP)", "MOVE AND TURN IT, LET GO TO KEEP IT", "THE OTHER ONE MIRRORS IT", "B: DONE    THUMBSTICK: RESET"}
	for i, l := range lines {
		px, c := textPx*1.1, cWhite
		if i == 0 {
			px, c = textPx*1.8, cEnergy
		}
		drawTextCentered(&bOpaque, l, p.add(uu.mul(-0.035*float64(i))), rr, uu, px, c)
	}
}
