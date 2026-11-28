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
		settings:     s,
		sidebar:      false,
		thumbs:       map[string]*thumbEntry{},
		thumbLoad:    map[string]bool{},
		treeOpen:     map[string]*bool{},
		treeRootOpen: map[string]*bool{},
		treeLoaded:   map[string]bool{},
		treeLoading:  map[string]bool{},
		treeKids:     map[string][]TreeEntry{},
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

func TestNativeStatusBarCentered(t *testing.T) {
	a := newViewTestApp()
	tt := ui.NewTester(a.view, 1280, 800)
	var centers []float32
	for _, label := range []string{"3 项", "F1 快捷键"} {
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("status label %q has no bounds: %q", label, tt.Texts())
		}
		centers = append(centers, r.Y+r.H/2)
	}
	if math.Abs(float64(centers[0]-centers[1])) > 0.5 {
		t.Fatalf("status labels are not vertically centered: %v", centers)
	}
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
	tt := ui.NewTester(a.view, 1280, 800)
	// A plain click selects one tile and nothing more: no floating bar over the
	// folder until the user actually enters multi-select.
	a.selectOnly("2")
	tt.Frame()
	if tt.HasText("已选 1 项") {
		t.Fatalf("single click showed the selection bar: %q", tt.Texts())
	}
	a.setMultiSelect(true)
	tt.Frame()
	if !tt.HasText("已选 1 项") {
		t.Fatalf("selection status missing: %q", tt.Texts())
	}
	if !tt.HasText("取消选择") {
		t.Fatalf("selection action bar missing: %q", tt.Texts())
	}
	if err := tt.Click("取消选择"); err != nil {
		t.Fatalf("clear selection: %v", err)
	}
	tt.Frame()
	if len(a.tab().Selected) != 0 || a.tab().MultiSelect {
		t.Fatalf("clear selection left %v (multi=%v)", a.tab().Selected, a.tab().MultiSelect)
	}
	a.selectOnly("2")
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

func TestNativeToolbarCompactSearch(t *testing.T) {
	a := newViewTestApp()
	a.sidebar = true
	a.sidebarWidth = 248
	tt := ui.NewTester(a.view, 900, 600)
	tt.Frame()
	if err := tt.Click("搜索 (Ctrl+F)"); err != nil {
		t.Fatalf("compact search button: %v", err)
	}
	tt.Frame()
	if !a.searchOpen {
		t.Fatal("compact search button did not open the field")
	}
	if !tt.HasText("收起搜索") {
		t.Fatalf("compact search close control missing: %q", tt.Texts())
	}
}

