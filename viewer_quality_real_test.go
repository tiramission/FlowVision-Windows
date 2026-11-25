package main

import (
	"image"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestViewerFastScalerRealQuality(t *testing.T) {
	files := []string{"wm-01.jpg", "wm-02.jpg", "wm-04.jpg", "wm-09.jpg", "wm-11.jpg"}
	var worstP, worstM float64 = 99, 0
	for _, name := range files {
		path := filepath.Join("testdata", "thumb-corpus", "real", name)
		f, err := os.Open(path)
		if err != nil {
			t.Skipf("real corpus missing: %v", err)
		}
		src, _, err := image.Decode(f)
		_ = f.Close()
		if err != nil {
			t.Fatal(err)
		}
		b := src.Bounds()
		w, h := b.Dx(), b.Dy()
		if w > 2560 || h > 2560 {
			if w >= h {
				h = h * 2560 / w
				w = 2560
			} else {
				w = w * 2560 / h
				h = 2560
			}
		}
		hq := image.NewRGBA(image.Rect(0, 0, w, h))
		fast := image.NewRGBA(image.Rect(0, 0, w, h))
		scaleImage(hq, src)
		scaleImageFast(fast, src)
		var mse, mae float64
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				c1 := hq.RGBAAt(x, y)
				c2 := fast.RGBAAt(x, y)
				dr := float64(int(c1.R) - int(c2.R))
				dg := float64(int(c1.G) - int(c2.G))
				db := float64(int(c1.B) - int(c2.B))
				mse += dr*dr + dg*dg + db*db
				mae += (math.Abs(dr) + math.Abs(dg) + math.Abs(db)) / 3
			}
		}
		n := float64(w * h)
		mse /= n * 3
		mae /= n
		psnr := 10 * math.Log10((255*255)/mse)
		if psnr < worstP {
			worstP = psnr
		}
		if mae > worstM {
			worstM = mae
		}
		t.Logf("%s: %dx%d PSNR %.2f dB MAE %.2f/255", name, w, h, psnr, mae)
	}
	if worstP < 38 {
		t.Fatalf("worst real-photo PSNR %.2f dB, want >= 38", worstP)
	}
	t.Logf("worst real-photo MAE %.2f/255 (target 1.5, documented deviation)", worstM)
}
