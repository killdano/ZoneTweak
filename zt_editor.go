package main

// ZoneTweak: the boundary editor (the app's main mode). Contraband, the game, is reached from
// here by the lever beside your play space (see zt_lever.go).

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"
)

func (e *editor) setPts(P []pt2, msg string) {
	if len(P) < 3 {
		e.say("That would leave fewer than 3 points")
		return
	}
	e.push()
	e.pts = P
	e.theta = domAngle(P)
	e.releaseAll()
	if msg != "" {
		e.say("%s", msg)
	}
}

func (e *editor) fmtLen(m float64) string {
	if e.feet {
		return fmt.Sprintf("%.1f FT", m*3.28084)
	}
	return fmt.Sprintf("%.2f M", m)
}

func (e *editor) fmtArea(a float64) string {
	if e.feet {
		return fmt.Sprintf("%.0f SQ FT", a*10.7639)
	}
	return fmt.Sprintf("%.1f M2", a)
}

func (e *editor) loadFromStore() {
	e.pts = clonePts(e.st.pts)
	e.orig = clonePts(e.st.pts)
	e.height = e.st.height
	e.theta = domAngle(e.pts)
	e.undo, e.redo = nil, nil
	e.dirty = false
	e.area = e.st.playArea
	e.center = e.st.adjust()
	e.centerDirty, e.areaDirty = false, false
	e.releaseAll()
}

// writeBoundary saves into every file holding the universe; steam and pimax say whose files those were
func writeBoundary(st *store, pts []pt2, h float64) (n int, steam, pimax bool, err error) {
	written, skipped, err := st.save(pts, h, 0)
	for _, w := range written {
		log.Println("wrote", w)
		if isPimaxFile(w) {
			pimax = true
		} else {
			steam = true
		}
	}
	for _, s := range skipped {
		log.Println("skipped", s)
	}
	return len(written), steam, pimax, err
}

// SteamVR work runs one call at a time off the render thread; results come back as status messages
var (
	steamJobs = make(chan func(), 16)
	asyncSay  = make(chan string, 16)
)

func steamWorker() {
	for j := range steamJobs {
		j()
	}
}

func queueSteam(j func()) {
	select {
	case steamJobs <- j:
	default:
		log.Println("SteamVR queue full, dropped a call")
	}
}

func (e *editor) applySteamVR(quiet bool) {
	queueSteam(func() {
		err := steamvrReload()
		log.Printf("SteamVR reload: %v", err)
		switch {
		case err == nil:
			e.liveFailed.Store(false)
			if !quiet {
				asyncSay <- "SteamVR updated live"
			}
		case !quiet:
			e.liveFailed.Store(true)
			asyncSay <- fmt.Sprintf("Saved, but SteamVR didn't reload it (%v). Restart SteamVR to apply", err)
		}
	})
}

func (e *editor) save() bool {
	movedCenter := e.centerDirty
	if movedCenter {
		m := e.centerPose().then(e.st.adjust().inv())
		ns := standingFromRigid(m.then(e.st.standingRigid()))
		e.st.newStand = &ns
	}
	if e.areaDirty || movedCenter {
		a := e.area
		e.st.newArea = &a
	}
	n, steamW, _, err := writeBoundary(e.st, e.pts, e.height)
	if err != nil {
		e.st.newStand, e.st.newArea = nil, nil
		e.say("Save failed: %v", err)
		return false
	}
	if movedCenter {
		e.st.setAdj(e.center)
	}
	e.centerDirty, e.areaDirty = false, false
	e.orig = clonePts(e.pts)
	e.dirty = false
	e.savedAny = true
	if steamMode() {
		e.say("Saved to %d file(s). Applying in SteamVR", n)
	} else {
		e.say("Saved to %d file(s). Restart Pimax to apply", n)
	}
	if steamW {
		e.applySteamVR(!steamMode())
	}
	return true
}

func (e *editor) spacePts(p playSpace) []pt2 {
	if p.Frame == "raw" {
		return e.st.toStageAll(p.pts())
	}
	return p.pts()
}

func (e *editor) useSpace(p playSpace) {
	e.setPts(e.spacePts(p), "")
	if p.Height > 0 {
		e.height = p.Height
	}
	if e.save() {
		e.lib.setActive(p.Name)
		if steamMode() {
			e.say("Using %s. Applying in SteamVR", p.Name)
		} else {
			e.say("Using %s. Restart Pimax to apply", p.Name)
		}
	}
}

