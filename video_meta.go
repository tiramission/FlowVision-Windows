package main

import (
	"encoding/binary"
	"errors"
	"math"
	"os"
	"sort"
	"time"
)

// VideoMeta is the subset of container metadata the viewer can show without
// depending on an external ffprobe. MP4/MOV sample tables also let the info
// panel report the encoded video bitrate around the current playhead, not just
// a file-wide average.
type VideoMeta struct {
	Width       int
	Height      int
	DurationMS  int64
	FPS         float64
	BitrateKbps int
	LiveBitrate bool
	Samples     []VideoSample
}

// VideoSample is one encoded video sample's presentation time and byte size.
type VideoSample struct {
	AtMS int64
	Size int64
}

// BitrateAtMS returns the video track's encoded bitrate in a rolling window
// ending at ms. It is the bitrate the viewer is actually consuming at that
// point in the timeline, including VBR peaks, rather than fileSize/duration.
func (m VideoMeta) BitrateAtMS(ms int64, window time.Duration) int {
	if !m.LiveBitrate || len(m.Samples) == 0 {
		return m.BitrateKbps
	}
	if ms < 0 {
		ms = 0
	}
	windowMS := window.Milliseconds()
	if windowMS < 250 {
		windowMS = 250
	}
	start := ms - windowMS
	if start < 0 {
		start = 0
	}
	first := sort.Search(len(m.Samples), func(i int) bool {
		return m.Samples[i].AtMS >= start
	})
	var bytes int64
	for i := first; i < len(m.Samples) && m.Samples[i].AtMS <= ms; i++ {
		bytes += m.Samples[i].Size
	}
	if bytes == 0 {
		return m.BitrateKbps
	}
	seconds := float64(ms-start) / 1000
	if seconds < 0.25 {
		seconds = 0.25
	}
	return int(math.Round(float64(bytes) * 8 / seconds / 1000))
}

type mp4Box struct {
	typ   string
	start int
	end   int
}

type mp4Track struct {
	width, height  int
	trackTimescale int64
	trackDuration  int64
	handler        string
	sampleSizes    []int64
	sampleDeltas   []uint32
	deltaCounts    []uint32
	avgBitrate     int
}

func probeVideoMeta(path string) VideoMeta {
	file, err := os.Open(path)
	if err != nil {
		return VideoMeta{}
	}
	defer file.Close()
	st, err := file.Stat()
	if err != nil {
		return VideoMeta{}
	}
	moov, err := readMoov(file, st.Size())
	if err != nil {
		return fallbackVideoMeta(st.Size())
	}
	movieScale, movieDuration := parseMovieHeader(moov)
	var tracks []mp4Track
	for _, box := range childBoxes(moov) {
		if box.typ != "trak" {
			continue
		}
		content := moov[box.start:box.end]
		tr := parseTrack(content, movieScale)
		tracks = append(tracks, tr)
	}
	best := -1
	for i, tr := range tracks {
		if tr.handler != "vide" || tr.width <= 0 || tr.height <= 0 {
			continue
		}
		if best < 0 {
			best = i
			continue
		}
		cur := tracks[best]
		if tr.width*tr.height > cur.width*cur.height || (tr.width*tr.height == cur.width*cur.height && len(tr.sampleSizes) > len(cur.sampleSizes)) {
			best = i
		}
	}
	if best < 0 {
		return fallbackVideoMeta(st.Size())
	}
	tr := tracks[best]
	duration := tr.trackDuration
	if duration <= 0 {
		duration = movieDuration
	}
	var durationMS int64
	if tr.trackTimescale > 0 && duration > 0 {
		durationMS = duration * 1000 / tr.trackTimescale
	}
	if durationMS <= 0 && movieScale > 0 && movieDuration > 0 {
		durationMS = movieDuration * 1000 / movieScale
	}
	meta := VideoMeta{
		Width:      tr.width,
		Height:     tr.height,
		DurationMS: durationMS,
	}
	second := float64(durationMS) / 1000
	count := len(tr.sampleSizes)
	if count > 0 && second > 0 {
		meta.FPS = math.Round(float64(count)/second*10) / 10
		var total int64
		for _, n := range tr.sampleSizes {
			total += n
		}
		meta.BitrateKbps = int(math.Round(float64(total) * 8 / second / 1000))
	} else if tr.avgBitrate > 0 {
		meta.BitrateKbps = tr.avgBitrate
	}
	if len(tr.sampleSizes) > 0 && len(tr.deltaCounts) > 0 && tr.trackTimescale > 0 {
		buildSamples(&meta, tr)
	}
	if meta.BitrateKbps <= 0 && durationMS > 0 && second > 0 {
		meta.BitrateKbps = int(math.Round(float64(st.Size()) * 8 / second / 1000))
	}
	return meta
}

