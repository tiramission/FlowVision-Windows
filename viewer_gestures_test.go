package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func newViewerGestureApp(t *testing.T) *App {
	t.Helper()
	a := newViewTestApp()
	dir := t.TempDir()
	path := filepath.Join(dir, "a.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	img.Set(40, 40, color.RGBA{R: 230, G: 80, B: 70, A: 255})
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	a1 := &Item{ID: "1", Name: "a.png", Kind: "image", Path: path, Size: 1, W: 100, H: 100}
	a2 := &Item{ID: "2", Name: "b.png", Kind: "image", Path: path, Size: 1, W: 100, H: 100}
	tab := a.tab()
	tab.All = []*Item{a1, a2}
	tab.Items = tab.All
	a.viewer = Viewer{
		Open:     true,
		Item:     a1,
		Index:    0,
		Bitmap:   ui.NewBitmap(img),
		Zoom:     1,
		ShowInfo: false,
	}
	return a
}

func canvasPoint(t *testing.T, tt *ui.Tester) (float32, float32) {
	t.Helper()
	r, ok := tt.Find("媒体查看器")
	if !ok {
		t.Fatal("viewer canvas missing")
	}
	return r.X + r.W/2, r.Y + r.H/2
}

func TestViewerLongPressLeftActualSize(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	tt := ui.NewTester(a.view, 900, 700)
	tt.Frame()
	x, y := canvasPoint(t, tt)
	a.beginViewerHold(0, x, y, tt.Now().Add(-viewerHoldDelay))
	tt.Frame()
	if a.viewer.HoldActive {
		t.Fatal("long-press still armed after firing")
	}
	if a.viewer.Zoom >= 0.5 || a.viewer.PanX != 0 || a.viewer.PanY != 0 {
		t.Fatalf("left hold zoom = %.3f pan = %.1f,%.1f, want actual size",
			a.viewer.Zoom, a.viewer.PanX, a.viewer.PanY)
	}
}

func TestViewerLongPressRightFit(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewer.Zoom = 0.2
	a.viewer.Angle = 90
	a.viewer.PanX, a.viewer.PanY = 100, -40
	tt := ui.NewTester(a.view, 900, 700)
	tt.Frame()
	x, y := canvasPoint(t, tt)
	a.beginViewerHold(1, x, y, tt.Now().Add(-viewerHoldDelay))
	tt.Frame()
	if a.viewer.Zoom != 1 || a.viewer.Angle != 90 || a.viewer.PanX != 0 || a.viewer.PanY != 0 {
		t.Fatalf("right hold zoom = %.3f angle = %.1f pan = %.1f,%.1f, want fit",
			a.viewer.Zoom, a.viewer.Angle, a.viewer.PanX, a.viewer.PanY)
	}
}

func TestViewerHoldCancelsOnDrag(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	tt := ui.NewTester(a.view, 900, 700)
	tt.Frame()
	x, y := canvasPoint(t, tt)
	a.beginViewerHold(0, x, y, tt.Now())
	a.moveViewerHold(x+viewerHoldSlop+4, y)
	if a.viewer.HoldActive {
		t.Fatal("drag did not cancel long press")
	}
	if a.viewer.Zoom != 1 {
		t.Fatalf("drag during hold changed zoom to %.3f", a.viewer.Zoom)
	}
}

func TestViewerHoldShortClickCancels(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.beginViewerHold(1, 100, 100, time.Now())
	a.endViewerHold(1, false)
	if a.viewer.HoldActive {
		t.Fatal("release did not cancel long press")
	}
	a.beginViewerHold(0, 100, 100, time.Now())
	a.endViewerHold(0, true)
	if a.viewer.HoldActive {
		t.Fatal("canceled release did not cancel long press")
	}
}

func TestViewerSideButtonsStepImages(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewer.Index = 1
	a.viewer.Item = a.tab().Items[1]
	tt := ui.NewTester(a.view, 900, 700)
	tt.Frame()

	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if a.viewer.Index != 0 || a.viewer.Item.ID != "1" {
		t.Fatalf("back side button went to index %d item %v", a.viewer.Index, a.viewer.Item)
	}
	tt.Key(0, ui.KeyForward)
	tt.Frame()
	if a.viewer.Index != 1 || a.viewer.Item.ID != "2" {
		t.Fatalf("forward side button went to index %d item %v", a.viewer.Index, a.viewer.Item)
	}
}

func TestWindowVerticalEdgeHit(t *testing.T) {
	type test struct {
		name       string
		y          int32
		wantTop    bool
		wantBottom bool
	}
	top, bottom, band := int32(0), int32(1000), int32(64)
	for _, tc := range []test{
		{name: "top", y: 10, wantTop: true},
		{name: "bottom", y: 990, wantBottom: true},
		{name: "middle", y: 500},
		{name: "top boundary", y: 63, wantTop: true},
		{name: "past top boundary", y: 64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotTop, gotBottom := windowVerticalEdgeHit(tc.y, top, bottom, band)
			if gotTop != tc.wantTop || gotBottom != tc.wantBottom {
				t.Fatalf("hit = %v,%v want %v,%v", gotTop, gotBottom, tc.wantTop, tc.wantBottom)
			}
		})
	}
	if topHit, bottomHit := windowVerticalEdgeHit(500, 0, 1000, 800); topHit || !bottomHit {
		t.Fatal("oversized edge band should be capped to half the window")
	}
}
