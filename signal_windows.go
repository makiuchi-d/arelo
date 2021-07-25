// +build windows

package main

import (
	"os"
	"syscall"
)

func parseSignalOption(str string) (os.Signal, string) {
	if str == "" {
		return syscall.SIGTERM, "SIGTERM"
	}
	return nil, "Signal option (--signal, -s) is not available on Windows."
}
