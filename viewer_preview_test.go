package main

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestViewerPreviewFastDirectAndDiskCache(t *testing.T) {
	thumbDir = t.TempDir()
	previewDir = thumbDir
	t.Setenv("FLOWVISION_VIEWER_PREVIEW", "fast")
	t.Setenv("FLOWVISION_VIEWER_SCALER", "fast")
	dir := t.TempDir()
	path := filepath.Join(dir, "large.jpg")
	src := image.NewRGBA(image.Rect(0, 0, 3200, 2400))
	for y := 0; y < 2400; y++ {
		for x := 0; x < 3200; x++ {
			src.SetRGBA(x, y, color.RGBA{R: uint8(x / 13), G: uint8(y / 10), B: uint8((x + y) / 25), A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(f, src, &jpeg.Options{Quality: 92}); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	it := &Item{ID: "preview-test", Name: "large.jpg", Kind: "image", Path: path, Size: st.Size(), Mtime: 42, W: 3200, H: 2400}

	res, err := loadViewerPreview(it)
	if err != nil {
		t.Fatal(err)
	}
	if res.Bmp == nil {
		t.Fatal("no bitmap")
	}
	w, h := res.Bmp.Size()
	if w != 2560 || h != 1920 {
		t.Fatalf("preview size = %dx%d, want 2560x1920", w, h)
	}
	if res.CacheHit {
		t.Fatal("first load unexpectedly hit the cache")
	}
	res.Bmp.ReleasePixels()

	cachePath := viewerPreviewCachePath(it, "fast")
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(cachePath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("preview cache was not written")
		}
		time.Sleep(20 * time.Millisecond)
	}

	res2, err := loadViewerPreview(it)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Bmp == nil {
		t.Fatal("no cached bitmap")
	}
	if !res2.CacheHit {
		t.Fatal("second load did not hit the cache")
	}
	w, h = res2.Bmp.Size()
	if w != 2560 || h != 1920 {
		t.Fatalf("cached preview size = %dx%d", w, h)
	}
	res2.Bmp.ReleasePixels()
}

func TestViewerPreviewSlotReusesBitmap(t *testing.T) {
	a := &App{}
	first := image.NewRGBA(image.Rect(0, 0, 2560, 1707))
	for i := range first.Pix {
		first.Pix[i] = 0x7f
	}
	a.attachViewerPreview(first)
	bmp := a.viewer.Bitmap
	if bmp == nil {
		t.Fatal("first attach created no bitmap")
	}
	second := image.NewRGBA(image.Rect(0, 0, 2560, 1707))
	for i := range second.Pix {
		second.Pix[i] = 0x33
	}
	a.attachViewerPreview(second)
	if a.viewer.Bitmap != bmp {
		t.Fatal("second attach replaced the viewer bitmap instead of reusing the slot")
	}
	if a.viewer.BitmapOwned {
		t.Fatal("slot bitmap must be app-owned, not released by closeViewer")
	}
	w, h := bmp.Size()
	if w != 2560 || h != 1707 {
		t.Fatalf("slot size = %dx%d", w, h)
	}
}

func TestViewerPreviewHQAndFastDiffer(t *testing.T) {
	thumbDir = t.TempDir()
	previewDir = thumbDir
	t.Setenv("FLOWVISION_VIEWER_PREVIEW", "hq")
	t.Setenv("FLOWVISION_VIEWER_SCALER", "hq")
	t.Setenv("FLOWVISION_VIEWER_CACHE", "off")
	dir := t.TempDir()
	path := filepath.Join(dir, "photo.jpg")
	src := image.NewRGBA(image.Rect(0, 0, 3000, 2000))
	for y := 0; y < 2000; y++ {
		for x := 0; x < 3000; x++ {
			src.SetRGBA(x, y, color.RGBA{R: uint8((x * 7) % 255), G: uint8((y * 11) % 255), B: uint8((x * y) % 255), A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(f, src, &jpeg.Options{Quality: 92}); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	_ = f.Close()
	st, _ := os.Stat(path)
	it := &Item{ID: "hq-test", Name: "photo.jpg", Kind: "image", Path: path, Size: st.Size(), Mtime: 43, W: 3000, H: 2000}

	hq, err := loadViewerPreview(it)
	if err != nil {
		t.Fatal(err)
	}
	if hq.Bmp == nil {
		t.Fatal("hq bitmap nil")
	}
	hqSize := func() (int, int) { return hq.Bmp.Size() }
	w, h := hqSize()
	hq.Bmp.ReleasePixels()

	t.Setenv("FLOWVISION_VIEWER_PREVIEW", "fast")
	t.Setenv("FLOWVISION_VIEWER_SCALER", "fast")
	fast, err := loadViewerPreview(it)
	if err != nil {
		t.Fatal(err)
	}
	if fast.Bmp == nil {
		t.Fatal("fast bitmap nil")
	}
	fw, fh := fast.Bmp.Size()
	if fw != w || fh != h {
		t.Fatalf("fast %dx%d vs hq %dx%d", fw, fh, w, h)
	}
	fast.Bmp.ReleasePixels()
}

func TestViewerFullResSkipsSmallImage(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewer.Item.W, a.viewer.Item.H = 100, 100
	a.requestViewerFullRes()
	if a.viewer.FullResLoading || a.viewer.FullResReady || a.viewer.FullResUnsupported {
		t.Fatalf("small image requested full-res: loading=%v ready=%v unsupported=%v",
			a.viewer.FullResLoading, a.viewer.FullResReady, a.viewer.FullResUnsupported)
	}
}

func TestViewerFullResLargeImageStartsDecode(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewer.Item.W, a.viewer.Item.H = 9000, 7000
	a.requestViewerFullRes()
	if !a.viewer.FullResLoading {
		t.Fatal("large image did not start full-resolution decoding")
	}
	if a.viewer.FullResUnsupported {
		t.Fatal("large image was marked unsupported")
	}
}

func TestViewerActualSizeWaitsForFullRes(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewer.Item.W, a.viewer.Item.H = 9000, 7000
	a.viewer.FullResLoading = true // prevent the async decode from completing in this unit test
	a.viewer.Zoom = 1
	a.viewer.OneToOne = false
	a.viewerActualSize()
	if !a.viewerPendingActual {
		t.Fatal("actual size did not wait for the original bitmap")
	}
	if a.viewer.OneToOne {
		t.Fatal("actual size entered 1:1 before the original was ready")
	}
	if a.viewer.Zoom != 1 {
		t.Fatalf("preview was upscaled while waiting: zoom=%v", a.viewer.Zoom)
	}
	if got, _ := a.viewerQualityLabel(); got != "大图 · 待切 100%" {
		t.Fatalf("waiting for the original reads as %q", got)
	}
}

func TestViewerActualSizeAtRecordsAnchorWhileWaiting(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewer.Item.W, a.viewer.Item.H = 9000, 7000
	a.viewer.FullResLoading = true
	a.viewerActualSizeAt(123, 456, 800, 600)
	if !a.viewerPendingActual || !a.viewerPendingActualAt {
		t.Fatalf("anchor pending state = active:%v at:%v", a.viewerPendingActual, a.viewerPendingActualAt)
	}
	if a.viewerPendingActualX != 123 || a.viewerPendingActualY != 456 ||
		a.viewerPendingActualW != 800 || a.viewerPendingActualH != 600 {
		t.Fatalf("anchor = %.0f,%.0f %.0fx%.0f", a.viewerPendingActualX, a.viewerPendingActualY, a.viewerPendingActualW, a.viewerPendingActualH)
	}
	a.dragViewer(0, 0, 800, 600)
	if !a.viewerPendingActual {
		t.Fatal("stationary pointer cleared the pending actual-size anchor")
	}
}

func TestViewerBitmapSwapPreservesActualZoom(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewer.Bitmap.ReleasePixels()
	a.viewer.Bitmap = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 1000, 1000)))
	a.viewer.OneToOne = true
	a.viewer.Zoom = 1
	a.adjustViewerZoomForBitmapSwap(1000, 1000, 5000, 5000)
	if a.viewer.Zoom < 0.15 || a.viewer.Zoom > 12 {
		t.Fatalf("zoomed outside clamp: %v", a.viewer.Zoom)
	}
	if a.viewer.Zoom == 1 {
		t.Fatal("zoom was not recomputed for the larger bitmap")
	}
}

func TestViewerBitmapSwapPreservesDisplayedWidth(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	oldW, oldH := 2560, 2048
	newW, newH := 10000, 8000
	a.viewer.Zoom = 1.25
	w, h := a.viewerCanvasSize(nil)
	oldFit := viewerFitScale(oldW, oldH, a.viewer.Angle, w, h)
	oldWidth := oldFit * a.viewer.Zoom * float32(oldW)
	a.adjustViewerZoomForBitmapSwap(oldW, oldH, newW, newH)
	newFit := viewerFitScale(newW, newH, a.viewer.Angle, w, h)
	newWidth := newFit * a.viewer.Zoom * float32(newW)
	if absFloat(newWidth-oldWidth) > 1 {
		t.Fatalf("displayed width changed from %.1f to %.1f (zoom=%v)", oldWidth, newWidth, a.viewer.Zoom)
	}
	if absFloat(a.viewer.Zoom-1.25) > 0.001 {
		t.Fatalf("zoom changed from 1.25 to %v", a.viewer.Zoom)
	}
}

func TestViewerAnchorPanUsesCanvasCenter(t *testing.T) {
	x, y := viewerAnchorPan(800, 400, 1280, 800, 10, -20, 2)
	if absFloat(x-(-140)) > 0.01 || absFloat(y-(-40)) > 0.01 {
		t.Fatalf("anchor pan = %v,%v, want -140,-40", x, y)
	}
}

func TestViewerPendingActualAnchorUsesCanvasCenter(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewer.Bitmap.ReleasePixels()
	a.viewer.Bitmap = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 100, 100)))
	a.viewer.Zoom = 1
	a.viewer.OneToOne = false
	a.viewerPendingActual = true
	a.viewerPendingActualAt = true
	a.viewerPendingActualX = 640
	a.viewerPendingActualY = 400
	a.viewerPendingActualW = 1280
	a.viewerPendingActualH = 800
	a.viewerPendingActualOldW = 50
	a.viewerPendingActualOldH = 50
	a.applyViewerPendingActualSize()
	if absFloat(a.viewer.PanX) > 0.01 || absFloat(a.viewer.PanY) > 0.01 {
		t.Fatalf("center anchor moved pan to %v,%v, want 0,0", a.viewer.PanX, a.viewer.PanY)
	}
}

func TestViewerPendingActualAnchorKeepsContentUnderPointer(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewer.Bitmap.ReleasePixels()
	a.viewer.Bitmap = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 100, 100)))
	a.viewer.Zoom = 1
	a.viewer.OneToOne = false
	a.viewerPendingActual = true
	a.viewerPendingActualAt = true
	a.viewerPendingActualX = 800
	a.viewerPendingActualY = 500
	a.viewerPendingActualW = 1280
	a.viewerPendingActualH = 800
	a.viewerPendingActualOldW = 50
	a.viewerPendingActualOldH = 50
	w, h := a.viewerCanvasSize(nil)
	newFit := viewerFitScale(100, 100, 0, w, h)
	actual := clampViewerZoom(viewerActualZoom(newFit, 1), newFit, 1)
	// This is the transform a long press would apply once the original is
	// already loaded: the image is shown at the same display size as the
	// preview, with Zoom=1 for this 100x100 bitmap.
	wantZoom, wantX, wantY := viewerZoomAtPoint(1, 0, 0, actual, 800, 500, w, h, newFit, 1)
	a.applyViewerPendingActualSize()
	if absFloat(a.viewer.Zoom-wantZoom) > 0.001 ||
		absFloat(a.viewer.PanX-wantX) > 0.5 ||
		absFloat(a.viewer.PanY-wantY) > 0.5 {
		t.Fatalf("pending anchor = %.3f pan %.1f,%.1f; direct actual = %.3f pan %.1f,%.1f",
			a.viewer.Zoom, a.viewer.PanX, a.viewer.PanY, wantZoom, wantX, wantY)
	}
}

