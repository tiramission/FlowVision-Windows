package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// Remote shares are read-only and live entirely in this process: the dialog
// below never mounts, maps or registers anything with Windows.

const statusNoticeLinger = 2600 * time.Millisecond

// tabReadOnly reports whether the active tab shows a location where write
// actions make no sense: a remote share, or the favorites folder, whose
// contents are references rather than a place on disk.
func (a *App) tabReadOnly() bool {
	t := a.tab()
	if t == nil {
		return false
	}
	return isRemotePath(t.Path) || isFavoritesPath(t.Path)
}

func (a *App) showStatusNotice(text string) {
	a.statusNotice = text
	a.statusNoticeAt = time.Now()
	a.refresh()
	if a.win == nil {
		return
	}
	token := a.statusNoticeAt
	time.AfterFunc(statusNoticeLinger, func() {
		a.update(func() {
			if a.statusNoticeAt.Equal(token) {
				a.statusNotice = ""
				a.refresh()
			}
		})
	})
}

func (a *App) statusNoticeText() string {
	if a.statusNotice == "" || time.Since(a.statusNoticeAt) > statusNoticeLinger {
		return ""
	}
	return a.statusNotice
}

func remoteProtocolIcon(protocol string) *ui.SVG {
	if protocol == remoteWebDAV {
		return lucide.Cloud()
	}
	return lucide.Network()
}

// openRemoteDialog starts a new connection.
func (a *App) openRemoteDialog() {
	a.remoteEditID = ""
	a.remoteDraft = remoteShare{Protocol: remoteSMB, Port: 445}
	a.remotePass = ""
	a.remoteErr = ""
	a.remoteBusy = false
	a.remoteEditing = false
	a.remoteDialogOpen = true
	a.syncRemotePortDraft()
	a.refresh()
}

// editRemoteShare reopens a saved connection.
func (a *App) editRemoteShare(share remoteShare) {
	a.remoteEditID = remoteShareID(share)
	a.remoteDraft = share
	a.remotePass = ""
	a.remoteErr = ""
	a.remoteBusy = false
	a.remoteEditing = true
	a.remoteDialogOpen = true
	a.syncRemotePortDraft()
	a.refresh()
}

// commitRemoteShare connects and saves. Dialing runs off the main thread: a
// wrong host name waits for a network timeout, and the dialog must keep
// drawing.
func (a *App) commitRemoteShare() {
	if a.remoteBusy {
		return
	}
	draft := a.remoteDraft
	draft.Protocol = strings.TrimSpace(strings.ToLower(draft.Protocol))
	draft.Host = strings.TrimSpace(draft.Host)
	draft.Share = strings.TrimSpace(strings.Trim(draft.Share, `/`))
	draft.Base = strings.Trim(strings.TrimSpace(draft.Base), "/")
	draft.User = strings.TrimSpace(draft.User)
	draft.Name = strings.TrimSpace(draft.Name)
	// The port field is the authority, not a copy kept in sync when a widget
	// notices a change: typing 5006 and clicking 连接 in the same frame used to
	// send the old default, which produced URLs like https://nas:80.
	draft.Port = int(a.remotePortDraft)
	if err := validateRemoteShare(draft); err != nil {
		a.remoteErr = err.Error()
		a.refresh()
		return
	}
	draft.ID = remoteShareID(draft)
	password := a.remotePass
	if draft.SavePass && password == "" {
		// Keep the remembered password when the field is left blank. On an
		// edit the address may have changed, which changes the id, so look the
		// password up under the entry being edited first: otherwise a path
		// edit would silently lose the saved password.
		if a.remoteEditID != "" {
			password = passwordForID(a.remoteEditID, draft.SavePass)
		}
		if password == "" {
			password = passwordForShare(draft)
		}
	}
	a.remoteBusy = true
	a.remoteErr = ""
	a.refresh()

	go func() {
		root := remoteRoot(draft)
		_, _, err := func() (remoteBackend, string, error) {
			b, err := remotePoolInstance.connect(context.Background(), draft, password)
			if err != nil {
				return nil, "", err
			}
			if _, err := b.list(context.Background(), ""); err != nil {
				return nil, "", err
			}
			return b, "", nil
		}()
		a.update(func() {
			a.remoteBusy = false
			if err != nil {
				a.remoteErr = err.Error()
				a.refresh()
				return
			}
			rememberRemotePassword(draft, password, draft.SavePass)
			if old := a.remoteEditID; old != "" && old != draft.ID {
				// The password moved to the new id; the old one is dead.
				forgetRemotePasswordByID(old)
			}
			a.upsertRemoteShare(draft)
			a.remoteDialogOpen = false
			a.remoteEditID = ""
			a.remotePass = ""
			a.goTo(root, true)
			a.refresh()
		})
	}()
}

