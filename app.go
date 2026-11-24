package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	mvideo "github.com/egoist/mygo/ui/video"
	onevra "github.com/oneo-me/onevra/source/theme"
)

// Tab is one folder open in the window. The grid, the search field and the
// scroll position all belong to a tab, so switching tabs restores them.
type Tab struct {
	ID    int
	Path  string
	Name  string
	Sort  string
	Desc  bool
	Query string
	// AppliedQuery is the query the filtered Items were built from. The search
	// field writes Query as the user types; comparing the two catches an edit
	// even when a widget's Changed flag has already been consumed.
	AppliedQuery string
	Scroll       ui.ScrollState
	History      []string
	Loading      bool
	Loaded       bool
	Err          string
	Result       DirResult
	All          []*Item
	Items        []*Item
	Selected     map[string]bool
	Anchor       string
	// pendingSelect names a file to select once the next listing arrives; it is
	// used after a rename, where the grid is still showing the pre-rename rows.
	pendingSelect string
	Gen           int
}

func newTab(id int, path string) *Tab {
	return &Tab{
		ID:       id,
		Path:     path,
		Name:     displayName(path),
		Sort:     "name",
		Selected: map[string]bool{},
	}
}

func (t *Tab) refilter() {
	q := strings.ToLower(strings.TrimSpace(t.Query))
	t.AppliedQuery = t.Query
	if q == "" {
		t.Items = t.All
	} else {
		itemsOut := make([]*Item, 0, len(t.All))
		for _, it := range t.All {
			if strings.Contains(strings.ToLower(it.Name), q) {
				itemsOut = append(itemsOut, it)
			}
		}
		t.Items = itemsOut
	}
	for id := range t.Selected {
		found := false
		for _, it := range t.Items {
			if it.ID == id {
				found = true
				break
			}
		}
		if !found {
			delete(t.Selected, id)
		}
	}
	if t.Anchor != "" && !t.hasID(t.Anchor) {
		t.Anchor = ""
	}
}

func (t *Tab) hasID(id string) bool {
	for _, it := range t.Items {
		if it.ID == id {
			return true
		}
	}
	return false
}

func (t *Tab) indexOf(id string) int {
	for i, it := range t.Items {
		if it.ID == id {
			return i
		}
	}
	return -1
}

func (t *Tab) selectedItems() []*Item {
	out := make([]*Item, 0, len(t.Selected))
	for _, it := range t.Items {
		if t.Selected[it.ID] {
			out = append(out, it)
		}
	}
	return out
}

func (t *Tab) selectedOne() *Item {
	if len(t.Selected) != 1 {
		return nil
	}
	for _, it := range t.Items {
		if t.Selected[it.ID] {
			return it
		}
	}
	return nil
}

// Viewer is the full-window media viewer.
type Viewer struct {
	Open      bool
	Item      *Item
	Index     int
	Loading   bool
	Err       string
	Bitmap    *ui.Bitmap
	Zoom      float32
	Angle     float32
	PanX      float32
	PanY      float32
	ShowInfo  bool
	Info      MediaInfo
	InfoReady bool
	Gen       int
	Player    *ui.VideoPlayer
	PlayerErr string
}

// App holds the state the native view builds from. It is only changed on the
// main thread, or through Window.Update from a goroutine.
type App struct {
	win      *mygo.Window
	settings Settings
	tabs     []*Tab
	active   int
	nextID   int

	sidebar         bool
	sidebarWidth    int
	activeTabHandle ui.Handle
	maximized       bool

	showSettings    bool
	draft           Settings
	cacheInfo       CacheInfo
	focusSearch     bool
	searchHandle    ui.Handle
	scrollActiveTab bool
	places          []Place
	drives          []Place

	showHelp  bool
	showAbout bool
	errorMsg  string

	renameOpen   bool
	renameTarget *Item
	renameDraft  string
	renameErr    string

	newFolderOpen  bool
	newFolderDraft string
	newFolderErr   string

	confirmDeleteOpen bool
	confirmPermanent  bool

	viewer Viewer

	themeDetected bool
	systemDark    bool

	contextIDs   []string
	contextEmpty bool
	contextAtX   float32
	contextAtY   float32
	pending      []func()

	thumbs     map[string]*thumbEntry
	thumbLoad  map[string]bool
	thumbOrder []string

	treeOpen    map[string]*bool
	treeLoaded  map[string]bool
	treeLoading map[string]bool
	treeKids    map[string][]TreeEntry
	treeGen     int

	statusMu sync.Mutex
}

