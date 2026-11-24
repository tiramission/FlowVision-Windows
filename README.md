# FlowVision for Windows

[![Release](https://img.shields.io/github/v/release/tiramission/FlowVision-Windows?color=c5553a&label=release)](https://github.com/tiramission/FlowVision-Windows/releases/latest)
[![License](https://img.shields.io/badge/license-GPL--3.0-c5553a)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Windows%2010%2F11%20x64-4b5563)](#下载与安装)

把本地图片和视频变成可浏览、可沉浸查看的瀑布流。FlowVision for Windows 是 [netdcy/FlowVision](https://github.com/netdcy/FlowVision)（macOS，Swift/AppKit）的 Windows 重写版：**Go + MyGo 原生 `ui` + Onevra**，不使用 WebView2、HTML 或 JavaScript，也不需要 cgo、Swift 或 Xcode。

当前版本 **1.2.14**：主窗口改为真正的左右分栏并设置 900×600 起的最小尺寸；侧栏、标题栏、内容区和底部状态区共用连续的根级发丝线；侧栏文件夹区改为当前文件夹为根的树，支持子目录开合、数量、空/加载状态与完整路径提示。

## 下载与安装

从 [Releases](https://github.com/tiramission/FlowVision-Windows/releases/latest) 下载安装包：

```text
flowvision-setup-<version>-amd64.exe
```

或者运行 MyGo 生成的一行安装脚本：

```powershell
irm https://github.com/tiramission/FlowVision-Windows/releases/latest/download/install.ps1 | iex
```

安装包写入 `%LOCALAPPDATA%\Programs\FlowVision`，按当前用户安装，不需要管理员权限。安装版启动后自动检查一次签名更新；发现新版本时打开应用内的中文更新面板，也可从标题栏云图标、帮助菜单、设置页或关于页手动检查。

## 功能

- **瀑布流浏览**：按窗口宽度和缩略图尺寸计算列数，按图片宽高比分配最短列；只绘制可见卡片、只请求可见缩略图，大目录也能流畅滚动。
- **文件夹与标签页**：子文件夹作为卡片进入；名称、日期、大小、类型排序；递归模式生成纯媒体墙；多标签页各自保存路径、历史、排序、搜索、选中和滚动位置。
- **图片查看器**：适应窗口、实际大小、缩放、旋转、拖动平移；长按左键切到 100%，长按右键切到适应窗口；鼠标侧键切换上一张/下一张，侧键方向可在设置里校准；`C` 或控制栏准星按钮一键把图片移回画布中心；滚轮和触控板捏合缩放以鼠标指针位置为锚点；`100%` 是真正的 1:1 像素比。
- **视频查看器**：原生解码与播放，支持拖动进度、±10 秒、0.5×–2× 倍速、静音和循环；鼠标侧键同样可切换上一个/下一个媒体；信息面板显示分辨率、帧率与随播放位置变化的实时码率；沉浸时光标隐藏后，在视频画面上移动鼠标也会立即恢复。
- **主窗口最大化与全屏**：自绘标题栏把最大化/还原和全屏/退出全屏分开，`F11` 切换全屏。最大化遵循系统工作区，全屏覆盖整个显示器；两种模式互不串用。
- **查看器沉浸**：查看器打开后只在主窗口内铺满画布，不再改变主窗口最大化/全屏状态。沉浸时标题栏、控制栏和信息栏退出布局，只有鼠标进入顶部或底部 64 DIP 边缘区域才浮出顶部/底部控件，离开边缘 2.5 秒后隐藏；`F11` 在查看器内切换沉浸，关闭查看器不会改变主窗口状态。
- **独立光标计时**：查看器沉浸时鼠标静止 2.5 秒才隐藏，移动、点击、滚轮或缩放都会立即恢复；浮层显示/隐藏不再直接控制指针。
- **快捷键帮助**：`F1` 打开分类帮助，可按全部、浏览、图片查看器、视频与窗口筛选；快捷键按场景分组为卡片，并显示条目数。
- **鼠标侧键校准**：设置 → 控制里为查看器的“上一张 / 下一张”分别校准实体侧键；可交换方向，兼容旧设置文件。
- **文件操作**：回收站删除、永久删除、重命名、新建文件夹、资源管理器定位、默认程序打开、复制路径；右键菜单与键盘操作并存。
- **原生界面**：Onevra 的暖白/暖黑主题、陶土色强调色、Sidebar、IconButton、TextBox、Switch、Badge 与 Lucide 矢量图标；照片网格由 Go 侧的 `ui.Box.Draw` 虚拟化绘制。
- **签名更新**：MyGo `Updater` 校验 Ed25519 签名，支持增量补丁；应用内更新面板负责检查、显示中文发布说明、下载进度、安装与重启，不再使用 MyGo 自带更新窗口。

## 快捷键

| 场景 | 按键 |
|---|---|
| 浏览 | `W` 上一级 · `S` 返回 · `A` 上一文件夹 · `D` 下一文件夹 · `F5` 刷新 |
| 标签与搜索 | `Ctrl+O` 打开文件夹 · `Ctrl+T` 新建标签 · `Ctrl+W` 关闭标签 · `Ctrl+F` 搜索 |
| 布局 | `Ctrl+B` 侧栏 · `Ctrl+R` 递归 · `Ctrl+=` / `Ctrl+-` 缩略图大小 · `Ctrl+,` 设置 · `F11` 全屏 · `F1` 帮助 |
| 网格 | 方向键移动 · `Home`/`End` 首尾 · `Enter`/`Space` 打开 · `Delete` 回收站 · `F2` 重命名 · `Ctrl+A` 全选 |
| 图片查看器 | `←`/`→` 翻页 · `+`/`-` 缩放 · `0` 适应窗口 · `1` 实际大小 · `C` 居中 · `R`/`Shift+R` 旋转 · `I` 信息 · `F11` 沉浸 · `Esc` 关闭 |
| 查看器鼠标 | 长按左键 100% · 长按右键 适应窗口 · 侧键（后退/前进）上一张/下一张 · 滚轮/触控板缩放以指针为锚点 |
| 视频查看器 | `Space` 播放/暂停 · `J`/`L` 或 `Shift+←`/`Shift+→` ±10 秒 · `M` 静音 · `F11` 切换沉浸 |

## 技术实现

- **纯 Go 原生窗口**：`ui.View` 构建整个界面；构建 tag `mygo_nowebview` 让 MyGo 进入 native-only 模式，避免引入 WebView 运行时。
- **虚拟化网格**：`grid.go` 只对可见区域绘制卡片、缩略图、文件名、选中描边和悬停阴影，不为几千张图片创建控件。
- **缩略图管线**：JPEG 磁盘缓存、EXIF 方向校正、透明底合成、CatmullRom 缩放；同一张图并发生成时按缓存路径串行，避免写文件竞争。
- **异步媒体信息**：尺寸、EXIF、MP4/MOV sample table、帧率和滚动码率在后台读取；视频先取容器 metadata，再启动播放器，避免文件句柄冲突。
- **自绘窗口**：`TitleBarHidden` + `HideWindowButtons`，48 DIP 标题栏承担拖动、双击最大化、最小化/最大化/全屏/关闭和 Windows 11 snap layouts；全屏状态由窗口自身维护。
- **更新链路**：`mygo.json` 配置 GitHub 更新源与公钥；`mygo.Updater.Check` 返回新版本，`Update.Install` 下载并校验签名归档或增量补丁，成功后 `mygo.App.Relaunch()`。

## 从源码构建

需要 Go 1.27+。国内网络建议设置代理：

```powershell
$env:GOPROXY = "https://goproxy.cn,direct"
$env:GOFLAGS = "-tags=mygo_nowebview"
go tool mygo build -platform windows/amd64
```

产物在 `build\windows-amd64\`：`FlowVision.exe`、`flowvision-setup-<version>-amd64.exe`、更新归档、`update-windows-amd64.json`、`install.ps1`。完整发布流程见 [`docs/发布流程.md`](docs/发布流程.md)。

直接 `go build` 可用于本地调试，但不会注入更新公钥，因此更新面板会说明此构建不可自更新：

```powershell
go build -trimpath -tags mygo_nowebview -ldflags "-H=windowsgui -s -w" -o FlowVision-dev.exe .
```

带本地调试端点的构建（release 不包含）：

```powershell
go build -tags "debug mygo_nowebview" -o fvdebug.exe .
```

数据目录：

- 设置、日志：`%APPDATA%\FlowVision`
- 缩略图缓存：`%LOCALAPPDATA%\FlowVision\thumbs`
- 安装目录：`%LOCALAPPDATA%\Programs\FlowVision`

## 测试

```powershell
go test -tags mygo_nowebview ./...
```

测试覆盖目录扫描、自然排序、递归与隐藏文件、缩略图缓存与并发生成、EXIF、设置合并、Windows 保留名、文件操作、文件夹卡片、查看器翻页与实际缩放比、**指针锚定缩放**、**长按左/右键缩放与拖动取消**、**侧键翻页**、**沉浸边缘命中**、**指针移动恢复光标**、**独立光标计时**、视频控件与码率、自绘标题栏、Onevra 设置页和 ffmpeg 隐藏窗口进程。

真实窗口回归通过 debug 构建的本地端点完成，覆盖 1320×850 与 760×500 布局、深/浅主题、瀑布流滚动与恢复、多标签、主窗口最大化/全屏、查看器沉浸、设置、重命名/新建/回收站、菜单快捷键和窗口控制。

## 已知限制

| 项目 | 处理方式 |
|---|---|
| Finder 标签（xattr） | Windows 没有等价机制，改为应用内“收藏” |
| HDR | 按 SDR 显示，不做色调映射 |
| HEIC / AVIF / RAW | Go 标准解码器不支持；查看器提示无法预览，可交系统关联程序打开 |
| 视频缩略图 | 依赖外部 `ffmpeg.exe`（PATH 或 exe 同级）；缺失时显示渐变占位图 |
| 视频播放 | 依赖本机解码器；部分 MKV/HEVC 只能外部打开 |
| 超大图 | 超过 3,000 万像素时使用 2,560 px 预览，避免解码卡顿 |
| 右键八向手势 | 未迁移；右键用于原生上下文菜单 |
| 系统主题实时跟随 | 启动时检测系统深浅色；运行中可在设置里立即切换主题 |

## 项目结构

```text
main.go          应用入口、窗口、单实例、启动路径、自动更新检查
app.go           App/Tab/Viewer 状态、异步加载、历史、选择、设置、缓存与主题
view.go          侧栏、标签栏、工具栏、内容区、状态栏、浏览快捷键
grid.go          自绘虚拟瀑布流、卡片、命中测试、选择、右键菜单、拖放
viewer.go        图片/视频查看器、主窗口内沉浸浮层、指针锚定缩放、长按缩放与侧键翻页
update_ui.go     应用内中文更新面板、检查/下载/安装状态
dialogs.go       重命名、新建、删除确认、帮助、关于
settings_ui.go   Onevra 设置分类、搜索、控件与保存
menu.go          原生菜单与更新入口
dir.go           目录扫描、排序、导航
thumb.go         缩略图生成、磁盘缓存、ffmpeg 取帧、缓存淘汰
meta.go          尺寸、EXIF、MediaSvc
video_meta.go    MP4/MOV sample table、帧率与滚动视频码率
actions.go       文件对话框、文件操作、系统集成
win32.go         隐藏属性、SHFileOperationW 回收站、沉浸浮层鼠标边缘检测
settings.go      设置模型与持久化
debug.go         仅 debug build 启用的本地控制端点
viewer_zoom_test.go 指针锚定缩放测试
native_test.go   原生 UI/布局/查看器测试
CHANGELOG.md     版本发布说明（更新面板读取对应章节）
docs/            移植说明与发布流程
.github/         GitHub Actions 发布工作流
```

## 版本与文档

- 版本历史：[`CHANGELOG.md`](CHANGELOG.md)
- 移植说明：[`docs/移植说明.md`](docs/移植说明.md)
- 发布流程：[`docs/发布流程.md`](docs/发布流程.md)

## 来源与许可

原项目 [FlowVision](https://github.com/netdcy/FlowVision) 是 macOS Swift/AppKit 应用，GPL-3.0。本仓库是独立重写，沿用 GPL-3.0，未包含上游二进制或 AppKit 代码；第三方组件许可见 [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md)。

UI 设计语言与控件来自 [Onevra](https://github.com/oneo-me/onevra)（MIT），通过 Go module `github.com/oneo-me/onevra/source` 引入。
