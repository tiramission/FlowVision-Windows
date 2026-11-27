package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func viewerBenchItems(b *testing.B) []*Item {
	root := os.Getenv("FLOWVISION_VIEWER_CORPUS")
	if root == "" {
		root = filepath.Join("testdata", "thumb-corpus", "real")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		b.Skip("viewer corpus missing:", err)
	}
	var files []string
	for _, e := range entries {
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if e.IsDir() || (ext != ".jpg" && ext != ".jpeg") {
			continue
		}
		// The exif-samples thumbnails are 59–480px; they are useful for
		// orientation fixtures but would skew a 2560 preview benchmark.
		if strings.HasPrefix(strings.ToLower(name), "cam-") {
			continue
		}
		files = append(files, filepath.Join(root, name))
	}
	sort.Strings(files)
	if len(files) == 0 {
		b.Skip("viewer corpus has no JPEG")
	}
	var out []*Item
	for _, p := range files {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		d := probeImageDims(p)
		out = append(out, &Item{
			ID: shortID(p), Name: filepath.Base(p), Kind: "image", Path: p,
			Size: st.Size(), Mtime: st.ModTime().Unix(), W: d.W, H: d.H, Orient: d.Orient,
		})
	}
	return out
}

func BenchmarkViewerPreviewCold(b *testing.B) {
	thumbDir = b.TempDir()
	previewDir = thumbDir
	b.Setenv("FLOWVISION_VIEWER_PREVIEW", "fast")
	b.Setenv("FLOWVISION_VIEWER_SCALER", "fast")
	b.Setenv("FLOWVISION_VIEWER_CACHE", "off")
	items := viewerBenchItems(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		it := items[i%len(items)]
		p := viewerPreviewCachePath(it, "fast")
		_ = os.Remove(p)
		res, err := loadViewerPreview(it)
		if err != nil {
			b.Fatal(err)
		}
		if res.Bmp != nil {
			res.Bmp.ReleasePixels()
		}
	}
}

func BenchmarkViewerPreviewWarm(b *testing.B) {
	thumbDir = b.TempDir()
	previewDir = thumbDir
	b.Setenv("FLOWVISION_VIEWER_PREVIEW", "fast")
	b.Setenv("FLOWVISION_VIEWER_SCALER", "fast")
	items := viewerBenchItems(b)
	if len(items) == 0 {
		b.Skip("no items")
	}
	for _, it := range items {
		res, err := loadViewerPreview(it)
		if err != nil {
			b.Fatal(err)
		}
		if res.Bmp != nil {
			res.Bmp.ReleasePixels()
		}
		p := viewerPreviewCachePath(it, "fast")
		for i := 0; i < 100; i++ {
			if _, err := os.Stat(p); err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		it := items[i%len(items)]
		res, err := loadViewerPreview(it)
		if err != nil {
			b.Fatal(err)
		}
		if res.Bmp != nil {
			res.Bmp.ReleasePixels()
		}
	}
}
