package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// KV is one line of metadata in the info panel.
type KV struct {
	K string `json:"k"`
	V string `json:"v"`
}

// probeImageDims reads an image header, honouring EXIF orientation. It opens
// the file once and reuses the handle for the EXIF scan; the old version
// opened the same JPEG twice for every directory item.
func probeImageDims(path string) dimInfo {
	var f io.ReadSeeker
	if isRemotePath(path) {
		// One request per image, not one per question: dimensions and EXIF
		// both come out of the same prefix.
		head := remoteHead(path)
		if len(head) == 0 {
			return dimInfo{}
		}
		f = bytes.NewReader(head)
	} else {
		file, err := os.Open(path)
		if err != nil {
			return dimInfo{}
		}
		defer file.Close()
		f = file
	}
	cfg, _, err := image.DecodeConfig(f)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return dimInfo{}
	}
	d := dimInfo{W: cfg.Width, H: cfg.Height}
	if isJPEG(path) {
		ex := readExifFile(f)
		d.Thumb = len(ex.Thumb) > 0
		if o := ex.Orientation; o > 1 && o < 9 {
			d.Orient = o
			if o >= 5 {
				d.W, d.H = cfg.Height, cfg.Width
			}
		}
	}
	return d
}

func isJPEG(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".jpe", ".jfif":
		return true
	}
	return false
}

// ----------------------------------------------------------------- EXIF parser

type exifData struct {
	Orientation int
	Values      []KV
	Thumb       []byte
}

var exifTags = map[uint16]string{
	0x010F: "相机厂商",
	0x0110: "相机型号",
	0x0112: "方向",
	0x0131: "软件",
	0x0132: "修改时间",
	0x829A: "曝光时间",
	0x829D: "光圈",
	0x8822: "曝光模式",
	0x8827: "ISO",
	0x9003: "拍摄时间",
	0x9201: "快门速度",
	0x9202: "光圈值",
	0x9204: "曝光补偿",
	0x9207: "测光模式",
	0x920A: "焦距",
	0x9291: "秒",
	0xA001: "色彩空间",
	0xA002: "像素宽",
	0xA003: "像素高",
	0xA405: "等效焦距",
	0xA434: "镜头",
}

func readExif(path string) exifData {
	if isRemotePath(path) {
		head := remoteHead(path)
		if len(head) == 0 {
			return exifData{}
		}
		return readExifFile(bytes.NewReader(head))
	}
	f, err := os.Open(path)
	if err != nil {
		return exifData{}
	}
	defer f.Close()
	return readExifFile(f)
}

// readExifFile parses EXIF from an already-open JPEG. It seeks back to the
// beginning because callers such as probeImageDims may have read the image
// header first.
func readExifFile(f io.ReadSeeker) exifData {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return exifData{}
	}
	// EXIF lives in the first APP1 segment in nearly every JPEG. Reading a
	// 128 KiB head keeps directory scans cheap; a larger APP1 is read from
	// its own offset instead of pulling half a megabyte per image.
	buf := make([]byte, 128*1024)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return exifData{}
	}
	buf = buf[:n]
	if len(buf) < 4 || buf[0] != 0xFF || buf[1] != 0xD8 {
		return exifData{}
	}
	off := 2
	for off+4 <= len(buf) {
		for off+1 < len(buf) && buf[off] == 0xFF && buf[off+1] == 0xFF {
			off++
		}
		if off+4 > len(buf) {
			break
		}
		if buf[off] != 0xFF {
			break
		}
		marker := buf[off+1]
		if marker == 0xD8 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			off += 2
			continue
		}
		if marker == 0xDA {
			break
		}
		size := int(binary.BigEndian.Uint16(buf[off+2 : off+4]))
		if size < 2 {
			break
		}
		if marker == 0xE1 {
			segLen := size - 2
			var seg []byte
			if off+4+segLen <= len(buf) {
				seg = buf[off+4 : off+4+segLen]
			} else if segLen <= 1024*1024 {
				// The buffer is a prefix, so a larger APP1 is read from the
				// stream when it can seek and skipped otherwise.
				if seeker, ok := f.(io.ReadSeeker); ok {
					seg = make([]byte, segLen)
					if _, err := seeker.Seek(int64(off+4), io.SeekStart); err != nil {
						return exifData{}
					}
					if _, err := io.ReadFull(seeker, seg); err != nil {
						return exifData{}
					}
				}
			}
			if bytes.HasPrefix(seg, []byte("Exif\x00\x00")) {
				return parseTIFF(seg[6:])
			}
		}
		off += 2 + size
	}
	return exifData{}
}

