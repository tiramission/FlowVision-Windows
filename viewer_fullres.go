package main

import (
	"fmt"
	"image"
	"os"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"
)

// viewerFullResEntry keeps the decoded original while the viewer stays on the
// current image. There is deliberately no pixel-count gate: 100% must always
// mean the decoded original. The preview remains visible until the decode
// completes, and failures keep that preview instead of a blank canvas.
type viewerFullResEntry struct {
	Key   string
	Bmp   *ui.Bitmap
	Bytes int64
	At    time.Time
}

func viewerFullResKey(it *Item) string {
	if it == nil {
		return ""
	}
	return fmt.Sprintf("%s|%d|%d", it.ID, it.Mtime, it.Size)
}

var (
	// viewerFullResSlots serializes original decodes. Flipping through a folder
	// must never run several multi-hundred-megabyte decodes at once.
	viewerFullResSlots = make(chan struct{}, 1)
	// viewerFullResWanted holds the key of the original the viewer is waiting
	// for. A queued decode compares it again after taking the slot and drops
	// itself when the user has already moved on, so a fast sequence of steps
	// leaves at most one wasted decode behind.
	viewerFullResWanted atomic.Value // string
)

func viewerFullResIsWanted(key string) bool {
	want, _ := viewerFullResWanted.Load().(string)
	return want != "" && want == key
}

// requestViewerFullRes decodes the original for an explicit 1:1 request. The
// preview stays visible while it runs and the "loading" notice only appears
// when the decode takes long enough to notice.
func (a *App) requestViewerFullRes() {
	a.startViewerFullRes(false)
}

// requestViewerFullResQuiet warms the original in the background without any
// notice; a failure leaves the preview in place silently.
func (a *App) requestViewerFullResQuiet() {
	a.startViewerFullRes(true)
}

// startViewerFullRes decodes and keeps one full-resolution bitmap. It never
// marks the viewer as 1:1 preemptively: actual-size callers either apply
// immediately when the bitmap is already ready, or wait for the pending flag
// below.
func (a *App) startViewerFullRes(silent bool) {
	it := a.viewer.Item
	if it == nil || it.Kind != "image" {
		return
	}
	if a.viewer.FullResReady {
		return
	}
	if a.viewer.FullResLoading || a.viewer.FullResUnsupported {
		return
	}
	if a.viewerFullRes != nil && a.viewerFullRes.Key == viewerFullResKey(it) && a.viewerFullRes.Bmp != nil {
		if a.viewer.Bitmap == nil {
			return
		}
		bw, bh := a.viewerFullRes.Bmp.Size()
		oldW, oldH := a.viewer.Bitmap.Size()
		a.viewer.Bitmap = a.viewerFullRes.Bmp
		a.viewer.BitmapOwned = true
		a.viewer.FullResReady = true
		a.viewer.Placeholder = nil
		a.adjustViewerZoomForBitmapSwap(oldW, oldH, bw, bh)
		a.refresh()
		return
	}
	w, h := it.W, it.H
	if w <= 0 || h <= 0 {
		d := imageDims(it.Path, it.Mtime)
		w, h = d.W, d.H
	}
	// A preview at or below 2560 is already the full image; decoding it again
	// would only add memory and change nothing on screen.
	if w > 0 && h > 0 && w <= viewerPreviewMaxDim && h <= viewerPreviewMaxDim {
		return
	}

	gen := a.viewer.Gen
	key := viewerFullResKey(it)
	a.viewer.FullResLoading = true
	viewerFullResWanted.Store(key)
	if !silent {
		a.startViewerFullTimer(gen)
	}
	go func() {
		if !viewerFullResIsWanted(key) {
			return
		}
		viewerFullResSlots <- struct{}{}
		defer func() { <-viewerFullResSlots }()
		if !viewerFullResIsWanted(key) {
			// The viewer moved on while this decode waited its turn.
			return
		}
		bmp, bytes, err := decodeViewerFullRes(it)
		a.update(func() {
			if a.viewer.Gen != gen || a.viewer.Item != it || viewerFullResKey(a.viewer.Item) != key {
				if bmp != nil {
					bmp.ReleasePixels()
				}
				return
			}
			a.stopViewerFullTimer()
			a.viewer.FullResLoading = false
			if err != nil {
				if bmp != nil {
					bmp.ReleasePixels()
				}
				a.viewer.FullResUnsupported = true
				a.clearViewerPendingActualSize()
				if !silent {
					a.notifyViewer("fullres", "原图载入失败："+err.Error(), viewerNoticeError, 4*time.Second)
				}
				a.refresh()
				return
			}
			a.clearViewerFullRes()
			oldBmp := a.viewer.Bitmap
			oldOwned := a.viewer.BitmapOwned
			oldW, oldH := 0, 0
			if oldBmp != nil {
				oldW, oldH = oldBmp.Size()
			}
			a.viewerFullRes = &viewerFullResEntry{Key: key, Bmp: bmp, Bytes: bytes, At: time.Now()}
			a.viewer.Bitmap = bmp
			a.viewer.BitmapOwned = true
			a.viewer.FullResReady = true
			a.viewer.Placeholder = nil
			a.viewerStats.FullResBytes = bytes
			nw, nh := bmp.Size()
			if a.viewerPendingActual {
				a.applyViewerPendingActualSize()
			} else {
				a.adjustViewerZoomForBitmapSwap(oldW, oldH, nw, nh)
			}
			if oldOwned && oldBmp != nil && oldBmp != bmp {
				oldBmp.ReleasePixels()
			}
			a.refresh()
		})
	}()
}

