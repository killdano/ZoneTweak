package main

import (
	"math"
	"math/rand"
	"testing"
)

func TestOneEuroSmoothsJitter(t *testing.T) {
	f := newPointFilter()
	r := rand.New(rand.NewSource(1))
	var rawVar, outVar float64
	for i := 0; i < 900; i++ {
		n := pt2{r.NormFloat64() * 0.02, r.NormFloat64() * 0.02}
		o := f.step(n, 1.0/90)
		if i > 90 {
			rawVar += n[0] * n[0]
			outVar += o[0] * o[0]
		}
	}
	if outVar > rawVar/8 {
		t.Fatalf("not smooth enough: raw %.5f out %.5f", rawVar, outVar)
	}
	f.reset()
	var last pt2
	for i := 0; i < 45; i++ {
		last = f.step(pt2{float64(i) * 0.05, 0}, 1.0/90)
	}
	if lag := 44*0.05 - last[0]; lag > 0.25 || math.IsNaN(lag) {
		t.Fatalf("too much lag on fast moves: %.3f m", lag)
	}
}
