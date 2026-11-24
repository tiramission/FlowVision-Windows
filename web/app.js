"use strict";

/* FlowVision for Windows — frontend.
   The page is the view; every file operation, scan, thumbnail and metadata
   read goes through the Go services bound by main.go. */

const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));
const clamp = (v, lo, hi) => Math.min(hi, Math.max(lo, v));

// On Windows MyGo serves custom schemes as http://<scheme>.localhost.
const IMG_ORIGIN = location.origin === "http://mygo.localhost"
  ? "http://img.localhost"
  : "img://localhost";

function call(method, ...args) {
  if (!window.mygo) return Promise.reject(new Error("MyGo runtime 不可用"));
  return window.mygo.call(method, ...args);
}

function esc(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}

// ------------------------------------------------------------------- state

const state = {
  startup: null,
  settings: null,
  tabs: [],
  active: -1,
  uid: 0,
  search: "",
  menuTarget: null,
  gesture: null,
  pendingOpen: null,
};

const viewer = {
  open: false,
  tab: null,
  index: -1,
  scale: 1,
  tx: 0,
  ty: 0,
  rot: 0,
  fit: true,
  mode: "image", // image | video | unsupported
  infoOpen: true,
};

let infoSeq = 0;

const el = {
  app: $("#app"),
  sidebar: $("#sidebar"),
  resizer: $("#resizer"),
  content: $("#content"),
  grid: $("#grid"),
  empty: $("#empty"),
  tabs: $("#tabs"),
  crumb: $("#crumb"),
  search: $("#search"),
  sort: $("#sort"),
  statusLeft: $("#status-left"),
  statusRight: $("#status-right"),
  viewer: $("#viewer"),
  vImage: $("#v-image"),
  vVideo: $("#v-video"),
  vStage: $("#v-stage"),
  vTitle: $("#v-title"),
  vSide: $("#v-side"),
  vSideBody: $("#v-side-body"),
  vZoom: $("#v-zoom"),
  vUnsupported: $("#v-unsupported"),
  menu: $("#menu"),
  dialogBack: $("#dialog-back"),
  dialog: $("#dialog"),
  toasts: $("#toasts"),
};

window.addEventListener("error", (e) => {
  window.__lastError = String(e.message || e.error);
  try { call("AppSvc.Log", "js error: " + window.__lastError); } catch (_) {}
});
window.addEventListener("unhandledrejection", (e) => {
  window.__lastError = String(e.reason);
  try { call("AppSvc.Log", "js rejection: " + window.__lastError); } catch (_) {}
});

// --------------------------------------------------------------- helpers

function activeTab() {
  return state.tabs[state.active] || null;
}

function thumbBucket(pixels) {
  for (const b of [96, 160, 240, 320, 480, 640, 800, 960]) if (b >= pixels) return b;
  return 960;
}

function itemVersion(item) {
  return `${item.mtime || 0}-${item.size || 0}`;
}

function thumbURL(item, pixels) {
  return `${IMG_ORIGIN}/t/${item.id}?s=${thumbBucket(pixels)}&v=${itemVersion(item)}`;
}

function origURL(item) {
  return `${IMG_ORIGIN}/o/${item.id}?v=${itemVersion(item)}`;
}

function previewURL(item) {
  return `${IMG_ORIGIN}/p/${item.id}?v=${itemVersion(item)}`;
}

function mediaURL(item) {
  const s = state.startup;
  if (!s || !s.mediaBase) return "";
  return `${s.mediaBase}/m/${item.id}?t=${s.mediaToken}&v=${itemVersion(item)}`;
}

function isConvertible(item) {
  return [".tif", ".tiff"].includes(item.ext);
}

// Formats the Go backend can decode for an oversized preview. AVIF/HEIC/JXL
// are browser or external decoders only, so they must keep their original.
function goPreviewable(item) {
  return [".jpg", ".jpeg", ".jpe", ".jfif", ".png", ".gif", ".webp", ".bmp",
    ".tif", ".tiff"].includes(item.ext);
}

function aspectOf(item) {
  if (item.w > 0 && item.h > 0) return clamp(item.w / item.h, 0.34, 3.2);
  return 1;
}

function fmtSize(n) {
  if (!n) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB", "PB", "EB"];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  return (i === 0 ? v : v.toFixed(1)) + " " + units[i];
}

