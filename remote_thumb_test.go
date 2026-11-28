package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"testing"
)

// jpegWithThumb builds a JPEG carrying an EXIF APP1 segment whose IFD1 points
// at a second JPEG. Cameras write exactly this, and the prefix path is only
// worth anything when the parser can find the thumbnail in it.
func jpegWithThumb(t *testing.T, mainW, mainH, thumbW, thumbH int) []byte {
	t.Helper()
	main := encodeTestJPEG(t, mainW, mainH, color.RGBA{R: 40, G: 90, B: 150, A: 255})
	thumb := encodeTestJPEG(t, thumbW, thumbH, color.RGBA{R: 200, G: 60, B: 40, A: 255})
	if len(main) < 2 || main[0] != 0xFF || main[1] != 0xD8 {
		t.Fatal("not a JPEG")
	}

	// TIFF header: little endian, magic 42, IFD0 at offset 8.
	// IFD0: one entry (Orientation) whose next-IFD field names IFD1.
	// IFD1: Compression=6, JPEGInterchangeFormat(+Length), next = 0.
	// The thumbnail bytes follow the IFD1 table directly.
	const ifd0 = 8
	const ifd0Len = 2 + 12 + 4
	const ifd1 = ifd0 + ifd0Len
	const ifd1Len = 2 + 12*3 + 4
	const thumbAt = ifd1 + ifd1Len

	tiff := make([]byte, thumbAt)
	copy(tiff[0:], []byte{'I', 'I', 42, 0})
	binary.LittleEndian.PutUint32(tiff[4:], ifd0)

	le := binary.LittleEndian
	le.PutUint16(tiff[ifd0:], 1)
	le.PutUint16(tiff[ifd0+2:], 0x0112) // Orientation
	le.PutUint16(tiff[ifd0+4:], 3)      // SHORT
	le.PutUint32(tiff[ifd0+6:], 1)
	le.PutUint16(tiff[ifd0+10:], 1)
	le.PutUint32(tiff[ifd0+14:], ifd1)

	le.PutUint16(tiff[ifd1:], 3)
	le.PutUint16(tiff[ifd1+2:], 0x0103) // Compression
	le.PutUint16(tiff[ifd1+4:], 3)
	le.PutUint32(tiff[ifd1+6:], 1)
	le.PutUint16(tiff[ifd1+10:], 6) // JPEG compression, required by the parser
	le.PutUint16(tiff[ifd1+14:], 0x0201)
	le.PutUint16(tiff[ifd1+16:], 4)
	le.PutUint32(tiff[ifd1+18:], 1)
	le.PutUint32(tiff[ifd1+22:], thumbAt)
	le.PutUint16(tiff[ifd1+26:], 0x0202)
	le.PutUint16(tiff[ifd1+28:], 4)
	le.PutUint32(tiff[ifd1+30:], 1)
	le.PutUint32(tiff[ifd1+34:], uint32(len(thumb)))
	le.PutUint32(tiff[ifd1+38:], 0)

	payload := append([]byte("Exif\x00\x00"), tiff...)
	payload = append(payload, thumb...)
	segLen := len(payload) + 2
	if segLen > 0xFFFF {
		t.Fatal("APP1 segment too large for this fixture")
	}
	var out bytes.Buffer
	out.Write(main[:2])
	out.Write([]byte{0xFF, 0xE1, byte(segLen >> 8), byte(segLen)})
	out.Write(payload)
	out.Write(main[2:])
	return out.Bytes()
}

