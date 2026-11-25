package main

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	viewerPreviewMaxDim     = 2560
	viewerPreviewQuality    = 90
	viewerPreviewCacheVer   = "v3"
	viewerPreviewFastMode   = "fast"
	viewerPreviewHQMode     = "hq"
	viewerPreviewLegacyMode = "legacy"
)

type viewerLoadStats struct {
	LoadMs         int64 `json:"loadMs"`
	DecodeMs       int64 `json:"decodeMs"`
	ScaleMs        int64 `json:"scaleMs"`
	EncodeMs       int64 `json:"encodeMs"`
	BitmapMs       int64 `json:"bitmapMs"`
	FullResMs      int64 `json:"fullResMs"`
	FullResBytes   int64 `json:"fullResBytes"`
	CacheHit       bool  `json:"cacheHit"`
	PlaceholderHit bool  `json:"placeholderHit"`
	PreloadHit     bool  `json:"preloadHit"`
}

type viewerPreviewResult struct {
	Img      image.Image
	Bmp      *ui.Bitmap
	Bytes    int64
	CacheHit bool
	DecodeMs int64
	ScaleMs  int64
	EncodeMs int64
	BitmapMs int64
}

// viewerPreviewMode selects the preview scaler. The old JPEG->DecodeBitmap
// path stays available for one release as an explicit diagnostic fallback.
func viewerPreviewMode() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FLOWVISION_VIEWER_PREVIEW"))) {
	case "legacy":
		return viewerPreviewLegacyMode
	case "hq":
		return viewerPreviewHQMode
	case "fast":
		return viewerPreviewFastMode
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FLOWVISION_VIEWER_SCALER"))) {
	case "hq":
		return viewerPreviewHQMode
	case "fast":
		return viewerPreviewFastMode
	}
	return viewerPreviewFastMode
}

func viewerPreviewWithinLimit(w, h int) bool {
	if w <= 0 || h <= 0 {
		return true
	}
	return int64(w)*int64(h) <= 80_000_000
}

func viewerPreviewCachePath(it *Item, mode string) string {
	if mode != viewerPreviewHQMode {
		mode = viewerPreviewFastMode
	}
	return filepath.Join(thumbDir, fmt.Sprintf("%s-preview-%s-%d-%d-%s.jpg",
		it.ID, mode, it.Mtime, it.Size, viewerPreviewCacheVer))
}

// loadViewerPreview returns a ready-to-draw 2560 bitmap. A disk hit still
// decodes the JPEG, but a miss decodes the original once, scales with
// ApproxBiLinear, creates a bitmap directly, then writes the JPEG cache.
func loadViewerPreview(it *Item) (*viewerPreviewResult, error) {
	res, err := loadViewerPreviewData(it)
	if err != nil {
		return nil, err
	}
	if res.Bmp == nil && res.Img != nil {
		start := time.Now()
		res.Bmp = ui.NewBitmap(res.Img)
		res.BitmapMs += time.Since(start).Milliseconds()
	}
	return res, nil
}

