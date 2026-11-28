package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

func (a *App) view(c *ui.Context) {
	c.SetTheme(a.uiTheme(c))
	th := c.Theme()
	_, winH := c.Size()
	if a.viewer.Open {
		// The viewer owns the whole window while it is open. Building the
		// normal chrome underneath would keep native hit targets alive — most
		// notably the maximize button's snap-layout zone under the info icon.
		a.viewerOverlay(c)
		a.runPending()
		return
	}
	a.viewShortcuts(c)
	ui.Column(c).Fill().Background(th.Background).Children(func() {
		// The sidebar is a genuine left column, not a strip below the title bar.
		// Its surface and right hairline run the full height of the window, while
		// the workbench owns the title bar, toolbar, content and status bar.
		ui.Row(c).Grow(1).FillWidth().Children(func() {
			if a.sidebar {
				a.sidebarView(c)
			}
			ui.Column(c).Grow(1).FillHeight().Children(func() {
				a.titleBar(c)
				a.toolbar(c)
				a.content(c)
				a.statusBar(c)
			})
		})

		// Draw the shell's internal grid at one level. Letting each panel paint
		// its own edge made the vertical sidebar rule change width where the
		// header, navigation and footer met it, and put the footer/status
		// dividers on two different y positions. These overlays join at exact
		// coordinates and sit above every surface.
		a.shellHairlines(c, winH, th)
	})

	a.selectionBar(c)
	a.gridMenuOverlay(c)
	a.selectMenuOverlay(c)
	a.tasksOverlay(c)
	a.overlays(c)
	a.runPending()
}

const titleBarHeight float32 = 48

// The browse toolbar is one command row under the tab strip. It replaces the
// previous two-row title/search stack, giving the media canvas more height.
const toolbarHeight float32 = 52

// The sidebar footer and the workbench status bar share one height so their
// top hairlines form a single full-width line, not two steps.
const bottomBarHeight float32 = 40

// On the minimum window the default 248 DIP sidebar would take over a quarter
// of the width. Keep a deliberately narrow sidebar, but collapse the ordinary
// range and any wide setting that would leave too little workbench beside it.
// A wide 520 DIP sidebar still gets its stored width once the window can spare
// both it and a comfortable command row.
func (a *App) sidebarLayoutWidth(windowW float32) float32 {
	if !a.sidebar {
		return 0
	}
	w := float32(a.sidebarWidth)
	if windowW > 0 && w > compactSidebarMax &&
		(windowW < compactSidebarThreshold || windowW < w+compactSidebarWorkbench) {
		w = compactSidebarMax
	}
	return w
}

// shellHairlines paints the frame's internal joints after all panels. A
// 1 DIP line at the sidebar edge runs continuously through the brand header,
// navigation, footer and status bar; the title-bar and bottom dividers run the
// full width. Painting these here avoids child borders fighting over the same
// corner pixels.
func (a *App) shellHairlines(c *ui.Context, winH float32, th *ui.Theme) {
	// Resolve the hairline against the panel surface once. Drawing a translucent
	// rule twice at a T-junction darkened the joint by another alpha step; an
	// opaque pre-blended color keeps every segment the same weight.
	winW, _ := c.Size()
	hair := th.Border.Alpha(.55).Over(th.Surface)
	ui.Box(c).Absolute().Left(0).Right(0).Top(titleBarHeight).Height(1).
		Background(hair).PassThrough()
	ui.Box(c).Absolute().Left(0).Right(0).Top(winH - bottomBarHeight).Height(1).
		Background(hair).PassThrough()
	if a.sidebar {
		ui.Box(c).Absolute().Left(a.sidebarLayoutWidth(winW) - 1).Top(0).Width(1).Height(winH).
			Background(hair).PassThrough()
	}
}

// brandContent draws the app mark and wordmark. It is shared by the sidebar
// header and the collapsed-sidebar title bar so the identity stays in the same
// visual language in both layouts.
func (a *App) brandContent(c *ui.Context) {
	th := c.Theme()
	ui.Box(c).Size(28, 28).Radius(9).Background(th.Accent.Alpha(.13)).Center().Children(func() {
		components.Icon(c, lucide.Images).Size(16, 16).TextColor(th.Accent)
	})
	components.Text(c, "FlowVision").FontSize(13).FontWeight(600).SingleLine()
}

// brandHeader is the 48 DIP header at the top of the full-height sidebar. The
// shared shell divider below it continues into the workbench title bar; its
// right edge meets the sidebar's full-height vertical hairline.
func (a *App) brandHeader(c *ui.Context) {
	th := c.Theme()
	ui.Row(c).Height(titleBarHeight).FillWidth().Background(th.Surface).
		Padding(0, 0, 0, 16).Gap(9).AlignItems(ui.Center).DragWindow().
		Children(func() { a.brandContent(c) })
}

// titleBar is the workbench's title bar. With the sidebar open, the brand lives
// in the sidebar header and this row starts at the sidebar's right hairline; with
// the sidebar collapsed the brand returns here and the row spans the window.
func (a *App) titleBar(c *ui.Context) {
	th := c.Theme()
	bar := ui.Row(c).Height(titleBarHeight).FillWidth().Background(th.Surface).
		Gap(8).AlignItems(ui.Center).DragWindow()
	if !a.sidebar {
		bar.Padding(0, 0, 0, 12)
	}
	bar.Children(func() {
		if !a.sidebar {
			brand := ui.Row(c).Height(titleBarHeight).Gap(9).
				Padding(0, 6, 0, 0).AlignItems(ui.Center).DragWindow()
			brand.Children(func() { a.brandContent(c) })
			ui.Box(c).Width(1).Height(22).Background(th.Border).Margin(0, 4)
			components.IconButton(c, lucide.PanelLeftOpen(), "展开侧栏 (Ctrl+B)", components.IconButtonPlain).
				OnClick(a.toggleSidebar)
		}
		a.tabStrip(c)
		a.windowControls(c)
	})
}

// windowControls draws the minimize, maximize/restore, fullscreen/exit and
// close buttons that replace the hidden system buttons. They sit at the far
// right of the title bar, full height, as native caption buttons do.
func (a *App) windowControls(c *ui.Context) {
	ui.Row(c).Height(titleBarHeight).Gap(0).Children(func() {
		a.windowControl(c, lucide.Minus(), "最小化", false, false, func() {
			if a.win != nil {
				a.win.Minimize()
			}
		})
		// Maximize and restore share one button; the app tracks the state
		// through OnMaximize/OnUnmaximize to draw the correct glyph. Fullscreen
		// is a separate mode, so its transition is not folded into maximize.
		if !a.fullscreen {
			icon := lucide.Square()
			label := "最大化"
			if a.maximized {
				icon = lucide.Copy()
				label = "还原"
			}
			a.windowControl(c, icon, label, false, true, func() {
				if a.win == nil {
					return
				}
				if a.maximized {
					a.win.Unmaximize()
				} else {
					a.win.Maximize()
				}
			})
		}
		fullIcon := lucide.Fullscreen()
		fullLabel := "全屏 (F11)"
		if a.fullscreen {
			fullIcon = lucide.Minimize2()
			fullLabel = "退出全屏 (F11)"
		}
		a.windowControl(c, fullIcon, fullLabel, false, false, func() {
			a.toggleWindowFullscreen()
		})
		a.windowControl(c, lucide.X(), "关闭", true, false, func() {
			if a.win != nil {
				a.win.Close()
			}
		})
	})
}

