package main

import (
	"math"
	"testing"
)

func TestViewerZoomAtPointKeepsPointerAnchor(t *testing.T) {
	cases := []struct {
		name              string
		zoom, panX, panY  float32
		delta, x, y, w, h float32
	}{
		{"top left", 1, 0, 0, 2, 120, 80, 1000, 800},
		{"off center", 1.4, 35, -18, 1.25, 760, 210, 1200, 900},
		{"zoom out", 2.5, -60, 40, 0.8, 340, 620, 1000, 800},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nz, nx, ny := viewerZoomAtPoint(tc.zoom, tc.panX, tc.panY, tc.delta, tc.x, tc.y, tc.w, tc.h, 0)
			wantZoom := clampViewerZoom(tc.zoom*tc.delta, 0)
			if nz != wantZoom {
				t.Fatalf("zoom = %v, want %v", nz, wantZoom)
			}
			ax, ay := tc.x-tc.w/2, tc.y-tc.h/2
			ratio := nz / tc.zoom
			if got, want := ax-nx, (ax-tc.panX)*ratio; math.Abs(float64(got-want)) > 0.001 {
				t.Fatalf("x anchor moved: got %v, want %v", got, want)
			}
			if got, want := ay-ny, (ay-tc.panY)*ratio; math.Abs(float64(got-want)) > 0.001 {
				t.Fatalf("y anchor moved: got %v, want %v", got, want)
			}
		})
	}
}

func TestViewerZoomAtPointCenterWithoutPan(t *testing.T) {
	nz, nx, ny := viewerZoomAtPoint(1.2, 0, 0, 1.25, 500, 400, 1000, 800, 0)
	if nz != 1.5 || nx != 0 || ny != 0 {
		t.Fatalf("center zoom = %v pan %v,%v; want 1.5 and 0,0", nz, nx, ny)
	}
}

func TestViewerZoomAtPointClampsWithoutBreakingAnchor(t *testing.T) {
	nz, nx, ny := viewerZoomAtPoint(10, 10, 5, 1.5, 300, 200, 1000, 800, 0)
	if nz != 12 {
		t.Fatalf("clamped zoom = %v, want 12", nz)
	}
	ax, ay := float32(300-500), float32(200-400)
	if got, want := ax-nx, (ax-10)*(12.0/10.0); math.Abs(float64(got-want)) > 0.001 {
		t.Fatalf("clamped x anchor got %v want %v", got, want)
	}
	if got, want := ay-ny, (ay-5)*(12.0/10.0); math.Abs(float64(got-want)) > 0.001 {
		t.Fatalf("clamped y anchor got %v want %v", got, want)
	}
}

func TestViewerZoomAtPointRejectsInvalidGeometry(t *testing.T) {
	if nz, nx, ny := viewerZoomAtPoint(1, 4, 5, 2, 100, 100, 0, 800, 0); nz != 1 || nx != 4 || ny != 5 {
		t.Fatalf("invalid width changed state: %v %v %v", nz, nx, ny)
	}
	if nz, nx, ny := viewerZoomAtPoint(1, 4, 5, 0, 100, 100, 1000, 800, 0); nz != 1 || nx != 4 || ny != 5 {
		t.Fatalf("zero delta changed state: %v %v %v", nz, nx, ny)
	}
}

func TestHugeImageCanReachActualSize(t *testing.T) {
	// A 20000×15000 image in a 1200×800 canvas fits at 0.06, so 1:1 needs about
	// 16.7× fit — above the ordinary relative ceiling of 12. It used to clamp
	// there and stop at 72% with no way to reach 100%.
	fit := viewerFitScale(20000, 15000, 0, 1200, 800)
	if fit <= 0 {
		t.Fatal("fit scale is zero")
	}
	actual := viewerActualZoom(fit)
	if actual <= viewerZoomMaxRelative {
		t.Fatalf("test image is not large enough: actual zoom %v", actual)
	}
	if got := clampViewerZoom(actual, fit); got != actual {
		t.Fatalf("actual size clamped to %v, want %v", got, actual)
	}
	if pct := viewerPercentValue(20000, 15000, 20000, 15000, 0, actual, 1200, 800); pct != 100 {
		t.Fatalf("1:1 shows %d%%, want 100%%", pct)
	}
	// Large images keep a little headroom past 1:1 instead of stopping dead.
	if got, want := clampViewerZoom(actual*10, fit), actual*1.5; math.Abs(float64(got-want)) > 0.01 {
		t.Fatalf("huge-image ceiling = %v, want %v", got, want)
	}

	// Ordinary images keep the old limit, so this did not change their feel.
	ordinaryFit := viewerFitScale(3000, 2000, 0, 1200, 800)
	if got := clampViewerZoom(100, ordinaryFit); got != viewerZoomMaxRelative {
		t.Fatalf("ordinary ceiling = %v, want %v", got, viewerZoomMaxRelative)
	}

	// Rotated and portrait images use the same rule.
	for _, tc := range []struct {
		bw, bh, angle float32
	}{{15000, 20000, 0}, {20000, 15000, 90}, {20000, 15000, 30}} {
		f := viewerFitScale(int(tc.bw), int(tc.bh), tc.angle, 1200, 800)
		a := viewerActualZoom(f)
		if got := clampViewerZoom(a, f); got != a {
			t.Fatalf("%.0fx%.0f at %.0f°: actual clamped to %v, want %v", tc.bw, tc.bh, tc.angle, got, a)
		}
	}
}
