package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/jpeg"
	"image/png"
	"os"
)

// icoThumb decodes the PNG-compressed frame that modern icon files use. Go's
// standard library has no ICO decoder, and Explorer can reject icons that
// carry PNG payloads on some Windows builds, so this small path keeps ICO
// previews deterministic. Classic DIB-only icons still go through Shell.
func icoThumb(it *Item, size int) ([]byte, error) {
	f, err := os.Open(it.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var dir [6]byte
	if _, err := f.Read(dir[:]); err != nil {
		return nil, err
	}
	if binary.LittleEndian.Uint16(dir[0:2]) != 0 || binary.LittleEndian.Uint16(dir[2:4]) != 1 {
		return nil, fmt.Errorf("not an ICO file")
	}
	count := int(binary.LittleEndian.Uint16(dir[4:6]))
	if count <= 0 || count > 64 {
		return nil, fmt.Errorf("bad ICO entry count %d", count)
	}

	type entry struct {
		w, h int
		off  int64
		len  int64
	}
	var best entry
	entries := make([]byte, count*16)
	if _, err := f.Read(entries); err != nil {
		return nil, err
	}
	for i := 0; i < count; i++ {
		e := entries[i*16:]
		w, h := int(e[0]), int(e[1])
		if w == 0 {
			w = 256
		}
		if h == 0 {
			h = 256
		}
		length := int64(binary.LittleEndian.Uint32(e[8:12]))
		offset := int64(binary.LittleEndian.Uint32(e[12:16]))
		if length <= 0 || offset <= 0 || w*h <= best.w*best.h {
			continue
		}
		best = entry{w: w, h: h, off: offset, len: length}
	}
	if best.w == 0 {
		return nil, fmt.Errorf("no usable ICO frame")
	}

	payload := make([]byte, best.len)
	if _, err := f.ReadAt(payload, best.off); err != nil {
		return nil, err
	}
	if len(payload) < 8 || !bytes.Equal(payload[:8], []byte("\x89PNG\r\n\x1a\n")) {
		return nil, fmt.Errorf("ICO frame is not PNG")
	}
	src, err := png.Decode(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	dst := thumbnailImage(src, dimInfo{W: src.Bounds().Dx(), H: src.Bounds().Dy(), Orient: 1}, size)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 84}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
