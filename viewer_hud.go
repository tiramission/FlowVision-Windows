package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const viewerNoticeLoadDelay = 500 * time.Millisecond

type viewerNoticeKind int

const (
	viewerNoticeInfo viewerNoticeKind = iota
	viewerNoticeLimit
	viewerNoticeWarn
	viewerNoticeError
)

type viewerNotice struct {
	Text  string
	Kind  viewerNoticeKind
	Until time.Time
}

// viewerHUDMode resolves the environment override first, then the saved
// setting. "debug" is an environment-only diagnostic mode.
func (a *App) viewerHUDMode() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FLOWVISION_VIEWER_HUD"))) {
	case "off":
		return "off"
	case "auto":
		return "auto"
	case "always":
		return "always"
	case "debug":
		return "debug"
	}
	switch a.settings.ViewerHUD {
	case "off", "auto", "always":
		return a.settings.ViewerHUD
	}
	return "auto"
}

func (a *App) viewerHUDVisible() bool {
	if !a.viewer.Open {
		return false
	}
	mode := a.viewerHUDMode()
	if mode == "off" {
		return false
	}
	if !a.viewer.Notice.Until.IsZero() && time.Now().Before(a.viewer.Notice.Until) {
		return true
	}
	if a.viewer.Notice.Text != "" && a.viewer.Notice.Until.IsZero() {
		return true
	}
	return mode == "always" || mode == "debug"
}

func (a *App) notifyViewer(key, text string, kind viewerNoticeKind, ttl time.Duration) {
	if !a.viewer.Open || text == "" {
		return
	}
	now := time.Now()
	if key == "zoom" && !a.viewer.NoticeAt.IsZero() && now.Sub(a.viewer.NoticeAt) < 80*time.Millisecond {
		return
	}
	a.viewer.Notice = viewerNotice{Text: text, Kind: kind, Until: now.Add(ttl)}
	if ttl <= 0 {
		a.viewer.Notice.Until = time.Time{}
	}
	a.viewer.NoticeKey = key
	a.viewer.NoticeAt = now
	if a.viewerNoticeTimer != nil {
		a.viewerNoticeTimer.Stop()
	}
	if ttl > 0 {
		a.viewerNoticeTimer = time.AfterFunc(ttl, func() {
			a.update(func() {
				a.refresh()
			})
		})
	}
	a.refresh()
}

func (a *App) notifyViewerError(text string) {
	a.notifyViewer("error", text, viewerNoticeError, 4*time.Second)
}

func (a *App) startViewerSlowNotice(gen int, text string, delay time.Duration) {
	a.viewerSlowTimer = time.AfterFunc(delay, func() {
		a.update(func() {
			if a.viewer.Gen != gen || !a.viewer.Loading {
				return
			}
			a.notifyViewer("load", text, viewerNoticeInfo, 0)
		})
	})
}

func (a *App) stopViewerLoadTimers() {
	if a.viewerSlowTimer != nil {
		a.viewerSlowTimer.Stop()
		a.viewerSlowTimer = nil
	}
	if a.viewerNoticeTimer != nil {
		a.viewerNoticeTimer.Stop()
		a.viewerNoticeTimer = nil
	}
}

func (a *App) viewerPreloadBytesValue() int64 {
	a.viewerPreloadMu.Lock()
	defer a.viewerPreloadMu.Unlock()
	return a.viewerPreloadBytes
}

func (a *App) drawViewerHUD(p *ui.Painter, r ui.Rect, th *ui.Theme, percent int) {
	mode := a.viewerHUDMode()
	if mode == "off" || !a.viewer.Open {
		return
	}
	now := time.Now()
	if mode == "debug" {
		a.drawViewerDebugHUD(p, r, th)
	}
	hasNotice := a.viewer.Notice.Text != "" && (a.viewer.Notice.Until.IsZero() || a.viewer.Notice.Until.After(now))
	if !hasNotice {
		if mode != "always" && mode != "debug" {
			return
		}
		if percent >= 0 {
			a.drawViewerStatusPill(p, r, th, fmt.Sprintf("%d%%", percent), viewerNoticeInfo, 1)
		}
		if mode == "debug" {
			p.After(500 * time.Millisecond)
		}
		return
	}
	remaining := time.Until(a.viewer.Notice.Until)
	if a.viewer.Notice.Until.IsZero() {
		remaining = time.Hour
	}
	alpha := float32(1)
	if remaining < 200*time.Millisecond {
		alpha = float32(remaining) / float32(200*time.Millisecond)
		if alpha < 0 {
			alpha = 0
		}
	}
	a.drawViewerStatusPill(p, r, th, a.viewer.Notice.Text, a.viewer.Notice.Kind, alpha)
	p.After(80 * time.Millisecond)
}

