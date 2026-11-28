package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// favoritesApp builds an app whose tab holds two images, one video and one
// folder, which is what the favorite and multi-select tests need.
func favoritesApp() *App {
	a := newViewTestApp()
	tab := a.tab()
	tab.All = []*Item{
		{ID: "1", Name: "a.jpg", Kind: "image", Path: `C:\Pictures\a.jpg`, Size: 10},
		{ID: "2", Name: "b.jpg", Kind: "image", Path: `C:\Pictures\b.jpg`, Size: 10},
		{ID: "3", Name: "c.mp4", Kind: "video", Path: `C:\Pictures\c.mp4`, Size: 10},
		{ID: "4", Name: "sub", Kind: "folder", Path: `C:\Pictures\sub`},
	}
	tab.Items = tab.All
	tab.Result = DirResult{Path: tab.Path, Name: "Pictures", Items: tab.All}
	return a
}

func TestFavoriteCollectsMediaOnly(t *testing.T) {
	a := favoritesApp()
	a.selectOnly("1")
	a.toggleItemFavorites()
	if len(a.settings.Favorites) != 1 {
		t.Fatalf("favorites = %+v", a.settings.Favorites)
	}
	fav := a.settings.Favorites[0]
	if fav.Kind != "image" || !strings.HasSuffix(fav.Path, "a.jpg") {
		t.Fatalf("favorite = %+v", fav)
	}
	// Pressing again removes it.
	a.toggleItemFavorites()
	if len(a.settings.Favorites) != 0 {
		t.Fatalf("second press did not remove: %+v", a.settings.Favorites)
	}
}

func TestFavoriteRefusesFoldersAndOtherFiles(t *testing.T) {
	a := favoritesApp()
	a.selectOnly("4")
	a.toggleItemFavorites()
	if len(a.settings.Favorites) != 0 {
		t.Fatalf("a folder was collected: %+v", a.settings.Favorites)
	}
	// The path-based entry point refuses too, which covers a stray caller.
	a.toggleFavoritePath(`C:\Pictures\notes.txt`)
	a.toggleFavoritePath(`C:\Pictures\sub`)
	if len(a.settings.Favorites) != 0 {
		t.Fatalf("non-media collected: %+v", a.settings.Favorites)
	}
}

func TestFavoriteMultipleSelection(t *testing.T) {
	a := favoritesApp()
	a.setMultiSelect(true)
	a.selectOnly("1")
	a.toggleSelect("2", true)
	a.toggleSelect("3", true)
	a.toggleItemFavorites()
	if len(a.settings.Favorites) != 3 {
		t.Fatalf("favorites = %+v", a.settings.Favorites)
	}
	// A mixed selection adds the missing ones rather than toggling each.
	a.removeFavorite(`C:\Pictures\b.jpg`)
	a.toggleItemFavorites()
	if len(a.settings.Favorites) != 3 || !a.isFavorite(`C:\Pictures\b.jpg`) {
		t.Fatalf("mixed selection = %+v", a.settings.Favorites)
	}
	// With everything collected, the same call clears them.
	a.toggleItemFavorites()
	if len(a.settings.Favorites) != 0 {
		t.Fatalf("all-collected press did not clear: %+v", a.settings.Favorites)
	}
}

// TestFavoritesFolderListsCollectedFiles is the virtual folder itself: it must
// show the collected images and videos as one flat grid, from their real paths.
func TestFavoritesFolderListsCollectedFiles(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "one.png"), 40, 30)
	writePNG(t, filepath.Join(dir, "two.png"), 40, 30)
	other := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(other, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	prev := getSettings()
	settingsMu.Lock()
	settingsData.Favorites = []Favorite{
		{Path: filepath.Join(dir, "one.png"), Name: "one.png", Kind: "image"},
		{Path: filepath.Join(dir, "two.png"), Name: "two.png", Kind: "image"},
		{Path: other, Name: "notes.txt", Kind: "other"},
	}
	settingsMu.Unlock()
	defer func() {
		settingsMu.Lock()
		settingsData = prev
		settingsMu.Unlock()
	}()

	res := (FS{}).Browse(BrowseRequest{Path: favoritesPath, Limit: 100})
	if res.Error != "" {
		t.Fatalf("browse: %s", res.Error)
	}
	if res.Name != favoritesName {
		t.Fatalf("name = %q", res.Name)
	}
	if len(res.Items) != 2 {
		t.Fatalf("items = %+v", itemNames(res.Items))
	}
	// Each item keeps its real path, so thumbnails and the viewer work.
	for _, it := range res.Items {
		if strings.HasPrefix(it.Path, favoritesScheme) {
			t.Fatalf("item path is virtual: %q", it.Path)
		}
	}
	if res.Images != 2 {
		t.Fatalf("counts = %d images", res.Images)
	}
}

