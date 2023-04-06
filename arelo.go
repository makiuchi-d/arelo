package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/fs"
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
	usage   = `Usage: arelo [OPTION]... -- COMMAND
Run the COMMAND and restart when a file matches the pattern has been modified.

Options:
`
	targets  = pflag.StringArrayP("target", "t", nil, "observation target `path` (default \"./\")")
	patterns = pflag.StringArrayP("pattern", "p", nil, "trigger pathname `glob` pattern (default \"**\")")
	ignores  = pflag.StringArrayP("ignore", "i", nil, "ignore pathname `glob` pattern")
	delay    = pflag.DurationP("delay", "d", time.Second, "`duration` to delay the restart of the command")
	restart  = pflag.BoolP("restart", "r", false, "restart the command on exit")
	sigopt   = pflag.StringP("signal", "s", "", "`signal` used to stop the command (default \"SIGTERM\")")
	verbose  = pflag.BoolP("verbose", "v", false, "verbose output")
	help     = pflag.BoolP("help", "h", false, "display this message")
	showver  = pflag.BoolP("version", "V", false, "display version")
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
	logVerbose("restart:  %v", *restart)

	if *help {
		fmt.Println("arelo version", versionstr())
		fmt.Fprintf(os.Stderr, usage)
		pflag.PrintDefaults()
		return
	}

	if len(cmd) == 0 {
		fmt.Fprintf(os.Stderr, "%s: COMMAND required.\n", os.Args[0])
		os.Exit(1)
	}
	if sig == nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", os.Args[0], sigstr)
		os.Exit(1)
	}

	modC, errC, err := watcher(*targets, *patterns, *ignores)
	if err != nil {
		log.Fatalf("[ARELO] wacher error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	reload := runner(ctx, &wg, cmd, *delay, sig.(syscall.Signal), *restart)

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
				logVerbose("modified: %q", name)
				reload <- name
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
				if event.Has(fsnotify.Create) {
					fi, err := os.Stat(name)
					if err != nil {
						// ignore stat errors (notfound, permission, etc.)
						log.Printf("[ARELO] watcher: %v", err)
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
		if strings.HasPrefix(t, "./") {
			m, err = doublestar.Match(p, t[2:])
			if err != nil {
				return false, xerrors.Errorf("match(%v, %v): %w", p, t[2:], err)
			}
			if m {
				return true, nil
			}
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
		logVerbose("watching target: %q", t)
		if err := w.Add(t); err != nil {
			return err
		}
	}
	return nil
}

func addDirRecursive(w *fsnotify.Watcher, fi fs.FileInfo, t string, patterns, ignores []string, ch chan<- string) error {
	logVerbose("watching target: %q", t)
	err := w.Add(t)
	if err != nil {
		return xerrors.Errorf("wacher add: %w", err)
	}
	des, err := os.ReadDir(t)
	if err != nil {
		return xerrors.Errorf("read dir: %w", err)
	}
	for _, de := range des {
		name := path.Join(t, de.Name())
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
		if de.IsDir() {
			fi, err := de.Info()
			if err != nil {
				return err
			}
			err = addDirRecursive(w, fi, name, patterns, ignores, ch)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

type bytesErr struct {
	bytes []byte
	err   error
}

// stdinReader bypasses stdin to child processes
//
// cmd.Wait() blocks until stdin.Read() returns.
// so stdinReader.Read() returns EOF when the child process exited.
type stdinReader struct {
	input    <-chan bytesErr
	chldDone <-chan struct{}
}

func (s *stdinReader) Read(b []byte) (int, error) {
	select {
	case be, ok := <-s.input:
		if !ok {
			return 0, io.EOF
		}
		return copy(b, be.bytes), be.err
	case <-s.chldDone:
		return 0, io.EOF
	}
}

func clearChBuf[T any](c <-chan T) {
	for {
		select {
		case <-c:
		default:
			return
		}
	}
}

func runner(ctx context.Context, wg *sync.WaitGroup, cmd []string, delay time.Duration, sig syscall.Signal, autorestart bool) chan<- string {
	reload := make(chan string)
	trigger := make(chan string)

	go func() {
		for name := range reload {
			// ignore restart when the trigger is not waiting
			select {
			case trigger <- name:
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

	stdinC := make(chan bytesErr, 1)
	go func() {
		b1 := make([]byte, 255)
		b2 := make([]byte, 255)
		for {
			n, err := os.Stdin.Read(b1)
			stdinC <- bytesErr{b1[:n], err}
			b1, b2 = b2, b1
		}
	}()

	chldDone := makeChildDoneChan()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			cmdctx, cancel := context.WithCancel(ctx)
			restart := make(chan struct{})
			done := make(chan struct{})

			go func() {
				log.Printf("[ARELO] start: %s", pcmd)
				clearChBuf(chldDone)
				stdin := &stdinReader{stdinC, chldDone}
				err := runCmd(cmdctx, cmd, sig, stdin)
				if err != nil {
					log.Printf("[ARELO] command error: %v", err)
				} else {
					log.Printf("[ARELO] command exit status 0")
				}
				if autorestart {
					close(restart)
				}

				close(done)
			}()

			select {
			case <-ctx.Done():
				cancel()
				<-done
				return
			case name := <-trigger:
				log.Printf("[ARELO] triggered: %q", name)
			case <-restart:
				logVerbose("auto restart")
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

	return reload
}

func runCmd(ctx context.Context, cmd []string, sig syscall.Signal, stdin *stdinReader) error {
	c := prepareCommand(cmd)
	c.Stdin = bufio.NewReader(stdin)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Start(); err != nil {
		return err
	}

	var cerr error
	done := make(chan struct{})
	go func() {
		cerr = waitCmd(c)
		close(done)
	}()

	select {
	case <-done:
		if cerr != nil {
			cerr = xerrors.Errorf("process exit: %w", cerr)
		}
		return cerr
	case <-ctx.Done():
		if err := killChilds(c, sig); err != nil {
			return xerrors.Errorf("kill childs: %w", err)
		}
	}

	select {
	case <-done:
	case <-time.NewTimer(waitForTerm).C:
		if err := killChilds(c, syscall.SIGKILL); err != nil {
			return xerrors.Errorf("kill childs (SIGKILL): %w", err)
		}
		<-done
	}

	if cerr != nil {
		return xerrors.Errorf("process canceled: %w", cerr)
	}
	return nil
}
