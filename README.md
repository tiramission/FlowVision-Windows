# FlowVision for Windows

[![Release](https://img.shields.io/github/v/release/tiramission/FlowVision-Windows?color=c5553a&label=release)](https://github.com/tiramission/FlowVision-Windows/releases/latest)
[![License](https://img.shields.io/badge/license-GPL--3.0-c5553a)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Windows%2010%2F11%20x64-4b5563)](#下载与安装)

把本地图片和视频变成可浏览、可沉浸查看的瀑布流。FlowVision for Windows 是 [netdcy/FlowVision](https://github.com/netdcy/FlowVision)（macOS，Swift/AppKit）的 Windows 重写版：**Go + MyGo 原生 `ui` + Onevra**，不使用 WebView2、HTML 或 JavaScript，也不需要 cgo、Swift 或 Xcode。

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
- **媒体筛选**：命令栏常驻“全部 / 图片 / 视频”分段控件；每个标签页独立记忆筛选，文件夹始终保留，筛选与搜索叠加，状态栏显示 N / M 项。
- **图片查看器**：默认打开 2560 px 快速预览，打开瞬间先用网格缩略图占位；相邻预览就绪后在后台自动解码当前图的原图（可在设置里关闭）。支持按需 1:1 真原图（不设像素上限）、适应窗口、实际大小、缩放、旋转、拖动平移。`100%` 是真正的 1:1：按屏幕**物理像素**算，150% 缩放的屏上也是 1 个图像像素对 1 个设备像素。滚轮和触控板捏合以指针为锚点缩放，且只在图片实际区域触发；拖动和精确滚动不会把图片完全移出画布。
- **底栏读数**：查看器底栏常驻三项——当前是**缩略图**、**大图**（2560 预览）还是**原图**（长边不超过 2560 的图预览就是原图，直接显示原图），**前后图预取**进度（`预取 3/4`），以及**缩放比**。沉浸模式下控制栏淡出后，三项缩成照片左下角一枚深色胶囊，静置 2 秒隐去，任何一项变化立刻回来；胶囊穿透点击，不挡拖动。
- **长按手势**：左键长按临时放大到 100% 并可拖动，松开回到原缩放；右键长按临时适合窗口，同样松开恢复。两者都能在设置里改成「保持」或关闭。
- **信息卡**：查看器窗口模式显示文件名、路径、尺寸/大小或时长/大小概览，以及文件格式、修改时间、EXIF 或视频参数；底部提供打开、定位和复制路径。
- **视频查看器**：原生解码与播放，支持拖动进度、±10 秒、0.5×–2× 倍速、静音和循环；信息卡显示时长、大小、分辨率、帧率与随播放位置变化的实时码率。
- **主窗口最大化与全屏**：自绘标题栏把最大化/还原和全屏/退出全屏分开，`F11` 切换全屏。最大化遵循系统工作区，全屏覆盖整个显示器；两种模式互不串用。
- **查看器沉浸**：可配置打开时是否自动沉浸（默认开启）；关闭后以带标题栏的窗口进入，仍可按 `F11` 切换。沉浸时只在主窗口内铺满画布，不改主窗口状态；鼠标进入顶部或底部边缘才浮出控件，离开 2.5 秒后隐藏。
- **快捷键帮助**：`F1` 打开分类帮助，可按全部、浏览、图片查看器、视频与窗口筛选。
- **侧键与预加载**：为「上一张 / 下一张」分别录制单击和长按快捷键，物理侧键可直接按下识别；查看器可预加载前后各若干张的 2560 预览（默认 2 张，384 MB 预算）；鼠标在左右边带停留会优先预取该方向相邻图片。
- **远程位置（只读）**：侧边栏「远程」可添加 SMB 或 WebDAV 位置，连接完全在应用进程内建立——不映射盘符、不创建系统会话、不写凭据管理器，资源管理器看不到也访问不了。可为每个连接起别名，侧边栏、标签页、窗口标题和路径栏显示别名，悬浮显示完整地址。远程位置只读：写操作在右键菜单和快捷键里都不出现，服务层也会拒绝。远程图片照常生成缩略图与预览，远程视频通过应用内的本机回环 HTTP 服务播放（支持拖动进度）。
- **收藏夹（虚拟目录）**：侧边栏「收藏」区是一个固定入口，点进去就是和普通文件夹一样的瀑布流，只收图片和视频。文件不会被复制或移动，条目只是指向原文件，所以本机、NAS、WebDAV 上的照片可以混在同一个视图里；在收藏夹里打开查看器，前后翻页就在收藏集内。
- **收藏的路径**：「收藏」区里收藏夹下面的条目是文件夹位置，点路径栏的星标即可加入，用来把很深的目录留成一次点击。它和收藏夹是两份独立的清单：收藏文件夹不会出现在收藏夹里，也不计入它的条目数。
- **多选**：`Ctrl+点击`、`Shift+点击`、`Ctrl+M` 或右键「选择…」进入多选；「选择…」提供全选、按类型选择、选择同类、反向选择。多选栏提供收藏、复制路径、删除和取消选择。
- **文件操作**：回收站删除、永久删除、重命名、新建文件夹、资源管理器定位、默认程序打开、复制路径；右键菜单与键盘操作并存。
- **补载状态**：状态栏右侧常驻一个胶囊，统一以「补载」开头说明当前文件夹的缩略图补载：进行中显示百分比（`补载 42%`），可点开查看进度并取消；其余状态说明「为什么不补」——`就绪`、`按需`（网络文件夹或超过张数上限）、`关闭`、`已停止`、`失败`，各配一句说明。
- **原生界面**：Onevra 的暖白画廊/中性暗房主题、陶土色强调色、Sidebar、IconButton、TextBox、Switch、Badge 与 Lucide 矢量图标；照片网格由 Go 侧的 `ui.Box.Draw` 虚拟化绘制。界面字体为 MiSans 官方可变字体（一个文件覆盖 100–900 全部字重），随安装包分发。
- **签名更新**：MyGo `Updater` 校验 Ed25519 签名，支持增量补丁；应用内更新面板负责检查、显示中文发布说明、下载进度、安装与重启。

## 快捷键

| 场景 | 按键 |
|---|---|
| 浏览 | `W` 上一级 · `S` 返回 · `A` 上一文件夹 · `D` 下一文件夹 · `F5` 刷新 · `Ctrl+M` 多选 · `Space` 收藏 |
| 标签与搜索 | `Ctrl+O` 打开文件夹 · `Ctrl+T` 新建标签 · `Ctrl+W` 关闭标签 · `Ctrl+F` 搜索 |
| 布局 | `Ctrl+B` 侧栏 · `Ctrl+R` 递归 · `Ctrl+=` / `Ctrl+-` 缩略图大小 · `Ctrl+,` 设置 · `F11` 全屏 · `F1` 帮助 |
| 网格 | 方向键移动 · `Home`/`End` 首尾 · `Enter`/`Space` 打开 · `Delete` 回收站 · `F2` 重命名 · `Ctrl+A` 全选 |
| 图片查看器 | `←`/`→` 翻页 · `Ctrl+←`/`Ctrl+→` 跨文件夹 · `+`/`-` 缩放 · `0` 适应窗口 · `1` 实际大小 · `C` 居中 · `R`/`Shift+R` 旋转 · `I` 信息 · `F11` 沉浸 · `Esc` 关闭 |
| 查看器鼠标 | 长按左键（指针在图片内）临时 100% · 长按右键 临时适合窗口 · 侧键上一张/下一张 · 滚轮/触控板以指针为锚点缩放 |
| 视频查看器 | `Space` 播放/暂停 · `J`/`L` 或 `Shift+←`/`Shift+→` ±10 秒 · `M` 静音 · `F11` 切换沉浸 |

## 已知限制

| 项目 | 处理方式 |
|---|---|
| Finder 标签（xattr） | Windows 没有等价机制，改为应用内「收藏」 |
| HDR | 按 SDR 显示，不做色调映射 |
| HEIC / AVIF / RAW | 缩略图：有系统 codec 时走 WIC/Shell；无 codec 时显示占位。查看器仍主要依赖 Go 解码，无法预览时交系统关联程序打开 |
| 视频缩略图 | 依赖外部 `ffmpeg.exe`（PATH 或 exe 同级）；缺失时显示渐变占位图 |
| 视频播放 | 依赖本机解码器；部分 MKV/HEVC 只能外部打开 |
| 超大图 | 默认显示最长边 2,560 px 预览；>80 MP 优先走系统缩略图 provider，失败再回退 WIC/Shell；1:1 按需解码原图，受单张纹理和可用内存限制 |
| 右键八向手势 | 未迁移；右键用于原生上下文菜单 |
| 系统主题实时跟随 | 启动时检测系统深浅色；运行中可在设置里立即切换主题 |
| 远程位置写操作 | 只读：不提供重命名、新建、删除、移动；需要写入时请用系统挂载方式 |
| 收藏夹 | 虚拟目录，只收图片和视频；不复制文件，原文件被删除或移动后条目自动略过（不会从清单里删除） |
| 远程位置密码 | 「记住密码」用 DPAPI 按当前 Windows 账户加密存到应用数据目录，换账户或换机器后需重新输入 |
| 远程位置证书 | HTTPS 默认校验证书；NAS 用自签名、过期或名字不符的证书时，可在连接设置里打开「跳过证书验证」 |
| 远程视频 | 经本机回环 HTTP 服务播放（随机端口 + 随机路径令牌，闲置 3 分钟自动关闭）；播放本身仍依赖本机解码器 |

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
- 缩略图缓存：`%LOCALAPPDATA%\FlowVision\thumbs`（网格卡片）与 `%LOCALAPPDATA%\FlowVision\previews`（查看器 2560 预览），共用一个上限额度
- 安装目录：`%LOCALAPPDATA%\Programs\FlowVision`

## 测试

```powershell
go test -tags mygo_nowebview ./...
```

测试覆盖目录扫描、自然排序、递归与隐藏文件、缩略图缓存与并发生成、EXIF、设置合并、Windows 保留名、文件操作、文件夹卡片、查看器翻页与缩放、指针锚定缩放、长按手势、侧键翻页、沉浸边缘命中、视频控件与码率、自绘标题栏、Onevra 设置页和 ffmpeg 隐藏窗口进程。

真实窗口回归通过 debug 构建的本地端点完成（`/ready`、`/state`、`/shot`、`/command`），覆盖多种窗口尺寸与主题、瀑布流滚动与恢复、多标签、最大化/全屏、查看器沉浸、设置、文件操作、菜单快捷键和窗口控制。

性能基准需要本地语料，默认不随仓库分发：

```powershell
python tools/make_thumb_corpus.py --full
pwsh tools/bench_thumb.ps1 -Go "C:\path\to\go.exe" -Backend auto
pwsh tools/bench_viewer.ps1 -Go "C:\path\to\go.exe" -Runs 10 -Out docs\viewer-bench-app.json
```

诊断用的环境变量：`FLOWVISION_THUMB_BACKEND=go|wic|shell|embedded` 切换缩略图后端（默认 `auto`）；`FLOWVISION_VIEWER_PREVIEW=fast|hq|legacy`、`FLOWVISION_VIEWER_SCALER=fast|hq`、`FLOWVISION_VIEWER_CACHE=off`、`FLOWVISION_UI_FONT` 用于查看器与字体的对照实验；`FLOWVISION_VIEWER_PRELOAD=off` 只用于基准隔离。

## 版本与文档

- 版本历史：[`CHANGELOG.md`](CHANGELOG.md)
- 文档索引：[`docs/README.md`](docs/README.md)——发布流程、移植说明、技术细节、历史性能记录

## 来源与许可

原项目 [FlowVision](https://github.com/netdcy/FlowVision) 是 macOS Swift/AppKit 应用，GPL-3.0。本仓库是独立重写，沿用 GPL-3.0，未包含上游二进制或 AppKit 代码；第三方组件许可见 [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md)。

UI 设计语言与控件来自 [Onevra](https://github.com/oneo-me/onevra)（MIT），通过 Go module `github.com/oneo-me/onevra/source` 引入。
