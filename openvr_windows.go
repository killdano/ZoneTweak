//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

// SteamVR's chaperone is changed through Valve's openvr_api.dll. Each change runs in a short-lived
// copy of ZoneTweak (--steamvr-...), never in the VR process itself: under SteamVR the OpenXR
// runtime shares vrclient with OpenVR, and connecting and disconnecting a second client there
// could take the running session down with it.

const (
	ovrAppBackground = 3
	ovrChapLive      = 1

	// IVRChaperoneSetup_006 function table
	ovrSetupRevert = 1
	ovrSetupLive   = 5
	ovrSetupReload = 13

	// IVRSettings_003 function table
	ovrSetInt32  = 2
	ovrSetFloat  = 3
	ovrRemoveKey = 10

	exitOK         = 0
	exitNoDLL      = 10
	exitNoSteamVR  = 11
	exitNoIface    = 12
	exitSettingErr = 13
	exitBadArgs    = 14
)

var errNoSteamVR = errors.New("SteamVR isn't running")

type openVR struct {
	dll      *syscall.LazyDLL
	setup    uintptr
	settings uintptr
}

func (o *openVR) proc(name string) uintptr {
	p := o.dll.NewProc(name)
	if p.Find() != nil {
		return 0
	}
	return p.Addr()
}

func fnAt(table uintptr, i int) uintptr {
	return *(*uintptr)(unsafe.Pointer(table + uintptr(i)*unsafe.Sizeof(uintptr(0))))
}

// open connects as a background app: it never starts SteamVR, it only talks to one that's running
func (o *openVR) open() int {
	exe, _ := os.Executable()
	o.dll = syscall.NewLazyDLL(filepath.Join(filepath.Dir(exe), "openvr_api.dll"))
	if o.dll.Load() != nil {
		return exitNoDLL
	}
	initFn, getFn := o.proc("VR_InitInternal2"), o.proc("VR_GetGenericInterface")
	if initFn == 0 || getFn == 0 {
		return exitNoDLL
	}
	var e int32
	call(initFn, uintptr(unsafe.Pointer(&e)), ovrAppBackground, 0)
	if e != 0 {
		return exitNoSteamVR
	}
	get := func(name string) uintptr {
		b := append([]byte(name), 0)
		var ge int32
		r := call(getFn, uintptr(unsafe.Pointer(&b[0])), uintptr(unsafe.Pointer(&ge)))
		if ge != 0 {
			return 0
		}
		return r
	}
	o.setup = get("FnTable:IVRChaperoneSetup_006")
	o.settings = get("FnTable:IVRSettings_003")
	if o.setup == 0 || o.settings == 0 {
		o.close()
		return exitNoIface
	}
	return exitOK
}

func (o *openVR) close() {
	if p := o.proc("VR_ShutdownInternal"); p != 0 {
		call(p)
	}
}

// steamvrMain runs the helper modes; ok is false when the arguments aren't a helper call
func steamvrMain(args []string) (code int, ok bool) {
	if len(args) < 2 || (args[1] != "--steamvr-reload" && args[1] != "--steamvr-set" && args[1] != "--steamvr-live") {
		return 0, false
	}
	var o openVR
	if c := o.open(); c != exitOK {
		return c, true
	}
	defer o.close()
	if args[1] == "--steamvr-reload" {
		// tear down SteamVR's chaperone and load it again from the file just written,
		// then bring the working copy in line so nothing stale gets committed later
		call(fnAt(o.setup, ovrSetupReload), ovrChapLive)
		call(fnAt(o.setup, ovrSetupRevert))
		return exitOK, true
	}
	if args[1] == "--steamvr-live" {
		// the live collision bounds, one floor corner per wall, as JSON on stdout
		var n uint32
		call(fnAt(o.setup, ovrSetupLive), 0, uintptr(unsafe.Pointer(&n)))
		pts := [][2]float64{}
		if n > 0 && n < 4096 {
			q := make([][4][3]float32, n)
			call(fnAt(o.setup, ovrSetupLive), uintptr(unsafe.Pointer(&q[0])), uintptr(unsafe.Pointer(&n)))
			for _, c := range q[:min(int(n), len(q))] {
				pts = append(pts, [2]float64{float64(c[0][0]), float64(c[0][2])})
			}
		}
		b, _ := json.Marshal(pts)
		os.Stdout.Write(b)
		return exitOK, true
	}
	if len(args) < 5 {
		return exitBadArgs, true
	}
	key, kind := args[2], args[3]
	v, err := strconv.ParseFloat(args[4], 64)
	if err != nil {
		return exitBadArgs, true
	}
	sec, k := append([]byte(svrSection), 0), append([]byte(key), 0)
	var se int32
	ps, pk, pe := uintptr(unsafe.Pointer(&sec[0])), uintptr(unsafe.Pointer(&k[0])), uintptr(unsafe.Pointer(&se))
	switch kind {
	case "float":
		call(fnAt(o.settings, ovrSetFloat), ps, pk, uintptr(math.Float32bits(float32(v))), pe)
	case "int":
		call(fnAt(o.settings, ovrSetInt32), ps, pk, uintptr(uint32(int32(v))), pe)
	case "remove":
		call(fnAt(o.settings, ovrRemoveKey), ps, pk, pe)
	default:
		return exitBadArgs, true
	}
	if se != 0 {
		return exitSettingErr, true
	}
	return exitOK, true
}

// steamvrRun runs one helper call in a separate ZoneTweak process and waits for it
func steamvrRun(args ...string) error {
	_, err := steamvrOutput(args...)
	return err
}

func steamvrOutput(args ...string) ([]byte, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	cmd := exec.Command(exe, args...)
	cmd.Stdout = &out
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
		return nil, errors.New("SteamVR didn't answer")
	}
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		return nil, err
	}
	switch code {
	case exitOK:
		return out.Bytes(), nil
	case exitNoDLL:
		return nil, errors.New("openvr_api.dll is missing next to ZoneTweak.exe")
	case exitNoSteamVR:
		return nil, errNoSteamVR
	case exitNoIface:
		return nil, errors.New("this SteamVR doesn't offer the chaperone interface")
	case exitSettingErr:
		return nil, errors.New("SteamVR refused the setting")
	}
	return nil, fmt.Errorf("SteamVR helper failed (%d)", code)
}

// steamvrLive: the outline SteamVR is showing right now
func steamvrLive() ([]pt2, error) {
	b, err := steamvrOutput("--steamvr-live")
	if err != nil {
		return nil, err
	}
	var raw [][2]float64
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := make([]pt2, len(raw))
	for i, p := range raw {
		out[i] = pt2{p[0], p[1]}
	}
	return out, nil
}

func steamvrReload() error { return steamvrRun("--steamvr-reload") }

func steamvrSet(key, kind string, v float64) error {
	err := steamvrRun("--steamvr-set", key, kind, strconv.FormatFloat(v, 'f', 4, 64))
	log.Printf("steamvr set %s %s %.4f: %v", key, kind, v, err)
	return err
}
