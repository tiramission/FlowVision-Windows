# FlowVision for Windows

用 **Go + MyGo + Onevra** 重写的 [FlowVision](https://github.com/netdcy/FlowVision)：一个以瀑布流展示本地图片/视频的 Windows 桌面浏览器。界面是 **MyGo 的纯 Go 原生 `ui` 控件、Onevra 主题/组件与自绘网格**，不依赖 WebView2、HTML 或 JavaScript；文件扫描、缩略图、元数据、查看器与文件操作全部在 Go 侧完成，不需要 cgo、Swift 或 Xcode。

- 上游项目：https://github.com/netdcy/FlowVision （GPL-3.0）
- 本移植版：GPL-3.0，见 `LICENSE`；第三方组件许可见 `THIRD_PARTY_NOTICES.md`
- 技术栈：Go 1.27+ · [MyGo](https://github.com/egoist/mygo) v0.10.0 · [Onevra](https://oneo-me.github.io/onevra/) v0.2026.18 · `github.com/oneo-me/onevra/source`
- 当前版本：**1.2.4（Onevra Settings / 设置与工具栏绑定修复 / 隐藏 ffmpeg 控制台）**
- 历史版本：1.2.2 接入签名自动更新；1.2.1 移除系统标题栏；1.2.0 接入 Onevra；1.1.0 是 MyGo 原生 UI 基线；1.0.0 是 Go + WebView2 的第一阶段移植，仍保留在 Git 历史与 GitHub Release 中

---

## 一、为什么是纯 Go 原生 UI

1.0.0 用 WebView2 渲染 HTML/JS，能快速复用上游交互，但带来三个问题：Windows 10 需要 WebView2 Runtime、页面与 Go 之间要靠 `Bind`/`Eval` 通信、WebView 事件线程里同步调用 `Eval` 容易自锁。

1.1.0 把窗口内容换成 `ui.View`，由 MyGo 的原生控件承担侧栏、标签栏、工具栏、对话框、查看器；照片网格用 `ui.Scroll` + `ui.Box.Draw` 自绘虚拟瀑布流。这样只为可见区域画图，不为几千张图创建控件，同时彻底移除 WebView2、HTML 与 JS。

1.2.0 在原生 UI 上接入 [Onevra](https://oneo-me.github.io/onevra/)：使用它温暖中性色 + 陶土色的主题、Sidebar、IconButton、TextBox、Switch、Button、Badge 和 Lucide 图标；网格仍自绘，以保留虚拟化性能。Onevra 也是 MyGo 生态的纯 Go 组件，没有引入新的 WebView、HTML 或浏览器运行时。

1.2.1 去掉系统标题栏和系统窗口按钮，窗口顶部改为 48 DIP 的 Onevra 自绘标题栏：左侧品牌，中部标签页与新建按钮，右侧自绘的最小化、最大化/还原、关闭。标题栏可拖动、双击最大化，最大化按钮保留 Windows 11 snap layouts；窗口仍保留系统缩放边缘。

1.2.2 接入 MyGo 的签名更新链路：`mygo.json` 配置公开的 GitHub 更新源与 Ed25519 公钥，应用使用 `native` updater 在后台检查、展示发布说明、下载和重启。发布由 GitHub Actions 在 `v*` 标签上构建安装包、更新归档与清单并上传，私钥只存在 GitHub secret 和本机备份中。

1.2.3 修复自绘标题栏后看不到更新入口的问题：原生菜单栏只能通过 Alt 调出，因此把“检查更新”常驻在标题栏；设置页新增“软件更新”区块，关于窗口也提供检查更新按钮。

1.2.4 修复设置与工具栏控件绑定丢失的问题：所有开关、滑块和下拉框改为直接绑定持久草稿字段，保存后真正写回设置；设置页改用 Onevra `Settings` 的分类、搜索和窄窗口布局，并补齐排序、缓存上限、自动播放等入口。ffmpeg 取帧改用隐藏窗口方式启动，不再弹出控制台。

构建 tag `mygo_nowebview` 让 MyGo 进入 native-only 模式。**纯 Go 构建必须带这个 tag**，否则 MyGo 会按默认 WebView 模式编译。

## 二、Onevra 集成

- 主题：`onevra.Palette(dark)` 提供暖白/暖黑的 surface、`#c5553a` 陶土色 accent、柔和的 border 与 focus ring；`app.go` 把它作为窗口唯一的 `ui.Theme`，所有原生控件、弹窗、菜单和自绘网格共享同一套 token。
- 组件：侧栏使用 `components.Sidebar` / `SidebarSection` / `SidebarItem`；工具按钮与标签关闭使用 `IconButton`；搜索框使用 `TextBox`（Lucide Search 前导图标）；递归/隐藏/设置开关使用 `Switch`；对话框按钮使用 `Primary/Secondary/Outline/Ghost`；设置页标题和缓存信息使用 `Badge`。
- 图标：全部使用 `source/icons/lucide` 的矢量图标，不引入字体图标或网络资源。
- 自绘标题栏：`TitleBarHidden` + `HideWindowButtons`，顶部 48 DIP 的 Onevra 标题栏包含品牌、标签页和窗口控制；`DragWindow` 支持拖动，双击空白区最大化，自绘的最大化/还原按钮带 Windows 11 snap layouts。标题栏与标签页共用同一行，侧栏不再重复品牌。
- 自绘部分：照片卡片、缩略图、视频角标、文件名、选中描边和悬停阴影仍由 `grid.go` 的 `Painter` 绘制；Onevra 提供卡片颜色、圆角、边框和强调色。
- 兼容性：Onevra 的 `go.mod` 基线是 MyGo v0.8.3，本项目继续使用 v0.10.0；两者的 `ui` API 在本项目用到的范围内兼容，`go test` 与 release 构建均已验证。

## 三、已实现的功能

| 功能 | 状态 | 说明 |
|---|---|---|
| 瀑布流网格 | ✅ | 自绘虚拟化：按窗口宽度、缩略图目标宽度与间距算列数，按宽高比分配最短列；只对可见区域请求缩略图；Onevra 风格卡片 + 悬停阴影 + 选中描边 |
| 文件夹浏览 / 递归 / 排序 / 隐藏文件 | ✅ | 名称（自然排序）、日期、大小、类型；递归最多 5 层、单次最多 6,000 项 |
| 多标签页 | ✅ | 每标签独立路径、排序、历史、搜索、选中状态与滚动位置；重启恢复 |
| 自绘标题栏 | ✅ | 无系统标题栏/窗口按钮；Onevra 品牌、标签、新建与窗口控制；拖动/双击最大化/最小化/最大化还原/关闭；保留缩放边缘与 snap layouts |
| 签名自动更新 | ✅ | MyGo `native` updater；后台每日检查 + 帮助菜单手动检查；Ed25519 校验更新归档/清单；安装到用户目录后下载、安装并重启 |
| 导航 | ✅ | 返回、上一级、同级上/下一个（只在含媒体文件夹间跳转） |
| 缩略图 | ✅ | JPEG 磁盘缓存、EXIF 方向校正、透明底合成、CatmullRom 缩放；同一张图并发生成时按缓存路径串行，避免写文件竞争 |
| 图片查看器 | ✅ | 适应/100%/缩放/旋转/平移、上一张/下一张、信息面板、文件操作；Onevra 图标按钮与信息面板 |
| 视频查看器 | ✅ | MyGo `ui.Video` + 原生解码；无法解码时提示并可用系统播放器打开 |
| 原生菜单 | ✅ | 文件/编辑/视图/转到/帮助；动作直接调用 App，经 `w.Update` 回主线程 |
| 文件操作 | ✅ | 回收站删除（`SHFileOperationW`）、永久删除、重命名（扩展名保护）、新建文件夹、资源管理器定位、默认程序打开、复制路径 |
| 设置 | ✅ | 主题、缩略图宽度、网格间距、侧栏/信息面板、隐藏/其他文件、递归、循环视频、缓存查看与清理；Onevra 开关与按钮 |
| 收藏 / 位置 / 盘符 / 文件夹树 | ✅ | 收藏夹、桌面/图片/下载/文档/视频/盘符；文件夹树惰性加载，新建目录后刷新 |
| 搜索 | ✅ | 当前文件夹内按文件名即时过滤；`Ctrl+F` 聚焦搜索框；Onevra TextBox |
| 拖入文件 | ✅ | 窗口接收文件/文件夹路径，打开所在目录 |

## 四、降级与未实现

| 项目 | 处理方式 |
|---|---|
| Finder 标签（xattr） | Windows 无等价机制；改为应用内“收藏” |
| HDR 动态范围 | 按 SDR 显示，不做色调映射 |
| QuickLook | 不存在；内置查看器承担预览 |
| HEIC / AVIF / RAW | Go 标准解码器不支持；查看器提示无法预览，可交系统关联程序打开 |
| 视频缩略图 | 依赖外部 `ffmpeg.exe`（PATH 或 exe 同级）；没有时画渐变+播放三角占位图 |
| 视频播放 | 依赖本机解码器；部分 MKV/HEVC 只能外部打开 |
| 超大图 | 查看器对超过 3,000 万像素的图使用 2,560px 预览，避免解码卡顿 |
| 右键八向手势 | **未迁移**。右键在原生版用于上下文菜单；八向手势需要单独的修饰键方案，当前不做 |
| 系统主题实时跟随 | 启动首帧检测系统深浅色；运行中在设置里切换主题立即生效，但系统主题变化需重启后读取 |
| 自动滚动播放、iCloud 相册等 macOS 集成 | 未移植 |

## 五、运行与构建

**安装 / 更新**：从 [Releases](https://github.com/tiramission/FlowVision-Windows/releases) 下载 `flowvision-setup-<version>-amd64.exe`，或运行 MyGo 生成的一行安装脚本：

```powershell
irm https://github.com/tiramission/FlowVision-Windows/releases/latest/download/install.ps1 | iex
```

安装包写入 `%LOCALAPPDATA%\Programs\FlowVision`，不需要管理员权限；安装版会在后台检查签名更新，并在“帮助 → 检查更新…”提供手动检查。

**直接运行**：安装版从开始菜单或 `%LOCALAPPDATA%\Programs\FlowVision\FlowVision.exe` 启动；也可以把文件夹或文件路径作为参数：

```powershell
& "$env:LOCALAPPDATA\Programs\FlowVision\FlowVision.exe" "D:\Photos"
```

单实例：第二次启动会把路径交给已运行的窗口并新增标签，不会开第二个进程。

**从源码构建**（需要 Go 1.27+；国内网络建议设代理）：

```powershell
$env:GOPROXY = "https://goproxy.cn,direct"
$env:GOFLAGS = "-tags=mygo_nowebview"
go tool mygo build -platform windows/amd64
```

产物在 `build\windows-amd64\`：`FlowVision.exe`、`flowvision-setup-<version>-amd64.exe`、更新归档、`update-windows-amd64.json`、`install.ps1`。分发和更新使用安装包；直接运行构建目录里的 exe 不支持自我更新。完整发布流程见 `docs\发布流程.md`。

直接 `go build` 仍可用于本地调试，但不会注入更新公钥，因此更新检查会提示该构建不可更新：

```powershell
go build -trimpath -tags mygo_nowebview -ldflags "-H=windowsgui -s -w" -o FlowVision-dev.exe .
```

调试构建（带本地测试端点，release 不含）：

```powershell
go build -tags "debug mygo_nowebview" -o fvdebug.exe .
```

数据目录：

- 设置、日志：`%APPDATA%\FlowVision`
- 缩略图缓存：`%LOCALAPPDATA%\FlowVision\thumbs`
- 安装目录：`%LOCALAPPDATA%\Programs\FlowVision`

## 六、快捷键

浏览（焦点不在输入框时）：`W` 上一级 · `S` 返回 · `A` 上一文件夹 · `D` 下一文件夹 · `F5` 刷新 · `Ctrl+F` 搜索 · `Ctrl+O` 打开文件夹 · `Ctrl+T` 新建标签 · `Ctrl+W` 关闭标签 · `Ctrl+B` 侧栏 · `Ctrl+R` 递归 · `Ctrl+=` / `Ctrl+-` 缩略图缩放 · `Ctrl+,` 设置 · `F1` 快捷键帮助

网格：方向键移动选中 · `Home`/`End` 首尾 · `Enter`/`Space` 打开查看器 · `Delete` 移入回收站 · `F2` 重命名 · `Ctrl+A` 全选 · `Esc` 取消选择 / 关闭弹窗

查看器：`←`/`→` 上一张/下一张 · `+`/`-` 缩放 · `0` 适应 · `1` 100% · `R` 顺时针旋转 · `Shift+R` 逆时针 · `I` 信息面板 · 滚轮缩放 · 拖动平移 · `Esc` 关闭

## 七、测试与验证

- **Go 测试全部通过**：`CGO_ENABLED=0 go test ./...` 与 `go test -tags mygo_nowebview .`。覆盖目录扫描/自然排序/递归/隐藏、同级导航、缩略图缓存与并发生成、EXIF、Settings 合并、Windows 保留名、文件操作，以及原生布局、Onevra 侧栏、Ctrl+F 搜索、标签关闭、搜索过滤、网格命中/键盘导航、右键菜单、设置分类/搜索/窄窗口/开关/滑块/保存、工具栏排序/递归开关、查看器浮层、自绘标题栏控件与 ffmpeg 隐藏窗口进程等回归。
- **真实窗口回归**：debug 构建的本地控制端点 + `CapturePage` 截图 + 真实鼠标点击，验证了 1320×850 与 760×500 布局、深/浅主题、空目录、11 项瀑布流、160 项滚动与滚动恢复、多标签溢出、查看器缩放/旋转/翻页/信息、设置滚动、单实例路径转发、同级/返回导航、搜索过滤、重命名/新建/回收站删除、菜单快捷键（`Ctrl+T/W/B/R/=/−`）与浏览键（`W/S/A/D`、`F5`）。
- **标题栏回归**：DPI-aware 实际窗口截图确认无系统标题栏；真实点击最小化（`IsIconic=true`）、最大化/还原（`maximized` 状态与客户区尺寸同步）、关闭（进程退出）；双击标题栏空白区最大化；拖动标题栏移动窗口；760×500 下品牌、标签、新建与三枚窗口按钮均可见可点击。
- **更新/构建回归**：本机 `go tool mygo build -platform windows/amd64` 成功生成 1.2.4 安装包；签名更新归档和 `update-windows-amd64.json` 由 GitHub Actions 使用仓库 secret 生成。MyGo 构建的应用可启动，状态栏显示 v1.2.4，自绘标题栏、标题栏“检查更新”按钮与窗口控制保持正常。GitHub Actions 发布后检查公开的 `releases/latest/download/update-windows-amd64.json`。

## 八、目录

```
main.go        应用入口、窗口、单实例、启动路径
app.go         App/Tab/Viewer 状态、异步加载、历史、选择、设置、缓存；Onevra 主题
view.go        侧栏、标签栏、工具栏、内容区、状态栏、浏览快捷键（Onevra 组件）
grid.go        自绘虚拟瀑布流、卡片、命中测试、选择、右键菜单、拖放
viewer.go      图片/视频查看器与 Onevra 信息面板
dialogs.go     重命名、新建、删除确认、帮助、关于（Onevra 对话框）
settings_ui.go 设置分类、搜索、控件与保存/恢复默认（Onevra Settings）
menu.go        原生菜单、检查更新入口
dir.go         目录扫描、排序、导航
thumb.go       缩略图生成、磁盘缓存、ffmpeg 取帧、缓存淘汰
meta.go        尺寸、EXIF、MediaSvc
actions.go     文件对话框、文件操作、系统集成
win32.go       隐藏属性、SHFileOperationW 回收站
settings.go    设置模型与持久化
debug.go       仅 debug build 启用的本地控制端点
native_test.go 原生 UI/布局测试
CHANGELOG.md   版本发布说明（更新窗口读取对应章节）
.github/       GitHub Actions 发布工作流
resources/     icon.png
docs/          移植说明、发布流程
```

## 九、来源与许可

原项目 [FlowVision](https://github.com/netdcy/FlowVision) 是 macOS Swift/AppKit 应用，GPL-3.0。本仓库是独立重写，沿用 GPL-3.0；未包含上游二进制或 AppKit 代码。

UI 设计语言与控件来自 [Onevra](https://github.com/oneo-me/onevra)（MIT），通过 Go module `github.com/oneo-me/onevra/source` 引入；应用图标与上游 FlowVision 图标分开处理。
