package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/pflag"
	"golang.org/x/xerrors"
)

const (
	waitForTerm = 5 * time.Second
)

var (
	version string
	usage   = `Usage: %s [OPTION]... -- COMMAND
Run the COMMAND and restart when a file matches the pattern has been modified.

Options:
`
	targets  = pflag.StringArrayP("target", "t", nil, "observation target `path`. (default \"./\")")
	patterns = pflag.StringArrayP("pattern", "p", nil, "trigger pathname `glob` pattern. (default \"**\")")
	ignores  = pflag.StringArrayP("ignore", "i", nil, "ignore pathname `glob` pattern.")
	delay    = pflag.DurationP("delay", "d", time.Second, "`duration` to delay the restart of the command.")
	sigopt   = pflag.StringP("signal", "s", "", "`signal` to stop the command. (default \"SIGTERM\")")
	verbose  = pflag.BoolP("verbose", "v", false, "verbose output.")
	help     = pflag.BoolP("help", "h", false, "show this document.")
	showver  = pflag.BoolP("version", "V", false, "show version.")
)

func main() {
	pflag.Parse()
	if *showver {
		fmt.Println("arelo version", versionstr())
		return
	}
	cmd := pflag.Args()
	if *targets == nil {
		*targets = []string{"./"}
	}
	if *patterns == nil {
		logVerbose("patterns nil")
		*patterns = []string{"**"}
	}
	sig, sigstr := parseSignalOption(*sigopt)
	logVerbose("command:  %q", cmd)
	logVerbose("targets:  %q", *targets)
	logVerbose("patterns: %q", *patterns)
	logVerbose("ignores:  %q", *ignores)
	logVerbose("delay:    %v", delay)
	logVerbose("signal:   %s", sigstr)

	if !*help {
		if len(cmd) == 0 {
			fmt.Fprintf(os.Stderr, "%s: COMMAND required.\n", os.Args[0])
			*help = true
		} else if sig == nil {
			fmt.Fprintf(os.Stderr, "%s: %s\n", os.Args[0], sigstr)
			*help = true
		}
	}
	if *help {
		fmt.Println("arelo version", versionstr())
		fmt.Fprintf(os.Stderr, usage, os.Args[0])
		pflag.PrintDefaults()
		return
	}

	modC, errC, err := watcher(*targets, *patterns, *ignores)
	if err != nil {
		log.Fatalf("[ARELO] wacher error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	restartC := runner(ctx, &wg, cmd, *delay, sig.(syscall.Signal))

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case name, ok := <-modC:
				if !ok {
					cancel()
					wg.Wait()
					log.Fatalf("[ARELO] wacher closed")
					return
				}
				log.Printf("[ARELO] modified: %q", name)
				restartC <- struct{}{}
			case err := <-errC:
				cancel()
				wg.Wait()
				log.Fatalf("[ARELO] wacher error: %v", err)
				return
			}
		}
	}()

	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	sig = <-s
	log.Printf("[ARELO] signal: %v", sig)
	cancel()
	wg.Wait()
}

func logVerbose(fmt string, args ...interface{}) {
	if *verbose {
		log.Printf("[ARELO] "+fmt, args...)
	}
}

func versionstr() string {
	if version != "" {
		return "v" + version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "(devel)"
	}
	return info.Main.Version
}

func watcher(targets, patterns, ignores []string) (<-chan string, <-chan error, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, err
	}

	if err := addTargets(w, targets, patterns, ignores); err != nil {
		return nil, nil, err
	}

	modC := make(chan string)
	errC := make(chan error)

	go func() {
		defer close(modC)
		for {
			select {
			case event, ok := <-w.Events:
				if !ok {
					errC <- xerrors.Errorf("watcher.Events closed")
					return
				}

				name := filepath.ToSlash(event.Name)

				if ignore, err := matchPatterns(name, ignores); err != nil {
					errC <- xerrors.Errorf("match ignores: %w", err)
					return
				} else if ignore {
					continue
				}

				if match, err := matchPatterns(name, patterns); err != nil {
					errC <- xerrors.Errorf("match patterns: %w", err)
					return
				} else if match {
					modC <- name
				}

				// add watcher if new directory.
				if event.Op&fsnotify.Create == fsnotify.Create {
					fi, err := os.Stat(name)
					if err != nil {
						// ignore stat errors (notfound, permission, etc.)
						log.Printf("watcher: %v", err)
					} else if fi.IsDir() {
						err := addDirRecursive(w, fi, name, patterns, ignores, modC)
						if err != nil {
							errC <- err
							return
						}
					}
				}

			case err, ok := <-w.Errors:
				errC <- xerrors.Errorf("watcher.Errors (%v): %w", ok, err)
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
			return false, xerrors.Errorf("match(%v, %v): %w", p, t, err)
		}
		if m {
			return true, nil
		}
	}
	return false, nil
}

