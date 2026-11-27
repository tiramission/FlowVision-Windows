package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/egoist/mygo"
)

// Remembered passwords are encrypted with DPAPI, which only this Windows user
// can decrypt, and kept in a separate file so settings.json stays readable. The
// app never writes to Credential Manager: the point of connecting in-process is
// that no other program sees the credentials.

var (
	credCrypt32           = syscall.NewLazyDLL("crypt32.dll")
	procCryptProtectData  = credCrypt32.NewProc("CryptProtectData")
	procCryptUnprotect    = credCrypt32.NewProc("CryptUnprotectData")
	procLocalFree         = kernel32.NewProc("LocalFree")
	credentialFileMu      sync.Mutex
	remotePasswordMu      sync.Mutex
	remotePasswordCache   = map[string]string{}
	remoteCredentialsName = remoteCredFile
	// remoteCredDirOverride points the credential file at a temp folder in
	// tests, so a test never touches the real profile.
	remoteCredDirOverride string
)

// remoteCredFile is the credential file name inside the app's data folder.
const remoteCredFile = "remote-credentials.json"

type dataBlob struct {
	size uint32
	data *byte
}

func blobFromBytes(b []byte) dataBlob {
	if len(b) == 0 {
		return dataBlob{}
	}
	return dataBlob{size: uint32(len(b)), data: &b[0]}
}

func (b dataBlob) bytes() []byte {
	if b.data == nil || b.size == 0 {
		return nil
	}
	return unsafe.Slice(b.data, b.size)
}

// protectSecret encrypts plain with DPAPI for the current user.
func protectSecret(plain string) (string, error) {
	in := blobFromBytes([]byte(plain))
	var out dataBlob
	ret, _, _ := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0,
		0x1, // CRYPTPROTECT_UI_FORBIDDEN
		uintptr(unsafe.Pointer(&out)),
	)
	if ret == 0 {
		return "", fmt.Errorf("无法加密密码（DPAPI）")
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.data)))
	return base64.StdEncoding.EncodeToString(out.bytes()), nil
}

// unprotectSecret decrypts what protectSecret produced.
func unprotectSecret(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	in := blobFromBytes(raw)
	var out dataBlob
	ret, _, _ := procCryptUnprotect.Call(
		uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0,
		0x1, // CRYPTPROTECT_UI_FORBIDDEN
		uintptr(unsafe.Pointer(&out)),
	)
	if ret == 0 {
		return "", fmt.Errorf("无法解密已保存的密码")
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.data)))
	return string(out.bytes()), nil
}

func remoteCredentialsPath() string {
	if remoteCredDirOverride != "" {
		return filepath.Join(remoteCredDirOverride, remoteCredFile)
	}
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return ""
	}
	return filepath.Join(dir, remoteCredentialsName)
}

func loadRemoteCredentials() map[string]string {
	path := remoteCredentialsPath()
	if path == "" {
		return map[string]string{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	out := map[string]string{}
	if json.Unmarshal(data, &out) != nil {
		return map[string]string{}
	}
	return out
}

func saveRemoteCredentials(creds map[string]string) {
	path := remoteCredentialsPath()
	if path == "" {
		return
	}
	credentialFileMu.Lock()
	defer credentialFileMu.Unlock()
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".remote-credentials-*.tmp")
	if err != nil {
		return
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(name)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return
	}
	_ = os.Rename(name, path)
}

// clearRemoteCredCache drops the decrypted passwords, as a restart would.
func clearRemoteCredCache() {
	remotePasswordMu.Lock()
	remotePasswordCache = map[string]string{}
	remotePasswordMu.Unlock()
}

// passwordForShare returns the password for a share: the one entered this
// session, or the remembered one, decrypted on demand.
func passwordForShare(share remoteShare) string {
	id := remoteShareID(share)
	remotePasswordMu.Lock()
	if pass, ok := remotePasswordCache[id]; ok {
		remotePasswordMu.Unlock()
		return pass
	}
	remotePasswordMu.Unlock()
	if !share.SavePass {
		return ""
	}
	encrypted := loadRemoteCredentials()[id]
	if encrypted == "" {
		return ""
	}
	pass, err := unprotectSecret(encrypted)
	if err != nil {
		logf("remote credentials: %v", err)
		return ""
	}
	remotePasswordMu.Lock()
	remotePasswordCache[id] = pass
	remotePasswordMu.Unlock()
	return pass
}

// rememberRemotePassword keeps a password for this session and, when asked,
// encrypts it for the next one.
func rememberRemotePassword(share remoteShare, password string, remember bool) {
	id := remoteShareID(share)
	remotePasswordMu.Lock()
	remotePasswordCache[id] = password
	remotePasswordMu.Unlock()
	creds := loadRemoteCredentials()
	if remember {
		if encrypted, err := protectSecret(password); err == nil {
			creds[id] = encrypted
		} else {
			logf("remote credentials: %v", err)
		}
	} else {
		delete(creds, id)
	}
	saveRemoteCredentials(creds)
}

// forgetRemotePassword drops a remembered password with the share.
func forgetRemotePassword(share remoteShare) {
	id := remoteShareID(share)
	remotePasswordMu.Lock()
	delete(remotePasswordCache, id)
	remotePasswordMu.Unlock()
	creds := loadRemoteCredentials()
	if _, ok := creds[id]; ok {
		delete(creds, id)
		saveRemoteCredentials(creds)
	}
}

// sweepRemoteConnections closes connections that have been idle for a while.
func sweepRemoteConnections() {
	for {
		time.Sleep(30 * time.Second)
		remotePoolInstance.sweepIdle(time.Now())
	}
}

func initRemoteConnections() {
	go sweepRemoteConnections()
}

// randomToken is the media server's URL secret: a random value per run, so a
// page or another process cannot guess a video URL.
func randomToken() string {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

// passwordForID reads a remembered password by share id, for the case where the
// share being edited no longer produces that id (its address changed).
func passwordForID(id string, remember bool) string {
	remotePasswordMu.Lock()
	if pass, ok := remotePasswordCache[id]; ok {
		remotePasswordMu.Unlock()
		return pass
	}
	remotePasswordMu.Unlock()
	if !remember {
		return ""
	}
	encrypted := loadRemoteCredentials()[id]
	if encrypted == "" {
		return ""
	}
	pass, err := unprotectSecret(encrypted)
	if err != nil {
		logf("remote credentials: %v", err)
		return ""
	}
	remotePasswordMu.Lock()
	remotePasswordCache[id] = pass
	remotePasswordMu.Unlock()
	return pass
}

// forgetRemotePasswordByID drops a remembered password by share id.
func forgetRemotePasswordByID(id string) {
	remotePasswordMu.Lock()
	delete(remotePasswordCache, id)
	remotePasswordMu.Unlock()
	creds := loadRemoteCredentials()
	if _, ok := creds[id]; !ok {
		return
	}
	delete(creds, id)
	saveRemoteCredentials(creds)
}