func (a *App) windowControl(c *ui.Context, icon *ui.SVG, label string, danger, maximize bool, action func()) {
	th := c.Theme()
	b := ui.ButtonBase(c).Label(label).Size(46, titleBarHeight).Padding(0).Radius(0)
	b.Draw(func(p *ui.Painter, r ui.Rect) {
		var face ui.Color
		switch {
		case b.IsDisabled():
		case b.Pressed():
			if danger {
				face = th.Danger.Mix(ui.RGB(0, 0, 0), .18)
			} else {
				face = th.Text.Alpha(.14)
			}
		case b.Hovered():
			if danger {
				face = th.Danger
			} else {
				face = th.Text.Alpha(.08)
			}
		}
		if face.A > 0 {
			p.Fill(r, face, 0)
		}
	}).Children(func() {
		fg := th.TextMuted
		if danger && b.Hovered() {
			fg = ui.RGB(255, 255, 255)
		}
		components.Icon(c, icon).Size(14, 14).TextColor(fg)
	})
	if maximize {
		b.MaximizeButton()
	}
	if b.Clicked() && action != nil {
		a.later(action)
	}
}

// viewShortcuts keeps the original single-letter browsing keys and the
// refresh/find keys working while the focus is outside a text field.
func (a *App) viewShortcuts(c *ui.Context) {
	c.OnWindowShortcut(0, ui.KeyW, func() { a.up() })
	c.OnWindowShortcut(0, ui.KeyS, func() { a.goBack() })
	c.OnWindowShortcut(0, ui.KeyA, func() { a.sibling(-1) })
	c.OnWindowShortcut(0, ui.KeyD, func() { a.sibling(1) })
	c.OnWindowShortcut(0, ui.KeyF5, func() { a.loadTab(a.tab(), false) })
	c.OnWindowShortcut(0, ui.KeyF1, a.openHelp)
	c.OnWindowShortcut(0, ui.KeyF11, func() { a.toggleWindowFullscreen() })
	c.OnWindowShortcut(ui.Ctrl, ui.KeyF, func() {
		a.searchOpen = true
		a.focusSearch = true
		a.refresh()
	})
}

func (a *App) sidebarView(c *ui.Context) {
	th := c.Theme()
	winW, _ := c.Size()
	side := ui.Column(c).Width(a.sidebarLayoutWidth(winW)).FillHeight().Background(th.Surface)
	side.Children(func() {
		// The sidebar owns the app mark at the top of its full-height column.
		// The workbench title bar starts to its right, so the tab row and its
		// left hairline share one continuous boundary instead of two offsets.
		a.brandHeader(c)
		// The scroll tracks its place, which ScrollIntoView needs: a container
		// that does not keep where it was would jump back on every rebuild.
		ui.Scroll(c).Key("sidebar-scroll").TrackScroll(&a.sidebarScroll).
			Grow(1).Padding(8, 6, 8, 8).Children(func() {
			components.Sidebar(c).Children(func() {
				components.SidebarSection(c, "收藏", func() {
					// The favorites folder collects files; the rows below it
					// keep folders. One section, because both answer "where do
					// I go back to".
					a.sidebarFavoritesRow(c)
					starred := a.settings.StarredPaths
					if len(starred) == 0 && len(a.settings.Favorites) == 0 {
						components.Text(c, "右键图片或视频加入收藏夹，点路径栏的星标收藏文件夹").
							FontSize(10.5).TextColor(th.TextMuted).Padding(2, 6).SingleLine()
					}
					for _, f := range starred {
						f := f
						a.sidebarStarredPathRow(c, f)
					}
				})

				if a.settings.ShowPlaces {
					components.SidebarSection(c, "位置", func() {
						for _, p := range a.places {
							p := p
							a.sidebarPathRow(c, p.Path, placeIcon(p.Icon), false, p.Name)
						}
					})
				}

				components.SidebarSection(c, "驱动器", func() {
					for _, p := range a.drives {
						p := p
						a.sidebarPathRow(c, p.Path, lucide.HardDrive(), false)
					}
				})

				components.SidebarSectionWithActions(c, "远程", func(c *ui.Context) {
					components.IconButton(c, lucide.Plus(), "添加远程位置", components.IconButtonCompact).
						OnClick(func() { a.later(a.openRemoteDialog) })
				}, func() {
					if len(a.settings.RemoteShares) == 0 {
						components.Text(c, `添加 SMB 或 WebDAV 共享，例如 smb://nas/photos`).FontSize(10.5).
							TextColor(th.TextMuted).Padding(2, 6).SingleLine().Ellipsis("…")
						return
					}
					for _, share := range a.settings.RemoteShares {
						share := share
						a.sidebarRemoteRow(c, share)
					}
				})

				components.SidebarSectionWithActions(c, "文件夹", func(c *ui.Context) {
					if t := a.tab(); t != nil {
						a.folderTreeCount(c, t)
						components.IconButton(c, lucide.RefreshCw(), "刷新文件夹树", components.IconButtonCompact).
							OnClick(func() {
								a.invalidateTree(t.Path)
							})
					}
				}, func() {
					a.sidebarFolders(c)
				})
			})
		})

		ui.Row(c).Height(bottomBarHeight).Padding(7, 8, 7, 8).Gap(2).Background(th.Surface).
			Children(func() {
				components.IconButton(c, lucide.SlidersHorizontal(), "设置", components.IconButtonPlain).
					OnClick(a.openSettings)
				components.IconButton(c, lucide.Keyboard(), "快捷键帮助", components.IconButtonPlain).
					OnClick(a.openHelp)
				ui.Spacer(c)
				components.IconButton(c, lucide.PanelLeftClose(), "收起侧栏 (Ctrl+B)", components.IconButtonPlain).
					OnClick(a.toggleSidebar)
				components.Text(c, "v"+appVersion()).FontSize(9.5).TextColor(th.TextMuted.Alpha(.75)).Padding(0, 4)
			})
	})
}

// sidebarFavoritesRow is the entry to the virtual favorites folder. It is one
// row, not a list: the collected files are a folder of their own, shown in the
// grid like any other.
func (a *App) sidebarFavoritesRow(c *ui.Context) {
	th := c.Theme()
	count := len(a.settings.Favorites)
	active := false
	if t := a.tab(); t != nil {
		active = isFavoritesPath(t.Path) && !a.viewer.Open
	}
	label := favoritesName
	if count > 0 {
		label = fmt.Sprintf("%s (%d)", favoritesName, count)
	}
	row := components.SidebarItemWithLeading(c, label, active, func(c *ui.Context, size float32) {
		color := th.TextMuted.Alpha(.9)
		if count > 0 {
			color = th.Accent.Alpha(.72)
		}
		if active {
			color = th.Accent
		}
		components.Icon(c, lucide.Star()).Size(size, size).TextColor(color)
	})
	row.Key("sidebar-favorites").Tooltip("收藏夹 · 只收图片和视频")
	if row.Clicked() {
		a.goTo(favoritesPath, true)
	}
	components.ContextMenu(c, row, func(m *components.Menu) {
		if m.Item("打开收藏夹").Chosen() {
			a.later(func() { a.goTo(favoritesPath, true) })
		}
		if m.Item("清空收藏夹").Disabled(count == 0).Chosen() {
			a.later(a.clearFavorites)
		}
	})
}

