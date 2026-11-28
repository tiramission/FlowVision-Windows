package main

import "github.com/egoist/mygo/ui"

// Layout and shape tokens shared by the gallery and viewer. Colors stay in the
// two palettes below; these dimensions keep the same tile language everywhere.
const (
	tileRadius      float32 = 9
	thumbnailRadius float32 = 8
	pillRadius      float32 = 999

	compactSidebarThreshold float32 = 960
	compactSidebarMax       float32 = 220
	compactSidebarWorkbench float32 = 640

	viewerInfoPanelDefault      float32 = 300
	viewerInfoPanelMin          float32 = 248
	viewerInfoPanelCompactRatio         = 0.28
	viewerInfoPanelCompactFrom  float32 = 1000
)

// FlowVision's visual refresh keeps Onevra's component system but separates
// two viewing contexts: a warm gallery for browsing and a neutral darkroom for
// the viewer. The palette intentionally spends accent color only on selection,
// primary actions and live state; surfaces stay quiet so photographs lead.

func galleryPalette(dark bool) *ui.Theme {
	if dark {
		t := ui.DarkTheme()
		t.Dark = true
		accent := ui.Hex("#e08a63")
		black := ui.RGB(0, 0, 0)
		white := ui.RGB(255, 255, 255)
		t.Background = ui.Hex("#141311")
		t.Surface = ui.Hex("#1d1b18")
		t.SurfaceHover = ui.Hex("#28251f")
		t.SurfacePressed = ui.Hex("#322e27")
		t.Border = ui.Hex("#3a352d")
		t.Text = ui.Hex("#ece8df")
		t.TextMuted = ui.Hex("#a8a196")
		t.Accent = accent
		t.AccentHover = accent.Mix(white, 0.12)
		t.AccentPressed = accent.Mix(black, 0.12)
		t.AccentText = ui.Hex("#26160f")
		t.Danger = ui.Hex("#e58f95")
		t.Warning = ui.Hex("#e6b567")
		t.Success = ui.Hex("#79bea1")
		t.Selection = accent.Alpha(0.16)
		// Keyboard focus is a real control state, so the ring uses the accent
		// itself instead of a 30% wash that measured below 3:1 on the warm
		// surface. High contrast later replaces it with the strongest text tone.
		t.Focus = accent
		t.Scrollbar = t.Text.Alpha(.30)
		t.Radius = 10
		return t
	}

	t := ui.LightTheme()
	accent := ui.Hex("#b54a29")
	black := ui.RGB(0, 0, 0)
	white := ui.RGB(255, 255, 255)
	t.Background = ui.Hex("#f3f0e9")
	t.Surface = ui.Hex("#faf8f3")
	t.SurfaceHover = ui.Hex("#ece8df")
	t.SurfacePressed = ui.Hex("#e0dbcf")
	t.Border = ui.Hex("#ddd8cd")
	t.Text = ui.Hex("#1f1e1a")
	t.TextMuted = ui.Hex("#6e6960")
	t.Accent = accent
	t.AccentHover = accent.Mix(black, 0.10)
	t.AccentPressed = accent.Mix(black, 0.22)
	t.AccentText = white
	t.Danger = ui.Hex("#b4444c")
	t.Warning = ui.Hex("#a9600b")
	t.Success = ui.Hex("#257a57")
	t.Selection = accent.Alpha(0.14)
	// A neutral focus ring keeps forms calm; the accent is reserved for the
	// selected state and primary actions. Solid accent reaches 4.5:1 or better
	// on the gallery surface, above the 3:1 focus requirement.
	t.Focus = accent
	t.Scrollbar = t.Text.Alpha(.26)
	t.Radius = 10
	return t
}

// viewerPalette is always neutral dark, even when the gallery follows a light
// desktop. A warm charcoal around photographs changes their apparent color;
// the viewer therefore removes color from its chrome instead of tinting it.
func viewerPalette() *ui.Theme {
	t := ui.DarkTheme()
	t.Dark = true
	accent := ui.Hex("#d97757")
	white := ui.RGB(255, 255, 255)
	black := ui.RGB(0, 0, 0)
	t.Background = ui.Hex("#0a0a0b")
	t.Surface = ui.Hex("#141416")
	t.SurfaceHover = ui.Hex("#1d1d20")
	t.SurfacePressed = ui.Hex("#26262a")
	t.Border = ui.Hex("#2c2c31")
	t.Text = ui.Hex("#f2f2f3")
	t.TextMuted = ui.Hex("#9b9ba2")
	t.Accent = accent
	t.AccentHover = accent.Mix(white, 0.14)
	t.AccentPressed = accent.Mix(black, 0.10)
	t.AccentText = ui.Hex("#1a0e09")
	t.Danger = ui.Hex("#f08a8a")
	t.Warning = ui.Hex("#e8b567")
	t.Success = ui.Hex("#74c69d")
	t.Selection = accent.Alpha(0.20)
	// The darkroom controls need the same visible keyboard state as the gallery;
	// the accent ring has ample contrast against the neutral black chrome.
	t.Focus = accent
	t.Scrollbar = t.Text.Alpha(.28)
	t.Radius = 10
	return t
}

