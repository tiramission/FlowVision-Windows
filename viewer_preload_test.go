package main

import (
	"testing"
)

func TestViewerPreloadItemsWalkBothSides(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	items := []*Item{
		{ID: "0", Kind: "image"},
		{ID: "1", Kind: "video"},
		{ID: "2", Kind: "image"},
		{ID: "3", Kind: "image"},
		{ID: "4", Kind: "image"},
		{ID: "5", Kind: "other"},
		{ID: "6", Kind: "image"},
	}
	tab := a.tab()
	tab.Items = items
	a.viewer.Item = items[3]
	a.viewer.Index = 3
	got := a.viewerPreloadItems(2)
	if len(got) != 4 {
		var ids []string
		for _, it := range got {
			ids = append(ids, it.ID)
		}
		t.Fatalf("preload items = %d %v (prev from 2 = %d, len=%d), want 4", len(got), ids, nextViewerIndex(items, 2, -1), len(items))
	}
	want := []string{"2", "0", "4", "6"}
	for i, it := range got {
		if it.ID != want[i] {
			t.Fatalf("preload[%d] = %s, want %s", i, it.ID, want[i])
		}
	}
}

func TestViewerPreloadWindowFollowsCurrentImage(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	tab := a.tab()
	items := make([]*Item, 0, 7)
	for i := 0; i < 7; i++ {
		id := string(rune('0' + i))
		items = append(items, &Item{ID: id, Kind: "image", Path: tab.Items[0].Path, Size: 1, W: 100, H: 100})
	}
	tab.Items = items
	a.viewer.Item = items[3]
	a.viewer.Index = 3
	a.settings.ViewerPreload = 2
	a.scheduleViewerPreload()
	if len(a.viewerPreloadDesired) != 5 {
		t.Fatalf("desired preload keys = %d, want 5 (current plus two each side)", len(a.viewerPreloadDesired))
	}
	if !a.viewerPreloadDesired[viewerPreloadKey(items[1])] {
		t.Fatal("second previous image was not preloaded")
	}
	if a.viewerPreloadDesired[viewerPreloadKey(items[0])] {
		t.Fatal("third previous image was preloaded beyond the configured window")
	}

	stale := viewerPreloadKey(items[0])
	a.viewerPreload[stale] = &viewerPreloadEntry{}
	a.viewer.Item = items[4]
	a.viewer.Index = 4
	a.scheduleViewerPreload()
	if _, ok := a.viewerPreload[stale]; ok {
		t.Fatal("stale preload entry survived the new window")
	}
}

func TestViewerPreloadZeroDisablesNeighbors(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.settings.ViewerPreload = 0
	a.scheduleViewerPreload()
	if len(a.viewerPreloadDesired) != 1 {
		t.Fatalf("preload off kept %d keys, want only current", len(a.viewerPreloadDesired))
	}
}
