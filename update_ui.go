package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// UpdatePhase is the state of the in-app updater panel.
type UpdatePhase int

const (
	UpdateIdle UpdatePhase = iota
	UpdateChecking
	UpdateAvailable
	UpdateDownloading
	UpdateReady
	UpdateCurrent
	UpdateError
	UpdateDisabled
)

func (p UpdatePhase) String() string {
	switch p {
	case UpdateChecking:
		return "checking"
	case UpdateAvailable:
		return "available"
	case UpdateDownloading:
		return "downloading"
	case UpdateReady:
		return "ready"
	case UpdateCurrent:
		return "current"
	case UpdateError:
		return "error"
	case UpdateDisabled:
		return "disabled"
	default:
		return "idle"
	}
}

// checkForUpdates opens the custom updater panel and starts a signed MyGo
// updater check. It is the single entry point for the title bar, menu and
// settings page.
func (a *App) checkForUpdates() {
	a.beginUpdateCheck(true)
}

func (a *App) beginUpdateCheck(show bool) {
	a.cancelUpdate()
	a.updateGen++
	gen := a.updateGen
	a.updatePhase = UpdateChecking
	a.updateVersion = ""
	a.updateNotes = ""
	a.updateDate = time.Time{}
	a.updateErr = ""
	a.updateProgress = 0
	a.updateDownloaded = 0
	a.updateTotal = 0
	if show {
		a.showAbout = false
		a.showHelp = false
		a.updateOpen = true
		a.refresh()
	}
	if !mygo.Updater.Enabled() {
		if show {
			a.updatePhase = UpdateDisabled
			a.updateErr = "此构建没有可用的更新源，或安装目录不可写。"
			a.refresh()
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	a.updateCancel = cancel
	go func() {
		defer cancel()
		up, err := mygo.Updater.Check(ctx)
		a.update(func() {
			if a.updateGen != gen {
				return
			}
			a.updateCancel = nil
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				logf("update check: %v", err)
				if show {
					a.updatePhase = UpdateError
					a.updateErr = updateCheckErrorText(err)
					a.updateOpen = true
					a.refresh()
				}
				return
			}
			if up == nil {
				logf("update check: up to date (%s)", appVersion())
				if show {
					a.updatePhase = UpdateCurrent
					a.updateOpen = true
					a.refresh()
				}
				return
			}
			logf("update check: %s -> %s", appVersion(), up.Version)
			a.updateAvailable = up
			a.updateVersion = up.Version
			a.updateNotes = up.Notes
			a.updateDate = up.Date
			a.updatePhase = UpdateAvailable
			a.updateOpen = true
			a.refresh()
		})
	}()
}

// startUpdateAutoCheck keeps the old plugin's background behavior: check once
// after launch, and open the custom panel only when a newer version exists.
func (a *App) startUpdateAutoCheck() {
	if a.updateAutoStarted {
		return
	}
	a.updateAutoStarted = true
	if !mygo.Updater.Enabled() {
		return
	}
	go func() {
		time.Sleep(2 * time.Second)
		a.update(func() {
			if a.win == nil {
				return
			}
			a.beginUpdateCheck(false)
		})
	}()
}

func (a *App) installUpdate() {
	if a.updateAvailable == nil || a.updatePhase == UpdateDownloading || a.updatePhase == UpdateReady {
		return
	}
	a.updateGen++
	gen := a.updateGen
	up := a.updateAvailable
	a.updatePhase = UpdateDownloading
	a.updateProgress = -1
	a.updateDownloaded = 0
	a.updateTotal = 0
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	a.updateCancel = cancel
	a.refresh()
	go func() {
		defer cancel()
		var last time.Time
		lastPct := -1
		err := up.Install(ctx, func(downloaded, total int64) {
			pct := -1
			if total > 0 {
				pct = int(float64(downloaded) / float64(total) * 100)
			}
			now := time.Now()
			if pct == lastPct && now.Sub(last) < 75*time.Millisecond {
				return
			}
			last, lastPct = now, pct
			a.update(func() {
				if a.updateGen != gen {
					return
				}
				a.updateDownloaded = downloaded
				a.updateTotal = total
				if total > 0 {
					a.updateProgress = float64(downloaded) / float64(total)
				} else {
					a.updateProgress = -1
				}
				a.refresh()
			})
		})
		a.update(func() {
			if a.updateGen != gen {
				return
			}
			a.updateCancel = nil
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				logf("update install: %v", err)
				a.updatePhase = UpdateError
				a.updateErr = updateInstallErrorText(err)
				a.updateOpen = true
				a.refresh()
				return
			}
			a.updatePhase = UpdateReady
			a.updateProgress = 1
			a.updateOpen = true
			a.refresh()
		})
	}()
}