function fmtDate(unix) {
  if (!unix) return "—";
  const d = new Date(unix * 1000);
  const p = (x) => String(x).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

let toastTimer = null;
function toast(title, sub = "", kind = "") {
  const t = document.createElement("div");
  t.className = "toast" + (kind ? " " + kind : "");
  t.innerHTML = `<div class="t-title">${esc(title)}</div>${sub ? `<div class="t-sub">${esc(sub)}</div>` : ""}`;
  el.toasts.appendChild(t);
  setTimeout(() => t.remove(), 4200);
}

function errMsg(e) {
  return typeof e === "string" ? e : (e && (e.message || e.error)) || String(e);
}

function samePath(a, b) {
  return !!a && !!b && a.toLowerCase() === b.toLowerCase();
}

function showError(e) {
  toast("操作失败", errMsg(e), "error");
}

// --------------------------------------------------------------- settings

function normalizeSettings(s) {
  const def = {
    theme: "system", thumbSize: 220, gridGap: 8, sort: "name", desc: false,
    showHidden: false, showOther: false, recursive: false, sidebar: true,
    sidebarWidth: 248, infoPanel: true, playVideo: true, loopVideo: false,
    lastPath: "", favorites: [], tabs: [], activeTab: 0, cacheLimitMB: 1500,
  };
  return Object.assign(def, s || {});
}

function applyTheme() {
  const t = state.settings.theme;
  let effective = t;
  if (t === "system") {
    effective = matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark";
  }
  document.documentElement.dataset.theme = effective;
}

matchMedia("(prefers-color-scheme: light)").addEventListener("change", () => {
  if (state.settings && state.settings.theme === "system") applyTheme();
});

let saveTimer = null;
function persist() {
  if (!state.settings) return;
  const tab = activeTab();
  state.settings.tabs = state.tabs.map((t) => ({
    path: t.path, sort: t.sort, desc: t.desc, scroll: t.id === (tab && tab.id) ? el.content.scrollTop : t.scroll,
  }));
  state.settings.activeTab = state.active;
  if (tab && tab.path) state.settings.lastPath = tab.path;
  clearTimeout(saveTimer);
  saveTimer = setTimeout(() => {
    call("Prefs.Save", state.settings).catch(showError);
  }, 450);
}

// ------------------------------------------------------------------- tabs

function makeTab(path, saved) {
  return {
    id: ++state.uid,
    path: path || "",
    name: "",
    parent: "",
    items: [],
    result: null,
    view: [],
    positions: [],
    totalHeight: 0,
    tiles: new Map(),
    sort: (saved && saved.sort) || state.settings.sort,
    desc: saved ? !!saved.desc : state.settings.desc,
    history: [],
    histIndex: -1,
    scroll: (saved && saved.scroll) || 0,
    sel: new Set(),
    anchor: -1,
    token: 0,
    loading: false,
  };
}

function renderTabs() {
  el.tabs.innerHTML = "";
  state.tabs.forEach((t, i) => {
    const d = document.createElement("div");
    d.className = "tab" + (i === state.active ? " active" : "");
    d.title = t.path || "新标签页";
    d.innerHTML = `<span class="tab-name">${esc(t.name || "新标签页")}</span><span class="x" title="关闭标签页">✕</span>`;
    d.addEventListener("pointerdown", (e) => {
      if (e.target.classList.contains("x")) {
        e.stopPropagation();
        closeTab(i);
        return;
      }
      if (i !== state.active) activateTab(i);
    });
    el.tabs.appendChild(d);
  });
  const activeEl = el.tabs.children[state.active];
  if (activeEl) {
    const tabsRect = el.tabs.getBoundingClientRect();
    const tabRect = activeEl.getBoundingClientRect();
    if (tabRect.left < tabsRect.left) el.tabs.scrollLeft -= tabsRect.left - tabRect.left;
    else if (tabRect.right > tabsRect.right) el.tabs.scrollLeft += tabRect.right - tabsRect.right;
  }
}

el.tabs.addEventListener("wheel", (e) => {
  if (el.tabs.scrollWidth <= el.tabs.clientWidth) return;
  el.tabs.scrollLeft += e.deltaY || e.deltaX;
  e.preventDefault();
}, { passive: false });

function closeTab(i) {
  if (state.tabs.length <= 1) {
    toast("至少保留一个标签页");
    return;
  }
  if (viewer.open && viewer.tab === state.tabs[i]) closeViewer();
  const wasActive = i === state.active;
  if (wasActive && viewer.open) closeViewer();
  state.tabs.splice(i, 1);
  if (wasActive) state.active = Math.min(i, state.tabs.length - 1);
  else if (i < state.active) state.active--;
  renderTabs();
  activateTab(state.active);
  persist();
}

async function newTab(path) {
  const t = makeTab(path || (activeTab() && activeTab().path));
  state.tabs.push(t);
  renderTabs();
  await activateTab(state.tabs.length - 1);
}

async function activateTab(i, restoring = false) {
  if (!state.tabs[i]) return;
  if (viewer.open && state.active !== i) closeViewer();
  if (state.active >= 0 && state.tabs[state.active] && state.active !== i) {
    state.tabs[state.active].scroll = el.content.scrollTop;
  }
  state.active = i;
  const t = state.tabs[i];
  renderTabs();
  document.title = (t.name ? t.name + " — " : "") + "FlowVision";
  if (t.path) {
    if (!t.result && !t.loading) {
      // A restored background tab already carries a saved scroll position.
      await browse(t, t.path, { restoring: restoring || (t.scroll || 0) > 0 });
    } else {
      // The filter box is shared by all tabs: an existing view must be
      // re-filtered when it becomes active, or it would silently ignore the
      // query still shown in the toolbar (or stay filtered after it is cleared).
      applyFilter(t);
      layout(t);
      el.content.scrollTop = t.scroll || 0;
      renderTiles(t);
      renderChrome(t);
    }
  } else {
    layout(t);
    renderChrome(t);
    emptyState("选择一个文件夹", "从左侧选择一个位置，或按 Ctrl+O 打开文件夹。");
    setStatus("就绪");
  }
  revealInTree(t.path);
}

function syncViewerToView(tab, viewedPath) {
  if (!viewer.open || viewer.tab !== tab) return;
  if (!viewedPath) {
    closeViewer();
    return;
  }
  const vi = tab.view.findIndex((it) => samePath(it.path, viewedPath));
  if (vi < 0) {
    closeViewer();
    return;
  }
  viewer.index = vi;
  el.vTitle.textContent = tab.view[vi].name + `   (${vi + 1}/${tab.view.length})`;
}

// ----------------------------------------------------------------- browse

async function browse(tab, path, opts = {}) {
  if (!tab) return false;
  path = String(path == null ? "" : path);
  if (!path) {
    toast("路径无效", "无法打开空路径", "error");
    return false;
  }
  const token = ++tab.token;
  // A viewer left open across a same-folder reload must follow the file,
  // not a numeric index: sorting, recursive and filters reorder the grid.
  const viewedPath = viewer.open && viewer.tab === tab && tab.view[viewer.index]
    ? tab.view[viewer.index].path : "";
  if (viewer.open && viewer.tab === tab && (!tab.path || path.toLowerCase() !== tab.path.toLowerCase())) {
    // A navigation that replaces the folder also invalidates the viewed item.
    closeViewer();
  }
  tab.loading = true;
  if (tab === activeTab()) setStatus("正在读取…");
  try {
    const res = await call("FS.Browse", {
      path,
      recursive: state.settings.recursive,
      sort: tab.sort,
      desc: tab.desc,
      showHidden: state.settings.showHidden,
      showOther: state.settings.showOther,
      limit: 6000,
    });
    if (token !== tab.token) return false;
    tab.loading = false;
    if (res && res.error) {
      browseFailed(tab, path, res.error);
      return false;
    }
    const sameFolder = tab.path && res.path.toLowerCase() === tab.path.toLowerCase();
    tab.path = res.path;
    tab.name = res.name || res.path;
    tab.parent = res.parent || "";
    tab.result = res;
    tab.items = res.items || [];
    invalidateTreeDir(res.path);
    tab.sel.clear();
    tab.anchor = -1;
    if (!sameFolder || opts.pushHistory !== false) pushHistory(tab, res.path);
    sortClient(tab);
    applyFilter(tab);
    syncViewerToView(tab, viewedPath);
    // Only the active tab owns nodes in the shared grid. A background browse
    // must not clear another tab's tiles or move its scroll position.
    if (tab === activeTab()) {
      layout(tab);
      el.content.scrollTop = opts.restoring ? tab.scroll : 0;
      renderTiles(tab);
      tab.scroll = el.content.scrollTop;
      renderChrome(tab);
      revealInTree(tab.path);
    } else {
      tab.scroll = 0;
    }
    renderTabs();
    persist();
    return true;
  } catch (e) {
    if (token !== tab.token) return false;
    tab.loading = false;
    browseFailed(tab, path, errMsg(e));
    return false;
  }
}

function browseFailText(path, msg) {
  return msg && msg !== "无法打开此文件夹" ? `无法打开 ${path}：${msg}` : `无法打开此文件夹：${path}`;
}

// browseFailed keeps whatever the tab was showing when a later navigation
// fails, instead of leaving stale tiles clickable under an error overlay.
function browseFailed(tab, path, msg) {
  toast("无法打开文件夹", msg, "error");
  if (tab !== activeTab()) {
    if (!tab.result) {
      tab.items = [];
      tab.view = [];
      tab.path = "";
      tab.name = "";
      tab.parent = "";
      renderTabs();
      persist();
    }
    return;
  }
  if (tab.result) {
    setStatus(browseFailText(path, msg));
    return;
  }
  tab.result = null;
  tab.items = [];
  tab.view = [];
  tab.path = "";
  tab.name = "";
  tab.parent = "";
  tab.tiles.forEach((t) => t.el.remove());
  tab.tiles.clear();
  layout(tab);
  renderChrome(tab);
  emptyState("无法打开此文件夹", browseFailText(path, msg));
  setStatus(browseFailText(path, msg));
  renderTabs();
  persist();
}

function pushHistory(tab, path) {
  if (tab.histIndex >= 0 && tab.history[tab.histIndex] === path) return;
  tab.history = tab.history.slice(0, tab.histIndex + 1);
  tab.history.push(path);
  tab.histIndex = tab.history.length - 1;
}

function histBack(tab) {
  if (tab.histIndex <= 0) return false;
  tab.histIndex--;
  browse(tab, tab.history[tab.histIndex], { pushHistory: false });
  return true;
}

async function openPath(path, opts = {}) {
  const tab = activeTab();
  if (!tab) {
    state.pendingOpen = { path, opts };
    return false;
  }
  return browse(tab, path, opts);
}

function sortClient(tab) {
  const key = tab.sort;
  const dir = tab.desc ? -1 : 1;
  const nat = (a, b) => a.localeCompare(b, "zh-Hans-CN", { numeric: true, sensitivity: "base" });
  const rank = (it) => (it.kind === "image" ? 0 : it.kind === "video" ? 1 : 2);
  tab.items.sort((a, b) => {
    let c = 0;
    if (key === "date") c = a.mtime - b.mtime;
    else if (key === "size") c = a.size - b.size;
    else if (key === "type") c = rank(a) - rank(b) || a.ext.localeCompare(b.ext) || nat(a.name, b.name);
    else c = nat(a.name, b.name);
    return c * dir;
  });
}

function applyFilter(tab) {
  const q = state.search.trim().toLowerCase();
  tab.view = q
    ? tab.items.filter((it) => it.name.toLowerCase().includes(q))
    : tab.items.slice();
}

// ----------------------------------------------------------------- layout

function layout(tab) {
  if (!tab || tab !== activeTab()) return;
  // The grid is shared by every tab. Drop the other tabs' nodes, but keep
  // this tab's existing tiles so a resize or a re-layout does not reload
  // every thumbnail.
  for (const t of state.tabs) {
    if (t === tab) continue;
    t.tiles.forEach((tile) => tile.el.remove());
    t.tiles.clear();
  }
  const keep = new Set();
  for (const t of tab.tiles.values()) keep.add(t.el);
  Array.from(el.grid.querySelectorAll(".tile")).forEach((n) => {
    if (!keep.has(n)) n.remove();
  });
  const rowH = clamp(state.settings.thumbSize, 110, 460);
  const gap = clamp(state.settings.gridGap, 0, 30);
  const padX = 16;
  const width = Math.max(200, el.content.clientWidth - padX * 2 - 12);
  const pos = new Array(tab.view.length);
  let x = 0;
  let y = 0;
  for (let i = 0; i < tab.view.length; i++) {
    const w = Math.min(Math.round(rowH * aspectOf(tab.view[i])), width);
    if (x > 0 && x + w > width) {
      x = 0;
      y += rowH + gap;
    }
    pos[i] = { x, y, w, h: rowH };
    x += w + gap;
  }
  tab.positions = pos;
  tab.totalHeight = (tab.view.length ? y + rowH : 0) + 36;
  el.grid.style.height = tab.totalHeight + "px";
  renderTiles(tab);
}

let rafPending = false;
function scheduleTiles() {
  if (rafPending) return;
  rafPending = true;
  requestAnimationFrame(() => {
    rafPending = false;
    const tab = activeTab();
    if (tab && tab.positions.length) renderTiles(tab);
  });
}

function renderTiles(tab) {
  if (!tab || state.tabs[state.active] !== tab) return;
  const st = el.content.scrollTop;
  const vh = el.content.clientHeight;
  const top = st - 800;
  const bottom = st + vh + 800;
  const padX = 16;
  const padY = 14;
  const wanted = new Set();
  for (let i = 0; i < tab.positions.length; i++) {
    const p = tab.positions[i];
    if (p.y + p.h < top || p.y > bottom) continue;
    const item = tab.view[i];
    wanted.add(item.id);
    const existing = tab.tiles.get(item.id);
    if (!existing || existing.el.dataset.kind !== item.kind) {
      if (existing) {
        existing.el.remove();
        tab.tiles.delete(item.id);
      }
      const node = makeTile(tab, item, p, i);
      tab.tiles.set(item.id, { el: node, index: i });
      el.grid.appendChild(node);
    } else {
      const t = existing;
      t.index = i;
      t.el.style.left = p.x + padX + "px";
      t.el.style.top = p.y + padY + "px";
      t.el.style.width = p.w + "px";
      t.el.style.height = p.h + "px";
      t.el.dataset.index = String(i);
      const nm = t.el.querySelector(".name");
      if (nm) nm.textContent = item.name;
      const img = t.el.querySelector("img");
      const h = Math.max(1, Math.round(p.h * (window.devicePixelRatio || 1)));
      const mt = itemVersion(item);
      if (img && (t.el.dataset.thumbH !== String(h) || t.el.dataset.mtime !== mt)) {
        t.el.dataset.thumbH = String(h);
        t.el.dataset.mtime = mt;
        img.dataset.retried = "";
        img.classList.remove("loaded", "failed");
        img.src = thumbURL(item, h);
      }
    }
  }
  for (const [id, t] of tab.tiles) {
    if (!wanted.has(id)) {
      t.el.remove();
      tab.tiles.delete(id);
    }
  }
  // Selection classes follow the ids, not the recycled nodes.
  for (const [id, t] of tab.tiles) {
    t.el.classList.toggle("selected", tab.sel.has(id));
  }
}

function makeTile(tab, item, p, index) {
  const node = document.createElement("div");
  node.className = "tile";
  node.dataset.id = item.id;
  node.dataset.index = String(index);
  node.dataset.kind = item.kind;
  node.style.left = p.x + 16 + "px";
  node.style.top = p.y + 14 + "px";
  node.style.width = p.w + "px";
  node.style.height = p.h + "px";

  const img = document.createElement("img");
  img.decoding = "async";
  // The service treats s as the thumbnail height, so ask for the tile's
  // height at the display scale (portrait tiles are tall but narrow).
  const thumbH = Math.max(1, Math.round(p.h * (window.devicePixelRatio || 1)));
  node.dataset.thumbH = String(thumbH);
  node.dataset.mtime = itemVersion(item);
  img.src = thumbURL(item, thumbH);
  const currentItem = () => tab.view.find((x) => x.id === item.id) || item;
  img.onload = () => {
    img.classList.add("loaded");
    const sh = node.querySelector(".shimmer");
    if (sh) sh.remove();
    const cur = currentItem();
    if (!cur.w && img.naturalWidth && img.naturalHeight) {
      cur.w = img.naturalWidth;
      cur.h = img.naturalHeight;
      scheduleRelayout(tab);
    }
  };
  img.onerror = () => {
    const cur = currentItem();
    if (!img.dataset.retried && cur.kind === "image") {
      img.dataset.retried = "1";
      img.src = isConvertible(cur) ? previewURL(cur) : origURL(cur);
      return;
    }
    node.classList.add("failed");
    img.classList.remove("loaded");
    const sh = node.querySelector(".shimmer");
    if (sh) sh.remove();
  };
  node.appendChild(img);

  const shimmer = document.createElement("div");
  shimmer.className = "shimmer";
  node.appendChild(shimmer);

  if (item.kind === "video") {
    const b = document.createElement("div");
    b.className = "badge";
    b.innerHTML = `<svg viewBox="0 0 24 24"><path d="M8 5l11 7-11 7z" fill="currentColor" stroke="none"/></svg><span>视频</span>`;
    node.appendChild(b);
  }

  const nm = document.createElement("div");
  nm.className = "name";
  nm.textContent = item.name;
  node.appendChild(nm);

  node.addEventListener("pointerdown", (e) => {
    const i = Number(node.dataset.index);
    if (e.button === 0 && Number.isInteger(i)) selectItem(tab, i, e.ctrlKey || e.metaKey, e.shiftKey);
  });
  node.addEventListener("dblclick", () => {
    const i = Number(node.dataset.index);
    if (Number.isInteger(i)) openViewer(tab, i);
  });
  return node;
}

let relayoutTimer = null;
function scheduleRelayout(tab) {
  clearTimeout(relayoutTimer);
  relayoutTimer = setTimeout(() => {
    // A thumbnail that finishes decoding after the user switched tabs must
    // not clear the tiles of the now-active tab.
    if (tab !== activeTab() || !tab.view.length) return;
    relayoutKeepingAnchor(tab);
  }, 260);
}

function findFirstVisible(tab) {
  const st = el.content.scrollTop;
  for (let i = 0; i < tab.positions.length; i++) if (tab.positions[i].y + tab.positions[i].h > st + 20) return i;
  return 0;
}

// Relayout while keeping the item under the viewport top at the same screen
// position. Snapping to its top would jump by a thumbnail height whenever a
// resize or a late image dimension arrives.
function relayoutKeepingAnchor(tab) {
  const st = el.content.scrollTop;
  const idx = findFirstVisible(tab);
  const old = tab.positions[idx];
  layout(tab);
  if (!old) return;
  const cur = tab.positions[idx];
  el.content.scrollTop = Math.max(0, cur ? cur.y + (st - old.y) : st);
}

// ------------------------------------------------------------- selection

function selectItem(tab, index, additive, range) {
  const item = tab.view[index];
  if (!item) return;
  if (range && tab.anchor >= 0) {
    const a = Math.min(tab.anchor, index);
    const b = Math.max(tab.anchor, index);
    if (!additive) tab.sel.clear();
    for (let i = a; i <= b; i++) tab.sel.add(tab.view[i].id);
  } else if (additive) {
    if (tab.sel.has(item.id)) tab.sel.delete(item.id);
    else tab.sel.add(item.id);
    tab.anchor = index;
  } else {
    tab.sel.clear();
    tab.sel.add(item.id);
    tab.anchor = index;
  }
  for (const [id, t] of tab.tiles) t.el.classList.toggle("selected", tab.sel.has(id));
  updateStatus(tab);
}

function selectedItems(tab) {
  return tab.view.filter((it) => tab.sel.has(it.id));
}

// ----------------------------------------------------------------- chrome

function renderChrome(tab) {
  if (!tab) return;
  renderCrumb(tab);
  el.sort.value = tab.sort;
  $("#btn-sort-dir").classList.toggle("active", tab.desc);
  document.title = (tab.name ? tab.name + " — " : "") + "FlowVision";
  const isEmpty = !tab.view.length && !tab.loading;
  el.empty.hidden = !isEmpty;
  if (isEmpty) {
    if (tab.result && tab.result.total === 0) emptyState("此文件夹没有图片或视频", "换一个文件夹，或打开“显示其他文件”。");
    else if (state.search) emptyState("没有匹配的文件", `“${state.search}” 在此文件夹中没有结果。`);
    else emptyState("空文件夹", "这个文件夹里没有可显示的内容。");
  }
  updateStatus(tab);
}

function emptyState(title, sub) {
  el.empty.hidden = false;
  $(".empty-title", el.empty).textContent = title;
  $(".empty-sub", el.empty).textContent = sub || "";
}

function renderCrumb(tab) {
  const raw = tab.path || "";
  const isUNC = raw.startsWith("\\\\") && !raw.startsWith("\\\\?\\");
  const parts = raw.split(/[\\/]+/).filter(Boolean);
  el.crumb.innerHTML = "";
  let acc = isUNC ? "\\\\" : "";
  parts.forEach((part, i) => {
    if (isUNC) {
      acc = i === 0 ? "\\\\" + part : acc.replace(/\\$/, "") + "\\" + part;
    } else if (i === 0) {
      acc = part + "\\";
    } else {
      acc = acc.replace(/\\$/, "") + "\\" + part;
    }
    if (i > 0) {
      const sep = document.createElement("span");
      sep.className = "crumb-sep";
      sep.textContent = "›";
      el.crumb.appendChild(sep);
    }
    const seg = document.createElement("span");
    seg.className = "crumb-seg" + (i === parts.length - 1 ? " current" : "");
    seg.textContent = part;
    seg.title = acc;
    seg.addEventListener("click", () => browse(tab, acc));
    el.crumb.appendChild(seg);
  });
}

function setStatus(text) {
  el.statusLeft.textContent = text;
}

function updateStatus(tab) {
  if (!tab || !tab.result) return;
  const r = tab.result;
  const sel = selectedItems(tab).length;
  let text = `${r.total} 个项目`;
  if (r.images) text += ` · ${r.images} 张图片`;
  if (r.videos) text += ` · ${r.videos} 个视频`;
  if (r.others) text += ` · ${r.others} 个其他文件`;
  if (r.truncated) text += " · 已截断";
  if (sel) text += ` · 已选 ${sel}`;
  if (state.search) text += ` · 筛选出 ${tab.view.length}`;
  el.statusLeft.textContent = text;
  el.statusRight.textContent = `缩略图 ${state.settings.thumbSize}px · ${r.elapsedMs} ms · v${state.startup.version}`;
}

// ------------------------------------------------------------------ viewer

function openViewer(tab, index) {
  if (!tab || !tab.view[index]) return;
  viewer.tab = tab;
  viewer.index = index;
  viewer.open = true;
  viewer.infoOpen = !!state.settings.infoPanel;
  $("#v-info-btn").classList.toggle("active", viewer.infoOpen);
  el.viewer.hidden = false;
  showViewerItem();
}

function currentViewerItem() {
  return viewer.tab && viewer.tab.view[viewer.index] || null;
}

function closeViewer() {
  viewer.open = false;
  ++infoSeq;
  el.viewer.hidden = true;
  el.vVideo.pause();
  el.vVideo.removeAttribute("src");
  el.vVideo.load();
  el.vVideo.dataset.item = "";
  el.vImage.dataset.item = "";
  el.vImage.hidden = true;
  el.vVideo.hidden = true;
  el.vUnsupported.hidden = true;
  el.vSide.hidden = true;
  const tab = viewer.tab;
  if (tab) {
    tab.tiles.forEach((t) => t.el.classList.toggle("selected", tab.sel.has(t.el.dataset.id)));
  }
}

function toggleInfoPanel(show) {
  if (typeof show === "boolean") viewer.infoOpen = show;
  else viewer.infoOpen = !viewer.infoOpen;
  el.vSide.hidden = !viewer.infoOpen;
  $("#v-info-btn").classList.toggle("active", viewer.infoOpen);
  state.settings.infoPanel = viewer.infoOpen;
  persist();
  if (viewer.open && viewer.mode === "image" && viewer.fit) applyViewerTransform();
}

function showViewerItem() {
  const tab = viewer.tab;
  const item = tab && tab.view[viewer.index];
  if (!item) return closeViewer();
  // Leaving a video must stop its audio, not just hide the element.
  if (el.vVideo.hasAttribute("src")) {
    el.vVideo.pause();
    el.vVideo.removeAttribute("src");
    el.vVideo.load();
  }
  viewer.rot = 0;
  viewer.fit = true;
  viewer.tx = 0;
  viewer.ty = 0;
  viewer.scale = 1;
  el.vTitle.textContent = item.name + `   (${viewer.index + 1}/${tab.view.length})`;
  el.vUnsupported.hidden = true;
  el.vImage.hidden = true;
  el.vVideo.hidden = true;
  el.vSide.hidden = !viewer.infoOpen;

  if (item.kind === "video" && !item.play) {
    // A container WebView2 cannot play must go to the system player even
    // when autoplay is off: otherwise its controls silently do nothing.
    showUnsupported(item, "WebView2 不支持这个视频容器（" + item.ext + "）。");
    return;
  }
  if (item.kind === "video") {
    const src = mediaURL(item);
    if (!src) {
      showUnsupported(item, "本地媒体服务未启动，无法播放视频。");
      return;
    }
    viewer.mode = "video";
    el.vVideo.hidden = false;
    el.vVideo.dataset.item = item.id;
    el.vVideo.src = src;
    el.vVideo.loop = !!state.settings.loopVideo;
    el.vVideo.autoplay = !!state.settings.playVideo;
    el.vVideo.onerror = () => {
      if (el.vVideo.dataset.item !== item.id) return;
      showUnsupported(item, "WebView2 无法解码这个视频（" + item.ext + "）。");
    };
    if (state.settings.playVideo) el.vVideo.play().catch(() => {});
    updateZoomLabel();
    loadInfo(item);
    return;
  }
  if (item.kind !== "image") {
    showUnsupported(item, "这不是可预览的图片或视频。");
    return;
  }
  viewer.mode = "image";
  el.vImage.hidden = false;
  el.vImage.dataset.item = item.id;
  const huge = item.w > 0 && item.h > 0 && item.w * item.h > 30_000_000;
  const src = isConvertible(item) || (huge && goPreviewable(item)) ? previewURL(item) : origURL(item);
  el.vImage.onload = () => {
    if (el.vImage.dataset.item !== item.id) return;
    el.vUnsupported.hidden = true;
    applyViewerTransform();
    updateZoomLabel();
  };
  el.vImage.onerror = () => {
    if (el.vImage.dataset.item !== item.id) return;
    showUnsupported(item, "这个格式无法在 WebView2 中解码（" + item.ext + "）。");
  };
  el.vImage.src = src;
  loadInfo(item);
}

function showUnsupported(item, reason) {
  viewer.mode = "unsupported";
  el.vVideo.pause();
  el.vVideo.removeAttribute("src");
  el.vVideo.load();
  el.vVideo.dataset.item = "";
  el.vImage.removeAttribute("src");
  el.vImage.dataset.item = "";
  el.vImage.hidden = true;
  el.vVideo.hidden = true;
  el.vUnsupported.hidden = false;
  $(".v-unsupported-sub", el.vUnsupported).textContent = reason;
  $("#v-unsupported-open").onclick = () => call("AppSvc.OpenExternal", item.path).catch(showError);
  updateZoomLabel();
  loadInfo(item);
}

function applyViewerTransform() {
  if (viewer.mode !== "image") return;
  const img = el.vImage;
  if (viewer.fit && img.naturalWidth) {
    const cw = el.vStage.clientWidth;
    const ch = el.vStage.clientHeight;
    const sideways = viewer.rot % 180 !== 0;
    const iw = sideways ? img.naturalHeight : img.naturalWidth;
    const ih = sideways ? img.naturalWidth : img.naturalHeight;
    viewer.scale = Math.min(cw / iw, ch / ih) * 0.95;
    viewer.tx = 0;
    viewer.ty = 0;
  }
  img.style.transform =
    `translate(-50%, -50%) translate(${viewer.tx}px, ${viewer.ty}px) scale(${viewer.scale}) rotate(${viewer.rot}deg)`;
}

function updateZoomLabel() {
  if (viewer.mode === "video") {
    el.vZoom.classList.remove("show");
    return;
  }
  if (viewer.mode === "image") {
    el.vZoom.textContent = Math.round(viewer.scale * 100) + "%";
  } else {
    el.vZoom.textContent = "无法预览";
  }
  el.vZoom.classList.add("show");
  clearTimeout(updateZoomLabel._t);
  updateZoomLabel._t = setTimeout(() => el.vZoom.classList.remove("show"), 1400);
}

function viewerStep(delta) {
  const tab = viewer.tab;
  if (!tab) return;
  const next = viewer.index + delta;
  if (next < 0 || next >= tab.view.length) return;
  viewer.index = next;
  showViewerItem();
}

async function loadInfo(item) {
  const seq = ++infoSeq;
  const base = `<div class="v-row"><span class="k">名称</span><span class="v">${esc(item.name)}</span></div>
    <div class="v-row"><span class="k">大小</span><span class="v">${fmtSize(item.size)}</span></div>
    <div class="v-row"><span class="k">修改时间</span><span class="v">${fmtDate(item.mtime)}</span></div>
    <div class="v-row"><span class="k">路径</span><span class="v v-path">${esc(item.path)}</span></div>`;
  el.vSideBody.innerHTML = base;
  try {
    const info = await call("MediaSvc.Info", item.id);
    if (seq !== infoSeq) return;
    const rows = [];
    if (info.width && info.height) rows.push(`<div class="v-row"><span class="k">尺寸</span><span class="v">${info.width} × ${info.height}</span></div>`);
    if (info.exif && info.exif.length) {
      rows.push(`<div class="v-sec">拍摄信息</div>`);
      for (const kv of info.exif) rows.push(`<div class="v-row"><span class="k">${esc(kv.k)}</span><span class="v">${esc(kv.v)}</span></div>`);
    }
    el.vSideBody.innerHTML = base + rows.join("");
  } catch (e) { /* metadata is optional */ }
}

// --------------------------------------------------------------- sidebar

function buildSidebar() {
  const s = state.startup;

  const fav = $("#fav-list");
  fav.innerHTML = "";
  const favs = state.settings.favorites || [];
  if (!favs.length) {
    fav.innerHTML = `<div class="side-item" style="color:var(--muted-2)">右键文件夹可收藏</div>`;
  }
  favs.forEach((p) => {
    fav.appendChild(sideItem("star", baseName(p), p, () => openPath(p), (e) => {
      renderMenu([
        ["打开", () => openPath(p)],
        ["在资源管理器中打开", () => call("AppSvc.OpenFolder", p).catch(showError)],
        null,
        ["取消收藏", () => {
          state.settings.favorites = state.settings.favorites.filter((x) => !samePath(x, p));
          buildSidebar();
          persist();
        }],
      ], e.clientX, e.clientY);
    }));
  });

  const places = $("#place-list");
  places.innerHTML = "";
  for (const p of s.places || []) {
    places.appendChild(sideItem(p.icon, p.name, p.path, () => openPath(p.path)));
  }
  for (const d of s.drives || []) {
    places.appendChild(sideItem("drive", d.name, d.path, () => openPath(d.path)));
  }

  buildTree();
}

function baseName(p) {
  const clean = String(p).replace(/[\\/]+$/, "");
  const parts = clean.split(/[\\/]/);
  return parts[parts.length - 1] || clean;
}

function sideItem(icon, label, path, onClick, onContext) {
  const d = document.createElement("div");
  d.className = "side-item";
  d.innerHTML = `${iconSVG(icon)}<span class="label">${esc(label)}</span>`;
  d.title = path;
  d.dataset.path = path;
  d.addEventListener("click", () => onClick && onClick());
  d.addEventListener("contextmenu", (e) => {
    e.preventDefault();
    if (onContext) onContext(e);
  });
  return d;
}

function iconSVG(name) {
  const icons = {
    drive: `<svg class="ico" viewBox="0 0 24 24"><rect x="3" y="5" width="18" height="14" rx="2"/><path d="M3 11h18" class="stroke"/><circle cx="7" cy="15" r="1" fill="currentColor" stroke="none"/></svg>`,
    desktop: `<svg class="ico" viewBox="0 0 24 24"><rect x="3" y="4" width="18" height="12" rx="1.5"/><path d="M9 20h6M12 16v4" class="stroke"/></svg>`,
    pictures: `<svg class="ico" viewBox="0 0 24 24"><rect x="3" y="5" width="18" height="14" rx="2"/><path d="M3 16l5-4 4 3 5-5 4 4" class="stroke"/></svg>`,
    downloads: `<svg class="ico" viewBox="0 0 24 24"><path d="M12 4v10M8 11l4 4 4-4M5 19h14" class="stroke"/></svg>`,
    documents: `<svg class="ico" viewBox="0 0 24 24"><path d="M7 3h7l4 4v14H7z" class="stroke"/><path d="M14 3v5h4" class="stroke"/></svg>`,
    videos: `<svg class="ico" viewBox="0 0 24 24"><rect x="3" y="6" width="13" height="12" rx="1.5"/><path d="M16 10l5-3v10l-5-3z" class="stroke"/></svg>`,
    star: `<svg class="ico" viewBox="0 0 24 24"><path d="M12 4l2.5 5.3 5.5.7-4 3.9 1 5.6-5 2.7-5-2.7 1-5.6-4-3.9 5.5-.7z" class="stroke"/></svg>`,
    folder: `<svg class="ico" viewBox="0 0 24 24"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" class="stroke"/></svg>`,
  };
  return icons[name] || icons.folder;
}

function buildTree() {
  const root = $("#tree-root");
  root.innerHTML = "";
  const drives = state.startup.drives || [];
  for (const d of drives) root.appendChild(treeRow(d.name, d.path, 0));
}

function treeRow(name, path, depth, hasMedia) {
  const wrap = document.createElement("div");
  const row = document.createElement("div");
  row.className = "tree-row";
  row.dataset.path = path;
  row.innerHTML = `<span class="twisty">${chevron()}</span>${iconSVG("folder")}<span class="label">${esc(name)}</span><span class="dot"></span>`;
  if (hasMedia === false) row.querySelector(".dot").hidden = true;
  const children = document.createElement("div");
  children.className = "tree-children";
  children.dataset.depth = String(depth);
  children.hidden = true;
  wrap.appendChild(row);
  wrap.appendChild(children);

  const twisty = row.querySelector(".twisty");
  row.addEventListener("click", async () => {
    const ok = await openPath(path);
    if (ok) markActiveTree(path);
  });
  twisty.addEventListener("click", async (e) => {
    e.stopPropagation();
    await toggleTree(row, children, path, depth, twisty);
  });
  row.addEventListener("contextmenu", (e) => {
    e.preventDefault();
    openMenu(e, null, path);
  });
  return wrap;
}

function chevron() {
  return `<svg viewBox="0 0 24 24" style="width:12px;height:12px"><path d="M9 6l6 6-6 6" class="stroke"/></svg>`;
}

async function toggleTree(row, children, path, depth, twisty) {
  if (!children.hidden) {
    children.hidden = true;
    twisty.classList.remove("open");
    return;
  }
  children.hidden = false;
  twisty.classList.add("open");
  if (children.dataset.loading) return;
  const placeholder = children.childElementCount === 1 && children.firstElementChild.classList.contains("tree-loading");
  if (children.childElementCount && !placeholder) return;
  await loadTreeChildren(children, path, depth, twisty);
}

async function loadTreeChildren(children, path, depth, twisty) {
  if (children.dataset.loading) return;
  const gen = String((Number(children.dataset.gen) || 0) + 1);
  children.dataset.gen = gen;
  children.dataset.loading = "1";
  children.innerHTML = `<div class="tree-loading">正在读取…</div>`;
  try {
    const kids = await call("FS.Tree", path, state.settings.showHidden);
    if (children.dataset.gen !== gen || children.hidden) return;
    children.innerHTML = "";
    twisty.classList.remove("empty");
    if (!kids || !kids.length) {
      twisty.classList.add("empty");
      const none = document.createElement("div");
      none.className = "tree-loading";
      none.textContent = "没有子文件夹";
      children.appendChild(none);
      return;
    }
    for (const k of kids) children.appendChild(treeRow(k.name, k.path, depth + 1, k.hasMedia));
  } catch (e) {
    if (children.dataset.gen !== gen) return;
    children.innerHTML = "";
    showError(e);
  } finally {
    if (children.dataset.gen === gen) delete children.dataset.loading;
  }
}

// Drop cached subfolder lists for one directory so a create/delete/refresh is
// visible in the sidebar without restarting the app. An expanded branch reloads
// in place; a collapsed one reloads the next time it is opened.
function invalidateTreeDir(dirPath) {
  const p = String(dirPath || "").toLowerCase().replace(/[\\/]+$/, "");
  if (!p) return;
  $$(".tree-row").forEach((row) => {
    if ((row.dataset.path || "").toLowerCase().replace(/[\\/]+$/, "") !== p) return;
    const children = row.nextElementSibling;
    if (!children || !children.classList.contains("tree-children")) return;
    const twisty = row.querySelector(".twisty");
    const depth = Number(children.dataset.depth) || 0;
    // Cancel any in-flight read for this branch and let a fresh one replace it.
    children.dataset.gen = String((Number(children.dataset.gen) || 0) + 1);
    delete children.dataset.loading;
    children.innerHTML = "";
    twisty.classList.remove("empty");
    if (!children.hidden) loadTreeChildren(children, row.dataset.path, depth, twisty);
  });
}

function markActiveTree(path) {
  if (!path) return;
  const p = path.toLowerCase();
  $$(".tree-row").forEach((r) => r.classList.toggle("active", (r.dataset.path || "").toLowerCase() === p));
  $$(".side-item").forEach((r) => r.classList.toggle("active", (r.dataset.path || "").toLowerCase() === p));
}

async function revealInTree(path) {
  if (!path) return;
  markActiveTree(path);
}

// -------------------------------------------------------------------- menu

function openMenu(e, tileEl, forcedPath) {
  const tab = activeTab();
  state.menuTarget = null;
  const items = [];
  if (tileEl) {
    const index = Number(tileEl.dataset.index);
    const item = tab.view[index];
    if (!item) return;
    state.menuTarget = { type: "item", item, tab };
    const openItem = () => {
      const i = tab.view.findIndex((x) => x.id === item.id);
      if (i >= 0) openViewer(tab, i);
    };
    items.push(["打开", openItem]);
    items.push(["用系统应用打开", () => call("AppSvc.OpenExternal", item.path).catch(showError)]);
    items.push(["在资源管理器中显示", () => call("AppSvc.Reveal", item.path).catch(showError)]);
    items.push(null);
    items.push(["复制路径", () => copyText(item.path)]);
    items.push(["文件信息", () => { openItem(); toggleInfoPanel(true); }]);
    items.push(null);
    items.push(["重命名…", () => renameDialog(item)]);
    items.push(["删除到回收站", () => deleteDialog(item, false), "danger"]);
    items.push(["永久删除", () => deleteDialog(item, true), "danger"]);
  } else {
    const path = forcedPath || (tab && tab.path);
    state.menuTarget = { type: "folder", path };
    const isCurrent = !!(path && tab && tab.path && path.toLowerCase() === tab.path.toLowerCase());
    if (isCurrent) items.push(["刷新视图", () => browse(tab, tab.path)]);
    items.push(["打开文件夹", () => openPath(path)]);
    items.push(["在资源管理器中打开", () => call("AppSvc.OpenFolder", path).catch(showError)]);
    items.push(null);
    items.push(["复制文件夹路径", () => copyText(path)]);
    items.push(["收藏此文件夹", () => addFavorite(path)]);
    items.push(["新建文件夹…", () => newFolderDialog(path)]);
  }
  renderMenu(items, e.clientX, e.clientY);
}

function renderMenu(items, x, y) {
  el.menu.innerHTML = "";
  items.forEach((it) => {
    if (!it) {
      const s = document.createElement("div");
      s.className = "sep";
      el.menu.appendChild(s);
      return;
    }
    const [label, fn, kind] = it;
    const d = document.createElement("div");
    d.className = "mi" + (kind === "danger" ? " danger" : "");
    d.textContent = label;
    d.addEventListener("click", () => {
      hideMenu();
      try { fn(); } catch (e) { showError(e); }
    });
    el.menu.appendChild(d);
  });
  el.menu.hidden = false;
  const w = el.menu.offsetWidth;
  const h = el.menu.offsetHeight;
  el.menu.style.left = Math.min(x, innerWidth - w - 8) + "px";
  el.menu.style.top = Math.min(y, innerHeight - h - 8) + "px";
}

function hideMenu() { el.menu.hidden = true; }
document.addEventListener("pointerdown", (e) => {
  if (!el.menu.hidden && !el.menu.contains(e.target)) hideMenu();
}, true);

// Keep the WebView's default context menu out of the chrome; editable fields
// retain the native cut/copy/paste menu.
document.addEventListener("contextmenu", (e) => {
  if (!e.target.closest("input, textarea")) e.preventDefault();
});

async function copyText(text) {
  try {
    await call("AppSvc.CopyText", text);
    toast("已复制到剪贴板");
  } catch (e) {
    toast("复制失败", errMsg(e), "error");
  }
}

function addFavorite(path) {
  if (!path) return;
  if (!state.settings.favorites.some((x) => samePath(x, path))) {
    state.settings.favorites.push(path);
    buildSidebar();
    persist();
    toast("已收藏");
  }
}

// ------------------------------------------------------------------ dialogs

function hideDialog() {
  el.dialogBack.hidden = true;
  el.dialog.innerHTML = "";
}

function dialog(html, setup) {
  el.dialog.innerHTML = html;
  el.dialogBack.hidden = false;
  const close = () => hideDialog();
  el.dialogBack.onclick = (e) => { if (e.target === el.dialogBack) close(); };
  $$("[data-close]", el.dialog).forEach((b) => b.addEventListener("click", close));
  if (setup) setup(close);
  return close;
}

function deleteDialog(items, permanent) {
  const list = Array.isArray(items) ? items : [items];
  const one = list.length === 1;
  dialog(
    `<div class="dlg-title">${one ? "删除“" + esc(list[0].name) + "”？" : "删除选中的 " + list.length + " 个文件？"}</div>
     <div class="dlg-body">${permanent ? "此操作会把文件从磁盘上永久删除，无法恢复。" : "文件将被移到回收站，可以从回收站还原。"}</div>
     <div class="dlg-buttons">
       <button class="btn" data-close>取消</button>
       <button class="btn danger" id="dlg-del">${permanent ? "永久删除" : "删除"}</button>
     </div>`,
    (close) => {
      let busy = false;
      $("#dlg-del").addEventListener("click", async () => {
        if (busy) return;
        busy = true;
        const btn = $("#dlg-del");
        btn.disabled = true;
        let done = 0;
        const failed = [];
        for (const it of list) {
          try {
            await call("AppSvc.Delete", it.path, permanent);
            done++;
          } catch (e) {
            failed.push(`${it.name}: ${errMsg(e)}`);
          }
        }
        if (!done) {
          busy = false;
          btn.disabled = false;
          toast("删除失败", failed.slice(0, 3).join("；"), "error");
          return;
        }
        close();
        const current = currentViewerItem();
        if (viewer.open && current && list.some((it) => samePath(it.path, current.path))) closeViewer();
        toast(permanent ? `已永久删除 ${done} 个文件` : `已移到回收站：${done} 个文件`);
        const tab = activeTab();
        if (tab && tab.path) browse(tab, tab.path);
        if (failed.length) toast("部分文件未删除", failed.slice(0, 3).join("；"), "error");
      });
    }
  );
}

function renameDialog(item) {
  const ext = item.ext || "";
  const stem = ext && item.name.toLowerCase().endsWith(ext) ? item.name.slice(0, -ext.length) : item.name;
  dialog(
    `<div class="dlg-title">重命名</div>
     <div class="dlg-field"><label>名称</label><input type="text" id="dlg-name" value="${esc(stem)}" /></div>
     <div class="dlg-body">扩展名保持为 ${esc(ext || "（无）")}。</div>
     <div class="dlg-buttons"><button class="btn" data-close>取消</button><button class="btn primary" id="dlg-ok">重命名</button></div>`,
    (close) => {
      const input = $("#dlg-name");
      input.focus();
      input.select();
      let busy = false;
      const go = async () => {
        if (busy) return;
        const v = input.value.trim();
        if (!v) return;
        // The dialog promises to keep the original extension. Accept it
        // case-insensitively, append it when omitted, and reject a different
        // one instead of silently producing "b.png.jpg".
        let finalName = v;
        const typed = /\.[a-z0-9]{1,8}$/i.exec(v);
        const typedExt = typed ? typed[0].toLowerCase() : "";
        if (ext && typedExt && typedExt !== ext.toLowerCase()) {
          toast("扩展名不能修改", `请保持为 ${ext}，或去掉输入的扩展名。`, "error");
          return;
        }
        if (ext && !typedExt) finalName = v + ext;
        if (finalName === item.name) { close(); return; }
        busy = true;
        const ok = $("#dlg-ok");
        ok.disabled = true;
        try {
          const next = await call("AppSvc.Rename", item.path, finalName);
          close();
          const current = currentViewerItem();
          if (viewer.open && current && samePath(current.path, item.path)) closeViewer();
          const tab = activeTab();
          if (tab) browse(tab, tab.path);
          toast("已重命名", next);
        } catch (e) {
          busy = false;
          ok.disabled = false;
          showError(e);
        }
      };
      $("#dlg-ok").addEventListener("click", go);
      input.addEventListener("keydown", (e) => { if (e.key === "Enter") go(); });
    }
  );
}

function newFolderDialog(dir) {
  dialog(
    `<div class="dlg-title">新建文件夹</div>
     <div class="dlg-field"><label>名称</label><input type="text" id="dlg-name" value="新建文件夹" /></div>
     <div class="dlg-buttons"><button class="btn" data-close>取消</button><button class="btn primary" id="dlg-ok">创建</button></div>`,
    (close) => {
      const input = $("#dlg-name");
      input.focus();
      input.select();
      let busy = false;
      const go = async () => {
        if (busy) return;
        const name = input.value.trim();
        if (!name) return;
        busy = true;
        const ok = $("#dlg-ok");
        ok.disabled = true;
        try {
          await call("AppSvc.NewFolder", dir, name);
          close();
          toast("已创建文件夹");
          invalidateTreeDir(dir);
          const tab = activeTab();
          if (tab && samePath(tab.path, dir)) browse(tab, dir);
        } catch (e) {
          busy = false;
          ok.disabled = false;
          showError(e);
        }
      };
      $("#dlg-ok").addEventListener("click", go);
      input.addEventListener("keydown", (e) => { if (e.key === "Enter") go(); });
    }
  );
}

function settingsDialog() {
  const s = state.settings;
  dialog(
    `<div class="dlg-title">设置</div>
     <div class="dlg-row"><span class="name">外观</span>
       <select id="set-theme"><option value="system">跟随系统</option><option value="dark">深色</option><option value="light">浅色</option></select></div>
     <div class="dlg-row"><span class="name">缩略图大小</span><input type="range" id="set-thumb" min="120" max="420" step="10" value="${s.thumbSize}"><span class="hint" id="set-thumb-v">${s.thumbSize}px</span></div>
     <div class="dlg-row"><span class="name">网格间距</span><input type="range" id="set-gap" min="0" max="24" step="1" value="${s.gridGap}"><span class="hint" id="set-gap-v">${s.gridGap}px</span></div>
     <div class="dlg-row"><span class="name">默认排序</span><select id="set-sort"><option value="name">名称</option><option value="date">日期</option><option value="size">大小</option><option value="type">类型</option></select></div>
     <div class="dlg-row"><span class="name">递归模式</span><input type="checkbox" id="set-recursive" ${s.recursive ? "checked" : ""}></div>
     <div class="dlg-row"><span class="name">显示隐藏文件</span><input type="checkbox" id="set-hidden" ${s.showHidden ? "checked" : ""}></div>
     <div class="dlg-row"><span class="name">显示其他文件</span><input type="checkbox" id="set-other" ${s.showOther ? "checked" : ""}></div>
     <div class="dlg-row"><span class="name">自动播放视频</span><input type="checkbox" id="set-play" ${s.playVideo ? "checked" : ""}></div>
     <div class="dlg-row"><span class="name">循环播放</span><input type="checkbox" id="set-loop" ${s.loopVideo ? "checked" : ""}></div>
     <div class="dlg-row"><span class="name">缓存上限 (MB)</span><input type="number" id="set-cache" min="200" max="20000" step="100" value="${s.cacheLimitMB}"></div>
     <div class="dlg-row"><span class="name">缩略图缓存</span><span class="hint" id="set-cache-info">—</span><button class="btn" id="set-clear">清空</button></div>
     <div class="dlg-buttons"><button class="btn" data-close>取消</button><button class="btn primary" id="dlg-save">保存</button></div>`,
    (close) => {
      $("#set-theme").value = s.theme;
      $("#set-sort").value = s.sort;
      $("#set-thumb").addEventListener("input", (e) => { $("#set-thumb-v").textContent = e.target.value + "px"; });
      $("#set-gap").addEventListener("input", (e) => { $("#set-gap-v").textContent = e.target.value + "px"; });
      call("Prefs.Cache").then((c) => { $("#set-cache-info").textContent = `${c.files} 个文件 · ${fmtSize(c.bytes)}`; }).catch(() => {});
      $("#set-clear").addEventListener("click", async () => {
        try {
          await call("Prefs.ClearCache");
          $("#set-cache-info").textContent = "0 个文件 · 0 B";
          const tab = activeTab();
          if (tab) { tab.tiles.forEach((t) => t.el.remove()); tab.tiles.clear(); layout(tab); }
          toast("缓存已清空");
        } catch (e) { showError(e); }
      });
      $("#dlg-save").addEventListener("click", async () => {
        s.theme = $("#set-theme").value;
        s.thumbSize = Number($("#set-thumb").value);
        s.gridGap = Number($("#set-gap").value);
        s.sort = $("#set-sort").value;
        s.recursive = $("#set-recursive").checked;
        s.showHidden = $("#set-hidden").checked;
        s.showOther = $("#set-other").checked;
        s.playVideo = $("#set-play").checked;
        s.loopVideo = $("#set-loop").checked;
        s.cacheLimitMB = clamp(Number($("#set-cache").value) || 1500, 200, 20000);
        document.documentElement.style.setProperty("--thumb", s.thumbSize + "px");
        applyTheme();
        close();
        try { await call("Prefs.Save", s); } catch (e) { showError(e); }
        const tab = activeTab();
        if (tab && tab.path) { tab.sort = s.sort; browse(tab, tab.path); }
      });
    }
  );
}

function helpDialog() {
  dialog(
    `<div class="dlg-title">快捷键与手势</div>
     <div class="dlg-body">
      <div class="v-sec">浏览</div>
      W 上一级 · S 返回 · A 上一文件夹 · D 下一文件夹 · Ctrl+O 打开文件夹 · F5 刷新 · Ctrl+F 筛选<br>
      Ctrl+T 新建标签页 · Ctrl+W 关闭标签页 · Ctrl+B 侧边栏 · Ctrl+- / Ctrl+= 缩略图缩放<br><br>
      <div class="v-sec">查看</div>
      双击缩略图打开 · 空格/→/↓ 下一张 · ←/↑ 上一张 · Esc 关闭 · I 信息 · F 适应窗口 · 1 100% · R 旋转<br>
      滚轮缩放 · 拖动平移 · 双击切换适应/100%<br><br>
      <div class="v-sec">右键手势（拖拽右键）</div>
      右：下一文件夹 · 左：上一文件夹 · 上：上一级 · 下：返回 · 右上：同级下一个含图片的文件夹 · 右下：关闭标签页
     </div>
     <div class="dlg-buttons"><button class="btn primary" data-close>知道了</button></div>`
  );
}

function aboutDialog() {
  dialog(
    `<div class="dlg-title">FlowVision for Windows</div>
     <div class="dlg-body">
       雨流式（真实比例）图片浏览器。用 Go + MyGo 重写自 macOS 版 FlowVision，运行在 WebView2 上。<br><br>
       版本 ${esc(state.startup.version)} · 原项目 GPL-3.0 · 本移植版同样以 GPL-3.0 发布。
     </div>
     <div class="dlg-buttons"><button class="btn primary" data-close>关闭</button></div>`
  );
}

// -------------------------------------------------------------- navigation

async function goParent() {
  const tab = activeTab();
  if (!tab || !tab.parent) return;
  browse(tab, tab.parent);
}

async function goSibling(direction) {
  const tab = activeTab();
  if (!tab || !tab.path) return;
  try {
    const r = await call("FS.Sibling", tab.path, direction, !!state.settings.showHidden);
    if (r && r.found) browse(tab, r.path);
    else toast("没有更多包含图片的文件夹");
  } catch (e) { showError(e); }
}

// -------------------------------------------------------------- interactions

el.content.addEventListener("scroll", () => {
  const t = activeTab();
  if (t) t.scroll = el.content.scrollTop;
  scheduleTiles();
  // persist() debounces the write, so a scroll gesture stores its final
  // position even when no later tab switch happens.
  persist();
}, { passive: true });

let resizeTimer = null;

function relayoutAfterResize() {
  const tab = activeTab();
  if (tab) {
    relayoutKeepingAnchor(tab);
    updateStatus(tab);
  }
  if (viewer.open && viewer.mode === "image" && viewer.fit) applyViewerTransform();
}

function scheduleRelayoutWindow() {
  clearTimeout(resizeTimer);
  resizeTimer = setTimeout(relayoutAfterResize, 180);
}

window.addEventListener("resize", scheduleRelayoutWindow);
// The window is not the only thing that changes content width: hiding the
// sidebar or dragging its splitter does too, and the tile positions must be
// recomputed or the last column overflows.
new ResizeObserver(scheduleRelayoutWindow).observe(el.content);

el.content.addEventListener("click", (e) => {
  if (e.button !== 0 || e.target.closest(".tile")) return;
  const tab = activeTab();
  if (!tab || !tab.sel.size) return;
  tab.sel.clear();
  tab.anchor = -1;
  for (const t of tab.tiles.values()) t.el.classList.remove("selected");
  updateStatus(tab);
});

el.content.addEventListener("pointerdown", (e) => {
  if (e.button !== 2) return;
  state.gesture = { x: e.clientX, y: e.clientY, moved: false, tile: e.target.closest(".tile") };
  try { el.content.setPointerCapture(e.pointerId); } catch (_) {}
});
el.content.addEventListener("pointermove", (e) => {
  const g = state.gesture;
  if (!g) return;
  if (Math.hypot(e.clientX - g.x, e.clientY - g.y) > 8) g.moved = true;
});
el.content.addEventListener("pointerup", (e) => {
  const g = state.gesture;
  state.gesture = null;
  if (e.button !== 2 || !g) return;
  const dx = e.clientX - g.x;
  const dy = e.clientY - g.y;
  const thr = 46;
  const ax = Math.abs(dx);
  const ay = Math.abs(dy);
  if (Math.max(ax, ay) < thr) {
    openMenu(e, g.tile);
    return;
  }
  if (ax >= thr && ay >= thr) {
    if (dx > 0 && dy < 0) goSibling(1);
    else if (dx > 0 && dy > 0) closeTab(state.active);
    else if (dx < 0 && dy < 0) goParent();
    else histBack(activeTab());
    return;
  }
  if (ax > ay) {
    goSibling(dx > 0 ? 1 : -1);
    return;
  }
  if (dy < 0) goParent();
  else histBack(activeTab());
});
el.content.addEventListener("pointercancel", () => { state.gesture = null; });
el.content.addEventListener("contextmenu", (e) => {
  // The right-button gesture opens its menu on pointerup; opening here would
  // leave that menu hanging over a folder swipe.
  e.preventDefault();
});

// Drag & drop: dropping a folder (or a file in Explorer) opens its folder.
["dragover", "dragenter"].forEach((ev) => el.content.addEventListener(ev, (e) => {
  e.preventDefault();
  el.content.classList.add("dragover");
}));
el.content.addEventListener("dragleave", () => el.content.classList.remove("dragover"));
el.content.addEventListener("drop", async (e) => {
  e.preventDefault();
  el.content.classList.remove("dragover");
  const p = dropPath(e.dataTransfer);
  if (p) await openDroppedPath(p);
});

let lastDrop = { path: "", at: 0 };
async function openDroppedPath(path) {
  if (!path) return;
  const now = Date.now();
  if (samePath(path, lastDrop.path) && now - lastDrop.at < 1500) return;
  lastDrop = { path, at: now };
  try {
    const dir = await call("AppSvc.ResolveDrop", path);
    if (dir) await openPath(dir);
    else toast("无法打开拖入的项目", path, "error");
  } catch (err) { showError(err); }
}

// MyGo reads the real paths of files dragged from Explorer (the DOM File
// objects intentionally expose none) and emits this trusted-page event.
if (window.mygo && typeof window.mygo.on === "function") {
  window.mygo.on("mygo:file-drop", (e) => {
    const paths = (e && e.paths) || [];
    if (paths.length) openDroppedPath(paths[0]);
  });
}

function dropPath(dt) {
  if (!dt) return "";
  const raw = dt.getData("text/uri-list") || dt.getData("text/plain") || "";
  const line = raw.split(/\r?\n/).map((s) => s.trim()).find((s) => s && !s.startsWith("#"));
  if (!line) {
    const f = dt.files && dt.files[0];
    return f && f.path ? f.path : "";
  }
  if (!/^file:/i.test(line)) return line; // Some apps put a plain path on the clipboard.
  let s = line.slice(5);
  let unc = false;
  if (s.startsWith("///")) s = s.slice(3);
  else if (s.startsWith("//")) { s = s.slice(2); unc = true; }
  try { s = decodeURIComponent(s); } catch (_) { /* keep raw */ }
  if (/^\/[a-z]:/i.test(s)) s = s.slice(1);
  s = s.replace(/\//g, "\\");
  return unc ? "\\\\" + s : s;
}

// ------------------------------------------------------------------ keyboard

document.addEventListener("keydown", (e) => {
  const target = e.target;
  const typing = target && (target.tagName === "INPUT" || target.tagName === "SELECT" || target.tagName === "TEXTAREA");
  const onButton = target && target.closest && target.closest("button, a");

  // A modal dialog owns the keyboard while it is open.
  if (!el.dialogBack.hidden) {
    if (e.key === "Escape") { hideDialog(); e.preventDefault(); }
    return;
  }

  // Leave activation keys alone when a control itself has focus.
  if (onButton && (e.key === " " || e.key === "Enter")) return;
  if (e.key === "F11") {
    toggleViewFullscreen();
    e.preventDefault();
    return;
  }

  if (viewer.open) {
    if (e.key === "Escape") { closeViewer(); e.preventDefault(); return; }
    if (typing || e.ctrlKey || e.metaKey || e.altKey) return;
    if (["ArrowRight", "ArrowDown", " "].includes(e.key)) { viewerStep(1); e.preventDefault(); return; }
    if (["ArrowLeft", "ArrowUp"].includes(e.key)) { viewerStep(-1); e.preventDefault(); return; }
    if (e.key === "i" || e.key === "I") { toggleInfoPanel(); e.preventDefault(); return; }
    if (e.key === "f" || e.key === "F") { viewer.fit = true; applyViewerTransform(); updateZoomLabel(); e.preventDefault(); return; }
    if (e.key === "1") { viewer.fit = false; viewer.scale = 1; viewer.tx = 0; viewer.ty = 0; applyViewerTransform(); updateZoomLabel(); e.preventDefault(); return; }
    if (e.key === "r" || e.key === "R") { viewer.rot = (viewer.rot + 90) % 360; applyViewerTransform(); e.preventDefault(); return; }
    return;
  }

  if (typing) {
    if (e.key === "Escape") target.blur();
    return;
  }
  if (e.key === "Escape") { hideMenu(); hideDialog(); return; }

  const tab = activeTab();
  if (!tab) return;

  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "f") { el.search.focus(); el.search.select(); e.preventDefault(); return; }
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "b") { toggleSidebar(); e.preventDefault(); return; }
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "t") { newTab(); e.preventDefault(); return; }
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "w") { closeTab(state.active); e.preventDefault(); return; }
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "o") { pickFolder(); e.preventDefault(); return; }
  if (e.key === "F5") { if (tab.path) browse(tab, tab.path); e.preventDefault(); return; }
  if (e.key === "Delete") { const items = selectedItems(tab); if (items.length) deleteDialog(items, false); e.preventDefault(); return; }
  if (e.key === "F2") { const items = selectedItems(tab); if (items.length) renameDialog(items[0]); e.preventDefault(); return; }
  if (e.key === "Enter") { const items = selectedItems(tab); if (items.length) { const idx = tab.view.indexOf(items[0]); openViewer(tab, idx); } e.preventDefault(); return; }
  if (e.key === " ") { const items = selectedItems(tab); const idx = items.length ? tab.view.indexOf(items[0]) : 0; openViewer(tab, idx); e.preventDefault(); return; }

  const key = e.key.toLowerCase();
  if (e.ctrlKey || e.metaKey || e.altKey) return;
  if (key === "w") { goParent(); e.preventDefault(); return; }
  if (key === "a") { goSibling(-1); e.preventDefault(); return; }
  if (key === "d") { goSibling(1); e.preventDefault(); return; }
  if (key === "s") { histBack(tab); e.preventDefault(); return; }

  if (["ArrowRight", "ArrowLeft", "ArrowUp", "ArrowDown"].includes(e.key)) {
    moveSelection(tab, e.key);
    e.preventDefault();
  }
});

