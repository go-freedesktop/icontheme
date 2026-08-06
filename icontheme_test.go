// Copyright (c) the go-freedesktop/icontheme authors
//
// SPDX-License-Identifier: BSD-3-Clause

package icontheme

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/adrg/xdg"
)

// testBaseDirs returns the fixture search path: the synthetic theme tree
// followed by an unthemed pixmaps directory.
func testBaseDirs() []string {
	return []string{
		filepath.Join("testdata", "icons"),
		filepath.Join("testdata", "pixmaps"),
	}
}

func newCustom() *Theme { return NewWithBaseDirs("Custom", testBaseDirs()) }

// wantPath asserts that a lookup succeeded and resolved to the expected file.
func wantPath(t *testing.T, got string, err error, want string) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != filepath.FromSlash(want) {
		t.Fatalf("path = %q, want %q", got, filepath.FromSlash(want))
	}
}

func TestLookupExactSize(t *testing.T) {
	th := newCustom()
	got, err := th.Lookup("text-editor", 16, 1)
	wantPath(t, got, err, "testdata/icons/Custom/16x16/apps/text-editor.png")
}

func TestLookupScalableExactMatch(t *testing.T) {
	// Size 20 matches only the scalable directory (8..512) exactly.
	th := newCustom()
	got, err := th.Lookup("text-editor", 20, 1)
	wantPath(t, got, err, "testdata/icons/Custom/scalable/apps/text-editor.svg")
}

func TestLookupScaleDirectory(t *testing.T) {
	th := newCustom()
	got, err := th.Lookup("text-editor", 32, 2)
	wantPath(t, got, err, "testdata/icons/Custom/32x32@2x/apps/text-editor.png")
}

func TestLookupClosestPrefersSmaller(t *testing.T) {
	// app-fixed exists only at 16 (Fixed) and 32 (Threshold). No directory
	// matches size 22 exactly, so the closest by distance wins: |16-22|=6 vs
	// (30-22)=8, so the 16x16 file is chosen.
	th := newCustom()
	got, err := th.Lookup("app-fixed", 22, 1)
	wantPath(t, got, err, "testdata/icons/Custom/16x16/apps/app-fixed.png")
}

func TestLookupClosestPrefersLarger(t *testing.T) {
	// At size 28 the threshold directory is closer: |16-28|=12 vs (30-28)=2.
	th := newCustom()
	got, err := th.Lookup("app-fixed", 28, 1)
	wantPath(t, got, err, "testdata/icons/Custom/32x32/apps/app-fixed.png")
}

func TestLookupClosestThresholdAbove(t *testing.T) {
	// At size 40 the threshold distance uses the above-max branch: 40-34=6.
	th := newCustom()
	got, err := th.Lookup("app-fixed", 40, 1)
	wantPath(t, got, err, "testdata/icons/Custom/32x32/apps/app-fixed.png")
}

func TestLookupInheritanceFallback(t *testing.T) {
	th := newCustom()
	got, err := th.Lookup("web-browser", 16, 1)
	wantPath(t, got, err, "testdata/icons/Parent/16x16/apps/web-browser.png")
}

func TestLookupHicolorFallback(t *testing.T) {
	th := newCustom()
	got, err := th.Lookup("folder", 16, 1)
	wantPath(t, got, err, "testdata/icons/hicolor/16x16/apps/folder.png")
}

func TestLookupUnthemedPixmapFallback(t *testing.T) {
	th := newCustom()
	got, err := th.Lookup("legacy", 16, 1)
	wantPath(t, got, err, "testdata/pixmaps/legacy.png")
}

func TestLookupGenericThemedFallback(t *testing.T) {
	// Truncating "text-editor-x" yields "text-editor", found in the theme.
	th := newCustom()
	got, err := th.Lookup("text-editor-x", 16, 1)
	wantPath(t, got, err, "testdata/icons/Custom/16x16/apps/text-editor.png")
}

func TestLookupGenericUnthemedFallback(t *testing.T) {
	// Truncating "legacy-x" yields "legacy", found in pixmaps.
	th := newCustom()
	got, err := th.Lookup("legacy-x", 16, 1)
	wantPath(t, got, err, "testdata/pixmaps/legacy.png")
}

