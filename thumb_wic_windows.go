//go:build windows

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"math"
	"os"
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// WIC is the Windows imaging codec. The app deliberately talks to it through
// a tiny hand-written COM shim instead of a large dependency: only three
// interfaces are needed, and keeping the calls explicit makes the fallback
// boundary (unsupported format, no codec, COM failure) obvious.
//
// The factory is free-threaded, but each call still locks its OS thread and
// initializes COM for that thread. A goroutine may migrate otherwise, and COM
// apartments are per-thread.

type wicGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	wicCLSIDFactory = wicGUID{0xcacaf262, 0x9370, 0x4615, [8]byte{0xa1, 0x3b, 0x9f, 0x55, 0x39, 0xda, 0x4c, 0x0a}}
	wicIIDFactory   = wicGUID{0xec5ec8a9, 0xc395, 0x4314, [8]byte{0x9c, 0x77, 0x54, 0xd7, 0xa9, 0x35, 0xff, 0x70}}
	// IWICBitmapSourceTransform lets a decoder deliver fewer pixels than the
	// frame holds. JPEG answers with 1/2, 1/4 or 1/8 by keeping fewer IDCT
	// coefficients, which is where most of a full decode's cost goes.
	//
	// The vtable order is CopyPixels, GetClosestSize, GetClosestPixelFormat,
	// DoesSupportTransform — CopyPixels is first here, not last, which is easy
	// to get wrong because the interface reads like a source.
	wicIIDSourceTransform = wicGUID{0x3b16811b, 0x6a43, 0x4ec9, [8]byte{0xb7, 0x13, 0x3d, 0x5a, 0x0c, 0x13, 0xb9, 0x40}}

	// wicScaledDecodes counts the frames a decoder answered at a reduced size.
	// It is what tells a benchmark or a test that the fast path ran at all.
	wicScaledDecodes atomic.Int64

	wicFmt24bppBGR   = wicGUID{0x6fddc324, 0x4e03, 0x4bfe, [8]byte{0xb1, 0x85, 0x3d, 0x77, 0x76, 0x8d, 0xc9, 0x0c}}
	wicFmt32bppBGR   = wicGUID{0x6fddc324, 0x4e03, 0x4bfe, [8]byte{0xb1, 0x85, 0x3d, 0x77, 0x76, 0x8d, 0xc9, 0x0e}}
	wicFmt32bppBGRA  = wicGUID{0x6fddc324, 0x4e03, 0x4bfe, [8]byte{0xb1, 0x85, 0x3d, 0x77, 0x76, 0x8d, 0xc9, 0x0f}}
	wicFmt32bppPBGRA = wicGUID{0x6fddc324, 0x4e03, 0x4bfe, [8]byte{0xb1, 0x85, 0x3d, 0x77, 0x76, 0x8d, 0xc9, 0x10}}
	wicFmt24bppRGB   = wicGUID{0x6fddc324, 0x4e03, 0x4bfe, [8]byte{0xb1, 0x85, 0x3d, 0x77, 0x76, 0x8d, 0xc9, 0x0b}}
	wicFmt8bppGray   = wicGUID{0x6fddc324, 0x4e03, 0x4bfe, [8]byte{0xb1, 0x85, 0x3d, 0x77, 0x76, 0x8d, 0xc9, 0x08}}
)

type comPtr uintptr

// COM interface pointers are owned by the API, not the Go heap. The vtable
// lookup has to round-trip through uintptr; checkptr cannot prove this pattern.
//
//go:nocheckptr
func (p comPtr) method(index int) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(uintptr(p)))
	return *(*uintptr)(unsafe.Pointer(vtbl + uintptr(index)*unsafe.Sizeof(uintptr(0))))
}

func (p comPtr) call(index int, args ...uintptr) uintptr {
	fn := p.method(index)
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, uintptr(p))
	all = append(all, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}

func (p comPtr) release() {
	if p != 0 {
		p.call(2)
	}
}

func hresultOK(v uintptr) bool { return int32(v) >= 0 }

func hresultErr(what string, v uintptr) error {
	return fmt.Errorf("%s: HRESULT 0x%08x", what, uint32(v))
}

var (
	ole32                = syscall.NewLazyDLL("ole32.dll")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
)

func coCreateInstance(clsid, iid *wicGUID, out *uintptr) error {
	r, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(clsid)),
		0,
		uintptr(windows.CLSCTX_INPROC_SERVER),
		uintptr(unsafe.Pointer(iid)),
		uintptr(unsafe.Pointer(out)),
	)
	if !hresultOK(r) {
		return hresultErr("CoCreateInstance", r)
	}
	return nil
}