// clearFavorites empties the virtual folder.
func (a *App) clearFavorites() {
	if len(a.settings.Favorites) == 0 {
		return
	}
	n := len(a.settings.Favorites)
	a.settings.Favorites = nil
	_ = saveSettings(a.settings)
	a.showStatusNotice(fmt.Sprintf("已清空收藏夹（%d 项）", n))
	if t := a.tab(); t != nil && isFavoritesPath(t.Path) {
		a.reloadActive()
	}
	a.refresh()
}

func (a *App) sidebarPathRow(c *ui.Context, path string, icon *ui.SVG, favorite bool, label ...string) {
	th := c.Theme()
	active := false
	if t := a.tab(); t != nil {
		active = strings.EqualFold(cleanDisplayPath(path), cleanDisplayPath(t.Path))
	}
	// A favorite and a built-in location can point at the same folder. Keep
	// both rows available, but let the favorite carry the active highlight so
	// the sidebar never shows two identical current locations at once.
	if active && !favorite && a.isFavorite(path) {
		active = false
	}
	name := displayName(path)
	if len(label) > 0 && label[0] != "" {
		name = label[0]
	}
	// Keep the accent for the active location and favorited items; muted
	// icons elsewhere make the selected row read at a glance instead of
	// turning every navigation item into an equally loud orange mark.
	row := components.SidebarItemWithLeading(c, name, active, func(c *ui.Context, size float32) {
		color := th.TextMuted.Alpha(.9)
		if favorite {
			color = th.Accent.Alpha(.72)
		}
		if active {
			color = th.Accent
		}
		components.Icon(c, icon).Size(size, size).TextColor(color)
	})
	row.Key(path).Tooltip(path)
	if row.Clicked() {
		a.goTo(path, true)
	}
	if favorite {
		components.ContextMenu(c, row, func(m *components.Menu) {
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

func placeIcon(kind string) *ui.SVG {
	switch kind {
	case "home":
		return lucide.House()
	case "pictures":
		return lucide.Images()
	case "desktop":
		return lucide.Monitor()
	case "downloads":
		return lucide.Download()
	case "documents":
		return lucide.FileText()
	case "videos":
		return lucide.Video()
	}
	return lucide.Folder()
}

// sidebarFolders renders the current folder as the root of a real tree. The
// previous section listed only the root's children, so the folder the section
// belonged to had no visible row and the first child looked like a stray item.
func (a *App) sidebarFolders(c *ui.Context) {
	t := a.tab()
	if t == nil {
		a.folderTreeNote(c, "没有打开的文件夹", false, 4)
		return
	}
	p := cleanDisplayPath(t.Path)
	a.ensureTree(p)
	rootName := t.Name
	if rootName == "" {
		rootName = displayName(p)
	}
	// The folder and the ones beside it in one list, in the order the file
	// system gives them: pinning the current folder to the top made the section
	// read as a tree with a root, when it is a row of places to switch between.
	// The current folder keeps its disclosure arrow and its sub folders; the
	// others are leaves, because their own children would turn the section into
	// a directory listing.
	tree := ui.TreeBase(c)
	tree.Root.FillWidth().MinWidth(0).Gap(1).Children(func() {
		entries, ok, complete := a.folderRowEntries(p, rootName)
		if !ok {
			// No parent to list from, as at a drive root: the folder stands
			// alone.
			_ = a.folderTreeRow(c, tree, p, rootName, true, false)
			if a.treeLoaded[p] && len(a.treeKids[p]) == 0 {
				a.folderTreeNote(c, "没有子文件夹", false, 39)
			}
			return
		}
		for _, e := range entries {
			if strings.EqualFold(cleanDisplayPath(e.entry.Path), p) {
				current := a.folderTreeRow(c, tree, p, rootName, true, false)
				// Centre the current folder once per change, and only once its
				// siblings are known: it sits among them by name, so it can fall
				// outside the section's height. Centring before they arrive would
				// mark it done and never fire again. Every frame would keep the
				// user from scrolling the section away.
				if complete && !strings.EqualFold(a.centeredFolderTree, p) {
					a.centeredFolderTree = p
					current.ScrollIntoView(ui.ScrollIntoViewOptions{Block: ui.ScrollCenter})
				}
				if a.treeLoaded[p] && len(a.treeKids[p]) == 0 {
					a.folderTreeNote(c, "没有子文件夹", false, 39)
				}
				continue
			}
			a.folderLeafRow(c, e.entry)
		}
	})
}

// folderRowEntry is one row of the 文件夹 section: the current folder, or a
// folder beside it.
type folderRowEntry struct {
	entry TreeEntry
}

// folderRowEntries lists the current folder and its siblings together, sorted
// by name as the file system lists them. ok is false when there is no parent
// to list from, as at a drive root.
func (a *App) folderRowEntries(path, name string) ([]folderRowEntry, bool, bool) {
	p := cleanDisplayPath(path)
	res := (FS{}).Parent(p)
	if !res.Found {
		return nil, false, false
	}
	parent := cleanDisplayPath(res.Path)
	if strings.EqualFold(parent, p) {
		return nil, false, false
	}
	a.ensureTree(parent)
	kids := a.treeKids[parent]
	if len(kids) == 0 {
		// The parent is still being read, or it holds only this folder: show
		// the folder alone rather than an empty section. complete says the
		// siblings are known, so centring can wait for them.
		return []folderRowEntry{{entry: TreeEntry{Path: p, Name: name}}},
			true, a.treeLoaded[parent]
	}
	out := make([]folderRowEntry, 0, len(kids))
	seen := false
	for _, kid := range kids {
		if strings.EqualFold(cleanDisplayPath(kid.Path), p) {
			out = append(out, folderRowEntry{entry: kid})
			seen = true
			continue
		}
		out = append(out, folderRowEntry{entry: kid})
	}
	if !seen {
		// The scan does not know this folder yet, so it cannot be placed among
		// its siblings: list it first rather than lose it.
		out = append([]folderRowEntry{{entry: TreeEntry{Path: p, Name: name}}}, out...)
	}
	return out, true, true
}

// folderLeafRow is a folder beside the current one. It has no disclosure arrow
// and opens no group: clicking it moves there, which is the whole point of
// listing it.
func (a *App) folderLeafRow(c *ui.Context, sib TreeEntry) {
	th := c.Theme()
	// The same start padding the current folder's row uses at depth 0, then a
	// gap the width of its disclosure arrow: without it the sibling's icon and
	// name sit one arrow to the left of the current folder's.
	row := ui.Row(c).FillWidth().MinWidth(0).Height(26).Gap(5).AlignItems(ui.Center).
		Radius(6).PaddingStart(8).PaddingEnd(8).
		Label(sib.Name).FocusRing(true).TextColor(th.Text)
	if row.Hovered() {
		row.Background(th.Text.Alpha(.035))
	}
	row.Children(func() {
		// No arrow of its own: a sibling opens nothing. The box keeps its room
		// so the icon lines up with the current folder's.
		ui.Box(c).Size(14, 14).Shrink(0)
		components.Icon(c, lucide.Folder()).Size(14, 14).Shrink(0).
			TextColor(th.TextMuted.Alpha(.9))
		components.Text(c, sib.Name).FontSize(11.5).SingleLine().Shrink(1).MinWidth(0).Ellipsis("…")
	})
	row.Tooltip(sib.Path)
	if row.Clicked() {
		path := sib.Path
		a.later(func() { a.goTo(path, true) })
	}
}

// folderTreeCount adds the immediate child count to the section heading. A
// single count is easier to scan than a longer "N subfolders" sentence, so the
// hover tooltip spells it out.
func (a *App) folderTreeCount(c *ui.Context, t *Tab) {
	p := cleanDisplayPath(t.Path)
	if !a.treeLoaded[p] {
		return
	}
	n := len(a.treeKids[p])
	if n == 0 {
		return
	}
	th := c.Theme()
	chip := components.Text(c, fmt.Sprintf("%d 项", n)).
		FontSize(9.5).FontWeight(600).TextColor(th.TextMuted.Alpha(.78)).
		Padding(0, 5).Radius(4).Background(th.Text.Alpha(.04)).Shrink(0)
	chip.Tooltip(fmt.Sprintf("当前文件夹有 %d 个子文件夹", n))
}

// folderTreeRow builds one folder row without losing the path key. Onevra's
// TreeItem derives its key from the visible label, which would collide when two
// branches contain folders with the same name; TreeBase lets the path remain
// the identity while the row gets a disclosure arrow, folder icon and selection.
func (a *App) folderTreeRow(c *ui.Context, tree ui.TreeParts, path, name string, root, hasChildren bool) ui.Element {
	th := c.Theme()
	p := cleanDisplayPath(path)
	selected := false
	if t := a.tab(); t != nil {
		selected = strings.EqualFold(p, cleanDisplayPath(t.Path))
	}

	branch := root
	if root && a.treeLoaded[p] && len(a.treeKids[p]) == 0 {
		// Once the scan proves the current folder empty, do not keep a
		// disclosure arrow that opens only an empty result.
		branch = false
	}
	if !root {
		if a.treeLoaded[p] {
			branch = len(a.treeKids[p]) > 0
		} else {
			branch = hasChildren
		}
	}
	var open *bool
	if branch {
		if root {
			open = a.treeRootOpenPtr(p)
		} else {
			open = a.treeOpenPtr(p)
		}
	}

	// The key must precede Item so focus stays on this path while siblings
	// above it open, close or refresh.
	c.Key(p)
	item := tree.Item(open)
	row := item.Row.FillWidth().MinWidth(0).Height(26).Gap(5).AlignItems(ui.Center).
		Radius(6).PaddingStart(8 + 12*float32(item.Depth())).PaddingEnd(8).
		Label(name).Selected(selected).FocusRing(true).TextColor(th.Text)
	if selected {
		row.Background(th.Accent.Alpha(.08))
	} else if row.Hovered() {
		row.Background(th.Text.Alpha(.035))
	}
	row.Children(func() {
		arrow := ui.Box(c).Size(14, 14).Shrink(0).Role(ui.RoleNone)
		if open != nil {
			arrow.OnClick(func() { *open = !*open }).Draw(func(p *ui.Painter, r ui.Rect) {
				drawFolderTreeArrow(p, r, *open, th.TextMuted.Alpha(.85))
			})
		}
		folderColor := th.TextMuted.Alpha(.9)
		if selected {
			folderColor = th.Accent.Alpha(.85)
		}
		icon := lucide.Folder()
		if root || (open != nil && *open) {
			icon = lucide.FolderOpen()
		}
		components.Icon(c, icon).Size(14, 14).Shrink(0).TextColor(folderColor)
		components.Text(c, name).FontSize(11.5).SingleLine().Shrink(1).MinWidth(0).Ellipsis("…")
	})
	row.Tooltip(p)
	if row.Clicked() {
		if root && open != nil {
			*open = !*open
		} else if !root {
			a.goTo(p, true)
		}
	}

	if open != nil {
		item.Group(func() {
			if !a.treeLoaded[p] {
				a.folderTreeNote(c, "正在读取文件夹…", true, 39)
				a.ensureTree(p)
				return
			}
			kids := a.treeKids[p]
			if len(kids) == 0 {
				return
			}
			for _, kid := range kids {
				_ = a.folderTreeRow(c, tree, kid.Path, kid.Name, false, kid.HasChildren)
			}
		})
	}
	return row
}

// folderTreeNote is the quiet loading/empty row inside a tree, aligned under
// the child folder icons rather than floating at the section edge.
func (a *App) folderTreeNote(c *ui.Context, text string, loading bool, padStart float32) {
	th := c.Theme()
	row := ui.Row(c).FillWidth().MinWidth(0).Height(26).PaddingStart(padStart).PaddingEnd(8).
		Gap(6).AlignItems(ui.Center)
	row.Children(func() {
		if loading {
			components.Spinner(c, components.SpinnerRing).Size(12, 12).Label(text)
		} else {
			components.Icon(c, lucide.Folder()).Size(13, 13).TextColor(th.TextMuted.Alpha(.62))
		}
		components.Text(c, text).FontSize(10.5).TextColor(th.TextMuted.Alpha(.88)).
			SingleLine().Shrink(1).MinWidth(0).Ellipsis("…")
	})
}

func drawFolderTreeArrow(p *ui.Painter, r ui.Rect, open bool, color ui.Color) {
	var path ui.Path
	if open {
		path.MoveTo(r.X+r.W*.32, r.Y+r.H*.4).
			LineTo(r.X+r.W*.5, r.Y+r.H*.6).
			LineTo(r.X+r.W*.68, r.Y+r.H*.4)
	} else {
		path.MoveTo(r.X+r.W*.4, r.Y+r.H*.32).
			LineTo(r.X+r.W*.6, r.Y+r.H*.5).
			LineTo(r.X+r.W*.4, r.Y+r.H*.68)
	}
	p.StrokePath(&path, 1.2, color)
}

func (a *App) tabStrip(c *ui.Context) {
	th := c.Theme()
	strip := ui.Row(c).Grow(1).MinWidth(0).Height(titleBarHeight).AlignItems(ui.Center)
	if a.sidebar {
		// In the split layout the first tab card starts at the same inset as the
		// content column, instead of floating away from the sidebar hairline.
		strip.Padding(0, 0, 0, 16)
	}
	strip.Children(func() {
		ui.Box(c).Grow(1).MinWidth(0).Height(titleBarHeight).Children(func() {
			ui.ScrollHorizontal(c).FillWidth().FillHeight().Scrollbars(false).DragWindow().
				TrackScroll(&a.tabScroll).Children(func() {
				ui.Row(c).Gap(4).Children(func() {
					for i, tab := range a.tabs {
						tab := tab
						i := i
						active := i == a.active
						box := ui.Row(c.Key(tab.ID)).Focusable().Height(34).Gap(7).Padding(0, 10).
							PaddingEnd(6).Radius(9).AlignItems(ui.Center)
						if active {
							box.Background(th.SurfaceHover.Alpha(.55)).Border(1, th.Border.Alpha(.62))
						} else if box.Hovered() {
							box.Background(th.Text.Alpha(.035))
						}
						box.DrawOver(func(p *ui.Painter, r ui.Rect) {
							if active {
								p.Fill(ui.Rect{X: r.X + 10, Y: r.Y + r.H - 2, W: r.W - 20, H: 2}, th.Accent, 1)
							}
						})
						box.Children(func() {
							fgText := th.TextMuted
							fgIcon := th.TextMuted.Alpha(.82)
							if active {
								fgText = th.Text
								fgIcon = th.Accent
							} else if box.Hovered() {
								fgText = th.Text
								fgIcon = th.TextMuted
							}
							components.Icon(c, lucide.FolderOpen()).Size(13, 13).TextColor(fgIcon)
							components.Text(c, tab.Name).SingleLine().MaxWidth(168).FontSize(12).
								TextColor(fgText).FontWeight(func() int {
								if active {
									return 600
								}
								return 400
							}())
							close := a.tabCloseButton(c, tab.Name, active, box.Hovered())
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
			// A clipped tab should fade into the title bar instead of ending in a
			// hard cut when the tab strip scrolls.
			if a.tabScroll.X > 1 {
				ui.Box(c).Absolute().Left(0).Top(0).Width(36).Height(titleBarHeight).
					Gradient(th.Surface, ui.Transparent, 90).PassThrough()
			}
			if a.tabScroll.MaxX-a.tabScroll.X > 1 {
				ui.Box(c).Absolute().Right(0).Top(0).Width(36).Height(titleBarHeight).
					Gradient(ui.Transparent, th.Surface, 90).PassThrough()
			}
		})
		if a.scrollActiveTab {
			a.scrollActiveTab = false
		}
		// The native menu is keyboard-only once the app draws its own title
		// bar, so keep the update command visible in the title bar. A newer
		// version tints the icon until the user opens or dismisses the panel.
		// The new-tab action stays next to the tab strip; both use the same
		// plain 30×30 face, so the old raised capsule no longer dominates the
		// title bar.
		components.IconButton(c, lucide.Plus(), "新建标签页 (Ctrl+T)", components.IconButtonPlain).
			Size(30, 30).Tooltip("新建标签页 (Ctrl+T)").
			OnClick(func() { a.later(func() { a.newTab("") }) })
		updateBtn := components.IconButton(c, lucide.CloudDownload(), "检查更新", components.IconButtonPlain).
			Size(30, 30).Tooltip("从 GitHub Release 检查签名更新")
		if a.updateAvailable != nil {
			updateBtn.TextColor(th.Accent)
		}
		updateBtn.OnClick(a.checkForUpdates)
	})
}

// tabCloseButton is deliberately quieter than a normal button: a tab row with
// five bordered close buttons reads as a row of tiny input fields.
func (a *App) tabCloseButton(c *ui.Context, tabName string, active, tabHovered bool) ui.Element {
	th := c.Theme()
	b := ui.ButtonBase(c).Label("关闭标签页 "+tabName).Tooltip("关闭标签页").
		Size(18, 18).Padding(0).Radius(5)
	b.Draw(func(p *ui.Painter, r ui.Rect) {
		switch {
		case b.Pressed():
			p.Fill(r, th.Text.Alpha(.13), 5)
		case b.Hovered():
			p.Fill(r, th.Text.Alpha(.07), 5)
		}
	})
	b.Children(func() {
		fg := th.TextMuted.Alpha(.52)
		if active || tabHovered {
			fg = th.TextMuted.Alpha(.8)
		}
		if b.Hovered() {
			fg = th.Text
		}
		components.Icon(c, lucide.X()).Size(11, 11).TextColor(fg)
	})
	return b
}

func (a *App) toolbar(c *ui.Context) {
	th := c.Theme()
	tab := a.tab()
	if tab == nil {
		return
	}
	winW, _ := c.Size()
	compact := winW < 1120
	searchVisible := !compact || a.searchOpen || tab.Query != "" || a.focusSearch

	buildSearch := func() {
		if !searchVisible {
			components.IconButton(c, lucide.Search(), "搜索 (Ctrl+F)", components.IconButtonPlain).
				OnClick(func() {
					a.searchOpen = true
					a.focusSearch = true
					a.refresh()
				})
			return
		}
		width := float32(230)
		if compact {
			width = 150
		}
		ui.Box(c.Key(fmt.Sprintf("search-%d", tab.ID))).Width(width).MinWidth(120).Children(func() {
			placeholder := "搜索此文件夹"
			if a.settings.Recursive {
				placeholder = "搜索（含子文件夹）"
			}
			components.TextBox(c, &tab.Query, components.TextBoxOptions{
				LeadingIcon: lucide.Search(),
			}).Placeholder(placeholder).Label("搜索").FillWidth().Bind(&a.searchHandle)
		})
		if a.focusSearch {
			a.searchHandle.SelectAll(c)
			a.focusSearch = false
		}
		if tab.Query != tab.AppliedQuery {
			tab.refilter()
			a.refresh()
		}
		if tab.Query != "" {
			components.IconButton(c, lucide.X(), "清除搜索", components.IconButtonCompact).
				OnClick(func() {
					tab.Query = ""
					tab.refilter()
					if compact {
						a.searchOpen = false
					}
					a.refresh()
				})
		} else if compact {
			components.IconButton(c, lucide.X(), "收起搜索", components.IconButtonCompact).
				OnClick(func() {
					a.searchOpen = false
					a.refresh()
				})
		}
	}

	buildFilter := func() {
		index := 0
		switch tab.Filter {
		case "image":
			index = 1
		case "video":
			index = 2
		}
		before := index
		tabs := components.Tabs(c, &index, components.TabsSegmented, "全部", "图片", "视频")
		// Segmented tabs align to the top of their track by default. The command
		// row centers its controls, so opt the track into the row's center line.
		tabs.List.AlignSelf(ui.Center)
		tabs.List.Label("显示内容，文件夹始终保留")
		if index != before {
			values := []string{"all", "image", "video"}
			a.later(func() { a.setMediaFilter(tab, values[index]) })
		}
	}

	// Sort field and direction are one control. They act on the same setting —
	// which order the folder is in — and as two neighbours, a bordered select
	// next to a bare arrow, they read as unrelated buttons.
	//
	// Sort field and direction are one control: one row per field, and the
	// direction is a property of the field already in use. Choosing the field
	// that is already sorted flips it, the way a column header does in a file
	// list; choosing another switches to it, ascending. Listing both directions
	// as separate rows would double the menu for one setting, and a button
	// nested in the trigger is invisible to the accessibility tree.
	buildSort := func() {
		sortWidth := float32(104)
		if compact {
			sortWidth = 92
		}
		sortSel := components.Select(c.Key("sort"), &tab.Sort)
		sortSel.Trigger.Width(sortWidth).Label("排序方式，再次选择同一项切换升序降序")
		sortSel.Trigger.Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(3).Children(func() {
				components.Icon(c, sortDirectionIcon(tab.Desc)).Size(12, 12).
					TextColor(th.TextMuted).Shrink(0)
				components.Text(c, sortLabel(tab.Sort)).Grow(1).MinWidth(0).
					SingleLine().FontSize(12).TextColor(th.Text)
			})
		})
		// A click on the field already in use does not change the select's
		// value, so its Changed stays quiet; the row reports the click itself.
		// The field is captured before the select runs, because applying the
		// click already writes the new one into tab.Sort.
		beforeSort := tab.Sort
		flip, switched := false, ""
		sortSel.Popup(func(panel ui.Element) {
			for _, key := range []string{"name", "date", "size", "type"} {
				key := key
				item := sortSel.Item(key)
				item.Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
						components.Text(c, sortLabel(key)).Grow(1).SingleLine()
						if key == tab.Sort {
							// The direction belongs to the field in use, so it
							// shows on that row only.
							ui.Row(c).AlignItems(ui.Center).Gap(3).Children(func() {
								components.Text(c, sortDirectionLabel(tab.Desc)).FontSize(10).
									TextColor(th.TextMuted)
								components.Icon(c, sortDirectionIcon(tab.Desc)).Size(11, 11).
									TextColor(th.TextMuted)
							})
						}
					})
				})
				if item.Clicked() {
					if key == beforeSort {
						flip = true
					} else {
						switched = key
					}
				}
			}
		})
		if flip {
			tab.Desc = !tab.Desc
			a.persist()
			a.loadTab(tab, true)
		} else if switched != "" {
			// The select wrote the new field; a field change starts ascending.
			tab.Sort, tab.Desc = switched, false
			a.persist()
			a.loadTab(tab, true)
		} else if sortSel.Trigger.Changed() {
			// A row that does not report its own click still changed the
			// field, as assistive technology choosing it does.
			tab.Desc = false
			a.persist()
			a.loadTab(tab, true)
		}
	}

	// Recursive scanning, card captions and hidden files are set-and-forget
	// options. Search, media filtering, sorting and navigation stay in the
	// command row; secondary navigation and folder actions live here.
	buildViewMenu := func() {
		more := components.IconButton(c, lucide.Ellipsis(), "更多操作", components.IconButtonPlain)
		components.MenuButton(c, more, func(m *components.Menu) {
			m.Group("当前文件夹", func(m *components.Menu) {

				if m.Item("上一文件夹").Chosen() {
					a.later(func() { a.sibling(-1) })
				}
				if m.Item("下一文件夹").Chosen() {
					a.later(func() { a.sibling(1) })
				}
				if m.Item("选择其他文件夹…").Chosen() {
					a.later(a.pickFolder)
				}
			})
			m.Group("显示选项", func(m *components.Menu) {
				rec := m.Item("递归扫描子文件夹").Checked(a.settings.Recursive)
				if rec.Chosen() {
					a.settings.Recursive = !a.settings.Recursive
					_ = saveSettings(a.settings)
					a.loadTab(tab, true)
				}
				names := m.Item("显示卡片文件名").Checked(a.settings.ShowCardNames)
				if names.Chosen() {
					a.settings.ShowCardNames = !a.settings.ShowCardNames
					_ = saveSettings(a.settings)
					a.refresh()
				}
				hidden := m.Item("显示隐藏文件").Checked(a.settings.ShowHidden)
				if hidden.Chosen() {
					a.settings.ShowHidden = !a.settings.ShowHidden
					_ = saveSettings(a.settings)
					a.treeGen++
					a.treeLoaded = map[string]bool{}
					a.treeKids = map[string][]TreeEntry{}
					a.loadTab(tab, true)
				}
			})
		})
	}

	divider := func() {
		ui.Box(c).Width(1).Height(20).Background(th.Border.Alpha(.55)).Margin(2, 6)
	}

	ui.Row(c).Height(toolbarHeight).FillWidth().Padding(0, 14).Gap(4).
		AlignItems(ui.Center).Background(th.Background).
		BorderWidth(0, 0, 1, 0).BorderColor(th.Border.Alpha(.5)).Children(func() {
		components.IconButton(c, lucide.ArrowLeft(), "返回", components.IconButtonPlain).
			Disabled(len(tab.History) == 0).OnClick(func() { a.goBack() })
		components.IconButton(c, lucide.ArrowUp(), "上一级", components.IconButtonPlain).
			OnClick(a.up)
		divider()

		if crumbs, paths := breadcrumbItems(tab.Path, tab.Name); len(crumbs) > 0 && !compact {
			chosen := -1
			ui.Box(c).Grow(1).MinWidth(0).Clip().Children(func() {
				b := components.Breadcrumbs(c, crumbs, &chosen)
				b.Label("当前位置：" + tab.Path)
				if b.Changed() && chosen >= 0 && chosen < len(paths) {
					path := paths[chosen]
					a.later(func() { a.goTo(path, true) })
				}
			})
		} else if compact && searchVisible {
			ui.Spacer(c)
		} else {
			components.Text(c, tab.Name).FontSize(13).FontWeight(600).SingleLine().Ellipsis("…")
		}

		// Star the folder being viewed. This keeps a place, unlike the
		// favorites folder, which collects images and videos.
		if path := tab.Path; path != "" && !isFavoritesPath(path) && !isRemotePath(path) {
			starred := a.isStarredPath(path)
			icon := lucide.Star()
			label := "收藏此路径"
			if starred {
				label = "取消收藏此路径"
			}
			button := components.IconButton(c, icon, label, components.IconButtonPlain)
			if starred {
				button.TextColor(th.Accent)
			}
			button.OnClick(func() {
				a.later(func() { a.toggleStarredPath(path) })
			})
		}

		buildSearch()
		buildFilter()
		buildSort()
		divider()
		// Refresh sits outside the overflow menu: it is the one action used
		// repeatedly while browsing, and hiding it behind a menu made a folder
		// refresh a two-step operation.
		components.IconButton(c, lucide.RefreshCw(), "刷新 (F5)", components.IconButtonPlain).
			OnClick(func() {
				t := tab
				a.later(func() { a.loadTab(t, false) })
			})
		buildViewMenu()
		if !a.sidebar {
			components.IconButton(c, lucide.Settings(), "设置", components.IconButtonPlain).
				OnClick(a.openSettings)
		}
	})
}

// breadcrumbItems turns a Windows path into labels and the absolute path each
// ancestor link should open. The final item is the current folder and is not
// interactive, matching components.Breadcrumbs.
func breadcrumbItems(path, name string) ([]string, []string) {
	path = cleanDisplayPath(path)
	if path == "" || path == "." {
		return nil, nil
	}
	if isRemotePath(path) {
		return remoteBreadcrumbItems(path, name)
	}
	volume := filepath.VolumeName(path)
	root := volume
	if root == "" {
		root = string(filepath.Separator)
	} else if !strings.HasSuffix(root, string(filepath.Separator)) {
		root += string(filepath.Separator)
	}
	rootLabel := volume
	if rootLabel == "" {
		rootLabel = string(filepath.Separator)
	}
	rest := strings.TrimPrefix(path, root)
	rest = strings.Trim(rest, string(filepath.Separator))
	labels := []string{rootLabel}
	paths := []string{root}
	if rest != "" {
		current := root
		for _, segment := range strings.Split(rest, string(filepath.Separator)) {
			if segment == "" {
				continue
			}
			current = filepath.Join(current, segment)
			labels = append(labels, segment)
			paths = append(paths, current)
		}
	}
	if name != "" && len(labels) > 1 {
		labels[len(labels)-1] = name
	}
	return labels, paths
}

// remoteBreadcrumbItems splits smb://nas/photos/2024 into a scheme root and
// the folders below it. The share is part of the root link: a NAS has nothing
// to show above it, and listing one would be another network round trip.
// remoteBreadcrumbItems splits a virtual path into crumbs, starting at the
// share's own root rather than at its host.
//
// The first crumb must be the share root: "webdav://host:5005" on its own is
// not a location the app can open — it is above the share, and clicking it
// produced "远程位置已不存在". The crumb is labelled with the alias when the
// share has one, so the address stays out of the toolbar.
func remoteBreadcrumbItems(path, name string) ([]string, []string) {
	share, ok := remoteShareForPath(path)
	if !ok {
		// An unsaved path: show it whole rather than inventing a hierarchy.
		return []string{path}, []string{path}
	}
	root := remoteRoot(share)
	rootLabel := remoteAlias(share)
	if rootLabel == "" {
		rootLabel = root
	}
	labels := []string{rootLabel}
	paths := []string{root}
	// Everything below the share root becomes its own crumb. The base path is
	// part of the root, so it must not appear again here.
	rest := strings.Trim(strings.TrimPrefix(strings.TrimSuffix(path, "/"), root), "/")
	if rest != "" {
		current := root
		for _, segment := range strings.Split(rest, "/") {
			if segment == "" {
				continue
			}
			current += "/" + segment
			labels = append(labels, unescapeRemoteName(segment))
			paths = append(paths, current)
		}
	}
	if name != "" && len(labels) > 1 {
		labels[len(labels)-1] = name
	}
	return labels, paths
}

func (a *App) setMediaFilter(tab *Tab, filter string) {
	filter = validMediaFilter(filter)
	if tab == nil || tab.Filter == filter {
		return
	}
	tab.Filter = filter
	tab.refilter()
	a.persist()
	a.refresh()
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
		emptyState(c, lucide.FolderOpen(), "没有打开的文件夹", "", func(c *ui.Context) {
			components.Button(c, "选择文件夹", components.ButtonPrimary).OnClick(a.pickFolder)
		})
		return
	}
	switch {
	case tab.Loading:
		loadingState(c)
	case tab.Err != "":
		ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
			components.Icon(c, lucide.ImageOff()).Size(30, 30).TextColor(th.TextMuted)
			components.Text(c, tab.Err).TextColor(th.Text).MaxWidth(520).Center()
			components.Button(c, "重试", components.ButtonSecondary).OnClick(func() { a.loadTab(tab, false) })
		})
	case len(tab.Items) == 0:
		a.emptyBrowseState(c, tab)
	default:
		a.gridView(c, tab)
	}
}

func (a *App) emptyBrowseState(c *ui.Context, tab *Tab) {
	if tab.Query != "" {
		emptyState(c, lucide.SearchX(), "没有匹配的内容", "换个关键词，或清除当前筛选后再看。", func(c *ui.Context) {
			ui.Row(c).Gap(8).Justify(ui.Center).Children(func() {
				components.Button(c, "清除搜索", components.ButtonSecondary).OnClick(func() {
					tab.Query = ""
					tab.refilter()
					a.refresh()
				})
				if tab.Filter != "all" {
					components.Button(c, "显示全部", components.ButtonSecondary).OnClick(func() {
						a.setMediaFilter(tab, "all")
					})
				}
			})
		})
		return
	}
	if tab.Filter == "image" || tab.Filter == "video" {
		kind := "图片"
		icon := lucide.Images()
		if tab.Filter == "video" {
			kind = "视频"
			icon = lucide.Video()
		}
		emptyState(c, icon, "没有可显示的"+kind, "当前仅展示"+kind+"；文件夹仍会保留，方便继续导航。", func(c *ui.Context) {
			components.Button(c, "显示全部", components.ButtonSecondary).OnClick(func() {
				a.setMediaFilter(tab, "all")
			})
		})
		return
	}
	emptyState(c, lucide.Images(), "此文件夹没有可显示的内容", "把图片、视频或文件夹拖进来，或选择其他文件夹。", func(c *ui.Context) {
		ui.Row(c).Gap(8).Justify(ui.Center).Children(func() {
			components.Button(c, "选择文件夹", components.ButtonSecondary).OnClick(a.pickFolder)
			components.Button(c, "重新读取", components.ButtonSecondary).OnClick(func() {
				a.loadTab(tab, false)
			})
		})
	})
}

func loadingState(c *ui.Context) {
	th := c.Theme()
	ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
		components.Spinner(c, components.SpinnerRing).Size(30, 30).Label("正在读取")
		components.Text(c, "正在读取…").FontSize(13).TextColor(th.TextMuted)
	})
}