function moveSelection(tab, key) {
  if (!tab.view.length) return;
  const cur = tab.anchor >= 0 ? tab.anchor : 0;
  let next = cur;
  if (key === "ArrowRight") next = Math.min(tab.view.length - 1, cur + 1);
  if (key === "ArrowLeft") next = Math.max(0, cur - 1);
  if (key === "ArrowUp" || key === "ArrowDown") {
    const p = tab.positions[cur];
    const dir = key === "ArrowUp" ? -1 : 1;
    let best = -1;
    let bestScore = Infinity;
    for (let i = 0; i < tab.positions.length; i++) {
      const q = tab.positions[i];
      const rowDiff = q.y - p.y;
      if (dir < 0 ? rowDiff >= 0 : rowDiff <= 0) continue;
      const score = Math.abs(rowDiff) * 100 + Math.abs(q.x - p.x);
      if (score < bestScore) { bestScore = score; best = i; }
    }
    if (best >= 0) next = best;
  }
  selectItem(tab, next, false, false);
  const pt = tab.positions[next];
  if (pt) el.content.scrollTop = Math.max(0, pt.y - 40);
}

// ------------------------------------------------------------------ sidebar

function toggleSidebar() {
  const hidden = el.sidebar.classList.toggle("hidden");
  state.settings.sidebar = !hidden;
  persist();
}

