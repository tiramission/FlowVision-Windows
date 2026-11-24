package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const appVersion = "1.1.0"

var (
	mainWin     *mygo.Window
	startPath   string
	cliPath     string
	pendingMu   sync.Mutex
	pendingOpen string
	logger      *log.Logger
	app         *App
)

func logf(format string, args ...any) {
	if logger != nil {
		logger.Printf(format, args...)
	}
}

func main() {
	mygo.App.SetName("FlowVision")
	mygo.App.SetVersion(appVersion)
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	mygo.App.OnSecondInstance(func(args []string, workingDir string) {
		p := pathFromArgs(args, workingDir)
		if mainWin == nil || app == nil {
			if p != "" {
				pendingMu.Lock()
				pendingOpen = p
				pendingMu.Unlock()
			}
			return
		}
		mainWin.Show()
		mainWin.Focus()
		if p != "" {
			mainWin.Update(func() { app.openPathArgument(p) })
		}
	})

	initLog()
	initSettings()
	cliPath = detectCLIPath()
	startPath = resolveStartPath()
	initThumbs()

	app = newApp()
	if cliPath != "" {
		// A launch with a path focuses that folder rather than restoring the
		// previous session's tabs.
		t := newTab(0, cliPath)
		t.Sort = app.settings.Sort
		t.Desc = app.settings.Desc
		app.tabs = []*Tab{t}
		app.active = 0
		app.nextID = 1
	}
	app.applyTheme()
	logf("starting with %s", startPath)

	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
		if app != nil {
			app.persist()
		}
	})

	mygo.App.WhenReady(func() {
		installMenu(app)
		openMainWindow()
	})
	if err := mygo.App.Run(); err != nil {
		logf("run: %v", err)
		os.Exit(1)
	}
}

func initLog() {
	dir, err := mygo.App.Path(mygo.PathLogs)
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "flowvision.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	logger = log.New(f, "", log.LstdFlags|log.Lshortfile)
}

func openMainWindow() {
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:     "FlowVision",
		Content:   ui.View(app.view),
		Width:     1320,
		Height:    850,
		MinWidth:  720,
		MinHeight: 460,
		StateKey:  "main",
		Hidden:    true,
	})
	mainWin = win
	app.attach(win)
	if data, err := os.ReadFile(iconPath()); err == nil {
		_ = win.SetIcon(data)
	}
	win.OnReadyToShow(func() {
		win.Show()
		win.Focus()
		pendingMu.Lock()
		p := pendingOpen
		pendingOpen = ""
		pendingMu.Unlock()
		if p != "" {
			win.Update(func() { app.openPathArgument(p) })
		}
	})
}

func iconPath() string {
	exe, err := mygo.App.Path(mygo.PathExe)
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	candidates := []string{
		filepath.Join(dir, "resources", "icon.png"),
		filepath.Join(dir, "icon.png"),
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// resolveStartPath picks the folder the first tab shows: the command line's,
// the last one the user browsed, or the pictures folder.
func resolveStartPath() string {
	if cliPath != "" {
		return cliPath
	}
	if p := getSettings().LastPath; p != "" {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	if p, err := mygo.App.Path(mygo.PathPictures); err == nil && p != "" {
		return p
	}
	if p, err := mygo.App.Path(mygo.PathHome); err == nil && p != "" {
		return p
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "C:\\"
}

// detectCLIPath turns a command line argument (folder or file) into a folder.
func detectCLIPath() string {
	wd, _ := os.Getwd()
	return pathFromArgs(os.Args[1:], wd)
}

// pathFromArgs resolves the first non-option argument against workingDir and
// returns the folder to show (a dropped file opens its containing folder).
func pathFromArgs(args []string, workingDir string) string {
	if len(args) == 0 {
		return ""
	}
	arg := strings.Trim(args[0], `"`)
	if arg == "" || strings.HasPrefix(arg, "-") {
		return ""
	}
	if !filepath.IsAbs(arg) && workingDir != "" {
		arg = filepath.Join(workingDir, arg)
	}
	st, err := os.Stat(arg)
	if err != nil {
		return ""
	}
	clean := filepath.Clean(arg)
	if st.IsDir() {
		return clean
	}
	return filepath.Dir(clean)
}
