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
	viewerHoldDelay     = 400 * time.Millisecond
	viewerHoldSlop      = float32(8)
	viewerPanMinVisible = float32(48)
)

func viewerRotatedBounds(bw, bh int, angle float32) (float32, float32) {
	if bw <= 0 || bh <= 0 {
		return 0, 0
	}
	rad := float64(angle) * math.Pi / 180
	boundsW := float32(math.Abs(float64(bw)*math.Cos(rad)) + math.Abs(float64(bh)*math.Sin(rad)))
	boundsH := float32(math.Abs(float64(bw)*math.Sin(rad)) + math.Abs(float64(bh)*math.Cos(rad)))
	return boundsW, boundsH
}

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
	boundsW, boundsH := viewerRotatedBounds(bw, bh, angle)
	if boundsW <= 0 || boundsH <= 0 {
		return 0
	}
	return float32(math.Min(float64(availW)/float64(boundsW), float64(availH)/float64(boundsH)))
}

// viewerBitmapPercentValue reports the scale of the bitmap currently on
// screen, relative to that bitmap's own pixels. It is used to decide when the
// 2560 preview itself has been enlarged enough to need the original.
func viewerBitmapPercentValue(bw, bh int, angle, zoom, availW, availH float32) int {
	fitted := viewerFitScale(bw, bh, angle, availW, availH)
	if fitted <= 0 {
		return int(math.Round(float64(zoom * 100)))
	}
	return int(math.Round(float64(fitted*zoom) * 100))
}

// viewerPercentValue reports the display scale against the original image,
// not against whichever preview bitmap happens to be loaded. A 2560 preview
// and the full-resolution bitmap therefore show the same percentage when they
// occupy the same area on screen.
func viewerPercentValue(bw, bh, refW, refH int, angle, zoom, availW, availH float32) int {
	fitted := viewerFitScale(bw, bh, angle, availW, availH)
	if fitted <= 0 {
		return int(math.Round(float64(zoom * 100)))
	}
	scale := fitted * zoom
	switch {
	case refW > 0 && bw > 0:
		scale *= float32(bw) / float32(refW)
	case refH > 0 && bh > 0:
		scale *= float32(bh) / float32(refH)
	}
	return int(math.Round(float64(scale) * 100))
}

// viewerActualZoom converts a fit scale into the relative zoom that displays
// the image at 100%, i.e. one bitmap pixel per logical display pixel.
func viewerActualZoom(fit float32) float32 {
	if fit <= 0 || math.IsNaN(float64(fit)) || math.IsInf(float64(fit), 0) {
		return 1
	}
	return 1 / fit
}

// viewerZoomMaxRelative is the ceiling for the relative zoom on ordinary
// images. A 3000 px photo in a 1200 px canvas fits at 0.4, so 12× means a 480%
// view before the wheel stops.
const viewerZoomMaxRelative = 12

// viewerZoomMax is the ceiling that keeps 100% reachable. Zoom is relative to
// the fit scale, and a 20000 px image in a 1200 px canvas needs 16.7× fit for
// 1:1: a fixed ceiling of 12 made actual size unreachable for exactly the
// images that need it. Large images therefore get 100% plus a little headroom,
// while ordinary images keep the old limit.
func viewerZoomMax(fit float32) float32 {
	limit := float32(viewerZoomMaxRelative)
	if actual := viewerActualZoom(fit); actual > limit {
		limit = actual * 1.5
	}
	return limit
}

func clampViewerZoom(z, fit float32) float32 {
	if math.IsNaN(float64(z)) || math.IsInf(float64(z), 0) {
		return 1
	}
	return max(0.15, min(viewerZoomMax(fit), z))
}

// viewerAnchorPan keeps a canvas point fixed while the image scale changes.
// Pan is relative to the canvas center, so the pointer must be measured from
// that center too; using the raw canvas coordinate shifts the 100% image by
// half a canvas width/height.
func viewerAnchorPan(px, py, w, h, panX, panY, ratio float32) (float32, float32) {
	ax := px - w/2
	ay := py - h/2
	return ax - (ax-panX)*ratio, ay - (ay-panY)*ratio
}

