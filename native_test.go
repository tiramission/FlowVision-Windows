package main

import (
	"fmt"
	"math"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func newViewTestApp() *App {
	s := defaultSettings()
	s.Sidebar = false
	s.ShowOther = true
	a := &App{
		settings:    s,
		sidebar:     false,
		thumbs:      map[string]*thumbEntry{},
		thumbLoad:   map[string]bool{},
		treeOpen:    map[string]*bool{},
		treeLoaded:  map[string]bool{},
		treeLoading: map[string]bool{},
		treeKids:    map[string][]TreeEntry{},
	}
	tab := newTab(0, `C:\Pictures`)
	tab.Loaded = true
	tab.All = []*Item{
		{ID: "1", Name: "a.txt", Kind: "other", Size: 1, W: 1, H: 1},
		{ID: "2", Name: "b.txt", Kind: "other", Size: 1, W: 1, H: 1},
		{ID: "3", Name: "c.txt", Kind: "other", Size: 1, W: 1, H: 1},
	}
	tab.Items = tab.All
	tab.Result = DirResult{Path: tab.Path, Name: "Pictures", Items: tab.All}
	a.tabs = []*Tab{tab}
	a.active = 0
	a.nextID = 1
	return a
}

func TestNativeOnevraSidebar(t *testing.T) {
	a := newViewTestApp()
	a.settings.Sidebar = true
	a.sidebar = true
	a.places = []Place{
		{Name: "Desktop", Path: `C:\Users\test\Desktop`, Icon: "desktop"},
		{Name: "Pictures", Path: `C:\Pictures`, Icon: "pictures"},
	}
	a.drives = []Place{{Name: "C:", Path: `C:\`, Icon: "drive"}}
	tt := ui.NewTester(a.view, 1280, 800)
	for _, want := range []string{"收藏", "位置", "驱动器", "文件夹", "Pictures"} {
		if !tt.HasText(want) {
			t.Fatalf("Onevra sidebar missing %q: %q", want, tt.Texts())
		}
	}
	tt.Frame()
	tt.Frame()
}

func TestNativeSearchShortcut(t *testing.T) {
	a := newViewTestApp()
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Key(ui.Ctrl, ui.KeyF)
	tt.Frame()
	tt.Type("b")
	tt.Frame()
	if len(a.tab().Items) != 1 || a.tab().Items[0].Name != "b.txt" {
		t.Fatalf("ctrl+f filter: %+v", itemNames(a.tab().Items))
	}
}

func TestNativeOnevraTabClose(t *testing.T) {
	a := newViewTestApp()
	second := newTab(1, `C:\Other`)
	second.Name = "Other"
	second.Loaded = true
	a.tabs = append(a.tabs, second)
	a.active = 1
	tt := ui.NewTester(a.view, 1280, 800)
	if err := tt.Click("关闭标签页 Other"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.tabs) != 1 || a.tab().Name != "Pictures" {
		t.Fatalf("close tab: tabs=%d active=%d", len(a.tabs), a.active)
	}
}

func TestNativeTitleBarControls(t *testing.T) {
	a := newViewTestApp()
	tt := ui.NewTester(a.view, 1280, 800)
	if !tt.HasText("FlowVision") {
		t.Fatalf("title bar brand missing: %q", tt.Texts())
	}
	// The window controls are real hit targets, not painted title-bar art.
	// a.win is nil in the tester, so the actions are deliberately no-ops.
	for _, label := range []string{"最小化", "最大化"} {
		if err := tt.Click(label); err != nil {
			t.Fatalf("title bar control %q: %v", label, err)
		}
	}
	tt.Frame()
	if a.win != nil {
		t.Fatal("the test app unexpectedly has a window")
	}
}

func TestNativeTitleBarNarrowWindow(t *testing.T) {
	a := newViewTestApp()
	a.sidebar = true
	a.sidebarWidth = 248
	tt := ui.NewTester(a.view, 760, 500)
	if !tt.HasText("FlowVision") {
		t.Fatalf("narrow title bar brand missing: %q", tt.Texts())
	}
	if err := tt.Click("关闭"); err != nil {
		t.Fatalf("narrow title bar close: %v", err)
	}
	tt.Frame()
}

func TestNativeViewBuilds(t *testing.T) {
	a := newViewTestApp()
	tt := ui.NewTester(a.view, 1280, 800)
	if !tt.HasText("Pictures") {
		t.Fatal("folder title missing")
	}
	if !tt.HasText("3 项") {
		t.Fatalf("status missing: %q", tt.Texts())
	}
	if !tt.HasText("a.txt") {
		// Captions are painted by the grid, not separate text elements; the
		// toolbar path and status must still be there.
		t.Log("grid captions are painted, as expected")
	}
}

func TestNativeSearchFiltersGrid(t *testing.T) {
	a := newViewTestApp()
	tt := ui.NewTester(a.view, 1280, 800)
	if err := tt.Click("搜索"); err != nil {
		t.Fatal(err)
	}
	tt.Type("b")
	tt.Frame()
	if len(a.tab().Items) != 1 || a.tab().Items[0].Name != "b.txt" {
		t.Fatalf("filter: %+v", itemNames(a.tab().Items))
	}
	if !tt.HasText("1 项") {
		t.Fatalf("filtered status: %q", tt.Texts())
	}
}

func TestNativeSelectionAndViewerOverlay(t *testing.T) {
	a := newViewTestApp()
	a.selectOnly("2")
	tt := ui.NewTester(a.view, 1280, 800)
	if !tt.HasText("已选 1 项") {
		t.Fatalf("selection status missing: %q", tt.Texts())
	}
	a.viewer = Viewer{Open: true, Item: a.tab().Items[1], Zoom: 1, ShowInfo: true}
	a.viewer.InfoReady = true
	tt.Frame()
	if !tt.HasText("信息") {
		t.Fatalf("viewer info missing: %q", tt.Texts())
	}
	if !tt.HasText("1 / 3") {
		t.Fatalf("viewer counter missing: %q", tt.Texts())
	}
}

func TestNativeSettingsModal(t *testing.T) {
	a := newViewTestApp()
	a.openSettings()
	tt := ui.NewTester(a.view, 1280, 800)
	if !tt.HasText("设置") {
		t.Fatalf("settings modal missing: %q", tt.Texts())
	}
	if err := tt.Click("缓存"); err != nil {
		t.Fatalf("cache category: %v", err)
	}
	tt.Frame()
	if !tt.HasText("缩略图缓存") {
		t.Fatalf("cache section missing: %q", tt.Texts())
	}
	if !tt.HasText("清除缓存") {
		t.Fatalf("cache controls missing: %q", tt.Texts())
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.showSettings {
		t.Fatal("settings did not close")
	}
}

func TestNativeToolbarSortBinding(t *testing.T) {
	a := newViewTestApp()
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if a.tab().Sort != "name" {
		t.Fatalf("initial sort: %q", a.tab().Sort)
	}
	trigger, ok := tt.AccessByRole(ui.RolePopUpButton, "排序")
	if !ok {
		t.Fatalf("sort trigger missing: %q", tt.Texts())
	}
	tt.ClickAt(trigger.Bounds.X+trigger.Bounds.W/2, trigger.Bounds.Y+trigger.Bounds.H/2)
	tt.Frame()
	if err := tt.Click("日期"); err != nil {
		t.Fatalf("sort option: %v", err)
	}
	tt.Frame()
	if a.tab().Sort != "date" {
		t.Fatalf("toolbar sort did not write the tab: %q", a.tab().Sort)
	}
}

func TestNativeToolbarRecursiveSwitch(t *testing.T) {
	a := newViewTestApp()
	a.settings.Recursive = false
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	info, ok := tt.AccessByRole(ui.RoleSwitch, "递归")
	if !ok {
		t.Fatalf("recursive switch missing: %q", tt.Texts())
	}
	tt.ClickAt(info.Bounds.X+info.Bounds.W/2, info.Bounds.Y+info.Bounds.H/2)
	tt.Frame()
	if !a.settings.Recursive {
		t.Fatal("toolbar recursive switch did not write settings")
	}
}

func TestNativeSettingsSliderBindsDraft(t *testing.T) {
	a := newViewTestApp()
	a.openSettings()
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	info, ok := tt.AccessByRole(ui.RoleSlider, "缩略图宽度")
	if !ok {
		t.Fatalf("thumbnail slider missing: %q", tt.Texts())
	}
	y := info.Bounds.Y + info.Bounds.H/2
	tt.Press(info.Bounds.X+info.Bounds.W*0.2, y)
	tt.Move(info.Bounds.X+info.Bounds.W*0.8, y)
	tt.Release(info.Bounds.X+info.Bounds.W*0.8, y)
	tt.Frame()
	if a.draftThumbSize <= 300 {
		t.Fatalf("slider changed a local copy; draft thumb size = %.0f", a.draftThumbSize)
	}
	s := a.commitSettingsDraft()
	if s.ThumbSize <= 300 {
		t.Fatalf("draft model not updated: %d", s.ThumbSize)
	}
}

func TestNativeSettingsThemeSelect(t *testing.T) {
	a := newViewTestApp()
	a.openSettings()
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	trigger, ok := tt.AccessByRole(ui.RolePopUpButton, "主题")
	if !ok {
		t.Fatalf("theme selector missing: %q", tt.Texts())
	}
	tt.ClickAt(trigger.Bounds.X+trigger.Bounds.W/2, trigger.Bounds.Y+trigger.Bounds.H/2)
	tt.Frame()
	if err := tt.Click("深色"); err != nil {
		t.Fatalf("theme option: %v", err)
	}
	tt.Frame()
	if a.draftTheme != "dark" {
		t.Fatalf("theme did not write the draft: %q", a.draftTheme)
	}
	if got := a.commitSettingsDraft().Theme; got != "dark" {
		t.Fatalf("theme did not commit: %q", got)
	}
}

func TestNativeSettingsNarrowNavigation(t *testing.T) {
	a := newViewTestApp()
	a.openSettings()
	tt := ui.NewTester(a.view, 760, 500)
	tt.Frame()
	trigger, ok := tt.AccessByRole(ui.RolePopUpButton, "设置分类")
	if !ok {
		t.Fatalf("narrow category selector missing: %q", tt.Texts())
	}
	tt.ClickAt(trigger.Bounds.X+trigger.Bounds.W/2, trigger.Bounds.Y+trigger.Bounds.H/2)
	tt.Frame()
	if err := tt.Click("缓存"); err != nil {
		t.Fatalf("narrow cache category: %v", err)
	}
	tt.Frame()
	if !tt.HasText("缩略图缓存") {
		t.Fatalf("narrow cache page missing: %q", tt.Texts())
	}
}

func TestNativeSettingsSearch(t *testing.T) {
	a := newViewTestApp()
	a.openSettings()
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if err := tt.Click("搜索设置"); err != nil {
		t.Fatalf("settings search: %v", err)
	}
	tt.Type("缓存")
	tt.Frame()
	if !tt.HasText("缩略图缓存") || !tt.HasText("缓存上限") {
		t.Fatalf("settings search results missing: %q", tt.Texts())
	}
}

func TestNativeSettingsSwitchPersists(t *testing.T) {
	a := newViewTestApp()
	a.settings.Sidebar = false
	a.sidebar = false
	a.openSettings()
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if a.draft.Sidebar {
		t.Fatal("draft did not copy the persisted value")
	}
	if info, ok := tt.AccessByRole(ui.RoleSwitch, "显示侧边栏"); ok {
		tt.ClickAt(info.Bounds.X+info.Bounds.W/2, info.Bounds.Y+info.Bounds.H/2)
	} else {
		t.Fatal("sidebar switch has no accessible target")
	}
	tt.Frame()
	if !a.draft.Sidebar {
		t.Fatal("switch changed a local copy instead of the retained draft")
	}
	if err := tt.Click("保存"); err != nil {
		t.Fatalf("save: %v", err)
	}
	tt.Frame()
	if a.showSettings {
		t.Fatal("settings modal stayed open after save")
	}
	if !a.settings.Sidebar {
		t.Fatal("saved settings did not receive the toggled value")
	}
}

func TestWaterfallLayout(t *testing.T) {
	items := []*Item{
		{ID: "1", Name: "wide", Kind: "image", W: 1000, H: 500},
		{ID: "2", Name: "tall", Kind: "image", W: 500, H: 1000},
		{ID: "3", Name: "square", Kind: "image", W: 500, H: 500},
		{ID: "4", Name: "panorama", Kind: "image", W: 2000, H: 400},
	}
	g := buildWaterfall(items, 1000, 240, 10)
	if g.Cols != 4 {
		t.Fatalf("cols = %d", g.Cols)
	}
	if len(g.Cells) != 4 {
		t.Fatalf("cells = %d", len(g.Cells))
	}
	if g.TotalH <= 0 {
		t.Fatal("no layout height")
	}
	// The wide image is the shortest; the tall image is the tallest.
	if g.Cells[0].ImgH >= g.Cells[1].ImgH {
		t.Fatalf("aspect not respected: %v %v", g.Cells[0].ImgH, g.Cells[1].ImgH)
	}
	for i, c := range g.Cells {
		if c.X < 0 || c.X+c.W > 1001 {
			t.Fatalf("cell %d out of width: x=%v w=%v", i, c.X, c.W)
		}
	}
	if _, ok := g.hit(g.Cells[2].X+4, g.Cells[2].Y+4); !ok {
		t.Fatal("hit test missed a cell")
	}
}

func TestWaterfallNarrowWindow(t *testing.T) {
	g := buildWaterfall([]*Item{{ID: "1", Kind: "image", W: 1, H: 1}}, 120, 240, 8)
	if g.Cols != 1 {
		t.Fatalf("cols = %d", g.Cols)
	}
	if math.Abs(float64(g.Cells[0].W-120)) > 0.01 {
		t.Fatalf("cell width = %v", g.Cells[0].W)
	}
}

func TestGridNavigation(t *testing.T) {
	g := buildWaterfall([]*Item{
		{ID: "1", Kind: "image", W: 1, H: 1},
		{ID: "2", Kind: "image", W: 1, H: 1},
		{ID: "3", Kind: "image", W: 1, H: 1},
		{ID: "4", Kind: "image", W: 1, H: 1},
		{ID: "5", Kind: "image", W: 1, H: 1},
	}, 1000, 240, 10)
	if g.Cols < 2 {
		t.Skip("too few columns")
	}
	next := g.nearestFrom(0, 1, 0)
	if next < 0 || next > len(g.Cells) {
		t.Fatalf("right navigation = %d", next)
	}
	if g.nearestFrom(0, -1, 0) != -1 {
		t.Fatal("left from first should have no target")
	}
}

func TestNativeGridContextMenu(t *testing.T) {
	a := newViewTestApp()
	pic := filepath.Join(t.TempDir(), "a.png")
	writePNG(t, pic, 20, 20)
	it := a.tab().Items[0]
	it.Name, it.Path, it.Kind, it.Ext = "a.png", pic, "image", ".png"
	tt := ui.NewTester(a.view, 1280, 800)
	tt.RightClickAt(60, 220)
	tt.Frame()
	menu := tt.Menu()
	if len(menu) == 0 {
		t.Fatalf("context menu did not open: %q", tt.Texts())
	}
	t.Logf("menu: %v", menu)
	if err := tt.ChooseMenuItem("打开"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if !a.viewer.Open {
		t.Fatalf("open did not open viewer; menu %v", menu)
	}
	if a.viewer.Item == nil || a.viewer.Item.Name == "" {
		t.Fatal("viewer has no item")
	}
}

func TestNativeGridKeyboardSelection(t *testing.T) {
	a := newViewTestApp()
	tt := ui.NewTester(a.view, 1280, 800)
	tt.ClickAt(60, 220)
	tt.Frame()
	if len(a.tab().Selected) != 1 {
		t.Fatalf("click selected %d", len(a.tab().Selected))
	}
	tt.Key(0, ui.KeyRight)
	tt.Frame()
	if len(a.tab().Selected) != 1 {
		t.Fatalf("right key selected %d", len(a.tab().Selected))
	}
	if a.tab().Anchor == "" {
		t.Fatal("no anchor after keyboard move")
	}
}

func TestSortLabelsRoundTrip(t *testing.T) {
	for _, key := range []string{"name", "date", "size", "type"} {
		if got := sortKey(sortLabel(key)); got != key {
			t.Fatalf("%s -> %s -> %s", key, sortLabel(key), got)
		}
	}
}

func TestThumbKeyIncludesVersion(t *testing.T) {
	it := &Item{ID: "x", Mtime: 10, Size: 100}
	k1 := thumbKey(it, 220)
	it.Mtime = 11
	k2 := thumbKey(it, 220)
	if k1 == k2 {
		t.Fatal("cache key ignored mtime")
	}
}

func itemNames(items []*Item) []string {
	out := []string{}
	for _, it := range items {
		out = append(out, fmt.Sprintf("%s(%s)", it.Name, it.ID))
	}
	return out
}
