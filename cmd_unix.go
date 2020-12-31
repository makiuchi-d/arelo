// +build !windows

package main

import (
	"os/exec"
	"syscall"
)

func prepareCommand(cmd []string) *exec.Cmd {
	c := exec.Command(cmd[0], cmd[1:]...)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return c
}

func killChilds(c *exec.Cmd, sig syscall.Signal) error {
	return syscall.Kill(-c.Process.Pid, sig)
}