func (a *App) drawViewerStatusPill(p *ui.Painter, r ui.Rect, th *ui.Theme, text string, kind viewerNoticeKind, alpha float32) {
	size := float32(11)
	pad := float32(13)
	// The pill takes at most this share of the viewer, so a long message is
	// shortened to fit rather than measured at its full length and clipped.
	maxW := r.W - 32
	if r.W > 0 {
		if limit := r.W * 0.62; limit < maxW {
			maxW = limit
		}
	}
	text = ellipsizeText(p, text, size, 500, maxW-pad*2)
	w, _ := p.MeasureText(0, ui.Span{Text: text, Size: size, Weight: 500})
	pillW := min(w+pad*2, max(0, maxW))
	h := float32(28)
	y := r.Y + r.H - 20 - h
	if a.viewer.Immersive && a.viewer.ChromeVisible {
		y = r.Y + r.H - 64 - h
	}
	x := r.X + (r.W-pillW)/2
	pill := ui.Rect{X: x, Y: y, W: pillW, H: h}
	bg := th.Surface.Alpha(.88 * alpha)
	fg := th.Text.Alpha(alpha)
	switch kind {
	case viewerNoticeWarn:
		fg = th.Warning.Alpha(alpha)
	case viewerNoticeError:
		fg = th.Danger.Alpha(alpha)
	case viewerNoticeLimit:
		fg = th.Warning.Alpha(alpha)
	}
	p.Fill(pill, bg, 10)
	p.Stroke(pill, th.Border.Alpha(.72*alpha), 10, 1)
	drawCenteredText(p, pill, x+pad, text, size, 500, fg)
}

func (a *App) drawViewerDebugHUD(p *ui.Painter, r ui.Rect, th *ui.Theme) {
	total := 0
	if t := a.tab(); t != nil {
		total = len(t.Items)
	}
	lines := []string{
		fmt.Sprintf("load %dms  cache %v  preload %v", a.viewerStats.LoadMs, a.viewerStats.CacheHit, a.viewerStats.PreloadHit),
		fmt.Sprintf("decode %dms  scale %dms  encode %dms  bitmap %dms", a.viewerStats.DecodeMs, a.viewerStats.ScaleMs, a.viewerStats.EncodeMs, a.viewerStats.BitmapMs),
		fmt.Sprintf("preload %d/%dMB  zoom %.0f%%  %d/%d", a.viewerPreloadBytesValue()>>20, viewerPreloadBudget>>20, a.viewer.Zoom*100, a.viewer.Index+1, total),
	}
	if a.viewer.FullResReady || a.viewer.FullResLoading || a.viewer.FullResUnsupported || a.viewerPendingActual {
		lines = append(lines, fmt.Sprintf("fullres ready=%v loading=%v unsupported=%v pending=%v 1:1=%v", a.viewer.FullResReady, a.viewer.FullResLoading, a.viewer.FullResUnsupported, a.viewerPendingActual, a.viewer.OneToOne))
	}
	x := r.X + r.W - 300
	if x < r.X+12 {
		x = r.X + 12
	}
	y := r.Y + 10
	p.Fill(ui.Rect{X: x - 8, Y: y - 4, W: 296, H: float32(len(lines)*15 + 8)}, ui.RGBA(8, 8, 12, 0.62), 8)
	for i, line := range lines {
		drawInterfaceText(p, x, y+float32(i)*15+4, line, 9, 0, ui.RGBA(210, 220, 235, 0.9))
	}
}
