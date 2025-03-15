package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/pflag"
	"golang.org/x/xerrors"

	"github.com/makiuchi-d/arelo/fspoll"
)

const (
	waitForTerm = 5 * time.Second
)

var (
	version string
	usage   = `Usage: arelo [OPTION]... -- COMMAND
Run the COMMAND and restart when a file matches the pattern has been modified.

Options:`
	targets  = pflag.StringArrayP("target", "t", nil, "observation target `path` (default \"./\")")
	patterns = pflag.StringArrayP("pattern", "p", nil, "trigger pathname `glob` pattern (default \"**\")")
	ignores  = pflag.StringArrayP("ignore", "i", nil, "ignore pathname `glob` pattern")
	delay    = pflag.DurationP("delay", "d", time.Second, "`duration` to delay the restart of the command")
	restart  = pflag.BoolP("restart", "r", false, "restart the command on exit")
	sigopt   = pflag.StringP("signal", "s", "", "`signal` used to stop the command (default \"SIGTERM\")")
	nostdin  = pflag.BoolP("no-stdin", "n", false, "do not forward stdin to the command")
	verbose  = pflag.BoolP("verbose", "v", false, "verbose output")
	help     = pflag.BoolP("help", "h", false, "display this message")
	showver  = pflag.BoolP("version", "V", false, "display version")
	filters  = pflag.StringArrayP("filter", "f", nil, "filter file system `event` (CREATE|WRITE|REMOVE|RENAME|CHMOD)")
	polling  = pflag.Duration("polling", 0, "poll files at given `interval` instead of using fsnotify")
)

