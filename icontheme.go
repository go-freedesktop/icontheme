// Copyright (c) the go-freedesktop/icontheme authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package icontheme is a pure-Go (CGO_ENABLED=0) implementation of the
// freedesktop.org Icon Theme Specification lookup algorithm.
//
// It resolves an icon name (such as the value of a .desktop file's Icon= key)
// to a concrete file path on disk at a requested pixel size and scale,
// following the specification exactly: it parses index.theme files, honours
// theme inheritance (with an implicit fall back to the hicolor base theme),
// and applies the DirectoryMatchesSize / DirectorySizeDistance rules to pick
// the best directory before falling back to unthemed pixmaps and, finally, a
// generic name-truncation fallback.
//
// The package returns a path only; rasterising the file (PNG/XPM/SVG) is the
// caller's responsibility. It never rasterises, decodes or otherwise inspects
// the pixels, so it stays dependency-light and fast.
//
// Specification: https://specifications.freedesktop.org/icon-theme/latest/
package icontheme

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/adrg/xdg"
)

// HicolorTheme is the name of the base theme every icon theme implicitly
// inherits from, as mandated by the specification.
const HicolorTheme = "hicolor"

// extensions is the ordered list of file extensions probed for each icon,
// matching the preference order recommended by the specification.
var extensions = []string{"png", "svg", "xpm"}

// dirType is the sizing model of an icon-theme directory.
type dirType int

const (
	// typeThreshold is the specification default when a directory omits Type.
	typeThreshold dirType = iota
	typeFixed
	typeScalable
)

// directory is a single entry from an index.theme Directories list, with its
// resolved sizing metadata.
type directory struct {
	name      string
	size      int
	scale     int
	minSize   int
	maxSize   int
	threshold int
	context   string
	typ       dirType
}

// index is a parsed index.theme file for one theme.
type index struct {
	name     string
	inherits []string
	dirs     []directory
}

// Theme resolves icon names for a chosen icon theme. It parses and caches the
// index.theme of the theme and every theme it inherits from, and memoises
// individual lookups. A Theme is safe for concurrent use by multiple
// goroutines.
type Theme struct {
	name     string
	baseDirs []string

	mu      sync.Mutex
	indexes map[string]*index // theme name -> parsed index; nil value = absent
	cache   map[lookupKey]string
}

// lookupKey identifies a memoised lookup result.
type lookupKey struct {
	name  string
	size  int
	scale int
}

// New returns a Theme for the named icon theme using the standard base
// directories derived from the XDG base-directory specification, plus
// $HOME/.icons and /usr/share/pixmaps.
func New(name string) *Theme {
	return NewWithBaseDirs(name, DefaultBaseDirs())
}

// NewWithBaseDirs returns a Theme for the named icon theme that searches the
// supplied base directories in order. It is the injection seam used for
// testing and for callers that manage their own icon search path.
func NewWithBaseDirs(name string, baseDirs []string) *Theme {
	return &Theme{
		name:     name,
		baseDirs: baseDirs,
		indexes:  make(map[string]*index),
		cache:    make(map[lookupKey]string),
	}
}

// DefaultBaseDirs returns the ordered list of icon base directories per the
// specification: $HOME/.icons, $XDG_DATA_HOME/icons, each $XDG_DATA_DIRS/icons,
// and finally /usr/share/pixmaps.
func DefaultBaseDirs() []string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".icons"))
	}
	dirs = append(dirs, filepath.Join(xdg.DataHome, "icons"))
	for _, d := range xdg.DataDirs {
		dirs = append(dirs, filepath.Join(d, "icons"))
	}
	dirs = append(dirs, "/usr/share/pixmaps")
	return dirs
}

// Lookup resolves icon name at the requested size (in device-independent
// pixels) and scale, returning the path to the best matching file. It searches
// the theme's inheritance chain, then hicolor, then unthemed pixmaps, then a
// generic fallback that strips trailing "-" separated components. It returns
// ErrNotFound if no file matches.
func (t *Theme) Lookup(name string, size, scale int) (string, error) {
	if scale < 1 {
		scale = 1
	}
	key := lookupKey{name: name, size: size, scale: scale}

	t.mu.Lock()
	defer t.mu.Unlock()

	if path, ok := t.cache[key]; ok {
		if path == "" {
			return "", ErrNotFound
		}
		return path, nil
	}

	path := t.resolve(name, size, scale)
	t.cache[key] = path
	if path == "" {
		return "", ErrNotFound
	}
	return path, nil
}

// FindIcon resolves the first of the candidate names that can be found,
// searching them in order. It is a convenience for a .desktop Icon= value
// followed by generic fallbacks. It returns ErrNotFound if none match.
func (t *Theme) FindIcon(names []string, size, scale int) (string, error) {
	for _, name := range names {
		if path, err := t.Lookup(name, size, scale); err == nil {
			return path, nil
		}
	}
	return "", ErrNotFound
}

// resolve performs the full FindIcon algorithm without locking or caching.
func (t *Theme) resolve(name string, size, scale int) string {
	if path := t.findIconInChain(name, size, scale); path != "" {
		return path
	}
	if path := t.lookupFallbackIcon(name); path != "" {
		return path
	}
	return t.lookupGenericIcon(name, size, scale)
}

