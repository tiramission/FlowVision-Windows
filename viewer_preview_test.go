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
	a.requestViewerFullRes(false)
	if a.viewer.FullResLoading || a.viewer.FullResReady || a.viewer.FullResUnsupported {
		t.Fatalf("small image requested full-res: loading=%v ready=%v unsupported=%v",
			a.viewer.FullResLoading, a.viewer.FullResReady, a.viewer.FullResUnsupported)
	}
}

func TestViewerFullResLargeImageStartsDecode(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewer.Item.W, a.viewer.Item.H = 9000, 7000
	a.requestViewerFullRes(false)
	if !a.viewer.FullResLoading {
		t.Fatal("large image did not start full-resolution decoding")
	}
	if a.viewer.FullResUnsupported {
		t.Fatal("large image was marked unsupported")
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

func TestViewerHUDModesAndPersistentNotice(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewer.Notice = viewerNotice{}
	t.Setenv("FLOWVISION_VIEWER_HUD", "debug")
	if got := a.viewerHUDMode(); got != "debug" {
		t.Fatalf("hud mode = %q", got)
	}
	a.notifyViewer("load", "正在载入大图…", viewerNoticeInfo, 0)
	if a.viewer.Notice.Text == "" {
		t.Fatal("persistent notice missing")
	}
	if !a.viewer.Notice.Until.IsZero() {
		t.Fatal("persistent notice should not have an expiry")
	}
	if !a.viewerHUDVisible() {
		t.Fatal("persistent notice not visible")
	}
	a.viewer.Notice = viewerNotice{}
	t.Setenv("FLOWVISION_VIEWER_HUD", "off")
	if a.viewerHUDVisible() {
		t.Fatal("debug notice visible while HUD off")
	}
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

func TestViewerHUDSettingIsInSettings(t *testing.T) {
	a := newViewTestApp()
	cats := a.settingsCategories()
	for _, cat := range cats {
		if cat.ID != "control" {
			continue
		}
		for _, group := range cat.Groups {
			for _, item := range group.Items {
				if item.ID == "viewer-hud" {
					return
				}
			}
		}
	}
	t.Fatal("viewer HUD setting missing from viewer-info category")
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
