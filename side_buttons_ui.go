package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

func (a *App) beginSideCalibration(target string) {
	a.sideCalibrationTarget = target
	a.sideCalibrationGen++
	a.sideCaptureBinding = SideBinding{}
	a.sideCaptureNote = ""
	a.sideCalibrationOpen = true
	a.refresh()
}

func (a *App) applySideCalibration() {
	if !a.sideCalibrationOpen || a.sideCaptureBinding.Key == ui.KeyUnknown {
		return
	}
	if !viewerSideBindingAllowed(a.sideCaptureBinding) {
		return
	}
	applySideBinding(&a.draft.SidePrev, &a.draft.SideNext, a.sideCalibrationTarget, a.sideCaptureBinding)
	a.sideCaptureBinding = SideBinding{}
	a.sideCaptureNote = ""
	a.sideCalibrationOpen = false
	a.refresh()
}

func (a *App) sideCalibrationModal(c *ui.Context) {
	if !a.sideCalibrationOpen {
		return
	}
	if !a.showSettings {
		a.sideCalibrationOpen = false
		return
	}
	th := c.Theme()
	target := "上一张"
	if a.sideCalibrationTarget == "next" {
		target = "下一张"
	}
	valid := a.sideCaptureBinding.Key != ui.KeyUnknown && viewerSideBindingAllowed(a.sideCaptureBinding)
	ui.Modal(c, &a.sideCalibrationOpen, func() {
		ui.Column(c).Width(440).Gap(12).Children(func() {
			dialogHeader(c, lucide.MousePointerClick(), "校准鼠标侧键", "按下侧键即可识别，用来对应查看器的上一张或下一张")
			ui.Column(c).Gap(6).Children(func() {
				components.Text(c, "要分配给「"+target+"」的按键").FontSize(11).TextColor(th.TextMuted)
				box := ui.Box(c).Key(fmt.Sprintf("side-capture-%d", a.sideCalibrationGen)).
					FillWidth().Height(82).Radius(12).
					Background(th.SurfaceHover.Alpha(.28)).BorderWidth(1).
					Focusable().AutoFocus().TabStop(false).Label("侧键按键捕获").Center()
				if valid {
					box.BorderColor(th.Accent.Alpha(.65)).Background(th.Accent.Alpha(.08))
				} else {
					box.BorderColor(th.Border.Alpha(.8))
				}
				box.Children(func() {
					if a.sideCaptureBinding.Key == ui.KeyUnknown {
						components.Text(c, "请按下鼠标侧键…").FontSize(14).FontWeight(600)
						components.Text(c, "通常为后退 / 前进键，也支持鼠标驱动映射出的具名按键").
							FontSize(10).TextColor(th.TextMuted).SingleLine().Ellipsis("…")
					} else {
						components.Text(c, sideBindingLabel(a.sideCaptureBinding)).FontSize(20).Bold().
							TextColor(func() ui.Color {
								if valid {
									return th.Accent
								}
								return th.Text
							}())
						note := "已识别，确认后更新设置"
						if !valid {
							note = "该按键已被查看器或应用快捷键占用，请换一个侧键"
						}
						components.Text(c, note).FontSize(10).TextColor(func() ui.Color {
							if valid {
								return th.TextMuted
							}
							return th.Danger
						}())
					}
				})
				if a.sideCaptureNote != "" {
					components.Text(c, a.sideCaptureNote).FontSize(10.5).TextColor(th.Danger)
				}
				box.HandleInput(func(ev ui.InputEvent) bool {
					switch ev.Kind {
					case ui.InputKeyUp:
						return true
					case ui.InputKeyDown:
						if ev.Repeat {
							return true
						}
						if ev.Key == ui.KeyEscape {
							return false // let the modal handle its own close key
						}
						if ev.Key == ui.KeyTab {
							return false // keep keyboard focus traversal working
						}
						if ev.Key == ui.KeyUnknown {
							a.sideCaptureBinding = SideBinding{}
							a.sideCaptureNote = "只能识别具名按键；请先在鼠标驱动里把该侧键映射为后退 / 前进。"
							a.refresh()
							return true
						}
						a.sideCaptureBinding = SideBinding{Key: ev.Key, Mods: ev.Mods}
						a.sideCaptureNote = ""
						a.refresh()
						return true
					}
					return false
				})
			})
			components.Text(c, "应用后仍需在设置页点击“保存”；若侧键方向相反，可用设置页的“交换方向”。").
				FontSize(10).TextColor(th.TextMuted)
			ui.Row(c).Gap(8).Children(func() {
				if components.Button(c, "取消", components.ButtonGhost).Clicked() {
					a.sideCalibrationOpen = false
				}
				ui.Spacer(c)
				if a.sideCaptureBinding.Key != ui.KeyUnknown {
					if components.Button(c, "重新识别", components.ButtonOutline).Clicked() {
						a.beginSideCalibration(a.sideCalibrationTarget)
					}
				}
				apply := components.Button(c, "应用侧键", components.ButtonPrimary)
				apply.Disabled(!valid)
				if apply.Clicked() {
					a.applySideCalibration()
				}
			})
		})
	})
}

func (a *App) sideButtonSetting(c *ui.Context) {
	th := c.Theme()
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		a.sideMappingRow(c, "上一张", a.draft.SidePrev, "prev", th)
		a.sideMappingRow(c, "下一张", a.draft.SideNext, "next", th)
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if components.Button(c, "交换方向", components.ButtonOutline).Clicked() {
				a.swapSideBindings()
			}
			components.Text(c, "侧键只在查看器内生效；方向反了可以交换，也可以分别按一次实体侧键重新校准。").
				FontSize(10.5).TextColor(th.TextMuted).Grow(1)
		})
	})
}

func (a *App) sideMappingRow(c *ui.Context, title string, binding SideBinding, target string, th *ui.Theme) {
	ui.Row(c).FillWidth().Padding(8, 10).Radius(10).Gap(10).
		Background(th.Background.Alpha(.55)).BorderWidth(1).BorderColor(th.Border.Alpha(.55)).Children(func() {
		icon := lucide.ArrowLeft()
		if target == "next" {
			icon = lucide.ArrowRight()
		}
		ui.Box(c).Size(26, 26).Radius(7).Background(th.Accent.Alpha(.1)).Center().Children(func() {
			components.Icon(c, icon).Size(14, 14).TextColor(th.Accent)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			components.Text(c, title).FontSize(11.5).FontWeight(600).SingleLine()
			components.Text(c, "鼠标侧键").FontSize(10).TextColor(th.TextMuted).SingleLine().Ellipsis("…")
		})
		keyBox := ui.Box(c).Padding(4, 9).Radius(6).Background(th.SurfaceHover).
			BorderWidth(1).BorderColor(th.Border.Alpha(.7))
		if !viewerSideBindingAllowed(binding) {
			keyBox.BorderColor(th.Danger.Alpha(.6))
		}
		keyBox.Children(func() {
			components.Text(c, sideBindingLabel(binding)).FontSize(11).FontWeight(600).SingleLine()
		})
		if components.Button(c, "校准", components.ButtonGhost).Clicked() {
			a.beginSideCalibration(target)
		}
	})
}

func (a *App) swapSideBindings() {
	a.draft.SidePrev, a.draft.SideNext = a.draft.SideNext, a.draft.SidePrev
	a.refresh()
}

func settingsIcon(icon *ui.SVG) func(*ui.Context) {
	return func(c *ui.Context) {
		components.Icon(c, icon).Size(16, 16).TextColor(c.Theme().TextMuted.Alpha(.8))
	}
}
