# Changelog

全部版本遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/) 的章节格式；更新窗口会读取当前版本对应的章节。

## [1.2.3] - 2026-11-24

### Fixed

- 自绘标题栏后原生菜单栏只能通过 Alt 打开，主界面看不到“检查更新”。标题栏新增常驻“检查更新”按钮，设置页新增“软件更新”区块，关于窗口也提供入口。

### Changed

- 更新说明和入口文案统一为中文“检查更新”，不再依赖隐藏的原生菜单。

## [1.2.2] - 2026-11-24

### Added

- 接入 MyGo 签名自动更新：帮助菜单新增“检查更新…”，后台默认每日检查，发现新版本后可查看发布说明、下载、安装并重启。
- 更新归档和清单使用 Ed25519 签名；应用只接受与 `mygo.json` 中 `updates.publicKey` 匹配的更新。
- 新增 `.github/workflows/release.yml`：推送 `v*` 标签后校验版本、创建草稿 Release、构建 Windows 安装包和更新文件、上传并发布。
- 新增 `CHANGELOG.md` 作为 Release notes 来源。
- 新增 `docs/发布流程.md`，记录密钥、secret、tag 发布与回滚方式。

### Changed

- Windows 发布产物改为 MyGo 构建的按用户安装包；安装到 `%LOCALAPPDATA%\Programs\FlowVision`，可自我更新，无需管理员权限。
- 版本号的唯一权威来源改为 `mygo.json`；`main.go` 中的 `devVersion` 只作为直接 `go build` 的兜底。

## [1.2.1] - 2026-11-24

### Changed

- 移除系统标题栏与系统窗口按钮，改为 48 DIP 的 Onevra 自绘标题栏：品牌、标签、新建、最小化、最大化/还原、关闭。
- 支持拖动、双击最大化、Windows 11 snap layouts 与系统缩放边缘。

## [1.2.0] - 2026-11-24

### Changed

- 接入 Onevra 设计系统：暖色主题、Sidebar、IconButton、TextBox、Switch、Button、Badge、Lucide 图标。
