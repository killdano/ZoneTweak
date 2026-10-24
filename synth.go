package main

import (
	"math"
	"sync"
)

// Procedural sound engine: modal metal, friction, hydraulics, arcs, formant robot speech,
// an industrial ambience bed and a large-hall feedback delay network reverb.

const fs = 48000.0

type res struct{ a1, a2, y1, y2, g float64 }

func (r *res) set(f, decay, gain float64) {
	if f > fs*0.45 {
		f = fs * 0.45
	}
	rr := math.Exp(-1 / (decay * fs))
	w := 2 * math.Pi * f / fs
	r.a1, r.a2 = 2*rr*math.Cos(w), -rr*rr
	r.g = gain * (1 - rr) * 2
}

func (r *res) step(x float64) float64 {
	y := r.a1*r.y1 + r.a2*r.y2 + x*r.g
	r.y2, r.y1 = r.y1, y
	return y
}

type svf struct{ lp, bp float64 }

func (s *svf) step(x, fc, q float64) (lp, bp, hp float64) {
	if fc > 7000 {
		fc = 7000
	}
	f := 2 * math.Sin(math.Pi*fc/fs)
	s.lp += f * s.bp
	hp = x - s.lp - q*s.bp
	s.bp += f * hp
	return s.lp, s.bp, hp
}

type voice struct {
	kind                 int
	age, dur             float64
	amp, pan, dist, send float64
	rng                  uint32
	r                    [10]res
	nr                   int
	f                    [4]svf
	ph                   [6]float64
	lp                   [4]float64
	a, b, c, d           float64
	cut                  float64
}

func (v *voice) rnd() float64 {
	v.rng ^= v.rng << 13
	v.rng ^= v.rng >> 17
	v.rng ^= v.rng << 5
	return float64(v.rng)/2147483648.0 - 1
}

func (v *voice) bank(f0 float64, ratios, decays []float64, gain float64) {
	v.nr = len(ratios)
	for i := range ratios {
		g := gain / math.Sqrt(float64(i)+1)
		v.r[i].set(f0*ratios[i]*(1+0.01*v.rnd()), decays[i], g)
	}
}

func (v *voice) bankStep(x float64) float64 {
	s := 0.0
	for i := 0; i < v.nr; i++ {
		s += v.r[i].step(x)
	}
	return s
}

var (
	creakRatios = [5]float64{1, 2.02, 3.07, 4.4, 5.9}
	plate       = []float64{1, 2.756, 5.404, 8.933, 13.34, 18.64, 24.81, 31.87}
	beamRatios  = []float64{1, 2.08, 3.32, 4.63, 6.07, 7.53, 9.22}
	vowels      = [][2]float64{{270, 2290}, {530, 1840}, {660, 1720}, {300, 870}, {640, 1190}, {390, 1990}, {440, 1020}, {490, 1350}}
)

func step01(x float64) float64 {
	if x > 0 {
		return 1
	}
	return 0
}

func env(t, a, d float64) float64 {
	if t < a {
		return t / a
	}
	return math.Exp(-(t - a) / d)
}

func (v *voice) init() {
	switch v.kind {
	case sndHit:
		v.dur = 2.2
		v.bank(230+260*math.Abs(v.rnd()), plate, []float64{1.6, 1.1, 0.8, 0.6, 0.45, 0.32, 0.25, 0.2}, 0.9)
	case sndStep, sndHeavyStep:
		v.dur = 0.9
		v.bank(70+20*v.rnd(), []float64{1, 1.71, 2.52, 3.9}, []float64{0.25, 0.18, 0.12, 0.08}, 1.2)
		v.a = 1600 + 600*v.rnd()
	case sndTick:
		v.dur = 0.08
		v.bank(3100+400*v.rnd(), []float64{1, 1.62}, []float64{0.012, 0.008}, 0.5)
	case sndEjectID:
		v.dur = 0.7
	case sndLandID:
		v.dur = 1.8
		v.bank(130+30*v.rnd(), plate, []float64{1.2, 0.9, 0.7, 0.5, 0.35, 0.25, 0.2, 0.15}, 1.0)
	case sndPulseID:
		v.dur = 1.2
	case sndTelescopeID:
		v.dur = 1.2
		v.bank(240+40*v.rnd(), beamRatios, []float64{0.25, 0.2, 0.15, 0.12, 0.1, 0.08, 0.06}, 0.6)
	case sndIgniteID:
		v.dur = 2.0
	case sndToolID:
		// heavy steel latch body: low, inharmonic, short
		v.dur = 0.95
		v.bank(310+50*v.rnd(), plate[:6], []float64{0.22, 0.17, 0.12, 0.09, 0.07, 0.05}, 0.8)
	case sndBeep:
		v.dur = 0.2
	case sndTone0, sndTone1, sndTone2, sndTone3:
		v.dur = 0.5
	case sndAlarm:
		v.dur = 1.9
	case sndBoom:
		v.dur = 3.5
		v.bank(70+20*v.rnd(), plate, []float64{2.4, 1.8, 1.4, 1.0, 0.8, 0.6, 0.45, 0.35}, 1.0)
	case sndScanID:
		v.dur = 2.0
	case sndSpark:
		v.dur = 0.5
		v.bank(5200+900*v.rnd(), []float64{1, 1.37}, []float64{0.02, 0.015}, 0.5)
	case sndSteam:
		v.dur = 1.6
	case sndSurge:
		v.dur = 0.55
	case sndZip:
		v.dur = 0.45
	case sndZap:
		v.dur = 0.7
		v.bank(880+200*v.rnd(), plate[:5], []float64{0.4, 0.3, 0.2, 0.15, 0.1}, 0.4)
	case sndHydraulic:
		v.dur = 1.0
		v.bank(110+20*v.rnd(), []float64{1, 2.4, 4.1}, []float64{0.18, 0.12, 0.08}, 0.9)
	case sndSpeech:
		v.dur = 0.55 + 0.9*math.Abs(v.rnd())
		v.a = 62 + 55*math.Abs(v.rnd())
		v.b = -1
	case sndTreadClank:
		v.dur = 0.4
		v.bank(1100+300*v.rnd(), []float64{1, 1.41, 2.2}, []float64{0.05, 0.04, 0.03}, 0.6)
	case sndRelay:
		v.dur = 0.18
		v.bank(1900+300*v.rnd(), []float64{1, 1.48, 2.6}, []float64{0.03, 0.025, 0.02}, 0.7)
	case sndCreak:
		v.dur = 1.5
		f0 := 95 + 70*math.Abs(v.rnd())
		v.bank(f0, []float64{1, 2.02, 3.07, 4.4, 5.9}, []float64{0.4, 0.35, 0.3, 0.25, 0.2}, 0.5)
		v.a = f0
	case sndClang:
		v.dur = 3.0
		v.bank(105+60*math.Abs(v.rnd()), plate, []float64{2.6, 2.0, 1.5, 1.2, 0.9, 0.7, 0.5, 0.4}, 1.0)
	case sndServo:
		v.dur = 0.5
	case sndRip:
		v.dur = 2.0
		v.bank(170+40*v.rnd(), plate, []float64{1.8, 1.3, 1.0, 0.8, 0.6, 0.45, 0.35, 0.3}, 1.0)
	case sndRegrow:
		v.dur = 2.4
		v.bank(150+30*v.rnd(), plate, []float64{1.8, 1.3, 1.0, 0.8, 0.6, 0.45, 0.35, 0.3}, 1.0)
	case sndPowerDown, sndPowerUp:
		v.dur = 1.3
		v.bank(900, []float64{1, 1.5}, []float64{0.05, 0.04}, 0.6)
	case sndGrind:
		v.dur = 0.95
	case sndDrip:
		v.dur = 0.12
		v.a = 1500 + 1500*math.Abs(v.rnd())
	default:
		v.dur = 0.5
	}
}

