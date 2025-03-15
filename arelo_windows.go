//go:build windows

package main

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/xerrors"
)

const STILL_ACTIVE = 259

var procC chan windows.Handle

func parseSignalOption(str string) (os.Signal, string) {
	if str == "" {
		return syscall.SIGTERM, "SIGTERM"
	}
	return nil, "Signal option (--signal, -s) is not available on Windows."
}

func prepareCommand(cmd []string, _ bool) *exec.Cmd {
	c := exec.Command(cmd[0], cmd[1:]...)
	c.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
	return c
}

// watchChild detects the termination of the child process by polling GetExitCodeProcess.
func watchChild(ctx context.Context, c *exec.Cmd) error {
	p, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(c.Process.Pid))
	if err != nil {
		return xerrors.Errorf("OpenProcess: %w", err)
	}
	defer windows.CloseHandle(p)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(*delay / 2):
		}

		var code uint32
		err := windows.GetExitCodeProcess(p, &code)
		if err != nil {
			return xerrors.Errorf("GetExitCodeProcess: %w", err)
		}
		if code != STILL_ACTIVE {
			return nil
		}
	}
}

func killChilds(c *exec.Cmd, _ syscall.Signal) error {
	kill := exec.Command("TASKKILL", "/T", "/F", "/PID", strconv.Itoa(c.Process.Pid))
	if *verbose {
		kill.Stderr = c.Stderr
		kill.Stdout = c.Stderr
	}
	return kill.Run()
}
