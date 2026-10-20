//go:build windows

package main

import (
	"fmt"
	"log"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

const (
	xrTypeInstanceCreateInfo  = 3
	xrTypeSystemGetInfo       = 4
	xrTypeSessionCreateInfo   = 8
	xrTypeSwapchainCreateInfo = 9
	xrTypeSessionBeginInfo    = 10
	xrTypeViewState           = 11
	xrTypeFrameEndInfo        = 12
	xrTypeHapticVibration     = 13
	xrTypeEventBuffer         = 16
	xrTypeEventLossPending    = 17
	xrTypeEventSessionState   = 18
	xrTypeActionStateBool     = 23
	xrTypeActionSetCreateInfo = 28
	xrTypeActionCreateInfo    = 29
	xrTypeFrameWaitInfo       = 33
	xrTypeLayerProjection     = 35
	xrTypeRefSpaceCreateInfo  = 37
	xrTypeActionSpaceCreate   = 38
	xrTypeViewConfigView      = 41
	xrTypeSpaceLocation       = 42
	xrTypeFrameState          = 44
	xrTypeFrameBeginInfo      = 46
	xrTypeProjView            = 48
	xrTypeProfileBindings     = 51
	xrTypeAcquireInfo         = 55
	xrTypeWaitInfo            = 56
	xrTypeReleaseInfo         = 57
	xrTypeActionStateGetInfo  = 58
	xrTypeHapticActionInfo    = 59
	xrTypeAttachInfo          = 60
	xrTypeSyncInfo            = 61
	xrTypeViewLocateInfo      = 6
	xrTypeView                = 7
	xrTypeBindingOpenGLWin32  = 1000023000
	xrTypeSwapchainImageGL    = 1000023004
	xrTypeGraphicsReqOpenGL   = 1000023005
	xrStereo                  = 2
	xrRefView                 = 1
	xrRefLocal                = 2
	xrRefStage                = 3
	xrActionBool              = 1
	xrActionPose              = 4
	xrActionVibration         = 100
	xrStateReady              = 2
	xrStateStopping           = 6
	xrStateLossPending        = 7
	xrStateExiting            = 8
	xrInfinite                = 0x7fffffffffffffff
	glSRGB8Alpha8             = 0x8C43
)

var (
	xrDLL = syscall.NewLazyDLL("openxr_loader.dll")
	xrF   = map[string]uintptr{}

	pwglGetCurrentContext = opengl32.NewProc("wglGetCurrentContext")
)

type xrHand struct {
	path     uint64
	space    uintptr
	aimSpace uintptr
}

type xrState struct {
	inst, session, stage, viewSpace        uintptr
	sysID                                  uint64
	views                                  [2]xrViewConfigView
	swap                                   [2]uintptr
	images                                 [2][]xrSwapchainImageGL
	w, h                                   [2]int32
	format                                 int64
	actionSet                              uintptr
	aTrigger, aDelete, aMenu, aUndo, aTool uintptr
	aPose, aHaptic, aAim                   uintptr
	hands                                  [2]xrHand
	running, quit                          bool
	exts                                   []string
	onStageMoved                           func(oldFromNew hmdMatrix34)
	alphaBlend, camera                     bool
	stageName                              string
	frameState                             *xrFrameState
	viewArr                                *[2]xrView
	projViews                              *[2]xrProjView
	layer                                  *xrLayerProjection
	layerPtr                               *uintptr
	event                                  *xrEventBuffer
	dstFBO                                 uint32
	hasMask                                bool
	waited                                 time.Duration // time this frame spent blocked in the runtime
	mask                                   [2][]float32  // hidden-area triangles per eye, view space
	maskVer                                int
}

var xs = &xrState{
	frameState: new(xrFrameState),
	viewArr:    new([2]xrView),
	projViews:  new([2]xrProjView),
	layer:      new(xrLayerProjection),
	layerPtr:   new(uintptr),
	event:      new(xrEventBuffer),
}

func xfn(name string) uintptr {
	if p, ok := xrF[name]; ok {
		return p
	}
	pr := xrDLL.NewProc(name)
	if pr.Find() != nil {
		return 0
	}
	xrF[name] = pr.Addr()
	return xrF[name]
}

//go:uintptrescapes
func xcall(name string, a ...uintptr) int32 {
	p := xfn(name)
	if p == 0 {
		return -1000
	}
	r, _, _ := syscall.SyscallN(p, a...)
	return int32(r)
}

func xok(r int32, what string) error {
	if r < 0 {
		return fmt.Errorf("%s failed (XrResult %d)", what, r)
	}
	return nil
}

func cbytes(s string) []byte { return append([]byte(s), 0) }

func setName(dst []byte, s string) { copy(dst, s) }

func identityPose() xrPose { return xrPose{o: xrQuat{w: 1}} }

func xrPath(s string) uint64 {
	var p uint64
	b := cbytes(s)
	xcall("xrStringToPath", xs.inst, uintptr(unsafe.Pointer(&b[0])), uintptr(unsafe.Pointer(&p)))
	return p
}

type xrExtProps struct {
	typ     int32
	next    uintptr
	name    [128]byte
	version uint32
}

func cName(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

func xrInit() error {
	if err := xrDLL.Load(); err != nil {
		return fmt.Errorf("openxr_loader.dll not found next to the exe: %w", err)
	}
	var ec uint32
	xcall("xrEnumerateInstanceExtensionProperties", 0, 0, uintptr(unsafe.Pointer(&ec)), 0)
	if ec > 0 {
		props := make([]xrExtProps, ec)
		for i := range props {
			props[i].typ = 2
		}
		xcall("xrEnumerateInstanceExtensionProperties", 0, uintptr(ec), uintptr(unsafe.Pointer(&ec)), uintptr(unsafe.Pointer(&props[0])))
		for _, p := range props[:ec] {
			xs.exts = append(xs.exts, cName(p.name[:]))
		}
	}
	log.Printf("runtime extensions: %v", xs.exts)
	ext := cbytes("XR_KHR_opengl_enable")
	extPtrs := []uintptr{uintptr(unsafe.Pointer(&ext[0]))}
	vmExt := cbytes("XR_KHR_visibility_mask")
	for _, e := range xs.exts {
		if e == "XR_KHR_visibility_mask" {
			extPtrs = append(extPtrs, uintptr(unsafe.Pointer(&vmExt[0])))
			xs.hasMask = true
		}
	}
	ci := &xrInstanceCreateInfo{typ: xrTypeInstanceCreateInfo, extCount: uint32(len(extPtrs)), extNames: uintptr(unsafe.Pointer(&extPtrs[0]))}
	setName(ci.app.appName[:], "ZoneTweak")
	setName(ci.app.engineName[:], "none")
	ci.app.apiVersion = 1 << 48
	if err := xok(xcall("xrCreateInstance", uintptr(unsafe.Pointer(ci)), uintptr(unsafe.Pointer(&xs.inst))), "xrCreateInstance"); err != nil {
		return fmt.Errorf("%w. Is Pimax set as the active OpenXR runtime?", err)
	}
	runtime.KeepAlive(ext)
	runtime.KeepAlive(vmExt)
	runtime.KeepAlive(extPtrs)
	gi := &xrSystemGetInfo{typ: xrTypeSystemGetInfo, formFactor: 1}
	if err := xok(xcall("xrGetSystem", xs.inst, uintptr(unsafe.Pointer(gi)), uintptr(unsafe.Pointer(&xs.sysID))), "xrGetSystem"); err != nil {
		return fmt.Errorf("%w. Is the headset connected?", err)
	}
	var bc uint32
	modes := make([]int32, 8)
	xcall("xrEnumerateEnvironmentBlendModes", xs.inst, uintptr(xs.sysID), xrStereo, 8, uintptr(unsafe.Pointer(&bc)), uintptr(unsafe.Pointer(&modes[0])))
	for _, m := range modes[:min(bc, 8)] {
		if m == 3 {
			xs.alphaBlend = true
		}
	}
	log.Printf("blend modes: %v", modes[:min(bc, 8)])
	var reqFn uintptr
	nm := cbytes("xrGetOpenGLGraphicsRequirementsKHR")
	xcall("xrGetInstanceProcAddr", xs.inst, uintptr(unsafe.Pointer(&nm[0])), uintptr(unsafe.Pointer(&reqFn)))
	if reqFn == 0 {
		return fmt.Errorf("runtime has no OpenGL support")
	}
	req := &xrGraphicsRequirementsOpenGL{typ: xrTypeGraphicsReqOpenGL}
	r, _, _ := syscall.SyscallN(reqFn, xs.inst, uintptr(xs.sysID), uintptr(unsafe.Pointer(req)))
	if err := xok(int32(r), "xrGetOpenGLGraphicsRequirementsKHR"); err != nil {
		return err
	}

	if err := xrCreateActions(); err != nil {
		return err
	}

	ctx, _, _ := pwglGetCurrentContext.Call()
	bind := &xrGraphicsBindingOpenGLWin32{typ: xrTypeBindingOpenGLWin32, hDC: hdc, hGLRC: ctx}
	sci := &xrSessionCreateInfo{typ: xrTypeSessionCreateInfo, next: uintptr(unsafe.Pointer(bind)), systemId: xs.sysID}
	err := xok(xcall("xrCreateSession", xs.inst, uintptr(unsafe.Pointer(sci)), uintptr(unsafe.Pointer(&xs.session))), "xrCreateSession")
	runtime.KeepAlive(bind)
	if err != nil {
		return err
	}
	sets := []uintptr{xs.actionSet}
	ai := &xrAttachInfo{typ: xrTypeAttachInfo, count: 1, sets: uintptr(unsafe.Pointer(&sets[0]))}
	err = xok(xcall("xrAttachSessionActionSets", xs.session, uintptr(unsafe.Pointer(ai))), "xrAttachSessionActionSets")
	runtime.KeepAlive(sets)
	if err != nil {
		return err
	}

	rs := &xrReferenceSpaceCreateInfo{typ: xrTypeRefSpaceCreateInfo, refType: xrRefStage, pose: identityPose()}
	xs.stageName = "STAGE"
	if xcall("xrCreateReferenceSpace", xs.session, uintptr(unsafe.Pointer(rs)), uintptr(unsafe.Pointer(&xs.stage))) < 0 {
		rs.refType = xrRefLocal
		xs.stageName = "LOCAL"
		if err := xok(xcall("xrCreateReferenceSpace", xs.session, uintptr(unsafe.Pointer(rs)), uintptr(unsafe.Pointer(&xs.stage))), "xrCreateReferenceSpace"); err != nil {
			return err
		}
	}
	rs.refType = xrRefView
	if err := xok(xcall("xrCreateReferenceSpace", xs.session, uintptr(unsafe.Pointer(rs)), uintptr(unsafe.Pointer(&xs.viewSpace))), "view space"); err != nil {
		return err
	}
	for i := range xs.hands {
		as := &xrActionSpaceCreateInfo{typ: xrTypeActionSpaceCreate, action: xs.aPose, subPath: xs.hands[i].path, pose: identityPose()}
		if err := xok(xcall("xrCreateActionSpace", xs.session, uintptr(unsafe.Pointer(as)), uintptr(unsafe.Pointer(&xs.hands[i].space))), "hand space"); err != nil {
			return err
		}
		aim := &xrActionSpaceCreateInfo{typ: xrTypeActionSpaceCreate, action: xs.aAim, subPath: xs.hands[i].path, pose: identityPose()}
		if err := xok(xcall("xrCreateActionSpace", xs.session, uintptr(unsafe.Pointer(aim)), uintptr(unsafe.Pointer(&xs.hands[i].aimSpace))), "aim space"); err != nil {
			return err
		}
	}

	var n uint32
	xs.views[0].typ, xs.views[1].typ = xrTypeViewConfigView, xrTypeViewConfigView
	if err := xok(xcall("xrEnumerateViewConfigurationViews", xs.inst, uintptr(xs.sysID), xrStereo, 2, uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&xs.views[0]))), "view configuration"); err != nil {
		return err
	}
	var fc uint32
	xcall("xrEnumerateSwapchainFormats", xs.session, 0, uintptr(unsafe.Pointer(&fc)), 0)
	formats := make([]int64, fc+1)
	xcall("xrEnumerateSwapchainFormats", xs.session, uintptr(fc), uintptr(unsafe.Pointer(&fc)), uintptr(unsafe.Pointer(&formats[0])))
	xs.format = 0
	for _, f := range formats[:fc] {
		if f == glRGBA8 {
			xs.format = f
		}
	}
	if xs.format == 0 {
		for _, f := range formats[:fc] {
			if f == glSRGB8Alpha8 {
				xs.format = f
			}
		}
	}
	if xs.format == 0 && fc > 0 {
		xs.format = formats[0]
	}
	log.Printf("OpenXR space %s, swapchain format 0x%x of %v", xs.stageName, xs.format, formats[:fc])
	for i := 0; i < 2; i++ {
		w, h := int32(xs.views[i].recW), int32(xs.views[i].recH)
		if m := max(w, h); m > 3200 {
			w, h = w*3200/m, h*3200/m
		}
		xs.w[i], xs.h[i] = w, h
		sc := &xrSwapchainCreateInfo{typ: xrTypeSwapchainCreateInfo, usage: 0x1 | 0x10, format: xs.format, sampleCount: 1,
			width: uint32(w), height: uint32(h), faceCount: 1, arraySize: 1, mipCount: 1}
		if err := xok(xcall("xrCreateSwapchain", xs.session, uintptr(unsafe.Pointer(sc)), uintptr(unsafe.Pointer(&xs.swap[i]))), "xrCreateSwapchain"); err != nil {
			return err
		}
		var ic uint32
		xcall("xrEnumerateSwapchainImages", xs.swap[i], 0, uintptr(unsafe.Pointer(&ic)), 0)
		imgs := make([]xrSwapchainImageGL, ic)
		for k := range imgs {
			imgs[k].typ = xrTypeSwapchainImageGL
		}
		if err := xok(xcall("xrEnumerateSwapchainImages", xs.swap[i], uintptr(ic), uintptr(unsafe.Pointer(&ic)), uintptr(unsafe.Pointer(&imgs[0]))), "swapchain images"); err != nil {
			return err
		}
		xs.images[i] = imgs
		log.Printf("eye %d: %dx%d, %d images", i, w, h, ic)
	}
	xs.dstFBO = genOne(glGenFramebuffersP)
	xrLoadMasks()
	return nil
}

