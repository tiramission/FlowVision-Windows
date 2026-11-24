package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// SideBinding is one physical mouse side button as Windows reports it to the
// window. Side buttons arrive as keyboard events, so a binding keeps both the
// named key and the modifiers held with it (usually none).
type SideBinding struct {
	Key  ui.Key       `json:"key"`
	Mods ui.Modifiers `json:"mods"`
}

func defaultSidePrev() SideBinding { return SideBinding{Key: ui.KeyBack} }
func defaultSideNext() SideBinding { return SideBinding{Key: ui.KeyForward} }

// viewerSideShortcuts registers the two calibrated viewer directions on the
// window, where they work regardless of which canvas control has focus.
func (a *App) viewerSideShortcuts(c *ui.Context) {
	prev, next := mergeSideBindings(a.settings.SidePrev, a.settings.SideNext, defaultSidePrev(), defaultSideNext())
	if prev.Key != ui.KeyUnknown {
		c.OnWindowShortcut(prev.Mods, prev.Key, func() { a.viewerStep(-1) })
	}
	if next.Key != ui.KeyUnknown {
		c.OnWindowShortcut(next.Mods, next.Key, func() { a.viewerStep(1) })
	}
}

func normalizeSideBinding(b, fallback SideBinding) SideBinding {
	if b.Key == ui.KeyUnknown {
		return fallback
	}
	return b
}

// mergeSideBindings keeps the two viewer directions usable after an older or
// hand-edited settings file, and never lets both directions share one key.
func mergeSideBindings(prev, next, defPrev, defNext SideBinding) (SideBinding, SideBinding) {
	prev = normalizeSideBinding(prev, defPrev)
	next = normalizeSideBinding(next, defNext)
	if prev == next {
		prev, next = defPrev, defNext
	}
	return prev, next
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

// applySideBinding assigns a captured key to one direction and keeps the
// other direction on a different key.
func applySideBinding(prev, next *SideBinding, target string, b SideBinding) {
	if target == "prev" {
		*prev = b
		if *next == b {
			if b.Key == ui.KeyForward {
				*next = SideBinding{Key: ui.KeyBack}
			} else {
				*next = SideBinding{Key: ui.KeyForward}
			}
		}
		return
	}
	*next = b
	if *prev == b {
		if b.Key == ui.KeyBack {
			*prev = SideBinding{Key: ui.KeyForward}
		} else {
			*prev = SideBinding{Key: ui.KeyBack}
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
