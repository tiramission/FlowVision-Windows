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
			nz, nx, ny := viewerZoomAtPoint(tc.zoom, tc.panX, tc.panY, tc.delta, tc.x, tc.y, tc.w, tc.h)
			wantZoom := clampViewerZoom(tc.zoom * tc.delta)
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
	nz, nx, ny := viewerZoomAtPoint(1.2, 0, 0, 1.25, 500, 400, 1000, 800)
	if nz != 1.5 || nx != 0 || ny != 0 {
		t.Fatalf("center zoom = %v pan %v,%v; want 1.5 and 0,0", nz, nx, ny)
	}
}

func TestViewerZoomAtPointClampsWithoutBreakingAnchor(t *testing.T) {
	nz, nx, ny := viewerZoomAtPoint(10, 10, 5, 1.5, 300, 200, 1000, 800)
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
	if nz, nx, ny := viewerZoomAtPoint(1, 4, 5, 2, 100, 100, 0, 800); nz != 1 || nx != 4 || ny != 5 {
		t.Fatalf("invalid width changed state: %v %v %v", nz, nx, ny)
	}
	if nz, nx, ny := viewerZoomAtPoint(1, 4, 5, 0, 100, 100, 1000, 800); nz != 1 || nx != 4 || ny != 5 {
		t.Fatalf("zero delta changed state: %v %v %v", nz, nx, ny)
	}
}