func decodeViewerFullRes(it *Item) (*ui.Bitmap, int64, error) {
	f, err := os.Open(it.Path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil, 0, err
	}
	d := imageDims(it.Path, it.Mtime)
	if d.Orient > 1 && d.Orient < 9 {
		src = orientImage(src, d.Orient)
	}
	b := src.Bounds()
	bmp := ui.NewBitmap(src)
	return bmp, int64(b.Dx()) * int64(b.Dy()) * 4, nil
}

func (a *App) startViewerFullTimer(gen int) {
	a.stopViewerFullTimer()
	a.viewerFullTimer = time.AfterFunc(1200*time.Millisecond, func() {
		a.update(func() {
			if a.viewer.Gen == gen && a.viewer.FullResLoading {
				a.notifyViewer("fullres", "正在载入 1:1 原图…", viewerNoticeInfo, 0)
			}
		})
	})
}

func (a *App) stopViewerFullTimer() {
	if a.viewerFullTimer != nil {
		a.viewerFullTimer.Stop()
		a.viewerFullTimer = nil
	}
}

func (a *App) clearViewerFullRes() {
	a.stopViewerFullTimer()
	viewerFullResWanted.Store("")
	if a.viewerFullRes != nil {
		if a.viewerFullRes.Bmp != nil {
			a.viewerFullRes.Bmp.ReleasePixels()
		}
		if a.viewer.Bitmap == a.viewerFullRes.Bmp {
			a.viewer.Bitmap = nil
			a.viewer.BitmapOwned = false
		}
		a.viewerFullRes = nil
	}
	a.viewerFullResKey = ""
	a.viewer.FullResReady = false
	a.viewer.FullResLoading = false
}

func (a *App) adjustViewerZoomForBitmapSwap(oldW, oldH, newW, newH int) {
	if oldW <= 0 || oldH <= 0 || newW <= 0 || newH <= 0 {
		return
	}
	w, h := a.viewerCanvasSize(nil)
	oldFit := viewerFitScale(oldW, oldH, a.viewer.Angle, w, h)
	newFit := viewerFitScale(newW, newH, a.viewer.Angle, w, h)
	if oldFit <= 0 || newFit <= 0 {
		return
	}
	if a.viewer.OneToOne {
		a.viewer.Zoom = clampViewerZoom(viewerActualZoom(newFit))
		return
	}
	// Zoom is relative to the current bitmap's fit. Preserving
	// Zoom*oldFit would preserve the preview's per-pixel scale, not the
	// displayed scene: replacing a 2560 preview with a 10000px original
	// would suddenly magnify the image. Preserve the displayed width instead.
	displayWidth := a.viewer.Zoom * oldFit * float32(oldW)
	newWidthAtFit := newFit * float32(newW)
	if displayWidth <= 0 || newWidthAtFit <= 0 {
		return
	}
	a.viewer.Zoom = clampViewerZoom(displayWidth / newWidthAtFit)
}

// viewerNeedsFullResForActualSize reports whether 1:1 would enlarge a 2560
// preview. If it would, actual-size waits for the original instead of showing
// an upscaled preview frame first.
func (a *App) viewerNeedsFullResForActualSize() bool {
	if a.viewer.FullResReady {
		return false
	}
	if a.viewer.Item == nil || a.viewer.Item.Kind != "image" {
		return false
	}
	w, h := a.viewer.Item.W, a.viewer.Item.H
	if w <= 0 || h <= 0 {
		d := imageDims(a.viewer.Item.Path, a.viewer.Item.Mtime)
		w, h = d.W, d.H
	}
	if w <= 0 || h <= 0 {
		return false
	}
	return w > viewerPreviewMaxDim || h > viewerPreviewMaxDim
}

// deferViewerActualSize starts the original decode and holds the current view
// until the bitmap is ready. The anchor variant keeps the point under the
// pointer stable when the final 1:1 transform is applied.
func (a *App) deferViewerActualSize(at bool, x, y, w, h float32) {
	if a.viewer.FullResUnsupported {
		a.clearViewerPendingActualSize()
		a.notifyViewer("fullres", "原图载入失败，无法切换到 100%", viewerNoticeError, 4*time.Second)
		a.refresh()
		return
	}
	a.viewerPendingActual = true
	a.viewerPendingActualAt = at
	a.viewerPendingActualX = x
	a.viewerPendingActualY = y
	a.viewerPendingActualW = w
	a.viewerPendingActualH = h
	if a.viewer.Bitmap != nil {
		a.viewerPendingActualOldW, a.viewerPendingActualOldH = a.viewer.Bitmap.Size()
	}
	a.viewer.OneToOne = false
	a.requestViewerFullRes()
	if a.viewer.FullResReady {
		a.applyViewerPendingActualSize()
		return
	}
	a.notifyViewer("fullres", "正在载入原图，完成后切换到 100%…", viewerNoticeInfo, 0)
	a.refresh()
}