func TestFavoritesFolderSkipsMissingFiles(t *testing.T) {
	prev := getSettings()
	settingsMu.Lock()
	settingsData.Favorites = []Favorite{
		{Path: `C:\nowhere\gone.jpg`, Name: "gone.jpg", Kind: "image"},
	}
	settingsMu.Unlock()
	defer func() {
		settingsMu.Lock()
		settingsData = prev
		settingsMu.Unlock()
	}()

	res := (FS{}).Browse(BrowseRequest{Path: favoritesPath, Limit: 100})
	if len(res.Items) != 0 {
		t.Fatalf("a deleted file still listed: %+v", itemNames(res.Items))
	}
}

func TestFavoritesFolderIsVirtualPath(t *testing.T) {
	if !isFavoritesPath(favoritesPath) || !isFavoritesPath("favorites://") || !isFavoritesPath("FAVORITES:") {
		t.Fatal("favorites path not recognised")
	}
	if isFavoritesPath(`C:\favorites`) {
		t.Fatal("a local folder named favorites was treated as virtual")
	}
	if got := cleanDisplayPath(favoritesPath); got != favoritesPath {
		t.Fatalf("cleanDisplayPath = %q", got)
	}
	if got := displayName(favoritesPath); got != favoritesName {
		t.Fatalf("displayName = %q", got)
	}
	// The virtual folder has no parent and no children to walk.
	if res := (FS{}).Parent(favoritesPath); res.Found {
		t.Fatalf("parent = %+v", res)
	}
	if kids := (FS{}).Tree(favoritesPath, false); len(kids) != 0 {
		t.Fatalf("tree = %+v", kids)
	}
}

func TestFavoritesFolderIsReadOnly(t *testing.T) {
	a := favoritesApp()
	tab := a.tab()
	tab.Path = favoritesPath
	tab.Name = favoritesName
	if !a.tabReadOnly() {
		t.Fatal("favorites folder is not read-only")
	}
	if _, err := (AppSvc{}).NewFolder(favoritesPath, "x"); err == nil {
		t.Fatal("new folder succeeded in the favorites folder")
	}
}

