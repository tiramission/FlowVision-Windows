package main

import (
	"image"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// autoFullResApp returns a headless app whose viewer shows one image item.
func autoFullResApp(t *testing.T, it *Item, preview *ui.Bitmap) *App {
	t.Helper()
	a := newViewTestApp()
	tab := a.tab()
	tab.All = []*Item{it}
	tab.Items = tab.All
	a.tab().Selected = map[string]bool{}
	if preview == nil {
		preview = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 64, 64)))
	}
	a.viewer = Viewer{
		Open:   true,
		Item:   it,
		Index:  0,
		Bitmap: preview,
		Zoom:   1,
	}
	return a
}

func waitFullResReady(t *testing.T, a *App, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if a.viewer.FullResReady || a.viewer.FullResUnsupported {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("original was not decoded within %s (loading=%v)", within, a.viewer.FullResLoading)
}

func TestAutoFullResLoadsCurrentOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.png")
	writePNG(t, path, 2700, 1400)
	it := &Item{ID: "auto-1", Name: "big.png", Kind: "image", Path: path, Size: 1, W: 2700, H: 1400}
	a := autoFullResApp(t, it, nil)

	if !a.settings.ViewerAutoFullRes {
		t.Fatal("automatic original loading must default to on")
	}
	if !a.maybeAutoViewerFullRes() {
		t.Fatal("a 2700px image should warm its original")
	}
	waitFullResReady(t, a, 10*time.Second)
	if a.viewer.FullResUnsupported {
		t.Fatal("original decode reported unsupported")
	}
	if a.viewerFullRes == nil || a.viewerFullRes.Bmp == nil {
		t.Fatal("original bitmap was not kept")
	}
	if w, h := a.viewerFullRes.Bmp.Size(); w != 2700 || h != 1400 {
		t.Fatalf("kept bitmap = %dx%d, want 2700x1400", w, h)
	}
	if a.viewer.Bitmap != a.viewerFullRes.Bmp {
		t.Fatal("viewer still draws the preview after the original arrived")
	}
	if a.viewerStats.FullResBytes <= 0 {
		t.Fatalf("FullResBytes = %d, want the decoded size", a.viewerStats.FullResBytes)
	}
	if a.viewer.OneToOne {
		t.Fatal("warming the original must not switch the viewer to 1:1")
	}

	a.clearViewerFullRes()
	if a.viewer.FullResReady || a.viewerFullRes != nil {
		t.Fatal("clearViewerFullRes kept the original")
	}
	if a.viewer.Bitmap != nil {
		t.Fatal("viewer bitmap should be released with the original")
	}
}

func TestAutoFullResSkipsImagesThePreviewCovers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small.png")
	writePNG(t, path, 64, 64)

	small := &Item{ID: "small-1", Name: "small.png", Kind: "image", Path: path, Size: 1, W: 800, H: 600}
	a := autoFullResApp(t, small, nil)
	if a.maybeAutoViewerFullRes() {
		t.Fatal("an 800px image is already the full image through the preview")
	}
	if a.viewer.FullResLoading {
		t.Fatal("decode started for an image the preview already covers")
	}

	// A video never warms a still original.
	video := &Item{ID: "v-1", Name: "clip.mp4", Kind: "video", Path: path, Size: 1}
	b := autoFullResApp(t, video, nil)
	if b.maybeAutoViewerFullRes() {
		t.Fatal("videos must not warm a still original")
	}

	// No preview on screen yet: nothing to replace.
	big := &Item{ID: "big-2", Name: "big.png", Kind: "image", Path: path, Size: 1, W: 4000, H: 3000}
	c := newViewTestApp()
	tab := c.tab()
	tab.All, tab.Items = []*Item{big}, []*Item{big}
	c.viewer = Viewer{Open: true, Item: big}
	if c.maybeAutoViewerFullRes() {
		t.Fatal("original warmed before any preview is on screen")
	}

	// The switch turns it off.
	d := autoFullResApp(t, big, nil)
	d.settings.ViewerAutoFullRes = false
	if d.maybeAutoViewerFullRes() {
		t.Fatal("disabled setting still warmed the original")
	}

	// The viewer being closed ends it too.
	e := autoFullResApp(t, big, nil)
	e.viewer.Open = false
	if e.maybeAutoViewerFullRes() {
		t.Fatal("closed viewer warmed an original")
	}
}

