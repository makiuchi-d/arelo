//go:build unix

package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
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

// makeChildDoneChan returns a chan that notifies the child process has exited.
//
// On UNIX like OS, it is notified by SIGCHLD.
func makeChildDoneChan() <-chan struct{} {
	c := make(chan struct{}, 1)
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGCHLD)
		for {
			<-sig
			select {
			case c <- struct{}{}:
			default:
			}
		}
	}()
	return c
}

func prepareCommand(cmd []string) *exec.Cmd {
	c := exec.Command(cmd[0], cmd[1:]...)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return c
}

func waitCmd(cmd *exec.Cmd) error {
	return cmd.Wait()
}

func killChilds(c *exec.Cmd, sig syscall.Signal) error {
	err := syscall.Kill(-c.Process.Pid, sig)
	if err == nil && sig != syscall.SIGKILL && sig != syscall.SIGCONT {
		// prosess can be stopped, so it must be start by SIGCONT.
		err = syscall.Kill(-c.Process.Pid, syscall.SIGCONT)
	}
	return err
}
