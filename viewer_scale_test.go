package main

import (
	"math"
	"testing"
)

// TestActualSizeIsOneDevicePixelPerPicturePixel is the point of the change: on
// a scaled display "100%" used to mean one picture pixel per DIP, which the
// renderer then drew as 1.5 or 2 device pixels. Actual size has to mean what it
// means in every other image viewer.
func TestActualSizeIsOneDevicePixelPerPicturePixel(t *testing.T) {
	const (
		bw, bh = 20000, 15000
		availW = 1200
		availH = 800
	)
	fit := viewerFitScale(bw, bh, 0, availW, availH)
	if fit <= 0 {
		t.Fatal("fit scale is zero")
	}

	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		zoom := viewerActualZoom(fit, scale)
		// Device pixels the picture occupies per picture pixel.
		devicePerPixel := fit * zoom * scale
		if math.Abs(float64(devicePerPixel)-1) > 0.001 {
			t.Fatalf("at device scale %.2f, 100%% shows %.3f device pixels per picture pixel",
				scale, devicePerPixel)
		}
		if pct := viewerPercentValue(bw, bh, bw, bh, 0, zoom, availW, availH, scale); pct != 100 {
			t.Fatalf("at device scale %.2f, actual size reads %d%%, want 100%%", scale, pct)
		}
	}
}

// TestPercentCountsDevicePixels covers the readout: the same picture at the
// same place reads higher on a scaled display, because it really does cover
// more pixels on the glass.
func TestPercentCountsDevicePixels(t *testing.T) {
	const (
		bw, bh = 1000, 1000
		availW = 500
		availH = 500
	)
	// Fitted: 500 DIP wide, so 50% of the picture per DIP.
	for _, tc := range []struct {
		scale float32
		want  int
	}{{1, 50}, {1.5, 75}, {2, 100}} {
		got := viewerPercentValue(bw, bh, bw, bh, 0, 1, availW, availH, tc.scale)
		if got != tc.want {
			t.Fatalf("fitted at scale %.2f reads %d%%, want %d%%", tc.scale, got, tc.want)
		}
	}
}

// TestZoomCeilingStillReachesActualSize covers the interaction with the
// ceiling: a scaled display needs a smaller relative zoom for 100%, and the
// clamp must not stop short of it.
func TestZoomCeilingStillReachesActualSize(t *testing.T) {
	for _, scale := range []float32{1, 1.5, 2} {
		for _, dims := range [][2]int{{20000, 15000}, {6000, 4000}, {800, 600}} {
			fit := viewerFitScale(dims[0], dims[1], 0, 1200, 800)
			actual := viewerActualZoom(fit, scale)
			if got := clampViewerZoom(actual, fit, scale); math.Abs(float64(got-actual)) > 0.001 {
				t.Fatalf("%dx%d at scale %.2f: actual %v clamped to %v", dims[0], dims[1], scale, actual, got)
			}
		}
	}
}

// TestPreviewUpgradeThresholdCountsDevicePixels covers the "is the preview
// being enlarged" test: at 1.5x the preview reaches 125% of its own pixels
// sooner, which is right — it is already being stretched on the glass.
func TestPreviewUpgradeThresholdCountsDevicePixels(t *testing.T) {
	const (
		bw, bh = 2560, 1920
		availW = 1200
		availH = 800
	)
	fitted := viewerBitmapPercentValue(bw, bh, 0, 1, availW, availH, 1)
	scaled := viewerBitmapPercentValue(bw, bh, 0, 1, availW, availH, 1.5)
	if scaled <= fitted {
		t.Fatalf("scaled display reads %d%%, unscaled %d%%", scaled, fitted)
	}
	if want := int(math.Round(float64(fitted) * 1.5)); scaled != want {
		t.Fatalf("scaled readout = %d, want %d", scaled, want)
	}
}