func (a *App) clearViewerPendingActualSize() {
	a.viewerPendingActual = false
	a.viewerPendingActualAt = false
	a.viewerPendingActualX = 0
	a.viewerPendingActualY = 0
	a.viewerPendingActualW = 0
	a.viewerPendingActualH = 0
	a.viewerPendingActualOldW = 0
	a.viewerPendingActualOldH = 0
	if a.viewer.NoticeKey == "fullres" && a.viewer.Notice.Until.IsZero() {
		a.viewer.Notice = viewerNotice{}
		a.viewer.NoticeKey = ""
	}
}

func (a *App) applyViewerPendingActualSize() {
	if !a.viewerPendingActual || a.viewer.Bitmap == nil {
		a.clearViewerPendingActualSize()
		return
	}
	at := a.viewerPendingActualAt
	px, py := a.viewerPendingActualX, a.viewerPendingActualY
	pw, ph := a.viewerPendingActualW, a.viewerPendingActualH
	bw, bh := a.viewer.Bitmap.Size()
	w, h := a.viewerCanvasSize(nil)
	if w <= 0 || h <= 0 {
		w, h = pw, ph
	}
	fit := viewerFitScale(bw, bh, a.viewer.Angle, w, h)
	if fit <= 0 {
		a.clearViewerPendingActualSize()
		return
	}
	actual := clampViewerZoom(viewerActualZoom(fit))
	if at {
		if px < 0 || py < 0 {
			px, py = w/2, h/2
		}
		current := a.viewer.Zoom
		if current <= 0 {
			current = 1
		}
		oldW, oldH := a.viewerPendingActualOldW, a.viewerPendingActualOldH
		oldFit := float32(0)
		if oldW > 0 && oldH > 0 {
			oldFit = viewerFitScale(oldW, oldH, a.viewer.Angle, w, h)
		}
		// The pointer must stay over the same image content, not the same
		// preview pixel. The preview and original have different pixel
		// widths, so the anchor ratio is the ratio of displayed widths
		// (fit * zoom * bitmap width), not the ratio of fit scales alone.
		oldDisplayW := oldFit * current * float32(oldW)
		newDisplayW := fit * actual * float32(bw)
		if oldDisplayW > 0 && newDisplayW > 0 {
			ratio := newDisplayW / oldDisplayW
			a.viewer.PanX, a.viewer.PanY = viewerAnchorPan(
				px, py, w, h, a.viewer.PanX, a.viewer.PanY, ratio)
		} else {
			a.viewer.PanX = 0
			a.viewer.PanY = 0
		}
		a.viewer.Zoom = actual
		a.viewer.OneToOne = true
	} else {
		a.viewer.Zoom = actual
		a.viewer.PanX = 0
		a.viewer.PanY = 0
		a.viewer.OneToOne = true
	}
	a.clearViewerPendingActualSize()
	a.notifyViewer("zoom", "100%", viewerNoticeInfo, 1200*time.Millisecond)
}

func (a *App) maybeRequestViewerFullRes() {
	if a.viewer.Item == nil || a.viewer.Item.Kind != "image" || a.viewer.Bitmap == nil {
		return
	}
	if a.viewer.FullResReady || a.viewer.FullResLoading || a.viewer.FullResUnsupported {
		return
	}
	bw, bh := a.viewer.Bitmap.Size()
	w, h := a.viewerCanvasSize(nil)
	if viewerBitmapPercentValue(bw, bh, a.viewer.Angle, a.viewer.Zoom, w, h) >= 125 {
		a.requestViewerFullRes()
	}
}

// maybeAutoViewerFullRes warms the original of the image on screen so pressing
// 1 is instant. It waits until the neighbouring 2560 previews are done, so
// paging always stays ahead of the original, and it does nothing when the
// preview already is the full image. Returns whether a decode was started.
func (a *App) maybeAutoViewerFullRes() bool {
	if !a.settings.ViewerAutoFullRes || !a.viewer.Open || a.viewer.Item == nil {
		return false
	}
	if a.viewer.Item.Kind != "image" || a.viewer.Bitmap == nil {
		return false
	}
	if a.viewer.FullResReady || a.viewer.FullResLoading || a.viewer.FullResUnsupported {
		return false
	}
	if !a.viewerNeedsFullResForActualSize() {
		return false
	}
	a.viewerPreloadMu.Lock()
	busy := len(a.viewerPreloadQueue) > 0 || len(a.viewerPreloadLoading) > 0
	a.viewerPreloadMu.Unlock()
	if busy {
		return false
	}
	a.requestViewerFullResQuiet()
	return a.viewer.FullResLoading
}
