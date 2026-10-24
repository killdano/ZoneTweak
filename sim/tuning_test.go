package sim

import "testing"

func TestTuningOverride(t *testing.T) {
	tu, err := TuningWith([]byte(`{"pressure": 0.9, "leechBite": 6}`))
	if err != nil {
		t.Fatal(err)
	}
	d := DefaultTuning()
	if tu.Pressure != 0.9 || tu.LeechBite != 6 || tu.Pool != d.Pool || tu.WaveEvery != d.WaveEvery {
		t.Fatalf("partial override wrong: %+v", tu)
	}
	if _, err := TuningWith([]byte(`{"pressure": 0.9,,}`)); err == nil {
		t.Fatal("a broken file was accepted")
	}
}