func validateRemoteShare(share remoteShare) error {
	if share.Host == "" {
		return fmt.Errorf("请填写主机地址")
	}
	if strings.ContainsAny(share.Host, `\/`) {
		return fmt.Errorf("主机地址只写主机名或 IP，不带路径")
	}
	switch share.Protocol {
	case remoteSMB:
		if share.Share == "" {
			return fmt.Errorf("请填写共享名")
		}
		if share.Port < 0 || share.Port > 65535 {
			return fmt.Errorf("端口要在 1–65535 之间")
		}
	case remoteWebDAV:
		if share.Port < 0 || share.Port > 65535 {
			return fmt.Errorf("端口要在 1–65535 之间")
		}
	default:
		return fmt.Errorf("请选择协议")
	}
	return nil
}

// upsertRemoteShare saves a connection in settings.
// upsertRemoteShare saves a connection, replacing the entry it came from.
//
// The id is derived from the address, so editing the path or the port produces
// a new one. Saving by id alone would then append a second row and leave the
// old one behind, which is exactly what happened when a path was edited; the
// edit therefore carries the original id and replaces that entry.
func (a *App) upsertRemoteShare(share remoteShare) {
	edited := a.remoteEditID
	a.remoteEditID = ""
	out := make([]remoteShare, 0, len(a.settings.RemoteShares)+1)
	replaced := false
	for _, existing := range a.settings.RemoteShares {
		existingID := remoteShareID(existing)
		// Drop the entry being edited, and any stale duplicate of the target
		// address, so the list cannot hold the same share twice.
		if existingID == edited || existingID == share.ID {
			if remoteRoot(existing) != remoteRoot(share) {
				// A changed address loses the old session.
				remotePoolInstance.release(existingID)
			}
			if !replaced {
				out = append(out, share)
				replaced = true
			}
			continue
		}
		out = append(out, existing)
	}
	if !replaced {
		out = append(out, share)
	}
	a.settings.RemoteShares = out
	_ = saveSettings(a.settings)
}

// removeRemoteShare drops the connection, its session and its password.
func (a *App) removeRemoteShare(share remoteShare) {
	id := remoteShareID(share)
	remotePoolInstance.release(id)
	forgetRemotePassword(share)
	kept := a.settings.RemoteShares[:0]
	for _, existing := range a.settings.RemoteShares {
		if remoteShareID(existing) != id {
			kept = append(kept, existing)
		}
	}
	a.settings.RemoteShares = kept
	_ = saveSettings(a.settings)
	a.refresh()
}

// disconnectRemoteShare closes the session but keeps the entry.
func (a *App) disconnectRemoteShare(share remoteShare) {
	remotePoolInstance.release(remoteShareID(share))
	a.showStatusNotice("已断开 " + remoteRoot(share))
}

// sidebarRemoteRow is one saved connection.
func (a *App) sidebarRemoteRow(c *ui.Context, share remoteShare) {
	th := c.Theme()
	root := remoteRoot(share)
	active := false
	if t := a.tab(); t != nil {
		active = strings.EqualFold(remoteShareKey(t.Path), remoteShareKey(root))
	}
	// The row shows the alias when there is one and the address otherwise; the
	// tooltip always carries the address, so a short name never hides where it
	// actually points.
	name := remoteAlias(share)
	tooltip := root + "\n远程位置（只读）"
	if name == "" {
		name = strings.TrimPrefix(strings.TrimPrefix(root, remoteSMB+"://"), remoteWebDAV+"://")
	} else {
		tooltip = name + "\n" + root + "\n远程位置（只读）"
	}
	row := components.SidebarItemWithLeading(c, name, active, func(c *ui.Context, size float32) {
		color := th.TextMuted.Alpha(.9)
		if active {
			color = th.Accent
		}
		components.Icon(c, remoteProtocolIcon(share.Protocol)).Size(size, size).TextColor(color)
	})
	row.Key(root).Tooltip(tooltip)
	if row.Clicked() {
		a.goTo(root, true)
	}
	components.ContextMenu(c, row, func(m *components.Menu) {
		if m.Item("打开").Chosen() {
			a.later(func() { a.goTo(root, true) })
		}
		if m.Item("编辑连接…").Chosen() {
			a.later(func() { a.editRemoteShare(share) })
		}
		if m.Item("断开连接").Chosen() {
			a.later(func() { a.disconnectRemoteShare(share) })
		}
		if m.Item("移除").Chosen() {
			a.later(func() { a.removeRemoteShare(share) })
		}
	})
}

