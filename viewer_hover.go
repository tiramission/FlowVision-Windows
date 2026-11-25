package main

import "time"

const (
	viewerHoverDelay      = 220 * time.Millisecond
	viewerHoverEdgeRatio  = 0.18
	viewerHoverMiddleLow  = 0.24
	viewerHoverMiddleHigh = 0.76
)

func (a *App) clearViewerHover() {
	if a.hoverTimer != nil {
		a.hoverTimer.Stop()
		a.hoverTimer = nil
	}
	a.hoverKey = ""
	a.hoverToken++
}

func (a *App) viewerHoverItem(dir int) *Item {
	t := a.tab()
	if t == nil || a.viewer.Item == nil {
		return nil
	}
	idx := a.viewer.Index
	if idx < 0 || idx >= len(t.Items) || t.Items[idx] != a.viewer.Item {
		return nil
	}
	for i := idx + dir; i >= 0 && i < len(t.Items); i += dir {
		if t.Items[i].Kind == "image" {
			return t.Items[i]
		}
	}
	return nil
}

// updateViewerHover only arms a timer when the pointer is in the left or
// right edge band and vertically away from the header and footer. Moving away
// cancels the pending job, so a pointer crossing the canvas never prefetches.
func (a *App) updateViewerHover(x, y, w, h float32) {
	if a.settings.ViewerPreload <= 0 || a.viewer.Item == nil || a.viewer.Item.Kind != "image" || w <= 0 || h <= 0 {
		a.clearViewerHover()
		return
	}
	dir := 0
	switch {
	case x < w*viewerHoverEdgeRatio:
		dir = -1
	case x > w*(1-viewerHoverEdgeRatio):
		dir = 1
	}
	if dir == 0 || y < h*viewerHoverMiddleLow || y > h*viewerHoverMiddleHigh {
		a.clearViewerHover()
		return
	}
	it := a.viewerHoverItem(dir)
	if it == nil {
		a.clearViewerHover()
		return
	}
	key := viewerPreloadKey(it)
	if key == a.hoverKey {
		return
	}
	a.clearViewerHover()
	a.hoverKey = key
	token := a.hoverToken
	a.hoverTimer = time.AfterFunc(viewerHoverDelay, func() {
		a.update(func() {
			if token != a.hoverToken || a.hoverKey != key {
				return
			}
			if !a.viewer.Open || a.viewer.Item == nil || a.viewer.Item.Kind != "image" {
				return
			}
			a.prefetchViewerHover(it)
		})
	})
}

func (a *App) prefetchViewerHover(it *Item) {
	if it == nil || a.settings.ViewerPreload <= 0 {
		return
	}
	key := viewerPreloadKey(it)
	a.viewerPreloadMu.Lock()
	if a.viewerPreload == nil {
		a.viewerPreload = map[string]*viewerPreloadEntry{}
	}
	if a.viewerPreloadLoading == nil {
		a.viewerPreloadLoading = map[string]bool{}
	}
	if a.viewerPreload[key] != nil || a.viewerPreloadLoading[key] {
		a.viewerPreloadMu.Unlock()
		return
	}
	if a.viewerPreloadDesired == nil {
		a.viewerPreloadDesired = map[string]bool{}
	}
	a.viewerPreloadDesired[key] = true
	var queue []viewerPreloadJob
	for _, job := range a.viewerPreloadQueue {
		if job.Key != key {
			queue = append(queue, job)
		}
	}
	queue = append([]viewerPreloadJob{{It: it, Key: key, Epoch: a.viewerPreloadEpoch}}, queue...)
	a.viewerPreloadQueue = queue
	start := false
	if a.viewerPreloadWorker == 0 {
		a.viewerPreloadWorker = a.viewerPreloadEpoch
		start = true
	}
	epoch := a.viewerPreloadEpoch
	a.viewerPreloadMu.Unlock()
	if start {
		go a.runViewerPreloadWorker(epoch)
	}
}
