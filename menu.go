package main

import (
	"strconv"

	"github.com/egoist/mygo"
)

func jsStr(s string) string { return strconv.Quote(s) }

func evalPage(win *mygo.Window, code string) {
	if win == nil {
		win = mainWin
	}
	if win == nil {
		return
	}
	if _, err := win.Page().Eval(code); err != nil {
		logf("eval: %v", err)
	}
}

func menuAction(win *mygo.Window, action string) {
	code := "window.fv && window.fv.menu(" + jsStr(action) + ")"
	// Menu accelerators run inside the WebView's key-event callback. Eval
	// waits for the page to run that script, so calling it there would
	// deadlock the UI thread until the eval timeout. Dispatch instead.
	go evalPage(win, code)
}

func installMenu() {
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{
			Label: "文件",
			Submenu: []*mygo.MenuItem{
				{Label: "打开文件夹…", Accelerator: "CmdOrCtrl+O", Click: func(_ *mygo.MenuItem, w *mygo.Window) {
					// Let the page open the picker so it starts at the active tab.
					menuAction(w, "openfolder")
				}},
				{Label: "新建标签页", Accelerator: "CmdOrCtrl+T", Click: func(_ *mygo.MenuItem, w *mygo.Window) { menuAction(w, "newtab") }},
				{Label: "关闭标签页", Accelerator: "CmdOrCtrl+W", Click: func(_ *mygo.MenuItem, w *mygo.Window) { menuAction(w, "closetab") }},
				mygo.Separator(),
				{Label: "在资源管理器中打开", Click: func(_ *mygo.MenuItem, w *mygo.Window) { menuAction(w, "reveal") }},
				mygo.Separator(),
				{Label: "退出", Accelerator: "CmdOrCtrl+Q", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { mygo.App.Quit() }},
			},
		},
		{
			Label: "视图",
			Submenu: []*mygo.MenuItem{
				{Label: "放大缩略图", Accelerator: "CmdOrCtrl+=", Click: func(_ *mygo.MenuItem, w *mygo.Window) { menuAction(w, "thumb+") }},
				{Label: "缩小缩略图", Accelerator: "CmdOrCtrl+-", Click: func(_ *mygo.MenuItem, w *mygo.Window) { menuAction(w, "thumb-") }},
				mygo.Separator(),
				{Label: "切换侧边栏", Accelerator: "CmdOrCtrl+B", Click: func(_ *mygo.MenuItem, w *mygo.Window) { menuAction(w, "sidebar") }},
				{Label: "递归模式", Accelerator: "CmdOrCtrl+R", Click: func(_ *mygo.MenuItem, w *mygo.Window) { menuAction(w, "recursive") }},
				{Label: "显示隐藏文件", Click: func(_ *mygo.MenuItem, w *mygo.Window) { menuAction(w, "hidden") }},
				mygo.Separator(),
				{Label: "设置…", Accelerator: "CmdOrCtrl+,", Click: func(_ *mygo.MenuItem, w *mygo.Window) { menuAction(w, "settings") }},
				{Role: mygo.RoleToggleFullScreen},
			},
		},
		{
			Label: "转到",
			Submenu: []*mygo.MenuItem{
				{Label: "上一文件夹", Accelerator: "CmdOrCtrl+Left", Click: func(_ *mygo.MenuItem, w *mygo.Window) { menuAction(w, "prevfolder") }},
				{Label: "下一文件夹", Accelerator: "CmdOrCtrl+Right", Click: func(_ *mygo.MenuItem, w *mygo.Window) { menuAction(w, "nextfolder") }},
				{Label: "上一级", Accelerator: "CmdOrCtrl+Up", Click: func(_ *mygo.MenuItem, w *mygo.Window) { menuAction(w, "up") }},
				{Label: "返回", Accelerator: "CmdOrCtrl+[", Click: func(_ *mygo.MenuItem, w *mygo.Window) { menuAction(w, "back") }},
			},
		},
		{
			Label: "帮助",
			Submenu: []*mygo.MenuItem{
				{Label: "快捷键与手势", Click: func(_ *mygo.MenuItem, w *mygo.Window) { menuAction(w, "help") }},
				{Label: "关于 FlowVision", Click: func(_ *mygo.MenuItem, w *mygo.Window) { menuAction(w, "about") }},
			},
		},
	})
	mygo.App.SetMenu(menu)
}
