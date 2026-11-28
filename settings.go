package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/egoist/mygo"
)

// TabState is one tab restored at the next launch.
type TabState struct {
	Path   string  `json:"path"`
	Sort   string  `json:"sort"`
	Desc   bool    `json:"desc"`
	Filter string  `json:"filter,omitempty"`
	Scroll float64 `json:"scroll"`
}

// Settings is the app's preferences, stored as settings.json in user data.
type Settings struct {
	Theme               string     `json:"theme"` // system | dark | light
	ThumbSize           int        `json:"thumbSize"`
	GridGap             int        `json:"gridGap"`
	ShowCardNames       bool       `json:"showCardNames"`
	Sort                string     `json:"sort"` // name | date | size | type
	Desc                bool       `json:"desc"`
	ShowHidden          bool       `json:"showHidden"`
	ShowOther           bool       `json:"showOther"`
	Recursive           bool       `json:"recursive"`
	Sidebar             bool       `json:"sidebar"`
	SidebarWidth        int        `json:"sidebarWidth"`
	InfoPanel           bool       `json:"infoPanel"`
	ViewerAutoImmersive bool       `json:"viewerAutoImmersive"`
	PlayVideo           bool       `json:"playVideo"`
	LoopVideo           bool       `json:"loopVideo"`
	LastPath            string     `json:"lastPath"`
	Favorites           []Favorite `json:"favorites"`
	// StarredPaths keeps folder locations for the sidebar, separate from
	// Favorites: that list collects images and videos into a virtual folder,
	// while this one remembers where to go back to. Sharing one list made
	// "收藏夹 (1)" count a starred folder the folder did not contain.
	StarredPaths []Favorite    `json:"starredPaths,omitempty"`
	RemoteShares []remoteShare `json:"remoteShares,omitempty"`
	// ShowPlaces hides the sidebar's well-known folders section for users who
	// only navigate through favorites and drives.
	ShowPlaces bool `json:"showPlaces"`
	// ThumbPrefetchMax is the largest folder the background fill still runs
	// on. Above it the fill would decode hundreds of pictures the user may
	// never reach, so a big folder fills only as it scrolls. Zero lifts the
	// limit.
	ThumbPrefetchMax  int        `json:"thumbPrefetchMax"`
	Tabs              []TabState `json:"tabs"`
	ActiveTab         int        `json:"activeTab"`
	CacheLimitMB      int        `json:"cacheLimitMB"`
	ThumbPrefetch     bool       `json:"thumbPrefetch"`
	ViewerPreload     int        `json:"viewerPreload"`
	ViewerAutoFullRes bool       `json:"viewerAutoFullRes"`
	// ViewerHoldZoom is what a long press of the primary button does on the
	// picture: jump to 100% and stay (jump), jump while held and go back to the
	// zoom it was at on release (peek), or nothing (off).
	ViewerHoldZoom string `json:"viewerHoldZoom"` // peek | jump | off
	// ViewerHoldFit is the same gesture on the secondary button: fit the whole
	// picture for as long as it is held (peek), keep it fitted (fit), or leave
	// the button alone (off).
	ViewerHoldFit string      `json:"viewerHoldFit"` // peek | fit | off
	SidePrev      SideBinding `json:"sidePrev"`
	SideNext      SideBinding `json:"sideNext"`
	SidePrevHold  SideBinding `json:"sidePrevHold"`
	SideNextHold  SideBinding `json:"sideNextHold"`
}

func defaultSettings() Settings {
	return Settings{
		Theme:               "system",
		ThumbSize:           220,
		GridGap:             8,
		ShowCardNames:       false,
		Sort:                "name",
		Desc:                false,
		ShowHidden:          false,
		ShowOther:           false,
		Recursive:           false,
		Sidebar:             true,
		SidebarWidth:        248,
		InfoPanel:           true,
		ViewerAutoImmersive: true,
		PlayVideo:           true,
		LoopVideo:           false,
		CacheLimitMB:        1500,
		ThumbPrefetch:       true,
		ThumbPrefetchMax:    200,
		ShowPlaces:          true,
		ViewerPreload:       2,
		ViewerAutoFullRes:   true,
		ViewerHoldZoom:      "peek",
		ViewerHoldFit:       "peek",
		SidePrev:            defaultSidePrev(),
		SideNext:            defaultSideNext(),
	}
}

var (
	settingsMu   sync.RWMutex
	settingsData = defaultSettings()
	settingsPath string
)

func initSettings() {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		logf("settings: userData: %v", err)
		return
	}
	settingsPath = filepath.Join(dir, "settings.json")
	// Favorites changed from a path list to objects; rewrite the document once
	// so the rest of the app only ever sees the current shape.
	migrateFavorites(settingsPath)
	data, err := os.ReadFile(settingsPath)
	if err == nil {
		if s, ok := parseSettings(data); ok {
			settingsData = s
		}
	}
}

