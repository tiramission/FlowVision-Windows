package main

import (
	"fmt"
	"math"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// nextViewerIndex walks past folders and other files so the arrow buttons
// only land on something the viewer can actually decode.
func nextViewerIndex(items []*Item, from, delta int) int {
	if delta == 0 {
		return -1
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	for i := from + step; i >= 0 && i < len(items); i += step {
		if isMediaKind(items[i].Kind) {
			return i
		}
	}
	return -1
}

func viewerMediaPosition(items []*Item, target *Item) (position, total int) {
	for _, it := range items {
		if !isMediaKind(it.Kind) {
			continue
		}
		total++
		if target != nil && it.ID == target.ID {
			position = total
		}
	}
	return position, total
}

// viewerFitScale is the scale that makes the rotated image fit its viewport.
// Rotating a wide image used to keep the unrotated fit, which let the rotated
// image overflow the bottom or side of the canvas.
func viewerFitScale(bw, bh int, angle, availW, availH float32) float32 {
	if bw <= 0 || bh <= 0 || availW <= 0 || availH <= 0 {
		return 0
	}
	rad := float64(angle) * math.Pi / 180
	boundsW := math.Abs(float64(bw)*math.Cos(rad)) + math.Abs(float64(bh)*math.Sin(rad))
	boundsH := math.Abs(float64(bw)*math.Sin(rad)) + math.Abs(float64(bh)*math.Cos(rad))
	if boundsW <= 0 || boundsH <= 0 {
		return 0
	}
	return float32(math.Min(float64(availW)/boundsW, float64(availH)/boundsH))
}

func viewerPercentValue(bw, bh int, angle, zoom, availW, availH float32) int {
	fitted := viewerFitScale(bw, bh, angle, availW, availH)
	if fitted <= 0 {
		return int(math.Round(float64(zoom * 100)))
	}
	return int(math.Round(float64(fitted*zoom) * 100))
}

// viewerActualZoom converts a fit scale into the relative zoom that displays
// the image at 100%, i.e. one bitmap pixel per logical display pixel.
func viewerActualZoom(fit float32) float32 {
	if fit <= 0 || math.IsNaN(float64(fit)) || math.IsInf(float64(fit), 0) {
		return 1
	}
	return 1 / fit
}

func (a *App) viewerCanvasSize(c *ui.Context) (float32, float32) {
	w, h := float32(1280), float32(800)
	if a.win != nil {
		cw, ch := a.win.ContentSize()
		w, h = float32(cw), float32(ch)
	} else if c != nil {
		w, h = c.Size()
	}
	if a.viewer.ShowInfo {
		w -= 300
	}
	h -= 50 + 48
	return w, h
}

// viewerPercent reports the real display scale (image pixels to logical
// pixels), not the relative fit multiplier. 100% therefore means 1:1.
func (a *App) viewerPercent(c *ui.Context) int {
	if a.viewer.Bitmap == nil || a.viewer.Item == nil || a.viewer.Item.Kind != "image" {
		return int(math.Round(float64(a.viewer.Zoom * 100)))
	}
	bw, bh := a.viewer.Bitmap.Size()
	w, h := a.viewerCanvasSize(c)
	if w <= 0 || h <= 0 {
		return 100
	}
	return viewerPercentValue(bw, bh, a.viewer.Angle, a.viewer.Zoom, w, h)
}

func (a *App) viewerOverlay(c *ui.Context) {
	if !a.viewer.Open {
		if a.viewer.Player != nil || a.viewer.Item != nil {
			a.closeViewer()
		}
		return
	}
	if a.viewer.Item == nil {
		a.closeViewer()
		return
	}
	th := c.Theme()
	it := a.viewer.Item
	t := a.tab()
	wasOpen := a.viewer.Open
	ui.DialogBase(c, &a.viewer.Open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.82))
		panel.Fill().Background(th.Background).Clip()
		ui.Column(c).Fill().Children(func() {
			ui.Row(c).Height(50).Padding(8, 14, 8, 14).Gap(8).Background(th.Surface).Children(func() {
				icon := lucide.Image()
				if it.Kind == "video" {
					icon = lucide.Film()
				}
				ui.Box(c).Size(30, 30).Radius(8).Background(th.Accent.Alpha(.12)).Center().Children(func() {
					components.Icon(c, icon).Size(15, 15).TextColor(th.Accent)
				})
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					components.Text(c, it.Name).FontSize(13.5).FontWeight(600).SingleLine().Ellipsis("…")
					if t != nil {
						pos, total := viewerMediaPosition(t.Items, a.viewer.Item)
						if total == 0 {
							pos, total = a.viewer.Index+1, len(t.Items)
						}
						if total > 0 {
							components.Text(c, fmt.Sprintf("%d / %d", pos, total)).
								FontSize(10).TextColor(th.TextMuted)
						}
					}
				})
				infoLabel := "显示信息 (I)"
				if a.viewer.ShowInfo {
					infoLabel = "隐藏信息 (I)"
				}
				components.IconButton(c, lucide.Info(), infoLabel, components.IconButtonPlain).
					OnClick(a.viewerToggleInfo)
				components.IconButton(c, lucide.X(), "关闭查看器 (Esc)", components.IconButtonPlain).
					OnClick(a.closeViewer)
			})

			ui.Row(c).Grow(1).Children(func() {
				a.viewerCanvas(c, it, th)
				if a.viewer.ShowInfo {
					a.viewerInfoPanel(c, it, th)
				}
			})

			ui.Row(c).Height(48).Padding(8, 14, 8, 14).Gap(4).Background(th.Surface).Children(func() {
				if it.Kind == "image" {
					components.Text(c, fmt.Sprintf("%d%%", a.viewerPercent(c))).FontSize(10.5).
						TextColor(th.TextMuted).Width(52)
					components.IconButton(c, lucide.ZoomOut(), "缩小 (−)", components.IconButtonPlain).
						OnClick(func() { a.viewerZoom(0.8) })
					components.IconButton(c, lucide.ZoomIn(), "放大 (+, =)", components.IconButtonPlain).
						OnClick(func() { a.viewerZoom(1.25) })
					components.IconButton(c, lucide.Maximize2(), "适合窗口 (0)", components.IconButtonPlain).
						OnClick(a.viewerReset)
					components.IconButton(c, lucide.RotateCw(), "顺时针旋转 (R)", components.IconButtonPlain).
						OnClick(func() { a.viewerRotate(90) })
					ui.Box(c).Width(1).Height(20).Background(th.Border).Margin(2, 6)
				}
				ui.Spacer(c)
				if a.viewer.Player != nil {
					label := "播放"
					if a.viewer.Player.Playing() {
						label = "暂停"
					}
					if components.Button(c, label, components.ButtonSecondary).Clicked() {
						if a.viewer.Player.Playing() {
							a.viewer.Player.Pause()
						} else {
							a.viewer.Player.Play()
						}
					}
					if components.Switch(c, &a.settings.LoopVideo, "循环", components.SwitchOptions{Compact: true}).Changed() {
						_ = saveSettings(a.settings)
						a.viewer.Player.SetLoop(a.settings.LoopVideo)
					}
				}
				pos, total := 0, 0
				if t != nil {
					pos, total = viewerMediaPosition(t.Items, a.viewer.Item)
				}
				prev := components.IconButton(c, lucide.ChevronLeft(), "上一张 (←)", components.IconButtonPlain).
					Disabled(pos <= 1)
				if prev.Clicked() {
					a.viewerStep(-1)
				}
				next := components.IconButton(c, lucide.ChevronRight(), "下一张 (→)", components.IconButtonPlain).
					Disabled(total == 0 || pos >= total)
				if next.Clicked() {
					a.viewerStep(1)
				}
			})

			c.OnWindowShortcut(0, ui.KeyLeft, func() { a.viewerStep(-1) })
			c.OnWindowShortcut(0, ui.KeyRight, func() { a.viewerStep(1) })
			c.OnWindowShortcut(0, ui.KeyEqual, func() { a.viewerZoom(1.25) })
			// "+" is Shift+"=" on a US keyboard; accept both spellings.
			c.OnWindowShortcut(ui.Shift, ui.KeyEqual, func() { a.viewerZoom(1.25) })
			c.OnWindowShortcut(0, ui.KeyMinus, func() { a.viewerZoom(0.8) })
			c.OnWindowShortcut(0, ui.Key0, func() { a.viewerReset() })
			c.OnWindowShortcut(0, ui.Key1, func() { a.viewerActualSize() })
			c.OnWindowShortcut(0, ui.KeyR, func() { a.viewerRotate(90) })
			c.OnWindowShortcut(ui.Shift, ui.KeyR, func() { a.viewerRotate(-90) })
			c.OnWindowShortcut(0, ui.KeyI, func() { a.viewerToggleInfo() })
		})
	})

	if wasOpen && !a.viewer.Open {
		a.closeViewer()
	}
}

