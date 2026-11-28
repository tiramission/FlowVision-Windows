package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestPointerPositionReadWhileBuilding covers the pattern the grid relies on: a
// view that reads the pointer while it builds, rather than inside a Draw
// callback, sees each move. Read in Draw, the framework only repaints the last
// frame's elements, so anything that follows the pointer — a hover mark, a name
// pill — waits for some other reason to redraw.
func TestPointerPositionReadWhileBuilding(t *testing.T) {
	var over bool
	var lastX float32
	tt := ui.NewTester(func(c *ui.Context) {
		box := ui.Box(c).Fill()
		if x, _, ok := box.PointerPosition(); ok {
			over, lastX = true, x
		} else {
			over = false
		}
	}, 400, 300)
	tt.Frame()
	if over {
		t.Fatal("the pointer starts off the box")
	}

	// Inside.
	tt.Move(200, 150)
	tt.Frame()
	if !over {
		t.Fatal("the pointer over the box was not seen while building")
	}

	// Moving within the box is seen too, which is what a hover mark needs.
	tt.Move(120, 150)
	tt.Frame()
	if !over || lastX < 100 || lastX > 140 {
		t.Fatalf("a move inside the box was not seen: over=%v x=%.1f", over, lastX)
	}

	// And leaving.
	tt.Move(600, 400)
	tt.Frame()
	if over {
		t.Fatal("the pointer leaving the box was not seen")
	}
}
