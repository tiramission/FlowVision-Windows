package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/transfer"
)

// checkForUpdates opens the updater's own window. The native menu is only
// reachable with Alt after the app hides the system title bar, so the title
// bar, Settings and About call it directly.
func (a *App) checkForUpdates() {
	a.later(updater.CheckForUpdates)
}

// AppSvc collects the app-level actions the native UI calls.
type AppSvc struct{}

// PickFolder opens the system folder picker and returns the chosen folder.
func (AppSvc) PickFolder(start string) (string, error) {
	if start == "" {
		start = startPath
	}
	list, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:      mainWin,
		Title:       "选择文件夹",
		DefaultPath: start,
		Directory:   true,
		ButtonLabel: "打开",
	})
	if err != nil {
		return "", err
	}
	if len(list) == 0 {
		return "", nil
	}
	return list[0], nil
}

// Reveal selects path in Explorer.
func (AppSvc) Reveal(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	mygo.Shell.ShowItemInFolder(path)
	return nil
}

// OpenFolder opens path in Explorer.
func (AppSvc) OpenFolder(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return errors.New("not a folder")
	}
	return mygo.Shell.OpenPath(path)
}

// OpenExternal opens a file with its default application.
func (AppSvc) OpenExternal(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	return mygo.Shell.OpenPath(path)
}

// CopyText copies text to the Windows clipboard from the native side.
func (AppSvc) CopyText(text string) error {
	// MyGo's clipboard handles the system clipboard protocols and clipboard
	// managers; the Win32 fallback covers a failed native write.
	if err := mygo.Clipboard.Write(transfer.TextData(text)); err == nil {
		return nil
	}
	return setClipboardText(text)
}

// ResolveDrop turns a dropped folder or file into the folder to open.
func (AppSvc) ResolveDrop(path string) (string, error) {
	path = strings.Trim(path, `"`)
	st, err := os.Stat(path)
	if err != nil {
		// Text drag sources may surround the path with whitespace; only fall
		// back to the trimmed form when the exact path cannot be opened.
		path = strings.TrimSpace(path)
		st, err = os.Stat(path)
	}
	if err != nil {
		return "", err
	}
	if st.IsDir() {
		return path, nil
	}
	return filepath.Dir(path), nil
}

// Delete moves path to the Recycle Bin, or deletes it for good.
func (AppSvc) Delete(path string, permanent bool) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !permanent {
		// Never fall back to a permanent delete when the user asked for the
		// Recycle Bin: losing the file silently is worse than showing an error.
		if err := recycle(path); err != nil {
			return fmt.Errorf("无法移到回收站：%w", err)
		}
		forgetDims(path)
		invalidateMedia(filepath.Dir(path))
		return nil
	}
	if st.IsDir() {
		// Removing a junction with RemoveAll can walk into its target. A
		// junction is a reparse point even when Lstat does not report a symlink.
		if lst, lerr := os.Lstat(path); (lerr == nil && lst.Mode()&os.ModeSymlink != 0) || isReparsePoint(path) {
			err = os.Remove(path)
		} else {
			err = os.RemoveAll(path)
		}
	} else {
		err = os.Remove(path)
	}
	if err == nil {
		forgetDims(path)
		invalidateMedia(filepath.Dir(path))
	}
	return err
}

// checkName rejects names Windows cannot create reliably. TrimSpace happens
// before this call, so trailing spaces are already gone.
func checkName(name string) error {
	if name == "" || strings.ContainsAny(name, `\/:*?"<>|`) {
		return errors.New("名称不能包含 \\ / : * ? \" < > |")
	}
	if name == "." || name == ".." {
		return errors.New("名称无效")
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return errors.New("名称不能以空格或句点结尾")
	}
	for _, r := range name {
		if r < 32 {
			return errors.New("名称包含控制字符")
		}
	}
	if utf8.RuneCountInString(name) > 255 || len(utf16.Encode([]rune(name))) > 255 {
		return errors.New("名称过长")
	}
	// CON, PRN, AUX, NUL, COM1..COM9 and LPT1..LPT9 are reserved even when
	// they carry an extension (CON.txt is still the console device).
	base := strings.TrimSuffix(name, filepath.Ext(name))
	switch strings.ToUpper(base) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
		"COM¹", "COM²", "COM³", "LPT¹", "LPT²", "LPT³":
		return errors.New("这是 Windows 保留名称")
	}
	return nil
}

// Rename gives path a new name in its own folder and returns the new path.
func (AppSvc) Rename(path, newName string) (string, error) {
	newName = strings.TrimSpace(newName)
	if err := checkName(newName); err != nil {
		return "", err
	}
	dst := filepath.Join(filepath.Dir(path), newName)
	if strings.EqualFold(dst, path) {
		// A case-only rename needs a detour: Windows sees the destination as
		// existing even when only the capitalization changed.
		if dst == path {
			return path, nil
		}
		tmp := filepath.Join(filepath.Dir(path), fmt.Sprintf(".fvrename-%d", time.Now().UnixNano()))
		if err := os.Rename(path, tmp); err != nil {
			return "", err
		}
		if err := os.Rename(tmp, dst); err != nil {
			_ = os.Rename(tmp, path)
			return "", err
		}
		forgetDims(path)
		invalidateMedia(filepath.Dir(path))
		return dst, nil
	}
	if _, err := os.Stat(dst); err == nil {
		return "", errors.New("同名文件已存在")
	}
	if err := os.Rename(path, dst); err != nil {
		return "", err
	}
	forgetDims(path)
	invalidateMedia(filepath.Dir(path))
	invalidateMedia(filepath.Dir(dst))
	return dst, nil
}

// NewFolder creates a folder inside dir.
func (AppSvc) NewFolder(dir, name string) (string, error) {
	name = strings.TrimSpace(name)
	if err := checkName(name); err != nil {
		return "", err
	}
	p := filepath.Join(dir, name)
	if err := os.Mkdir(p, 0o755); err != nil {
		return "", err
	}
	return p, nil
}

// Quit ends the app.
func (AppSvc) Quit() { mygo.App.Quit() }

// FormatTime renders an mtime for the info panel.
func (AppSvc) FormatTime(unix int64) string {
	return time.Unix(unix, 0).Format("2006-01-02 15:04:05")
}

// Version returns the app version.
func (AppSvc) Version() string { return appVersion() }

// Log writes a line to the app's log, for diagnostics from the page.
func (AppSvc) Log(msg string) { logf("page: %s", msg) }