type xrVisibilityMask struct {
	typ              int32
	next             uintptr
	vertCap, vertCnt uint32
	verts            uintptr
	idxCap, idxCnt   uint32
	idx              uintptr
}

// xrLoadMasks: the triangles of each eye's image hidden behind the lens edge, as view-space
// points on the z = -1 plane (XR_KHR_visibility_mask). Nothing is drawn there.
func xrLoadMasks() {
	if !xs.hasMask {
		log.Printf("visibility mask: not offered by the runtime")
		return
	}
	var fn uintptr
	nm := cbytes("xrGetVisibilityMaskKHR")
	xcall("xrGetInstanceProcAddr", xs.inst, uintptr(unsafe.Pointer(&nm[0])), uintptr(unsafe.Pointer(&fn)))
	if fn == 0 {
		return
	}
	for i := 0; i < 2; i++ {
		m := &xrVisibilityMask{typ: 1000031000}
		r, _, _ := syscall.SyscallN(fn, xs.session, xrStereo, uintptr(i), 1, uintptr(unsafe.Pointer(m)))
		if int32(r) < 0 || m.vertCnt == 0 || m.idxCnt == 0 {
			log.Printf("visibility mask eye %d: none (%d)", i, int32(r))
			continue
		}
		verts := make([]float32, m.vertCnt*2)
		idx := make([]uint32, m.idxCnt)
		m2 := &xrVisibilityMask{typ: 1000031000, vertCap: m.vertCnt, verts: uintptr(unsafe.Pointer(&verts[0])), idxCap: m.idxCnt, idx: uintptr(unsafe.Pointer(&idx[0]))}
		r, _, _ = syscall.SyscallN(fn, xs.session, xrStereo, uintptr(i), 1, uintptr(unsafe.Pointer(m2)))
		runtime.KeepAlive(verts)
		runtime.KeepAlive(idx)
		if int32(r) < 0 {
			continue
		}
		var tri []float32
		for _, k := range idx[:m2.idxCnt] {
			if int(k)*2+1 < len(verts) {
				tri = append(tri, verts[k*2], verts[k*2+1], -1)
			}
		}
		xs.mask[i] = tri
		xs.maskVer++
		log.Printf("visibility mask eye %d: %d triangles", i, len(tri)/9)
	}
}

