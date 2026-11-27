package main

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// gridCell is one tile of the waterfall. X/Y are in the content box's
// coordinates; scrolling moves the viewport, not the cells.
type gridCell struct {
	It    *Item
	X, Y  float32
	W     float32
	ImgH  float32
	CardH float32
}

func (c gridCell) height() float32 { return c.CardH }

type gridLayout struct {
	Cells    []gridCell
	TotalH   float32
	Width    float32
	Cols     int
	Gap      float32
	Captions bool
}

// buildWaterfall distributes items into the shortest column, using the image
// headers' aspect ratios. Columns share the width equally, as Photos' grid
// does; a row of short panoramas therefore leaves the next row higher up.
// Captions controls whether each tile leaves room for a file name below its
// picture; hiding them packs the cards together for an edge-to-edge wall.
func buildWaterfall(items []*Item, width float32, target, gap int, captions bool) gridLayout {
	if width < 80 {
		width = 80
	}
	if target < 48 {
		target = 48
	}
	g := float32(gap)
	cols := int((width + g) / (float32(target) + g))
	if cols < 1 {
		cols = 1
	}
	if cols > 12 {
		cols = 12
	}
	colW := (width - g*float32(cols-1)) / float32(cols)
	if colW < 40 {
		colW = 40
	}
	captionH := float32(2)
	if captions {
		captionH = 33
	}
	colY := make([]float32, cols)
	out := gridLayout{Width: width, Cols: cols, Gap: g, Captions: captions}
	out.Cells = make([]gridCell, 0, len(items))
	for _, it := range items {
		aspect := itemAspect(it)
		imgH := colW / aspect
		if imgH < 48 {
			imgH = 48
		}
		if it.Kind == "folder" || it.Kind == "other" {
			// Folders and generic files have no thumbnail to show. Letting them
			// take a full-width square turns a single-column narrow window into
			// one giant placeholder card, so keep their preview at the current
			// thumbnail scale instead.
			limit := float32(target)
			if it.Kind == "other" {
				limit *= 0.68
			}
			if imgH > limit {
				imgH = limit
			}
		}
		if imgH > colW*3.2 {
			imgH = colW * 3.2
		}
		cellCaptionH := captionH
		if it.Kind == "folder" {
			// A folder has no thumbnail; its name is the only way to tell
			// two folders apart, so keep that label even in immersive mode.
			cellCaptionH = 33
		}
		col := 0
		for i := 1; i < cols; i++ {
			if colY[i] < colY[col]-0.5 {
				col = i
			}
		}
		cell := gridCell{
			It:    it,
			X:     float32(col) * (colW + g),
			Y:     colY[col],
			W:     colW,
			ImgH:  imgH,
			CardH: imgH + cellCaptionH,
		}
		out.Cells = append(out.Cells, cell)
		colY[col] += cell.height() + g
	}
	for _, y := range colY {
		if y > out.TotalH {
			out.TotalH = y
		}
	}
	if out.TotalH > 0 {
		out.TotalH -= g
	}
	return out
}

func itemAspect(it *Item) float32 {
	if it.W > 0 && it.H > 0 {
		a := float32(it.W) / float32(it.H)
		if a < 0.2 {
			return 0.2
		}
		if a > 5 {
			return 5
		}
		return a
	}
	if it.Kind == "video" {
		return 16.0 / 9.0
	}
	return 1
}

func (g gridLayout) hit(x, y float32) (int, bool) {
	for i, c := range g.Cells {
		if x >= c.X && x <= c.X+c.W && y >= c.Y && y <= c.Y+c.height() {
			return i, true
		}
	}
	return 0, false
}

func (g gridLayout) center(i int) (float32, float32) {
	if i < 0 || i >= len(g.Cells) {
		return 0, 0
	}
	c := g.Cells[i]
	return c.X + c.W/2, c.Y + c.height()/2
}

func (g gridLayout) nearestFrom(i int, dx, dy float32) int {
	if len(g.Cells) == 0 {
		return -1
	}
	if i < 0 || i >= len(g.Cells) {
		return 0
	}
	cx, cy := g.center(i)
	best := -1
	bestScore := float32(math.MaxFloat32)
	for j, c := range g.Cells {
		if j == i {
			continue
		}
		x, y := c.X+c.W/2, c.Y+c.height()/2
		along := (x-cx)*dx + (y-cy)*dy
		if along <= 4 {
			continue
		}
		cross := float32(math.Abs(float64((x-cx)*dy - (y-cy)*dx)))
		score := along + cross*1.8
		if score < bestScore {
			bestScore = score
			best = j
		}
	}
	return best
}

