# FlowVision for Windows

用 **Go + MyGo** 重写的 [FlowVision](https://github.com/netdcy/FlowVision)：一个以瀑布流展示本地图片/视频的 Windows 桌面浏览器。界面是 **MyGo 的纯 Go 原生 `ui` 控件与自绘网格**，不依赖 WebView2、HTML 或 JavaScript；文件扫描、缩略图、元数据、查看器与文件操作全部在 Go 侧完成，不需要 cgo、Swift 或 Xcode。

- 上游项目：https://github.com/netdcy/FlowVision （GPL-3.0）
- 本移植版：GPL-3.0，见 `LICENSE`
- 技术栈：Go 1.27+ · [MyGo](https://github.com/egoist/mygo) v0.10.0 · `github.com/egoist/mygo/ui`
- 当前版本：**1.1.0（纯 Go 原生 UI）**
- 历史版本：1.0.0 是 Go + WebView2 的第一阶段移植，仍保留在 Git 历史与 GitHub Release 中

---

## 一、为什么改造成纯 Go 原生 UI

1.0.0 用 WebView2 渲染 HTML/JS。它能快速复用上游的瀑布流交互，但带来三个问题：Windows 10 需要 WebView2 Runtime、页面与 Go 之间要靠 `Bind`/`Eval` 通信、以及 WebView 事件线程里同步调用 `Eval` 容易自锁。

1.1.0 把窗口内容换成 `ui.View`，由 MyGo 的原生控件承担侧栏、标签栏、工具栏、对话框、查看器；照片网格用 `ui.Scroll` + `ui.Box.Draw` 自绘虚拟瀑布流。这样只为可见区域画图，不为几千张图创建控件，同时彻底移除 WebView2、HTML 与 JS。

构建 tag `mygo_nowebview` 让 MyGo 进入 native-only 模式。**纯 Go 构建必须带这个 tag**，否则 MyGo 会按默认 WebView 模式编译。

## 二、已实现的功能

| 功能 | 状态 | 说明 |
|---|---|---|
| 瀑布流网格 | ✅ | 自绘虚拟化：按窗口宽度、缩略图目标宽度与间距算列数，按宽高比分配最短列；只对可见区域请求缩略图 |
| 文件夹浏览 / 递归 / 排序 / 隐藏文件 | ✅ | 名称（自然排序）、日期、大小、类型；递归最多 5 层、单次最多 6,000 项 |
| 多标签页 | ✅ | 每标签独立路径、排序、历史、搜索、选中状态与滚动位置；重启恢复 |
| 导航 | ✅ | 返回、上一级、同级上/下一个（只在含媒体文件夹间跳转） |
| 缩略图 | ✅ | JPEG 磁盘缓存、EXIF 方向校正、透明底合成、CatmullRom 缩放；同一张图并发生成时按缓存路径串行，避免写文件竞争 |
| 图片查看器 | ✅ | 适应/100%/缩放/旋转/平移、上一张/下一张、信息面板、文件操作 |
| 视频查看器 | ✅ | MyGo `ui.Video` + 原生解码；无法解码时提示并可用系统播放器打开 |
| 原生菜单 | ✅ | 文件/编辑/视图/转到/帮助；动作直接调用 App，经 `w.Update` 回主线程 |
| 文件操作 | ✅ | 回收站删除（`SHFileOperationW`）、永久删除、重命名（扩展名保护）、新建文件夹、资源管理器定位、默认程序打开、复制路径 |
| 设置 | ✅ | 主题、缩略图宽度、网格间距、侧栏/信息面板、隐藏/其他文件、递归、循环视频、缓存查看与清理 |
| 收藏 / 位置 / 盘符 / 文件夹树 | ✅ | 收藏夹、桌面/图片/下载/文档/视频/盘符；文件夹树惰性加载，新建目录后刷新 |
| 搜索 | ✅ | 当前文件夹内按文件名即时过滤；`Ctrl+F` 聚焦搜索框 |
| 拖入文件 | ✅ | 窗口接收文件/文件夹路径，打开所在目录 |

## 三、降级与未实现

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

## 四、运行与构建

**直接运行**：双击 `FlowVision.exe`。也可以把文件夹或文件路径作为参数：

```powershell
.\FlowVision.exe "D:\Photos"
```

单实例：第二次启动会把路径交给已运行的窗口并新增标签，不会开第二个进程。

**从源码构建**（需要 Go 1.27+；国内网络建议设代理）：

```powershell
$env:GOPROXY = "https://goproxy.cn,direct"
go build -trimpath -tags mygo_nowebview -ldflags "-H=windowsgui -s -w" -o FlowVision.exe .
```

分发时只需 `FlowVision.exe` + `resources\icon.png`（图标可选）。**不需要安装 WebView2 Runtime**。

调试构建（带本地测试端点，release 不含）：

```powershell
go build -tags "debug mygo_nowebview" -o fvdebug.exe .
```

数据目录：

- 设置、日志：`%APPDATA%\FlowVision`
- 缩略图缓存：`%LOCALAPPDATA%\FlowVision\thumbs`

## 五、快捷键

浏览（焦点不在输入框时）：`W` 上一级 · `S` 返回 · `A` 上一文件夹 · `D` 下一文件夹 · `F5` 刷新 · `Ctrl+F` 搜索 · `Ctrl+O` 打开文件夹 · `Ctrl+T` 新建标签 · `Ctrl+W` 关闭标签 · `Ctrl+B` 侧栏 · `Ctrl+R` 递归 · `Ctrl+=` / `Ctrl+-` 缩略图缩放 · `Ctrl+,` 设置 · `F1` 快捷键帮助

网格：方向键移动选中 · `Home`/`End` 首尾 · `Enter`/`Space` 打开查看器 · `Delete` 移入回收站 · `F2` 重命名 · `Ctrl+A` 全选 · `Esc` 取消选择 / 关闭弹窗

查看器：`←`/`→` 上一张/下一张 · `+`/`-` 缩放 · `0` 适应 · `1` 100% · `R` 顺时针旋转 · `Shift+R` 逆时针 · `I` 信息面板 · 滚轮缩放 · 拖动平移 · `Esc` 关闭

## 六、测试与验证

- **Go 测试 34 个全部通过**：`go test -tags mygo_nowebview .`。覆盖目录扫描/自然排序/递归/隐藏、同级导航、缩略图缓存与并发生成、EXIF、Settings 合并、Windows 保留名、文件操作，以及原生布局、搜索过滤、网格命中/键盘导航、右键菜单、设置弹窗、查看器浮层等 11 个 UI 测试。
- **真实窗口回归**：debug 构建的本地控制端点 + `CapturePage` 截图 + `SendKeys`，验证了 1320×850 与 760×500 布局、深/浅主题、空目录、11 项瀑布流、160 项滚动与滚动恢复、多标签溢出、查看器缩放/旋转/翻页/信息、设置滚动、单实例路径转发、同级/返回导航、搜索过滤、重命名/新建/回收站删除、菜单快捷键（`Ctrl+T/W/B/R/=/−`）与浏览键（`W/S/A/D`、`F5`）。
- **Release**：`go build -tags mygo_nowebview` 通过；release 二进制不包含 debug 端点。

## 七、目录

```
main.go        应用入口、窗口、单实例、启动路径
app.go         App/Tab/Viewer 状态、异步加载、历史、选择、设置、缓存
view.go        侧栏、标签栏、工具栏、内容区、状态栏、浏览快捷键
grid.go        自绘虚拟瀑布流、命中测试、选择、右键菜单、拖放
viewer.go      图片/视频查看器与信息面板
dialogs.go     设置、重命名、新建、删除确认、帮助、关于
menu.go        原生菜单
dir.go         目录扫描、排序、导航
thumb.go       缩略图生成、磁盘缓存、ffmpeg 取帧、缓存淘汰
meta.go        尺寸、EXIF、MediaSvc
actions.go     文件对话框、文件操作、系统集成
win32.go       隐藏属性、SHFileOperationW 回收站
settings.go    设置模型与持久化
debug.go       仅 debug build 启用的本地控制端点
native_test.go 原生 UI/布局测试
resources/     icon.png
docs/          移植说明
```

## 八、来源与许可

原项目 [FlowVision](https://github.com/netdcy/FlowVision) 是 macOS Swift/AppKit 应用，GPL-3.0。本仓库是独立重写，沿用 GPL-3.0；未包含上游二进制或 AppKit 代码。
