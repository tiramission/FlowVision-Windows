package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// wicScaledTestImage writes a large JPEG with enough detail that a scaled
// decode cannot be confused with a blank frame.
func wicScaledTestImage(t *testing.T, w, h int) *Item {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(x * 7 % 256),
				G: uint8(y * 5 % 256),
				B: uint8((x + y) * 3 % 256),
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 88}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "big.jpg")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return &Item{ID: "wic-scaled", Name: "big.jpg", Kind: "image", Ext: ".jpg",
		Path: path, Size: int64(buf.Len())}
}

// TestWICScaledDecodeReducesTheFrame is the point of the change: a large JPEG
// must not be decoded at full size to fill a 220 px card.
func TestWICScaledDecodeReducesTheFrame(t *testing.T) {
	it := wicScaledTestImage(t, 3000, 2000)

	before := wicScaledDecodes.Load()
	start := time.Now()
	rendered, err := wicThumb(it, 220)
	scaled := time.Since(start)
	if err != nil {
		t.Fatalf("wicThumb: %v", err)
	}
	if wicScaledDecodes.Load() == before {
		t.Fatal("the decoder was asked for the full frame, not a reduced one")
	}
	if len(rendered.Data) == 0 {
		t.Fatal("no card produced")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(rendered.Data))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Height != 220 {
		t.Fatalf("card height = %d, want 220", cfg.Height)
	}

	// The escape hatch has to bring the old path back, so a regression can be
	// bisected without a rebuild.
	t.Setenv("FLOWVISION_WIC_SCALED", "off")
	before = wicScaledDecodes.Load()
	start = time.Now()
	fullRendered, err := wicThumb(it, 220)
	full := time.Since(start)
	if err != nil || len(fullRendered.Data) == 0 {
		t.Fatalf("wicThumb (full): %v", err)
	}
	if wicScaledDecodes.Load() != before {
		t.Fatal("the scaled path ran while it was switched off")
	}
	t.Logf("3000x2000 to 220: scaled %s, full %s", scaled, full)
}

// TestWICScaledDecodeMatchesTheFullDecode guards the picture: a reduced decode
// followed by the normal resize has to look like the full decode.
func TestWICScaledDecodeMatchesTheFullDecode(t *testing.T) {
	it := wicScaledTestImage(t, 2400, 1600)

	scaled, err := wicThumb(it, 320)
	if err != nil {
		t.Fatalf("wicThumb: %v", err)
	}
	t.Setenv("FLOWVISION_WIC_SCALED", "off")
	full, err := wicThumb(it, 320)
	if err != nil {
		t.Fatalf("wicThumb (full): %v", err)
	}

	a, err := jpeg.Decode(bytes.NewReader(scaled.Data))
	if err != nil {
		t.Fatal(err)
	}
	b, err := jpeg.Decode(bytes.NewReader(full.Data))
	if err != nil {
		t.Fatal(err)
	}
	if a.Bounds() != b.Bounds() {
		t.Fatalf("scaled card is %v, full card is %v", a.Bounds(), b.Bounds())
	}
	psnr, _, mae := imageDiff(a, b)
	t.Logf("scaled vs full: PSNR %.1f dB, MAE %.2f", psnr, mae)
	// A gradient survives a 1/4 decode; anything below this means the scaled
	// path is dropping content rather than resolution.
	if psnr < 30 {
		t.Fatalf("scaled card differs from the full decode: PSNR %.1f dB", psnr)
	}
}

// TestWICScaledDecodeLeavesSmallImagesAlone covers the guard: when the decoder
// cannot offer a smaller frame, the old scaler path still produces the card.
func TestWICScaledDecodeLeavesSmallImagesAlone(t *testing.T) {
	it := wicScaledTestImage(t, 300, 200)
	before := wicScaledDecodes.Load()
	rendered, err := wicThumb(it, 220)
	if err != nil {
		t.Fatalf("wicThumb: %v", err)
	}
	if len(rendered.Data) == 0 {
		t.Fatal("no card produced")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(rendered.Data))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Height != 220 {
		t.Fatalf("card height = %d, want 220", cfg.Height)
	}
	t.Logf("small image scaled decodes: %d", wicScaledDecodes.Load()-before)
}
