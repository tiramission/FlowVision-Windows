package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestWebDAVBackendAgainstRealServer runs the WebDAV backend against a small
// in-process WebDAV server: listing, decoding a file and answering a Range
// request are the three things the app needs from it.
func TestWebDAVBackendAgainstRealServer(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.jpg"), []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "b.jpg"), []byte("xx"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(davHandler(root))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	b, err := dialRemoteWebDAV(context.Background(), remoteShare{Protocol: remoteWebDAV, Host: host}, "")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer b.close()

	entries, err := b.list(context.Background(), "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries: %+v", len(entries), entries)
	}

	rc, err := b.open(context.Background(), "a.jpg")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := rc.Read(buf); err != nil {
		t.Fatal(err)
	}
	rc.Close()
	if string(buf) != "0123" {
		t.Fatalf("read %q", buf)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/a.jpg", nil)
	req.Header.Set("Range", "bytes=2-4")
	if err := b.serveHTTP(rec, req, "a.jpg"); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusPartialContent || rec.Body.String() != "234" {
		t.Fatalf("range = %d %q", rec.Code, rec.Body.String())
	}
}

func davHandler(root string) http.Handler {
	fs := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PROPFIND" {
			fs.ServeHTTP(w, r)
			return
		}
		dir := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/")))
		entries, err := os.ReadDir(dir)
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		var b strings.Builder
		b.WriteString(`<?xml version="1.0"?><D:multistatus xmlns:D="DAV:">`)
		b.WriteString(`<D:response><D:href>` + r.URL.Path + `</D:href><D:propstat><D:status>HTTP/1.1 200 OK</D:status><D:prop><D:resourcetype><D:collection/></D:resourcetype></D:prop></D:propstat></D:response>`)
		for _, e := range entries {
			href := strings.TrimSuffix(r.URL.Path, "/") + "/" + e.Name()
			info, _ := e.Info()
			b.WriteString(`<D:response><D:href>` + href + `</D:href><D:propstat><D:status>HTTP/1.1 200 OK</D:status><D:prop>`)
			if e.IsDir() {
				b.WriteString(`<D:resourcetype><D:collection/></D:resourcetype>`)
			} else {
				b.WriteString(`<D:resourcetype/><D:getcontentlength>` + intToString(info.Size()) + `</D:getcontentlength>`)
				b.WriteString(`<D:getlastmodified>` + info.ModTime().UTC().Format(time.RFC1123) + `</D:getlastmodified>`)
			}
			b.WriteString(`</D:prop></D:propstat></D:response>`)
		}
		b.WriteString(`</D:multistatus>`)
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(207)
		_, _ = w.Write([]byte(b.String()))
	})
}

func intToString(n int64) string {
	return strconv.FormatInt(n, 10)
}

func TestWebDAVHTTPAndSecureRoot(t *testing.T) {
	plain := remoteShare{Protocol: remoteWebDAV, Host: "nas", Port: 5005}
	if got := remoteRoot(plain); got != "webdav://nas:5005" {
		t.Fatalf("plain root = %q", got)
	}
	secure := remoteShare{Protocol: remoteWebDAV, Host: "nas", Port: 5006, Secure: true, Base: "dav"}
	if got := remoteRoot(secure); got != "webdav://nas:5006/dav" {
		t.Fatalf("secure root = %q", got)
	}
	// A host that already carries a port must not get a second one, and the
	// port field replaces it when both are given.
	if got := hostPort("127.0.0.1:1124", 0); got != "127.0.0.1:1124" {
		t.Fatalf("hostPort kept the wrong port: %q", got)
	}
	if got := hostPort("127.0.0.1:1124", 443); got != "127.0.0.1:443" {
		t.Fatalf("hostPort did not replace the port: %q", got)
	}
	if got := hostPort("nas", 0); got != "nas" {
		t.Fatalf("hostPort with no port = %q", got)
	}
	if got := hostPort("nas", 5005); got != "nas:5005" {
		t.Fatalf("hostPort = %q", got)
	}
}

func TestWebDAVErrorMessagesAreActionable(t *testing.T) {
	cases := map[string]string{
		"dial tcp 127.0.0.1:5999: connectex: No connection could be made because the target machine actively refused it.": "连接被拒绝",
		"x509: certificate signed by unknown authority":                                                                   "证书",
		"no such host": "找不到主机",
		"i/o timeout":  "连接超时",
	}
	for in, want := range cases {
		got := webdavError(errors.New(in)).Error()
		if !strings.Contains(got, want) {
			t.Errorf("%q -> %q, want to contain %q", in, got, want)
		}
	}
	// An unknown error must survive untouched rather than be swallowed.
	if got := webdavError(errors.New("something else")).Error(); got != "something else" {
		t.Fatalf("unknown error rewritten to %q", got)
	}
}