func (a *App) gridView(c *ui.Context, tab *Tab) {
	th := c.Theme()
	windowW, windowH := c.Size()
	width := windowW
	if a.sidebar {
		width -= a.sidebarLayoutWidth(windowW)
	}
	width -= 32
	if width < 120 {
		width = 120
	}
	layout := buildWaterfall(tab.Items, width, a.settings.ThumbSize, a.settings.GridGap, a.settings.ShowCardNames)
	viewTop := tab.Scroll.Y
	viewBottom := viewTop + windowH
	viewCenter := viewTop + windowH/2
	a.thumbFrame++
	frame := a.thumbFrame
	for _, cell := range layout.Cells {
		if cell.Y+cell.height() < viewTop-160 || cell.Y > viewBottom+160 {
			continue
		}
		if cell.It.Kind == "image" || cell.It.Kind == "video" {
			cardCenter := cell.Y + cell.height()/2
			a.ensureThumb(cell.It, a.settings.ThumbSize, math.Abs(float64(cardCenter-viewCenter)), frame)
		}
	}
	a.endThumbFrame(frame)
	a.pumpThumbPrefetch(tab)

	scroll := ui.Scroll(c).Fill().TrackScroll(&tab.Scroll).Padding(16).Gap(0).Key("media-grid-scroll")
	scroll.Children(func() {
		// Keep the hit-test box at least as tall as the viewport so a
		// secondary click below the last row still reaches the grid's
		// context menu instead of falling through to the scroll surface.
		contentH := layout.TotalH
		if minH := windowH - titleBarHeight - toolbarHeight - bottomBarHeight - 32; contentH < minH {
			contentH = minH
		}
		box := ui.Box(c).Key("media-grid").Height(contentH).Focusable().Role(ui.RoleList).
			Label(fmt.Sprintf("%s，%d 项", tab.Name, len(tab.Items)))
		box.Bind(&a.gridFocusHandle)
		if a.gridMenuRefocus {
			a.gridMenuRefocus = false
			box.Focus()
		}
		if box.FileDragOver() {
			box.Border(2, th.Accent)
		}
		if files := box.DroppedFiles(); len(files) > 0 {
			first := files[0]
			a.later(func() { a.openPathArgument(first) })
		}

		box.Draw(func(p *ui.Painter, r ui.Rect) {
			hover := -1
			if x, y, over := box.PointerPosition(); over {
				if idx, ok := layout.hit(x, y); ok {
					hover = idx
				}
			}
			a.drawGrid(p, r, layout, th, hover)
			if box.FileDragOver() {
				p.Fill(r, th.Selection, 10)
			}
		})

		// The pointer is in the content box's coordinates, which include the
		// scroll offset, so hit testing needs no adjustment.
		if box.Clicked() {
			a.gridMenuOpen = false
			x, y, _ := box.PointerPosition()
			if idx, ok := layout.hit(x, y); ok {
				it := layout.Cells[idx].It
				mods := box.ClickModifiers()
				switch {
				case mods&ui.Shift != 0 && tab.Anchor != "":
					// Shift extends from the anchor, and doing so is itself a
					// multi-selection: enter the mode rather than making the
					// user press Ctrl+M first.
					tab.MultiSelect = true
					a.selectRange(it.ID)
				case mods&ui.Cmd != 0:
					// Ctrl+click builds a selection one tile at a time, which is
					// the other way into multi-select.
					tab.MultiSelect = true
					a.toggleSelect(it.ID, true)
				case tab.MultiSelect:
					// Already in the mode: a plain click toggles too, so a run
					// of photos is one click each.
					a.toggleSelect(it.ID, true)
				default:
					a.selectOnly(it.ID)
				}
			}
		}
		if box.DoubleClicked() {
			x, y, _ := box.PointerPosition()
			if idx, ok := layout.hit(x, y); ok {
				it := layout.Cells[idx].It
				a.later(func() { a.activateItem(it) })
			}
		}
		if box.RightClicked() {
			x, y, _ := box.PointerPosition()
			a.resolveGridContextAt(tab, layout, x, y)
			b := box.Bounds()
			a.openGridMenu(b.X+x, b.Y+y, false)
		}

		openKeyboardMenu := func() {
			a.resolveGridContextSelection(tab)
			b := box.Bounds()
			ax, ay := a.gridMenuKeyboardAnchor(tab, layout, b)
			a.openGridMenu(ax, ay, true)
		}
		box.OnShortcut(0, ui.KeyContextMenu, openKeyboardMenu)
		box.OnShortcut(ui.Shift, ui.KeyF10, openKeyboardMenu)

		box.OnShortcut(0, ui.KeyEnter, func() {
			if it := tab.selectedOne(); it != nil {
				a.later(func() { a.activateItem(it) })
			}
		})
		box.OnShortcut(0, ui.KeyDelete, func() {
			if a.settings.ShowOther || len(tab.Selected) > 0 {
				a.later(func() { a.askDelete(false) })
			}
		})
		box.OnShortcut(ui.Cmd, ui.KeyA, func() { a.later(a.selectAll) })
		// Multi-select has its own mode, so it also has its own key: Ctrl+M
		// enters, Escape leaves and clears.
		box.OnShortcut(ui.Cmd, ui.KeyM, func() {
			a.later(func() {
				if tab.MultiSelect {
					a.exitMultiSelect()
					return
				}
				a.setMultiSelect(true)
			})
		})
		box.OnShortcut(0, ui.KeyEscape, func() { a.later(a.clearSelection) })
		// Space stars the selected tile, matching the viewer's shortcut. It
		// used to open the item, which duplicated Enter and left no keyboard
		// way to favorite from the grid.
		box.OnShortcut(0, ui.KeySpace, func() {
			if tab.selectedOne() != nil {
				a.later(a.toggleItemFavorites)
			}
		})
		box.OnShortcut(0, ui.KeyF2, func() {
			if tab.selectedOne() != nil {
				a.later(a.beginRename)
			}
		})
		box.OnShortcut(ui.Shift, ui.KeyLeft, func() {
			a.extendGridSelection(tab, layout, -1, 0)
		})
		box.OnShortcut(ui.Shift, ui.KeyRight, func() {
			a.extendGridSelection(tab, layout, 1, 0)
		})
		box.OnShortcut(ui.Shift, ui.KeyUp, func() {
			a.extendGridSelection(tab, layout, 0, -1)
		})
		box.OnShortcut(ui.Shift, ui.KeyDown, func() {
			a.extendGridSelection(tab, layout, 0, 1)
		})
		box.OnShortcut(0, ui.KeyLeft, func() {
			a.moveGridSelection(tab, layout, -1, 0)
		})
		box.OnShortcut(0, ui.KeyRight, func() {
			a.moveGridSelection(tab, layout, 1, 0)
		})
		box.OnShortcut(0, ui.KeyUp, func() {
			a.moveGridSelection(tab, layout, 0, -1)
		})
		box.OnShortcut(0, ui.KeyDown, func() {
			a.moveGridSelection(tab, layout, 0, 1)
		})
		box.OnShortcut(0, ui.KeyHome, func() {
			a.selectGridIndex(tab, layout, 0)
		})
		box.OnShortcut(0, ui.KeyEnd, func() {
			a.selectGridIndex(tab, layout, len(layout.Cells)-1)
		})
	})
}