func (a *App) viewerCanvas(c *ui.Context, it *Item, th *ui.Theme) {
	ui.Box(c).Grow(1).FillHeight().Clip().Children(func() {
		box := ui.Box(c).Fill().Focusable().Label("媒体查看器")
		box.Draw(func(p *ui.Painter, r ui.Rect) {
			switch {
			case it.Kind == "video" && a.viewer.Player != nil:
				p.Fill(r, ui.RGBA(0, 0, 0, 245), 0)
			case a.viewer.Bitmap == nil:
				p.Fill(r, ui.RGBA(0, 0, 0, 245), 0)
				msg := "正在载入…"
				if a.viewer.Err != "" {
					msg = a.viewer.Err
				}
				w, _ := p.MeasureText(0, ui.Span{Text: msg, Size: 13})
				p.Text(r.X+(r.W-w)/2, r.Y+r.H/2, msg, 13, ui.RGBA(230, 230, 235, 255))
			default:
				bw, bh := a.viewer.Bitmap.Size()
				if bw <= 0 || bh <= 0 {
					return
				}
				p.Fill(r, ui.RGBA(0, 0, 0, 245), 0)
				fit := viewerFitScale(bw, bh, a.viewer.Angle, r.W, r.H)
				s := fit * a.viewer.Zoom
				cx := r.X + r.W/2 + a.viewer.PanX
				cy := r.Y + r.H/2 + a.viewer.PanY
				w, h := float32(bw), float32(bh)
				p.Transform(ui.Scaling(s, s).Then(ui.Rotation(a.viewer.Angle)).Then(ui.Translation(cx, cy)), 0, 0, func() {
					p.Image(a.viewer.Bitmap, ui.Rect{X: -w / 2, Y: -h / 2, W: w, H: h}, ui.FillBox)
				})
			}
		})
		if a.viewer.Player != nil {
			ui.Video(c, a.viewer.Player).Absolute().Fill().Fit(ui.Contain).IgnoreInput(true)
		}
		if dx, dy, held := box.Dragged(); held {
			a.viewer.PanX += dx
			a.viewer.PanY += dy
		}
		box.HandleInput(func(ev ui.InputEvent) bool {
			switch ev.Kind {
			case ui.InputZoom:
				if ev.Scale > 0 {
					a.viewerZoom(ev.Scale)
				}
				return true
			case ui.InputScroll:
				if ev.Precise {
					a.viewer.PanX -= ev.DX
					a.viewer.PanY -= ev.DY
				} else if ev.DY < 0 {
					a.viewerZoom(1.12)
				} else if ev.DY > 0 {
					a.viewerZoom(0.89)
				}
				a.refresh()
				return true
			}
			return false
		})
	})
	_ = th
}