func parseTIFF(t []byte) exifData {
	if len(t) < 8 {
		return exifData{}
	}
	var bo binary.ByteOrder = binary.BigEndian
	switch {
	case t[0] == 'I' && t[1] == 'I':
		bo = binary.LittleEndian
	case t[0] == 'M' && t[1] == 'M':
		bo = binary.BigEndian
	default:
		return exifData{}
	}
	ifd0 := int(bo.Uint32(t[4:8]))
	vals := map[uint16]string{}
	readIFD(t, bo, ifd0, vals, 0)
	res := exifData{}
	res.Orientation = 1
	if v, ok := vals[0x0112]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			res.Orientation = n
		}
	}
	// IFD1 holds the optional embedded JPEG thumbnail. Camera JPEGs often
	// carry a 160/320px version that is far cheaper to decode than the full
	// frame and is perfect as a first-paint preview.
	if ifd1 := nextIFDOffset(t, bo, ifd0); ifd1 > 0 {
		thumbVals := map[uint16]string{}
		readIFD(t, bo, ifd1, thumbVals, 0)
		if comp, _ := strconv.Atoi(thumbVals[0x0103]); comp == 6 {
			off, err1 := strconv.Atoi(thumbVals[0x0201])
			length, err2 := strconv.Atoi(thumbVals[0x0202])
			if err1 == nil && err2 == nil && off > 0 && length > 0 && length <= 2<<20 &&
				off+length <= len(t) {
				res.Thumb = append([]byte(nil), t[off:off+length]...)
			}
		}
	}
	for _, tag := range orderedExifTags {
		if v, ok := vals[tag]; ok && v != "" && tag != 0x0112 {
			res.Values = append(res.Values, KV{K: exifTags[tag], V: v})
		}
	}
	return res
}

var orderedExifTags = []uint16{
	0x010F, 0x0110, 0xA434, 0x9003, 0x829A, 0x829D, 0x8827, 0x920A, 0xA405,
	0x9204, 0x9207, 0x8822, 0x0131, 0x0132, 0xA001,
}

func readIFD(t []byte, bo binary.ByteOrder, offset int, vals map[uint16]string, depth int) {
	if depth > 2 || offset <= 0 || offset+2 > len(t) {
		return
	}
	n := int(bo.Uint16(t[offset : offset+2]))
	for i := 0; i < n; i++ {
		e := offset + 2 + i*12
		if e+12 > len(t) {
			return
		}
		tag := bo.Uint16(t[e : e+2])
		typ := bo.Uint16(t[e+2 : e+4])
		count := int(bo.Uint32(t[e+4 : e+8]))
		if count <= 0 || count > 4096 {
			continue
		}
		size := typeSize(typ)
		if size == 0 {
			continue
		}
		var raw []byte
		if size*count <= 4 {
			raw = t[e+8 : e+12]
		} else {
			valOff := int(bo.Uint32(t[e+8 : e+12]))
			if valOff < 0 || valOff+size*count > len(t) {
				continue
			}
			raw = t[valOff : valOff+size*count]
		}
		if tag == 0x8769 || tag == 0x8825 {
			// Sub-IFD pointer tags. (Checking the tag, not the value type: the
			// pointer itself is a LONG, so the old check never followed it.)
			if len(raw) >= 4 {
				if sub := int(bo.Uint32(raw[:4])); sub > 0 {
					readIFD(t, bo, sub, vals, depth+1)
				}
			}
			continue
		}
		if s, ok := formatValue(raw, typ, count, bo); ok && s != "" {
			if _, exists := vals[tag]; !exists {
				vals[tag] = s
			}
		}
	}
}

func nextIFDOffset(t []byte, bo binary.ByteOrder, offset int) int {
	if offset <= 0 || offset+2 > len(t) {
		return 0
	}
	n := int(bo.Uint16(t[offset : offset+2]))
	next := offset + 2 + n*12
	if next+4 > len(t) {
		return 0
	}
	return int(bo.Uint32(t[next : next+4]))
}

func typeSize(typ uint16) int {
	switch typ {
	case 1, 2, 6, 7:
		return 1
	case 3, 8:
		return 2
	case 4, 9, 11:
		return 4
	case 5, 10, 12:
		return 8
	}
	return 0
}