func TestViewerPointInImageRejectsLetterbox(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	if !a.viewerPointInImage(640, 400, 1280, 800) {
		t.Fatal("center of image was rejected")
	}
	if a.viewerPointInImage(100, 400, 1280, 800) {
		t.Fatal("left letterbox was treated as image area")
	}
}

func TestViewerPointInImageRejectsTopLetterbox(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewer.Bitmap.ReleasePixels()
	a.viewer.Bitmap = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 100, 50)))
	if !a.viewerPointInImage(640, 400, 1280, 800) {
		t.Fatal("center of wide image was rejected")
	}
	if a.viewerPointInImage(640, 40, 1280, 800) {
		t.Fatal("top letterbox was treated as image area")
	}
}

func TestViewerActualSizeAtIgnoresLetterbox(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewer.Item.W, a.viewer.Item.H = 9000, 7000
	a.viewer.FullResLoading = true
	a.viewerActualSizeAt(100, 400, 1280, 800)
	if a.viewerPendingActual {
		t.Fatal("letterbox long press started the original-size wait")
	}
	a.viewerActualSizeAt(640, 400, 1280, 800)
	if !a.viewerPendingActual {
		t.Fatal("image-area long press did not start the original-size wait")
	}
}

func TestViewerZoomAtIgnoresLetterbox(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	before := a.viewer.Zoom
	a.viewerZoomAt(1.25, 100, 400, 1280, 800)
	if a.viewer.Zoom != before {
		t.Fatalf("letterbox wheel changed zoom from %v to %v", before, a.viewer.Zoom)
	}
	a.viewerZoomAt(1.25, 640, 400, 1280, 800)
	if a.viewer.Zoom == before {
		t.Fatal("image-area wheel did not zoom")
	}
}

