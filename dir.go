package main

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// ---------------------------------------------------------------- media types

var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".jpe": true, ".png": true, ".gif": true,
	".webp": true, ".bmp": true, ".tif": true, ".tiff": true, ".avif": true,
	".heic": true, ".heif": true, ".jxl": true, ".ico": true, ".jfif": true,
	// RAW formats: listed, but decoding falls back to the shell.
	".cr2": true, ".cr3": true, ".nef": true, ".arw": true, ".dng": true,
	".orf": true, ".rw2": true, ".raf": true, ".pef": true, ".srw": true,
}

var videoExts = map[string]bool{
	".mp4": true, ".m4v": true, ".mov": true, ".webm": true, ".mkv": true,
	".avi": true, ".wmv": true, ".flv": true, ".ts": true, ".m2ts": true,
	".mpg": true, ".mpeg": true, ".3gp": true, ".ogv": true,
}

// browserPlayable are the containers Chromium (WebView2) plays natively.
var browserPlayable = map[string]bool{
	".mp4": true, ".m4v": true, ".mov": true, ".webm": true, ".ogv": true,
	".m4a": true, ".mp3": true,
}

func classify(name string) (kind, ext string) {
	ext = strings.ToLower(filepath.Ext(name))
	switch {
	case imageExts[ext]:
		return "image", ext
	case videoExts[ext]:
		return "video", ext
	}
	return "other", ext
}

// ---------------------------------------------------------------------- model

// Item is one file or subfolder in a folder, as the grid sees it.
type Item struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Ext    string `json:"ext"`
	Size   int64  `json:"size"`
	Mtime  int64  `json:"mtime"`
	W      int    `json:"w"`
	H      int    `json:"h"`
	Orient int    `json:"orient,omitempty"`
	Video  bool   `json:"video"`
	Play   bool   `json:"play"`
}

// Store keeps every item the page may ask about by a short id, so that image
// URLs carry no paths and cannot escape the browsed folders.
type Store struct {
	mu    sync.RWMutex
	max   int
	byID  map[string]*Item
	order []string
}

var items = &Store{byID: map[string]*Item{}, max: 20000}

func shortID(path string) string {
	sum := md5.Sum([]byte(strings.ToLower(path)))
	return hex.EncodeToString(sum[:])[:14]
}

func (s *Store) Intern(path string) *Item {
	return s.intern(path, false)
}

func (s *Store) intern(path string, isDir bool) *Item {
	name := filepath.Base(path)
	kind, ext := classify(name)
	st, err := os.Stat(path)
	var size int64
	var mtime int64
	if err == nil {
		size = st.Size()
		mtime = st.ModTime().Unix()
		if st.IsDir() {
			kind, ext = "folder", ""
			// A directory's size is not useful for sorting, and stamping it
			// would make a folder look like a zero-byte file.
			size = 0
		}
	}
	if isDir {
		kind, ext = "folder", ""
	}
	id := shortID(path)
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.byID[id]; ok {
		updated := *old
		oldKind := updated.Kind
		updated.Name = name
		updated.Path = path
		updated.Kind = kind
		updated.Ext = ext
		updated.Video = kind == "video"
		updated.Play = kind == "video" && browserPlayable[ext]
		dimsChanged := updated.Mtime != mtime || updated.Size != size || oldKind != kind
		updated.Size = size
		updated.Mtime = mtime
		if dimsChanged {
			updated.W, updated.H, updated.Orient = 0, 0, 0
		}
		s.byID[id] = &updated
		out := updated
		return &out
	}
	it := &Item{
		ID: id, Name: name, Path: path, Kind: kind, Ext: ext,
		Size: size, Mtime: mtime, Video: kind == "video",
		Play: kind == "video" && browserPlayable[ext],
	}
	s.byID[id] = it
	s.order = append(s.order, id)
	// The page keeps ids only for things it may still display; bound the map
	// so a long session walking huge trees cannot grow without limit.
	if s.max > 0 && len(s.order) > s.max {
		drop := len(s.order) - s.max + s.max/10
		for _, old := range s.order[:drop] {
			delete(s.byID, old)
		}
		s.order = append([]string(nil), s.order[drop:]...)
	}
	out := *it
	return &out
}

func (s *Store) Get(id string) (*Item, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	it, ok := s.byID[id]
	if !ok {
		return nil, false
	}
	out := *it
	return &out, true
}

// ------------------------------------------------------------------ dimension

type dimKey struct {
	path  string
	mtime int64
}

var dimCache sync.Map // dimKey -> dimInfo

