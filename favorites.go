package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Favorites is a virtual folder, not a star on a folder: it collects images and
// videos from anywhere and shows them as one flat grid. Nothing is copied — each
// entry is a reference to the original file, so starring a photo on a NAS and
// one on a local disk puts both in the same view.
const (
	favoritesScheme = "favorites"
	favoritesPath   = favoritesScheme + "://"
	favoritesName   = "收藏夹"
)

// Favorite is one collected image or video.
type Favorite struct {
	Path string `json:"path"`
	// Name and Kind are stored so the grid can show a tile without reaching
	// the file, which matters for a share that is currently offline.
	Name string `json:"name,omitempty"`
	Kind string `json:"kind,omitempty"`
	Size int64  `json:"size,omitempty"`
	// Mtime keeps the thumbnail cache key stable across restarts.
	Mtime int64 `json:"mtime,omitempty"`
}

// isFavoritesPath reports whether a path is the virtual favorites folder.
func isFavoritesPath(p string) bool {
	trimmed := strings.TrimRight(p, "/")
	return strings.EqualFold(trimmed, favoritesScheme+":") || strings.EqualFold(trimmed, favoritesScheme+"://")
}

// favoriteKey is the identity used for comparisons: case-insensitive on the
// path, so C:\Pictures\a.jpg and c:\pictures\a.jpg are one favorite, and a
// virtual path keeps its scheme instead of being folded by filepath.Clean.
func favoriteKey(path string) string {
	return strings.ToLower(cleanDisplayPath(path))
}

func sameFavorite(f Favorite, path string) bool {
	return favoriteKey(f.Path) == favoriteKey(path)
}

// favoriteLabel is what a favorite row shows.
func favoriteLabel(f Favorite) string {
	if strings.TrimSpace(f.Name) != "" {
		return f.Name
	}
	return displayName(f.Path)
}

// toggleStarredPath stars or unstars a folder location for the sidebar. It is
// deliberately not the favorites folder: that one collects images and videos
// into a view of their own, while this remembers where to go back to. Keeping
// them in one list made the favorites folder count entries it did not show.
func (a *App) toggleStarredPath(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	path = cleanDisplayPath(path)
	if a.isStarredPath(path) {
		a.removeStarredPath(path)
		a.showStatusNotice("已取消收藏该路径")
		return
	}
	a.settings.StarredPaths = append(a.settings.StarredPaths, Favorite{
		Path: path,
		Name: displayName(path),
		Kind: "folder",
	})
	_ = saveSettings(a.settings)
	a.showStatusNotice("已收藏该路径")
	a.refresh()
}

// isStarredPath reports whether a folder location is starred.
func (a *App) isStarredPath(path string) bool {
	key := favoriteKey(path)
	for _, f := range a.settings.StarredPaths {
		if favoriteKey(f.Path) == key {
			return true
		}
	}
	return false
}

// removeStarredPath drops one starred location. It copies instead of filtering
// in place, for the same reason removeFavorite does.
func (a *App) removeStarredPath(path string) {
	key := favoriteKey(path)
	out := make([]Favorite, 0, len(a.settings.StarredPaths))
	for _, f := range a.settings.StarredPaths {
		if favoriteKey(f.Path) == key {
			continue
		}
		out = append(out, f)
	}
	a.settings.StarredPaths = out
	_ = saveSettings(a.settings)
	a.refresh()
}

// favoriteFromItem builds a favorite from a grid item.
func favoriteFromItem(it *Item) Favorite {
	return Favorite{
		Path:  it.Path,
		Name:  it.Name,
		Kind:  it.Kind,
		Size:  it.Size,
		Mtime: it.Mtime,
	}
}

// favoritesBrowse lists the virtual folder. Items are interned from their real
// paths, so thumbnails, the viewer and the info panel work exactly as they do
// in the folder the file lives in.
func favoritesBrowse(req BrowseRequest) DirResult {
	started := time.Now()
	res := DirResult{Path: favoritesPath, Name: favoritesName, IsDrive: true}
	saved := getSettings().Favorites
	list := make([]*Item, 0, len(saved))
	for _, f := range saved {
		if !isMediaKind(f.Kind) {
			continue
		}
		// A file that has since been deleted or renamed would only add a
		// broken tile; drop it silently and let the next save prune it.
		if !favoriteStillExists(f) {
			continue
		}
		it := items.Intern(f.Path)
		if it.Name == "" || it.Name == "." {
			it.Name = favoriteLabel(f)
		}
		if it.Kind != "image" && it.Kind != "video" {
			continue
		}
		if it.Mtime == 0 {
			it.Mtime = f.Mtime
		}
		list = append(list, it)
	}
	sortItems(list, req.Sort, req.Desc)
	for _, it := range list {
		switch it.Kind {
		case "image":
			res.Images++
		case "video":
			res.Videos++
		}
	}
	res.Items = list
	res.Total = len(list)
	fillDims(list, 160)
	res.ElapsedMS = time.Since(started).Milliseconds()
	return res
}

