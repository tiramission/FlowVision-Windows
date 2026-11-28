package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newPrefetchTestApp builds the smallest App the background fill needs. Priming
// thumbOnce keeps the worker goroutines out of the test; runThumbTask is driven
// by hand so the fill is deterministic.
func newPrefetchTestApp(t *testing.T, tab *Tab) *App {
	t.Helper()
	a := &App{
		settings:   Settings{ThumbSize: 160, ThumbPrefetch: true},
		thumbs:     map[string]*thumbEntry{},
		thumbLoad:  map[string]bool{},
		thumbQueue: newThumbQueue(),
		tabs:       []*Tab{tab},
		active:     0,
	}
	a.thumbOnce.Do(func() {})
	return a
}

// drainPrefetch runs queued fills the way thumbWorker would, without the
// goroutines, until the folder is finished.
func drainPrefetch(a *App) int {
	ran := 0
	for {
		task := a.thumbQueue.popNow()
		if task == nil {
			return ran
		}
		if a.prefetchActive > a.fillWidth(time.Now()) {
			panic("prefetch width exceeded")
		}
		a.runThumbTask(task)
		ran++
		if ran > 10000 {
			panic("prefetch did not converge")
		}
	}
}

func TestRetainKeepsBackgroundFills(t *testing.T) {
	q := newThumbQueue()
	q.submit(&thumbTask{key: "visible|f", frame: 1})
	q.submit(&thumbTask{key: "fill|f", frame: 1, preload: true})

	// The next grid frame only touches the visible card; the fill must survive
	// because no frame ever claims it.
	if dropped := q.retain(2); len(dropped) != 1 || dropped[0] != "visible|f" {
		t.Fatalf("retain dropped %v, want [visible|f]", dropped)
	}
	if task := q.popNow(); task == nil || task.key != "fill|f" {
		t.Fatalf("background fill did not survive retain: %+v", task)
	}

	q2 := newThumbQueue()
	q2.submit(&thumbTask{key: "a|f", preload: true})
	q2.submit(&thumbTask{key: "b|f", preload: true})
	if got := q2.dropPreload(); len(got) != 2 {
		t.Fatalf("dropPreload returned %v, want both fills", got)
	}
	if task := q.popNow(); task != nil {
		t.Fatalf("dropPreload left %q queued", task.key)
	}

	// A visible card always outranks a background fill, whatever order they
	// were submitted in.
	q3 := newThumbQueue()
	q3.submit(&thumbTask{key: "fill|f", priority: thumbPrefetchBase + 1, preload: true})
	q3.submit(&thumbTask{key: "visible|f", priority: 4200})
	if task := q3.popNow(); task == nil || task.key != "visible|f" {
		t.Fatalf("pop took %+v, want the visible card first", task)
	}
}

func TestPrefetchFillsWholeFolder(t *testing.T) {
	dir := t.TempDir()
	thumbDir = t.TempDir()
	previewDir = thumbDir
	thumbSlots = make(chan struct{}, 4)

	for _, spec := range []struct {
		name string
		w, h int
	}{
		{"a.png", 400, 300},
		{"b.png", 200, 500},
		{"c.png", 300, 300},
		{"d.png", 640, 360},
		{"e.png", 100, 800},
		{"f.png", 320, 240},
	} {
		writePNG(t, filepath.Join(dir, spec.name), spec.w, spec.h)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := (FS{}).Browse(BrowseRequest{Path: dir, Limit: 100, ShowOther: true})
	if res.Error != "" {
		t.Fatalf("browse error: %s", res.Error)
	}
	images := 0
	for _, it := range res.Items {
		if it.Kind == "image" {
			images++
		}
	}
	if images != 6 {
		t.Fatalf("browse found %d images, want 6", images)
	}

	tab := &Tab{ID: 1, Path: dir, Loaded: true, Gen: 1, All: res.Items, Items: res.Items, Selected: map[string]bool{}}
	a := newPrefetchTestApp(t, tab)

	a.pumpThumbPrefetch(tab)
	if a.prefetchActive == 0 || a.prefetchNext == 0 {
		t.Fatalf("pump queued nothing: next=%d active=%d", a.prefetchNext, a.prefetchActive)
	}
	if width := a.fillWidth(time.Now()); a.prefetchActive > width {
		t.Fatalf("active fills = %d, want <= %d", a.prefetchActive, width)
	}
	// The first batch must already be the window's worth of media items.
	if a.prefetchNext > len(tab.All) {
		t.Fatalf("cursor %d past %d items", a.prefetchNext, len(tab.All))
	}

	drainPrefetch(a)

	if a.prefetchNext != len(tab.All) {
		t.Fatalf("fill stopped at %d of %d items", a.prefetchNext, len(tab.All))
	}
	if a.prefetchActive != 0 {
		t.Fatalf("active fills left over: %d", a.prefetchActive)
	}
	for _, it := range res.Items {
		_, err := os.Stat(thumbPath(it, 160))
		if it.Kind == "image" && err != nil {
			t.Errorf("%s missing from the disk cache: %v", it.Name, err)
		}
	}
	// Folders and generic files must never be decoded.
	if n := len(a.thumbLoad); n != 0 {
		t.Fatalf("thumbLoad still has %d entries", n)
	}
	if n := len(a.thumbs); n != 0 {
		t.Fatalf("background fill kept %d bitmaps in memory, want 0", n)
	}

	files := 0
	entries, err := os.ReadDir(thumbDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			files++
		}
	}
	if files != images {
		t.Fatalf("disk cache has %d files, want %d (one per image)", files, images)
	}
}