func forgetDims(path string) {
	dimCache.Range(func(k, _ any) bool {
		if key, ok := k.(dimKey); ok && strings.EqualFold(key.path, path) {
			dimCache.Delete(k)
		}
		return true
	})
}

type dimInfo struct {
	W, H   int
	Orient int
}

// fillDims reads image headers for the first budget items, concurrently.
func fillDims(list []*Item, budget int) {
	if budget <= 0 {
		return
	}
	type job struct{ it *Item }
	ch := make(chan *Item, len(list))
	for _, it := range list {
		if it.Kind != "image" || it.W > 0 || budget <= 0 {
			continue
		}
		budget--
		ch <- it
	}
	close(ch)

	workers := runtime.NumCPU()
	if workers > 12 {
		workers = 12
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range ch {
				d := imageDims(it.Path, it.Mtime)
				if d.W > 0 {
					it.W, it.H, it.Orient = d.W, d.H, d.Orient
				}
			}
		}()
	}
	wg.Wait()
}

func imageDims(path string, mtime int64) dimInfo {
	key := dimKey{path, mtime}
	if v, ok := dimCache.Load(key); ok {
		return v.(dimInfo)
	}
	d := probeImageDims(path)
	dimCache.Store(key, d)
	return d
}

// ---------------------------------------------------------------------- browse

// FS is the folder service the page calls.
type FS struct{}

// BrowseRequest is what the page asks for.
type BrowseRequest struct {
	Path       string `json:"path"`
	Recursive  bool   `json:"recursive"`
	Sort       string `json:"sort"`
	Desc       bool   `json:"desc"`
	ShowHidden bool   `json:"showHidden"`
	ShowOther  bool   `json:"showOther"`
	Limit      int    `json:"limit"`
}

// DirResult is a folder listing.
type DirResult struct {
	Path      string  `json:"path"`
	Name      string  `json:"name"`
	Parent    string  `json:"parent"`
	IsDrive   bool    `json:"isDrive"`
	Items     []*Item `json:"items"`
	Total     int     `json:"total"`
	Images    int     `json:"images"`
	Videos    int     `json:"videos"`
	Folders   int     `json:"folders"`
	Others    int     `json:"others"`
	Truncated bool    `json:"truncated"`
	ElapsedMS int64   `json:"elapsedMs"`
	Error     string  `json:"error,omitempty"`
}

// Browse lists a folder the way the grid shows it.
func (FS) Browse(req BrowseRequest) DirResult {
	started := time.Now()
	if req.Limit <= 0 {
		req.Limit = 4000
	}
	if req.Limit > 20000 {
		req.Limit = 20000
	}
	if req.Path == "" {
		return DirResult{Error: "无法打开此文件夹"}
	}
	path := filepath.Clean(req.Path)
	res := DirResult{Path: path, Name: displayName(path), IsDrive: isDriveRoot(path)}
	st, err := os.Stat(path)
	if err != nil || !st.IsDir() {
		res.Error = "无法打开此文件夹"
		return res
	}
	if parent := filepath.Dir(path); parent != path {
		res.Parent = parent
	}

	var list []*Item
	seen := map[string]bool{}

	var errLimitReached = errors.New("walk limit reached")

	add := func(p string, isDir bool) bool {
		if seen[p] {
			return true
		}
		if !req.ShowHidden && (isHidden(p) || strings.HasPrefix(filepath.Base(p), ".")) {
			return true
		}
		if !isDir {
			kind, _ := classify(p)
			if kind == "other" && !req.ShowOther {
				return true
			}
		}
		it := items.intern(p, isDir)
		if it.Kind == "other" && !req.ShowOther {
			return true
		}
		if len(list) > req.Limit {
			return false
		}
		seen[p] = true
		list = append(list, it)
		// Keep collecting one extra entry so a folder holding exactly Limit
		// entries is not reported as truncated.
		return len(list) <= req.Limit
	}

	if req.Recursive {
		// Recursive mode is a flat media wall. The direct subfolders are left
		// out on purpose: walking into them is a sidebar-tree gesture, and
		// showing them beside the flattened photos made the wall repeat
		// navigation instead of showing pictures.
		_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if p == path {
					return nil
				}
				if !req.ShowHidden && (isHidden(p) || strings.HasPrefix(d.Name(), ".")) {
					return filepath.SkipDir
				}
				baseDepth := strings.Count(path, string(os.PathSeparator))
				if strings.Count(p, string(os.PathSeparator))-baseDepth > 5 {
					return filepath.SkipDir
				}
				return nil
			}
			if !add(p, false) {
				return errLimitReached
			}
			return nil
		})
	} else {
		entries, err := os.ReadDir(path)
		if err != nil {
			res.Error = "无法读取此文件夹"
			return res
		}
		for _, e := range entries {
			if !add(filepath.Join(path, e.Name()), e.IsDir()) {
				break
			}
		}
	}

	if len(list) > req.Limit {
		list = list[:req.Limit]
		res.Truncated = true
	}
	sortItems(list, req.Sort, req.Desc)
	res.Images, res.Videos, res.Folders, res.Others = 0, 0, 0, 0
	for _, it := range list {
		switch it.Kind {
		case "image":
			res.Images++
		case "video":
			res.Videos++
		case "folder":
			res.Folders++
		default:
			res.Others++
		}
	}

	res.Items = list
	res.Total = len(list)
	// Read image headers for the first screenfuls so the layout settles fast.
	fillDims(list, 900)
	res.ElapsedMS = time.Since(started).Milliseconds()
	return res
}

