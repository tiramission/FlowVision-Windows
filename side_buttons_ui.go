package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// beginBindingCapture puts one shortcut field into listening mode. Unlike the
// old side-button calibration dialog, this records the next key press directly
// in the settings row, including mouse side buttons and modifier chords.
func (a *App) beginBindingCapture(target string) {
	a.bindingCapture = target
	a.bindingCaptureGen++
	a.bindingCaptureNote = ""
	a.refresh()
}

func (a *App) cancelBindingCapture() {
	a.bindingCapture = ""
	a.bindingCaptureNote = ""
	a.refresh()
}

func (a *App) applyBindingCapture(target string, b SideBinding) {
	if !a.showSettings {
		a.cancelBindingCapture()
		return
	}
	if b.Key == ui.KeyUnknown || !viewerSideBindingAllowed(b) {
		a.bindingCaptureNote = "该按键已被查看器占用，请换一个键或先清除对应快捷键。"
		a.refresh()
		return
	}
	assignViewerBinding(&a.draft.SidePrev, &a.draft.SideNext, &a.draft.SidePrevHold, &a.draft.SideNextHold, target, b)
	a.bindingCapture = ""
	a.bindingCaptureNote = ""
	a.refresh()
}

func (a *App) clearBinding(target string) {
	switch target {
	case "prevHold":
		a.draft.SidePrevHold = SideBinding{}
	case "nextHold":
		a.draft.SideNextHold = SideBinding{}
	}
	a.bindingCapture = ""
	a.bindingCaptureNote = ""
	a.refresh()
}

func (a *App) sideButtonSetting(c *ui.Context) {
	th := c.Theme()
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		a.sideDirectionRow(c, "上一张", lucide.ArrowLeft(), "prev", th)
		a.sideDirectionRow(c, "下一张", lucide.ArrowRight(), "next", th)
		if a.bindingCaptureNote != "" {
			components.Text(c, a.bindingCaptureNote).FontSize(10).TextColor(th.Danger)
		}
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if components.Button(c, "交换方向", components.ButtonOutline).Clicked() {
				a.swapSideBindings()
			}
			components.Text(c, "侧键只在查看器内生效；点击方框后按下按键即可录制，单击和长按可分别指向上一张或下一张。").
				FontSize(10.5).TextColor(th.TextMuted).Grow(1)
		})
	})
}

func (a *App) sideDirectionRow(c *ui.Context, title string, icon *ui.SVG, prefix string, th *ui.Theme) {
	ui.Column(c).FillWidth().Padding(8, 10).Radius(10).Gap(7).
		Background(th.Background.Alpha(.55)).BorderWidth(1).BorderColor(th.Border.Alpha(.55)).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			ui.Box(c).Size(26, 26).Radius(7).Background(th.Accent.Alpha(.1)).Center().Children(func() {
				components.Icon(c, icon).Size(14, 14).TextColor(th.Accent)
			})
			components.Text(c, title).FontSize(11.5).FontWeight(600).SingleLine()
			ui.Spacer(c)
			components.Text(c, "查看器").FontSize(10).TextColor(th.TextMuted)
		})
		ui.Row(c).Gap(8).Wrap().Children(func() {
			components.Text(c, "单击").FontSize(10).TextColor(th.TextMuted).Width(30)
			a.sideBindingField(c, title, "单击", prefix+"Click", a.draftBinding(prefix+"Click"), th)
			components.Text(c, "长按").FontSize(10).TextColor(th.TextMuted).Width(30)
			a.sideBindingField(c, title, "长按", prefix+"Hold", a.draftBinding(prefix+"Hold"), th)
			if hold := a.draftBinding(prefix + "Hold"); hold.Key != ui.KeyUnknown {
				if components.IconButton(c, lucide.X(), "清除"+title+"长按", components.IconButtonPlain).Clicked() {
					a.clearBinding(prefix + "Hold")
				}
			}
		})
	})
}

func (a *App) draftBinding(target string) SideBinding {
	switch target {
	case "prevClick":
		return a.draft.SidePrev
	case "nextClick":
		return a.draft.SideNext
	case "prevHold":
		return a.draft.SidePrevHold
	case "nextHold":
		return a.draft.SideNextHold
	}
	return SideBinding{}
}

func (a *App) sideBindingField(c *ui.Context, title, role, target string, binding SideBinding, th *ui.Theme) {
	capturing := a.bindingCapture == target
	box := ui.Box(c).Key("side-binding-"+target).Width(176).Height(30).Padding(4, 9).Radius(7).
		Background(th.SurfaceHover.Alpha(.7)).BorderWidth(1).BorderColor(th.Border.Alpha(.7)).
		Focusable().TabStop(false)
	if capturing {
		box.BorderColor(th.Accent.Alpha(.7)).Background(th.Accent.Alpha(.08)).AutoFocus()
	}
	if box.Clicked() && !capturing {
		a.beginBindingCapture(target)
	}
	box.Label(title + role + "快捷键")
	box.Children(func() {
		label := sideBindingLabel(binding)
		if capturing {
			label = "按下快捷键…"
		}
		components.Text(c, label).FontSize(10.5).FontWeight(600).SingleLine().Ellipsis("…")
	})
	box.HandleInput(func(ev ui.InputEvent) bool {
		if a.bindingCapture != target {
			return false
		}
		switch ev.Kind {
		case ui.InputKeyUp:
			return true
		case ui.InputKeyDown:
			if ev.Repeat {
				return true
			}
			if ev.Key == ui.KeyEscape {
				a.cancelBindingCapture()
				return true
			}
			if ev.Key == ui.KeyTab {
				return false
			}
			if ev.Key == ui.KeyUnknown {
				a.bindingCaptureNote = "只识别具名按键；按下鼠标侧键或键盘按键。"
				a.refresh()
				return true
			}
			a.applyBindingCapture(target, SideBinding{Key: ev.Key, Mods: ev.Mods})
			return true
		}
		return false
	})
}

func (a *App) swapSideBindings() {
	a.draft.SidePrev, a.draft.SideNext = a.draft.SideNext, a.draft.SidePrev
	a.draft.SidePrevHold, a.draft.SideNextHold = a.draft.SideNextHold, a.draft.SidePrevHold
	a.bindingCapture = ""
	a.bindingCaptureNote = ""
	a.refresh()
}

func settingsIcon(icon *ui.SVG) func(*ui.Context) {
	return func(c *ui.Context) {
		components.Icon(c, icon).Size(16, 16).TextColor(c.Theme().TextMuted)
	}
}
