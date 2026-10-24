package main

// The arms. Only the head and hands are tracked, so the rest of each arm is worked out: the
// body faces roughly where you look (it only turns once your head has turned well past it),
// the shoulders hang off it, and each elbow lands where a real one would for that hand -
// at the bend the hand's distance from the shoulder demands, swung round to the side that
// keeps the wrist least bent for the way the hand is turned. The hand then bends at the
// wrist on its own, within what a wrist can do.

import "math"

const (
	upperArmLen = 0.30
	foreArmLen  = 0.27
	maxWristK   = 65 * math.Pi / 180 // the most a wrist bends
)

var armDebug bool // preview: show the estimated shoulders and elbows

var torso struct {
	yaw, t float64
	ok     bool
}

// torsoAxes: which way your body faces (forward and right, level).
func torsoAxes() (vec3, vec3) {
	t := fxTime()
	f := ed.hmd.axis(2).mul(-1)
	f.y = 0
	if f.length() > 0.1 {
		hy := math.Atan2(f.z, f.x)
		if !torso.ok {
			torso.yaw, torso.ok = hy, true
		} else if dt := math.Min(0.1, t-torso.t); dt > 0 {
			const dead = 30 * math.Pi / 180
			d := angDiff2(torso.yaw, hy)
			if d > dead {
				torso.yaw += d - dead
			} else if d < -dead {
				torso.yaw += d + dead
			}
			torso.yaw += angDiff2(torso.yaw, hy) * math.Min(1, dt*0.7) // and drifts round to it
		}
		torso.t = t
	}
	fwd := v3(math.Cos(torso.yaw), 0, math.Sin(torso.yaw))
	return fwd, fwd.cross(v3(0, 1, 0)).norm()
}

// shoulderOf: where that side's shoulder is (side -1 left, +1 right).
func shoulderOf(side float64) vec3 {
	hu, hb := ed.hmd.axis(1), ed.hmd.axis(2)
	neck := ed.hmd.pos().sub(hu.mul(0.11)).add(hb.mul(0.09))
	_, right := torsoAxes()
	return neck.add(v3(0, -0.15, 0)).add(right.mul(0.18 * side))
}

// elbowFor: the elbow of an arm from shoulder to wrist, the hand running along handDir
// (wrist toward the fingers).
func elbowFor(shoulder, wrist, handDir vec3, side float64) vec3 {
	fwd, right := torsoAxes()
	d := wrist.sub(shoulder)
	L := d.length()
	if L < 1e-4 {
		return wrist.sub(handDir.mul(foreArmLen))
	}
	dir := d.mul(1 / L)
	l1, l2 := upperArmLen, foreArmLen
	Lc := math.Max(math.Abs(l1-l2)+0.02, math.Min(l1+l2-0.001, L))
	a := (l1*l1 + Lc*Lc - l2*l2) / (2 * Lc)
	h := math.Sqrt(math.Max(0, l1*l1-a*a))
	C := shoulder.add(dir.mul(a + math.Max(0, L-Lc))) // out of reach: you're leaning in
	// a relaxed elbow hangs down and a little out...
	natural := projPerp(v3(0, -1, 0).add(right.mul(0.35*side)).sub(fwd.mul(0.15)), dir)
	// ...and swings round with the hand, toward where it would be with a straight wrist
	straight := projPerp(wrist.sub(handDir.mul(l2)).sub(C), dir)
	bp := v3(0, 0, 0)
	if natural.length() > 1e-4 {
		bp = natural.norm().mul(0.6)
	}
	if straight.length() > 1e-4 {
		bp = bp.add(straight.norm())
	}
	if bp.length() < 1e-4 {
		bp = right.mul(side)
	}
	return C.add(bp.norm().mul(h))
}

// limitWrist: the forearm direction (wrist toward elbow) turned just enough that the hand
// isn't bent past what a wrist can do.
func limitWrist(fd, handDir vec3) vec3 {
	along := fd.mul(-1) // the way the forearm points, elbow to wrist
	c := math.Max(-1, math.Min(1, along.dot(handDir)))
	k := math.Acos(c)
	if k <= maxWristK {
		return fd
	}
	ax := along.cross(handDir)
	if ax.length() < 1e-5 {
		ax = projPerp(v3(0, 1, 0), along)
		if ax.length() < 1e-5 {
			ax = v3(1, 0, 0)
		}
	}
	return rotAbout(along, ax.norm(), k-maxWristK).mul(-1).norm()
}

// forearmDir: which way the forearm runs from the wrist (toward the elbow), smoothed.
func (gh *gHand) forearmDir(wrist vec3) vec3 {
	_, _, Z := gh.axes()
	handDir := Z.mul(-1)
	sh := shoulderOf(gh.side)
	el := elbowFor(sh, wrist, handDir, gh.side)
	d := el.sub(wrist)
	if d.length() < 1e-3 {
		d = Z
	}
	d = limitWrist(d.norm(), handDir)
	if gh.fore == (vec3{}) {
		gh.fore = d
	}
	gh.fore = gh.fore.add(d.sub(gh.fore).mul(0.5)).norm()
	gh.shoulder, gh.elbow = sh, wrist.add(gh.fore.mul(foreArmLen))
	return gh.fore
}

// drawArmDebug: the estimated shoulder, elbow and upper arm (preview only).
func drawArmDebug(gh *gHand) {
	if !armDebug {
		return
	}
	useMat(mPaint)
	if gh.side > 0 {
		ball(ed.hmd.pos(), 0.09, rgba{0.6, 0.6, 0.65, 1})
		other := shoulderOf(-1)
		cyl(&bEnv, gh.shoulder, other, 0.025, 0.025, 10, rgba{0.5, 0.5, 0.55, 1}, true)
	}
	ball(gh.shoulder, 0.03, rgba{0.9, 0.2, 0.2, 1})
	ball(gh.elbow, 0.025, rgba{0.9, 0.8, 0.2, 1})
	cyl(&bEnv, gh.shoulder, gh.elbow, 0.02, 0.02, 10, rgba{0.5, 0.5, 0.55, 1}, false)
}