func xrCreateActions() error {
	asi := &xrActionSetCreateInfo{typ: xrTypeActionSetCreateInfo}
	setName(asi.name[:], "editor")
	setName(asi.localized[:], "Boundary editing")
	if err := xok(xcall("xrCreateActionSet", xs.inst, uintptr(unsafe.Pointer(asi)), uintptr(unsafe.Pointer(&xs.actionSet))), "xrCreateActionSet"); err != nil {
		return err
	}
	xs.hands[0].path = xrPath("/user/hand/left")
	xs.hands[1].path = xrPath("/user/hand/right")
	subs := []uint64{xs.hands[0].path, xs.hands[1].path}
	mk := func(name, label string, typ int32, out *uintptr) error {
		ci := &xrActionCreateInfo{typ: xrTypeActionCreateInfo, actionType: typ, subCount: 2, subPaths: uintptr(unsafe.Pointer(&subs[0]))}
		setName(ci.name[:], name)
		setName(ci.localized[:], label)
		err := xok(xcall("xrCreateAction", xs.actionSet, uintptr(unsafe.Pointer(ci)), uintptr(unsafe.Pointer(out))), "xrCreateAction "+name)
		runtime.KeepAlive(subs)
		return err
	}
	for _, a := range []struct {
		n, l string
		t    int32
		p    *uintptr
	}{
		{"grab", "Grab / place point", xrActionBool, &xs.aTrigger},
		{"delete", "Delete point", xrActionBool, &xs.aDelete},
		{"menu", "Menu", xrActionBool, &xs.aMenu},
		{"undo", "Undo", xrActionBool, &xs.aUndo},
		{"tool", "Switch tool mode", xrActionBool, &xs.aTool},
		{"hand", "Hand pose", xrActionPose, &xs.aPose},
		{"aim", "Pointer", xrActionPose, &xs.aAim},
		{"buzz", "Haptics", xrActionVibration, &xs.aHaptic},
	} {
		if err := mk(a.n, a.l, a.t, a.p); err != nil {
			return err
		}
	}
	type bind struct {
		a *uintptr
		l string
		r string
	}
	profiles := map[string][]bind{
		"/interaction_profiles/valve/index_controller": {
			{&xs.aTrigger, "trigger/click", ""}, {&xs.aDelete, "a/click", ""}, {&xs.aMenu, "b/click", ""},
			{&xs.aUndo, "thumbstick/click", ""}, {&xs.aTool, "trackpad/force", ""}, {&xs.aPose, "grip/pose", ""}, {&xs.aAim, "aim/pose", ""}, {&xs.aHaptic, "haptic", ""}},
		"/interaction_profiles/htc/vive_controller": {
			{&xs.aTrigger, "trigger/click", ""}, {&xs.aDelete, "squeeze/click", ""}, {&xs.aMenu, "menu/click", ""},
			{&xs.aUndo, "trackpad/click", ""}, {&xs.aPose, "grip/pose", ""}, {&xs.aAim, "aim/pose", ""}, {&xs.aHaptic, "haptic", ""}},
		"/interaction_profiles/oculus/touch_controller": {
			{&xs.aTrigger, "trigger/value", ""}, {&xs.aDelete, "x/click", "a/click"}, {&xs.aMenu, "y/click", "b/click"},
			{&xs.aUndo, "thumbstick/click", ""}, {&xs.aTool, "squeeze/value", ""}, {&xs.aPose, "grip/pose", ""}, {&xs.aAim, "aim/pose", ""}, {&xs.aHaptic, "haptic", ""}},
		"/interaction_profiles/khr/simple_controller": {
			{&xs.aTrigger, "select/click", ""}, {&xs.aMenu, "menu/click", ""}, {&xs.aPose, "grip/pose", ""}, {&xs.aAim, "aim/pose", ""}, {&xs.aHaptic, "haptic", ""}},
	}
	suggest := func(prof string, bs []bind, skipTool bool) int32 {
		var sb []xrSuggestedBinding
		for _, b := range bs {
			if skipTool && b.a == &xs.aTool {
				continue
			}
			for side, hand := range []string{"left", "right"} {
				comp := b.l
				if side == 1 && b.r != "" {
					comp = b.r
				}
				kind := "input"
				if comp == "haptic" {
					kind = "output"
				}
				sb = append(sb, xrSuggestedBinding{action: *b.a, binding: xrPath("/user/hand/" + hand + "/" + kind + "/" + comp)})
			}
		}
		pb := &xrProfileBindings{typ: xrTypeProfileBindings, profile: xrPath(prof), count: uint32(len(sb)), bindings: uintptr(unsafe.Pointer(&sb[0]))}
		r := xcall("xrSuggestInteractionProfileBindings", xs.inst, uintptr(unsafe.Pointer(pb)))
		runtime.KeepAlive(sb)
		return r
	}
	for prof, bs := range profiles {
		r := suggest(prof, bs, false)
		log.Printf("bindings %s: %d", prof, r)
		if r != 0 {
			r = suggest(prof, bs, true)
			log.Printf("bindings %s without tool button: %d", prof, r)
		}
	}
	return nil
}

