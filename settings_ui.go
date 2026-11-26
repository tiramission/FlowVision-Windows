package main

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// settingsModal presents the full settings catalog in Onevra's searchable
// Settings surface. All controls bind to retained draft fields; this keeps an
// edit alive across the rebuild that delivers its Changed notice.
func (a *App) settingsModal(c *ui.Context) {
	if !a.showSettings || !a.settingsModalOpen {
		return
	}
	winW, winH := c.Size()
	w := min(float32(880), winW-64)
	h := min(float32(700), winH-96)
	if w < 520 {
		w = min(winW, 520)
	}
	if h < 360 {
		h = min(winH, 360)
	}
	ui.Modal(c, &a.settingsModalOpen, func() {
		ui.Column(c).Width(w).Height(h).Key("settings-panel").Children(func() {
			components.Settings(c, &a.settingsUI, a.settingsCategories(), components.SettingsOptions{
				Title:             "设置",
				SearchLabel:       "搜索设置",
				SearchPlaceholder: "搜索设置项",
				CategoryLabel:     "设置分类",
				ResultsTitle:      "搜索结果",
				EmptyText:         "没有匹配的设置",
				CloseLabel:        "取消",
				Narrow:            w < 700,
				Close:             a.requestSettingsClose,
				HeaderActions:     a.settingsHeaderActions,
				HideSidebar:       false,
				HideSearch:        false,
			})
		})
	}).Radius(14).Clip()
	// The X button and the modal's own Escape/backdrop handling both close the
	// element before a callback can intervene. If a draft is pending, reopen the
	// editor through the confirmation instead of dropping it silently.
	if !a.settingsModalOpen {
		a.requestSettingsClose()
	}
}

func (a *App) settingsDiscardModal(c *ui.Context) {
	if !a.settingsConfirm {
		return
	}
	wasOpen := a.settingsConfirm
	th := c.Theme()
	ui.Modal(c, &a.settingsConfirm, func() {
		ui.Column(c).Width(450).Gap(11).Children(func() {
			dialogHeader(c, lucide.CircleAlert(), "放弃未保存的修改？", "本次修改不会生效。")
			components.Text(c, "设置尚未保存。继续编辑会保留当前草稿；放弃修改将恢复上次保存的值。").
				FontSize(12).TextColor(th.TextMuted)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if components.Button(c, "继续编辑", components.ButtonSecondary).Clicked() {
					a.settingsConfirm = false
					a.settingsCloseAfter = nil
					a.settingsModalOpen = true
					a.refresh()
				}
				if components.Button(c, "放弃修改", components.ButtonPrimary).Clicked() {
					a.settingsConfirm = false
					a.closeSettingsNow()
				}
			})
		})
	})
	// Dismissing the warning with Escape or a backdrop click keeps editing.
	// Any follow-up that was queued before the warning (such as opening help)
	// must not fire later from the retained draft.
	if wasOpen && !a.settingsConfirm && a.showSettings && a.settingsModalOpen {
		a.settingsCloseAfter = nil
	}
	// Escape or a backdrop click on the confirmation returns to the editor,
	// which is the safe outcome: the draft stays alive.
	if !a.settingsConfirm && a.showSettings && !a.settingsModalOpen {
		a.settingsCloseAfter = nil
		a.settingsModalOpen = true
		a.refresh()
	}
}

func (a *App) settingsHeaderActions(c *ui.Context) {
	th := c.Theme()
	// The heading's text sits optically higher than its buttons. Lift the
	// action group as one unit so the badge and both buttons share the title's
	// baseline without changing the heading height.
	ui.Row(c).AlignSelf(ui.Center).Shrink(0).Gap(th.Space(2)).Margin(-th.Space(1.5), 0, 0, 0).Children(func() {
		if a.settingsDirty() {
			// Badge opts itself into start alignment; wrap it so the action
			// row can center it with the buttons.
			ui.Row(c).AlignSelf(ui.Center).Shrink(0).Children(func() {
				components.Badge(c, "未保存", components.BadgeOptions{Tone: components.BadgeWarning})
			})
		}
		if components.Button(c, "恢复默认", components.ButtonGhost).Clicked() {
			a.resetSettingsDraft()
		}
		if components.Button(c, "保存", components.ButtonPrimary).Clicked() {
			a.later(a.applySettings)
		}
	})
}

