package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
)

// gridMenuEntry is one row of the grid's context menu. Divider rows render
// the same hairline as Onevra's menu separators.
type gridMenuEntry struct {
	label    string
	shortcut string
	disabled bool
	divider  bool
	// write marks an entry that changes files; a network tab hides those.
	write bool
	// localOnly marks an entry that hands a path to Windows. Nothing outside
	// the app can reach a remote file, so a network tab hides those too.
	localOnly bool
	action    func()
}

// resolveGridContextAt selects the tile under a secondary click and records
// which items the menu acts on. A click in the empty area keeps the existing
// selection but opens the folder menu.
func (a *App) resolveGridContextAt(tab *Tab, layout gridLayout, x, y float32) {
	a.contextIDs = nil
	if idx, ok := layout.hit(x, y); ok {
		it := layout.Cells[idx].It
		if !tab.Selected[it.ID] {
			tab.Selected = map[string]bool{it.ID: true}
			tab.Anchor = it.ID
		}
		a.contextEmpty = false
	} else {
		a.contextEmpty = true
	}
	for id := range tab.Selected {
		a.contextIDs = append(a.contextIDs, id)
	}
}

// resolveGridContextSelection prepares the menu opened from the keyboard,
// where the current selection is the target.
func (a *App) resolveGridContextSelection(tab *Tab) {
	a.contextIDs = nil
	for id := range tab.Selected {
		a.contextIDs = append(a.contextIDs, id)
	}
	a.contextEmpty = len(tab.Selected) == 0
}

// gridMenuKeyboardAnchor places the keyboard menu beside the first selected
// tile, or at the top of the grid when nothing is selected.
func (a *App) gridMenuKeyboardAnchor(tab *Tab, layout gridLayout, bounds ui.Rect) (float32, float32) {
	for _, cell := range layout.Cells {
		if tab.Selected[cell.It.ID] {
			return bounds.X + cell.X + 8, bounds.Y + cell.Y + cell.height()/2
		}
	}
	return bounds.X + 16, bounds.Y + 20
}

func (a *App) openGridMenu(x, y float32, keyboard bool) {
	a.gridMenuOpen = true
	a.tasksOpen = false
	a.gridMenuX, a.gridMenuY = x, y
	a.gridMenuFocusFirst = keyboard
	a.gridMenuFocusPanel = !keyboard
	if tab := a.tab(); tab != nil {
		a.gridMenuTabID = tab.ID
	}
}

// gridMenuOverlay draws the menu in the window-level overlay. Anchoring it
// to a 1×1 point at the click keeps the popover's collision handling close
// to the pointer and lets its glass material paint outside the scroll clip.
func (a *App) gridMenuOverlay(c *ui.Context) {
	if !a.gridMenuOpen {
		return
	}
	tab := a.tab()
	if tab == nil || tab.ID != a.gridMenuTabID {
		a.gridMenuOpen = false
		return
	}
	if a.showSettings || a.showHelp || a.showAbout || a.updateOpen || a.errorMsg != "" ||
		a.renameOpen || a.newFolderOpen || a.confirmDeleteOpen || a.viewer.Open {
		a.gridMenuOpen = false
		return
	}
	ui.Overlay(c, func() {
		windowW, windowH := c.Size()
		x := min(max(a.gridMenuX, 8), windowW-8)
		y := min(max(a.gridMenuY, titleBarHeight+toolbarHeight+8), windowH-bottomBarHeight-8)
		anchor := ui.Box(c).Key("grid-menu-anchor").
			Absolute().Left(x).Top(y).Size(1, 1).PassThrough()
		components.Popover(c, anchor, &a.gridMenuOpen, func(panel ui.Element) {
			panel.Role(ui.RoleMenu).Focusable().MinWidth(180).MaxWidth(360).Padding(4).Gap(1)
			a.buildGridMenu(c, panel)
		})
	})
}

func (a *App) buildGridMenu(c *ui.Context, panel ui.Element) {
	th := c.Theme()
	entries := a.gridMenuEntries()
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
			a.gridMenuOpen = false
			a.gridMenuRefocus = true
			a.gridFocusHandle.Focus(c)
			if entry.action != nil {
				a.later(entry.action)
			}
		}
		rows = append(rows, row)
		enabled = append(enabled, !entry.disabled)
	}

	if a.gridMenuFocusFirst {
		a.gridMenuFocusFirst = false
		a.gridMenuFocusPanel = false
		for i, row := range rows {
			if enabled[i] {
				row.Focus()
				break
			}
		}
	} else if a.gridMenuFocusPanel {
		a.gridMenuFocusPanel = false
		panel.Focus()
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
		a.gridMenuOpen = false
		a.gridMenuRefocus = true
		a.gridFocusHandle.Focus(c)
	}
}

