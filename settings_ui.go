package main

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
)

// settingsModal presents the full settings catalog in Onevra's searchable
// Settings surface. All controls bind to retained draft fields; this keeps an
// edit alive across the rebuild that delivers its Changed notice.
func (a *App) settingsModal(c *ui.Context) {
	if !a.showSettings {
		return
	}
	winW, winH := c.Size()
	w := min(float32(860), winW-64)
	h := min(float32(680), winH-96)
	if w < 520 {
		w = min(winW, 520)
	}
	if h < 360 {
		h = min(winH, 360)
	}
	ui.Modal(c, &a.showSettings, func() {
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
				Close:             func() { a.showSettings = false; a.refresh() },
				HeaderActions:     a.settingsHeaderActions,
				HideSidebar:       false,
				HideSearch:        false,
			})
		})
	}).Radius(14).Clip()
}

func (a *App) settingsHeaderActions(c *ui.Context) {
	if components.Button(c, "恢复默认", components.ButtonGhost).Clicked() {
		a.resetSettingsDraft()
	}
	if components.Button(c, "保存", components.ButtonPrimary).Clicked() {
		a.later(a.applySettings)
	}
}

func (a *App) settingsCategories() []components.SettingsCategory {
	return []components.SettingsCategory{
		{
			ID: "appearance", Title: "外观", Section: "常规",
			Description: "主题、缩略图与窗口布局",
			Groups: []components.SettingsGroup{
				{
					ID: "theme", Title: "主题与布局",
					Items: []components.SettingsItem{
						{
							ID: "theme", Title: "主题",
							Description: "跟随系统，或固定为浅色 / 深色",
							Keywords:    []string{"theme", "dark", "light", "主题", "深色", "浅色"},
							Build:       a.themeSetting,
						},
						{
							ID: "thumbs", Title: "缩略图宽度",
							Description: "网格视图里的缩略图尺寸",
							Keywords:    []string{"thumbnail", "thumb", "缩略图"},
							Build: func(c *ui.Context) {
								a.settingsSlider(c, &a.draftThumbSize, 96, 480, 8, "缩略图宽度", "px")
							},
						},
						{
							ID: "grid-gap", Title: "网格间距",
							Description: "缩略图之间的空隙",
							Keywords:    []string{"grid", "gap", "间距"},
							Build: func(c *ui.Context) {
								a.settingsSlider(c, &a.draftGridGap, 0, 24, 2, "网格间距", "px")
							},
						},
						{
							ID: "card-names", Title: "显示卡片文件名",
							Description: "关闭后卡片只保留画面，瀑布流更沉浸",
							Keywords:    []string{"caption", "filename", "name", "文件名", "卡片"},
							Build: func(c *ui.Context) {
								components.Switch(c, &a.draft.ShowCardNames, "显示卡片文件名", components.SwitchOptions{HideLabel: true, Compact: true})
							},
						},
						{
							ID: "sidebar-width", Title: "侧边栏宽度",
							Description: "侧边栏展开时占用的宽度",
							Keywords:    []string{"sidebar", "width", "侧边栏", "宽度"},
							Build: func(c *ui.Context) {
								a.settingsSlider(c, &a.draftSidebarW, 160, 520, 4, "侧边栏宽度", "px")
							},
						},
						{
							ID: "sidebar", Title: "显示侧边栏",
							Description: "左侧收藏、位置和目录树",
							Keywords:    []string{"sidebar", "侧边栏"},
							Build: func(c *ui.Context) {
								components.Switch(c, &a.draft.Sidebar, "显示侧边栏", components.SwitchOptions{HideLabel: true, Compact: true})
							},
						},
						{
							ID: "info-panel", Title: "查看器默认显示信息面板",
							Description: "窗口化查看器显示尺寸、路径和元数据；全屏沉浸时隐藏",
							Keywords:    []string{"info", "metadata", "信息", "元数据"},
							Build: func(c *ui.Context) {
								components.Switch(c, &a.draft.InfoPanel, "查看器默认显示信息面板", components.SwitchOptions{HideLabel: true, Compact: true})
							},
						},
					},
				},
			},
		},
		{
			ID: "browsing", Title: "浏览", Section: "常规",
			Description: "文件过滤、递归扫描和视频播放",
			Groups: []components.SettingsGroup{
				{
					ID: "filter", Title: "文件显示",
					Items: []components.SettingsItem{
						{
							ID: "hidden", Title: "显示隐藏文件",
							Description: "包含以点开头或带隐藏属性的文件",
							Keywords:    []string{"hidden", "隐藏"},
							Build: func(c *ui.Context) {
								components.Switch(c, &a.draft.ShowHidden, "显示隐藏文件", components.SwitchOptions{HideLabel: true, Compact: true})
							},
						},
						{
							ID: "other", Title: "显示非图片文件",
							Description: "在图片网格里同时列出文档、压缩包等文件",
							Keywords:    []string{"files", "other", "非图片"},
							Build: func(c *ui.Context) {
								components.Switch(c, &a.draft.ShowOther, "显示非图片文件", components.SwitchOptions{HideLabel: true, Compact: true})
							},
						},
						{
							ID: "recursive", Title: "递归扫描子文件夹",
							Description: "最多向下扫描 5 层，适合整理照片库",
							Keywords:    []string{"recursive", "folder", "递归", "子文件夹"},
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
							Build: func(c *ui.Context) {
								components.Switch(c, &a.draft.PlayVideo, "打开视频时自动播放", components.SwitchOptions{HideLabel: true, Compact: true})
							},
						},
						{
							ID: "loop", Title: "循环播放视频",
							Description: "播放到结尾后从头继续",
							Keywords:    []string{"video", "loop", "视频", "循环"},
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
							Build:       a.sortSetting,
						},
						{
							ID: "desc", Title: "降序排列",
							Description: "名称从 Z 到 A，日期从新到旧",
							Keywords:    []string{"descending", "降序"},
							Build: func(c *ui.Context) {
								components.Switch(c, &a.draft.Desc, "降序排列", components.SwitchOptions{HideLabel: true, Compact: true})
							},
						},
					},
				},
			},
		},
		{
			ID: "storage", Title: "缓存", Section: "维护",
			Description: "缩略图磁盘缓存与占用",
			Groups: []components.SettingsGroup{
				{
					ID: "cache", Title: "缩略图缓存",
					Items: []components.SettingsItem{
						{
							ID: "cache-limit", Title: "缓存上限",
							Description: fmt.Sprintf("当前占用 %s；超出上限后按最近使用清理", (MediaSvc{}).FormatSize(a.cacheInfo.Bytes)),
							Keywords:    []string{"cache", "disk", "缓存", "磁盘"},
							Build: func(c *ui.Context) {
								components.NumberInput(c.Key("settings-cache-limit"), &a.draftCacheLimit, 200, 20000, 100).
									Label("缓存上限（MB）").Width(150)
								components.Text(c, "MB").FontSize(10.5).TextColor(c.Theme().TextMuted)
							},
						},
						{
							ID: "cache-clear", Title: "清理缩略图",
							Description: fmt.Sprintf("%d 个缓存文件；清理后会在浏览时重新生成", a.cacheInfo.Files),
							Keywords:    []string{"clear", "cache", "清理", "缓存"},
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
			Groups: []components.SettingsGroup{
				{
					ID: "version", Title: "FlowVision",
					Items: []components.SettingsItem{
						{
							ID: "version", Title: "当前版本",
							Description: fmt.Sprintf("v%s · 从 GitHub Release 获取签名更新", appVersion()),
							Keywords:    []string{"version", "update", "版本", "更新"},
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
							Build: func(c *ui.Context) {
								if components.Button(c, "查看关于", components.ButtonGhost).Clicked() {
									a.showSettings = false
									a.later(func() { a.showAbout = true; a.refresh() })
								}
							},
						},
					},
				},
			},
		},
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
	ui.Column(c).Width(260).Gap(4).Children(func() {
		components.Slider(c, value, lo, hi, components.SliderOptions{Step: step}).Label(label).FillWidth()
		components.Text(c, fmt.Sprintf("%d %s", int(math.Round(*value)), unit)).
			FontSize(10.5).TextColor(c.Theme().TextMuted)
	})
}