// fallbackVideoMeta is used for containers whose sample tables we do not parse.
// Resolution still comes from the player; FPS and live bitrate stay unknown.
func fallbackVideoMeta(_ int64) VideoMeta {
	return VideoMeta{}
}

func readMoov(file *os.File, size int64) ([]byte, error) {
	const maxMoov = 96 << 20
	for off := int64(0); off+8 <= size; {
		var hdr [16]byte
		n, err := file.ReadAt(hdr[:8], off)
		if err != nil || n < 8 {
			return nil, err
		}
		boxSize := int64(binary.BigEndian.Uint32(hdr[0:4]))
		typ := string(hdr[4:8])
		header := int64(8)
		if boxSize == 1 {
			if n, err = file.ReadAt(hdr[:16], off); err != nil || n < 16 {
				return nil, errors.New("invalid MP4 box")
			}
			boxSize = int64(binary.BigEndian.Uint64(hdr[8:16]))
			header = 16
		}
		if boxSize == 0 {
			boxSize = size - off
		}
		if boxSize < header || off+boxSize > size {
			return nil, errors.New("invalid MP4 box size")
		}
		if typ == "moov" {
			if boxSize-header > maxMoov {
				return nil, errors.New("moov too large")
			}
			data := make([]byte, boxSize-header)
			if _, err := file.ReadAt(data, off+header); err != nil {
				return nil, err
			}
			return data, nil
		}
		off += boxSize
	}
	return nil, errors.New("no moov box")
}

func childBoxes(data []byte) []mp4Box {
	var out []mp4Box
	for off := 0; off+8 <= len(data); {
		boxSize := int(binary.BigEndian.Uint32(data[off : off+4]))
		typ := string(data[off+4 : off+8])
		header := 8
		if boxSize == 1 {
			if off+16 > len(data) {
				break
			}
			boxSize = int(binary.BigEndian.Uint64(data[off+8 : off+16]))
			header = 16
		} else if boxSize == 0 {
			boxSize = len(data) - off
		}
		if boxSize < header || off+boxSize > len(data) {
			break
		}
		out = append(out, mp4Box{typ: typ, start: off + header, end: off + boxSize})
		off += boxSize
	}
	return out
}

func findBox(data []byte, typ string) (mp4Box, bool) {
	for _, box := range childBoxes(data) {
		if box.typ == typ {
			return box, true
		}
	}
	return mp4Box{}, false
}

func parseMovieHeader(data []byte) (int64, int64) {
	box, ok := findBox(data, "mvhd")
	if !ok || box.end-box.start < 20 {
		return 0, 0
	}
	b := data[box.start:box.end]
	version := b[0]
	if version == 1 {
		if len(b) < 32 {
			return 0, 0
		}
		return int64(binary.BigEndian.Uint32(b[20:24])), int64(binary.BigEndian.Uint64(b[24:32]))
	}
	if len(b) < 20 {
		return 0, 0
	}
	return int64(binary.BigEndian.Uint32(b[12:16])), int64(binary.BigEndian.Uint32(b[16:20]))
}

