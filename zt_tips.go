package main

import "strings"

var buttonTips = []struct{ prefix, tip string }{
	{"PLAY CONTRABAND", "The bonus game: defend the room you just shaped (saved or not). The game's pause menu brings you back here, your edits as you left them."},
	{"SAVE & RESTART", "Saves everything, closes ZoneTweak and restarts the Pimax service so your changes load right away."},
	{"SAVE AS NEW", "Name and store the outline you see now as a saved space, like DOOR OPEN or DOOR CLOSED."},
	{"SAVED SPACES", "Store named boundaries (door open, door closed...) and switch between them with one press."},
	{"SAVE", "Writes the boundary, play center and play area to Pimax's files. Pimax shows them after a restart."},
	{"REVERT", "Throws away unsaved edits and goes back to the last saved boundary."},
	{"UNDO", "Steps back one edit."},
	{"REDO", "Re-applies the edit you just undid."},
	{"SQUARE TO ROOM", "Turns the play area so its edges run parallel to your walls."},
	{"SQUARE UP", "Turns the traced outline into straight walls at right angles, lined up with your room."},
	{"SIMPLIFY", "Removes extra points that sit on an almost straight line (within 8 cm). The shape stays the same."},
	{"NEW OUTLINE", "Start over: walk to each corner of the room and pull the trigger. Pull it near the first corner to finish."},
	{"CANCEL OUTLINE", "Stops drawing the new outline and keeps the old one."},
	{"SNAP", "ON: dragged points pull square to their neighbors. OFF: free movement for fine adjustments."},
	{"ZONE SETTINGS", "How close you can get before Pimax's safety zone appears, a live proximity test, and the zone on/off switch."},
	{"FIX FLOOR", "If ZoneTweak's grid isn't on your real floor: rest a controller on the floor during the countdown. View only, nothing is saved."},
	{"GRAPHICS", "Quality presets, render scale and foveated rendering, with live GPU and CPU timings."},
	{"RENDER SCALE", "Renders fewer pixels and scales up to the headset. Each step down is roughly 20% less GPU work, a little softer."},
	{"FOVEATED", "Shades the outer part of each eye at a lower rate, where the lens blurs it anyway. NVIDIA RTX and GTX 16 GPUs."},
	{"FOVEA SIZE", "How far from the centre of each lens everything stays full quality. Smaller saves more GPU time."},
	{"EDGES", "How coarse shading gets outside that circle: the fraction of pixels actually shaded, first in a 12 degree ring, then beyond it."},
	{"OPTIMIZED RENDERING", "OFF draws the way earlier versions did, for comparing GPU time or if something looks wrong."},
	{"ULTRA", "Best look: 8x anti-aliasing, 4K shadows, full-detail fog, floor reflections."},
	{"HIGH", "4x anti-aliasing and 2K shadows. Try this first if ULTRA stutters or reprojects."},
	{"MEDIUM", "2x anti-aliasing, 1K shadows and lighter fog."},
	{"LOW", "For slower GPUs: no floor reflections, fewer lights, low-res fog."},
	{"POTATO", "Runs on anything: no anti-aliasing, shadows, floor water, surface detail or mist, very few lights and particles."},
	{"POINTER", "FLOOR: beam straight down; grab a generator within arm's reach without touching it. LASER: aim from a distance. Click the thumbstick (or press the Index trackpad) to switch."},
	{"GHOST", "Shows the last saved outline as a grey line while you edit, so you can compare."},
	{"REALIGN VIEW", "If ZoneTweak's walls don't line up with your room (after a Pimax recenter, for example), hold the trigger and drag or twist the outline into place. Stick click resets. Doesn't change what's saved."},
	{"RESTORE ORIGINAL", "Loads Pimax's own boundary from the oldest backup, from before ZoneTweak changed anything. Save to keep it."},
	{"EXIT", "Close ZoneTweak. Asks first if you have unsaved changes, and offers to restart Pimax if you saved."},
	{"CLOSER", "Warning distance: how close you can get to a wall before Pimax shows the safety zone. Smaller = closer."},
	{"FARTHER", "Warning distance: how close you can get to a wall before Pimax shows the safety zone. Bigger = earlier warning."},
	{"PIMAX DEFAULT", "Removes ZoneTweak's warning distance so Pimax goes back to its own built-in value."},
	{"SAFETY ZONE", "Turns Pimax's safety zone on or off everywhere. OFF means no warning at all near walls."},
	{"PROXIMITY TEST", "Shows the warning distance as an orange line inside the walls. Walls glow red when your head or hands cross it. Drag the line to change the distance."},
	{"UPDATE", "Overwrites the active saved space with the outline you see now."},
	{"DELETE", "Deletes the active saved space. Press twice to confirm."},
	{"CONFIRM DELETE", "Press again to delete for good."},
	{"FIT AREA", "Grows the blue play area to the biggest rectangle that fits inside your walls, with a 10 cm margin. Games use this rectangle for room-scale."},
	{"CENTER IN ROOM", "Moves the play center (where games put you) to the middle of the room."},
	{"RESET", "Puts the play center and play area back to what's saved."},
	{"LOCK PLAY AREA", "Locks the play area so its floor handles can't be grabbed by accident. Same as clicking the padlock on the floor."},
}

// under SteamVR: the tips that differ in more than the name
var steamTips = []struct {
	label, tip string
	exact      bool
}{
	{"SAVE & EXIT", "Saves everything and closes ZoneTweak. SteamVR already shows what you saved.", true},
	{"SAVE", "Writes the boundary, play center and play area to SteamVR's chaperone file and reloads it in SteamVR straight away.", true},
	{"EXIT", "Close ZoneTweak. Asks first if you have unsaved changes.", true},
	{"ZONE SETTINGS", "How close you can get before SteamVR's chaperone fades in, a live proximity test, and showing or hiding the chaperone.", false},
	{"STEAMVR DEFAULT", "Removes ZoneTweak's fade distance so SteamVR goes back to its own built-in value.", false},
	{"CHAPERONE", "Shows or hides SteamVR's chaperone everywhere. OFF means no warning at all near walls.", false},
}

func tipFor(label string) string {
	if steamMode() {
		for _, t := range steamTips {
			if label == t.label || (!t.exact && strings.HasPrefix(label, t.label)) {
				return t.tip
			}
		}
	}
	tip := ""
	if strings.HasSuffix(label, ")") && strings.Contains(label, " (") {
		tip = "Loads this saved space and writes it to Pimax. The number is how many corners it has."
	} else {
		for _, t := range buttonTips {
			if strings.HasPrefix(label, t.prefix) {
				tip = t.tip
				break
			}
		}
	}
	if steamMode() {
		tip = strings.ReplaceAll(strings.ReplaceAll(tip, "Pimax's safety zone", "SteamVR's chaperone"), "Pimax", "SteamVR")
	}
	return tip
}

func wrapText(s string, width int) []string {
	var lines []string
	line := ""
	for _, w := range strings.Fields(strings.ToUpper(s)) {
		if line != "" && len(line)+1+len(w) > width {
			lines = append(lines, line)
			line = w
			continue
		}
		if line != "" {
			line += " "
		}
		line += w
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
