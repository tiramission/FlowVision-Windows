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

	a.overlays(c)
	a.runPending()
}

const titleBarHeight float32 = 48

// The sidebar footer and the workbench status bar share one height so their
// top hairlines form a single full-width line, not two steps.
const bottomBarHeight float32 = 40

// shellHairlines paints the frame's internal joints after all panels. A
// 1 DIP line at the sidebar edge runs continuously through the brand header,
// navigation, footer and status bar; the title-bar and bottom dividers run the
// full width. Painting these here avoids child borders fighting over the same
// corner pixels.
func (a *App) shellHairlines(c *ui.Context, winH float32, th *ui.Theme) {
	// Resolve the hairline against the panel surface once. Drawing a translucent
	// rule twice at a T-junction darkened the joint by another alpha step; an
	// opaque pre-blended color keeps every segment the same weight.
	hair := th.Border.Alpha(.55).Over(th.Surface)
	ui.Box(c).Absolute().Left(0).Right(0).Top(titleBarHeight).Height(1).
		Background(hair).PassThrough()
	ui.Box(c).Absolute().Left(0).Right(0).Top(winH - bottomBarHeight).Height(1).
		Background(hair).PassThrough()
	if a.sidebar {
		ui.Box(c).Absolute().Left(float32(a.sidebarWidth) - 1).Top(0).Width(1).Height(winH).
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
		a.focusSearch = true
		a.refresh()
	})
}

