package main

import (
	"context"
	"fmt"
	"image"
	"math"
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
)

// Tab is one folder open in the window. The grid, the search field and the
// scroll position all belong to a tab, so switching tabs restores them.
type Tab struct {
	ID   int
	Path string
	Name string
	Sort string
	Desc bool
	// Filter narrows the grid to images or videos while keeping folders
	// navigable. "" and "all" show every kind.
	Filter string
	Query  string
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
	// MultiSelect turns a plain click into a toggle. The grid starts in
	// single-select mode so clicking a photo never leaves a selection bar
	// hovering over the folder; multi-select is entered from the context menu.
	MultiSelect bool
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
		Filter:   "all",
		Selected: map[string]bool{},
	}
}

func (t *Tab) refilter() {
	q := strings.ToLower(strings.TrimSpace(t.Query))
	t.AppliedQuery = t.Query
	mediaOnly := t.Filter == "image" || t.Filter == "video"
	if q == "" && !mediaOnly {
		t.Items = t.All
	} else {
		itemsOut := make([]*Item, 0, len(t.All))
		for _, it := range t.All {
			if q != "" && !strings.Contains(strings.ToLower(it.Name), q) {
				continue
			}
			if !mediaFilterMatch(it, t.Filter) {
				continue
			}
			itemsOut = append(itemsOut, it)
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
	// Peeking marks a long press that jumped to 100% only while it is held,
	// with the view it came from saved here to return to on release.
	Peeking                      bool
	PeekZoom, PeekPanX, PeekPanY float32
	PeekAngle                    float32
	PeekOneToOne                 bool
	PeekButton                   int
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

	showSettings       bool
	settingsModalOpen  bool
	settingsConfirm    bool
	settingsCloseAfter func()
	draft              Settings
	settingsUI         components.SettingsState
	draftThumbSize     float64
	draftGridGap       float64
	draftSidebarW      float64
	draftCacheLimit    float64
	draftPrefetchMax   float64
	draftPreload       float64
	draftTheme         string
	draftSort          string
	cacheInfo          CacheInfo
	focusSearch        bool
	searchOpen         bool
	searchHandle       ui.Handle
	scrollActiveTab    bool
	// centeredFolderTree is the folder the sidebar's 文件夹 section last
	// centred. The section compares it with the current folder and centres when
	// they differ, so it happens once per change, after the data it needs has
	// arrived — a one-shot flag set when the folder changed would be spent by a
	// build that ran before the parent's children were read.
	centeredFolderTree string
	// sidebarScroll keeps the sidebar's place, and lets the 文件夹 section be
	// scrolled to the current folder: a container that tracks its scroll keeps
	// it across rebuilds, which ScrollIntoView needs to stick.
	sidebarScroll ui.ScrollState
	tabScroll     ui.ScrollState
	places        []Place
	drives        []Place

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

	viewerStats             viewerLoadStats
	viewerSlot              *ui.Bitmap
	viewerFullRes           *viewerFullResEntry
	viewerFullResKey        string
	viewerPendingActual     bool
	viewerPendingActualAt   bool
	viewerPendingActualX    float32
	viewerPendingActualY    float32
	viewerPendingActualW    float32
	viewerPendingActualH    float32
	viewerPendingActualOldW int
	viewerPendingActualOldH int
	viewerSlowTimer         *time.Timer
	viewerFullTimer         *time.Timer
	viewerNoticeTimer       *time.Timer
	// The resident readouts (quality, preload, zoom) follow the controls out of
	// sight, but come back whenever one of them changes: viewerStatusKey is the
	// last snapshot, viewerStatusUntil how long it may stay on screen, and
	// viewerStatusChrome whether the control bar was showing on the last frame,
	// so hiding the bar reveals the readouts once more.
	viewerStatusKey    string
	viewerStatusUntil  time.Time
	viewerStatusTimer  *time.Timer
	viewerStatusChrome bool

	hoverTimer *time.Timer
	hoverKey   string
	hoverToken int

	themeDetected bool
	systemDark    bool

	contextIDs   []string
	contextEmpty bool
	// pendingOpenPath is a file to open in the viewer once the folder it lives
	// in has finished loading; a favorite can point at a file in any folder.
	pendingOpenPath string

	// The grid context menu lives in a window-level overlay so its material
	// is not clipped by the scrolling content and its placement can flip at
	// the pointer instead of at the scroll viewport.
	gridMenuOpen       bool
	gridMenuX          float32
	gridMenuY          float32
	gridMenuFocusFirst bool
	gridMenuFocusPanel bool
	gridMenuRefocus    bool
	gridMenuTabID      int
	gridFocusHandle    ui.Handle
	// The selection submenu is its own overlay so it can sit above the grid
	// menu and return to it on Escape.
	selectMenuOpen       bool
	selectMenuFocusFirst bool
	selectMenuTabID      int
	pending              []func()

	thumbs     map[string]*thumbEntry
	thumbLoad  map[string]bool
	thumbQueue *thumbQueue
	// thumbFastQueue carries cards that are already cached on disk, so a
	// scroll-back never queues behind a full-resolution render.
	thumbFastQueue      *thumbQueue
	thumbFrame          uint64
	thumbOnce           sync.Once
	thumbBytes          int64
	thumbClock          int64
	thumbPendingRefresh bool

	// lastGridScrollY and lastGridActivity decide how wide the background fill
	// may be: scrolling narrows it so the cards being looked at keep a worker.
	lastGridScrollY  float32
	lastGridActivity time.Time
	// Background fill of the active folder. It is bound to (tab, Gen), so a
	// tab switch or a new listing drops whatever is still queued.
	prefetchTab    *Tab
	prefetchGen    int
	prefetchNext   int
	prefetchActive int
	// fillStopped marks a listing whose fill the user cancelled, so the next
	// grid frame does not simply restart it.
	fillStoppedTab *Tab
	fillStoppedGen int

	// Remote connection dialog and its draft. The password never leaves the
	// process unencrypted; DPAPI covers the remembered copy.
	remoteDialogOpen bool
	remoteEditing    bool
	remoteDraft      remoteShare
	// remoteEditID is the id of the share being edited. The id is derived from
	// the address, so editing the path changes it; keeping the original lets
	// the save replace that entry instead of adding a second one.
	remoteEditID    string
	remotePortDraft float64
	// The protocol and HTTPS state the port field was last derived from, so a
	// change can be detected before the field is built.
	remotePortProtocol string
	remotePortSecure   bool
	remotePass         string
	remoteBusy         bool
	remoteErr          string

	// notice is free-form text the debug endpoint can report back.
	notice string
	// A short note in the status bar, for actions that do not open a dialog.
	statusNotice   string
	statusNoticeAt time.Time

	// Background task list shown in the status bar and its popover.
	tasks            []*backgroundTask
	taskSeq          int
	tasksOpen        bool
	taskTimer        *time.Timer
	taskRefreshTimer *time.Timer
	taskTimerPainted time.Time

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
	thumbCacheMax = 1200
	// thumbCacheBytesMax is the in-memory bitmap budget for grid cards. It
	// covers a full screen of very large thumbnails (640 px at 12 columns)
	// several times over, so scrolling back never decodes again.
	thumbCacheBytesMax = 512 << 20

	// A background fill is queued thumbFillIdle tasks deep; while the user is
	// scrolling it drops to thumbFillBusy so the cards being looked at always
	// have workers. The depth is also the width: a fill task never waits for a
	// slot of its own, it simply waits in the queue behind visible cards.
	thumbFillIdle = 4
	thumbFillBusy = 2
	// thumbFillBusyWindow is how long after the last scroll the fill stays
	// narrow. It is longer than a flick, shorter than reading a screen.
	thumbFillBusyWindow = 700 * time.Millisecond
	// thumbPrefetchBase keeps fills behind every visible card (whose priority
	// is a viewport distance in DIP) and ahead of nothing else.
	thumbPrefetchBase = 1e9

	// thumbCacheLaneWorkers is how many cards can be read back from disk at
	// once. The work is a stat, a 30 KB read and a JPEG decode, so a small
	// number covers a screen; more would only contend for the disk.
	thumbCacheLaneWorkers = 3
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
		t.Filter = validMediaFilter(st.Filter)
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
			if a.pendingOpenPath != "" {
				want := a.pendingOpenPath
				a.pendingOpenPath = ""
				for _, cand := range t.Items {
					if strings.EqualFold(cleanDisplayPath(cand.Path), cleanDisplayPath(want)) {
						t.Selected = map[string]bool{cand.ID: true}
						t.Anchor = cand.ID
						a.openViewer(cand)
						break
					}
				}
			}
			if !keepScroll {
				// A new folder starts at the top, unless a restored tab had a
				// saved offset for this path.
				if t.Scroll.Y < 0 {
					t.Scroll.Y = 0
				}
			}
			if a.win != nil {
				a.setWindowTitle(t)
			}
			a.persist()
			a.refresh()
		})
	}()
}

