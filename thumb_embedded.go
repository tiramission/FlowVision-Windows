package main

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"
)

// embeddedThumb decodes the IFD1 thumbnail of a JPEG. minRatio filters out
// previews too small to be useful as a final card image; pass 0 to accept any
// thumbnail as a preview.
func embeddedThumb(it *Item, size int, minRatio float64) ([]byte, error) {
	ex := readExif(it.Path)
	if len(ex.Thumb) == 0 {
		return nil, fmt.Errorf("no embedded thumbnail")
	}
	src, _, err := image.Decode(bytes.NewReader(ex.Thumb))
	if err != nil {
		return nil, err
	}
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	if sw <= 0 || sh <= 0 {
		return nil, fmt.Errorf("empty embedded thumbnail")
	}
	d := imageDims(it.Path, it.Mtime)
	if d.W <= 0 || d.H <= 0 {
		d = dimInfo{W: sw, H: sh, Orient: ex.Orientation}
		if d.Orient >= 5 && d.Orient <= 8 {
			d.W, d.H = sh, sw
		}
	} else if d.Orient == 0 {
		d.Orient = ex.Orientation
	}
	if minRatio > 0 {
		targetW, targetH, _ := thumbTargetSize(src, d, size)
		if targetW <= 0 || targetH <= 0 {
			return nil, fmt.Errorf("bad thumbnail target")
		}
		if float64(sw) < float64(targetW)*minRatio || float64(sh) < float64(targetH)*minRatio {
			return nil, fmt.Errorf("embedded thumbnail too small")
		}
	}
	dst := thumbnailImage(src, d, size)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 84}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// embeddedThumbPreview is the low-cost preview path. It deliberately accepts
// a thumbnail even when it is smaller than the final card, because a slightly
// soft preview beats a blank card while WIC decodes the full frame.
func embeddedThumbPreview(it *Item, size int) ([]byte, error) {
	if !isJPEG(it.Path) {
		return nil, fmt.Errorf("not a JPEG")
	}
	return embeddedThumb(it, size, 0)
}

// readEmbeddedJPEG is a tiny helper used by tests and diagnostics; it reads an
// embedded thumbnail without decoding it.
func readEmbeddedJPEG(path string) ([]byte, error) {
	ex := readExif(path)
	if len(ex.Thumb) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	return ex.Thumb, nil
}
