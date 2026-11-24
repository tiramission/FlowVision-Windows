package main

import (
	"github.com/egoist/mygo"
)

func installMenu(a *App) {
	item := func(label, accel string, fn func()) *mygo.MenuItem {
		return &mygo.MenuItem{Label: label, Accelerator: accel, Click: func(_ *mygo.MenuItem, w *mygo.Window) {
			w.Update(fn)
		}}
	}

	menu := mygo.NewMenu([]*mygo.MenuItem{
		{
			Label: "文件",
			Submenu: []*mygo.MenuItem{
				item("打开文件夹…", "CmdOrCtrl+O", func() { a.pickFolder() }),
				item("新建标签页", "CmdOrCtrl+T", func() { a.newTab("") }),
				item("关闭标签页", "CmdOrCtrl+W", func() { a.closeTab(a.active) }),
				mygo.Separator(),
				item("在资源管理器中显示", "", func() { a.revealSelected() }),
				item("刷新", "CmdOrCtrl+Shift+R", func() { a.loadTab(a.tab(), false) }),
				mygo.Separator(),
				{Label: "退出", Accelerator: "CmdOrCtrl+Q", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { mygo.App.Quit() }},
			},
		},
		{
			Label: "编辑",
			Submenu: []*mygo.MenuItem{
				item("重命名…", "F2", func() { a.beginRename() }),
				item("新建文件夹…", "CmdOrCtrl+Shift+N", func() { a.beginNewFolder() }),
				item("移到回收站", "", func() { a.askDelete(false) }),
				item("永久删除", "CmdOrCtrl+Delete", func() { a.askDelete(true) }),
				mygo.Separator(),
				item("全选", "CmdOrCtrl+A", func() { a.selectAll() }),
				item("查找", "CmdOrCtrl+F", func() { a.focusSearch = true; a.refresh() }),
				item("复制路径", "CmdOrCtrl+Shift+C", func() { a.copySelectedPath() }),
				mygo.Separator(),
				item("用默认应用打开", "", func() { a.openSelectedExternal() }),
			},
		},
		{
			Label: "视图",
			Submenu: []*mygo.MenuItem{
				item("放大缩略图", "CmdOrCtrl+=", func() { a.setThumbSize(32) }),
				item("缩小缩略图", "CmdOrCtrl+-", func() { a.setThumbSize(-32) }),
				mygo.Separator(),
				item("切换侧边栏", "CmdOrCtrl+B", func() { a.toggleSidebar() }),
				item("递归模式", "CmdOrCtrl+R", func() { a.toggleRecursive() }),
				item("显示隐藏文件", "CmdOrCtrl+Shift+H", func() { a.toggleHidden() }),
				mygo.Separator(),
				item("设置…", "CmdOrCtrl+,", func() { a.openSettings() }),
				{Role: mygo.RoleToggleFullScreen},
			},
		},
		{
			Label: "转到",
			Submenu: []*mygo.MenuItem{
				item("上一文件夹", "CmdOrCtrl+Left", func() { a.sibling(-1) }),
				item("下一文件夹", "CmdOrCtrl+Right", func() { a.sibling(1) }),
				item("上一级", "CmdOrCtrl+Up", func() { a.up() }),
				item("返回", "CmdOrCtrl+[", func() { a.goBack() }),
			},
		},
		{
			Label: "帮助",
			Submenu: []*mygo.MenuItem{
				item("快捷键与手势", "F1", func() { a.showHelp = true; a.refresh() }),
				item("检查更新…", "", func() { a.checkForUpdates() }),
				mygo.Separator(),
				item("关于 FlowVision", "", func() { a.showAbout = true; a.refresh() }),
			},
		},
	})
	mygo.App.SetMenu(menu)
}
