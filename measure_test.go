package main

import (
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestMeasureInkRise prints, for each interface text size, where the middle of
// a label's ink lands relative to its baseline. The numbers fill inkRiseAt:
// deriving them from the font's metrics does not work, because where the ink
// falls inside the em band shifts with the pixel grid as the size changes.
func TestMeasureInkRise(t *testing.T) {
	registerShippedFont(t)
	for _, family := range []string{"", "MeasureMiSans"} {
		for _, size := range []float32{9, 10, 10.5, 11, 12, 13} {
			var sum float32
			var n int
			for _, s := range []string{"正在载入原图", "327%", "视频", "Screenshots", "收藏夹"} {
				inkCentre := measureInkDrop(t, family, s, size)
				line, ok := firstLineOf(s, size, 500)
				if !ok {
					t.Fatalf("%q: no layout", s)
				}
				sum += (line.Baseline - inkCentre) / size
				n++
			}
			t.Logf("family=%-14q size=%5.1f rise=%.4f", family, size, sum/float32(n))
		}
	}
}

// TestInkSitsOnThePillCentre renders a label the way the pills do and checks
// that its ink, not its line box, lands on the pill's middle.
func TestInkSitsOnThePillCentre(t *testing.T) {
	registerShippedFont(t)
	for _, family := range []string{"", "MeasureMiSans"} {
		for _, s := range []string{"正在载入原图", "327%", "视频", "Screenshots", "收藏夹"} {
			for _, size := range []float32{9, 10, 10.5, 11, 12, 13} {
				const pillH = float32(28)
				inkDrop := measureInkDrop(t, family, s, size)
				off := textCentreOffset(pillH, s, size, 500)
				if delta := off + inkDrop - pillH/2; delta < -1.5 || delta > 1.5 {
					t.Fatalf("family=%q text=%q size=%.1f: ink at %.1f, want %.1f, off by %+.1f",
						family, s, size, off+inkDrop, pillH/2, delta)
				}
			}
		}
	}
}

// measureInkDrop paints s at a known y and returns how far below that y the
// middle of its ink falls.
func measureInkDrop(t *testing.T, family, s string, size float32) float32 {
	t.Helper()
	const at = float32(40)
	img := renderLabel(family, s, size, at)
	top, bot, ok := inkBand(img)
	if !ok {
		t.Fatalf("family=%q %q size=%.1f: no ink", family, s, size)
	}
	return (top+bot)/2 - at
}

// registerShippedFont registers the built MiSans as MeasureMiSans, so the tests
// measure the font the app ships rather than only the platform's.
func registerShippedFont(t *testing.T) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("build", "windows-amd64", "fonts", "MiSans VF.ttf"))
	if err != nil {
		t.Skipf("shipped font not built, skipping: %v", err)
	}
	if err := ui.RegisterFont(data, "MeasureMiSans"); err != nil {
		t.Fatalf("register: %v", err)
	}
}

// renderLabel paints s at y=at on a white surface, in black ink: the window
// always paints a background, so ink is found by darkness.
func renderLabel(family, s string, size, at float32) *image.RGBA {
	return ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Background(ui.RGBA(255, 255, 255, 255)).
			Draw(func(p *ui.Painter, r ui.Rect) {
				sp := ui.Span{Text: s, Size: size, Weight: 500, Color: ui.RGBA(0, 0, 0, 255)}
				if family != "" {
					sp.Font = family
				}
				p.RichText(10, at, 0, sp)
			})
	}, 400, 200).Image()
}

// inkBand returns the vertical extent of the dark pixels.
func inkBand(img *image.RGBA) (top, bot float32, ok bool) {
	top, bot = -1, -1
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r>>8 < 160 && g>>8 < 160 && bl>>8 < 160 {
				if top < 0 {
					top = float32(y)
				}
				bot = float32(y)
				break
			}
		}
	}
	return top, bot, top >= 0
}