// cancelUpdate invalidates late callbacks and stops an in-flight download.
func (a *App) cancelUpdate() {
	a.updateGen++
	if a.updateCancel != nil {
		a.updateCancel()
		a.updateCancel = nil
	}
	a.updateAvailable = nil
	a.updatePhase = UpdateIdle
	a.updateProgress = 0
	a.updateDownloaded = 0
	a.updateTotal = 0
}

func updateCheckErrorText(err error) string {
	switch {
	case errors.Is(err, mygo.ErrUpdatesDisabled):
		return "此构建不支持自我更新。请使用从 GitHub Release 安装的版本。"
	case errors.Is(err, context.DeadlineExceeded):
		return "检查更新超时，请检查网络或系统代理后重试。"
	default:
		return "无法连接更新服务，请检查网络或系统代理后重试。"
	}
}

func updateInstallErrorText(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "下载更新超时，当前版本未受影响。请稍后重试。"
	case errors.Is(err, mygo.ErrUpdatesDisabled):
		return "当前安装位置不可写，无法覆盖应用文件。"
	default:
		return "更新下载或安装失败，当前版本未受影响。请稍后重试。"
	}
}

// debugUpdateUI drives the panel states for screenshots and layout checks.
func (a *App) debugUpdateUI(state string) {
	a.cancelUpdate()
	a.updateOpen = true
	a.updatePhase = UpdateAvailable
	a.updateVersion = "1.2.9"
	a.updateNotes = "### 新增\n\n- 图片查看器新增一键居中，滚轮缩放以鼠标位置为锚点。\n- 更新窗口改为应用内中文界面。\n\n### 修复\n\n- 整理 README 与仓库简介，移除过时的 WebView2 描述。"
	a.updateDate = time.Now()
	a.updateAvailable = &mygo.Update{Version: a.updateVersion, Notes: a.updateNotes, Date: a.updateDate}
	switch state {
	case "closed":
		a.updateOpen = false
		a.updatePhase = UpdateIdle
		a.updateVersion = ""
		a.updateAvailable = nil
	case "current":
		a.updatePhase = UpdateCurrent
		a.updateVersion = ""
		a.updateDate = time.Time{}
		a.updateAvailable = nil
	case "error":
		a.updatePhase = UpdateError
		a.updateErr = "无法连接更新服务，请检查网络或系统代理后重试。"
		a.updateDate = time.Time{}
		a.updateAvailable = nil
		a.updateVersion = ""
	case "disabled":
		a.updatePhase = UpdateDisabled
		a.updateDate = time.Time{}
		a.updateAvailable = nil
		a.updateVersion = ""
	case "checking":
		a.updatePhase = UpdateChecking
		a.updateVersion = ""
		a.updateDate = time.Time{}
		a.updateAvailable = nil
	case "downloading":
		a.updatePhase = UpdateDownloading
		a.updateProgress = .42
		a.updateDownloaded = 3_485_184
		a.updateTotal = 8_298_240
	case "ready":
		a.updatePhase = UpdateReady
		a.updateProgress = 1
	}
	a.refresh()
}