// sample returns a mono sample for the voice at its current age
func (v *voice) sample() float64 {
	t := v.age
	n := v.rnd()
	switch v.kind {
	case sndHit:
		// touching the laser wall: a violent short-circuit - crack, arc blast, mains-buzz plasma
		// spitting and crackling, metal ring, sizzle. No pitch sweep.
		x := 0.0
		if t < 0.004 {
			x = n * 3
		}
		out := v.bankStep(x) * 0.22
		// the crack + pressure thump
		if t < 0.012 {
			out += n * 1.4 * (1 - t/0.012)
		}
		v.ph[0] += (85 - 40*math.Min(1, t/0.08)) / fs
		out += math.Tanh(math.Sin(2*math.Pi*v.ph[0])*3) * math.Exp(-t/0.06) * 0.6
		// plasma arc: noise band-passed, chopped by a 100 Hz buzz and torn into spits
		buzz := 0.5 + 0.5*math.Sin(2*math.Pi*100*t+2*math.Sin(2*math.Pi*7*t))
		_, bp, _ := v.f[0].step(n, 1800+1600*hash01(math.Floor(t*90)+v.d), 0.35)
		spit := 1.0
		if hash01(math.Floor(t*230)+v.d*3) > 0.55+0.35*math.Min(1, t/0.5) {
			spit = 2.4
		}
		arc := math.Tanh(bp*buzz*spit*3) * math.Exp(-t/0.28) * 0.55
		// crackle: sparse sharp impulses
		cr := 0.0
		if math.Abs(n) > 0.985-0.03*math.Exp(-t*6) {
			cr = n * 1.1 * math.Exp(-t*3.5)
		}
		// sizzle tail
		_, _, hp := v.f[1].step(n, 4500, 0.7)
		sz := hp * 0.12 * math.Exp(-t/0.5) * math.Min(1, t/0.05)
		return math.Tanh((out+arc+cr+sz)*1.3) * 0.8
	case sndStep, sndHeavyStep:
		x := 0.0
		if t < 0.012 {
			x = n
		}
		_, bp, _ := v.f[0].step(n, 300, 1.2)
		thud := math.Sin(2*math.Pi*(55*t-30*t*t)) * env(t, 0.003, 0.11) * 1.2
		body := v.bankStep(x*2+bp*0.05) * 0.6
		_, _, hp := v.f[1].step(n, 2500, 0.8)
		hiss := hp * env(t-0.02, 0.01, 0.1) * 0.12
		if t < 0.02 {
			hiss = 0
		}
		rattle := 0.0
		if t > 0.01 && t < 0.1 && math.Abs(n) > 0.985 {
			rattle = n * 0.5
		}
		_, rb, _ := v.f[2].step(rattle, v.a, 0.15)
		return thud + body + hiss + rb*0.5
	case sndTick:
		x := 0.0
		if t < 0.0015 {
			x = 1
		}
		return v.bankStep(x)*0.3 + n*env(t, 0.0005, 0.002)*0.15
	case sndEjectID:
		thump := math.Sin(2*math.Pi*(95*t-60*t*t)) * env(t, 0.002, 0.07)
		fc := 1600 * math.Exp(-t*3)
		_, bp, _ := v.f[0].step(n, fc+300, 0.6)
		air := bp * env(t, 0.008, 0.16) * 0.9
		return thump + air
	case sndLandID:
		x := 0.0
		if t < 0.006 {
			x = n * 2
		}
		thud := math.Sin(2*math.Pi*(60*t-25*t*t)) * env(t, 0.002, 0.09) * 1.3
		rattle := 0.0
		if t > 0.02 && t < 0.25 && math.Abs(n) > 0.99 {
			rattle = n
		}
		return v.bankStep(x+rattle*0.3)*0.4 + thud
	case sndPulseID:
		sub := math.Sin(2*math.Pi*(42*t+8*t*t)) * env(t, 0.01, 0.35) * 1.2
		fc := 2200*math.Exp(-t*5) + 120
		lp, _, _ := v.f[0].step(n, fc, 0.7)
		cr := 0.0
		if math.Abs(n) > 0.997-0.01*math.Exp(-t*4) {
			cr = n * 0.6 * math.Exp(-t*2)
		}
		return sub + lp*env(t, 0.005, 0.25)*0.7 + cr
	case sndTelescopeID:
		// three box sections sliding out: dry steel-on-steel scrape, each ending in a heavy
		// latch - no pitch sweeps, just friction and iron
		seg := math.Floor(t / 0.28)
		lt := t - seg*0.28
		out := 0.0
		if seg < 3 {
			if math.Abs(n) > 0.93 {
				v.c = 0.5 + 0.5*math.Abs(v.rnd()) // grit catching
			}
			v.c *= 0.9985
			lp, _, _ := v.f[0].step(n*(0.4+v.c), 850-120*seg, 0.9)
			e := math.Min(1, lt/0.04) * math.Max(0, 1-lt/0.24)
			out += math.Tanh(lp*2.2) * 0.3 * e
			x := 0.0
			if lt > 0.24 && lt < 0.243 {
				x = n * 2.4
			}
			out += v.bankStep(x)*0.35 + math.Sin(2*math.Pi*68*lt)*env(lt-0.24, 0.002, 0.04)*0.5*step01(lt-0.24)
		} else {
			out += v.bankStep(0) * 0.35
		}
		return out
	case sndIgniteID:
		out := 0.0
		if t < 0.7 {
			if math.Abs(n) > 0.993 {
				v.c = 0.04
			}
			if v.c > 0 {
				v.c -= 1 / fs
				v.ph[0] += 120 / fs
				out += (math.Mod(v.ph[0], 1)*2 - 1) * 0.3
				_, _, hp := v.f[0].step(n, 3000, 0.7)
				out += hp * 0.25
			}
		} else {
			b := t - 0.7
			e := math.Min(1, b/0.04) * math.Exp(-b*1.4)
			h := 0.0
			for k := 1; k <= 4; k++ {
				h += math.Sin(2*math.Pi*60*float64(k)*b) / float64(k)
			}
			out += h * e * 0.7
			out += math.Sin(2*math.Pi*38*b) * math.Exp(-b*6) * 0.9
		}
		return out
	case sndToolID:
		// mode change: geared servo grinds the shroud round, a solenoid latch slams home,
		// relays chatter, the coil arcs and the transformer settles with a dirty buzz
		out := 0.0
		const latch = 0.3
		if t < latch+0.02 {
			f := 48 + 40*smoothstep01(t/latch)
			v.ph[0] += f / fs
			ph := math.Mod(v.ph[0], 1)
			saw := ph*2 - 1
			teeth := 0.55 + 0.45*math.Abs(math.Sin(math.Pi*v.ph[0]*13)) // gear mesh
			brush := 0.0
			if hash01(math.Floor(t*900)+v.d) > 0.8 {
				brush = n * 0.6
			}
			m := math.Tanh((saw*teeth+brush)*3) * env(t, 0.015, latch)
			lp, _, _ := v.f[0].step(m, 1300, 0.8)
			_, bp, _ := v.f[1].step(m, 3.0*f*13*0.5, 0.25)
			out += lp*0.22 + bp*0.13
		}
		// solenoid: chest thump + steel ring
		if b := t - latch; b >= 0 {
			if b < 0.004 {
				out += v.bankStep(n * 3)
			} else {
				out += v.bankStep(0) * 0.55
			}
			v.ph[1] += (72 - 30*math.Min(1, b/0.06)) / fs
			out += math.Tanh(math.Sin(2*math.Pi*v.ph[1])*2) * math.Exp(-b/0.05) * 0.28
		}
		// relay chatter
		for k := 0; k < 4; k++ {
			at := latch + 0.03 + float64(k)*0.022 + 0.008*hash01(float64(k)+v.d)
			if b := t - at; b >= 0 && b < 0.006 {
				_, _, hp := v.f[2].step(n, 2500, 0.6)
				out += hp * 0.35 * (1 - b/0.006)
			}
		}
		// coil arc: sparse crackle
		if b := t - latch - 0.02; b >= 0 && b < 0.22 {
			if hash01(math.Floor(t*1400)+v.d*3) > 0.86-0.1*math.Exp(-b/0.05) {
				_, bp, _ := v.f[3].step(n, 3200+1500*hash01(math.Floor(t*300)), 0.4)
				out += bp * 0.5 * math.Exp(-b/0.09)
			}
		}
		// transformer settle: rich 100 Hz buzz, slightly unstable, clipped and band-limited
		if b := t - latch - 0.03; b >= 0 {
			v.ph[2] += (100 + 3*math.Sin(2*math.Pi*7*t)) / fs
			z := math.Mod(v.ph[2], 1)*2 - 1
			z = math.Tanh(z*4 + n*0.3)
			_, bp, _ := v.f[1].step(z, 520, 0.5)
			out += bp * 0.11 * math.Min(1, b/0.03) * math.Exp(-b/0.22)
		}
		return out * 0.9
	case sndBeep:
		f := 1760.0
		if t > 0.085 {
			f = 1320
		}
		x := math.Sin(2 * math.Pi * f * t)
		x = math.Max(-0.6, math.Min(0.6, x*1.6)) // a cheap little speaker
		k := env(math.Mod(t, 0.085), 0.002, 0.03)
		if t > 0.17 {
			k = 0
		}
		return x * k * 0.55
	case sndTone0, sndTone1, sndTone2, sndTone3:
		// Simon's tones (the old game's notes: E4, A4, E3, C#4), a soft square through a
		// small speaker
		f := []float64{329.63, 440, 164.81, 277.18}[v.kind-sndTone0]
		x := math.Sin(2*math.Pi*f*t) + 0.3*math.Sin(6*math.Pi*f*t) + 0.12*math.Sin(10*math.Pi*f*t)
		k := math.Min(1, t/0.006) * math.Min(1, (0.5-t)/0.08)
		return x * math.Max(0, k) * 0.32
	case sndAlarm:
		// industrial two-tone klaxon, harsh and slightly detuned, through a cheap horn
		f := 640.0
		if math.Mod(t, 0.95) > 0.475 {
			f = 470
		}
		v.ph[0] += f / fs
		v.ph[1] += f * 1.007 / fs
		sq := math.Tanh(math.Sin(2*math.Pi*v.ph[0])*4) + 0.6*math.Tanh(math.Sin(2*math.Pi*v.ph[1])*4)
		_, bp, _ := v.f[0].step(sq, 1100, 0.8)
		return bp * 0.32 * env(t, 0.02, 1.85)
	case sndBoom:
		// explosion: blast crack, deep pressure wave, metal shrapnel ring, rumbling tail
		out := 0.0
		if t < 0.02 {
			out += n * 1.6 * (1 - t/0.02)
		}
		v.ph[0] += (55 - 30*math.Min(1, t/0.4)) / fs
		out += math.Tanh(math.Sin(2*math.Pi*v.ph[0])*2.5) * math.Exp(-t/0.35) * 0.9
		lp, _, _ := v.f[0].step(n, 260, 0.6)
		out += lp * 1.4 * math.Exp(-t/0.9)
		x := 0.0
		if t < 0.006 {
			x = n * 3
		}
		out += v.bankStep(x) * 0.25
		if math.Abs(n) > 0.99-0.04*math.Exp(-t*3) {
			out += n * 0.8 * math.Exp(-t*1.5)
		}
		return math.Tanh(out * 1.2)
	case sndScanID:
		out := 0.0
		for _, st := range [2]float64{0, 1.0} {
			b := t - st
			if b < 0 || b > 0.9 {
				continue
			}
			e := math.Sin(b / 0.9 * math.Pi)
			out += math.Sin(2*math.Pi*4300*b+3*math.Sin(2*math.Pi*55*b)) * 0.025 * e
			if hash01(math.Floor(b*70)+v.d+st) > 0.55 {
				_, _, hp := v.f[0].step(n, 3500, 0.7)
				out += hp * 0.06 * e
			}
		}
		return out
	case sndSpark:
		x := 0.0
		if t < 0.002 {
			x = 3
		}
		if math.Abs(n) > 0.97-0.1*math.Exp(-t*12) {
			x += n * math.Exp(-t*10)
		}
		_, _, hp := v.f[0].step(n, 2500, 0.8)
		sizzle := hp * env(t, 0.004, 0.08) * 0.25
		return v.bankStep(x)*0.25 + x*0.3 + sizzle
	case sndSteam:
		_, _, hp := v.f[0].step(n, 1200, 0.9)
		_, bp, _ := v.f[1].step(hp, 3800+500*math.Sin(t*7), 0.7)
		e := math.Min(1, t/0.03)
		if t > 0.55 {
			e = math.Exp(-(t - 0.55) / 0.3)
		}
		thump := math.Sin(2*math.Pi*70*t) * env(t, 0.003, 0.05) * 0.4
		return (hp*0.25+bp*0.4)*e + thump
	case sndSurge:
		// corona build-up: crackle getting denser, mains strain underneath
		k := t / v.dur
		rate := 60 + 900*k*k
		x := 0.0
		if math.Abs(n) > 1-rate/fs*2 {
			x = n * (0.5 + 0.5*math.Abs(v.rnd()))
		}
		_, _, hp := v.f[0].step(x, 1800, 0.5)
		_, ring, _ := v.f[1].step(hp, 3200+1500*math.Sin(v.d), 0.15)
		hum := (math.Sin(2*math.Pi*120*t) + 0.4*math.Sin(2*math.Pi*240*t)) * 0.06 * k
		return (hp*0.5+ring*0.25)*math.Min(1, (v.dur-t)/0.05) + hum
	case sndZip:
		// the arc travelling along the wall: dense crackle, mains-modulated hiss
		mains := 0.55 + 0.45*math.Abs(math.Sin(2*math.Pi*60*t))
		x := 0.0
		if math.Abs(n) > 0.96 {
			x = n
		}
		fc := 2600 + 900*math.Sin(t*23+v.d) + 500*v.rnd()
		_, bp, _ := v.f[0].step(n*0.35+x, fc, 0.35)
		_, _, hp := v.f[1].step(x, 2500, 0.6)
		e := math.Min(1, t/0.02) * math.Min(1, (v.dur-t)/0.12)
		return (bp*0.35*mains + hp*0.35) * e
	case sndZap:
		// discharge into the generator: crack, thump, sizzle
		crack := 0.0
		if t < 0.004 {
			crack = n * 2.2 * (1 - t/0.004)
		}
		thump := math.Sin(2*math.Pi*(70*t-40*t*t)) * env(t, 0.002, 0.07) * 0.7
		_, _, hp := v.f[0].step(n, 3500, 0.7)
		sizzle := hp * env(t, 0.004, 0.16) * 0.22
		x := 0.0
		if math.Abs(n) > 0.985-0.04*math.Exp(-t*12) {
			x = n * math.Exp(-t*7)
		}
		return crack + thump + sizzle + x*0.35 + v.bankStep(crack*0.2)*0.08
	case sndHydraulic:
		// a pump motor labouring under load, oil forced through the lines, then the valve
		// slams shut and the line bleeds off
		run := math.Min(1, t/0.05) * math.Min(1, math.Max(0, (0.6-t)/0.08))
		v.ph[0] += 47 / fs
		saw := math.Mod(v.ph[0], 1)*2 - 1
		nl, _, _ := v.f[2].step(n, 30, 0.9)
		mlp, _, _ := v.f[1].step(saw, 280, 0.9)
		motor := mlp * (0.75 + 2.5*nl) * 0.35 * run
		fl, _, _ := v.f[0].step(n, 1600, 0.9)
		fluid := fl * (0.7 + 0.3*math.Sin(2*math.Pi*13*t)) * 0.3 * run
		x := 0.0
		if t > 0.56 && t < 0.564 {
			x = n * 3
		}
		thud := math.Sin(2*math.Pi*52*(t-0.56)) * env(t-0.56, 0.002, 0.07) * 0.6 * step01(t-0.56)
		_, _, hp := v.f[3].step(n, 3200, 0.8)
		bleed := hp * 0.16 * env(t-0.58, 0.01, 0.13) * step01(t-0.58)
		return motor + fluid + v.bankStep(x)*0.5 + thud + bleed
	case sndSpeech:
		return v.speech(t, n)
	case sndTreadClank:
		x := 0.0
		for k := 0; k < 5; k++ {
			at := 0.02 + float64(k)*0.06 + 0.012*math.Sin(float64(k)*7.1+v.d)
			if t > at && t < at+0.0015 {
				x = 1.5
			}
		}
		lp, _, _ := v.f[0].step(n, 500, 0.7)
		return v.bankStep(x)*0.3 + lp*0.3*env(t, 0.01, 0.15)
	case sndRelay:
		x := 0.0
		if t < 0.002 || (t > 0.03 && t < 0.0315) {
			x = 2
		}
		return v.bankStep(x)*0.35 + n*env(t, 0.0005, 0.003)*0.2
	case sndCreak:
		v.a *= 1 + 0.00004*v.rnd()
		rate := 70 + 60*math.Sin(t*2.3+v.d) + 25*math.Sin(t*11)
		v.ph[0] += rate / fs
		x := 0.0
		if v.ph[0] > 1 {
			v.ph[0]--
			x = 0.8 + 0.4*n
		}
		x += n * 0.08
		e := math.Sin(math.Min(1, t/1.5) * math.Pi)
		if int(t*fs)%64 == 0 {
			for i := 0; i < v.nr; i++ {
				v.r[i].set(v.a*creakRatios[i], 0.3, 0.5/float64(i+1))
			}
		}
		return v.bankStep(x) * 0.5 * e
	case sndClang:
		x := 0.0
		if t < 0.005 {
			x = n * 2.5
		}
		return v.bankStep(x)*0.45 + math.Sin(2*math.Pi*50*t)*env(t, 0.002, 0.1)*0.5
	case sndServo:
		f := 150 + 200*math.Sin(t*6)
		v.ph[0] += f / fs
		saw := math.Mod(v.ph[0], 1)*2 - 1
		_, bp, _ := v.f[0].step(saw, f*4, 0.4)
		return bp * 0.3 * math.Sin(math.Min(1, t/0.5)*math.Pi)
	case sndRip:
		// the mount tearing loose: a heavy impact, a dense crunch of shearing metal, the
		// plate groaning, cables snapping like whips, then debris settling
		out := 0.0
		x := 0.0
		if t < 0.008 {
			x = n * 3
		}
		out += math.Sin(2*math.Pi*44*t) * env(t, 0.003, 0.22) * 1.1
		if t < 0.7 {
			if math.Abs(n) > 0.88 {
				v.c = 0.4 + 0.6*math.Abs(v.rnd())
			}
			v.c *= 0.9990
			lp, _, _ := v.f[0].step(n*v.c, 2400, 0.8)
			_, bp, _ := v.f[1].step(n*v.c, 850, 0.7)
			crunch := math.Tanh((lp*1.4+bp*1.2)*2.5) * 0.42 * math.Exp(-t*3.2)
			out += crunch
			x += crunch * 0.25 // the tear drives the plate: it groans as it goes
		}
		for _, at := range []float64{0.11, 0.23, 0.34} {
			if t > at && t < at+0.018 {
				_, _, hp := v.f[2].step(n, 3500, 0.7)
				out += hp * 0.55 * (1 - (t-at)/0.018)
			}
		}
		if t > 0.5 && math.Abs(n) > 0.994 {
			x += n * 0.6 * math.Exp(-(t-0.5)*2.5)
		}
		out += v.bankStep(x) * 0.4
		return out
	case sndRegrow:
		// the new mount being driven out and bolted down: heavy actuator rumble, gear
		// ratchet, stress creaks, an impact wrench run along the bolts, then the clamp
		out := 0.0
		run := math.Min(1, t/0.12) * math.Min(1, math.Max(0, (1.05-t)/0.15))
		if run > 0 {
			v.ph[0] += 38 / fs
			v.ph[1] += 38.7 / fs
			saw := math.Mod(v.ph[0], 1) + math.Mod(v.ph[1], 1) - 1
			rum, _, _ := v.f[0].step(saw, 210, 0.9)
			_, gr, _ := v.f[1].step(n, 620, 0.9)
			ratchet := math.Pow(0.5+0.5*math.Sin(2*math.Pi*22*t), 4)
			out += (rum*0.45 + gr*0.22*ratchet) * run
		}
		x := 0.0
		if t > 0.3 && t < 1.0 && math.Abs(n) > 0.9993 {
			x += n * 0.9 // stress creaks in the plate
		}
		for k := 0; k < 4; k++ {
			at := 1.05 + float64(k)*0.11
			if lt := t - at; lt > 0 && lt < 0.075 {
				if math.Mod(lt*55, 1) < 0.18 {
					_, _, hp := v.f[2].step(n, 2600, 0.8)
					out += hp * 0.5
					x += 0.12
				}
			}
		}
		if t > 1.55 && t < 1.556 {
			x += n * 3
		}
		out += math.Sin(2*math.Pi*45*(t-1.55)) * env(t-1.55, 0.002, 0.12) * 0.8 * step01(t-1.55)
		out += v.bankStep(x) * 0.4
		return out
	case sndPowerDown:
		// contactor drops out: a hard clack, the transformer hum dying with a sag and arcing
		x := 0.0
		if t < 0.002 {
			x = 2
		}
		v.ph[0] += 100 * (1 - 0.05*math.Min(1, t/0.4)) / fs
		ph := 2 * math.Pi * v.ph[0]
		h := math.Tanh((math.Sin(ph)+0.5*math.Sin(2*ph)+0.3*math.Sin(3*ph))*1.8) * 0.26 * math.Exp(-t*2.6)
		cr := 0.0
		if math.Abs(n) > 0.992 {
			cr = n * 0.5 * math.Exp(-t*3)
		}
		return v.bankStep(x)*0.4 + h + cr
	case sndPowerUp:
		// contactor pulls in: clack, an arc snap, then the transformer buzz swells in at
		// mains pitch - steady, no rising whine
		x := 0.0
		if t < 0.002 {
			x = 2
		}
		v.ph[0] += 100 / fs
		ph := 2 * math.Pi * v.ph[0]
		h := math.Tanh((math.Sin(ph)+0.5*math.Sin(2*ph)+0.3*math.Sin(3*ph))*1.8) * 0.24 * math.Min(1, t/0.35) * math.Exp(-math.Max(0, t-0.4)*1.6)
		cr := 0.0
		if t < 0.25 && math.Abs(n) > 0.985 {
			cr = n * 0.5
		}
		return v.bankStep(x)*0.4 + h + cr
	case sndGrind:
		f := 4300 + 300*math.Sin(t*40) + 200*v.rnd()
		v.ph[0] += f / fs
		w := math.Sin(2 * math.Pi * v.ph[0])
		_, _, hp := v.f[0].step(n, 2200, 0.7)
		e := math.Sin(math.Min(1, t/0.95) * math.Pi)
		cr := 0.0
		if math.Abs(n) > 0.96 {
			cr = n * 0.4
		}
		return (w*0.08 + hp*0.3*(0.6+0.4*math.Sin(t*90)) + cr) * e
	case sndDrip:
		f := v.a * math.Exp(-t*25)
		v.ph[0] += f / fs
		return math.Sin(2*math.Pi*v.ph[0]) * env(t, 0.001, 0.02) * 0.3
	}
	return 0
}