func TestViewerDragCannotMoveImageFullyOffCanvas(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewer.Zoom = 4
	a.dragViewer(5000, 5000, 1280, 800)
	bw, bh := a.viewer.Bitmap.Size()
	fit := viewerFitScale(bw, bh, a.viewer.Angle, 1280, 800)
	boundsW, _ := viewerRotatedBounds(bw, bh, a.viewer.Angle)
	dispW := boundsW * fit * a.viewer.Zoom
	cx := 1280/2 + a.viewer.PanX
	visible := min(cx+dispW/2, float32(1280)) - max(cx-dispW/2, float32(0))
	if visible < viewerPanMinVisible-0.1 {
		t.Fatalf("image left only %.1f px visible after drag", visible)
	}
}

func TestZeroMovementDragKeepsPendingActual(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewerPendingActual = true
	a.dragViewer(0, 0, 800, 600)
	if !a.viewerPendingActual {
		t.Fatal("zero-movement drag cleared pending actual size")
	}
	a.dragViewer(3, 4, 800, 600)
	if a.viewerPendingActual {
		t.Fatal("real drag did not clear pending actual size")
	}
	if a.viewer.PanX != 3 || a.viewer.PanY != 4 {
		t.Fatalf("pan = %v,%v, want 3,4", a.viewer.PanX, a.viewer.PanY)
	}
}

