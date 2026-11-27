package main

import (
	"fmt"
	"os"
	"time"

	"github.com/egoist/mygo/ui"
)

// viewerPreloadBudget is the bitmap budget for the neighbouring previews. A
// 2560×1440 preview is 14.7 MB and a 2560×2560 one 26 MB, so 384 MB keeps the
// configured 8 neighbours per side on tall images instead of silently pruning
// them down to the first few.
const viewerPreloadBudget = 384 << 20

type viewerPreloadEntry struct {
	Bmp   *ui.Bitmap
	Bytes int64
	At    int64
	Err   string
}

type viewerPreloadJob struct {
	It    *Item
	Key   string
	Epoch int
}

func viewerPreloadKey(it *Item) string {
	if it == nil {
		return ""
	}
	return fmt.Sprintf("%s|%d|%d", it.ID, it.Mtime, it.Size)
}

func decodeViewerImage(it *Item) (*ui.Bitmap, error) {
	res, err := loadViewerPreview(it)
	if err != nil {
		return nil, err
	}
	return res.Bmp, nil
}

func (a *App) viewerPreloadItems(n int) []*Item {
	t := a.tab()
	if t == nil || n <= 0 {
		return nil
	}
	cur := a.viewer.Index
	if cur < 0 || cur >= len(t.Items) || t.Items[cur] != a.viewer.Item {
		return nil
	}
	var out []*Item
	for i := cur - 1; i >= 0 && len(out) < n; i-- {
		if t.Items[i].Kind == "image" {
			out = append(out, t.Items[i])
		}
	}
	next := 0
	for i := cur + 1; i < len(t.Items) && next < n; i++ {
		if t.Items[i].Kind == "image" {
			out = append(out, t.Items[i])
			next++
		}
	}
	return out
}

// scheduleViewerPreload keeps the configured number of images on both sides
// of the current one decoded and ready. One worker drains the queue in order,
// so a fast sequence of steps cannot pile up dozens of full-size decodes.
func (a *App) scheduleViewerPreload() {
	if !a.viewer.Open || a.viewer.Item == nil {
		return
	}
	n := a.settings.ViewerPreload
	if os.Getenv("FLOWVISION_VIEWER_PRELOAD") == "off" {
		n = 0
	}
	if n < 0 {
		n = 0
	}
	if n > 8 {
		n = 8
	}
	items := a.viewerPreloadItems(n)
	desired := map[string]bool{}
	currentKey := ""
	if a.viewer.Item.Kind == "image" {
		currentKey = viewerPreloadKey(a.viewer.Item)
		desired[currentKey] = true
	}
	for _, it := range items {
		desired[viewerPreloadKey(it)] = true
	}

	a.viewerPreloadMu.Lock()
	if a.viewerPreload == nil {
		a.viewerPreload = map[string]*viewerPreloadEntry{}
	}
	if a.viewerPreloadLoading == nil {
		a.viewerPreloadLoading = map[string]bool{}
	}
	a.viewerPreloadDesired = desired
	for key, entry := range a.viewerPreload {
		if !desired[key] {
			a.releaseViewerPreloadEntryLocked(entry)
			delete(a.viewerPreload, key)
		}
	}
	// Rebuild the queue from the new window, keeping the loaded entries. A
	// job for an image still in range keeps its place; everything stale is
	// dropped so the worker does not decode last step's neighbors.
	keep := map[string]bool{}
	var queue []viewerPreloadJob
	for _, job := range a.viewerPreloadQueue {
		if desired[job.Key] && !keep[job.Key] {
			keep[job.Key] = true
			queue = append(queue, job)
		}
	}
	for _, it := range items {
		key := viewerPreloadKey(it)
		if key == currentKey || a.viewerPreload[key] != nil || a.viewerPreloadLoading[key] || keep[key] {
			continue
		}
		keep[key] = true
		queue = append(queue, viewerPreloadJob{It: it, Key: key, Epoch: a.viewerPreloadEpoch})
	}
	a.viewerPreloadQueue = queue
	start := false
	if a.viewerPreloadWorker == 0 && len(queue) > 0 {
		a.viewerPreloadWorker = a.viewerPreloadEpoch
		start = true
	}
	epoch := a.viewerPreloadEpoch
	a.pruneViewerPreloadLocked(currentKey)
	a.viewerPreloadMu.Unlock()

	if start {
		go a.runViewerPreloadWorker(epoch)
	}
	// With no neighbours to decode (none, all cached, or preload turned off)
	// the original of the current image can start right away.
	a.maybeAutoViewerFullRes()
}

