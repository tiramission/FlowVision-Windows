package main

import (
	"context"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
)

// cacheLaneApp prepares an app whose thumbnail cache lives in a temp directory
// and whose window is a stand-in, so a submission can be inspected without
// starting workers.
func cacheLaneApp(t *testing.T) *App {
	t.Helper()
	thumbDir = t.TempDir()
	previewDir = t.TempDir()
	thumbSlots = make(chan struct{}, 4)

	a := newViewTestApp()
	a.thumbOnce.Do(func() {})
	a.thumbs = map[string]*thumbEntry{}
	a.thumbLoad = map[string]bool{}
	a.thumbQueue = newThumbQueue()
	// Only distinguishes the real window from a headless view: nothing calls
	// into it while a task is being submitted.
	a.win = &mygo.Window{}
	a.tab().Loaded = true
	return a
}

func testColor() color.RGBA { return color.RGBA{R: 60, G: 120, B: 190, A: 255} }

// writeCachedCard stores a card for it at the requested size, the way a
// previous session would have.
func writeCachedCard(t *testing.T, it *Item, size int) {
	t.Helper()
	writeThumbCache(thumbCachePath(it, size), encodeTestJPEG(t, 40, 30, testColor()))
}

// TestCachedCardTakesTheCacheLane is the point of the change: a card that is
// already on disk must not queue behind a full-resolution render.
func TestCachedCardTakesTheCacheLane(t *testing.T) {
	a := cacheLaneApp(t)
	it := &Item{ID: "lane-1", Name: "a.jpg", Kind: "image", Ext: ".jpg", Path: `C:\photos\a.jpg`, Size: 100}
	writeCachedCard(t, it, 220)

	if !a.submitThumbTask("k1|f", "k1", it, 220, 10, 1, thumbStageFinal, false) {
		t.Fatal("the task was refused")
	}
	if a.thumbFastQueue == nil {
		t.Fatal("the cache lane was not created")
	}
	if pending, _ := a.thumbFastQueue.stats(); pending != 1 {
		t.Fatalf("cache lane holds %d tasks, want 1", pending)
	}
	if pending, _ := a.thumbQueue.stats(); pending != 0 {
		t.Fatalf("decode pool holds %d tasks, want 0", pending)
	}
}

// TestUncachedCardStaysInTheDecodePool covers the other side: a cold card is
// not routed through a lane that would only hand it back.
func TestUncachedCardStaysInTheDecodePool(t *testing.T) {
	a := cacheLaneApp(t)
	it := &Item{ID: "lane-2", Name: "b.jpg", Kind: "image", Ext: ".jpg", Path: `C:\photos\b.jpg`, Size: 100}

	if !a.submitThumbTask("k2|f", "k2", it, 220, 10, 1, thumbStageFinal, false) {
		t.Fatal("the task was refused")
	}
	if pending, _ := a.thumbQueue.stats(); pending != 1 {
		t.Fatalf("decode pool holds %d tasks, want 1", pending)
	}
	if a.thumbFastQueue != nil {
		if pending, _ := a.thumbFastQueue.stats(); pending != 0 {
			t.Fatalf("cache lane holds %d tasks, want 0", pending)
		}
	}
}

// TestCacheLaneAnswersFromDisk covers the lane doing its job: the card lands in
// memory without the decode pool ever seeing it.
func TestCacheLaneAnswersFromDisk(t *testing.T) {
	a := cacheLaneApp(t)
	it := &Item{ID: "lane-3", Name: "c.jpg", Kind: "image", Ext: ".jpg", Path: `C:\photos\c.jpg`, Size: 100}
	k := thumbKey(it, 220)
	writeCachedCard(t, it, 220)

	// With no window the result is applied inline, which is what this test
	// inspects.
	a.win = nil
	a.thumbFastQueue = newThumbQueue()
	task := &thumbTask{key: k + "|f", base: k, item: it, size: 220, stage: thumbStageFinal}
	if !a.serveCachedCard(task) {
		t.Fatal("the cache lane did not answer a cached card")
	}
	if a.thumbs[k] == nil {
		t.Fatal("the bitmap never reached the card cache")
	}
	if pending, _ := a.thumbQueue.stats(); pending != 0 {
		t.Fatalf("the decode pool received %d tasks", pending)
	}
}

// TestCacheLaneHandsAMissToTheDecodePool covers the handoff: a card that is not
// where the stat said it was still gets rendered, exactly once.
func TestCacheLaneHandsAMissToTheDecodePool(t *testing.T) {
	a := cacheLaneApp(t)
	it := &Item{ID: "lane-4", Name: "d.jpg", Kind: "image", Ext: ".jpg", Path: `C:\photos\d.jpg`, Size: 100}
	k := thumbKey(it, 220)
	a.thumbFastQueue = newThumbQueue()
	a.thumbLoad = map[string]bool{k + "|f": true}

	task := &thumbTask{key: k + "|f", base: k, item: it, size: 220, stage: thumbStageFinal}
	if a.serveCachedCard(task) {
		t.Fatal("a missing card was answered by the cache lane")
	}
	if pending, _ := a.thumbQueue.stats(); pending != 1 {
		t.Fatalf("the decode pool received %d tasks, want 1", pending)
	}
	if !a.thumbLoad[k+"|f"] {
		t.Fatal("the load marker was dropped, so the card could be queued twice")
	}
}

// TestRenderedCardKeepsItsPixels covers the first-paint saving: a card that had
// to be rendered comes back with its pixels, and its cache file is left to the
// caller so the bitmap is posted first.
func TestRenderedCardKeepsItsPixels(t *testing.T) {
	thumbDir = t.TempDir()
	thumbSlots = make(chan struct{}, 4)
	dir := t.TempDir()
	path := filepath.Join(dir, "card.jpg")
	data := encodeTestJPEG(t, 400, 300, testColor())
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	it := &Item{ID: "pix-1", Name: "card.jpg", Kind: "image", Ext: ".jpg", Path: path, Size: int64(len(data))}

	rendered, err := renderThumbForCard(context.Background(), it, 220)
	if err != nil {
		t.Fatalf("renderThumbForCard: %v", err)
	}
	if rendered.Cached {
		t.Fatal("a rendered card reported itself as cached")
	}
	if rendered.Img == nil {
		t.Fatal("a rendered card lost its pixels")
	}
	if len(rendered.Data) == 0 {
		t.Fatal("a rendered card has no bytes for the cache")
	}
	if _, err := os.Stat(thumbCachePath(it, 220)); err == nil {
		t.Fatal("the cache file was written before the card was posted")
	}
	writeThumbCache(thumbCachePath(it, 220), rendered.Data)

	// The second read comes from disk and no longer carries pixels.
	again, err := renderThumbForCard(context.Background(), it, 220)
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if !again.Cached {
		t.Fatal("the second read did not come from the cache")
	}
	if again.Img != nil {
		t.Fatal("a cached card reported pixels it never decoded")
	}
}
