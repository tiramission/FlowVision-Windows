package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

const (
	helpFilterAll = iota
	helpFilterBrowse
	helpFilterImage
	helpFilterVideo
)

type helpShortcut struct {
	keys string
	what string
}

type helpSection struct {
	title  string
	icon   *ui.SVG
	filter int
	items  []helpShortcut
}

func (a *App) openHelp() {
	a.helpFilter = helpFilterAll
	a.showHelp = true
	a.refresh()
}

// helpViewportHeight uses the available window height instead of locking the
// shortcut cards to a short fixed viewport, while keeping enough room for the
// dialog header, filters and footer on smaller windows.
func helpViewportHeight(c *ui.Context) float32 {
	_, winH := c.Size()
	h := winH - 260
	if h < 340 {
		h = 340
	}
	if h > 500 {
		h = 500
	}
	return h
}

func (a *App) helpSections() []helpSection {
	return []helpSection{
		{
			title: "浏览与标签", icon: lucide.FolderOpen(), filter: helpFilterBrowse,
			items: []helpShortcut{
				{"Ctrl+O", "打开文件夹"},
				{"Ctrl+T", "新建标签页"},
				{"Ctrl+W", "关闭标签页"},
				{"Ctrl+[", "返回上一个位置"},
				{"Ctrl+Up", "上一级文件夹"},
				{"Ctrl+Left / Right", "上一个 / 下一个文件夹"},
				{"Ctrl+B", "显示 / 隐藏侧边栏"},
				{"Ctrl+R", "递归扫描"},
				{"Ctrl+= / -", "放大 / 缩小缩略图"},
				{"Ctrl+,", "打开设置"},
			},
		},
		{
			title: "网格与文件", icon: lucide.LayoutGrid(), filter: helpFilterBrowse,
			items: []helpShortcut{
				{"双击", "打开文件夹或查看器"},
				{"Enter / Space", "打开选中的文件夹或图片"},
				{"方向键", "在网格中选择"},
				{"Ctrl+F", "搜索当前文件夹"},
				{"F2", "重命名"},
				{"Delete", "移到回收站"},
				{"Ctrl+Delete", "永久删除"},
				{"Ctrl+A", "全选"},
				{"Esc", "取消选择 / 关闭查看器"},
				{"F5", "刷新当前文件夹"},
				{"F1", "打开快捷键参考"},
			},
		},
		{
			title: "图片查看器", icon: lucide.Image(), filter: helpFilterImage,
			items: []helpShortcut{
				{"左 / 右", "上一张 / 下一张"},
				{"鼠标侧键", "上一张 / 下一张（可在设置校准）"},
				{"+ / -", "放大 / 缩小"},
				{"R / Shift+R", "顺时针 / 逆时针旋转"},
				{"0", "适合窗口"},
				{"1", "100% 原始像素"},
				{"C", "图片居中"},
				{"长按左 / 右键", "100% / 适合窗口"},
				{"I", "显示 / 隐藏信息面板"},
				{"双击画面", "进入 / 退出沉浸"},
			},
		},
		{
			title: "视频与窗口", icon: lucide.Film(), filter: helpFilterVideo,
			items: []helpShortcut{
				{"Space", "播放 / 暂停"},
				{"J / L", "后退 / 前进 10 秒"},
				{"Shift+Left / Right", "后退 / 前进 10 秒"},
				{"M", "静音 / 恢复声音"},
				{"双击画面", "进入 / 退出沉浸"},
				{"F11", "查看器沉浸 / 主窗口全屏"},
				{"鼠标到上下边缘", "显示查看器浮层"},
				{"Esc", "关闭查看器"},
			},
		},
	}
}