func (e *editor) buildPages() {
	e.pageMain = &page{cols: 2, keyH: 0.056,
		header: func() []string {
			l1 := fmt.Sprintf("UNIVERSE %s  %d PTS  %s", e.st.uniID, len(e.pts), e.fmtArea(math.Abs(polyArea(e.pts))))
			if e.dirty {
				l1 += "  UNSAVED"
			}
			var hs []string
			for _, h := range e.hands {
				if h.valid {
					hs = append(hs, fmt.Sprintf("%.2f", h.m.pos().y))
				}
			}
			act := e.lib.active()
			if act == "" {
				act = "-"
			}
			return []string{l1, "SPACE " + act + "   CONTROLLER HEIGHT " + strings.Join(hs, " / ") + " M"}
		},
		buttons: func() []button {
			return []button{
				{label: func() string { return "PLAY CONTRABAND" }, sub: "TWEAK YOUR ZONE. THEN DEFEND IT.", span: 2, big: true, act: func() {
					e.menuOpen = false
					if onPlay != nil {
						onPlay()
					}
				}},
				{label: func() string { return "SAVE" }, act: func() { e.save() }},
				{label: func() string { return "REVERT" }, act: func() {
					e.setPts(clonePts(e.orig), "Back to the saved boundary")
					e.dirty = false
				}},
				{label: func() string { return "UNDO" }, act: e.doUndo},
				{label: func() string { return "REDO" }, act: e.doRedo},
				{label: func() string { return "SQUARE UP" }, act: func() {
					P := squareUp(simplify(e.pts, 0.10), domAngle(e.pts), 0.20)
					if P == nil {
						e.say("Couldn't find 4 clear walls")
						return
					}
					e.setPts(P, fmt.Sprintf("Squared up into %d walls", len(P)))
				}},
				{label: func() string { return "SIMPLIFY" }, act: func() {
					n := len(e.pts)
					P := simplify(e.pts, 0.08)
					if len(P) == n {
						e.say("Nothing to simplify")
						return
					}
					e.setPts(P, fmt.Sprintf("Simplified %d to %d points", n, len(P)))
				}},
				{label: func() string {
					if e.drawing {
						return "CANCEL OUTLINE"
					}
					return "NEW OUTLINE"
				}, act: func() {
					e.drawing = !e.drawing
					e.draft = nil
					e.releaseAll()
					if e.drawing {
						e.menuOpen = false
						e.say("Walk to each corner and pull the trigger")
					} else {
						e.say("Outline cancelled")
					}
				}},
				{label: func() string { return "SNAP: " + onoff(e.snap) }, act: func() { e.snap = !e.snap }, on: func() bool { return e.snap }},
				{label: func() string { return "SAVED SPACES" }, act: func() { e.page = e.pageSpaces }},
				{label: func() string { return "ZONE SETTINGS" }, act: func() { e.page = e.pageZone }},
				{label: func() string {
					if floorOff != 0 {
						return fmt.Sprintf("FIX FLOOR (%+.0f CM)", floorOff*100)
					}
					return "FIX FLOOR"
				}, act: func() {
					e.floorAt = time.Now().Add(3 * time.Second)
					e.menuOpen = false
				}},
				{label: func() string {
					if e.laser {
						return "POINTER: LASER"
					}
					return "POINTER: DOWN"
				}, act: func() { e.laser = !e.laser; toolSwitch = fxTime(); e.sfx(sndTool, e.hmd.pos(), 0.6) }, on: func() bool { return e.laser }},
				{label: func() string { return "FIT TOOL" }, act: e.startToolFit},
				{label: func() string { return "GRAPHICS: " + qualities[qualityIdx].name }, act: func() { e.page = e.pageGfx }},
				{label: func() string {
					if e.feet {
						return "UNITS: FEET"
					}
					return "UNITS: METERS"
				}, act: func() { e.feet = !e.feet }},
				{label: func() string { return "GHOST: " + onoff(e.ghost) }, act: func() { e.ghost = !e.ghost }, on: func() bool { return e.ghost }},
				{label: func() string {
					if e.matching {
						return "DONE REALIGNING"
					}
					return "REALIGN VIEW"
				}, act: func() {
					e.matching = !e.matching
					e.matchHand = nil
					e.drawing = false
					e.releaseAll()
					if e.matching {
						e.menuOpen = false
						e.say("Hold trigger and drag or twist to line up. Stick click resets")
					} else {
						e.say("Match done")
					}
				}, on: func() bool { return e.matching }},
				{label: func() string { return "RESTORE ORIGINAL" }, act: func() {
					P, from, err := e.st.oldestBackup()
					if err != nil {
						e.say("%v", err)
						return
					}
					log.Println("restoring from", from)
					e.setPts(P, fmt.Sprintf("Restored %d points from backup. Save to keep", len(P)))
				}},
				{label: func() string {
					if steamMode() {
						return "SAVE & EXIT"
					}
					return "SAVE & RESTART PIMAX"
				}, act: func() {
					if e.dirty || e.centerDirty || e.areaDirty {
						if !e.save() {
							return
						}
					}
					e.restartAfter = !steamMode()
					e.quit = true
				}},
				{label: func() string { return "EXIT" }, act: func() {
					if e.dirty && time.Now().After(e.exitArmed) {
						e.exitArmed = time.Now().Add(4 * time.Second)
						e.say("Unsaved changes. Press exit again to quit")
						return
					}
					e.quit = true
				}},
			}
		},
	}

	e.pageSpaces = &page{cols: 2, keyH: 0.056,
		header: func() []string {
			act := e.lib.active()
			if act == "" {
				act = "NONE YET"
			}
			return []string{"SAVED SPACES   ACTIVE: " + act, "PICK ONE TO WRITE IT TO " + strings.ToUpper(boundarySys) + ". SAVE AS NEW STORES WHAT YOU SEE NOW"}
		},
		buttons: func() []button {
			var bs []button
			act := e.lib.active()
			for _, p := range e.lib.list() {
				p := p
				bs = append(bs, button{label: func() string { return fmt.Sprintf("%s (%d)", p.Name, len(p.Points)) }, act: func() { e.useSpace(p) },
					on: func() bool { return strings.EqualFold(p.Name, act) }})
				if len(bs) >= 12 {
					break
				}
			}
			if len(bs)%2 == 1 {
				bs = append(bs, button{label: func() string { return "" }, act: func() {}})
			}
			bs = append(bs,
				button{label: func() string { return "SAVE AS NEW" }, act: func() {
					e.kbText = ""
					e.page = e.pageKb
				}},
				button{label: func() string {
					if act == "" {
						return "UPDATE ACTIVE"
					}
					return "UPDATE " + act
				}, act: func() {
					if act == "" {
						e.say("No active space yet. Use save as new")
						return
					}
					if err := e.lib.save(act, e.st.uniID, e.st.toFileAll(e.pts), e.height, "raw"); err != nil {
						e.say("%v", err)
						return
					}
					e.say("Updated %s", act)
				}},
				button{label: func() string {
					if time.Now().Before(e.confirmDel) {
						return "CONFIRM DELETE"
					}
					if act == "" {
						return "DELETE ACTIVE"
					}
					return "DELETE " + act
				}, act: func() {
					if act == "" {
						return
					}
					if time.Now().After(e.confirmDel) {
						e.confirmDel = time.Now().Add(4 * time.Second)
						return
					}
					e.confirmDel = time.Time{}
					if err := e.lib.remove(act); err != nil {
						e.say("%v", err)
						return
					}
					e.lib.setActive("")
					e.say("Deleted %s", act)
				}},
				button{label: func() string { return "BACK" }, act: func() { e.page = e.pageMain }},
			)
			return bs
		},
	}

	distLabel := func() string {
		if v, ok := e.zone.warnDistance(); ok {
			if e.feet {
				return fmt.Sprintf("%.1f FT", v*3.28084)
			}
			return fmt.Sprintf("%.2f M", v)
		}
		return strings.ToUpper(e.zone.sysName()) + " DEFAULT"
	}
	zoneName := func() string {
		if steamMode() {
			return "CHAPERONE"
		}
		return "SAFETY ZONE"
	}
	nudge := func(d float64) func() {
		return func() {
			v, ok := e.zone.warnDistance()
			if !ok {
				v = e.zone.defaultDist()
			}
			nv, err := e.zone.setWarnDistance(v+d, false)
			if err != nil {
				e.say("Couldn't write %s settings: %v", e.zone.sysName(), err)
				return
			}
			e.say("Warning distance %.2f M saved", nv)
		}
	}
	gfxBtn := func(i int, desc string) button {
		return button{label: func() string { return qualities[i].name + "  " + desc }, span: 2,
			on: func() bool { return qualityIdx == i },
			act: func() {
				if qualityIdx == i {
					return
				}
				qualityIdx = i
				saveQuality(appDir, i)
				wantQuality = i
				e.say("Graphics %s", qualities[i].name)
			}}
	}
	e.pageGfx = &page{cols: 2, keyH: 0.056,
		header: func() []string {
			return append([]string{"GRAPHICS QUALITY", "APPLIES INSTANTLY. IF IT STUTTERS, GO DOWN ONE"}, gt.lines()...)
		},
		buttons: func() []button {
			return []button{
				gfxBtn(0, "8X AA, 4K SHADOWS"),
				gfxBtn(1, "4X AA, 2K SHADOWS"),
				gfxBtn(2, "2X AA, LIGHTER FOG"),
				gfxBtn(3, "NO REFLECTIONS"),
				gfxBtn(4, "BARE MINIMUM"),
				{label: func() string { return fmt.Sprintf("RENDER SCALE %d%%", int(renderScale*100+0.5)) }, act: func() {
					k := 0
					for i, v := range renderScales {
						if math.Abs(v-renderScale) < 0.01 {
							k = i
						}
					}
					renderScale = renderScales[(k+1)%len(renderScales)]
					wantRebuild = true
					saveGfx(appDir)
					e.say("Render scale %d%%", int(renderScale*100+0.5))
				}},
				{label: func() string {
					if !vrsOK {
						return "FOVEATED: NOT SUPPORTED"
					}
					return "FOVEATED: " + onoff(foveated)
				}, on: func() bool { return vrsOK && foveated }, act: func() {
					if !vrsOK {
						e.say("Foveated rendering needs an NVIDIA RTX (or GTX 16) GPU")
						return
					}
					foveated = !foveated
					saveGfx(appDir)
					e.say("Foveated rendering %s", onoff(foveated))
				}},
				{label: func() string { return fmt.Sprintf("FOVEA SIZE %.0f DEG", foveaDeg) }, act: func() {
					k := 0
					for i, v := range foveaSizes {
						if v == foveaDeg {
							k = i
						}
					}
					foveaDeg = foveaSizes[(k+1)%len(foveaSizes)]
					saveGfx(appDir)
					e.say("Full quality within %.0f degrees of the centre", foveaDeg)
				}},
				{label: func() string { return "EDGES " + foveaLabels[foveaEdge] }, act: func() {
					foveaEdge = (foveaEdge + 1) % 3
					saveGfx(appDir)
					e.say("Edges shaded at %s rate", strings.ToLower(foveaLabels[foveaEdge]))
				}},
				{label: func() string { return "OPTIMIZED RENDERING: " + onoff(perfOpt) }, on: func() bool { return perfOpt }, span: 2, act: func() {
					perfOpt = !perfOpt
					wantRebuild = true
					saveGfx(appDir)
					e.say("Optimized rendering %s", onoff(perfOpt))
				}},
				{label: func() string { return "BACK" }, act: func() { e.page = e.pageMain }, span: 2},
			}
		},
	}

	e.pageZone = &page{cols: 2, keyH: 0.056,
		header: func() []string {
			return []string{zoneName() + " SETTINGS", "WARNING DISTANCE " + distLabel() + "   " + zoneName() + " " + onoff(e.zone.zoneOn())}
		},
		buttons: func() []button {
			return []button{
				{label: func() string { return "CLOSER -10 CM" }, act: nudge(-0.10)},
				{label: func() string { return "FARTHER +10 CM" }, act: nudge(0.10)},
				{label: func() string { return "CLOSER -2 CM" }, act: nudge(-0.02)},
				{label: func() string { return "FARTHER +2 CM" }, act: nudge(0.02)},
				{label: func() string { return strings.ToUpper(e.zone.sysName()) + " DEFAULT" }, act: func() {
					if _, err := e.zone.setWarnDistance(0, true); err != nil {
						e.say("Couldn't write %s settings: %v", e.zone.sysName(), err)
						return
					}
					e.say("Warning distance back to %s default", e.zone.sysName())
				}},
				{label: func() string { return zoneName() + ": " + onoff(e.zone.zoneOn()) }, act: func() {
					on := !e.zone.zoneOn()
					if err := e.zone.setZoneOn(on); err != nil {
						e.say("Couldn't write %s settings: %v", e.zone.sysName(), err)
						return
					}
					e.say("%s %s", zoneName(), onoff(on))
				}, on: func() bool { return e.zone.zoneOn() }},
				{label: func() string { return "PROXIMITY TEST: " + onoff(e.proxTest) }, act: func() {
					e.proxTest = !e.proxTest
					if e.proxTest {
						e.say("Walk toward a wall. Drag the orange line to change the distance")
					}
				}, on: func() bool { return e.proxTest }, span: 2},
				{label: func() string { return "BACK" }, act: func() { e.page = e.pageMain }, span: 2},
			}
		},
	}

	e.pageArea = &page{cols: 2, keyH: 0.056,
		header: func() []string {
			return []string{fmt.Sprintf("CENTER & PLAY AREA  %s X %s", e.fmtLen(e.area[0]), e.fmtLen(e.area[1])), "ON THE FLOOR: CENTER MOVES, CURVED ARROWS TURN, BLUE HANDLES STRETCH"}
		},
		buttons: func() []button {
			return []button{
				{label: func() string { return "FIT AREA" }, act: func() {
					g := e.centerPose()
					w, h, ok := fitArea(g.inv().all(e.pts), 0.1)
					if !ok {
						e.say("Put the center inside the boundary first")
						return
					}
					e.area = [2]float64{w, h}
					e.areaDirty = true
					e.say("Play area %s x %s", e.fmtLen(w), e.fmtLen(h))
				}},
				{label: func() string { return "CENTER IN ROOM" }, act: func() {
					var cx, cz, A float64
					for i := range e.pts {
						p, q := e.pts[i], e.pts[(i+1)%len(e.pts)]
						cr := p[0]*q[1] - q[0]*p[1]
						A += cr
						cx += (p[0] + q[0]) * cr
						cz += (p[1] + q[1]) * cr
					}
					if math.Abs(A) < 1e-9 {
						return
					}
					cx, cz = cx/(3*A), cz/(3*A)
					g := e.centerPose()
					e.center = rigid2{a: g.a, b: g.b, c: g.c, d: g.d, tx: cx, tz: cz}
					e.centerDirty = true
					e.say("Center moved to the middle of the room")
				}},
				{label: func() string { return "SQUARE TO ROOM" }, act: func() {
					g := e.centerPose()
					cur := math.Atan2(g.c, g.a)
					th := domAngle(e.pts)
					best := th
					for k := -4; k <= 4; k++ {
						cand := th + float64(k)*math.Pi/2
						if math.Abs(math.Remainder(cand-cur, 2*math.Pi)) < math.Abs(math.Remainder(best-cur, 2*math.Pi)) {
							best = cand
						}
					}
					e.center = rigidFromYaw(best, g.tx, g.tz)
					e.centerDirty = true
					e.say("Play area turned to match the walls")
				}},
				{label: func() string { return "RESET" }, act: func() {
					e.center = e.st.adjust()
					e.area = e.st.playArea
					e.centerDirty, e.areaDirty = false, false
					e.say("Center and area back to saved")
				}},
				{label: func() string { return "LOCK PLAY AREA" }, act: func() { e.setAreaMode(false) }, span: 2},
			}
		},
	}

	key := func(s string) button {
		return button{label: func() string { return s }, act: func() {
			if len(e.kbText) < 24 {
				e.kbText += s
			}
		}}
	}
	e.pageKb = &page{cols: 10, keyH: 0.05,
		header: func() []string { return []string{"NAME THIS SPACE", "> " + e.kbText + "_"} },
		buttons: func() []button {
			var bs []button
			for _, row := range []string{"1234567890", "QWERTYUIOP", "ASDFGHJKL-", "ZXCVBNM"} {
				for _, r := range row {
					bs = append(bs, key(string(r)))
				}
			}
			bs = append(bs,
				button{label: func() string { return "DEL" }, span: 3, act: func() {
					if len(e.kbText) > 0 {
						e.kbText = e.kbText[:len(e.kbText)-1]
					}
				}},
				button{label: func() string { return "SPACE" }, span: 4, act: func() {
					if len(e.kbText) < 24 && e.kbText != "" {
						e.kbText += " "
					}
				}},
				button{label: func() string { return "CANCEL" }, span: 3, act: func() { e.page = e.pageSpaces }},
				button{label: func() string { return "SAVE" }, span: 3, act: func() {
					name := strings.TrimSpace(e.kbText)
					if err := e.lib.save(name, e.st.uniID, e.st.toFileAll(e.pts), e.height, "raw"); err != nil {
						e.say("%v", err)
						return
					}
					e.lib.setActive(name)
					e.say("Saved space %s", name)
					e.page = e.pageSpaces
				}},
			)
			return bs
		},
	}
	e.page = e.pageMain
}