func withWIC(fn func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// S_FALSE (already initialized) and RPC_E_CHANGED_MODE (the thread is in
	// another apartment) are both fine for a free-threaded WIC factory.
	r, _, _ := procCoInitializeEx.Call(0, uintptr(windows.COINIT_MULTITHREADED))
	if !hresultOK(r) && uint32(r) != 0x80010106 {
		return hresultErr("CoInitializeEx", r)
	}
	err := fn()
	// Do not call CoUninitialize: the Go runtime may reuse this OS thread for
	// another worker, and balancing a one-time init on every call is fragile.
	// COM's per-thread count is released when the process exits.
	return err
}

func wicCreateFactory() (comPtr, error) {
	var p uintptr
	if err := coCreateInstance(&wicCLSIDFactory, &wicIIDFactory, &p); err != nil {
		return 0, err
	}
	return comPtr(p), nil
}

func wicSize(src comPtr) (w, h int, err error) {
	var sw, sh uint32
	hr := src.call(3, uintptr(unsafe.Pointer(&sw)), uintptr(unsafe.Pointer(&sh)))
	if !hresultOK(hr) {
		return 0, 0, hresultErr("IWICBitmapSource.GetSize", hr)
	}
	if sw == 0 || sh == 0 {
		return 0, 0, fmt.Errorf("WIC returned empty size")
	}
	return int(sw), int(sh), nil
}

func wicPixelFormat(src comPtr) (wicGUID, error) {
	var pf wicGUID
	hr := src.call(4, uintptr(unsafe.Pointer(&pf)))
	if !hresultOK(hr) {
		return wicGUID{}, hresultErr("IWICBitmapSource.GetPixelFormat", hr)
	}
	return pf, nil
}

