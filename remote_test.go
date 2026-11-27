package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestIsRemotePath(t *testing.T) {
	for _, p := range []string{
		"smb://nas/photos",
		"smb://nas/photos/2024/a.jpg",
		"webdav://nas.example:5006/dav/photos",
	} {
		if !isRemotePath(p) {
			t.Fatalf("%s should be remote", p)
		}
	}
	for _, p := range []string{`C:\Pictures`, `\\nas\photos`, "photos", "", "smb:/nas"} {
		if isRemotePath(p) {
			t.Fatalf("%s should not be remote", p)
		}
	}
}

func TestRemoteShareKey(t *testing.T) {
	cases := map[string]string{
		"smb://nas/photos":                    "smb://nas/photos",
		"smb://nas/photos/2024/a.jpg":         "smb://nas/photos",
		"smb://nas/photos/2024/deep/b.jpg":    "smb://nas/photos",
		"webdav://nas.example:5006/dav/a.jpg": "webdav://nas.example:5006",
		"webdav://nas.example:5006":           "webdav://nas.example:5006",
		"smb://nas":                           "",
		"C:\\Pictures":                        "",
	}
	for path, want := range cases {
		if got := remoteShareKey(path); got != want {
			t.Errorf("remoteShareKey(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestRemoteNameAndParent(t *testing.T) {
	cases := []struct {
		path, name, parent string
	}{
		{"smb://nas/photos", "photos", ""},
		{"smb://nas/photos/2024", "2024", "smb://nas/photos"},
		{"smb://nas/photos/2024/a.jpg", "a.jpg", "smb://nas/photos/2024"},
		{"smb://nas/photos/2024/", "2024", "smb://nas/photos"},
	}
	for _, c := range cases {
		if got := remoteName(c.path); got != c.name {
			t.Errorf("remoteName(%q) = %q, want %q", c.path, got, c.name)
		}
		if got := remoteParent(c.path); got != c.parent {
			t.Errorf("remoteParent(%q) = %q, want %q", c.path, got, c.parent)
		}
	}
}

func TestRemotePathOnShare(t *testing.T) {
	key, rel := remotePathOnShare("smb://nas/photos/2024/a.jpg")
	if key != "smb://nas/photos" || rel != "2024/a.jpg" {
		t.Fatalf("got %q %q", key, rel)
	}
	key, rel = remotePathOnShare("smb://nas/photos")
	if key != "smb://nas/photos" || rel != "" {
		t.Fatalf("root: got %q %q", key, rel)
	}
}

func TestBuildRemotePathEscapesNames(t *testing.T) {
	got := buildRemotePath("smb://nas/photos", "2024 旅行/a b.jpg")
	if !strings.HasPrefix(got, "smb://nas/photos/") {
		t.Fatalf("lost the share root: %q", got)
	}
	if strings.Contains(got, " ") {
		t.Fatalf("name was not escaped: %q", got)
	}
	// The escaped tail must decode back to the original relative path.
	_, rel := remotePathOnShare(got)
	if decoded := unescapeRemoteName(rel); decoded != "2024 旅行/a b.jpg" {
		t.Fatalf("round trip = %q", decoded)
	}
}

// TestRemoteShareIDIsStable guards the value that keys saved shares, the
// password store and the connection pool: it must not depend on the protocol
// in ways that make a saved share unreachable after a restart.
func TestRemoteShareIDIsStable(t *testing.T) {
	smb := remoteShare{Protocol: remoteSMB, Host: "nas", Port: 4455, Share: "photos", User: "me"}
	again := remoteShare{Protocol: remoteSMB, Host: "NAS", Port: 4455, Share: "photos", User: "ME"}
	if remoteShareID(smb) != remoteShareID(again) {
		t.Fatalf("%q != %q", remoteShareID(smb), remoteShareID(again))
	}
	// A share field must appear exactly once, or the saved id and the looked-up
	// id drift apart and a restart loses the password.
	if got := strings.Count(remoteShareID(smb), "photos"); got != 1 {
		t.Fatalf("share name appears %d times in %q", got, remoteShareID(smb))
	}
}

func TestRemoteRootAndID(t *testing.T) {
	smb := remoteShare{Protocol: remoteSMB, Host: "nas", Share: "photos", User: "me"}
	if got := remoteRoot(smb); got != "smb://nas/photos" {
		t.Fatalf("smb root = %q", got)
	}
	dav := remoteShare{Protocol: remoteWebDAV, Host: "nas.example", Port: 5006, Base: "/dav/photos"}
	if got := remoteRoot(dav); got != "webdav://nas.example:5006/dav/photos" {
		t.Fatalf("webdav root = %q", got)
	}
	if remoteShareID(smb) == remoteShareID(dav) {
		t.Fatal("different shares got the same id")
	}
	// Case in the host or user must not fork the connection.
	upper := smb
	upper.Host = "NAS"
	upper.User = "ME"
	if remoteShareID(smb) != remoteShareID(upper) {
		t.Fatal("id is case sensitive on host or user")
	}
}

func TestCleanDisplayPathKeepsVirtualPaths(t *testing.T) {
	if got := cleanDisplayPath("smb://nas/photos/"); got != "smb://nas/photos" {
		t.Fatalf("virtual path = %q", got)
	}
	if got := cleanDisplayPath(`C:\Pictures\..\Pictures`); got != `C:\Pictures` {
		t.Fatalf("local path = %q", got)
	}
}

func TestValidateRemoteShare(t *testing.T) {
	if err := validateRemoteShare(remoteShare{Protocol: remoteSMB, Host: "nas", Share: "photos"}); err != nil {
		t.Fatalf("valid share rejected: %v", err)
	}
	bad := []remoteShare{
		{Protocol: remoteSMB, Share: "photos"},
		{Protocol: remoteSMB, Host: "nas"},
		{Protocol: remoteSMB, Host: `nas\photos`, Share: "x"},
		{Protocol: "ftp", Host: "nas"},
	}
	for _, share := range bad {
		if err := validateRemoteShare(share); err == nil {
			t.Fatalf("%+v should be rejected", share)
		}
	}
}

// fakeBackend serves one file and one folder, so the browsing path can be
// tested without a server.
type fakeBackend struct {
	entries []remoteEntry
	data    string
	closed  bool
}

func (f *fakeBackend) list(ctx context.Context, dir string) ([]remoteEntry, error) {
	return f.entries, nil
}

func (f *fakeBackend) open(ctx context.Context, name string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(f.data)), nil
}

func (f *fakeBackend) serveHTTP(w http.ResponseWriter, r *http.Request, name string) error {
	http.ServeContent(w, r, name, time.Unix(1700000000, 0), strings.NewReader(f.data))
	return nil
}

func (f *fakeBackend) close() error {
	f.closed = true
	return nil
}

func TestRemoteBrowseBuildsItems(t *testing.T) {
	backend := &fakeBackend{entries: []remoteEntry{
		{Name: "a.jpg", Size: 1200, ModTime: time.Unix(1700000000, 0)},
		{Name: "clip.mp4", Size: 9000},
		{Name: "sub", IsDir: true},
		{Name: "notes.txt", Size: 12},
	}}
	prev := remoteListFn
	remoteListFn = func(ctx context.Context, p string) ([]remoteEntry, error) {
		return backend.list(ctx, p)
	}
	defer func() { remoteListFn = prev }()

	res := remoteBrowse(BrowseRequest{Path: "smb://nas/photos", Limit: 100, ShowOther: true})
	if res.Error != "" {
		t.Fatalf("browse error: %s", res.Error)
	}
	if res.Name != "photos" {
		t.Fatalf("name = %q", res.Name)
	}
	if res.Parent != "" {
		t.Fatalf("share root should have no parent, got %q", res.Parent)
	}
	if len(res.Items) != 4 {
		t.Fatalf("got %d items, want 4 (folder kept)", len(res.Items))
	}
	if res.Images != 1 || res.Videos != 1 || res.Folders != 1 {
		t.Fatalf("counts = %d/%d/%d", res.Images, res.Videos, res.Folders)
	}
	for _, it := range res.Items {
		if !isRemotePath(it.Path) {
			t.Fatalf("item path is not virtual: %q", it.Path)
		}
	}
}

func TestRemoteBrowseHidesOtherFilesByDefault(t *testing.T) {
	prev := remoteListFn
	remoteListFn = func(ctx context.Context, p string) ([]remoteEntry, error) {
		return []remoteEntry{{Name: "notes.txt", Size: 12}, {Name: "a.jpg"}}, nil
	}
	defer func() { remoteListFn = prev }()

	res := remoteBrowse(BrowseRequest{Path: "smb://nas/photos", Limit: 100})
	if len(res.Items) != 1 || res.Items[0].Name != "a.jpg" {
		t.Fatalf("other files should be hidden: %+v", res.Items)
	}
	res = remoteBrowse(BrowseRequest{Path: "smb://nas/photos", Limit: 100, ShowOther: true})
	if len(res.Items) != 2 {
		t.Fatalf("ShowOther should include them: %d", len(res.Items))
	}
}

func TestRemoteBrowseReportsErrors(t *testing.T) {
	prev := remoteListFn
	remoteListFn = func(ctx context.Context, p string) ([]remoteEntry, error) {
		return nil, errors.New("用户名或密码不正确")
	}
	defer func() { remoteListFn = prev }()

	res := remoteBrowse(BrowseRequest{Path: "smb://nas/photos", Limit: 10})
	if res.Error != "用户名或密码不正确" {
		t.Fatalf("error = %q", res.Error)
	}
}

func TestRemoteBrowseTruncates(t *testing.T) {
	entries := make([]remoteEntry, 50)
	for i := range entries {
		entries[i] = remoteEntry{Name: "p" + strings.Repeat("x", i%3) + ".jpg"}
	}
	prev := remoteListFn
	remoteListFn = func(ctx context.Context, p string) ([]remoteEntry, error) {
		return entries, nil
	}
	defer func() { remoteListFn = prev }()

	res := remoteBrowse(BrowseRequest{Path: "smb://nas/photos", Limit: 10})
	if len(res.Items) > 10 {
		t.Fatalf("limit ignored: %d items", len(res.Items))
	}
	if !res.Truncated {
		t.Fatal("a capped listing should report Truncated")
	}
}

func TestMediaServerRejectsBadTokenAndLocalPaths(t *testing.T) {
	server := &appMediaServer{}
	server.token = "secret"

	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest("GET", "/m/wrong/smb://nas/photos/a.mp4", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("wrong token: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest("GET", "/m/secret/"+urlPathEscape(`C:\secret.mp4`), nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("local path: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest("PUT", "/m/secret/"+urlPathEscape("smb://nas/photos/a.mp4"), nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("write method: %d", rec.Code)
	}
}

func TestMediaServerServesRange(t *testing.T) {
	backend := &fakeBackend{data: "0123456789"}
	prev := remoteListFn
	remoteListFn = func(ctx context.Context, p string) ([]remoteEntry, error) { return nil, nil }
	defer func() { remoteListFn = prev }()

	prevBackend := remoteBackendFn
	remoteBackendFn = func(ctx context.Context, p string) (remoteBackend, string, error) {
		return backend, "", nil
	}
	defer func() { remoteBackendFn = prevBackend }()

	server := &appMediaServer{token: "t"}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/m/t/"+urlPathEscape("smb://nas/photos/v.mp4"), nil)
	req.Header.Set("Range", "bytes=2-5")
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", rec.Code)
	}
	if got := rec.Body.String(); got != "2345" {
		t.Fatalf("body = %q, want 2345", got)
	}
}

func urlPathEscape(s string) string {
	return (&urlEscaper{}).escape(s)
}
func TestRemoteWritesAreRejected(t *testing.T) {
	svc := AppSvc{}
	remote := "smb://nas/photos/a.jpg"
	if err := svc.Delete(remote, true); err != errReadOnlyRemote {
		t.Fatalf("Delete = %v", err)
	}
	if _, err := svc.Rename(remote, "b.jpg"); err != errReadOnlyRemote {
		t.Fatalf("Rename = %v", err)
	}
	if _, err := svc.NewFolder("smb://nas/photos", "x"); err != errReadOnlyRemote {
		t.Fatalf("NewFolder = %v", err)
	}
	for name, err := range map[string]error{
		"Reveal":       svc.Reveal(remote),
		"OpenFolder":   svc.OpenFolder("smb://nas/photos"),
		"OpenExternal": svc.OpenExternal(remote),
	} {
		if err != errRemoteNoExplorer {
			t.Fatalf("%s = %v", name, err)
		}
	}
	// A local path must still go through: the guard is a prefix test, not a
	// blanket refusal.
	if err := svc.Delete(filepath.Join(t.TempDir(), "missing.jpg"), true); err == errReadOnlyRemote {
		t.Fatal("local delete was refused")
	}
}
func TestRemotePasswordRoundTripsThroughDPAPI(t *testing.T) {
	dir := t.TempDir()
	prev := remoteCredDirOverride
	remoteCredDirOverride = dir
	defer func() { remoteCredDirOverride = prev; clearRemoteCredCache() }()

	share := remoteShare{Protocol: remoteWebDAV, Host: "nas", Port: 5005, Base: "dav", User: "me", SavePass: true}
	rememberRemotePassword(share, "s3cr3t-p@ss", true)
	if got := passwordForShare(share); got != "s3cr3t-p@ss" {
		t.Fatalf("password = %q", got)
	}

	// The file on disk must not contain the plaintext.
	raw, err := os.ReadFile(filepath.Join(dir, remoteCredFile))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(raw), "s3cr3t") {
		t.Fatal("password was stored in plaintext")
	}

	// A fresh cache (as after a restart) must still decrypt.
	clearRemoteCredCache()
	if got := passwordForShare(share); got != "s3cr3t-p@ss" {
		t.Fatalf("after reload = %q", got)
	}

	// A different share must not see it.
	other := remoteShare{Protocol: remoteWebDAV, Host: "nas2", Port: 5005, Base: "dav", User: "me"}
	if got := passwordForShare(other); got != "" {
		t.Fatalf("password leaked to another share: %q", got)
	}

	forgetRemotePassword(share)
	clearRemoteCredCache()
	if got := passwordForShare(share); got != "" {
		t.Fatalf("password survived forget: %q", got)
	}
}
func TestWithoutWriteEntriesHidesRemoteOnlyRows(t *testing.T) {
	entries := []gridMenuEntry{
		{label: "打开"},
		{label: "在资源管理器中显示", localOnly: true},
		{label: "复制路径"},
		{divider: true},
		{label: "重命名…", write: true},
		{label: "移到回收站", write: true},
		{divider: true},
		{label: "显示信息"},
	}
	got := withoutWriteEntries(true, entries)
	labels := make([]string, 0, len(got))
	for _, e := range got {
		if e.divider {
			labels = append(labels, "—")
			continue
		}
		labels = append(labels, e.label)
	}
	want := []string{"打开", "复制路径", "—", "显示信息"}
	if strings.Join(labels, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v, want %v", labels, want)
	}
	// A local tab keeps everything.
	if len(withoutWriteEntries(false, entries)) != len(entries) {
		t.Fatal("local tab lost entries")
	}
	// A menu that is all writes must not start with a dangling divider.
	allWrites := withoutWriteEntries(true, []gridMenuEntry{{divider: true}, {label: "x", write: true}})
	if len(allWrites) != 0 {
		t.Fatalf("left %v", allWrites)
	}
}
func TestShareForPathFindsSavedShare(t *testing.T) {
	prev := getSettings()
	settingsMu.Lock()
	settingsData.RemoteShares = []remoteShare{
		{ID: "x", Protocol: remoteWebDAV, Host: "127.0.0.1", Port: 5999},
	}
	settingsMu.Unlock()
	defer func() {
		settingsMu.Lock()
		settingsData = prev
		settingsMu.Unlock()
	}()

	for _, p := range []string{"webdav://127.0.0.1:5999", "webdav://127.0.0.1:5999/sub/a.jpg"} {
		share, err := shareForPath(p)
		if err != nil {
			t.Fatalf("shareForPath(%q): %v", p, err)
		}
		if share.Port != 5999 {
			t.Fatalf("wrong share: %+v", share)
		}
	}
}

func TestSMBErrorMessagesAreActionable(t *testing.T) {
	cases := map[string]string{
		"The user name or password is incorrect": "密码",
		"Access is denied":                       "权限",
		"The network path was not found":         "找不到网络路径",
		"The network name cannot be found":       "找不到共享名",
		"connection refused":                     "连接被拒绝",
		"no such host":                           "找不到主机",
		"i/o timeout":                            "网络",
	}
	for in, want := range cases {
		got := smbErrorMessage(errors.New(in)).Error()
		if !strings.Contains(got, want) {
			t.Errorf("%q -> %q, want to contain %q", in, got, want)
		}
	}
	if got := smbErrorMessage(nil); got != nil {
		t.Fatalf("nil -> %v", got)
	}
}
func TestSMBBackendAgainstLocalServer(t *testing.T) {
	user := os.Getenv("FLOWVISION_SMB_USER")
	pass := os.Getenv("FLOWVISION_SMB_PASS")
	shareName := os.Getenv("FLOWVISION_SMB_SHARE")
	if user == "" || shareName == "" {
		t.Skip("set FLOWVISION_SMB_USER / FLOWVISION_SMB_PASS / FLOWVISION_SMB_SHARE")
	}
	port := 445
	if p := os.Getenv("FLOWVISION_SMB_PORT"); p != "" {
		port, _ = strconv.Atoi(p)
	}
	share := remoteShare{Protocol: remoteSMB, Host: "127.0.0.1", Port: port, Share: shareName, User: user}
	b, err := dialRemoteSMB(context.Background(), share, pass)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer b.close()
	entries, err := b.list(context.Background(), "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("share listed nothing")
	}
	t.Logf("listed %d entries", len(entries))

	// Read a file through the same backend, so the whole path (list, open,
	// stream) is covered against a real SMB2 server rather than a fake.
	for _, e := range entries {
		if e.IsDir {
			continue
		}
		rc, err := b.open(context.Background(), e.Name)
		if err != nil {
			t.Fatalf("open %s: %v", e.Name, err)
		}
		buf := make([]byte, 16)
		n, _ := io.ReadFull(rc, buf)
		rc.Close()
		if n == 0 {
			t.Fatalf("%s read nothing", e.Name)
		}
		break
	}
}

func TestRemoteThumbnailEndToEnd(t *testing.T) {
	root := t.TempDir()
	writePNG(t, filepath.Join(root, "a.png"), 900, 600)
	srv := httptest.NewServer(davHandler(root))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")

	share := remoteShare{Protocol: remoteWebDAV, Host: host, ID: remoteShareID(remoteShare{Protocol: remoteWebDAV, Host: host})}
	prev := getSettings()
	settingsMu.Lock()
	settingsData.RemoteShares = []remoteShare{share}
	settingsMu.Unlock()
	defer func() {
		settingsMu.Lock()
		settingsData.RemoteShares = prev.RemoteShares
		settingsMu.Unlock()
		remotePoolInstance.release(remoteShareID(share))
	}()

	virtual := remoteRoot(share) + "/a.png"
	t.Logf("virtual = %s", virtual)

	d := imageDims(virtual, 0)
	t.Logf("dims = %+v", d)
	if d.W == 0 {
		t.Fatalf("dimensions not read from the share")
	}

	f, err := openPathStream(virtual)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	f.Close()

	it := items.Intern(virtual)
	it.Kind, it.Ext = "image", ".png"
	data, err := renderThumbGo(it, 240)
	if err != nil {
		t.Fatalf("renderThumbGo: %v", err)
	}
	if len(data) < 500 {
		t.Fatalf("thumbnail too small: %d bytes", len(data))
	}
}

func TestSMBDialReportsTranslatedError(t *testing.T) {
	// Port 1 is never an SMB server: the point is that the failure arrives as a
	// readable message rather than a raw protocol error, and quickly.
	_, err := dialRemoteSMB(context.Background(), remoteShare{Protocol: remoteSMB, Host: "127.0.0.1", Port: 1, Share: "x"}, "")
	if err == nil {
		t.Fatal("dial to a closed port succeeded")
	}
	t.Logf("error: %v", err)
}
func TestRemoteBreadcrumbItems(t *testing.T) {
	labels, paths := breadcrumbItems("smb://nas/photos/2024", "2024")
	if len(labels) != 2 || labels[0] != "smb://nas/photos" || labels[1] != "2024" {
		t.Fatalf("labels = %v", labels)
	}
	if paths[0] != "smb://nas/photos" || paths[1] != "smb://nas/photos/2024" {
		t.Fatalf("paths = %v", paths)
	}
	// The share root is a single, non-interactive-looking crumb.
	labels, paths = breadcrumbItems("smb://nas/photos", "photos")
	if len(labels) != 1 || labels[0] != "smb://nas/photos" || paths[0] != "smb://nas/photos" {
		t.Fatalf("root = %v %v", labels, paths)
	}
	// Escaped names decode for display but keep their escaped form in paths.
	labels, paths = breadcrumbItems("smb://nas/photos/2024%20%E6%97%85%E8%A1%8C", "")
	if labels[len(labels)-1] != "2024 旅行" {
		t.Fatalf("escaped label = %q", labels[len(labels)-1])
	}
	if paths[len(paths)-1] != "smb://nas/photos/2024%20%E6%97%85%E8%A1%8C" {
		t.Fatalf("escaped path = %q", paths[len(paths)-1])
	}
	// A local path still uses drive crumbs.
	labels, paths = breadcrumbItems(`C:\Pictures\2024`, "2024")
	if len(labels) != 3 || labels[0] != "C:" || paths[0] != `C:\` {
		t.Fatalf("local = %v %v", labels, paths)
	}
}
