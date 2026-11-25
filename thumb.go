package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

var (
	thumbDir     string
	thumbOnce    sync.Once
	thumbSlots   chan struct{}
	thumbWrites  int64
	thumbWaiters int64
	thumbLocks   sync.Map // cache path -> *sync.Mutex, one writer per thumbnail
)

// lockThumb serializes the generation of one cache file. Without it, several
// grid cells asking for the same thumbnail at once all render it and race on
// the rename, which can leave no cache file on Windows.
func lockThumb(path string) func() {
	v, _ := thumbLocks.LoadOrStore(path, &sync.Mutex{})
	m := v.(*sync.Mutex)
	m.Lock()
	return m.Unlock
}

func initThumbs() {
	n := runtime.NumCPU()
	if n > 6 {
		n = 6
	}
	if n < 2 {
		n = 2
	}
	thumbSlots = make(chan struct{}, n)
	cacheDir, err := mygo.App.Path(mygo.PathCache)
	if err != nil {
		logf("thumbs: cache dir: %v", err)
		return
	}
	thumbDir = filepath.Join(cacheDir, "thumbs")
	if err := os.MkdirAll(thumbDir, 0o755); err != nil {
		logf("thumbs: mkdir: %v", err)
	}
	go trimThumbCache()
}

func thumbBudget(size int) int {
	switch {
	case size <= 96:
		return 96
	case size <= 160:
		return 160
	case size <= 240:
		return 240
	case size <= 320:
		return 320
	case size <= 480:
		return 480
	case size <= 640:
		return 640
	case size <= 800:
		return 800
	}
	return 960
}

func thumbPath(it *Item, size int) string {
	// Include the source version and backend generation in the cache name: a
	// file replaced within the same second with a different size must not
	// reuse the old bytes, and a WIC/Shell result must not hide a Go result
	// after a troubleshooting backend switch.
	return filepath.Join(thumbDir, fmt.Sprintf("%s-%d-%d-%d-%s.jpg", it.ID, size, it.Mtime, it.Size, thumbCacheVersion))
}

const thumbCacheVersion = "v2"