func TestLookupNotFoundNoDash(t *testing.T) {
	th := newCustom()
	_, err := th.Lookup("missing", 16, 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestLookupNotFoundGenericExhausted(t *testing.T) {
	th := newCustom()
	_, err := th.Lookup("zz-yy", 16, 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestLookupScaleClampedToOne(t *testing.T) {
	// scale 0 is clamped to 1, matching the size-16 fixed directory.
	th := newCustom()
	got, err := th.Lookup("text-editor", 16, 0)
	wantPath(t, got, err, "testdata/icons/Custom/16x16/apps/text-editor.png")
}

func TestLookupCacheHitAndMiss(t *testing.T) {
	th := newCustom()

	// Prime and re-hit a successful lookup.
	first, err := th.Lookup("text-editor", 16, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := th.Lookup("text-editor", 16, 1)
	if err != nil || first != second {
		t.Fatalf("cache hit mismatch: %q %q %v", first, second, err)
	}

	// Prime and re-hit a negative lookup (cached empty path).
	if _, err := th.Lookup("missing", 16, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if _, err := th.Lookup("missing", 16, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want cached ErrNotFound, got %v", err)
	}
}

func TestFindIcon(t *testing.T) {
	th := newCustom()
	got, err := th.FindIcon([]string{"nope", "also-nope", "folder"}, 16, 1)
	wantPath(t, got, err, "testdata/icons/hicolor/16x16/apps/folder.png")

	if _, err := th.FindIcon([]string{"nope", "still-nope"}, 16, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestLonelyThemeReachesHicolorBranch(t *testing.T) {
	// A theme with no index still gets an implicit hicolor fall back, so folder
	// resolves even though "Lonely" has no index.theme.
	th := NewWithBaseDirs("Lonely", testBaseDirs())
	got, err := th.Lookup("folder", 16, 1)
	wantPath(t, got, err, "testdata/icons/hicolor/16x16/apps/folder.png")
}

func TestCyclicInheritanceTerminates(t *testing.T) {
	// Cycle1 -> Cycle2 -> Cycle1 must not loop forever; folder still resolves
	// via the implicit hicolor fall back.
	th := NewWithBaseDirs("Cycle1", testBaseDirs())
	got, err := th.Lookup("folder", 16, 1)
	wantPath(t, got, err, "testdata/icons/hicolor/16x16/apps/folder.png")
}

func TestBrokenIndexTreatedAsAbsent(t *testing.T) {
	// The Broken theme has a malformed Size; it is skipped, and the implicit
	// hicolor fall back still resolves folder.
	th := NewWithBaseDirs("Broken", testBaseDirs())
	got, err := th.Lookup("folder", 16, 1)
	wantPath(t, got, err, "testdata/icons/hicolor/16x16/apps/folder.png")
}

func TestNewUsesDefaultBaseDirs(t *testing.T) {
	th := New("hicolor")
	if len(th.baseDirs) == 0 {
		t.Fatal("New produced no base dirs")
	}
}

func TestDefaultBaseDirs(t *testing.T) {
	// Force the XDG-derived slice to be non-empty so the data-dirs loop runs.
	origHome, origDirs := xdg.DataHome, xdg.DataDirs
	defer func() { xdg.DataHome, xdg.DataDirs = origHome, origDirs }()
	xdg.DataHome = "/x/data"
	xdg.DataDirs = []string{"/x/share"}

	dirs := DefaultBaseDirs()
	if !contains(dirs, "/usr/share/pixmaps") {
		t.Fatalf("expected pixmaps in %v", dirs)
	}
	if !contains(dirs, filepath.Join("/x/data", "icons")) {
		t.Fatalf("expected data-home icons in %v", dirs)
	}
	if !contains(dirs, filepath.Join("/x/share", "icons")) {
		t.Fatalf("expected data-dir icons in %v", dirs)
	}
}

func TestFileExists(t *testing.T) {
	if !fileExists(filepath.Join("testdata", "pixmaps", "legacy.png")) {
		t.Fatal("legacy.png should exist")
	}
	if fileExists(filepath.Join("testdata", "does-not-exist")) {
		t.Fatal("missing file reported as existing")
	}
	if fileExists("testdata") {
		t.Fatal("directory reported as a regular file")
	}
}

func TestAbsAndLastDash(t *testing.T) {
	if abs(-3) != 3 || abs(4) != 4 {
		t.Fatal("abs")
	}
	if lastDash("a-b") != 1 || lastDash("abc") != -1 {
		t.Fatal("lastDash")
	}
}

func TestDirectoryMatchesSize(t *testing.T) {
	fixed := directory{scale: 1, size: 16, typ: typeFixed}
	scal := directory{scale: 1, minSize: 8, maxSize: 32, typ: typeScalable}
	thr := directory{scale: 1, size: 24, threshold: 2, typ: typeThreshold}

	cases := []struct {
		dir         directory
		size, scale int
		want        bool
	}{
		{fixed, 16, 1, true},
		{fixed, 17, 1, false},
		{fixed, 16, 2, false}, // scale mismatch
		{scal, 8, 1, true},
		{scal, 33, 1, false},
		{thr, 24, 1, true},
		{thr, 27, 1, false},
	}
	for i, c := range cases {
		if got := directoryMatchesSize(c.dir, c.size, c.scale); got != c.want {
			t.Fatalf("case %d: got %v want %v", i, got, c.want)
		}
	}
}

func TestDirectorySizeDistance(t *testing.T) {
	fixed := directory{scale: 2, size: 16, typ: typeFixed}
	scal := directory{scale: 1, minSize: 8, maxSize: 32, typ: typeScalable}
	thr := directory{scale: 1, size: 24, threshold: 2, typ: typeThreshold}

	cases := []struct {
		dir         directory
		size, scale int
		want        int
	}{
		{fixed, 16, 1, abs(16*2 - 16)}, // 16
		{scal, 4, 1, 8 - 4},            // below min
		{scal, 40, 1, 40 - 32},         // above max
		{scal, 16, 1, 0},               // within
		{thr, 10, 1, (24 - 2) - 10},    // below
		{thr, 40, 1, 40 - (24 + 2)},    // above
		{thr, 24, 1, 0},                // within
	}
	for i, c := range cases {
		if got := directorySizeDistance(c.dir, c.size, c.scale); got != c.want {
			t.Fatalf("case %d: got %d want %d", i, got, c.want)
		}
	}
}
