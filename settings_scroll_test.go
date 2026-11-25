package main

import (
	"bytes"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestSettingsScrollsToViewerHUD(t *testing.T) {
	a := newViewTestApp()
	a.openSettings()
	a.settingsUI.Category = "control"
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
	if rect, ok := tt.Find("查看器状态提示"); !ok || rect.Y > 620 {
		t.Fatalf("viewer HUD setting not visible after scrolling: %#v ok=%v", rect, ok)
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