type slot struct{ cx, cy, w, h float64 }

// headerLines: the page's header, word-wrapped to the panel width (title in the big font)
func headerLines(p *page) []string {
	var out []string
	for i, l := range p.header() {
		px := smallPx
		if i == 0 {
			px = textPx
		}
		n := int((panelW - 2*panelPad - 0.03) / (6 * px))
		if i == 0 {
			out = append(out, l) // the title shrinks to fit instead (see titlePx)
			continue
		}
		out = append(out, wrapWords(l, n)...)
	}
	return out
}

// titlePx: the header title's pixel size, smaller when it wouldn't fit the panel
func titlePx(s string) float64 {
	return min(textPx, (panelW-2*panelPad-0.05)/(6*float64(len(s))))
}

// pageHeaderH: room for the page's header lines (three fit the standard height)
func pageHeaderH(p *page) float64 {
	return max(headerH0, 0.03+float64(len(headerLines(p)))*0.026+0.003)
}

func (e *editor) layout() ([]slot, float64) {
	p := e.page
	headerH := pageHeaderH(p)
	cols := p.cols
	cellW := (panelW - 2*panelPad - float64(cols-1)*btnGap) / float64(cols)
	var slots []slot
	col := 0
	y, rowH := -headerH, 0.0 // the top of the row being filled, and its height so far
	for _, b := range e.cur {
		span := max(b.span, 1)
		if col+span > cols {
			y, col, rowH = y-rowH-btnGap, 0, 0
		}
		h := p.keyH
		if b.big {
			h = p.keyH * 1.8
		}
		w := cellW*float64(span) + btnGap*float64(span-1)
		x := -panelW/2 + panelPad + float64(col)*(cellW+btnGap) + w/2
		slots = append(slots, slot{x, y - h/2, w, h})
		rowH = math.Max(rowH, h)
		col += span
		if col >= cols {
			y, col, rowH = y-rowH-btnGap, 0, 0
		}
	}
	if col > 0 {
		y -= rowH + btnGap
	}
	H := -y + footerH
	for i := range slots {
		slots[i].cy += H / 2
	}
	return slots, H
}

