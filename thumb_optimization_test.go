package main

import (
	"image"
	"image/color"
	"testing"
)

func absByte(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}

func TestAreaScaleAveragesQuadrants(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	colors := []color.RGBA{
		{0xff, 0x00, 0x00, 0xff},
		{0x00, 0xff, 0x00, 0xff},
		{0x00, 0x00, 0xff, 0xff},
		{0xff, 0xff, 0xff, 0xff},
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			c := colors[(y/2)*2+x/2]
			src.SetRGBA(x, y, c)
		}
	}
	dst := areaScale(src, 2, 2)
	if got := dst.Bounds().Size(); got.X != 2 || got.Y != 2 {
		t.Fatalf("size = %v, want 2x2", got)
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			want := colors[y*2+x]
			got := dst.RGBAAt(x, y)
			if absByte(int(got.R), int(want.R)) > 1 ||
				absByte(int(got.G), int(want.G)) > 1 ||
				absByte(int(got.B), int(want.B)) > 1 ||
				got.A != 0xff {
				t.Errorf("pixel (%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}
}

func TestAreaScaleCompositesTransparencyOnSurface(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for i := 0; i < len(src.Pix); i += 4 {
		src.Pix[i+0] = 0xff
		src.Pix[i+3] = 0x80
	}
	dst := areaScale(src, 1, 1)
	got := dst.RGBAAt(0, 0)
	// 50% red over #2a2a2e: red = 127 + 42*127/255 ~= 148.
	if got.A != 0xff || got.R < 145 || got.R > 151 || got.G < 19 || got.G > 23 || got.B < 21 || got.B > 25 {
		t.Fatalf("transparent composite = %v, want roughly (148,21,23,255)", got)
	}
}

func TestThumbnailImageAppliesOrientationAtThumbnailSize(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 120, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 120; x++ {
			if x < 60 {
				src.SetRGBA(x, y, color.RGBA{0xff, 0, 0, 0xff})
			} else {
				src.SetRGBA(x, y, color.RGBA{0, 0, 0xff, 0xff})
			}
		}
	}
	// EXIF orientation 6 turns the 120x80 landscape into an 80x120 portrait.
	// Rotating clockwise puts the red left edge on top.
	dst := thumbnailImage(src, dimInfo{W: 80, H: 120, Orient: 6}, 24)
	if got := dst.Bounds().Size(); got.X != 16 || got.Y != 24 {
		t.Fatalf("size = %v, want 16x24", got)
	}
	rgb, ok := dst.(*image.RGBA)
	if !ok {
		t.Fatalf("thumbnail = %T, want *image.RGBA", dst)
	}
	if !rgb.Opaque() {
		t.Fatal("thumbnail surface should be opaque")
	}
	top := color.RGBAModel.Convert(dst.At(8, 0)).(color.RGBA)
	bottom := color.RGBAModel.Convert(dst.At(8, 23)).(color.RGBA)
	if top.R < 240 || top.B > 15 {
		t.Fatalf("top pixel = %v, want red", top)
	}
	if bottom.B < 240 || bottom.R > 15 {
		t.Fatalf("bottom pixel = %v, want blue", bottom)
	}
}

func TestThumbnailUsesAreaPrefilterForLargeReduction(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 120, 80))
	if !useAreaPrefilter(src, 8, 5) {
		t.Fatal("120x80 -> 8x5 should use the area prefilter")
	}
	if useAreaPrefilter(src, 60, 40) {
		t.Fatal("2x reduction should keep the direct Catmull-Rom path")
	}
}

func TestThumbQueueRunsClosestFirstAndRefreshesPriority(t *testing.T) {
	q := newThumbQueue()
	q.submit(&thumbTask{key: "far", priority: 100, frame: 1})
	q.submit(&thumbTask{key: "near", priority: 10, frame: 1})
	q.submit(&thumbTask{key: "middle", priority: 50, frame: 1})
	if got := q.popNow(); got == nil || got.key != "near" {
		t.Fatalf("first = %#v, want near", got)
	}
	q.submit(&thumbTask{key: "far", priority: 5, frame: 2})
	if got := q.popNow(); got == nil || got.key != "far" {
		t.Fatalf("second = %#v, want refreshed far", got)
	}
	if got := q.popNow(); got == nil || got.key != "middle" {
		t.Fatalf("third = %#v, want middle", got)
	}
}

func TestThumbQueueDropsStalePendingTasks(t *testing.T) {
	q := newThumbQueue()
	q.submit(&thumbTask{key: "old", priority: 10, frame: 1})
	q.submit(&thumbTask{key: "new", priority: 20, frame: 2})
	if dropped := q.retain(2); len(dropped) != 1 || dropped[0] != "old" {
		t.Fatalf("dropped = %v, want [old]", dropped)
	}
	if got := q.popNow(); got == nil || got.key != "new" {
		t.Fatalf("remaining = %#v, want new", got)
	}
}
