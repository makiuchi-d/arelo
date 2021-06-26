package main

import (
	"os"
	"testing"
	"time"
)

func TestWatcher(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "arelo-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	dirs := []string{
		tmpdir + "/target",
		tmpdir + "/target/sub",
		tmpdir + "/target/ignore",
		tmpdir + "/mv/mvsub",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}

	targets := []string{tmpdir + "/target"}
	ignores := []string{"**/ignore"}
	patterns := []string{"**/file"}

	modC, errC, err := watcher(targets, patterns, ignores, time.Second/10)
	if err != nil {
		t.Fatalf("watcher: %v", err)
	}

	// move directory into the target to check the subdirectories are watched.
	if err := os.Rename(tmpdir+"/mv", tmpdir+"/target/mv"); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	tests := []struct {
		file   string
		detect bool
	}{
		{tmpdir + "/target/file", true},
		{tmpdir + "/target/file2", false},
		{tmpdir + "/file", false},
		{tmpdir + "/target/sub/file", true},
		{tmpdir + "/target/ignore/file", false},
		{tmpdir + "/target/mv/file", true},
		{tmpdir + "/target/mv/mvsub/file", true},
	}
	for _, test := range tests {
		clearChan(modC, errC)
		<-time.NewTimer(time.Second / 5).C
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
		case <-time.NewTimer(time.Second / 5).C:
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