func TestViewerLongPressKeepsPendingDuringHold(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewer.Item.W, a.viewer.Item.H = 10000, 8000
	a.viewer.FullResLoading = true // keep the original decode from finishing
	tt := ui.NewTester(a.view, 900, 700)
	tt.Frame()
	x, y := canvasPoint(t, tt)
	tt.Press(x, y)
	tt.Frame()
	tt.Advance(viewerHoldDelay + 20*time.Millisecond)
	tt.Frame()
	if !a.viewerPendingActual {
		t.Fatal("long press cleared pending while the pointer was still held")
	}
	if a.viewer.OneToOne {
		t.Fatal("long press entered 1:1 before the original was ready")
	}
	if a.viewer.Zoom != 1 {
		t.Fatalf("long press moved the preview while waiting: zoom=%v", a.viewer.Zoom)
	}
	tt.Release(x, y)
}

func TestViewerHoverZoneArmsAndCancels(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.settings.ViewerPreload = 2
	tab := a.tab()
	tab.All = []*Item{
		{ID: "a", Name: "a.png", Kind: "image", Path: "a", W: 10, H: 10},
		{ID: "b", Name: "b.png", Kind: "image", Path: "b", W: 10, H: 10},
		{ID: "c", Name: "c.png", Kind: "image", Path: "c", W: 10, H: 10},
	}
	tab.Items = tab.All
	a.viewer.Index = 1
	a.viewer.Item = tab.Items[1]
	a.updateViewerHover(20, 350, 900, 700)
	if a.hoverKey == "" {
		t.Fatal("left edge did not arm hover prefetch")
	}
	a.updateViewerHover(450, 350, 900, 700)
	if a.hoverKey != "" {
		t.Fatal("center did not cancel hover prefetch")
	}
	a.clearViewerHover()
}