func tabIndex(tabs []*Tab, want *Tab) int {
	for i, t := range tabs {
		if t == want {
			return i
		}
	}
	return -1
}

func (a *App) contextOne() *Item {
	if len(a.contextIDs) != 1 {
		return nil
	}
	id := a.contextIDs[0]
	if t := a.tab(); t != nil {
		if idx := t.indexOf(id); idx >= 0 {
			return t.Items[idx]
		}
	}
	it, _ := items.Get(id)
	return it
}

func (a *App) drawGrid(p *ui.Painter, r ui.Rect, layout gridLayout, th *ui.Theme, hover int) {
	for i, cell := range layout.Cells {
		it := cell.It
		card := ui.Rect{X: r.X + cell.X, Y: r.Y + cell.Y, W: cell.W, H: cell.height()}
		imgR := ui.Rect{X: card.X + 1, Y: card.Y + 1, W: card.W - 2, H: cell.ImgH}
		selected := false
		if t := a.tab(); t != nil {
			selected = t.Selected[it.ID]
		}
		// Photographs are the tile. Only folders and file placeholders need a
		// card face; images/videos sit directly on the gallery canvas so the
		// eye reads the picture, not a white rectangle around it.
		imageTile := it.Kind == "image" || it.Kind == "video"
		cardFace := th.Surface
		cardBorder := ui.Color{}
		borderWidth := float32(0)
		if selected {
			cardBorder = th.Accent.Alpha(.92)
			borderWidth = 2
		}
		if i == hover {
			if imageTile {
				p.Shadow(imgR, 14, 0, 4, 16, 0, ui.RGBA(0, 0, 0, .13))
			} else {
				cardFace = th.SurfaceHover
				p.Shadow(card, 10, 0, 3, 10, 0, ui.RGBA(0, 0, 0, .10))
			}
			if !selected {
				cardBorder = th.Accent.Alpha(.35)
				borderWidth = 1
			}
		}
		if !imageTile {
			p.Fill(card, cardFace, tileRadius)
		}

		if it.Kind == "folder" {
			// A folder is a quiet tile in the same visual family as a photo:
			// one outer surface, one inset preview, and the caption underneath.
			// The old card border made it read as a card inside a card; the inset
			// preview is what gives the folder its file-like weight.
			inner := th.SurfaceHover
			if i == hover {
				inner = th.Surface
			}
			p.Fill(imgR, inner, tileRadius)
			side := float32(44)
			iconY := imgR.Y + (imgR.H-side)/2 - 8
			p.Icon(lucide.Folder(), ui.Rect{X: imgR.X + (imgR.W-side)/2, Y: iconY, W: side, H: side}, th.Accent.Alpha(.86))
			label := "文件夹"
			w, _ := p.MeasureText(0, ui.Span{Text: label, Size: 10.5, Weight: 500})
			drawInterfaceText(p, imgR.X+(imgR.W-w)/2, iconY+side+4, label, 10.5, 500, th.TextMuted.Alpha(.9))
		} else if it.Kind == "image" || it.Kind == "video" {
			bmp := a.thumb(it, a.settings.ThumbSize)
			if bmp != nil {
				p.Clip(imgR, thumbnailRadius, func() {
					p.Image(bmp, imgR, ui.Cover)
				})
			} else {
				p.Fill(imgR, th.SurfaceHover, thumbnailRadius)
				icon := lucide.Image()
				if it.Kind == "video" {
					icon = lucide.Film()
				}
				side := float32(30)
				p.Icon(icon, ui.Rect{X: imgR.X + (imgR.W-side)/2, Y: imgR.Y + (imgR.H-side)/2, W: side, H: side}, th.TextMuted.Alpha(.78))
			}
			if it.Kind == "video" {
				// A play mark alone: it says "video" without a word, and its
				// geometry is arithmetic, so no font or script can knock it off
				// centre. A label beside it had to be nudged by a factor
				// measured from the interface font, which broke as soon as the
				// user chose another one.
				g := videoBadgeGeometry(ui.Rect{X: imgR.X + 8, Y: imgR.Y + 8})
				p.Fill(g.Pill, ui.RGBA(0, 0, 0, 150), g.Pill.H/2)
				p.Icon(lucide.Play(), g.Icon, ui.RGBA(255, 255, 255, 240))
			}
		} else {
			p.Fill(imgR, th.SurfaceHover, tileRadius)
			ext := strings.TrimPrefix(strings.ToUpper(filepath.Ext(it.Name)), ".")
			if ext == "" {
				ext = "文件"
			}
			icon := lucide.File()
			side := float32(30)
			p.Icon(icon, ui.Rect{X: imgR.X + (imgR.W-side)/2, Y: imgR.Y + (imgR.H-side)/2 - 8, W: side, H: side}, th.TextMuted)
			w, _ := p.MeasureText(0, ui.Span{Text: ext, Size: 12, Weight: 600})
			drawInterfaceText(p, imgR.X+(imgR.W-w)/2, imgR.Y+imgR.H/2+10, ext, 12, 600, th.TextMuted)
		}
		captionY := imgR.Y + imgR.H + 8
		if layout.Captions || it.Kind == "folder" {
			p.Clip(ui.Rect{X: imgR.X + 7, Y: captionY, W: max(0, imgR.W-14), H: 18}, 0, func() {
				color := th.Text.Alpha(.88)
				if selected {
					color = th.Accent
				}
				drawInterfaceText(p, imgR.X+7, captionY, it.Name, 11, 0, color)
			})
		} else if i == hover {
			// With captions hidden the wall stays clean, but a hovered tile
			// still needs to say which file it is before it is opened.
			label := it.Name
			textW, _ := p.MeasureText(0, ui.Span{Text: label, Size: 11, Weight: 500})
			pillW := min(imgR.W-16, textW+20)
			pill := ui.Rect{X: imgR.X + 8, Y: imgR.Y + imgR.H - 34, W: max(0, pillW), H: 25}
			p.Fill(pill, ui.RGBA(10, 10, 11, 176), 8)
			p.Clip(ui.Rect{X: pill.X + 10, Y: pill.Y + 5, W: max(0, pill.W-20), H: 16}, 0, func() {
				drawInterfaceText(p, pill.X+10, pill.Y+5, label, 11, 500, ui.RGBA(255, 255, 255, 240))
			})
		}

		if imageTile {
			// Selection hugs the picture, not an invisible caption card.
			if selected {
				p.Stroke(imgR, th.Accent.Alpha(.92), 8, 2)
			}
		} else if borderWidth > 0 {
			p.Stroke(card, cardBorder, tileRadius, borderWidth)
		}
		if selected {
			badgeBase := card
			if imageTile {
				badgeBase = imgR
			}
			badge := ui.Rect{X: badgeBase.X + badgeBase.W - 28, Y: badgeBase.Y + 8, W: 22, H: 22}
			p.Fill(badge, th.Accent, 10)
			p.Icon(lucide.Check(), ui.Rect{X: badge.X + 6, Y: badge.Y + 6, W: 10, H: 10}, ui.RGBA(255, 255, 255, 255))
		}
	}
}