func formatValue(raw []byte, typ uint16, count int, bo binary.ByteOrder) (string, bool) {
	switch typ {
	case 2:
		s := string(raw)
		s = strings.TrimRight(s, "\x00")
		s = strings.ReplaceAll(s, "\x00", " ")
		return strings.TrimSpace(s), true
	case 1, 7:
		if count == 1 {
			return strconv.Itoa(int(raw[0])), true
		}
		return "", false
	case 3:
		if count >= 1 && len(raw) >= 2 {
			return strconv.Itoa(int(bo.Uint16(raw[:2]))), true
		}
	case 4:
		if len(raw) >= 4 {
			return strconv.FormatUint(uint64(bo.Uint32(raw[:4])), 10), true
		}
	case 5:
		if len(raw) >= 8 {
			num := bo.Uint32(raw[:4])
			den := bo.Uint32(raw[4:8])
			if den == 0 {
				return "", false
			}
			return formatRational(num, den, count), true
		}
	case 9:
		if len(raw) >= 4 {
			return strconv.Itoa(int(int32(bo.Uint32(raw[:4])))), true
		}
	case 10:
		if len(raw) >= 8 {
			num := int32(bo.Uint32(raw[:4]))
			den := int32(bo.Uint32(raw[4:8]))
			if den == 0 {
				return "", false
			}
			return formatSRational(num, den, count), true
		}
	}
	return "", false
}

func formatRational(num, den uint32, count int) string {
	if den == 0 {
		if count == 1 && num != 0 {
			return strconv.FormatUint(uint64(num), 10)
		}
		return ""
	}
	v := float64(num) / float64(den)
	switch {
	case v == 0:
		return "0"
	case v < 0.5:
		return "1/" + strconv.FormatFloat(1/v, 'f', 0, 64)
	case v < 10:
		return strconv.FormatFloat(v, 'f', 1, 64)
	default:
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
}

// formatSRational prints a signed rational. Exposure compensation and APEX
// values are signed; formatting them as unsigned produced huge nonsense.
func formatSRational(num, den int32, count int) string {
	if den == 0 {
		return ""
	}
	v := float64(num) / float64(den)
	if v == 0 {
		return "0"
	}
	an, ad := num, den
	if an < 0 {
		an = -an
	}
	if ad < 0 {
		ad = -ad
	}
	if an > 0 && an < ad && an <= 10 {
		return strconv.FormatInt(int64(num), 10) + "/" + strconv.FormatInt(int64(den), 10)
	}
	av := v
	if av < 0 {
		av = -av
	}
	if av < 10 {
		return strconv.FormatFloat(v, 'f', 1, 64)
	}
	return strconv.FormatFloat(v, 'f', 0, 64)
}

// ------------------------------------------------------------------- service

// MediaInfo is the metadata panel's content.
type MediaInfo struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Kind    string    `json:"kind"`
	Ext     string    `json:"ext"`
	Size    int64     `json:"size"`
	Mtime   int64     `json:"mtime"`
	Width   int       `json:"width"`
	Height  int       `json:"height"`
	Video   VideoMeta `json:"-"`
	Exif    []KV      `json:"exif"`
	Message string    `json:"message,omitempty"`
}

// MediaSvc serves metadata about one item.
type MediaSvc struct{}

// Info returns size, dimensions and EXIF for id.
func (MediaSvc) Info(id string) MediaInfo {
	it, ok := items.Get(id)
	if !ok {
		return MediaInfo{Message: "unknown item"}
	}
	out := MediaInfo{
		ID: it.ID, Name: it.Name, Path: it.Path, Kind: it.Kind, Ext: it.Ext,
		Size: it.Size, Mtime: it.Mtime, Width: it.W, Height: it.H,
	}
	if it.Kind == "image" {
		d := imageDims(it.Path, it.Mtime)
		out.Width, out.Height = d.W, d.H
		if isJPEG(it.Path) {
			out.Exif = readExif(it.Path).Values
		}
	}
	if it.Kind == "video" {
		out.Video = probeVideoMeta(it.Path)
		if out.Video.Width > 0 && out.Video.Height > 0 {
			out.Width, out.Height = out.Video.Width, out.Video.Height
		}
	}
	return out
}

// FormatSize renders bytes the way the status bar shows them.
func (MediaSvc) FormatSize(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	div := int64(unit)
	exp := 0
	for v := n / unit; v >= unit && exp < 5; v /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB", "PB", "EB"}
	return strconv.FormatFloat(float64(n)/float64(div), 'f', 1, 64) + " " + units[exp]
}
