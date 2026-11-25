package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

const (
	viewerHoldDelay = 400 * time.Millisecond
	viewerHoldSlop  = float32(8)
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

func clampViewerZoom(z float32) float32 {
	if math.IsNaN(float64(z)) || math.IsInf(float64(z), 0) {
		return 1
	}
	return max(0.15, min(12, z))
}

// viewerZoomAtPoint scales the image while keeping the bitmap point under the
// pointer at the same canvas position. Pan is relative to the canvas center,
// so both the pointer and the current pan are measured from that center.
func viewerZoomAtPoint(zoom, panX, panY, delta, x, y, w, h float32) (newZoom, newPanX, newPanY float32) {
	if zoom <= 0 || w <= 0 || h <= 0 || delta <= 0 {
		return zoom, panX, panY
	}
	newZoom = clampViewerZoom(zoom * delta)
	if newZoom == zoom {
		return zoom, panX, panY
	}
	ax := x - w/2
	ay := y - h/2
	ratio := newZoom / zoom
	return newZoom, ax - (ax-panX)*ratio, ay - (ay-panY)*ratio
}

// viewerImmersive reports whether the viewer is filling the host window. It
// deliberately does not consult Win32 fullscreen: the viewer never owns that
// window mode.
func (a *App) viewerImmersive() bool {
	return a.viewer.Open && a.viewer.Immersive
}

func viewerCanvasMetrics(w, h float32, immersive, info bool) (float32, float32) {
	if info && !immersive {
		w -= 300
	}
	if !immersive {
		h -= 50 + 52
	}
	return w, h
}

func (a *App) viewerCanvasSize(c *ui.Context) (float32, float32) {
	w, h := float32(1280), float32(800)
	if a.win != nil {
		cw, ch := a.win.ContentSize()
		w, h = float32(cw), float32(ch)
	} else if c != nil {
		w, h = c.Size()
	}
	return viewerCanvasMetrics(w, h, a.viewerImmersive(), a.viewer.ShowInfo)
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
	wasOpen := a.viewer.Open
	immersive := a.viewerImmersive()
	ui.DialogBase(c, &a.viewer.Open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.82))
		panel.Fill().Background(th.Background).Clip()
		ui.Column(c).Fill().Children(func() {
			if immersive {
				// The viewer fills the host window. Its title and controls are
				// transient overlays, never a Win32 fullscreen transition.
				a.viewerCanvas(c, it, th)
				if a.viewer.ChromeVisible {
					a.viewerHeader(c, it, th, true)
					a.viewerFooter(c, it, th, true)
				}
			} else {
				a.viewerHeader(c, it, th, false)
				ui.Row(c).Grow(1).Children(func() {
					a.viewerCanvas(c, it, th)
					if a.viewer.ShowInfo {
						a.viewerInfoPanel(c, it, th)
					}
				})
				a.viewerFooter(c, it, th, false)
			}
			a.viewerShortcuts(c, it)
		})
	})

	if wasOpen && !a.viewer.Open {
		a.closeViewer()
	}
}

