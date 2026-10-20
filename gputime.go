//go:build windows

package main

import (
	"fmt"
	"log"
	"strings"
	"time"
	"unsafe"
)

// GPU pass timers: a timestamp query after each pass, read back a few frames later so the
// CPU never waits on the GPU. Shown on the graphics page and logged every 10 seconds.

const (
	glTimestamp            = 0x8E28
	glQueryResult          = 0x8866
	glQueryResultAvailable = 0x8867
	gpuRing                = 4
)

var (
	glGenQueriesP, glQueryCounterP, glGetQueryObjectivP, glGetQueryObjectui64vP uintptr
)

type gpuSlot struct {
	ids    []uint32
	names  []string
	n      int
	issued bool
}

type gpuTimes struct {
	ok      bool
	ring    [gpuRing]gpuSlot
	cur     int
	avg     map[string]float64
	order   []string
	total   float64
	cpu     float64
	lastLog time.Time
}

var gt = &gpuTimes{avg: map[string]float64{}}

func (g *gpuTimes) init() {
	glGenQueriesP = glProc("glGenQueries")
	glQueryCounterP = glProc("glQueryCounter")
	glGetQueryObjectivP = glProc("glGetQueryObjectiv")
	glGetQueryObjectui64vP = glProc("glGetQueryObjectui64v")
	g.ok = glGenQueriesP != 0 && glQueryCounterP != 0 && glGetQueryObjectivP != 0 && glGetQueryObjectui64vP != 0
	if !g.ok {
		log.Printf("GPU timers unavailable")
	}
}

// frame starts a new set of marks, first collecting the oldest set if the GPU has finished it
func (g *gpuTimes) frame() {
	if !g.ok {
		return
	}
	g.cur = (g.cur + 1) % gpuRing
	s := &g.ring[g.cur]
	if s.issued && s.n > 1 {
		var avail int32
		call(glGetQueryObjectivP, uintptr(s.ids[s.n-1]), glQueryResultAvailable, uintptr(unsafe.Pointer(&avail)))
		if avail != 0 {
			ts := make([]uint64, s.n)
			for i := 0; i < s.n; i++ {
				call(glGetQueryObjectui64vP, uintptr(s.ids[i]), glQueryResult, uintptr(unsafe.Pointer(&ts[i])))
			}
			sum := map[string]float64{}
			for i := 1; i < s.n; i++ {
				if ts[i] >= ts[i-1] {
					sum[s.names[i]] += float64(ts[i]-ts[i-1]) / 1e6
				}
			}
			for _, nm := range g.order {
				g.avg[nm] += (sum[nm] - g.avg[nm]) * 0.05
			}
			if ts[s.n-1] >= ts[0] {
				g.total += (float64(ts[s.n-1]-ts[0])/1e6 - g.total) * 0.05
			}
		}
	}
	s.n, s.issued = 0, false
	g.mark("")
	if time.Since(g.lastLog) > 10*time.Second {
		g.lastLog = time.Now()
		log.Printf("perf: %s", strings.Join(g.lines(), " | "))
	}
}

// mark ends the pass called name (times for the same name add up across both eyes)
func (g *gpuTimes) mark(name string) {
	if !g.ok {
		return
	}
	s := &g.ring[g.cur]
	if s.n == len(s.ids) {
		var id uint32
		call(glGenQueriesP, 1, uintptr(unsafe.Pointer(&id)))
		s.ids = append(s.ids, id)
		s.names = append(s.names, "")
	}
	call(glQueryCounterP, uintptr(s.ids[s.n]), glTimestamp)
	s.names[s.n] = name
	s.n++
	s.issued = true
	if name != "" {
		for _, o := range g.order {
			if o == name {
				return
			}
		}
		g.order = append(g.order, name)
	}
}

func (g *gpuTimes) lines() []string {
	if !g.ok {
		return []string{"GPU TIMERS NOT SUPPORTED BY THIS DRIVER", fmt.Sprintf("CPU %.1f MS", g.cpu)}
	}
	a := fmt.Sprintf("GPU %.1f MS   CPU %.1f MS", g.total, g.cpu)
	var parts []string
	for _, nm := range g.order {
		parts = append(parts, fmt.Sprintf("%s %.1f", nm, g.avg[nm]))
	}
	b, c := parts, []string(nil)
	if len(parts) > 4 {
		b, c = parts[:4], parts[4:]
	}
	return []string{a, strings.Join(b, "  "), strings.Join(c, "  ")}
}
