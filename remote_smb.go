package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/cloudsoda/go-smb2"
)

// remoteSMBSession is one SMB2 connection with its mounted share. It lives in
// the app's memory only: no system session, no drive letter, no credential
// entry.
type remoteSMBSession struct {
	session *smb2.Session
	share   *smb2.Share
	// conn is the TCP connection underneath. go-smb2 has no way to drop a
	// session outright, and a server that stopped answering would keep a
	// logoff waiting forever, so the connection is kept for that fallback.
	conn net.Conn
}

func dialRemoteSMB(ctx context.Context, share remoteShare, password string) (remoteBackend, error) {
	host := share.Host
	port := share.Port
	if port <= 0 {
		port = 445
	}
	address := net.JoinHostPort(host, fmt.Sprint(port))
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	d := &smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{
			User:     share.User,
			Password: password,
		},
	}
	var nd net.Dialer
	conn, err := nd.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("连接 %s 失败：%v", address, smbErrorMessage(err))
	}
	session, err := d.DialConn(ctx, conn, address)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("连接 %s 失败：%v", address, smbErrorMessage(err))
	}
	mounted, err := session.Mount(share.Share)
	if err != nil {
		_ = session.Logoff()
		_ = conn.Close()
		return nil, fmt.Errorf("打开共享 %q 失败：%v", share.Share, smbErrorMessage(err))
	}
	return &remoteSMBSession{session: session, share: mounted, conn: conn}, nil
}

// smbErrorMessage keeps the protocol's own words for the cases the user can act
// on, and translates the common ones.
func smbErrorMessage(err error) error {
	if err == nil {
		return nil
	}
	text := err.Error()
	switch {
	case strings.Contains(text, "The user name or password is incorrect"):
		return fmt.Errorf("用户名或密码不正确")
	case strings.Contains(text, "Access is denied"), strings.Contains(text, "STATUS_ACCESS_DENIED"):
		return fmt.Errorf("拒绝访问：账号没有这个共享的权限")
	case strings.Contains(text, "The network path was not found"):
		return fmt.Errorf("找不到网络路径：主机名或地址不对，或者设备不在线")
	case strings.Contains(text, "The network name cannot be found"):
		return fmt.Errorf("找不到共享名：主机能连上，但没有这个共享")
	case strings.Contains(text, "connection refused"), strings.Contains(text, "actively refused"),
		strings.Contains(text, "No connection could be made"):
		return fmt.Errorf("连接被拒绝：对方没有开 SMB 服务，或端口不对")
	case strings.Contains(text, "no such host"):
		return fmt.Errorf("找不到主机，请检查地址")
	case strings.Contains(text, "no route to host"), strings.Contains(text, "i/o timeout"),
		strings.Contains(text, "deadline exceeded"):
		return fmt.Errorf("无法连接到此主机，请检查网络")
	case strings.Contains(text, "account is disabled"):
		return fmt.Errorf("账号已被禁用")
	// go-smb2 surfaces a rejected NTLM handshake as a bare permission error,
	// which reads like a filesystem problem rather than a login one.
	case strings.Contains(text, "permission denied"), strings.Contains(text, "STATUS_LOGON_FAILURE"):
		return fmt.Errorf("登录被拒绝：用户名、密码或域不正确")
	}
	return err
}

func (s *remoteSMBSession) list(ctx context.Context, dir string) ([]remoteEntry, error) {
	name := dir
	if name == "" {
		name = "."
	}
	infos, err := s.share.ReadDir(name)
	if err != nil {
		return nil, smbErrorMessage(err)
	}
	out := make([]remoteEntry, 0, len(infos))
	for _, info := range infos {
		entry := remoteEntry{
			Name:    info.Name(),
			IsDir:   info.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}
		if entry.IsDir {
			entry.Size = 0
		}
		out = append(out, entry)
	}
	return out, nil
}

func (s *remoteSMBSession) open(ctx context.Context, name string) (io.ReadCloser, error) {
	f, err := s.share.Open(name)
	if err != nil {
		return nil, smbErrorMessage(err)
	}
	return f, nil
}

// serveRange reads with ReadAt, so the media server can answer a Range request
// without downloading what comes before it.
func (s *remoteSMBSession) serveHTTP(w http.ResponseWriter, r *http.Request, name string) error {
	f, err := s.share.Open(name)
	if err != nil {
		return smbErrorMessage(err)
	}
	defer f.Close()
	var modified time.Time
	if info, err := f.Stat(); err == nil {
		modified = info.ModTime()
	}
	// ServeContent answers Range, If-Range and HEAD for a seekable file,
	// which is what Media Foundation asks for when it seeks in a video.
	http.ServeContent(w, r, path.Base(name), modified, f)
	return nil
}

func (s *remoteSMBSession) close() error {
	// A logoff is a network round trip, and a server that has stopped
	// answering must not block the pool's sweep or the app's exit. The
	// connection is closed underneath regardless.
	done := make(chan struct{})
	go func() {
		defer close(done)
		if s.share != nil {
			_ = s.share.Umount()
		}
		if s.session != nil {
			_ = s.session.Logoff()
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		if s.conn != nil {
			_ = s.conn.Close()
		}
	}
	return nil
}