// setWindowTitle names the window after the tab. A share shows its full
// address: "PHOTOS" alone does not say which machine it came from.
func (a *App) setWindowTitle(t *Tab) {
	if a.win == nil || t == nil {
		return
	}
	// A share shows its alias when it has one and its address otherwise: the
	// alias is what the user named it, and the address is the fallback that
	// still says which machine it came from.
	name := t.Name
	if isRemotePath(t.Path) {
		if alias := remoteAliasForPath(t.Path); alias != "" {
			name = alias
		} else {
			name = t.Path
		}
	}
	a.win.SetTitle(name + " — FlowVision")
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
			a.setWindowTitle(t)
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
	if cur := a.tab(); cur != nil {
		t.Filter = validMediaFilter(cur.Filter)
	}
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
	path = cleanDisplayPath(path)
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
	// Multi-select is a mode for the folder being viewed. Carrying it into the
	// next folder would make the first click there toggle instead of select.
	t.MultiSelect = false
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
	t.Anchor = ""
	t.MultiSelect = false
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
	if isRemotePath(path) {
		// A virtual path has no os.Stat and no file name to select.
		a.goTo(path, true)
		return
	}
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
	want = cleanDisplayPath(want)
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
		a.update(func() {
			if a.tab() == nil {
				a.newTab(path)
				return
			}
			a.goTo(path, true)
		})
	}()
}

func (a *App) revealSelected() {
	t := a.tab()
	if t == nil {
		return
	}
	if it := t.selectedOne(); it != nil {
		if isRemotePath(it.Path) {
			a.showStatusNotice("网络位置不在资源管理器里显示")
			return
		}
		_ = (AppSvc{}).Reveal(it.Path)
		return
	}
	if isRemotePath(t.Path) {
		a.showStatusNotice("网络位置不在资源管理器里显示")
		return
	}
	_ = (AppSvc{}).OpenFolder(t.Path)
}

