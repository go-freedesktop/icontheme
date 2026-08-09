// Copyright (c) the go-freedesktop/icontheme authors
//
// SPDX-License-Identifier: BSD-3-Clause

package icontheme

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// benchSizes and benchContexts describe a realistic, Adwaita-scale directory
// matrix for the benchmark theme tree.
var (
	benchSizes    = []int{16, 22, 24, 32, 48, 64, 96, 128, 256}
	benchContexts = []string{"actions", "apps", "devices", "places", "status", "mimetypes"}
)

// buildBenchTree writes a representative multi-theme icon tree under a temporary
// base directory and returns the base dirs to search. The layout mirrors a real
// desktop: a top theme "Main" that inherits "Mid" that inherits the implicit
// hicolor base, each with a full size x context x scale directory matrix
// populated with icon files. It returns the base dirs plus the name of the top
// theme.
func buildBenchTree(tb testing.TB) []string {
	tb.Helper()
	base := tb.TempDir()

	// A handful of icon names present in every populated directory.
	icons := []string{"text-editor", "folder", "user-home", "edit-copy", "system-run"}

	writeDir := func(theme, subdir string, ext string, names []string) {
		dir := filepath.Join(base, theme, subdir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			tb.Fatal(err)
		}
		for _, n := range names {
			if err := os.WriteFile(filepath.Join(dir, n+"."+ext), nil, 0o644); err != nil {
				tb.Fatal(err)
			}
		}
	}

	// writeIndex assembles an index.theme with a full directory matrix.
	writeTheme := func(theme string, inherits string, populate []string) {
		var dirs []string
		body := "[Icon Theme]\nName=" + theme + "\n"
		if inherits != "" {
			body += "Inherits=" + inherits + "\n"
		}
		type ent struct{ name, section string }
		var ents []ent
		for _, sz := range benchSizes {
			for _, ctx := range benchContexts {
				for _, scale := range []int{1, 2} {
					name := fmt.Sprintf("%dx%d/%s", sz, sz, ctx)
					suffix := ""
					if scale == 2 {
						name += "@2x"
						suffix = fmt.Sprintf("Scale=2\n")
					}
					dirs = append(dirs, name)
					ents = append(ents, ent{name, fmt.Sprintf("[%s]\nSize=%d\nContext=%s\nType=Fixed\n%s", name, sz, ctx, suffix)})
					writeDir(theme, name, "png", populate)
				}
			}
			// A scalable directory per context.
		}
		for _, ctx := range benchContexts {
			name := "scalable/" + ctx
			dirs = append(dirs, name)
			ents = append(ents, ent{name, fmt.Sprintf("[%s]\nSize=48\nMinSize=8\nMaxSize=512\nContext=%s\nType=Scalable\n", name, ctx)})
			writeDir(theme, name, "svg", populate)
		}
		body += "Directories=" + join(dirs) + "\n\n"
		for _, e := range ents {
			body += e.section + "\n"
		}
		if err := os.WriteFile(filepath.Join(base, theme, "index.theme"), []byte(body), 0o644); err != nil {
			tb.Fatal(err)
		}
	}

	// hicolor holds a couple of names NOT present in Main/Mid so that resolving
	// them exercises the full inheritance fall-through.
	writeTheme("hicolor", "", append([]string{"web-browser", "application-default-icon"}, icons...))
	writeTheme("Mid", "hicolor", icons)
	writeTheme("Main", "Mid", icons)

	// An unthemed pixmap for the fallback path.
	if err := os.WriteFile(filepath.Join(base, "legacy-app.png"), nil, 0o644); err != nil {
		tb.Fatal(err)
	}

	return []string{base}
}

// join concatenates dir names with commas.
func join(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}

// warm forces every theme index in the chain to be parsed and cached, then
// clears only the per-lookup memo so resolve-cost benchmarks measure the
// algorithm rather than one-off index parsing.
func warm(t *Theme) {
	_, _ = t.Lookup("text-editor", 32, 1)
	t.mu.Lock()
	t.cache = map[lookupKey]string{}
	t.mu.Unlock()
}

func BenchmarkLookupHitCached(b *testing.B) {
	t := NewWithBaseDirs("Main", buildBenchTree(b))
	warm(t)
	_, _ = t.Lookup("text-editor", 32, 1) // prime the cache entry
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = t.Lookup("text-editor", 32, 1)
	}
}

func BenchmarkLookupHitUncached(b *testing.B) {
	t := NewWithBaseDirs("Main", buildBenchTree(b))
	warm(t)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t.mu.Lock()
		delete(t.cache, lookupKey{"text-editor", 32, 1})
		t.mu.Unlock()
		_, _ = t.Lookup("text-editor", 32, 1)
	}
}

func BenchmarkLookupInheritedFallback(b *testing.B) {
	t := NewWithBaseDirs("Main", buildBenchTree(b))
	warm(t)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t.mu.Lock()
		delete(t.cache, lookupKey{"web-browser", 32, 1})
		t.mu.Unlock()
		_, _ = t.Lookup("web-browser", 32, 1) // only in hicolor
	}
}

func BenchmarkLookupMiss(b *testing.B) {
	t := NewWithBaseDirs("Main", buildBenchTree(b))
	warm(t)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t.mu.Lock()
		delete(t.cache, lookupKey{"no-such-icon-anywhere", 32, 1})
		t.mu.Unlock()
		_, _ = t.Lookup("no-such-icon-anywhere", 32, 1)
	}
}

func BenchmarkLookupAcrossSizes(b *testing.B) {
	t := NewWithBaseDirs("Main", buildBenchTree(b))
	warm(t)
	sizes := benchSizes
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sz := sizes[i%len(sizes)]
		t.mu.Lock()
		delete(t.cache, lookupKey{"folder", sz, 1})
		t.mu.Unlock()
		_, _ = t.Lookup("folder", sz, 1)
	}
}

func BenchmarkFindIconList(b *testing.B) {
	t := NewWithBaseDirs("Main", buildBenchTree(b))
	warm(t)
	names := []string{"nonexistent-primary", "another-missing", "text-editor"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = t.FindIcon(names, 48, 1)
	}
}

func BenchmarkLookupHitCachedParallel(b *testing.B) {
	t := NewWithBaseDirs("Main", buildBenchTree(b))
	warm(t)
	_, _ = t.Lookup("text-editor", 32, 1)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = t.Lookup("text-editor", 32, 1)
		}
	})
}
