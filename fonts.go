package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// The interface font can ship with the app as plain files under
// resources/fonts. MiSans's license forbids modifying the font, so the files
// are used exactly as published: no subsetting, no conversion, no renaming.
// A build without them (or a machine without MiSans installed) keeps the
// platform's own font.

var (
	// interfaceFontFamily is what resources/fonts is registered as.
	interfaceFontFamily = "MiSans"

	// interfaceFontPreference is tried in order when looking for a family the
	// system already has.
	interfaceFontPreference = []string{"MiSans", "MiSans VF"}

	interfaceFontMu   sync.Mutex
	interfaceFontName string
	interfaceFontFile []string
)

// interfaceFont is the family the interface should use, or "" for the
// platform's own font.
func interfaceFont() string {
	interfaceFontMu.Lock()
	defer interfaceFontMu.Unlock()
	return interfaceFontName
}

func setInterfaceFont(name string) {
	interfaceFontMu.Lock()
	interfaceFontName = name
	interfaceFontMu.Unlock()
}

// interfaceFontFiles is what the app registered, for the about window and for
// diagnostics.
func interfaceFontFiles() []string {
	interfaceFontMu.Lock()
	defer interfaceFontMu.Unlock()
	return append([]string(nil), interfaceFontFile...)
}

// fontSearchDirs lists where shipped fonts live: resources/fonts next to the
// executable, so `mygo build` installs them with the app.
// interfaceFontPreferenceList allows a diagnostic override, like the app's other
// environment switches: FLOWVISION_UI_FONT="Times New Roman" turns the check
// into a side-by-side comparison without rebuilding.
func interfaceFontPreferenceList() []string {
	if v := strings.TrimSpace(os.Getenv("FLOWVISION_UI_FONT")); v != "" {
		return []string{v}
	}
	return interfaceFontPreference
}

func fontSearchDirs() []string {
	dir, err := mygo.App.Path(mygo.PathResources)
	if err != nil || dir == "" {
		return nil
	}
	return []string{filepath.Join(dir, "fonts")}
}

// loadInterfaceFonts registers every font file in dirs as interfaceFontFamily
// and returns the paths it could register. The family is given explicitly so
// the app knows what it got without asking the system for its font list, which
// is not ready this early in startup.
func loadInterfaceFonts(dirs []string) []string {
	var loaded []string
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !isFontFile(e.Name()) {
				continue
			}
			path := filepath.Join(dir, e.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				logf("font %s: %v", e.Name(), err)
				continue
			}
			if err := ui.RegisterFont(data, interfaceFontFamily); err != nil {
				logf("font %s: %v", e.Name(), err)
				continue
			}
			loaded = append(loaded, path)
		}
	}
	return loaded
}

func isFontFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".ttf", ".otf", ".ttc":
		return true
	}
	return false
}

// initFonts registers the shipped fonts before any window exists, so the first
// frame already uses MiSans. The family name is known from the content of
// resources/fonts, so no system font list is consulted here.
func initFonts() {
	loaded := loadInterfaceFonts(fontSearchDirs())
	interfaceFontMu.Lock()
	interfaceFontFile = loaded
	interfaceFontMu.Unlock()
	if len(loaded) == 0 {
		return
	}
	setInterfaceFont(interfaceFontFamily)
}

// detectInstalledInterfaceFont uses a MiSans installed on this machine when the
// build does not ship one, and applies FLOWVISION_UI_FONT even when it does. It
// asks the system for its font list, which takes long enough that it waits
// until the window is up.
func (a *App) detectInstalledInterfaceFont() {
	override := strings.TrimSpace(os.Getenv("FLOWVISION_UI_FONT")) != ""
	if interfaceFont() != "" && !override {
		return
	}
	go func() {
		name := firstAvailable(interfaceFontPreferenceList(), fontFamilyAvailable)
		if name == "" {
			return
		}
		a.update(func() {
			if interfaceFont() == name {
				return
			}
			setInterfaceFont(name)
			a.refresh()
		})
	}()
}

// fontFamilyAvailable reports whether text can use a family, either because the
// app registered it or because the system has it.
func fontFamilyAvailable(family string) bool {
	return firstAvailable([]string{family}, func(name string) bool {
		for _, f := range ui.FontFamilies() {
			if strings.EqualFold(f.Name, name) {
				return true
			}
		}
		return false
	}) != ""
}

// firstAvailable returns the first name known accepts, or "" when it knows none.
func firstAvailable(names []string, known func(string) bool) string {
	for _, name := range names {
		if known(name) {
			return name
		}
	}
	return ""
}
