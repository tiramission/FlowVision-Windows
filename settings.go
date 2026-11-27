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
	// ThumbPrefetchMin is the folder size at which the background fill starts.
	// Below it the visible cards already cover the folder, so filling would
	// only add a task row and disk churn.
	ThumbPrefetchMin  int         `json:"thumbPrefetchMin"`
	Tabs              []TabState  `json:"tabs"`
	ActiveTab         int         `json:"activeTab"`
	CacheLimitMB      int         `json:"cacheLimitMB"`
	ThumbPrefetch     bool        `json:"thumbPrefetch"`
	ViewerPreload     int         `json:"viewerPreload"`
	ViewerAutoFullRes bool        `json:"viewerAutoFullRes"`
	ViewerHUD         string      `json:"viewerHUD"` // off | auto | always
	SidePrev          SideBinding `json:"sidePrev"`
	SideNext          SideBinding `json:"sideNext"`
	SidePrevHold      SideBinding `json:"sidePrevHold"`
	SideNextHold      SideBinding `json:"sideNextHold"`
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
		ThumbPrefetchMin:    200,
		ShowPlaces:          true,
		ViewerPreload:       2,
		ViewerAutoFullRes:   true,
		ViewerHUD:           "auto",
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
		if _, ok := fields["thumbPrefetchMin"]; !ok {
			s.ThumbPrefetchMin = def.ThumbPrefetchMin
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
	if s.ThumbPrefetchMin < 0 {
		s.ThumbPrefetchMin = 0
	}
	if s.ThumbPrefetchMin > 10000 {
		s.ThumbPrefetchMin = 10000
	}
	switch s.ViewerHUD {
	case "off", "auto", "always":
	default:
		s.ViewerHUD = def.ViewerHUD
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