func emptyState(c *ui.Context, icon *ui.SVG, title, detail string, action func(*ui.Context)) {
	th := c.Theme()
	ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
		ui.Box(c).Size(64, 64).Radius(18).Background(th.Accent.Alpha(.08)).
			BorderWidth(1).BorderColor(th.Accent.Alpha(.14)).Center().Children(func() {
			components.Icon(c, icon).Size(28, 28).TextColor(th.Accent.Alpha(.88))
		})
		components.Text(c, title).FontSize(14).FontWeight(600).TextColor(th.Text).Center()
		if detail != "" {
			components.Text(c, detail).FontSize(11.5).TextColor(th.TextMuted).MaxWidth(460).Center()
		}
		if action != nil {
			action(c)
		}
	})
}

// selectionBar is the floating action bar for a multi-select run. It appears
// only in multi-select mode: a single click selects one tile for the keyboard
// and the context menu, and a bar following every click was noise.
func (a *App) selectionBar(c *ui.Context) {
	th := c.Theme()
	tab := a.tab()
	if tab == nil || !tab.MultiSelect || len(tab.Selected) == 0 {
		return
	}
	if a.showSettings || a.showHelp || a.showAbout || a.updateOpen || a.errorMsg != "" ||
		a.renameOpen || a.newFolderOpen || a.confirmDeleteOpen || a.viewer.Open {
		return
	}
	winW, _ := c.Size()
	left := a.sidebarLayoutWidth(winW)
	width := winW - left
	one := tab.selectedOne()
	n := len(tab.Selected)
	readOnly := a.tabReadOnly()
	allFavorited := a.selectionAllFavorited(tab)
	favLabel, favIcon := "收藏", lucide.Star
	if allFavorited {
		favLabel, favIcon = "取消收藏", lucide.StarOff
	}
	// "Open" and "Locate" only make sense for one tile, so they disappear in a
	// multi-selection instead of sitting there greyed out: a bar of disabled
	// buttons reads as a broken toolbar.
	items := make([]components.FloatingButtonGroupItem, 0, 8)
	if one != nil {
		items = append(items, components.FloatingButtonGroupItem{ID: "open", Label: "打开", Icon: lucide.ExternalLink})
	}
	items = append(items,
		components.FloatingButtonGroupItem{ID: "favorite", Label: favLabel, Icon: favIcon},
		components.FloatingButtonGroupItem{ID: "copy", Label: "复制路径", Icon: lucide.Copy},
	)
	if !readOnly {
		if one != nil {
			items = append(items, components.FloatingButtonGroupItem{ID: "reveal", Label: "定位", Icon: lucide.Locate})
		}
		items = append(items,
			components.FloatingButtonGroupItem{Separator: true},
			components.FloatingButtonGroupItem{ID: "delete", Label: "删除", Icon: lucide.Trash, Shortcut: "Delete"},
		)
	}
	items = append(items, components.FloatingButtonGroupItem{ID: "clear", Label: "取消选择", Icon: lucide.X, Shortcut: "Esc"})
	ui.Overlay(c, func() {
		ui.Row(c).Absolute().Left(left).Width(width).Bottom(bottomBarHeight + 12).Height(52).
			Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				// The count alone says how many, not what: a run of videos and a
				// run of folders read the same. One short breakdown answers the
				// question the actions beside it are about to act on.
				ui.Box(c).Padding(9, 4).Radius(pillRadius).Background(th.Surface).
					BorderWidth(1).BorderColor(th.Border.Alpha(.8)).Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						components.Text(c, fmt.Sprintf("已选 %d 项", n)).FontSize(11).FontWeight(600).
							TextColor(th.Text)
						if detail := selectionBreakdown(tab); detail != "" {
							components.Text(c, detail).FontSize(10).TextColor(th.TextMuted).SingleLine()
						}
					})
				})
				group := components.FloatingButtonGroup(c, "选中项操作", components.FloatingButtonGroupRaised, ui.Horizontal, items)
				switch group.Clicked() {
				case "open":
					if one != nil {
						a.later(func() { a.activateItem(one) })
					}
				case "favorite":
					a.later(a.toggleItemFavorites)
				case "reveal":
					a.later(a.revealSelected)
				case "copy":
					a.later(a.copySelectedPath)
				case "delete":
					a.later(func() { a.askDelete(false) })
				case "clear":
					a.later(a.exitMultiSelect)
				}
			})
		})
	})
}