func (e *editor) openMenu() {
	hp := e.hmd.pos()
	fwd := e.hmd.axis(2).mul(-1)
	fwd.y = 0
	fwd = fwd.norm()
	if fwd.length() < 0.5 {
		fwd = v3(0, 0, -1)
	}
	e.menuPos = hp.add(fwd.mul(0.48)).add(v3(0, -0.22, 0))
	e.menuN = fwd.mul(-1)
	e.menuR = worldUp.cross(e.menuN).norm()
	e.menuU = worldUp
	e.menuOpen = true
	e.menuOpenT = fxTime()
	e.sfx(sndTool, e.menuPos, 0.35)
	e.page = e.pageMain
	if e.areaMode {
		e.page = e.pageArea
	}
}

func (e *editor) hitButton(p vec3) int {
	slots, _ := e.layout()
	d := p.sub(e.menuPos)
	lx, ly, lz := d.dot(e.menuR), d.dot(e.menuU), d.dot(e.menuN)
	if lz < -0.06 || lz > 0.10 {
		return -1
	}
	for i, s := range slots {
		if math.Abs(lx-s.cx) <= s.w/2+btnGap/2 && math.Abs(ly-s.cy) <= s.h/2+btnGap/2 {
			return i
		}
	}
	return -1
}

func (e *editor) nearest(c pt2) (int, int) {
	bv, bd := -1, e.grabR()
	for i, p := range e.pts {
		if d := dist2(p, c); d < bd {
			bd, bv = d, i
		}
	}
	if bv >= 0 {
		return bv, -1
	}
	be, bde := -1, edgeRadius
	for i := range e.pts {
		d, _ := segDist(c, e.pts[i], e.pts[(i+1)%len(e.pts)])
		if d < bde {
			bde, be = d, i
		}
	}
	return -1, be
}

func (e *editor) draftSnap(c pt2) pt2 {
	if !e.snap || len(e.draft) < 2 {
		return c
	}
	last, first := e.draft[len(e.draft)-1], e.draft[0]
	th := math.Atan2(e.draft[1][1]-e.draft[0][1], e.draft[1][0]-e.draft[0][0])
	if len(e.draft) >= 3 {
		return snapToNeighbors(c, &last, &first, th)
	}
	return snapToNeighbors(c, &last, nil, th)
}

