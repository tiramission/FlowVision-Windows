package main

import (
	"image"
	"image/color"
	"math"

	"golang.org/x/image/draw"
)

// Thumbnail cards use the same neutral surface in both themes, so images
// with transparency read as a picture rather than a hole in the card.
const (
	thumbSurfaceR = 0x2a
	thumbSurfaceG = 0x2a
	thumbSurfaceB = 0x2e
)

// thumbCanvas returns the neutral destination a thumbnail is composited into.
func thumbCanvas(w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(dst.Pix); i += 4 {
		dst.Pix[i+0] = thumbSurfaceR
		dst.Pix[i+1] = thumbSurfaceG
		dst.Pix[i+2] = thumbSurfaceB
		dst.Pix[i+3] = 0xff
	}
	return dst
}

// thumbTargetSize returns the displayed thumbnail size. d.W and d.H already
// describe the upright image, including an EXIF swap.
func thumbTargetSize(src image.Image, d dimInfo, size int) (w, h int, swap bool) {
	dw, dh := d.W, d.H
	if dw <= 0 || dh <= 0 {
		dw, dh = src.Bounds().Dx(), src.Bounds().Dy()
		if d.Orient >= 5 && d.Orient <= 8 {
			dw, dh = dh, dw
		}
	}
	if dw <= 0 || dh <= 0 {
		return 0, 0, false
	}
	h = size
	w = int(math.Round(float64(size) * clampF(float64(dw)/float64(dh), 0.25, 4)))
	if w < 8 {
		w = 8
	}
	swap = d.Orient >= 5 && d.Orient <= 8
	return w, h, swap
}

// thumbnailImage scales src to the card-sized image. Large reductions use an
// area-average pass to 2x the target followed by Catmull-Rom, which preserves
// detail without Catmull-Rom sampling a huge kernel over the full-resolution
// source. Small reductions keep the direct Catmull-Rom path.
func thumbnailImage(src image.Image, d dimInfo, size int) image.Image {
	w, h, swap := thumbTargetSize(src, d, size)
	if w <= 0 || h <= 0 {
		return thumbCanvas(8, size)
	}
	preW, preH := w, h
	if swap {
		preW, preH = h, w
	}
	if useAreaPrefilter(src, preW, preH) {
		// The intermediate is already at 2x the final size, so the final
		// Catmull-Rom pass only has a 2x kernel footprint.
		var out image.Image = areaScale(src, preW*2, preH*2)
		if d.Orient > 1 && d.Orient < 9 {
			// Rotating a ~2x thumbnail costs milliseconds; rotating the
			// original can allocate hundreds of megabytes.
			out = orientImage(out, d.Orient)
		}
		dst := thumbCanvas(w, h)
		draw.CatmullRom.Scale(dst, dst.Bounds(), out, out.Bounds(), draw.Over, nil)
		return dst
	}
	out := src
	if d.Orient > 1 && d.Orient < 9 {
		out = orientImage(out, d.Orient)
	}
	dst := thumbCanvas(w, h)
	draw.CatmullRom.Scale(dst, dst.Bounds(), out, out.Bounds(), draw.Over, nil)
	return dst
}

// useAreaPrefilter reports whether the source is reduced enough that an
// area-average first pass is materially cheaper than Catmull-Rom's scaled
// kernel over every source pixel.
func useAreaPrefilter(src image.Image, w, h int) bool {
	if w <= 0 || h <= 0 {
		return false
	}
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	return sw >= w*3 || sh >= h*3
}

// areaScale averages each destination pixel over the source rectangle it
// covers, then composites it onto the thumbnail surface. Common decoded image
// types have direct-pixel fast paths; unusual formats use image.Image.At.
func areaScale(src image.Image, w, h int) *image.RGBA {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	if sw <= 0 || sh <= 0 {
		return thumbCanvas(w, h)
	}
	sample := areaSampler(src)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for dy := 0; dy < h; dy++ {
		y0 := dy * sh / h
		y1 := (dy + 1) * sh / h
		if y1 <= y0 {
			y1 = y0 + 1
		}
		y0 += src.Bounds().Min.Y
		y1 += src.Bounds().Min.Y
		for dx := 0; dx < w; dx++ {
			x0 := dx * sw / w
			x1 := (dx + 1) * sw / w
			if x1 <= x0 {
				x1 = x0 + 1
			}
			x0 += src.Bounds().Min.X
			x1 += src.Bounds().Min.X
			r, g, b, a := sample(x0, y0, x1, y1)
			i := dy*dst.Stride + dx*4
			dst.Pix[i+0] = compositeOnSurface(r, a, thumbSurfaceR)
			dst.Pix[i+1] = compositeOnSurface(g, a, thumbSurfaceG)
			dst.Pix[i+2] = compositeOnSurface(b, a, thumbSurfaceB)
			dst.Pix[i+3] = 0xff
		}
	}
	return dst
}