func xrPollEvents() {
	for {
		*xs.event = xrEventBuffer{typ: xrTypeEventBuffer}
		if xcall("xrPollEvent", xs.inst, uintptr(unsafe.Pointer(xs.event))) != 0 {
			return
		}
		switch xs.event.typ {
		case xrTypeEventLossPending:
			xs.quit = true
		case 1000031001: // the runtime changed the lens mask (FOV or IPD setting)
			xrLoadMasks()
		case 40:
			ev := (*xrEventSpaceChange)(unsafe.Pointer(xs.event))
			log.Printf("reference space %d changed, pose valid %d, pos %.3f %.3f %.3f", ev.refType, ev.poseValid, ev.pose.p.x, ev.pose.p.y, ev.pose.p.z)
			if ev.poseValid != 0 && ev.refType == xrRefStage && xs.stageName == "STAGE" && xs.onStageMoved != nil {
				xs.onStageMoved(poseTo34(ev.pose))
			}
		case xrTypeEventSessionState:
			st := (*xrEventSessionState)(unsafe.Pointer(xs.event)).state
			log.Printf("session state %d", st)
			switch st {
			case xrStateReady:
				bi := &xrSessionBeginInfo{typ: xrTypeSessionBeginInfo, viewConfig: xrStereo}
				if r := xcall("xrBeginSession", xs.session, uintptr(unsafe.Pointer(bi))); r >= 0 {
					xs.running = true
				} else {
					log.Printf("xrBeginSession %d", r)
				}
			case xrStateStopping:
				xcall("xrEndSession", xs.session)
				xs.running = false
			case xrStateExiting, xrStateLossPending:
				xs.quit = true
			}
		}
	}
}