func (a *App) sidebarView(c *ui.Context) {
	th := c.Theme()
	side := ui.Column(c).Width(float32(a.sidebarWidth)).FillHeight().Background(th.Surface)
	side.Children(func() {
		// The sidebar owns the app mark at the top of its full-height column.
		// The workbench title bar starts to its right, so the tab row and its
		// left hairline share one continuous boundary instead of two offsets.
		a.brandHeader(c)
		ui.Scroll(c).Grow(1).Padding(8, 6, 8, 8).Children(func() {
			components.Sidebar(c).Children(func() {
				components.SidebarSection(c, "收藏", func() {
					if len(a.settings.Favorites) == 0 {
						components.Text(c, "用工具栏的星标收藏文件夹").FontSize(10.5).TextColor(th.TextMuted).
							Padding(2, 6).SingleLine()
					}
					for _, fav := range a.settings.Favorites {
						fav := fav
						a.sidebarPathRow(c, fav, lucide.Star(), true)
					}
				})

				components.SidebarSection(c, "位置", func() {
					for _, p := range a.places {
						p := p
						a.sidebarPathRow(c, p.Path, placeIcon(p.Icon), false, p.Name)
					}
				})

				components.SidebarSection(c, "驱动器", func() {
					for _, p := range a.drives {
						p := p
						a.sidebarPathRow(c, p.Path, lucide.HardDrive(), false)
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
				components.Text(c, "v"+appVersion()).FontSize(9.5).TextColor(th.TextMuted.Alpha(.75)).Padding(0, 4)
			})
	})
}

func (a *App) sidebarPathRow(c *ui.Context, path string, icon *ui.SVG, favorite bool, label ...string) {
	th := c.Theme()
	active := false
	if t := a.tab(); t != nil {
		active = strings.EqualFold(filepath.Clean(path), filepath.Clean(t.Path))
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
	p := filepath.Clean(t.Path)
	a.ensureTree(p)
	rootName := t.Name
	if rootName == "" {
		rootName = displayName(p)
	}
	tree := ui.TreeBase(c)
	tree.Root.FillWidth().MinWidth(0).Gap(1).Children(func() {
		a.folderTreeRow(c, tree, p, rootName, true, false)
		if a.treeLoaded[p] && len(a.treeKids[p]) == 0 {
			a.folderTreeNote(c, "没有子文件夹", false, 39)
		}
	})
}

// folderTreeCount adds the immediate child count to the section heading. A
// single count is easier to scan than a longer "N subfolders" sentence, so the
// hover tooltip spells it out.
func (a *App) folderTreeCount(c *ui.Context, t *Tab) {
	p := filepath.Clean(t.Path)
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
func (a *App) folderTreeRow(c *ui.Context, tree ui.TreeParts, path, name string, root, hasChildren bool) {
	th := c.Theme()
	p := filepath.Clean(path)
	selected := false
	if t := a.tab(); t != nil {
		selected = strings.EqualFold(p, filepath.Clean(t.Path))
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
				a.folderTreeRow(c, tree, kid.Path, kid.Name, false, kid.HasChildren)
			}
		})
	}
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

	buildSearch := func() {
		ui.Box(c.Key(fmt.Sprintf("search-%d", tab.ID))).Grow(1).MinWidth(0).MaxWidth(420).Children(func() {
			components.TextBox(c, &tab.Query, components.TextBoxOptions{
				LeadingIcon: lucide.Search(),
			}).Placeholder("搜索此文件夹").Label("搜索").FillWidth().Bind(&a.searchHandle)
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
					a.refresh()
				})
		}
	}

	buildSort := func() {
		sortSel := components.Select(c.Key("sort"), &tab.Sort)
		sortSel.Trigger.Width(92).Label("排序")
		sortSel.Trigger.Children(func() {
			components.Text(c, sortLabel(tab.Sort)).SingleLine()
		})
		sortSel.Popup(func(panel ui.Element) {
			for _, key := range []string{"name", "date", "size", "type"} {
				key := key
				item := sortSel.Item(key)
				item.Children(func() {
					components.Text(c, sortLabel(key)).SingleLine()
				})
			}
		})
		if sortSel.Trigger.Changed() {
			a.persist()
			a.loadTab(tab, true)
		}
		dirIcon := lucide.ArrowUp()
		dirLabel := "升序，点击切换为降序"
		if tab.Desc {
			dirIcon = lucide.ArrowDown()
			dirLabel = "降序，点击切换为升序"
		}
		components.IconButton(c, dirIcon, dirLabel, components.IconButtonPlain).
			OnClick(func() {
				tab.Desc = !tab.Desc
				a.persist()
				a.loadTab(tab, true)
			})
	}

	// Recursive scanning, card captions and hidden files are set-and-forget
	// view options. Keeping them behind one menu leaves search and sort as the
	// only always-visible controls in the workbench.
	buildViewMenu := func() {
		more := components.IconButton(c, lucide.Ellipsis(), "更多视图选项", components.IconButtonPlain)
		components.MenuButton(c, more, func(m *components.Menu) {
			m.Group("显示内容", func(m *components.Menu) {
				all := m.Item("图片和视频").Checked(tab.Filter == "" || tab.Filter == "all")
				if all.Chosen() {
					a.setMediaFilter(tab, "all")
				}
				images := m.Item("仅展示图片").Checked(tab.Filter == "image")
				if images.Chosen() {
					a.setMediaFilter(tab, "image")
				}
				videos := m.Item("仅展示视频").Checked(tab.Filter == "video")
				if videos.Chosen() {
					a.setMediaFilter(tab, "video")
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

	ui.Column(c).Padding(10, 14, 8, 14).Gap(8).Background(th.Background).Children(func() {
		ui.Row(c).Gap(2).Children(func() {
			components.IconButton(c, lucide.ArrowLeft(), "返回", components.IconButtonPlain).
				Disabled(len(tab.History) == 0).OnClick(func() { a.goBack() })
			components.IconButton(c, lucide.ArrowUp(), "上一级", components.IconButtonPlain).
				OnClick(a.up)
			components.IconButton(c, lucide.ChevronLeft(), "上一文件夹", components.IconButtonPlain).
				OnClick(func() { a.sibling(-1) })
			components.IconButton(c, lucide.ChevronRight(), "下一文件夹", components.IconButtonPlain).
				OnClick(func() { a.sibling(1) })
			ui.Box(c).Width(1).Height(20).Background(th.Border.Alpha(.55)).Margin(2, 6)
			components.IconButton(c, lucide.RefreshCw(), "刷新当前文件夹 (F5)", components.IconButtonPlain).
				Disabled(tab.Loading).
				OnClick(func() { a.loadTab(tab, false) })

			ui.Column(c).Grow(1).MinWidth(0).PaddingX(8).Clip().Children(func() {
				components.Text(c, tab.Name).FontSize(17).FontWeight(600).SingleLine().Ellipsis("…")
				components.Text(c, tab.Path).FontSize(10).TextColor(th.TextMuted).SingleLine().Ellipsis("…")
			})
			star := components.IconButton(c, lucide.Star(), "收藏此文件夹", components.IconButtonPlain)
			if a.isFavorite(tab.Path) {
				star.TextColor(th.Accent)
			}
			if star.Clicked() {
				a.toggleFavorite()
			}
			components.IconButton(c, lucide.Settings(), "设置", components.IconButtonPlain).
				OnClick(a.openSettings)
		})
		ui.Row(c).Gap(6).Children(func() {
			buildSearch()
			buildSort()
			divider()
			buildViewMenu()
		})
	})
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
		emptyState(c, lucide.FolderOpen(), "没有打开的文件夹", "")
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
		if tab.Query != "" {
			ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
				components.Icon(c, lucide.SearchX()).Size(30, 30).TextColor(th.TextMuted)
				components.Text(c, "没有匹配的图片").TextColor(th.TextMuted)
				components.Button(c, "清除搜索", components.ButtonSecondary).OnClick(func() {
					tab.Query = ""
					tab.refilter()
					a.refresh()
				})
			})
		} else {
			emptyState(c, lucide.Images(), "此文件夹没有可显示的图片", "把图片或文件夹拖进来，或换一个文件夹")
		}
	default:
		a.gridView(c, tab)
	}
}

func loadingState(c *ui.Context) {
	th := c.Theme()
	ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
		components.Spinner(c, components.SpinnerRing).Size(30, 30).Label("正在读取")
		components.Text(c, "正在读取…").FontSize(13).TextColor(th.TextMuted)
	})
}

func emptyState(c *ui.Context, icon *ui.SVG, title, detail string) {
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
	})
}

func (a *App) statusBar(c *ui.Context) {
	th := c.Theme()
	tab := a.tab()
	if tab == nil {
		return
	}
	ui.Row(c).Height(bottomBarHeight).Padding(0, 14).Gap(10).AlignItems(ui.Center).Background(th.Surface).
		Children(func() {
			components.Text(c, fmt.Sprintf("%d 项", len(tab.Items))).FontSize(11).TextColor(th.TextMuted).
				AlignSelf(ui.Center)
			if tab.Query != "" && len(tab.Items) != len(tab.All) {
				components.Text(c, fmt.Sprintf("（共 %d 项）", len(tab.All))).FontSize(11).TextColor(th.TextMuted).
					AlignSelf(ui.Center)
			}
			counts := fmt.Sprintf("%d 图片 · %d 视频", countKind(tab.Items, "image"), countKind(tab.Items, "video"))
			if n := countKind(tab.Items, "folder"); n > 0 {
				counts += fmt.Sprintf(" · %d 文件夹", n)
			}
			components.Text(c, counts).FontSize(11).TextColor(th.TextMuted).AlignSelf(ui.Center)
			if n := len(tab.Selected); n > 0 {
				components.Text(c, fmt.Sprintf("已选 %d 项", n)).FontSize(11).TextColor(th.Accent).
					AlignSelf(ui.Center)
			}
			if tab.Result.Truncated {
				components.Text(c, "已截断").FontSize(11).TextColor(th.TextMuted).AlignSelf(ui.Center)
			}
			ui.Spacer(c)
			if a.settings.Recursive {
				components.Badge(c, "递归", components.BadgeOptions{Tone: components.BadgeWarning}).
					AlignSelf(ui.Center)
			}
			switch tab.Filter {
			case "image":
				components.Badge(c, "仅图片", components.BadgeOptions{Tone: components.BadgeNeutral}).
					AlignSelf(ui.Center)
			case "video":
				components.Badge(c, "仅视频", components.BadgeOptions{Tone: components.BadgeNeutral}).
					AlignSelf(ui.Center)
			}
			if a.settings.ShowHidden {
				components.Badge(c, "隐藏", components.BadgeOptions{Tone: components.BadgeNeutral}).
					AlignSelf(ui.Center)
			}
			if !a.settings.ShowCardNames {
				components.Badge(c, "文件名已隐藏", components.BadgeOptions{Tone: components.BadgeNeutral}).
					AlignSelf(ui.Center)
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