type thumbEntry struct {
	Bmp *ui.Bitmap
	At  int64
}

const thumbCacheMax = 420

func newApp() *App {
	a := &App{
		settings:    getSettings(),
		sidebar:     getSettings().Sidebar,
		thumbs:      map[string]*thumbEntry{},
		thumbLoad:   map[string]bool{},
		treeOpen:    map[string]*bool{},
		treeLoaded:  map[string]bool{},
		treeLoading: map[string]bool{},
		treeKids:    map[string][]TreeEntry{},
	}
	a.sidebarWidth = a.settings.SidebarWidth
	a.places = (FS{}).Places()
	a.drives = (FS{}).Drives()
	a.restoreTabs()
	return a
}

func (a *App) restoreTabs() {
	s := a.settings
	for _, st := range s.Tabs {
		if st.Path == "" {
			continue
		}
		if stt, err := os.Stat(st.Path); err != nil || !stt.IsDir() {
			continue
		}
		t := newTab(a.nextID, st.Path)
		a.nextID++
		if st.Sort != "" {
			t.Sort = st.Sort
		}
		t.Desc = st.Desc
		if st.Scroll > 0 {
			t.Scroll.Y = float32(st.Scroll)
		}
		a.tabs = append(a.tabs, t)
		if len(a.tabs) > 12 {
			break
		}
	}
	if len(a.tabs) == 0 {
		a.tabs = append(a.tabs, newTab(a.nextID, startPath))
		a.nextID++
	}
	a.active = s.ActiveTab
	if a.active < 0 || a.active >= len(a.tabs) {
		a.active = 0
	}
}

func (a *App) attach(win *mygo.Window) {
	a.win = win
	a.loadTab(a.tabs[a.active], false)
}

func (a *App) tab() *Tab {
	if a.active < 0 || a.active >= len(a.tabs) {
		return nil
	}
	return a.tabs[a.active]
}

func (a *App) refresh() {
	if a.win != nil {
		a.win.Invalidate()
	}
}

// later runs a command after the current build pass, so a menu or a row can
// change the collection it was built from without interrupting the loop.
func (a *App) later(fn func()) {
	a.pending = append(a.pending, fn)
}

func (a *App) runPending() {
	if len(a.pending) == 0 {
		return
	}
	jobs := a.pending
	a.pending = nil
	for _, fn := range jobs {
		fn()
	}
}

func (a *App) update(fn func()) {
	if a.win == nil {
		fn()
		return
	}
	a.win.Update(fn)
}

