# FlowVision for Windows

用 **Go + MyGo** 重写的 [FlowVision](https://github.com/netdcy/FlowVision)：一个以瀑布流展示本地图片/视频的桌面浏览器。
原项目是 macOS 原生的 Swift/AppKit 应用；本仓库把它移植到 Windows，界面由 WebView2 渲染，文件系统、缩略图、元数据与协议服务全部在 Go 侧完成，**不需要 cgo，也不需要安装 Swift 或 Xcode**。

- 上游项目：https://github.com/netdcy/FlowVision （GPL-3.0）
- 本移植版：GPL-3.0，见 `LICENSE`
- 技术栈：Go 1.27+ · [MyGo](https://github.com/egoist/mygo) v0.10.0 · WebView2（Windows 10/11 自带）

---

## 一、评估结论：能不能直接改造？必须重写吗？

**必须重写，但不需要从零发明。** 原项目是纯 macOS 技术栈，Windows 上没有可编译、可运行的基础：

| 维度 | 实测数据（原仓库 `FlowVision/Sources`） |
|---|---|
| 代码规模 | 54 个 Swift 文件，33,088 行 |
| 工程形态 | Xcode 工程 + `Main.storyboard`(113 KB) + 6 个 XIB + `.xcstrings` 本地化 |
| AppKit 依赖 | `NSView`/`NSWindow`/`NSControl` 等出现 **1,427** 次 |
| 绘图 | CoreGraphics **366** 次（缩略图、圆角、占位、动画都走它） |
| 文件与元数据 | `FileManager` 92、ImageIO 49、xattr/Finder 标签 22 |
| 媒体 | AVFoundation 21、CoreImage 11 |
| 事件 | FSEvents / `DispatchSource` 18 |
| Metal / vImage / QuickLook | **0 / 0 / 0**（没有自绘 GPU 管线，渲染全靠 AppKit） |
| 第三方 | SDWebImage + WebP 解码、ffmpeg-kit（已停止分发）、BTree、Settings |

结论拆开看：

1. **UI 层必须重写。** `NSCollectionView` + 自定义 `NSCollectionViewLayout` + `NSViewController` 这套东西在 Windows 上没有对应物；把 1,427 处 AppKit 调用逐个翻译成 WinUI 的成本高于重画一套 Web UI。
2. **平台能力必须替换。** HDR（`preferredImageDynamicRange`）、Finder 标签（xattr）、`NSEvent` 全局事件监听、QuickLook 都没有 Windows 等价 API。
3. **业务逻辑可以迁移。** 瀑布流算法、排序规则、缩略图缓存策略、EXIF 读取、右键八向手势、快捷键表都是纯逻辑，已经按原实现逐段对照迁移（见 `docs/移植说明.md`）。
4. **渲染管线没有 GPU 包袱。** 原项目 Metal 计数为 0，说明它本来就把图片缩放/合成交给 CoreGraphics；换成 Go 的 `image` + `x/image/draw` 后视觉结果可以对齐，代价是超大图缩略图会慢一些，因此加了磁盘缓存与信号量限流。

因此方案是：**Go 后端（扫描/缩略图/元数据/自定义协议/视频流）+ HTML/CSS/JS 前端（WebView2）**，而不是逐行翻译 Swift。

---

## 二、已移植的功能

| 功能 | 状态 | 说明 |
|---|---|---|
| 瀑布流网格 | ✅ | 固定行高、宽度 = 行高 × 宽高比、逐行左对齐换行；宽高比 clamp 到 0.34–3.2，与原 `CustomFlowLayout` 一致 |
| 文件夹浏览 / 递归 / 排序 / 隐藏文件 | ✅ | 按名称（自然排序）、日期、大小、类型；递归最多 5 层、前端单次请求最多 6,000 项（后端硬上限 20,000） |
| 多标签页 | ✅ | 每个标签独立路径、排序、历史、滚动位置，重启恢复 |
| 前进/后退/上一级/同级文件夹跳转 | ✅ | 同级跳转只在“含图片/视频”的文件夹之间移动，对应原项目的行为 |
| 缩略图 | ✅ | JPEG 磁盘缓存（96/160/240/320/480/640/800/960 分桶）、缓存名带源文件 mtime 与大小、CatmullRom 缩放、EXIF 方向校正、透明底合成 |
| 缩略图缓存治理 | ✅ | 容量上限可设，超限按最旧访问淘汰；设置里可查看占用并一键清空 |
| 图片查看器 | ✅ | 滚轮以指针为中心缩放（0.02×–40×）、拖动平移、旋转、适应窗口、1:1、上一张/下一张、信息面板 |
| 视频查看器 | ✅ | 本机回环 HTTP 流（带随机 token，支持 Range 拖动进度）；容器不被 WebView2 支持时提供“用系统播放器打开” |
| 右键八向手势 | ✅ | 右=下一文件夹、左=上一文件夹、上=父级、下=返回、右上=同级下一个、右下=关闭标签页；未过阈值弹上下文菜单 |
| 原生菜单 / 快捷键 | ✅ | 文件、视图、转到、帮助四组菜单；菜单项通过 `window.fv.menu()` 回调页面 |
| 文件操作 | ✅ | 回收站删除（`SHFileOperationW`）、永久删除、重命名、新建文件夹、资源管理器定位、系统默认程序打开 |
| 设置 | ✅ | 主题（深/浅/跟随系统）、缩略图大小、间距、排序、递归、隐藏文件、其他文件、视频自动播放/循环、缓存上限、侧边栏宽度 |
| 收藏与位置 | ✅ | 收藏夹、桌面/图片/下载/文档/视频/各磁盘盘符；侧边栏文件夹树惰性展开 |
| 搜索筛选 | ✅ | 当前文件夹内按文件名即时过滤 |

## 三、降级与未实现（windows 上确实做不到或成本过高）

| 项目 | 处理方式 |
|---|---|
| Finder 标签（xattr） | Windows 无等价机制；改为应用内“收藏”，不伪造系统标签 |
| HDR 动态范围 | WebView2 无 `preferredImageDynamicRange` 等价能力；按 SDR 显示，不做色调映射 |
| QuickLook | 不存在；查看器自身承担预览 |
| HEIC / AVIF / RAW | Go 标准解码器不支持；查看器会显示“无法预览”，并提供系统关联程序打开 |
| 视频缩略图 | 依赖外部 `ffmpeg.exe`（PATH 或 exe 同级目录）。没有 ffmpeg 时生成渐变+播放三角占位图，不会报错 |
| 视频播放 | 依赖 WebView2/系统编解码器；MKV/部分 HEVC 可能只能外部打开 |
| 超大图 | 查看器对超过 3,000 万像素的图改用 2,560px 预览图，避免 WebView2 解码卡顿 |
| 自动滚动播放、iCloud 相册等 macOS 集成 | 未移植 |

## 四、运行与构建

**直接运行**：双击 `FlowVision.exe`。也可以把文件夹路径作为参数：

```powershell
.\FlowVision.exe "D:\Photos"
```

首次启动打开“图片”文件夹；之后恢复上次的标签页。设置与日志位于 `%APPDATA%\FlowVision`，缩略图缓存位于 `%LOCALAPPDATA%\FlowVision\thumbs`。

**从源码构建**（需要 Go 1.27+，MyGo 要求 go 1.27；国内网络建议设代理）：

```powershell
$env:GOPROXY = "https://goproxy.cn,direct"
go build -ldflags "-H=windowsgui -s -w" -o FlowVision.exe .
```

开发期想看控制台输出就去掉 `-H=windowsgui`。前端已用 `go:embed` 打进二进制，**分发时只需 `FlowVision.exe` + `resources\icon.png`**（图标可选，缺省用系统图标）。

可选：安装 [WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/)（Windows 11 已内置；Windows 10 一般随 Edge 安装）。

## 五、快捷键

浏览：`W` 上一级 · `S` 返回 · `A` 上一文件夹 · `D` 下一文件夹 · `Ctrl+O` 打开文件夹 · `F5` 刷新 · `Ctrl+F` 筛选 · `Ctrl+T` 新标签 · `Ctrl+W` 关标签 · `Ctrl+B` 侧边栏 · `Ctrl+R` 递归 · `Ctrl+,` 设置 · `Ctrl+-` / `Ctrl+=` 缩略图缩放

查看：双击缩略图打开 · `Space`/`→`/`↓` 下一张 · `←`/`↑` 上一张 · `Esc` 关闭 · `I` 信息 · `F` 适应 · `1` 100% · `R` 旋转 · 滚轮缩放 · 拖动平移 · 双击切换适应/100%

## 六、测试与验证

- **Go 单元测试：23 个，`go test .` 全部通过。** 覆盖目录扫描、自然排序（大数字/前导零）、递归与隐藏文件、同级文件夹跳转、Store 快照与大小写重命名、缩略图/视频占位、Range 请求、EXIF 方向与 SRational、Settings 合并、Windows 保留名与 255 UTF-16 名称限制、拖入路径前导空格、空路径浏览等。
  运行：`go test -v .`
- **Windows 端到端检查（debug 构建 + 真实 WebView2）：** 首启与 CLI 带空格路径、单实例转发、标签恢复/关闭/溢出滚动、缩略图与瀑布流尺寸、树形目录展开与新建目录后的刷新、查看器缩放/旋转/翻页、排序/筛选时查看器跟随、搜索无结果关闭查看器、跨标签共享筛选、空白处取消选择、右键八向手势（含右下关闭标签）、重命名扩展名保护、回收站删除、新建文件夹、滚动位置恢复，以及原生菜单快捷键（Ctrl+T/W/B/R/+/−、Ctrl+Left/Right 等）均实测通过。
- **Release 构建：** `go build -ldflags "-H=windowsgui -s -w"` 通过；debug 专用测试端点不包含在 release 中。

## 七、目录

```
main.go        应用入口、窗口、服务绑定、启动路径
dir.go         目录扫描、排序、导航（Tree/Sibling/Parent/Drives/Places）
thumb.go       缩略图生成、磁盘缓存、ffmpeg 取帧、占位图、缓存淘汰
meta.go        尺寸探测、EXIF 解析、MediaSvc
media.go       img:// 协议（缩略图/原图/预览）+ 回环视频流
settings.go    设置模型与 Prefs 服务
actions.go     AppSvc：启动信息、对话框、文件操作、退出
win32.go       隐藏属性判断、回收站删除
menu.go        原生菜单（菜单动作异步派发到页面，避免与 WebView 事件线程互锁）
debug.go       仅 debug 构建启用的本地测试控制端点
web/           index.html / style.css / app.js
resources/     icon.png
docs/          移植说明（模块对照与原实现细节）
```
