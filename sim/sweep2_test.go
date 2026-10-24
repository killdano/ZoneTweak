package sim

import (
	"fmt"
	"os"
	"testing"
)

func botRate(room []V2, n int, tu *Tuning, seeds int) int {
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
	return w * 100 / seeds
}

// TestSweep2 (WINSWEEP2=1): small bases with a stronger device field.
func TestSweep2(t *testing.T) {
	if os.Getenv("WINSWEEP2") == "" {
		t.Skip()
	}
	seeds := 12
	rooms := [][2]float64{{7, 7}, {10, 10}, {12, 10}, {14, 12}, {16, 16}}
	for _, ff := range []float64{0, 2, 4} {
		for _, pr := range []float64{0.5, 0.6} {
			line := fmt.Sprintf("fieldFew %.0f pr %.2f:", ff, pr)
			tot, cnt := 0, 0
			for _, rm := range rooms {
				room := rectRoom(rm[0], rm[1])
				tu := DefaultTuning()
				tu.WaveSize, tu.Pressure, tu.FieldFew = -0.5, pr, ff
				lo, hi := GenRange(room, tu)
				cell := ""
				for _, n := range []int{lo, lo + 1, (lo + hi + 1) / 2, hi} {
					if n > hi {
						continue
					}
					w := botRate(room, n, tu, seeds)
					cell += fmt.Sprintf(" %d:%3d", n, w)
					tot += w
					cnt++
				}
				line += fmt.Sprintf(" |%2.0fx%-2.0f%s", rm[0], rm[1], cell)
			}
			t.Logf("%s | avg %d%%", line, tot/cnt)
		}
	}
}
