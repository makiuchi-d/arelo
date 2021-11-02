//go:build windows
// +build windows

package main

import (
	"os/exec"
	"strconv"
	"syscall"
)

func prepareCommand(cmd []string) *exec.Cmd {
	return exec.Command(cmd[0], cmd[1:]...)
}

func killChilds(c *exec.Cmd, sig syscall.Signal) error {
	kill := exec.Command("TASKKILL", "/T", "/F", "/PID", strconv.Itoa(c.Process.Pid))
	kill.Stderr = c.Stderr
	kill.Stdout = c.Stdout
	return kill.Run()
}
