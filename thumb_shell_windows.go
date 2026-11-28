//go:build windows

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32                  = syscall.NewLazyDLL("shell32.dll")
	gdi32                    = syscall.NewLazyDLL("gdi32.dll")
	procSHCreateItemFromName = shell32.NewProc("SHCreateItemFromParsingName")
	procGetDC                = user32.NewProc("GetDC")
	procReleaseDC            = user32.NewProc("ReleaseDC")
	procGetObject            = gdi32.NewProc("GetObjectW")
	procGetDIBits            = gdi32.NewProc("GetDIBits")
	procDeleteObject         = gdi32.NewProc("DeleteObject")

	shellIIDImageFactory = wicGUID{0xbcc18b79, 0xba16, 0x442f, [8]byte{0x80, 0xc4, 0x8a, 0x59, 0xc3, 0x0c, 0x46, 0x3b}}
)

type bitmap struct {
	Type       int32
	Width      int32
	Height     int32
	WidthBytes int32
	Planes     uint16
	BitsPixel  uint16
	Bits       uintptr
}

type rgbQuad struct {
	Blue, Green, Red, Reserved byte
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]rgbQuad
}

// shellThumb asks Explorer's thumbnail provider for an image. It is used for
// formats WIC cannot decode (HEIC/RAW without a WIC codec) and as a fallback
// when the WIC decoder rejects a file. Shell thumbnails are already upright.
func shellThumb(it *Item, size int) (thumbRender, error) {
	var out []byte
	err := withWIC(func() error {
		filename, err := windows.UTF16PtrFromString(it.Path)
		if err != nil {
			return err
		}
		var factory uintptr
		hr, _, _ := procSHCreateItemFromName.Call(
			uintptr(unsafe.Pointer(filename)),
			0,
			uintptr(unsafe.Pointer(&shellIIDImageFactory)),
			uintptr(unsafe.Pointer(&factory)),
		)
		if !hresultOK(hr) || factory == 0 {
			return hresultErr("SHCreateItemFromParsingName", hr)
		}
		obj := comPtr(factory)
		defer obj.release()

		// Ask for a square that fits the card aspect. BIGGERSIZEOK lets the
		// provider return its cached 1024px image, which we downscale with the
		// same Catmull-Rom path as the rest of the app.
		req := int(size)
		if req < 256 {
			req = 256
		}
		packed := uint64(uint32(req)) | uint64(uint32(req))<<32
		flags := uint32(0x8 | 0x1) // THUMBNAILONLY | BIGGERSIZEOK
		if strings.EqualFold(it.Ext, ".ico") {
			flags = 0x4 | 0x1 // ICONONLY | BIGGERSIZEOK
		}
		var hbm uintptr
		hr = obj.call(3, uintptr(packed), uintptr(flags), uintptr(unsafe.Pointer(&hbm)))
		if !hresultOK(hr) || hbm == 0 {
			return hresultErr("IShellItemImageFactory.GetImage", hr)
		}
		defer procDeleteObject.Call(hbm)

		var bm bitmap
		r, _, _ := procGetObject.Call(hbm, unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm)))
		if r == 0 || bm.Width <= 0 || bm.Height <= 0 {
			return fmt.Errorf("GetObject failed")
		}
		w, h := int(bm.Width), int(bm.Height)
		if w*h > 4096*4096 {
			return fmt.Errorf("shell bitmap too large")
		}
		hdc, _, _ := procGetDC.Call(0)
		if hdc == 0 {
			return fmt.Errorf("GetDC failed")
		}
		defer procReleaseDC.Call(0, hdc)

		bi := bitmapInfo{}
		bi.Header.Size = uint32(unsafe.Sizeof(bi.Header))
		bi.Header.Width = int32(w)
		bi.Header.Height = -int32(h) // top-down
		bi.Header.Planes = 1
		bi.Header.BitCount = 32
		bi.Header.Compression = 0 // BI_RGB
		buf := make([]byte, w*h*4)
		lines, _, _ := procGetDIBits.Call(
			hdc, hbm, 0, uintptr(h),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(unsafe.Pointer(&bi)), 0,
		)
		if int32(lines) <= 0 {
			return fmt.Errorf("GetDIBits failed")
		}
		runtime.KeepAlive(buf)

		alphaAny := false
		for i := 3; i < len(buf); i += 4 {
			if buf[i] != 0 {
				alphaAny = true
				break
			}
		}
		dst := image.NewNRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			row := y * w * 4
			for x := 0; x < w; x++ {
				i := row + x*4
				j := y*dst.Stride + x*4
				dst.Pix[j+0] = buf[i+2]
				dst.Pix[j+1] = buf[i+1]
				dst.Pix[j+2] = buf[i+0]
				if alphaAny {
					dst.Pix[j+3] = buf[i+3]
				} else {
					dst.Pix[j+3] = 0xff
				}
			}
		}
		d := dimInfo{W: w, H: h, Orient: 1}
		final := thumbnailImage(dst, d, size)
		var jb bytes.Buffer
		if err := jpeg.Encode(&jb, final, &jpeg.Options{Quality: 84}); err != nil {
			return err
		}
		out = jb.Bytes()
		return nil
	})
	if err != nil || len(out) == 0 {
		return thumbRender{}, err
	}
	return thumbRender{Data: out}, nil
}