func (a *App) viewerHeader(c *ui.Context, it *Item, th *ui.Theme, floating bool) {
	t := a.tab()
	bar := ui.Row(c).Height(50).Padding(8, 14, 8, 14).Gap(8)
	if floating {
		bar.Absolute().Left(0).Right(0).Top(0).
			Background(th.Surface.Alpha(.94)).BorderWidth(0, 0, 1, 0).BorderColor(th.Border.Alpha(.6))
	} else {
		bar.Background(th.Surface).BorderWidth(0, 0, 1, 0).BorderColor(th.Border.Alpha(.55))
	}
	bar.Children(func() {
		icon := lucide.Image()
		if it.Kind == "video" {
			icon = lucide.Film()
		}
		ui.Box(c).Size(30, 30).Radius(8).Background(th.Accent.Alpha(.12)).Center().Children(func() {
			components.Icon(c, icon).Size(15, 15).TextColor(th.Accent)
		})
		title := ui.Column(c).Grow(1).MinWidth(0)
		if !floating {
			title.DragWindow()
		}
		title.Children(func() {
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
		if !floating {
			infoLabel := "显示信息 (I)"
			if a.viewer.ShowInfo {
				infoLabel = "隐藏信息 (I)"
			}
			info := components.IconButton(c, lucide.Info(), infoLabel, components.IconButtonPlain)
			if a.viewer.ShowInfo {
				info.TextColor(th.Accent)
			}
			if info.Clicked() {
				a.viewerToggleInfo()
			}
		}
		immersiveLabel := "进入沉浸 (F11)"
		immersiveIcon := lucide.Fullscreen()
		if a.viewer.Immersive {
			immersiveLabel = "退出沉浸 (F11)"
			immersiveIcon = lucide.Minimize2()
		}
		if components.IconButton(c, immersiveIcon, immersiveLabel, components.IconButtonPlain).Clicked() {
			a.viewerToggleImmersive()
		}
		components.IconButton(c, lucide.X(), "关闭查看器 (Esc)", components.IconButtonPlain).
			OnClick(a.closeViewer)
	})
}

func (a *App) viewerShortcuts(c *ui.Context, it *Item) {
	c.OnWindowShortcut(0, ui.KeyLeft, func() { a.viewerStep(-1) })
	c.OnWindowShortcut(0, ui.KeyRight, func() { a.viewerStep(1) })
	// Windows mouse side buttons arrive as back/forward keys. The settings page
	// records each direction's click and long-press shortcut, including vendor
	// drivers that send a different named key.
	a.viewerSideShortcuts(c)
	if !a.viewerImmersive() {
		// The info panel is deliberately absent in immersive viewer mode; F11
		// returns to the windowed layout where it can be shown again.
		c.OnWindowShortcut(0, ui.KeyI, func() { a.viewerToggleInfo() })
	}
	c.OnWindowShortcut(0, ui.KeyF11, func() { a.viewerToggleImmersive() })
	if it.Kind == "video" {
		c.OnWindowShortcut(0, ui.KeySpace, func() { a.viewerTogglePlay() })
		c.OnWindowShortcut(0, ui.KeyJ, func() { a.viewerSeekBy(-10 * time.Second) })
		c.OnWindowShortcut(0, ui.KeyL, func() { a.viewerSeekBy(10 * time.Second) })
		c.OnWindowShortcut(0, ui.KeyM, func() { a.viewerToggleMute() })
		c.OnWindowShortcut(ui.Shift, ui.KeyLeft, func() { a.viewerSeekBy(-10 * time.Second) })
		c.OnWindowShortcut(ui.Shift, ui.KeyRight, func() { a.viewerSeekBy(10 * time.Second) })
		return
	}
	c.OnWindowShortcut(0, ui.KeyEqual, func() { a.viewerZoom(1.25) })
	// "+" is Shift+"=" on a US keyboard; accept both spellings.
	c.OnWindowShortcut(ui.Shift, ui.KeyEqual, func() { a.viewerZoom(1.25) })
	c.OnWindowShortcut(0, ui.KeyMinus, func() { a.viewerZoom(0.8) })
	c.OnWindowShortcut(0, ui.Key0, func() { a.viewerReset() })
	c.OnWindowShortcut(0, ui.Key1, func() { a.viewerActualSize() })
	c.OnWindowShortcut(0, ui.KeyC, func() { a.viewerCenter() })
	c.OnWindowShortcut(0, ui.KeyR, func() { a.viewerRotate(90) })
	c.OnWindowShortcut(ui.Shift, ui.KeyR, func() { a.viewerRotate(-90) })
}

func (a *App) viewerFooter(c *ui.Context, it *Item, th *ui.Theme, floating bool) {
	bar := ui.Row(c).Height(52).Padding(8, 12, 8, 12).Gap(4)
	if floating {
		bar.Absolute().Left(0).Right(0).Bottom(0).
			Background(th.Surface.Alpha(.94)).BorderWidth(1, 0, 0, 0).BorderColor(th.Border.Alpha(.6))
	} else {
		bar.Background(th.Surface).BorderWidth(1, 0, 0, 0).BorderColor(th.Border.Alpha(.55))
	}
	bar.Children(func() {
		if it.Kind == "video" {
			a.viewerVideoControls(c, it, th)
		} else {
			a.viewerImageControls(c, th)
			ui.Spacer(c)
		}
		a.viewerNavControls(c, it, th)
	})
}

func (a *App) viewerImageControls(c *ui.Context, th *ui.Theme) {
	components.Text(c, fmt.Sprintf("%d%%", a.viewerPercent(c))).FontSize(10.5).
		TextColor(th.TextMuted).Width(52)
	components.IconButton(c, lucide.ZoomOut(), "缩小 (−)", components.IconButtonPlain).
		OnClick(func() { a.viewerZoom(0.8) })
	components.IconButton(c, lucide.ZoomIn(), "放大 (+, =)", components.IconButtonPlain).
		OnClick(func() { a.viewerZoom(1.25) })
	components.IconButton(c, lucide.Maximize2(), "适合窗口 (0)", components.IconButtonPlain).
		OnClick(a.viewerReset)
	components.IconButton(c, lucide.ScanLine(), "实际大小 1:1 (1)", components.IconButtonPlain).
		OnClick(a.viewerActualSize)
	components.IconButton(c, lucide.Crosshair(), "居中图片 (C)", components.IconButtonPlain).
		OnClick(a.viewerCenter)
	components.IconButton(c, lucide.RotateCw(), "顺时针旋转 (R)", components.IconButtonPlain).
		OnClick(func() { a.viewerRotate(90) })
	ui.Box(c).Width(1).Height(20).Background(th.Border).Margin(2, 6)
}

func (a *App) viewerVideoControls(c *ui.Context, it *Item, th *ui.Theme) {
	p := a.viewer.Player
	if p == nil {
		ui.Spacer(c)
		return
	}
	playing := p.Playing()
	playIcon, playLabel := lucide.Play(), "播放 (Space)"
	if playing {
		playIcon, playLabel = lucide.Pause(), "暂停 (Space)"
	}
	if components.IconButton(c, playIcon, playLabel, components.IconButtonPlain).Clicked() {
		a.viewerTogglePlay()
	}
	components.IconButton(c, lucide.Rewind(), "后退 10 秒 (J)", components.IconButtonPlain).
		OnClick(func() { a.viewerSeekBy(-10 * time.Second) })
	components.IconButton(c, lucide.FastForward(), "前进 10 秒 (L)", components.IconButtonPlain).
		OnClick(func() { a.viewerSeekBy(10 * time.Second) })

	pos, dur := p.Position(), p.Duration()
	if dur > 0 && pos > dur {
		pos = dur
	}
	components.Text(c, fmt.Sprintf("%s / %s", formatClock(pos), formatClock(dur))).
		FontSize(10.5).TextColor(th.TextMuted).SingleLine().Width(104)

	durSec := dur.Seconds()
	sliderDisabled := durSec <= 0
	if durSec <= 0 {
		durSec = 1
	}
	now := time.Now()
	if !a.viewer.DraggingSeek && now.After(a.viewer.SeekUntil) {
		a.viewer.SeekSeconds = math.Max(0, math.Min(durSec, pos.Seconds()))
	}
	ui.Box(c).Grow(1).MinWidth(70).Height(24).Children(func() {
		slider := components.Slider(c, &a.viewer.SeekSeconds, 0, durSec)
		slider.Label("播放进度").FillWidth()
		if sliderDisabled {
			slider.Disabled(true)
		}
		if slider.Changed() {
			a.viewer.SeekUntil = time.Now().Add(700 * time.Millisecond)
			a.viewerSeekTo(a.viewer.SeekSeconds)
		}
		_, _, held := slider.Dragged()
		a.viewer.DraggingSeek = held
	})

	rateLabel := fmt.Sprintf("%g×", p.Rate())
	if p.Rate() == 1 {
		rateLabel = "1×"
	}
	speed := components.Button(c, rateLabel, components.ButtonGhost).
		Size(44, 28).Padding(0, 6).Radius(7).
		Tooltip("播放速度")
	if speed.Clicked() {
		a.viewerCycleRate()
	}

	muteIcon, muteLabel := lucide.Volume2(), "静音 (M)"
	if p.Muted() {
		muteIcon, muteLabel = lucide.VolumeX(), "取消静音 (M)"
	}
	if components.IconButton(c, muteIcon, muteLabel, components.IconButtonPlain).Clicked() {
		a.viewerToggleMute()
	}
	loopIcon := lucide.Repeat()
	if a.settings.LoopVideo {
		loopIcon = lucide.Repeat1()
	}
	loop := components.IconButton(c, loopIcon, "循环播放", components.IconButtonPlain)
	if a.settings.LoopVideo {
		loop.TextColor(th.Accent)
	}
	if loop.Clicked() {
		a.settings.LoopVideo = !a.settings.LoopVideo
		_ = saveSettings(a.settings)
		p.SetLoop(a.settings.LoopVideo)
		a.refresh()
	}
}

func (a *App) viewerNavControls(c *ui.Context, it *Item, th *ui.Theme) {
	_ = it
	pos, total := 0, 0
	if t := a.tab(); t != nil {
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
	_ = th
}

func (a *App) cancelViewerHold() {
	a.viewer.HoldActive = false
	a.viewer.HoldButton = -1
	a.viewer.HoldAt = time.Time{}
}

func (a *App) beginViewerHold(button int, x, y float32, at time.Time) {
	if button != 0 && button != 1 {
		return
	}
	a.viewer.HoldActive = true
	a.viewer.HoldButton = button
	a.viewer.HoldAt = at
	a.viewer.HoldX = x
	a.viewer.HoldY = y
}

func (a *App) moveViewerHold(x, y float32) {
	if !a.viewer.HoldActive {
		return
	}
	dx, dy := x-a.viewer.HoldX, y-a.viewer.HoldY
	if dx*dx+dy*dy > viewerHoldSlop*viewerHoldSlop {
		a.cancelViewerHold()
	}
}

func (a *App) endViewerHold(button int, canceled bool) {
	if !a.viewer.HoldActive {
		return
	}
	if canceled || button == a.viewer.HoldButton {
		a.cancelViewerHold()
	}
}

// finishViewerHold turns a stationary long press into a pointer-anchored jump
// to 100% on the primary button, and a fit-and-center reset on the secondary
// button. If the image already displays at 100% or more, the primary gesture
// does nothing. The secondary gesture ignores the pointer position entirely.
func (a *App) finishViewerHold(c *ui.Context, w, h float32) {
	if !a.viewer.HoldActive {
		return
	}
	elapsed := c.Now().Sub(a.viewer.HoldAt)
	if elapsed < viewerHoldDelay {
		c.After(viewerHoldDelay - elapsed)
		return
	}
	button, x, y := a.viewer.HoldButton, a.viewer.HoldX, a.viewer.HoldY
	a.cancelViewerHold()
	if button == 0 {
		a.viewerActualSizeAt(x, y, w, h)
	} else if button == 1 {
		a.viewerFit()
	}
}

// showViewerCursor gives the pointer its own idle clock. Moving over the
// canvas wakes it even when the floating bars are hidden, so the two
// visibility states cannot strand a hidden cursor.
func (a *App) showViewerCursor() {
	if a.wakeViewerCursor() {
		a.refresh()
	}
}

// wakeViewerCursor extends the cursor's idle clock and reports whether it
// changed from hidden to visible. Input handlers and the chrome ticker both use
// it, so the cursor wakes whether a pointer event reaches the canvas or only
// the physical pointer moved.
func (a *App) wakeViewerCursor() bool {
	if !a.viewer.Open || !a.viewer.Immersive {
		return false
	}
	a.viewer.CursorUntil = time.Now().Add(viewerCursorIdle)
	if !a.viewer.CursorVisible {
		a.viewer.CursorVisible = true
		if a.win != nil {
			restoreCursorOverWindow(a.win.NativeHandle())
		}
		return true
	}
	return false
}

func viewerCursorShouldHide(immersive, visible bool, now, until time.Time) bool {
	return immersive && visible && !now.Before(until)
}

func viewerCursorMoved(tracked bool, x0, y0, x, y int32) bool {
	return tracked && (x != x0 || y != y0)
}

func (a *App) viewerCanvas(c *ui.Context, it *Item, th *ui.Theme) {
	ui.Box(c).Grow(1).FillHeight().Clip().Children(func() {
		canvasW, canvasH := a.viewerCanvasSize(c)
		box := ui.Box(c).Fill().Focusable().Label("媒体查看器")
		a.finishViewerHold(c, canvasW, canvasH)
		if a.viewerImmersive() && !a.viewer.CursorVisible {
			box.Cursor(ui.CursorNone)
		}
		box.Draw(func(p *ui.Painter, r ui.Rect) {
			switch {
			case it.Kind == "video" && a.viewer.Player != nil:
				p.Fill(r, ui.RGBA(0, 0, 0, 245), 0)
				if !a.viewer.Player.Ready() {
					msg := "正在载入视频…"
					if err := a.viewer.Player.Err(); err != nil {
						msg = err.Error()
					}
					w, _ := p.MeasureText(0, ui.Span{Text: msg, Size: 13})
					p.Text(r.X+(r.W-w)/2, r.Y+r.H/2, msg, 13, ui.RGBA(230, 230, 235, 255))
				}
			case a.viewer.Bitmap == nil && a.viewer.Placeholder != nil:
				p.Fill(r, ui.RGBA(0, 0, 0, 245), 0)
				bw, bh := a.viewer.Placeholder.Size()
				if bw > 0 && bh > 0 {
					fit := viewerFitScale(bw, bh, a.viewer.Angle, r.W, r.H)
					s := fit
					cx := r.X + r.W/2
					cy := r.Y + r.H/2
					w, h := float32(bw), float32(bh)
					p.Transform(ui.Scaling(s, s).Then(ui.Rotation(a.viewer.Angle)).Then(ui.Translation(cx, cy)), 0, 0, func() {
						p.Image(a.viewer.Placeholder, ui.Rect{X: -w / 2, Y: -h / 2, W: w, H: h}, ui.FillBox)
					})
				}
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
			if it.Kind == "image" {
				pct := -1
				if a.viewer.Bitmap != nil {
					bw, bh := a.viewer.Bitmap.Size()
					pct = viewerPercentValue(bw, bh, a.viewer.Angle, a.viewer.Zoom, r.W, r.H)
				}
				a.drawViewerHUD(p, r, th, pct)
			}
		})
		if a.viewer.Player != nil {
			ui.Video(c, a.viewer.Player).Absolute().Fill().Fit(ui.Contain).IgnoreInput(true)
		}
		if it.Kind == "video" {
			// The video branch returns before the image canvas handler below.
			// Without its own handler, moving over the video never woke the
			// independently timed cursor, leaving the pointer hidden over a
			// playing video. The physical-pointer ticker is a second net for
			// events the video layer consumes.
			box.HandleInput(func(ev ui.InputEvent) bool {
				a.showViewerCursor()
				return false
			})
			if box.DoubleClicked() {
				a.viewerToggleImmersive()
			} else if box.Clicked() {
				a.viewerTogglePlay()
			}
			return
		}
		if dx, dy, held := box.Dragged(); held {
			a.viewer.PanX += dx
			a.viewer.PanY += dy
		}
		box.HandleInput(func(ev ui.InputEvent) bool {
			a.showViewerCursor()
			switch ev.Kind {
			case ui.InputZoom:
				if ev.Scale > 0 {
					a.viewerZoomAt(ev.Scale, ev.X, ev.Y, canvasW, canvasH)
				}
				return true
			case ui.InputScroll:
				if ev.Precise {
					a.viewer.PanX -= ev.DX
					a.viewer.PanY -= ev.DY
					a.refresh()
				} else if ev.DY < 0 {
					a.viewerZoomAt(1.12, ev.X, ev.Y, canvasW, canvasH)
				} else if ev.DY > 0 {
					a.viewerZoomAt(0.89, ev.X, ev.Y, canvasW, canvasH)
				}
				return true
			case ui.InputPointerDown:
				at := ev.Time
				if at.IsZero() {
					at = c.Now()
				}
				a.beginViewerHold(ev.Button, ev.X, ev.Y, at)
				if a.viewer.HoldActive {
					c.After(viewerHoldDelay)
				}
				return false
			case ui.InputPointerMove:
				a.updateViewerHover(ev.X, ev.Y, canvasW, canvasH)
				a.moveViewerHold(ev.X, ev.Y)
				return false
			case ui.InputPointerUp:
				a.endViewerHold(ev.Button, ev.Canceled)
				return false
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
		ext := stringsUpper(it.Ext)
		if ext == "" {
			ext = "文件"
		}
		components.Text(c, ext+" · "+(MediaSvc{}).FormatSize(it.Size)).FontSize(10).TextColor(th.TextMuted).SingleLine().Ellipsis("…")
		components.Text(c, "修改于 "+(AppSvc{}).FormatTime(it.Mtime)).FontSize(9.5).TextColor(th.TextMuted).SingleLine().Ellipsis("…")
		ui.Divider(c)
		ui.Scroll(c).Grow(1).Scrollbars(true).Children(func() {
			ui.Column(c).Gap(5).Children(func() {
				if it.Kind == "video" {
					a.videoInfoRows(c, it)
				} else if a.viewer.InfoReady {
					if a.viewer.Info.Width > 0 {
						infoRow(c, "尺寸", fmt.Sprintf("%d × %d", a.viewer.Info.Width, a.viewer.Info.Height))
					}
					switch {
					case a.viewer.FullResUnsupported:
						infoRow(c, "1:1", "原图超过 50MP，只用 2560 预览")
					case a.viewer.FullResReady:
						infoRow(c, "1:1", "已载入全分辨率")
					case a.viewer.FullResLoading:
						infoRow(c, "1:1", "正在载入全分辨率…")
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

func (a *App) videoInfoRows(c *ui.Context, it *Item) {
	p := a.viewer.Player
	meta := a.viewer.Video
	w, h := 0, 0
	dur := time.Duration(0)
	if p != nil {
		if p.Ready() {
			w, h = p.Size()
		}
		dur = p.Duration()
	}
	if w <= 0 || h <= 0 {
		w, h = meta.Width, meta.Height
	}
	if dur <= 0 && meta.DurationMS > 0 {
		dur = time.Duration(meta.DurationMS) * time.Millisecond
	}
	res := "—"
	if w > 0 && h > 0 {
		res = fmt.Sprintf("%d × %d", w, h)
	}
	infoRow(c, "分辨率", res)
	if dur > 0 {
		infoRow(c, "时长", formatClock(dur))
	}
	if !a.viewer.VideoReady {
		components.Text(c, "正在读取视频元数据…").FontSize(10.5).TextColor(c.Theme().TextMuted)
		return
	}
	fps := "—"
	if meta.FPS > 0 {
		fps = fmt.Sprintf("%.1f fps", meta.FPS)
	}
	infoRow(c, "帧率", fps)
	label, bitrate := "实时码率", 0
	if meta.LiveBitrate && p != nil {
		bitrate = meta.BitrateAtMS(p.Position().Milliseconds(), 2*time.Second)
	} else {
		label = "平均码率"
		bitrate = meta.BitrateKbps
		if bitrate <= 0 && dur > 0 {
			bitrate = int(math.Round(float64(it.Size) * 8 / dur.Seconds() / 1000))
		}
	}
	value := "—"
	if bitrate > 0 {
		if bitrate >= 1000 {
			value = fmt.Sprintf("%.2f Mbps", float64(bitrate)/1000)
		} else {
			value = fmt.Sprintf("%d kbps", bitrate)
		}
	}
	infoRow(c, label, value)
}

func infoRow(c *ui.Context, key, value string) {
	ui.Column(c).Gap(1).Children(func() {
		components.Text(c, key).FontSize(9.5).TextColor(c.Theme().TextMuted)
		components.Text(c, value).FontSize(11).TextColor(c.Theme().Text)
	})
}

func formatClock(d time.Duration) string {
	if d <= 0 {
		return "0:00"
	}
	d = d.Round(time.Second)
	if d >= time.Hour {
		return fmt.Sprintf("%d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
	}
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

func stringsUpper(s string) string {
	if s == "" {
		return "文件"
	}
	s = strings.TrimPrefix(s, ".")
	return strings.ToUpper(s)
}
