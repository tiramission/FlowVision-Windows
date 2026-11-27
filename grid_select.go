package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
)

// The grid's context menu is where multi-select lives. A plain click selects
// one tile and nothing else, so the waterfall never grows a selection bar the
// user did not ask for; picking several photos is an explicit menu action.
// This popover is the submenu behind that action, with the same keyboard
// handling as the main menu: arrows move, Escape backs out one level.

func (a *App) openSelectMenu() {
	a.gridMenuOpen = false
	if a.gridMenuX <= 0 && a.gridMenuY <= 0 {
		// Opened without a click position (keyboard path): put it in the
		// content area instead of at the window's origin.
		a.gridMenuX, a.gridMenuY = 320, titleBarHeight+toolbarHeight+80
	}
	a.selectMenuOpen = true
	a.selectMenuFocusFirst = true
	if t := a.tab(); t != nil {
		a.selectMenuTabID = t.ID
	}
}

// selectMenuOverlay draws the selection popover above the grid menu's anchor,
// so opening it from the menu feels like the menu folded into a submenu rather
// than a second, unrelated panel.
func (a *App) selectMenuOverlay(c *ui.Context) {
	if !a.selectMenuOpen {
		return
	}
	tab := a.tab()
	if tab == nil || tab.ID != a.selectMenuTabID {
		a.selectMenuOpen = false
		return
	}
	if a.showSettings || a.showHelp || a.showAbout || a.updateOpen || a.errorMsg != "" ||
		a.renameOpen || a.newFolderOpen || a.confirmDeleteOpen || a.viewer.Open {
		a.selectMenuOpen = false
		return
	}
	ui.Overlay(c, func() {
		windowW, windowH := c.Size()
		// Anchor inside the content area, not at the raw click: the popover
		// clamps itself to the window, and a click near the sidebar edge used
		// to lay the menu over the sidebar instead of over the photos.
		left := a.sidebarLayoutWidth(windowW) + 24
		x := min(max(a.gridMenuX, left), windowW-8)
		y := min(max(a.gridMenuY, titleBarHeight+toolbarHeight+8), windowH-bottomBarHeight-8)
		anchor := ui.Box(c).Key("select-menu-anchor").
			Absolute().Left(x).Top(y).Size(1, 1).PassThrough()
		components.Popover(c, anchor, &a.selectMenuOpen, func(panel ui.Element) {
			panel.Role(ui.RoleMenu).Focusable().MinWidth(200).MaxWidth(300).Padding(4).Gap(1)
			a.buildSelectMenu(c, panel, tab)
		})
	})
}

// selectMenuEntries lists the selection actions. They are built from the tab
// rather than from the context menu's ids: the point of this menu is to change
// the selection, so it acts on the folder, not on what was clicked.
func (a *App) selectMenuEntries(tab *Tab) []gridMenuEntry {
	one := tab.selectedOne()
	if one == nil && tab.Anchor != "" {
		one = tab.byID(tab.Anchor)
	}
	hasAnchor := one != nil
	images, videos, folders := 0, 0, 0
	for _, it := range tab.Items {
		switch it.Kind {
		case "image":
			images++
		case "video":
			videos++
		case "folder":
			folders++
		}
	}
	label := func(name string, n int) string {
		if n == 0 {
			return name
		}
		return name + " (" + itoaCount(n) + ")"
	}
	enterLabel := "进入多选"
	if tab.MultiSelect {
		enterLabel = "退出多选（保留选中）"
	}
	return []gridMenuEntry{
		{label: enterLabel, shortcut: "Ctrl+M", action: func() {
			if tab.MultiSelect {
				a.setMultiSelect(false)
				return
			}
			a.setMultiSelect(true)
		}},
		{divider: true},
		{label: "全选", shortcut: "Ctrl+A", action: func() { a.selectAll() }},
		{label: label("选择全部图片", images), disabled: images == 0, action: func() {
			a.selectKind("image")
		}},
		{label: label("选择全部视频", videos), disabled: videos == 0, action: func() {
			a.selectKind("video")
		}},
		{label: label("选择全部文件夹", folders), disabled: folders == 0, action: func() {
			a.selectKind("folder")
		}},
		{label: "选择同类（与当前项相同）", disabled: !hasAnchor, action: func() { a.selectSameKind() }},
		{label: "反向选择", disabled: len(tab.Items) == 0, action: func() { a.invertSelection() }},
		{divider: true},
		{label: "取消选择", shortcut: "Esc", action: func() { a.exitMultiSelect() }},
	}
}

