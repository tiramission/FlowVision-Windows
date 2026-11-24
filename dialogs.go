package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
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

func dialogHeader(c *ui.Context, icon *ui.SVG, title, subtitle string) {
	th := c.Theme()
	ui.Row(c).Gap(11).Children(func() {
		ui.Box(c).Size(34, 34).Radius(10).Background(th.Accent.Alpha(.13)).Center().Children(func() {
			components.Icon(c, icon).Size(18, 18).TextColor(th.Accent)
		})
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			components.Text(c, title).FontSize(18).Bold().SingleLine()
			if subtitle != "" {
				components.Text(c, subtitle).FontSize(10.5).TextColor(th.Text.Alpha(.55)).SingleLine().Ellipsis("…")
			}
		})
	})
}

func (a *App) settingsModal(c *ui.Context) {
	if !a.showSettings {
		return
	}
	th := c.Theme()
	_, winH := c.Size()
	bodyH := winH - 240
	if bodyH < 220 {
		bodyH = 220
	}
	if bodyH > 540 {
		bodyH = 540
	}
	ui.Modal(c, &a.showSettings, func() {
		ui.Column(c).Width(540).Gap(11).Children(func() {
			dialogHeader(c, lucide.SlidersHorizontal(), "设置", "FlowVision · Onevra 原生界面")
			ui.Divider(c)

			ui.Scroll(c).Height(bodyH).Children(func() {
				ui.Column(c).Gap(9).Children(func() {
					components.Text(c, "外观").FontSize(11).Bold().TextColor(th.Text.Alpha(.72))
					components.Text(c, "主题").FontSize(11).TextColor(th.TextMuted)
					themeLabel := themeName(a.draft.Theme)
					if ui.Select(c, &themeLabel, []string{"跟随系统", "浅色", "深色"}).Label("主题").Changed() {
						a.draft.Theme = themeKey(themeLabel)
					}
					components.Text(c, fmt.Sprintf("缩略图宽度：%d px", a.draft.ThumbSize)).FontSize(11).TextColor(th.TextMuted)
					thumb := float64(a.draft.ThumbSize)
					if ui.StepSlider(c, &thumb, 96, 480, 8).Label("缩略图宽度").Changed() {
						a.draft.ThumbSize = int(thumb)
					}
					components.Text(c, fmt.Sprintf("网格间距：%d px", a.draft.GridGap)).FontSize(11).TextColor(th.TextMuted)
					gap := float64(a.draft.GridGap)
					if ui.StepSlider(c, &gap, 0, 24, 2).Label("网格间距").Changed() {
						a.draft.GridGap = int(gap)
					}
					sidebar := a.draft.Sidebar
					if components.Switch(c, &sidebar, "显示侧边栏", components.SwitchOptions{Compact: true}).Changed() {
						a.draft.Sidebar = sidebar
					}
					info := a.draft.InfoPanel
					if components.Switch(c, &info, "查看器默认显示信息面板", components.SwitchOptions{Compact: true}).Changed() {
						a.draft.InfoPanel = info
					}

					ui.Divider(c)
					components.Text(c, "浏览").FontSize(11).Bold().TextColor(th.Text.Alpha(.72))
					hidden := a.draft.ShowHidden
					if components.Switch(c, &hidden, "显示隐藏文件", components.SwitchOptions{Compact: true}).Changed() {
						a.draft.ShowHidden = hidden
					}
					other := a.draft.ShowOther
					if components.Switch(c, &other, "显示非图片文件", components.SwitchOptions{Compact: true}).Changed() {
						a.draft.ShowOther = other
					}
					recursive := a.draft.Recursive
					if components.Switch(c, &recursive, "递归扫描子文件夹（最多 5 层）", components.SwitchOptions{Compact: true}).Changed() {
						a.draft.Recursive = recursive
					}
					loop := a.draft.LoopVideo
					if components.Switch(c, &loop, "循环播放视频", components.SwitchOptions{Compact: true}).Changed() {
						a.draft.LoopVideo = loop
					}

					ui.Divider(c)
					components.Text(c, "缩略图缓存").FontSize(11).Bold().TextColor(th.Text.Alpha(.72))
					components.Text(c, fmt.Sprintf("%d 个文件 · %s", a.cacheInfo.Files, (MediaSvc{}).FormatSize(a.cacheInfo.Bytes))).
						FontSize(11).TextColor(th.TextMuted)
					ui.Row(c).Gap(8).Children(func() {
						if components.Button(c, "清除缓存", components.ButtonSecondary).Clicked() {
							a.clearCache()
						}
						if components.Button(c, "打开缓存文件夹", components.ButtonOutline).Clicked() {
							_ = (AppSvc{}).OpenFolder(a.cacheInfo.Dir)
						}
					})
				})
			})

			ui.Divider(c)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if components.Button(c, "取消", components.ButtonSecondary).Clicked() {
					a.showSettings = false
				}
				if components.Button(c, "保存", components.ButtonPrimary).Clicked() {
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
		ui.Column(c).Width(430).Gap(11).Children(func() {
			dialogHeader(c, lucide.Pencil(), "重命名", "只修改名称，扩展名保持不变")
			if a.renameTarget != nil {
				components.Text(c, a.renameTarget.Name).FontSize(11).TextColor(th.TextMuted).SingleLine()
			}
			field := components.TextBox(c, &a.renameDraft, components.TextBoxOptions{}).AutoFocus()
			field.Label("新名称")
			if field.Submitted() {
				a.later(a.commitRename)
			}
			if a.renameErr != "" {
				components.Text(c, a.renameErr).FontSize(11).TextColor(th.Danger)
			}
			components.Text(c, "扩展名不能修改。").FontSize(11).TextColor(th.TextMuted)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if components.Button(c, "取消", components.ButtonSecondary).Clicked() {
					a.renameOpen = false
				}
				if components.Button(c, "重命名", components.ButtonPrimary).Clicked() {
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
		ui.Column(c).Width(410).Gap(11).Children(func() {
			dialogHeader(c, lucide.FolderPlus(), "新建文件夹", "在当前文件夹中创建")
			field := components.TextBox(c, &a.newFolderDraft, components.TextBoxOptions{}).AutoFocus()
			field.Label("文件夹名称")
			if field.Submitted() {
				a.later(a.commitNewFolder)
			}
			if a.newFolderErr != "" {
				components.Text(c, a.newFolderErr).FontSize(11).TextColor(th.Danger)
			}
			components.Text(c, "将在当前文件夹中创建。").FontSize(11).TextColor(th.TextMuted)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if components.Button(c, "取消", components.ButtonSecondary).Clicked() {
					a.newFolderOpen = false
				}
				if components.Button(c, "创建", components.ButtonPrimary).Clicked() {
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
		ui.Column(c).Width(450).Gap(11).Children(func() {
			title := "移到回收站？"
			detail := "可从 Windows 回收站恢复。"
			icon := lucide.Trash()
			if a.confirmPermanent {
				title = "永久删除？"
				detail = "此操作不可撤销。"
			}
			dialogHeader(c, icon, title, detail)
			components.Text(c, fmt.Sprintf("将处理 %d 项。", n)).FontSize(12).TextColor(th.TextMuted)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if components.Button(c, "取消", components.ButtonSecondary).Clicked() {
					a.confirmDeleteOpen = false
				}
				label := "移到回收站"
				if a.confirmPermanent {
					label = "永久删除"
				}
				del := components.Button(c, label, components.ButtonPrimary)
				if a.confirmPermanent {
					del.Background(th.Danger).TextColor(th.AccentText)
				}
				if del.Clicked() {
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
	th := c.Theme()
	open := true
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(470).Gap(11).Children(func() {
			dialogHeader(c, lucide.CircleAlert(), "操作未完成", "")
			components.Text(c, a.errorMsg).FontSize(12).TextColor(th.TextMuted)
			ui.Row(c).Justify(ui.End).Children(func() {
				if components.Button(c, "好", components.ButtonPrimary).Clicked() {
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
		ui.Column(c).Width(580).Gap(11).Children(func() {
			dialogHeader(c, lucide.Keyboard(), "快捷键与手势", "原生控件，所有操作都在窗口内完成")
			ui.Scroll(c).Height(340).Children(func() {
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
					components.Text(c, "网格支持拖入文件或文件夹；拖动查看器中的图片可平移。").FontSize(10.5).TextColor(th.TextMuted)
				})
			})
			ui.Row(c).Justify(ui.End).Children(func() {
				if components.Button(c, "关闭", components.ButtonPrimary).Clicked() {
					a.showHelp = false
				}
			})
		})
	})
}

func shortcutRow(c *ui.Context, key, what string) {
	th := c.Theme()
	ui.Row(c).Gap(12).Children(func() {
		ui.Box(c).Width(158).Padding(4, 8).Radius(6).Background(th.SurfaceHover).Children(func() {
			components.Text(c, key).FontSize(11.5).FontWeight(600).SingleLine()
		})
		components.Text(c, what).FontSize(11.5).TextColor(th.TextMuted).Grow(1)
	})
}

func (a *App) aboutModal(c *ui.Context) {
	if !a.showAbout {
		return
	}
	th := c.Theme()
	ui.Modal(c, &a.showAbout, func() {
		ui.Column(c).Width(450).Gap(11).Children(func() {
			ui.Row(c).Gap(12).Children(func() {
				ui.Box(c).Size(42, 42).Radius(12).Background(th.Accent.Alpha(.13)).Center().Children(func() {
					components.Icon(c, lucide.Aperture()).Size(24, 24).TextColor(th.Accent)
				})
				ui.Column(c).Grow(1).Children(func() {
					components.Text(c, "FlowVision").FontSize(21).Bold()
					components.Text(c, "Windows 版 · 纯 Go / MyGo / Onevra").FontSize(10.5).TextColor(th.TextMuted)
				})
				components.Badge(c, "v"+appVersion, components.BadgeOptions{Tone: components.BadgeAccent})
			})
			ui.Divider(c)
			components.Text(c, "原项目：netdcy/FlowVision（macOS，Swift/AppKit）").FontSize(10.5).TextColor(th.TextMuted)
			components.Text(c, "Windows 重写：Go 后端与 MyGo 原生 ui 包，Onevra 提供主题与控件；不使用 WebView2、HTML 或 JavaScript。").
				FontSize(10.5).TextColor(th.TextMuted)
			components.Text(c, "许可证：GPL-3.0").FontSize(10.5)
			ui.Row(c).Justify(ui.End).Children(func() {
				if components.Button(c, "关闭", components.ButtonPrimary).Clicked() {
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
