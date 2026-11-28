package main

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
)

// fillChip is the resident status bar entry for the folder's thumbnail fill.
// Unlike a task row it never leaves: the grid cannot answer "is this folder
// still being cached, and how much is left", and that answer is worth a
// permanent place, while a task that comes and goes mostly flickers. Every
// label starts with 补载 so the chip means the same thing in every state.
type fillChip struct {
	Label      string
	Tone       fillChipTone
	Tip        string
	Actionable bool
}

// fillChipTone is the weight of the readout: quiet for the states that only
// describe the folder, plain for running work, danger for a failure.
type fillChipTone int

const (
	fillToneQuiet fillChipTone = iota
	fillToneBusy
	fillToneDanger
)

// color maps a tone onto the theme, so the state machine above needs no theme
// and can be checked without building a view.
func (t fillChipTone) color(th *ui.Theme) ui.Color {
	switch t {
	case fillToneBusy:
		return th.Text
	case fillToneDanger:
		return th.Danger
	}
	return th.TextMuted
}

// taskChip draws the fill chip. Only a running fill or a failure opens the task
// panel: in a quiet state there is nothing to cancel and nothing to read.
func (a *App) taskChip(c *ui.Context, th *ui.Theme) {
	chip := a.fillChip()
	if !chip.Actionable {
		box := ui.Box(c).Padding(3, 9).Radius(999).AlignItems(ui.Center).
			AlignSelf(ui.Center).Background(th.Text.Alpha(.04)).
			Border(1, th.Border.Alpha(.7))
		box.Children(func() {
			components.Text(c, chip.Label).FontSize(11).TextColor(chip.Tone.color(th))
		})
		box.Tooltip(chip.Tip)
		return
	}
	btn := ui.ButtonBase(c).Label(chip.Label).Focusable().Role(ui.RoleButton).
		Padding(3, 9).Radius(999).AlignItems(ui.Center).AlignSelf(ui.Center).
		Background(th.Text.Alpha(.05)).Border(1, th.Border)
	btn.Children(func() {
		components.Text(c, chip.Label).FontSize(11).TextColor(chip.Tone.color(th))
	})
	btn.Tooltip(chip.Tip)
	if btn.Clicked() {
		a.tasksOpen = !a.tasksOpen
		a.refresh()
	}
}

// fillChip describes the fill right now. A running fill reports progress, and
// every other state says why the folder is not being filled, because "nothing
// is happening" has four different reasons and the user is owed the right one.
func (a *App) fillChip() fillChip {
	// A failure outranks everything: it is the only state that waits to be read.
	for _, t := range a.tasks {
		if t.Kind != taskKindFill || t.State != taskFailed {
			continue
		}
		tip := "整目录缩略图补载失败"
		if t.Err != "" {
			tip += "：" + t.Err
		}
		return fillChip{Label: "补载 失败", Tone: fillToneDanger, Tip: tip, Actionable: true}
	}
	for _, t := range a.tasks {
		if t.Kind != taskKindFill || t.State != taskRunning {
			continue
		}
		if t.Total > 0 {
			return fillChip{
				Label: fmt.Sprintf("补载 %d%%", t.Done*100/t.Total),
				Tone:  fillToneBusy,
				Tip: fmt.Sprintf("正在把本文件夹剩余的缩略图写进缓存：%s / %s 张，点开可查看并取消",
					formatCount(t.Done), formatCount(t.Total)),
				Actionable: true,
			}
		}
		return fillChip{Label: "补载中", Tone: fillToneBusy, Tip: "正在把本文件夹剩余的缩略图写进缓存", Actionable: true}
	}
	tab := a.tab()
	if tab == nil || len(tab.All) == 0 {
		return fillChip{Label: "补载 —", Tone: fillToneQuiet, Tip: "当前文件夹没有可补载的内容"}
	}
	switch {
	case !a.settings.ThumbPrefetch:
		return fillChip{Label: "补载 关闭", Tone: fillToneQuiet, Tip: "整目录补载已关闭（设置 → 缓存与更新）"}
	case isRemotePath(tab.Path):
		return fillChip{Label: "补载 按需", Tone: fillToneQuiet, Tip: "网络文件夹不整目录补载，滚动到哪就生成到哪"}
	case a.settings.ThumbPrefetchMax > 0 && len(tab.All) > a.settings.ThumbPrefetchMax:
		return fillChip{
			Label: "补载 按需", Tone: fillToneQuiet,
			Tip: fmt.Sprintf("这里的 %d 张超过设置的上限 %d 张，不整目录补载，滚动到哪就生成到哪",
				len(tab.All), a.settings.ThumbPrefetchMax),
		}
	case a.fillStoppedTab == tab && a.fillStoppedGen == tab.Gen:
		return fillChip{Label: "补载 已停止", Tone: fillToneQuiet, Tip: "本文件夹的补载已停止；重新打开文件夹可继续"}
	case a.prefetchNext >= len(tab.All) && a.prefetchActive == 0:
		return fillChip{Label: "补载 就绪", Tone: fillToneQuiet, Tip: "本文件夹的缩略图都已写进缓存"}
	}
	return fillChip{Label: "补载 等待", Tone: fillToneQuiet, Tip: "可见卡片加载完成后开始补载本文件夹"}
}

