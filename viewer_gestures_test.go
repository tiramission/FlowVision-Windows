package main

import (
	"image"
	"image/color"
	"image/gif"
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
		Open:          true,
		Item:          a1,
		Index:         0,
		Bitmap:        ui.NewBitmap(img),
		Zoom:          1,
		ShowInfo:      false,
		Immersive:     true,
		CursorVisible: true,
		CursorUntil:   time.Now().Add(viewerCursorIdle),
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

func canvasLocalPoint(t *testing.T, tt *ui.Tester) (float32, float32, float32, float32) {
	t.Helper()
	r, ok := tt.Find("媒体查看器")
	if !ok {
		t.Fatal("viewer canvas missing")
	}
	return r.W / 2, r.H / 2, r.W, r.H
}

func TestViewerLongPressLeftZoomsAtPointer(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	tt := ui.NewTester(a.view, 900, 700)
	tt.Frame()
	_, _, w, h := canvasLocalPoint(t, tt)
	x, y := w/2+20, h/2-15
	a.viewer.Zoom = 0.1
	a.viewer.PanX, a.viewer.PanY = 0, 0
	bw, bh := a.viewer.Bitmap.Size()
	actual := viewerActualZoom(viewerFitScale(bw, bh, a.viewer.Angle, w, h))
	wantZoom, wantPanX, wantPanY := viewerZoomAtPoint(a.viewer.Zoom, 0, 0, actual/a.viewer.Zoom, x, y, w, h, viewerFitScale(bw, bh, a.viewer.Angle, w, h))
	a.beginViewerHold(0, x, y, tt.Now().Add(-viewerHoldDelay))
	tt.Frame()
	if a.viewer.HoldActive {
		t.Fatal("long-press still armed after firing")
	}
	if absFloat(a.viewer.Zoom-wantZoom) > 0.001 ||
		absFloat(a.viewer.PanX-wantPanX) > 0.5 ||
		absFloat(a.viewer.PanY-wantPanY) > 0.5 {
		t.Fatalf("left hold zoom = %.3f pan = %.1f,%.1f; want %.3f %.1f,%.1f",
			a.viewer.Zoom, a.viewer.PanX, a.viewer.PanY, wantZoom, wantPanX, wantPanY)
	}
	if a.viewer.PanX == 0 && a.viewer.PanY == 0 {
		t.Fatal("pointer-anchored zoom did not move the image")
	}
}

func TestViewerLongPressLeftStopsAtHundredPercent(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	tt := ui.NewTester(a.view, 900, 700)
	tt.Frame()
	x, y, _, _ := canvasLocalPoint(t, tt)
	a.viewer.Zoom = 1
	a.viewer.PanX, a.viewer.PanY = 24, -16
	before := a.viewer
	a.beginViewerHold(0, x, y, tt.Now().Add(-viewerHoldDelay))
	tt.Frame()
	if a.viewer.Zoom != before.Zoom || a.viewer.PanX != before.PanX || a.viewer.PanY != before.PanY {
		t.Fatalf("long press at 100%% changed zoom/pan: %.3f %.1f,%.1f",
			a.viewer.Zoom, a.viewer.PanX, a.viewer.PanY)
	}
}

func TestViewerLongPressRightFitsAndCenters(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewer.Zoom = 0.4
	a.viewer.Angle = 90
	a.viewer.PanX, a.viewer.PanY = 100, -40
	tt := ui.NewTester(a.view, 900, 700)
	tt.Frame()
	// The pointer is deliberately off-center; the right gesture must still
	// fit the image and center it without anchoring to this position.
	a.beginViewerHold(1, 12, 13, tt.Now().Add(-viewerHoldDelay))
	tt.Frame()
	if a.viewer.Zoom != 1 || a.viewer.Angle != 90 || a.viewer.PanX != 0 || a.viewer.PanY != 0 {
		t.Fatalf("right hold zoom = %.3f angle = %.1f pan = %.1f,%.1f, want fit and center",
			a.viewer.Zoom, a.viewer.Angle, a.viewer.PanX, a.viewer.PanY)
	}
}

func absFloat(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
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

func TestViewerCursorShowsWithoutChrome(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewer.ChromeVisible = false
	a.viewer.CursorVisible = false
	a.showViewerCursor()
	if !a.viewer.CursorVisible {
		t.Fatal("pointer move did not restore the cursor")
	}
	if a.viewer.ChromeVisible {
		t.Fatal("restoring the cursor also brought back the floating bars")
	}
	if !a.viewer.CursorUntil.After(time.Now()) {
		t.Fatal("pointer move did not extend the cursor idle deadline")
	}
}

func TestViewerPointerMoveRestoresHiddenCursor(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	tt := ui.NewTester(a.view, 900, 700)
	tt.Frame()
	x, y := canvasPoint(t, tt)
	a.viewer.ChromeVisible = false
	a.viewer.CursorVisible = false
	a.viewer.CursorUntil = time.Time{}
	tt.Move(x+1, y)
	tt.Frame()
	if !a.viewer.CursorVisible {
		t.Fatal("moving the pointer over the canvas did not restore the cursor")
	}
	if a.viewer.ChromeVisible {
		t.Fatal("pointer move over the middle of the canvas showed the floating bars")
	}
}

func TestViewerPhysicalPointerMovementRestoresCursor(t *testing.T) {
	if !viewerCursorMoved(true, 10, 20, 11, 20) {
		t.Fatal("physical pointer movement not detected")
	}
	if viewerCursorMoved(true, 10, 20, 10, 20) {
		t.Fatal("unchanged physical pointer reported as movement")
	}
	if viewerCursorMoved(false, 10, 20, 11, 20) {
		t.Fatal("first tracked sample should only establish the baseline")
	}
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewer.CursorVisible = false
	if !a.wakeViewerCursor() {
		t.Fatal("wakeViewerCursor did not report a hidden cursor restored")
	}
	if !a.viewer.CursorVisible {
		t.Fatal("wakeViewerCursor left the cursor hidden")
	}
	if !a.viewer.CursorUntil.After(time.Now()) {
		t.Fatal("wakeViewerCursor did not extend the cursor deadline")
	}
}

func TestVideoViewerPointerMoveRestoresHiddenCursor(t *testing.T) {
	a := newViewTestApp()
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.gif")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	palette := color.Palette{color.RGBA{20, 20, 20, 255}, color.RGBA{230, 80, 70, 255}}
	frames := make([]*image.Paletted, 0, 2)
	for _, c := range []color.Color{palette[0], palette[1]} {
		frame := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
		for i := range frame.Pix {
			if c == palette[1] {
				frame.Pix[i] = 1
			}
		}
		frames = append(frames, frame)
	}
	if err := gif.EncodeAll(f, &gif.GIF{Image: frames, Delay: []int{10, 10}, LoopCount: 0}); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := ui.NewGIFDecoder(data)
	if err != nil {
		t.Fatal(err)
	}
	player, err := ui.NewVideoPlayer(decoder)
	if err != nil {
		t.Fatal(err)
	}
	item := &Item{ID: "v", Name: "clip.gif", Kind: "video", Path: path, Size: int64(len(data)), W: 2, H: 2}
	tab := a.tab()
	tab.All = []*Item{item}
	tab.Items = tab.All
	a.viewer = Viewer{
		Open:          true,
		Item:          item,
		Index:         0,
		Immersive:     true,
		CursorVisible: false,
		CursorUntil:   time.Time{},
		Player:        player,
	}
	defer a.closeViewer()
	tt := ui.NewTester(a.view, 900, 700)
	tt.Move(450, 350)
	tt.Frame()
	if !a.viewer.CursorVisible {
		t.Fatal("viewer cursor did not start visible")
	}
	if tt.Cursor() != ui.CursorDefault {
		t.Fatalf("visible viewer cursor style = %v, want CursorDefault", tt.Cursor())
	}
	a.viewer.CursorVisible = false
	a.viewer.CursorUntil = time.Time{}
	tt.Frame()
	if tt.Cursor() != ui.CursorNone {
		t.Fatalf("hidden viewer cursor style = %v, want CursorNone", tt.Cursor())
	}
	tt.Move(452, 352)
	tt.Frame()
	if !a.viewer.CursorVisible {
		t.Fatal("pointer movement over the video did not restore the cursor")
	}
	if tt.Cursor() != ui.CursorDefault {
		t.Fatalf("restored viewer cursor style = %v, want CursorDefault", tt.Cursor())
	}
}

func TestViewerCursorIdleIsIndependentFromChrome(t *testing.T) {
	now := time.Now()
	if viewerCursorShouldHide(true, true, now, now.Add(time.Second)) {
		t.Fatal("cursor hid before its own deadline")
	}
	if !viewerCursorShouldHide(true, true, now, now) {
		t.Fatal("cursor did not hide at its own deadline")
	}
	if viewerCursorShouldHide(false, true, now, now) {
		t.Fatal("windowed viewer should keep the cursor visible")
	}
}

func TestViewerImmersiveToggleKeepsChromeIndependent(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewer.Immersive = true
	a.viewerToggleImmersive()
	if a.viewer.Immersive {
		t.Fatal("immersive toggle did not leave immersive mode")
	}
	if !a.viewer.CursorVisible {
		t.Fatal("leaving immersive mode hid the cursor")
	}
	a.viewerToggleImmersive()
	if !a.viewer.Immersive {
		t.Fatal("immersive toggle did not re-enter immersive mode")
	}
	if a.viewer.ChromeVisible {
		t.Fatal("immersive mode should start with hidden floating bars")
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