func (a *App) openSelectedExternal() {
	if it := a.tab().selectedOne(); it != nil {
		if isRemotePath(it.Path) {
			a.showStatusNotice("网络位置只在应用内浏览，不能交给其他程序打开")
			return
		}
		_ = (AppSvc{}).OpenExternal(it.Path)
	}
}

// copySelectedPath copies one path, or every selected path one per line. The
// selection bar offers it for a run too, because pasting a list of photos into
// another tool is the common reason to select several in the first place.
func (a *App) copySelectedPath() {
	t := a.tab()
	if t == nil {
		return
	}
	if it := t.selectedOne(); it != nil {
		_ = (AppSvc{}).CopyText(it.Path)
		a.showStatusNotice("已复制路径")
		return
	}
	selected := t.selectedItems()
	if len(selected) == 0 {
		return
	}
	paths := make([]string, 0, len(selected))
	for _, it := range selected {
		paths = append(paths, it.Path)
	}
	// Windows clipboard consumers expect CRLF between lines.
	_ = (AppSvc{}).CopyText(strings.Join(paths, "\r\n"))
	a.showStatusNotice(fmt.Sprintf("已复制 %d 条路径", len(paths)))
}

// toggleFavoritePath adds an image or video to the favorites folder, or
// removes it when it is already there.
func (a *App) toggleFavoritePath(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	if a.isFavorite(path) {
		a.removeFavorite(path)
		return
	}
	kind, _ := classify(path)
	if !isMediaKind(kind) {
		a.showStatusNotice("收藏夹只收图片和视频")
		return
	}
	fav := Favorite{Path: path, Name: displayName(path), Kind: kind}
	if !isRemotePath(path) {
		if st, err := os.Stat(path); err == nil {
			fav.Size = st.Size()
			fav.Mtime = st.ModTime().Unix()
		}
	}
	a.settings.Favorites = append(a.settings.Favorites, fav)
	_ = saveSettings(a.settings)
	a.showStatusNotice("已加入收藏夹")
	a.refreshFavoritesTab()
	a.refresh()
}

// toggleItemFavorites collects the selected images and videos, or removes them
// when all of them are already collected. A mixed selection adds the missing
// ones, which is what a user pressing the same button twice expects to undo.
func (a *App) toggleItemFavorites() {
	t := a.tab()
	if t == nil {
		return
	}
	media := make([]*Item, 0, len(t.Selected))
	for _, it := range t.selectedItems() {
		if isMediaKind(it.Kind) {
			media = append(media, it)
		}
	}
	if len(media) == 0 {
		a.showStatusNotice("收藏夹只收图片和视频")
		return
	}
	allSaved := true
	for _, it := range media {
		if !a.isFavorite(it.Path) {
			allSaved = false
			break
		}
	}
	if allSaved {
		for _, it := range media {
			a.removeFavorite(it.Path)
		}
		a.showStatusNotice(fmt.Sprintf("已移出收藏夹 %d 项", len(media)))
		return
	}
	added := 0
	for _, it := range media {
		if a.isFavorite(it.Path) {
			continue
		}
		a.settings.Favorites = append(a.settings.Favorites, favoriteFromItem(it))
		added++
	}
	_ = saveSettings(a.settings)
	a.showStatusNotice(fmt.Sprintf("已加入收藏夹 %d 项", added))
	a.refreshFavoritesTab()
}

// removeFavorite drops one entry. The filter copies instead of reusing the
// backing array: an in-place append would leave the stored settings holding a
// longer slice of the same array, and the next save would resurrect the entry.
func (a *App) removeFavorite(path string) {
	out := make([]Favorite, 0, len(a.settings.Favorites))
	for _, f := range a.settings.Favorites {
		if !sameFavorite(f, path) {
			out = append(out, f)
		}
	}
	a.settings.Favorites = out
	_ = saveSettings(a.settings)
	a.refreshFavoritesTab()
	a.refresh()
}

// refreshFavoritesTab reloads the virtual folder when it is on screen. Its
// listing comes from settings, not from the disk, so nothing else would tell
// the grid that an entry was added or removed.
func (a *App) refreshFavoritesTab() {
	if t := a.tab(); t != nil && isFavoritesPath(t.Path) {
		a.loadTab(t, true)
	}
}

func (a *App) isFavorite(path string) bool {
	for _, f := range a.settings.Favorites {
		if sameFavorite(f, path) {
			return true
		}
	}
	return false
}

// openFavorite opens a collected file from the favorites folder, which is
// always the folder being viewed: the viewer's neighbours are the other
// favorites, so stepping through them works without leaving the virtual folder.
func (a *App) openFavorite(f Favorite) {
	t := a.tab()
	if t != nil && isFavoritesPath(t.Path) {
		for _, it := range t.Items {
			if sameFavorite(f, it.Path) {
				a.selectOnly(it.ID)
				a.openViewer(it)
				return
			}
		}
	}
	// Not in the favorites tab: open it there first, then the file.
	a.goTo(favoritesPath, true)
	a.pendingOpenPath = f.Path
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
	a.settingsModalOpen = true
	a.settingsConfirm = false
	a.settingsCloseAfter = nil
	a.showSettings = true
	a.refresh()
}