func (a *App) settingsCategories() []components.SettingsCategory {
	return []components.SettingsCategory{
		{
			ID: "appearance", Title: "外观", Section: "常规",
			Description: "主题、缩略图与窗口布局",
			Leading:     a.settingsCategoryIcon("appearance", lucide.Palette()),
			Groups: []components.SettingsGroup{
				{
					ID: "theme", Title: "主题与布局",
					Items: []components.SettingsItem{
						{
							ID: "theme", Title: "主题",
							Description: "跟随系统，或固定为浅色 / 深色",
							Keywords:    []string{"theme", "dark", "light", "主题", "深色", "浅色"},
							Leading:     settingsIcon(lucide.Monitor()),
							Build:       a.themeSetting,
						},
						{
							ID: "thumbs", Title: "缩略图宽度",
							Description: "网格视图里的缩略图尺寸",
							Keywords:    []string{"thumbnail", "thumb", "缩略图"},
							Leading:     settingsIcon(lucide.Image()),
							Stacked:     true,
							Build: func(c *ui.Context) {
								a.settingsSlider(c, &a.draftThumbSize, 96, 480, 8, "缩略图宽度", "px")
							},
						},
						{
							ID: "grid-gap", Title: "网格间距",
							Description: "缩略图之间的空隙",
							Keywords:    []string{"grid", "gap", "间距"},
							Leading:     settingsIcon(lucide.Grid3x3()),
							Stacked:     true,
							Build: func(c *ui.Context) {
								a.settingsSlider(c, &a.draftGridGap, 0, 24, 2, "网格间距", "px")
							},
						},
						{
							ID: "card-names", Title: "显示卡片文件名",
							Description: "关闭后卡片只保留画面，瀑布流更沉浸",
							Keywords:    []string{"caption", "filename", "name", "文件名", "卡片"},
							Leading:     settingsIcon(lucide.Type()),
							Build: func(c *ui.Context) {
								components.Switch(c, &a.draft.ShowCardNames, "显示卡片文件名", components.SwitchOptions{HideLabel: true, Compact: true})
							},
						},
						{
							ID: "sidebar", Title: "侧边栏",
							Description: "显示左侧收藏、位置和目录树；滑块调整展开宽度",
							Keywords:    []string{"sidebar", "width", "侧边栏", "宽度"},
							Leading:     settingsIcon(lucide.PanelLeft()),
							Stacked:     true,
							Build: func(c *ui.Context) {
								ui.Row(c).FillWidth().Gap(16).AlignItems(ui.Center).Children(func() {
									ui.Column(c).Grow(1).MinWidth(0).Children(func() {
										a.settingsSlider(c, &a.draftSidebarW, 160, 520, 4, "侧边栏宽度", "px")
									})
									components.Switch(c, &a.draft.Sidebar, "显示侧边栏", components.SwitchOptions{HideLabel: true, Compact: true})
								})
							},
						},
					},
				},
			},
		},
		{
			ID: "browsing", Title: "浏览", Section: "常规",
			Description: "文件过滤、递归扫描和视频播放",
			Leading:     a.settingsCategoryIcon("browsing", lucide.FolderOpen()),
			Groups: []components.SettingsGroup{
				{
					ID: "filter", Title: "文件显示",
					Items: []components.SettingsItem{
						{
							ID: "hidden", Title: "显示隐藏文件",
							Description: "包含以点开头或带隐藏属性的文件",
							Keywords:    []string{"hidden", "隐藏"},
							Leading:     settingsIcon(lucide.EyeOff()),
							Build: func(c *ui.Context) {
								components.Switch(c, &a.draft.ShowHidden, "显示隐藏文件", components.SwitchOptions{HideLabel: true, Compact: true})
							},
						},
						{
							ID: "other", Title: "显示非图片文件",
							Description: "在图片网格里同时列出文档、压缩包等文件",
							Keywords:    []string{"files", "other", "非图片"},
							Leading:     settingsIcon(lucide.FileText()),
							Build: func(c *ui.Context) {
								components.Switch(c, &a.draft.ShowOther, "显示非图片文件", components.SwitchOptions{HideLabel: true, Compact: true})
							},
						},
						{
							ID: "recursive", Title: "递归扫描子文件夹",
							Description: "最多向下扫描 5 层，适合整理照片库",
							Keywords:    []string{"recursive", "folder", "递归", "子文件夹"},
							Leading:     settingsIcon(lucide.FolderTree()),
							Build: func(c *ui.Context) {
								components.Switch(c, &a.draft.Recursive, "递归扫描子文件夹", components.SwitchOptions{HideLabel: true, Compact: true})
							},
						},
					},
				},
				{
					ID: "video", Title: "视频",
					Items: []components.SettingsItem{
						{
							ID: "autoplay", Title: "打开视频时自动播放",
							Description: "关闭后，查看器会停在视频第一帧",
							Keywords:    []string{"video", "play", "视频", "播放"},
							Leading:     settingsIcon(lucide.Play()),
							Build: func(c *ui.Context) {
								components.Switch(c, &a.draft.PlayVideo, "打开视频时自动播放", components.SwitchOptions{HideLabel: true, Compact: true})
							},
						},
						{
							ID: "loop", Title: "循环播放视频",
							Description: "播放到结尾后从头继续",
							Keywords:    []string{"video", "loop", "视频", "循环"},
							Leading:     settingsIcon(lucide.Repeat()),
							Build: func(c *ui.Context) {
								components.Switch(c, &a.draft.LoopVideo, "循环播放视频", components.SwitchOptions{HideLabel: true, Compact: true})
							},
						},
					},
				},
				{
					ID: "sorting", Title: "默认排序",
					Items: []components.SettingsItem{
						{
							ID: "sort", Title: "排序方式",
							Description: "应用到当前标签页，并作为新标签页的默认值",
							Keywords:    []string{"sort", "order", "排序"},
							Leading:     settingsIcon(lucide.ArrowUpDown()),
							Build:       a.sortSetting,
						},
						{
							ID: "desc", Title: "降序排列",
							Description: "名称从 Z 到 A，日期从新到旧",
							Keywords:    []string{"descending", "降序"},
							Leading:     settingsIcon(lucide.ArrowDownWideNarrow()),
							Build: func(c *ui.Context) {
								components.Switch(c, &a.draft.Desc, "降序排列", components.SwitchOptions{HideLabel: true, Compact: true})
							},
						},
					},
				},
			},
		},
		{
			ID: "control", Title: "控制", Section: "常规",
			Description: "鼠标侧键、查看器信息与快捷键参考",
			Leading:     a.settingsCategoryIcon("control", lucide.MousePointerClick()),
			Groups: []components.SettingsGroup{
				{
					ID: "side-buttons", Title: "鼠标侧键",
					Items: []components.SettingsItem{
						{
							ID: "side-map", Title: "查看器侧键",
							Description: "侧键在查看器里切换上一张 / 下一张；单击和长按可分别配置快捷键",
							Keywords:    []string{"mouse", "side", "button", "back", "forward", "侧键", "鼠标", "快捷键", "单击", "长按"},
							Leading:     settingsIcon(lucide.MousePointer2()),
							Stacked:     true,
							Build:       a.sideButtonSetting,
						},
					},
				},
				{
					ID: "viewer-info", Title: "查看器",
					Items: []components.SettingsItem{
						{
							ID: "viewer-auto-immersive", Title: "打开时自动沉浸",
							Description: "直接铺满窗口看图；关闭后以带标题栏的窗口打开，仍可按 F11 切换",
							Keywords:    []string{"immersive", "fullscreen", "auto", "沉浸", "全屏", "自动"},
							Leading:     settingsIcon(lucide.Fullscreen()),
							Build: func(c *ui.Context) {
								components.Switch(c, &a.draft.ViewerAutoImmersive, "打开时自动沉浸", components.SwitchOptions{HideLabel: true, Compact: true})
							},
						},
						{
							ID: "info-panel", Title: "默认显示信息面板",
							Description: "窗口化查看器显示尺寸、路径和元数据；沉浸时隐藏",
							Keywords:    []string{"info", "metadata", "信息", "元数据"},
							Leading:     settingsIcon(lucide.Info()),
							Build: func(c *ui.Context) {
								components.Switch(c, &a.draft.InfoPanel, "默认显示信息面板", components.SwitchOptions{HideLabel: true, Compact: true})
							},
						},
						{
							ID: "viewer-preload", Title: "预加载相邻图片",
							Description: "打开图片时预先解码前后各若干张，切换更快；0 关闭，同时关闭悬停预取",
							Keywords:    []string{"preload", "cache", "预加载", "缓存", "相邻", "悬停"},
							Leading:     settingsIcon(lucide.Images()),
							Stacked:     true,
							Build: func(c *ui.Context) {
								th := c.Theme()
								ui.Row(c).Gap(8).Children(func() {
									components.NumberInput(c.Key("settings-viewer-preload"), &a.draftPreload, 0, 8, 1).
										Label("前后各预加载（张）").Width(180)
									components.Text(c, "张").FontSize(10.5).TextColor(th.TextMuted)
									ui.Spacer(c)
									components.Text(c, "默认 2 张").FontSize(10).TextColor(th.TextMuted)
								})
							},
						},
						{
							ID: "viewer-hud", Title: "查看器状态提示",
							Description: "切图、加载、缩放、边界等瞬时状态；不堆叠，不挡画面中心",
							Keywords:    []string{"hud", "toast", "status", "状态", "提示", "缩放"},
							Leading:     settingsIcon(lucide.Sparkles()),
							Build: func(c *ui.Context) {
								a.viewerHUDSetting(c)
							},
						},
					},
				},
				{
					ID: "shortcut-help", Title: "帮助",
					Items: []components.SettingsItem{
						{
							ID: "shortcuts", Title: "快捷键参考",
							Description: "按场景查看浏览、网格、图片和视频查看器的全部快捷键",
							Keywords:    []string{"shortcut", "help", "keyboard", "快捷键", "帮助"},
							Leading:     settingsIcon(lucide.Keyboard()),
							Build: func(c *ui.Context) {
								if components.Button(c, "打开快捷键参考", components.ButtonSecondary).Clicked() {
									a.requestSettingsCloseAfter(a.openHelp)
								}
							},
						},
					},
				},
			},
		},
		{
			ID: "storage", Title: "缓存", Section: "维护",
			Description: "缩略图磁盘缓存与占用",
			Leading:     a.settingsCategoryIcon("storage", lucide.Database()),
			Groups: []components.SettingsGroup{
				{
					ID: "cache", Title: "缩略图缓存",
					Items: []components.SettingsItem{
						{
							ID: "cache-limit", Title: "缓存上限",
							Description: fmt.Sprintf("当前占用 %s；超出上限后按最近使用清理", (MediaSvc{}).FormatSize(a.cacheInfo.Bytes)),
							Keywords:    []string{"cache", "disk", "缓存", "磁盘"},
							Leading:     settingsIcon(lucide.HardDrive()),
							Stacked:     true,
							Build: func(c *ui.Context) {
								th := c.Theme()
								limit := float64(a.draft.CacheLimitMB) * 1024 * 1024
								ratio := 0.0
								if limit > 0 {
									ratio = float64(a.cacheInfo.Bytes) / limit
								}
								ui.Column(c).Width(340).Gap(6).Children(func() {
									ui.Row(c).Gap(8).Children(func() {
										components.NumberInput(c.Key("settings-cache-limit"), &a.draftCacheLimit, 200, 20000, 100).
											Label("缓存上限（MB）").Width(150)
										components.Text(c, "MB").FontSize(10.5).TextColor(th.TextMuted)
										ui.Spacer(c)
										components.Text(c, fmt.Sprintf("已用 %d%%", int(math.Round(min(ratio, 1)*100)))).
											FontSize(10.5).TextColor(th.TextMuted)
									})
									components.Progress(c, ratio).Label("缓存占用率").Height(3)
									if ratio > 1 {
										components.Text(c, "已超过上限，下次启动或浏览时会清理旧缩略图。").
											FontSize(10).TextColor(th.Danger)
									}
								})
							},
						},
						{
							ID: "cache-clear", Title: "清理缩略图",
							Description: fmt.Sprintf("%d 个缓存文件；清理后会在浏览时重新生成", a.cacheInfo.Files),
							Keywords:    []string{"clear", "cache", "清理", "缓存"},
							Leading:     settingsIcon(lucide.BrushCleaning()),
							Stacked:     true,
							Build: func(c *ui.Context) {
								ui.Row(c).Gap(8).Children(func() {
									if components.Button(c, "清除缓存", components.ButtonSecondary).Clicked() {
										a.clearCache()
									}
									if components.Button(c, "打开缓存文件夹", components.ButtonOutline).Clicked() {
										_ = (AppSvc{}).OpenFolder(a.cacheInfo.Dir)
									}
								})
							},
						},
					},
				},
			},
		},
		{
			ID: "updates", Title: "更新", Section: "维护",
			Description: "版本与签名更新",
			Leading:     a.settingsCategoryIcon("updates", lucide.RefreshCw()),
			Groups: []components.SettingsGroup{
				{
					ID: "version", Title: "FlowVision",
					Items: []components.SettingsItem{
						{
							ID: "version", Title: "当前版本",
							Description: fmt.Sprintf("v%s · 从 GitHub Release 获取签名更新", appVersion()),
							Keywords:    []string{"version", "update", "版本", "更新"},
							Leading:     settingsIcon(lucide.CloudDownload()),
							Build: func(c *ui.Context) {
								if components.Button(c, "检查更新", components.ButtonSecondary).Clicked() {
									a.checkForUpdates()
								}
							},
						},
						{
							ID: "about", Title: "关于 FlowVision",
							Description: "GPL-3.0 · Go / MyGo / Onevra 原生界面",
							Keywords:    []string{"about", "关于", "license"},
							Leading:     settingsIcon(lucide.Info()),
							Build: func(c *ui.Context) {
								if components.Button(c, "查看关于", components.ButtonGhost).Clicked() {
									a.requestSettingsCloseAfter(func() { a.showAbout = true; a.refresh() })
								}
							},
						},
					},
				},
			},
		},
	}
}

