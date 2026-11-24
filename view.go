package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *App) view(c *ui.Context) {
	c.SetTheme(a.uiTheme(c))
	th := c.Theme()
	a.viewShortcuts(c)
	ui.Row(c).Fill().Background(th.Background).Children(func() {
		if a.sidebar {
			a.sidebarView(c)
		}
		ui.Column(c).Grow(1).FillHeight().Children(func() {
			a.tabStrip(c)
			a.toolbar(c)
			a.content(c)
			a.statusBar(c)
		})
	})

	a.overlays(c)
	a.runPending()
}

// viewShortcuts keeps the original single-letter browsing keys and the
// refresh/find keys working while the focus is outside a text field.
func (a *App) viewShortcuts(c *ui.Context) {
	c.OnWindowShortcut(0, ui.KeyW, func() { a.up() })
	c.OnWindowShortcut(0, ui.KeyS, func() { a.goBack() })
	c.OnWindowShortcut(0, ui.KeyA, func() { a.sibling(-1) })
	c.OnWindowShortcut(0, ui.KeyD, func() { a.sibling(1) })
	c.OnWindowShortcut(0, ui.KeyF5, func() { a.loadTab(a.tab(), false) })
	c.OnWindowShortcut(ui.Ctrl, ui.KeyF, func() {
		a.focusSearch = true
		a.refresh()
	})
}

func (a *App) sidebarView(c *ui.Context) {
	th := c.Theme()
	side := ui.Column(c).Width(float32(a.sidebarWidth)).FillHeight().Background(th.Surface)
	side.BorderStartWidth(1).BorderColor(th.Border)
	side.Children(func() {
		ui.Row(c).Padding(12, 12, 6, 12).Gap(8).Children(func() {
			ui.Text(c, "FlowVision").FontSize(17).Bold().Grow(1)
			ui.Text(c, "v"+appVersion).FontSize(10).TextColor(th.TextMuted)
		})
		ui.Scroll(c).Grow(1).Padding(4, 8, 12, 8).Gap(2).Children(func() {
			a.sidebarSection(c, "收藏")
			if len(a.settings.Favorites) == 0 {
				ui.Text(c, "用工具栏的星标收藏文件夹").FontSize(11).TextColor(th.TextMuted).Padding(4, 8)
			}
			for _, fav := range a.settings.Favorites {
				fav := fav
				a.sidebarPathRow(c, fav, "★", true)
			}

			a.sidebarSection(c, "位置")
			for _, p := range a.places {
				p := p
				a.sidebarPathRow(c, p.Path, sidebarGlyph(p.Icon), false)
			}

			a.sidebarSection(c, "驱动器")
			for _, p := range a.drives {
				p := p
				a.sidebarPathRow(c, p.Path, "▣", false)
			}

			folderRow := ui.Row(c).Gap(6).Padding(12, 4, 4, 4).Children(func() {
				ui.Text(c, "文件夹").Bold().FontSize(11).TextColor(th.TextMuted).Grow(1)
				if ui.Button(c, "").Label("刷新文件夹树").Tooltip("刷新").Children(func() {
					ui.Text(c, "↻")
				}).Clicked() {
					if t := a.tab(); t != nil {
						a.invalidateTree(t.Path)
					}
				}
			})
			_ = folderRow
			if t := a.tab(); t != nil {
				a.ensureTree(t.Path)
				if !a.treeLoaded[t.Path] {
					ui.Text(c, "正在读取文件夹…").FontSize(11).TextColor(th.TextMuted).Padding(4, 8)
				} else {
					kids := a.treeKids[t.Path]
					if len(kids) == 0 {
						ui.Text(c, "没有子文件夹").FontSize(11).TextColor(th.TextMuted).Padding(4, 8)
					}
					ui.Tree(c, func() {
						for _, kid := range kids {
							a.treeFolder(c, kid.Path, kid.Name)
						}
					})
				}
			}
		})
	})
}

func (a *App) sidebarSection(c *ui.Context, title string) {
	ui.Text(c, title).FontSize(10).FontWeight(600).TextColor(c.Theme().TextMuted).
		Padding(10, 8, 2, 8)
}

func (a *App) sidebarPathRow(c *ui.Context, path, glyph string, favorite bool) {
	th := c.Theme()
	active := false
	if t := a.tab(); t != nil {
		active = strings.EqualFold(filepath.Clean(path), filepath.Clean(t.Path))
	}
	row := ui.Row(c.Key(path)).Focusable().Gap(7).Padding(3, 8).Radius(6)
	if active {
		row.Background(ui.RGBA(127, 127, 127, 28))
	}
	if row.Hovered() && !active {
		row.Background(ui.RGBA(127, 127, 127, 14))
	}
	row.Children(func() {
		ui.Text(c, glyph).FontSize(12).TextColor(th.TextMuted)
		ui.Text(c, displayName(path)).SingleLine().FontSize(12).Grow(1)
	})
	if row.Clicked() {
		a.goTo(path, true)
	}
	if favorite {
		row.ContextMenu(func(m *ui.Menu) {
			if m.Item("打开").Chosen() {
				a.later(func() { a.goTo(path, true) })
			}
			if m.Item("从收藏中移除").Chosen() {
				a.later(func() { a.removeFavorite(path) })
			}
			if m.Item("在资源管理器中打开").Chosen() {
				a.later(func() { _ = (AppSvc{}).OpenFolder(path) })
			}
		})
	}
}

