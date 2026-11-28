package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Remote folders live inside the app. A share is connected with its own client,
// credentials stay here (encrypted with DPAPI when remembered), and nothing is
// mounted, mapped or registered with Windows.
//
// Above the transport everything is a virtual path, so a tab, an Item and the
// viewer do not care which protocol a folder came from:
//
//	smb://nas/photos/2024/a.jpg
//	webdav://nas.example:5006/photos/2024/a.jpg

const (
	remoteSMB    = "smb"
	remoteWebDAV = "webdav"
)

// remoteEntry is one entry a backend lists.
type remoteEntry struct {
	Name    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

// remoteBackend is what a share's protocol provides. Everything takes a path
// relative to the share root ("2024/a.jpg", "" for the root).
type remoteBackend interface {
	list(ctx context.Context, dir string) ([]remoteEntry, error)
	open(ctx context.Context, name string) (io.ReadCloser, error)
	// serveHTTP answers a Range request for the local media server: SMB serves
	// from a seekable file, WebDAV forwards the range. Remote video plays
	// through this instead of downloading the file first.
	serveHTTP(w http.ResponseWriter, r *http.Request, name string) error
	close() error
}

// remoteShare is a saved connection. The password is not here: it lives in
// memory for the session and, when remembered, in an encrypted file.
type remoteShare struct {
	ID       string `json:"id"`
	Protocol string `json:"protocol"` // smb | webdav
	Name     string `json:"name,omitempty"`
	Host     string `json:"host"`
	Port     int    `json:"port,omitempty"`
	Share    string `json:"share,omitempty"` // SMB share name
	Base     string `json:"base,omitempty"`  // WebDAV base path
	User     string `json:"user,omitempty"`
	SavePass bool   `json:"savePass,omitempty"`
	// Secure picks HTTPS for WebDAV. Many NAS boxes serve plain HTTP on their
	// WebDAV port, so it is a choice rather than an assumption.
	Secure bool `json:"secure,omitempty"`
	// SkipVerify turns off certificate checking entirely, for a NAS whose
	// certificate is self-signed, expired or issued for another name. It is
	// one switch rather than a policy enum because every case it covers needs
	// the same thing, and a finer split would only add states to explain.
	SkipVerify bool `json:"skipVerify,omitempty"`
}

// remoteAlias is the label a share is shown under, or "" when it has none.
// An alias is per connection, not per folder: every path inside the share uses
// it as the first breadcrumb, which is what keeps a long address out of the
// interface.
func remoteAlias(s remoteShare) string {
	return strings.TrimSpace(s.Name)
}

// remoteShareForPath finds the saved share a virtual path belongs to.
func remoteShareForPath(p string) (remoteShare, bool) {
	key := remoteShareKey(p)
	if key == "" {
		return remoteShare{}, false
	}
	for _, s := range getSettings().RemoteShares {
		if remoteShareKey(remoteRoot(s)) == key {
			return s, true
		}
	}
	return remoteShare{}, false
}

// remoteAliasForPath is the alias of the share a path belongs to, or "".
func remoteAliasForPath(p string) string {
	s, ok := remoteShareForPath(p)
	if !ok {
		return ""
	}
	return remoteAlias(s)
}

// remoteRoot is the virtual path a share opens at.
func remoteRoot(s remoteShare) string {
	switch s.Protocol {
	case remoteSMB:
		return "smb://" + s.Host + "/" + s.Share
	default:
		root := "webdav://" + hostPort(s.Host, s.Port)
		if base := strings.Trim(s.Base, "/"); base != "" {
			root += "/" + base
		}
		return root
	}
}

// remoteShareID is stable for a connection's identity: two shares to the same
// place with the same account are the same connection.
func remoteShareID(s remoteShare) string {
	parts := []string{s.Protocol, strings.ToLower(s.Host), fmt.Sprint(s.Port), s.Share, strings.Trim(s.Base, "/"), strings.ToLower(s.User)}
	return strings.Join(parts, "|")
}

// hostPort joins a host and a port, keeping any port the host already carries
// and defaulting the scheme's own port when none is given.
func hostPort(host string, port int) string {
	if h, p, err := net.SplitHostPort(host); err == nil {
		if port <= 0 {
			return net.JoinHostPort(h, p)
		}
		host = h
	}
	if port <= 0 {
		return host
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// isRemotePath reports whether p is a virtual remote path.
func isRemotePath(p string) bool {
	// The scheme is matched case-insensitively: a favorite or a pasted address
	// can arrive as SMB://, and treating it as a local path would send it
	// through filepath.Clean and lose the slashes.
	if len(p) < 6 {
		return false
	}
	lower := strings.ToLower(p)
	return strings.HasPrefix(lower, remoteSMB+"://") || strings.HasPrefix(lower, remoteWebDAV+"://")
}

// remoteShareKey returns the share a virtual path belongs to, as
// smb://host/share or webdav://host:port/base — the key the pool uses.
func remoteShareKey(p string) string {
	switch {
	case strings.HasPrefix(p, remoteSMB+"://"):
		rest := strings.TrimPrefix(p, remoteSMB+"://")
		host, tail, hasSlash := strings.Cut(rest, "/")
		if host == "" || !hasSlash {
			return ""
		}
		share, _, _ := strings.Cut(tail, "/")
		if share == "" {
			return ""
		}
		return remoteSMB + "://" + host + "/" + share
	case strings.HasPrefix(p, remoteWebDAV+"://"):
		rest := strings.TrimPrefix(p, remoteWebDAV+"://")
		host, _, _ := strings.Cut(rest, "/")
		if host == "" {
			return ""
		}
		return remoteWebDAV + "://" + host
	}
	return ""
}

// remoteName is the display name of a virtual path: its last segment.
// remoteName is the label a tab or breadcrumb shows. The virtual path stores
// escaped segments, so the name is decoded before it reaches the interface:
// a folder should read 相册, not %E7%9B%B8%E5%86%8C.
func remoteName(p string) string {
	trimmed := strings.TrimSuffix(p, "/")
	if i := strings.LastIndex(trimmed, "/"); i >= 0 && i > strings.Index(trimmed, "://")+2 {
		if name := trimmed[i+1:]; name != "" {
			return unescapeRemoteName(name)
		}
	}
	return unescapeRemoteName(trimmed)
}

// remoteParent is the folder above a virtual path, or "" at a share root.
func remoteParent(p string) string {
	key := remoteShareKey(p)
	if key == "" || p == key {
		return ""
	}
	trimmed := strings.TrimSuffix(p, "/")
	parent := trimmed[:strings.LastIndex(trimmed, "/")]
	if len(parent) < len(key) {
		return key
	}
	return parent
}

// remotePathOnShare turns a virtual path into the path a backend wants, and the
// share it belongs to.
//
// The virtual path escapes each segment (so a name with a space, a "#" or a
// Chinese character survives a URL), but a backend wants the real name: SMB2
// hands the string straight to the server, and a server looks for a folder
// literally called "%E7%9B%B8" if that is what it is given. Decoding happens
// here, at the one boundary between the two forms.
func remotePathOnShare(p string) (key, rel string) {
	key = remoteShareKey(p)
	if key == "" || p == key {
		return key, ""
	}
	rel = strings.TrimPrefix(strings.TrimSuffix(p, "/")[len(key):], "/")
	// A WebDAV share's base path is part of the address, not part of the
	// relative path: the backend already prefixes it. Leaving it in produced
	// /sub/sub for a share whose base is /sub.
	if base := shareBaseForKey(key); base != "" {
		rel = strings.TrimPrefix(strings.TrimPrefix(rel, base), "/")
	}
	return key, unescapeRemoteName(rel)
}

// shareBaseForKey is the base path of the saved share a virtual path belongs
// to, or "" when it has none (SMB, or a WebDAV root).
func shareBaseForKey(key string) string {
	for _, s := range getSettings().RemoteShares {
		if remoteShareKey(remoteRoot(s)) == key {
			return strings.Trim(s.Base, "/")
		}
	}
	return ""
}

// buildRemotePath joins a share root and a relative entry name into a virtual
// path.
func buildRemotePath(root, name string) string {
	enc := &url.URL{Path: name}
	return strings.TrimSuffix(root, "/") + "/" + strings.TrimPrefix(enc.EscapedPath(), "/")
}

// unescapeRemoteName decodes %XX in a virtual path segment.
func unescapeRemoteName(name string) string {
	if decoded, err := url.PathUnescape(name); err == nil {
		return decoded
	}
	return name
}

// --------------------------------------------------------------------- pool

// remoteConn is one open backend with the time it was last used.
type remoteConn struct {
	backend  remoteBackend
	lastUsed time.Time
}

type remotePool struct {
	mu    sync.Mutex
	conns map[string]*remoteConn
}

var remotePoolInstance = &remotePool{conns: map[string]*remoteConn{}}

const remoteIdleTimeout = 90 * time.Second

// remoteListTimeout bounds one directory listing. A share that stopped
// answering should become an error the user can retry, not a spinner that
// never ends.
const remoteListTimeout = 15 * time.Second

// connect returns the backend for a share, dialing it when needed. Passwords
// come from the caller: remembered ones are decrypted by the app, never by the
// pool.
func (p *remotePool) connect(ctx context.Context, share remoteShare, password string) (remoteBackend, error) {
	key := remoteShareID(share)
	p.mu.Lock()
	if c := p.conns[key]; c != nil {
		c.lastUsed = time.Now()
		p.mu.Unlock()
		return c.backend, nil
	}
	p.mu.Unlock()

	var (
		b   remoteBackend
		err error
	)
	switch share.Protocol {
	case remoteSMB:
		b, err = dialRemoteSMB(ctx, share, password)
	case remoteWebDAV:
		b, err = dialRemoteWebDAV(ctx, share, password)
	default:
		err = fmt.Errorf("不支持的协议 %q", share.Protocol)
	}
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	if old := p.conns[key]; old != nil {
		// Another goroutine won the race; keep the first connection.
		_ = b.close()
		old.lastUsed = time.Now()
		p.mu.Unlock()
		return old.backend, nil
	}
	p.conns[key] = &remoteConn{backend: b, lastUsed: time.Now()}
	p.mu.Unlock()
	return b, nil
}

// release closes a connection, for a share the user edited or removed.
func (p *remotePool) release(key string) {
	p.mu.Lock()
	c := p.conns[key]
	delete(p.conns, key)
	p.mu.Unlock()
	if c != nil {
		_ = c.backend.close()
	}
}

// sweepIdle drops connections nobody used for a while, so an idle NAS session
// does not sit open forever.
func (p *remotePool) sweepIdle(now time.Time) {
	p.mu.Lock()
	var stale []*remoteConn
	for key, c := range p.conns {
		if now.Sub(c.lastUsed) > remoteIdleTimeout {
			stale = append(stale, c)
			delete(p.conns, key)
		}
	}
	p.mu.Unlock()
	for _, c := range stale {
		_ = c.backend.close()
	}
}

// ----------------------------------------------------------------- browsing

// shareForPath finds the saved share a virtual path belongs to.
func shareForPath(p string) (remoteShare, error) {
	key := remoteShareKey(p)
	if key == "" {
		return remoteShare{}, errors.New("不是远程位置")
	}
	for _, s := range getSettings().RemoteShares {
		if remoteShareKey(remoteRoot(s)) == key {
			return s, nil
		}
	}
	return remoteShare{}, errors.New("远程位置已不存在")
}

// remoteBackendForPath opens (or reuses) the connection a path needs.
func remoteBackendForPath(ctx context.Context, p string) (remoteBackend, string, error) {
	share, err := shareForPath(p)
	if err != nil {
		return nil, "", err
	}
	password := passwordForShare(share)
	if password == "" && share.User != "" && share.SavePass {
		// The user asked to remember the password and it is gone: the file was
		// cleared, or the profile changed. A protocol-level error would blame
		// the server for something the user can fix in one dialog.
		return nil, "", errRemoteNeedsPassword
	}
	b, err := remotePoolInstance.connect(ctx, share, password)
	if err != nil {
		return nil, "", err
	}
	_, rel := remotePathOnShare(p)
	return b, rel, nil
}

// openRemoteStream opens a remote file for decoding.
func openRemoteStream(ctx context.Context, p string) (io.ReadCloser, error) {
	b, rel, err := remoteBackendForPath(ctx, p)
	if err != nil {
		return nil, err
	}
	return b.open(ctx, rel)
}

// remoteOpenFn is the seam tests replace to read a share without a server,
// the same shape as remoteListFn. Thumbnail decoding goes through it, so the
// prefix path and the full-download path can both be exercised.
var remoteOpenFn = openRemoteStream

// remoteListFn and remoteBackendFn are the seams tests replace, so browsing
// and the media server can be exercised without a server.
var (
	remoteListFn    = remoteList
	remoteBackendFn = remoteBackendForPath
)

// remoteList lists a virtual folder.
func remoteList(ctx context.Context, p string) ([]remoteEntry, error) {
	b, rel, err := remoteBackendFn(ctx, p)
	if err != nil {
		return nil, err
	}
	entries, err := b.list(ctx, rel)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

// openItemFile opens a file for decoding: a local file, or the share's stream
// for a virtual path.
func openItemFile(it *Item) (io.ReadCloser, error) {
	return openPathStream(it.Path)
}

// openPathStream opens a local file or a remote share stream. A remote stream
// keeps its request context alive until it is closed: cancelling when the open
// call returned would abort the download before a single byte was read.
func openPathStream(path string) (io.ReadCloser, error) {
	if !isRemotePath(path) {
		return os.Open(path)
	}
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := remoteOpenFn(ctx, path)
	if err != nil {
		cancel()
		return nil, err
	}
	return &cancellableStream{ReadCloser: stream, cancel: cancel}, nil
}

// cancellableStream releases a remote request when the reader is closed.
type cancellableStream struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancellableStream) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

const (
	// remoteHeadBytes bounds the prefix a remote picture is read from.
	// Dimensions and the EXIF segment both live in the first 128 KiB — the
	// spec caps an APP1 segment at 64 KiB — so half a megabyte per picture was
	// four times the traffic for the same answer.
	remoteHeadBytes = 256 << 10
	// remoteHeadCacheMax is how many prefixes stay in memory. Two remote
	// decodes run at a time and a card's preview is followed by its final
	// within a second, so a handful covers every reader that asks twice.
	remoteHeadCacheMax = 8
)

// The same picture is read for its dimensions, again for its EXIF preview and
// a third time when the final decode starts. The cache turns those three
// requests into one; it holds bytes, not answers, because all three want
// different pieces of the same prefix.
var remoteHeadCache = struct {
	mu    sync.Mutex
	order []string
	items map[string][]byte
}{items: map[string][]byte{}}

// remoteHead reads the start of a remote file: enough for image dimensions and
// the EXIF segment, without pulling a whole photo over the link.
func remoteHead(path string) []byte {
	remoteHeadCache.mu.Lock()
	if head, ok := remoteHeadCache.items[path]; ok {
		remoteHeadCache.mu.Unlock()
		return head
	}
	remoteHeadCache.mu.Unlock()

	head := remoteHeadRead(path)
	if len(head) == 0 {
		// Nothing to remember: a failed request should not be cached as an
		// empty prefix, or a momentary hiccup would blank every later question
		// about this picture.
		return nil
	}
	remoteHeadCache.mu.Lock()
	if _, ok := remoteHeadCache.items[path]; !ok {
		remoteHeadCache.items[path] = head
		remoteHeadCache.order = append(remoteHeadCache.order, path)
		for len(remoteHeadCache.order) > remoteHeadCacheMax {
			oldest := remoteHeadCache.order[0]
			remoteHeadCache.order = remoteHeadCache.order[1:]
			delete(remoteHeadCache.items, oldest)
		}
	}
	remoteHeadCache.mu.Unlock()
	return head
}

// forgetRemoteHead drops a cached prefix when a picture changed on the share.
func forgetRemoteHead(path string) {
	remoteHeadCache.mu.Lock()
	if _, ok := remoteHeadCache.items[path]; ok {
		delete(remoteHeadCache.items, path)
		kept := remoteHeadCache.order[:0]
		for _, p := range remoteHeadCache.order {
			if p != path {
				kept = append(kept, p)
			}
		}
		remoteHeadCache.order = kept
	}
	remoteHeadCache.mu.Unlock()
}

func remoteHeadRead(path string) []byte {
	f, err := openPathStream(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	buf := make([]byte, remoteHeadBytes)
	n, _ := io.ReadFull(f, buf)
	return buf[:n]
}

// errRemoteNeedsPassword asks the user for a password the app does not hold.
var errRemoteNeedsPassword = errors.New("需要重新输入密码：请在侧边栏右键这个远程位置，选择「编辑连接」")

// cleanDisplayPath normalises a path for maps and comparisons. A virtual path
// keeps its scheme and forward slashes; only a local path goes through
// filepath.Clean, which would turn smb://nas/photos into smb:\nas\photos.
func cleanDisplayPath(p string) string {
	// A virtual path keeps its scheme and forward slashes. filepath.Clean
	// would fold a share or the favorites folder into a backslash path, which
	// no lookup recognises.
	if isFavoritesPath(p) {
		// One canonical spelling, so path comparisons and tab restoration
		// agree on which folder this is.
		return favoritesPath
	}
	if isRemotePath(p) {
		return strings.TrimSuffix(p, "/")
	}
	return filepath.Clean(p)
}