func parseTrack(trak []byte, movieScale int64) mp4Track {
	tr := mp4Track{trackTimescale: movieScale}
	if tkhd, ok := findBox(trak, "tkhd"); ok {
		b := trak[tkhd.start:tkhd.end]
		if len(b) >= 4 {
			off := 76
			if b[0] == 1 {
				off = 88
			}
			if len(b) >= off+8 {
				tr.width = int(binary.BigEndian.Uint32(b[off:off+4]) >> 16)
				tr.height = int(binary.BigEndian.Uint32(b[off+4:off+8]) >> 16)
			}
		}
	}
	mdia, ok := findBox(trak, "mdia")
	if !ok {
		return tr
	}
	mdiaData := trak[mdia.start:mdia.end]
	if mdhd, ok := findBox(mdiaData, "mdhd"); ok {
		b := mdiaData[mdhd.start:mdhd.end]
		if len(b) >= 20 {
			if b[0] == 1 {
				if len(b) >= 32 {
					tr.trackTimescale = int64(binary.BigEndian.Uint32(b[20:24]))
					tr.trackDuration = int64(binary.BigEndian.Uint64(b[24:32]))
				}
			} else {
				tr.trackTimescale = int64(binary.BigEndian.Uint32(b[12:16]))
				tr.trackDuration = int64(binary.BigEndian.Uint32(b[16:20]))
			}
		}
	}
	if hdlr, ok := findBox(mdiaData, "hdlr"); ok {
		b := mdiaData[hdlr.start:hdlr.end]
		if len(b) >= 12 {
			tr.handler = string(b[8:12])
		}
	}
	minf, ok := findBox(mdiaData, "minf")
	if !ok {
		return tr
	}
	stbl, ok := findBox(mdiaData[minf.start:minf.end], "stbl")
	if !ok {
		return tr
	}
	stblData := mdiaData[minf.start+stbl.start : minf.start+stbl.end]
	if stsd, ok := findBox(stblData, "stsd"); ok {
		b := stblData[stsd.start:stsd.end]
		if len(b) >= 44 {
			// b starts at the full-box version/flags and entry_count. The first
			// VisualSampleEntry then has its own 8-byte box header, an 8-byte
			// SampleEntry prefix and 16 bytes of pre_defined/reserved fields.
			entry := 8
			if len(b) >= entry+36 {
				tr.width = int(binary.BigEndian.Uint16(b[entry+32 : entry+34]))
				tr.height = int(binary.BigEndian.Uint16(b[entry+34 : entry+36]))
			}
		}
	}
	if stts, ok := findBox(stblData, "stts"); ok {
		b := stblData[stts.start:stts.end]
		if len(b) >= 8 {
			n := int(binary.BigEndian.Uint32(b[4:8]))
			if n > 0 && n < 1_000_000 {
				tr.deltaCounts = make([]uint32, 0, n)
				tr.sampleDeltas = make([]uint32, 0, n)
				off := 8
				for i := 0; i < n && off+8 <= len(b); i++ {
					tr.deltaCounts = append(tr.deltaCounts, binary.BigEndian.Uint32(b[off:off+4]))
					tr.sampleDeltas = append(tr.sampleDeltas, binary.BigEndian.Uint32(b[off+4:off+8]))
					off += 8
				}
			}
		}
	}
	if stsz, ok := findBox(stblData, "stsz"); ok {
		b := stblData[stsz.start:stsz.end]
		if len(b) >= 12 {
			uniform := binary.BigEndian.Uint32(b[4:8])
			n := int(binary.BigEndian.Uint32(b[8:12]))
			if n > 0 && n < 2_000_000 {
				tr.sampleSizes = make([]int64, 0, n)
				if uniform != 0 {
					for i := 0; i < n; i++ {
						tr.sampleSizes = append(tr.sampleSizes, int64(uniform))
					}
				} else {
					off := 12
					for i := 0; i < n && off+4 <= len(b); i++ {
						tr.sampleSizes = append(tr.sampleSizes, int64(binary.BigEndian.Uint32(b[off:off+4])))
						off += 4
					}
				}
			}
		}
	}
	if tr.trackTimescale > 0 && tr.trackDuration > 0 && len(tr.sampleSizes) > 0 {
		var total int64
		for _, n := range tr.sampleSizes {
			total += n
		}
		seconds := float64(tr.trackDuration) / float64(tr.trackTimescale)
		if seconds > 0 {
			tr.avgBitrate = int(math.Round(float64(total) * 8 / seconds / 1000))
		}
	}
	return tr
}

func buildSamples(meta *VideoMeta, tr mp4Track) {
	nsamples := len(tr.sampleSizes)
	var at int64
	acc := make([]VideoSample, 0, nsamples)
	for i := 0; i < len(tr.deltaCounts) && len(acc) < nsamples; i++ {
		delta := tr.sampleDeltas[i]
		for j := uint32(0); j < tr.deltaCounts[i] && len(acc) < nsamples; j++ {
			acc = append(acc, VideoSample{AtMS: at * 1000 / max(1, tr.trackTimescale), Size: tr.sampleSizes[len(acc)]})
			at += int64(delta)
		}
	}
	if len(acc) == 0 {
		// No reliable stts: spread samples evenly across the duration so a
		// current-position readout is still useful.
		duration := meta.DurationMS
		for i, size := range tr.sampleSizes {
			atMS := int64(0)
			if nsamples > 1 {
				atMS = int64(float64(duration) * float64(i) / float64(nsamples-1))
			}
			acc = append(acc, VideoSample{AtMS: atMS, Size: size})
		}
	}
	if len(acc) >= 2 {
		meta.Samples = acc
		meta.LiveBitrate = true
	}
}
