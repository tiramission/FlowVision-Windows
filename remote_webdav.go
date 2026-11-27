package main

import (
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// remoteWebDAVSession speaks WebDAV over net/http. Listing is one PROPFIND with
// Depth: 1, and because WebDAV is HTTP the media server can forward a Range
// request straight through, which is what makes remote video stream instead of
// downloading first.
type remoteWebDAVSession struct {
	base   string // https://host:port/base, no trailing slash
	user   string
	pass   string
	client *http.Client
}

func dialRemoteWebDAV(ctx context.Context, share remoteShare, password string) (remoteBackend, error) {
	scheme := "http"
	if share.Secure {
		scheme = "https"
	}
	port := share.Port
	if port <= 0 && share.Secure {
		port = 443
	}
	base := strings.TrimSuffix(share.Base, "/")
	if base != "" && !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	transport := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 12 * time.Second}).DialContext,
		TLSHandshakeTimeout:   12 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		ForceAttemptHTTP2:     false,
	}
	if share.Insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	session := &remoteWebDAVSession{
		base:   fmt.Sprintf("%s://%s%s", scheme, hostPort(share.Host, port), base),
		user:   share.User,
		pass:   password,
		client: &http.Client{Transport: transport},
	}
	// One request up front: a wrong URL or password should fail while the
	// dialog is still open, not on the first thumbnail.
	if _, err := session.list(ctx, ""); err != nil {
		return nil, err
	}
	return session, nil
}

// url joins the base and a relative name, escaping each segment.
func (s *remoteWebDAVSession) url(name string) string {
	parts := []string{}
	for _, seg := range strings.Split(strings.Trim(name, "/"), "/") {
		if seg == "" {
			continue
		}
		parts = append(parts, pathEscape(seg))
	}
	if len(parts) == 0 {
		return s.base
	}
	return s.base + "/" + strings.Join(parts, "/")
}

// pathEscape escapes one path segment the way WebDAV servers expect, keeping
// the characters people actually have in file names readable.
func pathEscape(seg string) string {
	escaped := (&urlEscaper{}).escape(seg)
	return escaped
}

type urlEscaper struct{}

func (urlEscaper) escape(seg string) string {
	var b strings.Builder
	for i := 0; i < len(seg); i++ {
		c := seg[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~', c == '(', c == ')', c == ',',
			c == ';', c == '=', c == '@', c == '!', c == '$', c == '&', c == '\'', c == '*', c == '+':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func (s *remoteWebDAVSession) request(ctx context.Context, method, target string, body io.Reader, header http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	if s.user != "" {
		req.SetBasicAuth(s.user, s.pass)
	}
	for key, values := range header {
		for _, v := range values {
			req.Header.Add(key, v)
		}
	}
	return s.client.Do(req)
}

type davMultistatus struct {
	Responses []davResponse `xml:"response"`
}

type davResponse struct {
	Href     string        `xml:"href"`
	Propstat []davPropstat `xml:"propstat"`
}

type davPropstat struct {
	Status string  `xml:"status"`
	Prop   davProp `xml:"prop"`
}

type davProp struct {
	ContentLength string          `xml:"getcontentlength"`
	LastModified  string          `xml:"getlastmodified"`
	ResourceType  davResourceType `xml:"resourcetype"`
}

type davResourceType struct {
	Collection *struct{} `xml:"collection"`
}

const davListBody = `<?xml version="1.0" encoding="utf-8"?>
<D:propfind xmlns:D="DAV:"><D:prop>
<D:resourcetype/><D:getcontentlength/><D:getlastmodified/>
</D:prop></D:propfind>`

func (s *remoteWebDAVSession) list(ctx context.Context, dir string) ([]remoteEntry, error) {
	header := http.Header{
		"Depth":        {"1"},
		"Content-Type": {"application/xml; charset=utf-8"},
	}
	resp, err := s.request(ctx, "PROPFIND", s.url(dir), strings.NewReader(davListBody), header)
	if err != nil {
		return nil, webdavError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("用户名或密码不正确")
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("拒绝访问：账号没有这个目录的权限")
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("目录不存在")
	}
	if resp.StatusCode != 207 && resp.StatusCode != 200 {
		return nil, fmt.Errorf("列目录失败（HTTP %d）", resp.StatusCode)
	}
	var doc davMultistatus
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&doc); err != nil {
		return nil, fmt.Errorf("服务器返回的目录不是 WebDAV（%v）", err)
	}
	self := strings.TrimSuffix(resp.Request.URL.Path, "/")
	out := make([]remoteEntry, 0, len(doc.Responses))
	for _, r := range doc.Responses {
		href := r.Href
		if u, err := resp.Request.URL.Parse(href); err == nil {
			href = u.Path
		}
		href = strings.TrimSuffix(href, "/")
		if href == "" || href == self {
			continue
		}
		prop := davProp{}
		for _, ps := range r.Propstat {
			if strings.Contains(ps.Status, "200") {
				prop = ps.Prop
				break
			}
		}
		name := href
		if i := strings.LastIndex(href, "/"); i >= 0 {
			name = href[i+1:]
		}
		name = unescapeRemoteName(name)
		if name == "" || name == "." || name == ".." {
			continue
		}
		size, _ := strconv.ParseInt(strings.TrimSpace(prop.ContentLength), 10, 64)
		var modified time.Time
		if t, err := http.ParseTime(strings.TrimSpace(prop.LastModified)); err == nil {
			modified = t
		}
		isDir := prop.ResourceType.Collection != nil
		if isDir {
			size = 0
		}
		out = append(out, remoteEntry{Name: name, IsDir: isDir, Size: size, ModTime: modified})
	}
	return out, nil
}

func (s *remoteWebDAVSession) open(ctx context.Context, name string) (io.ReadCloser, error) {
	resp, err := s.request(ctx, "GET", s.url(name), nil, nil)
	if err != nil {
		return nil, webdavError(err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("读取失败（HTTP %d）", resp.StatusCode)
	}
	return resp.Body, nil
}

func (s *remoteWebDAVSession) serveHTTP(w http.ResponseWriter, r *http.Request, name string) error {
	header := http.Header{}
	if rng := r.Header.Get("Range"); rng != "" {
		header.Set("Range", rng)
	}
	resp, err := s.request(r.Context(), "GET", s.url(name), nil, header)
	if err != nil {
		return webdavError(err)
	}
	defer resp.Body.Close()
	for _, key := range []string{"Content-Range", "Content-Length", "Content-Type", "Last-Modified", "Accept-Ranges"} {
		if v := resp.Header.Get(key); v != "" {
			w.Header().Set(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	return err
}

func (s *remoteWebDAVSession) close() error {
	s.client.CloseIdleConnections()
	return nil
}

func webdavError(err error) error {
	text := err.Error()
	switch {
	case strings.Contains(text, "x509"), strings.Contains(text, "certificate"):
		return fmt.Errorf("证书不受信任：可在连接设置里允许自签名证书")
	// Windows says "actively refused it" where Go's own dialer says
	// "connection refused", and a closed port is the usual cause.
	case strings.Contains(text, "connection refused"), strings.Contains(text, "actively refused"),
		strings.Contains(text, "No connection could be made"):
		return fmt.Errorf("连接被拒绝：对方没有开 WebDAV 服务，或端口不对")
	case strings.Contains(text, "no such host"):
		return fmt.Errorf("找不到主机，请检查地址")
	case strings.Contains(text, "i/o timeout"), strings.Contains(text, "deadline exceeded"):
		return fmt.Errorf("连接超时，请检查网络")
	}
	return err
}
