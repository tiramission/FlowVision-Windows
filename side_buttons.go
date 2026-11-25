package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// SideBinding is one shortcut that can trigger a viewer direction. Windows
// mouse side buttons arrive as keyboard events, so a binding keeps both the
// named key and the modifiers held with it (usually none).
type SideBinding struct {
	Key  ui.Key       `json:"key"`
	Mods ui.Modifiers `json:"mods"`
}

func defaultSidePrev() SideBinding { return SideBinding{Key: ui.KeyBack} }
func defaultSideNext() SideBinding { return SideBinding{Key: ui.KeyForward} }

// viewerBindings returns the four configured ways to move through the viewer:
// a short press and a long press for each direction. A missing click binding
// falls back to the side buttons; missing hold bindings stay unset.
func (a *App) viewerBindings() (prevClick, nextClick, prevHold, nextHold SideBinding) {
	return mergeViewerBindings(a.settings.SidePrev, a.settings.SideNext, a.settings.SidePrevHold, a.settings.SideNextHold)
}

// viewerSideShortcuts keeps the keyboard part of the bindings working without
// the native side-button hook. Physical side buttons are handled by
// viewerSideMouse because MyGo reports them only when they are released.
func (a *App) viewerSideShortcuts(c *ui.Context) {
	prev, next, prevHold, nextHold := a.viewerBindings()
	seen := map[SideBinding]bool{}
	for _, b := range []SideBinding{prev, next, prevHold, nextHold} {
		if b.Key == ui.KeyUnknown || seen[b] {
			continue
		}
		if a.sideMouseHook != 0 && (b.Key == ui.KeyBack || b.Key == ui.KeyForward) {
			continue
		}
		seen[b] = true
		binding := b
		c.OnWindowShortcut(binding.Mods, binding.Key, func() {
			a.beginViewerShortcut(binding)
		})
	}
}

func normalizeSideBinding(b, fallback SideBinding) SideBinding {
	if b.Key == ui.KeyUnknown {
		return fallback
	}
	return b
}

// mergeViewerBindings keeps old settings usable and prevents two click
// actions (or two hold actions) from sharing one shortcut. A click and a hold
// may share a key on purpose: that is how one side button gets two actions.
func mergeViewerBindings(prev, next, prevHold, nextHold SideBinding) (SideBinding, SideBinding, SideBinding, SideBinding) {
	prev = normalizeSideBinding(prev, defaultSidePrev())
	next = normalizeSideBinding(next, defaultSideNext())
	if prev == next {
		prev, next = defaultSidePrev(), defaultSideNext()
	}
	if !viewerSideBindingAllowed(prev) {
		prev = defaultSidePrev()
	}
	if !viewerSideBindingAllowed(next) {
		next = defaultSideNext()
	}
	if prevHold.Key != ui.KeyUnknown && !viewerSideBindingAllowed(prevHold) {
		prevHold = SideBinding{}
	}
	if nextHold.Key != ui.KeyUnknown && !viewerSideBindingAllowed(nextHold) {
		nextHold = SideBinding{}
	}
	if prevHold == nextHold {
		nextHold = SideBinding{}
	}
	return prev, next, prevHold, nextHold
}

// viewerSideBindingAllowed rejects keys already used by the viewer. A side
// button usually sends Back or Forward, but a vendor driver may map it to a
// keyboard key; binding a viewer command to that key would run both actions.
func viewerSideBindingAllowed(b SideBinding) bool {
	switch b.Key {
	case ui.KeyUnknown, ui.KeyEscape, ui.KeyEnter, ui.KeyTab, ui.KeySpace,
		ui.KeyLeft, ui.KeyRight, ui.KeyUp, ui.KeyDown,
		ui.KeyF1, ui.KeyF11, ui.KeyI, ui.KeyJ, ui.KeyL, ui.KeyM,
		ui.KeyEqual, ui.KeyMinus, ui.Key0, ui.Key1, ui.KeyC, ui.KeyR:
		return false
	}
	return true
}

func oppositeSideBinding(b SideBinding) SideBinding {
	if b.Key == ui.KeyForward {
		return SideBinding{Key: ui.KeyBack}
	}
	return SideBinding{Key: ui.KeyForward}
}

// assignViewerBinding updates one of the four draft bindings. Click bindings
// move a duplicate click to the opposite default; hold bindings simply clear
// the other hold, since unset is a normal long-press state.
func assignViewerBinding(prev, next, prevHold, nextHold *SideBinding, target string, b SideBinding) {
	switch target {
	case "prevClick":
		*prev = b
		if *next == b {
			*next = oppositeSideBinding(b)
		}
	case "nextClick":
		*next = b
		if *prev == b {
			*prev = oppositeSideBinding(b)
		}
	case "prevHold":
		*prevHold = b
		if *nextHold == b {
			*nextHold = SideBinding{}
		}
	case "nextHold":
		*nextHold = b
		if *prevHold == b {
			*prevHold = SideBinding{}
		}
	}
}

func sideBindingLabel(b SideBinding) string {
	var parts []string
	if b.Mods&ui.Ctrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if b.Mods&ui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if b.Mods&ui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	if b.Mods&ui.Super != 0 {
		parts = append(parts, "Win")
	}
	key := sideKeyLabel(b.Key)
	if key == "" {
		return "未设置"
	}
	if len(parts) == 0 {
		return key
	}
	return strings.Join(append(parts, key), " + ")
}

func sideKeyLabel(key ui.Key) string {
	if key == 0 || key == ui.KeyUnknown {
		return ""
	}
	switch key {
	case ui.KeyBack:
		return "后退键"
	case ui.KeyForward:
		return "前进键"
	case ui.KeyPageUp:
		return "Page Up"
	case ui.KeyPageDown:
		return "Page Down"
	case ui.KeyHome:
		return "Home"
	case ui.KeyEnd:
		return "End"
	case ui.KeyInsert:
		return "Insert"
	case ui.KeyDelete:
		return "Delete"
	case ui.KeyContextMenu:
		return "菜单键"
	}
	if key >= ui.KeyA && key <= ui.KeyZ {
		return string(rune('A' + int(key-ui.KeyA)))
	}
	if key >= ui.Key0 && key <= ui.Key9 {
		return string(rune('0' + int(key-ui.Key0)))
	}
	if key >= ui.KeyF1 && key <= ui.KeyF12 {
		return fmt.Sprintf("F%d", int(key-ui.KeyF1)+1)
	}
	return fmt.Sprintf("按键 %d", key)
}

func sideBindingDirection(b SideBinding) int {
	if b.Key == ui.KeyBack {
		return -1
	}
	if b.Key == ui.KeyForward {
		return 1
	}
	return 0
}
