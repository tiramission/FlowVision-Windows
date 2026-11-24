package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo"
)

//go:embed all:web
var webEmbed embed.FS

const appVersion = "1.0.0"

var (
	mainWin     *mygo.Window
	startPath   string
	cliPath     string
	pendingMu   sync.Mutex
	pendingOpen string
	logger      *log.Logger
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
		// A second launch hands its command line to this instance, then exits.
		p := pathFromArgs(args, workingDir)
		if mainWin == nil {
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
			// Eval waits for the page; keep the instance callback from
			// blocking the UI thread while that happens.
			go evalPage(mainWin, "window.fv && window.fv.openPath("+jsStr(p)+")")
		}
	})
	initLog()
	initSettings()
	cliPath = detectCLIPath()

	web, err := fs.Sub(webEmbed, "web")
	if err != nil {
		panic(err)
	}
	mygo.SetFrontend(web)

	initThumbs()
	registerImageProtocol()
	startMediaServer()

	mygo.Bind(FS{})
	mygo.Bind(MediaSvc{})
	mygo.Bind(Prefs{})
	mygo.Bind(AppSvc{})

	startPath = resolveStartPath()
	logf("starting with %s", startPath)

	mygo.App.WhenReady(func() {
		installMenu()
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
		Title:           "FlowVision",
		URL:             "/",
		Width:           1320,
		Height:          850,
		MinWidth:        720,
		MinHeight:       460,
		StateKey:        "main",
		Hidden:          true,
		BackgroundColor: "light-dark(#f6f6f8, #1b1b1e)",
	})
	mainWin = win
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
			go evalPage(win, "window.fv && window.fv.openPath("+jsStr(p)+")")
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

// describe is used by the log when a path is needed in a message.
func describe(p string) string { return fmt.Sprintf("%q", p) }
