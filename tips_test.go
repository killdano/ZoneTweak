package main

import (
	"strings"
	"testing"
)

func TestTips(t *testing.T) {
	for _, l := range []string{"SAVE", "SAVE & RESTART PIMAX", "SNAP: OFF", "POINTER: LASER", "REALIGN VIEW", "CLOSER -2 CM", "PROXIMITY TEST: ON", "FIT AREA", "DOOR OPEN (12)"} {
		if tipFor(l) == "" {
			t.Fatal("no tip for", l)
		}
	}
	if tipFor("SAVE & RESTART PIMAX") == tipFor("SAVE") {
		t.Fatal("prefix order")
	}
	boundarySys = "SteamVR"
	defer func() { boundarySys = "Pimax" }()
	for _, l := range []string{"SAVE", "SAVE & EXIT", "SAVE AS NEW", "EXIT", "CHAPERONE: ON", "STEAMVR DEFAULT", "CLOSER -2 CM", "DOOR OPEN (12)"} {
		if tip := tipFor(l); tip == "" || strings.Contains(tip, "Pimax") {
			t.Fatalf("steam tip for %s: %q", l, tip)
		}
	}
	if tipFor("SAVE") == tipFor("SAVE AS NEW") {
		t.Fatal("steam prefix order")
	}
	if len(wrapText("one two three four five six seven", 10)) < 3 {
		t.Fatal("wrap")
	}
}