func xrBool(action uintptr, sub uint64) bool {
	gi := &xrActionStateGetInfo{typ: xrTypeActionStateGetInfo, action: action, subPath: sub}
	st := &xrActionStateBool{typ: xrTypeActionStateBool}
	if xcall("xrGetActionStateBoolean", xs.session, uintptr(unsafe.Pointer(gi)), uintptr(unsafe.Pointer(st))) < 0 {
		return false
	}
	return st.active != 0 && st.current != 0
}

func xrLocate(space uintptr, t int64) (hmdMatrix34, bool) {
	loc := &xrSpaceLocation{typ: xrTypeSpaceLocation}
	if xcall("xrLocateSpace", space, xs.stage, uintptr(t), uintptr(unsafe.Pointer(loc))) < 0 {
		return hmdMatrix34{}, false
	}
	return poseTo34(loc.pose), loc.flags&0x3 == 0x3
}

func xrSync() {
	set := []xrActiveActionSet{{set: xs.actionSet}}
	si := &xrActionsSyncInfo{typ: xrTypeSyncInfo, count: 1, sets: uintptr(unsafe.Pointer(&set[0]))}
	xcall("xrSyncActions", xs.session, uintptr(unsafe.Pointer(si)))
	runtime.KeepAlive(set)
}

func haptic(i int, us int) { hapticAmp(i, us/100, 0.6) }