// robot language: buzz + formants that jump between vowels, ring-mod, crushed through a radio band
func (v *voice) speech(t, n float64) float64 {
	syl := 0.07 + 0.05*math.Abs(math.Sin(v.d*3.1))
	si := math.Floor(t / syl)
	if si != v.b {
		v.b = si
		k := int(math.Abs(v.rnd()) * float64(len(vowels)))
		if k >= len(vowels) {
			k = len(vowels) - 1
		}
		v.lp[0], v.lp[1] = vowels[k][0], vowels[k][1]
		v.lp[2] = v.a * (0.85 + 0.35*math.Abs(v.rnd()))
		v.lp[3] = 0
		if v.rnd() > 0.55 {
			v.lp[3] = 1
		}
	}
	v.ph[1] += (v.lp[0] - v.ph[1]) * 0.004
	v.ph[2] += (v.lp[1] - v.ph[2]) * 0.004
	v.ph[3] += (v.lp[2] - v.ph[3]) * 0.003
	lt := t - si*syl
	gate := math.Min(1, lt/0.01) * math.Min(1, (syl-lt)/0.012)
	if v.lp[3] > 0.5 {
		gate *= 0.15
	}
	v.ph[0] += v.ph[3] * (1 + 0.03*n) / fs
	pulse := 0.0
	if math.Mod(v.ph[0], 1) < 0.18 {
		pulse = 1
	}
	src := pulse*0.8 + n*0.08
	if lt < 0.012 && v.lp[3] > 0.5 {
		src += n * 1.5
	}
	_, f1, _ := v.f[0].step(src, v.ph[1], 0.18)
	_, f2, _ := v.f[1].step(src, v.ph[2], 0.12)
	s := (f1 + f2*0.8) * gate
	s *= 0.55 + 0.45*math.Sin(2*math.Pi*(37+v.d)*t)
	// sample-and-hold + bit crush
	if int(t*fs)%2 == 0 {
		v.c = math.Round(s*40) / 40
	}
	s = v.c
	_, _, hp := v.f[2].step(s, 320, 0.7)
	lp, _, _ := v.f[3].step(hp, 3000, 0.7)
	st := 0.0
	if math.Abs(n) > 0.997 {
		st = n * 0.3
	}
	e := math.Min(1, t/0.02) * math.Min(1, (v.dur-t)/0.05)
	return math.Tanh(lp*2.2)*0.55*e + st*e
}

