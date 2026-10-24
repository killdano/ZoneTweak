package main

// The radio voice. Your contact on the outside only talks into the handset: the next wave's
// intel and his warnings wait there until you hold it to your ear. The radio's speaker just
// plays a recorded prompt - an automated voice on a worn tape - to pick up. The lines are
// short recordings (voice/*.raw: 16 kHz mono, already band-passed and roughened like a radio;
// n_*.raw is the speaker's prompt) that the mixer plays from moving sources.

import (
	"boundaryvr/sim"
	"embed"
	"encoding/binary"
	"math"
	"math/rand"
	"strings"
)

//go:embed voice/*.raw
var voiceFS embed.FS

var voiceCache = map[string][]int16{}

const (
	srcHandset = 0
	srcRadio   = 1
)

// voiceClip: one recorded line by id (see voice/lines.py); nil if there's no such line.
func voiceClip(id string) []int16 {
	if c, ok := voiceCache[id]; ok {
		return c
	}
	b, err := voiceFS.ReadFile("voice/" + id + ".raw")
	var c []int16
	if err == nil {
		c = make([]int16, len(b)/2)
		for i := range c {
			c[i] = int16(binary.LittleEndian.Uint16(b[2*i:]))
		}
	}
	voiceCache[id] = c
	return c
}

// intelLines: what the contact says about a wave, as clip ids with the pause after each.
func intelLines(r *sim.Round) []clipItem {
	in := r.Intel
	var out []clipItem
	add := func(id string, gap float64) {
		if c := voiceClip(id); c != nil {
			out = append(out, clipItem{c, gap})
		}
	}
	if in.Wave <= 15 {
		add("w_"+itoa(in.Wave), 0.35)
	} else {
		add("w_next", 0.35)
	}
	for _, gr := range in.Groups {
		kind := "walker"
		switch gr.Kind {
		case sim.Heavy:
			kind = "heavy"
		case sim.Drone:
			kind = "leech"
		}
		n := "many"
		if gr.N <= 12 {
			n = itoa(gr.N)
		}
		add("c_"+kind+"_"+n, 0.04)
		add("d_"+strings.ToLower(r.Doors[gr.Door].Name), 0.3)
	}
	add("out_"+itoa(1+rand.Intn(3)), 0)
	return out
}

// playIntel: the next wave's intel, into the handset.
func playIntel(r *sim.Round) {
	eng.queueClips(srcHandset, intelLines(r), true)
}

var radioLast = map[string]float64{}

// radioMsg: something your contact wants to tell you, waiting on the handset.
type radioMsg struct {
	id      string
	at, ttl float64
	urgent  bool
}

// msgTTL: how long a waiting message is still worth hearing.
func msgTTL(id string) float64 {
	switch {
	case strings.HasPrefix(id, "h_"):
		return 120 // where the screwdriver is: still true later
	case id == "x_armed" || id == "x_start":
		return 60
	case strings.HasPrefix(id, "x_harp") || id == "x_cart_spear":
		return 120
	case id == "x_reel_mode":
		return 40
	case id == "x_leads":
		return 90
	case strings.HasPrefix(id, "x_mf_"):
		return 120
	}
	return 20
}

// stillTrue: whether a waiting line still means anything - no news of a leech you've
// already dealt with, or a generator that's back up.
func (g *game) stillTrue(id string) bool {
	r := g.r
	if r.Phase != sim.Playing {
		return id == "x_won" || id == "x_lost"
	}
	switch id {
	case "x_leech":
		for _, b := range r.Robots {
			if b.Kind == sim.Drone && b.Alive() && b.State == sim.RAttached {
				return true
			}
		}
		return false
	case "x_genlow":
		for _, gn := range r.Gens {
			if gn.State == sim.GenUp && gn.Charge < 0.3*r.T.ChargeMax {
				return true
			}
		}
		return false
	case "x_genblown":
		for _, gn := range r.Gens {
			if gn.State == sim.GenBlown {
				return true
			}
		}
		return false
	case "x_stolen":
		return r.Dev.State == sim.DevStolen
	case "x_exposed":
		return r.Dev.State == sim.DevHome && r.Dev.Exposed
	case "x_ringdown":
		return len(r.RingPoly) < 3
	case "x_recovered":
		return r.Dev.State == sim.DevHome
	case "x_armed":
		return r.Harp.Up && r.Harp.Armed
	case "x_livewire":
		return r.Harp.Kind == sim.ShotAnchor || len(r.Harp.Traps) > 0
	}
	switch id {
	case "x_harp_super", "x_harp_tie", "x_cart_spear":
		return r.Harp.Up
	case "x_tractor":
		return r.Dev.State == sim.DevDropped && r.Harp.Up
	case "x_mf_start", "x_mf_again":
		return r.Dev.Misfire && g.misfire != nil && !g.misfire.raised
	case "x_mf_up", "x_mf_wrong", "x_mf_help":
		return g.misfire != nil && g.misfire.ordersOpen()
	case "x_mf_patch":
		return g.misfire != nil && g.misfire.needsPatch() != nil
	case "x_mf_done":
		return r.Dev.Misfire && g.misfire != nil && g.misfire.allDone()
	case "x_leads":
		return r.Dev.State == sim.DevHome && r.Dev.Leads < sim.LeadsNeeded
	case "x_reel_mode":
		return r.Harp.Up && (r.Harp.Reels > 0 || r.Harp.ReelShot)
	}
	if strings.HasPrefix(id, "h_") {
		// where the screwdriver or the wrench is: only while there's still something on the
		// cart that needs it and it isn't in your hand
		need := g.toolNeeded()
		switch id {
		case "h_need", "h_d_need":
			return need == fastScrew && (g.driver == nil || g.handHolding(g.driver.p) == nil)
		case "h_wneed", "h_w_need":
			return need == fastBolt && (g.wrench == nil || g.handHolding(g.wrench.p) == nil)
		}
		switch need {
		case fastScrew:
			return g.driver == nil || g.handHolding(g.driver.p) == nil
		case fastBolt:
			return g.wrench == nil || g.handHolding(g.wrench.p) == nil
		}
		return false
	}
	return true
}