// wicImage copies a WIC bitmap source into an image.RGBA. Only the formats
// WIC's common decoders produce are handled; anything else falls back to Go's
// image.Decode rather than guessing a stride.
func wicImage(src comPtr) (image.Image, error) {
	w, h, err := wicSize(src)
	if err != nil {
		return nil, err
	}
	pf, err := wicPixelFormat(src)
	if err != nil {
		return nil, err
	}
	bpp := 0
	switch pf {
	case wicFmt24bppBGR, wicFmt24bppRGB:
		bpp = 3
	case wicFmt32bppBGR, wicFmt32bppBGRA, wicFmt32bppPBGRA:
		bpp = 4
	case wicFmt8bppGray:
		bpp = 1
	default:
		return nil, fmt.Errorf("unsupported WIC pixel format %08x", pf.Data1)
	}
	stride := w * bpp
	buf := make([]byte, stride*h)
	hr := src.call(7, 0, uintptr(stride), uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	if !hresultOK(hr) {
		return nil, hresultErr("IWICBitmapSource.CopyPixels", hr)
	}
	runtime.KeepAlive(buf)

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	switch pf {
	case wicFmt24bppBGR:
		for y := 0; y < h; y++ {
			row := y * stride
			for x := 0; x < w; x++ {
				i := row + x*3
				j := y*dst.Stride + x*4
				dst.Pix[j+0] = buf[i+2]
				dst.Pix[j+1] = buf[i+1]
				dst.Pix[j+2] = buf[i+0]
				dst.Pix[j+3] = 0xff
			}
		}
	case wicFmt24bppRGB:
		for y := 0; y < h; y++ {
			row := y * stride
			for x := 0; x < w; x++ {
				i := row + x*3
				j := y*dst.Stride + x*4
				dst.Pix[j+0] = buf[i+0]
				dst.Pix[j+1] = buf[i+1]
				dst.Pix[j+2] = buf[i+2]
				dst.Pix[j+3] = 0xff
			}
		}
	case wicFmt32bppBGR:
		for y := 0; y < h; y++ {
			row := y * stride
			for x := 0; x < w; x++ {
				i := row + x*4
				j := y*dst.Stride + x*4
				dst.Pix[j+0] = buf[i+2]
				dst.Pix[j+1] = buf[i+1]
				dst.Pix[j+2] = buf[i+0]
				dst.Pix[j+3] = 0xff
			}
		}
	case wicFmt32bppBGRA:
		for y := 0; y < h; y++ {
			row := y * stride
			for x := 0; x < w; x++ {
				i := row + x*4
				j := y*dst.Stride + x*4
				dst.Pix[j+0] = buf[i+2]
				dst.Pix[j+1] = buf[i+1]
				dst.Pix[j+2] = buf[i+0]
				dst.Pix[j+3] = buf[i+3]
			}
		}
	case wicFmt32bppPBGRA:
		for y := 0; y < h; y++ {
			row := y * stride
			for x := 0; x < w; x++ {
				i := row + x*4
				a := buf[i+3]
				j := y*dst.Stride + x*4
				if a == 0 {
					dst.Pix[j+0], dst.Pix[j+1], dst.Pix[j+2], dst.Pix[j+3] = 0, 0, 0, 0
					continue
				}
				dst.Pix[j+0] = uint8(uint32(buf[i+2]) * 255 / uint32(a))
				dst.Pix[j+1] = uint8(uint32(buf[i+1]) * 255 / uint32(a))
				dst.Pix[j+2] = uint8(uint32(buf[i+0]) * 255 / uint32(a))
				dst.Pix[j+3] = a
			}
		}
	case wicFmt8bppGray:
		for y := 0; y < h; y++ {
			row := y * stride
			for x := 0; x < w; x++ {
				v := buf[row+x]
				j := y*dst.Stride + x*4
				dst.Pix[j+0], dst.Pix[j+1], dst.Pix[j+2], dst.Pix[j+3] = v, v, v, 0xff
			}
		}
	}
	return dst, nil
}

func wicTargetSize(srcW, srcH, orient, size int) (rawW, rawH, finalW, finalH int) {
	if srcW <= 0 || srcH <= 0 {
		return 0, 0, 0, 0
	}
	aspect := float64(srcW) / float64(srcH)
	if orient >= 5 && orient <= 8 {
		aspect = 1 / aspect
	}
	finalH = size
	finalW = int(math.Round(float64(size) * clampF(aspect, 0.25, 4)))
	if finalW < 8 {
		finalW = 8
	}
	rawW, rawH = finalW, finalH
	if orient >= 5 && orient <= 8 {
		rawW, rawH = finalH, finalW
	}
	return rawW, rawH, finalW, finalH
}

// wicThumb decodes with WIC, preferring the embedded thumbnail. It returns
// ok=false for any failure so renderThumb can keep the pure-Go path.
func wicThumb(it *Item, size int) (thumbRender, error) {
	var out thumbRender
	err := withWIC(func() error {
		factory, err := wicCreateFactory()
		if err != nil {
			return err
		}
		defer factory.release()

		filename, err := windows.UTF16PtrFromString(it.Path)
		if err != nil {
			return err
		}
		var decoder uintptr
		hr := factory.call(3,
			uintptr(unsafe.Pointer(filename)),
			0,
			uintptr(0x80000000), // GENERIC_READ
			0,                   // WICDecodeMetadataCacheOnDemand
			uintptr(unsafe.Pointer(&decoder)),
		)
		if !hresultOK(hr) {
			return hresultErr("CreateDecoderFromFilename", hr)
		}
		dec := comPtr(decoder)
		defer dec.release()

		var frame uintptr
		hr = dec.call(13, 0, uintptr(unsafe.Pointer(&frame)))
		if !hresultOK(hr) {
			return hresultErr("IWICBitmapDecoder.GetFrame", hr)
		}
		fr := comPtr(frame)
		defer fr.release()

		src := fr
		var embedded uintptr
		hr = dec.call(11, uintptr(unsafe.Pointer(&embedded)))
		if hresultOK(hr) && embedded != 0 {
			src = comPtr(embedded)
			defer src.release()
		}

		srcW, srcH, err := wicSize(src)
		if err != nil {
			return err
		}
		orient := 1
		if it.W > 0 {
			// Browse's dimension probe already parsed the APP1 segment.
			if it.Orient >= 2 && it.Orient <= 8 {
				orient = it.Orient
			}
		} else if isJPEG(it.Path) {
			if o := readExif(it.Path).Orientation; o >= 2 && o <= 8 {
				orient = o
			}
		}
		rawW, rawH, finalW, finalH := wicTargetSize(srcW, srcH, orient, size)
		if rawW <= 0 || rawH <= 0 {
			return fmt.Errorf("bad target size")
		}
		// img holds the pixels when the decoder produced them at a reduced
		// size; otherwise the scaler path fills it further down.
		var img image.Image

		// An embedded thumbnail below roughly three quarters of the requested
		// card is visibly soft at final size; decode the frame instead. At or
		// above that ratio the camera thumbnail is a legitimate fast final for
		// the small cache buckets, matching the Phase 3 decision.
		useEmbedded := src != fr && srcW >= rawW*3/4 && srcH >= rawH*3/4
		if useEmbedded && (srcW > rawW || srcH > rawH) {
			// Embedded thumbnails are usually already small. If it is bigger,
			// shrinking it through the scaler keeps memory bounded.
			if scaled, ok := wicScale(factory, src, rawW, rawH); ok {
				src = scaled
				defer src.release()
			}
		}
		if !useEmbedded {
			// Ask the decoder for a smaller frame first: for a 24 MP JPEG this
			// is the difference between decoding every block and decoding one
			// in sixty-four. Twice the card is asked for, because the decoder
			// answers with the closest size it supports and that may be barely
			// above the card; two cards' worth keeps the resize honest.
			if reduced, ok := wicScaledDecode(fr, rawW*2, rawH*2); ok {
				img = reduced
			} else {
				scaled, ok := wicScale(factory, fr, rawW, rawH)
				if !ok {
					return fmt.Errorf("WIC scaler unavailable")
				}
				src = scaled
				defer src.release()
			}
		}

		if img == nil {
			var err error
			img, err = wicImage(src)
			if err != nil {
				return err
			}
		}
		d := dimInfo{W: finalW, H: finalH, Orient: orient}
		dst := thumbnailImage(img, d, size)
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 84}); err != nil {
			return err
		}
		out = thumbRender{Data: buf.Bytes(), Img: dst}
		return nil
	})
	if err != nil {
		return thumbRender{}, err
	}
	if len(out.Data) == 0 {
		return thumbRender{}, fmt.Errorf("WIC produced no JPEG")
	}
	return out, nil
}