// loadTab lists a folder in the background and applies the result on the main
// thread. Generation numbers keep a late answer from an old folder from
// overwriting a newer one.
func (a *App) loadTab(t *Tab, keepScroll bool) {
	if t == nil {
		return
	}
	t.Gen++
	gen := t.Gen
	t.Loading = true
	t.Err = ""
	path := t.Path
	req := BrowseRequest{
		Path:       path,
		Recursive:  a.settings.Recursive,
		Sort:       t.Sort,
		Desc:       t.Desc,
		ShowHidden: a.settings.ShowHidden,
		ShowOther:  a.settings.ShowOther,
		Limit:      6000,
	}
	go func() {
		res := (FS{}).Browse(req)
		a.update(func() {
			if t.Gen != gen {
				return
			}
			t.Loading = false
			t.Loaded = true
			if res.Error != "" {
				t.Err = res.Error
				t.All, t.Items = nil, nil
				t.pendingSelect = ""
				a.refresh()
				return
			}
			t.Path = res.Path
			t.Name = res.Name
			t.Result = res
			t.All = res.Items
			t.refilter()
			if t.pendingSelect != "" {
				for _, cand := range t.Items {
					if strings.EqualFold(cand.Name, t.pendingSelect) {
						t.Selected = map[string]bool{cand.ID: true}
						t.Anchor = cand.ID
						break
					}
				}
				t.pendingSelect = ""
			}
			if !keepScroll {
				// A new folder starts at the top, unless a restored tab had a
				// saved offset for this path.
				if t.Scroll.Y < 0 {
					t.Scroll.Y = 0
				}
			}
			if a.win != nil {
				a.win.SetTitle(t.Name + " — FlowVision")
			}
			a.persist()
			a.refresh()
		})
	}()
}

func (a *App) reloadActive() {
	a.loadTab(a.tab(), false)
}

func (a *App) activateTab(i int) {
	if i < 0 || i >= len(a.tabs) {
		return
	}
	a.active = i
	a.scrollActiveTab = true
	t := a.tabs[i]
	if !t.Loaded && !t.Loading {
		a.loadTab(t, false)
	} else if t.Loaded {
		if a.win != nil {
			a.win.SetTitle(t.Name + " — FlowVision")
		}
	}
	a.persist()
	a.refresh()
}

func (a *App) newTab(path string) {
	if path == "" {
		path = startPath
	}
	t := newTab(a.nextID, path)
	a.nextID++
	t.Sort = a.settings.Sort
	t.Desc = a.settings.Desc
	a.tabs = append(a.tabs, t)
	a.active = len(a.tabs) - 1
	a.scrollActiveTab = true
	a.loadTab(t, false)
	a.persist()
}

func (a *App) closeTab(i int) {
	if i < 0 || i >= len(a.tabs) {
		return
	}
	if len(a.tabs) == 1 {
		// Keep one tab alive; closing the last one returns to the Pictures
		// folder rather than quitting under the user.
		a.goTo(resolveStartPath(), false)
		return
	}
	a.tabs = append(a.tabs[:i], a.tabs[i+1:]...)
	if a.active >= len(a.tabs) {
		a.active = len(a.tabs) - 1
	} else if a.active > i {
		a.active--
	}
	if a.active < 0 {
		a.active = 0
	}
	a.scrollActiveTab = true
	if t := a.tab(); t != nil && !t.Loaded {
		a.loadTab(t, false)
	}
	a.persist()
	a.refresh()
}

func (a *App) goTo(path string, pushHistory bool) {
	t := a.tab()
	if t == nil || path == "" {
		return
	}
	path = filepath.Clean(path)
	if strings.EqualFold(path, t.Path) && t.Loaded {
		return
	}
	if pushHistory && t.Path != "" {
		t.History = append(t.History, t.Path)
		if len(t.History) > 40 {
			t.History = t.History[len(t.History)-40:]
		}
	}
	t.Path = path
	t.Name = displayName(path)
	t.Selected = map[string]bool{}
	t.Anchor = ""
	t.pendingSelect = ""
	t.Scroll.Y = 0
	a.loadTab(t, false)
}

func (a *App) goBack() {
	t := a.tab()
	if t == nil || len(t.History) == 0 {
		return
	}
	path := t.History[len(t.History)-1]
	t.History = t.History[:len(t.History)-1]
	t.Path = path
	t.Name = displayName(path)
	t.Selected = map[string]bool{}
	t.pendingSelect = ""
	t.Scroll.Y = 0
	a.loadTab(t, false)
}