func main() {
	pflag.Parse()
	if *help {
		fmt.Println("arelo version", versionstr())
		fmt.Println(usage)
		pflag.PrintDefaults()
		return
	}
	if *showver {
		fmt.Println("arelo version", versionstr())
		return
	}
	cmd := pflag.Args()
	if *targets == nil {
		*targets = []string{"./"}
	}
	if *patterns == nil {
		*patterns = []string{"**"}
	}
	*patterns = removeCurDirPrefix(*patterns)
	*ignores = removeCurDirPrefix(*ignores)
	sig, sigstr := parseSignalOption(*sigopt)
	filtOp, err := parseFilters(*filters)
	if err != nil {
		log.Fatalf("[ARELO] %v", err)
	}
	logVerbose("command:  %q", cmd)
	logVerbose("targets:  %q", *targets)
	logVerbose("patterns: %q", *patterns)
	logVerbose("ignores:  %q", *ignores)
	logVerbose("filter:   %v", filtOp)
	logVerbose("delay:    %v", delay)
	logVerbose("signal:   %s", sigstr)
	logVerbose("restart:  %v", *restart)
	logVerbose("no-stdin: %v", *nostdin)
	if *polling != 0 {
		logVerbose("polling:  true (%v)", *polling)
	} else {
		logVerbose("polling:  false")
	}

	if len(cmd) == 0 {
		fmt.Fprintf(os.Stderr, "%s: COMMAND required.\n", os.Args[0])
		os.Exit(1)
	}
	if sig == nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", os.Args[0], sigstr)
		os.Exit(1)
	}

	modC, errC, err := watcher(*targets, *patterns, *ignores, filtOp, *polling)
	if err != nil {
		log.Fatalf("[ARELO] wacher error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	reload := runner(ctx, &wg, cmd, *delay, sig.(syscall.Signal), *restart, *nostdin)

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

func removeCurDirPrefix(arr []string) []string {
	for i, s := range arr {
		if strings.HasPrefix(s, "./") {
			arr[i] = s[2:]
		}
	}
	return arr
}

func parseFilters(filters []string) (fsnotify.Op, error) {
	var op fsnotify.Op
	for _, f := range filters {
		switch strings.ToUpper(f) {
		case "CREATE":
			op |= fsnotify.Create
		case "WRITE":
			op |= fsnotify.Write
		case "REMOVE":
			op |= fsnotify.Remove
		case "RENAME":
			op |= fsnotify.Rename
		case "CHMOD":
			op |= fsnotify.Chmod
		default:
			return 0, xerrors.Errorf("invalid filter event: %s", f)
		}
	}
	return op, nil
}

func newWatcher(interval time.Duration) (fspoll.Watcher, error) {
	if interval == 0 {
		return fspoll.Wrap(fsnotify.NewWatcher())
	}
	return fspoll.New(interval), nil
}

func watcher(targets, patterns, ignores []string, filtOp fsnotify.Op, interval time.Duration) (<-chan string, <-chan error, error) {
	w, err := newWatcher(interval)
	if err != nil {
		return nil, nil, err
	}
	if err := addTargets(w, targets, patterns, ignores); err != nil {
		return nil, nil, err
	}

	modC := make(chan string)
	errC := make(chan error)
	watchOp := ^filtOp

	go func() {
		defer close(modC)
		for {
			select {
			case event, ok := <-w.Events():
				if !ok {
					errC <- xerrors.Errorf("watcher.Events closed")
					return
				}

				name := filepath.ToSlash(event.Name)
				logVerbose("event: %v %q", event.Op, name)

				if ignore, err := matchPatterns(name, ignores); err != nil {
					errC <- xerrors.Errorf("match ignores: %w", err)
					return
				} else if ignore {
					continue
				}

				if event.Has(watchOp) {
					if match, err := matchPatterns(name, patterns); err != nil {
						errC <- xerrors.Errorf("match patterns: %w", err)
						return
					} else if match {
						modC <- name
					}
				}

				// add watcher if new directory.
				if event.Has(fsnotify.Create) {
					fi, err := os.Stat(name)
					if err != nil {
						// ignore stat errors (notfound, permission, etc.)
						log.Printf("[ARELO] watcher: %v", err)
					} else if fi.IsDir() {
						err := addDirRecursive(w, name, patterns, ignores, modC)
						if err != nil {
							errC <- err
							return
						}
					}
				}

			case err, ok := <-w.Errors():
				errC <- xerrors.Errorf("watcher.Errors (%v): %w", ok, err)
				return
			}
		}
	}()

	return modC, errC, nil
}

func matchPatterns(t string, pats []string) (bool, error) {
	if strings.HasPrefix(t, "./") {
		t = t[2:]
	}
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

func addTargets(w fspoll.Watcher, targets, patterns, ignores []string) error {
	for _, t := range targets {
		t = path.Clean(t)
		fi, err := os.Stat(t)
		if err != nil {
			return xerrors.Errorf("stat: %w", err)
		}
		if fi.IsDir() {
			return addDirRecursive(w, t, patterns, ignores, nil)
		}
		logVerbose("watching target: %q", t)
		if err := w.Add(t); err != nil {
			return err
		}
	}
	return nil
}

func addDirRecursive(w fspoll.Watcher, t string, patterns, ignores []string, ch chan<- string) error {
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
			err = addDirRecursive(w, name, patterns, ignores, ch)
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
// see also: watchChild()
type stdinReader struct {
	input <-chan bytesErr
	done  <-chan struct{}
}

func (s *stdinReader) Read(b []byte) (int, error) {
	select {
	case be, ok := <-s.input:
		if !ok {
			return 0, io.EOF
		}
		return copy(b, be.bytes), be.err
	case <-s.done:
		return 0, io.EOF
	}
}

func runner(ctx context.Context, wg *sync.WaitGroup, cmd []string, delay time.Duration, sig syscall.Signal, autorestart, nostdin bool) chan<- string {
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
		if strings.ContainsFunc(s, unicode.IsSpace) {
			s = strconv.Quote(s)
		}
		pcmd += " " + s
	}
	pcmd = pcmd[1:]

	var stdinC chan bytesErr
	if !nostdin {
		stdinC = make(chan bytesErr)
		go func() {
			b1 := make([]byte, 255)
			b2 := make([]byte, 255)
			for {
				n, err := os.Stdin.Read(b1)
				stdinC <- bytesErr{b1[:n], err}
				b1, b2 = b2, b1
			}
		}()
	}

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
				err := runCmd(cmdctx, cmd, sig, stdinC)
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
			select {
			case <-ctx.Done():
				cancel()
				<-done
				return
			case <-time.After(delay):
			}
			cancel()
			<-done // wait process closed
		}
	}()

	return reload
}

func runCmd(ctx context.Context, cmd []string, sig syscall.Signal, stdinC <-chan bytesErr) error {
	withStdin := stdinC != nil
	c := prepareCommand(cmd, withStdin)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	childctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if withStdin {
		c.Stdin = bufio.NewReader(&stdinReader{
			input: stdinC,
			done:  childctx.Done(),
		})
	}
	if err := c.Start(); err != nil {
		return err
	}

	var werrC chan error
	if withStdin {
		werrC = make(chan error, 1)
		go func() {
			err := watchChild(ctx, c)
			cancel()
			if err != nil {
				werrC <- xerrors.Errorf("watchChild: %w", err)
			}
		}()
	}

	var cerr error
	done := make(chan struct{})
	go func() {
		cerr = c.Wait()
		close(done)
	}()

	select {
	case <-done:
		return cerr
	case err := <-werrC:
		log.Printf("[ARELO] %v", err)
		// kill childs
	case <-ctx.Done():
		// kill childs
	}

	if err := killChilds(c, sig); err != nil {
		return xerrors.Errorf("kill childs: %w", err)
	}

	select {
	case <-done:
	case <-time.After(waitForTerm):
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
