package main

import (
	"errors"
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

const (
	fileAttributeHidden       = 0x2
	fileAttributeReparsePoint = 0x400
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procSetClipboardData = user32.NewProc("SetClipboardData")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	procGlobalLock       = kernel32.NewProc("GlobalLock")
	procGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
	procGlobalFree       = kernel32.NewProc("GlobalFree")
	procRtlMoveMemory    = kernel32.NewProc("RtlMoveMemory")
)

func procErr(err error) error {
	if e, ok := err.(syscall.Errno); ok && e != 0 {
		return e
	}
	return nil
}

type win32Point struct {
	X int32
	Y int32
}

// cursorPosition reads the global mouse position. Viewer chrome auto-hide
// only needs to know whether the pointer moved; screen coordinates avoid
// mixing DPI-scaled client coordinates with the physical cursor.
func cursorPosition() (int32, int32, bool) {
	var p win32Point
	r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return p.X, p.Y, r != 0
}

// setClipboardText puts UTF-16 text on the Windows clipboard. The WebView's
// navigator.clipboard is unavailable on the custom app scheme, so copy actions
// go through this instead of failing silently.
func setClipboardText(text string) error {
	const (
		cfUnicodeText = 13
		gmemMoveable  = 0x0042
	)
	var opened bool
	for i := 0; i < 5; i++ {
		r, _, _ := procOpenClipboard.Call(0)
		if r != 0 {
			opened = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !opened {
		return errors.New("无法打开剪贴板")
	}
	defer procCloseClipboard.Call()

	if _, _, err := procEmptyClipboard.Call(); procErr(err) != nil {
		return fmt.Errorf("清空剪贴板失败：%w", procErr(err))
	}
	w, err := syscall.UTF16FromString(text)
	if err != nil {
		return fmt.Errorf("文本无法编码：%w", err)
	}
	size := uintptr(len(w) * 2)
	h, _, err := procGlobalAlloc.Call(gmemMoveable, size)
	if h == 0 {
		return fmt.Errorf("分配剪贴板内存失败：%w", procErr(err))
	}
	ptr, _, err := procGlobalLock.Call(h)
	if ptr == 0 {
		procGlobalFree.Call(h)
		return fmt.Errorf("锁定剪贴板内存失败：%w", procErr(err))
	}
	// RtlMoveMemory copies the UTF-16 string into the global block without
	// converting the kernel pointer back into a Go pointer, keeping the
	// syscall boundary vet-clean.
	procRtlMoveMemory.Call(ptr, uintptr(unsafe.Pointer(&w[0])), size)
	procGlobalUnlock.Call(h)
	if r, _, err := procSetClipboardData.Call(cfUnicodeText, h); r == 0 {
		procGlobalFree.Call(h)
		return fmt.Errorf("写入剪贴板失败：%w", procErr(err))
	}
	return nil
}

// isHidden reports whether path carries Windows' hidden attribute.
func isHidden(path string) bool {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	attrs, err := syscall.GetFileAttributes(p)
	if err != nil {
		return false
	}
	return attrs&fileAttributeHidden != 0
}

// isReparsePoint reports whether path is a junction, symlink or another
// reparse point. Removing the link itself must never remove its target.
func isReparsePoint(path string) bool {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	attrs, err := syscall.GetFileAttributes(p)
	if err != nil {
		return false
	}
	return attrs&fileAttributeReparsePoint != 0
}

// recycle deletes path into the Recycle Bin, as Explorer does.
func recycle(path string) error {
	var (
		shell32             = syscall.NewLazyDLL("shell32.dll")
		procSHFileOperation = shell32.NewProc("SHFileOperationW")
	)
	const (
		foDelete           = 0x0003
		fofSilent          = 0x0004
		fofNoConfirmation  = 0x0010
		fofAllowUndo       = 0x0040
		fofNoErrorUI       = 0x0400
		fofWantNukeWarning = 0x4000
	)

	from, err := syscall.UTF16FromString(path)
	if err != nil {
		return err
	}
	// SHFileOperation wants a double-NUL terminated list.
	from = append(from, 0)

	op := shFileOpStruct{
		wFunc:  foDelete,
		pFrom:  &from[0],
		fFlags: fofAllowUndo | fofNoConfirmation | fofSilent | fofNoErrorUI | fofWantNukeWarning,
	}
	ret, _, _ := procSHFileOperation.Call(uintptr(unsafe.Pointer(&op)))
	if ret != 0 {
		return syscall.Errno(ret)
	}
	if op.fAnyOperationsAborted != 0 {
		return syscall.EINVAL
	}
	return nil
}

// shFileOpStruct mirrors SHFILEOPSTRUCTW; the padding keeps the C layout.
type shFileOpStruct struct {
	hwnd                  uintptr
	wFunc                 uint32
	_pad0                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	_pad1                 uint16
	fAnyOperationsAborted int32
	_pad2                 uint32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}
