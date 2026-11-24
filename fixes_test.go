package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newJPEG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 255), uint8(y % 255), 80, 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
}

func TestBrowseLimitExact(t *testing.T) {
	dir := t.TempDir()
	for i := 1; i <= 5; i++ {
		writePNG(t, filepath.Join(dir, itoa(i)+".png"), 40+i, 30+i)
	}
	res := (FS{}).Browse(BrowseRequest{Path: dir, Limit: 3})
	if res.Total != 3 || !res.Truncated || res.Images != 3 {
		t.Fatalf("limit 3: total=%d truncated=%v images=%d", res.Total, res.Truncated, res.Images)
	}
	res = (FS{}).Browse(BrowseRequest{Path: dir, Limit: 5})
	if res.Total != 5 || res.Truncated {
		t.Fatalf("limit 5: total=%d truncated=%v (exact fit must not be truncated)", res.Total, res.Truncated)
	}
	res = (FS{}).Browse(BrowseRequest{Path: dir, Limit: 9})
	if res.Total != 5 || res.Truncated {
		t.Fatalf("limit 9: total=%d truncated=%v", res.Total, res.Truncated)
	}
}

func itoa(n int) string { return string(rune('0' + n)) }

func TestBrowseRecursiveTruncated(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(dir, "a.png"), 10, 10)
	writePNG(t, filepath.Join(dir, "b.png"), 10, 10)
	writePNG(t, filepath.Join(sub, "c.png"), 10, 10)
	res := (FS{}).Browse(BrowseRequest{Path: dir, Recursive: true, Limit: 2})
	if res.Total != 2 || !res.Truncated {
		t.Fatalf("recursive limit: total=%d truncated=%v", res.Total, res.Truncated)
	}
}

