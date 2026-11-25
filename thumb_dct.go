package main

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"os"
)

// This is the Phase 4 research prototype: a baseline-JPEG DC-only decoder.
// It intentionally does not replace image/jpeg. It parses the entropy stream,
// keeps only the DC coefficient of each 8x8 block, and produces a 1/8-size
// image. That is enough to judge whether a pure-Go scaled decode is worth
// maintaining; progressive JPEG, CMYK and exotic restart layouts deliberately
// fall back to the normal decoder.

type dctComponent struct {
	id, h, v, tq int
	dcTable      int
	acTable      int
}

type dctHuff struct {
	counts  [17]int
	symbols []byte
	minCode [17]int
	maxCode [17]int
	valPtr  [17]int
}

func (h *dctHuff) build(counts [17]int, symbols []byte) error {
	h.counts = counts
	h.symbols = symbols
	code := 0
	k := 0
	for l := 1; l <= 16; l++ {
		h.valPtr[l] = k
		h.minCode[l] = code
		code += counts[l]
		k += counts[l]
		if counts[l] > 0 {
			h.maxCode[l] = code - 1
		} else {
			h.maxCode[l] = -1
		}
		code <<= 1
	}
	if k != len(symbols) {
		return fmt.Errorf("bad huffman table")
	}
	return nil
}

func (h *dctHuff) decode(br *dctBits) (int, error) {
	code := 0
	for l := 1; l <= 16; l++ {
		b, err := br.bit()
		if err != nil {
			return 0, err
		}
		code = code<<1 | b
		if h.maxCode[l] >= 0 && code >= h.minCode[l] && code <= h.maxCode[l] {
			idx := h.valPtr[l] + code - h.minCode[l]
			if idx >= 0 && idx < len(h.symbols) {
				return int(h.symbols[idx]), nil
			}
		}
	}
	return 0, fmt.Errorf("huffman code not found")
}

type dctBits struct {
	data []byte
	pos  int
	buf  uint32
	n    uint
}

func (b *dctBits) reset() { b.buf, b.n = 0, 0 }

func (b *dctBits) byte() (byte, error) {
	if b.pos >= len(b.data) {
		return 0, fmt.Errorf("entropy data ended")
	}
	v := b.data[b.pos]
	b.pos++
	if v == 0xFF {
		if b.pos < len(b.data) && b.data[b.pos] == 0x00 {
			b.pos++
			return 0xFF, nil
		}
		return 0, fmt.Errorf("unexpected JPEG marker in entropy data")
	}
	return v, nil
}

func (b *dctBits) bit() (int, error) {
	if b.n == 0 {
		v, err := b.byte()
		if err != nil {
			return 0, err
		}
		b.buf, b.n = uint32(v), 8
	}
	b.n--
	return int((b.buf >> b.n) & 1), nil
}

func (b *dctBits) bits(n int) (int, error) {
	v := 0
	for i := 0; i < n; i++ {
		bit, err := b.bit()
		if err != nil {
			return 0, err
		}
		v = v<<1 | bit
	}
	return v, nil
}

func dctExtend(v, n int) int {
	if n == 0 {
		return 0
	}
	if v < 1<<(n-1) {
		return v - (1 << n) + 1
	}
	return v
}

type dctDecoder struct {
	data       []byte
	pos        int
	width      int
	height     int
	components []dctComponent
	quant      map[int][64]int
	dcHuff     map[int]*dctHuff
	acHuff     map[int]*dctHuff
	restart    int
	adobe      bool
}

func (d *dctDecoder) marker() (int, error) {
	if d.pos+1 >= len(d.data) || d.data[d.pos] != 0xFF {
		return 0, fmt.Errorf("expected marker at %d", d.pos)
	}
	m := int(d.data[d.pos+1])
	d.pos += 2
	return m, nil
}

func (d *dctDecoder) segment() ([]byte, error) {
	if d.pos+2 > len(d.data) {
		return nil, fmt.Errorf("short segment")
	}
	n := int(d.data[d.pos])<<8 | int(d.data[d.pos+1])
	if n < 2 || d.pos+n > len(d.data) {
		return nil, fmt.Errorf("bad segment length")
	}
	s := d.data[d.pos+2 : d.pos+n]
	d.pos += n
	return s, nil
}