// selectionAllFavorited reports whether every selected media file is already
// starred, which decides whether the bar's button adds or removes.
func (a *App) selectionAllFavorited(tab *Tab) bool {
	media := 0
	for _, it := range tab.selectedItems() {
		if !isMediaKind(it.Kind) {
			continue
		}
		media++
		if !a.isFavorite(it.Path) {
			return false
		}
	}
	return media > 0
}

func (a *App) statusBar(c *ui.Context) {
	th := c.Theme()
	tab := a.tab()
	if tab == nil {
		return
	}
	ui.Row(c).Height(bottomBarHeight).Padding(0, 14).Gap(10).AlignItems(ui.Center).Background(th.Surface).
		Children(func() {
			count := fmt.Sprintf("%d 项", len(tab.Items))
			filtered := tab.Query != "" || (tab.Filter != "" && tab.Filter != "all")
			if filtered && len(tab.Items) != len(tab.All) {
				count = fmt.Sprintf("%d / %d 项", len(tab.Items), len(tab.All))
			}
			components.Text(c, count).FontSize(11).TextColor(th.TextMuted).AlignSelf(ui.Center)
			counts := fmt.Sprintf("%d 图片 · %d 视频", countKind(tab.Items, "image"), countKind(tab.Items, "video"))
			if n := countKind(tab.Items, "folder"); n > 0 {
				counts += fmt.Sprintf(" · %d 文件夹", n)
			}
			components.Text(c, counts).FontSize(11).TextColor(th.TextMuted).AlignSelf(ui.Center)
			if tab.Result.Truncated {
				components.Text(c, "已截断").FontSize(11).TextColor(th.TextMuted).AlignSelf(ui.Center)
			}
			ui.Spacer(c)
			a.taskChip(c, th)
			if a.settings.Recursive {
				components.Badge(c, "递归", components.BadgeOptions{Tone: components.BadgeWarning}).
					AlignSelf(ui.Center)
			}
			if a.settings.ShowHidden {
				components.Badge(c, "隐藏", components.BadgeOptions{Tone: components.BadgeNeutral}).
					AlignSelf(ui.Center)
			}
			if t := a.tab(); t != nil && isFavoritesPath(t.Path) {
				// The favorites folder is read-only for a different reason than
				// a share, and the badge should say which.
				components.Badge(c, "虚拟目录", components.BadgeOptions{Tone: components.BadgeNeutral}).
					AlignSelf(ui.Center)
			} else if a.tabReadOnly() {
				components.Badge(c, "网络 · 只读", components.BadgeOptions{Tone: components.BadgeNeutral}).
					AlignSelf(ui.Center)
			}
			if notice := a.statusNoticeText(); notice != "" {
				components.Text(c, notice).FontSize(10.5).TextColor(th.Danger).
					AlignSelf(ui.Center).SingleLine().Ellipsis("…")
			}
			components.Text(c, "F1 快捷键").FontSize(10).TextColor(th.TextMuted.Alpha(.88)).
				AlignSelf(ui.Center)
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

// selectionBreakdown summarises a multi-selection by kind, skipping zero counts
// so the pill stays short: "3 图片 · 1 视频".
func selectionBreakdown(tab *Tab) string {
	if tab == nil || len(tab.Selected) < 2 {
		return ""
	}
	images, videos, folders, others := 0, 0, 0, 0
	for _, it := range tab.selectedItems() {
		switch it.Kind {
		case "image":
			images++
		case "video":
			videos++
		case "folder":
			folders++
		default:
			others++
		}
	}
	parts := make([]string, 0, 4)
	add := func(n int, label string) {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, label))
		}
	}
	add(images, "图片")
	add(videos, "视频")
	add(folders, "文件夹")
	add(others, "其他")
	return strings.Join(parts, " · ")
}

