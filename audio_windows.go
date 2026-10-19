//go:build windows

package main

import (
	"log"
	"syscall"
	"time"
	"unsafe"
)

type waveFormatEx struct {
	formatTag      uint16
	channels       uint16
	samplesPerSec  uint32
	avgBytesPerSec uint32
	blockAlign     uint16
	bitsPerSample  uint16
	cbSize         uint16
}

type waveHdr struct {
	data          *byte
	bufferLength  uint32
	bytesRecorded uint32
	user          uintptr
	flags         uint32
	loops         uint32
	next          uintptr
	reserved      uintptr
}

const (
	sampleRate = 48000
	blockLen   = 960
	numBufs    = 4
)

var (
	winmm          = syscall.NewLazyDLL("winmm.dll")
	pWaveOutOpen   = winmm.NewProc("waveOutOpen")
	pWaveOutPrep   = winmm.NewProc("waveOutPrepareHeader")
	pWaveOutWrite  = winmm.NewProc("waveOutWrite")
	pWaveOutReset  = winmm.NewProc("waveOutReset")
	pWaveOutClose  = winmm.NewProc("waveOutClose")
	pWaveOutUnprep = winmm.NewProc("waveOutUnprepareHeader")
)

func audioSet(humVol, humPan, buzz, buzzPan, crackle float64) {
	eng.set(humVol, humPan, buzz, buzzPan, crackle)
}

func audioImpact(strength float64) {
	eng.play(sndHit, 0.8*(0.25+0.6*strength), 0, 0.4)
}

func audioStep(strength, pan float64, kind int) { eng.play(kind, strength, pan, 2) }

func audioPlay(kind int, amp, pan, dist float64) { eng.play(kind, amp, pan, dist) }

func audioRobots(weld, weldPan, servo, servoPan float64) { eng.robots(weld, weldPan, servo, servoPan) }

func audioClick() { eng.play(sndRelay, 0.5, 0, 0.3) }

type synth struct {
	quit, running bool
	fbuf          []float64
}

var snd = &synth{}

func (s *synth) fill(out []int16) {
	if len(s.fbuf) != len(out) {
		s.fbuf = make([]float64, len(out))
	}
	eng.render(s.fbuf)
	for i, x := range s.fbuf {
		out[i] = int16(x * 30000)
	}
}

var (
	audioHandle uintptr
	audioHdrs   [numBufs]*waveHdr
	audioBufs   [numBufs][]int16
	audioFmt    = new(waveFormatEx)
)

func startAudio() {
	*audioFmt = waveFormatEx{formatTag: 1, channels: 2, samplesPerSec: sampleRate, avgBytesPerSec: sampleRate * 4, blockAlign: 4, bitsPerSample: 16}
	var h uintptr
	r, _, _ := pWaveOutOpen.Call(uintptr(unsafe.Pointer(&h)), 0xFFFFFFFF, uintptr(unsafe.Pointer(audioFmt)), 0, 0, 0)
	if r != 0 {
		log.Printf("audio: waveOutOpen failed %d", r)
		return
	}
	audioHandle = h
	for i := range audioHdrs {
		audioBufs[i] = make([]int16, blockLen*2)
		audioHdrs[i] = &waveHdr{data: (*byte)(unsafe.Pointer(&audioBufs[i][0])), bufferLength: blockLen * 4}
		pWaveOutPrep.Call(audioHandle, uintptr(unsafe.Pointer(audioHdrs[i])), unsafe.Sizeof(waveHdr{}))
		audioHdrs[i].flags |= 1
	}
	snd.running = true
	log.Println("audio started")
	go func() {
		for !snd.quit {
			busy := true
			for i, hd := range audioHdrs {
				if hd.flags&1 != 0 {
					snd.fill(audioBufs[i])
					hd.flags &^= 1
					pWaveOutWrite.Call(audioHandle, uintptr(unsafe.Pointer(hd)), unsafe.Sizeof(waveHdr{}))
					busy = false
				}
			}
			if busy {
				time.Sleep(3 * time.Millisecond)
			}
		}
	}()
}

func stopAudio() {
	if !snd.running {
		return
	}
	snd.quit = true
	time.Sleep(20 * time.Millisecond)
	pWaveOutReset.Call(audioHandle)
	for _, hd := range audioHdrs {
		pWaveOutUnprep.Call(audioHandle, uintptr(unsafe.Pointer(hd)), unsafe.Sizeof(waveHdr{}))
	}
	pWaveOutClose.Call(audioHandle)
}
