package main

import (
	"encoding/binary"
	"math"
	"os"
	"testing"
)

func writeWav(path string, s []float64) {
	f, _ := os.Create(path)
	defer f.Close()
	n := len(s)
	hdr := []any{[4]byte{'R', 'I', 'F', 'F'}, uint32(36 + n*2), [4]byte{'W', 'A', 'V', 'E'}, [4]byte{'f', 'm', 't', ' '}, uint32(16), uint16(1), uint16(2), uint32(48000), uint32(48000 * 4), uint16(4), uint16(16), [4]byte{'d', 'a', 't', 'a'}, uint32(n * 2)}
	for _, h := range hdr {
		binary.Write(f, binary.LittleEndian, h)
	}
	for _, x := range s {
		binary.Write(f, binary.LittleEndian, int16(math.Max(-1, math.Min(1, x))*32000))
	}
}

func TestSynthLevels(t *testing.T) {
	dir := os.Getenv("SYNTH_OUT")
	for k := 0; k < sndCount+1; k++ {
		if k == sndCount {
			continue
		}
		e := &engine{rev: newFDN(), rng: 12345, nextAmb: 1e9}
		e.play(k, 1, 0, 1)
		buf := make([]float64, int(48000*3.2)*2)
		e.render(buf)
		peak, rms := 0.0, 0.0
		for _, x := range buf {
			if math.IsNaN(x) {
				t.Fatalf("kind %d NaN", k)
			}
			peak = math.Max(peak, math.Abs(x))
			rms += x * x
		}
		rms = math.Sqrt(rms / float64(len(buf)))
		t.Logf("kind %2d peak %.3f rms %.4f", k, peak, rms)
		if dir != "" {
			writeWav(dir+"/k"+string(rune('a'+k))+".wav", buf)
		}
	}
	e := &engine{rev: newFDN(), rng: 1, nextAmb: 0}
	e.set(0.25, 0.3, 0.035, 0, 0)
	e.robots(0.3, 0, 0.4, 0)
	buf := make([]float64, 48000*10*2)
	e.render(buf)
	peak := 0.0
	for _, x := range buf {
		peak = math.Max(peak, math.Abs(x))
	}
	t.Logf("ambient bed 10s peak %.3f voices %d", peak, len(e.voices))
	if dir != "" {
		writeWav(dir+"/ambient.wav", buf)
	}
}
