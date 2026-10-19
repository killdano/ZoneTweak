//go:build windows

package main

import (
	"path/filepath"
	"syscall"
	"unsafe"
)

func steamConfigDirs() []string {
	dirs := []string{}
	var k syscall.Handle
	if syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, wstr(`Software\Valve\Steam`), 0, syscall.KEY_READ, &k) == nil {
		defer syscall.RegCloseKey(k)
		buf := make([]uint16, 1024)
		n := uint32(len(buf) * 2)
		var typ uint32
		if syscall.RegQueryValueEx(k, wstr("SteamPath"), nil, &typ, (*byte)(unsafe.Pointer(&buf[0])), &n) == nil {
			if p := syscall.UTF16ToString(buf); p != "" {
				dirs = append(dirs, filepath.Join(filepath.FromSlash(p), "config"))
			}
		}
	}
	return append(dirs, `C:\Program Files (x86)\Steam\config`)
}