func (a *App) moveGridSelection(tab *Tab, layout gridLayout, dx, dy float32) {
	cur := -1
	if tab.Anchor != "" {
		cur = tab.indexOf(tab.Anchor)
	}
	if cur < 0 {
		a.selectGridIndex(tab, layout, 0)
		return
	}
	if idx := layout.nearestFrom(cur, dx, dy); idx >= 0 {
		a.selectGridIndex(tab, layout, idx)
	}
}

// extendGridSelection grows the selection from its anchor to the tile one step
// away, entering multi-select on the way. The anchor stays put, so repeated
// Shift+arrow walks the run rather than dragging it along.
func (a *App) extendGridSelection(tab *Tab, layout gridLayout, dx, dy float32) {
	anchor := -1
	if tab.Anchor != "" {
		anchor = tab.indexOf(tab.Anchor)
	}
	if anchor < 0 {
		a.selectGridIndex(tab, layout, 0)
		return
	}
	// The step starts from the moving end of the run, not from the anchor:
	// stepping from the anchor twice would select the same neighbour again.
	edge := anchor
	if tab.MultiSelect && len(tab.Selected) > 1 {
		for i, it := range tab.Items {
			if tab.Selected[it.ID] && i != anchor {
				edge = i
			}
		}
	}
	next := layout.nearestFrom(edge, dx, dy)
	if next < 0 || next >= len(tab.Items) {
		return
	}
	tab.MultiSelect = true
	a.selectRangeKeepingAnchor(tab, tab.Items[next].ID)
}

