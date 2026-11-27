package main

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Media Foundation cannot open smb:// or a WebDAV URL with the credentials the
// app holds, but it plays http:// happily, Range requests included. The app
// therefore serves remote video to its own player from a listener on loopback:
// a random port, a random path token, and it shuts down again when nothing has
// played for a few minutes. Nothing is registered with the system.

const mediaServerIdle = 3 * time.Minute

type appMediaServer struct {
	mu    sync.Mutex
	srv   *http.Server
	ln    net.Listener
	token string
	timer *time.Timer
}

var remoteMediaServer = &appMediaServer{}

// URL is the loopback URL for a remote video, starting the server on first use.
// It takes no App: thumbnail capture asks for a URL from a worker goroutine, and
// the server needs nothing from the UI.
func (m *appMediaServer) URL(virtual string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.token == "" {
		m.token = randomToken()
	}
	if m.srv == nil {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return "", fmt.Errorf("无法启动本地播放服务：%v", err)
		}
		m.ln = ln
		m.srv = &http.Server{Handler: m, ReadHeaderTimeout: 10 * time.Second}
		go func() {
			_ = m.srv.Serve(ln)
		}()
	}
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	addr := m.ln.Addr().String()
	return "http://" + addr + "/m/" + m.token + "/" + url.PathEscape(virtual), nil
}

// touch restarts the idle timer after a request.
func (m *appMediaServer) touch() {
	if m.timer != nil {
		m.timer.Stop()
	}
	m.timer = time.AfterFunc(mediaServerIdle, m.stop)
}

// stop closes the listener: no video is playing, and a port that is not needed
// should not be open.
func (m *appMediaServer) stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.srv == nil {
		return
	}
	_ = m.srv.Close()
	if m.ln != nil {
		_ = m.ln.Close()
	}
	m.srv = nil
	m.ln = nil
	m.token = ""
}

func (m *appMediaServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	prefix := "/m/" + m.token + "/"
	m.mu.Unlock()
	if m.token == "" || !strings.HasPrefix(r.URL.Path, prefix) {
		http.NotFound(w, r)
		return
	}
	raw, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, prefix))
	if err != nil || !isRemotePath(raw) {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	m.touch()
	backend, rel, err := remoteBackendFn(r.Context(), raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if err := backend.serveHTTP(w, r, rel); err != nil {
		// Headers are usually already sent by now; the player reports its own
		// error and the viewer shows it.
		logf("media: %s: %v", raw, err)
	}
}
