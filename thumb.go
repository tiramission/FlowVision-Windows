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
	previewDir   string
	thumbOnce    sync.Once
	thumbSlots   chan struct{}
	thumbWrites  int64
	thumbWaiters int64
	thumbLocks   sync.Map // cache path -> *sync.Mutex, one writer per thumbnail

	// thumbPrefetchSlots keeps the folder fill from occupying every worker:
	// two background decodes leave the rest free for cards that scroll in.
	thumbPrefetchSlots = make(chan struct{}, 2)
	// remoteThumbSlots keeps a network share from being hit by the whole worker
	// pool: six parallel 20 MB reads over SMB starve the link the UI needs.
	remoteThumbSlots = make(chan struct{}, 2)
	thumbLastTrim    atomic.Int64 // unix nanos of the last automatic trim
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
	// Two buckets under one budget: the grid's cards and the viewer's 2560
	// previews have very different lifetimes and sizes, so keeping them apart
	// makes both the folder and the cleanup readable.
	thumbDir = filepath.Join(cacheDir, "thumbs")
	previewDir = filepath.Join(cacheDir, "previews")
	for _, dir := range []string{thumbDir, previewDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			logf("thumbs: mkdir %s: %v", dir, err)
		}
	}
	go migrateLegacyPreviews()
	go trimThumbCache()
}

// migrateLegacyPreviews moves previews written by older builds, which shared
// one directory with the grid thumbnails. Previews cost a full decode to
// rebuild, so moving them is worth the one startup pass.
func migrateLegacyPreviews() {
	if thumbDir == "" || previewDir == "" {
		return
	}
	entries, err := os.ReadDir(thumbDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.Contains(e.Name(), "-preview-") {
			continue
		}
		_ = os.Rename(filepath.Join(thumbDir, e.Name()), filepath.Join(previewDir, e.Name()))
	}
}

// cacheBuckets lists the directories that share the cache size budget.
func cacheBuckets() []string {
	out := make([]string, 0, 2)
	for _, dir := range []string{thumbDir, previewDir} {
		if dir != "" {
			out = append(out, dir)
		}
	}
	return out
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
	thumb, err := loadThumb(ctx, it, size, true)
	if err != nil {
		return nil, err
	}
	return thumb.Data, nil
}

// renderThumbForCard is getThumb for a card that is about to be drawn: the
// pixels come back whenever the card had to be rendered, so the bitmap does
// not decode the file that was just written. The cache file is the caller's
// job, so it lands after the picture is on screen.
func renderThumbForCard(ctx context.Context, it *Item, size int) (thumbRender, error) {
	return loadThumb(ctx, it, size, false)
}

// thumbCachePath is the file a card belongs in, at the bucket its size lands
// in.
func thumbCachePath(it *Item, size int) string {
	return thumbPath(it, thumbBudget(size))
}

// loadThumb reads the card from disk or renders it. writeCache is off for the
// callers that want to post the bitmap first, so the file lands after the
// picture is on screen rather than in front of it.
func loadThumb(ctx context.Context, it *Item, size int, writeCache bool) (thumbRender, error) {
	if thumbDir == "" {
		// initThumbs could not resolve a cache directory. Decoding here would
		// write the cache file next to the executable instead.
		return thumbRender{}, fmt.Errorf("thumbnail cache directory is unavailable")
	}
	size = thumbBudget(size)
	p := thumbPath(it, size)
	if data, ok := readFreshCache(p, it.Mtime); ok {
		return thumbRender{Data: data, Cached: true}, nil
	}
	unlock := lockThumb(p)
	defer unlock()
	if data, ok := readFreshCache(p, it.Mtime); ok {
		return thumbRender{Data: data, Cached: true}, nil
	}
	select {
	case thumbSlots <- struct{}{}:
		defer func() { <-thumbSlots }()
	case <-ctx.Done():
		return thumbRender{}, ctx.Err()
	}
	if data, ok := readFreshCache(p, it.Mtime); ok {
		return thumbRender{Data: data, Cached: true}, nil
	}
	if ctx.Err() != nil {
		return thumbRender{}, ctx.Err()
	}
	thumb, err := renderThumb(it, size)
	if err != nil {
		return thumbRender{}, err
	}
	if writeCache {
		writeThumbCache(p, thumb.Data)
	}
	return thumb, nil
}