func (a *App) runViewerPreloadWorker(epoch int) {
	for {
		a.viewerPreloadMu.Lock()
		if a.viewerPreloadEpoch != epoch {
			if a.viewerPreloadWorker == epoch {
				a.viewerPreloadWorker = 0
			}
			a.viewerPreloadMu.Unlock()
			return
		}
		if len(a.viewerPreloadQueue) == 0 {
			if a.viewerPreloadWorker == epoch {
				a.viewerPreloadWorker = 0
			}
			a.viewerPreloadMu.Unlock()
			return
		}
		job := a.viewerPreloadQueue[0]
		a.viewerPreloadQueue = a.viewerPreloadQueue[1:]
		if a.viewerPreload[job.Key] != nil {
			a.viewerPreloadMu.Unlock()
			continue
		}
		if !a.viewerPreloadDesired[job.Key] {
			a.viewerPreloadMu.Unlock()
			continue
		}
		a.viewerPreloadLoading[job.Key] = true
		a.viewerPreloadMu.Unlock()

		bmp, err := decodeViewerImage(job.It)

		a.viewerPreloadMu.Lock()
		delete(a.viewerPreloadLoading, job.Key)
		if a.viewerPreloadEpoch == epoch && a.viewerPreloadDesired[job.Key] && err == nil {
			if a.viewerPreload == nil {
				a.viewerPreload = map[string]*viewerPreloadEntry{}
			}
			if old := a.viewerPreload[job.Key]; old != nil {
				a.releaseViewerPreloadEntryLocked(old)
			}
			entry := &viewerPreloadEntry{Bmp: bmp, Bytes: viewerBitmapBytes(bmp), At: time.Now().UnixNano()}
			a.viewerPreload[job.Key] = entry
			a.viewerPreloadBytes += entry.Bytes
			a.pruneViewerPreloadLocked(viewerPreloadKey(a.viewer.Item))
		} else if bmp != nil {
			bmp.ReleasePixels()
		}
		drained := a.viewerPreloadEpoch == epoch && len(a.viewerPreloadQueue) == 0 && len(a.viewerPreloadLoading) == 0
		a.viewerPreloadMu.Unlock()
		if drained {
			// The neighbours are ready; the current image may now warm its
			// original without competing with them.
			a.update(func() { a.maybeAutoViewerFullRes() })
		}
	}
}

func (a *App) clearViewerPreloadCache() {
	a.viewerPreloadMu.Lock()
	a.viewerPreloadEpoch++
	for _, entry := range a.viewerPreload {
		a.releaseViewerPreloadEntryLocked(entry)
	}
	a.viewerPreload = map[string]*viewerPreloadEntry{}
	a.viewerPreloadLoading = map[string]bool{}
	a.viewerPreloadDesired = map[string]bool{}
	a.viewerPreloadQueue = nil
	a.viewerPreloadWorker = 0
	a.viewerPreloadBytes = 0
	a.viewerPreloadMu.Unlock()
}

// takeViewerPreload removes the entry and transfers its pixels to the viewer.
// The caller must set Viewer.BitmapOwned so the bitmap is released on close or
// on the next image.
func (a *App) takeViewerPreload(it *Item) *ui.Bitmap {
	key := viewerPreloadKey(it)
	a.viewerPreloadMu.Lock()
	defer a.viewerPreloadMu.Unlock()
	if entry := a.viewerPreload[key]; entry != nil {
		delete(a.viewerPreload, key)
		a.viewerPreloadBytes -= entry.Bytes
		if a.viewerPreloadBytes < 0 {
			a.viewerPreloadBytes = 0
		}
		return entry.Bmp
	}
	return nil
}

func (a *App) storeViewerPreload(it *Item, bmp *ui.Bitmap) {
	if it == nil || bmp == nil || !a.viewer.Open {
		return
	}
	// The current image is already drawn and owned by the viewer. Caching the
	// same bitmap would give it two owners and double-release it on close.
	if a.viewer.Item != nil && viewerPreloadKey(a.viewer.Item) == viewerPreloadKey(it) {
		return
	}
	key := viewerPreloadKey(it)
	a.viewerPreloadMu.Lock()
	if a.viewerPreload == nil {
		a.viewerPreload = map[string]*viewerPreloadEntry{}
	}
	if old := a.viewerPreload[key]; old != nil {
		a.releaseViewerPreloadEntryLocked(old)
	}
	entry := &viewerPreloadEntry{Bmp: bmp, Bytes: viewerBitmapBytes(bmp), At: time.Now().UnixNano()}
	a.viewerPreload[key] = entry
	a.viewerPreloadBytes += entry.Bytes
	a.pruneViewerPreloadLocked(viewerPreloadKey(a.viewer.Item))
	a.viewerPreloadMu.Unlock()
}

func viewerBitmapBytes(bmp *ui.Bitmap) int64 {
	if bmp == nil {
		return 0
	}
	w, h := bmp.Size()
	return int64(w) * int64(h) * 4
}

func (a *App) releaseViewerPreloadEntryLocked(entry *viewerPreloadEntry) {
	if entry == nil {
		return
	}
	a.viewerPreloadBytes -= entry.Bytes
	if a.viewerPreloadBytes < 0 {
		a.viewerPreloadBytes = 0
	}
	if entry.Bmp != nil {
		entry.Bmp.ReleasePixels()
	}
}

func (a *App) pruneViewerPreloadLocked(currentKey string) {
	if a.viewerPreloadBytes <= viewerPreloadBudget {
		return
	}
	for a.viewerPreloadBytes > viewerPreloadBudget {
		var oldestKey string
		var oldestAt int64 = 1<<63 - 1
		for key, entry := range a.viewerPreload {
			if key == currentKey {
				continue
			}
			if entry != nil && entry.At < oldestAt {
				oldestAt = entry.At
				oldestKey = key
			}
		}
		if oldestKey == "" {
			break
		}
		a.releaseViewerPreloadEntryLocked(a.viewerPreload[oldestKey])
		delete(a.viewerPreload, oldestKey)
	}
}

func (a *App) viewerPreloadCount() int {
	a.viewerPreloadMu.Lock()
	defer a.viewerPreloadMu.Unlock()
	return len(a.viewerPreload)
}