func displayName(path string) string {
	clean := filepath.Clean(path)
	vol := filepath.VolumeName(clean)
	rest := strings.Trim(strings.TrimPrefix(clean, vol), string(os.PathSeparator))
	if rest == "" {
		return vol
	}
	return filepath.Base(clean)
}

func isDriveRoot(path string) bool {
	clean := filepath.Clean(path)
	vol := filepath.VolumeName(clean)
	return vol != "" && strings.Trim(strings.TrimPrefix(clean, vol), string(os.PathSeparator)) == ""
}

func sortItems(list []*Item, key string, desc bool) {
	less := func(a, b *Item) bool {
		switch key {
		case "date":
			if a.Mtime != b.Mtime {
				return a.Mtime < b.Mtime
			}
		case "size":
			if a.Size != b.Size {
				return a.Size < b.Size
			}
		case "type":
			if a.Kind != b.Kind {
				return kindRank(a.Kind) < kindRank(b.Kind)
			}
			if a.Ext != b.Ext {
				return a.Ext < b.Ext
			}
		}
		return naturalLess(a.Name, b.Name)
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		aFolder, bFolder := a.Kind == "folder", b.Kind == "folder"
		if aFolder != bFolder {
			// Directories stay above files whatever the sort direction:
			// they are navigation targets, not media results.
			return aFolder
		}
		if desc {
			return less(b, a)
		}
		return less(a, b)
	})
}

func kindRank(k string) int {
	switch k {
	case "folder":
		return -1
	case "image":
		return 0
	case "video":
		return 1
	}
	return 2
}

func naturalLess(a, b string) bool {
	la, lb := strings.ToLower(a), strings.ToLower(b)
	i, j := 0, 0
	for i < len(la) && j < len(lb) {
		if isDigit(la[i]) && isDigit(lb[j]) {
			si, sj := i, j
			for i < len(la) && isDigit(la[i]) {
				i++
			}
			for j < len(lb) && isDigit(lb[j]) {
				j++
			}
			na := strings.TrimLeft(la[si:i], "0")
			nb := strings.TrimLeft(lb[sj:j], "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		if la[i] != lb[j] {
			return la[i] < lb[j]
		}
		i++
		j++
	}
	if i == len(la) && j == len(lb) {
		// Leading zeros inside number runs can leave the two input lengths
		// different even though every compared character matched.
		return false
	}
	return i == len(la)
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// --------------------------------------------------------------- navigation

// TreeEntry is one folder in the sidebar tree.
type TreeEntry struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	HasMedia    bool   `json:"hasMedia"`
	HasChildren bool   `json:"hasChildren"`
}

// Place is a drive or well known folder.
type Place struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Icon string `json:"icon"`
}

// NavResult is the answer of a folder gesture.
type NavResult struct {
	Path  string `json:"path"`
	Found bool   `json:"found"`
}

type mediaCacheEntry struct {
	ok      bool
	hasDirs bool
	at      time.Time
}

var mediaCache sync.Map // path -> mediaCacheEntry

// folderInfo answers two questions with one directory scan: does this folder
// directly contain an image or video, and does it contain a visible child
// folder? The sidebar needs both for its tree. The answer is cached for a few
// seconds because sibling navigation asks the same question repeatedly while
// the user walks through a large tree.
func folderInfo(dir string, showHidden bool) (media, children bool) {
	key := strings.ToLower(filepath.Clean(dir))
	if showHidden {
		key += "|hidden"
	}
	if v, ok := mediaCache.Load(key); ok {
		e := v.(mediaCacheEntry)
		if time.Since(e.at) < 5*time.Second {
			return e.ok, e.hasDirs
		}
	}
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range entries {
			full := filepath.Join(dir, e.Name())
			if !showHidden && (isHidden(full) || strings.HasPrefix(e.Name(), ".")) {
				continue
			}
			if e.IsDir() {
				children = true
				if media {
					break
				}
				continue
			}
			kind, _ := classify(e.Name())
			if kind == "image" || kind == "video" {
				media = true
				if children {
					break
				}
			}
		}
	}
	mediaCache.Store(key, mediaCacheEntry{ok: media, hasDirs: children, at: time.Now()})
	return media, children
}

