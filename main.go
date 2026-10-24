//go:build windows

package main

// ZoneTweak's main loop. The app starts in the boundary editor; the lever beside your play
// space (or PLAY CONTRABAND on the menu) switches to the game in the room you've shaped, and
// the game's pause menu switches back. Both run on the same window, OpenXR session, renderer
// and settings.

import (
	"boundaryvr/sim"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

const (
	modeZone = iota // ZoneTweak, the editor
	modeGame        // Contraband
)

var appMode = modeZone

func fail(msg string) {
	log.Println("FATAL:", msg)
	messageBox("ZoneTweak", msg)
	os.Exit(1)
}

func qualityName() string { return qualities[qualityIdx].name }

func cycleQuality() {
	qualityIdx = (qualityIdx + 1) % len(qualities)
	saveQuality(appDir, qualityIdx)
	wantQuality = qualityIdx
}

func cliUse(name string, lib spaceLib) {
	p, ok := lib.find(name)
	if !ok {
		var names []string
		for _, s := range lib.list() {
			names = append(names, s.Name)
		}
		fail(fmt.Sprintf("No saved space called %q.\n\nSaved spaces: %s", name, strings.Join(names, ", ")))
	}
	st, err := loadStore("")
	if err != nil {
		fail(err.Error())
	}
	pts := p.pts()
	if p.Frame == "raw" {
		pts = st.toStageAll(pts)
	}
	n, steamW, pimaxW, err := writeBoundary(st, pts, p.Height)
	if err != nil {
		fail("Couldn't write the boundary: " + err.Error())
	}
	lib.setActive(p.Name)
	log.Printf("used %s on universe %s, %d files", p.Name, st.uniID, n)
	msg := fmt.Sprintf("%s is now the boundary (universe %s, %d files).", p.Name, st.uniID, n)
	if steamW {
		err := steamvrReload()
		log.Printf("SteamVR reload: %v", err)
		switch {
		case err == nil:
			msg += "\n\nSteamVR updated it live."
		case errors.Is(err, errNoSteamVR):
			msg += "\n\nSteamVR picks it up next time it starts."
		default:
			msg += fmt.Sprintf("\n\nSteamVR didn't reload it (%v). Restart SteamVR to apply.", err)
		}
	}
	if pimaxW {
		askRestart(msg)
	} else {
		info(msg)
	}
}

func main() {
	if code, ok := steamvrMain(os.Args); ok {
		os.Exit(code)
	}
	runtime.LockOSThread()
	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	// the last session's log is kept beside this one (zonetweak-prev.log)
	os.Rename(filepath.Join(dir, "zonetweak.log"), filepath.Join(dir, "zonetweak-prev.log"))
	if f, err := os.OpenFile(filepath.Join(dir, "zonetweak.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644); err == nil {
		log.SetOutput(f)
		defer f.Close()
	}
	log.SetFlags(log.Ltime)
	// with no console window a crash would vanish: put it in the log and say so
	defer func() {
		if e := recover(); e != nil {
			log.Printf("PANIC: %v\n%s", e, debug.Stack())
			messageBox("ZoneTweak", fmt.Sprintf("ZoneTweak crashed: %v\n\nThe details are in zonetweak.log next to the exe.", e))
			os.Exit(2)
		}
	}()
	os.MkdirAll(filepath.Join(dir, "spaces"), 0755)
	ed.lib = spaceLib{dir: filepath.Join(dir, "spaces")}
	ed.prof = newPimaxProfile()
	ed.zone = ed.prof
	log.Printf("Pimax recenter allowed: %v", ed.prof.recenterOn())
	go steamWorker()
	ed.prof.onSet = func(key string, v float64, isFloat bool) {
		if isFloat {
			piSetFloat(key, v)
		} else {
			piSetInt(key, int(v))
		}
	}
	play := false
	for i, a := range os.Args {
		if (a == "--use" || a == "-use") && i+1 < len(os.Args) {
			cliUse(strings.Join(os.Args[i+1:], " "), ed.lib)
			return
		}
		if a == "--play" || a == "--contraband" {
			play = true // straight into the game (a desktop shortcut)
		}
	}

	if err := createWindow("ZoneTweak", 960, 600); err != nil {
		fail("Couldn't create the window: " + err.Error())
	}
	if err := loadGL(); err != nil {
		fail(err.Error())
	}
	keepAwake()
	st, err := loadStore("")
	if err != nil {
		// no boundary files to edit (a headset whose runtime keeps its own): the game still runs
		log.Printf("no Pimax or SteamVR boundary files: %v", err)
		st = nil
	}
	ed.st = st
	if st != nil {
		ed.loadFromStore()
		for _, f := range st.files {
			log.Println("found", f)
		}
		for _, id := range st.ids {
			u := st.unis[id]
			log.Printf("universe %s: %d points, pimax=%v, %d file(s), source %s", id, u.points, u.pimax, len(u.files), u.source)
		}
		log.Printf("editing universe %s, %d points, writes %v, standing %.3f %.3f yaw %.3f, align %s", st.uniID, len(st.pts), st.uniFiles, st.tx, st.tz, st.yaw, frameModeName(st.mode))
	}
	// the game's numbers can be overridden by a file beside the exe (for tuning playtests)
	if b, err := os.ReadFile(filepath.Join(dir, "contraband-tuning.json")); err == nil {
		if t, err := sim.TuningWith(b); err != nil {
			log.Printf("contraband-tuning.json ignored: %v", err)
		} else {
			gameTuning = t
			log.Printf("tuning override loaded")
		}
	}

	go registerWithPimax(dir, exe)
	if err := xrInit(); err != nil {
		fail(err.Error())
	}
	defer xrShutdown()
	if strings.Contains(strings.ToLower(xs.runtimeName), "steamvr") {
		boundarySys = "SteamVR"
		ed.zone = newSteamZone(configDirs(), func(key, kind string, v float64) error {
			queueSteam(func() {
				if err := steamvrSet(key, kind, v); err != nil {
					asyncSay <- fmt.Sprintf("Couldn't change the SteamVR setting: %v", err)
				}
			})
			return nil
		})
	}
	log.Printf("OpenXR runtime %q, headset %q, boundary owner %s", xs.runtimeName, xs.systemName, boundarySys)
	if steamMode() && st != nil {
		// edit whatever SteamVR is using now: nobody should have to pick a universe
		live, err := steamvrLive()
		log.Printf("SteamVR live bounds: %d points, %v", len(live), err)
		if id := st.steamUniverse(live); id != "" && id != st.uniID {
			if err := st.selectUniverse(id); err == nil {
				ed.loadFromStore()
				log.Printf("switched to SteamVR's universe %s, %d points, writes %v", id, len(st.pts), st.uniFiles)
			}
		}
	}
	qi := loadQuality(dir)
	qualityIdx, appDir = qi, dir
	gloveDir = dir
	loadGloves()
	rr, err := newRenderer(qi)
	if err != nil {
		fail("Graphics setup failed: " + err.Error())
	}
	R = rr
	var eyes [2]*eyeTarget
	for i := 0; i < 2; i++ {
		t, err := newEyeTargetBest(scaled(xs.w[i]), scaled(xs.h[i]), int(R.q.samples))
		if err != nil {
			fail(err.Error())
		}
		eyes[i] = t
	}
	if st != nil {
		ed.buildPages()
	}
	startAudio()
	defer stopAudio()

	hs := []*hand{{idx: 0, grab: -1, hoverV: -1, hoverE: -1, btn: -1}, {idx: 1, grab: -1, hoverV: -1, hoverE: -1, btn: -1}}
	ed.hands = hs
	ed.laserHand = hs[1]
	ed.laser = true // start with the laser pointer
	initHands(hs)

	// ---- the game, and back ----
	watch := newBoundaryWatch(nil)
	var snap zoneSnap
	newRound := func(P []pt2) {
		roomWorld = clonePts(P)
		G = newGame(clonePts(P), time.Now().UnixNano()%100000)
	}
	enterGame := func() {
		if appMode == modeGame {
			return
		}
		snap = takeZoneSnap()
		P := gameRoom(dir, st)
		stageFix, stageFixOn = identity2(), false
		watchSt := st
		if roomSource != "headset" {
			watchSt = nil // only a boundary file can move under us
		}
		watch = newBoundaryWatch(watchSt)
		newRound(P)
		appMode = modeGame
		gloveIn = fxTime()
		log.Printf("Contraband: %s room, %d corners, %.1f m2", roomSource, len(P), math.Abs(polyArea(P)))
	}
	leaveGame := func() {
		if appMode != modeGame || st == nil {
			return
		}
		snap.restore()
		// re-centred while you played: the boundary you're editing follows it too
		if stageFixOn {
			ed.applyStageMove(stageFix.inv())
		}
		stageFix, stageFixOn = identity2(), false
		resetEngineForZone()
		G = nil
		appMode = modeZone
		ed.say("Back in ZoneTweak")
		log.Printf("back to ZoneTweak")
	}
	onPlay = func() { wantMode = modeGame }
	onLeave = func() { wantMode = modeZone }
	canLeave = st != nil
	onTraced = func(P []pt2) {
		saveTraced(dir, stageFix.inv().all(P))
		roomSource, watch.st = "traced", nil
		newRound(P)
		log.Printf("traced room: %d corners, %.1f m2", len(P), math.Abs(polyArea(P)))
	}
	onHeadsetRoom = func() {
		saveTraced(dir, nil)
		P := gameRoom(dir, st)
		stageFix, stageFixOn = identity2(), false
		if roomSource == "headset" {
			watch.st = st
		}
		newRound(P)
	}
	xs.onStageMoved = func(m hmdMatrix34) {
		if appMode == modeGame {
			stageMoved(m)
			floorOff = 0 // the runtime's had another go at the floor: start over from its
			movedSay()
			watch.next = time.Time{} // and see what the boundary file says about it, now
			return
		}
		oldFromNew := rigid2{a: float64(m[0][0]), b: float64(m[0][2]), c: float64(m[2][0]), d: float64(m[2][2]), tx: float64(m[0][3]), tz: float64(m[2][3])}
		ed.applyStageMove(oldFromNew.inv())
		ed.say("%s recentered. Boundary kept in place", boundarySys)
	}
	if st == nil || play {
		enterGame()
	} else {
		ed.say("%s boundary, universe %s, %d points. B opens the menu", boundarySys, st.uniID, len(st.pts))
	}

	exitRequested := false
	lastFrame := time.Now()
	awakeAt := time.Now()
	for !xs.quit {
		pumpMessages()
		if (ed.quit || windowQuit) && !exitRequested {
			exitRequested = true
			if !xs.running {
				break
			}
			xrRequestExit()
		}
		xrPollEvents()
		if !xs.running {
			if exitRequested {
				break
			}
			time.Sleep(20 * time.Millisecond)
			continue
		}
		fi, err := xrBeginFrameAndLocate()
		if err != nil {
			log.Println(err)
			time.Sleep(10 * time.Millisecond)
			continue
		}
		if time.Since(awakeAt) > 30*time.Second {
			keepAwake() // you're in the headset: no blanking the monitors
			awakeAt = time.Now()
		}
		switch {
		case wantMode == modeGame:
			enterGame()
		case wantMode == modeZone:
			leaveGame()
		}
		wantMode = -1
		frameDT = math.Max(0.001, math.Min(0.1, time.Since(lastFrame).Seconds()))
		lastFrame = time.Now()
		if appMode == modeGame && watch.check() {
			movedSay()
		}
		if fi.headValid {
			ed.hmd = fixPose(fi.head)
			ed.hmd[1][3] -= float32(floorOff) // the floor where it really is (see game_floor.go)
		}
		xrSync()
		for i, h := range hs {
			h.m, h.valid = xrLocate(xs.hands[i].space, fi.time)
			h.aim, h.aimValid = xrLocate(xs.hands[i].aimSpace, fi.time)
			h.m, h.aim = fixPose(h.m), fixPose(h.aim)
			h.m[1][3] -= float32(floorOff)
			h.aim[1][3] -= float32(floorOff)
			h.updateLaser(frameDT)
			h.prev = h.pressed
			h.pressed = 0
			sub := xs.hands[i].path
			h.grip = xrFloat(xs.aGrabF, sub)
			h.force = xrFloat(xs.aGrabForce, sub)
			h.trig = xrFloat(xs.aTrigF, sub)
			touch := strings.Contains(ctrlProfile[i], "touch")
			if touch {
				// no trigger click on Touch: the last 3% of the pull is the click, the rest
				// is the half-pull (aim) - as on an Index trigger
				was := h.prev&(1<<bitTrigger) != 0
				if h.trig >= 0.97 || was && h.trig >= 0.88 {
					h.pressed |= 1 << bitTrigger
				}
			} else if xrBool(xs.aTrigger, sub) {
				h.pressed |= 1 << bitTrigger
			}
			if xrBool(xs.aDelete, sub) {
				h.pressed |= 1 << bitGrip
			}
			h.aTouch = xrBool(xs.aATouch, sub)
			h.bTouch = xrBool(xs.aBTouch, sub)
			if xrBool(xs.aMenu, sub) {
				h.pressed |= 1 << bitMenu
			}
			if xrBool(xs.aUndo, sub) {
				h.pressed |= 1 << bitStick
			}
			if xrBool(xs.aGrab, sub) {
				h.pressed |= 1 << bitGrab
			}
			if xrBool(xs.aTool, sub) {
				h.pressed |= 1 << bitTool
			}
		}
		if q := wantQuality >= 0 && wantQuality != R.q.idx(); q || wantRebuild {
			// rebuild everything at the new quality or render scale between frames
			if q {
				R.setQuality(wantQuality)
			}
			for i := 0; i < 2; i++ {
				if t, err := newEyeTargetBest(scaled(xs.w[i]), scaled(xs.h[i]), int(R.q.samples)); err == nil {
					freePosts(eyes[i])
					eyes[i].free()
					eyes[i] = t
				} else {
					log.Printf("graphics switch: %v", err)
				}
			}
		}
		wantQuality, wantRebuild = -1, false
		select {
		case m := <-asyncSay:
			ed.say("%s", m)
		default:
		}
		cpu0 := time.Now()
		xs.waited = 0
		if appMode == modeGame {
			G.frame(frameDT)
			G.buildScene()
		} else {
			ed.cur = ed.page.buttons()
			for _, h := range hs {
				if h.valid {
					ed.handleHand(h)
				}
			}
			ed.updateFloor()
			if autoFloor.step(ed.hmd.pos(), frameDT, false) {
				ed.say("Floor adjusted. Not right? Menu > Fix floor")
			}
			ed.cur = ed.page.buttons()
			ed.buildScene()
		}
		nl := R.beginFrame(ed.hmd.pos())
		if err := xrRenderAndEnd(fi, eyes, func(eye int, t *eyeTarget, proj mat4, view hmdMatrix34, dst uint32, dw, dh int32) {
			R.renderEye(eye, t, proj, fixView(view), ed.hmd.pos(), nl, float32(floorOff), dst, dw, dh)
		}); err != nil {
			log.Println(err)
		}
		// CPU work only: the time spent blocked waiting on the runtime (the GPU) doesn't count
		gt.cpu += ((time.Since(cpu0)-xs.waited).Seconds()*1000 - gt.cpu) * 0.05
		xs.waited = 0
		swapBuffers()
	}
	xrShutdown()
	log.Println("exit")
	switch {
	case steamMode():
		if ed.savedAny && ed.liveFailed.Load() {
			info("Your changes are saved. SteamVR didn't take them live, so restart SteamVR to apply them.")
		}
	case ed.restartAfter:
		restartPimax()
	case ed.savedAny || ed.prof.changed:
		askRestart("Your changes are saved. Pimax applies them after a restart.")
	}
}