func (a *App) helpModal(c *ui.Context) {
	if !a.showHelp {
		return
	}
	th := c.Theme()
	winW, _ := c.Size()
	width := min(float32(820), winW-48)
	if width < 560 {
		width = min(winW, 560)
	}
	total := 0
	for _, section := range a.helpSections() {
		total += len(section.items)
	}
	subtitle := fmt.Sprintf("共 %d 项 · 按场景分组 · F1 可随时打开", total)
	if a.helpFilter == helpFilterAll || a.helpFilter == helpFilterBrowse {
		subtitle += " · 可滚动"
	}
	ui.Modal(c, &a.showHelp, func() {
		ui.Column(c).Width(width).Gap(11).Children(func() {
			dialogHeader(c, lucide.Keyboard(), "快捷键与手势", subtitle)
			ui.Row(c).Gap(6).Wrap().Children(func() {
				for _, f := range []struct {
					label  string
					filter int
				}{
					{"全部", helpFilterAll},
					{"浏览", helpFilterBrowse},
					{"图片查看器", helpFilterImage},
					{"视频与窗口", helpFilterVideo},
				} {
					f := f
					style := components.ButtonGhost
					if a.helpFilter == f.filter {
						style = components.ButtonSecondary
					}
					button := components.Button(c, f.label, style)
					if button.Clicked() {
						a.helpFilter = f.filter
						a.refresh()
					}
				}
			})
			ui.Divider(c)
			ui.Box(c).FillWidth().Height(helpViewportHeight(c)).Children(func() {
				ui.Scroll(c).Fill().Children(func() {
					if a.helpFilter == helpFilterAll {
						if width < 700 {
							// At narrow widths the two shortcut columns would leave
							// keycaps too little room, so stack the cards instead.
							ui.Column(c).Gap(12).Children(func() {
								for _, section := range a.helpSections() {
									a.helpSectionCard(c, section)
								}
							})
						} else {
							ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
								ui.Column(c).Grow(1).MinWidth(0).Gap(12).Children(func() {
									a.helpSectionCard(c, a.helpSections()[0])
									a.helpSectionCard(c, a.helpSections()[1])
								})
								ui.Column(c).Grow(1).MinWidth(0).Gap(12).Children(func() {
									a.helpSectionCard(c, a.helpSections()[2])
									a.helpSectionCard(c, a.helpSections()[3])
								})
							})
						}
					} else {
						ui.Column(c).Gap(12).Children(func() {
							for _, section := range a.helpSections() {
								if section.filter == a.helpFilter {
									a.helpSectionCard(c, section)
								}
							}
						})
					}
				})
				if a.helpFilter == helpFilterAll || a.helpFilter == helpFilterBrowse {
					// The all and browse views can overflow the viewport. MyGo only
					// shows scrollbars on hover, so fade the last rows into the
					// dialog background as a quiet cue that there is more below.
					ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Height(18).IgnoreInput(true).
						Gradient(th.Background.Alpha(0), th.Background.Alpha(.9), 180)
				}
			})
			ui.Divider(c)
			ui.Row(c).Gap(9).AlignItems(ui.Center).Children(func() {
				components.Icon(c, lucide.Info()).Size(14, 14).TextColor(th.TextMuted)
				components.Text(c, "侧键方向可在 设置 → 控制 中校准；查看器沉浸在主窗口内，浮层只在鼠标到达上下边缘时出现。").
					FontSize(10.5).TextColor(th.TextMuted).Grow(1)
				if components.Button(c, "关闭", components.ButtonPrimary).Clicked() {
					a.showHelp = false
				}
			})
		})
	})
}

func (a *App) helpSectionCard(c *ui.Context, section helpSection) {
	th := c.Theme()
	ui.Column(c).FillWidth().Padding(13).Gap(9).Radius(12).
		Background(th.Surface).BorderWidth(1).BorderColor(th.Border.Alpha(.7)).Children(func() {
		ui.Row(c).Gap(9).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(25, 25).Radius(7).Background(th.Accent.Alpha(.1)).Center().Children(func() {
				components.Icon(c, section.icon).Size(14, 14).TextColor(th.Accent)
			})
			components.Text(c, section.title).FontSize(12.5).Bold()
			ui.Spacer(c)
			components.Text(c, fmt.Sprintf("%d 项", len(section.items))).FontSize(10).TextColor(th.TextMuted)
		})
		ui.Column(c).Gap(1).Children(func() {
			for _, item := range section.items {
				helpShortcutRow(c, item.keys, item.what)
			}
		})
	})
}

func helpShortcutRow(c *ui.Context, keys, what string) {
	th := c.Theme()
	ui.Row(c).FillWidth().MinHeight(25).Gap(10).Children(func() {
		components.Text(c, what).FontSize(11.5).TextColor(th.Text.Alpha(.84)).Grow(1).Wrap()
		ui.Box(c).Padding(3, 7).Radius(5).Background(th.SurfaceHover.Alpha(.65)).
			BorderWidth(1).BorderColor(th.Border.Alpha(.65)).Children(func() {
			components.Text(c, keys).FontSize(10.5).FontWeight(600).SingleLine()
		})
	})
}