const resizer = $("#resizer");
resizer.addEventListener("pointerdown", (e) => {
  e.preventDefault();
  resizer.setPointerCapture(e.pointerId);
  const startX = e.clientX;
  const startW = el.sidebar.getBoundingClientRect().width;
  const move = (ev) => {
    const w = clamp(startW + ev.clientX - startX, 180, 480);
    el.sidebar.style.width = w + "px";
    state.settings.sidebarWidth = w;
  };
  const up = () => {
    resizer.removeEventListener("pointermove", move);
    resizer.removeEventListener("pointerup", up);
    persist();
  };
  resizer.addEventListener("pointermove", move);
  resizer.addEventListener("pointerup", up);
});

// --------------------------------------------------------------- buttons

$("#btn-pick-folder").addEventListener("click", pickFolder);
$("#btn-newtab").addEventListener("click", () => newTab());
$("#btn-back").addEventListener("click", () => histBack(activeTab()));
$("#btn-prev-folder").addEventListener("click", () => goSibling(-1));
$("#btn-next-folder").addEventListener("click", () => goSibling(1));
$("#btn-up").addEventListener("click", goParent);
$("#btn-refresh").addEventListener("click", () => { const tab = activeTab(); if (tab && tab.path) browse(tab, tab.path); });
$("#btn-settings").addEventListener("click", settingsDialog);
$("#btn-thumb-smaller").addEventListener("click", () => changeThumb(-24));
$("#btn-thumb-larger").addEventListener("click", () => changeThumb(24));
$("#btn-sort-dir").addEventListener("click", () => {
  const tab = activeTab();
  if (!tab) return;
  const viewedPath = viewer.open && viewer.tab === tab && tab.view[viewer.index]
    ? tab.view[viewer.index].path : "";
  tab.desc = !tab.desc;
  state.settings.desc = tab.desc;
  sortClient(tab);
  applyFilter(tab);
  syncViewerToView(tab, viewedPath);
  el.content.scrollTop = 0;
  layout(tab);
  renderChrome(tab);
  persist();
});
$("#btn-view-fullscreen").addEventListener("click", toggleViewFullscreen);