// loadViewerPreviewData is the viewer's own path: it returns raw pixels so the
// app can reuse one persistent ui.Bitmap instead of leaving one texture per
// open in MyGo's texture cache.
func loadViewerPreviewData(it *Item) (*viewerPreviewResult, error) {
	if it == nil {
		return nil, fmt.Errorf("nil item")
	}
	mode := viewerPreviewMode()
	if mode == viewerPreviewLegacyMode {
		start := time.Now()
		data, err := previewBytes(it)
		if err != nil {
			return nil, err
		}
		bitmapStart := time.Now()
		bmp, err := ui.DecodeBitmap(data)
		if err != nil {
			return nil, err
		}
		return &viewerPreviewResult{
			Bmp:      bmp,
			Bytes:    int64(len(data)),
			BitmapMs: time.Since(bitmapStart).Milliseconds(),
			DecodeMs: time.Since(start).Milliseconds(),
		}, nil
	}

	p := viewerPreviewCachePath(it, mode)
	if data, ok := readFreshCache(p, it.Mtime); ok {
		decodeStart := time.Now()
		if img, err := jpeg.Decode(bytes.NewReader(data)); err == nil {
			return &viewerPreviewResult{
				Img:      img,
				Bytes:    int64(len(data)),
				CacheHit: true,
				DecodeMs: time.Since(decodeStart).Milliseconds(),
			}, nil
		}
		// A truncated cache entry must not poison every later open.
		_ = os.Remove(p)
	}

	previewMu.Lock()
	if data, ok := readFreshCache(p, it.Mtime); ok {
		previewMu.Unlock()
		decodeStart := time.Now()
		if img, err := jpeg.Decode(bytes.NewReader(data)); err == nil {
			return &viewerPreviewResult{
				Img:      img,
				Bytes:    int64(len(data)),
				CacheHit: true,
				DecodeMs: time.Since(decodeStart).Milliseconds(),
			}, nil
		}
		_ = os.Remove(p)
		previewMu.Lock()
	}
	defer previewMu.Unlock()

	d := imageDims(it.Path, it.Mtime)
	if !viewerPreviewWithinLimit(d.W, d.H) {
		return nil, fmt.Errorf("image too large to preview")
	}

	f, err := os.Open(it.Path)
	if err != nil {
		return nil, err
	}
	decodeStart := time.Now()
	src, _, err := image.Decode(f)
	_ = f.Close()
	if err != nil {
		return nil, err
	}
	orient := d.Orient
	if orient <= 1 || orient >= 9 {
		orient = 0
	}
	if orient > 0 {
		src = orientImage(src, orient)
	}
	if d.W <= 0 || d.H <= 0 {
		b := src.Bounds()
		d.W, d.H = b.Dx(), b.Dy()
	}
	decodeMs := time.Since(decodeStart).Milliseconds()
	oriented := src.Bounds()
	w, h := oriented.Dx(), oriented.Dy()
	maxDim := viewerPreviewMaxDim
	if w > maxDim || h > maxDim {
		ratio := float64(maxDim) / float64(w)
		if r2 := float64(maxDim) / float64(h); r2 < ratio {
			ratio = r2
		}
		w, h = int(float64(w)*ratio), int(float64(h)*ratio)
		if w < 1 {
			w = 1
		}
		if h < 1 {
			h = 1
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	scaleStart := time.Now()
	if mode == viewerPreviewHQMode {
		scaleImage(dst, src)
	} else {
		scaleImageFast(dst, src)
	}
	scaleMs := time.Since(scaleStart).Milliseconds()

	var buf bytes.Buffer
	encodeStart := time.Now()
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: viewerPreviewQuality}); err != nil {
		return nil, err
	}
	encodeMs := time.Since(encodeStart).Milliseconds()
	data := append([]byte(nil), buf.Bytes()...)
	if strings.ToLower(strings.TrimSpace(os.Getenv("FLOWVISION_VIEWER_CACHE"))) != "off" {
		go writePreviewCache(p, data)
	}

	return &viewerPreviewResult{
		Img:      dst,
		Bytes:    int64(len(data)),
		DecodeMs: decodeMs,
		ScaleMs:  scaleMs,
		EncodeMs: encodeMs,
	}, nil
}

func writePreviewCache(p string, data []byte) {
	tmp := fmt.Sprintf("%s.tmp-%d", p, time.Now().UnixNano())
	if err := os.WriteFile(tmp, data, 0o644); err == nil {
		if err := os.Rename(tmp, p); err != nil {
			_ = os.Remove(tmp)
		}
	}
}

// viewerPlaceholder borrows an already-decoded grid thumbnail. It is drawn
// only until the viewer preview is ready; the grid cache still owns it.
func (a *App) viewerPlaceholder(it *Item) *ui.Bitmap {
	if it == nil || it.Kind != "image" {
		return nil
	}
	sizes := []int{a.settings.ThumbSize, 480, 320, 240, 220, 160, 96, 640, 800, 960}
	for _, size := range sizes {
		if size <= 0 {
			continue
		}
		if bmp := a.thumb(it, size); bmp != nil {
			return bmp
		}
	}
	return nil
}
