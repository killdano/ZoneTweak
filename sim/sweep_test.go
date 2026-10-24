package sim

import (
	"fmt"
	"os"
	"testing"
)

// TestSweep (WINSWEEP=1): the human-like bot's win rate across rooms for a grid of wave
// size exponents and pressures.
func TestSweep(t *testing.T) {
	if os.Getenv("WINSWEEP") == "" {
		t.Skip("set WINSWEEP=1")
	}
	seeds := 8
	rooms := [][2]float64{{7, 7}, {10, 10}, {14, 12}, {16, 16}}
	for _, ws := range []float64{-0.2, -0.5, -0.8} {
		for _, pr := range []float64{0.5, 0.7, 0.9} {
			line := fmt.Sprintf("ws %.1f pr %.1f:", ws, pr)
			tot, cnt := 0, 0
			for _, rm := range rooms {
				room := rectRoom(rm[0], rm[1])
				tu := DefaultTuning()
				tu.WaveSize, tu.Pressure = ws, pr
				lo, hi := GenRange(room, tu)
				cell := ""
				for _, n := range []int{lo, (lo + hi) / 2, hi} {
					w := 0
					for s := 1; s <= seeds; s++ {
						r := NewRound(room, int64(s*1000+n), tu, nil)
						r.SetGens(n)
						b := NewBot(r, int64(s*31+n))
						b.BuildMaze(0.8)
						r.Start()
						for r.Phase == Playing && r.Time < 900 {
							r.Step(0.05)
							b.Step(0.05)
						}
						if r.Phase == Won {
							w++
						}
					}
					cell += fmt.Sprintf("%3d", w*100/seeds)
					tot += w
					cnt += seeds
				}
				line += fmt.Sprintf(" | %2.0fx%-2.0f%s", rm[0], rm[1], cell)
			}
			t.Logf("%s | all %d%%", line, tot*100/cnt)
		}
	}
}