function toggleViewFullscreen() {
  if (document.fullscreenElement) {
    document.exitFullscreen().catch(() => {});
  } else {
    document.documentElement.requestFullscreen().catch(() => toast("无法进入全屏", "当前窗口不允许全屏。", "error"));
  }
}
el.sort.addEventListener("change", () => {
  const tab = activeTab();
  if (!tab) return;
  const viewedPath = viewer.open && viewer.tab === tab && tab.view[viewer.index]
    ? tab.view[viewer.index].path : "";
  tab.sort = el.sort.value;
  state.settings.sort = tab.sort;
  sortClient(tab);
  applyFilter(tab);
  syncViewerToView(tab, viewedPath);
  el.content.scrollTop = 0;
  layout(tab);
  renderChrome(tab);
  persist();
});

let searchTimer = null;
el.search.addEventListener("input", () => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => {
    state.search = el.search.value;
    const tab = activeTab();
    if (!tab) return;
    const viewedPath = viewer.open && viewer.tab === tab && tab.view[viewer.index]
      ? tab.view[viewer.index].path : "";
    tab.sel.clear();
    tab.anchor = -1;
    applyFilter(tab);
    syncViewerToView(tab, viewedPath);
    tab.tiles.forEach((t) => t.el.remove());
    tab.tiles.clear();
    layout(tab);
    el.content.scrollTop = 0;
    renderTiles(tab);
    renderChrome(tab);
  }, 180);
});