func (a *App) up() {
	t := a.tab()
	if t == nil {
		return
	}
	res := (FS{}).Parent(t.Path)
	if res.Found && !strings.EqualFold(res.Path, t.Path) {
		a.goTo(res.Path, true)
	}
}

func (a *App) sibling(dir int) {
	t := a.tab()
	if t == nil {
		return
	}
	res := (FS{}).Sibling(t.Path, dir, a.settings.ShowHidden)
	if res.Found && res.Path != "" && !strings.EqualFold(res.Path, t.Path) {
		a.goTo(res.Path, true)
	}
}

func (a *App) openPathArgument(path string) {
	path = strings.Trim(path, `"`)
	st, err := os.Stat(path)
	if err != nil {
		path = strings.TrimSpace(path)
		st, err = os.Stat(path)
	}
	if err != nil {
		return
	}
	want := path
	var selectName string
	if !st.IsDir() {
		want = filepath.Dir(path)
		selectName = filepath.Base(path)
	}
	want = filepath.Clean(want)
	for i, t := range a.tabs {
		if strings.EqualFold(t.Path, want) {
			a.active = i
			if selectName != "" {
				if t.Loaded {
					t.Selected = map[string]bool{}
					for _, it := range t.Items {
						if strings.EqualFold(it.Name, selectName) {
							t.Selected[it.ID] = true
							t.Anchor = it.ID
							break
						}
					}
				} else {
					t.pendingSelect = selectName
				}
			}
			a.refresh()
			return
		}
	}
	a.newTab(want)
	if selectName != "" {
		if t := a.tab(); t != nil {
			t.pendingSelect = selectName
		}
	}
}

func (a *App) pickFolder() {
	start := ""
	if t := a.tab(); t != nil {
		start = t.Path
	}
	go func() {
		path, err := (AppSvc{}).PickFolder(start)
		if err != nil || path == "" {
			return
		}
		a.update(func() { a.goTo(path, true) })
	}()
}

func (a *App) revealSelected() {
	t := a.tab()
	if t == nil {
		return
	}
	if it := t.selectedOne(); it != nil {
		_ = (AppSvc{}).Reveal(it.Path)
		return
	}
	_ = (AppSvc{}).OpenFolder(t.Path)
}

func (a *App) openSelectedExternal() {
	if it := a.tab().selectedOne(); it != nil {
		_ = (AppSvc{}).OpenExternal(it.Path)
	}
}

func (a *App) copySelectedPath() {
	if it := a.tab().selectedOne(); it != nil {
		_ = (AppSvc{}).CopyText(it.Path)
	}
}

func (a *App) toggleFavorite() {
	t := a.tab()
	if t == nil {
		return
	}
	p := strings.ToLower(filepath.Clean(t.Path))
	found := -1
	for i, f := range a.settings.Favorites {
		if strings.ToLower(filepath.Clean(f)) == p {
			found = i
			break
		}
	}
	if found >= 0 {
		a.settings.Favorites = append(a.settings.Favorites[:found], a.settings.Favorites[found+1:]...)
	} else {
		a.settings.Favorites = append(a.settings.Favorites, t.Path)
	}
	_ = saveSettings(a.settings)
	a.refresh()
}

func (a *App) removeFavorite(path string) {
	out := a.settings.Favorites[:0]
	for _, f := range a.settings.Favorites {
		if !strings.EqualFold(filepath.Clean(f), filepath.Clean(path)) {
			out = append(out, f)
		}
	}
	a.settings.Favorites = out
	_ = saveSettings(a.settings)
	a.refresh()
}

func (a *App) isFavorite(path string) bool {
	for _, f := range a.settings.Favorites {
		if strings.EqualFold(filepath.Clean(f), filepath.Clean(path)) {
			return true
		}
	}
	return false
}

func (a *App) toggleSidebar() {
	a.sidebar = !a.sidebar
	a.settings.Sidebar = a.sidebar
	_ = saveSettings(a.settings)
	a.refresh()
}

