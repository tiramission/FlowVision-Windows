package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *App) overlays(c *ui.Context) {
	a.settingsModal(c)
	a.renameModal(c)
	a.newFolderModal(c)
	a.deleteModal(c)
	a.errorModal(c)
	a.helpModal(c)
	a.aboutModal(c)
	a.viewerOverlay(c)
}

func (a *App) settingsModal(c *ui.Context) {
	if !a.showSettings {
		return
	}
	th := c.Theme()
	_, winH := c.Size()
	bodyH := winH - 230
	if bodyH < 200 {
		bodyH = 200
	}
	if bodyH > 520 {
		bodyH = 520
	}
	ui.Modal(c, &a.showSettings, func() {
		ui.Column(c).Width(520).Gap(10).Children(func() {
			ui.Text(c, "设置").FontSize(19).Bold()
			ui.Text(c, "FlowVision 原生 Go 界面").FontSize(11).TextColor(th.TextMuted)

			ui.Divider(c)
			ui.Scroll(c).Height(bodyH).Children(func() {
				ui.Column(c).Gap(8).Children(func() {
					ui.Text(c, "外观").FontSize(12).Bold()
					ui.Text(c, "主题").FontSize(11).TextColor(th.TextMuted)
					themeLabel := themeName(a.draft.Theme)
					if ui.Select(c, &themeLabel, []string{"跟随系统", "浅色", "深色"}).Label("主题").Changed() {
						a.draft.Theme = themeKey(themeLabel)
					}
					ui.Text(c, fmt.Sprintf("缩略图宽度：%d px", a.draft.ThumbSize)).FontSize(11).TextColor(th.TextMuted)
					thumb := float64(a.draft.ThumbSize)
					if ui.StepSlider(c, &thumb, 96, 480, 8).Label("缩略图宽度").Changed() {
						a.draft.ThumbSize = int(thumb)
					}
					ui.Text(c, fmt.Sprintf("网格间距：%d px", a.draft.GridGap)).FontSize(11).TextColor(th.TextMuted)
					gap := float64(a.draft.GridGap)
					if ui.StepSlider(c, &gap, 0, 24, 2).Label("网格间距").Changed() {
						a.draft.GridGap = int(gap)
					}
					sidebar := a.draft.Sidebar
					if ui.Checkbox(c, &sidebar, "显示侧边栏").Changed() {
						a.draft.Sidebar = sidebar
					}
					info := a.draft.InfoPanel
					if ui.Checkbox(c, &info, "查看器默认显示信息面板").Changed() {
						a.draft.InfoPanel = info
					}

					ui.Divider(c)
					ui.Text(c, "浏览").FontSize(12).Bold()
					hidden := a.draft.ShowHidden
					if ui.Checkbox(c, &hidden, "显示隐藏文件").Changed() {
						a.draft.ShowHidden = hidden
					}
					other := a.draft.ShowOther
					if ui.Checkbox(c, &other, "显示非图片文件").Changed() {
						a.draft.ShowOther = other
					}
					recursive := a.draft.Recursive
					if ui.Checkbox(c, &recursive, "递归扫描子文件夹（最多 5 层）").Changed() {
						a.draft.Recursive = recursive
					}
					loop := a.draft.LoopVideo
					if ui.Checkbox(c, &loop, "循环播放视频").Changed() {
						a.draft.LoopVideo = loop
					}

					ui.Divider(c)
					ui.Text(c, "缩略图缓存").FontSize(12).Bold()
					ui.Text(c, fmt.Sprintf("%d 个文件 · %s", a.cacheInfo.Files, (MediaSvc{}).FormatSize(a.cacheInfo.Bytes))).
						FontSize(11).TextColor(th.TextMuted)
					ui.Row(c).Gap(8).Children(func() {
						if ui.Button(c, "清除缓存").Clicked() {
							a.clearCache()
						}
						if ui.Button(c, "打开缓存文件夹").Clicked() {
							_ = (AppSvc{}).OpenFolder(a.cacheInfo.Dir)
						}
					})
				})
			})

			ui.Divider(c)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.showSettings = false
				}
				if ui.PrimaryButton(c, "保存").Clicked() {
					a.later(a.applySettings)
				}
			})
		})
	})
}

