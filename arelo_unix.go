//go:build unix

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"golang.org/x/xerrors"
)

func parseSignalOption(str string) (os.Signal, string) {
	switch strings.ToUpper(str) {
	case "1", "HUP", "SIGHUP", "SIG_HUP":
		return syscall.SIGHUP, "SIGHUP"
	case "2", "INT", "SIGINT", "SIG_INT":
		return syscall.SIGINT, "SIGINT"
	case "3", "QUIT", "SIGQUIT", "SIG_QUIT":
		return syscall.SIGQUIT, "SIGQUIT"
	case "9", "KILL", "SIGKILL", "SIG_KILL":
		return syscall.SIGKILL, "SIGKILL"
	case "10", "USR1", "SIGUSR1", "SIG_USR1":
		return syscall.SIGUSR1, "SIGUSR1"
	case "12", "USR2", "SIGUSR2", "SIG_USR2":
		return syscall.SIGUSR2, "SIGUSR2"
	case "15", "TERM", "SIGTERM", "SIG_TERM", "":
		return syscall.SIGTERM, "SIGTERM"
	case "28", "WINCH", "SIGWINCH", "SIG_WINCH":
		return syscall.SIGWINCH, "SIGWINCH"
	}

	return nil, fmt.Sprintf("unspported signal: %s", str)
}

var sigchldC chan os.Signal

func clearChBuf[T any](c <-chan T) {
	for {
		select {
		case <-c:
		default:
			return
		}
	}
}

func prepareCommand(cmd []string, withstdin bool) *exec.Cmd {
	if withstdin {
		// On UNIX like OS, termination of child process is notified by SIGCHLD.
		if sigchldC == nil {
			sigchldC = make(chan os.Signal, 1)
			signal.Notify(sigchldC, syscall.SIGCHLD)
		} else {
			clearChBuf(sigchldC)
		}
	}
	c := exec.Command(cmd[0], cmd[1:]...)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return c
}

// watchChild detects the termination of the child process by using SIGCHLD and the wait4 syscall.
func watchChild(ctx context.Context, c *exec.Cmd) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-sigchldC:
		}

		var wstatus syscall.WaitStatus
		var rusage syscall.Rusage
		pid, err := syscall.Wait4(c.Process.Pid, &wstatus, syscall.WNOHANG, &rusage)
		if errors.Is(err, syscall.ECHILD) || (pid == c.Process.Pid && wstatus.Exited()) {
			return nil
		}
		if err != nil {
			return xerrors.Errorf("syscall.Wait4: %w", err)
		}
	}
}

func killChilds(c *exec.Cmd, sig syscall.Signal) error {
	err := syscall.Kill(-c.Process.Pid, sig)
	if err == nil && sig != syscall.SIGKILL && sig != syscall.SIGCONT {
		// prosess can be stopped, so it must be start by SIGCONT.
		err = syscall.Kill(-c.Process.Pid, syscall.SIGCONT)
	}
	return err
}