func TestAutoFullResWaitsForNeighbourPreviews(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.png")
	writePNG(t, path, 2700, 1400)
	it := &Item{ID: "auto-2", Name: "big.png", Kind: "image", Path: path, Size: 1, W: 2700, H: 1400}
	a := autoFullResApp(t, it, nil)

	a.viewerPreloadMu.Lock()
	a.viewerPreloadQueue = []viewerPreloadJob{{It: it, Key: "neighbour", Epoch: a.viewerPreloadEpoch}}
	a.viewerPreloadLoading = map[string]bool{"neighbour": true}
	a.viewerPreloadMu.Unlock()
	if a.maybeAutoViewerFullRes() {
		t.Fatal("the original started while neighbour previews were still decoding")
	}

	a.viewerPreloadMu.Lock()
	a.viewerPreloadQueue = nil
	a.viewerPreloadLoading = map[string]bool{}
	a.viewerPreloadMu.Unlock()
	if !a.maybeAutoViewerFullRes() {
		t.Fatal("the original did not start after the previews drained")
	}
	waitFullResReady(t, a, 10*time.Second)
}

func TestAutoFullResFailureStaysQuiet(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "gone.png")
	it := &Item{ID: "auto-3", Name: "gone.png", Kind: "image", Path: missing, Size: 1, W: 3000, H: 2000}
	a := autoFullResApp(t, it, nil)

	if !a.maybeAutoViewerFullRes() {
		t.Fatal("decode did not start")
	}
	waitFullResReady(t, a, 5*time.Second)
	if !a.viewer.FullResUnsupported {
		t.Fatal("a missing file should mark the original unsupported")
	}
	if a.viewer.NoticeKey == "fullres" || a.viewer.Notice.Text != "" {
		t.Fatalf("background failure raised a notice: %+v", a.viewer.Notice)
	}
}

func TestClearViewerFullResDropsWantedKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.png")
	writePNG(t, path, 2700, 1400)
	it := &Item{ID: "auto-4", Name: "big.png", Kind: "image", Path: path, Size: 1, W: 2700, H: 1400}
	a := autoFullResApp(t, it, nil)
	key := viewerFullResKey(it)

	a.viewer.FullResLoading = true
	viewerFullResWanted.Store(key)
	if !viewerFullResIsWanted(key) {
		t.Fatal("key was not registered as wanted")
	}
	a.clearViewerFullRes()
	if viewerFullResIsWanted(key) {
		t.Fatal("a queued decode would still run after the viewer left the image")
	}
}

func TestViewerPreloadSettingsMentionPreviewSize(t *testing.T) {
	a := newViewTestApp()
	var sawPreload, sawAuto bool
	for _, cat := range a.settingsCategories() {
		if cat.ID != "viewer" {
			continue
		}
		for _, group := range cat.Groups {
			for _, item := range group.Items {
				switch item.ID {
				case "viewer-preload":
					sawPreload = true
					if !strings.Contains(item.Description, "2560") || !strings.Contains(item.Description, "不是原图") {
						t.Fatalf("preload description does not say it fetches the 2560 preview: %q", item.Description)
					}
				case "viewer-auto-fullres":
					sawAuto = true
					if !a.settings.ViewerAutoFullRes {
						t.Fatal("automatic original loading must default to on")
					}
					a.fillSettingsDraft(a.settings)
					if a.settingsDirty() {
						t.Fatal("untouched draft reported as dirty")
					}
					a.draft.ViewerAutoFullRes = false
					if !a.settingsDirty() {
						t.Fatal("turning automatic original loading off must count as an unsaved change")
					}
				}
			}
		}
	}
	if !sawPreload {
		t.Fatal("neighbour preload setting missing")
	}
	if !sawAuto {
		t.Fatal("automatic original setting missing")
	}
}

func TestViewerAutoFullResSettingSurvivesRoundTrip(t *testing.T) {
	data := []byte(`{"viewerPreload":2}`)
	s, ok := parseSettings(data)
	if !ok {
		t.Fatal("settings did not parse")
	}
	if !s.ViewerAutoFullRes {
		t.Fatal("a settings file without viewerAutoFullRes must default to on")
	}
	off, _ := parseSettings([]byte(`{"viewerAutoFullRes":false}`))
	if off.ViewerAutoFullRes {
		t.Fatal("an explicit false was not honored")
	}
}

func TestParseSettingsToleratesBOM(t *testing.T) {
	data := append([]byte{0xef, 0xbb, 0xbf}, []byte(`{"viewerAutoFullRes":false,"thumbPrefetch":false}`)...)
	s, ok := parseSettings(data)
	if !ok {
		t.Fatal("a BOM-prefixed settings file was rejected")
	}
	if s.ViewerAutoFullRes || s.ThumbPrefetch {
		t.Fatalf("BOM reset the file to defaults: autoFullRes=%v thumbPrefetch=%v", s.ViewerAutoFullRes, s.ThumbPrefetch)
	}
}