func (e *editor) handleHand(h *hand) {
	c, cOK := h.pointer(e.laser)
	// one active hand in both tool modes (laserHand): the last hand to pull its trigger
	if e.laserHand == nil {
		e.laserHand = h
	}
	h.btn = -1
	h.menuRay = false
	if e.menuOpen {
		if e.laser && h == e.laserHand {
			if p, ok := e.rayPanel(h); ok {
				h.menuRay, h.menuPt = true, p
				h.btn = e.hitButton(p)
			}
		}
		if h.btn < 0 {
			h.btn = e.hitButton(h.tip())
		}
	}
	if h.down(bitMenu) {
		if e.menuOpen {
			e.menuOpen = false
		} else {
			e.openMenu()
		}
		return
	}
	if h.down(bitTool) {
		e.switchTool(h)
		return
	}
	if h.btn >= 0 && h.down(bitTrigger) {
		haptic(h.idx, 1500)
		audioClick()
		e.cur[h.btn].act()
		return
	}
	if e.menuOpen && (h.btn >= 0 || h.menuRay) {
		return
	}
	if h != e.laserHand {
		busy := e.laserHand != nil && (e.laserHand.grab >= 0 || e.laserHand.areaGrab != 0)
		if h.down(bitTrigger) && !busy {
			e.laserHand = h
			haptic(h.idx, 600)
			return
		}
		cOK = false
	}
	if e.matching {
		e.handleMatch(h)
		return
	}
	h.rayHitOK = false
	if e.laser && h == e.laserHand && !e.areaMode && !e.drawing && (h.rayGrab || h.grab < 0) {
		if e.handleRayGen(h) {
			return
		}
	}
	if !cOK {
		h.hoverV, h.hoverE = -1, -1
		return
	}
	if h.down(bitTrigger) && h.areaGrab == 0 && dist2(c, e.lockPos()) < 0.15 {
		e.setAreaMode(!e.areaMode)
		haptic(h.idx, 2000)
		return
	}
	if e.areaMode {
		h.hoverV, h.hoverE = -1, -1
		e.handleArea(h, c)
		return
	}
	if e.drawing {
		h.hoverV, h.hoverE = -1, -1
		if h.down(bitTrigger) {
			p := e.draftSnap(c)
			if len(e.draft) >= 3 && dist2(p, e.draft[0]) < closeRadius {
				e.drawing = false
				e.setPts(clonePts(e.draft), fmt.Sprintf("Outline closed with %d corners", len(e.draft)))
				e.draft = nil
				haptic(h.idx, 3000)
				return
			}
			e.draft = append(e.draft, p)
			e.startDeploy(h, p, false)
			e.draftHand = h
			haptic(h.idx, 1200)
			e.say("Corner %d placed", len(e.draft))
		}
		if h.down(bitGrip) && len(e.draft) > 0 {
			e.draft = e.draft[:len(e.draft)-1]
			e.say("Corner removed")
		}
		if h.down(bitStick) {
			e.drawing = false
			e.draft = nil
			e.say("Outline cancelled")
		}
		return
	}
	if h.grab >= 0 {
		if !h.held(bitTrigger) || h.grab >= len(e.pts) {
			e.grabEnd(h)
			h.grab = -1
			e.theta = domAngle(e.pts)
			haptic(h.idx, 800)
		} else {
			e.pts[h.grab] = e.wallPush(h, e.snapped(h.grab, pt2{c[0] + h.grabOff[0], c[1] + h.grabOff[1]}))
			e.grabUpdate(h)
		}
		return
	}
	h.hoverV, h.hoverE = e.nearest(c)
	if e.handleWarnLine(h, c) {
		return
	}
	if h.down(bitTrigger) {
		taken := false
		for _, o := range e.hands {
			if o != h && o.grab == h.hoverV && h.hoverV >= 0 {
				taken = true
			}
		}
		switch {
		case h.hoverV >= 0 && !taken:
			e.push()
			h.grab = h.hoverV
			e.grabStart(h, h.grab, false)
			gp := e.pts[h.grab]
			h.grabOff = pt2{gp[0] - c[0], gp[1] - c[1]}
			haptic(h.idx, 1200)
		case h.hoverE >= 0:
			e.push()
			e.releaseAll()
			a, b := e.pts[h.hoverE], e.pts[(h.hoverE+1)%len(e.pts)]
			_, t := segDist(c, a, b)
			np := pt2{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t}
			e.pts = append(e.pts[:h.hoverE+1], append([]pt2{np}, e.pts[h.hoverE+1:]...)...)
			h.grab = h.hoverE + 1
			h.grabOff = pt2{np[0] - c[0], np[1] - c[1]}
			e.grabStart(h, h.grab, true)
			e.startDeploy(h, np, true)
			haptic(h.idx, 1800)
		}
		return
	}
	if h.down(bitGrip) && h.hoverV >= 0 {
		if len(e.pts) <= 3 {
			e.say("A boundary needs at least 3 points")
			return
		}
		e.push()
		e.releaseAll()
		e.startRemoval(h.hoverV)
		e.pts = append(e.pts[:h.hoverV], e.pts[h.hoverV+1:]...)
		e.theta = domAngle(e.pts)
		haptic(h.idx, 2500)
		e.say("Point deleted")
		return
	}
	if h.down(bitStick) {
		e.switchTool(h) // stick click: pointer or floor mode (undo's on the menu)
	}
}

func (e *editor) updateFloor() {
	if !e.floorWarned {
		for _, h := range e.hands {
			if h.valid && h.m.pos().y < -0.15 {
				e.floorWarned = true
				e.say("Floor looks wrong. Menu > Fix floor")
			}
		}
	}
	if e.floorAt.IsZero() || time.Now().Before(e.floorAt) {
		return
	}
	e.floorAt = time.Time{}
	minY := math.Inf(1)
	for _, h := range e.hands {
		if h.valid {
			minY = math.Min(minY, h.m.pos().y)
		}
	}
	if math.IsInf(minY, 1) {
		e.say("No controller tracked, floor unchanged")
		return
	}
	floorOff += minY - 0.04
	e.say("Floor moved %+.0f CM (view only)", (minY-0.04)*100)
}

func handPose2(h *hand) rigid2 {
	f := h.m.axis(2).mul(-1)
	p := h.m.pos()
	return rigidFromYaw(math.Atan2(f.z, f.x), p.x, p.z)
}

func (e *editor) applyStageMove(r rigid2) {
	e.center = e.centerPose().then(r)
	e.st.moveStage(r)
	e.pts = r.all(e.pts)
	e.orig = r.all(e.orig)
	e.draft = r.all(e.draft)
	for i := range e.undo {
		e.undo[i] = r.all(e.undo[i])
	}
	for i := range e.redo {
		e.redo[i] = r.all(e.redo[i])
	}
	e.releaseAll()
	e.matchHand = nil
	e.menuOpen = false
}

