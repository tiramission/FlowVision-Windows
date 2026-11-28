package main

import (
	"image"
	"testing"

	"github.com/egoist/mygo/ui"
)

// viewerStripApp builds a viewer holding one picture, with a preload window of
// two neighbours on each side and none of them decoded yet.
func viewerStripApp(t *testing.T, want int, ready int) *App {
	t.Helper()
	a := newViewerGestureApp(t)
	a.viewer.Open = true
	a.viewer.Item = &Item{ID: "cur", Name: "cur.jpg", Kind: "image", Path: `C:\Pictures\cur.jpg`}
	a.viewer.Bitmap = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 8, 8)))

	a.viewerPreloadMu.Lock()
	a.viewerPreload = map[string]*viewerPreloadEntry{}
	a.viewerPreloadDesired = map[string]bool{}
	a.viewerPreloadDesired[viewerPreloadKey(a.viewer.Item)] = true
	for i := 0; i < want; i++ {
		key := viewerPreloadKey(&Item{ID: "n" + string(rune('a'+i)), Name: "n.jpg", Kind: "image"})
		a.viewerPreloadDesired[key] = true
		if i < ready {
			a.viewerPreload[key] = &viewerPreloadEntry{At: 1}
		}
	}
	a.viewerPreloadMu.Unlock()
	return a
}

// TestViewerPreloadProgressCountsNeighbours covers the counter: the picture on
// screen is not one of the neighbours being preloaded, and only decoded entries
// count as ready.
func TestViewerPreloadProgressCountsNeighbours(t *testing.T) {
	a := viewerStripApp(t, 4, 0)
	defer a.closeViewer()
	if ready, want := a.viewerPreloadProgress(); ready != 0 || want != 4 {
		t.Fatalf("progress = %d/%d, want 0/4", ready, want)
	}
	if text, _ := a.viewerPreloadLabel(); text != "预取 0/4" {
		t.Fatalf("label = %q", text)
	}

	a.viewerPreloadMu.Lock()
	keys := make([]string, 0, 4)
	for key := range a.viewerPreloadDesired {
		if key != viewerPreloadKey(a.viewer.Item) {
			keys = append(keys, key)
		}
	}
	a.viewerPreload[keys[0]] = &viewerPreloadEntry{At: 1}
	a.viewerPreload[keys[1]] = &viewerPreloadEntry{At: 2}
	a.viewerPreloadMu.Unlock()

	if text, tip := a.viewerPreloadLabel(); text != "预取 2/4" {
		t.Fatalf("label = %q", text)
	} else if tip == "" {
		t.Fatal("a pending preload has no tooltip")
	}

	// With nothing to preload the readout disappears entirely.
	a.viewerPreloadMu.Lock()
	a.viewerPreloadDesired = map[string]bool{}
	a.viewerPreloadMu.Unlock()
	if text, _ := a.viewerPreloadLabel(); text != "" {
		t.Fatalf("label = %q with no neighbours, want nothing", text)
	}
}

// TestViewerFooterKeepsStatusWhenChromeHides is the point of the change: in the
// immersive layout the controls fade with the pointer, but the picture's state
// and the preload counter stay on screen.
func TestViewerFooterKeepsStatusWhenChromeHides(t *testing.T) {
	a := viewerStripApp(t, 2, 1)
	defer a.closeViewer()
	a.viewer.Immersive = true

	a.viewer.FullResReady = false
	a.viewer.ChromeVisible = true
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if !tt.HasText("大图") {
		t.Fatalf("the control bar lost the quality readout: %q", tt.Texts())
	}
	if !tt.HasText("预取 1/2") {
		t.Fatalf("the control bar lost the preload readout: %q", tt.Texts())
	}
	// The controls themselves are there while the pointer is active.
	if !tt.HasText("适合窗口 (0)") {
		t.Fatalf("the control bar is missing its buttons: %q", tt.Texts())
	}

	a.viewer.ChromeVisible = false
	tt.Frame()
	if !tt.HasText("大图") || !tt.HasText("预取 1/2") {
		t.Fatalf("the readouts vanished with the controls: %q", tt.Texts())
	}
	if tt.HasText("适合窗口 (0)") {
		t.Fatal("the controls stayed on screen after the chrome hid")
	}
}

// TestViewerFooterIsQuietForVideos covers the empty case: a video has no
// quality readout, and a video with nothing preloading leaves no strip behind.
func TestViewerFooterIsQuietForVideos(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewer.Open = true
	a.viewer.Immersive = true
	a.viewer.ChromeVisible = false
	a.viewer.Item = &Item{ID: "v", Name: "v.mp4", Kind: "video", Path: `C:\Pictures\v.mp4`}
	a.viewerPreloadMu.Lock()
	a.viewerPreloadDesired = map[string]bool{}
	a.viewerPreloadMu.Unlock()

	if len(a.viewerStatusItems()) != 0 {
		t.Fatalf("a video reported %d readouts", len(a.viewerStatusItems()))
	}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if tt.HasText("大图") || tt.HasText("原图") || tt.HasText("预取") {
		t.Fatalf("a video drew a picture readout: %q", tt.Texts())
	}
}