// settingsEditingEqual compares only the fields the settings surface can edit.
// Favorites, tabs and the last path are application state that happen to share
// the Settings file, not unsaved preference changes.
func settingsEditingEqual(a, b Settings) bool {
	return a.Theme == b.Theme &&
		a.ThumbSize == b.ThumbSize &&
		a.GridGap == b.GridGap &&
		a.ShowCardNames == b.ShowCardNames &&
		a.Sort == b.Sort &&
		a.Desc == b.Desc &&
		a.ShowHidden == b.ShowHidden &&
		a.ShowOther == b.ShowOther &&
		a.Recursive == b.Recursive &&
		a.Sidebar == b.Sidebar &&
		a.SidebarWidth == b.SidebarWidth &&
		a.InfoPanel == b.InfoPanel &&
		a.ViewerAutoImmersive == b.ViewerAutoImmersive &&
		a.PlayVideo == b.PlayVideo &&
		a.LoopVideo == b.LoopVideo &&
		a.CacheLimitMB == b.CacheLimitMB &&
		a.ViewerPreload == b.ViewerPreload &&
		a.ViewerHUD == b.ViewerHUD &&
		a.ViewerHoldZoom == b.ViewerHoldZoom &&
		a.ViewerHoldFit == b.ViewerHoldFit &&
		a.SidePrev == b.SidePrev &&
		a.SideNext == b.SideNext &&
		a.SidePrevHold == b.SidePrevHold &&
		a.SideNextHold == b.SideNextHold &&
		a.ThumbPrefetch == b.ThumbPrefetch &&
		a.ThumbPrefetchMax == b.ThumbPrefetchMax &&
		a.ShowPlaces == b.ShowPlaces &&
		a.ViewerAutoFullRes == b.ViewerAutoFullRes
}

func (a *App) settingsDirty() bool {
	return !settingsEditingEqual(a.settings, a.commitSettingsDraft())
}

// requestSettingsClose is the only path that closes the settings surface for
// a user action. A modal's X, Escape and backdrop click all arrive here; with
// no edits it closes at once, otherwise it asks before dropping the draft.
func (a *App) requestSettingsClose() {
	if !a.showSettings || a.settingsConfirm {
		return
	}
	if !a.settingsDirty() {
		a.closeSettingsNow()
		return
	}
	// Keep the editor behind the confirmation. The settings modal's own X,
	// Escape and backdrop handling flip the shared open flag before this runs;
	// restoring it here means the warning sits over the draft it is protecting
	// rather than over the browser page.
	a.settingsModalOpen = true
	a.settingsConfirm = true
	a.refresh()
}

// requestSettingsCloseAfter closes settings and runs an explicit follow-up,
// such as opening help or about. With pending edits the user decides first;
// continuing to edit cancels the follow-up.
func (a *App) requestSettingsCloseAfter(after func()) {
	a.settingsCloseAfter = after
	a.requestSettingsClose()
}

func (a *App) closeSettingsNow() {
	a.showSettings = false
	a.settingsModalOpen = false
	a.settingsConfirm = false
	a.bindingCapture = ""
	a.bindingCaptureNote = ""
	after := a.settingsCloseAfter
	a.settingsCloseAfter = nil
	if after != nil {
		a.later(after)
	}
	a.refresh()
}

// fillSettingsDraft copies persisted settings into the editable draft. Controls
// bind to these retained fields, not to locals rebuilt every frame: a Changed
// notice arrives on the following build pass, when a local would already have
// been reset to the old value.
func (a *App) fillSettingsDraft(s Settings) {
	a.draft = s
	a.draft.Favorites = append([]Favorite(nil), s.Favorites...)
	a.draft.StarredPaths = append([]Favorite(nil), s.StarredPaths...)
	a.draft.Tabs = append([]TabState(nil), s.Tabs...)
	a.draftTheme = s.Theme
	a.draftSort = s.Sort
	a.draftThumbSize = float64(s.ThumbSize)
	a.draftGridGap = float64(s.GridGap)
	a.draftSidebarW = float64(s.SidebarWidth)
	a.draftCacheLimit = float64(s.CacheLimitMB)
	a.draftPrefetchMax = float64(s.ThumbPrefetchMax)
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
	a.draft.ThumbPrefetchMax = int(math.Round(a.draftPrefetchMax))
	a.draft.ViewerPreload = int(math.Round(a.draftPreload))
	return a.draft
}

func (a *App) resetSettingsDraft() {
	favorites := append([]Favorite(nil), a.draft.Favorites...)
	starred := append([]Favorite(nil), a.draft.StarredPaths...)
	tabs := append([]TabState(nil), a.draft.Tabs...)
	active := a.draft.ActiveTab
	lastPath := a.draft.LastPath
	s := defaultSettings()
	s.Favorites = favorites
	s.StarredPaths = starred
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
	a.settingsModalOpen = false
	a.settingsConfirm = false
	a.settingsCloseAfter = nil
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
	// A raised threshold or a switched-off feature must stop the fill that is
	// already running, not only the next one: reloadActive would leave the
	// current listing's queued work in place until it drained.
	if !s.ThumbPrefetch || s.ThumbPrefetchMax > 0 {
		a.cancelThumbPrefetch()
		a.dropTasksOfKind(taskKindFill)
	}
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
// The gallery palette keeps chrome warm and quiet around photographs; the
// viewer swaps to its own neutral darkroom palette.
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
	t := galleryPalette(dark)
	applyThemePreferences(t, c)
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
	a.cancelThumbPrefetch()
	if t := a.tab(); t != nil {
		// A manual clear should leave the cache empty until the user browses,
		// so mark the current listing as already filled.
		a.prefetchTab = t
		a.prefetchGen = t.Gen
		a.prefetchNext = len(t.All)
	}
	if a.thumbQueue != nil {
		a.thumbQueue.reset()
	}
	a.refresh()
}

