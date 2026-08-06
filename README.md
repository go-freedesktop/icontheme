# icontheme

[![CI](https://github.com/go-freedesktop/icontheme/actions/workflows/ci.yml/badge.svg)](https://github.com/go-freedesktop/icontheme/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/go-freedesktop/icontheme.svg)](https://pkg.go.dev/github.com/go-freedesktop/icontheme)
[![License: BSD-3-Clause](https://img.shields.io/badge/License-BSD--3--Clause-blue.svg)](LICENSE)

Pure-Go (`CGO_ENABLED=0`) implementation of the freedesktop.org
[Icon Theme Specification](https://specifications.freedesktop.org/icon-theme/latest/)
lookup algorithm. It resolves an icon **name** (such as the `Icon=` value of a
`.desktop` entry) to a concrete file **path** on disk at a requested pixel size
and scale.

To the best of our knowledge this is the first pure-Go implementation of the
full spec lookup; the mature references are the Rust
[`oknozor/freedesktop-icons`](https://github.com/oknozor/freedesktop-icons) and
the D [`FreeSlave/icontheme`](https://github.com/FreeSlave/icontheme).

## Features

- Parses `index.theme`: `Inherits`, `Directories`/`ScaledDirectories`, and each
  directory's `Size`, `Scale`, `Context`, `Type` (`Fixed`/`Scalable`/`Threshold`),
  `MinSize`, `MaxSize`, `Threshold`, with spec defaults for absent keys.
- Full `FindIcon`/`LookupIcon` chain: exact directory-size match, then closest by
  `DirectorySizeDistance`, walked across the theme's inheritance chain, then the
  implicit **hicolor** base theme, then unthemed pixmaps, then a generic
  name-truncation fallback.
- Standard search path via [`github.com/adrg/xdg`](https://github.com/adrg/xdg):
  `$HOME/.icons`, `$XDG_DATA_HOME/icons`, each `$XDG_DATA_DIRS/icons`, and
  `/usr/share/pixmaps`.
- `@2x` (and any `Scale`) directories via the spec's scale rules.
- Extension preference `png` → `svg` → `xpm`.
- In-memory caches of parsed indexes and per-`(name,size,scale)` results; safe
  for concurrent use.

## Scope

This package returns a **path only**. Rasterising the file (PNG/XPM decode or
SVG render) is the caller's job — in the wasmdesk stack that is done by
go-widgets / go-opentype. It does not read pixels, so it stays fast and
dependency-light.

## Install

```sh
go get github.com/go-freedesktop/icontheme
```

## Quickstart

```go
package main

import (
	"fmt"

	"github.com/go-freedesktop/icontheme"
)

func main() {
	theme := icontheme.New("Adwaita") // falls back through Inherits, then hicolor

	// Resolve a single name at 48px, scale 1.
	path, err := theme.Lookup("text-editor", 48, 1)
	if err != nil {
		fmt.Println("not found:", err)
		return
	}
	fmt.Println(path) // e.g. /usr/share/icons/Adwaita/48x48/apps/text-editor.png

	// Try a list of candidate names (Icon= value plus fallbacks), HiDPI scale 2.
	path, err = theme.FindIcon([]string{"org.example.App", "application-x-executable"}, 24, 2)
	fmt.Println(path, err)
}
```

## API

- `New(name string) *Theme` — theme using the standard XDG base directories.
- `NewWithBaseDirs(name string, baseDirs []string) *Theme` — inject a custom search path.
- `DefaultBaseDirs() []string` — the standard ordered base directories.
- `(*Theme) Lookup(name string, size, scale int) (string, error)` — resolve one name.
- `(*Theme) FindIcon(names []string, size, scale int) (string, error)` — first name that resolves.
- `ErrNotFound` — returned when nothing matches.

## License

BSD-3-Clause. See [LICENSE](LICENSE).
