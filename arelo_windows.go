//go:build windows

package main

import (
	"bufio"
	"log"
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

// childWatcher monitors the process and close channel when it exits.
//
// On Windows, poll until GetExitCodeProcess() returns anything other than STILL_ACTIVE.
func childWatcher(p windows.Handle, done chan struct{}) {
	for {
		time.Sleep(*delay / 2)
		var code uint32
		err := windows.GetExitCodeProcess(p, &code)
		if err != nil {
			log.Printf("[ARELO] GetExitCodeProcess: %v", err)
			close(done)
			return
		}
		if code != STILL_ACTIVE {
			close(done)
			windows.CloseHandle(p)
			return
		}
	}
}

func prepareCommand(cmd []string) *exec.Cmd {
	c := exec.Command(cmd[0], cmd[1:]...)
	c.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
	return c
}

func startCommand(c *exec.Cmd, stdinC <-chan bytesErr) error {
	var childDone chan struct{}
	if stdinC != nil {
		childDone = make(chan struct{})

		c.Stdin = bufio.NewReader(&stdinReader{
			input: stdinC,
			done:  childDone,
		})
	}

	err := c.Start()
	if err != nil {
		if childDone != nil {
			close(childDone)
		}
		return err
	}

	if childDone != nil {
		p, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(c.Process.Pid))
		if err != nil {
			if err := killChilds(c, syscall.SIGINT); err != nil {
				log.Printf("[ARELO] killChilds: %v", err)
			}
			close(childDone)
			return xerrors.Errorf("OpenProcess: %w", err)
		}

		go childWatcher(p, childDone)
	}
	return nil
}

func killChilds(c *exec.Cmd, _ syscall.Signal) error {
	kill := exec.Command("TASKKILL", "/T", "/F", "/PID", strconv.Itoa(c.Process.Pid))
	kill.Stderr = c.Stderr
	kill.Stdout = c.Stderr
	return kill.Run()
}
