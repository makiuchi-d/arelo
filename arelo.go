package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/bmatcuk/doublestar"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/pflag"
	"golang.org/x/xerrors"
)

var (
	usage = `Usage: %s [OPTION]... -- COMMAND
Run the COMMAND when a file matching the pattern is modified.

Options:
`
	targets  = pflag.StringSliceP("target", "t", nil, "observation target `path`. (default \"./\")")
	patterns = pflag.StringSliceP("pattern", "p", nil, "trigger pathname `glob` pattern.")
	ignores  = pflag.StringSliceP("ignore", "i", nil, "ignore pathname `glob` pattern.")
	skip     = pflag.DurationP("skip", "s", time.Second, "`duration` to skip the trigger.")
	verbose  = pflag.BoolP("verbose", "v", false, "verbose output.")
	help     = pflag.BoolP("help", "h", false, "show this document.")
)

func main() {
	pflag.Parse()
	cmd := pflag.Args()
	if *targets == nil {
		*targets = []string{"./"}
	}
	logVerbose("command:  %q", cmd)
	logVerbose("targets:  %q", targets)
	logVerbose("patterns: %q", patterns)
	logVerbose("ignores:  %q", ignores)
	logVerbose("skip:     %v", skip)

	if *help {
		fmt.Printf(usage, os.Args[0])
		pflag.PrintDefaults()
		return
	}

	modC, errC, err := watcher()
	if err != nil {
		log.Fatalf("wacher error: %v", err)
	}
	for {
		stop := make(chan struct{})
		go func() {
			logVerbose("run command: %q", cmd)
			err := runCmd(cmd, stop)
			if err != nil {
				log.Printf("command error: %v", err)
			} else {
				logVerbose("command exit status 0")
			}
		}()

		select {
		case name, ok := <-modC:
			if !ok {
				log.Fatalf("wacher closed")
				return
			}
			logVerbose("detect modified: %v", name)
			close(stop)
			continue
		case err := <-errC:
			log.Fatalf("wacher error: %v", err)
			return
		}
	}
}

func logVerbose(fmt string, args ...interface{}) {
	if *verbose {
		log.Printf(fmt, args...)
	}
}

func watcher() (<-chan string, <-chan error, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, err
	}

	for _, t := range *targets {
		err := addTarget(w, t)
		if err != nil {
			return nil, nil, err
		}
	}

	modC := make(chan string)
	errC := make(chan error)

	go func() {
		defer close(modC)
		next := time.Now().Add(*skip)
		for {
			select {
			case event, ok := <-w.Events:
				if !ok {
					errC <- xerrors.Errorf("watcher.Events closed")
					return
				}

				name := event.Name

				if ignore, err := matchPatterns(name, *ignores); err != nil {
					errC <- xerrors.Errorf("ignore match error: %w", err)
					return
				} else if ignore {
					continue
				}

				if match, err := matchPatterns(name, *patterns); err != nil {
					errC <- xerrors.Errorf("pattern match error: %w", err)
					return
				} else if match && time.Now().After(next) {
					modC <- name
					next = time.Now().Add(*skip)
				}

				// add watcher if new directory.
				if event.Op == fsnotify.Create {
					fi, err := os.Stat(name)
					if err != nil {
						errC <- err
						return
					}
					if fi.IsDir() {
						err := addDirRecursive(w, fi, name, modC)
						if err != nil {
							errC <- err
							return
						}
					}
				}

			case err, ok := <-w.Errors:
				if !ok {
					errC <- xerrors.Errorf("watcher.Errors closed")
					return
				}
				errC <- xerrors.Errorf("wacher error: %w", err)
				return
			}
		}
	}()

	return modC, errC, nil
}

func matchPatterns(t string, pats []string) (bool, error) {
	for _, p := range pats {
		m, err := doublestar.Match(p, t)
		if err != nil {
			return false, err
		}
		if m {
			return true, nil
		}
	}
	return false, nil
}

func addTarget(w *fsnotify.Watcher, t string) error {
	t = path.Clean(t)
	fi, err := os.Stat(t)
	if err != nil {
		return xerrors.Errorf("stat: %w", err)
	}
	if fi.IsDir() {
		return addDirRecursive(w, fi, t, nil)
	}
	logVerbose("watching target: %q", t)
	return w.Add(t)
}

func addDirRecursive(w *fsnotify.Watcher, fi os.FileInfo, t string, ch chan<- string) error {
	logVerbose("watching target: %q", t)
	err := w.Add(t)
	if err != nil {
		return xerrors.Errorf("wacher add: %w", err)
	}
	fis, err := ioutil.ReadDir(t)
	if err != nil {
		return xerrors.Errorf("read dir: %w", err)
	}
	for _, fi := range fis {
		name := path.Join(t, fi.Name())
		if ignore, err := matchPatterns(name, *ignores); err != nil {
			return xerrors.Errorf("ignore match error: %w", err)
		} else if ignore {
			continue
		}
		if ch != nil {
			if match, err := matchPatterns(name, *patterns); err != nil {
				return xerrors.Errorf("pattern match error: %w", err)
			} else if match {
				ch <- name
			}
		}
		if fi.IsDir() {
			err := addDirRecursive(w, fi, name, ch)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func runCmd(cmd []string, stop <-chan struct{}) error {
	c := exec.Command(cmd[0], cmd[1:]...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Start(); err != nil {
		return err
	}
	done := make(chan error)
	go func() {
		done <- c.Wait()
	}()

	select {
	case <-stop:
		err := c.Process.Kill()
		if err != nil {
			return xerrors.Errorf("kill error: %w", err)
		}
		return xerrors.New("process killed")
	case err := <-done:
		if err != nil {
			err = xerrors.Errorf("process exit: %w", err)
		}
		return err
	}
}
