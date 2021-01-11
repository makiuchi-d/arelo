// +build !windows

package main

import (
	"os"
	"syscall"
	"testing"
)

func TestParseSignalOption(t *testing.T) {
	tests := []struct {
		inputs []string
		sig    os.Signal
		out    string
	}{
		{[]string{"1", "HUP", "SIGHUP", "SIG_HUP", "hup", "SigHup"}, syscall.SIGHUP, "SIGHUP"},
		{[]string{"2", "INT", "SIGINT", "SIG_INT", "int", "SigInt"}, syscall.SIGINT, "SIGINT"},
		{[]string{"9", "KILL", "SIGKILL", "SIG_KILL", "SIgKill"}, syscall.SIGKILL, "SIGKILL"},
		{[]string{"10", "USR1", "SIGUSR1", "SIG_USR1", "SIgUsr1"}, syscall.SIGUSR1, "SIGUSR1"},
		{[]string{"12", "USR2", "SIGUSR2", "SIG_USR2", "SIgUsr2"}, syscall.SIGUSR2, "SIGUSR2"},
		{[]string{"15", "TERM", "SIGTERM", "SIG_TERM", "SIgTerm"}, syscall.SIGTERM, "SIGTERM"},
	}
	for _, test := range tests {
		for _, in := range test.inputs {
			s, o := parseSignalOption(in)
			if s != test.sig || o != test.out {
				t.Fatalf("%q: got %q, %q, wants %q, %q", in, s, o, test.sig, test.out)
			}
		}
	}
}
