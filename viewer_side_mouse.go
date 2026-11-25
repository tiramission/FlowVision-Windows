package main

import (
	"syscall"
	"time"
	"unsafe"

	"github.com/egoist/mygo/ui"
)

const (
	whMouseLL     = 14
	wmXButtonDown = 0x020B
	wmXButtonUp   = 0x020C
	wmXButtonDbl  = 0x020D
)

type msllHookStruct struct {
	Pt          win32Point
	MouseData   uint32
	Flags       uint32
	Time        uint32
	_pad        uint32
	DwExtraInfo uintptr
}

var (
	mouseHookCallback = syscall.NewCallback(viewerSideMouseHookProc)
	sideMouseSink     func(button int, down bool)
)

func viewerSideMouseHookProc(nCode, wParam uintptr, lParam *msllHookStruct) uintptr {
	if int32(nCode) >= 0 {
		switch wParam {
		case wmXButtonDown, wmXButtonUp, wmXButtonDbl:
			button := int(lParam.MouseData >> 16)
			if (button == 1 || button == 2) && sideMouseSink != nil {
				sideMouseSink(button, wParam != wmXButtonUp)
			}
		}
	}
	ret, _, _ := procCallNextHookEx.Call(0, nCode, wParam, uintptr(unsafe.Pointer(lParam)))
	return ret
}

// installViewerSideMouseHook must run on the UI thread: a low-level mouse hook
// is delivered through the message loop of the thread that installed it.
func (a *App) installViewerSideMouseHook() {
	if a.win == nil || a.sideMouseHook != 0 {
		return
	}
	sideMouseSink = a.viewerSideMouseEvent
	// WH_MOUSE_LL receives input for every process, so the hook procedure's
	// owning module has to be named even though the callback lives in this exe.
	module, _, _ := procGetModuleHandleW.Call(0)
	hook, _, _ := procSetWindowsHookEx.Call(whMouseLL, mouseHookCallback, module, 0)
	if hook == 0 {
		sideMouseSink = nil
		return
	}
	a.sideMouseHook = hook
}

func (a *App) stopViewerSideMouseHook() {
	if a.sideMouseHook != 0 {
		procUnhookWindowsHook.Call(a.sideMouseHook)
		a.sideMouseHook = 0
	}
	sideMouseSink = nil
	for i := range a.sideMouseTimer {
		if a.sideMouseTimer[i] != nil {
			a.sideMouseTimer[i].Stop()
			a.sideMouseTimer[i] = nil
		}
	}
	a.sideMouseDown = [2]bool{}
	a.sideMouseHold = [2]bool{}
}

func (a *App) viewerBindingDirs(b SideBinding) (clickDir, holdDir int) {
	prev, next, prevHold, nextHold := a.viewerBindings()
	switch b {
	case prev:
		clickDir = -1
	case next:
		clickDir = 1
	}
	switch b {
	case prevHold:
		holdDir = -1
	case nextHold:
		holdDir = 1
	}
	return clickDir, holdDir
}

func bindingVirtualKey(b SideBinding) int {
	switch b.Key {
	case ui.KeyBack:
		// Browser Back, not Backspace: KeyBack is the history key that
		// keyboards with side navigation keys send.
		return 0xA6
	case ui.KeyForward:
		return 0xA5
	case ui.KeyPageUp:
		return 0x21
	case ui.KeyPageDown:
		return 0x22
	case ui.KeyHome:
		return 0x24
	case ui.KeyEnd:
		return 0x23
	case ui.KeyInsert:
		return 0x2D
	case ui.KeyDelete:
		return 0x2E
	case ui.KeyContextMenu:
		return 0x5D
	case ui.KeySpace:
		return 0x20
	case ui.KeyEscape:
		return 0x1B
	case ui.KeyTab:
		return 0x09
	case ui.KeyEnter:
		return 0x0D
	case ui.KeyLeft:
		return 0x25
	case ui.KeyUp:
		return 0x26
	case ui.KeyRight:
		return 0x27
	case ui.KeyDown:
		return 0x28
	case ui.KeyMinus:
		return 0xBD
	case ui.KeyEqual:
		return 0xBB
	}
	switch {
	case b.Key >= ui.KeyA && b.Key <= ui.KeyZ:
		return int('A' + (b.Key - ui.KeyA))
	case b.Key >= ui.Key0 && b.Key <= ui.Key9:
		return int('0' + (b.Key - ui.Key0))
	case b.Key >= ui.KeyF1 && b.Key <= ui.KeyF12:
		return 0x70 + int(b.Key-ui.KeyF1)
	}
	return 0
}

func bindingModsMatch(b SideBinding) bool {
	return (b.Mods&ui.Ctrl != 0) == asyncKeyDown(0x11) &&
		(b.Mods&ui.Alt != 0) == asyncKeyDown(0x12) &&
		(b.Mods&ui.Shift != 0) == asyncKeyDown(0x10) &&
		(b.Mods&ui.Super != 0) == (asyncKeyDown(0x5B) || asyncKeyDown(0x5C))
}

