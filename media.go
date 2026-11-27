package main

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
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

func serveItemContent(w http.ResponseWriter, r *http.Request, it *Item) {
	if isRemotePath(it.Path) {
		// The remote file has no local handle to ServeContent from; the
		// media server already answers Range for virtual paths.
		http.Error(w, "远程文件由媒体服务提供", http.StatusNotImplemented)
		return
	}
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
	p := ""
	if previewDir != "" {
		p = filepath.Join(previewDir, fmt.Sprintf("%s-preview-%d-%d.jpg", it.ID, it.Mtime, it.Size))
		if data, ok := readFreshCache(p, it.Mtime); ok {
			return data, nil
		}
	}
	d := imageDims(it.Path, it.Mtime)
	if d.W > 0 && d.H > 0 && d.W*d.H > viewerPreviewMaxPixels {
		return nil, fmt.Errorf("图片像素 %d×%d 超过预览上限（8000 万）", d.W, d.H)
	}
	f, err := openItemFile(it)
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
	if p != "" {
		tmp := fmt.Sprintf("%s.tmp-%d", p, time.Now().UnixNano())
		if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err == nil {
			if err := os.Rename(tmp, p); err != nil {
				_ = os.Remove(tmp)
			}
		}
	}
	return buf.Bytes(), nil
}

// nowUnix is a small helper for templates.
func nowUnix() int64 { return time.Now().Unix() }