// ---- reverb ----

type fdn struct {
	buf  [8][]float64
	pos  [8]int
	damp [8]float64
	g    float64
}

func newFDN() *fdn {
	f := &fdn{g: 0.82}
	lens := []int{2477, 2687, 2381, 2203, 1987, 2129, 1847, 1733}
	for i := range f.buf {
		f.buf[i] = make([]float64, lens[i])
	}
	return f
}

func (f *fdn) step(in float64) (float64, float64) {
	var o [8]float64
	for i := 0; i < 8; i++ {
		o[i] = f.buf[i][f.pos[i]]
	}
	sum := 0.0
	for i := 0; i < 8; i++ {
		sum += o[i]
	}
	sum *= 2.0 / 8
	for i := 0; i < 8; i++ {
		x := (o[i] - sum) * f.g
		f.damp[i] += (x - f.damp[i]) * 0.35
		f.buf[i][f.pos[i]] = f.damp[i] + in*0.35
		f.pos[i]++
		if f.pos[i] >= len(f.buf[i]) {
			f.pos[i] = 0
		}
	}
	return o[0] + o[2] + o[4] + o[6], o[1] + o[3] + o[5] + o[7]
}

// ---- engine ----

type engine struct {
	mu                               sync.Mutex
	pending                          []voice
	voices                           []voice
	humT, humP, buzzT, buzzP, crackT float64
	weldT, weldP, machT, machP       float64
	// the EMP cart's gas engine: how loud, how fast (rpm), where; smoothed; its firing
	motorT, rpmT, motorPT                float64
	motor, rpm, motorP, firePh, fireAmp  float64
	fireJit                              float64
	hum, buzz, crack, weld, mach, hp, bp float64
	wp, mp                               float64
	t                                    float64
	rev                                  *fdn
	rng                                  uint32
	nextAmb                              float64
	f                                    [12]svf
	ph                                   [8]float64
	drone                                [3]res
	gate                                 float64
	dcL, dcR                             float64
	// pointer-on-wall interference: sample-and-hold static, stutter gate, hopping carrier
	ifHold, ifCnt, ifN, ifGate, ifGT float64
	ifHop, ifPh, ifHT, ifHP          float64
	// recorded clips (the radio voice), each from one of a few moving sources
	clips  []clipPlay
	cpend  []clipPlay
	csrc   [4]clipSrc // what the game wants each source to sound like
	ccur   [4]clipSrc // smoothed toward csrc
	cclear [4]bool
	cend   [4]float64 // when the last clip queued on each source ends
	tNow   float64    // engine time at the start of the current block (for the game thread)
}