// getThumb returns the JPEG bytes of a thumbnail for it, generating and
// caching them on first use.
func getThumb(ctx context.Context, it *Item, size int) ([]byte, error) {
	size = thumbBudget(size)
	p := thumbPath(it, size)
	if data, ok := readFreshCache(p, it.Mtime); ok {
		return data, nil
	}
	unlock := lockThumb(p)
	defer unlock()
	if data, ok := readFreshCache(p, it.Mtime); ok {
		return data, nil
	}
	select {
	case thumbSlots <- struct{}{}:
		defer func() { <-thumbSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if data, ok := readFreshCache(p, it.Mtime); ok {
		return data, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	data, err := renderThumb(it, size)
	if err != nil {
		return nil, err
	}
	tmp := fmt.Sprintf("%s.tmp-%d", p, time.Now().UnixNano())
	if err := os.WriteFile(tmp, data, 0o644); err == nil {
		if err := os.Rename(tmp, p); err != nil {
			_ = os.Remove(tmp)
		} else {
			_ = os.Chtimes(p, time.Now(), time.Now())
		}
	}
	if atomic.AddInt64(&thumbWrites, 1)%150 == 0 {
		go trimThumbCache()
	}
	return data, nil
}

// brokenThumb draws the card used when every decoder rejects a file. It keeps
// the card's aspect ratio and shows a muted warning mark instead of a blank
// hole, without caching the failure to disk.
func brokenThumb(it *Item, size int) ([]byte, error) {
	w, h := size, size
	if it.Kind == "image" {
		d := imageDims(it.Path, it.Mtime)
		if d.W > 0 && d.H > 0 {
			h = size
			w = int(math.Round(float64(size) * clampF(float64(d.W)/float64(d.H), 0.25, 4)))
			if w < 8 {
				w = 8
			}
		}
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{thumbSurfaceR, thumbSurfaceG, thumbSurfaceB, 255})
		}
	}
	m := int(math.Min(float64(w), float64(h)) * 0.18)
	if m < 8 {
		m = 8
	}
	cx, cy := w/2, h/2
	for i := -m; i <= m; i++ {
		for t := -2; t <= 2; t++ {
			if cx+i >= 0 && cx+i < w && cy+i+t >= 0 && cy+i+t < h {
				img.SetRGBA(cx+i, cy+i+t, color.RGBA{170, 110, 110, 255})
			}
			if cx+i >= 0 && cx+i < w && cy-i+t >= 0 && cy-i+t < h {
				img.SetRGBA(cx+i, cy-i+t, color.RGBA{170, 110, 110, 255})
			}
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 82}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func readFreshCache(p string, srcMtime int64) ([]byte, bool) {
	st, err := os.Stat(p)
	if err != nil || st.Size() == 0 {
		return nil, false
	}
	if st.ModTime().Unix() < srcMtime {
		return nil, false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}
	return data, true
}

func renderThumb(it *Item, size int) ([]byte, error) {
	if it.Kind == "video" {
		if data, err := videoFrame(it, size); err == nil {
			return data, nil
		}
		return placeholderThumb(it, size)
	}
	return renderStill(it, size)
}

// renderStill dispatches a still image through the configured backend chain.
// auto is intentionally conservative: WIC first for normal formats, Shell
// first only for formats WIC commonly lacks (HEIC/AVIF/JXL/RAW), and the
// original Go decoder last.
func renderStill(it *Item, size int) ([]byte, error) {
	switch thumbBackendMode() {
	case "go":
		return renderThumbGo(it, size)
	case "wic":
		if data, err := wicThumb(it, size); err == nil {
			return data, nil
		}
		return renderThumbGo(it, size)
	case "shell":
		if data, err := shellThumb(it, size); err == nil {
			return data, nil
		}
		return renderThumbGo(it, size)
	case "embedded":
		if data, err := embeddedThumb(it, size, 0.75); err == nil {
			return data, nil
		}
		return renderThumbGo(it, size)
	case "dct":
		if data, err := dctThumb(it, size); err == nil {
			return data, nil
		}
		return renderThumbGo(it, size)
	}

	if strings.EqualFold(it.Ext, ".ico") {
		if data, err := icoThumb(it, size); err == nil {
			return data, nil
		}
	}
	if shellPreferred(it.Ext) {
		if data, err := shellThumb(it, size); err == nil {
			return data, nil
		}
	}
	if data, err := wicThumb(it, size); err == nil {
		return data, nil
	}
	if data, err := shellThumb(it, size); err == nil {
		return data, nil
	}
	return renderThumbGo(it, size)
}

func shellPreferred(ext string) bool {
	switch strings.ToLower(ext) {
	case ".heic", ".heif", ".avif", ".jxl", ".ico",
		".cr2", ".cr3", ".nef", ".arw", ".dng", ".orf", ".rw2", ".raf", ".pef", ".srw":
		return true
	}
	return false
}

func thumbBackendMode() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FLOWVISION_THUMB_BACKEND"))) {
	case "wic", "shell", "go", "embedded", "dct":
		return strings.ToLower(strings.TrimSpace(os.Getenv("FLOWVISION_THUMB_BACKEND")))
	}
	return "auto"
}

// renderThumbGo is the original pure-Go path and the final fallback.
func renderThumbGo(it *Item, size int) ([]byte, error) {
	d := imageDims(it.Path, it.Mtime)
	if d.W <= 0 || d.H <= 0 {
		return nil, fmt.Errorf("unsupported image format: %s", it.Ext)
	}
	if d.W*d.H > 150_000_000 {
		return nil, fmt.Errorf("image too large to thumbnail")
	}
	f, err := os.Open(it.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	dst := thumbnailImage(src, d, size)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 84}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// scaleImage draws src into dst with Catmull-Rom, the high quality resampler
// that keeps moire down as the originals' previews do.
func scaleImage(dst draw.Image, src image.Image) {
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
}

// orientImage applies an EXIF orientation (1..8) to a decoded image.
func orientImage(src image.Image, o int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	var out *image.NRGBA
	swap := o >= 5 && o <= 8
	if swap {
		out = image.NewNRGBA(image.Rect(0, 0, h, w))
	} else {
		out = image.NewNRGBA(image.Rect(0, 0, w, h))
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			var dx, dy int
			switch o {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			default:
				dx, dy = x, y
			}
			out.SetNRGBA(dx, dy, color.NRGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), uint8(a >> 8)})
		}
	}
	return out
}

// --------------------------------------------------------------- video frames

var (
	ffmpegOnce sync.Once
	ffmpegExe  string
)

func findFFmpeg() string {
	ffmpegOnce.Do(func() {
		candidates := []string{}
		if exe, err := mygo.App.Path(mygo.PathExe); err == nil {
			dir := filepath.Dir(exe)
			candidates = append(candidates,
				filepath.Join(dir, "ffmpeg.exe"),
				filepath.Join(dir, "bin", "ffmpeg.exe"),
				filepath.Join(dir, "resources", "ffmpeg.exe"),
			)
		}
		for _, c := range candidates {
			if st, err := os.Stat(c); err == nil && !st.IsDir() {
				ffmpegExe = c
				return
			}
		}
		if p, err := exec.LookPath("ffmpeg.exe"); err == nil {
			ffmpegExe = p
			return
		}
		if p, err := exec.LookPath("ffmpeg"); err == nil {
			ffmpegExe = p
		}
	})
	return ffmpegExe
}