func (a *App) viewerInfoPanel(c *ui.Context, it *Item, th *ui.Theme) {
	ui.Column(c).Width(300).FillHeight().Background(th.Surface).
		BorderStartWidth(1).BorderColor(th.Border.Alpha(.7)).
		Padding(12, 14).Gap(7).Children(func() {
		ui.Row(c).Gap(7).Children(func() {
			components.Icon(c, lucide.SlidersHorizontal()).Size(14, 14).TextColor(th.TextMuted)
			components.Text(c, "信息").FontSize(12).Bold()
		})
		components.Text(c, it.Name).FontSize(11.5).FontWeight(600).SingleLine().Ellipsis("…")
		components.Text(c, it.Path).FontSize(9.5).TextColor(th.TextMuted).SingleLine().Ellipsis("…")
		ui.Divider(c)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(5).Children(func() {
				infoRow(c, "类型", stringsUpper(it.Ext))
				infoRow(c, "大小", (MediaSvc{}).FormatSize(it.Size))
				infoRow(c, "修改时间", (AppSvc{}).FormatTime(it.Mtime))
				if a.viewer.InfoReady {
					if a.viewer.Info.Width > 0 {
						infoRow(c, "尺寸", fmt.Sprintf("%d × %d", a.viewer.Info.Width, a.viewer.Info.Height))
					}
					for _, kv := range a.viewer.Info.Exif {
						infoRow(c, kv.K, kv.V)
					}
				} else {
					components.Text(c, "正在读取元数据…").FontSize(10.5).TextColor(th.TextMuted)
				}
			})
		})
		ui.Divider(c)
		if components.Button(c, "用默认应用打开", components.ButtonSecondary).FillWidth().Clicked() {
			_ = (AppSvc{}).OpenExternal(it.Path)
		}
		if components.Button(c, "在资源管理器中显示", components.ButtonOutline).FillWidth().Clicked() {
			_ = (AppSvc{}).Reveal(it.Path)
		}
		if components.Button(c, "复制完整路径", components.ButtonGhost).FillWidth().Clicked() {
			_ = (AppSvc{}).CopyText(it.Path)
		}
	})
}

func infoRow(c *ui.Context, key, value string) {
	ui.Column(c).Gap(1).Children(func() {
		components.Text(c, key).FontSize(9.5).TextColor(c.Theme().TextMuted)
		components.Text(c, value).FontSize(11).TextColor(c.Theme().Text)
	})
}

func stringsUpper(s string) string {
	if s == "" {
		return "文件"
	}
	s = strings.TrimPrefix(s, ".")
	return strings.ToUpper(s)
}