// ---- recorded clips ----

const clipRate = 16000.0 // the voice clips are 16 kHz mono

type clipPlay struct {
	data []int16
	pos  float64 // in clip samples
	at   float64 // engine time it starts
	src  int
}

type clipSrc struct{ amp, pan, send float64 }

type clipItem struct {
	data []int16
	gap  float64 // silence after it
}

// queueClips: play these one after another from source src (interrupt cuts whatever that
// source was saying).
func (e *engine) queueClips(src int, items []clipItem, interrupt bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if interrupt {
		e.cclear[src] = true
		keep := e.cpend[:0]
		for _, c := range e.cpend {
			if c.src != src {
				keep = append(keep, c)
			}
		}
		e.cpend = keep
		e.cend[src] = 0
	}
	at := math.Max(e.tNow+0.06, e.cend[src])
	for _, it := range items {
		if len(it.data) == 0 {
			continue
		}
		e.cpend = append(e.cpend, clipPlay{data: it.data, at: at, src: src})
		at += float64(len(it.data))/clipRate + it.gap
	}
	e.cend[src] = at
}

// stopClips: silence every recorded clip, playing or queued (leaving the game).
func (e *engine) stopClips() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cpend = e.cpend[:0]
	for i := range e.cclear {
		e.cclear[i], e.cend[i] = true, 0
	}
}