func parseDCTJPEG(data []byte) (image.Image, error) {
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil, fmt.Errorf("not a JPEG")
	}
	d := &dctDecoder{data: data, pos: 2, quant: map[int][64]int{}, dcHuff: map[int]*dctHuff{}, acHuff: map[int]*dctHuff{}}
	var scan []dctComponent
	for {
		m, err := d.marker()
		if err != nil {
			return nil, err
		}
		switch m {
		case 0xD9: // EOI
			return nil, fmt.Errorf("no scan")
		case 0xDA: // SOS
			seg, err := d.segment()
			if err != nil {
				return nil, err
			}
			if len(seg) < 1 || len(seg) < 1+2*int(seg[0]) {
				return nil, fmt.Errorf("bad SOS")
			}
			n := int(seg[0])
			scan = make([]dctComponent, 0, n)
			for i := 0; i < n; i++ {
				id := int(seg[1+i*2])
				td := int(seg[2+i*2]) >> 4
				ta := int(seg[2+i*2]) & 0xf
				found := false
				for _, c := range d.components {
					if c.id == id {
						c.dcTable, c.acTable = td, ta
						scan = append(scan, c)
						found = true
						break
					}
				}
				if !found {
					return nil, fmt.Errorf("unknown scan component %d", id)
				}
			}
			// Entropy data starts here and runs to the next marker. The
			// decoder consumes it directly; pos now points at the marker.
			return d.decodeScan(scan)
		case 0xDB: // DQT
			seg, err := d.segment()
			if err != nil {
				return nil, err
			}
			for len(seg) > 0 {
				pq := int(seg[0] >> 4)
				tq := int(seg[0] & 0xf)
				seg = seg[1:]
				var q [64]int
				if pq == 0 {
					if len(seg) < 64 {
						return nil, fmt.Errorf("short DQT")
					}
					for i := 0; i < 64; i++ {
						q[i] = int(seg[i])
					}
					seg = seg[64:]
				} else {
					if len(seg) < 128 {
						return nil, fmt.Errorf("short DQT")
					}
					for i := 0; i < 64; i++ {
						q[i] = int(seg[i*2])<<8 | int(seg[i*2+1])
					}
					seg = seg[128:]
				}
				d.quant[tq] = q
			}
		case 0xC0, 0xC1: // baseline / extended sequential
			seg, err := d.segment()
			if err != nil {
				return nil, err
			}
			if len(seg) < 6 {
				return nil, fmt.Errorf("short SOF")
			}
			if seg[0] != 8 {
				return nil, fmt.Errorf("unsupported precision %d", seg[0])
			}
			d.height = int(seg[1])<<8 | int(seg[2])
			d.width = int(seg[3])<<8 | int(seg[4])
			nc := int(seg[5])
			if nc < 1 || len(seg) < 6+3*nc {
				return nil, fmt.Errorf("bad SOF components")
			}
			for i := 0; i < nc; i++ {
				c := dctComponent{
					id: int(seg[6+i*3]),
					h:  int(seg[7+i*3] >> 4),
					v:  int(seg[7+i*3] & 0xf),
					tq: int(seg[8+i*3]),
				}
				if c.h < 1 || c.v < 1 || c.h > 4 || c.v > 4 {
					return nil, fmt.Errorf("bad sampling")
				}
				d.components = append(d.components, c)
			}
		case 0xC2, 0xC3, 0xC5, 0xC6, 0xC7, 0xC9, 0xCA, 0xCB, 0xCD, 0xCE, 0xCF:
			return nil, fmt.Errorf("progressive/arithmetic JPEG is not supported by the DCT prototype")
		case 0xC4: // DHT
			seg, err := d.segment()
			if err != nil {
				return nil, err
			}
			for len(seg) > 0 {
				if len(seg) < 17 {
					return nil, fmt.Errorf("short DHT")
				}
				tc := int(seg[0] >> 4)
				th := int(seg[0] & 0xf)
				var counts [17]int
				total := 0
				for i := 1; i <= 16; i++ {
					counts[i] = int(seg[i])
					total += counts[i]
				}
				seg = seg[17:]
				if len(seg) < total {
					return nil, fmt.Errorf("short DHT symbols")
				}
				symbols := append([]byte(nil), seg[:total]...)
				seg = seg[total:]
				h := &dctHuff{}
				if err := h.build(counts, symbols); err != nil {
					return nil, err
				}
				if tc == 0 {
					d.dcHuff[th] = h
				} else {
					d.acHuff[th] = h
				}
			}
		case 0xDD: // DRI
			seg, err := d.segment()
			if err != nil || len(seg) < 2 {
				return nil, fmt.Errorf("bad DRI")
			}
			d.restart = int(seg[0])<<8 | int(seg[1])
		case 0xEE: // Adobe APP14
			seg, err := d.segment()
			if err == nil && len(seg) >= 12 {
				d.adobe = true
			}
		case 0xE0, 0xE1, 0xE2, 0xE3, 0xE4, 0xE5, 0xE6, 0xE7, 0xE8, 0xE9, 0xEA, 0xEB, 0xEC, 0xED, 0xEF, 0xFE:
			if _, err := d.segment(); err != nil {
				return nil, err
			}
		case 0xD0, 0xD1, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6, 0xD7:
			// stray restart marker before SOS; ignore
		default:
			if _, err := d.segment(); err != nil {
				return nil, err
			}
		}
	}
}

