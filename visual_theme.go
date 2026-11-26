package main

import "github.com/egoist/mygo/ui"

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
		// Focus stays visible without turning every focused field into an
		// orange-bordered card. High contrast restores a stronger ring.
		t.Focus = t.Text.Alpha(0.30)
		t.Scrollbar = t.Text.Alpha(.30)
		t.Radius = 10
		return t
	}

	t := ui.LightTheme()
	accent := ui.Hex("#bf5330")
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
	// selected state and primary actions. High contrast strengthens it later.
	t.Focus = t.Text.Alpha(0.30)
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
	t.Focus = accent.Alpha(0.72)
	t.Scrollbar = t.Text.Alpha(.28)
	t.Radius = 10
	return t
}

// applyThemePreferences keeps the app's existing accessibility behavior after
// a custom palette replaces Onevra's defaults.
func applyThemePreferences(t *ui.Theme, c *ui.Context) {
	p := c.Preferences()
	if p.HighContrast {
		t.Border = t.Border.Mix(t.Text, 0.45)
		t.TextMuted = t.TextMuted.Mix(t.Text, 0.4)
		t.Focus.A = 255
		t.Scrollbar = t.Scrollbar.Alpha(1.6)
	}
	if s := p.TextScale; s > 0 && s != 1 {
		t.FontSize *= s
	}
}
