package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// prepareCacheDirs points both buckets at fresh temporary directories.
func prepareCacheDirs(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	thumbDir = filepath.Join(root, "thumbs")
	previewDir = filepath.Join(root, "previews")
	for _, dir := range []string{thumbDir, previewDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return thumbDir, previewDir
}

func TestGridThumbnailsAndViewerPreviewsUseSeparateDirs(t *testing.T) {
	gridDir, previewsDir := prepareCacheDirs(t)
	thumbSlots = make(chan struct{}, 4)

	dir := t.TempDir()
	path := filepath.Join(dir, "photo.png")
	writePNG(t, path, 900, 600)
	it := items.Intern(path)

	if p := thumbPath(it, 240); filepath.Dir(p) != gridDir {
		t.Fatalf("grid thumbnail lives in %s, want %s", filepath.Dir(p), gridDir)
	}
	if p := viewerPreviewCachePath(it, viewerPreviewFastMode); filepath.Dir(p) != previewsDir {
		t.Fatalf("viewer preview lives in %s, want %s", filepath.Dir(p), previewsDir)
	}
	if p := viewerPreviewCachePath(it, viewerPreviewHQMode); filepath.Dir(p) != previewsDir {
		t.Fatalf("hq viewer preview lives in %s, want %s", filepath.Dir(p), previewsDir)
	}
	// The TIFF/legacy preview writer must follow the same bucket.
	if _, err := previewBytes(it); err != nil {
		t.Fatalf("previewBytes: %v", err)
	}
	entries, err := os.ReadDir(previewsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("previews dir has %d files, want 1", len(entries))
	}
	grid, err := os.ReadDir(gridDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(grid) != 0 {
		t.Fatalf("grid dir received preview files: %v", grid)
	}
}

func TestCacheBucketsShareOneBudget(t *testing.T) {
	gridDir, previewsDir := prepareCacheDirs(t)
	for i := 0; i < 3; i++ {
		if err := os.WriteFile(filepath.Join(gridDir, fmt.Sprintf("g%d.jpg", i)), make([]byte, 1024), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(previewsDir, fmt.Sprintf("p%d.jpg", i)), make([]byte, 1024), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	info := thumbCacheInfo()
	if info.Files != 6 {
		t.Fatalf("cache info counted %d files, want 6", info.Files)
	}
	if info.Bytes != 6*1024 {
		t.Fatalf("cache info counted %d bytes, want %d", info.Bytes, 6*1024)
	}
	if info.Dir != filepath.Dir(gridDir) {
		t.Fatalf("cache info dir = %s, want the parent of the buckets", info.Dir)
	}

	cleared, err := clearThumbCache()
	if err != nil || !cleared {
		t.Fatalf("clearThumbCache = %v, %v", cleared, err)
	}
	for _, dir := range []string{gridDir, previewsDir} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Fatalf("%s still has %d files after clear", dir, len(entries))
		}
	}
}

func TestTrimAppliesToBothBuckets(t *testing.T) {
	gridDir, previewsDir := prepareCacheDirs(t)
	// One megabyte each, with the grid files older than the previews.
	for i := 0; i < 4; i++ {
		p := filepath.Join(gridDir, fmt.Sprintf("g%d.jpg", i))
		if err := os.WriteFile(p, make([]byte, 1<<20), 0o644); err != nil {
			t.Fatal(err)
		}
		old := timeAgo(t, i+10)
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 4; i++ {
		p := filepath.Join(previewsDir, fmt.Sprintf("p%d.jpg", i))
		if err := os.WriteFile(p, make([]byte, 1<<20), 0o644); err != nil {
			t.Fatal(err)
		}
		old := timeAgo(t, i)
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}

	// The persisted value is clamped to a 200 MB floor, so the test sets the
	// in-memory copy directly and restores it afterwards.
	prev := getSettings()
	settingsMu.Lock()
	settingsData.CacheLimitMB = 5 // 5 MB budget for 8 MB of files
	settingsMu.Unlock()
	defer func() {
		settingsMu.Lock()
		settingsData.CacheLimitMB = prev.CacheLimitMB
		settingsMu.Unlock()
	}()

	trimThumbCache()

	info := thumbCacheInfo()
	if info.Bytes > 5<<20 {
		t.Fatalf("cache still holds %d bytes, want at most %d", info.Bytes, 5<<20)
	}
	// The oldest files are gone first: the oldest grid files must be the ones
	// removed, not a whole bucket.
	if _, err := os.Stat(filepath.Join(gridDir, "g3.jpg")); err == nil {
		t.Fatal("oldest grid thumbnail survived the trim")
	}
	if _, err := os.Stat(filepath.Join(previewsDir, "p0.jpg")); err != nil {
		t.Fatal("newest preview was removed before older grid thumbnails")
	}
}

func TestMigrateLegacyPreviewsMovesThemOut(t *testing.T) {
	gridDir, previewsDir := prepareCacheDirs(t)
	legacy := []string{
		"abc-preview-fast-1-2-v3.jpg",
		"def-preview-hq-1-2-v3.jpg",
		"xyz-preview-1-2.jpg",
	}
	for _, name := range legacy {
		if err := os.WriteFile(filepath.Join(gridDir, name), []byte("jpeg"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A grid thumbnail must stay where it is.
	keep := "abc-240-1-2-v2.jpg"
	if err := os.WriteFile(filepath.Join(gridDir, keep), []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}

	migrateLegacyPreviews()

	for _, name := range legacy {
		if _, err := os.Stat(filepath.Join(previewsDir, name)); err != nil {
			t.Fatalf("%s was not moved: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(gridDir, name)); err == nil {
			t.Fatalf("%s is still in the grid bucket", name)
		}
	}
	if _, err := os.Stat(filepath.Join(gridDir, keep)); err != nil {
		t.Fatalf("grid thumbnail was moved out of its bucket: %v", err)
	}
}

func TestCacheBudgetsAreGenerousEnough(t *testing.T) {
	// 8 neighbours on each side of a square 2560 preview is 16 × 26 MB.
	const squarePreviewBytes = int64(viewerPreviewMaxDim) * viewerPreviewMaxDim * 4
	if got := int64(viewerPreloadBudget) / squarePreviewBytes; got < 14 {
		t.Fatalf("preload budget fits only %d square previews, want at least 14", got)
	}
	// A full 12-column screen of 640 px cards must fit many times over.
	const bigThumb = 640 * 640 * 4
	if got := thumbCacheBytesMax / bigThumb; got < 300 {
		t.Fatalf("thumb budget fits only %d large cards, want at least 300", got)
	}
	if thumbCacheMax < 1000 {
		t.Fatalf("thumb entry cap = %d, want at least 1000", thumbCacheMax)
	}
}

// timeAgo returns a modification time some seconds in the past.
func timeAgo(t *testing.T, seconds int) time.Time {
	t.Helper()
	return time.Now().Add(-time.Duration(seconds) * time.Second)
}