// clipSource: where a source is right now, as the listener hears it.
func (e *engine) clipSource(src int, amp, pan, send float64) {
	e.mu.Lock()
	e.csrc[src] = clipSrc{amp, math.Max(-1, math.Min(1, pan)), send}
	e.mu.Unlock()
}

// clipBusyFor: how many seconds of speech are still queued on a source.
func (e *engine) clipBusyFor(src int) float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return math.Max(0, e.cend[src]-e.tNow)
}

var eng = &engine{rev: newFDN(), rng: 0x9E3779B9, nextAmb: 4}

func (e *engine) rnd() float64 {
	e.rng ^= e.rng << 13
	e.rng ^= e.rng >> 17
	e.rng ^= e.rng << 5
	return float64(e.rng)/2147483648.0 - 1
}

var kindGain = map[int]float64{sndCreak: 12, sndTreadClank: 3, sndRelay: 1.6, sndTelescopeID: 1.8, sndTick: 1.4}

// eventGain: one-shot effects sit lower in the mix than the ambience and hums
const eventGain = 0.55

func (e *engine) play(kind int, amp, pan, dist float64) {
	if g, ok := kindGain[kind]; ok {
		amp *= g
	}
	amp *= eventGain
	if amp < 0.005 {
		return
	}
	e.mu.Lock()
	if len(e.pending)+len(e.voices) < 64 {
		v := voice{kind: kind, amp: amp, pan: math.Max(-1, math.Min(1, pan)), dist: dist}
		v.rng = e.rng ^ uint32(len(e.pending)*7919+kind*104729+1)
		e.rnd()
		v.d = math.Abs(e.rnd()) * 10
		v.send = math.Min(0.75, 0.12+0.07*dist)
		v.cut = math.Exp(-2 * math.Pi * math.Min(16000, 18000/(1+dist*0.45)) / fs)
		v.init()
		e.pending = append(e.pending, v)
	}
	e.mu.Unlock()
}