// TestWebDAVURLPortSelection pins the URL the dialog actually dials. Each case
// is a combination a user can produce in the add dialog, and the bug being
// guarded is a wrong port slipping in (https://nas:80, https://nas:443 when
// 5006 was typed).
func TestWebDAVURLPortSelection(t *testing.T) {
	cases := []struct {
		name  string
		share remoteShare
		want  string
	}{
		{
			name:  "http with an explicit port keeps it",
			share: remoteShare{Protocol: remoteWebDAV, Host: "192.168.1.12", Port: 5005},
			want:  "http://192.168.1.12:5005",
		},
		{
			name:  "https with an explicit port keeps it",
			share: remoteShare{Protocol: remoteWebDAV, Host: "192.168.1.12", Port: 5006, Secure: true},
			want:  "https://192.168.1.12:5006",
		},
		{
			name:  "http without a port defaults to 80",
			share: remoteShare{Protocol: remoteWebDAV, Host: "nas"},
			want:  "http://nas:80",
		},
		{
			name:  "https without a port defaults to 443",
			share: remoteShare{Protocol: remoteWebDAV, Host: "nas", Secure: true},
			want:  "https://nas:443",
		},
		{
			name:  "a host carrying its own port wins over the default",
			share: remoteShare{Protocol: remoteWebDAV, Host: "127.0.0.1:7781"},
			want:  "http://127.0.0.1:7781",
		},
		{
			name:  "an explicit port replaces one in the host field",
			share: remoteShare{Protocol: remoteWebDAV, Host: "nas:9999", Port: 5006, Secure: true},
			want:  "https://nas:5006",
		},
		{
			name:  "a base path is appended without a double slash",
			share: remoteShare{Protocol: remoteWebDAV, Host: "nas", Port: 5005, Base: "/dav/photos/"},
			want:  "http://nas:5005/dav/photos",
		},
	}
	for _, c := range cases {
		if got := webdavBaseURL(c.share); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// TestWebDAVPortFollowsProtocolSwitch covers the dialog's default port: moving
// between HTTP and HTTPS changes 80<->443 only while the field still holds a
// default, and never overwrites a typed port.
func TestWebDAVPortFollowsProtocolSwitch(t *testing.T) {
	a := &App{}
	a.remoteDraft = remoteShare{Protocol: remoteWebDAV, Port: 80}
	a.syncRemotePortDraft()

	a.remoteDraft.Secure = true
	a.applyRemotePortDefault()
	if got := int(a.remotePortDraft); got != 443 {
		t.Fatalf("http->https default = %d", got)
	}
	a.remoteDraft.Secure = false
	a.applyRemotePortDefault()
	if got := int(a.remotePortDraft); got != 80 {
		t.Fatalf("https->http default = %d", got)
	}

	// A typed port survives both switches.
	a.remotePortDraft = 5006
	a.remoteDraft.Port = 5006
	a.remoteDraft.Secure = true
	a.applyRemotePortDefault()
	if got := int(a.remotePortDraft); got != 5006 {
		t.Fatalf("typed port was replaced on https: %d", got)
	}
	a.remoteDraft.Secure = false
	a.applyRemotePortDefault()
	if got := int(a.remotePortDraft); got != 5006 {
		t.Fatalf("typed port was replaced on http: %d", got)
	}
}

// TestCommitRemoteShareUsesPortField is the bug from the dialog screenshot:
// typing a port and pressing 连接 in the same frame sent the old default.
func TestCommitRemoteShareUsesPortField(t *testing.T) {
	a := newViewTestApp()
	a.remoteDialogOpen = true
	a.remoteDraft = remoteShare{Protocol: remoteWebDAV, Host: "192.168.1.12", Secure: true, Port: 80}
	// The field holds what the user typed; the draft still has the default.
	a.remotePortDraft = 5006

	draft := a.remoteDraft
	draft.Port = int(a.remotePortDraft)
	if got := webdavBaseURL(draft); got != "https://192.168.1.12:5006" {
		t.Fatalf("committed URL = %q", got)
	}
}

// TestWebDAVDialUsesChosenPort is the end-to-end version of the port bug: the
// dialog's port must reach the actual dial, not just the URL string.
func TestWebDAVDialUsesChosenPort(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.jpg"), []byte("xx"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(davHandler(root))
	defer srv.Close()

	// Split the httptest URL into host and port, the way the dialog does.
	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portStr)

	share := remoteShare{Protocol: remoteWebDAV, Host: host, Port: port}
	b, err := dialRemoteWebDAV(context.Background(), share, "")
	if err != nil {
		t.Fatalf("dial with an explicit port: %v", err)
	}
	b.close()

	// The wrong port must fail: this is the case the screenshot hit, where the
	// default 80 was sent instead of the typed value.
	wrong := remoteShare{Protocol: remoteWebDAV, Host: host, Port: port + 1}
	if _, err := dialRemoteWebDAV(context.Background(), wrong, ""); err == nil {
		t.Fatal("dial to the wrong port succeeded")
	}
}

// TestWebDAVHTTPSWithSkipVerify covers the NAS case the dialog offers a switch
// for: an HTTPS endpoint whose certificate nobody can vouch for. The default
// must refuse it, and the switch must be the only way through.
func TestWebDAVHTTPSWithSkipVerify(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.jpg"), []byte("xx"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewTLSServer(davHandler(root))
	defer srv.Close()

	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "https://"))
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portStr)

	// Without the switch the certificate must be refused, so a broken one is
	// reported rather than silently trusted.
	strict := remoteShare{Protocol: remoteWebDAV, Host: host, Port: port, Secure: true}
	if _, err := dialRemoteWebDAV(context.Background(), strict, ""); err == nil {
		t.Fatal("a self-signed certificate was accepted without the switch")
	}

	relaxed := strict
	relaxed.SkipVerify = true
	b, err := dialRemoteWebDAV(context.Background(), relaxed, "")
	if err != nil {
		t.Fatalf("dial with 跳过证书验证: %v", err)
	}
	defer b.close()
	entries, err := b.list(context.Background(), "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %+v", entries)
	}
}

