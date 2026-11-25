package main

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestScaleImageFastQuality(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2400, 1600))
	for y := 0; y < 1600; y++ {
		for x := 0; x < 2400; x++ {
			r := uint8(80 + 60*math.Sin(float64(x)/70))
			g := uint8(90 + 50*math.Sin(float64(y)/90+0.7))
			b := uint8(100 + 40*math.Sin(float64(x+y)/110))
			src.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	hq := image.NewRGBA(image.Rect(0, 0, 1200, 800))
	fast := image.NewRGBA(image.Rect(0, 0, 1200, 800))
	scaleImage(hq, src)
	scaleImageFast(fast, src)

	var mse float64
	for y := 0; y < 800; y++ {
		for x := 0; x < 1200; x++ {
			c1 := hq.RGBAAt(x, y)
			c2 := fast.RGBAAt(x, y)
			dr := float64(int(c1.R) - int(c2.R))
			dg := float64(int(c1.G) - int(c2.G))
			db := float64(int(c1.B) - int(c2.B))
			mse += dr*dr + dg*dg + db*db
		}
	}
	mse /= float64(1200 * 800 * 3)
	if mse == 0 {
		t.Fatal("fast scaler produced an identical image")
	}
	psnr := 10 * math.Log10((255*255)/mse)
	if psnr < 30 {
		t.Fatalf("fast scaler PSNR %.2f dB, want >= 30", psnr)
	}
}
