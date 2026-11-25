package main

import (
	"fmt"
	"os"

	"github.com/egoist/mygo/ui"
)

type viewerPreloadEntry struct {
	Bmp *ui.Bitmap
	Err string
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
	data, err := previewBytes(it)
	if err != nil {
		if raw, rerr := os.ReadFile(it.Path); rerr == nil {
			data, err = raw, nil
		}
	}
	if err != nil {
		return nil, err
	}
	return ui.DecodeBitmap(data)
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
	if n < 0 {
		n = 0
	}
	if n > 8 {
		n = 8
	}
	items := a.viewerPreloadItems(n)
	desired := map[string]bool{}
	if a.viewer.Item.Kind == "image" {
		desired[viewerPreloadKey(a.viewer.Item)] = true
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
	for key := range a.viewerPreload {
		if !desired[key] {
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
		if a.viewerPreload[key] != nil || a.viewerPreloadLoading[key] || keep[key] {
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
	a.viewerPreloadMu.Unlock()

	if start {
		go a.runViewerPreloadWorker(epoch)
	}
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
			a.viewerPreload[job.Key] = &viewerPreloadEntry{Bmp: bmp}
		}
		a.viewerPreloadMu.Unlock()
	}
}

func (a *App) clearViewerPreloadCache() {
	a.viewerPreloadMu.Lock()
	a.viewerPreloadEpoch++
	a.viewerPreload = map[string]*viewerPreloadEntry{}
	a.viewerPreloadLoading = map[string]bool{}
	a.viewerPreloadDesired = map[string]bool{}
	a.viewerPreloadQueue = nil
	a.viewerPreloadWorker = 0
	a.viewerPreloadMu.Unlock()
}

func (a *App) takeViewerPreload(it *Item) *ui.Bitmap {
	key := viewerPreloadKey(it)
	a.viewerPreloadMu.Lock()
	defer a.viewerPreloadMu.Unlock()
	if entry := a.viewerPreload[key]; entry != nil {
		return entry.Bmp
	}
	return nil
}

func (a *App) storeViewerPreload(it *Item, bmp *ui.Bitmap) {
	if it == nil || bmp == nil || !a.viewer.Open {
		return
	}
	a.viewerPreloadMu.Lock()
	if a.viewerPreload == nil {
		a.viewerPreload = map[string]*viewerPreloadEntry{}
	}
	a.viewerPreload[viewerPreloadKey(it)] = &viewerPreloadEntry{Bmp: bmp}
	a.viewerPreloadMu.Unlock()
}

func (a *App) viewerPreloadCount() int {
	a.viewerPreloadMu.Lock()
	defer a.viewerPreloadMu.Unlock()
	return len(a.viewerPreload)
}