func (a *App) toggleRecursive() {
	a.settings.Recursive = !a.settings.Recursive
	_ = saveSettings(a.settings)
	a.reloadActive()
}

func (a *App) toggleHidden() {
	a.settings.ShowHidden = !a.settings.ShowHidden
	_ = saveSettings(a.settings)
	// Folder trees depend on hidden entries; start them over.
	a.treeGen++
	a.treeLoaded = map[string]bool{}
	a.treeKids = map[string][]TreeEntry{}
	a.reloadActive()
}

func (a *App) setThumbSize(delta int) {
	n := a.settings.ThumbSize + delta
	if n < 96 {
		n = 96
	}
	if n > 480 {
		n = 480
	}
	a.settings.ThumbSize = n
	_ = saveSettings(a.settings)
	a.refresh()
}

func (a *App) openSettings() {
	a.draft = a.settings
	a.draft.Favorites = append([]string(nil), a.settings.Favorites...)
	a.draft.Tabs = append([]TabState(nil), a.settings.Tabs...)
	a.cacheInfo = thumbCacheInfo()
	a.showSettings = true
	a.refresh()
}

func (a *App) applySettings() {
	s := mergeSettings(defaultSettings(), a.draft)
	a.settings = s
	a.sidebar = s.Sidebar
	a.sidebarWidth = s.SidebarWidth
	_ = saveSettings(s)
	a.showSettings = false
	a.applyTheme()
	if t := a.tab(); t != nil {
		t.Sort = s.Sort
		t.Desc = s.Desc
	}
	a.treeGen++
	a.treeLoaded = map[string]bool{}
	a.treeKids = map[string][]TreeEntry{}
	a.reloadActive()
}

func (a *App) applyTheme() {
	var src mygo.ThemeSource
	switch a.settings.Theme {
	case "light":
		src = mygo.ThemeLight
	case "dark":
		src = mygo.ThemeDark
	default:
		src = mygo.ThemeSystem
	}
	mygo.Theme.SetSource(src)
	a.refresh()
}

// uiTheme picks the native widget theme. It follows the desktop's accent and
// accessibility preferences while honoring the app's light/dark choice.
// Onevra's warm neutral palette and terracotta accent replace MyGo's defaults;
// the palette is applied before any widget is built so controls share it.
func (a *App) uiTheme(c *ui.Context) *ui.Theme {
	if !a.themeDetected {
		a.systemDark = c.Theme().Dark
		a.themeDetected = true
	}
	dark := a.systemDark
	switch a.settings.Theme {
	case "dark":
		dark = true
	case "light":
		dark = false
	}
	t := onevra.Palette(dark)
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
	return t
}

func (a *App) clearCache() {
	_, _ = clearThumbCache()
	a.cacheInfo = thumbCacheInfo()
	a.thumbs = map[string]*thumbEntry{}
	a.thumbLoad = map[string]bool{}
	a.thumbOrder = nil
	a.refresh()
}

func (a *App) beginRename() {
	it := a.tab().selectedOne()
	if it == nil {
		return
	}
	a.renameTarget = it
	a.renameDraft = it.Name
	a.renameErr = ""
	a.renameOpen = true
	a.refresh()
}

func (a *App) commitRename() {
	it := a.renameTarget
	if it == nil {
		a.renameOpen = false
		return
	}
	name := strings.TrimSpace(a.renameDraft)
	if name == "" || name == it.Name {
		a.renameOpen = false
		a.refresh()
		return
	}
	// Keep the extension unless the user typed one intentionally.
	oldExt := filepath.Ext(it.Name)
	if oldExt != "" && filepath.Ext(name) == "" {
		name += oldExt
	}
	if oldExt != "" && !strings.EqualFold(filepath.Ext(name), oldExt) {
		a.renameErr = "扩展名不能修改"
		a.refresh()
		return
	}
	newPath, err := (AppSvc{}).Rename(it.Path, name)
	if err != nil {
		a.renameErr = err.Error()
		a.refresh()
		return
	}
	a.renameOpen = false
	a.renameErr = ""
	// Select the renamed file once the new listing arrives.
	newName := filepath.Base(newPath)
	if t := a.tab(); t != nil {
		t.pendingSelect = newName
	}
	a.reloadActive()
}

