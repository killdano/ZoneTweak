package main

import "testing"

func TestTips(t *testing.T) {
	for _, l := range []string{"SAVE", "SAVE & RESTART PIMAX", "SNAP: OFF", "POINTER: LASER", "REALIGN VIEW", "CLOSER -2 CM", "PROXIMITY TEST: ON", "FIT AREA", "DOOR OPEN (12)"} {
		if tipFor(l) == "" {
			t.Fatal("no tip for", l)
		}
	}
	if tipFor("SAVE & RESTART PIMAX") == tipFor("SAVE") {
		t.Fatal("prefix order")
	}
	if len(wrapText("one two three four five six seven", 10)) < 3 {
		t.Fatal("wrap")
	}
}