// TestFavoritesMigrateFromPathList covers a settings file written before the
// folder became virtual: only media paths survive, since folders are no longer
// collected.
func TestFavoritesMigrateFromPathList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	old := `{"theme":"dark","favorites":["C:\\Pictures\\a.jpg","C:\\Pictures"]}`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	migrateFavorites(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Theme     string     `json:"theme"`
		Favorites []Favorite `json:"favorites"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("migrated document does not parse: %v\n%s", err, data)
	}
	if doc.Theme != "dark" {
		t.Fatalf("migration dropped other fields: %q", doc.Theme)
	}
	if len(doc.Favorites) != 1 || doc.Favorites[0].Kind != "image" {
		t.Fatalf("favorites = %+v", doc.Favorites)
	}
}

func TestParseSettingsAcceptsBothFavoriteShapes(t *testing.T) {
	s, ok := parseSettings([]byte(`{"favorites":["C:\\Pictures\\a.jpg"],"thumbSize":300}`))
	if !ok {
		t.Fatal("old settings failed to parse")
	}
	if s.ThumbSize != 300 || len(s.Favorites) != 1 || s.Favorites[0].Kind != "image" {
		t.Fatalf("old shape = %+v", s.Favorites)
	}
	s, ok = parseSettings([]byte(`{"favorites":[{"path":"C:\\a.jpg","kind":"image"}]}`))
	if !ok || len(s.Favorites) != 1 || s.Favorites[0].Kind != "image" {
		t.Fatalf("new shape = %+v ok=%v", s.Favorites, ok)
	}
	s, ok = parseSettings([]byte(`{"theme":"light"}`))
	if !ok || len(s.Favorites) != 0 {
		t.Fatalf("missing favorites = %+v", s.Favorites)
	}
}

func TestThumbPrefetchThresholdDefaultsTo200(t *testing.T) {
	if got := defaultSettings().ThumbPrefetchMax; got != 200 {
		t.Fatalf("default threshold = %d", got)
	}
	s, ok := parseSettings([]byte(`{"thumbPrefetch":true}`))
	if !ok || s.ThumbPrefetchMax != 200 {
		t.Fatalf("threshold after old document = %d", s.ThumbPrefetchMax)
	}
	s, _ = parseSettings([]byte(`{"thumbPrefetchMax":0}`))
	if s.ThumbPrefetchMax != 0 {
		t.Fatalf("explicit 0 = %d", s.ThumbPrefetchMax)
	}
	s, _ = parseSettings([]byte(`{"thumbPrefetchMax":-5}`))
	if s.ThumbPrefetchMax != 0 {
		t.Fatalf("negative clamp = %d", s.ThumbPrefetchMax)
	}
	// A document written before the key was renamed still loads its value.
	s, _ = parseSettings([]byte(`{"thumbPrefetchMin":50}`))
	if s.ThumbPrefetchMax != 50 {
		t.Fatalf("legacy key = %d", s.ThumbPrefetchMax)
	}
}

// TestPumpThumbPrefetchSkipsBigFolders covers the ceiling: a folder past the
// limit fills only as it scrolls, so the whole-folder fill must not start.
func TestPumpThumbPrefetchSkipsBigFolders(t *testing.T) {
	tab := &Tab{Path: `C:\Pictures`, Name: "Pictures", Loaded: true}
	for i := 0; i < 10; i++ {
		tab.All = append(tab.All, &Item{
			ID: string(rune('a' + i)), Name: "f.jpg", Kind: "image",
			Path: `C:\Pictures\f` + string(rune('a'+i)) + `.jpg`,
		})
	}
	a := newPrefetchTestApp(t, tab)
	a.settings.ThumbPrefetchMax = 5
	a.pumpThumbPrefetch(tab)
	if a.prefetchTab != nil || a.prefetchNext != 0 {
		t.Fatalf("a folder past the limit started a fill: tab=%v next=%d", a.prefetchTab, a.prefetchNext)
	}
}

// TestPumpThumbPrefetchRunsUnderTheCeiling is the other side: a folder within
// the limit fills.
func TestPumpThumbPrefetchRunsUnderTheCeiling(t *testing.T) {
	tab := &Tab{Path: `C:\Pictures`, Name: "Pictures", Loaded: true}
	tab.All = []*Item{{ID: "a", Name: "f.jpg", Kind: "image", Path: `C:\Pictures\f.jpg`}}
	a := newPrefetchTestApp(t, tab)
	a.settings.ThumbPrefetchMax = 5
	// The fill writes to the cache, so it needs a cache folder.
	prev := thumbDir
	thumbDir = t.TempDir()
	t.Cleanup(func() { thumbDir = prev })
	a.pumpThumbPrefetch(tab)
	if a.prefetchTab == nil {
		t.Fatal("a folder within the limit did not start a fill")
	}
}

func TestMultiSelectModeChangesClickBehaviour(t *testing.T) {
	a := favoritesApp()
	tab := a.tab()
	a.selectOnly("1")
	a.toggleSelect("2", false)
	if len(tab.Selected) != 1 || !tab.Selected["2"] {
		t.Fatalf("single select = %v", tab.Selected)
	}
	a.setMultiSelect(true)
	a.toggleSelect("1", true)
	if len(tab.Selected) != 2 {
		t.Fatalf("multi select = %v", tab.Selected)
	}
	a.toggleSelect("1", true)
	if len(tab.Selected) != 1 {
		t.Fatalf("toggle off = %v", tab.Selected)
	}
	a.setMultiSelect(false)
	if len(tab.Selected) != 1 {
		t.Fatalf("leaving multi-select cleared the selection: %v", tab.Selected)
	}
	a.exitMultiSelect()
	if len(tab.Selected) != 0 || tab.MultiSelect {
		t.Fatalf("exit = %v multi=%v", tab.Selected, tab.MultiSelect)
	}
}

func TestSelectKindAndInvert(t *testing.T) {
	a := favoritesApp()
	a.selectKind("image")
	tab := a.tab()
	if len(tab.Selected) != 2 || !tab.Selected["1"] || !tab.Selected["2"] {
		t.Fatalf("images = %v", tab.Selected)
	}
	if !tab.MultiSelect {
		t.Fatal("selectKind did not enter multi-select")
	}
	a.invertSelection()
	if len(tab.Selected) != 2 || !tab.Selected["3"] || !tab.Selected["4"] {
		t.Fatalf("invert = %v", tab.Selected)
	}
}

func TestSelectSameKindUsesAnchor(t *testing.T) {
	a := favoritesApp()
	a.selectOnly("3")
	a.selectSameKind()
	if len(a.tab().Selected) != 1 || !a.tab().Selected["3"] {
		t.Fatalf("same kind = %v", a.tab().Selected)
	}
	a.selectOnly("1")
	a.selectSameKind()
	if len(a.tab().Selected) != 2 || !a.tab().Selected["2"] {
		t.Fatalf("same kind images = %v", a.tab().Selected)
	}
}

func TestSelectMenuListsCounts(t *testing.T) {
	a := favoritesApp()
	a.setMultiSelect(true)
	labels := make([]string, 0, 8)
	for _, e := range a.selectMenuEntries(a.tab()) {
		if !e.divider {
			labels = append(labels, e.label)
		}
	}
	joined := strings.Join(labels, "|")
	// The first row reflects the current state, so it reads as the way out
	// once the mode is on.
	for _, want := range []string{"退出多选", "全选", "选择全部图片 (2)", "选择全部视频 (1)", "选择全部文件夹 (1)", "反向选择", "取消选择"} {
		if !strings.Contains(joined, want) {
			t.Errorf("menu missing %q: %s", want, joined)
		}
	}
	a.setMultiSelect(false)
	labels = labels[:0]
	for _, e := range a.selectMenuEntries(a.tab()) {
		if !e.divider {
			labels = append(labels, e.label)
		}
	}
	if !strings.Contains(strings.Join(labels, "|"), "进入多选") {
		t.Errorf("menu does not offer entering: %v", labels)
	}
}

func TestSelectionBarOnlyInMultiSelect(t *testing.T) {
	a := favoritesApp()
	a.selectOnly("1")
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if tt.HasText("已选 1 项") {
		t.Fatal("single click showed the bar")
	}
	a.setMultiSelect(true)
	tt.Frame()
	if !tt.HasText("已选 1 项") {
		t.Fatalf("multi-select did not show the bar: %q", tt.Texts())
	}
	if !tt.HasText("收藏") {
		t.Fatalf("bar has no favorite action: %q", tt.Texts())
	}
}

func TestSelectionBreakdown(t *testing.T) {
	a := favoritesApp()
	a.setMultiSelect(true)
	a.selectOnly("1")
	a.toggleSelect("2", true)
	a.toggleSelect("3", true)
	if got := selectionBreakdown(a.tab()); got != "2 图片 · 1 视频" {
		t.Fatalf("breakdown = %q", got)
	}
	a.selectOnly("1")
	if got := selectionBreakdown(a.tab()); got != "" {
		t.Fatalf("single selection = %q", got)
	}
}

func TestSidebarShowsFavoritesFolderEntry(t *testing.T) {
	a := favoritesApp()
	a.settings.Sidebar = true
	a.sidebar = true
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if !tt.HasText("收藏") {
		t.Fatalf("favorites section missing: %q", tt.Texts())
	}
	// One row for the virtual folder, not a list of collected files.
	if !tt.HasText(favoritesName) {
		t.Fatalf("favorites entry missing: %q", tt.Texts())
	}
	// The count reflects what the folder would show, so the collected file
	// has to exist: a favorite whose file is gone is not counted.
	dir := t.TempDir()
	real := filepath.Join(dir, "a.jpg")
	writePNG(t, real, 8, 8)
	a.toggleFavoritePath(real)
	tt.Frame()
	if !tt.HasText("收藏夹 (1)") {
		t.Fatalf("count not shown: %q", tt.Texts())
	}
}

func TestSidebarPlacesSectionIsOptional(t *testing.T) {
	a := favoritesApp()
	a.settings.Sidebar = true
	a.sidebar = true
	// A name that appears nowhere else: the breadcrumb also says "当前位置".
	a.places = []Place{{Name: "我的图片库", Path: `C:\Pictures`, Icon: "pictures"}}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if !tt.HasText("我的图片库") {
		t.Fatalf("places section missing: %q", tt.Texts())
	}
	a.settings.ShowPlaces = false
	tt.Frame()
	if tt.HasText("我的图片库") {
		t.Fatalf("places section still shown: %q", tt.Texts())
	}
	if !tt.HasText("收藏") || !tt.HasText("驱动器") {
		t.Fatalf("other sections disappeared: %q", tt.Texts())
	}
}

func TestViewerFavoriteShortcutAndButton(t *testing.T) {
	a := favoritesApp()
	it := a.tab().Items[0]
	a.viewer = Viewer{Open: true, Item: it, Zoom: 1}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if err := tt.Click("收藏 (Space)"); err != nil {
		t.Fatalf("star button: %v", err)
	}
	tt.Frame()
	if !a.isFavorite(it.Path) {
		t.Fatalf("button did not collect: %+v", a.settings.Favorites)
	}
	if !tt.HasText("取消收藏 (Space)") {
		t.Fatalf("star button did not flip: %q", tt.Texts())
	}
	a.toggleViewerFavorite(it)
	if a.isFavorite(it.Path) {
		t.Fatal("shortcut did not remove the favorite")
	}
}

func TestReadOnlyMenuKeepsFavorites(t *testing.T) {
	entries := withoutWriteEntries(true, []gridMenuEntry{
		{label: "打开"},
		{label: "在资源管理器中显示", localOnly: true},
		{label: "收藏"},
		{divider: true},
		{label: "重命名…", write: true},
	})
	labels := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.divider {
			labels = append(labels, e.label)
		}
	}
	if strings.Join(labels, "|") != "打开|收藏" {
		t.Fatalf("read-only menu = %v", labels)
	}
}

func TestFavoritePathOnRemoteShare(t *testing.T) {
	a := favoritesApp()
	remote := "smb://nas/photos/2024/a.jpg"
	a.toggleFavoritePath(remote)
	if !a.isFavorite(remote) {
		t.Fatal("remote file was not collected")
	}
	if a.settings.Favorites[0].Path != remote {
		t.Fatalf("stored path = %q", a.settings.Favorites[0].Path)
	}
	if !a.isFavorite("SMB://NAS/photos/2024/a.jpg") {
		t.Fatal("remote favorite comparison is case sensitive")
	}
}

func TestMultiSelectEndsWhenFolderChanges(t *testing.T) {
	a := favoritesApp()
	a.setMultiSelect(true)
	a.selectOnly("1")
	a.goTo(`C:\Other`, true)
	if a.tab().MultiSelect {
		t.Fatal("multi-select survived a folder change")
	}
	if len(a.tab().Selected) != 0 {
		t.Fatalf("selection survived a folder change: %v", a.tab().Selected)
	}
}

func TestFavoritesSurviveSaveAndReload(t *testing.T) {
	dir := t.TempDir()
	prevPath := settingsPath
	settingsPath = filepath.Join(dir, "settings.json")
	defer func() { settingsPath = prevPath }()

	a := favoritesApp()
	a.toggleFavoritePath(`C:\Pictures\a.jpg`)
	a.toggleFavoritePath(`C:\Pictures\c.mp4`)

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := parseSettings(data)
	if !ok {
		t.Fatalf("saved settings do not parse: %s", data)
	}
	if len(s.Favorites) != 2 {
		t.Fatalf("reloaded %d favorites from %s", len(s.Favorites), data)
	}
}

// TestRemoveFavoriteDoesNotAliasSettings guards a shared backing array: an
// in-place filter used to leave the stored settings holding a longer slice of
// the same array, so a later save resurrected the entry.
func TestRemoveFavoriteDoesNotAliasSettings(t *testing.T) {
	dir := t.TempDir()
	prevPath := settingsPath
	settingsPath = filepath.Join(dir, "settings.json")
	defer func() { settingsPath = prevPath }()

	a := favoritesApp()
	a.toggleFavoritePath(`C:\Pictures\a.jpg`)
	a.toggleFavoritePath(`C:\Pictures\b.jpg`)
	a.toggleFavoritePath(`C:\Pictures\c.mp4`)
	if len(a.settings.Favorites) != 3 {
		t.Fatalf("setup = %+v", a.settings.Favorites)
	}
	a.removeFavorite(`C:\Pictures\b.jpg`)
	if len(a.settings.Favorites) != 2 {
		t.Fatalf("after remove = %+v", a.settings.Favorites)
	}
	for _, f := range a.settings.Favorites {
		if strings.HasSuffix(f.Path, "b.jpg") {
			t.Fatalf("removed favorite still present: %+v", a.settings.Favorites)
		}
	}
	if got := len(getSettings().Favorites); got != 2 {
		t.Fatalf("stored favorites = %d", got)
	}
}

func TestClearFavorites(t *testing.T) {
	a := favoritesApp()
	a.toggleFavoritePath(`C:\Pictures\a.jpg`)
	a.clearFavorites()
	if len(a.settings.Favorites) != 0 {
		t.Fatalf("clear left %+v", a.settings.Favorites)
	}
}

// TestFavoritesFolderReloadsAfterChange covers the one thing that makes a
// virtual folder different from a real one: nothing on disk tells the grid
// that its contents changed, so the app has to reload it itself.
func TestFavoritesFolderReloadsAfterChange(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "one.png"), 40, 30)
	writePNG(t, filepath.Join(dir, "two.png"), 40, 30)

	a := newViewTestApp()
	tab := a.tab()
	tab.Path = favoritesPath
	tab.Name = favoritesName
	// The listing reads settingsData, not the app's copy, so set both.
	prev := getSettings()
	settingsMu.Lock()
	settingsData.Favorites = []Favorite{
		{Path: filepath.Join(dir, "one.png"), Name: "one.png", Kind: "image"},
		{Path: filepath.Join(dir, "two.png"), Name: "two.png", Kind: "image"},
	}
	settingsMu.Unlock()
	a.settings.Favorites = getSettings().Favorites
	defer func() {
		settingsMu.Lock()
		settingsData = prev
		settingsMu.Unlock()
	}()
	// Prime the listing as loadTab would.
	res := (FS{}).Browse(BrowseRequest{Path: favoritesPath, Limit: 100})
	tab.All, tab.Items, tab.Loaded = res.Items, res.Items, true
	if len(tab.Items) != 2 {
		t.Fatalf("setup = %+v", itemNames(tab.Items))
	}

	a.removeFavorite(filepath.Join(dir, "two.png"))
	// loadTab runs the listing on a goroutine; wait for the tab to settle.
	for i := 0; i < 100; i++ {
		if !tab.Loading && len(tab.Items) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(tab.Items) != 1 {
		t.Fatalf("grid did not reload: %+v", itemNames(tab.Items))
	}
}

// TestCtrlClickEntersMultiSelect covers the way most people start a run: they
// hold Ctrl and click a second tile, and the selection bar must appear without
// them having to find a menu first.
func TestCtrlClickEntersMultiSelect(t *testing.T) {
	a := favoritesApp()
	tab := a.tab()
	a.selectOnly("1")
	if tab.MultiSelect {
		t.Fatal("a plain click entered multi-select")
	}
	// What the grid does for Ctrl+click.
	tab.MultiSelect = true
	a.toggleSelect("2", true)
	if !tab.MultiSelect || len(tab.Selected) != 2 {
		t.Fatalf("ctrl+click = %v multi=%v", tab.Selected, tab.MultiSelect)
	}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if !tt.HasText("已选 2 项") {
		t.Fatalf("bar missing after ctrl+click: %q", tt.Texts())
	}
}

func TestShiftArrowExtendsAndKeepsAnchor(t *testing.T) {
	a := favoritesApp()
	tab := a.tab()
	a.selectOnly("1")
	layout := buildWaterfall(tab.Items, 900, 220, 8, false)
	a.extendGridSelection(tab, layout, 1, 0)
	if !tab.MultiSelect {
		t.Fatal("shift+arrow did not enter multi-select")
	}
	if len(tab.Selected) != 2 {
		t.Fatalf("after one step = %v", tab.Selected)
	}
	if tab.Anchor != "1" {
		t.Fatalf("anchor moved to %q", tab.Anchor)
	}
	a.extendGridSelection(tab, layout, 1, 0)
	if len(tab.Selected) != 3 {
		t.Fatalf("after two steps = %v", tab.Selected)
	}
	if tab.Anchor != "1" {
		t.Fatalf("anchor moved to %q", tab.Anchor)
	}
}

// TestSelectionBarHidesSingleItemActions covers the bar's shape: Open and
// Locate act on one tile, so a multi-selection drops them rather than showing
// a row of disabled buttons.
func TestSelectionBarHidesSingleItemActions(t *testing.T) {
	a := favoritesApp()
	a.setMultiSelect(true)
	a.selectOnly("1")
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if !tt.HasText("打开") {
		t.Fatalf("single selection lost Open: %q", tt.Texts())
	}
	a.toggleSelect("2", true)
	tt.Frame()
	if tt.HasText("打开") {
		t.Fatalf("multi-selection still shows Open: %q", tt.Texts())
	}
	if !tt.HasText("已选 2 项") || !tt.HasText("复制路径") {
		t.Fatalf("bar lost its multi actions: %q", tt.Texts())
	}
}

// TestRemoteDialogHTTPSUpdatesPort clicks the real switch and checks the port
// field follows: the earlier version of this code updated a copy of the port
// that the widget had already stopped reading.
func TestRemoteDialogHTTPSUpdatesPort(t *testing.T) {
	a := newViewTestApp()
	a.remoteDialogOpen = true
	a.remoteDraft = remoteShare{Protocol: remoteWebDAV, Host: "nas", Port: 80}
	a.syncRemotePortDraft()
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()

	if err := tt.Click("使用 HTTPS"); err != nil {
		t.Fatalf("click HTTPS: %v", err)
	}
	tt.Frame()
	if !a.remoteDraft.Secure {
		t.Fatalf("switch did not turn on: %+v", a.remoteDraft)
	}
	if got := int(a.remotePortDraft); got != 443 {
		t.Fatalf("port after enabling HTTPS = %d, want 443", got)
	}
	if got := webdavBaseURL(remoteShare{
		Protocol: remoteWebDAV, Host: a.remoteDraft.Host,
		Port: int(a.remotePortDraft), Secure: a.remoteDraft.Secure,
	}); got != "https://nas:443" {
		t.Fatalf("url = %q", got)
	}

	// Turning it back off returns the port to 80.
	if err := tt.Click("使用 HTTPS"); err != nil {
		t.Fatalf("click HTTPS again: %v", err)
	}
	tt.Frame()
	if got := int(a.remotePortDraft); got != 80 {
		t.Fatalf("port after disabling HTTPS = %d, want 80", got)
	}
}

// TestRemoteDialogOpensWithProtocolDefaultPort covers the first frame: opening
// the dialog for WebDAV must show 80, and for SMB 445, rather than whatever the
// previous dialog left behind.
func TestRemoteDialogOpensWithProtocolDefaultPort(t *testing.T) {
	a := newViewTestApp()
	tt := ui.NewTester(a.view, 1280, 900)

	a.openRemoteDialog()
	tt.Frame()
	if got := int(a.remotePortDraft); got != 445 {
		t.Fatalf("SMB dialog port = %d, want 445", got)
	}

	// Switching to WebDAV inside the dialog moves it to 80.
	if err := tt.Click("WebDAV"); err != nil {
		t.Fatalf("click WebDAV: %v", err)
	}
	tt.Frame()
	if got := int(a.remotePortDraft); got != 80 {
		t.Fatalf("WebDAV dialog port = %d, want 80", got)
	}

	// Reopening starts clean instead of keeping the last value.
	a.remoteDialogOpen = false
	a.openRemoteDialog()
	tt.Frame()
	if got := int(a.remotePortDraft); got != 445 {
		t.Fatalf("reopened dialog port = %d, want 445", got)
	}
}

// TestRemoteDialogKeepsTypedPortAcrossProtocolSwitch makes sure the follow-up
// never overwrites a port the user chose.
func TestRemoteDialogKeepsTypedPortAcrossProtocolSwitch(t *testing.T) {
	a := newViewTestApp()
	tt := ui.NewTester(a.view, 1280, 900)
	a.openRemoteDialog()
	tt.Frame()

	a.remotePortDraft = 5006
	tt.Frame()
	if err := tt.Click("WebDAV"); err != nil {
		t.Fatalf("click WebDAV: %v", err)
	}
	tt.Frame()
	if got := int(a.remotePortDraft); got != 5006 {
		t.Fatalf("typed port replaced on protocol switch: %d", got)
	}
	if err := tt.Click("使用 HTTPS"); err != nil {
		t.Fatalf("click HTTPS: %v", err)
	}
	tt.Frame()
	if got := int(a.remotePortDraft); got != 5006 {
		t.Fatalf("typed port replaced on HTTPS switch: %d", got)
	}
}

// TestFavoritesMenuHiddenForFolders covers a command that used to show on every
// tile and then refuse the click: the favourites row now only appears where it
// can act.
func TestFavoritesMenuHiddenForFolders(t *testing.T) {
	a := favoritesApp()
	dir := t.TempDir()
	folder := &Item{ID: "f1", Name: "相册", Path: filepath.Join(dir, "相册"), Kind: "folder"}
	image := &Item{ID: "i1", Name: "a.jpg", Path: filepath.Join(dir, "a.jpg"), Kind: "image"}
	items.byID[folder.ID] = folder
	items.byID[image.ID] = image
	t.Cleanup(func() {
		delete(items.byID, folder.ID)
		delete(items.byID, image.ID)
	})
	tab := a.tab()

	// A folder alone: no favourites row.
	tab.Selected = map[string]bool{folder.ID: true}
	a.contextIDs = []string{folder.ID}
	if hasEntry(a.gridMenuEntries(), "收藏到收藏夹") {
		t.Fatal("favourites row shown for a folder")
	}

	// An image: the row is there.
	tab.Selected = map[string]bool{image.ID: true}
	a.contextIDs = []string{image.ID}
	if !hasEntry(a.gridMenuEntries(), "收藏到收藏夹") {
		t.Fatal("favourites row missing for an image")
	}

	// A folder and an image together: the image makes it apply.
	tab.Selected = map[string]bool{folder.ID: true, image.ID: true}
	a.contextIDs = []string{folder.ID, image.ID}
	if !hasEntry(a.gridMenuEntries(), "收藏到收藏夹") {
		t.Fatal("favourites row missing for a mixed selection")
	}
}

// hasEntry reports whether the menu offers a row with this label.
func hasEntry(entries []gridMenuEntry, label string) bool {
	for _, e := range entries {
		if e.label == label {
			return true
		}
	}
	return false
}

// TestStarredPathsAreSeparateFromTheFavoritesFolder covers the two things the
// 收藏 section holds. The favorites folder collects images and videos into a
// view; the starred paths remember folders. They are separate lists, so
// starring a folder never shows up as a count in the favorites folder.
func TestStarredPathsAreSeparateFromTheFavoritesFolder(t *testing.T) {
	a := favoritesApp()
	dir := t.TempDir()
	folder := filepath.Join(dir, "相册")
	image := filepath.Join(dir, "a.jpg")
	writePNG(t, image, 8, 8)

	a.toggleStarredPath(folder)
	if !a.isStarredPath(folder) {
		t.Fatal("folder path was not starred")
	}
	// The favorites folder is untouched by a starred folder: not its count,
	// not its contents.
	if got := len(a.settings.Favorites); got != 0 {
		t.Fatalf("starring a folder added %d favorites", got)
	}
	if got := len(favoritesBrowse(BrowseRequest{Path: favoritesPath, Limit: 100}).Items); got != 0 {
		t.Fatalf("favorites folder holds %d items after starring a folder", got)
	}

	// Collecting an image does not make it a starred path.
	a.toggleFavoritePath(image)
	if a.isStarredPath(image) {
		t.Fatal("a collected image appeared as a starred path")
	}
	if got := len(a.settings.StarredPaths); got != 1 {
		t.Fatalf("starred paths = %d, want 1", got)
	}
	if got := len(favoritesBrowse(BrowseRequest{Path: favoritesPath, Limit: 100}).Items); got != 1 {
		t.Fatalf("favorites folder holds %d items, want the collected image", got)
	}

	// Unstarring removes only the folder.
	a.toggleStarredPath(folder)
	if a.isStarredPath(folder) {
		t.Fatal("folder still starred after unstarring")
	}
	if !a.isFavorite(image) {
		t.Fatal("unstarring the folder removed the media favorite")
	}
}

// TestSidebarCountIgnoresMissingFiles covers the count the sidebar shows: it
// used to be len(settings.Favorites), so a collected file that had been deleted
// still counted while the folder hid it — the row said "(3)" over two tiles.
func TestSidebarCountIgnoresMissingFiles(t *testing.T) {
	a := newViewTestApp()
	dir := t.TempDir()
	keep := filepath.Join(dir, "keep.jpg")
	writePNG(t, keep, 8, 8)
	gone := filepath.Join(dir, "gone.jpg")

	a.settings.Favorites = []Favorite{
		{Path: keep, Kind: "image"},
		{Path: gone, Kind: "image"},
		{Path: filepath.Join(dir, "notes.txt"), Kind: "other"},
	}
	a.invalidateLiveFavorites()

	if got := len(a.liveFavorites()); got != 1 {
		t.Fatalf("live favorites = %d, want 1", got)
	}

	a.sidebar = true
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if !tt.HasText(favoritesName + " (1)") {
		t.Fatalf("sidebar count is wrong: %q", tt.Texts())
	}

	// The settings keep the missing entry: a drive that is unplugged for an
	// afternoon must not empty the collection.
	if len(a.settings.Favorites) != 3 {
		t.Fatalf("settings were pruned to %d entries", len(a.settings.Favorites))
	}
}
