package main

import (
	"context"
	"errors"
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
		"x509: certificate signed by unknown authority":                                                                   "自签名证书",
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