func (a *App) beginRename() {
	if a.tabReadOnly() {
		a.showStatusNotice("网络位置是只读的，不能重命名")
		return
	}
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
	if a.tabReadOnly() {
		a.showStatusNotice("网络位置是只读的，不能新建文件夹")
		return
	}
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
	if a.tabReadOnly() {
		a.showStatusNotice("网络位置是只读的，不能删除文件")
		return
	}
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
	a.treeLoaded[cleanDisplayPath(dir)] = false
	delete(a.treeKids, cleanDisplayPath(dir))
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

// setMultiSelect turns the tab's multi-select mode on or off. Leaving the mode
// keeps the selection: the user switched modes to act on what is selected, not
// to throw it away.
func (a *App) setMultiSelect(on bool) {
	t := a.tab()
	if t == nil {
		return
	}
	t.MultiSelect = on
	if !on && len(t.Selected) == 0 {
		t.Anchor = ""
	}
	a.refresh()
}

// exitMultiSelect leaves the mode and clears what was selected.
func (a *App) exitMultiSelect() {
	t := a.tab()
	if t == nil {
		return
	}
	t.MultiSelect = false
	t.Selected = map[string]bool{}
	t.Anchor = ""
	a.refresh()
}

// selectSameKind extends the selection to every visible item of the same kind
// as the anchor, which is how "star all these videos" becomes one action.
func (a *App) selectSameKind() {
	t := a.tab()
	if t == nil {
		return
	}
	one := t.selectedOne()
	if one == nil && t.Anchor != "" {
		one = t.byID(t.Anchor)
	}
	if one == nil {
		return
	}
	for _, it := range t.Items {
		if it.Kind == one.Kind {
			t.Selected[it.ID] = true
		}
	}
	t.MultiSelect = true
	t.Anchor = one.ID
	a.refresh()
}

// invertSelection flips every visible item's selected state.
func (a *App) invertSelection() {
	t := a.tab()
	if t == nil {
		return
	}
	next := make(map[string]bool, len(t.Selected))
	for _, it := range t.Items {
		if !t.Selected[it.ID] {
			next[it.ID] = true
		}
	}
	t.Selected = next
	t.MultiSelect = true
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
	t.MultiSelect = true
	a.refresh()
}

func (a *App) clearSelection() {
	if t := a.tab(); t != nil {
		t.Selected = map[string]bool{}
		t.MultiSelect = false
		t.Anchor = ""
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

func mediaFilterMatch(it *Item, filter string) bool {
	if it == nil {
		return false
	}
	if filter != "image" && filter != "video" {
		return true
	}
	// Folders stay visible in either solo mode so the browser never becomes a
	// dead end: the folder remains the way to reach more media.
	return it.Kind == "folder" || it.Kind == filter
}

func validMediaFilter(filter string) string {
	switch filter {
	case "image", "video":
		return filter
	default:
		return "all"
	}
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
	case isRemotePath(it.Path):
		a.showStatusNotice("网络位置只在应用内浏览，不能交给其他程序打开")
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
		Immersive:     a.settings.ViewerAutoImmersive,
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
	a.clearViewerPendingActualSize()
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
	if it.Kind == "video" {
		// A picture says 缩略图 in the footer while its preview decodes, so only
		// a video needs a spoken notice for a slow first frame.
		a.startViewerSlowNotice(gen, "正在载入大图…", viewerNoticeLoadDelay)
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
		var player *ui.VideoPlayer
		var err error
		if isRemotePath(it.Path) {
			// Media Foundation cannot open a share with this app's credentials,
			// so the local media server hands it a loopback URL that answers
			// Range requests straight from the share.
			var mediaURL string
			if mediaURL, err = remoteMediaServer.URL(it.Path); err == nil {
				player, err = mvideo.OpenURL(mediaURL)
			}
		} else {
			player, err = mvideo.Open(it.Path)
		}
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
	if a.viewerStatusTimer != nil {
		a.viewerStatusTimer.Stop()
		a.viewerStatusTimer = nil
	}
	a.viewerStatusKey = ""
	a.viewerStatusUntil = time.Time{}
	a.viewerStatusChrome = false
	a.clearViewerHover()
	a.clearViewerFullRes()
	a.clearViewerPendingActualSize()
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
	a.clearViewerPendingActualSize()
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
	a.clearViewerPendingActualSize()
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
	if a.viewerNeedsFullResForActualSize() {
		// Do not enlarge the 2560 preview to 1:1 while the original is still
		// decoding. The blurry intermediate frame is what makes the first
		// switch feel discontinuous, so hold the current fit view and apply
		// 1:1 only when the original bitmap arrives.
		a.deferViewerActualSize(false, 0, 0, 0, 0)
		return
	}
	a.clearViewerPendingActualSize()
	bw, bh := a.viewer.Bitmap.Size()
	w, h := a.viewerCanvasSize(nil)
	fit := viewerFitScale(bw, bh, a.viewer.Angle, w, h)
	if fit <= 0 {
		return
	}
	scale := a.viewerDeviceScale()
	a.viewer.Zoom = clampViewerZoom(viewerActualZoom(fit, scale), fit, scale)
	a.viewer.PanX = 0
	a.viewer.PanY = 0
	a.viewer.OneToOne = true
	a.refresh()
}

// viewerActualSizeAt jumps to 100% while keeping the bitmap point under the
// pointer fixed. Once the display is already at 100% or beyond, it leaves the
// image alone so repeated long presses cannot keep enlarging it.
func (a *App) viewerActualSizeAt(x, y, w, h float32) {
	if a.viewer.Bitmap == nil || a.viewer.Item == nil || a.viewer.Item.Kind != "image" {
		return
	}
	if w <= 0 || h <= 0 {
		w, h = a.viewerCanvasSize(nil)
	}
	if x < 0 || y < 0 {
		x, y = w/2, h/2
	}
	if !a.viewerPointInImage(x, y, w, h) {
		return
	}
	if a.viewerNeedsFullResForActualSize() {
		a.deferViewerActualSize(true, x, y, w, h)
		return
	}
	a.clearViewerPendingActualSize()
	a.viewer.OneToOne = true
	bw, bh := a.viewer.Bitmap.Size()
	fit := viewerFitScale(bw, bh, a.viewer.Angle, w, h)
	if fit <= 0 {
		return
	}
	scale := a.viewerDeviceScale()
	actual := clampViewerZoom(viewerActualZoom(fit, scale), fit, scale)
	if a.viewer.Zoom >= actual {
		return
	}
	a.viewer.Zoom, a.viewer.PanX, a.viewer.PanY = viewerZoomAtPoint(
		a.viewer.Zoom, a.viewer.PanX, a.viewer.PanY, actual/a.viewer.Zoom, x, y, w, h, fit, scale)
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
	a.clearViewerPendingActualSize()
	a.viewer.PanX = 0
	a.viewer.PanY = 0
	a.refresh()
}

func (a *App) viewerZoom(delta float32) {
	if a.viewer.Item == nil || a.viewer.Item.Kind != "image" {
		return
	}
	a.clearViewerPendingActualSize()
	before := a.viewer.Zoom
	a.viewer.Zoom = clampViewerZoom(a.viewer.Zoom*delta, a.viewerFitForDisplay(), a.viewerDeviceScale())
	if a.viewer.Zoom != before {
		a.viewer.OneToOne = false
	}
	a.maybeRequestViewerFullRes()
	a.refresh()
}

// viewerFitForDisplay is the fit scale of the bitmap on screen, which is what
// the zoom ceiling is expressed against.
func (a *App) viewerFitForDisplay() float32 {
	if a.viewer.Bitmap == nil {
		return 0
	}
	bw, bh := a.viewer.Bitmap.Size()
	w, h := a.viewerCanvasSize(nil)
	return viewerFitScale(bw, bh, a.viewer.Angle, w, h)
}

func (a *App) viewerZoomAt(delta, x, y, w, h float32) {
	if a.viewer.Item == nil || a.viewer.Item.Kind != "image" {
		return
	}
	if w <= 0 || h <= 0 {
		w, h = a.viewerCanvasSize(nil)
	}
	if !a.viewerPointInImage(x, y, w, h) {
		return
	}
	a.clearViewerPendingActualSize()
	before := a.viewer.Zoom
	fit := float32(0)
	if a.viewer.Bitmap != nil {
		bw, bh := a.viewer.Bitmap.Size()
		fit = viewerFitScale(bw, bh, a.viewer.Angle, w, h)
	}
	a.viewer.Zoom, a.viewer.PanX, a.viewer.PanY = viewerZoomAtPoint(
		a.viewer.Zoom, a.viewer.PanX, a.viewer.PanY, delta, x, y, w, h, fit, a.viewerDeviceScale())
	if a.viewer.Zoom != before {
		a.viewer.OneToOne = false
	}
	a.maybeRequestViewerFullRes()
	a.refresh()
}

func (a *App) viewerRotate(delta float32) {
	if a.viewer.Item == nil || a.viewer.Item.Kind != "image" {
		return
	}
	a.clearViewerPendingActualSize()
	a.viewer.Angle += delta
	w, h := a.viewerCanvasSize(nil)
	a.clampViewerPan(w, h)
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
		tabs = append(tabs, TabState{Path: t.Path, Sort: t.Sort, Desc: t.Desc, Filter: validMediaFilter(t.Filter), Scroll: float64(t.Scroll.Y)})
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
	a.submitThumbTask(key+"|f", key, it, size, priority, frame, thumbStageFinal, false)
	if e := a.thumbs[key]; e == nil && it.HasThumb {
		previewSize := size
		if previewSize > 160 {
			previewSize = 160
		}
		// Preview is closer to the viewport center than the final decode, so
		// it runs first and paints the card while WIC works on the full image.
		a.submitThumbTask(key+"|p", key, it, previewSize, priority-1, frame, thumbStagePreview, false)
	}
}

func (a *App) submitThumbTask(taskKey, base string, it *Item, size int, priority float64, frame uint64, stage thumbStage, preload bool) bool {
	if thumbDir == "" {
		// Without a cache directory a decoded thumbnail cannot be stored, so do
		// not start workers. Headless view tests take this path.
		return false
	}
	if a.thumbLoad == nil {
		a.thumbLoad = map[string]bool{}
	}
	if a.thumbLoad[taskKey] {
		if a.thumbQueue != nil {
			a.thumbQueue.touch(taskKey, priority, frame)
		}
		return false
	}
	if a.thumbQueue == nil {
		a.thumbQueue = newThumbQueue()
	}
	a.thumbLoad[taskKey] = true
	a.startThumbWorkers()
	task := &thumbTask{
		key:      taskKey,
		base:     base,
		item:     it,
		size:     size,
		priority: priority,
		frame:    frame,
		stage:    stage,
		preload:  preload,
	}
	// A card whose file is already on disk only has to be read and decoded.
	// Sending it to the decode pool put it behind a full 24 MP render, which is
	// why scrolling back through a warm folder still stuttered.
	if !preload && stage == thumbStageFinal && a.win != nil && thumbCacheExists(it, size) {
		if a.thumbFastQueue == nil {
			a.thumbFastQueue = newThumbQueue()
		}
		a.thumbFastQueue.submit(task)
		return true
	}
	a.thumbQueue.submit(task)
	return true
}

func (a *App) startThumbWorkers() {
	if a.win == nil {
		// Results are applied on the window's thread, so there is nothing to
		// start without a window. The headless view builder runs on one thread;
		// letting workers apply results there would race its map access.
		return
	}
	a.thumbOnce.Do(func() {
		n := cap(thumbSlots)
		if n < 1 {
			n = 6
		}
		for i := 0; i < n; i++ {
			go a.thumbWorker()
		}
		// The cache lane is separate on purpose: its work is a read and a JPEG
		// decode of a 30 KB file, and sharing the pool with the decoders is
		// exactly what made it wait.
		if a.thumbFastQueue == nil {
			a.thumbFastQueue = newThumbQueue()
		}
		for i := 0; i < thumbCacheLaneWorkers; i++ {
			go a.thumbCacheWorker()
		}
	})
}

// thumbCacheWorker serves cards that are already cached on disk.
func (a *App) thumbCacheWorker() {
	for {
		task := a.thumbFastQueue.next()
		if task == nil {
			return
		}
		a.serveCachedCard(task)
	}
}

// serveCachedCard answers one card from the cache lane. A miss is handed to the
// decode pool rather than rendered here: the lane exists to be quick, not to
// become a second decoder. It reports whether the card was answered here.
func (a *App) serveCachedCard(task *thumbTask) bool {
	if data, ok := readThumbCache(task.item, task.size); ok {
		a.postThumbResult(task, data, nil, a.thumbFastQueue)
		return true
	}
	a.thumbFastQueue.done(task.key)
	a.thumbQueue.submit(task)
	return false
}

// endThumbFrame drops queued loads that the latest grid frame did not mark
// visible. Running loads finish, so at most the workers' current tasks delay
// a newly visible card. Background fills are exempt: the grid never marks
// them, so the queue would otherwise cancel them every frame.
func (a *App) endThumbFrame(frame uint64) {
	if a.thumbQueue == nil {
		return
	}
	dropped := a.thumbQueue.retain(frame)
	if a.thumbFastQueue != nil {
		dropped = append(dropped, a.thumbFastQueue.retain(frame)...)
	}
	for _, key := range dropped {
		delete(a.thumbLoad, key)
	}
}

// pumpThumbPrefetch keeps filling the active folder after the visible cards
// are queued. Fills run at the lowest priority, so scrolling always wins, and
// two at a time, so a burst of new cards still finds free workers. Nothing
// keeps a bitmap: the fill stops at the disk cache and the waterfall learns
// each aspect ratio when its card actually becomes visible.
//
// The fill belongs to one listing. Switching tabs, opening another folder or
// turning the feature off cancels what is still queued; a decode already in
// flight just finishes and leaves a valid cache file behind.
func (a *App) pumpThumbPrefetch(t *Tab) {
	if thumbDir == "" || a.thumbQueue == nil || !a.settings.ThumbPrefetch || t == nil || !t.Loaded || t.Loading ||
		// A whole-folder fill over SMB would saturate the share and the
		// cache budget alike; network folders fill as they scroll.
		isRemotePath(t.Path) ||
		// A folder past the limit would fill hundreds of pictures the user may
		// never reach; it fills as it scrolls instead.
		a.settings.ThumbPrefetchMax > 0 && len(t.All) > a.settings.ThumbPrefetchMax {
		a.cancelThumbPrefetch()
		a.dropTasksOfKind(taskKindFill)
		return
	}
	if a.fillStoppedTab == t && a.fillStoppedGen == t.Gen {
		// Checked before the identity reset below, so cancelling on the current
		// folder keeps its "stopped" row instead of dropping it as stale.
		a.cancelThumbPrefetch()
		return
	}
	if a.prefetchTab != t || a.prefetchGen != t.Gen {
		a.cancelThumbPrefetch()
		// Another folder ends the old fill by itself; that is not a cancel the
		// user asked for, so it leaves no row behind.
		a.dropTasksOfKind(taskKindFill)
		a.prefetchTab = t
		a.prefetchGen = t.Gen
	}
	size := a.settings.ThumbSize
	width := a.fillWidth(time.Now())
	for a.prefetchActive < width && a.prefetchNext < len(t.All) {
		it := t.All[a.prefetchNext]
		a.prefetchNext++
		if it.Kind != "image" && it.Kind != "video" {
			continue
		}
		key := thumbKey(it, size)
		if e := a.thumbs[key]; e != nil {
			// Already decoded for a visible card, so the file is cached too.
			continue
		}
		if a.thumbLoad[key+"|f"] {
			// A visible card is loading it right now; a fill would duplicate it.
			continue
		}
		priority := thumbPrefetchBase + float64(a.prefetchNext)
		if a.submitThumbTask(key+"|f", key, it, size, priority, a.thumbFrame, thumbStageFinal, true) {
			a.prefetchActive++
		}
	}
	a.syncFillTask(t)
}

// taskKindFill is the only background task: filling a folder's thumbnail
// cache. The viewer's original-resolution decode is not a task because it can
// only happen while the viewer owns the window, where the status bar and the
// task panel are not drawn.
const taskKindFill = "fill"

// fillWidth is how many background fills may be outstanding. Four decode at
// once when the app is at rest; two while the user is scrolling, so the cards
// under the pointer keep most of the workers.
func (a *App) fillWidth(now time.Time) int {
	if now.Sub(a.lastGridActivity) < thumbFillBusyWindow {
		return thumbFillBusy
	}
	return thumbFillIdle
}

// noteGridActivity records that the grid moved, which is what narrows the fill.
func (a *App) noteGridActivity(t *Tab) {
	if t == nil {
		return
	}
	if t.Scroll.Y != a.lastGridScrollY {
		a.lastGridScrollY = t.Scroll.Y
		a.lastGridActivity = time.Now()
	}
}

// syncFillTask mirrors the folder fill into the task list: it starts a row on
// the first frame that has work, reports how much is still outstanding, and
// finishes it once the folder is exhausted.
func (a *App) syncFillTask(t *Tab) {
	if t == nil || len(t.All) == 0 {
		return
	}
	total := len(t.All)
	// The cursor counts pictures handed to the workers; the ones still in
	// flight are not finished. Subtracting the in-flight count from the total
	// instead, as this once did, reported almost 100% from the first frame.
	done := a.prefetchNext - a.prefetchActive
	if done < 0 {
		done = 0
	}
	task := a.taskByKind(taskKindFill)
	if a.prefetchNext >= total && a.prefetchActive == 0 {
		if task != nil {
			a.finishTask(task.ID)
		}
		return
	}
	if task == nil {
		folder := t.Name
		task = a.startTask(taskKindFill, "补载缩略图", folder, total, func() { a.stopThumbFill(t) })
	}
	a.setTaskProgress(task.ID, done, total)
}

// stopThumbFill is the user cancelling a folder fill. The marker keeps the next
// grid frame from queueing the same work again.
func (a *App) stopThumbFill(t *Tab) {
	if t == nil {
		return
	}
	a.fillStoppedTab = t
	a.fillStoppedGen = t.Gen
	a.cancelThumbPrefetch()
}

// cancelThumbPrefetch drops the queued remainder of a folder fill. The cursor
// restarts at the top of the next listing, so a folder visited again warms the
// files that were skipped rather than repeating nothing.
func (a *App) cancelThumbPrefetch() {
	if a.prefetchTab == nil && a.prefetchNext == 0 && a.prefetchActive == 0 {
		return
	}
	a.prefetchTab = nil
	a.prefetchNext = 0
	a.prefetchActive = 0
	if a.thumbQueue != nil {
		for _, key := range a.thumbQueue.dropPreload() {
			delete(a.thumbLoad, key)
		}
	}
}

func (a *App) thumbWorker() {
	for {
		task := a.thumbQueue.next()
		if task == nil {
			return
		}
		a.runThumbTask(task)
	}
}

// runThumbTask loads one queued thumbnail and applies the result on the main
// thread.
func (a *App) runThumbTask(task *thumbTask) {
	if !task.preload && isRemotePath(task.item.Path) {
		remoteThumbSlots <- struct{}{}
		defer func() { <-remoteThumbSlots }()
	}
	if task.preload {
		// Background fill: stop at the disk cache. No bitmap, no dims, no
		// repaint; the card decodes from cache when it becomes visible. The
		// fill's width is bounded by how many tasks the pump queues, not by a
		// slot taken here, because a worker parked on a fill would leave a
		// visible card waiting behind it.
		_, _ = getThumb(context.Background(), task.item, task.size)
		a.update(func() {
			delete(a.thumbLoad, task.key)
			a.thumbQueue.done(task.key)
			if a.prefetchActive > 0 {
				a.prefetchActive--
			}
			a.pumpThumbPrefetch(a.tab())
		})
		return
	}
	var data []byte
	var src image.Image
	if task.stage == thumbStagePreview {
		data, _ = embeddedThumbPreview(task.item, task.size)
	} else {
		// The pixels come back whenever the backend had them anyway, which is
		// every card that is rendered rather than read from the cache.
		rendered, err := renderThumbForCard(context.Background(), task.item, task.size)
		data, src = rendered.Data, rendered.Img
		if err != nil {
			data = nil
		}
		a.postThumbResult(task, data, src, a.thumbQueue)
		if err == nil && !rendered.Cached && len(rendered.Data) > 0 {
			// After the bitmap has been handed over: the picture reaches the
			// screen before the worker spends time on its own cache file.
			writeThumbCache(thumbCachePath(task.item, task.size), rendered.Data)
		}
		return
	}
	a.postThumbResult(task, data, src, a.thumbQueue)
}

// postThumbResult turns encoded card bytes into the in-memory bitmap and hands
// them to the window thread. q is the lane the task came from, so its running
// marker is cleared on the right queue.
func (a *App) postThumbResult(task *thumbTask, data []byte, src image.Image, q *thumbQueue) {
	var bmp *ui.Bitmap
	var err error
	if task.stage == thumbStageFinal && src != nil {
		// The bytes were just rendered from these pixels, so decoding them
		// back would be a second, pointless trip through the JPEG codec.
		bmp = ui.NewBitmap(src)
	} else {
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
	if q == nil {
		q = a.thumbQueue
	}
	a.update(func() {
		delete(a.thumbLoad, task.key)
		q.done(task.key)
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
	p := cleanDisplayPath(path)
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

// byID finds a visible item by its stable id.
func (t *Tab) byID(id string) *Item {
	for _, it := range t.Items {
		if it.ID == id {
			return it
		}
	}
	return nil
}
