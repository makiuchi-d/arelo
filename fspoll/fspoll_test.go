package fspoll_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/makiuchi-d/arelo/fspoll"
)

const (
	eventWaitTimeout = time.Second / 2
	pollingInterval  = time.Second / 10
)

func TestFsnotify(t *testing.T) {
	t.Parallel()
	newW := func() fspoll.Watcher {
		w, _ := fspoll.Wrap(fsnotify.NewWatcher())
		return w
	}

	testSingleFile(t, newW)
	testDirectory(t, newW)
}

func TestFspoll(t *testing.T) {
	t.Parallel()
	newW := func() fspoll.Watcher {
		return fspoll.New(pollingInterval)
	}

	testSingleFile(t, newW)
	testDirectory(t, newW)
}

func must(t *testing.T, err error) {
	if err != nil {
		_, f, l, _ := runtime.Caller(1)
		t.Fatalf("%v:%v: %v", f, l, err)
	}
}

func waitEvent(t *testing.T, w fspoll.Watcher, name string, op fspoll.Op) {
	timeout := time.After(eventWaitTimeout)
	for {
		select {
		case <-timeout:
			t.Fatalf("timeout: waiting %v for %q", op, name)

		case ev, ok := <-w.Events():
			t.Logf("event: %v", ev)
			if !ok {
				t.Fatal("watcher closed")
			}
			if ev.Op.Has(op) && ev.Name == name {
				return // ok
			}
		}
	}
}

func waitNoEvent(t *testing.T, w fspoll.Watcher) {
	timeout := time.After(eventWaitTimeout)
	select {
	case <-timeout:
		return // ok

	case ev, ok := <-w.Events():
		if !ok {
			t.Fatalf("watcher closed")
		}
		t.Fatalf("unexpected event: %v", ev)
	}
}

func testSingleFile[W fspoll.Watcher](t *testing.T, newW func() W) {
	t.Run("SingleFile", func(t *testing.T) {
		t.Parallel()
		w := newW()

		dir := t.TempDir()
		fname := filepath.Join(dir, "file")

		err := w.Add(fname)
		if err == nil {
			t.Fatal("Add no available file must be error")
		}

		t.Log("create")
		fp, err := os.Create(fname)
		must(t, err)
		defer fp.Close()

		must(t, w.Add(fname))

		t.Log("watchlist")
		l := w.WatchList()
		t.Log(l)
		exp := []string{fname}
		if !reflect.DeepEqual(l, exp) {
			t.Fatalf("WatchList: %v, wants %v", l, exp)
		}

		t.Log("write")
		fp.Write([]byte("a"))
		waitEvent(t, w, fname, fspoll.Write)

		t.Log("chmod")
		fp.Chmod(0700)
		waitEvent(t, w, fname, fspoll.Chmod)

		t.Log("remove")
		fp.Close()
		os.Remove(fname)
		waitEvent(t, w, fname, fspoll.Remove)

		t.Log("create after removed")
		fp2, err := os.Create(fname)
		must(t, err)
		defer fp2.Close()
		waitNoEvent(t, w)

		t.Log("call Remove after removed")
		err = w.Remove(fname)
		if !errors.Is(err, fspoll.ErrNonExistentWatch) {
			t.Fatalf("watcher.Remove must be ErrNonExistentWatch: err=%v", err)
		}

		t.Log("close watcher")
		w.Close()
		select {
		case _, ok := <-w.Events():
			if ok {
				t.Fatalf("Event channel is not closed")
			}
		case <-time.After(pollingInterval * 2):
			t.Fatalf("Events channel is not closed")
		}
		select {
		case _, ok := <-w.Errors():
			if ok {
				t.Fatalf("Errors channel is not closed")
			}
		case <-time.After(pollingInterval * 2):
			t.Fatalf("Errors channel is not closed")
		}
	})
}

func testDirectory[W fspoll.Watcher](t *testing.T, newW func() W) {
	t.Run("Directory", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		dname1 := filepath.Join(dir, "dir1")
		fname1 := filepath.Join(dname1, "file1")
		dname2 := filepath.Join(dname1, "dir2")
		fname2 := filepath.Join(dname2, "file2")

		w := newW()

		os.Mkdir(dname1, 0755)
		must(t, w.Add(dname1))

		t.Log("chmod basedir")
		os.Chmod(dir, 0700)
		waitNoEvent(t, w)

		t.Log("create file")
		fp1, err := os.Create(fname1)
		must(t, err)
		defer fp1.Close()
		waitEvent(t, w, fname1, fspoll.Create)

		t.Log("write file")
		fp1.Write([]byte("a"))
		waitEvent(t, w, fname1, fspoll.Write)

		t.Log("chmod file")
		os.Chmod(fname1, 0700)
		waitEvent(t, w, fname1, fspoll.Chmod)

		t.Log("create subdir")
		os.Mkdir(dname2, 0755)
		waitEvent(t, w, dname2, fspoll.Create)

		t.Log("create file in subdir")
		fp2, err := os.Create(fname2)
		must(t, err)
		defer fp2.Close()
		waitNoEvent(t, w)

		t.Log("remove file")
		os.Remove(fname1)
		waitEvent(t, w, fname1, fspoll.Remove)

		t.Log("chmod dir")
		os.Chmod(dname1, 0700)
		waitEvent(t, w, dname1, fspoll.Chmod)

		t.Log("remove from watcher")
		must(t, w.Remove(dname1))

		t.Log("write after removed")
		fp1.Write([]byte("a"))
		waitNoEvent(t, w)
	})
}