func bindingHeld(b SideBinding) bool {
	vk := bindingVirtualKey(b)
	return vk != 0 && asyncKeyDown(vk) && bindingModsMatch(b)
}

func (a *App) viewerCanAct() bool {
	if !a.viewer.Open {
		return false
	}
	// Overlays sit above the viewer: a side button pressed there is either
	// recording a shortcut or operating the overlay, never navigating the
	// image behind it.
	if a.showSettings || a.showHelp || a.showAbout || a.updateOpen || a.bindingCapture != "" {
		return false
	}
	return a.win == nil || windowForeground(a.win.NativeHandle())
}

func (a *App) fireViewerDirection(dir int) {
	if dir == 0 || !a.viewerCanAct() {
		return
	}
	a.viewerStep(dir)
}

// beginViewerShortcut handles a keyboard binding. A binding with only a click
// acts at once; one with a hold waits either for release or the hold delay.
// A watcher ignores auto-repeat until the key is released.
func (a *App) beginViewerShortcut(b SideBinding) {
	if !a.viewerCanAct() {
		return
	}
	if a.viewerKeysDown == nil {
		a.viewerKeysDown = map[SideBinding]bool{}
	}
	if a.viewerKeysDown[b] {
		return
	}
	clickDir, holdDir := a.viewerBindingDirs(b)
	if clickDir == 0 && holdDir == 0 {
		return
	}
	a.viewerKeysDown[b] = true
	if clickDir != 0 && holdDir == 0 {
		a.fireViewerDirection(clickDir)
	}
	go a.watchViewerShortcut(b, clickDir, holdDir)
}

func (a *App) watchViewerShortcut(b SideBinding, clickDir, holdDir int) {
	defer a.update(func() { delete(a.viewerKeysDown, b) })
	if bindingVirtualKey(b) == 0 {
		// Without a virtual-key mapping only the immediate click is possible.
		if holdDir != 0 {
			time.Sleep(viewerHoldDelay)
			a.update(func() { a.fireViewerDirection(holdDir) })
		}
		return
	}
	start := time.Now()
	deadline := start.Add(5 * time.Second)
	holdFired := false
	for {
		held := bindingHeld(b)
		now := time.Now()
		if !held || now.After(deadline) {
			if !holdFired && clickDir != 0 {
				a.update(func() { a.fireViewerDirection(clickDir) })
			}
			return
		}
		if !holdFired && holdDir != 0 && now.Sub(start) >= viewerHoldDelay {
			holdFired = true
			a.update(func() { a.fireViewerDirection(holdDir) })
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (a *App) viewerSideMouseEvent(button int, down bool) {
	if button != 1 && button != 2 {
		return
	}
	if !a.viewerCanAct() {
		if !down {
			a.cancelSideMousePress(button)
		}
		return
	}
	base := SideBinding{Key: ui.KeyBack}
	if button == 2 {
		base = SideBinding{Key: ui.KeyForward}
	}
	clickDir, holdDir := a.viewerBindingDirs(base)
	if clickDir == 0 && holdDir == 0 {
		return
	}
	idx := button - 1
	if down {
		if a.sideMouseDown[idx] {
			return
		}
		a.sideMouseDown[idx] = true
		a.sideMouseAt[idx] = time.Now()
		a.sideMouseHold[idx] = false
		if holdDir != 0 {
			button := button
			a.sideMouseTimer[idx] = time.AfterFunc(viewerHoldDelay, func() {
				a.update(func() { a.fireSideMouseHold(button) })
			})
		}
		return
	}
	if !a.sideMouseDown[idx] {
		return
	}
	a.sideMouseDown[idx] = false
	if a.sideMouseTimer[idx] != nil {
		a.sideMouseTimer[idx].Stop()
		a.sideMouseTimer[idx] = nil
	}
	if !a.sideMouseHold[idx] && clickDir != 0 {
		a.fireViewerDirection(clickDir)
	}
	a.sideMouseHold[idx] = false
}

func (a *App) fireSideMouseHold(button int) {
	idx := button - 1
	if !a.sideMouseDown[idx] {
		return
	}
	base := SideBinding{Key: ui.KeyBack}
	if button == 2 {
		base = SideBinding{Key: ui.KeyForward}
	}
	_, holdDir := a.viewerBindingDirs(base)
	if holdDir == 0 {
		return
	}
	a.sideMouseHold[idx] = true
	a.sideMouseTimer[idx] = nil
	a.fireViewerDirection(holdDir)
}

func (a *App) cancelSideMousePress(button int) {
	idx := button - 1
	a.sideMouseDown[idx] = false
	a.sideMouseHold[idx] = false
	if a.sideMouseTimer[idx] != nil {
		a.sideMouseTimer[idx].Stop()
		a.sideMouseTimer[idx] = nil
	}
}