func (a *App) viewerHUDSetting(c *ui.Context) {
	sel := components.Select(c.Key("settings-viewer-hud"), &a.draft.ViewerHUD)
	sel.Trigger.Width(180).Label("查看器状态提示")
	sel.Trigger.Children(func() {
		components.Text(c, viewerHUDName(a.draft.ViewerHUD)).SingleLine()
	})
	sel.Popup(func(panel ui.Element) {
		for _, key := range []string{"off", "auto", "always"} {
			key := key
			item := sel.Item(key)
			item.Children(func() {
				components.Text(c, viewerHUDName(key)).SingleLine()
			})
		}
	})
}

func viewerHUDName(key string) string {
	switch key {
	case "off":
		return "关闭"
	case "always":
		return "始终显示"
	default:
		return "自动（推荐）"
	}
}

func (a *App) themeSetting(c *ui.Context) {
	sel := components.Select(c.Key("settings-theme"), &a.draftTheme)
	sel.Trigger.Width(180).Label("主题")
	sel.Trigger.Children(func() {
		components.Text(c, themeName(a.draftTheme)).SingleLine()
	})
	sel.Popup(func(panel ui.Element) {
		for _, key := range []string{"system", "light", "dark"} {
			key := key
			item := sel.Item(key)
			item.Children(func() {
				components.Text(c, themeName(key)).SingleLine()
			})
		}
	})
}

