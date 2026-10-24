//go:build windows

package main

import (
	"math"
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	opengl32 = syscall.NewLazyDLL("opengl32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	pRegisterClassExW  = user32.NewProc("RegisterClassExW")
	pCreateWindowExW   = user32.NewProc("CreateWindowExW")
	pDefWindowProcW    = user32.NewProc("DefWindowProcW")
	pPeekMessageW      = user32.NewProc("PeekMessageW")
	pTranslateMessage  = user32.NewProc("TranslateMessage")
	pDispatchMessageW  = user32.NewProc("DispatchMessageW")
	pGetDC             = user32.NewProc("GetDC")
	pLoadCursorW       = user32.NewProc("LoadCursorW")
	pMessageBoxW       = user32.NewProc("MessageBoxW")
	pPostQuitMessage   = user32.NewProc("PostQuitMessage")
	pGetClientRect     = user32.NewProc("GetClientRect")
	pSetWindowTextW    = user32.NewProc("SetWindowTextW")
	pSystemParamsInfoW = user32.NewProc("SystemParametersInfoW")
	pChoosePixelFormat = gdi32.NewProc("ChoosePixelFormat")
	pSetPixelFormat    = gdi32.NewProc("SetPixelFormat")
	pSwapBuffers       = gdi32.NewProc("SwapBuffers")
	pwglCreateContext  = opengl32.NewProc("wglCreateContext")
	pwglMakeCurrent    = opengl32.NewProc("wglMakeCurrent")
	pwglGetProcAddress = opengl32.NewProc("wglGetProcAddress")
	pGetModuleHandleW  = kernel32.NewProc("GetModuleHandleW")
	pGetWindowLongPtrW = user32.NewProc("GetWindowLongPtrW")
	pSetWindowLongPtrW = user32.NewProc("SetWindowLongPtrW")
	pGetWindowRect     = user32.NewProc("GetWindowRect")
	pSetWindowPos      = user32.NewProc("SetWindowPos")
	pMonitorFromWindow = user32.NewProc("MonitorFromWindow")
	pGetMonitorInfoW   = user32.NewProc("GetMonitorInfoW")
)

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type winMsg struct {
	hwnd     uintptr
	message  uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	ptX, ptY int32
	lPrivate uint32
}

type pixelFormatDescriptor struct {
	nSize, nVersion                                    uint16
	dwFlags                                            uint32
	iPixelType, cColorBits, cRedBits, cRedShift        byte
	cGreenBits, cGreenShift, cBlueBits, cBlueShift     byte
	cAlphaBits, cAlphaShift, cAccumBits, cAccumRedBits byte
	cAccumGreenBits, cAccumBlueBits, cAccumAlphaBits   byte
	cDepthBits, cStencilBits, cAuxBuffers, iLayerType  byte
	bReserved                                          byte
	dwLayerMask, dwVisibleMask, dwDamageMask           uint32
}

type rect struct{ l, t, r, b int32 }

var (
	hwnd        uintptr
	hdc         uintptr
	windowQuit  bool
	gMsg        = new(winMsg)
	gClientRect = new(rect)
	gWndClass   = new(wndClassEx)
	gPFD        = new(pixelFormatDescriptor)
)

func f32(f float32) uintptr { return uintptr(math.Float32bits(f)) }

//go:uintptrescapes
func call(p uintptr, a ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(p, a...)
	return r
}

func wstr(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }

func messageBox(title, text string) {
	pMessageBoxW.Call(0, uintptr(unsafe.Pointer(wstr(text))), uintptr(unsafe.Pointer(wstr(title))), 0x10)
}

func info(msg string) {
	pMessageBoxW.Call(0, uintptr(unsafe.Pointer(wstr(msg))), uintptr(unsafe.Pointer(wstr("ZoneTweak"))), 0x40|0x40000)
}

// Alt+Enter: borderless fullscreen on the window's monitor, and back.
var (
	winFull    bool
	savedStyle uintptr
	savedRect  rect
)

type monitorInfo struct {
	cbSize    uint32
	rcMonitor rect
	rcWork    rect
	dwFlags   uint32
}

func toggleFullscreen() {
	const gwlStyle = ^uintptr(15)        // GWL_STYLE (-16)
	const swp = 0x0020 | 0x0200 | 0x0040 // SWP_FRAMECHANGED | SWP_NOOWNERZORDER | SWP_SHOWWINDOW
	if !winFull {
		savedStyle, _, _ = pGetWindowLongPtrW.Call(hwnd, gwlStyle)
		pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&savedRect)))
		mon, _, _ := pMonitorFromWindow.Call(hwnd, 2) // MONITOR_DEFAULTTONEAREST
		mi := monitorInfo{cbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
		if ok, _, _ := pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); ok == 0 {
			return
		}
		style := savedStyle&^0x00CF0000 | 0x80000000 | 0x10000000 // no frame: WS_POPUP | WS_VISIBLE
		pSetWindowLongPtrW.Call(hwnd, gwlStyle, style)
		m := mi.rcMonitor
		pSetWindowPos.Call(hwnd, 0, uintptr(uint32(m.l)), uintptr(uint32(m.t)), uintptr(uint32(m.r-m.l)), uintptr(uint32(m.b-m.t)), swp)
	} else {
		pSetWindowLongPtrW.Call(hwnd, gwlStyle, savedStyle)
		r := savedRect
		pSetWindowPos.Call(hwnd, 0, uintptr(uint32(r.l)), uintptr(uint32(r.t)), uintptr(uint32(r.r-r.l)), uintptr(uint32(r.b-r.t)), swp)
	}
	winFull = !winFull
}