func videoFrame(it *Item, size int) ([]byte, error) {
	ff := findFFmpeg()
	if ff == "" {
		return nil, fmt.Errorf("ffmpeg not found")
	}
	tries := []string{"0.6", "0.1", "2.0"}
	for _, at := range tries {
		args := []string{
			"-v", "error", "-nostdin", "-y",
			"-ss", at, "-i", it.Path,
			"-frames:v", "1",
			"-vf", fmt.Sprintf("scale=-2:%d", thumbBudget(size)),
			"-f", "image2pipe", "-vcodec", "mjpeg", "pipe:1",
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		cmd := exec.CommandContext(ctx, ff, args...)
		hideConsoleWindow(cmd)
		var out bytes.Buffer
		cmd.Stdout = &out
		err := cmd.Run()
		cancel()
		if err == nil && out.Len() > 1024 {
			return out.Bytes(), nil
		}
	}
	return nil, fmt.Errorf("no frame")
}

// placeholderThumb draws the card a video without a real frame shows.
func placeholderThumb(it *Item, size int) ([]byte, error) {
	w := int(math.Round(float64(size) * 16.0 / 9.0))
	h := size
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		t := float64(y) / float64(h)
		for x := 0; x < w; x++ {
			s := float64(x) / float64(w)
			v := 0.10 + 0.10*t + 0.05*s
			img.Set(x, y, color.RGBA{uint8(255 * v), uint8(255 * (v + 0.01)), uint8(255 * (v + 0.04)), 255})
		}
	}
	cx, cy := w/2, h/2
	r := int(math.Min(float64(w), float64(h)) * 0.18)
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if x < 0 || y < 0 || x >= w || y >= h {
				continue
			}
			dx, dy := float64(x-cx), float64(y-cy)
			if dx*dx+dy*dy <= float64(r*r) {
				old := img.RGBAAt(x, y)
				img.SetRGBA(x, y, color.RGBA{
					uint8(0.55*float64(old.R) + 0.45*235),
					uint8(0.55*float64(old.G) + 0.45*238),
					uint8(0.55*float64(old.B) + 0.45*242),
					255,
				})
			}
		}
	}
	// Play triangle inside the disc.
	for y := cy - r/2; y <= cy+r/2; y++ {
		for x := cx - r/2; x <= cx+r/3; x++ {
			if x < 0 || y < 0 || x >= w || y >= h {
				continue
			}
			frac := float64(y-(cy-r/2)) / float64(r)
			half := math.Min(frac, 1-frac) * float64(r) * 0.9
			if math.Abs(float64(y-cy)) <= half {
				img.SetRGBA(x, y, color.RGBA{24, 24, 28, 255})
			}
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 82}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ----------------------------------------------------------------- cache trim

func thumbCacheInfo() CacheInfo {
	info := CacheInfo{Dir: thumbDir}
	_ = filepath.WalkDir(thumbDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if st, e := d.Info(); e == nil {
			info.Files++
			info.Bytes += st.Size()
		}
		return nil
	})
	return info
}

func clearThumbCache() (bool, error) {
	thumbCacheMu.Lock()
	defer thumbCacheMu.Unlock()
	entries, err := os.ReadDir(thumbDir)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		_ = os.Remove(filepath.Join(thumbDir, e.Name()))
	}
	return true, nil
}

var thumbCacheMu sync.Mutex

type ageFile struct {
	path  string
	size  int64
	atime time.Time
}

func trimThumbCache() {
	thumbCacheMu.Lock()
	defer thumbCacheMu.Unlock()
	limit := int64(getSettings().CacheLimitMB) << 20
	if limit <= 0 {
		return
	}
	var files []ageFile
	var total int64
	_ = filepath.WalkDir(thumbDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		st, e := d.Info()
		if e != nil {
			return nil
		}
		total += st.Size()
		files = append(files, ageFile{p, st.Size(), st.ModTime()})
		return nil
	})
	if total <= limit {
		return
	}
	target := limit * 8 / 10
	sort.Slice(files, func(i, j int) bool { return files[i].atime.Before(files[j].atime) })
	for _, f := range files {
		if total <= target {
			break
		}
		if err := os.Remove(f.path); err == nil {
			total -= f.size
		}
	}
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return n
}
