package main

import (
	"image"
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
	a.viewer.Item = &Item{ID: "1", Name: "a.jpg", Kind: "image", Path: `C:\Pictures\a.jpg`}

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
	a.viewer.Item = &Item{ID: "2", Name: "b.mp4", Kind: "video", Path: `C:\Pictures\b.mp4`}
	if got, _ := a.viewerQualityLabel(); got != "" {
		t.Fatalf("video reported %q", got)
	}
}
