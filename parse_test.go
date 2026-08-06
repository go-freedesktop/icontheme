// Copyright (c) the go-freedesktop/icontheme authors
//
// SPDX-License-Identifier: BSD-3-Clause

package icontheme

import "testing"

func TestParseIndexFieldErrors(t *testing.T) {
	cases := map[string]string{
		"size":      "[Icon Theme]\nDirectories=d\n[d]\nSize=x\n",
		"scale":     "[Icon Theme]\nDirectories=d\n[d]\nSize=16\nScale=x\n",
		"threshold": "[Icon Theme]\nDirectories=d\n[d]\nSize=16\nThreshold=x\n",
		"minsize":   "[Icon Theme]\nDirectories=d\n[d]\nSize=16\nMinSize=x\n",
		"maxsize":   "[Icon Theme]\nDirectories=d\n[d]\nSize=16\nMaxSize=x\n",
	}
	for name, content := range cases {
		if _, err := parseIndex("T", []byte(content)); err == nil {
			t.Fatalf("%s: expected parse error", name)
		}
	}
}

func TestParseIndexTypesAndDefaults(t *testing.T) {
	content := "" +
		"# a comment\n" +
		"\n" +
		"garbage line without equals\n" +
		"[Icon Theme]\n" +
		"Name = Sample\n" +
		"Inherits = A, , B\n" +
		"Directories = fixed, scal, thr, plain, weird\n" +
		"ScaledDirectories = scal, extra\n" +
		"\n" +
		"[fixed]\n" +
		"Size=16\n" +
		"Type=Fixed\n" +
		"Context=Applications\n" +
		"[scal]\n" +
		"Size=48\n" +
		"MinSize=8\n" +
		"MaxSize=256\n" +
		"Type=Scalable\n" +
		"[thr]\n" +
		"Size=32\n" +
		"Type=Threshold\n" +
		"[plain]\n" +
		"Size=24\n" +
		"[weird]\n" +
		"Size=64\n" +
		"Type=Bogus\n" +
		"[extra]\n" +
		"Size=128\n" +
		"Type=Fixed\n"

	idx, err := parseIndex("Sample", []byte(content))
	if err != nil {
		t.Fatal(err)
	}

	if got := idx.inherits; len(got) != 2 || got[0] != "A" || got[1] != "B" {
		t.Fatalf("inherits = %v", got)
	}

	// scal appears in both lists but is not duplicated; extra is appended.
	names := make([]string, len(idx.dirs))
	byName := map[string]directory{}
	for i, d := range idx.dirs {
		names[i] = d.name
		byName[d.name] = d
	}
	want := []string{"fixed", "scal", "thr", "plain", "weird", "extra"}
	if len(names) != len(want) {
		t.Fatalf("dir names = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("dir order = %v, want %v", names, want)
		}
	}

	if d := byName["fixed"]; d.typ != typeFixed || d.size != 16 || d.context != "Applications" {
		t.Fatalf("fixed = %+v", d)
	}
	if d := byName["scal"]; d.typ != typeScalable || d.minSize != 8 || d.maxSize != 256 {
		t.Fatalf("scal = %+v", d)
	}
	if d := byName["thr"]; d.typ != typeThreshold || d.threshold != defaultThreshold {
		t.Fatalf("thr = %+v", d)
	}
	// plain omits Type/MinSize/MaxSize: defaults are Threshold and Size.
	if d := byName["plain"]; d.typ != typeThreshold || d.minSize != 24 || d.maxSize != 24 || d.scale != 1 {
		t.Fatalf("plain = %+v", d)
	}
	// An unrecognised Type falls back to Threshold.
	if d := byName["weird"]; d.typ != typeThreshold {
		t.Fatalf("weird = %+v", d)
	}
}

func TestSplitHelpers(t *testing.T) {
	if _, _, ok := splitKeyValue("noequals"); ok {
		t.Fatal("expected split failure")
	}
	k, v, ok := splitKeyValue("k = v")
	if !ok || k != "k" || v != "v" {
		t.Fatalf("split = %q %q %v", k, v, ok)
	}
	if got := splitList(" a , , b ,"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("splitList = %v", got)
	}
}
