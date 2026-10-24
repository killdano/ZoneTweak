//go:build !windows

package main

import (
	"boundaryvr/sim"
	"math"
	"testing"
)

func BenchmarkCrowd(b *testing.B) {
	tt := newTester(&testing.T{}, true)
	r := tt.g.r
	r.Robots = nil
	for i := 0; i < 24; i++ {
		k := sim.Walker
		if i%4 == 0 {
			k = sim.Heavy
		}
		rb := r.Spawn(k, i%4, "")
		a := float64(i) / 24 * 6.283
		d := 2.2 + float64(i%3)*1.5
		rb.Pos = sim.V(d*cosF(a), d*sinF(a))
		rb.State, rb.Path, rb.Wait = sim.RWait, nil, 100
	}
	tt.g.bots = map[int]*robot{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetBatches()
		tt.g.drawRobots()
	}
	b.ReportMetric(float64(len(bEnv.v)/vstride), "verts")
}

func cosF(a float64) float64 { return math.Cos(a) }
func sinF(a float64) float64 { return math.Sin(a) }