func encodeTestJPEG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Some gradient, so the encoder cannot collapse the picture into a
			// flat block and hide a scaling mistake.
			img.SetRGBA(x, y, color.RGBA{
				R: uint8((int(c.R) + x) % 256),
				G: uint8((int(c.G) + y) % 256),
				B: uint8((int(c.B) + x + y) % 256),
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// withRemoteBytes serves every remote path from one buffer and records how
// many times the share was opened.
func withRemoteBytes(t *testing.T, data []byte) *int {
	t.Helper()
	opens := 0
	prev := remoteOpenFn
	remoteOpenFn = func(ctx context.Context, p string) (io.ReadCloser, error) {
		opens++
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	prevHead := len(remoteHeadCache.items)
	_ = prevHead
	forgetAllRemoteHeads()
	t.Cleanup(func() {
		remoteOpenFn = prev
		forgetAllRemoteHeads()
	})
	return &opens
}

// forgetAllRemoteHeads clears the prefix cache between fixtures, whose paths
// repeat.
func forgetAllRemoteHeads() {
	remoteHeadCache.mu.Lock()
	remoteHeadCache.items = map[string][]byte{}
	remoteHeadCache.order = nil
	remoteHeadCache.mu.Unlock()
}

// TestRemoteThumbUsesTheEmbeddedFinal is the point of the prefix path: a card
// can be answered without moving the whole picture across the link.
func TestRemoteThumbUsesTheEmbeddedFinal(t *testing.T) {
	data := jpegWithThumb(t, 3000, 2000, 640, 427)
	opens := withRemoteBytes(t, data)

	it := &Item{ID: "remote-1", Name: "big.jpg", Kind: "image", Ext: ".jpg",
		Path: "smb://nas/photos/big.jpg", Size: int64(len(data))}
	rendered, err := renderThumbRemote(it, 220)
	if err != nil {
		t.Fatalf("renderThumbRemote: %v", err)
	}
	out := rendered.Data
	if len(out) == 0 {
		t.Fatal("no thumbnail produced")
	}
	// One read of the prefix, and no second read for the full frame: the whole
	// point is that a card no longer costs a whole picture.
	if *opens != 1 {
		t.Fatalf("the share was opened %d times, want 1 (the prefix only)", *opens)
	}

	// The result is the card size, not the full picture.
	cfg, err := jpegConfig(out)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Height != 220 {
		t.Fatalf("thumbnail height = %d, want 220", cfg.Height)
	}
}

// TestRemoteThumbFallsBackWhenTheThumbIsSmall covers the other half: a camera
// thumbnail far below the card size must not be used as the final image.
func TestRemoteThumbFallsBackWhenTheThumbIsSmall(t *testing.T) {
	data := jpegWithThumb(t, 3000, 2000, 160, 107)
	withRemoteBytes(t, data)

	it := &Item{ID: "remote-2", Name: "big.jpg", Kind: "image", Ext: ".jpg",
		Path: "smb://nas/photos/big.jpg", Size: int64(len(data))}
	if _, err := remoteEmbeddedFinal(it, data, probeDimsFromHead(data, it.Path), 220); err == nil {
		t.Fatal("a 160 px thumbnail was accepted for a 220 px card")
	}
	// And the full decode still produces a usable card.
	rendered, err := renderThumbRemote(it, 220)
	if err != nil || len(rendered.Data) == 0 {
		t.Fatalf("fallback produced nothing: %v", err)
	}
}

// TestRemotePrefixIsReadOnce covers the traffic change: dimensions, the EXIF
// preview and the final decode share a single prefix request per picture.
func TestRemotePrefixIsReadOnce(t *testing.T) {
	data := jpegWithThumb(t, 3000, 2000, 160, 107)
	opens := withRemoteBytes(t, data)

	it := &Item{ID: "remote-3", Name: "big.jpg", Kind: "image", Ext: ".jpg",
		Path: "smb://nas/photos/big.jpg", Size: int64(len(data))}
	_ = imageDims(it.Path, it.Mtime)
	_ = readExif(it.Path)
	_ = remoteHead(it.Path)

	if *opens != 1 {
		t.Fatalf("the prefix was fetched %d times, want 1", *opens)
	}
}

// jpegConfig reads the produced card's dimensions.
func jpegConfig(data []byte) (image.Config, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	return cfg, err
}
