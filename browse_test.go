package main

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 255), uint8(y % 255), 128, 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestBrowseAndThumbs(t *testing.T) {
	dir := t.TempDir()
	thumbDir = t.TempDir()
	thumbSlots = make(chan struct{}, 4)

	wide := filepath.Join(dir, "wide.png")
	tall := filepath.Join(dir, "tall.png")
	small := filepath.Join(dir, "small.png")
	writePNG(t, wide, 800, 400)
	writePNG(t, tall, 300, 600)
	writePNG(t, small, 64, 64)

	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(sub, "nested.png"), 200, 200)

	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.png"), []byte("not a png"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := (FS{}).Browse(BrowseRequest{Path: dir, Limit: 100})
	if res.Error != "" {
		t.Fatalf("browse error: %s", res.Error)
	}
	if res.Images != 4 {
		t.Fatalf("images = %d, want 4 (broken.png is still an image by extension)", res.Images)
	}
	if res.Others != 0 {
		t.Fatalf("others = %d, want 0 (other files hidden)", res.Others)
	}

	byName := map[string]*Item{}
	for _, it := range res.Items {
		byName[it.Name] = it
	}
	if byName["wide.png"].W != 800 || byName["wide.png"].H != 400 {
		t.Fatalf("wide dims = %dx%d", byName["wide.png"].W, byName["wide.png"].H)
	}
	if byName["tall.png"].W != 300 || byName["tall.png"].H != 600 {
		t.Fatalf("tall dims = %dx%d", byName["tall.png"].W, byName["tall.png"].H)
	}

	rec := (FS{}).Browse(BrowseRequest{Path: dir, Recursive: true, Limit: 100, ShowOther: true})
	if rec.Images != 5 {
		t.Fatalf("recursive images = %d, want 5", rec.Images)
	}
	if rec.Others != 1 {
		t.Fatalf("recursive others = %d, want 1", rec.Others)
	}

	img := byName["wide.png"]
	data, err := getThumb(context.Background(), img, 240)
	if err != nil {
		t.Fatalf("getThumb: %v", err)
	}
	if len(data) < 100 {
		t.Fatalf("thumbnail looks empty: %d bytes", len(data))
	}
	if _, err := os.Stat(thumbPath(img, thumbBudget(240))); err != nil {
		t.Fatalf("thumbnail not cached on disk: %v", err)
	}

	vid := filepath.Join(dir, "movie.mp4")
	if err := os.WriteFile(vid, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	vres := (FS{}).Browse(BrowseRequest{Path: dir, Limit: 100})
	var vItem *Item
	for _, it := range vres.Items {
		if it.Name == "movie.mp4" {
			vItem = it
		}
	}
	if vItem == nil {
		t.Fatal("video item missing from listing")
	}
	vdata, err := getThumb(context.Background(), vItem, 240)
	if err != nil || len(vdata) < 100 {
		t.Fatalf("video placeholder failed: %d bytes, err=%v", len(vdata), err)
	}
}
