// +build !windows

package main

import (
	"fmt"
	"os"
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
	case "15", "TERM", "SIGTERM", "SIG_TERM":
		return syscall.SIGTERM, "SIGTERM"
	case "28", "WINCH", "SIGWINCH", "SIG_WINCH":
		return syscall.SIGWINCH, "SIGWINCH"
	}

	return nil, fmt.Sprintf("unspported signal: %s", str)
}