// viewerZoomAtPoint scales the image while keeping the bitmap point under the
// pointer at the same canvas position. Pan is relative to the canvas center,
// so both the pointer and the current pan are measured from that center.
func viewerZoomAtPoint(zoom, panX, panY, delta, x, y, w, h, fit float32) (newZoom, newPanX, newPanY float32) {
	if zoom <= 0 || w <= 0 || h <= 0 || delta <= 0 {
		return zoom, panX, panY
	}
	newZoom = clampViewerZoom(zoom*delta, fit)
	if newZoom == zoom {
		return zoom, panX, panY
	}
	ratio := newZoom / zoom
	panX, panY = viewerAnchorPan(x, y, w, h, panX, panY, ratio)
	return newZoom, panX, panY
}

// viewerPointInBitmap reports whether a canvas point is over the visible
// bitmap rather than the letterbox around it. The bitmap is centered at
// (w/2+panX, h/2+panY), scaled by fit*zoom, and rotated by angle.
func viewerPointInBitmap(x, y, w, h float32, bw, bh int, angle, zoom, panX, panY float32) bool {
	if bw <= 0 || bh <= 0 || w <= 0 || h <= 0 {
		return false
	}
	fit := viewerFitScale(bw, bh, angle, w, h)
	if fit <= 0 {
		return false
	}
	scale := fit * zoom
	if scale <= 0 {
		return false
	}
	rad := float64(angle) * math.Pi / 180
	cos := float32(math.Cos(rad))
	sin := float32(math.Sin(rad))
	dx := x - (w/2 + panX)
	dy := y - (h/2 + panY)
	lx := dx*cos + dy*sin
	ly := -dx*sin + dy*cos
	halfW := float32(bw) * scale / 2
	halfH := float32(bh) * scale / 2
	return float32(math.Abs(float64(lx))) <= halfW &&
		float32(math.Abs(float64(ly))) <= halfH
}

func (a *App) viewerPointInImage(x, y, w, h float32) bool {
	if a.viewer.Bitmap == nil || a.viewer.Item == nil || a.viewer.Item.Kind != "image" {
		return false
	}
	bw, bh := a.viewer.Bitmap.Size()
	return viewerPointInBitmap(x, y, w, h, bw, bh, a.viewer.Angle,
		a.viewer.Zoom, a.viewer.PanX, a.viewer.PanY)
}

// clampViewerPan keeps at least a small strip of the image inside the canvas.
// Without it a drag can move the bitmap completely off-screen, leaving an
// empty canvas with no way back except the fit/center buttons.
func (a *App) clampViewerPan(w, h float32) {
	if a.viewer.Bitmap == nil || w <= 0 || h <= 0 {
		return
	}
	bw, bh := a.viewer.Bitmap.Size()
	fit := viewerFitScale(bw, bh, a.viewer.Angle, w, h)
	if fit <= 0 {
		return
	}
	boundsW, boundsH := viewerRotatedBounds(bw, bh, a.viewer.Angle)
	dispW := boundsW * fit * a.viewer.Zoom
	dispH := boundsH * fit * a.viewer.Zoom
	if dispW <= 0 || dispH <= 0 {
		return
	}
	minX := min(viewerPanMinVisible, dispW, w/2)
	minY := min(viewerPanMinVisible, dispH, h/2)
	cx := w/2 + a.viewer.PanX
	cy := h/2 + a.viewer.PanY
	cx = max(minX-dispW/2, min(cx, w-minX+dispW/2))
	cy = max(minY-dispH/2, min(cy, h-minY+dispH/2))
	a.viewer.PanX = cx - w/2
	a.viewer.PanY = cy - h/2
}

// viewerImmersive reports whether the viewer is filling the host window. It
// deliberately does not consult Win32 fullscreen: the viewer never owns that
// window mode.
func (a *App) viewerImmersive() bool {
	return a.viewer.Open && a.viewer.Immersive
}

