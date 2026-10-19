package main

import "testing"

func TestSpaces(t *testing.T) {
	l := spaceLib{dir: t.TempDir()}
	pts := []pt2{{0, 0}, {2, 0}, {2, 3}, {0, 3}}
	if err := l.save("Door Open", "31", pts, 3, "raw"); err != nil {
		t.Fatal(err)
	}
	if err := l.save("door closed", "31", pts[:3], 3, "raw"); err != nil {
		t.Fatal(err)
	}
	if err := l.save("Door Open", "31", append(pts, pt2{-1, 1}), 3, "raw"); err != nil {
		t.Fatal(err)
	}
	ls := l.list()
	if len(ls) != 2 || ls[0].Name != "door closed" || len(ls[1].Points) != 5 {
		t.Fatalf("%+v", ls)
	}
	p, ok := l.find("DOOR OPEN")
	if !ok || len(p.pts()) != 5 {
		t.Fatal("find")
	}
	l.setActive("Door Open")
	if l.active() != "Door Open" {
		t.Fatal("active")
	}
	if l.remove("door open") != nil || len(l.list()) != 1 {
		t.Fatal("remove")
	}
	if l.save("  ", "31", pts, 3, "raw") == nil {
		t.Fatal("empty name accepted")
	}
}
