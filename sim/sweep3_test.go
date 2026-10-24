package sim

import (
	"fmt"
	"os"
	"testing"
)

type roundOutcome struct {
	won, lost, timeout int
	blown, steals      float64
	dur                float64
}

func outcomes(room []V2, n int, tu *Tuning, seeds int) roundOutcome {
	var o roundOutcome
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
		switch r.Phase {
		case Won:
			o.won++
			o.dur += r.Time
		case Lost:
			o.lost++
		default:
			o.timeout++
		}
		o.blown += float64(r.Stats.GensBlown)
		o.steals += float64(r.Stats.Steals)
	}
	o.blown /= float64(seeds)
	o.steals /= float64(seeds)
	if o.won > 0 {
		o.dur /= float64(o.won)
	}
	return o
}

// THREAT=1: how dangerous the wardens are for the person-like bot.
func TestThreatSweep(t *testing.T) {
	if os.Getenv("THREAT") == "" {
		t.Skip()
	}
	seeds := 10
	cases := []struct {
		name string
		room []V2
		n    int
	}{{"13x14/8", rectRoom(13, 14), 8}, {"10x10/6", rectRoom(10, 10), 6}}
	for _, dr := range []float64{0.9, 1.3, 1.8} {
		for _, st := range []float64{0.35, 0.25} {
			for _, pr := range []float64{0.45, 0.55} {
				line := fmt.Sprintf("drain %.1f sting %.2f pr %.2f:", dr, st, pr)
				for _, c := range cases {
					tu := DefaultTuning()
					tu.Drain, tu.Sting, tu.Pressure = dr, st, pr
					o := outcomes(c.room, c.n, tu, seeds)
					line += fmt.Sprintf(" | %s won %d lost %d to %d blown %.1f steals %.1f t %.0f", c.name, o.won, o.lost, o.timeout, o.blown, o.steals, o.dur)
				}
				t.Log(line)
			}
		}
	}
}
