package main

import (
	"image"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestViewerQualityLabelNamesWhatIsShown covers the footer's quality badge: the
// viewer had no way to say whether a soft picture was the grid thumbnail, the
// 2560 preview or the original, so a slow decode and a small source looked the
// same.
func TestViewerQualityLabelNamesWhatIsShown(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewer.Item = &Item{ID: "1", Name: "a.jpg", Kind: "image", Path: `C:\Pictures\a.jpg`, W: 6000, H: 4000}

	// Only the thumbnail is available while the preview decodes.
	a.viewer.Bitmap = nil
	a.viewer.FullResReady = false
	if got, _ := a.viewerQualityLabel(); got != "缩略图" {
		t.Fatalf("thumbnail state = %q", got)
	}

	// The preview, not yet the original.
	a.viewer.Bitmap = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 4, 4)))
	a.viewer.FullResReady = false
	if got, _ := a.viewerQualityLabel(); got != "大图" {
		t.Fatalf("preview state = %q", got)
	}

	// The original.
	a.viewer.FullResReady = true
	if got, _ := a.viewerQualityLabel(); got != "原图" {
		t.Fatalf("full-resolution state = %q", got)
	}

	// A video has no such states, so it shows no badge.
	a.viewer.Item = &Item{ID: "2", Name: "b.mp4", Kind: "video", Path: `C:\Pictures\b.mp4`, W: 1920, H: 1080}
	if got, _ := a.viewerQualityLabel(); got != "" {
		t.Fatalf("video reported %q", got)
	}
}

// TestSmallImageReadsAsTheOriginal covers a picture whose longest side is
// already within the preview limit: its preview is the whole picture, the
// full-resolution decode is skipped on purpose, and FullResReady never turns
// true — so testing that flag alone kept every small picture saying 大图.
func TestSmallImageReadsAsTheOriginal(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewer.Open = true
	a.viewer.FullResReady = false

	// 2000×1500: below the 2560 cap, so the preview is the original.
	a.viewer.Item = &Item{ID: "small", Name: "small.jpg", Kind: "image", Path: `C:\Pictures\small.jpg`, W: 2000, H: 1500}
	a.viewer.Bitmap = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 2000, 1500)))
	if got, _ := a.viewerQualityLabel(); got != "原图" {
		t.Fatalf("a 2000x1500 picture at 2000x1500 reads as %q", got)
	}
	if a.viewerStatusItems(nil)[0].Strong {
		t.Fatal("a settled original is shown as a pending state")
	}

	// 4000×3000: the preview is capped at 2560, so the original is still to come.
	a.viewer.Item = &Item{ID: "big", Name: "big.jpg", Kind: "image", Path: `C:\Pictures\big.jpg`, W: 4000, H: 3000}
	a.viewer.Bitmap = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 2560, 1920)))
	if got, _ := a.viewerQualityLabel(); got != "大图" {
		t.Fatalf("a capped preview reads as %q", got)
	}

	// And once the original arrives it says so.
	a.viewer.FullResReady = true
	if got, _ := a.viewerQualityLabel(); got != "原图" {
		t.Fatalf("a loaded original reads as %q", got)
	}
}

// TestQualityReadoutCarriesThePendingAndFailedStates covers what the floating
// notice used to announce: waiting for the original after pressing 1, and a
// failure to decode it. Both are states of the picture, so both belong in the
// readout that is always there.
func TestQualityReadoutCarriesThePendingAndFailedStates(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.viewer.Open = true
	a.viewer.Item = &Item{ID: "big", Name: "big.jpg", Kind: "image", Path: `C:\Pictures\big.jpg`, W: 6000, H: 4000}
	a.viewer.Bitmap = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 2560, 1707)))

	// Pressing 1 with no original yet: the readout says what will happen.
	a.viewerPendingActual = true
	a.viewer.FullResLoading = true
	if got, tip := a.viewerQualityLabel(); got != "大图 · 待切 100%" {
		t.Fatalf("pending state = %q", got)
	} else if tip == "" {
		t.Fatal("the pending state has no tooltip")
	}

	// Without the pending switch it is just a decode in progress.
	a.viewerPendingActual = false
	if got, _ := a.viewerQualityLabel(); got != "大图 · 载入中" {
		t.Fatalf("loading state = %q", got)
	}

	// A failed original is a failure, not a stage, and it says why.
	a.viewer.FullResLoading = false
	a.viewer.FullResUnsupported = true
	a.viewer.FullResErr = "磁盘错误"
	got, tip := a.viewerQualityLabel()
	if got != "原图失败" {
		t.Fatalf("failed state = %q", got)
	}
	if !strings.Contains(tip, "磁盘错误") {
		t.Fatalf("failure tooltip lost the reason: %q", tip)
	}
	if stats := a.viewerStatusItems(nil); len(stats) == 0 || !stats[0].Danger {
		t.Fatal("the failure is not marked as one, so it cannot be coloured")
	}
}
