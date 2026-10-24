//go:build !windows

package main

import (
	"encoding/binary"
	"math"
	"os"
	"testing"
)

// AUDIOWAV=out.wav go test -run TestDumpRadioAudio: the mixer's output with the radio
// reading a wave's intel into the handset at your ear, then a shout from the speaker.
func TestDumpRadioAudio(t *testing.T) {
	path := os.Getenv("AUDIOWAV")
	if path == "" {
		t.Skip("set AUDIOWAV")
	}
	tt := newTester(t, true)
	r := tt.g.r
	eng.clipSource(srcHandset, 1, 0.85, 0)
	eng.clipSource(srcRadio, 0.5, -0.3, 0.3)
	playIntel(r)
	secs := 9.0
	n := int(secs * fs)
	out := make([]float64, 0, 2*n)
	buf := make([]float64, 2*4800)
	var vr, ar float64
	for len(out) < 2*n {
		if len(out) == 2*int(6*fs) {
			radioSay("x_leech", true)
		}
		eng.render(buf)
		out = append(out, buf...)
	}
	for i := 0; i < len(out); i += 2 {
		x := (out[i] + out[i+1]) / 2
		if i < 2*int(0.05*fs) {
			continue
		}
		if i < 2*int(4*fs) {
			vr += x * x
		} else if i > 2*int(5.0*fs) && i < 2*int(5.9*fs) {
			ar += x * x
		}
	}
	t.Logf("rms with the voice %.3f, ambience alone %.3f", math.Sqrt(vr/(4*fs)), math.Sqrt(ar/(0.9*fs)))
	f, _ := os.Create(path)
	defer f.Close()
	hdr := []any{[4]byte{'R', 'I', 'F', 'F'}, uint32(36 + len(out)*2), [4]byte{'W', 'A', 'V', 'E'}, [4]byte{'f', 'm', 't', ' '},
		uint32(16), uint16(1), uint16(2), uint32(fs), uint32(fs * 4), uint16(4), uint16(16), [4]byte{'d', 'a', 't', 'a'}, uint32(len(out) * 2)}
	for _, h := range hdr {
		binary.Write(f, binary.LittleEndian, h)
	}
	pcm := make([]int16, len(out))
	for i, x := range out {
		pcm[i] = int16(x * 30000)
	}
	binary.Write(f, binary.LittleEndian, pcm)
}