// selectKind selects every visible item of one kind.
func (a *App) selectKind(kind string) {
	t := a.tab()
	if t == nil {
		return
	}
	t.Selected = map[string]bool{}
	for _, it := range t.Items {
		if it.Kind == kind {
			t.Selected[it.ID] = true
		}
	}
	t.MultiSelect = true
	if len(t.Items) > 0 {
		t.Anchor = t.Items[0].ID
	}
	a.refresh()
}

func (a *App) buildSelectMenu(c *ui.Context, panel ui.Element, tab *Tab) {
	th := c.Theme()
	entries := a.selectMenuEntries(tab)
	var rows []ui.Element
	var enabled []bool
	for _, entry := range entries {
		if entry.divider {
			ui.Box(c).FillWidth().Height(1).Margin(4, 8, 4, 8).
				Background(th.Border).Shrink(0)
			continue
		}
		row := ui.ButtonBase(c).Label(entry.label).Disabled(entry.disabled).
			FillWidth().MinWidth(0).Padding(6, 8).Radius(6).
			Justify(ui.Start).AlignItems(ui.Center).Gap(8).
			Background(ui.Color{}).TextColor(th.Text)
		row.Role(ui.RoleMenuItem)
		switch {
		case entry.disabled:
			row.TextColor(th.TextMuted)
		case row.Hovered() || row.Focused():
			row.Background(th.Text.Alpha(.05))
		}
		row.Children(func() {
			components.Text(c, entry.label).Grow(1).MinWidth(0).
				SingleLine().Ellipsis("…").FontSize(13)
			if entry.shortcut != "" {
				components.Text(c, entry.shortcut).Shrink(0).FontSize(11.5).
					TextColor(th.TextMuted).MarginStart(16)
			}
		})
		if row.Clicked() {
			a.selectMenuOpen = false
			a.gridMenuRefocus = true
			a.gridFocusHandle.Focus(c)
			if entry.action != nil {
				a.later(entry.action)
			}
		}
		rows = append(rows, row)
		enabled = append(enabled, !entry.disabled)
	}

	if a.selectMenuFocusFirst {
		a.selectMenuFocusFirst = false
		for i, row := range rows {
			if enabled[i] {
				row.Focus()
				break
			}
		}
	}

	switch {
	case panel.Shortcut(0, ui.KeyDown):
		moveGridMenuFocus(rows, enabled, 1)
	case panel.Shortcut(0, ui.KeyUp):
		moveGridMenuFocus(rows, enabled, -1)
	case panel.Shortcut(0, ui.KeyHome):
		moveGridMenuFocusTo(rows, enabled, 0, 1)
	case panel.Shortcut(0, ui.KeyEnd):
		moveGridMenuFocusTo(rows, enabled, len(rows)-1, -1)
	case panel.OverlayShortcut(0, ui.KeyEscape):
		// Escape backs out to the grid menu it came from, which is one level
		// up rather than a jump to the grid.
		a.selectMenuOpen = false
		a.reopenGridMenu()
	}
}

// reopenGridMenu restores the folder menu at the same anchor.
func (a *App) reopenGridMenu() {
	a.gridMenuOpen = true
	a.gridMenuFocusFirst = true
	a.gridMenuFocusPanel = false
}

// itoaCount formats a count without pulling strconv into the UI layer.
func itoaCount(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	digits := make([]byte, 0, 6)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