func (e *editor) handleMatch(h *hand) {
	if h.down(bitStick) {
		e.applyStageMove(e.st.adjust().inv())
		e.say("Match reset")
		return
	}
	if e.matchHand == nil && h.down(bitTrigger) {
		e.matchHand = h
		h.matchG0 = handPose2(h)
		e.matchAdj0 = e.st.adjust()
		e.matchPts0, e.matchOrig0 = clonePts(e.pts), clonePts(e.orig)
		haptic(h.idx, 1200)
		return
	}
	if e.matchHand != h {
		return
	}
	if !h.held(bitTrigger) {
		e.matchHand = nil
		haptic(h.idx, 800)
		return
	}
	delta := h.matchG0.inv().then(handPose2(h))
	e.st.setAdj(e.matchAdj0.then(delta))
	e.pts = delta.all(e.matchPts0)
	e.orig = delta.all(e.matchOrig0)
}

func (e *editor) changed() bool {
	if len(e.orig) != len(e.pts) {
		return true
	}
	for i := range e.pts {
		if e.pts[i] != e.orig[i] {
			return true
		}
	}
	return false
}

func (e *editor) buildScene() {
	bOpaque.reset()
	bLines.reset()
	bTrans.reset()
	for _, b := range []*batch{&bEnv, &bEnvL, &bFloor, &bFloorL, &bGlowTri, &bGlowLine, &bGlowWide} {
		b.reset()
	}
	bSoft.reset()
	bShadow.reset()
	bSmoke.reset()
	bFire.reset()
	frameLights = frameLights[:0]
	fogVols = fogVols[:0]
	seedTex(0)
	cam := e.hmd.pos()
	camPos, camR, camU = cam, e.hmd.axis(0), e.hmd.axis(1)
	e.updateTool(frameDT)
	e.updateDeploys()
	e.buildEnvironment()
	e.updateFaults(0)

	level := e.proximityLevel()
	e.drawArea()
	e.drawProximity(level)

	if e.ghost && e.changed() && len(e.orig) > 2 {
		for i := range e.orig {
			a, b := e.orig[i], e.orig[(i+1)%len(e.orig)]
			bLines.line(fl(a, 0.004), fl(b, 0.004), cGhost)
		}
	}

	hotV, hotE := map[int]bool{}, map[int]bool{}
	for _, h := range e.hands {
		if !h.valid || e.drawing {
			continue
		}
		if h.grab >= 0 {
			hotV[h.grab] = true
		} else if h.hoverV >= 0 {
			hotV[h.hoverV] = true
		} else if h.hoverE >= 0 {
			hotE[h.hoverE] = true
		}
	}

	n := len(e.pts)
	beginGenFrame()
	defer endGenFrame()
	if !e.drawing {
		e.updateBladeExt(frameDT)
		e.wallInterlocks()
		e.computeJoints()
		e.drawPlates()
		for i := 0; i < n; i++ {
			a, b := e.pts[i], e.pts[(i+1)%n]
			e.buildLaserWall(i, a, b, e.height, level, hotE[i])
			if L := dist2(a, b); L > 0.25 {
				mid := fl(pt2{(a[0] + b[0]) / 2, (a[1] + b[1]) / 2}, 1.15)
				rr, uu := billboard(cam, mid)
				drawTextCentered(&bOpaque, e.fmtLen(L), mid, rr, uu, textPx*1.4, cWhite)
			}
		}
		for i, p := range e.pts {
			e.drawBladeExt(i, p, e.pts[(i-1+n)%n], e.pts[(i+1)%n], e.wallTint(level, false))
			if d := deployAt(p); d != nil && e.drawDeploy(d, p, e.height, e.pts[(i-1+n)%n], e.pts[(i+1)%n]) {
				continue
			}
			e.drawGen(i, p, e.height, e.pts[(i-1+n)%n], e.pts[(i+1)%n], hotV[i], e.wallTint(level, false), genFault[i])
		}
		updateMist()
		e.drawWallPush()
		e.updateRemovals()
		e.drawRemovals(e.wallTint(level, false))
		e.drawOldBases()
		e.updateFire(frameDT)
		e.drawFire()
	} else {
		for i, p := range e.draft {
			prev, next := p, p
			if i > 0 {
				prev = e.draft[i-1]
				e.buildLaserWall(i, e.draft[i-1], p, e.height, 0, false)
			}
			if i+1 < len(e.draft) {
				next = e.draft[i+1]
			}
			if d := deployAt(p); d != nil && e.drawDeploy(d, p, e.height, prev, next) {
				continue
			}
			e.buildGenerator(p, e.height, prev, next, false, cBeam, 0, p, nil)
		}
		if e.draftHand != nil && e.draftHand.valid && len(e.draft) > 0 {
			dc, _ := e.draftHand.pointer(e.laser)
			c := e.draftSnap(dc)
			glowLine(fl(e.draft[len(e.draft)-1], 0.004), fl(c, 0.004), cBeam.alpha(0.5), 0.3)
			if len(e.draft) >= 3 && dist2(c, e.draft[0]) < closeRadius {
				ringGlow(fl(e.draft[0], 0.003), closeRadius, cAmber)
			}
		}
	}

	buzz, buzzPan := 0.0, 0.0
	for _, h := range e.hands {
		if !h.valid {
			continue
		}
		e.drawTool(h)
		if h != e.laserHand {
			continue // only the active hand shows its beam, floor or pointer
		}
		if wb, wh := e.drawToolBeams(h); wb {
			flare(wh.p, wh.n, 0.1, cBeam)
			buzz = 0.046
			buzzPan = wh.p.sub(cam).norm().dot(e.hmd.axis(0))
		}
		if e.menuOpen {
			tc := cMuted
			if h.btn >= 0 {
				tc = cBtnHot
			}
			sprite(h.tip(), 0.01, tc)
		}
	}

	e.drawBaseStations()
	e.drawToolFit()
	touching := e.updateWallContacts(frameDT)
	e.updateRobots(frameDT)
	e.dronesFrame(frameDT)
	e.drawDrones()
	updateParticles(frameDT)
	hv, hp := e.humFor()
	cr := 0.0
	if touching {
		cr = 0.144
	}
	audioSet(hv, hp, buzz, buzzPan, cr)

	if !e.floorAt.IsZero() {
		fwd := e.hmd.axis(2).mul(-1)
		fwd.y = 0
		fwd = fwd.norm()
		p := cam.add(fwd.mul(0.8))
		rr, uu := billboard(cam, p)
		drawTextCentered(&bOpaque, fmt.Sprintf("REST A CONTROLLER ON THE FLOOR  %.0f", math.Ceil(time.Until(e.floorAt).Seconds())), p, rr, uu, textPx*1.5, cAmber)
	} else if !e.menuOpen && time.Since(e.statusAt) < 4*time.Second && e.status != "" {
		fwd := e.hmd.axis(2).mul(-1)
		fwd.y = 0
		fwd = fwd.norm()
		p := cam.add(fwd.mul(0.9)).add(v3(0, -0.3, 0))
		rr, uu := billboard(cam, p)
		drawTextCentered(&bOpaque, e.status, p, rr, uu, textPx*1.3, cWhite)
	}
	t := fxTime()
	if e.menuWasOpen && !e.menuOpen {
		e.menuCloseT = t // play the opening animation in reverse
		e.sfx(sndTool, e.menuPos, 0.25)
	}
	e.menuWasOpen = e.menuOpen
	if e.menuOpen || e.menuCloseT > 0 && t-e.menuCloseT < 0.28 {
		e.buildPanel()
	}
}

