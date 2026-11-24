package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBrowseListsDirectFolders(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "2024")
	nested := filepath.Join(sub, "Nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(dir, "a.png"), 40, 30)
	writePNG(t, filepath.Join(sub, "b.png"), 20, 20)

	res := (FS{}).Browse(BrowseRequest{Path: dir, Limit: 100, ShowOther: true})
	if res.Folders != 1 || res.Images != 1 || res.Videos != 0 || res.Others != 0 {
		t.Fatalf("direct folders: folders=%d images=%d videos=%d others=%d", res.Folders, res.Images, res.Videos, res.Others)
	}
	var folder *Item
	for _, it := range res.Items {
		if it.Name == "2024" {
			folder = it
		}
	}
	if folder == nil || folder.Kind != "folder" || folder.Ext != "" {
		t.Fatalf("subfolder missing or misclassified: %+v", folder)
	}
	if res.Total != 2 {
		t.Fatalf("direct total = %d, want file plus folder", res.Total)
	}

	rec := (FS{}).Browse(BrowseRequest{Path: dir, Recursive: true, Limit: 100, ShowOther: true})
	if rec.Folders != 0 || rec.Images != 2 {
		t.Fatalf("recursive should be a flat media wall: folders=%d images=%d", rec.Folders, rec.Images)
	}
	if rec.Total != 2 {
		t.Fatalf("recursive total = %d, want both images without the navigation folder", rec.Total)
	}
}

func TestBrowseHiddenFoldersFollowSetting(t *testing.T) {
	dir := t.TempDir()
	hidden := filepath.Join(dir, ".private")
	if err := os.Mkdir(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	if res := (FS{}).Browse(BrowseRequest{Path: dir, Limit: 100}); res.Folders != 0 {
		t.Fatalf("hidden folder leaked by default: %d", res.Folders)
	}
	if res := (FS{}).Browse(BrowseRequest{Path: dir, Limit: 100, ShowHidden: true}); res.Folders != 1 {
		t.Fatalf("showHidden folder = %d, want 1", res.Folders)
	}
}

func TestSortKeepsFoldersFirst(t *testing.T) {
	list := []*Item{
		{ID: "1", Name: "z.png", Kind: "image"},
		{ID: "2", Name: "b", Kind: "folder"},
		{ID: "3", Name: "a", Kind: "folder"},
		{ID: "4", Name: "m.mp4", Kind: "video"},
	}
	for _, desc := range []bool{false, true} {
		got := append([]*Item(nil), list...)
		sortItems(got, "name", desc)
		if got[0].Kind != "folder" || got[1].Kind != "folder" {
			t.Fatalf("desc=%v folders not first: %v %v %v %v", desc, got[0].Kind, got[1].Kind, got[2].Kind, got[3].Kind)
		}
	}
}

func TestViewerStepSkipsNonMedia(t *testing.T) {
	list := []*Item{
		{ID: "folder", Kind: "folder"},
		{ID: "one", Kind: "image"},
		{ID: "text", Kind: "other"},
		{ID: "clip", Kind: "video"},
	}
	if idx := nextViewerIndex(list, 1, 1); idx != 3 {
		t.Fatalf("next viewer index = %d, want video at 3", idx)
	}
	if idx := nextViewerIndex(list, 3, -1); idx != 1 {
		t.Fatalf("previous viewer index = %d, want image at 1", idx)
	}
	if idx := nextViewerIndex(list, 1, -1); idx != -1 {
		t.Fatalf("previous from first media = %d, want -1", idx)
	}
	pos, total := viewerMediaPosition(list, list[3])
	if pos != 2 || total != 2 {
		t.Fatalf("media position = %d/%d, want 2/2", pos, total)
	}
}

func TestRenameFolderDoesNotPreserveExtension(t *testing.T) {
	folder := &Item{Name: "2024.照片", Kind: "folder"}
	got, err := renameName(folder, "2025")
	if err != "" || got != "2025" {
		t.Fatalf("folder rename = %q, %q", got, err)
	}
	file := &Item{Name: "cover.png", Kind: "image"}
	got, err = renameName(file, "cover")
	if err != "" || got != "cover.png" {
		t.Fatalf("file rename = %q, %q", got, err)
	}
	got, err = renameName(file, "cover.jpg")
	if err == "" {
		t.Fatalf("file extension change was allowed: %q", got)
	}
}

func TestViewerCanvasMetricsFullscreenKeepsInfoClosed(t *testing.T) {
	if w, h := viewerCanvasMetrics(1000, 800, true, true); w != 1000 || h != 800 {
		t.Fatalf("immersive canvas = %vx%v, want 1000x800", w, h)
	}
	if w, h := viewerCanvasMetrics(1000, 800, false, true); w != 700 || h != 698 {
		t.Fatalf("windowed info canvas = %vx%v, want 700x698", w, h)
	}
	if w, h := viewerCanvasMetrics(1000, 800, false, false); w != 1000 || h != 698 {
		t.Fatalf("windowed canvas = %vx%v, want 1000x698", w, h)
	}
}

func TestViewerPercentIsActualScale(t *testing.T) {
	// 100 px image in a 300 px viewport at fit is a real 300% scale.
	if got := viewerPercentValue(100, 100, 0, 1, 300, 300); got != 300 {
		t.Fatalf("fit percent = %d, want 300", got)
	}
	if got := viewerPercentValue(100, 100, 0, 1.25, 300, 300); got != 375 {
		t.Fatalf("zoomed percent = %d, want 375", got)
	}
	// Rotating 200x100 by 90 degrees swaps the fit bounds: a 200x400
	// viewport fits at 200%, not at the old unrotated 100%.
	if got := viewerPercentValue(200, 100, 90, 1, 200, 400); got != 200 {
		t.Fatalf("rotated percent = %d, want 200", got)
	}
	// The 1 key must land on a real 100%, not a 100% relative to fit.
	for _, tc := range []struct{ bw, bh, vw, vh int }{
		{100, 100, 300, 300},  // fit > 1: actual size shrinks to 1:1
		{1000, 800, 500, 400}, // fit < 1: actual size magnifies to 1:1
	} {
		fit := viewerFitScale(tc.bw, tc.bh, 0, float32(tc.vw), float32(tc.vh))
		got := viewerPercentValue(tc.bw, tc.bh, 0, viewerActualZoom(fit), float32(tc.vw), float32(tc.vh))
		if got != 100 {
			t.Fatalf("actual size %dx%d in %dx%d = %d, want 100", tc.bw, tc.bh, tc.vw, tc.vh, got)
		}
	}
}