func compositeOnSurface(premultiplied, alpha, surface uint8) uint8 {
	return uint8(uint32(premultiplied) + uint32(surface)*uint32(255-alpha)/255)
}

// areaSampler returns a function averaging the absolute source coordinates
// [x0,x1) x [y0,y1). The returned channels are premultiplied, with alpha in
// 0..255, ready for compositeOnSurface.
func areaSampler(src image.Image) func(x0, y0, x1, y1 int) (r, g, b, a uint8) {
	bounds := src.Bounds()
	minX, minY := bounds.Min.X, bounds.Min.Y
	switch im := src.(type) {
	case *image.YCbCr:
		return func(x0, y0, x1, y1 int) (uint8, uint8, uint8, uint8) {
			var sy, scb, scr, n uint64
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					sy += uint64(im.Y[im.YOffset(x, y)])
					ci := im.COffset(x, y)
					scb += uint64(im.Cb[ci])
					scr += uint64(im.Cr[ci])
					n++
				}
			}
			r, g, b := color.YCbCrToRGB(uint8(sy/n), uint8(scb/n), uint8(scr/n))
			return r, g, b, 255
		}
	case *image.RGBA:
		return func(x0, y0, x1, y1 int) (uint8, uint8, uint8, uint8) {
			var sr, sg, sb, sa, n uint64
			for y := y0; y < y1; y++ {
				row := (y-minY)*im.Stride + (x0-minX)*4
				for x := x0; x < x1; x++ {
					sr += uint64(im.Pix[row+0])
					sg += uint64(im.Pix[row+1])
					sb += uint64(im.Pix[row+2])
					sa += uint64(im.Pix[row+3])
					row += 4
					n++
				}
			}
			return uint8(sr / n), uint8(sg / n), uint8(sb / n), uint8(sa / n)
		}
	case *image.NRGBA:
		return func(x0, y0, x1, y1 int) (uint8, uint8, uint8, uint8) {
			var sr, sg, sb, sa, n uint64
			for y := y0; y < y1; y++ {
				row := (y-minY)*im.Stride + (x0-minX)*4
				for x := x0; x < x1; x++ {
					alpha := uint64(im.Pix[row+3])
					sr += uint64(im.Pix[row+0]) * alpha / 255
					sg += uint64(im.Pix[row+1]) * alpha / 255
					sb += uint64(im.Pix[row+2]) * alpha / 255
					sa += alpha
					row += 4
					n++
				}
			}
			return uint8(sr / n), uint8(sg / n), uint8(sb / n), uint8(sa / n)
		}
	case *image.Gray:
		return func(x0, y0, x1, y1 int) (uint8, uint8, uint8, uint8) {
			var sy, n uint64
			for y := y0; y < y1; y++ {
				row := (y-minY)*im.Stride + (x0 - minX)
				for x := x0; x < x1; x++ {
					sy += uint64(im.Pix[row])
					row++
					n++
				}
			}
			v := uint8(sy / n)
			return v, v, v, 255
		}
	}
	return func(x0, y0, x1, y1 int) (uint8, uint8, uint8, uint8) {
		var sr, sg, sb, sa, n uint64
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				c := color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA)
				alpha := uint64(c.A)
				sr += uint64(c.R) * alpha / 255
				sg += uint64(c.G) * alpha / 255
				sb += uint64(c.B) * alpha / 255
				sa += alpha
				n++
			}
		}
		return uint8(sr / n), uint8(sg / n), uint8(sb / n), uint8(sa / n)
	}
}

// scaleImageFast draws src into dst with an approximate bilinear filter. It is
// used only for the viewer's 2560 first-frame preview; the grid keeps the
// higher-quality Catmull-Rom path.
func scaleImageFast(dst draw.Image, src image.Image) {
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
}
