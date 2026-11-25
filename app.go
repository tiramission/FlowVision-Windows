package main

import (
	"context"
	"fmt"
	"math"
	"image"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	mvideo "github.com/egoist/mygo/ui/video"
	"github.com/oneo-me/onevra/source/components"
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

// Viewer is the media viewer shown inside the host window.
type Viewer struct {
	Open    bool
	Item    *Item
	Index   int
	Loading bool
	Err     string
	Bitmap  *ui.Bitmap
	// Placeholder is borrowed from the grid thumbnail cache. It is drawn while
	// the real preview is still decoding and must never be released here.
	Placeholder *ui.Bitmap
	// BitmapOwned marks a bitmap this viewer created (currently only the 1:1
	// full-resolution image). Preload-cache bitmaps stay owned by that cache.
	BitmapOwned        bool
	FullResReady       bool
	FullResLoading     bool
	FullResUnsupported bool
	OneToOne           bool
	LoadStarted        time.Time
	Notice             viewerNotice
	NoticeKey          string
	NoticeAt           time.Time
	Zoom               float32
	Angle              float32
	PanX               float32
	PanY               float32
	ShowInfo           bool
	Immersive          bool
	ChromeVisible      bool
	ChromeUntil        time.Time
	CursorVisible      bool
	CursorUntil        time.Time
	// CursorX and CursorY track the physical pointer between frames, so a
	// pointer moving over the video layer wakes the cursor even when the
	// video element, rather than the canvas, receives the motion.
	CursorX       int32
	CursorY       int32
	CursorTracked bool
	Info          MediaInfo
	InfoReady     bool
	Video         VideoMeta
	VideoReady    bool
	Gen           int
	Player        *ui.VideoPlayer
	PlayerErr     string
	SeekSeconds   float64
	SeekUntil     time.Time
	DraggingSeek  bool

	HoldActive bool
	HoldButton int
	HoldAt     time.Time
	HoldX      float32
	HoldY      float32
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
	fullscreen      bool

	showSettings    bool
	draft           Settings
	settingsUI      components.SettingsState
	draftThumbSize  float64
	draftGridGap    float64
	draftSidebarW   float64
	draftCacheLimit float64
	draftPreload    float64
	draftTheme      string
	draftSort       string
	cacheInfo       CacheInfo
	focusSearch     bool
	searchHandle    ui.Handle
	scrollActiveTab bool
	tabScroll       ui.ScrollState
	places          []Place
	drives          []Place

	showHelp   bool
	helpFilter int
	showAbout  bool
	errorMsg   string

	bindingCapture     string
	bindingCaptureGen  int
	bindingCaptureNote string

	sideMouseDown  [2]bool
	sideMouseAt    [2]time.Time
	sideMouseHold  [2]bool
	sideMouseTimer [2]*time.Timer
	sideMouseHook  uintptr
	viewerKeysDown map[SideBinding]bool

	updateOpen        bool
	updatePhase       UpdatePhase
	updateVersion     string
	updateNotes       string
	updateDate        time.Time
	updateErr         string
	updateProgress    float64
	updateDownloaded  int64
	updateTotal       int64
	updateAvailable   *mygo.Update
	updateCancel      context.CancelFunc
	updateGen         int
	updateAutoStarted bool

	renameOpen   bool
	renameTarget *Item
	renameDraft  string
	renameErr    string

	newFolderOpen  bool
	newFolderDraft string
	newFolderErr   string

	confirmDeleteOpen bool
	confirmPermanent  bool

	viewer               Viewer
	viewerGen            int
	viewerPreload        map[string]*viewerPreloadEntry
	viewerPreloadLoading map[string]bool
	viewerPreloadDesired map[string]bool
	viewerPreloadQueue   []viewerPreloadJob
	viewerPreloadWorker  int
	viewerPreloadEpoch   int
	viewerPreloadMu      sync.Mutex
	viewerPreloadBytes   int64

	viewerStats       viewerLoadStats
	viewerSlot        *ui.Bitmap
	viewerFullRes     *viewerFullResEntry
	viewerFullResKey  string
	viewerSlowTimer   *time.Timer
	viewerFullTimer   *time.Timer
	viewerNoticeTimer *time.Timer

	hoverTimer *time.Timer
	hoverKey   string
	hoverToken int

	themeDetected bool
	systemDark    bool

	contextIDs   []string
	contextEmpty bool
	contextAtX   float32
	contextAtY   float32
	pending      []func()

	thumbs              map[string]*thumbEntry
	thumbLoad           map[string]bool
	thumbQueue          *thumbQueue
	thumbFrame          uint64
	thumbOnce           sync.Once
	thumbBytes          int64
	thumbClock          int64
	thumbPendingRefresh bool

	treeOpen     map[string]*bool
	treeRootOpen map[string]*bool
	treeLoaded   map[string]bool
	treeLoading  map[string]bool
	treeKids     map[string][]TreeEntry
	treeGen      int

	statusMu sync.Mutex
}

type thumbEntry struct {
	Bmp     *ui.Bitmap
	At      int64
	Bytes   int64
	Preview bool
}

const (
	thumbCacheMax      = 420
	thumbCacheBytesMax = 256 << 20
)

func newApp() *App {
	a := &App{
		settings:             getSettings(),
		sidebar:              getSettings().Sidebar,
		thumbs:               map[string]*thumbEntry{},
		thumbLoad:            map[string]bool{},
		thumbQueue:           newThumbQueue(),
		treeOpen:             map[string]*bool{},
		treeRootOpen:         map[string]*bool{},
		treeLoaded:           map[string]bool{},
		treeLoading:          map[string]bool{},
		treeKids:             map[string][]TreeEntry{},
		viewerPreload:        map[string]*viewerPreloadEntry{},
		viewerPreloadLoading: map[string]bool{},
		viewerPreloadDesired: map[string]bool{},
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
	a.syncWindowMinimum()
	a.refresh()
}

// syncWindowMinimum keeps the native resize floor in step with a visible
// sidebar. Changing the sidebar width must not leave the right workbench below
// the width its tab row needs, and restoring an older window state should not
// resurrect the pre-split 720×460 floor.
func (a *App) syncWindowMinimum() {
	if a.win == nil {
		return
	}
	minW := windowMinWidth(a.sidebar, a.sidebarWidth)
	a.win.SetMinimumSize(minW, minWindowHeight)
	if a.maximized || a.fullscreen {
		return
	}
	b := a.win.Bounds()
	if b.Width > 0 && (b.Width < minW || b.Height < minWindowHeight) {
		a.win.SetSize(max(b.Width, minW), max(b.Height, minWindowHeight))
	}
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
	a.fillSettingsDraft(a.settings)
	a.settingsUI.Query = ""
	a.cacheInfo = thumbCacheInfo()
	a.bindingCapture = ""
	a.bindingCaptureNote = ""
	a.showSettings = true
	a.refresh()
}

// fillSettingsDraft copies persisted settings into the editable draft. Controls
// bind to these retained fields, not to locals rebuilt every frame: a Changed
// notice arrives on the following build pass, when a local would already have
// been reset to the old value.
func (a *App) fillSettingsDraft(s Settings) {
	a.draft = s
	a.draft.Favorites = append([]string(nil), s.Favorites...)
	a.draft.Tabs = append([]TabState(nil), s.Tabs...)
	a.draftTheme = s.Theme
	a.draftSort = s.Sort
	a.draftThumbSize = float64(s.ThumbSize)
	a.draftGridGap = float64(s.GridGap)
	a.draftSidebarW = float64(s.SidebarWidth)
	a.draftCacheLimit = float64(s.CacheLimitMB)
	a.draftPreload = float64(s.ViewerPreload)
}

// commitSettingsDraft writes values owned by controls that need a numeric or
// keyed model back into the Settings copy before range checks and persistence.
func (a *App) commitSettingsDraft() Settings {
	a.draft.Theme = a.draftTheme
	a.draft.Sort = a.draftSort
	a.draft.ThumbSize = int(math.Round(a.draftThumbSize))
	a.draft.GridGap = int(math.Round(a.draftGridGap))
	a.draft.SidebarWidth = int(math.Round(a.draftSidebarW))
	a.draft.CacheLimitMB = int(math.Round(a.draftCacheLimit))
	a.draft.ViewerPreload = int(math.Round(a.draftPreload))
	return a.draft
}

func (a *App) resetSettingsDraft() {
	favorites := append([]string(nil), a.draft.Favorites...)
	tabs := append([]TabState(nil), a.draft.Tabs...)
	active := a.draft.ActiveTab
	lastPath := a.draft.LastPath
	s := defaultSettings()
	s.Favorites = favorites
	s.Tabs = tabs
	s.ActiveTab = active
	s.LastPath = lastPath
	a.fillSettingsDraft(s)
	a.refresh()
}

func (a *App) applySettings() {
	s := mergeSettings(defaultSettings(), a.commitSettingsDraft())
	a.settings = s
	a.sidebar = s.Sidebar
	a.sidebarWidth = s.SidebarWidth
	_ = saveSettings(s)
	a.syncWindowMinimum()
	a.showSettings = false
	a.bindingCapture = ""
	a.bindingCaptureNote = ""
	if a.viewer.Open {
		a.scheduleViewerPreload()
	}
	a.applyTheme()
	if t := a.tab(); t != nil {
		t.Sort = s.Sort
		t.Desc = s.Desc
	}
	a.treeGen++
	a.treeLoaded = map[string]bool{}
	a.treeKids = map[string][]TreeEntry{}
	a.reloadActive()
	// The active tab's sort is part of persisted tab state, not only the
	// defaults in Settings. Write it after the active tab has been updated.
	a.persist()
}

func (a *App) applyTheme() {
	// The headless tester has no application loop for Theme.SetSource to
	// notify; theme selection is a window concern, not a model concern.
	if a.win == nil {
		return
	}
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
	for _, e := range a.thumbs {
		if e != nil && e.Bmp != nil {
			e.Bmp.ReleasePixels()
		}
	}
	a.thumbs = map[string]*thumbEntry{}
	a.thumbLoad = map[string]bool{}
	a.thumbBytes = 0
	if a.thumbQueue != nil {
		a.thumbQueue.reset()
	}
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

func renameName(it *Item, typed string) (string, string) {
	name := strings.TrimSpace(typed)
	// A folder has no extension to preserve; a name containing a dot is
	// still a complete folder name.
	if it == nil || it.Kind == "folder" {
		return name, ""
	}
	oldExt := filepath.Ext(it.Name)
	if oldExt != "" && filepath.Ext(name) == "" {
		name += oldExt
	}
	if oldExt != "" && !strings.EqualFold(filepath.Ext(name), oldExt) {
		return name, "扩展名不能修改"
	}
	return name, ""
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
	var errMsg string
	name, errMsg = renameName(it, name)
	if errMsg != "" {
		a.renameErr = errMsg
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
		a.invalidateTree(filepath.Dir(newPath))
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

func isMediaKind(kind string) bool {
	return kind == "image" || kind == "video"
}

// activateItem is the one place a grid tile is opened: folders navigate,
// images/videos enter the viewer, and other files go to their default app.
func (a *App) activateItem(it *Item) {
	if it == nil {
		return
	}
	switch {
	case it.Kind == "folder":
		a.goTo(it.Path, true)
	case isMediaKind(it.Kind):
		a.openViewer(it)
	default:
		if err := (AppSvc{}).OpenExternal(it.Path); err != nil {
			a.showError(err.Error())
		}
	}
}

func (a *App) openViewer(it *Item) {
	if it == nil {
		return
	}
	if !isMediaKind(it.Kind) {
		// A folder or an "other" tile can arrive through a context menu or
		// the debug command. Never put a non-decodable file in the viewer.
		a.activateItem(it)
		return
	}
	t := a.tab()
	if t == nil {
		return
	}
	a.closeViewer()
	now := time.Now()
	a.viewer = Viewer{
		Open:          true,
		Item:          it,
		Zoom:          1,
		ShowInfo:      a.settings.InfoPanel,
		Immersive:     true,
		CursorVisible: true,
		CursorUntil:   now.Add(viewerCursorIdle),
	}
	a.viewer.Index = t.indexOf(it.ID)
	if x, y, ok := cursorPosition(); ok {
		// Establish the baseline at open, so the first pointer movement after
		// opening is recognized instead of being consumed as the initial sample.
		a.viewer.CursorX, a.viewer.CursorY, a.viewer.CursorTracked = x, y, true
	}
	a.installViewerSideMouseHook()
	a.loadViewerMedia(it)
	a.refresh()
}

func (a *App) loadViewerMedia(it *Item) {
	// Viewer generations must keep increasing across close/open cycles. If
	// Viewer.Gen restarted at 1, a ticker left over from the previous video
	// could mistake the next video for its own and keep running.
	a.viewerGen++
	a.viewer.Gen = a.viewerGen
	gen := a.viewer.Gen
	a.startViewerChromeTicker(gen)
	a.clearViewerHover()
	a.stopViewerLoadTimers()
	a.clearViewerFullRes()
	a.releaseViewerOwnedBitmap()
	a.viewerStats = viewerLoadStats{}
	a.viewer.Loading = true
	a.viewer.Err = ""
	a.viewer.Bitmap = nil
	a.viewer.BitmapOwned = false
	a.viewer.Placeholder = a.viewerPlaceholder(it)
	a.viewer.FullResReady = false
	a.viewer.FullResLoading = false
	a.viewer.FullResUnsupported = false
	a.viewer.OneToOne = false
	a.viewer.Notice = viewerNotice{}
	a.viewer.LoadStarted = time.Now()
	a.viewer.SeekSeconds = 0
	a.viewer.SeekUntil = time.Time{}
	a.viewer.DraggingSeek = false
	a.viewer.Video = VideoMeta{}
	a.viewer.VideoReady = false
	if a.viewer.Player != nil {
		a.viewer.Player.Close()
		a.viewer.Player = nil
	}
	if a.viewer.Placeholder != nil {
		a.viewerStats.PlaceholderHit = true
	}
	a.startViewerSlowNotice(gen, "正在载入大图…", viewerNoticeLoadDelay)
	if it.Kind == "video" {
		// Read the container before Media Foundation opens the file. Some
		// decoders keep an exclusive handle while playing, which would make a
		// later probe fail even though the file itself is valid.
		a.viewer.Video = probeVideoMeta(it.Path)
		a.viewer.VideoReady = true
	}
	if a.viewer.ShowInfo {
		if it.Kind == "video" {
			a.viewer.Info = MediaInfo{Video: a.viewer.Video}
			a.viewer.InfoReady = true
		} else {
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
			a.stopViewerLoadTimers()
			a.viewer.Loading = false
			a.viewer.Placeholder = nil
			a.viewer.Notice = viewerNotice{}
			if err != nil {
				a.viewer.Err = err.Error()
				a.notifyViewerError(err.Error())
			} else {
				a.viewer.Player = player
				player.SetLoop(a.settings.LoopVideo)
				if a.settings.PlayVideo {
					player.Play()
				} else {
					player.Pause()
				}
				a.startVideoTicker(gen)
			}
			a.refresh()
		})
	default:
		if bmp := a.takeViewerPreload(it); bmp != nil {
			a.stopViewerLoadTimers()
			a.viewer.Loading = false
			a.viewer.Bitmap = bmp
			a.viewer.BitmapOwned = true
			a.viewer.Placeholder = nil
			a.viewer.Notice = viewerNotice{}
			a.viewerStats.PreloadHit = true
			a.viewerStats.LoadMs = time.Since(a.viewer.LoadStarted).Milliseconds()
			a.scheduleViewerPreload()
			a.refresh()
			break
		}
		go func() {
			res, err := loadViewerPreviewData(it)
			a.update(func() {
				if a.viewer.Gen != gen || a.viewer.Item != it {
					if res != nil && res.Bmp != nil {
						res.Bmp.ReleasePixels()
					}
					return
				}
				a.stopViewerLoadTimers()
				a.viewer.Loading = false
				a.viewer.Notice = viewerNotice{}
				if err != nil {
					if res != nil && res.Bmp != nil {
						res.Bmp.ReleasePixels()
					}
					a.viewer.Err = err.Error()
					a.notifyViewerError(err.Error())
					a.refresh()
					return
				}
				bitmapMs := res.BitmapMs
				if res.Img != nil {
					bitmapMs = a.attachViewerPreview(res.Img)
				} else {
					a.viewer.Bitmap = res.Bmp
					a.viewer.BitmapOwned = false
				}
				a.viewer.Placeholder = nil
				a.viewerStats.CacheHit = res.CacheHit
				a.viewerStats.DecodeMs = res.DecodeMs
				a.viewerStats.ScaleMs = res.ScaleMs
				a.viewerStats.EncodeMs = res.EncodeMs
				a.viewerStats.BitmapMs = bitmapMs
				a.viewerStats.LoadMs = time.Since(a.viewer.LoadStarted).Milliseconds()
				if res.Bmp != nil {
					a.storeViewerPreload(it, res.Bmp)
				}
				a.scheduleViewerPreload()
				a.refresh()
			})
		}()
	}
}

const (
	viewerChromeEdgeBandDIP = 64
	viewerChromeLinger      = 2500 * time.Millisecond
	viewerCursorIdle        = 2500 * time.Millisecond
)

func (a *App) startViewerChromeTicker(gen int) {
	go func() {
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			alive := true
			mygo.RunOnMain(func() {
				if a.viewer.Gen != gen || !a.viewer.Open {
					alive = false
					return
				}
				if a.win == nil || !a.viewer.Immersive {
					return
				}
				x, y, pointerOK := cursorPosition()
				pointerMoved := false
				needRefresh := false
				if pointerOK {
					// A pointer moving over the video layer may never deliver a
					// canvas input event. Watching the physical position keeps the
					// cursor's own idle clock working there as over image content.
					pointerMoved = viewerCursorMoved(a.viewer.CursorTracked, a.viewer.CursorX, a.viewer.CursorY, x, y)
					a.viewer.CursorX, a.viewer.CursorY = x, y
					a.viewer.CursorTracked = true
					if pointerMoved {
						if a.wakeViewerCursor() {
							needRefresh = true
						}
						// Keep the native pointer in step even when the app already
						// believes it is visible: a video surface can hide it out of
						// band, and only physical movement tells us to restore it.
						if a.viewer.CursorVisible && a.win != nil {
							restoreCursorOverWindow(a.win.NativeHandle())
						}
					}
				}
				now := time.Now()
				inTop, inBottom, edgeOK := false, false, false
				if pointerOK {
					inTop, inBottom, edgeOK = cursorAtWindowVerticalEdge(a.win.NativeHandle(), viewerChromeEdgeBandDIP)
				}
				if edgeOK && (inTop || inBottom) {
					// Keep the bars alive while the pointer rests at either edge.
					// Leaving the edge starts the normal fade timer.
					a.viewer.ChromeUntil = now.Add(viewerChromeLinger)
					if !a.viewer.ChromeVisible {
						a.viewer.ChromeVisible = true
						needRefresh = true
					}
				} else if a.viewer.ChromeVisible && !now.Before(a.viewer.ChromeUntil) {
					a.viewer.ChromeVisible = false
					needRefresh = true
				}
				// Cursor visibility has its own idle timer. It never follows
				// ChromeVisible, so hiding the bars does not hide the pointer.
				if viewerCursorShouldHide(a.viewer.Immersive, a.viewer.CursorVisible, now, a.viewer.CursorUntil) {
					a.viewer.CursorVisible = false
					needRefresh = true
				}
				if needRefresh {
					a.refresh()
				}
			})
			if !alive {
				return
			}
		}
	}()
}

func (a *App) closeViewer() {
	wasOpen := a.viewer.Open
	a.stopViewerSideMouseHook()
	a.stopViewerLoadTimers()
	a.clearViewerHover()
	a.clearViewerFullRes()
	a.releaseViewerOwnedBitmap()
	a.clearViewerPreloadCache()
	a.viewerKeysDown = nil
	if a.viewer.Player != nil {
		a.viewer.Player.Close()
		a.viewer.Player = nil
	}
	a.viewer = Viewer{}
	if wasOpen {
		// A 24MP decode allocates ~100MB in a few short-lived buffers. Go's
		// pacer otherwise keeps that high-water mark for the next open, which
		// looks like a leak across fifty open/close cycles on a 4GB machine.
		// Collect and return the freed pages once the viewer's owner has
		// dropped every bitmap; closing the viewer is already a transition.
		debug.FreeOSMemory()
		a.refresh()
	}
}

func (a *App) attachViewerPreview(img image.Image) int64 {
	start := time.Now()
	if a.viewerSlot == nil {
		a.viewerSlot = ui.NewBitmap(img)
	} else {
		a.viewerSlot.Update(img)
	}
	a.viewer.Bitmap = a.viewerSlot
	// The app, not the viewer, owns the slot; it survives close/open so MyGo
	// keeps one GPU texture instead of one per image for 240 frames.
	a.viewer.BitmapOwned = false
	return time.Since(start).Milliseconds()
}

func (a *App) releaseViewerOwnedBitmap() {
	if a.viewer.BitmapOwned && a.viewer.Bitmap != nil {
		if a.viewerFullRes == nil || a.viewer.Bitmap != a.viewerFullRes.Bmp {
			a.viewer.Bitmap.ReleasePixels()
		}
	}
	a.viewer.Bitmap = nil
	a.viewer.BitmapOwned = false
}

func (a *App) startVideoTicker(gen int) {
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			alive := true
			a.update(func() {
				if a.viewer.Gen != gen || !a.viewer.Open || a.viewer.Player == nil {
					alive = false
					return
				}
				if a.viewer.Player.Playing() {
					a.refresh()
				}
			})
			if !alive {
				return
			}
		}
	}()
}

func (a *App) viewerTogglePlay() {
	p := a.viewer.Player
	if p == nil {
		return
	}
	if p.Playing() {
		p.Pause()
	} else {
		p.Play()
	}
	a.refresh()
}

func (a *App) viewerSeekBy(delta time.Duration) {
	p := a.viewer.Player
	if p == nil {
		return
	}
	pos := p.Position() + delta
	dur := p.Duration()
	if pos < 0 {
		pos = 0
	}
	if dur > 0 && pos > dur {
		pos = dur
	}
	p.Seek(pos)
	a.viewer.SeekSeconds = pos.Seconds()
	a.viewer.SeekUntil = time.Now().Add(500 * time.Millisecond)
	a.refresh()
}

func (a *App) viewerSeekTo(seconds float64) {
	p := a.viewer.Player
	if p == nil {
		return
	}
	dur := p.Duration()
	if seconds < 0 {
		seconds = 0
	}
	if dur > 0 && seconds > dur.Seconds() {
		seconds = dur.Seconds()
	}
	p.Seek(time.Duration(seconds * float64(time.Second)))
	a.viewer.SeekSeconds = seconds
	a.refresh()
}

func (a *App) viewerToggleImmersive() {
	if !a.viewer.Open {
		return
	}
	a.viewer.Immersive = !a.viewer.Immersive
	a.viewer.ChromeVisible = false
	a.viewer.ChromeUntil = time.Time{}
	a.viewer.CursorVisible = true
	a.viewer.CursorUntil = time.Now().Add(viewerCursorIdle)
	a.refresh()
}

// toggleWindowFullscreen changes only the host window. The media viewer's
// immersive state is independent, so closing the viewer can never leave the
// main window in a mode it did not own.
func (a *App) toggleWindowFullscreen() {
	if a.win == nil {
		return
	}
	full := !a.win.IsFullScreen()
	a.win.SetFullScreen(full)
	a.fullscreen = full
	a.maximized = a.win.IsMaximized()
	a.refresh()
}

func (a *App) viewerToggleMute() {
	p := a.viewer.Player
	if p == nil {
		return
	}
	p.SetMuted(!p.Muted())
	a.refresh()
}

var viewerRates = []float64{0.5, 0.75, 1, 1.25, 1.5, 2}

func (a *App) viewerCycleRate() {
	p := a.viewer.Player
	if p == nil {
		return
	}
	cur := p.Rate()
	next := viewerRates[0]
	for i, r := range viewerRates {
		if math.Abs(cur-r) < 0.01 {
			next = viewerRates[(i+1)%len(viewerRates)]
			break
		}
	}
	p.SetRate(next)
	a.refresh()
}

func (a *App) viewerStep(delta int) {
	t := a.tab()
	if t == nil || len(t.Items) == 0 {
		return
	}
	idx := nextViewerIndex(t.Items, a.viewer.Index, delta)
	if idx < 0 {
		return
	}
	it := t.Items[idx]
	a.cancelViewerHold()
	a.viewer.Open = true
	a.viewer.Item = it
	a.viewer.Index = idx
	a.viewer.Zoom = 1
	a.viewer.Angle = 0
	a.viewer.PanX = 0
	a.viewer.PanY = 0
	a.releaseViewerOwnedBitmap()
	a.loadViewerMedia(it)
	a.refresh()
}

// viewerFit fits the current orientation into the canvas without undoing a
// rotation. The long-press right gesture uses it, matching upstream's "switch
// zoom to view".
func (a *App) viewerFit() {
	if a.viewer.Item == nil || a.viewer.Item.Kind != "image" {
		return
	}
	a.viewer.Zoom = 1
	a.viewer.PanX = 0
	a.viewer.PanY = 0
	a.viewer.OneToOne = false
	a.refresh()
}

func (a *App) viewerReset() {
	if a.viewer.Item == nil || a.viewer.Item.Kind != "image" {
		return
	}
	a.viewer.Zoom = 1
	a.viewer.Angle = 0
	a.viewer.PanX = 0
	a.viewer.PanY = 0
	a.viewer.OneToOne = false
	a.refresh()
}

// viewerActualSize sets the image to 100%: one bitmap pixel per display pixel.
// Zoom is stored relative to the fit scale, so it is the reciprocal of that fit.
// Unlike the long-press gesture, the explicit action always recenters the image
// and keeps working after the image has already reached 100%.
func (a *App) viewerActualSize() {
	if a.viewer.Bitmap == nil || a.viewer.Item == nil || a.viewer.Item.Kind != "image" {
		return
	}
	a.requestViewerFullRes(true)
	bw, bh := a.viewer.Bitmap.Size()
	w, h := a.viewerCanvasSize(nil)
	fit := viewerFitScale(bw, bh, a.viewer.Angle, w, h)
	if fit <= 0 {
		return
	}
	a.viewer.Zoom = clampViewerZoom(viewerActualZoom(fit))
	a.viewer.PanX = 0
	a.viewer.PanY = 0
	a.viewer.OneToOne = true
	a.notifyViewer("zoom", "100%", viewerNoticeInfo, 1200*time.Millisecond)
	a.refresh()
}

// viewerActualSizeAt jumps to 100% while keeping the bitmap point under the
// pointer fixed. Once the display is already at 100% or beyond, it leaves the
// image alone so repeated long presses cannot keep enlarging it.
func (a *App) viewerActualSizeAt(x, y, w, h float32) {
	if a.viewer.Bitmap == nil || a.viewer.Item == nil || a.viewer.Item.Kind != "image" {
		return
	}
	a.requestViewerFullRes(true)
	a.viewer.OneToOne = true
	bw, bh := a.viewer.Bitmap.Size()
	if w <= 0 || h <= 0 {
		w, h = a.viewerCanvasSize(nil)
	}
	fit := viewerFitScale(bw, bh, a.viewer.Angle, w, h)
	if fit <= 0 {
		return
	}
	actual := clampViewerZoom(viewerActualZoom(fit))
	if a.viewer.Zoom >= actual {
		return
	}
	if x < 0 || y < 0 {
		x, y = w/2, h/2
	}
	a.viewer.Zoom, a.viewer.PanX, a.viewer.PanY = viewerZoomAtPoint(
		a.viewer.Zoom, a.viewer.PanX, a.viewer.PanY, actual/a.viewer.Zoom, x, y, w, h)
	a.refresh()
}

func (a *App) viewerToggleInfo() {
	a.viewerSetInfo(!a.viewer.ShowInfo)
}

func (a *App) viewerSetInfo(show bool) {
	a.viewer.ShowInfo = show
	a.settings.InfoPanel = show
	_ = saveSettings(a.settings)
	if show && !a.viewer.InfoReady && a.viewer.Item != nil {
		it := a.viewer.Item
		if it.Kind == "video" {
			// Video metadata is cached before the player opens, so opening the
			// panel later must not try to reopen a file held by the decoder.
			a.viewer.Info = MediaInfo{Video: a.viewer.Video}
			a.viewer.InfoReady = true
			a.refresh()
			return
		}
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

func (a *App) viewerCenter() {
	if a.viewer.Item == nil || a.viewer.Item.Kind != "image" {
		return
	}
	a.viewer.PanX = 0
	a.viewer.PanY = 0
	a.refresh()
}

func (a *App) viewerZoom(delta float32) {
	if a.viewer.Item == nil || a.viewer.Item.Kind != "image" {
		return
	}
	before := a.viewer.Zoom
	a.viewer.Zoom = clampViewerZoom(a.viewer.Zoom * delta)
	if a.viewer.Zoom != before {
		a.viewer.OneToOne = false
	}
	a.maybeRequestViewerFullRes()
	a.notifyViewer("zoom", fmt.Sprintf("%d%%", a.viewerPercent(nil)), viewerNoticeInfo, 1200*time.Millisecond)
	a.refresh()
}

func (a *App) viewerZoomAt(delta, x, y, w, h float32) {
	if a.viewer.Item == nil || a.viewer.Item.Kind != "image" {
		return
	}
	before := a.viewer.Zoom
	a.viewer.Zoom, a.viewer.PanX, a.viewer.PanY = viewerZoomAtPoint(
		a.viewer.Zoom, a.viewer.PanX, a.viewer.PanY, delta, x, y, w, h)
	if a.viewer.Zoom != before {
		a.viewer.OneToOne = false
	}
	a.maybeRequestViewerFullRes()
	a.notifyViewer("zoom", fmt.Sprintf("%d%%", a.viewerPercent(nil)), viewerNoticeInfo, 1200*time.Millisecond)
	a.refresh()
}

func (a *App) viewerRotate(delta float32) {
	if a.viewer.Item == nil || a.viewer.Item.Kind != "image" {
		return
	}
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

// ensureThumb queues the thumbnail of an item if it is not in memory yet.
// priority is the card's distance from the viewport center; lower loads first.
// It must be called on the main thread.
func (a *App) ensureThumb(it *Item, size int, priority float64, frame uint64) {
	key := thumbKey(it, size)
	if a.thumbLoad == nil {
		a.thumbLoad = map[string]bool{}
	}
	if e := a.thumbs[key]; e != nil && !e.Preview {
		return
	}
	a.submitThumbTask(key+"|f", key, it, size, priority, frame, thumbStageFinal)
	if e := a.thumbs[key]; e == nil && it.HasThumb {
		previewSize := size
		if previewSize > 160 {
			previewSize = 160
		}
		// Preview is closer to the viewport center than the final decode, so
		// it runs first and paints the card while WIC works on the full image.
		a.submitThumbTask(key+"|p", key, it, previewSize, priority-1, frame, thumbStagePreview)
	}
}

func (a *App) submitThumbTask(taskKey, base string, it *Item, size int, priority float64, frame uint64, stage thumbStage) {
	if a.thumbLoad[taskKey] {
		if a.thumbQueue != nil {
			a.thumbQueue.touch(taskKey, priority, frame)
		}
		return
	}
	if a.thumbQueue == nil {
		a.thumbQueue = newThumbQueue()
	}
	a.thumbLoad[taskKey] = true
	a.startThumbWorkers()
	a.thumbQueue.submit(&thumbTask{
		key:      taskKey,
		base:     base,
		item:     it,
		size:     size,
		priority: priority,
		frame:    frame,
		stage:    stage,
	})
}

func (a *App) startThumbWorkers() {
	a.thumbOnce.Do(func() {
		n := cap(thumbSlots)
		if n < 1 {
			n = 6
		}
		for i := 0; i < n; i++ {
			go a.thumbWorker()
		}
	})
}

// endThumbFrame drops queued loads that the latest grid frame did not mark
// visible. Running loads finish, so at most the workers' current tasks delay
// a newly visible card.
func (a *App) endThumbFrame(frame uint64) {
	if a.thumbQueue == nil {
		return
	}
	for _, key := range a.thumbQueue.retain(frame) {
		delete(a.thumbLoad, key)
	}
}

func (a *App) thumbWorker() {
	for {
		task := a.thumbQueue.next()
		if task == nil {
			return
		}
		var data []byte
		var err error
		if task.stage == thumbStagePreview {
			data, err = embeddedThumbPreview(task.item, task.size)
		} else {
			data, err = getThumb(context.Background(), task.item, task.size)
		}
		var bmp *ui.Bitmap
		if err == nil {
			bmp, err = ui.DecodeBitmap(data)
		}
		if (err != nil || bmp == nil) && task.stage == thumbStageFinal {
			if data, e := brokenThumb(task.item, task.size); e == nil {
				bmp, err = ui.DecodeBitmap(data)
			}
		}
		d := dimInfo{}
		if err == nil && task.stage == thumbStageFinal && task.item.Kind == "image" {
			d = imageDims(task.item.Path, task.item.Mtime)
		}
		a.update(func() {
			delete(a.thumbLoad, task.key)
			a.thumbQueue.done(task.key)
			if err != nil || bmp == nil {
				return
			}
			w, h := bmp.Size()
			a.thumbClock++
			e := &thumbEntry{Bmp: bmp, At: a.thumbClock, Bytes: int64(w * h * 4), Preview: task.stage == thumbStagePreview}
			old := a.thumbs[task.base]
			if old != nil {
				if old.Preview == false && e.Preview {
					// A full image already won; do not replace it with a
					// late preview.
					bmp.ReleasePixels()
					return
				}
				a.thumbBytes -= old.Bytes
				old.Bmp.ReleasePixels()
			}
			a.thumbs[task.base] = e
			a.thumbBytes += e.Bytes
			if d.W > 0 && (task.item.W == 0 || task.item.H == 0) {
				task.item.W, task.item.H, task.item.Orient = d.W, d.H, d.Orient
				task.item.HasThumb = d.Thumb
			}
			a.pruneThumbs()
			a.refreshThumbs()
		})
	}
}

// refreshThumbs coalesces the invalidations from a burst of workers into one
// redraw per pending frame. A 300-image folder otherwise invalidates the
// window hundreds of times while the thumbnails arrive.
func (a *App) refreshThumbs() {
	if a.thumbPendingRefresh {
		return
	}
	a.thumbPendingRefresh = true
	a.later(func() {
		a.thumbPendingRefresh = false
		a.refresh()
	})
}

func (a *App) thumb(it *Item, size int) *ui.Bitmap {
	if e := a.thumbs[thumbKey(it, size)]; e != nil {
		// Mark the card as recently used. The cache is tiny, so a monotonic
		// stamp plus an on-demand scan is cheaper than maintaining an LRU list
		// on every frame the grid paints.
		a.thumbClock++
		e.At = a.thumbClock
		return e.Bmp
	}
	return nil
}

func (a *App) pruneThumbs() {
	for len(a.thumbs) > thumbCacheMax || a.thumbBytes > thumbCacheBytesMax {
		var oldestKey string
		var oldestAt int64 = 1<<63 - 1
		for key, e := range a.thumbs {
			if e != nil && e.At < oldestAt {
				oldestAt, oldestKey = e.At, key
			}
		}
		if oldestKey == "" {
			break
		}
		e := a.thumbs[oldestKey]
		a.thumbBytes -= e.Bytes
		if e.Bmp != nil {
			e.Bmp.ReleasePixels()
		}
		delete(a.thumbs, oldestKey)
	}
}

// treeRootOpenPtr returns the stable expansion flag of the current folder at
// the top of the sidebar folder tree. A newly visited root starts expanded so
// its children are visible without an extra click; collapsing it is remembered
// for that folder until the app exits.
func (a *App) treeRootOpenPtr(path string) *bool {
	p := filepath.Clean(path)
	if a.treeRootOpen == nil {
		a.treeRootOpen = map[string]*bool{}
	}
	if v, ok := a.treeRootOpen[p]; ok {
		return v
	}
	v := true
	a.treeRootOpen[p] = &v
	return &v
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