func TestViewerPreviewLimitsAtBoundaries(t *testing.T) {
	if !viewerPreviewWithinLimit(10000, 8000) {
		t.Fatal("exactly 80MP preview should be allowed")
	}
	if viewerPreviewWithinLimit(10000, 8001) {
		t.Fatal("over 80MP preview should be rejected")
	}
}

func TestViewerOversizePreviewFallback(t *testing.T) {
	srcPath := filepath.Join("testdata", "thumb-corpus", "viewer", "80mp.jpg")
	st, err := os.Stat(srcPath)
	if err != nil {
		t.Skipf("oversize fixture unavailable: %v", err)
	}
	thumbDir = t.TempDir()
	previewDir = thumbDir
	t.Setenv("FLOWVISION_VIEWER_CACHE", "off")
	it := &Item{
		ID: "oversize-preview-test", Name: "80mp.jpg", Kind: "image",
		Path: srcPath, Ext: ".jpg", Size: st.Size(), Mtime: st.ModTime().Unix(),
	}
	res, err := loadViewerPreviewFallback(it, dimInfo{W: 10000, H: 8000}, "")
	if err != nil {
		t.Fatalf("oversize fallback failed: %v", err)
	}
	if res == nil || res.Img == nil {
		t.Fatal("oversize fallback returned no image")
	}
	b := res.Img.Bounds()
	if b.Dx() > viewerPreviewMaxDim || b.Dy() > viewerPreviewMaxDim {
		t.Fatalf("fallback image %dx%d exceeds %d", b.Dx(), b.Dy(), viewerPreviewMaxDim)
	}
	if res.Bytes == 0 {
		t.Fatal("fallback produced no cached JPEG")
	}
}

func TestViewerPreviewFallbackHeight(t *testing.T) {
	cases := []struct {
		w, h, want int
	}{
		{10000, 8000, 2048},
		{8000, 10000, 2560},
		{10000, 10000, 2560},
		{10000, 1000, 256},
	}
	for _, tc := range cases {
		if got := viewerPreviewFallbackHeight(tc.w, tc.h); got != tc.want {
			t.Errorf("fallback height(%d,%d) = %d, want %d", tc.w, tc.h, got, tc.want)
		}
	}
}

func TestViewerPreloadBudgetAccounting(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.settings.ViewerPreload = 2
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	bmp := ui.NewBitmap(img)
	other := a.tab().Items[1]
	a.storeViewerPreload(other, bmp)
	if a.viewerPreloadBytes == 0 {
		t.Fatal("preload bytes not counted")
	}
	got := a.takeViewerPreload(other)
	if got != bmp {
		t.Fatal("take did not return the stored bitmap")
	}
	if a.viewerPreloadBytes != 0 {
		t.Fatalf("bytes after take = %d", a.viewerPreloadBytes)
	}
	if a.viewerPreload[viewerPreloadKey(other)] != nil {
		t.Fatal("cache entry not removed on take")
	}
	got.ReleasePixels()
}
