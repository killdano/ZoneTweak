//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	psProcs         = map[string]uintptr{}
	psReady         bool
	pLoadLibraryExW = kernel32.NewProc("LoadLibraryExW")
	pGetProcAddress = kernel32.NewProc("GetProcAddress")
)

func pimaxInstallDir() string {
	for _, base := range []string{os.Getenv("ProgramW6432"), os.Getenv("ProgramFiles"), `C:\Program Files`} {
		if base == "" {
			continue
		}
		d := filepath.Join(base, "Pimax")
		if _, err := os.Stat(filepath.Join(d, "Runtime", "PiPlayService.dll")); err == nil {
			return d
		}
	}
	return `C:\Program Files\Pimax`
}

func piServiceInit() bool {
	if psReady {
		return true
	}
	path := filepath.Join(pimaxInstallDir(), "Runtime", "PiPlayService.dll")
	h, _, err := pLoadLibraryExW.Call(uintptr(unsafe.Pointer(wstr(path))), 0, 0x8)
	if h == 0 {
		log.Printf("PiPlayService.dll not loaded: %v", err)
		return false
	}
	for _, n := range []string{"PiServiceInit", "PiService_SetIntConfig", "PiService_SetFloatConfig", "PiService_ImportLocalApp", "PiService_RefreshAppList", "PiService_GetAppList", "PiService_RemoveLocalApp"} {
		b := append([]byte(n), 0)
		p, _, _ := pGetProcAddress.Call(h, uintptr(unsafe.Pointer(&b[0])))
		psProcs[n] = p
	}
	if psProcs["PiServiceInit"] == 0 {
		log.Println("PiServiceInit missing")
		return false
	}
	for i := 0; i < 3 && !psReady; i++ {
		r := int32(call(psProcs["PiServiceInit"]))
		log.Printf("PiServiceInit = %d", r)
		psReady = r == 0
		if !psReady {
			time.Sleep(time.Second)
		}
	}
	return psReady
}

func piSetInt(key string, v int) int32 {
	if !psReady || psProcs["PiService_SetIntConfig"] == 0 {
		return -1
	}
	b := append([]byte(key), 0)
	r := int32(call(psProcs["PiService_SetIntConfig"], uintptr(unsafe.Pointer(&b[0])), uintptr(v)))
	log.Printf("PiService_SetIntConfig %s=%d -> %d", key, v, r)
	return r
}

func piSetFloat(key string, v float64) int32 {
	if !psReady || psProcs["PiService_SetFloatConfig"] == 0 {
		return -1
	}
	b := append([]byte(key), 0)
	r := int32(call(psProcs["PiService_SetFloatConfig"], uintptr(unsafe.Pointer(&b[0])), uintptr(math.Float32bits(float32(v)))))
	log.Printf("PiService_SetFloatConfig %s=%.3f -> %d", key, v, r)
	return r
}

func piCallJSON(fn string, v any) int32 {
	if psProcs[fn] == 0 {
		return -1
	}
	j, _ := json.Marshal(v)
	b := append(j, 0)
	return int32(call(psProcs[fn], uintptr(unsafe.Pointer(&b[0]))))
}

func piAppList() []map[string]any {
	fn := psProcs["PiService_GetAppList"]
	if fn == 0 {
		return nil
	}
	get := func(params any, size int) ([]byte, int32) {
		j, _ := json.Marshal(params)
		pb := append(j, 0)
		buf := make([]byte, size)
		r := int32(call(fn, uintptr(unsafe.Pointer(&pb[0])), uintptr(unsafe.Pointer(&buf[0])), uintptr(size)))
		if i := bytes.IndexByte(buf, 0); i >= 0 {
			buf = buf[:i]
		}
		return buf, r
	}
	pre, r := get(map[string]int{"page": 0, "pageSize": 0}, 1024)
	if r != 0 {
		log.Printf("GetAppList probe %d", r)
		return nil
	}
	var info struct {
		TotalCount int `json:"totalCount"`
		ArrayBytes int `json:"arrayBytes"`
	}
	json.Unmarshal(pre, &info)
	if info.TotalCount == 0 {
		return nil
	}
	body, r := get(map[string]int{"page": 1, "pageSize": info.TotalCount}, info.ArrayBytes+1024)
	if r != 0 {
		log.Printf("GetAppList fetch %d", r)
		return nil
	}
	var out struct {
		List []map[string]any `json:"list"`
	}
	json.Unmarshal(body, &out)
	return out.List
}

func registerWithPimax(dir, exe string) {
	marker := filepath.Join(dir, "spaces", "pimax-library.txt")
	cover := filepath.Join(dir, "cover.png")
	want := "v2|" + exe + "|" + cover
	if b, err := os.ReadFile(marker); err == nil && string(b) == want {
		return
	}
	if !piServiceInit() {
		return
	}
	for _, app := range piAppList() {
		route := fmt.Sprint(app["route"])
		if fp, ok := app["file_path"]; ok && route == "<nil>" {
			route = fmt.Sprint(fp)
		}
		if strings.EqualFold(filepath.Clean(route), filepath.Clean(exe)) || strings.EqualFold(fmt.Sprint(app["name"]), "ZoneTweak") {
			r := piCallJSON("PiService_RemoveLocalApp", map[string]any{"id": app["id"]})
			log.Printf("removed old library entry %v -> %d", app["id"], r)
		}
	}
	icon := ""
	if _, err := os.Stat(cover); err == nil {
		icon = cover
	}
	r := piCallJSON("PiService_ImportLocalApp", map[string]string{"name": "ZoneTweak", "icon": icon, "route": exe, "source": "pimax_import"})
	if psProcs["PiService_RefreshAppList"] != 0 {
		call(psProcs["PiService_RefreshAppList"])
	}
	log.Printf("PiService_ImportLocalApp %s (cover %q) -> %d", exe, icon, r)
	if r == 0 {
		os.MkdirAll(filepath.Dir(marker), 0755)
		os.WriteFile(marker, []byte(want), 0644)
	}
}

func restartPimax() {
	launcher := filepath.Join(pimaxInstallDir(), "Runtime", "launcher.exe")
	for _, svc := range []string{"PiPlayService", "PiPlatformService"} {
		cmd := exec.Command(launcher, svc, "restart")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000008 | 0x00000200}
		if err := cmd.Start(); err != nil {
			log.Printf("restart %s: %v", svc, err)
		} else {
			log.Printf("restarting %s", svc)
		}
	}
}

func askRestart(msg string) {
	r, _, _ := pMessageBoxW.Call(0, uintptr(unsafe.Pointer(wstr(msg+"\n\nRestart Pimax now?"))), uintptr(unsafe.Pointer(wstr("ZoneTweak"))), 0x4|0x20|0x40000)
	if r == 6 {
		restartPimax()
	}
}
