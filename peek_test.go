package main

import (
	"image"
	"testing"

	"github.com/egoist/mygo/ui"
)

// peekApp prepares a viewer showing a small picture, zoomed out from the fit
// so that a jump to 100% is a visible change.
func peekApp(t *testing.T) (*App, *ui.Tester) {
	t.Helper()
	a := newViewerGestureApp(t)
	tt := ui.NewTester(a.view, 900, 700)
	tt.Frame()
	a.viewer.Open = true
	a.viewer.Bitmap = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 64, 64)))
	a.viewer.Zoom = 0.05
	a.viewer.PanX = 12
	a.viewer.PanY = -7
	a.viewer.OneToOne = false
	return a, tt
}

// TestViewerPeekRestoresTheView is the point of the gesture: a long press looks
// at the pixels, and letting go puts the picture back exactly where it was.
func TestViewerPeekRestoresTheView(t *testing.T) {
	a, _ := peekApp(t)
	defer a.closeViewer()

	before := struct {
		zoom, panX, panY, angle float32
		oneToOne                bool
	}{a.viewer.Zoom, a.viewer.PanX, a.viewer.PanY, a.viewer.Angle, a.viewer.OneToOne}

	a.beginViewerPeek(400, 300, 800, 600)
	if !a.viewer.Peeking {
		t.Fatal("the peek did not start")
	}
	if a.viewer.Zoom == before.zoom && a.viewer.PanX == before.panX && a.viewer.PanY == before.panY {
		t.Fatal("the peek did not change the view")
	}

	a.endViewerPeek()
	if a.viewer.Peeking {
		t.Fatal("the peek did not end")
	}
	if a.viewer.Zoom != before.zoom || a.viewer.PanX != before.panX || a.viewer.PanY != before.panY {
		t.Fatalf("view came back as zoom %v pan %v,%v, want %v %v,%v",
			a.viewer.Zoom, a.viewer.PanX, a.viewer.PanY, before.zoom, before.panX, before.panY)
	}
	if a.viewer.Angle != before.angle || a.viewer.OneToOne != before.oneToOne {
		t.Fatal("rotation or the actual-size flag was left behind")
	}
}

// TestViewerPeekPansWhileHeld covers the "and can drag it" half: moving the
// pointer during a peek moves the picture instead of cancelling it.
func TestViewerPeekPansWhileHeld(t *testing.T) {
	a, _ := peekApp(t)
	defer a.closeViewer()

	a.beginViewerPeek(400, 300, 800, 600)
	panX, panY := a.viewer.PanX, a.viewer.PanY
	a.dragViewer(20, -15, 800, 600)

	if a.viewer.PanX == panX && a.viewer.PanY == panY {
		t.Fatal("a drag during the peek did not move the picture")
	}
	if !a.viewer.Peeking {
		t.Fatal("a drag during the peek cancelled it")
	}
}

// TestViewerPeekEndsOnRelease covers the wiring: the release that ends the
// press also ends the peek, so the picture cannot stay at 100%.
func TestViewerPeekEndsOnRelease(t *testing.T) {
	a, _ := peekApp(t)
	defer a.closeViewer()

	zoom := a.viewer.Zoom
	a.viewer.HoldActive = true
	a.viewer.HoldButton = 0
	a.beginViewerPeek(400, 300, 800, 600)
	a.endViewerHold(0, false)

	if a.viewer.Peeking {
		t.Fatal("the release did not end the peek")
	}
	if a.viewer.Zoom != zoom {
		t.Fatalf("zoom came back as %v, want %v", a.viewer.Zoom, zoom)
	}
}