// applyThemePreferences keeps the app's existing accessibility behavior after
// a custom palette replaces Onevra's defaults.
func applyThemePreferences(t *ui.Theme, c *ui.Context) {
	// The interface font is an appearance preference like the palette, so it
	// applies to the gallery and the viewer chrome alike.
	if family := interfaceFont(); family != "" {
		t.Font = family
	}
	p := c.Preferences()
	if p.HighContrast {
		t.Border = t.Border.Mix(t.Text, 0.45)
		t.TextMuted = t.TextMuted.Mix(t.Text, 0.4)
		t.Focus = t.Text
		t.Scrollbar = t.Scrollbar.Alpha(1.6)
	}
	if s := p.TextScale; s > 0 && s != 1 {
		t.FontSize *= s
	}
}

// drawInterfaceText paints one line of custom-drawn text in the interface
// font. Painter.Text lays text out with the platform's default family, which
// left the grid captions and the viewer HUD in the system font while every
// component already used the theme's family.
func drawInterfaceText(p *ui.Painter, x, y float32, s string, size float32, weight int, c ui.Color) {
	p.RichText(x, y, 0, ui.Span{Text: s, Size: size, Weight: weight, Color: c})
}

// inkRiseAt is how far above the baseline a label's ink centres, as a share of
// the em, per interface text size. It is read from the renderer, not derived:
// the font's ascent and descent describe the em box, whose band is much taller
// than the glyphs, and where the ink lands inside that band shifts with the
// pixel grid as the size changes — a single ratio was 2 px off at one size and
// right at another. Entries come from TestMeasureInkRise, which prints them for
// the shipped font and the platform's; sizes not listed interpolate.
var inkRiseAt = []struct{ size, rise float32 }{
	{9, -0.16},
	{10, -0.01},
	{10.5, 0.08},
	{11, 0.13},
	{12, 0.25},
	{13, 0.34},
}

// inkRise interpolates the measured ratio for a size.
func inkRise(size float32) float32 {
	table := inkRiseAt
	if size <= table[0].size {
		return table[0].rise
	}
	for i := 1; i < len(table); i++ {
		if size <= table[i].size {
			lo, hi := table[i-1], table[i]
			t := (size - lo.size) / (hi.size - lo.size)
			return lo.rise + t*(hi.rise-lo.rise)
		}
	}
	return table[len(table)-1].rise
}

// drawCenteredText paints one line of text centred on a box's vertical middle
// by its ink rather than its line box. The line box carries the font's full
// ascent and descent, while a short string fills only a band inside it, so
// placing the line box at the middle leaves the ink off centre — by a few
// pixels once the interface font is pinned to a CJK-first family, which is why
// labels in pills sat away from the middle.
func drawCenteredText(p *ui.Painter, box ui.Rect, x float32, s string, size float32, weight int, c ui.Color) {
	p.RichText(x, box.Y+textCentreOffset(box.H, s, size, weight), 0,
		ui.Span{Text: s, Size: size, Weight: weight, Color: c})
}

// textCentreOffset is how far below a box's top a line of text must start so
// that its ink sits on the box's middle. boxH is the height to centre in — the
// pill's, not the text's own line box, which is shorter and would centre the
// text on itself instead of on the pill.
func textCentreOffset(boxH float32, s string, size float32, weight int) float32 {
	if size <= 0 || s == "" {
		return boxH / 2
	}
	line, ok := firstLineOf(s, size, weight)
	if !ok {
		return boxH / 2
	}
	// The layout gives the baseline; the ink's middle sits a little above it.
	inkCentre := line.Baseline - size*inkRise(size)
	return boxH/2 - inkCentre
}

// firstLineOf lays out s as the interface draws it and returns its first line,
// which carries the baseline the ink is measured from.
func firstLineOf(s string, size float32, weight int) (ui.TextLine, bool) {
	l := ui.ShapeRichTextWith(
		[]ui.Span{{Text: s, Size: size, Weight: weight}},
		ui.Font{Size: size, Weight: weight},
		ui.TextShape{},
	)
	if l == nil {
		return ui.TextLine{}, false
	}
	lines := l.Lines()
	if len(lines) == 0 {
		return ui.TextLine{}, false
	}
	return lines[0], true
}

// ellipsizeText shortens s with an ellipsis until it fits width at the given
// size, so a label can be drawn inside a box without running past its edge.
// It returns s unchanged when it already fits, and keeps at least one character
// when even that does not.
func ellipsizeText(p *ui.Painter, s string, size float32, weight int, width float32) string {
	if width <= 0 || s == "" {
		return ""
	}
	measure := func(t string) float32 {
		w, _ := p.MeasureText(0, ui.Span{Text: t, Size: size, Weight: weight})
		return w
	}
	if measure(s) <= width {
		return s
	}
	const ellipsis = "…"
	runes := []rune(s)
	// Binary search the longest prefix that fits with the ellipsis appended.
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if measure(string(runes[:mid])+ellipsis) <= width {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo <= 0 {
		return ellipsis
	}
	return string(runes[:lo]) + ellipsis
}
