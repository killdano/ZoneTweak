//go:build !windows

package main

import (
	"boundaryvr/sim"
	"os"
	"testing"
)

// DANRATE=1: the person-like simulated player's win rate in the captured real room, at each
// difficulty and generator count.
func TestDanRoomRates(t *testing.T) {
	if os.Getenv("DANRATE") == "" {
		t.Skip()
	}
	os.Chdir("preview")
	os.Setenv("ROOM", "chap")
	poly := previewRoom()
	os.Chdir("..")
	// DANTUNE='{"pressure":0.5}': try other numbers
	if v := os.Getenv("DANTUNE"); v != "" {
		tu, err := sim.TuningWith([]byte(v))
		if err != nil {
			t.Fatal(err)
		}
		gameTuning = tu
		defer func() { gameTuning = nil }()
	}
	var R []sim.V2
	for _, p := range poly {
		R = append(R, sv(p))
	}
	for dif := 0; dif < 3; dif++ {
		difficulty = dif
		line := difficultyNames[dif] + ":"
		lo, hi := sim.GenRange(R, tuningFor())
		for n := lo; n <= hi; n++ {
			w := 0
			const seeds = 16
			for s := 1; s <= seeds; s++ {
				r := sim.NewRound(R, int64(s*1000+n), tuningFor(), nil)
				r.SetGens(n)
				b := sim.NewBot(r, int64(s*31+n))
				b.BuildMaze(0.8)
				r.Start()
				for r.Phase == sim.Playing && r.Time < 900 {
					r.Step(0.05)
					b.Step(0.05)
				}
				if r.Phase == sim.Won {
					w++
				}
			}
			line += " " + itoa(n) + " gens " + itoa(w*100/seeds) + "%"
		}
		t.Log(line)
	}
	difficulty = 1
}
