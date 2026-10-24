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

// the graphics settings ZoneTweak's menu shows (the renderer is Windows-only)
var (
	qualities          = []struct{ name string }{{"ULTRA"}, {"HIGH"}, {"MEDIUM"}, {"LOW"}, {"POTATO"}}
	qualityIdx, appDir = 2, ""
	wantQuality        = -1
	wantRebuild        bool
	renderScale        = 1.0
	renderScales       = []float64{1, 0.9, 0.8, 0.7}
	vrsOK, foveated    bool
	foveaDeg           = 30.0
	foveaSizes         = []float64{20, 30, 40}
	foveaEdge          int
	foveaLabels        = []string{"HALF", "QUARTER", "SIXTEENTH"}
	perfOpt            = true
)

func saveQuality(dir string, i int) {}
func saveGfx(dir string)            {}
func steamvrReload() error          { return nil }

type gpuStub struct{ cpu float64 }

func (g *gpuStub) lines() []string { return nil }

var gt = &gpuStub{}