// TestViewerHoldModeDefaultsToPeek pins the default: a new install gets the
// glance gesture, which is what the setting describes.
func TestViewerHoldModeDefaultsToPeek(t *testing.T) {
	s := defaultSettings()
	if s.ViewerHoldZoom != "peek" {
		t.Fatalf("default hold mode is %q, want peek", s.ViewerHoldZoom)
	}
	// An unknown value falls back rather than leaving the gesture dead.
	got, ok := parseSettings([]byte(`{"viewerHoldZoom":"nonsense"}`))
	if !ok {
		t.Fatal("the document did not parse")
	}
	if got.ViewerHoldZoom != "peek" {
		t.Fatalf("unknown mode parsed as %q, want peek", got.ViewerHoldZoom)
	}
	// A value that was saved is kept.
	got, _ = parseSettings([]byte(`{"viewerHoldZoom":"jump"}`))
	if got.ViewerHoldZoom != "jump" {
		t.Fatalf("saved mode parsed as %q, want jump", got.ViewerHoldZoom)
	}
}

// TestViewerFitPeekRestoresTheView covers the secondary button: a long press
// fits the whole picture while it is held and the release puts the view back.
func TestViewerFitPeekRestoresTheView(t *testing.T) {
	a, _ := peekApp(t)
	defer a.closeViewer()

	// Zoom in first, so fitting is a visible change.
	a.viewer.Zoom = 3
	a.viewer.PanX, a.viewer.PanY = 25, -18
	a.viewer.OneToOne = true
	before := struct {
		zoom, panX, panY float32
		oneToOne         bool
	}{a.viewer.Zoom, a.viewer.PanX, a.viewer.PanY, a.viewer.OneToOne}

	a.beginViewerFitPeek()
	if !a.viewer.Peeking {
		t.Fatal("the fit peek did not start")
	}
	if a.viewer.Zoom != 1 || a.viewer.PanX != 0 || a.viewer.PanY != 0 || a.viewer.OneToOne {
		t.Fatalf("fit peek left zoom %v pan %v,%v oneToOne %v",
			a.viewer.Zoom, a.viewer.PanX, a.viewer.PanY, a.viewer.OneToOne)
	}

	a.endViewerPeek()
	if a.viewer.Zoom != before.zoom || a.viewer.PanX != before.panX || a.viewer.PanY != before.panY {
		t.Fatalf("view came back as zoom %v pan %v,%v, want %v %v,%v",
			a.viewer.Zoom, a.viewer.PanX, a.viewer.PanY, before.zoom, before.panX, before.panY)
	}
	if a.viewer.OneToOne != before.oneToOne {
		t.Fatal("the actual-size flag was left behind")
	}
}

// TestViewerFitPeekEndsOnTheRightButton covers the wiring: the secondary button
// that started the fit peek is the one whose release ends it, so a primary
// release in between cannot end it early.
func TestViewerFitPeekEndsOnTheRightButton(t *testing.T) {
	a, _ := peekApp(t)
	defer a.closeViewer()

	a.viewer.Zoom = 3
	a.beginViewerFitPeek()

	// A primary release must not end a peek the secondary button started.
	a.endViewerHold(0, false)
	if !a.viewer.Peeking {
		t.Fatal("the wrong button ended the peek")
	}
	a.endViewerHold(1, false)
	if a.viewer.Peeking {
		t.Fatal("the right button did not end the peek")
	}
	if a.viewer.Zoom != 3 {
		t.Fatalf("zoom came back as %v, want 3", a.viewer.Zoom)
	}
}

// TestViewerHoldFitDefaultsToPeek pins the default for the secondary gesture.
func TestViewerHoldFitDefaultsToPeek(t *testing.T) {
	s := defaultSettings()
	if s.ViewerHoldFit != "peek" {
		t.Fatalf("default fit mode is %q, want peek", s.ViewerHoldFit)
	}
	got, ok := parseSettings([]byte(`{"viewerHoldFit":"nonsense"}`))
	if !ok {
		t.Fatal("the document did not parse")
	}
	if got.ViewerHoldFit != "peek" {
		t.Fatalf("unknown fit mode parsed as %q, want peek", got.ViewerHoldFit)
	}
	got, _ = parseSettings([]byte(`{"viewerHoldFit":"fit"}`))
	if got.ViewerHoldFit != "fit" {
		t.Fatalf("saved fit mode parsed as %q, want fit", got.ViewerHoldFit)
	}
}