func hasMedia(dir string, showHidden bool) bool {
	media, _ := folderInfo(dir, showHidden)
	return media
}

func invalidateMedia(dir string) {
	key := strings.ToLower(filepath.Clean(dir))
	mediaCache.Delete(key)
	mediaCache.Delete(key + "|hidden")
}

// Tree lists the sub folders of path.
func (FS) Tree(path string, showHidden bool) []TreeEntry {
	path = filepath.Clean(path)
	entries, err := os.ReadDir(path)
	if err != nil {
		return []TreeEntry{}
	}
	out := []TreeEntry{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		full := filepath.Join(path, e.Name())
		if !showHidden && (isHidden(full) || strings.HasPrefix(e.Name(), ".")) {
			continue
		}
		media, hasChildren := folderInfo(full, showHidden)
		out = append(out, TreeEntry{
			Name:        e.Name(),
			Path:        full,
			HasMedia:    media,
			HasChildren: hasChildren,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return naturalLess(out[i].Name, out[j].Name) })
	return out
}

// Sibling walks the disk order from path by direction (1 next, -1 previous)
// to the nearest folder that holds images or videos.
func (FS) Sibling(path string, direction int, showHidden bool) NavResult {
	path = filepath.Clean(path)
	parent := filepath.Dir(path)
	if parent == path {
		return NavResult{Path: path, Found: true}
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return NavResult{}
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		full := filepath.Join(parent, e.Name())
		if !showHidden && (isHidden(full) || strings.HasPrefix(e.Name(), ".")) {
			continue
		}
		dirs = append(dirs, full)
	}
	sort.SliceStable(dirs, func(i, j int) bool { return naturalLess(filepath.Base(dirs[i]), filepath.Base(dirs[j])) })
	idx := -1
	for i, d := range dirs {
		if strings.EqualFold(d, path) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return NavResult{}
	}
	for i := idx + direction; i >= 0 && i < len(dirs); i += direction {
		if hasMedia(dirs[i], showHidden) {
			return NavResult{Path: dirs[i], Found: true}
		}
	}
	return NavResult{}
}

// Parent returns the folder above path.
func (FS) Parent(path string) NavResult {
	path = filepath.Clean(path)
	parent := filepath.Dir(path)
	if parent == path {
		return NavResult{Path: path, Found: false}
	}
	return NavResult{Path: parent, Found: true}
}

// Drives lists the mounted volumes.
func (FS) Drives() []Place {
	out := []Place{}
	for c := 'C'; c <= 'Z'; c++ {
		p := string(c) + ":\\"
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			out = append(out, Place{Name: string(c) + ":", Path: p, Icon: "drive"})
		}
	}
	return out
}

// Places lists the user's well known folders.
func (FS) Places() []Place {
	list := []struct {
		name string
		pn   mygo.PathName
		icon string
	}{
		{"用户目录", mygo.PathHome, "home"},
		{"桌面", mygo.PathDesktop, "desktop"},
		{"图片", mygo.PathPictures, "pictures"},
		{"下载", mygo.PathDownloads, "downloads"},
		{"文档", mygo.PathDocuments, "documents"},
		{"视频", mygo.PathVideos, "videos"},
	}
	out := []Place{}
	for _, it := range list {
		if p, err := mygo.App.Path(it.pn); err == nil && p != "" {
			out = append(out, Place{Name: it.name, Path: p, Icon: it.icon})
		}
	}
	return out
}

// CountMedia counts images and videos directly inside a folder.
func (FS) CountMedia(path string) int {
	entries, err := os.ReadDir(filepath.Clean(path))
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		kind, _ := classify(e.Name())
		if kind == "image" || kind == "video" {
			n++
		}
	}
	return n
}

// ExistsResult is the answer to FS.Exists.
type ExistsResult struct {
	OK   bool   `json:"ok"`
	Path string `json:"path"`
}

// Exists reports whether path is a reachable folder.
func (FS) Exists(path string) ExistsResult {
	clean := filepath.Clean(path)
	st, err := os.Stat(clean)
	if err != nil || !st.IsDir() {
		return ExistsResult{}
	}
	return ExistsResult{OK: true, Path: clean}
}