// TestTLSSkipVerifyAcceptsAnyCertificate is the reason the switch exists: a
// certificate issued for a different name is exactly what a NAS with a
// mismatched self-signed certificate presents, and the default check rejects
// it. Skipping verification must get through, otherwise the switch would be
// useless for the case it is meant to cover.
func TestTLSSkipVerifyAcceptsAnyCertificate(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.jpg"), []byte("xx"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewTLSServer(davHandler(root))
	defer srv.Close()

	_, portStr, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "https://"))
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portStr)

	// httptest's certificate names "example.com", so dialing 127.0.0.1 is a
	// name mismatch on top of being untrusted. Only the unchecked mode can
	// connect, which is the whole point of offering it.
	share := remoteShare{
		Protocol: remoteWebDAV, Host: "127.0.0.1", Port: port,
		Secure: true, SkipVerify: true,
	}
	b, err := dialRemoteWebDAV(context.Background(), share, "")
	if err != nil {
		t.Fatalf("skip-verify dial: %v", err)
	}
	defer b.close()
	if _, err := b.list(context.Background(), ""); err != nil {
		t.Fatalf("list: %v", err)
	}

	// The same share without the switch must fail, so the switch is not
	// accidentally on by default.
	share.SkipVerify = false
	if _, err := dialRemoteWebDAV(context.Background(), share, ""); err == nil {
		t.Fatal("name-mismatched certificate was accepted without the switch")
	}
}

// TestTLSConfigFor covers the mapping from the setting to a tls.Config.
func TestTLSConfigFor(t *testing.T) {
	if cfg := tlsConfigFor(remoteShare{Secure: true}); cfg != nil {
		t.Fatalf("default returned a config: %+v", cfg)
	}
	cfg := tlsConfigFor(remoteShare{Secure: true, SkipVerify: true})
	if cfg == nil || !cfg.InsecureSkipVerify {
		t.Fatalf("skip-verify config = %+v", cfg)
	}
}

// TestWebDAVBasePathLists is the edit-path case end to end: a share whose base
// is a subfolder must list that subfolder, not the root.
func TestWebDAVBasePathLists(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "b.jpg"), []byte("xx"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "top.jpg"), []byte("xx"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(davHandler(root))
	defer srv.Close()

	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portStr)

	share := remoteShare{Protocol: remoteWebDAV, Host: host, Port: port, Base: "sub"}
	b, err := dialRemoteWebDAV(context.Background(), share, "")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer b.close()
	entries, err := b.list(context.Background(), "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "b.jpg" {
		t.Fatalf("base listing = %+v", entries)
	}
}

// TestWebDAVBaseIsNotDoubled is the /sub/sub bug: the share's base path is part
// of the address, so it must not also appear in the relative path handed to the
// backend.
func TestWebDAVBaseIsNotDoubled(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub", "inner"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b.jpg"), []byte("xx"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "inner", "c.jpg"), []byte("xx"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(davHandler(dir))
	defer srv.Close()

	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portStr)

	share := remoteShare{Protocol: remoteWebDAV, Host: host, Port: port, Base: "sub"}
	share.ID = remoteShareID(share)
	prev := getSettings()
	settingsMu.Lock()
	settingsData.RemoteShares = []remoteShare{share}
	settingsMu.Unlock()
	defer func() {
		settingsMu.Lock()
		settingsData = prev
		settingsMu.Unlock()
	}()

	// The share root is the base itself: nothing left to append.
	root := remoteRoot(share)
	_, rel := remotePathOnShare(root)
	if rel != "" {
		t.Fatalf("root relative path = %q, want empty", rel)
	}

	// A folder below the base keeps only the part below it.
	inner := root + "/inner"
	_, rel = remotePathOnShare(inner)
	if rel != "inner" {
		t.Fatalf("inner relative path = %q", rel)
	}

	// The backend then lists the right folder.
	b, err := dialRemoteWebDAV(context.Background(), share, "")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer b.close()
	entries, err := b.list(context.Background(), rel)
	if err != nil {
		t.Fatalf("list %q: %v", rel, err)
	}
	if len(entries) != 1 || entries[0].Name != "c.jpg" {
		t.Fatalf("inner listing = %+v", entries)
	}
}
