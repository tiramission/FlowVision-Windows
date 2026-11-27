//go:build debug

package main

// Test-only control endpoint for the native UI. It is compiled only with
// `go build -tags "debug mygo_nowebview"` and is never part of a release.
// It can inspect the app state, issue commands on the main thread, and capture
// the rendered window as PNG, which is how the headless test harness checks
// the native layout.

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

type debugState struct {
	Version                  string   `json:"version"`
	Active                   int      `json:"active"`
	Path                     string   `json:"path"`
	Name                     string   `json:"name"`
	Loading                  bool     `json:"loading"`
	Err                      string   `json:"error,omitempty"`
	Total                    int      `json:"total"`
	Filtered                 int      `json:"filtered"`
	Query                    string   `json:"query"`
	ScrollY                  float32  `json:"scrollY"`
	Selected                 []string `json:"selected"`
	Tabs                     []string `json:"tabs"`
	ViewerOpen               bool     `json:"viewerOpen"`
	ViewerIndex              int      `json:"viewerIndex"`
	ViewerName               string   `json:"viewerName,omitempty"`
	ViewerBytes              int64    `json:"viewerBytes,omitempty"`
	ViewerZoom               float32  `json:"viewerZoom"`
	ViewerPanX               float32  `json:"viewerPanX"`
	ViewerPanY               float32  `json:"viewerPanY"`
	ViewerAngle              float32  `json:"viewerAngle"`
	ViewerInfo               bool     `json:"viewerInfo"`
	ViewerImmersive          bool     `json:"viewerImmersive"`
	ViewerChrome             bool     `json:"viewerChrome"`
	ViewerCursor             bool     `json:"viewerCursorVisible"`
	ViewerErr                string   `json:"viewerError,omitempty"`
	HasBitmap                bool     `json:"hasBitmap"`
	ThumbSize                int      `json:"thumbSize"`
	ThumbPending             int      `json:"thumbPending"`
	ThumbRunning             int      `json:"thumbRunning"`
	ThumbReady               int      `json:"thumbReady"`
	ThumbPreview             int      `json:"thumbPreview"`
	ThumbCacheMB             float64  `json:"thumbCacheMB"`
	CacheThumbFiles          int      `json:"cacheThumbFiles"`
	CachePreviewFiles        int      `json:"cachePreviewFiles"`
	ThumbPrefetchEnabled     bool     `json:"thumbPrefetchEnabled"`
	ThumbPrefetchNext        int      `json:"thumbPrefetchNext"`
	ThumbPrefetchActive      int      `json:"thumbPrefetchActive"`
	ThumbPrefetchTotal       int      `json:"thumbPrefetchTotal"`
	ThumbBackend             string   `json:"thumbBackend"`
	Goroutines               int      `json:"goroutines"`
	Sidebar                  bool     `json:"sidebar"`
	Recursive                bool     `json:"recursive"`
	ShowHidden               bool     `json:"showHidden"`
	ShowCardNames            bool     `json:"showCardNames"`
	MediaFilter              string   `json:"mediaFilter"`
	ViewerAutoImmersive      bool     `json:"viewerAutoImmersive"`
	ShowSettings             bool     `json:"showSettings"`
	SettingsCat              string   `json:"settingsCategory"`
	BindingCapture           string   `json:"bindingCapture"`
	SidePrev                 string   `json:"sidePrev"`
	SideNext                 string   `json:"sideNext"`
	SidePrevHold             string   `json:"sidePrevHold"`
	SideNextHold             string   `json:"sideNextHold"`
	SideMouseHook            bool     `json:"sideMouseHook"`
	ViewerCanAct             bool     `json:"viewerCanAct"`
	ViewerPreload            int      `json:"viewerPreload"`
	ViewerCached             int      `json:"viewerPreloadCached"`
	ViewerPreloadMB          float64  `json:"viewerPreloadMB"`
	ViewerPreloadHit         bool     `json:"viewerPreloadHit"`
	ViewerHUD                string   `json:"viewerHUD"`
	ViewerLoadMs             int64    `json:"viewerLoadMs"`
	ViewerDecodeMs           int64    `json:"viewerDecodeMs"`
	ViewerScaleMs            int64    `json:"viewerScaleMs"`
	ViewerEncodeMs           int64    `json:"viewerEncodeMs"`
	ViewerBitmapMs           int64    `json:"viewerBitmapMs"`
	ViewerCacheHit           bool     `json:"viewerCacheHit"`
	ViewerPlaceholder        bool     `json:"viewerPlaceholder"`
	ViewerFullRes            bool     `json:"viewerFullRes"`
	ViewerFullResMB          float64  `json:"viewerFullResMB"`
	ViewerOneToOne           bool     `json:"viewerOneToOne"`
	ViewerFullResLoading     bool     `json:"viewerFullResLoading"`
	ViewerFullResUnsupported bool     `json:"viewerFullResUnsupported"`
	ViewerFullResPending     bool     `json:"viewerFullResPending"`
	ViewerNotice             string   `json:"viewerNotice,omitempty"`
	HeapAllocMB              float64  `json:"heapAllocMB"`
	HeapSysMB                float64  `json:"heapSysMB"`
	StackSysMB               float64  `json:"stackSysMB"`
	NumGC                    uint32   `json:"numGC"`
	ShowHelp                 bool     `json:"showHelp"`
	ShowAbout                bool     `json:"showAbout"`
	UpdateOpen               bool     `json:"updateOpen"`
	UpdatePhase              string   `json:"updatePhase"`
	UpdateVersion            string   `json:"updateVersion,omitempty"`
	Maximized                bool     `json:"maximized"`
	Fullscreen               bool     `json:"fullscreen"`
	PlayerReady              bool     `json:"playerReady"`
	PlayerPlaying            bool     `json:"playerPlaying"`
	PlayerPosMS              int64    `json:"playerPositionMs"`
	PlayerDurMS              int64    `json:"playerDurationMs"`
	VideoWidth               int      `json:"videoWidth"`
	VideoHeight              int      `json:"videoHeight"`
	VideoFPS                 float64  `json:"videoFps"`
	VideoKbps                int      `json:"videoKbps"`
	VideoLive                bool     `json:"videoLiveBitrate"`
	ContentW                 int      `json:"contentWidth"`
	ContentH                 int      `json:"contentHeight"`
	ErrorMsg                 string   `json:"errorMessage,omitempty"`
	Items                    []string `json:"items"`
}