func (e *editor) panelPoint(x, y, z float64) vec3 {
	return e.menuPos.add(e.menuR.mul(x * menuSX)).add(e.menuU.mul(y * menuSY)).add(e.menuN.mul(z))
}

var menuSX, menuSY = 1.0, 1.0

func (e *editor) panelQuad(b *batch, x0, y0, x1, y1, z float64, c0, c1 rgba) {
	b.quadC(e.panelPoint(x0, y0, z), e.panelPoint(x1, y0, z), e.panelPoint(x1, y1, z), e.panelPoint(x0, y1, z), c0, c0, c1, c1)
}

func (e *editor) panelBox(cx, cy, cz, hx, hy, hz, ch float64, col rgba) {
	bevelBox(&bEnv, e.panelPoint(cx, cy, cz), e.menuR.mul(menuSX), e.menuU.mul(menuSY), e.menuN, hx, hy, hz, ch, col)
}

func (e *editor) panelGlowRect(x0, y0, x1, y1, z float64, col rgba, w float32) {
	a, b, c, d := e.panelPoint(x0, y0, z), e.panelPoint(x1, y0, z), e.panelPoint(x1, y1, z), e.panelPoint(x0, y1, z)
	glowLine(a, b, col, w)
	glowLine(b, c, col, w)
	glowLine(c, d, col, w)
	glowLine(d, a, col, w)
}