func updateSize(n int64) string {
	if n <= 0 {
		return "—"
	}
	return (MediaSvc{}).FormatSize(n)
}

func (a *App) updateModal(c *ui.Context) {
	if !a.updateOpen {
		return
	}
	wasOpen := a.updateOpen
	th := c.Theme()
	ui.Modal(c, &a.updateOpen, func() {
		ui.Column(c).Width(560).Gap(12).Children(func() {
			dialogHeader(c, lucide.CloudDownload(), "软件更新", "MyGo 更新服务 · 签名校验 · 增量下载")
			a.updateVersionRow(c, th)
			a.updateBody(c, th)
			a.updateActions(c)
		})
	})
	if wasOpen && !a.updateOpen {
		a.cancelUpdate()
	}
}

func (a *App) updateVersionRow(c *ui.Context, th *ui.Theme) {
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		components.Text(c, "当前版本").FontSize(10.5).TextColor(th.Text.Alpha(.55))
		components.Badge(c, "v"+appVersion())
		if a.updateVersion != "" {
			components.Text(c, "→").FontSize(12).TextColor(th.TextMuted)
			components.Badge(c, "v"+a.updateVersion, components.BadgeOptions{Tone: components.BadgeAccent})
		}
		ui.Spacer(c)
		if !a.updateDate.IsZero() {
			components.Text(c, a.updateDate.Local().Format("2006-01-02")).
				FontSize(10).TextColor(th.Text.Alpha(.45))
		}
	})
}

func (a *App) updateBody(c *ui.Context, th *ui.Theme) {
	switch a.updatePhase {
	case UpdateChecking:
		ui.Row(c).FillWidth().Padding(6, 8).Gap(10).AlignItems(ui.Center).Children(func() {
			components.Spinner(c, components.SpinnerSpokes).Size(20, 20).Label("正在检查更新")
			ui.Column(c).Gap(2).Children(func() {
				components.Text(c, "正在检查新版本…").FontSize(12).Bold()
				components.Text(c, "向 GitHub Release 请求签名清单，并校验当前平台的最新版本。").
					FontSize(10.5).TextColor(th.TextMuted)
			})
		})
	case UpdateAvailable:
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			components.Icon(c, lucide.Sparkles()).Size(18, 18).TextColor(th.Accent)
			ui.Column(c).Gap(2).Children(func() {
				components.Text(c, "发现新版本 v"+a.updateVersion).FontSize(13).Bold()
				components.Text(c, "更新包已签名；安装失败时当前版本不会被改动。").
					FontSize(10.5).TextColor(th.TextMuted)
			})
		})
		updateNotesCard(c, th, a.updateNotes)
	case UpdateDownloading:
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			components.Text(c, "正在下载更新…").FontSize(12).Bold()
			components.Progress(c, a.updateProgress).Label("更新下载进度")
			ui.Row(c).FillWidth().Children(func() {
				if a.updateTotal > 0 {
					components.Text(c, fmt.Sprintf("%d%% · %s / %s",
						int(a.updateProgress*100), updateSize(a.updateDownloaded), updateSize(a.updateTotal))).
						FontSize(10.5).TextColor(th.TextMuted)
				} else {
					components.Text(c, "正在准备增量补丁…").FontSize(10.5).TextColor(th.TextMuted)
				}
				ui.Spacer(c)
				components.Text(c, "可取消，当前程序不会被提前替换").FontSize(10).TextColor(th.Text.Alpha(.45))
			})
		})
	case UpdateReady:
		ui.Row(c).FillWidth().Padding(6, 8).Gap(10).AlignItems(ui.Center).Children(func() {
			components.Icon(c, lucide.CircleCheck()).Size(24, 24).TextColor(th.Success)
			ui.Column(c).Gap(2).Children(func() {
				components.Text(c, "更新已安装").FontSize(13).Bold()
				components.Text(c, "重启后会运行 v"+a.updateVersion+"；当前窗口仍运行旧版本。").
					FontSize(10.5).TextColor(th.TextMuted)
			})
		})
	case UpdateCurrent:
		ui.Row(c).FillWidth().Padding(6, 8).Gap(10).AlignItems(ui.Center).Children(func() {
			components.Icon(c, lucide.CircleCheck()).Size(20, 20).TextColor(th.Success)
			ui.Column(c).Gap(2).Children(func() {
				components.Text(c, "已是最新版本").FontSize(12).Bold()
				components.Text(c, "FlowVision v"+appVersion()+" 已与 GitHub Release 保持一致。").
					FontSize(10.5).TextColor(th.TextMuted)
			})
		})
	case UpdateError:
		ui.Row(c).FillWidth().Padding(6, 8).Gap(10).AlignItems(ui.Center).Children(func() {
			components.Icon(c, lucide.CircleAlert()).Size(20, 20).TextColor(th.Danger)
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				components.Text(c, "更新未完成").FontSize(12).Bold()
				components.Text(c, a.updateErr).FontSize(10.5).TextColor(th.TextMuted)
			})
		})
	case UpdateDisabled:
		ui.Row(c).FillWidth().Padding(6, 8).Gap(10).AlignItems(ui.Center).Children(func() {
			components.Icon(c, lucide.CircleAlert()).Size(20, 20).TextColor(th.Warning)
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				components.Text(c, "此构建不能自我更新").FontSize(12).Bold()
				components.Text(c, "开发构建、直接 go build 的产物或只读安装目录不会写入更新公钥。请使用 GitHub Release 的安装包。").
					FontSize(10.5).TextColor(th.TextMuted)
			})
		})
	case UpdateIdle:
		components.Text(c, "准备检查更新。").FontSize(10.5).TextColor(th.TextMuted)
	}
}