// favoriteStillExists checks a favorite cheaply. A remote path cannot be
// stat'ed without a connection, so it is assumed to exist and the grid shows a
// placeholder if the share has gone away.
func favoriteStillExists(f Favorite) bool {
	if isRemotePath(f.Path) {
		return true
	}
	_, err := os.Stat(f.Path)
	return err == nil
}

// pruneFavorites drops entries whose file is gone, so the virtual folder does
// not keep showing tiles that can never load.
func (a *App) pruneFavorites() {
	out := a.settings.Favorites[:0]
	changed := false
	for _, f := range a.settings.Favorites {
		if !isMediaKind(f.Kind) || !favoriteStillExists(f) {
			changed = true
			continue
		}
		out = append(out, f)
	}
	if !changed {
		return
	}
	a.settings.Favorites = out
	_ = saveSettings(a.settings)
}

// favoritesFromPaths upgrades the plain path list older builds wrote. Those
// entries were folders, which this folder no longer holds, so only the ones
// that are media files survive.
func favoritesFromPaths(paths []string) []Favorite {
	out := make([]Favorite, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		kind, _ := classify(p)
		if !isMediaKind(kind) {
			continue
		}
		out = append(out, Favorite{Path: p, Name: displayName(p), Kind: kind})
	}
	return out
}

// parseFavorites accepts both the current object form and the older string
// array, so a settings file from an earlier build still loads.
func parseFavorites(raw json.RawMessage) ([]Favorite, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var list []Favorite
	if err := json.Unmarshal(raw, &list); err == nil {
		// Drop anything that is not a media file: an older build stored
		// folders here, and this folder holds only images and videos.
		kept := make([]Favorite, 0, len(list))
		for _, f := range list {
			if f.Kind == "" {
				f.Kind, _ = classify(f.Path)
			}
			if !isMediaKind(f.Kind) {
				continue
			}
			kept = append(kept, f)
		}
		return kept, true
	}
	var paths []string
	if err := json.Unmarshal(raw, &paths); err == nil {
		return favoritesFromPaths(paths), true
	}
	return nil, false
}

// migrateFavorites rewrites a settings document whose favorites are still the
// old shape, so the file on disk matches what the app now reads.
func migrateFavorites(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	data = trimBOM(data)
	raw := rawField(data, "favorites")
	if len(raw) == 0 {
		return
	}
	list, ok := parseFavorites(raw)
	if !ok {
		return
	}
	encoded, err := json.Marshal(list)
	if err != nil {
		return
	}
	rest, err := withoutField(data, "favorites")
	if err != nil {
		return
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(rest, &fields) != nil {
		return
	}
	fields["favorites"] = encoded
	merged, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return
	}
	// Reuse the settings writer's atomic replace rather than a second copy of
	// the temp-file dance.
	f, err := os.CreateTemp(filepath.Dir(path), ".settings-*.tmp")
	if err != nil {
		return
	}
	tmp := f.Name()
	if _, err := f.Write(merged); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
	}
}

func trimBOM(data []byte) []byte {
	if len(data) >= 3 && data[0] == 0xef && data[1] == 0xbb && data[2] == 0xbf {
		return data[3:]
	}
	return data
}

// rawField pulls one field out of a JSON document without decoding the rest,
// so a value with a changed shape cannot fail the whole settings load.
func rawField(data []byte, name string) json.RawMessage {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return nil
	}
	return fields[name]
}

// withoutField returns the document with one key removed, so a value whose
// shape changed can be decoded by hand without failing the rest.
func withoutField(data []byte, name string) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	delete(fields, name)
	return json.Marshal(fields)
}

// sortFavorites keeps the stored order deterministic after a removal.
func sortFavorites(list []Favorite) {
	sort.SliceStable(list, func(i, j int) bool {
		return strings.ToLower(list[i].Path) < strings.ToLower(list[j].Path)
	})
}

// toggleViewerFavorite collects or drops whatever the viewer is showing.
func (a *App) toggleViewerFavorite(it *Item) {
	if it == nil || !isMediaKind(it.Kind) {
		return
	}
	a.toggleFavoritePath(it.Path)
}
