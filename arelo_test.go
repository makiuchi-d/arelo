package main

import (
	"os"
	"path"
	"testing"
	"time"
)

func TestWatcher(t *testing.T) {
	tmpdir := t.TempDir()

	dirs := []string{
		path.Join(tmpdir, "target"),
		path.Join(tmpdir, "target", "sub"),
		path.Join(tmpdir, "target", "ignore"),
		path.Join(tmpdir, "mv", "mvsub"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}

	targets := []string{tmpdir + "/target"}
	ignores := []string{"**/ignore"}
	patterns := []string{"**/file"}

	modC, errC, err := watcher(targets, patterns, ignores, 0)
	if err != nil {
		t.Fatalf("watcher: %v", err)
	}

	// move directory into the target to check the subdirectories are watched.
	if err := os.Rename(path.Join(tmpdir, "mv"), path.Join(tmpdir, "target", "mv")); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	tests := []struct {
		file   string
		detect bool
	}{
		{path.Join(tmpdir, "target", "file"), true},
		{path.Join(tmpdir, "target", "file2"), false},
		{path.Join(tmpdir, "file"), false},
		{path.Join(tmpdir, "target", "sub", "file"), true},
		{path.Join(tmpdir, "target", "ignore", "file"), false},
		{path.Join(tmpdir, "target", "mv", "file"), true},
		{path.Join(tmpdir, "target", "mv", "mvsub", "file"), true},
	}
	for _, test := range tests {
		<-time.After(time.Second / 5)
		clearChan(modC, errC)
		touchFile(test.file)
		select {
		case f := <-modC:
			if f != test.file {
				t.Fatalf("unexpected file modified: %q, wants %q", f, test.file)
			}
			if !test.detect {
				t.Fatalf("must not be detect: %q", f)
			}
		case e := <-errC:
			t.Fatalf("watcher error: %v", e)
		case <-time.After(time.Second / 5):
			if test.detect {
				t.Fatalf("must be detect: %q", test.file)
			}
		}
	}
}

func clearChan(c <-chan string, ce <-chan error) {
	for {
		select {
		case <-c:
		case <-ce:
		default:
			return
		}
	}
}

func touchFile(file string) {
	os.WriteFile(file, []byte("a"), 0644)
}

func TestMatchPatterns(t *testing.T) {
	tests := []struct {
		t, pat string
		wants  bool
	}{
		{"ab/cd/efg", "**/efg", true},
		{"ab/cd/efg", "*/efg", false},
		{"./abc.efg", "**/*.efg", true},
		{"./abc.efg", "*.efg", true},
		{"./.abc", "**/.*", true},
		{"./.abc", ".*", true},
	}

	for _, test := range tests {
		r, err := matchPatterns(test.t, []string{test.pat})
		if err != nil {
			t.Fatalf("matchPatterns(%v, {%v}): %v", test.t, test.pat, err)
		}
		if r != test.wants {
			t.Fatalf("matchPatterns(%v, {%v}) = %v wants %v", test.t, test.pat, r, test.wants)
		}
	}
}