async function changeThumb(delta) {
  const tab = activeTab();
  state.settings.thumbSize = clamp(state.settings.thumbSize + delta, 120, 420);
  document.documentElement.style.setProperty("--thumb", state.settings.thumbSize + "px");
  if (tab) {
    relayoutKeepingAnchor(tab);
    updateStatus(tab);
  }
  persist();
}

async function pickFolder() {
  try {
    const p = await call("AppSvc.PickFolder", activeTab() ? activeTab().path : "");
    if (p) {
      if (activeTab()) await openPath(p);
      else await newTab(p);
    }
  } catch (e) { showError(e); }
}

// ------------------------------------------------------------------ viewer

el.vImage.addEventListener("error", () => {
  if (!viewer.open) return;
  const item = viewer.tab && viewer.tab.view[viewer.index];
  if (item && item.kind === "image" && el.vImage.dataset.item === item.id) {
    showUnsupported(item, "这个格式无法在 WebView2 中解码（" + item.ext + "）。");
  }
});

el.vStage.addEventListener("pointerdown", (e) => {
  if (viewer.mode !== "image") return;
  if (e.button !== 0 && e.button !== 1) return;
  const start = { x: e.clientX, y: e.clientY, tx: viewer.tx, ty: viewer.ty };
  el.vStage.setPointerCapture(e.pointerId);
  el.vStage.classList.add("panning");
  const move = (ev) => {
    viewer.fit = false;
    viewer.tx = start.tx + (ev.clientX - start.x);
    viewer.ty = start.ty + (ev.clientY - start.y);
    applyViewerTransform();
  };
  const up = () => {
    el.vStage.classList.remove("panning");
    el.vStage.removeEventListener("pointermove", move);
    el.vStage.removeEventListener("pointerup", up);
    updateZoomLabel();
  };
  el.vStage.addEventListener("pointermove", move);
  el.vStage.addEventListener("pointerup", up);
});

