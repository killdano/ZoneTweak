package main

import "testing"

func TestSlideEdge(t *testing.T) {
	old, nw := 3.0, 4.5
	// at the start the moved wall sits where it was, at the end where it now is
	if v := slideEdge(nw, old, nw, 1, 0); v != old {
		t.Fatal("wall face not at old place", v)
	}
	if v := slideEdge(nw-wallDepth, old, nw, 1, 0); v != old-wallDepth {
		t.Fatal("wall fittings", v)
	}
	if v := slideEdge(1.0, old, nw, 1, 0.3); v != 1.0 {
		t.Fatal("room interior moved", v)
	}
	for _, k := range []float64{0, 0.4, 1} {
		a := slideEdge(nw-wallDepth-1e-9, old, nw, 1, k)
		b := slideEdge(nw-wallDepth+1e-9, old, nw, 1, k)
		if d := a - b; d > 1e-6 || d < -1e-6 {
			t.Fatal("tear at the wall's inner edge", k, a, b)
		}
		if v := slideEdge(nw+0.3, old, nw, 1, 1); v != nw+0.3 {
			t.Fatal("end state not identity")
		}
	}
	// min-side wall mirrors it
	if v := slideEdge(-nw, -old, -nw, -1, 0); v != -old {
		t.Fatal("min side", v)
	}
}
