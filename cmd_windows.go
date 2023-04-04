//go:build windows

package main

import (
	"log"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/xerrors"
)

const STILL_ACTIVE = 259

var procC chan windows.Handle

// makeChildDoneChan returns a chan that notifies the child process has exited.
//
// On Windows, poll until GetExitCodeProcess() returns anything other than STILL_ACTIVE.
func makeChildDoneChan() <-chan struct{} {
	c := make(chan struct{}, 1)
	procC = make(chan windows.Handle)
	go func() {
		for {
			p := <-procC
			for {
				time.Sleep(*delay / 2)
				var code uint32
				err := windows.GetExitCodeProcess(p, &code)
				if err != nil {
					log.Printf("GetExitCodeProcess: %v", err)
					c <- struct{}{}
					break
				}
				if code != STILL_ACTIVE {
					c <- struct{}{}
					break
				}
			}
			windows.CloseHandle(p)
		}
	}()
	return c
}

func waitCmd(cmd *exec.Cmd) error {
	p, err := windows.OpenProcess(
		windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(cmd.Process.Pid))
	log.Printf("pid=%v handle=%v", cmd.Process.Pid, p)
	if err != nil {
		return xerrors.Errorf("OpenProcess: %w", err)
	}
	procC <- p
	return cmd.Wait()
}

func prepareCommand(cmd []string) *exec.Cmd {
	return exec.Command(cmd[0], cmd[1:]...)
}

func killChilds(c *exec.Cmd, sig syscall.Signal) error {
	kill := exec.Command("TASKKILL", "/T", "/F", "/PID", strconv.Itoa(c.Process.Pid))
	kill.Stderr = c.Stderr
	kill.Stdout = c.Stdout
	return kill.Run()
}