// findIconInChain walks the theme's inheritance chain (deduplicated and
// cycle-safe), then hicolor, applying LookupIcon at each theme.
func (t *Theme) findIconInChain(name string, size, scale int) string {
	visited := make(map[string]bool)
	for _, themeName := range t.chain(visited) {
		if path := t.lookupIcon(name, size, scale, themeName); path != "" {
			return path
		}
	}
	return ""
}

// chain returns the ordered, deduplicated list of theme names to search: the
// configured theme and its transitive Inherits, followed by hicolor.
func (t *Theme) chain(visited map[string]bool) []string {
	var order []string
	t.walk(t.name, visited, &order)
	if !visited[HicolorTheme] {
		t.walk(HicolorTheme, visited, &order)
	}
	return order
}

// walk appends themeName and its parents to order in depth-first order.
func (t *Theme) walk(themeName string, visited map[string]bool, order *[]string) {
	if visited[themeName] {
		return
	}
	visited[themeName] = true
	*order = append(*order, themeName)
	idx := t.loadIndex(themeName)
	if idx == nil {
		return
	}
	for _, parent := range idx.inherits {
		t.walk(parent, visited, order)
	}
}

// lookupIcon implements the specification's LookupIcon for a single theme: an
// exact size-match pass, then a closest-by-distance pass.
func (t *Theme) lookupIcon(name string, size, scale int, themeName string) string {
	idx := t.loadIndex(themeName)
	if idx == nil {
		return ""
	}

	for _, dir := range idx.dirs {
		if !directoryMatchesSize(dir, size, scale) {
			continue
		}
		if path := t.probe(themeName, dir.name, name); path != "" {
			return path
		}
	}

	minDistance := -1
	closest := ""
	for _, dir := range idx.dirs {
		path := t.probe(themeName, dir.name, name)
		if path == "" {
			continue
		}
		dist := directorySizeDistance(dir, size, scale)
		if minDistance < 0 || dist < minDistance {
			minDistance = dist
			closest = path
		}
	}
	return closest
}

// probe returns the first existing file for name in a theme subdirectory,
// trying each known extension, or "" if none exists.
func (t *Theme) probe(themeName, subdir, name string) string {
	for _, base := range t.baseDirs {
		for _, ext := range extensions {
			path := filepath.Join(base, themeName, subdir, name+"."+ext)
			if fileExists(path) {
				return path
			}
		}
	}
	return ""
}

// lookupFallbackIcon implements LookupFallbackIcon: unthemed files sitting
// directly in a base directory (for example /usr/share/pixmaps).
func (t *Theme) lookupFallbackIcon(name string) string {
	for _, base := range t.baseDirs {
		for _, ext := range extensions {
			path := filepath.Join(base, name+"."+ext)
			if fileExists(path) {
				return path
			}
		}
	}
	return ""
}

// lookupGenericIcon implements the generic-icon fallback: repeatedly strip the
// trailing "-" separated component and retry the themed and unthemed lookups.
// For example "input-mouse-usb" degrades to "input-mouse" then "input".
func (t *Theme) lookupGenericIcon(name string, size, scale int) string {
	for {
		cut := lastDash(name)
		if cut < 0 {
			return ""
		}
		name = name[:cut]
		if path := t.findIconInChain(name, size, scale); path != "" {
			return path
		}
		if path := t.lookupFallbackIcon(name); path != "" {
			return path
		}
	}
}

// loadIndex returns the parsed, cached index.theme for themeName, or nil if the
// theme has no readable index in any base directory.
func (t *Theme) loadIndex(themeName string) *index {
	if idx, ok := t.indexes[themeName]; ok {
		return idx
	}
	var idx *index
	for _, base := range t.baseDirs {
		path := filepath.Join(base, themeName, "index.theme")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		parsed, err := parseIndex(themeName, data)
		if err != nil {
			continue
		}
		idx = parsed
		break
	}
	t.indexes[themeName] = idx
	return idx
}

// lastDash returns the index of the last '-' in s, or -1 if there is none.
func lastDash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '-' {
			return i
		}
	}
	return -1
}

// fileExists reports whether path names an existing regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// directoryMatchesSize implements the specification's DirectoryMatchesSize.
func directoryMatchesSize(d directory, size, scale int) bool {
	if d.scale != scale {
		return false
	}
	switch d.typ {
	case typeFixed:
		return d.size == size
	case typeScalable:
		return d.minSize <= size && size <= d.maxSize
	default: // typeThreshold
		return d.size-d.threshold <= size && size <= d.size+d.threshold
	}
}

// directorySizeDistance implements the specification's DirectorySizeDistance,
// operating in scaled pixels.
func directorySizeDistance(d directory, size, scale int) int {
	want := size * scale
	switch d.typ {
	case typeFixed:
		return abs(d.size*d.scale - want)
	case typeScalable:
		if want < d.minSize*d.scale {
			return d.minSize*d.scale - want
		}
		if want > d.maxSize*d.scale {
			return want - d.maxSize*d.scale
		}
		return 0
	default: // typeThreshold
		if want < (d.size-d.threshold)*d.scale {
			return (d.size-d.threshold)*d.scale - want
		}
		if want > (d.size+d.threshold)*d.scale {
			return want - (d.size+d.threshold)*d.scale
		}
		return 0
	}
}

// abs returns the absolute value of n.
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