func themeName(v string) string {
	switch v {
	case "light":
		return "浅色"
	case "dark":
		return "深色"
	}
	return "跟随系统"
}

func themeKey(v string) string {
	switch v {
	case "浅色":
		return "light"
	case "深色":
		return "dark"
	}
	return "system"
}

func (a *App) renameModal(c *ui.Context) {
	if !a.renameOpen {
		return
	}
	th := c.Theme()
	ui.Modal(c, &a.renameOpen, func() {
		ui.Column(c).Width(420).Gap(10).Children(func() {
			ui.Text(c, "重命名").FontSize(18).Bold()
			if a.renameTarget != nil {
				ui.Text(c, a.renameTarget.Name).FontSize(11).TextColor(th.TextMuted).SingleLine()
			}
			field := ui.TextInput(c, &a.renameDraft).AutoFocus()
			field.Label("新名称")
			if field.Submitted() {
				a.later(a.commitRename)
			}
			if a.renameErr != "" {
				ui.Text(c, a.renameErr).FontSize(11).TextColor(ui.RGBA(220, 70, 70, 255))
			}
			ui.Text(c, "扩展名不能修改。").FontSize(11).TextColor(th.TextMuted)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.renameOpen = false
				}
				if ui.PrimaryButton(c, "重命名").Clicked() {
					a.later(a.commitRename)
				}
			})
		})
	})
}

func (a *App) newFolderModal(c *ui.Context) {
	if !a.newFolderOpen {
		return
	}
	th := c.Theme()
	ui.Modal(c, &a.newFolderOpen, func() {
		ui.Column(c).Width(400).Gap(10).Children(func() {
			ui.Text(c, "新建文件夹").FontSize(18).Bold()
			field := ui.TextInput(c, &a.newFolderDraft).AutoFocus()
			field.Label("文件夹名称")
			if field.Submitted() {
				a.later(a.commitNewFolder)
			}
			if a.newFolderErr != "" {
				ui.Text(c, a.newFolderErr).FontSize(11).TextColor(ui.RGBA(220, 70, 70, 255))
			}
			ui.Text(c, "将在当前文件夹中创建。").FontSize(11).TextColor(th.TextMuted)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.newFolderOpen = false
				}
				if ui.PrimaryButton(c, "创建").Clicked() {
					a.later(a.commitNewFolder)
				}
			})
		})
	})
}

func (a *App) deleteModal(c *ui.Context) {
	if !a.confirmDeleteOpen {
		return
	}
	th := c.Theme()
	n := 0
	if t := a.tab(); t != nil {
		n = len(t.Selected)
	}
	ui.Modal(c, &a.confirmDeleteOpen, func() {
		ui.Column(c).Width(440).Gap(10).Children(func() {
			title := "移到回收站？"
			detail := "可从 Windows 回收站恢复。"
			if a.confirmPermanent {
				title = "永久删除？"
				detail = "此操作不可撤销。"
			}
			ui.Text(c, title).FontSize(18).Bold()
			ui.Text(c, fmt.Sprintf("将处理 %d 项。%s", n, detail)).FontSize(12).TextColor(th.TextMuted)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.confirmDeleteOpen = false
				}
				label := "移到回收站"
				if a.confirmPermanent {
					label = "永久删除"
				}
				if ui.Button(c, label).Clicked() {
					a.later(a.doDelete)
				}
			})
		})
	})
}

func (a *App) errorModal(c *ui.Context) {
	if a.errorMsg == "" {
		return
	}
	open := true
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(460).Gap(10).Children(func() {
			ui.Text(c, "操作未完成").FontSize(18).Bold()
			ui.Text(c, a.errorMsg).FontSize(12).TextColor(c.Theme().TextMuted)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.PrimaryButton(c, "好").Clicked() {
					a.errorMsg = ""
				}
			})
		})
	})
	if !open {
		a.errorMsg = ""
	}
}

