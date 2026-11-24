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
	Scroll float64 `json:"scroll"`
}

// Settings is the app's preferences, stored as settings.json in user data.
type Settings struct {
	Theme         string     `json:"theme"` // system | dark | light
	ThumbSize     int        `json:"thumbSize"`
	GridGap       int        `json:"gridGap"`
	ShowCardNames bool       `json:"showCardNames"`
	Sort          string     `json:"sort"` // name | date | size | type
	Desc          bool       `json:"desc"`
	ShowHidden    bool       `json:"showHidden"`
	ShowOther     bool       `json:"showOther"`
	Recursive     bool       `json:"recursive"`
	Sidebar       bool       `json:"sidebar"`
	SidebarWidth  int        `json:"sidebarWidth"`
	InfoPanel     bool       `json:"infoPanel"`
	PlayVideo     bool       `json:"playVideo"`
	LoopVideo     bool       `json:"loopVideo"`
	LastPath      string     `json:"lastPath"`
	Favorites     []string   `json:"favorites"`
	Tabs          []TabState `json:"tabs"`
	ActiveTab     int        `json:"activeTab"`
	CacheLimitMB  int        `json:"cacheLimitMB"`
}

func defaultSettings() Settings {
	return Settings{
		Theme:         "system",
		ThumbSize:     220,
		GridGap:       8,
		ShowCardNames: true,
		Sort:          "name",
		Desc:          false,
		ShowHidden:    false,
		ShowOther:     false,
		Recursive:     false,
		Sidebar:       true,
		SidebarWidth:  248,
		InfoPanel:     true,
		PlayVideo:     true,
		LoopVideo:     false,
		CacheLimitMB:  1500,
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
	s := defaultSettings()
	if json.Unmarshal(data, &s) != nil {
		return defaultSettings(), false
	}
	return mergeSettings(defaultSettings(), s), true
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
	if s.Favorites == nil {
		s.Favorites = []string{}
	}
	if s.Tabs == nil {
		s.Tabs = []TabState{}
	}
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