el.vStage.addEventListener("wheel", (e) => {
  if (viewer.mode !== "image") return;
  e.preventDefault();
  const old = viewer.scale;
  const next = clamp(old * Math.exp(-e.deltaY * 0.0016), 0.02, 40);
  const rect = el.vStage.getBoundingClientRect();
  const px = e.clientX - rect.left - rect.width / 2;
  const py = e.clientY - rect.top - rect.height / 2;
  viewer.tx = px - (px - viewer.tx) * (next / old);
  viewer.ty = py - (py - viewer.ty) * (next / old);
  viewer.scale = next;
  viewer.fit = false;
  applyViewerTransform();
  updateZoomLabel();
}, { passive: false });

el.vStage.addEventListener("dblclick", () => {
  if (viewer.mode !== "image") return;
  if (viewer.fit) {
    viewer.fit = false;
    viewer.scale = 1;
    viewer.tx = 0;
    viewer.ty = 0;
  } else {
    viewer.fit = true;
  }
  applyViewerTransform();
  updateZoomLabel();
});

$("#v-close").addEventListener("click", closeViewer);
$("#v-prev").addEventListener("click", () => viewerStep(-1));
$("#v-next").addEventListener("click", () => viewerStep(1));
$("#v-fit").addEventListener("click", () => { viewer.fit = true; applyViewerTransform(); updateZoomLabel(); });
$("#v-100").addEventListener("click", () => { viewer.fit = false; viewer.scale = 1; viewer.tx = 0; viewer.ty = 0; applyViewerTransform(); updateZoomLabel(); });
$("#v-rotate").addEventListener("click", () => { viewer.rot = (viewer.rot + 90) % 360; applyViewerTransform(); });
$("#v-info-btn").addEventListener("click", () => toggleInfoPanel());
$("#v-side-close").addEventListener("click", () => toggleInfoPanel(false));
$("#v-open-ext").addEventListener("click", () => {
  const item = viewer.tab && viewer.tab.view[viewer.index];
  if (item) call("AppSvc.OpenExternal", item.path).catch(showError);
});