func debugSnapshot() debugState {
	st := debugState{Version: appVersion(), Active: app.active}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	st.HeapAllocMB = float64(ms.HeapAlloc) / (1 << 20)
	st.HeapSysMB = float64(ms.HeapSys) / (1 << 20)
	st.StackSysMB = float64(ms.StackSys) / (1 << 20)
	st.NumGC = ms.NumGC
	st.Sidebar = app.sidebar
	st.ThumbSize = app.settings.ThumbSize
	if app.thumbQueue != nil {
		st.ThumbPending, st.ThumbRunning = app.thumbQueue.stats()
	}
	st.ThumbReady = len(app.thumbs)
	for _, e := range app.thumbs {
		if e.Preview {
			st.ThumbPreview++
		}
	}
	st.ThumbCacheMB = float64(app.thumbBytes) / (1 << 20)
	st.CacheThumbFiles = countCacheFiles(thumbDir)
	st.CachePreviewFiles = countCacheFiles(previewDir)
	st.ThumbPrefetchEnabled = app.settings.ThumbPrefetch
	st.ThumbPrefetchNext = app.prefetchNext
	st.ThumbPrefetchActive = app.prefetchActive
	if t := app.tab(); t != nil {
		st.ThumbPrefetchTotal = len(t.All)
	}
	st.ThumbBackend = thumbBackendMode()
	st.Goroutines = runtime.NumGoroutine()
	st.Recursive = app.settings.Recursive
	st.ShowHidden = app.settings.ShowHidden
	st.ShowCardNames = app.settings.ShowCardNames
	st.MediaFilter = "all"
	if t := app.tab(); t != nil {
		st.MediaFilter = t.Filter
	}
	st.ViewerAutoImmersive = app.settings.ViewerAutoImmersive
	st.ShowSettings = app.showSettings
	st.SettingsCat = app.settingsUI.Category
	st.SidePrev = sideBindingLabel(app.settings.SidePrev)
	st.SideNext = sideBindingLabel(app.settings.SideNext)
	st.SidePrevHold = sideBindingLabel(app.settings.SidePrevHold)
	st.SideNextHold = sideBindingLabel(app.settings.SideNextHold)
	st.SideMouseHook = app.sideMouseHook != 0
	st.ViewerCanAct = app.viewerCanAct()
	st.BindingCapture = app.bindingCapture
	st.ViewerPreload = app.settings.ViewerPreload
	st.ViewerCached = app.viewerPreloadCount()
	st.ViewerPreloadMB = float64(app.viewerPreloadBytesValue()) / (1 << 20)
	st.ViewerHUD = app.viewerHUDMode()
	st.ShowHelp = app.showHelp
	st.ShowAbout = app.showAbout
	st.UpdateOpen = app.updateOpen
	st.UpdatePhase = app.updatePhase.String()
	st.UpdateVersion = app.updateVersion
	st.Maximized = app.maximized
	if mainWin != nil {
		st.ContentW, st.ContentH = mainWin.ContentSize()
		st.Fullscreen = mainWin.IsFullScreen()
	}
	st.ErrorMsg = app.errorMsg
	for _, t := range app.tabs {
		st.Tabs = append(st.Tabs, t.Path)
	}
	if t := app.tab(); t != nil {
		st.Path, st.Name = t.Path, t.Name
		st.Loading, st.Err = t.Loading, t.Err
		st.Total, st.Filtered = len(t.All), len(t.Items)
		st.Query = t.Query
		st.ScrollY = t.Scroll.Y
		for _, it := range t.selectedItems() {
			st.Selected = append(st.Selected, it.Name)
		}
		for i, it := range t.Items {
			if i >= 200 {
				break
			}
			st.Items = append(st.Items, it.Name)
		}
	}
	if app.viewer.Open {
		st.ViewerOpen = true
		st.ViewerIndex = app.viewer.Index
		if app.viewer.Item != nil {
			st.ViewerName = app.viewer.Item.Name
			st.ViewerBytes = app.viewer.Item.Size
		}
		st.ViewerZoom = app.viewer.Zoom
		st.ViewerPanX = app.viewer.PanX
		st.ViewerPanY = app.viewer.PanY
		st.ViewerAngle = app.viewer.Angle
		st.ViewerInfo = app.viewer.ShowInfo
		st.ViewerImmersive = app.viewer.Immersive
		st.ViewerChrome = app.viewer.ChromeVisible
		st.ViewerCursor = app.viewer.CursorVisible
		st.ViewerErr = app.viewer.Err
		st.HasBitmap = app.viewer.Bitmap != nil
		st.ViewerLoadMs = app.viewerStats.LoadMs
		st.ViewerDecodeMs = app.viewerStats.DecodeMs
		st.ViewerScaleMs = app.viewerStats.ScaleMs
		st.ViewerEncodeMs = app.viewerStats.EncodeMs
		st.ViewerBitmapMs = app.viewerStats.BitmapMs
		st.ViewerCacheHit = app.viewerStats.CacheHit
		st.ViewerPreloadHit = app.viewerStats.PreloadHit
		st.ViewerPlaceholder = app.viewer.Placeholder != nil
		st.ViewerFullRes = app.viewer.FullResReady
		st.ViewerFullResMB = float64(app.viewerStats.FullResBytes) / (1 << 20)
		st.ViewerOneToOne = app.viewer.OneToOne
		st.ViewerFullResLoading = app.viewer.FullResLoading
		st.ViewerFullResUnsupported = app.viewer.FullResUnsupported
		st.ViewerFullResPending = app.viewerPendingActual
		st.ViewerNotice = app.viewer.Notice.Text
		if p := app.viewer.Player; p != nil {
			st.PlayerReady = p.Ready()
			st.PlayerPlaying = p.Playing()
			st.PlayerPosMS = p.Position().Milliseconds()
			st.PlayerDurMS = p.Duration().Milliseconds()
			if p.Ready() {
				st.VideoWidth, st.VideoHeight = p.Size()
			}
		}
		if app.viewer.VideoReady {
			meta := app.viewer.Video
			if st.VideoWidth <= 0 {
				st.VideoWidth, st.VideoHeight = meta.Width, meta.Height
			}
			st.VideoFPS = meta.FPS
			st.VideoLive = meta.LiveBitrate
			st.VideoKbps = meta.BitrateKbps
			if meta.LiveBitrate && app.viewer.Player != nil {
				st.VideoKbps = meta.BitrateAtMS(app.viewer.Player.Position().Milliseconds(), 2*time.Second)
			}
		}
	}
	return st
}