func (a *App) beginNewFolder() {
	a.newFolderOpen = true
	a.newFolderDraft = "新建文件夹"
	a.newFolderErr = ""
	a.refresh()
}

func (a *App) commitNewFolder() {
	t := a.tab()
	if t == nil {
		a.newFolderOpen = false
		return
	}
	_, err := (AppSvc{}).NewFolder(t.Path, a.newFolderDraft)
	if err != nil {
		a.newFolderErr = err.Error()
		a.refresh()
		return
	}
	a.newFolderOpen = false
	a.newFolderErr = ""
	a.invalidateTree(t.Path)
	a.reloadActive()
}

func (a *App) askDelete(permanent bool) {
	if len(a.tab().Selected) == 0 {
		return
	}
	a.confirmPermanent = permanent
	a.confirmDeleteOpen = true
	a.refresh()
}

func (a *App) doDelete() {
	t := a.tab()
	if t == nil {
		a.confirmDeleteOpen = false
		return
	}
	permanent := a.confirmPermanent
	ids := make([]string, 0, len(t.Selected))
	for id := range t.Selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	failed := []string{}
	for _, id := range ids {
		it, ok := items.Get(id)
		if !ok {
			continue
		}
		if err := (AppSvc{}).Delete(it.Path, permanent); err != nil {
			failed = append(failed, it.Name+": "+err.Error())
		}
	}
	a.confirmDeleteOpen = false
	a.invalidateTree(t.Path)
	a.reloadActive()
	if len(failed) > 0 {
		a.errorMsg = strings.Join(failed, "\n")
	}
}

func (a *App) invalidateTree(dir string) {
	a.treeGen++
	a.treeLoaded[filepath.Clean(dir)] = false
	delete(a.treeKids, filepath.Clean(dir))
	invalidateMedia(dir)
	a.refresh()
}

func (a *App) selectOnly(id string) {
	t := a.tab()
	if t == nil {
		return
	}
	t.Selected = map[string]bool{id: true}
	t.Anchor = id
	a.refresh()
}

func (a *App) toggleSelect(id string, add bool) {
	t := a.tab()
	if t == nil {
		return
	}
	if add {
		if t.Selected[id] {
			delete(t.Selected, id)
		} else {
			t.Selected[id] = true
		}
		t.Anchor = id
	} else {
		t.Selected = map[string]bool{id: true}
		t.Anchor = id
	}
	a.refresh()
}

func (a *App) selectRange(toID string) {
	t := a.tab()
	if t == nil {
		return
	}
	to := t.indexOf(toID)
	from := t.indexOf(t.Anchor)
	if from < 0 || to < 0 {
		t.Selected = map[string]bool{toID: true}
		t.Anchor = toID
		a.refresh()
		return
	}
	if from > to {
		from, to = to, from
	}
	t.Selected = map[string]bool{}
	for i := from; i <= to; i++ {
		t.Selected[t.Items[i].ID] = true
	}
	a.refresh()
}

func (a *App) selectAll() {
	t := a.tab()
	if t == nil {
		return
	}
	for _, it := range t.Items {
		t.Selected[it.ID] = true
	}
	if len(t.Items) > 0 {
		t.Anchor = t.Items[0].ID
	}
	a.refresh()
}

func (a *App) clearSelection() {
	if t := a.tab(); t != nil {
		t.Selected = map[string]bool{}
	}
	a.refresh()
}

func (a *App) showError(msg string) {
	a.errorMsg = msg
	a.refresh()
}

