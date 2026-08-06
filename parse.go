// Copyright (c) the go-freedesktop/icontheme authors
//
// SPDX-License-Identifier: BSD-3-Clause

package icontheme

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrNotFound is returned by Lookup and FindIcon when no matching icon file can
// be located anywhere in the search path.
var ErrNotFound = errors.New("icontheme: icon not found")

// defaultThreshold is the specification default for a Threshold directory when
// the Threshold key is absent.
const defaultThreshold = 2

// parseIndex parses the bytes of an index.theme file for the theme with the
// given directory name. It returns an error if a numeric field is malformed.
func parseIndex(themeName string, data []byte) (*index, error) {
	idx := &index{name: themeName}

	// First pass: read the [Icon Theme] section for Inherits and the list of
	// directory names, and collect the raw key/value pairs of every other
	// section keyed by section name.
	var (
		section  string
		dirNames []string
		scaled   []string
		sections = map[string]map[string]string{}
	)

	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || line[0] == '#' {
			continue
		}
		if line[0] == '[' && line[len(line)-1] == ']' {
			section = line[1 : len(line)-1]
			continue
		}
		key, value, ok := splitKeyValue(line)
		if !ok {
			continue
		}
		if section == "Icon Theme" {
			switch key {
			case "Inherits":
				idx.inherits = splitList(value)
			case "Directories":
				dirNames = splitList(value)
			case "ScaledDirectories":
				scaled = splitList(value)
			}
			continue
		}
		if _, ok := sections[section]; !ok {
			sections[section] = map[string]string{}
		}
		sections[section][key] = value
	}

	// A theme may list directories only under ScaledDirectories; merge the two
	// lists, preserving order and avoiding duplicates.
	for _, name := range scaled {
		if !contains(dirNames, name) {
			dirNames = append(dirNames, name)
		}
	}

	for _, name := range dirNames {
		dir, err := parseDirectory(name, sections[name])
		if err != nil {
			return nil, err
		}
		idx.dirs = append(idx.dirs, dir)
	}
	return idx, nil
}

// parseDirectory builds a directory from its section's key/value pairs,
// applying the specification defaults for absent keys.
func parseDirectory(name string, fields map[string]string) (directory, error) {
	d := directory{
		name:      name,
		scale:     1,
		threshold: defaultThreshold,
		typ:       typeThreshold,
	}

	size, err := intField(name, fields, "Size", 0)
	if err != nil {
		return directory{}, err
	}
	d.size = size

	if scale, err := intField(name, fields, "Scale", 1); err != nil {
		return directory{}, err
	} else {
		d.scale = scale
	}

	if threshold, err := intField(name, fields, "Threshold", defaultThreshold); err != nil {
		return directory{}, err
	} else {
		d.threshold = threshold
	}

	// MinSize and MaxSize default to Size when absent.
	if minSize, err := intField(name, fields, "MinSize", size); err != nil {
		return directory{}, err
	} else {
		d.minSize = minSize
	}

	if maxSize, err := intField(name, fields, "MaxSize", size); err != nil {
		return directory{}, err
	} else {
		d.maxSize = maxSize
	}

	d.context = fields["Context"]

	switch strings.ToLower(fields["Type"]) {
	case "fixed":
		d.typ = typeFixed
	case "scalable":
		d.typ = typeScalable
	case "threshold", "":
		d.typ = typeThreshold
	default:
		d.typ = typeThreshold
	}

	return d, nil
}

// intField returns the integer value of key in fields, or def if the key is
// absent. It returns an error if the value is present but not a valid integer.
func intField(section string, fields map[string]string, key string, def int) (int, error) {
	value, ok := fields[key]
	if !ok {
		return def, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("icontheme: section %q: invalid %s %q: %w", section, key, value, err)
	}
	return n, nil
}

// splitKeyValue splits "key = value" on the first '='.
func splitKeyValue(line string) (key, value string, ok bool) {
	eq := strings.IndexByte(line, '=')
	if eq < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:eq]), strings.TrimSpace(line[eq+1:]), true
}

// splitList splits a comma-separated value into trimmed, non-empty items.
func splitList(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// contains reports whether s is present in list.
func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