func updateNotesCard(c *ui.Context, th *ui.Theme, notes string) {
	ui.Box(c).FillWidth().Padding(12, 10).Radius(10).
		BorderWidth(1).BorderColor(th.Border.Alpha(.7)).Background(th.Surface).Children(func() {
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			components.Text(c, "发布说明").FontSize(10.5).Bold().TextColor(th.TextMuted)
			if notes == "" {
				components.Text(c, "此版本未提供发布说明。").FontSize(10.5).TextColor(th.TextMuted)
				return
			}
			ui.Scroll(c).Height(164).Scrollbars(true).Children(func() {
				markdown.View(c, notes, nil).FillWidth()
			})
		})
	})
}

func (a *App) updateActions(c *ui.Context) {
	ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Children(func() {
		switch a.updatePhase {
		case UpdateChecking:
			components.Button(c, "取消", components.ButtonSecondary).
				OnClick(func() { a.updateOpen = false })
		case UpdateAvailable:
			components.Button(c, "稍后", components.ButtonSecondary).
				OnClick(func() { a.updateOpen = false })
			components.Button(c, "下载并安装", components.ButtonPrimary).
				OnClick(func() { a.later(a.installUpdate) })
		case UpdateDownloading:
			components.Button(c, "取消下载", components.ButtonSecondary).
				OnClick(func() { a.updateOpen = false })
		case UpdateReady:
			components.Button(c, "稍后重启", components.ButtonSecondary).
				OnClick(func() { a.updateOpen = false })
			components.Button(c, "立即重启", components.ButtonPrimary).
				OnClick(func() { a.later(func() { mygo.App.Relaunch() }) })
		case UpdateError:
			components.Button(c, "关闭", components.ButtonSecondary).
				OnClick(func() { a.updateOpen = false })
			components.Button(c, "重试", components.ButtonPrimary).
				OnClick(func() { a.later(func() { a.beginUpdateCheck(true) }) })
		default:
			components.Button(c, "关闭", components.ButtonSecondary).
				OnClick(func() { a.updateOpen = false })
		}
	})
}