// parseSettings merges a stored document over the defaults, so a file written
// by an older build (or edited by hand) keeps sane values for omitted fields.
func parseSettings(data []byte) (Settings, bool) {
	// An editor that saves as "UTF-8 with BOM" must not reset every preference.
	data = trimBOM(data)
	def := defaultSettings()
	s := def
	// Favorites are decoded separately: a 1.9 document stores plain paths, and
	// the typed field would make the whole settings load fail on them.
	favorites, hasFavorites := parseFavorites(rawField(data, "favorites"))
	rest, err := withoutField(data, "favorites")
	if err != nil || json.Unmarshal(rest, &s) != nil {
		return defaultSettings(), false
	}
	if hasFavorites {
		s.Favorites = favorites
	}
	// Older settings have no preload field. Keep the default instead of
	// treating the zero value as an explicit "off" choice.
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) == nil {
		if _, ok := fields["viewerPreload"]; !ok {
			s.ViewerPreload = def.ViewerPreload
		}
		if _, ok := fields["thumbPrefetch"]; !ok {
			s.ThumbPrefetch = def.ThumbPrefetch
		}
		if _, ok := fields["viewerAutoFullRes"]; !ok {
			s.ViewerAutoFullRes = def.ViewerAutoFullRes
		}
		if _, ok := fields["showPlaces"]; !ok {
			s.ShowPlaces = def.ShowPlaces
		}
		// The threshold became a ceiling and its key was renamed. A document
		// written before that carries the old key, whose value still means what
		// the new one does, so it is read by hand.
		if _, ok := fields["thumbPrefetchMax"]; !ok {
			if legacy, has := fields["thumbPrefetchMin"]; has {
				var v int
				if json.Unmarshal(legacy, &v) == nil {
					s.ThumbPrefetchMax = v
				}
			} else {
				s.ThumbPrefetchMax = def.ThumbPrefetchMax
			}
		}
	}
	return mergeSettings(def, s), true
}

func mergeSettings(def, s Settings) Settings {
	if s.ThumbSize < 80 {
		s.ThumbSize = def.ThumbSize
	}
	if s.ThumbSize > 640 {
		s.ThumbSize = 640
	}
	if s.GridGap < 0 || s.GridGap > 40 {
		s.GridGap = def.GridGap
	}
	if s.SidebarWidth < 160 || s.SidebarWidth > 520 {
		s.SidebarWidth = def.SidebarWidth
	}
	switch s.Sort {
	case "name", "date", "size", "type":
	default:
		s.Sort = def.Sort
	}
	switch s.Theme {
	case "system", "dark", "light":
	default:
		s.Theme = def.Theme
	}
	if s.CacheLimitMB < 200 {
		s.CacheLimitMB = 200
	}
	if s.CacheLimitMB > 20000 {
		s.CacheLimitMB = 20000
	}
	if s.ViewerPreload < 0 {
		s.ViewerPreload = 0
	}
	if s.ViewerPreload > 8 {
		s.ViewerPreload = 8
	}
	if s.ThumbPrefetchMax < 0 {
		s.ThumbPrefetchMax = 0
	}
	if s.ThumbPrefetchMax > 10000 {
		s.ThumbPrefetchMax = 10000
	}
	switch s.ViewerHoldFit {
	case "peek", "fit", "off":
	default:
		s.ViewerHoldFit = def.ViewerHoldFit
	}
	switch s.ViewerHoldZoom {
	case "peek", "jump", "off":
	default:
		s.ViewerHoldZoom = def.ViewerHoldZoom
	}
	if s.Favorites == nil {
		s.Favorites = []Favorite{}
	}
	if s.RemoteShares == nil {
		s.RemoteShares = []remoteShare{}
	}
	if s.Tabs == nil {
		s.Tabs = []TabState{}
	}
	s.SidePrev, s.SideNext, s.SidePrevHold, s.SideNextHold = mergeViewerBindings(
		s.SidePrev, s.SideNext, s.SidePrevHold, s.SideNextHold)
	return s
}

func getSettings() Settings {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	return settingsData
}

var settingsFileMu sync.Mutex

func saveSettings(s Settings) error {
	s = mergeSettings(defaultSettings(), s)
	settingsFileMu.Lock()
	defer settingsFileMu.Unlock()
	settingsMu.Lock()
	settingsData = s
	settingsMu.Unlock()
	if settingsPath == "" {
		return nil
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(settingsPath), ".settings-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, settingsPath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// CacheInfo describes the thumbnail cache.
type CacheInfo struct {
	Dir   string `json:"dir"`
	Files int    `json:"files"`
	Bytes int64  `json:"bytes"`
}