func addTargets(w *fsnotify.Watcher, targets, patterns, ignores []string) error {
	for _, t := range targets {
		t = path.Clean(t)
		fi, err := os.Stat(t)
		if err != nil {
			return xerrors.Errorf("stat: %w", err)
		}
		if fi.IsDir() {
			if err := addDirRecursive(w, fi, t, patterns, ignores, nil); err != nil {
				return err
			}
		}
		logVerbose("[ARELO] watching target: %q", t)
		if err := w.Add(t); err != nil {
			return err
		}
	}
	return nil
}

func addDirRecursive(w *fsnotify.Watcher, fi os.FileInfo, t string, patterns, ignores []string, ch chan<- string) error {
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
		if ignore, err := matchPatterns(name, ignores); err != nil {
			return xerrors.Errorf("match ignores: %w", err)
		} else if ignore {
			continue
		}
		if ch != nil {
			if match, err := matchPatterns(name, patterns); err != nil {
				return xerrors.Errorf("match patterns: %w", err)
			} else if match {
				ch <- name
			}
		}
		if fi.IsDir() {
			err := addDirRecursive(w, fi, name, patterns, ignores, ch)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func runner(ctx context.Context, wg *sync.WaitGroup, cmd []string, delay time.Duration, sig syscall.Signal) chan<- struct{} {
	restart := make(chan struct{})
	trigger := make(chan struct{})

	go func() {
		for range restart {
			// ignore restart when the trigger is not waiting
			select {
			case trigger <- struct{}{}:
			default:
			}
		}
	}()

	var pcmd string // command string for display.
	for _, s := range cmd {
		if strings.ContainsAny(s, " \t\"'") {
			s = fmt.Sprintf("%q", s)
		}
		pcmd += " " + s
	}
	pcmd = pcmd[1:]

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			ctx, cancel := context.WithCancel(ctx)
			done := make(chan struct{})

			go func() {
				log.Printf("[ARELO] start: %s", pcmd)
				err := runCmd(ctx, cmd, sig)
				if err != nil {
					log.Printf("[ARELO] command error: %v", err)
				} else {
					log.Printf("[ARELO] command exit status 0")
				}
				close(done)
			}()

			select {
			case <-ctx.Done():
				cancel()
				<-done
				return
			case <-trigger:
			}

			logVerbose("wait %v", delay)
			t := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				cancel()
				<-done
				return
			case <-t.C:
			}
			cancel()
			<-done // wait process closed
		}
	}()

	return restart
}

func runCmd(ctx context.Context, cmd []string, sig syscall.Signal) error {
	c := prepareCommand(cmd)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Start(); err != nil {
		return err
	}

	var cerr error
	done := make(chan struct{})
	go func() {
		cerr = c.Wait()
		close(done)
	}()

	select {
	case <-done:
		if cerr != nil {
			cerr = xerrors.Errorf("process exit: %w", cerr)
		}
		return cerr
	case <-ctx.Done():
	}

	if err := killChilds(c, sig); err != nil {
		return xerrors.Errorf("kill childs: %w", err)
	}

	select {
	case <-time.NewTimer(waitForTerm).C:
		if err := killChilds(c, syscall.SIGKILL); err != nil {
			return xerrors.Errorf("kill childs (SIGKILL): %w", err)
		}
		<-done
	case <-done:
	}

	if cerr != nil {
		return xerrors.Errorf("process canceled: %w", cerr)
	}
	return nil
}
