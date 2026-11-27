# FlowVision for Windows

[![Release](https://img.shields.io/github/v/release/tiramission/FlowVision-Windows?color=c5553a&label=release)](https://github.com/tiramission/FlowVision-Windows/releases/latest)
[![License](https://img.shields.io/badge/license-GPL--3.0-c5553a)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Windows%2010%2F11%20x64-4b5563)](#下载与安装)

把本地图片和视频变成可浏览、可沉浸查看的瀑布流。FlowVision for Windows 是 [netdcy/FlowVision](https://github.com/netdcy/FlowVision)（macOS，Swift/AppKit）的 Windows 重写版：**Go + MyGo 原生 `ui` + Onevra**，不使用 WebView2、HTML 或 JavaScript，也不需要 cgo、Swift 或 Xcode。

当前版本 **1.9.0**：界面与查看器 chrome 统一改用 MiSans（官方可变字体 `MiSans VF`，100–900 全字重，随安装包分发），瀑布流卡片文字与查看器 HUD 也一并跟随主题字体；1.8.0 的后台任务面板与超大图 100% 缩放修复保持不变。

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
- **图片查看器**：默认打开 2560 px 快速预览，打开瞬间先用网格缩略图占位；相邻预览就绪后在后台自动解码当前图的原图（可在设置里关闭），按 `1` 或放大到 100% 立即可用；支持按需 1:1 真原图（不设 50 MP 门禁）；适应窗口、实际大小、缩放、旋转、拖动平移；长按左键（指针在图片内）按鼠标指针位置缩放到 100%，已到 100% 不再放大；长按右键恢复适应窗口并固定居中，不受指针位置影响；鼠标侧键切换上一张/下一张，单击和长按可分别配置；`C` 或控制栏准星按钮一键把图片移回画布中心；滚轮和触控板捏合缩放以鼠标指针位置为锚点，且只在图片实际区域触发；`100%` 是真正的 1:1 像素比，超大图同样能到 100%（缩放上限随图片尺寸放宽）；拖动和精确滚动不会把图片完全移出画布。查看器使用底部单槽状态提示，不堆叠、不遮挡画面中心，可在设置里切换 off/auto/always。
- **信息卡**：查看器窗口模式显示文件名、路径、尺寸/大小或时长/大小概览，以及文件格式、修改时间、EXIF 或视频参数；底部提供打开、定位和复制路径。参数按单行键值排列，长列表可滚动。
- **视频查看器**：原生解码与播放，支持拖动进度、±10 秒、0.5×–2× 倍速、静音和循环；鼠标侧键同样可切换上一个/下一个媒体；信息卡显示时长、大小、分辨率、帧率与随播放位置变化的实时码率；沉浸时光标隐藏后，在视频画面上移动鼠标也会立即恢复。
- **主窗口最大化与全屏**：自绘标题栏把最大化/还原和全屏/退出全屏分开，`F11` 切换全屏。最大化遵循系统工作区，全屏覆盖整个显示器；两种模式互不串用。
- **查看器沉浸**：设置 → 控制 → 查看器可配置打开时是否自动沉浸，默认开启；关闭后以带标题栏的窗口进入，仍可按 F11 手动切换。沉浸时查看器只在主窗口内铺满画布，不改主窗口最大化/全屏状态；标题栏、控制栏和信息栏退出布局，鼠标进入顶部或底部 64 DIP 边缘才浮出控件，离开边缘 2.5 秒后隐藏。
- **独立光标计时**：查看器沉浸时鼠标静止 2.5 秒才隐藏，移动、点击、滚轮或缩放都会立即恢复；浮层显示/隐藏不再直接控制指针。
- **快捷键帮助**：`F1` 打开分类帮助，可按全部、浏览、图片查看器、视频与窗口筛选；快捷键按场景分组为卡片，并显示条目数。
- **侧键与预加载**：设置 → 控制里为查看器的“上一张 / 下一张”分别录制单击和长按快捷键，物理侧键可直接按下识别；查看器可预加载前后各若干张的 2560 预览（默认 2 张，384 MB 预算，不预取原图），当前图的原图由“自动载入当前图原图”在预览完成后补上；鼠标在左右边带停留 220 ms 会优先预取该方向相邻图片，`ViewerPreload=0` 时同步关闭。
- **远程位置（只读）**：侧边栏「远程」可添加 SMB 或 WebDAV 位置，连接完全在应用进程内建立——不映射盘符、不创建系统会话、不写凭据管理器，资源管理器看不到也访问不了。地址按 `smb://主机/共享` / `webdav://主机:端口/路径` 显示，面包屑、标签页和窗口标题都用这种形式。远程位置只读：重命名、新建、删除等写操作在右键菜单和快捷键里都不出现，服务层也会拒绝；「在资源管理器中显示」「用默认应用打开」同样隐藏，因为系统里没有任何东西能解析这个路径。远程图片照常生成缩略图与 2560 预览，远程视频通过应用内的本机回环 HTTP 服务交给播放器（支持拖动进度），ffmpeg 取帧也走同一个服务。
- **文件操作**：回收站删除、永久删除、重命名、新建文件夹、资源管理器定位、默认程序打开、复制路径；右键菜单与键盘操作并存。
- **原生界面**：Onevra 的暖白画廊/中性暗房主题、陶土色强调色、Sidebar、IconButton、TextBox、Switch、Badge 与 Lucide 矢量图标；照片网格由 Go 侧的 `ui.Box.Draw` 虚拟化绘制。
- **界面字体**：`resources/fonts` 里的 MiSans（官方可变字体 `MiSans VF.ttf`，一个文件覆盖 100–900 全部字重）在启动时用 `ui.RegisterFont` 注册为 DirectWrite 内存字体集合（Windows 10 1703+），字重轴覆盖 100–900，界面用到的 400 / 500 / 600 / 700 都是真字重；缺少文件时回退系统字体，系统装过 MiSans 也会自动启用，`FLOWVISION_UI_FONT` 可临时指定其他字体对照。许可与体积见 `resources/fonts/README.md`。
- **签名更新**：MyGo `Updater` 校验 Ed25519 签名，支持增量补丁；应用内更新面板负责检查、显示中文发布说明、下载进度、安装与重启，不再使用 MyGo 自带更新窗口。
- **后台任务**：状态栏右侧在耗时任务运行时出现「后台 N」，点开可看进度并取消；目前覆盖整目录缩略图补载与查看器原图载入。1 秒内的短任务不出现，完成的消息 4 秒后自动消失，失败的需要手动关闭。查看器沉浸时改用底部 HUD 单行显示进度，不额外弹层。

## 快捷键

| 场景 | 按键 |
|---|---|
| 浏览 | `W` 上一级 · `S` 返回 · `A` 上一文件夹 · `D` 下一文件夹 · `F5` 刷新 |
| 标签与搜索 | `Ctrl+O` 打开文件夹 · `Ctrl+T` 新建标签 · `Ctrl+W` 关闭标签 · `Ctrl+F` 搜索 |
| 布局 | `Ctrl+B` 侧栏 · `Ctrl+R` 递归 · `Ctrl+=` / `Ctrl+-` 缩略图大小 · `Ctrl+,` 设置 · `F11` 全屏 · `F1` 帮助 |
| 网格 | 方向键移动 · `Home`/`End` 首尾 · `Enter`/`Space` 打开 · `Delete` 回收站 · `F2` 重命名 · `Ctrl+A` 全选 |
| 图片查看器 | `←`/`→` 翻页 · `+`/`-` 缩放 · `0` 适应窗口 · `1` 实际大小 · `C` 居中 · `R`/`Shift+R` 旋转 · `I` 信息 · `F11` 沉浸 · `Esc` 关闭 |
| 查看器鼠标 | 长按左键（指针在图片内）100% · 长按右键 适应窗口 · 侧键（后退/前进）上一张/下一张 · 滚轮/触控板在图片上缩放以指针为锚点 |
| 视频查看器 | `Space` 播放/暂停 · `J`/`L` 或 `Shift+←`/`Shift+→` ±10 秒 · `M` 静音 · `F11` 切换沉浸 |

## 技术实现

- **纯 Go 原生窗口**：`ui.View` 构建整个界面；构建 tag `mygo_nowebview` 让 MyGo 进入 native-only 模式，避免引入 WebView 运行时。
- **虚拟化网格**：`grid.go` 只对可见区域绘制卡片、缩略图、文件名、选中描边和悬停阴影，不为几千张图片创建控件。
- **缩略图管线**：Windows Imaging Component（WIC）优先读取 JPEG/PNG/TIFF/BMP/GIF/WebP，内嵌缩略图或 `IWICBitmapScaler`（HighQualityCubic）缩放到目标尺寸；HEIC/RAW/ICO 优先系统 Shell provider，普通 JPEG 不依赖 Shell；WIC/Shell 失败自动回退纯 Go。EXIF IFD1 内嵌缩略图先发 160 px preview，最终图到达后替换；磁盘 JPEG q84、`v2` 后端版本、内存 1200 条目 + 512 MB 字节双上限、近似 LRU；缓存按用途分两个目录：`thumbs`（网格卡片）与 `previews`（查看器 2560 预览），共用设置里的容量额度，启动时把旧版混在 `thumbs` 里的预览文件搬到 `previews`；透明图预乘后合成到中性底色。可见卡片排完之后，后台以两路低优先级继续把当前文件夹剩余的缩略图写进磁盘缓存（只写缓存、不保留位图），切换文件夹或关闭开关立即停止。
- **查看器管线**：最长边 2560、`ApproxBiLinear` 快缩放、`ui.NewBitmap` 直接上屏、JPEG q90 异步写缓存（存 `previews` 目录）；缓存命中走 JPEG 解码后更新一个持久 bitmap 槽，避免每次打开都新建 17 MB 纹理。邻居预加载单 worker、384 MB 字节预算、当前图跳过，只取 2560 预览；当前图的原图在邻居预览排空后自动后台解码（全局串行一张，切图即放弃队列里的其余解码）；1:1 对任意尺寸按需解码原图，当前图保留全分辨率 bitmap，关闭/切图立即释放并回收大图临时堆。
- **远程读取层**：`remote.go` 定义统一后端接口（列目录、打开流、HTTP 转发、关闭），SMB2 用 `go-smb2` 在进程内建会话，WebDAV 用 `net/http` 发 PROPFIND 与带 Range 的 GET；连接按「主机 + 共享 + 账号」进池，闲置 90 秒回收，回收时的登出带 2 秒上限，服务器不响应就直接断开底层 TCP。路径是虚拟的（`smb://主机/共享/子目录`），`cleanDisplayPath` 让虚拟路径绕开 `filepath.Clean`（否则会变成 `smb:\主机\...`），缩略图、预览、EXIF、ICO 解码都改走 `openItemFile` 拿流，所以远程与本地共用同一套解码管线。
- **远程视频**：Media Foundation 能用 `http://` 但用不了应用私有的 SMB 会话，因此在 `127.0.0.1:0` 上起一个只服务远程路径的 HTTP 服务（随机端口、随机路径令牌、只接受 GET/HEAD），把 `Range` 原样转给后端，空闲 3 分钟自动关闭；ffmpeg 取帧也把该 URL 当输入。
- **异步媒体信息**：尺寸、EXIF、MP4/MOV sample table、帧率和滚动码率在后台读取；视频先取容器 metadata，再启动播放器，避免文件句柄冲突。远程文件只读一次 512 KiB 前缀，尺寸与 EXIF 都从这段数据里出。
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
- 缩略图缓存：`%LOCALAPPDATA%\FlowVision\thumbs`（网格卡片）与 `%LOCALAPPDATA%\FlowVision\previews`（查看器 2560 预览），共用一个上限额度
- 安装目录：`%LOCALAPPDATA%\Programs\FlowVision`

## 测试

```powershell
go test -tags mygo_nowebview ./...
```

缩略图性能/格式矩阵需要本地语料，默认不随仓库分发：

```powershell
python tools/make_thumb_corpus.py --full
pwsh tools/bench_thumb.ps1 -Go "C:\path\to\go.exe" -Backend auto
pwsh tools/bench_thumb.ps1 -Go "C:\path\to\go.exe" -Backend go
```

可以用 `FLOWVISION_THUMB_BACKEND=go|wic|shell|embedded|dct` 单独切换后端；正式应用默认 `auto`。

查看器性能基准需要先构建 debug 版本；脚本会启动临时实例、记录分项与内存，并写出 JSON：

```powershell
pwsh tools/bench_viewer.ps1 -Go "C:\path\to\go.exe" -Runs 10 -Out docs\viewer-bench-app.json
pwsh tools/bench_viewer.ps1 -Go "C:\path\to\go.exe" -Warm -Runs 10 -Out docs\viewer-bench-app-warm.json
pwsh tools/bench_viewer.ps1 -Runs 50 -Out docs\viewer-bench-app-cycles.json
go test -run '^$' -bench BenchmarkViewerPreview -benchtime=20x -count=1 .
```

`FLOWVISION_VIEWER_PREVIEW=fast|hq|legacy`、`FLOWVISION_VIEWER_SCALER=fast|hq`、`FLOWVISION_VIEWER_CACHE=off`、`FLOWVISION_VIEWER_HUD=off|auto|always|debug` 可用于诊断；`FLOWVISION_VIEWER_PRELOAD=off` 只用于基准隔离。

测试覆盖目录扫描、自然排序、递归与隐藏文件、缩略图缓存与并发生成、EXIF、设置合并、Windows 保留名、文件操作、文件夹卡片、查看器翻页与实际缩放比、**指针锚定缩放**、**长按左/右键缩放与拖动取消**、**侧键翻页**、**沉浸边缘命中**、**指针移动恢复光标**、**独立光标计时**、视频控件与码率、自绘标题栏、Onevra 设置页和 ffmpeg 隐藏窗口进程。

真实窗口回归通过 debug 构建的本地端点完成，覆盖 1320×850 与 760×500 布局、深/浅主题、瀑布流滚动与恢复、多标签、主窗口最大化/全屏、查看器沉浸、设置、重命名/新建/回收站、菜单快捷键和窗口控制。

## 已知限制

| 项目 | 处理方式 |
|---|---|
| Finder 标签（xattr） | Windows 没有等价机制，改为应用内“收藏” |
| HDR | 按 SDR 显示，不做色调映射 |
| HEIC / AVIF / RAW | 缩略图：有系统 codec 时走 WIC/Shell；当前机器无 HEIC codec 时显示占位。查看器仍主要依赖 Go 解码，无法预览时交系统关联程序打开 |
| 视频缩略图 | 依赖外部 `ffmpeg.exe`（PATH 或 exe 同级）；缺失时显示渐变占位图 |
| 视频播放 | 依赖本机解码器；部分 MKV/HEVC 只能外部打开 |
| 超大图 | 默认显示最长边 2,560 px 预览；>80 MP 优先走系统缩略图 provider，失败再回退 WIC/Shell，provider 不可用时显示中文提示；1:1 按需解码原图，不设 50 MP 上限，受单张纹理和可用内存限制 |
| 右键八向手势 | 未迁移；右键用于原生上下文菜单 |
| 系统主题实时跟随 | 启动时检测系统深浅色；运行中可在设置里立即切换主题 |
| 远程位置写操作 | 只读：不提供重命名、新建、删除、移动；需要写入时请用系统挂载方式 |
| 远程位置密码 | 「记住密码」用 DPAPI 按当前 Windows 账户加密存到应用数据目录，换账户或换机器后需重新输入 |
| 远程视频 | 经本机回环 HTTP 服务播放（随机端口 + 随机路径令牌，闲置 3 分钟自动关闭）；播放本身仍依赖本机解码器 |

## 项目结构

```text
main.go          应用入口、窗口、单实例、启动路径、自动更新检查
app.go           App/Tab/Viewer 状态、异步加载、历史、选择、设置、缓存与主题
view.go          侧栏、标签栏、工具栏、内容区、状态栏、浏览快捷键
grid.go          自绘虚拟瀑布流、卡片、命中测试、选择、右键菜单、拖放
viewer.go        图片/视频查看器、主窗口内沉浸浮层、指针锚定缩放、长按缩放与侧键翻页
viewer_preview.go 查看器 2560 预览生成、快/慢缩放、缩略图占位、磁盘缓存
viewer_fullres.go 1:1 真原图按需解码、取消 50 MP 门禁与释放
viewer_preload.go 邻图预加载、字节预算、所有权转移
viewer_hover.go  左右边带悬停预取
viewer_hud.go    单槽状态提示与 debug HUD
settings_ui.go   Onevra 设置分类、搜索、控件与保存
update_ui.go     应用内中文更新面板、检查/下载/安装状态
dialogs.go       重命名、新建、删除确认、帮助、关于
menu.go          原生菜单与更新入口
dir.go           目录扫描、排序、导航
remote.go        远程位置模型、虚拟路径、连接池、统一读取接口
remote_smb.go    SMB2 后端（go-smb2，进程内会话）
remote_webdav.go WebDAV 后端（PROPFIND / Range）
remote_creds.go  DPAPI 加密的密码存储
remote_media.go  本机回环媒体服务，远程视频播放与取帧
remote_ui.go     添加/编辑远程位置对话框与侧边栏条目
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
- 缩略图性能报告：[`docs/缩略图性能报告.md`](docs/缩略图性能报告.md)
- 缩略图基线：[`docs/缩略图性能基线.md`](docs/缩略图性能基线.md)
- 查看器性能报告：[`docs/查看器加载性能报告.md`](docs/查看器加载性能报告.md)
- 查看器性能基线：[`docs/查看器加载性能基线.md`](docs/查看器加载性能基线.md)
- 查看器优化计划：[`docs/查看器加载性能优化计划.md`](docs/查看器加载性能优化计划.md)
- 后端 spike：[`docs/缩略图WIC-spike.md`](docs/缩略图WIC-spike.md)
- 测试语料说明：[`docs/缩略图测试语料说明.md`](docs/缩略图测试语料说明.md)
- 移植说明：[`docs/移植说明.md`](docs/移植说明.md)
- 发布流程：[`docs/发布流程.md`](docs/发布流程.md)

## 来源与许可

原项目 [FlowVision](https://github.com/netdcy/FlowVision) 是 macOS Swift/AppKit 应用，GPL-3.0。本仓库是独立重写，沿用 GPL-3.0，未包含上游二进制或 AppKit 代码；第三方组件许可见 [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md)。

UI 设计语言与控件来自 [Onevra](https://github.com/oneo-me/onevra)（MIT），通过 Go module `github.com/oneo-me/onevra/source` 引入。