// remoteDialog is the add/edit connection dialog.
func (a *App) remoteDialog(c *ui.Context) {
	if !a.remoteDialogOpen {
		return
	}
	th := c.Theme()
	title := "添加远程位置"
	if a.remoteEditing {
		title = "编辑远程位置"
	}
	ui.Modal(c, &a.remoteDialogOpen, func() {
		// Runs before any field is built, so the port it shows is the one the
		// current protocol implies.
		a.syncPortToProtocol()
		ui.Column(c).Width(480).Gap(11).Children(func() {
			dialogHeader(c, lucide.Network(), title, "连接只在应用内建立；远程位置为只读")

			ui.Row(c).Gap(6).Children(func() {
				for _, protocol := range []struct {
					id    string
					label string
				}{{remoteSMB, "SMB"}, {remoteWebDAV, "WebDAV"}} {
					protocol := protocol
					variant := components.ButtonOutline
					if a.remoteDraft.Protocol == protocol.id {
						variant = components.ButtonPrimary
					}
					if components.Button(c, protocol.label, variant).Clicked() {
						if a.remoteDraft.Protocol != protocol.id {
							a.remoteDraft.Protocol = protocol.id
							a.refresh()
						}
					}
				}
			})

			host := components.TextBox(c, &a.remoteDraft.Host, components.TextBoxOptions{}).AutoFocus()
			host.Label("主机").Placeholder("主机名或 IP，例如 nas 或 192.168.1.10")

			ui.Row(c).Gap(8).FillWidth().Children(func() {
				components.NumberInput(c, &a.remotePortDraft, 1, 65535, 1).Label("端口").Width(120)
				user := components.TextBox(c, &a.remoteDraft.User, components.TextBoxOptions{}).Grow(1).MinWidth(0)
				user.Label("用户名").Placeholder("用户名（留空为匿名／当前会话）")
			})

			if a.remoteDraft.Protocol == remoteSMB {
				share := components.TextBox(c, &a.remoteDraft.Share, components.TextBoxOptions{})
				share.Label("共享名").Placeholder("共享名，例如 photos")
			} else {
				base := components.TextBox(c, &a.remoteDraft.Base, components.TextBoxOptions{})
				base.Label("路径").Placeholder("路径（可选），例如 dav/photos")
			}

			// The alias is what the sidebar and breadcrumb show; the address
			// stays available on hover and in the dialog, so a long share does
			// not have to be read every time.
			name := components.TextBox(c, &a.remoteDraft.Name, components.TextBoxOptions{})
			name.Label("别名").Placeholder("别名（可选），例如 家里照片")

			pass := components.TextBox(c, &a.remotePass, components.TextBoxOptions{}).Password()
			pass.Label("密码")
			if a.remoteEditing {
				pass.Placeholder("留空表示沿用已保存的密码")
			} else {
				pass.Placeholder("密码")
			}

			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				components.Switch(c, &a.remoteDraft.SavePass, "记住密码", components.SwitchOptions{Compact: true})
				ui.Spacer(c)
				components.Text(c, "DPAPI 加密后存在本机，仅此账号可解").FontSize(10).TextColor(th.TextMuted).SingleLine()
			})
			if a.remoteDraft.Protocol == remoteWebDAV {
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					components.Switch(c, &a.remoteDraft.Secure, "使用 HTTPS", components.SwitchOptions{Compact: true})
					ui.Spacer(c)
					components.Text(c, "群晖等 NAS 的 WebDAV 常是 http，按实际端口选").
						FontSize(10).TextColor(th.TextMuted).SingleLine()
				})
				// One switch, not a policy picker: a NAS certificate can be
				// self-signed, expired or issued for another name, and all of
				// those need the same thing. The warning is inline because the
				// consequence is not obvious from the label alone.
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					components.Switch(c, &a.remoteDraft.SkipVerify, "跳过证书验证", components.SwitchOptions{Compact: true})
					ui.Spacer(c)
					if a.remoteDraft.SkipVerify {
						components.Text(c, "不校验证书，容易被中间人冒充").FontSize(10).
							TextColor(th.Danger).SingleLine()
					} else {
						components.Text(c, "证书自签名或已过期时打开").FontSize(10).
							TextColor(th.TextMuted).SingleLine()
					}
				})
			}

			if a.remoteErr != "" {
				components.Text(c, a.remoteErr).FontSize(11).TextColor(th.Danger)
			}
			if a.remoteBusy {
				components.Text(c, "正在连接…").FontSize(11).TextColor(th.TextMuted)
			}

			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if components.Button(c, "取消", components.ButtonSecondary).Clicked() {
					a.remoteDialogOpen = false
					a.remoteEditID = ""
				}
				if components.Button(c, "连接", components.ButtonPrimary).Clicked() {
					a.later(a.commitRemoteShare)
				}
			})
		})
	})
}