func TestPrefetchWindowBoundsPendingWork(t *testing.T) {
	thumbDir = t.TempDir()
	previewDir = thumbDir
	thumbSlots = make(chan struct{}, 4)

	items := make([]*Item, 0, 200)
	for i := 0; i < 200; i++ {
		items = append(items, &Item{ID: fmt.Sprintf("prefetch-%d", i), Name: "p.png", Kind: "image", Size: 1, W: 10, H: 10})
	}
	tab := &Tab{ID: 1, Loaded: true, Gen: 1, All: items, Items: items, Selected: map[string]bool{}}
	a := newPrefetchTestApp(t, tab)

	// Repeated builds must not stack fills beyond the current width.
	width := a.fillWidth(time.Now())
	for i := 0; i < 20; i++ {
		a.pumpThumbPrefetch(tab)
	}
	if a.prefetchActive != width {
		t.Fatalf("active fills = %d, want %d", a.prefetchActive, width)
	}
	pending, running := a.thumbQueue.stats()
	if pending != width || running != 0 {
		t.Fatalf("queue pending=%d running=%d, want %d waiting", pending, running, width)
	}
	if a.prefetchNext != width {
		t.Fatalf("cursor = %d, want %d", a.prefetchNext, width)
	}
}

func TestClearCacheDoesNotRefillTheFolder(t *testing.T) {
	dir := t.TempDir()
	thumbDir = t.TempDir()
	previewDir = thumbDir
	thumbSlots = make(chan struct{}, 4)
	writePNG(t, filepath.Join(dir, "one.png"), 200, 200)
	writePNG(t, filepath.Join(dir, "two.png"), 200, 200)

	res := (FS{}).Browse(BrowseRequest{Path: dir, Limit: 100})
	if res.Error != "" {
		t.Fatalf("browse error: %s", res.Error)
	}
	tab := &Tab{ID: 1, Path: dir, Loaded: true, Gen: 3, All: res.Items, Items: res.Items, Selected: map[string]bool{}}
	a := newPrefetchTestApp(t, tab)

	a.pumpThumbPrefetch(tab)
	if a.prefetchActive == 0 {
		t.Fatal("fill did not start")
	}
	a.clearCache()
	drainPrefetch(a)

	// The explicit clear wins: nothing may be written back into the folder the
	// user just emptied, until they browse it again.
	a.pumpThumbPrefetch(tab)
	if a.prefetchActive != 0 {
		t.Fatalf("clear did not stop the fill: active=%d", a.prefetchActive)
	}
	entries, err := os.ReadDir(thumbDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		t.Fatalf("cache file written after clear: %s", e.Name())
	}

	// Opening the folder again starts a fresh pass.
	tab.Gen++
	a.pumpThumbPrefetch(tab)
	if a.prefetchActive == 0 {
		t.Fatal("reopening the folder did not restart the fill")
	}
}

func TestThumbPrefetchSettingIsInSettings(t *testing.T) {
	a := newViewTestApp()
	for _, cat := range a.settingsCategories() {
		if cat.ID != "system" {
			continue
		}
		for _, group := range cat.Groups {
			for _, item := range group.Items {
				if item.ID != "thumb-prefetch" {
					continue
				}
				if !a.settings.ThumbPrefetch {
					t.Fatal("background prefetch must default to on")
				}
				a.fillSettingsDraft(a.settings)
				if a.settingsDirty() {
					t.Fatal("untouched draft reported as dirty")
				}
				a.draft.ThumbPrefetch = false
				if !a.settingsDirty() {
					t.Fatal("turning prefetch off must count as an unsaved change")
				}
				return
			}
		}
	}
	t.Fatal("background prefetch setting missing from the cache category")
}

func TestPrefetchStopsWhenFolderChanges(t *testing.T) {
	dir := t.TempDir()
	thumbDir = t.TempDir()
	previewDir = thumbDir
	thumbSlots = make(chan struct{}, 4)
	writePNG(t, filepath.Join(dir, "one.png"), 200, 200)
	writePNG(t, filepath.Join(dir, "two.png"), 200, 200)

	first := (FS{}).Browse(BrowseRequest{Path: dir, Limit: 100})
	if first.Error != "" {
		t.Fatalf("browse error: %s", first.Error)
	}
	tab := &Tab{ID: 1, Path: dir, Loaded: true, Gen: 1, All: first.Items, Items: first.Items, Selected: map[string]bool{}}
	a := newPrefetchTestApp(t, tab)

	a.pumpThumbPrefetch(tab)
	if a.prefetchActive == 0 {
		t.Fatal("first folder queued nothing")
	}

	// A new listing of another folder must drop the old queue instead of
	// letting it run to 6000 files.
	other := &Tab{ID: 2, Path: dir, Loaded: true, Gen: 7, All: first.Items[:1], Items: first.Items[:1], Selected: map[string]bool{}}
	a.pumpThumbPrefetch(other)
	if a.prefetchTab != other || a.prefetchGen != 7 {
		t.Fatalf("fill still bound to %+v gen %d", a.prefetchTab, a.prefetchGen)
	}
	for _, key := range a.thumbQueue.dropPreload() {
		if key == thumbKey(first.Items[1], 160)+"|f" {
			t.Fatalf("queued fill for the previous folder survived: %s", key)
		}
	}

	// Turning the feature off cancels the rest immediately.
	a.settings.ThumbPrefetch = false
	a.pumpThumbPrefetch(other)
	if a.prefetchActive != 0 || a.prefetchNext != 0 {
		t.Fatalf("disabled fill still running: next=%d active=%d", a.prefetchNext, a.prefetchActive)
	}
	if pending, _ := a.thumbQueue.stats(); pending != 0 {
		t.Fatalf("%d fills left queued after disabling", pending)
	}
}