// viewerInfoPanelWidth keeps the information card from taking a third of a
// compact window. The stored 300 DIP card remains at normal widths; below
// 1000 DIP it contracts to 28% of the window, with a 248 DIP floor.
func viewerInfoPanelWidth(windowW float32) float32 {
	if windowW > 0 && windowW < viewerInfoPanelCompactFrom {
		return max(viewerInfoPanelMin, windowW*viewerInfoPanelCompactRatio)
	}
	return viewerInfoPanelDefault
}

func viewerCanvasMetrics(w, h float32, immersive, info bool) (float32, float32) {
	if info && !immersive {
		w -= viewerInfoPanelWidth(w)
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

// viewerPercent reports the real display scale against the original image
// dimensions, not the relative fit multiplier or the currently loaded preview.
// 100% therefore always means one original pixel per logical display pixel.
func (a *App) viewerPercent(c *ui.Context) int {
	if a.viewer.Bitmap == nil || a.viewer.Item == nil || a.viewer.Item.Kind != "image" {
		return int(math.Round(float64(a.viewer.Zoom * 100)))
	}
	bw, bh := a.viewer.Bitmap.Size()
	refW, refH := a.viewerReferenceSize()
	w, h := a.viewerCanvasSize(c)
	if w <= 0 || h <= 0 {
		return 100
	}
	return viewerPercentValue(bw, bh, refW, refH, a.viewer.Angle, a.viewer.Zoom, w, h)
}

// viewerReferenceSize is the fixed pixel size of the original image. The
// bitmap on screen can be a 2560 preview or the original, but percentages must
// not change just because that bitmap was swapped.
func (a *App) viewerReferenceSize() (int, int) {
	it := a.viewer.Item
	if it != nil {
		if it.W > 0 && it.H > 0 {
			return it.W, it.H
		}
		if a.viewer.Info.Width > 0 && a.viewer.Info.Height > 0 {
			return a.viewer.Info.Width, a.viewer.Info.Height
		}
		if d := imageDims(it.Path, it.Mtime); d.W > 0 && d.H > 0 {
			return d.W, d.H
		}
	}
	if a.viewer.Info.Width > 0 && a.viewer.Info.Height > 0 {
		return a.viewer.Info.Width, a.viewer.Info.Height
	}
	if a.viewer.Bitmap != nil {
		return a.viewer.Bitmap.Size()
	}
	return 0, 0
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
	// The viewer is a darkroom: neutral black chrome avoids tinting the
	// photograph, regardless of the gallery's light or dark appearance.
	vt := viewerPalette()
	applyThemePreferences(vt, c)
	c.SetTheme(vt)
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
		ui.Box(c).Size(30, 30).Radius(8).Background(th.SurfaceHover).Center().Children(func() {
			components.Icon(c, icon).Size(15, 15).TextColor(th.TextMuted)
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
		// Starring from the viewer is the natural place for it: the photo is
		// open, and the grid may be scrolled far away from it.
		starLabel := "收藏 (Space)"
		starIcon := lucide.Star()
		if a.isFavorite(it.Path) {
			starLabel = "取消收藏 (Space)"
			starIcon = lucide.StarOff()
		}
		star := components.IconButton(c, starIcon, starLabel, components.IconButtonPlain)
		if a.isFavorite(it.Path) {
			star.TextColor(th.Accent)
		}
		if star.Clicked() {
			item := it
			a.later(func() { a.toggleViewerFavorite(item) })
		}
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
	// Ctrl+arrows carry on into the neighbouring folder, the same move the
	// step buttons make at the ends. Shift+arrows seek in a video, so the
	// folder step takes the modifier nothing else uses.
	c.OnWindowShortcut(ui.Ctrl, ui.KeyLeft, func() { a.viewerFolderStep(-1) })
	c.OnWindowShortcut(ui.Ctrl, ui.KeyRight, func() { a.viewerFolderStep(1) })
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
		// Space plays a video, so starring a video uses Shift+Space; an image
		// has no transport control to compete with.
		c.OnWindowShortcut(ui.Shift, ui.KeySpace, func() { a.toggleViewerFavorite(it) })
		c.OnWindowShortcut(0, ui.KeySpace, func() { a.viewerTogglePlay() })
		c.OnWindowShortcut(0, ui.KeyJ, func() { a.viewerSeekBy(-10 * time.Second) })
		c.OnWindowShortcut(0, ui.KeyL, func() { a.viewerSeekBy(10 * time.Second) })
		c.OnWindowShortcut(0, ui.KeyM, func() { a.viewerToggleMute() })
		c.OnWindowShortcut(ui.Shift, ui.KeyLeft, func() { a.viewerSeekBy(-10 * time.Second) })
		c.OnWindowShortcut(ui.Shift, ui.KeyRight, func() { a.viewerSeekBy(10 * time.Second) })
		return
	}
	c.OnWindowShortcut(0, ui.KeySpace, func() { a.toggleViewerFavorite(it) })
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
	a.viewerQualityBadge(c, th)
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
	_ = th
	pos, total := 0, 0
	if t := a.tab(); t != nil {
		pos, total = viewerMediaPosition(t.Items, a.viewer.Item)
	}
	atStart := pos <= 1
	atEnd := total == 0 || pos >= total

	// At the ends the step becomes a folder step: the viewer carries on into
	// the neighbouring folder rather than stopping, which is how a folder of
	// photos is read as one run. The label says so, so the button never
	// silently means something else.
	prevLabel, prevIcon := "上一张 (←)", lucide.ChevronLeft()
	prevDisabled := atStart
	if atStart {
		if !a.viewerFolderStepPossible(-1) {
			prevDisabled = true
		} else {
			prevLabel, prevIcon = "上一文件夹的最后一张 (Ctrl+←)", lucide.ChevronsLeft()
			prevDisabled = false
		}
	}
	nextLabel, nextIcon := "下一张 (→)", lucide.ChevronRight()
	nextDisabled := atEnd
	if atEnd {
		if !a.viewerFolderStepPossible(1) {
			nextDisabled = true
		} else {
			nextLabel, nextIcon = "下一文件夹的第一张 (Ctrl+→)", lucide.ChevronsRight()
			nextDisabled = false
		}
	}

	prev := components.IconButton(c, prevIcon, prevLabel, components.IconButtonPlain).
		Disabled(prevDisabled)
	if prev.Clicked() {
		if atStart {
			a.viewerFolderStep(-1)
		} else {
			a.viewerStep(-1)
		}
	}
	next := components.IconButton(c, nextIcon, nextLabel, components.IconButtonPlain).
		Disabled(nextDisabled)
	if next.Clicked() {
		if atEnd {
			a.viewerFolderStep(1)
		} else {
			a.viewerStep(1)
		}
	}
}

// viewerFolderStepPossible reports whether there is a neighbouring folder to
// carry on into. It reads only the parent's listing, which is cheap and
// already cached by the file system.
func (a *App) viewerFolderStepPossible(dir int) bool {
	t := a.tab()
	if t == nil || t.Path == "" || isRemotePath(t.Path) || isFavoritesPath(t.Path) {
		return false
	}
	res := (FS{}).Sibling(t.Path, dir, a.settings.ShowHidden)
	return res.Found && res.Path != "" && !strings.EqualFold(res.Path, t.Path)
}

// viewerFolderStep moves the viewer into the neighbouring folder: the last
// picture going back, the first going forward. The tab follows, so the grid
// and the viewer stay on the same folder.
func (a *App) viewerFolderStep(dir int) {
	t := a.tab()
	if t == nil || t.Path == "" {
		return
	}
	res := (FS{}).Sibling(t.Path, dir, a.settings.ShowHidden)
	if !res.Found || res.Path == "" || strings.EqualFold(res.Path, t.Path) {
		return
	}
	// List the neighbour without the asynchronous tab load: the viewer needs
	// its pictures now, and the tab is switched in the same step.
	listing := (FS{}).Browse(BrowseRequest{
		Path:       res.Path,
		Recursive:  a.settings.Recursive,
		Sort:       t.Sort,
		Desc:       t.Desc,
		ShowHidden: a.settings.ShowHidden,
		ShowOther:  a.settings.ShowOther,
		Limit:      6000,
	})
	if listing.Error != "" || len(listing.Items) == 0 {
		return
	}
	var media []*Item
	for _, item := range listing.Items {
		if isMediaKind(item.Kind) {
			media = append(media, item)
		}
	}
	if len(media) == 0 {
		return
	}
	target := media[0]
	if dir < 0 {
		target = media[len(media)-1]
	}

	// Switch the tab first, so the grid behind the viewer already shows the
	// folder the picture belongs to.
	t.History = append(t.History, t.Path)
	if len(t.History) > 40 {
		t.History = t.History[len(t.History)-40:]
	}
	t.Path = res.Path
	t.Name = displayName(res.Path)
	t.Selected = map[string]bool{}
	t.Anchor = ""
	t.MultiSelect = false
	t.pendingSelect = ""
	t.Scroll.Y = 0
	t.Gen++
	t.Loading = false
	t.Err = ""
	t.Result = listing
	t.All = listing.Items
	t.Items = listing.Items
	a.centeredFolderTree = ""

	idx := -1
	for i, item := range listing.Items {
		if item.ID == target.ID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	a.cancelViewerHold()
	a.viewer.Open = true
	a.viewer.Item = target
	a.viewer.Index = idx
	a.viewer.Zoom = 1
	a.viewer.Angle = 0
	a.viewer.PanX = 0
	a.viewer.PanY = 0
	a.releaseViewerOwnedBitmap()
	a.loadViewerMedia(target)
	a.persist()
	a.refresh()
}

func (a *App) cancelViewerHold() {
	a.viewer.HoldActive = false
	a.viewer.HoldButton = -1
	a.viewer.HoldAt = time.Time{}
}

// dragViewer applies a real pointer movement. MyGo reports held=true for a
// stationary primary press too, so zero movement must not cancel the pending
// long-press actual-size transition. While a peek holds the picture at 100%,
// the same drag pans it, which is what makes the glance usable on a big photo.
func (a *App) dragViewer(dx, dy, w, h float32) {
	if dx == 0 && dy == 0 {
		return
	}
	if a.viewer.Peeking {
		a.viewer.PanX += dx
		a.viewer.PanY += dy
		a.clampViewerPan(w, h)
		a.refresh()
		return
	}
	a.clearViewerPendingActualSize()
	a.viewer.PanX += dx
	a.viewer.PanY += dy
	a.clampViewerPan(w, h)
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
	if a.viewer.Peeking && (canceled || button == a.viewer.PeekButton) {
		// The button that started the peek came up, or the system took it.
		a.endViewerPeek()
	}
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
	mode := a.settings.ViewerHoldZoom
	a.cancelViewerHold()
	if button == 0 {
		switch mode {
		case "off":
			// The gesture is turned off: a long press does nothing.
		case "jump":
			a.viewerActualSizeAt(x, y, w, h)
		default: // peek
			a.beginViewerPeek(x, y, w, h)
		}
	} else if button == 1 {
		switch a.settings.ViewerHoldFit {
		case "off":
			// The gesture is turned off: a long press does nothing.
		case "fit":
			a.viewerFit()
		default: // peek
			a.beginViewerFitPeek()
		}
	}
}

// beginViewerFitPeek fits the whole picture for as long as the button is held,
// saving the view it came from. Releasing restores it, so the gesture is a look
// at the whole frame rather than a change of state.
func (a *App) beginViewerFitPeek() {
	if a.viewer.Item == nil || a.viewer.Item.Kind != "image" {
		return
	}
	a.saveViewerPeekState()
	a.viewer.Peeking = true
	a.viewer.PeekButton = 1
	a.viewer.Zoom = 1
	a.viewer.PanX = 0
	a.viewer.PanY = 0
	a.viewer.OneToOne = false
	a.clearViewerPendingActualSize()
	a.refresh()
}

// beginViewerPeek jumps to 100% for as long as the button is held, saving the
// view it came from. Releasing restores it, so a long press is a glance at the
// pixels rather than a change of state.
func (a *App) beginViewerPeek(x, y, w, h float32) {
	if a.viewer.Bitmap == nil || a.viewer.Item == nil || a.viewer.Item.Kind != "image" {
		return
	}
	a.saveViewerPeekState()
	a.viewer.Peeking = true
	a.viewer.PeekButton = 0
	a.viewerActualSizeAt(x, y, w, h)
}

// saveViewerPeekState records the view a peek starts from, so releasing can put
// it back exactly.
func (a *App) saveViewerPeekState() {
	a.viewer.PeekZoom = a.viewer.Zoom
	a.viewer.PeekPanX = a.viewer.PanX
	a.viewer.PeekPanY = a.viewer.PanY
	a.viewer.PeekAngle = a.viewer.Angle
	a.viewer.PeekOneToOne = a.viewer.OneToOne
}

// endViewerPeek restores the view a peek came from. A press that never reached
// 100% changed nothing, so restoring is harmless either way.
func (a *App) endViewerPeek() {
	if !a.viewer.Peeking {
		return
	}
	a.viewer.Peeking = false
	a.viewer.Zoom = a.viewer.PeekZoom
	a.viewer.PanX = a.viewer.PeekPanX
	a.viewer.PanY = a.viewer.PeekPanY
	a.viewer.Angle = a.viewer.PeekAngle
	a.viewer.OneToOne = a.viewer.PeekOneToOne
	a.clearViewerPendingActualSize()
	a.refresh()
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
				p.Fill(r, ui.Hex("#0a0a0b"), 0)
				if !a.viewer.Player.Ready() {
					msg := "正在载入视频…"
					if err := a.viewer.Player.Err(); err != nil {
						msg = err.Error()
					}
					w, _ := p.MeasureText(0, ui.Span{Text: msg, Size: 13})
					drawInterfaceText(p, r.X+(r.W-w)/2, r.Y+r.H/2, msg, 13, 0, ui.RGBA(230, 230, 235, 255))
				}
			case a.viewer.Bitmap == nil && a.viewer.Placeholder != nil:
				p.Fill(r, ui.Hex("#0a0a0b"), 0)
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
				p.Fill(r, ui.Hex("#0a0a0b"), 0)
				msg := "正在载入…"
				if a.viewer.Err != "" {
					msg = a.viewer.Err
				}
				w, _ := p.MeasureText(0, ui.Span{Text: msg, Size: 13})
				drawInterfaceText(p, r.X+(r.W-w)/2, r.Y+r.H/2, msg, 13, 0, ui.RGBA(230, 230, 235, 255))
			default:
				bw, bh := a.viewer.Bitmap.Size()
				if bw <= 0 || bh <= 0 {
					return
				}
				p.Fill(r, ui.Hex("#0a0a0b"), 0)
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
					refW, refH := a.viewerReferenceSize()
					pct = viewerPercentValue(bw, bh, refW, refH, a.viewer.Angle, a.viewer.Zoom, r.W, r.H)
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
			a.dragViewer(dx, dy, canvasW, canvasH)
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
					a.clearViewerPendingActualSize()
					a.viewer.PanX -= ev.DX
					a.viewer.PanY -= ev.DY
					a.clampViewerPan(canvasW, canvasH)
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
	winW, _ := c.Size()
	ui.Column(c).Width(viewerInfoPanelWidth(winW)).FillHeight().Background(th.Surface).
		BorderStartWidth(1).BorderColor(th.Border.Alpha(.7)).
		Padding(12, 14).Gap(10).Children(func() {
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			components.Icon(c, lucide.Info()).Size(14, 14).TextColor(th.TextMuted)
			components.Text(c, "信息").FontSize(12).FontWeight(600)
			ui.Spacer(c)
			components.IconButton(c, lucide.X(), "隐藏信息 (I)", components.IconButtonPlain).
				OnClick(a.viewerToggleInfo)
		})

		ui.Column(c).Gap(3).Children(func() {
			components.Text(c, it.Name).FontSize(13.5).FontWeight(600).SingleLine().Ellipsis("…").Tooltip(it.Name)
			components.Text(c, it.Path).FontSize(9.5).TextColor(th.TextMuted).SingleLine().Ellipsis("…").Tooltip(it.Path)
		})

		// A glanceable summary replaces the old stack of identical label/value
		// paragraphs. Dimensions and weight are the two facts a viewer reads
		// before opening the disclosure sections below.
		ui.Row(c).Gap(16).Children(func() {
			if it.Kind == "video" {
				_, _, dur := a.viewerVideoSummary()
				infoMetric(c, "时长", formatClock(dur))
				infoMetric(c, "大小", (MediaSvc{}).FormatSize(it.Size))
			} else {
				infoMetric(c, "尺寸", a.viewerImageDimensions(it))
				infoMetric(c, "大小", (MediaSvc{}).FormatSize(it.Size))
			}
		})
		ui.Divider(c)

		ui.Scroll(c).Grow(1).Scrollbars(true).Children(func() {
			ui.Column(c).Gap(11).Children(func() {
				infoSection(c, "文件")
				infoDetailRow(c, "格式", infoFormat(it))
				infoDetailRow(c, "修改时间", (AppSvc{}).FormatTime(it.Mtime))
				if it.Kind == "video" {
					a.viewerVideoInfoDetails(c)
				} else if a.viewer.InfoReady {
					infoSection(c, "图像")
					switch {
					case a.viewer.FullResUnsupported:
						infoDetailRow(c, "清晰度", "2560 预览")
					case a.viewer.FullResReady:
						infoDetailRow(c, "清晰度", "全分辨率")
					case a.viewer.FullResLoading:
						infoDetailRow(c, "清晰度", "正在载入全分辨率…")
					}
					if len(a.viewer.Info.Exif) == 0 {
						components.Text(c, "没有可读取的拍摄参数").FontSize(10).TextColor(th.TextMuted)
					}
					for _, kv := range a.viewer.Info.Exif {
						infoDetailRow(c, kv.K, kv.V)
					}
				} else {
					components.Text(c, "正在读取元数据…").FontSize(10.5).TextColor(th.TextMuted)
				}
			})
		})

		ui.Divider(c)
		ui.Row(c).Gap(6).Children(func() {
			viewerInfoAction(c, th, "打开", lucide.ExternalLink, true, func() {
				_ = (AppSvc{}).OpenExternal(it.Path)
			}).Grow(1)
			viewerInfoAction(c, th, "定位", lucide.FolderOpen, false, func() {
				_ = (AppSvc{}).Reveal(it.Path)
			}).Grow(1)
			viewerInfoAction(c, th, "复制", lucide.Copy, false, func() {
				_ = (AppSvc{}).CopyText(it.Path)
			}).Grow(1)
		})
	})
}

func (a *App) viewerImageDimensions(it *Item) string {
	w, h := it.W, it.H
	if a.viewer.InfoReady && a.viewer.Info.Width > 0 && a.viewer.Info.Height > 0 {
		w, h = a.viewer.Info.Width, a.viewer.Info.Height
	}
	if w <= 0 || h <= 0 {
		return "—"
	}
	return fmt.Sprintf("%d × %d", w, h)
}

func (a *App) viewerVideoSummary() (w, h int, dur time.Duration) {
	p := a.viewer.Player
	meta := a.viewer.Video
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
	return w, h, dur
}

func (a *App) viewerVideoInfoDetails(c *ui.Context) {
	w, h, dur := a.viewerVideoSummary()
	meta := a.viewer.Video
	res := "—"
	if w > 0 && h > 0 {
		res = fmt.Sprintf("%d × %d", w, h)
	}
	infoDetailRow(c, "分辨率", res)
	if !a.viewer.VideoReady {
		components.Text(c, "正在读取视频元数据…").FontSize(10.5).TextColor(c.Theme().TextMuted)
		return
	}
	fps := "—"
	if meta.FPS > 0 {
		fps = fmt.Sprintf("%.1f fps", meta.FPS)
	}
	infoDetailRow(c, "帧率", fps)
	label, bitrate := "实时码率", 0
	if meta.LiveBitrate && a.viewer.Player != nil {
		bitrate = meta.BitrateAtMS(a.viewer.Player.Position().Milliseconds(), 2*time.Second)
	} else {
		label = "平均码率"
		bitrate = meta.BitrateKbps
		if bitrate <= 0 && dur > 0 {
			bitrate = int(math.Round(float64(a.viewer.Item.Size) * 8 / dur.Seconds() / 1000))
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
	infoDetailRow(c, label, value)
}

func infoFormat(it *Item) string {
	ext := stringsUpper(it.Ext)
	if ext == "" {
		return "文件"
	}
	return ext
}

func infoMetric(c *ui.Context, label, value string) {
	ui.Column(c).Grow(1).Gap(2).Children(func() {
		components.Text(c, label).FontSize(9.5).TextColor(c.Theme().TextMuted)
		components.Text(c, value).FontSize(16.5).FontWeight(600).SingleLine().Ellipsis("…")
	})
}

func infoSection(c *ui.Context, label string) {
	components.Text(c, label).FontSize(9.5).FontWeight(600).
		TextColor(c.Theme().TextMuted).Margin(2, 0, 0, 0)
}

// infoDetailRow is a compact key/value row: a short label column and one line
// of value, instead of the old stacked paragraph that made six EXIF fields
// fill an entire panel.
func infoDetailRow(c *ui.Context, key, value string) {
	ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
		components.Text(c, key).FontSize(9.5).TextColor(c.Theme().TextMuted).Width(62).SingleLine()
		components.Text(c, value).FontSize(10.5).TextColor(c.Theme().Text).Grow(1).SingleLine().Ellipsis("…")
	})
}

// viewerInfoAction is a darkroom-native button. Onevra's shared button styles
// resolve against the app's light theme, which makes their labels nearly
// invisible on a #141416 panel; these colors come from the viewer palette.
func viewerInfoAction(c *ui.Context, th *ui.Theme, label string, icon func() *ui.SVG, primary bool, onClick func()) ui.Element {
	b := ui.ButtonBase(c).Label(label).Radius(7).Height(30).Padding(0, 8)
	bg, hover, pressed := th.Surface, th.SurfaceHover, th.SurfacePressed
	fg, border := th.Text, th.Border.Alpha(.65)
	if primary {
		bg, hover, pressed = th.Text, th.Text.Alpha(.88), th.Text.Alpha(.78)
		fg, border = th.Background, th.Text
	}
	if b.Pressed() {
		bg = pressed
	} else if b.Hovered() {
		bg = hover
	}
	b.Background(bg).TextColor(fg).Border(1, border)
	b.Children(func() {
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			components.Icon(c, icon).Size(13, 13).TextColor(fg)
			components.Text(c, label).FontSize(10.5).FontWeight(600).TextColor(fg).SingleLine()
		})
	})
	if b.Clicked() {
		onClick()
	}
	return b
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

// viewerQualityLabel names what the viewer is showing right now: the grid's
// thumbnail while the real picture decodes, the 2560 preview, or the original.
// Without it a soft picture looked the same whether it was a slow decode or a
// small source, and there was no way to tell whether pressing 1 would help.
func (a *App) viewerQualityLabel() (text, tip string) {
	if a.viewer.Item == nil || a.viewer.Item.Kind != "image" {
		return "", ""
	}
	switch {
	case a.viewer.FullResReady:
		return "原图", "正在显示原始分辨率"
	case a.viewer.Bitmap == nil:
		return "缩略图", "正在显示网格缩略图，大图解码中"
	case a.viewer.FullResLoading:
		return "大图", "正在显示 2560 预览，原图载入中"
	default:
		return "大图", "正在显示 2560 预览；按 1 载入原图"
	}
}

// viewerQualityBadge draws the quality label in the footer, quiet enough not to
// pull the eye from the picture but present on every frame.
func (a *App) viewerQualityBadge(c *ui.Context, th *ui.Theme) {
	text, tip := a.viewerQualityLabel()
	if text == "" {
		return
	}
	// The original is the state the user wants, so it stays quiet; the two
	// interim states are the ones worth noticing.
	color := th.TextMuted.Alpha(.75)
	if !a.viewer.FullResReady {
		color = th.Text.Alpha(.85)
	}
	components.Text(c, text).FontSize(10).FontWeight(600).TextColor(color).
		Padding(2, 6).Radius(4).Background(th.Text.Alpha(.05)).Shrink(0).
		Tooltip(tip)
}