func TestBreadcrumbItems(t *testing.T) {
	labels, paths := breadcrumbItems(`C:\Users\test\Pictures`, "Pictures")
	wantLabels := []string{"C:", "Users", "test", "Pictures"}
	wantPaths := []string{`C:\`, `C:\Users`, `C:\Users\test`, `C:\Users\test\Pictures`}
	if fmt.Sprint(labels) != fmt.Sprint(wantLabels) {
		t.Fatalf("labels = %v, want %v", labels, wantLabels)
	}
	if fmt.Sprint(paths) != fmt.Sprint(wantPaths) {
		t.Fatalf("paths = %v, want %v", paths, wantPaths)
	}
	labels, paths = breadcrumbItems(`C:\`, `C:\`)
	if len(labels) != 1 || labels[0] != "C:" || len(paths) != 1 || paths[0] != `C:\` {
		t.Fatalf("root breadcrumb = %v %v", labels, paths)
	}
	if labels, paths = breadcrumbItems("", ""); labels != nil || paths != nil {
		t.Fatalf("empty breadcrumb = %v %v", labels, paths)
	}
}

func TestNativeSettingsModal(t *testing.T) {
	a := newViewTestApp()
	a.openSettings()
	tt := ui.NewTester(a.view, 1280, 800)
	if !tt.HasText("设置") {
		t.Fatalf("settings modal missing: %q", tt.Texts())
	}
	if err := tt.Click("缓存与更新"); err != nil {
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
	// The direction starts from the saved setting, so pin it for the test.
	a.tab().Desc = false
	tt.Frame()
	trigger, ok := tt.AccessByRole(ui.RolePopUpButton, "排序方式，再次选择同一项切换升序降序")
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
	if a.tab().Desc {
		t.Fatalf("switching field should start ascending: desc=%v", a.tab().Desc)
	}

	// Switching away from a descending field drops the direction, so a new
	// field never inherits the previous one's order.
	a.tab().Desc = true
	tt.Frame()
	tt.ClickAt(trigger.Bounds.X+trigger.Bounds.W/2, trigger.Bounds.Y+trigger.Bounds.H/2)
	tt.Frame()
	if err := tt.Click("大小"); err != nil {
		t.Fatalf("sort option: %v", err)
	}
	tt.Frame()
	if a.tab().Sort != "size" || a.tab().Desc {
		t.Fatalf("switch from descending: sort=%q desc=%v", a.tab().Sort, a.tab().Desc)
	}
}

// TestNativeToolbarSortDirection covers the direction half of the sort
// control: one row per field, and choosing the field in use flips the order.
func TestNativeToolbarSortDirection(t *testing.T) {
	a := newViewTestApp()
	a.tab().Sort, a.tab().Desc = "name", false
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()

	trigger, ok := tt.AccessByRole(ui.RolePopUpButton, "排序方式，再次选择同一项切换升序降序")
	if !ok {
		t.Fatalf("sort trigger missing: %q", tt.Texts())
	}
	tt.ClickAt(trigger.Bounds.X+trigger.Bounds.W/2, trigger.Bounds.Y+trigger.Bounds.H/2)
	tt.Frame()

	// One row per field; the direction is not a second set of rows.
	for _, label := range []string{"名称", "日期", "大小", "类型"} {
		if !tt.HasText(label) {
			t.Fatalf("sort menu missing %q: %q", label, tt.Texts())
		}
	}
	if tt.HasText("升序") && countText(tt.Texts(), "升序") > 1 {
		t.Fatalf("the menu lists directions as rows again: %q", tt.Texts())
	}

	// Choosing the field already in use flips the order.
	if err := tt.Click("名称"); err != nil {
		t.Fatalf("click 名称: %v", err)
	}
	tt.Frame()
	if !a.tab().Desc {
		t.Fatalf("re-choosing the field did not flip the order: desc=%v", a.tab().Desc)
	}
	if a.tab().Sort != "name" {
		t.Fatalf("field changed while flipping: %q", a.tab().Sort)
	}
}

// countText counts how many of texts are label.
func countText(texts []string, label string) int {
	n := 0
	for _, text := range texts {
		if text == label {
			n++
		}
	}
	return n
}

func TestNativeToolbarViewMenuRecursive(t *testing.T) {
	a := newViewTestApp()
	a.settings.Recursive = false
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	more, ok := tt.AccessByRole(ui.RoleMenuButton, "更多操作")
	if !ok {
		t.Fatalf("view options menu missing: %q", tt.Texts())
	}
	tt.ClickAt(more.Bounds.X+more.Bounds.W/2, more.Bounds.Y+more.Bounds.H/2)
	tt.Frame()
	if err := tt.Click("递归扫描子文件夹"); err != nil {
		t.Fatalf("recursive menu item: %v", err)
	}
	tt.Frame()
	if !a.settings.Recursive {
		t.Fatal("view options menu did not write recursive settings")
	}
}

func TestNativeToolbarMediaFilterTabs(t *testing.T) {
	a := newViewTestApp()
	tab := a.tab()
	tab.All = []*Item{
		{ID: "folder", Name: "2024", Kind: "folder"},
		{ID: "image", Name: "photo.jpg", Kind: "image"},
		{ID: "video", Name: "clip.mp4", Kind: "video"},
		{ID: "other", Name: "notes.txt", Kind: "other"},
	}
	tab.Items = tab.All
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if err := tt.Click("图片"); err != nil {
		t.Fatalf("media filter tab: %v", err)
	}
	tt.Frame()
	if tab.Filter != "image" {
		t.Fatalf("media filter tab wrote %q, want image", tab.Filter)
	}
	if len(tab.Items) != 2 || tab.Items[0].ID != "folder" || tab.Items[1].ID != "image" {
		t.Fatalf("image filter produced %v", tab.Items)
	}
	if !tt.HasText("2 / 4 项") {
		t.Fatalf("filtered count missing: %q", tt.Texts())
	}
}

func TestNativeEmptyFilterAction(t *testing.T) {
	a := newViewTestApp()
	tab := a.tab()
	tab.Filter = "video"
	tab.All = []*Item{
		{ID: "image", Name: "photo.jpg", Kind: "image"},
	}
	tab.Items = nil
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if !tt.HasText("没有可显示的视频") {
		t.Fatalf("filtered empty state missing: %q", tt.Texts())
	}
	if err := tt.Click("显示全部"); err != nil {
		t.Fatalf("show all action: %v", err)
	}
	tt.Frame()
	if tab.Filter != "all" || len(tab.Items) != 1 || tab.Items[0].ID != "image" {
		t.Fatalf("show all left filter=%q items=%v", tab.Filter, tab.Items)
	}
}

func TestDefaultSettingsHideCardCaptions(t *testing.T) {
	if defaultSettings().ShowCardNames {
		t.Fatal("new galleries should open with card captions hidden")
	}
}

func TestDefaultSettingsViewerAutoImmersive(t *testing.T) {
	if !defaultSettings().ViewerAutoImmersive {
		t.Fatal("new installs should enter the viewer in immersive mode")
	}
	s, ok := parseSettings([]byte(`{"theme":"light"}`))
	if !ok || !s.ViewerAutoImmersive {
		t.Fatal("older settings without the field should keep auto-immersive on")
	}
}

func TestMediaFilterKeepsFoldersAndMatchingKind(t *testing.T) {
	tab := newTab(1, `C:\Pictures`)
	tab.All = []*Item{
		{ID: "folder", Name: "2024", Kind: "folder"},
		{ID: "image", Name: "photo.jpg", Kind: "image"},
		{ID: "video", Name: "clip.mp4", Kind: "video"},
		{ID: "other", Name: "notes.txt", Kind: "other"},
	}
	tab.Items = tab.All
	for filter, want := range map[string]string{
		"image": "2024|photo.jpg",
		"video": "2024|clip.mp4",
		"all":   "2024|photo.jpg|clip.mp4|notes.txt",
	} {
		tab.Filter = filter
		tab.refilter()
		var got string
		for i, it := range tab.Items {
			if i > 0 {
				got += "|"
			}
			got += it.Name
		}
		if got != want {
			t.Fatalf("filter %q produced %q, want %q", filter, got, want)
		}
	}
	tab.Filter = "image"
	tab.Query = "photo"
	tab.refilter()
	if len(tab.Items) != 1 || tab.Items[0].Name != "photo.jpg" {
		t.Fatalf("query should combine with filter: %v", tab.Items)
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
	if err := tt.Click("缓存与更新"); err != nil {
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
	g := buildWaterfall(items, 1000, 240, 10, true)
	if g.Cols != 4 {
		t.Fatalf("cols = %d", g.Cols)
	}
	if len(g.Cells) != 4 {
		t.Fatalf("cells = %d", len(g.Cells))
	}
	if g.TotalH <= 0 {
		t.Fatal("no layout height")
	}
	if !g.Captions {
		t.Fatal("captions should be part of the layout")
	}
	hidden := buildWaterfall(items, 1000, 240, 10, false)
	if hidden.Captions || hidden.Cells[0].height() >= g.Cells[0].height() {
		t.Fatalf("hidden captions did not shrink cards: %v vs %v", hidden.Cells[0].height(), g.Cells[0].height())
	}
	folders := buildWaterfall([]*Item{{ID: "f", Name: "文件夹", Kind: "folder", W: 1, H: 1}}, 500, 240, 10, false)
	if got := folders.Cells[0].height() - folders.Cells[0].ImgH; got < 32 {
		t.Fatalf("folder caption disappeared in immersive mode: %.1f", got)
	}
	if got := folders.Cells[0].ImgH; got > 240 {
		t.Fatalf("folder placeholder grew past its thumbnail scale: %.1f", got)
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
	g := buildWaterfall([]*Item{{ID: "1", Kind: "image", W: 1, H: 1}}, 120, 240, 8, true)
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
	}, 1000, 240, 10, true)
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
	if !tt.HasText("在资源管理器中显示") {
		t.Fatalf("context menu did not open: %q", tt.Texts())
	}
	if len(a.tab().Selected) != 1 {
		t.Fatalf("context click selected %d items", len(a.tab().Selected))
	}
	if err := tt.Click("显示信息"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if !a.viewer.Open {
		t.Fatalf("open did not open viewer; texts %q", tt.Texts())
	}
	if !a.viewer.ShowInfo {
		t.Fatal("context action did not open the information panel")
	}
	if a.viewer.Item == nil || a.viewer.Item.Name == "" {
		t.Fatal("viewer has no item")
	}
}

func TestNativeGridContextMenuKeyboard(t *testing.T) {
	for _, tc := range []struct {
		name string
		mod  ui.Modifiers
		key  ui.Key
	}{
		{"menu-key", 0, ui.KeyContextMenu},
		{"shift-f10", ui.Shift, ui.KeyF10},
	} {
		a := newViewTestApp()
		pic := filepath.Join(t.TempDir(), "a.png")
		writePNG(t, pic, 20, 20)
		it := a.tab().Items[0]
		it.Name, it.Path, it.Kind, it.Ext = "a.png", pic, "image", ".png"
		tt := ui.NewTester(a.view, 1280, 800)
		tt.ClickAt(60, 220)
		tt.Frame()
		tt.Key(tc.mod, tc.key)
		tt.Frame()
		if !tt.HasText("在资源管理器中显示") {
			t.Fatalf("%s: grid menu did not open: %q", tc.name, tt.Texts())
		}
		if len(a.tab().Selected) != 1 {
			t.Fatalf("%s: selection changed to %v", tc.name, a.tab().Selected)
		}
	}
}

func TestNativeGridBlankContextMenu(t *testing.T) {
	a := newViewTestApp()
	tt := ui.NewTester(a.view, 1280, 800)
	tt.ClickAt(60, 220)
	tt.Frame()
	tt.RightClickAt(60, 650)
	tt.Frame()
	if !a.gridMenuOpen || !tt.HasText("新建文件夹…") || tt.HasText("在资源管理器中显示") {
		t.Fatalf("blank menu: open=%v texts=%q", a.gridMenuOpen, tt.Texts())
	}
	if !a.tab().Selected["1"] {
		t.Fatal("blank menu should keep the existing selection")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.gridMenuOpen {
		t.Fatal("escape did not close the blank menu")
	}
}

func TestNativeGridKeyboardMenuVisibleAfterScroll(t *testing.T) {
	a := newViewTestApp()
	tb := a.tab()
	tb.Items, tb.All = nil, nil
	for i := 0; i < 40; i++ {
		it := &Item{ID: fmt.Sprintf("%d", i+1), Name: fmt.Sprintf("p%02d.png", i), Kind: "image", W: 800, H: 600}
		tb.Items = append(tb.Items, it)
		tb.All = append(tb.All, it)
	}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Scroll(600, 400, 0, 100000)
	tt.Frame()
	tt.ClickAt(1000, 600)
	tt.Frame()
	tb.Selected = map[string]bool{}
	tb.Anchor = ""
	tt.Key(0, ui.KeyContextMenu)
	tt.Frame()
	if !a.gridMenuOpen || !tt.HasText("新建文件夹…") {
		t.Fatalf("keyboard menu after scroll: open=%v anchor=%.0f,%.0f texts=%q",
			a.gridMenuOpen, a.gridMenuX, a.gridMenuY, tt.Texts())
	}
}

func TestCompactSidebarAndViewerInfoWidth(t *testing.T) {
	a := &App{sidebar: true, sidebarWidth: 248}
	if got := a.sidebarLayoutWidth(900); got != compactSidebarMax {
		t.Fatalf("compact sidebar = %.0f, want %.0f", got, compactSidebarMax)
	}
	if got := a.sidebarLayoutWidth(1320); got != 248 {
		t.Fatalf("normal sidebar = %.0f, want 248", got)
	}
	a.sidebarWidth = 160
	if got := a.sidebarLayoutWidth(900); got != 160 {
		t.Fatalf("narrow custom sidebar = %.0f, want 160", got)
	}
	a.sidebarWidth = 520
	if got := a.sidebarLayoutWidth(900); got != compactSidebarMax {
		t.Fatalf("wide custom sidebar at compact width = %.0f, want %.0f", got, compactSidebarMax)
	}
	if got := a.sidebarLayoutWidth(1040); got != compactSidebarMax {
		t.Fatalf("wide custom sidebar at minimum wide width = %.0f, want %.0f", got, compactSidebarMax)
	}
	if got := a.sidebarLayoutWidth(1320); got != 520 {
		t.Fatalf("wide custom sidebar at roomy width = %.0f, want 520", got)
	}
	if got := viewerInfoPanelWidth(900); got != 252 {
		t.Fatalf("compact info panel = %.0f, want 252", got)
	}
	if got := viewerInfoPanelWidth(1000); got != 300 {
		t.Fatalf("normal info panel = %.0f, want 300", got)
	}
}

func TestSettingsDirtyTracksEditableFields(t *testing.T) {
	a := &App{settings: defaultSettings()}
	a.fillSettingsDraft(a.settings)
	if a.settingsDirty() {
		t.Fatal("fresh draft reported dirty")
	}
	a.draft.ShowHidden = !a.settings.ShowHidden
	if !a.settingsDirty() {
		t.Fatal("changed preference was not dirty")
	}
	a.draft = a.settings
	a.draft.Favorites = append(a.draft.Favorites, Favorite{Path: `C:\Pictures`, Kind: "folder"})
	if a.settingsDirty() {
		t.Fatal("favorites are not an editable preference")
	}
}

func TestSettingsDirtyCloseAsksBeforeDiscard(t *testing.T) {
	a := newViewTestApp()
	a.openSettings()
	a.draft.ShowHidden = !a.settings.ShowHidden
	tt := ui.NewTester(a.view, 1200, 900)
	tt.Frame()
	a.requestSettingsClose()
	tt.Frame()
	if !a.showSettings || !a.settingsConfirm || !a.settingsModalOpen {
		t.Fatalf("dirty close: open=%v confirm=%v modal=%v", a.showSettings, a.settingsConfirm, a.settingsModalOpen)
	}
	if !tt.HasText("放弃未保存的修改？") {
		t.Fatalf("dirty close did not show confirmation: %q", tt.Texts())
	}
	if err := tt.Click("继续编辑"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.showSettings || a.settingsConfirm || !a.settingsModalOpen {
		t.Fatalf("continue editing: open=%v confirm=%v modal=%v", a.showSettings, a.settingsConfirm, a.settingsModalOpen)
	}
	a.requestSettingsClose()
	tt.Frame()
	if !a.settingsConfirm {
		t.Fatal("second dirty close did not confirm")
	}
	if err := tt.Click("放弃修改"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.showSettings || a.settingsConfirm || a.settingsModalOpen {
		t.Fatalf("discard: open=%v confirm=%v modal=%v", a.showSettings, a.settingsConfirm, a.settingsModalOpen)
	}
}

func TestSettingsEscapeKeepsDraft(t *testing.T) {
	a := newViewTestApp()
	a.openSettings()
	tt := ui.NewTester(a.view, 1200, 900)
	tt.Frame()
	a.draft.ShowHidden = !a.settings.ShowHidden
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if !a.showSettings || !a.settingsConfirm || !a.settingsModalOpen {
		t.Fatalf("escape: open=%v confirm=%v modal=%v", a.showSettings, a.settingsConfirm, a.settingsModalOpen)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if !a.showSettings || a.settingsConfirm || !a.settingsModalOpen {
		t.Fatalf("escape confirm: open=%v confirm=%v modal=%v", a.showSettings, a.settingsConfirm, a.settingsModalOpen)
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
	if !a.tab().Selected["2"] {
		t.Fatalf("right key did not advance to the second item: %v", a.tab().Selected)
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

func TestWindowMinimumWidthTracksSidebar(t *testing.T) {
	if got := windowMinWidth(false, 248); got != minWindowWidth {
		t.Fatalf("collapsed sidebar min width = %d, want %d", got, minWindowWidth)
	}
	if got := windowMinWidth(true, 248); got != minWindowWidth {
		t.Fatalf("default sidebar min width = %d, want %d", got, minWindowWidth)
	}
	// A wide sidebar must push the native floor out instead of squeezing the
	// workbench below the width its title bar and toolbar need.
	if got := windowMinWidth(true, 520); got != 1040 {
		t.Fatalf("wide sidebar min width = %d, want 1040", got)
	}
}

func TestNativeFolderTreeRootAndLeaves(t *testing.T) {
	a := newViewTestApp()
	a.settings.Sidebar = true
	a.sidebar = true
	a.sidebarWidth = defaultSettings().SidebarWidth
	rootPath := filepath.Clean(a.tab().Path)
	folderPath := filepath.Join(rootPath, "Folder")
	emptyPath := filepath.Join(rootPath, "Empty")
	a.treeLoaded[rootPath] = true
	a.treeKids[rootPath] = []TreeEntry{
		{Name: "Folder", Path: folderPath, HasChildren: true},
		{Name: "Empty", Path: emptyPath, HasChildren: false},
	}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	root, ok := tt.AccessByRole(ui.RoleTreeItem, "Pictures")
	if !ok {
		t.Fatalf("current folder is not the tree root: %q", tt.Texts())
	}
	if !root.Expandable || !root.Expanded || !root.Selected || root.Level != 1 {
		t.Fatalf("root disclosure state = %+v", root)
	}
	branch, ok := tt.AccessByRole(ui.RoleTreeItem, "Folder")
	if !ok || !branch.Expandable || branch.Expanded || branch.Level != 2 {
		t.Fatalf("branch state = %+v, ok=%v", branch, ok)
	}
	leaf, ok := tt.AccessByRole(ui.RoleTreeItem, "Empty")
	if !ok || leaf.Expandable || leaf.Level != 2 {
		t.Fatalf("leaf state = %+v, ok=%v", leaf, ok)
	}
	if !tt.HasText("2 项") {
		t.Fatalf("folder child count missing: %q", tt.Texts())
	}

	open := a.treeRootOpenPtr(rootPath)
	tt.ClickAt(root.Bounds.X+root.Bounds.W/2, root.Bounds.Y+root.Bounds.H/2)
	tt.Frame()
	if *open {
		t.Fatal("clicking the current folder did not collapse it")
	}
	if _, ok := tt.AccessByRole(ui.RoleTreeItem, "Empty"); ok {
		t.Fatal("collapsed root still exposes child rows")
	}
}

func TestTreeRootOpenDefaultsExpanded(t *testing.T) {
	a := newViewTestApp()
	open := a.treeRootOpenPtr(`C:\Pictures`)
	if !*open {
		t.Fatal("a newly visited folder root starts collapsed")
	}
	*open = false
	if *a.treeRootOpenPtr(`C:\Pictures`) {
		t.Fatal("root expansion reset to expanded instead of being retained")
	}
	open = a.treeRootOpenPtr(`C:\Other`)
	if !*open {
		t.Fatal("a separate folder root should start expanded")
	}
}

func TestNativeFolderTreeArrowExpandsWithoutNavigating(t *testing.T) {
	a := newViewTestApp()
	a.settings.Sidebar = true
	a.sidebar = true
	a.sidebarWidth = defaultSettings().SidebarWidth
	rootPath := filepath.Clean(a.tab().Path)
	branchPath := filepath.Join(rootPath, "Folder")
	nestedPath := filepath.Join(branchPath, "Nested")
	a.treeLoaded[rootPath] = true
	a.treeKids[rootPath] = []TreeEntry{{Name: "Folder", Path: branchPath, HasChildren: true}}
	a.treeLoaded[branchPath] = true
	a.treeKids[branchPath] = []TreeEntry{{Name: "Nested", Path: nestedPath, HasChildren: false}}

	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	branch, ok := tt.AccessByRole(ui.RoleTreeItem, "Folder")
	if !ok {
		t.Fatalf("branch row missing: %q", tt.Texts())
	}
	before := a.tab().Path
	// The arrow occupies the first 14 DIP inside the row, after its depth
	// indentation. Click it, not the label.
	tt.ClickAt(branch.Bounds.X+27, branch.Bounds.Y+branch.Bounds.H/2)
	tt.Frame()
	if a.tab().Path != before {
		t.Fatalf("clicking the disclosure arrow navigated to %q", a.tab().Path)
	}
	if !*a.treeOpenPtr(branchPath) {
		t.Fatal("clicking the disclosure arrow did not expand the branch")
	}
	nested, ok := tt.AccessByRole(ui.RoleTreeItem, "Nested")
	if !ok || nested.Level != 3 {
		t.Fatalf("nested row = %+v, ok=%v", nested, ok)
	}
}

func TestNativeEmptyFolderTreeRootIsLeaf(t *testing.T) {
	a := newViewTestApp()
	a.settings.Sidebar = true
	a.sidebar = true
	a.sidebarWidth = defaultSettings().SidebarWidth
	rootPath := filepath.Clean(a.tab().Path)
	a.treeLoaded[rootPath] = true
	a.treeKids[rootPath] = nil
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	root, ok := tt.AccessByRole(ui.RoleTreeItem, "Pictures")
	if !ok {
		t.Fatalf("empty folders page has no tree root: %q", tt.Texts())
	}
	if root.Expandable || root.Expanded || !root.Selected {
		t.Fatalf("empty root disclosure state = %+v", root)
	}
	if !tt.HasText("没有子文件夹") {
		t.Fatalf("empty tree hint missing: %q", tt.Texts())
	}
	if tt.HasText("0 项") {
		t.Fatalf("zero count should not duplicate the empty hint: %q", tt.Texts())
	}
}

func TestNativeViewBuildsAtMinimumContentSize(t *testing.T) {
	a := newViewTestApp()
	a.settings.Sidebar = true
	a.sidebar = true
	a.sidebarWidth = defaultSettings().SidebarWidth
	// The native outer floor is 900×600; the page area is a few DIP smaller.
	tt := ui.NewTester(a.view, minWindowWidth-15, minWindowHeight-8)
	for _, want := range []string{"FlowVision", "收藏", "3 项", "F1 快捷键"} {
		if !tt.HasText(want) {
			t.Fatalf("minimum-size split layout missing %q: %q", want, tt.Texts())
		}
	}
	tt.Frame()
}

func itemNames(items []*Item) []string {
	out := []string{}
	for _, it := range items {
		out = append(out, fmt.Sprintf("%s(%s)", it.Name, it.ID))
	}
	return out
}

// TestNativeSortMenuRowsSnapshot records the sort menu's shape: one row per
// field, with the direction shown on the field in use.
func TestNativeSortMenuRowsSnapshot(t *testing.T) {
	a := newViewTestApp()
	a.tab().Sort, a.tab().Desc = "date", true
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	trigger, ok := tt.AccessByRole(ui.RolePopUpButton, "排序方式，再次选择同一项切换升序降序")
	if !ok {
		t.Fatalf("trigger missing")
	}
	tt.ClickAt(trigger.Bounds.X+trigger.Bounds.W/2, trigger.Bounds.Y+trigger.Bounds.H/2)
	tt.Frame()

	texts := tt.Texts()
	for _, label := range []string{"名称", "日期", "大小", "类型"} {
		if countText(texts, label) < 1 {
			t.Fatalf("%q missing from the menu: %q", label, texts)
		}
	}
	// The order shows once, on the field in use, not on every row. The trigger
	// carries the same order as an arrow, which has no text.
	if got := countText(texts, "降序"); got != 1 {
		t.Fatalf("降序 appears %d times, want only the active row: %q", got, texts)
	}
}