// hapticAmp: ms of vibration at amplitude 0..1
func hapticAmp(i int, ms int, amp float32) {
	if i < 0 || i > 1 || xs.session == 0 {
		return
	}
	hi := &xrActionStateGetInfo{typ: xrTypeHapticActionInfo, action: xs.aHaptic, subPath: xs.hands[i].path}
	v := &xrHapticVibration{typ: xrTypeHapticVibration, duration: int64(ms) * 1000000, amplitude: amp}
	xcall("xrApplyHapticFeedback", xs.session, uintptr(unsafe.Pointer(hi)), uintptr(unsafe.Pointer(v)))
}

func xrRequestExit() {
	if xs.session != 0 {
		xcall("xrRequestExitSession", xs.session)
	}
}

func (x *xrState) mirror(srcW, srcH, ww, wh int32) {
	if ww <= 0 || wh <= 0 {
		return
	}
	call(glBindFramebufferP, glReadFramebuffer, uintptr(x.dstFBO))
	call(glBindFramebufferP, glDrawFramebuffer, 0)
	call(glViewportP, 0, 0, uintptr(ww), uintptr(wh))
	sw, sh := float32(srcW), float32(srcH)
	scale := float32(ww) / sw
	if s2 := float32(wh) / sh; s2 > scale {
		scale = s2
	}
	cw, ch := float32(ww)/scale, float32(wh)/scale
	x0, y0 := (sw-cw)/2, (sh-ch)/2
	call(glBlitFramebufferP, uintptr(int32(x0)), uintptr(int32(y0)), uintptr(int32(x0+cw)), uintptr(int32(y0+ch)), 0, 0, uintptr(ww), uintptr(wh), glColorBufferBit, glLinear)
	call(glBindFramebufferP, glFramebuffer, 0)
}