func debugOnMain(fn func() any) any {
	if mainWin == nil {
		return nil
	}
	done := make(chan any, 1)
	mainWin.Update(func() { done <- fn() })
	return <-done
}

func init() {
	go func() {
		for i := 0; i < 600; i++ {
			if mainWin != nil && app != nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if mainWin == nil || app == nil {
			return
		}
		mux := http.NewServeMux()
		mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("ok"))
		})
		mux.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
			v := debugOnMain(func() any { s := debugSnapshot(); return s })
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(v)
		})
		mux.HandleFunc("/shot", func(w http.ResponseWriter, r *http.Request) {
			var data []byte
			var err error
			_ = debugOnMain(func() any {
				data, err = mainWin.CapturePage()
				return nil
			})
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(data)
		})
		mux.HandleFunc("/command", func(w http.ResponseWriter, r *http.Request) {
			name := strings.TrimSpace(r.URL.Query().Get("name"))
			arg := r.URL.Query().Get("arg")
			debugOnMain(func() any {
				debugCommand(name, arg)
				return nil
			})
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(debugSnapshot())
		})
		ln, err := net.Listen("tcp", "127.0.0.1:47831")
		if err != nil {
			_, _ = os.Stderr.WriteString("debug server: " + err.Error() + "\n")
			return
		}
		_ = http.Serve(ln, mux)
	}()
}

