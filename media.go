package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

var (
	mediaBase  string
	mediaToken string
)

var contentTypes = map[string]string{
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".jpe": "image/jpeg", ".jfif": "image/jpeg",
	".png": "image/png", ".gif": "image/gif", ".webp": "image/webp",
	".bmp": "image/bmp", ".ico": "image/x-icon", ".avif": "image/avif",
	".svg": "image/svg+xml", ".tif": "image/tiff", ".tiff": "image/tiff",
	".heic": "image/heic", ".heif": "image/heif", ".jxl": "image/jxl",
	".mp4": "video/mp4", ".m4v": "video/mp4", ".mov": "video/quicktime",
	".webm": "video/webm", ".mkv": "video/x-matroska", ".avi": "video/x-msvideo",
	".wmv": "video/x-ms-wmv", ".flv": "video/x-flv", ".ts": "video/mp2t",
	".m2ts": "video/mp2t", ".mpg": "video/mpeg", ".mpeg": "video/mpeg",
	".3gp": "video/3gpp", ".ogv": "video/ogg",
	".mp3": "audio/mpeg", ".m4a": "audio/mp4", ".wav": "audio/wav",
	".flac": "audio/flac", ".aac": "audio/aac", ".ogg": "audio/ogg",
}

func contentType(name string) string {
	if ct, ok := contentTypes[strings.ToLower(filepath.Ext(name))]; ok {
		return ct
	}
	return "application/octet-stream"
}

func browserImage(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".jpe", ".jfif", ".png", ".gif", ".webp", ".bmp",
		".avif", ".ico", ".svg":
		return true
	}
	return false
}

func registerImageProtocol() {
	if err := mygo.Protocol.HandleFunc("img", serveImageRequest); err != nil {
		logf("protocol img: %v", err)
	}
}

// serveImageRequest serves /t/<id> thumbnails, /o/<id> originals and /p/<id>
// converted previews of formats the webview cannot decode.
func serveImageRequest(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	it, ok := items.Get(parts[1])
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	switch parts[0] {
	case "t":
		size := parseIntDefault(r.URL.Query().Get("s"), 240)
		data, err := getThumb(r.Context(), it, size)
		if err != nil {
			w.Header().Set("X-FlowVision-Error", err.Error())
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		_, _ = w.Write(data)
	case "o":
		serveItemContent(w, r, it)
	case "p":
		data, err := previewBytes(it)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "public, max-age=604800")
		_, _ = w.Write(data)
	default:
		http.NotFound(w, r)
	}
}

func serveItemContent(w http.ResponseWriter, r *http.Request, it *Item) {
	f, err := os.Open(it.Path)
	if err != nil {
		http.Error(w, "cannot open", http.StatusNotFound)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.Error(w, "cannot stat", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType(it.Name))
	w.Header().Set("Content-Disposition", "inline")
	http.ServeContent(w, r, it.Name, st.ModTime(), f)
}

// previewMu keeps a folder full of TIFF thumbnails from decoding several
// full-size images at once. Cache hits are cheap but still serialized.
var previewMu sync.Mutex

// previewBytes decodes formats Chromium lacks (TIFF) into JPEG.
func previewBytes(it *Item) ([]byte, error) {
	previewMu.Lock()
	defer previewMu.Unlock()
	p := filepath.Join(thumbDir, fmt.Sprintf("%s-preview-%d-%d.jpg", it.ID, it.Mtime, it.Size))
	if data, ok := readFreshCache(p, it.Mtime); ok {
		return data, nil
	}
	d := imageDims(it.Path, it.Mtime)
	if d.W > 0 && d.H > 0 && d.W*d.H > 80_000_000 {
		return nil, fmt.Errorf("image too large to preview")
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
	if o := d.Orient; o > 1 && o < 9 {
		src = orientImage(src, o)
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	maxDim := 2560
	if w > maxDim || h > maxDim {
		ratio := float64(maxDim) / float64(w)
		if r2 := float64(maxDim) / float64(h); r2 < ratio {
			ratio = r2
		}
		w, h = int(float64(w)*ratio), int(float64(h)*ratio)
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	scaleImage(dst, src)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	tmp := fmt.Sprintf("%s.tmp-%d", p, time.Now().UnixNano())
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err == nil {
		if err := os.Rename(tmp, p); err != nil {
			_ = os.Remove(tmp)
		}
	}
	return buf.Bytes(), nil
}

// ------------------------------------------------------------ media streaming

// startMediaServer serves videos over loopback HTTP so that seeking works
// with range requests on every platform.
func startMediaServer() {
	tok := make([]byte, 6)
	_, _ = rand.Read(tok)
	mediaToken = hex.EncodeToString(tok)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		logf("media server: %v", err)
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/m/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") != mediaToken {
			http.NotFound(w, r)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/m/")
		it, ok := items.Get(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		serveItemContent(w, r, it)
	})
	go func() {
		_ = http.Serve(ln, mux)
	}()
	mediaBase = "http://" + ln.Addr().String()
	logf("media server on %s", mediaBase)
}

// MediaURL builds the loopback URL of an item's bytes.
func mediaURL(it *Item) string {
	return fmt.Sprintf("%s/m/%s?t=%s", mediaBase, it.ID, mediaToken)
}

// nowUnix is a small helper for templates.
func nowUnix() int64 { return time.Now().Unix() }