func (a *App) openViewer(it *Item) {
	t := a.tab()
	if t == nil || it == nil {
		return
	}
	a.closeViewer()
	a.viewer = Viewer{Open: true, Item: it, Zoom: 1, ShowInfo: a.settings.InfoPanel}
	a.viewer.Index = t.indexOf(it.ID)
	a.loadViewerMedia(it)
	a.refresh()
}

func (a *App) loadViewerMedia(it *Item) {
	a.viewer.Gen++
	gen := a.viewer.Gen
	a.viewer.Loading = true
	a.viewer.Err = ""
	a.viewer.Bitmap = nil
	if a.viewer.Player != nil {
		a.viewer.Player.Close()
		a.viewer.Player = nil
	}
	if a.viewer.ShowInfo {
		a.viewer.InfoReady = false
		go func() {
			info := (MediaSvc{}).Info(it.ID)
			a.update(func() {
				if a.viewer.Gen != gen || a.viewer.Item != it {
					return
				}
				a.viewer.Info = info
				a.viewer.InfoReady = true
				a.refresh()
			})
		}()
	}
	switch it.Kind {
	case "video":
		player, err := mvideo.Open(it.Path)
		a.update(func() {
			if a.viewer.Gen != gen || a.viewer.Item != it {
				if player != nil {
					player.Close()
				}
				return
			}
			a.viewer.Loading = false
			if err != nil {
				a.viewer.Err = err.Error()
			} else {
				a.viewer.Player = player
				player.SetLoop(a.settings.LoopVideo)
			}
			a.refresh()
		})
	default:
		go func() {
			data, err := previewBytes(it)
			if err != nil {
				if raw, rerr := os.ReadFile(it.Path); rerr == nil {
					data, err = raw, nil
				}
			}
			var bmp *ui.Bitmap
			if err == nil {
				bmp, err = ui.DecodeBitmap(data)
			}
			a.update(func() {
				if a.viewer.Gen != gen || a.viewer.Item != it {
					return
				}
				a.viewer.Loading = false
				if err != nil {
					a.viewer.Err = err.Error()
				} else {
					a.viewer.Bitmap = bmp
				}
				a.refresh()
			})
		}()
	}
}

func (a *App) closeViewer() {
	if a.viewer.Player != nil {
		a.viewer.Player.Close()
		a.viewer.Player = nil
	}
	a.viewer = Viewer{}
	a.refresh()
}

func (a *App) viewerStep(delta int) {
	t := a.tab()
	if t == nil || len(t.Items) == 0 {
		return
	}
	idx := a.viewer.Index + delta
	if idx < 0 {
		idx = 0
	}
	if idx >= len(t.Items) {
		idx = len(t.Items) - 1
	}
	if idx == a.viewer.Index && a.viewer.Item == t.Items[idx] {
		return
	}
	it := t.Items[idx]
	a.viewer.Open = true
	a.viewer.Item = it
	a.viewer.Index = idx
	a.viewer.Zoom = 1
	a.viewer.Angle = 0
	a.viewer.PanX = 0
	a.viewer.PanY = 0
	a.viewer.Bitmap = nil
	a.loadViewerMedia(it)
	a.refresh()
}

func (a *App) viewerReset() {
	a.viewer.Zoom = 1
	a.viewer.Angle = 0
	a.viewer.PanX = 0
	a.viewer.PanY = 0
	a.refresh()
}

func (a *App) viewerToggleInfo() {
	a.viewer.ShowInfo = !a.viewer.ShowInfo
	a.settings.InfoPanel = a.viewer.ShowInfo
	_ = saveSettings(a.settings)
	if a.viewer.ShowInfo && !a.viewer.InfoReady && a.viewer.Item != nil {
		it := a.viewer.Item
		gen := a.viewer.Gen
		a.viewer.InfoReady = false
		go func() {
			info := (MediaSvc{}).Info(it.ID)
			a.update(func() {
				if a.viewer.Gen != gen || a.viewer.Item != it {
					return
				}
				a.viewer.Info = info
				a.viewer.InfoReady = true
				a.refresh()
			})
		}()
	}
	a.refresh()
}

