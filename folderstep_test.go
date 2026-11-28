package main

import (
	"os"
	"path/filepath"
	"testing"
)

// folderStepApp builds a tab on a folder with one image, beside another folder
// that also holds one, so a folder step has somewhere to go.
func folderStepApp(t *testing.T) (a *App, here, next string) {
	t.Helper()
	root := t.TempDir()
	here = filepath.Join(root, "a-here")
	next = filepath.Join(root, "b-next")
	for _, dir := range []string{here, next} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writePNG(t, filepath.Join(here, "one.png"), 8, 8)
	writePNG(t, filepath.Join(next, "first.png"), 8, 8)
	writePNG(t, filepath.Join(next, "second.png"), 8, 8)

	a = newViewTestApp()
	a.tabs[0].Path = here
	a.tabs[0].Name = "a-here"
	return a, here, next
}

// TestViewerFolderStepGoesForward covers the step that leaves a folder: at the
// last picture it moves into the next folder and opens its first picture, and
// the tab follows so the grid shows the same folder.
func TestViewerFolderStepGoesForward(t *testing.T) {
	a, here, next := folderStepApp(t)
	listing := (FS{}).Browse(BrowseRequest{Path: here, Limit: 100})
	if len(listing.Items) == 0 {
		t.Skip("no listing")
	}
	a.viewer.Open = true
	a.viewer.Item = listing.Items[0]
	a.viewer.Index = 0
	a.tabs[0].Items = listing.Items
	a.tabs[0].All = listing.Items
	a.tabs[0].Result = listing
	a.tabs[0].Loaded = true

	if !a.viewerFolderStepPossible(1) {
		t.Fatal("a neighbouring folder was not found")
	}
	a.viewerFolderStep(1)

	if a.tab().Path != next {
		t.Fatalf("tab stayed on %q", a.tab().Path)
	}
	if a.viewer.Item == nil || a.viewer.Item.Name != "first.png" {
		t.Fatalf("opened %v, want the next folder's first picture", a.viewer.Item)
	}
}

// TestViewerFolderStepGoesBackward covers the other end: the last picture of
// the folder before.
func TestViewerFolderStepGoesBackward(t *testing.T) {
	a, here, _ := folderStepApp(t)
	// Start in the second folder, going back into the first.
	next := filepath.Join(filepath.Dir(here), "b-next")
	listing := (FS{}).Browse(BrowseRequest{Path: next, Limit: 100})
	if len(listing.Items) < 2 {
		t.Skip("not enough pictures")
	}
	a.tabs[0].Path = next
	a.tabs[0].Items = listing.Items
	a.tabs[0].All = listing.Items
	a.tabs[0].Result = listing
	a.tabs[0].Loaded = true
	a.viewer.Open = true
	a.viewer.Item = listing.Items[0]
	a.viewer.Index = 0

	if !a.viewerFolderStepPossible(-1) {
		t.Fatal("no folder before this one")
	}
	a.viewerFolderStep(-1)

	if a.tab().Path != here {
		t.Fatalf("tab stayed on %q", a.tab().Path)
	}
	if a.viewer.Item == nil || a.viewer.Item.Name != "one.png" {
		t.Fatalf("opened %v, want the previous folder's last picture", a.viewer.Item)
	}
}

// TestViewerFolderStepStopsAtTheEnd covers the boundary: with no neighbour the
// step is not offered, so the button stays disabled rather than lying.
func TestViewerFolderStepStopsAtTheEnd(t *testing.T) {
	a, _, _ := folderStepApp(t)
	// A lone folder in its own parent has no siblings.
	lone := filepath.Join(t.TempDir(), "only")
	if err := os.MkdirAll(lone, 0o755); err != nil {
		t.Fatal(err)
	}
	a.tabs[0].Path = lone
	if a.viewerFolderStepPossible(1) || a.viewerFolderStepPossible(-1) {
		t.Fatal("a lone folder reported a neighbour")
	}
}