func (e *engine) ambience() {
	if e.t < e.nextAmb {
		return
	}
	e.nextAmb = e.t + 7 + 12*math.Abs(e.rnd())
	pan := e.rnd()
	switch k := math.Abs(e.rnd()); {
	case k < 0.22:
		e.playLocked(sndClang, 0.13, pan, 14)
	case k < 0.42:
		e.playLocked(sndCreak, 0.17, pan, 9)
	case k < 0.55:
		e.playLocked(sndSteam, 0.09, pan, 12)
	case k < 0.68:
		for i := 0; i < 3; i++ {
			e.playLocked(sndTreadClank, 0.07, pan, 15)
		}
	case k < 0.8:
		e.playLocked(sndSpeech, 0.06, pan, 16)
	default:
		for i := 0; i < 2+int(3*math.Abs(e.rnd())); i++ {
			e.playLocked(sndDrip, 0.05+0.04*math.Abs(e.rnd()), e.rnd(), 6)
		}
	}
}

func (e *engine) playLocked(kind int, amp, pan, dist float64) {
	if g, ok := kindGain[kind]; ok {
		amp *= g
	}
	v := voice{kind: kind, amp: amp, pan: pan, dist: dist}
	v.rng = e.rng ^ uint32(kind*104729+12345)
	e.rnd()
	v.d = math.Abs(e.rnd()) * 10
	v.send = math.Min(0.8, 0.12+0.07*dist)
	v.cut = math.Exp(-2 * math.Pi * math.Min(16000, 18000/(1+dist*0.45)) / fs)
	v.init()
	if kind == sndDrip {
		v.age = -math.Abs(e.rnd()) * 1.5
	}
	e.voices = append(e.voices, v)
}