func moveGridMenuFocus(rows []ui.Element, enabled []bool, step int) {
	n := len(rows)
	if n == 0 {
		return
	}
	at := -1
	for i, row := range rows {
		if row.Focused() {
			at = i
			break
		}
	}
	if at < 0 {
		if step > 0 {
			moveGridMenuFocusTo(rows, enabled, 0, 1)
		} else {
			moveGridMenuFocusTo(rows, enabled, n-1, -1)
		}
		return
	}
	for k := 1; k <= n; k++ {
		i := ((at+step*k)%n + n) % n
		if enabled[i] {
			rows[i].Focus()
			return
		}
	}
}

func moveGridMenuFocusTo(rows []ui.Element, enabled []bool, start, step int) {
	for i := start; i >= 0 && i < len(rows); i += step {
		if enabled[i] {
			rows[i].Focus()
			return
		}
	}
}

func (a *App) gridMenuEntries() []gridMenuEntry {
	if a.contextEmpty || len(a.contextIDs) == 0 {
		t := a.tab()
		return withoutWriteEntries(a.tabReadOnly(), []gridMenuEntry{
			{label: "新建文件夹…", write: true, action: func() { a.later(a.beginNewFolder) }},
			{label: "刷新", shortcut: "F5", action: func() {
				if t != nil {
					a.later(func() { a.loadTab(t, false) })
				}
			}},
			{divider: true},
			{label: "全选", shortcut: "Ctrl+A", action: func() {
				if t != nil {
					a.later(func() {
						a.active = tabIndex(a.tabs, t)
						a.selectAll()
					})
				}
			}},
			{label: "取消选择", action: func() { a.later(a.clearSelection) }},
		})
	}

	one := a.contextOne()
	ids := append([]string(nil), a.contextIDs...)
	t := a.tab()
	entries := []gridMenuEntry{
		{label: "打开", disabled: one == nil, action: func() {
			if one != nil {
				a.later(func() { a.activateItem(one) })
			}
		}},
		{label: "在资源管理器中显示", localOnly: true, action: func() {
			a.later(func() {
				if len(ids) == 1 {
					if it, ok := items.Get(ids[0]); ok {
						_ = (AppSvc{}).Reveal(it.Path)
					}
				} else if t != nil {
					_ = (AppSvc{}).OpenFolder(t.Path)
				}
			})
		}},
		{label: "用默认应用打开", localOnly: true, disabled: one == nil, action: func() {
			if one != nil {
				path := one.Path
				a.later(func() { _ = (AppSvc{}).OpenExternal(path) })
			}
		}},
		{label: "复制路径", disabled: one == nil, action: func() {
			if one != nil {
				path := one.Path
				a.later(func() { _ = (AppSvc{}).CopyText(path) })
			}
		}},
		{divider: true},
		{label: "重命名…", shortcut: "F2", disabled: one == nil, write: true, action: func() {
			if one != nil {
				a.later(a.beginRename)
			}
		}},
		{label: "移到回收站", shortcut: "Delete", write: true, action: func() {
			a.later(func() { a.askDelete(false) })
		}},
		{label: "永久删除", write: true, action: func() {
			a.later(func() { a.askDelete(true) })
		}},
		{divider: true},
		{label: "显示信息", disabled: one == nil || !isMediaKind(one.Kind), action: func() {
			if one != nil && isMediaKind(one.Kind) {
				a.later(func() {
					a.activateItem(one)
					a.viewerSetInfo(true)
				})
			}
		}},
	}
	return withoutWriteEntries(a.tabReadOnly(), entries)
}

// withoutWriteEntries hides the entries that change files on a read-only
// location, and the ones that would hand the path to Windows, then collapses
// the dividers they leave behind.
func withoutWriteEntries(readOnly bool, entries []gridMenuEntry) []gridMenuEntry {
	if !readOnly {
		return entries
	}
	out := entries[:0]
	for _, e := range entries {
		if e.write || e.localOnly {
			continue
		}
		if e.divider && (len(out) == 0 || out[len(out)-1].divider) {
			continue
		}
		out = append(out, e)
	}
	for len(out) > 0 && out[len(out)-1].divider {
		out = out[:len(out)-1]
	}
	return out
}