// selectRangeKeepingAnchor selects the span between the anchor and to, without
// moving the anchor itself.
func (a *App) selectRangeKeepingAnchor(tab *Tab, toID string) {
	to := tab.indexOf(toID)
	from := tab.indexOf(tab.Anchor)
	if from < 0 || to < 0 {
		return
	}
	if from > to {
		from, to = to, from
	}
	tab.Selected = map[string]bool{}
	for i := from; i <= to; i++ {
		tab.Selected[tab.Items[i].ID] = true
	}
	a.refresh()
}

func (a *App) selectGridIndex(tab *Tab, layout gridLayout, idx int) {
	if idx < 0 || idx >= len(layout.Cells) || idx >= len(tab.Items) {
		return
	}
	it := tab.Items[idx]
	// Moving with the arrow keys replaces the selection, as it does in a file
	// list; Shift+arrow is handled separately and extends from the anchor.
	tab.Selected = map[string]bool{it.ID: true}
	tab.Anchor = it.ID
	_, cy := layout.center(idx)
	height := float32(700)
	if a.win != nil {
		if _, h := a.win.ContentSize(); h > 0 {
			height = float32(h)
		}
	}
	tab.Scroll.Y = cy - height/2
	if tab.Scroll.Y < 0 {
		tab.Scroll.Y = 0
	}
	a.refresh()
}

// badgeGeometry is where the video badge lands, shared by the painter and the
// tests. Its pill is a circle around the play mark: with no label to size
// against, the badge needs no text measurement at all.
type badgeGeometry struct {
	Pill ui.Rect
	Icon ui.Rect
}

// videoBadgeGeometry lays out the video badge at origin. The mark is centred
// in a round pill by plain arithmetic, so it cannot drift with the interface
// font, the display scale or the text direction.
func videoBadgeGeometry(origin ui.Rect) badgeGeometry {
	const side, iconSide = float32(24), float32(12)
	pill := ui.Rect{X: origin.X, Y: origin.Y, W: side, H: side}
	return badgeGeometry{
		Pill: pill,
		Icon: ui.Rect{
			X: pill.X + (side-iconSide)/2,
			Y: pill.Y + (side-iconSide)/2,
			W: iconSide,
			H: iconSide,
		},
	}
}