func (e *editor) buildPanel() {
	seedTex(20000)
	t := fxTime()
	open := smooth((t - e.menuOpenT) / 0.28)
	if !e.menuOpen {
		open = 1 - smooth((t-e.menuCloseT)/0.28)
	}
	menuSX, menuSY = 0.85+0.15*open, math.Max(0.02, open)
	defer func() { menuSX, menuSY = 1, 1 }()
	slots, H := e.layout()
	w2, h2 := panelW/2, H/2
	R, U := e.menuR.mul(menuSX), e.menuU.mul(menuSY)
	defer cleanRoles()()

	e.panelBox(0, 0, -0.018, w2+0.02, h2+0.02, 0.012, 0.008, cFrame)
	for _, sy := range []float64{-1, 1} {
		e.panelBox(0, sy*(h2+0.012), 0.0, w2+0.02, 0.008, 0.008, 0.004, cMetalDk)
	}
	for _, sx := range []float64{-1, 1} {
		e.panelBox(sx*(w2+0.012), 0, 0.0, 0.008, h2+0.004, 0.008, 0.004, cMetalDk)
		// armour rails down both sides, the full height of the frame
		e.panelBox(sx*(w2+0.03), 0, -0.01, 0.012, h2+0.016, 0.014, 0.006, cArmor)
		a, b := e.panelPoint(sx*(w2+0.043), -h2+0.02, -0.004), e.panelPoint(sx*(w2+0.043), h2-0.02, -0.004)
		ribbon(a, b, 0.01, cBeam.alpha(0.5))
		bGlowLine.line(a, b, cBeam.alpha(0.9))
		for _, sy := range []float64{-1, 1} {
			e.panelBox(sx*(w2+0.012), sy*(h2+0.012), 0.004, 0.022, 0.022, 0.01, 0.006, cMetal)
			sprite(e.panelPoint(sx*(w2+0.012), sy*(h2+0.012), 0.016), 0.006, cAmber.alpha(float32(0.5+0.5*math.Sin(t*3+sx+sy))))
		}
	}
	glassT := rgba{0.03, 0.07, 0.09, 0.9}
	glassB := rgba{0.015, 0.03, 0.04, 0.92}
	e.panelQuad(&bTrans, -w2, -h2, w2, h2, -0.004, glassB, glassT)
	for y := -h2 + 0.006; y < h2; y += 0.012 {
		a, b := e.panelPoint(-w2, y, -0.003), e.panelPoint(w2, y, -0.003)
		bGlowLine.line(a, b, cBeam.alpha(0.025))
	}
	sweep := -h2 + math.Mod(t*0.12, 1)*2*h2
	ribbon(e.panelPoint(-w2, sweep, -0.002), e.panelPoint(w2, sweep, -0.002), 0.02, cBeam.alpha(0.05))
	if open < 1 {
		ribbon(e.panelPoint(-w2, 0, 0.004), e.panelPoint(w2, 0, 0.004), 0.03, cBeam.alpha(float32(0.8*(1-open))))
	}
	e.panelGlowRect(-w2, -h2, w2, h2, 0.002, cBeam.alpha(0.45), 0.3)
	per := 4 * (w2 + h2)
	for k := 0; k < 2; k++ {
		s := math.Mod(t*0.25+float64(k)*0.5, 1) * per
		var x, y float64
		switch {
		case s < 2*w2:
			x, y = -w2+s, h2
		case s < 2*w2+2*h2:
			x, y = w2, h2-(s-2*w2)
		case s < 4*w2+2*h2:
			x, y = w2-(s-2*w2-2*h2), -h2
		default:
			x, y = -w2, -h2+(s-4*w2-2*h2)
		}
		sprite(e.panelPoint(x, y, 0.003), 0.025, cBeam.alpha(0.7))
	}

	hdrY := h2 - pageHeaderH(e.page) + 0.008
	ribbon(e.panelPoint(-w2+0.01, hdrY, 0.002), e.panelPoint(w2-0.01, hdrY, 0.002), 0.004, cEnergy.alpha(0.6))
	bGlowLine.line(e.panelPoint(-w2+0.01, hdrY, 0.002), e.panelPoint(w2-0.01, hdrY, 0.002), cEnergy.alpha(0.8))
	for i, l := range headerLines(e.page) {
		px, col := smallPx, cMuted
		if i == 0 {
			px, col = titlePx(l), cEnergy
			sprite(e.panelPoint(-w2+panelPad+0.08, h2-0.025, 0.001), 0.06, cEnergy.alpha(0.08))
		}
		drawText(&bOpaque, l, e.panelPoint(-w2+panelPad, h2-0.03-float64(i)*0.026, 0.003), R, U, px, col)
	}
	bars := 5
	for k := 0; k < bars; k++ {
		hgt := 0.004 + 0.012*noise1(float64(k), t*2)
		x := w2 - panelPad - float64(bars-k)*0.008
		e.panelQuad(&bGlowTri, x, h2-0.035, x+0.005, h2-0.035+hgt, 0.002, cEnergy.alpha(0.7), cEnergy.alpha(0.7))
	}

	hot := map[int]bool{}
	for _, h := range e.hands {
		if h.valid && h.btn >= 0 {
			hot[h.btn] = true
			if h.btn != h.prevBtn {
				e.sfx(2, e.panelPoint(0, 0, 0), 0.12)
				haptic(h.idx, 300)
			}
		}
		h.prevBtn = h.btn
	}
	for i, b := range e.cur {
		s := slots[i]
		label := b.label()
		if label == "" {
			continue
		}
		on := b.on != nil && b.on()
		x0, x1, y0, y1 := s.cx-s.w/2, s.cx+s.w/2, s.cy-s.h/2, s.cy+s.h/2
		z := 0.004
		col, tcol := rgba{0.12, 0.15, 0.17, 1}, cWhite
		if on {
			col = rgba{0.08, 0.2, 0.14, 1}
		}
		if b.big {
			col = rgba{0.26, 0.07, 0.05, 1}
		}
		if hot[i] {
			z = 0.008
			col = rgba{0.16, 0.24, 0.22, 1}
			tcol = rgba{0.95, 1, 0.97, 1}
		}
		e.panelBox(s.cx, s.cy, z-0.002, s.w/2, s.h/2, 0.004, 0.003, col)
		face := z + 0.0021
		if hot[i] {
			pulse := float32(0.75 + 0.25*math.Sin(t*8))
			e.panelQuad(&bGlowTri, x0+0.003, y0+0.003, x1-0.003, y1-0.003, face, cEnergy.alpha(0.18*pulse), cEnergy.alpha(0.32*pulse))
			e.panelGlowRect(x0, y0, x1, y1, face+0.0005, cEnergy.alpha(0.9*pulse), 0.4)
			sprite(e.panelPoint(s.cx, s.cy, face-0.006), s.w*0.7, cEnergy.alpha(0.12))
		} else if b.big {
			pulse := float32(0.6 + 0.4*math.Sin(t*3))
			e.panelQuad(&bGlowTri, x0+0.003, y0+0.003, x1-0.003, y1-0.003, face, cDanger.alpha(0.1*pulse), cDanger.alpha(0.22*pulse))
			e.panelGlowRect(x0, y0, x1, y1, face+0.0005, cDanger.alpha(0.8*pulse), 0.4)
		} else {
			e.panelGlowRect(x0+0.002, y0+0.002, x1-0.002, y1-0.002, face, cBeam.alpha(0.18), 0)
		}
		if b.big {
			px := textPx * 1.9
			if tw := float64(textWidth(label)) * px; tw > s.w-0.03 {
				px = (s.w - 0.03) / float64(textWidth(label))
			}
			lc := rgba{1, 0.35, 0.28, 1}
			if hot[i] {
				lc = cWhite
			}
			drawTextCentered(&bOpaque, label, e.panelPoint(s.cx, s.cy+0.011, face+0.0008), R, U, px, lc)
			spx := smallPx
			if tw := float64(textWidth(b.sub)) * spx; tw > s.w-0.03 {
				spx = (s.w - 0.03) / float64(textWidth(b.sub))
			}
			drawTextCentered(&bOpaque, b.sub, e.panelPoint(s.cx, s.cy-0.026, face+0.0008), R, U, spx, cAmber)
			continue
		}
		if on {
			a, b := e.panelPoint(x0+0.006, y0+0.008, face+0.0005), e.panelPoint(x0+0.006, y1-0.008, face+0.0005)
			ribbon(a, b, 0.006, cEnergy.alpha(0.9))
			sprite(e.panelPoint(x0+0.006, s.cy, face+0.001), 0.02, cEnergy.alpha(0.4))
		}
		px := textPx
		if tw := float64(textWidth(label)) * px; tw > s.w-0.02 {
			px = (s.w - 0.02) / float64(textWidth(label))
		}
		drawTextCentered(&bOpaque, label, e.panelPoint(s.cx, s.cy, face+0.0008), R, U, px, tcol)
	}
	tipIdx := -1
	for i := range e.cur {
		if hot[i] {
			tipIdx = i
		}
	}
	if tipIdx >= 0 {
		if tip := tipFor(e.cur[tipIdx].label()); tip != "" {
			const boxW, pad, tpx, lh = 0.34, 0.014, 0.0019, 0.02
			bw := boxW - 2*pad
			chars := int(bw / (6 * tpx))
			lines := wrapText(tip, chars)
			boxH := float64(len(lines))*lh + 2*pad - (lh - 7*tpx)
			s := slots[tipIdx]
			x0 := w2 + 0.06
			if s.cx < 0 {
				x0 = -w2 - 0.06 - boxW
			}
			yTop := math.Min(h2, math.Max(-h2+boxH, s.cy+boxH/2))
			e.panelQuad(&bTrans, x0, yTop-boxH, x0+boxW, yTop, 0.01, rgba{0.02, 0.05, 0.07, 0.93}, rgba{0.04, 0.09, 0.12, 0.93})
			e.panelBox(x0+boxW/2, yTop-boxH/2, 0.002, boxW/2+0.006, boxH/2+0.006, 0.006, 0.004, cFrame)
			e.panelGlowRect(x0, yTop-boxH, x0+boxW, yTop, 0.0105, cBeam.alpha(0.7), 0.3)
			ax := w2 + 0.012
			bx := x0
			if s.cx < 0 {
				ax, bx = -w2-0.012, x0+boxW
			}
			ay := math.Min(yTop-0.01, math.Max(yTop-boxH+0.01, s.cy))
			glowLine(e.panelPoint(ax, s.cy, 0.008), e.panelPoint(bx, ay, 0.0105), cBeam.alpha(0.8), 0.3)
			sprite(e.panelPoint(ax, s.cy, 0.008), 0.012, cBeam)
			for i, l := range lines {
				drawText(&bOpaque, l, e.panelPoint(x0+pad, yTop-pad-float64(i)*lh-7*tpx, 0.0112), R, U, tpx, cLaser)
			}
		}
	}
	ribbon(e.panelPoint(-w2+0.01, -h2+footerH-0.006, 0.002), e.panelPoint(w2-0.01, -h2+footerH-0.006, 0.002), 0.003, cBeam.alpha(0.4))
	drawText(&bOpaque, clip(e.status, 41), e.panelPoint(-w2+panelPad, -h2+0.014, 0.003), R, U, smallPx, cWhite)
}