// writeThumbCache stores one card. The temporary file plus rename keeps a
// reader from ever seeing half a picture.
func writeThumbCache(p string, data []byte) {
	tmp := fmt.Sprintf("%s.tmp-%d", p, time.Now().UnixNano())
	if err := os.WriteFile(tmp, data, 0o644); err == nil {
		if err := os.Rename(tmp, p); err != nil {
			_ = os.Remove(tmp)
		} else {
			_ = os.Chtimes(p, time.Now(), time.Now())
		}
	}
	if atomic.AddInt64(&thumbWrites, 1)%150 == 0 {
		go trimThumbCacheIfDue()
	}
}

// trimThumbCacheIfDue rate-limits the cache walk. A folder fill writes
// thousands of thumbnails, and walking the cache directory every 150 writes
// would compete with the fill itself.
func trimThumbCacheIfDue() {
	now := time.Now().UnixNano()
	last := thumbLastTrim.Load()
	if last != 0 && now-last < int64(20*time.Second) {
		return
	}
	if !thumbLastTrim.CompareAndSwap(last, now) {
		return
	}
	trimThumbCache()
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

// readThumbCache returns the cached card for it, if there is one. The cache
// lane uses it to answer a scroll-back without touching the decode pool.
func readThumbCache(it *Item, size int) ([]byte, bool) {
	if thumbDir == "" {
		return nil, false
	}
	return readFreshCache(thumbPath(it, thumbBudget(size)), it.Mtime)
}

// thumbCacheExists answers the same question with one stat. It runs once per
// card, when the card is first queued, and only decides which lane the load
// starts in.
func thumbCacheExists(it *Item, size int) bool {
	if thumbDir == "" {
		return false
	}
	st, err := os.Stat(thumbPath(it, thumbBudget(size)))
	return err == nil && st.Size() > 0 && st.ModTime().Unix() >= it.Mtime
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

// thumbRender is one rendered card: the JPEG that goes into the cache and,
// when the renderer was holding them anyway, the pixels it was made from. A
// card that has to appear now builds its bitmap from those pixels instead of
// decoding the file it has just written, which took a second trip through the
// JPEG codec off every first paint.
type thumbRender struct {
	Data []byte
	Img  image.Image
	// Cached marks bytes that came from disk, so the caller knows there is
	// nothing left to write.
	Cached bool
}

// bytesThumb adapts a renderer that only produces bytes.
func bytesThumb(data []byte, err error) (thumbRender, error) {
	return thumbRender{Data: data}, err
}

func renderThumb(it *Item, size int) (thumbRender, error) {
	if it.Kind == "video" {
		if data, err := videoFrame(it, size); err == nil {
			return thumbRender{Data: data, Cached: true}, nil
		}
		return bytesThumb(placeholderThumb(it, size))
	}
	return renderStill(it, size)
}

// renderStill dispatches a still image through the configured backend chain.
// auto is intentionally conservative: WIC first for normal formats, Shell
// first only for formats WIC commonly lacks (HEIC/AVIF/JXL/RAW), and the
// original Go decoder last.
func renderStill(it *Item, size int) (thumbRender, error) {
	if isRemotePath(it.Path) {
		// WIC and the Shell provider need a local path, so a share is decoded
		// from its stream — but only after the prefix has had its chance to
		// answer on its own.
		return renderThumbRemote(it, size)
	}
	switch thumbBackendMode() {
	case "go":
		return renderThumbGo(it, size)
	case "wic":
		if r, err := wicThumb(it, size); err == nil {
			return r, nil
		}
		return renderThumbGo(it, size)
	case "shell":
		if r, err := shellThumb(it, size); err == nil {
			return r, nil
		}
		return renderThumbGo(it, size)
	case "embedded":
		if r, err := embeddedThumb(it, size, 0.75); err == nil {
			return r, nil
		}
		return renderThumbGo(it, size)
	case "dct":
		if r, err := dctThumb(it, size); err == nil {
			return r, nil
		}
		return renderThumbGo(it, size)
	}

	if strings.EqualFold(it.Ext, ".ico") {
		if r, err := icoThumb(it, size); err == nil {
			return r, nil
		}
	}
	if shellPreferred(it.Ext) {
		if r, err := shellThumb(it, size); err == nil {
			return r, nil
		}
	}
	if r, err := wicThumb(it, size); err == nil {
		return r, nil
	}
	if r, err := shellThumb(it, size); err == nil {
		return r, nil
	}
	return renderThumbGo(it, size)
}

// renderThumbRemote decodes a picture over a share. The prefix is read once
// and answers everything it can: dimensions, the EXIF thumbnail as the final
// card when it is large enough, and the dimensions the full decode would
// otherwise ask for again.
func renderThumbRemote(it *Item, size int) (thumbRender, error) {
	head := remoteHead(it.Path)
	if len(head) > 0 {
		d := probeDimsFromHead(head, it.Path)
		if d.W > 0 {
			// Remember the answer: renderThumbGo asks imageDims for exactly
			// this, and the second head request would be pure waste.
			dimCache.Store(dimKey{it.Path, it.Mtime}, d)
		}
		if data, err := remoteEmbeddedFinal(it, head, d, size); err == nil {
			return thumbRender{Data: data, Cached: true}, nil
		}
	}
	if r, err := renderThumbGo(it, size); err == nil {
		return r, nil
	}
	return bytesThumb(placeholderThumb(it, size))
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
func renderThumbGo(it *Item, size int) (thumbRender, error) {
	d := imageDims(it.Path, it.Mtime)
	if d.W <= 0 || d.H <= 0 {
		return thumbRender{}, fmt.Errorf("unsupported image format: %s", it.Ext)
	}
	if d.W*d.H > 150_000_000 {
		return thumbRender{}, fmt.Errorf("image too large to thumbnail")
	}
	f, err := openItemFile(it)
	if err != nil {
		return thumbRender{}, err
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return thumbRender{}, err
	}
	dst := thumbnailImage(src, d, size)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 84}); err != nil {
		return thumbRender{}, err
	}
	return thumbRender{Data: buf.Bytes(), Img: dst}, nil
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
	// ffmpeg cannot use the app's share session, but it reads a URL happily:
	// hand a remote clip to the loopback server and let it fetch the frames.
	source := it.Path
	if isRemotePath(it.Path) {
		url, err := remoteMediaServer.URL(it.Path)
		if err != nil {
			return nil, err
		}
		source = url
	}
	ff := findFFmpeg()
	if ff == "" {
		return nil, fmt.Errorf("ffmpeg not found")
	}
	tries := []string{"0.6", "0.1", "2.0"}
	for _, at := range tries {
		args := []string{
			"-v", "error", "-nostdin", "-y",
			"-ss", at, "-i", source,
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
	info := CacheInfo{}
	if thumbDir != "" {
		info.Dir = filepath.Dir(thumbDir)
	}
	for _, dir := range cacheBuckets() {
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if st, e := d.Info(); e == nil {
				info.Files++
				info.Bytes += st.Size()
			}
			return nil
		})
	}
	return info
}

func clearThumbCache() (bool, error) {
	thumbCacheMu.Lock()
	defer thumbCacheMu.Unlock()
	cleared := false
	for _, dir := range cacheBuckets() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		cleared = true
		for _, e := range entries {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	return cleared, nil
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
	for _, dir := range cacheBuckets() {
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
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
	}
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

// countCacheFiles reports how many files a bucket holds. The debug endpoint
// uses it to show the split between grid thumbnails and viewer previews.
func countCacheFiles(dir string) int {
	if dir == "" {
		return 0
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			n++
		}
	}
	return n
}