// sortDirectionLabel names the two orders for the sort menu.
func sortDirectionLabel(desc bool) string {
	if desc {
		return "降序"
	}
	return "升序"
}

// sortDirectionIcon is the arrow the sort control shows for an order.
func sortDirectionIcon(desc bool) *ui.SVG {
	if desc {
		return lucide.ArrowDown()
	}
	return lucide.ArrowUp()
}

// sidebarStarredPathRow is one starred folder in the 收藏 section. Its tooltip
// carries the full path, because the label is only the last segment.
func (a *App) sidebarStarredPathRow(c *ui.Context, f Favorite) {
	th := c.Theme()
	active := false
	if t := a.tab(); t != nil {
		active = strings.EqualFold(cleanDisplayPath(f.Path), cleanDisplayPath(t.Path))
	}
	row := components.SidebarItemWithLeading(c, favoriteLabel(f), active, func(c *ui.Context, size float32) {
		color := th.TextMuted.Alpha(.9)
		if active {
			color = th.Accent
		}
		components.Icon(c, lucide.FolderBookmark()).Size(size, size).TextColor(color)
	})
	row.Key("starred-" + favoriteKey(f.Path))
	row.Tooltip(f.Path)
	if row.Clicked() {
		a.goTo(f.Path, true)
	}
	components.ContextMenu(c, row, func(m *components.Menu) {
		if m.Item("打开").Chosen() {
			a.later(func() { a.goTo(f.Path, true) })
		}
		if m.Item("在资源管理器中显示").Chosen() {
			a.later(func() { _ = (AppSvc{}).OpenFolder(f.Path) })
		}
		if m.Item("取消收藏").Chosen() {
			a.later(func() { a.toggleStarredPath(f.Path) })
		}
	})
}