func (a *App) viewerZoom(delta float32) {
	z := a.viewer.Zoom * delta
	if z < 0.15 {
		z = 0.15
	}
	if z > 12 {
		z = 12
	}
	a.viewer.Zoom = z
	a.refresh()
}

func (a *App) viewerRotate(delta float32) {
	a.viewer.Angle += delta
	a.refresh()
}

func (a *App) persist() {
	if len(a.tabs) == 0 {
		return
	}
	tabs := make([]TabState, 0, len(a.tabs))
	for _, t := range a.tabs {
		if t.Path == "" {
			continue
		}
		tabs = append(tabs, TabState{Path: t.Path, Sort: t.Sort, Desc: t.Desc, Scroll: float64(t.Scroll.Y)})
	}
	a.settings.Tabs = tabs
	a.settings.ActiveTab = a.active
	if t := a.tab(); t != nil {
		a.settings.LastPath = t.Path
		a.settings.Sort = t.Sort
		a.settings.Desc = t.Desc
	}
	a.settings.Sidebar = a.sidebar
	a.settings.SidebarWidth = a.sidebarWidth
	_ = saveSettings(a.settings)
}

// thumbKey includes the source version and the requested bucket, so a file
// replaced in place never shows an old picture.
func thumbKey(it *Item, size int) string {
	return fmt.Sprintf("%s-%d-%d-%d", it.ID, thumbBudget(size), it.Mtime, it.Size)
}

// ensureThumb loads the thumbnail of an item if it is not in memory yet.
// It must be called on the main thread.
func (a *App) ensureThumb(it *Item, size int) {
	key := thumbKey(it, size)
	if _, ok := a.thumbs[key]; ok || a.thumbLoad[key] {
		return
	}
	a.thumbLoad[key] = true
	itCopy := *it
	go func() {
		data, err := getThumb(context.Background(), &itCopy, size)
		var bmp *ui.Bitmap
		if err == nil {
			bmp, err = ui.DecodeBitmap(data)
		}
		a.update(func() {
			delete(a.thumbLoad, key)
			if err != nil || bmp == nil {
				return
			}
			a.thumbs[key] = &thumbEntry{Bmp: bmp, At: nowUnix()}
			a.thumbOrder = append(a.thumbOrder, key)
			a.pruneThumbs()
			a.refresh()
		})
	}()
}

func (a *App) thumb(it *Item, size int) *ui.Bitmap {
	if e := a.thumbs[thumbKey(it, size)]; e != nil {
		return e.Bmp
	}
	return nil
}

func (a *App) pruneThumbs() {
	for len(a.thumbOrder) > thumbCacheMax {
		key := a.thumbOrder[0]
		a.thumbOrder = a.thumbOrder[1:]
		if e := a.thumbs[key]; e != nil {
			e.Bmp.ReleasePixels()
			delete(a.thumbs, key)
		}
	}
}

// treeOpenPtr returns the stable open flag of a folder in the sidebar tree.
func (a *App) treeOpenPtr(path string) *bool {
	p := filepath.Clean(path)
	if v, ok := a.treeOpen[p]; ok {
		return v
	}
	v := false
	a.treeOpen[p] = &v
	return &v
}

func (a *App) ensureTree(path string) {
	p := filepath.Clean(path)
	if a.treeLoaded[p] || a.treeLoading[p] {
		return
	}
	a.treeLoading[p] = true
	gen := a.treeGen
	hidden := a.settings.ShowHidden
	go func() {
		kids := (FS{}).Tree(p, hidden)
		a.update(func() {
			delete(a.treeLoading, p)
			if a.treeGen != gen {
				return
			}
			a.treeLoaded[p] = true
			a.treeKids[p] = kids
			a.refresh()
		})
	}()
}
