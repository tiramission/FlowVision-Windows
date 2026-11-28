package main

import (
	"path/filepath"
	"testing"
)

// TestFolderRowEntriesPlaceTheCurrentFolderInOrder covers the 文件夹 section:
// the current folder sits among its siblings in name order, not pinned to the
// top, so the section reads as a row of places to switch between rather than a
// tree with a root.
func TestFolderRowEntriesPlaceTheCurrentFolderInOrder(t *testing.T) {
	a := newViewTestApp()
	parent := `C:\Pictures`
	current := filepath.Join(parent, "Fig Player")
	a.treeLoaded[parent] = true
	// The file system's order, which the tree keeps.
	a.treeKids[parent] = []TreeEntry{
		{Name: "Camera Roll", Path: filepath.Join(parent, "Camera Roll")},
		{Name: "Feedback", Path: filepath.Join(parent, "Feedback")},
		{Name: "Fig Player", Path: current},
		{Name: "Wallpapers", Path: filepath.Join(parent, "Wallpapers")},
	}

	got, ok, _ := a.folderRowEntries(current, "Fig Player")
	if !ok {
		t.Fatal("no rows for a folder inside a parent")
	}
	var names []string
	for _, e := range got {
		names = append(names, e.entry.Name)
	}
	want := []string{"Camera Roll", "Feedback", "Fig Player", "Wallpapers"}
	if len(names) != len(want) {
		t.Fatalf("rows = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("rows = %v, want %v", names, want)
		}
	}
}

// TestFolderRowEntriesKeepAnUnknownFolder covers a folder the parent's scan has
// not listed yet, as while it is still loading: it is shown rather than lost.
func TestFolderRowEntriesKeepAnUnknownFolder(t *testing.T) {
	a := newViewTestApp()
	parent := `C:\Pictures`
	a.treeLoaded[parent] = true
	a.treeKids[parent] = []TreeEntry{
		{Name: "Wallpapers", Path: filepath.Join(parent, "Wallpapers")},
	}
	got, ok, _ := a.folderRowEntries(filepath.Join(parent, "New"), "New")
	if !ok {
		t.Fatal("no rows")
	}
	if len(got) != 2 || got[0].entry.Name != "New" {
		t.Fatalf("rows = %+v, want the folder first, then its siblings", got)
	}
}

// TestFolderRowEntriesEmptyAtDriveRoot covers the top of a drive: it has no
// parent, so there is nothing to list it among.
func TestFolderRowEntriesEmptyAtDriveRoot(t *testing.T) {
	a := newViewTestApp()
	if _, ok, _ := a.folderRowEntries(`C:\`, "C:"); ok {
		t.Fatal("a drive root reported siblings")
	}
}
