package main

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
)

// taskChip is the status bar entry that appears while background work is
// listed. It counts running work; a failure keeps it visible until it is read.
func (a *App) taskChip(c *ui.Context, th *ui.Theme) {
	visible := a.visibleTasks(time.Now())
	if len(visible) == 0 {
		a.tasksOpen = false
		return
	}
	running, failed := 0, 0
	for _, t := range visible {
		switch t.State {
		case taskRunning:
			running++
		case taskFailed:
			failed++
		}
	}
	label := "后台"
	color := th.Text
	switch {
	case running > 0:
		label = fmt.Sprintf("后台 %d", running)
	case failed > 0:
		label = "后台 · 失败"
		color = th.Danger
	}
	chip := ui.ButtonBase(c).Label(label).Focusable().Role(ui.RoleButton).
		Padding(3, 9).Radius(999).AlignItems(ui.Center).AlignSelf(ui.Center).
		Background(th.Text.Alpha(.05)).Border(1, th.Border)
	chip.Children(func() {
		components.Text(c, label).FontSize(11).TextColor(color)
	})
	if chip.Clicked() {
		a.tasksOpen = !a.tasksOpen
		a.refresh()
	}
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