func TestParentOfDriveRoot(t *testing.T) {
	res := (FS{}).Parent(`C:\`)
	if res.Found {
		t.Fatalf("drive root reported a parent: %+v", res)
	}
	res = (FS{}).Parent(`C:\Users`)
	if !res.Found || res.Path != `C:\` {
		t.Fatalf("parent of C:\\Users = %+v", res)
	}
}

func TestSiblingSkipsEmptyFolders(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a", "b", "c", "d"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writePNG(t, filepath.Join(root, "a", "1.png"), 20, 20)
	writePNG(t, filepath.Join(root, "c", "2.png"), 20, 20)
	if err := os.Mkdir(filepath.Join(root, ".hidden-media"), 0o755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(root, ".hidden-media", "3.png"), 20, 20)
	a := filepath.Join(root, "a")
	c := filepath.Join(root, "c")
	if got := (FS{}).Sibling(a, 1, false); !got.Found || !strings.EqualFold(got.Path, c) {
		t.Fatalf("next of a = %+v, want c", got)
	}
	if got := (FS{}).Sibling(c, -1, false); !got.Found || !strings.EqualFold(got.Path, a) {
		t.Fatalf("prev of c = %+v, want a", got)
	}
	if got := (FS{}).Sibling(c, 1, false); got.Found {
		t.Fatalf("next of c = %+v, want none", got)
	}
	if got := (FS{}).Sibling(a, -1, false); got.Found {
		t.Fatalf("prev of a must skip the dot folder, got %+v", got)
	}
	if got := (FS{}).Sibling(a, -1, true); !got.Found || !strings.EqualFold(filepath.Base(got.Path), ".hidden-media") {
		t.Fatalf("prev of a with hidden shown = %+v", got)
	}
}

func TestTypeSort(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "z.png"), 10, 10)
	newJPEG(t, filepath.Join(dir, "a.jpg"), 10, 10)
	if err := os.WriteFile(filepath.Join(dir, "m.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := (FS{}).Browse(BrowseRequest{Path: dir, Sort: "type"})
	var kinds []string
	for _, it := range res.Items {
		kinds = append(kinds, it.Kind+":"+it.Ext)
	}
	want := []string{"image:.jpg", "image:.png", "video:.mp4"}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("type sort = %v, want %v", kinds, want)
	}
}

func TestParseSettingsPartialKeepsDefaults(t *testing.T) {
	s, ok := parseSettings([]byte(`{"thumbSize": 300}`))
	if !ok {
		t.Fatal("parseSettings rejected valid JSON")
	}
	if s.ThumbSize != 300 || !s.Sidebar || !s.InfoPanel || !s.PlayVideo || s.Sort != "name" {
		t.Fatalf("partial settings lost defaults: %+v", s)
	}
	if _, ok := parseSettings([]byte(`not json`)); ok {
		t.Fatal("parseSettings accepted broken JSON")
	}
}

func TestStoreEvictsOldItems(t *testing.T) {
	st := &Store{byID: map[string]*Item{}, max: 3}
	for i := 0; i < 6; i++ {
		st.Intern(filepath.Join(t.TempDir(), "x"+itoa(i)+".jpg"))
	}
	if len(st.byID) > 4 {
		t.Fatalf("store grew past cap: %d", len(st.byID))
	}
}

func TestEXIFOrientationSwapsDimensions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oriented.jpg")
	newJPEG(t, path, 80, 40)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Build a little-endian TIFF IFD with one entry: Orientation = 6.
	tiff := new(bytes.Buffer)
	tiff.WriteString("II")
	_ = binary.Write(tiff, binary.LittleEndian, uint16(42))
	_ = binary.Write(tiff, binary.LittleEndian, uint32(8))
	_ = binary.Write(tiff, binary.LittleEndian, uint16(1))
	_ = binary.Write(tiff, binary.LittleEndian, uint16(0x0112))
	_ = binary.Write(tiff, binary.LittleEndian, uint16(3))
	_ = binary.Write(tiff, binary.LittleEndian, uint32(1))
	_ = binary.Write(tiff, binary.LittleEndian, uint16(6))
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
	st, _ := os.Stat(path)
	d := imageDims(path, st.ModTime().Unix())
	if d.W != 40 || d.H != 80 || d.Orient != 6 {
		t.Fatalf("oriented dims = %+v, want 40x80 orient 6", d)
	}
}

func TestEXIFSubIFD(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub.jpg")
	newJPEG(t, path, 80, 40)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// IFD0: one ExifIFD pointer. SubIFD: ISO 400 and focal length 50/1.
	tiff := new(bytes.Buffer)
	tiff.WriteString("II")
	_ = binary.Write(tiff, binary.LittleEndian, uint16(42))
	_ = binary.Write(tiff, binary.LittleEndian, uint32(8))
	_ = binary.Write(tiff, binary.LittleEndian, uint16(1))
	_ = binary.Write(tiff, binary.LittleEndian, uint16(0x8769))
	_ = binary.Write(tiff, binary.LittleEndian, uint16(4))
	_ = binary.Write(tiff, binary.LittleEndian, uint32(1))
	_ = binary.Write(tiff, binary.LittleEndian, uint32(26))
	_ = binary.Write(tiff, binary.LittleEndian, uint32(0))
	// SubIFD at offset 26.
	_ = binary.Write(tiff, binary.LittleEndian, uint16(2))
	_ = binary.Write(tiff, binary.LittleEndian, uint16(0x8827))
	_ = binary.Write(tiff, binary.LittleEndian, uint16(3))
	_ = binary.Write(tiff, binary.LittleEndian, uint32(1))
	_ = binary.Write(tiff, binary.LittleEndian, uint16(400))
	_ = binary.Write(tiff, binary.LittleEndian, uint16(0))
	_ = binary.Write(tiff, binary.LittleEndian, uint16(0x920A))
	_ = binary.Write(tiff, binary.LittleEndian, uint16(5))
	_ = binary.Write(tiff, binary.LittleEndian, uint32(1))
	_ = binary.Write(tiff, binary.LittleEndian, uint32(56))
	_ = binary.Write(tiff, binary.LittleEndian, uint32(0))
	_ = binary.Write(tiff, binary.LittleEndian, uint32(50))
	_ = binary.Write(tiff, binary.LittleEndian, uint32(1))
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
	exif := readExif(path)
	got := map[string]string{}
	for _, kv := range exif.Values {
		got[kv.K] = kv.V
	}
	if got["ISO"] != "400" {
		t.Fatalf("ISO = %q, want 400 (values %+v)", got["ISO"], exif.Values)
	}
	if got["焦距"] != "50" {
		t.Fatalf("focal length = %q, want 50 (values %+v)", got["焦距"], exif.Values)
	}
}

func TestConcurrentThumbsAndRange(t *testing.T) {
	dir := t.TempDir()
	thumbDir = t.TempDir()
	thumbSlots = make(chan struct{}, 4)
	path := filepath.Join(dir, "big.png")
	writePNG(t, path, 600, 300)
	it := items.Intern(path)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := getThumb(context.Background(), it, 240); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent thumbnail failed: %v", err)
	}
	if _, err := os.Stat(thumbPath(it, 240)); err != nil {
		t.Fatalf("thumbnail missing: %v", err)
	}

	req := httptest.NewRequest("GET", "/o/"+it.ID, nil)
	req.Header.Set("Range", "bytes=0-3")
	rec := httptest.NewRecorder()
	serveItemContent(rec, req, it)
	if rec.Code != 206 {
		t.Fatalf("range status = %d, want 206", rec.Code)
	}
	if got := rec.Body.Len(); got != 4 {
		t.Fatalf("range body = %d bytes, want 4", got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content type = %q", ct)
	}
}

func TestDotFilesFollowShowHidden(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "normal.png"), 10, 10)
	writePNG(t, filepath.Join(dir, ".secret.png"), 10, 10)
	res := (FS{}).Browse(BrowseRequest{Path: dir})
	if res.Total != 1 || res.Images != 1 {
		t.Fatalf("hidden dot file shown by default: total=%d images=%d", res.Total, res.Images)
	}
	res = (FS{}).Browse(BrowseRequest{Path: dir, ShowHidden: true})
	if res.Total != 2 {
		t.Fatalf("showHidden did not reveal dot file: total=%d", res.Total)
	}
}

func TestRenameCaseOnly(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "Photo.jpg")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := AppSvc{}
	if got, err := svc.Rename(src, "Photo.jpg"); err != nil || got != src {
		t.Fatalf("same-name rename = %q, %v", got, err)
	}
	got, err := svc.Rename(src, "photo.JPG")
	if err != nil {
		t.Fatalf("case-only rename: %v", err)
	}
	if filepath.Base(got) != "photo.JPG" {
		t.Fatalf("case-only rename produced %q, want photo.JPG", filepath.Base(got))
	}
	if st, err := os.Stat(got); err != nil || st.Size() != 1 {
		t.Fatalf("renamed file missing: %v", err)
	}
}

func TestResolveDrop(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "photo.jpg")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := AppSvc{}
	if got, err := svc.ResolveDrop(dir); err != nil || !strings.EqualFold(got, dir) {
		t.Fatalf("folder drop = %q, %v", got, err)
	}
	if got, err := svc.ResolveDrop(file); err != nil || !strings.EqualFold(got, dir) {
		t.Fatalf("file drop = %q, %v", got, err)
	}
	if _, err := svc.ResolveDrop(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing drop accepted")
	}
}

func TestInvalidWindowsNamesRejected(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := AppSvc{}
	for _, name := range []string{"", ".", "..", "CON", "con.txt", "NUL.log", "COM1", "foo.", "a<b", "x\ty", "bad:name"} {
		if _, err := svc.Rename(src, name); err == nil {
			t.Fatalf("Rename accepted invalid name %q", name)
		}
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("invalid rename removed the source: %v", err)
	}
	for _, name := range []string{"", ".", "..", "PRN", "aux.txt", "foo.", "a|b"} {
		if _, err := svc.NewFolder(dir, name); err == nil {
			t.Fatalf("NewFolder accepted invalid name %q", name)
		}
	}
	got, err := svc.NewFolder(dir, "照片 001")
	if err != nil {
		t.Fatalf("NewFolder rejected a valid name: %v", err)
	}
	if st, err := os.Stat(got); err != nil || !st.IsDir() {
		t.Fatalf("valid folder missing: %v", err)
	}
}