el.vVideo.addEventListener("error", () => {
  if (!viewer.open || viewer.mode !== "video" || el.vVideo.hidden) return;
  const item = viewer.tab && viewer.tab.view[viewer.index];
  if (item && el.vVideo.dataset.item === item.id) {
    showUnsupported(item, "WebView2 无法播放这个视频（编解码器不受支持）。");
  }
});

// --------------------------------------------------------------- menu API

window.fv = {
  openPath: (p) => openPath(p),  menu: (action) => {
    const tab = activeTab();
    switch (action) {
      case "newtab": newTab(); break;
      case "openfolder": pickFolder(); break;
      case "closetab": closeTab(state.active); break;
      case "reveal": if (tab && tab.path) call("AppSvc.OpenFolder", tab.path).catch(showError); break;
      case "back": if (tab) histBack(tab); break;
      case "up": goParent(); break;
      case "nextfolder": goSibling(1); break;
      case "prevfolder": goSibling(-1); break;
      case "thumb+": changeThumb(24); break;
      case "thumb-": changeThumb(-24); break;
      case "sidebar": toggleSidebar(); break;
      case "recursive":
        state.settings.recursive = !state.settings.recursive;
        toast(state.settings.recursive ? "已开启递归模式" : "已关闭递归模式");
        persist();
        if (tab && tab.path) browse(tab, tab.path);
        break;
      case "hidden":
        state.settings.showHidden = !state.settings.showHidden;
        persist();
        if (tab && tab.path) browse(tab, tab.path);
        break;
      case "settings": settingsDialog(); break;
      case "help": helpDialog(); break;
      case "about": aboutDialog(); break;
      default: break;
    }
  },
};

// ------------------------------------------------------------------- start

async function init() {
  try {
    const s = await call("AppSvc.Startup");
    state.startup = s;
    state.settings = normalizeSettings(s.settings);
    applyTheme();
    el.sidebar.style.width = state.settings.sidebarWidth + "px";
    if (!state.settings.sidebar) el.sidebar.classList.add("hidden");
    document.documentElement.style.setProperty("--thumb", state.settings.thumbSize + "px");

    buildSidebar();

    const saved = (state.settings.tabs || []).filter((t) => t && t.path);
    const initial = saved.length ? saved : [{ path: s.startPath }];
    for (const t of initial) state.tabs.push(makeTab(t.path, t));
    let activateIndex = clamp(Number(state.settings.activeTab) || 0, 0, state.tabs.length - 1);
    if (s.cliPath) {
      const found = state.tabs.findIndex((t) => t.path && t.path.toLowerCase() === s.cliPath.toLowerCase());
      if (found >= 0) activateIndex = found;
      else {
        state.tabs.push(makeTab(s.cliPath));
        activateIndex = state.tabs.length - 1;
      }
    }
    await activateTab(activateIndex, true);
    const at = activeTab();
    if (at && !at.result && s.startPath) {
      const differs = (at.path || "").toLowerCase() !== s.startPath.toLowerCase();
      if (differs) await browse(at, s.startPath);
    }
    const pending = state.pendingOpen;
    state.pendingOpen = null;
    if (pending && activeTab()) await browse(activeTab(), pending.path, pending.opts);
  } catch (e) {
    console.error(e);
    emptyState("启动失败", String(e && (e.message || e.error) || e));
  }
}

window.addEventListener("error", (e) => {
  if (!e.message) return; // resource load failures are handled by their own callbacks
  window.__lastError = `${e.message} @ ${e.filename}:${e.lineno}:${e.colno}`;
  console.error(window.__lastError);
});
window.addEventListener("unhandledrejection", (e) => {
  window.__lastError = "unhandledrejection: " + String(e.reason && (e.reason.stack || e.reason.message || e.reason));
  console.error(e.reason);
});

init();