// msgClips: what a message says when you hear it - its recording, or for "you need the
// screwdriver/wrench", where it's lying right now (that first: it's what matters).
func msgClips(id string) []clipItem {
	if g := G; g != nil && (id == "h_d_need" || id == "h_w_need") {
		tool, p := "h_d_", (*prop)(nil)
		if id == "h_d_need" && g.driver != nil {
			p = g.driver.p
		}
		if id == "h_w_need" && g.wrench != nil {
			tool, p = "h_w_", g.wrench.p
		}
		if p != nil {
			if c := voiceClip(tool + g.whereIs(p)); c != nil {
				return []clipItem{{c, 0.3}}
			}
		}
	}
	if g := G; g != nil && id == "x_leech" {
		// and where the prod is
		out := []clipItem{{voiceClip(id), 0.3}}
		if c := voiceClip(g.poleWhere()); c != nil {
			out = append(out, clipItem{c, 0.3})
		}
		return out
	}
	if g := G; g != nil && id == "x_mf_help" && g.misfire != nil {
		return append([]clipItem{{voiceClip(id), 0.3}}, g.misfire.detailClips()...)
	}
	return []clipItem{{voiceClip(id), 0.3}}
}

// urgentNag: lines the speaker's prompt calls urgent
var urgentNag = map[string]bool{"x_stolen": true, "x_ringdown": true, "x_leech": true, "x_genlow": true, "x_genblown": true, "x_exposed": true}

// radioSay: your contact has something to tell you. He only talks into the handset: with it
// at your ear the line plays straight away (urgent cuts in), otherwise it waits there and the
// radio's speaker plays its recorded prompt to pick up.
func radioSay(id string, urgent bool) {
	g := G
	if g == nil || g.radio == nil || voiceClip(id) == nil {
		return
	}
	rd := g.radio
	if !g.stillTrue(id) {
		return
	}
	if rd.listening {
		if t, ok := radioLast[id]; ok && g.t-t < 12 && !urgent {
			return
		}
		radioLast[id] = g.t
		eng.queueClips(srcHandset, msgClips(id), urgent)
		return
	}
	for i := range rd.inbox {
		if rd.inbox[i].id == id { // one of each line: the newer one replaces it
			rd.inbox = append(rd.inbox[:i], rd.inbox[i+1:]...)
			break
		}
	}
	rd.inbox = append(rd.inbox, radioMsg{id: id, at: g.t, ttl: msgTTL(id), urgent: urgentNag[id]})
	if len(rd.inbox) > 6 {
		rd.inbox = rd.inbox[len(rd.inbox)-6:]
	}
	rd.nagLeft = 2
	rd.nagAt = math.Min(rd.nagAt, g.t+0.4)
}

// waiting: the messages still worth hearing (stale ones are dropped).
func (rd *radio) waiting(now float64) []radioMsg {
	keep := rd.inbox[:0]
	for _, m := range rd.inbox {
		if now-m.at < m.ttl && (G == nil || G.stillTrue(m.id)) {
			keep = append(keep, m)
		}
	}
	rd.inbox = keep
	return keep
}

// playWaiting: at your ear, everything waiting - urgent first, then in order.
func (rd *radio) playWaiting(now float64) {
	var items []clipItem
	for _, urgent := range []bool{true, false} {
		for _, m := range rd.waiting(now) {
			if m.urgent == urgent {
				items = append(items, msgClips(m.id)...)
				radioLast[m.id] = now
			}
		}
	}
	rd.inbox = rd.inbox[:0]
	if len(items) > 0 {
		eng.queueClips(srcHandset, items, true)
	}
}

// nag: the speaker's recorded prompt (a worn tape: you can just make it out).
func (rd *radio) nag(urgent bool) {
	id := "n_pickup_" + itoa(1+rd.nags%2)
	if urgent {
		id = "n_urgent"
	}
	rd.nags++
	if c := voiceClip(id); c != nil {
		eng.queueClips(srcRadio, []clipItem{{c, 0.2}}, true)
	}
}

// radioAudio: per frame, where the handset and the speaker are as you hear them.
func (g *game) radioAudio() {
	rd := g.radio
	if rd == nil {
		return
	}
	head := ed.hmd.pos()
	// the handset: at your ear it's right there, clear; anywhere else it's a tinny little
	// speaker you can barely make out
	if rd.listening {
		side := rd.set.pos.sub(head).dot(ed.hmd.axis(0))
		eng.clipSource(srcHandset, 1.0, math.Copysign(0.85, side), 0)
	} else {
		a, pan := ed.panAt(rd.set.pos)
		eng.clipSource(srcHandset, 0.22*a, pan, 0.05)
	}
	sp := fv(rd.pos, 1.0)
	a, pan := ed.panAt(sp)
	eng.clipSource(srcRadio, 0.8*math.Max(a, 0.35), pan, 0.3)
}