func (e *engine) render(out []float64) {
	e.mu.Lock()
	e.voices = append(e.voices, e.pending...)
	e.pending = e.pending[:0]
	for si := range e.cclear {
		if e.cclear[si] {
			keep := e.clips[:0]
			for _, c := range e.clips {
				if c.src != si {
					keep = append(keep, c)
				}
			}
			e.clips = keep
			e.cclear[si] = false
		}
	}
	e.clips = append(e.clips, e.cpend...)
	e.cpend = e.cpend[:0]
	csrc := e.csrc
	ht, hpT, bt, bpT, ct := e.humT, e.humP, e.buzzT, e.buzzP, e.crackT
	wt, wpT, mt, mpT := e.weldT, e.weldP, e.machT, e.machP
	mot, rpmT, motP := e.motorT, e.rpmT, e.motorPT
	e.mu.Unlock()
	e.drone[0].set(55, 2.5, 0.02)
	e.drone[1].set(82.4, 2.2, 0.015)
	e.drone[2].set(130.8, 1.8, 0.01)
	dt := 1 / fs
	for i := 0; i < len(out)/2; i++ {
		e.ambience()
		e.hum += (ht - e.hum) * 0.0003
		e.buzz += (bt - e.buzz) * 0.002
		e.crack += (ct - e.crack) * 0.003
		e.weld += (wt - e.weld) * 0.003
		e.mach += (mt - e.mach) * 0.002
		e.hp += (hpT - e.hp) * 0.001
		e.bp += (bpT - e.bp) * 0.002
		e.wp += (wpT - e.wp) * 0.002
		e.mp += (mpT - e.mp) * 0.002
		t := e.t
		n := e.rnd()
		L, Rr, send := 0.0, 0.0, 0.0
		pan := func(s, p, g float64) {
			L += s * (0.72 - 0.28*p) * g
			Rr += s * (0.72 + 0.28*p) * g
		}
		// ambience: sub rumble + bowed-metal drone
		rl, _, _ := e.f[0].step(n, 55, 0.9)
		dn := e.drone[0].step(n*0.6) + e.drone[1].step(n*0.6) + e.drone[2].step(n*0.6)
		amb := rl*0.35 + dn*(0.6+0.4*math.Sin(t*0.13))
		L += amb * 0.5
		Rr += amb * 0.5
		send += amb * 0.2
		// generator transformer hum
		if e.hum > 1e-4 {
			h := 0.45*math.Sin(2*math.Pi*120*t) + 0.25*math.Sin(2*math.Pi*240*t+0.3) + 0.14*math.Sin(2*math.Pi*360.4*t+1.1) + 0.08*math.Sin(2*math.Pi*480*t) + 0.2*math.Sin(2*math.Pi*60*t)
			h *= 0.85 + 0.15*math.Sin(2*math.Pi*0.31*t)
			_, _, hpn := e.f[1].step(n, 4200, 0.8)
			h += hpn * math.Abs(math.Sin(2*math.Pi*60*t)) * 0.18
			pan(h*e.hum, e.hp, 1)
			send += h * e.hum * 0.2
		}
		if e.buzz > 1e-4 {
			// electronic interference, not a whoosh: crushed sample-and-hold static that
			// stutters in packets, a thin carrier squeal hopping between frequencies, and clicks
			e.ifCnt--
			if e.ifCnt <= 0 {
				e.ifN = 2 + math.Floor(math.Abs(e.rnd())*9)
				e.ifCnt = e.ifN
				e.ifHold = math.Round(e.rnd()*3) / 3 // 3-bit static
			}
			e.ifGT -= dt
			if e.ifGT <= 0 {
				e.ifGT = 0.012 + math.Abs(e.rnd())*0.05
				if math.Abs(e.rnd()) < 0.3 {
					e.ifGate = 0
				} else {
					e.ifGate = 0.35 + 0.65*math.Abs(e.rnd())
				}
			}
			e.ifHT -= dt
			if e.ifHT <= 0 {
				e.ifHT = 0.03 + math.Abs(e.rnd())*0.07
				e.ifHop = 900 + math.Abs(e.rnd())*2600
				e.ifHP = 0
				if math.Abs(e.rnd()) < 0.55 {
					e.ifHP = 1
				}
			}
			e.ifPh += e.ifHop / fs
			sq := 1.0
			if math.Mod(e.ifPh, 1) > 0.5 {
				sq = -1
			}
			_, _, st := e.f[2].step(e.ifHold, 1800, 0.7) // keep the hiss bright, lose the rumble
			b := (st*0.55 + sq*0.1*e.ifHP) * e.ifGate
			b += 0.06 * math.Sin(2*math.Pi*50*t) * math.Sin(2*math.Pi*150*t) // mains bleed
			if math.Abs(n) > 0.9975 {
				b += n * 0.7
			}
			pan(b*e.buzz*0.6, e.bp, 1)
		}
		if e.crack > 1e-4 && math.Abs(n) > 0.97 {
			L += n * e.crack * 0.7
			Rr += n * e.crack * 0.7
		}
		if e.weld > 1e-4 {
			if math.Abs(n) > 0.9985 {
				e.gate = 0.25 + 0.75*math.Abs(e.rnd())
			}
			_, _, whp := e.f[3].step(n, 2200, 0.8)
			e.ph[0] += 120 / fs
			saw := math.Mod(e.ph[0], 1)*2 - 1
			_, wb, _ := e.f[4].step(saw, 450, 0.4)
			w := (whp*0.45 + wb*0.2) * e.gate
			if math.Abs(n) > 0.995 {
				w += n * 0.6
			}
			pan(w*e.weld, e.wp, 1)
			send += w * e.weld * 0.3
		}
		e.motor += (mot - e.motor) * 0.0015
		e.rpm += (rpmT - e.rpm) * 0.00025
		e.motorP += (motP - e.motorP) * 0.002
		if e.motor > 1e-4 {
			// a four-cylinder gas engine: a fat exhaust thump on every firing (two per turn of
			// the crank), each a little different, through a throaty resonance that opens up
			// as it revs, plus valve-train chatter
			fire := math.Max(10, e.rpm/60*2)
			e.firePh += fire * (1 + e.fireJit) / fs
			if e.firePh >= 1 {
				e.firePh -= 1
				e.fireAmp = 0.65 + 0.35*math.Abs(e.rnd())
				e.fireJit = 0.06 * e.rnd()
			}
			ph := e.firePh
			pop := math.Exp(-ph*6) * e.fireAmp
			_, nb, _ := e.f[7].step(n, 220+fire*5, 0.7)
			ex := pop*(1.6-0.9*ph) + nb*pop*0.9
			lp, _, _ := e.f[8].step(ex, 90+fire*2.6, 0.92)
			_, rb, _ := e.f[9].step(ex, 2*fire+60, 0.25)
			e.ph[3] += e.rpm / 60 * 4 / fs
			tick := math.Pow(0.5+0.5*math.Sin(2*math.Pi*e.ph[3]), 12)
			_, _, vh := e.f[10].step(n, 3500, 0.6)
			m := lp*0.9 + rb*0.35 + vh*tick*0.05*math.Min(1, e.rpm/2500)
			pan(m*e.motor, e.motorP, 1)
			send += m * e.motor * 0.15
		}
		if e.mach > 1e-4 {
			e.ph[1] += (18 + 4*math.Sin(t)) / fs
			pulse := math.Pow(0.5+0.5*math.Sin(2*math.Pi*e.ph[1]), 6)
			ml, _, _ := e.f[5].step(n, 700, 0.6)
			e.ph[2] += (210 + 40*math.Sin(t*1.7)) / fs
			gear := math.Mod(e.ph[2], 1)*2 - 1
			_, gb, _ := e.f[6].step(gear, 1300, 0.25)
			m := ml*pulse*0.7 + gb*0.12
			pan(m*e.mach, e.mp, 1)
			send += m * e.mach * 0.25
		}
		live := e.voices[:0]
		for _, v := range e.voices {
			if v.age < 0 {
				v.age += dt
				live = append(live, v)
				continue
			}
			if v.age > v.dur {
				continue
			}
			s := v.sample() * v.amp
			v.a2lp(s)
			s = v.ph[5]
			pan(s, v.pan, 1)
			send += s * v.send
			v.age += dt
			live = append(live, v)
		}
		e.voices = live
		if len(e.clips) > 0 {
			for si := range e.ccur {
				c, w := &e.ccur[si], csrc[si]
				c.amp += (w.amp - c.amp) * 0.002
				c.pan += (w.pan - c.pan) * 0.002
				c.send += (w.send - c.send) * 0.002
			}
			cl := e.clips[:0]
			for _, c := range e.clips {
				if t < c.at {
					cl = append(cl, c)
					continue
				}
				k := int(c.pos)
				if k+1 >= len(c.data) {
					continue
				}
				f := c.pos - float64(k)
				x := (float64(c.data[k])*(1-f) + float64(c.data[k+1])*f) / 32768
				src := e.ccur[c.src]
				pan(x*src.amp, src.pan, 1)
				send += x * src.amp * src.send
				c.pos += clipRate / fs
				cl = append(cl, c)
			}
			e.clips = cl
		}
		rl2, rr2 := e.rev.step(send * 0.5)
		L += rl2 * 0.32
		Rr += rr2 * 0.32
		e.dcL += (L - e.dcL) * 0.0005
		e.dcR += (Rr - e.dcR) * 0.0005
		out[2*i] = math.Tanh((L - e.dcL) * 1.1)
		out[2*i+1] = math.Tanh((Rr - e.dcR) * 1.1)
		e.t += dt
	}
	e.mu.Lock()
	e.tNow = e.t
	e.mu.Unlock()
}

// one-pole distance lowpass into ph[5]
func (v *voice) a2lp(s float64) {
	v.ph[5] = s*(1-v.cut) + v.ph[5]*v.cut
}

func (e *engine) set(humVol, humPan, buzz, buzzPan, crackle float64) {
	e.mu.Lock()
	e.humT, e.humP, e.buzzT, e.buzzP, e.crackT = humVol, humPan, buzz, buzzPan, crackle
	e.mu.Unlock()
}

// motorSet: the cart's gas engine - volume 0 is off; rpm, and pan.
func (e *engine) motorSet(vol, rpm, pan float64) {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.motorT, e.rpmT, e.motorPT = vol, rpm, pan
	if vol > 0 && e.rpm < 1 {
		e.rpm = rpm
	}
	e.mu.Unlock()
}

func (e *engine) robots(weld, weldPan, mach, machPan float64) {
	e.mu.Lock()
	e.weldT, e.weldP, e.machT, e.machP = weld, weldPan, mach, machPan
	e.mu.Unlock()
}

func hash01(x float64) float64 { return math.Mod(math.Abs(math.Sin(x*91.7)*43758.5453), 1) }

func smoothstep01(x float64) float64 {
	x = math.Max(0, math.Min(1, x))
	return x * x * (3 - 2*x)
}