func (d *dctDecoder) decodeScan(scan []dctComponent) (image.Image, error) {
	if d.width <= 0 || d.height <= 0 || len(d.components) == 0 {
		return nil, fmt.Errorf("missing frame")
	}
	if d.width*d.height > 150_000_000 {
		return nil, fmt.Errorf("image too large")
	}
	hMax, vMax := 1, 1
	for _, c := range d.components {
		if c.h > hMax {
			hMax = c.h
		}
		if c.v > vMax {
			vMax = c.v
		}
	}
	w8 := (d.width + 7) / 8
	h8 := (d.height + 7) / 8
	mcusX := (d.width + 8*hMax - 1) / (8 * hMax)
	mcusY := (d.height + 8*vMax - 1) / (8 * vMax)
	planes := make([]*image.YCbCr, 0)
	var gray *image.Gray
	if len(d.components) == 1 {
		gray = image.NewGray(image.Rect(0, 0, w8, h8))
	} else {
		planes = append(planes, image.NewYCbCr(image.Rect(0, 0, w8, h8), image.YCbCrSubsampleRatio444))
	}

	pred := make([]int, len(d.components))
	br := &dctBits{data: d.data, pos: d.pos}
	mcu := 0
	for my := 0; my < mcusY; my++ {
		for mx := 0; mx < mcusX; mx++ {
			if d.restart > 0 && mcu > 0 && mcu%d.restart == 0 {
				br.reset()
				if br.pos+1 < len(br.data) && br.data[br.pos] == 0xFF && br.data[br.pos+1] >= 0xD0 && br.data[br.pos+1] <= 0xD7 {
					br.pos += 2
				}
				for i := range pred {
					pred[i] = 0
				}
			}
			for ci, c := range scan {
				q, ok := d.quant[c.tq]
				if !ok {
					return nil, fmt.Errorf("missing quant table %d", c.tq)
				}
				for by := 0; by < c.v; by++ {
					for bx := 0; bx < c.h; bx++ {
						dcSym, err := d.dcHuff[c.dcTable].decode(br)
						if err != nil {
							return nil, err
						}
						diff := 0
						if dcSym > 0 {
							v, err := br.bits(dcSym)
							if err != nil {
								return nil, err
							}
							diff = dctExtend(v, dcSym)
						}
						pred[ci] += diff
						// Skip the AC coefficients. We still have to walk
						// them so the bitstream stays aligned.
						k := 1
						for k < 64 {
							rs, err := d.acHuff[c.acTable].decode(br)
							if err != nil {
								return nil, err
							}
							r, s := rs>>4, rs&15
							if s == 0 {
								if r == 15 {
									k += 16
									continue
								}
								break
							}
							if _, err := br.bits(s); err != nil {
								return nil, err
							}
							k += r + 1
						}
						val := pred[ci]*q[0]/8 + 128
						if val < 0 {
							val = 0
						}
						if val > 255 {
							val = 255
						}
						x := mx*c.h + bx
						y := my*c.v + by
						if gray != nil {
							if x < w8 && y < h8 {
								gray.Pix[y*gray.Stride+x] = uint8(val)
							}
							continue
						}
						if ci == 0 {
							if x < w8 && y < h8 {
								planes[0].Y[y*planes[0].YStride+x] = uint8(val)
							}
						} else {
							// Chroma is one block per MCU for 4:2:0/4:2:2.
							// Paint its value over the whole MCU footprint.
							plane := planes[0]
							for yy := my * vMax; yy < (my+1)*vMax && yy < h8; yy++ {
								for xx := mx * hMax; xx < (mx+1)*hMax && xx < w8; xx++ {
									if ci == 1 {
										plane.Cb[yy*plane.CStride+xx] = uint8(val)
									} else {
										plane.Cr[yy*plane.CStride+xx] = uint8(val)
									}
								}
							}
						}
					}
				}
			}
			mcu++
		}
	}
	if gray != nil {
		return gray, nil
	}
	return planes[0], nil
}

func dctThumb(it *Item, size int) ([]byte, error) {
	data, err := osReadFile(it.Path)
	if err != nil {
		return nil, err
	}
	img, err := parseDCTJPEG(data)
	if err != nil {
		return nil, err
	}
	d := imageDims(it.Path, it.Mtime)
	if d.W <= 0 || d.H <= 0 {
		d = dimInfo{W: img.Bounds().Dx() * 8, H: img.Bounds().Dy() * 8}
	}
	dst := thumbnailImage(img, d, size)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 84}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// osReadFile is kept as a variable-free helper so the DCT path can be
// benchmarked without opening a second file descriptor abstraction.
func osReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
