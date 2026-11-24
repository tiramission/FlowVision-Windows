package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func buildMP4Box(typ string, content []byte) []byte {
	b := make([]byte, 8+len(content))
	binary.BigEndian.PutUint32(b[0:4], uint32(len(b)))
	copy(b[4:8], typ)
	copy(b[8:], content)
	return b
}

func mp4FullBox(typ string, payload []byte) []byte {
	return buildMP4Box(typ, append([]byte{0, 0, 0, 0}, payload...))
}

func TestProbeVideoMetaReadsMP4SampleTable(t *testing.T) {
	mvhd := mp4FullBox("mvhd", func() []byte {
		b := make([]byte, 16)
		binary.BigEndian.PutUint32(b[8:12], 1000)
		binary.BigEndian.PutUint32(b[12:16], 2000)
		return b
	}())
	tkhd := mp4FullBox("tkhd", func() []byte {
		// Leave tkhd's display dimensions empty: the parser should fall back to
		// the VisualSampleEntry in stsd.
		return make([]byte, 84)
	}())
	mdhd := mp4FullBox("mdhd", func() []byte {
		b := make([]byte, 16)
		binary.BigEndian.PutUint32(b[8:12], 1000)
		binary.BigEndian.PutUint32(b[12:16], 2000)
		return b
	}())
	hdlr := mp4FullBox("hdlr", func() []byte {
		b := make([]byte, 8)
		copy(b[4:8], "vide")
		return b
	}())
	stsd := mp4FullBox("stsd", func() []byte {
		b := make([]byte, 40)
		binary.BigEndian.PutUint32(b[0:4], 1)
		binary.BigEndian.PutUint32(b[4:8], 40)
		copy(b[8:12], "avc1")
		// The sample entry starts at payload offset 4 (after entry_count). It
		// has an 8-byte box header, an 8-byte SampleEntry prefix and 16 bytes
		// of pre_defined/reserved fields before width and height.
		binary.BigEndian.PutUint16(b[36:38], 640)
		binary.BigEndian.PutUint16(b[38:40], 360)
		return b
	}())
	stts := mp4FullBox("stts", func() []byte {
		b := make([]byte, 12)
		binary.BigEndian.PutUint32(b[0:4], 1)
		binary.BigEndian.PutUint32(b[4:8], 60)
		binary.BigEndian.PutUint32(b[8:12], 33)
		return b
	}())
	stsz := mp4FullBox("stsz", func() []byte {
		b := make([]byte, 12+60*4)
		binary.BigEndian.PutUint32(b[0:4], 0)
		binary.BigEndian.PutUint32(b[4:8], 60)
		for i := 0; i < 60; i++ {
			size := uint32(10000)
			if i >= 30 {
				size = 20000
			}
			binary.BigEndian.PutUint32(b[8+i*4:12+i*4], size)
		}
		return b
	}())
	stbl := buildMP4Box("stbl", concatBytes(stsd, stts, stsz))
	minf := buildMP4Box("minf", stbl)
	mdia := buildMP4Box("mdia", concatBytes(mdhd, hdlr, minf))
	trak := buildMP4Box("trak", concatBytes(tkhd, mdia))
	moov := buildMP4Box("moov", concatBytes(mvhd, trak))

	path := filepath.Join(t.TempDir(), "sample.mp4")
	if err := os.WriteFile(path, concatBytes(buildMP4Box("ftyp", []byte("isom")), moov), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := probeVideoMeta(path)
	if meta.Width != 640 || meta.Height != 360 {
		t.Fatalf("dimensions = %dx%d", meta.Width, meta.Height)
	}
	if meta.FPS != 30 {
		t.Fatalf("fps = %v", meta.FPS)
	}
	if meta.DurationMS != 2000 {
		t.Fatalf("duration = %dms", meta.DurationMS)
	}
	if !meta.LiveBitrate || len(meta.Samples) != 60 {
		t.Fatalf("live bitrate samples = %d, live=%v", len(meta.Samples), meta.LiveBitrate)
	}
	if meta.BitrateKbps != 3600 {
		t.Fatalf("average bitrate = %d", meta.BitrateKbps)
	}
}

func TestBitrateAtMSUsesRollingWindow(t *testing.T) {
	meta := VideoMeta{
		BitrateKbps: 1000,
		LiveBitrate: true,
		Samples: []VideoSample{
			{AtMS: 0, Size: 1000},
			{AtMS: 500, Size: 1000},
			{AtMS: 1000, Size: 1000},
			{AtMS: 1500, Size: 5000},
			{AtMS: 2000, Size: 5000},
		},
	}
	if got := meta.BitrateAtMS(500, time.Second); got != 32 {
		t.Fatalf("early bitrate = %d, want 32", got)
	}
	if got := meta.BitrateAtMS(2000, time.Second); got != 88 {
		t.Fatalf("peak bitrate = %d, want 88", got)
	}
}

func concatBytes(parts ...[]byte) []byte {
	var total int
	for _, p := range parts {
		total += len(p)
	}
	out := make([]byte, 0, total)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}
