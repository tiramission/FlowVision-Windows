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
	It   *Item
	X, Y float32
	W    float32
	ImgH float32
}

func (c gridCell) height() float32 { return c.ImgH + 33 }

type gridLayout struct {
	Cells  []gridCell
	TotalH float32
	Width  float32
	Cols   int
	Gap    float32
}

// buildWaterfall distributes items into the shortest column, using the image
// headers' aspect ratios. Columns share the width equally, as Photos' grid
// does; a row of short panoramas therefore leaves the next row higher up.
func buildWaterfall(items []*Item, width float32, target, gap int) gridLayout {
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
	colY := make([]float32, cols)
	out := gridLayout{Width: width, Cols: cols, Gap: g}
	out.Cells = make([]gridCell, 0, len(items))
	for _, it := range items {
		aspect := itemAspect(it)
		imgH := colW / aspect
		if imgH < 48 {
			imgH = 48
		}
		if imgH > colW*3.2 {
			imgH = colW * 3.2
		}
		col := 0
		for i := 1; i < cols; i++ {
			if colY[i] < colY[col]-0.5 {
				col = i
			}
		}
		cell := gridCell{
			It:   it,
			X:    float32(col) * (colW + g),
			Y:    colY[col],
			W:    colW,
			ImgH: imgH,
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
		width -= float32(a.sidebarWidth)
	}
	width -= 32
	if width < 120 {
		width = 120
	}
	layout := buildWaterfall(tab.Items, width, a.settings.ThumbSize, a.settings.GridGap)
	viewTop := tab.Scroll.Y
	viewBottom := viewTop + windowH
	for _, cell := range layout.Cells {
		if cell.Y+cell.height() < viewTop-160 || cell.Y > viewBottom+160 {
			continue
		}
		if cell.It.Kind == "image" || cell.It.Kind == "video" {
			a.ensureThumb(cell.It, a.settings.ThumbSize)
		}
	}

	scroll := ui.Scroll(c).Fill().TrackScroll(&tab.Scroll).Padding(16).Gap(0)
	scroll.Children(func() {
		box := ui.Box(c).Height(layout.TotalH).Focusable().Role(ui.RoleList).
			Label(fmt.Sprintf("%s，%d 项", tab.Name, len(tab.Items)))
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
			x, y, _ := box.PointerPosition()
			if idx, ok := layout.hit(x, y); ok {
				it := layout.Cells[idx].It
				mods := box.ClickModifiers()
				switch {
				case mods&ui.Shift != 0 && tab.Anchor != "":
					a.selectRange(it.ID)
				case mods&ui.Cmd != 0:
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
				a.later(func() { a.openViewer(it) })
			}
		}
		if box.RightClicked() {
			x, y, _ := box.PointerPosition()
			a.contextAtX, a.contextAtY = x, y
		}

		box.ContextMenu(func(m *ui.Menu) {
			// The native context menu opens on the secondary click before the
			// element reports RightClicked, so locate the tile here, at the
			// pointer position the menu is anchored to.
			x, y, have := box.PointerPosition()
			if !have {
				x, y = a.contextAtX, a.contextAtY
			}
			a.contextIDs = nil
			if idx, ok := layout.hit(x, y); ok {
				it := layout.Cells[idx].It
				if !tab.Selected[it.ID] {
					tab.Selected = map[string]bool{it.ID: true}
					tab.Anchor = it.ID
				}
				a.contextEmpty = false
			} else {
				a.contextEmpty = true
			}
			for id := range tab.Selected {
				a.contextIDs = append(a.contextIDs, id)
			}

			if a.contextEmpty || len(a.contextIDs) == 0 {
				if m.Item("新建文件夹…").Chosen() {
					a.later(a.beginNewFolder)
				}
				if m.Item("刷新").Shortcut(0, ui.KeyF5).Chosen() {
					t := tab
					a.later(func() { a.loadTab(t, false) })
				}
				m.Separator()
				if m.Item("全选").Shortcut(ui.Cmd, ui.KeyA).Chosen() {
					t := tab
					a.later(func() { a.active = tabIndex(a.tabs, t); a.selectAll() })
				}
				if m.Item("取消选择").Chosen() {
					a.later(a.clearSelection)
				}
				return
			}
			one := a.contextOne()
			if m.Item("打开").Chosen() && one != nil {
				a.later(func() { a.openViewer(one) })
			}
			if m.Item("在资源管理器中显示").Chosen() {
				ids := append([]string(nil), a.contextIDs...)
				a.later(func() {
					if ids2 := ids; len(ids2) == 1 {
						if it, ok := items.Get(ids2[0]); ok {
							_ = (AppSvc{}).Reveal(it.Path)
						}
					} else if t := a.tab(); t != nil {
						_ = (AppSvc{}).OpenFolder(t.Path)
					}
				})
			}
			if m.Item("用默认应用打开").Disabled(one == nil).Chosen() && one != nil {
				path := one.Path
				a.later(func() { _ = (AppSvc{}).OpenExternal(path) })
			}
			if m.Item("复制路径").Disabled(one == nil).Chosen() && one != nil {
				path := one.Path
				a.later(func() { _ = (AppSvc{}).CopyText(path) })
			}
			m.Separator()
			if m.Item("重命名…").Shortcut(0, ui.KeyF2).Disabled(one == nil).Chosen() {
				a.later(a.beginRename)
			}
			if m.Item("移到回收站").Shortcut(0, ui.KeyDelete).Chosen() {
				a.later(func() { a.askDelete(false) })
			}
			if m.Item("永久删除").Chosen() {
				a.later(func() { a.askDelete(true) })
			}
			m.Separator()
			if m.Item("显示信息").Disabled(one == nil).Chosen() && one != nil {
				a.later(func() {
					a.openViewer(one)
					a.viewerToggleInfo()
				})
			}
		})

		box.OnShortcut(0, ui.KeyEnter, func() {
			if it := tab.selectedOne(); it != nil {
				a.later(func() { a.openViewer(it) })
			}
		})
		box.OnShortcut(0, ui.KeySpace, func() {
			if it := tab.selectedOne(); it != nil {
				a.later(func() { a.openViewer(it) })
			}
		})
		box.OnShortcut(0, ui.KeyDelete, func() {
			if a.settings.ShowOther || len(tab.Selected) > 0 {
				a.later(func() { a.askDelete(false) })
			}
		})
		box.OnShortcut(ui.Cmd, ui.KeyA, func() { a.later(a.selectAll) })
		box.OnShortcut(0, ui.KeyEscape, func() { a.later(a.clearSelection) })
		box.OnShortcut(0, ui.KeyF2, func() {
			if tab.selectedOne() != nil {
				a.later(a.beginRename)
			}
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

		// Each tile is a Onevra surface card: a warm face, a hairline border and
		// a caption that stays inside the card rather than floating on the page.
		cardFace := th.Surface
		cardBorder := th.Border.Alpha(.75)
		if i == hover {
			cardFace = th.SurfaceHover
			cardBorder = th.Accent.Alpha(.55)
			p.Shadow(card, 10, 0, 3, 10, 0, ui.RGBA(0, 0, 0, .10))
		}
		p.Fill(card, cardFace, 10)
		p.Stroke(card, cardBorder, 10, 1)

		if it.Kind == "image" || it.Kind == "video" {
			bmp := a.thumb(it, a.settings.ThumbSize)
			if bmp != nil {
				p.Clip(imgR, 9, func() {
					p.Image(bmp, imgR, ui.Cover)
				})
			} else {
				p.Fill(imgR, th.SurfaceHover, 9)
				icon := lucide.ImageOff()
				if it.Kind == "video" {
					icon = lucide.Video()
				}
				side := float32(30)
				p.Icon(icon, ui.Rect{X: imgR.X + (imgR.W-side)/2, Y: imgR.Y + (imgR.H-side)/2, W: side, H: side}, th.TextMuted)
			}
			if it.Kind == "video" {
				badge := ui.Rect{X: imgR.X + 8, Y: imgR.Y + 8, W: 56, H: 22}
				p.Fill(badge, ui.RGBA(0, 0, 0, 150), 11)
				p.Icon(lucide.Play(), ui.Rect{X: badge.X + 7, Y: badge.Y + 6, W: 10, H: 10}, ui.RGBA(255, 255, 255, 235))
				p.Text(badge.X+20, badge.Y+4, "视频", 10.5, ui.RGBA(255, 255, 255, 235))
			}
		} else {
			p.Fill(imgR, th.SurfaceHover, 9)
			ext := strings.TrimPrefix(strings.ToUpper(filepath.Ext(it.Name)), ".")
			if ext == "" {
				ext = "文件"
			}
			icon := lucide.File()
			side := float32(30)
			p.Icon(icon, ui.Rect{X: imgR.X + (imgR.W-side)/2, Y: imgR.Y + (imgR.H-side)/2 - 8, W: side, H: side}, th.TextMuted)
			w, _ := p.MeasureText(0, ui.Span{Text: ext, Size: 12, Weight: 600})
			p.Text(imgR.X+(imgR.W-w)/2, imgR.Y+imgR.H/2+10, ext, 12, th.TextMuted)
		}
		if selected {
			p.Stroke(imgR, th.Accent, 9, 3)
		}

		captionY := imgR.Y + imgR.H + 8
		p.Clip(ui.Rect{X: imgR.X + 9, Y: captionY, W: max(0, imgR.W-18), H: 18}, 0, func() {
			color := th.Text
			if i == hover {
				color = th.Accent
			}
			p.Text(imgR.X+9, captionY, it.Name, 11.5, color)
		})
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

func (a *App) selectGridIndex(tab *Tab, layout gridLayout, idx int) {
	if idx < 0 || idx >= len(layout.Cells) || idx >= len(tab.Items) {
		return
	}
	it := tab.Items[idx]
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
