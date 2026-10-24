//go:build !windows

package main

// stand-ins for the Windows-only VR, audio and window layers, so the engine and the game
// compile on Linux for tests and the screenshot preview

func haptic(i int, us int)                                    {}
func hapticAmp(i int, ms int, amp float32)                    {}
func audioSet(humVol, humPan, buzz, buzzPan, crackle float64) {}
func audioImpact(strength float64)                            {}
func audioStep(strength, pan float64, kind int)               {}
func audioPlay(kind int, amp, pan, dist float64)              {}
func audioRobots(weld, weldPan, servo, servoPan float64)      {}
func audioClick()                                             {}
func setTitle(s string)                                       {}
func staticChanged(v int)                                     {}

func qualityName() string { return "PREVIEW" }
func cycleQuality()       {}
