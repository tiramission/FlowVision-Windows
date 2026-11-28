package main

import (
	"bytes"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestSettingsScrollsToTheLastViewerGroup(t *testing.T) {
	a := newViewTestApp()
	a.openSettings()
	a.settingsUI.Category = "viewer"
	tt := ui.NewTester(a.view, 1320, 745)
	tt.Frame()
	before := append([]byte(nil), tt.Image().Pix...)

	tt.Wheel(800, 480, 0, 600)
	tt.Advance(1200 * time.Millisecond)
	tt.Frame()
	after := tt.Image()
	if bytes.Equal(before, after.Pix) {
		t.Fatal("settings page did not scroll")
	}
	// The point is that scrolling brought the setting into the window, not
	// where the row lands: the categories above it change the offset.
	const winH = 745
	if rect, ok := tt.Find("查看器侧键"); !ok || rect.Y < 0 || rect.Y > winH-40 {
		t.Fatalf("last viewer setting not visible after scrolling: %#v ok=%v", rect, ok)
	}
	if out := os.Getenv("FLOWVISION_CAPTURE"); out != "" {
		f, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, after); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		_ = f.Close()
	}
}

func TestSettingsShowsViewerPreloadScope(t *testing.T) {
	a := newViewTestApp()
	a.openSettings()
	a.settingsUI.Category = "viewer"
	tt := ui.NewTester(a.view, 1320, 900)
	tt.Frame()

	// The viewer group sits below the fold; scroll until the preload rows show.
	for i := 0; i < 8; i++ {
		if tt.HasText("自动载入当前图原图") {
			break
		}
		tt.Wheel(800, 480, 0, 400)
		tt.Advance(600 * time.Millisecond)
		tt.Frame()
	}
	if !tt.HasText("预加载相邻图片") {
		t.Fatalf("neighbour preload row missing: %q", tt.Texts())
	}
	if !tt.HasText("自动载入当前图原图") {
		t.Fatalf("automatic original row missing: %q", tt.Texts())
	}
	if !tt.HasText("2560 预览 · 默认 2 张") {
		t.Fatalf("preload row does not label the preview size: %q", tt.Texts())
	}
}