func (a *App) treeFolder(c *ui.Context, path, name string) {
	th := c.Theme()
	open := a.treeOpenPtr(path)
	item := ui.TreeItem(c.Key(path), name, open, func() {
		if !a.treeLoaded[path] {
			a.ensureTree(path)
			ui.Text(c, "正在读取…").FontSize(11).TextColor(th.TextMuted)
			return
		}
		for _, kid := range a.treeKids[path] {
			a.treeFolder(c, kid.Path, kid.Name)
		}
	})
	active := false
	if t := a.tab(); t != nil {
		active = strings.EqualFold(filepath.Clean(path), filepath.Clean(t.Path))
	}
	item.Selected(active)
	if item.Clicked() {
		a.goTo(path, true)
	}
	if *open {
		a.ensureTree(path)
	}
}

func (a *App) tabStrip(c *ui.Context) {
	th := c.Theme()
	ui.Row(c).Height(38).Background(th.Surface).Padding(6, 8, 2, 8).Gap(6).Children(func() {
		ui.ScrollHorizontal(c).Grow(1).Scrollbars(false).Children(func() {
			ui.Row(c).Gap(4).Children(func() {
				for i, tab := range a.tabs {
					tab := tab
					i := i
					box := ui.Row(c.Key(tab.ID)).Focusable().Height(30).Gap(4).Padding(0, 10).
						PaddingEnd(6).Radius(7)
					if i == a.active {
						box.Background(th.Background)
					} else if box.Hovered() {
						box.Background(ui.RGBA(127, 127, 127, 18))
					}
					box.Children(func() {
						ui.Text(c, tab.Name).SingleLine().MaxWidth(176).FontSize(12)
						close := ui.Button(c, "").Label("关闭标签页 "+tab.Name).Tooltip("关闭标签页").
							Padding(2, 4).Children(func() { ui.Text(c, "×").FontSize(12) })
						if close.Clicked() {
							a.later(func() { a.closeTab(i) })
						}
					})
					if box.Clicked() {
						a.activateTab(i)
					}
					if a.scrollActiveTab && i == a.active {
						box.ScrollIntoView()
					}
				}
			})
		})
		if a.scrollActiveTab {
			a.scrollActiveTab = false
		}
		if ui.Button(c, "").Label("新建标签页").Tooltip("新建标签页 (Ctrl+T)").Children(func() {
			ui.Text(c, "＋").FontSize(15)
		}).Clicked() {
			a.later(func() { a.newTab("") })
		}
	})
}

func (a *App) toolbar(c *ui.Context) {
	th := c.Theme()
	tab := a.tab()
	if tab == nil {
		return
	}
	ui.Column(c).Padding(8, 12, 6, 12).Gap(6).Background(th.Background).Children(func() {
		ui.Row(c).Gap(6).Children(func() {
			iconButton(c, "←", "返回", func() { a.goBack() }, len(tab.History) == 0)
			iconButton(c, "↑", "上一级", a.up, false)
			iconButton(c, "‹", "上一文件夹", func() { a.sibling(-1) }, false)
			iconButton(c, "›", "下一文件夹", func() { a.sibling(1) }, false)

			ui.Column(c).Grow(1).PaddingX(4).Children(func() {
				ui.Text(c, tab.Name).FontSize(15).FontWeight(600).SingleLine()
				ui.Text(c, tab.Path).FontSize(10).TextColor(th.TextMuted).SingleLine()
			})
			star := "☆"
			if a.isFavorite(tab.Path) {
				star = "★"
			}
			iconButton(c, star, "收藏此文件夹", a.toggleFavorite, false)
			iconButton(c, "⚙", "设置", a.openSettings, false)
		})
		ui.Row(c).Gap(6).Children(func() {
			ui.TextInput(c.Key(fmt.Sprintf("search-%d", tab.ID)), &tab.Query).
				Placeholder("搜索此文件夹").Label("搜索").Grow(1).MaxWidth(420).
				Bind(&a.searchHandle)
			if a.focusSearch {
				a.searchHandle.SelectAll(c)
				a.focusSearch = false
			}
			if tab.Query != tab.AppliedQuery {
				tab.refilter()
				a.refresh()
			}
			if tab.Query != "" {
				if ui.Button(c, "").Label("清除搜索").Tooltip("清除搜索").Padding(2, 6).Children(func() {
					ui.Text(c, "×").FontSize(12)
				}).Clicked() {
					tab.Query = ""
					tab.refilter()
					a.refresh()
				}
			}
			sortValue := sortLabel(tab.Sort)
			sel := ui.Select(c.Key("sort"), &sortValue, []string{"名称", "日期", "大小", "类型"}).
				Label("排序").Width(84)
			if sel.Changed() {
				if key := sortKey(sortValue); key != tab.Sort {
					tab.Sort = key
					a.persist()
					a.loadTab(tab, true)
				}
			}
			dirGlyph := "↑"
			if tab.Desc {
				dirGlyph = "↓"
			}
			if ui.Button(c, "").Label("切换排序方向").Tooltip("升序 / 降序").
				Padding(3, 8).Children(func() { ui.Text(c, dirGlyph).FontSize(13) }).Clicked() {
				tab.Desc = !tab.Desc
				a.persist()
				a.loadTab(tab, true)
			}
			recursive := a.settings.Recursive
			if ui.Toggle(c, &recursive, "递归").Changed() {
				a.settings.Recursive = recursive
				_ = saveSettings(a.settings)
				a.loadTab(tab, true)
			}
			hidden := a.settings.ShowHidden
			if ui.Toggle(c, &hidden, "隐藏").Changed() {
				a.settings.ShowHidden = hidden
				_ = saveSettings(a.settings)
				a.treeGen++
				a.treeLoaded = map[string]bool{}
				a.treeKids = map[string][]TreeEntry{}
				a.loadTab(tab, true)
			}
		})
	})
}

