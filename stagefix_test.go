package main

import (
	"math"
	"testing"
)

// The runtime re-centres: the boundary file now has your room turned and shifted in its
// coordinates. The game follows it - poses and eye views alike - and doesn't move again when
// the file's rewritten with the same room.
func TestStageFollow(t *testing.T) {
	defer func() { stageFix, stageFixOn, roomWorld = identity2(), false, nil }()
	roomWorld = []pt2{{-1.5, -0.8}, {1.2, -1.0}, {1.4, 1.6}, {-0.2, 2.1}, {-1.6, 1.0}}
	moved := rigidFromYaw(0.5, 0.7, -0.3) // the runtime's new stage, relative to the old
	now := moved.inv().all(roomWorld)
	if !followStage(now) {
		t.Fatal("didn't follow the move")
	}
	for i, p := range now {
		q := stageFix.apply(p)
		if math.Hypot(q[0]-roomWorld[i][0], q[1]-roomWorld[i][1]) > 1e-6 {
			t.Fatalf("corner %d lands at %v, want %v", i, q, roomWorld[i])
		}
	}
	if followStage(now) {
		t.Fatal("the same file again shouldn't move it")
	}
	// a tracked point and an eye view agree
	pose := rigid34(rigidFromYaw(0.2, now[2][0], now[2][1]))
	pose[1][3] = 1.6
	w := fixPose(pose)
	if math.Hypot(float64(w[0][3])-roomWorld[2][0], float64(w[2][3])-roomWorld[2][1]) > 1e-4 {
		t.Fatalf("a pose at corner 2 should land on the game's corner 2: %v", w.pos())
	}
	v := fixView(inv34(pose))
	iv := inv34(w)
	for r := 0; r < 3; r++ {
		for c := 0; c < 4; c++ {
			if math.Abs(float64(v[r][c]-iv[r][c])) > 1e-4 {
				t.Fatalf("eye view doesn't match the pose: %v vs %v", v, iv)
			}
		}
	}
	// the runtime's own event: the stage moved again, and the file agrees after
	prev := stageFix
	more := rigidFromYaw(-0.3, 0.2, 0.4)
	ev := rigid34(more)
	stageMoved(ev)
	now2 := more.inv().all(now)
	if followStage(now2) {
		t.Fatal("after the event, the rewritten file should already match")
	}
	_ = prev
	// a different shape: not followed
	if followStage([]pt2{{0, 0}, {3, 0}, {3, 3}}) {
		t.Fatal("a different boundary shouldn't be taken as a move")
	}
}
