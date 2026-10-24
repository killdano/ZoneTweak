package main

// The editor's undo history and snapping (shared with the engine's tool code).

func (e *editor) push() {
	e.undo = append(e.undo, clonePts(e.pts))
	if len(e.undo) > 200 {
		e.undo = e.undo[1:]
	}
	e.redo = nil
	e.dirty = true
}

func (e *editor) releaseAll() {
	for _, h := range e.hands {
		h.grab = -1
		h.rayGrab = false
	}
}

func (e *editor) doUndo() {
	if len(e.undo) == 0 {
		e.say("Nothing to undo")
		return
	}
	e.redo = append(e.redo, e.pts)
	e.pts = e.undo[len(e.undo)-1]
	e.undo = e.undo[:len(e.undo)-1]
	e.releaseAll()
	e.dirty = true
	e.say("Undo")
}

func (e *editor) doRedo() {
	if len(e.redo) == 0 {
		e.say("Nothing to redo")
		return
	}
	e.undo = append(e.undo, e.pts)
	e.pts = e.redo[len(e.redo)-1]
	e.redo = e.redo[:len(e.redo)-1]
	e.releaseAll()
	e.dirty = true
	e.say("Redo")
}

func (e *editor) snapped(i int, c pt2) pt2 {
	if !e.snap || len(e.pts) < 3 {
		return c
	}
	n := len(e.pts)
	prev, next := e.pts[(i-1+n)%n], e.pts[(i+1)%n]
	return snapToNeighbors(c, &prev, &next, e.theta)
}