func iconButton(c *ui.Context, glyph, label string, fn func(), disabled bool) {
	btn := ui.Button(c, "").Label(label).Tooltip(label).Disabled(disabled).
		Padding(3, 8).Children(func() { ui.Text(c, glyph).FontSize(14) })
	btn.OnClick(fn)
}

func sortLabel(key string) string {
	switch key {
	case "date":
		return "日期"
	case "size":
		return "大小"
	case "type":
		return "类型"
	}
	return "名称"
}

func sortKey(label string) string {
	switch label {
	case "日期":
		return "date"
	case "大小":
		return "size"
	case "类型":
		return "type"
	}
	return "name"
}

func (a *App) content(c *ui.Context) {
	th := c.Theme()
	tab := a.tab()
	if tab == nil {
		ui.Column(c).Grow(1).Center().Children(func() {
			ui.Text(c, "没有打开的文件夹").TextColor(th.TextMuted)
		})
		return
	}
	switch {
	case tab.Loading:
		ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
			ui.Text(c, "正在读取…").TextColor(th.TextMuted)
		})
	case tab.Err != "":
		ui.Column(c).Grow(1).Center().Gap(10).Children(func() {
			ui.Text(c, tab.Err).TextColor(th.Text)
			ui.Button(c, "重试").OnClick(func() { a.loadTab(tab, false) })
		})
	case len(tab.Items) == 0:
		ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
			if tab.Query != "" {
				ui.Text(c, "没有匹配的图片").TextColor(th.TextMuted)
			} else {
				ui.Text(c, "此文件夹没有可显示的图片").TextColor(th.TextMuted)
			}
			if tab.Query != "" {
				ui.Button(c, "清除搜索").OnClick(func() {
					tab.Query = ""
					tab.refilter()
					a.refresh()
				})
			}
		})
	default:
		a.gridView(c, tab)
	}
}

func (a *App) statusBar(c *ui.Context) {
	th := c.Theme()
	tab := a.tab()
	if tab == nil {
		return
	}
	ui.Row(c).Height(27).Padding(0, 12).Gap(10).Background(th.Surface).Children(func() {
		ui.Text(c, fmt.Sprintf("%d 项", len(tab.Items))).FontSize(11).TextColor(th.TextMuted)
		if tab.Query != "" && len(tab.Items) != len(tab.All) {
			ui.Text(c, fmt.Sprintf("（共 %d 项）", len(tab.All))).FontSize(11).TextColor(th.TextMuted)
		}
		ui.Text(c, fmt.Sprintf("%d 图片 · %d 视频", countKind(tab.Items, "image"), countKind(tab.Items, "video"))).
			FontSize(11).TextColor(th.TextMuted)
		if n := len(tab.Selected); n > 0 {
			ui.Text(c, fmt.Sprintf("已选 %d 项", n)).FontSize(11).TextColor(th.Accent)
		}
		if tab.Result.Truncated {
			ui.Text(c, "已截断").FontSize(11).TextColor(th.TextMuted)
		}
		ui.Spacer(c)
		if a.settings.Recursive {
			ui.Badge(c, "递归").FontSize(10)
		}
		if a.settings.ShowHidden {
			ui.Badge(c, "隐藏").FontSize(10)
		}
		ui.Text(c, "纯 Go 原生 UI").FontSize(10).TextColor(th.TextMuted)
	})
}

func countKind(items []*Item, kind string) int {
	n := 0
	for _, it := range items {
		if it.Kind == kind {
			n++
		}
	}
	return n
}

func sidebarGlyph(icon string) string {
	switch icon {
	case "pictures":
		return "▦"
	case "desktop":
		return "▭"
	case "downloads":
		return "↓"
	case "documents":
		return "▤"
	case "videos":
		return "▶"
	}
	return "▢"
}