// tasksOverlay draws the task list in the window-level overlay, anchored to the
// status bar so the popover keeps its material and flips above the bar.
func (a *App) tasksOverlay(c *ui.Context) {
	if !a.tasksOpen {
		return
	}
	// The list belongs to the browser page: every modal surface and the viewer
	// own the screen when they are open.
	if a.showSettings || a.showHelp || a.showAbout || a.updateOpen || a.errorMsg != "" ||
		a.renameOpen || a.newFolderOpen || a.confirmDeleteOpen || a.viewer.Open || a.gridMenuOpen {
		a.tasksOpen = false
		return
	}
	now := time.Now()
	if len(a.visibleTasks(now)) == 0 {
		a.tasksOpen = false
		return
	}
	ui.Overlay(c, func() {
		windowW, windowH := c.Size()
		anchor := ui.Box(c).Key("tasks-anchor").
			Absolute().Left(windowW-48).Top(windowH-bottomBarHeight-6).
			Size(1, 1).PassThrough()
		components.Popover(c, anchor, &a.tasksOpen, func(panel ui.Element) {
			panel.Role(ui.RoleDialog).Focusable().MinWidth(292).MaxWidth(360).
				Padding(10).Gap(8)
			a.buildTasksPanel(c, panel, now)
		})
	})
}

func (a *App) buildTasksPanel(c *ui.Context, panel ui.Element, now time.Time) {
	th := c.Theme()
	visible := a.visibleTasks(now)
	components.Text(c, "后台任务").FontSize(11).TextColor(th.TextMuted)
	ui.Box(c).FillWidth().Height(1).Margin(0, 2, 6, 2).
		Background(th.Border).Shrink(0)
	for i, t := range visible {
		if i > 0 {
			ui.Box(c).FillWidth().Height(1).Margin(0, 6, 0, 6).
				Background(th.Border).Shrink(0)
		}
		title := t.Title
		if t.Detail != "" {
			title += " · " + t.Detail
		}
		row := ui.Column(c).FillWidth().Gap(5).Shrink(0)
		row.Children(func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				components.Text(c, title).FontSize(12.5).Grow(1).MinWidth(0).
					SingleLine().Ellipsis("…")
				switch t.State {
				case taskRunning:
					if t.cancel != nil && components.Button(c, "取消", components.ButtonGhost).Clicked() {
						a.stopTask(t.ID)
						a.refresh()
					}
				case taskFailed:
					if components.Button(c, "关闭", components.ButtonGhost).Clicked() {
						a.dropTask(t.ID)
						a.refresh()
					}
				}
			})
			if t.State == taskRunning && t.Total > 0 {
				ratio := float64(t.Done) / float64(t.Total)
				components.Progress(c, ratio).Label(taskStatusLine(t)).Height(3)
			}
			color := th.TextMuted
			if t.State == taskFailed {
				color = th.Danger
			}
			components.Text(c, taskStatusLine(t)).FontSize(10.5).
				TextColor(color).SingleLine().Ellipsis("…")
		})
	}
	if panel.OverlayShortcut(0, ui.KeyEscape) {
		a.tasksOpen = false
		a.refresh()
	}
}

// taskStatusLine is the compact state under a task row; the viewer HUD uses the
// longer taskProgressText sentence instead.
func taskStatusLine(t *backgroundTask) string {
	if t == nil {
		return ""
	}
	switch t.State {
	case taskDone:
		return "已完成"
	case taskStopped:
		return "已停止"
	case taskFailed:
		if t.Err != "" {
			return "失败：" + t.Err
		}
		return "失败"
	}
	if t.Total > 0 {
		return fmt.Sprintf("%s / %s · %d%%", formatCount(t.Done), formatCount(t.Total), t.Done*100/t.Total)
	}
	return "进行中"
}