// wicScaledDecode asks the decoder itself for a reduced frame. A JPEG decoder
// answers with a DCT-scaled size — half, a quarter or an eighth — which costs a
// fraction of a full decode. The result is still larger than the card, so the
// normal resize finishes the job.
func wicScaledDecode(src comPtr, wantW, wantH int) (image.Image, bool) {
	if os.Getenv("FLOWVISION_WIC_SCALED") == "off" {
		// Troubleshooting switch: forces the full decode plus scaler path.
		return nil, false
	}
	var transform uintptr
	hr := src.call(0,
		uintptr(unsafe.Pointer(&wicIIDSourceTransform)),
		uintptr(unsafe.Pointer(&transform)))
	if !hresultOK(hr) || transform == 0 {
		return nil, false
	}
	tr := comPtr(transform)
	defer tr.release()

	srcW, srcH, err := wicSize(src)
	if err != nil {
		return nil, false
	}
	w, h := uint32(wantW), uint32(wantH)
	if int(w) > srcW {
		w = uint32(srcW)
	}
	if int(h) > srcH {
		h = uint32(srcH)
	}
	// What the decoder can actually deliver for that request.
	if hr := tr.call(4, uintptr(unsafe.Pointer(&w)), uintptr(unsafe.Pointer(&h))); !hresultOK(hr) || w == 0 || h == 0 {
		return nil, false
	}
	if int(w) >= srcW && int(h) >= srcH {
		// No reduction on offer, so the scaler path is just as good.
		return nil, false
	}
	stride := int(w) * 3
	buf := make([]byte, stride*int(h))
	format := wicFmt24bppBGR
	hr = tr.call(3,
		0, // whole frame
		uintptr(w), uintptr(h),
		uintptr(unsafe.Pointer(&format)),
		0, // WICBitmapTransformRotate0: orientation is applied after scaling
		uintptr(stride), uintptr(len(buf)),
		uintptr(unsafe.Pointer(&buf[0])))
	if !hresultOK(hr) {
		return nil, false
	}
	runtime.KeepAlive(buf)

	wicScaledDecodes.Add(1)
	dst := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	for y := 0; y < int(h); y++ {
		row := y * stride
		for x := 0; x < int(w); x++ {
			i := row + x*3
			j := y*dst.Stride + x*4
			dst.Pix[j+0] = buf[i+2]
			dst.Pix[j+1] = buf[i+1]
			dst.Pix[j+2] = buf[i+0]
			dst.Pix[j+3] = 0xff
		}
	}
	return dst, true
}

func wicScale(factory comPtr, src comPtr, w, h int) (comPtr, bool) {
	var scaler uintptr
	hr := factory.call(11, uintptr(unsafe.Pointer(&scaler)))
	if !hresultOK(hr) || scaler == 0 {
		return 0, false
	}
	sc := comPtr(scaler)
	hr = sc.call(8, uintptr(src), uintptr(w), uintptr(h), uintptr(4)) // WICBitmapInterpolationModeHighQualityCubic
	if !hresultOK(hr) {
		sc.release()
		return 0, false
	}
	return sc, true
}