func (a *App) sortSetting(c *ui.Context) {
	sel := components.Select(c.Key("settings-sort"), &a.draftSort)
	sel.Trigger.Width(160).Label("排序方式")
	sel.Trigger.Children(func() {
		components.Text(c, sortLabel(a.draftSort)).SingleLine()
	})
	sel.Popup(func(panel ui.Element) {
		for _, key := range []string{"name", "date", "size", "type"} {
			key := key
			item := sel.Item(key)
			item.Children(func() {
				components.Text(c, sortLabel(key)).SingleLine()
			})
		}
	})
}

func (a *App) settingsSlider(c *ui.Context, value *float64, lo, hi, step float64, label, unit string) {
	th := c.Theme()
	ui.Row(c).Width(340).Gap(8).AlignItems(ui.Center).Children(func() {
		components.Slider(c, value, lo, hi, components.SliderOptions{Step: step}).Label(label).
			Grow(1).MinWidth(0)
		ui.Box(c).Width(58).Height(22).Radius(7).Background(th.Background.Alpha(.55)).
			BorderWidth(1).BorderColor(th.Border.Alpha(.4)).Center().Children(func() {
			components.Text(c, fmt.Sprintf("%d %s", int(math.Round(*value)), unit)).
				FontSize(10.5).FontWeight(500).TextColor(th.TextMuted)
		})
	})
}

// settingsCategoryIcon keeps the settings navigation consistent with the main
// sidebar: active categories carry the accent, while inactive ones stay muted.
func (a *App) settingsCategoryIcon(id string, icon *ui.SVG) func(*ui.Context, float32) {
	return func(c *ui.Context, size float32) {
		th := c.Theme()
		color := th.TextMuted.Alpha(.9)
		if a.settingsUI.Category == id {
			color = th.Accent
		}
		components.Icon(c, icon).Size(size, size).TextColor(color)
	}
}