func debugCommand(name, arg string) {
	t := app.tab()
	switch name {
	case "open":
		app.goTo(arg, true)
	case "openpath":
		app.openPathArgument(arg)
	case "newtab":
		app.newTab(arg)
	case "closetab":
		i, _ := strconv.Atoi(arg)
		if arg == "" {
			i = app.active
		}
		app.closeTab(i)
	case "activate":
		i, _ := strconv.Atoi(arg)
		app.activateTab(i)
	case "select":
		i, _ := strconv.Atoi(arg)
		if t != nil && i >= 0 && i < len(t.Items) {
			app.selectOnly(t.Items[i].ID)
		}
	case "selectall":
		app.selectAll()
	case "selectmany":
		parts := strings.Split(arg, ",")
		if t != nil {
			t.Selected = map[string]bool{}
			for _, p := range parts {
				i, _ := strconv.Atoi(strings.TrimSpace(p))
				if i >= 0 && i < len(t.Items) {
					t.Selected[t.Items[i].ID] = true
					t.Anchor = t.Items[i].ID
				}
			}
			app.refresh()
		}
	case "viewer":
		i, _ := strconv.Atoi(arg)
		if t != nil && i >= 0 && i < len(t.Items) {
			app.openViewer(t.Items[i])
		}
	case "closeviewer":
		app.closeViewer()
	case "viewerinfo":
		app.viewerToggleInfo()
	case "viewerpan":
		parts := strings.Split(arg, ",")
		if len(parts) == 2 {
			x, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 32)
			y, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 32)
			app.viewer.PanX, app.viewer.PanY = float32(x), float32(y)
			app.refresh()
		}
	case "viewercenter":
		app.viewerCenter()
	case "viewerchrome":
		switch arg {
		case "auto":
			app.viewer.ChromeVisible = true
			app.viewer.ChromeUntil = time.Now().Add(1500 * time.Millisecond)
		case "hide":
			app.viewer.ChromeVisible = false
			app.viewer.ChromeUntil = time.Time{}
		default:
			app.viewer.ChromeVisible = !app.viewer.ChromeVisible
			if app.viewer.ChromeVisible {
				app.viewer.ChromeUntil = time.Now().Add(time.Hour)
			} else {
				app.viewer.ChromeUntil = time.Time{}
			}
		}
		app.refresh()
	case "viewerfullscreen", "viewerimmersive":
		app.viewerToggleImmersive()
	case "viewerplay":
		app.viewerTogglePlay()
	case "viewerseek":
		f, _ := strconv.ParseFloat(arg, 64)
		app.viewerSeekTo(f)
	case "viewerrate":
		app.viewerCycleRate()
	case "captions":
		app.settings.ShowCardNames = !app.settings.ShowCardNames
		_ = saveSettings(app.settings)
		app.refresh()
	case "viewerstep":
		i, _ := strconv.Atoi(arg)
		app.viewerStep(i)
	case "viewerzoom":
		f, _ := strconv.ParseFloat(arg, 64)
		app.viewerZoom(float32(f))
	case "viewerrotate":
		f, _ := strconv.ParseFloat(arg, 64)
		app.viewerRotate(float32(f))
	case "viewerreset":
		app.viewerReset()
	case "vieweractual":
		app.viewerActualSize()
	case "gc":
		runtime.GC()
		debug.FreeOSMemory()
	case "search":
		if t != nil {
			t.Query = arg
			t.refilter()
			app.refresh()
		}
	case "filter":
		if t != nil {
			app.setMediaFilter(t, arg)
		}
	case "autoimmersive":
		if arg == "" {
			app.settings.ViewerAutoImmersive = !app.settings.ViewerAutoImmersive
		} else {
			app.settings.ViewerAutoImmersive = arg != "off"
		}
		_ = saveSettings(app.settings)
		app.refresh()
	case "find":
		app.focusSearch = true
		app.refresh()
	case "scroll":
		{
			f, _ := strconv.ParseFloat(arg, 64)
			if t != nil {
				t.Scroll.Y = float32(f)
				app.refresh()
			}
		}
	case "theme":
		app.settings.Theme = arg
		_ = saveSettings(app.settings)
		app.applyTheme()
	case "size":
		{
			parts := strings.Split(strings.ToLower(arg), "x")
			if len(parts) == 2 && mainWin != nil {
				w, _ := strconv.Atoi(parts[0])
				h, _ := strconv.Atoi(parts[1])
				mainWin.SetSize(w, h)
			}
		}
	case "position":
		{
			parts := strings.Split(arg, ",")
			if len(parts) == 2 && mainWin != nil {
				x, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
				y, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
				mainWin.SetPosition(x, y)
			}
		}
	case "window":
		if mainWin != nil {
			switch arg {
			case "minimize":
				mainWin.Minimize()
			case "maximize":
				mainWin.Maximize()
			case "restore":
				mainWin.Unmaximize()
			case "fullscreen":
				app.toggleWindowFullscreen()
			}
		}
	case "sidebar":
		app.toggleSidebar()
	case "recursive":
		app.toggleRecursive()
	case "hidden":
		app.toggleHidden()
	case "thumbup":
		app.setThumbSize(32)
	case "thumbdown":
		app.setThumbSize(-32)
	case "sort":
		if t != nil {
			t.Sort = arg
			app.loadTab(t, true)
		}
	case "order":
		if t != nil {
			t.Desc = arg == "desc"
			app.loadTab(t, true)
		}
	case "reload":
		app.loadTab(t, false)
	case "clearcache":
		app.clearCache()
		app.refresh()
	case "back":
		app.goBack()
	case "up":
		app.up()
	case "sibling":
		i, _ := strconv.Atoi(arg)
		app.sibling(i)
	case "settings":
		app.openSettings()
		if arg != "" {
			app.settingsUI.Category = arg
			app.refresh()
		}
	case "settingsquery":
		app.openSettings()
		app.settingsUI.Query = arg
		app.refresh()
	case "sidebinding":
		if arg == "" {
			arg = "prevClick"
		}
		app.beginBindingCapture(arg)
	case "closeui":
		app.closeViewer()
		if app.showSettings {
			app.closeSettingsNow()
		}
		app.showHelp = false
		app.showAbout = false
		app.updateOpen = false
		app.refresh()
	case "applysettings":
		app.applySettings()
	case "help":
		app.openHelp()
	case "about":
		app.showAbout = true
		app.refresh()
	case "updateui":
		app.debugUpdateUI(arg)
	case "error":
		app.showError(arg)
	case "favorite":
		app.toggleFavorite()
	case "newfolder":
		app.newFolderDraft = arg
		app.newFolderOpen = true
		app.commitNewFolder()
	case "rename":
		app.beginRename()
		app.renameDraft = arg
		app.commitRename()
	case "delete":
		app.doDelete()
	case "copy":
		app.copySelectedPath()
	case "quit":
		mygo.App.Quit()
	}
}