// remoteDefaultPort is the port a protocol uses when the field still holds a
// default value. Many NAS boxes serve WebDAV over plain HTTP on 5005, so the
// HTTP default stays 80 rather than pretending everything is TLS.
func remoteDefaultPort(protocol string, secure bool) int {
	if protocol == remoteWebDAV {
		if secure {
			return 443
		}
		return 80
	}
	return 445
}

// isRemoteDefaultPort reports whether a port is one the dialog fills in by
// itself, so replacing it does not throw away a typed value.
func isRemoteDefaultPort(port int) bool {
	switch port {
	case 0, 80, 443, 445:
		return true
	}
	return false
}

// syncPortToProtocol moves the port field to the protocol's default when the
// protocol or HTTPS switch changed since the last frame. It runs at the top of
// the dialog, before the number field is built: the field copies its bound
// value into local text while it renders, so changing the value after that
// point is overwritten by the text it already had.
func (a *App) syncPortToProtocol() {
	if a.remotePortProtocol == a.remoteDraft.Protocol && a.remotePortSecure == a.remoteDraft.Secure {
		return
	}
	first := a.remotePortProtocol == "" && !a.remotePortSecure
	a.remotePortProtocol = a.remoteDraft.Protocol
	a.remotePortSecure = a.remoteDraft.Secure
	if first {
		// Opening the dialog seeds the field from the draft; nothing to move.
		a.remotePortDraft = float64(remoteDefaultPort(a.remoteDraft.Protocol, a.remoteDraft.Secure))
		if a.remoteDraft.Port > 0 {
			a.remotePortDraft = float64(a.remoteDraft.Port)
		}
		return
	}
	if isRemoteDefaultPort(int(a.remotePortDraft)) {
		a.remotePortDraft = float64(remoteDefaultPort(a.remoteDraft.Protocol, a.remoteDraft.Secure))
	}
}

// applyRemotePortDefault moves the port along with the protocol or the HTTPS
// switch, unless the user typed something of their own.
func (a *App) applyRemotePortDefault() {
	// Read the field, not the draft's copy: it is the authority now, and a
	// port the user typed must survive a protocol or HTTPS switch.
	port := int(a.remotePortDraft)
	if isRemoteDefaultPort(port) {
		port = remoteDefaultPort(a.remoteDraft.Protocol, a.remoteDraft.Secure)
	}
	a.remoteDraft.Port = port
	a.remotePortDraft = float64(port)
}

// remotePortDraft mirrors the port as a number for the input widget.
func (a *App) syncRemotePortDraft() {
	if a.remoteDraft.Port <= 0 {
		a.remotePortDraft = float64(remoteDefaultPort(a.remoteDraft.Protocol, a.remoteDraft.Secure))
	} else {
		a.remotePortDraft = float64(a.remoteDraft.Port)
	}
}

// remoteShareByID finds a saved share by its id.
func (a *App) remoteShareByID(id string) remoteShare {
	for _, share := range a.settings.RemoteShares {
		if remoteShareID(share) == id || remoteRoot(share) == id {
			return share
		}
	}
	return remoteShare{}
}