type frameInfo struct {
	time         int64
	shouldRender bool
	head         hmdMatrix34
	headValid    bool
}

func xrBeginFrameAndLocate() (frameInfo, error) {
	var fi frameInfo
	*xs.frameState = xrFrameState{typ: xrTypeFrameState}
	wi := &xrPlainInfo{typ: xrTypeFrameWaitInfo}
	if err := xok(xcall("xrWaitFrame", xs.session, uintptr(unsafe.Pointer(wi)), uintptr(unsafe.Pointer(xs.frameState))), "xrWaitFrame"); err != nil {
		return fi, err
	}
	bi := &xrPlainInfo{typ: xrTypeFrameBeginInfo}
	if err := xok(xcall("xrBeginFrame", xs.session, uintptr(unsafe.Pointer(bi))), "xrBeginFrame"); err != nil {
		return fi, err
	}
	fi.time = xs.frameState.displayTime
	fi.shouldRender = xs.frameState.shouldRender != 0
	fi.head, fi.headValid = xrLocate(xs.viewSpace, fi.time)
	return fi, nil
}

func xrRenderAndEnd(fi frameInfo, eyes [2]*eyeTarget, draw func(eye int, t *eyeTarget, proj mat4, view hmdMatrix34, dst uint32, dw, dh int32)) error {
	layers := uint32(0)
	if fi.shouldRender {
		xs.viewArr[0] = xrView{typ: xrTypeView}
		xs.viewArr[1] = xrView{typ: xrTypeView}
		li := &xrViewLocateInfo{typ: xrTypeViewLocateInfo, viewConfig: xrStereo, time: fi.time, space: xs.stage}
		vs := &xrViewState{typ: xrTypeViewState}
		var cnt uint32
		r := xcall("xrLocateViews", xs.session, uintptr(unsafe.Pointer(li)), uintptr(unsafe.Pointer(vs)), 2, uintptr(unsafe.Pointer(&cnt)), uintptr(unsafe.Pointer(&xs.viewArr[0])))
		if r >= 0 && cnt == 2 && vs.flags&0x3 == 0x3 {
			for i := 0; i < 2; i++ {
				v := xs.viewArr[i]
				var idx uint32
				ai := &xrPlainInfo{typ: xrTypeAcquireInfo}
				if err := xok(xcall("xrAcquireSwapchainImage", xs.swap[i], uintptr(unsafe.Pointer(ai)), uintptr(unsafe.Pointer(&idx))), "acquire image"); err != nil {
					return err
				}
				wi := &xrSwapchainWaitInfo{typ: xrTypeWaitInfo, timeout: xrInfinite}
				w0 := time.Now()
				if err := xok(xcall("xrWaitSwapchainImage", xs.swap[i], uintptr(unsafe.Pointer(wi))), "wait image"); err != nil {
					return err
				}
				xs.waited += time.Since(w0)
				proj := projFromFov(v.fov, 0.05, 60)
				view := inv34(poseTo34(v.pose))
				call(glBindFramebufferP, glFramebuffer, uintptr(xs.dstFBO))
				call(glFramebufferTexture2DP, glFramebuffer, glColorAttachment0, glTexture2D, uintptr(xs.images[i][idx].image), 0)
				draw(i, eyes[i], proj, view, xs.dstFBO, xs.w[i], xs.h[i])
				if i == 0 {
					cw, ch := clientSize()
					xs.mirror(xs.w[i], xs.h[i], cw, ch)
				}
				call(glBindFramebufferP, glFramebuffer, 0)
				ri := &xrPlainInfo{typ: xrTypeReleaseInfo}
				xcall("xrReleaseSwapchainImage", xs.swap[i], uintptr(unsafe.Pointer(ri)))
				xs.projViews[i] = xrProjView{typ: xrTypeProjView, pose: v.pose, fov: v.fov,
					sub: xrSubImage{swapchain: xs.swap[i], w: xs.w[i], h: xs.h[i]}}
			}
			*xs.layer = xrLayerProjection{typ: xrTypeLayerProjection, space: xs.stage, viewCount: 2, views: uintptr(unsafe.Pointer(&xs.projViews[0]))}
			if xs.camera {
				xs.layer.flags = 0x2 | 0x4
			}
			*xs.layerPtr = uintptr(unsafe.Pointer(xs.layer))
			layers = 1
		}
	}
	blend := int32(1)
	if xs.camera {
		blend = 3
	}
	ei := &xrFrameEndInfo{typ: xrTypeFrameEndInfo, displayTime: fi.time, blend: blend, layerCount: layers, layers: uintptr(unsafe.Pointer(xs.layerPtr))}
	w0 := time.Now()
	r := xcall("xrEndFrame", xs.session, uintptr(unsafe.Pointer(ei)))
	xs.waited += time.Since(w0)
	return xok(r, "xrEndFrame")
}

func xrShutdown() {
	if xs.inst != 0 {
		xcall("xrDestroyInstance", xs.inst)
		xs.inst = 0
	}
}