func wndProc(h uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case 0x0104: // WM_SYSKEYDOWN: Alt+Enter toggles borderless fullscreen
		if wp == 0x0D && lp&(1<<29) != 0 {
			if lp&(1<<30) == 0 { // not a key repeat
				toggleFullscreen()
			}
			return 0
		}
	case 0x0106: // WM_SYSCHAR: no beep for Alt+Enter
		if wp == 0x0D {
			return 0
		}
	case 0x0010:
		windowQuit = true
		return 0
	case 0x0002:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(h, uintptr(msg), wp, lp)
	return r
}

func createWindow(title string, w, h int32) error {
	hinst, _, _ := pGetModuleHandleW.Call(0)
	cursor, _, _ := pLoadCursorW.Call(0, 32512)
	cls := wstr("Contraband")
	*gWndClass = wndClassEx{
		cbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		style:         0x0020,
		lpfnWndProc:   syscall.NewCallback(wndProc),
		hInstance:     hinst,
		hCursor:       cursor,
		lpszClassName: cls,
	}
	if r, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(gWndClass))); r == 0 {
		return err
	}
	// open on the primary monitor, centred in its work area (CW_USEDEFAULT would follow
	// whichever monitor the launching Explorer window was on)
	x, y := uintptr(0x80000000), uintptr(0x80000000)
	var wa struct{ left, top, right, bottom int32 }
	if ok, _, _ := pSystemParamsInfoW.Call(0x0030, 0, uintptr(unsafe.Pointer(&wa)), 0); ok != 0 && wa.right > wa.left {
		cx := wa.left + (wa.right-wa.left-w)/2
		cy := wa.top + (wa.bottom-wa.top-h)/2
		if cx < wa.left {
			cx = wa.left
		}
		if cy < wa.top {
			cy = wa.top
		}
		x, y = uintptr(uint32(cx)), uintptr(uint32(cy))
	}
	r, _, err := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(wstr(title))),
		0x10CF0000, x, y, uintptr(w), uintptr(h), 0, 0, hinst, 0)
	if r == 0 {
		return err
	}
	hwnd = r
	hdc, _, _ = pGetDC.Call(hwnd)
	*gPFD = pixelFormatDescriptor{
		nSize: uint16(unsafe.Sizeof(pixelFormatDescriptor{})), nVersion: 1,
		dwFlags: 0x4 | 0x20 | 0x1, iPixelType: 0, cColorBits: 32, cAlphaBits: 8, cDepthBits: 24, cStencilBits: 8,
	}
	pf, _, _ := pChoosePixelFormat.Call(hdc, uintptr(unsafe.Pointer(gPFD)))
	if pf == 0 {
		return syscall.Errno(0)
	}
	pSetPixelFormat.Call(hdc, pf, uintptr(unsafe.Pointer(gPFD)))
	ctx, _, err := pwglCreateContext.Call(hdc)
	if ctx == 0 {
		return err
	}
	pwglMakeCurrent.Call(hdc, ctx)
	return nil
}

func setTitle(s string) { pSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(wstr(s)))) }

func pumpMessages() {
	for {
		r, _, _ := pPeekMessageW.Call(uintptr(unsafe.Pointer(gMsg)), 0, 0, 0, 1)
		if r == 0 {
			return
		}
		if gMsg.message == 0x0012 {
			windowQuit = true
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(gMsg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(gMsg)))
	}
}

func clientSize() (int32, int32) {
	pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(gClientRect)))
	return gClientRect.r - gClientRect.l, gClientRect.b - gClientRect.t
}

func swapBuffers() { pSwapBuffers.Call(hdc) }

func glProc(name string) uintptr {
	b := append([]byte(name), 0)
	r, _, _ := pwglGetProcAddress.Call(uintptr(unsafe.Pointer(&b[0])))
	if r == 0 || r == 1 || r == 2 || r == 3 || r == ^uintptr(0) {
		return 0
	}
	return r
}

// keepAwake: tell Windows someone's using the PC (you are, in the headset) so it doesn't
// blank the monitors, start the screen saver or go to sleep while the app runs. Called every
// half minute: the execution state (ES_CONTINUOUS | ES_SYSTEM_REQUIRED | ES_DISPLAY_REQUIRED)
// keeps the display and the PC on, and a mouse "move" of nothing resets the idle clock the
// screen saver and the lock screen go by (the pointer doesn't move).
var (
	pSetThreadExecutionState = kernel32.NewProc("SetThreadExecutionState")
	pMouseEvent              = user32.NewProc("mouse_event")
)

func keepAwake() {
	pSetThreadExecutionState.Call(0x80000000 | 0x00000001 | 0x00000002)
	pMouseEvent.Call(0x0001, 0, 0, 0, 0) // MOUSEEVENTF_MOVE by 0, 0
}
