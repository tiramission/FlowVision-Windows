package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestEllipsizeTextFitsTheWidth covers the label that ran past its pill: a long
// name must be shortened until it fits, and a name that already fits must be
// left alone.
func TestEllipsizeTextFitsTheWidth(t *testing.T) {
	const size, weight = float32(11), 500
	long := "2024_12_07_20_56_48_3028461.jpeg"

	var measured map[string]float32
	var results map[float32]string
	var empty string
	ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
			if measured != nil {
				return
			}
			measured = map[string]float32{}
			results = map[float32]string{}
			m := func(s string) float32 {
				if w, ok := measured[s]; ok {
					return w
				}
				w, _ := p.MeasureText(0, ui.Span{Text: s, Size: size, Weight: weight})
				measured[s] = w
				return w
			}
			full := m(long)
			if full <= 100 {
				return
			}
			for _, width := range []float32{40, 80, 100, full - 1} {
				results[width] = ellipsizeText(p, long, size, weight, width)
			}
			results[-1] = ellipsizeText(p, long, size, weight, full+10)
			empty = ellipsizeText(p, long, size, weight, 0)
		})
	}, 200, 40).Frame()

	if measured == nil || results == nil {
		t.Fatal("no painter to measure with")
	}
	full := measured[long]
	if full <= 100 {
		t.Skipf("label too narrow to exercise the limit: %.1f", full)
	}

	for _, width := range []float32{40, 80, 100, full - 1} {
		got := results[width]
		if w := measured[got]; w > width+0.5 {
			t.Fatalf("width %.1f: %q measures %.1f, over the limit", width, got, w)
		}
		if got == long {
			t.Fatalf("width %.1f: the label was left whole", width)
		}
	}
	// A label that fits keeps its whole name, without an ellipsis.
	if got := results[-1]; got != long {
		t.Fatalf("label that fits was changed: %q", got)
	}
	if empty != "" {
		t.Fatalf("no room should give an empty label, got %q", empty)
	}
}

// TestHoverPillFitsTheGallery covers the hovered tile's name pill: it may grow
// past the tile so the whole name shows, but it must stay inside the gallery,
// and only a name too long even for the gallery is shortened.
func TestHoverPillFitsTheGallery(t *testing.T) {
	const size, weight, padX = float32(11), 500, float32(10)
	gallery := ui.Rect{X: 0, Y: 0, W: 900, H: 600}
	// A tile in the middle, narrow enough that the name cannot fit inside it.
	tile := ui.Rect{X: 400, Y: 100, W: 120, H: 120}
	long := "2024_12_07_20_56_48_3028461.jpeg"

	var pillW, pillX float32
	var label string
	ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
			maxW := max(0, gallery.W-16)
			textW, _ := p.MeasureText(0, ui.Span{Text: long, Size: size, Weight: weight})
			label = long
			if textW+padX*2 > maxW {
				label = ellipsizeText(p, long, size, weight, maxW-padX*2)
				textW, _ = p.MeasureText(0, ui.Span{Text: label, Size: size, Weight: weight})
			}
			pillW = min(textW+padX*2, maxW)
			pillX = tile.X + (tile.W-pillW)/2
			if pillX < gallery.X+8 {
				pillX = gallery.X + 8
			}
			if right := gallery.X + gallery.W - 8; pillX+pillW > right {
				pillX = right - pillW
			}
		})
	}, 900, 600).Frame()

	// The name is shown whole: the pill grew past the 120-wide tile.
	if label != long {
		t.Fatalf("name was shortened although the gallery has room: %q", label)
	}
	if pillW <= tile.W {
		t.Fatalf("pill %.1f did not grow past the tile %.1f", pillW, tile.W)
	}
	// And it stays inside the gallery.
	if pillX < gallery.X+8 || pillX+pillW > gallery.X+gallery.W-8 {
		t.Fatalf("pill spans %.1f..%.1f, outside the gallery", pillX, pillX+pillW)
	}
}
