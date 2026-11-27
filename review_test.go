package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowseEmptyPath(t *testing.T) {
	res := (FS{}).Browse(BrowseRequest{Path: "   "})
	if res.Error == "" || res.Total != 0 {
		t.Fatalf("blank path browse = %+v, want an error and no items", res)
	}
}

func TestNaturalSortHugeNumbers(t *testing.T) {
	if !naturalLess("img2.jpg", "img10.jpg") {
		t.Fatal("2 should sort before 10")
	}
	a := "img999999999999999999999.jpg"
	b := "img1000000000000000000000.jpg"
	if !naturalLess(a, b) {
		t.Fatal("a 21-digit number should sort before a 22-digit one")
	}
	if naturalLess("IMG02.jpg", "img2.jpg") || naturalLess("img2.jpg", "IMG02.jpg") {
		t.Fatal("zero-padded equal numbers must compare equal")
	}
}

func TestStoreSnapshotsAndCaseOnlyUpdate(t *testing.T) {
	dir := t.TempDir()
	name := "Photo.JPG"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := &Store{byID: map[string]*Item{}}
	a := st.Intern(path)
	// The page only ever receives copies: mutating one must not mutate the
	// store's snapshot that concurrent media requests read.
	a.W = 123
	if got, _ := st.Get(a.ID); got.W != 0 {
		t.Fatalf("store snapshot was mutated through a returned item: W=%d", got.W)
	}

	upper := filepath.Join(dir, strings.ToUpper(name))
	b := st.Intern(upper)
	if b.ID != a.ID {
		t.Fatalf("case-only path got a different id: %s vs %s", a.ID, b.ID)
	}
	if b.Path != upper || b.Name != strings.ToUpper(name) {
		t.Fatalf("case-only update kept stale path/name: %+v", b)
	}
	if b.W != 0 || b.H != 0 {
		t.Fatalf("case-only update kept stale dimensions: %+v", b)
	}
	if got, _ := st.Get(b.ID); got.Path != upper {
		t.Fatalf("store still has stale path after case-only update: %+v", got)
	}
}

func TestFormatSizeLargeUnits(t *testing.T) {
	svc := MediaSvc{}
	cases := map[int64]string{
		1 << 20: "1.0 MB",
		1 << 30: "1.0 GB",
		1 << 40: "1.0 TB",
		1 << 50: "1.0 PB",
		1 << 60: "1.0 EB",
	}
	for n, want := range cases {
		if got := svc.FormatSize(n); got != want {
			t.Errorf("FormatSize(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestCheckNameUTF16Limit(t *testing.T) {
	// 128 emoji are 128 runes but 256 UTF-16 code units; Windows counts the
	// latter, so this name must be rejected rather than failing at the API.
	if err := checkName(strings.Repeat("😀", 128)); err == nil {
		t.Fatal("128 emoji should exceed the 255 UTF-16 unit limit")
	}
	if err := checkName(strings.Repeat("😀", 127)); err != nil {
		t.Fatalf("127 emoji (254 units) should be accepted: %v", err)
	}
	if err := checkName("COM¹.txt"); err == nil {
		t.Fatal("COM¹.txt is reserved on Windows")
	}
}

func TestResolveDropLeadingSpace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, " leading space.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := AppSvc{}
	got, err := svc.ResolveDrop(path)
	if err != nil {
		t.Fatalf("exact path with a leading space failed: %v", err)
	}
	if got != dir {
		t.Fatalf("ResolveDrop = %q, want %q", got, dir)
	}
	normal := filepath.Join(dir, "normal.txt")
	if err := os.WriteFile(normal, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = svc.ResolveDrop("  " + normal + "  ")
	if err != nil || got != dir {
		t.Fatalf("whitespace-wrapped path = %q, %v; want %q", got, err, dir)
	}
}

func TestSRationalAndExifOrientationPreviewScale(t *testing.T) {
	if got := formatSRational(-5, 2, 1); got != "-2.5" {
		t.Fatalf("formatSRational(-5,2) = %q, want -2.5", got)
	}
	if got := formatSRational(3, 2, 1); got != "1.5" {
		t.Fatalf("formatSRational(3,2) = %q, want 1.5", got)
	}

	path := filepath.Join(t.TempDir(), "wide.jpg")
	writeOrientedJPEG(t, path, 5000, 1000, 6)
	thumbDir = t.TempDir()
	previewDir = thumbDir
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	it := &Item{ID: "preview-orient", Path: path, Kind: "image", Ext: ".jpg", Size: st.Size(), Mtime: st.ModTime().Unix()}
	data, err := previewBytes(it)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	b := cfg.Bounds()
	// Orientation 6 rotates 5000x1000 to 1000x5000, then the 2560 cap
	// scales it to 512x2560. A pre-rotation scale would have produced the
	// reverse shape and an oversized image.
	if b.Dx() != 512 || b.Dy() != 2560 {
		t.Fatalf("oriented preview = %dx%d, want 512x2560", b.Dx(), b.Dy())
	}
}

func writeOrientedJPEG(t *testing.T, path string, w, h int, orientation uint16) {
	t.Helper()
	newJPEG(t, path, w, h)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tiff := new(bytes.Buffer)
	tiff.WriteString("II")
	_ = binary.Write(tiff, binary.LittleEndian, uint16(42))
	_ = binary.Write(tiff, binary.LittleEndian, uint32(8))
	_ = binary.Write(tiff, binary.LittleEndian, uint16(1))
	_ = binary.Write(tiff, binary.LittleEndian, uint16(0x0112))
	_ = binary.Write(tiff, binary.LittleEndian, uint16(3))
	_ = binary.Write(tiff, binary.LittleEndian, uint32(1))
	_ = binary.Write(tiff, binary.LittleEndian, orientation)
	_ = binary.Write(tiff, binary.LittleEndian, uint16(0))
	_ = binary.Write(tiff, binary.LittleEndian, uint32(0))
	payload := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	app1 := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(app1[2:], uint16(len(payload)+2))
	app1 = append(app1, payload...)
	out := append([]byte{}, data[:2]...)
	out = append(out, app1...)
	out = append(out, data[2:]...)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}