func (a *App) helpModal(c *ui.Context) {
	if !a.showHelp {
		return
	}
	th := c.Theme()
	ui.Modal(c, &a.showHelp, func() {
		ui.Column(c).Width(560).Gap(10).Children(func() {
			ui.Text(c, "快捷键与手势").FontSize(18).Bold()
			ui.Scroll(c).Height(330).Children(func() {
				ui.Column(c).Gap(5).Children(func() {
					shortcutRow(c, "Ctrl+O", "打开文件夹")
					shortcutRow(c, "Ctrl+T", "新建标签页")
					shortcutRow(c, "Ctrl+W", "关闭标签页")
					shortcutRow(c, "Ctrl+[", "返回上一个位置")
					shortcutRow(c, "Ctrl+↑", "上一级文件夹")
					shortcutRow(c, "Ctrl+← / →", "上一个 / 下一个文件夹")
					shortcutRow(c, "Ctrl+B", "显示 / 隐藏侧边栏")
					shortcutRow(c, "Ctrl+R", "递归扫描")
					shortcutRow(c, "Ctrl+= / -", "放大 / 缩小缩略图")
					shortcutRow(c, "Ctrl+,", "设置")
					shortcutRow(c, "双击图片", "打开查看器")
					shortcutRow(c, "Enter / Space", "打开选中的图片")
					shortcutRow(c, "← ↑ → ↓", "在网格中选择")
					shortcutRow(c, "F2", "重命名")
					shortcutRow(c, "Delete", "移到回收站")
					shortcutRow(c, "Ctrl+A", "全选")
					shortcutRow(c, "Esc", "取消选择 / 关闭查看器")
					shortcutRow(c, "查看器：+ / -", "缩放")
					shortcutRow(c, "查看器：R / Shift+R", "顺时针 / 逆时针旋转")
					shortcutRow(c, "查看器：0", "适合窗口")
					shortcutRow(c, "查看器：← / →", "上一张 / 下一张")
					shortcutRow(c, "查看器：I", "显示 / 隐藏信息")
					ui.Text(c, "网格支持拖入文件或文件夹；拖动查看器中的图片可平移。").FontSize(11).TextColor(th.TextMuted)
				})
			})
			ui.Row(c).Justify(ui.End).Children(func() {
				if ui.PrimaryButton(c, "关闭").Clicked() {
					a.showHelp = false
				}
			})
		})
	})
}

func shortcutRow(c *ui.Context, key, what string) {
	ui.Row(c).Gap(12).Children(func() {
		ui.Text(c, key).FontSize(12).FontWeight(600).Width(150)
		ui.Text(c, what).FontSize(12).TextColor(c.Theme().TextMuted).Grow(1)
	})
}

func (a *App) aboutModal(c *ui.Context) {
	if !a.showAbout {
		return
	}
	th := c.Theme()
	ui.Modal(c, &a.showAbout, func() {
		ui.Column(c).Width(440).Gap(10).Children(func() {
			ui.Text(c, "FlowVision").FontSize(22).Bold()
			ui.Text(c, "Windows 版 · 纯 Go / MyGo 原生 UI").FontSize(12).TextColor(th.TextMuted)
			ui.Text(c, "版本 "+appVersion).FontSize(12)
			ui.Divider(c)
			ui.Text(c, "原项目：netdcy/FlowVision（macOS，Swift/AppKit）").FontSize(11).TextColor(th.TextMuted)
			ui.Text(c, "Windows 重写：Go 后端与 MyGo 原生 ui 包，不使用 WebView2、HTML 或 JavaScript。").FontSize(11).TextColor(th.TextMuted)
			ui.Text(c, "许可证：GPL-3.0").FontSize(11)
			ui.Row(c).Justify(ui.End).Children(func() {
				if ui.PrimaryButton(c, "关闭").Clicked() {
					a.showAbout = false
				}
			})
		})
	})
}

func (a *App) selectionSummary() string {
	t := a.tab()
	if t == nil {
		return ""
	}
	names := []string{}
	for _, it := range t.selectedItems() {
		names = append(names, it.Name)
		if len(names) == 3 {
			break
		}
	}
	return strings.Join(names, "、")
}
