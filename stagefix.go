package main

// Following the play space when the runtime re-centres it under us. The game's world is fixed
// at launch: your boundary as it was then. If the headset's tracking origin moves later (Pimax
// re-centres when you put the headset back on, say), the runtime's STAGE coordinates of your
// real room change - Pimax's own boundary follows, the game wouldn't. stageFix maps the
// runtime's stage onto the game's world; it's worked out from the boundary file whenever the
// runtime rewrites it (the same corners, re-expressed), and from the runtime's own "stage
// moved" event when it sends one. Every tracked pose goes through it, and so do the eye views.

import (
	"log"
	"math"
)

var (
	stageFix   = rigid2{a: 1, d: 1} // stage -> world (on the floor plane)
	roomWorld  []pt2                // the room as the game has it, in the boundary file's order
	stageFixOn bool
)

// fixPose: a tracked pose from the runtime's stage into the game's world.
func fixPose(m hmdMatrix34) hmdMatrix34 {
	if !stageFixOn {
		return m
	}
	return mul34(rigid34(stageFix), m)
}

// fixView: an eye's view (stage to eye) for the game's world (world to eye).
func fixView(v hmdMatrix34) hmdMatrix34 {
	if !stageFixOn {
		return v
	}
	return mul34(v, rigid34(stageFix.inv()))
}

// rigid34: a floor-plane rigid transform as a 3D one (y untouched).
func rigid34(r rigid2) hmdMatrix34 {
	return hmdMatrix34{
		{float32(r.a), 0, float32(r.b), float32(r.tx)},
		{0, 1, 0, 0},
		{float32(r.c), 0, float32(r.d), float32(r.tz)},
	}
}

// fitRigid2: the rigid transform taking a's points onto b's (matched by index), and the worst
// leftover error.
func fitRigid2(a, b []pt2) (rigid2, float64) {
	n := len(a)
	if n == 0 || n != len(b) {
		return identity2(), math.Inf(1)
	}
	var ca, cb pt2
	for i := range a {
		ca[0], ca[1] = ca[0]+a[i][0]/float64(n), ca[1]+a[i][1]/float64(n)
		cb[0], cb[1] = cb[0]+b[i][0]/float64(n), cb[1]+b[i][1]/float64(n)
	}
	var sd, sc float64
	for i := range a {
		ax, az := a[i][0]-ca[0], a[i][1]-ca[1]
		bx, bz := b[i][0]-cb[0], b[i][1]-cb[1]
		sd += ax*bx + az*bz
		sc += ax*bz - az*bx
	}
	th := math.Atan2(sc, sd)
	c, s := math.Cos(th), math.Sin(th)
	r := rigid2{a: c, b: -s, c: s, d: c}
	r.tx = cb[0] - (r.a*ca[0] + r.b*ca[1])
	r.tz = cb[1] - (r.c*ca[0] + r.d*ca[1])
	worst := 0.0
	for i := range a {
		q := r.apply(a[i])
		worst = math.Max(worst, math.Hypot(q[0]-b[i][0], q[1]-b[i][1]))
	}
	return r, worst
}

// followStage: the boundary file says your room is now at these stage coordinates (file order).
// If that's the same room moved, the game follows it. Returns whether the mapping changed.
func followStage(nowStage []pt2) bool {
	if len(roomWorld) < 3 {
		return false
	}
	// already where it should be (the runtime's event got there first, or nothing moved)
	if len(nowStage) == len(roomWorld) {
		worst := 0.0
		for i, p := range nowStage {
			q := stageFix.apply(p)
			worst = math.Max(worst, math.Hypot(q[0]-roomWorld[i][0], q[1]-roomWorld[i][1]))
		}
		if worst < 0.03 {
			return false
		}
	}
	r, worst := fitRigid2(nowStage, roomWorld)
	if worst > 0.05 {
		log.Printf("boundary changed shape (%d points, off by %.2f m): not following it - restart the round to use it", len(nowStage), worst)
		return false
	}
	stageFix, stageFixOn = r, true
	log.Printf("play space moved: following it (turn %.1f deg, shift %.2f, %.2f)", math.Atan2(r.c, r.a)*180/math.Pi, r.tx, r.tz)
	return true
}

// stageMoved: the runtime says the stage moved: oldFromNew is the new stage in the old one.
func stageMoved(oldFromNew hmdMatrix34) {
	x, z := float64(oldFromNew[0][3]), float64(oldFromNew[2][3])
	yaw := math.Atan2(float64(oldFromNew[2][0]), float64(oldFromNew[0][0]))
	t := rigidFromYaw(yaw, x, z)
	stageFix, stageFixOn = t.then(stageFix), true
	log.Printf("stage moved: following it (turn %.1f deg, shift %.2f, %.2f)", yaw*180/math.Pi, x, z)
}
